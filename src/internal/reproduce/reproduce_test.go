package reproduce

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// The test binary doubles as an owned fake generator: invoked as `generate ...` it behaves
// like `tree-sitter generate` with the argv reproduce builds, and its grammar text selects
// a behaviour (MODE:...). It writes outputs derived from the declared closure only.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "generate" {
		os.Exit(fakeGenerate(os.Args[2:]))
	}
	os.Exit(m.Run())
}

func fakeGenerate(args []string) int {
	var out, js, entry string
	opt := "opt"
	abi := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--abi":
			i++
			abi = args[i]
		case "--disable-optimization":
			opt = "noopt"
		case "--output":
			i++
			out = args[i]
		case "--js-runtime":
			i++
			js = args[i]
		default:
			entry = args[i]
		}
	}
	grammar, err := os.ReadFile(entry)
	if err != nil {
		fmt.Fprintln(os.Stderr, "entry:", err)
		return 2
	}
	if js != "" {
		if _, err := os.Stat(js); err != nil {
			fmt.Fprintln(os.Stderr, "js runtime:", err)
			return 2
		}
	}
	files := fakeOutputs(string(grammar), abi, opt, jsClosure(js != ""))
	// A reused workspace shows up as different bytes, so determinism catches it.
	if prior, _ := os.ReadDir(out); len(prior) != 0 {
		files["parser.c"] += " STALE"
	}
	mode := string(grammar)
	switch {
	case strings.Contains(mode, "MODE:exit3"):
		os.WriteFile(filepath.Join(out, "parser.c"), []byte("partial"), 0o644)
		fmt.Fprint(os.Stderr, "generation failed")
		return 3
	case strings.Contains(mode, "MODE:flood"):
		for {
			os.Stdout.Write(make([]byte, 65536))
		}
	case strings.Contains(mode, "MODE:hang"):
		time.Sleep(time.Hour)
	case strings.Contains(mode, "MODE:random"):
		var b [8]byte
		rand.Read(b[:])
		files["parser.c"] += hex.EncodeToString(b[:])
	case strings.Contains(mode, "MODE:no-node-types"):
		delete(files, "node-types.json")
	case strings.Contains(mode, "MODE:extra"):
		files["extra.txt"] = "unregistered"
	case strings.Contains(mode, "MODE:write-source"):
		os.WriteFile("injected.txt", []byte("x"), 0o644)
	case strings.Contains(mode, "MODE:grow-b"):
		// Only workspace B writes 64 KiB into its isolated home (counted as workspace storage,
		// not an output), so only B can exceed storage_bytes while every output stays equal.
		if strings.HasSuffix(filepath.Dir(out), "-b") {
			os.WriteFile(filepath.Join(os.Getenv("HOME"), "padding.bin"), []byte(strings.Repeat("x", 65536)), 0o644)
		}
	case strings.Contains(mode, "MODE:touch-root "):
		_, target, _ := strings.Cut(mode, "MODE:touch-root ")
		f, _ := os.OpenFile(strings.TrimSpace(target), os.O_APPEND|os.O_WRONLY, 0)
		f.Write([]byte("// changed during the run"))
		f.Close()
	case strings.Contains(mode, "MODE:read-home"):
		// A configuration or cache in the isolated home would change the result.
		if data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "poison")); err == nil {
			files["parser.c"] += string(data)
		}
	}
	for name, body := range files {
		p := filepath.Join(out, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return 4
		}
	}
	return 0
}

