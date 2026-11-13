package kit

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func ordinarySQLCapacityRequest(t *testing.T, routeName string) (OrdinaryQuerySemanticsRequest, string, string) {
	req, _, _, _, _, caseID, shardID := ordinarySelectedInput(t, routeName, "", OrdinaryAssetRegistration{}, "plain", false, nil)
	return req, caseID, shardID
}

// ordinarySelectedInput supplies exact source/query pins for one selected case. The rest of
// the large plan remains the existing source-only test fixture used by capacity probes.
func ordinarySelectedInput(t *testing.T, routeName, caseID string, assets OrdinaryAssetRegistration, closure string, exactSource bool, executable []byte, planned ...OrdinaryMember) (OrdinaryQuerySemanticsRequest, OrdinaryRegistration, OrdinaryShardPlan, OrdinaryProfile, []byte, string, string) {
	t.Helper()
	v, inventory := ordinaryP0ForPlanTest(t)
	plan := ordinaryPlanForTest(v, inventory)
	routeIndex := ordinaryRouteIndex(inventory, routeName)
	route := inventory.Routes[routeIndex]
	if caseID == "" {
		for _, shard := range plan.Shards {
			if shard.Route == route.Route && shard.Platform == ordinaryHostPlatform() {
				caseID = shard.Case
				break
			}
		}
	}
	if len(planned) == 0 {
		planned = []OrdinaryMember{}
	}
	var runtimePin OrdinaryPin
	if exactSource {
		runtimeManifest, err := drivers.NativeC.ReadFile("native-c/runtime-manifest.json")
		if err != nil {
			t.Fatal(err)
		}
		runtimePin = queryregistrycore.FilePin("runtime/runtime-manifest.json", runtimeManifest)
	}
	for i := range plan.Bindings {
		if plan.Bindings[i].Route == route.Route {
			plan.Bindings[i].ClosureEncoding = closure
			plan.Bindings[i].Members = append([]OrdinaryMember{}, planned...)
			if exactSource {
				plan.Bindings[i].Runtime = runtimePin
			}
		}
	}
	p := ordinaryProfileFixture()
	p.Candidate, p.SemanticCohort, p.Bindings = plan.Candidate, plan.SemanticCohort, plan.Bindings
	planningProfile := ordinaryJSON(t, p)
	if assets.Schema == "" {
		assets = OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Assets: []OrdinaryAsset{}}
		for _, asset := range []struct{ id, role string }{{"m", "metadata"}, {"t", "expected-tree"}, {"c", "expected-captures"}, {"f", "expected-facts"}} {
			assets.Assets = append(assets.Assets, OrdinaryAsset{ID: asset.id, Role: asset.role, State: "UNAVAILABLE"})
		}
	}
	assets.Registry, assets.Candidate, assets.SemanticCohort = plan.Registry, plan.Candidate, plan.SemanticCohort
	assetData := ordinaryJSON(t, assets)
	plan.Profile = queryregistrycore.FilePin("profile/ordinary.json", planningProfile)
	plan.Assets = queryregistrycore.FilePin("profile/assets.json", assetData)
	registration := OrdinaryRegistration{Schema: "tsgk-query-semantics-registration/r2", Registry: plan.Registry, Namespace: plan.Namespace,
		Inventory: queryregistrycore.QualificationPin, Slots: queryregistrycore.SlotPin, Profile: plan.Profile, Assets: plan.Assets,
		Route: route.Route, Set: route.Workload.Set, Platform: ordinaryHostPlatform(), Subject: ordinaryHostSubject(), MembershipContext: "qualification/r8",
		Context: OrdinaryContext, ClosureEncoding: closure, Candidate: plan.Candidate, SemanticCohort: plan.SemanticCohort, PlannedMembers: append([]OrdinaryMember{}, planned...)}
	selectedIndex := -1
	for i, shard := range plan.Shards {
		if shard.Route == route.Route && shard.Platform == registration.Platform {
			selectedIndex = i
			break
		}
	}
	if selectedIndex < 0 {
		t.Fatalf("no selected shard for %s", routeName)
	}
	selectedIndex = slices.IndexFunc(plan.Shards, func(shard OrdinaryPlanShard) bool {
		return shard.Route == route.Route && shard.Platform == registration.Platform && shard.Case == caseID
	})
	if selectedIndex < 0 {
		t.Fatalf("no selected shard for %s case %s", routeName, caseID)
	}
	sources := ordinaryRouteSources(t, routeName)
	queries := ordinaryRouteQueries(t, routeName)
	var selectedSource []byte
	var selectedStepPins []OrdinaryPin
	for i, original := range route.Workload.Cases {
		index := slices.IndexFunc(plan.Shards, func(shard OrdinaryPlanShard) bool {
			return shard.Case == original.ID && shard.Route == route.Route && shard.Platform == registration.Platform
		})
		if index < 0 {
			t.Fatalf("no shard for %s case %s", routeName, original.ID)
		}
		shard := &plan.Shards[index]
		shard.SourceHistory[0] = OrdinaryPlanHistoryStep{Bytes: original.Input.Bytes, SHA256: original.Input.SHA256}
		if exactSource {
			for step, size := range stepSizes(original.Input.Bytes, original.Edits) {
				if step < len(shard.SourceHistory) {
					shard.SourceHistory[step].Bytes = size
				}
			}
		}
		if exactSource && original.ID == caseID {
			text, ok := sources[caseID]
			if !ok {
				t.Fatalf("route source fixture has no case %s", caseID)
			}
			selectedSource = []byte(text)
			if uint64(len(selectedSource)) != original.Input.Bytes || digestHex(selectedSource) != original.Input.SHA256 {
				t.Fatalf("route source fixture pin mismatch for %s", caseID)
			}
			versions, points, err := ApplyEdits(EncodingUTF8, selectedSource, original.Edits, 65536)
			if err != nil {
				t.Fatal(err)
			}
			shard.SourceHistory = make([]OrdinaryPlanHistoryStep, len(versions))
			shard.EditPoints = points
			selectedStepPins = make([]OrdinaryPin, len(versions))
			for step, version := range versions {
				hash := digestHex(version)
				pin := OrdinaryPin{Path: "source-witness/" + hash + ".utf8", Bytes: uint64(len(version)), SHA256: hash}
				selectedStepPins[step] = pin
				shard.SourceHistory[step] = OrdinaryPlanHistoryStep{Bytes: pin.Bytes, SHA256: pin.SHA256}
			}
		}
		shard.Capacity = ordinaryShardCapacity(shard.SourceHistory, planned)
		c := OrdinaryCase{ID: original.ID, OriginalSelector: "qualification_inventory#/routes/" + strconv.Itoa(routeIndex) + "/workload/cases/" + strconv.Itoa(i),
			Edits: original.Edits, Points: shard.EditPoints}
		for _, version := range shard.SourceHistory {
			c.VersionPins = append(c.VersionPins, OrdinaryPin{Path: "source-witness/" + version.SHA256 + ".utf8", Bytes: version.Bytes, SHA256: version.SHA256})
		}
		c.InitialSource = c.VersionPins[0]
		if original.ID == caseID && exactSource {
			c.VersionPins, c.Points, c.InitialSource = selectedStepPins, shard.EditPoints, selectedStepPins[0]
		}
		registration.Cases = append(registration.Cases, c)
	}
	if selectedSource == nil && exactSource {
		t.Fatalf("selected case %s is absent from route %s", caseID, routeName)
	}
	for _, query := range route.Workload.Queries {
		querySource, ok := queries[query.ID]
		if exactSource && (!ok || digestHex([]byte(querySource)) != query.SHA256) {
			t.Fatalf("query fixture pin mismatch for %s/%s", routeName, query.ID)
		}
		queryBytes := uint64(1)
		if exactSource {
			queryBytes = uint64(len(querySource))
		}
		metadataAsset := "m"
		if exactSource && ordinaryRegisteredAsset(assets, "meta-"+query.ID, "metadata") {
			metadataAsset = "meta-" + query.ID
		}
		registration.Queries = append(registration.Queries, OrdinaryQuery{ID: query.ID,
			Source: OrdinaryPin{Path: "queries/" + query.ID, Bytes: queryBytes, SHA256: query.SHA256}, MetadataAsset: metadataAsset, Kind: queryregistrycore.Structural})
	}
	for _, coord := range v.Coordinates() {
		if coord.Kind != queryregistrycore.Structural || coord.Route != route.Route || coord.Platform != registration.Platform {
			continue
		}
		treeAsset, captureAsset := "t", "c"
		tupleSuffix := fmt.Sprintf("s%d-%s", coord.Step, coord.TreeRole)
		if exactSource && coord.Case == caseID {
			for _, candidate := range []string{"tree-selected", "tsql-tree-" + tupleSuffix} {
				if ordinaryRegisteredAsset(assets, candidate, "expected-tree") {
					treeAsset = candidate
				}
			}
			for _, candidate := range []string{"captures-" + coord.Query, "tsql-captures-" + coord.Query + "-" + tupleSuffix} {
				if ordinaryRegisteredAsset(assets, candidate, "expected-captures") {
					captureAsset = candidate
				}
			}
		}
		required := OrdinaryRequired{ID: coord.ID(registration.Namespace), Coordinate: coord, TreeAsset: treeAsset, CaptureAsset: captureAsset}
		if strings.HasPrefix(coord.Query, "facts.") {
			required.FactAsset = "f"
			if exactSource && coord.Case == caseID {
				for _, candidate := range []string{"fact-declarations", "tsql-facts-" + coord.Query + "-" + tupleSuffix} {
					if ordinaryRegisteredAsset(assets, candidate, "expected-facts") {
						required.FactAsset = candidate
					}
				}
			}
		}
		registration.Required = append(registration.Required, required)
	}
	registrationPath, profilePath, assetsPath := "registrations/"+routeName+"-"+ordinaryHostPlatform()+".json", "profile/ordinary.json", "profile/assets.json"
	if exactSource {
		registrationPath, profilePath, assetsPath = "registrations/"+routeName+"/"+ordinaryHostPlatform()+".json", "profile/tsgk-oracle-r4.json", "profile/ordinary-assets-r1.json"
	}
	plan.Registrations = []OrdinaryPin{}
	plan.Profile = queryregistrycore.FilePin(profilePath, planningProfile)
	plan.Assets = queryregistrycore.FilePin(assetsPath, assetData)
	registration.Profile, registration.Assets = plan.Profile, plan.Assets
	registrationData := ordinaryJSON(t, registration)
	plan.Registrations = []OrdinaryPin{queryregistrycore.FilePin(registrationPath, registrationData)}
	commitmentData, selection := ordinarySelectionForTest(t, plan, []int{selectedIndex})
	selection.Binding.ClosureEncoding = closure
	if len(executable) != 0 {
		selection.Binding.Profile = queryregistrycore.FilePin("profile/executable.json", executable)
	}
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	registry, qualification, slots := read("../contracts/query-semantic-registry-r1.json"), read("../contracts/qualification-c1.json"), read("../contracts/query-slot-inventory-r1.json")
	request := OrdinaryQuerySemanticsRequest{Selection: OrdinaryQuerySemanticsBinding(selection.Binding), PlanningProfile: planningProfile, Registration: registrationData,
		AssetRegistration: assetData, PlanCommitment: commitmentData, ShardProofs: selection.Proofs, Registry: registry, RegistrySHA256: ordinaryRegistrySHA,
		QualificationInventory: qualification, SlotInventory: slots}
	request.Profile = executable
	return request, registration, plan, p, selectedSource, plan.Shards[selectedIndex].Case, plan.Shards[selectedIndex].ID
}

