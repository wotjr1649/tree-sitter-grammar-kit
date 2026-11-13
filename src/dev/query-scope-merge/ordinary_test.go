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
// like the other scopes: each host's own rows, the other host's rows BLOCKED. A host that judged fewer of the
// selected shards' tuples than the registry derives (97 each) is reported under-filled.
func TestMergeTakesTheOrdinaryFileOfEachHost(t *testing.T) {
	f := fixtureFor(t)
	both := twoHosts(t, f)
	both[windows] = withOrdinary(t, f, windows, ordinaryHostFile(t, f, windows, 97, 0, nil))
	both[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 97, 0, nil))
	rep := merge(root, expected(), both)
	if rep.Status != statusMerged || rep.Completion == nil || rep.Scopes[kit.QueryScopeOrdinary] == "" {
		t.Fatalf("both hosts: %s %s %v", rep.Status, rep.Code, rep.Scopes)
	}
	if w, l := ordinaryVerified(rep); w != 97 || l != 97 {
		t.Errorf("ordinary verified windows %d linux %d, want 97 and 97", w, l)
	}
	if tot := rep.Completion.Total; tot.Verified != 2*(67+263+30+60)+97+97 || tot.Failed != 0 || rep.Completion.Complete {
		t.Errorf("total %+v", tot)
	}
	for _, in := range rep.Inputs {
		if len(in.Files) != 6 || in.OrdinaryCode != "" {
			t.Errorf("%s: %d files (want the closed five and the ordinary one), dropped %q", in.Host, len(in.Files), in.OrdinaryCode)
		}
	}
	// the report says what the VERIFIED rows rest on (F6 of #435) and how many tuples the selection derives
	if rep.Ordinary == nil || rep.Ordinary.Basis != kit.OrdinaryScopeBasis || !strings.Contains(rep.Ordinary.Basis, "synthetic reference rows and approval") ||
		rep.Ordinary.Claimable[windows] != 97 || rep.Ordinary.Claimable[linux] != 97 || rep.Ordinary.Code != "" || len(rep.Ordinary.Unjudged) != 0 {
		t.Errorf("ordinary report %+v", rep.Ordinary)
	}

	// one host's file stands alone: the other host's ordinary rows stay BLOCKED, and are reported unjudged
	one := twoHosts(t, f)
	one[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 97, 0, nil))
	rep = merge(root, expected(), one)
	if rep.Status != statusBlocked || rep.Code != "QUERY_SCOPE_ORDINARY_UNDERFILLED" || rep.Completion == nil || rep.Ordinary == nil || rep.Ordinary.Unjudged[windows] != 97 || len(rep.Ordinary.Unjudged) != 1 {
		t.Fatalf("one host: %s %s %+v", rep.Status, rep.Code, rep.Ordinary)
	}
	if w, l := ordinaryVerified(rep); w != 0 || l != 97 {
		t.Errorf("one-sided ordinary verified windows %d linux %d, want 0 and 97", w, l)
	}

	// a host that ran fewer of the selected tuples (a test that failed stores nothing): folded, reported under-filled
	short := twoHosts(t, f)
	short[windows] = withOrdinary(t, f, windows, ordinaryHostFile(t, f, windows, 97, 0, nil))
	short[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 90, 0, nil))
	rep = merge(root, expected(), short)
	if rep.Status != statusBlocked || rep.Code != "QUERY_SCOPE_ORDINARY_UNDERFILLED" || rep.Ordinary == nil || rep.Ordinary.Unjudged[linux] != 7 || len(rep.Ordinary.Unjudged) != 1 {
		t.Fatalf("under-filled host: %s %s %+v", rep.Status, rep.Code, rep.Ordinary)
	}
	if w, l := ordinaryVerified(rep); w != 97 || l != 90 {
		t.Errorf("under-filled ordinary verified windows %d linux %d, want 97 and 90", w, l)
	}

	// neither: the scope is absent, the other evidence unchanged
	rep = merge(root, expected(), twoHosts(t, f))
	if w, l := ordinaryVerified(rep); rep.Status != statusMerged || w != 0 || l != 0 || rep.Scopes[kit.QueryScopeOrdinary] != "" || rep.Ordinary != nil {
		t.Errorf("no ordinary files: %s %d %d %v %+v", rep.Status, w, l, rep.Scopes, rep.Ordinary)
	}
}

// A FAILED ordinary tuple is kept as the run's FAIL, like a FAILED predicate or SVC judgment, ahead of an under-fill.
func TestMergeKeepsAnOrdinaryFailure(t *testing.T) {
	f := fixtureFor(t)
	dirs := twoHosts(t, f)
	dirs[windows] = withOrdinary(t, f, windows, ordinaryHostFile(t, f, windows, 70, 2, nil))
	rep := merge(root, expected(), dirs)
	if rep.Status != statusFail || rep.Completion == nil || rep.Completion.Total.Failed != 2 || rep.Code != "ORDINARY_CAPTURE_MISMATCH" {
		t.Fatalf("%s %s %+v", rep.Status, rep.Code, rep.Completion)
	}
}

