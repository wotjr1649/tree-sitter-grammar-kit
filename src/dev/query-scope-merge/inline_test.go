package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/scopeobligations"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The merge checks the inline judgments against the committed inline expected asset (#362): the offline run
// has no C# grammar, so the offline file leaves the inline kinds BLOCKED with their registered codes, and the
// inline run may judge only the obligations the asset derives. A host that judged less than it could is
// reported as such, not as a complete MERGED.
func TestMergeChecksTheInlineJudgmentsAgainstTheDerivableSet(t *testing.T) {
	f := fixtureFor(t)
	offline, err := kit.VerifySvcExpected(f.svc, f.inventory)
	if err != nil {
		t.Fatal(err)
	}
	obligations := f.svc.Obligations()
	blockedTree := -1 // an inline tree the asset does not derive
	for i, o := range obligations {
		if o.Kind == kit.SvcKindInlineTree && f.codes[o.ID] != "" {
			blockedTree = i
			break
		}
	}
	if blockedTree < 0 {
		t.Fatal("the asset derives every inline tree")
	}
	verified := func(i int) kit.SvcJudgment { return kit.SvcJudgment{ID: obligations[i].ID, State: "VERIFIED"} }
	claimsAllInline := func(js []kit.SvcJudgment) {
		for i, o := range obligations {
			if o.Kind == kit.SvcKindInlineTree || o.Kind == kit.SvcKindQuery {
				js[i] = verified(i)
			}
		}
	}
	forgedOffline := slices.Clone(offline)
	claimsAllInline(forgedOffline)
	for name, c := range map[string]func(rs map[string]kit.QueryScopeHostResult){
		// every inline tree (82) and query (164) VERIFIED, not the 30 + 60 the asset derives, with a matching companion
		"overclaim": func(rs map[string]kit.QueryScopeHostResult) {
			rs[svcInlineKey] = f.inlineResult(t, linux, offline, claimsAllInline)
		},
		// the offline file claims the inline kinds VERIFIED: it has no grammar
		"offline claims the inline kinds": func(rs map[string]kit.QueryScopeHostResult) {
			rs[kit.QueryScopeSvc] = svcHostResult(t, f, linux, forgedOffline)
			rs[svcInlineKey] = f.inlineResult(t, linux, forgedOffline, nil)
		},
		"a blocked tree judged VERIFIED": func(rs map[string]kit.QueryScopeHostResult) {
			rs[svcInlineKey] = f.inlineResult(t, linux, offline, func(js []kit.SvcJudgment) { js[blockedTree] = verified(blockedTree) })
		},
		"a blocked tree judged FAILED": func(rs map[string]kit.QueryScopeHostResult) {
			rs[svcInlineKey] = f.inlineResult(t, linux, offline, func(js []kit.SvcJudgment) {
				js[blockedTree] = kit.SvcJudgment{ID: obligations[blockedTree].ID, State: "FAILED", Code: "SVC_INLINE_NATIVE_TREE"}
			})
		},
	} {
		dirs := twoHosts(t, f)
		dirs[linux] = writeHost(t, f, linux, nil, c)
		if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != scopeobligations.CodeInline || rep.Completion != nil {
			t.Errorf("%s: %s %s", name, rep.Status, rep.Code)
		}
	}
	// the inline run judged less than the asset derives: folded and sound, but not a complete merge
	notRun := func(i int) kit.SvcJudgment {
		return kit.SvcJudgment{ID: obligations[i].ID, State: "BLOCKED", Code: kit.SvcInlineNativeNotRunCode}
	}
	for name, c := range map[string]struct {
		edit     func(js []kit.SvcJudgment)
		unjudged int
	}{
		"nothing judged":   {func(js []kit.SvcJudgment) { copy(js, offline) }, len(f.trees) + len(f.queries)},
		"one tree not run": {func(js []kit.SvcJudgment) { js[f.trees[0]] = notRun(f.trees[0]) }, 1},
	} {
		dirs := twoHosts(t, f)
		dirs[linux] = writeHost(t, f, linux, nil, func(rs map[string]kit.QueryScopeHostResult) {
			rs[svcInlineKey] = f.inlineResult(t, linux, offline, c.edit)
		})
		rep := merge(root, expected(), dirs)
		if rep.Status != statusBlocked || rep.Code != "QUERY_SCOPE_SVC_INLINE_UNDERFILLED" || rep.Completion == nil {
			t.Errorf("%s: %s %s", name, rep.Status, rep.Code)
			continue
		}
		if want := uint64(2*(67+263+len(f.trees)+len(f.queries)) - c.unjudged); rep.Completion.Total.Verified != want {
			t.Errorf("%s: verified %d, want %d", name, rep.Completion.Total.Verified, want)
		}
		for _, in := range rep.Inputs {
			if want := map[string]int{windows: 0, linux: c.unjudged}[in.Host]; in.InlineUnjudged != want {
				t.Errorf("%s: host %s unjudged %d, want %d", name, in.Host, in.InlineUnjudged, want)
			}
		}
	}
	// a FAILED judgment is still reported as FAIL, also with obligations left unjudged
	dirs := twoHosts(t, f)
	dirs[linux] = writeHost(t, f, linux, nil, func(rs map[string]kit.QueryScopeHostResult) {
		rs[svcInlineKey] = f.inlineResult(t, linux, offline, func(js []kit.SvcJudgment) {
			js[f.trees[0]] = kit.SvcJudgment{ID: obligations[f.trees[0]].ID, State: "FAILED", Code: "SVC_INLINE_NATIVE_TREE"}
			js[f.trees[1]] = notRun(f.trees[1])
		})
	})
	if rep := merge(root, expected(), dirs); rep.Status != statusFail {
		t.Errorf("failed and unjudged: %s %s", rep.Status, rep.Code)
	}
}

