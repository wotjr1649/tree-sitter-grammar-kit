package kit

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The SVC inline derivation (#362): the CST of an inline C# fragment and the captures of the two
// fact queries are derived from the C# grammar rules (tree-sitter-c-sharp 0.23.5 grammar.json) and
// the query source text alone. It parses only the exact C# subset the valid SVC fragments use and
// refuses everything else, so nothing here reads native output. Error recovery (the fragment is not
// in the language) and the external-scanner preprocessor handling are never guessed.

// Classes of a refused fragment.
const (
	siSyntax      = "syntax"      // no grammar rule accepts the token at that position: the real parser must recover
	siLexical     = "lexical"     // a token that no C# lexical rule completes (unterminated string)
	siUnsupported = "unsupported" // valid or unknown C# outside the derivable subset
	siPreproc     = "preproc"     // a preprocessor directive: the external scanner decides, not the grammar rules alone
)

type siError struct{ class, msg string }

func (e *siError) Error() string { return e.class + ": " + e.msg }

type siTok struct {
	text           string
	kind           byte // w word, n number, p punctuation, q string quote, c string content, e escape sequence, m line comment
	start, end     uint32
	sr, sc, er, ec uint32
}

var siEscape = regexp.MustCompile(`^(\\x[0-9a-fA-F]{1,4}|\\u[0-9a-fA-F]{4}|\\U[0-9a-fA-F]{8}|\\[abefnrtv'"\\?0])`)

// siTokenize splits fragment bytes into the lexical tokens of the subset. b0, r0 and c0 are the
// absolute byte, row and byte column of the first byte.
func siTokenize(src []byte, b0, r0, c0 uint32) ([]siTok, error) {
	var toks []siTok
	row, col := r0, c0
	adv := func(n int) {
		for _, b := range src[:n] {
			if b == '\n' {
				row, col = row+1, 0
			} else {
				col++
			}
		}
	}
	pos := uint32(0)
	emit := func(kind byte, n int) {
		sr, sc, start := row, col, b0+pos
		adv(n)
		toks = append(toks, siTok{text: string(src[:n]), kind: kind, start: start, end: start + uint32(n), sr: sr, sc: sc, er: row, ec: col})
		src, pos = src[n:], pos+uint32(n)
	}
	skip := func(n int) { adv(n); src, pos = src[n:], pos+uint32(n) }
	isUnicodeLineBreak := func(b []byte) bool {
		return bytes.Contains(b, []byte("\u0085")) || bytes.Contains(b, []byte(" ")) || bytes.Contains(b, []byte(" "))
	}
	for len(src) > 0 {
		c := src[0]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v':
			skip(1)
		case c >= 0x80:
			return nil, &siError{siUnsupported, "non-ASCII byte outside a string or comment"}
		case c == '/' && len(src) > 1 && src[1] == '/':
			n := bytes.IndexAny(src, "\r\n")
			if n < 0 {
				n = len(src)
			}
			if isUnicodeLineBreak(src[:n]) {
				return nil, &siError{siUnsupported, "Unicode line break inside a comment"}
			}
			emit('m', n)
		case c == '/' && len(src) > 1 && src[1] == '*':
			return nil, &siError{siUnsupported, "block comment"}
		case c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z':
			n := 1
			for n < len(src) && (src[n] == '_' || src[n] >= 'A' && src[n] <= 'Z' || src[n] >= 'a' && src[n] <= 'z' || src[n] >= '0' && src[n] <= '9') {
				n++
			}
			emit('w', n)
		case c >= '0' && c <= '9':
			n := 1
			for n < len(src) && src[n] >= '0' && src[n] <= '9' {
				n++
			}
			if n < len(src) && (src[n] == '_' || src[n] == '.' || src[n] >= 'A' && src[n] <= 'Z' || src[n] >= 'a' && src[n] <= 'z') {
				return nil, &siError{siUnsupported, "numeric literal form outside the subset"}
			}
			emit('n', n)
		case c == '"':
			emit('q', 1)
			for {
				if len(src) == 0 || src[0] == '\n' || src[0] == '\r' {
					return nil, &siError{siLexical, "unterminated string literal"}
				}
				if src[0] == '"' {
					emit('q', 1)
					break
				}
				if src[0] == '\\' {
					m := siEscape.Find(src)
					if m == nil {
						return nil, &siError{siLexical, "invalid escape sequence"}
					}
					emit('e', len(m))
					continue
				}
				n := bytes.IndexAny(src, "\"\\\r\n")
				if n < 0 {
					n = len(src)
				}
				if isUnicodeLineBreak(src[:n]) {
					return nil, &siError{siUnsupported, "Unicode line break inside a string"}
				}
				emit('c', n)
			}
		case c < 0x21 || c == 0x7f || c == '@' || c == '$' || c == '#' || c == '\\' || c == '\'' || c == '`':
			return nil, &siError{siUnsupported, "character outside the subset"}
		default:
			emit('p', 1) // every other ASCII punctuation is a single token; the parser accepts only the subset's
		}
	}
	return toks, nil
}