// What the selected shards do not derive is not accepted (F2 of #435): a host file that judges more tuples than the
// 97 the registry derives for the selection (the reviewer's mutant: every one of the 53,104 VERIFIED) is dropped, its
// rows stay BLOCKED, and the rest of the evidence is still folded.
func TestMergeDropsAnOverclaimedOrdinaryFile(t *testing.T) {
	f := fixtureFor(t)
	all := ordinaryRows(t, f)[0].Required
	for _, c := range []struct {
		name                           string
		windowsVerified, windowsFailed uint64
		linuxVerified                  uint64
		windowsDropped, linuxDropped   bool
	}{
		{"every tuple verified on both hosts", all, 0, all, true, true},
		{"every tuple verified on one host", 97, 0, all, false, true},
		{"one verified beyond the selection", 98, 0, 97, true, false},
		{"one failed beyond the selection", 97, 1, 97, true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dirs := twoHosts(t, f)
			dirs[windows] = withOrdinary(t, f, windows, ordinaryHostFile(t, f, windows, c.windowsVerified, c.windowsFailed, nil))
			dirs[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, c.linuxVerified, 0, nil))
			rep := merge(root, expected(), dirs)
			if rep.Status != statusBlocked || rep.Code != "QUERY_SCOPE_ORDINARY_REJECTED" || rep.Completion == nil || rep.Ordinary == nil || rep.Ordinary.Code != "QUERY_SCOPE_ORDINARY_OVERCLAIM" {
				t.Fatalf("%s %s %+v", rep.Status, rep.Code, rep.Ordinary)
			}
			dropped := map[string]bool{}
			for _, in := range rep.Inputs {
				dropped[in.Host] = in.OrdinaryCode == "QUERY_SCOPE_ORDINARY_OVERCLAIM"
			}
			if dropped[windows] != c.windowsDropped || dropped[linux] != c.linuxDropped {
				t.Errorf("dropped hosts %v, want windows %v linux %v", dropped, c.windowsDropped, c.linuxDropped)
			}
			// nothing above the selection survives: a dropped host's rows are BLOCKED, a kept host's are its own 97
			wantW, wantL := uint64(0), uint64(0)
			if !c.windowsDropped {
				wantW = 97
			}
			if !c.linuxDropped {
				wantL = 97
			}
			if w, l := ordinaryVerified(rep); w != wantW || l != wantL || rep.Completion.Total.Failed != 0 {
				t.Errorf("ordinary verified windows %d linux %d failed %d, want %d and %d", w, l, rep.Completion.Total.Failed, wantW, wantL)
			}
		})
	}
}

