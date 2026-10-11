// Command large-api runs the development-only full-node audit under the registered
// native-query-large limits. It does not add API claims to summary oracle records.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

type receipt struct {
	Schema          string  `json:"schema"`
	ID              string  `json:"id"`
	SourceSHA       string  `json:"source_sha256"`
	SourceBytes     uint64  `json:"source_bytes"`
	Nodes           uint64  `json:"nodes"`
	Start           uint64  `json:"range_start"`
	End             uint64  `json:"range_end"`
	Depth           uint64  `json:"depth"`
	Fields          uint64  `json:"field_count"`
	Checks          uint64  `json:"checks"`
	ChildFields     uint64  `json:"child_fields"`
	FieldLookups    uint64  `json:"field_lookups"`
	PointChecks     uint64  `json:"point_checks"`
	Differences     uint64  `json:"differences"`
	Divergences     uint64  `json:"position_navigation_divergences"`
	FirstDivergence []int64 `json:"first_position_divergence"`
	First           []int64 `json:"first_difference"`
	ExpectedSHA     string  `json:"expected_sha256"`
	ObservedSHA     string  `json:"observed_sha256"`
	Complete        bool    `json:"complete"`
}

// This is the existing registered three-fixture inventory, not a caller-selected
// replacement workload. Updating it requires an explicit contract change.
const fixtureInventorySHA = "b9febb20fc2f6ba1e5b8d6d4b71b30f391b9cb32bf71eb1c4a68a8bd6f2f89f3"

type fixture struct {
	ID     string            `json:"id"`
	Header string            `json:"header"`
	Block  string            `json:"block"`
	Footer string            `json:"footer"`
	SHA256 string            `json:"sha256"`
	Repeat int               `json:"repeat"`
	Bytes  uint64            `json:"bytes"`
	Points []kit.NativePoint `json:"points"`
	Expect []json.RawMessage `json:"expect"`
}
type fixtureInventory struct {
	Schema    string    `json:"schema"`
	Route     string    `json:"route"`
	Operation string    `json:"operation"`
	Note      string    `json:"note"`
	Fixtures  []fixture `json:"fixtures"`
}

func loadFixtures(data []byte) (fixtureInventory, error) {
	var x fixtureInventory
	if digest(data) != fixtureInventorySHA {
		return x, errors.New("registered fixture inventory identity mismatch")
	}
	if e := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); e != nil {
		return x, e
	}
	if x.Schema != "tsgk-native-large-fixtures/r1" || x.Route != "csharp" || len(x.Fixtures) != 3 {
		return x, errors.New("fixture inventory invalid")
	}
	seen := map[string]bool{}
	for _, f := range x.Fixtures {
		if seen[f.ID] {
			return x, errors.New("duplicate fixture id")
		}
		seen[f.ID] = true
	}
	return x, nil
}

func digest(data []byte) string { x := sha256.Sum256(data); return hex.EncodeToString(x[:]) }
func decode(data []byte, req native.Request, start, end, expectedNodes uint64) (receipt, error) {
	var r receipt
	payload, err := native.SplitFrame(data)
	if err != nil {
		return r, err
	}
	if err = jsonv2.Unmarshal(payload, &r, jsonv2.RejectUnknownMembers(true)); err != nil {
		return r, err
	}
	for _, s := range []string{r.SourceSHA, r.ExpectedSHA, r.ObservedSHA} {
		x, e := hex.DecodeString(s)
		if e != nil || len(x) != 32 || strings.ToLower(s) != s {
			return r, errors.New("audit digest invalid")
		}
	}
	if r.Schema != "tsgk-large-api-audit/r1" || r.ID != req.ID || r.SourceBytes != uint64(len(req.Source)) || r.SourceSHA != digest(req.Source) || !r.Complete || r.Nodes == 0 || r.Nodes > req.Limits.Nodes || r.Depth == 0 || r.Depth > req.Limits.Depth || r.Fields > 65535 || len(r.First) != 4 || len(r.FirstDivergence) != 4 {
		return r, errors.New("audit identity or completion invalid")
	}
	if (expectedNodes != 0 && r.Nodes != expectedNodes) || r.Start != start || r.End != min(end, r.Nodes) || r.End <= r.Start || r.ChildFields > r.Nodes-1 || (r.Start == 0 && r.End == r.Nodes && r.ChildFields != r.Nodes-1) || r.FieldLookups != 2*(r.End-r.Start)*r.Fields || r.PointChecks != 3*uint64(len(req.Points)+2) || r.Checks != 13*(r.End-r.Start)+r.ChildFields+r.FieldLookups+r.PointChecks {
		return r, errors.New("audit coverage counts invalid")
	}
	if (r.Differences == 0) != (r.ExpectedSHA == r.ObservedSHA) || r.Differences > r.Checks || r.Divergences > 4*r.Nodes {
		return r, errors.New("audit comparison invalid")
	}
	if r.Differences == 0 {
		for _, v := range r.First {
			if v != 0 {
				return r, errors.New("unexpected first difference")
			}
		}
	} else if !((r.First[1] >= 0 && uint64(r.First[1]) < 13+r.ChildFields) || (r.First[1] >= 1000002 && uint64(r.First[1]) <= 1000001+2*r.Fields) || (r.First[1] >= 2000000 && r.First[1] <= 2000002)) || r.First[2] == r.First[3] || r.First[0] < 0 || r.First[1] < 0 || (r.First[1] < 2000000 && (uint64(r.First[0]) < r.Start || uint64(r.First[0]) >= r.End)) || (r.First[1] >= 2000000 && (r.First[1] > 2000002 || uint64(r.First[0]) >= uint64(len(req.Points)+2))) {
		return r, errors.New("missing first difference")
	}
	if r.Divergences == 0 {
		for _, v := range r.FirstDivergence {
			if v != 0 {
				return r, errors.New("unexpected positional divergence")
			}
		}
	} else if r.FirstDivergence[0] < 0 || uint64(r.FirstDivergence[0]) < r.Start || uint64(r.FirstDivergence[0]) >= r.End || r.FirstDivergence[1] < 1 || r.FirstDivergence[1] > 4 || r.FirstDivergence[2] == r.FirstDivergence[3] {
		return r, errors.New("positional divergence coordinates invalid")
	}
	return r, nil
}

