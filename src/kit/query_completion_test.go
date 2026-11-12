package kit

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

type faninFixture struct {
	req       QueryCompletionRequest
	v         *queryregistrycore.Validated
	svc       SvcExpected
	plan      faninPlan
	predAsset []byte
	svcAsset  []byte
	inventory []byte
}

var (
	faninOnce sync.Once
	faninFix  faninFixture
)

func faninFixtureFor(t *testing.T) *faninFixture {
	t.Helper()
	faninOnce.Do(func() {
		r, v, _ := ordinaryRegistryFixture(t)
		svcAsset, registry, inventory, svcFixture := svcAssets(t)
		predAsset, _ := predicateAssets(t)
		svc, err := ParseSvcExpected(svcAsset, registry, inventory, svcFixture)
		if err != nil {
			t.Fatal(err)
		}
		faninFix = faninFixture{
			req: QueryCompletionRequest{Registry: r.Registry, RegistrySHA256: r.RegistrySHA256, Qualification: r.Qualification, Slots: r.Slots, Basis: r.Basis, SvcExpected: svcAsset, SvcFixture: svcFixture},
			v:   v, svc: svc, plan: buildFaninPlan(v, svc), predAsset: predAsset, svcAsset: svcAsset, inventory: inventory,
		}
	})
	if faninFix.v == nil {
		t.Fatal("fan-in fixture unavailable")
	}
	return &faninFix
}

func (f *faninFixture) structuralIDs(platform string, n int) []string {
	var ids []string
	for _, c := range f.v.Coordinates() {
		if c.Kind == queryregistrycore.Structural && c.Platform == platform && len(ids) < n {
			ids = append(ids, c.ID(f.v.Namespace()))
		}
	}
	return ids
}

func faninShard(ids []string, states []string, bound bool) OrdinaryQuerySemanticsResult {
	p := newOrdinarySemanticsResult()
	p.BindingValid = bound
	p.SelectedShardIDs = []string{"shard-" + ids[0][6:14]}
	p.SelectedObligationIDs = ids
	p.CoverageSHA256 = ordinaryIDDigest(ids)
	p.Tuple.Required = uint64(len(ids))
	for i, state := range states {
		code := ""
		if state != ordinaryTupleVerified {
			code = "CODE_" + state
		}
		p.Obligations = append(p.Obligations, OrdinaryQuerySemanticsTuple{ID: ids[i], State: state, Code: code})
		switch state {
		case ordinaryTupleVerified:
			p.Tuple.Verified++
		case ordinaryTupleFailed:
			p.Tuple.Failed++
		default:
			p.Tuple.Blocked++
		}
	}
	p.Tuple.Unprocessed = uint64(len(ids) - len(states))
	if p.Tuple.Failed > 0 {
		p.Semantic = AssessFail
		p.finding("CODE_FAILED", "", "")
	}
	return p
}

