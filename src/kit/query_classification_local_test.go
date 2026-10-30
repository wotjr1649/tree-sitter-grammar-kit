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

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

const classificationGoAnchorsSHA = "096785af5cbd09ece1f649ab7ea41648f11b45c751386636238537787bec0f19"
const classificationPositiveAnchorsSHA = "921d28f6e805bf443260961e7394cfb4bcd0526066425b60057b37d514169288"
const classificationPositiveAPISHA = "34eaae3b1b620c426fdc3156b746b084c14439fef42e9f0fd505e2d042c86658"
const classificationCSharpAnchorsSHA = "06933d6516106308ec9fc20762e8eebd9718b26b2453ceea2cec090f382c938b"
const classificationDonorRegistrationSHA = "4032145988f49f7092830e5f30e759fd41811c41b3a690f3f6a80cfc89c69055"

type classificationAnchors struct {
	Case        string            `json:"case,omitempty"`
	API         bool              `json:"api,omitempty"`
	Operation   string            `json:"operation,omitempty"`
	DriverMode  string            `json:"driver_mode,omitempty"`
	Workload    string            `json:"workload,omitempty"`
	Expect      []StepExpectation `json:"expect,omitempty"`
	sha256      string
	expectedAPI *querywire.API
	apiSHA      string

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
	NativeContext           string                    `json:"native_context,omitempty"`
	APIAnchorsSHA256        string                    `json:"api_anchors_sha256,omitempty"`
	Schema                  string                    `json:"schema"`
	SourceRevision          string                    `json:"source_revision"`
	RunID                   string                    `json:"run_id"`
	AnchorsSHA256           string                    `json:"anchors_sha256"`
	RuntimeManifestSHA256   string                    `json:"runtime_manifest_sha256"`
	DonorRegistrationSHA256 string                    `json:"donor_registration_sha256"`
	Grammar                 []NativeInput             `json:"grammar"`
	CompilerSHA256          string                    `json:"compiler_sha256"`
	LibraryRequest          eligibilityProcessRequest `json:"library_request"`
}

// These stages retain caller inputs; neither planning nor assembly issues approval.
func TestQueryClassificationGoLocal(t *testing.T) {
	if os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE") == "csharp15-empty" {
		t.Skip("C# classification uses its explicit local entry point")
	}
	classificationLocal(t)
}

func TestQueryClassificationCSharpLocal(t *testing.T) {
	if os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE") != "csharp15-empty" {
		t.Skip("explicit C# classification profile required")
	}
	classificationLocal(t)
}

func classificationLocal(t *testing.T) {
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
	case "definitions":
		classificationGoVerifyDefinitions(t, root, os.Getenv("TSGK_QUERY_CLASSIFICATION_ASSEMBLY"), os.Getenv("TSGK_QUERY_CLASSIFICATION_APPROVAL"))
	default:
		t.Fatal("unknown classification stage", stage)
	}
}

func classificationGoAnchors(t *testing.T) (classificationAnchors, []byte) {
	t.Helper()
	name, expectedSHA := "go3-classification-anchors.json", classificationGoAnchorsSHA
	profile := os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE")
	route, definitions := "go", 3
	switch profile {
	case "":
	case "positive-batch":
		name, expectedSHA = "go-positive-batch-anchors.json", classificationPositiveAnchorsSHA
	case "csharp15-empty":
		name, expectedSHA, route, definitions = "csharp15-classification-anchors.json", classificationCSharpAnchorsSHA, "csharp", 15
	default:
		t.Fatal("unknown closed classification profile", profile)
	}
	data := actualJSONRead(t, "../testdata/query-semantics/"+name)
	if digestHex(data) != expectedSHA {
		t.Fatal("reviewed classification anchors changed")
	}
	var a classificationAnchors
	if err := jsonv2.Unmarshal(data, &a, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	if a.Schema != "tsgk-query-classification-anchors/r1" || a.Route != route || a.Source.Bytes != uint64(len(a.Source.UTF8)) || a.Source.SHA256 != digestHex([]byte(a.Source.UTF8)) || len(a.Queries) != definitions || len(a.Required) != definitions || len(a.Edits) != 0 {
		t.Fatal("independent definition denominator or source changed")
	}
	if profile == "" {
		if a.Source.Bytes != 0 || len(a.Nodes) != 1 || a.Case != "" || a.API || a.Operation != "" || a.DriverMode != "" || a.Workload != "" || len(a.Expect) != 0 {
			t.Fatal("legacy Go3 profile changed")
		}
		a.Case, a.Operation, a.DriverMode, a.Workload = "go3", "native-query", "single", "go3-definition-classification"
	} else if profile == "positive-batch" {
		if a.Case != "probe-go-B04-P-6" || !a.API || a.Operation != "native-query-batch" || a.DriverMode != "batch" || a.Workload != "go-positive-single-frame-batch" || a.Source.Bytes != 50 || len(a.Nodes) != 25 || len(a.Expect) != 1 {
			t.Fatal("positive batch profile changed")
		}
	} else if a.Case != "csharp15-empty-definitions" || a.API || a.Operation != "native-query" || a.DriverMode != "single" || a.Workload != "csharp15-empty-definition-single-frame" || a.Source.Bytes != 0 || len(a.Nodes) != 1 || len(a.Expect) != 1 || a.Expect[0].Step != 0 || a.Expect[0].Syntax != "NO_ERROR" {
		t.Fatal("C# empty single profile changed")
	}
	if a.API {
		apiData := actualJSONRead(t, "../testdata/query-semantics/go-positive-batch-api.json")
		if digestHex(apiData) != classificationPositiveAPISHA {
			t.Fatal("reviewed API expectations changed")
		}
		if err := jsonv2.Unmarshal(apiData, &a.expectedAPI, jsonv2.RejectUnknownMembers(true)); err != nil {
			t.Fatal(err)
		}
		a.apiSHA = classificationPositiveAPISHA
	}
	a.sha256 = expectedSHA
	return a, data
}

func (a classificationAnchors) nativeContext() string {
	switch a.Route {
	case "go":
		return "full-input-utf8/r2"
	case "csharp":
		return semanticPagedContext
	default:
		return ""
	}
}

func (a classificationAnchors) nativeSymbol() string {
	switch a.Route {
	case "go":
		return "tree_sitter_go"
	case "csharp":
		return "tree_sitter_c_sharp"
	default:
		return ""
	}
}

func (a classificationAnchors) libraryName() string { return a.Route + ".dll" }

func classificationPlanContextMatches(a classificationAnchors, plan classificationPlan) bool {
	if a.Route == "csharp" {
		return a.sha256 == classificationCSharpAnchorsSHA && plan.NativeContext == semanticPagedContext
	}
	return a.Route == "go" && plan.NativeContext == ""
}

func classificationNativePrefix(a classificationAnchors) string {
	if a.Route == "csharp" {
		return "oracle/"
	}
	return "current/native/"
}

func classificationGrammarContext(s *eligibilitySession, a classificationAnchors, plan classificationPlan) error {
	if s.reg.Native == nil || s.reg.Context != a.nativeContext() || !classificationPlanContextMatches(a, plan) || len(s.reg.Native.Grammar) != len(plan.Grammar) {
		return fmt.Errorf("classification grammar context differs")
	}
	for _, g := range plan.Grammar {
		pin, exists := s.reg.Native.Grammar[g.Path]
		inv := s.invocations[s.reg.Native.InvocationID]
		if !exists || inv == nil || pin.NodeID != inv.Node.ID {
			return fmt.Errorf("classification grammar owner differs: %s", g.Path)
		}
		if a.nativeContext() == semanticPagedContext && g.Role == "parser" {
			if pin.Member.Role != "grammar-paged-descriptor" {
				return fmt.Errorf("classification parser descriptor missing")
			}
			data, ok, err := s.member(&pin)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("guarded parser descriptor absent")
			}
			var d semanticParserDescriptor
			if err := s.b.decode("classification/parser-descriptor", data, &d, true); err != nil {
				return err
			}
			if d.Schema != "tsgk-query-parser-pages/r1" || d.Owner != "D" || d.Logical.Path != g.Path || d.Logical.Role != "grammar-parser" || d.Logical.Bytes != g.Bytes || d.Logical.SHA256 != g.SHA256 {
				return fmt.Errorf("classification logical parser pin differs")
			}
			// Eligibility already validated descriptor ownership, all parts, and the logical hash in this snapshot.
		} else if pin.Member.Bytes != g.Bytes || pin.Member.SHA256 != g.SHA256 || pin.Member.Role != "grammar-"+g.Role {
			return fmt.Errorf("classification grammar pin differs: %s", g.Path)
		}
	}
	return nil
}

