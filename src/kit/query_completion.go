package kit

import (
	jsonv2 "encoding/json/v2"
	"reflect"
	"regexp"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The query completion fan-in (#362 scope 7) folds the scope-local results of the four #362
// scopes (ordinary, predicate, SVC, large) into one completion verdict. It is a new revision:
// the r9 ordinary replay, the r10 qualification result and the P0 Fold keep their meaning. It
// reads no file and makes no external claim: a scope result is a stored claim of the verifier
// that produced it, and the fan-in checks presence, integrity and arithmetic, not authenticity.

const (
	QueryScopeResultSchema = "tsgk-query-scope-result/r1"
	QueryCompletionSchema  = "tsgk-query-completion/r1"
	// shortcut: a scope result has fewer than 64 counter rows; raise only with a new revision.
	queryScopeResultMaxBytes = 256 * 1024

	QueryScopeOrdinary  = "ordinary"
	QueryScopePredicate = "predicate"
	QueryScopeSvc       = "svc"
	QueryScopeLarge     = "large"

	QueryCompletionComplete = "COMPLETE"
	QueryCompletionBlocked  = "BLOCKED"
	QueryCompletionFail     = "FAIL"

	queryLargePositive = "LARGE_WORKLOAD_POSITIVE"
	queryLargeControl  = "LARGE_WORKLOAD_CONTROL"
)

// queryCompletionUnregistered lists the P0 missing scope that revision r1 cannot close: the large
// node/occurrence/page/footer denominator has no registered contract. The SVC denominator is
// closed by query-svc-expected-r1; independent-assets and current-cohort-approval-replay are not
// listed because r1 accepts a VERIFIED count only with a caller-pinned evidence asset.
var queryCompletionUnregistered = []string{"large-node-occurrence-page-footer"}

var (
	queryScopeOrder    = []string{QueryScopeOrdinary, QueryScopePredicate, QueryScopeSvc, QueryScopeLarge}
	queryPlatformOrder = []string{"windows-amd64", "linux-amd64"}
	queryKindOrder     = map[string][]string{
		QueryScopeOrdinary:  {queryregistrycore.Structural},
		QueryScopePredicate: {queryregistrycore.Predicate, queryregistrycore.Control},
		QueryScopeSvc:       {SvcKindHistory, SvcKindEditPoint, SvcKindSegment, SvcKindCodeBehind, SvcKindNegative, SvcKindInlineTree, SvcKindQuery},
		QueryScopeLarge:     {queryLargePositive, queryLargeControl},
	}
	queryHex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// QueryScopeCounter keeps required = verified + failed + blocked + unprocessed per kind and host.
type QueryScopeCounter struct {
	Kind        string `json:"kind"`
	Platform    string `json:"platform"`
	Required    uint64 `json:"required"`
	Verified    uint64 `json:"verified"`
	Failed      uint64 `json:"failed"`
	Blocked     uint64 `json:"blocked"`
	Unprocessed uint64 `json:"unprocessed"`
}

// QueryScopeResult is the stored, scope-local input of the fan-in. Assets are the evidence pins
// the VERIFIED counts rest on; FirstFailure is the code of the first FAILED obligation in the
// scope's consumption order.
type QueryScopeResult struct {
	Schema            string                  `json:"schema"`
	Scope             string                  `json:"scope"`
	RegistrySHA256    string                  `json:"registry_sha256"`
	DenominatorSHA256 string                  `json:"denominator_sha256"`
	Assets            []queryregistrycore.Pin `json:"assets"`
	Counters          []QueryScopeCounter     `json:"counters"`
	FirstFailure      string                  `json:"first_failure"`
	ResultSHA256      string                  `json:"result_sha256"`
}

func queryCompletionFault(code string) error {
	return fail(KindInvalidInput, code, "query-completion", nil)
}

// strictRoundTrip rejects unknown, missing and null members: the typed value must encode back
// to exactly the document that was read.
func strictRoundTrip(data []byte, v any) error {
	if err := jsonv2.Unmarshal(data, v, jsonv2.RejectUnknownMembers(true)); err != nil {
		return err
	}
	encoded, err := jsonv2.Marshal(v)
	if err != nil {
		return err
	}
	var a, b any
	if jsonv2.Unmarshal(data, &a) != nil || jsonv2.Unmarshal(encoded, &b) != nil || !reflect.DeepEqual(a, b) {
		return queryCompletionFault("QUERY_COMPLETION_FIELD_SHAPE")
	}
	return nil
}

func scopeDenominatorDigest(scope string, counters []QueryScopeCounter) string {
	rows := make([]any, 0, len(counters))
	for _, c := range counters {
		rows = append(rows, []any{c.Kind, c.Platform, c.Required})
	}
	return queryregistrycore.Digest(queryregistrycore.Canonical([]any{"tsgk-query-scope-denominator/r1", scope, rows}))
}

func (r QueryScopeResult) digest() string {
	return queryregistrycore.Digest(queryregistrycore.Canonical([]any{QueryScopeResultSchema, r.Scope, r.RegistrySHA256, r.DenominatorSHA256, r.Assets, r.Counters, r.FirstFailure}))
}

// NewQueryScopeResult seals a scope-local fold. Counters are put in the canonical kind and host
// order; the denominator pin is the digest of their required counts.
func NewQueryScopeResult(scope, registrySHA string, assets []queryregistrycore.Pin, counters []QueryScopeCounter, firstFailure string) QueryScopeResult {
	counters = slices.Clone(counters)
	slices.SortFunc(counters, func(a, b QueryScopeCounter) int {
		if d := slices.Index(queryKindOrder[scope], a.Kind) - slices.Index(queryKindOrder[scope], b.Kind); d != 0 {
			return d
		}
		return slices.Index(queryPlatformOrder, a.Platform) - slices.Index(queryPlatformOrder, b.Platform)
	})
	if assets == nil {
		assets = []queryregistrycore.Pin{}
	}
	r := QueryScopeResult{Schema: QueryScopeResultSchema, Scope: scope, RegistrySHA256: registrySHA, DenominatorSHA256: scopeDenominatorDigest(scope, counters), Assets: assets, Counters: counters, FirstFailure: firstFailure}
	r.ResultSHA256 = r.digest()
	return r
}

// DecodeQueryScopeResult strictly decodes a stored scope result and checks its own digest. It
// does not know the registry; FoldQueryCompletion binds it to one.
func DecodeQueryScopeResult(data []byte) (QueryScopeResult, error) {
	var r QueryScopeResult
	if len(data) > queryScopeResultMaxBytes {
		return r, fail(KindResourceLimit, "QUERY_SCOPE_SIZE", "query-scope-result", nil)
	}
	if err := strictRoundTrip(data, &r); err != nil {
		return QueryScopeResult{}, fail(KindInvalidInput, "QUERY_SCOPE_JSON", "query-scope-result", err)
	}
	if r.Schema != QueryScopeResultSchema {
		return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_SCHEMA")
	}
	if r.ResultSHA256 != r.digest() {
		return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_RESULT_DIGEST")
	}
	return r, nil
}

// QueryCompletionRequest carries the pinned sources of the denominators and the scope results.
// RegistrySHA256, AssetPins and ResultPins are selected by the caller before any result is read. A scope
// that is not in Scopes is absent, never zero.
type QueryCompletionRequest struct {
	Registry       []byte
	RegistrySHA256 string
	Qualification  []byte
	Slots          []byte
	Basis          map[string][]byte
	SvcExpected    []byte
	SvcFixture     []byte
	AssetPins      map[string][]queryregistrycore.Pin
	ResultPins     map[string]string // scope -> expected result_sha256; a present scope without a pin is rejected
	Scopes         []QueryScopeResult
}

// QueryCompletionCounts are the arithmetic of one level: required = verified + failed + blocked + unprocessed.
type QueryCompletionCounts struct {
	Required    uint64 `json:"required"`
	Verified    uint64 `json:"verified"`
	Failed      uint64 `json:"failed"`
	Blocked     uint64 `json:"blocked"`
	Unprocessed uint64 `json:"unprocessed"`
}

func (c *QueryCompletionCounts) add(o QueryCompletionCounts) {
	c.Required, c.Verified, c.Failed, c.Blocked, c.Unprocessed = c.Required+o.Required, c.Verified+o.Verified, c.Failed+o.Failed, c.Blocked+o.Blocked, c.Unprocessed+o.Unprocessed
}

func (c QueryCompletionCounts) sums() bool {
	return c.Verified+c.Failed+c.Blocked+c.Unprocessed == c.Required
}

type QueryCompletionScope struct {
	Scope             string `json:"scope"`
	Present           bool   `json:"present"`
	DenominatorSHA256 string `json:"denominator_sha256"`
	ResultSHA256      string `json:"result_sha256"`
	FirstFailure      string `json:"first_failure"`
}

type QueryCompletionCounter struct {
	Scope    string `json:"scope"`
	Kind     string `json:"kind"`
	Platform string `json:"platform"`
	QueryCompletionCounts
}

type QueryCompletionHost struct {
	Platform string `json:"platform"`
	QueryCompletionCounts
}

// QueryCompletionResult is the fan-in verdict. Complete is true only when every registered
// obligation of every scope is VERIFIED and no scope remains unregistered. Under the r1 plan the
// large denominator is unregistered, so no r1 result is COMPLETE.
type QueryCompletionResult struct {
	Schema             string                   `json:"schema"`
	RegistrySHA256     string                   `json:"registry_sha256"`
	Scopes             []QueryCompletionScope   `json:"scopes"`
	Counters           []QueryCompletionCounter `json:"counters"`
	Hosts              []QueryCompletionHost    `json:"hosts"`
	Total              QueryCompletionCounts    `json:"total"`
	UnregisteredScopes []string                 `json:"unregistered_scopes"`
	Completion         string                   `json:"completion"`
	Code               string                   `json:"code"`
	Complete           bool                     `json:"complete"`
}

type faninPlan struct {
	registrySHA  string
	rows         map[string][]QueryScopeCounter // required counts only, canonical order
	unregistered []string
}

func faninRows(scope string, required func(kind, platform string) uint64) []QueryScopeCounter {
	var rows []QueryScopeCounter
	for _, kind := range queryKindOrder[scope] {
		for _, platform := range queryPlatformOrder {
			if n := required(kind, platform); n > 0 {
				rows = append(rows, QueryScopeCounter{Kind: kind, Platform: platform, Required: n})
			}
		}
	}
	return rows
}

// buildFaninPlan derives every denominator from the validated registry and the SVC asset, never
// from a scope result.
func buildFaninPlan(v *queryregistrycore.Validated, svc SvcExpected) faninPlan {
	type key struct{ kind, platform string }
	registry := map[key]uint64{}
	for _, c := range v.Coordinates() {
		registry[key{c.Kind, c.Platform}]++
	}
	svcRequired := map[string]uint64{}
	for _, o := range svc.Obligations() {
		svcRequired[o.Kind]++
	}
	plan := faninPlan{registrySHA: v.SHA256(), rows: map[string][]QueryScopeCounter{}, unregistered: slices.Clone(queryCompletionUnregistered)}
	for _, scope := range queryScopeOrder {
		plan.rows[scope] = faninRows(scope, func(kind, platform string) uint64 {
			if scope == QueryScopeSvc {
				return svcRequired[kind]
			}
			return registry[key{kind, platform}]
		})
	}
	return plan
}

func (p faninPlan) denominator(scope string) string {
	return scopeDenominatorDigest(scope, p.rows[scope])
}

// checkScope returns the counters of a present scope after every presence and integrity check.
func (p faninPlan) checkScope(r QueryScopeResult, pins []queryregistrycore.Pin, resultPin string) error {
	rows := p.rows[r.Scope]
	switch {
	case r.Schema != QueryScopeResultSchema:
		return queryCompletionFault("QUERY_SCOPE_SCHEMA")
	case r.RegistrySHA256 != p.registrySHA:
		return queryCompletionFault("QUERY_SCOPE_REGISTRY_PIN")
	case r.DenominatorSHA256 != p.denominator(r.Scope):
		return queryCompletionFault("QUERY_SCOPE_DENOMINATOR_PIN")
	case r.Counters == nil || r.Assets == nil || len(r.Counters) != len(rows):
		return queryCompletionFault("QUERY_SCOPE_DENOMINATOR")
	}
	var failed, verified uint64
	for i, c := range r.Counters {
		if c.Kind != rows[i].Kind || c.Platform != rows[i].Platform || c.Required != rows[i].Required {
			return queryCompletionFault("QUERY_SCOPE_DENOMINATOR")
		}
		if !(QueryCompletionCounts{c.Required, c.Verified, c.Failed, c.Blocked, c.Unprocessed}).sums() {
			return queryCompletionFault("QUERY_SCOPE_SUM")
		}
		failed, verified = failed+c.Failed, verified+c.Verified
	}
	if (failed > 0) != (r.FirstFailure != "") {
		return queryCompletionFault("QUERY_SCOPE_FAILURE_CODE")
	}
	if r.ResultSHA256 != r.digest() {
		return queryCompletionFault("QUERY_SCOPE_RESULT_DIGEST")
	}
	if resultPin == "" {
		return queryCompletionFault("QUERY_SCOPE_RESULT_UNPINNED")
	}
	if r.ResultSHA256 != resultPin {
		return queryCompletionFault("QUERY_SCOPE_RESULT_PIN")
	}
	if verified > 0 && len(r.Assets) == 0 {
		return queryCompletionFault("QUERY_SCOPE_ASSET_PIN")
	}
	for _, a := range r.Assets {
		if !slices.Contains(pins, a) || !queryHex64.MatchString(a.SHA256) {
			return queryCompletionFault("QUERY_SCOPE_ASSET_PIN")
		}
	}
	return nil
}

// fold is the pure fan-in over an already built plan; production entry points build the plan from
// the pinned sources. Counters of an absent scope are BLOCKED, never zero.
func (p faninPlan) fold(scopes []QueryScopeResult, pins map[string][]queryregistrycore.Pin, resultPins map[string]string) (QueryCompletionResult, error) {
	out := QueryCompletionResult{Schema: QueryCompletionSchema, RegistrySHA256: p.registrySHA, Scopes: []QueryCompletionScope{}, Counters: []QueryCompletionCounter{},
		Hosts: []QueryCompletionHost{}, UnregisteredScopes: p.unregistered}
	byScope := map[string]QueryScopeResult{}
	var fault error
	for _, r := range scopes {
		if _, known := p.rows[r.Scope]; !known {
			fault = cmpFirst(fault, queryCompletionFault("QUERY_SCOPE_UNKNOWN"))
		} else if _, dup := byScope[r.Scope]; dup {
			fault = cmpFirst(fault, queryCompletionFault("QUERY_SCOPE_DUPLICATE"))
		}
		byScope[r.Scope] = r
	}
	hosts := map[string]*QueryCompletionHost{}
	for _, scope := range queryScopeOrder {
		r, present := byScope[scope]
		if present {
			if err := p.checkScope(r, pins[scope], resultPins[scope]); err != nil {
				fault = cmpFirst(fault, err)
				present = false
			}
		}
		entry := QueryCompletionScope{Scope: scope, Present: present, DenominatorSHA256: p.denominator(scope)}
		counters := p.rows[scope]
		if present {
			entry.ResultSHA256, entry.FirstFailure, counters = r.ResultSHA256, r.FirstFailure, r.Counters
		} else {
			counters = slices.Clone(counters)
			for i := range counters {
				counters[i].Blocked = counters[i].Required
			}
		}
		out.Scopes = append(out.Scopes, entry)
		for _, c := range counters {
			counts := QueryCompletionCounts{c.Required, c.Verified, c.Failed, c.Blocked, c.Unprocessed}
			out.Counters = append(out.Counters, QueryCompletionCounter{Scope: scope, Kind: c.Kind, Platform: c.Platform, QueryCompletionCounts: counts})
			if hosts[c.Platform] == nil {
				hosts[c.Platform] = &QueryCompletionHost{Platform: c.Platform}
			}
			hosts[c.Platform].add(counts)
			out.Total.add(counts)
		}
	}
	for _, platform := range queryPlatformOrder {
		if h := hosts[platform]; h != nil {
			out.Hosts = append(out.Hosts, *h)
		}
	}
	out.Completion, out.Code = completionVerdict(out)
	out.Complete = out.Completion == QueryCompletionComplete
	if fault != nil {
		// A defect is a FAIL that keeps the first failure already confirmed by a well-formed scope.
		code := ""
		for _, s := range out.Scopes {
			if code == "" && s.Present {
				code = s.FirstFailure
			}
		}
		if f, ok := fault.(*Error); ok && code == "" {
			code = f.Code
		}
		return QueryCompletionResult{Schema: QueryCompletionSchema, RegistrySHA256: p.registrySHA, Scopes: []QueryCompletionScope{}, Counters: []QueryCompletionCounter{},
			Hosts: []QueryCompletionHost{}, UnregisteredScopes: p.unregistered, Completion: QueryCompletionFail, Code: code}, fault
	}
	return out, nil
}

// cmpFirst keeps the first defect in consumption order.
func cmpFirst(first, next error) error {
	if first != nil {
		return first
	}
	return next
}

// completionVerdict derives the verdict from counts alone: FAIL first with the first failure code
// in scope order, then BLOCKED, and COMPLETE only for an all-VERIFIED, fully registered state.
func completionVerdict(r QueryCompletionResult) (string, string) {
	if r.Total.Failed > 0 {
		for _, s := range r.Scopes {
			if s.FirstFailure != "" {
				return QueryCompletionFail, s.FirstFailure
			}
		}
		return QueryCompletionFail, "QUERY_COMPLETION_FAILURE_CODE"
	}
	for _, s := range r.Scopes {
		if !s.Present {
			return QueryCompletionBlocked, "QUERY_SCOPE_RESULT_ABSENT"
		}
	}
	if r.Total.Verified != r.Total.Required {
		return QueryCompletionBlocked, "QUERY_COMPLETION_INCOMPLETE"
	}
	if len(r.UnregisteredScopes) > 0 {
		return QueryCompletionBlocked, "QUERY_COMPLETION_UNREGISTERED_SCOPE"
	}
	return QueryCompletionComplete, ""
}

// FoldQueryCompletion binds the scope results to the pinned registry and folds them. It never
// reports a PASS or opens a release gate by itself: the verdict is COMPLETE only for an
// all-VERIFIED registered denominator, and the r1 plan has an unregistered large scope.
func FoldQueryCompletion(req QueryCompletionRequest) (QueryCompletionResult, error) {
	v, _, err := ordinaryRegistry(OrdinaryPlanRequest{Registry: req.Registry, RegistrySHA256: req.RegistrySHA256, Qualification: req.Qualification, Slots: req.Slots, Basis: req.Basis})
	if err != nil {
		return QueryCompletionResult{Schema: QueryCompletionSchema, Completion: QueryCompletionFail, Code: errorCode(err)}, err
	}
	svc, err := ParseSvcExpected(req.SvcExpected, req.Registry, req.Qualification, req.SvcFixture)
	if err != nil {
		return QueryCompletionResult{Schema: QueryCompletionSchema, RegistrySHA256: v.SHA256(), Completion: QueryCompletionFail, Code: errorCode(err)}, err
	}
	return buildFaninPlan(v, svc).fold(req.Scopes, req.AssetPins, req.ResultPins)
}

func errorCode(err error) string {
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	if f, ok := err.(*queryregistrycore.Fault); ok {
		return f.Code
	}
	return "QUERY_COMPLETION_SOURCE"
}

// DecodeQueryCompletionResult strictly decodes a stored completion result and re-derives every
// level of arithmetic and the verdict from its counters. A stored result cannot claim a state its
// own counters do not support; it is not a recomputation from the scope evidence.
func DecodeQueryCompletionResult(data []byte) (QueryCompletionResult, error) {
	var r QueryCompletionResult
	if len(data) > queryScopeResultMaxBytes {
		return r, fail(KindResourceLimit, "QUERY_COMPLETION_SIZE", "query-completion-result", nil)
	}
	if err := strictRoundTrip(data, &r); err != nil {
		return QueryCompletionResult{}, fail(KindInvalidInput, "QUERY_COMPLETION_JSON", "query-completion-result", err)
	}
	bad := func(code string) (QueryCompletionResult, error) {
		return QueryCompletionResult{}, queryCompletionFault(code)
	}
	if r.Schema != QueryCompletionSchema || !queryHex64.MatchString(r.RegistrySHA256) {
		return bad("QUERY_COMPLETION_SCHEMA")
	}
	if len(r.Scopes) != len(queryScopeOrder) || !slices.Equal(r.UnregisteredScopes, queryCompletionUnregistered) {
		return bad("QUERY_COMPLETION_SCOPES")
	}
	for i, s := range r.Scopes {
		if s.Scope != queryScopeOrder[i] || !queryHex64.MatchString(s.DenominatorSHA256) || s.Present && !queryHex64.MatchString(s.ResultSHA256) || !s.Present && (s.ResultSHA256 != "" || s.FirstFailure != "") {
			return bad("QUERY_COMPLETION_SCOPES")
		}
	}
	hosts := map[string]*QueryCompletionCounts{}
	var total QueryCompletionCounts
	scopeFailed := map[string]uint64{}
	for _, c := range r.Counters {
		if !slices.Contains(queryKindOrder[c.Scope], c.Kind) || !slices.Contains(queryPlatformOrder, c.Platform) {
			return bad("QUERY_COMPLETION_COUNTER")
		}
		if !c.sums() {
			return bad("QUERY_COMPLETION_SUM")
		}
		if hosts[c.Platform] == nil {
			hosts[c.Platform] = &QueryCompletionCounts{}
		}
		hosts[c.Platform].add(c.QueryCompletionCounts)
		total.add(c.QueryCompletionCounts)
		scopeFailed[c.Scope] += c.Failed
		if s := r.Scopes[slices.Index(queryScopeOrder, c.Scope)]; !s.Present && c.Verified+c.Failed+c.Unprocessed != 0 {
			return bad("QUERY_COMPLETION_ABSENT_SCOPE")
		}
	}
	var wantHosts []QueryCompletionHost
	for _, platform := range queryPlatformOrder {
		if h := hosts[platform]; h != nil {
			wantHosts = append(wantHosts, QueryCompletionHost{Platform: platform, QueryCompletionCounts: *h})
		}
	}
	if !reflect.DeepEqual(r.Hosts, wantHosts) || r.Total != total {
		return bad("QUERY_COMPLETION_SUM")
	}
	for _, s := range r.Scopes {
		if (scopeFailed[s.Scope] > 0) != (s.FirstFailure != "") {
			return bad("QUERY_COMPLETION_FAILURE_CODE")
		}
	}
	completion, code := completionVerdict(r)
	if r.Completion != completion || r.Code != code || r.Complete != (completion == QueryCompletionComplete) {
		return bad("QUERY_COMPLETION_VERDICT")
	}
	return r, nil
}
