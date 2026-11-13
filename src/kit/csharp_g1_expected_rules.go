package kit

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Rule table of the G1-3a expectation: how each alternative of csharp-B01 and csharp-B02 is located
// in the observation and which tree-sitter node types it allows. The B01 rules read the ECMA-334
// derivation of csharp_g1_spec.go; the B02 rules name the Roslyn node kinds that exhibit a type form.

type g1RuleSrc struct{ ID, Row, Production, Kind, Ref string }

// block codes
const (
	g1BlockMeta     = "LEXICAL_META_PRODUCTION"         // sequence structure of the input: no node and no token boundary
	g1BlockNoNode   = "NO_TREE_COUNTERPART"             // whitespace, line ends, character classes: tree-sitter has no node and Roslyn trivia is not observed
	g1BlockPosition = "POSITION_DEPENDENT"              // the node is decided by the slot it occupies, which the observation does not record
	g1BlockUnion    = "ABSTRACT_CATEGORY"               // a union of productions: no node of its own
	g1BlockSemantic = "SEMANTIC_BINDING"                // distinguishing the forms needs name binding, not syntax
	g1BlockExternal = "EXTERNAL_SCANNER_REPRESENTATION" // how tree-sitter's external scanner represents it is not in grammar.js
	g1BlockFact     = "FACT_DESCRIPTION"
)

func specRule(form string, ts []string, f g1Finder) g1Rule {
	return g1Rule{basis: G1BasisSpec, form: form, ts: ts, find: f}
}

func diffRule(form string, kinds []string, ts []string, f g1Finder) g1Rule {
	return g1Rule{basis: G1BasisDiff, form: form, kinds: kinds, ts: ts, find: f}
}

func blockRule(code, reason string) g1Rule { return g1Rule{block: code, reason: reason} }

// ---- finders ------------------------------------------------------------------------------

func fTokens(pred func(s *g1Src, t g1Tok) bool) g1Finder {
	return func(_ *g1Ctx, s *g1Src) []g1Hit {
		var out []g1Hit
		for _, t := range s.toks {
			if t.end > t.start && pred(s, t) {
				out = append(out, g1Hit{t.start, t.end, t.kind, "span"})
			}
		}
		return out
	}
}

func fText(texts ...string) g1Finder {
	return fTokens(func(_ *g1Src, t g1Tok) bool { return slices.Contains(texts, t.text) })
}

// fForm selects the tokens whose whole text matches one of the lexical forms; an identifier form
// excludes the reserved keywords (6.4.3: an identifier is not a keyword).
func fForm(c *g1Ctx, forms ...string) g1Finder {
	return fFormRe(c, nil, forms...)
}

func fFormRe(c *g1Ctx, extra *regexp.Regexp, forms ...string) g1Finder {
	return fTokens(func(_ *g1Src, t g1Tok) bool {
		for _, f := range forms {
			if !g1sForms[f].MatchString(t.text) {
				continue
			}
			if f == "ident.basic" && c.keywords[t.text] {
				continue
			}
			if extra == nil || extra.MatchString(t.text) {
				return true
			}
		}
		return false
	})
}

// fEscape selects the escape sequences (by form prefix) inside regular strings and character literals.
func fEscape(prefix string) g1Finder {
	return func(_ *g1Ctx, s *g1Src) []g1Hit {
		var out []g1Hit
		for _, t := range s.toks {
			if !g1sForms["char"].MatchString(t.text) && !g1sForms["string.regular"].MatchString(t.text) {
				continue
			}
			for _, e := range g1sEscapes(t.text) {
				if strings.HasPrefix(e.Form, prefix) {
					out = append(out, g1Hit{t.start + e.Start, t.start + e.End, t.kind, "span"})
				}
			}
		}
		return out
	}
}

func fNodes(kinds []string, pred func(s *g1Src, i int) bool) g1Finder {
	return func(_ *g1Ctx, s *g1Src) []g1Hit {
		var out []g1Hit
		for i, n := range s.nodes {
			if slices.Contains(kinds, n.kind) && (pred == nil || pred(s, i)) {
				out = append(out, g1Hit{n.start, n.end, n.kind, "span"})
			}
		}
		return out
	}
}

// nodeTok: the node has a direct token (not inside a child node) of one of the kinds.
func nodeTok(kinds ...string) func(s *g1Src, i int) bool {
	return func(s *g1Src, i int) bool {
		for _, ti := range s.nodes[i].direct {
			if slices.Contains(kinds, s.toks[ti].kind) {
				return true
			}
		}
		return false
	}
}

func nodeText(re *regexp.Regexp, want bool) func(s *g1Src, i int) bool {
	return func(s *g1Src, i int) bool { return re.MatchString(s.nodeText(s.nodes[i])) == want }
}

func nodeAnd(ps ...func(s *g1Src, i int) bool) func(s *g1Src, i int) bool {
	return func(s *g1Src, i int) bool {
		for _, p := range ps {
			if !p(s, i) {
				return false
			}
		}
		return true
	}
}

// childKinds: the direct child nodes of the node are exactly these kinds, in order.
func childKinds(kinds ...string) func(s *g1Src, i int) bool {
	return func(s *g1Src, i int) bool {
		var got []string
		for j := i + 1; j < len(s.nodes); j++ {
			p := s.nodes[j].parent
			for p > i {
				p = s.nodes[p].parent
			}
			if p != i {
				break // preorder: the subtree of i ended
			}
			if s.nodes[j].parent == i {
				got = append(got, s.nodes[j].kind)
			}
		}
		return slices.Equal(got, kinds)
	}
}

// ancestorText: the nearest ancestor of the given kind has text matching re.
func ancestorText(kind string, re *regexp.Regexp) func(s *g1Src, i int) bool {
	return func(s *g1Src, i int) bool {
		for p := s.nodes[i].parent; p >= 0; p = s.nodes[p].parent {
			if s.nodes[p].kind == kind {
				return re.MatchString(s.nodeText(s.nodes[p]))
			}
		}
		return false
	}
}

// fTokensIn selects tokens of one kind whose nearest enclosing node of kind `in` has text matching re.
func fTokensIn(tokKind, in string, re *regexp.Regexp, pred func(t g1Tok) bool) g1Finder {
	return func(_ *g1Ctx, s *g1Src) []g1Hit {
		var out []g1Hit
		for ni, n := range s.nodes {
			for _, ti := range n.direct {
				t := s.toks[ti]
				if t.kind != tokKind || pred != nil && !pred(t) {
					continue
				}
				for p := ni; p >= 0; p = s.nodes[p].parent {
					if s.nodes[p].kind == in { // the nearest enclosing node of that kind decides
						if re.MatchString(s.nodeText(s.nodes[p])) {
							out = append(out, g1Hit{t.start, t.end, t.kind, "span"})
						}
						break
					}
				}
			}
		}
		slices.SortFunc(out, func(a, b g1Hit) int { return a.start - b.start })
		return out
	}
}

