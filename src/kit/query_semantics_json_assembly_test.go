package kit

import (
	"context"
	"encoding/base64"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
)

// Assembly creates a review candidate, not an eligibility approval.
func TestQuerySemanticsJSONAssemble(t *testing.T) {
	if os.Getenv("TSGK_QUERY_JSON_LOCAL_STAGE") != "assemble" {
		t.Skip("explicit local captured input is required")
	}
	actualJSONAssemble(t, os.Getenv("TSGK_QUERY_JSON_LOCAL_ROOT"), os.Getenv("TSGK_QUERY_JSON_FRESH_REFERENCE"), os.Getenv("TSGK_QUERY_JSON_CAPTURE"))
}

type actualJSONAssembly struct {
	t            *testing.T
	evidence     string
	reg          eligibilityRegistration
	nodes        map[string]EvidenceNode
	cohortSource map[string]string
	bound        []semanticBoundMember
}

func (a *actualJSONAssembly) node(id, cohort, tool, input, query, protocol, run string, status, assessment string) EvidenceNode {
	n := EvidenceNode{ID: id, Kind: "run", ExecutionStatus: status, EvidenceMode: ModeNewRun, Assessment: assessment, Identities: map[string]string{
		"source": a.cohortSource[cohort], "tool": tool, "input": input, "query": query, "policy": digestHex([]byte(eligibilityCatalogueJSON)), "comparator": CaptureCompareScheme,
		"protocol": protocol, "workload": "json24-reference", "platform": "windows/amd64", "run": run, "attempt": "1"}, Files: []ReplayMember{}, Refs: []EvidenceRef{}}
	a.nodes[id] = n
	return n
}

func (a *actualJSONAssembly) add(owner, path, role string, data []byte) eligibilityMember {
	a.t.Helper()
	if len(data) > MaxDocumentBytes {
		a.t.Fatal("member too large", path)
	}
	full := filepath.Join(a.evidence, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		a.t.Fatal(err)
	}
	actualJSONWrite(a.t, full, data)
	m := eligibilityMember{NodeID: owner, Member: ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)}}
	n, ok := a.nodes[owner]
	if !ok {
		a.t.Fatal("missing owner", owner)
	}
	n.Files = append(n.Files, m.Member)
	a.nodes[owner] = n
	a.reg.OwnedMembers = append(a.reg.OwnedMembers, m)
	return m
}

func (a *actualJSONAssembly) copy(owner, path, role, source string) eligibilityMember {
	return a.add(owner, path, role, actualJSONRead(a.t, source))
}
func actualJSONDecode(t *testing.T, path string, value any) []byte {
	t.Helper()
	data := actualJSONRead(t, path)
	if err := jsonv2.Unmarshal(data, value); err != nil {
		t.Fatal(path, err)
	}
	return data
}

func (a *actualJSONAssembly) reference(id, cohort, catalogue, requestPath, receiptPath string, inlineExit int, toolSHA string) int {
	var req eligibilityProcessRequest
	request := actualJSONDecode(a.t, requestPath, &req)
	var receipt eligibilityReceipt
	actualJSONDecode(a.t, receiptPath, &receipt)
	tool := toolSHA
	assessment := AssessNotAssessed
	if receipt.Execution.Status == StatusCompleted && receipt.Execution.ExitCode == 0 {
		assessment = AssessPass
	}
	n := a.node(id, cohort, tool, digestHex(request), "NOT_APPLICABLE", instrumentedReferenceSchema, id, receipt.Execution.Status, assessment)
	r := a.copy(id, cohort+"/invocations/"+id+"-request.json", "process-request", requestPath)
	s := a.copy(id, cohort+"/invocations/"+id+"-receipt.json", "process-receipt", receiptPath)
	a.reg.Invocations = append(a.reg.Invocations, eligibilityInvocation{ID: id, Kind: "REFERENCE_PROCESS", CohortID: cohort, CatalogueID: catalogue, Node: QueryReferenceNode{ID: n.ID, Identities: n.Identities}, Request: &r, Receipt: &s, ApprovedRequest: request, Inputs: map[string]eligibilityInput{}, ExitAttestation: &eligibilityExit{Basis: "reviewed-inline/r1", Value: inlineExit, ProvenanceID: "p-" + cohort}})
	return len(a.reg.Invocations) - 1
}

