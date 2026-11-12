package kit

import (
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// Adapters from the existing scope-local folds to QueryScopeResult. A scope without a stored
// result is BLOCKED as absent.

func scopeCounters[T any](rows []T, to func(T) QueryScopeCounter) []QueryScopeCounter {
	out := make([]QueryScopeCounter, 0, len(rows))
	for _, r := range rows {
		out = append(out, to(r))
	}
	return out
}

// OrdinaryScopeResult folds decoded ordinary shard results into the ordinary scope result. A
// stored ordinary result carries no registry pin, so it is bound by membership: every selected ID
// must be a STRUCTURAL_ONLY ID of this registry's namespace, in exactly one shard. IDs no shard
// selected are absent, hence BLOCKED; selected IDs without a judgment are UNPROCESSED. A VERIFIED
// tuple needs the shard's external binding, and a shard FAIL that no tuple carries is kept as
// FAILED on every tuple of that shard that is not VERIFIED, never lowered to BLOCKED. When every
// tuple of the shard is VERIFIED the first tuple carries the FAIL instead (fewer VERIFIED, never
// more), because the counters are the only place the fan-in reads a FAIL from; a FAIL shard that
// selects no tuple has nowhere to put it and is rejected, not dropped.
func OrdinaryScopeResult(v *queryregistrycore.Validated, assets []queryregistrycore.Pin, results []OrdinaryQuerySemanticsResult) (QueryScopeResult, error) {
	platformOf := map[string]string{}
	counters := map[string]*QueryScopeCounter{}
	for _, c := range v.Coordinates() {
		if c.Kind != queryregistrycore.Structural {
			continue
		}
		platformOf[c.ID(v.Namespace())] = c.Platform
		if counters[c.Platform] == nil {
			counters[c.Platform] = &QueryScopeCounter{Kind: queryregistrycore.Structural, Platform: c.Platform}
		}
		counters[c.Platform].Required++
		counters[c.Platform].Blocked++
	}
	seen := map[string]bool{}
	first := ""
	for _, res := range results {
		if !validOrdinaryStoredResult(res) {
			return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_ORDINARY_RESULT")
		}
		if res.Assessment == AssessFail && len(res.SelectedObligationIDs) == 0 {
			return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_ORDINARY_FAIL_UNCARRIED")
		}
		shardFailed := res.Assessment == AssessFail && res.Tuple.Failed == 0
		allVerified := len(res.Obligations) == len(res.SelectedObligationIDs) &&
			!slices.ContainsFunc(res.Obligations, func(o OrdinaryQuerySemanticsTuple) bool { return o.State != ordinaryTupleVerified })
		for i, id := range res.SelectedObligationIDs {
			platform, ok := platformOf[id]
			if !ok {
				return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_UNDECLARED")
			}
			if seen[id] {
				return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_OVERLAP")
			}
			seen[id] = true
			state, code := "UNPROCESSED", ""
			if i < len(res.Obligations) {
				state, code = res.Obligations[i].State, res.Obligations[i].Code
			}
			if state == ordinaryTupleVerified && !res.BindingValid {
				return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_ORDINARY_UNBOUND")
			}
			if shardFailed && (state != ordinaryTupleVerified || allVerified && i == 0) {
				state, code = ordinaryTupleFailed, res.Code
			}
			c := counters[platform]
			c.Blocked--
			switch state {
			case ordinaryTupleVerified:
				c.Verified++
			case ordinaryTupleFailed:
				c.Failed++
				if first == "" {
					first = code
				}
			case ordinaryTupleBlocked:
				c.Blocked++
			default:
				c.Unprocessed++
			}
		}
	}
	var rows []QueryScopeCounter
	for _, c := range counters {
		rows = append(rows, *c)
	}
	return NewQueryScopeResult(QueryScopeOrdinary, v.SHA256(), assets, rows, first), nil
}

// PredicateScopeResult folds the predicate scope with FoldPredicateScope and pins the expected
// asset its VERIFIED counts rest on. The first failure is the first FAILED judgment as given.
func PredicateScopeResult(registry, asset []byte, judgments []queryregistrycore.Judgment, blocked map[string]bool) (QueryScopeResult, error) {
	sha := queryregistrycore.Digest(registry)
	if _, err := ParsePredicateExpected(asset, registry); err != nil {
		return QueryScopeResult{}, err
	}
	cov, err := FoldPredicateScope(registry, sha, judgments, blocked)
	if err != nil {
		return QueryScopeResult{}, err
	}
	first := ""
	for _, j := range judgments {
		if j.State == "FAILED" && first == "" {
			first = j.Code
		}
	}
	rows := scopeCounters(cov.Counters, func(c PredicateScopeCounter) QueryScopeCounter {
		return QueryScopeCounter{c.Kind, c.Platform, c.Required, c.Verified, c.Failed, c.Blocked, c.Unprocessed}
	})
	return NewQueryScopeResult(QueryScopePredicate, sha, []queryregistrycore.Pin{queryregistrycore.FilePin(PredicateExpectedPath, asset)}, rows, first), nil
}

// SvcScopeResult folds one host's SVC judgments with FoldSvcScope (both hosts' obligations are
// counted) and pins the expected asset. expectedBytes must be the bytes e was parsed from.
func SvcScopeResult(e SvcExpected, expectedBytes, registry []byte, platform string, judgments []SvcJudgment, blocked map[string]bool) (QueryScopeResult, error) {
	cov, err := FoldSvcScope(e, platform, judgments, blocked)
	if err != nil {
		return QueryScopeResult{}, err
	}
	first := ""
	for _, j := range judgments {
		if j.State == "FAILED" && first == "" {
			first = j.Code
		}
	}
	rows := scopeCounters(cov.Counters, func(c SvcScopeCounter) QueryScopeCounter {
		return QueryScopeCounter{c.Kind, c.Platform, c.Required, c.Verified, c.Failed, c.Blocked, c.Unprocessed}
	})
	return NewQueryScopeResult(QueryScopeSvc, queryregistrycore.Digest(registry), []queryregistrycore.Pin{queryregistrycore.FilePin(SvcExpectedPath, expectedBytes)}, rows, first), nil
}

// LargeScopeResult folds the Windows large judgments with FoldLargeScope over the lower
// obligations of the large contract and pins the expected asset. The contract must be the one the
// registry pins; expectedBytes must be the bytes e was parsed from. The Linux/macOS N/A hosts are
// not counted; the fan-in carries their registered reasons from the contract.
func LargeScopeResult(e LargeExpected, expectedBytes, registry []byte, judgments []LargeJudgment) (QueryScopeResult, error) {
	if e.Registry != queryregistrycore.FilePin(queryregistrycore.Path, registry) {
		return QueryScopeResult{}, queryCompletionFault("QUERY_SCOPE_REGISTRY_PIN")
	}
	cov, err := FoldLargeScope(e, judgments)
	if err != nil {
		return QueryScopeResult{}, err
	}
	first := ""
	for _, j := range judgments {
		if j.State == "FAILED" && first == "" {
			first = j.Code
		}
	}
	rows := scopeCounters(cov.Counters, func(c LargeScopeCounter) QueryScopeCounter {
		return QueryScopeCounter{c.Kind, c.Platform, c.Required, c.Verified, c.Failed, c.Blocked, c.Unprocessed}
	})
	return NewQueryScopeResult(QueryScopeLarge, queryregistrycore.Digest(registry), []queryregistrycore.Pin{queryregistrycore.FilePin(LargeExpectedPath, expectedBytes)}, rows, first), nil
}
