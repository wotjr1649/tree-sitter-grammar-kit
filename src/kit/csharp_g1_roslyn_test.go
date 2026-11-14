package kit

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
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
		if a.data, err = os.ReadFile("../contracts/csharp-g1-roslyn-observed-r2.json"); err != nil {
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
	t.Parallel()
	a := g1rLoad(t)
	if len(a.data) != 2142812 || digestHex(a.data) != "8930422a2b06743e22ffe47c5189447974c7567e6f0c0ff6943417170335c39f" {
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
	accepted, tokens, missing := 0, 0, 0
	for _, s := range x.Sources {
		if s.OracleError != "" {
			t.Errorf("%s: no observation", s.ID)
		}
		if *s.Accepted {
			accepted++
		}
		tokens += len(s.Tokens)
		missing += len(s.Missing)
	}
	if accepted != 361 {
		t.Errorf("accepted %d, want 361", accepted)
	}
	// the sealed asset records tokens and Roslyn's missing marker (the SHA-256 above fixes the exact values)
	if tokens != 44144 || missing != 80 {
		t.Errorf("%d tokens, %d missing, want 44144 and 80", tokens, missing)
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
	t.Parallel()
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
	otherKind, notEOF := -1, -1
	for i, k := range base.Kinds {
		if k == "CompilationUnit" {
			otherKind = (i + 1) % len(base.Kinds)
		}
		if k != "EndOfFileToken" && notEOF < 0 {
			notEOF = i
		}
	}
	twoMissing := g1rFirst(t, base, "two missing tokens", func(s G1RoslynSource) bool { return len(s.Missing) > 1 })
	acceptedSource := g1rFirst(t, base, "acceptance", func(s G1RoslynSource) bool { return *s.Accepted })
	// a source whose first token has width and that has no missing token: dropping that token leaves the compilation unit without a token at its start
	plain := g1rFirst(t, base, "a first token with width", func(s G1RoslynSource) bool { return len(s.Missing) == 0 && s.Tokens[0][2] > s.Tokens[0][1] })
	// a token with width in the rejected source, and one followed by another token
	wide := func(x *CSharpG1RoslynObserved, src int) int {
		for i, tk := range x.Sources[src].Tokens {
			if tk[2] > tk[1] && i+1 < len(x.Sources[src].Tokens) {
				return i
			}
		}
		t.Fatal("no token with width")
		return -1
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
		{"no tokens", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) { x.Sources[0].Tokens = [][3]int{} }},
		{"token kind index", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) { x.Sources[0].Tokens[0][0] = len(x.Kinds) }},
		{"token span", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) { x.Sources[0].Tokens[0][2] = x.Sources[0].Bytes + 1 }},
		{"tokens overlap", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) {
			i := wide(x, 0)
			x.Sources[0].Tokens[i+1][1] = x.Sources[0].Tokens[i][2] - 1
		}},
		{"token beyond the source", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) {
			// a second end-of-file token past the end: ordered, last of its kind, and no node boundary changes
			tk := x.Sources[0].Tokens
			x.Sources[0].Tokens = append(tk, [3]int{tk[len(tk)-1][0], x.Sources[0].Bytes + 1, x.Sources[0].Bytes + 1})
		}},
		{"token repeated", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) {
			// every boundary stays a token boundary; only the overlap is wrong
			i := wide(x, 0)
			tk := x.Sources[0].Tokens
			x.Sources[0].Tokens = append(tk[:i+1], tk[i:]...)
		}},
		{"tokens out of order", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) {
			i := wide(x, 0)
			tk := x.Sources[0].Tokens
			tk[i], tk[i+1] = tk[i+1], tk[i]
		}},
		{"last token is not the end of file", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) {
			tk := x.Sources[0].Tokens
			tk[len(tk)-1][0] = notEOF
		}},
		{"end-of-file token dropped", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) {
			tk := x.Sources[0].Tokens
			x.Sources[0].Tokens = tk[:len(tk)-1]
		}},
		{"missing token with width", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) { x.Sources[rejected].Missing = []int{wide(x, rejected)} }},
		{"missing index beyond the tokens", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) { x.Sources[rejected].Missing = []int{len(x.Sources[rejected].Tokens)} }},
		{"missing indexes out of order", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) {
			m := x.Sources[twoMissing].Missing
			m[0], m[1] = m[1], m[0]
		}},
		{"missing token in an accepted source", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) {
			x.Sources[acceptedSource].Missing = []int{len(x.Sources[acceptedSource].Tokens) - 1} // the end-of-file token has no width
		}},
		{"node does not start at a token", "G1_ROSLYN_TOKENS", func(x *CSharpG1RoslynObserved) { x.Sources[plain].Tokens = x.Sources[plain].Tokens[1:] }},
		{"oracle error with data", "G1_ROSLYN_ORACLE_ERROR", func(x *CSharpG1RoslynObserved) {
			x.Sources[0].OracleError = "GetDiagnostics ArgumentOutOfRangeException"
		}},
		{"observation member missing", "G1_ROSLYN_OBSERVATION", func(x *CSharpG1RoslynObserved) { x.Sources[0].Nodes = nil }},
		{"tokens member missing", "G1_ROSLYN_OBSERVATION", func(x *CSharpG1RoslynObserved) { x.Sources[0].Tokens = nil }},
		{"missing member missing", "G1_ROSLYN_OBSERVATION", func(x *CSharpG1RoslynObserved) { x.Sources[0].Missing = nil }},
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
	// a leaf node whose start or end moves off every token boundary: the node/token check, not the nesting check, rejects it
	for _, side := range []struct {
		name string
		col  int
		move int
	}{{"start", 1, 1}, {"end", 2, -1}} {
		rejectedByTokens := false
		nodes := base.Sources[0].Nodes
		for k := 1; k < len(nodes) && !rejectedByTokens; k++ {
			if k+1 < len(nodes) && nodes[k+1][1] < nodes[k][2] || nodes[k][2]-nodes[k][1] < 2 {
				continue // not a leaf, or too narrow to move inside
			}
			rejectedByTokens = a.mutated(t, func(x *CSharpG1RoslynObserved) { x.Sources[0].Nodes[k][side.col] += side.move }) == "G1_ROSLYN_TOKENS"
		}
		if !rejectedByTokens {
			t.Errorf("no node %s moved off a token boundary was rejected", side.name)
		}
	}
}

