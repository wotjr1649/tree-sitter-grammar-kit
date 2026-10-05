package native

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func readJSON(t *testing.T, rel string, v any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
}

type registryFile struct {
	Path, Role, Origin, SHA256 string
	Bytes                      int64
}

type registryRoute struct {
	Route, Repository, Commit, Symbol, Source string
	GrammarDir                                string `json:"grammar_dir"`
	Files                                     []registryFile
	Regeneration                              *struct {
		Outputs      []registryFile
		Inputs       []registryFile
		Grammar      string
		ABI          int
		Optimize     bool
		PatchChain   []struct{ Subject, Pointer, Field, Target string } `json:"patch_chain"`
		PatchedFiles []registryFile                                     `json:"patched_files"`
	}
}

type nativeRegistry struct {
	Schema string
	Routes []registryRoute
}

type sourceRecord struct {
	RouteID  string `json:"route_id"`
	Adoption *struct {
		PatchedFiles []registryFile `json:"patched_files"`
	}
	Regeneration *struct {
		Outputs []registryFile
	}
}

type sourceRecords struct{ Routes []sourceRecord }

type reproRecord struct {
	Route            string
	PrepareGenerated map[string]registryFile `json:"prepare_generated"`
}

type reproRecords struct{ Routes []reproRecord }

// checkNativeRoutes accepts exactly two route classes: an adopted route (its literal patch
// chain ends at the adoption hashes, the reproduction reference pins parser.c) and a
// regenerated-only route (C2-REGENERATE-r1: no chain, no patched files, every output
// pinned by the source record and the reproduction reference). Any other route, including
// one built from the upstream generated files, is rejected.
func checkNativeRoutes(reg nativeRegistry, sources sourceRecords, repro reproRecords) error {
	if reg.Schema != "tsgk-native-routes/r1" || len(reg.Routes) != len(sources.Routes) || len(reg.Routes) != 26 {
		return fmt.Errorf("registry %s with %d routes", reg.Schema, len(reg.Routes))
	}
	for i, r := range reg.Routes {
		src := sources.Routes[i]
		if r.Route != src.RouteID || !kit.ValidLanguageSymbol(r.Symbol) {
			return fmt.Errorf("route %d: %s %s", i, r.Route, r.Symbol)
		}
		parsers := 0
		for k, f := range r.Files {
			if len(f.SHA256) != 64 || f.Bytes <= 0 || (k > 0 && r.Files[k-1].Path >= f.Path) {
				return fmt.Errorf("%s file %+v", r.Route, f)
			}
			if f.Role == "parser" {
				parsers++
				if f.Origin != "generated" {
					return fmt.Errorf("%s parser is not a regenerated output", r.Route)
				}
			}
		}
		if parsers != 1 {
			return fmt.Errorf("%s has %d parsers", r.Route, parsers)
		}
		adopted, regenerated := src.Adoption != nil, src.Regeneration != nil
		if adopted == regenerated || r.Regeneration == nil || r.Source != "regenerated" {
			return fmt.Errorf("%s adoption/regeneration mismatch", r.Route)
		}
		g := r.Regeneration
		if g.ABI != 15 || !g.Optimize || g.Grammar == "" || len(g.Inputs) == 0 {
			return fmt.Errorf("%s regeneration settings", r.Route)
		}
		if adopted && (!slices.Equal(g.PatchedFiles, src.Adoption.PatchedFiles) || len(g.PatchChain) == 0) {
			return fmt.Errorf("%s patched files differ from the adoption record", r.Route)
		}
		if regenerated && (len(g.PatchChain) != 0 || len(g.PatchedFiles) != 0) {
			return fmt.Errorf("%s regenerated-only route carries patches", r.Route)
		}
		if regenerated && !slices.Equal(g.Outputs, src.Regeneration.Outputs) {
			return fmt.Errorf("%s outputs differ from the source regeneration record", r.Route)
		}
		prefix := "src/"
		if r.GrammarDir != "." {
			prefix = r.GrammarDir + "/src/"
		}
		for _, f := range r.Files {
			switch f.Origin {
			case "generated":
				at := slices.IndexFunc(g.Outputs, func(o registryFile) bool { return prefix+o.Path == f.Path })
				if at < 0 || g.Outputs[at].SHA256 != f.SHA256 || g.Outputs[at].Bytes != f.Bytes {
					return fmt.Errorf("%s generated file %s is not a registered output", r.Route, f.Path)
				}
			case "patched":
				at := slices.IndexFunc(g.PatchedFiles, func(p registryFile) bool { return p.Path == f.Path })
				if at < 0 || g.PatchedFiles[at].SHA256 != f.SHA256 || g.PatchedFiles[at].Bytes != f.Bytes {
					return fmt.Errorf("%s patched file %s without its patch chain", r.Route, f.Path)
				}
			case "upstream":
				// a registered output path is built from the regenerated file, never from an
				// upstream generated copy (prepare-routes would overwrite the output with it)
				if slices.ContainsFunc(g.Outputs, func(o registryFile) bool { return prefix+o.Path == f.Path }) {
					return fmt.Errorf("%s output %s is taken from upstream", r.Route, f.Path)
				}
			default:
				return fmt.Errorf("%s file %s origin %q", r.Route, f.Path, f.Origin)
			}
		}
		at := slices.IndexFunc(repro.Routes, func(e reproRecord) bool { return e.Route == r.Route })
		if at < 0 {
			return fmt.Errorf("%s has no reproduction reference", r.Route)
		}
		ref := repro.Routes[at].PrepareGenerated
		if regenerated && len(ref) != len(g.Outputs) {
			return fmt.Errorf("%s reproduction reference pins %d of %d outputs", r.Route, len(ref), len(g.Outputs))
		}
		for _, o := range g.Outputs {
			pin, ok := ref[o.Path]
			if (o.Path == "parser.c" || regenerated) && (!ok || o.SHA256 != pin.SHA256 || o.Bytes != pin.Bytes) {
				return fmt.Errorf("%s regenerated %s differs from the reproduction reference", r.Route, o.Path)
			}
		}
	}
	return nil
}

