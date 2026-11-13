package kit

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// #427 P2: capacity measurement of one public ordinary pass per case. MEASUREMENT ONLY.
//
// The expected assets used here are copied from the native output of the same run ("self-expected"). That
// is exactly what the project forbids as evidence, and nothing here is evidence: no result is stored as a
// scope result, no tuple is counted as VERIFIED anywhere, and no limit, claim or workflow changes. The copy is
// needed because the product charges the pass ledger per decoded asset, tree node, capture and reference row;
// a pass without assets stops before most of those charges and cannot show their cost. The pass itself is the
// unchanged product pass (VerifyOrdinaryQuerySemantics) with its unchanged limits.

// ordinaryMeasureClosure is the closure encoding table of the #427 P3 generator (compiled constants there).
func ordinaryMeasureClosure(route string) string {
	switch route {
	case "cpp", "csharp", "kotlin", "postgresql-sql", "swift":
		return "parser-paged"
	case "tsql":
		return "binary-paged"
	}
	return "plain"
}

type ordinaryMeasureInputs struct {
	Req          OrdinaryQuerySemanticsRequest
	Profile      []byte
	Source       []byte
	Registration OrdinaryRegistration
	Plan         []byte
	Planning     []byte
	AssetReg     []byte
	GrammarRoot  string
	Pack         []byte
	Mapping      []byte
	Queries      []OracleQuery
	FactRef      *FactPackRef
	QualCase     QualCase
}