func TestCSharpG1RoslynRejectsMalformedBytes(t *testing.T) {
	t.Parallel()
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
	if _, err := ParseCSharpG1RoslynObserved(a.data, bytes.Replace(a.ledger, []byte(`"NO_EXPECTED"`), []byte(`"SEALED"`), 1), a.src); g1Code(err) != "G1_LEDGER_ROWS" {
		t.Errorf("tampered ledger: %v", err)
	}
	// and an observation bound to another ledger revision
	other := bytes.Replace(a.ledger, []byte(`"basis": "`), []byte(`"basis": " `), 1)
	if _, err := ParseCSharpG1RoslynObserved(a.data, other, a.src); err == nil {
		t.Error("an observation of another ledger was accepted")
	}
}

// g1rInstallPathFlow reports every way an install path or an absolute path can reach an assembly attribute or
// property of the observer: in the csproj, $(RoslynDir) and $(NetCoreRoot) may only define the Roslyn directory and
// feed a Reference HintPath (restore and compile read it, the assembly does not carry it), and AssemblyMetadata is only
// the SDK version; in Observe.cs no assembly attribute may take a path, and no string literal is an absolute path.
func g1rInstallPathFlow(csproj, cs string) []string {
	var out []string
	if items := regexp.MustCompile(`<AssemblyMetadata\b[^>]*>`).FindAllString(csproj, -1); len(items) != 1 || !regexp.MustCompile(`^<AssemblyMetadata\s+Include="TSGK\.SDK"\s+Value="\$\(NETCoreSdkVersion\)"\s*/>$`).MatchString(items[0]) {
		out = append(out, "AssemblyMetadata must be exactly TSGK.SDK = $(NETCoreSdkVersion)")
	}
	pathProp := regexp.MustCompile(`\$\((RoslynDir|NetCoreRoot|DOTNET_[A-Za-z_]*|MSBuild[A-Za-z]*|ProjectDir)\)`)
	for _, line := range strings.Split(csproj, "\n") {
		t := strings.TrimSpace(line)
		if pathProp.MatchString(t) && !strings.HasPrefix(t, "<Reference ") && !strings.HasPrefix(t, "<RoslynDir ") {
			out = append(out, "csproj line takes a path outside a Reference or the RoslynDir definition: "+t)
		}
	}
	if regexp.MustCompile(`GetFullPath|\b[A-Za-z]:[\\/]|"/(usr|home|opt|Users|mnt|var)/`).MatchString(csproj) {
		out = append(out, "csproj holds GetFullPath or an absolute path literal")
	}
	for _, attr := range regexp.MustCompile(`(?m)^\s*\[\s*assembly\s*:.*$`).FindAllString(cs, -1) {
		if regexp.MustCompile(`GetFullPath|roslynDir|RoslynDir|Location|BaseDirectory|CurrentDirectory|\b[A-Za-z]:[\\/]|"/`).MatchString(attr) {
			out = append(out, "assembly attribute takes a path: "+attr)
		}
	}
	for _, lit := range regexp.MustCompile(`"(?:[^"\\\n]|\\.)*"`).FindAllString(cs, -1) {
		if regexp.MustCompile(`^"(?:[A-Za-z]:[\\/]|(?:\\\\|/)(?:usr|home|opt|Users|mnt|var|Program)\b)`).MatchString(lit) {
			out = append(out, "Observe.cs holds an absolute path literal: "+lit)
		}
	}
	return out
}

