package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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

// #218/#277: the csharp-symbols qualification role on the registered grammar. Three
// registered symbol workloads run on this host as run-routes.ps1 -Symbols runs them; Qualify
// passes the role row and keeps the other platform DEFERRED. A changed or reordered profile,
// a consistent forgery of profile and witness, a dropped case, a changed witness or result
// case, a result moved from the other workload, an unlisted set file or a registered
// selection that is not the recorded one fails the row. An unregistered set, or evidence a
// host of the deferred platform submits, fails completeness. An inventory that drops the
// deferred platform or registers a stopped case with another outcome is refused.
func TestQualifySymbolRole(t *testing.T) {
	rt, cc := os.Getenv("TSGK_NATIVE_RUNTIME"), os.Getenv("TSGK_NATIVE_CC")
	if rt == "" || cc == "" {
		if os.Getenv("TSGK_NATIVE_REQUIRED") == "1" {
			t.Fatal("native tools required")
		}
		t.Skip("native tools not configured")
	}
	if os.Getenv("TSGK_BENCH_PREPARED") == "" {
		t.Skip("registered C# prepared closure not configured")
	}
	repo := repoRoot(t)
	read := func(p string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var inv kit.QualificationInventory
	if err := json.Unmarshal(read("src/contracts/qualification-c1.json"), &inv); err != nil {
		t.Fatal(err)
	}
	const none, ac, switched = "s05-csharp-symbols-none", "s05-csharp-symbols-a-c", "s05-csharp-symbols-switch"
	var ws []kit.QualWorkload
	for _, x := range inv.ExtraRoles {
		for _, y := range x.Workloads {
			if x.ID == "csharp-symbols" && (y.Set == none || y.Set == ac || y.Set == switched) {
				ws = append(ws, y)
			}
		}
	}
	if len(ws) != 3 || ws[0].Set != none || ws[2].Set != switched {
		t.Fatal("registered symbol workloads missing")
	}
	// this host executes the role; another registered platform is deferred
	var hp, other kit.QualPlatform
	for _, p := range inv.Platforms {
		switch {
		case p.GOOS == runtime.GOOS && p.GOARCH == runtime.GOARCH:
			hp = p
		case other.ID == "":
			other = p
		}
	}
	if hp.ID == "" {
		t.Skip("host platform not registered")
	}
	mini := func(change func(r *kit.QualRole)) []byte {
		t.Helper()
		m := inv
		m.Platforms, m.Routes, m.Cells = []kit.QualPlatform{hp, other}, inv.Routes[:1], 2
		r := kit.QualRole{ID: "csharp-symbols", Role: "owned", Status: kit.RoleExecuted, Reason: "test", Platforms: []string{hp.ID}, NotApplicable: map[string]string{},
			Deferred: map[string]string{other.ID: "deferred in this test"}}
		for _, w := range ws {
			w.Cases = slices.Clone(w.Cases)
			r.Workloads = append(r.Workloads, w)
		}
		if change != nil {
			change(&r)
		}
		m.ExtraRoles = []kit.QualRole{r}
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	// the host runs, as run-routes.ps1 -Symbols makes them
	base := t.TempDir()
	root, host := filepath.Join(base, "root"), filepath.Join(base, "host")
	symbol, grammar := csharpClosure(t, root)
	var corpus struct{ Cases []struct{ ID, Source string } }
	if err := json.Unmarshal(read("src/testdata/csharp-pp/cases.json"), &corpus); err != nil {
		t.Fatal(err)
	}
	ccData, err := os.ReadFile(cc)
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(base, "work")
	for _, d := range []string{filepath.Join(root, "cases"), work, filepath.Join(host, "incremental")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range ws {
		sources := map[string][]byte{}
		var cases []any
		for _, c := range w.Cases {
			for _, k := range corpus.Cases {
				if k.ID == c.ID {
					sources[c.ID], _ = base64.StdEncoding.DecodeString(k.Source)
				}
			}
			if err := os.WriteFile(filepath.Join(root, "cases", c.ID+".cs"), sources[c.ID], 0o644); err != nil {
				t.Fatal(err)
			}
			cases = append(cases, map[string]any{"id": c.ID, "input": fileEntry(t, root, "cases/"+c.ID+".cs", "case"), "edits": c.Edits, "points": []any{}, "expect": c.Expect})
		}
		pdata, _ := json.Marshal(map[string]any{"schema": kit.IncrementalPreprocessSchema, "id": w.Profile, "route": w.Route, "operation": w.Operation, "symbol": symbol,
			"encoding": "UTF-8", "output": w.Output, "compiler": map[string]any{"name": "cc", "version": "test", "sha256": sum(ccData), "bytes": len(ccData)},
			"grammar": grammar, "declarations": nil, "preprocess": w.Preprocess, "cases": cases})
		pf, out := filepath.Join(base, w.Set+".json"), filepath.Join(host, "incremental", w.Set)
		if err := os.WriteFile(pf, pdata, 0o644); err != nil {
			t.Fatal(err)
		}
		args := []string{"incremental", "--root", root, "--profile", pf, "--runtime", rt, "--tool", "cc=" + cc, "--work", work, "--out", out, "--allow", "BUILD_NATIVE", "--allow", "EXEC_NATIVE", "--reuse-observation"}
		if cg := os.Getenv("TSGK_CGROUP_PARENT"); cg != "" {
			args = append(args, "--cgroup-parent", cg)
		}
		cli(t, context.Background(), args...) // a stopped case blocks the run; Qualify judges it
		if err := os.WriteFile(filepath.Join(out, "profile.json"), pdata, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(out, "sources"), 0o755); err != nil {
			t.Fatal(err)
		}
		for i, c := range w.Cases {
			if err := os.WriteFile(filepath.Join(out, "sources", fmt.Sprintf("%05d-%s.bin", i, c.ID)), sources[c.ID], 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	cand := strings.Repeat("c", 40)
	identity := func(dir string, p kit.QualPlatform) {
		t.Helper()
		id, _ := json.Marshal(kit.RunIdentity{Schema: kit.RunIdentitySchema, Repository: "o/r", Workflow: "foundation", RunID: "1", RunAttempt: "1", Event: "push", SHA: cand, HeadSHA: cand,
			Checkout: cand, Job: "native routes", RunnerOS: p.GOOS, RunnerArch: p.GOARCH, Image: "test", GOOS: p.GOOS, GOARCH: p.GOARCH, GoVersion: runtime.Version(), EvidenceMode: kit.ModeNewRun})
		if err := os.WriteFile(filepath.Join(dir, "run-identity.json"), id, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	identity(host, hp)
	qualify := func(invData []byte, dir string, more ...kit.QualifyHost) (kit.QualificationResult, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return kit.Qualify(ctx, kit.QualifyRequest{Inventory: invData, Candidate: cand, Hosts: append([]kit.QualifyHost{{Platform: hp.ID, Root: dir}}, more...)})
	}
	row := func(r kit.QualificationResult, plat string) kit.QualRoleRow {
		for _, x := range r.ExtraRoles {
			if x.ID == "csharp-symbols" && x.Platform == plat {
				return x
			}
		}
		t.Fatalf("no csharp-symbols row for %s", plat)
		return kit.QualRoleRow{}
	}
	codes := func(x kit.QualRoleRow) (out []string) {
		for _, s := range x.Sets {
			for _, f := range s.Findings {
				out = append(out, f.Code)
			}
		}
		return out
	}
	unregistered := func(r kit.QualificationResult) bool {
		return r.Completeness == kit.AssessFail && slices.ContainsFunc(r.Findings, func(f kit.Finding) bool { return f.Code == "HOST_FILE_UNREGISTERED" })
	}
	copyHost := func(name string) string {
		t.Helper()
		dir := filepath.Join(base, "t-"+strings.ReplaceAll(name, " ", "-"))
		if err := os.CopyFS(dir, os.DirFS(host)); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	res, err := qualify(mini(nil), host)
	if err != nil {
		t.Fatal(err)
	}
	if r := row(res, hp.ID); r.Status != kit.CellPass || r.Mechanism != kit.AssessPass || r.Checks != "PASS" || len(r.Sets) != 3 || r.Sets[0].Records != len(ws[0].Cases) || r.Sets[2].Records != 1 {
		t.Fatalf("host row %s %s %s %v", r.Status, r.Mechanism, r.Checks, codes(r))
	}
	if d := row(res, other.ID); d.Status != kit.RoleDeferred || d.Mechanism != "" || d.Checks != "" || len(d.Sets) != 0 {
		t.Fatalf("deferred row %+v", d)
	}
	// tampered host evidence fails the row
	profile := func(dir string, change func(p map[string]any)) {
		t.Helper()
		path := filepath.Join(dir, "profile.json")
		var p map[string]any
		if data, err := os.ReadFile(path); err != nil || json.Unmarshal(data, &p) != nil {
			t.Fatal(err)
		}
		change(p)
		data, _ := json.Marshal(p)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, set, code string
		change          func(dir string)
	}{
		{"symbols", none, "REGISTRATION_MISMATCH", func(dir string) {
			rewrite(t, filepath.Join(dir, "profile.json"), []byte(`"symbols":[]`), []byte(`"symbols":["A"]`))
		}},
		{"symbols reordered", ac, "REGISTRATION_MISMATCH", func(dir string) {
			rewrite(t, filepath.Join(dir, "profile.json"), []byte(`"symbols":["A","C"]`), []byte(`"symbols":["C","A"]`))
		}},
		{"default mode", none, "REGISTRATION_MISMATCH", func(dir string) {
			rewrite(t, filepath.Join(dir, "profile.json"), []byte(`"preprocess":{"mode":"csharp-symbols/r1","symbols":[]},`), nil)
		}},
		{"case dropped", none, "REGISTRATION_MISMATCH", func(dir string) {
			profile(dir, func(p map[string]any) { p["cases"] = p["cases"].([]any)[1:] })
		}},
		// a forgery consistent inside the set: another source, its witness and its digest
		{"consistent forgery", none, "REGISTRATION_MISMATCH", func(dir string) {
			forged := []byte("#if NEVER\nclass D {}\n#endif\nclass C {}\n")
			if err := os.WriteFile(filepath.Join(dir, "sources", "00000-"+ws[0].Cases[0].ID+".bin"), forged, 0o644); err != nil {
				t.Fatal(err)
			}
			profile(dir, func(p map[string]any) {
				p["cases"].([]any)[0].(map[string]any)["input"] = map[string]any{"path": "cases/" + ws[0].Cases[0].ID + ".cs", "role": "case", "sha256": sum(forged), "bytes": len(forged)}
			})
		}},
		{"witness", none, "REPLAY_INVALID", func(dir string) {
			rewrite(t, filepath.Join(dir, "sources", "00000-"+ws[0].Cases[0].ID+".bin"), []byte("class C {}"), []byte("class D {}"))
		}},
		{"case outcome", none, "REPLAY_INVALID", func(dir string) {
			rewrite(t, filepath.Join(dir, "result.json"), []byte(`"code":"PREPROCESS_ZERO_ACTIVE"`), []byte(`"code":"PREPROCESS_MALFORMED"`))
		}},
		{"result moved", ac, "REPLAY_INVALID", func(dir string) {
			data, err := os.ReadFile(filepath.Join(filepath.Dir(dir), none, "result.json"))
			if err != nil || os.WriteFile(filepath.Join(dir, "result.json"), data, 0o644) != nil {
				t.Fatal(err)
			}
		}},
		{"unlisted file", none, "MEMBER_UNLISTED", func(dir string) {
			if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		dir := copyHost(tc.name)
		tc.change(filepath.Join(dir, "incremental", tc.set))
		res, err := qualify(mini(nil), dir)
		if r := row(res, hp.ID); err != nil || r.Status != kit.CellFail || !slices.Contains(codes(r), tc.code) {
			t.Errorf("%s: %v %s %v", tc.name, err, r.Status, codes(r))
		}
	}
	// a registered selection that is not the recorded one fails the row
	res, err = qualify(mini(func(r *kit.QualRole) {
		c := &r.Workloads[0].Cases[0]
		c.PreprocessExpect = slices.Clone(c.PreprocessExpect)
		st := c.PreprocessExpect[0]
		st.Active = []kit.ByteRange{{Start: st.Active[0].Start, End: st.Active[0].End - 1}}
		c.PreprocessExpect[0] = st
	}), host)
	if r := row(res, hp.ID); err != nil || r.Status != kit.CellFail || r.Mechanism != kit.AssessPass || r.Checks != "FAIL" {
		t.Errorf("registered selection: %v %s %s %s %v", err, r.Status, r.Mechanism, r.Checks, codes(r))
	}
	// a set the inventory does not register fails completeness
	extra := copyHost("extra set")
	if err := os.CopyFS(filepath.Join(extra, "incremental", "s05-csharp-symbols-zz"), os.DirFS(filepath.Join(host, "incremental", none))); err != nil {
		t.Fatal(err)
	}
	if res, _ := qualify(mini(nil), extra); !unregistered(res) {
		t.Errorf("unregistered set: %s", res.Completeness)
	}
	// a host of the deferred platform that submits the role's evidence fails completeness
	deferred := copyHost("deferred host")
	identity(deferred, other)
	if res, _ := qualify(mini(nil), host, kit.QualifyHost{Platform: other.ID, Root: deferred}); !unregistered(res) || row(res, other.ID).Status != kit.RoleDeferred {
		t.Errorf("deferred host evidence: %s %s", res.Completeness, row(res, other.ID).Status)
	}
	// refused inventories
	for name, change := range map[string]func(r *kit.QualRole){
		"deferred dropped":  func(r *kit.QualRole) { r.Deferred = map[string]string{} },
		"deferred executed": func(r *kit.QualRole) { r.Deferred[hp.ID] = "x" },
		"stopped outcome": func(r *kit.QualRole) {
			for i, c := range r.Workloads[0].Cases {
				if c.ExpectCode == "PREPROCESS_ZERO_ACTIVE" {
					r.Workloads[0].Cases[i].ExpectCode = "PREPROCESS_MALFORMED"
				}
			}
		},
		"no selection": func(r *kit.QualRole) { r.Workloads[0].Cases[0].PreprocessExpect = nil },
	} {
		if _, err := kit.ParseQualificationInventory(mini(change)); err == nil {
			t.Errorf("%s: inventory accepted", name)
		}
	}
}

// rewrite replaces old (exactly once) by new in a file.
func rewrite(t *testing.T, path string, old, new []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || bytes.Count(data, old) != 1 {
		t.Fatalf("%s: %v %d", path, err, bytes.Count(data, old))
	}
	if err := os.WriteFile(path, bytes.Replace(data, old, new, 1), 0o644); err != nil {
		t.Fatal(err)
	}
}
