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
	registry, predAsset, svcAsset []byte
	svc                           kit.SvcExpected
	inventory                     []byte
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
		fix.registry, fix.predAsset, fix.svcAsset = read(queryregistrycore.Path), read(kit.PredicateExpectedPath), read(kit.SvcExpectedPath)
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
// one VERIFIED SVC judgment into FAILED.
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
	out := map[string]kit.QueryScopeHostResult{}
	for scope, r := range map[string]kit.QueryScopeResult{kit.QueryScopePredicate: predicate, kit.QueryScopeSvc: svc} {
		h, err := kit.NewQueryScopeHostResult(host, r)
		if err != nil {
			t.Fatal(err)
		}
		out[scope] = h
	}
	return out
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
		if err := os.WriteFile(filepath.Join(dir, kit.QueryScopeHostFile(scope, host)), data, 0o644); err != nil {
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
	// predicate 2 x 67 and SVC offline 2 x 263, computed from the registered denominators
	if tot := rep.Completion.Total; tot.Verified != 2*(67+263) || tot.Failed != 0 || rep.Completion.Complete {
		t.Fatalf("total %+v", tot)
	}
	for _, h := range rep.Completion.Hosts {
		if h.Verified != 330 {
			t.Errorf("host %s verified %d, want 330", h.Platform, h.Verified)
		}
	}
	if len(rep.Inputs) != 2 || len(rep.Inputs[0].Files) != 3 || len(rep.Scopes) != 2 {
		t.Fatalf("inputs %+v scopes %v", rep.Inputs, rep.Scopes)
	}
}

func TestMergeKeepsAHostFailure(t *testing.T) {
	f := fixtureFor(t)
	dirs := twoHosts(t, f)
	dirs[linux] = writeHost(t, f, linux, nil, func(rs map[string]kit.QueryScopeHostResult) {
		r := f.hostResults(t, linux, true)[kit.QueryScopeSvc]
		rs[kit.QueryScopeSvc] = r
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
