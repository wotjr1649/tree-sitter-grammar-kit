package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/scopeobligations"
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
	rec                                      map[string][]kit.SvcJudgment // svc judgments by the result they were folded into
	codes                                    map[string]string            // the inline asset's per-obligation codes ("" = derivable)
	trees, queries                           []int                        // obligation indexes of the derivable inline trees and queries
}

// record remembers the judgments a host's svc result was built from; writeHost stores them as the
// per-obligation companion of that result.
func (f *fixture) record(h kit.QueryScopeHostResult, js []kit.SvcJudgment) kit.QueryScopeHostResult {
	f.rec[h.Result.ResultSHA256] = slices.Clone(js)
	return h
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
		x, err := kit.ParseSvcInlineExpected(fix.svcInline, fix.svcAsset, fix.registry, fix.inventory, read("src/testdata/native/n461/svc.json"), read("src/contracts/fact-query-pack.json"))
		if err != nil {
			t.Fatal(err)
		}
		fix.codes = x.InlineCodes()
		for i, o := range svc.Obligations() {
			switch {
			case fix.codes[o.ID] != "":
			case o.Kind == kit.SvcKindInlineTree:
				fix.trees = append(fix.trees, i)
			case o.Kind == kit.SvcKindQuery:
				fix.queries = append(fix.queries, i)
			}
		}
		fix.rec = map[string][]kit.SvcJudgment{}
	})
	return &fix
}

// hostResults are the scope results one host stores when it judges all its own obligations
// VERIFIED (the shape the native fan-in test stores), the other host stays BLOCKED. failSvc turns
// one VERIFIED SVC judgment into FAILED. The svc-inline entry is the svc result of the real-grammar
// run: the same judgments with 30 inline trees and 60 inline queries VERIFIED on top.
func (f *fixture) hostResults(t *testing.T, host string, failSvc bool) map[string]kit.QueryScopeHostResult {
	t.Helper()
	hostMu.Lock()
	defer hostMu.Unlock()
	k := hostKey{host, failSvc}
	if _, ok := hostCache[k]; !ok {
		hostCache[k] = f.computeHostResults(t, host, failSvc)
	}
	return maps.Clone(hostCache[k]) // a test edits its copy of the map, never the shared one
}

// hostKey and hostCache: the results of one host are computed once (predicate fold, SVC verification and
// folds are the slow part of every test); a variant is made by editing a copy.
type hostKey struct {
	host    string
	failSvc bool
}

var (
	hostMu    sync.Mutex
	hostCache = map[hostKey]map[string]kit.QueryScopeHostResult{}
)

func (f *fixture) computeHostResults(t *testing.T, host string, failSvc bool) map[string]kit.QueryScopeHostResult {
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
		if scope == kit.QueryScopeSvc {
			f.record(h, svcJudgments)
		}
	}
	return out
}

