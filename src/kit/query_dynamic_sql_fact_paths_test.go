package kit

import (
	"os"
	"slices"
	"testing"
)

type factPathCharge struct{ work, logical, sort uint64 }

func factPathAdjudication(t *testing.T, b *eligibilityBudget, route string, caps []Capture, src []byte) *Error {
	t.Helper()
	id := ordinaryTupleIdentity{Case: "fact-path", Query: "dynamic", TreeRole: "incremental", Source: OrdinaryPin{Path: "source.utf8", Bytes: uint64(len(src)), SHA256: digestHex(src)}}
	pack := FactPack{Schema: FactPackSchema, Revision: "facts-r1", Mapping: "mapping-r1", DynamicSQL: "dynamic-sql-r1", XML: "xml-structure-r1", Routes: []FactPackRoute{{Route: route, Declarations: []NativeDeclaration{}, Queries: []FactQuery{{ID: id.Query, Facts: FactDynamicSQL, Source: "(identifier) @x"}}}}}
	data := ordinaryJSON(t, pack)
	pack.SHA256 = digestHex(data)
	ref := FactPackRef{Revision: pack.Revision, SHA256: pack.SHA256, Route: route}
	mapping := []byte(`{"schema":"tsgk-fact-mapping/r1","revision":"mapping-r1"}`)
	inputs := ordinaryFactInputs{Pack: pack, PackData: data, PackRef: ref, PackPin: OrdinaryPin{Path: "pack.json", Bytes: uint64(len(data)), SHA256: pack.SHA256}, Mapping: mapping, MappingPin: ordinaryFactMappingPin{Schema: "tsgk-fact-mapping/r1", Revision: "mapping-r1", SHA256: digestHex(mapping)}, Ready: true}
	pin := ordinaryPackFactQueryPins(pack, route)[0]
	expected := ordinaryExpectedFact{Schema: ordinaryExpectedFactSchema, Case: id.Case, Query: id.Query, Step: id.Step, TreeRole: id.TreeRole, Source: id.Source, Pack: ref, Mapping: inputs.MappingPin, FactQuery: pin, Status: StatusCompleted}
	q := ordinaryFactBudgetQuery(caps)
	q.Status = StatusCompleted
	judgment := &ordinaryJudgmentBuilder{}
	if e := adjudicateOrdinaryFact(b, id, caps, src, q, ordinaryJSON(t, expected), inputs, judgment); e != nil {
		return e
	}
	// An omitted expected payload must be rejected after derivation; this probes accounting, not independent fact truth.
	if !judgment.failed || judgment.failureCode != "ORDINARY_EXPECTED_FACT_SHAPE" {
		t.Fatalf("fact payload changed: %+v", judgment)
	}
	return nil
}

func factPathSvc(caps []Capture, src []byte, facts DynamicSQLFacts, views int) (SvcInlineExpected, SvcInlineObserved) {
	caps = slices.Clone(caps)
	slices.SortStableFunc(caps, func(a, b Capture) int {
		if a.StartByte < b.StartByte {
			return -1
		}
		if a.StartByte > b.StartByte {
			return 1
		}
		return 0
	})
	q := siSummarize(svcInlineDynamicID, 1, siCapturesOf(caps))
	count := uint64(len(facts.Items))
	q.Facts = &count
	nodes := []TreeNode{{Parent: -1, Type: "compilation_unit", Named: true, EndByte: uint32(len(src))}}
	rows, _ := siRowsOfNodes(nodes)
	cst := siRowsDigest(rows)
	step := SvcInlineStep{Case: "fact-path", State: SvcInlineDerived, Fragment: SvcInlineFragment{EndByte: uint32(len(src)), Bytes: uint64(len(src)), SHA256: digestHex(src)}, CST: &cst, Queries: []SvcInlineQuery{q}}
	o := SvcInlineObserved{Case: step.Case, Source: src, Views: []SvcInlineView{{Role: SvcInlineRoleIncremental, Nodes: nodes, Queries: map[string]SvcInlineQueryResult{q.ID: {Status: StatusCompleted, Captures: caps}}}}}
	if views == 2 {
		step.Step, o.Step = 1, 1
		second := o.Views[0]
		second.Role = SvcInlineRoleFresh
		o.Views = append(o.Views, second)
	}
	x := SvcInlineExpected{Steps: []SvcInlineStep{step}, Queries: []SvcInlineQueryPin{{ID: q.ID, Patterns: 1}}}
	if views == 2 {
		x.Steps = append([]SvcInlineStep{{Case: step.Case, State: SvcInlineBlocked}}, x.Steps...)
	}
	return x, o
}