// siNode is one derived node of the CST.
type siNode struct {
	typ, field     string
	flags          uint8
	start, end     uint32
	sr, sc, er, ec uint32
	kids           []*siNode
	index          int
}

const siExtraNamed = LargeRowNamed | LargeRowExtra

var siReserved = map[string]bool{}
var siPredefined = map[string]bool{}
var siModifiers = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`abstract as base bool break byte case catch char checked class const continue decimal default delegate do double else enum event explicit extern false finally fixed float for foreach goto if implicit in int interface internal is lock long namespace new null object operator out override params private protected public readonly ref return sbyte sealed short sizeof stackalloc static string struct switch this throw true try typeof uint ulong unchecked unsafe ushort using virtual void volatile while`) {
		siReserved[w] = true
	}
	for _, w := range strings.Fields(`bool byte char decimal double float int long object sbyte short string uint ulong ushort nint nuint void`) {
		siPredefined[w] = true
	}
	for _, w := range strings.Fields(`abstract async const extern file fixed internal new override partial private protected public readonly required sealed static unsafe virtual volatile`) {
		siModifiers[w] = true
	}
}

type siParser struct {
	toks []siTok
	pos  int
}

func (p *siParser) fail(msg string) {
	where := "end of fragment"
	if p.pos < len(p.toks) {
		t := p.toks[p.pos]
		where = fmt.Sprintf("%q at byte %d", t.text, t.start)
	}
	panic(&siError{siSyntax, msg + " before " + where})
}

func (p *siParser) peek(n int) *siTok {
	if p.pos+n < len(p.toks) {
		return &p.toks[p.pos+n]
	}
	return nil
}

func (p *siParser) isPunct(text string, n int) bool {
	t := p.peek(n)
	return t != nil && t.kind == 'p' && t.text == text
}

func (p *siParser) isWord(text string, n int) bool {
	t := p.peek(n)
	return t != nil && t.kind == 'w' && t.text == text
}

func siLeaf(t *siTok, typ, field string, named bool) *siNode {
	n := &siNode{typ: typ, field: field, start: t.start, end: t.end, sr: t.sr, sc: t.sc, er: t.er, ec: t.ec}
	if named {
		n.flags = LargeRowNamed
	}
	return n
}

func (p *siParser) take(kind byte, what string) *siTok {
	t := p.peek(0)
	if t == nil || t.kind != kind {
		p.fail("expected " + what)
	}
	p.pos++
	return t
}

func (p *siParser) punct(text string) *siNode {
	t := p.peek(0)
	if t == nil || t.kind != 'p' || t.text != text {
		p.fail("expected " + strconv.Quote(text))
	}
	p.pos++
	return siLeaf(t, text, "", false)
}

