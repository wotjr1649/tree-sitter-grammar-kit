package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

type ordinaryE2EReference struct {
	Case     string
	Step     uint32
	Query    string
	Response []byte
}

type ordinaryE2EProducerInput struct {
	Profile, PlanningProfile, AssetRegistration, ShardPlan []byte
	RegistrationBytes                                      []byte
	Registration                                           OrdinaryRegistration
	ShardIDs                                               []string
	Sources                                                [][]byte
	GrammarRoot                                            string
	FactPack, FactMapping                                  []byte
}

// ordinaryE2EAssemble binds a real owned r8 to a synthetic approval and caller-authored
// reference rows. The rows are stored fixtures; this helper never executes a reference.
func ordinaryE2EAssemble(t *testing.T, req OrdinaryQuerySemanticsRequest, producerDir string, references []ordinaryE2EReference, assetBodies map[string][]byte) OrdinaryQuerySemanticsRequest {
	t.Helper()
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	metadataBytes := read(filepath.Join(producerDir, "producer.json"))
	var produced struct {
		Profile    []byte `json:"profile"`
		RunBinding []byte `json:"run_binding"`
		RunID      string `json:"run_id"`
	}
	if err := json.Unmarshal(metadataBytes, &produced); err != nil {
		t.Fatal(err)
	}
	setDir := filepath.Join(producerDir, "r8")
	manifestBytes := read(filepath.Join(setDir, "manifest.json"))
	var manifest OrdinaryOracleManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != OracleOrdinaryManifestSchema || !manifest.Complete || manifest.RunID != produced.RunID || len(manifest.Cases) == 0 {
		t.Fatalf("owned producer did not write a complete ordinary r8: %+v", manifest)
	}
	if produced.Profile == nil || produced.RunBinding == nil || produced.RunID == "" {
		t.Fatal("ordinary producer metadata is incomplete")
	}
	profile, err := ParseOracleProfile(produced.Profile)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := ParseOrdinaryRegistration(req.Registration)
	if err != nil {
		t.Fatal(err)
	}
	assetRegistration, err := ParseOrdinaryAssets(req.AssetRegistration)
	if err != nil {
		t.Fatal(err)
	}
	if len(registration.PlannedMembers) != 1 || registration.PlannedMembers[0].Role != "reference-library" || registration.PlannedMembers[0].Owner != "D_build" {
		t.Fatalf("expected one planned synthetic reference library, got %+v", registration.PlannedMembers)
	}
	if assetRegistration.Candidate != manifest.Candidate || assetRegistration.SemanticCohort != manifest.SemanticCohort {
		t.Fatal("asset registration and actual producer cohort differ")
	}
	refLibraryBytes := []byte("synthetic reference library; never loaded")
	if registration.PlannedMembers[0].Pin.Bytes != uint64(len(refLibraryBytes)) || registration.PlannedMembers[0].Pin.SHA256 != digestHex(refLibraryBytes) {
		t.Fatal("planned synthetic reference library pin differs")
	}

	f := newEligibilityFixture(t)
	f.reg.Context, f.reg.Native, f.reg.Observations = OrdinaryContext, nil, []eligibilityObservation{}
	f.reg.Invocations = slices.DeleteFunc(f.reg.Invocations, func(inv eligibilityInvocation) bool { return inv.Kind == "NATIVE_DRIVER" })
	for _, member := range manifest.Members {
		data := read(filepath.Join(setDir, filepath.FromSlash(member.Path)))
		path := "oracle/" + member.Path
		m := f.member(member.Owner, path, member.Role, data)
		if m.Member.Bytes != member.Pin.Bytes || m.Member.SHA256 != member.Pin.SHA256 {
			t.Fatalf("producer member changed while assembling graph: %s", member.Path)
		}
	}
	f.member("O", "oracle/manifest.json", "manifest", manifestBytes)
	for _, asset := range assetRegistration.Assets {
		if asset.State != "PINNED" || asset.Pin == nil || asset.AuthoringBasis == nil || asset.IndependentReview == nil {
			continue
		}
		for _, item := range []struct {
			pin  OrdinaryPin
			role string
		}{{*asset.Pin, asset.Role}, {*asset.AuthoringBasis, "authoring-basis"}, {*asset.IndependentReview, "independent-review"}} {
			data, ok := assetBodies[item.pin.Path]
			if !ok || uint64(len(data)) != item.pin.Bytes || digestHex(data) != item.pin.SHA256 {
				t.Fatalf("expected asset fixture bytes do not match pin %s", item.pin.Path)
			}
			f.member("O", item.pin.Path, item.role, data)
		}
	}
	refLibrary := f.member("D_build", registration.PlannedMembers[0].Path, "reference-library", refLibraryBytes)
	compilerIdentity := f.member("D_build", "approval/ordinary/compiler-identity.json", "compiler", []byte("synthetic caller approval for the observed compiler"))
	f.reg.Tools = append(f.reg.Tools, eligibilityTool{ID: "ordinary-current-compiler", CohortID: "current", ToolName: profile.Native.Compiler.Name,
		PathLiteral: os.Getenv("TSGK_NATIVE_CC"), SubjectSHA256: profile.Native.Compiler.SHA256, IdentityMember: compilerIdentity, ProvenanceID: "p-current"})

	cases := map[string]OrdinaryCase{}
	for _, c := range registration.Cases {
		cases[c.ID] = c
	}
	queries := map[string]OrdinaryQuery{}
	for _, q := range registration.Queries {
		queries[q.ID] = q
	}
	byProfileQuery := map[string]OracleQuery{}
	for _, q := range profile.Queries {
		byProfileQuery[q.ID] = q
	}
	selection := ordinaryReferenceSelection{Schema: ordinaryReferenceSelectionSchema, Rows: []ordinaryReferenceSelectionRow{}}
	for i, ref := range references {
		c, ok := cases[ref.Case]
		if !ok || int(ref.Step) >= len(c.VersionPins) {
			t.Fatalf("synthetic reference names an unregistered source: %+v", ref)
		}
		_, queryOK := queries[ref.Query]
		profileQuery, profileOK := byProfileQuery[ref.Query]
		if !queryOK || !profileOK || ref.Response == nil {
			t.Fatalf("synthetic reference names an unregistered query or has no response: %+v", ref)
		}
		var response instrumentedReference
		if err := json.Unmarshal(ref.Response, &response); err != nil {
			t.Fatalf("synthetic reference is not one instrumented row: %v", err)
		}
		source := c.VersionPins[ref.Step]
		if response.Input.Encoding != EncodingUTF8 || response.Input.Bytes != source.Bytes || response.Input.SHA256 != source.SHA256 ||
			response.Query.ID != ref.Query || response.Query.SHA256 != digestHex([]byte(profileQuery.Source)) ||
			response.Grammar.LibrarySHA256 != refLibrary.Member.SHA256 || response.Grammar.Symbol != profile.Native.Symbol {
			t.Fatalf("synthetic reference does not bind the selected source/query: case=%s step=%d query=%s", ref.Case, ref.Step, ref.Query)
		}
		invocationID := fmt.Sprintf("ordinary-e2e-ref-%06d", i)
		row := bytesTrimOneLF(ref.Response)
		selection.Rows = append(selection.Rows, ordinaryReferenceSelectionRow{Case: ref.Case, Step: ref.Step, Query: ref.Query, Source: source,
			Invocation: invocationID, ResponseOrdinal: 0, ByteOffset: 0, ByteLength: uint64(len(row)), SHA256: digestHex(row)})

		libraryPath := `C:\ordinary\reference-library.bin`
		sourcePath := fmt.Sprintf(`C:\ordinary\sources\%s-%d.txt`, ref.Case, ref.Step)
		queryPath := fmt.Sprintf(`C:\ordinary\queries\%s.scm`, ref.Query)
		spec := eligibilityProcessRequest{Path: `C:\ordinary\reference.exe`, Args: []string{libraryPath, profile.Native.Symbol, sourcePath, queryPath, ref.Query, response.Grammar.LibrarySHA256},
			Dir: `C:\ordinary`, Env: []string{}, StdoutBytes: 64 << 20, StderrBytes: 1 << 20, Wall: 10_000_000_000, Grace: 2_000_000_000}
		spec.Memory.Bytes, spec.Memory.Hard = 1<<30, true
		stdout := append(append([]byte(nil), row...), '\n')
		inv := f.invocation(invocationID, "current", invocationID, spec, stdout, 0, false)
		inv.Executable, inv.ExecutablePath = f.reg.Build.ReferenceBinary, spec.Path
		initialSource := read(filepath.Join(setDir, filepath.FromSlash(ordinarySourceMemberPath(t, manifest, ref.Case))))
		versions, _, err := ApplyEdits(EncodingUTF8, initialSource, c.Edits[:ref.Step], 65536)
		if err != nil || int(ref.Step) >= len(versions) || !ordinaryPinMatchesBytes(source, versions[ref.Step]) {
			t.Fatalf("cannot reconstruct synthetic reference source for %s/%d: %v", ref.Case, ref.Step, err)
		}
		sourceData := versions[ref.Step]
		sourceMember := f.member(invocationID, invocationID+"/source.bin", "source", sourceData)
		inv.Inputs["source"] = eligibilityInput{Kind: "PINNED", MemberPin: &sourceMember, ContentBasis: "APPROVED_EXPECTED_BYTES", ArgvIndex: eligibilityIndex(2), PathLiteral: sourcePath}
		inv.Inputs["library"] = eligibilityInput{Kind: "PINNED", MemberPin: &refLibrary, ContentBasis: "PRESERVED_ORIGINAL", ArgvIndex: eligibilityIndex(0), PathLiteral: libraryPath}
		queryMember := f.member(invocationID, invocationID+"/query.scm", "query", []byte(profileQuery.Source))
		inv.Inputs["query-0"] = eligibilityInput{Kind: "PINNED", MemberPin: &queryMember, ContentBasis: "APPROVED_EXPECTED_BYTES", ArgvIndex: eligibilityIndex(3), PathLiteral: queryPath}
		f.receiptMutation(invocationID, func(receipt *eligibilityReceipt) {
			receipt.Execution = ordinaryE2EWindowsExecution(receipt.Execution)
		})
		for j := range f.reg.Invocations {
			if f.reg.Invocations[j].ID == invocationID {
				f.reg.Invocations[j].ApprovedRequest = eligibilityJSON(t, spec)
				break
			}
		}
		inv.Node.Identities["platform"] = "windows/amd64"
		node := f.nodes[inv.Node.ID]
		node.Identities["platform"] = "windows/amd64"
		f.nodes[inv.Node.ID] = node
	}
	selectionBytes := eligibilityJSON(t, selection)
	refSelectionPath := req.Selection.ReferenceSelection.Path
	if refSelectionPath == "" {
		refSelectionPath = "caller/reference-selection.json"
	}

	// The checked-in eligibility fixture's current reference processes are synthetic Linux
	// processes; the actual owned producer runs on this Windows worker.
	ordinaryE2EWindowsFixtureReferences(t, f)
	eligibility := f.request()
	approval := f.approval
	approval.WitnessSHA256 = digestHex(nil)
	approval.Candidate.Platform, approval.Candidate.Run = manifest.Producer["platform"], manifest.RunID
	for i := range approval.Cohorts {
		if approval.Cohorts[i].Purpose == "current" {
			approval.Cohorts[i].ID = manifest.SemanticCohort
			approval.Cohorts[i].Platform, approval.Cohorts[i].Run = manifest.Producer["platform"], manifest.RunID
		}
	}
	for i := range f.reg.Invocations {
		if f.reg.Invocations[i].CohortID == "current" {
			f.reg.Invocations[i].CohortID = manifest.SemanticCohort
		}
	}
	for i := range f.reg.Tools {
		if f.reg.Tools[i].CohortID == "current" {
			f.reg.Tools[i].CohortID = manifest.SemanticCohort
		}
	}
	eligibility.Registration = eligibilityJSON(t, f.reg)
	approval.RegistrationSHA256 = digestHex(eligibility.Registration)
	ordinary := req.Selection
	ordinary.Schema, ordinary.Context, ordinary.ClosureEncoding = ordinaryEligibilityBindingSchema, manifest.Context, manifest.ClosureEncoding
	ordinary.Profile, ordinary.PlanningProfile, ordinary.Registration, ordinary.Assets, ordinary.Plan = manifest.Profile, manifest.PlanningProfile, manifest.Registration, manifest.Assets, manifest.Plan
	ordinary.Commitment = OrdinaryPin{Path: req.Selection.Commitment.Path, Bytes: uint64(len(req.PlanCommitment)), SHA256: digestHex(req.PlanCommitment)}
	ordinary.ReferenceSelection = OrdinaryPin{Path: refSelectionPath, Bytes: uint64(len(selectionBytes)), SHA256: digestHex(selectionBytes)}
	ordinary.RunBinding = OrdinaryPin{Path: "caller/run-binding.json", Bytes: uint64(len(produced.RunBinding)), SHA256: digestHex(produced.RunBinding)}
	ordinary.Candidate, ordinary.SemanticCohort, ordinary.RunID = manifest.Candidate, manifest.SemanticCohort, manifest.RunID
	ordinary.ShardIDs = append([]string(nil), manifest.ShardIDs...)
	approval.Ordinary = &ordinary
	eligibility.Approval = eligibilityJSON(t, approval)

	req.Evidence, req.EligibilityApproval, req.EligibilityRegistration = eligibility.Evidence, eligibility.Approval, eligibility.Registration
	req.Profile, req.RunBinding, req.ReferenceSelection = produced.Profile, produced.RunBinding, selectionBytes
	req.Selection = OrdinaryQuerySemanticsBinding(ordinary)
	return req
}

