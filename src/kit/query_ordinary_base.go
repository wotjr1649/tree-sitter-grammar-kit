package kit

import (
	"encoding/json/jsontext"
	"math/bits"
	"reflect"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

type ordinaryBaseResult struct {
	Verdict  Verdict
	Gates    []ReplayGate
	Findings []Finding
}

type ordinaryBaseProjection struct {
	ID                string                       `json:"id"`
	Input             NativeInput                  `json:"input"`
	Encoding          string                       `json:"encoding"`
	Status            string                       `json:"execution_status"`
	Oracle            jsontext.Value               `json:"oracle_claims,omitempty"`
	QueryExpectations jsontext.Value               `json:"query_expectations,omitempty"`
	Facts             jsontext.Value               `json:"facts,omitempty"`
	Steps             []ordinaryBaseProjectionStep `json:"steps"`
}

type ordinaryBaseOracleClaims struct {
	QueryEquality     string `json:"query_equality"`
	QueryExpectations string `json:"query_expectations"`
	API               string `json:"api"`
	FactReproduction  string `json:"fact_reproduction"`
	DynamicSQL        string `json:"dynamic_sql"`
}

type ordinaryBaseProjectionStep struct {
	Step        uint32         `json:"step"`
	Bytes       uint64         `json:"source_bytes"`
	SHA256      string         `json:"source_sha256"`
	Incremental jsontext.Value `json:"incremental"`
	Fresh       jsontext.Value `json:"fresh"`
}

type ordinaryBaseProjectionTreeWire struct {
	Status   string         `json:"status"`
	Code     string         `json:"code"`
	Form     string         `json:"form"`
	Count    uint64         `json:"descendant_count"`
	HasError bool           `json:"has_error"`
	Digest   string         `json:"digest"`
	Tree     jsontext.Value `json:"tree,omitempty"`
	Queries  jsontext.Value `json:"queries,omitempty"`
	API      jsontext.Value `json:"api,omitempty"`
}

type ordinaryBaseProjectionTree struct {
	Status   string                     `json:"status"`
	Code     string                     `json:"code"`
	Form     string                     `json:"form"`
	Count    uint64                     `json:"descendant_count"`
	HasError bool                       `json:"has_error"`
	Digest   string                     `json:"digest"`
	Tree     *ordinaryBaseProjectionCST `json:"-"`
	Queries  []witnessQueryProjection   `json:"-"`
	API      *ordinaryBaseAPISummary    `json:"-"`
}

type ordinaryBaseProjectionCST struct {
	Schema string     `json:"schema"`
	Input  rcInput    `json:"input"`
	Status string     `json:"status"`
	Nodes  []TreeNode `json:"nodes"`
}

type ordinaryBaseAPISummary struct {
	Revision           string                    `json:"revision"`
	Consistent         bool                      `json:"consistent"`
	First              *nativewire.APIDifference `json:"first_difference"`
	PositionNavigation int                       `json:"position_navigation_divergences"`
	FirstDivergence    *nativewire.APIDifference `json:"first_divergence"`
}

// recomputeOrdinaryBase reuses the legacy base reducer over one verified ordinary case.
func (b *eligibilityBudget) recomputeOrdinaryBase(profile OracleProfile, caseIndex int, cache *ordinaryCaseCache, facts *ordinaryFactInputs) (ordinaryBaseResult, *Error) {
	out := ordinaryBaseResult{Verdict: Verdict{ExecutionStatus: StatusNotRun, Assessment: AssessBlocked}, Gates: []ReplayGate{}, Findings: []Finding{}}
	if cache == nil || caseIndex < 0 || caseIndex >= len(profile.Native.Cases) || caseIndex >= len(profile.OracleCases) ||
		profile.Native.Cases[caseIndex].ID != cache.Case || profile.OracleCases[caseIndex].ID != cache.Case {
		return out, fail(KindInvalidInput, "ORDINARY_BASE_CASE", "ordinary", nil)
	}
	if cache.recordedBase.Assessment == AssessFail {
		out.Verdict = cache.recordedBase
		code := cache.recordedBaseCode
		if code == "" {
			code = "ORDINARY_BASE_RECORDED_FAILED"
		}
		out.Findings = append(out.Findings, Finding{Severity: "error", Code: code, Path: cache.Case, Message: "guarded reader가 확인한 원래 base FAIL을 보존한다"})
	}
	if len(cache.projection) == 0 {
		return out, fail(KindInvalidInput, "ORDINARY_BASE_PROJECTION", cache.Case, nil)
	}
	if cache.unanswered {
		// A sent frame without a complete response (watchdog, memory or output limit, batch limit, cancellation,
		// crash) has no raw response to recompute from. Its stored RESOURCE_LIMIT/BLOCKED projection is honest
		// evidence, so the base stays BLOCKED; an original FAIL kept above stays FAIL. A projection that claims a
		// completed or passing base for a frame that never answered contradicts the transport.
		if cache.recordedBase.ExecutionStatus == StatusCompleted || cache.recordedBase.Assessment == AssessPass {
			out.Verdict = Verdict{ExecutionStatus: StatusFailed, Assessment: AssessFail}
			out.Findings = append(out.Findings, Finding{Severity: "error", Code: "ORDINARY_BASE_PROJECTION_BINDING", Path: cache.Case, Message: "응답이 없는 frame의 projection이 완료나 PASS를 주장한다"})
		}
		return out, nil
	}
	one := profile
	one.Native.Cases = []IncrementalCase{profile.Native.Cases[caseIndex]}
	one.OracleCases = []OracleCase{profile.OracleCases[caseIndex]}
	semanticCache, e := b.ordinarySemanticBaseCache(one, cache)
	if e != nil {
		return out, e
	}
	res := QuerySemanticsResult{Report: newReport("ordinary base recomputation"), BaseVerified: Verdict{ExecutionStatus: StatusNotRun, Assessment: AssessBlocked}}
	res.Assessment = AssessBlocked
	res.Findings = out.Findings
	add := func(code, at, message string) {
		res.failures++
		if res.Code == "" {
			res.Code = code
		}
		res.Assessment = AssessFail
		if len(res.Findings) < 1000 {
			res.Findings = append(res.Findings, Finding{Severity: "error", Code: code, Path: at, Message: message})
		}
	}
	projection, stepViews, e := b.ordinaryBindBaseProjection(one, cache, add)
	if e != nil {
		if res.Assessment == AssessFail {
			out.Verdict.Assessment = AssessFail
		}
		out.Gates, out.Findings = res.Gates, res.Findings
		return out, e
	}
	if e := b.charge(4, 1024, "ordinary-base-claims"); e != nil {
		return out, e
	}
	semanticCache.ordinaryClaims = map[string]string{}
	if profile.FactPack == nil {
		semanticCache.ordinaryClaims["fact-reproduction"], semanticCache.ordinaryClaims["dynamic-sql"] = claimNotClaimed, claimNotClaimed
	}
	if e := b.ordinaryCompareBaseAPI(one, cache, stepViews, projection, semanticCache.ordinaryClaims, add); e != nil {
		if res.Assessment == AssessFail {
			out.Verdict.Assessment = AssessFail
		}
		out.Findings = res.Findings
		return out, e
	}
	if e := b.ordinaryCompareBaseFacts(one, cache, projection, facts, semanticCache.ordinaryClaims, add); e != nil {
		if res.Assessment == AssessFail {
			out.Verdict.Assessment = AssessFail
		}
		out.Findings = res.Findings
		return out, e
	}
	if e := b.semanticBase(one, cache.projection, semanticCache, &res, add); e != nil {
		if res.BaseVerified.Assessment == AssessFail || out.Verdict.Assessment == AssessFail {
			out.Verdict.Assessment = AssessFail
		}
		out.Gates, out.Findings = res.Gates, res.Findings
		return out, e
	}
	if e := b.ordinaryCompareBaseQueryExpectations(one, semanticCache, projection, add); e != nil {
		if res.BaseVerified.Assessment == AssessFail || res.Assessment == AssessFail || out.Verdict.Assessment == AssessFail {
			out.Verdict.Assessment = AssessFail
		}
		out.Gates, out.Findings = res.Gates, res.Findings
		return out, e
	}
	out.Gates, out.Findings = res.Gates, res.Findings
	if res.BaseVerified.Assessment == AssessFail || res.Assessment == AssessFail || out.Verdict.Assessment == AssessFail {
		out.Verdict = Verdict{ExecutionStatus: res.BaseVerified.ExecutionStatus, Assessment: AssessFail}
		if !executionStatuses[out.Verdict.ExecutionStatus] {
			out.Verdict.ExecutionStatus = StatusFailed
		}
	} else {
		out.Verdict = res.BaseVerified
	}
	return out, nil
}

func (b *eligibilityBudget) ordinarySemanticBaseCache(profile OracleProfile, cache *ordinaryCaseCache) (*semanticNativeCache, *Error) {
	out := &semanticNativeCache{
		ordinaryBase: true,
		record:       eligibilityNativeRecord{raw: cache.raw, rawBytes: uint64(max(0, len(cache.ResponseFrame)-4)), haveRaw: true},
		checked:      cache.checked, versions: cache.versions, points: cache.points, ready: cache.ready,
	}
	out.record.approved.ProducerSHA256 = cache.request.ObservationProducerSHA256
	count := len(profile.OracleCases[0].QueryExpect)
	work, e := eligibilityProduct("ordinary-base-cache", uint64(count), uint64(len(profile.Queries))+1)
	if e != nil {
		return nil, e
	}
	logical, e := eligibilityProduct("ordinary-base-cache", 512, uint64(count))
	if e != nil {
		return nil, e
	}
	if e := b.charge(work, logical, "ordinary-base-cache"); e != nil {
		return nil, e
	}
	out.tuples = make([]QueryWitnessTuple, 0, count)
	out.values = make([]witnessEvaluation, 0, count)
	out.evaluated = make([]bool, 0, count)
	out.queryClaims = make([]string, count)
	for _, expected := range profile.OracleCases[0].QueryExpect {
		out.tuples = append(out.tuples, QueryWitnessTuple{Query: expected.Query, Step: uint32(max(expected.Step, 0)), TreeRole: "incremental"})
		if expected.Step < 0 || expected.Step >= len(cache.checked) || expected.Step >= len(cache.versions) {
			out.values = append(out.values, witnessEvaluation{})
			out.evaluated = append(out.evaluated, false)
			continue
		}
		step := cache.checked[expected.Step].Incremental
		queryIndex := -1
		for i, query := range profile.Queries {
			if query.ID == expected.Query {
				queryIndex = i
				break
			}
		}
		if queryIndex < 0 || queryIndex >= len(step.Queries) || step.Form != "full" {
			out.values = append(out.values, witnessEvaluation{})
			out.evaluated = append(out.evaluated, false)
			continue
		}
		query := step.Queries[queryIndex]
		allocationWork := uint64(len(query.Captures)) + 1
		allocationLogical := uint64(len(query.Captures)) * 128
		if expected.Captures != nil {
			allocationLogical += uint64(len(*expected.Captures)) * 128
		}
		if e := b.charge(allocationWork, allocationLogical, "ordinary-base-query-captures"); e != nil {
			return nil, e
		}
		reservation := instrumentedReference{}
		reservation.Tree.Nodes = step.Nodes
		reservation.Query.Predicates, reservation.Query.CaptureNames = query.Predicates, query.CaptureNames
		for _, row := range query.Captures {
			reservation.Query.Structural = append(reservation.Query.Structural, witnessCapture(query, row))
		}
		if expected.Captures != nil {
			reservation.Query.Evaluated = make([]Capture, len(*expected.Captures))
		}
		if e := b.reserveReference(&reservation, [][]byte{cache.versions[expected.Step]}); e != nil {
			return nil, e
		}
		value, err := witnessEvaluate(b.r, query, profile.Native.Encoding, cache.versions[expected.Step])
		if err != nil {
			return nil, asError(err)
		}
		out.values = append(out.values, value)
		out.evaluated = append(out.evaluated, true)
	}
	return out, nil
}

func (b *eligibilityBudget) ordinaryBindBaseProjection(profile OracleProfile, cache *ordinaryCaseCache, add func(string, string, string)) (ordinaryBaseProjection, []ordinaryBaseProjectionStepView, *Error) {
	var projection ordinaryBaseProjection
	if e := b.decodeOrdinaryTypedShape("ordinary-base-projection", cache.projection, &projection, false); e != nil {
		return projection, nil, e
	}
	if projection.ID != cache.Case || projection.Input != profile.Native.Cases[0].Input || projection.Encoding != profile.Native.Encoding || projection.Status != cache.raw.Status {
		add("ORDINARY_BASE_PROJECTION_BINDING", cache.Case, "저장 base projection의 case·source·encoding·status가 현재 native 결과와 다르다")
	}
	rawAST, e := b.semanticAST("ordinary-base-projection", cache.projection)
	if e != nil {
		return projection, nil, e
	}
	inputBytes, e := b.ordinaryBaseInputBytes(cache.raw, rawAST)
	if e != nil {
		return projection, nil, e
	}
	if e := b.semanticProjectionInputs(cache.raw, rawAST, inputBytes, add); e != nil {
		return projection, nil, e
	}
	if len(projection.Steps) != len(cache.checked) {
		add("ORDINARY_BASE_STEP_PROJECTION", cache.Case, "저장 projection의 step 수가 checked raw와 다르다")
	}
	if e := b.charge(uint64(len(cache.projection))+1, 0, "ordinary-base-projection-index"); e != nil {
		return projection, nil, e
	}
	stepViews := make([]ordinaryBaseProjectionStepView, len(projection.Steps))
	for i, step := range projection.Steps {
		stepViews[i] = ordinaryBaseProjectionStepView{Step: step.Step, Bytes: step.Bytes, SHA256: step.SHA256}
		var e *Error
		stepViews[i].Incremental, e = b.ordinaryBaseProjectionTree(step.Incremental)
		if e != nil {
			return projection, nil, e
		}
		stepViews[i].Fresh, e = b.ordinaryBaseProjectionTree(step.Fresh)
		if e != nil {
			return projection, nil, e
		}
	}
	comparisonWork := uint64(1)
	for stepIndex, checked := range cache.checked {
		comparisonWork += uint64(len(checked.Incremental.Nodes) + len(checked.Incremental.Queries) + 1)
		for _, query := range checked.Incremental.Queries {
			comparisonWork += uint64(len(query.Captures) + len(query.Predicates))
		}
		if checked.Fresh != nil {
			comparisonWork += uint64(len(checked.Fresh.Nodes) + len(checked.Fresh.Queries) + 1)
			for _, query := range checked.Fresh.Queries {
				comparisonWork += uint64(len(query.Captures) + len(query.Predicates))
			}
		}
		if stepIndex < len(stepViews) {
			if stepViews[stepIndex].Incremental != nil {
				comparisonWork += uint64(len(stepViews[stepIndex].Incremental.Queries))
			}
			if stepViews[stepIndex].Fresh != nil {
				comparisonWork += uint64(len(stepViews[stepIndex].Fresh.Queries))
			}
		}
	}
	projectionLogical, e := eligibilityProduct("ordinary-base-projection-compare", 512, comparisonWork)
	if e != nil {
		return projection, nil, e
	}
	if e := b.charge(16*comparisonWork+1, projectionLogical, "ordinary-base-projection-compare"); e != nil {
		return projection, nil, e
	}
	for stepIndex, checked := range cache.checked {
		if stepIndex >= len(stepViews) || stepIndex >= len(cache.versions) {
			continue
		}
		step := stepViews[stepIndex]
		version := cache.versions[stepIndex]
		if step.Step != uint32(stepIndex) || step.Bytes != uint64(len(version)) || step.SHA256 != digestHex(version) {
			add("ORDINARY_BASE_STEP_IDENTITY", cache.Case, "저장 projection step이 재구성한 source history와 다르다")
		}
		for _, role := range []struct {
			name string
			raw  nativewire.Tree
			view *ordinaryBaseProjectionTree
		}{{"incremental", checked.Incremental, step.Incremental}, {"fresh", ordinaryFreshTree(checked.Fresh), step.Fresh}} {
			if role.raw.Form == "" || role.raw.Status != StatusCompleted {
				if role.view != nil {
					add("ORDINARY_BASE_TREE_PROJECTION", cache.Case, "raw에 없는 tree가 저장 projection에 있다")
				}
				continue
			}
			if role.view == nil {
				add("ORDINARY_BASE_TREE_PROJECTION", cache.Case, "완료된 raw tree가 저장 projection에 없다")
				continue
			}
			if role.view.Status != role.raw.Status || role.view.Code != role.raw.Code || role.view.Form != role.raw.Form || role.view.Count != role.raw.Wire.DescendantCount || role.view.HasError != role.raw.Wire.HasError || role.view.Digest != role.raw.Wire.Digest {
				add("ORDINARY_BASE_TREE_PROJECTION", cache.Case, "저장 tree 요약이 checked raw와 다르다")
			}
			if role.raw.Form == "full" {
				if role.view.Tree == nil || role.view.Tree.Schema != TreeSchema || role.view.Tree.Status != role.raw.Status || role.view.Tree.Input.Encoding != profile.Native.Encoding || role.view.Tree.Input.Bytes != uint64(len(version)) || role.view.Tree.Input.SHA256 != digestHex(version) || CompareTrees(role.raw.Nodes, role.view.Tree.Nodes) != nil {
					add("ORDINARY_BASE_CST_PROJECTION", cache.Case, "저장 full CST가 같은 step의 checked raw nodes와 다르다")
				}
				if len(role.raw.Queries) != len(profile.Queries) || len(role.view.Queries) != len(profile.Queries) {
					add("ORDINARY_BASE_QUERY_PROJECTION", cache.Case, "저장 query projection의 수가 caller profile 또는 raw와 다르다")
				} else {
					for queryIndex, query := range profile.Queries {
						projected := role.view.Queries[queryIndex]
						if projected.ID != query.ID || projected.SHA256 != digestHex([]byte(query.Source)) || !witnessProjectionQueryHeader(projected, profile.Native.Encoding) || !witnessQueryPassthrough(projected, role.raw.Queries[queryIndex], profile.Native.Encoding) {
							add("ORDINARY_BASE_QUERY_PROJECTION", query.ID, "저장 query metadata·status·capture가 checked raw와 다르다")
						}
					}
				}
			}
		}
	}
	return projection, stepViews, nil
}

type ordinaryBaseProjectionStepView struct {
	Step        uint32
	Bytes       uint64
	SHA256      string
	Incremental *ordinaryBaseProjectionTree
	Fresh       *ordinaryBaseProjectionTree
}

func (b *eligibilityBudget) ordinaryBaseProjectionTree(raw jsontext.Value) (*ordinaryBaseProjectionTree, *Error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var wire ordinaryBaseProjectionTreeWire
	if e := b.decodeOrdinaryTypedShape("ordinary-base-tree-projection", raw, &wire, false); e != nil {
		return nil, e
	}
	out := &ordinaryBaseProjectionTree{Status: wire.Status, Code: wire.Code, Form: wire.Form, Count: wire.Count, HasError: wire.HasError, Digest: wire.Digest}
	if len(wire.Tree) != 0 && string(wire.Tree) != "null" {
		var tree ordinaryBaseProjectionCST
		if e := b.decodeOrdinaryTypedShape("ordinary-base-cst-projection", wire.Tree, &tree, false); e != nil {
			return nil, e
		}
		out.Tree = &tree
	}
	if len(wire.Queries) != 0 && string(wire.Queries) != "null" {
		if e := b.decodeOrdinaryTypedShape("ordinary-base-query-projection", wire.Queries, &out.Queries, false); e != nil {
			return nil, e
		}
	}
	if len(wire.API) != 0 && string(wire.API) != "null" {
		var api ordinaryBaseAPISummary
		if e := b.decodeOrdinaryTypedShape("ordinary-base-api-projection", wire.API, &api, false); e != nil {
			return nil, e
		}
		out.API = &api
	}
	return out, nil
}

func ordinaryFreshTree(tree *nativewire.Tree) nativewire.Tree {
	if tree == nil {
		return nativewire.Tree{}
	}
	return *tree
}

func (b *eligibilityBudget) ordinaryCompareBaseQueryExpectations(profile OracleProfile, cache *semanticNativeCache, projection ordinaryBaseProjection, add func(string, string, string)) *Error {
	expectations := profile.OracleCases[0].QueryExpect
	if e := b.charge(uint64(len(expectations))+1, 256*uint64(len(expectations)), "ordinary-base-query-expectations"); e != nil {
		return e
	}
	var recorded []semanticQueryExpectationResult
	if len(projection.QueryExpectations) != 0 && string(projection.QueryExpectations) != "null" {
		if e := b.decodeOrdinaryTypedShape("ordinary-base-query-expectations", projection.QueryExpectations, &recorded, false); e != nil {
			return e
		}
	}
	if len(recorded) != len(expectations) || len(cache.queryClaims) != len(expectations) {
		if len(expectations) != 0 || len(recorded) != 0 {
			add("ORDINARY_BASE_QUERY_EXPECTATIONS", profile.Native.Cases[0].ID, "저장 query expectation 항목 수가 재계산 분모와 다르다")
		}
		return nil
	}
	for i, expected := range expectations {
		got := recorded[i]
		if got.Query != expected.Query || got.Step != expected.Step || got.Status != expected.Status || got.Code != expected.Code ||
			!reflect.DeepEqual(got.Captures, expected.Captures) || !reflect.DeepEqual(got.Error, expected.Error) || got.Result != cache.queryClaims[i] {
			add("ORDINARY_BASE_QUERY_EXPECTATIONS", expected.Query, "저장 query expectation이 source·checked raw 재계산과 다르다")
		}
	}
	return nil
}

func (b *eligibilityBudget) ordinaryCompareBaseAPI(profile OracleProfile, cache *ordinaryCaseCache, stepViews []ordinaryBaseProjectionStepView, projection ordinaryBaseProjection, claims map[string]string, add func(string, string, string)) *Error {
	claim := claimNotClaimed
	for _, checked := range cache.checked {
		for _, tree := range []*nativewire.Tree{&checked.Incremental, checked.Fresh} {
			if tree == nil || tree.Status != StatusCompleted {
				continue
			}
			if !profile.API {
				continue
			}
			if tree.Form != "full" || tree.API == nil || checked.Step.Step < 0 || checked.Step.Step >= len(cache.versions) {
				claim = worseClaim(claim, claimBlocked)
				continue
			}
			rows := uint64(len(tree.API.Nodes) + len(tree.API.ChildFields) + len(tree.API.FieldLookups) + len(tree.API.Points) + len(tree.API.Names))
			work, e := eligibilityProduct("ordinary-base-api", 64, uint64(len(tree.Nodes))+rows+1)
			if e != nil {
				return e
			}
			// The comparison's own bound (field and name bytes, one search per point) plus the tree and shape walks.
			work = max(work, nativewire.CompareAPIWork(tree.Nodes, tree.API)+16*(uint64(len(tree.Nodes))+rows))
			logical, e := eligibilityProduct("ordinary-base-api", 512, uint64(len(tree.Nodes))+rows+1)
			if e != nil {
				return e
			}
			logical = max(logical, nativewire.CompareAPILogical(tree.Nodes, tree.API))
			if e := b.charge(work, logical, "ordinary-base-api"); e != nil {
				return e
			}
			if nativewire.ValidateTree(tree.Nodes, uint64(len(cache.versions[checked.Step.Step]))) != nil || !ordinaryAPIShapeValid(tree.API, len(tree.Nodes)) {
				claim = worseClaim(claim, claimBlocked)
				continue
			}
			first, divergences := nativewire.CompareAPI(tree.Nodes, tree.API)
			want := &ordinaryBaseAPISummary{Revision: tree.API.Revision, Consistent: first == nil, First: first, PositionNavigation: len(divergences)}
			if len(divergences) > 0 {
				want.FirstDivergence = &divergences[0]
			}
			claimValue := claimPass
			if first != nil || len(divergences) != 0 {
				claimValue = claimFail
			}
			claim = worseClaim(claim, claimValue)
			step := int(checked.Step.Step)
			if step >= len(stepViews) {
				continue
			}
			view := stepViews[step].Incremental
			if tree == checked.Fresh {
				view = stepViews[step].Fresh
			}
			if view == nil || !reflect.DeepEqual(view.API, want) {
				add("ORDINARY_BASE_API_PROJECTION", cache.Case, "저장 API 판정이 raw API와 full CST의 재계산과 다르다")
			}
		}
	}
	if len(projection.Oracle) != 0 && string(projection.Oracle) != "null" {
		var recorded ordinaryBaseOracleClaims
		if e := b.decodeOrdinaryTypedShape("ordinary-base-oracle-claims", projection.Oracle, &recorded, false); e != nil {
			return e
		}
		if recorded.API != claim {
			add("ORDINARY_BASE_API_CLAIM", cache.Case, "저장 API claim이 native raw/table 재계산과 다르다")
		}
	}
	claims["api"] = claim
	return nil
}

type ordinaryBaseFacts struct {
	Pack         FactPackRef                `json:"pack"`
	Declarations *ordinaryProducerDeclFacts `json:"declarations"`
	DynamicSQL   *DynamicSQLFacts           `json:"dynamic_sql"`
	XML          map[string]int             `json:"xml_structure,omitempty"`
}

// Facts reproduce producer evidence; independent expectations remain in the tuple adjudicator.
func (b *eligibilityBudget) ordinaryCompareBaseFacts(profile OracleProfile, cache *ordinaryCaseCache, projection ordinaryBaseProjection, inputs *ordinaryFactInputs, claims map[string]string, add func(string, string, string)) *Error {
	if profile.FactPack == nil || inputs == nil || cache.raw.Status != StatusCompleted {
		return nil
	}
	if !inputs.Ready || *profile.FactPack != inputs.PackRef {
		return fail(KindInvalidInput, "ORDINARY_BASE_FACT_INPUTS", cache.Case, nil)
	}
	var recorded ordinaryProducerFactRecord
	if e := b.decodeOrdinaryTyped("ordinary-base-step-facts", cache.stepFacts, &recorded); e != nil {
		return e
	}
	if e := b.charge(uint64(len(inputs.Pack.Routes))+uint64(len(profile.Queries))+1, 256, "ordinary-base-fact-index"); e != nil {
		return e
	}
	route, ok := ordinaryFactRoute(inputs.Pack, inputs.PackRef.Route)
	if !ok || len(route.Queries) == 0 || len(profile.Queries) < len(route.Queries) {
		return fail(KindInvalidInput, "ORDINARY_BASE_FACT_ROUTE", cache.Case, nil)
	}
	// Reserve pin hashing/comparison before constructing the ordered suffix.
	pinBytes := uint64(len(cache.stepFacts))
	for _, q := range route.Queries {
		pinBytes += uint64(len(q.ID) + len(q.Source) + len(q.Facts) + 88)
	}
	if e := b.charge(4*pinBytes+uint64(len(inputs.Pack.Routes))+1, 4*pinBytes+512, "ordinary-base-fact-pins"); e != nil {
		return e
	}
	pins := ordinaryPackFactQueryPins(inputs.Pack, inputs.PackRef.Route)
	start := len(profile.Queries) - len(pins)
	for i, q := range route.Queries {
		if profile.Queries[start+i].ID != q.ID || profile.Queries[start+i].Source != q.Source {
			return fail(KindInvalidInput, "ORDINARY_FACT_SUFFIX", cache.Case, nil)
		}
	}
	if recorded.Schema != "tsgk-ordinary-step-facts/r1" || recorded.Case != cache.Case || recorded.Pack != inputs.PackRef || recorded.Mapping != inputs.MappingPin ||
		recorded.DynamicSQLRevision != inputs.Pack.DynamicSQL || recorded.XMLRevision != inputs.Pack.XML || !slices.Equal(recorded.FactQueries, pins) || recorded.FactSuffixSHA256 != ordinaryFactSuffixDigest(pins) {
		add("ORDINARY_BASE_FACT_BINDING", cache.Case, "저장 step facts의 pack·mapping·query suffix pin이 다르다")
		return nil
	}
	if len(cache.checked) == 0 || len(cache.checked) != len(cache.versions) || len(recorded.Steps) != 2*len(cache.checked)-1 {
		add("ORDINARY_BASE_FACT_STEPS", cache.Case, "저장 step facts의 전체 step·role 분모가 다르다")
		return nil
	}
	if e := b.charge(uint64(len(cache.stepFacts))+uint64(len(recorded.Steps))+1, 512, "ordinary-base-fact-compare"); e != nil {
		return e
	}
	first := ordinaryBaseFacts{Pack: inputs.PackRef}
	factClaim, dynamicClaim := claimNotClaimed, claimNotClaimed
	index := 0
	for step, checked := range cache.checked {
		version := cache.versions[step]
		if e := b.charge(uint64(len(version))+1, 128, "ordinary-base-fact-source"); e != nil {
			return e
		}
		sourceSHA := digestHex(version)
		for roleIndex, tree := range []*nativewire.Tree{&checked.Incremental, checked.Fresh} {
			if step == 0 && roleIndex == 1 {
				continue
			}
			role := "incremental"
			if roleIndex == 1 {
				role = "fresh"
			}
			stored := recorded.Steps[index]
			index++
			if stored.Step != step || stored.TreeRole != role || stored.SourceBytes != uint64(len(version)) || stored.SourceSHA256 != sourceSHA || len(stored.Queries) != len(pins) {
				add("ORDINARY_BASE_FACT_SOURCE", cache.Case, "저장 fact가 해당 step·role의 편집 후 source와 결속되지 않는다")
				continue
			}
			if tree == nil || tree.Status != StatusCompleted || tree.Form != "full" || len(tree.Queries) != len(profile.Queries) {
				if step == 0 {
					factClaim = worseClaim(factClaim, claimBlocked)
				}
				continue
			}
			roleStatus, roleCode := StatusCompleted, ""
			for qi, q := range route.Queries {
				query := tree.Queries[start+qi]
				got := ordinaryProducerFactQuery{ID: q.ID, SHA256: pins[qi].SHA256, Kind: q.Facts, Status: StatusNotRun, Code: "FACT_QUERY_NOT_COMPLETE_STRUCTURAL"}
				if query.ID == q.ID && query.Status == StatusCompleted && !query.Partial && len(query.Predicates) == 0 && query.Captures != nil {
					if e := b.ordinaryBaseFactReservation(q, route, query, version); e != nil {
						return e
					}
					captures := make([]Capture, len(query.Captures))
					for i, row := range query.Captures {
						captures[i] = witnessCapture(query, row)
					}
					got.Status, got.Code = StatusCompleted, ""
					switch q.Facts {
					case FactDeclarations:
						items, e := deriveDeclarations(route.Declarations, captures, b)
						if e != nil {
							return e
						}
						reproduces := ordinaryBaseDeclarationsEqual(items, tree.Wire.Declarations)
						got.Declarations = &ordinaryProducerDeclFacts{Items: items, Assessment: ordinaryDeclarationAssessment(items), Reproduces: reproduces}
						if !reproduces {
							got.Status, got.Code = StatusFailed, "FACT_REPRODUCTION_MISMATCH"
						}
					case FactDynamicSQL:
						facts, err := DeriveDynamicSQLChecked(inputs.PackRef.Route, profile.Native.Encoding, captures, version)
						if err != nil {
							got.Status, got.Code = StatusFailed, "FACT_DERIVATION_FAILED"
						} else {
							got.DynamicSQL = &facts
						}
					case FactXMLStructure:
						got.XML = map[string]int{}
						for _, c := range captures {
							got.XML[c.Name]++
						}
					}
				}
				want := stored.Queries[qi]
				if want.ID != got.ID || want.SHA256 != got.SHA256 || want.Kind != got.Kind || want.Status != got.Status || want.Code != got.Code ||
					!reflect.DeepEqual(want.Declarations, got.Declarations) || !reflect.DeepEqual(want.DynamicSQL, got.DynamicSQL) || !reflect.DeepEqual(want.XML, got.XML) {
					add("ORDINARY_BASE_FACT_MISMATCH", cache.Case, "저장 fact가 같은 step·role의 raw captures·source 재계산과 다르다")
				}
				if got.Status == StatusFailed {
					roleStatus, roleCode = StatusFailed, got.Code
				} else if got.Status != StatusCompleted && roleStatus == StatusCompleted {
					roleStatus, roleCode = got.Status, got.Code
				}
				if step == 0 {
					if query.Status != StatusCompleted {
						factClaim = worseClaim(factClaim, claimBlocked)
					}
					switch q.Facts {
					case FactDeclarations:
						first.Declarations = got.Declarations
						if got.Declarations != nil {
							value := claimPass
							if !got.Declarations.Reproduces {
								value = claimFail
							}
							factClaim = worseClaim(factClaim, value)
						}
					case FactDynamicSQL:
						first.DynamicSQL = got.DynamicSQL
						if got.Code == "FACT_DERIVATION_FAILED" {
							dynamicClaim = worseClaim(dynamicClaim, claimBlocked)
						}
						if expected := profile.OracleCases[0].DynamicSQL; expected != nil && got.DynamicSQL != nil {
							if e := b.charge(256*uint64(len(expected.Facts)+len(expected.KnownMisses)+1), 0, "ordinary-base-fact-expectation"); e != nil {
								return e
							}
							dynamicClaim = ordinaryBaseDynamicClaim(*expected, got.DynamicSQL)
						}
					case FactXMLStructure:
						first.XML = got.XML
					}
				}
			}
			if stored.Status != roleStatus || stored.Code != roleCode {
				add("ORDINARY_BASE_FACT_STATUS", cache.Case, "저장 step·role fact status가 재계산과 다르다")
			}
		}
	}
	var storedFirst ordinaryBaseFacts
	if e := b.decodeOrdinaryTypedShape("ordinary-base-facts", projection.Facts, &storedFirst, false); e != nil {
		return e
	}
	if !reflect.DeepEqual(storedFirst, first) {
		add("ORDINARY_BASE_FACT_PROJECTION", cache.Case, "저장 base fact projection이 step0 재계산과 다르다")
	}
	claims["fact-reproduction"], claims["dynamic-sql"] = factClaim, dynamicClaim
	return b.r.check()
}

func ordinaryBaseDeclarationsEqual(items []DeclarationItem, raw *[]nativewire.DeclItem) bool {
	if raw == nil || len(items) != len(*raw) {
		return false
	}
	for i, a := range items {
		b := (*raw)[i]
		if a.Fact != b.Fact || a.NodeType != b.NodeType || a.StartByte != b.StartByte || a.EndByte != b.EndByte || a.Status != b.Status ||
			(a.Name == nil) != (b.Name == nil) || a.Name != nil && (a.Name.StartByte != b.Name.StartByte || a.Name.EndByte != b.Name.EndByte) {
			return false
		}
	}
	return true
}

func ordinaryBaseDynamicClaim(want DynamicSQLExpectation, got *DynamicSQLFacts) string {
	if got == nil {
		return claimBlocked
	}
	if !slices.EqualFunc(want.Facts, got.Items, func(a, b DynamicSQLFact) bool { return reflect.DeepEqual(a, b) }) || len(want.KnownMisses) != len(got.KnownMisses) {
		return claimFail
	}
	for i, miss := range got.KnownMisses {
		if miss.StartByte != want.KnownMisses[i].StartByte || miss.EndByte != want.KnownMisses[i].EndByte {
			return claimFail
		}
	}
	return claimPass
}

func (b *eligibilityBudget) ordinaryBaseFactReservation(q FactQuery, route FactPackRoute, query querywire.Query, source []byte) *Error {
	if e := b.charge(uint64(len(query.Captures))+1, 0, "ordinary-base-fact-index"); e != nil {
		return e
	}
	n := uint64(len(query.Captures)) + 1
	var text, spans, execs, withErrors uint64
	var tsql tsqlFactCounts
	for _, row := range query.Captures {
		name := query.CaptureNames[row[2]]
		tsql.add(name)
		text += uint64(len(name) + len(query.Types[row[4]]))
		if row[7] >= row[6] && row[6] >= 0 && uint64(row[7]) <= uint64(len(source)) {
			spans += uint64(row[7] - row[6])
		}
		if name == "exec" {
			execs++
		} else if name == "errwith" {
			withErrors++
		}
	}
	// Capture conversion, indexes and XML counts; declaration frontiers reserve in the walk.
	if e := b.charge(64*n+8*text, 1024*n+4*text, "ordinary-base-fact-derive"); e != nil {
		return e
	}
	if q.Facts != FactDynamicSQL {
		return nil
	}
	if route.Route == "csharp" {
		return b.ordinaryBaseCSharpFactReservation(query, n)
	}
	if route.Route != "tsql" {
		// Keep the legacy conservative bound for other dynamic SQL adapters.
		work, e := eligibilityProduct("ordinary-base-fact-derive", 64, n, n, n+uint64(len(source))+text+1)
		if e != nil {
			return e
		}
		logical, e := eligibilityProduct("ordinary-base-fact-derive", 1024, n, n+uint64(len(source))+text+1)
		if e != nil {
			return e
		}
		return b.charge(work, logical, "ordinary-base-fact-derive")
	}
	// Pair scans/sorts and repeated child/known-miss scans from the capture counts (tsqlFactBound),
	// then the decoded source spans.
	tsqlWork, tsqlLogical, e := tsqlFactBound(tsql)
	if e != nil {
		return e
	}
	if e := b.charge(tsqlWork, tsqlLogical, "ordinary-base-fact-derive"); e != nil {
		return e
	}
	for _, term := range [][]uint64{{64, execs + 1, spans}, {64, execs, withErrors, uint64(len(source))}} {
		work, e := eligibilityProduct("ordinary-base-fact-derive", term...)
		if e != nil {
			return e
		}
		if e := b.charge(work, work, "ordinary-base-fact-derive"); e != nil {
			return e
		}
	}
	return nil
}

// csharpFactScans is deriveCSharp's fixed number of sorted passes over the captures: eleven
// capIndex.pairs calls plus the final site sort.
const csharpFactScans = 12

// The decode term charges each decoded span byte csharpDecodeWork work and csharpDecodeLogical
// logical bytes. Work: a counted decode step (64: one source byte read, up to three output
// bytes written) plus csharpIdentifierName's reads and writes of the decoded text at the 8 per
// text byte of the shared conversion charge, at most 3 reads + 3 writes per source byte
// (CP949 turns one invalid byte into a 3-byte U+FFFD): 64 + 8·6 = 112 <= 128. Logical: the
// cumulative bytes allocated for one span by srcText.span and csharpIdentifierName, measured
// at at most 32.03 per source byte (CP949 invalid bytes, 1 MiB span; UTF-16LE 12.6-20.6, UTF-8
// 6.0, CP949 text 10-17.5), doubled for margin. Not a peak or RSS measure.
const (
	csharpDecodeWork    = 128
	csharpDecodeLogical = 64
)

// ordinaryBaseCSharpFactReservation charges what deriveCSharp does: csharpFactScans sorted
// passes (n log n), one chain walk per command argument (at most the qualified-name captures
// long) and at most two decoded spans per argument or assignment (bounded by the longest span
// of a capture that is decoded). n is the capture count plus one.
func (b *eligibilityBudget) ordinaryBaseCSharpFactReservation(query querywire.Query, n uint64) *Error {
	var args, lefts, names, longest uint64
	for _, row := range query.Captures {
		name := query.CaptureNames[row[2]]
		switch name {
		case "oc.arg":
			args++
		case "ini.left", "ma.name":
			lefts++
		case "qn.name", "aq.name", "gn.name":
			names++
		}
		if (name == "bin.op" || name == "ini.left" || name == "ma.name" || query.Types[row[4]] == "identifier") && row[6] >= 0 && row[7] >= row[6] {
			longest = max(longest, uint64(row[7]-row[6]))
		}
	}
	decodes := 2 * (args + lefts)
	for _, term := range []struct {
		work, logical []uint64
	}{
		{[]uint64{64, csharpFactScans, n, 2 + uint64(bits.Len64(n))}, []uint64{1024, csharpFactScans, n}},
		{[]uint64{64, args, names + 1}, nil},
		{[]uint64{csharpDecodeWork, decodes, longest + 1}, []uint64{csharpDecodeLogical, decodes, longest + 1}},
	} {
		work, e := eligibilityProduct("ordinary-base-fact-derive", term.work...)
		if e != nil {
			return e
		}
		var logical uint64
		if term.logical != nil {
			if logical, e = eligibilityProduct("ordinary-base-fact-derive", term.logical...); e != nil {
				return e
			}
		}
		if e := b.charge(work, logical, "ordinary-base-fact-derive"); e != nil {
			return e
		}
	}
	return nil
}

// Bound only the values semanticProjectionInputs serializes, including JSON escaping.
func (b *eligibilityBudget) ordinaryBaseInputBytes(raw nativewire.Response, projection *jv) (uint64, *Error) {
	var rootKeys uint64
	if projection != nil {
		rootKeys = uint64(len(projection.keys))
	}
	// Include this estimator's lookups and the shared comparator's bounded field names.
	if e := b.charge(256*rootKeys+3, 0, "ordinary-base-input-size"); e != nil {
		return 0, e
	}
	status := jvMember(projection, "execution_status")
	if raw.Status != StatusCompleted || status == nil || status.s != StatusCompleted {
		return 0, nil
	}
	p := raw.Producer
	n := uint64(1024 + 6*(len(p.Query)+len(p.ReuseObservation)+len(raw.Status)+len(raw.Code)))
	for _, value := range []*string{p.API, p.Predicates} {
		if value != nil {
			n += 6 * uint64(len(*value))
		}
	}
	steps := jvMember(projection, "steps")
	for i, step := range raw.Steps {
		var view *jv
		if steps != nil && i < len(steps.vals) {
			view = steps.vals[i]
		}
		var keys uint64
		if view != nil {
			keys = uint64(len(view.keys))
		}
		if e := b.charge(256*keys+5, 0, "ordinary-base-input-size"); e != nil {
			return 0, e
		}
		for _, role := range []string{"incremental", "fresh"} {
			if tree := jvMember(view, role); tree != nil {
				if e := b.charge(32*uint64(len(tree.keys)), 0, "ordinary-base-input-size"); e != nil {
					return 0, e
				}
			}
		}
		n += 512 // route, null declarations and role/container punctuation
		for _, tree := range []*nativewire.WireTree{&step.Incremental, step.Fresh} {
			if tree == nil || tree.Declarations == nil {
				continue
			}
			if e := b.charge(uint64(len(*tree.Declarations)), 0, "ordinary-base-input-size"); e != nil {
				return 0, e
			}
			for _, item := range *tree.Declarations {
				n += 256 + 6*uint64(len(item.Fact)+len(item.NodeType)+len(item.Status))
			}
		}
	}
	return n, nil
}

// Decode bytes are reserved separately; retain every shared reducer check.
func (b *eligibilityBudget) reserveOrdinaryBaseReplay(c *rcCase, want *IncrementalCase, n uint64) *Error {
	if e := b.charge(uint64(3*len(c.Steps)+len(c.Expectations)+len(c.References))+1, 0, "ordinary-base-replay-size"); e != nil {
		return e
	}
	work := 2*n + 128 + 64*uint64(len(c.Steps)+len(c.Expectations)+len(c.References))
	identities := func(ids []IdentityRef) *Error {
		if e := b.charge(uint64(len(ids)), 0, "ordinary-base-replay-size"); e != nil {
			return e
		}
		work += 64 * uint64(len(ids))
		for _, id := range ids {
			work += 8 * uint64(bits.Len64(n)+1) * uint64(len(id.Role)+len(id.Schema)+len(id.SHA256)+1)
		}
		return nil
	}
	for _, step := range c.Steps {
		work += 8 * uint64(len(step.Observations))
		if step.Composite != nil {
			if e := identities(step.Composite.Identities); e != nil {
				return e
			}
			if step.Composite.Inline != nil && step.Composite.Inline.Tree != nil {
				if e := identities(step.Composite.Inline.Tree.Identities); e != nil {
					return e
				}
			}
		}
		for _, tree := range []*rcTree{step.Incremental, step.Fresh} {
			if tree == nil {
				continue
			}
			work += 64 * uint64(len(tree.Queries)+1)
			if e := b.charge(uint64(len(tree.Queries))+1, 0, "ordinary-base-replay-size"); e != nil {
				return e
			}
			for _, query := range tree.Queries {
				work += 64 * uint64(len(query.Captures))
				if e := b.charge(uint64(len(query.Captures)), 0, "ordinary-base-replay-size"); e != nil {
					return e
				}
				for _, capture := range query.Captures {
					work += 16 * uint64(len(capture.Name)+len(capture.Type))
				}
			}
			if tree.Tree != nil {
				if e := identities(tree.Tree.Identities); e != nil {
					return e
				}
				if e := b.charge(uint64(len(tree.Tree.Nodes)), 0, "ordinary-base-replay-size"); e != nil {
					return e
				}
				for _, node := range tree.Tree.Nodes {
					work += 256 + 8*uint64(len(node.Type))
					if node.Field != nil {
						work += 8 * uint64(len(*node.Field))
					}
				}
			}
			if tree.Summary != nil {
				if e := identities(tree.Summary.Identities); e != nil {
					return e
				}
			}
		}
	}
	// expectResultCached builds a step tree's type set and anchor index at most once per case: the first
	// expectation that needs one reserves it, the later ones only their lookups.
	typeSet, anchorSet := map[int]bool{}, map[int]bool{}
	for _, expected := range want.Expect {
		if e := b.charge(uint64(len(expected.Contains)+len(expected.Anchors))+1, 0, "ordinary-base-replay-size"); e != nil {
			return e
		}
		work += 64 * uint64(len(expected.Contains)+len(expected.Anchors)+1)
		for _, value := range expected.Contains {
			work += 4 * uint64(len(value))
		}
		if expected.Step < 0 || expected.Step >= len(c.Steps) || len(expected.Contains)+len(expected.Anchors) == 0 {
			continue
		}
		tree := c.Steps[expected.Step].Incremental
		if tree == nil || tree.Tree == nil {
			continue
		}
		nodes := tree.Tree.Nodes
		if e := b.charge(uint64(len(nodes)), 0, "ordinary-base-replay-size"); e != nil {
			return e
		}
		var types uint64
		for _, node := range nodes {
			types += uint64(len(node.Type))
		}
		if len(expected.Contains) != 0 && !typeSet[expected.Step] {
			typeSet[expected.Step] = true
			if e := b.charge(0, 256*uint64(len(nodes)), "semantic-base-replay"); e != nil {
				return e
			}
			work += 64*uint64(len(nodes)) + 4*types
		}
		if len(expected.Anchors) >= anchorIndexMin {
			// The anchor index holds an insert per node (hashing its type, as Contains does); every anchor
			// is one lookup (hashing its type and comparing the key), not a scan of the tree.
			if !anchorSet[expected.Step] {
				anchorSet[expected.Step] = true
				if e := b.charge(0, 256*uint64(len(nodes)), "semantic-base-replay"); e != nil {
					return e
				}
				work += 64*uint64(len(nodes)) + 4*types
			}
			for _, anchor := range expected.Anchors {
				work += 64 + 8*uint64(len(anchor.Type))
			}
			continue
		}
		for _, anchor := range expected.Anchors {
			part, e := eligibilityProduct("semantic-base-replay", uint64(len(nodes)), 64+4*uint64(len(anchor.Type)))
			if e != nil {
				return e
			}
			work += part + 4*types
		}
	}
	return b.charge(work, 0, "semantic-base-replay")
}