// Capture the ledger at the first adapter step, never from charges made after derivation.
func factPathLedger(t *testing.T, path, route string, caps []Capture, src []byte, facts DynamicSQLFacts, metered bool, views int) (pre factPathCharge, called bool, err error) {
	t.Helper()
	b := semanticTestBudget(t)
	var reserved factPathCharge
	oldCharge, oldStep := ordinaryChargeHook, factStepHook
	ordinaryChargeHook = func(ledger *eligibilityBudget, w, l uint64, at string, _, _ uint64) {
		if !metered && at == "ordinary-base-fact-index" {
			ledger.unmetered = true
		}
		if at == "ordinary-base-fact-index" || at == "ordinary-base-fact-derive" || at == "ordinary-base-fact-known-miss-sort" {
			reserved.work += w
			reserved.logical += l
			if at == "ordinary-base-fact-known-miss-sort" {
				reserved.sort += w
			}
		}
	}
	factStepHook = func(string, uint64) {
		if !called {
			pre = reserved
			called = true
		}
	}
	defer func() { ordinaryChargeHook, factStepHook = oldCharge, oldStep }()
	switch path {
	case "base":
		if e := b.ordinaryBaseFactReservation(FactQuery{Facts: FactDynamicSQL}, FactPackRoute{Route: route}, ordinaryFactBudgetQuery(caps), src); e != nil {
			err = e
		}
		pre = reserved
	case "adjudication":
		if e := factPathAdjudication(t, b, route, caps, src); e != nil {
			err = e
		}
	case "svc":
		x, o := factPathSvc(caps, src, facts, views)
		js, e := JudgeSvcInline(x, []SvcInlineObserved{o})
		err = e
		if e == nil {
			for _, j := range js {
				if j.State == "FAILED" {
					t.Fatalf("SVC verdict changed: %+v", j)
				}
			}
		}
	default:
		t.Fatalf("unknown fact path %q", path)
	}
	return pre, called, err
}

func assertFactPathReservations(t *testing.T, route string, caps []Capture, src []byte, facts DynamicSQLFacts, usedWork, usedLogical, usedSort uint64) {
	t.Helper()
	if len(src) == 0 {
		src = []byte(" ")
	}
	base, _, err := factPathLedger(t, "base", route, caps, src, facts, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"adjudication"}
	if route == "csharp" {
		paths = append(paths, "svc")
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			pre, called, err := factPathLedger(t, path, route, caps, src, facts, false, 1)
			if err != nil {
				t.Fatal(err)
			}
			if !called || pre != base {
				t.Errorf("pre-reservation %s=%+v base=%+v derivation=%v", path, pre, base, called)
			}
			if pre.work < usedWork || pre.logical < usedLogical || pre.sort < usedSort {
				t.Errorf("%s pre=%+v below measured work=%d logical=%d sort=%d", path, pre, usedWork, usedLogical, usedSort)
			}
		})
	}
}

func TestDynamicSQLFactPathsCSharp(t *testing.T) {
	g := csharpFixtureGen(t)
	r := csharpFactSteps(t, g.caps, g.src, EncodingUTF8)
	assertFactPathReservations(t, "csharp", g.caps, g.src, r.facts, 64*(r.pair+r.hops+r.decoded)+8*r.text, r.alloc, 0)
}

func TestDynamicSQLFactPathsTSQL(t *testing.T) {
	c := tsqlRepeatedErrorParent(EncodingUTF8)
	r := c.derive(t, EncodingUTF8)
	assertFactPathReservations(t, "tsql", c.caps, c.src, r.facts, 64*(r.steps+r.decoded+r.missSort), r.alloc, 64*r.missSort)
}

func TestDynamicSQLFactPathsRejectBeforeDerivation(t *testing.T) {
	for _, route := range []string{"csharp", "tsql"} {
		caps, src := advHugeSpans(EncodingUTF8, "zwsp", 1<<17, 8)
		if route == "tsql" {
			c := tsqlFactDuplicateExec(400, 400)
			caps, src = c.caps, c.src
		}
		facts, err := DeriveDynamicSQLChecked(route, EncodingUTF8, caps, src)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"adjudication", "svc"} {
			if path == "svc" && route != "csharp" {
				continue
			}
			t.Run(route+"/"+path, func(t *testing.T) {
				_, called, err := factPathLedger(t, path, route, caps, src, facts, true, 1)
				if e, ok := err.(*Error); !ok || e.Code != "ELIGIBILITY_WORK_LIMIT" || called {
					t.Fatalf("expected pre-derivation limit: called=%v err=%v", called, err)
				}
			})
		}
	}
}