// S05-A14/A22: the route build registry covers exactly the 26 registered routes with one
// parser each, a restricted entry symbol, sorted unique hashed files, and the regeneration
// of every route: the 6 adopted routes with their adoption hashes, the other 20 without
// patches, each against its reproduction reference.
func TestNativeRoutesRegistry(t *testing.T) {
	load := func() (nativeRegistry, sourceRecords, reproRecords) {
		var reg nativeRegistry
		var sources sourceRecords
		var repro reproRecords
		readJSON(t, "src/contracts/native-routes.json", &reg)
		readJSON(t, "src/contracts/language-sources.json", &sources)
		readJSON(t, "src/contracts/reproduction-routes.json", &repro)
		return reg, sources, repro
	}
	reg, sources, repro := load()
	if err := checkNativeRoutes(reg, sources, repro); err != nil {
		t.Fatal(err)
	}
	route := func(reg *nativeRegistry, name string) *registryRoute {
		for i := range reg.Routes {
			if reg.Routes[i].Route == name {
				return &reg.Routes[i]
			}
		}
		t.Fatalf("route %s missing", name)
		return nil
	}
	file := func(r *registryRoute, path string) *registryFile {
		for i := range r.Files {
			if r.Files[i].Path == path {
				return &r.Files[i]
			}
		}
		t.Fatalf("%s file %s missing", r.Route, path)
		return nil
	}
	source := func(s *sourceRecords, name string) *sourceRecord {
		for i := range s.Routes {
			if s.Routes[i].RouteID == name {
				return &s.Routes[i]
			}
		}
		t.Fatalf("source route %s missing", name)
		return nil
	}
	pins := func(x *reproRecords, name string) map[string]registryFile {
		for _, e := range x.Routes {
			if e.Route == name {
				return e.PrepareGenerated
			}
		}
		t.Fatalf("reproduction route %s missing", name)
		return nil
	}
	for _, tc := range []struct {
		name, diagnostic string
		mutate           func(*nativeRegistry, *sourceRecords, *reproRecords)
	}{
		{"upstream route", "adoption/regeneration", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			r := route(reg, "go")
			r.Regeneration, r.Source = nil, "upstream"
		}},
		{"upstream parser", "not a regenerated output", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			file(route(reg, "go"), "src/parser.c").Origin = "upstream"
		}},
		{"neither source record", "adoption/regeneration", func(_ *nativeRegistry, s *sourceRecords, _ *reproRecords) {
			source(s, "go").Regeneration = nil
		}},
		{"adoption on a regenerated-only route", "adoption/regeneration", func(_ *nativeRegistry, s *sourceRecords, _ *reproRecords) {
			source(s, "go").Adoption = source(s, "csharp").Adoption
		}},
		{"patched files without a chain", "carries patches", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			route(reg, "go").Regeneration.PatchedFiles = route(reg, "csharp").Regeneration.PatchedFiles
		}},
		{"chain on a regenerated-only route", "carries patches", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			route(reg, "go").Regeneration.PatchChain = route(reg, "csharp").Regeneration.PatchChain
		}},
		{"patched origin without a chain", "without its patch chain", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			file(route(reg, "python"), "src/scanner.c").Origin = "patched"
		}},
		{"output differs from the source record", "source regeneration record", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			route(reg, "go").Regeneration.Outputs[3].Bytes++
		}},
		{"missing output pin", "pins 5 of 6", func(_ *nativeRegistry, _ *sourceRecords, x *reproRecords) {
			delete(pins(x, "go"), "tree_sitter/array.h")
		}},
		{"generated file is not the output", "not a registered output", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			file(route(reg, "go"), "src/tree_sitter/array.h").SHA256 = strings.Repeat("0", 64)
		}},
		{"upstream header on an output path", "taken from upstream", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			file(route(reg, "go"), "src/tree_sitter/array.h").Origin = "upstream"
		}},
		{"unoptimized regeneration", "settings", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			route(reg, "json").Regeneration.Optimize = false
		}},
		{"adopted route without a chain", "adoption record", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			route(reg, "csharp").Regeneration.PatchChain = nil
		}},
		{"adopted parser differs from the reference", "reproduction reference", func(_ *nativeRegistry, _ *sourceRecords, x *reproRecords) {
			pin := pins(x, "csharp")["parser.c"]
			pin.Bytes++
			pins(x, "csharp")["parser.c"] = pin
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg, sources, repro := load()
			tc.mutate(&reg, &sources, &repro)
			if err := checkNativeRoutes(reg, sources, repro); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("mutation not detected as %q: %v", tc.diagnostic, err)
			}
		})
	}
}

