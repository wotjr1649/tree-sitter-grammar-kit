package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/largeaudit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

type auditObservation struct {
	Fault         string             `json:"fault"`
	Executable    string             `json:"executable_sha256"`
	Receipt       json.RawMessage    `json:"receipt"`
	Process       json.RawMessage    `json:"process"`
	Compile       []native.BuildStep `json:"compile"`
	Error         string             `json:"error"`
	ExpectedFault bool               `json:"expected_fault"`
}
type largeResult struct {
	Schema       string              `json:"schema"`
	Mode         string              `json:"mode"`
	Complete     bool                `json:"large_complete"`
	Pass         bool                `json:"pass"`
	Inventory    string              `json:"fixture_inventory_sha256"`
	Registry     string              `json:"route_registry_sha256"`
	Source       string              `json:"audit_source_sha256"`
	Executable   string              `json:"audit_executable_sha256"`
	Build        *native.Build       `json:"base_build"`
	Compile      []native.BuildStep  `json:"audit_compile"`
	Observations []auditObservation  `json:"observations"`
	Small        []native.CaseResult `json:"small_full_oracle"`
	Baseline     []native.CaseResult `json:"large_baseline"`
	Faults       []auditObservation  `json:"faults"`
	Resources    []auditObservation  `json:"resource_controls"`
}

func requiredFields(data []byte, keys ...string) error {
	var object map[string]json.RawMessage
	if decode(data, &object) != nil {
		return errors.New("object invalid")
	}
	for _, key := range keys {
		value, ok := object[key]
		if !ok || strings.TrimSpace(string(value)) == "null" {
			return errors.New("required field missing or null")
		}
	}
	return nil
}

func processResult(data []byte, status string, code int) (runner.Result, error) {
	var p runner.Result
	if requiredFields(data, "status", "exit_code", "stdout_bytes", "stderr_bytes", "stdout_truncated", "stderr_truncated", "wall_ms", "cleanup", "memory") != nil || decode(data, &p) != nil {
		return p, errors.New("audit process invalid")
	}
	var object map[string]json.RawMessage
	if decode(data, &object) != nil || requiredFields(object["cleanup"], "verified", "residual_after_exit") != nil || requiredFields(object["memory"], "enforcement", "limit_bytes", "peak_bytes", "peak_known", "limit_hit") != nil {
		return p, errors.New("audit process limits absent")
	}
	op := kit.NativeOperations()["native-query-large"]
	if p.Status != status || p.ExitCode != code || p.StdoutTruncated || p.StderrTruncated || !p.Cleanup.Verified || p.Cleanup.ResidualAfterExit != 0 ||
		p.Memory.Enforcement != "HARD" || !p.Memory.PeakKnown || p.Memory.Limit != op.MemoryBytes || p.Memory.LimitHit || p.Memory.Peak > p.Memory.Limit || p.WallMillis < 0 || p.WallMillis > op.ParseWall.Milliseconds() || p.StdoutBytes < 0 || p.StdoutBytes > int64(op.OutputBytes)+4 || p.StderrBytes < 0 || p.StderrBytes > 65536 {
		return p, errors.New("audit process incomplete or outside limits")
	}
	return p, nil
}

func compiled(steps []native.BuildStep) bool {
	if len(steps) == 0 {
		return false
	}
	for _, s := range steps {
		if !s.Executed || !s.Result.Succeeded() || s.Result.StdoutTruncated || s.Result.StderrTruncated {
			return false
		}
	}
	return true
}

func baseline(r native.CaseResult, req native.Request) (uint64, error) {
	if r.ID != req.ID || r.ExecutionStatus != kit.StatusCompleted || r.Assessment != kit.AssessPass || len(r.Steps) != 1 || r.Process == nil || !r.Process.Succeeded() || r.Producer == nil {
		return 0, errors.New("large baseline incomplete")
	}
	s := r.Steps[0]
	if s.Step != 0 || s.SourceSHA256 != digest(req.Source) || s.SourceBytes != uint64(len(req.Source)) || s.Incremental == nil || s.Incremental.Status != kit.StatusCompleted || s.Incremental.DescendantCount == 0 || s.Incremental.DescendantCount > req.Limits.Nodes {
		return 0, errors.New("large baseline source or node count invalid")
	}
	return s.Incremental.DescendantCount, nil
}