func actualJSONPinned(pin eligibilityMember, index int, literal, basis string) eligibilityInput {
	return eligibilityInput{Kind: "PINNED", MemberPin: &pin, ContentBasis: basis, ArgvIndex: eligibilityIndex(index), PathLiteral: literal}
}

func actualJSONAssemble(t *testing.T, root, fresh, capture string) {
	t.Helper()
	for _, p := range []string{root, fresh, capture} {
		if !filepath.IsAbs(p) {
			t.Fatal("absolute local input required")
		}
	}
	var freshContext, currentContext struct {
		SourceRevision string `json:"source_revision"`
		SessionID      string `json:"session_id"`
	}
	actualJSONDecode(t, filepath.Join(fresh, "context-before.json"), &freshContext)
	for _, stage := range []string{"outer-build", "outer-controls"} {
		var receipt eligibilityReceipt
		actualJSONDecode(t, filepath.Join(fresh, stage, "receipt.json"), &receipt)
		if receipt.Execution.Status != StatusCompleted || receipt.Execution.ExitCode != 0 || !receipt.Execution.Cleanup.Verified || receipt.Execution.Cleanup.ResidualAfterExit != 0 {
			t.Fatal("static capture operation", stage)
		}
	}
	actualJSONDecode(t, filepath.Join(capture, "context-before.json"), &currentContext)
	var captured struct {
		Passed              bool   `json:"passed"`
		NativeCaptureOrigin string `json:"native_capture_origin"`
		Library             struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"library"`
		References []struct {
			ID      string `json:"id"`
			Step    uint32 `json:"step"`
			Query   string `json:"query"`
			Request string `json:"request"`
			Receipt string `json:"receipt"`
		} `json:"reference_invocations"`
	}
	actualJSONDecode(t, filepath.Join(capture, "final.json"), &captured)
	if !captured.Passed || len(captured.References) != 16 {
		t.Fatal("actual capture not completed")
	}
	var buildIdentity eligibilityBuildIdentity
	actualJSONDecode(t, filepath.Join(fresh, "build", "identity.json"), &buildIdentity)
	var cat eligibilityCatalogue
	if err := jsonv2.Unmarshal([]byte(eligibilityCatalogueJSON), &cat); err != nil {
		t.Fatal(err)
	}
	assembly, err := os.MkdirTemp(root, "assembly-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("assembly_root=%q", assembly)
	evidence := filepath.Join(assembly, "evidence")
	caller := filepath.Join(assembly, "caller")
	for _, p := range []string{evidence, caller} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	a := &actualJSONAssembly{t: t, evidence: evidence, nodes: map[string]EvidenceNode{}, cohortSource: map[string]string{"build": freshContext.SourceRevision, "control": freshContext.SourceRevision, "current": currentContext.SourceRevision},
		reg: eligibilityRegistration{Schema: QueryReferenceEligibilityRegistrationSchema, Context: "full-input-utf8/r2", CatalogueSHA256: digestHex([]byte(eligibilityCatalogueJSON)), OwnedMembers: []eligibilityMember{}, Tools: []eligibilityTool{}, Invocations: []eligibilityInvocation{}, Observations: []eligibilityObservation{}}}
	a.node("build-static", "build", buildIdentity.Tools["python"], digestHex(eligibilityJSON(t, buildIdentity)), "NOT_APPLICABLE", "tsgk-instrumented-reference-build/r1", "outer-build", StatusCompleted, AssessPass)
	identity := a.copy("build-static", "build/identity.json", "build-identity", filepath.Join(fresh, "build", "identity.json"))
	source := a.copy("build-static", "build/source-snapshot.json", "source-snapshot", filepath.Join(fresh, "build", "source-snapshot.json"))
	vendor := a.copy("build-static", "build/vendor-snapshot.json", "vendor-snapshot", filepath.Join(fresh, "build", "vendor-snapshot.json"))
	binaryPath := filepath.Join(fresh, "build", "target", buildIdentity.Target, "release", "tsgk-query-reference.exe")
	binary := a.copy("build-static", "build/reference.exe", "reference-executable", binaryPath)
	patchPath := filepath.Join("..", "dev", "query-reference", "instrumentation.patch")
	patch := a.copy("build-static", "build/instrumentation.patch", "patch", patchPath)
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bp := map[string]string{"B": filepath.Join(fresh, "build"), "H": filepath.Join(repo, "src", "dev", "query-reference"), "S": filepath.Join(fresh, "build", "source"), "T": buildIdentity.Target, "R": binaryPath}
	a.reg.Build = &eligibilityBuild{CohortID: "build", Paths: bp, Identity: identity, SourceSnapshot: source, VendorSnapshot: vendor, ReferenceBinary: &binary}
	for _, name := range []string{"cargo", "rustc", "cc", "ar", "git", "supervisor", "python"} {
		a.reg.Tools = append(a.reg.Tools, eligibilityTool{ID: "build-tool-" + name, CohortID: "build", ToolName: name, PathLiteral: buildIdentity.ToolPaths[name], SubjectSHA256: buildIdentity.Tools[name], IdentityMember: identity, ProvenanceID: "p-build"})
	}
	for _, rule := range cat.Builds {
		i := a.reference("build-"+rule.ID, "build", rule.ID, filepath.Join(fresh, "build", rule.ID+"-request.json"), filepath.Join(fresh, "build", rule.ID+".json"), 0, buildIdentity.Tools[rule.Tool])
		inv := &a.reg.Invocations[i]
		inv.ToolID = "build-tool-" + rule.Tool
		inv.Inputs["source"] = eligibilityInput{Kind: "NOT_APPLICABLE"}
		inv.Inputs["library"] = eligibilityInput{Kind: "NOT_APPLICABLE"}
		if rule.ID == "patch" || rule.ID == "patch-check" {
			var req eligibilityProcessRequest
			if err := jsonv2.Unmarshal(inv.ApprovedRequest, &req); err != nil {
				t.Fatal(err)
			}
			index := 2
			if rule.ID == "patch-check" {
				index = 3
			}
			inv.Inputs["patch"] = actualJSONPinned(patch, index, req.Args[index], "PRESERVED_ORIGINAL")
		}
	}
	a.node("control-static", "control", buildIdentity.Tools["python"], identity.Member.SHA256, "NOT_APPLICABLE", "owned-controls/r1", "outer-controls", StatusCompleted, AssessPass)
	controlRoot := filepath.Join(fresh, "controls")
	summary := a.copy("control-static", "control/summary.json", "control-summary", filepath.Join(controlRoot, "summary.json"))
	controlLibrary := a.copy("control-static", "control/plain.dll", "runtime-library", filepath.Join(controlRoot, "plain.dll"))
	grammar := map[string]eligibilityMember{}
	for name := range cat.GrammarInputs {
		role := "grammar-header"
		if name == "parser.c" {
			role = "grammar-parser"
		}
		grammar[name] = a.copy("control-static", "control/grammar/"+name, role, filepath.Join(controlRoot, "grammar", filepath.FromSlash(name)))
	}
	inputs := map[string]eligibilityMember{}
	for name, row := range cat.Inputs {
		pin := a.copy("control-static", "control/input/"+name, "control-input", filepath.Join(controlRoot, filepath.FromSlash(name)))
		if pin.Member.Bytes != row.Bytes || pin.Member.SHA256 != row.SHA256 {
			t.Fatal("preserved control input mismatch", name)
		}
		inputs[name] = pin
	}
	cp := map[string]string{"C": controlRoot, "G": filepath.Join(controlRoot, "grammar"), "L": filepath.Join(controlRoot, "plain.dll"), "LH": controlLibrary.Member.SHA256, "R": binaryPath}
	a.reg.Control = &eligibilityControl{CohortID: "control", Paths: cp, Summary: summary, Library: &controlLibrary, Grammar: grammar}
	for _, name := range []string{"cc", "supervisor", "python"} {
		a.reg.Tools = append(a.reg.Tools, eligibilityTool{ID: "control-tool-" + name, CohortID: "control", ToolName: name, PathLiteral: buildIdentity.ToolPaths[name], SubjectSHA256: buildIdentity.Tools[name], IdentityMember: summary, ProvenanceID: "p-control"})
	}
	for _, rule := range cat.Controls {
		toolSHA := binary.Member.SHA256
		if rule.Tool == "cc" {
			toolSHA = buildIdentity.Tools["cc"]
		}
		i := a.reference("control-"+rule.ID, "control", rule.ID, filepath.Join(controlRoot, rule.ID+"-request.json"), filepath.Join(controlRoot, rule.ID+".json"), rule.ExpectedSupervisorExit, toolSHA)
		inv := &a.reg.Invocations[i]
		var req eligibilityProcessRequest
		if err := jsonv2.Unmarshal(inv.ApprovedRequest, &req); err != nil {
			t.Fatal(err)
		}
		if rule.ID == "compile" {
			inv.ToolID = "control-tool-cc"
			inv.Inputs["source"] = eligibilityInput{Kind: "NOT_APPLICABLE"}
			inv.Inputs["library"] = eligibilityInput{Kind: "NOT_APPLICABLE"}
			inv.Inputs["grammar-parser"] = actualJSONPinned(grammar["parser.c"], 5, req.Args[5], "PRESERVED_ORIGINAL")
			inv.Inputs["output-library"] = actualJSONPinned(controlLibrary, 7, req.Args[7], "PRESERVED_ORIGINAL")
			continue
		}
		inv.Executable = &binary
		inv.ExecutablePath = binaryPath
		si, li := 2, 0
		batch := req.Args[0] == "--batch"
		if batch {
			si, li = 3, 1
		}
		name, err := filepath.Rel(controlRoot, req.Args[si])
		if err != nil {
			t.Fatal(err)
		}
		src, ok := inputs[filepath.ToSlash(name)]
		if !ok {
			t.Fatal("control source missing", name)
		}
		inv.Inputs["source"] = actualJSONPinned(src, si, req.Args[si], "PRESERVED_ORIGINAL")
		inv.Inputs["library"] = actualJSONPinned(controlLibrary, li, req.Args[li], "PRESERVED_ORIGINAL")
		indices := []int{3}
		if batch {
			indices = nil
			for k := 6; k < len(req.Args); k += 2 {
				indices = append(indices, k)
			}
		}
		for j, k := range indices {
			name, err := filepath.Rel(controlRoot, req.Args[k])
			if err != nil {
				t.Fatal(err)
			}
			slot := fmt.Sprintf("query-%d", j)
			if filepath.ToSlash(name) == "missing.scm" {
				inv.Inputs[slot] = eligibilityInput{Kind: "EXPECTED_ABSENT", ArgvIndex: eligibilityIndex(k), PathLiteral: req.Args[k]}
				continue
			}
			q, ok := inputs[filepath.ToSlash(name)]
			if !ok {
				t.Fatal("control query missing", name)
			}
			inv.Inputs[slot] = actualJSONPinned(q, k, req.Args[k], "PRESERVED_ORIGINAL")
		}
	}
	actualJSONAssembleCurrent(t, a, root, caller, capture, captured.NativeCaptureOrigin, captured.Library.Path, binary, binaryPath)
	graphNodes := make([]EvidenceNode, 0, len(a.nodes))
	ids := make([]string, 0, len(a.nodes))
	for id := range a.nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	required := []map[string]any{}
	for _, id := range ids {
		n := a.nodes[id]
		graphNodes = append(graphNodes, n)
		required = append(required, map[string]any{"node": n.ID, "kind": n.Kind, "identities": n.Identities})
	}
	graph := eligibilityJSON(t, map[string]any{"schema": EvidenceSchema, "nodes": graphNodes})
	a.reg.Graph = ReplayMember{Path: "evidence.json", Role: "evidence-graph", Bytes: uint64(len(graph)), SHA256: digestHex(graph)}
	actualJSONWrite(t, filepath.Join(evidence, "evidence.json"), graph)
	policy := eligibilityJSON(t, map[string]any{"schema": EvidencePolicySchema, "id": "actual-json24", "evidence_sha256": digestHex(graph), "required": required, "carry_forward": []any{}})
	actualJSONWrite(t, filepath.Join(caller, "policy.json"), policy)
	actualJSONWrite(t, filepath.Join(caller, "eligibility-registration.json"), eligibilityJSON(t, a.reg))
	actualJSONWrite(t, filepath.Join(caller, "binding-members.json"), eligibilityJSON(t, a.bound))
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	result, e := VerifyEvidence(ctx, EvidenceRequest{Root: evidence, Policy: policy})
	actualJSONWrite(t, filepath.Join(caller, "evidence-result.json"), eligibilityJSON(t, result))
	if e != nil || result.ExecutionStatus != StatusCompleted || result.Assessment != AssessPass {
		t.Fatal("actual graph integrity", e, result.Findings)
	}
	t.Log("actual graph candidate assembled; caller approval and run-binding intentionally absent")
}

func actualJSONAssembleCurrent(t *testing.T, a *actualJSONAssembly, root, caller, capture, origin, libraryPath string, binary eligibilityMember, binaryPath string) {
	var reg semanticRegistration
	regData := actualJSONDecode(t, filepath.Join(root, "plan", "registration.json"), &reg)
	profileData := actualJSONRead(t, filepath.Join(root, "plan", "profile.json"))
	profile, err := ParseOracleProfile(profileData)
	if err != nil {
		t.Fatal(err)
	}
	var nativeRequest semanticNativeRequest
	frame := actualJSONRead(t, filepath.Join(root, "evidence", "oracle", "capture", "00000-c1-stdin.bin"))
	payload, e := semanticTestBudget(t).semanticFrame("actual-stdin", frame)
	if e != nil {
		t.Fatal(e)
	}
	if e := jsonv2.Unmarshal(payload, &nativeRequest); e != nil {
		t.Fatal(e)
	}
	if nativeRequest.Protocol != "tsgk-native/r4" || nativeRequest.ID != reg.Case || nativeRequest.API || nativeRequest.ObservationHistoryStart != 0 {
		t.Fatal("native request context")
	}
	var captured semanticProcessCapture
	actualJSONDecode(t, filepath.Join(root, "evidence", "oracle", "capture", "00000-c1.json"), &captured)
	if captured.Stage != "RUN_RETURNED" || captured.Spec == nil || captured.Error != nil {
		t.Fatal("native capture incomplete")
	}
	var expectedProcess eligibilityProcessRequest
	if e := jsonv2.Unmarshal(eligibilityJSON(t, captured.Spec), &expectedProcess); e != nil {
		t.Fatal(e)
	}
	// runner.launch executes a nil environment as an empty array; keep raw capture unchanged.
	if expectedProcess.Env == nil {
		expectedProcess.Env = []string{}
	}
	nativeExe := actualJSONRead(t, filepath.Join(root, "evidence", "oracle", "capture", "driver.bin"))
	runtimeManifest, e2 := drivers.NativeC.ReadFile("native-c/runtime-manifest.json")
	if e2 != nil {
		t.Fatal(e2)
	}
	expected := eligibilityNativeExpected{Protocol: "tsgk-native/r4", RequestID: reg.Case, NodeLimit: reg.Limits.Nodes, API: false, Points: []uint32{}, RuntimeManifestSHA256: digestHex(runtimeManifest), ProducerSHA256: nativeRequest.ObservationProducerSHA256, ExecutableSHA256: digestHex(nativeExe), ProducerABI: eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: 23, FieldCount: 2}, ObservationRunID: nativeRequest.ObservationRunID, HistoryStart: 0, Process: expectedProcess}
	witness := eligibilityWitness{QueryWitnessRegistration: reg.QueryWitnessRegistration, Native: expected}
	witness.Schema = "tsgk-query-witness-registration/r2"
	witnessData := eligibilityJSON(t, witness)
	a.node("native-c1", "current", expected.ExecutableSHA256, reg.Input.SHA256, digestHex(eligibilityJSON(t, profile.Queries)), expected.Protocol, "native-c1", StatusCompleted, AssessPass)
	var report struct {
		CLISHA256 string `json:"cli_sha256"`
		SessionID string `json:"session_id"`
	}
	actualJSONDecode(t, filepath.Join(capture, "final.json"), &report)
	a.node("oracle-json24", "current", report.CLISHA256, digestHex(profileData), digestHex(eligibilityJSON(t, profile.Queries)), OracleSemanticsManifestSchema, report.SessionID, StatusCompleted, AssessBlocked)
	n := a.nodes["native-c1"]
	n.Identities["policy"] = reg.NativePolicy.SHA256
	a.nodes[n.ID] = n
	o := a.nodes["oracle-json24"]
	o.Identities["policy"] = reg.NativePolicy.SHA256
	a.nodes[o.ID] = o
	set := VerifyOracleSet(os.DirFS(filepath.Join(root, "evidence", "oracle")))
	if !set.Valid || set.Assessment != AssessBlocked {
		t.Fatal("original native set", set.Findings)
	}
	nativeMembers := map[string]eligibilityMember{}
	for _, plan := range reg.PlannedMembers {
		owner := "native-c1"
		if plan.Owner == "O" {
			owner = "oracle-json24"
		}
		pin := a.copy(owner, "oracle/"+plan.Path, plan.Role, filepath.Join(root, "evidence", "oracle", filepath.FromSlash(plan.Path)))
		a.bound = append(a.bound, semanticBoundMember{Owner: plan.Owner, Local: plan.Path, NodeID: owner, Member: pin.Member})
		nativeMembers[plan.Path] = pin
	}
	// Additional source/query pins belong to the actual native invocation, not a new run.
	qPins := map[string]eligibilityMember{}
	for _, q := range reg.Queries {
		pin := a.copy("native-c1", "current/input/"+q.ID+".scm", "query-source", filepath.Join(root, "input", "queries", q.ID+".scm"))
		if pin.Member.SHA256 != digestHex([]byte(q.Source)) {
			t.Fatal("query source changed", q.ID)
		}
		qPins[q.ID] = pin
	}
	sourcePins := map[uint32]eligibilityMember{}
	for step := uint32(0); step < 2; step++ {
		sourcePins[step] = a.copy("native-c1", fmt.Sprintf("current/input/source-%d.bin", step), "reference-source", filepath.Join(root, "input", fmt.Sprintf("source-%d.bin", step)))
	}
	nativeSource := nativeMembers["source/00000-c1.bin"]
	build := nativeMembers["capture/build-manifest.txt"]
	nativeGrammar := map[string]eligibilityMember{}
	for i, g := range profile.Native.Grammar {
		nativeGrammar[g.Path] = nativeMembers[fmt.Sprintf("capture/build-input-%05d.bin", 83+i)]
	}
	// The JSON library compilation is supplementary evidence, not a reference invocation.
	var buildIdentity eligibilityBuildIdentity
	actualJSONDecode(t, filepath.Join(a.evidence, "build", "identity.json"), &buildIdentity)
	a.node("json-library-build", "current", buildIdentity.Tools["cc"], digestHex(eligibilityJSON(t, profile.Native.Grammar)), "NOT_APPLICABLE", "compiler-capture/r1", "json-library", StatusCompleted, AssessPass)
	library := a.copy("json-library-build", "current/json.dll", "runtime-library", libraryPath)
	a.copy("json-library-build", "current/library-request.json", "compiler-request", filepath.Join(origin, "json-library", "request.json"))
	a.copy("json-library-build", "current/library-receipt.json", "compiler-receipt", filepath.Join(origin, "json-library", "receipt.json"))
	a.reg.Native = &eligibilityNative{InvocationID: "native-c1", Source: nativeSource, BuildManifest: build, ReferenceLibrary: &library, Symbol: "tree_sitter_json", Grammar: nativeGrammar}
	request := a.add("native-c1", "current/native-expected-request.json", "native-request", eligibilityJSON(t, expected))
	process := nativeMembers["capture/00000-c1-process.json"]
	response := nativeMembers["raw/00000-c1.json"]
	executable := nativeMembers["capture/driver.bin"]
	nativeInv := eligibilityInvocation{ID: "native-c1", Kind: "NATIVE_DRIVER", CohortID: "current", CatalogueID: "native-c1", Node: QueryReferenceNode{ID: n.ID, Identities: n.Identities}, Request: &request, Response: &response, Process: &process, Executable: &executable, ExecutablePath: expectedProcess.Path, ApprovedRequest: eligibilityJSON(t, expected), Inputs: map[string]eligibilityInput{"source": {Kind: "PINNED", MemberPin: &nativeSource, ContentBasis: "PRESERVED_ORIGINAL"}, "library": {Kind: "NOT_APPLICABLE"}}}
	for _, q := range reg.Queries {
		pin := qPins[q.ID]
		nativeInv.Inputs["query-"+q.ID] = eligibilityInput{Kind: "PINNED", MemberPin: &pin, ContentBasis: "APPROVED_EXPECTED_BYTES"}
	}
	a.reg.Invocations = append(a.reg.Invocations, nativeInv)
	var capturedRefs struct {
		References []struct {
			ID      string `json:"id"`
			Step    uint32 `json:"step"`
			Query   string `json:"query"`
			Request string `json:"request"`
			Receipt string `json:"receipt"`
		} `json:"reference_invocations"`
	}
	actualJSONDecode(t, filepath.Join(capture, "final.json"), &capturedRefs)
	type observed struct {
		id    string
		bytes uint64
		sha   string
	}
	observations := map[string]observed{}
	refs := []semanticReferenceBinding{}
	for _, r := range capturedRefs.References {
		i := a.reference(r.ID, "current", r.ID, r.Request, r.Receipt, 0, binary.Member.SHA256)
		inv := &a.reg.Invocations[i]
		inv.Executable = &binary
		inv.ExecutablePath = binaryPath
		var req eligibilityProcessRequest
		if e := jsonv2.Unmarshal(inv.ApprovedRequest, &req); e != nil {
			t.Fatal(e)
		}
		inv.Inputs["library"] = actualJSONPinned(library, 0, req.Args[0], "PRESERVED_ORIGINAL")
		inv.Inputs["source"] = actualJSONPinned(sourcePins[r.Step], 2, req.Args[2], "PRESERVED_ORIGINAL")
		inv.Inputs["query-0"] = actualJSONPinned(qPins[r.Query], 3, req.Args[3], "PRESERVED_ORIGINAL")
		var receipt eligibilityReceipt
		actualJSONDecode(t, r.Receipt, &receipt)
		stdout, e := base64.StdEncoding.DecodeString(receipt.StdoutBase64)
		if e != nil {
			t.Fatal(e)
		}
		observations[fmt.Sprintf("%d/%s", r.Step, r.Query)] = observed{r.ID, uint64(len(stdout)), digestHex(stdout)}
		refs = append(refs, semanticReferenceBinding{InvocationID: r.ID, CohortID: "current", Receipt: *inv.Receipt})
	}
	for _, tuple := range reg.Required {
		observation, ok := observations[fmt.Sprintf("%d/%s", tuple.Step, tuple.Query)]
		if !ok {
			t.Fatal("missing reference", tuple)
		}
		source := sourcePins[tuple.Step]
		a.reg.Observations = append(a.reg.Observations, eligibilityObservation{Query: tuple.Query, Step: tuple.Step, TreeRole: tuple.TreeRole, SourceBytes: source.Member.Bytes, SourceSHA256: source.Member.SHA256, InvocationID: observation.id, QueryID: tuple.Query, ResponseOrdinal: 0, ByteOffset: 0, ByteLength: observation.bytes, SHA256: observation.sha})
	}
	actualJSONWrite(t, filepath.Join(caller, "profile.json"), profileData)
	actualJSONWrite(t, filepath.Join(caller, "semantic-registration.json"), regData)
	actualJSONWrite(t, filepath.Join(caller, "witness.json"), witnessData)
	actualJSONWrite(t, filepath.Join(caller, "reference-bindings.json"), eligibilityJSON(t, refs))
	actualJSONWrite(t, filepath.Join(caller, "capture-facts.json"), eligibilityJSON(t, map[string]any{"state": "PENDING_OUTSIDE_CALLER_REVIEW", "native_invocation": "native-c1", "driver_node": "native-c1", "oracle_node": "oracle-json24", "source_revision": a.cohortSource["current"], "current_session": report.SessionID, "origin_capture": origin, "continuation_capture": capture, "source_profile_sha256": digestHex(profileData), "manifest_sha256": nativeMembers["manifest.json"].Member.SHA256}))
}