func compile(ctx context.Context, b *native.Build, cc, source, define string) (string, []native.BuildStep, error) {
	compilerBytes, e := os.ReadFile(cc)
	if e != nil || digest(compilerBytes) != b.Compiler.SHA256 {
		return "", nil, errors.New("compiler identity changed")
	}
	exe := filepath.Join(b.Dir, "api-audit"+define)
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	args := []string{"-c", "-O2", "-std=gnu11", "-I", b.Dir, "-I", filepath.Join(b.Dir, "runtime/lib/include"), "-I", filepath.Join(b.Dir, "driver"), source, "-o", filepath.Join(b.Dir, "audit.o")}
	if define != "" {
		args = append(args, "-DTSGK_AUDIT_FAULT_"+define)
	}
	link := []string{filepath.Join(b.Dir, "lib.o"), filepath.Join(b.Dir, "parser.o")}
	for i := 0; ; i++ {
		p := filepath.Join(b.Dir, fmt.Sprintf("scanner%d.o", i))
		if _, e := os.Stat(p); os.IsNotExist(e) {
			break
		}
		link = append(link, p)
	}
	link = append(link, filepath.Join(b.Dir, "audit.o"), filepath.Join(b.Dir, "shim.o"), "-o", exe)
	tmp := filepath.Join(b.Dir, "tmp")
	env := []string{"PATH=" + filepath.Dir(cc), "TMP=" + tmp, "TEMP=" + tmp, "TMPDIR=" + tmp}
	for _, n := range []string{"SystemRoot", "WINDIR", "ProgramData", "ProgramFiles", "ProgramFiles(x86)", "PROCESSOR_ARCHITECTURE"} {
		if v, ok := os.LookupEnv(n); ok {
			env = append(env, n+"="+v)
		}
	}
	var results []native.BuildStep
	for _, argv := range [][]string{args, link} {
		r, e := runner.Run(ctx, runner.Spec{Path: cc, Args: argv, Dir: b.Dir, Env: env, StdoutBytes: 8388608, StderrBytes: 8388608, Wall: 120 * time.Second, Grace: 2 * time.Second, Memory: runner.Memory{Bytes: 4294967296, Hard: true}})
		results = append(results, native.BuildStep{Name: "audit", Argv: argv, Result: r, Executed: true})
		if e != nil || !r.Succeeded() {
			return exe, results, fmt.Errorf("audit compile refused/failed: %v; %s", e, string(r.Stderr))
		}
	}
	return exe, results, nil
}

type observation struct {
	Fault         string             `json:"fault,omitempty"`
	ExecutableSHA string             `json:"executable_sha256"`
	Compile       []native.BuildStep `json:"compile,omitempty"`
	Receipt       receipt            `json:"receipt"`
	Process       runner.Result      `json:"process"`
	Error         string             `json:"error,omitempty"`
	ExpectedFault bool               `json:"expected_fault"`
}

