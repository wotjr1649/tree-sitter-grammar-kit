package kit

import (
	"bytes"
	"context"
	"encoding/base64"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

const classificationGoAnchorsSHA = "096785af5cbd09ece1f649ab7ea41648f11b45c751386636238537787bec0f19"
const classificationDonorRegistrationSHA = "4032145988f49f7092830e5f30e759fd41811c41b3a690f3f6a80cfc89c69055"

type classificationAnchors struct {
	Schema string `json:"schema"`
	Route  string `json:"route"`
	Source struct {
		UTF8   string `json:"utf8"`
		Bytes  uint64 `json:"bytes"`
		SHA256 string `json:"sha256"`
	} `json:"source"`
	Edits   []Edit     `json:"edits"`
	Nodes   []TreeNode `json:"nodes_each_version"`
	Queries []struct {
		ID       string               `json:"id"`
		Source   string               `json:"source"`
		SHA256   string               `json:"sha256"`
		Metadata QueryWitnessMetadata `json:"metadata"`
	} `json:"queries"`
	Required []QueryWitnessTuple    `json:"required"`
	ABI      eligibilityProducerABI `json:"expected_producer_abi"`
}

type classificationPlan struct {
	Schema                  string        `json:"schema"`
	SourceRevision          string        `json:"source_revision"`
	RunID                   string        `json:"run_id"`
	AnchorsSHA256           string        `json:"anchors_sha256"`
	RuntimeManifestSHA256   string        `json:"runtime_manifest_sha256"`
	DonorRegistrationSHA256 string        `json:"donor_registration_sha256"`
	Grammar                 []NativeInput `json:"grammar"`
}

// These stages retain caller inputs; neither planning nor assembly issues approval.
func TestQueryClassificationGoLocal(t *testing.T) {
	stage := os.Getenv("TSGK_QUERY_CLASSIFICATION_STAGE")
	if stage == "" {
		t.Skip("explicit preregistered local classification is required")
	}
	root := os.Getenv("TSGK_QUERY_CLASSIFICATION_ROOT")
	if !filepath.IsAbs(root) {
		t.Fatal("absolute caller-owned classification root required")
	}
	switch stage {
	case "plan":
		classificationGoPlan(t, root)
	case "assemble":
		classificationGoAssemble(t, root, os.Getenv("TSGK_QUERY_CLASSIFICATION_DONOR"))
	case "verify":
		classificationGoVerify(t, root, os.Getenv("TSGK_QUERY_CLASSIFICATION_ASSEMBLY"), os.Getenv("TSGK_QUERY_CLASSIFICATION_APPROVAL"))
	default:
		t.Fatal("unknown classification stage", stage)
	}
}

func classificationGoAnchors(t *testing.T) (classificationAnchors, []byte) {
	t.Helper()
	data := actualJSONRead(t, "../testdata/query-semantics/go3-classification-anchors.json")
	if digestHex(data) != classificationGoAnchorsSHA {
		t.Fatal("reviewed Go anchors changed")
	}
	var a classificationAnchors
	if err := jsonv2.Unmarshal(data, &a, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	if a.Schema != "tsgk-query-classification-anchors/r1" || a.Route != "go" || a.Source.UTF8 != "" || a.Source.Bytes != 0 || a.Source.SHA256 != digestHex(nil) || len(a.Queries) != 3 || len(a.Required) != 3 || len(a.Nodes) != 1 || len(a.Edits) != 0 {
		t.Fatal("independent Go definition denominator")
	}
	return a, data
}

func classificationGoRegistration(a classificationAnchors) QueryWitnessRegistration {
	w := QueryWitnessRegistration{Schema: queryEligibilityWitnessSchema, Policy: "tsgk-predicates/r1", Case: "go3", Route: "go", Platform: "windows/amd64", Role: "owned", Encoding: EncodingUTF8, Input: NativeInput{Path: "source.bin", Role: "case", Bytes: 0, SHA256: a.Source.SHA256}, Edits: []Edit{}, Queries: []QueryWitnessQuery{}, Required: a.Required}
	for _, q := range a.Queries {
		metadata := q.Metadata
		w.Queries = append(w.Queries, QueryWitnessQuery{ID: q.ID, Source: q.Source, Metadata: &metadata})
	}
	return w
}

func classificationGoWitnessMatches(a classificationAnchors, w QueryWitnessRegistration) bool {
	return reflect.DeepEqual(w, classificationGoRegistration(a))
}

func TestQueryClassificationGoRegistration(t *testing.T) {
	a, _ := classificationGoAnchors(t)
	w := classificationGoRegistration(a)
	if !classificationGoWitnessMatches(a, w) {
		t.Fatal("registered Go witness rejected")
	}
	if _, err := validateQueryWitnessRegistration(w, queryEligibilityWitnessSchema); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*QueryWitnessRegistration){
		func(w *QueryWitnessRegistration) { w.Route = "json" },
		func(w *QueryWitnessRegistration) { w.Queries[0].Source = "(source_file) @type" },
		func(w *QueryWitnessRegistration) { w.Queries[0].Metadata.Predicates = nil },
		func(w *QueryWitnessRegistration) { w.Required = w.Required[:2] },
	} {
		var other QueryWitnessRegistration
		if err := jsonv2.Unmarshal(eligibilityJSON(t, w), &other); err != nil {
			t.Fatal(err)
		}
		change(&other)
		if classificationGoWitnessMatches(a, other) {
			t.Fatal("classification accepted unrelated witness with identical empty-capture expectations")
		}
	}
}

func classificationGoPlan(t *testing.T, root string) {
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("plan root must be new and empty", err)
	}
	_, data := classificationGoAnchors(t)
	plan := classificationPlan{Schema: "tsgk-query-classification-plan/r1", SourceRevision: os.Getenv("TSGK_QUERY_CLASSIFICATION_SOURCE"), RunID: os.Getenv("TSGK_QUERY_CLASSIFICATION_RUN"), AnchorsSHA256: digestHex(data), DonorRegistrationSHA256: classificationDonorRegistrationSHA}
	if !eligibilityRevision(plan.SourceRevision) || !validID(plan.RunID) {
		t.Fatal("separate source revision and observation run preregistration required")
	}
	var registry struct {
		Routes []struct {
			Route string        `json:"route"`
			Files []NativeInput `json:"files"`
		} `json:"routes"`
	}
	actualJSONDecode(t, "../contracts/native-routes.json", &registry)
	for _, row := range registry.Routes {
		if row.Route == "go" {
			plan.Grammar = row.Files
		}
	}
	if len(plan.Grammar) != 5 {
		t.Fatal("Go grammar closure missing")
	}
	runtimeData := actualJSONRead(t, "../drivers/native-c/runtime-manifest.json")
	plan.RuntimeManifestSHA256 = digestHex(runtimeData)
	for _, dir := range []string{"plan", "native", "library", "references"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	actualJSONWrite(t, filepath.Join(root, "plan", "anchors.json"), data)
	actualJSONWrite(t, filepath.Join(root, "plan", "preregistration.json"), eligibilityJSON(t, plan))
	t.Log("Go3 definition preregistration only; no compiler, native, reference or approval")
}

func classificationGoAssemble(t *testing.T, root, donor string) {
	if !filepath.IsAbs(donor) {
		t.Fatal("absolute caller-selected static donor required")
	}
	anchors, _ := classificationGoAnchors(t)
	var plan classificationPlan
	actualJSONDecode(t, filepath.Join(root, "plan", "preregistration.json"), &plan)
	if plan.Schema != "tsgk-query-classification-plan/r1" || plan.AnchorsSHA256 != classificationGoAnchorsSHA || plan.DonorRegistrationSHA256 != classificationDonorRegistrationSHA {
		t.Fatal("preregistration changed")
	}
	var captured struct {
		Passed         bool   `json:"passed"`
		SourceRevision string `json:"source_revision"`
		RunID          string `json:"run_id"`
	}
	actualJSONDecode(t, filepath.Join(root, "capture-final.json"), &captured)
	if !captured.Passed || captured.SourceRevision != plan.SourceRevision || captured.RunID != plan.RunID {
		t.Fatal("current capture controller did not complete preregistered cohort")
	}
	var old eligibilityRegistration
	oldData := actualJSONDecode(t, filepath.Join(donor, "caller", "eligibility-registration.json"), &old)
	if digestHex(oldData) != classificationDonorRegistrationSHA || old.CatalogueSHA256 != digestHex([]byte(eligibilityCatalogueJSON)) {
		t.Fatal("reviewed static donor changed")
	}
	var graph struct {
		Nodes []EvidenceNode `json:"nodes"`
	}
	graphData := actualJSONDecode(t, filepath.Join(donor, "evidence", "evidence.json"), &graph)
	if digestHex(graphData) != old.Graph.SHA256 || uint64(len(graphData)) != old.Graph.Bytes {
		t.Fatal("donor graph mismatch")
	}
	assembly, err := os.MkdirTemp(root, "assembly-")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"evidence", "caller"} {
		if err := os.Mkdir(filepath.Join(assembly, p), 0700); err != nil {
			t.Fatal(err)
		}
	}
	a := &actualJSONAssembly{t: t, evidence: filepath.Join(assembly, "evidence"), workload: "go3-definition-classification", nodes: map[string]EvidenceNode{}, cohortSource: map[string]string{"current": plan.SourceRevision},
		reg: eligibilityRegistration{Schema: QueryReferenceEligibilityRegistrationSchema, Context: "full-input-utf8/r2", CatalogueSHA256: old.CatalogueSHA256, Build: old.Build, Control: old.Control, Tools: []eligibilityTool{}, Invocations: []eligibilityInvocation{}, OwnedMembers: []eligibilityMember{}, Observations: []eligibilityObservation{}}}
	oldNodes := map[string]EvidenceNode{}
	for _, n := range graph.Nodes {
		oldNodes[n.ID] = n
	}
	for _, inv := range old.Invocations {
		if inv.CohortID == "build" || inv.CohortID == "control" {
			a.reg.Invocations = append(a.reg.Invocations, inv)
		}
	}
	for _, tool := range old.Tools {
		if tool.CohortID == "build" || tool.CohortID == "control" {
			a.reg.Tools = append(a.reg.Tools, tool)
		}
	}
	for _, m := range old.OwnedMembers {
		if !strings.HasPrefix(m.Member.Path, "build/") && !strings.HasPrefix(m.Member.Path, "control/") {
			continue
		}
		if _, exists := a.nodes[m.NodeID]; !exists {
			n, ok := oldNodes[m.NodeID]
			if !ok {
				t.Fatal("donor owner missing")
			}
			n.Files = []ReplayMember{}
			a.nodes[n.ID] = n
		}
		data := actualJSONRead(t, filepath.Join(donor, "evidence", filepath.FromSlash(m.Member.Path)))
		if uint64(len(data)) != m.Member.Bytes || digestHex(data) != m.Member.SHA256 {
			t.Fatal("donor member changed", m.Member.Path)
		}
		if got := a.add(m.NodeID, m.Member.Path, m.Member.Role, data); !reflect.DeepEqual(got, m) {
			t.Fatal("donor identity changed")
		}
	}
	if len(a.reg.OwnedMembers) != 178 || len(a.reg.Invocations) != 62 || len(a.reg.Tools) != 10 || a.reg.Build == nil || a.reg.Control == nil {
		t.Fatal("static donor closure incomplete")
	}
	classificationGoCurrent(t, a, root, anchors, plan)
	ids := make([]string, 0, len(a.nodes))
	for id := range a.nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	nodes := make([]EvidenceNode, 0, len(ids))
	required := []map[string]any{}
	for _, id := range ids {
		n := a.nodes[id]
		nodes = append(nodes, n)
		required = append(required, map[string]any{"node": n.ID, "kind": n.Kind, "identities": n.Identities})
	}
	data := eligibilityJSON(t, map[string]any{"schema": EvidenceSchema, "nodes": nodes})
	a.reg.Graph = ReplayMember{Path: "evidence.json", Role: "evidence-graph", Bytes: uint64(len(data)), SHA256: digestHex(data)}
	actualJSONWrite(t, filepath.Join(a.evidence, "evidence.json"), data)
	policy := eligibilityJSON(t, map[string]any{"schema": EvidencePolicySchema, "id": "actual-go3-classification", "evidence_sha256": digestHex(data), "required": required, "carry_forward": []any{}})
	actualJSONWrite(t, filepath.Join(assembly, "caller", "policy.json"), policy)
	actualJSONWrite(t, filepath.Join(assembly, "caller", "eligibility-registration.json"), eligibilityJSON(t, a.reg))
	actualJSONWrite(t, filepath.Join(assembly, "caller", "static-donor.json"), eligibilityJSON(t, map[string]any{"registration_sha256": digestHex(oldData), "graph_sha256": old.Graph.SHA256, "cohorts": "original build/control identities retained; new graph requires new outside approval"}))
	t.Logf("assembly_root=%q; no caller approval issued", assembly)
}

