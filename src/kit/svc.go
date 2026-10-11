package kit

import (
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ServiceHost composite records, produced from S05 and evaluated by S06/replay.
const (
	SvcFormat          = "SVC-SERVICEHOST-r2"
	SvcLegacyFormat    = "SVC-SERVICEHOST-r1"
	SvcCompositeSchema = "tsgk-svc-composite/r2"
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
	Range           Span           `json:"range"`
	Open            Span           `json:"open"`
	Close           *Span          `json:"close"`
	Name            Span           `json:"name"`
	Attributes      []SvcAttribute `json:"attributes"`
	Diagnostics     []string       `json:"diagnostics"`
	DiagnosticSpans []Span         `json:"diagnostic_spans"`
}

// SvcLanguage is the Language value range and its status.
type SvcLanguage struct {
	Origin string `json:"origin,omitempty"`
	Source string `json:"source,omitempty"`
	Value  *Span  `json:"value"`
	Status string `json:"status"`
}

// SvcCodeBehind is the declared CodeBehind reference; the kit never resolves it itself.
type SvcCodeBehind struct {
	Case       string `json:"case,omitempty"`
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
	AdditionalDirectives []*SvcDirective `json:"additional_directives,omitempty"`
	Directive            *SvcDirective   `json:"directive"`
	Language             SvcLanguage     `json:"language"`
	CodeBehind           *SvcCodeBehind  `json:"code_behind"`
	IncludedRanges       []Span          `json:"included_ranges"`
	Coverage             SvcCoverage     `json:"coverage"`
}

var svcKnown = map[string]string{"service": "Service", "factory": "Factory", "debug": "Debug", "language": "Language", "codebehind": "CodeBehind", "warninglevel": "WarningLevel", "compileroptions": "CompilerOptions"}

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
	lineStarts := []int{0}
	for i := 0; i < n; i++ {
		if at(i) == '\n' {
			lineStarts = append(lineStarts, (i+1)*step)
		}
	}
	point := func(offset int) Point {
		row := sort.Search(len(lineStarts), func(i int) bool { return lineStarts[i] > offset }) - 1
		return Point{Row: uint32(row), Column: uint32(offset - lineStarts[row])}
	}
	span := func(a, b int) Span {
		return Span{StartByte: uint32(a * step), EndByte: uint32(b * step), StartPoint: point(a * step), EndPoint: point(b * step)}
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
	space := func(i int) bool { return svcSpace(enc, src, i) }
	text := func(a, b int) string {
		return svcString(enc, src[a*step:b*step])
	}
	openAt := func(i int) int {
		if !match(i, "<%") {
			return -1
		}
		j := i + 2
		for j < n && space(j) {
			j++
		}
		if j < n && at(j) == '@' {
			return j + 1
		}
		return -1
	}
	word := func(i int) bool {
		r := at(i)
		return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
	}
	obs := SvcObservation{Coverage: SvcCoverage{Directive: "ABSENT", CodeBehind: "ABSENT", Inline: "ABSENT"}, Language: SvcLanguage{Status: "NOT_REQUIRED"}}
	start := -1
	for i := 0; i < n; i++ {
		if openAt(i) >= 0 {
			start = i
			break
		}
	}
	inlineFrom := 0
	for start >= 0 {
		i := openAt(start)
		d := &SvcDirective{Open: span(start, i), Attributes: []SvcAttribute{}, Diagnostics: []string{}, DiagnosticSpans: []Span{}}
		diagnose := func(code string, where Span) {
			d.Diagnostics = append(d.Diagnostics, code)
			d.DiagnosticSpans = append(d.DiagnosticSpans, where)
		}
		for i < n && space(i) {
			i++
		}
		ns := i
		for i < n && word(i) {
			i++
		}
		d.Name = span(ns, i)
		isMain := strings.EqualFold(text(ns, i), "ServiceHost")
		isAssembly := strings.EqualFold(text(ns, i), "Assembly")
		if !isMain && !isAssembly {
			diagnose("DIRECTIVE_UNKNOWN", d.Name)
		}
		if isMain && obs.Directive != nil {
			diagnose("DIRECTIVE_DUPLICATE", d.Name)
		}
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
			if i == as {
				next := i + 1
				if step == 2 && at(i) >= 0xD800 && at(i) <= 0xDBFF && next < n {
					next++
				} else if enc == EncodingCP949 && at(i) >= 0x80 && next < n {
					next++
				} else if step == 1 && enc != EncodingCP949 {
					_, width := utf8.DecodeRune(src[i:])
					next = i + width
				}
				diagnose("ATTRIBUTE_SYNTAX", span(i, next))
				i = next
				continue
			}
			attr := SvcAttribute{Name: span(as, i)}
			name := strings.ToLower(text(as, i))
			canon, known := svcKnown[name]
			if isAssembly {
				canon = name
				known = name == "name" || name == "src"
			}
			if !known {
				diagnose("ATTRIBUTE_UNKNOWN", attr.Name)
			} else if seen[name] {
				diagnose("ATTRIBUTE_DUPLICATE", attr.Name)
			}
			seen[name] = true
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
					for i < n && at(i) != rune(q[0]) {
						i++
					}
					v := span(vs, i)
					attr.Value = &v
					if i < n && at(i) == rune(q[0]) {
						i++
					} else {
						diagnose("QUOTE_UNTERMINATED", span(vs-1, i))
					}
				} else {
					vs := i
					invalid := false
					for i < n && !space(i) && !match(i, "%>") {
						invalid = invalid || strings.ContainsRune("\"'=<>", at(i))
						i++
					}
					v := span(vs, i)
					attr.Value = &v
					if invalid {
						diagnose("ATTRIBUTE_SYNTAX", v)
					}
				}
			}
			d.Attributes = append(d.Attributes, attr)
			if isAssembly && known && (attr.Value == nil || strings.TrimSpace(text(int(attr.Value.StartByte)/step, int(attr.Value.EndByte)/step)) == "") {
				diagnose("ATTRIBUTE_VALUE_INVALID", attr.Name)
			}
			if known && attr.Value != nil && isMain && obs.Directive == nil {
				value := strings.TrimSpace(text(int(attr.Value.StartByte)/step, int(attr.Value.EndByte)/step))
				if canon == "Debug" && value != "" && !strings.EqualFold(value, "true") && !strings.EqualFold(value, "false") {
					diagnose("ATTRIBUTE_VALUE_INVALID", attr.Name)
				}
				if canon == "WarningLevel" && value != "" {
					level, err := strconv.ParseInt(value, 10, 32)
					if err != nil || level < 0 {
						diagnose("ATTRIBUTE_VALUE_INVALID", attr.Name)
					}
				}
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
		if isMain && !seen["service"] && !seen["factory"] {
			diagnose("MAIN_ATTRIBUTE_MISSING", d.Name)
		}
		if isAssembly {
			if !seen["name"] && !seen["src"] {
				diagnose("MAIN_ATTRIBUTE_MISSING", d.Name)
			}
			if seen["name"] && seen["src"] {
				diagnose("ATTRIBUTE_CONFLICT", d.Name)
			}
		}
		if end < 0 {
			diagnose("TERMINATOR_MISSING", span(n, n))
			end = n
		}
		d.Range = span(start, end)
		if isMain && obs.Directive == nil {
			obs.Directive = d
		} else {
			obs.AdditionalDirectives = append(obs.AdditionalDirectives, d)
		}
		obs.Coverage.Directive = "OBSERVED"
		inlineFrom = end
		start = -1
		for k := end; k+2 < n; k++ {
			if openAt(k) >= 0 {
				start = k
				break
			}
		}
	}
	hasInline := false
	for k := inlineFrom; k < n; k++ {
		if !space(k) {
			hasInline = true
			break
		}
	}
	if obs.Directive == nil {
		hasInline = false // a source without a directive is not a ServiceHost composite
	}
	switch {
	case obs.Language.Value != nil:
		v := obs.Language.Value
		lang := strings.TrimSpace(text(int(v.StartByte)/step, int(v.EndByte)/step))
		if svcCSharp(lang) {
			obs.Language.Origin = "DIRECTIVE"
			obs.Language.Status = "CSHARP"
		} else if lang == "" {
			if hasInline {
				obs.Language.Status = "UNRESOLVED_LANGUAGE"
			} else {
				obs.Language.Status = "NOT_REQUIRED"
			}
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

// Syntax reports the composite syntax state. An unknown inline segment cannot prove
// NO_ERROR; a directive diagnostic already proves ERROR without inventing an inline tree.
func (o SvcObservation) Syntax(inlineHasError *bool) (hasError, known bool) {
	for _, d := range o.AdditionalDirectives {
		if d == nil || d.Close == nil || len(d.Diagnostics) > 0 {
			return true, true
		}
	}
	if o.Directive == nil || o.Directive.Close == nil || len(o.Directive.Diagnostics) > 0 {
		return true, true
	}
	if o.Coverage.Inline == "ABSENT" {
		return false, true
	}
	if inlineHasError != nil {
		return *inlineHasError, true
	}
	return false, false
}
