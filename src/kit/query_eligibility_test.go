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
		}, assessment: AssessFail, code: "ELIGIBILITY_BATCH_FULL_PARITY", check: func(t *testing.T, g QueryReferenceEligibilityResult) {
			if g.Control.Failed != 1 || g.Control.Verified != 52 || g.Assertion.Failed != 1 {
				t.Fatalf("unrelated control/assertion contaminated control%+v assertion%+v", g.Control, g.Assertion)
			}
		}},
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

func TestQueryReferenceEligibilityReview34(t *testing.T) {
	for _, kind := range []string{"current-missing-executable", "current-wrong-executable-path"} {
		t.Run(kind, func(t *testing.T) {
			f := newEligibilityFixture(t)
			for i := range f.reg.Invocations {
				inv := &f.reg.Invocations[i]
				if inv.ID != "current-ref-1" {
					continue
				}
				if kind == "current-missing-executable" {
					inv.Executable, inv.ExecutablePath = nil, ""
				} else {
					inv.ExecutablePath = "/different/reference"
				}
			}
			got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
			want := AssessFail
			if kind == "current-missing-executable" {
				want = AssessBlocked
			}
			if err != nil || !got.Complete || got.Assessment != want {
				t.Fatalf("current descriptor %v assessment%s", err, got.Assessment)
			}
			if want == AssessBlocked && (got.Tuple.Verified != 1 || got.Tuple.Blocked != 2 || got.Tuple.Failed != 0) {
				t.Fatalf("missing descriptor %+v", got.Tuple)
			}
		})
	}
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact-strings-partial-%v", partial), func(t *testing.T) {
			r, e := startRun(ctxT(t), 30*time.Second)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			b := &eligibilityBudget{r: r}
			r.eligibility = b
			s := &eligibilitySession{r: r, b: b}
			tree := nativewire.WireTree{Queries: []byte(`[]`), Types: []string{strings.Repeat("x", 1<<20)}, Fields: []string{strings.Repeat("y", 1<<20)}}
			rows := make([][]int64, 10000)
			for i := range rows {
				rows[i] = []int64{0, 0, 0, 1, 0, 1, 0, 0, 0, 1}
			}
			rows[0][0], rows[0][2] = -1, -1
			if partial {
				tree.PartialTrees = []nativewire.WirePartial{{Nodes: rows}}
			} else {
				tree.Nodes = rows
			}
			e = s.nativeReservation(nativewire.Response{Steps: []nativewire.Step{{Incremental: tree}}}, nil)
			if e == nil || e.Kind != KindResourceLimit {
				t.Fatalf("compact string amplification unreserved: %v", e)
			}
		})
	}
	t.Run("batch-rows", func(t *testing.T) {
		f := newEligibilityFixture(t)
		r, e := startRun(ctxT(t), 30*time.Second)
		if e != nil {
			t.Fatal(e)
		}
		defer r.cancel()
		b := &eligibilityBudget{r: r}
		r.eligibility = b
		s := &eligibilitySession{r: r, b: b, res: &QueryReferenceEligibilityResult{}}
		rule := f.cat.Controls[slices.IndexFunc(f.cat.Controls, func(r eligibilityControlRule) bool { return r.ID == "batch-sixteen" })]
		for range 4 {
			out := &eligibilityOutput{stdout: []byte(strings.Repeat("\n", 120000))}
			good, e := s.controlResponses(rule, out, nil)
			if good || e != nil || len(out.rows) > 18 {
				t.Fatalf("batch split unbounded: good%v err%v rows%d", good, e, len(out.rows))
			}
		}
	})
}

func TestQueryReferenceEligibilityMemberInterruptions(t *testing.T) {
	for _, where := range []string{"tool", "input", "replay"} {
		for _, kind := range []string{"cancel", "wall", "work", "values"} {
			if kind == "values" && where != "replay" {
				continue
			}
			for _, prior := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s-%s-prior-%v", where, kind, prior), func(t *testing.T) {
					caller, cancel := context.WithCancel(ctxT(t))
					defer cancel()
					r, e := startRun(caller, 30*time.Second)
					if e != nil {
						t.Fatal(e)
					}
					defer r.cancel()
					wall, expire := context.WithCancel(context.Background())
					defer expire()
					r.wall = wall
					b := &eligibilityBudget{r: r}
					r.eligibility = b
					s := &eligibilitySession{r: r, b: b, res: &QueryReferenceEligibilityResult{}, tools: map[string]eligibilityTool{}}
					x := &replayEnv{r: r}
					if prior {
						s.finding("PRIOR", "known", true)
						x.finding("PRIOR", "known", "known")
					}
					f := newEligibilityFixture(t)
					tool := f.reg.Tools[0]
					s.tools[tool.ID] = tool
					switch kind {
					case "cancel":
						cancel()
					case "wall":
						expire()
					case "work":
						b.work = eligibilityWorkLimit
					}
					want := "ELIGIBILITY_WORK_LIMIT"
					if kind == "cancel" {
						want = "CANCELLED"
					}
					if kind == "wall" {
						want = "WALL_LIMIT"
					}
					if kind == "values" {
						want = "JSON_VALUE_LIMIT"
					}
					switch where {
					case "tool":
						_, e = s.tool(tool.ID, tool.CohortID, tool.ToolName, tool.IdentityMember, map[string]string{tool.ToolName: tool.SubjectSHA256}, map[string]string{tool.ToolName: tool.PathLiteral})
					case "input":
						inv := &eligibilityInvocation{ID: "test", Inputs: map[string]eligibilityInput{"source": {Kind: "PINNED", MemberPin: &tool.IdentityMember, ArgvIndex: eligibilityIndex(0), PathLiteral: "source"}}}
						_, e = s.input(inv, "source", "PINNED", eligibilityIndex(0), "source", "", nil)
					case "replay":
						data := []byte(`{}`)
						if kind == "values" {
							data = []byte(`{"padding":[` + strings.Repeat("0,", 2000000) + `0]}`)
						}
						node := &EvidenceNode{Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
						e = x.checkReplayFile(node, &EvidenceNode{}, map[string][]byte{"result.json": data}, x.finding)
					}
					n := 0
					if prior {
						n = 1
					}
					if e == nil || e.Code != want || int(s.failures) != n || len(s.res.Findings) != n || len(x.findings) != n {
						t.Fatalf("interruption %v failures%d/%d/%d", e, s.failures, len(s.res.Findings), len(x.findings))
					}
				})
			}
		}
	}
}

