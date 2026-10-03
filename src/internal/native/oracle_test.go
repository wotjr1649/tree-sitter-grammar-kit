package native

import (
	"context"
	jsonv2 "encoding/json/v2"
	"slices"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// oracleContext is the native-query context with the given queries and API switch.
func oracleContext(api bool, queries ...string) Context {
	x := testContext("native-query")
	x.Protocol, x.API = ProtocolR2, api
	for i, q := range queries {
		x.Queries = append(x.Queries, QuerySource{ID: "q" + string(rune('0'+i)), Source: []byte(q)})
	}
	return x
}

func runOracleCase(t *testing.T, b *Build, x Context, src string, edits []kit.Edit, points ...kit.NativePoint) CaseResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c := kit.IncrementalCase{ID: "oracle", Encoding: kit.EncodingUTF8, Edits: edits, Points: points}
	return b.RunCase(ctx, x, c, []byte(src))
}

type capText struct{ name, typ, text string }

func streamOf(t *testing.T, q QueryOut, src string) []capText {
	t.Helper()
	var out []capText
	for _, c := range q.Captures {
		out = append(out, capText{c.Name, c.Type, src[c.StartByte:c.EndByte]})
	}
	return out
}

// S06-A03: repeated captures of one node by two patterns, a tie at the same byte and two
// patterns keep the runtime's stream: start byte, then pattern index, then the match's
// capture order. The expectation is derived from that documented order, not from a run.
func TestQueryStream(t *testing.T) {
	b := fixtureBuild(t, "plain")
	src := "a = f(1);\n"
	x := oracleContext(false, "(assignment name: (identifier) @name) @assign\n(identifier) @id\n")
	r := runOracleCase(t, b, x, src, nil)
	if r.ExecutionStatus != kit.StatusCompleted || r.Assessment != kit.AssessPass {
		t.Fatalf("%s %s %s", r.ExecutionStatus, r.Assessment, r.Code)
	}
	q := r.Steps[0].Incremental.Queries[0]
	want := []capText{{"assign", "assignment", "a = f(1);"}, {"name", "identifier", "a"}, {"id", "identifier", "a"}, {"id", "identifier", "f"}}
	got := streamOf(t, q, src)
	if q.Status != kit.StatusCompleted || q.Evaluation != EvalStructural || len(got) != len(want) {
		t.Fatalf("%s %s %v", q.Status, q.Evaluation, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("capture %d: %v, want %v (stream %v)", i, got[i], want[i], got)
		}
	}
	// the same node twice, under two patterns: one node index, two capture records
	if q.Captures[1].Node != q.Captures[2].Node || q.Captures[1].Pattern == q.Captures[2].Pattern || q.Captures[1].Match == q.Captures[2].Match {
		t.Fatalf("duplicate capture not preserved: %+v %+v", q.Captures[1], q.Captures[2])
	}
	nodes := r.Steps[0].Incremental.Tree.Nodes
	for _, c := range q.Captures {
		n := nodes[c.Node]
		if n.Type != c.Type || n.StartByte != c.StartByte || n.EndByte != c.EndByte {
			t.Fatalf("node link %+v -> %+v", c, n)
		}
	}
}

// S06-A06: nodes sharing a span (the root and its only statement) and zero-width MISSING
// nodes at one byte keep distinct node indices; the mapping never collapses them.
func TestCaptureIdentity(t *testing.T) {
	b := fixtureBuild(t, "plain")
	t.Run("same-span", func(t *testing.T) {
		r := runOracleCase(t, b, oracleContext(false, "(_) @n\n"), "x;", nil)
		q := r.Steps[0].Incremental.Queries[0]
		if q.Status != kit.StatusCompleted || len(q.Captures) < 2 {
			t.Fatalf("%s %+v", q.Status, q.Captures)
		}
		if a, c := q.Captures[0], q.Captures[1]; a.StartByte != c.StartByte || a.EndByte != c.EndByte || a.Node == c.Node {
			t.Fatalf("same-span nodes collapsed: %+v %+v", a, c)
		}
	})
	t.Run("zero-width", func(t *testing.T) {
		// the S05 MISSING fixture: a zero-width MISSING ")" where the ";" starts
		src := "z = [1, 2];\na = (1;\nb = 2;\n"
		r := runOracleCase(t, b, oracleContext(false, "_ @any\n"), src, nil)
		q := r.Steps[0].Incremental.Queries[0]
		zero := 0
		for _, c := range q.Captures {
			if c.StartByte != c.EndByte {
				continue
			}
			zero++
			if !c.IsMissing {
				t.Fatalf("zero-width capture is not MISSING: %+v", c)
			}
			for _, o := range q.Captures {
				if o.StartByte == c.StartByte && o.Node == c.Node && o.Type != c.Type {
					t.Fatalf("zero-width capture shares a node index with %+v", o)
				}
			}
		}
		if zero == 0 {
			t.Fatalf("no zero-width capture in %+v", q.Captures)
		}
	})
}