func (p *siParser) kw(text string) *siNode {
	t := p.peek(0)
	if t == nil || t.kind != 'w' || t.text != text {
		p.fail("expected " + strconv.Quote(text))
	}
	p.pos++
	return siLeaf(t, text, "", false)
}

func (p *siParser) ident(field string) *siNode {
	t := p.peek(0)
	if t == nil || t.kind != 'w' || siReserved[t.text] {
		p.fail("expected an identifier")
	}
	p.pos++
	return siLeaf(t, "identifier", field, true)
}

func siNew(typ, field string, kids ...*siNode) *siNode {
	a, b := kids[0], kids[len(kids)-1]
	return &siNode{typ: typ, field: field, flags: LargeRowNamed, start: a.start, end: b.end, sr: a.sr, sc: a.sc, er: b.er, ec: b.ec, kids: kids}
}

func siField(n *siNode, field string) *siNode {
	n.field = field
	return n
}

// name is _name: identifier, or qualified_name as a left-associative chain.
func (p *siParser) name(field string) *siNode {
	n := p.ident("")
	for p.isPunct(".", 0) && p.peek(1) != nil && p.peek(1).kind == 'w' {
		dot := p.punct(".")
		n = siNew("qualified_name", "", siField(n, "qualifier"), dot, p.ident("name"))
	}
	n.field = field
	return n
}

func (p *siParser) typeNode(field string) *siNode {
	if t := p.peek(0); t != nil && t.kind == 'w' && siPredefined[t.text] {
		p.pos++
		return siLeaf(t, "predefined_type", field, true)
	}
	return p.name(field)
}

func (p *siParser) stringLiteral() *siNode {
	kids := []*siNode{siLeaf(p.take('q', "a string"), `"`, "", false)}
	for {
		t := p.peek(0)
		switch {
		case t != nil && t.kind == 'c':
			p.pos++
			kids = append(kids, siLeaf(t, "string_literal_content", "", true))
		case t != nil && t.kind == 'e':
			p.pos++
			kids = append(kids, siLeaf(t, "escape_sequence", "", true))
		default:
			kids = append(kids, siLeaf(p.take('q', "a closing quote"), `"`, "", false))
			return siNew("string_literal", "", kids...)
		}
	}
}

func (p *siParser) primary() *siNode {
	t := p.peek(0)
	switch {
	case t != nil && t.kind == 'q':
		return p.stringLiteral()
	case t != nil && t.kind == 'n':
		p.pos++
		return siLeaf(t, "integer_literal", "", true)
	case t != nil && t.kind == 'w' && !siReserved[t.text]:
		n := p.ident("")
		for p.isPunct(".", 0) && p.peek(1) != nil && p.peek(1).kind == 'w' {
			dot := p.punct(".")
			n = siNew("member_access_expression", "", siField(n, "expression"), dot, p.ident("name"))
		}
		return n
	}
	p.fail("expected an expression")
	return nil
}

func (p *siParser) expr() *siNode {
	left := p.primary()
	for p.isPunct("+", 0) {
		op := siField(p.punct("+"), "operator")
		right := p.primary()
		left = siNew("binary_expression", "", siField(left, "left"), op, siField(right, "right"))
	}
	return left
}

func (p *siParser) attributeLists() []*siNode {
	var out []*siNode
	for p.isPunct("[", 0) {
		kids := []*siNode{p.punct("[")}
		for {
			attr := []*siNode{p.name("name")}
			if p.isPunct("(", 0) {
				args := []*siNode{p.punct("(")}
				for !p.isPunct(")", 0) {
					if len(args) > 1 {
						args = append(args, p.punct(","))
					}
					if t := p.peek(0); t != nil && t.kind == 'w' && (p.isPunct("=", 1) && !p.isPunct("=", 2) || p.isPunct(":", 1)) {
						name := p.ident("name")
						sep := p.punct(p.peek(0).text)
						args = append(args, siNew("attribute_argument", "", name, sep, p.expr()))
					} else {
						args = append(args, siNew("attribute_argument", "", p.expr()))
					}
				}
				args = append(args, p.punct(")"))
				attr = append(attr, siNew("attribute_argument_list", "", args...))
			}
			kids = append(kids, siNew("attribute", "", attr...))
			if !p.isPunct(",", 0) {
				break
			}
			kids = append(kids, p.punct(","))
		}
		kids = append(kids, p.punct("]"))
		out = append(out, siNew("attribute_list", "", kids...))
	}
	return out
}

