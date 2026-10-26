package foundation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/ppexpect"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// symbolRanges is the evaluator's registered active range cap (csharp-pp/r1).
const symbolRanges = 16

// symbolCase registers one corpus case of a symbol-mode workload (#218, 218-C): the corpus
// case it runs, optionally one edit that turns its source into the exact source of the corpus
// case edit (same symbols), and the syntax expectation of every step of an all-OK case,
// registered by hand: the selection comes from Roslyn alone, never from the evaluator.
type symbolCase struct {
	id           string
	find         string // the edit's old text, at its first occurrence
	replace      string
	edit         string // the corpus case the edited source must equal
	syntax       []string
	contains     [][]string
	routeBlocked bool
	cstOnly      []uint32 // independent primary token anchors, registered from the #279 survey
}

var class = []string{"class_declaration"}

// symbolSets are the registered symbol-mode workloads: one per caller symbol set, together
// covering #218's Roslyn comparison categories (the #198 cases with their own symbols,
// ! == != && || parentheses true/false, define/undef order, nesting, #if-like lines inside
// active strings, comments, raw and verbatim literals, CRLF, BOM, EOF, a zero-active and a
// malformed selection, and edits whose step selection is recomputed from a new condition).
// #277 adds a separate branch-switch workload whose expected route is BLOCKED; its
// selection/syntax checks do not claim a successful reuse observation.
var symbolSets = []struct {
	set     string
	symbols []string
	cases   []symbolCase
}{
	{"s05-csharp-symbols-none", []string{}, []symbolCase{
		{id: "issue198-inactive-broken", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "ops-none", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "equality-false", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "literal-true", find: "true", replace: "A && !B || (C == D)", edit: "ops-none", syntax: []string{"NO_ERROR", "NO_ERROR"}, contains: [][]string{class, class}},
		{id: "literal-not-true", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "elif-none", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "define-undef-order", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "define-inactive"},
		{id: "define-after-token"},
		{id: "verbatim-string", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "raw-string", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "block-comment", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "line-comment", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "non-ascii", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
	}},
	{"s05-csharp-symbols-a", []string{"A"}, []symbolCase{
		{id: "ops-a", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "unicode-escape", find: `\u0041`, replace: `\U00000041`, edit: "escape-upper", syntax: []string{"NO_ERROR", "NO_ERROR"}, contains: [][]string{class, class}},
		{id: "crlf", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "eof-no-newline", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "bom-non-ascii", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "trailing-line-comment", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "region-in-group", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
		{id: "malformed-expression"},
		{id: "unterminated"},
		{id: "directive-string-no-escape", syntax: []string{"ERROR"}, contains: [][]string{{"ERROR"}}},
		{id: "razor-comment-lines", syntax: []string{"ERROR"}, contains: [][]string{{"ERROR"}}},
	}},
	{"s05-csharp-symbols-enabled", []string{"ENABLED"}, []symbolCase{
		{id: "issue198-undef-inactive", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
	}},
	{"s05-csharp-symbols-a-c", []string{"A", "C"}, []symbolCase{
		{id: "issue198-nested-directives", syntax: []string{"NO_ERROR"}, contains: [][]string{class}},
	}},
	{"s05-csharp-symbols-switch", []string{}, []symbolCase{
		{id: "literal-true", find: "true", replace: "(false)", edit: "literal-paren-false", syntax: []string{"NO_ERROR", "NO_ERROR"}, contains: [][]string{class, class}, routeBlocked: true},
	}},
	{"s05-csharp-symbols-cst-mismatch", []string{}, []symbolCase{
		{id: "dollars-two-quotes-raw", syntax: []string{"ERROR"}, contains: [][]string{{"ERROR"}}},
		{id: "nested-string-open-no-runaway", syntax: []string{"ERROR"}, contains: [][]string{{"ERROR"}}},
	}},
}

// symbolRole builds the csharp-symbols extra role from the pinned corpus and its Roslyn
// observation: Windows and Linux execute it; darwin is deferred to #243.
func symbolRole(t *testing.T, root, symbol string, grammar []kit.NativeInput) kit.QualRole {
	t.Helper()
	read := func(p string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	casesData, observedData := read("src/testdata/csharp-pp/cases.json"), read("src/testdata/csharp-pp/roslyn-observed.json")
	var corpus struct {
		Cases []struct {
			ID, Source string
			Symbols    []string
		}
	}
	var observed struct {
		Schema, Roslyn string
		CasesSHA256    string                 `json:"cases_sha256"`
		Cases          []ppexpect.Observation `json:"cases"`
	}
	if json.Unmarshal(casesData, &corpus) != nil || json.Unmarshal(observedData, &observed) != nil {
		t.Fatal("csharp-pp corpus or observation unreadable")
	}
	// the observation is of exactly this corpus, by the pinned Roslyn
	if observed.Schema != ppexpect.ObservedSchema || observed.Roslyn != ppexpect.Roslyn || observed.CasesSHA256 != shaHex(casesData) || len(observed.Cases) != len(corpus.Cases) {
		t.Fatalf("roslyn-observed.json does not observe cases.json: %s %s %s", observed.Schema, observed.Roslyn, observed.CasesSHA256)
	}
	type entry struct {
		src     []byte
		symbols []string
		obs     ppexpect.Observation
	}
	byID := map[string]entry{}
	for i, c := range corpus.Cases {
		src, err := base64.StdEncoding.DecodeString(c.Source)
		if err != nil || observed.Cases[i].ID != c.ID {
			t.Fatalf("corpus case %d: %v", i, err)
		}
		byID[c.ID] = entry{src, c.Symbols, observed.Cases[i]}
	}
	selection := func(id string, step int, want []string) kit.QualPreprocessStep {
		t.Helper()
		e, ok := byID[id]
		if !ok || !slices.Equal(e.symbols, want) || e.obs.OracleError != "" || len(e.obs.Features) > 0 {
			t.Fatalf("%s: not an observed corpus case of symbols %q", id, want)
		}
		x := ppexpect.Expect(e.src, e.obs, symbolRanges)
		st := kit.QualPreprocessStep{Step: step, Status: x.Status, Directives: []kit.PreprocessDirective{}, Active: []kit.ByteRange{}, Inactive: []kit.ByteRange{}}
		for _, d := range x.Directives {
			st.Directives = append(st.Directives, kit.PreprocessDirective{Kind: d.Kind, Hash: d.Start, Active: d.Active, Taken: d.Taken})
		}
		for _, r := range x.Active {
			st.Active = append(st.Active, kit.ByteRange{Start: uint32(r.Start), End: uint32(r.End)})
		}
		for _, r := range x.Inactive {
			st.Inactive = append(st.Inactive, kit.ByteRange{Start: uint32(r.Start), End: uint32(r.End)})
		}
		return st
	}
	manifest := struct{ Commit string }{}
	if json.Unmarshal(read("src/drivers/native-c/runtime-manifest.json"), &manifest) != nil || manifest.Commit == "" {
		t.Fatal("runtime manifest unreadable")
	}
	var workloads []kit.QualWorkload
	for _, set := range symbolSets {
		w := kit.QualWorkload{Set: set.set, Profile: set.set, Route: "csharp", Operation: "native-parse-edit", Output: "tree", Symbol: symbol, Grammar: grammar,
			Queries: []kit.QualQuery{}, Preprocess: &kit.Preprocess{Mode: kit.PreprocessMode, Symbols: set.symbols}, RuntimeCommit: manifest.Commit}
		for _, sc := range set.cases {
			src := byID[sc.id].src
			c := kit.QualCase{ID: sc.id, Role: "support", Input: kit.NativeInput{SHA256: shaHex(src), Bytes: uint64(len(src))}, Edits: []kit.Edit{}, Expect: []kit.StepExpectation{},
				QueryExpect: []kit.QueryExpectation{}, Points: []kit.NativePoint{}, Covers: map[string][]string{}, Alternatives: []string{},
				PreprocessExpect: []kit.QualPreprocessStep{selection(sc.id, 0, set.symbols)}}
			steps := []string{sc.id}
			if sc.edit != "" {
				at := bytes.Index(src, []byte(sc.find))
				if at < 0 {
					t.Fatalf("%s: edit text not found", sc.id)
				}
				next := slices.Concat(src[:at], []byte(sc.replace), src[at+len(sc.find):])
				// the edited source is exactly another observed case's source, so its expected
				// selection is Roslyn's own observation of it
				if to, ok := byID[sc.edit]; !ok || !bytes.Equal(next, to.src) {
					t.Fatalf("%s: the edit does not give the source of %s", sc.id, sc.edit)
				}
				c.Edits = append(c.Edits, kit.Edit{StartByte: uint32(at), OldEndByte: uint32(at + len(sc.find)), NewEndByte: uint32(at + len(sc.replace)), Old: []byte(sc.find), New: []byte(sc.replace)})
				steps = append(steps, sc.edit)
			}
			// a stopped step ends the case: no step after it is evaluated or expected
			for k := 1; k < len(steps) && c.PreprocessExpect[k-1].Status == kit.PreprocessOK; k++ {
				c.PreprocessExpect = append(c.PreprocessExpect, selection(steps[k], k, set.symbols))
			}
			last := c.PreprocessExpect[len(c.PreprocessExpect)-1]
			if last.Status != kit.PreprocessOK {
				c.ExpectStatus, c.ExpectCode = kit.PreprocessOutcome(last.Status)
				c.ExpectAssessment = kit.AssessBlocked
				if len(sc.syntax) > 0 {
					t.Fatalf("%s: a stopped case registers no syntax", sc.id)
				}
			} else {
				for k, id := range steps {
					version := byID[id].src
					host := []uint32{}
					for _, d := range c.PreprocessExpect[k].Directives {
						host = append(host, uint32(d.Hash))
					}
					positions := slices.Clone(host)
					if positions == nil {
						positions = []uint32{}
					}
					for _, at := range sc.cstOnly {
						if k != 0 || int(at) >= len(version) || version[at] != '#' {
							t.Fatalf("%s: invalid manual CST anchor %d", id, at)
						}
						positions = append(positions, at)
					}
					slices.Sort(positions)
					st := kit.PreprocessCSTStep{Step: k, SourceSHA256: shaHex(version), SourceBytes: uint64(len(version)), Result: "MATCH", HostHashes: host, CSTHashes: positions, HostOnly: []uint32{}, CSTOnly: []uint32{}}
					if len(sc.cstOnly) > 0 {
						st.Result, st.Code = "MISMATCH", fmt.Sprintf("%s%d", kit.PreprocessCSTPrefix, k)
						st.CSTOnly = slices.Clone(sc.cstOnly)
					}
					c.PreprocessCSTExpect = append(c.PreprocessCSTExpect, st)
					if st.Result == "MISMATCH" {
						break
					}
				}
				if len(sc.cstOnly) > 0 {
					if len(sc.syntax) > 0 || len(sc.contains) > 0 {
						t.Fatalf("%s: CST-stopped case registers no active syntax", sc.id)
					}
					c.ExpectStatus, c.ExpectAssessment, c.ExpectCode = kit.StatusNotRun, kit.AssessBlocked, kit.PreprocessCSTPrefix+"0"
					w.Cases = append(w.Cases, c)
					continue
				}
				if len(sc.syntax) != len(steps) || len(sc.contains) != len(steps) {
					t.Fatalf("%s: register the syntax of each of its %d steps", sc.id, len(steps))
				}
				for k := range steps {
					c.Expect = append(c.Expect, kit.StepExpectation{Step: k, Syntax: sc.syntax[k], Contains: sc.contains[k]})
				}
				if sc.routeBlocked {
					c.ExpectStatus, c.ExpectAssessment, c.ExpectCode = kit.StatusCompleted, kit.AssessBlocked, kit.SelectionRoutePrefix+"1"
				}
			}
			w.Cases = append(w.Cases, c)
		}
		workloads = append(workloads, w)
	}
	return kit.QualRole{ID: "csharp-symbols", Role: "owned", Status: kit.RoleExecuted,
		Reason: fmt.Sprintf("C# symbol mode (%s, #218): selections derived from %s observation sha256 %s by Roslyn %s of src/testdata/csharp-pp/cases.json sha256 %s, with hand-registered syntax; syntax only",
			kit.PreprocessMode, ppexpect.ObservedSchema, shaHex(observedData), ppexpect.Roslyn, observed.CasesSHA256),
		Platforms: []string{"windows-amd64", "linux-amd64"}, NotApplicable: map[string]string{},
		Deferred:  map[string]string{"darwin-arm64": "#243: the macOS symbol-mode host result is deferred (Windows amd64 and Linux amd64 are the completion criteria)"},
		Workloads: workloads}
}

// #218: the symbol role selects by Roslyn alone: a wrong selection in the corpus observation
// would change the registration, and an edit that does not land on another corpus source,
// symbols that differ from the case's own, or syntax on a stopped case are refused.
func TestSymbolRoleRegistration(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	symbol, grammar := "tree_sitter_c_sharp", []kit.NativeInput{{Path: "src/parser.c", Role: "grammar-source", SHA256: strings.Repeat("0", 64), Bytes: 1}}
	r := symbolRole(t, root, symbol, grammar)
	n, stopped, edited := 0, 0, 0
	for _, w := range r.Workloads {
		for _, c := range w.Cases {
			n++
			if c.ExpectStatus != "" && c.ExpectStatus != kit.StatusCompleted {
				stopped++
			}
			if len(c.Edits) > 0 {
				edited++
			}
		}
	}
	if len(r.Workloads) != 6 || n != 30 || stopped != 4 || edited != 3 {
		t.Fatalf("workloads %d cases %d stopped %d edited %d", len(r.Workloads), n, stopped, edited)
	}
	// literal-true's edit replaces the condition: both steps are selected from Roslyn's own
	// observation of each source
	for _, c := range r.Workloads[0].Cases {
		if c.ID == "literal-true" && (len(c.PreprocessExpect) != 2 || c.PreprocessExpect[1].Directives[1].Hash == c.PreprocessExpect[0].Directives[1].Hash) {
			t.Fatalf("literal-true selection %+v", c.PreprocessExpect)
		}
	}
}