// S06-A04: an invalid query keeps its rejection class and position; a predicate outside
// the host subset is UNSUPPORTED without captures; an executed query with no match is an
// empty completed result, distinct from a query that was not requested.
func TestQueryErrorsAndPredicates(t *testing.T) {
	b := fixtureBuild(t, "plain")
	src := "a = f(1);\nb = a;\n"
	x := oracleContext(false,
		"(assignment name: (nosuch))",                 // q0 NODE_TYPE
		"(assignment",                                 // q1 SYNTAX
		"(assignment bogus: (_))",                     // q2 FIELD
		"((identifier) @x (#match? @x \"^a\"))",       // q3 unsupported predicate
		"((identifier) @x (#set! kind \"v\"))",        // q4 directive
		"((identifier) @x (#eq? @x \"a\"))",           // q5 evaluated
		"((identifier) @x (#any-of? @x \"f\" \"b\"))", // q6 evaluated
		"(string) @s",                                 // q7 executed, empty
	)
	r := runOracleCase(t, b, x, src, nil)
	if r.ExecutionStatus != kit.StatusCompleted {
		t.Fatalf("%s %s", r.ExecutionStatus, r.Code)
	}
	qs := r.Steps[0].Incremental.Queries
	for i, want := range []struct{ status, code, typ string }{
		{"INVALID_QUERY", "QUERY_NODE_TYPE", "NODE_TYPE"}, {"INVALID_QUERY", "QUERY_SYNTAX", "SYNTAX"}, {"INVALID_QUERY", "QUERY_FIELD", "FIELD"},
	} {
		if q := qs[i]; q.Status != want.status || q.Code != want.code || q.Error == nil || q.Error.Type != want.typ || q.Captures != nil {
			t.Fatalf("q%d: %+v", i, q)
		}
	}
	if qs[0].Error.Offset != 19 || qs[0].Error.Point != [2]uint32{0, 19} {
		t.Fatalf("NODE_TYPE position %+v", qs[0].Error)
	}
	for _, i := range []int{3, 4} {
		if q := qs[i]; q.Status != EvalUnsupported || q.Evaluation != EvalUnsupported || q.Captures != nil {
			t.Fatalf("q%d presented as evaluated: %+v", i, q)
		}
	}
	if got := streamOf(t, qs[5], src); len(got) != 2 || got[0].text != "a" || got[1].text != "a" || qs[5].Evaluation != EvalEvaluated {
		t.Fatalf("eq? %v %s", got, qs[5].Evaluation)
	}
	if got := streamOf(t, qs[6], src); len(got) != 2 || got[0].text != "f" || got[1].text != "b" {
		t.Fatalf("any-of? %v", got)
	}
	if q := qs[7]; q.Status != kit.StatusCompleted || q.Captures == nil || len(q.Captures) != 0 {
		t.Fatalf("empty result %+v", q)
	}
	// a query that was never requested is absent, not empty
	r1 := runOracleCase(t, b, oracleContext(false), src, nil)
	if r1.Steps[0].Incremental.Queries == nil || len(r1.Steps[0].Incremental.Queries) != 0 {
		t.Fatalf("absent queries %+v", r1.Steps[0].Incremental.Queries)
	}
}