func (p *siParser) modifiers() []*siNode {
	var out []*siNode
	for t := p.peek(0); t != nil && t.kind == 'w' && siModifiers[t.text]; t = p.peek(0) {
		out = append(out, siNew("modifier", "", p.kw(t.text)))
	}
	return out
}

func (p *siParser) declList(field string) *siNode {
	kids := []*siNode{p.punct("{")}
	for !p.isPunct("}", 0) {
		if p.peek(0) == nil {
			p.fail("expected \"}\"")
		}
		kids = append(kids, p.member())
	}
	return siNew("declaration_list", field, append(kids, p.punct("}"))...)
}

func (p *siParser) usingDirective() *siNode {
	return siNew("using_directive", "", p.kw("using"), p.name(""), p.punct(";"))
}

func (p *siParser) namespaceDecl() *siNode {
	kw := p.kw("namespace")
	name := p.name("name")
	return siNew("namespace_declaration", "", kw, name, p.declList("body"))
}

// typeRest parses class and interface declarations after their attribute lists and modifiers.
func (p *siParser) typeRest(lead []*siNode) *siNode {
	kind := ""
	switch {
	case p.isWord("class", 0):
		kind = "class"
	case p.isWord("interface", 0):
		kind = "interface"
	default:
		p.fail("expected a type declaration")
	}
	kids := append(lead, p.kw(kind), p.ident("name"))
	if p.isPunct(":", 0) {
		base := []*siNode{p.punct(":"), p.typeNode("")}
		for p.isPunct(",", 0) {
			base = append(base, p.punct(","), p.typeNode(""))
		}
		kids = append(kids, siNew("base_list", "", base...))
	}
	return siNew(kind+"_declaration", "", append(kids, p.declList("body"))...)
}

func (p *siParser) parameters() *siNode {
	kids := []*siNode{p.punct("(")}
	for !p.isPunct(")", 0) {
		if len(kids) > 1 {
			kids = append(kids, p.punct(","))
		}
		typ := p.typeNode("type")
		kids = append(kids, siNew("parameter", "", typ, p.ident("name")))
	}
	return siNew("parameter_list", "parameters", append(kids, p.punct(")"))...)
}

func (p *siParser) block() *siNode {
	kids := []*siNode{p.punct("{")}
	for !p.isPunct("}", 0) {
		if p.peek(0) == nil {
			p.fail("expected \"}\"")
		}
		// return_statement is the only statement of the subset
		ret := []*siNode{p.kw("return")}
		if !p.isPunct(";", 0) {
			ret = append(ret, p.expr())
		}
		kids = append(kids, siNew("return_statement", "", append(ret, p.punct(";"))...))
	}
	return siNew("block", "body", append(kids, p.punct("}"))...)
}

// member is one `declaration` of a declaration_list: a type, a method or a field.
func (p *siParser) member() *siNode {
	if p.isWord("using", 0) {
		return p.usingDirective()
	}
	if p.isWord("namespace", 0) {
		return p.namespaceDecl()
	}
	lead := append(p.attributeLists(), p.modifiers()...)
	if p.isWord("class", 0) || p.isWord("interface", 0) {
		return p.typeRest(lead)
	}
	typ := p.typeNode("")
	name := p.ident("name")
	if p.isPunct("(", 0) {
		kids := append(lead, siField(typ, "returns"), name, p.parameters())
		if p.isPunct(";", 0) {
			kids = append(kids, p.punct(";"))
		} else {
			kids = append(kids, p.block())
		}
		return siNew("method_declaration", "", kids...)
	}
	decl := []*siNode{name}
	if p.isPunct("=", 0) {
		decl = append(decl, p.punct("="), p.expr())
	}
	vd := siNew("variable_declaration", "", siField(typ, "type"), siNew("variable_declarator", "", decl...))
	return siNew("field_declaration", "", append(lead, vd, p.punct(";"))...)
}

