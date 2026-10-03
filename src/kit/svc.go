package kit

import "strings"

// SVC-SERVICEHOST-r1 (net461-workload.md) composite record, produced from S05.
const (
	SvcFormat          = "SVC-SERVICEHOST-r1"
	SvcCompositeSchema = "tsgk-svc-composite/r1"
)

// Span is a half-open original byte range with 0-based row and byte-column points.
type Span struct {
	StartByte  uint32 `json:"start_byte"`
	EndByte    uint32 `json:"end_byte"`
	StartPoint Point  `json:"start_point"`
	EndPoint   Point  `json:"end_point"`
}

// SvcAttribute is one directive attribute: name and value ranges and the quote used.
type SvcAttribute struct {
	Name  Span    `json:"name"`
	Value *Span   `json:"value"`
	Quote *string `json:"quote"`
}

// SvcDirective is the observed <%@ ServiceHost ... %> directive.
type SvcDirective struct {
	Range       Span           `json:"range"`
	Open        Span           `json:"open"`
	Close       *Span          `json:"close"`
	Name        Span           `json:"name"`
	Attributes  []SvcAttribute `json:"attributes"`
	Diagnostics []string       `json:"diagnostics"`
}

// SvcLanguage is the Language value range and its status.
type SvcLanguage struct {
	Value  *Span  `json:"value"`
	Status string `json:"status"`
}

// SvcCodeBehind is the declared CodeBehind reference; the kit never resolves it itself.
type SvcCodeBehind struct {
	Value      Span   `json:"value"`
	Resolution string `json:"resolution"`
}

// SvcCoverage separates directive, CodeBehind and inline coverage.
type SvcCoverage struct {
	Directive  string `json:"directive"`
	CodeBehind string `json:"code_behind"`
	Inline     string `json:"inline"`
}

// SvcObservation is the offline part of the composite: everything except the inline tree.
// IncludedRanges is set only when the inline language is C#.
type SvcObservation struct {
	Directive      *SvcDirective  `json:"directive"`
	Language       SvcLanguage    `json:"language"`
	CodeBehind     *SvcCodeBehind `json:"code_behind"`
	IncludedRanges []Span         `json:"included_ranges"`
	Coverage       SvcCoverage    `json:"coverage"`
}

var svcKnown = map[string]string{"service": "Service", "factory": "Factory", "debug": "Debug", "language": "Language", "codebehind": "CodeBehind"}