// S06-A05: a capture, match or time limit after some captures keeps them as partial: the
// case is RESOURCE_LIMIT/BLOCKED, never COMPLETED/PASS, and the process cleanup is verified.
func TestQueryLimits(t *testing.T) {
	b := fixtureBuild(t, "plain")
	src := "a = f(1, [2, 3]);\nb = [4, 5, 6];\nc = (7);\n"
	for _, tc := range []struct {
		name, code string
		set        func(*kit.NativeOperation)
	}{
		{"captures", "CAPTURE_LIMIT", func(o *kit.NativeOperation) { o.Captures = 3 }},
		{"matches", "MATCH_LIMIT", func(o *kit.NativeOperation) { o.Matches = 2 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := oracleContext(false, "(number) @n\n(identifier) @i\n")
			tc.set(&x.Op)
			r := runOracleCase(t, b, x, src, nil)
			if r.ExecutionStatus != kit.StatusResourceLimit || r.Assessment != kit.AssessBlocked || r.Code != tc.code || r.Process == nil || !r.Process.Cleanup.Verified {
				t.Fatalf("%s %s %s", r.ExecutionStatus, r.Assessment, r.Code)
			}
			q := r.Steps[0].Incremental.Queries[0]
			if q.Status != kit.StatusResourceLimit || !q.Partial || len(q.Captures) == 0 {
				t.Fatalf("partial not kept: %+v", q)
			}
		})
	}
	t.Run("time", func(t *testing.T) {
		big := ""
		for len(big) < 60000 {
			big += "a = f(g(h(1, [2, [3, [4]]])), (5));\n"
		}
		x := oracleContext(false, "(_ (_ (_ (_ (_))))) @deep\n(_ (_) @a (_) @b) @pair\n_ @any\n")
		// test-only operation values: a large tree and a 1 ms query budget
		x.Op.QueryMillis, x.Op.Nodes, x.Op.FullNodes, x.Op.Captures, x.Op.Matches = 1, 200000, 200000, 1000000, 1000000
		r := runOracleCase(t, b, x, big, nil)
		if r.ExecutionStatus != kit.StatusResourceLimit || r.Assessment != kit.AssessBlocked || r.Code != "QUERY_TIME_LIMIT" || !r.Process.Cleanup.Verified {
			t.Fatalf("%s %s %s", r.ExecutionStatus, r.Assessment, r.Code)
		}
		// either the stream stopped at the cancellation (partial captures kept) or the runtime
		// ran past it and the driver ended with a typed frame without steps
		if len(r.Steps) > 0 {
			if q := r.Steps[0].Incremental.Queries[0]; q.Status != kit.StatusResourceLimit || !q.Partial {
				t.Fatalf("time limit not partial: %+v", q)
			}
		}
	})
}

// S06-A07: the node API, field lookups, cursor positions and byte lookups agree with the
// cursor serialization, including ERROR, MISSING and extra (comment) nodes.
func TestAPIObservations(t *testing.T) {
	b := fixtureBuild(t, "plain")
	src := "# c\na = f(1, [2, 3]);\n{ b = ); }\nc = (1;\n"
	r := runOracleCase(t, b, oracleContext(true), src, nil, kit.NativePoint{ID: "p1", Byte: 6}, kit.NativePoint{ID: "p2", Byte: 0}, kit.NativePoint{ID: "p3", Byte: uint32(len(src))})
	if r.ExecutionStatus != kit.StatusCompleted || r.Oracle.API != ClaimPass {
		t.Fatalf("%s %s %+v %+v", r.ExecutionStatus, r.Code, r.Oracle, *r.Steps[0].Incremental.API.First)
	}
	var errs, missing, extra int
	for _, n := range r.Steps[0].Incremental.Tree.Nodes {
		if n.IsError {
			errs++
		}
		if n.IsMissing {
			missing++
		}
		if n.Extra {
			extra++
		}
	}
	if errs == 0 || missing == 0 || extra == 0 {
		t.Fatalf("fixture lacks ERROR/MISSING/extra: %d %d %d", errs, missing, extra)
	}
	// the pinned runtime's positional sibling navigation skips the zero-width MISSING ")":
	// observed and recorded, never silently equal
	api := r.Steps[0].Incremental.API
	if api.PositionNavigation == 0 || api.FirstDivergence == nil || api.FirstDivergence.Check != "position_navigation:next_sibling" {
		t.Fatalf("zero-width sibling divergence not recorded: %+v", api)
	}
	// negative controls on the comparison itself: a wrong parent and a wrong sibling of a
	// node with width are differences, not divergences
	tree, raw := r.Steps[0].Incremental.Tree.Nodes, r.Raw
	resp, err := DecodeResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	var w WireAPI
	if err := jsonv2.Unmarshal(resp.Steps[0].Incremental.API, &w); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		col   int
		check string
	}{{0, "parent"}, {2, "next_sibling"}, {7, "child_count"}, {10, "cursor_depth"}} {
		bad := w
		bad.Nodes = make([][]int64, len(w.Nodes))
		for i := range w.Nodes {
			bad.Nodes[i] = slices.Clone(w.Nodes[i])
		}
		bad.Nodes[2][tc.col] += 1 // node 2 has width
		if d, _ := compareAPI(tree, &bad); d == nil || d.Check != tc.check {
			t.Fatalf("tampered %s not detected: %+v", tc.check, d)
		}
	}
	// the zero-width MISSING node naming itself as its previous sibling is a difference, not
	// a positional divergence
	for i, n := range tree {
		if n.IsMissing && n.StartByte == n.EndByte {
			self := w
			self.Nodes = make([][]int64, len(w.Nodes))
			for k := range w.Nodes {
				self.Nodes[k] = slices.Clone(w.Nodes[k])
			}
			self.Nodes[i][1] = int64(i)
			if d, _ := compareAPI(tree, &self); d == nil || d.Check != "prev_sibling" {
				t.Fatalf("self as sibling accepted: %+v", d)
			}
			break
		}
	}
	lookups := w
	lookups.FieldLookups = w.FieldLookups[1:]
	if d, _ := compareAPI(tree, &lookups); d == nil {
		t.Fatal("a dropped field lookup was not detected")
	}
}