// The offline result is recomputed from the companion on its own: a stored offline result that claims an
// inline tree VERIFIED while the companion's offline list (and the inline result) are the honest ones.
func TestMergeRecomputesTheOfflineResultFromTheCompanion(t *testing.T) {
	f := fixtureFor(t)
	offline, err := kit.VerifySvcExpected(f.svc, f.inventory)
	if err != nil {
		t.Fatal(err)
	}
	stored := slices.Clone(offline)
	stored[f.trees[0]] = kit.SvcJudgment{ID: f.svc.Obligations()[f.trees[0]].ID, State: "VERIFIED"}
	honest := scopeobligations.New(linux, offline, offline).Offline
	dirs := twoHosts(t, f)
	dirs[linux] = writeHostWith(t, f, linux, nil, func(rs map[string]kit.QueryScopeHostResult) {
		rs[kit.QueryScopeSvc] = svcHostResult(t, f, linux, stored)
	}, func(d *scopeobligations.Document) { d.Offline = honest })
	if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != scopeobligations.CodeShape {
		t.Errorf("%s %s", rep.Status, rep.Code)
	}
}

// refinesOffline compares the two stored results alone; each condition has a case of its own.
func TestRefinesOfflineConditions(t *testing.T) {
	f := fixtureFor(t)
	rs := f.hostResults(t, linux, false)
	off, in := rs[kit.QueryScopeSvc], rs[svcInlineKey]
	if !refinesOffline(off, in) {
		t.Fatal("the honest pair does not refine")
	}
	i := slices.IndexFunc(off.Result.Counters, func(c kit.QueryScopeCounter) bool { return c.Platform == linux && c.Kind == kit.SvcKindInlineTree })
	// the offline run recorded a FAILED on an inline row; the inline result has none there
	failedOff := off
	failedOff.Result.Counters = slices.Clone(off.Result.Counters)
	failedOff.Result.Counters[i].Failed, failedOff.Result.Counters[i].Blocked = 1, off.Result.Counters[i].Blocked-1
	// the offline pins are not a prefix of the inline pins
	pinned := in
	pinned.Result.Assets = slices.Clone(in.Result.Assets)
	pinned.Result.Assets[0] = queryregistrycore.FilePin(kit.SvcExpectedPath, []byte("another asset"))
	for name, c := range map[string]struct{ off, in kit.QueryScopeHostResult }{
		"offline FAILED on an inline row taken back":   {failedOff, in},
		"offline pins not a prefix of the inline pins": {off, pinned},
	} {
		if refinesOffline(c.off, c.in) {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Only regular files make a host directory: a symbolic link under an expected name is refused although it
// points at the right bytes.
func TestMergeRefusesALinkedHostFile(t *testing.T) {
	f := fixtureFor(t)
	dirs := twoHosts(t, f)
	name := kit.QueryScopeHostFile(kit.QueryScopeSvc, linux)
	path := filepath.Join(dirs[linux], name)
	target := filepath.Join(t.TempDir(), name)
	if err := os.Rename(path, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("no symbolic link here: %v", err)
	}
	if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_HOST_FILES" {
		t.Errorf("%s %s", rep.Status, rep.Code)
	}
}

// An unreadable host directory (here: a file, not a directory) is refused; only a missing one is absent.
func TestMergeRefusesAHostPathThatIsNotADirectory(t *testing.T) {
	f := fixtureFor(t)
	dirs := twoHosts(t, f)
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadDir(file); errors.Is(err, fs.ErrNotExist) {
		t.Skip("this platform reports a file read as a directory as not existing")
	}
	dirs[linux] = file
	if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_HOST_FILES" {
		t.Errorf("%s %s", rep.Status, rep.Code)
	}
}