func TestQueryReferenceEligibilityReview35(t *testing.T) {
	for _, kind := range []string{"source", "library", "source-metadata", "source-present-mismatch"} {
		t.Run("current-"+kind, func(t *testing.T) {
			f := newEligibilityFixture(t)
			for i := range f.reg.Invocations {
				inv := &f.reg.Invocations[i]
				if inv.ID != "current-ref-1" {
					continue
				}
				if kind == "source-present-mismatch" {
					x := inv.Inputs["source"]
					x.MemberPin = inv.Inputs["query-0"].MemberPin
					inv.Inputs["source"] = x
				} else {
					delete(inv.Inputs, strings.Split(kind, "-")[0])
				}
			}
			if kind == "source-metadata" {
				f.stdoutMutation("current-ref-1", func(data []byte) []byte {
					var ref instrumentedReference
					if e := jsonv2.Unmarshal(data, &ref); e != nil {
						t.Fatal(e)
					}
					ref.Query.Patterns = 2
					return append(eligibilityJSON(t, ref), '\n')
				})
			}
			got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
			want := AssessBlocked
			if strings.Contains(kind, "-") {
				want = AssessFail
			}
			if err != nil || !got.Complete || got.Assessment != want {
				t.Fatalf("missing/mismatched %v assessment%s", err, got.Assessment)
			}
			if want == AssessBlocked && (got.Tuple.Verified != 1 || got.Tuple.Blocked != 2 || got.Tuple.Failed != 0) {
				t.Fatalf("missing counter %+v", got.Tuple)
			}
		})
	}
	for _, id := range []string{"control-structural-0", "current-ref-1"} {
		for _, bad := range []string{"truncated", "duplicate", "unknown", "type"} {
			for _, prior := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s-%s-prior-%v", id, bad, prior), func(t *testing.T) {
					f := newEligibilityFixture(t)
					f.stdoutMutation(id, func(data []byte) []byte {
						switch bad {
						case "truncated":
							return []byte("{")
						case "duplicate":
							return append([]byte(`{"schema":"duplicate",`), data[1:]...)
						case "unknown":
							return append([]byte(`{"unknown":true,`), data[1:]...)
						default:
							return []byte(strings.Replace(string(data), `"complete":true`, `"complete":"wrong"`, 1))
						}
					})
					req := f.request()
					if prior {
						a := f.approval
						a.RegistrationSHA256 = strings.Repeat("0", 64)
						req.Approval = eligibilityJSON(t, a)
					}
					got, err := VerifyQueryReferenceEligibility(ctxT(t), req)
					if err == nil || asError(err).Kind != KindInvalidInput || got.Complete || prior && got.Assessment != AssessFail || !prior && got.Assessment == AssessFail {
						t.Fatalf("malformed err%v complete%v assessment%s", err, got.Complete, got.Assessment)
					}
					eligibilityAssertCounts(t, got)
				})
			}
		}
	}
	for _, id := range []string{"build-patch-check", "control-structural-0"} {
		for _, malformed := range []bool{false, true} {
			t.Run(fmt.Sprintf("binding-%s-malformed-%v", id, malformed), func(t *testing.T) {
				f := newEligibilityFixture(t)
				for i := range f.reg.Invocations {
					inv := &f.reg.Invocations[i]
					if inv.ID != id {
						continue
					}
					inv.Node.Identities = maps.Clone(inv.Node.Identities)
					inv.Node.Identities["tool"] = "different-valid-tool"
					if malformed {
						f.replace(inv.Receipt.Member.Path, []byte("{"))
					}
				}
				got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
				counts := got.Control
				required := uint64(53)
				verified := uint64(52)
				if strings.HasPrefix(id, "build-") {
					counts = got.Build
					required = 9
					verified = 8
				}
				if (err != nil) != malformed || got.Complete == malformed || got.Assessment != AssessFail || counts.Failed != 1 {
					t.Fatalf("binding %v complete%v assessment%s counter%+v", err, got.Complete, got.Assessment, counts)
				}
				if !malformed && counts.Verified != verified {
					t.Fatalf("completion %+v", counts)
				}
				if malformed {
					v := uint64(0)
					if required == 53 {
						v = 1
					}
					if counts.Verified != v || counts.Unprocessed != required-v-1 {
						t.Fatalf("interruption %+v", counts)
					}
				}
				eligibilityAssertCounts(t, got)
			})
		}
	}
}

func TestQueryReferenceEligibilityBindingInterruption(t *testing.T) {
	for _, id := range []string{"build-patch-check", "control-structural-0"} {
		for _, kind := range []string{"cancel", "wall", "work"} {
			t.Run(id+"-"+kind, func(t *testing.T) {
				f := newEligibilityFixture(t)
				f.request()
				caller, cancel := context.WithCancel(ctxT(t))
				defer cancel()
				r, e := startRun(caller, 30*time.Second)
				if e != nil {
					t.Fatal(e)
				}
				defer r.cancel()
				wall, expire := context.WithCancel(context.Background())
				defer expire()
				r.wall = wall
				b := &eligibilityBudget{r: r}
				r.eligibility = b
				s := &eligibilitySession{r: r, b: b, res: &QueryReferenceEligibilityResult{}, reg: f.reg, witness: f.witness, catalogue: f.cat, cohorts: map[string]eligibilityCohort{}, provenance: map[string]eligibilityProvenance{}, owned: map[string]eligibilityMember{}, owners: map[string]string{}, nodes: f.nodes, ready: map[string]bool{}, build: map[string]string{}, control: map[string]string{}, units: map[string]string{}, assertions: map[string]string{}, tuples: map[int]string{}, failedInvocations: map[string]bool{}, snapshot: &evidenceSnapshot{files: f.files, verifiedMembers: map[string]ReplayMember{}, invalidNodes: map[string]bool{}}}
				for _, c := range f.approval.Cohorts {
					s.cohorts[c.ID] = c
				}
				for _, p := range f.approval.Provenance {
					s.provenance[p.ID] = p
				}
				for _, p := range f.reg.OwnedMembers {
					s.owned[p.Member.Path] = p
					s.owners[p.Member.Path] = p.NodeID
					s.snapshot.verifiedMembers[p.Member.Path] = p.Member
				}
				for _, inv := range f.reg.Invocations {
					if inv.ID != id {
						continue
					}
					inv.Node.Identities = maps.Clone(inv.Node.Identities)
					inv.Node.Identities["tool"] = "different-valid-tool"
					if e := s.bindInvocation(inv); e != nil {
						t.Fatal(e)
					}
				}
				switch kind {
				case "cancel":
					cancel()
				case "wall":
					expire()
				case "work":
					b.work = eligibilityWorkLimit
				}
				if strings.HasPrefix(id, "build-") {
					e = s.verifyBuild()
				} else {
					e = s.verifyControls()
				}
				if e == nil || s.failures != 1 {
					t.Fatalf("lost binding failure err%v count%d", e, s.failures)
				}
				s.counts(false)
				counts := s.res.Control
				if strings.HasPrefix(id, "build-") {
					counts = s.res.Build
				}
				if counts.Failed != 1 || counts.Unprocessed != counts.Required-1 {
					t.Fatalf("counter %+v", counts)
				}
			})
		}
	}
}

func TestQueryReferenceEligibilityTupleBindingFailure(t *testing.T) {
	for _, id := range []string{"current-ref-1", "native-current"} {
		for _, malformed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-malformed-%v", id, malformed), func(t *testing.T) {
				f := newEligibilityFixture(t)
				for i := range f.reg.Invocations {
					inv := &f.reg.Invocations[i]
					if inv.ID != id {
						continue
					}
					inv.Node.Identities = maps.Clone(inv.Node.Identities)
					inv.Node.Identities["tool"] = "different-valid-tool"
					if malformed {
						p := inv.Response
						if inv.Kind == "REFERENCE_PROCESS" {
							p = inv.Receipt
						}
						f.replace(p.Member.Path, []byte("{"))
					}
				}
				got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
				n := uint64(2)
				if id == "native-current" {
					n = 3
				}
				if (err != nil) != malformed || got.Complete == malformed || got.Assessment != AssessFail || got.Tuple.Failed != n {
					t.Fatalf("tuple binding %v assessment%s complete%v counts%+v", err, got.Assessment, got.Complete, got.Tuple)
				}
				if !malformed && got.Tuple.Verified != 3-n {
					t.Fatalf("tuple completion %+v", got.Tuple)
				}
				if malformed && (got.Tuple.Verified != 0 || got.Tuple.Unprocessed != 3-n) {
					t.Fatalf("tuple interruption %+v", got.Tuple)
				}
				eligibilityAssertCounts(t, got)
			})
		}
	}
}

