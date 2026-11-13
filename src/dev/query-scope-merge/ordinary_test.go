package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// ordinaryRows are the registered ordinary rows (Required per platform), read from the fold of a merge that stored no
// ordinary file: every row of an absent scope is BLOCKED = Required.
func ordinaryRows(t *testing.T, f *fixture) []kit.QueryScopeCounter {
	t.Helper()
	rowsOnce.Do(func() { rowsCache = ordinaryRowsOf(t, f) })
	return slices.Clone(rowsCache)
}

var (
	rowsOnce  sync.Once
	rowsCache []kit.QueryScopeCounter
)

func ordinaryRowsOf(t *testing.T, f *fixture) []kit.QueryScopeCounter {
	t.Helper()
	rep := merge(root, expected(), twoHosts(t, f))
	if rep.Completion == nil {
		t.Fatalf("%s %s", rep.Status, rep.Code)
	}
	var rows []kit.QueryScopeCounter
	for _, c := range rep.Completion.Counters {
		if c.Scope == kit.QueryScopeOrdinary {
			rows = append(rows, kit.QueryScopeCounter{Kind: c.Kind, Platform: c.Platform, Required: c.Required, Blocked: c.Required})
		}
	}
	if len(rows) != 2 || rows[0].Required == 0 {
		t.Fatalf("ordinary rows %+v", rows)
	}
	return rows
}

// ordinaryHostFile is the host's stored ordinary scope result with the given judged rows of its own platform; the
// other host's row stays BLOCKED, as the store step writes it.
func ordinaryHostFile(t *testing.T, f *fixture, host string, verified, failed uint64, edit func(*kit.QueryScopeResult)) kit.QueryScopeHostResult {
	t.Helper()
	rows := ordinaryRows(t, f)
	for i := range rows {
		if rows[i].Platform == host {
			rows[i].Verified, rows[i].Failed = verified, failed
			rows[i].Blocked -= verified + failed
		}
	}
	assets, err := kit.OrdinaryScopeAssetPins(func(path string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(path))) })
	if err != nil {
		t.Fatal(err)
	}
	first := ""
	if failed > 0 {
		first = "ORDINARY_CAPTURE_MISMATCH"
	}
	r := kit.NewQueryScopeResult(kit.QueryScopeOrdinary, queryregistrycore.Digest(f.registry), assets, rows, first)
	if edit != nil {
		edit(&r)
	}
	h, err := kit.NewQueryScopeHostResult(host, r)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func withOrdinary(t *testing.T, f *fixture, host string, h kit.QueryScopeHostResult) string {
	t.Helper()
	return writeHost(t, f, host, nil, func(rs map[string]kit.QueryScopeHostResult) { rs[ordinaryKey] = h })
}

func ordinaryVerified(rep report) (windowsRow, linuxRow uint64) {
	for _, c := range rep.Completion.Counters {
		if c.Scope == kit.QueryScopeOrdinary {
			if c.Platform == windows {
				windowsRow = c.Verified
			} else {
				linuxRow = c.Verified
			}
		}
	}
	return windowsRow, linuxRow
}

// The ordinary file is optional per host (its collection is a step of its own), but one that is present is merged
// like the other scopes: each host's own rows, the other host's rows BLOCKED.
func TestMergeTakesTheOrdinaryFileOfEachHost(t *testing.T) {
	f := fixtureFor(t)
	both := twoHosts(t, f)
	both[windows] = withOrdinary(t, f, windows, ordinaryHostFile(t, f, windows, 97, 0, nil))
	both[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 90, 0, nil))
	rep := merge(root, expected(), both)
	if rep.Status != statusMerged || rep.Completion == nil || rep.Scopes[kit.QueryScopeOrdinary] == "" {
		t.Fatalf("both hosts: %s %s %v", rep.Status, rep.Code, rep.Scopes)
	}
	if w, l := ordinaryVerified(rep); w != 97 || l != 90 {
		t.Errorf("ordinary verified windows %d linux %d, want 97 and 90", w, l)
	}
	if tot := rep.Completion.Total; tot.Verified != 2*(67+263+30+60)+97+90 || tot.Failed != 0 || rep.Completion.Complete {
		t.Errorf("total %+v", tot)
	}
	for _, in := range rep.Inputs {
		if len(in.Files) != 6 {
			t.Errorf("%s: %d files, want the closed five and the ordinary one", in.Host, len(in.Files))
		}
	}

	// one host's file stands alone: the other host's ordinary rows stay BLOCKED
	one := twoHosts(t, f)
	one[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 90, 0, nil))
	rep = merge(root, expected(), one)
	if rep.Status != statusMerged || rep.Completion == nil {
		t.Fatalf("one host: %s %s", rep.Status, rep.Code)
	}
	if w, l := ordinaryVerified(rep); w != 0 || l != 90 {
		t.Errorf("one-sided ordinary verified windows %d linux %d, want 0 and 90", w, l)
	}

	// neither: the scope is absent, the other evidence unchanged
	rep = merge(root, expected(), twoHosts(t, f))
	if w, l := ordinaryVerified(rep); rep.Status != statusMerged || w != 0 || l != 0 || rep.Scopes[kit.QueryScopeOrdinary] != "" {
		t.Errorf("no ordinary files: %s %d %d %v", rep.Status, w, l, rep.Scopes)
	}
}