// S05-A18: the large fixtures are reproduced here from their recipe, independently of the
// PowerShell generator, and must match the registered size and sha256.
func TestLargeFixtureIdentity(t *testing.T) {
	var spec struct {
		Route, Operation string
		Fixtures         []struct {
			ID, Header, Block, Footer, SHA256 string
			Repeat                            int
			Bytes                             int
			Points                            []kit.NativePoint
		}
	}
	readJSON(t, "src/contracts/native-large-fixtures.json", &spec)
	if spec.Route != "csharp" || spec.Operation != "real-world-source-r2" || len(spec.Fixtures) < 2 {
		t.Fatalf("spec %+v", spec.Route)
	}
	for _, fx := range spec.Fixtures {
		var b bytes.Buffer
		b.WriteString(fx.Header)
		for i := 0; i < fx.Repeat; i++ {
			b.WriteString(strings.ReplaceAll(fx.Block, "{N}", strconv.Itoa(i)))
		}
		b.WriteString(fx.Footer)
		sum := sha256.Sum256(b.Bytes())
		if b.Len() != fx.Bytes || hex.EncodeToString(sum[:]) != fx.SHA256 || fx.Bytes > 33554432 {
			t.Fatalf("%s: %d bytes %x", fx.ID, b.Len(), sum)
		}
		for _, p := range fx.Points {
			if int(p.Byte) >= fx.Bytes {
				t.Fatalf("%s point %s outside", fx.ID, p.ID)
			}
		}
	}
}

