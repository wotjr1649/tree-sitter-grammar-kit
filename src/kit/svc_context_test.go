package kit

import (
	"encoding/base64"
	"testing"
)

func TestSvcCallerContext(t *testing.T) {
	context := &SvcContext{DefaultLanguage: "C#", LanguageSource: "owned-project", CodeBehind: []SvcReference{{Name: "S😀.svc.cs", Case: "svc-reference"}}}
	for _, encoding := range []string{EncodingUTF8, EncodingUTF16LE, EncodingUTF16BE} {
		src := svcEncoded(encoding, "<% @ServiceHost Service=\"S\" CodeBehind=\"S😀.svc.cs\" %>\nclass S {}")
		o := ObserveServiceHostWithContext(encoding, src, context)
		if o.Language.Status != "CSHARP" || o.Language.Origin != "CALLER_DEFAULT" || o.Language.Source != context.LanguageSource || len(o.IncludedRanges) != 1 || o.CodeBehind.Case != "svc-reference" || o.CodeBehind.Resolution != "DECLARED" {
			t.Fatalf("%s %+v", encoding, o)
		}
		o = ObserveServiceHostWithContext(encoding, svcEncoded(encoding, "<%@ ServiceHost Language=\"VB\" Service=\"S\" %>\nClass S"), context)
		if o.Language.Status != "UNSUPPORTED_LANGUAGE" || o.IncludedRanges != nil {
			t.Fatal("explicit language overwritten")
		}
		o = ObserveServiceHostWithContext(encoding, svcEncoded(encoding, "<%@ ServiceHost Service=\"S\"\nclass S {}"), context)
		if o.IncludedRanges != nil || o.Coverage.Inline != "UNRESOLVED" {
			t.Fatal("invented missing boundary")
		}
	}
}

func TestSvcCP949ReferenceSpelling(t *testing.T) {
	src := append([]byte(`<%@ ServiceHost Service="S" CodeBehind="S`), 0xB0, 0xA1)
	src = append(src, []byte(".svc.cs\" %>\nz = [1, 2];")...)
	ctx := &SvcContext{DefaultLanguage: "C#", LanguageSource: "owned", CodeBehind: []SvcReference{{Name: "S가.svc.cs", Case: "ref"}}}
	if !SourceEncodingValid(EncodingCP949, src) {
		t.Fatal("invalid owned CP949 input")
	}
	o := ObserveServiceHostWithContext(EncodingCP949, src, ctx)
	if o.CodeBehind == nil || o.CodeBehind.Case != "ref" || len(o.IncludedRanges) != 1 || o.Language.Status != "CSHARP" {
		t.Fatalf("%+v", o)
	}
}

func TestSvcProfileSourceIdentity(t *testing.T) {
	for _, schema := range []string{IncrementalSchema, OracleSchema} {
		for _, bad := range []string{"", "hash", "encoding", "noncanonical", "self-reference", "prefixed-self-reference", "batch", "reference-string"} {
			p := anchorProfile(schema, nil, nil)
			p["symbol"], p["format"] = "tree_sitter_c_sharp", SvcFormat
			c := p["cases"].([]any)[0].(map[string]any)
			source := []byte(`<%@ ServiceHost Service="S" CodeBehind="self.cs" %>`)
			if bad == "prefixed-self-reference" {
				source = append([]byte("// prefix\n"), source...)
			}
			if bad == "reference-string" {
				source = []byte(`class S { string text = "<%@ ServiceHost Service='S' %>"; }`)
				p["svc_context"] = map[string]any{"default_language": "C#", "language_source": "owned", "code_behind": []any{map[string]any{"name": "S.cs", "case": "c1"}}}
			}
			c["input"].(map[string]any)["sha256"], c["input"].(map[string]any)["bytes"] = digestHex(source), len(source)
			c["edits"], c["expect"], c["svc_source"] = []any{}, []any{}, base64.StdEncoding.EncodeToString(source)
			switch bad {
			case "hash":
				c["input"].(map[string]any)["sha256"] = fxCompiler
			case "encoding":
				c["encoding"] = EncodingUTF16LE
			case "noncanonical":
				c["svc_source"] = c["svc_source"].(string) + "\n"
			case "self-reference", "prefixed-self-reference", "batch":
				p["svc_context"] = map[string]any{"default_language": "C#", "language_source": "owned", "code_behind": []any{map[string]any{"name": "self.cs", "case": "c1"}}}
				if bad == "batch" {
					p["operation"], p["output"] = "private-corpus-local", OutputRecord
					if schema == OracleSchema {
						continue
					}
				}
			}
			_, err := parseProfileOf(schema, p)
			if (err != nil) != (bad != "" && bad != "reference-string") {
				t.Fatalf("%s/%s: %v", schema, bad, err)
			}
		}
	}
}

