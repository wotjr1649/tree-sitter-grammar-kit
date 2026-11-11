package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// ordinaryFullPosCase is one selected JSON shard: the exact versions of its source history and the
// grammar-derived expectation of every version. Trees[step] == nil means no independent expectation.
type ordinaryFullPosCase struct {
	ID       string
	Versions [][]byte
	Edits    []Edit
	Points   []NativePoint
	Expect   []StepExpectation
	Trees    [][]TreeNode
	Captures [][]Capture
}

// ordinaryFullPosMutation changes caller-authored expectations before they are pinned, so that
// every mutated pin stays internally consistent and only the independent check can object.
type ordinaryFullPosMutation struct {
	Cases    func(cases []ordinaryFullPosCase)
	Required func(required *OrdinaryRequired)
}

type ordinaryFullPosInput struct {
	Request      OrdinaryQuerySemanticsRequest
	Producer     ordinaryE2EProducerInput
	References   []ordinaryE2EReference
	AssetBodies  map[string][]byte
	Cases        []ordinaryFullPosCase
	Registration OrdinaryRegistration
	ShardIDs     []string
}

func ordinaryFullPosTreeAsset(caseID string, step int, role string) string {
	return fmt.Sprintf("tree-%s-s%d-%s", caseID, step, role)
}

func ordinaryFullPosCaptureAsset(caseID string, step int, role string) string {
	return fmt.Sprintf("captures-%s-s%d-%s-q.structure", caseID, step, role)
}