// S06-A01/A12: an r1 request on the extended driver still gets an r1 response; a response
// in another revision than the request is rejected.
func TestRevisions(t *testing.T) {
	b := fixtureBuild(t, "plain")
	req := baseRequest("a = 1;", "native-parse-edit")
	_, resp, err := rawExec(t, b, Frame(req.Encode()), "native-parse-edit")
	if err != nil || resp.Protocol != Protocol || resp.Producer.Query != "UNSUPPORTED" || resp.Producer.API != nil {
		t.Fatalf("r1 %v %+v", err, resp.Producer)
	}
	r2 := req
	r2.Protocol, r2.Limits = ProtocolR2, LimitsFor(kit.NativeOperations()["native-query"])
	res, resp2, err := rawExec(t, b, Frame(r2.Encode()), "native-query")
	if err != nil || resp2.Protocol != ProtocolR2 || resp2.Producer.Query != QueryCapability {
		t.Fatalf("r2 %v %+v", err, resp2.Producer)
	}
	versions, points, _ := kit.ApplyEdits(kit.EncodingUTF8, req.Source, nil, 65536)
	if _, err := Check(resp2, req, versions, points, res.ExitCode); err == nil || err.(*Error).Code != "RESPONSE_PROTOCOL_MISMATCH" {
		t.Fatalf("r2 response accepted for an r1 request: %v", err)
	}
	if _, err := Check(resp2, r2, versions, points, res.ExitCode); err != nil {
		t.Fatalf("r2 check %v", err)
	}
}

// S06-A08: a producer that does not declare the query capability blocks a query case
// before any capture is read.
func TestCapabilityMissing(t *testing.T) {
	b := fixtureBuild(t, "plain", "TSGK_FAULT_NO_QUERY")
	r := runOracleCase(t, b, oracleContext(false, "(identifier) @i"), "a = 1;", nil)
	if r.ExecutionStatus != kit.StatusNotRun || r.Assessment != kit.AssessBlocked || r.Code != "QUERY_CAPABILITY_MISSING" || len(r.Steps) != 0 {
		t.Fatalf("%s %s %s %d", r.ExecutionStatus, r.Assessment, r.Code, len(r.Steps))
	}
}

// S06-A02/A12: on edit steps every query runs on the incremental and the fresh tree and the
// two evaluated streams are equal, alongside the S05 claims.
func TestQueryAcrossEdits(t *testing.T) {
	b := fixtureBuild(t, "plain")
	src := "z = [1, 2];\na = f(1);\n"
	r := runOracleCase(t, b, oracleContext(true, "(call function: (identifier) @fn)\n(number) @n\n"), src, editsBy(src, edit{"f(1)", "g(1, 2)"}, edit{"[1, 2]", "[1]"}))
	if r.Assessment != kit.AssessPass || r.Claims != (Claims{ClaimPass, ClaimPass, ClaimNotClaimed}) || r.Oracle.QueryEquality != ClaimPass || r.Oracle.API != ClaimPass {
		t.Fatalf("%s %s %+v %+v", r.Assessment, r.Code, r.Claims, r.Oracle)
	}
	for _, s := range r.Steps[1:] {
		if s.QueryCompare == nil || !s.QueryCompare.Equal || len(s.Fresh.Queries) != 1 {
			t.Fatalf("step %d %+v", s.Step, s.QueryCompare)
		}
	}
	if got := streamOf(t, r.Steps[1].Incremental.Queries[0], "z = [1, 2];\na = g(1, 2);\n"); len(got) != 5 || got[2] != (capText{"fn", "identifier", "g"}) {
		t.Fatalf("step 1 stream %v", got)
	}
}