func (p *siParser) compilationUnit() []*siNode {
	var out []*siNode
	for p.peek(0) != nil {
		switch {
		case p.isWord("using", 0):
			out = append(out, p.usingDirective())
		case p.isWord("namespace", 0):
			out = append(out, p.namespaceDecl())
		default:
			out = append(out, p.typeRest(append(p.attributeLists(), p.modifiers()...)))
		}
	}
	return out
}

var siDirectiveLine = regexp.MustCompile(`(?m)^[ \t]*#`)

// SvcInlineFragment is the source of one inline derivation: the bytes of the included range (or of
// the whole file of a plain C# case) and where they start and end in the original coordinates.
type SvcInlineFragment struct {
	StartByte  uint32    `json:"start_byte"`
	EndByte    uint32    `json:"end_byte"`
	StartPoint [2]uint32 `json:"start_point"`
	EndPoint   [2]uint32 `json:"end_point"`
	Bytes      uint64    `json:"bytes"`
	SHA256     string    `json:"sha256"`
}

// siTree is a derived inline CST: the root node and its rows in preorder.
type siTree struct {
	root *siNode
	rows []LargeRow
}

// siDerive derives the CST of one fragment. The returned error is an *siError of a refused fragment.
func siDerive(text []byte, f SvcInlineFragment) (tree siTree, err error) {
	if siDirectiveLine.Match(text) {
		return tree, &siError{siPreproc, "a preprocessor directive line: the external scanner and the parse state decide its tokens and ranges"}
	}
	toks, err := siTokenize(text, f.StartByte, f.StartPoint[0], f.StartPoint[1])
	if err != nil {
		return tree, err
	}
	var code []siTok
	var comments []*siNode
	for i := range toks {
		if toks[i].kind == 'm' {
			c := siLeaf(&toks[i], "comment", "", true)
			c.flags = siExtraNamed
			comments = append(comments, c)
		} else {
			code = append(code, toks[i])
		}
	}
	if len(code) == 0 {
		return tree, &siError{siUnsupported, "fragment without a token"}
	}
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*siError)
			if !ok {
				panic(r)
			}
			err = e
		}
	}()
	p := &siParser{toks: code}
	kids := p.compilationUnit()
	root := &siNode{typ: "compilation_unit", flags: LargeRowNamed, end: f.EndByte, er: f.EndPoint[0], ec: f.EndPoint[1], kids: kids}
	first := kids[0]
	for _, c := range comments {
		if c.start < first.start {
			first = c
		}
	}
	root.start, root.sr, root.sc = first.start, first.sr, first.sc
	// an extra belongs to the innermost node whose span contains it; outside every child it is a root child
	for _, c := range comments {
		n := root
	descend:
		for {
			for _, k := range n.kids {
				if k.start <= c.start && c.end <= k.end && k.flags&LargeRowExtra == 0 && len(k.kids) > 0 {
					n = k
					continue descend
				}
			}
			break
		}
		at := slices.IndexFunc(n.kids, func(k *siNode) bool { return k.start > c.start })
		if at < 0 {
			at = len(n.kids)
		}
		n.kids = slices.Insert(n.kids, at, c)
	}
	tree.root = root
	var walk func(n *siNode, parent int32, depth uint32)
	walk = func(n *siNode, parent int32, depth uint32) {
		n.index = len(tree.rows)
		tree.rows = append(tree.rows, LargeRow{Index: uint32(n.index), Parent: parent, StartByte: n.start, EndByte: n.end, StartRow: n.sr, StartCol: n.sc,
			EndRow: n.er, EndCol: n.ec, Flags: n.flags, Type: n.typ, Field: n.field, Depth: depth})
		for _, k := range n.kids {
			walk(k, int32(n.index), depth+1)
		}
	}
	walk(root, -1, 1)
	return tree, nil
}