func TestQueryReferenceEligibilityReview36(t *testing.T) {
	for mask := 1; mask < 8; mask++ {
		t.Run(fmt.Sprintf("missing-groups-%d", mask), func(t *testing.T) {
			f := newEligibilityFixture(t)
			if mask&1 != 0 {
				f.reg.Build = nil
			}
			if mask&2 != 0 {
				f.reg.Control = nil
			}
			if mask&4 != 0 {
				f.reg.Native = nil
			}
			got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
			if err != nil || !got.Complete || got.Assessment != AssessBlocked || got.Build.Failed+got.Unit.Failed+got.Control.Failed+got.Assertion.Failed+got.Tuple.Failed != 0 {
				t.Fatalf("absent groups err%v complete%v assessment%s counts%+v/%+v/%+v", err, got.Complete, got.Assessment, got.Build, got.Control, got.Tuple)
			}
			eligibilityAssertCounts(t, got)
		})
	}
	for _, id := range []string{"current-ref-0", "current-ref-1"} {
		t.Run("missing-observation-"+id, func(t *testing.T) {
			f := newEligibilityFixture(t)
			f.reg.Observations = slices.DeleteFunc(f.reg.Observations, func(o eligibilityObservation) bool { return o.InvocationID == id })
			got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
			wantBlocked := uint64(1)
			if id == "current-ref-1" {
				wantBlocked = 2
			}
			if err != nil || !got.Complete || got.Assessment != AssessBlocked || got.Tuple.Failed != 0 || got.Tuple.Blocked != wantBlocked || got.Tuple.Verified != 3-wantBlocked {
				t.Fatalf("missing observation err%v assessment%s counts%+v", err, got.Assessment, got.Tuple)
			}
		})
	}
	for _, part := range []string{"queries", "api", "producer-observation", "observation"} {
		for _, bad := range []string{"unknown", "type", "null", "missing"} {
			for _, prior := range []bool{false, true} {
				t.Run(fmt.Sprintf("nested-%s-%s-prior-%v", part, bad, prior), func(t *testing.T) {
					f := newEligibilityFixture(t)
					var raw nativewire.Response
					if err := jsonv2.Unmarshal(f.files["native/response.json"], &raw); err != nil {
						t.Fatal(err)
					}
					var target map[string]any
					switch part {
					case "queries":
						var qs []map[string]any
						if err := jsonv2.Unmarshal(raw.Steps[0].Incremental.Queries, &qs); err != nil {
							t.Fatal(err)
						}
						target = qs[0]
					case "api":
						target = map[string]any{"revision": "tsgk-api/r1", "nodes": [][]int64{}, "child_fields": [][]int64{}, "field_lookups": [][]int64{}, "points": [][]int64{}, "names": []string{}}
					case "producer-observation":
						if err := jsonv2.Unmarshal(raw.Producer.ReuseObservation, &target); err != nil {
							t.Fatal(err)
						}
					default:
						var os []map[string]any
						if err := jsonv2.Unmarshal(raw.Steps[0].Observations, &os); err != nil {
							t.Fatal(err)
						}
						target = os[0]
					}
					field := "status"
					if part == "api" {
						field = "revision"
					}
					if part == "producer-observation" {
						field = "policy"
					}
					switch bad {
					case "unknown":
						target["unknown"] = true
					case "type":
						target[field] = 42
					case "null":
						target[field] = nil
					case "missing":
						delete(target, field)
					}
					switch part {
					case "queries":
						raw.Steps[0].Incremental.Queries = eligibilityJSON(t, []map[string]any{target})
					case "api":
						raw.Steps[0].Incremental.API = eligibilityJSON(t, target)
					case "producer-observation":
						raw.Producer.ReuseObservation = eligibilityJSON(t, target)
					default:
						raw.Steps[0].Observations = eligibilityJSON(t, []map[string]any{target})
					}
					data := eligibilityJSON(t, raw)
					f.replace("native/response.json", data)
					var process eligibilityExecution
					if err := jsonv2.Unmarshal(f.files["native/process.json"], &process); err != nil {
						t.Fatal(err)
					}
					process.StdoutBytes = uint64(len(data)) + 4
					f.replace("native/process.json", eligibilityJSON(t, process))
					req := f.request()
					if prior {
						a := f.approval
						a.RegistrationSHA256 = strings.Repeat("0", 64)
						req.Approval = eligibilityJSON(t, a)
					}
					got, err := VerifyQueryReferenceEligibility(ctxT(t), req)
					if err == nil || asError(err).Kind != KindInvalidInput || got.Complete || prior && got.Assessment != AssessFail || !prior && got.Assessment == AssessFail {
						t.Fatalf("nested err%v complete%v assessment%s", err, got.Complete, got.Assessment)
					}
					eligibilityAssertCounts(t, got)
				})
			}
		}
	}
	t.Run("capture-type-amplification", func(t *testing.T) {
		r, e := startRun(ctxT(t), 30*time.Second)
		if e != nil {
			t.Fatal(e)
		}
		defer r.cancel()
		b := &eligibilityBudget{r: r}
		r.eligibility = b
		s := &eligibilitySession{r: r, b: b}
		name := strings.Repeat("x", 64<<10)
		rows := make([][]int64, 20000)
		for i := range rows {
			rows[i] = []int64{0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 1}
		}
		q := querywire.Query{ID: "q", Status: StatusCompleted, Patterns: 1, Matches: 1, CaptureNames: []string{"x"}, Predicates: []QueryPredicate{}, Types: []string{name}, Captures: rows}
		tree := nativewire.WireTree{Types: []string{name}, Nodes: [][]int64{{-1, 0, -1, 1, 0, 1, 0, 0, 0, 1}}, Queries: eligibilityJSON(t, []querywire.Query{q}), API: []byte("null")}
		e = s.nativeReservation(nativewire.Response{Steps: []nativewire.Step{{Incremental: tree}}}, nil)
		if e == nil || e.Kind != KindResourceLimit {
			t.Fatalf("capture amplification unreserved: %v", e)
		}
	})
}

