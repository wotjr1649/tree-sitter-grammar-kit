package kit

import (
	"maps"
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
// If, Elif and Else.
type ppDirective struct {
	Kind   string
	Hash   int // offset of '#'
	Active bool
	Taken  *bool
}

type ppResult struct {
	Status     string
	Reason     string
	Directives []ppDirective
	Active     []Span // byte ranges the parser reads, ordered and disjoint
	Inactive   []Span // skipped text (Roslyn's disabled text), directive lines excluded
	NonLF      bool   // a lone CR, NEL, LS or PS ends a line (tree-sitter counts rows on LF)
}

// ppGroup is one open #if group. branch holds the #define and #undef of its current branch
// while that branch is skipped and the group is evaluated: Roslyn's directive stack lets the
// next #elif of the group see them (the branches before are skipped, and the group's #endif
// keeps only the taken branch's, which were active anyway), observed.
type ppGroup struct {
	parent, taken, sawElse, current bool
	branch                          map[string]bool
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
		if _, ok := ppSymbol([]byte(s)); !ok || ppKeyword(s) {
			return fail(ppInvalidInput, "symbol "+s)
		}
		if ppTooLong(s) {
			return fail(ppLimit, "symbol length")
		}
		defined[s] = true // Roslyn keeps caller symbols as given; only source names are folded
	}
	var res ppResult
	var groups []ppGroup
	regions, openRegions := []int{0}, 0 // open #region per group level; a group may not cross one
	active := func() bool { return len(groups) == 0 || groups[len(groups)-1].current }
	lex := &csLexer{depth: lim.Depth}
	sawToken, sawIf := false, false
	start := 0
	if len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF {
		start = 3
	}
	var drop []Span                // directive lines and skipped text, in order
	rawQuotes, rawDrop := 0, false // a raw string literal a directive opened, and whether its lines leave
	for line := 0; line < len(src); {
		end, next, nonLF := ppLineEnd(src, line)
		res.NonLF = res.NonLF || nonLF
		if rawQuotes > 0 { // the line belongs to the directive: neither code, directive nor skipped text
			if k := ppQuotesEnd(src, line, end, rawQuotes); k >= 0 {
				rawQuotes = ppTailRaw(src, k, end) // the directive's tokens go on after the literal
			}
			if rawDrop {
				ppAppend(&drop, line, next)
			}
			line = next
			continue
		}
		first := line
		if line == 0 {
			first = start
		}
		hash := -1
		if lex.atTop() {
			if i := ppSkipLead(src, first, end); i < end && src[i] == '#' {
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
			lex.endLine(end+1 < next && src[end] == 13 && src[end+1] == 10) // CRLF
			line = next
			continue
		}
		if len(res.Directives) == lim.Directives {
			return fail(ppLimit, "directives")
		}
		d, args := ppDirectiveName(src, hash+1, end)
		d.Hash, d.Active = hash, active()
		// C# 14 file-based programs (Roslyn's FileBasedProgram feature, which the registered
		// grammar accepts): #! only at offset 0, #: only before the first token; both stay
		// for the grammar
		switch {
		case hash+1 < end && src[hash+1] == '!' && !(hash+2 < end && src[hash+2] == '='): // #!= is no shebang (observed)
			if hash != 0 {
				return fail(ppMalformed, "#! not at the start")
			}
			d.Kind = "Shebang"
		case hash+1 < end && src[hash+1] == ':':
			if d.Active && (sawToken || sawIf) { // CS9297, CS9299 (after any #if, even closed)
				return fail(ppMalformed, "#: after token or #if")
			}
			d.Kind = "Ignored"
		}
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
			defs := defined
			if n := len(groups); d.Kind == "Elif" && n > 0 && groups[n-1].branch != nil {
				defs = maps.Clone(defined)
				maps.Copy(defs, groups[n-1].branch)
			}
			p := &ppParser{src: expr, defined: defs, depth: lim.ExprDepth}
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
				sawIf = true
				g := ppGroup{parent: active()}
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
			g.current = g.parent && !g.taken && value
			g.taken = g.taken || g.current
			g.branch = nil
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
			g.branch = nil
			d.Taken = ppBool(g.current)
		case "Define", "Undef":
			name, ok := ppTail(src, args, end, false)
			sym, valid := ppSymbol(ppTrim(name))
			if !ok || !valid || ppKeyword(string(ppTrim(name))) {
				return fail(ppMalformed, "symbol")
			}
			if ppTooLong(string(ppTrim(name))) {
				return fail(ppLimit, "symbol length")
			}
			if d.Active { // Roslyn 5.9 reports no position error in a skipped section (observed)
				if sawToken {
					return fail(ppMalformed, "define after token")
				}
				defined[sym] = d.Kind == "Define"
			} else if n := len(groups); groups[n-1].parent { // directly in a skipped branch of an evaluated group
				if groups[n-1].branch == nil {
					groups[n-1].branch = map[string]bool{}
				}
				groups[n-1].branch[sym] = d.Kind == "Define"
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
		switch d.Kind {
		case "Nullable", "Line", "PragmaWarning", "PragmaChecksum", "Reference", "Load":
			rawQuotes, rawDrop = ppTailRaw(src, args, end), steering
		}
		line = next
	}
	if len(groups) > 0 || openRegions > 0 {
		return fail(ppMalformed, "unterminated #if or #region")
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

// ppSkipSpace skips C# horizontal whitespace (ppSpace).
func ppSkipSpace(src []byte, i, end int) int {
	for i < end {
		r, n := utf8.DecodeRune(src[i:end])
		if !ppSpace(r) {
			break
		}
		i += n
	}
	return i
}

// ppSpace is C# whitespace (Roslyn's SyntaxFacts.IsWhitespace: Zs, tab, vertical tab, form
// feed, U+001A and U+FEFF), in code and between the tokens of a directive alike. U+FEFF is
// also a formatting character, so after an identifier's first character it continues the
// identifier instead (#if<U+FEFF>A is the unknown directive ifA, observed).
func ppSpace(r rune) bool {
	return r == '\t' || r == '\v' || r == '\f' || r == 0x1A || r == 0xFEFF || unicode.Is(unicode.Zs, r)
}

// ppSkipLead skips the whitespace before a directive's '#'.
func ppSkipLead(src []byte, i, end int) int {
	for i < end {
		r, n := utf8.DecodeRune(src[i:end])
		if !ppSpace(r) {
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
		r, n := utf8.DecodeLastRune(b[i:j])
		if !ppSpace(r) {
			break
		}
		if r == 0xFEFF { // a U+FEFF run right after a name character continues the name (Cf), observed
			k := j
			for k-3 >= i && b[k-3] == 0xEF && b[k-2] == 0xBB && b[k-1] == 0xBF {
				k -= 3
			}
			if p, _ := utf8.DecodeLastRune(b[i:k]); k > i && !ppSpace(p) {
				break
			}
			n = j - k // the whole run is whitespace
		}
		j -= n
	}
	return b[i:j]
}

var ppNames = map[string]string{"if": "If", "elif": "Elif", "else": "Else", "endif": "EndIf", "define": "Define", "undef": "Undef",
	"region": "Region", "endregion": "EndRegion", "nullable": "Nullable", "line": "Line", "error": "Error", "warning": "Warning", "pragma": "Pragma",
	"r": "Reference", "load": "Load"} // #r and #load are script directives Roslyn also reads in regular code

// ppDirectiveName reads the directive name after '#'; Kind is "" for an unknown directive.
func ppDirectiveName(src []byte, i, end int) (ppDirective, int) {
	i = ppSkipSpace(src, i, end)
	j := i
	for j < end && (src[j] >= 'a' && src[j] <= 'z') {
		j++
	}
	kind := ppNames[string(src[i:j])]
	escape := j+1 < end && src[j] == '\\' && (src[j+1] == 'u' || src[j+1] == 'U')
	if r, _ := ppIdentRune(src[j:end]); j < end && (ppIdentPart(r) || escape) {
		kind = "" // a longer word such as #ifdef, or a \u or \U Roslyn reads into the name (valid or not)
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

// ppTailRaw returns the opening quote count of a multi-line raw string literal in the rest of
// a directive line, or 0. Roslyn lexes the tokens of #nullable, #line, #pragma, #r and #load,
// and such a literal (three or more quotes, then only whitespace) takes the following lines
// into the directive, active or skipped, up to a run of as many quotes anywhere on a line
// (observed). A // comment ends the tokens; strings before are skipped.
func ppTailRaw(src []byte, i, end int) int {
	for i < end {
		switch {
		case src[i] == '/' && i+1 < end && src[i+1] == '/':
			return 0
		case src[i] != '"':
			i++
		default:
			q := 0
			for i+q < end && src[i+q] == '"' {
				q++
			}
			switch {
			case q >= 3 && ppSkipLead(src, i+q, end) == end:
				return q
			case q >= 3: // a single-line raw literal, to its closing quotes or the line end
				if i = ppQuotesEnd(src, i+q, end, q); i < 0 {
					return 0
				}
			case q == 2:
				i += 2
			default: // a regular string literal
				for i++; i < end && src[i] != '"'; i++ {
					if src[i] == '\\' {
						i++
					}
				}
				i++
			}
		}
	}
	return 0
}

// ppQuotesEnd returns the position after the first run of at least q '"' in src[i:end], or -1.
func ppQuotesEnd(src []byte, i, end, q int) int {
	for i < end {
		n := 0
		for i+n < end && src[i+n] == '"' {
			n++
		}
		if n >= q {
			return i + n
		}
		i += max(n, 1)
	}
	return -1
}

// ppEscapeBrace reports whether the escape sequence at b[0] ('\') makes '{' or '}': \{ and
// \} themselves, or a \u, \U or \x code of one (Roslyn CS8087, observed).
func ppEscapeBrace(b []byte) bool {
	if len(b) < 2 {
		return false
	}
	digits := map[byte]int{'u': 4, 'U': 8, 'x': 4}[b[1]]
	if digits == 0 {
		return b[1] == '{' || b[1] == '}'
	}
	v, n := 0, 0
	for ; n < digits && 2+n < len(b) && ppHex(b[2+n]) >= 0; n++ {
		v = v*16 + ppHex(b[2+n])
	}
	return n > 0 && (v == '{' || v == '}') // fewer digits than due still make the char (CS1009), observed
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

// Roslyn classifies directive identifier characters one UTF-16 unit at a time, so a
// character outside the BMP (a surrogate pair) is never part of a symbol.
func ppIdentStart(r rune) bool {
	return r <= 0xFFFF && (r == '_' || unicode.IsLetter(r) || unicode.Is(unicode.Nl, r))
}

func ppIdentPart(r rune) bool {
	return ppIdentStart(r) || r <= 0xFFFF && (unicode.IsDigit(r) || unicode.In(r, unicode.Mn, unicode.Mc, unicode.Pc, unicode.Cf))
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
	p.long = p.long || ppTooLong(string(word)) // before the constants: a long spelling claims nothing
	switch {
	case ok && ppASCIIFold(name, "true"): // Roslyn: bool.TryParse on the decoded name
		return true, true
	case ok && ppASCIIFold(name, "false"):
		return false, true
	}
	return p.defined[name], ok
}

// ppASCIIFold compares a name with a lowercase ASCII word, ignoring ASCII case only.
func ppASCIIFold(name, word string) bool {
	if len(name) != len(word) {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != word[i] {
			return false
		}
	}
	return true
}

// ppTooLong reports a symbol whose raw spelling (escapes as written) is longer than the 128
// UTF-16 code units Roslyn compares; Roslyn truncates the spelling, the evaluator refuses
// it instead (LIMIT).
func ppTooLong(raw string) bool {
	n := 0
	for _, r := range raw {
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

// csFrame is one open construct. kind: 'c' delimited comment, 'z' Razor comment, 's' regular string
// an escape carried over a line break, 'v' verbatim string, 'r' raw
// string (quotes), 'h' interpolation hole, 'i' regular interpolated string, 'V' interpolated
// verbatim, 'R' interpolated raw (dollars). A raw string is single-line when text follows
// its opening quotes on their line. A hole keeps the closers its open brackets expect; a ':'
// outside them, even the first of '::', starts the format clause, which is text up to the
// closing '}' or a '"'. As in Roslyn's scan of an interpolated string, a closer in a hole that
// matches nothing, a '#' in a hole, a format clause ended by '"' and too few quotes after $$
// are errors of that string (err; a nested interpolated string has its own, and a nested
// regular string left open at the line end is not one), and after an error the next '"' in a
// hole closes the string.
type csFrame struct {
	kind            byte
	quotes, dollars int
	single          bool
	closers         []byte
	format, err     bool
	escEnd          bool // a regular interpolated string's line ended in '\': the escape takes the line break
}

// holeErr records an error of the string whose hole is open.
func (l *csLexer) holeErr() { l.stack[len(l.stack)-2].err = true }

func (l *csLexer) atTop() bool { return len(l.stack) == 0 }

// endLine closes the single-line constructs a line end terminates. An escape '\' at the end
// of a regular interpolated string's line takes the line break, one character (the CR of a
// CRLF), and the string goes on (CS1009, observed).
func (l *csLexer) endLine(crlf bool) {
	for len(l.stack) > 0 {
		f := &l.stack[len(l.stack)-1]
		if f.kind != 'i' && f.kind != 's' && !f.single {
			return
		}
		if f.escEnd && !crlf {
			f.escEnd = false
			return
		}
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

// scan lexes one line of active code; it returns false for a '#' token in code outside an
// interpolation hole (CS1040) or at the depth bound (deep).
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
		if top != nil && top.format { // format clause text up to the hole's '}' or a '"'
			switch {
			case c == '}':
				l.stack = l.stack[:len(l.stack)-1]
			case c == '"' && l.stack[len(l.stack)-2].kind == 'V' && i+1 < len(b) && b[i+1] == '"':
				i++ // an escaped quote of a verbatim string
			case c == '\\' && l.stack[len(l.stack)-2].kind == 'i':
				i++ // an escape sequence of a regular string (\" stays in the clause)
			case c == '"':
				l.holeErr() // the hole is left open; its string reads the '"'
				l.stack = l.stack[:len(l.stack)-1]
				continue
			}
			i++
			continue
		}
		switch {
		case c == '/' && i+1 < len(b) && b[i+1] == '/',
			top == nil && c == '@' && i+1 < len(b) && b[i+1] == ':': // a Razor line transition (CS1646), observed
			return true
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			l.push(csFrame{kind: 'c'})
			i += 2
		case c == '@' && i+1 < len(b) && b[i+1] == '*': // a Razor comment up to *@ (CS1056, observed)
			l.push(csFrame{kind: 'z'})
			i += 2
		case top != nil && (c == '{' || c == '(' || c == '['):
			top.closers = append(top.closers, map[byte]byte{'{': '}', '(': ')', '[': ']'}[c])
			i++
		case top != nil && (c == '}' || c == ')' || c == ']'):
			switch n := len(top.closers); {
			case n > 0 && top.closers[n-1] == c:
				top.closers = top.closers[:n-1]
			case n == 0 && c == '}':
				l.stack = l.stack[:len(l.stack)-1] // the hole ends; back in its string
			default:
				l.holeErr()
			}
			i++
		case top != nil && c == '"' && l.stack[len(l.stack)-2].err:
			l.stack = l.stack[:len(l.stack)-1] // runaway recovery: the hole ends; its string reads the '"'
		case c == ':' && top != nil && len(top.closers) == 0:
			top.format = true
			i++
		case c == '\'':
			*sawToken = true
			i = l.char(b, i)
		case c == '"' || c == '@' || c == '$':
			n, _ := l.string(b, i) // a $/@ run without a quote is consumed whole, never rescanned
			*sawToken = true
			i = n
		case c == '#' && top != nil:
			l.holeErr() // Roslyn reads no directive in a hole, even at the start of a line
			i++
		case c == '#':
			return false
		default:
			r, n := utf8.DecodeRune(b[i:])
			if !ppSpace(r) {
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
// position after the opening; ok is false when b[i] does not start a string. As in Roslyn,
// an interpolated prefix is an @ run, a $ run and an @ run; a prefix not followed by '"' is a
// token of its own (CS9009 for $@$", observed) and lexing goes on after it.
func (l *csLexer) string(b []byte, i int) (int, bool) {
	j, dollars, ats := i, 0, 0
	for j < len(b) && b[j] == '@' {
		j, ats = j+1, ats+1
	}
	for j < len(b) && b[j] == '$' {
		j, dollars = j+1, dollars+1
	}
	for dollars > 0 && j < len(b) && b[j] == '@' {
		j, ats = j+1, ats+1
	}
	if j >= len(b) || b[j] != '"' {
		return j, false
	}
	verbatim := ats > 0 // @@" without $ is a verbatim string too (CS9008)
	q := 0
	for j+q < len(b) && b[j+q] == '"' {
		q++
	}
	switch {
	// $$, or $ with more than one @, starts a raw string even with fewer quotes (CS9004) or an
	// @ (CS9008); like any error of the string, these start the runaway recovery
	case q >= 3 && !verbatim, dollars > 1, dollars > 0 && ats > 1:
		f := csFrame{kind: 'r', quotes: q, single: ppSkipLead(b, j+q, len(b)) < len(b), err: q < 3 || verbatim}
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
		n, _, esc := l.regular(b, j+1)
		if esc {
			l.push(csFrame{kind: 's', escEnd: true})
		}
		return n, true
	}
	return j + 1, true
}

// regular skips a regular string's content; esc reports a '\' at the line end, whose
// escape takes the line break (CS1009, observed).
func (l *csLexer) regular(b []byte, i int) (n int, closed, esc bool) {
	for ; i < len(b); i++ {
		if b[i] == '\\' {
			if i+1 == len(b) {
				return len(b), false, true
			}
			i++
			continue
		}
		if b[i] == '"' {
			return i + 1, true, false
		}
	}
	return len(b), false, false
}

// inside advances through the content of the open construct f.
func (l *csLexer) inside(b []byte, i int, f *csFrame) int {
	pop := func() { l.stack = l.stack[:len(l.stack)-1] }
	switch f.kind {
	case 's': // a regular string an escape carried over the line break
		n, closed, esc := l.regular(b, i)
		if closed {
			pop()
		}
		f.escEnd = esc
		return n
	case 'c', 'z':
		end := map[byte]byte{'c': '/', 'z': '@'}[f.kind]
		for ; i+1 < len(b); i++ {
			if b[i] == '*' && b[i+1] == end {
				pop()
				return i + 2
			}
		}
		return len(b)
	case 'v', 'V':
		for ; i < len(b); i++ {
			switch {
			case b[i] == '"' && i+1 < len(b) && b[i+1] == '"' && !f.err: // after an error a '"' closes (observed)
				i++
			case b[i] == '"':
				pop()
				return i + 1
			case f.kind == 'V' && (b[i] == '{' || b[i] == '}') && i+1 < len(b) && b[i+1] == b[i]:
				i++
			case f.kind == 'V' && b[i] == '{':
				l.push(csFrame{kind: 'h'})
				return i + 1
			case f.kind == 'V' && b[i] == '}':
				f.err = true // a lone } (CS8086)
			}
		}
		return len(b)
	case 'i':
		for ; i < len(b); i++ {
			switch {
			case b[i] == '\\':
				f.err = f.err || ppEscapeBrace(b[i:]) // an escape that makes a brace (CS8087)
				f.escEnd = i+1 == len(b)
				i++
			case b[i] == '"':
				pop()
				return i + 1
			case b[i] == '{' && i+1 < len(b) && b[i+1] == '{', b[i] == '}' && i+1 < len(b) && b[i+1] == '}':
				i++
			case b[i] == '{':
				l.push(csFrame{kind: 'h'})
				return i + 1
			case b[i] == '}':
				f.err = true // a lone } (CS8086)
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
					f.err = f.err || n >= 2*f.dollars // too many opening braces for content (CS9006)
					l.push(csFrame{kind: 'h'})
					return i + n
				}
				i += n - 1
			}
			if f.kind == 'R' && b[i] == '}' {
				n := 0
				for i+n < len(b) && b[i+n] == '}' {
					n++
				}
				f.err = f.err || n >= f.dollars // too many closing braces for content (CS9007)
				i += n - 1
			}
		}
		return len(b)
	}
}
