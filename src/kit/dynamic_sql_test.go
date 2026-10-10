package kit

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestCSharpCommandIdentifierSpelling(t *testing.T) {
	for _, name := range []string{"SqlCommand", "@SqlCommand", `\u0053qlCommand`, `\U00000053qlCommand`, "Sq\u200blCommand", `Sq\u200blCommand`, `@\u0053qlCommand`} {
		src := "new " + name + "(query)"
		d := &dynCaps{t: t, src: src}
		creation := d.at("object_creation_expression", src, 1, true)
		d.pair("oc", creation, "oc.type", d.at("identifier", name, 1, true))
		argument := d.at("argument", "query", 1, true)
		d.pair("oc.a", creation, "oc.arg", argument)
		d.pair("arg", argument, "arg.expr", d.at("identifier", "query", 1, true))
		got, err := DeriveDynamicSQLChecked("csharp", EncodingUTF8, d.caps, []byte(src))
		start := uint32(strings.Index(src, "query"))
		if err != nil || len(got.Items) != 1 || !got.Heuristic || got.Items[0].StartByte != start || got.Items[0].EndByte != start+5 || got.Items[0].Variable == nil || *got.Items[0].Variable != "query" {
			t.Fatalf("%q: %+v, %v", name, got, err)
		}
	}
	for _, name := range []string{"CommandText", "@CommandText", `\u0043ommandText`, "Com\u200bmandText", `Com\U0000200BmandText`} {
		for _, initializer := range []bool{false, true} {
			src := "x." + name + " = query"
			d := &dynCaps{t: t, src: src}
			assignment := d.at("assignment_expression", src, 1, true)
			left := d.at("identifier", name, 1, true)
			right := d.at("identifier", "query", 1, true)
			if initializer {
				d.pair("ini.as", assignment, "ini.left", left)
				d.pair("ini.as", assignment, "ini.right", right)
			} else {
				d.pair("ma.as", assignment, "ma.name", left)
				d.pair("ma.as", assignment, "ma.right", right)
			}
			got, err := DeriveDynamicSQLChecked("csharp", EncodingUTF8, d.caps, []byte(src))
			start := uint32(strings.Index(src, "query"))
			if err != nil || len(got.Items) != 1 || got.Items[0].StartByte != start || got.Items[0].EndByte != start+5 {
				t.Fatalf("%q initializer=%v: %+v, %v", name, initializer, got, err)
			}
		}
	}
	for _, name := range []string{`\u0053qlCommand`, `\U00000053qlCommand`} {
		if csharpIdentifierName(name) != "SqlCommand" {
			t.Fatalf("escaped name %q", name)
		}
	}
	for _, name := range []string{`\u00`, `\x53qlCommand`, `\uZZZZqlCommand`, `\U00110000qlCommand`, `\uD800qlCommand`, `sqlCommand`, `@@SqlCommand`, "SlCommand"} {
		if slices.Contains(CommandAPITypes, csharpIdentifierName(name)) {
			t.Fatalf("invalid or different spelling matched: %q", name)
		}
	}
}

// dynCaps builds the captures of the tsql dynamic SQL pack query for a synthetic tree over
// src. Nodes are located by their text (the occurrence-th match, from 1).
type dynCaps struct {
	t     *testing.T
	src   string
	caps  []Capture
	match uint32
	node  int64
}

type dynNode struct {
	Node       int64
	Type       string
	Named      bool
	IsError    bool
	Start, End uint32
}

func (d *dynCaps) at(typ, text string, occurrence int, named bool) dynNode {
	d.t.Helper()
	from := 0
	for i := 0; ; i++ {
		k := strings.Index(d.src[from:], text)
		if k < 0 {
			d.t.Fatalf("%q occurrence %d not in source", text, occurrence)
		}
		if i+1 == occurrence {
			d.node++
			s := uint32(from + k)
			return dynNode{d.node, typ, named, typ == "ERROR", s, s + uint32(len(text))}
		}
		from += k + 1
	}
}

func (d *dynCaps) capture(m uint32, name string, n dynNode) {
	d.caps = append(d.caps, Capture{Match: m, Name: name, Node: n.Node, Type: n.Type, Named: n.Named, IsError: n.IsError, StartByte: n.Start, EndByte: n.End})
}

// one adds a match with a single capture.
func (d *dynCaps) one(name string, n dynNode) {
	d.match++
	d.capture(d.match, name, n)
}

// pair adds a match with a parent and a child capture.
func (d *dynCaps) pair(parent string, p dynNode, child string, c dynNode) {
	d.match++
	d.capture(d.match, parent, p)
	d.capture(d.match, child, c)
}

// exec adds an execute_statement with its children (one exec.parent/exec.child match each).
func (d *dynCaps) exec(e dynNode, kids ...dynNode) {
	d.one("exec", e)
	for _, k := range kids {
		d.pair("exec.parent", e, "exec.child", k)
	}
}

