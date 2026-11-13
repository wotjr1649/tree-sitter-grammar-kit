package kit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// #427 P2: several shards of one route in ONE public pass (MEASUREMENT ONLY, see query_ordinary_measure_test.go).
// The single-shard measurements put every per-shard cost into the route's fixed cost F; this pass shows whether
// the cost of k shards is F plus the sum of their own marginal costs, which the packing rule assumes.

// ordinarySelectedInputMulti is ordinarySelectedInput for several selected cases of one route. Assets of the
// selected cases are named with the case ID (ordinaryMeasureAssetsInfix).
func ordinarySelectedInputMulti(t *testing.T, routeName string, caseIDs []string, assets OrdinaryAssetRegistration, closure string, executable []byte, planned ...OrdinaryMember) (OrdinaryQuerySemanticsRequest, OrdinaryRegistration, OrdinaryShardPlan, map[string][]byte, []string) {
	t.Helper()
	v, inventory := ordinaryP0ForPlanTest(t)
	plan := ordinaryPlanForTest(v, inventory)
	routeIndex := ordinaryRouteIndex(inventory, routeName)
	route := inventory.Routes[routeIndex]
	if len(planned) == 0 {
		planned = []OrdinaryMember{}
	}
	runtimeManifest, err := drivers.NativeC.ReadFile("native-c/runtime-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	runtimePin := queryregistrycore.FilePin("runtime/runtime-manifest.json", runtimeManifest)
	for i := range plan.Bindings {
		if plan.Bindings[i].Route == route.Route {
			plan.Bindings[i].ClosureEncoding = closure
			plan.Bindings[i].Members = append([]OrdinaryMember{}, planned...)
			plan.Bindings[i].Runtime = runtimePin
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
	shardIndex := func(caseID string) int {
		return slices.IndexFunc(plan.Shards, func(shard OrdinaryPlanShard) bool {
			return shard.Route == route.Route && shard.Platform == registration.Platform && shard.Case == caseID
		})
	}
	var selectedIndices []int
	for _, id := range caseIDs {
		i := shardIndex(id)
		if i < 0 {
			t.Fatalf("no selected shard for %s case %s", routeName, id)
		}
		selectedIndices = append(selectedIndices, i)
	}
	slices.Sort(selectedIndices)
	sources, queries := ordinaryRouteSources(t, routeName), ordinaryRouteQueries(t, routeName)
	selectedSources := map[string][]byte{}
	selectedStepPins := map[string][]OrdinaryPin{}
	for i, original := range route.Workload.Cases {
		index := shardIndex(original.ID)
		if index < 0 {
			t.Fatalf("no shard for %s case %s", routeName, original.ID)
		}
		shard := &plan.Shards[index]
		shard.SourceHistory[0] = OrdinaryPlanHistoryStep{Bytes: original.Input.Bytes, SHA256: original.Input.SHA256}
		selected := slices.Contains(caseIDs, original.ID)
		if selected {
			text, ok := sources[original.ID]
			if !ok {
				t.Fatalf("route source fixture has no case %s", original.ID)
			}
			source := []byte(text)
			if uint64(len(source)) != original.Input.Bytes || digestHex(source) != original.Input.SHA256 {
				t.Fatalf("route source fixture pin mismatch for %s", original.ID)
			}
			selectedSources[original.ID] = source
			versions, points, err := ApplyEdits(EncodingUTF8, source, original.Edits, 65536)
			if err != nil {
				t.Fatal(err)
			}
			shard.SourceHistory = make([]OrdinaryPlanHistoryStep, len(versions))
			shard.EditPoints = points
			pins := make([]OrdinaryPin, len(versions))
			for step, version := range versions {
				hash := digestHex(version)
				pins[step] = OrdinaryPin{Path: "source-witness/" + hash + ".utf8", Bytes: uint64(len(version)), SHA256: hash}
				shard.SourceHistory[step] = OrdinaryPlanHistoryStep{Bytes: pins[step].Bytes, SHA256: pins[step].SHA256}
			}
			selectedStepPins[original.ID] = pins
		}
		shard.Capacity = ordinaryShardCapacity(shard.SourceHistory, planned)
		c := OrdinaryCase{ID: original.ID, OriginalSelector: "qualification_inventory#/routes/" + strconv.Itoa(routeIndex) + "/workload/cases/" + strconv.Itoa(i),
			Edits: original.Edits, Points: shard.EditPoints}
		for _, version := range shard.SourceHistory {
			c.VersionPins = append(c.VersionPins, OrdinaryPin{Path: "source-witness/" + version.SHA256 + ".utf8", Bytes: version.Bytes, SHA256: version.SHA256})
		}
		c.InitialSource = c.VersionPins[0]
		if selected {
			c.VersionPins, c.InitialSource = selectedStepPins[original.ID], selectedStepPins[original.ID][0]
		}
		registration.Cases = append(registration.Cases, c)
	}
	for _, query := range route.Workload.Queries {
		querySource, ok := queries[query.ID]
		if !ok || digestHex([]byte(querySource)) != query.SHA256 {
			t.Fatalf("query fixture pin mismatch for %s/%s", routeName, query.ID)
		}
		metadataAsset := "m"
		if ordinaryRegisteredAsset(assets, "meta-"+query.ID, "metadata") {
			metadataAsset = "meta-" + query.ID
		}
		registration.Queries = append(registration.Queries, OrdinaryQuery{ID: query.ID,
			Source: OrdinaryPin{Path: "queries/" + query.ID, Bytes: uint64(len(querySource)), SHA256: query.SHA256}, MetadataAsset: metadataAsset, Kind: queryregistrycore.Structural})
	}
	for _, coord := range v.Coordinates() {
		if coord.Kind != queryregistrycore.Structural || coord.Route != route.Route || coord.Platform != registration.Platform {
			continue
		}
		treeAsset, captureAsset := "t", "c"
		suffix := fmt.Sprintf("s%d-%s", coord.Step, coord.TreeRole)
		selected := slices.Contains(caseIDs, coord.Case)
		if selected {
			if id := "tsql-tree-" + coord.Case + "-" + suffix; ordinaryRegisteredAsset(assets, id, "expected-tree") {
				treeAsset = id
			}
			if id := "tsql-captures-" + coord.Case + "-" + coord.Query + "-" + suffix; ordinaryRegisteredAsset(assets, id, "expected-captures") {
				captureAsset = id
			}
		}
		required := OrdinaryRequired{ID: coord.ID(registration.Namespace), Coordinate: coord, TreeAsset: treeAsset, CaptureAsset: captureAsset}
		if strings.HasPrefix(coord.Query, "facts.") {
			required.FactAsset = "f"
			if selected {
				if id := "tsql-facts-" + coord.Case + "-" + coord.Query + "-" + suffix; ordinaryRegisteredAsset(assets, id, "expected-facts") {
					required.FactAsset = id
				}
			}
		}
		registration.Required = append(registration.Required, required)
	}
	registrationPath, profilePath, assetsPath := "registrations/"+routeName+"/"+ordinaryHostPlatform()+".json", "profile/tsgk-oracle-r4.json", "profile/ordinary-assets-r1.json"
	plan.Registrations = []OrdinaryPin{}
	plan.Profile = queryregistrycore.FilePin(profilePath, planningProfile)
	plan.Assets = queryregistrycore.FilePin(assetsPath, assetData)
	registration.Profile, registration.Assets = plan.Profile, plan.Assets
	registrationData := ordinaryJSON(t, registration)
	plan.Registrations = []OrdinaryPin{queryregistrycore.FilePin(registrationPath, registrationData)}
	commitmentData, selection := ordinarySelectionForTest(t, plan, selectedIndices)
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
	return request, registration, plan, selectedSources, selection.Binding.ShardIDs
}

// ordinaryMeasureMultiInputs is the caller side of one pass over several cases of a route.
type ordinaryMeasureMultiInputs struct {
	Route      string
	Closure    string
	CaseIDs    []string
	Single     []ordinaryMeasureInputs // per-case view (QualCase, Source, shared profile/queries/pack)
	Req        OrdinaryQuerySemanticsRequest
	Profile    []byte
	Registered OrdinaryRegistration
	Plan       []byte
	GrammarDir string
	Pack, Map  []byte
	FactRef    *FactPackRef
	ShardIDs   []string
	Sources    map[string][]byte
}

func ordinaryMeasureMultiFor(t *testing.T, route string, caseIDs []string, assets OrdinaryAssetRegistration, closure string) ordinaryMeasureMultiInputs {
	t.Helper()
	_, inventory := ordinaryP0ForPlanTest(t)
	workload := inventory.Routes[ordinaryRouteIndex(inventory, route)].Workload
	// the profile cases and their sources follow the registration's case order (the workload order: the producer
	// refuses another one, ORDINARY_CASE_BINDING); the selection follows the plan's shard order (shard ID order)
	caseIDs = slices.Clone(caseIDs)
	slices.SortFunc(caseIDs, func(a, b string) int {
		return slices.IndexFunc(workload.Cases, func(c QualCase) bool { return c.ID == a }) - slices.IndexFunc(workload.Cases, func(c QualCase) bool { return c.ID == b })
	})
	querySources := ordinaryRouteQueries(t, route)
	queries := make([]OracleQuery, 0, len(workload.Queries))
	for _, query := range workload.Queries {
		queries = append(queries, OracleQuery{ID: query.ID, Source: querySources[query.ID]})
	}
	packBytes, err := os.ReadFile("../contracts/fact-query-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := ParseFactPack(packBytes)
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := os.ReadFile("../contracts/fact-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	var factRef *FactPackRef
	if _, has := ordinaryFactRoute(pack, route); has {
		factRef = &FactPackRef{Revision: pack.Revision, SHA256: pack.SHA256, Route: route}
	}
	compiler, err := os.ReadFile(os.Getenv("TSGK_NATIVE_CC"))
	if err != nil {
		t.Fatal(err)
	}
	compilerHash := sha256.Sum256(compiler)
	compilerIdentity := ToolIdentity{Name: "cc", Version: "configured-pinned-tool", SHA256: hex.EncodeToString(compilerHash[:]), Bytes: uint64(len(compiler))}
	sources := ordinaryRouteSources(t, route)
	var oracleCases []OracleCase
	var quals []QualCase
	for _, id := range caseIDs {
		var qual *QualCase
		for i := range workload.Cases {
			if workload.Cases[i].ID == id {
				qual = &workload.Cases[i]
			}
		}
		if qual == nil {
			t.Fatalf("case %s/%s is not in the inventory", route, id)
		}
		if uint64(len(sources[id])) != qual.Input.Bytes || digestHex([]byte(sources[id])) != qual.Input.SHA256 {
			t.Fatalf("source pin mismatch for %s/%s", route, id)
		}
		quals = append(quals, *qual)
		oracleCases = append(oracleCases, OracleCase{IncrementalCase: IncrementalCase{ID: qual.ID, Input: NativeInput{Path: "cases/" + qual.ID + ".src", Role: "case", Bytes: qual.Input.Bytes, SHA256: qual.Input.SHA256},
			Encoding: EncodingUTF8, Edits: qual.Edits, Points: append([]NativePoint{}, qual.Points...), Expect: qual.Expect}, QueryExpect: []QueryExpectation{}})
	}
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
	}{Schema: OracleSchema, ID: "ordinary-measure", Route: route, Operation: workload.Operation, Symbol: workload.Symbol,
		Encoding: EncodingUTF8, Output: OutputTree, Compiler: compilerIdentity, Grammar: workload.Grammar, Declarations: workload.Declarations,
		Queries: queries, FactPack: factRef, API: true, Cases: oracleCases}
	profileBytes := ordinaryJSON(t, profile)
	refLibrary := []byte("synthetic reference library; never loaded")
	refLibraryPin := OrdinaryPin{Path: "build/reference-library.bin", Bytes: uint64(len(refLibrary)), SHA256: digestHex(refLibrary)}
	planned := OrdinaryMember{Owner: "D_build", Path: refLibraryPin.Path, Role: "reference-library", CaseOrBatch: "build", Pin: refLibraryPin}
	req, registration, plan, selected, shardIDs := ordinarySelectedInputMulti(t, route, caseIDs, assets, closure, profileBytes, planned)
	out := ordinaryMeasureMultiInputs{Route: route, Closure: closure, CaseIDs: caseIDs, Req: req, Profile: profileBytes, Registered: registration, Plan: ordinaryJSON(t, plan),
		GrammarDir: ordinaryPreparedRoute(t, route), Pack: packBytes, Map: mapping, FactRef: factRef, ShardIDs: shardIDs, Sources: selected}
	for i, id := range caseIDs {
		out.Single = append(out.Single, ordinaryMeasureInputs{Req: req, Profile: profileBytes, Source: selected[id], Registration: registration, Queries: queries, FactRef: factRef,
			Pack: packBytes, Mapping: mapping, QualCase: quals[i]})
	}
	return out
}

type ordinaryMeasurePackCase struct {
	Case        string
	SourceBytes []int
	Stats       ordinaryMeasureStats
	Tuples      int
}

type ordinaryMeasurePackRecord struct {
	Route, Closure      string
	Cases               []ordinaryMeasurePackCase
	ProducerAWallMillis int64
	ProducerBWallMillis int64
	References          int
	Passes              []ordinaryMeasurePass
	Reproducible        bool
	Limits              OrdinaryLimits
}

func ordinaryMeasurePackOne(t *testing.T, outRoot, route string, caseIDs []string, group int) {
	t.Helper()
	dir := filepath.Join(outRoot, route, fmt.Sprintf("group-%02d", group))
	closure := ordinaryMeasureClosure(route)
	rec := ordinaryMeasurePackRecord{Route: route, Closure: closure, Limits: OrdinaryPlanLimits()}
	save := func() {
		data, err := json.MarshalIndent(rec, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "measure.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	defer save()
	produce := func(name string, m ordinaryMeasureMultiInputs) (string, int64) {
		sources := make([][]byte, len(m.ShardIDs))
		// the producer takes the sources in the order of the selected shard IDs, which follow the case list
		for i, id := range m.CaseIDs {
			sources[i] = m.Sources[id]
		}
		in := ordinaryMeasureInputs{Req: m.Req, Profile: m.Profile, Registration: m.Registered, Plan: m.Plan, Planning: m.Req.PlanningProfile, AssetReg: m.Req.AssetRegistration,
			GrammarRoot: m.GrammarDir, Pack: m.Pack, Mapping: m.Map, FactRef: m.FactRef}
		return ordinaryMeasureProduceMulti(t, filepath.Join(dir, name), in, sources)
	}
	// Phase A: placeholder assets, the native output of every case.
	mA := ordinaryMeasureMultiFor(t, route, caseIDs, OrdinaryAssetRegistration{}, closure)
	caseIDs = mA.CaseIDs
	producerA, wallA := produce("phase-a", mA)
	rec.ProducerAWallMillis = wallA
	merged := OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Assets: []OrdinaryAsset{}}
	bodies := map[string][]byte{}
	var references []ordinaryE2EReference
	have := map[string]bool{}
	type caseExpect struct {
		in        ordinaryMeasureInputs
		expected  ordinarySQLExpectedSet
		metadata  map[string]QueryWitnessMetadata
		versions  [][]byte
		stepFacts []byte
	}
	perCase := map[string]caseExpect{}
	for i, id := range caseIDs {
		checked, versions, facts := ordinaryMeasureActualAt(t, mA.Single[i], producerA, i)
		assets, b, expected, metadata, stats := ordinaryMeasureAssetsInfix(t, route, mA.Single[i], checked, versions, id+"-")
		for _, a := range assets.Assets {
			if !have[a.ID] {
				have[a.ID] = true
				merged.Assets = append(merged.Assets, a)
			}
		}
		for k, v := range b {
			bodies[k] = v
		}
		pc := ordinaryMeasurePackCase{Case: id, Stats: stats}
		for _, v := range versions {
			pc.SourceBytes = append(pc.SourceBytes, len(v))
		}
		rec.Cases = append(rec.Cases, pc)
		perCase[id] = caseExpect{mA.Single[i], expected, metadata, versions, facts}
	}
	save()
	// Phase B: the registration pins the merged assets; the producer runs again for it.
	mB := ordinaryMeasureMultiFor(t, route, caseIDs, merged, closure)
	for i, id := range caseIDs {
		ce := perCase[id]
		ce.in.Registration = mB.Registered
		sub := ordinarySQLSyntheticReferencesFor(t, mB.Profile, id, ce.in.Source, mB.Registered, ce.expected, ce.metadata)
		references = append(references, sub...)
		_ = i
	}
	// the reference rows follow the registration's required-coordinate order across all selected cases
	versionsByCase := map[string][][]byte{}
	for id, ce := range perCase {
		versionsByCase[id] = ce.versions
	}
	byKey := map[string]ordinaryE2EReference{}
	for _, row := range references {
		byKey[ordinaryReferenceKey(row.Case, row.Step, row.Query)] = row
	}
	references = references[:0]
	parsedRegistration, err := ParseOrdinaryRegistration(mB.Req.Registration) // the verifier orders by the parsed registration
	if err != nil {
		t.Fatal(err)
	}
	// the verifier's registration lists the cases in plan order (shard ID order), each case in coordinate order
	sortedCases := slices.Sorted(maps.Keys(versionsByCase))
	for _, id := range sortedCases {
		for _, key := range ordinaryReferenceKeys(parsedRegistration, map[string][][]byte{id: versionsByCase[id]}) {
			references = append(references, byKey[key])
		}
	}
	rec.References = len(references)
	producerB, wallB := produce("phase-b", mB)
	rec.ProducerBWallMillis = wallB
	if raw, err := os.ReadFile(filepath.Join(producerB, "r8", "manifest.json")); err == nil {
		var manifest OrdinaryOracleManifest
		if json.Unmarshal(raw, &manifest) == nil {
			t.Logf("selection shard IDs %v; manifest shard IDs %v", mB.ShardIDs, manifest.ShardIDs)
		}
	}
	req := ordinaryE2EAssemble(t, mB.Req, producerB, references, bodies)
	for i := 0; i < 2; i++ {
		rec.Passes = append(rec.Passes, ordinaryMeasurePublicPass(t, req, filepath.Join(dir, fmt.Sprintf("public-ledger-%d.jsonl", i+1))))
		save()
	}
	rec.Reproducible = true
	if same, _ := ordinaryMeasureSame(rec.Passes[0], rec.Passes[1]); !same {
		rec.Reproducible = false
	}
	for i := range rec.Cases {
		rec.Cases[i].Tuples = 0
	}
	p := rec.Passes[0]
	t.Logf("obligations %v code=%s", p.Obligations, p.Code)
	t.Logf("%s group %d cases=%d | semantic=%s verified=%d/%d logical=%d work=%d read=%d err=%s last=%s | producer A %dms B %dms pass %dms reproducible=%t",
		route, group, len(caseIDs), p.Semantic, p.Verified, p.Required, p.Logical, p.Work, p.Read, p.ErrorCode, p.LastStage, rec.ProducerAWallMillis, rec.ProducerBWallMillis, p.WallMillis, rec.Reproducible)
}

// ordinaryMeasureProduceMulti is ordinaryMeasureProduce for several sources.
func ordinaryMeasureProduceMulti(t *testing.T, dir string, in ordinaryMeasureInputs, sources [][]byte) (string, int64) {
	t.Helper()
	producerDir, wall := ordinaryMeasureProduceSources(t, dir, in, sources)
	return producerDir, wall.Milliseconds()
}

// ordinarySQLSyntheticReferencesFor is ordinarySQLSyntheticReferences for the case caseID of a multi-case profile.
func ordinarySQLSyntheticReferencesFor(t *testing.T, profileBytes []byte, caseID string, source []byte, registration OrdinaryRegistration, expected ordinarySQLExpectedSet, metadata map[string]QueryWitnessMetadata) []ordinaryE2EReference {
	t.Helper()
	profile, err := ParseOracleProfile(profileBytes)
	if err != nil {
		t.Fatal(err)
	}
	var edits []Edit
	for _, c := range profile.Native.Cases {
		if c.ID == caseID {
			edits = c.Edits
		}
	}
	versions, _, err := ApplyEdits(EncodingUTF8, source, edits, 65536)
	if err != nil {
		t.Fatal(err)
	}
	querySources := map[string]string{}
	for _, query := range profile.Queries {
		querySources[query.ID] = query.Source
	}
	librarySHA := digestHex([]byte("synthetic reference library; never loaded"))
	var rows []ordinaryE2EReference
	for _, key := range ordinaryReferenceKeys(registration, map[string][][]byte{caseID: versions}) {
		parts := strings.Split(key, "/")
		step64, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil || int(step64) >= len(versions) {
			t.Fatalf("invalid reference key %q", key)
		}
		queryMetadata := metadata[parts[2]]
		captures, _ := expected.Captures(parts[2], uint32(step64))
		response := ordinaryE2ESyntheticResponseWithExpected(t, parts[2], querySources[parts[2]], profile.Native.Symbol, librarySHA, versions[step64], expected.Tree(uint32(step64)), queryMetadata, captures)
		rows = append(rows, ordinaryE2EReference{Case: parts[0], Step: uint32(step64), Query: parts[2], Response: response})
	}
	return rows
}

// TestOrdinaryMeasurePack measures one public pass per group of cases ("a+b+c;d+e") of one route.
func TestOrdinaryMeasurePack(t *testing.T) {
	out, route, packs := os.Getenv("TSGK_MEASURE_OUT"), os.Getenv("TSGK_MEASURE_ROUTE"), os.Getenv("TSGK_MEASURE_PACKS")
	if out == "" || route == "" || packs == "" {
		t.Skip("set TSGK_MEASURE_OUT, TSGK_MEASURE_ROUTE and TSGK_MEASURE_PACKS")
	}
	ordinaryNativeTools(t)
	ordinaryPreparedRoutes(t)
	for g, group := range strings.Split(packs, ";") {
		ids := strings.Split(group, "+")
		t.Run(fmt.Sprintf("group-%02d", g), func(t *testing.T) { ordinaryMeasurePackOne(t, filepath.Join(out, "pack"), route, ids, g) })
	}
	_ = context.Background
}
