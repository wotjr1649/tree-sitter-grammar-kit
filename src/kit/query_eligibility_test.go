package kit

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	jsonv2 "encoding/json/v2"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// This synthetic, caller-approved fixture exercises offline consistency, not execution.
type eligibilityFixture struct {
	t        *testing.T
	cat      eligibilityCatalogue
	reg      eligibilityRegistration
	approval eligibilityApproval
	witness  eligibilityWitness
	files    map[string][]byte
	nodes    map[string]EvidenceNode
	root     string
}

func eligibilityJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := jsonv2.Marshal(v, jsonv2.Deterministic(true), jsonv2.FormatNilSliceAsNull(true))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (f *eligibilityFixture) node(id, source string) EvidenceNode {
	if n, ok := f.nodes[id]; ok {
		return n
	}
	n := EvidenceNode{ID: id, Kind: "run", ExecutionStatus: StatusCompleted, EvidenceMode: ModeNewRun, Assessment: AssessPass, Identities: map[string]string{}, Files: []ReplayMember{}, Refs: []EvidenceRef{}}
	for _, role := range EvidenceRoles {
		n.Identities[role] = "fixture-" + role
	}
	n.Identities["source"] = source
	n.Identities["platform"] = "linux/amd64"
	n.Identities["run"] = id
	n.Identities["attempt"] = "1"
	f.nodes[id] = n
	return n
}

func (f *eligibilityFixture) member(owner, path, role string, data []byte) eligibilityMember {
	if _, ok := f.files[path]; ok {
		f.t.Fatalf("duplicate fixture member %s", path)
	}
	m := eligibilityMember{NodeID: owner, Member: ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)}}
	f.files[path] = data
	f.reg.OwnedMembers = append(f.reg.OwnedMembers, m)
	n := f.node(owner, strings.Repeat("a", 40))
	n.Files = append(n.Files, m.Member)
	f.nodes[owner] = n
	return m
}

func eligibilityFixtureExecution(req eligibilityProcessRequest, exit int, truncated bool, stdout, stderr []byte) eligibilityExecution {
	x := eligibilityExecution{Status: StatusCompleted, Reason: "EXITED", ExitCode: exit, StdoutBytes: uint64(len(stdout)), StderrBytes: uint64(len(stderr))}
	if exit != 0 {
		x.Status = StatusFailed
	}
	x.Cleanup.Method, x.Cleanup.Scope, x.Cleanup.Verified = "CGROUP_KILL", "CGROUP_KILL", true
	x.Memory.Limit, x.Memory.Enforcement, x.Memory.Metric, x.Memory.PeakKnown = req.Memory.Bytes, "HARD", "cgroup v2 memory.max/memory.peak (bytes, includes page cache)", true
	x.Capabilities.OS, x.Capabilities.Backend, x.Capabilities.TreeCleanup, x.Capabilities.EscapedDescendants = "linux/amd64", "linux-cgroup-v2", "CGROUP_KILL", "CONTAINED"
	x.Capabilities.Memory, x.Capabilities.MemoryMetric, x.Capabilities.PoliteTermination = "HARD", x.Memory.Metric, true
	if truncated {
		x.Status, x.Reason, x.StdoutTruncated = StatusResourceLimit, "OUTPUT_LIMIT", true
		x.StdoutBytes += 500
	}
	return x
}

func (f *eligibilityFixture) invocation(id, cohort, catalogue string, req eligibilityProcessRequest, stdout []byte, exit int, truncated bool) *eligibilityInvocation {
	n := f.node(id, strings.Repeat("a", 40))
	if exit != 0 {
		n.ExecutionStatus, n.Assessment = StatusFailed, AssessNotAssessed
	}
	if truncated {
		n.ExecutionStatus = StatusResourceLimit
	}
	f.nodes[id] = n
	request := f.member(id, id+"/request.json", "request", eligibilityJSON(f.t, req))
	receipt := eligibilityReceipt{Schema: "tsgk-reference-process/r1", Execution: eligibilityFixtureExecution(req, exit, truncated, stdout, []byte{}), StdoutBase64: base64.StdEncoding.EncodeToString(stdout), StderrBase64: ""}
	saved := f.member(id, id+"/receipt.json", "receipt", eligibilityJSON(f.t, receipt))
	inv := eligibilityInvocation{ID: id, Kind: "REFERENCE_PROCESS", CohortID: cohort, CatalogueID: catalogue, Node: QueryReferenceNode{ID: id, Identities: n.Identities}, Request: &request, Receipt: &saved, ApprovedRequest: eligibilityJSON(f.t, req), Inputs: map[string]eligibilityInput{}, ExitAttestation: &eligibilityExit{Basis: "reviewed-inline/r1", Value: exit, ProvenanceID: "p-" + cohort}}
	f.reg.Invocations = append(f.reg.Invocations, inv)
	return &f.reg.Invocations[len(f.reg.Invocations)-1]
}

func eligibilityFixtureEnv(root string, tools map[string]string, build bool, target string) []string {
	env := map[string]string{"PATH": "/tools:/usr/bin:/bin", "TEMP": root + "/tmp", "TMP": root + "/tmp", "TMPDIR": root + "/tmp"}
	if build {
		env["PATH"] = "/tools:/tools:/usr/bin:/bin"
		env["CARGO_HOME"] = root + "/cargo-home"
		env["CARGO_TARGET_DIR"] = root + "/target"
		env["CARGO_NET_OFFLINE"] = "true"
		env["CARGO_TERM_COLOR"] = "never"
		env["CARGO_INCREMENTAL"] = "0"
		env["RUSTC"] = tools["rustc"]
		env["GIT_CONFIG_NOSYSTEM"] = "1"
		env["GIT_CONFIG_GLOBAL"] = "/dev/null"
		env["GIT_CEILING_DIRECTORIES"] = root
		key := strings.ReplaceAll(target, "-", "_")
		env["CC_"+key] = tools["cc"]
		env["AR_"+key] = tools["ar"]
		env["CARGO_TARGET_"+strings.ToUpper(key)+"_LINKER"] = tools["cc"]
	}
	rows := []string{}
	for k, v := range env {
		rows = append(rows, k+"="+v)
	}
	slices.Sort(rows)
	return rows
}

func eligibilityFixtureExpand(value string, paths map[string]string) string {
	for k, v := range paths {
		value = strings.ReplaceAll(value, "{"+k+"}", v)
	}
	return strings.ReplaceAll(value, "{SHARED}", "-shared")
}
func eligibilityFixtureSpec(rule eligibilityBuildRule, paths, tools map[string]string, build bool) eligibilityProcessRequest {
	req := eligibilityProcessRequest{Path: tools[rule.Tool], Args: []string{}, Dir: eligibilityFixtureExpand(rule.Dir, paths), Env: eligibilityFixtureEnv(paths["C"], tools, false, paths["T"]), StdoutBytes: rule.StdoutBytes, StderrBytes: rule.StderrBytes, Wall: rule.WallSeconds * 1000000000, Grace: rule.GraceSeconds * 1000000000, CgroupParent: "/delegated"}
	if build {
		req.Env = eligibilityFixtureEnv(paths["B"], tools, true, paths["T"])
	}
	for _, arg := range rule.Args {
		req.Args = append(req.Args, eligibilityFixtureExpand(arg, paths))
	}
	req.Memory.Bytes, req.Memory.Hard = rule.MemoryBytes, true
	return req
}

func (f *eligibilityFixture) response(id, library string) []byte {
	want := f.cat.Responses[id]
	ref := instrumentedReference{Schema: instrumentedReferenceSchema, Status: StatusCompleted, Code: "", Upstream: instrumentedReferenceCommit, CursorExhausted: true, Complete: true}
	ref.Input.Encoding, ref.Input.Bytes, ref.Input.SHA256 = EncodingUTF8, f.cat.Inputs[want.Source].Bytes, f.cat.Inputs[want.Source].SHA256
	ref.Grammar.LibrarySHA256, ref.Grammar.Symbol = library, "tree_sitter_tsgk_plain"
	ref.Tree.Nodes = f.cat.Trees[want.Tree]
	ref.Query.ID, ref.Query.SHA256, ref.Query.Patterns, ref.Query.CaptureNames, ref.Query.Predicates = want.ID, f.cat.Inputs[want.Query].SHA256, want.Patterns, want.CaptureNames, want.Predicates
	ref.Query.Structural, ref.Query.Evaluated = []Capture{}, []Capture{}
	for _, key := range want.Structural {
		c := f.cat.Captures[key]
		ref.Query.Structural = append(ref.Query.Structural, c)
		ref.Query.Matches = max(ref.Query.Matches, uint64(c.Match)+1)
	}
	for _, key := range want.Evaluated {
		ref.Query.Evaluated = append(ref.Query.Evaluated, f.cat.Captures[key])
	}
	ref.Query.MatchMapping = make([]struct {
		Match     uint32 `json:"match"`
		RuntimeID uint32 `json:"runtime_id"`
	}, ref.Query.Matches)
	for i := range ref.Query.MatchMapping {
		ref.Query.MatchMapping[i].Match = uint32(i)
		ref.Query.MatchMapping[i].RuntimeID = 17 + uint32(i)*12
	}
	ref.Limits.Nodes, ref.Limits.Captures, ref.Limits.GroupCaptures, ref.Limits.Matches, ref.Limits.WallMillis = 100000, 50000, 200000, 100000, 3000
	return append(eligibilityJSON(f.t, ref), '\n')
}

