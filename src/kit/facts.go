package kit

import (
	"bytes"
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// FactPackSchema is the versioned fact query pack (src/contracts/fact-query-pack.json).
const FactPackSchema = "tsgk-fact-query-pack/r1"

// Fact query kinds of a pack query.
const (
	FactDeclarations = "declarations"
	FactDynamicSQL   = "dynamic_sql"
	FactXMLStructure = "xml_structure"
)

// FactPack is a decoded fact query pack: per route the declaration items its declaration
// query was generated from and the queries themselves.
type FactPack struct {
	Schema     string          `json:"schema"`
	Revision   string          `json:"revision"`
	Mapping    string          `json:"mapping"`
	DynamicSQL string          `json:"dynamic_sql"`
	XML        string          `json:"xml_structure"`
	Routes     []FactPackRoute `json:"routes"`
	SHA256     string          `json:"-"`
}

// FactPackRoute is one route of a pack.
type FactPackRoute struct {
	Route        string              `json:"route"`
	Declarations []NativeDeclaration `json:"declarations"`
	Queries      []FactQuery         `json:"queries"`
}

// FactQuery is one pack query: its id, the fact kind it serves and its source.
type FactQuery struct {
	ID     string `json:"id"`
	Facts  string `json:"facts"`
	Source string `json:"source"`
}

// ParseFactPack strictly decodes a fact query pack.
func ParseFactPack(data []byte) (FactPack, error) {
	p, e := parseFactPack(data)
	if e != nil {
		return FactPack{}, e
	}
	return p, nil
}

func parseFactPack(data []byte) (FactPack, *Error) {
	var p FactPack
	t := typed{doc: "fact-pack"}
	v, e := decodeStrict(t.doc, data, MaxDocumentBytes)
	if e != nil {
		return p, e
	}
	m, e := t.object(v, []string{"schema", "revision", "mapping", "dynamic_sql", "xml_structure", "routes"})
	if e != nil {
		return p, e
	}
	for _, f := range []struct {
		dst  *string
		name string
	}{{&p.Schema, "schema"}, {&p.Revision, "revision"}, {&p.Mapping, "mapping"}, {&p.DynamicSQL, "dynamic_sql"}, {&p.XML, "xml_structure"}} {
		if *f.dst, e = t.str(m[f.name]); e != nil {
			return p, e
		}
	}
	if p.Schema != FactPackSchema || !validID(p.Revision) {
		return p, t.bad("SCHEMA_UNSUPPORTED", m["schema"])
	}
	routes, e := t.array(m["routes"])
	if e != nil {
		return p, e
	}
	seen := map[string]bool{}
	for _, rv := range routes {
		rm, e := t.object(rv, []string{"route", "declarations", "queries"})
		if e != nil {
			return p, e
		}
		var r FactPackRoute
		if r.Route, e = t.str(rm["route"]); e != nil {
			return p, e
		} else if !validID(r.Route) || seen[r.Route] {
			return p, t.bad("ROUTE_INVALID", rm["route"])
		}
		seen[r.Route] = true
		decls, e := t.array(rm["declarations"])
		if e != nil {
			return p, e
		}
		for _, dv := range decls {
			dm, e := t.object(dv, []string{"fact", "node", "name"})
			if e != nil {
				return p, e
			}
			var d NativeDeclaration
			for _, f := range []struct {
				dst  *string
				name string
			}{{&d.Fact, "fact"}, {&d.Node, "node"}, {&d.Name, "name"}} {
				if *f.dst, e = nativeText(t, dm[f.name], "DECLARATION_INVALID"); e != nil {
					return p, e
				}
			}
			if !validLocator(d.Name) {
				return p, t.bad("LOCATOR_INVALID", dm["name"])
			}
			r.Declarations = append(r.Declarations, d)
		}
		qs, e := t.array(rm["queries"])
		if e != nil {
			return p, e
		}
		ids := map[string]bool{}
		for _, qv := range qs {
			qm, e := t.object(qv, []string{"id", "facts", "source"})
			if e != nil {
				return p, e
			}
			var q FactQuery
			if q.ID, e = t.str(qm["id"]); e != nil {
				return p, e
			} else if !validID(q.ID) || ids[q.ID] {
				return p, t.bad("QUERY_ID_INVALID", qm["id"])
			}
			ids[q.ID] = true
			if q.Facts, e = t.str(qm["facts"]); e != nil {
				return p, e
			}
			switch q.Facts {
			case FactDeclarations, FactDynamicSQL, FactXMLStructure:
			default:
				return p, t.bad("FACTS_INVALID", qm["facts"])
			}
			if q.Source, e = t.str(qm["source"]); e != nil {
				return p, e
			} else if q.Source == "" || len(q.Source) > MaxQueryBytes {
				return p, t.bad("QUERY_SOURCE_INVALID", qm["source"])
			}
			r.Queries = append(r.Queries, q)
		}
		p.Routes = append(p.Routes, r)
	}
	p.SHA256 = digestHex(data)
	return p, nil
}

// DeclarationQuery generates the declaration query of a pack route from its items: per
// item i the declaration node as @decl.i and, unless the locator is "node", the locator
// path as a nested pattern whose levels are captured @owner.i, @s<k>.i and finally
// @name.i. The pack stores the generated text; this function is its reproducible origin.
func DeclarationQuery(items []NativeDeclaration) string {
	var b strings.Builder
	for i, d := range items {
		n := strconv.Itoa(i)
		b.WriteString("(" + d.Node + ") @decl." + n + "\n")
		if d.Name == "node" {
			continue
		}
		segs := splitPath(d.Name)
		inner := ""
		for k := len(segs) - 1; k >= 0; k-- {
			kind, arg, _ := strings.Cut(segs[k], ":")
			capture := "@s" + strconv.Itoa(k) + "." + n
			if k == len(segs)-1 {
				capture = "@name." + n
			}
			node := "(" + arg + inner + ")"
			if kind == "field" {
				node = arg + ": _"
				if inner != "" {
					node = arg + ": (_" + inner + ")"
				}
			}
			inner = " " + node + " " + capture
		}
		b.WriteString("(" + d.Node + inner + ") @owner." + n + "\n")
	}
	return b.String()
}

// DeclarationItem is one declaration structure item (tsgk-tree-summary/r1 declarations).
type DeclarationItem struct {
	Fact      string    `json:"fact"`
	NodeType  string    `json:"node_type"`
	StartByte uint32    `json:"start_byte"`
	EndByte   uint32    `json:"end_byte"`
	Name      *ByteSpan `json:"name"`
	Status    string    `json:"status"`
}

// ByteSpan is a byte range.
type ByteSpan struct {
	StartByte uint32 `json:"start_byte"`
	EndByte   uint32 `json:"end_byte"`
}

// DeriveDeclarations reproduces the S05 declaration structure items from the captures of
// the pack's declaration query: one item per name of each declaration node in preorder,
// following the locator semantics (field:F and child:T take the first such child, so for
// those levels only the earliest node under the same path prefix is kept; children:T keeps
// every node), names in document order, NAME_MISSING without a name, HAS_ERROR when the
// declaration node has an error.
func DeriveDeclarations(items []NativeDeclaration, caps []Capture) []DeclarationItem {
	type chain struct{ nodes []Capture }
	decl := map[int]map[int64]Capture{} // item -> node -> capture
	matches := map[uint32][]Capture{}
	for _, c := range caps {
		prefix, idx, ok := strings.Cut(c.Name, ".")
		i, err := strconv.Atoi(idx)
		if !ok || err != nil || i < 0 || i >= len(items) {
			continue
		}
		if prefix == "decl" {
			if decl[i] == nil {
				decl[i] = map[int64]Capture{}
			}
			decl[i][c.Node] = c
			continue
		}
		matches[c.Match] = append(matches[c.Match], c)
	}
	// chains per item and owner node: [owner, s0, ..., name]
	chains := map[int]map[int64][]chain{}
	for _, mc := range matches {
		var owner *Capture
		levels := map[int]Capture{}
		item, depth := -1, 0
		for k := range mc {
			c := mc[k]
			prefix, idx, _ := strings.Cut(c.Name, ".")
			item, _ = strconv.Atoi(idx)
			switch {
			case prefix == "owner":
				owner = &mc[k]
			case prefix == "name":
				depth = len(splitPath(items[item].Name))
				levels[depth-1] = c
			case strings.HasPrefix(prefix, "s"):
				lv, err := strconv.Atoi(prefix[1:])
				if err == nil {
					levels[lv] = c
				}
			}
		}
		if owner == nil || item < 0 || len(levels) != depth {
			continue
		}
		ch := chain{nodes: []Capture{*owner}}
		for lv := range depth {
			ch.nodes = append(ch.nodes, levels[lv])
		}
		if chains[item] == nil {
			chains[item] = map[int64][]chain{}
		}
		chains[item][owner.Node] = append(chains[item][owner.Node], ch)
	}
	type key struct {
		node int64
		item int
	}
	var keys []key
	for i, nodes := range decl {
		for n := range nodes {
			keys = append(keys, key{n, i})
		}
	}
	slices.SortFunc(keys, func(a, b key) int { return cmp.Or(cmp.Compare(a.node, b.node), cmp.Compare(a.item, b.item)) })
	out := []DeclarationItem{}
	for _, k := range keys {
		d, c := items[k.item], decl[k.item][k.node]
		segs := splitPath(d.Name)
		list := chains[k.item][k.node]
		for lv := range segs {
			if !strings.HasPrefix(segs[lv], "children:") {
				// keep only chains whose node at this level is the first under its prefix
				first := map[string]int64{}
				pre := func(ch chain) string {
					var b strings.Builder
					for _, n := range ch.nodes[:lv+1] {
						b.WriteString(strconv.FormatInt(n.Node, 10) + "/")
					}
					return b.String()
				}
				for _, ch := range list {
					p, n := pre(ch), ch.nodes[lv+1].Node
					if v, ok := first[p]; !ok || n < v {
						first[p] = n
					}
				}
				list = slices.DeleteFunc(slices.Clone(list), func(ch chain) bool { return ch.nodes[lv+1].Node != first[pre(ch)] })
			}
		}
		var names []Capture
		for _, ch := range list {
			n := ch.nodes[len(ch.nodes)-1]
			if !slices.ContainsFunc(names, func(x Capture) bool { return x.Node == n.Node }) {
				names = append(names, n)
			}
		}
		slices.SortFunc(names, func(a, b Capture) int { return cmp.Compare(a.Node, b.Node) })
		status := "PASS"
		if c.HasError {
			status = "HAS_ERROR"
		}
		item := DeclarationItem{Fact: d.Fact, NodeType: d.Node, StartByte: c.StartByte, EndByte: c.EndByte, Status: status}
		if d.Name == "node" {
			out = append(out, item)
			continue
		}
		if len(names) == 0 {
			item.Status = "NAME_MISSING"
			out = append(out, item)
			continue
		}
		for _, n := range names {
			it := item
			it.Name = &ByteSpan{n.StartByte, n.EndByte}
			out = append(out, it)
		}
	}
	return out
}

// DynamicSQLFact is one dynamic SQL location fact (dynamic-sql-r1).
type DynamicSQLFact struct {
	Construct    string  `json:"construct"`
	ArgumentKind string  `json:"argument_kind"`
	StartByte    uint32  `json:"start_byte"`
	EndByte      uint32  `json:"end_byte"`
	StartPoint   Point   `json:"start_point"`
	EndPoint     Point   `json:"end_point"`
	Variable     *string `json:"variable"`
	Heuristic    bool    `json:"heuristic"`
}

// KnownMiss is a site of a known dynamic-sql-r1 miss: reported with its range so it is
// never silently absent.
type KnownMiss struct {
	Case      string `json:"case"`
	StartByte uint32 `json:"start_byte"`
	EndByte   uint32 `json:"end_byte"`
}

// DynamicSQLFacts is the derived result of one tree.
type DynamicSQLFacts struct {
	Mapping     string           `json:"mapping"`
	Heuristic   bool             `json:"heuristic"`
	Items       []DynamicSQLFact `json:"items"`
	KnownMisses []KnownMiss      `json:"known_misses"`
	Unlisted    []string         `json:"unlisted_known_misses"` // mapping known misses this route cannot observe
}

// Known miss cases of dynamic-sql-r1.
const (
	MissAtDataSource    = "AT_DATA_SOURCE"
	MissWithResultSets  = "WITH_RESULT_SETS"
	MissBatchFirstNoExe = "BATCH_FIRST_CALL_WITHOUT_EXEC"
)

type capIndex struct {
	byMatch map[uint32][]Capture
	byName  map[string][]Capture
}

func indexCaptures(caps []Capture) capIndex {
	x := capIndex{byMatch: map[uint32][]Capture{}, byName: map[string][]Capture{}}
	for _, c := range caps {
		x.byMatch[c.Match] = append(x.byMatch[c.Match], c)
		x.byName[c.Name] = append(x.byName[c.Name], c)
	}
	return x
}

// pairs returns, for every match holding both captures, the (parent, child) pair.
func (x capIndex) pairs(parent, child string) [][2]Capture {
	var out [][2]Capture
	for _, mc := range x.byMatch {
		var p, c *Capture
		for k := range mc {
			switch mc[k].Name {
			case parent:
				p = &mc[k]
			case child:
				c = &mc[k]
			}
		}
		if p != nil && c != nil {
			out = append(out, [2]Capture{*p, *c})
		}
	}
	slices.SortFunc(out, func(a, b [2]Capture) int {
		return cmp.Or(cmp.Compare(a[0].Node, b[0].Node), cmp.Compare(a[1].Node, b[1].Node))
	})
	return out
}

// childrenOf maps each parent node to its captured children in node order.
func (x capIndex) childrenOf(parent, child string) map[int64][]Capture {
	out := map[int64][]Capture{}
	for _, pc := range x.pairs(parent, child) {
		out[pc[0].Node] = append(out[pc[0].Node], pc[1])
	}
	return out
}

func text(src []byte, c Capture) string { return string(src[c.StartByte:c.EndByte]) }

// unquote removes [] or "" quoting of a T-SQL identifier.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '[' && s[len(s)-1] == ']' || s[0] == '"' && s[len(s)-1] == '"') {
		return s[1 : len(s)-1]
	}
	return s
}

