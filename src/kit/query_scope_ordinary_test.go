package kit

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
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

// ordinaryScopeRecorder keeps the public result of a selected-shard regression for TestOrdinaryScopeStore when
// QueryScopeOrdinaryResultsEnv names a directory. The rule is the one of #362 L2/L3: a run with a run-integrity error
// stores nothing, a genuine FAILED verdict is stored. The regression takes the recorder as its first statement, so that
// its cleanup is the last one to run (cleanups are LIFO): a cleanup registered earlier that fails later still
// counts. At that point the test has either failed (a build, a start, a replay parity, a ledger, a negative control, a
// panic; a timeout never reaches the cleanup), skipped, or ended, and only a test that did not fail stores:
//   - a PASS stores its result;
//   - a verdict-only failure stores it as well and then fails the test. It is reported by VerdictFailed, which only
//     Judged (a complete, bound run whose only deviation is tuples FAILED because the expected tree, captures or
//     metadata differ from the native result) allows; the test goes on with its other checks, and any failure of
//     those clears the claim, so the FAILED tuples reach the merge only from a run whose integrity held.
//
// A stored file is never overwritten. Without the environment nothing is stored, but the verdict still fails the test.
type ordinaryScopeRecorder struct {
	name    string
	dir     string
	result  *OrdinaryQuerySemanticsResult
	verdict string
}

// ordinaryDivergenceCodes are the tuple codes that say the independent expectation differs from the native result.
var ordinaryDivergenceCodes = []string{"ORDINARY_EXPECTED_TREE_MISMATCH", "ORDINARY_CAPTURE_MISMATCH", "ORDINARY_CAPTURE_MATCHES_MISMATCH", "ORDINARY_METADATA_MISMATCH"}

func ordinaryScopeCollect(t *testing.T, name string) *ordinaryScopeRecorder {
	t.Helper()
	r := &ordinaryScopeRecorder{name: name, dir: os.Getenv(QueryScopeOrdinaryResultsEnv)}
	if r.dir != "" && !slices.Contains(ordinaryScopeResultNames, name) {
		t.Fatalf("ordinary scope result %q is not a collected one", name)
	}
	t.Cleanup(func() { r.finish(t) })
	return r
}

// ordinaryVerdictOnly reports whether a public result is a finished, bound judgment of exactly `required` tuples
// whose only deviation from PASS is tuples FAILED by a divergence of the expectation from the native result.
func ordinaryVerdictOnly(r OrdinaryQuerySemanticsResult, err error, required int) bool {
	if err != nil || r.ExecutionStatus != StatusCompleted || !r.ProcessingComplete || !r.BindingValid || r.BaseVerified.Assessment != AssessPass ||
		r.Tuple.Required != uint64(required) || r.Tuple.Failed == 0 || r.Tuple.Verified+r.Tuple.Failed != r.Tuple.Required ||
		len(r.SelectedObligationIDs) != required || len(r.Obligations) != required {
		return false
	}
	for _, o := range r.Obligations {
		if o.State != ordinaryTupleVerified && !(o.State == ordinaryTupleFailed && slices.Contains(ordinaryDivergenceCodes, o.Code)) {
			return false
		}
	}
	return true
}

// Judged keeps the public result and reports whether the run is verdict-only (see ordinaryVerdictOnly). A nil
// recorder keeps nothing and reports false: the caller then checks for PASS.
func (r *ordinaryScopeRecorder) Judged(result OrdinaryQuerySemanticsResult, err error, required int) bool {
	if r == nil {
		return false
	}
	r.result = &result
	return ordinaryVerdictOnly(result, err, required)
}

// VerdictFailed defers the FAILED verdict: the test fails with it once its cleanups have run and nothing else failed.
func (r *ordinaryScopeRecorder) VerdictFailed(format string, args ...any) {
	r.verdict = fmt.Sprintf(format, args...)
}

func (r *ordinaryScopeRecorder) finish(t *testing.T) {
	if t.Failed() || t.Skipped() {
		if r.dir != "" {
			t.Logf("ordinary scope result %q not stored: the test failed or skipped before its verdict was final", r.name)
		}
		if r.verdict != "" {
			t.Logf("verdict (not stored): %s", r.verdict)
		}
		return
	}
	if r.verdict != "" {
		defer t.Error(r.verdict)
	}
	if r.dir == "" {
		return
	}
	if r.result == nil {
		t.Errorf("ordinary scope result %q was never recorded", r.name)
		return
	}
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		t.Errorf("ordinary scope result %q: %v", r.name, err)
		return
	}
	f, err := os.OpenFile(filepath.Join(r.dir, r.name+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Errorf("ordinary scope result %q: %v", r.name, err)
		return
	}
	_, werr := f.Write(ordinaryJSON(t, *r.result))
	if err := f.Close(); werr != nil || err != nil {
		t.Errorf("ordinary scope result %q write: %v %v", r.name, werr, err)
	}
}

