package kit

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
)

// The large fixtures (#362 scope 6) are generated: header + block with {N} replaced by
// 0..repeat-1 + footer. This file derives the full CST and the `facts.declarations` occurrences
// of such a template from the C# grammar rules alone (tree-sitter-c-sharp 0.23.5 grammar.json),
// without any native output. It parses only the exact subset the valid template uses and refuses
// everything else: error-recovery trees (the two `*-errors` fixtures contain `x = x + ;`) depend
// on the generated parse tables, so they are reported NOT_DERIVABLE instead of guessed.

// LargeTemplate is one registered fixture's generation rule (src/contracts/native-large-fixtures.json).
type LargeTemplate struct {
	ID     string
	Header string
	Block  string
	Footer string
	Repeat int
	Bytes  uint64
	SHA256 string
}

const (
	// LargeNotDerivable is the code of a template outside the derivable subset.
	LargeNotDerivable = "LARGE_TEMPLATE_NOT_DERIVABLE"
	// Bits of the canonical CST row flags.
	LargeRowNamed   = 1
	LargeRowExtra   = 2
	LargeRowMissing = 4
	LargeRowError   = 8
	LargeRowHasErr  = 16
)

// Source reproduces the fixture bytes and checks the registered size and SHA-256.
func (t LargeTemplate) Source() ([]byte, error) {
	if t.Bytes == 0 || t.Bytes > 33554432 || t.Repeat < 0 || t.Repeat > 10000000 || t.Block == "" {
		return nil, largeFault("LARGE_TEMPLATE_SIZE")
	}
	var b bytes.Buffer
	b.Grow(int(t.Bytes))
	b.WriteString(t.Header)
	for n := 0; n < t.Repeat; n++ {
		b.WriteString(strings.ReplaceAll(t.Block, "{N}", strconv.Itoa(n)))
		if uint64(b.Len()) > t.Bytes {
			return nil, largeFault("LARGE_TEMPLATE_SIZE")
		}
	}
	b.WriteString(t.Footer)
	sum := sha256.Sum256(b.Bytes())
	if uint64(b.Len()) != t.Bytes || hex.EncodeToString(sum[:]) != t.SHA256 {
		return nil, largeFault("LARGE_TEMPLATE_IDENTITY")
	}
	return b.Bytes(), nil
}

// LargeRow is one canonical CST row: a node of the full tree in preorder.
type LargeRow struct {
	Index              uint32
	Parent             int32 // -1 for the root
	StartByte, EndByte uint32
	StartRow, StartCol uint32
	EndRow, EndCol     uint32
	Flags              uint8
	Type, Field        string
	Depth              uint32 // root is 1; not part of the encoding
}

// Append appends the canonical encoding (big endian) shared with src/dev/large-api/session-cst.c.
func (r LargeRow) Append(b []byte) []byte {
	b = binary.BigEndian.AppendUint32(b, r.Index)
	b = binary.BigEndian.AppendUint32(b, uint32(r.Parent))
	for _, v := range []uint32{r.StartByte, r.EndByte, r.StartRow, r.StartCol, r.EndRow, r.EndCol} {
		b = binary.BigEndian.AppendUint32(b, v)
	}
	b = append(b, r.Flags)
	b = binary.BigEndian.AppendUint16(b, uint16(len(r.Type)))
	b = append(b, r.Type...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(r.Field)))
	return append(b, r.Field...)
}

// LargeCapture is one `facts.declarations` occurrence.
type LargeCapture struct {
	Name, Type         string
	StartByte, EndByte uint32
	StartRow, StartCol uint32
	EndRow, EndCol     uint32
}

// Record is the canonical occurrence encoding used by the multiset digest.
func (c LargeCapture) Record() []byte {
	var b []byte
	b = binary.BigEndian.AppendUint16(b, uint16(len(c.Name)))
	b = append(b, c.Name...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(c.Type)))
	b = append(b, c.Type...)
	for _, v := range []uint32{c.StartByte, c.EndByte, c.StartRow, c.StartCol, c.EndRow, c.EndCol} {
		b = binary.BigEndian.AppendUint32(b, v)
	}
	return b
}

