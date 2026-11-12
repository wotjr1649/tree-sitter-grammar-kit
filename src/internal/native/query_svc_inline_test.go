package native

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// TestNativeSvcInlineRealGrammar runs the SVC69 cases that have a DERIVED inline expectation on the
// real C# grammar (a directory from TSGK_SVC_CSHARP_GRAMMAR whose files match the registered
// qualification closure byte for byte) and judges the native trees and the two query streams
// against the sealed independent expectation. The scope fold of the whole SVC69 denominator is
// logged. BLOCKED fragments are never run for a verdict and never become VERIFIED.
func TestNativeSvcInlineRealGrammar(t *testing.T) {
	// The prepared C# grammar is not tracked, so this run is opt-in even under TSGK_NATIVE_REQUIRED=1;
	// a set but mismatching grammar still fails against the registered closure below.
	grammarRoot := os.Getenv("TSGK_SVC_CSHARP_GRAMMAR")
	if grammarRoot == "" {
		t.Skip("set TSGK_SVC_CSHARP_GRAMMAR to the prepared registered C# grammar directory to run the SVC inline native regression (the grammar is not tracked)")
	}
	rt, cc := os.Getenv("TSGK_NATIVE_RUNTIME"), os.Getenv("TSGK_NATIVE_CC")
	if rt == "" || cc == "" {
		if os.Getenv("TSGK_NATIVE_REQUIRED") == "1" {
			t.Fatal("TSGK_SVC_CSHARP_GRAMMAR is set but the required native runtime or compiler is absent")
		}
		t.Skip("native runtime or compiler not configured")
	}
	read := func(name string) []byte {
		data, err := os.ReadFile("../../" + name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	svcContract, registry, inventory, fixture, pack := read(kit.SvcExpectedPath[len("src/"):]), read("contracts/query-semantic-registry-r1.json"), read("contracts/qualification-c1.json"),
		read("testdata/native/n461/svc.json"), read("contracts/fact-query-pack.json")
	e, err := kit.ParseSvcExpected(svcContract, registry, inventory, fixture)
	if err != nil {
		t.Fatal(err)
	}
	x, err := kit.ParseSvcInlineExpected(read(kit.SvcInlinePath[len("src/"):]), svcContract, registry, inventory, fixture, pack)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := kit.ParseQualificationInventory(inventory)
	if err != nil {
		t.Fatal(err)
	}
	var workload kit.QualWorkload
	for _, role := range parsed.ExtraRoles {
		if role.ID == "n461-svc" {
			workload = role.Workloads[0]
		}
	}
	for _, f := range workload.Grammar { // the registered closure, byte for byte
		data, err := os.ReadFile(filepath.Join(grammarRoot, filepath.FromSlash(f.Path)))
		if err != nil || uint64(len(data)) != f.Bytes || sha(data) != f.SHA256 {
			t.Fatalf("registered C# grammar file %s differs: %v", f.Path, err)
		}
	}
	factPack, err := kit.ParseFactPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	var queries []QuerySource
	for _, r := range factPack.Routes {
		if r.Route != "csharp" {
			continue
		}
		for _, q := range workload.Queries {
			for _, f := range r.Queries {
				if f.ID == q.ID {
					if sha([]byte(f.Source)) != q.SHA256 {
						t.Fatalf("query %s differs from the registered hash", q.ID)
					}
					queries = append(queries, QuerySource{ID: f.ID, Source: []byte(f.Source)})
				}
			}
		}
	}
	if len(queries) != 2 {
		t.Fatalf("%d queries", len(queries))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	compiler, err := os.ReadFile(cc)
	if err != nil {
		t.Fatal(err)
	}
	req := testBuildRequest(t.TempDir(), rt, grammarRoot, workload.Grammar, cc, kit.ToolIdentity{Name: "cc", Version: "host", SHA256: sha(compiler), Bytes: uint64(len(compiler))})
	req.Symbol = "tree_sitter_c_sharp"
	b, err := NewBuild(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	derived := map[string][]kit.SvcInlineStep{}
	var order []string
	for _, s := range x.Steps {
		if s.State == kit.SvcInlineDerived {
			if _, ok := derived[s.Case]; !ok {
				order = append(order, s.Case)
			}
			derived[s.Case] = append(derived[s.Case], s)
		}
	}
	op := kit.NativeOperations()["native-query"]
	var observed []kit.SvcInlineObserved
	for i, c := range e.Cases {
		steps := derived[c.ID]
		if len(steps) == 0 {
			continue
		}
		q := workload.Cases[i]
		t.Run(c.ID, func(t *testing.T) {
			cx := Context{Op: op, Route: "csharp", Output: kit.OutputTree, PolicyRef: PolicyRef(op, kit.OutputTree), Protocol: ProtocolR2, Queries: queries, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")}
			if !c.Reference {
				cx.Format, cx.SvcContext = kit.SvcFormat, &e.Workload.Context
			}
			versions, _, err := kit.ApplyEdits(kit.EncodingUTF8, []byte(*q.Source), q.Edits, op.InputBytes)
			if err != nil {
				t.Fatal(err)
			}
			cctx, ccancel := context.WithTimeout(ctx, 2*time.Minute)
			defer ccancel()
			r := b.RunCase(cctx, cx, kit.IncrementalCase{ID: c.ID, Encoding: kit.EncodingUTF8, Edits: q.Edits}, []byte(*q.Source))
			if r.ExecutionStatus != kit.StatusCompleted || len(r.Steps) != len(c.Steps) {
				t.Fatalf("%s: %s %s steps %d/%d", c.ID, r.ExecutionStatus, r.Code, len(r.Steps), len(c.Steps))
			}
			for _, s := range steps {
				sr := r.Steps[s.Step]
				o := kit.SvcInlineObserved{Case: c.ID, Step: s.Step, Source: versions[s.Step]}
				// a role the producer did not report is left out here; JudgeSvcInline blocks a step that needs it
				for _, r := range []struct {
					role string
					tree *TreeOut
				}{{kit.SvcInlineRoleIncremental, sr.Incremental}, {kit.SvcInlineRoleFresh, sr.Fresh}} {
					tree := r.tree
					if tree == nil {
						continue
					}
					if tree.Status != kit.StatusCompleted || tree.Tree == nil {
						t.Fatalf("%s step %d: no complete tree: %s %s", c.ID, s.Step, tree.Status, tree.Code)
					}
					v := kit.SvcInlineView{Role: r.role, Nodes: tree.Tree.Nodes, Queries: map[string]kit.SvcInlineQueryResult{}}
					for _, qo := range tree.Queries {
						v.Queries[qo.ID] = kit.SvcInlineQueryResult{Status: qo.Status, Captures: qo.Captures}
					}
					o.Views = append(o.Views, v)
				}
				if len(o.Views) == 0 {
					t.Fatalf("%s step %d: no tree view", c.ID, s.Step)
				}
				observed = append(observed, o)
			}
		})
	}
	if len(observed) != x.Derived {
		t.Fatalf("observed %d steps, derived %d", len(observed), x.Derived)
	}

	inline, err := kit.JudgeSvcInline(x, observed)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range inline {
		if j.State == "BLOCKED" && j.Code == kit.SvcInlineNativeNotRunCode {
			t.Errorf("%s: the native run did not report every tree role the step needs", j.ID)
		}
	}
	offline, err := kit.VerifySvcExpected(e, inventory)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := kit.MergeSvcJudgments(offline, inline)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range merged {
		if j.State == "FAILED" {
			t.Errorf("FAILED %s: %s", j.ID, j.Code)
		}
	}
	platform, _ := predicateHost()
	otherHost, registered := otherQueryHost(platform)
	if !registered {
		t.Fatalf("host %s is not a registered query host", platform)
	}
	coverage, err := kit.FoldSvcScope(e, platform, merged, map[string]bool{otherHost: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range coverage.Counters {
		t.Log(c.String())
	}
	t.Logf("completion %s full_scope_complete %t", coverage.Completion, coverage.FullScopeComplete)

	// forged native evidence must not verify: each mutation of the real native output fails the judgment
	first := observed[0]
	mutate := map[string]func(o kit.SvcInlineObserved) kit.SvcInlineObserved{
		"dropped node": func(o kit.SvcInlineObserved) kit.SvcInlineObserved {
			v := o.Views[0]
			v.Nodes = slices.Delete(slices.Clone(v.Nodes), len(v.Nodes)-1, len(v.Nodes))
			o.Views = append([]kit.SvcInlineView{v}, o.Views[1:]...)
			return o
		},
		"moved node": func(o kit.SvcInlineObserved) kit.SvcInlineObserved {
			v := o.Views[0]
			v.Nodes = slices.Clone(v.Nodes)
			v.Nodes[1].EndByte++
			o.Views = append([]kit.SvcInlineView{v}, o.Views[1:]...)
			return o
		},
		"dropped capture": func(o kit.SvcInlineObserved) kit.SvcInlineObserved {
			v := o.Views[0]
			v.Queries = map[string]kit.SvcInlineQueryResult{}
			for id, r := range o.Views[0].Queries {
				r.Captures = slices.Clone(r.Captures[:max(len(r.Captures)-1, 0)])
				v.Queries[id] = r
			}
			o.Views = append([]kit.SvcInlineView{v}, o.Views[1:]...)
			return o
		},
	}
	for name, f := range mutate {
		forged := slices.Clone(observed)
		forged[0] = f(first)
		js, err := kit.JudgeSvcInline(x, forged)
		if err != nil {
			t.Fatal(err)
		}
		failed := 0
		for _, j := range js {
			if j.State == "FAILED" {
				failed++
			}
		}
		if failed == 0 {
			t.Errorf("forged native evidence %q verified", name)
		}
	}
}