func TestQueryReferenceEligibilityMissingBatchObservation(t *testing.T) {
	f := newEligibilityFixture(t)
	q := f.witness.Queries[0]
	q.ID = "q1"
	f.witness.Queries = append(f.witness.Queries, q)
	tuple := f.witness.Required[0]
	tuple.Query = "q1"
	f.witness.Required = append(f.witness.Required, tuple)
	var raw nativewire.Response
	if e := jsonv2.Unmarshal(f.files["native/response.json"], &raw); e != nil {
		t.Fatal(e)
	}
	for i := range raw.Steps {
		for _, tree := range []*nativewire.WireTree{&raw.Steps[i].Incremental, raw.Steps[i].Fresh} {
			if tree == nil {
				continue
			}
			var qs []querywire.Query
			if e := jsonv2.Unmarshal(tree.Queries, &qs); e != nil {
				t.Fatal(e)
			}
			q := qs[0]
			q.ID = "q1"
			tree.Queries = eligibilityJSON(t, append(qs, q))
		}
	}
	data := eligibilityJSON(t, raw)
	f.replace("native/response.json", data)
	var process eligibilityExecution
	if e := jsonv2.Unmarshal(f.files["native/process.json"], &process); e != nil {
		t.Fatal(e)
	}
	process.StdoutBytes = uint64(len(data)) + 4
	f.replace("native/process.json", eligibilityJSON(t, process))
	for i := range f.reg.Invocations {
		inv := &f.reg.Invocations[i]
		if inv.ID == "native-current" {
			inv.Inputs["query-q1"] = inv.Inputs["query-q0"]
		}
		if inv.ID != "current-ref-0" {
			continue
		}
		var req eligibilityProcessRequest
		if e := jsonv2.Unmarshal(inv.ApprovedRequest, &req); e != nil {
			t.Fatal(e)
		}
		old := req.Args
		req.Args = []string{"--batch", old[0], old[1], old[2], old[5], "q0", old[3], "q1", old[3]}
		inv.ApprovedRequest = eligibilityJSON(t, req)
		f.replace(inv.Request.Member.Path, inv.ApprovedRequest)
		for name, index := range map[string]uint64{"library": 1, "source": 3, "query-0": 6} {
			x := inv.Inputs[name]
			x.ArgvIndex = eligibilityIndex(int(index))
			inv.Inputs[name] = x
		}
		x := inv.Inputs["query-0"]
		x.ArgvIndex = eligibilityIndex(8)
		inv.Inputs["query-1"] = x
		var receipt eligibilityReceipt
		if e := jsonv2.Unmarshal(f.files[inv.Receipt.Member.Path], &receipt); e != nil {
			t.Fatal(e)
		}
		original, e := base64.StdEncoding.DecodeString(receipt.StdoutBase64)
		if e != nil {
			t.Fatal(e)
		}
		row := []byte(strings.TrimSpace(string(original)))
		var ref instrumentedReference
		if e := jsonv2.Unmarshal(row, &ref); e != nil {
			t.Fatal(e)
		}
		ref.Query.ID = "q1"
		stdout := append(slices.Clone(row), '\n')
		stdout = append(stdout, eligibilityJSON(t, ref)...)
		stdout = append(stdout, '\n')
		footer := eligibilityBatchFooter{Schema: "tsgk-instrumented-query-reference-batch/r1", Status: StatusCompleted, Complete: true, Queries: []string{"q0", "q1"}, ResponsesCompleted: 2}
		stdout = append(stdout, eligibilityJSON(t, footer)...)
		stdout = append(stdout, '\n')
		f.stdoutMutation(inv.ID, func([]byte) []byte { return stdout })
		for j := range f.reg.Observations {
			o := &f.reg.Observations[j]
			if o.InvocationID == inv.ID {
				o.ByteLength = uint64(len(row))
				o.SHA256 = digestHex(row)
			}
		}
	}
	got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
	if err != nil || !got.Complete || got.Assessment != AssessBlocked || got.Tuple.Failed != 0 || got.Tuple.Blocked != 1 || got.Tuple.Verified != 3 {
		t.Fatalf("missing batch observation err%v assessment%s counts%+v", err, got.Assessment, got.Tuple)
	}
	eligibilityAssertCounts(t, got)
}

func TestQueryReferenceEligibilityMissingObservationKnownInputs(t *testing.T) {
	for _, bad := range []string{"missing-source", "missing-query", "query-hash", "library-hash", "malformed"} {
		t.Run(bad, func(t *testing.T) {
			f := newEligibilityFixture(t)
			f.reg.Observations = slices.DeleteFunc(f.reg.Observations, func(o eligibilityObservation) bool { return o.InvocationID == "current-ref-0" })
			if strings.HasPrefix(bad, "missing-") {
				for i := range f.reg.Invocations {
					if f.reg.Invocations[i].ID == "current-ref-0" {
						key := "source"
						if bad == "missing-query" {
							key = "query-0"
						}
						delete(f.reg.Invocations[i].Inputs, key)
					}
				}
			} else {
				f.stdoutMutation("current-ref-0", func(data []byte) []byte {
					if bad == "malformed" {
						return append([]byte(`{"unknown":true,`), data[1:]...)
					}
					var ref instrumentedReference
					if e := jsonv2.Unmarshal(data, &ref); e != nil {
						t.Fatal(e)
					}
					if bad == "query-hash" {
						ref.Query.SHA256 = strings.Repeat("0", 64)
					} else {
						ref.Grammar.LibrarySHA256 = strings.Repeat("0", 64)
					}
					return eligibilityJSON(t, ref)
				})
			}
			got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
			if bad == "malformed" {
				if err == nil || asError(err).Kind != KindInvalidInput || got.Complete {
					t.Fatalf("malformed err%v complete%v", err, got.Complete)
				}
			} else {
				want := AssessFail
				if strings.HasPrefix(bad, "missing-") {
					want = AssessBlocked
				}
				if err != nil || !got.Complete || got.Assessment != want {
					t.Fatalf("known input err%v assessment%s", err, got.Assessment)
				}
			}
			eligibilityAssertCounts(t, got)
		})
	}
}

func TestQueryReferenceEligibilityReview37(t *testing.T) {
	t.Run("partial-field-product", func(t *testing.T) {
		r, e := startRun(ctxT(t), 30*time.Second)
		if e != nil {
			t.Fatal(e)
		}
		defer r.cancel()
		b := &eligibilityBudget{r: r}
		r.eligibility = b
		s := &eligibilitySession{r: r, b: b}
		fields := make([]string, 131072)
		for i := range fields {
			fields[i] = "f"
		}
		tree := nativewire.WireTree{Fields: fields, PartialTrees: make([]nativewire.WirePartial, 32768), Queries: []byte("[]"), API: []byte("null")}
		e = s.nativeReservation(nativewire.Response{Steps: []nativewire.Step{{Incremental: tree}}}, nil)
		if e == nil || e.Kind != KindResourceLimit {
			t.Fatalf("partial field product unreserved: %v", e)
		}
	})
	t.Run("source-points-without-native", func(t *testing.T) {
		f := newEligibilityFixture(t)
		f.reg.Native = nil
		f.stdoutMutation("current-ref-0", func(data []byte) []byte {
			var ref instrumentedReference
			if e := jsonv2.Unmarshal(data, &ref); e != nil {
				t.Fatal(e)
			}
			ref.Tree.Nodes[3].StartPoint.Column = 0
			changed := eligibilityJSON(t, ref)
			for i := range f.reg.Observations {
				o := &f.reg.Observations[i]
				if o.InvocationID == "current-ref-0" {
					o.ByteLength = uint64(len(changed))
					o.SHA256 = digestHex(changed)
				}
			}
			return changed
		})
		got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
		found := false
		for _, f := range got.Findings {
			if f.Code == "ELIGIBILITY_CURRENT_RESPONSE" && strings.HasSuffix(f.Path, "/REFERENCE_TREE_POINTS") {
				found = true
			}
		}
		if err != nil || !got.Complete || got.Assessment != AssessFail || !found {
			t.Fatalf("point contradiction hidden: err%v assessment%s finding%v", err, got.Assessment, found)
		}
	})
	t.Run("known-empty-query", func(t *testing.T) {
		f := newEligibilityFixture(t)
		pin := f.member("current-ref-0", "current-ref-0/empty-query.bin", "query", []byte{})
		f.reg.Observations = slices.DeleteFunc(f.reg.Observations, func(o eligibilityObservation) bool { return o.InvocationID == "current-ref-0" })
		for i := range f.reg.Invocations {
			if f.reg.Invocations[i].ID == "current-ref-0" {
				x := f.reg.Invocations[i].Inputs["query-0"]
				x.MemberPin = &pin
				f.reg.Invocations[i].Inputs["query-0"] = x
			}
		}
		got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
		if err != nil || !got.Complete || got.Assessment != AssessFail || !eligibilityFinding(got, "ELIGIBILITY_CURRENT_RESPONSE") {
			t.Fatalf("empty query hash hidden: err%v assessment%s", err, got.Assessment)
		}
	})
}

