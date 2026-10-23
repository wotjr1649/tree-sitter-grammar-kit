package kit

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// C# conditional-compilation evaluation for the symbol mode of #218. Nothing calls it yet:
// it is checked against recorded Roslyn observations (src/testdata/csharp-pp) and is wired
// to an operation in a later revision. Offsets are source byte offsets.

// Preprocess result statuses. ppOK carries directives and ranges, ppZeroActive directives
// and inactive ranges only; every other status claims no section selection.
const (
	ppOK                  = "OK"
	ppZeroActive          = "ZERO_ACTIVE"          // no source byte is left for the parser
	ppRangeLimit          = "RANGE_LIMIT"          // more active ranges than the registered cap
	ppLimit               = "LIMIT"                // a size, count or depth bound was exceeded
	ppMalformed           = "MALFORMED"            // a directive Roslyn reports as an error
	ppUnsupportedEncoding = "UNSUPPORTED_ENCODING" // the first revision reads UTF-8 only
	ppInvalidInput        = "INVALID_INPUT"        // invalid UTF-8 or an invalid caller symbol
)

// ppLimits bounds one evaluation. Depth also bounds the nesting of comments, strings and
// interpolation holes the lexer follows; Symbols bounds the caller symbols, whose bytes
// count against Bytes too.
type ppLimits struct {
	Bytes      int // source bytes
	Directives int // directive lines
	Depth      int // open conditional groups
	ExprDepth  int // nesting of ! and parentheses in one condition
	Ranges     int // active ranges handed to the parser (the registered range policy)
	Symbols    int // caller symbols
}

var ppDefaultLimits = ppLimits{Bytes: 32 << 20, Directives: 4096, Depth: 64, ExprDepth: 64, Ranges: 16, Symbols: 256}

// ppDirective is one directive line. Kind follows Roslyn's directive trivia names without
// the "DirectiveTrivia" suffix. Active is false inside a skipped section. Taken is set for
// If, Elif and Else; Value only for a condition that was evaluated.
type ppDirective struct {
	Kind   string
	Hash   int // offset of '#'
	Active bool
	Taken  *bool
	Value  *bool
}

type ppResult struct {
	Status     string
	Reason     string
	Directives []ppDirective
	Active     []Span // byte ranges the parser reads, ordered and disjoint
	Inactive   []Span // skipped text (Roslyn's disabled text), directive lines excluded
	NonLF      bool   // a lone CR, NEL, LS or PS ends a line (tree-sitter counts rows on LF)
}

// ppGroup is one open #if group.
type ppGroup struct {
	parent, taken, sawElse, current bool
}