func (f *faninFixture) predicateScope(t *testing.T) QueryScopeResult {
	t.Helper()
	reg, err := queryregistrycore.Parse(f.req.Registry)
	if err != nil {
		t.Fatal(err)
	}
	var judgments []queryregistrycore.Judgment
	for _, c := range queryregistrycore.Expand(nil, queryregistrycore.Platforms(), reg.Probes, nil) {
		if c.Platform != "windows-amd64" {
			continue
		}
		j := queryregistrycore.Judgment{ID: c.ID(reg.Namespace), Coordinate: c, State: "VERIFIED"}
		if c.Kind == queryregistrycore.Control {
			for _, p := range reg.Probes {
				if p.ID == c.Case {
					j.Subject = &queryregistrycore.Subject{Status: p.Status, Code: p.Code, Evaluation: p.Evaluation, Partial: p.Partial, Structural: p.Structural, Expected: p.Expected}
				}
			}
		}
		judgments = append(judgments, j)
	}
	r, err := PredicateScopeResult(f.req.Registry, f.predAsset, judgments, map[string]bool{"linux-amd64": true})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *faninFixture) svcScope(t *testing.T) QueryScopeResult {
	t.Helper()
	judgments, err := VerifySvcExpected(f.svc, f.inventory)
	if err != nil {
		t.Fatal(err)
	}
	r, err := SvcScopeResult(f.svc, f.svcAsset, f.req.Registry, "windows-amd64", judgments, map[string]bool{"linux-amd64": true})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

var faninOrdinaryAsset = queryregistrycore.Pin{Path: "ordinary-asset-registration", Bytes: 1, SHA256: strings.Repeat("a", 64)}

func (f *faninFixture) ordinaryScope(t *testing.T, states ...string) QueryScopeResult {
	t.Helper()
	ids := f.structuralIDs("windows-amd64", 4)
	r, err := OrdinaryScopeResult(f.v, []queryregistrycore.Pin{faninOrdinaryAsset}, []OrdinaryQuerySemanticsResult{faninShard(ids, states, true)})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// request pins every scope result and the evidence assets the way a caller selects them.
func (f *faninFixture) request(scopes ...QueryScopeResult) QueryCompletionRequest {
	req := f.req
	req.Scopes = slices.Clone(scopes)
	req.ResultPins = map[string]string{}
	for _, s := range scopes {
		req.ResultPins[s.Scope] = s.ResultSHA256
	}
	req.AssetPins = map[string][]queryregistrycore.Pin{
		QueryScopeOrdinary:  {faninOrdinaryAsset},
		QueryScopePredicate: {queryregistrycore.FilePin(PredicateExpectedPath, f.predAsset)},
		QueryScopeSvc:       {queryregistrycore.FilePin(SvcExpectedPath, f.svcAsset)},
	}
	return req
}

func cloneScope(r QueryScopeResult) QueryScopeResult {
	r.Counters, r.Assets = slices.Clone(r.Counters), slices.Clone(r.Assets)
	return r
}

// seal recomputes the digest of an edited result, as a forger who knows the format would.
func seal(r QueryScopeResult) QueryScopeResult {
	r.ResultSHA256 = r.digest()
	return r
}

func faninCode(t *testing.T, err error) string {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("not a kit error: %v", err)
	}
	return e.Code
}

func totalsOf(r QueryCompletionResult, scope string) QueryCompletionCounts {
	var c QueryCompletionCounts
	for _, k := range r.Counters {
		if k.Scope == scope {
			c.add(k.QueryCompletionCounts)
		}
	}
	return c
}

func TestQueryCompletionDenominatorsAreDerivedFromTheRegistry(t *testing.T) {
	f := faninFixtureFor(t)
	want := map[string][]QueryScopeCounter{
		QueryScopeOrdinary:  {{Kind: "STRUCTURAL_ONLY", Platform: "windows-amd64", Required: 53104}, {Kind: "STRUCTURAL_ONLY", Platform: "linux-amd64", Required: 53104}},
		QueryScopePredicate: {{Kind: "PREDICATE_REQUIRED", Platform: "windows-amd64", Required: 20}, {Kind: "PREDICATE_REQUIRED", Platform: "linux-amd64", Required: 20}, {Kind: "EXPECTED_REJECTION", Platform: "windows-amd64", Required: 47}, {Kind: "EXPECTED_REJECTION", Platform: "linux-amd64", Required: 47}},
		QueryScopeLarge:     {{Kind: "LARGE_WORKLOAD_POSITIVE", Platform: "windows-amd64", Required: 3}, {Kind: "LARGE_WORKLOAD_CONTROL", Platform: "windows-amd64", Required: 1}},
	}
	for scope, rows := range want {
		if got := f.plan.rows[scope]; !slices.Equal(got, rows) {
			t.Errorf("%s rows %+v, want %+v", scope, got, rows)
		}
	}
	svcWant := map[string]uint64{SvcKindHistory: 110, SvcKindEditPoint: 41, SvcKindSegment: 109, SvcKindCodeBehind: 2, SvcKindNegative: 1, SvcKindInlineTree: 82, SvcKindQuery: 164}
	var svcTotal uint64
	for _, row := range f.plan.rows[QueryScopeSvc] {
		if row.Required != svcWant[row.Kind] {
			t.Errorf("svc %s/%s = %d", row.Kind, row.Platform, row.Required)
		}
		svcTotal += row.Required
	}
	if svcTotal != 2*509 || len(f.plan.rows[QueryScopeSvc]) != 14 {
		t.Fatalf("svc denominator %d", svcTotal)
	}
	if !slices.Equal(f.plan.unregistered, []string{"large-node-occurrence-page-footer"}) {
		t.Fatalf("unregistered %v", f.plan.unregistered)
	}
}

// The real registered state: no scope result exists in the repository, so every obligation is
// BLOCKED by presence and the gate stays closed. An absent scope is never counted as zero.
func TestQueryCompletionAbsentScopesAreBlockedNotZero(t *testing.T) {
	f := faninFixtureFor(t)
	res, err := FoldQueryCompletion(f.request())
	if err != nil {
		t.Fatal(err)
	}
	if res.Completion != QueryCompletionBlocked || res.Code != "QUERY_SCOPE_RESULT_ABSENT" || res.Complete {
		t.Fatalf("verdict %s %s complete=%v", res.Completion, res.Code, res.Complete)
	}
	want := QueryCompletionCounts{Required: 106208 + 134 + 4 + 1018, Blocked: 106208 + 134 + 4 + 1018}
	if res.Total != want {
		t.Fatalf("total %+v, want %+v", res.Total, want)
	}
	if len(res.Hosts) != 2 || res.Hosts[0].Required != 53104+67+509+4 || res.Hosts[1].Required != 53104+67+509 || res.Hosts[0].Blocked != res.Hosts[0].Required {
		t.Fatalf("hosts %+v", res.Hosts)
	}
	for _, s := range res.Scopes {
		if s.Present || s.ResultSHA256 != "" {
			t.Errorf("absent scope reported present: %+v", s)
		}
	}
}

func TestQueryCompletionFoldsRealScopeResults(t *testing.T) {
	f := faninFixtureFor(t)
	ordinary := f.ordinaryScope(t, ordinaryTupleVerified, ordinaryTupleBlocked)
	predicate, svc := f.predicateScope(t), f.svcScope(t)
	res, err := FoldQueryCompletion(f.request(ordinary, predicate, svc))
	if err != nil {
		t.Fatal(err)
	}
	// large is absent, nothing failed: BLOCKED, never COMPLETE
	if res.Completion != QueryCompletionBlocked || res.Code != "QUERY_SCOPE_RESULT_ABSENT" || res.Complete {
		t.Fatalf("verdict %s %s", res.Completion, res.Code)
	}
	for scope, want := range map[string]QueryCompletionCounts{
		// 2 judged of the 4 selected: 1 VERIFIED, 1 BLOCKED, 2 UNPROCESSED; the other 106,204 were never selected
		QueryScopeOrdinary:  {Required: 106208, Verified: 1, Blocked: 106205, Unprocessed: 2},
		QueryScopePredicate: {Required: 134, Verified: 67, Blocked: 67},
		QueryScopeSvc:       {Required: 1018, Verified: 263, Blocked: 755},
		QueryScopeLarge:     {Required: 4, Blocked: 4},
	} {
		if got := totalsOf(res, scope); got != want {
			t.Errorf("%s %+v, want %+v", scope, got, want)
		}
	}
	var sum QueryCompletionCounts
	for _, h := range res.Hosts {
		sum.add(h.QueryCompletionCounts)
		if !h.sums() {
			t.Errorf("host %s does not add up", h.Platform)
		}
	}
	if sum != res.Total || !res.Total.sums() {
		t.Fatalf("hosts %+v total %+v", sum, res.Total)
	}
	raw, err := jsonv2.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeQueryCompletionResult(raw); err != nil {
		t.Fatalf("stored result: %v", err)
	}
}

func TestQueryCompletionFailPreemptsAndKeepsTheFirstCode(t *testing.T) {
	f := faninFixtureFor(t)
	failing := f.ordinaryScope(t, ordinaryTupleFailed, ordinaryTupleVerified)
	res, err := FoldQueryCompletion(f.request(failing, f.predicateScope(t)))
	if err != nil || res.Completion != QueryCompletionFail || res.Code != "CODE_FAILED" || res.Complete || failing.FirstFailure != "CODE_FAILED" {
		t.Fatalf("%s %s %v", res.Completion, res.Code, err)
	}
	// a defect elsewhere is a FAIL that still reports the failure already confirmed by a good scope
	bad := cloneScope(f.predicateScope(t))
	bad.RegistrySHA256 = strings.Repeat("0", 64)
	res, err = FoldQueryCompletion(f.request(failing, seal(bad)))
	if err == nil || res.Completion != QueryCompletionFail || res.Code != "CODE_FAILED" {
		t.Fatalf("defect: %s %s %v", res.Completion, res.Code, err)
	}
}

func TestQueryCompletionRejectsForgedScopeResults(t *testing.T) {
	f := faninFixtureFor(t)
	failing := f.ordinaryScope(t, ordinaryTupleFailed, ordinaryTupleVerified)
	predicate, svc := f.predicateScope(t), f.svcScope(t)
	type forge struct {
		name, code string
		build      func() QueryCompletionRequest
	}
	// forged edits one result; pin=true means the forger also controls the caller pin
	edit := func(base QueryScopeResult, pin bool, others []QueryScopeResult, mutate func(*QueryScopeResult)) func() QueryCompletionRequest {
		return func() QueryCompletionRequest {
			honest := f.request(append([]QueryScopeResult{base}, others...)...)
			r := cloneScope(base)
			mutate(&r)
			r = seal(r)
			req := honest
			req.Scopes = append([]QueryScopeResult{r}, others...)
			if pin {
				req.ResultPins = map[string]string{r.Scope: r.ResultSHA256}
				for _, o := range others {
					req.ResultPins[o.Scope] = o.ResultSHA256
				}
			}
			return req
		}
	}
	rest := []QueryScopeResult{svc}
	forges := []forge{
		{"row omitted (a missing row is not zero)", "QUERY_SCOPE_DENOMINATOR", edit(predicate, true, rest, func(r *QueryScopeResult) { r.Counters = r.Counters[:len(r.Counters)-1] })},
		{"row required zero", "QUERY_SCOPE_DENOMINATOR", edit(svc, true, nil, func(r *QueryScopeResult) {
			r.Counters[0].Required, r.Counters[0].Verified = 0, 0
		})},
		{"denominator shrunk, old pin kept", "QUERY_SCOPE_DENOMINATOR", edit(predicate, true, rest, func(r *QueryScopeResult) {
			r.Counters[0].Required, r.Counters[0].Verified = 19, 19
		})},
		{"denominator shrunk and its pin recomputed", "QUERY_SCOPE_DENOMINATOR_PIN", edit(predicate, true, rest, func(r *QueryScopeResult) {
			r.Counters[0].Required, r.Counters[0].Verified = 19, 19
			r.DenominatorSHA256 = scopeDenominatorDigest(r.Scope, r.Counters)
		})},
		{"denominator pin of another contract", "QUERY_SCOPE_DENOMINATOR_PIN", edit(predicate, true, rest, func(r *QueryScopeResult) { r.DenominatorSHA256 = strings.Repeat("1", 64) })},
		{"result mixed from another registry", "QUERY_SCOPE_REGISTRY_PIN", edit(predicate, true, rest, func(r *QueryScopeResult) { r.RegistrySHA256 = strings.Repeat("2", 64) })},
		{"old revision called new (ordinary r2 result schema)", "QUERY_SCOPE_SCHEMA", edit(predicate, true, rest, func(r *QueryScopeResult) { r.Schema = OrdinaryQuerySemanticsResultSchema })},
		{"old revision called new (r0)", "QUERY_SCOPE_SCHEMA", edit(predicate, true, rest, func(r *QueryScopeResult) { r.Schema = "tsgk-query-scope-result/r0" })},
		{"counters do not add up", "QUERY_SCOPE_SUM", edit(predicate, true, rest, func(r *QueryScopeResult) { r.Counters[0].Verified++ })},
		{"FAIL lowered to BLOCKED, code kept", "QUERY_SCOPE_FAILURE_CODE", edit(failing, true, nil, func(r *QueryScopeResult) {
			r.Counters[0].Failed, r.Counters[0].Blocked = 0, r.Counters[0].Blocked+1
		})},
		{"FAIL lowered to BLOCKED, code dropped, resealed, caller pin is the honest one", "QUERY_SCOPE_RESULT_PIN", edit(failing, false, nil, func(r *QueryScopeResult) {
			r.Counters[0].Failed, r.Counters[0].Blocked, r.FirstFailure = 0, r.Counters[0].Blocked+1, ""
		})},
		{"BLOCKED raised to VERIFIED, resealed, caller pin is the honest one", "QUERY_SCOPE_RESULT_PIN", edit(predicate, false, rest, func(r *QueryScopeResult) {
			r.Counters[1].Verified, r.Counters[1].Blocked = r.Counters[1].Required, 0
		})},
		{"VERIFIED without any evidence pin", "QUERY_SCOPE_ASSET_PIN", edit(svc, true, nil, func(r *QueryScopeResult) { r.Assets = []queryregistrycore.Pin{} })},
		{"VERIFIED on an evidence pin the caller did not select", "QUERY_SCOPE_ASSET_PIN", edit(svc, true, nil, func(r *QueryScopeResult) { r.Assets[0].SHA256 = strings.Repeat("3", 64) })},
		{"large VERIFIED with no registered evidence", "QUERY_SCOPE_ASSET_PIN", edit(QueryScopeResult{Schema: QueryScopeResultSchema, Scope: QueryScopeLarge, RegistrySHA256: f.v.SHA256(), Assets: []queryregistrycore.Pin{}, Counters: []QueryScopeCounter{}}, true, nil, func(r *QueryScopeResult) {
			r.Counters = slices.Clone(f.plan.rows[QueryScopeLarge])
			for i := range r.Counters {
				r.Counters[i].Verified = r.Counters[i].Required
			}
			r.DenominatorSHA256 = f.plan.denominator(QueryScopeLarge)
		})},
		{"result digest edited without resealing", "QUERY_SCOPE_RESULT_DIGEST", func() QueryCompletionRequest {
			r := cloneScope(predicate)
			r.Counters[1].Verified, r.Counters[1].Blocked = r.Counters[1].Required, 0
			req := f.request(predicate)
			req.Scopes = []QueryScopeResult{r}
			return req
		}},
		{"no caller pin for a present scope", "QUERY_SCOPE_RESULT_UNPINNED", func() QueryCompletionRequest {
			req := f.request(predicate)
			req.ResultPins = map[string]string{}
			return req
		}},
		{"scope result twice", "QUERY_SCOPE_DUPLICATE", func() QueryCompletionRequest { return f.request(predicate, predicate) }},
		{"unknown scope", "QUERY_SCOPE_UNKNOWN", func() QueryCompletionRequest {
			r := cloneScope(predicate)
			r.Scope = "other"
			return f.request(seal(r))
		}},
	}
	for _, c := range forges {
		t.Run(c.name, func(t *testing.T) {
			res, err := FoldQueryCompletion(c.build())
			if err == nil || faninCode(t, err) != c.code {
				t.Fatalf("code %v, want %s", err, c.code)
			}
			if res.Completion != QueryCompletionFail || res.Complete {
				t.Fatalf("forged input produced %s complete=%v", res.Completion, res.Complete)
			}
		})
	}
}

func TestOrdinaryScopeResultRejectsUnboundOverlappingAndForeignResults(t *testing.T) {
	f := faninFixtureFor(t)
	ids := f.structuralIDs("windows-amd64", 4)
	fold := func(results ...OrdinaryQuerySemanticsResult) (QueryScopeResult, error) {
		return OrdinaryScopeResult(f.v, nil, results)
	}
	if _, err := fold(faninShard(ids, []string{ordinaryTupleVerified}, false)); err == nil || faninCode(t, err) != "QUERY_SCOPE_ORDINARY_UNBOUND" {
		t.Errorf("VERIFIED without the external binding: %v", err)
	}
	if _, err := fold(faninShard(ids[:2], []string{ordinaryTupleBlocked}, false), faninShard(ids[1:3], []string{ordinaryTupleBlocked}, false)); err == nil || faninCode(t, err) != "QUERY_SCOPE_OVERLAP" {
		t.Errorf("overlapping shards: %v", err)
	}
	var foreign string
	reg, _ := queryregistrycore.Parse(f.req.Registry)
	for _, c := range queryregistrycore.Expand(nil, queryregistrycore.Platforms(), reg.Probes, nil) {
		foreign = c.ID(reg.Namespace)
		break
	}
	if _, err := fold(faninShard([]string{foreign}, []string{ordinaryTupleBlocked}, false)); err == nil || faninCode(t, err) != "QUERY_SCOPE_UNDECLARED" {
		t.Errorf("a predicate ID in the ordinary scope: %v", err)
	}
	other := "qsem1:" + strings.Repeat("7", 64)
	if _, err := fold(faninShard([]string{other}, []string{ordinaryTupleBlocked}, false)); err == nil || faninCode(t, err) != "QUERY_SCOPE_UNDECLARED" {
		t.Errorf("an ID of another namespace: %v", err)
	}
	// a shard FAIL that no tuple carries is not lowered to BLOCKED
	shard := faninShard(ids[:3], []string{ordinaryTupleVerified, ordinaryTupleBlocked}, true)
	shard.finding("SHARD_FAIL", "", "")
	got, err := fold(shard)
	if err != nil || got.FirstFailure != "SHARD_FAIL" {
		t.Fatalf("shard failure: %+v %v", got, err)
	}
	c := got.Counters[0]
	if c.Verified != 1 || c.Failed != 2 || c.Required != 53104 || c.Blocked != 53101 {
		t.Fatalf("counter %+v", c)
	}
}

// The gate opens only for a fully registered, all-VERIFIED state. The real r1 plan has an
// unregistered large denominator, so only a synthetic plan can show the open branch.
func TestQueryCompletionGateOpensOnlyForAnAllVerifiedRegisteredState(t *testing.T) {
	f := faninFixtureFor(t)
	pin := queryregistrycore.Pin{Path: "evidence", Bytes: 1, SHA256: strings.Repeat("c", 64)}
	pins := map[string][]queryregistrycore.Pin{}
	build := func(mutate func(scope string, rows []QueryScopeCounter) (string, bool)) ([]QueryScopeResult, map[string]string) {
		var out []QueryScopeResult
		resultPins := map[string]string{}
		for _, scope := range queryScopeOrder {
			rows := slices.Clone(f.plan.rows[scope])
			for i := range rows {
				rows[i].Verified = rows[i].Required
			}
			first, keep := "", true
			if mutate != nil {
				first, keep = mutate(scope, rows)
			}
			if !keep {
				continue
			}
			r := NewQueryScopeResult(scope, f.plan.registrySHA, []queryregistrycore.Pin{pin}, rows, first)
			pins[scope] = []queryregistrycore.Pin{pin}
			out = append(out, r)
			resultPins[scope] = r.ResultSHA256
		}
		return out, resultPins
	}
	registered := f.plan
	registered.unregistered = nil
	scopes, resultPins := build(nil)
	res, err := registered.fold(scopes, pins, resultPins)
	if err != nil || res.Completion != QueryCompletionComplete || !res.Complete || res.Code != "" || res.Total.Verified != res.Total.Required {
		t.Fatalf("all VERIFIED and registered: %+v %v", res, err)
	}
	if _, err := DecodeQueryCompletionResult(mustJSON(t, res)); err == nil {
		t.Fatal("r1 decoder accepted a COMPLETE result although its plan has an unregistered scope")
	}
	// the real plan cannot complete: the large node/occurrence/page/footer denominator is unregistered
	res, err = f.plan.fold(scopes, pins, resultPins)
	if err != nil || res.Complete || res.Completion != QueryCompletionBlocked || res.Code != "QUERY_COMPLETION_UNREGISTERED_SCOPE" {
		t.Fatalf("real plan: %s %s %v", res.Completion, res.Code, err)
	}
	for _, c := range []struct {
		name, completion, code string
		mutate                 func(scope string, rows []QueryScopeCounter) (string, bool)
	}{
		{"one BLOCKED", QueryCompletionBlocked, "QUERY_COMPLETION_INCOMPLETE", func(s string, r []QueryScopeCounter) (string, bool) {
			if s == QueryScopeSvc {
				r[3].Verified, r[3].Blocked = r[3].Required-1, 1
			}
			return "", true
		}},
		{"one UNPROCESSED", QueryCompletionBlocked, "QUERY_COMPLETION_INCOMPLETE", func(s string, r []QueryScopeCounter) (string, bool) {
			if s == QueryScopeOrdinary {
				r[0].Verified, r[0].Unprocessed = r[0].Required-1, 1
			}
			return "", true
		}},
		{"one scope absent", QueryCompletionBlocked, "QUERY_SCOPE_RESULT_ABSENT", func(s string, r []QueryScopeCounter) (string, bool) { return "", s != QueryScopeLarge }},
		{"one FAILED", QueryCompletionFail, "PREDICATE_CODE", func(s string, r []QueryScopeCounter) (string, bool) {
			if s == QueryScopePredicate {
				r[0].Verified, r[0].Failed = r[0].Required-1, 1
				return "PREDICATE_CODE", true
			}
			return "", true
		}},
		{"FAIL preempts an absent scope", QueryCompletionFail, "SVC_CODE", func(s string, r []QueryScopeCounter) (string, bool) {
			if s == QueryScopeSvc {
				r[0].Verified, r[0].Failed = r[0].Required-1, 1
				return "SVC_CODE", true
			}
			return "", s != QueryScopeLarge
		}},
		{"the first failing scope in scope order owns the code", QueryCompletionFail, "ORDINARY_CODE", func(s string, r []QueryScopeCounter) (string, bool) {
			if s == QueryScopeOrdinary || s == QueryScopeLarge {
				r[0].Verified, r[0].Failed = r[0].Required-1, 1
				return strings.ToUpper(s) + "_CODE", true
			}
			return "", true
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			scopes, resultPins := build(c.mutate)
			res, err := registered.fold(scopes, pins, resultPins)
			if err != nil || res.Completion != c.completion || res.Code != c.code || res.Complete {
				t.Fatalf("%s %s %v", res.Completion, res.Code, err)
			}
		})
	}
}

func TestDecodeQueryScopeResultIsStrictAndRejectsOldRevisions(t *testing.T) {
	f := faninFixtureFor(t)
	r := f.predicateScope(t)
	raw := mustJSON(t, r)
	if got, err := DecodeQueryScopeResult(raw); err != nil || got.ResultSHA256 != r.ResultSHA256 {
		t.Fatalf("round trip: %v", err)
	}
	for name, mutated := range map[string]string{
		"unknown member":    strings.Replace(string(raw), `"scope":`, `"extra":1,"scope":`, 1),
		"missing member":    strings.Replace(string(raw), `"assets":`, `"assetz":`, 1),
		"null counters":     strings.Replace(string(raw), `"counters":[`, `"counters":null,"x":[`, 1),
		"edited counter":    strings.Replace(string(raw), `"verified":20`, `"verified":19`, 1),
		"old revision":      strings.Replace(string(raw), QueryScopeResultSchema, "tsgk-query-semantics-result/r1", 1),
		"trailing value":    string(raw) + "1",
		"ordinary r2 shape": string(mustJSON(t, ordinaryStoredPartial())),
	} {
		if mutated == string(raw) {
			t.Fatalf("%s: mutation did not apply", name)
		}
		if _, err := DecodeQueryScopeResult([]byte(mutated)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := DecodeQueryScopeResult(make([]byte, queryScopeResultMaxBytes+1)); err == nil || faninCode(t, err) != "QUERY_SCOPE_SIZE" {
		t.Errorf("oversize: %v", err)
	}
	if _, err := DecodeQueryCompletionResult([]byte(`{"schema":"tsgk-query-semantics-result/r2"}`)); err == nil {
		t.Error("an old result accepted as a completion result")
	}
}

func TestDecodeQueryCompletionResultRederivesTheVerdict(t *testing.T) {
	f := faninFixtureFor(t)
	blocked, err := FoldQueryCompletion(f.request(f.predicateScope(t)))
	if err != nil {
		t.Fatal(err)
	}
	failed, err := FoldQueryCompletion(f.request(f.ordinaryScope(t, ordinaryTupleFailed)))
	if err != nil || failed.Completion != QueryCompletionFail {
		t.Fatalf("%v %s", err, failed.Completion)
	}
	for _, r := range []QueryCompletionResult{blocked, failed} {
		if _, err := DecodeQueryCompletionResult(mustJSON(t, r)); err != nil {
			t.Fatalf("stored %s: %v", r.Completion, err)
		}
	}
	copyResult := func(r QueryCompletionResult) QueryCompletionResult {
		r.Scopes, r.Counters, r.Hosts, r.UnregisteredScopes = slices.Clone(r.Scopes), slices.Clone(r.Counters), slices.Clone(r.Hosts), slices.Clone(r.UnregisteredScopes)
		return r
	}
	for _, c := range []struct {
		name string
		base QueryCompletionResult
		edit func(*QueryCompletionResult)
		code string
	}{
		{"BLOCKED claimed COMPLETE", blocked, func(r *QueryCompletionResult) { r.Completion, r.Code, r.Complete = QueryCompletionComplete, "", true }, "QUERY_COMPLETION_VERDICT"},
		{"BLOCKED claimed COMPLETE with every counter raised", blocked, func(r *QueryCompletionResult) {
			for i := range r.Counters {
				r.Counters[i].Verified, r.Counters[i].Blocked, r.Counters[i].Unprocessed = r.Counters[i].Required, 0, 0
			}
			r.Completion, r.Code, r.Complete = QueryCompletionComplete, "", true
		}, "QUERY_COMPLETION_ABSENT_SCOPE"},
		{"unregistered scope dropped", blocked, func(r *QueryCompletionResult) { r.UnregisteredScopes = []string{} }, "QUERY_COMPLETION_SCOPES"},
		{"FAIL lowered to BLOCKED", failed, func(r *QueryCompletionResult) {
			r.Completion, r.Code = QueryCompletionBlocked, "QUERY_SCOPE_RESULT_ABSENT"
		}, "QUERY_COMPLETION_VERDICT"},
		{"FAIL code replaced", failed, func(r *QueryCompletionResult) { r.Code = "OTHER" }, "QUERY_COMPLETION_VERDICT"},
		{"failure code dropped from its scope", failed, func(r *QueryCompletionResult) { r.Scopes[0].FirstFailure = "" }, "QUERY_COMPLETION_FAILURE_CODE"},
		{"host sum edited", blocked, func(r *QueryCompletionResult) { r.Hosts[0].Verified++ }, "QUERY_COMPLETION_SUM"},
		{"total edited", blocked, func(r *QueryCompletionResult) { r.Total.Blocked-- }, "QUERY_COMPLETION_SUM"},
		{"counter row edited", blocked, func(r *QueryCompletionResult) { r.Counters[0].Blocked-- }, "QUERY_COMPLETION_SUM"},
		{"absent scope with a verified count", blocked, func(r *QueryCompletionResult) {
			for i, c := range r.Counters {
				if c.Scope == QueryScopeLarge {
					r.Counters[i].Verified, r.Counters[i].Blocked = 1, c.Blocked-1
					r.Hosts = slices.Clone(r.Hosts)
					r.Hosts[0].Verified, r.Hosts[0].Blocked = r.Hosts[0].Verified+1, r.Hosts[0].Blocked-1
					r.Total.Verified, r.Total.Blocked = r.Total.Verified+1, r.Total.Blocked-1
					return
				}
			}
		}, "QUERY_COMPLETION_ABSENT_SCOPE"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := copyResult(c.base)
			c.edit(&r)
			_, err := DecodeQueryCompletionResult(mustJSON(t, r))
			if err == nil || faninCode(t, err) != c.code {
				t.Fatalf("got %v, want %s", err, c.code)
			}
		})
	}
}

// The legacy surfaces keep their meaning: the P0 Fold still refuses positive VERIFIED while its
// assets are UNAVAILABLE, and an ordinary r2 result is not a scope result.
func TestQueryCompletionLeavesLegacyRevisionsUntouched(t *testing.T) {
	f := faninFixtureFor(t)
	ids := f.structuralIDs("windows-amd64", 1)
	var judged queryregistrycore.Judgment
	for _, c := range f.v.Coordinates() {
		if c.ID(f.v.Namespace()) == ids[0] {
			judged = queryregistrycore.Judgment{ID: ids[0], Coordinate: c, State: "VERIFIED"}
		}
	}
	cov, err := f.v.Fold([]queryregistrycore.Shard{{ID: "s", RegistrySHA256: f.v.SHA256(), Judgments: []queryregistrycore.Judgment{judged}}}, false)
	if err == nil || cov.Verdict != "FAIL" || cov.FullScopeComplete {
		t.Fatalf("P0 Fold changed: %+v %v", cov, err)
	}
	if QueryScopeResultSchema == OrdinaryQuerySemanticsResultSchema || QueryCompletionSchema == QualificationSemanticsResultSchema {
		t.Fatal("new revision reuses an old schema string")
	}
}