func TestQueryReferenceEligibilityMissingSourceMalformedNative(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(fmt.Sprint(nested), func(t *testing.T) {
			f := newEligibilityFixture(t)
			delete(f.files, "native/source.bin")
			data := []byte(`{"unknown":true}`)
			if nested {
				var raw nativewire.Response
				if e := jsonv2.Unmarshal(f.files["native/response.json"], &raw); e != nil {
					t.Fatal(e)
				}
				raw.Steps[0].Incremental.Queries = []byte(`[{"unknown":true}]`)
				data = eligibilityJSON(t, raw)
			}
			f.replace("native/response.json", data)
			got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
			if err == nil || asError(err).Kind != KindInvalidInput || got.Complete || got.Assessment != AssessFail {
				t.Fatalf("shape hidden by source absence: err%v complete%v assessment%s", err, got.Complete, got.Assessment)
			}
			eligibilityAssertCounts(t, got)
		})
	}
}

func (f *eligibilityFixture) nativeMutation(change func([]byte) []byte) {
	data := change(slices.Clone(f.files["native/response.json"]))
	f.replace("native/response.json", data)
	var process eligibilityExecution
	if e := jsonv2.Unmarshal(f.files["native/process.json"], &process); e != nil {
		f.t.Fatal(e)
	}
	process.StdoutBytes = uint64(len(data)) + 4
	f.replace("native/process.json", eligibilityJSON(f.t, process))
}

func TestQueryReferenceEligibilityReview38(t *testing.T) {
	for _, bad := range []string{"missing-step", "null-has-error", "null-node-integer"} {
		for _, missing := range []bool{false, true} {
			for _, prior := range []bool{false, true} {
				t.Run(fmt.Sprintf("outer-%s-source-missing-%v-prior-%v", bad, missing, prior), func(t *testing.T) {
					f := newEligibilityFixture(t)
					f.nativeMutation(func(data []byte) []byte {
						var raw map[string]any
						if e := jsonv2.Unmarshal(data, &raw); e != nil {
							t.Fatal(e)
						}
						step := raw["steps"].([]any)[0].(map[string]any)
						tree := step["incremental"].(map[string]any)
						switch bad {
						case "missing-step":
							delete(step, "step")
						case "null-has-error":
							tree["has_error"] = nil
						default:
							tree["nodes"].([]any)[0].([]any)[0] = nil
						}
						return eligibilityJSON(t, raw)
					})
					if missing {
						delete(f.files, "native/source.bin")
					}
					req := f.request()
					if prior {
						a := f.approval
						a.RegistrationSHA256 = strings.Repeat("0", 64)
						req.Approval = eligibilityJSON(t, a)
					}
					got, err := VerifyQueryReferenceEligibility(ctxT(t), req)
					if err == nil || asError(err).Kind != KindInvalidInput || got.Complete || (prior || missing) && got.Assessment != AssessFail || !prior && !missing && got.Assessment == AssessFail {
						t.Fatalf("outer shape err%v complete%v assessment%s", err, got.Complete, got.Assessment)
					}
					eligibilityAssertCounts(t, got)
				})
			}
		}
	}
	for _, anchor := range []bool{false, true} {
		for _, bad := range []string{"index-only", "metadata", "identity", "summary-index"} {
			t.Run(fmt.Sprintf("cst-%s-anchor-%v", bad, anchor), func(t *testing.T) {
				f := newEligibilityFixture(t)
				if !anchor {
					f.witness.Required[0].Structural = nil
					f.witness.Required[0].Expected = nil
				}
				if bad == "summary-index" {
					f.nativeMutation(func(data []byte) []byte {
						var raw nativewire.Response
						if e := jsonv2.Unmarshal(data, &raw); e != nil {
							t.Fatal(e)
						}
						summary := "summary"
						tree := &raw.Steps[0].Incremental
						tree.Form = &summary
						tree.Nodes = nil
						tree.Errors = &nativewire.WireErrors{Items: []nativewire.WireError{}}
						return eligibilityJSON(t, raw)
					})
				}

				f.stdoutMutation("current-ref-0", func(data []byte) []byte {
					var ref instrumentedReference
					if e := jsonv2.Unmarshal(data, &ref); e != nil {
						t.Fatal(e)
					}
					old := ref.Tree.Nodes
					wrapper := old[1]
					wrapper.Type = "uncaptured-wrapper"
					wrapper.Parent = 0
					nodes := append(slices.Clone(old[:1]), wrapper)
					nodes = append(nodes, old[1:]...)
					for i := 2; i < len(nodes); i++ {
						if nodes[i].Parent > 0 {
							nodes[i].Parent++
						}
					}
					nodes[2].Parent = 1
					ref.Tree.Nodes = nodes
					for i := range ref.Query.Structural {
						ref.Query.Structural[i].Node++
					}
					for i := range ref.Query.Evaluated {
						ref.Query.Evaluated[i].Node++
					}
					if bad == "metadata" {
						ref.Query.Patterns = 2
					}
					if bad == "identity" {
						ref.Query.Matches = 0
						ref.Query.MatchMapping = ref.Query.MatchMapping[:0]
						ref.Query.Structural = []Capture{}
						ref.Query.Evaluated = []Capture{}
					}
					changed := eligibilityJSON(t, ref)
					for i := range f.reg.Observations {
						o := &f.reg.Observations[i]
						if o.InvocationID == "current-ref-0" {
							o.ByteLength = uint64(len(changed))
							o.SHA256 = digestHex(changed)
						}
					}
					return changed
				})
				got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
				want := AssessFail
				if bad == "index-only" || bad == "summary-index" && !anchor {
					want = AssessBlocked
				}
				if err != nil || !got.Complete || got.Assessment != want || bad == "index-only" && got.Tuple.Failed != 0 {
					t.Fatalf("CST distinction err%v assessment%s tuple%+v", err, got.Assessment, got.Tuple)
				}
				eligibilityAssertCounts(t, got)
			})
		}
	}
	for _, required := range []bool{false, true} {
		for _, kind := range []string{"point", "eof"} {
			t.Run(fmt.Sprintf("native-%s-required-%v", kind, required), func(t *testing.T) {
				f := newEligibilityFixture(t)
				if !required {
					f.witness.Required = f.witness.Required[:1]
					f.reg.Observations = slices.DeleteFunc(f.reg.Observations, func(o eligibilityObservation) bool { return o.Step > 0 })
					f.reg.Invocations = slices.DeleteFunc(f.reg.Invocations, func(i eligibilityInvocation) bool { return i.ID == "current-ref-1" })
				}
				f.nativeMutation(func(data []byte) []byte {
					var raw nativewire.Response
					if e := jsonv2.Unmarshal(data, &raw); e != nil {
						t.Fatal(e)
					}
					tree := &raw.Steps[1].Incremental
					if kind == "point" {
						tree.Nodes[3][7] = 0
					} else {
						tree.Nodes[0][5] = 5
						tree.Nodes[0][8] = 1
						tree.Nodes[0][9] = 2
					}
					ns, e := nativewire.Nodes(tree.Nodes, tree.Types, tree.Fields)
					if e != nil {
						t.Fatal(e)
					}
					tree.Digest = TreeDigest(ns)
					return eligibilityJSON(t, raw)
				})
				got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
				code := "ELIGIBILITY_NATIVE_TREE_POINTS"
				if kind == "eof" {
					code = "ELIGIBILITY_NATIVE_TREE_EOF"
				}
				if err != nil || !got.Complete || got.Assessment != AssessFail || !eligibilityFinding(got, code) {
					t.Fatalf("native source contradiction err%v assessment%s finding%v", err, got.Assessment, eligibilityFinding(got, code))
				}
				eligibilityAssertCounts(t, got)
			})
		}
	}
}

