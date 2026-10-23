package kit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// #218: on every recorded case the evaluator agrees with Roslyn 5.9's own preprocessor:
// a case Roslyn reports an error for is never OK, and an OK or ZERO_ACTIVE case has Roslyn's directives
// (kind, '#' offset, active, branch taken) and Roslyn's disabled text as its inactive
// ranges. The observations come from src/dev/csharp-pp-oracle over cases.json.
func TestCSharpPreprocessRoslyn(t *testing.T) {
	dir := filepath.Join("..", "testdata", "csharp-pp")
	var cases struct {
		Cases []struct {
			ID      string   `json:"id"`
			Source  string   `json:"source"`
			Symbols []string `json:"symbols"`
		} `json:"cases"`
	}
	var observed struct {
		Schema, Roslyn string
		CasesSHA256    string `json:"cases_sha256"`
		Cases          []struct {
			ID         string `json:"id"`
			Directives []struct {
				Kind   string `json:"kind"`
				Start  int    `json:"start"`
				Active bool   `json:"active"`
				Taken  *bool  `json:"taken"`
			} `json:"directives"`
			Disabled []struct {
				Start int `json:"start"`
				End   int `json:"end"`
			} `json:"disabled"`
			Errors []string `json:"errors"`
		} `json:"cases"`
	}
	digests := map[string]string{}
	for name, v := range map[string]any{"cases.json": &cases, "roslyn-observed.json": &observed} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || json.Unmarshal(data, v) != nil {
			t.Fatalf("%s: %v", name, err)
		}
		digests[name] = digestHex(data)
	}
	// the observation is of exactly these cases
	if observed.Schema != "tsgk-csharp-pp-roslyn/r1" || observed.Roslyn != "5.9.0.0" || observed.CasesSHA256 != digests["cases.json"] || len(observed.Cases) != len(cases.Cases) {
		t.Fatalf("observation %s %s %s %d cases", observed.Schema, observed.Roslyn, observed.CasesSHA256, len(observed.Cases))
	}
	ok := 0
	for i, c := range cases.Cases {
		o := observed.Cases[i]
		src, err := base64.StdEncoding.DecodeString(c.Source)
		if err != nil || o.ID != c.ID {
			t.Fatalf("case %d: %v %s %s", i, err, o.ID, c.ID)
		}
		r := csharpPreprocess(src, EncodingUTF8, c.Symbols, ppDefaultLimits)
		if len(o.Errors) > 0 { // the preprocessing controls Roslyn reports an error for
			if r.Status == ppOK || r.Status == ppZeroActive || r.Directives != nil || r.Active != nil || r.Inactive != nil {
				t.Errorf("%s: %s with a selection where Roslyn reports %v", c.ID, r.Status, o.Errors)
			}
			continue
		}
		ppCheckPoints(t, c.ID, src, r)
		if r.Status != ppOK && r.Status != ppZeroActive {
			t.Errorf("%s: %s (%s) where Roslyn reports no error", c.ID, r.Status, r.Reason)
			continue
		}
		ok++
		var got, want []string
		for _, d := range r.Directives {
			got = append(got, fmt.Sprintf("%s@%d active=%v taken=%s", d.Kind, d.Hash, d.Active, ppShow(d.Taken)))
		}
		for _, d := range o.Directives {
			want = append(want, fmt.Sprintf("%s@%d active=%v taken=%s", strings.TrimSuffix(d.Kind, "DirectiveTrivia"), d.Start, d.Active, ppShow(d.Taken)))
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s directives:\n got %v\nwant %v", c.ID, got, want)
		}
		got, want = nil, nil
		for _, s := range r.Inactive {
			got = append(got, fmt.Sprintf("%d-%d", s.StartByte, s.EndByte))
		}
		for _, s := range o.Disabled {
			want = append(want, fmt.Sprintf("%d-%d", s.Start, s.End))
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s inactive %v, Roslyn disabled text %v", c.ID, got, want)
		}
	}
	if ok < 30 {
		t.Fatalf("only %d cases compared", ok)
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
		"selected":       {"#region r\n#if A\nclass X {}\n#else\nclass Y {}\n#endif\nclass Z {}\n", []string{"A"}, nil, "OK 0-10,16-27,51-62"},
		"define-leaves":  {"#define A\nclass X {}\n", nil, nil, "OK 10-21"},
		"zero-active":    {"#if A\nclass X {}\n#endif\n", nil, nil, "ZERO_ACTIVE "},
		"cap":            {strings.Repeat("#if A\n#endif\nx\n", 3), nil, &ppLimits{Bytes: 1 << 20, Directives: 64, Depth: 8, ExprDepth: 8, Ranges: 2}, "RANGE_LIMIT "},
		"at-cap":         {strings.Repeat("#if A\n#endif\nx\n", 2), nil, &ppLimits{Bytes: 1 << 20, Directives: 64, Depth: 8, ExprDepth: 8, Ranges: 2}, "OK 13-15,28-30"},
		"depth":          {strings.Repeat("#if A\n", 3) + strings.Repeat("#endif\n", 3), nil, &ppLimits{Bytes: 1 << 20, Directives: 64, Depth: 2, ExprDepth: 8, Ranges: 16}, "LIMIT "},
		"expr-depth":     {"#if " + strings.Repeat("!", 9) + "A\nx\n#endif\n", nil, &ppLimits{Bytes: 1 << 20, Directives: 64, Depth: 8, ExprDepth: 8, Ranges: 16}, "LIMIT "},
		"directives":     {strings.Repeat("#region\n", 3), nil, &ppLimits{Bytes: 1 << 20, Directives: 2, Depth: 8, ExprDepth: 8, Ranges: 16}, "LIMIT "},
		"bytes":          {"class X {}\n", nil, &ppLimits{Bytes: 4, Directives: 8, Depth: 8, ExprDepth: 8, Ranges: 16}, "LIMIT "},
		"lexer-depth":    {"class X { string s = $\"{$\"{$\"{1}\"}\"}\"; }\n", nil, &ppLimits{Bytes: 1 << 20, Directives: 8, Depth: 3, ExprDepth: 8, Ranges: 16, Symbols: 4}, "LIMIT "},
		"lexer-at-depth": {"class X { string s = $\"{$\"{1}\"}\"; }\n", nil, &ppLimits{Bytes: 1 << 20, Directives: 8, Depth: 4, ExprDepth: 8, Ranges: 16, Symbols: 4}, "OK 0-36"},
		"symbol-count":   {"class X {}\n", []string{"A", "B", "C"}, &ppLimits{Bytes: 1 << 20, Directives: 8, Depth: 8, ExprDepth: 8, Ranges: 16, Symbols: 2}, "LIMIT "},
		"symbol-bytes":   {"class X {}\n", []string{"ABCDEF"}, &ppLimits{Bytes: 14, Directives: 8, Depth: 8, ExprDepth: 8, Ranges: 16, Symbols: 2}, "LIMIT "},
		"invalid-utf8":   {"class X { string s = \"\xff\"; }\n", nil, nil, "INVALID_INPUT "},
		"bad-symbol":     {"class X {}\n", []string{"@A"}, nil, "INVALID_INPUT "},
		"keyword":        {"class X {}\n", []string{"true"}, nil, "INVALID_INPUT "},
		"unknown":        {"#ifdef A\n#endif\n", nil, nil, "MALFORMED "},
		"if-paren":       {"#if(A)\nclass X {}\n#endif\nx\n", []string{"A"}, nil, "OK 7-18,25-27"},
	} {
		lim := ppDefaultLimits
		if c.lim != nil {
			lim = *c.lim
		}
		if got := ranges(csharpPreprocess([]byte(c.src), EncodingUTF8, c.symbols, lim)); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
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
