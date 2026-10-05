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
		PatchChain   []chainStep    `json:"patch_chain"`
		PatchedFiles []registryFile `json:"patched_files"`
	}
}

type chainStep struct{ Subject, Pointer, Field, Target string }

// c2Subject is a project-local patch subject (src/dev/c2-patches/<route>.json).
type c2Subject struct {
	Schema, Route string
	Files         []struct{ Target string }
}

type nativeRegistry struct {
	Schema string
	Routes []registryRoute
}

type sourceRecord struct {
	RouteID  string `json:"route_id"`
	Adoption *struct {
		PatchSubjects []string       `json:"patch_subjects"`
		PatchedFiles  []registryFile `json:"patched_files"`
	}
	Regeneration *struct {
		Outputs []registryFile
	}
	C2Patch *struct {
		Subjects     []string
		PatchedFiles []registryFile `json:"patched_files"`
	} `json:"c2_patch"`
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
// one built from the upstream generated files, is rejected. Either class may carry a C2
// record (C2-PATCH-r1): its chain is the adoption chain, if any, followed by exactly the
// C2 subjects' files in order, its patched files are the C2 record's, and the reproduction
// reference pins every output.
func checkNativeRoutes(reg nativeRegistry, sources sourceRecords, repro reproRecords, subjects map[string]c2Subject) error {
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
		var c2 []chainStep
		if p := src.C2Patch; p != nil {
			for _, s := range p.Subjects {
				doc, ok := subjects[s]
				if !ok || doc.Schema != "tsgk-c2-patch/r1" || doc.Route != r.Route || len(doc.Files) == 0 {
					return fmt.Errorf("%s C2 subject %s is not a patch of this route", r.Route, s)
				}
				for i, f := range doc.Files {
					c2 = append(c2, chainStep{s, fmt.Sprintf("/files/%d", i), "operations", f.Target})
				}
			}
		}
		own := len(g.PatchChain) - len(c2) // the chain before the C2 steps
		if own < 0 || !slices.Equal(g.PatchChain[own:], c2) {
			return fmt.Errorf("%s patch chain does not end with its C2 subjects", r.Route)
		}
		if adopted && (own == 0 || slices.ContainsFunc(g.PatchChain[:own], func(s chainStep) bool { return !slices.Contains(src.Adoption.PatchSubjects, s.Subject) })) {
			return fmt.Errorf("%s chain does not start with the adoption record's chain", r.Route)
		}
		if adopted && src.C2Patch == nil && !slices.Equal(g.PatchedFiles, src.Adoption.PatchedFiles) {
			return fmt.Errorf("%s patched files differ from the adoption record", r.Route)
		}
		if regenerated && (own != 0 || (src.C2Patch == nil && len(g.PatchedFiles) != 0)) {
			return fmt.Errorf("%s regenerated-only route carries patches", r.Route)
		}
		if src.C2Patch != nil && !slices.Equal(g.PatchedFiles, src.C2Patch.PatchedFiles) {
			return fmt.Errorf("%s patched files differ from the C2 patch record", r.Route)
		}
		// on an adopted route the C2 record starts from the adoption hashes: every adopted file is
		// in it, one that no C2 step patches keeps its adoption hash, and every other file in it
		// is a C2 target
		if adopted && src.C2Patch != nil {
			for _, p := range src.C2Patch.PatchedFiles {
				if !slices.ContainsFunc(src.Adoption.PatchedFiles, func(a registryFile) bool { return a.Path == p.Path }) && !slices.ContainsFunc(c2, func(s chainStep) bool { return s.Target == p.Path }) {
					return fmt.Errorf("%s C2 record file %s is neither an adoption patched file nor a C2 target", r.Route, p.Path)
				}
			}
			for _, a := range src.Adoption.PatchedFiles {
				at := slices.IndexFunc(src.C2Patch.PatchedFiles, func(p registryFile) bool { return p.Path == a.Path })
				if at < 0 {
					return fmt.Errorf("%s adoption patched file %s is missing from the C2 patch record", r.Route, a.Path)
				}
				if !slices.ContainsFunc(c2, func(s chainStep) bool { return s.Target == a.Path }) && src.C2Patch.PatchedFiles[at] != a {
					return fmt.Errorf("%s patched file %s, which no C2 step targets, differs from its adoption hash", r.Route, a.Path)
				}
			}
		}
		// adoption steps patch adoption files only, so each of their results has an adoption hash
		if adopted && slices.ContainsFunc(g.PatchChain[:own], func(s chainStep) bool {
			return !slices.ContainsFunc(src.Adoption.PatchedFiles, func(a registryFile) bool { return a.Path == s.Target })
		}) {
			return fmt.Errorf("%s adoption chain step targets a file without an adoption hash", r.Route)
		}
		// every chain target is a patched file, and a patched file is used patched
		targets := map[string]bool{}
		for _, s := range g.PatchChain {
			targets[s.Target] = true
		}
		for _, p := range g.PatchedFiles {
			if !targets[p.Path] {
				return fmt.Errorf("%s patched file %s is no chain target", r.Route, p.Path)
			}
			delete(targets, p.Path)
			in := slices.IndexFunc(g.Inputs, func(f registryFile) bool { return f.Path == p.Path })
			file := slices.IndexFunc(r.Files, func(f registryFile) bool { return f.Path == p.Path })
			if (in >= 0 && (g.Inputs[in].SHA256 != p.SHA256 || g.Inputs[in].Bytes != p.Bytes)) || (file >= 0 && r.Files[file].Origin != "patched") || (in < 0 && file < 0) {
				return fmt.Errorf("%s patched file %s is not used patched", r.Route, p.Path)
			}
		}
		if len(targets) != 0 {
			return fmt.Errorf("%s chain target without its patched file", r.Route)
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
		all := regenerated || src.C2Patch != nil // the reference pins every output
		if all && len(ref) != len(g.Outputs) {
			return fmt.Errorf("%s reproduction reference pins %d of %d outputs", r.Route, len(ref), len(g.Outputs))
		}
		for _, o := range g.Outputs {
			pin, ok := ref[o.Path]
			if (o.Path == "parser.c" || all) && (!ok || o.SHA256 != pin.SHA256 || o.Bytes != pin.Bytes) {
				return fmt.Errorf("%s regenerated %s differs from the reproduction reference", r.Route, o.Path)
			}
		}
	}
	return nil
}

// S05-A14/A22: the route build registry covers exactly the 26 registered routes with one
// parser each, a restricted entry symbol, sorted unique hashed files, and the regeneration
// of every route: the 6 adopted routes with their adoption hashes, the other 20 without
// patches, each against its reproduction reference, and any route's C2 patches.
func TestNativeRoutesRegistry(t *testing.T) {
	load := func() (nativeRegistry, sourceRecords, reproRecords, map[string]c2Subject) {
		var reg nativeRegistry
		var sources sourceRecords
		var repro reproRecords
		readJSON(t, "src/contracts/native-routes.json", &reg)
		readJSON(t, "src/contracts/language-sources.json", &sources)
		readJSON(t, "src/contracts/reproduction-routes.json", &repro)
		subjects := map[string]c2Subject{}
		for _, s := range sources.Routes {
			if s.C2Patch == nil {
				continue
			}
			for _, name := range s.C2Patch.Subjects {
				var doc c2Subject
				readJSON(t, name, &doc)
				subjects[name] = doc
			}
		}
		return reg, sources, repro, subjects
	}
	reg, sources, repro, subjects := load()
	if err := checkNativeRoutes(reg, sources, repro, subjects); err != nil {
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
		// go without its C2 patch (chain, patched files and record) is a regenerated-only route
		{"patched files without a chain", "carries patches", func(reg *nativeRegistry, s *sourceRecords, _ *reproRecords) {
			g := route(reg, "go").Regeneration
			g.PatchChain, source(s, "go").C2Patch = nil, nil
			g.PatchedFiles = route(reg, "csharp").Regeneration.PatchedFiles
		}},
		{"chain on a regenerated-only route", "carries patches", func(reg *nativeRegistry, s *sourceRecords, _ *reproRecords) {
			g := route(reg, "go").Regeneration
			g.PatchedFiles, source(s, "go").C2Patch = nil, nil
			g.PatchChain = route(reg, "csharp").Regeneration.PatchChain
		}},
		{"patched origin without a chain", "without its patch chain", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			file(route(reg, "r"), "src/scanner.c").Origin = "patched" // r's C2 patch leaves its scanner unpatched
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
		{"adopted route without a chain", "adoption record's chain", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			route(reg, "csharp").Regeneration.PatchChain = nil
		}},
		{"adopted patched file off its adoption record", "patched files differ from the adoption record", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			route(reg, "csharp").Regeneration.PatchedFiles[0].Bytes++
		}},
		{"patched file without a chain step", "is no chain target", func(reg *nativeRegistry, s *sourceRecords, _ *reproRecords) {
			extra := registryFile{Path: "grammar.js", Bytes: 1, SHA256: strings.Repeat("0", 64)}
			route(reg, "yaml").Regeneration.PatchedFiles = append(route(reg, "yaml").Regeneration.PatchedFiles, extra)
			source(s, "yaml").C2Patch.PatchedFiles = append(source(s, "yaml").C2Patch.PatchedFiles, extra)
		}},
		{"adopted parser differs from the reference", "reproduction reference", func(_ *nativeRegistry, _ *sourceRecords, x *reproRecords) {
			pin := pins(x, "csharp")["parser.c"]
			pin.Bytes++
			pins(x, "csharp")["parser.c"] = pin
		}},
		{"adopted chain step of another subject", "adoption record's chain", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			route(reg, "csharp").Regeneration.PatchChain[0].Subject = "src/dev/prepare-p05/remedy-tsql-r6.json"
		}},
		// C2-PATCH-r1, on the patched yaml route
		{"C2 chain without a record", "carries patches", func(_ *nativeRegistry, s *sourceRecords, _ *reproRecords) {
			source(s, "yaml").C2Patch = nil
		}},
		{"C2 record without a chain", "does not end with its C2 subjects", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			route(reg, "yaml").Regeneration.PatchChain = nil
		}},
		{"C2 chain step off its subject file", "does not end with its C2 subjects", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			route(reg, "yaml").Regeneration.PatchChain[0].Pointer = "/files/1"
		}},
		{"C2 chain step on another target", "does not end with its C2 subjects", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			route(reg, "yaml").Regeneration.PatchChain[0].Target = "grammar.js"
		}},
		{"C2 patched file hash", "differ from the C2 patch record", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			route(reg, "yaml").Regeneration.PatchedFiles[0].SHA256 = strings.Repeat("0", 64)
		}},
		{"C2 record hash", "differ from the C2 patch record", func(_ *nativeRegistry, s *sourceRecords, _ *reproRecords) {
			source(s, "yaml").C2Patch.PatchedFiles[0].Bytes++
		}},
		{"C2 record without the target's file", "chain target without its patched file", func(reg *nativeRegistry, s *sourceRecords, _ *reproRecords) {
			route(reg, "yaml").Regeneration.PatchedFiles, source(s, "yaml").C2Patch.PatchedFiles = nil, nil
		}},
		{"C2 patched file built from upstream", "not used patched", func(reg *nativeRegistry, _ *sourceRecords, _ *reproRecords) {
			file(route(reg, "yaml"), "src/scanner.c").Origin = "upstream"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg, sources, repro, subjects := load()
			tc.mutate(&reg, &sources, &repro)
			if err := checkNativeRoutes(reg, sources, repro, subjects); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("mutation not detected as %q: %v", tc.diagnostic, err)
			}
		})
	}
	t.Run("C2 subject of another route", func(t *testing.T) {
		reg, sources, repro, subjects := load()
		doc := subjects["src/dev/c2-patches/yaml.json"]
		doc.Route = "json"
		subjects["src/dev/c2-patches/yaml.json"] = doc
		if err := checkNativeRoutes(reg, sources, repro, subjects); err == nil || !strings.Contains(err.Error(), "not a patch of this route") {
			t.Fatalf("mutation not detected: %v", err)
		}
	})
	// A synthetic C2 record on the adopted csharp route: one C2 step on grammar.js, the C2
	// record starting from the adoption hashes, every output pinned. It is accepted; each
	// mutation below breaks exactly one rule.
	addAdoptionStepOnPackageJSON := func(reg *nativeRegistry, s *sourceRecords) {
		g := route(reg, "csharp").Regeneration
		src := source(s, "csharp")
		step := chainStep{src.Adoption.PatchSubjects[0], "/patches/0/files/9", "operations", "package.json"}
		own := len(g.PatchChain) - 1 // the synthetic record has one C2 step
		g.PatchChain = append(g.PatchChain[:own:own], append([]chainStep{step}, g.PatchChain[own:]...)...)
		in := g.Inputs[slices.IndexFunc(g.Inputs, func(f registryFile) bool { return f.Path == "package.json" })]
		pin := registryFile{Path: in.Path, Bytes: in.Bytes, SHA256: in.SHA256}
		src.C2Patch.PatchedFiles = append(src.C2Patch.PatchedFiles, pin)
		g.PatchedFiles = append(g.PatchedFiles, pin)
	}
	adoptedC2 := func() (nativeRegistry, sourceRecords, reproRecords, map[string]c2Subject) {
		reg, sources, repro, subjects := load()
		name := "src/dev/c2-patches/csharp.json"
		subjects[name] = c2Subject{Schema: "tsgk-c2-patch/r1", Route: "csharp", Files: []struct{ Target string }{{"grammar.js"}}}
		src := source(&sources, "csharp")
		src.C2Patch = &struct {
			Subjects     []string
			PatchedFiles []registryFile `json:"patched_files"`
		}{[]string{name}, slices.Clone(src.Adoption.PatchedFiles)}
		g := route(&reg, "csharp").Regeneration
		g.PatchChain = append(g.PatchChain, chainStep{name, "/files/0", "operations", "grammar.js"})
		g.PatchedFiles = slices.Clone(src.C2Patch.PatchedFiles)
		ref := pins(&repro, "csharp")
		for _, o := range g.Outputs {
			ref[o.Path] = registryFile{SHA256: o.SHA256, Bytes: o.Bytes}
		}
		return reg, sources, repro, subjects
	}
	if err := checkNativeRoutes(adoptedC2()); err != nil {
		t.Fatalf("synthetic C2 record on an adopted route: %v", err)
	}
	for _, tc := range []struct {
		name, diagnostic string
		mutate           func(*nativeRegistry, *sourceRecords, *reproRecords, map[string]c2Subject)
	}{
		{"C2-patched adopted route with its parser.c pin only", "pins 2 of 6", func(_ *nativeRegistry, _ *sourceRecords, x *reproRecords, _ map[string]c2Subject) {
			for path := range pins(x, "csharp") {
				if path != "parser.c" && path != "tree_sitter/parser.h" {
					delete(pins(x, "csharp"), path)
				}
			}
		}},
		{"C2-patched adopted route with a wrong output pin", "differs from the reproduction reference", func(_ *nativeRegistry, _ *sourceRecords, x *reproRecords, _ map[string]c2Subject) {
			pin := pins(x, "csharp")["node-types.json"]
			pin.Bytes++
			pins(x, "csharp")["node-types.json"] = pin
		}},
		{"adoption file missing from the C2 record", "missing from the C2 patch record", func(reg *nativeRegistry, s *sourceRecords, _ *reproRecords, _ map[string]c2Subject) {
			drop := func(f registryFile) bool { return f.Path == "src/scanner.c" }
			source(s, "csharp").C2Patch.PatchedFiles = slices.DeleteFunc(source(s, "csharp").C2Patch.PatchedFiles, drop)
			route(reg, "csharp").Regeneration.PatchedFiles = slices.DeleteFunc(route(reg, "csharp").Regeneration.PatchedFiles, drop)
		}},
		{"file no C2 step targets off its adoption hash", "differs from its adoption hash", func(reg *nativeRegistry, s *sourceRecords, _ *reproRecords, _ map[string]c2Subject) {
			for _, files := range [][]registryFile{source(s, "csharp").C2Patch.PatchedFiles, route(reg, "csharp").Regeneration.PatchedFiles} {
				files[slices.IndexFunc(files, func(f registryFile) bool { return f.Path == "src/scanner.c" })].SHA256 = strings.Repeat("0", 64)
			}
		}},
		// an adoption step on a new file whose hash is carried into the C2 record and the registry
		{"C2 record file outside the adoption and C2 files", "neither an adoption patched file nor a C2 target", func(reg *nativeRegistry, s *sourceRecords, _ *reproRecords, _ map[string]c2Subject) {
			addAdoptionStepOnPackageJSON(reg, s)
		}},
		// the same file also made a C2 target: the record rule holds, the adoption step is still unpinned
		{"adoption chain step on a file without an adoption hash", "adoption chain step targets a file without an adoption hash", func(reg *nativeRegistry, s *sourceRecords, _ *reproRecords, subjects map[string]c2Subject) {
			addAdoptionStepOnPackageJSON(reg, s)
			name := "src/dev/c2-patches/csharp.json"
			doc := subjects[name]
			doc.Files = append(doc.Files, struct{ Target string }{"package.json"})
			subjects[name] = doc
			g := route(reg, "csharp").Regeneration
			g.PatchChain = append(g.PatchChain, chainStep{name, "/files/1", "operations", "package.json"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg, sources, repro, subjects := adoptedC2()
			tc.mutate(&reg, &sources, &repro, subjects)
			if err := checkNativeRoutes(reg, sources, repro, subjects); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
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