func classificationGoCurrent(t *testing.T, a *actualJSONAssembly, root string, anchors classificationAnchors, plan classificationPlan) {
	nativeRoot := filepath.Join(root, "native")
	var captured struct {
		Schema         string         `json:"schema"`
		AnchorsSHA256  string         `json:"anchors_sha256"`
		BuildIdentity  string         `json:"build_identity"`
		ExecutablePath string         `json:"executable_path"`
		Files          []ReplayMember `json:"files"`
		RawSHA256      string         `json:"raw_sha256"`
	}
	actualJSONDecode(t, filepath.Join(nativeRoot, "native-complete.json"), &captured)
	if captured.Schema != "tsgk-query-classification-native-capture/r1" || captured.AnchorsSHA256 != classificationGoAnchorsSHA {
		t.Fatal("native capture not bound to preregistration")
	}
	var capture semanticProcessCapture
	actualJSONDecode(t, filepath.Join(nativeRoot, "capture", "go3.json"), &capture)
	if capture.Spec == nil || capture.Error != nil || capture.Stage != "RUN_RETURNED" || capture.Process == nil || capture.Stdout == nil || capture.Spec.Stdin == nil {
		t.Fatal("native process capture incomplete")
	}
	var process eligibilityProcessRequest
	if err := jsonv2.Unmarshal(eligibilityJSON(t, capture.Spec), &process); err != nil {
		t.Fatal(err)
	}
	if process.Env == nil {
		process.Env = []string{}
	}
	queries := make([]OracleQuery, 0, 3)
	for _, q := range anchors.Queries {
		queries = append(queries, OracleQuery{ID: q.ID, Source: q.Source})
	}
	n := a.node("native-go3", "current", digestHex(actualJSONRead(t, filepath.Join(nativeRoot, "capture", "driver.bin"))), anchors.Source.SHA256, digestHex(eligibilityJSON(t, queries)), "tsgk-native/r4", plan.RunID, StatusCompleted, AssessPass)
	n.Identities["policy"] = NativePolicy(NativeOperations()["native-query"], OutputTree, nil).SHA256
	a.nodes[n.ID] = n
	pins := map[string]eligibilityMember{}
	for _, pin := range captured.Files {
		if !portable(pin.Path, false) || !strings.HasPrefix(pin.Path, "capture/") {
			t.Fatal("capture path")
		}
		data := actualJSONRead(t, filepath.Join(nativeRoot, filepath.FromSlash(pin.Path)))
		if uint64(len(data)) != pin.Bytes || digestHex(data) != pin.SHA256 {
			t.Fatal("native capture pin", pin.Path)
		}
		pins[pin.Path] = a.add(n.ID, "current/native/"+pin.Path, pin.Role, data)
	}
	raw := actualJSONRead(t, filepath.Join(nativeRoot, "raw", "go3.json"))
	if digestHex(raw) != captured.RawSHA256 {
		t.Fatal("raw capture changed")
	}
	response := a.add(n.ID, "current/native/raw.json", "raw", raw)
	source := a.copy(n.ID, "current/input/source.bin", "reference-source", filepath.Join(nativeRoot, "input", "source.bin"))
	if source.Member.Bytes != 0 || source.Member.SHA256 != anchors.Source.SHA256 {
		t.Fatal("source changed")
	}
	executable := pins["capture/driver.bin"]
	expected := eligibilityNativeExpected{Protocol: "tsgk-native/r4", RequestID: "go3", NodeLimit: NativeOperations()["native-query"].Nodes, API: false, Points: []uint32{}, RuntimeManifestSHA256: plan.RuntimeManifestSHA256, ProducerSHA256: captured.BuildIdentity, ExecutableSHA256: executable.Member.SHA256, ProducerABI: anchors.ABI, ObservationRunID: plan.RunID, HistoryStart: 0, Process: process}
	witness := eligibilityWitness{QueryWitnessRegistration: classificationGoRegistration(anchors), Native: expected}
	queryPins := map[string]eligibilityMember{}
	for _, q := range anchors.Queries {
		queryPins[q.ID] = a.copy(n.ID, "current/input/"+q.ID+".scm", "query-source", filepath.Join(nativeRoot, "input", q.ID+".scm"))
	}
	var build struct {
		Closure []struct {
			Path string `json:"path"`
			Role string `json:"role"`
		} `json:"closure"`
	}
	actualJSONDecode(t, filepath.Join(nativeRoot, "capture", "build.json"), &build)
	grammar := map[string]eligibilityMember{}
	for i, row := range build.Closure {
		if strings.HasPrefix(row.Role, "grammar-") {
			grammar[row.Path] = pins[fmt.Sprintf("capture/build-input-%05d.bin", i)]
		}
	}
	var buildIdentity eligibilityBuildIdentity
	actualJSONDecode(t, filepath.Join(a.evidence, filepath.FromSlash(a.reg.Build.Identity.Member.Path)), &buildIdentity)
	a.node("go-library-build", "current", buildIdentity.Tools["cc"], digestHex(eligibilityJSON(t, plan.Grammar)), "NOT_APPLICABLE", "compiler-capture/r1", "go-library", StatusCompleted, AssessPass)
	library := a.copy("go-library-build", "current/go.dll", "runtime-library", filepath.Join(root, "library", "go.dll"))
	a.copy("go-library-build", "current/library-request.json", "compiler-request", filepath.Join(root, "library", "request.json"))
	a.copy("go-library-build", "current/library-receipt.json", "compiler-receipt", filepath.Join(root, "library", "receipt.json"))
	a.reg.Native = &eligibilityNative{InvocationID: n.ID, Source: source, BuildManifest: pins["capture/build-manifest.txt"], ReferenceLibrary: &library, Symbol: "tree_sitter_go", Grammar: grammar}
	request := a.add(n.ID, "current/native-expected-request.json", "native-request", eligibilityJSON(t, expected))
	processPin := pins[capture.Process.Path]
	inv := eligibilityInvocation{ID: n.ID, Kind: "NATIVE_DRIVER", CohortID: "current", CatalogueID: n.ID, Node: QueryReferenceNode{ID: n.ID, Identities: n.Identities}, Request: &request, Response: &response, Process: &processPin, Executable: &executable, ExecutablePath: captured.ExecutablePath, ApprovedRequest: eligibilityJSON(t, expected), Inputs: map[string]eligibilityInput{"source": {Kind: "PINNED", MemberPin: &source, ContentBasis: "PRESERVED_ORIGINAL"}, "library": {Kind: "NOT_APPLICABLE"}}}
	for id, pin := range queryPins {
		copy := pin
		inv.Inputs["query-"+id] = eligibilityInput{Kind: "PINNED", MemberPin: &copy, ContentBasis: "APPROVED_EXPECTED_BYTES"}
	}
	a.reg.Invocations = append(a.reg.Invocations, inv)
	refBinary := *a.reg.Build.ReferenceBinary
	for i, q := range anchors.Queries {
		id := fmt.Sprintf("current-ref-%d", i)
		dir := filepath.Join(root, "references", id)
		k := a.reference(id, "current", id, filepath.Join(dir, "request.json"), filepath.Join(dir, "receipt.json"), 0, refBinary.Member.SHA256)
		ri := &a.reg.Invocations[k]
		ri.Executable = &refBinary
		var spec eligibilityProcessRequest
		if err := jsonv2.Unmarshal(ri.ApprovedRequest, &spec); err != nil {
			t.Fatal(err)
		}
		ri.ExecutablePath = spec.Path
		if len(spec.Args) != 6 || spec.Args[1] != "tree_sitter_go" || spec.Args[4] != q.ID || spec.Args[5] != library.Member.SHA256 {
			t.Fatal("reference argv changed")
		}
		ri.Inputs["library"] = actualJSONPinned(library, 0, spec.Args[0], "PRESERVED_ORIGINAL")
		ri.Inputs["source"] = actualJSONPinned(source, 2, spec.Args[2], "PRESERVED_ORIGINAL")
		ri.Inputs["query-0"] = actualJSONPinned(queryPins[q.ID], 3, spec.Args[3], "PRESERVED_ORIGINAL")
		var receipt eligibilityReceipt
		actualJSONDecode(t, filepath.Join(dir, "receipt.json"), &receipt)
		stdout, err := base64.StdEncoding.DecodeString(receipt.StdoutBase64)
		if err != nil {
			t.Fatal(err)
		}
		a.reg.Observations = append(a.reg.Observations, eligibilityObservation{Query: q.ID, Step: 0, TreeRole: "incremental", SourceBytes: 0, SourceSHA256: anchors.Source.SHA256, InvocationID: id, QueryID: q.ID, ResponseOrdinal: 0, ByteOffset: 0, ByteLength: uint64(len(stdout)), SHA256: digestHex(stdout)})
	}
	actualJSONWrite(t, filepath.Join(filepath.Dir(a.evidence), "caller", "witness.json"), eligibilityJSON(t, witness))
}

