package kit

import (
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// SvcContext is caller-supplied project context; references name registered C# cases.
type SvcContext struct {
	DefaultLanguage string         `json:"default_language"`
	LanguageSource  string         `json:"language_source"`
	CodeBehind      []SvcReference `json:"code_behind"`
}

// SvcReference connects an inert CodeBehind spelling to an explicitly registered case.
type SvcReference struct {
	Name string `json:"name"`
	Case string `json:"case"`
}

// ObserveServiceHostWithContext observes source using explicit caller context. It never
// reads configuration or follows CodeBehind paths. The referenced case needs its own parse.
func ObserveServiceHostWithContext(enc string, src []byte, context *SvcContext) SvcObservation {
	o := ObserveServiceHost(enc, src)
	if context == nil {
		return o
	}
	if o.Language.Status == "UNRESOLVED_LANGUAGE" && o.Directive != nil && o.Directive.Close != nil && context.DefaultLanguage != "" && context.LanguageSource != "" {
		o.Language.Origin = "CALLER_DEFAULT"
		o.Language.Source = context.LanguageSource
		if svcCSharp(strings.TrimSpace(context.DefaultLanguage)) {
			o.Language.Status = "CSHARP"
			o.Coverage.Inline = "OBSERVED"
			start := o.Directive.Close.EndByte
			for _, d := range o.AdditionalDirectives {
				if d.Range.EndByte > start {
					start = d.Range.EndByte
				}
			}
			o.IncludedRanges = []Span{{StartByte: start, EndByte: uint32(len(src)), StartPoint: PointAt(enc, src, int(start)), EndPoint: PointAt(enc, src, len(src))}}
		} else {
			o.Language.Status = "UNSUPPORTED_LANGUAGE"
			o.Coverage.Inline = "UNSUPPORTED"
		}
	}
	if o.CodeBehind != nil {
		v := o.CodeBehind.Value
		name := svcString(enc, src[v.StartByte:v.EndByte])
		for _, r := range context.CodeBehind {
			if name == r.Name {
				o.CodeBehind.Case = r.Case
				o.CodeBehind.Resolution = "DECLARED"
				break
			}
		}
	}
	return o
}

func svcString(enc string, src []byte) string {
	switch enc {
	case EncodingUTF16LE, EncodingUTF16BE:
		units := make([]uint16, 0, len(src)/2)
		for i := 0; i+1 < len(src); i += 2 {
			units = append(units, uint16(unit16(enc, src[i:])))
		}
		return string(utf16.Decode(units))
	case EncodingCP949:
		var out strings.Builder
		for i := 0; i < len(src); i++ {
			if src[i] < 0x80 {
				out.WriteByte(src[i])
				continue
			}
			if i+1 >= len(src) || !CP949Pair(src[i], src[i+1]) {
				return ""
			}
			p := (int(src[i])-0x81)*190 + int(src[i+1]) - 0x41
			out.WriteRune(rune(euckrTable()[p]))
			i++
		}
		return out.String()
	default:
		return string(src)
	}
}

func svcCSharp(language string) bool {
	return strings.EqualFold(language, "C#") || strings.EqualFold(language, "cs") || strings.EqualFold(language, "csharp")
}

func svcSpace(enc string, src []byte, i int) bool {
	space := unicode.IsSpace
	if enc == EncodingUTF16LE || enc == EncodingUTF16BE {
		return space(rune(unit16(enc, src[2*i:])))
	}
	if src[i] < 0x80 {
		return space(rune(src[i]))
	}
	if enc == EncodingCP949 {
		if i+1 < len(src) && CP949Pair(src[i], src[i+1]) {
			p := (int(src[i])-0x81)*190 + int(src[i+1]) - 0x41
			if space(rune(euckrTable()[p])) {
				return true
			}
		}
		if i > 0 && CP949Pair(src[i-1], src[i]) {
			p := (int(src[i-1])-0x81)*190 + int(src[i]) - 0x41
			return space(rune(euckrTable()[p]))
		}
		return false
	}
	start := i
	for k := 0; k < 3 && start > 0 && src[start]&0xC0 == 0x80; k++ {
		start--
	}
	r, width := utf8.DecodeRune(src[start:])
	return start+width > i && space(r)
}

func parseSvcContext(t typed, v *jv, p IncrementalProfile) (*SvcContext, *Error) {
	m, e := t.object(v, []string{"default_language", "language_source", "code_behind"})
	if e != nil {
		return nil, e
	}
	out := &SvcContext{}
	if out.DefaultLanguage, e = t.str(m["default_language"]); e != nil {
		return nil, e
	}
	if out.LanguageSource, e = t.str(m["language_source"]); e != nil {
		return nil, e
	}
	if len(out.DefaultLanguage) > 128 || len(out.LanguageSource) > 128 || (out.DefaultLanguage == "") != (out.LanguageSource == "") || out.DefaultLanguage != "" && (strings.TrimSpace(out.DefaultLanguage) == "" || strings.TrimSpace(out.LanguageSource) == "") {
		return nil, t.bad("SVC_CONTEXT_INVALID", v)
	}
	rows, e := t.array(m["code_behind"])
	if e != nil {
		return nil, e
	}
	if len(rows) > 16 || len(rows) > 0 && NativeOperations()[p.Operation].Batch {
		return nil, t.bad("SVC_CONTEXT_INVALID", v)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		fields, e := t.object(row, []string{"name", "case"})
		if e != nil {
			return nil, e
		}
		var r SvcReference
		if r.Name, e = t.str(fields["name"]); e != nil {
			return nil, e
		}
		if r.Case, e = t.str(fields["case"]); e != nil {
			return nil, e
		}
		found := false
		for _, c := range p.Cases {
			if c.ID == r.Case {
				if c.SvcSource != nil {
					o := ObserveServiceHost(c.Encoding, c.SvcSource)
					if o.HasDirectivePrefix(c.Encoding, c.SvcSource) {
						return nil, t.bad("SVC_REFERENCE_INVALID", row)
					}
				}
				found = true
				break
			}
		}
		if r.Name == "" || len(r.Name) > 1024 || seen[r.Name] || !found {
			return nil, t.bad("SVC_REFERENCE_INVALID", row)
		}
		seen[r.Name] = true
		out.CodeBehind = append(out.CodeBehind, r)
	}
	return out, nil
}

// HasDirectivePrefix reports a ServiceHost directive preceded only by whitespace/BOM.
// The observation and source must describe the same source and encoding.
func (o SvcObservation) HasDirectivePrefix(enc string, src []byte) bool {
	if o.Directive == nil || uint64(o.Directive.Open.StartByte) > uint64(len(src)) {
		return false
	}
	prefix := svcString(enc, src[:o.Directive.Open.StartByte])
	return strings.TrimSpace(strings.TrimPrefix(prefix, "\ufeff")) == ""
}