// A FAILED ordinary tuple is kept as the run's FAIL, like a FAILED predicate or SVC judgment.
func TestMergeKeepsAnOrdinaryFailure(t *testing.T) {
	f := fixtureFor(t)
	dirs := twoHosts(t, f)
	dirs[windows] = withOrdinary(t, f, windows, ordinaryHostFile(t, f, windows, 70, 2, nil))
	rep := merge(root, expected(), dirs)
	if rep.Status != statusFail || rep.Completion == nil || rep.Completion.Total.Failed != 2 || rep.Code != "ORDINARY_CAPTURE_MISMATCH" {
		t.Fatalf("%s %s %+v", rep.Status, rep.Code, rep.Completion)
	}
}

// Evidence that is not this host's, not this checkout's or not intact is refused, never folded.
func TestMergeRefusesAnOrdinaryFileThatIsNotTheHostsOwn(t *testing.T) {
	f := fixtureFor(t)
	good := func() map[string]string {
		d := twoHosts(t, f)
		d[windows] = withOrdinary(t, f, windows, ordinaryHostFile(t, f, windows, 97, 0, nil))
		d[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 90, 0, nil))
		return d
	}
	if rep := merge(root, expected(), good()); rep.Status != statusMerged {
		t.Fatalf("honest pair: %s %s", rep.Status, rep.Code)
	}
	refused := func(name, code string, dirs map[string]string) {
		t.Helper()
		if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Completion != nil || (code != "" && rep.Code != code) {
			t.Errorf("%s: %s %s", name, rep.Status, rep.Code)
		}
	}
	// the Windows result under the Linux name
	dirs := good()
	dirs[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, windows, 97, 0, nil))
	refused("windows ordinary result under linux name", "QUERY_SCOPE_HOST_MISMATCH", dirs)
	// another scope's file under the ordinary name
	dirs = good()
	dirs[linux] = writeHost(t, f, linux, nil, func(rs map[string]kit.QueryScopeHostResult) { rs[ordinaryKey] = rs[kit.QueryScopePredicate] })
	refused("predicate result under the ordinary name", "QUERY_SCOPE_HOST_MISMATCH", dirs)
	// a tampered count, digest and truncation of the stored file
	name := kit.QueryScopeHostFile(kit.QueryScopeOrdinary, linux)
	for label, change := range map[string]func(string) string{
		"count":     func(s string) string { return strings.Replace(s, `"verified":90`, `"verified":9000`, 1) },
		"digest":    func(s string) string { return strings.Replace(s, `"host_sha256":"`, `"host_sha256":"0`, 1) },
		"truncated": func(s string) string { return s[:len(s)/2] },
	} {
		dirs = good()
		path := filepath.Join(dirs[linux], name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if edited := change(string(data)); edited == string(data) {
			t.Fatalf("%s: the edit changed nothing", label)
		} else if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		refused(label, "", dirs)
	}
	// evidence pinned to other expectation sources than this checkout's (one host's file alone, so that the merge of
	// the two hosts does not refuse the difference first)
	dirs = twoHosts(t, f)
	dirs[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 90, 0, func(r *kit.QueryScopeResult) {
		r.Assets = append([]queryregistrycore.Pin{}, r.Assets...)
		r.Assets[0].SHA256 = strings.Repeat("0", 64)
		*r = kit.NewQueryScopeResult(r.Scope, r.RegistrySHA256, r.Assets, r.Counters, r.FirstFailure)
	}))
	refused("asset pin of another checkout", "QUERY_SCOPE_ASSET_PIN", dirs)
	// a verified count beyond the registered denominator
	dirs = good()
	dirs[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 90, 0, func(r *kit.QueryScopeResult) {
		r.Counters[1].Required++
		r.Counters[1].Blocked++
		*r = kit.NewQueryScopeResult(r.Scope, r.RegistrySHA256, r.Assets, r.Counters, r.FirstFailure)
	}))
	refused("denominator changed", "", dirs)
	// a stranger next to the closed set is still refused
	dirs = good()
	if err := os.WriteFile(filepath.Join(dirs[linux], "query-scope-ordinary-extra.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	refused("extra file", "QUERY_SCOPE_HOST_FILES", dirs)
}