// inlineResult is the host's svc result after the inline run: the derivable inline tree (30) and query (60)
// obligations of the committed asset are judged VERIFIED on top of the offline judgments (edit may then
// change them).
func (f *fixture) inlineResult(t *testing.T, host string, offline []kit.SvcJudgment, edit func([]kit.SvcJudgment)) kit.QueryScopeHostResult {
	t.Helper()
	js := slices.Clone(offline)
	for _, i := range append(slices.Clone(f.trees), f.queries...) {
		js[i] = kit.SvcJudgment{ID: f.svc.Obligations()[i].ID, State: "VERIFIED"}
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
	return f.record(h, js)
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
	return writeHostWith(t, f, host, id, results, nil)
}

// writeHostWith also lets a test alter the per-obligation companion before it is written. The companion
// holds the judgments the two svc results were built from (the standard ones for a result a test made
// by editing counters).
func writeHostWith(t *testing.T, f *fixture, host string, id func(*kit.RunIdentity), results func(map[string]kit.QueryScopeHostResult), companion func(*scopeobligations.Document)) string {
	t.Helper()
	dir := t.TempDir()
	writeHostTo(t, f, dir, host, id, results, companion)
	return dir
}

func writeHostTo(t *testing.T, f *fixture, dir, host string, id func(*kit.RunIdentity), results func(map[string]kit.QueryScopeHostResult), companion func(*scopeobligations.Document)) {
	t.Helper()
	identity := runIdentity(host)
	if id != nil {
		id(&identity)
	}
	writeJSON(t, filepath.Join(dir, identityFile), identity)
	rs := f.hostResults(t, host, false)
	standard := [2][]kit.SvcJudgment{f.rec[rs[kit.QueryScopeSvc].Result.ResultSHA256], f.rec[rs[svcInlineKey].Result.ResultSHA256]}
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
	offline, ok := f.rec[rs[kit.QueryScopeSvc].Result.ResultSHA256]
	if !ok {
		offline = standard[0]
	}
	inline, ok := f.rec[rs[svcInlineKey].Result.ResultSHA256]
	if !ok {
		inline = standard[1]
	}
	doc := scopeobligations.New(host, offline, inline)
	if companion != nil {
		companion(&doc)
	}
	data, err := scopeobligations.Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, scopeobligations.FileName(host)), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func expected() kit.RunIdentity {
	id := runIdentity(windows)
	id.Job, id.GOOS, id.GOARCH = "query-scope-merge", "linux", "amd64"
	return id
}

var (
	baseOnce sync.Once
	baseDir  string
)

// twoHosts is a fresh copy of the two honest host directories, which are written once per test binary.
func twoHosts(t *testing.T, f *fixture) map[string]string {
	t.Helper()
	baseOnce.Do(func() {
		dir, err := os.MkdirTemp("", "query-scope-merge-base-")
		if err != nil {
			t.Fatal(err)
		}
		baseDir = dir
		for _, host := range []string{windows, linux} {
			if err := os.Mkdir(filepath.Join(dir, host), 0o755); err != nil {
				t.Fatal(err)
			}
			writeHostTo(t, f, filepath.Join(dir, host), host, nil, nil, nil)
		}
	})
	dirs := map[string]string{}
	for _, host := range []string{windows, linux} {
		dirs[host] = filepath.Join(t.TempDir(), host)
		if err := os.CopyFS(dirs[host], os.DirFS(filepath.Join(baseDir, host))); err != nil {
			t.Fatal(err)
		}
	}
	return dirs
}

func TestMain(m *testing.M) {
	code := m.Run()
	if baseDir != "" {
		os.RemoveAll(baseDir)
	}
	os.Exit(code)
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
	if len(rep.Inputs) != 2 || len(rep.Inputs[0].Files) != 5 || len(rep.Scopes) != 2 {
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
		"inline asset pin of another asset on one host": {func(rs map[string]kit.QueryScopeHostResult) kit.QueryScopeHostResult {
			return foreignPin(rs[svcInlineKey])
		}, "QUERY_SCOPE_SVC_OBLIGATIONS"},
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
	// both hosts agree on an inline asset pin that is not the checkout's: the recomputation from the
	// per-obligation companion (which pins the checkout's inline asset) refuses it
	dirs := twoHosts(t, f)
	for _, host := range []string{windows, linux} {
		dirs[host] = writeHost(t, f, host, nil, func(rs map[string]kit.QueryScopeHostResult) { rs[svcInlineKey] = foreignPin(rs[svcInlineKey]) })
	}
	if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_SVC_OBLIGATIONS" || rep.Completion != nil {
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
		"arch":       func(id *kit.RunIdentity) { id.GOARCH = "arm64" },
		"workflow":   func(id *kit.RunIdentity) { id.Workflow = "Other" },
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

// svcHostResult seals a host's svc result of the given judgments (the offline file when the judgments
// leave the inline obligations BLOCKED).
func svcHostResult(t *testing.T, f *fixture, host string, js []kit.SvcJudgment) kit.QueryScopeHostResult {
	t.Helper()
	r, err := kit.SvcScopeResult(f.svc, f.svcAsset, f.registry, host, js, map[string]bool{otherOf(host): true})
	if err != nil {
		t.Fatal(err)
	}
	h, err := kit.NewQueryScopeHostResult(host, r)
	if err != nil {
		t.Fatal(err)
	}
	return f.record(h, js)
}

// The inline file only refines the offline one (#362): it moves inline obligations out of BLOCKED and
// never takes an offline VERIFIED or FAILED back, so a FAILED the offline run recorded on one of this
// host's inline rows cannot disappear behind a later all-VERIFIED inline run.
func TestMergeRefusesAnInlineResultThatTakesBackAnOfflineJudgment(t *testing.T) {
	f := fixtureFor(t)
	offline, err := kit.VerifySvcExpected(f.svc, f.inventory)
	if err != nil {
		t.Fatal(err)
	}
	with := func(i int, j kit.SvcJudgment) []kit.SvcJudgment {
		js := slices.Clone(offline)
		js[i] = j
		return js
	}
	tree, query := f.trees[0], f.queries[0]
	allTrees := slices.Clone(offline)
	for _, i := range f.trees {
		allTrees[i] = kit.SvcJudgment{ID: f.svc.Obligations()[i].ID, State: "VERIFIED"}
	}
	id := func(i int) string { return f.svc.Obligations()[i].ID }
	for name, c := range map[string]struct {
		offline []kit.SvcJudgment
		inline  func(js []kit.SvcJudgment)
	}{
		// the inline run judges everything VERIFIED on top: the offline FAILED would be erased
		"offline FAILED inline tree row":  {with(tree, kit.SvcJudgment{ID: id(tree), State: "FAILED", Code: "SVC_OFFLINE_INLINE_FAIL"}), nil},
		"offline FAILED inline query row": {with(query, kit.SvcJudgment{ID: id(query), State: "FAILED", Code: "SVC_OFFLINE_INLINE_FAIL"}), nil},
		// an offline VERIFIED inline row turned back into BLOCKED (rows are counted per kind: the offline file
		// holds all 30 inline trees VERIFIED, so one taken back shows as a lower count)
		"offline VERIFIED inline row blocked again": {allTrees, func(js []kit.SvcJudgment) {
			js[tree] = kit.SvcJudgment{ID: id(tree), State: "BLOCKED", Code: "SVC_BLOCKED_AGAIN"}
		}},
	} {
		dirs := twoHosts(t, f)
		dirs[windows] = writeHost(t, f, windows, nil, func(rs map[string]kit.QueryScopeHostResult) {
			rs[kit.QueryScopeSvc] = svcHostResult(t, f, windows, c.offline)
			rs[svcInlineKey] = f.inlineResult(t, windows, c.offline, c.inline)
		})
		if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_SVC_INLINE_MISMATCH" || rep.Completion != nil {
			t.Errorf("%s: %s %s", name, rep.Status, rep.Code)
		}
	}
	// an inline row may not move an obligation to UNPROCESSED either (the sums still hold)
	dirs := twoHosts(t, f)
	dirs[windows] = writeHost(t, f, windows, nil, func(rs map[string]kit.QueryScopeHostResult) {
		r := rs[svcInlineKey].Result
		rows := slices.Clone(r.Counters)
		for i, c := range rows {
			if c.Platform == windows && c.Kind == kit.SvcKindInlineTree {
				rows[i].Blocked, rows[i].Unprocessed = c.Blocked-1, c.Unprocessed+1
				break
			}
		}
		h, err := kit.NewQueryScopeHostResult(windows, kit.NewQueryScopeResult(r.Scope, r.RegistrySHA256, r.Assets, rows, r.FirstFailure))
		if err != nil {
			t.Fatal(err)
		}
		rs[svcInlineKey] = h
	})
	if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_SVC_INLINE_MISMATCH" {
		t.Errorf("inline obligation moved to UNPROCESSED: %s %s", rep.Status, rep.Code)
	}
	// the refinement itself still merges: the offline file leaves the inline rows BLOCKED
	if rep := merge(root, expected(), twoHosts(t, f)); rep.Status != statusMerged {
		t.Errorf("refinement refused: %s %s", rep.Status, rep.Code)
	}
}

// The identity of the run this tool executes in names every field a host identity is compared with, and
// an identity file is decoded strictly (#362).
func TestMergeRefusesIncompleteExpectedAndLooseIdentities(t *testing.T) {
	f := fixtureFor(t)
	for name, blank := range map[string]func(*kit.RunIdentity){
		"repository": func(id *kit.RunIdentity) { id.Repository = "" },
		"workflow":   func(id *kit.RunIdentity) { id.Workflow = "" },
		"event":      func(id *kit.RunIdentity) { id.Event = "" },
		"checkout":   func(id *kit.RunIdentity) { id.Checkout = "" },
		"job":        func(id *kit.RunIdentity) { id.Job = "" },
		"goos":       func(id *kit.RunIdentity) { id.GOOS = "" },
		"head":       func(id *kit.RunIdentity) { id.HeadSHA = " " },
		"goarch":     func(id *kit.RunIdentity) { id.GOARCH = "" },
		"mode":       func(id *kit.RunIdentity) { id.EvidenceMode = "REPLAY" },
	} {
		// the hosts carry the same blank value: equal blanks must not count as a binding
		dirs := map[string]string{windows: writeHost(t, f, windows, blank, nil), linux: writeHost(t, f, linux, blank, nil)}
		e := expected()
		blank(&e)
		if name == "job" || name == "goos" {
			dirs = twoHosts(t, f)
		}
		if rep := merge(root, e, dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_RUN_EXPECTED" {
			t.Errorf("blank expected %s: %s %s", name, rep.Status, rep.Code)
		}
	}
	for name, change := range map[string]func(string) string{
		"upper-case member name": func(s string) string { return strings.Replace(s, `"run_id"`, `"RUN_ID"`, 1) },
		"repeated member":        func(s string) string { return strings.Replace(s, `"run_id":"100"`, `"run_id":"100","run_id":"100"`, 1) },
		"trailing data":          func(s string) string { return s + ` {}` },
	} {
		dirs := twoHosts(t, f)
		path := filepath.Join(dirs[linux], identityFile)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		edited := change(string(data))
		if edited == string(data) {
			t.Fatalf("%s: identity not edited", name)
		}
		if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_RUN_IDENTITY" {
			t.Errorf("%s: %s %s", name, rep.Status, rep.Code)
		}
		// the expected identity file is read with the same decoder
		if _, err := readIdentity([]byte(edited)); err == nil {
			t.Errorf("%s: readIdentity accepted it", name)
		}
	}
}