func largeFault(code string) error { return fail(KindInvalidInput, code, LargeExpectedPath, nil) }

type ltTok struct {
	text       string
	kind       byte // i identifier or keyword, n integer, p punctuation, c line comment
	start, end uint32
	srow, scol uint32
	erow, ecol uint32
}

// ltTokenize splits the C# subset into tokens; anything else is not derivable.
func ltTokenize(s string) ([]ltTok, error) {
	var toks []ltTok
	row, col := uint32(0), uint32(0)
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\n' {
			row, col = row+1, 0
			i++
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' {
			col++
			i++
			continue
		}
		j := i
		kind := byte('p')
		switch {
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			for j < len(s) && s[j] != '\n' && s[j] != '\r' {
				j++
			}
			kind = 'c'
		case c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z':
			for j < len(s) && (s[j] == '_' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= '0' && s[j] <= '9') {
				j++
			}
			kind = 'i'
		case c >= '0' && c <= '9':
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			kind = 'n'
		case strings.IndexByte("{}();=+", c) >= 0:
			j++
		default:
			return nil, largeFault(LargeNotDerivable)
		}
		// columns are byte columns; a non-ASCII byte would also be legal but is outside the subset
		for k := i; k < j; k++ {
			if s[k] >= 0x80 {
				return nil, largeFault(LargeNotDerivable)
			}
		}
		toks = append(toks, ltTok{text: s[i:j], kind: kind, start: uint32(i), end: uint32(j), srow: row, scol: col, erow: row, ecol: col + uint32(j-i)})
		col += uint32(j - i)
		i = j
	}
	return toks, nil
}

// ltNode is a derived tree node over a token range.
type ltNode struct {
	typ, field  string
	flags       uint8
	first, last int // token indexes
	kids        []*ltNode
}

type ltParser struct {
	toks []ltTok
	pos  int
	err  bool
}

func (p *ltParser) tok() *ltTok {
	if p.pos >= len(p.toks) {
		p.err = true
		return &ltTok{}
	}
	return &p.toks[p.pos]
}

// leaf consumes one token. text "" accepts any token of the kind; typ "" makes it anonymous.
func (p *ltParser) leaf(kind byte, text, typ, field string) *ltNode {
	t := p.tok()
	if p.err || t.kind != kind || (text != "" && t.text != text) {
		p.err = true
		return &ltNode{}
	}
	n := &ltNode{typ: typ, field: field, first: p.pos, last: p.pos}
	if typ == "" {
		n.typ = t.text
	} else {
		n.flags = LargeRowNamed
	}
	p.pos++
	return n
}

func (p *ltParser) kw(text, field string) *ltNode { return p.leaf('i', text, "", field) }
func (p *ltParser) punct(text string) *ltNode     { return p.leaf('p', text, "", "") }
func (p *ltParser) ident(field string) *ltNode    { return p.leaf('i', "", "identifier", field) }
func (p *ltParser) predefined(field string) *ltNode {
	return p.leaf('i', "int", "predefined_type", field)
}

func (p *ltParser) node(typ, field string, kids ...*ltNode) *ltNode {
	if p.err || len(kids) == 0 {
		p.err = true
		return &ltNode{}
	}
	return &ltNode{typ: typ, field: field, flags: LargeRowNamed, first: kids[0].first, last: kids[len(kids)-1].last, kids: kids}
}

func (p *ltParser) modifier(text string) *ltNode { return p.node("modifier", "", p.kw(text, "")) }