// jsClosure hashes every .js file below the working directory in JS mode only.
func jsClosure(js bool) string {
	if !js {
		return "json"
	}
	h := sha256.New()
	filepath.WalkDir(".", func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".js") {
			data, _ := os.ReadFile(p)
			fmt.Fprintf(h, "%s:%x\n", filepath.ToSlash(p), sha256.Sum256(data))
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

func fakeOutputs(grammar, abi, opt, closure string) map[string]string {
	g := fmt.Sprintf("%x", sha256.Sum256([]byte(grammar)))
	return map[string]string{
		"parser.c":             "parser abi=" + abi + " " + opt + " " + g + " " + closure,
		"grammar.json":         `{"g":"` + g + `"}`,
		"node-types.json":      "[]",
		"tree_sitter/parser.h": "#pragma once",
	}
}

type fixture struct {
	root, work, out, tool string
	tools                 map[string]string
	profile               map[string]any
}

func digestOf(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func toolIdentity(t *testing.T, name, path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{"name": name, "version": "test", "sha256": digestOf(data), "bytes": len(data)}
}

// newFixture builds a source root, a JS (or JSON) profile and reference outputs computed
// from the same closure the fake generator reads.
func newFixture(t *testing.T, mode, grammar string) *fixture {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{root: t.TempDir(), work: t.TempDir(), tool: exe, tools: map[string]string{"tree-sitter": exe, "node": exe}}
	f.out = filepath.Join(t.TempDir(), "result")
	grammar = strings.ReplaceAll(grammar, "{ROOT}", f.root)
	src := map[string]string{"grammar.js": grammar, "common/helper.js": "module.exports = 1;\n", "src/grammar.json": grammar, "src/scanner.c": "int x;\n"}
	var inputs []map[string]any
	for _, name := range []string{"common/helper.js", "grammar.js", "src/grammar.json", "src/scanner.c"} {
		p := filepath.Join(f.root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(src[name]), 0o644)
		role := map[string]string{"common/helper.js": "js_helper", "grammar.js": "grammar_js", "src/grammar.json": "grammar_json", "src/scanner.c": "scanner"}[name]
		inputs = append(inputs, map[string]any{"path": name, "role": role, "sha256": digestOf([]byte(src[name])), "bytes": len(src[name])})
	}
	closure := "json"
	entry := "src/grammar.json"
	if mode == kit.ModeJS {
		entry = "grammar.js"
		h := sha256.New()
		for _, name := range []string{"common/helper.js", "grammar.js"} {
			fmt.Fprintf(h, "%s:%x\n", name, sha256.Sum256([]byte(src[name])))
		}
		closure = hex.EncodeToString(h.Sum(nil))
	}
	ref := fakeOutputs(src[entry], "15", "opt", closure)
	var outputs []map[string]any
	for _, name := range []string{"grammar.json", "node-types.json", "parser.c", "tree_sitter/parser.h"} {
		outputs = append(outputs, map[string]any{"path": name, "reference": kit.ReferencePresent, "sha256": digestOf([]byte(ref[name])), "bytes": len(ref[name])})
	}
	f.profile = map[string]any{"schema": kit.ReproduceSchema, "id": "fixture", "route": "fixture", "mode": mode,
		"generator": toolIdentity(t, "tree-sitter", exe), "abi": 15, "optimize": true, "grammar": entry, "inputs": inputs, "outputs": outputs,
		"limits": map[string]any{"wall_seconds": 60, "output_bytes": 1 << 20, "storage_bytes": 64 << 20, "memory_bytes": 1 << 30, "input_files": 64, "input_bytes": 1 << 20, "file_bytes": 1 << 20}}
	if mode == kit.ModeJS {
		f.profile["js_runtime"] = toolIdentity(t, "node", exe)
	}
	return f
}

func (f *fixture) run(t *testing.T, mut ...func(*Request)) (Result, error) {
	t.Helper()
	data, _ := json.Marshal(f.profile)
	req := Request{Root: f.root, Profile: data, Tools: f.tools, Work: f.work, Out: f.out, Allow: []string{CapExecGenerator},
		CgroupParent: os.Getenv("TSGK_CGROUP_PARENT"), Grace: 2 * time.Second}
	for _, m := range mut {
		m(&req)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return Reproduce(ctx, req)
}

func skipWithoutHardBackend(t *testing.T) {
	if runtime.GOOS == "linux" && os.Getenv("TSGK_CGROUP_PARENT") == "" {
		t.Skip("Linux reproduction requires the delegated cgroup hard memory backend (CI provides it)")
	}
}

func treeDigest(t *testing.T, root string) string {
	h := sha256.New()
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, p)
		data := []byte{}
		if !d.IsDir() {
			data, _ = os.ReadFile(p)
		}
		fmt.Fprintf(h, "%s %v %x\n", filepath.ToSlash(rel), d.IsDir(), sha256.Sum256(data))
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

func entries(t *testing.T, dir string) []string {
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

// S04-A01/A05: two fresh workspaces, explicit per-output comparisons, an unchanged source,
// a no-clobber publication and removed workspaces.
func TestReproducePass(t *testing.T) {
	skipWithoutHardBackend(t)
	f := newFixture(t, kit.ModeJS, "MODE:ok")
	before := treeDigest(t, f.root)
	res, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	c := res.Claims
	if res.Assessment != kit.AssessPass || res.ExecutionStatus != kit.StatusCompleted || c.GeneratorRan != ClaimPass || c.Deterministic != ClaimPass ||
		c.ReferenceMatch != ClaimPass || c.JSReproduction != ClaimPass || c.JSONRegeneration != ClaimNone {
		t.Fatalf("%+v %+v", res.Report, c)
	}
	if len(res.Runs) != 2 || res.Runs[0].State != "EXECUTED" || res.Runs[1].State != "EXECUTED" {
		t.Fatalf("runs %+v", res.Runs)
	}
	for _, cmp := range res.Comparisons {
		if cmp.AcrossRun != "EQUAL" || cmp.Reference != "MATCH" {
			t.Fatalf("%+v", cmp)
		}
	}
	if treeDigest(t, f.root) != before || res.SourceBefore != res.SourceAfter {
		t.Fatal("source root changed")
	}
	if res.Workspaces != "REMOVED" || len(entries(t, f.work)) != 0 {
		t.Fatalf("workspaces left: %s %v", res.Workspaces, entries(t, f.work))
	}
	for _, p := range []string{"result.json", "workspace-a/out/parser.c", "workspace-b/out/tree_sitter/parser.h", "workspace-a/stderr.log"} {
		if _, err := os.Stat(filepath.Join(f.out, filepath.FromSlash(p))); err != nil {
			t.Fatalf("publication missing %s", p)
		}
	}
	if !slices.Contains(res.Argv, "--js-runtime") || slices.Contains(res.EnvNames, "GOPATH") || res.RunIdentity == "" {
		t.Fatalf("argv/env/identity %v %v %q", res.Argv, res.EnvNames, res.RunIdentity)
	}
	// A second publication to the same destination is refused before any launch.
	if _, err := f.run(t); !isCode(err, "OUTPUT_EXISTS") {
		t.Fatalf("clobber: %v", err)
	}
}

func isCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// S04-A02: a stale, changed or missing reference is a finding; the source is not touched.
func TestReferenceFindings(t *testing.T) {
	skipWithoutHardBackend(t)
	f := newFixture(t, kit.ModeJS, "MODE:ok")
	outs := f.profile["outputs"].([]map[string]any)
	outs[2]["sha256"] = strings.Repeat("0", 64) // stale parser.c reference
	before := treeDigest(t, f.root)
	res, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if res.Claims.ReferenceMatch != ClaimFail || res.Comparisons[2].Reference != "MISMATCH" || res.Claims.Deterministic != ClaimPass ||
		res.Assessment != kit.AssessFail || res.Claims.JSReproduction != ClaimFail {
		t.Fatalf("%+v %+v", res.Claims, res.Comparisons)
	}
	if treeDigest(t, f.root) != before {
		t.Fatal("reference mismatch regenerated into the source")
	}
	f = newFixture(t, kit.ModeJS, "MODE:ok")
	outs = f.profile["outputs"].([]map[string]any)
	outs[2] = map[string]any{"path": "parser.c", "reference": kit.ReferenceAbsent}
	res, err = f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if res.Claims.ReferenceMatch != ClaimNone || res.Comparisons[2].Reference != "REFERENCE_ABSENT" || res.Claims.Deterministic != ClaimPass ||
		res.Assessment != kit.AssessBlocked || res.Claims.JSReproduction != ClaimNone {
		t.Fatalf("absent reference: %+v %+v", res.Claims, res.Comparisons)
	}
}

// S04-A03: a JSON-only run cannot claim the JS closure, whatever the JS helper holds.
func TestJSONOnlyRun(t *testing.T) {
	skipWithoutHardBackend(t)
	f := newFixture(t, kit.ModeJSON, "MODE:ok")
	os.WriteFile(filepath.Join(f.root, "common", "helper.js"), []byte("module.exports = 2; // modified\n"), 0o644)
	// The helper is still declared; drop it so the JSON closure excludes it, as a JSON profile does.
	f.profile["inputs"] = slices.DeleteFunc(f.profile["inputs"].([]map[string]any), func(m map[string]any) bool { return m["role"] == "js_helper" })
	res, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if res.Claims.JSONRegeneration != ClaimPass || res.Claims.JSReproduction != ClaimNone || slices.Contains(res.Argv, "--js-runtime") ||
		!slices.Contains(res.Coverage.Unsupported, "js_reproduction: JSON_ONLY_RUN") {
		t.Fatalf("%+v %v", res.Claims, res.Argv)
	}
	// In JS mode the same modified helper is a declared-input mismatch, refused before launch.
	g := newFixture(t, kit.ModeJS, "MODE:ok")
	os.WriteFile(filepath.Join(g.root, "common", "helper.js"), []byte("module.exports = 2; // modified\n"), 0o644)
	if _, err := g.run(t); !isCode(err, "SOURCE_MISMATCH") {
		t.Fatalf("modified JS helper: %v", err)
	}
}

// S04-A04: tools, options, headers and dependencies are part of the run identity; a
// poisoned work directory or home is never reused.
func TestRunIdentityAndPoison(t *testing.T) {
	skipWithoutHardBackend(t)
	f := newFixture(t, kit.ModeJS, "MODE:read-home")
	for _, name := range []string{"tsgk-ws-000000000000-a", "home"} {
		os.MkdirAll(filepath.Join(f.work, name), 0o755)
		os.WriteFile(filepath.Join(f.work, name, "poison"), []byte("POISON"), 0o644)
	}
	res, err := f.run(t)
	if err != nil || res.Assessment != kit.AssessPass {
		t.Fatalf("poisoned work dir changed the result: %v %+v", err, res.Claims)
	}
	base := res.RunIdentity
	variants := map[string]func(p map[string]any){
		"abi":      func(p map[string]any) { p["abi"] = 14 },
		"optimize": func(p map[string]any) { p["optimize"] = false },
		"tool":     func(p map[string]any) { p["generator"].(map[string]any)["version"] = "other" },
		"header": func(p map[string]any) {
			p["inputs"] = append(p["inputs"].([]map[string]any), map[string]any{"path": "src/tree_sitter/parser.h", "role": "header", "sha256": digestOf([]byte("h")), "bytes": 1})
		},
	}
	for name, mut := range variants {
		g := newFixture(t, kit.ModeJS, "MODE:read-home")
		mut(g.profile)
		if name == "header" {
			os.MkdirAll(filepath.Join(g.root, "src", "tree_sitter"), 0o755)
			os.WriteFile(filepath.Join(g.root, "src", "tree_sitter", "parser.h"), []byte("h"), 0o644)
		}
		r, err := g.run(t)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if r.RunIdentity == base || r.RunIdentity == "" {
			t.Fatalf("%s did not change the run identity", name)
		}
	}
}

// S04-A05: writes into the workspace source copy are findings; the root stays unchanged;
// a failed run keeps its partial outputs and logs as a FAILED publication.
func TestSourceWritesAndFailedPublication(t *testing.T) {
	skipWithoutHardBackend(t)
	f := newFixture(t, kit.ModeJS, "MODE:write-source")
	before := treeDigest(t, f.root)
	res, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if treeDigest(t, f.root) != before || !hasFinding(res, "WORKSPACE_SOURCE_WRITTEN") {
		t.Fatalf("%+v", res.Findings)
	}
	h := newFixture(t, kit.ModeJS, "MODE:touch-root "+filepath.Join("{ROOT}", "src", "scanner.c"))
	res, err = h.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(res, "SOURCE_CHANGED") || res.ExecutionStatus != kit.StatusFailed || res.Assessment == kit.AssessPass || res.SourceBefore == res.SourceAfter ||
		res.Claims.JSReproduction == ClaimPass || res.Claims.Deterministic != ClaimPass {
		t.Fatalf("root change not reported: %+v", res.Report)
	}
	g := newFixture(t, kit.ModeJS, "MODE:exit3")
	res, err = g.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExecutionStatus != kit.StatusFailed || res.Claims.GeneratorRan != ClaimFail || res.Runs[1].State != "NOT_RUN" || !hasFinding(res, "GENERATOR_EXIT_NONZERO") {
		t.Fatalf("%+v %+v", res.Report, res.Runs)
	}
	data, err := os.ReadFile(filepath.Join(g.out, "result.json"))
	if err != nil || !strings.Contains(string(data), `"FAILED"`) {
		t.Fatalf("failed result not published: %v", err)
	}
	if log, _ := os.ReadFile(filepath.Join(g.out, "workspace-a", "stderr.log")); string(log) != "generation failed" {
		t.Fatalf("stderr not retained: %q", log)
	}
}

func hasFinding(r Result, code string) bool {
	for _, f := range r.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

// S04-A06: a missing or different executable, an ungranted capability and a wrong ABI are
// refused before any workspace exists; there is no installer or PATH fallback.
func TestBlockedBeforeLaunch(t *testing.T) {
	other := filepath.Join(t.TempDir(), "other")
	os.WriteFile(other, []byte("not the pinned tool"), 0o755)
	cases := map[string]func(*fixture, *Request){
		"TOOL_MISSING":           func(f *fixture, r *Request) { delete(r.Tools, "tree-sitter") },
		"TOOL_IDENTITY_MISMATCH": func(f *fixture, r *Request) { r.Tools = map[string]string{"tree-sitter": other, "node": f.tool} },
		"CAPABILITY_NOT_GRANTED": func(f *fixture, r *Request) { r.Allow = nil },
		"ABI_UNSUPPORTED": func(f *fixture, r *Request) {
			f.profile["abi"] = 13
			r.Profile, _ = json.Marshal(f.profile)
		},
		"SOURCE_MISMATCH": func(f *fixture, r *Request) {
			os.WriteFile(filepath.Join(f.root, "src", "scanner.c"), []byte("changed"), 0o644)
		},
	}
	if runtime.GOOS == "linux" {
		cases["MEMORY_HARD_CAP_UNSUPPORTED"] = func(f *fixture, r *Request) { r.CgroupParent = "" }
	}
	for code, mut := range cases {
		f := newFixture(t, kit.ModeJS, "MODE:ok")
		res, err := f.run(t, func(r *Request) {
			r.Tools = map[string]string{"tree-sitter": f.tool, "node": f.tool}
			mut(f, r)
		})
		if !isCode(err, code) || res.ExecutionStatus != kit.StatusNotRun || res.EvidenceMode != kit.ModeNotRun {
			if code == "MEMORY_HARD_CAP_UNSUPPORTED" && os.Getenv("TSGK_CGROUP_PARENT") == "" && isCode(err, code) {
				continue
			}
			t.Fatalf("%s: %v %+v", code, err, res.Report)
		}
		if names := entries(t, f.work); len(names) != 0 {
			t.Fatalf("%s left work entries %v", code, names)
		}
		if _, err := os.Stat(f.out); err == nil {
			t.Fatalf("%s published a result", code)
		}
	}
}

// S04-A07/A11: missing outputs, unregistered extras, nondeterminism, floods and hangs are
// typed results, never forged success.
func TestFailureKinds(t *testing.T) {
	skipWithoutHardBackend(t)
	type want struct {
		status, assessment, claim, finding string
	}
	cases := map[string]want{
		"MODE:no-node-types": {kit.StatusCompleted, kit.AssessFail, "reference", "OUTPUT_MISSING"},
		"MODE:random":        {kit.StatusCompleted, kit.AssessFail, "deterministic", ""},
		"MODE:extra":         {kit.StatusCompleted, kit.AssessPass, "", "UNREGISTERED_OUTPUT"},
		"MODE:flood":         {kit.StatusResourceLimit, kit.AssessBlocked, "ran", "OUTPUT_LIMIT"},
		"MODE:hang":          {kit.StatusResourceLimit, kit.AssessBlocked, "ran", "WALL_LIMIT"},
	}
	for mode, w := range cases {
		f := newFixture(t, kit.ModeJS, mode)
		if mode == "MODE:hang" {
			f.profile["limits"].(map[string]any)["wall_seconds"] = 1
		}
		res, err := f.run(t)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if res.ExecutionStatus != w.status || res.Assessment != w.assessment {
			t.Fatalf("%s: %s/%s %+v", mode, res.ExecutionStatus, res.Assessment, res.Findings)
		}
		switch w.claim {
		case "reference":
			if res.Claims.ReferenceMatch != ClaimFail || res.Comparisons[1].Reference != "OUTPUT_MISSING" {
				t.Fatalf("%s: %+v", mode, res.Comparisons)
			}
		case "deterministic":
			if res.Claims.Deterministic != ClaimFail || res.Comparisons[2].AcrossRun != "DIFFERENT" {
				t.Fatalf("%s: %+v", mode, res.Comparisons)
			}
		case "ran":
			if res.Claims.GeneratorRan != ClaimFail || res.Claims.JSReproduction == ClaimPass {
				t.Fatalf("%s: %+v", mode, res.Claims)
			}
		}
		if w.finding != "" && w.finding != "OUTPUT_MISSING" && !hasFinding(res, w.finding) {
			t.Fatalf("%s: finding %s missing in %+v", mode, w.finding, res.Findings)
		}
	}
}

// S04-A09: a successful tool run whose workspace cleanup or evidence write fails is not a
// completed success.
func TestEvidenceAndCleanupFailures(t *testing.T) {
	skipWithoutHardBackend(t)
	f := newFixture(t, kit.ModeJS, "MODE:ok")
	removeAll = func(p string) error {
		if strings.Contains(filepath.Base(p), "tsgk-ws-") {
			return errors.New("injected cleanup failure")
		}
		return os.RemoveAll(p)
	}
	res, err := f.run(t)
	removeAll = os.RemoveAll
	if err != nil || res.ExecutionStatus != kit.StatusFailed || !hasFinding(res, "WORKSPACE_CLEANUP_FAILED") || res.Assessment == kit.AssessPass {
		t.Fatalf("cleanup failure: %v %+v", err, res.Report)
	}
	k := newFixture(t, kit.ModeJS, "MODE:ok")
	removeAll = func(p string) error {
		if strings.Contains(filepath.Base(p), "tsgk-tools-") {
			return errors.New("injected tool cleanup failure")
		}
		return os.RemoveAll(p)
	}
	res, err = k.run(t)
	removeAll = os.RemoveAll
	if err != nil || res.ExecutionStatus != kit.StatusFailed || !hasFinding(res, "TOOL_CLEANUP_FAILED") || res.Assessment == kit.AssessPass {
		t.Fatalf("tool cleanup failure: %v %+v", err, res.Report)
	}
	g := newFixture(t, kit.ModeJS, "MODE:ok")
	orig := writeFile
	writeFile = func(name string, data []byte) error {
		if filepath.Base(name) == "result.json" {
			return errors.New("injected evidence failure")
		}
		return orig(name, data)
	}
	res, err = g.run(t)
	writeFile = orig
	if !isCode(err, "EVIDENCE_WRITE_FAILED") || res.ExecutionStatus != kit.StatusFailed || res.Assessment == kit.AssessPass {
		t.Fatalf("evidence failure: %v %+v", err, res.Report)
	}
}

// S04-A11: the observed workspace storage at the profile limit passes; one byte under
// it ends RESOURCE_LIMIT/STORAGE_LIMIT (an observation after the run, not a quota).
func TestStorageLimit(t *testing.T) {
	skipWithoutHardBackend(t)
	f := newFixture(t, kit.ModeJS, "MODE:ok")
	res, err := f.run(t)
	if err != nil || res.Assessment != kit.AssessPass {
		t.Fatalf("%v %+v", err, res.Report)
	}
	used := res.Runs[0].StorageBytes
	if used <= 0 || res.Runs[1].StorageBytes != used {
		t.Fatalf("storage observations %d %d", used, res.Runs[1].StorageBytes)
	}
	for limit, want := range map[int64]string{used: kit.AssessPass, used - 1: kit.AssessBlocked} {
		g := newFixture(t, kit.ModeJS, "MODE:ok")
		g.profile["limits"].(map[string]any)["storage_bytes"] = limit
		res, err := g.run(t)
		if err != nil || res.Assessment != want {
			t.Fatalf("limit %d: %v %+v", limit, err, res.Report)
		}
		if want == kit.AssessBlocked && (res.ExecutionStatus != kit.StatusResourceLimit || !hasFinding(res, "STORAGE_LIMIT") || res.Runs[1].State != "NOT_RUN") {
			t.Fatalf("limit %d: %+v %+v", limit, res.Report, res.Runs)
		}
	}
}

// R2-02/R2-03: when only workspace B exceeds storage the run is RESOURCE_LIMIT/BLOCKED and
// no mode claim stays PASS; the cause is recorded beside any other.
func TestStorageLimitWorkspaceB(t *testing.T) {
	skipWithoutHardBackend(t)
	f := newFixture(t, kit.ModeJS, "MODE:grow-b")
	f.profile["limits"].(map[string]any)["storage_bytes"] = 1 << 20
	res, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	a, b := res.Runs[0].StorageBytes, res.Runs[1].StorageBytes
	if b <= a {
		t.Fatalf("fixture did not grow B: %d %d", a, b)
	}
	g := newFixture(t, kit.ModeJS, "MODE:grow-b")
	g.profile["limits"].(map[string]any)["storage_bytes"] = a
	res, err = g.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExecutionStatus != kit.StatusResourceLimit || res.Assessment != kit.AssessBlocked || !hasFinding(res, "STORAGE_LIMIT") ||
		res.Claims.Deterministic != ClaimPass || res.Claims.ReferenceMatch != ClaimPass || res.Claims.JSReproduction != ClaimNone || res.Runs[1].State != "EXECUTED" {
		t.Fatalf("%+v %+v", res.Report, res.Claims)
	}
}

// R1-03: a node_modules reachable from the workspace but outside the declared snapshot
// blocks a JS run before launch; a JSON run does not resolve modules and is unaffected.
func TestJSClosureLeak(t *testing.T) {
	skipWithoutHardBackend(t)
	f := newFixture(t, kit.ModeJS, "MODE:ok")
	if err := os.MkdirAll(filepath.Join(f.work, "node_modules", "undeclared"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t); !isCode(err, "JS_CLOSURE_LEAK") {
		t.Fatalf("leak not refused: %v", err)
	}
	if names := entries(t, f.work); len(names) != 1 {
		t.Fatalf("work entries after refusal: %v", names)
	}
	g := newFixture(t, kit.ModeJSON, "MODE:ok")
	g.work = f.work
	if res, err := g.run(t); err != nil || res.Assessment != kit.AssessPass {
		t.Fatalf("json run: %v %+v", err, res.Report)
	}
}