func TestDynamicSQLFactPathsSvcAccumulatesViews(t *testing.T) {
	caps, src := advHugeSpans(EncodingUTF8, "zwsp", 50000, 1)
	r := csharpFactSteps(t, caps, src, EncodingUTF8)
	if _, called, err := factPathLedger(t, "svc", "csharp", caps, src, r.facts, true, 1); err != nil || !called {
		t.Fatalf("single view: called=%v err=%v", called, err)
	}
	_, called, err := factPathLedger(t, "svc", "csharp", caps, src, r.facts, true, 2)
	if e, ok := err.(*Error); !ok || e.Code != "ELIGIBILITY_WORK_LIMIT" || !called {
		t.Fatalf("second view should exceed shared limit: called=%v err=%v", called, err)
	}
}

func TestDynamicSQLFactPathLedgerChanges(t *testing.T) {
	g := csharpFixtureGen(t)
	sql, err := os.ReadFile("../testdata/native/dynamic-sql/tsql-dynamic.sql")
	if err != nil {
		t.Fatal(err)
	}
	d := &dynCaps{t: t, src: string(sql)}
	d.spExecutesql("EXEC sp_executesql N'SELECT 4 AS a'", "N'SELECT 4 AS a'")
	for _, c := range []struct {
		route string
		caps  []Capture
		src   []byte
	}{{"csharp", g.caps, g.src}, {"tsql", d.caps, sql}} {
		t.Run(c.route, func(t *testing.T) {
			facts, err := DeriveDynamicSQLChecked(c.route, EncodingUTF8, c.caps, c.src)
			if err != nil {
				t.Fatal(err)
			}
			pre, called, err := factPathLedger(t, "adjudication", c.route, c.caps, c.src, facts, true, 1)
			if err != nil || !called || pre.work == 0 || pre.logical == 0 {
				t.Fatalf("representative adjudication: pre=%+v called=%v err=%v", pre, called, err)
			}
			oldW, oldL := uint64(len(c.caps))+1, 256*uint64(len(c.caps))
			t.Logf("captures=%d adjudication fact work %d -> %d; logical %d -> %d; delta=%+v", len(c.caps), oldW, oldW+pre.work, oldL, oldL+pre.logical, pre)
		})
	}
	_, svc, registry, inventory, fixture, pack := svcInlineAssets(t)
	expected, err := DeriveSvcInline(svc, registry, inventory, fixture, pack)
	if err != nil {
		t.Fatal(err)
	}
	workload, err := svcInventoryWorkload(inventory)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range expected.Steps {
		if step.State != SvcInlineDerived {
			continue
		}
		ci := slices.IndexFunc(workload.Cases, func(c QualCase) bool { return c.ID == step.Case })
		if ci < 0 {
			t.Fatal("missing inline fixture case")
		}
		c := workload.Cases[ci]
		versions, _, err := ApplyEdits(EncodingUTF8, []byte(*c.Source), c.Edits, NativeOperations()["native-query-large"].InputBytes)
		if err != nil {
			t.Fatal(err)
		}
		fragment := versions[step.Step][step.Fragment.StartByte:step.Fragment.EndByte]
		x, o, _ := siSyntheticStep(t, string(fragment))
		old := ordinaryChargeHook
		var charge factPathCharge
		ordinaryChargeHook = func(_ *eligibilityBudget, w, l uint64, _ string, _, _ uint64) { charge.work += w; charge.logical += l }
		js, err := JudgeSvcInline(x, []SvcInlineObserved{o})
		ordinaryChargeHook = old
		if err != nil || charge.work == 0 || charge.logical == 0 {
			t.Fatalf("representative inline: charge=%+v err=%v", charge, err)
		}
		for _, j := range js {
			if j.State != "VERIFIED" {
				t.Fatalf("inline verdict: %+v", j)
			}
		}
		t.Logf("SVC fixture %s step=%d fragment=%q fact work 0 -> %d; logical 0 -> %d", step.Case, step.Step, string(fragment), charge.work, charge.logical)
		return
	}
	t.Fatal("no derived SVC fixture")
}