// fTextEscape selects escape sequences (by form prefix) inside interpolated string text tokens of the
// nearest interpolated string whose text matches re.
func fTextEscape(prefix string, re *regexp.Regexp) g1Finder {
	return func(c *g1Ctx, s *g1Src) []g1Hit {
		var out []g1Hit
		for _, h := range fTokensIn("InterpolatedStringTextToken", "InterpolatedStringExpression", re, nil)(c, s) {
			for _, e := range g1sEscapes(s.text(h.start, h.end)) {
				if strings.HasPrefix(e.Form, prefix) {
					out = append(out, g1Hit{h.start + e.Start, h.start + e.End, h.roslyn, "span"})
				}
			}
		}
		return out
	}
}

// fDirective selects the directive lines the specification reads as the given kind (6.5.1).
func fDirective(kinds ...string) g1Finder {
	return func(_ *g1Ctx, s *g1Src) []g1Hit {
		var out []g1Hit
		for _, d := range s.dirs {
			line, end := s.directive(d)
			if slices.Contains(kinds, g1sDirectiveKind(line)) {
				out = append(out, g1Hit{d.Start, end, d.Kind, "starts-at"})
			}
		}
		return out
	}
}

// fDirectiveLine runs a per-line selector over the directive lines of one specification kind; it
// returns spans relative to the source.
func fDirectiveLine(kinds []string, sel func(line string) [][2]int, match string) g1Finder {
	return func(_ *g1Ctx, s *g1Src) []g1Hit {
		var out []g1Hit
		for _, d := range s.dirs {
			line, _ := s.directive(d)
			if !slices.Contains(kinds, g1sDirectiveKind(line)) {
				continue
			}
			base := d.LineStart
			for _, r := range sel(line) {
				if r[0] < 0 { // the whole directive, from its #
					out = append(out, g1Hit{d.Start, base + r[1], d.Kind, match})
					continue
				}
				out = append(out, g1Hit{base + r[0], base + r[1], d.Kind, match})
			}
		}
		return out
	}
}

// fComments selects the comments of the text between tokens, skipping directive lines and disabled sections.
func fComments(kind string) g1Finder {
	return func(_ *g1Ctx, s *g1Src) []g1Hit {
		var opaque [][2]int
		for _, d := range s.dirs {
			opaque = append(opaque, [2]int{d.LineStart, d.LineEnd})
		}
		for _, r := range s.disabled {
			opaque = append(opaque, [2]int{r.Start, r.End})
		}
		var out []g1Hit
		prev := 0
		gap := func(to int) {
			for _, it := range g1sComments(s.data, prev, to, opaque) {
				if kind == "" || it.Kind == kind {
					out = append(out, g1Hit{it.Start, it.End, "", "span"})
				}
			}
		}
		for _, t := range s.toks {
			gap(t.start)
			prev = t.end
		}
		gap(len(s.data))
		return out
	}
}

func fUnion(fs ...g1Finder) g1Finder {
	return func(c *g1Ctx, s *g1Src) []g1Hit {
		var out []g1Hit
		for _, f := range fs {
			out = append(out, f(c, s)...)
		}
		slices.SortStableFunc(out, func(a, b g1Hit) int { return a.start - b.start })
		return out
	}
}

// ---- spec-derived type sets ---------------------------------------------------------------

// The tree-sitter types a lexeme can have, from grammar.js: the anonymous token, the wrapper rules
// that span only the lexeme, and identifier for the words grammar.js lists as reserved identifiers.
var (
	g1sPredefined = strings.Fields("bool byte char decimal double float int long nint nuint object sbyte short string uint ulong ushort void")
	g1sModifiers  = strings.Fields("abstract async const extern file fixed internal new override partial private protected public readonly required sealed static unsafe virtual volatile ref out in this scoped")
	g1sReserved   = strings.Fields("alias ascending by descending equals field file from global group into join let method notnull on orderby param property scoped select type typevar unmanaged var when where yield")
)

func g1TsLexeme(v G1KMVocabulary, w string) []string {
	var ts []string
	if G1KindMapHasType(v, w, false) {
		ts = append(ts, G1KindMapAnon(w))
	}
	if slices.Contains(g1sPredefined, w) {
		ts = append(ts, "predefined_type")
	}
	if slices.Contains(g1sModifiers, w) {
		ts = append(ts, "modifier")
	}
	if slices.Contains(g1sReserved, w) {
		ts = append(ts, "identifier")
	}
	switch w {
	case "true", "false":
		ts = append(ts, "boolean_literal")
	case "null":
		ts = append(ts, "null_literal")
	case "var":
		ts = append(ts, "implicit_type")
	case "_":
		ts = append(ts, "discard", "identifier")
	}
	slices.Sort(ts)
	return slices.Compact(ts)
}

// ---- B01 ----------------------------------------------------------------------------------

var g1LexConst = map[string]string{"DEFAULT": "default", "FALSE": "false", "NULL": "null", "TRUE": "true"}

func g1Lexeme(rhs string) string {
	if w, ok := g1LexConst[strings.TrimSpace(rhs)]; ok {
		return w
	}
	lits, reason := g1RequiredLiterals(rhs)
	if reason != "" || len(lits) != 1 {
		return ""
	}
	return lits[0]
}

func re(s string) *regexp.Regexp { return regexp.MustCompile(s) }

var (
	g1reDecimalWords = re(`^[0-9]`)
	g1reRealExp      = re(`[Ee][+-]?[0-9]`)
	g1reUnderscore   = re(`_`)
	g1reHexDigits    = re(`^0[xX]`)
	g1reSingleChar   = re(`^'[^\\]'$`)
	g1reCharSimple   = re(`^'\\['"\\0abefnrtv]'$`)
	g1reCharHex      = re(`^'\\x`)
	g1reCharUnicode  = re(`^'\\[uU]`)
	g1reVerbatimQ    = re(`^@"(?:[^"]|"")*""`)
	g1reRegularIS    = re(`^\$"`)
	g1reVerbatimIS   = re(`^(?:\$@|@\$)"`)
)

const g1sIntForms = "int.dec|int.hex|int.bin"

func g1IntSuffixRe(sfx string) *regexp.Regexp {
	return re(`^(?:` + g1sDecDigits + `|0[xX](?:_*[0-9A-Fa-f])+|0[bB](?:_*[01])+)` + regexp.QuoteMeta(sfx) + `$`)
}

func g1RealSuffixRe(sfx string) *regexp.Regexp {
	return re(`^(?:` + g1sDecDigits + `\.` + g1sDecDigits + `|\.` + g1sDecDigits + `|` + g1sDecDigits + `)(?:` + g1sExponent + `)?` + regexp.QuoteMeta(sfx) + `$`)
}

// identTail returns the identifier text without its optional @ and its first element.
func g1IdentTail(text string) string {
	t := strings.TrimPrefix(text, "@")
	if m := re(`^` + g1sIdentStart).FindString(t); m != "" {
		return t[len(m):]
	}
	return ""
}

func fIdentPart(c *g1Ctx, class string, tail bool) g1Finder {
	cls := re(class)
	return fTokens(func(_ *g1Src, t g1Tok) bool {
		if !g1sForms["ident.basic"].MatchString(t.text) && !g1sForms["ident.escaped"].MatchString(t.text) {
			return false
		}
		if c.keywords[t.text] {
			return false
		}
		if tail {
			return cls.MatchString(g1IdentTail(t.text))
		}
		return cls.MatchString(t.text)
	})
}

