package kit

import (
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The ordinary scope of a host is judged by the native ordinary regressions of the selected shards (#427 P1):
// the opt-in tests leave the stored results of their public passes in QueryScopeOrdinaryResultsEnv, and
// TestOrdinaryScopeStore folds them with OrdinaryScopeResult into this host's tsgk-query-scope-host/r1 file
// (QueryScopeHostFile(QueryScopeOrdinary, host)). A shard no result selected stays BLOCKED, so the file claims
// only what ran on this host.

// QueryScopeOrdinaryResultsEnv names the directory the ordinary regressions store their public pass results in.
const QueryScopeOrdinaryResultsEnv = "TSGK_ORDINARY_SCOPE_RESULTS"

// OrdinaryScopeBasis says what a VERIFIED ordinary row of this collection rests on (decision B5 of #427). The stored
// r1 file has no member to carry it, so the merge report and the collection log do.
const OrdinaryScopeBasis = "synthetic reference rows and approval (#427 decision B5): VERIFIED means the owned native tree equals the independently authored expected, not that a reference ran"

// ordinaryScopeClaimCases are the shards (route, case) the P1 collection selects on each host: the JSON zero-edit and
// edit-history runs, the PostgreSQL and the TSQL public pass. Only the tuples of these shards may be claimed.
var ordinaryScopeClaimCases = map[string][]string{
	"json":           {"alt-json-B01-05", "alt-json-B01-09", "json-feature-r1", "json-query-r1"},
	"postgresql-sql": {"alt-postgresql-sql-B04-22"},
	"tsql":           {"alt-tsql-B01-r001"},
}

// ordinaryScopeClaimIDs derives from the registry the tuple IDs of the selected shards per platform: the structural
// primary coordinates whose route and case the collection selects, the same ones the shard plan lists per shard.
func ordinaryScopeClaimIDs(v *queryregistrycore.Validated) map[string][]string {
	out := map[string][]string{}
	for _, c := range v.Coordinates() {
		if c.Kind == queryregistrycore.Structural && c.Role == "primary" && slices.Contains(ordinaryScopeClaimCases[c.Route], c.Case) {
			out[c.Platform] = append(out[c.Platform], c.ID(v.Namespace()))
		}
	}
	return out
}

// OrdinaryScopeClaims is how many ordinary tuples per platform the P1 collection may judge: the ceiling the merge
// holds a stored ordinary file to (more than this is an overclaim) and the fill it expects (fewer is under-filled).
func OrdinaryScopeClaims(req QueryCompletionRequest) (map[string]uint64, error) {
	v, _, err := ordinaryRegistry(OrdinaryPlanRequest{Registry: req.Registry, RegistrySHA256: req.RegistrySHA256, Qualification: req.Qualification, Slots: req.Slots, Basis: req.Basis})
	if err != nil {
		return nil, err
	}
	claims := map[string]uint64{}
	for platform, ids := range ordinaryScopeClaimIDs(v) {
		claims[platform] = uint64(len(ids))
	}
	return claims, nil
}

// OrdinaryScopeOverclaim reports whether a stored ordinary scope result judges (VERIFIED, FAILED or UNPROCESSED)
// more tuples on a platform than claims allows. The result carries counts only; the identity of each judged tuple is
// bound where the result is stored (the store refuses a tuple outside the selected shards).
func OrdinaryScopeOverclaim(r QueryScopeResult, claims map[string]uint64) bool {
	for _, c := range r.Counters {
		if c.Verified+c.Failed+c.Unprocessed > claims[c.Platform] {
			return true
		}
	}
	return false
}

// OrdinaryScopeUnjudged is, per platform, how many claimable tuples a stored ordinary scope result left unjudged
// (neither VERIFIED nor FAILED); platforms that are filled are absent.
func OrdinaryScopeUnjudged(r QueryScopeResult, claims map[string]uint64) map[string]uint64 {
	judged := map[string]uint64{}
	for _, c := range r.Counters {
		judged[c.Platform] += c.Verified + c.Failed
	}
	out := map[string]uint64{}
	for platform, n := range claims {
		if judged[platform] < n {
			out[platform] = n - judged[platform]
		}
	}
	return out
}

// OrdinaryScopeAssetPaths are the committed sources of the independent expectations the selected shards'
// VERIFIED tuples rest on: the authoring code of the JSON, TSQL and PostgreSQL expectations and the anchors it
// reads. The expectations are authored in these files (they are not stored as separate assets), so the scope
// result pins them by content and the merge selects the same pins from its own checkout before it reads a result.
var OrdinaryScopeAssetPaths = []string{
	"src/kit/query_ordinary_json_expected_test.go",
	"src/kit/query_ordinary_tsql_expected_test.go",
	"src/kit/query_ordinary_sql_capacity_test.go",
	"src/testdata/query-semantics/json1-classification-anchors.json",
	"src/testdata/query-semantics/postgresql9-classification-anchors.json",
}

// OrdinaryScopeAssetPins pins OrdinaryScopeAssetPaths with the bytes read gives. A missing file is an error: a
// pin of nothing would make the result claim an asset that is not there.
func OrdinaryScopeAssetPins(read func(path string) ([]byte, error)) ([]queryregistrycore.Pin, error) {
	pins := make([]queryregistrycore.Pin, 0, len(OrdinaryScopeAssetPaths))
	for _, path := range OrdinaryScopeAssetPaths {
		data, err := read(path)
		if err != nil || len(data) == 0 {
			return nil, queryCompletionFault("QUERY_SCOPE_ASSET_PIN")
		}
		pins = append(pins, queryregistrycore.FilePin(path, data))
	}
	return pins, nil
}
