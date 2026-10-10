package kit

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// anchorNodes is "call(x); call(y);" with two calls and an unnamed "call" token: the same
// type at two ranges, and another named type and an unnamed node on the first range.
func anchorNodes() []TreeNode {
	return []TreeNode{
		{Parent: -1, Type: "program", Named: true, EndByte: 17},
		{Parent: 0, Type: "call", Named: true, StartByte: 0, EndByte: 7},
		{Parent: 1, Type: "identifier", Named: true, StartByte: 0, EndByte: 4},
		{Parent: 0, Type: "call", Named: true, StartByte: 9, EndByte: 16},
		{Parent: 3, Type: "call", Named: false, StartByte: 9, EndByte: 13},
	}
}

func anchorTree(form string) *rcTree {
	t := &rcTree{Status: StatusCompleted, Form: form}
	if form == "full" {
		t.Tree = &rcFullTree{Nodes: anchorNodes()}
	}
	return t
}

// #76 P1: an anchor passes only on a named node of exactly its type and byte range; the
// same type elsewhere, another type on the range or an unnamed node fail, and a tree
// without nodes (summary or record form) blocks.
func TestExpectResultAnchors(t *testing.T) {
	at := func(ty string, s, e uint32) StepExpectation {
		return StepExpectation{Step: 0, Syntax: "NO_ERROR", Contains: []string{"call"}, Anchors: []ExpectAnchor{{Type: ty, StartByte: s, EndByte: e}}}
	}
	for name, tc := range map[string]struct {
		e     StepExpectation
		form  string
		want  string
		known bool
	}{
		"exact":            {at("call", 9, 16), "full", claimPass, true},
		"both-calls":       {StepExpectation{Syntax: "ANY", Anchors: []ExpectAnchor{{"call", 0, 7}, {"call", 9, 16}}}, "full", claimPass, true},
		"same-type-other":  {at("call", 9, 15), "full", claimFail, true},
		"other-type-range": {at("identifier", 9, 16), "full", claimFail, true},
		"unnamed-node":     {StepExpectation{Syntax: "ANY", Anchors: []ExpectAnchor{{"call", 9, 13}}}, "full", claimFail, true},
		"one-missing":      {StepExpectation{Syntax: "ANY", Anchors: []ExpectAnchor{{"call", 0, 7}, {"call", 1, 7}}}, "full", claimFail, true},
		"summary":          {StepExpectation{Syntax: "ANY", Anchors: []ExpectAnchor{{"call", 9, 16}}}, "summary", claimBlocked, true},
		"record":           {StepExpectation{Syntax: "ANY", Anchors: []ExpectAnchor{{"call", 9, 16}}}, "record", claimBlocked, true},
		"no-anchors":       {StepExpectation{Syntax: "ANY", Contains: []string{"call"}}, "full", claimPass, true},
	} {
		got, known := expectResult(tc.e, anchorTree(tc.form))
		if got != tc.want || known != tc.known {
			t.Errorf("%s: %s %v, want %s %v", name, got, known, tc.want, tc.known)
		}
	}
	// a full tree whose nodes were not recorded is unknown, never a pass
	tr := anchorTree("full")
	tr.Tree = nil
	if got, known := expectResult(StepExpectation{Syntax: "ANY", Anchors: []ExpectAnchor{{"call", 9, 16}}}, tr); known || got != "" {
		t.Fatalf("full tree without nodes: %s %v", got, known)
	}
}

// anchorProfile is a profile with one edited case ("call(x);" -> "call(xy);", 8 then 9
// bytes) whose step expectations carry the given anchors.
func anchorProfile(schema string, step0, step1 any) map[string]any {
	e0 := map[string]any{"step": 0, "syntax": "NO_ERROR", "contains": []string{}, "declarations": ""}
	e1 := map[string]any{"step": 1, "syntax": "NO_ERROR", "contains": []string{}, "declarations": ""}
	if step0 != nil {
		e0["anchors"] = step0
	}
	if step1 != nil {
		e1["anchors"] = step1
	}
	p := map[string]any{"schema": schema, "id": "a", "route": "owned-plain", "operation": "native-parse-edit", "symbol": "tree_sitter_fx", "encoding": "UTF-8", "output": "tree",
		"compiler": map[string]any{"name": "cc", "version": "1", "sha256": fxCompiler, "bytes": 1}, "grammar": []any{map[string]any{"path": "src/parser.c", "role": "parser", "sha256": fxCompiler, "bytes": 1}},
		"declarations": nil, "cases": []any{map[string]any{"id": "c1", "input": map[string]any{"path": "cases/c1.txt", "role": "case", "sha256": fxCompiler, "bytes": 8},
			"edits": []any{map[string]any{"start_byte": 6, "old_end_byte": 6, "new_end_byte": 7, "old": "", "new": "eQ=="}}, "points": []any{}, "expect": []any{e0, e1}}}}
	if strings.HasPrefix(schema, "tsgk-oracle/") {
		p["operation"], p["queries"], p["fact_pack"], p["api"] = "native-query", []any{}, nil, false
		for _, c := range p["cases"].([]any) {
			c.(map[string]any)["query_expect"], c.(map[string]any)["dynamic_sql_expect"] = []any{}, nil
		}
	}
	return p
}