// DeriveDynamicSQL derives dynamic-sql-r1 location facts for a tsql or csharp tree from the
// captures of the pack's dynamic SQL query and the tree's source bytes. Facts are in
// document order of their site; tsql known misses are reported with their ranges.
func DeriveDynamicSQL(route string, caps []Capture, src []byte) DynamicSQLFacts {
	x := indexCaptures(caps)
	out := DynamicSQLFacts{Mapping: "dynamic-sql-r1", Items: []DynamicSQLFact{}, KnownMisses: []KnownMiss{}, Unlisted: []string{}}
	switch route {
	case "tsql":
		deriveTSQL(x, src, &out)
	case "csharp":
		out.Heuristic = true
		deriveCSharp(x, src, &out)
		out.Unlisted = []string{MissAtDataSource, MissWithResultSets, MissBatchFirstNoExe}
	}
	return out
}

func fact(construct, kind string, arg Capture, variable *string, heuristic bool) DynamicSQLFact {
	return DynamicSQLFact{construct, kind, arg.StartByte, arg.EndByte, arg.StartPoint, arg.EndPoint, variable, heuristic}
}

func deriveTSQL(x capIndex, src []byte, out *DynamicSQLFacts) {
	children := x.childrenOf("exec.parent", "exec.child")
	procs := map[int64]Capture{}
	for _, pc := range x.pairs("proc.exec", "proc") {
		procs[pc[0].Node] = pc[1]
	}
	refName, refSchema := map[int64]Capture{}, map[int64]Capture{}
	for _, pc := range x.pairs("ref", "ref.name") {
		refName[pc[0].Node] = pc[1]
	}
	for _, pc := range x.pairs("ref.s", "ref.schema") {
		refSchema[pc[0].Node] = pc[1]
	}
	argName, argValue := map[int64]Capture{}, map[int64]Capture{}
	for _, pc := range x.pairs("arg", "arg.name") {
		argName[pc[0].Node] = pc[1]
	}
	for _, pc := range x.pairs("arg.v", "arg.value") {
		argValue[pc[0].Node] = pc[1]
	}
	binOp := map[int64]Capture{}
	for _, pc := range x.pairs("bin", "bin.op") {
		binOp[pc[0].Node] = pc[1]
	}
	fieldName := map[int64]Capture{}
	for _, pc := range x.pairs("field", "field.name") {
		fieldName[pc[0].Node] = pc[1]
	}
	semis := x.byName["semi"]
	semiEnd := func(from uint32) uint32 {
		for _, s := range semis {
			if s.StartByte >= from {
				return s.EndByte
			}
		}
		return from
	}
	withErrors := x.byName["errwith"]
	kind := func(arg Capture) (string, *string) {
		switch arg.Type {
		case "literal":
			t := text(src, arg)
			switch {
			case strings.HasPrefix(t, "N'") || strings.HasPrefix(t, "n'"):
				return "literal_unicode", nil
			case strings.HasPrefix(t, "'"):
				return "literal_char", nil
			}
		case "field", "identifier":
			id := arg
			if arg.Type == "field" {
				n, ok := fieldName[arg.Node]
				if !ok {
					return "other", nil
				}
				id = n
			}
			t := text(src, id)
			if strings.HasPrefix(t, "@") && !strings.HasPrefix(t, "@@") {
				return "variable", &t
			}
			if strings.HasPrefix(t, "@@") {
				return "other", nil
			}
			return "bare_word", nil
		case "binary_expression":
			if op, ok := binOp[arg.Node]; ok && text(src, op) == "+" {
				return "concatenation", nil
			}
		}
		return "other", nil
	}
	var execs []Capture
	execs = append(execs, x.byName["exec"]...)
	slices.SortFunc(execs, func(a, b Capture) int { return cmp.Compare(a.Node, b.Node) })
	for _, e := range execs {
		// WITH RESULT SETS: the statement's options are an ERROR right after the EXEC
		withMiss := slices.ContainsFunc(withErrors, func(w Capture) bool {
			return w.StartByte >= e.EndByte && len(bytes.TrimSpace(src[e.EndByte:w.StartByte])) == 0
		})
		construct := ""
		var arg *Capture
		if p, ok := procs[e.Node]; ok {
			name, ok := refName[p.Node]
			if !ok {
				continue
			}
			n := text(src, name)
			schema, hasSchema := refSchema[p.Node]
			if strings.HasPrefix(n, "@") || !strings.EqualFold(unquote(n), "sp_executesql") || (hasSchema && !strings.EqualFold(unquote(text(src, schema)), "sys")) {
				continue // EXEC @module_var (not dynamic) or a static procedure call
			}
			construct = "SP_EXECUTESQL"
			var args []Capture
			for _, c := range children[e.Node] {
				if c.Type == "exec_argument" {
					args = append(args, c)
				}
			}
			for i, a := range args {
				n, named := argName[a.Node]
				if (i == 0 && !named) || (named && strings.EqualFold(text(src, n), "@stmt")) {
					if v, ok := argValue[a.Node]; ok {
						arg = &v
					}
					break
				}
			}
		} else {
			kids := children[e.Node]
			open := slices.IndexFunc(kids, func(c Capture) bool { return c.Type == "(" && !c.Named })
			if open < 0 {
				continue
			}
			for _, c := range kids[open+1:] {
				if c.Named {
					arg = &c
					break
				}
			}
			construct = "EXEC_PAREN"
			if at := slices.IndexFunc(kids, func(c Capture) bool { return c.Type == "keyword_at" }); at >= 0 {
				construct = "EXEC_PAREN_AT"
				if at+1 < len(kids) && kids[at+1].Type == "identifier" && strings.EqualFold(text(src, kids[at+1]), "DATA_SOURCE") {
					out.KnownMisses = append(out.KnownMisses, KnownMiss{MissAtDataSource, e.StartByte, semiEnd(e.EndByte)})
					continue
				}
			}
		}
		if withMiss {
			out.KnownMisses = append(out.KnownMisses, KnownMiss{MissWithResultSets, e.StartByte, semiEnd(e.EndByte)})
			continue
		}
		if arg == nil {
			continue
		}
		k, v := kind(*arg)
		out.Items = append(out.Items, fact(construct, k, *arg, v, false))
	}
	for _, pc := range x.pairs("err", "err.first") {
		if strings.EqualFold(unquote(text(src, pc[1])), "sp_executesql") {
			out.KnownMisses = append(out.KnownMisses, KnownMiss{MissBatchFirstNoExe, pc[0].StartByte, pc[0].EndByte})
		}
	}
	slices.SortFunc(out.KnownMisses, func(a, b KnownMiss) int { return cmp.Compare(a.StartByte, b.StartByte) })
}

