package kit

import "regexp"

// The instance rules of the position audit (#233, G1AUDIT76).
//
// A row of the audit lists the extent of every instance of its form in the source text; the form is free text. The rule of
// a row is the form as a pattern of the text of one instance: the verifier requires that the text of each listed instance
// (text[lo:hi]) matches the rule in full (and does not match Not). A list that names a place where the form does not occur
// (an instance moved to another token) fails here, whatever the witness of r2 says. The rules are authored with the
// instances, from the specification symbols the row names; they are held in this table and not in the asset, so the
// bytes of the sealed audit asset stay as they were. A rule decides about the instances that are listed; it does not find
// an instance that was left out (that would take a lexer, which the audit is not).

const (
	g1ruleID = `[A-Za-z_]\w*`
	// an element of the Mid and Format tokens of a regular and of a verbatim interpolated string (6.4.5.2): a character that is no
	// quote, backslash or brace, a doubled brace, an escape; in a verbatim string a doubled quote, no escape
	g1ruleRegElem = `(?:[^"\\{}]|\{\{|\}\}|\\(?:['"\\0abfnrtv]|x[0-9A-Fa-f]{1,4}|u[0-9A-Fa-f]{4}))`
	g1ruleVerElem = `(?:[^"{}]|""|\{\{|\}\})`
	g1rulePtr     = g1ruleID + `\*+`
	g1ruleSimple  = `sbyte|byte|short|ushort|int|uint|long|ulong|char`
)

// g1a76RuleText is the pattern of the text of an instance: it matches in full, and Not (when given) does not.
type g1a76RuleText struct {
	Match string
	Not   string
}

