package kit

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
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

// DeclarationQuery generates the declaration query of a pack route from its items. Per item
// i the declaration node is @decl.i and, unless the locator is "node", for every prefix of
// the locator path (depth d = 1..len) one nested pattern captures the owner as @c.i.d.0
// and the node at each level k as @c.i.d.k. The last level of a pattern takes any node
// (field: F as "F: _", child/children: T as "(T)"), so the first candidate of a level is
// chosen whether or not deeper levels exist below it. The pack stores the generated text;
// this function is its reproducible origin.
func DeclarationQuery(items []NativeDeclaration) string {
	var b strings.Builder
	for i, d := range items {
		n := strconv.Itoa(i)
		b.WriteString("(" + d.Node + ") @decl." + n + "\n")
		if d.Name == "node" {
			continue
		}
		segs := splitPath(d.Name)
		for depth := 1; depth <= len(segs); depth++ {
			pre := "@c." + n + "." + strconv.Itoa(depth) + "."
			inner := ""
			for k := depth - 1; k >= 0; k-- {
				kind, arg, _ := strings.Cut(segs[k], ":")
				node := "(" + arg + inner + ")"
				if kind == "field" {
					node = arg + ": _"
					if inner != "" {
						node = arg + ": (_" + inner + ")"
					}
				}
				inner = " " + node + " " + pre + strconv.Itoa(k+1)
			}
			b.WriteString("(" + d.Node + inner + ") " + pre + "0\n")
		}
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
// the pack's declaration query, following the S05 locator walk: from the declaration node,
// each level takes, below every current node, the first candidate (field:F, child:T) or
// every candidate (children:T) in document order, where the candidates of a level are the
// nodes the depth-k patterns captured below that node; a level without a candidate ends the
// walk. One item per name in document order; NAME_MISSING without a name; HAS_ERROR when
// the declaration node has an error; items in declaration-node preorder, then item order.
func DeriveDeclarations(items []NativeDeclaration, caps []Capture) []DeclarationItem {
	decl := map[int]map[int64]Capture{} // item -> node -> capture
	// item -> depth -> chains (level 0 owner .. level depth)
	chains := map[int]map[int][][]Capture{}
	matches := map[uint32][]Capture{}
	for _, c := range caps {
		parts := strings.Split(c.Name, ".")
		switch {
		case len(parts) == 2 && parts[0] == "decl":
			i, err := strconv.Atoi(parts[1])
			if err != nil || i < 0 || i >= len(items) {
				continue
			}
			if decl[i] == nil {
				decl[i] = map[int64]Capture{}
			}
			decl[i][c.Node] = c
		case len(parts) == 4 && parts[0] == "c":
			matches[c.Match] = append(matches[c.Match], c)
		}
	}
	for _, mc := range matches {
		var item, depth = -1, -1
		levels := map[int]Capture{}
		for _, c := range mc {
			parts := strings.Split(c.Name, ".")
			i, e1 := strconv.Atoi(parts[1])
			d, e2 := strconv.Atoi(parts[2])
			k, e3 := strconv.Atoi(parts[3])
			if e1 != nil || e2 != nil || e3 != nil || i < 0 || i >= len(items) || k < 0 || k > d || (item >= 0 && (i != item || d != depth)) {
				item = -2
				break
			}
			item, depth, levels[k] = i, d, c
		}
		if item < 0 || len(levels) != depth+1 {
			continue
		}
		ch := make([]Capture, depth+1)
		for k := range ch {
			ch[k] = levels[k]
		}
		if chains[item] == nil {
			chains[item] = map[int][][]Capture{}
		}
		chains[item][depth] = append(chains[item][depth], ch)
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
		status := "PASS"
		if c.HasError {
			status = "HAS_ERROR"
		}
		item := DeclarationItem{Fact: d.Fact, NodeType: d.Node, StartByte: c.StartByte, EndByte: c.EndByte, Status: status}
		if d.Name == "node" {
			out = append(out, item)
			continue
		}
		segs := splitPath(d.Name)
		// cur holds the walked paths (node chains from the owner) at the current level
		cur := [][]Capture{{c}}
		for lv := 1; lv <= len(segs) && len(cur) > 0; lv++ {
			var next [][]Capture
			for _, path := range cur {
				var cands [][]Capture
				for _, ch := range chains[k.item][lv] {
					same := true
					for j := range path {
						same = same && ch[j].Node == path[j].Node
					}
					if same {
						cands = append(cands, ch)
					}
				}
				slices.SortFunc(cands, func(a, b []Capture) int { return cmp.Compare(a[lv].Node, b[lv].Node) })
				cands = slices.CompactFunc(cands, func(a, b []Capture) bool { return a[lv].Node == b[lv].Node })
				if len(cands) > 0 && !strings.HasPrefix(segs[lv-1], "children:") {
					cands = cands[:1]
				}
				next = append(next, cands...)
			}
			cur = next
		}
		if len(cur) == 0 {
			item.Status = "NAME_MISSING"
			out = append(out, item)
			continue
		}
		names := make([]Capture, 0, len(cur))
		for _, p := range cur {
			names = append(names, p[len(p)-1])
		}
		slices.SortFunc(names, func(a, b Capture) int { return cmp.Compare(a.Node, b.Node) })
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

// pairs returns, for every match holding the parent capture, one (parent, child) pair per
// child capture of that match (a pattern can capture several children in one match).
func (x capIndex) pairs(parent, child string) [][2]Capture {
	var out [][2]Capture
	for _, mc := range x.byMatch {
		var p *Capture
		for k := range mc {
			if mc[k].Name == parent {
				p = &mc[k]
			}
		}
		if p == nil {
			continue
		}
		for _, c := range mc {
			if c.Name == child {
				out = append(out, [2]Capture{*p, c})
			}
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

// srcText is a tree's original bytes with their declared encoding: text comparisons of
// the dynamic SQL rules decode the node bytes, so UTF-16 and CP949 sources are judged the
// same way as UTF-8 ones.
type srcText struct {
	b   []byte
	enc string
}

func (s srcText) span(start, end uint32) string {
	b := s.b[start:end]
	switch s.enc {
	case EncodingUTF16LE, EncodingUTF16BE:
		u := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			if s.enc == EncodingUTF16LE {
				u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
			} else {
				u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
			}
		}
		return string(utf16.Decode(u))
	case EncodingCP949:
		var r strings.Builder
		for i := 0; i < len(b); i++ {
			if b[i] < 0x80 || i+1 >= len(b) {
				r.WriteByte(b[i])
				continue
			}
			if c, ok := CP949Rune(b[i], b[i+1]); ok {
				r.WriteRune(c)
				i++
				continue
			}
			r.WriteRune(utf8.RuneError)
		}
		return r.String()
	}
	return string(b)
}

func text(src srcText, c Capture) string { return src.span(c.StartByte, c.EndByte) }

// unquote removes [] or "" quoting of a T-SQL identifier.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '[' && s[len(s)-1] == ']' || s[0] == '"' && s[len(s)-1] == '"') {
		return s[1 : len(s)-1]
	}
	return s
}

// DeriveDynamicSQL derives dynamic-sql-r1 location facts for a tsql or csharp tree from the
// captures of the pack's dynamic SQL query and the tree's source bytes in their encoding
// (UTF-8, UTF-16LE/BE or CP949; text is compared after decoding, ranges stay original
// bytes). Facts are in document order of their site; tsql known misses are reported with
// their ranges. Extra nodes (comments) are never a construct part or an argument.
func DeriveDynamicSQL(route, enc string, caps []Capture, srcBytes []byte) DynamicSQLFacts {
	src := srcText{srcBytes, enc}
	caps = slices.DeleteFunc(slices.Clone(caps), func(c Capture) bool { return c.Extra })
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

func deriveTSQL(x capIndex, src srcText, out *DynamicSQLFacts) {
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
	var execs []Capture
	execs = append(execs, x.byName["exec"]...)
	slices.SortFunc(execs, func(a, b Capture) int { return cmp.Compare(a.Node, b.Node) })
	// a known miss runs from the EXEC to the end of the first ";" after it that starts
	// before the next EXEC; without one it is the EXEC alone
	semiEnd := func(e Capture) uint32 {
		limit := uint32(len(src.b))
		for _, n := range execs {
			if n.StartByte > e.StartByte {
				limit = n.StartByte
				break
			}
		}
		for _, s := range semis {
			if s.StartByte >= e.EndByte && s.StartByte < limit {
				return s.EndByte
			}
		}
		return e.EndByte
	}
	withErrors := x.byName["errwith"]
	withResultSets := regexp.MustCompile(`(?i)^WITH\s+RESULT\s+SETS\b`)
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
	for _, e := range execs {
		// WITH RESULT SETS: the statement's options are an ERROR right after the EXEC (only
		// whitespace between) whose text begins WITH RESULT SETS
		withMiss := slices.ContainsFunc(withErrors, func(w Capture) bool {
			return w.StartByte >= e.EndByte && strings.TrimSpace(src.span(e.EndByte, w.StartByte)) == "" && withResultSets.MatchString(text(src, w))
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
					out.KnownMisses = append(out.KnownMisses, KnownMiss{MissAtDataSource, e.StartByte, semiEnd(e)})
					continue
				}
			}
		}
		if withMiss {
			out.KnownMisses = append(out.KnownMisses, KnownMiss{MissWithResultSets, e.StartByte, semiEnd(e)})
			continue
		}
		if arg == nil {
			continue
		}
		k, v := kind(*arg)
		out.Items = append(out.Items, fact(construct, k, *arg, v, false))
	}
	// the first call of a batch without EXEC: an ERROR that starts with sp_executesql,
	// optionally qualified by sys (brackets or quotes allowed)
	firstCall := regexp.MustCompile(`(?i)^(?:(?:\[sys\]|"sys"|sys)\s*\.\s*)?(?:\[sp_executesql\]|"sp_executesql"|sp_executesql)\b`)
	for _, pc := range x.pairs("err", "err.first") {
		if firstCall.MatchString(text(src, pc[0])) {
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

func deriveCSharp(x capIndex, src srcText, out *DynamicSQLFacts) {
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
