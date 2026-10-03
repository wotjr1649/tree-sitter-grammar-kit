package native

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		PatchChain   []struct{ Subject, Pointer, Field, Target string } `json:"patch_chain"`
		PatchedFiles []registryFile                                     `json:"patched_files"`
	}
}

// S05-A14/A22: the route build registry covers exactly the 26 registered routes with one
// parser each, a restricted entry symbol, sorted unique hashed files, and for the 6
// adopted routes the adoption hashes and the reproduction reference parser.
func TestNativeRoutesRegistry(t *testing.T) {
	var reg struct {
		Schema string
		Routes []registryRoute
	}
	readJSON(t, "src/contracts/native-routes.json", &reg)
	var sources struct {
		Routes []struct {
			RouteID  string `json:"route_id"`
			Adoption *struct {
				PatchedFiles []registryFile `json:"patched_files"`
			}
		}
	}
	readJSON(t, "src/contracts/language-sources.json", &sources)
	var repro struct {
		Routes []struct {
			Route            string
			PrepareGenerated map[string]registryFile `json:"prepare_generated"`
		}
	}
	readJSON(t, "src/contracts/reproduction-routes.json", &repro)
	if reg.Schema != "tsgk-native-routes/r1" || len(reg.Routes) != len(sources.Routes) || len(reg.Routes) != 26 {
		t.Fatalf("registry %s with %d routes", reg.Schema, len(reg.Routes))
	}
	for i, r := range reg.Routes {
		src := sources.Routes[i]
		if r.Route != src.RouteID || !kit.ValidLanguageSymbol(r.Symbol) {
			t.Fatalf("route %d: %s %s", i, r.Route, r.Symbol)
		}
		parsers := 0
		for k, f := range r.Files {
			if len(f.SHA256) != 64 || f.Bytes <= 0 || (k > 0 && r.Files[k-1].Path >= f.Path) {
				t.Fatalf("%s file %+v", r.Route, f)
			}
			if f.Role == "parser" {
				parsers++
			}
		}
		if parsers != 1 {
			t.Fatalf("%s has %d parsers", r.Route, parsers)
		}
		if (src.Adoption != nil) != (r.Regeneration != nil) || (r.Source == "regenerated") != (src.Adoption != nil) {
			t.Fatalf("%s adoption/regeneration mismatch", r.Route)
		}
		if r.Regeneration == nil {
			continue
		}
		if !slices.Equal(r.Regeneration.PatchedFiles, src.Adoption.PatchedFiles) || len(r.Regeneration.PatchChain) == 0 {
			t.Fatalf("%s patched files differ from the adoption record", r.Route)
		}
		for _, rr := range repro.Routes {
			if rr.Route != r.Route {
				continue
			}
			ref := rr.PrepareGenerated["parser.c"]
			for _, o := range r.Regeneration.Outputs {
				if o.Path == "parser.c" && (o.SHA256 != ref.SHA256 || o.Bytes != ref.Bytes) {
					t.Fatalf("%s regenerated parser differs from the reproduction reference", r.Route)
				}
			}
		}
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