func TestQueryReferenceEligibilityNativeShape(t *testing.T) {
	f := newEligibilityFixture(t)
	var raw nativewire.Response
	if e := jsonv2.Unmarshal(f.files["native/response.json"], &raw); e != nil {
		t.Fatal(e)
	}
	raw.Steps[1].Route = &nativewire.Route{}
	raw.Steps[0].Incremental.Errors = &nativewire.WireErrors{Items: []nativewire.WireError{{}}}
	declarations := []nativewire.DeclItem{{Name: &nativewire.NameSpan{}}}
	raw.Steps[0].Incremental.Declarations = &declarations
	raw.Steps[0].Incremental.PartialTrees = []nativewire.WirePartial{{Nodes: [][]int64{}}}
	original := eligibilityJSON(t, raw)
	// These objects exercise encoding only; their synthetic values make no semantic claim.
	subjects := [][]any{{}, {"producer"}, {"steps", 1}, {"steps", 0, "incremental"}, {"steps", 1, "edit"}, {"steps", 1, "route"}, {"steps", 0, "incremental", "errors"}, {"steps", 0, "incremental", "errors", "items", 0}, {"steps", 0, "incremental", "declarations", 0}, {"steps", 0, "incremental", "declarations", 0, "name"}, {"steps", 0, "incremental", "partial_trees", 0}}
	at := func(root any, path []any) map[string]any {
		for _, part := range path {
			switch p := part.(type) {
			case string:
				root = root.(map[string]any)[p]
			case int:
				root = root.([]any)[p]
			}
		}
		return root.(map[string]any)
	}
	for index, path := range subjects {
		var root any
		if e := jsonv2.Unmarshal(original, &root); e != nil {
			t.Fatal(e)
		}
		for field, value := range at(root, path) {
			for _, bad := range []string{"missing", "null", "type"} {
				opaque := field == "queries" || field == "api" && index == 3 || field == "reuse_observation" || field == "observations"
				if opaque && bad != "missing" {
					continue
				}
				nullable := index == 2 && (field == "edit" || field == "route" || field == "fresh") || index == 3 && (field == "form" || field == "types" || field == "fields" || field == "nodes" || field == "errors" || field == "declarations" || field == "partial_trees") || index == 8 && field == "name"
				t.Run(fmt.Sprintf("object-%d-%s-%s", index, field, bad), func(t *testing.T) {
					var changed any
					if e := jsonv2.Unmarshal(original, &changed); e != nil {
						t.Fatal(e)
					}
					obj := at(changed, path)
					switch bad {
					case "missing":
						delete(obj, field)
					case "null":
						obj[field] = nil
					default:
						if _, ok := value.(string); ok {
							obj[field] = true
						} else {
							obj[field] = "wrong type"
						}
					}
					r, e := startRun(ctxT(t), 30*time.Second)
					if e != nil {
						t.Fatal(e)
					}
					defer r.cancel()
					b := &eligibilityBudget{r: r}
					r.eligibility = b
					var decoded nativewire.Response
					e = b.decode("native-shape", eligibilityJSON(t, changed), &decoded, true)
					if bad == "null" && nullable {
						if e != nil {
							t.Fatalf("nullable wire arm rejected: %v", e)
						}
					} else if e == nil || e.Kind != KindInvalidInput {
						t.Fatalf("malformed wire accepted: %v", e)
					}
				})
			}
		}
	}
}

func TestQueryReferenceEligibilityReview39(t *testing.T) {
	for _, bad := range []string{"valid", "missing-step", "null-has-error", "nested-unknown"} {
		for _, prior := range []bool{false, true} {
			t.Run(fmt.Sprintf("missing-native-%s-prior-%v", bad, prior), func(t *testing.T) {
				f := newEligibilityFixture(t)
				f.reg.Native = nil
				if bad != "valid" {
					f.nativeMutation(func(data []byte) []byte {
						var raw map[string]any
						if e := jsonv2.Unmarshal(data, &raw); e != nil {
							t.Fatal(e)
						}
						step := raw["steps"].([]any)[0].(map[string]any)
						tree := step["incremental"].(map[string]any)
						switch bad {
						case "missing-step":
							delete(step, "step")
						case "null-has-error":
							tree["has_error"] = nil
						default:
							tree["queries"] = []any{map[string]any{"unknown": true}}
						}
						return eligibilityJSON(t, raw)
					})
				}
				if prior {
					for i := range f.reg.Invocations {
						if f.reg.Invocations[i].ID == "current-ref-0" {
							f.reg.Invocations[i].Node.Identities["run"] = "wrong-run"
						}
					}
				}
				got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
				malformed := bad != "valid"
				if (err != nil) != malformed || malformed && asError(err).Kind != KindInvalidInput || got.Complete == malformed {
					t.Fatalf("raw shape hidden: err%v complete%v assessment%s", err, got.Complete, got.Assessment)
				}
				if prior {
					if got.Assessment != AssessFail || got.Tuple.Failed != 1 || !eligibilityFinding(got, "ELIGIBILITY_NODE_BINDING") {
						t.Fatalf("known failure lost: assessment%s counts%+v", got.Assessment, got.Tuple)
					}
				} else if got.Tuple.Failed != 0 || !malformed && got.Assessment != AssessBlocked || malformed && got.Assessment != AssessNotAssessed {
					t.Fatalf("missing group assessment%s counts%+v", got.Assessment, got.Tuple)
				}
				eligibilityAssertCounts(t, got)
			})
		}
	}
}