// ObserveServiceHost observes the ServiceHost directive of a .svc source in its declared
// encoding without reading anything else: ranges of the directive, its delimiters, name,
// attributes (name, value, quote), diagnostics, the Language status, the CodeBehind
// reference and, for C# inline code after the directive, the included range to parse.
// Values are never extracted; the source must already satisfy SourceEncodingValid.
func ObserveServiceHost(enc string, src []byte) SvcObservation {
	step := 1
	if enc == EncodingUTF16LE || enc == EncodingUTF16BE {
		step = 2
	}
	n := len(src) / step
	at := func(i int) rune { // the code unit at unit index i as an ASCII-comparable rune
		if step == 2 {
			return rune(unit16(enc, src[2*i:]))
		}
		return rune(src[i])
	}
	span := func(a, b int) Span {
		return Span{StartByte: uint32(a * step), EndByte: uint32(b * step), StartPoint: PointAt(enc, src, a*step), EndPoint: PointAt(enc, src, b*step)}
	}
	match := func(i int, s string) bool {
		if i+len(s) > n {
			return false
		}
		for k := 0; k < len(s); k++ {
			if at(i+k) != rune(s[k]) {
				return false
			}
		}
		return true
	}
	space := func(i int) bool { r := at(i); return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == 0xFEFF }
	word := func(i int) bool {
		r := at(i)
		return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
	}
	obs := SvcObservation{Coverage: SvcCoverage{Directive: "ABSENT", CodeBehind: "ABSENT", Inline: "ABSENT"}, Language: SvcLanguage{Status: "NOT_REQUIRED"}}
	start := -1
	for i := 0; i < n; i++ {
		if match(i, "<%@") {
			start = i
			break
		}
	}
	inlineFrom := 0
	if start >= 0 {
		d := &SvcDirective{Open: span(start, start+3), Attributes: []SvcAttribute{}, Diagnostics: []string{}}
		i := start + 3
		for i < n && space(i) {
			i++
		}
		ns := i
		for i < n && word(i) {
			i++
		}
		d.Name = span(ns, i)
		seen := map[string]bool{}
		end := -1
		for i < n {
			for i < n && space(i) {
				i++
			}
			if i >= n {
				break
			}
			if match(i, "%>") {
				c := span(i, i+2)
				d.Close = &c
				end = i + 2
				break
			}
			as := i
			for i < n && word(i) {
				i++
			}
			if i == as { // not an attribute name: skip one unit
				i++
				continue
			}
			attr := SvcAttribute{Name: span(as, i)}
			name := strings.ToLower(string(src[as*step : i*step]))
			if step == 2 {
				var b strings.Builder
				for k := as; k < i; k++ {
					b.WriteRune(at(k))
				}
				name = strings.ToLower(b.String())
			}
			canon, known := svcKnown[name]
			if !known {
				d.Diagnostics = append(d.Diagnostics, "ATTRIBUTE_UNKNOWN")
			} else if seen[canon] {
				d.Diagnostics = append(d.Diagnostics, "ATTRIBUTE_DUPLICATE")
			}
			seen[canon] = true
			for i < n && space(i) {
				i++
			}
			if i < n && at(i) == '=' {
				i++
				for i < n && space(i) {
					i++
				}
				if i < n && (at(i) == '"' || at(i) == '\'') {
					q := string(at(i))
					attr.Quote = &q
					vs := i + 1
					i = vs
					for i < n && at(i) != rune(q[0]) && !match(i, "%>") {
						i++
					}
					v := span(vs, i)
					attr.Value = &v
					if i < n && at(i) == rune(q[0]) {
						i++
					} else {
						d.Diagnostics = append(d.Diagnostics, "QUOTE_UNTERMINATED")
					}
				} else {
					vs := i
					for i < n && !space(i) && !match(i, "%>") {
						i++
					}
					v := span(vs, i)
					attr.Value = &v
				}
			}
			d.Attributes = append(d.Attributes, attr)
			if known && attr.Value != nil {
				switch canon {
				case "Language":
					v := *attr.Value
					obs.Language.Value = &v
				case "CodeBehind":
					obs.CodeBehind = &SvcCodeBehind{Value: *attr.Value, Resolution: "NOT_RESOLVED"}
					obs.Coverage.CodeBehind = "OBSERVED"
				}
			}
		}
		if end < 0 {
			d.Diagnostics = append(d.Diagnostics, "TERMINATOR_MISSING")
			end = n
		}
		for k := end; k+2 < n; k++ {
			if match(k, "<%@") {
				d.Diagnostics = append(d.Diagnostics, "DIRECTIVE_DUPLICATE")
				break
			}
		}
		d.Range = span(start, end)
		obs.Directive = d
		obs.Coverage.Directive = "OBSERVED"
		inlineFrom = end
	}
	hasInline := false
	for k := inlineFrom; k < n; k++ {
		if !space(k) {
			hasInline = true
			break
		}
	}
	if start < 0 {
		hasInline = false // a source without a directive is not a ServiceHost composite
	}
	switch {
	case obs.Language.Value != nil:
		v := obs.Language.Value
		lang := strings.TrimSpace(string(src[v.StartByte:v.EndByte]))
		if step == 2 {
			var b strings.Builder
			for k := int(v.StartByte) / 2; k < int(v.EndByte)/2; k++ {
				b.WriteRune(at(k))
			}
			lang = b.String()
		}
		if lang == "C#" || lang == "c#" {
			obs.Language.Status = "CSHARP"
		} else {
			obs.Language.Status = "UNSUPPORTED_LANGUAGE"
		}
	case hasInline:
		obs.Language.Status = "UNRESOLVED_LANGUAGE"
	}
	switch {
	case obs.Directive != nil && obs.Directive.Close == nil:
		obs.Coverage.Inline = "UNRESOLVED" // the directive runs to EOF: inline code cannot be told apart
	case !hasInline:
	case obs.Language.Status == "CSHARP":
		obs.IncludedRanges = []Span{span(inlineFrom, n)}
		obs.Coverage.Inline = "OBSERVED"
	case obs.Language.Status == "UNSUPPORTED_LANGUAGE":
		obs.Coverage.Inline = "UNSUPPORTED"
	default:
		obs.Coverage.Inline = "UNRESOLVED"
	}
	return obs
}