// csharpPreprocess evaluates the conditional directives of a C# source under the given
// symbols. Directive lines that steer selection (#if, #elif, #else, #endif, #define,
// #undef) and skipped sections leave the active ranges; other active directives stay for
// the grammar.
func csharpPreprocess(src []byte, encoding string, symbols []string, lim ppLimits) ppResult {
	fail := func(status, reason string) ppResult { return ppResult{Status: status, Reason: reason} }
	if encoding != EncodingUTF8 {
		return fail(ppUnsupportedEncoding, encoding)
	}
	if len(src) > lim.Bytes {
		return fail(ppLimit, "bytes")
	}
	if !utf8.Valid(src) {
		return fail(ppInvalidInput, "utf-8")
	}
	symbolBytes := 0
	for _, s := range symbols {
		symbolBytes += len(s)
	}
	if len(symbols) > lim.Symbols || len(src)+symbolBytes > lim.Bytes {
		return fail(ppLimit, "symbols")
	}
	defined := map[string]bool{}
	for _, s := range symbols {
		name, ok := ppSymbol([]byte(s))
		if !ok || ppKeyword(s) {
			return fail(ppInvalidInput, "symbol "+s)
		}
		if ppTooLong(name) {
			return fail(ppLimit, "symbol length")
		}
		defined[name] = true
	}
	var res ppResult
	var groups []ppGroup
	regions, openRegions := []int{0}, 0 // open #region per group level; a group may not cross one
	active := func() bool { return len(groups) == 0 || groups[len(groups)-1].current }
	lex := &csLexer{depth: lim.Depth}
	sawToken := false
	start := 0
	if len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF {
		start = 3
	}
	var drop []Span // directive lines and skipped text, in order
	for line := 0; line < len(src); {
		end, next, nonLF := ppLineEnd(src, line)
		res.NonLF = res.NonLF || nonLF
		first := line
		if line == 0 {
			first = start
		}
		hash := -1
		if lex.atTop() {
			if i := ppSkipSpace(src, first, end); i < end && src[i] == '#' {
				hash = i
			}
		}
		if hash < 0 {
			if !active() {
				ppAppend(&res.Inactive, line, next)
				ppAppend(&drop, line, next)
			} else if ok := lex.scan(src[first:end], &sawToken); !ok {
				if lex.deep {
					return fail(ppLimit, "lexer depth")
				}
				return fail(ppMalformed, "# in code")
			}
			lex.endLine()
			line = next
			continue
		}
		if len(res.Directives) == lim.Directives {
			return fail(ppLimit, "directives")
		}
		d, args := ppDirectiveName(src, hash+1, end)
		d.Hash, d.Active = hash, active()
		if d.Kind == "" {
			return fail(ppMalformed, "unknown directive")
		}
		steering := true
		switch d.Kind {
		case "If", "Elif":
			if d.Kind == "If" && len(groups) == lim.Depth {
				return fail(ppLimit, "depth")
			}
			expr, ok := ppTail(src, args, end, false)
			if !ok {
				return fail(ppMalformed, "condition tail")
			}
			p := &ppParser{src: expr, defined: defined, depth: lim.ExprDepth}
			value, ok, deep := p.parse()
			if deep {
				return fail(ppLimit, "expression depth")
			}
			if p.long {
				return fail(ppLimit, "symbol length")
			}
			if d.Kind == "Elif" && regions[len(regions)-1] > 0 {
				return fail(ppMalformed, "region crossed")
			}
			if !ok {
				return fail(ppMalformed, "condition")
			}
			if d.Kind == "If" {
				g := ppGroup{parent: active()}
				if g.parent {
					d.Value = &value
				}
				g.current = g.parent && value
				g.taken = g.current
				groups, regions = append(groups, g), append(regions, 0)
				d.Taken = ppBool(g.current)
				break
			}
			if len(groups) == 0 || groups[len(groups)-1].sawElse {
				return fail(ppMalformed, "elif")
			}
			g := &groups[len(groups)-1]
			d.Active = g.parent
			if g.parent && !g.taken {
				d.Value = &value
			}
			g.current = g.parent && !g.taken && value
			g.taken = g.taken || g.current
			d.Taken = ppBool(g.current)
		case "Else", "EndIf":
			if _, ok := ppTail(src, args, end, true); !ok {
				return fail(ppMalformed, "tail")
			}
			if len(groups) == 0 {
				return fail(ppMalformed, "unmatched "+d.Kind)
			}
			if regions[len(regions)-1] > 0 {
				return fail(ppMalformed, "region crossed")
			}
			g := &groups[len(groups)-1]
			d.Active = g.parent
			if d.Kind == "EndIf" {
				groups, regions = groups[:len(groups)-1], regions[:len(regions)-1]
				break
			}
			if g.sawElse {
				return fail(ppMalformed, "else after else")
			}
			g.sawElse, g.current = true, g.parent && !g.taken
			g.taken = g.taken || g.current
			d.Taken = ppBool(g.current)
		case "Define", "Undef":
			name, ok := ppTail(src, args, end, false)
			sym, valid := ppSymbol(ppTrim(name))
			if !ok || !valid || ppKeyword(string(ppTrim(name))) {
				return fail(ppMalformed, "symbol")
			}
			if ppTooLong(sym) {
				return fail(ppLimit, "symbol length")
			}
			if d.Active { // Roslyn 5.9 reports no position error in a skipped section (observed)
				if sawToken {
					return fail(ppMalformed, "define after token")
				}
				defined[sym] = d.Kind == "Define"
			}
		case "Region", "EndRegion":
			lvl := &regions[len(regions)-1]
			switch {
			case d.Kind == "Region" && openRegions == lim.Depth:
				return fail(ppLimit, "depth")
			case d.Kind == "Region":
				*lvl, openRegions = *lvl+1, openRegions+1
			case *lvl == 0:
				return fail(ppMalformed, "unmatched EndRegion")
			default:
				*lvl, openRegions = *lvl-1, openRegions-1
			}
			steering = !d.Active // an active region stays for the grammar
		default: // pragma, nullable, line, error, warning: text for the grammar
			steering = !d.Active
		}
		res.Directives = append(res.Directives, d)
		if steering {
			ppAppend(&drop, line, next)
		}
		line = next
	}
	if len(groups) > 0 || openRegions > 0 {
		return fail(ppMalformed, "unterminated #if or #region")
	}
	if !lex.atTop() {
		lex.endLine()
	}
	at := uint32(0)
	for _, s := range append(drop, Span{StartByte: uint32(len(src)), EndByte: uint32(len(src))}) {
		if s.StartByte > at {
			ppAppend(&res.Active, int(at), int(s.StartByte))
		}
		at = max(at, s.EndByte)
	}
	ppPoints(src, res.Active)
	ppPoints(src, res.Inactive)
	switch {
	case len(res.Active) == 0:
		res.Status = ppZeroActive // a complete observation with nothing for the parser
	case len(res.Active) > lim.Ranges:
		return fail(ppRangeLimit, "")
	default:
		res.Status = ppOK
	}
	return res
}

