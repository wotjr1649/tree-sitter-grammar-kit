package kit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// #218: on every recorded case the evaluator agrees with Roslyn 5.9's own preprocessor by
// ppRoslynDiff. The observations come from src/dev/csharp-pp-oracle over cases.json.
func TestCSharpPreprocessRoslyn(t *testing.T) {
	dir := filepath.Join("..", "testdata", "csharp-pp")
	if ok, unobserved := ppCompareBatch(t, filepath.Join(dir, "cases.json"), filepath.Join(dir, "roslyn-observed.json")); ok < 30 || unobserved != nil {
		t.Fatalf("only %d cases compared; no observation for %v", ok, unobserved)
	}
}

// #218 Roslyn differential (local development check, never in CI): compares the evaluator with
// Roslyn over the generated batch-*.json and their observed-*.json in TSGK_PP_DIFF_DIR. See
// src/dev/csharp-pp-diff.
func TestCSharpPreprocessDifferential(t *testing.T) {
	dir := os.Getenv("TSGK_PP_DIFF_DIR")
	if dir == "" {
		t.Skip("TSGK_PP_DIFF_DIR is not set")
	}
	batches, err := filepath.Glob(filepath.Join(dir, "batch-*.json"))
	if err != nil || len(batches) == 0 {
		t.Fatalf("no batches in %s: %v", dir, err)
	}
	total, unobserved := 0, []string(nil)
	for _, b := range batches {
		n, u := ppCompareBatch(t, b, filepath.Join(dir, "observed-"+strings.TrimPrefix(filepath.Base(b), "batch-")))
		total, unobserved = total+n, append(unobserved, u...)
	}
	t.Logf("%d batches, %d cases with a selection compared, %d without a Roslyn observation %v", len(batches), total, len(unobserved), unobserved)
	// a case Roslyn itself failed on has nothing to compare with: it passes only when named in
	// TSGK_PP_DIFF_UNOBSERVED (comma-separated ids), so a lost observation never passes silently
	allowed := strings.Split(os.Getenv("TSGK_PP_DIFF_UNOBSERVED"), ",")
	for _, id := range unobserved {
		if !slices.Contains(allowed, id) {
			t.Errorf("%s: no Roslyn observation", id)
		}
	}
	if total == 0 {
		t.Error("no case with a selection compared")
	}
}