// TestCSharpG1ObserverCarriesNoInstallPath: the observer's assembly hash (tool_sha256) must not depend on where
// dotnet is installed, so the only assembly metadata is the SDK version string (not a path); the Roslyn
// directory is found at run time. A local check (two directory junctions to one SDK build the same Observe.dll)
// is in the csharp-g1-ledger spec.
func TestCSharpG1ObserverCarriesNoInstallPath(t *testing.T) {
	csproj, cs := string(mustRead(t, "../dev/csharp-g1-roslyn/Observe.csproj")), string(mustRead(t, "../dev/csharp-g1-roslyn/Observe.cs"))
	if got := g1rInstallPathFlow(csproj, cs); len(got) != 0 {
		t.Errorf("the observer assembly can carry an install path: %q", got)
	}
	// mutants: each way for a path to reach the assembly must be reported
	for name, m := range map[string][2]string{
		"description from RoslynDir":  {strings.Replace(csproj, "<AssemblyMetadata Include=", "<Description>$(RoslynDir)</Description><AssemblyMetadata Include=", 1), cs},
		"second AssemblyMetadata":     {strings.Replace(csproj, "<Reference ", `<AssemblyMetadata Include="TSGK.Dir" Value="$(RoslynDir)" /><Reference `, 1), cs},
		"informational version path":  {strings.Replace(csproj, "<DebugType>", "<InformationalVersion>$(NetCoreRoot)</InformationalVersion><DebugType>", 1), cs},
		"absolute path in the csproj": {strings.Replace(csproj, "<DebugType>", `<Description>C:\dotnet\</Description><DebugType>`, 1), cs},
		"assembly attribute path":     {csproj, cs + "\n[assembly: System.Reflection.AssemblyMetadata(\"dir\", System.IO.Path.GetFullPath(\".\"))]\n"},
		"assembly attribute literal":  {csproj, cs + "\n[assembly: System.Reflection.AssemblyDescription(\"C:\\\\dotnet\")]\n"},
		"absolute path literal":       {csproj, cs + "\nclass Z { string p = \"/usr/share/dotnet\"; }\n"},
	} {
		if len(g1rInstallPathFlow(m[0], m[1])) == 0 {
			t.Errorf("mutant %q was not reported", name)
		}
	}
}