func verifyLarge(root string, expected kit.RunIdentity) (map[string]any, error) {
	dir := filepath.Join(root, "large-api-"+expected.RunID+"-"+expected.RunAttempt)
	identity, err := readFile(filepath.Join(dir, "large-api-identity-"+expected.RunID+"-"+expected.RunAttempt, "run-identity.json"), 65536)
	if err != nil {
		return nil, err
	}
	var observed kit.RunIdentity
	if decode(identity, &observed) != nil || !sameRun(&observed, expected) || observed.Job != "large-api" || observed.GOOS != "windows" || observed.GOARCH != "amd64" {
		return nil, errors.New("large API run mismatch")
	}
	data, err := readFile(filepath.Join(dir, "large-api-"+expected.RunID+"-"+expected.RunAttempt+".json"), 16777216)
	if err != nil {
		return nil, err
	}
	fixtureBytes, err := readFile("src/contracts/native-large-fixtures.json", 1048576)
	if err != nil {
		return nil, err
	}
	fixtures, err := largeaudit.LoadFixtures(fixtureBytes)
	if err != nil {
		return nil, errors.New("registered large fixtures invalid")
	}
	registry, err := readFile("src/contracts/native-routes.json", 16777216)
	if err != nil {
		return nil, err
	}
	source, err := readFile("src/dev/large-api/audit.c", 1048576)
	if err != nil {
		return nil, err
	}
	return checkLarge(data, fixtures, digest(registry), digest(source))
}