func TestQueryReferenceEligibilityReview42(t *testing.T) {
	for _, extra := range []bool{false, true} {
		for _, bad := range []string{"valid", "missing-query-valid", "missing-query-code", "missing-query-link", "valid-zero-null", "valid-zero-array", "pinned-null", "valid-own", "valid-reordered", "raw-id", "raw-abi", "raw-capability", "raw-complete", "raw-status", "raw-steps-completed", "raw-step-index", "raw-initial-shape", "raw-source-hash", "raw-source-bytes", "raw-point", "raw-eof", "logger-run", "logger-role", "logger-initial-accepted", "logger-old-source", "request-id", "request-shape", "approved-shape", "process-shape", "process-stdout", "process-cleanup", "process-limit", "request-path", "request-limit", "executable-pin"} {
			for _, prior := range []bool{false, true} {
				if !extra && bad == "valid-reordered" {
					continue
				}
				if prior && bad != "request-id" && bad != "request-shape" && !strings.HasPrefix(bad, "missing-query-") && !strings.HasPrefix(bad, "valid-zero-") && bad != "pinned-null" {
					continue
				}
				t.Run(fmt.Sprintf("extra-%v-%s-prior-%v", extra, bad, prior), func(t *testing.T) {
					f := newEligibilityFixture(t)
					index := slices.IndexFunc(f.reg.Invocations, func(inv eligibilityInvocation) bool { return inv.ID == "native-current" })
					if extra {
						inv := f.reg.Invocations[index]
						inv.Inputs = maps.Clone(inv.Inputs)
						inv.ID, inv.CatalogueID = "native-extra", "native-extra"
						node := f.node(inv.ID, strings.Repeat("a", 40))
						inv.Node = QueryReferenceNode{ID: node.ID, Identities: node.Identities}
						for _, pair := range []struct {
							pin  **eligibilityMember
							name string
						}{{&inv.Request, "request"}, {&inv.Response, "response"}, {&inv.Process, "process"}} {
							old := **pair.pin
							replacement := f.member(inv.ID, "native-extra/"+pair.name+".json", old.Member.Role, slices.Clone(f.files[old.Member.Path]))
							*pair.pin = &replacement
						}
						f.reg.Invocations = append(f.reg.Invocations, inv)
						index = len(f.reg.Invocations) - 1
					} else {
						f.reg.Native = nil
					}
					inv := &f.reg.Invocations[index]
					malformed := false
					switch bad {
					case "request-id", "request-path", "request-limit":
						var request eligibilityNativeExpected
						if e := jsonv2.Unmarshal(f.files[inv.Request.Member.Path], &request); e != nil {
							t.Fatal(e)
						}
						switch bad {
						case "request-id":
							request.RequestID = "other-request"
						case "request-path":
							request.Process.Path = "/other/native"
						case "request-limit":
							request.Process.Wall = 0
							malformed = true
						}
						f.replace(inv.Request.Member.Path, eligibilityJSON(t, request))
						if bad != "request-id" {
							inv.ApprovedRequest = eligibilityJSON(t, request)
						}
					case "request-shape":
						f.replace(inv.Request.Member.Path, []byte("{"))
						malformed = true
					case "approved-shape":
						inv.ApprovedRequest = []byte(`{"unknown":true}`)
						malformed = true
					case "process-shape":
						f.replace(inv.Process.Member.Path, []byte("{"))
						malformed = true
					case "process-stdout", "process-cleanup", "process-limit":
						var process eligibilityExecution
						if e := jsonv2.Unmarshal(f.files[inv.Process.Member.Path], &process); e != nil {
							t.Fatal(e)
						}
						switch bad {
						case "process-stdout":
							process.StdoutBytes++
						case "process-cleanup":
							process.Cleanup.Verified = false
						default:
							process.Memory.Limit++
						}
						f.replace(inv.Process.Member.Path, eligibilityJSON(t, process))
					case "executable-pin":
						pin := *inv.Executable
						pin.Member.SHA256 = strings.Repeat("0", 64)
						inv.Executable = &pin
					default:
						f.nativeIndependentMutation(inv, bad)
					}
					if prior {
						for i := range f.reg.Invocations {
							if f.reg.Invocations[i].ID == "current-ref-0" {
								f.reg.Invocations[i].Node.Identities["run"] = "wrong-run"
							}
						}
					}
					got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
					if (err != nil) != malformed || malformed && asError(err).Kind != KindInvalidInput || got.Complete == malformed {
						t.Fatalf("unselected validation err%v complete%v assessment%s", err, got.Complete, got.Assessment)
					}
					if prior {
						if got.Assessment != AssessFail || got.Tuple.Failed != 1 || !eligibilityFinding(got, "ELIGIBILITY_NODE_BINDING") {
							t.Fatalf("prior failure lost: assessment%s counts%+v", got.Assessment, got.Tuple)
						}
					} else if !malformed {
						want := AssessFail
						if bad == "missing-query-valid" {
							want = AssessBlocked
						}
						if strings.HasPrefix(bad, "valid") {
							want = AssessBlocked
							if extra {
								want = AssessPass
							}
						}
						if got.Assessment != want || got.Tuple.Failed != 0 {
							t.Fatalf("unselected assessment%s want%s counts%+v", got.Assessment, want, got.Tuple)
						}
					}
					if bad == "request-id" && !eligibilityFinding(got, "ELIGIBILITY_NATIVE_REQUEST") {
						t.Fatal("stored-approved request contradiction was not checked")
					}
					if (bad == "missing-query-code" || bad == "missing-query-link") && !eligibilityFinding(got, "ELIGIBILITY_NATIVE_QUERY") {
						t.Fatal("source-independent query contradiction was not checked")
					}
					if bad == "pinned-null" && !eligibilityFinding(got, "ELIGIBILITY_NATIVE_QUERY_MISSING") {
						t.Fatal("missing pinned query was not checked")
					}
					eligibilityAssertCounts(t, got)
				})
			}
		}
	}
}

