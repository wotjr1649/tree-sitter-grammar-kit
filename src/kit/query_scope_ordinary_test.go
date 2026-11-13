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

// ordinaryScopeRecord keeps the public pass result of a selected-shard regression for TestOrdinaryScopeStore when
// QueryScopeOrdinaryResultsEnv names a directory, and stores it only once the test has finished: a test that failed
// anywhere (a FAILED tuple, a later check, a panic or a timeout that never reaches the cleanup) or skipped stores
// nothing, so that no claim includes tuples of a test that did not complete with PASS; its tuples stay BLOCKED and the
// collection step names the failed test. A stored file is never overwritten.
func ordinaryScopeRecord(t *testing.T, name string, result OrdinaryQuerySemanticsResult) {
	t.Helper()
	dir := os.Getenv(QueryScopeOrdinaryResultsEnv)
	if dir == "" {
		return
	}
	if !slices.Contains(ordinaryScopeResultNames, name) {
		t.Fatalf("ordinary scope result %q is not a collected one", name)
	}
	data := ordinaryJSON(t, result)
	t.Cleanup(func() {
		if t.Failed() || t.Skipped() {
			t.Logf("ordinary scope result %q not stored: the test did not complete with PASS", name)
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Errorf("ordinary scope result %q: %v", name, err)
			return
		}
		f, err := os.OpenFile(filepath.Join(dir, name+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			t.Errorf("ordinary scope result %q: %v", name, err)
			return
		}
		_, werr := f.Write(data)
		if err := f.Close(); werr != nil || err != nil {
			t.Errorf("ordinary scope result %q write: %v %v", name, werr, err)
		}
	})
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
	claimable := ordinaryScopeClaimIDs(v)[host]
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
		// the identity of every judged tuple is bound here: the merge sees counts only (OrdinaryScopeOverclaim)
		for _, id := range result.SelectedObligationIDs {
			if !slices.Contains(claimable, id) {
				t.Fatalf("%s selects %s, a tuple outside the shards this collection may claim on %s", e.Name(), id, host)
			}
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
	t.Logf("BASIS %s", OrdinaryScopeBasis)
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
		ids := ordinaryScopeClaimIDs(f.v)[host][:4]
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
		write(foreign, "sql-tsql", ordinaryScopeClaimIDs(f.v)[other][:1], "VERIFIED")
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

// A result is stored only by a test that completed with PASS: one that failed after recording it (a later check, a
// FAILED tuple) or skipped leaves no file, so no claim includes the tuples of a test that did not pass (F1 of #435).
// The inner tests run through testing.RunTests; the failing ones print their own "--- FAIL: ordinaryRecord…" lines.
func TestOrdinaryScopeRecordStoresOnlyPassingTests(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(QueryScopeOrdinaryResultsEnv, dir)
	inner := func(name, result string, body func(t *testing.T)) {
		testing.RunTests(func(string, string) (bool, error) { return true, nil }, []testing.InternalTest{{Name: name, F: func(t *testing.T) {
			ordinaryScopeRecord(t, result, newOrdinarySemanticsResult())
			body(t)
		}}})
	}
	inner("ordinaryRecordPasses", "json-zero-edit", func(t *testing.T) {})
	inner("ordinaryRecordFailsLater", "json-history", func(t *testing.T) { t.Error("a check after the record failed") })
	inner("ordinaryRecordSkips", "sql-postgresql-sql", func(t *testing.T) { t.Skip("skipped after the record") })
	inner("ordinaryRecordFailsNow", "sql-tsql", func(t *testing.T) { t.Fatal("a FAILED tuple") })
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "json-zero-edit.json" {
		t.Fatalf("stored %v %v: only the passing test may leave a file", entries, err)
	}
}

// The Linux run of PR CI 43408861497: both JSON regressions recorded their 2 and 14 tuples and then failed (the r9
// replay refused the subject), the PostgreSQL one failed before it recorded anything, only TSQL passed. The stored
// file used to claim 88 VERIFIED (the 16 JSON tuples of the failed tests too); it may claim the 72 of TSQL only.
func TestOrdinaryScopeStoreClaimsOnlyWhatPassed(t *testing.T) {
	f := faninFixtureFor(t)
	const host = "linux-amd64"
	ids := ordinaryScopeClaimIDs(f.v)[host]
	verified := func(ids []string) OrdinaryQuerySemanticsResult {
		return faninShard(ids, slices.Repeat([]string{"VERIFIED"}, len(ids)), true)
	}
	dir := t.TempDir()
	t.Setenv(QueryScopeOrdinaryResultsEnv, dir)
	run := func(name, result string, ids []string, fails bool) {
		testing.RunTests(func(string, string) (bool, error) { return true, nil }, []testing.InternalTest{{Name: name, F: func(t *testing.T) {
			if ids != nil {
				ordinaryScopeRecord(t, result, verified(ids))
			}
			if fails {
				t.Error("the r9 replay refused the subject")
			}
		}}})
	}
	run("ordinaryLinuxJSONZeroEdit", "json-zero-edit", ids[:2], true)
	run("ordinaryLinuxJSONHistory", "json-history", ids[2:16], true)
	run("ordinaryLinuxPostgreSQL", "sql-postgresql-sql", nil, true)
	run("ordinaryLinuxTSQL", "sql-tsql", ids[16:88], false)
	h := ordinaryHostScope(t, f.v, []queryregistrycore.Pin{faninOrdinaryAsset}, host, dir)
	for _, c := range h.Result.Counters {
		if c.Platform == host && (c.Verified != 72 || c.Failed != 0 || c.Unprocessed != 0) {
			t.Errorf("the Linux ordinary row claims %+v, want exactly the 72 VERIFIED of the passing TSQL regression", c)
		}
	}
	claims := map[string]uint64{"windows-amd64": 97, "linux-amd64": 97}
	if OrdinaryScopeOverclaim(h.Result, claims) || OrdinaryScopeUnjudged(h.Result, claims)[host] != 25 {
		t.Errorf("the file is not reported 25 tuples short on %s: %v", host, OrdinaryScopeUnjudged(h.Result, claims))
	}
}

// What the collection may claim is derived from the registry: 97 tuples per host in the selected shards (JSON 1+1 and
// 7+7, PostgreSQL 9, TSQL 72), none shared with another shard, and the ceiling and the fill read the counts only.
func TestOrdinaryScopeClaimsAreTheSelectedShards(t *testing.T) {
	f := faninFixtureFor(t)
	for _, platform := range []string{"windows-amd64", "linux-amd64"} {
		perShard := map[string]int{}
		for _, c := range f.v.Coordinates() {
			if c.Kind == queryregistrycore.Structural && c.Platform == platform && slices.Contains(ordinaryScopeClaimCases[c.Route], c.Case) {
				perShard[c.Route+"/"+c.Case]++
			}
		}
		want := map[string]int{"json/alt-json-B01-05": 1, "json/alt-json-B01-09": 1, "json/json-feature-r1": 7, "json/json-query-r1": 7, "postgresql-sql/alt-postgresql-sql-B04-22": 9, "tsql/alt-tsql-B01-r001": 72}
		if !reflect.DeepEqual(perShard, want) {
			t.Errorf("%s: selected shards hold %v, want %v", platform, perShard, want)
		}
		if ids := ordinaryScopeClaimIDs(f.v)[platform]; len(ids) != 97 {
			t.Errorf("%s: %d claimable tuples, want 97", platform, len(ids))
		}
	}
	claims, err := OrdinaryScopeClaims(f.req)
	if err != nil || !reflect.DeepEqual(claims, map[string]uint64{"windows-amd64": 97, "linux-amd64": 97}) {
		t.Fatalf("claims %v %v", claims, err)
	}
	row := func(platform string, verified, failed, unprocessed uint64) QueryScopeCounter {
		return QueryScopeCounter{Kind: queryregistrycore.Structural, Platform: platform, Required: 1000, Verified: verified, Failed: failed, Unprocessed: unprocessed, Blocked: 1000 - verified - failed - unprocessed}
	}
	result := func(w, l QueryScopeCounter) QueryScopeResult {
		return NewQueryScopeResult(QueryScopeOrdinary, "r", nil, []QueryScopeCounter{w, l}, "")
	}
	for _, c := range []struct {
		name                 string
		r                    QueryScopeResult
		overclaim            bool
		unjudgedW, unjudgedL uint64
	}{
		{"filled", result(row("windows-amd64", 97, 0, 0), row("linux-amd64", 90, 7, 0)), false, 0, 0},
		{"under-filled", result(row("windows-amd64", 90, 0, 0), row("linux-amd64", 0, 0, 0)), false, 7, 97},
		{"verified beyond the selection", result(row("windows-amd64", 98, 0, 0), row("linux-amd64", 0, 0, 0)), true, 0, 97},
		{"failed beyond the selection", result(row("windows-amd64", 97, 1, 0), row("linux-amd64", 0, 0, 0)), true, 0, 97},
		{"unprocessed beyond the selection", result(row("windows-amd64", 90, 0, 8), row("linux-amd64", 0, 0, 0)), true, 7, 97},
		{"all of the denominator", result(row("windows-amd64", 1000, 0, 0), row("linux-amd64", 1000, 0, 0)), true, 0, 0},
	} {
		unjudged := OrdinaryScopeUnjudged(c.r, claims)
		if got := OrdinaryScopeOverclaim(c.r, claims); got != c.overclaim || unjudged["windows-amd64"] != c.unjudgedW || unjudged["linux-amd64"] != c.unjudgedL {
			t.Errorf("%s: overclaim %v unjudged %v", c.name, got, unjudged)
		}
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