func run(ctx context.Context, exe, expectedSHA, dir string, req native.Request, start, end, expectedNodes uint64) observation {
	executable, err := os.ReadFile(exe)
	if err != nil || digest(executable) != expectedSHA {
		return observation{Error: "EXECUTABLE_IDENTITY_MISMATCH"}
	}
	op := kit.NativeOperations()["native-query-large"]
	r, err := runner.Run(ctx, runner.Spec{Path: exe, Args: []string{fmt.Sprint(start), fmt.Sprint(end)}, Dir: dir, Stdin: native.Frame(req.Encode()), StdoutBytes: int64(op.OutputBytes) + 4, StderrBytes: 65536, Wall: op.ParseWall, Grace: 5 * time.Second, Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: true}})
	out := observation{Process: r}
	if executable, e := os.ReadFile(exe); e == nil {
		out.ExecutableSHA = digest(executable)
		if out.ExecutableSHA != expectedSHA {
			out.Error = "EXECUTABLE_CHANGED"
			return out
		}
	} else {
		out.Error = "EXECUTABLE_IDENTITY_UNREADABLE"
		return out
	}
	if err != nil {
		out.Error = "RUN_REFUSED"
		return out
	}
	out.Receipt, err = decode(r.Stdout, req, start, end, expectedNodes)
	if err != nil {
		if payload, e := native.SplitFrame(r.Stdout); e == nil {
			if failure, e := native.DecodeResponse(payload); e == nil && failure.Status == kit.StatusResourceLimit && failure.Code == "ALLOCATION_LIMIT" && r.Cleanup.Verified && !r.StdoutTruncated && !r.StderrTruncated && r.Memory.Enforcement == runner.MemoryHard {
				if _, e = native.Check(failure, req, [][]byte{req.Source}, nil, r.ExitCode); e == nil && failure.StepsCompleted == 0 {
					out.Error = "RESOURCE_LIMIT:ALLOCATION_LIMIT"
					return out
				}
			}
		}
		out.Error = err.Error()
		return out
	}
	if !r.Cleanup.Verified || r.StdoutTruncated || r.StderrTruncated || r.Memory.Enforcement != runner.MemoryHard || (out.Receipt.Differences == 0 && (r.Status != runner.StatusCompleted || r.ExitCode != 0)) || (out.Receipt.Differences > 0 && (r.Status != runner.StatusFailed || r.Reason != runner.ReasonExited || r.ExitCode != 1)) {
		out.Error = "PROCESS_OR_COMPLETION_INVALID"
	}
	return out
}

