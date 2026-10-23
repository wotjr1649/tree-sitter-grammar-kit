package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"testing"
)

func preprocessProfile(schema string) map[string]any {
	p := anchorProfile(schema, []any{anchor("call", 0, 8)}, nil)
	p["symbol"] = "tree_sitter_c_sharp"
	p["preprocess"] = map[string]any{"mode": PreprocessMode, "symbols": []any{"B", "A\u200c", "B"}}
	return p
}

// #218 (218-B): an r3 incremental profile takes the preprocess member for native-parse-edit
// on UTF-8 C# without SVC, keeps the anchors and the caller symbols as given; every other
// combination is unsupported, and invalid symbols are an invalid profile. r3 without the
// member keeps the r2 meaning, and no other revision or document takes it.
func TestPreprocessProfile(t *testing.T) {
	p, e := parseProfileOf(IncrementalPreprocessSchema, preprocessProfile(IncrementalPreprocessSchema))
	if e != nil {
		t.Fatal(e)
	}
	if p.Preprocess == nil || p.Preprocess.Mode != PreprocessMode || !slices.Equal(p.Preprocess.Symbols, []string{"B", "A\u200c", "B"}) || len(p.Cases[0].Expect[0].Anchors) != 1 {
		t.Fatalf("%+v %+v", p.Preprocess, p.Cases[0].Expect[0])
	}
	plain := preprocessProfile(IncrementalPreprocessSchema)
	delete(plain, "preprocess")
	if p, e := parseProfileOf(IncrementalPreprocessSchema, plain); e != nil || p.Preprocess != nil || len(p.Cases[0].Expect[0].Anchors) != 1 {
		t.Fatalf("r3 without preprocess: %v %+v", e, p.Preprocess)
	}
	long := strings.Repeat("A", 129)
	many := []any{}
	for range 257 {
		many = append(many, "A")
	}
	for name, c := range map[string]struct {
		edit func(p map[string]any)
		code string
		kind string
	}{
		"r2": {func(p map[string]any) { p["schema"] = IncrementalSchema }, "PREPROCESS_OPERATION_UNSUPPORTED", KindUnsupported},
		"batch": {func(p map[string]any) {
			c := p["cases"].([]any)[0].(map[string]any)
			p["operation"], p["output"], c["edits"], c["expect"] = "private-corpus-local", OutputRecord, []any{}, c["expect"].([]any)[:1]
		}, "PREPROCESS_OPERATION_UNSUPPORTED", KindUnsupported},
		"svc":          {func(p map[string]any) { p["format"] = SvcLegacyFormat }, "PREPROCESS_SVC_UNSUPPORTED", KindUnsupported},
		"language":     {func(p map[string]any) { p["symbol"] = "tree_sitter_fx" }, "PREPROCESS_LANGUAGE_UNSUPPORTED", KindUnsupported},
		"encoding":     {func(p map[string]any) { p["cases"].([]any)[0].(map[string]any)["encoding"] = EncodingUTF16LE }, "PREPROCESS_ENCODING_UNSUPPORTED", KindUnsupported},
		"mode":         {func(p map[string]any) { p["preprocess"].(map[string]any)["mode"] = "csharp-symbols/r2" }, "PREPROCESS_MODE_UNSUPPORTED", KindUnsupported},
		"bad-symbol":   {func(p map[string]any) { p["preprocess"].(map[string]any)["symbols"] = []any{"1A"} }, "PREPROCESS_SYMBOL_INVALID", KindInvalidInput},
		"keyword":      {func(p map[string]any) { p["preprocess"].(map[string]any)["symbols"] = []any{"true"} }, "PREPROCESS_SYMBOL_INVALID", KindInvalidInput},
		"long-symbol":  {func(p map[string]any) { p["preprocess"].(map[string]any)["symbols"] = []any{long} }, "PREPROCESS_SYMBOLS_LIMIT", KindInvalidInput},
		"many-symbols": {func(p map[string]any) { p["preprocess"].(map[string]any)["symbols"] = many }, "PREPROCESS_SYMBOLS_LIMIT", KindInvalidInput},
		"extra-member": {func(p map[string]any) { p["preprocess"].(map[string]any)["limits"] = nil }, "JSON_UNKNOWN_FIELD", KindInvalidInput},
	} {
		p := preprocessProfile(IncrementalPreprocessSchema)
		c.edit(p)
		if _, e := parseProfileOf(p["schema"].(string), p); e == nil || e.Code != c.code || e.Kind != c.kind {
			t.Errorf("%s: %v, want %s", name, e, c.code)
		}
	}
	o := anchorProfile(OracleSchema, nil, nil)
	o["symbol"], o["preprocess"] = "tree_sitter_c_sharp", map[string]any{"mode": PreprocessMode, "symbols": []any{}}
	if _, e := parseProfileOf(OracleSchema, o); e == nil || e.Code != "PREPROCESS_OPERATION_UNSUPPORTED" {
		t.Errorf("oracle profile: %v", e)
	}
	// qualification compares oracle profiles; a preprocess member never reaches a claim
	if d := registrationDiff(OracleProfile{Native: IncrementalProfile{Preprocess: &Preprocess{Mode: PreprocessMode}}}, QualWorkload{}); d != "preprocess" {
		t.Errorf("registration diff %q", d)
	}
}

