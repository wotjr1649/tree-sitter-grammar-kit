// Package ppexpect derives the expected C# preprocessor selection of one source from a
// pinned Roslyn observation alone (#218). The kit's differential test and the qualification
// inventory generator share it, so neither expectation comes from the evaluator under test.
// It imports nothing of the kit.
package ppexpect

import (
	"slices"
	"strings"
)

// ObservedSchema and Roslyn are the observation revision and the Roslyn build it pins.
const (
	ObservedSchema = "tsgk-csharp-pp-roslyn/r2"
	Roslyn         = "5.9.0.0"
)

// Observation is one case of a tsgk-csharp-pp-roslyn/r2 document (src/dev/csharp-pp-oracle).
type Observation struct {
	ID          string       `json:"id"`
	Features    []string     `json:"features"`
	Directives  []Directive  `json:"directives"`
	Disabled    []Range      `json:"disabled"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	OracleError string       `json:"oracle_error"` // Roslyn failed: no observation
}

// Directive is one Roslyn directive trivia: its kind, the offset of its '#', whether it is
// active, whether its branch is taken (#if, #elif, #else) and its physical line.
type Directive struct {
	Kind      string `json:"kind"`
	Start     int    `json:"start"`
	Active    bool   `json:"active"`
	Taken     *bool  `json:"taken"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
}

// Range is a half-open byte range.
type Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Diagnostic is one Roslyn error: where it starts and what Roslyn attached it to (a directive
// kind without the DirectiveTrivia suffix, EndOfFile, or Other).
type Diagnostic struct {
	ID      string `json:"id"`
	Start   int    `json:"start"`
	Context string `json:"context"`
}

// Step is the expected selection of one source: the evaluator status and, for OK and
// ZERO_ACTIVE, the directives (kind without the suffix, '#' offset, active, taken) and the
// active and inactive byte ranges. MALFORMED and RANGE_LIMIT claim no selection.
type Step struct {
	Status     string      `json:"status"`
	Directives []Directive `json:"directives"`
	Active     []Range     `json:"active"`
	Inactive   []Range     `json:"inactive"`
}

// SelectionError names a Roslyn error that voids the section selection: an error in a
// directive that steers or brackets selection (or an unknown directive), a group left open at
// the end of the file, and a directive after code on its line (CS1040). Errors elsewhere
// (ordinary C# syntax, including a '#' in an interpolation hole, where Roslyn reads no
// directive; #error, #line, #pragma, #nullable) leave the selection standing.
func SelectionError(o Observation) string {
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

// Expect derives the selection of src from its Roslyn observation under a cap of ranges
// active ranges: with a selection error it is MALFORMED; otherwise the directives and the
// inactive ranges (Roslyn's disabled text) are Roslyn's, and the active ranges are the source
// minus the disabled text and the physical lines (Roslyn's line breaks) of every steering
// (#if, #elif, #else, #endif, #define, #undef) or inactive directive; no active range is
// ZERO_ACTIVE and more than ranges is RANGE_LIMIT.
func Expect(src []byte, o Observation, ranges int) Step {
	if SelectionError(o) != "" {
		return Step{Status: "MALFORMED"}
	}
	var drop []Range
	drop = append(drop, o.Disabled...)
	for _, d := range o.Directives {
		switch strings.TrimSuffix(d.Kind, "DirectiveTrivia") {
		case "If", "Elif", "Else", "EndIf", "Define", "Undef":
		default:
			if d.Active {
				continue
			}
		}
		drop = append(drop, Range{d.LineStart, d.LineEnd})
	}
	slices.SortFunc(drop, func(a, b Range) int { return a.Start - b.Start })
	s := Step{Status: "OK", Directives: []Directive{}, Active: []Range{}, Inactive: []Range{}}
	at := 0
	for _, r := range append(drop, Range{len(src), len(src)}) {
		if r.Start > at {
			s.Active = append(s.Active, Range{at, r.Start})
		}
		at = max(at, r.End)
	}
	switch {
	case len(s.Active) == 0:
		s.Status = "ZERO_ACTIVE"
	case len(s.Active) > ranges:
		return Step{Status: "RANGE_LIMIT"}
	}
	for _, d := range o.Directives {
		s.Directives = append(s.Directives, Directive{Kind: strings.TrimSuffix(d.Kind, "DirectiveTrivia"), Start: d.Start, Active: d.Active, Taken: d.Taken})
	}
	s.Inactive = append(s.Inactive, o.Disabled...)
	return s
}
