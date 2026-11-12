package kit

import (
	jsonv2 "encoding/json/v2"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// A host scope result stores the scope-local result one host produced (#362 L1). The wrapped
// QueryScopeResult is the existing tsgk-query-scope-result/r1 and keeps its meaning; the wrapper
// only binds it to the host that observed it, so a file of one host cannot be read as the other
// host's evidence. MergeQueryScopeHostResults turns the stored files of both hosts into the one
// QueryScopeResult per scope that FoldQueryCompletion takes. Like the scope result, the file is a
// stored claim of its producer: presence, integrity and the host binding are checked, not
// authenticity.

const (
	QueryScopeHostSchema = "tsgk-query-scope-host/r1"
	// QueryScopeHostEnv names the opt-in output directory of the tests that store their judgments.
	QueryScopeHostEnv = "TSGK_QUERY_SCOPE_OUT"
)

// QueryScopeHostResult is one host's stored scope result. HostSHA256 covers the schema, the host
// and the wrapped result's own digest.
type QueryScopeHostResult struct {
	Schema     string           `json:"schema"`
	Host       string           `json:"host"`
	Result     QueryScopeResult `json:"result"`
	HostSHA256 string           `json:"host_sha256"`
}

func (h QueryScopeHostResult) digest() string {
	return queryregistrycore.Digest(queryregistrycore.Canonical([]any{QueryScopeHostSchema, h.Host, h.Result.ResultSHA256}))
}

// QueryScopeHostFile is the file name a host's result of a scope is stored under.
func QueryScopeHostFile(scope, host string) string {
	return "query-scope-" + scope + "-" + host + ".json"
}

// check is every integrity rule of a stored host result. The rows of the other hosts are never
// this host's claim: a VERIFIED or FAILED count there means judgments of another host were mixed in.
func (h QueryScopeHostResult) check() error {
	switch {
	case h.Schema != QueryScopeHostSchema:
		return queryCompletionFault("QUERY_SCOPE_HOST_SCHEMA")
	case !slices.Contains(queryPlatformOrder, h.Host):
		return queryCompletionFault("QUERY_SCOPE_HOST_UNKNOWN")
	case h.Result.Schema != QueryScopeResultSchema:
		return queryCompletionFault("QUERY_SCOPE_SCHEMA")
	case h.Result.Counters == nil || h.Result.Assets == nil:
		return queryCompletionFault("QUERY_SCOPE_DENOMINATOR")
	case h.Result.ResultSHA256 != h.Result.digest():
		return queryCompletionFault("QUERY_SCOPE_RESULT_DIGEST")
	}
	var failed uint64
	for _, c := range h.Result.Counters {
		if !(QueryCompletionCounts{c.Required, c.Verified, c.Failed, c.Blocked, c.Unprocessed}).sums() {
			return queryCompletionFault("QUERY_SCOPE_SUM")
		}
		if c.Platform != h.Host {
			if c.Verified != 0 || c.Failed != 0 {
				return queryCompletionFault("QUERY_SCOPE_HOST_FOREIGN")
			}
			continue
		}
		failed += c.Failed
	}
	if (failed > 0) != (h.Result.FirstFailure != "") {
		return queryCompletionFault("QUERY_SCOPE_FAILURE_CODE")
	}
	if h.HostSHA256 != h.digest() {
		return queryCompletionFault("QUERY_SCOPE_HOST_DIGEST")
	}
	return nil
}

// NewQueryScopeHostResult binds a scope result to the host that produced it and seals it. The
// result must claim nothing for another host: its rows of the other hosts stay BLOCKED or UNPROCESSED.
func NewQueryScopeHostResult(host string, r QueryScopeResult) (QueryScopeHostResult, error) {
	h := QueryScopeHostResult{Schema: QueryScopeHostSchema, Host: host, Result: r}
	h.HostSHA256 = h.digest()
	if err := h.check(); err != nil {
		return QueryScopeHostResult{}, err
	}
	return h, nil
}

// EncodeQueryScopeHostResult is the stored form; the result is checked first.
func EncodeQueryScopeHostResult(h QueryScopeHostResult) ([]byte, error) {
	if err := h.check(); err != nil {
		return nil, err
	}
	return jsonv2.Marshal(h)
}

// DecodeQueryScopeHostResult strictly decodes a stored host result and re-checks every integrity
// rule, so an edited host, count, digest or member is rejected.
func DecodeQueryScopeHostResult(data []byte) (QueryScopeHostResult, error) {
	var h QueryScopeHostResult
	if len(data) > queryScopeResultMaxBytes+1024 {
		return h, fail(KindResourceLimit, "QUERY_SCOPE_SIZE", "query-scope-host", nil)
	}
	if err := strictRoundTrip(data, &h); err != nil {
		return QueryScopeHostResult{}, fail(KindInvalidInput, "QUERY_SCOPE_JSON", "query-scope-host", err)
	}
	if err := h.check(); err != nil {
		return QueryScopeHostResult{}, err
	}
	return h, nil
}

// MergeQueryScopeHostResults combines the stored results of one scope into the result the fan-in
// takes: each row comes from the result of the host the row counts, and nothing else is read from
// it. Every result must describe the same scope, registry, denominator, rows and evidence pins; every
// host of the rows needs exactly one result (a missing host is an error, fold the present host's own
// result instead to leave the other host BLOCKED), and a result of a host without rows is refused.
func MergeQueryScopeHostResults(hosts []QueryScopeHostResult) (QueryScopeResult, error) {
	if len(hosts) == 0 {
		return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_HOST_ABSENT")
	}
	byHost := map[string]QueryScopeHostResult{}
	for _, h := range hosts {
		if err := h.check(); err != nil {
			return QueryScopeResult{}, err
		}
		if _, dup := byHost[h.Host]; dup {
			return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_HOST_DUPLICATE")
		}
		byHost[h.Host] = h
	}
	base := hosts[0].Result
	for _, h := range hosts[1:] {
		r := h.Result
		if r.Scope != base.Scope || r.RegistrySHA256 != base.RegistrySHA256 || r.DenominatorSHA256 != base.DenominatorSHA256 || !slices.Equal(r.Assets, base.Assets) || len(r.Counters) != len(base.Counters) {
			return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_HOST_MISMATCH")
		}
		for i, c := range r.Counters {
			if b := base.Counters[i]; c.Kind != b.Kind || c.Platform != b.Platform || c.Required != b.Required {
				return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_HOST_MISMATCH")
			}
		}
	}
	rows := make([]QueryScopeCounter, 0, len(base.Counters))
	used := map[string]bool{}
	for i, c := range base.Counters {
		h, ok := byHost[c.Platform]
		if !ok {
			return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_HOST_ABSENT")
		}
		used[c.Platform] = true
		rows = append(rows, h.Result.Counters[i])
	}
	if len(used) != len(byHost) {
		return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_HOST_UNUSED")
	}
	first := ""
	for _, platform := range queryPlatformOrder {
		if h, ok := byHost[platform]; ok && first == "" {
			first = h.Result.FirstFailure
		}
	}
	return NewQueryScopeResult(base.Scope, base.RegistrySHA256, base.Assets, rows, first), nil
}