type ppObservedCase struct {
	ID         string `json:"id"`
	Directives []struct {
		Kind      string `json:"kind"`
		Start     int    `json:"start"`
		Active    bool   `json:"active"`
		Taken     *bool  `json:"taken"`
		LineStart int    `json:"line_start"`
		LineEnd   int    `json:"line_end"`
	} `json:"directives"`
	Disabled []struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"disabled"`
	Diagnostics []ppDiagnostic `json:"diagnostics"`
	OracleError string         `json:"oracle_error"` // Roslyn failed: no observation
}

// ppDiagnostic is one Roslyn error: where it starts and what Roslyn attached it to (a directive
// kind without the DirectiveTrivia suffix, EndOfFile, or Other).
type ppDiagnostic struct {
	ID      string `json:"id"`
	Start   int    `json:"start"`
	Context string `json:"context"`
}

// ppCompareBatch checks one cases file against its Roslyn observation and returns how many
// cases Roslyn gives a selection for (the rest must be MALFORMED) and the cases Roslyn failed on.
func ppCompareBatch(t *testing.T, casesPath, observedPath string) (selected int, unobserved []string) {
	t.Helper()
	var cases struct {
		Cases []struct {
			ID      string   `json:"id"`
			Source  string   `json:"source"`
			Symbols []string `json:"symbols"`
		} `json:"cases"`
	}
	var observed struct {
		Schema, Roslyn string
		CasesSHA256    string           `json:"cases_sha256"`
		Cases          []ppObservedCase `json:"cases"`
	}
	digests := map[string]string{}
	for name, v := range map[string]any{casesPath: &cases, observedPath: &observed} {
		data, err := os.ReadFile(name)
		if err != nil || json.Unmarshal(data, v) != nil {
			t.Fatalf("%s: %v", name, err)
		}
		digests[name] = digestHex(data)
	}
	// the observation is of exactly these cases
	if observed.Schema != "tsgk-csharp-pp-roslyn/r2" || observed.Roslyn != "5.9.0.0" || observed.CasesSHA256 != digests[casesPath] || len(observed.Cases) != len(cases.Cases) {
		t.Fatalf("%s: observation %s %s %s %d cases", observedPath, observed.Schema, observed.Roslyn, observed.CasesSHA256, len(observed.Cases))
	}
	for i, c := range cases.Cases {
		o := observed.Cases[i]
		src, err := base64.StdEncoding.DecodeString(c.Source)
		if err != nil || o.ID != c.ID {
			t.Fatalf("case %d: %v %s %s", i, err, o.ID, c.ID)
		}
		if o.OracleError != "" {
			unobserved = append(unobserved, c.ID)
			continue
		}
		r := csharpPreprocess(src, EncodingUTF8, c.Symbols, ppDefaultLimits)
		if d := ppRoslynDiff(src, r, o, ppDefaultLimits); d != "" {
			t.Errorf("%s: %s", c.ID, d)
			continue
		}
		if r.Status == ppRangeLimit { // compare the selection the range cap withheld, too
			lim := ppDefaultLimits
			lim.Ranges = math.MaxInt
			if r = csharpPreprocess(src, EncodingUTF8, c.Symbols, lim); ppRoslynDiff(src, r, o, lim) != "" {
				t.Errorf("%s: without the range cap: %s", c.ID, ppRoslynDiff(src, r, o, lim))
				continue
			}
		}
		if r.Status == ppOK || r.Status == ppZeroActive {
			ppCheckPoints(t, c.ID, src, r)
			selected++
		}
	}
	return selected, unobserved
}

// ppSelectionError names a Roslyn error that voids the section selection: an error in a
// directive that steers or brackets selection (or an unknown directive), a group left open at
// the end of the file, and a directive after code on its line (CS1040). Errors elsewhere
// (ordinary C# syntax, including a '#' in an interpolation hole, where Roslyn reads no
// directive; #error, #line, #pragma, #nullable) leave the selection standing.
func ppSelectionError(o ppObservedCase) string {
	for _, d := range o.Diagnostics {
		switch {
		case d.ID == "CS1040",
			d.Context == "EndOfFile" && (d.ID == "CS1027" || d.ID == "CS1038"):
			return d.ID
		}
		switch d.Context {
		case "If", "Elif", "Else", "EndIf", "Define", "Undef", "Region", "EndRegion", "Bad", "Shebang", "Ignored":
			return d.ID + " in " + d.Context
		}
	}
	return ""
}

// ppRoslynDiff describes how the evaluator result r for src differs from the Roslyn
// observation o, or returns "". The expectation comes from Roslyn alone plus the evaluator's
// contract: with a selection error the result is MALFORMED and claims nothing; otherwise the
// directives (kind, '#' offset, active, branch taken) and the inactive ranges (Roslyn's
// disabled text) match, and the active ranges are the source minus the disabled text and the
// physical lines (Roslyn's line breaks) of every steering (#if, #elif, #else, #endif, #define,
// #undef) or inactive directive; no active range is ZERO_ACTIVE and more than lim.Ranges is
// RANGE_LIMIT.
func ppRoslynDiff(src []byte, r ppResult, o ppObservedCase, lim ppLimits) string {
	none := r.Directives == nil && r.Active == nil && r.Inactive == nil
	if e := ppSelectionError(o); e != "" {
		if r.Status != ppMalformed || !none {
			return fmt.Sprintf("%s (%s) where Roslyn reports %s", r.Status, r.Reason, e)
		}
		return ""
	}
	var drop [][2]int
	for _, s := range o.Disabled {
		drop = append(drop, [2]int{s.Start, s.End})
	}
	for _, d := range o.Directives {
		switch strings.TrimSuffix(d.Kind, "DirectiveTrivia") {
		case "If", "Elif", "Else", "EndIf", "Define", "Undef":
		default:
			if d.Active {
				continue
			}
		}
		drop = append(drop, [2]int{d.LineStart, d.LineEnd})
	}
	slices.SortFunc(drop, func(a, b [2]int) int { return a[0] - b[0] })
	var wantActive []string
	at := 0
	for _, s := range append(drop, [2]int{len(src), len(src)}) {
		if s[0] > at {
			wantActive = append(wantActive, fmt.Sprintf("%d-%d", at, s[0]))
		}
		at = max(at, s[1])
	}
	want := ppOK
	switch {
	case len(wantActive) == 0:
		want = ppZeroActive
	case len(wantActive) > lim.Ranges:
		want = ppRangeLimit
	}
	if r.Status != want {
		return fmt.Sprintf("%s (%s), want %s", r.Status, r.Reason, want)
	}
	if want == ppRangeLimit {
		if !none {
			return "RANGE_LIMIT with a selection"
		}
		return ""
	}
	show := func(spans []Span) (out []string) {
		for _, s := range spans {
			out = append(out, fmt.Sprintf("%d-%d", s.StartByte, s.EndByte))
		}
		return out
	}
	var got, wantDirectives, wantInactive []string
	for _, d := range r.Directives {
		got = append(got, fmt.Sprintf("%s@%d active=%v taken=%s", d.Kind, d.Hash, d.Active, ppShow(d.Taken)))
	}
	for _, d := range o.Directives {
		wantDirectives = append(wantDirectives, fmt.Sprintf("%s@%d active=%v taken=%s", strings.TrimSuffix(d.Kind, "DirectiveTrivia"), d.Start, d.Active, ppShow(d.Taken)))
	}
	for _, s := range o.Disabled {
		wantInactive = append(wantInactive, fmt.Sprintf("%d-%d", s.Start, s.End))
	}
	switch {
	case !slices.Equal(got, wantDirectives):
		return fmt.Sprintf("directives\n got %v\nwant %v", got, wantDirectives)
	case !slices.Equal(show(r.Inactive), wantInactive):
		return fmt.Sprintf("inactive %v, Roslyn disabled text %v", show(r.Inactive), wantInactive)
	case !slices.Equal(show(r.Active), wantActive):
		return fmt.Sprintf("active %v, want %v", show(r.Active), wantActive)
	}
	return ""
}

// #218: ppRoslynDiff notices each kind of wrong result (a comparator that passes everything
// would make the Roslyn comparison vacuous).
func TestPPRoslynDiffDetects(t *testing.T) {
	src := []byte("#if A\nclass X {}\n#else\nclass Y {}\n#endif\nclass Z {}\n#if B\nclass W {}\n#endif\n")
	var o ppObservedCase
	if err := json.Unmarshal([]byte(`{"id":"m","directives":[
		{"kind":"IfDirectiveTrivia","start":0,"active":true,"taken":true,"line_start":0,"line_end":6},
		{"kind":"ElseDirectiveTrivia","start":17,"active":true,"taken":false,"line_start":17,"line_end":23},
		{"kind":"EndIfDirectiveTrivia","start":34,"active":true,"taken":null,"line_start":34,"line_end":41},
		{"kind":"IfDirectiveTrivia","start":52,"active":true,"taken":false,"line_start":52,"line_end":58},
		{"kind":"EndIfDirectiveTrivia","start":69,"active":true,"taken":null,"line_start":69,"line_end":76}],
		"disabled":[{"start":23,"end":34},{"start":58,"end":69}],"diagnostics":[]}`), &o); err != nil {
		t.Fatal(err)
	}
	good := csharpPreprocess(src, EncodingUTF8, []string{"A"}, ppDefaultLimits)
	if d := ppRoslynDiff(src, good, o, ppDefaultLimits); d != "" || good.Status != ppOK {
		t.Fatalf("baseline %s: %s", good.Status, d)
	}
	clone := func() ppResult {
		r := good
		r.Directives, r.Active, r.Inactive = slices.Clone(good.Directives), slices.Clone(good.Active), slices.Clone(good.Inactive)
		return r
	}
	mutants := map[string]func(*ppResult){
		"branch flipped":   func(r *ppResult) { r.Directives[0].Taken = ppBool(false) },
		"directive moved":  func(r *ppResult) { r.Directives[1].Hash++ },
		"inactive shifted": func(r *ppResult) { r.Inactive[0].StartByte++ },
		"active dropped":   func(r *ppResult) { r.Active = r.Active[:len(r.Active)-1] },
		"active cut":       func(r *ppResult) { r.Active[0].EndByte-- },
		"zero active":      func(r *ppResult) { r.Status = ppZeroActive },
		"range limit":      func(r *ppResult) { *r = ppResult{Status: ppRangeLimit} },
		"malformed":        func(r *ppResult) { *r = ppResult{Status: ppMalformed} },
	}
	for name, m := range mutants {
		r := clone()
		m(&r)
		if ppRoslynDiff(src, r, o, ppDefaultLimits) == "" {
			t.Errorf("%s: not noticed", name)
		}
	}
	// a range cap the selection exceeds requires RANGE_LIMIT, and a selection error MALFORMED exactly
	if d := ppRoslynDiff(src, good, o, ppLimits{Ranges: 1}); d == "" {
		t.Error("OK over the range cap not noticed")
	}
	if ppRoslynDiff(src, ppResult{Status: ppRangeLimit, Active: good.Active}, o, ppLimits{Ranges: 1}) == "" {
		t.Error("RANGE_LIMIT with a selection not noticed")
	}
	// selection errors Roslyn attaches outside a directive, and errors that leave the selection
	for _, e := range []ppDiagnostic{{ID: "CS1040", Context: "Other"}, {ID: "CS1027", Context: "EndOfFile"}, {ID: "CS1038", Context: "EndOfFile"}} {
		oe := o
		oe.Diagnostics = []ppDiagnostic{e}
		if ppRoslynDiff(src, good, oe, ppDefaultLimits) == "" {
			t.Errorf("%s in %s not taken as a selection error", e.ID, e.Context)
		}
	}
	for _, e := range []ppDiagnostic{{ID: "CS1576", Context: "Line"}, {ID: "CS1029", Context: "Error"}, {ID: "CS1056", Context: "Other"}, {ID: "CS1513", Context: "EndOfFile"}} {
		oe := o
		oe.Diagnostics = []ppDiagnostic{e}
		if d := ppRoslynDiff(src, good, oe, ppDefaultLimits); d != "" {
			t.Errorf("%s in %s voided the selection: %s", e.ID, e.Context, d)
		}
	}
	// a skipped directive that steers nothing still leaves the active ranges with its line
	skipped := []byte("#if B\n#pragma warning disable\nclass Y {}\n#endif\n")
	var so ppObservedCase
	if err := json.Unmarshal([]byte(`{"id":"s","directives":[
		{"kind":"IfDirectiveTrivia","start":0,"active":true,"taken":false,"line_start":0,"line_end":6},
		{"kind":"PragmaWarningDirectiveTrivia","start":6,"active":false,"taken":null,"line_start":6,"line_end":30},
		{"kind":"EndIfDirectiveTrivia","start":41,"active":true,"taken":null,"line_start":41,"line_end":48}],
		"disabled":[{"start":30,"end":41}],"diagnostics":[]}`), &so); err != nil {
		t.Fatal(err)
	}
	if r := csharpPreprocess(skipped, EncodingUTF8, nil, ppDefaultLimits); r.Status != ppZeroActive || ppRoslynDiff(skipped, r, so, ppDefaultLimits) != "" {
		t.Errorf("skipped #pragma: %s %s", r.Status, ppRoslynDiff(skipped, r, so, ppDefaultLimits))
	}
	if ppRoslynDiff(skipped, ppResult{Status: ppOK, Directives: csharpPreprocess(skipped, EncodingUTF8, nil, ppDefaultLimits).Directives, Active: []Span{{StartByte: 6, EndByte: 30}}, Inactive: []Span{{StartByte: 30, EndByte: 41}}}, so, ppDefaultLimits) == "" {
		t.Error("a skipped #pragma line left active not noticed")
	}
	o.Diagnostics = append(o.Diagnostics, ppDiagnostic{ID: "CS1517", Context: "If"})
	for _, r := range []ppResult{good, {Status: ppInvalidInput}, {Status: ppLimit}, {Status: ppMalformed, Active: good.Active}} {
		if ppRoslynDiff(src, r, o, ppDefaultLimits) == "" {
			t.Errorf("%s with a selection error not noticed", r.Status)
		}
	}
	if d := ppRoslynDiff(src, ppResult{Status: ppMalformed}, o, ppDefaultLimits); d != "" {
		t.Errorf("MALFORMED rejected: %s", d)
	}
}

// ppCheckPoints compares every span's points with a count from the start of the source:
// rows on LF, byte columns.
func ppCheckPoints(t *testing.T, id string, src []byte, r ppResult) {
	t.Helper()
	at := func(off uint32) Point {
		var p Point
		for _, c := range src[:off] {
			if c == '\n' {
				p = Point{Row: p.Row + 1}
			} else {
				p.Column++
			}
		}
		return p
	}
	for _, s := range append(slices.Clone(r.Active), r.Inactive...) {
		if s.StartPoint != at(s.StartByte) || s.EndPoint != at(s.EndByte) {
			t.Errorf("%s: span %d-%d points %v-%v", id, s.StartByte, s.EndByte, s.StartPoint, s.EndPoint)
		}
	}
}

func ppShow(b *bool) string {
	if b == nil {
		return "-"
	}
	return fmt.Sprint(*b)
}

// #218: what the parser reads, and the results that claim no selection.
func TestCSharpPreprocessResults(t *testing.T) {
	ranges := func(r ppResult) string {
		var out []string
		for _, s := range r.Active {
			out = append(out, fmt.Sprintf("%d-%d", s.StartByte, s.EndByte))
		}
		return r.Status + " " + strings.Join(out, ",")
	}
	for name, c := range map[string]struct {
		src     string
		symbols []string
		lim     *ppLimits
		want    string
	}{
		// the #if/#else/#endif lines and the skipped section leave; #region stays for the grammar
		"selected":             {"#region r\n#if A\nclass X {}\n#else\nclass Y {}\n#endif\nclass Z {}\n#endregion\n", []string{"A"}, nil, "OK 0-10,16-27,51-73"},
		"long-symbol":          {"#if " + strings.Repeat("A", 129) + "\nx\n#endif\n", nil, nil, "LIMIT "},
		"long-define":          {"#define " + strings.Repeat("A", 129) + "\nx\n", nil, nil, "LIMIT "},
		"escaped-long":         {"#if " + strings.Repeat("\\u0041", 22) + "\nx\n#endif\n", []string{strings.Repeat("A", 22)}, nil, "LIMIT "},
		"long-constant":        {"#if TRUE" + strings.Repeat("\\u200C", 21) + "\nclass Selected {}\n#else\nclass Other {}\n#endif\n", nil, nil, "LIMIT "},
		"long-caller":          {"x\n", []string{strings.Repeat("A", 129)}, nil, "LIMIT "},
		"at-128":               {"#if " + strings.Repeat("A", 128) + "\nx\n#endif\n", []string{strings.Repeat("A", 128)}, nil, "OK 133-135"},
		"define-leaves":        {"#define A\nclass X {}\n", nil, nil, "OK 10-21"},
		"zero-active":          {"#if A\nclass X {}\n#endif\n", nil, nil, "ZERO_ACTIVE "},
		"cap":                  {strings.Repeat("#if A\n#endif\nx\n", 3), nil, &ppLimits{Bytes: 1 << 20, Directives: 64, Depth: 8, ExprDepth: 8, Ranges: 2}, "RANGE_LIMIT "},
		"at-cap":               {strings.Repeat("#if A\n#endif\nx\n", 2), nil, &ppLimits{Bytes: 1 << 20, Directives: 64, Depth: 8, ExprDepth: 8, Ranges: 2}, "OK 13-15,28-30"},
		"depth":                {strings.Repeat("#if A\n", 3) + strings.Repeat("#endif\n", 3), nil, &ppLimits{Bytes: 1 << 20, Directives: 64, Depth: 2, ExprDepth: 8, Ranges: 16}, "LIMIT "},
		"expr-depth":           {"#if " + strings.Repeat("!", 9) + "A\nx\n#endif\n", nil, &ppLimits{Bytes: 1 << 20, Directives: 64, Depth: 8, ExprDepth: 8, Ranges: 16}, "LIMIT "},
		"directives":           {strings.Repeat("#region\n", 3), nil, &ppLimits{Bytes: 1 << 20, Directives: 2, Depth: 8, ExprDepth: 8, Ranges: 16}, "LIMIT "},
		"bytes":                {"class X {}\n", nil, &ppLimits{Bytes: 4, Directives: 8, Depth: 8, ExprDepth: 8, Ranges: 16}, "LIMIT "},
		"define-feff-long":     {"#define A" + strings.Repeat("\uFEFF", 128) + "\n#if A\nx\n#endif\n", nil, nil, "LIMIT "},
		"define-feff-at-128":   {"#define A" + strings.Repeat("\uFEFF", 127) + "\n#if A\nx\n#endif\n", nil, nil, "OK 397-399"},
		"feff-long":            {"#if A" + strings.Repeat("\uFEFF", 128) + "\nx\n#endif\n", []string{"A"}, nil, "LIMIT "}, // U+FEFF continues the name
		"lexer-depth":          {"class X { string s = $\"{$\"{$\"{1}\"}\"}\"; }\n", nil, &ppLimits{Bytes: 1 << 20, Directives: 8, Depth: 3, ExprDepth: 8, Ranges: 16, Symbols: 4}, "LIMIT "},
		"lexer-at-depth":       {"class X { string s = $\"{$\"{1}\"}\"; }\n", nil, &ppLimits{Bytes: 1 << 20, Directives: 8, Depth: 4, ExprDepth: 8, Ranges: 16, Symbols: 4}, "OK 0-36"},
		"lexer-depth-eof":      {"$\"{", nil, &ppLimits{Bytes: 1 << 20, Directives: 8, Depth: 1, ExprDepth: 8, Ranges: 16, Symbols: 4}, "LIMIT "},
		"lexer-depth-line-end": {"$\"{\nx\n", nil, &ppLimits{Bytes: 1 << 20, Directives: 8, Depth: 1, ExprDepth: 8, Ranges: 16, Symbols: 4}, "LIMIT "},
		"symbol-count":         {"class X {}\n", []string{"A", "B", "C"}, &ppLimits{Bytes: 1 << 20, Directives: 8, Depth: 8, ExprDepth: 8, Ranges: 16, Symbols: 2}, "LIMIT "},
		"symbol-bytes":         {"class X {}\n", []string{"ABCDEF"}, &ppLimits{Bytes: 14, Directives: 8, Depth: 8, ExprDepth: 8, Ranges: 16, Symbols: 2}, "LIMIT "},
		"invalid-utf8":         {"class X { string s = \"\xff\"; }\n", nil, nil, "INVALID_INPUT "},
		"bad-symbol":           {"class X {}\n", []string{"@A"}, nil, "INVALID_INPUT "},
		"keyword":              {"class X {}\n", []string{"true"}, nil, "INVALID_INPUT "},
		"unknown":              {"#ifdef A\n#endif\n", nil, nil, "MALFORMED "},
		"if-paren":             {"#if(A)\nclass X {}\n#endif\nx\n", []string{"A"}, nil, "OK 7-18,25-27"},
	} {
		lim := ppDefaultLimits
		if c.lim != nil {
			lim = *c.lim
		}
		if got := ranges(csharpPreprocess([]byte(c.src), EncodingUTF8, c.symbols, lim)); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
	// a long $ or @ run without a quote is read once (it was rescanned per byte: quadratic)
	for _, c := range []string{"$", "@"} {
		start := time.Now()
		r := csharpPreprocess([]byte("x "+strings.Repeat(c, 400000)+"\n"), EncodingUTF8, nil, ppDefaultLimits)
		if el := time.Since(start); r.Status != ppOK || el > 5*time.Second {
			t.Errorf("%s run: %s in %v", c, r.Status, el)
		}
	}
	if r := csharpPreprocess([]byte("class X {}\n"), EncodingUTF16LE, nil, ppDefaultLimits); r.Status != ppUnsupportedEncoding || r.Active != nil {
		t.Errorf("UTF-16: %+v", r)
	}
	// a lone CR or a Unicode line separator ends a C# line but not a tree-sitter row
	for src, want := range map[string]bool{"x\ny\r\n": false, "x\ry\n": true, "x y\n": true, "x\u0085y\n": true} {
		if r := csharpPreprocess([]byte(src), EncodingUTF8, nil, ppDefaultLimits); r.NonLF != want {
			t.Errorf("%q: NonLF %v", src, r.NonLF)
		}
	}
}