// SvcInlineCST is the independent CST expectation of one inline step: the node count and the SHA-256
// of the canonical rows (LargeRow.Append) in preorder.
type SvcInlineCST struct {
	Nodes      uint64 `json:"nodes"`
	MaxDepth   uint32 `json:"max_depth"`
	RowsSHA256 string `json:"rows_sha256"`
	HasError   bool   `json:"has_error"`
}

// siRowsDigest digests rows; every encoded row is hashed in preorder.
func siRowsDigest(rows []LargeRow) SvcInlineCST {
	h := sha256.New()
	var buf []byte
	out := SvcInlineCST{Nodes: uint64(len(rows))}
	for _, r := range rows {
		buf = r.Append(buf[:0])
		h.Write(buf)
		out.MaxDepth = max(out.MaxDepth, r.Depth)
		out.HasError = out.HasError || r.Flags&(LargeRowError|LargeRowMissing|LargeRowHasErr) != 0
	}
	out.RowsSHA256 = hex.EncodeToString(h.Sum(nil))
	return out
}

// siRowsOfNodes converts the public CST of a native tree to canonical rows. It reports false, without rows, when a
// parent is not -1 for the root or an earlier index for any other node: such a value must not be narrowed to int32
// (a Parent of 2^32-1 would become the root's -1 and an out-of-range parent would name a different node).
func siRowsOfNodes(nodes []TreeNode) ([]LargeRow, bool) {
	rows := make([]LargeRow, len(nodes))
	for i, n := range nodes {
		if i == 0 && n.Parent != -1 || i > 0 && (n.Parent < 0 || n.Parent >= int64(i)) {
			return nil, false
		}
		r := LargeRow{Index: uint32(i), Parent: int32(n.Parent), StartByte: n.StartByte, EndByte: n.EndByte, StartRow: n.StartPoint.Row, StartCol: n.StartPoint.Column,
			EndRow: n.EndPoint.Row, EndCol: n.EndPoint.Column, Type: n.Type, Depth: 1}
		if n.Field != nil {
			r.Field = *n.Field
		}
		for flag, on := range map[uint8]bool{LargeRowNamed: n.Named, LargeRowExtra: n.Extra, LargeRowMissing: n.IsMissing, LargeRowError: n.IsError, LargeRowHasErr: n.HasError} {
			if on {
				r.Flags |= flag
			}
		}
		if n.Parent >= 0 && int(n.Parent) < i {
			r.Depth = rows[n.Parent].Depth + 1
		}
		rows[i] = r
	}
	return rows, true
}

// siQNode is one node pattern of a query; typ "_" is the wildcard, named restricts it to named nodes.
type siQNode struct {
	typ   string
	named bool
	field string
	caps  []string
	items []siQItem
}

type siQItem struct {
	anchor bool
	node   *siQNode
}

type siQPattern struct {
	index int
	root  *siQNode
}

type siQScanner struct {
	src string
	pos int
}

// siParseQuery reads the query syntax the two fact queries use: node patterns, wildcards, fields,
// the first-child anchor and captures. Anything else is refused rather than interpreted.
func siParseQuery(src string) ([]siQPattern, error) {
	s := &siQScanner{src: src}
	var out []siQPattern
	for {
		s.space()
		if s.pos >= len(s.src) {
			return out, nil
		}
		n, err := s.node("")
		if err != nil {
			return nil, err
		}
		out = append(out, siQPattern{index: len(out), root: n})
	}
}

func (s *siQScanner) space() {
	for s.pos < len(s.src) && strings.IndexByte(" \t\r\n", s.src[s.pos]) >= 0 {
		s.pos++
	}
}

