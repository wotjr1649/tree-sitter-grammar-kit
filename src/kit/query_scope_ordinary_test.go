package kit

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// ordinaryScopeResultNames are the results the native ordinary regressions store, one file each. The JSON
// single-case regression does not store its result: its tuple is the first one of the two-case run.
var ordinaryScopeResultNames = []string{"json-zero-edit", "json-history", "sql-postgresql-sql", "sql-tsql"}

// ordinaryScopeRecord stores the public pass result of a selected-shard regression for TestOrdinaryScopeStore when
// QueryScopeOrdinaryResultsEnv names a directory, before the test judges the result: a FAILED tuple is a result, not
// a collection failure. A stored file is never overwritten.
func ordinaryScopeRecord(t *testing.T, name string, result OrdinaryQuerySemanticsResult) {
	t.Helper()
	dir := os.Getenv(QueryScopeOrdinaryResultsEnv)
	if dir == "" {
		return
	}
	if !slices.Contains(ordinaryScopeResultNames, name) {
		t.Fatalf("ordinary scope result %q is not a collected one", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, name+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, werr := f.Write(ordinaryJSON(t, result))
	if err := f.Close(); werr != nil || err != nil {
		t.Fatalf("write: %v %v", werr, err)
	}
}

// ordinaryHostScope folds the stored results of a directory into the host's ordinary scope result. The directory
// is closed: only the named results, each decoded and recomputed by DecodeOrdinaryQuerySemanticsResult; a name
// missing is a shard no pass judged (it stays BLOCKED), a stranger is a defect.
func ordinaryHostScope(t *testing.T, v *queryregistrycore.Validated, assets []queryregistrycore.Pin, host, dir string) QueryScopeHostResult {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var results []OrdinaryQuerySemanticsResult
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !e.Type().IsRegular() || !slices.Contains(ordinaryScopeResultNames, name) {
			t.Fatalf("unexpected entry %q in the ordinary scope results", e.Name())
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		result, err := DecodeOrdinaryQuerySemanticsResult(data)
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		results = append(results, result)
	}
	if len(results) == 0 {
		t.Fatal("no ordinary result stored: nothing to claim")
	}
	scope, err := OrdinaryScopeResult(v, assets, results)
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewQueryScopeHostResult(host, scope)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestOrdinaryScopeStore is the last step of the gate-external ordinary collection (#427 P1): it stores this host's
// ordinary scope result next to the predicate and SVC ones. It runs only when the collection names both
// directories; the collection counts it only when it ran, not when it skipped.
func TestOrdinaryScopeStore(t *testing.T) {
	dir, out := os.Getenv(QueryScopeOrdinaryResultsEnv), os.Getenv(QueryScopeHostEnv)
	if dir == "" || out == "" {
		t.Skip("set " + QueryScopeOrdinaryResultsEnv + " and " + QueryScopeHostEnv + " to store the ordinary scope result")
	}
	host, _ := ordinaryRunHost()
	v, _ := ordinaryP0ForPlanTest(t)
	assets, err := OrdinaryScopeAssetPins(func(path string) ([]byte, error) {
		return os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(path)))
	})
	if err != nil {
		t.Fatal(err)
	}
	h := ordinaryHostScope(t, v, assets, host, dir)
	data, err := EncodeQueryScopeHostResult(h)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(out, QueryScopeHostFile(QueryScopeOrdinary, host)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, werr := f.Write(data)
	if err := f.Close(); werr != nil || err != nil {
		t.Fatalf("write: %v %v", werr, err)
	}
	back, err := DecodeQueryScopeHostResult(data)
	if err != nil || !reflect.DeepEqual(back, h) {
		t.Fatalf("stored ordinary/%s does not read back: %v", host, err)
	}
	t.Logf("STORED %s %s result=%s counters=%+v", QueryScopeHostFile(QueryScopeOrdinary, host), host, h.Result.ResultSHA256, h.Result.Counters)
}

// The store is a fold of what ran: a host's selected shards are VERIFIED on its own rows only, everything else is
// BLOCKED, a result of the other host's rows cannot be stored as this host's, and the directory is closed.
func TestOrdinaryHostScopeFoldsOnlyWhatRanOnThisHost(t *testing.T) {
	f := faninFixtureFor(t)
	assets := []queryregistrycore.Pin{faninOrdinaryAsset}
	write := func(dir, name string, ids []string, states ...string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name+".json"), ordinaryJSON(t, faninShard(ids, states, true)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, host := range []string{"windows-amd64", "linux-amd64"} {
		other := "linux-amd64"
		if host == other {
			other = "windows-amd64"
		}
		dir := t.TempDir()
		ids := f.structuralIDs(host, 4)
		write(dir, "json-zero-edit", ids[:2], "VERIFIED", "VERIFIED")
		write(dir, "json-history", ids[2:], "VERIFIED", "FAILED")
		h := ordinaryHostScope(t, f.v, assets, host, dir)
		for _, c := range h.Result.Counters {
			switch c.Platform {
			case host:
				if c.Verified != 3 || c.Failed != 1 || c.Unprocessed != 0 || c.Blocked != c.Required-4 {
					t.Errorf("%s own row: %+v", host, c)
				}
			default:
				if c.Verified != 0 || c.Failed != 0 || c.Blocked != c.Required {
					t.Errorf("%s foreign row: %+v", host, c)
				}
			}
		}
		// a result that judged the other host's coordinates is not this host's evidence
		foreign := t.TempDir()
		write(foreign, "sql-tsql", f.structuralIDs(other, 1), "VERIFIED")
		if _, err := NewQueryScopeHostResult(host, mustScope(t, f, assets, foreign)); err == nil {
			t.Errorf("%s: a result of the other host's rows was sealed as this host's", host)
		}
	}
}

func mustScope(t *testing.T, f *faninFixture, assets []queryregistrycore.Pin, dir string) QueryScopeResult {
	t.Helper()
	var results []OrdinaryQuerySemanticsResult
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		r, err := DecodeOrdinaryQuerySemanticsResult(data)
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, r)
	}
	scope, err := OrdinaryScopeResult(f.v, assets, results)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

// Two passes that judged the same tuple are refused, not merged: a JSON single-case run and the two-case run share
// a tuple, so only one of them is stored.
func TestOrdinaryScopeResultsRefuseAnOverlappingPass(t *testing.T) {
	f := faninFixtureFor(t)
	dir := t.TempDir()
	ids := f.structuralIDs("windows-amd64", 2)
	for name, shard := range map[string][]string{"json-zero-edit": ids, "json-history": ids[:1]} {
		if err := os.WriteFile(filepath.Join(dir, name+".json"), ordinaryJSON(t, faninShard(shard, slices.Repeat([]string{"VERIFIED"}, len(shard)), true)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var results []OrdinaryQuerySemanticsResult
	for _, name := range []string{"json-zero-edit", "json-history"} {
		data, err := os.ReadFile(filepath.Join(dir, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		r, err := DecodeOrdinaryQuerySemanticsResult(data)
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, r)
	}
	if _, err := OrdinaryScopeResult(f.v, []queryregistrycore.Pin{faninOrdinaryAsset}, results); err == nil || faninCode(t, err) != "QUERY_SCOPE_OVERLAP" {
		t.Fatalf("overlapping passes: %v", err)
	}
}

// The pinned sources of the expectations exist and the pins cover each by content; a missing one is refused.
func TestOrdinaryScopeAssetPins(t *testing.T) {
	read := func(path string) ([]byte, error) {
		return os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(path)))
	}
	pins, err := OrdinaryScopeAssetPins(read)
	if err != nil || len(pins) != len(OrdinaryScopeAssetPaths) {
		t.Fatalf("pins: %v %v", pins, err)
	}
	for i, p := range pins {
		if p.Path != OrdinaryScopeAssetPaths[i] || p.Bytes == 0 {
			t.Errorf("pin %d: %+v", i, p)
		}
	}
	if _, err := OrdinaryScopeAssetPins(func(path string) ([]byte, error) {
		if path == OrdinaryScopeAssetPaths[2] {
			return nil, os.ErrNotExist
		}
		return read(path)
	}); err == nil {
		t.Fatal("a missing expectation source was pinned")
	}
}

// The native regressions run on Windows as they always did, on Linux only while the collection asks for them (the
// required foundation run must not gain their time or failures), and on no other host.
func TestOrdinaryHostPlan(t *testing.T) {
	for _, c := range []struct {
		platform   string
		collecting bool
		skip       bool
	}{
		{"windows-amd64", false, false},
		{"windows-amd64", true, false},
		{"linux-amd64", false, true},
		{"linux-amd64", true, false},
		{"darwin-arm64", true, true},
		{"windows-arm64", true, true},
	} {
		if skip := ordinaryHostPlan(c.platform, c.collecting); (skip != "") != c.skip {
			t.Errorf("%s collecting=%v: skip %q", c.platform, c.collecting, skip)
		}
	}
}