func execute() error {
	prepared := flag.String("prepared", "", "verified preparation directory")
	cc := flag.String("cc", "", "pinned compiler path")
	work := flag.String("work", "", "new task-owned build directory")
	out := flag.String("out", "", "new receipt file")
	controlsOnly := flag.Bool("controls-only", false, "small controls and faults only")
	flag.Parse()
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return errors.New("large API audit currently requires windows/amd64")
	}
	for _, p := range []*string{prepared, cc, work, out} {
		if *p == "" {
			return errors.New("missing path")
		}
		a, e := filepath.Abs(*p)
		if e != nil {
			return e
		}
		*p = a
	}
	if _, e := os.Lstat(*work); !os.IsNotExist(e) {
		return errors.New("work already exists")
	}
	if _, e := os.Lstat(*out); !os.IsNotExist(e) {
		return errors.New("output already exists")
	}
	if err := os.Mkdir(*work, 0755); err != nil {
		return err
	}
	repo, e := os.Getwd()
	if e != nil {
		return e
	}
	source := filepath.Join(repo, "src/dev/large-api/audit.c")
	auditSource, e := os.ReadFile(source)
	if e != nil {
		return e
	}
	ccBytes, e := os.ReadFile(*cc)
	if e != nil {
		return e
	}
	compiler := kit.ToolIdentity{Name: "cc", Version: "host", SHA256: digest(ccBytes), Bytes: uint64(len(ccBytes))}
	var registry struct {
		Routes []struct {
			Route  string            `json:"route"`
			Symbol string            `json:"symbol"`
			Files  []kit.NativeInput `json:"files"`
		} `json:"routes"`
	}
	data, e := os.ReadFile("src/contracts/native-routes.json")
	if e != nil {
		return e
	}
	registrySHA := digest(data)
	if e = jsonv2.Unmarshal(data, &registry); e != nil {
		return e
	}
	var files []kit.NativeInput
	var symbol string
	selected := 0
	for _, r := range registry.Routes {
		if r.Route == "csharp" {
			selected++
			files = r.Files
			symbol = r.Symbol
		}
	}
	if len(files) == 0 || selected != 1 {
		return errors.New("csharp registration absent")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	b, e := native.NewBuild(ctx, native.BuildRequest{Work: *work, Runtime: filepath.Join(*prepared, "runtime"), GrammarRoot: filepath.Join(*prepared, "routes/csharp"), Grammar: files, Symbol: symbol, Compiler: *cc, CompilerID: compiler})
	if e != nil {
		return e
	}
	source = filepath.Join(b.Dir, "audit.c")
	if e = os.WriteFile(source, auditSource, 0644); e != nil {
		return e
	}
	exe, steps, e := compile(ctx, b, *cc, source, "")
	if e != nil {
		return e
	}
	executable, e := os.ReadFile(exe)
	if e != nil {
		return e
	}
	result := struct {
		Schema              string              `json:"schema"`
		Mode                string              `json:"mode"`
		LargeComplete       bool                `json:"large_complete"`
		FixtureInventorySHA string              `json:"fixture_inventory_sha256"`
		RouteRegistrySHA    string              `json:"route_registry_sha256"`
		AuditSHA            string              `json:"audit_source_sha256"`
		ExecutableSHA       string              `json:"audit_executable_sha256"`
		Build               *native.Build       `json:"base_build"`
		Compile             []native.BuildStep  `json:"audit_compile"`
		Observations        []observation       `json:"observations"`
		SmallOracle         []native.CaseResult `json:"small_full_oracle"`
		ResourceControls    []observation       `json:"resource_controls"`
		LargeBaseline       []native.CaseResult `json:"large_baseline"`
		Faults              []observation       `json:"faults"`
		Pass                bool                `json:"pass"`
	}{Mode: "REGISTERED_LARGE_ALL_NODES", FixtureInventorySHA: fixtureInventorySHA, RouteRegistrySHA: registrySHA, Schema: "tsgk-large-api-validation/r1", AuditSHA: digest(auditSource), ExecutableSHA: digest(executable), Build: b, Compile: steps, Pass: true}
	if *controlsOnly {
		result.Schema = "tsgk-large-api-controls/r1"
		result.Mode = "SMALL_CONTROLS"
	}
	completedFixtures := 0
	op := kit.NativeOperations()["native-query-large"]
	small := []string{"    class C { int M(int a) { return a + 1; } }\n", "class C { int M(int a) { return a + ; } }\n", "class C { void M() { F(1; } }\n", "class C { int M() { return 1 } }\n"}
	var first native.Request
	var firstNodes uint64
	var zero native.Request
	var zeroNodes uint64
	for i, s := range small {
		req := native.Request{ID: fmt.Sprintf("small-%d", i), Encoding: kit.EncodingUTF8, Output: kit.OutputAuto, Limits: native.LimitsFor(op), Source: []byte(s), Points: []kit.NativePoint{{ID: "middle", Byte: uint32(len(s) / 2)}}}
		if i == 0 {
			first = req
		}
		nq := kit.NativeOperations()["native-query"]
		oracle := b.RunCase(ctx, native.Context{Op: nq, Route: "csharp", Output: kit.OutputTree, Protocol: native.ProtocolR2, API: true}, kit.IncrementalCase{ID: req.ID, Encoding: kit.EncodingUTF8, Points: req.Points}, req.Source)
		result.SmallOracle = append(result.SmallOracle, oracle)
		if oracle.ExecutionStatus != kit.StatusCompleted || oracle.Oracle == nil || oracle.Oracle.API != native.ClaimPass || len(oracle.Steps) != 1 || oracle.Steps[0].Incremental == nil || oracle.Steps[0].Incremental.API == nil || oracle.Steps[0].Incremental.API.PositionNavigation != 0 {
			result.Pass = false
			continue
		}
		nodes := oracle.Steps[0].Incremental.DescendantCount
		if i == 0 {
			if oracle.Steps[0].Incremental.Tree.Nodes[0].StartByte != 4 {
				result.Pass = false
				continue
			}
			firstNodes = nodes
		}
		if i == 3 {
			zero = req
			zeroNodes = nodes
		}
		o := run(ctx, exe, result.ExecutableSHA, b.Dir, req, 0, op.Nodes, nodes)
		result.Observations = append(result.Observations, o)
		if o.Error != "" || o.Receipt.Differences != 0 || o.Receipt.Divergences != 0 || oracle.Producer == nil || oracle.Producer.FieldCount == nil || o.Receipt.Fields != uint64(*oracle.Producer.FieldCount) {
			result.Pass = false
		}
	}
	for _, fault := range []string{"PARENT", "SIBLING", "COUNT", "CURSOR", "FIELD_NAME", "FIELD_LOOKUP", "POINT", "POSITIVE_SKIP", "ZERO_SKIP"} {
		mutant, steps, e := compile(ctx, b, *cc, source, fault)
		if e != nil {
			return e
		}
		input, nodes := first, firstNodes
		if fault == "ZERO_SKIP" {
			input, nodes = zero, zeroNodes
		}
		mutantBytes, e := os.ReadFile(mutant)
		if e != nil {
			return e
		}
		o := run(ctx, mutant, digest(mutantBytes), b.Dir, input, 0, op.Nodes, nodes)
		o.ExpectedFault = true
		o.Fault = fault
		o.Compile = steps
		result.Faults = append(result.Faults, o)
		if fault == "ZERO_SKIP" {
			if o.Error != "" || o.Receipt.Differences != 0 || o.Receipt.Divergences == 0 {
				result.Pass = false
			}
		} else if o.Error != "" || o.Receipt.Differences == 0 {
			result.Pass = false
		}
	}
	resource := first
	resource.Limits.MemoryBytes = 1
	limited := run(ctx, exe, result.ExecutableSHA, b.Dir, resource, 0, op.Nodes, firstNodes)
	result.ResourceControls = append(result.ResourceControls, limited)
	if limited.Error != "RESOURCE_LIMIT:ALLOCATION_LIMIT" {
		result.Pass = false
	}
	if !*controlsOnly && result.Pass {
		data, e := os.ReadFile("src/contracts/native-large-fixtures.json")
		if e != nil {
			return e
		}
		fixtures, e := loadFixtures(data)
		if e != nil {
			return e
		}
		for _, f := range fixtures.Fixtures {
			var source bytes.Buffer
			source.WriteString(f.Header)
			for n := 0; n < f.Repeat; n++ {
				source.WriteString(strings.ReplaceAll(f.Block, "{N}", fmt.Sprint(n)))
			}
			source.WriteString(f.Footer)
			if uint64(source.Len()) != f.Bytes || digest(source.Bytes()) != f.SHA256 {
				return errors.New("registered fixture identity mismatch")
			}
			req := native.Request{ID: f.ID, Encoding: kit.EncodingUTF8, Output: kit.OutputAuto, Limits: native.LimitsFor(op), Source: source.Bytes(), Points: f.Points}
			baseline := b.RunCase(ctx, native.Context{Op: op, Route: "csharp", Output: kit.OutputAuto}, kit.IncrementalCase{ID: req.ID, Encoding: kit.EncodingUTF8, Points: req.Points}, req.Source)
			result.LargeBaseline = append(result.LargeBaseline, baseline)
			if baseline.ExecutionStatus != kit.StatusCompleted || len(baseline.Steps) != 1 || baseline.Steps[0].Incremental == nil || baseline.Steps[0].Incremental.Status != kit.StatusCompleted {
				result.Pass = false
				break
			}
			expectedNodes := baseline.Steps[0].Incremental.DescendantCount
			var covered, nodes, children uint64
			for {
				o := run(ctx, exe, result.ExecutableSHA, b.Dir, req, covered, min(covered+1000000, op.Nodes), expectedNodes)
				result.Observations = append(result.Observations, o)
				fmt.Printf("audit %s: range=%d..%d nodes=%d differences=%d status=%s\n", f.ID, o.Receipt.Start, o.Receipt.End, o.Receipt.Nodes, o.Receipt.Differences, o.Process.Status)
				if o.Error != "" || o.Receipt.Differences != 0 || o.Receipt.Divergences != 0 || o.Receipt.Fields != result.Observations[0].Receipt.Fields || (nodes != 0 && nodes != o.Receipt.Nodes) {
					result.Pass = false
					break
				}
				nodes = o.Receipt.Nodes
				children += o.Receipt.ChildFields
				covered = o.Receipt.End
				if covered == nodes {
					if children != nodes-1 {
						result.Pass = false
					} else {
						completedFixtures++
					}
					break
				}
			}
		}
	}
	if !*controlsOnly {
		result.LargeComplete = result.Pass && completedFixtures == 3
		result.Pass = result.LargeComplete
	}
	data, e = json.MarshalIndent(result, "", "  ")
	if e != nil {
		return e
	}
	file, e := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if e != nil {
		return e
	}
	_, e = file.Write(data)
	closeErr := file.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if !result.Pass {
		return errors.New("audit verification failed; receipt retained")
	}
	return nil
}
func main() {
	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
