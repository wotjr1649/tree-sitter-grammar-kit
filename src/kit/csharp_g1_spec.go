package kit

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Specification-derived lexical facts for the G1 expectation of the lexical row (#233, G1-3a).
//
// This file is the deriver of the spec-derived expectations. It holds the ECMA-334 lexical grammar as published in the C# standard draft v13 on learn.microsoft.com
// grammar (clauses 6.3 to 6.5) as regular expressions and scanners over plain text. It must not
// read the Roslyn observation, the kind map or any native output: the independence test
// (TestCSharpG1ExpectedSeparation) parses this file and rejects such references. Lexemes of
// keywords and punctuators are not listed here: they come from the quoted terminals of the
// specification's own productions, which the feature-alternatives inventory carries.

const (
	g1sIntSuffix  = `(?:[Uu][Ll]?|[Ll][Uu]?)`                                                 // 6.4.5.3 Integer_Type_Suffix
	g1sRealSuffix = `[FfDdMm]`                                                                // 6.4.5.4 Real_Type_Suffix
	g1sDecDigits  = `[0-9](?:_*[0-9])*`                                                       // Decimal_Digit Decorated_Decimal_Digit*
	g1sExponent   = `[Ee][+-]?` + g1sDecDigits                                                // Exponent_Part
	g1sNewLines   = `\r\n\x{85}\x{2028}\x{2029}`                                              // New_Line_Character
	g1sEscape     = `\\(?:['"\\0abefnrtv]|x[0-9A-Fa-f]{1,4}|u[0-9A-Fa-f]{4}|U[0-9A-Fa-f]{8})` // 6.4.5.5 escape sequences
	g1sUnicodeEsc = `\\(?:u[0-9A-Fa-f]{4}|U[0-9A-Fa-f]{8})`                                   // 6.4.2 Unicode_Escape_Sequence
	g1sIdentStart = `(?:[\p{L}\p{Nl}_]|` + g1sUnicodeEsc + `)`
	g1sIdentPart  = `(?:[\p{L}\p{Nl}\p{Nd}\p{Pc}\p{Mn}\p{Mc}\p{Cf}]|` + g1sUnicodeEsc + `)`
)

// g1sForms are the lexical forms of the spec productions the lexical row lists, anchored to a whole token.
var g1sForms = map[string]*regexp.Regexp{
	"int.dec":         regexp.MustCompile(`^` + g1sDecDigits + g1sIntSuffix + `?$`),
	"int.hex":         regexp.MustCompile(`^0[xX](?:_*[0-9A-Fa-f])+` + g1sIntSuffix + `?$`),
	"int.bin":         regexp.MustCompile(`^0[bB](?:_*[01])+` + g1sIntSuffix + `?$`),
	"real.1":          regexp.MustCompile(`^` + g1sDecDigits + `\.` + g1sDecDigits + `(?:` + g1sExponent + `)?` + g1sRealSuffix + `?$`),
	"real.2":          regexp.MustCompile(`^\.` + g1sDecDigits + `(?:` + g1sExponent + `)?` + g1sRealSuffix + `?$`),
	"real.3":          regexp.MustCompile(`^` + g1sDecDigits + g1sExponent + g1sRealSuffix + `?$`),
	"real.4":          regexp.MustCompile(`^` + g1sDecDigits + g1sRealSuffix + `$`),
	"char":            regexp.MustCompile(`^'(?:[^'\\` + g1sNewLines + `]|` + g1sEscape + `)'$`),
	"string.regular":  regexp.MustCompile(`^"(?:[^"\\` + g1sNewLines + `]|` + g1sEscape + `)*"(?:[uU]8)?$`),
	"string.verbatim": regexp.MustCompile(`^@"(?:[^"]|"")*"(?:[uU]8)?$`),
	"ident.escaped":   regexp.MustCompile(`^@` + g1sIdentStart + g1sIdentPart + `*$`),
	"ident.basic":     regexp.MustCompile(`^` + g1sIdentStart + g1sIdentPart + `*$`),
}

// g1sFormOf returns the first lexical form (in a fixed order) the whole token text matches, or "".
func g1sFormsOf(text string) []string {
	var out []string
	for _, f := range g1sFormOrder {
		if g1sForms[f].MatchString(text) {
			out = append(out, f)
		}
	}
	return out
}

var g1sFormOrder = []string{"int.dec", "int.hex", "int.bin", "real.1", "real.2", "real.3", "real.4", "char", "string.regular", "string.verbatim", "ident.escaped", "ident.basic"}

// g1sEscapeSpan is one escape sequence inside a regular string or character literal.
type g1sEscapeSpan struct {
	Start, End int    // byte offsets in the token text
	Form       string // "simple:n", "hex", "unicode4", "unicode8"
}

// g1sEscapes lists the escape sequences of a regular string or character literal token text by the
// maximal-munch reading of 6.4.5.5: \x takes one to four hex digits, \u four, \U eight.
func g1sEscapes(text string) []g1sEscapeSpan {
	var out []g1sEscapeSpan
	isHex := func(b byte) bool { return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F' }
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' || i+1 >= len(text) {
			continue
		}
		c, end, form := text[i+1], i+2, ""
		switch {
		case strings.IndexByte(`'"\0abefnrtv`, c) >= 0:
			form = "simple:" + string(c)
		case c == 'x':
			for end < len(text) && end < i+6 && isHex(text[end]) {
				end++
			}
			if end > i+2 {
				form = "hex"
			}
		case c == 'u' || c == 'U':
			n := 4
			form = "unicode4"
			if c == 'U' {
				n, form = 8, "unicode8"
			}
			for end < len(text) && end < i+2+n && isHex(text[end]) {
				end++
			}
			if end != i+2+n {
				form = ""
			}
		}
		if form != "" {
			out = append(out, g1sEscapeSpan{i, end, form})
			i = end - 1
		}
	}
	return out
}

