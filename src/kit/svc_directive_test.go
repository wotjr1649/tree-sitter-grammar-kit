package kit

import (
	"encoding/binary"
	"slices"
	"testing"
	"unicode/utf16"
)

func TestSvcDirectiveCompatibility(t *testing.T) {
	cases := []struct {
		name, source string
		diagnostics  []string
	}{
		{"spaced opener", `<% @ServiceHost Service="S" %>`, nil},
		{"quoted terminator", `<%@ ServiceHost Service="S" CodeBehind="a%>b.cs" %>`, nil},
		{"compiler parameters", `<%@ ServiceHost Service="S" WarningLevel="4" CompilerOptions="/checked" %>`, nil},
		{"bare attribute", `<%@ ServiceHost Service %>`, nil},
		{"empty value", `<%@ ServiceHost Service= %>`, nil},
		{"unquoted consumes debug", `<% @ServiceHost Service= Debug="true" %>`, []string{"ATTRIBUTE_SYNTAX"}},
		{"unquoted consumes language", `<%@ ServiceHost Factory= Language="C#" WarningLevel="4" %>`, []string{"ATTRIBUTE_SYNTAX"}},
		{"unquoted language consumes warning", `<%@ ServiceHost Factory="F" Language= WarningLevel="4" %>`, []string{"ATTRIBUTE_SYNTAX"}},
		{"unquoted consumes codebehind", `<%@ ServiceHost Service= CodeBehind="a%>b.cs" %>`, []string{"ATTRIBUTE_SYNTAX"}},
		{"wrong name", `<%@ Page Service="S" %>`, []string{"DIRECTIVE_UNKNOWN"}},
		{"missing main", `<%@ ServiceHost %>`, []string{"MAIN_ATTRIBUTE_MISSING"}},
		{"punctuation", `<%@ ServiceHost Service="S" ; %>`, []string{"ATTRIBUTE_SYNTAX"}},
		{"bad debug", `<%@ ServiceHost Service="S" Debug="banana" %>`, []string{"ATTRIBUTE_VALUE_INVALID"}},
		{"bad warning", `<%@ ServiceHost Service="S" WarningLevel="-1" %>`, []string{"ATTRIBUTE_VALUE_INVALID"}},
		{"duplicate", `<%@ ServiceHost Service="S" SERVICE="T" %>`, []string{"ATTRIBUTE_DUPLICATE"}},
	}
	for _, c := range cases {
		for _, encoding := range []string{EncodingUTF8, EncodingUTF16LE, EncodingUTF16BE, EncodingCP949} {
			t.Run(c.name+"/"+encoding, func(t *testing.T) {
				src := svcEncoded(encoding, c.source)
				o := ObserveServiceHost(encoding, src)
				d := o.Directive
				if d == nil && len(o.AdditionalDirectives) == 1 {
					d = o.AdditionalDirectives[0]
				}
				if d == nil || d.Close == nil || !slices.Equal(d.Diagnostics, c.diagnostics) {
					t.Fatalf("directive: %+v", o.Directive)
				}
				if len(d.Diagnostics) != len(d.DiagnosticSpans) {
					t.Fatal("unbound diagnostic")
				}
				for _, sp := range d.DiagnosticSpans {
					if sp.StartByte > sp.EndByte || sp.EndByte > uint32(len(src)) || sp.StartPoint != PointAt(encoding, src, int(sp.StartByte)) || sp.EndPoint != PointAt(encoding, src, int(sp.EndByte)) {
						t.Fatalf("bad diagnostic span %+v", sp)
					}
				}
				got, known := o.Syntax(nil)
				if !known || got != (len(c.diagnostics) > 0) {
					t.Fatalf("syntax %v/%v", got, known)
				}
			})
		}
	}
	for _, encoding := range []string{EncodingUTF8, EncodingUTF16LE, EncodingUTF16BE} {
		o := ObserveServiceHost(encoding, svcEncoded(encoding, "<%@ ServiceHost Language=\" C# \" Service=\"S\" %>\nclass S {}"))
		if o.Language.Status != "CSHARP" || len(o.IncludedRanges) != 1 {
			t.Fatalf("%s %+v", encoding, o)
		}
	}
}

func TestSvcUnicodeWhitespaceAndPoints(t *testing.T) {
	for _, enc := range []string{EncodingUTF8, EncodingUTF16LE, EncodingUTF16BE} {
		src := svcEncoded(enc, "\ufeff<%\u2003@ServiceHost\u00a0Service=\"😀\"\r\nLanguage=\" C# \" ; %>\nclass S {}")
		o := ObserveServiceHost(enc, src)
		if o.Language.Status != "CSHARP" || len(o.Directive.Diagnostics) != 1 {
			t.Fatalf("%s %+v", enc, o)
		}
		sp := o.Directive.DiagnosticSpans[0]
		if svcString(enc, src[sp.StartByte:sp.EndByte]) != ";" || sp.StartPoint != PointAt(enc, src, int(sp.StartByte)) {
			t.Fatalf("%s %+v", enc, sp)
		}
	}
}

func svcEncoded(encoding, text string) []byte {
	if encoding != EncodingUTF16LE && encoding != EncodingUTF16BE {
		return []byte(text)
	}
	var src []byte
	for _, u := range utf16.Encode([]rune(text)) {
		if encoding == EncodingUTF16LE {
			src = binary.LittleEndian.AppendUint16(src, u)
		} else {
			src = binary.BigEndian.AppendUint16(src, u)
		}
	}
	return src
}
