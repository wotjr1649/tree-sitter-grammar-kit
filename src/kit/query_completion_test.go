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

	largeAsset, largeFixtures []byte
	large                     LargeExpected
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
		largeAsset, _, _, largeFixtures := largeAssets(t)
		large, err := ParseLargeExpected(largeAsset, registry, inventory, largeFixtures)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := buildFaninPlan(v, svc, large)
		if err != nil {
			t.Fatal(err)
		}
		faninFix = faninFixture{
			req: QueryCompletionRequest{Registry: r.Registry, RegistrySHA256: r.RegistrySHA256, Qualification: r.Qualification, Slots: r.Slots, Basis: r.Basis, SvcExpected: svcAsset, SvcFixture: svcFixture, LargeExpected: largeAsset, LargeFixtures: largeFixtures},
			v:   v, svc: svc, plan: plan, predAsset: predAsset, svcAsset: svcAsset, inventory: inventory,
			largeAsset: largeAsset, largeFixtures: largeFixtures, large: large,
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

// largeJudgments mirrors the recorded #412 state (30 VERIFIED, 9 BLOCKED) without a native run: every
// derivable obligation is VERIFIED, the registered-BLOCKED ones are BLOCKED and the 32 MiB status
// is BLOCKED by the HARD memory cap. The native-derived judgments enter through the current-state
// test in src/internal/native.
func (f *faninFixture) largeJudgments() []LargeJudgment {
	var out []LargeJudgment
	for _, o := range f.large.Obligations() {
		switch {
		case o.Blocked != "":
			out = append(out, LargeJudgment{ID: o.ID, State: "BLOCKED", Code: o.Blocked})
		case o.Kind == LargeKindStatus && o.Case == "cs-large-32mib-errors":
			out = append(out, LargeJudgment{ID: o.ID, State: "BLOCKED", Code: "LARGE_MEMORY_CAP_4GIB_EXCEEDED"})
		default:
			out = append(out, LargeJudgment{ID: o.ID, State: "VERIFIED"})
		}
	}
	return out
}

func (f *faninFixture) largeScope(t *testing.T, mutate func([]LargeJudgment) []LargeJudgment) QueryScopeResult {
	t.Helper()
	j := f.largeJudgments()
	if mutate != nil {
		j = mutate(j)
	}
	r, err := LargeScopeResult(f.large, f.largeAsset, f.req.Registry, j)
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
		QueryScopeLarge:     {queryregistrycore.FilePin(LargeExpectedPath, f.largeAsset)},
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
		// the 39 lower obligations replace the 4 P0 coordinates; Linux/macOS are N/A and have no row
		QueryScopeLarge: {{Kind: LargeKindSource, Platform: "windows-amd64", Required: 3}, {Kind: LargeKindStatus, Platform: "windows-amd64", Required: 3},
			{Kind: LargeKindCSTPage, Platform: "windows-amd64", Required: 14}, {Kind: LargeKindCSTFooter, Platform: "windows-amd64", Required: 3},
			{Kind: LargeKindOccPage, Platform: "windows-amd64", Required: 12}, {Kind: LargeKindOccFooter, Platform: "windows-amd64", Required: 3},
			{Kind: LargeKindControl, Platform: "windows-amd64", Required: 1}},
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
	// Upper and lower are not both counted: 39 lower obligations stand for the 4 P0 coordinates.
	var largeTotal uint64
	for _, row := range f.plan.rows[QueryScopeLarge] {
		largeTotal += row.Required
	}
	if n := uint64(len(f.large.Obligations())); largeTotal != 39 || largeTotal != n {
		t.Fatalf("large denominator %d, obligations %d", largeTotal, n)
	}
	// the P0 `large-node-occurrence-page-footer` is covered for the derivable part only
	if !slices.Equal(f.plan.unregistered, []string{"large-error-recovery-node-occurrence-page-footer"}) {
		t.Fatalf("unregistered %v", f.plan.unregistered)
	}
	if na := f.plan.notApplicable[QueryScopeLarge]; len(na) != 2 || na[0].Platform != "darwin-arm64" || na[1].Platform != "linux-amd64" || na[0].Reason == "" || na[0].Reason != na[1].Reason {
		t.Fatalf("N/A %+v", na)
	}
}

// Every P0 upper coordinate has lower obligations and none is left over: this is what lets the
// lower obligations replace the upper ones.
func TestQueryCompletionLargeLowerObligationsRefineTheUpperCoordinates(t *testing.T) {
	f := faninFixtureFor(t)
	if err := checkLargeBinding(f.v, f.large); err != nil {
		t.Fatal(err)
	}
	lower := map[string]map[string]int{}
	for _, o := range f.large.Obligations() {
		if lower[o.Case] == nil {
			lower[o.Case] = map[string]int{}
		}
		lower[o.Case][o.Kind]++
	}
	var upper int
	for _, c := range f.v.Coordinates() {
		if c.Kind != queryLargePositive && c.Kind != queryLargeControl {
			continue
		}
		upper++
		if c.Kind == queryLargePositive && (lower[c.Case][LargeKindSource] != 1 || lower[c.Case][LargeKindStatus] != 1 || lower[c.Case][LargeKindCSTPage] == 0 || lower[c.Case][LargeKindOccFooter] != 1) {
			t.Errorf("%s has no complete lower obligations: %v", c.Case, lower[c.Case])
		}
		if c.Kind == queryLargeControl && lower[c.Case][LargeKindControl] != 1 {
			t.Errorf("control %s has no lower obligation", c.Case)
		}
	}
	if upper != 4 || len(lower) != 4 {
		t.Fatalf("upper %d, cases with lower obligations %d", upper, len(lower))
	}
	// a contract that drops a case, renames the control or changes the N/A hosts is not this registry's contract
	for name, mutate := range map[string]func(*LargeExpected){
		"case dropped":    func(e *LargeExpected) { e.Cases = e.Cases[:2] },
		"case renamed":    func(e *LargeExpected) { e.Cases = slices.Clone(e.Cases); e.Cases[1].ID = "cs-large-other" },
		"control renamed": func(e *LargeExpected) { e.Control.ID = "cs-large-other" },
		"N/A host added": func(e *LargeExpected) {
			e.NotApplicable = map[string]string{"darwin-arm64": "r", "linux-amd64": "r", "windows-amd64": "r"}
		},
		"N/A host swapped": func(e *LargeExpected) {
			e.NotApplicable = map[string]string{"darwin-arm64": "r", "freebsd-amd64": "r"}
		},
	} {
		e := f.large
		mutate(&e)
		if _, err := buildFaninPlan(f.v, f.svc, e); err == nil || faninCode(t, err) != "QUERY_COMPLETION_LARGE_BINDING" {
			t.Errorf("%s: %v", name, err)
		}
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
	want := QueryCompletionCounts{Required: 106208 + 134 + 39 + 1018, Blocked: 106208 + 134 + 39 + 1018}
	if res.Total != want {
		t.Fatalf("total %+v, want %+v", res.Total, want)
	}
	if len(res.Hosts) != 2 || res.Hosts[0].Required != 53104+67+509+39 || res.Hosts[1].Required != 53104+67+509 || res.Hosts[0].Blocked != res.Hosts[0].Required {
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
		QueryScopeLarge:     {Required: 39, Blocked: 39},
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
	largeScope := f.largeScope(t, nil)
	largeFailing := f.largeScope(t, func(js []LargeJudgment) []LargeJudgment {
		for i, j := range js {
			if strings.HasPrefix(j.ID, LargeKindControl+"/") {
				js[i] = VerifyLargeControl(f.large.Control, StatusCompleted, "", AssessPass, false)
			}
		}
		return js
	})
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
		// large: the lower obligations are the denominator; every shape below must be refused
		{"large VERIFIED with no registered evidence", "QUERY_SCOPE_ASSET_PIN", edit(largeScope, true, nil, func(r *QueryScopeResult) { r.Assets = []queryregistrycore.Pin{} })},
		{"large VERIFIED on an evidence pin of another contract", "QUERY_SCOPE_ASSET_PIN", edit(largeScope, true, nil, func(r *QueryScopeResult) { r.Assets[0] = queryregistrycore.FilePin(SvcExpectedPath, f.svcAsset) })},
		{"large denominator shrunk (a row dropped), old pin kept", "QUERY_SCOPE_DENOMINATOR", edit(largeScope, true, nil, func(r *QueryScopeResult) { r.Counters = r.Counters[:len(r.Counters)-1] })},
		{"large denominator shrunk (CST pages 14 to 12), old pin kept", "QUERY_SCOPE_DENOMINATOR", edit(largeScope, true, nil, func(r *QueryScopeResult) {
			r.Counters[2].Required, r.Counters[2].Blocked = 12, 0
		})},
		{"large denominator shrunk and its pin recomputed", "QUERY_SCOPE_DENOMINATOR_PIN", edit(largeScope, true, nil, func(r *QueryScopeResult) {
			r.Counters[2].Required, r.Counters[2].Blocked = 12, 0
			r.DenominatorSHA256 = scopeDenominatorDigest(r.Scope, r.Counters)
		})},
		{"large counted twice: the 4 upper rows added to the lower rows", "QUERY_SCOPE_DENOMINATOR", edit(largeScope, true, nil, func(r *QueryScopeResult) {
			r.Counters = append(r.Counters, QueryScopeCounter{Kind: queryLargePositive, Platform: "windows-amd64", Required: 3, Verified: 3}, QueryScopeCounter{Kind: queryLargeControl, Platform: "windows-amd64", Required: 1, Verified: 1})
		})},
		{"large counted twice, pin recomputed", "QUERY_SCOPE_DENOMINATOR_PIN", edit(largeScope, true, nil, func(r *QueryScopeResult) {
			r.Counters = append(r.Counters, QueryScopeCounter{Kind: queryLargePositive, Platform: "windows-amd64", Required: 3, Verified: 3}, QueryScopeCounter{Kind: queryLargeControl, Platform: "windows-amd64", Required: 1, Verified: 1})
			r.DenominatorSHA256 = scopeDenominatorDigest(r.Scope, r.Counters)
		})},
		{"large with the old 4-obligation denominator (upper only), pin recomputed", "QUERY_SCOPE_DENOMINATOR_PIN", edit(largeScope, true, nil, func(r *QueryScopeResult) {
			r.Counters = []QueryScopeCounter{{Kind: queryLargePositive, Platform: "windows-amd64", Required: 3, Verified: 3}, {Kind: queryLargeControl, Platform: "windows-amd64", Required: 1, Verified: 1}}
			r.DenominatorSHA256 = scopeDenominatorDigest(r.Scope, r.Counters)
		})},
		{"large denominator pin of another scope", "QUERY_SCOPE_DENOMINATOR_PIN", edit(largeScope, true, nil, func(r *QueryScopeResult) { r.DenominatorSHA256 = f.plan.denominator(QueryScopeSvc) })},
		{"large result mixed from another registry", "QUERY_SCOPE_REGISTRY_PIN", edit(largeScope, true, nil, func(r *QueryScopeResult) { r.RegistrySHA256 = strings.Repeat("2", 64) })},
		{"large BLOCKED raised to VERIFIED, pins chosen by the forger", "QUERY_SCOPE_BLOCKED_VERIFIED", edit(largeScope, true, nil, func(r *QueryScopeResult) {
			r.Counters[2].Verified, r.Counters[2].Blocked = r.Counters[2].Required, 0
		})},
		{"large BLOCKED raised to VERIFIED, caller pin is the honest one", "QUERY_SCOPE_RESULT_PIN", edit(largeScope, false, nil, func(r *QueryScopeResult) {
			r.Counters[1].Verified, r.Counters[1].Blocked = r.Counters[1].Required, 0
		})},
		{"large N/A host counted as VERIFIED (a Linux row), old pin kept", "QUERY_SCOPE_DENOMINATOR", edit(largeScope, true, nil, func(r *QueryScopeResult) {
			r.Counters = append(r.Counters, QueryScopeCounter{Kind: LargeKindSource, Platform: "linux-amd64", Required: 3, Verified: 3})
		})},
		{"large N/A host counted as VERIFIED (a Linux row), pin recomputed", "QUERY_SCOPE_DENOMINATOR_PIN", edit(largeScope, true, nil, func(r *QueryScopeResult) {
			r.Counters = append(r.Counters, QueryScopeCounter{Kind: LargeKindSource, Platform: "linux-amd64", Required: 3, Verified: 3})
			r.DenominatorSHA256 = scopeDenominatorDigest(r.Scope, r.Counters)
		})},
		{"large N/A host moved onto the Windows rows", "QUERY_SCOPE_DENOMINATOR", edit(largeScope, true, nil, func(r *QueryScopeResult) { r.Counters[0].Platform = "linux-amd64" })},
		{"large FAIL lowered to BLOCKED, code kept", "QUERY_SCOPE_FAILURE_CODE", edit(largeFailing, true, nil, func(r *QueryScopeResult) {
			i := len(r.Counters) - 1
			r.Counters[i].Failed, r.Counters[i].Blocked = 0, 1
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
	// A future revision whose error-recovery fixtures were derivable would have no registered-BLOCKED
	// obligation; this synthetic plan stands for it. The real r1 plan keeps them registered BLOCKED.
	registered := f.plan
	registered.unregistered = nil
	registered.blockedFloor = map[string][]uint64{}
	for scope, floor := range f.plan.blockedFloor {
		registered.blockedFloor[scope] = make([]uint64, len(floor))
	}
	scopes, resultPins := build(nil)
	res, err := registered.fold(scopes, pins, resultPins)
	if err != nil || res.Completion != QueryCompletionComplete || !res.Complete || res.Code != "" || res.Total.Verified != res.Total.Required {
		t.Fatalf("all VERIFIED and registered: %+v %v", res, err)
	}
	if _, err := DecodeQueryCompletionResult(mustJSON(t, res)); err == nil {
		t.Fatal("r1 decoder accepted a COMPLETE result although its plan has an unregistered scope")
	}
	// the real plan cannot complete. (1) The registered-BLOCKED obligations of the large contract can
	// never be VERIFIED: an all-VERIFIED large result is refused outright.
	res, err = f.plan.fold(scopes, pins, resultPins)
	if err == nil || faninCode(t, err) != "QUERY_SCOPE_BLOCKED_VERIFIED" || res.Complete || res.Completion != QueryCompletionFail {
		t.Fatalf("real plan, all VERIFIED: %s %s %v", res.Completion, res.Code, err)
	}
	// (2) Even without them, the error-recovery denominator stays unregistered in r1.
	floorless := f.plan
	floorless.blockedFloor = registered.blockedFloor
	res, err = floorless.fold(scopes, pins, resultPins)
	if err != nil || res.Complete || res.Completion != QueryCompletionBlocked || res.Code != "QUERY_COMPLETION_UNREGISTERED_SCOPE" {
		t.Fatalf("real plan, unregistered scope: %s %s %v", res.Completion, res.Code, err)
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
		{"N/A reason erased", blocked, func(r *QueryCompletionResult) {
			r.Scopes[3].NotApplicable = []QueryCompletionNotApplicable{{"darwin-arm64", ""}, {"linux-amd64", "x"}}
		}, "QUERY_COMPLETION_NOT_APPLICABLE"},
		{"N/A hosts dropped", blocked, func(r *QueryCompletionResult) { r.Scopes[3].NotApplicable = []QueryCompletionNotApplicable{} }, "QUERY_COMPLETION_NOT_APPLICABLE"},
		{"one N/A host dropped", blocked, func(r *QueryCompletionResult) {
			r.Scopes[3].NotApplicable = []QueryCompletionNotApplicable{{"linux-amd64", "x"}}
		}, "QUERY_COMPLETION_NOT_APPLICABLE"},
		{"N/A host claimed by another scope", blocked, func(r *QueryCompletionResult) {
			r.Scopes[0].NotApplicable = []QueryCompletionNotApplicable{{"linux-amd64", "x"}}
		}, "QUERY_COMPLETION_NOT_APPLICABLE"},
		{"N/A host counted as a row", blocked, func(r *QueryCompletionResult) {
			r.Counters = append(r.Counters, QueryCompletionCounter{Scope: QueryScopeLarge, Kind: LargeKindSource, Platform: "linux-amd64", QueryCompletionCounts: QueryCompletionCounts{Required: 3, Verified: 3}})
		}, "QUERY_COMPLETION_COUNTER"},
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

// The large scope at the recorded #412 state: 30 of 39 VERIFIED, 9 BLOCKED, and the gate stays
// closed (BLOCKED, never COMPLETE). The N/A hosts keep their reasons and add nothing to the counts.
func TestQueryCompletionFoldsTheLargeScopeAtTheRecordedState(t *testing.T) {
	f := faninFixtureFor(t)
	large := f.largeScope(t, nil)
	if large.FirstFailure != "" {
		t.Fatalf("large failure %q", large.FirstFailure)
	}
	res, err := FoldQueryCompletion(f.request(f.ordinaryScope(t, ordinaryTupleVerified), f.predicateScope(t), f.svcScope(t), large))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := totalsOf(res, QueryScopeLarge), (QueryCompletionCounts{Required: 39, Verified: 30, Blocked: 9}); got != want {
		t.Fatalf("large %+v, want %+v", got, want)
	}
	for _, c := range res.Counters {
		if c.Scope == QueryScopeLarge && c.Platform != "windows-amd64" {
			t.Errorf("N/A host counted: %+v", c)
		}
	}
	if res.Completion != QueryCompletionBlocked || res.Code != "QUERY_COMPLETION_INCOMPLETE" || res.Complete {
		t.Fatalf("verdict %s %s", res.Completion, res.Code)
	}
	var na []QueryCompletionNotApplicable
	for _, s := range res.Scopes {
		if s.Scope == QueryScopeLarge {
			na = s.NotApplicable
			if !s.Present || s.ResultSHA256 != large.ResultSHA256 {
				t.Errorf("large scope %+v", s)
			}
		} else if len(s.NotApplicable) != 0 {
			t.Errorf("%s has N/A %+v", s.Scope, s.NotApplicable)
		}
	}
	if len(na) != 2 || na[0].Platform != "darwin-arm64" || na[1].Platform != "linux-amd64" || na[0].Reason != f.large.NotApplicable["darwin-arm64"] || na[1].Reason != f.large.NotApplicable["linux-amd64"] {
		t.Fatalf("N/A %+v", na)
	}
	// the N/A reasons survive an absent large scope, too: they come from the pinned contract
	res, err = FoldQueryCompletion(f.request(f.predicateScope(t)))
	if err != nil || len(res.Scopes[3].NotApplicable) != 2 || res.Scopes[3].Present {
		t.Fatalf("absent large scope: %+v %v", res.Scopes[3], err)
	}
	if _, err := DecodeQueryCompletionResult(mustJSON(t, res)); err != nil {
		t.Fatalf("stored result: %v", err)
	}
}

// A control that stays over the limit is never a semantic PASS: a completed or PASS outcome is a
// FAILED control and the fan-in turns it into a FAIL that keeps the control's code.
func TestQueryCompletionLargeControlNeverBecomesPass(t *testing.T) {
	f := faninFixtureFor(t)
	passed := f.largeScope(t, func(js []LargeJudgment) []LargeJudgment {
		for i, j := range js {
			if strings.HasPrefix(j.ID, LargeKindControl+"/") {
				js[i] = VerifyLargeControl(f.large.Control, StatusCompleted, "", AssessPass, false)
			}
		}
		return js
	})
	res, err := FoldQueryCompletion(f.request(passed))
	if err != nil || res.Completion != QueryCompletionFail || res.Code != "LARGE_CONTROL_PASSED" || res.Complete {
		t.Fatalf("control PASS: %s %s %v", res.Completion, res.Code, err)
	}
	// the honest control is the registered exact status and code
	if j := VerifyLargeControl(f.large.Control, f.large.Control.Status, f.large.Control.Code, AssessFail, false); j.State != "VERIFIED" {
		t.Fatalf("honest control %+v", j)
	}
	// raising the FAILED control to VERIFIED in the stored result changes its digest; the caller's pin refuses it
	raised := cloneScope(passed)
	for i := range raised.Counters {
		if raised.Counters[i].Kind == LargeKindControl {
			raised.Counters[i].Failed, raised.Counters[i].Verified = 0, 1
		}
	}
	raised.FirstFailure = ""
	req := f.request(passed)
	req.Scopes = []QueryScopeResult{seal(raised)}
	if _, err := FoldQueryCompletion(req); err == nil || faninCode(t, err) != "QUERY_SCOPE_RESULT_PIN" {
		t.Fatalf("control FAIL raised to VERIFIED: %v", err)
	}
}

// The contract bytes the plan reads are bound to the registry: another large contract is refused
// before any scope result is looked at.
func TestQueryCompletionRefusesAnotherLargeContract(t *testing.T) {
	f := faninFixtureFor(t)
	for name, edit := range map[string]func(*QueryCompletionRequest){
		"control semantic_pass flipped": func(r *QueryCompletionRequest) {
			r.LargeExpected = []byte(strings.Replace(string(f.largeAsset), `"semantic_pass": false`, `"semantic_pass": true`, 1))
		},
		"fixture inventory of another content": func(r *QueryCompletionRequest) {
			r.LargeFixtures = append(slices.Clone(f.largeFixtures), ' ')
		},
		"contract of another scope": func(r *QueryCompletionRequest) { r.LargeExpected = f.svcAsset },
		"no contract":               func(r *QueryCompletionRequest) { r.LargeExpected = nil },
	} {
		req := f.request(f.predicateScope(t))
		edit(&req)
		res, err := FoldQueryCompletion(req)
		if err == nil || res.Completion != QueryCompletionFail || res.Complete {
			t.Errorf("%s: %s %v", name, res.Completion, err)
		}
	}
}

// A shard whose FAIL no tuple carries (every selected tuple VERIFIED, the producer's FAIL kept
// beside them) must still reach the fan-in as a FAIL with its first code.
func TestOrdinaryScopeResultKeepsAShardFailThatEveryTupleVerifies(t *testing.T) {
	f := faninFixtureFor(t)
	ids := f.structuralIDs("windows-amd64", 3)
	shard := faninShard(ids, []string{ordinaryTupleVerified, ordinaryTupleVerified, ordinaryTupleVerified}, true)
	shard.ProducerRecorded = Verdict{StatusCompleted, AssessFail}
	shard.finding("PRODUCER_FAIL", "", "")
	if !validOrdinaryStoredResult(shard) {
		t.Fatal("the shard must be a valid stored result")
	}
	scope, err := OrdinaryScopeResult(f.v, []queryregistrycore.Pin{faninOrdinaryAsset}, []OrdinaryQuerySemanticsResult{shard})
	if err != nil {
		t.Fatal(err)
	}
	c := scope.Counters[0]
	if scope.FirstFailure != "PRODUCER_FAIL" || c.Failed == 0 || c.Verified+c.Failed != 3 {
		t.Fatalf("the shard FAIL was lost: first=%q counter=%+v", scope.FirstFailure, c)
	}
	res, err := FoldQueryCompletion(f.request(scope))
	if err != nil || res.Completion != QueryCompletionFail || res.Code != "PRODUCER_FAIL" || res.Total.Failed == 0 {
		t.Fatalf("fan-in %s %s failed=%d: %v", res.Completion, res.Code, res.Total.Failed, err)
	}
	// a FAIL shard that selects nothing has no tuple to carry its FAIL: it is rejected, not dropped
	empty := newOrdinarySemanticsResult()
	empty.ProducerRecorded = Verdict{StatusCompleted, AssessFail}
	empty.finding("PRODUCER_FAIL", "", "")
	if !validOrdinaryStoredResult(empty) {
		t.Fatal("the empty shard must be a valid stored result")
	}
	if _, err := OrdinaryScopeResult(f.v, nil, []OrdinaryQuerySemanticsResult{empty}); err == nil || faninCode(t, err) != "QUERY_SCOPE_ORDINARY_FAIL_UNCARRIED" {
		t.Errorf("a FAIL shard without tuples: %v", err)
	}
}

// A counter sum that wraps around uint64 is not a distribution: required = verified + failed +
// blocked + unprocessed must hold without overflow at every level and in the stored decoder.
func TestQueryCompletionRejectsCounterSumsThatWrap(t *testing.T) {
	huge := ^uint64(0)
	if (QueryCompletionCounts{Required: 53104, Verified: 53104, Blocked: huge, Unprocessed: 1}).sums() {
		t.Error("a wrapping distribution was accepted by sums")
	}
	f := faninFixtureFor(t)
	forge := func(c *QueryScopeCounter) { c.Verified, c.Failed, c.Blocked, c.Unprocessed = c.Required, 0, huge, 1 }
	bad := cloneScope(f.predicateScope(t))
	forge(&bad.Counters[0])
	if _, err := FoldQueryCompletion(f.request(seal(bad))); err == nil || faninCode(t, err) != "QUERY_SCOPE_SUM" {
		t.Errorf("a wrapping scope row was accepted: %v", err)
	}

	blocked, err := FoldQueryCompletion(f.request(f.predicateScope(t)))
	if err != nil {
		t.Fatal(err)
	}
	r := blocked
	r.Counters, r.Hosts = slices.Clone(r.Counters), slices.Clone(r.Hosts)
	for i := range r.Counters {
		if r.Counters[i].Scope == QueryScopePredicate && r.Counters[i].Platform == "windows-amd64" {
			c := &r.Counters[i]
			c.Verified, c.Failed, c.Blocked, c.Unprocessed = c.Required, 0, huge, 1
			break
		}
	}
	// the stored host and total rows a forger would compute with the same wrapping addition
	r.Total = QueryCompletionCounts{}
	for h := range r.Hosts {
		r.Hosts[h].QueryCompletionCounts = QueryCompletionCounts{}
		for _, c := range r.Counters {
			if c.Platform == r.Hosts[h].Platform {
				w := &r.Hosts[h].QueryCompletionCounts
				w.Required, w.Verified, w.Failed, w.Blocked, w.Unprocessed = w.Required+c.Required, w.Verified+c.Verified, w.Failed+c.Failed, w.Blocked+c.Blocked, w.Unprocessed+c.Unprocessed
			}
		}
		w, h := &r.Total, r.Hosts[h].QueryCompletionCounts
		w.Required, w.Verified, w.Failed, w.Blocked, w.Unprocessed = w.Required+h.Required, w.Verified+h.Verified, w.Failed+h.Failed, w.Blocked+h.Blocked, w.Unprocessed+h.Unprocessed
	}
	r.Completion, r.Code = completionVerdict(r)
	r.Complete = r.Completion == QueryCompletionComplete
	if _, err := DecodeQueryCompletionResult(mustJSON(t, r)); err == nil || faninCode(t, err) != "QUERY_COMPLETION_SUM" {
		t.Errorf("a wrapping stored result was accepted: %v", err)
	}
}

// Rows that each sum correctly can still add up past uint64 into a host or total: the stored
// decoder must reject the wrapped host and total rows a forger computes with plain addition.
func TestDecodeQueryCompletionResultRejectsHostTotalsThatWrap(t *testing.T) {
	f := faninFixtureFor(t)
	r, err := FoldQueryCompletion(f.request(f.predicateScope(t)))
	if err != nil {
		t.Fatal(err)
	}
	r.Counters, r.Hosts = slices.Clone(r.Counters), slices.Clone(r.Hosts)
	for i := range r.Counters {
		if r.Counters[i].Scope == QueryScopePredicate && r.Counters[i].Platform == "windows-amd64" {
			c := &r.Counters[i]
			c.Required, c.Verified, c.Failed, c.Blocked, c.Unprocessed = ^uint64(0), 0, 0, ^uint64(0), 0
			break
		}
	}
	r.Total = QueryCompletionCounts{}
	for h := range r.Hosts {
		w := QueryCompletionCounts{}
		for _, c := range r.Counters {
			if c.Platform == r.Hosts[h].Platform {
				w.Required, w.Verified, w.Failed, w.Blocked, w.Unprocessed = w.Required+c.Required, w.Verified+c.Verified, w.Failed+c.Failed, w.Blocked+c.Blocked, w.Unprocessed+c.Unprocessed
			}
		}
		r.Hosts[h].QueryCompletionCounts = w
		r.Total.Required, r.Total.Verified, r.Total.Failed, r.Total.Blocked, r.Total.Unprocessed = r.Total.Required+w.Required, r.Total.Verified+w.Verified, r.Total.Failed+w.Failed, r.Total.Blocked+w.Blocked, r.Total.Unprocessed+w.Unprocessed
	}
	r.Completion, r.Code = completionVerdict(r)
	r.Complete = r.Completion == QueryCompletionComplete
	if _, err := DecodeQueryCompletionResult(mustJSON(t, r)); err == nil || faninCode(t, err) != "QUERY_COMPLETION_SUM" {
		t.Errorf("a stored result whose host totals wrap was accepted: %v", err)
	}
}