// g1sTriviaItem is a comment found in the text between two tokens.
type g1sTriviaItem struct {
	Start, End int
	Kind       string // "line-comment" or "delimited-comment"
}

// g1sComments reads the comments of src[from:to] (the text between two tokens) by 6.3.3: a single-line
// comment runs to the next New_Line_Character, a delimited comment to the next "*/". opaque lists
// byte ranges the caller knows are not ordinary text (directive lines and disabled sections).
func g1sComments(src []byte, from, to int, opaque [][2]int) []g1sTriviaItem {
	var out []g1sTriviaItem
	inOpaque := func(p int) (int, bool) {
		for _, r := range opaque {
			if p >= r[0] && p < r[1] {
				return r[1], true
			}
		}
		return 0, false
	}
	for i := from; i < to; {
		if end, ok := inOpaque(i); ok {
			i = end
			continue
		}
		switch {
		case i+1 < to && src[i] == '/' && src[i+1] == '/':
			j := i + 2
			for j < to && !g1sIsNewLineAt(src, j) {
				j++
			}
			out = append(out, g1sTriviaItem{i, j, "line-comment"})
			i = j
		case i+1 < to && src[i] == '/' && src[i+1] == '*':
			j := i + 2
			for j+1 < to && !(src[j] == '*' && src[j+1] == '/') {
				j++
			}
			if j+1 >= to {
				return out // unterminated: not a comment of the specification
			}
			out = append(out, g1sTriviaItem{i, j + 2, "delimited-comment"})
			i = j + 2
		default:
			i++
		}
	}
	return out
}

// g1sIsNewLineAt reports whether a New_Line_Character (CR, LF, NEL, LS, PS) starts at p.
func g1sIsNewLineAt(src []byte, p int) bool {
	switch src[p] {
	case '\r', '\n':
		return true
	case 0xC2:
		return p+1 < len(src) && src[p+1] == 0x85
	case 0xE2:
		return p+2 < len(src) && src[p+1] == 0x80 && (src[p+2] == 0xA8 || src[p+2] == 0xA9)
	}
	return false
}

var g1sDirectiveRe = regexp.MustCompile(`^[\p{Zs}\t\x0B\x0C]*#[\p{Zs}\t\x0B\x0C]*(define|undef|if|elif|else|endif|error|warning|region|endregion|line|nullable|pragma)(?:[\p{Zs}\t\x0B\x0C]|//|$)`)

// g1sDirectiveKind names the directive of a source line by 6.5.1 (PP_Start PP_Kind), or "".
func g1sDirectiveKind(line string) string {
	m := g1sDirectiveRe.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	return m[1]
}

var g1sWS = `[\p{Zs}\t\x0B\x0C]`