// A FAILED judgment of the inline run is stored with the host's result and reaches the merge as FAIL
// (#362): the producer keeps a result whose judgment failed instead of dropping the host.
func TestMergeKeepsAnInlineJudgmentFailure(t *testing.T) {
	f := fixtureFor(t)
	offline, err := kit.VerifySvcExpected(f.svc, f.inventory)
	if err != nil {
		t.Fatal(err)
	}
	dirs := twoHosts(t, f)
	dirs[linux] = writeHost(t, f, linux, nil, func(rs map[string]kit.QueryScopeHostResult) {
		rs[svcInlineKey] = f.inlineResult(t, linux, offline, func(js []kit.SvcJudgment) {
			i := f.trees[0]
			js[i] = kit.SvcJudgment{ID: js[i].ID, State: "FAILED", Code: "SVC_INLINE_NATIVE_TREE"}
		})
	})
	rep := merge(root, expected(), dirs)
	if rep.Status != statusFail || rep.Completion == nil || rep.Completion.Total.Failed != 1 || rep.Completion.Code == "" {
		t.Fatalf("%s %s %+v", rep.Status, rep.Code, rep.Completion)
	}
}

// The inline file may change only the offline BLOCKED obligations, one by one (#362): an exchange that
// keeps every per-kind count (an offline FAILED turned VERIFIED while another inline obligation turns
// FAILED; an offline VERIFIED turned BLOCKED while another turns VERIFIED) must not merge.
func TestMergeRefusesAnExchangeOfInlineJudgments(t *testing.T) {
	f := fixtureFor(t)
	offline, err := kit.VerifySvcExpected(f.svc, f.inventory)
	if err != nil {
		t.Fatal(err)
	}
	trees := f.trees
	id := func(i int) string { return f.svc.Obligations()[i].ID }
	allTrees := slices.Clone(offline) // the derivable inline trees VERIFIED offline
	for _, i := range trees {
		allTrees[i] = kit.SvcJudgment{ID: id(i), State: "VERIFIED"}
	}
	failedFirst := slices.Clone(offline) // the first inline tree FAILED offline
	failedFirst[trees[0]] = kit.SvcJudgment{ID: id(trees[0]), State: "FAILED", Code: "SVC_OFFLINE_INLINE_FAIL"}
	for name, c := range map[string]struct {
		offline []kit.SvcJudgment
		inline  func(js []kit.SvcJudgment)
	}{
		"offline FAILED turned VERIFIED, another inline tree FAILED": {failedFirst, func(js []kit.SvcJudgment) {
			js[trees[0]] = kit.SvcJudgment{ID: id(trees[0]), State: "VERIFIED"}
			js[trees[1]] = kit.SvcJudgment{ID: id(trees[1]), State: "FAILED", Code: "SVC_INLINE_NATIVE_TREE"}
		}},
		"offline VERIFIED turned BLOCKED, another inline tree VERIFIED": {allTrees, func(js []kit.SvcJudgment) {
			js[trees[0]] = kit.SvcJudgment{ID: id(trees[0]), State: "BLOCKED", Code: "SVC_BLOCKED_AGAIN"}
			js[trees[1]] = kit.SvcJudgment{ID: id(trees[1]), State: "VERIFIED"}
		}},
	} {
		dirs := twoHosts(t, f)
		dirs[windows] = writeHost(t, f, windows, nil, func(rs map[string]kit.QueryScopeHostResult) {
			rs[kit.QueryScopeSvc] = svcHostResult(t, f, windows, c.offline)
			rs[svcInlineKey] = f.inlineResult(t, windows, c.offline, c.inline)
		})
		if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_SVC_INLINE_MISMATCH" || rep.Completion != nil {
			t.Errorf("%s: %s %s", name, rep.Status, rep.Code)
		}
	}
}