// ordinaryMeasureInputsFor builds the caller side of a one-case selected pass for any route.
func ordinaryMeasureInputsFor(t *testing.T, route, caseID string, assets OrdinaryAssetRegistration, closure string) ordinaryMeasureInputs {
	t.Helper()
	_, inventory := ordinaryP0ForPlanTest(t)
	workload := inventory.Routes[ordinaryRouteIndex(inventory, route)].Workload
	var qual *QualCase
	for i := range workload.Cases {
		if workload.Cases[i].ID == caseID {
			qual = &workload.Cases[i]
			break
		}
	}
	if qual == nil {
		t.Fatalf("case %s/%s is not in the qualification inventory", route, caseID)
	}
	source, ok := ordinaryRouteSources(t, route)[caseID]
	if !ok || uint64(len(source)) != qual.Input.Bytes || digestHex([]byte(source)) != qual.Input.SHA256 {
		t.Fatalf("source pin mismatch for %s/%s", route, caseID)
	}
	querySources := ordinaryRouteQueries(t, route)
	queries := make([]OracleQuery, 0, len(workload.Queries))
	for _, query := range workload.Queries {
		text, ok := querySources[query.ID]
		if !ok || digestHex([]byte(text)) != query.SHA256 {
			t.Fatalf("query pin mismatch for %s/%s", route, query.ID)
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
	mapping, err := os.ReadFile("../contracts/fact-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	var factRef *FactPackRef
	if _, has := ordinaryFactRoute(pack, route); has {
		factRef = &FactPackRef{Revision: pack.Revision, SHA256: pack.SHA256, Route: route}
	}
	cc := os.Getenv("TSGK_NATIVE_CC")
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
	}{Schema: OracleSchema, ID: "ordinary-measure", Route: route, Operation: workload.Operation, Symbol: workload.Symbol,
		Encoding: EncodingUTF8, Output: OutputTree, Compiler: compilerIdentity, Grammar: workload.Grammar, Declarations: workload.Declarations,
		Queries: queries, FactPack: factRef, API: true,
		Cases: []OracleCase{{IncrementalCase: IncrementalCase{ID: qual.ID, Input: NativeInput{Path: "cases/" + qual.ID + ".src", Role: "case", Bytes: qual.Input.Bytes, SHA256: qual.Input.SHA256},
			Encoding: EncodingUTF8, Edits: qual.Edits, Points: append([]NativePoint{}, qual.Points...), Expect: qual.Expect}, QueryExpect: []QueryExpectation{}}}}
	profileBytes := ordinaryJSON(t, profile)
	refLibrary := []byte("synthetic reference library; never loaded")
	refLibraryPin := OrdinaryPin{Path: "build/reference-library.bin", Bytes: uint64(len(refLibrary)), SHA256: digestHex(refLibrary)}
	planned := OrdinaryMember{Owner: "D_build", Path: refLibraryPin.Path, Role: "reference-library", CaseOrBatch: "build", Pin: refLibraryPin}
	req, registration, plan, _, selectedSource, _, _ := ordinarySelectedInput(t, route, caseID, assets, closure, true, profileBytes, planned)
	return ordinaryMeasureInputs{Req: req, Profile: profileBytes, Source: selectedSource, Registration: registration, Plan: ordinaryJSON(t, plan),
		Planning: req.PlanningProfile, AssetReg: req.AssetRegistration, GrammarRoot: ordinaryPreparedRoute(t, route), Pack: packBytes, Mapping: mapping,
		Queries: queries, FactRef: factRef, QualCase: *qual}
}

// ordinaryMeasureProduce runs the owned native producer once in dir and returns its wall time.
func ordinaryMeasureProduce(t *testing.T, dir string, in ordinaryMeasureInputs) (producerDir string, wall time.Duration) {
	t.Helper()
	return ordinaryMeasureProduceSources(t, dir, in, [][]byte{in.Source})
}

// ordinaryMeasureProduceSources runs the producer for the given initial sources (one per selected shard, in the
// order of the selected shard IDs).
func ordinaryMeasureProduceSources(t *testing.T, dir string, in ordinaryMeasureInputs, sources [][]byte) (producerDir string, wall time.Duration) {
	t.Helper()
	if _, err := os.Stat(dir); err == nil {
		t.Fatalf("preserving existing measurement evidence at %s", dir)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	producerDir = filepath.Join(dir, "owned-producer")
	producerInput := ordinaryE2EProducerInput{Profile: in.Profile, PlanningProfile: in.Planning, AssetRegistration: in.AssetReg,
		ShardPlan: in.Plan, RegistrationBytes: in.Req.Registration, Registration: in.Registration, ShardIDs: in.Req.Selection.ShardIDs,
		Sources: sources, GrammarRoot: in.GrammarRoot, FactPack: in.Pack, FactMapping: in.Mapping}
	if in.FactRef == nil {
		producerInput.FactPack, producerInput.FactMapping = nil, nil
	}
	inputPath := filepath.Join(dir, "producer-input.json")
	if err := os.WriteFile(inputPath, ordinaryJSON(t, producerInput), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "./src/internal/native", "-run", "^TestOracleOrdinaryE2EFixture$", "-count=1", "-timeout=700s")
	cmd.Dir = root
	tmp := filepath.Join(dir, "tmp")
	cmd.Env = ordinaryE2EEnv(map[string]string{"TSGK_ORDINARY_OWNED_INPUTS": inputPath, "TSGK_ORDINARY_OWNED_FIXTURE": producerDir,
		"TSGK_NATIVE_REQUIRED": "1", "TMP": tmp, "TEMP": tmp, "TMPDIR": tmp})
	start := time.Now()
	output, runErr := cmd.CombinedOutput()
	wall = time.Since(start)
	if err := os.WriteFile(filepath.Join(dir, "native-producer.log"), output, 0o600); err != nil {
		t.Fatal(err)
	}
	if runErr != nil {
		t.Fatalf("owned producer failed after %s: %v\n%s", wall, runErr, tail(output, 3000))
	}
	return producerDir, wall
}

func tail(b []byte, n int) string {
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return string(b)
}

// ordinaryMeasureActual checks the native raw response of the produced r8 and returns the checked trees,
// as ordinaryJudgeR8 does, plus the producer's step facts.
func ordinaryMeasureActual(t *testing.T, in ordinaryMeasureInputs, producerDir string) (nativewire.Checked, [][]byte, []byte) {
	t.Helper()
	return ordinaryMeasureActualAt(t, in, producerDir, 0)
}

// ordinaryMeasureActualAt is ordinaryMeasureActual for the idx-th case of the producer profile; in.Source is
// that case's initial source.
func ordinaryMeasureActualAt(t *testing.T, in ordinaryMeasureInputs, producerDir string, idx int) (nativewire.Checked, [][]byte, []byte) {
	t.Helper()
	profile, err := ParseOracleProfile(in.Profile)
	if err != nil || idx >= len(profile.Native.Cases) {
		t.Fatalf("profile: %v", err)
	}
	caseProfile := profile.Native.Cases[idx]
	r8 := filepath.Join(producerDir, "r8")
	read := func(rel string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(r8, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	response, err := nativewire.DecodeResponse(read(fmt.Sprintf("raw/%05d-%s.json", idx, caseProfile.ID)))
	if err != nil {
		t.Fatal(err)
	}
	versions, points, err := ApplyEdits(EncodingUTF8, in.Source, caseProfile.Edits, 65536)
	if err != nil {
		t.Fatal(err)
	}
	var observations []struct {
		RunID          string `json:"run_id"`
		ProducerSHA256 string `json:"producer_sha256"`
		HistoryStart   uint64 `json:"history_start"`
	}
	if err := json.Unmarshal(response.Steps[0].Observations, &observations); err != nil || len(observations) == 0 {
		t.Fatalf("native observations: %v", err)
	}
	expect := nativewire.Expect{Revision: nativewire.ProtocolR4, ID: caseProfile.ID, Nodes: 100000, API: profile.API, SourceBytes: uint64(len(in.Source)), EditPoints: points,
		Observation: &nativewire.Binding{Manifest: reuseobs.RuntimeManifestSHA256, RunID: observations[0].RunID, ProducerSHA256: observations[0].ProducerSHA256, HistoryStart: observations[0].HistoryStart}}
	for _, query := range profile.Queries {
		expect.Queries = append(expect.Queries, querywire.Source{ID: query.ID, Source: []byte(query.Source)})
	}
	for _, point := range caseProfile.Points {
		expect.Points = append(expect.Points, point.Byte)
	}
	for _, edit := range caseProfile.Edits {
		expect.Edits = append(expect.Edits, nativewire.EditSpan{StartByte: edit.StartByte, OldEndByte: edit.OldEndByte, NewEndByte: edit.NewEndByte})
	}
	for _, version := range versions {
		expect.Sources = append(expect.Sources, nativewire.SourceID{Bytes: uint64(len(version)), SHA256: digestHex(version)})
	}
	exit := 0
	expect.Exit = &exit
	checked, err := nativewire.Check(response, expect)
	if err != nil || len(checked.Steps) != len(versions) {
		t.Fatalf("native response does not check: %v", err)
	}
	var stepFacts []byte
	if in.FactRef != nil {
		var record struct {
			StepFacts json.RawMessage `json:"step_facts"`
		}
		if err := json.Unmarshal(read(fmt.Sprintf("records/%05d-%s.json", idx, caseProfile.ID)), &record); err == nil {
			stepFacts = record.StepFacts
		}
	}
	return checked, versions, stepFacts
}

type ordinaryMeasureStats struct {
	TreeRoles, Nodes, Captures, Matches, FactBodies, AssetBodies int
	ActualNotCompleted                                           int
	// per source step, incremental tree; captures and matches summed over the queries
	StepNodes, StepCaptures, StepMatches []int
	QueryCaptures                        map[string]int
}

// ordinaryMeasureAssets builds the self-expected asset registration from the checked native output.
func ordinaryMeasureAssets(t *testing.T, route string, in ordinaryMeasureInputs, checked nativewire.Checked, versions [][]byte) (OrdinaryAssetRegistration, map[string][]byte, ordinarySQLExpectedSet, map[string]QueryWitnessMetadata, ordinaryMeasureStats) {
	t.Helper()
	return ordinaryMeasureAssetsInfix(t, route, in, checked, versions, "")
}

// ordinaryMeasureAssetsInfix names the per-tuple assets with infix (the case ID plus "-" when several cases share
// one registration) so that their IDs do not collide.
func ordinaryMeasureAssetsInfix(t *testing.T, route string, in ordinaryMeasureInputs, checked nativewire.Checked, versions [][]byte, infix string) (OrdinaryAssetRegistration, map[string][]byte, ordinarySQLExpectedSet, map[string]QueryWitnessMetadata, ordinaryMeasureStats) {
	t.Helper()
	var stats ordinaryMeasureStats
	stats.StepNodes, stats.StepCaptures, stats.StepMatches = make([]int, len(versions)), make([]int, len(versions)), make([]int, len(versions))
	stats.QueryCaptures = map[string]int{}
	assets := OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Assets: []OrdinaryAsset{
		{ID: "m", Role: "metadata", State: "UNAVAILABLE"}, {ID: "t", Role: "expected-tree", State: "UNAVAILABLE"},
		{ID: "c", Role: "expected-captures", State: "UNAVAILABLE"}, {ID: "f", Role: "expected-facts", State: "UNAVAILABLE"}}}
	bodies := map[string][]byte{}
	add := func(id, role string, body []byte) {
		t.Helper()
		base := "expected/measure/" + id
		bodyPath, basisPath, reviewPath := base+".json", "basis/"+base+".txt", "review/"+base+".txt"
		basis := []byte("measurement-only copy of the native output; not evidence")
		review := []byte("measurement-only; no review")
		pin := OrdinaryPin{Path: bodyPath, Bytes: uint64(len(body)), SHA256: digestHex(body)}
		basisPin := OrdinaryPin{Path: basisPath, Bytes: uint64(len(basis)), SHA256: digestHex(basis)}
		reviewPin := OrdinaryPin{Path: reviewPath, Bytes: uint64(len(review)), SHA256: digestHex(review)}
		assets.Assets = append(assets.Assets, OrdinaryAsset{ID: id, Role: role, State: "PINNED", Pin: &pin, AuthoringBasis: &basisPin, IndependentReview: &reviewPin})
		bodies[bodyPath], bodies[basisPath], bodies[reviewPath] = body, basis, review
		stats.AssetBodies++
	}
	metadata := map[string]QueryWitnessMetadata{}
	trees := map[uint32][]TreeNode{}
	captureByKey := map[string]ordinaryExpectedCaptures{}
	pack, err := ParseFactPack(in.Pack)
	if err != nil {
		t.Fatal(err)
	}
	var packRoute FactPackRoute
	var hasFacts bool
	var mappingMeta struct {
		Schema   string `json:"schema"`
		Revision string `json:"revision"`
	}
	if in.FactRef != nil {
		packRoute, hasFacts = ordinaryFactRoute(pack, route)
		if err := json.Unmarshal(in.Mapping, &mappingMeta); err != nil {
			t.Fatal(err)
		}
	}
	for step, version := range versions {
		sourcePin := OrdinaryPin{Path: "source-witness/" + digestHex(version) + ".utf8", Bytes: uint64(len(version)), SHA256: digestHex(version)}
		roles := []string{"incremental"}
		if step > 0 {
			roles = append(roles, "fresh")
		}
		for _, role := range roles {
			actual := checked.Steps[step].Incremental
			if role == "fresh" {
				actual = *checked.Steps[step].Fresh
			}
			if actual.Status != StatusCompleted || actual.Form != "full" || actual.Nodes == nil {
				stats.ActualNotCompleted++
				continue
			}
			stats.TreeRoles++
			stats.Nodes += len(actual.Nodes)
			if role == "incremental" {
				trees[uint32(step)] = actual.Nodes
				stats.StepNodes[step] = len(actual.Nodes)
			}
			add(fmt.Sprintf("tsql-tree-%ss%d-%s", infix, step, role), "expected-tree",
				ordinaryJSON(t, ordinaryExpectedTree{Schema: ordinaryExpectedTreeSchema, Case: in.QualCase.ID, Step: uint32(step), TreeRole: role, Source: sourcePin, Tree: actual.Nodes}))
			for _, query := range in.Queries {
				q, ok := ordinaryActualQuery(actual, query.ID)
				if !ok || q.Status != StatusCompleted || q.Partial || q.Captures == nil {
					stats.ActualNotCompleted++
					continue
				}
				if _, seen := metadata[query.ID]; !seen {
					metadata[query.ID] = QueryWitnessMetadata{Patterns: q.Patterns, CaptureNames: q.CaptureNames, Predicates: q.Predicates}
					add("meta-"+query.ID, "metadata", ordinaryJSON(t, metadata[query.ID]))
				}
				stream, valid := ordinaryCaptureStream(q, actual.Nodes, version)
				if !valid {
					stats.ActualNotCompleted++
					continue
				}
				if stream == nil {
					stream = []Capture{}
				}
				stats.Captures += len(stream)
				stats.Matches += int(q.Matches)
				if role == "incremental" {
					stats.StepCaptures[step] += len(stream)
					stats.StepMatches[step] += int(q.Matches)
					stats.QueryCaptures[query.ID] += len(stream)
				}
				captures := ordinaryExpectedCaptures{Schema: ordinaryExpectedCaptureSchema, Case: in.QualCase.ID, Query: query.ID, Step: uint32(step), TreeRole: role, Source: sourcePin, Matches: q.Matches, Ordered: stream}
				if role == "incremental" {
					captureByKey[fmt.Sprintf("%s/%d", query.ID, step)] = captures
				}
				add(fmt.Sprintf("tsql-captures-%s%s-s%d-%s", infix, query.ID, step, role), "expected-captures", ordinaryJSON(t, captures))
				if !hasFacts {
					continue
				}
				fi := slices.IndexFunc(packRoute.Queries, func(f FactQuery) bool { return f.ID == query.ID })
				if fi < 0 {
					continue
				}
				fq := packRoute.Queries[fi]
				fact := ordinaryExpectedFact{Schema: ordinaryExpectedFactSchema, Case: in.QualCase.ID, Query: fq.ID, Step: uint32(step), TreeRole: role, Source: sourcePin,
					Pack: *in.FactRef, Mapping: ordinaryFactMappingPin{Schema: mappingMeta.Schema, Revision: mappingMeta.Revision, SHA256: digestHex(in.Mapping)},
					FactQuery: ordinaryFactQueryPin{ID: fq.ID, SHA256: digestHex([]byte(fq.Source)), Kind: fq.Facts}, Status: StatusCompleted}
				switch fq.Facts {
				case FactDeclarations:
					items := DeriveDeclarations(packRoute.Declarations, stream)
					fact.Declarations = &ordinaryExpectedDeclarations{Items: items, Assessment: ordinaryDeclarationAssessment(items)}
				case FactDynamicSQL:
					facts, err := DeriveDynamicSQLChecked(route, EncodingUTF8, stream, version)
					if err != nil {
						fact.Status, fact.Code = StatusFailed, "FACT_DERIVATION_FAILED"
					} else {
						fact.DynamicSQL = &facts
					}
				case FactXMLStructure:
					xml := make(map[string]int)
					for _, capture := range stream {
						xml[capture.Name]++
					}
					fact.XML = &xml
				default:
					t.Fatalf("unexpected fact kind %s", fq.Facts)
				}
				stats.FactBodies++
				add(fmt.Sprintf("tsql-facts-%s%s-s%d-%s", infix, fq.ID, step, role), "expected-facts", ordinaryJSON(t, fact))
			}
		}
	}
	expected := ordinarySQLExpectedSet{Tree: func(step uint32) []TreeNode { return trees[step] }, Captures: func(query string, step uint32) ([]Capture, uint64) {
		c := captureByKey[fmt.Sprintf("%s/%d", query, step)]
		return c.Ordered, c.Matches
	}}
	return assets, bodies, expected, metadata, stats
}

type ordinaryMeasureStage struct {
	Stage   string `json:"stage"`
	Logical uint64 `json:"logical"`
	Work    uint64 `json:"work"`
	Read    uint64 `json:"read"`
}

type ordinaryMeasurePass struct {
	WallMillis  int64                  `json:"wall_ms"`
	Error       string                 `json:"error,omitempty"`
	ErrorKind   string                 `json:"error_kind,omitempty"`
	ErrorCode   string                 `json:"error_code,omitempty"`
	Semantic    string                 `json:"semantic"`
	Complete    bool                   `json:"complete"`
	Code        string                 `json:"code,omitempty"`
	Required    int                    `json:"tuples_required"`
	Verified    int                    `json:"tuples_verified"`
	Failed      int                    `json:"tuples_failed"`
	Blocked     int                    `json:"tuples_blocked"`
	Unprocessed int                    `json:"tuples_unprocessed"`
	Logical     uint64                 `json:"logical"`
	Work        uint64                 `json:"work"`
	Read        uint64                 `json:"read"`
	Stages      []ordinaryMeasureStage `json:"stages"`
	LastStage   string                 `json:"last_stage"`
	Rejected    any                    `json:"rejected,omitempty"`
	Obligations map[string]int         `json:"obligations,omitempty"`
	Labels      map[string][4]uint64   `json:"labels,omitempty"`
	Decodes     map[string][2]int      `json:"decodes,omitempty"`
	Visits      map[string][2]uint64   `json:"visits,omitempty"`
}

// ordinaryMeasureRead parses a ledger file into the stage list and the exit record.
func ordinaryMeasureRead(t *testing.T, path string) (stages []ordinaryMeasureStage, rejected any, labels map[string][4]uint64, decodes map[string][2]int, visits map[string][2]uint64) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	for scanner.Scan() {
		var event struct {
			Kind    string               `json:"kind"`
			Stage   string               `json:"stage"`
			Logical uint64               `json:"logical"`
			Work    uint64               `json:"work"`
			Read    uint64               `json:"read"`
			Labels  map[string][4]uint64 `json:"labels"`
			Decodes map[string][2]int    `json:"decodes"`
			Visits  map[string][2]uint64 `json:"visits"`
		}
		line := scanner.Bytes()
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		switch event.Kind {
		case "stage":
			stages = append(stages, ordinaryMeasureStage{event.Stage, event.Logical, event.Work, event.Read})
			if event.Stage == "exit" {
				labels, decodes, visits = event.Labels, event.Decodes, event.Visits
			}
		case "rejected":
			var any1 any
			_ = json.Unmarshal(line, &any1)
			rejected = any1
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return
}

// ordinaryMeasurePublicPass runs the unchanged public pass once with the ledger hooks installed.
func ordinaryMeasurePublicPass(t *testing.T, req OrdinaryQuerySemanticsRequest, ledgerPath string) ordinaryMeasurePass {
	t.Helper()
	if err := os.WriteFile(ledgerPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ordinaryInstallLedgerHooks(t, ledgerPath)
	ctx, stop := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	defer stop()
	start := time.Now()
	result, verifyErr := VerifyOrdinaryQuerySemantics(ctx, req)
	pass := ordinaryMeasurePass{WallMillis: time.Since(start).Milliseconds(), Semantic: string(result.Semantic), Complete: result.Complete, Code: result.Code,
		Required: int(result.Tuple.Required), Verified: int(result.Tuple.Verified), Failed: int(result.Tuple.Failed), Blocked: int(result.Tuple.Blocked), Unprocessed: int(result.Tuple.Unprocessed),
		Logical: result.LogicalBytes, Work: result.Work, Read: result.BytesRead}
	if verifyErr != nil {
		e := asError(verifyErr)
		pass.Error, pass.ErrorKind, pass.ErrorCode = verifyErr.Error(), string(e.Kind), e.Code
	}
	pass.Obligations = map[string]int{}
	for _, o := range result.Obligations {
		pass.Obligations[o.State+":"+o.Code]++
	}
	stages, rejected, labels, decodes, visits := ordinaryMeasureRead(t, ledgerPath)
	pass.Stages, pass.Rejected, pass.Labels, pass.Decodes, pass.Visits = stages, rejected, labels, decodes, visits
	for _, s := range stages {
		if s.Stage != "exit" {
			pass.LastStage = s.Stage
		}
	}
	return pass
}

type ordinaryMeasureRecord struct {
	Route, Case, Closure string
	Steps, Edits         int
	SourceBytes          []int
	Tuples, FactTuples   int
	Stats                ordinaryMeasureStats
	References           int
	ProducerAWallMillis  int64
	ProducerBWallMillis  int64
	ProducerCWallMillis  int64
	Passes               []ordinaryMeasurePass
	Reproducible         bool
	ReproducibleDetail   string
	Limits               OrdinaryLimits
}

// ordinaryMeasureSame reports whether two passes of the same input recorded the same ledger.
func ordinaryMeasureSame(a, b ordinaryMeasurePass) (bool, string) {
	if !reflect.DeepEqual(a.Stages, b.Stages) {
		return false, "stage ledgers differ"
	}
	if a.Logical != b.Logical || a.Work != b.Work || a.Read != b.Read || a.Verified != b.Verified || a.Semantic != b.Semantic || a.ErrorCode != b.ErrorCode {
		return false, "pass totals or verdict differ"
	}
	// The sort's comparison count is an observer-only counter (ordinaryVisitHook); it varies between runs
	// while every charge is the same, so it is not part of the reproducibility comparison.
	va, vb := maps.Clone(a.Visits), maps.Clone(b.Visits)
	delete(va, "sort")
	delete(vb, "sort")
	if !reflect.DeepEqual(a.Labels, b.Labels) || !reflect.DeepEqual(a.Decodes, b.Decodes) || !reflect.DeepEqual(va, vb) {
		return false, "per-label charges differ"
	}
	return true, ""
}

func ordinaryMeasureOne(t *testing.T, outRoot, route, caseID string, repeats int) {
	t.Helper()
	dir := filepath.Join(outRoot, route, caseID)
	closure := ordinaryMeasureClosure(route)
	rec := ordinaryMeasureRecord{Route: route, Case: caseID, Closure: closure, Limits: OrdinaryPlanLimits()}
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
	defer save() // whatever stopped the run, the record so far is kept (a fatal runs deferred calls)

	// Phase A: placeholder assets, only to obtain the actual native output of this case.
	inA := ordinaryMeasureInputsFor(t, route, caseID, OrdinaryAssetRegistration{}, closure)
	producerA, wallA := ordinaryMeasureProduce(t, filepath.Join(dir, "phase-a"), inA)
	rec.ProducerAWallMillis = wallA.Milliseconds()
	checked, versions, _ := ordinaryMeasureActual(t, inA, producerA)
	rec.Steps, rec.Edits = len(versions), len(inA.QualCase.Edits)
	for _, v := range versions {
		rec.SourceBytes = append(rec.SourceBytes, len(v))
	}
	assets, bodies, expected, metadata, stats := ordinaryMeasureAssets(t, route, inA, checked, versions)
	rec.Stats = stats
	save()

	// Phase B: the registration pins the assets, so the native r8 is produced again for it. A second,
	// independent producer run (phase C) with its own run id repeats the measurement from scratch.
	inB := ordinaryMeasureInputsFor(t, route, caseID, assets, closure)
	references := ordinarySQLSyntheticReferences(t, inB.Profile, inB.Source, inB.Registration, expected, metadata)
	rec.References = len(references)
	runs := 2
	if os.Getenv("TSGK_MEASURE_SKIP_PHASE_C") != "" { // host-calibration runs: one producer run, both passes on its r8
		runs = 1
	}
	for run := 0; run < runs; run++ {
		producer, wall := ordinaryMeasureProduce(t, filepath.Join(dir, []string{"phase-b", "phase-c"}[run]), inB)
		if run == 0 {
			rec.ProducerBWallMillis = wall.Milliseconds()
		} else {
			rec.ProducerCWallMillis = wall.Milliseconds()
		}
		req := ordinaryE2EAssemble(t, inB.Req, producer, references, bodies)
		for i := 0; i < repeats-run; i++ {
			rec.Passes = append(rec.Passes, ordinaryMeasurePublicPass(t, req, filepath.Join(dir, fmt.Sprintf("public-ledger-%d.jsonl", len(rec.Passes)+1))))
			save()
		}
	}
	rec.Tuples = rec.Passes[0].Required
	rec.Reproducible, rec.ReproducibleDetail = true, ""
	for i := 1; i < len(rec.Passes); i++ {
		if same, detail := ordinaryMeasureSame(rec.Passes[0], rec.Passes[i]); !same {
			rec.Reproducible, rec.ReproducibleDetail = false, detail
		}
	}
	for _, p := range rec.Passes[:1] {
		t.Logf("%s/%s closure=%s steps=%d tuples=%d nodes=%d captures=%d refs=%d | semantic=%s verified=%d/%d logical=%d/%d work=%d/%d read=%d err=%s last=%s | producer A %dms B %dms pass %dms | reproducible=%t %s",
			route, caseID, closure, rec.Steps, rec.Tuples, stats.Nodes, stats.Captures, rec.References, p.Semantic, p.Verified, p.Required, p.Logical, rec.Limits.Logical, p.Work, rec.Limits.Work, p.Read, p.ErrorCode, p.LastStage,
			rec.ProducerAWallMillis, rec.ProducerBWallMillis, p.WallMillis, rec.Reproducible, rec.ReproducibleDetail)
	}
}

// TestOrdinaryMeasurePass measures one public ordinary pass per listed case of one route. Opt-in; run it in
// the HARD Job holding hardjob.lock. TSGK_MEASURE_SKIP_PHASE_C=1 skips the independent phase-C producer run.
func TestOrdinaryMeasurePass(t *testing.T) {
	out := os.Getenv("TSGK_MEASURE_OUT")
	route := os.Getenv("TSGK_MEASURE_ROUTE")
	cases := strings.Split(os.Getenv("TSGK_MEASURE_CASES"), ",")
	if out == "" || route == "" || cases[0] == "" {
		t.Skip("set TSGK_MEASURE_OUT, TSGK_MEASURE_ROUTE and TSGK_MEASURE_CASES")
	}
	ordinaryNativeTools(t)
	ordinaryPreparedRoutes(t)
	repeats := 2
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { ordinaryMeasureOne(t, filepath.Join(out, "measure"), route, c, repeats) })
	}
}
