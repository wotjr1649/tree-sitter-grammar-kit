package kit

import (
	"encoding/binary"
	"slices"
	"testing"
)

func spanText(src []byte, s *Span) string {
	if s == nil {
		return "<nil>"
	}
	return string(src[s.StartByte:s.EndByte])
}

// N461-SVC-DIRECTIVE/CODEBEHIND/INLINE/MULTILINE/QUOTE/TERMINATOR/LANGUAGE: the offline
// directive observation keeps ranges, quotes, diagnostics, language status and coverage.
func TestObserveServiceHost(t *testing.T) {
	cases := []struct {
		name, src          string
		lang, dir, cb, inl string
		diags              []string
		included           string
		attrs              int
	}{
		{"directive-only", `<%@ ServiceHost Service="App.Svc" %>`, "NOT_REQUIRED", "OBSERVED", "ABSENT", "ABSENT", nil, "", 1},
		{"codebehind", "<%@ ServiceHost Language=\"C#\" Debug=\"true\" Service=\"App.Svc\" CodeBehind=\"Svc.svc.cs\" %>\r\n", "CSHARP", "OBSERVED", "OBSERVED", "ABSENT", nil, "", 4},
		{"inline", "<%@ ServiceHost Language=\"C#\" Service=\"App.Svc\" %>\nusing System;\nclass Svc { }\n", "CSHARP", "OBSERVED", "ABSENT", "OBSERVED", nil, "\nusing System;\nclass Svc { }\n", 2},
		{"multiline", "<%@ ServiceHost\r\n    Language='c#'\r\n    Factory=\"F\"\r\n    Service=\"S\" %>", "CSHARP", "OBSERVED", "ABSENT", "ABSENT", nil, "", 3},
		{"quote-unterminated", `<%@ ServiceHost Service="App.Svc %>`, "NOT_REQUIRED", "OBSERVED", "ABSENT", "UNRESOLVED", []string{"QUOTE_UNTERMINATED", "TERMINATOR_MISSING"}, "", 1},
		{"terminator-missing", "<%@ ServiceHost Language=\"C#\" Service=\"S\"\nclass Svc { }\n", "CSHARP", "OBSERVED", "ABSENT", "UNRESOLVED", []string{"ATTRIBUTE_UNKNOWN", "ATTRIBUTE_UNKNOWN", "ATTRIBUTE_SYNTAX", "ATTRIBUTE_SYNTAX", "TERMINATOR_MISSING"}, "", 4}, // all text up to EOF belongs to the open directive
		{"language-missing-inline", "<%@ ServiceHost Service=\"S\" %>\nclass Svc { }\n", "UNRESOLVED_LANGUAGE", "OBSERVED", "ABSENT", "UNRESOLVED", nil, "", 1},
		{"language-unsupported", "<%@ ServiceHost Language=\"VB\" Service=\"S\" %>\nClass Svc\n", "UNSUPPORTED_LANGUAGE", "OBSERVED", "ABSENT", "UNSUPPORTED", nil, "", 2},
		{"duplicates", `<%@ ServiceHost Service="A" Service="B" Bogus="x" %><%@ ServiceHost Service="C" %>`, "NOT_REQUIRED", "OBSERVED", "ABSENT", "ABSENT",
			[]string{"ATTRIBUTE_DUPLICATE", "ATTRIBUTE_UNKNOWN"}, "", 3},
		{"no-directive", "class Svc { }\n", "NOT_REQUIRED", "ABSENT", "ABSENT", "ABSENT", nil, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			o := ObserveServiceHost(EncodingUTF8, src)
			if o.Language.Status != tc.lang || o.Coverage.Directive != tc.dir || o.Coverage.CodeBehind != tc.cb || o.Coverage.Inline != tc.inl {
				t.Fatalf("observation %+v %+v", o.Language, o.Coverage)
			}
			if o.Directive == nil {
				if tc.attrs != 0 {
					t.Fatal("directive missing")
				}
				return
			}
			d := o.Directive
			if tc.name == "duplicates" && (len(o.AdditionalDirectives) != 1 || !slices.Equal(o.AdditionalDirectives[0].Diagnostics, []string{"DIRECTIVE_DUPLICATE"})) {
				t.Fatal("duplicate directive diagnosis missing")
			}
			if len(d.Attributes) != tc.attrs || !slices.Equal(d.Diagnostics, append([]string{}, tc.diags...)) {
				t.Fatalf("attributes %d diagnostics %v", len(d.Attributes), d.Diagnostics)
			}
			if spanText(src, &d.Open) != "<%@" || spanText(src, &d.Name) != "ServiceHost" {
				t.Fatalf("open/name %q %q", spanText(src, &d.Open), spanText(src, &d.Name))
			}
			if tc.included != "" && (len(o.IncludedRanges) != 1 || spanText(src, &o.IncludedRanges[0]) != tc.included) {
				t.Fatalf("included %v", o.IncludedRanges)
			}
			if tc.name == "codebehind" && (spanText(src, &o.CodeBehind.Value) != "Svc.svc.cs" || o.CodeBehind.Resolution != "NOT_RESOLVED" || spanText(src, d.Close) != "%>") {
				t.Fatalf("codebehind %+v", o.CodeBehind)
			}
			if tc.name == "multiline" {
				if q := d.Attributes[0].Quote; q == nil || *q != "'" || d.Attributes[2].Value.StartPoint.Row != 3 {
					t.Fatalf("quote or multiline point %+v", d.Attributes)
				}
			}
		})
	}
}

// N461-SVC-ENCODING: UTF-16 input is observed in code units; ranges stay original bytes.
func TestObserveServiceHostUTF16(t *testing.T) {
	text := "<%@ ServiceHost Language=\"C#\" Service=\"S\" %>\r\nclass Svc { }\r\n"
	src := []byte{0xFF, 0xFE}
	for _, r := range text {
		src = binary.LittleEndian.AppendUint16(src, uint16(r))
	}
	o := ObserveServiceHost(EncodingUTF16LE, src)
	if o.Language.Status != "CSHARP" || len(o.IncludedRanges) != 1 || o.Directive.Open.StartByte != 2 || o.IncludedRanges[0].StartByte%2 != 0 ||
		o.IncludedRanges[0].EndByte != uint32(len(src)) || o.IncludedRanges[0].EndPoint.Row != 2 {
		t.Fatalf("utf-16 observation %+v %+v", o.Language, o.IncludedRanges)
	}
}