func (s *siQScanner) word() string {
	i := s.pos
	for s.pos < len(s.src) && (s.src[s.pos] == '_' || s.src[s.pos] == '.' || s.src[s.pos] >= '0' && s.src[s.pos] <= '9' || s.src[s.pos] >= 'a' && s.src[s.pos] <= 'z' || s.src[s.pos] >= 'A' && s.src[s.pos] <= 'Z') {
		s.pos++
	}
	return s.src[i:s.pos]
}

func (s *siQScanner) node(field string) (*siQNode, error) {
	n := &siQNode{field: field}
	switch {
	case s.pos < len(s.src) && s.src[s.pos] == '(':
		s.pos++
		s.space()
		n.typ = s.word()
		if n.typ == "" {
			return nil, &siError{siUnsupported, "query pattern without a node type"}
		}
		n.named = true
		for {
			s.space()
			if s.pos >= len(s.src) {
				return nil, &siError{siUnsupported, "unterminated query pattern"}
			}
			switch c := s.src[s.pos]; {
			case c == ')':
				s.pos++
				return n, s.captures(n)
			case c == '.':
				s.pos++
				n.items = append(n.items, siQItem{anchor: true})
			default:
				f := ""
				if w := s.peekWord(); w != "" && s.pos+len(w) < len(s.src) && s.src[s.pos+len(w)] == ':' {
					f = w
					s.pos += len(w) + 1
					s.space()
				}
				child, err := s.node(f)
				if err != nil {
					return nil, err
				}
				n.items = append(n.items, siQItem{node: child})
			}
		}
	case s.pos < len(s.src) && s.src[s.pos] == '_':
		s.pos++
		n.typ = "_"
		return n, s.captures(n)
	}
	return nil, &siError{siUnsupported, "query syntax outside the subset at " + strconv.Itoa(s.pos)}
}

func (s *siQScanner) peekWord() string {
	save := s.pos
	w := s.word()
	s.pos = save
	return w
}

func (s *siQScanner) captures(n *siQNode) error {
	for {
		s.space()
		if s.pos < len(s.src) && s.src[s.pos] == '@' {
			s.pos++
			w := s.word()
			if w == "" {
				return &siError{siUnsupported, "empty capture name"}
			}
			n.caps = append(n.caps, w)
			continue
		}
		return nil
	}
}

type siCap struct {
	name string
	node *siNode
}

// siMatchNode returns every way pattern pn matches node n: the capture lists of each match.
func siMatchNode(pn *siQNode, n *siNode) [][]siCap {
	if pn.typ != "_" && (n.typ != pn.typ || n.flags&LargeRowNamed == 0) || pn.typ == "_" && pn.named && n.flags&LargeRowNamed == 0 {
		return nil
	}
	own := make([]siCap, 0, len(pn.caps))
	for _, c := range pn.caps {
		own = append(own, siCap{c, n})
	}
	results := [][]siCap{own}
	var rec func(i, from int, acc []siCap, first bool, out *[][]siCap)
	rec = func(i, from int, acc []siCap, firstOnly bool, out *[][]siCap) {
		for i < len(pn.items) && pn.items[i].anchor {
			firstOnly = true
			i++
		}
		if i == len(pn.items) {
			*out = append(*out, acc)
			return
		}
		item := pn.items[i]
		for j := from; j < len(n.kids); j++ {
			k := n.kids[j]
			if firstOnly {
				// an anchored child pattern matches the first named child only
				if k.flags&LargeRowNamed == 0 {
					continue
				}
			}
			if item.node.field != "" && k.field != item.node.field {
				if firstOnly {
					return
				}
				continue
			}
			for _, sub := range siMatchNode(item.node, k) {
				rec(i+1, j+1, append(slices.Clone(acc), sub...), false, out)
			}
			if firstOnly {
				return
			}
		}
	}
	var all [][]siCap
	for _, base := range results {
		rec(0, 0, base, false, &all)
	}
	return all
}