// Ordinary evidence is optional, so a defect of it takes that file alone (F5 of #435): not this host's, not this
// checkout's, not intact or beyond the denominator drops the file (status BLOCKED, QUERY_SCOPE_ORDINARY_REJECTED, the
// cause in ordinary.code) and leaves the predicate and SVC fold of both hosts untouched. A defect of the shared
// evidence (a stranger next to the closed set, the run identity) still refuses the host.
func TestMergeDropsAnOrdinaryFileThatIsNotTheHostsOwn(t *testing.T) {
	f := fixtureFor(t)
	good := func() map[string]string {
		d := twoHosts(t, f)
		d[windows] = withOrdinary(t, f, windows, ordinaryHostFile(t, f, windows, 97, 0, nil))
		d[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 97, 0, nil))
		return d
	}
	if rep := merge(root, expected(), good()); rep.Status != statusMerged {
		t.Fatalf("honest pair: %s %s", rep.Status, rep.Code)
	}
	const withoutOrdinary = 2 * (67 + 263 + 30 + 60)
	// dropped: the Linux file is dropped alone, the Windows host's own file stands, and the rest is folded as before
	dropped := func(name, code string, dirs map[string]string) {
		t.Helper()
		rep := merge(root, expected(), dirs)
		if rep.Status != statusBlocked || rep.Code != "QUERY_SCOPE_ORDINARY_REJECTED" || rep.Completion == nil || rep.Ordinary == nil || (code != "" && rep.Ordinary.Code != code) || rep.Ordinary.Code == "" {
			t.Errorf("%s: %s %s %+v", name, rep.Status, rep.Code, rep.Ordinary)
			return
		}
		if tot := rep.Completion.Total; tot.Verified != withoutOrdinary+97 || tot.Failed != 0 {
			t.Errorf("%s: total %+v", name, tot)
		}
		if w, l := ordinaryVerified(rep); w != 97 || l != 0 {
			t.Errorf("%s: ordinary verified windows %d linux %d, want 97 and 0", name, w, l)
		}
	}
	// the Windows result under the Linux name
	dirs := good()
	dirs[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, windows, 97, 0, nil))
	dropped("windows ordinary result under linux name", "QUERY_SCOPE_HOST_MISMATCH", dirs)
	// another scope's file under the ordinary name
	dirs = good()
	dirs[linux] = writeHost(t, f, linux, nil, func(rs map[string]kit.QueryScopeHostResult) { rs[ordinaryKey] = rs[kit.QueryScopePredicate] })
	dropped("predicate result under the ordinary name", "QUERY_SCOPE_HOST_MISMATCH", dirs)
	// a tampered count, digest and truncation of the stored file
	name := kit.QueryScopeHostFile(kit.QueryScopeOrdinary, linux)
	for label, change := range map[string]func(string) string{
		"count":     func(s string) string { return strings.Replace(s, `"verified":97`, `"verified":9000`, 1) },
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
		dropped(label, "", dirs)
	}
	// a file over the size limit
	dirs = good()
	if err := os.WriteFile(filepath.Join(dirs[linux], name), []byte(strings.Repeat(" ", maxHostBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	dropped("oversize", "QUERY_SCOPE_HOST_FILES", dirs)
	// evidence pinned to other expectation sources than this checkout's: the host check accepts the file, the fold
	// does not, and the same fold without it succeeds (the Linux file alone, so that the merge of the two hosts does
	// not refuse the difference first)
	pinned := func(r *kit.QueryScopeResult) {
		r.Assets = append([]queryregistrycore.Pin{}, r.Assets...)
		r.Assets[0].SHA256 = strings.Repeat("0", 64)
		*r = kit.NewQueryScopeResult(r.Scope, r.RegistrySHA256, r.Assets, r.Counters, r.FirstFailure)
	}
	dirs = twoHosts(t, f)
	dirs[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 97, 0, pinned))
	if rep := merge(root, expected(), dirs); rep.Status != statusBlocked || rep.Code != "QUERY_SCOPE_ORDINARY_REJECTED" || rep.Completion == nil || rep.Ordinary == nil || rep.Ordinary.Code != "QUERY_SCOPE_ASSET_PIN" ||
		rep.Completion.Total.Verified != withoutOrdinary || rep.Scopes[kit.QueryScopeOrdinary] != "" {
		t.Errorf("asset pin of another checkout: %s %s %+v", rep.Status, rep.Code, rep.Ordinary)
	}
	// a count beyond the registered denominator
	dirs = good()
	dirs[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 97, 0, func(r *kit.QueryScopeResult) {
		r.Counters[1].Required++
		r.Counters[1].Blocked++
		*r = kit.NewQueryScopeResult(r.Scope, r.RegistrySHA256, r.Assets, r.Counters, r.FirstFailure)
	}))
	if rep := merge(root, expected(), dirs); rep.Status != statusBlocked || rep.Code != "QUERY_SCOPE_ORDINARY_REJECTED" || rep.Completion == nil || rep.Ordinary == nil || rep.Ordinary.Code == "" {
		t.Errorf("denominator changed: %s %s %+v", rep.Status, rep.Code, rep.Ordinary)
	}
	// two files that do not merge: nothing of the ordinary scope is kept, the rest is folded
	dirs = good()
	dirs[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 97, 0, pinned))
	if rep := merge(root, expected(), dirs); rep.Status != statusBlocked || rep.Code != "QUERY_SCOPE_ORDINARY_REJECTED" || rep.Completion == nil || rep.Ordinary == nil || rep.Ordinary.Code == "" || rep.Completion.Total.Verified != withoutOrdinary {
		t.Errorf("files that do not merge: %s %s %+v", rep.Status, rep.Code, rep.Ordinary)
	}
	// a stranger next to the closed set is still refused
	dirs = good()
	if err := os.WriteFile(filepath.Join(dirs[linux], "query-scope-ordinary-extra.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Completion != nil || rep.Code != "QUERY_SCOPE_HOST_FILES" {
		t.Errorf("extra file: %s %s", rep.Status, rep.Code)
	}
}

// PR CI run 43408861497: PostgreSQL failed on both hosts and both JSON regressions failed on Linux, so the files
// hold 88 (2+14+72) on Windows and 72 (TSQL only) on Linux. The merge counts exactly that and reports both hosts
// short of the 97 the selection derives, instead of MERGED.
func TestMergeReportsWhatPassedOnEachHost(t *testing.T) {
	f := fixtureFor(t)
	dirs := twoHosts(t, f)
	dirs[windows] = withOrdinary(t, f, windows, ordinaryHostFile(t, f, windows, 88, 0, nil))
	dirs[linux] = withOrdinary(t, f, linux, ordinaryHostFile(t, f, linux, 72, 0, nil))
	rep := merge(root, expected(), dirs)
	if rep.Status != statusBlocked || rep.Code != "QUERY_SCOPE_ORDINARY_UNDERFILLED" || rep.Completion == nil || rep.Ordinary == nil {
		t.Fatalf("%s %s %+v", rep.Status, rep.Code, rep.Ordinary)
	}
	if w, l := ordinaryVerified(rep); w != 88 || l != 72 {
		t.Errorf("ordinary verified windows %d linux %d, want 88 and 72", w, l)
	}
	if u := rep.Ordinary.Unjudged; u[windows] != 9 || u[linux] != 25 || len(u) != 2 {
		t.Errorf("unjudged %v, want windows 9 and linux 25", u)
	}
}