// S06-A06/A11: a capture row whose node index names another node of the tree is rejected;
// the record never links a capture to the wrong node.
func TestCaptureLink(t *testing.T) {
	b := fixtureBuild(t, "plain")
	req := baseRequest("a = f(1);", "native-query")
	req.Protocol, req.Limits = ProtocolR2, LimitsFor(kit.NativeOperations()["native-query"])
	req.Queries = []QuerySource{{ID: "q", Source: []byte("(identifier) @id")}}
	res, resp, err := rawExec(t, b, Frame(req.Encode()), "native-query")
	if err != nil {
		t.Fatal(err)
	}
	versions, points, _ := kit.ApplyEdits(kit.EncodingUTF8, req.Source, nil, 65536)
	if _, err := Check(resp, req, versions, points, res.ExitCode); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}
	var qs []WireQuery
	if err := jsonv2.Unmarshal(resp.Steps[0].Incremental.Queries, &qs); err != nil || len(qs[0].Captures) < 2 {
		t.Fatalf("%v %+v", err, qs)
	}
	qs[0].Captures[0][3] = qs[0].Captures[1][3] // the first identifier linked to the second's node
	tampered, _ := jsonv2.Marshal(qs)
	resp.Steps[0].Incremental.Queries = tampered
	if _, err := Check(resp, req, versions, points, res.ExitCode); err == nil || err.(*Error).Code != "CAPTURE_LINK_MISMATCH" {
		t.Fatalf("relinked capture accepted: %v", err)
	}
}

// S06-A02/A11: the incremental/fresh query comparison fails on a changed capture or status.
func TestQueryEquality(t *testing.T) {
	a := []QueryOut{{ID: "q", Status: kit.StatusCompleted, Evaluation: EvalStructural, Captures: []kit.Capture{{Name: "x", Node: 1}, {Name: "y", Node: 2}}}}
	if eq, _ := compareQueries(a, a); !eq {
		t.Fatal("equal results differ")
	}
	b := []QueryOut{{ID: "q", Status: kit.StatusCompleted, Evaluation: EvalStructural, Captures: []kit.Capture{{Name: "x", Node: 1}, {Name: "y", Node: 3}}}}
	if eq, first := compareQueries(a, b); eq || first == "" {
		t.Fatal("changed capture node not detected")
	}
	c := []QueryOut{{ID: "q", Status: kit.StatusResourceLimit, Code: "CAPTURE_LIMIT", Evaluation: EvalStructural, Captures: a[0].Captures}}
	if eq, _ := compareQueries(a, c); eq {
		t.Fatal("changed status not detected")
	}
}

// S06-A04/A11: an r2 query result without its predicate list (or with a wrong match count)
// is rejected, never read as "no predicates" or accepted as consistent.
func TestQueryMembersRequired(t *testing.T) {
	b := fixtureBuild(t, "plain")
	req := baseRequest("a = f(1);", "native-query")
	req.Protocol, req.Limits = ProtocolR2, LimitsFor(kit.NativeOperations()["native-query"])
	req.Queries = []QuerySource{{ID: "q", Source: []byte("((identifier) @id (#match? @id \"^a\"))")}}
	res, resp, err := rawExec(t, b, Frame(req.Encode()), "native-query")
	if err != nil {
		t.Fatal(err)
	}
	versions, points, _ := kit.ApplyEdits(kit.EncodingUTF8, req.Source, nil, 65536)
	tamper := func(f func(q map[string]any)) error {
		r := resp
		var qs []map[string]any
		if err := jsonv2.Unmarshal(resp.Steps[0].Incremental.Queries, &qs); err != nil {
			t.Fatal(err)
		}
		f(qs[0])
		raw, _ := jsonv2.Marshal(qs)
		steps := slices.Clone(resp.Steps)
		steps[0].Incremental.Queries = raw
		r.Steps = steps
		_, err := Check(r, req, versions, points, res.ExitCode)
		return err
	}
	if err := tamper(func(map[string]any) {}); err != nil {
		t.Fatalf("untampered response rejected: %v", err)
	}
	if err := tamper(func(q map[string]any) { delete(q, "predicates") }); err == nil {
		t.Fatal("a result without predicates was accepted")
	}
	if err := tamper(func(q map[string]any) { q["matches"] = 7.0 }); err == nil {
		t.Fatal("a wrong match count was accepted")
	}
	// a member no other shape check covers: its absence alone must be rejected
	if err := tamper(func(q map[string]any) { delete(q, "partial") }); err == nil {
		t.Fatal("a result without partial was accepted")
	}
	// nested: a predicate without its pattern must not default to pattern 0
	if err := tamper(func(q map[string]any) { delete(q["predicates"].([]any)[0].(map[string]any), "pattern") }); err == nil {
		t.Fatal("a predicate without its pattern was accepted")
	}
	if err := tamper(func(q map[string]any) {
		delete(q["predicates"].([]any)[0].(map[string]any)["steps"].([]any)[0].(map[string]any), "kind")
	}); err == nil {
		t.Fatal("a predicate step without its kind was accepted")
	}
}