// SvcInlineCapture is one capture of a derived query stream.
type SvcInlineCapture struct {
	Pattern            uint32
	Name, Type         string
	StartByte, EndByte uint32
	StartRow, StartCol uint32
	EndRow, EndCol     uint32
}

// Record is the canonical encoding used by the multiset digest: the pattern index, the capture
// name and node type, and the byte and point range.
func (c SvcInlineCapture) Record() []byte {
	b := binary.BigEndian.AppendUint32(nil, c.Pattern)
	b = binary.BigEndian.AppendUint16(b, uint16(len(c.Name)))
	b = append(b, c.Name...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(c.Type)))
	b = append(b, c.Type...)
	for _, v := range []uint32{c.StartByte, c.EndByte, c.StartRow, c.StartCol, c.EndRow, c.EndCol} {
		b = binary.BigEndian.AppendUint32(b, v)
	}
	return b
}

func siWalk(n *siNode, f func(*siNode)) {
	f(n)
	for _, k := range n.kids {
		siWalk(k, f)
	}
}

// siRunQuery derives the capture stream of the patterns over the tree: every pattern is tried at
// every node and every match contributes its captures.
func siRunQuery(patterns []siQPattern, root *siNode) []SvcInlineCapture {
	var out []SvcInlineCapture
	siWalk(root, func(n *siNode) {
		for _, p := range patterns {
			for _, m := range siMatchNode(p.root, n) {
				for _, c := range m {
					out = append(out, SvcInlineCapture{Pattern: uint32(p.index), Name: c.name, Type: c.node.typ, StartByte: c.node.start, EndByte: c.node.end,
						StartRow: c.node.sr, StartCol: c.node.sc, EndRow: c.node.er, EndCol: c.node.ec})
				}
			}
		}
	})
	return out
}

// SvcInlineNameCount is the number of captures of one capture name.
type SvcInlineNameCount struct {
	Name  string `json:"name"`
	Count uint64 `json:"count"`
}

// SvcInlineQuery is the independent capture expectation of one query on one inline step: the total
// number, the number per capture name and the multiset digest (LargeMultisetDigest over the
// canonical records). Facts is set for the dynamic SQL query: the number of derived facts.
type SvcInlineQuery struct {
	ID       string               `json:"id"`
	Patterns int                  `json:"patterns"`
	Captures uint64               `json:"captures"`
	ByName   []SvcInlineNameCount `json:"by_name"`
	SHA256   string               `json:"sha256"`
	Facts    *uint64              `json:"facts,omitempty"`
}

func siSummarize(id string, patterns int, caps []SvcInlineCapture) SvcInlineQuery {
	q := SvcInlineQuery{ID: id, Patterns: patterns, Captures: uint64(len(caps)), ByName: []SvcInlineNameCount{}}
	counts := map[string]uint64{}
	records := make([][]byte, 0, len(caps))
	for _, c := range caps {
		counts[c.Name]++
		records = append(records, c.Record())
	}
	for name, n := range counts {
		q.ByName = append(q.ByName, SvcInlineNameCount{name, n})
	}
	slices.SortFunc(q.ByName, func(a, b SvcInlineNameCount) int { return strings.Compare(a.Name, b.Name) })
	q.SHA256 = LargeMultisetDigest(records)
	return q
}

// siCapturesOf converts a runtime capture stream to the derived form.
func siCapturesOf(caps []Capture) []SvcInlineCapture {
	out := make([]SvcInlineCapture, len(caps))
	for i, c := range caps {
		out[i] = SvcInlineCapture{Pattern: c.Pattern, Name: c.Name, Type: c.Type, StartByte: c.StartByte, EndByte: c.EndByte, StartRow: c.StartPoint.Row, StartCol: c.StartPoint.Column,
			EndRow: c.EndPoint.Row, EndCol: c.EndPoint.Column}
	}
	return out
}