// A paren EXEC: EXEC ( arg ) AT <server>; the server text is given.
func (d *dynCaps) parenAt(stmt, arg, server string) dynNode {
	e := d.at("execute_statement", stmt, 1, true)
	kw := d.at("keyword_exec", "EXEC", strings.Count(d.src[:e.Start], "EXEC")+1, true)
	open := d.at("(", "(", strings.Count(d.src[:e.Start], "(")+1, false)
	lit := d.at("literal", arg, 1, true)
	closeParen := d.at(")", ")", strings.Count(d.src[:lit.End], ")")+1, false)
	at := d.at("keyword_at", "AT", strings.Count(d.src[:e.Start], "AT")+1, true)
	srv := d.at("identifier", server, 1, true)
	d.exec(e, kw, open, lit, closeParen, at, srv)
	return e
}

// An sp_executesql EXEC with one positional argument.
func (d *dynCaps) spExecutesql(stmt, arg string) dynNode {
	e := d.at("execute_statement", stmt, 1, true)
	kw := d.at("keyword_exec", "EXEC", strings.Count(d.src[:e.Start], "EXEC")+1, true)
	proc := d.at("object_reference", "sp_executesql", strings.Count(d.src[:e.Start], "sp_executesql")+1, true)
	name := proc
	name.Type = "identifier"
	d.node++
	name.Node = d.node
	a := d.at("exec_argument", arg, 1, true)
	v := d.at("literal", arg, 1, true)
	d.exec(e, kw, proc, a)
	d.pair("proc.exec", e, "proc", proc)
	d.pair("ref", proc, "ref.name", name)
	d.pair("arg.v", a, "arg.value", v)
	return e
}

func (d *dynCaps) semis() {
	for i := 1; i <= strings.Count(d.src, ";"); i++ {
		d.one("semi", d.at(";", ";", i, false))
	}
}

func litFact(construct string, src, lit string) DynamicSQLFact {
	s := uint32(strings.Index(src, lit))
	return DynamicSQLFact{Construct: construct, ArgumentKind: "literal_unicode", StartByte: s, EndByte: s + uint32(len(lit))}
}

func span(src, text string) (uint32, uint32) {
	s := uint32(strings.Index(src, text))
	return s, s + uint32(len(text))
}

// The three tsql known-miss branches of dynamic-sql-r1 (the registered fixture no longer
// exercises them: the C2 tsql grammar parses AT DATA_SOURCE and WITH RESULT SETS). Each
// case pairs a miss with a neighbouring ordinary call that must stay a fact.
func TestDeriveDynamicSQLKnownMisses(t *testing.T) {
	t.Run("AT DATA_SOURCE identifier", func(t *testing.T) {
		src := "EXEC (N'a') AT RemoteSrv;\nEXEC (N'b') AT DATA_SOURCE;\n"
		d := &dynCaps{t: t, src: src}
		d.parenAt("EXEC (N'a') AT RemoteSrv", "N'a'", "RemoteSrv")
		d.parenAt("EXEC (N'b') AT DATA_SOURCE", "N'b'", "DATA_SOURCE")
		d.semis()
		s, _ := span(src, "EXEC (N'b')")
		_, e := span(src, "DATA_SOURCE;")
		check(t, DeriveDynamicSQL("tsql", EncodingUTF8, d.caps, []byte(src)),
			[]DynamicSQLFact{litFact("EXEC_PAREN_AT", src, "N'a'")},
			[]KnownMiss{{MissAtDataSource, s, e}})
	})
	t.Run("WITH RESULT SETS error", func(t *testing.T) {
		src := "EXEC sp_executesql N'a';\nEXEC sp_executesql N'b' WITH RESULT SETS NONE;\n"
		d := &dynCaps{t: t, src: src}
		d.spExecutesql("EXEC sp_executesql N'a'", "N'a'")
		d.spExecutesql("EXEC sp_executesql N'b'", "N'b'")
		errNode := d.at("ERROR", "WITH RESULT SETS NONE", 1, true)
		d.pair("errwith", errNode, "errwith.first", d.at("keyword_with", "WITH", 1, true))
		d.semis()
		s, _ := span(src, "EXEC sp_executesql N'b'")
		_, e := span(src, "NONE;")
		check(t, DeriveDynamicSQL("tsql", EncodingUTF8, d.caps, []byte(src)),
			[]DynamicSQLFact{litFact("SP_EXECUTESQL", src, "N'a'")},
			[]KnownMiss{{MissWithResultSets, s, e}})
	})
	t.Run("batch-first call without EXEC", func(t *testing.T) {
		src := "EXEC sp_executesql N'a';\nGO\n;sp_executesql N'b';\nGO\n;other_proc N'c';\n"
		d := &dynCaps{t: t, src: src}
		e := d.spExecutesql("EXEC sp_executesql N'a'", "N'a'")
		d.one("batch.child", e)
		d.one("batch.child", d.at("go_statement", "GO", 1, true))
		miss := d.at("ERROR", "sp_executesql N'b'", 1, true)
		d.pair("err", miss, "err.first", d.at("identifier", "sp_executesql", 2, true))
		other := d.at("ERROR", "other_proc N'c'", 1, true)
		d.pair("err", other, "err.first", d.at("identifier", "other_proc", 1, true))
		d.one("batch.program", d.at("program", src, 1, true))
		d.semis()
		check(t, DeriveDynamicSQL("tsql", EncodingUTF8, d.caps, []byte(src)),
			[]DynamicSQLFact{litFact("SP_EXECUTESQL", src, "N'a'")},
			[]KnownMiss{{MissBatchFirstNoExe, miss.Start, miss.End}})
	})
}

