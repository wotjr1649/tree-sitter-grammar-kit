package kit

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

type g1rAssets struct {
	data    []byte
	ledger  []byte
	pin     queryregistrycore.Pin
	input   []byte
	sources []G1RoslynCaseSource
	src     CSharpG1Sources
}

var g1rOnce sync.Once
var g1rLoaded g1rAssets

// g1rLoad reads the tracked observation asset and derives what it must bind to (the ledger pin, the
// tool input and the case source versions) once.
func g1rLoad(t *testing.T) g1rAssets {
	t.Helper()
	g1rOnce.Do(func() {
		l := g1Load(t)
		a := &g1rLoaded
		var err error
		if a.data, err = os.ReadFile("../contracts/csharp-g1-roslyn-observed-r1.json"); err != nil {
			t.Fatal(err)
		}
		a.ledger, a.src = l.asset, l.src
		a.pin = queryregistrycore.FilePin(CSharpG1LedgerPath, a.ledger)
		if a.sources, err = CSharpG1RoslynSources(a.src); err != nil {
			t.Fatal(err)
		}
		if a.input, err = MarshalCSharpG1RoslynInput(a.sources); err != nil {
			t.Fatal(err)
		}
	})
	if g1rLoaded.data == nil {
		t.Fatal("observation asset did not load")
	}
	return g1rLoaded
}