func ordinarySourceMemberPath(t *testing.T, manifest OrdinaryOracleManifest, caseID string) string {
	t.Helper()
	for _, member := range manifest.Members {
		if member.Owner == "O" && member.Role == "source" && member.CaseOrBatch == caseID {
			return member.Path
		}
	}
	t.Fatalf("ordinary r8 has no source member for %s", caseID)
	return ""
}

func ordinaryE2EWindowsExecution(execution eligibilityExecution) eligibilityExecution {
	execution.Cleanup.Method, execution.Cleanup.Scope = "JOB_OBJECT", "JOB_OBJECT"
	execution.Memory.Metric = "windows job-object process memory metric"
	execution.Capabilities.OS, execution.Capabilities.Backend = "windows/amd64", "windows-job-object"
	execution.Capabilities.TreeCleanup, execution.Capabilities.EscapedDescendants = "JOB_OBJECT", "CONTAINED"
	execution.Capabilities.Memory, execution.Capabilities.MemoryMetric = "HARD", execution.Memory.Metric
	return execution
}

func ordinaryE2ESyntheticResponse(t *testing.T, queryID, querySource, symbol, librarySHA string, source []byte) []byte {
	t.Helper()
	return ordinaryE2ESyntheticResponseWithExpected(t, queryID, querySource, symbol, librarySHA, source,
		[]TreeNode{{Parent: -1, Type: "source_file", Named: true, EndByte: uint32(len(source)), EndPoint: ordinaryE2EEndPoint(source)}},
		QueryWitnessMetadata{Patterns: 1, CaptureNames: []string{}, Predicates: []QueryPredicate{}}, []Capture{})
}

