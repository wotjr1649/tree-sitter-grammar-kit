package kit

import (
	"slices"
	"testing"
)

// TestCSharpG1CrossCheckFindsDisagreements feeds the cross-check tokens and directives that disagree
// with the specification derivation and requires each kind of disagreement to be reported, and an
// agreeing token and an unaccepted source to be left alone.
func TestCSharpG1CrossCheckFindsDisagreements(t *testing.T) {
	a := g1ExpLoad(t)
	_, c, _, err := g1ExpContext(a.in)
	if err != nil {
		t.Fatal(err)
	}
	c.keywords = map[string]bool{"class": true}
	c.puncts = map[string]bool{"+": true}
	tok := func(id, text, kind string, accepted bool) *g1Src {
		data := []byte(text)
		return &g1Src{id: id, accepted: accepted, data: data, toks: []g1Tok{{kind: kind, start: 0, end: len(data), text: text}}}
	}
	dir := func(id, text, kind string) *g1Src {
		data := []byte(text + "\n")
		return &g1Src{id: id, accepted: true, data: data, dirs: []G1RoslynDirective{{Kind: kind, Start: 0, LineStart: 0, LineEnd: len(data)}}}
	}
	c.order = []*g1Src{
		tok("keyword", "class", "IdentifierToken", true),
		tok("punctuator", "+", "MinusToken", true),
		tok("numeric", "abc", "NumericLiteralToken", true),
		tok("character", "'ab'", "CharacterLiteralToken", true),
		tok("string", `"a`, "StringLiteralToken", true),
		tok("identifier", "1a", "IdentifierToken", true),
		tok("numeric-kind", "0x1F", "OpenBraceToken", true),
		tok("character-kind", "'a'", "OpenBraceToken", true),
		tok("string-kind", `"a"`, "OpenBraceToken", true),
		tok("identifier-kind", "foo", "OpenParenToken", true),
		tok("raw", `"""a"""`, "SingleLineRawStringLiteralToken", true),
		dir("directive-kind", "#if X", "DefineDirectiveTrivia"),
		dir("directive-beyond", "#!x", "ShebangDirectiveTrivia"),
		// agreeing, and ignored
		tok("agree-keyword", "class", "ClassKeyword", true),
		tok("agree-punctuator", "+", "PlusToken", true),
		tok("agree-identifier", "x", "IdentifierToken", true),
		dir("agree-directive", "#if X", "IfDirectiveTrivia"),
		tok("unaccepted", "class", "IdentifierToken", false),
	}
	got, err := g1CrossCheck(c)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, m := range got.Mismatches {
		codes = append(codes, m.Code)
		if m.Count != 1 || (m.Code != "TOKEN_OF_OTHER_ROW") != (m.Disposition == "UNDISPOSED") {
			t.Errorf("mismatch %+v", m)
		}
	}
	slices.Sort(codes)
	want := []string{"CHARACTER_FORM", "CHARACTER_KIND", "DIRECTIVE_BEYOND_SPEC", "DIRECTIVE_KIND", "IDENTIFIER_FORM", "IDENTIFIER_KIND", "KEYWORD_KIND", "NUMERIC_FORM", "NUMERIC_KIND", "PUNCTUATOR_KIND", "STRING_FORM", "STRING_KIND", "TOKEN_OF_OTHER_ROW"}
	if !slices.Equal(codes, want) {
		t.Errorf("codes %v\nwant  %v", codes, want)
	}
	if got.Sources != 17 || got.Directives != 3 {
		t.Errorf("sources %d directives %d", got.Sources, got.Directives)
	}
}

// TestCSharpG1SpecLineTerminators: a single-line comment ends at any New_Line_Character (CR, LF,
// NEL, LS, PS) of 6.3.2, not only at LF.
func TestCSharpG1SpecLineTerminators(t *testing.T) {
	for name, nl := range map[string]string{"LF": "\n", "CR": "\r", "NEL": "\u0085", "LS": " ", "PS": " "} {
		src := []byte("// c" + nl + "/* d */")
		cs := g1sComments(src, 0, len(src), nil)
		if len(cs) != 2 || cs[0].Kind != "line-comment" || string(src[cs[0].Start:cs[0].End]) != "// c" || cs[1].Kind != "delimited-comment" {
			t.Errorf("%s: %+v", name, cs)
		}
	}
	// an unterminated delimited comment is no comment of the specification
	src := []byte("/* open")
	if cs := g1sComments(src, 0, len(src), nil); len(cs) != 0 {
		t.Errorf("unterminated comment: %+v", cs)
	}
}

// TestCSharpG1ExpectedRequiresDispositions: a disagreement without an authored disposition makes the
// asset unsealable even when the asset equals the derivation byte for byte.
func TestCSharpG1ExpectedRequiresDispositions(t *testing.T) {
	a := g1ExpLoad(t)
	const key = "TOKEN_OF_OTHER_ROW/SingleLineRawStringLiteralToken"
	old := csharpG1Dispositions[key]
	delete(csharpG1Dispositions, key)
	defer func() { csharpG1Dispositions[key] = old }()
	in := a.in
	in.Sealed = a.x.Metamorphic.Results()
	want, err := DeriveCSharpG1Expected(in)
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalCSharpG1Expected(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckCSharpG1Expected(data, want, want); g1Code(err) != "G1_EXPECTED_UNDISPOSED" {
		t.Errorf("undisposed mismatch: %v", err)
	}
}

// TestCSharpG1LedgerSealedSetIsChecked: the ledger takes its SEALED statuses from the expected asset
// and refuses an asset that names an alternative the ledger lacks or one of another row.
func TestCSharpG1LedgerSealedSetIsChecked(t *testing.T) {
	t.Parallel()
	a := g1ExpLoad(t)
	src := a.in.Src
	src.Executed = nil
	for name, doc := range map[string]string{
		"unknown alternative": `{"alternatives":[{"id":"csharp-B01.a999","row":"csharp-B01","status":"EXPECTED"}]}`,
		"other row":           `{"alternatives":[{"id":"csharp-B03.a001","row":"csharp-B03","status":"EXPECTED"}]}`,
		"no alternatives":     `{"alternatives":[]}`,
		"not json":            `x`,
	} {
		src.Expected = []byte(doc)
		if _, err := DeriveCSharpG1Ledger(src); g1Code(err) != "G1_LEDGER_SEALED" {
			t.Errorf("%s: %v", name, err)
		}
	}
}