var (
	g1sSymbol   = regexp.MustCompile(`(?:^|[^A-Za-z0-9_@\\])(?:[\p{L}\p{Nl}_][\p{L}\p{Nl}\p{Nd}\p{Pc}\p{Mn}\p{Mc}\p{Cf}]*)`)
	g1sDefine   = regexp.MustCompile(`^` + g1sWS + `*#` + g1sWS + `*(?:define|undef)` + g1sWS + `+([\p{L}\p{Nl}_][\p{L}\p{Nl}\p{Nd}\p{Pc}\p{Mn}\p{Mc}\p{Cf}]*)`)
	g1sCondExpr = regexp.MustCompile(`^` + g1sWS + `*#` + g1sWS + `*(?:if|elif)` + g1sWS + `+(.*?)` + g1sWS + `*(?://.*)?$`)
	g1sMessage  = regexp.MustCompile(`^` + g1sWS + `*#` + g1sWS + `*(?:error|warning|region|endregion)` + g1sWS + `+(\S.*?)` + g1sWS + `*$`)
	g1sLineInd  = regexp.MustCompile(`^` + g1sWS + `*#` + g1sWS + `*line` + g1sWS + `+(.*?)` + g1sWS + `*(?://.*)?$`)
	g1sNullable = regexp.MustCompile(`^` + g1sWS + `*#` + g1sWS + `*nullable` + g1sWS + `+(disable|enable|restore)(?:` + g1sWS + `+(warnings|annotations))?` + g1sWS + `*(?://.*)?$`)
	g1sLineFile = regexp.MustCompile(`^[0-9]+` + g1sWS + `+("[^"\r\n]*")$`)
	g1sLineNum  = regexp.MustCompile(`^[0-9]+$`)
)

// g1sDefineSymbol returns the conditional symbol of a #define or #undef line (6.5.4) and its offset in the line.
func g1sDefineSymbol(line string) (string, int) {
	m := g1sDefine.FindStringSubmatchIndex(line)
	if m == nil {
		return "", 0
	}
	return line[m[2]:m[3]], m[2]
}

// g1sCondForms returns the expression forms (6.5.3) present in the condition of an #if or #elif line.
func g1sCondForms(line string) []string {
	m := g1sCondExpr.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	e := m[1]
	var out []string
	add := func(ok bool, f string) {
		if ok {
			out = append(out, f)
		}
	}
	add(strings.Contains(e, "||"), "or")
	add(strings.Contains(e, "&&"), "and")
	add(strings.Contains(e, "==") || strings.Contains(e, "!="), "equality")
	add(strings.Contains(strings.ReplaceAll(e, "!=", ""), "!"), "not")
	add(strings.Contains(e, "("), "paren")
	words := regexp.MustCompile(`[\p{L}\p{Nl}_][\p{L}\p{Nl}\p{Nd}\p{Pc}\p{Mn}\p{Mc}\p{Cf}]*`).FindAllString(e, -1)
	var sym, tr, fa bool
	for _, w := range words {
		switch w {
		case "true":
			tr = true
		case "false":
			fa = true
		default:
			sym = true
		}
	}
	add(tr, "true")
	add(fa, "false")
	add(sym, "symbol")
	return out
}

// g1sMessageSpan returns the byte span of the message of an #error, #warning, #region or #endregion line.
func g1sMessageSpan(line string) (int, int, bool) {
	m := g1sMessage.FindStringSubmatchIndex(line)
	if m == nil {
		return 0, 0, false
	}
	return m[2], m[3], true
}

// g1sLineForm names the line indicator of a #line directive (6.5.8): "number-file", "number", "default" or "hidden"; the file name span is returned for the first.
func g1sLineForm(line string) (form string, fileStart, fileEnd int) {
	m := g1sLineInd.FindStringSubmatchIndex(line)
	if m == nil {
		return "", 0, 0
	}
	ind := line[m[2]:m[3]]
	switch {
	case ind == "default" || ind == "hidden":
		return ind, 0, 0
	case g1sLineNum.MatchString(ind):
		return "number", 0, 0
	case g1sLineFile.MatchString(ind):
		f := g1sLineFile.FindStringSubmatchIndex(ind)
		return "number-file", m[2] + f[2], m[2] + f[3]
	}
	return "", 0, 0
}

// g1sNullableParts returns the action and the optional target of a #nullable line (6.5.9) with their offsets.
func g1sNullableParts(line string) (action string, actionAt int, target string, targetAt int) {
	m := g1sNullable.FindStringSubmatchIndex(line)
	if m == nil {
		return "", 0, "", 0
	}
	action, actionAt = line[m[2]:m[3]], m[2]
	if m[4] >= 0 {
		target, targetAt = line[m[4]:m[5]], m[4]
	}
	return
}

// g1sValidUTF8 reports whether b is well-formed UTF-8 (the sources the expectation reads are).
func g1sValidUTF8(b []byte) bool { return utf8.Valid(b) }