func classificationGoRegistration(a classificationAnchors) QueryWitnessRegistration {
	w := QueryWitnessRegistration{Schema: queryEligibilityWitnessSchema, Policy: "tsgk-predicates/r1", Case: a.Case, Route: a.Route, Platform: "windows/amd64", Role: "owned", Encoding: EncodingUTF8, Input: NativeInput{Path: "source.bin", Role: "case", Bytes: a.Source.Bytes, SHA256: a.Source.SHA256}, Edits: []Edit{}, Queries: []QueryWitnessQuery{}, Required: a.Required}
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
	anchors, data := classificationGoAnchors(t)
	plan := classificationPlan{Schema: "tsgk-query-classification-plan/r1", SourceRevision: os.Getenv("TSGK_QUERY_CLASSIFICATION_SOURCE"), RunID: os.Getenv("TSGK_QUERY_CLASSIFICATION_RUN"), AnchorsSHA256: digestHex(data), APIAnchorsSHA256: anchors.apiSHA, DonorRegistrationSHA256: classificationDonorRegistrationSHA}
	if anchors.Route == "csharp" {
		plan.NativeContext = anchors.nativeContext()
	}
	plan.CompilerSHA256 = os.Getenv("TSGK_QUERY_CLASSIFICATION_COMPILER_SHA256")
	if err := jsonv2.Unmarshal([]byte(os.Getenv("TSGK_QUERY_CLASSIFICATION_LIBRARY_REQUEST")), &plan.LibraryRequest, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal("preregistered compiler request", err)
	}
	if !hex64.MatchString(plan.CompilerSHA256) || eligibilityValidateRequest(plan.LibraryRequest, "go-library") != nil || !eligibilityAbsolutePath(plan.LibraryRequest.Path, "windows/amd64") || plan.LibraryRequest.Dir != root || !plan.LibraryRequest.Memory.Hard || plan.LibraryRequest.Memory.Bytes != 4<<30 || plan.LibraryRequest.Wall != 120e9 || plan.LibraryRequest.Grace != 2e9 || plan.LibraryRequest.StdoutBytes != 16<<20 || plan.LibraryRequest.StderrBytes != 4<<20 {
		t.Fatal("compiler preregistration identity or limits")
	}
	grammarRoot := filepath.Join(os.Getenv("TSGK_BENCH_PREPARED"), "routes", anchors.Route)
	if !filepath.IsAbs(grammarRoot) || !reflect.DeepEqual(plan.LibraryRequest.Args, []string{"-shared", "-O0", "-std=gnu11", "-I", filepath.Join(grammarRoot, "src"), filepath.Join(grammarRoot, "src", "parser.c"), filepath.Join(grammarRoot, "src", "scanner.c"), "-o", filepath.Join(root, "library", anchors.libraryName())}) {
		t.Fatal("compiler grammar inputs or output changed")
	}
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
		if row.Route == anchors.Route {
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
	t.Logf("%s preregistration only; no compiler, native, reference or approval", anchors.Case)
}

func classificationGoAssemble(t *testing.T, root, donor string) {
	if !filepath.IsAbs(donor) {
		t.Fatal("absolute caller-selected static donor required")
	}
	anchors, _ := classificationGoAnchors(t)
	plan := classificationGoReadPlan(t, root)
	if plan.Schema != "tsgk-query-classification-plan/r1" || plan.AnchorsSHA256 != anchors.sha256 || plan.APIAnchorsSHA256 != anchors.apiSHA || plan.DonorRegistrationSHA256 != classificationDonorRegistrationSHA || !classificationPlanContextMatches(anchors, plan) {
		t.Fatal("preregistration changed")
	}
	var captured struct {
		Passed         bool   `json:"passed"`
		SourceRevision string `json:"source_revision"`
		RunID          string `json:"run_id"`
		PlanSHA256     string `json:"plan_sha256"`
	}
	actualJSONDecode(t, filepath.Join(root, "capture-final.json"), &captured)
	if !captured.Passed || captured.SourceRevision != plan.SourceRevision || captured.RunID != plan.RunID || captured.PlanSHA256 != os.Getenv("TSGK_QUERY_CLASSIFICATION_PLAN_SHA256") {
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
	a := &actualJSONAssembly{t: t, evidence: filepath.Join(assembly, "evidence"), workload: anchors.Workload, nodes: map[string]EvidenceNode{}, cohortSource: map[string]string{"current": plan.SourceRevision},
		reg: eligibilityRegistration{Schema: QueryReferenceEligibilityRegistrationSchema, Context: anchors.nativeContext(), CatalogueSHA256: old.CatalogueSHA256, Build: old.Build, Control: old.Control, Tools: []eligibilityTool{}, Invocations: []eligibilityInvocation{}, OwnedMembers: []eligibilityMember{}, Observations: []eligibilityObservation{}}}
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
	policyID := "actual-go3-classification"
	if anchors.DriverMode == "batch" || anchors.Route == "csharp" {
		policyID = "actual-" + anchors.Workload
	}
	policy := eligibilityJSON(t, map[string]any{"schema": EvidencePolicySchema, "id": policyID, "evidence_sha256": digestHex(data), "required": required, "carry_forward": []any{}})
	actualJSONWrite(t, filepath.Join(assembly, "caller", "policy.json"), policy)
	actualJSONWrite(t, filepath.Join(assembly, "caller", "eligibility-registration.json"), eligibilityJSON(t, a.reg))
	actualJSONWrite(t, filepath.Join(assembly, "caller", "static-donor.json"), eligibilityJSON(t, map[string]any{"registration_sha256": digestHex(oldData), "graph_sha256": old.Graph.SHA256, "cohorts": "original build/control identities retained; new graph requires new outside approval"}))
	t.Logf("assembly_root=%q; no caller approval issued", assembly)
}

func classificationGoCurrent(t *testing.T, a *actualJSONAssembly, root string, anchors classificationAnchors, plan classificationPlan) {
	nativeRoot := filepath.Join(root, "native")
	var captured struct {
		Route          string         `json:"route"`
		Definitions    uint64         `json:"structural_definitions"`
		SemanticPass   bool           `json:"semantic_pass"`
		Schema         string         `json:"schema"`
		AnchorsSHA256  string         `json:"anchors_sha256"`
		BuildIdentity  string         `json:"build_identity"`
		ExecutablePath string         `json:"executable_path"`
		Files          []ReplayMember `json:"files"`
		RawSHA256      string         `json:"raw_sha256"`
	}
	actualJSONDecode(t, filepath.Join(nativeRoot, "native-complete.json"), &captured)
	if captured.Schema != "tsgk-query-classification-native-capture/r1" || captured.AnchorsSHA256 != anchors.sha256 || captured.Route != anchors.Route || captured.Definitions != uint64(len(anchors.Queries)) || captured.SemanticPass {
		t.Fatal("native capture not bound to preregistration")
	}
	var capture semanticProcessCapture
	actualJSONDecode(t, filepath.Join(nativeRoot, "capture", anchors.Case+".json"), &capture)
	if capture.Spec == nil || capture.Spec.Interactive || capture.Error != nil || capture.Stage != "RUN_RETURNED" || capture.Process == nil || capture.Stdout == nil || capture.Spec.Stdin == nil {
		t.Fatal("native process capture incomplete")
	}
	process, err := classificationGoProcessSpec(*capture.Spec)
	if err != nil {
		t.Fatal(err)
	}
	queries := make([]OracleQuery, 0, len(anchors.Queries))
	for _, q := range anchors.Queries {
		queries = append(queries, OracleQuery{ID: q.ID, Source: q.Source})
	}
	n := a.node("native-"+anchors.Case, "current", digestHex(actualJSONRead(t, filepath.Join(nativeRoot, "capture", "driver.bin"))), anchors.Source.SHA256, digestHex(eligibilityJSON(t, queries)), "tsgk-native/r4", "native-"+anchors.Case, StatusCompleted, AssessPass)
	n.Identities["policy"] = NativePolicy(NativeOperations()[anchors.Operation], OutputTree, nil).SHA256
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
		pins[pin.Path] = a.add(n.ID, classificationNativePrefix(anchors)+pin.Path, pin.Role, data)
	}
	raw := actualJSONRead(t, filepath.Join(nativeRoot, "raw", anchors.Case+".json"))
	if digestHex(raw) != captured.RawSHA256 {
		t.Fatal("raw capture changed")
	}
	response := a.add(n.ID, "current/native/raw.json", "raw", raw)
	source := a.copy(n.ID, "current/input/source.bin", "reference-source", filepath.Join(nativeRoot, "input", "source.bin"))
	if source.Member.Bytes != anchors.Source.Bytes || source.Member.SHA256 != anchors.Source.SHA256 {
		t.Fatal("source changed")
	}
	executable := pins["capture/driver.bin"]
	expected := eligibilityNativeExpected{Protocol: "tsgk-native/r4", RequestID: anchors.Case, NodeLimit: NativeOperations()[anchors.Operation].Nodes, API: anchors.API, Points: []uint32{}, RuntimeManifestSHA256: plan.RuntimeManifestSHA256, ProducerSHA256: captured.BuildIdentity, ExecutableSHA256: executable.Member.SHA256, ProducerABI: anchors.ABI, ObservationRunID: plan.RunID, HistoryStart: 0, Process: process}
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
			name := fmt.Sprintf("capture/build-input-%05d.bin", i)
			if anchors.nativeContext() == semanticPagedContext && row.Role == "grammar-parser" {
				name = fmt.Sprintf("capture/build-input-%05d.pages.json", i)
			}
			pin, exists := pins[name]
			if !exists {
				t.Fatal("native grammar capture member absent", name)
			}
			grammar[row.Path] = pin
		}
	}
	requestData := actualJSONRead(t, filepath.Join(root, "library", "request.json"))
	receiptData := actualJSONRead(t, filepath.Join(root, "library", "receipt.json"))
	compilerData := actualJSONRead(t, plan.LibraryRequest.Path)
	libraryData := actualJSONRead(t, filepath.Join(root, "library", anchors.libraryName()))
	if err := classificationGoLibrary(semanticTestBudget(t), plan, requestData, receiptData, compilerData, libraryData); err != nil {
		t.Fatal("library compiler capture", err)
	}
	a.node("go-library-build", "current", plan.CompilerSHA256, digestHex(eligibilityJSON(t, plan.Grammar)), "NOT_APPLICABLE", "compiler-capture/r1", "go-library", StatusCompleted, AssessPass)
	library := a.add("go-library-build", "current/go.dll", "runtime-library", libraryData)
	a.add("go-library-build", "current/library-request.json", "compiler-request", requestData)
	a.add("go-library-build", "current/library-receipt.json", "compiler-receipt", receiptData)
	a.add("go-library-build", "current/library-compiler.bin", "compiler", compilerData)
	a.reg.Native = &eligibilityNative{InvocationID: n.ID, Source: source, BuildManifest: pins["capture/build-manifest.txt"], ReferenceLibrary: &library, Symbol: anchors.nativeSymbol(), Grammar: grammar}
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
		if len(spec.Args) != 6 || spec.Args[1] != anchors.nativeSymbol() || spec.Args[4] != q.ID || spec.Args[5] != library.Member.SHA256 {
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
		a.reg.Observations = append(a.reg.Observations, eligibilityObservation{Query: q.ID, Step: 0, TreeRole: "incremental", SourceBytes: anchors.Source.Bytes, SourceSHA256: anchors.Source.SHA256, InvocationID: id, QueryID: q.ID, ResponseOrdinal: 0, ByteOffset: 0, ByteLength: uint64(len(stdout)), SHA256: digestHex(stdout)})
	}
	actualJSONWrite(t, filepath.Join(filepath.Dir(a.evidence), "caller", "witness.json"), eligibilityJSON(t, witness))
}