func classificationGoVerify(t *testing.T, root, assembly, approvalPath string) {
	for _, p := range []string{assembly, approvalPath} {
		if !filepath.IsAbs(p) {
			t.Fatal("absolute caller approval/assembly required")
		}
	}
	anchors, _ := classificationGoAnchors(t)
	var plan classificationPlan
	actualJSONDecode(t, filepath.Join(root, "plan", "preregistration.json"), &plan)
	if plan.Schema != "tsgk-query-classification-plan/r1" || plan.AnchorsSHA256 != classificationGoAnchorsSHA {
		t.Fatal("classification preregistration changed")
	}
	caller := filepath.Join(assembly, "caller")
	req := QueryReferenceEligibilityRequest{Evidence: EvidenceRequest{Root: filepath.Join(assembly, "evidence"), Policy: actualJSONRead(t, filepath.Join(caller, "policy.json"))}, Approval: actualJSONRead(t, approvalPath), Registration: actualJSONRead(t, filepath.Join(caller, "eligibility-registration.json")), WitnessRegistration: actualJSONRead(t, filepath.Join(caller, "witness.json"))}
	r, e := startRun(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		t.Fatal(e)
	}
	defer r.cancel()
	cache := &semanticNativeCache{}
	scope := &eligibilityScope{r: r, native: cache}
	core, err := verifyQueryReferenceEligibilityInRun(r.caller, req, scope)
	if err != nil || core.Assessment != AssessPass || !core.Complete || core.Tuple != (QueryEligibilityCounts{Required: 3, Verified: 3}) {
		t.Fatal("eligibility not complete", err, core.Findings)
	}
	if !cache.ready || len(cache.checked) != 1 || CompareTrees(cache.checked[0].Incremental.Nodes, anchors.Nodes) != nil {
		t.Fatal("guarded native independent CST")
	}
	s := scope.session
	if !classificationGoWitnessMatches(anchors, s.witness.QueryWitnessRegistration) || s.witness.Native.ProducerABI != anchors.ABI || s.witness.Native.ObservationRunID != plan.RunID || s.witness.Native.RuntimeManifestSHA256 != plan.RuntimeManifestSHA256 || s.approval.Candidate.SourceRevision != plan.SourceRevision || s.approval.Candidate.Platform != "windows/amd64" || s.approval.Candidate.Run != plan.RunID || s.approval.Candidate.Attempt != 1 || s.reg.Native == nil || s.reg.Native.Symbol != "tree_sitter_go" || len(s.reg.Native.Grammar) != len(plan.Grammar) {
		t.Fatal("classification of an unrelated approved workload")
	}
	for _, g := range plan.Grammar {
		m, ok := s.reg.Native.Grammar[g.Path]
		if !ok || m.Member.Bytes != g.Bytes || m.Member.SHA256 != g.SHA256 || m.Member.Role != "grammar-"+g.Role {
			t.Fatal("classification grammar context", g.Path)
		}
	}
	classificationGoFrames(t, s, anchors, plan)
	for _, tuple := range anchors.Required {
		o, ok := s.observation(tuple)
		if !ok {
			t.Fatal("guarded reference observation absent")
		}
		out := s.outputs[o.InvocationID]
		if out == nil || o.ResponseOrdinal >= uint64(len(out.responses)) || out.responses[o.ResponseOrdinal] == nil || CompareTrees(out.responses[o.ResponseOrdinal].Tree.Nodes, anchors.Nodes) != nil || digestHex(out.rows[o.ResponseOrdinal]) != o.SHA256 {
			t.Fatal("guarded reference independent CST")
		}
	}
	// This second API operation checks its public wrapper; anchors use the retained core snapshot above.
	public, err := VerifyQueryReferenceEligibility(context.Background(), req)
	if err != nil || public.Assessment != AssessPass || !public.Complete || public.Tuple != core.Tuple || public.Build != core.Build || public.Unit != core.Unit || public.Control != core.Control || public.Assertion != core.Assertion || public.WitnessSHA256 != core.WitnessSHA256 || public.RegistrationSHA256 != core.RegistrationSHA256 || public.GraphSHA256 != core.GraphSHA256 {
		t.Fatal("public eligibility parity", err)
	}
	rows := []map[string]any{}
	for _, q := range anchors.Queries {
		rows = append(rows, map[string]any{"route": "go", "role": "owned", "platform": "windows/amd64", "query": q.ID, "query_sha256": q.SHA256, "class": "STRUCTURAL_ONLY", "assessment": AssessPass, "semantic_evaluated": false})
	}
	actualJSONWrite(t, filepath.Join(caller, "classification-result.json"), eligibilityJSON(t, map[string]any{"schema": "tsgk-query-classification-local-result/r1", "assessment": AssessPass, "required": 3, "verified": 3, "complete": true, "anchors_sha256": classificationGoAnchorsSHA, "registration_sha256": core.RegistrationSHA256, "witness_sha256": core.WitnessSHA256, "approval_sha256": core.ApprovalSHA256, "graph_sha256": core.GraphSHA256, "definitions": rows, "eligibility": public, "independent_native_reference_cst": "PASS", "scope": "caller-approved conditional definition classification; not semantic EVALUATED or inventory completion"}))
	t.Log("guarded independent CST and public eligibility3 verified; conditional Go3 classification only")
}