func checkLarge(data []byte, fixtures largeaudit.Inventory, registrySHA, sourceSHA string) (map[string]any, error) {
	var result largeResult
	if decode(data, &result) != nil {
		return nil, errors.New("large API JSON invalid")
	}
	var raw struct {
		Observations []json.RawMessage `json:"observations"`
		Faults       []json.RawMessage `json:"faults"`
		Resources    []json.RawMessage `json:"resource_controls"`
	}
	if decode(data, &raw) != nil {
		return nil, errors.New("large API observations invalid")
	}
	for _, group := range [][]json.RawMessage{raw.Observations, raw.Faults, raw.Resources} {
		for _, o := range group {
			if requiredFields(o, "receipt", "process", "executable_sha256", "expected_fault") != nil {
				return nil, errors.New("audit observation fields absent")
			}
		}
	}
	runtime, err := native.LoadRuntimeManifest()
	if err != nil {
		return nil, errors.New("runtime manifest unavailable")
	}
	if result.Schema != "tsgk-large-api-validation/r1" || result.Mode != "REGISTERED_LARGE_ALL_NODES" || !result.Pass || !result.Complete ||
		result.Inventory != largeaudit.FixtureInventorySHA || result.Registry != registrySHA || result.Source != sourceSHA || !hex64.MatchString(result.Executable) ||
		result.Build == nil || !hex64.MatchString(result.Build.Identity) || result.Build.Runtime != runtime.Commit || result.Build.Symbol != "tree_sitter_c_sharp" ||
		len(result.Build.Defines) != 0 || result.Build.Sanitize || !compiled(result.Build.Steps) || !compiled(result.Compile) ||
		len(result.Small) != 4 || len(result.Baseline) != 3 || len(fixtures.Fixtures) != 3 || len(result.Faults) != 9 || len(result.Resources) != 1 {
		return nil, errors.New("large API gate incomplete or unsuccessful")
	}
	op := kit.NativeOperations()["native-query-large"]
	requests := map[string]native.Request{}
	nodes := map[string]uint64{}
	covered := map[string]uint64{}
	children := map[string]uint64{}
	seen := map[string]int{}
	for i, source := range largeaudit.SmallSources() {
		id := fmt.Sprintf("small-%d", i)
		req := native.Request{ID: id, Encoding: kit.EncodingUTF8, Output: kit.OutputAuto, Limits: native.LimitsFor(op), Source: []byte(source), Points: []kit.NativePoint{{ID: "middle", Byte: uint32(len(source) / 2)}}}
		r := result.Small[i]
		n, e := baseline(r, req)
		if e != nil || r.Oracle == nil || r.Oracle.API != native.ClaimPass || r.Producer.FieldCount == nil || r.Steps[0].Incremental.API == nil || r.Steps[0].Incremental.API.Revision != "tsgk-api/r1" || !r.Steps[0].Incremental.API.Consistent || r.Steps[0].Incremental.API.First != nil || r.Steps[0].Incremental.API.PositionNavigation != 0 || r.Steps[0].Incremental.API.FirstDivergence != nil || r.Steps[0].Incremental.Tree == nil {
			return nil, errors.New("small full oracle invalid")
		}
		tree := r.Steps[0].Incremental.Tree
		if tree.Schema != kit.TreeSchema || tree.Status != kit.StatusCompleted || tree.Input.SHA256 != digest(req.Source) || tree.Input.Bytes != uint64(len(req.Source)) || tree.Input.Encoding != kit.EncodingUTF8 || kit.ValidateTree(tree.Nodes, uint64(len(req.Source))) != nil || uint64(len(tree.Nodes)) != n || kit.TreeDigest(tree.Nodes) != r.Steps[0].Incremental.Digest {
			return nil, errors.New("small full oracle tree invalid")
		}
		requests[id] = req
		nodes[id] = n
	}
	for i, f := range fixtures.Fixtures {
		source, e := largeaudit.Source(f)
		if e != nil {
			return nil, e
		}
		req := native.Request{ID: f.ID, Encoding: kit.EncodingUTF8, Output: kit.OutputAuto, Limits: native.LimitsFor(op), Source: source, Points: f.Points}
		n, e := baseline(result.Baseline[i], req)
		if e != nil {
			return nil, e
		}
		requests[f.ID] = req
		nodes[f.ID] = n
	}
	fields := *result.Small[0].Producer.FieldCount
	for _, r := range result.Small {
		if *r.Producer.FieldCount != fields {
			return nil, errors.New("small baseline field inventory mismatch")
		}
	}
	// Large baselines are native r1 and do not declare fields. The four r2 full
	// controls bind the inventory used by every page and fault of the same build.
	producer := result.Small[0].Producer
	for _, r := range result.Baseline {
		p := r.Producer
		if p.Query != "UNSUPPORTED" || p.FieldCount != nil || p.LanguageVersion != producer.LanguageVersion || p.RuntimeLanguageVersion != producer.RuntimeLanguageVersion || p.RuntimeMinCompatible != producer.RuntimeMinCompatible {
			return nil, errors.New("large r1 baseline producer mismatch")
		}
	}
	var wall int64
	var memory uint64
	pages := 0
	for _, o := range result.Observations {
		var header struct {
			ID string `json:"id"`
		}
		if decode(o.Receipt, &header) != nil {
			return nil, errors.New("audit receipt unavailable")
		}
		req, ok := requests[header.ID]
		if !ok || o.ExpectedFault || o.Fault != "" || o.Executable != result.Executable || o.Error != "" {
			return nil, errors.New("unexpected audit observation")
		}
		start, end := uint64(0), nodes[req.ID]
		if !strings.HasPrefix(req.ID, "small-") {
			start = covered[req.ID]
			end = min(start+1000000, nodes[req.ID])
			pages++
		}
		r, e := largeaudit.Decode(o.Receipt, req, start, end, nodes[req.ID])
		if e != nil || r.Differences != 0 || r.Divergences != 0 {
			return nil, errors.New("audit page invalid or differs")
		}
		p, e := processResult(o.Process, runner.StatusCompleted, 0)
		if e != nil || p.WallMillis > op.ParseWall.Milliseconds() || p.Memory.Limit != op.MemoryBytes {
			return nil, errors.New("audit page process outside limits")
		}
		if r.Fields != uint64(*result.Small[0].Producer.FieldCount) || strings.HasPrefix(req.ID, "small-") && r.Fields != uint64(*result.Small[int(req.ID[len(req.ID)-1]-'0')].Producer.FieldCount) {
			return nil, errors.New("audit field inventory mismatch")
		}
		seen[req.ID]++
		covered[req.ID] = r.End
		children[req.ID] += r.ChildFields
		wall = max(wall, p.WallMillis)
		memory = max(memory, p.Memory.Peak)
	}
	for id, n := range nodes {
		if covered[id] != n || children[id] != n-1 || seen[id] == 0 || strings.HasPrefix(id, "small-") && seen[id] != 1 {
			return nil, errors.New("audit page coverage incomplete")
		}
	}
	faults := map[string]bool{}
	executables := map[string]bool{result.Executable: true}
	for _, o := range result.Faults {
		if !slices.Contains([]string{"PARENT", "SIBLING", "COUNT", "CURSOR", "FIELD_NAME", "FIELD_LOOKUP", "POINT", "POSITIVE_SKIP", "ZERO_SKIP"}, o.Fault) || faults[o.Fault] || !o.ExpectedFault || o.Error != "" || !hex64.MatchString(o.Executable) || executables[o.Executable] || !compiled(o.Compile) {
			return nil, errors.New("fault control identity invalid")
		}
		faults[o.Fault] = true
		executables[o.Executable] = true
		defines := 0
		for _, step := range o.Compile {
			for _, arg := range step.Argv {
				if strings.HasPrefix(arg, "-DTSGK_AUDIT_FAULT_") {
					if arg != "-DTSGK_AUDIT_FAULT_"+o.Fault {
						return nil, errors.New("fault define mismatch")
					}
					defines++
				}
			}
		}
		if defines != 1 {
			return nil, errors.New("fault define absent")
		}
		id, status, code := "small-0", runner.StatusFailed, 1
		if o.Fault == "ZERO_SKIP" {
			id, status, code = "small-3", runner.StatusCompleted, 0
		}
		r, e := largeaudit.Decode(o.Receipt, requests[id], 0, nodes[id], nodes[id])
		if e != nil || r.Fields != uint64(fields) || (o.Fault == "ZERO_SKIP" && (r.Differences != 0 || r.Divergences == 0)) || (o.Fault != "ZERO_SKIP" && r.Differences == 0) {
			return nil, errors.New("fault not detected")
		}
		p, e := processResult(o.Process, status, code)
		if e != nil {
			return nil, e
		}
		if o.Fault != "ZERO_SKIP" && p.Reason != runner.ReasonExited {
			return nil, errors.New("fault process termination reason mismatch")
		}
	}
	resource := result.Resources[0]
	if resource.Executable != result.Executable || resource.Error != "RESOURCE_LIMIT:ALLOCATION_LIMIT" || resource.ExpectedFault || resource.Fault != "" {
		return nil, errors.New("resource control absent")
	}
	// Live capture validates the typed native failure, then retains an empty audit
	// receipt; the native failure frame itself is not part of this artifact schema.
	var resourceRaw, expectedEmpty map[string]json.RawMessage
	zero, _ := json.Marshal(largeaudit.Receipt{})
	if decode(resource.Receipt, &resourceRaw) != nil || decode(zero, &expectedEmpty) != nil || len(resourceRaw) != len(expectedEmpty) {
		return nil, errors.New("resource receipt shape invalid")
	}
	for key, want := range expectedEmpty {
		if value, ok := resourceRaw[key]; !ok || strings.TrimSpace(string(value)) != string(want) {
			return nil, errors.New("resource receipt must be complete empty audit result")
		}
	}
	if _, e := processResult(resource.Process, runner.StatusFailed, 3); e != nil {
		return nil, e
	}
	return map[string]any{"pages": pages, "fixtures": 3, "faults": len(faults), "resource_controls": 1, "max_page_wall_ms": wall, "max_memory_bytes": memory}, nil
}