func anchor(ty string, s, e any) map[string]any {
	return map[string]any{"type": ty, "start_byte": s, "end_byte": e}
}

func parseProfileOf(schema string, p map[string]any) (IncrementalProfile, *Error) {
	data, _ := json.Marshal(p)
	if strings.HasPrefix(schema, "tsgk-oracle/") {
		o, e := parseOracle(data)
		return o.Native, e
	}
	return parseIncremental(data)
}

// #76 P1: incremental r2 and oracle r2 take anchors and keep them; r1 still decodes for
// replay but has no anchors member; an anchor with an empty or malformed type, an end
// before its start, an end beyond its step's source (8 bytes at step 0, 9 after the edit)
// or too many anchors is EXPECT_ANCHOR_INVALID.
func TestProfileAnchors(t *testing.T) {
	for _, pair := range [][2]string{{IncrementalSchema, incrementalSchemaR1}, {OracleSchema, oracleSchemaR1}} {
		cur, old := pair[0], pair[1]
		p, e := parseProfileOf(cur, anchorProfile(cur, []any{anchor("call", 0, 8), anchor("x", 5, 5)}, []any{anchor("call", 0, 9)}))
		if e != nil {
			t.Fatalf("%s: %v", cur, e)
		}
		if p.Schema != cur || !slices.Equal(p.Cases[0].Expect[0].Anchors, []ExpectAnchor{{"call", 0, 8}, {"x", 5, 5}}) ||
			!slices.Equal(p.Cases[0].Expect[1].Anchors, []ExpectAnchor{{"call", 0, 9}}) {
			t.Fatalf("%s: anchors %+v", cur, p.Cases[0].Expect)
		}
		if p, e := parseProfileOf(old, anchorProfile(old, nil, nil)); e != nil || p.Schema != old || p.Cases[0].Expect[0].Anchors != nil {
			t.Fatalf("%s without anchors: %v %+v", old, e, p)
		}
		if _, e := parseProfileOf(old, anchorProfile(old, []any{anchor("call", 0, 8)}, nil)); e == nil || e.Code != "JSON_UNKNOWN_FIELD" {
			t.Fatalf("%s with anchors: %v", old, e)
		}
		many := []any{}
		for i := 0; i <= MaxExpectAnchors; i++ {
			many = append(many, anchor("call", 0, 1))
		}
		for name, tc := range map[string][2]any{
			"empty-type":      {[]any{anchor("", 0, 1)}, nil},
			"control-type":    {[]any{anchor("ca\tll", 0, 1)}, nil},
			"end-before":      {[]any{anchor("call", 3, 2)}, nil},
			"beyond-step0":    {[]any{anchor("call", 0, 9)}, nil},
			"beyond-step1":    {nil, []any{anchor("call", 0, 10)}},
			"beyond-uint32":   {[]any{anchor("call", 0, uint64(1)<<32)}, nil},
			"too-many":        {many, nil},
			"start-past-end8": {[]any{anchor("call", 9, 9)}, nil},
		} {
			if _, e := parseProfileOf(cur, anchorProfile(cur, tc[0], tc[1])); e == nil || e.Code != "EXPECT_ANCHOR_INVALID" {
				t.Errorf("%s %s: %v", cur, name, e)
			}
		}
		if _, e := parseProfileOf(cur, anchorProfile(cur, []any{map[string]any{"type": "call", "start_byte": 0}}, nil)); e == nil || e.Code != "JSON_MISSING_FIELD" {
			t.Errorf("%s missing end_byte: %v", cur, e)
		}
	}
}