func ppBool(b bool) *bool { return &b }

// ppPoints fills the points of ordered spans in one pass: rows count LF only and columns are
// bytes, as tree-sitter counts them.
func ppPoints(src []byte, spans []Span) {
	var at int
	var p Point
	advance := func(to uint32) Point {
		for ; at < int(to); at++ {
			if src[at] == '\n' {
				p = Point{Row: p.Row + 1}
			} else {
				p.Column++
			}
		}
		return p
	}
	for i := range spans {
		spans[i].StartPoint = advance(spans[i].StartByte)
		spans[i].EndPoint = advance(spans[i].EndByte)
	}
}

// ppAppend adds [start, end) to ordered spans, merging a span that touches the last.
func ppAppend(spans *[]Span, start, end int) {
	if n := len(*spans); n > 0 && (*spans)[n-1].EndByte == uint32(start) {
		(*spans)[n-1].EndByte = uint32(end)
		return
	}
	*spans = append(*spans, Span{StartByte: uint32(start), EndByte: uint32(end)})
}

// ppLineEnd returns the end of the line's content, the start of the next line and whether
// the terminator is one tree-sitter does not count as a row (lone CR, NEL, LS, PS).
func ppLineEnd(src []byte, i int) (end, next int, nonLF bool) {
	for j := i; j < len(src); j++ {
		switch {
		case src[j] == '\n':
			return j, j + 1, false
		case src[j] == '\r':
			if j+1 < len(src) && src[j+1] == '\n' {
				return j, j + 2, false
			}
			return j, j + 1, true
		case src[j] == 0xC2 && j+1 < len(src) && src[j+1] == 0x85:
			return j, j + 2, true
		case src[j] == 0xE2 && j+2 < len(src) && src[j+1] == 0x80 && (src[j+2] == 0xA8 || src[j+2] == 0xA9):
			return j, j + 3, true
		}
	}
	return len(src), len(src), false
}

// ppSkipSpace skips C# horizontal whitespace (Zs, tab, vertical tab, form feed).
func ppSkipSpace(src []byte, i, end int) int {
	for i < end {
		r, n := utf8.DecodeRune(src[i:end])
		if r != '\t' && r != '\v' && r != '\f' && !unicode.Is(unicode.Zs, r) {
			break
		}
		i += n
	}
	return i
}

func ppTrim(b []byte) []byte {
	i := ppSkipSpace(b, 0, len(b))
	j := len(b)
	for j > i {
		k := j - 1
		for k > i && !utf8.RuneStart(b[k]) {
			k--
		}
		if ppSkipSpace(b, k, j) != j {
			break
		}
		j = k
	}
	return b[i:j]
}

var ppNames = map[string]string{"if": "If", "elif": "Elif", "else": "Else", "endif": "EndIf", "define": "Define", "undef": "Undef",
	"region": "Region", "endregion": "EndRegion", "nullable": "Nullable", "line": "Line", "error": "Error", "warning": "Warning", "pragma": "Pragma"}