func newEligibilityFixture(t *testing.T) *eligibilityFixture {
	t.Helper()
	f := &eligibilityFixture{t: t, files: map[string][]byte{}, nodes: map[string]EvidenceNode{}, root: t.TempDir()}
	if err := jsonv2.Unmarshal([]byte(eligibilityCatalogueJSON), &f.cat); err != nil {
		t.Fatal(err)
	}
	f.reg = eligibilityRegistration{Schema: QueryReferenceEligibilityRegistrationSchema, Context: "full-input-utf8/r2", CatalogueSHA256: digestHex([]byte(eligibilityCatalogueJSON)), OwnedMembers: []eligibilityMember{}, Tools: []eligibilityTool{}, Invocations: []eligibilityInvocation{}, Observations: []eligibilityObservation{}}
	target := "x86_64-unknown-linux-gnu"
	bp := map[string]string{"B": "/build", "H": "/controller", "S": "/build/source", "T": target, "R": "/build/target/" + target + "/release/tsgk-query-reference"}
	reference := f.member("build-static", "build/reference.bin", "reference-executable", []byte("synthetic reference; never executed"))
	library := f.member("control-compile", "control/library.bin", "runtime-library", []byte("synthetic library; never loaded"))
	cp := map[string]string{"C": "/control", "G": "/control/grammar", "L": "/control/plain.so", "LH": library.Member.SHA256, "R": bp["R"]}
	hashes, tools := map[string]string{}, map[string]string{}
	for _, name := range []string{"cargo", "rustc", "cc", "ar", "git", "supervisor", "python"} {
		hashes[name] = digestHex([]byte("synthetic tool " + name))
		tools[name] = "/tools/" + name
	}
	sourceRows := map[string]eligibilitySnapshotRow{"Cargo.toml": {Bytes: 1, SHA256: digestHex([]byte("workspace"))}, ".cargo/config.toml": {Bytes: 1, SHA256: digestHex([]byte("config"))}, "Cargo.lock": {Bytes: 1, SHA256: digestHex([]byte("lock"))}, "lib/binding_rust/lib.rs": {Bytes: 1, SHA256: f.cat.BindingSHA256}, "crates/tsgk-query-reference/Cargo.toml": {Bytes: 1, SHA256: f.cat.ReferenceInputs["Cargo.toml"]}, "crates/tsgk-query-reference/main.rs": {Bytes: 1, SHA256: f.cat.ReferenceInputs["main.rs"]}}
	source := f.member("build-static", "build/source-snapshot.json", "source-snapshot", eligibilityJSON(t, sourceRows))
	vendor := f.member("build-static", "build/vendor-snapshot.json", "vendor-snapshot", eligibilityJSON(t, map[string]eligibilitySnapshotRow{"fixture/Cargo.toml": {Bytes: 1, SHA256: digestHex([]byte("vendor"))}}))
	identity := eligibilityBuildIdentity{Schema: instrumentedReferenceBuildSchema, UpstreamCommit: f.cat.UpstreamCommit, ArchiveSHA256: f.cat.ArchiveSHA256, BindingSHA256: f.cat.BindingSHA256, Target: target, BinarySHA256: reference.Member.SHA256, LockSHA256: sourceRows["Cargo.lock"].SHA256, Versions: map[string]string{"rustc": "release: 1.97.1\n", "cargo": "release: 1.97.1\n", "cc": "synthetic cc\n", "python": "synthetic Python"}, Inputs: f.cat.ReferenceInputs, Tools: hashes, ToolPaths: tools, SourceSnapshotSHA256: source.Member.SHA256, VendorSnapshotSHA256: vendor.Member.SHA256, Complete: true}
	patchBytes, err := os.ReadFile("../dev/query-reference/instrumentation.patch")
	if err != nil {
		t.Fatal(err)
	}
	patch := f.member("build-static", "build/instrumentation.patch", "patch", patchBytes)
	for _, rule := range f.cat.Builds {
		req := eligibilityFixtureSpec(rule, bp, tools, true)
		stdout := []byte{}
		switch rule.ID {
		case "rustc-version":
			stdout = []byte(identity.Versions["rustc"])
		case "cargo-version":
			stdout = []byte(identity.Versions["cargo"])
		case "cc-version":
			stdout = []byte(identity.Versions["cc"])
		case "metadata":
			stdout = []byte("{}\n")
			identity.MetadataSHA256 = digestHex(stdout)
		case "selected-tree":
			stdout = []byte("synthetic selected tree\n")
		case "unit":
			stdout = []byte("running 4 tests\n")
			for _, id := range f.cat.Units {
				stdout = append(stdout, []byte("test tests::"+id+" ... ok\n")...)
			}
			stdout = append(stdout, []byte("test result: ok. 4 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.00s\n")...)
		}
		inv := f.invocation("build-"+rule.ID, "build", rule.ID, req, stdout, 0, false)
		inv.ToolID = "build-tool-" + rule.Tool
		inv.Inputs["source"], inv.Inputs["library"] = eligibilityInput{Kind: "NOT_APPLICABLE"}, eligibilityInput{Kind: "NOT_APPLICABLE"}
		if rule.ID == "patch" || rule.ID == "patch-check" {
			k := 2
			if rule.ID == "patch-check" {
				k = 3
			}
			inv.Inputs["patch"] = eligibilityInput{Kind: "PINNED", MemberPin: &patch, ContentBasis: "PRESERVED_ORIGINAL", ArgvIndex: eligibilityIndex(k), PathLiteral: req.Args[k]}
		}
		if rule.ID == "selected-tree" {
			identity.SelectedTreeSHA256 = inv.Receipt.Member.SHA256
		}
	}
	identityPin := f.member("build-static", "build/identity.json", "build-identity", eligibilityJSON(t, identity))
	f.reg.Build = &eligibilityBuild{CohortID: "build", Paths: bp, Identity: identityPin, SourceSnapshot: source, VendorSnapshot: vendor, ReferenceBinary: &reference}
	for name := range tools {
		f.reg.Tools = append(f.reg.Tools, eligibilityTool{ID: "build-tool-" + name, CohortID: "build", ToolName: name, PathLiteral: tools[name], SubjectSHA256: hashes[name], IdentityMember: identityPin, ProvenanceID: "p-build"})
	}
	grammar := map[string]eligibilityMember{}
	for name := range f.cat.GrammarInputs {
		data, err := os.ReadFile("../testdata/native/plain/src/" + name)
		if err != nil {
			t.Fatal(err)
		}
		role := "grammar-header"
		if name == "parser.c" {
			role = "grammar-parser"
		}
		grammar[name] = f.member("control-static", "control/grammar/"+name, role, data)
	}
	inputPins := map[string]eligibilityMember{}
	for name, row := range f.cat.Inputs {
		data, err := hex.DecodeString(row.Hex)
		if err != nil {
			t.Fatal(err)
		}
		if row.RepeatByte != "" {
			data = []byte(strings.Repeat(" ", int(row.Bytes)))
		}
		inputPins[name] = f.member("control-static", "control/input/"+name, "control-input", data)
	}
	for _, rule := range f.cat.Controls {
		br := eligibilityBuildRule{ID: rule.ID, Tool: rule.Tool, Args: rule.Args, Dir: rule.Dir, WallSeconds: rule.WallSeconds, GraceSeconds: rule.GraceSeconds, MemoryBytes: rule.MemoryBytes, StdoutBytes: rule.StdoutBytes, StderrBytes: rule.StderrBytes}
		ct := map[string]string{"cc": tools["cc"], "reference": bp["R"]}
		req := eligibilityFixtureSpec(br, cp, ct, false)
		stdout := []byte{}
		for _, id := range rule.Responses {
			stdout = append(stdout, f.response(id, cp["LH"])...)
		}
		if rule.Negative && rule.ID != "batch-truncated" {
			stdout = append(stdout, eligibilityJSON(t, eligibilityFailureResponse{Schema: instrumentedReferenceSchema, Status: StatusFailed, Code: rule.FailureCode})...)
			stdout = append(stdout, '\n')
		}
		if len(rule.Args) > 0 && rule.Args[0] == "--batch" && rule.ID != "batch-truncated" {
			status := StatusCompleted
			if rule.Negative {
				status = StatusFailed
			}
			stdout = append(stdout, eligibilityJSON(t, eligibilityBatchFooter{Schema: "tsgk-instrumented-query-reference-batch/r1", Status: status, Complete: !rule.Negative, Queries: rule.QueryIDs, ResponsesCompleted: uint64(len(rule.Responses))})...)
			stdout = append(stdout, '\n')
		}
		if rule.ID == "batch-truncated" {
			stdout = []byte(strings.Repeat("x", 1024))
		}
		inv := f.invocation("control-"+rule.ID, "control", rule.ID, req, stdout, rule.ExpectedSupervisorExit, rule.ID == "batch-truncated")
		if rule.ID == "compile" {
			inv.ToolID = "control-tool-cc"
			inv.Inputs["source"], inv.Inputs["library"] = eligibilityInput{Kind: "NOT_APPLICABLE"}, eligibilityInput{Kind: "NOT_APPLICABLE"}
			parser := grammar["parser.c"]
			inv.Inputs["grammar-parser"] = eligibilityInput{Kind: "PINNED", MemberPin: &parser, ContentBasis: "PRESERVED_ORIGINAL", ArgvIndex: eligibilityIndex(5), PathLiteral: req.Args[5]}
			inv.Inputs["output-library"] = eligibilityInput{Kind: "PINNED", MemberPin: &library, ContentBasis: "PRESERVED_ORIGINAL", ArgvIndex: eligibilityIndex(7), PathLiteral: req.Args[7]}
			continue
		}
		inv.Executable, inv.ExecutablePath = &reference, bp["R"]
		batch := req.Args[0] == "--batch"
		si, li := 2, 0
		if batch {
			si, li = 3, 1
		}
		src := inputPins[strings.TrimPrefix(req.Args[si], "/control/")]
		inv.Inputs["source"] = eligibilityInput{Kind: "PINNED", MemberPin: &src, ContentBasis: "APPROVED_EXPECTED_BYTES", ArgvIndex: eligibilityIndex(si), PathLiteral: req.Args[si]}
		inv.Inputs["library"] = eligibilityInput{Kind: "PINNED", MemberPin: &library, ContentBasis: "PRESERVED_ORIGINAL", ArgvIndex: eligibilityIndex(li), PathLiteral: req.Args[li]}
		indices := []int{3}
		if batch {
			indices = nil
			for k := 6; k < len(req.Args); k += 2 {
				indices = append(indices, k)
			}
		}
		for i, k := range indices {
			name := fmt.Sprintf("query-%d", i)
			path := strings.TrimPrefix(req.Args[k], "/control/")
			if path == "missing.scm" {
				inv.Inputs[name] = eligibilityInput{Kind: "EXPECTED_ABSENT", ArgvIndex: eligibilityIndex(k), PathLiteral: req.Args[k]}
			} else {
				pin := inputPins[path]
				inv.Inputs[name] = eligibilityInput{Kind: "PINNED", MemberPin: &pin, ContentBasis: "APPROVED_EXPECTED_BYTES", ArgvIndex: eligibilityIndex(k), PathLiteral: req.Args[k]}
			}
		}
	}
	passed := []string{}
	for _, a := range f.cat.Assertions {
		passed = append(passed, a.ID)
	}
	summary := eligibilityControlSummary{Kind: "OWNED_MANUAL_CONTROLS_NOT_ALL_ROUTE_ELIGIBILITY", Passed: passed, BinarySHA256: reference.Member.SHA256, LibrarySHA256: library.Member.SHA256, GrammarInputs: f.cat.GrammarInputs, IdentitySHA256: identityPin.Member.SHA256, ControlInputs: map[string]string{"check.py": f.cat.ControllerSHA256["check.py"], "checks.py": f.cat.ControllerSHA256["checks.py"], "cc": hashes["cc"], "supervisor": hashes["supervisor"], "python": hashes["python"]}}
	summaryPin := f.member("control-static", "control/summary.json", "control-summary", eligibilityJSON(t, summary))
	f.reg.Control = &eligibilityControl{CohortID: "control", Paths: cp, Summary: summaryPin, Library: &library, Grammar: grammar}
	for _, name := range []string{"cc", "supervisor", "python"} {
		f.reg.Tools = append(f.reg.Tools, eligibilityTool{ID: "control-tool-" + name, CohortID: "control", ToolName: name, PathLiteral: tools[name], SubjectSHA256: hashes[name], IdentityMember: summaryPin, ProvenanceID: "p-control"})
	}
	f.addNative(reference, library, grammar, inputPins)
	return f
}