func (f *eligibilityFixture) nativeIndependentMutation(inv *eligibilityInvocation, bad string) {
	if bad == "valid" {
		return
	}
	t := f.t
	var raw nativewire.Response
	if e := jsonv2.Unmarshal(f.files[inv.Response.Member.Path], &raw); e != nil {
		t.Fatal(e)
	}
	switch bad {
	case "missing-query-valid", "missing-query-code", "missing-query-link", "valid-zero-null", "valid-zero-array", "pinned-null":
		if bad != "pinned-null" {
			delete(inv.Inputs, "query-q0")
		}
		for k := range raw.Steps {
			for _, tree := range []*nativewire.WireTree{&raw.Steps[k].Incremental, raw.Steps[k].Fresh} {
				if tree == nil {
					continue
				}
				switch bad {
				case "valid-zero-null", "pinned-null":
					tree.Queries = []byte("null")
				case "valid-zero-array":
					tree.Queries = []byte("[]")
				case "missing-query-code", "missing-query-link":
					var queries []querywire.Query
					if e := jsonv2.Unmarshal(tree.Queries, &queries); e != nil {
						t.Fatal(e)
					}
					if bad == "missing-query-code" {
						queries[0].Code = "FORGED"
					} else {
						queries[0].Captures[0][3] = 1
					}
					tree.Queries = eligibilityJSON(t, queries)
				}
			}
		}
	case "valid-own":
		var own eligibilityNativeExpected
		if e := jsonv2.Unmarshal(inv.ApprovedRequest, &own); e != nil {
			t.Fatal(e)
		}
		own.RequestID, own.ObservationRunID, own.ProducerSHA256, own.Process.Path = "own-request", "own-run", strings.Repeat("e", 64), "/own/native"
		pin := f.member("native-static", "native/other-executable.bin", "native-executable", []byte("other synthetic executable"))
		own.ExecutableSHA256 = pin.Member.SHA256
		own.ProducerABI.SymbolCount++
		source := f.member("native-static", "native/other-source.bin", "source", []byte("z;\n1;\n"))
		inv.Inputs["source"] = eligibilityInput{Kind: "PINNED", MemberPin: &source, ContentBasis: "PRESERVED_ORIGINAL"}
		inv.Executable, inv.ExecutablePath, inv.ApprovedRequest = &pin, own.Process.Path, eligibilityJSON(t, own)
		f.replace(inv.Request.Member.Path, inv.ApprovedRequest)
		raw.ID, raw.Producer.SymbolCount = own.RequestID, &own.ProducerABI.SymbolCount
		raw.Steps[0].SourceSHA256 = source.Member.SHA256
		for k := range raw.Steps {
			var obs []reuseobs.Observation
			if e := jsonv2.Unmarshal(raw.Steps[k].Observations, &obs); e != nil {
				t.Fatal(e)
			}
			for j := range obs {
				obs[j].RequestID, obs[j].RunID, obs[j].ProducerSHA256 = own.RequestID, own.ObservationRunID, own.ProducerSHA256
				if k == 0 {
					obs[j].SourceSHA256 = source.Member.SHA256
				} else if k == 1 && obs[j].OldTree {
					sha := source.Member.SHA256
					obs[j].OldSourceSHA256 = &sha
				}
			}
			raw.Steps[k].Observations = eligibilityJSON(t, obs)
		}
	case "valid-reordered":
		inv.Inputs["query-q1"] = inv.Inputs["query-q0"]
		for k := range raw.Steps {
			for _, tree := range []*nativewire.WireTree{&raw.Steps[k].Incremental, raw.Steps[k].Fresh} {
				if tree == nil {
					continue
				}
				var queries []querywire.Query
				if e := jsonv2.Unmarshal(tree.Queries, &queries); e != nil {
					t.Fatal(e)
				}
				second := queries[0]
				second.ID = "q1"
				tree.Queries = eligibilityJSON(t, []querywire.Query{second, queries[0]})
			}
		}
	case "raw-id":
		raw.ID = "wrong-request"
	case "raw-abi":
		*raw.Producer.SymbolCount++
	case "raw-capability":
		raw.Producer.ReuseObservation = eligibilityJSON(t, map[string]string{"policy": "wrong", "runtime_manifest_sha256": reuseobs.RuntimeManifestSHA256})
	case "raw-complete":
		raw.Complete = false
	case "raw-status":
		raw.Status, raw.Code = StatusResourceLimit, "ALLOCATION_LIMIT"
	case "raw-steps-completed":
		raw.StepsCompleted--
	case "raw-step-index":
		raw.Steps[0].Step++
	case "raw-initial-shape":
		raw.Steps[0].Edit = raw.Steps[1].Edit
	case "raw-source-hash":
		raw.Steps[0].SourceSHA256 = strings.Repeat("0", 64)
	case "raw-source-bytes":
		raw.SourceBytes++
	case "raw-point", "raw-eof":
		tree := &raw.Steps[0].Incremental
		if bad == "raw-point" {
			tree.Nodes[0][9] = 77
		} else {
			tree.Nodes[0][5]--
		}
		nodes, e := nativewire.Nodes(tree.Nodes, tree.Types, tree.Fields)
		if e != nil {
			t.Fatal(e)
		}
		tree.Digest = TreeDigest(nodes)
	case "logger-run", "logger-role", "logger-initial-accepted", "logger-old-source":
		k := 0
		if bad == "logger-old-source" {
			k = 1
		}
		var obs []reuseobs.Observation
		if e := jsonv2.Unmarshal(raw.Steps[k].Observations, &obs); e != nil {
			t.Fatal(e)
		}
		switch bad {
		case "logger-run":
			obs[0].RunID = "wrong-run"
		case "logger-role":
			obs[0].Role = "fresh"
		case "logger-initial-accepted":
			obs[0].Counts.Accepted = 1
			obs[0].Events++
		default:
			sha := strings.Repeat("0", 64)
			obs[0].OldSourceSHA256 = &sha
		}
		raw.Steps[k].Observations = eligibilityJSON(t, obs)
	default:
		t.Fatalf("unknown native mutation %s", bad)
	}
	data := eligibilityJSON(t, raw)
	f.replace(inv.Response.Member.Path, data)
	var process eligibilityExecution
	if e := jsonv2.Unmarshal(f.files[inv.Process.Member.Path], &process); e != nil {
		t.Fatal(e)
	}
	process.StdoutBytes = uint64(len(data)) + 4
	f.replace(inv.Process.Member.Path, eligibilityJSON(t, process))
}

func TestQueryReferenceEligibilityIndependentBudget(t *testing.T) {
	for _, mode := range []string{"full", "partial", "fields", "query-source", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(ctxT(t))
			defer cancel()
			r, e := startRun(ctx, 30*time.Second)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			b := &eligibilityBudget{r: r}
			r.eligibility = b
			s := &eligibilitySession{r: r, b: b}
			tree := nativewire.WireTree{}
			switch mode {
			case "full", "partial":
				tree.Types = []string{strings.Repeat("x", 1<<20)}
				rows := make([][]int64, 10000)
				for i := range rows {
					rows[i] = []int64{0, 0, -1, 1, 0, 1, 0, 0, 0, 1}
				}
				if mode == "full" {
					tree.Nodes = rows
				} else {
					tree.PartialTrees = []nativewire.WirePartial{{Nodes: rows}}
				}
			case "fields":
				tree.Fields = make([]string, 65536)
				tree.PartialTrees = make([]nativewire.WirePartial, 1024)
			case "query-source":
				b.work = eligibilityWorkLimit - 1<<20
				q := querywire.Query{ID: "q", Status: "INVALID_QUERY", Code: "QUERY_SYNTAX", Error: &querywire.CompileError{Type: "SYNTAX", Offset: 8 << 20, Point: [2]uint32{0, 8 << 20}}}
				checked := nativewire.Tree{Wire: nativewire.WireTree{Queries: eligibilityJSON(t, []querywire.Query{q})}}
				e = s.nativeIndependentQueries(&eligibilityInvocation{ID: "extra"}, &checked, map[string][]byte{"q": make([]byte, 8<<20)}, map[string]bool{"q": true}, 0, nil)
				if e == nil || e.Kind != KindResourceLimit || e.Path != "native-independent-query-source" {
					t.Fatalf("query source scan unreserved: %v", e)
				}
				return
			case "cancel":
				cancel()
			}
			e = s.nativeIndependentTreeBudget(tree, nil, false)
			kind := KindResourceLimit
			if mode == "cancel" {
				kind = KindCancelled
			}
			if e == nil || e.Kind != kind {
				t.Fatalf("independent traversal guard: %v", e)
			}
		})
	}
}

func TestQueryReferenceEligibilityIndependentFailureBeforeCancellation(t *testing.T) {
	for _, semantic := range []bool{false, true} {
		t.Run(fmt.Sprint(semantic), func(t *testing.T) {
			ctx, cancel := context.WithCancel(ctxT(t))
			defer cancel()
			r, e := startRun(ctx, 30*time.Second)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			result := QueryReferenceEligibilityResult{Report: newReport("test")}
			failed := false
			s := &eligibilitySession{r: r, res: &result, failItem: func() { failed = true }}
			_, contradiction := nativewire.CheckTreeStructure(nativewire.WireTree{Status: "FORGED"}, 0, nativewire.Expect{})
			if contradiction == nil {
				t.Fatal("fixture must contradict tree status")
			}
			cancel()
			if !semantic {
				contradiction = r.check()
			}
			got := s.nativeIndependentError("extra", "ELIGIBILITY_NATIVE_TREE", contradiction)
			if got == nil || got.Kind != KindCancelled {
				t.Fatalf("cancellation lost: %v", got)
			}
			if failed != semantic || (result.Assessment == AssessFail) != semantic || (s.failures == 1) != semantic {
				t.Fatalf("known contradiction retention: semantic=%v failed=%v assessment=%s failures=%d", semantic, failed, result.Assessment, s.failures)
			}
		})
	}
}
