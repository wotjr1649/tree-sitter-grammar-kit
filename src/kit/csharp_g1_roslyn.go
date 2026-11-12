package kit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// Roslyn observation of the G1 ledger's case sources (#233, G1-2).
//
// src/dev/csharp-g1-roslyn records, for every source version of the ledger's cases, what the Roslyn
// of the pinned .NET SDK observes: acceptance, diagnostics, declarations, syntax nodes, directives and
// the acceptance under each LanguageVersion. This verifier reads that asset offline. It binds the
// asset to the ledger and the case sources, checks the provenance format and the size bound, and
// checks every observation's structure against the source. It cannot re-run Roslyn: the claim of
// the asset is "the behavior of the Roslyn DLLs of SDK 10.0.401 on these sources", nothing more.
// It reads no native output and derives no expected value.

const (
	CSharpG1RoslynSchema      = "tsgk-csharp-g1-roslyn-observed/r1"
	CSharpG1RoslynInputSchema = "tsgk-csharp-g1-roslyn-input/r1"
	CSharpG1RoslynPath        = "src/contracts/csharp-g1-roslyn-observed-r1.json"
	// shortcut: the asset is about 1.5 MB today; raise only with a new revision.
	csharpG1RoslynMaxBytes = 4 << 20
	g1RoslynInputMaxBytes  = 32 << 20
	g1RoslynMinVersion     = 703 // LanguageVersion.CSharp7_3
)

// csharpG1RoslynAssemblies are the Roslyn assemblies the tool loads, in name order. Their SDK path, size and
// SHA-256 are provenance, not code: the repository's version guard keeps tool pins out of source, so the
// asset's own SHA-256 (sealed in the tests, and bound by an approved evidence pin) is what fixes the DLLs.
var csharpG1RoslynAssemblies = []string{"Microsoft.CodeAnalysis", "Microsoft.CodeAnalysis.CSharp"}