func (f *eligibilityFixture) addNative(reference, library eligibilityMember, grammar, inputPins map[string]eligibilityMember) {
	t := f.t
	initial := []byte("x;\n1;\n")
	later := []byte("y;\n1;\n")
	executable := f.member("native-static", "native/executable.bin", "native-executable", []byte("synthetic native executable; never executed"))
	source := f.member("native-static", "native/source.bin", "source", initial)
	queryPin := inputPins["eq.scm"]
	queryData := f.files[queryPin.Member.Path]
	var runtime struct {
		Commit string `json:"commit"`
		Files  []struct {
			Path   string `json:"path"`
			Bytes  uint64 `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	data, err := drivers.NativeC.ReadFile("native-c/runtime-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := jsonv2.Unmarshal(data, &runtime); err != nil {
		t.Fatal(err)
	}
	closure := map[string]eligibilitySnapshotRow{}
	for _, row := range runtime.Files {
		closure["runtime "+row.Path] = eligibilitySnapshotRow{Bytes: row.Bytes, SHA256: row.SHA256}
	}
	for _, name := range []string{"driver.c", "cp949_table.h"} {
		data, err := drivers.NativeC.ReadFile("native-c/" + name)
		if err != nil {
			t.Fatal(err)
		}
		closure["driver driver/"+name] = eligibilitySnapshotRow{Bytes: uint64(len(data)), SHA256: digestHex(data)}
	}
	shim := []byte("#include <tree_sitter/api.h>\nconst TSLanguage *tree_sitter_tsgk_plain(void);\nconst TSLanguage *tsgk_language(void) { return tree_sitter_tsgk_plain(); }\n")
	closure["driver driver/shim.c"] = eligibilitySnapshotRow{Bytes: uint64(len(shim)), SHA256: digestHex(shim)}
	for name, pin := range grammar {
		closure[pin.Member.Role+" "+name] = eligibilitySnapshotRow{Bytes: pin.Member.Bytes, SHA256: pin.Member.SHA256}
	}
	keys := []string{}
	for key := range closure {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	manifest := "tsgk-native-build/r1\nsymbol=tree_sitter_tsgk_plain\ncompiler=" + digestHex([]byte("synthetic cc")) + " 3\nruntime=" + runtime.Commit + "\nos=linux/amd64\n"
	for _, key := range keys {
		row := closure[key]
		manifest += fmt.Sprintf("file=%s %s %d\n", key, row.SHA256, row.Bytes)
	}
	for _, name := range []string{"version", "runtime", "parser", "driver", "shim", "link"} {
		args := []string{"approved synthetic build"}
		if name == "version" {
			args = []string{"--version"}
		}
		manifest += fmt.Sprintf("step=%s %q\n", name, args)
	}
	build := f.member("native-static", "native/build-closure.txt", "native-build", []byte(manifest))
	w := eligibilityWitness{QueryWitnessRegistration: QueryWitnessRegistration{Schema: queryEligibilityWitnessSchema, Policy: "tsgk-predicates/r1", Case: "eligibility-case", Route: "owned-plain", Platform: "linux/amd64", Role: "owned", Encoding: EncodingUTF8, Input: NativeInput{Path: "native/source.bin", Role: "source", Bytes: uint64(len(initial)), SHA256: digestHex(initial)}, Edits: []Edit{{StartByte: 0, OldEndByte: 1, NewEndByte: 1, Old: []byte("x"), New: []byte("y")}}, Queries: []QueryWitnessQuery{}, Required: []QueryWitnessTuple{}}}
	expected := f.cat.Responses["response-2"] // eq-0, independently source-reviewed fixed catalogue.
	if expected.Query != "eq.scm" {
		t.Fatal("eq fixture catalogue ID changed")
	}
	anchor := f.cat.Captures[expected.Structural[0]]
	w.Queries = []QueryWitnessQuery{{ID: "q0", Source: string(queryData), Metadata: &QueryWitnessMetadata{Patterns: 1, CaptureNames: []string{"x"}, Predicates: expected.Predicates}}}
	w.Required = []QueryWitnessTuple{{Query: "q0", Step: 0, TreeRole: "incremental", Structural: []Capture{anchor}, Expected: []Capture{anchor}}, {Query: "q0", Step: 1, TreeRole: "incremental", Structural: []Capture{anchor}, Expected: []Capture{}}, {Query: "q0", Step: 1, TreeRole: "fresh", Structural: []Capture{anchor}, Expected: []Capture{}}}
	nativeSpec := eligibilityProcessRequest{Path: "/current/native", Args: []string{}, Dir: "/current", Env: []string{}, StdoutBytes: 64 << 20, StderrBytes: 1 << 20, Wall: 50000000000, Grace: 2000000000, CgroupParent: "/delegated"}
	nativeSpec.Memory.Bytes, nativeSpec.Memory.Hard = 1<<30, true
	w.Native = eligibilityNativeExpected{Protocol: nativewire.ProtocolR4, RequestID: w.Case, NodeLimit: 100000, API: false, Points: []uint32{}, RuntimeManifestSHA256: reuseobs.RuntimeManifestSHA256, ProducerSHA256: build.Member.SHA256, ExecutableSHA256: executable.Member.SHA256, ProducerABI: eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: 25, FieldCount: 3}, ObservationRunID: "synthetic-current", Process: nativeSpec}
	f.witness = w
	full := "full"
	api, predicates := "tsgk-api/r1", "NOT_EVALUATED"
	symbols, fields := uint32(25), uint32(3)
	raw := nativewire.Response{Protocol: nativewire.ProtocolR4, ID: w.Case, Status: StatusCompleted, SourceBytes: 6, StepsCompleted: 2, Complete: true, Producer: nativewire.Producer{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, Query: "tsgk-query/r1", API: &api, Predicates: &predicates, SymbolCount: &symbols, FieldCount: &fields, ReuseObservation: eligibilityJSON(t, map[string]string{"policy": reuseobs.Policy, "runtime_manifest_sha256": reuseobs.RuntimeManifestSHA256})}, Steps: []nativewire.Step{}}
	for k, sourceBytes := range [][]byte{initial, later} {
		ns := f.cat.Trees["tree-0"]
		types := []string{}
		rows := [][]int64{}
		for _, n := range ns {
			index := slices.Index(types, n.Type)
			if index < 0 {
				index = len(types)
				types = append(types, n.Type)
			}
			flags := int64(0)
			if n.Named {
				flags = 1
			}
			rows = append(rows, []int64{n.Parent, int64(index), -1, flags, int64(n.StartByte), int64(n.EndByte), int64(n.StartPoint.Row), int64(n.StartPoint.Column), int64(n.EndPoint.Row), int64(n.EndPoint.Column)})
		}
		q := querywire.Query{ID: "q0", Status: StatusCompleted, Code: "", Patterns: 1, CaptureNames: []string{"x"}, Predicates: expected.Predicates, Matches: 1, Types: []string{"identifier"}, Captures: [][]int64{{0, 0, 0, 2, 0, 1, 0, 1, 0, 0, 0, 1}}}
		tree := nativewire.WireTree{Status: StatusCompleted, Form: &full, DescendantCount: 7, MaxDepth: 2, Digest: TreeDigest(ns), Types: types, Fields: []string{}, Nodes: rows, Queries: eligibilityJSON(t, []querywire.Query{q}), API: []byte("null")}
		observation := reuseobs.Observation{Schema: reuseobs.Schema, Policy: reuseobs.Policy, RuntimeManifestSHA256: reuseobs.RuntimeManifestSHA256, RequestID: w.Case, RunID: w.Native.ObservationRunID, ProducerSHA256: build.Member.SHA256, Step: uint64(k), Role: "initial", SourceSHA256: digestHex(sourceBytes), SourceBytes: 6, Status: "COMPLETE", Events: 2, Bytes: 30, Counts: &reuseobs.Counts{NewParse: 1, Done: 1}}
		obs := []reuseobs.Observation{observation}
		step := nativewire.Step{Step: k, SourceBytes: 6, SourceSHA256: digestHex(sourceBytes), Incremental: tree}
		if k > 0 {
			old := digestHex(initial)
			observation.Role, observation.OldTree, observation.OldSourceSHA256, observation.Counts = "incremental", true, &old, &reuseobs.Counts{ParseAfterEdit: 1, Done: 1}
			fresh := obs[0]
			fresh.Role = "fresh"
			obs = []reuseobs.Observation{observation, fresh}
			step.Fresh = &tree
			step.Edit = &nativewire.StepEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, StartPoint: [2]uint32{0, 0}, OldEndPoint: [2]uint32{0, 1}, NewEndPoint: [2]uint32{0, 1}}
		}
		step.Observations = eligibilityJSON(t, obs)
		raw.Steps = append(raw.Steps, step)
	}
	node := f.node("native-current", strings.Repeat("a", 40))
	request := f.member("native-current", "native/request.json", "approved-request", eligibilityJSON(t, w.Native))
	response := f.member("native-current", "native/response.json", "response", eligibilityJSON(t, raw))
	process := eligibilityFixtureExecution(nativeSpec, 0, false, f.files[response.Member.Path], []byte{})
	process.StdoutBytes += 4
	processPin := f.member("native-current", "native/process.json", "process", eligibilityJSON(t, process))
	inv := eligibilityInvocation{ID: "native-current", Kind: "NATIVE_DRIVER", CohortID: "current", CatalogueID: "native-current", Node: QueryReferenceNode{ID: node.ID, Identities: node.Identities}, Request: &request, Response: &response, Process: &processPin, ApprovedRequest: eligibilityJSON(t, w.Native), Executable: &executable, ExecutablePath: nativeSpec.Path, Inputs: map[string]eligibilityInput{"source": {Kind: "PINNED", MemberPin: &source, ContentBasis: "PRESERVED_ORIGINAL"}, "library": {Kind: "NOT_APPLICABLE"}, "query-q0": {Kind: "PINNED", MemberPin: &queryPin, ContentBasis: "APPROVED_EXPECTED_BYTES"}}}
	f.reg.Invocations = append(f.reg.Invocations, inv)
	f.reg.Native = &eligibilityNative{InvocationID: inv.ID, Source: source, BuildManifest: build, ReferenceLibrary: &library, Symbol: "tree_sitter_tsgk_plain", Grammar: maps.Clone(grammar)}
	for k, sourcePin := range []eligibilityMember{source, inputPins["eq-1.txt"]} {
		id := fmt.Sprintf("current-ref-%d", k)
		req := eligibilityProcessRequest{Path: "/current/reference", Args: []string{"/current/library", "tree_sitter_tsgk_plain", fmt.Sprintf("/current/source-%d", k), "/current/query", "q0", library.Member.SHA256}, Dir: "/current", Env: []string{}, StdoutBytes: 64 << 20, StderrBytes: 1 << 20, Wall: 10000000000, Grace: 2000000000, CgroupParent: "/delegated"}
		req.Memory.Bytes, req.Memory.Hard = 1<<30, true
		rid := "response-2"
		if k == 1 {
			rid = "response-3"
		}
		stdout := f.response(rid, library.Member.SHA256)
		ref := f.invocation(id, "current", id, req, stdout, 0, false)
		ref.Executable, ref.ExecutablePath = &reference, req.Path
		ref.Inputs["source"] = eligibilityInput{Kind: "PINNED", MemberPin: &sourcePin, ContentBasis: "APPROVED_EXPECTED_BYTES", ArgvIndex: eligibilityIndex(2), PathLiteral: req.Args[2]}
		ref.Inputs["library"] = eligibilityInput{Kind: "PINNED", MemberPin: &library, ContentBasis: "PRESERVED_ORIGINAL", ArgvIndex: eligibilityIndex(0), PathLiteral: req.Args[0]}
		ref.Inputs["query-0"] = eligibilityInput{Kind: "PINNED", MemberPin: &queryPin, ContentBasis: "APPROVED_EXPECTED_BYTES", ArgvIndex: eligibilityIndex(3), PathLiteral: req.Args[3]}
		roles := []string{"incremental"}
		if k > 0 {
			roles = append(roles, "fresh")
		}
		for _, role := range roles {
			f.reg.Observations = append(f.reg.Observations, eligibilityObservation{Query: "q0", Step: uint32(k), TreeRole: role, SourceBytes: 6, SourceSHA256: sourcePin.Member.SHA256, InvocationID: id, QueryID: "q0", ResponseOrdinal: 0, ByteOffset: 0, ByteLength: uint64(len(stdout)), SHA256: digestHex(stdout)})
		}
	}
}

func (f *eligibilityFixture) request() QueryReferenceEligibilityRequest {
	t := f.t
	ids := []string{}
	for id := range f.nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	nodes := []EvidenceNode{}
	required := []struct {
		Node       string            `json:"node"`
		Kind       string            `json:"kind"`
		Identities map[string]string `json:"identities"`
	}{}
	for _, id := range ids {
		n := f.nodes[id]
		nodes = append(nodes, n)
		required = append(required, struct {
			Node       string            `json:"node"`
			Kind       string            `json:"kind"`
			Identities map[string]string `json:"identities"`
		}{n.ID, n.Kind, n.Identities})
	}
	graph := eligibilityJSON(t, struct {
		Schema string         `json:"schema"`
		Nodes  []EvidenceNode `json:"nodes"`
	}{EvidenceSchema, nodes})
	f.reg.Graph = ReplayMember{Path: "evidence.json", Role: "evidence-graph", Bytes: uint64(len(graph)), SHA256: digestHex(graph)}
	policy := eligibilityJSON(t, map[string]any{"schema": EvidencePolicySchema, "id": "eligibility-synthetic", "evidence_sha256": digestHex(graph), "required": required, "carry_forward": []any{}})
	candidate := eligibilityCandidate{SourceRevision: strings.Repeat("a", 40), Platform: "linux/amd64", Run: "fixture-external-run", Attempt: 1}
	f.approval = eligibilityApproval{Schema: QueryReferenceEligibilityApprovalSchema, RegistrationSHA256: digestHex(eligibilityJSON(t, f.reg)), WitnessSHA256: digestHex(eligibilityJSON(t, f.witness)), EvidencePolicySHA256: digestHex(policy), CatalogueSHA256: f.reg.CatalogueSHA256, Candidate: candidate, Cohorts: []eligibilityCohort{}, Provenance: []eligibilityProvenance{}}
	for _, purpose := range []string{"build", "control", "current"} {
		invocations := []string{}
		for _, inv := range f.reg.Invocations {
			if inv.CohortID == purpose {
				invocations = append(invocations, inv.ID)
			}
		}
		f.approval.Cohorts = append(f.approval.Cohorts, eligibilityCohort{ID: purpose, Purpose: purpose, SourceRevision: candidate.SourceRevision, Platform: candidate.Platform, Run: candidate.Run, Attempt: 1, ProvenanceID: "p-" + purpose})
		f.approval.Provenance = append(f.approval.Provenance, eligibilityProvenance{ID: "p-" + purpose, Kind: "approved-local", SourceRevision: candidate.SourceRevision, RunnerSHA256: digestHex([]byte("synthetic runner")), ControllerSourcesSHA256: digestHex([]byte("synthetic controller")), ReviewedContextSHA256: digestHex([]byte("synthetic context")), ReviewRecordSHA256: digestHex([]byte("synthetic review")), EvidenceSHA256: digestHex(graph), InvocationIDs: invocations, SessionID: "offline-synthetic-no-runtime-authenticity"})
	}
	for path, data := range f.files {
		target := filepath.Join(f.root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(f.root, "evidence.json"), graph, 0600); err != nil {
		t.Fatal(err)
	}
	return QueryReferenceEligibilityRequest{Evidence: EvidenceRequest{Root: f.root, Policy: policy}, Approval: eligibilityJSON(t, f.approval), Registration: eligibilityJSON(t, f.reg), WitnessRegistration: eligibilityJSON(t, f.witness)}
}

func TestQueryReferenceEligibilityComplete(t *testing.T) {
	f := newEligibilityFixture(t)
	req := f.request()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got, err := VerifyQueryReferenceEligibility(ctx, req)
	if err != nil || got.Assessment != AssessPass || !got.Complete {
		t.Fatalf("eligibility err=%v assessment=%s complete=%v counts=%+v/%+v/%+v/%+v/%+v findings=%+v", err, got.Assessment, got.Complete, got.Build, got.Unit, got.Control, got.Assertion, got.Tuple, got.Findings)
	}
	if got.Build.Verified != 9 || got.Unit.Verified != 4 || got.Control.Verified != 53 || got.Assertion.Verified != 36 || got.Tuple.Verified != 3 || got.Work == 0 || got.Work > eligibilityWorkLimit || got.LogicalBytes > eligibilityLogicalLimit {
		t.Fatalf("counts/budget %+v", got)
	}
	t.Logf("synthetic normal counts=9/4/53/36/3 work=%d logical=%d read=%d", got.Work, got.LogicalBytes, got.BytesRead)
}

func TestQueryReferenceEligibilityCatalogue(t *testing.T) {
	data, err := os.ReadFile("../contracts/query-reference-eligibility-catalogue-r1.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != eligibilityCatalogueJSON {
		t.Fatal("canonical/private catalogue differ")
	}
	var c eligibilityCatalogue
	if err := jsonv2.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Builds) != 9 || len(c.Units) != 4 || len(c.Controls) != 53 || len(c.Assertions) != 36 || c.Responses["response-25"].Structural[0] != "capture-3" {
		t.Fatal("fixed denominator/order changed")
	}
}

func (f *eligibilityFixture) replace(path string, data []byte) {
	f.files[path] = data
	update := func(m ReplayMember) ReplayMember {
		if m.Path == path {
			m.Bytes = uint64(len(data))
			m.SHA256 = digestHex(data)
		}
		return m
	}
	for id, node := range f.nodes {
		for i, m := range node.Files {
			node.Files[i] = update(m)
		}
		f.nodes[id] = node
	}
	var visit func(reflect.Value)
	visit = func(v reflect.Value) {
		if v.Kind() == reflect.Pointer {
			if !v.IsNil() {
				visit(v.Elem())
			}
			return
		}
		if v.Type() == reflect.TypeFor[ReplayMember]() {
			v.Set(reflect.ValueOf(update(v.Interface().(ReplayMember))))
			return
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				visit(v.Field(i))
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				visit(v.Index(i))
			}
		case reflect.Map:
			for _, k := range v.MapKeys() {
				item := reflect.New(v.Type().Elem()).Elem()
				item.Set(v.MapIndex(k))
				visit(item)
				v.SetMapIndex(k, item)
			}
		}
	}
	visit(reflect.ValueOf(&f.reg).Elem())
}

func (f *eligibilityFixture) receiptMutation(id string, change func(*eligibilityReceipt)) {
	path := id + "/receipt.json"
	var receipt eligibilityReceipt
	if err := jsonv2.Unmarshal(f.files[path], &receipt); err != nil {
		f.t.Fatal(err)
	}
	change(&receipt)
	f.replace(path, eligibilityJSON(f.t, receipt))
}

func (f *eligibilityFixture) stdoutMutation(id string, change func([]byte) []byte) {
	f.receiptMutation(id, func(receipt *eligibilityReceipt) {
		data, err := base64.StdEncoding.DecodeString(receipt.StdoutBase64)
		if err != nil {
			f.t.Fatal(err)
		}
		data = change(data)
		receipt.StdoutBase64 = base64.StdEncoding.EncodeToString(data)
		receipt.Execution.StdoutBytes = uint64(len(data))
	})
}

func eligibilityFinding(got QueryReferenceEligibilityResult, code string) bool {
	for _, f := range got.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

func eligibilityAssertCounts(t *testing.T, got QueryReferenceEligibilityResult) {
	t.Helper()
	for _, c := range []QueryEligibilityCounts{got.Build, got.Unit, got.Control, got.Assertion, got.Tuple} {
		if c.Required != c.Verified+c.Failed+c.Blocked+c.Unprocessed {
			t.Fatalf("counter partition broken %+v", c)
		}
	}
	if got.Build.Required != 9 || got.Unit.Required != 4 || got.Control.Required != 53 || got.Assertion.Required != 36 {
		t.Fatal("catalogue denominator changed")
	}
}

func TestQueryReferenceEligibilityAdversarial(t *testing.T) {
	cases := []struct {
		name             string
		mutate           func(*eligibilityFixture)
		assessment, code string
		check            func(*testing.T, QueryReferenceEligibilityResult)
	}{
		{name: "missing-fresh-observation", mutate: func(f *eligibilityFixture) {
			f.reg.Observations = slices.DeleteFunc(f.reg.Observations, func(o eligibilityObservation) bool { return o.TreeRole == "fresh" })
		}, assessment: AssessBlocked, code: "ELIGIBILITY_REFERENCE_OBSERVATION_MISSING", check: func(t *testing.T, g QueryReferenceEligibilityResult) {
			if g.Tuple.Verified != 2 || g.Tuple.Blocked != 1 {
				t.Fatalf("unrelated tuple counts %+v", g.Tuple)
			}
		}},
		{name: "missing-input-slot", mutate: func(f *eligibilityFixture) {
			for i := range f.reg.Invocations {
				if f.reg.Invocations[i].CatalogueID == "structural-0" {
					delete(f.reg.Invocations[i].Inputs, "source")
				}
			}
		}, assessment: AssessBlocked, code: "ELIGIBILITY_INPUT_MISSING"},
		{name: "missing-binary-is-not-a-descriptor", mutate: func(f *eligibilityFixture) { f.reg.Build.ReferenceBinary = nil }, assessment: AssessBlocked, code: "ELIGIBILITY_BUILD_BINARY_MISSING"},
		{name: "missing-control-executable", mutate: func(f *eligibilityFixture) {
			for i := range f.reg.Invocations {
				inv := &f.reg.Invocations[i]
				if inv.ID == "control-structural-0" {
					inv.Executable = nil
					inv.ExecutablePath = ""
				}
			}
		}, assessment: AssessBlocked, code: "ELIGIBILITY_INCOMPLETE", check: func(t *testing.T, g QueryReferenceEligibilityResult) {
			if g.Control.Verified != 52 || g.Control.Blocked != 1 || g.Control.Failed != 0 {
				t.Fatalf("missing descriptor became contradiction %+v", g.Control)
			}
		}},
		{name: "missing-library", mutate: func(f *eligibilityFixture) { f.reg.Control.Library = nil }, assessment: AssessBlocked, code: "ELIGIBILITY_CONTROL_LIBRARY_MISSING"},
		{name: "53-cannot-be-replaced-by-36", mutate: func(f *eligibilityFixture) {
			seen := 0
			f.reg.Invocations = slices.DeleteFunc(f.reg.Invocations, func(inv eligibilityInvocation) bool {
				if inv.CohortID != "control" {
					return false
				}
				seen++
				return seen > 36
			})
		}, assessment: AssessBlocked, code: "ELIGIBILITY_INCOMPLETE", check: func(t *testing.T, g QueryReferenceEligibilityResult) {
			if g.Control.Required != 53 || g.Control.Verified != 36 || g.Control.Blocked != 17 {
				t.Fatalf("control denominator %+v", g.Control)
			}
		}},
		{name: "cross-owner", mutate: func(f *eligibilityFixture) {
			for i := range f.reg.Invocations {
				if f.reg.Invocations[i].ID == "current-ref-1" {
					f.reg.Invocations[i].Receipt.NodeID = "current-ref-0"
				}
			}
		}, assessment: AssessFail, code: "ELIGIBILITY_MEMBER_OWNER"},
		{name: "observation-offset", mutate: func(f *eligibilityFixture) { f.reg.Observations[0].ByteOffset++ }, assessment: AssessFail, code: "ELIGIBILITY_OBSERVATION_SLICE"},
		{name: "raw-canonical-base64", mutate: func(f *eligibilityFixture) {
			f.receiptMutation("control-structural-0", func(r *eligibilityReceipt) { r.StdoutBase64 += "\n" })
		}, assessment: AssessFail, code: "ELIGIBILITY_STREAM_INVALID"},
		{name: "raw-byte-count", mutate: func(f *eligibilityFixture) {
			f.receiptMutation("control-structural-0", func(r *eligibilityReceipt) { r.Execution.StdoutBytes++ })
		}, assessment: AssessFail, code: "ELIGIBILITY_STREAM_INVALID"},
		{name: "ordinary-negative-child-exit", mutate: func(f *eligibilityFixture) {
			f.receiptMutation("control-unsupported", func(r *eligibilityReceipt) { r.Execution.ExitCode = 0; r.Execution.Status = StatusCompleted })
		}, assessment: AssessFail, code: "ELIGIBILITY_EXECUTION_INCOMPLETE"},
		{name: "normal-cannot-use-truncated-exception", mutate: func(f *eligibilityFixture) {
			f.receiptMutation("control-structural-0", func(r *eligibilityReceipt) { r.Execution.StdoutTruncated = true })
		}, assessment: AssessFail, code: "ELIGIBILITY_TRUNCATED_PREFIX"},
		{name: "full-batch-parity-retains-runtime-ids", mutate: func(f *eligibilityFixture) {
			f.stdoutMutation("control-batch-sixteen", func(data []byte) []byte {
				lines := strings.Split(string(data), "\n")
				var r instrumentedReference
				if err := jsonv2.Unmarshal([]byte(lines[0]), &r); err != nil {
					f.t.Fatal(err)
				}
				r.Query.MatchMapping[0].RuntimeID += 100
				lines[0] = string(eligibilityJSON(f.t, r))
				return []byte(strings.Join(lines, "\n"))
			})
		}, assessment: AssessFail, code: "ELIGIBILITY_BATCH_FULL_PARITY"},
		{name: "batch-footer-order", mutate: func(f *eligibilityFixture) {
			f.stdoutMutation("control-batch-sixteen", func(data []byte) []byte {
				lines := strings.Split(string(data), "\n")
				var footer eligibilityBatchFooter
				if err := jsonv2.Unmarshal([]byte(lines[16]), &footer); err != nil {
					f.t.Fatal(err)
				}
				slices.Reverse(footer.Queries)
				lines[16] = string(eligibilityJSON(f.t, footer))
				return []byte(strings.Join(lines, "\n"))
			})
		}, assessment: AssessFail, code: "ELIGIBILITY_BATCH_FOOTER"},
		{name: "snapshot-case-collision", mutate: func(f *eligibilityFixture) {
			path := f.reg.Build.VendorSnapshot.Member.Path
			var rows map[string]eligibilitySnapshotRow
			if err := jsonv2.Unmarshal(f.files[path], &rows); err != nil {
				f.t.Fatal(err)
			}
			rows["Fixture/Cargo.toml"] = rows["fixture/Cargo.toml"]
			f.replace(path, eligibilityJSON(f.t, rows))
		}, assessment: AssessFail, code: "ELIGIBILITY_SNAPSHOT_INVALID"},
		{name: "unit-unregistered-fifth-name", mutate: func(f *eligibilityFixture) {
			f.stdoutMutation("build-unit", func(data []byte) []byte { return append(data, []byte("test tests::not_registered ... ok\n")...) })
		}, assessment: AssessFail, code: "ELIGIBILITY_RUST_UNIT_SET"},
		{name: "r4-downgrade", mutate: func(f *eligibilityFixture) {
			var r nativewire.Response
			if err := jsonv2.Unmarshal(f.files["native/response.json"], &r); err != nil {
				f.t.Fatal(err)
			}
			r.Protocol = nativewire.ProtocolR2
			f.replace("native/response.json", eligibilityJSON(f.t, r))
		}, assessment: AssessFail, code: "ELIGIBILITY_R4_HEADER"},
		{name: "prefix-pass-is-not-full-logger-complete", mutate: func(f *eligibilityFixture) {
			var r nativewire.Response
			if err := jsonv2.Unmarshal(f.files["native/response.json"], &r); err != nil {
				f.t.Fatal(err)
			}
			var observations []reuseobs.Observation
			if err := jsonv2.Unmarshal(r.Steps[0].Observations, &observations); err != nil {
				f.t.Fatal(err)
			}
			o := &observations[0]
			o.Status, o.Reason, o.Events = "TRUNCATED", "MESSAGE_LIMIT", 1
			o.Counts.Done = 0
			r.Steps[0].Observations = eligibilityJSON(f.t, observations)
			f.replace("native/response.json", eligibilityJSON(f.t, r))
			var process eligibilityExecution
			if err := jsonv2.Unmarshal(f.files["native/process.json"], &process); err != nil {
				f.t.Fatal(err)
			}
			process.StdoutBytes = uint64(len(f.files["native/response.json"])) + 4
			f.replace("native/process.json", eligibilityJSON(f.t, process))
		}, assessment: AssessBlocked, code: "ELIGIBILITY_LOGGER_INCOMPLETE", check: func(t *testing.T, g QueryReferenceEligibilityResult) {
			if g.Tuple.Verified != 0 || g.Tuple.Blocked != 3 {
				t.Fatalf("logger prefix promoted %+v", g.Tuple)
			}
		}},
		{name: "ineligible-cst-does-not-hide-metadata", mutate: func(f *eligibilityFixture) {
			f.stdoutMutation("current-ref-1", func(data []byte) []byte {
				var r instrumentedReference
				if err := jsonv2.Unmarshal(data, &r); err != nil {
					f.t.Fatal(err)
				}
				r.Tree.Nodes[0].Type = "ERROR"
				r.Tree.Nodes[0].IsError, r.Tree.Nodes[0].HasError = true, true
				r.Query.Patterns = 2
				return append(eligibilityJSON(f.t, r), '\n')
			})
		}, assessment: AssessFail, code: "ELIGIBILITY_REFERENCE_METADATA"},
		{name: "declared-file-missing", mutate: func(f *eligibilityFixture) { delete(f.files, "current-ref-1/receipt.json") }, assessment: AssessFail, code: "ELIGIBILITY_MEMBER_UNVERIFIED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newEligibilityFixture(t)
			tc.mutate(f)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			got, err := VerifyQueryReferenceEligibility(ctx, f.request())
			if err != nil || got.Assessment != tc.assessment || !got.Complete || !eligibilityFinding(got, tc.code) {
				t.Fatalf("err=%v assessment=%s complete=%v code=%s findings=%+v", err, got.Assessment, got.Complete, tc.code, got.Findings)
			}
			eligibilityAssertCounts(t, got)
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestQueryReferenceEligibilityMalformedAndApproval(t *testing.T) {
	f := newEligibilityFixture(t)
	req := f.request()
	for _, tc := range []struct {
		name   string
		change func(*QueryReferenceEligibilityRequest)
	}{
		{"duplicate-approval", func(r *QueryReferenceEligibilityRequest) {
			r.Approval = append([]byte(`{"schema":"duplicate",`), r.Approval[1:]...)
		}},
		{"unknown-registration-field", func(r *QueryReferenceEligibilityRequest) {
			r.Registration = append([]byte(`{"unexpected":true,`), r.Registration[1:]...)
		}},
		{"fractional-attempt", func(r *QueryReferenceEligibilityRequest) {
			r.Approval = []byte(strings.Replace(string(r.Approval), `"attempt":1`, `"attempt":1.5`, 1))
		}},
		{"union-mixed-arm", func(r *QueryReferenceEligibilityRequest) {
			g := f.reg
			g.Invocations = slices.Clone(g.Invocations)
			g.Invocations[0].Inputs = map[string]eligibilityInput{"source": {Kind: "EXPECTED_ABSENT", ArgvIndex: eligibilityIndex(0), PathLiteral: "/missing", MemberPin: &g.Build.Identity}}
			r.Registration = eligibilityJSON(t, g)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := req
			tc.change(&copy)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			got, err := VerifyQueryReferenceEligibility(ctx, copy)
			if err == nil || got.Assessment == AssessPass || got.Complete {
				t.Fatalf("malformed accepted: %v %+v", err, got)
			}
			eligibilityAssertCounts(t, got)
		})
	}
	t.Run("no-self-approval", func(t *testing.T) {
		copy := req
		copy.Approval = nil
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		got, err := VerifyQueryReferenceEligibility(ctx, copy)
		if err != nil || got.Assessment != AssessBlocked || got.Complete || !eligibilityFinding(got, "ELIGIBILITY_APPROVAL_MISSING") {
			t.Fatalf("self approval %v %+v", err, got)
		}
	})
	t.Run("wrong-approved-registration", func(t *testing.T) {
		copy := req
		approval := f.approval
		approval.RegistrationSHA256 = strings.Repeat("0", 64)
		copy.Approval = eligibilityJSON(t, approval)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		got, err := VerifyQueryReferenceEligibility(ctx, copy)
		if err != nil || got.Assessment != AssessFail || !eligibilityFinding(got, "ELIGIBILITY_APPROVAL_MISMATCH") {
			t.Fatalf("approval pin %v %+v", err, got)
		}
	})
}

func TestQueryReferenceEligibilityInterruption(t *testing.T) {
	for _, kind := range []string{"cancel", "wall", "both", "fail-cancel", "fail-wall", "fail-budget"} {
		t.Run(kind, func(t *testing.T) {
			f := newEligibilityFixture(t)
			req := f.request()
			prior := strings.HasPrefix(kind, "fail-")
			if prior {
				approval := f.approval
				approval.RegistrationSHA256 = strings.Repeat("0", 64)
				req.Approval = eligibilityJSON(t, approval)
			}
			caller, cancel := context.WithCancel(ctxT(t))
			defer cancel()
			wall, expire := context.WithCancel(context.Background())
			defer expire()
			testHookWall = func() (context.Context, context.CancelFunc) { return wall, func() {} }
			testHookOpen = func(path string) {
				if path == "evidence.json" {
					if strings.Contains(kind, "cancel") || kind == "both" {
						cancel()
					}
					if strings.Contains(kind, "wall") || kind == "both" {
						expire()
					}
				}
			}
			if kind == "fail-budget" {
				testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.TotalBytes = 1; return l }
			}
			defer func() { testHookWall = nil; testHookOpen = nil; testHookReplayLimits = nil }()
			got, err := VerifyQueryReferenceEligibility(caller, req)
			code := "CANCELLED"
			if strings.Contains(kind, "wall") {
				code = "WALL_LIMIT"
			}
			if kind == "fail-budget" {
				code = "TOTAL_BYTES_LIMIT"
			}
			if err == nil || asError(err).Code != code || got.Complete || got.Assessment == AssessPass || prior && got.Assessment != AssessFail || !prior && got.Assessment == AssessFail {
				t.Fatalf("interruption %s %v %+v", kind, err, got)
			}
			eligibilityAssertCounts(t, got)
		})
	}
}

func TestQueryReferenceEligibilityFreshSnapshot(t *testing.T) {
	f := newEligibilityFixture(t)
	req := f.request()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	first, err := VerifyQueryReferenceEligibility(ctx, req)
	if err != nil || first.Assessment != AssessPass {
		t.Fatalf("initial %v %+v", err, first)
	}
	path := filepath.Join(f.root, "current-ref-1/receipt.json")
	if err := os.WriteFile(path, []byte("swapped after first call"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := VerifyQueryReferenceEligibility(ctx, req)
	if err != nil || second.Assessment != AssessFail || second.Tuple.Verified != 1 || !eligibilityFinding(second, "ELIGIBILITY_MEMBER_UNVERIFIED") {
		t.Fatalf("stale snapshot reused %v %+v", err, second)
	}
	eligibilityAssertCounts(t, second)
}

func TestQueryReferenceEligibilityBudgetReservations(t *testing.T) {
	for _, kind := range []string{"graph", "unit", "native-static"} {
		t.Run(kind, func(t *testing.T) {
			r, e := startRun(ctxT(t), 30*time.Second)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			b := &eligibilityBudget{r: r}
			r.eligibility = b
			switch kind {
			case "graph":
				e = b.reserveGraph(make([]EvidenceNode, 1000), evidencePolicy{})
			case "unit":
				s := &eligibilitySession{r: r, b: b}
				_, e = s.unitSet([]byte(strings.Repeat("\n", 8000000)))
			case "native-static":
				f := newEligibilityFixture(t)
				pin := f.reg.Native.BuildManifest
				s := &eligibilitySession{r: r, b: b, res: &QueryReferenceEligibilityResult{}, witness: f.witness, owned: map[string]eligibilityMember{pin.Member.Path: pin}, owners: map[string]string{pin.Member.Path: pin.NodeID}, snapshot: &evidenceSnapshot{files: f.files, verifiedMembers: map[string]ReplayMember{pin.Member.Path: pin.Member}}}
				b.logical = eligibilityLogicalLimit - 40*pin.Member.Bytes - uint64(len(pin.Member.Path)+len(pin.Member.Role)+len(pin.Member.SHA256))
				_, e = s.nativeBuild(f.reg.Native, f.witness.Native)
				if s.failures != 0 || len(s.res.Findings) != 0 {
					t.Fatalf("resource turned into contradiction %+v", s.res)
				}
			}
			if e == nil || e.Kind != KindResourceLimit || b.work > eligibilityWorkLimit || b.logical > eligibilityLogicalLimit {
				t.Fatalf("reservation %v work%d logical%d", e, b.work, b.logical)
			}
		})
	}
}

func TestQueryReferenceEligibilityInvalidParent(t *testing.T) {
	for _, parent := range []int64{-2, 100000} {
		t.Run(fmt.Sprint(parent), func(t *testing.T) {
			f := newEligibilityFixture(t)
			f.stdoutMutation("current-ref-1", func(data []byte) []byte {
				var ref instrumentedReference
				if e := jsonv2.Unmarshal(data, &ref); e != nil {
					t.Fatal(e)
				}
				ref.Tree.Nodes[1].Parent = parent
				return append(eligibilityJSON(t, ref), '\n')
			})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			got, err := VerifyQueryReferenceEligibility(ctx, f.request())
			if err != nil || got.Assessment != AssessFail {
				t.Fatalf("invalid parent accepted %v %+v", err, got)
			}
		})
	}
}

func (f *eligibilityFixture) nativeManifestMutation(change func(string) string) {
	path := f.reg.Native.BuildManifest.Member.Path
	old := f.reg.Native.BuildManifest.Member.SHA256
	f.replace(path, []byte(change(string(f.files[path]))))
	next := f.reg.Native.BuildManifest.Member.SHA256
	f.witness.Native.ProducerSHA256 = next
	f.replace("native/response.json", []byte(strings.ReplaceAll(string(f.files["native/response.json"]), old, next)))
	f.replace("native/request.json", eligibilityJSON(f.t, f.witness.Native))
	for i := range f.reg.Invocations {
		if f.reg.Invocations[i].ID == "native-current" {
			f.reg.Invocations[i].ApprovedRequest = eligibilityJSON(f.t, f.witness.Native)
		}
	}
}

func TestQueryReferenceEligibilityReview33(t *testing.T) {
	cases := []struct {
		name                       string
		change                     func(*eligibilityFixture)
		incomplete                 bool
		buildFailed, controlFailed uint64
	}{
		{name: "empty-step", change: func(f *eligibilityFixture) {
			f.nativeManifestMutation(func(s string) string { at := strings.Index(s, "step="); return s[:at] + "step=\n" })
		}},
		{name: "missing-link", change: func(f *eligibilityFixture) {
			f.nativeManifestMutation(func(s string) string {
				lines := strings.Split(s, "\n")
				lines = slices.DeleteFunc(lines, func(l string) bool { return strings.HasPrefix(l, "step=link ") })
				return strings.Join(lines, "\n")
			})
		}},
		{name: "duplicate-link", change: func(f *eligibilityFixture) {
			f.nativeManifestMutation(func(s string) string { return s + "step=link [\"duplicate\"]\n" })
		}},
		{name: "unknown-step", change: func(f *eligibilityFixture) {
			f.nativeManifestMutation(func(s string) string { return strings.Replace(s, "step=link ", "step=unknown ", 1) })
		}},
		{name: "header-only", change: func(f *eligibilityFixture) {
			f.nativeManifestMutation(func(s string) string {
				lines := strings.Split(s, "\n")
				lines = slices.DeleteFunc(lines, func(l string) bool { return strings.HasPrefix(l, "file=grammar-parser ") })
				return strings.Join(lines, "\n")
			})
			delete(f.reg.Native.Grammar, "parser.c")
		}},
		{name: "control-path", controlFailed: 1, change: func(f *eligibilityFixture) {
			for i := range f.reg.Invocations {
				if f.reg.Invocations[i].ID == "control-structural-0" {
					f.reg.Invocations[i].ExecutablePath = "/different/reference"
				}
			}
		}},
		{name: "build-fail-before-malformed-receipt", incomplete: true, buildFailed: 1, change: func(f *eligibilityFixture) {
			for i := range f.reg.Invocations {
				inv := &f.reg.Invocations[i]
				if inv.ID != "build-patch-check" {
					continue
				}
				var spec eligibilityProcessRequest
				if e := jsonv2.Unmarshal(inv.ApprovedRequest, &spec); e != nil {
					f.t.Fatal(e)
				}
				spec.Dir = "/different"
				inv.ApprovedRequest = eligibilityJSON(f.t, spec)
				f.replace(inv.Request.Member.Path, inv.ApprovedRequest)
				f.replace(inv.Receipt.Member.Path, []byte("{"))
			}
		}},
		{name: "control-fail-before-malformed-receipt", incomplete: true, controlFailed: 1, change: func(f *eligibilityFixture) {
			for i := range f.reg.Invocations {
				inv := &f.reg.Invocations[i]
				if inv.ID != "control-structural-0" {
					continue
				}
				inv.ExecutablePath = "/different/reference"
				f.replace(inv.Receipt.Member.Path, []byte("{"))
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newEligibilityFixture(t)
			tc.change(f)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			got, err := VerifyQueryReferenceEligibility(ctx, f.request())
			if (err != nil) != tc.incomplete || got.Complete == tc.incomplete || got.Assessment != AssessFail || got.Build.Failed < tc.buildFailed || got.Control.Failed < tc.controlFailed {
				t.Fatalf("review33 err%v complete%v assessment%s build%+v control%+v findings%+v", err, got.Complete, got.Assessment, got.Build, got.Control, got.Findings)
			}
			if tc.incomplete && tc.buildFailed == 1 && (got.Build.Failed != 1 || got.Build.Unprocessed != 8 || got.Build.Verified != 0) {
				t.Fatalf("build failure counter lost %+v", got.Build)
			}
			if tc.incomplete && tc.controlFailed == 1 && (got.Build.Verified != 9 || got.Control.Failed != 1 || got.Control.Verified != 1 || got.Control.Unprocessed != 51) {
				t.Fatalf("previous completion/failure lost build%+v control%+v", got.Build, got.Control)
			}
			eligibilityAssertCounts(t, got)
		})
	}
}

func TestQueryReferenceEligibilityManifestAndABI(t *testing.T) {
	for _, kind := range []string{"space-path", "missing-parser-step", "unknown-grammar-role", "runtime-abi"} {
		t.Run(kind, func(t *testing.T) {
			f := newEligibilityFixture(t)
			want := AssessFail
			switch kind {
			case "space-path":
				want = AssessPass
				pin := f.reg.Native.Grammar["parser.c"]
				delete(f.reg.Native.Grammar, "parser.c")
				f.reg.Native.Grammar["dir with space/parser.c"] = pin
				f.nativeManifestMutation(func(s string) string {
					return strings.Replace(s, "file=grammar-parser parser.c ", "file=grammar-parser dir with space/parser.c ", 1)
				})
			case "missing-parser-step":
				f.nativeManifestMutation(func(s string) string {
					lines := strings.Split(s, "\n")
					lines = slices.DeleteFunc(lines, func(l string) bool { return strings.HasPrefix(l, "step=parser ") })
					return strings.Join(lines, "\n")
				})
			case "unknown-grammar-role":
				f.nativeManifestMutation(func(s string) string { return strings.Replace(s, "file=grammar-header ", "file=grammar-unknown ", 1) })
			case "runtime-abi":
				f.witness.Native.ProducerABI.RuntimeLanguageVersion = 30
				f.replace("native/request.json", eligibilityJSON(t, f.witness.Native))
				for i := range f.reg.Invocations {
					if f.reg.Invocations[i].ID == "native-current" {
						f.reg.Invocations[i].ApprovedRequest = eligibilityJSON(t, f.witness.Native)
					}
				}
				var raw nativewire.Response
				if e := jsonv2.Unmarshal(f.files["native/response.json"], &raw); e != nil {
					t.Fatal(e)
				}
				raw.Producer.RuntimeLanguageVersion = 30
				f.replace("native/response.json", eligibilityJSON(t, raw))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			got, err := VerifyQueryReferenceEligibility(ctx, f.request())
			if err != nil || got.Assessment != want || !got.Complete {
				t.Fatalf("manifest/ABI %v %+v", err, got)
			}
			eligibilityAssertCounts(t, got)
		})
	}
}