func classificationGoFrames(t *testing.T, s *eligibilitySession, a classificationAnchors, plan classificationPlan) {
	t.Helper()
	captureData, ok := s.snapshot.files["current/native/capture/go3.json"]
	if !ok {
		t.Fatal("guarded process capture absent")
	}
	capture, e := s.b.processCapture("go3/capture", captureData)
	if e != nil || capture.Spec == nil || capture.Spec.Stdin == nil || capture.Process == nil || capture.Stdout == nil || capture.Error != nil {
		t.Fatal("guarded process capture", e)
	}
	read := func(pin *ReplayMember) []byte {
		t.Helper()
		mapped := *pin
		mapped.Path = "current/native/" + mapped.Path
		data, ok, e := s.member(&eligibilityMember{NodeID: "native-go3", Member: mapped})
		if e != nil || !ok {
			t.Fatal("guarded frame member", mapped.Path, e)
		}
		return data
	}
	request, e := s.b.nativeRequestFrame("go3/stdin", read(capture.Spec.Stdin))
	if e != nil {
		t.Fatal(e)
	}
	if request.Protocol != "tsgk-native/r4" || request.ID != "go3" || request.Encoding != EncodingUTF8 || request.Output != OutputTree || request.Limits != semanticLimitsFor(NativeOperations()["native-query"]) || request.API || len(request.source) != 0 || len(request.edits) != 0 || len(request.Points) != 0 || len(request.Declarations) != 0 || len(request.Ranges) != 0 || len(request.queries) != 3 || request.ReuseObservation != reuseobs.Policy || request.ObservationRunID != plan.RunID || request.ObservationProducerSHA256 != s.witness.Native.ProducerSHA256 || request.ObservationHistoryStart != 0 {
		t.Fatal("captured request differs from preregistration")
	}
	for i, q := range a.Queries {
		if request.queries[i].ID != q.ID || !bytes.Equal(request.queries[i].Source, []byte(q.Source)) {
			t.Fatal("captured query differs", q.ID)
		}
	}
	var spec eligibilityProcessRequest
	if e := jsonv2.Unmarshal(eligibilityJSON(t, capture.Spec), &spec); e != nil {
		t.Fatal(e)
	}
	if spec.Env == nil {
		spec.Env = []string{}
	}
	if !reflect.DeepEqual(spec, s.witness.Native.Process) {
		t.Fatal("actual process spec differs from approval")
	}
	stdout, e := s.b.semanticFrame("go3/stdout", read(capture.Stdout))
	if e != nil {
		t.Fatal(e)
	}
	inv := s.invocations["native-go3"]
	if inv == nil || inv.Response == nil || inv.Process == nil {
		t.Fatal("native invocation absent")
	}
	raw, ok, e := s.member(inv.Response)
	if e != nil || !ok || !bytes.Equal(stdout, raw) {
		t.Fatal("guarded stdout differs from native raw", e)
	}
	process, ok, e := s.member(inv.Process)
	if e != nil || !ok || !bytes.Equal(process, read(capture.Process)) {
		t.Fatal("guarded process differs from invocation", e)
	}
}
