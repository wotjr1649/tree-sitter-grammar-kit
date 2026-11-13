package native

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// storeQueryScopeHost writes this host's scope result to the directory named by
// TSGK_QUERY_SCOPE_OUT under the given file name and reads it back; without the variable nothing is
// written. A stored file is never overwritten: evidence of an earlier run stays.
func storeQueryScopeHost(t *testing.T, host, file string, r kit.QueryScopeResult) {
	t.Helper()
	dir := os.Getenv(kit.QueryScopeHostEnv)
	if dir == "" {
		return
	}
	h, err := kit.NewQueryScopeHostResult(host, r)
	if err != nil {
		t.Fatal(err)
	}
	data, err := kit.EncodeQueryScopeHostResult(h)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, file), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, werr := f.Write(data)
	if err := f.Close(); werr != nil || err != nil {
		t.Fatalf("write: %v %v", werr, err)
	}
	back, err := kit.DecodeQueryScopeHostResult(data)
	if err != nil || !reflect.DeepEqual(back, h) {
		t.Fatalf("stored %s/%s does not read back: %v", r.Scope, host, err)
	}
	t.Logf("STORED %s %s result=%s", file, host, r.ResultSHA256)
}