func classificationGoVerify(t *testing.T, root, assembly, approvalPath string) {
	classificationGoVerifyWith(t, root, assembly, approvalPath, nil)
}

// consume runs while the original guarded run and snapshot are still alive.
func classificationGoVerifyWith(t *testing.T, root, assembly, approvalPath string, consume func(classificationAnchors, classificationPlan, *eligibilitySession, *semanticNativeCache, QueryReferenceEligibilityResult)) {
	for _, p := range []string{assembly, approvalPath} {
		if !filepath.IsAbs(p) {
			t.Fatal("absolute caller approval/assembly required")
		}
	}
	anchors, _ := classificationGoAnchors(t)
	plan := classificationGoReadPlan(t, root)
	if plan.Schema != "tsgk-query-classification-plan/r1" || plan.AnchorsSHA256 != anchors.sha256 || plan.APIAnchorsSHA256 != anchors.apiSHA || !classificationPlanContextMatches(anchors, plan) {
		t.Fatal("classification preregistration changed")
	}
	caller := filepath.Join(assembly, "caller")
	req := QueryReferenceEligibilityRequest{Evidence: EvidenceRequest{Root: filepath.Join(assembly, "evidence"), Policy: actualJSONRead(t, filepath.Join(caller, "policy.json"))}, Approval: actualJSONRead(t, approvalPath), Registration: actualJSONRead(t, filepath.Join(caller, "eligibility-registration.json")), WitnessRegistration: actualJSONRead(t, filepath.Join(caller, "witness.json"))}
	ctx, cancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	defer cancel()
	r, e := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		t.Fatal(e)
	}
	defer r.cancel()
	cache := &semanticNativeCache{}
	scope := &eligibilityScope{r: r, native: cache}
	core, err := verifyQueryReferenceEligibilityInRun(r.caller, req, scope)
	if err != nil || core.Assessment != AssessPass || !core.Complete || core.Tuple != (QueryEligibilityCounts{Required: uint64(len(anchors.Queries)), Verified: uint64(len(anchors.Queries))}) {
		t.Fatal("eligibility not complete", err, core.Findings)
	}
	if !cache.ready || len(cache.checked) != 1 || CompareTrees(cache.checked[0].Incremental.Nodes, anchors.Nodes) != nil {
		t.Fatal("guarded native independent CST")
	}
	s := scope.session
	if !classificationGoWitnessMatches(anchors, s.witness.QueryWitnessRegistration) || s.witness.Native.ProducerABI != anchors.ABI || s.witness.Native.ObservationRunID != plan.RunID || s.witness.Native.RuntimeManifestSHA256 != plan.RuntimeManifestSHA256 || s.approval.Candidate.SourceRevision != plan.SourceRevision || s.approval.Candidate.Platform != "windows/amd64" || s.approval.Candidate.Run != plan.RunID || s.approval.Candidate.Attempt != 1 || s.reg.Native == nil || s.reg.Native.Symbol != anchors.nativeSymbol() || s.reg.Context != anchors.nativeContext() || len(s.reg.Native.Grammar) != len(plan.Grammar) {
		t.Fatal("classification of an unrelated approved workload")
	}
	if err := classificationGrammarContext(s, anchors, plan); err != nil {
		t.Fatal(err)
	}
	if err := classificationGoFrames(s, anchors, plan); err != nil {
		t.Fatal(err)
	}
	if !classificationGoLibraryNodeMatches(s.nodes["go-library-build"], plan, anchors.Workload) {
		t.Fatal("guarded library node contradicts preregistration or compiler receipt")
	}
	if !classificationLibraryMatches(s) {
		t.Fatal("reference library differs from the guarded compiler output")
	}
	readLibrary := func(path string) []byte {
		t.Helper()
		pin, exists := s.owned[path]
		data, ok, err := s.member(&pin)
		if !exists || pin.NodeID != "go-library-build" || !ok || err != nil {
			t.Fatal("guarded library member", path, err)
		}
		return data
	}
	if err := classificationGoLibrary(s.b, plan, readLibrary("current/library-request.json"), readLibrary("current/library-receipt.json"), readLibrary("current/library-compiler.bin"), readLibrary("current/go.dll")); err != nil {
		t.Fatal("guarded library compiler capture", err)
	}
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
	publicCtx, publicCancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	defer publicCancel()
	public, err := VerifyQueryReferenceEligibility(publicCtx, req)
	if err != nil || public.Assessment != AssessPass || !public.Complete || public.Tuple != core.Tuple || public.Build != core.Build || public.Unit != core.Unit || public.Control != core.Control || public.Assertion != core.Assertion || public.WitnessSHA256 != core.WitnessSHA256 || public.RegistrationSHA256 != core.RegistrationSHA256 || public.GraphSHA256 != core.GraphSHA256 || public.ApprovalSHA256 != core.ApprovalSHA256 || public.CatalogueSHA256 != core.CatalogueSHA256 || public.ResultSchema != core.ResultSchema || public.ExecutionStatus != core.ExecutionStatus {
		t.Fatal("public eligibility parity", err)
	}
	if consume != nil {
		if err := classificationGoDefinitionDuring(r.caller, func() { consume(anchors, plan, s, cache, core) }); err != nil {
			t.Fatal("guarded definition run expired", err)
		}
		return
	}
	rows := []map[string]any{}
	for _, q := range anchors.Queries {
		rows = append(rows, map[string]any{"route": anchors.Route, "role": "owned", "platform": "windows/amd64", "query": q.ID, "query_sha256": q.SHA256, "class": "STRUCTURAL_ONLY", "assessment": AssessPass, "semantic_evaluated": false})
	}
	actualJSONWrite(t, filepath.Join(caller, "classification-result.json"), eligibilityJSON(t, map[string]any{"schema": "tsgk-query-classification-local-result/r1", "assessment": AssessPass, "required": len(anchors.Queries), "verified": len(anchors.Queries), "complete": true, "anchors_sha256": anchors.sha256, "registration_sha256": core.RegistrationSHA256, "witness_sha256": core.WitnessSHA256, "approval_sha256": core.ApprovalSHA256, "graph_sha256": core.GraphSHA256, "definitions": rows, "eligibility": public, "independent_native_reference_cst": "PASS", "scope": "caller-approved conditional definition classification; not semantic EVALUATED or inventory completion"}))
	t.Logf("guarded independent CST and public eligibility%d verified; conditional %s classification only", len(anchors.Queries), anchors.Case)
}