type routeCases struct {
	Schema, Route string
	Format        string
	Cases         []struct {
		ID, Kind   string
		Features   []string
		SourceUTF8 string `json:"source_utf8"`
		Edits      []struct{ Find, Replace string }
		Expect     []kit.StepExpectation
	}
}

// S05-A14/A21/A22: every registered case file is well formed: unique ids, edits that
// apply in order, expectations on existing steps, and two base cases for each route.
func TestRouteCaseFiles(t *testing.T) {
	var reg struct{ Routes []registryRoute }
	readJSON(t, "src/contracts/native-routes.json", &reg)
	seen := map[string]bool{}
	for _, kind := range []string{"routes", "gaps", "n461"} {
		dir := filepath.Join("..", "..", "testdata", "native", kind)
		entries, _ := os.ReadDir(dir)
		if kind == "routes" && len(entries) != 26 {
			t.Fatalf("route case files: %d", len(entries))
		}
		for _, e := range entries {
			var rc routeCases
			readJSON(t, "src/testdata/native/"+kind+"/"+e.Name(), &rc)
			name := rc.Route + ".json"
			if rc.Format == kit.SvcFormat {
				name = "svc.json" // the C# route with the SVC-SERVICEHOST-r1 composite
			}
			if rc.Schema != "tsgk-native-route-cases/r1" || name != e.Name() || len(rc.Cases) == 0 || (rc.Format != "" && rc.Format != kit.SvcFormat) {
				t.Fatalf("%s/%s header", kind, e.Name())
			}
			if kind == "routes" && len(rc.Cases) < 2 {
				t.Fatalf("%s: fewer than two cases", rc.Route)
			}
			for _, c := range rc.Cases {
				if seen[c.ID] || len(c.Edits) > 4 || len(c.SourceUTF8) > 65536 {
					t.Fatalf("%s: duplicate or oversized case", c.ID)
				}
				seen[c.ID] = true
				cur := c.SourceUTF8
				for _, ed := range c.Edits {
					at := len(cur)
					if ed.Find != "" {
						at = strings.Index(cur, ed.Find)
					}
					if at < 0 {
						t.Fatalf("%s: edit target %q not found", c.ID, ed.Find)
					}
					cur = cur[:at] + ed.Replace + cur[at+len(ed.Find):]
				}
				for _, x := range c.Expect {
					if x.Step < 0 || x.Step > len(c.Edits) || (x.Syntax != "NO_ERROR" && x.Syntax != "ERROR" && x.Syntax != "ANY") {
						t.Fatalf("%s: expectation %+v", c.ID, x)
					}
				}
			}
		}
	}
}

// Regressions only a Linux CI host shows: a test build that bypasses testBuildRequest
// misses the delegated cgroup parent (MEMORY_HARD_CAP_UNSUPPORTED), and a CI helper
// script without a final exit hands its last native exit code (tsgk 3 for the expected
// 32 MiB RESOURCE_LIMIT) to the workflow's $LASTEXITCODE check.
func TestHostSettingsReachCI(t *testing.T) {
	literal := "BuildRequest" + "{" // split so this file does not match itself
	files, _ := filepath.Glob("*_test.go")
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(string(data), literal); n > 0 && !(f == "native_test.go" && n == 1) {
			t.Errorf("%s builds a BuildRequest literal outside testBuildRequest", f)
		}
	}
	for _, rel := range []string{"src/dev/s05-native/run-routes.ps1", "src/dev/s05-native/prepare-routes.ps1"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if !strings.HasPrefix(lines[len(lines)-1], "exit 0") {
			t.Errorf("%s must end with an explicit exit 0", rel)
		}
	}
	// A linked compiler must be identified by its resolved file: Get-Item Length is the
	// link's own size (0 on Windows, the target name length on Linux).
	for _, rel := range []string{"src/dev/s05-native/run-routes.ps1", "src/dev/s05-native/run-corpus.ps1", "src/dev/s05-native/select-compiler.ps1"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "ResolveLinkTarget($true)") {
			t.Errorf("%s must resolve a linked compiler before identifying it", rel)
		}
	}
}