func ordinaryE2ESyntheticResponseWithExpected(t *testing.T, queryID, querySource, symbol, librarySHA string, source []byte, nodes []TreeNode, metadata QueryWitnessMetadata, captures []Capture) []byte {
	t.Helper()
	return ordinaryJSON(t, instrumentedReference{
		Schema: instrumentedReferenceSchema, Status: StatusCompleted, Code: "", Upstream: instrumentedReferenceCommit,
		Input: struct {
			Encoding string `json:"encoding"`
			Bytes    uint64 `json:"bytes"`
			SHA256   string `json:"sha256"`
		}{Encoding: EncodingUTF8, Bytes: uint64(len(source)), SHA256: digestHex(source)},
		Grammar: struct {
			LibrarySHA256 string `json:"library_sha256"`
			Symbol        string `json:"symbol"`
		}{LibrarySHA256: librarySHA, Symbol: symbol},
		Tree: struct {
			Nodes []TreeNode `json:"nodes"`
		}{Nodes: nodes},
		Query: struct {
			ID           string           `json:"id"`
			SHA256       string           `json:"sha256"`
			Patterns     uint32           `json:"patterns"`
			CaptureNames []string         `json:"capture_names"`
			Predicates   []QueryPredicate `json:"predicates"`
			Matches      uint64           `json:"matches"`
			MatchMapping []struct {
				Match     uint32 `json:"match"`
				RuntimeID uint32 `json:"runtime_id"`
			} `json:"match_mapping"`
			Structural []Capture `json:"structural"`
			Evaluated  []Capture `json:"evaluated"`
		}{ID: queryID, SHA256: digestHex([]byte(querySource)), Patterns: metadata.Patterns, CaptureNames: metadata.CaptureNames, Predicates: metadata.Predicates, Matches: uint64(len(captures)), MatchMapping: []struct {
			Match     uint32 `json:"match"`
			RuntimeID uint32 `json:"runtime_id"`
		}{}, Structural: captures, Evaluated: captures},
		Limits: struct {
			Nodes         uint64 `json:"nodes"`
			Captures      uint64 `json:"captures"`
			GroupCaptures uint64 `json:"group_captures"`
			Matches       uint64 `json:"matches"`
			WallMillis    uint64 `json:"wall_ms"`
		}{Nodes: 100000, Captures: 50000, GroupCaptures: 200000, Matches: 100000, WallMillis: 3000},
		CursorExhausted: true, DidExceedMatchLimit: false, Complete: true,
	})
}