// ppDirectiveName reads the directive name after '#'; Kind is "" for an unknown directive.
func ppDirectiveName(src []byte, i, end int) (ppDirective, int) {
	i = ppSkipSpace(src, i, end)
	j := i
	for j < end && (src[j] >= 'a' && src[j] <= 'z') {
		j++
	}
	kind := ppNames[string(src[i:j])]
	if r, _ := ppIdentRune(src[j:end]); j < end && ppIdentPart(r) {
		kind = "" // a longer word such as #ifdef
	}
	if kind == "Pragma" { // Roslyn reads any pragma but checksum as a (possibly bad) warning pragma
		kind = "PragmaWarning"
		if w := ppSkipSpace(src, j, end); ppHasWord(src[w:end], "checksum") {
			kind = "PragmaChecksum"
		}
	}
	return ppDirective{Kind: kind}, j
}

func ppHasWord(b []byte, w string) bool {
	if len(b) < len(w) || string(b[:len(w)]) != w {
		return false
	}
	r, n := ppIdentRune(b[len(w):])
	return n == 0 || !ppIdentPart(r)
}

// ppTail returns a conditional or define directive's argument: the rest of the line up to
// an optional // comment, which must be empty when empty is true. A delimited comment stays
// in the argument, where no condition, symbol or empty tail accepts it (CS1025).
func ppTail(src []byte, i, end int, empty bool) ([]byte, bool) {
	arg := src[i:end]
	for k := 0; k+1 < len(arg); k++ {
		if arg[k] == '/' && arg[k+1] == '/' {
			arg = arg[:k]
			break
		}
	}
	if empty && len(ppTrim(arg)) != 0 {
		return nil, false
	}
	return arg, true
}

func ppKeyword(s string) bool { return s == "true" || s == "false" }

func ppIdentStart(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.Is(unicode.Nl, r) }

func ppIdentPart(r rune) bool {
	return ppIdentStart(r) || unicode.IsDigit(r) || unicode.In(r, unicode.Mn, unicode.Mc, unicode.Pc, unicode.Cf)
}

// ppSymbol decodes a conditional symbol (identifier characters and \u/\U escapes) and
// reports whether all of b is one; '@' is not part of a symbol. The returned name compares
// as C# identifiers do: escapes decoded and formatting characters (Cf) removed.
func ppSymbol(b []byte) (string, bool) {
	var out []rune
	first := true
	for i := 0; i < len(b); {
		r, n := ppIdentRune(b[i:])
		if n == 0 || (first && !ppIdentStart(r)) || (!first && !ppIdentPart(r)) {
			return "", false
		}
		if !unicode.Is(unicode.Cf, r) {
			out = append(out, r)
		}
		first, i = false, i+n
	}
	return string(out), !first
}

// ppIdentRune reads one identifier character, decoding \uXXXX and \UXXXXXXXX.
func ppIdentRune(b []byte) (rune, int) {
	if len(b) >= 2 && b[0] == '\\' && (b[1] == 'u' || b[1] == 'U') {
		n := 4
		if b[1] == 'U' {
			n = 8
		}
		if len(b) < 2+n {
			return 0, 0
		}
		var r rune
		for _, c := range b[2 : 2+n] {
			v := ppHex(c)
			if v < 0 {
				return 0, 0
			}
			r = r*16 + rune(v)
		}
		return r, 2 + n
	}
	r, n := utf8.DecodeRune(b)
	return r, n
}

func ppHex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// ppParser parses and evaluates one pp-expression: || over && over == / != over ! over
// primary (true, false, a symbol, a parenthesised expression).
type ppParser struct {
	src     []byte
	i       int
	defined map[string]bool
	depth   int
	deep    bool
	long    bool // a symbol longer than Roslyn compares
}

func (p *ppParser) parse() (value, ok, deep bool) {
	v, ok := p.or(0)
	p.space()
	return v, ok && p.i == len(p.src) && !p.deep, p.deep
}

func (p *ppParser) space() { p.i = ppSkipSpace(p.src, p.i, len(p.src)) }

func (p *ppParser) op(s string) bool {
	p.space()
	if len(p.src)-p.i >= len(s) && string(p.src[p.i:p.i+len(s)]) == s {
		p.i += len(s)
		return true
	}
	return false
}

func (p *ppParser) or(d int) (bool, bool) {
	v, ok := p.and(d)
	for ok && p.op("||") {
		var r bool
		r, ok = p.and(d)
		v = v || r
	}
	return v, ok
}

func (p *ppParser) and(d int) (bool, bool) {
	v, ok := p.equality(d)
	for ok && p.op("&&") {
		var r bool
		r, ok = p.equality(d)
		v = v && r
	}
	return v, ok
}