func (p *ltParser) statement() *ltNode {
	t := p.tok()
	switch {
	case t.kind == 'i' && t.text == "int": // int x = a;
		typ := p.predefined("type")
		decl := p.node("variable_declarator", "", p.ident("name"), p.punct("="), p.ident(""))
		return p.node("local_declaration_statement", "", p.node("variable_declaration", "", typ, decl), p.punct(";"))
	case t.kind == 'i' && t.text == "return":
		return p.node("return_statement", "", p.kw("return", ""), p.ident(""), p.punct(";"))
	case t.kind == 'i': // x = x + K;
		left := p.ident("left")
		op := p.punct("=")
		op.field = "operator"
		bl := p.ident("left")
		bop := p.punct("+")
		bop.field = "operator"
		br := p.leaf('n', "", "integer_literal", "right")
		bin := p.node("binary_expression", "right", bl, bop, br)
		return p.node("expression_statement", "", p.node("assignment_expression", "", left, op, bin), p.punct(";"))
	}
	p.err = true
	return &ltNode{}
}

func (p *ltParser) namespace() *ltNode {
	kw := p.kw("namespace", "")
	name := p.ident("name")
	open := p.punct("{")
	class := p.class()
	list := p.node("declaration_list", "body", open, class, p.punct("}"))
	return p.node("namespace_declaration", "", kw, name, list)
}

func (p *ltParser) class() *ltNode {
	mod := p.modifier("public")
	kw := p.kw("class", "")
	name := p.ident("name")
	open := p.punct("{")
	// private int f;
	fmod := p.modifier("private")
	vd := p.node("variable_declaration", "", p.predefined("type"), p.node("variable_declarator", "", p.ident("name")))
	field := p.node("field_declaration", "", fmod, vd, p.punct(";"))
	// public int M(int a) { ... }
	mmod := p.modifier("public")
	ret := p.predefined("returns")
	mname := p.ident("name")
	params := p.node("parameter_list", "parameters", p.punct("("), p.node("parameter", "", p.predefined("type"), p.ident("name")), p.punct(")"))
	kids := []*ltNode{p.punct("{")}
	for !p.err && !(p.tok().kind == 'p' && p.tok().text == "}") {
		kids = append(kids, p.statement())
	}
	kids = append(kids, p.punct("}"))
	body := p.node("block", "body", kids...)
	method := p.node("method_declaration", "", mmod, ret, mname, params, body)
	list := p.node("declaration_list", "body", open, field, method, p.punct("}"))
	return p.node("class_declaration", "", mod, kw, name, list)
}

// ltBlockTree derives the namespace subtree of one block text.
func ltBlockTree(text string) ([]ltTok, *ltNode, error) {
	toks, err := ltTokenize(text)
	if err != nil {
		return nil, nil, err
	}
	p := &ltParser{toks: toks}
	n := p.namespace()
	if p.err || p.pos != len(toks) {
		return nil, nil, largeFault(LargeNotDerivable)
	}
	return toks, n, nil
}

// ltFrame binds a token list to its absolute byte and row origin.
type ltFrame struct {
	toks  []ltTok
	byte0 uint32
	row0  uint32
}