// The stored results count obligations per kind; the per-obligation companion says which obligation was
// judged how, and both stored svc results must recompute from it (#362). A companion that is missing,
// another host's, incomplete, out of order, not strict JSON, or that does not recompute to the stored
// results is refused, and so is an exchange among obligations the inline run may not change.
func TestMergeChecksThePerObligationCompanion(t *testing.T) {
	f := fixtureFor(t)
	n := len(f.svc.Obligations())
	tree := f.trees[0]
	history := slices.IndexFunc(f.svc.Obligations(), func(o kit.SvcObligation) bool {
		return o.Kind != kit.SvcKindInlineTree && o.Kind != kit.SvcKindQuery
	})
	for name, c := range map[string]struct {
		edit func(d *scopeobligations.Document)
		code string
	}{
		"another host's companion": {func(d *scopeobligations.Document) { d.Host = linux }, scopeobligations.CodeShape},
		"an obligation dropped":    {func(d *scopeobligations.Document) { d.Inline = d.Inline[:n-1] }, scopeobligations.CodeShape},
		"obligations out of order": {func(d *scopeobligations.Document) {
			d.Offline[0], d.Offline[1], d.Inline[0], d.Inline[1] = d.Offline[1], d.Offline[0], d.Inline[1], d.Inline[0]
		}, scopeobligations.CodeShape},
		"unknown state": {func(d *scopeobligations.Document) { d.Inline[0].State = "PASSED" }, scopeobligations.CodeShape},
		"inline list does not recompute to the stored inline result": {func(d *scopeobligations.Document) {
			// a derivable obligation reported as not run is a sound list, but the stored inline result has it VERIFIED
			d.Inline[tree] = scopeobligations.Judgment{ID: d.Inline[tree].ID, State: "BLOCKED", Code: kit.SvcInlineNativeNotRunCode}
		}, scopeobligations.CodeShape},
		"offline judgment taken back by the inline list": {func(d *scopeobligations.Document) {
			d.Inline[history] = scopeobligations.Judgment{ID: d.Inline[history].ID, State: "BLOCKED", Code: "SVC_ONLY_HERE"}
		}, scopeobligations.CodeInline},
	} {
		dirs := twoHosts(t, f)
		dirs[windows] = writeHostWith(t, f, windows, nil, nil, c.edit)
		if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != c.code || rep.Completion != nil {
			t.Errorf("%s: %s %s, want %s", name, rep.Status, rep.Code, c.code)
		}
	}
	// the file itself: missing, unknown member, repeated member, trailing data
	file := scopeobligations.FileName(windows)
	for name, change := range map[string]func(string) string{
		"unknown member": func(s string) string { return strings.Replace(s, `"host":`, `"extra":1,"host":`, 1) },
		"repeated member": func(s string) string {
			return strings.Replace(s, `"host":"windows-amd64"`, `"host":"windows-amd64","host":"windows-amd64"`, 1)
		},
		"trailing data": func(s string) string { return s + ` {}` },
		"empty":         func(string) string { return "" },
	} {
		dirs := twoHosts(t, f)
		data, err := os.ReadFile(filepath.Join(dirs[windows], file))
		if err != nil {
			t.Fatal(err)
		}
		if edited := change(string(data)); edited == string(data) {
			t.Fatalf("%s: not edited", name)
		} else if err := os.WriteFile(filepath.Join(dirs[windows], file), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != scopeobligations.CodeShape {
			t.Errorf("%s: %s %s", name, rep.Status, rep.Code)
		}
	}
	dirs := twoHosts(t, f)
	if err := os.Remove(filepath.Join(dirs[windows], file)); err != nil {
		t.Fatal(err)
	}
	if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != "QUERY_SCOPE_HOST_FILES" {
		t.Errorf("missing companion: %s %s", rep.Status, rep.Code)
	}

	// an exchange among obligations the inline run may not change at all (offline FAILED and VERIFIED of
	// one kind swapped): every per-kind count is kept
	offline, err := kit.VerifySvcExpected(f.svc, f.inventory)
	if err != nil {
		t.Fatal(err)
	}
	x := history
	y := slices.IndexFunc(f.svc.Obligations()[x+1:], func(o kit.SvcObligation) bool { return o.Kind == f.svc.Obligations()[x].Kind }) + x + 1
	if y <= x {
		t.Fatal("no second obligation of the kind")
	}
	id := func(i int, state, code string) kit.SvcJudgment {
		return kit.SvcJudgment{ID: f.svc.Obligations()[i].ID, State: state, Code: code}
	}
	for name, c := range map[string]struct {
		offline, inline [2]kit.SvcJudgment // the judgments of x and y before and after the inline run
	}{
		"offline FAILED and VERIFIED swapped":           {[2]kit.SvcJudgment{id(x, "FAILED", "SVC_OFFLINE_FAIL"), id(y, "VERIFIED", "")}, [2]kit.SvcJudgment{id(x, "VERIFIED", ""), id(y, "FAILED", "SVC_OFFLINE_FAIL")}},
		"offline BLOCKED and VERIFIED swapped":          {[2]kit.SvcJudgment{id(x, "BLOCKED", "SVC_BLOCKED"), id(y, "VERIFIED", "")}, [2]kit.SvcJudgment{id(x, "VERIFIED", ""), id(y, "BLOCKED", "SVC_BLOCKED")}},
		"offline FAILED kept with another failure code": {[2]kit.SvcJudgment{id(x, "FAILED", "SVC_OFFLINE_FAIL"), id(y, "VERIFIED", "")}, [2]kit.SvcJudgment{id(x, "FAILED", "SVC_OTHER_FAIL"), id(y, "VERIFIED", "")}},
	} {
		offlineJ := slices.Clone(offline)
		offlineJ[x], offlineJ[y] = c.offline[0], c.offline[1]
		dirs = twoHosts(t, f)
		dirs[windows] = writeHost(t, f, windows, nil, func(rs map[string]kit.QueryScopeHostResult) {
			rs[kit.QueryScopeSvc] = svcHostResult(t, f, windows, offlineJ)
			rs[svcInlineKey] = f.inlineResult(t, windows, offlineJ, func(js []kit.SvcJudgment) { js[x], js[y] = c.inline[0], c.inline[1] })
		})
		if rep := merge(root, expected(), dirs); rep.Status != statusRefused || rep.Code != scopeobligations.CodeInline {
			t.Errorf("%s: %s %s", name, rep.Status, rep.Code)
		}
	}
}