const (
	g1rR3SHA = "01b220ba178af7457e42994a3d3be6af6f8401c29c0083d004c981f1084e6e71"
	// the observer source a clean build of which gives CSharpG1RoslynR3ToolSHA256 (Release, --no-incremental,
	// UseSharedCompilation=false, SDK 10.0.401; the source as of commit a12bd3c). A change of the source needs a new
	// build, a new tool hash and a new observation revision.
	g1rObserverCsSHA     = "b166185ef8d3eeed7a3b7f257f65c5328e0171a0235d0676983cc7226c14a5e1"
	g1rObserverCsprojSHA = "efeae420c28e37ef9011ace82370ee0e6526b4f675bfe86284c0011bbffc1bc7"
	g1rObserverGlobalSHA = "9969064a379ad96f0fd9358b4133bf2652018a32c60d44f37a0638168381a90d"
)

// TestCSharpG1RoslynR3IsR2WithTheBuildHash: r3 is the observation the tooling writes now; the frozen r2 stays as it
// is and r3 differs from it in provenance.tool_sha256 only (an independent byte-level comparison besides the verifier's).
func TestCSharpG1RoslynR3IsR2WithTheBuildHash(t *testing.T) {
	t.Parallel()
	a := g1rLoad(t)
	r3, err := os.ReadFile("../contracts/csharp-g1-roslyn-observed-r3.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(r3) != 2142812 || digestHex(r3) != g1rR3SHA {
		t.Fatalf("r3 observation changed: %d/%s", len(r3), digestHex(r3))
	}
	x, err := ParseCSharpG1RoslynObservedR3(r3, a.data, a.ledger, a.src)
	if err != nil {
		t.Fatal(err)
	}
	if x.Schema != CSharpG1RoslynSchema {
		t.Errorf("schema %q", x.Schema)
	}
	old := a.decode(t)
	if old.Provenance.ToolSHA256 == x.Provenance.ToolSHA256 || x.Provenance.ToolSHA256 != CSharpG1RoslynR3ToolSHA256 {
		t.Errorf("tool hashes: r2 %s, r3 %s", old.Provenance.ToolSHA256, x.Provenance.ToolSHA256)
	}
	// byte level: replacing the one hash in r2 gives r3 exactly
	if !bytes.Equal(bytes.Replace(a.data, []byte(old.Provenance.ToolSHA256), []byte(CSharpG1RoslynR3ToolSHA256), -1), r3) || bytes.Count(a.data, []byte(old.Provenance.ToolSHA256)) != 1 {
		t.Error("r3 differs from r2 by more than provenance.tool_sha256")
	}
	// the hash is the one of the tracked observer source, so a changed source cannot keep this r3
	for file, want := range map[string]string{"Observe.cs": g1rObserverCsSHA, "Observe.csproj": g1rObserverCsprojSHA, "global.json": g1rObserverGlobalSHA} {
		if got := digestHex(mustRead(t, "../dev/csharp-g1-roslyn/"+file)); got != want {
			t.Errorf("%s changed (%s): build it again and write a new observation revision", file, got)
		}
	}

	check := func(name string, data []byte, want string) {
		t.Helper()
		_, err := ParseCSharpG1RoslynObservedR3(data, a.data, a.ledger, a.src)
		if got := g1Code(err); got != want {
			t.Errorf("%s: code %q, want %q (%v)", name, got, want, err)
		}
	}
	// mutants: another changed value, another tool hash, r2 given as r3
	var m CSharpG1RoslynObserved = x
	m.Provenance.Runtime.Version += ".1"
	moved, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	check("another changed field", moved, "G1_ROSLYN_R3_DIFFERS")
	check("another tool hash", bytes.Replace(r3, []byte(CSharpG1RoslynR3ToolSHA256), []byte(strings.Repeat("0", 64)), 1), "G1_ROSLYN_R3_TOOL")
	check("r2 as r3", a.data, "G1_ROSLYN_R3_TOOL")
	check("r3 not a valid observation", r3[:len(r3)/2], "G1_ROSLYN_JSON")
}