// LargeWalk derives every CST row of the template in preorder and calls emit for each; onDecl
// (optional) receives every declaration node with a row lookup for its descendants. The root ends
// at the source end (the end-of-file padding belongs to it). It returns the row count.
func (t LargeTemplate) LargeWalk(emit func(LargeRow), onDecl func(n *ltNode, row func(*ltNode) LargeRow)) (uint64, error) {
	src, err := t.Source()
	if err != nil {
		return 0, err
	}
	headToks, err := ltTokenize(t.Header)
	if err != nil || len(headToks) == 0 || headToks[0].start != 0 {
		return 0, largeFault(LargeNotDerivable)
	}
	// header: [comment] using System;
	hp := &ltParser{toks: headToks}
	var headKids []*ltNode
	if hp.tok().kind == 'c' {
		c := hp.leaf('c', "", "comment", "")
		c.flags |= LargeRowExtra
		headKids = append(headKids, c)
	}
	headKids = append(headKids, hp.node("using_directive", "", hp.kw("using", ""), hp.ident(""), hp.punct(";")))
	if hp.err || hp.pos != len(headToks) {
		return 0, largeFault(LargeNotDerivable)
	}
	footToks, err := ltTokenize(t.Footer)
	if err != nil {
		return 0, err
	}
	fp := &ltParser{toks: footToks}
	foot := fp.leaf('c', "", "comment", "")
	foot.flags |= LargeRowExtra
	if fp.err || fp.pos != len(footToks) {
		return 0, largeFault(LargeNotDerivable)
	}
	var index uint32
	rowOf := func(f ltFrame, n *ltNode, parent int32, depth uint32) LargeRow {
		a, b := f.toks[n.first], f.toks[n.last]
		return LargeRow{Index: index, Parent: parent, StartByte: f.byte0 + a.start, EndByte: f.byte0 + b.end,
			StartRow: f.row0 + a.srow, StartCol: a.scol, EndRow: f.row0 + b.erow, EndCol: b.ecol, Flags: n.flags, Type: n.typ, Field: n.field, Depth: depth}
	}
	var walk func(f ltFrame, n *ltNode, parent int32, depth uint32)
	walk = func(f ltFrame, n *ltNode, parent int32, depth uint32) {
		self := int32(index)
		emit(rowOf(f, n, parent, depth))
		index++
		if onDecl != nil {
			if _, ok := largeDeclarations[n.typ]; ok {
				onDecl(n, func(x *ltNode) LargeRow { return rowOf(f, x, 0, 0) })
			}
		}
		for _, k := range n.kids {
			walk(f, k, self, depth+1)
		}
	}
	// the root covers the whole source
	srcRows := uint32(bytes.Count(src, []byte("\n")))
	lastNL := bytes.LastIndexByte(src, '\n')
	emit(LargeRow{Index: 0, Parent: -1, StartByte: 0, EndByte: uint32(len(src)), StartRow: 0, StartCol: 0, EndRow: srcRows,
		EndCol: uint32(len(src) - lastNL - 1), Flags: LargeRowNamed, Type: "compilation_unit", Depth: 1})
	index = 1
	hf := ltFrame{toks: headToks}
	for _, k := range headKids {
		walk(hf, k, 0, 2)
	}
	byte0, row0 := uint32(len(t.Header)), uint32(strings.Count(t.Header, "\n"))
	blockLines := uint32(strings.Count(t.Block, "\n"))
	for n := 0; n < t.Repeat; n++ {
		text := strings.ReplaceAll(t.Block, "{N}", strconv.Itoa(n))
		toks, tree, err := ltBlockTree(text)
		if err != nil {
			return 0, err
		}
		walk(ltFrame{toks: toks, byte0: byte0, row0: row0}, tree, 0, 2)
		byte0 += uint32(len(text))
		row0 += blockLines
	}
	walk(ltFrame{toks: footToks, byte0: byte0, row0: row0}, foot, 0, 2)
	return uint64(index), nil
}

// LargeCSTDigests holds the per-page digests and the complete footer values of the full CST.
type LargeCSTDigests struct {
	Nodes      uint64
	Pages      []string
	PagesChain string // SHA-256 over the page digests, each followed by "\n"
	AllRows    string
	RowBytes   uint64
	MaxDepth   uint32
}

// CSTDigests streams the canonical rows into digests per page of pageRows rows.
func (t LargeTemplate) CSTDigests(pageRows uint64) (LargeCSTDigests, error) {
	var out LargeCSTDigests
	if pageRows == 0 {
		return out, largeFault("LARGE_PAGE_SIZE")
	}
	all, page := sha256.New(), sha256.New()
	var buf []byte
	var inPage uint64
	flushPage := func() {
		out.Pages = append(out.Pages, hex.EncodeToString(page.Sum(nil)))
		page.Reset()
		inPage = 0
	}
	n, err := t.LargeWalk(func(r LargeRow) {
		buf = r.Append(buf[:0])
		page.Write(buf)
		all.Write(buf)
		out.RowBytes += uint64(len(buf))
		out.MaxDepth = max(out.MaxDepth, r.Depth)
		if inPage++; inPage == pageRows {
			flushPage()
		}
	}, nil)
	if err != nil {
		return out, err
	}
	if inPage > 0 {
		flushPage()
	}
	chain := sha256.New()
	for _, p := range out.Pages {
		chain.Write([]byte(p + "\n"))
	}
	out.Nodes, out.PagesChain, out.AllRows = n, hex.EncodeToString(chain.Sum(nil)), hex.EncodeToString(all.Sum(nil))
	return out, nil
}