// g1a76RuleTexts holds the rule of each of the 76 rows, by id.
var g1a76RuleTexts = map[string]g1a76RuleText{
	"csharp-B01.a378": {Match: `\$"(?s:.*)"`},
	"csharp-B01.a379": {Match: `(?:\$@|@\$)"(?s:.*)"`},
	"csharp-B01.a380": {Match: `\$"(?s:.*)"`},
	"csharp-B01.a381": {Match: `[^{}]+`},
	"csharp-B01.a382": {Match: `-?[0-9]+`},
	"csharp-B01.a383": {Match: `\$"`},
	"csharp-B01.a384": {Match: g1ruleRegElem + `+`},
	"csharp-B01.a385": {Match: `:` + g1ruleRegElem + `+`},
	"csharp-B01.a386": {Match: `"`},
	"csharp-B01.a387": {Match: `:?` + g1ruleRegElem + `*[^"\\{}]` + g1ruleRegElem + `*`},
	"csharp-B01.a393": {Match: `:?` + g1ruleRegElem + `*[^"\\{}]` + g1ruleRegElem + `*`},
	"csharp-B01.a394": {Match: `(?:\$@|@\$)"(?s:.*)"`},
	"csharp-B01.a395": {Match: `[^{}]+`},
	"csharp-B01.a396": {Match: `\$@"`},
	"csharp-B01.a397": {Match: `@\$"`},
	"csharp-B01.a398": {Match: g1ruleVerElem + `+`},
	"csharp-B01.a399": {Match: `:` + g1ruleVerElem + `+`},
	"csharp-B01.a400": {Match: `"`},
	"csharp-B01.a401": {Match: `:?` + g1ruleVerElem + `*[^"{}]` + g1ruleVerElem + `*`},
	"csharp-B01.a405": {Match: `:?` + g1ruleVerElem + `*[^"{}]` + g1ruleVerElem + `*`},
	"csharp-B02.a008": {Match: g1rulePtr},
	"csharp-B02.a012": {Match: `[A-Za-z_][\w.<>?]*(?:\[,*\])+`},
	"csharp-B02.a016": {Match: `object`},
	"csharp-B02.a017": {Match: `string`},
	"csharp-B02.a019": {Match: `[A-Za-z_][\w.<>?]*(?:\[,*\])+`},
	"csharp-B02.a021": {Match: g1rulePtr},
	"csharp-B02.a028": {Match: `\[,*\]`},
	"csharp-B02.a030": {Match: `\?`},
	"csharp-B02.a032": {Match: g1ruleID + `(?:\.` + g1ruleID + `)*\?`},
	"csharp-B02.a036": {Match: `bool|decimal|float|double|` + g1ruleSimple},
	"csharp-B02.a037": {Match: `\([^()]+\)`},
	"csharp-B02.a038": {Match: `decimal|float|double|` + g1ruleSimple},
	"csharp-B02.a039": {Match: `bool`},
	"csharp-B02.a040": {Match: g1ruleSimple},
	"csharp-B02.a041": {Match: `float|double`},
	"csharp-B02.a042": {Match: `decimal`},
	"csharp-B02.a043": {Match: `sbyte`},
	"csharp-B02.a044": {Match: `byte`},
	"csharp-B02.a045": {Match: `short`},
	"csharp-B02.a046": {Match: `ushort`},
	"csharp-B02.a047": {Match: `int`},
	"csharp-B02.a048": {Match: `uint`},
	"csharp-B02.a049": {Match: `long`},
	"csharp-B02.a050": {Match: `ulong`},
	"csharp-B02.a051": {Match: `char`},
	"csharp-B02.a052": {Match: `float`},
	"csharp-B02.a053": {Match: `double`},
	"csharp-B02.a054": {Match: `\([^(),]+(?:,[^(),]+)+\)`},
	"csharp-B02.a055": {Match: g1ruleID + `(?: ` + g1ruleID + `)?`},
	"csharp-B02.a057": {Match: g1ruleID + `(?:\.` + g1ruleID + `)*\?`},
	"csharp-B02.a058": {Match: `<[^<>]+(?:<[^<>]+>)*>`},
	"csharp-B02.a062": {Match: g1rulePtr},
	"csharp-B02.a064": {Match: `in ` + g1ruleID},
	"csharp-B02.a065": {Match: `ref ` + g1ruleID},
	"csharp-B02.a066": {Match: `out (?:(?:var|` + g1ruleID + `) )?` + g1ruleID},
	"csharp-B02.a067": {Match: g1ruleID + `->` + g1ruleID},
	"csharp-B02.a069": {Match: `\*` + g1ruleID},
	"csharp-B02.a070": {Match: `&` + g1ruleID},
	"csharp-B02.a071": {Match: `where ` + g1ruleID + ` : .+`},
	"csharp-B02.a072": {Match: `[A-Za-z_][\w(), ]*`},
	"csharp-B02.a077": {Match: `class`},
	"csharp-B02.a078": {Match: `struct`},
	"csharp-B02.a083": {Match: `new\(\)`},
	"csharp-B02.a084": {Match: `ref`},
	"csharp-B02.a086": {Match: `ref|out|in`},
	"csharp-B02.a087": {Match: `ref`},
	"csharp-B02.a088": {Match: `out`},
	"csharp-B02.a089": {Match: `in`},
	"csharp-B02.a090": {Match: g1rulePtr, Not: `void\*`},
	"csharp-B02.a091": {Match: `void\*`},
	"csharp-B02.a092": {Match: g1rulePtr, Not: `void\*+`},
	"csharp-B02.a093": {Match: `void\*\*+`},
	"csharp-B02.a094": {Match: `void\*`},
	"csharp-B02.a095": {Match: `\*` + g1ruleID},
	"csharp-B02.a096": {Match: g1ruleID + `->` + g1ruleID},
	"csharp-B02.a098": {Match: `&` + g1ruleID},
}

// g1a76Rule is a compiled rule.
type g1a76Rule struct{ match, not *regexp.Regexp }

// g1a76Rules holds the compiled rules by id.
var g1a76Rules = func() map[string]*g1a76Rule {
	out := map[string]*g1a76Rule{}
	for id, r := range g1a76RuleTexts {
		c := &g1a76Rule{match: regexp.MustCompile(`^(?:` + r.Match + `)$`)}
		if r.Not != "" {
			c.not = regexp.MustCompile(`^(?:` + r.Not + `)$`)
		}
		out[id] = c
	}
	return out
}()

// accepts reports whether the text matches the rule in full. A row with no rule (a nil rule) accepts nothing.
func (r *g1a76Rule) accepts(text string) bool {
	return r != nil && r.match.MatchString(text) && (r.not == nil || !r.not.MatchString(text))
}