func classificationGoDefinitionDuring(ctx context.Context, project func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	project()
	return ctx.Err()
}

func classificationGoDefinitionPublish(t *testing.T, output string, produce func() ([]byte, error)) error {
	t.Helper()
	data, err := produce()
	if err != nil {
		return err
	}
	actualJSONWrite(t, output, data)
	return nil
}

func classificationGoDefinitionOutput(root, assembly, output string) error {
	if !filepath.IsAbs(output) {
		return fmt.Errorf("absolute separate definition output required")
	}
	real, err := filepath.EvalSymlinks(output)
	if err != nil || !strings.EqualFold(filepath.Clean(real), filepath.Clean(output)) {
		return fmt.Errorf("definition output must be a physical caller-owned directory")
	}
	entries, err := os.ReadDir(output)
	if err != nil || len(entries) != 0 {
		return fmt.Errorf("definition output must be new and empty")
	}
	for _, input := range []string{root, assembly} {
		physical, err := filepath.EvalSymlinks(input)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(physical, real)
		if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return fmt.Errorf("definition output overlaps retained observation inputs")
		}
	}
	return nil
}

func classificationGoVerifyDefinitions(t *testing.T, root, assembly, approvalPath string) {
	t.Helper()
	output := os.Getenv("TSGK_QUERY_CLASSIFICATION_DEFINITION_OUTPUT")
	reader := os.Getenv("TSGK_QUERY_CLASSIFICATION_READER_REVISION")
	profile := os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE")
	if (profile != "positive-batch" && profile != "csharp15-empty") || !hex40.MatchString(reader) {
		t.Fatal("definition projection requires a fixed reviewed profile and separately pinned reader revision")
	}
	if err := classificationGoDefinitionOutput(root, assembly, output); err != nil {
		t.Fatal(err)
	}
	var definitions, links uint64
	err := classificationGoDefinitionPublish(t, filepath.Join(output, "definition-classification-result.json"), func() ([]byte, error) {
		var result []byte
		classificationGoVerifyWith(t, root, assembly, approvalPath, func(anchors classificationAnchors, plan classificationPlan, s *eligibilitySession, cache *semanticNativeCache, core QueryReferenceEligibilityResult) {
			inventoryData := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
			qualificationData := actualJSONRead(t, "../contracts/qualification-c1.json")
			var record classificationGoDefinitionProjectionResult
			var err error
			if anchors.Route == "csharp" {
				inputs := classificationCSharpDefinitionQueryInputs{QueryFile: actualJSONRead(t, "../testdata/native/queries/csharp.json"), FactQueryPack: actualJSONRead(t, "../contracts/fact-query-pack.json")}
				queryData, encodeErr := jsonv2.Marshal(inputs)
				if encodeErr != nil {
					t.Fatal(encodeErr)
				}
				record, err = classificationCSharpDefinitionProjection(anchors, plan, s, cache, core, inventoryData, qualificationData, queryData, reader)
			} else {
				record, err = classificationGoDefinitionProjection(anchors, plan, s, cache, core, inventoryData, qualificationData, actualJSONRead(t, "../testdata/native/queries/go.json"), reader)
			}
			if err != nil {
				t.Fatal("guarded definition projection", err)
			}
			definitions, links = record.Inventory.Definitions, record.Inventory.PotentialQueryTreeRoles
			result = eligibilityJSON(t, record)
		})
		return result, nil
	})
	if err != nil {
		t.Fatal("definition publication", err)
	}
	t.Logf("same guarded snapshot: %d definitions classified, %d potential links; no case or semantic completion", definitions, links)
}