// CommandAPITypes and CommandAPIProperties are csharp-command-api-r1.
var (
	CommandAPITypes      = []string{"SqlCommand", "SqlDataAdapter", "OleDbCommand", "OleDbDataAdapter", "OdbcCommand", "OdbcDataAdapter"}
	CommandAPIProperties = []string{"CommandText"}
)

func deriveCSharp(x capIndex, src []byte, out *DynamicSQLFacts) {
	lastIdent := map[int64]Capture{} // type node -> its last identifier
	for _, n := range []struct{ parent, child string }{{"qn", "qn.name"}, {"aq", "aq.name"}, {"gn", "gn.name"}} {
		for _, pc := range x.pairs(n.parent, n.child) {
			lastIdent[pc[0].Node] = pc[1]
		}
	}
	var resolve func(c Capture) (Capture, bool)
	resolve = func(c Capture) (Capture, bool) {
		if c.Type == "identifier" {
			return c, true
		}
		n, ok := lastIdent[c.Node]
		if !ok {
			return c, false
		}
		return resolve(n)
	}
	binOp := map[int64]Capture{}
	for _, pc := range x.pairs("bin", "bin.op") {
		binOp[pc[0].Node] = pc[1]
	}
	argExpr := map[int64]Capture{} // argument -> its last named child (the expression)
	for _, pc := range x.pairs("arg", "arg.expr") {
		argExpr[pc[0].Node] = pc[1]
	}
	kind := func(v Capture) (string, *string) {
		switch v.Type {
		case "string_literal", "verbatim_string_literal", "raw_string_literal":
			return "literal_unicode", nil
		case "identifier":
			t := text(src, v)
			return "variable", &t
		case "interpolated_string_expression":
			return "concatenation", nil
		case "binary_expression":
			if op, ok := binOp[v.Node]; ok && text(src, op) == "+" {
				return "concatenation", nil
			}
		}
		return "other", nil
	}
	type site struct {
		node  int64
		value Capture
	}
	var sites []site
	types := map[int64]Capture{}
	for _, pc := range x.pairs("oc", "oc.type") {
		types[pc[0].Node] = pc[1]
	}
	for _, pc := range x.pairs("oc.a", "oc.arg") {
		t, ok := types[pc[0].Node]
		if !ok {
			continue
		}
		id, ok := resolve(t)
		if !ok || !slices.Contains(CommandAPITypes, text(src, id)) {
			continue
		}
		if v, ok := argExpr[pc[1].Node]; ok {
			sites = append(sites, site{pc[0].Node, v})
		}
	}
	for _, n := range []struct{ parent, left, right string }{{"ini.as", "ini.left", "ini.right"}, {"ma.as", "ma.name", "ma.right"}} {
		left, right := map[int64]Capture{}, map[int64]Capture{}
		for _, pc := range x.pairs(n.parent, n.left) {
			left[pc[0].Node] = pc[1]
		}
		for _, pc := range x.pairs(n.parent, n.right) {
			right[pc[0].Node] = pc[1]
		}
		for as, l := range left {
			if r, ok := right[as]; ok && slices.Contains(CommandAPIProperties, text(src, l)) {
				sites = append(sites, site{as, r})
			}
		}
	}
	slices.SortFunc(sites, func(a, b site) int { return cmp.Compare(a.node, b.node) })
	for _, s := range sites {
		k, v := kind(s.value)
		out.Items = append(out.Items, fact("CSHARP_COMMAND", k, s.value, v, true))
	}
}