// The symbols digest is fixed: sorted by bytes, deduplicated, escapes and formatting
// characters kept, the empty list its prefix alone.
func TestPreprocessSymbolsDigest(t *testing.T) {
	sum := func(s string) string {
		h := sha256.Sum256([]byte(s))
		return hex.EncodeToString(h[:])
	}
	for _, c := range []struct {
		symbols []string
		text    string
	}{
		{nil, "tsgk-pp-symbols/r1\n"},
		{[]string{"B", "A", "B"}, "tsgk-pp-symbols/r1\n1:A\n1:B\n"},
		{[]string{"A\u200c", "\\u0041"}, "tsgk-pp-symbols/r1\n4:A\u200c\n6:\\u0041\n"},
	} {
		if got := (Preprocess{Symbols: c.symbols}).SymbolsDigest(); got != sum(c.text) {
			t.Errorf("%q: %s", c.symbols, got)
		}
	}
}

// The observation export runs the evaluator with its fixed limits and owns its slices.
func TestObserveCSharpPreprocess(t *testing.T) {
	src := []byte("#if A\nclass X {}\n#else\nclass Y {}\n#endif\n")
	o := ObserveCSharpPreprocess(src, []string{"A"})
	if o.Status != PreprocessOK || len(o.Directives) != 3 || len(o.Active) != 1 || o.Active[0].StartByte != 6 || o.Active[0].EndByte != 17 || len(o.Inactive) != 1 {
		t.Fatalf("%+v", o)
	}
	if z := ObserveCSharpPreprocess(src[:6], nil); z.Status != PreprocessMalformed || z.Directives == nil || z.Active == nil || z.Inactive == nil {
		t.Fatalf("%+v", z)
	}
	if steps := PreprocessSteps([][]byte{src, []byte("#if\n"), src}, &Preprocess{Symbols: []string{"A"}}); len(steps) != 2 || steps[1].Status != PreprocessMalformed {
		t.Fatalf("steps stop at the first refusal: %+v", steps)
	}
	for status, want := range map[string][2]string{
		PreprocessZeroActive: {StatusNotRun, "PREPROCESS_ZERO_ACTIVE"}, PreprocessMalformed: {StatusNotRun, "PREPROCESS_MALFORMED"},
		PreprocessInvalidInput: {StatusNotRun, "PREPROCESS_INVALID_INPUT"}, PreprocessRangeLimit: {StatusResourceLimit, "PREPROCESS_RANGE_LIMIT"},
		PreprocessLimit: {StatusResourceLimit, "PREPROCESS_LIMIT"},
	} {
		if e, c := PreprocessOutcome(status); e != want[0] || c != want[1] {
			t.Errorf("%s: %s %s", status, e, c)
		}
	}
}
