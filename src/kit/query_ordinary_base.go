package kit

import (
	"encoding/json/jsontext"
	"reflect"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
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
func (b *eligibilityBudget) recomputeOrdinaryBase(profile OracleProfile, caseIndex int, cache *ordinaryCaseCache) (ordinaryBaseResult, *Error) {
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
		record:  eligibilityNativeRecord{raw: cache.raw, rawBytes: uint64(max(0, len(cache.ResponseFrame)-4)), haveRaw: true},
		checked: cache.checked, versions: cache.versions, points: cache.points, ready: cache.ready,
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
	if e := b.semanticProjectionInputs(cache.raw, rawAST, uint64(max(0, len(cache.ResponseFrame)-4)), add); e != nil {
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
			logical, e := eligibilityProduct("ordinary-base-api", 512, uint64(len(tree.Nodes))+rows+1)
			if e != nil {
				return e
			}
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