func classificationGoPlanMatches(data []byte, expectedSHA256 string) bool {
	return hex64.MatchString(expectedSHA256) && digestHex(data) == expectedSHA256
}

func classificationGoReadPlan(t *testing.T, root string) classificationPlan {
	t.Helper()
	data := actualJSONRead(t, filepath.Join(root, "plan", "preregistration.json"))
	if !classificationGoPlanMatches(data, os.Getenv("TSGK_QUERY_CLASSIFICATION_PLAN_SHA256")) {
		t.Fatal("caller must supply the plan digest independently frozen before query capture")
	}
	var plan classificationPlan
	if err := jsonv2.Unmarshal(data, &plan, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	return plan
}

func classificationGoLibraryNodeMatches(node EvidenceNode, plan classificationPlan, workload string) bool {
	grammar, err := jsonv2.Marshal(plan.Grammar)
	if err != nil {
		return false
	}
	identities := map[string]string{"source": plan.SourceRevision, "tool": plan.CompilerSHA256, "input": digestHex(grammar), "query": "NOT_APPLICABLE", "policy": digestHex([]byte(eligibilityCatalogueJSON)), "comparator": CaptureCompareScheme, "protocol": "compiler-capture/r1", "workload": workload, "platform": "windows/amd64", "run": "go-library", "attempt": "1"}
	return node.ID == "go-library-build" && node.Kind == "run" && node.ExecutionStatus == StatusCompleted && node.Assessment == AssessPass && node.EvidenceMode == ModeNewRun && reflect.DeepEqual(node.Identities, identities)
}

func TestQueryClassificationFrozenBindings(t *testing.T) {
	plan := classificationPlan{SourceRevision: strings.Repeat("a", 40), CompilerSHA256: strings.Repeat("b", 64), Grammar: []NativeInput{{Path: "src/parser.c", Role: "parser", Bytes: 1, SHA256: digestHex([]byte("p"))}}}
	plan.LibraryRequest.Wall = 120e9
	original := eligibilityJSON(t, plan)
	frozenSHA := digestHex(original)
	if !classificationGoPlanMatches(original, frozenSHA) || classificationGoPlanMatches(original, "") {
		t.Fatal("independent preregistration pin")
	}
	changed := plan
	changed.LibraryRequest.Wall++
	// Matching a changed request to a changed plan cannot replace the pre-query digest.
	if classificationGoPlanMatches(eligibilityJSON(t, changed), frozenSHA) {
		t.Fatal("jointly changed plan and compiler request accepted")
	}
	a := &actualJSONAssembly{workload: "go3-definition-classification", cohortSource: map[string]string{"current": plan.SourceRevision}, nodes: map[string]EvidenceNode{}}
	node := a.node("go-library-build", "current", plan.CompilerSHA256, digestHex(eligibilityJSON(t, plan.Grammar)), "NOT_APPLICABLE", "compiler-capture/r1", "go-library", StatusCompleted, AssessPass)
	if !classificationGoLibraryNodeMatches(node, plan, a.workload) {
		t.Fatal("completed preregistered library node rejected")
	}
	for _, change := range []func(*EvidenceNode){
		func(n *EvidenceNode) { n.ExecutionStatus = StatusFailed; n.Assessment = AssessFail },
		func(n *EvidenceNode) { n.Identities["tool"] = strings.Repeat("c", 64) },
		func(n *EvidenceNode) { n.Identities["input"] = strings.Repeat("c", 64) },
		func(n *EvidenceNode) { n.Identities["source"] = strings.Repeat("c", 40) },
		func(n *EvidenceNode) { n.Identities["run"] = "another-library" },
	} {
		var other EvidenceNode
		if err := jsonv2.Unmarshal(eligibilityJSON(t, node), &other); err != nil {
			t.Fatal(err)
		}
		change(&other)
		graph := eligibilityJSON(t, map[string]any{"schema": EvidenceSchema, "nodes": []EvidenceNode{other}})
		policy := eligibilityJSON(t, map[string]any{"schema": EvidencePolicySchema, "id": "rebound-library", "evidence_sha256": digestHex(graph), "required": []map[string]any{{"node": other.ID, "kind": other.Kind, "identities": other.Identities}}, "carry_forward": []any{}})
		root := t.TempDir()
		actualJSONWrite(t, filepath.Join(root, "evidence.json"), graph)
		result, err := VerifyEvidence(ctxT(t), EvidenceRequest{Root: root, Policy: policy})
		if err != nil || result.Assessment != AssessPass {
			t.Fatal("rebound graph must pass integrity before the classification guard", err, result.Findings)
		}
		if classificationGoLibraryNodeMatches(other, plan, a.workload) {
			t.Fatal("rehashed graph/policy accepted a contradictory library node")
		}
	}
}

func classificationGoRequestMatches(request semanticNativeRequest, args []string, a classificationAnchors, plan classificationPlan, producer string) bool {
	if request.Protocol != "tsgk-native/r4" || request.ID != a.Case || !slices.Equal(args, []string{a.DriverMode}) || request.Encoding != EncodingUTF8 || request.Output != OutputTree || request.Limits != semanticLimitsFor(NativeOperations()[a.Operation]) || request.API != a.API || !bytes.Equal(request.source, []byte(a.Source.UTF8)) || len(request.edits) != 0 || len(request.Points) != 0 || len(request.Declarations) != 0 || len(request.Ranges) != 0 || len(request.queries) != len(a.Queries) || request.ReuseObservation != reuseobs.Policy || request.ObservationRunID != plan.RunID || request.ObservationProducerSHA256 != producer || request.ObservationHistoryStart != 0 {
		return false
	}
	for i, q := range a.Queries {
		if request.queries[i].ID != q.ID || !bytes.Equal(request.queries[i].Source, []byte(q.Source)) {
			return false
		}
	}
	return true
}

func classificationGoNativeNodeMatches(node EvidenceNode, a classificationAnchors, plan classificationPlan, executableSHA string) bool {
	queries := make([]OracleQuery, 0, len(a.Queries))
	for _, q := range a.Queries {
		queries = append(queries, OracleQuery{ID: q.ID, Source: q.Source})
	}
	data, err := jsonv2.Marshal(queries)
	if err != nil {
		return false
	}
	id := "native-" + a.Case
	identities := map[string]string{"source": plan.SourceRevision, "tool": executableSHA, "input": a.Source.SHA256, "query": digestHex(data), "policy": NativePolicy(NativeOperations()[a.Operation], OutputTree, nil).SHA256, "comparator": CaptureCompareScheme, "protocol": "tsgk-native/r4", "workload": a.Workload, "platform": "windows/amd64", "run": id, "attempt": "1"}
	return node.ID == id && node.Kind == "run" && node.ExecutionStatus == StatusCompleted && node.Assessment == AssessPass && node.EvidenceMode == ModeNewRun && reflect.DeepEqual(node.Identities, identities)
}

func classificationLibraryMatches(s *eligibilitySession) bool {
	if s == nil || s.reg.Native == nil || s.reg.Native.ReferenceLibrary == nil {
		return false
	}
	library, exists := s.owned["current/go.dll"]
	return exists && library.NodeID == "go-library-build" && library.Member.Role == "runtime-library" && *s.reg.Native.ReferenceLibrary == library
}

func classificationCaptureStderr(a classificationAnchors, capture semanticProcessCapture, process eligibilityExecution, read func(*ReplayMember) ([]byte, error)) error {
	if capture.Stderr == nil {
		if a.Route == "csharp" || process.StderrBytes != 0 {
			return fmt.Errorf("guarded stderr member missing")
		}
		return nil
	}
	if capture.Stderr.Role != "stderr" || capture.Stderr.Bytes != process.StderrBytes {
		return fmt.Errorf("guarded stderr pin differs from process byte count")
	}
	data, err := read(capture.Stderr)
	if err != nil || uint64(len(data)) != process.StderrBytes {
		return fmt.Errorf("guarded stderr member differs from process: %v", err)
	}
	return nil
}

func classificationNativeInvocation(s *eligibilitySession, a classificationAnchors) (*eligibilityInvocation, error) {
	id := "native-" + a.Case
	if s == nil || s.reg.Native == nil || s.reg.Native.InvocationID != id {
		return nil, fmt.Errorf("guarded native invocation differs from profile")
	}
	inv := s.invocations[id]
	if inv == nil || inv.ID != id || inv.Node.ID != id || inv.Kind != "NATIVE_DRIVER" {
		return nil, fmt.Errorf("guarded native invocation differs from profile")
	}
	return inv, nil
}

func classificationGoFrames(s *eligibilitySession, a classificationAnchors, plan classificationPlan) error {
	label, id := a.Case, "native-"+a.Case
	inv, err := classificationNativeInvocation(s, a)
	if err != nil {
		return err
	}
	w, op := s.witness.Native, NativeOperations()[a.Operation]
	if w.Protocol != "tsgk-native/r4" || w.RequestID != a.Case || w.API != a.API || w.NodeLimit != op.Nodes || len(w.Points) != 0 || w.HistoryStart != 0 {
		return fmt.Errorf("guarded native expectation differs from profile")
	}
	if s.semantic == nil || len(s.semantic.checked) != 1 || !reflect.DeepEqual(s.semantic.checked[0].Incremental.API, a.expectedAPI) {
		return fmt.Errorf("guarded native API differs from independent expectations")
	}
	if !classificationGoNativeNodeMatches(s.nodes[id], a, plan, w.ExecutableSHA256) {
		return fmt.Errorf("guarded native node differs from profile")
	}
	path := classificationNativePrefix(a) + "capture/" + label + ".json"
	pin, exists := s.owned[path]
	if !exists || pin.NodeID != id || pin.Member.Role != "capture" {
		return fmt.Errorf("guarded process capture owner differs")
	}
	captureData, ok, memberErr := s.member(&pin)
	if memberErr != nil || !ok {
		return fmt.Errorf("guarded process capture absent: %v", memberErr)
	}
	capture, e := s.b.processCapture(label+"/capture", captureData)
	if e != nil || capture.Spec == nil || capture.Spec.Interactive || capture.Spec.Stdin == nil || capture.Process == nil || capture.Stdout == nil || capture.Error != nil {
		return fmt.Errorf("guarded process capture: %v", e)
	}
	read := func(pin *ReplayMember) ([]byte, error) {
		mapped := *pin
		mapped.Path = classificationNativePrefix(a) + mapped.Path
		data, ok, e := s.member(&eligibilityMember{NodeID: "native-" + a.Case, Member: mapped})
		if e != nil || !ok {
			return nil, fmt.Errorf("guarded frame member %s: %v", mapped.Path, e)
		}
		return data, nil
	}
	stdin, err := read(capture.Spec.Stdin)
	if err != nil {
		return err
	}
	request, e := s.b.nativeRequestFrame(label+"/stdin", stdin)
	if e != nil {
		return e
	}
	if !classificationGoRequestMatches(request, capture.Spec.Args, a, plan, s.witness.Native.ProducerSHA256) {
		return fmt.Errorf("captured request differs from preregistration")
	}
	spec, err := classificationGoProcessSpec(*capture.Spec)
	if err != nil {
		return err
	}
	if spec.Memory.Bytes != op.MemoryBytes || !spec.Memory.Hard || spec.Wall != uint64(op.ParseWall) || spec.Grace != 5e9 || spec.StdoutBytes != op.OutputBytes+4 || spec.StderrBytes != 65536 || len(spec.Env) != 0 || spec.CgroupParent != "" || !reflect.DeepEqual(spec, s.witness.Native.Process) {
		return fmt.Errorf("actual process spec differs from approval")
	}
	stdoutFrame, err := read(capture.Stdout)
	if err != nil {
		return err
	}
	stdout, e := s.b.semanticFrame(label+"/stdout", stdoutFrame)
	if e != nil {
		return e
	}
	if inv.Response == nil || inv.Process == nil {
		return fmt.Errorf("native invocation absent")
	}
	raw, ok, e := s.member(inv.Response)
	if e != nil || !ok || !bytes.Equal(stdout, raw) {
		return fmt.Errorf("guarded stdout differs from native raw: %v", e)
	}
	capturedProcess, err := read(capture.Process)
	if err != nil {
		return err
	}
	process, ok, e := s.member(inv.Process)
	if e != nil || !ok || !bytes.Equal(process, capturedProcess) {
		return fmt.Errorf("guarded process differs from invocation: %v", e)
	}
	return classificationCaptureStderr(a, capture, s.semantic.record.process, read)
}

func classificationGoProcessSpec(capture semanticCaptureSpec) (eligibilityProcessRequest, error) {
	var spec eligibilityProcessRequest
	if capture.Interactive {
		return spec, fmt.Errorf("classification requires a noninteractive native process")
	}
	data, err := jsonv2.Marshal(capture)
	if err != nil {
		return spec, err
	}
	if err := jsonv2.Unmarshal(data, &spec); err != nil {
		return spec, err
	}
	if spec.Env == nil {
		spec.Env = []string{}
	}
	return spec, nil
}

func TestQueryClassificationProcessSpec(t *testing.T) {
	capture := semanticCaptureSpec{Path: "C:/driver.exe", Args: []string{"single"}}
	spec, err := classificationGoProcessSpec(capture)
	if err != nil || spec.Path != capture.Path || !reflect.DeepEqual(spec.Args, capture.Args) || spec.Env == nil {
		t.Fatal("noninteractive spec projection", err)
	}
	capture.Interactive = true
	if _, err := classificationGoProcessSpec(capture); err == nil {
		t.Fatal("Interactive was lost by the process request projection")
	}
}

func classificationGoLibrary(b *eligibilityBudget, plan classificationPlan, requestData, receiptData, compiler, library []byte) error {
	if err := b.charge(uint64(len(compiler)+len(library)), 0, "go-library/bytes"); err != nil {
		return err
	}
	var request eligibilityProcessRequest
	var receipt eligibilityReceipt
	if err := b.decode("go-library/request", requestData, &request, true); err != nil {
		return err
	}
	if err := b.decode("go-library/receipt", receiptData, &receipt, true); err != nil {
		return err
	}
	if eligibilityValidateRequest(request, "go-library") != nil || !reflect.DeepEqual(request, plan.LibraryRequest) || !hex64.MatchString(plan.CompilerSHA256) || digestHex(compiler) != plan.CompilerSHA256 || len(library) == 0 || len(library) > MaxDocumentBytes || receipt.Schema != "tsgk-reference-process/r1" {
		return fmt.Errorf("library request, compiler or output differs from preregistration")
	}
	check := &eligibilitySession{res: &QueryReferenceEligibilityResult{}}
	if !check.execution("go-library", receipt.Execution, request, "windows/amd64", 0, false) {
		return fmt.Errorf("library compiler did not complete under verified limits/cleanup: %s", check.firstFailure)
	}
	for i, text := range []string{receipt.StdoutBase64, receipt.StderrBase64} {
		if err := b.charge(1, 4*uint64(len(text)), "go-library/streams"); err != nil {
			return err
		}
		raw, err := base64.StdEncoding.Strict().DecodeString(text)
		count, limit := receipt.Execution.StdoutBytes, request.StdoutBytes
		if i == 1 {
			count, limit = receipt.Execution.StderrBytes, request.StderrBytes
		}
		if err != nil || base64.StdEncoding.EncodeToString(raw) != text || uint64(len(raw)) != count || count > limit {
			return fmt.Errorf("library compiler stream differs from receipt")
		}
	}
	return nil
}

func TestQueryClassificationLibrary(t *testing.T) {
	request := eligibilityProcessRequest{Path: "C:/tools/cc.exe", Args: []string{"-shared", "-o", "C:/case/library/go.dll"}, Dir: "C:/case", Env: []string{}, Wall: 120e9, Grace: 2e9, StdoutBytes: 16 << 20, StderrBytes: 4 << 20}
	request.Memory.Bytes, request.Memory.Hard = 4<<30, true
	plan := classificationPlan{CompilerSHA256: digestHex([]byte("compiler")), LibraryRequest: request}
	x := eligibilityFixtureExecution(request, 0, false, nil, nil)
	x.Capabilities.OS, x.Capabilities.Backend = "windows/amd64", "windows-job-object"
	x.Cleanup.Method, x.Cleanup.Scope, x.Capabilities.TreeCleanup = "JOB_OBJECT", "JOB_OBJECT", "JOB_OBJECT"
	receipt := eligibilityReceipt{Schema: "tsgk-reference-process/r1", Execution: x}
	check := func(req eligibilityProcessRequest, rec eligibilityReceipt, compiler, library []byte) error {
		return classificationGoLibrary(semanticTestBudget(t), plan, eligibilityJSON(t, req), eligibilityJSON(t, rec), compiler, library)
	}
	if err := check(request, receipt, []byte("compiler"), []byte("library")); err != nil {
		t.Fatal(err)
	}
	changedPlan := plan
	changedPlan.LibraryRequest.Wall++
	if err := classificationGoLibrary(semanticTestBudget(t), changedPlan, eligibilityJSON(t, changedPlan.LibraryRequest), eligibilityJSON(t, receipt), []byte("compiler"), []byte("library")); err != nil {
		t.Fatal("joint mutation must remain internally consistent for this counterexample", err)
	}
	if classificationGoPlanMatches(eligibilityJSON(t, changedPlan), digestHex(eligibilityJSON(t, plan))) {
		t.Fatal("joint plan/request mutation replaced the independent pre-query pin")
	}
	for _, change := range []func(*eligibilityReceipt){
		func(r *eligibilityReceipt) { r.Execution.Status = StatusFailed },
		func(r *eligibilityReceipt) { r.Execution.ExitCode = 1 },
		func(r *eligibilityReceipt) { r.Execution.Cleanup.Verified = false },
		func(r *eligibilityReceipt) { r.Execution.Cleanup.ResidualAfterExit = 1 },
		func(r *eligibilityReceipt) { r.Execution.Memory.Enforcement = "SOFT" },
		func(r *eligibilityReceipt) { r.Execution.Memory.Limit-- },
		func(r *eligibilityReceipt) { r.StdoutBase64 = "YQ==" },
	} {
		other := receipt
		change(&other)
		if check(request, other, []byte("compiler"), []byte("library")) == nil {
			t.Fatal("failed, unclean or inconsistent compiler receipt accepted")
		}
	}
	for _, change := range []func(*eligibilityProcessRequest){
		func(r *eligibilityProcessRequest) { r.Path += ".other" },
		func(r *eligibilityProcessRequest) { r.Args = []string{"-o", "C:/outside/go.dll"} },
		func(r *eligibilityProcessRequest) { r.Memory.Hard = false },
		func(r *eligibilityProcessRequest) { r.Wall++ },
	} {
		other := request
		change(&other)
		if check(other, receipt, []byte("compiler"), []byte("library")) == nil {
			t.Fatal("unregistered compiler request accepted")
		}
	}
	if check(request, receipt, []byte("changed compiler"), []byte("library")) == nil || check(request, receipt, []byte("compiler"), nil) == nil {
		t.Fatal("changed compiler or empty output accepted")
	}
}