// Recovery may put the preceding statement in a separate batch without a GO.
// Only a captured top-level GO, never GO text in a string/comment, resets firstness.
func TestDynamicSQLBatchFirstContext(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
		first        bool
		goBoundary   bool
	}{
		{"first", "", true, false},
		{"nonfirst-single", "SELECT 1;", false, false},
		{"nonfirst-double", "SELECT 1;;", false, false},
		{"nonfirst-comment", "SELECT 1; /* owned */ ", false, false},
		{"go-in-string", "SELECT N'\nGO\n';;", false, false},
		{"go-in-comment", "SELECT 1; /*\nGO\n*/;", false, false},
		{"after-go", "SELECT 1;\nGO\n;", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.prefix + "sp_executesql N'b';\n"
			d := &dynCaps{t: t, src: src}
			d.one("batch.program", d.at("program", src, 1, true))
			if tc.prefix != "" {
				d.one("batch.child", d.at("statement", "SELECT", 1, true))
			}
			if tc.goBoundary {
				d.one("batch.child", d.at("go_statement", "GO", 1, true))
			}
			errNode := d.at("ERROR", "sp_executesql N'b'", 1, true)
			d.pair("err", errNode, "err.first", d.at("identifier", "sp_executesql", 1, true))
			misses := []KnownMiss{}
			if tc.first {
				misses = append(misses, KnownMiss{MissBatchFirstNoExe, errNode.Start, errNode.End})
			}
			check(t, DeriveDynamicSQL("tsql", EncodingUTF8, d.caps, []byte(src)), []DynamicSQLFact{}, misses)
			// An old/incomplete query has no evidence of actual batch position.
			d.caps = slices.DeleteFunc(d.caps, func(c Capture) bool { return c.Name == "batch.program" })
			check(t, DeriveDynamicSQL("tsql", EncodingUTF8, d.caps, []byte(src)), []DynamicSQLFact{}, []KnownMiss{})
		})
	}
}

func TestDynamicSQLContextChildren(t *testing.T) {
	for _, tc := range []struct {
		prefix, typ         string
		named, extra, first bool
	}{
		{";", "batch", true, false, true},
		{";", ";", false, false, true},
		{"/* owned */", "marginalia", true, true, true},
		{"bad;", "ERROR", true, true, false},
		{"BEGIN SELECT 1; END;", "block", true, false, false},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			src := tc.prefix + "sp_executesql N'b';"
			d := &dynCaps{t: t, src: src}
			d.one("batch.program", d.at("program", src, 1, true))
			n := d.at(tc.typ, tc.prefix, 1, tc.named)
			d.one("batch.child", n)
			d.caps[len(d.caps)-1].Extra = tc.extra
			errNode := d.at("ERROR", "sp_executesql N'b'", 1, true)
			d.pair("err", errNode, "err.first", d.at("identifier", "sp_executesql", 1, true))
			misses := []KnownMiss{}
			if tc.first {
				misses = append(misses, KnownMiss{MissBatchFirstNoExe, errNode.Start, errNode.End})
			}
			check(t, DeriveDynamicSQL("tsql", EncodingUTF8, d.caps, []byte(src)), []DynamicSQLFact{}, misses)
		})
	}
}

func check(t *testing.T, got DynamicSQLFacts, items []DynamicSQLFact, misses []KnownMiss) {
	t.Helper()
	if got.Mapping != "dynamic-sql-r1" || got.Heuristic || len(got.Unlisted) != 0 {
		t.Fatalf("result identity %+v", got)
	}
	if !reflect.DeepEqual(got.Items, items) {
		t.Fatalf("items\n got %+v\nwant %+v", got.Items, items)
	}
	if !reflect.DeepEqual(got.KnownMisses, misses) {
		t.Fatalf("known misses\n got %+v\nwant %+v", got.KnownMisses, misses)
	}
}