func ordinaryE2EEndPoint(source []byte) Point {
	var point Point
	for _, value := range source {
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}

type ordinaryE2EJSONFixture struct {
	Producer     ordinaryE2EProducerInput
	Request      OrdinaryQuerySemanticsRequest
	References   []ordinaryE2EReference
	AssetBodies  map[string][]byte
	Profile      OracleProfile
	SelectedCase string
}

func ordinaryE2EOracleProfileBytes(t *testing.T, profile OracleProfile) []byte {
	t.Helper()
	wire := struct {
		Schema       string        `json:"schema"`
		ID           string        `json:"id"`
		Route        string        `json:"route"`
		Operation    string        `json:"operation"`
		Symbol       string        `json:"symbol"`
		Encoding     string        `json:"encoding"`
		Output       string        `json:"output"`
		Compiler     ToolIdentity  `json:"compiler"`
		Grammar      []NativeInput `json:"grammar"`
		Declarations *Declarations `json:"declarations"`
		Cases        []OracleCase  `json:"cases"`
		Queries      []OracleQuery `json:"queries"`
		FactPack     *FactPackRef  `json:"fact_pack"`
		API          bool          `json:"api"`
	}{profile.Native.Schema, profile.Native.ID, profile.Native.Route, profile.Native.Operation, profile.Native.Symbol, profile.Native.Encoding,
		profile.Native.Output, profile.Native.Compiler, profile.Native.Grammar, profile.Native.Declarations, profile.OracleCases,
		profile.Queries, profile.FactPack, profile.API}
	return ordinaryJSON(t, wire)
}

func ordinaryE2EJSONInput(t *testing.T) ordinaryE2EJSONFixture {
	t.Helper()
	const caseID = "alt-json-B01-05"
	routeSources := ordinaryRouteSources(t, "json")
	source, ok := routeSources[caseID]
	if !ok || source != "null" {
		t.Fatalf("P0 JSON source fixture changed for %s", caseID)
	}
	querySources := ordinaryRouteQueries(t, "json")
	querySource, ok := querySources["q.structure"]
	if !ok {
		t.Fatal("P0 JSON structure query fixture is absent")
	}
	p0, qualification := ordinaryP0ForPlanTest(t)
	plan := ordinaryPlanForTest(p0, qualification)
	binding := plan.Bindings[slices.IndexFunc(plan.Bindings, func(v OrdinaryBinding) bool { return v.Route == "json" && v.Platform == "windows-amd64" })]
	if len(binding.Grammar) == 0 {
		t.Fatal("P0 JSON grammar pins are absent")
	}
	cc := os.Getenv("TSGK_NATIVE_CC")
	compilerPath, err := filepath.EvalSymlinks(cc)
	if err != nil {
		t.Fatal(err)
	}
	compilerBytes, err := os.ReadFile(compilerPath)
	if err != nil {
		t.Fatal(err)
	}
	compiler := ToolIdentity{Name: "cc", Version: "pinned-ucrt64-gcc", SHA256: digestHex(compilerBytes), Bytes: uint64(len(compilerBytes))}
	sourceBytes := []byte(source)
	input := NativeInput{Path: "source/alt-json-B01-05.json", Role: "case", Bytes: uint64(len(sourceBytes)), SHA256: digestHex(sourceBytes)}
	caseProfile := IncrementalCase{ID: caseID, Input: input, Encoding: EncodingUTF8, Edits: []Edit{}, Points: []NativePoint{},
		Expect: []StepExpectation{{Step: 0, Syntax: "NO_ERROR", Contains: []string{"document", "null"}}}}
	emptyCaptures := []CaptureExpectation{}
	profile := OracleProfile{
		Native: IncrementalProfile{Schema: OracleSchema, ID: "ordinary-e2e-json", Route: "json", Operation: "native-query-batch", Symbol: "tree_sitter_json",
			Encoding: EncodingUTF8, Output: OutputTree, Compiler: compiler, Grammar: binding.Grammar, Cases: []IncrementalCase{caseProfile}},
		Queries: []OracleQuery{{ID: "q.structure", Source: querySource}}, API: true,
		OracleCases: []OracleCase{{IncrementalCase: caseProfile, QueryExpect: []QueryExpectation{{Query: "q.structure", Step: 0, Status: StatusCompleted, Captures: &emptyCaptures}}}},
	}
	executableProfile := ordinaryE2EOracleProfileBytes(t, profile)
	assets := OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Assets: []OrdinaryAsset{}}
	bodies := map[string][]byte{}
	addAsset := func(id, role, name string, data []byte) {
		t.Helper()
		basisPath, reviewPath, bodyPath := "review/"+name+"-basis.txt", "review/"+name+"-record.txt", "expected/"+name+".json"
		basis, review := []byte("independent source and query review for "+name), []byte("separately authored fixed JSON expectation for "+name)
		pin := OrdinaryPin{Path: bodyPath, Bytes: uint64(len(data)), SHA256: digestHex(data)}
		basisPin := OrdinaryPin{Path: basisPath, Bytes: uint64(len(basis)), SHA256: digestHex(basis)}
		reviewPin := OrdinaryPin{Path: reviewPath, Bytes: uint64(len(review)), SHA256: digestHex(review)}
		bodies[bodyPath], bodies[basisPath], bodies[reviewPath] = data, basis, review
		assets.Assets = append(assets.Assets, OrdinaryAsset{ID: id, Role: role, State: "PINNED", Pin: &pin, AuthoringBasis: &basisPin, IndependentReview: &reviewPin})
	}
	var anchors struct {
		Queries []QueryWitnessQuery `json:"queries"`
		Nodes   []TreeNode          `json:"nodes_each_version"`
	}
	anchorBytes, err := os.ReadFile("../testdata/query-semantics/json1-classification-anchors.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(anchorBytes, &anchors); err != nil || len(anchors.Queries) != 1 || len(anchors.Nodes) != 2 {
		t.Fatalf("fixed independent JSON anchor is malformed: %v", err)
	}
	metadata := *anchors.Queries[0].Metadata
	metadataData := ordinaryJSON(t, metadata)
	addAsset("m", "metadata", "json-metadata", metadataData)
	casePin := OrdinaryPin{Path: "source-witness/" + input.SHA256 + ".utf8", Bytes: input.Bytes, SHA256: input.SHA256}
	id := ordinaryTupleIdentity{Case: caseID, Query: "q.structure", Step: 0, TreeRole: "incremental", Source: casePin}
	tree := ordinaryExpectedTree{Schema: ordinaryExpectedTreeSchema, Case: caseID, Step: 0, TreeRole: "incremental", Source: casePin, Tree: anchors.Nodes}
	treeData := ordinaryJSON(t, tree)
	addAsset("t", "expected-tree", "json-tree", treeData)
	captures := ordinaryExpectedCaptures{Schema: ordinaryExpectedCaptureSchema, Case: id.Case, Query: id.Query, Step: id.Step, TreeRole: id.TreeRole, Source: id.Source, Matches: 0, Ordered: []Capture{}}
	captureData := ordinaryJSON(t, captures)
	addAsset("c", "expected-captures", "json-captures", captureData)
	assets.Assets = append(assets.Assets, OrdinaryAsset{ID: "f", Role: "expected-facts", State: "UNAVAILABLE"})
	refLibrary := []byte("synthetic reference library; never loaded")
	refMember := OrdinaryMember{Owner: "D_build", Path: "capture/reference-library.bin", Role: "reference-library", CaseOrBatch: "build",
		Pin: OrdinaryPin{Path: "capture/reference-library.bin", Bytes: uint64(len(refLibrary)), SHA256: digestHex(refLibrary)}}
	req, registration, fullPlan, planning, selectedSource, selectedCase, shardID := ordinarySelectedInput(t, "json", caseID, assets, "plain", true, executableProfile, refMember)
	if selectedCase != caseID || string(selectedSource) != source || shardID == "" {
		t.Fatalf("ordinary input helper selected wrong JSON case: %s/%s", selectedCase, shardID)
	}
	runtimeManifest, err := drivers.NativeC.ReadFile("native-c/runtime-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	runtimePin := queryregistrycore.FilePin("runtime/runtime-manifest.json", runtimeManifest)
	for i := range planning.Bindings {
		if planning.Bindings[i].Route == "json" && planning.Bindings[i].Platform == "windows-amd64" {
			planning.Bindings[i].Runtime = runtimePin
		}
	}
	for i := range fullPlan.Bindings {
		if fullPlan.Bindings[i].Route == "json" && fullPlan.Bindings[i].Platform == "windows-amd64" {
			fullPlan.Bindings[i].Runtime = runtimePin
		}
	}
	planningBytes := ordinaryJSON(t, planning)
	fullPlan.Profile = queryregistrycore.FilePin(fullPlan.Profile.Path, planningBytes)
	registration.Profile = fullPlan.Profile
	registration.Assets = fullPlan.Assets
	registrationBytes := ordinaryJSON(t, registration)
	fullPlan.Registrations[0] = queryregistrycore.FilePin(fullPlan.Registrations[0].Path, registrationBytes)
	selectedIndex := slices.IndexFunc(fullPlan.Shards, func(s OrdinaryPlanShard) bool { return s.ID == shardID })
	if selectedIndex < 0 {
		t.Fatal("selected JSON shard missing from complete plan")
	}
	commitment, selection := ordinarySelectionForTest(t, fullPlan, []int{selectedIndex})
	selection.Binding.Profile = queryregistrycore.FilePin("profile/executable.json", executableProfile)
	selection.Binding.Registration = fullPlan.Registrations[0]
	selection.Binding.PlanningProfile, selection.Binding.Assets = fullPlan.Profile, fullPlan.Assets
	req.Profile, req.PlanningProfile, req.Registration = executableProfile, planningBytes, registrationBytes
	req.PlanCommitment, req.ShardProofs = commitment, selection.Proofs
	req.Selection = OrdinaryQuerySemanticsBinding(selection.Binding)
	producer := ordinaryE2EProducerInput{Profile: executableProfile, PlanningProfile: planningBytes, AssetRegistration: req.AssetRegistration,
		RegistrationBytes: registrationBytes, Registration: registration, ShardPlan: ordinaryJSON(t, fullPlan), ShardIDs: []string{shardID},
		Sources: [][]byte{selectedSource}, GrammarRoot: `D:\AIDEV\tree-sitter-grammar-kit\.work\213-prep\verify-v0271\routes\json`}
	ref := ordinaryE2ESyntheticResponseWithExpected(t, "q.structure", querySource, profile.Native.Symbol, digestHex(refLibrary), selectedSource, anchors.Nodes, metadata, []Capture{})
	return ordinaryE2EJSONFixture{Producer: producer, Request: req, References: []ordinaryE2EReference{{Case: caseID, Query: "q.structure", Response: ref}},
		AssetBodies: bodies, Profile: profile, SelectedCase: caseID}
}

func ordinaryE2EWindowsFixtureReferences(t *testing.T, f *eligibilityFixture) {
	t.Helper()
	for i := range f.reg.Invocations {
		inv := &f.reg.Invocations[i]
		if inv.CohortID != "current" || inv.Kind != "REFERENCE_PROCESS" || inv.Request == nil {
			continue
		}
		var spec eligibilityProcessRequest
		if err := json.Unmarshal(f.files[inv.Request.Member.Path], &spec); err != nil {
			t.Fatal(err)
		}
		path := func(value string) string {
			if strings.HasPrefix(value, "/current") {
				return `C:\synthetic` + strings.ReplaceAll(strings.TrimPrefix(value, "/current"), "/", `\`)
			}
			return value
		}
		spec.Path = `C:\synthetic\reference.exe`
		spec.Dir = `C:\synthetic`
		spec.CgroupParent = ""
		for j := range spec.Args {
			spec.Args[j] = path(spec.Args[j])
		}
		f.replace(inv.Request.Member.Path, eligibilityJSON(t, spec))
		inv = &f.reg.Invocations[i]
		inv.ApprovedRequest = eligibilityJSON(t, spec)
		inv.ExecutablePath = spec.Path
		for name, index := range map[string]int{"library": 0, "source": 2, "query-0": 3} {
			input := inv.Inputs[name]
			input.PathLiteral = spec.Args[index]
			inv.Inputs[name] = input
		}
		f.receiptMutation(inv.ID, func(receipt *eligibilityReceipt) {
			receipt.Execution = ordinaryE2EWindowsExecution(receipt.Execution)
		})
		inv.Node.Identities["platform"] = "windows/amd64"
		node := f.nodes[inv.Node.ID]
		node.Identities["platform"] = "windows/amd64"
		f.nodes[inv.Node.ID] = node
	}
}

type ordinaryE2EIdentityContext struct {
	context.Context
	Root  *os.Root
	Path  string
	Data  []byte
	Fired bool
}

func (c *ordinaryE2EIdentityContext) Err() error {
	if c.Fired {
		return c.Context.Err()
	}
	pcs := make([]uintptr, 24)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ".ordinarySnapshotIdentity") {
			temp := filepath.ToSlash(filepath.Join(filepath.Dir(c.Path), "identity-replacement.bin"))
			file, err := c.Root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			if _, err := file.Write(c.Data); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			if err := c.Root.Remove(c.Path); err != nil {
				return err
			}
			if err := c.Root.Rename(temp, c.Path); err != nil {
				return err
			}
			c.Fired = true
			break
		}
		if !more {
			break
		}
	}
	return c.Context.Err()
}

func ordinaryE2EIdentityChangedContext(ctx context.Context, root *os.Root, path string, original []byte) context.Context {
	changed := append([]byte(nil), original...)
	if len(changed) == 0 {
		changed = []byte("x")
	} else {
		changed[len(changed)-1] ^= 1
	}
	return &ordinaryE2EIdentityContext{Context: ctx, Root: root, Path: path, Data: changed}
}

func ordinaryE2EWriteProducerInput(t *testing.T, input ordinaryE2EProducerInput) (string, string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	workRoot := filepath.Join(root, ".work", "e2e-checks")
	inputDir := filepath.Join(workRoot, "inputs")
	if err := os.MkdirAll(inputDir, 0700); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(inputDir, fmt.Sprintf("ordinary-%d.json", time.Now().UnixNano()))
	data := ordinaryJSON(t, input)
	if err := os.WriteFile(inputPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	producerDir := filepath.Join(workRoot, fmt.Sprintf("producer-%d", time.Now().UnixNano()))
	return inputPath, producerDir
}

func ordinaryE2ERunOwnedProducer(t *testing.T, inputPath, producerDir string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("TSGK_NATIVE_RUNTIME") == "" || os.Getenv("TSGK_NATIVE_CC") == "" {
		t.Skip("ordinary public positive requires the pinned runtime and compiler")
	}
	if os.Getenv("TSGK_NATIVE_RUNTIME") != `D:\AIDEV\tree-sitter-grammar-kit\.work\352-data\s05\runtime` ||
		os.Getenv("TSGK_NATIVE_CC") != `C:\msys64\ucrt64\bin\gcc.exe` || os.Getenv("TSGK_NATIVE_REQUIRED") != "1" {
		t.Fatalf("ordinary native public probe requires the brief's exact pinned runtime/compiler and TSGK_NATIVE_REQUIRED=1")
	}
	env := ordinaryOwnedNativeEnv(producerDir)
	env = slices.DeleteFunc(env, func(entry string) bool {
		return strings.HasPrefix(strings.ToUpper(entry), "TSGK_ORDINARY_OWNED_INPUTS=")
	})
	env = append(env, "TSGK_ORDINARY_OWNED_INPUTS="+inputPath)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "./src/internal/native", "-run", "^TestOracleOrdinaryE2EFixture$", "-count=1", "-timeout=180s")
	cmd.Dir, cmd.Env = root, env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ordinary owned r8 producer failed: %v\n%s", err, output)
	}
}

func TestVerifyOrdinaryQuerySemanticsOwnedJSONPublicPassAndNegativeCases(t *testing.T) {
	if os.Getenv("TSGK_NATIVE_RUNTIME") == "" || os.Getenv("TSGK_NATIVE_CC") == "" {
		t.Skip("ordinary public positive requires the pinned runtime and compiler")
	}
	fixture := ordinaryE2EJSONInput(t)
	inputPath, producerDir := ordinaryE2EWriteProducerInput(t, fixture.Producer)
	ordinaryE2ERunOwnedProducer(t, inputPath, producerDir)
	req := ordinaryE2EAssemble(t, fixture.Request, producerDir, fixture.References, fixture.AssetBodies)
	call := func(ctx context.Context, request OrdinaryQuerySemanticsRequest) (OrdinaryQuerySemanticsResult, error) {
		t.Helper()
		return VerifyOrdinaryQuerySemantics(ctx, request)
	}
	run := func(request OrdinaryQuerySemanticsRequest) (OrdinaryQuerySemanticsResult, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
		defer cancel()
		return call(ctx, request)
	}

	positive, err := run(req)
	if err != nil || positive.Semantic != AssessPass || positive.Assessment != AssessPass || !positive.Complete || !positive.ProcessingComplete || !positive.BindingValid ||
		positive.Tuple.Required != 1 || positive.Tuple.Verified != 1 || positive.Tuple.Failed != 0 || positive.Tuple.Blocked != 0 || positive.Tuple.Unprocessed != 0 ||
		positive.Eligibility == nil || positive.Eligibility.Assessment != AssessPass || !positive.Eligibility.Complete {
		t.Fatalf("public owned JSON pass failed: err=%v result=%+v", err, positive)
	}
	t.Logf("public VERIFY pass uses owned native r8, synthetic independent eligibility/reference: case=%s tuple=%+v base=%s work=%d logical=%d read=%d complete=%t",
		fixture.SelectedCase, positive.Tuple, positive.BaseVerified.Assessment, positive.Work, positive.LogicalBytes, positive.BytesRead, positive.Complete)

	missing := req
	missing.EligibilityApproval = nil
	missingResult, missingErr := run(missing)
	if missingErr != nil || missingResult.Code != "ELIGIBILITY_APPROVAL_MISSING" || missingResult.Tuple.Blocked != missingResult.Tuple.Required || missingResult.Tuple.Unprocessed != 0 || missingResult.Complete {
		t.Fatalf("public missing-approval result changed: err=%v result=%+v", missingErr, missingResult)
	}
	t.Logf("negative missing approval: code=%s blocked=%d required=%d complete=%t", missingResult.Code, missingResult.Tuple.Blocked, missingResult.Tuple.Required, missingResult.Complete)

	approvalMismatch := req
	var mismatchApproval eligibilityApproval
	if err := json.Unmarshal(req.EligibilityApproval, &mismatchApproval); err != nil {
		t.Fatal(err)
	}
	mismatchApproval.Ordinary.ShardIDs[0] = "foreign-approved-shard"
	approvalMismatch.EligibilityApproval = eligibilityJSON(t, mismatchApproval)
	mismatchResult, mismatchErr := run(approvalMismatch)
	if mismatchErr != nil && asError(mismatchErr).Code != "ELIGIBILITY_ORDINARY_SCOPE" {
		t.Fatalf("approval/selection mismatch returned an unexpected error: %v result=%+v", mismatchErr, mismatchResult)
	}
	if mismatchResult.Semantic == AssessPass || mismatchResult.Tuple.Verified != 0 || mismatchResult.BindingValid {
		t.Fatalf("approval/selection mismatch retained VERIFIED: %+v", mismatchResult)
	}
	t.Logf("negative approval/selection mismatch: err=%v code=%s verified=%d binding=%t", mismatchErr, mismatchResult.Code, mismatchResult.Tuple.Verified, mismatchResult.BindingValid)

	otherCohort := req
	var cohortApproval eligibilityApproval
	if err := json.Unmarshal(req.EligibilityApproval, &cohortApproval); err != nil {
		t.Fatal(err)
	}
	var cohortRegistration eligibilityRegistration
	if err := json.Unmarshal(req.EligibilityRegistration, &cohortRegistration); err != nil {
		t.Fatal(err)
	}
	previousCohort := ""
	for i := range cohortApproval.Cohorts {
		if cohortApproval.Cohorts[i].Purpose == "current" {
			previousCohort = cohortApproval.Cohorts[i].ID
			cohortApproval.Cohorts[i].ID = "different-current-cohort"
		}
	}
	for i := range cohortRegistration.Invocations {
		if cohortRegistration.Invocations[i].CohortID == previousCohort {
			cohortRegistration.Invocations[i].CohortID = "different-current-cohort"
		}
	}
	for i := range cohortRegistration.Tools {
		if cohortRegistration.Tools[i].CohortID == previousCohort {
			cohortRegistration.Tools[i].CohortID = "different-current-cohort"
		}
	}
	otherCohort.EligibilityRegistration = eligibilityJSON(t, cohortRegistration)
	cohortApproval.RegistrationSHA256 = digestHex(otherCohort.EligibilityRegistration)
	otherCohort.EligibilityApproval = eligibilityJSON(t, cohortApproval)
	cohortResult, cohortErr := run(otherCohort)
	if cohortErr != nil || cohortResult.Code != "ORDINARY_ELIGIBILITY_COHORT" {
		t.Fatalf("different-cohort case returned an unexpected error: %v result=%+v", cohortErr, cohortResult)
	}
	if cohortResult.Semantic == AssessPass || cohortResult.Tuple.Verified != 0 || cohortResult.BindingValid {
		t.Fatalf("different cohort retained VERIFIED: %+v", cohortResult)
	}
	t.Logf("negative different cohort: err=%v code=%s verified=%d binding=%t", cohortErr, cohortResult.Code, cohortResult.Tuple.Verified, cohortResult.BindingValid)

	mutated := req
	manifestPath := filepath.Join(req.Evidence.Root, "oracle", "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil || len(manifestData) == 0 {
		t.Fatalf("read member to mutate: %v", err)
	}
	manifestData[len(manifestData)-1] ^= 1
	if err := os.WriteFile(manifestPath, manifestData, 0600); err != nil {
		t.Fatal(err)
	}
	memberResult, memberErr := run(mutated)
	if memberResult.Semantic == AssessPass || memberResult.Tuple.Verified != 0 || memberResult.BindingValid {
		t.Fatalf("member mutation retained VERIFIED: err=%v result=%+v", memberErr, memberResult)
	}
	t.Logf("negative member mutation: err=%v code=%s verified=%d binding=%t", memberErr, memberResult.Code, memberResult.Tuple.Verified, memberResult.BindingValid)

	identityReq := ordinaryE2EAssemble(t, fixture.Request, producerDir, fixture.References, fixture.AssetBodies)
	identityPath := "oracle/manifest.json"
	identityData, err := os.ReadFile(filepath.Join(identityReq.Evidence.Root, filepath.FromSlash(identityPath)))
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(identityReq.Evidence.Root)
	if err != nil {
		t.Fatal(err)
	}
	identityBase, identityCancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	identityCtx := ordinaryE2EIdentityChangedContext(identityBase, root, identityPath, identityData)
	identityResult, identityErr := call(identityCtx, identityReq)
	identityCancel()
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	if identityErr == nil || asError(identityErr).Code != "SOURCE_CHANGED" || identityResult.Tuple.Verified != 0 || identityResult.Tuple.Blocked != identityResult.Tuple.Required || identityResult.Complete {
		t.Fatalf("public final identity change did not downgrade VERIFIED: err=%v result=%+v", identityErr, identityResult)
	}
	t.Logf("negative final identity change: code=%s verified=%d blocked=%d complete=%t", asError(identityErr).Code, identityResult.Tuple.Verified, identityResult.Tuple.Blocked, identityResult.Complete)

	baseFailInput := fixture.Producer
	baseFailProfile := fixture.Profile
	baseFailProfile.OracleCases[0].QueryExpect[0].Status = StatusFailed
	baseFailProfile.OracleCases[0].QueryExpect[0].Code = "QUERY_COMPILE"
	baseFailProfile.OracleCases[0].QueryExpect[0].Captures = nil
	baseFailInput.Profile = ordinaryE2EOracleProfileBytes(t, baseFailProfile)
	baseFailInputPath, baseFailProducerDir := ordinaryE2EWriteProducerInput(t, baseFailInput)
	ordinaryE2ERunOwnedProducer(t, baseFailInputPath, baseFailProducerDir)
	baseFailReq := ordinaryE2EAssemble(t, fixture.Request, baseFailProducerDir, fixture.References, fixture.AssetBodies)
	baseResult, baseErr := run(baseFailReq)
	if baseErr != nil || baseResult.BaseVerified.Assessment != AssessFail || baseResult.Semantic == AssessPass || baseResult.Tuple.Verified != 0 {
		t.Fatalf("recorded base FAIL was not preserved by the public call: err=%v result=%+v", baseErr, baseResult)
	}
	t.Logf("public base failure preserved: base=%s semantic=%s code=%s", baseResult.BaseVerified.Assessment, baseResult.Semantic, baseResult.Code)
}
