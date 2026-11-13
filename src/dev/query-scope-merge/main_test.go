package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

const root = "../../.."

const (
	windows = "windows-amd64"
	linux   = "linux-amd64"
)

func otherOf(host string) string {
	if host == windows {
		return linux
	}
	return windows
}

type fixture struct {
	registry, predAsset, svcAsset, svcInline []byte
	svc                                      kit.SvcExpected
	inventory                                []byte
}

var (
	fixOnce sync.Once
	fix     fixture
)

func fixtureFor(t *testing.T) *fixture {
	t.Helper()
	fixOnce.Do(func() {
		read := func(p string) []byte {
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
		fix.registry, fix.predAsset, fix.svcAsset, fix.svcInline = read(queryregistrycore.Path), read(kit.PredicateExpectedPath), read(kit.SvcExpectedPath), read(kit.SvcInlinePath)
		fix.inventory = read(queryregistrycore.QualificationPin.Path)
		svc, err := kit.ParseSvcExpected(fix.svcAsset, fix.registry, fix.inventory, read("src/testdata/native/n461/svc.json"))
		if err != nil {
			t.Fatal(err)
		}
		fix.svc = svc
	})
	return &fix
}

// hostResults are the scope results one host stores when it judges all its own obligations
// VERIFIED (the shape the native fan-in test stores), the other host stays BLOCKED. failSvc turns
// one VERIFIED SVC judgment into FAILED. The svc-inline entry is the svc result of the real-grammar
// run: the same judgments with 30 inline trees and 60 inline queries VERIFIED on top.
func (f *fixture) hostResults(t *testing.T, host string, failSvc bool) map[string]kit.QueryScopeHostResult {
	t.Helper()
	reg, err := queryregistrycore.Parse(f.registry)
	if err != nil {
		t.Fatal(err)
	}
	var judgments []queryregistrycore.Judgment
	for _, c := range queryregistrycore.Expand(nil, queryregistrycore.Platforms(), reg.Probes, nil) {
		if c.Platform != host {
			continue
		}
		j := queryregistrycore.Judgment{ID: c.ID(reg.Namespace), Coordinate: c, State: "VERIFIED"}
		if c.Kind == queryregistrycore.Control {
			for _, p := range reg.Probes {
				if p.ID == c.Case {
					j.Subject = &queryregistrycore.Subject{Status: p.Status, Code: p.Code, Evaluation: p.Evaluation, Partial: p.Partial, Structural: p.Structural, Expected: p.Expected}
				}
			}
		}
		judgments = append(judgments, j)
	}
	other := map[string]bool{otherOf(host): true}
	predicate, err := kit.PredicateScopeResult(f.registry, f.predAsset, judgments, other)
	if err != nil {
		t.Fatal(err)
	}
	svcJudgments, err := kit.VerifySvcExpected(f.svc, f.inventory)
	if err != nil {
		t.Fatal(err)
	}
	if failSvc {
		i := slices.IndexFunc(svcJudgments, func(j kit.SvcJudgment) bool { return j.State == "VERIFIED" })
		svcJudgments[i] = kit.SvcJudgment{ID: svcJudgments[i].ID, State: "FAILED", Code: "SVC_LINUX_ONLY"}
	}
	svc, err := kit.SvcScopeResult(f.svc, f.svcAsset, f.registry, host, svcJudgments, other)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]kit.QueryScopeHostResult{svcInlineKey: f.inlineResult(t, host, svcJudgments, nil)}
	for scope, r := range map[string]kit.QueryScopeResult{kit.QueryScopePredicate: predicate, kit.QueryScopeSvc: svc} {
		h, err := kit.NewQueryScopeHostResult(host, r)
		if err != nil {
			t.Fatal(err)
		}
		out[scope] = h
	}
	return out
}

