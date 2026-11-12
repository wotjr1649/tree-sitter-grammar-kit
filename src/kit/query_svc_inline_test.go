package kit

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

func svcInlineAssets(t *testing.T) (asset, svcContract, registry, inventory, fixture, pack []byte) {
	t.Helper()
	read := func(name string) []byte {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	return read("../contracts/query-svc-inline-expected-r1.json"), read("../contracts/query-svc-expected-r1.json"), read("../contracts/query-semantic-registry-r1.json"),
		read("../contracts/qualification-c1.json"), read("../testdata/native/n461/svc.json"), read("../contracts/fact-query-pack.json")
}

func TestSvcInlineSealedAndBound(t *testing.T) {
	asset, svc, registry, inventory, fixture, pack := svcInlineAssets(t)
	if len(asset) != 76826 || digestHex(asset) != "f2ad5d41e7852d6f5f80c6727a0d53381ccb2e5f2825b554270d1f17d03dcbb7" {
		t.Fatalf("sealed inline expected asset changed: %d/%s", len(asset), digestHex(asset))
	}
	x, err := ParseSvcInlineExpected(asset, svc, registry, inventory, fixture, pack)
	if err != nil {
		t.Fatal(err)
	}
	e, err := ParseSvcExpected(svc, registry, inventory, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if x.Derived != 30 || x.Blocked != 52 || len(x.Steps) != 82 {
		t.Fatalf("derived %d blocked %d steps %d", x.Derived, x.Blocked, len(x.Steps))
	}
	codes := map[string]int{}
	for _, s := range x.Steps {
		codes[s.Code]++
	}
	if codes[""] != 30 || codes[SvcInlineErrorRecoveryCode] != 8 || codes[SvcInlinePreprocCode] != 44 || codes[SvcInlineSubsetCode] != 0 {
		t.Fatalf("codes %v", codes)
	}
	// the steps are exactly the inline obligations of the SVC denominator, in order
	var want []string
	for _, o := range e.Obligations() {
		if o.Kind == SvcKindInlineTree {
			want = append(want, o.ID)
		}
	}
	var got []string
	for _, s := range x.Steps {
		got = append(got, svcInlineID(SvcKindInlineTree, s, ""))
	}
	if !slices.Equal(want, got) {
		t.Fatal("inline steps differ from the inline tree obligations")
	}
}

// Every change of the asset is rejected: it must equal the derivation of the pinned sources.
func TestSvcInlineAssetForgeries(t *testing.T) {
	asset, svc, registry, inventory, fixture, pack := svcInlineAssets(t)
	text := string(asset)
	forge := map[string]string{
		"cst digest":     strings.Replace(text, `"rows_sha256": "`, `"rows_sha256": "0`, 1),
		"capture count":  strings.Replace(text, `"captures": 3`, `"captures": 4`, 1),
		"state":          strings.Replace(text, `"state": "BLOCKED"`, `"state": "DERIVED"`, 1),
		"code":           strings.Replace(text, SvcInlinePreprocCode, SvcInlineErrorRecoveryCode, 1),
		"schema":         strings.Replace(text, SvcInlineSchema, "x", 1),
		"unknown member": strings.Replace(text, `"derived":`, `"extra": 1, "derived":`, 1),
		"grammar":        strings.Replace(text, "704882b5", "804882b5", 1),
		"pin":            strings.Replace(text, `"bytes": `, `"bytes": 1`, 1),
	}
	for name, forged := range forge {
		if forged == text {
			t.Fatalf("%s: forgery did not change the asset", name)
		}
		if _, err := ParseSvcInlineExpected([]byte(forged), svc, registry, inventory, fixture, pack); err == nil {
			t.Errorf("forged asset %q parsed", name)
		}
	}
	// a changed pinned source is rejected too
	other := append([]byte{}, pack...)
	other = append(other, ' ')
	if _, err := ParseSvcInlineExpected(asset, svc, registry, inventory, fixture, other); err == nil {
		t.Error("asset parsed against a different fact pack")
	}
}

func siTypes(rows []LargeRow) []string {
	var out []string
	for _, r := range rows {
		s := r.Type
		if r.Field != "" {
			s = r.Field + ":" + s
		}
		out = append(out, s)
	}
	return out
}

// Hand-derived from the grammar rules: `class S {}` after a newline at byte 44 of row 0.
func TestSvcInlineHandDerivedClass(t *testing.T) {
	f := SvcInlineFragment{StartByte: 44, EndByte: 55, StartPoint: [2]uint32{0, 44}, EndPoint: [2]uint32{1, 10}}
	tree, err := siDerive([]byte("\nclass S {}"), f)
	if err != nil {
		t.Fatal(err)
	}
	want := []LargeRow{
		{0, -1, 45, 55, 1, 0, 1, 10, LargeRowNamed, "compilation_unit", "", 1},
		{1, 0, 45, 55, 1, 0, 1, 10, LargeRowNamed, "class_declaration", "", 2},
		{2, 1, 45, 50, 1, 0, 1, 5, 0, "class", "", 3},
		{3, 1, 51, 52, 1, 6, 1, 7, LargeRowNamed, "identifier", "name", 3},
		{4, 1, 53, 55, 1, 8, 1, 10, LargeRowNamed, "declaration_list", "body", 3},
		{5, 4, 53, 54, 1, 8, 1, 9, 0, "{", "", 4},
		{6, 4, 54, 55, 1, 9, 1, 10, 0, "}", "", 4},
	}
	if !slices.Equal(tree.rows, want) {
		t.Fatalf("rows %+v", tree.rows)
	}
	caps := siRunQuery(mustQuery(t, "(class_declaration) @c.0.0.0\n(class_declaration name: _ @c.0.1.1) @c.0.1.0\n(struct_declaration) @c.1.0.0\n"), tree.root)
	if len(caps) != 3 || caps[0].Pattern != 0 || caps[1].Pattern != 1 || caps[2].Pattern != 1 {
		t.Fatalf("captures %+v", caps)
	}
	names := []string{caps[0].Name, caps[1].Name, caps[2].Name}
	slices.Sort(names)
	if !slices.Equal(names, []string{"c.0.0.0", "c.0.1.0", "c.0.1.1"}) {
		t.Fatalf("names %v", names)
	}
}

func mustQuery(t *testing.T, src string) []siQPattern {
	t.Helper()
	p, err := siParseQuery(src)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Hand-derived: a leading comment is a root child, an internal one a child of the innermost node,
// the qualified name is left-associative, and a named attribute argument keeps its field.
func TestSvcInlineHandDerivedShapes(t *testing.T) {
	src := "// h\nusing A.B.C;\n[X(N = \"a\\\"b\")]\nclass K : I {\n  // c\n  int f = 1 + n.m;\n}\n"
	f := SvcInlineFragment{EndByte: uint32(len(src)), EndPoint: [2]uint32{7, 0}}
	tree, err := siDerive([]byte(src), f)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Fields(`compilation_unit comment using_directive using qualified_name qualifier:qualified_name qualifier:identifier . name:identifier . name:identifier ;
		class_declaration attribute_list [ attribute name:identifier attribute_argument_list ( attribute_argument name:identifier = string_literal " string_literal_content escape_sequence string_literal_content " ) ] class name:identifier
		base_list : identifier body:declaration_list { comment field_declaration variable_declaration type:predefined_type variable_declarator name:identifier =
		binary_expression left:integer_literal operator:+ right:member_access_expression expression:identifier . name:identifier ; }`)
	// the field declaration holds modifiers-less children; its tokens follow the grammar order
	got := siTypes(tree.rows)
	if !slices.Equal(got, want) {
		t.Fatalf("types\n%v\nwant\n%v", got, want)
	}
}

func TestSvcInlineRefusals(t *testing.T) {
	cases := []struct {
		name, src, code string
	}{
		{"valid", "class C {}", ""},
		{"directive", "#if A\nclass C {}\n#endif\n", SvcInlinePreprocCode},
		{"indented directive", "class C {}\n  #define X\n", SvcInlinePreprocCode},
		{"unexpected token", "class C { int M() { return text)); } }", SvcInlineErrorRecoveryCode},
		{"keyword as expression", "class C { int M() { return return 1; } }", SvcInlineErrorRecoveryCode},
		{"leading operator", ">\nclass C {}", SvcInlineErrorRecoveryCode},
		{"unterminated string", "\";", SvcInlineErrorRecoveryCode},
		{"invalid escape", "class C { string s = \"\\q\"; }", SvcInlineErrorRecoveryCode},
		{"block comment", "class C { /* x */ }", SvcInlineSubsetCode},
		{"non-ascii identifier", "class Ünï {}", SvcInlineSubsetCode},
		{"empty", " \n", SvcInlineSubsetCode},
	}
	for _, c := range cases {
		f := SvcInlineFragment{EndByte: uint32(len(c.src)), EndPoint: [2]uint32{uint32(strings.Count(c.src, "\n")), 0}}
		_, err := siDerive([]byte(c.src), f)
		var se *siError
		switch {
		case c.code == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.code != "" && !errors.As(err, &se):
			t.Errorf("%s: derived, want %s", c.name, c.code)
		case c.code != "" && svcInlineCode(se) != c.code:
			t.Errorf("%s: %s (%v), want %s", c.name, svcInlineCode(se), err, c.code)
		}
	}
}

func TestSvcInlineQueryMatcher(t *testing.T) {
	leaf := func(typ, field string, named bool, start uint32) *siNode {
		n := &siNode{typ: typ, field: field, start: start, end: start + 1}
		if named {
			n.flags = LargeRowNamed
		}
		return n
	}
	a1, a2 := leaf("argument", "", true, 1), leaf("argument", "", true, 3)
	list := &siNode{typ: "argument_list", flags: LargeRowNamed, kids: []*siNode{leaf("(", "", false, 0), a1, leaf(",", "", false, 2), a2, leaf(")", "", false, 4)}, end: 5}
	count := func(q string) int { return len(siRunQuery(mustQuery(t, q), list)) }
	if n := count("(argument_list . (argument) @a)"); n != 1 {
		t.Errorf("anchored first named child: %d captures", n)
	}
	if n := count("(argument_list (argument) @a)"); n != 2 {
		t.Errorf("unanchored: %d captures", n)
	}
	if n := count("(argument_list (_) @a) @l"); n != 4 { // two named children and the list itself, twice
		t.Errorf("named wildcard: %d captures", n)
	}
	if n := count("(argument_list _ @a)"); n != 5 {
		t.Errorf("wildcard sees anonymous tokens: %d captures", n)
	}
	bin := &siNode{typ: "binary_expression", flags: LargeRowNamed, kids: []*siNode{leaf("identifier", "left", true, 0), leaf("+", "operator", false, 1), leaf("identifier", "right", true, 2)}, end: 3}
	if caps := siRunQuery(mustQuery(t, "(binary_expression operator: _ @op) @b"), bin); len(caps) != 2 || caps[0].Name != "op" && caps[1].Name != "op" {
		t.Errorf("field wildcard: %+v", caps)
	}
	if caps := siRunQuery(mustQuery(t, "(binary_expression left: (identifier) @l right: (integer_literal) @r)"), bin); len(caps) != 0 {
		t.Errorf("a type mismatch must not match: %+v", caps)
	}
	for _, bad := range []string{"(a [b c])", "(a (#eq? @x \"y\"))", "((a) (b))"} {
		if _, err := siParseQuery(bad); err == nil {
			t.Errorf("query syntax outside the subset accepted: %s", bad)
		}
	}
}

// siSyntheticStep builds an expectation for one derived fragment and the evidence a faithful native
// producer would report for it. This checks the judge only; it is not native evidence.
func siSyntheticStep(t *testing.T, src string) (SvcInlineExpected, SvcInlineObserved, []string) {
	t.Helper()
	_, _, _, _, _, pack := svcInlineAssets(t)
	p, err := ParseFactPack(pack)
	if err != nil {
		t.Fatal(err)
	}
	var ids, sources []string
	for _, r := range p.Routes {
		if r.Route == "csharp" {
			for _, q := range r.Queries {
				ids, sources = append(ids, q.ID), append(sources, q.Source)
			}
		}
	}
	f := SvcInlineFragment{EndByte: uint32(len(src)), EndPoint: [2]uint32{uint32(strings.Count(src, "\n")), 0}}
	tree, err := siDerive([]byte(src), f)
	if err != nil {
		t.Fatal(err)
	}
	cst := siRowsDigest(tree.rows)
	step := SvcInlineStep{Case: "synthetic", Step: 0, State: SvcInlineDerived, Fragment: f, CST: &cst}
	view := SvcInlineView{Role: SvcInlineRoleIncremental, Queries: map[string]SvcInlineQueryResult{}}
	for _, r := range tree.rows {
		n := TreeNode{Parent: int64(r.Parent), Type: r.Type, Named: r.Flags&LargeRowNamed != 0, Extra: r.Flags&LargeRowExtra != 0, StartByte: r.StartByte, EndByte: r.EndByte,
			StartPoint: Point{Row: r.StartRow, Column: r.StartCol}, EndPoint: Point{Row: r.EndRow, Column: r.EndCol}}
		if r.Field != "" {
			field := r.Field
			n.Field = &field
		}
		view.Nodes = append(view.Nodes, n)
	}
	x := SvcInlineExpected{Steps: []SvcInlineStep{step}}
	for q, id := range ids {
		pat := mustQuery(t, sources[q])
		caps := siRunQuery(pat, tree.root)
		sum := siSummarize(id, len(pat), caps)
		if id == svcInlineDynamicID {
			zero := uint64(0)
			sum.Facts = &zero
		}
		x.Steps[0].Queries = append(x.Steps[0].Queries, sum)
		x.Queries = append(x.Queries, SvcInlineQueryPin{ID: id, Patterns: len(pat)})
		res := SvcInlineQueryResult{Status: StatusCompleted}
		slices.SortStableFunc(caps, func(a, b SvcInlineCapture) int { return int(a.StartByte) - int(b.StartByte) })
		for _, c := range caps {
			res.Captures = append(res.Captures, Capture{Pattern: c.Pattern, Name: c.Name, Type: c.Type, StartByte: c.StartByte, EndByte: c.EndByte,
				StartPoint: Point{Row: c.StartRow, Column: c.StartCol}, EndPoint: Point{Row: c.EndRow, Column: c.EndCol}})
		}
		view.Queries[id] = res
	}
	obs := SvcInlineObserved{Case: "synthetic", Step: 0, Source: []byte(src), Views: []SvcInlineView{view}}
	return x, obs, ids
}

func siVerdicts(t *testing.T, x SvcInlineExpected, obs ...SvcInlineObserved) map[string]SvcJudgment {
	t.Helper()
	js, err := JudgeSvcInline(x, obs)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]SvcJudgment{}
	for _, j := range js {
		out[j.ID] = j
	}
	return out
}

// Evidence that differs from the expectation in any way is FAILED with the exact code, and the
// untouched evidence is VERIFIED (so every failure below is caused by its own mutation).
func TestSvcInlineJudgeCounterexamples(t *testing.T) {
	src := "using System;\n// c\nclass K\n{\n    string M(string s)\n    {\n        return \"a\" + s + \"é\";\n    }\n}\n"
	x, obs, ids := siSyntheticStep(t, src)
	treeID, declID, dynID := svcID(SvcKindInlineTree, "synthetic", 0, ""), svcID(SvcKindQuery, "synthetic", 0, ids[0]), svcID(SvcKindQuery, "synthetic", 0, ids[1])
	for id, j := range siVerdicts(t, x, obs) {
		if j.State != "VERIFIED" {
			t.Fatalf("faithful evidence: %s %s %s", id, j.State, j.Code)
		}
	}
	clone := func(o SvcInlineObserved) SvcInlineObserved {
		v := o.Views[0]
		v.Nodes = slices.Clone(v.Nodes)
		v.Queries = map[string]SvcInlineQueryResult{}
		for id, r := range o.Views[0].Queries {
			r.Captures = slices.Clone(r.Captures)
			v.Queries[id] = r
		}
		o.Views = []SvcInlineView{v}
		return o
	}
	type mutation struct {
		id, code string
		edit     func(o *SvcInlineObserved)
	}
	last := func(o *SvcInlineObserved) *SvcInlineView { return &o.Views[0] }
	str := func(s string) *string { return &s }
	muts := map[string]mutation{
		"dropped node":    {treeID, "SVC_INLINE_CST_NODES", func(o *SvcInlineObserved) { v := last(o); v.Nodes = v.Nodes[:len(v.Nodes)-1] }},
		"extra node":      {treeID, "SVC_INLINE_CST_NODES", func(o *SvcInlineObserved) { v := last(o); v.Nodes = append(v.Nodes, v.Nodes[len(v.Nodes)-1]) }},
		"empty tree":      {treeID, "SVC_INLINE_CST_EMPTY", func(o *SvcInlineObserved) { last(o).Nodes = nil }},
		"type":            {treeID, "SVC_INLINE_CST_DIGEST", func(o *SvcInlineObserved) { last(o).Nodes[3].Type += "x" }},
		"field":           {treeID, "SVC_INLINE_CST_DIGEST", func(o *SvcInlineObserved) { last(o).Nodes[3].Field = str("other") }},
		"range":           {treeID, "SVC_INLINE_CST_DIGEST", func(o *SvcInlineObserved) { last(o).Nodes[4].EndByte++ }},
		"point":           {treeID, "SVC_INLINE_CST_DIGEST", func(o *SvcInlineObserved) { last(o).Nodes[4].StartPoint.Column++ }},
		"parent":          {treeID, "SVC_INLINE_CST_DIGEST", func(o *SvcInlineObserved) { last(o).Nodes[3].Parent = 0 }},
		"named flag":      {treeID, "SVC_INLINE_CST_DIGEST", func(o *SvcInlineObserved) { last(o).Nodes[2].Named = !last(o).Nodes[2].Named }},
		"extra flag":      {treeID, "SVC_INLINE_CST_DIGEST", func(o *SvcInlineObserved) { last(o).Nodes[2].Extra = true }},
		"root error flag": {treeID, "SVC_INLINE_CST_ERROR", func(o *SvcInlineObserved) { last(o).Nodes[0].HasError = true }},
		"query not done": {declID, "SVC_INLINE_QUERY_NOT_COMPLETED", func(o *SvcInlineObserved) {
			r := last(o).Queries[ids[0]]
			r.Status = StatusFailed
			last(o).Queries[ids[0]] = r
		}},
		"query missing": {declID, "SVC_INLINE_QUERY_NOT_COMPLETED", func(o *SvcInlineObserved) { delete(last(o).Queries, ids[0]) }},
		"dropped capture": {declID, "SVC_INLINE_QUERY_COUNT", func(o *SvcInlineObserved) {
			r := last(o).Queries[ids[0]]
			r.Captures = r.Captures[1:]
			last(o).Queries[ids[0]] = r
		}},
		"extra capture": {declID, "SVC_INLINE_QUERY_COUNT", func(o *SvcInlineObserved) {
			r := last(o).Queries[ids[0]]
			r.Captures = append(r.Captures, r.Captures[len(r.Captures)-1])
			last(o).Queries[ids[0]] = r
		}},
		"renamed capture": {declID, "SVC_INLINE_QUERY_NAMES", func(o *SvcInlineObserved) {
			r := last(o).Queries[ids[0]]
			r.Captures[0].Name = "c.99.0.0"
			last(o).Queries[ids[0]] = r
		}},
		"capture range": {declID, "SVC_INLINE_QUERY_DIGEST", func(o *SvcInlineObserved) {
			r := last(o).Queries[ids[0]]
			r.Captures[2].EndByte++
			last(o).Queries[ids[0]] = r
		}},
		"capture pattern": {declID, "SVC_INLINE_QUERY_DIGEST", func(o *SvcInlineObserved) {
			r := last(o).Queries[ids[0]]
			r.Captures[0].Pattern += 2
			last(o).Queries[ids[0]] = r
		}},
		"capture type": {declID, "SVC_INLINE_QUERY_DIGEST", func(o *SvcInlineObserved) {
			r := last(o).Queries[ids[0]]
			r.Captures[1].Type = "x"
			last(o).Queries[ids[0]] = r
		}},
		"capture order": {declID, "SVC_INLINE_QUERY_ORDER", func(o *SvcInlineObserved) {
			r := last(o).Queries[ids[0]]
			slices.Reverse(r.Captures)
			last(o).Queries[ids[0]] = r
		}},
		"dynamic capture": {dynID, "SVC_INLINE_QUERY_COUNT", func(o *SvcInlineObserved) {
			r := last(o).Queries[ids[1]]
			r.Captures = r.Captures[:len(r.Captures)-1]
			last(o).Queries[ids[1]] = r
		}},
	}
	for name, m := range muts {
		o := clone(obs)
		m.edit(&o)
		got := siVerdicts(t, x, o)
		if j := got[m.id]; j.State != "FAILED" || j.Code != m.code {
			t.Errorf("%s: %s %s, want FAILED %s", name, j.State, j.Code, m.code)
		}
	}
	// the fresh view of an edited step (step 1 follows inline step 0) must match as well
	next := x.Steps[0]
	next.Step = 1
	edited := x
	edited.Steps = append(slices.Clone(x.Steps), next)
	o := clone(obs)
	o.Step = 1
	bad := clone(obs).Views[0]
	bad.Role = SvcInlineRoleFresh
	bad.Nodes[2].EndByte++
	o.Views = append(o.Views, bad)
	if j := siVerdicts(t, edited, obs, o)[svcID(SvcKindInlineTree, "synthetic", 1, "")]; j.State != "FAILED" {
		t.Errorf("a diverging fresh view verified: %+v", j)
	}
	// no evidence is BLOCKED, never VERIFIED
	for id, j := range siVerdicts(t, x) {
		if j.State != "BLOCKED" || j.Code != SvcInlineNativeNotRunCode {
			t.Errorf("%s without evidence: %s %s", id, j.State, j.Code)
		}
	}
}

// A BLOCKED step never becomes VERIFIED, and evidence that does not belong to a derived step is an error.
func TestSvcInlineBlockedNeverVerified(t *testing.T) {
	x, obs, ids := siSyntheticStep(t, "class C {}")
	x.Steps = append(x.Steps, SvcInlineStep{Case: "synthetic", Step: 1, State: SvcInlineBlocked, Code: SvcInlinePreprocCode, Reason: "r"})
	got := siVerdicts(t, x, obs)
	if j := got[svcID(SvcKindInlineTree, "synthetic", 1, "")]; j.State != "BLOCKED" || j.Code != SvcInlinePreprocCode {
		t.Errorf("blocked tree: %+v", j)
	}
	for _, id := range ids {
		if j := got[svcID(SvcKindQuery, "synthetic", 1, id)]; j.State != "BLOCKED" || j.Code != SvcInlineQueryTreeBlockedCode {
			t.Errorf("blocked query %s: %+v", id, j)
		}
	}
	forged := obs
	forged.Step = 1
	for code, list := range map[string][]SvcInlineObserved{
		"SVC_INLINE_OBSERVED_FOR_BLOCKED": {obs, forged},
		"SVC_INLINE_OBSERVED_REPEATED":    {obs, obs},
		"SVC_INLINE_OBSERVED_UNKNOWN":     {obs, {Case: "other", Step: 0, Views: obs.Views}},
	} {
		_, err := JudgeSvcInline(x, list)
		if err == nil || svcCode(t, err) != code {
			t.Errorf("%s: %v", code, err)
		}
	}
}

// Without native evidence the inline obligations stay BLOCKED in the fold; with the offline
// judgments the scope is still BLOCKED, never COMPLETE.
func TestSvcInlineFoldWithoutEvidence(t *testing.T) {
	asset, svc, registry, inventory, fixture, pack := svcInlineAssets(t)
	x, err := ParseSvcInlineExpected(asset, svc, registry, inventory, fixture, pack)
	if err != nil {
		t.Fatal(err)
	}
	e, err := ParseSvcExpected(svc, registry, inventory, fixture)
	if err != nil {
		t.Fatal(err)
	}
	offline, err := VerifySvcExpected(e, inventory)
	if err != nil {
		t.Fatal(err)
	}
	inline, err := JudgeSvcInline(x, nil)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := MergeSvcJudgments(offline, inline)
	if err != nil {
		t.Fatal(err)
	}
	cov, err := FoldSvcScope(e, "windows-amd64", merged, map[string]bool{"linux-amd64": true})
	if err != nil {
		t.Fatal(err)
	}
	if cov.Completion != "BLOCKED" || cov.FullScopeComplete {
		t.Fatalf("completion %s", cov.Completion)
	}
	for _, c := range cov.Counters {
		if c.Platform == "windows-amd64" && (c.Kind == SvcKindInlineTree && (c.Blocked != 82 || c.Verified != 0) || c.Kind == SvcKindQuery && (c.Blocked != 164 || c.Verified != 0)) {
			t.Errorf("%s", c)
		}
	}
	if _, err := MergeSvcJudgments(offline, []SvcJudgment{{ID: "svc1/nope", State: "BLOCKED", Code: "X"}}); err == nil {
		t.Error("a judgment for an undeclared obligation merged")
	}
}

// A step the native run reached by editing the previous inline step has an incremental and a fresh
// tree; a missing role must not verify its tree or its queries. A step with no inline step before it
// (step 0, or the start of a segment) has the incremental view only.
func TestSvcInlineEditedStepNeedsIncrementalAndFreshViews(t *testing.T) {
	x, obs, _ := siSyntheticStep(t, "class C {}")
	next := x.Steps[0]
	next.Step = 1
	x.Steps = append(x.Steps, next)
	view := func(role string) SvcInlineView {
		v := obs.Views[0]
		v.Role = role
		return v
	}
	step := func(n int, views ...SvcInlineView) SvcInlineObserved {
		o := obs
		o.Step, o.Views = n, views
		return o
	}
	verdicts := func(x SvcInlineExpected, id string, o ...SvcInlineObserved) []SvcJudgment {
		var out []SvcJudgment
		for _, j := range siVerdicts(t, x, o...) {
			if strings.Contains(j.ID, id) {
				out = append(out, j)
			}
		}
		return out
	}
	both := []SvcInlineView{view(SvcInlineRoleIncremental), view(SvcInlineRoleFresh)}
	// both roles: every obligation of the edited step is VERIFIED
	for _, j := range verdicts(x, "/synthetic/1/", step(0, view(SvcInlineRoleIncremental)), step(1, both...)) {
		if j.State != "VERIFIED" {
			t.Errorf("%s with both roles: %s %s", j.ID, j.State, j.Code)
		}
	}
	// a missing role is incomplete evidence: BLOCKED, never VERIFIED
	for name, views := range map[string][]SvcInlineView{
		"incremental only": {view(SvcInlineRoleIncremental)},
		"fresh only":       {view(SvcInlineRoleFresh)},
	} {
		got := verdicts(x, "/synthetic/1/", step(0, view(SvcInlineRoleIncremental)), step(1, views...))
		if len(got) != 3 {
			t.Fatalf("%s: %d judgments", name, len(got))
		}
		for _, j := range got {
			if j.State != "BLOCKED" || j.Code != SvcInlineNativeNotRunCode {
				t.Errorf("%s: %s %s %s", name, j.ID, j.State, j.Code)
			}
		}
	}
	// a view that mismatches stays FAILED even when the other role is missing
	bad := view(SvcInlineRoleIncremental)
	bad.Nodes = slices.Clone(bad.Nodes)
	bad.Nodes[2].EndByte++
	for _, j := range verdicts(x, svcID(SvcKindInlineTree, "synthetic", 1, ""), step(0, view(SvcInlineRoleIncremental)), step(1, bad)) {
		if j.State != "FAILED" {
			t.Errorf("a mismatching incremental view without its fresh view: %+v", j)
		}
	}
	// unknown, repeated and misplaced roles are errors
	for name, views := range map[string][]SvcInlineView{
		"no role":        {view("")},
		"repeated role":  {view(SvcInlineRoleIncremental), view(SvcInlineRoleIncremental)},
		"repeated fresh": {view(SvcInlineRoleIncremental), view(SvcInlineRoleFresh), view(SvcInlineRoleFresh)},
		"unknown role":   {view(SvcInlineRoleIncremental), view("other")},
	} {
		if _, err := JudgeSvcInline(x, []SvcInlineObserved{step(0, view(SvcInlineRoleIncremental)), step(1, views...)}); err == nil || svcCode(t, err) != "SVC_INLINE_VIEW_ROLE" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := JudgeSvcInline(x, []SvcInlineObserved{step(0, both...), step(1, both...)}); err == nil || svcCode(t, err) != "SVC_INLINE_VIEW_ROLE" {
		t.Errorf("a fresh view for step 0: %v", err)
	}
	// a restarted segment: step 1 has no inline step 0 before it, so the incremental view is complete
	restart := x
	restart.Steps = x.Steps[1:]
	for _, j := range siVerdicts(t, restart, step(1, view(SvcInlineRoleIncremental))) {
		if j.State != "VERIFIED" {
			t.Errorf("%s: a restarted segment's incremental view must verify: %s %s", j.ID, j.State, j.Code)
		}
	}
	if _, err := JudgeSvcInline(restart, []SvcInlineObserved{step(1, both...)}); err == nil || svcCode(t, err) != "SVC_INLINE_VIEW_ROLE" {
		t.Errorf("a fresh view for a restarted segment: %v", err)
	}
}