func b01Rule(c *g1Ctx, id, head, rhs, prod string) g1Rule {
	v := c.km.Vocabulary
	lex := g1Lexeme(rhs)
	switch head {
	case "keyword", "DEFAULT", "NULL", "TRUE", "FALSE", "boolean_literal", "null_literal":
		if lex == "" {
			break
		}
		return specRule("token text "+lex+" (reserved keyword or literal keyword)", g1TsLexeme(v, lex), fText(lex))
	case "contextual_keyword":
		ts := append(g1TsLexeme(v, lex), "identifier")
		slices.Sort(ts)
		return specRule("token text "+lex+" (contextual keyword: an identifier or the keyword by context)", slices.Compact(ts), fText(lex))
	case "operator_or_punctuator", "ASTERISK", "SLASH", "right_shift", "right_shift_assignment":
		switch head {
		case "right_shift":
			lex = ">>"
		case "right_shift_assignment":
			lex = ">>="
		}
		if lex == "" {
			break
		}
		return specRule("token text "+lex+" (operator or punctuator)", g1TsLexeme(v, lex), fText(lex))
	case "discard_token":
		return specRule("token text _ (discard)", g1TsLexeme(v, "_"), fText("_"))
	case "literal", "token", "identifier":
		return g1Rule{inherit: strings.TrimSpace(rhs)}
	case "input", "input_section", "input_section_part", "input_element":
		return blockRule(g1BlockMeta, "sequence structure of the input (6.3.1): neither tree has a node or token boundary for it")
	case "New_Line", "New_Line_Character", "Whitespace", "Input_Character", "Not_Slash_Or_Asterisk", "Delimited_Comment_Section":
		return blockRule(g1BlockNoNode, "tree-sitter treats it as an extra without a node and the observation carries no trivia")
	case "Comment":
		return g1Rule{inherit: strings.TrimSpace(rhs)}
	case "Single_Line_Comment":
		return specRule("comment between tokens starting with // (6.3.3)", []string{"comment"}, fComments("line-comment"))
	case "Delimited_Comment":
		return specRule("comment between tokens delimited by /* */ (6.3.3)", []string{"comment"}, fComments("delimited-comment"))
	case "Unicode_Escape_Sequence":
		w := `\\u[0-9A-Fa-f]{4}`
		if strings.Contains(rhs, `'\\U'`) {
			w = `\\U[0-9A-Fa-f]{8}`
		}
		return specRule("token containing a Unicode escape sequence (6.4.2)", []string{"character_literal", "identifier", "string_literal"},
			fFormRe(c, re(w), "ident.basic", "ident.escaped", "string.regular", "char"))
	case "Simple_Identifier", "Available_Identifier", "Basic_Identifier":
		return specRule("identifier token (6.4.3)", []string{"identifier"}, fForm(c, "ident.basic"))
	case "Escaped_Identifier":
		return specRule("@-escaped identifier token (6.4.3)", []string{"identifier"}, fForm(c, "ident.escaped"))
	case "Identifier_Start_Character":
		if strings.Contains(rhs, "Underscore") {
			return specRule("identifier starting with an underscore (6.4.3)", []string{"identifier"}, fFormRe(c, re(`^@?(?:_|\\u005[Ff]|\\U0000005[Ff])`), "ident.basic", "ident.escaped"))
		}
		return specRule("identifier starting with a letter (6.4.3)", []string{"identifier"}, fFormRe(c, re(`^@?(?:[\p{L}\p{Nl}]|`+g1sUnicodeEsc+`)`), "ident.basic", "ident.escaped"))
	case "Underscore_Character":
		switch lex {
		case "_":
			return specRule("identifier containing an underscore (6.4.3)", []string{"identifier"}, fFormRe(c, re(`_`), "ident.basic", "ident.escaped"))
		case `\u005F`, `\U0000005F`:
			return specRule("identifier containing the escaped underscore (6.4.3)", []string{"identifier"}, fFormRe(c, re(regexp.QuoteMeta(lex)), "ident.basic", "ident.escaped"))
		}
		return specRule("identifier containing an underscore escape (6.4.3)", []string{"identifier"}, fFormRe(c, re(`\\u005[Ff]|\\U0000005[Ff]`), "ident.basic", "ident.escaped"))
	case "Identifier_Part_Character":
		switch {
		case strings.Contains(rhs, "Letter"):
			return specRule("identifier with a letter after its first character (6.4.3)", []string{"identifier"}, fIdentPart(c, `[\p{L}\p{Nl}]`, true))
		case strings.Contains(rhs, "Decimal"):
			return specRule("identifier with a decimal digit after its first character (6.4.3)", []string{"identifier"}, fIdentPart(c, `\p{Nd}`, true))
		case strings.Contains(rhs, "Connecting"):
			return specRule("identifier with a connecting character after its first character (6.4.3)", []string{"identifier"}, fIdentPart(c, `\p{Pc}`, true))
		case strings.Contains(rhs, "Combining"):
			return specRule("identifier with a combining character (6.4.3)", []string{"identifier"}, fIdentPart(c, `[\p{Mn}\p{Mc}]`, true))
		default:
			return specRule("identifier with a formatting character (6.4.3)", []string{"identifier"}, fIdentPart(c, `\p{Cf}`, true))
		}
	case "Letter_Character":
		if strings.Contains(rhs, "Unicode_Escape") {
			return specRule("identifier containing a Unicode escape sequence (6.4.3)", []string{"identifier"}, fIdentPart(c, g1sUnicodeEsc, false))
		}
		return specRule("identifier containing a letter (6.4.3)", []string{"identifier"}, fIdentPart(c, `[\p{L}\p{Nl}]`, false))
	case "Combining_Character", "Decimal_Digit_Character", "Connecting_Character", "Formatting_Character":
		if strings.Contains(rhs, "Unicode_Escape") {
			return specRule("identifier containing a Unicode escape sequence (6.4.3)", []string{"identifier"}, fIdentPart(c, g1sUnicodeEsc, false))
		}
		cls := map[string]string{"Combining_Character": `[\p{Mn}\p{Mc}]`, "Decimal_Digit_Character": `\p{Nd}`, "Connecting_Character": `\p{Pc}`, "Formatting_Character": `\p{Cf}`}[head]
		return specRule("identifier containing a "+strings.ToLower(strings.TrimSuffix(head, "_Character"))+" character (6.4.3)", []string{"identifier"}, fIdentPart(c, cls, false))
	case "Integer_Literal":
		forms := map[string][]string{"Decimal_Integer_Literal": {"int.dec"}, "Hexadecimal_Integer_Literal": {"int.hex"}, "Binary_Integer_Literal": {"int.bin"}}[strings.TrimSpace(rhs)]
		return specRule("integer literal token (6.4.5.3)", []string{"integer_literal"}, fForm(c, forms...))
	case "Decimal_Integer_Literal":
		return specRule("decimal integer literal token (6.4.5.3)", []string{"integer_literal"}, fForm(c, "int.dec"))
	case "Hexadecimal_Integer_Literal":
		return specRule("hexadecimal integer literal token (6.4.5.3)", []string{"integer_literal"}, fForm(c, "int.hex"))
	case "Binary_Integer_Literal":
		return specRule("binary integer literal token (6.4.5.3)", []string{"integer_literal"}, fForm(c, "int.bin"))
	case "Decorated_Decimal_Digit":
		return specRule("decimal literal with a digit separator (6.4.5.3)", []string{"integer_literal", "real_literal"}, fFormRe(c, g1reUnderscore, "int.dec", "real.1", "real.2", "real.3", "real.4"))
	case "Decimal_Digit":
		return specRule("decimal literal token (6.4.5.3)", []string{"integer_literal", "real_literal"}, fForm(c, "int.dec", "real.1", "real.2", "real.3", "real.4"))
	case "Integer_Type_Suffix":
		return specRule("integer literal with the suffix "+lex+" (6.4.5.3)", []string{"integer_literal"}, fTokens(func(_ *g1Src, t g1Tok) bool { return g1IntSuffixRe(lex).MatchString(t.text) }))
	case "Decorated_Hex_Digit":
		return specRule("hexadecimal literal with a digit separator (6.4.5.3)", []string{"integer_literal"}, fFormRe(c, g1reUnderscore, "int.hex"))
	case "Hex_Digit":
		r := map[string]*regexp.Regexp{"'0'..'9'": re(`^0[xX]_*[0-9]`), "'A'..'F'": re(`^0[xX][0-9A-Fa-f_]*[A-F]`), "'a'..'f'": re(`^0[xX][0-9A-Fa-f_]*[a-f]`)}[strings.Fields(rhs)[0]]
		if r == nil {
			r = g1reHexDigits
		}
		return specRule("hexadecimal literal containing "+strings.Fields(rhs)[0]+" (6.4.5.3)", []string{"integer_literal"}, fFormRe(c, r, "int.hex"))
	case "Decorated_Binary_Digit":
		return specRule("binary literal with a digit separator (6.4.5.3)", []string{"integer_literal"}, fFormRe(c, g1reUnderscore, "int.bin"))
	case "Binary_Digit":
		return specRule("binary literal containing the digit "+lex+" (6.4.5.3)", []string{"integer_literal"}, fFormRe(c, re(`^0[bB][01_]*`+lex), "int.bin"))
	case "Real_Literal":
		n := 1
		if i := strings.Index(prod, "[alt "); i >= 0 {
			fmt.Sscanf(prod[i:], "[alt %d", &n)
		}
		return specRule(fmt.Sprintf("real literal of form %d (6.4.5.4)", n), []string{"real_literal"}, fForm(c, fmt.Sprintf("real.%d", n)))
	case "Exponent_Part":
		return specRule("real literal with an exponent (6.4.5.4)", []string{"real_literal"}, fFormRe(c, g1reRealExp, "real.1", "real.2", "real.3"))
	case "Sign":
		return specRule("real literal with an exponent sign "+lex+" (6.4.5.4)", []string{"real_literal"}, fFormRe(c, re(`[Ee]`+regexp.QuoteMeta(lex)), "real.1", "real.2", "real.3"))
	case "Real_Type_Suffix":
		return specRule("real literal with the suffix "+lex+" (6.4.5.4)", []string{"real_literal"}, fTokens(func(_ *g1Src, t g1Tok) bool { return g1RealSuffixRe(lex).MatchString(t.text) }))
	case "Character_Literal":
		return specRule("character literal token (6.4.5.5)", []string{"character_literal"}, fForm(c, "char"))
	case "Character":
		r := map[string]*regexp.Regexp{"Single_Character": g1reSingleChar, "Simple_Escape_Sequence": g1reCharSimple, "Hexadecimal_Escape_Sequence": g1reCharHex, "Unicode_Escape_Sequence": g1reCharUnicode}[strings.TrimSpace(strings.Split(rhs, "[")[0])]
		return specRule("character literal of one "+strings.TrimSpace(strings.Split(rhs, "[")[0])+" (6.4.5.5)", []string{"character_literal"}, fFormRe(c, r, "char"))
	case "Single_Character":
		return specRule("character literal of one plain character (6.4.5.5)", []string{"character_literal"}, fFormRe(c, g1reSingleChar, "char"))
	case "Simple_Escape_Sequence":
		return specRule("simple escape sequence "+lex+" in a character or regular string literal (6.4.5.5)", []string{"escape_sequence"}, fEscape("simple:"+strings.TrimPrefix(lex, `\`)))
	case "Hexadecimal_Escape_Sequence":
		return specRule(`hexadecimal escape sequence \x in a character or regular string literal (6.4.5.5)`, []string{"escape_sequence"}, fEscape("hex"))
	case "String_Literal":
		forms := map[string][]string{"Regular_String_Literal": {"string.regular"}, "Verbatim_String_Literal": {"string.verbatim"}}[strings.TrimSpace(strings.Split(rhs, "[")[0])]
		return specRule("string literal token (6.4.5.6)", []string{"string_literal", "verbatim_string_literal"}, fForm(c, forms...))
	case "Regular_String_Literal":
		return specRule("regular string literal token (6.4.5.6)", []string{"string_literal"}, fForm(c, "string.regular"))
	case "Verbatim_String_Literal":
		return specRule("verbatim string literal token (6.4.5.6)", []string{"verbatim_string_literal"}, fForm(c, "string.verbatim"))
	case "Regular_String_Literal_Character":
		a := strings.TrimSpace(strings.Split(rhs, "[")[0])
		switch a {
		case "Single_Regular_String_Literal_Character":
			return specRule("regular string literal (6.4.5.6)", []string{"string_literal"}, fForm(c, "string.regular"))
		case "Simple_Escape_Sequence":
			return specRule("regular string literal with a simple escape sequence (6.4.5.6)", []string{"string_literal"}, fFormRe(c, re(`\\['"\\0abefnrtv]`), "string.regular"))
		case "Hexadecimal_Escape_Sequence":
			return specRule(`regular string literal with a \x escape sequence (6.4.5.6)`, []string{"string_literal"}, fFormRe(c, re(`\\x`), "string.regular"))
		default:
			return specRule("regular string literal with a Unicode escape sequence (6.4.5.6)", []string{"string_literal"}, fFormRe(c, re(g1sUnicodeEsc), "string.regular"))
		}
	case "Single_Regular_String_Literal_Character":
		return specRule("regular string literal (6.4.5.6)", []string{"string_literal"}, fForm(c, "string.regular"))
	case "Verbatim_String_Literal_Character", "Single_Verbatim_String_Literal_Character":
		return specRule("verbatim string literal (6.4.5.6)", []string{"verbatim_string_literal"}, fForm(c, "string.verbatim"))
	case "Quote_Escape_Sequence":
		return specRule(`verbatim string literal with a "" escape (6.4.5.6)`, []string{"verbatim_string_literal"}, fFormRe(c, g1reVerbatimQ, "string.verbatim"))
	case "interpolated_string_expression":
		r := g1reRegularIS
		if strings.Contains(rhs, "verbatim") {
			r = g1reVerbatimIS
		}
		return diffRule("interpolated string expression ("+strings.TrimPrefix(strings.TrimPrefix(rhs, "interpolated_"), "_string_expression")+")", []string{"InterpolatedStringExpression"}, nil, fNodes([]string{"InterpolatedStringExpression"}, nodeText(r, true)))
	case "interpolated_regular_string_expression":
		return diffRule("regular interpolated string expression (12.8.3)", []string{"InterpolatedStringExpression"}, nil, fNodes([]string{"InterpolatedStringExpression"}, nodeText(g1reRegularIS, true)))
	case "interpolated_verbatim_string_expression":
		return diffRule("verbatim interpolated string expression (12.8.3)", []string{"InterpolatedStringExpression"}, nil, fNodes([]string{"InterpolatedStringExpression"}, nodeText(g1reVerbatimIS, true)))
	case "regular_interpolation":
		return diffRule("interpolation of a regular interpolated string (12.8.3)", []string{"Interpolation"}, nil, fNodes([]string{"Interpolation"}, ancestorText("InterpolatedStringExpression", g1reRegularIS)))
	case "verbatim_interpolation":
		return diffRule("interpolation of a verbatim interpolated string (12.8.3)", []string{"Interpolation"}, nil, fNodes([]string{"Interpolation"}, ancestorText("InterpolatedStringExpression", g1reVerbatimIS)))
	case "interpolation_minimum_width":
		return diffRule("interpolation alignment clause (12.8.3)", []string{"InterpolationAlignmentClause"}, nil, fNodes([]string{"InterpolationAlignmentClause"}, nil))
	case "Interpolated_Regular_String_Start":
		return diffRule(`interpolated regular string start $" (12.8.3)`, []string{"InterpolatedStringStartToken"}, nil, fTokensStart(`$"`))
	case "Interpolated_Verbatim_String_Start":
		return diffRule("interpolated verbatim string start "+lex+" (12.8.3)", []string{"InterpolatedVerbatimStringStartToken"}, nil, fTokensStart(lex))
	case "Interpolated_Regular_String_End", "Interpolated_Verbatim_String_End":
		r := g1reRegularIS
		if head == "Interpolated_Verbatim_String_End" {
			r = g1reVerbatimIS
		}
		return diffRule("interpolated string end quote (12.8.3)", []string{"InterpolatedStringEndToken"}, nil, fTokensIn("InterpolatedStringEndToken", "InterpolatedStringExpression", r, nil))
	case "Interpolated_Regular_String_Element", "Interpolated_Verbatim_String_Element":
		verbatim := head == "Interpolated_Verbatim_String_Element"
		r, kind := g1reRegularIS, "regular"
		if verbatim {
			r, kind = g1reVerbatimIS, "verbatim"
		}
		switch strings.TrimSpace(rhs) {
		case "Simple_Escape_Sequence":
			return specRule("simple escape sequence in the text of a "+kind+" interpolated string (6.4.5.5)", []string{"escape_sequence"}, fTextEscape("simple:", r))
		case "Hexadecimal_Escape_Sequence":
			return specRule("hexadecimal escape sequence in the text of a "+kind+" interpolated string (6.4.5.5)", []string{"escape_sequence"}, fTextEscape("hex", r))
		case "Unicode_Escape_Sequence":
			return specRule("Unicode escape sequence in the text of a "+kind+" interpolated string (6.4.5.5)", []string{"escape_sequence"}, fTextEscape("unicode", r))
		case "Open_Brace_Escape_Sequence", "Close_Brace_Escape_Sequence", "Quote_Escape_Sequence":
			return blockRule(g1BlockExternal, "tree-sitter's external scanner decides how brace and quote escapes inside interpolated text are represented; grammar.js does not say")
		}
		return diffRule("text of a "+kind+" interpolated string (12.8.3)", []string{"InterpolatedStringTextToken"}, nil, fTokensIn("InterpolatedStringTextToken", "InterpolatedStringExpression", r, nil))
	case "Interpolated_Regular_String_Mid", "Interpolated_Regular_String_Character":
		return diffRule("text of a regular interpolated string (12.8.3)", []string{"InterpolatedStringTextToken"}, nil, fTokensIn("InterpolatedStringTextToken", "InterpolatedStringExpression", g1reRegularIS, nil))
	case "Interpolated_Verbatim_String_Mid", "Interpolated_Verbatim_String_Character":
		return diffRule("text of a verbatim interpolated string (12.8.3)", []string{"InterpolatedStringTextToken"}, nil, fTokensIn("InterpolatedStringTextToken", "InterpolatedStringExpression", g1reVerbatimIS, nil))
	case "Regular_Interpolation_Format":
		return diffRule("format clause of a regular interpolation (12.8.3)", []string{"InterpolationFormatClause"}, nil, fNodes([]string{"InterpolationFormatClause"}, ancestorText("InterpolatedStringExpression", g1reRegularIS)))
	case "Verbatim_Interpolation_Format":
		return diffRule("format clause of a verbatim interpolation (12.8.3)", []string{"InterpolationFormatClause"}, nil, fNodes([]string{"InterpolationFormatClause"}, ancestorText("InterpolatedStringExpression", g1reVerbatimIS)))
	case "Open_Brace_Escape_Sequence", "Close_Brace_Escape_Sequence":
		return blockRule(g1BlockExternal, "tree-sitter's external scanner decides how {{ and }} inside interpolated text are represented; grammar.js does not say")
	}
	if strings.HasPrefix(head, "PP_") {
		return ppRule(c, head, rhs, lex)
	}
	return blockRule("RULE_NOT_AUTHORED", "no rule for the production "+id+" "+head)
}

func fTokensStart(lexeme string) g1Finder {
	return fTokens(func(_ *g1Src, t g1Tok) bool { return t.text == lexeme })
}

// ---- preprocessor directives (6.5) ---------------------------------------------------------

var g1PPNodes = map[string][]string{
	"define": {"preproc_define"}, "undef": {"preproc_undef"}, "if": {"preproc_if"}, "elif": {"preproc_elif"},
	"else": {"preproc_else"}, "endif": {`"#endif"`}, "error": {"preproc_error"}, "warning": {"preproc_warning"},
	"region": {"preproc_region"}, "endregion": {"preproc_endregion"}, "line": {"preproc_line"},
	"nullable": {"preproc_nullable"}, "pragma": {"preproc_pragma"},
}

func ppDirRule(form string, kinds ...string) g1Rule {
	var ts []string
	for _, k := range kinds {
		ts = append(ts, g1PPNodes[k]...)
	}
	slices.Sort(ts)
	return specRule(form, ts, fDirective(kinds...))
}

func ppRule(c *g1Ctx, head, rhs, lex string) g1Rule {
	all := []string{"define", "undef", "if", "elif", "else", "endif", "error", "warning", "region", "endregion", "line", "nullable", "pragma"}
	lineTS := func(kinds ...string) []string {
		var ts []string
		for _, k := range kinds {
			ts = append(ts, g1PPNodes[k]...)
		}
		slices.Sort(ts)
		return ts
	}
	switch head {
	case "PP_Directive", "PP_Start":
		return ppDirRule("directive line (6.5.1)", all...)
	case "PP_Kind":
		g := strings.TrimSpace(strings.Split(rhs, "[")[0])
		kinds := map[string][]string{"PP_Declaration": {"define", "undef"}, "PP_Conditional": {"if", "elif", "else", "endif"}, "PP_Line": {"line"}, "PP_Diagnostic": {"error", "warning"}, "PP_Region": {"region", "endregion"}, "PP_Pragma": {"pragma"}, "PP_Nullable": {"nullable"}}[g]
		return ppDirRule("directive line of kind "+g+" (6.5.1)", kinds...)
	case "PP_Declaration":
		return ppDirRule("#"+lex+" directive (6.5.4)", lex)
	case "PP_Conditional":
		k := map[string]string{"PP_If_Section": "if", "PP_Elif_Section": "elif", "PP_Else_Section": "else", "PP_Endif": "endif"}[strings.TrimSpace(strings.Split(rhs, "[")[0])]
		return ppDirRule("#"+k+" directive (6.5.5)", k)
	case "PP_If_Section":
		return ppDirRule("#if directive (6.5.5)", "if")
	case "PP_Elif_Section":
		return ppDirRule("#elif directive (6.5.5)", "elif")
	case "PP_Else_Section":
		return ppDirRule("#else directive (6.5.5)", "else")
	case "PP_Endif":
		return ppDirRule("#endif directive (6.5.5)", "endif")
	case "PP_Diagnostic":
		k := strings.Trim(strings.Fields(rhs)[0], "'")
		return ppDirRule("#"+k+" directive (6.5.6)", k)
	case "PP_Region":
		return ppDirRule("region directive (6.5.7)", "region", "endregion")
	case "PP_Start_Region":
		return ppDirRule("#region directive (6.5.7)", "region")
	case "PP_End_Region":
		return ppDirRule("#endregion directive (6.5.7)", "endregion")
	case "PP_Line":
		return ppDirRule("#line directive (6.5.8)", "line")
	case "PP_Nullable":
		return ppDirRule("#nullable directive (6.5.9)", "nullable")
	case "PP_Pragma":
		return ppDirRule("#pragma directive (6.5.10)", "pragma")
	case "PP_Conditional_Symbol":
		return specRule("conditional symbol of #define or #undef (6.5.4)", []string{"identifier"}, fDirectiveLine([]string{"define", "undef"}, func(l string) [][2]int {
			if w, at := g1sDefineSymbol(l); w != "" {
				return [][2]int{{at, at + len(w)}}
			}
			return nil
		}, "span"))
	case "PP_Expression", "PP_Or_Expression", "PP_And_Expression", "PP_Equality_Expression", "PP_Unary_Expression", "PP_Primary_Expression":
		form := ""
		switch head {
		case "PP_Or_Expression":
			form = "or"
		case "PP_And_Expression":
			form = "and"
		case "PP_Equality_Expression":
			form = "equality"
		case "PP_Unary_Expression":
			form = "not"
			if !strings.Contains(rhs, "'!'") {
				form = ""
			}
		case "PP_Primary_Expression":
			switch {
			case strings.Contains(rhs, "TRUE"):
				form = "true"
			case strings.Contains(rhs, "FALSE"):
				form = "false"
			case strings.Contains(rhs, "PP_Conditional_Symbol"):
				form = "symbol"
			default:
				form = "paren"
			}
		}
		ts := lineTS("if", "elif")
		return specRule("condition of #if or #elif with the form "+head+" (6.5.3)", ts, func(_ *g1Ctx, s *g1Src) []g1Hit {
			var out []g1Hit
			for _, d := range s.dirs {
				line, end := s.directive(d)
				k := g1sDirectiveKind(line)
				if k != "if" && k != "elif" {
					continue
				}
				if form == "" || slices.Contains(g1sCondForms(line), form) {
					out = append(out, g1Hit{d.Start, end, d.Kind, "starts-at"})
				}
			}
			return out
		})
	case "PP_Message":
		return specRule("message of #error, #warning, #region or #endregion (6.5.6, 6.5.7)", []string{"preproc_arg"}, fDirectiveLine([]string{"error", "warning", "region", "endregion"}, func(l string) [][2]int {
			if a, b, ok := g1sMessageSpan(l); ok {
				return [][2]int{{a, b}}
			}
			return nil
		}, "span"))
	case "PP_Line_Indicator":
		w := ""
		if lex == "hidden" {
			w = "hidden"
		}
		if strings.HasPrefix(rhs, "DEFAULT") {
			w = "default"
		} else if strings.HasPrefix(rhs, "Decimal_Digit+ PP_Whitespace") {
			w = "number-file"
		} else if rhs == "Decimal_Digit+" {
			w = "number"
		}
		return specRule("#line directive with the indicator "+w+" (6.5.8)", lineTS("line"), fDirectiveLine([]string{"line"}, func(l string) [][2]int {
			if f, _, _ := g1sLineForm(l); f == w {
				return [][2]int{{-1, len(l)}}
			}
			return nil
		}, "starts-at"))
	case "PP_Compilation_Unit_Name":
		return specRule("file name of a #line directive (6.5.8)", []string{"string_literal"}, fDirectiveLine([]string{"line"}, func(l string) [][2]int {
			if f, a, b := g1sLineForm(l); f == "number-file" {
				return [][2]int{{a, b}}
			}
			return nil
		}, "span"))
	case "PP_Nullable_Action", "PP_Nullable_Target":
		return specRule("word "+lex+" of a #nullable directive (6.5.9)", []string{G1KindMapAnon(lex)}, fDirectiveLine([]string{"nullable"}, func(l string) [][2]int {
			a, at, t, tt := g1sNullableParts(l)
			if a == lex {
				return [][2]int{{at, at + len(a)}}
			}
			if t == lex {
				return [][2]int{{tt, tt + len(t)}}
			}
			return nil
		}, "span"))
	}
	return blockRule(g1BlockNoNode, "whitespace, line end or text of a directive line: tree-sitter has no node for it and the observation carries no trivia")
}

// ---- B02 ----------------------------------------------------------------------------------

type g1B02 struct {
	nodes  []string
	toks   []string // a direct token of one of these kinds
	text   *regexp.Regexp
	notRe  *regexp.Regexp
	child  []string
	form   string
	block  string
	reason string
}

var (
	g1pBool     = []string{"BoolKeyword"}
	g1pIntegral = []string{"SByteKeyword", "ByteKeyword", "ShortKeyword", "UShortKeyword", "IntKeyword", "UIntKeyword", "LongKeyword", "ULongKeyword", "CharKeyword"}
	g1pFloat    = []string{"FloatKeyword", "DoubleKeyword"}
	g1pNumeric  = append(append(slices.Clone(g1pIntegral), g1pFloat...), "DecimalKeyword")
	g1pSimple   = append(slices.Clone(g1pNumeric), "BoolKeyword")
)

func b02Pred(r g1B02) func(s *g1Src, i int) bool {
	var ps []func(s *g1Src, i int) bool
	if r.toks != nil {
		ps = append(ps, nodeTok(r.toks...))
	}
	if r.text != nil {
		ps = append(ps, nodeText(r.text, true))
	}
	if r.notRe != nil {
		ps = append(ps, nodeText(r.notRe, false))
	}
	if r.child != nil {
		ps = append(ps, childKinds(r.child...))
	}
	if len(ps) == 0 {
		return nil
	}
	return nodeAnd(ps...)
}

func predef(form string, toks []string) g1B02 {
	return g1B02{nodes: []string{"PredefinedType"}, toks: toks, form: form}
}

func nodeForm(form string, nodes ...string) g1B02 { return g1B02{nodes: nodes, form: form} }

func blockB02(code, reason string) g1B02 { return g1B02{block: code, reason: reason} }

var g1reVoidPtr = re(`^void\s*\*`)

// g1B02Table maps the alternative's ordinal in csharp-B02 (the number after ".a") to its rule.
var g1B02Table = map[int]g1B02{
	1:  blockB02(g1BlockPosition, "namespace_name is a name in a namespace position; the observation records no slot"),
	2:  blockB02(g1BlockPosition, "type_name is a name in a type position; the observation records no slot"),
	3:  blockB02(g1BlockPosition, "namespace_or_type_name is decided by its position"),
	4:  blockB02(g1BlockPosition, "an alias-qualified name occurs in type and expression positions alike"),
	5:  blockB02(g1BlockUnion, "type: reference_type is a union production"),
	6:  blockB02(g1BlockUnion, "type: value_type is a union production"),
	7:  blockB02(g1BlockPosition, "type_parameter is an identifier in a type position"),
	8:  nodeForm("pointer type (8.8)", "PointerType"),
	9:  blockB02(g1BlockUnion, "reference_type is a union production"),
	10: blockB02(g1BlockSemantic, "class_type is told from interface_type and delegate_type only by binding"),
	11: blockB02(g1BlockSemantic, "interface_type is told from class_type only by binding"),
	12: nodeForm("array type (8.2.1)", "ArrayType"),
	13: blockB02(g1BlockSemantic, "delegate_type is told from class_type only by binding"),
	14: blockB02(g1BlockSemantic, "dynamic is an identifier until bound"),
	15: blockB02(g1BlockSemantic, "class_type: type_name is told from other type names only by binding"),
	16: predef("object type (8.2.4)", []string{"ObjectKeyword"}),
	17: predef("string type (8.2.5)", []string{"StringKeyword"}),
	18: blockB02(g1BlockSemantic, "interface_type: type_name is told from other type names only by binding"),
	19: nodeForm("array type with rank specifiers (17.2)", "ArrayType"),
	20: blockB02(g1BlockUnion, "non_array_type is a union production"),
	21: nodeForm("pointer type as a non-array type (24.3)", "PointerType"),
	22: blockB02(g1BlockUnion, "non_array_non_nullable_type is a union production"),
	23: blockB02(g1BlockSemantic, "class_type is told from other type names only by binding"),
	24: blockB02(g1BlockSemantic, "interface_type is told from other type names only by binding"),
	25: blockB02(g1BlockSemantic, "delegate_type is told from other type names only by binding"),
	26: blockB02(g1BlockSemantic, "dynamic is an identifier until bound"),
	27: blockB02(g1BlockPosition, "type_parameter is an identifier in a type position"),
	28: {nodes: []string{"ArrayRankSpecifier"}, text: re(`^\[\s*(?:,\s*)*\]$`), form: "rank specifier: [ and commas only (17.2)"},
	29: blockB02(g1BlockSemantic, "delegate_type is told from class_type only by binding"),
	30: nodeForm("nullable type annotation ? (8.3.1)", "NullableType"),
	31: blockB02(g1BlockUnion, "value_type: non_nullable_value_type is a union production"),
	32: nodeForm("nullable value type (8.3.1)", "NullableType"),
	33: blockB02(g1BlockUnion, "non_nullable_value_type: struct_type is a union production"),
	34: blockB02(g1BlockUnion, "non_nullable_value_type: enum_type is a union production"),
	35: blockB02(g1BlockSemantic, "struct_type: type_name is told from other type names only by binding"),
	36: predef("simple type (8.3.1)", g1pSimple),
	37: nodeForm("tuple type as a struct type (8.3.11)", "TupleType"),
	38: predef("numeric type (8.3.1)", g1pNumeric),
	39: predef("bool type (8.3.1)", g1pBool),
	40: predef("integral type (8.3.6)", g1pIntegral),
	41: predef("floating-point type (8.3.7)", g1pFloat),
	42: predef("decimal type (8.3.8)", []string{"DecimalKeyword"}),
	43: predef("sbyte type", []string{"SByteKeyword"}),
	44: predef("byte type", []string{"ByteKeyword"}),
	45: predef("short type", []string{"ShortKeyword"}),
	46: predef("ushort type", []string{"UShortKeyword"}),
	47: predef("int type", []string{"IntKeyword"}),
	48: predef("uint type", []string{"UIntKeyword"}),
	49: predef("long type", []string{"LongKeyword"}),
	50: predef("ulong type", []string{"ULongKeyword"}),
	51: predef("char type", []string{"CharKeyword"}),
	52: predef("float type", []string{"FloatKeyword"}),
	53: predef("double type", []string{"DoubleKeyword"}),
	54: nodeForm("tuple type (8.3.11)", "TupleType"),
	55: nodeForm("tuple type element (8.3.11)", "TupleElement"),
	56: blockB02(g1BlockSemantic, "enum_type is told from other type names only by binding"),
	57: nodeForm("nullable value type (8.3.12)", "NullableType"),
	58: nodeForm("type argument list (8.4.2)", "TypeArgumentList"),
	59: blockB02(g1BlockUnion, "type_argument: type is a union production"),
	60: blockB02(g1BlockPosition, "type_parameter is an identifier in a type position"),
	61: blockB02(g1BlockUnion, "unmanaged_type: value_type is a union production"),
	62: nodeForm("pointer type as an unmanaged type (8.8)", "PointerType"),
	63: blockB02(g1BlockSemantic, "a variable reference is an expression told from a value only by binding"),
	64: {nodes: []string{"Argument"}, toks: []string{"InKeyword"}, form: "argument with the in modifier (9.5)"},
	65: {nodes: []string{"Argument"}, toks: []string{"RefKeyword"}, form: "argument with the ref modifier (9.5)"},
	66: {nodes: []string{"Argument"}, toks: []string{"OutKeyword"}, form: "argument with the out modifier (9.5)"},
	67: nodeForm("pointer member access (24.6.3)", "PointerMemberAccessExpression"),
	68: blockB02(g1BlockSemantic, "a pointer element access is told from an array element access only by binding"),
	69: nodeForm("pointer indirection (24.6.2)", "PointerIndirectionExpression"),
	70: nodeForm("address-of expression (24.6.5)", "AddressOfExpression"),
	71: nodeForm("type parameter constraints clause (15.2.5)", "TypeParameterConstraintClause"),
	72: nodeForm("type parameter constraints clause (15.2.5)", "TypeParameterConstraintClause"),
	73: blockB02(g1BlockSemantic, "a leading class_type constraint is told from an interface_type constraint only by binding"),
	74: blockB02(g1BlockSemantic, "a leading interface_type or type_parameter constraint is told from a class_type constraint only by binding"),
	75: {nodes: []string{"TypeParameterConstraintClause"}, child: []string{"ConstructorConstraint"}, form: "constraint clause holding only a constructor constraint (15.2.5)"},
	76: blockB02(g1BlockSemantic, "a class_type constraint is told from an interface_type constraint only by binding"),
	77: nodeForm("class constraint (15.2.5)", "ClassConstraint"),
	78: nodeForm("struct constraint (15.2.5)", "StructConstraint"),
	79: nodeForm("default constraint (15.2.5)", "DefaultConstraint"),
	80: blockB02(g1BlockSemantic, "an interface_type constraint is told from a class_type constraint only by binding"),
	81: blockB02(g1BlockSemantic, "a type_parameter constraint is told from a type constraint only by binding"),
	82: blockB02(g1BlockSemantic, "secondary_constraints mix interface and type parameter constraints, told apart only by binding"),
	83: nodeForm("constructor constraint (15.2.5)", "ConstructorConstraint"),
	84: {nodes: []string{"Parameter"}, toks: []string{"RefKeyword"}, form: "parameter with the ref modifier (15.6.2.1)"},
	85: {nodes: []string{"Parameter"}, toks: []string{"RefKeyword"}, text: re(`^(?s:.*?)\bref\s+readonly\b`), form: "parameter with ref readonly (15.6.2.1)"},
	86: {nodes: []string{"Parameter"}, toks: []string{"RefKeyword", "OutKeyword", "InKeyword"}, form: "parameter with a ref, out or in modifier (15.6.2.1)"},
	87: {nodes: []string{"Parameter"}, toks: []string{"RefKeyword"}, form: "parameter with the ref modifier (15.6.2.1)"},
	88: {nodes: []string{"Parameter"}, toks: []string{"OutKeyword"}, form: "parameter with the out modifier (15.6.2.1)"},
	89: {nodes: []string{"Parameter"}, toks: []string{"InKeyword"}, form: "parameter with the in modifier (15.6.2.1)"},
	90: nodeForm("pointer type (24.3)", "PointerType"),
	91: {nodes: []string{"PointerType"}, text: g1reVoidPtr, form: "void pointer type (24.3)"},
	92: {nodes: []string{"PointerType"}, notRe: g1reVoidPtr, form: "data pointer type to a value type (24.3)"},
	93: {nodes: []string{"PointerType"}, text: re(`^void\s*\*\s*\*`), form: "pointer type to a void pointer (24.3)"},
	94: {nodes: []string{"PointerType"}, text: re(`^void\s*\*$`), form: "void pointer type (24.3)"},
	95: nodeForm("pointer indirection (24.6.2)", "PointerIndirectionExpression"),
	96: nodeForm("pointer member access (24.6.3)", "PointerMemberAccessExpression"),
	97: blockB02(g1BlockSemantic, "a pointer element access is told from an array element access only by binding"),
	98: nodeForm("address-of expression (24.6.5)", "AddressOfExpression"),
}

// ---- rule assembly ------------------------------------------------------------------------

// g1BuildRules builds the rule of every alternative (in input order) and resolves the inherited
// ones. It also fills c.keywords from the reserved keyword productions.
func g1BuildRules(c *g1Ctx, srcs []g1RuleSrc) ([]g1Rule, error) {
	c.keywords, c.puncts = map[string]bool{}, map[string]bool{}
	for _, a := range srcs {
		if a.Row != g1ExpRowB01 {
			continue
		}
		head, rhs, ok := g1SplitProduction(a.Production)
		if !ok {
			continue
		}
		switch head {
		case "keyword":
			if w := g1Lexeme(rhs); w != "" {
				c.keywords[w] = true
			}
		case "operator_or_punctuator":
			if w := g1Lexeme(rhs); w != "" {
				c.puncts[w] = true
			}
		case "right_shift":
			c.puncts[">>"] = true
		case "right_shift_assignment":
			c.puncts[">>="] = true
		}
	}
	if len(c.keywords) != 77 {
		return nil, fmt.Errorf("expected 77 reserved keywords, got %d", len(c.keywords))
	}
	rules := make([]g1Rule, len(srcs))
	byHead := map[string][]int{}
	for i, a := range srcs {
		head, rhs, ok := g1SplitProduction(a.Production)
		if !ok {
			return nil, fmt.Errorf("%s: production form", a.ID)
		}
		switch a.Row {
		case g1ExpRowB01:
			rules[i] = b01Rule(c, a.ID, head, rhs, a.Production)
			byHead[head] = append(byHead[head], i)
		case g1ExpRowB02:
			var n int
			if _, err := fmt.Sscanf(a.ID[strings.LastIndex(a.ID, ".a")+2:], "%d", &n); err != nil {
				return nil, fmt.Errorf("%s: ordinal", a.ID)
			}
			r, ok := g1B02Table[n]
			if !ok {
				return nil, fmt.Errorf("%s: no B02 rule", a.ID)
			}
			switch {
			case r.block != "":
				rules[i] = blockRule(r.block, r.reason)
			default:
				rules[i] = diffRule(r.form, r.nodes, nil, fNodes(r.nodes, b02Pred(r)))
			}
		}
	}
	// inherited alternatives union the finders and types of the productions they name; a target that
	// itself inherits is resolved first (token: identifier -> identifier: Simple_Identifier -> ...)
	for pass := 0; pass < 4; pass++ {
		for i, r := range rules {
			if r.inherit == "" {
				continue
			}
			ready := true
			for _, j := range byHead[r.inherit] {
				if rules[j].inherit != "" {
					ready = false
				}
			}
			if !ready {
				continue
			}
			var fs []g1Finder
			var ts, forms []string
			basis := G1BasisSpec
			for _, j := range byHead[r.inherit] {
				t := rules[j]
				if t.block != "" {
					continue // a blocked target adds nothing
				}
				fs = append(fs, t.find)
				ts = append(ts, t.ts...)
				for _, k := range t.kinds {
					ts = append(ts, c.kmIdx[k].TS...)
				}
				if t.basis == G1BasisDiff {
					basis = G1BasisDiff
				}
				forms = append(forms, t.form)
			}
			if len(fs) == 0 {
				rules[i] = blockRule(g1BlockMeta, "the production it names has no locatable alternative")
				continue
			}
			slices.Sort(ts)
			rules[i] = g1Rule{basis: basis, form: "any alternative of " + r.inherit, ts: slices.Compact(ts), find: fUnion(fs...)}
		}
	}
	for i, r := range rules {
		if r.inherit != "" {
			return nil, fmt.Errorf("%s: inheritance did not resolve", srcs[i].ID)
		}
	}
	return rules, nil
}