var (
	g1rHex     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	g1rKind    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	g1rVersion = regexp.MustCompile(`^CSharp\d+(_\d+)?$`)
	g1rDiagID  = regexp.MustCompile(`^[A-Z]+[0-9]+$`)
	g1rDirKind = regexp.MustCompile(`^[A-Za-z]+DirectiveTrivia$`)
	g1rContext = regexp.MustCompile(`^[A-Za-z]+$`)
	g1rSDK     = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

type G1RoslynInputPin struct {
	Bytes  uint64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type G1RoslynRuntime struct {
	Framework string `json:"framework"`
	Version   string `json:"version"`
}

type G1RoslynAssembly struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	Bytes           uint64 `json:"bytes"`
	SHA256          string `json:"sha256"`
	AssemblyVersion string `json:"assembly_version"`
	ProductVersion  string `json:"product_version"`
}

type G1RoslynEnumValue struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type G1RoslynLatest struct {
	Effective string `json:"effective"`
	Value     int    `json:"value"`
}

type G1RoslynLanguageVersions struct {
	Enumeration []G1RoslynEnumValue `json:"enumeration"`
	Observed    []string            `json:"observed"`
	Latest      G1RoslynLatest      `json:"latest"`
}

type G1RoslynParse struct {
	DocumentationMode string `json:"documentation_mode"`
	SourceCodeKind    string `json:"source_code_kind"`
}

type G1RoslynProvenance struct {
	SDK              string                   `json:"sdk"`
	Runtime          G1RoslynRuntime          `json:"runtime"`
	ToolSHA256       string                   `json:"tool_sha256"`
	Assemblies       []G1RoslynAssembly       `json:"assemblies"`
	LanguageVersions G1RoslynLanguageVersions `json:"language_versions"`
	Parse            G1RoslynParse            `json:"parse"`
}

type G1RoslynDirective struct {
	Kind      string `json:"kind"`
	Start     int    `json:"start"`
	Active    bool   `json:"active"`
	Taken     *bool  `json:"taken"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
}

type G1RoslynRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type G1RoslynDiagnostic struct {
	ID      string `json:"id"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Context string `json:"context"`
}

type G1RoslynVersionDiagnostic struct {
	ID    string `json:"id"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type G1RoslynName struct {
	Text  string `json:"text"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type G1RoslynDeclaration struct {
	Kind  string         `json:"kind"`
	Start int            `json:"start"`
	End   int            `json:"end"`
	Names []G1RoslynName `json:"names"`
}

type G1RoslynVersionObservation struct {
	Version     string                      `json:"version"`
	Accepted    *bool                       `json:"accepted,omitzero"`
	Diagnostics []G1RoslynVersionDiagnostic `json:"diagnostics,omitzero"`
	OracleError string                      `json:"oracle_error,omitzero"`
}

// G1RoslynSource is the observation of one source version, named "<case>#<step>". When Roslyn
// cannot enumerate the diagnostics (OracleError) nothing else is observed.
type G1RoslynSource struct {
	ID          string                       `json:"id"`
	SHA256      string                       `json:"sha256"`
	Bytes       int                          `json:"bytes"`
	OracleError string                       `json:"oracle_error,omitzero"`
	Accepted    *bool                        `json:"accepted,omitzero"`
	Features    []string                     `json:"features,omitzero"`
	Directives  []G1RoslynDirective          `json:"directives,omitzero"`
	Disabled    []G1RoslynRange              `json:"disabled,omitzero"`
	Errors      []string                     `json:"errors,omitzero"`
	Diagnostics []G1RoslynDiagnostic         `json:"diagnostics,omitzero"`
	Declaration []G1RoslynDeclaration        `json:"declarations,omitzero"`
	Nodes       [][3]int                     `json:"nodes,omitzero"`
	Versions    []G1RoslynVersionObservation `json:"versions,omitzero"`
}

// CSharpG1RoslynObserved is the decoded tsgk-csharp-g1-roslyn-observed/r1 asset.
type CSharpG1RoslynObserved struct {
	Schema     string                `json:"schema"`
	Ledger     queryregistrycore.Pin `json:"ledger"`
	Input      G1RoslynInputPin      `json:"input"`
	Provenance G1RoslynProvenance    `json:"provenance"`
	Kinds      []string              `json:"kinds"`
	Sources    []G1RoslynSource      `json:"sources"`
}

// G1RoslynCaseSource is one source version of a ledger case: the unit the tool observes.
type G1RoslynCaseSource struct {
	ID     string // "<case>#<step>"
	Case   string
	Step   int
	SHA256 string
	Data   []byte
}

func g1rFault(code string) error { return fail(KindInvalidInput, code, CSharpG1RoslynPath, nil) }

// CSharpG1RoslynSources lists the source versions of the ledger's cases (the original source and
// the source after each edit), in ledger case order. The ledger must be derivable from src.
func CSharpG1RoslynSources(src CSharpG1Sources) ([]G1RoslynCaseSource, error) {
	ledger, data, err := deriveCSharpG1(src)
	if err != nil {
		return nil, err
	}
	var out []G1RoslynCaseSource
	for _, c := range ledger.Cases {
		d := data[c.ID]
		if d == nil {
			return nil, g1rFault("G1_ROSLYN_CASE")
		}
		for step, v := range d.versions {
			out = append(out, G1RoslynCaseSource{ID: fmt.Sprintf("%s#%d", c.ID, step), Case: c.ID, Step: step, SHA256: digestHex(v), Data: v})
		}
	}
	return out, nil
}

type g1rInputCase struct {
	ID      string   `json:"id"`
	Source  string   `json:"source"`
	Symbols []string `json:"symbols"`
}

// MarshalCSharpG1RoslynInput writes the tool's input document: one case per source version, in the
// shape of src/testdata/csharp-pp/cases.json, with no preprocessor symbols (the default mode of the
// route cases).
func MarshalCSharpG1RoslynInput(sources []G1RoslynCaseSource) ([]byte, error) {
	doc := struct {
		Schema string         `json:"schema"`
		Cases  []g1rInputCase `json:"cases"`
	}{Schema: CSharpG1RoslynInputSchema, Cases: make([]g1rInputCase, 0, len(sources))}
	for _, s := range sources {
		doc.Cases = append(doc.Cases, g1rInputCase{ID: s.ID, Source: base64.StdEncoding.EncodeToString(s.Data), Symbols: []string{}})
	}
	b, err := json.Marshal(doc)
	if err == nil && len(b) > g1RoslynInputMaxBytes {
		return nil, fail(KindResourceLimit, "G1_ROSLYN_INPUT_SIZE", CSharpG1RoslynPath, nil)
	}
	return b, err
}

// ParseCSharpG1RoslynObserved decodes the asset and requires it to describe exactly the source
// versions of the ledger's cases. ledger is the tracked ledger asset; it must itself verify against src.
func ParseCSharpG1RoslynObserved(data, ledger []byte, src CSharpG1Sources) (CSharpG1RoslynObserved, error) {
	if _, err := ParseCSharpG1Ledger(ledger, src); err != nil {
		return CSharpG1RoslynObserved{}, err
	}
	sources, err := CSharpG1RoslynSources(src)
	if err != nil {
		return CSharpG1RoslynObserved{}, err
	}
	input, err := MarshalCSharpG1RoslynInput(sources)
	if err != nil {
		return CSharpG1RoslynObserved{}, err
	}
	return checkCSharpG1Roslyn(data, queryregistrycore.FilePin(CSharpG1LedgerPath, ledger), input, sources)
}

func checkCSharpG1Roslyn(data []byte, ledger queryregistrycore.Pin, input []byte, sources []G1RoslynCaseSource) (CSharpG1RoslynObserved, error) {
	var x CSharpG1RoslynObserved
	if len(data) > csharpG1RoslynMaxBytes {
		return x, fail(KindResourceLimit, "G1_ROSLYN_SIZE", CSharpG1RoslynPath, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_ROSLYN_JSON", CSharpG1RoslynPath, err)
	}
	if x.Schema != CSharpG1RoslynSchema {
		return x, g1rFault("G1_ROSLYN_SCHEMA")
	}
	if x.Ledger != ledger {
		return x, g1rFault("G1_ROSLYN_LEDGER_PIN")
	}
	if x.Input != (G1RoslynInputPin{Bytes: uint64(len(input)), SHA256: digestHex(input)}) {
		return x, g1rFault("G1_ROSLYN_INPUT_PIN")
	}
	if err := g1rCheckProvenance(x.Provenance); err != nil {
		return x, err
	}
	if len(x.Kinds) == 0 || !slices.IsSorted(x.Kinds) {
		return x, g1rFault("G1_ROSLYN_KINDS")
	}
	for i, k := range x.Kinds {
		if !g1rKind.MatchString(k) || i > 0 && x.Kinds[i-1] == k {
			return x, g1rFault("G1_ROSLYN_KINDS")
		}
	}
	if len(x.Sources) != len(sources) {
		return x, g1rFault("G1_ROSLYN_SOURCE_SET")
	}
	for i, s := range sources {
		o := x.Sources[i]
		if o.ID != s.ID || o.SHA256 != s.SHA256 || o.Bytes != len(s.Data) {
			return x, g1rFault("G1_ROSLYN_SOURCE_SET")
		}
		if err := g1rCheckSource(x, o, s.Data); err != nil {
			return x, err
		}
	}
	return x, nil
}

func g1rCheckProvenance(p G1RoslynProvenance) error {
	bad := func() error { return g1rFault("G1_ROSLYN_PROVENANCE") }
	if !g1rSDK.MatchString(p.SDK) || p.Runtime.Framework == "" || p.Runtime.Version == "" || !g1rHex.MatchString(p.ToolSHA256) {
		return bad()
	}
	if p.Parse != (G1RoslynParse{DocumentationMode: "Parse", SourceCodeKind: "Regular"}) {
		return bad()
	}
	if len(p.Assemblies) != len(csharpG1RoslynAssemblies) {
		return bad()
	}
	for i, a := range p.Assemblies {
		// the assemblies of one SDK share a product version; the path names that SDK
		wantPath := "sdk/" + p.SDK + "/Roslyn/bincore/" + a.Name + ".dll"
		if a.Name != csharpG1RoslynAssemblies[i] || a.Path != wantPath || a.Bytes == 0 || !g1rHex.MatchString(a.SHA256) || a.AssemblyVersion == "" || a.ProductVersion == "" || a.ProductVersion != p.Assemblies[0].ProductVersion {
			return bad()
		}
	}
	v := p.LanguageVersions
	if len(v.Enumeration) == 0 || len(v.Observed) == 0 {
		return bad()
	}
	values := map[string]int{}
	for i, e := range v.Enumeration {
		if _, dup := values[e.Name]; dup || e.Name == "" {
			return bad()
		}
		values[e.Name] = e.Value
		if i > 0 {
			prev := v.Enumeration[i-1]
			if prev.Value > e.Value || prev.Value == e.Value && prev.Name >= e.Name {
				return bad()
			}
		}
	}
	last := 0
	for _, name := range v.Observed {
		val, ok := values[name]
		if !ok || !g1rVersion.MatchString(name) || val < g1RoslynMinVersion || val <= last {
			return bad()
		}
		last = val
	}
	if val, ok := values[v.Latest.Effective]; !ok || val != v.Latest.Value || !slices.Contains(v.Observed, v.Latest.Effective) {
		return bad()
	}
	return nil
}

func g1rSpan(start, end, size int) bool { return 0 <= start && start <= end && end <= size }

func g1rCheckSource(x CSharpG1RoslynObserved, o G1RoslynSource, src []byte) error {
	bad := func(code string) error { return g1rFault("G1_ROSLYN_" + code) }
	size := len(src)
	if o.OracleError != "" {
		if o.Accepted != nil || o.Features != nil || o.Directives != nil || o.Disabled != nil || o.Errors != nil || o.Diagnostics != nil || o.Declaration != nil || o.Nodes != nil || o.Versions != nil {
			return bad("ORACLE_ERROR")
		}
		return nil
	}
	if o.Accepted == nil || o.Features == nil || o.Directives == nil || o.Disabled == nil || o.Errors == nil || o.Diagnostics == nil || o.Declaration == nil || o.Nodes == nil || o.Versions == nil {
		return bad("OBSERVATION")
	}
	// diagnostics: errors only, so acceptance is the absence of any
	ids := []string{}
	for _, d := range o.Diagnostics {
		if !g1rSpan(d.Start, d.End, size) || !g1rDiagID.MatchString(d.ID) || !g1rContext.MatchString(d.Context) {
			return bad("DIAGNOSTIC")
		}
		ids = append(ids, d.ID)
	}
	slices.Sort(ids)
	if !slices.Equal(slices.Compact(ids), o.Errors) {
		return bad("DIAGNOSTIC")
	}
	if *o.Accepted != (len(o.Diagnostics) == 0) {
		return bad("DIAGNOSTIC")
	}
	for _, d := range o.Directives {
		if !g1rDirKind.MatchString(d.Kind) || !g1rSpan(d.Start, d.Start, size) || d.Start >= size || src[d.Start] != '#' || d.LineStart > d.Start || d.LineEnd < d.Start || d.LineEnd > size {
			return bad("DIRECTIVE")
		}
	}
	prev := 0
	for _, r := range o.Disabled {
		if !g1rSpan(r.Start, r.End, size) || r.Start < prev {
			return bad("DIRECTIVE")
		}
		prev = r.End
	}
	// nodes: the first is the compilation unit and spans nest in preorder (no partial overlap, so starts never decrease)
	type open struct{ start, end int }
	var stack []open
	type nodeKey struct{ kind, start, end int }
	var keys []nodeKey
	if len(o.Nodes) == 0 {
		return bad("NODES")
	}
	for i, n := range o.Nodes {
		if n[0] < 0 || n[0] >= len(x.Kinds) || !g1rSpan(n[1], n[2], size) || i == 0 != (x.Kinds[n[0]] == "CompilationUnit") {
			return bad("NODES")
		}
		// a node that starts at or after the end of the open one is not its child (the root stays open); one that starts inside must end inside
		for len(stack) > 1 && stack[len(stack)-1].end <= n[1] {
			stack = stack[:len(stack)-1]
		}
		if i > 0 && (n[1] < stack[len(stack)-1].start || n[2] > stack[len(stack)-1].end) {
			return bad("NODES")
		}
		stack = append(stack, open{n[1], n[2]})
		keys = append(keys, nodeKey{n[0], n[1], n[2]})
	}
	// declarations: each is one of the nodes, in preorder, and its names are source text inside it
	at := 0
	for _, d := range o.Declaration {
		for at < len(keys) && !(x.Kinds[keys[at].kind] == d.Kind && keys[at].start == d.Start && keys[at].end == d.End) {
			at++
		}
		if at == len(keys) {
			return bad("DECLARATION")
		}
		at++
		for _, n := range d.Names {
			if n.Start >= n.End || !g1rSpan(n.Start, n.End, size) || n.Start < d.Start || n.End > d.End || n.Text != string(src[n.Start:n.End]) {
				return bad("DECLARATION")
			}
		}
	}
	// versions: the observed list in order; the effective Latest version agrees with the Latest parse
	if len(o.Versions) != len(x.Provenance.LanguageVersions.Observed) {
		return bad("VERSIONS")
	}
	for i, v := range o.Versions {
		if v.Version != x.Provenance.LanguageVersions.Observed[i] {
			return bad("VERSIONS")
		}
		if v.OracleError != "" {
			if v.Accepted != nil || v.Diagnostics != nil {
				return bad("VERSIONS")
			}
			continue
		}
		if v.Accepted == nil || *v.Accepted != (len(v.Diagnostics) == 0) {
			return bad("VERSIONS")
		}
		for _, d := range v.Diagnostics {
			if !g1rSpan(d.Start, d.End, size) || !g1rDiagID.MatchString(d.ID) {
				return bad("VERSIONS")
			}
		}
		if v.Version == x.Provenance.LanguageVersions.Latest.Effective {
			if len(v.Diagnostics) != len(o.Diagnostics) {
				return bad("VERSIONS")
			}
			for j, d := range v.Diagnostics {
				m := o.Diagnostics[j]
				if d.ID != m.ID || d.Start != m.Start || d.End != m.End {
					return bad("VERSIONS")
				}
			}
		}
	}
	return nil
}