// #76 P1: inventory r3 anchors are checked like the profile's against the case input bytes
// and edits (CASE_INVALID); valid anchors and none at all are accepted.
func TestInventoryAnchors(t *testing.T) {
	set := func(i int, as ...ExpectAnchor) func(inv *QualificationInventory) {
		return func(inv *QualificationInventory) {
			c := &inv.Routes[0].Workload.Cases[i]
			if len(c.Expect) == 0 { // c2 ("beta", one same-length edit): anchor its step 1
				c.Expect = []StepExpectation{{Step: 1, Syntax: "ANY", Contains: []string{}}}
				c.Expect[0].Anchors = as
				return
			}
			c.Expect[0].Anchors = as
		}
	}
	for name, mut := range map[string]func(inv *QualificationInventory){
		"empty-type":   set(0, ExpectAnchor{"", 0, 5}),
		"end-before":   set(0, ExpectAnchor{"word", 3, 2}),
		"beyond-input": set(0, ExpectAnchor{"word", 0, 6}),
		"beyond-step1": set(1, ExpectAnchor{"word", 0, 5}),
	} {
		expectCode(t, name, "CASE_INVALID", mut)
	}
	f := newQfx(t)
	set(0, ExpectAnchor{"word", 0, 5}, ExpectAnchor{"doc", 5, 5})(&f.inv)
	set(1, ExpectAnchor{"word", 0, 4})(&f.inv)
	inv, err := ParseQualificationInventory(f.invBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := inv.Routes[0].Workload.Cases[0].Expect[0].Anchors; !slices.Equal(got, []ExpectAnchor{{"word", 0, 5}, {"doc", 5, 5}}) {
		t.Fatalf("anchors %v", got)
	}
}

// #76 P1: replay recomputes anchored expectations from the recorded tree and the workload
// registration: a recorded result the tree does not support, or recorded anchors other than
// the registered ones, is EXPECTATION_MISMATCH.
func TestReplayAnchors(t *testing.T) {
	anchored := func(reg, rec []ExpectAnchor, result string) *fxNative {
		f := newFxNative()
		f.profile["schema"] = IncrementalSchema
		f.profile["cases"].([]any)[0].(map[string]any)["expect"].([]any)[0].(map[string]any)["anchors"] = reg
		x := f.cases[0]["expectations"].([]any)[0].(map[string]any)
		x["anchors"], x["result"] = rec, result
		if result != "PASS" {
			f.cases[0]["claims"].(map[string]string)["expectations"], f.cases[0]["assessment"], f.cases[0]["code"] = result, result, "EXPECTATION_FAILED"
			f.top["assessment"] = result
			f.top["summary"].(map[string]any)["assessments"] = map[string]int{"PASS": 1, result: 1}
			f.top["summary"].(map[string]any)["codes"] = map[string]int{"EXPECTATION_FAILED": 1}
		}
		return f
	}
	word := []ExpectAnchor{{"word", 0, 5}}
	root, reg := anchored(word, word, "PASS").write(t, nil)
	r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	if err != nil || r.Assessment != AssessPass || !r.EvidenceValid {
		t.Fatalf("anchored replay: %v %s %v %v", err, r.Assessment, r.EvidenceValid, r.Findings)
	}
	shifted := []ExpectAnchor{{"word", 1, 5}}
	replayFails(t, anchored(shifted, shifted, "PASS"), nil, "EXPECTATION_MISMATCH") // the tree has no word at [1,5)
	replayFails(t, anchored(word, shifted, "PASS"), nil, "EXPECTATION_MISMATCH")    // recorded anchors are not the registered ones
	replayFails(t, anchored(word, nil, "PASS"), nil, "EXPECTATION_MISMATCH")
	// a FAIL recorded for a missing anchor replays as that FAIL
	root, reg = anchored(shifted, shifted, "FAIL").write(t, func(root string, reg map[string]any) {
		reg["subject"].(map[string]any)["assessment"] = "FAIL"
	})
	r, err = Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	if err != nil || r.Recomputed.Assessment != AssessFail || !r.EvidenceValid {
		t.Fatalf("recorded anchor FAIL: %v %+v %v %v", err, r.Recomputed, r.EvidenceValid, r.Findings)
	}
}

// #76 P1: qualify judges an anchored P step from the recorded tree (expectResult): the
// registered anchor on the recorded word passes; one on another range fails the P
// obligation even when the record says PASS.
func TestQualifyAnchors(t *testing.T) {
	run := func(a ExpectAnchor) QualificationResult {
		f := newQfx(t)
		f.inv.Routes[0].Workload.Cases[0].Expect[0].Anchors = []ExpectAnchor{a}
		echo := qfxMut{record: func(set string, recs []map[string]any) {
			if set == "s06-fxa" {
				recs[0]["expectations"].([]any)[0].(map[string]any)["anchors"] = []ExpectAnchor{a}
			}
		}}
		return f.run(t, f.all(t, qfxAll(echo)))
	}
	r := run(ExpectAnchor{"word", 0, 5})
	if c := cellOf(r, "fxa", "windows-amd64"); c.Status != CellPass || obligation(c, "fxa-B01", "P").Result != claimPass {
		t.Fatalf("anchored P: %s %+v %v", c.Status, obligation(c, "fxa-B01", "P"), setCodes(c))
	}
	r = run(ExpectAnchor{"word", 1, 5})
	c := cellOf(r, "fxa", "windows-amd64")
	if c.Status == CellPass || obligation(c, "fxa-B01", "P").Result != claimFail {
		t.Fatalf("misplaced anchor: %s %+v %v", c.Status, obligation(c, "fxa-B01", "P"), setCodes(c))
	}
	// a host profile whose anchors differ from the inventory's is another registration
	f := newQfx(t)
	f.inv.Routes[0].Workload.Cases[0].Expect[0].Anchors = []ExpectAnchor{{"word", 0, 5}}
	drop := qfxMut{profile: func(set string, prof map[string]any) {
		if set == "s06-fxa" {
			expect := prof["cases"].([]any)[0].(map[string]any)["expect"].([]any)
			e := expect[0].(StepExpectation)
			e.Anchors = []ExpectAnchor{{"word", 0, 4}}
			expect[0] = e
		}
	}}
	r = f.run(t, f.all(t, qfxAll(drop)))
	if c := cellOf(r, "fxa", "windows-amd64"); c.Status == CellPass || !slices.Contains(setCodes(c), "REGISTRATION_MISMATCH") {
		t.Fatalf("profile anchors other than the inventory's: %s %v", c.Status, setCodes(c))
	}
}