// inlineResult is the host's svc result after the inline run: the first 30 inline tree and 60 inline
// query obligations are judged VERIFIED on top of the offline judgments (edit may then change them).
func (f *fixture) inlineResult(t *testing.T, host string, offline []kit.SvcJudgment, edit func([]kit.SvcJudgment)) kit.QueryScopeHostResult {
	t.Helper()
	js := slices.Clone(offline)
	trees, queries := 30, 60
	for i, o := range f.svc.Obligations() {
		switch {
		case o.Kind == kit.SvcKindInlineTree && trees > 0:
			js[i], trees = kit.SvcJudgment{ID: o.ID, State: "VERIFIED"}, trees-1
		case o.Kind == kit.SvcKindQuery && queries > 0:
			js[i], queries = kit.SvcJudgment{ID: o.ID, State: "VERIFIED"}, queries-1
		}
	}
	if edit != nil {
		edit(js)
	}
	r, err := kit.SvcScopeResult(f.svc, f.svcAsset, f.registry, host, js, map[string]bool{otherOf(host): true})
	if err != nil {
		t.Fatal(err)
	}
	pins := append(slices.Clone(r.Assets), queryregistrycore.FilePin(kit.SvcInlinePath, f.svcInline))
	h, err := kit.NewQueryScopeHostResult(host, kit.NewQueryScopeResult(r.Scope, r.RegistrySHA256, pins, r.Counters, r.FirstFailure))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func runIdentity(host string) kit.RunIdentity {
	pair := hostPairs[host]
	return kit.RunIdentity{Schema: kit.RunIdentitySchema, Repository: "o/r", Workflow: "Foundation", RunID: "100", RunAttempt: "1", Event: "pull_request",
		SHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), Checkout: strings.Repeat("a", 40), Job: producerJob, RunnerOS: "X", RunnerArch: "X64",
		Image: "i/1", GOOS: pair[0], GOARCH: pair[1], GoVersion: "go1.27.4", EvidenceMode: "NEW_RUN"}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeHost stores a host artifact directory as the CI producer step does. The identity and the
// results may be altered before they are written.
func writeHost(t *testing.T, f *fixture, host string, id func(*kit.RunIdentity), results func(map[string]kit.QueryScopeHostResult)) string {
	t.Helper()
	dir := t.TempDir()
	identity := runIdentity(host)
	if id != nil {
		id(&identity)
	}
	writeJSON(t, filepath.Join(dir, identityFile), identity)
	rs := f.hostResults(t, host, false)
	if results != nil {
		results(rs)
	}
	for scope, h := range rs {
		data, err := kit.EncodeQueryScopeHostResult(h)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, fileOf(scope, host)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func expected() kit.RunIdentity {
	id := runIdentity(windows)
	id.Job, id.GOOS, id.GOARCH = "query-scope-merge", "linux", "amd64"
	return id
}

func twoHosts(t *testing.T, f *fixture) map[string]string {
	return map[string]string{windows: writeHost(t, f, windows, nil, nil), linux: writeHost(t, f, linux, nil, nil)}
}

func TestMergeBothHosts(t *testing.T) {
	f := fixtureFor(t)
	rep := merge(root, expected(), twoHosts(t, f))
	if rep.Status != statusMerged || rep.Code != "QUERY_SCOPE_RESULT_ABSENT" || rep.Completion == nil {
		t.Fatalf("%s %s", rep.Status, rep.Code)
	}
	// predicate 2 x 67, SVC offline 2 x 263 and the inline 2 x (30 trees + 60 queries), computed from
	// the registered denominators; the inline rows are rows of the svc scope, not an extra scope
	if tot := rep.Completion.Total; tot.Verified != 2*(67+263+30+60) || tot.Failed != 0 || rep.Completion.Complete {
		t.Fatalf("total %+v", tot)
	}
	for _, h := range rep.Completion.Hosts {
		if h.Verified != 420 {
			t.Errorf("host %s verified %d, want 420", h.Platform, h.Verified)
		}
	}
	inline := map[string]uint64{}
	for _, c := range rep.Completion.Counters {
		if c.Scope == kit.QueryScopeSvc && c.Platform == windows && (c.Kind == kit.SvcKindInlineTree || c.Kind == kit.SvcKindQuery) {
			inline[c.Kind] = c.Verified
			if c.Verified+c.Blocked != c.Required {
				t.Errorf("%s: %+v", c.Kind, c.QueryCompletionCounts)
			}
		}
	}
	if inline[kit.SvcKindInlineTree] != 30 || inline[kit.SvcKindQuery] != 60 {
		t.Errorf("inline rows %v, want 30 trees and 60 queries VERIFIED", inline)
	}
	if len(rep.Inputs) != 2 || len(rep.Inputs[0].Files) != 4 || len(rep.Scopes) != 2 {
		t.Fatalf("inputs %+v scopes %v", rep.Inputs, rep.Scopes)
	}
}

// The inline file is the host's svc result: it must agree with the offline svc file everywhere but
// that host's inline rows, and its inline asset pin must be the checkout's.
func TestMergeRefusesAnInlineResultThatDisagreesWithTheOfflineOne(t *testing.T) {
	f := fixtureFor(t)
	offline, err := kit.VerifySvcExpected(f.svc, f.inventory)
	if err != nil {
		t.Fatal(err)
	}
	foreignPin := func(h kit.QueryScopeHostResult) kit.QueryScopeHostResult {
		r := h.Result
		pins := slices.Clone(r.Assets)
		pins[len(pins)-1] = queryregistrycore.FilePin(kit.SvcInlinePath, []byte("another asset"))
		sealed, err := kit.NewQueryScopeHostResult(h.Host, kit.NewQueryScopeResult(r.Scope, r.RegistrySHA256, pins, r.Counters, r.FirstFailure))
		if err != nil {
			t.Fatal(err)
		}
		return sealed
	}
	for name, c := range map[string]struct {
		inline func(rs map[string]kit.QueryScopeHostResult) kit.QueryScopeHostResult
		code   string
	}{
		"offline obligation judged differently": {func(map[string]kit.QueryScopeHostResult) kit.QueryScopeHostResult {
			return f.inlineResult(t, linux, offline, func(js []kit.SvcJudgment) {
				i := slices.IndexFunc(js, func(j kit.SvcJudgment) bool { return j.State == "VERIFIED" })
				js[i] = kit.SvcJudgment{ID: js[i].ID, State: "BLOCKED", Code: "SVC_ONLY_HERE"}
			})
		}, "QUERY_SCOPE_SVC_INLINE_MISMATCH"},
		"offline result in the inline file (no inline pin)": {func(rs map[string]kit.QueryScopeHostResult) kit.QueryScopeHostResult {
			return rs[kit.QueryScopeSvc]
		}, "QUERY_SCOPE_SVC_INLINE_MISMATCH"},
		// the two hosts then disagree on the pinned assets
		"inline asset pin of another asset on one host": {func(rs map[string]kit.QueryScopeHostResult) kit.QueryScopeHostResult {
			return foreignPin(rs[svcInlineKey])
		}, "QUERY_SCOPE_HOST_MISMATCH"},
		"the other host's rows claimed": {func(map[string]kit.QueryScopeHostResult) kit.QueryScopeHostResult {
			return f.inlineResult(t, windows, offline, nil) // a Windows result under the Linux name
		}, "QUERY_SCOPE_HOST_MISMATCH"},
	} {
		dirs := twoHosts(t, f)
		dirs[linux] = writeHost(t, f, linux, nil, func(rs map[string]kit.QueryScopeHostResult) { rs[svcInlineKey] = c.inline(rs) })
		if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != c.code || rep.Completion != nil {
			t.Errorf("%s: %s %s", name, rep.Status, rep.Code)
		}
	}
	// both hosts agree on an inline asset pin that is not the checkout's: the fold refuses it
	dirs := twoHosts(t, f)
	for _, host := range []string{windows, linux} {
		dirs[host] = writeHost(t, f, host, nil, func(rs map[string]kit.QueryScopeHostResult) { rs[svcInlineKey] = foreignPin(rs[svcInlineKey]) })
	}
	if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_ASSET_PIN" || rep.Completion != nil {
		t.Errorf("foreign inline pin on both hosts: %s %s", rep.Status, rep.Code)
	}
}

func TestMergeKeepsAHostFailure(t *testing.T) {
	f := fixtureFor(t)
	dirs := twoHosts(t, f)
	dirs[linux] = writeHost(t, f, linux, nil, func(rs map[string]kit.QueryScopeHostResult) {
		failed := f.hostResults(t, linux, true)
		rs[kit.QueryScopeSvc], rs[svcInlineKey] = failed[kit.QueryScopeSvc], failed[svcInlineKey]
	})
	rep := merge(root, expected(), dirs)
	if rep.Status != statusFail || rep.Completion == nil || rep.Completion.Total.Failed != 1 {
		t.Fatalf("%s %s %+v", rep.Status, rep.Code, rep.Completion)
	}
}

func TestMergeBlocksWhenAHostIsAbsent(t *testing.T) {
	f := fixtureFor(t)
	both := twoHosts(t, f)
	for name, dirs := range map[string]map[string]string{
		"no-linux-flag":   {windows: both[windows]},
		"no-windows-flag": {linux: both[linux]},
		"no-hosts":        {},
		"missing-dir":     {windows: both[windows], linux: filepath.Join(t.TempDir(), "absent")},
	} {
		rep := merge(root, expected(), dirs)
		if rep.Status != statusBlocked || rep.Code != "QUERY_SCOPE_HOST_ABSENT" || rep.Completion != nil {
			t.Errorf("%s: %s %s", name, rep.Status, rep.Code)
		}
	}
}

func TestMergeRefusesAnotherRunsArtifact(t *testing.T) {
	f := fixtureFor(t)
	for name, change := range map[string]func(*kit.RunIdentity){
		"run":        func(id *kit.RunIdentity) { id.RunID = "99" },
		"attempt":    func(id *kit.RunIdentity) { id.RunAttempt = "2" },
		"sha":        func(id *kit.RunIdentity) { id.SHA = strings.Repeat("c", 40) },
		"head":       func(id *kit.RunIdentity) { id.HeadSHA = strings.Repeat("c", 40) },
		"checkout":   func(id *kit.RunIdentity) { id.Checkout = strings.Repeat("c", 40) },
		"repository": func(id *kit.RunIdentity) { id.Repository = "o/other" },
		"event":      func(id *kit.RunIdentity) { id.Event = "push" },
		"job":        func(id *kit.RunIdentity) { id.Job = "foundation" },
		"mode":       func(id *kit.RunIdentity) { id.EvidenceMode = "REPLAY" },
		"os":         func(id *kit.RunIdentity) { id.GOOS = "linux" },
	} {
		dirs := twoHosts(t, f)
		dirs[windows] = writeHost(t, f, windows, change, nil)
		if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_RUN_MISMATCH" {
			t.Errorf("%s: %s %s", name, rep.Status, rep.Code)
		}
	}
	// a directory with no readable identity, and an identity with an unknown member
	dirs := twoHosts(t, f)
	if err := os.Remove(filepath.Join(dirs[linux], identityFile)); err != nil {
		t.Fatal(err)
	}
	if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_HOST_FILES" {
		t.Errorf("no identity: %s %s", rep.Status, rep.Code)
	}
	dirs = twoHosts(t, f)
	if err := os.WriteFile(filepath.Join(dirs[linux], identityFile), []byte(`{"schema":"tsgk-run-identity/r1","extra":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_RUN_IDENTITY" {
		t.Errorf("unknown member: %s %s", rep.Status, rep.Code)
	}
	// the run this tool executes in must itself be a NEW_RUN with its identity
	bad := expected()
	bad.RunID = ""
	if rep := merge(root, bad, twoHosts(t, f)); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_RUN_EXPECTED" {
		t.Errorf("expected identity: %s %s", rep.Status, rep.Code)
	}
}

func TestMergeRefusesTamperedAndMisplacedFiles(t *testing.T) {
	f := fixtureFor(t)
	edit := func(host, name string, change func(string) string) map[string]string {
		dirs := twoHosts(t, f)
		path := filepath.Join(dirs[host], name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(change(string(data))), 0o644); err != nil {
			t.Fatal(err)
		}
		return dirs
	}
	svcFile := kit.QueryScopeHostFile(kit.QueryScopeSvc, linux)
	for name, dirs := range map[string]map[string]string{
		"count":      edit(linux, svcFile, func(s string) string { return strings.Replace(s, `"verified":`, `"verified":9`, 1) }),
		"digest":     edit(linux, svcFile, func(s string) string { return strings.Replace(s, `"host_sha256":"`, `"host_sha256":"0`, 1) }),
		"truncated":  edit(linux, svcFile, func(s string) string { return s[:len(s)/2] }),
		"empty":      edit(linux, svcFile, func(string) string { return "" }),
		"host-field": edit(linux, svcFile, func(s string) string { return strings.Replace(s, `"host":"linux-amd64"`, `"host":"windows-amd64"`, 1) }),
	} {
		if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Completion != nil {
			t.Errorf("%s: %s %s", name, rep.Status, rep.Code)
		}
	}
	// files outside the closed set
	dirs := twoHosts(t, f)
	if err := os.WriteFile(filepath.Join(dirs[linux], "extra.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rep := merge(root, expected(), dirs); rep.Code != "QUERY_SCOPE_HOST_FILES" {
		t.Errorf("extra file: %s", rep.Code)
	}
	dirs = twoHosts(t, f)
	if err := os.Remove(filepath.Join(dirs[linux], svcFile)); err != nil {
		t.Fatal(err)
	}
	if rep := merge(root, expected(), dirs); rep.Code != "QUERY_SCOPE_HOST_FILES" {
		t.Errorf("missing file: %s", rep.Code)
	}
	// duplicate evidence: the Windows results copied under Linux names with a Linux identity
	// (every directory check passes; only the host inside the files betrays it)
	dirs = twoHosts(t, f)
	dirs[linux] = writeHost(t, f, linux, nil, func(rs map[string]kit.QueryScopeHostResult) {
		for scope, h := range f.hostResults(t, windows, false) {
			rs[scope] = h
		}
	})
	rep := merge(root, expected(), dirs)
	if rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_HOST_MISMATCH" {
		t.Errorf("windows results under linux names: %s %s", rep.Status, rep.Code)
	}
	// a scope file of the other scope under a scope's name
	dirs = twoHosts(t, f)
	dirs[linux] = writeHost(t, f, linux, nil, func(rs map[string]kit.QueryScopeHostResult) {
		rs[kit.QueryScopePredicate], rs[kit.QueryScopeSvc] = rs[kit.QueryScopeSvc], rs[kit.QueryScopePredicate]
	})
	if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_HOST_MISMATCH" {
		t.Errorf("swapped scopes: %s %s", rep.Status, rep.Code)
	}
}

func TestHostFlagsRefuseDuplicatesAndUnknownHosts(t *testing.T) {
	h := hostFlags{}
	for _, v := range []string{windows + "=a", linux + "=b"} {
		if err := h.Set(v); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []string{windows + "=c", "darwin-arm64=d", "linux-amd64", linux + "="} {
		if err := h.Set(v); err == nil {
			t.Errorf("accepted %q", v)
		}
	}
	if h[windows] != "a" || h[linux] != "b" {
		t.Fatalf("%v", h)
	}
}