// ordinaryFullPosGrammarRoot assembles the JSON grammar the P0 registry pins from tracked files only:
// the generated parser.c is checked in beside this regression and the three runtime headers are the
// tracked plain-grammar fixture's, which are byte-identical to the pinned ones. Every file is
// compared with the registry pin before it is used.
func ordinaryFullPosGrammarRoot(t *testing.T) string {
	t.Helper()
	_, inventory := ordinaryP0ForPlanTest(t)
	workload := inventory.Routes[ordinaryRouteIndex(inventory, "json")].Workload
	sources := map[string]string{
		"src/parser.c":             "../testdata/ordinary-json/parser.c",
		"src/tree_sitter/alloc.h":  "../testdata/native/plain/src/tree_sitter/alloc.h",
		"src/tree_sitter/array.h":  "../testdata/native/plain/src/tree_sitter/array.h",
		"src/tree_sitter/parser.h": "../testdata/native/plain/src/tree_sitter/parser.h",
	}
	root := filepath.Join(t.TempDir(), "json")
	for _, pin := range workload.Grammar {
		from, ok := sources[pin.Path]
		if !ok {
			t.Fatalf("the pinned JSON grammar has an unexpected member %s", pin.Path)
		}
		data, err := os.ReadFile(from)
		if err != nil {
			t.Fatal(err)
		}
		if uint64(len(data)) != pin.Bytes || digestHex(data) != pin.SHA256 {
			t.Fatalf("tracked fixture %s does not match the P0 pin for %s", from, pin.Path)
		}
		target := filepath.Join(root, filepath.FromSlash(pin.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if len(workload.Grammar) != len(sources) {
		t.Fatalf("the pinned JSON grammar has %d members, the tracked fixture supplies %d", len(workload.Grammar), len(sources))
	}
	return root
}

func ordinaryFullPosQuality(t *testing.T, caseID string) ordinaryFullPosCase {
	t.Helper()
	_, inventory := ordinaryP0ForPlanTest(t)
	workload := inventory.Routes[ordinaryRouteIndex(inventory, "json")].Workload
	index := slices.IndexFunc(workload.Cases, func(c QualCase) bool { return c.ID == caseID })
	if index < 0 {
		t.Fatalf("P0 JSON case %s is missing", caseID)
	}
	qual := workload.Cases[index]
	sourceCase := caseID
	var queryCases struct {
		Cases []struct {
			ID         string `json:"id"`
			SourceCase string `json:"source_case"`
		} `json:"cases"`
	}
	queryFile, err := os.ReadFile("../testdata/native/queries/json.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(queryFile, &queryCases); err != nil {
		t.Fatal(err)
	}
	for _, c := range queryCases.Cases {
		if c.ID == caseID && c.SourceCase != "" {
			sourceCase = c.SourceCase // json-query-r1 reuses the json-feature-r1 source text
		}
	}
	text, ok := ordinaryRouteSources(t, "json")[sourceCase]
	if !ok || uint64(len(text)) != qual.Input.Bytes || digestHex([]byte(text)) != qual.Input.SHA256 {
		t.Fatalf("P0 source pin mismatch for %s", caseID)
	}
	versions, _, err := ApplyEdits(EncodingUTF8, []byte(text), qual.Edits, 65536)
	if err != nil {
		t.Fatal(err)
	}
	c := ordinaryFullPosCase{ID: caseID, Versions: versions, Edits: qual.Edits, Points: append([]NativePoint{}, qual.Points...), Expect: qual.Expect}
	for step, version := range versions {
		tree, err := ordinaryJSONExpectedTree(version)
		if err != nil {
			t.Fatalf("%s step %d is outside the grammar-derived expectation: %v", caseID, step, err)
		}
		c.Trees = append(c.Trees, tree)
		c.Captures = append(c.Captures, ordinaryJSONExpectedCaptures(tree))
	}
	return c
}

// ordinaryFullPosJSONInput builds the complete caller input, the producer input and the synthetic
// reference rows for the given P0 JSON cases from tracked files only.
func ordinaryFullPosJSONInput(t *testing.T, grammarRoot string, caseIDs []string, mutate ordinaryFullPosMutation) ordinaryFullPosInput {
	t.Helper()
	v, inventory := ordinaryP0ForPlanTest(t)
	plan := ordinaryPlanForTest(v, inventory)
	routeIndex := ordinaryRouteIndex(inventory, "json")
	route := inventory.Routes[routeIndex]
	const platform = "windows-amd64"
	cases := make([]ordinaryFullPosCase, 0, len(caseIDs))
	for _, id := range caseIDs {
		cases = append(cases, ordinaryFullPosQuality(t, id))
	}
	shardIndex := func(caseID string) int {
		return slices.IndexFunc(plan.Shards, func(s OrdinaryPlanShard) bool { return s.Route == "json" && s.Platform == platform && s.Case == caseID })
	}
	// A commitment needs sorted shard IDs and the producer needs registration order; the caller must pick
	// cases where both orders agree.
	slices.SortFunc(cases, func(a, b ordinaryFullPosCase) int { return shardIndex(a.ID) - shardIndex(b.ID) })
	// Reference rows model an independent reference run: they keep the true expectation while a
	// counterexample mutates only the caller-authored expectation assets below.
	pristine := map[string]ordinaryFullPosCase{}
	for i := range cases {
		c := cases[i]
		c.Trees, c.Captures = slices.Clone(c.Trees), slices.Clone(c.Captures)
		pristine[c.ID] = c
	}
	if mutate.Cases != nil {
		mutate.Cases(cases)
	}
	selectedCase := map[string]*ordinaryFullPosCase{}
	var indices []int
	var shardIDs []string
	for i := range cases {
		selectedCase[cases[i].ID] = &cases[i]
		index := shardIndex(cases[i].ID)
		if index < 0 {
			t.Fatalf("no JSON shard for %s", cases[i].ID)
		}
		indices = append(indices, index)
		shardIDs = append(shardIDs, plan.Shards[index].ID)
	}

	querySource := ordinaryRouteQueries(t, "json")["q.structure"]
	if digestHex([]byte(querySource)) != route.Workload.Queries[0].SHA256 || len(route.Workload.Queries) != 1 {
		t.Fatal("the JSON query registry changed")
	}
	var anchors struct {
		Queries []QueryWitnessQuery `json:"queries"`
	}
	anchorBytes, err := os.ReadFile("../testdata/query-semantics/json1-classification-anchors.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(anchorBytes, &anchors); err != nil || len(anchors.Queries) != 1 || anchors.Queries[0].Metadata == nil {
		t.Fatalf("fixed independent JSON metadata is malformed: %v", err)
	}
	metadata := *anchors.Queries[0].Metadata

	// Assets: independent expectations per tuple; the rest stays UNAVAILABLE.
	assets := OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Assets: []OrdinaryAsset{}}
	for _, asset := range []struct{ id, role string }{{"m", "metadata"}, {"t", "expected-tree"}, {"c", "expected-captures"}, {"f", "expected-facts"}} {
		assets.Assets = append(assets.Assets, OrdinaryAsset{ID: asset.id, Role: asset.role, State: "UNAVAILABLE"})
	}
	bodies := map[string][]byte{}
	add := func(id, role, basisText string, body []byte) {
		t.Helper()
		base := "expected/json/" + id
		bodyPath, basisPath, reviewPath := base+".json", "basis/"+base+".txt", "review/"+base+".txt"
		basis := []byte(basisText)
		review := []byte("Fixture review: tuple identity and pins checked against the fixed P0 JSON route and the pinned grammar.")
		pin := OrdinaryPin{Path: bodyPath, Bytes: uint64(len(body)), SHA256: digestHex(body)}
		basisPin := OrdinaryPin{Path: basisPath, Bytes: uint64(len(basis)), SHA256: digestHex(basis)}
		reviewPin := OrdinaryPin{Path: reviewPath, Bytes: uint64(len(review)), SHA256: digestHex(review)}
		assets.Assets = append(assets.Assets, OrdinaryAsset{ID: id, Role: role, State: "PINNED", Pin: &pin, AuthoringBasis: &basisPin, IndependentReview: &reviewPin})
		bodies[bodyPath], bodies[basisPath], bodies[reviewPath] = body, basis, review
	}
	add("meta-q.structure", "metadata", "Fixed JSON q.structure compiled-query metadata anchor (json1-classification-anchors.json).", ordinaryJSON(t, metadata))
	for _, c := range cases {
		for step, version := range c.Versions {
			source := OrdinaryPin{Path: "source-witness/" + digestHex(version) + ".utf8", Bytes: uint64(len(version)), SHA256: digestHex(version)}
			for _, role := range []string{"incremental", "fresh"} {
				if role == "fresh" && step == 0 {
					continue
				}
				if c.Trees[step] == nil {
					continue
				}
				tree := ordinaryExpectedTree{Schema: ordinaryExpectedTreeSchema, Case: c.ID, Step: uint32(step), TreeRole: role, Source: source, Tree: c.Trees[step]}
				add(ordinaryFullPosTreeAsset(c.ID, step, role), "expected-tree", "Syntax tree derived from the pinned JSON grammar productions and lexer for this exact version.", ordinaryJSON(t, tree))
				captures := ordinaryExpectedCaptures{Schema: ordinaryExpectedCaptureSchema, Case: c.ID, Query: "q.structure", Step: uint32(step), TreeRole: role, Source: source,
					Matches: ordinaryJSONExpectedMatches(c.Captures[step]), Ordered: c.Captures[step]}
				add(ordinaryFullPosCaptureAsset(c.ID, step, role), "expected-captures", "q.structure evaluated over the grammar-derived tree: three pair patterns, matches numbered by first appearance.", ordinaryJSON(t, captures))
			}
		}
	}

	// Compiler and profile.
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
	profileCases := make([]OracleCase, 0, len(cases))
	sources := make([][]byte, 0, len(cases))
	for _, c := range cases {
		input := NativeInput{Path: "cases/" + c.ID + ".json", Role: "case", Bytes: uint64(len(c.Versions[0])), SHA256: digestHex(c.Versions[0])}
		profileCases = append(profileCases, OracleCase{IncrementalCase: IncrementalCase{ID: c.ID, Input: input, Encoding: EncodingUTF8, Edits: c.Edits, Points: c.Points, Expect: c.Expect}, QueryExpect: []QueryExpectation{}})
		sources = append(sources, c.Versions[0])
	}
	profile := OracleProfile{
		Native: IncrementalProfile{Schema: OracleSchema, ID: "ordinary-full-positive-json", Route: "json", Operation: route.Workload.Operation, Symbol: route.Workload.Symbol,
			Encoding: EncodingUTF8, Output: OutputTree, Compiler: compiler, Grammar: route.Workload.Grammar, Declarations: route.Workload.Declarations},
		Queries: []OracleQuery{{ID: "q.structure", Source: querySource}}, API: true, OracleCases: profileCases,
	}
	for _, c := range profileCases {
		profile.Native.Cases = append(profile.Native.Cases, c.IncrementalCase)
	}
	executable := ordinaryE2EOracleProfileBytes(t, profile)

	// Plan, registration and selection.
	refLibrary := []byte("synthetic reference library; never loaded")
	planned := OrdinaryMember{Owner: "D_build", Path: "capture/reference-library.bin", Role: "reference-library", CaseOrBatch: "build",
		Pin: OrdinaryPin{Path: "capture/reference-library.bin", Bytes: uint64(len(refLibrary)), SHA256: digestHex(refLibrary)}}
	runtimeManifest, err := drivers.NativeC.ReadFile("native-c/runtime-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	runtimePin := queryregistrycore.FilePin("runtime/runtime-manifest.json", runtimeManifest)
	for i := range plan.Bindings {
		if plan.Bindings[i].Route == "json" {
			plan.Bindings[i].ClosureEncoding = "plain"
			plan.Bindings[i].Members = []OrdinaryMember{planned}
			plan.Bindings[i].Runtime = runtimePin
		}
	}
	p := ordinaryProfileFixture()
	p.Candidate, p.SemanticCohort, p.Bindings = plan.Candidate, plan.SemanticCohort, plan.Bindings
	planningProfile := ordinaryJSON(t, p)
	assets.Registry, assets.Candidate, assets.SemanticCohort = plan.Registry, plan.Candidate, plan.SemanticCohort
	assetData := ordinaryJSON(t, assets)
	profilePath, assetsPath := "profile/tsgk-oracle-r4.json", "profile/ordinary-assets-r1.json"
	plan.Profile = queryregistrycore.FilePin(profilePath, planningProfile)
	plan.Assets = queryregistrycore.FilePin(assetsPath, assetData)
	registration := OrdinaryRegistration{Schema: "tsgk-query-semantics-registration/r2", Registry: plan.Registry, Namespace: plan.Namespace,
		Inventory: queryregistrycore.QualificationPin, Slots: queryregistrycore.SlotPin, Profile: plan.Profile, Assets: plan.Assets,
		Route: route.Route, Set: route.Workload.Set, Platform: platform, Subject: "windows/amd64", MembershipContext: "qualification/r8",
		Context: OrdinaryContext, ClosureEncoding: "plain", Candidate: plan.Candidate, SemanticCohort: plan.SemanticCohort, PlannedMembers: []OrdinaryMember{planned}}
	for i, original := range route.Workload.Cases {
		index := shardIndex(original.ID)
		if index < 0 {
			t.Fatalf("no shard for JSON case %s", original.ID)
		}
		shard := &plan.Shards[index]
		shard.SourceHistory[0] = OrdinaryPlanHistoryStep{Bytes: original.Input.Bytes, SHA256: original.Input.SHA256}
		for step, size := range stepSizes(original.Input.Bytes, original.Edits) {
			if step < len(shard.SourceHistory) {
				shard.SourceHistory[step].Bytes = size
			}
		}
		c := OrdinaryCase{ID: original.ID, OriginalSelector: "qualification_inventory#/routes/" + strconv.Itoa(routeIndex) + "/workload/cases/" + strconv.Itoa(i), Edits: original.Edits, Points: shard.EditPoints}
		if selected := selectedCase[original.ID]; selected != nil {
			versions, points, err := ApplyEdits(EncodingUTF8, selected.Versions[0], original.Edits, 65536)
			if err != nil || len(versions) != len(selected.Versions) {
				t.Fatalf("history of %s is not reproducible: %v", original.ID, err)
			}
			shard.SourceHistory = make([]OrdinaryPlanHistoryStep, len(versions))
			shard.EditPoints = points
			c.Points = points
			for step, version := range versions {
				hash := digestHex(version)
				shard.SourceHistory[step] = OrdinaryPlanHistoryStep{Bytes: uint64(len(version)), SHA256: hash}
			}
		}
		shard.Capacity = ordinaryShardCapacity(shard.SourceHistory, []OrdinaryMember{planned})
		for _, version := range shard.SourceHistory {
			c.VersionPins = append(c.VersionPins, OrdinaryPin{Path: "source-witness/" + version.SHA256 + ".utf8", Bytes: version.Bytes, SHA256: version.SHA256})
		}
		c.InitialSource = c.VersionPins[0]
		registration.Cases = append(registration.Cases, c)
	}
	registration.Queries = []OrdinaryQuery{{ID: "q.structure", Source: OrdinaryPin{Path: "queries/q.structure", Bytes: uint64(len(querySource)), SHA256: route.Workload.Queries[0].SHA256},
		MetadataAsset: "meta-q.structure", Kind: queryregistrycore.Structural}}
	for _, coord := range v.Coordinates() {
		if coord.Kind != queryregistrycore.Structural || coord.Route != "json" || coord.Platform != platform {
			continue
		}
		treeAsset, captureAsset := "t", "c"
		if selected := selectedCase[coord.Case]; selected != nil && selected.Trees[coord.Step] != nil {
			treeAsset = ordinaryFullPosTreeAsset(coord.Case, int(coord.Step), coord.TreeRole)
			captureAsset = ordinaryFullPosCaptureAsset(coord.Case, int(coord.Step), coord.TreeRole)
		}
		required := OrdinaryRequired{ID: coord.ID(registration.Namespace), Coordinate: coord, TreeAsset: treeAsset, CaptureAsset: captureAsset}
		if mutate.Required != nil {
			mutate.Required(&required)
		}
		registration.Required = append(registration.Required, required)
	}
	registration.Required = slices.Clone(registration.Required)
	registrationData := ordinaryJSON(t, registration)
	plan.Registrations = []OrdinaryPin{queryregistrycore.FilePin("registrations/json/windows-amd64.json", registrationData)}
	commitment, selection := ordinarySelectionForTest(t, plan, indices)
	selection.Binding.ClosureEncoding = "plain"
	selection.Binding.Profile = queryregistrycore.FilePin("profile/executable.json", executable)

	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	request := OrdinaryQuerySemanticsRequest{Selection: OrdinaryQuerySemanticsBinding(selection.Binding), PlanningProfile: planningProfile, Registration: registrationData,
		AssetRegistration: assetData, PlanCommitment: commitment, ShardProofs: selection.Proofs,
		Registry: read("../contracts/query-semantic-registry-r1.json"), RegistrySHA256: ordinaryRegistrySHA,
		QualificationInventory: read("../contracts/qualification-c1.json"), SlotInventory: read("../contracts/query-slot-inventory-r1.json")}
	request.Profile = executable

	// Synthetic reference rows over the same grammar-derived expectation; no reference runs.
	versionsByCase := map[string][][]byte{}
	for _, c := range cases {
		versionsByCase[c.ID] = c.Versions
	}
	var references []ordinaryE2EReference
	for _, key := range ordinaryReferenceKeys(registration, versionsByCase) {
		parts := strings.Split(key, "/")
		step, err := strconv.Atoi(parts[1])
		if err != nil || len(parts) != 3 || parts[2] != "q.structure" {
			t.Fatalf("unexpected reference key %q", key)
		}
		c := pristine[parts[0]]
		references = append(references, ordinaryE2EReference{Case: parts[0], Step: uint32(step), Query: parts[2],
			Response: ordinaryFullPosReference(t, querySource, profile.Native.Symbol, digestHex(refLibrary), c.Versions[step], c.Trees[step], metadata, c.Captures[step])})
	}
	producer := ordinaryE2EProducerInput{Profile: executable, PlanningProfile: planningProfile, AssetRegistration: assetData, RegistrationBytes: registrationData,
		Registration: registration, ShardPlan: ordinaryJSON(t, plan), ShardIDs: shardIDs, Sources: sources, GrammarRoot: grammarRoot}
	return ordinaryFullPosInput{Request: request, Producer: producer, References: references, AssetBodies: bodies, Cases: cases, Registration: registration, ShardIDs: shardIDs}
}

// ordinaryFullPosReference is a stored instrumented-reference row. It reuses the synthetic row
// builder and then states the match count and runtime mapping of the expected capture stream.
func ordinaryFullPosReference(t *testing.T, querySource, symbol, librarySHA string, source []byte, tree []TreeNode, metadata QueryWitnessMetadata, captures []Capture) []byte {
	t.Helper()
	raw := ordinaryE2ESyntheticResponseWithExpected(t, "q.structure", querySource, symbol, librarySHA, source, tree, metadata, captures)
	var row instrumentedReference
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatal(err)
	}
	row.Query.Matches = ordinaryJSONExpectedMatches(captures)
	row.Query.MatchMapping = row.Query.MatchMapping[:0]
	for match := uint64(0); match < row.Query.Matches; match++ {
		row.Query.MatchMapping = append(row.Query.MatchMapping, struct {
			Match     uint32 `json:"match"`
			RuntimeID uint32 `json:"runtime_id"`
		}{uint32(match), uint32(match)})
	}
	return ordinaryJSON(t, row)
}

// ordinaryFullPosNativeSetup skips without the pinned native tools exactly like the other owned-native regressions.
func ordinaryFullPosNativeSetup(t *testing.T) {
	t.Helper()
	if os.Getenv("TSGK_NATIVE_RUNTIME") == "" || os.Getenv("TSGK_NATIVE_CC") == "" {
		t.Skip("ordinary full positive requires the pinned runtime and compiler")
	}
	if os.Getenv("TSGK_NATIVE_RUNTIME") == "D:/AIDEV/tree-sitter-grammar-kit/.work/213-prep/verify-v0271/runtime" {
		// The general kit suite and this owned producer use separately pinned runtime roots.
		t.Setenv("TSGK_NATIVE_RUNTIME", `D:\AIDEV\tree-sitter-grammar-kit\.work\352-data\s05\runtime`)
	}
}

func ordinaryFullPosProduce(t *testing.T, in ordinaryFullPosInput) (OrdinaryQuerySemanticsRequest, string) {
	t.Helper()
	inputPath, producerDir := ordinaryE2EWriteProducerInput(t, in.Producer)
	ordinaryE2ERunOwnedProducer(t, inputPath, producerDir)
	return ordinaryE2EAssemble(t, in.Request, producerDir, in.References, in.AssetBodies), producerDir
}

type ordinaryFullPosReplayEnv struct {
	exe     string
	repo    string
	scratch string
}

func ordinaryFullPosBuildCLI(t *testing.T) ordinaryFullPosReplayEnv {
	t.Helper()
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	scratch := t.TempDir()
	exe := filepath.Join(scratch, "tsgk.exe")
	build := exec.Command("go", "build", "-o", exe, "./src/cmd/tsgk")
	build.Dir = repo
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the CLI: %v\n%s", err, output)
	}
	return ordinaryFullPosReplayEnv{exe: exe, repo: repo, scratch: scratch}
}

// ordinaryFullPosParity runs the r9 replay in process and through `tsgk replay`, and requires that both
// match the API result tuple for tuple.
func ordinaryFullPosParity(t *testing.T, ctx context.Context, env ordinaryFullPosReplayEnv, req OrdinaryQuerySemanticsRequest, api OrdinaryQuerySemanticsResult) (inProcess, cli *OrdinaryQuerySemanticsResult) {
	t.Helper()
	root, profile := ordinaryCLIReplayProfile(t, req, api)
	replayed, err := Replay(ctx, ReplayRequest{Root: root, Profile: profile})
	if err != nil || replayed.OrdinaryQuerySemantics == nil || replayed.Assessment != api.Assessment || !validOrdinaryReplayResult(replayed) {
		t.Fatalf("r9 replay: err=%v result=%+v", err, replayed)
	}
	got := replayed.OrdinaryQuerySemantics
	if got.Semantic != api.Semantic || got.Code != api.Code || got.Tuple != api.Tuple || got.CoverageSHA256 != api.CoverageSHA256 || !slices.Equal(got.SelectedObligationIDs, api.SelectedObligationIDs) {
		t.Fatalf("API/replay parity changed: api=%+v replay=%+v", api, got)
	}
	profilePath := filepath.Join(env.scratch, "replay-profile-"+strconv.FormatInt(time.Now().UnixNano(), 10)+".json")
	if err := os.WriteFile(profilePath, profile, 0o600); err != nil {
		t.Fatal(err)
	}
	cliOut, cliErr, cliCode := ordinaryCLIRun(t, ctx, env.exe, "replay", "--input", root, "--profile", profilePath)
	var replayCLI ReplayResult
	if json.Unmarshal(cliOut, &replayCLI) != nil || replayCLI.OrdinaryQuerySemantics == nil {
		t.Fatalf("tsgk replay: exit=%d stderr=%s stdout=%.1000s", cliCode, cliErr, cliOut)
	}
	cv := replayCLI.OrdinaryQuerySemantics
	if replayCLI.Assessment != replayed.Assessment || cv.Semantic != api.Semantic || cv.Code != api.Code || cv.Tuple != api.Tuple || cv.CoverageSHA256 != api.CoverageSHA256 ||
		!slices.Equal(cv.SelectedObligationIDs, api.SelectedObligationIDs) || cv.BytesRead != got.BytesRead || cv.Work != got.Work || cv.LogicalBytes != got.LogicalBytes {
		t.Fatalf("CLI replay differs from the API result: cli=%+v api=%+v", cv, api)
	}
	return got, cv
}