func (p *ppParser) equality(d int) (bool, bool) {
	v, ok := p.unary(d)
	for ok {
		switch {
		case p.op("=="):
			var r bool
			r, ok = p.unary(d)
			v = v == r
		case p.op("!="):
			var r bool
			r, ok = p.unary(d)
			v = v != r
		default:
			return v, ok
		}
	}
	return v, ok
}

func (p *ppParser) unary(d int) (bool, bool) {
	if d >= p.depth {
		p.deep = true
		return false, false
	}
	p.space()
	if p.i < len(p.src) && p.src[p.i] == '!' && (p.i+1 == len(p.src) || p.src[p.i+1] != '=') {
		p.i++
		v, ok := p.unary(d + 1)
		return !v, ok
	}
	if p.op("(") {
		v, ok := p.or(d + 1)
		return v, ok && p.op(")")
	}
	j := p.i
	for j < len(p.src) {
		r, n := ppIdentRune(p.src[j:])
		if n == 0 || !(ppIdentPart(r) || r == '\\') {
			break
		}
		j += n
	}
	word := p.src[p.i:j]
	p.i = j
	switch string(word) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	name, ok := ppSymbol(word)
	switch {
	case ok && strings.EqualFold(name, "true"): // Roslyn: bool.TryParse on the decoded name
		return true, true
	case ok && strings.EqualFold(name, "false"):
		return false, true
	}
	p.long = p.long || ppTooLong(name)
	return p.defined[name], ok
}