// largeDeclarations lists, per declaration node type, the capture names of each pattern of
// `facts.declarations` (src/contracts/fact-query-pack.json, csharp): the first capture is the
// declaration node or its declarator chain; largeCaptureNodes finds the nodes each pattern captures.
var largeDeclarations = map[string][][]string{
	"namespace_declaration": {{"c.6.0.0"}, {"c.6.1.0", "c.6.1.1"}},
	"class_declaration":     {{"c.0.0.0"}, {"c.0.1.0", "c.0.1.1"}},
	"method_declaration":    {{"c.8.0.0"}, {"c.8.1.0", "c.8.1.1"}},
	"field_declaration":     {{"c.14.0.0"}, {"c.14.1.0", "c.14.1.1"}, {"c.14.3.1", "c.14.3.2", "c.14.3.3"}},
}

func ltKid(n *ltNode, typ, field string) *ltNode {
	if n == nil {
		return nil
	}
	for _, k := range n.kids {
		if (typ == "" || k.typ == typ) && (field == "" || k.field == field) {
			return k
		}
	}
	return nil
}

// largeCaptureNodes returns the nodes one pattern captures, in the order of its capture names.
func largeCaptureNodes(n *ltNode, pattern int) []*ltNode {
	if n.typ == "field_declaration" {
		vd := ltKid(n, "variable_declaration", "")
		switch pattern {
		case 0:
			return []*ltNode{n}
		case 1:
			return []*ltNode{n, vd}
		}
		// `(field_declaration (variable_declaration (variable_declarator name: _ @c.14.3.3) @c.14.3.2) @c.14.3.1)`:
		// @c.14.3.1 follows the variable_declaration's closing parenthesis, not the field_declaration's
		dr := ltKid(vd, "variable_declarator", "")
		return []*ltNode{vd, dr, ltKid(dr, "identifier", "name")}
	}
	if pattern == 0 {
		return []*ltNode{n}
	}
	return []*ltNode{n, ltKid(n, "identifier", "name")}
}

// LargeOccurrences are the derived `facts.declarations` captures of the template. The runtime
// order is not derived: the multiset is (Digest over the sorted canonical records).
type LargeOccurrences struct {
	Count  uint64
	ByName map[string]uint64
	Digest string
}

// Occurrences derives the declaration captures of the template.
func (t LargeTemplate) Occurrences() (LargeOccurrences, error) {
	out := LargeOccurrences{ByName: map[string]uint64{}}
	var records [][]byte
	var bad bool
	_, err := t.LargeWalk(func(LargeRow) {}, func(n *ltNode, row func(*ltNode) LargeRow) {
		for p, names := range largeDeclarations[n.typ] {
			nodes := largeCaptureNodes(n, p)
			if len(nodes) != len(names) {
				bad = true
				return
			}
			for i, x := range nodes {
				if x == nil {
					bad = true
					return
				}
				r := row(x)
				records = append(records, LargeCapture{Name: names[i], Type: x.typ, StartByte: r.StartByte, EndByte: r.EndByte,
					StartRow: r.StartRow, StartCol: r.StartCol, EndRow: r.EndRow, EndCol: r.EndCol}.Record())
				out.ByName[names[i]]++
			}
		}
	})
	if err != nil {
		return out, err
	}
	if bad {
		return out, largeFault(LargeNotDerivable)
	}
	out.Count, out.Digest = uint64(len(records)), LargeMultisetDigest(records)
	return out, nil
}

// LargeMultisetDigest is the SHA-256 over the lexicographically sorted records, each preceded by
// its uint32 big-endian length.
func LargeMultisetDigest(records [][]byte) string {
	slices.SortFunc(records, bytes.Compare)
	h := sha256.New()
	for _, r := range records {
		h.Write(binary.BigEndian.AppendUint32(nil, uint32(len(r))))
		h.Write(r)
	}
	return hex.EncodeToString(h.Sum(nil))
}
