package kit

import (
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The ordinary scope of a host is judged by the native ordinary regressions of the selected shards (#427 P1):
// the opt-in tests leave the stored results of their public passes in QueryScopeOrdinaryResultsEnv, and
// TestOrdinaryScopeStore folds them with OrdinaryScopeResult into this host's tsgk-query-scope-host/r1 file
// (QueryScopeHostFile(QueryScopeOrdinary, host)). A shard no result selected stays BLOCKED, so the file claims
// only what ran on this host.

// QueryScopeOrdinaryResultsEnv names the directory the ordinary regressions store their public pass results in.
const QueryScopeOrdinaryResultsEnv = "TSGK_ORDINARY_SCOPE_RESULTS"

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