// ordinaryHostScope folds the stored results of a directory into the host's ordinary scope result. The directory
// is closed: only the named results, each decoded and recomputed by DecodeOrdinaryQuerySemanticsResult; a name
// missing is a shard no run judged (it stays BLOCKED, all of them when the directory is empty or absent), a stranger is a defect.
func ordinaryHostScope(t *testing.T, v *queryregistrycore.Validated, assets []queryregistrycore.Pin, host, dir string) QueryScopeHostResult {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) { // no directory: no test stored a result
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
	// no stored result is a zero-judged file, not an absent one: the merge then reports every tuple of the host unjudged
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

// A result is stored only by a test whose run integrity held (the #362 L2/L3 rule, R2 of #435): a PASS stores it; a
// test whose only failure is the FAILED verdict stores it too (and still fails); a test that failed or skipped for any
// other reason leaves no file. The inner tests run through testing.RunTests and print their own
// "--- FAIL: ordinaryRecord…" lines, which do not match the CI completion patterns.
func TestOrdinaryScopeRecorderStoresVerdictsButNotIntegrityFailures(t *testing.T) {
	f := faninFixtureFor(t)
	ids := ordinaryScopeClaimIDs(f.v)["windows-amd64"]
	// two tuples judged against the native tree: one VERIFIED, one FAILED by a divergence of the expectation
	failedShard := func(code string) OrdinaryQuerySemanticsResult {
		r := faninShard(ids[:2], []string{"VERIFIED", "FAILED"}, true)
		r.Obligations[1].Code = code
		r.Findings, r.Code = nil, ""
		r.finding(code, ids[1], "")
		r.BaseVerified = Verdict{StatusCompleted, AssessPass} // the recomputed base held: the tuples were judged
		for _, id := range []string{"case-binding", "case-status", "tree", "verdict", "incremental-equality", "incremental-route", "expectations", "query-equality", "api", "query-expectations", "fact-reproduction", "dynamic-sql"} {
			gate := ReplayGate{ID: id, Recomputed: 1}
			gate.finish()
			r.Gates = append(r.Gates, gate)
		}
		r.ProcessingComplete, r.Complete = true, true
		return r
	}
	diverged := failedShard("ORDINARY_EXPECTED_TREE_MISMATCH")
	if !ordinaryVerdictOnly(diverged, nil, 2) {
		t.Fatalf("a divergence of the expectation is a verdict: %+v", diverged)
	}
	for name, c := range map[string]struct {
		r        OrdinaryQuerySemanticsResult
		err      error
		required int
	}{
		"an error":                 {diverged, errors.New("stopped"), 2},
		"a binding failure":        {failedShard("ORDINARY_EXPECTED_TREE_BINDING"), nil, 2},
		"a source binding failure": {failedShard("ORDINARY_TUPLE_SOURCE_BINDING"), nil, 2},
		"a blocked tuple":          {faninShard(ids[:2], []string{"VERIFIED", "BLOCKED"}, true), nil, 2},
		"all VERIFIED":             {faninShard(ids[:2], []string{"VERIFIED", "VERIFIED"}, true), nil, 2},
		"another number of tuples": {diverged, nil, 3},
		"a binding that is invalid": {func() OrdinaryQuerySemanticsResult {
			r := diverged
			r.BindingValid = false
			return r
		}(), nil, 2},
		"a base that is not PASS": {func() OrdinaryQuerySemanticsResult {
			r := diverged
			r.BaseVerified.Assessment = AssessFail
			return r
		}(), nil, 2},
	} {
		if ordinaryVerdictOnly(c.r, c.err, c.required) {
			t.Errorf("%s was taken for a verdict-only failure", name)
		}
	}

	inner := func(dir, name, result string, body func(t *testing.T, rec *ordinaryScopeRecorder)) (passed bool) {
		t.Setenv(QueryScopeOrdinaryResultsEnv, dir)
		return testing.RunTests(func(string, string) (bool, error) { return true, nil }, []testing.InternalTest{{Name: name, F: func(t *testing.T) {
			body(t, ordinaryScopeCollect(t, result))
		}}})
	}
	pass := newOrdinarySemanticsResult()
	record := func(rec *ordinaryScopeRecorder) { rec.Judged(pass, nil, 0) }
	// as the regressions do: the verdict is deferred only for a run Judged takes for verdict-only
	verdict := func(t *testing.T, rec *ordinaryScopeRecorder) {
		if !rec.Judged(diverged, nil, 2) {
			t.Fatal("the divergence was not taken for a verdict")
		}
		rec.VerdictFailed("2 tuples FAILED")
	}
	stored := func(dir string) []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return names
	}

	// the reviewer's cases: the PASS and the verdict-only run store, every integrity failure stores nothing
	dir := t.TempDir()
	if !inner(dir, "ordinaryRecordPasses", "json-zero-edit", func(t *testing.T, rec *ordinaryScopeRecorder) { record(rec) }) {
		t.Error("a PASS failed")
	}
	if inner(dir, "ordinaryRecordVerdictOnly", "json-history", func(t *testing.T, rec *ordinaryScopeRecorder) { verdict(t, rec) }) {
		t.Error("the FAILED verdict did not fail the test")
	}
	if inner(dir, "ordinaryRecordFailsLater", "sql-postgresql-sql", func(t *testing.T, rec *ordinaryScopeRecorder) {
		record(rec)
		t.Error("a check after the record failed")
	}) {
		t.Error("a failing test passed")
	}
	if inner(dir, "ordinaryRecordVerdictThenIntegrity", "sql-tsql", func(t *testing.T, rec *ordinaryScopeRecorder) {
		verdict(t, rec)
		t.Error("a negative control after the verdict did not behave")
	}) {
		t.Error("a failing test passed")
	}
	if got := stored(dir); !slices.Equal(got, []string{"json-history.json", "json-zero-edit.json"}) {
		t.Fatalf("stored %v: the PASS and the verdict-only run store, the integrity failures do not", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, "json-history.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeOrdinaryQuerySemanticsResult(data); err != nil || got.Tuple.Failed != 1 || got.Tuple.Verified != 1 {
		t.Fatalf("the stored verdict-only result keeps its FAILED tuple: %+v %v", got.Tuple, err)
	}

	// K3 of the review: shapes the real tests have, and a cleanup that fails after the verdict
	dir2 := t.TempDir()
	inner(dir2, "ordinaryRecordSubtestFails", "json-zero-edit", func(t *testing.T, rec *ordinaryScopeRecorder) {
		record(rec)
		t.Run("negative", func(t *testing.T) { t.Error("a negative control failed") })
	})
	inner(dir2, "ordinaryRecordSkips", "json-history", func(t *testing.T, rec *ordinaryScopeRecorder) {
		record(rec)
		t.Skip("skipped after the record")
	})
	// R6 of the review: a cleanup the test or a helper registers (a temporary directory, a hook restore) fails after
	// the verdict was reached. The recorder is the first statement, so its cleanup runs last and sees the failure.
	inner(dir2, "ordinaryRecordCleanupFails", "sql-tsql", func(t *testing.T, rec *ordinaryScopeRecorder) {
		t.Cleanup(func() { t.Error("a cleanup registered by the test failed") })
		record(rec)
	})
	inner(dir2, "ordinaryRecordCleanupFailsAfterVerdict", "sql-postgresql-sql", func(t *testing.T, rec *ordinaryScopeRecorder) {
		t.Cleanup(func() { t.Error("a cleanup registered by the test failed") })
		verdict(t, rec)
	})
	if got := stored(dir2); len(got) != 0 {
		t.Errorf("stored %v: a test that failed anywhere, skipped, or whose cleanup failed stores nothing", got)
	}
}

// R6 of #435: the recorder stores from the cleanup that runs last only when it is the first one registered, so every
// call of ordinaryScopeCollect must be the first statement of its test function (or function literal). This reads the
// package's test sources: a collecting regression that registers a cleanup first would store before that cleanup fails.
func TestOrdinaryScopeCollectIsTheFirstStatement(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no test sources: %v", err)
	}
	isCollect := func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return false
		}
		id, ok := c.Fun.(*ast.Ident)
		return ok && id.Name == "ordinaryScopeCollect"
	}
	all, first := 0, 0
	for _, name := range files {
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if isCollect(n) {
				all++
			}
			var body *ast.BlockStmt
			switch fn := n.(type) {
			case *ast.FuncDecl:
				body = fn.Body
			case *ast.FuncLit:
				body = fn.Body
			}
			if body != nil && len(body.List) > 0 {
				ast.Inspect(body.List[0], func(m ast.Node) bool {
					if _, nested := m.(*ast.FuncLit); nested {
						return false
					}
					if isCollect(m) {
						first++
					}
					return true
				})
			}
			return true
		})
	}
	if all < 5 || first != all {
		t.Errorf("%d calls of ordinaryScopeCollect, %d of them the first statement of their function: the recorder must be registered before any other cleanup", all, first)
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
			rec := ordinaryScopeCollect(t, result)
			if ids != nil {
				rec.Judged(verified(ids), nil, len(ids))
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

// R1 of #435: a host whose regressions all failed stores no result, and the store still writes its file: every tuple
// of the host unjudged (a zero-judged file), not an absent one. An absent directory is the same case.
func TestOrdinaryHostScopeWithNothingStoredIsZeroJudged(t *testing.T) {
	f := faninFixtureFor(t)
	claims := map[string]uint64{"windows-amd64": 97, "linux-amd64": 97}
	for name, dir := range map[string]string{"empty": t.TempDir(), "absent": filepath.Join(t.TempDir(), "never-created")} {
		h := ordinaryHostScope(t, f.v, []queryregistrycore.Pin{faninOrdinaryAsset}, "linux-amd64", dir)
		for _, c := range h.Result.Counters {
			if c.Verified != 0 || c.Failed != 0 || c.Unprocessed != 0 || c.Blocked != c.Required {
				t.Errorf("%s: %+v", name, c)
			}
		}
		if got := OrdinaryScopeUnjudged(h.Result, claims); got["linux-amd64"] != 97 || got["windows-amd64"] != 97 || OrdinaryScopeOverclaim(h.Result, claims) {
			t.Errorf("%s: unjudged %v", name, got)
		}
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