func TestSvcContextProfileBoundary(t *testing.T) {
	for _, schema := range []string{IncrementalSchema, OracleSchema} {
		for _, bad := range []string{"", "missing-case", "duplicate-name", "missing-provenance", "unknown-field", "legacy-format"} {
			p := anchorProfile(schema, nil, nil)
			p["symbol"] = "tree_sitter_c_sharp"
			p["format"] = SvcFormat
			r := map[string]any{"name": "S.svc.cs", "case": "c1"}
			c := map[string]any{"default_language": "C#", "language_source": "owned-project", "code_behind": []any{r}}
			switch bad {
			case "missing-case":
				r["case"] = "absent"
			case "duplicate-name":
				c["code_behind"] = []any{r, r}
			case "missing-provenance":
				c["language_source"] = ""
			case "unknown-field":
				c["automatic_path"] = true
			case "legacy-format":
				p["format"] = SvcLegacyFormat
			}
			p["svc_context"] = c
			parsed, err := parseProfileOf(schema, p)
			if bad == "" {
				if err != nil || parsed.SvcContext == nil {
					t.Fatalf("%s %v", schema, err)
				}
			} else if err == nil {
				t.Fatalf("%s accepted %s", schema, bad)
			}
		}
	}
}

func TestSvcAssemblyDirectives(t *testing.T) {
	for _, encoding := range []string{EncodingUTF8, EncodingUTF16LE, EncodingUTF16BE} {
		text := "<%@ Assembly Name=\"System\" %>\n<% @ServiceHost Language=\"C#\" Service=\"S\" %>\n<%@ Assembly Src=\"../inert.cs\" %>\nclass S {}"
		src := svcEncoded(encoding, text)
		o := ObserveServiceHost(encoding, src)
		if o.Directive == nil || len(o.AdditionalDirectives) != 2 || len(o.IncludedRanges) != 1 {
			t.Fatalf("%s %+v", encoding, o)
		}
		if err, known := o.Syntax(nil); err || known {
			t.Fatal("valid inline must await its tree")
		}
		for _, d := range o.AdditionalDirectives {
			if len(d.Diagnostics) > 0 || d.Close == nil {
				t.Fatalf("%+v", d)
			}
		}
		want := svcEncoded(encoding, "\nclass S {}")
		if got := src[o.IncludedRanges[0].StartByte:]; string(got) != string(want) {
			t.Fatal("directive left in inline")
		}
		for _, bad := range []string{`<%@ Assembly %>`, `<%@ Assembly Name="" %>`, `<%@ Assembly Name="System" Src="inert.cs" %>`} {
			o = ObserveServiceHost(encoding, svcEncoded(encoding, `<%@ ServiceHost Service="S" %>`+bad))
			hasError, known := o.Syntax(nil)
			if !hasError || !known {
				t.Fatalf("%s %s accepted", encoding, bad)
			}
		}
	}
}

func TestSvcDirectivePrefixEncodings(t *testing.T) {
	for _, enc := range []string{EncodingUTF8, EncodingUTF16LE, EncodingUTF16BE, EncodingCP949} {
		for _, prefix := range []string{"", "\n ", "class S { string s = \"", "@\""} {
			src := svcEncoded(enc, prefix+`<%@ ServiceHost Service="S" %>`)
			o := ObserveServiceHost(enc, src)
			if o.HasDirectivePrefix(enc, src) != (prefix == "" || prefix == "\n ") {
				t.Fatalf("%s %q", enc, prefix)
			}
		}
	}
	src := append([]byte{0xA1, 0xA1}, []byte(`<%@ ServiceHost Service="S" %>`)...)
	o := ObserveServiceHost(EncodingCP949, src)
	if !SourceEncodingValid(EncodingCP949, src) || !o.HasDirectivePrefix(EncodingCP949, src) {
		t.Fatal("CP949 U+3000 prefix bypass")
	}
	o.Directive.Open.StartByte = uint32(len(src) + 1)
	if o.HasDirectivePrefix(EncodingCP949, src) {
		t.Fatal("out-of-source prefix accepted")
	}
}

func TestSvcBOMIsNotDirectiveSeparator(t *testing.T) {
	for _, enc := range []string{EncodingUTF8, EncodingUTF16LE, EncodingUTF16BE} {
		o := ObserveServiceHost(enc, svcEncoded(enc, "<%\ufeff@ServiceHost Service=\"S\" %>"))
		bad, known := o.Syntax(nil)
		if o.Directive != nil || !bad || !known {
			t.Fatalf("%s: %+v", enc, o)
		}
	}
}