// ppTooLong reports a symbol longer than the 128 UTF-16 code units Roslyn compares; Roslyn
// truncates such names, and the evaluator refuses them instead (LIMIT).
func ppTooLong(name string) bool {
	n := 0
	for _, r := range name {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n > 128
}

// csLexer follows active C# code across lines only far enough to know whether a line starts
// inside a multi-line token (delimited comment, verbatim or raw string, an interpolation
// hole of one) and whether a non-trivia token has occurred.
type csLexer struct {
	stack []csFrame
	depth int  // the most open constructs followed
	deep  bool // scan stopped at depth
}

// csFrame is one open construct. kind: 'c' delimited comment, 'v' verbatim string, 'r' raw
// string (quotes), 'h' interpolation hole, 'i' regular interpolated string, 'V' interpolated
// verbatim, 'R' interpolated raw (dollars). A hole counts its own braces and parentheses;
// a ':' at its top level starts the format clause, which is text up to the closing '}'.
type csFrame struct {
	kind            byte
	quotes, dollars int
	braces, parens  int
	format          bool
}

func (l *csLexer) atTop() bool { return len(l.stack) == 0 }

// endLine closes the single-line constructs a line end terminates.
func (l *csLexer) endLine() {
	for len(l.stack) > 0 && l.stack[len(l.stack)-1].kind == 'i' {
		l.stack = l.stack[:len(l.stack)-1]
	}
}

func (l *csLexer) push(f csFrame) {
	if len(l.stack) >= l.depth {
		l.deep = true
		return
	}
	l.stack = append(l.stack, f)
}

// scan lexes one line of active code; it returns false for a '#' token in code (CS1040)
// or at the depth bound (deep).
func (l *csLexer) scan(b []byte, sawToken *bool) bool {
	for i := 0; i < len(b); {
		if l.deep {
			return false
		}
		var top *csFrame
		if len(l.stack) > 0 {
			top = &l.stack[len(l.stack)-1]
		}
		if top != nil && top.kind != 'h' {
			i = l.inside(b, i, top)
			continue
		}
		c := b[i]
		if top != nil && top.format { // format clause text up to the hole's '}'
			if c == '}' {
				l.stack = l.stack[:len(l.stack)-1]
			}
			i++
			continue
		}
		switch {
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			return true
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			l.push(csFrame{kind: 'c'})
			i += 2
		case c == '{' && top != nil:
			top.braces++
			i++
		case c == '}' && top != nil:
			if top.braces == 0 {
				l.stack = l.stack[:len(l.stack)-1] // the hole ends; back in its string
			} else {
				top.braces--
			}
			i++
		case (c == '(' || c == '[') && top != nil:
			top.parens++
			i++
		case (c == ')' || c == ']') && top != nil:
			top.parens--
			i++
		case c == ':' && top != nil && top.braces == 0 && top.parens <= 0 && !(i+1 < len(b) && b[i+1] == ':') && !(i > 0 && b[i-1] == ':'):
			top.format = true
			i++
		case c == '\'':
			*sawToken = true
			i = l.char(b, i)
		case c == '"' || c == '@' || c == '$':
			n, ok := l.string(b, i)
			if !ok {
				*sawToken = true
				i++
				continue
			}
			*sawToken = true
			i = n
		case c == '#':
			return false
		default:
			r, n := utf8.DecodeRune(b[i:])
			if r != ' ' && r != '\t' && r != '\v' && r != '\f' && !unicode.Is(unicode.Zs, r) {
				*sawToken = true
			}
			i += n
		}
	}
	return !l.deep // a construct refused at the line's last character
}

// char skips a character literal.
func (l *csLexer) char(b []byte, i int) int {
	for j := i + 1; j < len(b); j++ {
		if b[j] == '\\' {
			j++
			continue
		}
		if b[j] == '\'' {
			return j + 1
		}
	}
	return len(b)
}

// string opens the string starting at b[i] (with its @ and $ prefixes) and returns the
// position after the opening; ok is false when b[i] does not start a string.
func (l *csLexer) string(b []byte, i int) (int, bool) {
	j, dollars, verbatim := i, 0, false
	for j < len(b) && (b[j] == '$' || b[j] == '@') {
		if b[j] == '$' {
			dollars++
		} else {
			verbatim = true
		}
		j++
	}
	if j >= len(b) || b[j] != '"' {
		return i, false
	}
	q := 0
	for j+q < len(b) && b[j+q] == '"' {
		q++
	}
	switch {
	case q >= 3 && !verbatim:
		f := csFrame{kind: 'r', quotes: q}
		if dollars > 0 {
			f.kind, f.dollars = 'R', dollars
		}
		l.push(f)
		return j + q, true
	case q == 2 && !verbatim:
		return j + 2, true // an empty regular string
	case verbatim && dollars > 0:
		l.push(csFrame{kind: 'V', dollars: 1})
	case verbatim:
		l.push(csFrame{kind: 'v'})
	case dollars > 0:
		l.push(csFrame{kind: 'i', dollars: 1})
	default:
		return l.regular(b, j+1), true
	}
	return j + 1, true
}

// regular skips a regular string's content.
func (l *csLexer) regular(b []byte, i int) int {
	for ; i < len(b); i++ {
		if b[i] == '\\' {
			i++
			continue
		}
		if b[i] == '"' {
			return i + 1
		}
	}
	return len(b)
}

// inside advances through the content of the open construct f.
func (l *csLexer) inside(b []byte, i int, f *csFrame) int {
	pop := func() { l.stack = l.stack[:len(l.stack)-1] }
	switch f.kind {
	case 'c':
		for ; i+1 < len(b); i++ {
			if b[i] == '*' && b[i+1] == '/' {
				pop()
				return i + 2
			}
		}
		return len(b)
	case 'v', 'V':
		for ; i < len(b); i++ {
			switch {
			case b[i] == '"' && i+1 < len(b) && b[i+1] == '"':
				i++
			case b[i] == '"':
				pop()
				return i + 1
			case f.kind == 'V' && b[i] == '{' && i+1 < len(b) && b[i+1] == '{':
				i++
			case f.kind == 'V' && b[i] == '{':
				l.push(csFrame{kind: 'h'})
				return i + 1
			}
		}
		return len(b)
	case 'i':
		for ; i < len(b); i++ {
			switch {
			case b[i] == '\\':
				i++
			case b[i] == '"':
				pop()
				return i + 1
			case b[i] == '{' && i+1 < len(b) && b[i+1] == '{':
				i++
			case b[i] == '{':
				l.push(csFrame{kind: 'h'})
				return i + 1
			}
		}
		return len(b)
	default: // 'r', 'R'
		for ; i < len(b); i++ {
			if b[i] == '"' {
				n := 0
				for i+n < len(b) && b[i+n] == '"' {
					n++
				}
				if n >= f.quotes {
					pop()
					return i + n
				}
				i += n - 1
				continue
			}
			if f.kind == 'R' && b[i] == '{' {
				n := 0
				for i+n < len(b) && b[i+n] == '{' {
					n++
				}
				if n >= f.dollars {
					l.push(csFrame{kind: 'h'})
					return i + n
				}
				i += n - 1
			}
		}
		return len(b)
	}
}