func ordinaryRegisteredAsset(registration OrdinaryAssetRegistration, id, role string) bool {
	for _, asset := range registration.Assets {
		if asset.ID == id && asset.Role == role {
			return true
		}
	}
	return false
}

// ordinarySQLExpectedSet is the independent tree/capture expectation of the selected shard per source step.
type ordinarySQLExpectedSet struct {
	Tree     func(step uint32) []TreeNode
	Captures func(query string, step uint32) ([]Capture, uint64)
}

func ordinarySQLOwnedInputs(t *testing.T, route string, mutation *ordinaryTSQLMutation) (OrdinaryQuerySemanticsRequest, []byte, []byte, OrdinaryRegistration, []byte, []byte, []byte, string, []byte, []byte, map[string][]byte, ordinarySQLExpectedSet, map[string]QueryWitnessMetadata) {
	t.Helper()
	selected := map[string]string{"tsql": "alt-tsql-B01-r001", "postgresql-sql": "alt-postgresql-sql-B04-22"}[route]
	if selected == "" {
		t.Fatalf("unsupported SQL route %s", route)
	}
	_, inventory := ordinaryP0ForPlanTest(t)
	routeIndex := ordinaryRouteIndex(inventory, route)
	workload := inventory.Routes[routeIndex].Workload
	var qual *QualCase
	for i := range workload.Cases {
		if workload.Cases[i].ID == selected {
			qual = &workload.Cases[i]
			break
		}
	}
	if qual == nil {
		t.Fatalf("selected P0 case %s is missing", selected)
	}
	sources := ordinaryRouteSources(t, route)
	source, ok := sources[selected]
	if !ok || uint64(len(source)) != qual.Input.Bytes || digestHex([]byte(source)) != qual.Input.SHA256 {
		t.Fatalf("selected P0 source pin mismatch for %s", selected)
	}
	querySources := ordinaryRouteQueries(t, route)
	queries := make([]OracleQuery, 0, len(workload.Queries))
	for _, query := range workload.Queries {
		text, ok := querySources[query.ID]
		if !ok || digestHex([]byte(text)) != query.SHA256 {
			t.Fatalf("P0 query source pin mismatch for %s/%s", route, query.ID)
		}
		queries = append(queries, OracleQuery{ID: query.ID, Source: text})
	}
	packBytes, err := os.ReadFile("../contracts/fact-query-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := ParseFactPack(packBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("../contracts/fact-mapping.json"); err != nil {
		t.Fatal(err)
	}
	factMapping, err := os.ReadFile("../contracts/fact-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	cc := os.Getenv("TSGK_NATIVE_CC")
	if cc == "" {
		t.Fatal("TSGK_NATIVE_CC required for the SQL owned producer")
	}
	compiler, err := os.ReadFile(cc)
	if err != nil {
		t.Fatal(err)
	}
	compilerHash := sha256.Sum256(compiler)
	compilerIdentity := ToolIdentity{Name: "cc", Version: "configured-pinned-tool", SHA256: hex.EncodeToString(compilerHash[:]), Bytes: uint64(len(compiler))}
	profile := struct {
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
		Queries      []OracleQuery `json:"queries"`
		FactPack     *FactPackRef  `json:"fact_pack"`
		API          bool          `json:"api"`
		Cases        []OracleCase  `json:"cases"`
	}{Schema: OracleSchema, ID: "ordinary-sql-owned-e2e", Route: route, Operation: workload.Operation, Symbol: workload.Symbol,
		Encoding: EncodingUTF8, Output: OutputTree, Compiler: compilerIdentity, Grammar: workload.Grammar, Declarations: workload.Declarations,
		Queries: queries, FactPack: &FactPackRef{Revision: pack.Revision, SHA256: pack.SHA256, Route: route}, API: true,
		Cases: []OracleCase{{IncrementalCase: IncrementalCase{ID: qual.ID, Input: NativeInput{Path: "cases/" + qual.ID + ".sql", Role: "case", Bytes: qual.Input.Bytes, SHA256: qual.Input.SHA256},
			Encoding: EncodingUTF8, Edits: qual.Edits, Points: append([]NativePoint{}, qual.Points...), Expect: qual.Expect}, QueryExpect: []QueryExpectation{}}}}
	profileBytes := ordinaryJSON(t, profile)
	closure := map[string]string{"postgresql-sql": "parser-paged", "tsql": "binary-paged"}[route]
	refLibrary := []byte("synthetic reference library; never loaded")
	refLibraryPin := OrdinaryPin{Path: "build/reference-library.bin", Bytes: uint64(len(refLibrary)), SHA256: digestHex(refLibrary)}
	plannedReferenceLibrary := OrdinaryMember{Owner: "D_build", Path: refLibraryPin.Path, Role: "reference-library", CaseOrBatch: "build", Pin: refLibraryPin}
	assets := OrdinaryAssetRegistration{}
	assetBodies := map[string][]byte{}
	var expectedSet ordinarySQLExpectedSet
	var independentMetadata map[string]QueryWitnessMetadata
	switch route {
	case "postgresql-sql":
		var expectedTree []TreeNode
		assets, assetBodies, expectedTree, independentMetadata = ordinaryPostgreSQLDoAssets(t, selected, []byte(source), profileBytes)
		expectedSet = ordinarySQLExpectedSet{Tree: func(uint32) []TreeNode { return expectedTree }, Captures: func(string, uint32) ([]Capture, uint64) { return []Capture{}, 0 }}
	case "tsql":
		var expected ordinaryTSQLExpected
		assets, assetBodies, expected, independentMetadata = ordinaryTSQLAssets(t, selected, []byte(source), profileBytes, mutation)
		expectedSet = ordinarySQLExpectedSet{Tree: func(step uint32) []TreeNode { return expected.Trees[step] }, Captures: func(query string, step uint32) ([]Capture, uint64) {
			c := expected.Captures[tsqlKey(query, step)]
			return c.Ordered, c.Matches
		}}
	}
	planReq, registration, plan, _, selectedSource, _, _ := ordinarySelectedInput(t, route, selected, assets, closure, true, profileBytes, plannedReferenceLibrary)
	grammarRoot := ordinaryPreparedRoute(t, route)
	return planReq, profileBytes, selectedSource, registration, ordinaryJSON(t, plan), planReq.PlanningProfile, planReq.AssetRegistration, grammarRoot, packBytes, factMapping, assetBodies, expectedSet, independentMetadata
}

func ordinaryPostgreSQLDoAssets(t *testing.T, caseID string, source, profileBytes []byte) (OrdinaryAssetRegistration, map[string][]byte, []TreeNode, map[string]QueryWitnessMetadata) {
	t.Helper()
	ordinaryVerifyPostgreSQLPreparedGrammar(t)
	profile, err := ParseOracleProfile(profileBytes)
	if err != nil || len(profile.Native.Cases) != 1 || profile.Native.Cases[0].ID != caseID || len(profile.Native.Cases[0].Edits) != 0 {
		t.Fatalf("expected the fixed no-edit PostgreSQL DO case: err=%v", err)
	}
	versions, _, err := ApplyEdits(EncodingUTF8, source, nil, 65536)
	if err != nil || len(versions) != 1 || string(versions[0]) != "DO 'x';\n" {
		t.Fatalf("unexpected pinned PostgreSQL DO source: err=%v bytes=%q", err, source)
	}
	sourcePin := OrdinaryPin{Path: "source-witness/" + digestHex(source) + ".utf8", Bytes: uint64(len(source)), SHA256: digestHex(source)}
	metadataFile, err := os.ReadFile("../testdata/query-semantics/postgresql9-classification-anchors.json")
	if err != nil {
		t.Fatal(err)
	}
	var anchors struct {
		Queries []struct {
			ID       string               `json:"id"`
			Metadata QueryWitnessMetadata `json:"metadata"`
		} `json:"queries"`
	}
	if err := json.Unmarshal(metadataFile, &anchors); err != nil {
		t.Fatal(err)
	}
	metadata := make(map[string]QueryWitnessMetadata, len(anchors.Queries))
	for _, query := range anchors.Queries {
		metadata[query.ID] = query.Metadata
	}
	for _, query := range profile.Queries {
		value, ok := metadata[query.ID]
		if !ok || value.Patterns == 0 || value.CaptureNames == nil || value.Predicates == nil || len(value.Predicates) != 0 {
			t.Fatalf("independent PostgreSQL metadata missing for %s", query.ID)
		}
	}
	tree := ordinaryPostgreSQLDoTree(source)
	assetBodies := map[string][]byte{}
	assets := OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Assets: []OrdinaryAsset{
		{ID: "m", Role: "metadata", State: "UNAVAILABLE"}, {ID: "t", Role: "expected-tree", State: "UNAVAILABLE"},
		{ID: "c", Role: "expected-captures", State: "UNAVAILABLE"}, {ID: "f", Role: "expected-facts", State: "UNAVAILABLE"},
	}}
	add := func(id, role, basisText string, body []byte) {
		t.Helper()
		base := "expected/postgresql-do/" + id
		bodyPath, basisPath, reviewPath := base+".json", "basis/"+base+".txt", "review/"+base+".txt"
		basis := []byte(basisText)
		review := []byte("Fixture review: pins and tuple identity checked against the fixed PostgreSQL P0 route and grammar derivation.")
		pin := OrdinaryPin{Path: bodyPath, Bytes: uint64(len(body)), SHA256: digestHex(body)}
		basisPin := OrdinaryPin{Path: basisPath, Bytes: uint64(len(basis)), SHA256: digestHex(basis)}
		reviewPin := OrdinaryPin{Path: reviewPath, Bytes: uint64(len(review)), SHA256: digestHex(review)}
		assets.Assets = append(assets.Assets, OrdinaryAsset{ID: id, Role: role, State: "PINNED", Pin: &pin, AuthoringBasis: &basisPin, IndependentReview: &reviewPin})
		assetBodies[bodyPath], assetBodies[basisPath], assetBodies[reviewPath] = body, basis, review
	}
	for _, query := range profile.Queries {
		meta := metadata[query.ID]
		add("meta-"+query.ID, "metadata", "PostgreSQL9 static compiled-query metadata anchor; source SHA-256 is bound by the P0 query registry.", ordinaryJSON(t, meta))
		captures := ordinaryExpectedCaptures{Schema: ordinaryExpectedCaptureSchema, Case: caseID, Query: query.ID, Step: 0, TreeRole: "incremental", Source: sourcePin, Matches: 0, Ordered: []Capture{}}
		add("captures-"+query.ID, "expected-captures", "Hand-authored empty capture expectation: DO contains none of the registered PostgreSQL structural query node types.", ordinaryJSON(t, captures))
	}
	expectedTree := ordinaryExpectedTree{Schema: ordinaryExpectedTreeSchema, Case: caseID, Step: 0, TreeRole: "incremental", Source: sourcePin, Tree: tree}
	add("tree-selected", "expected-tree", "Hand-derived PostgreSQL DO syntax tree from DO 'x'; and the local prepared grammar productions and node type catalog.", ordinaryJSON(t, expectedTree))
	packBytes, err := os.ReadFile("../contracts/fact-query-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := ParseFactPack(packBytes)
	if err != nil {
		t.Fatal(err)
	}
	mappingBytes, err := os.ReadFile("../contracts/fact-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	ref := *profile.FactPack
	var mapping struct {
		Schema   string `json:"schema"`
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(mappingBytes, &mapping); err != nil {
		t.Fatal(err)
	}
	packRoute, ok := ordinaryFactRoute(pack, ref.Route)
	if !ok {
		t.Fatalf("PostgreSQL fact route missing from pack: %s", ref.Route)
	}
	factQueryIndex := slices.IndexFunc(packRoute.Queries, func(query FactQuery) bool { return query.ID == "facts.declarations" })
	if factQueryIndex < 0 {
		t.Fatal("PostgreSQL declarations fact query missing")
	}
	factQuery := packRoute.Queries[factQueryIndex]
	expectedFact := ordinaryExpectedFact{Schema: ordinaryExpectedFactSchema, Case: caseID, Query: factQuery.ID, Step: 0, TreeRole: "incremental", Source: sourcePin,
		Pack: ref, Mapping: ordinaryFactMappingPin{Schema: mapping.Schema, Revision: mapping.Revision, SHA256: digestHex(mappingBytes)},
		FactQuery: ordinaryFactQueryPin{ID: factQuery.ID, SHA256: digestHex([]byte(factQuery.Source)), Kind: FactDeclarations},
		Status:    StatusCompleted, Code: "", Declarations: &ordinaryExpectedDeclarations{Items: []DeclarationItem{}, Assessment: "NOT_APPLICABLE"}}
	add("fact-declarations", "expected-facts", "Hand-derived empty declaration facts: this DO statement contains no declaration nodes selected by the pinned PostgreSQL mapping.", ordinaryJSON(t, expectedFact))
	return assets, assetBodies, tree, metadata
}

func ordinaryVerifyPostgreSQLPreparedGrammar(t *testing.T) {
	t.Helper()
	contract, err := os.ReadFile("../contracts/reproduction-routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var routes struct {
		Routes []struct {
			Route            string `json:"route"`
			PrepareGenerated map[string]struct {
				Bytes  uint64 `json:"bytes"`
				SHA256 string `json:"sha256"`
			} `json:"prepare_generated"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(contract, &routes); err != nil {
		t.Fatal(err)
	}
	var pins map[string]struct {
		Bytes  uint64 `json:"bytes"`
		SHA256 string `json:"sha256"`
	}
	for _, route := range routes.Routes {
		if route.Route == "postgresql-sql" {
			pins = route.PrepareGenerated
			break
		}
	}
	if len(pins) == 0 {
		t.Fatal("pinned PostgreSQL generated grammar outputs are absent")
	}
	base := ordinaryPreparedRepro(t, "postgresql-sql")
	outputs := map[string][]byte{}
	for _, workspace := range []string{"workspace-a", "workspace-b"} {
		for _, name := range []string{"grammar.json", "node-types.json"} {
			data, err := os.ReadFile(filepath.Join(base, workspace, "out", name))
			if err != nil {
				t.Fatal(err)
			}
			pin, ok := pins[name]
			if !ok || uint64(len(data)) != pin.Bytes || digestHex(data) != pin.SHA256 {
				t.Fatalf("prepared PostgreSQL %s does not match its pinned output", name)
			}
			if prior := outputs[name]; prior != nil && !bytes.Equal(prior, data) {
				t.Fatalf("prepared PostgreSQL %s workspaces differ", name)
			}
			outputs[name] = data
		}
	}
	var grammar struct {
		Rules map[string]json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal(outputs["grammar.json"], &grammar); err != nil {
		t.Fatal(err)
	}
	for rule, symbols := range map[string][]string{"source_file": {"toplevel_stmt"}, "toplevel_stmt": {"stmt"}, "stmt": {"DoStmt"},
		"DoStmt": {"kw_do", "dostmt_opt_list"}, "dostmt_opt_list": {"dostmt_opt_item"}, "dostmt_opt_item": {"Sconst"}, "Sconst": {"string_literal"}} {
		data := grammar.Rules[rule]
		if len(data) == 0 {
			t.Fatalf("prepared PostgreSQL grammar omits rule %s", rule)
		}
		var normalized any
		if err := json.Unmarshal(data, &normalized); err != nil {
			t.Fatal(err)
		}
		canonical, err := json.Marshal(normalized)
		if err != nil {
			t.Fatal(err)
		}
		for _, symbol := range symbols {
			if !strings.Contains(string(canonical), `"name":"`+symbol+`"`) {
				t.Fatalf("prepared PostgreSQL grammar rule %s omits %s", rule, symbol)
			}
		}
		if rule == "source_file" && !strings.Contains(string(canonical), `"value":";"`) {
			t.Fatal("prepared PostgreSQL source_file production does not place a semicolon after toplevel_stmt")
		}
	}
	var nodes []struct {
		Type  string `json:"type"`
		Named bool   `json:"named"`
	}
	if err := json.Unmarshal(outputs["node-types.json"], &nodes); err != nil {
		t.Fatal(err)
	}
	wantNamed := map[string]bool{"source_file": true, "toplevel_stmt": true, "stmt": true, "DoStmt": true, "kw_do": true, "dostmt_opt_list": true,
		"dostmt_opt_item": true, "Sconst": true, "string_literal": true, ";": false}
	for _, node := range nodes {
		if named, ok := wantNamed[node.Type]; ok {
			if node.Named != named {
				t.Fatalf("prepared PostgreSQL node %s named=%t, want %t", node.Type, node.Named, named)
			}
			delete(wantNamed, node.Type)
		}
	}
	if len(wantNamed) != 0 {
		t.Fatalf("prepared PostgreSQL node-types omit expected DO syntax nodes: %v", wantNamed)
	}
}

// This preorder is hand-authored from the fixed DO source and the local prepared PostgreSQL grammar.json/node-types.json.
func ordinaryPostgreSQLDoTree(source []byte) []TreeNode {
	point := func(column uint32) Point { return Point{Row: 0, Column: column} }
	node := func(parent int64, typ string, named bool, start, end uint32) TreeNode {
		return TreeNode{Parent: parent, Type: typ, Named: named, StartByte: start, EndByte: end, StartPoint: point(start), EndPoint: point(end)}
	}
	return []TreeNode{
		{Parent: -1, Type: "source_file", Named: true, StartByte: 0, EndByte: uint32(len(source)), StartPoint: point(0), EndPoint: ordinaryE2EEndPoint(source)},
		node(0, "toplevel_stmt", true, 0, 6), node(1, "stmt", true, 0, 6), node(2, "DoStmt", true, 0, 6), node(3, "kw_do", true, 0, 2),
		node(3, "dostmt_opt_list", true, 3, 6), node(5, "dostmt_opt_item", true, 3, 6),
		node(6, "Sconst", true, 3, 6), node(7, "string_literal", true, 3, 6), node(0, ";", false, 6, 7),
	}
}

func ordinarySQLSyntheticReferences(t *testing.T, profileBytes, source []byte, registration OrdinaryRegistration, expected ordinarySQLExpectedSet, metadata map[string]QueryWitnessMetadata) []ordinaryE2EReference {
	t.Helper()
	profile, err := ParseOracleProfile(profileBytes)
	if err != nil || len(profile.Native.Cases) != 1 {
		t.Fatalf("SQL profile decode failed: %v", err)
	}
	versions, _, err := ApplyEdits(EncodingUTF8, source, profile.Native.Cases[0].Edits, 65536)
	if err != nil {
		t.Fatal(err)
	}
	sourceVersions := map[string][][]byte{profile.Native.Cases[0].ID: versions}
	querySources := map[string]string{}
	for _, query := range profile.Queries {
		querySources[query.ID] = query.Source
	}
	librarySHA := digestHex([]byte("synthetic reference library; never loaded"))
	rows := make([]ordinaryE2EReference, 0, len(ordinaryReferenceKeys(registration, sourceVersions)))
	for _, key := range ordinaryReferenceKeys(registration, sourceVersions) {
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			t.Fatalf("unexpected reference key %q", key)
		}
		step64, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil || int(step64) >= len(versions) {
			t.Fatalf("invalid source step in reference key %q", key)
		}
		query, ok := querySources[parts[2]]
		if !ok {
			t.Fatalf("unregistered query in reference key %q", key)
		}
		response := ordinaryE2ESyntheticResponse(t, parts[2], query, profile.Native.Symbol, librarySHA, versions[step64])
		if expected.Tree != nil {
			queryMetadata, ok := metadata[parts[2]]
			if !ok {
				t.Fatalf("independent metadata missing for synthetic reference %q", key)
			}
			captures, _ := expected.Captures(parts[2], uint32(step64))
			response = ordinaryE2ESyntheticResponseWithExpected(t, parts[2], query, profile.Native.Symbol, librarySHA, versions[step64], expected.Tree(uint32(step64)), queryMetadata, captures)
		}
		rows = append(rows, ordinaryE2EReference{Case: parts[0], Step: uint32(step64), Query: parts[2], Response: response})
	}
	return rows
}

// ordinarySQLEnv enforces the measurement environment shared by the opt-in SQL runs and
// returns a fresh artifact root under this worktree.
func ordinarySQLEnv(t *testing.T) string {
	t.Helper()
	if os.Getenv("TSGK_ORDINARY_SQL_E2E") != "1" {
		t.Skip("set TSGK_ORDINARY_SQL_E2E=1 to run the serialized SQL native capacity measurement")
	}
	ordinaryNativeTools(t)
	ordinaryPreparedRoutes(t)
	artifacts := os.Getenv("TSGK_ORDINARY_E2E_ARTIFACTS")
	if artifacts == "" {
		t.Fatal("TSGK_ORDINARY_E2E_ARTIFACTS must point to a directory for this run's evidence")
	}
	artifactRoot, err := filepath.Abs(artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(artifactRoot, 0700); err != nil {
		t.Fatal(err)
	}
	return artifactRoot
}

type ordinarySQLRouteRun struct {
	Result       OrdinaryQuerySemanticsResult
	Err          error
	Capacity     string
	LastStage    string
	LastRejected any
	AssetBodies  int
	TreeAssets   int // distinct expected-tree bodies of the registration
	References   int
	LedgerPath   string
	LogPath      string
}

// ordinarySQLProduced is the caller-side input and the owned native r8 of one route run.
type ordinarySQLProduced struct {
	Req          OrdinaryQuerySemanticsRequest
	Profile      []byte
	Source       []byte
	Registration OrdinaryRegistration
	AssetBodies  map[string][]byte
	Expected     ordinarySQLExpectedSet
	Metadata     map[string]QueryWitnessMetadata
	ProducerDir  string
	LogPath      string
	RouteDir     string
}

// ordinarySQLProduce authors the route's inputs and independent expectations and runs the
// owned native producer once in a fresh directory.
func ordinarySQLProduce(t *testing.T, routeDir, route string, mutation *ordinaryTSQLMutation) ordinarySQLProduced {
	t.Helper()
	if _, err := os.Stat(routeDir); err == nil {
		t.Fatalf("preserving existing capacity evidence at %s; choose a fresh artifact root", routeDir)
	}
	if err := os.MkdirAll(routeDir, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := filepath.Join(routeDir, "tmp")
	if err := os.Mkdir(tmpDir, 0700); err != nil {
		t.Fatal(err)
	}
	req, profile, source, registration, plan, planning, assets, grammarRoot, pack, mapping, assetBodies, expected, metadata := ordinarySQLOwnedInputs(t, route, mutation)
	producerDir := filepath.Join(routeDir, "owned-producer")
	producerInput := ordinaryE2EProducerInput{Profile: profile, PlanningProfile: planning, AssetRegistration: assets,
		ShardPlan: plan, RegistrationBytes: req.Registration, Registration: registration, ShardIDs: req.Selection.ShardIDs,
		Sources: [][]byte{source}, GrammarRoot: grammarRoot, FactPack: pack, FactMapping: mapping}
	inputBytes := ordinaryJSON(t, producerInput)
	inputPath := filepath.Join(routeDir, "producer-input.json")
	if err := os.WriteFile(inputPath, inputBytes, 0600); err != nil {
		t.Fatal(err)
	}
	cmdCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "go", "test", "./src/internal/native", "-run", "^TestOracleOrdinaryE2EFixture$", "-count=1", "-timeout=240s")
	cmd.Dir = root
	cmd.Env = ordinaryE2EEnv(map[string]string{"TSGK_ORDINARY_OWNED_INPUTS": inputPath, "TSGK_ORDINARY_OWNED_FIXTURE": producerDir,
		"TSGK_NATIVE_REQUIRED": "1", // the runtime and compiler are inherited as given
		"TMP":                  tmpDir, "TEMP": tmpDir, "TMPDIR": tmpDir})
	output, produceErr := cmd.CombinedOutput()
	logPath := filepath.Join(routeDir, "native-producer.log")
	if err := os.WriteFile(logPath, output, 0600); err != nil {
		t.Fatal(err)
	}
	if produceErr != nil {
		t.Fatalf("owned SQL producer failed: %v; log=%s\n%s", produceErr, logPath, output)
	}
	return ordinarySQLProduced{Req: req, Profile: profile, Source: source, Registration: registration, AssetBodies: assetBodies, Expected: expected, Metadata: metadata, ProducerDir: producerDir, LogPath: logPath, RouteDir: routeDir}
}

// ordinarySQLRunRoute runs the owned native producer and one public verification for a route
// in a fresh directory. The mutation, if any, changes an independent TSQL expectation before
// it is pinned; the native r8 is always produced for that run's own registration.
func ordinarySQLRunRoute(t *testing.T, routeDir, route string, mutation *ordinaryTSQLMutation) ordinarySQLRouteRun {
	t.Helper()
	produced := ordinarySQLProduce(t, routeDir, route, mutation)
	req, profile, source, registration, assetBodies, expected, metadata, producerDir, logPath := produced.Req, produced.Profile, produced.Source, produced.Registration, produced.AssetBodies, produced.Expected, produced.Metadata, produced.ProducerDir, produced.LogPath
	ledgerPath := filepath.Join(routeDir, "public-ledger.jsonl")
	if err := os.WriteFile(ledgerPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ordinaryInstallLedgerHooks(t, ledgerPath)
	references := ordinarySQLSyntheticReferences(t, profile, source, registration, expected, metadata)
	req = ordinaryE2EAssemble(t, req, producerDir, references, assetBodies)
	ctx, stop := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	defer stop()
	result, verifyErr := VerifyOrdinaryQuerySemantics(ctx, req)
	if err := os.WriteFile(filepath.Join(routeDir, "public-result.json"), ordinaryJSON(t, result), 0600); err != nil {
		t.Fatal(err)
	}
	var errorRecord any
	if verifyErr != nil {
		e := asError(verifyErr)
		errorRecord = struct {
			Kind string `json:"kind"`
			Code string `json:"code"`
			Path string `json:"path"`
		}{e.Kind, e.Code, e.Path}
		if e.Kind != KindResourceLimit && e.Kind != KindCancelled {
			t.Fatalf("ordinary SQL public call stopped before a capacity result: %v; result=%+v", verifyErr, result)
		}
	}
	stages, lastStage, lastRejected := ordinarySQLLedgerStages(t, ledgerPath)
	capacity := "CAPACITY_UNPROVEN"
	if len(assetBodies) != 0 && verifyErr == nil && result.Semantic == AssessPass && result.Complete {
		capacity = "FULL_BODY_MEASURED_VERIFIED"
	}
	stopBytes, err := json.Marshal(struct {
		Capacity, Boundary string
		Error              any
		LastRejected       any
		Stages             []string
	}{Capacity: capacity, Boundary: lastStage, Error: errorRecord, LastRejected: lastRejected, Stages: stages})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(routeDir, "capacity-stop.json"), stopBytes, 0600); err != nil {
		t.Fatal(err)
	}
	trees := map[string]bool{}
	for _, body := range assetBodies {
		if bytes.Contains(body, []byte(ordinaryExpectedTreeSchema)) {
			trees[digestHex(body)] = true
		}
	}
	return ordinarySQLRouteRun{Result: result, Err: verifyErr, Capacity: capacity, LastStage: lastStage, LastRejected: lastRejected,
		AssetBodies: len(assetBodies), TreeAssets: len(trees), References: len(references), LedgerPath: ledgerPath, LogPath: logPath}
}

func TestOrdinarySQLPublicPassCapacity(t *testing.T) {
	artifactRoot := ordinarySQLEnv(t)
	for _, route := range []string{"postgresql-sql", "tsql"} {
		t.Run(route, func(t *testing.T) {
			run := ordinarySQLRunRoute(t, filepath.Join(artifactRoot, "sql-"+route), route, nil)
			result, verifyErr := run.Result, run.Err
			ordinaryScopeRecord(t, "sql-"+route, result)
			limits := OrdinaryPlanLimits()
			if run.AssetBodies == 0 && (result.Tuple.Verified != 0 || result.Semantic == AssessPass) {
				t.Fatalf("synthetic SQL references without independent expected assets were incorrectly treated as verified: %+v", result)
			}
			if run.AssetBodies != 0 && result.Tuple.Failed != 0 {
				t.Fatalf("%s: an independently authored tuple FAILED against the native r8: %+v", route, result)
			}
			// A route with independently authored expected assets must finish with every required tuple VERIFIED;
			// a capacity stop is a failure here, not a measurement.
			if run.AssetBodies != 0 && verifyErr != nil {
				t.Fatalf("%s did not finish: err=%v capacity=%s stop_boundary=%s rejected=%v verified=%d/%d", route, verifyErr, run.Capacity, run.LastStage, run.LastRejected, result.Tuple.Verified, result.Tuple.Required)
			}
			if run.AssetBodies != 0 {
				if !result.Complete || !result.ProcessingComplete || result.BaseVerified.Assessment != AssessPass || result.Semantic != AssessPass || result.Assessment != AssessPass || result.Tuple.Required == 0 ||
					result.Tuple.Failed != 0 || result.Tuple.Verified != result.Tuple.Required || result.Tuple.Blocked != 0 {
					t.Fatalf("%s body adjudication did not finish with a recomputed base PASS: %+v", route, result)
				}
				for _, obligation := range result.Obligations {
					if obligation.State != ordinaryTupleVerified || obligation.Code != "" {
						t.Fatalf("%s independent tuple was not VERIFIED: %+v", route, obligation)
					}
				}
			}
			// Each distinct expected tree is decoded at most once per pass, for the judgment and the reference alike.
			decodes := ordinaryLedgerDecodes(t, run.LedgerPath)
			treeNew, treeShared := 0, 0
			for _, doc := range []string{"ordinary-expected-tree", "ordinary-reference-expected-tree"} {
				treeNew, treeShared = treeNew+decodes[doc][0], treeShared+decodes[doc][1]
			}
			if treeNew > run.TreeAssets {
				t.Fatalf("%s: %d expected-tree decodes for %d distinct trees (%v)", route, treeNew, run.TreeAssets, decodes)
			}
			t.Logf("route=%s expected-asset decodes {new, shared}: %v; distinct trees %d, tree decodes %d, tree shared uses %d", route, decodes, run.TreeAssets, treeNew, treeShared)
			// What the query-independent checks and the final-identity sort really did never exceeds what the
			// ledger charged for them.
			_, labels, visits := ordinaryLedgerExit(t, run.LedgerPath)
			for kind, label := range map[string]string{"sort": "ordinary-final-identity-order", "tree-check": "ordinary-actual-tree-check", "api-compare": "ordinary-api-compare", "tree-compare": "ordinary-tree-compare", "physical": "ordinary-final-physical-identity", "dir-sort": "semantic-directory-sort"} {
				if visits[kind][0] > labels[label][0] {
					t.Fatalf("%s: %s did %d visits for the %d work charged under %s", route, kind, visits[kind][0], labels[label][0], label)
				}
				t.Logf("route=%s %s: runs=%d visits=%d charged work=%d (%d charges)", route, kind, visits[kind][1], visits[kind][0], labels[label][0], labels[label][2])
			}
			if run.AssetBodies != 0 && visits["tree-check"][1] >= uint64(result.Tuple.Required) && result.Tuple.Required > 1 {
				t.Fatalf("%s: %d actual-tree checks for %d tuples, want one per distinct actual tree", route, visits["tree-check"][1], result.Tuple.Required)
			}
			expectedState := "UNAVAILABLE"
			if run.AssetBodies != 0 {
				expectedState = "independently-authored CST/query/fact fixtures"
			}
			t.Logf("route=%s selected=%v required=%d verified=%d failed=%d blocked=%d state=%s complete=%t capacity=%s logical=%d/%d work=%d/%d read=%d/%d stop_boundary=%s rejected=%v producer=owned-native reference=synthetic rows=%d expected=%s ledger=%s producer-log=%s",
				route, result.SelectedShardIDs, result.Tuple.Required, result.Tuple.Verified, result.Tuple.Failed, result.Tuple.Blocked, result.Semantic, result.Complete, run.Capacity, result.LogicalBytes, limits.Logical,
				result.Work, limits.Work, result.BytesRead, limits.ReadBytes, run.LastStage, run.LastRejected, run.References, expectedState, run.LedgerPath, run.LogPath)
		})
	}
}

func ordinarySQLLedgerStages(t *testing.T, path string) ([]string, string, any) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	stages := []string{}
	lastStage := ""
	var lastRejected any
	for scanner.Scan() {
		var event map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if event["kind"] == "stage" {
			stage, ok := event["stage"].(string)
			if !ok {
				t.Fatalf("ledger stage has no name: %v", event)
			}
			stages = append(stages, stage)
			if stage != "exit" {
				lastStage = stage
			}
		} else if event["kind"] == "rejected" {
			lastRejected = event
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(stages) == 0 || stages[len(stages)-1] != "exit" {
		t.Fatalf("ordinary ledger omitted the final boundary: stages=%v", stages)
	}
	return stages, lastStage, lastRejected
}

func ordinaryE2EEnv(overrides map[string]string) []string {
	values := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key := entry
		if index := strings.IndexByte(entry, '='); index >= 0 {
			key = entry[:index]
		}
		if _, replace := overrides[strings.ToUpper(key)]; !replace {
			values = append(values, entry)
		}
	}
	for key, value := range overrides {
		values = append(values, key+"="+value)
	}
	return values
}

func ordinaryRouteSources(t *testing.T, route string) map[string]string {
	t.Helper()
	var file struct {
		Cases []struct {
			ID     string `json:"id"`
			Source string `json:"source_utf8"`
		} `json:"cases"`
	}
	data, err := os.ReadFile("../testdata/native/routes/" + route + ".json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	result := make(map[string]string, len(file.Cases))
	for _, c := range file.Cases {
		result[c.ID] = c.Source
	}
	return result
}

func ordinaryRouteQueries(t *testing.T, route string) map[string]string {
	t.Helper()
	var file struct {
		Queries []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"queries"`
	}
	data, err := os.ReadFile("../testdata/native/queries/" + route + ".json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	result := make(map[string]string, len(file.Queries))
	for _, q := range file.Queries {
		result[q.ID] = q.Source
	}
	packBytes, err := os.ReadFile("../contracts/fact-query-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := ParseFactPack(packBytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, routePack := range pack.Routes {
		if routePack.Route == route {
			for _, query := range routePack.Queries {
				result[query.ID] = query.Source
			}
			break
		}
	}
	return result
}

func TestOrdinarySQLPassCapacityBeforeExternalApproval(t *testing.T) {
	t.Parallel()
	for _, route := range []string{"postgresql-sql", "tsql"} {
		t.Run(route, func(t *testing.T) {
			req, caseID, shardID := ordinarySQLCapacityRequest(t, route)
			ctx, cancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
			defer cancel()
			res, err := VerifyOrdinaryQuerySemantics(ctx, req)
			if err != nil {
				t.Fatalf("caller admission unexpectedly failed: %v; result=%+v", err, res)
			}
			if res.Code != "ELIGIBILITY_APPROVAL_MISSING" || res.Semantic != AssessBlocked || res.Tuple.Required == 0 ||
				res.Tuple.Blocked != res.Tuple.Required || res.Tuple.Unprocessed != 0 || res.Complete {
				t.Fatalf("missing external approval did not preserve selected denominator: %+v", res)
			}
			limits := OrdinaryPlanLimits()
			if res.LogicalBytes > limits.Logical || res.Work > limits.Work || res.BytesRead > limits.ReadBytes {
				t.Fatalf("selected caller pass exceeded an established cap: logical=%d work=%d read=%d", res.LogicalBytes, res.Work, res.BytesRead)
			}
			t.Logf("route=%s case=%s shard=%s required=%d stage=caller-admission-then-missing-approval logical=%d work=%d read=%d remaining_logical=%d remaining_work=%d remaining_read=%d; native/eligibility/evidence/expected/reference closure excluded",
				route, caseID, shardID, res.Tuple.Required, res.LogicalBytes, res.Work, res.BytesRead,
				limits.Logical-res.LogicalBytes, limits.Work-res.Work, limits.ReadBytes-res.BytesRead)
		})
	}
}