func (a g1rAssets) decode(t *testing.T) CSharpG1RoslynObserved {
	t.Helper()
	var x CSharpG1RoslynObserved
	if err := jsonv2.Unmarshal(a.data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	return x
}

// g1rMutated returns the verifier's code for the asset after mutate.
func (a g1rAssets) mutated(t *testing.T, mutate func(x *CSharpG1RoslynObserved)) string {
	t.Helper()
	x := a.decode(t)
	mutate(&x)
	data, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	_, err = checkCSharpG1Roslyn(data, a.pin, a.input, a.sources)
	return g1Code(err)
}

// first returns the index of the first source that satisfies ok.
func g1rFirst(t *testing.T, x CSharpG1RoslynObserved, what string, ok func(s G1RoslynSource) bool) int {
	t.Helper()
	for i, s := range x.Sources {
		if s.OracleError == "" && ok(s) {
			return i
		}
	}
	t.Fatalf("no source with %s", what)
	return -1
}

func TestCSharpG1RoslynAssetSealedAndBound(t *testing.T) {
	a := g1rLoad(t)
	if len(a.data) != 1500932 || digestHex(a.data) != "d5e3be3f2cbd864ecc8032d9b849fd60d728c5b5f4f539d05c3ca0a205ef8c95" {
		t.Fatalf("sealed observation changed: %d/%s", len(a.data), digestHex(a.data))
	}
	x, err := checkCSharpG1Roslyn(a.data, a.pin, a.input, a.sources)
	if err != nil {
		t.Fatal(err)
	}
	// the public entry point derives the same bindings from the repository declarations
	if _, err := ParseCSharpG1RoslynObserved(a.data, a.ledger, a.src); err != nil {
		t.Fatal(err)
	}
	if len(x.Sources) != 514 || len(a.sources) != 514 {
		t.Fatalf("%d observed, %d derived sources", len(x.Sources), len(a.sources))
	}
	accepted := 0
	for _, s := range x.Sources {
		if s.OracleError != "" {
			t.Errorf("%s: no observation", s.ID)
		}
		if *s.Accepted {
			accepted++
		}
	}
	if accepted != 361 {
		t.Errorf("accepted %d, want 361", accepted)
	}
	p := x.Provenance
	if len(p.Assemblies) != 2 || strings.Join(p.LanguageVersions.Observed, " ") != "CSharp7_3 CSharp8 CSharp9 CSharp10 CSharp11 CSharp12 CSharp13 CSharp14" || p.LanguageVersions.Latest.Effective != "CSharp14" {
		t.Errorf("provenance %+v", p)
	}
	// every ledger case step is observed under the id "<case>#<step>", edits included
	ledger, err := ParseCSharpG1Ledger(a.ledger, a.src)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, s := range x.Sources {
		seen[s.ID] = true
	}
	for _, c := range ledger.Cases {
		for _, s := range c.Steps {
			if id := c.ID + "#" + string(rune('0'+s.Step)); s.Step < 10 && !seen[id] {
				t.Errorf("ledger step %s is not observed", id)
			}
		}
	}
}

func TestCSharpG1RoslynInputIsDerivedAndStable(t *testing.T) {
	a := g1rLoad(t)
	again, err := MarshalCSharpG1RoslynInput(a.sources)
	if err != nil || !bytes.Equal(again, a.input) {
		t.Fatalf("input is not stable: %v", err)
	}
	var doc struct {
		Schema string `json:"schema"`
		Cases  []struct {
			ID      string   `json:"id"`
			Symbols []string `json:"symbols"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(a.input, &doc); err != nil || doc.Schema != CSharpG1RoslynInputSchema || len(doc.Cases) != len(a.sources) {
		t.Fatalf("input document: %v", err)
	}
	ids := map[string]bool{}
	edits := 0
	for i, c := range doc.Cases {
		if ids[c.ID] || c.ID != a.sources[i].ID || len(c.Symbols) != 0 || digestHex(a.sources[i].Data) != a.sources[i].SHA256 {
			t.Fatalf("case %d %s", i, c.ID)
		}
		ids[c.ID] = true
		if a.sources[i].Step > 0 {
			edits++
		}
	}
	if edits == 0 {
		t.Error("no edited source version is observed")
	}
}

func TestCSharpG1RoslynRejectsTampering(t *testing.T) {
	a := g1rLoad(t)
	base := a.decode(t)
	// the sources the mutations act on
	rejected := g1rFirst(t, base, "diagnostics", func(s G1RoslynSource) bool { return len(s.Diagnostics) > 0 })
	withDirective := g1rFirst(t, base, "a directive", func(s G1RoslynSource) bool { return len(s.Directives) > 0 })
	withDisabled := g1rFirst(t, base, "two disabled ranges", func(s G1RoslynSource) bool { return len(s.Disabled) > 1 })
	twoDiagnostics := g1rFirst(t, base, "two diagnostics", func(s G1RoslynSource) bool { return len(s.Diagnostics) > 1 })
	withDecl := g1rFirst(t, base, "a named declaration", func(s G1RoslynSource) bool {
		return len(s.Declaration) > 0 && len(s.Declaration[0].Names) > 0
	})
	otherKind := -1
	for i, k := range base.Kinds {
		if k == "CompilationUnit" {
			otherKind = (i + 1) % len(base.Kinds)
		}
	}
	for _, tc := range []struct {
		name   string
		code   string
		mutate func(x *CSharpG1RoslynObserved)
	}{
		{"schema", "G1_ROSLYN_SCHEMA", func(x *CSharpG1RoslynObserved) { x.Schema += "x" }},
		{"ledger path", "G1_ROSLYN_LEDGER_PIN", func(x *CSharpG1RoslynObserved) { x.Ledger.Path = "src/contracts/other.json" }},
		{"ledger bytes", "G1_ROSLYN_LEDGER_PIN", func(x *CSharpG1RoslynObserved) { x.Ledger.Bytes++ }},
		{"ledger sha", "G1_ROSLYN_LEDGER_PIN", func(x *CSharpG1RoslynObserved) { x.Ledger.SHA256 = strings.Repeat("0", 64) }},
		{"input bytes", "G1_ROSLYN_INPUT_PIN", func(x *CSharpG1RoslynObserved) { x.Input.Bytes++ }},
		{"input sha", "G1_ROSLYN_INPUT_PIN", func(x *CSharpG1RoslynObserved) { x.Input.SHA256 = strings.Repeat("0", 64) }},
		{"sdk format", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) {
			// consistent paths, so only the SDK format can reject it
			x.Provenance.SDK = "latest"
			for i := range x.Provenance.Assemblies {
				x.Provenance.Assemblies[i].Path = "sdk/latest/Roslyn/bincore/" + x.Provenance.Assemblies[i].Name + ".dll"
			}
		}},
		{"sdk differs from the assembly paths", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) { x.Provenance.SDK = "2" + x.Provenance.SDK[1:] }}, // same format, another SDK than the paths name
		{"runtime", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) { x.Provenance.Runtime.Framework = "" }},
		{"tool sha not hex", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) { x.Provenance.ToolSHA256 = "ZZ" }},
		{"parse options", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) { x.Provenance.Parse.SourceCodeKind = "Script" }},
		{"assembly dropped", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) { x.Provenance.Assemblies = x.Provenance.Assemblies[:1] }},
		{"assembly sha not hex", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) { x.Provenance.Assemblies[1].SHA256 = "ZZ" }},
		{"assembly bytes", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) { x.Provenance.Assemblies[0].Bytes = 0 }},
		{"assembly path", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) {
			x.Provenance.Assemblies[0].Path += "x"
		}},
		{"assembly product versions differ", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) { x.Provenance.Assemblies[1].ProductVersion += "x" }},
		{"assembly order", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) {
			a := x.Provenance.Assemblies
			a[0], a[1] = a[1], a[0]
		}},
		{"observed version dropped", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) {
			v := &x.Provenance.LanguageVersions
			v.Observed = v.Observed[:len(v.Observed)-1]
		}},
		{"observed version unknown", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) {
			v := &x.Provenance.LanguageVersions
			v.Observed[0] = "CSharp7_4"
		}},
		{"observed below 7.3", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) {
			v := &x.Provenance.LanguageVersions
			v.Observed = append([]string{"CSharp7_2"}, v.Observed...)
		}},
		{"latest effective", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) { x.Provenance.LanguageVersions.Latest.Value++ }},
		{"enumeration order", "G1_ROSLYN_PROVENANCE", func(x *CSharpG1RoslynObserved) {
			e := x.Provenance.LanguageVersions.Enumeration
			e[0], e[1] = e[1], e[0]
		}},
		{"kinds unsorted", "G1_ROSLYN_KINDS", func(x *CSharpG1RoslynObserved) { x.Kinds[0], x.Kinds[1] = x.Kinds[1], x.Kinds[0] }},
		{"kinds repeated", "G1_ROSLYN_KINDS", func(x *CSharpG1RoslynObserved) { x.Kinds[1] = x.Kinds[0] }},
		{"source dropped", "G1_ROSLYN_SOURCE_SET", func(x *CSharpG1RoslynObserved) { x.Sources = x.Sources[:len(x.Sources)-1] }},
		{"source added", "G1_ROSLYN_SOURCE_SET", func(x *CSharpG1RoslynObserved) { x.Sources = append(x.Sources, x.Sources[0]) }},
		{"source swapped", "G1_ROSLYN_SOURCE_SET", func(x *CSharpG1RoslynObserved) { x.Sources[0], x.Sources[1] = x.Sources[1], x.Sources[0] }},
		{"source renamed", "G1_ROSLYN_SOURCE_SET", func(x *CSharpG1RoslynObserved) { x.Sources[3].ID += "x" }},
		{"source sha", "G1_ROSLYN_SOURCE_SET", func(x *CSharpG1RoslynObserved) { x.Sources[3].SHA256 = strings.Repeat("2", 64) }},
		{"source bytes", "G1_ROSLYN_SOURCE_SET", func(x *CSharpG1RoslynObserved) { x.Sources[3].Bytes++ }},
		{"accepted flipped", "G1_ROSLYN_DIAGNOSTIC", func(x *CSharpG1RoslynObserved) { *x.Sources[rejected].Accepted = true }},
		{"errors list", "G1_ROSLYN_DIAGNOSTIC", func(x *CSharpG1RoslynObserved) { x.Sources[rejected].Errors = []string{"CS0000"} }},
		{"diagnostic span", "G1_ROSLYN_DIAGNOSTIC", func(x *CSharpG1RoslynObserved) {
			x.Sources[rejected].Diagnostics[0].End = x.Sources[rejected].Bytes + 1
		}},
		{"diagnostic id", "G1_ROSLYN_DIAGNOSTIC", func(x *CSharpG1RoslynObserved) { x.Sources[rejected].Diagnostics[0].ID = "bad id" }},
		{"directive not at #", "G1_ROSLYN_DIRECTIVE", func(x *CSharpG1RoslynObserved) { x.Sources[withDirective].Directives[0].Start++ }},
		{"directive kind", "G1_ROSLYN_DIRECTIVE", func(x *CSharpG1RoslynObserved) { x.Sources[withDirective].Directives[0].Kind = "If" }},
		{"disabled order", "G1_ROSLYN_DIRECTIVE", func(x *CSharpG1RoslynObserved) {
			d := x.Sources[withDisabled].Disabled
			d[0], d[1] = d[1], d[0]
		}},
		{"first node", "G1_ROSLYN_NODES", func(x *CSharpG1RoslynObserved) { x.Sources[0].Nodes[0][0] = otherKind }},
		{"node kind index", "G1_ROSLYN_NODES", func(x *CSharpG1RoslynObserved) { x.Sources[0].Nodes[1][0] = len(x.Kinds) }},
		{"node span", "G1_ROSLYN_NODES", func(x *CSharpG1RoslynObserved) { x.Sources[0].Nodes[1][2] = x.Sources[0].Bytes + 1 }},
		{"node preorder", "G1_ROSLYN_NODES", func(x *CSharpG1RoslynObserved) {
			n := x.Sources[0].Nodes
			n[len(n)-1][1] = n[len(n)-2][1] - 1
		}},
		{"no nodes", "G1_ROSLYN_NODES", func(x *CSharpG1RoslynObserved) { x.Sources[0].Nodes = [][3]int{} }},
		{"declaration kind", "G1_ROSLYN_DECLARATION", func(x *CSharpG1RoslynObserved) { x.Sources[withDecl].Declaration[0].Kind = "NoSuchDeclaration" }},
		{"declaration span", "G1_ROSLYN_DECLARATION", func(x *CSharpG1RoslynObserved) { x.Sources[withDecl].Declaration[0].End++ }},
		{"declaration name text", "G1_ROSLYN_DECLARATION", func(x *CSharpG1RoslynObserved) { x.Sources[withDecl].Declaration[0].Names[0].Text += "x" }},
		{"declaration name outside", "G1_ROSLYN_DECLARATION", func(x *CSharpG1RoslynObserved) {
			d := &x.Sources[withDecl].Declaration[0]
			d.Names[0].End = d.End + 1
		}},
		{"versions dropped", "G1_ROSLYN_VERSIONS", func(x *CSharpG1RoslynObserved) { x.Sources[0].Versions = x.Sources[0].Versions[1:] }},
		{"version renamed", "G1_ROSLYN_VERSIONS", func(x *CSharpG1RoslynObserved) { x.Sources[0].Versions[0].Version = "CSharp7_2" }},
		{"version acceptance", "G1_ROSLYN_VERSIONS", func(x *CSharpG1RoslynObserved) {
			v := &x.Sources[rejected].Versions[0]
			*v.Accepted = !*v.Accepted
		}},
		{"latest version disagrees", "G1_ROSLYN_VERSIONS", func(x *CSharpG1RoslynObserved) {
			v := &x.Sources[rejected].Versions[len(x.Sources[rejected].Versions)-1]
			v.Diagnostics[0].Start++
		}},
		{"latest version lacks a diagnostic", "G1_ROSLYN_VERSIONS", func(x *CSharpG1RoslynObserved) {
			v := &x.Sources[twoDiagnostics].Versions[len(x.Sources[twoDiagnostics].Versions)-1]
			v.Diagnostics = v.Diagnostics[:1]
		}},
		{"oracle error with data", "G1_ROSLYN_ORACLE_ERROR", func(x *CSharpG1RoslynObserved) {
			x.Sources[0].OracleError = "GetDiagnostics ArgumentOutOfRangeException"
		}},
		{"observation member missing", "G1_ROSLYN_OBSERVATION", func(x *CSharpG1RoslynObserved) { x.Sources[0].Nodes = nil }},
		{"accepted missing", "G1_ROSLYN_OBSERVATION", func(x *CSharpG1RoslynObserved) { x.Sources[0].Accepted = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := a.mutated(t, tc.mutate); got != tc.code {
				t.Errorf("code %q, want %q", got, tc.code)
			}
		})
	}
	// a node that starts inside another and ends beyond it crosses it: some node of the first source must show that
	crossed := false
	for k := 1; k < len(base.Sources[0].Nodes) && !crossed; k++ {
		crossed = a.mutated(t, func(x *CSharpG1RoslynObserved) {
			n := x.Sources[0].Nodes
			n[k][2] = n[0][2] + 1
			n[k][2] = min(n[k][2], x.Sources[0].Bytes)
		}) == "G1_ROSLYN_NODES"
	}
	if !crossed {
		t.Error("no node extension was rejected as crossing")
	}
}

func TestCSharpG1RoslynRejectsMalformedBytes(t *testing.T) {
	a := g1rLoad(t)
	check := func(name string, data []byte, want string) {
		t.Helper()
		_, err := checkCSharpG1Roslyn(data, a.pin, a.input, a.sources)
		if got := g1Code(err); got != want {
			t.Errorf("%s: code %q, want %q (%v)", name, got, want, err)
		}
	}
	check("unknown member", bytes.Replace(a.data, []byte(`"kinds"`), []byte(`"extra": 1, "kinds"`), 1), "G1_ROSLYN_JSON")
	check("truncated", a.data[:len(a.data)/2], "G1_ROSLYN_JSON")
	check("empty", nil, "G1_ROSLYN_JSON")
	check("oversize", append(bytes.Clone(a.data), bytes.Repeat([]byte(" "), csharpG1RoslynMaxBytes)...), "G1_ROSLYN_SIZE")
	check("source member renamed", bytes.Replace(a.data, []byte(`"accepted"`), []byte(`"acceptd"`), 1), "G1_ROSLYN_JSON")

	// the public entry point rejects a ledger that is not the derivation of the sources
	if _, err := ParseCSharpG1RoslynObserved(a.data, bytes.Replace(a.ledger, []byte(`"NO_EXPECTED"`), []byte(`"SEALED"`), 1), a.src); g1Code(err) != "G1_LEDGER_STATUS" {
		t.Errorf("tampered ledger: %v", err)
	}
	// and an observation bound to another ledger revision
	other := bytes.Replace(a.ledger, []byte(`"basis": "`), []byte(`"basis": " `), 1)
	if _, err := ParseCSharpG1RoslynObserved(a.data, other, a.src); err == nil {
		t.Error("an observation of another ledger was accepted")
	}
}
