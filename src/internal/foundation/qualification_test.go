package foundation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// The qualification inventory (src/contracts/qualification-c1.json) is generated from the
// adopted registries: campaign routes and platforms, REQ rows of the feature disposition,
// the native route registry and the registered cases, built exactly as
// src/dev/s05-native/run-routes.ps1 builds each oracle workload. Regenerate with
// TSGK_WRITE_INVENTORY=1 go test ./src/internal/foundation -run TestQualificationInventory.
const inventoryPath = "src/contracts/qualification-c1.json"

type caseFile struct {
	Format string `json:"format"`
	Cases  []struct {
		ID       string   `json:"id"`
		Kind     string   `json:"kind"`
		Features []string `json:"features"`
		Source   string   `json:"source_utf8"`
		Edits    []struct {
			Find    string `json:"find"`
			Replace string `json:"replace"`
		} `json:"edits"`
		Expect []struct {
			Step     int      `json:"step"`
			Syntax   string   `json:"syntax"`
			Contains []string `json:"contains"`
		} `json:"expect"`
	} `json:"cases"`
}

type queryFile struct {
	Queries []struct {
		ID     string `json:"id"`
		Source string `json:"source"`
	} `json:"queries"`
	Cases []struct {
		ID          string                 `json:"id"`
		SourceCase  string                 `json:"source_case"`
		Features    []string               `json:"features"`
		QueryExpect []kit.QueryExpectation `json:"query_expect"`
	} `json:"cases"`
}

func shaHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// buildInventory derives the inventory from the repository files.
func buildInventory(t *testing.T, root string) []byte {
	t.Helper()
	read := func(p string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	decode := func(p string, v any) {
		t.Helper()
		if err := json.Unmarshal(read(p), v); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	var campaign campaignDefinition
	decode("src/contracts/campaign-01.json", &campaign)
	var routes struct {
		Routes []struct {
			Route, Symbol string
			Files         []struct {
				Path, Role, SHA256 string
				Bytes              uint64
			}
		}
	}
	decode("src/contracts/native-routes.json", &routes)
	packBytes := read("src/contracts/fact-query-pack.json")
	var pack struct {
		Revision string
		Routes   []struct {
			Route   string
			Queries []struct{ ID, Source, Facts string }
		}
	}
	if err := json.Unmarshal(packBytes, &pack); err != nil {
		t.Fatal(err)
	}
	var dyn struct {
		Files []struct {
			Path, Route, SHA256 string
			Bytes               uint64
		}
	}
	decode("src/testdata/native/dynamic-sql/expected.json", &dyn)
	var large struct {
		Route    string
		Fixtures []struct {
			ID, SHA256 string
			Bytes      uint64
			Expect     []struct{ Syntax, Declarations string }
		}
	}
	decode("src/contracts/native-large-fixtures.json", &large)
	reqs := requirementRows(t, string(read("docs/validation/language-feature-disposition.md")))

	grammarOf := func(route string) (string, []kit.NativeInput) {
		for _, r := range routes.Routes {
			if r.Route == route {
				var g []kit.NativeInput
				for _, f := range r.Files {
					g = append(g, kit.NativeInput{Path: f.Path, Role: f.Role, SHA256: f.SHA256, Bytes: f.Bytes})
				}
				slices.SortFunc(g, func(a, b kit.NativeInput) int { return strings.Compare(a.Path, b.Path) })
				return r.Symbol, g
			}
		}
		t.Fatalf("route %s not in native-routes.json", route)
		return "", nil
	}
	packQueries := func(route, only string) []kit.QualQuery {
		out := []kit.QualQuery{}
		for _, r := range pack.Routes {
			if r.Route != route {
				continue
			}
			for _, q := range r.Queries {
				if only == "" || q.Facts == only {
					out = append(out, kit.QualQuery{ID: q.ID, SHA256: shaHex([]byte(q.Source))})
				}
			}
		}
		return out
	}
	packRef := func(route string) *kit.FactPackRef {
		for _, r := range pack.Routes {
			if r.Route == route {
				return &kit.FactPackRef{Revision: pack.Revision, SHA256: shaHex(packBytes), Route: route}
			}
		}
		return nil
	}
	// convert turns a registered case (find/replace edits on UTF-8 text) into the byte
	// edits run-routes.ps1 sends: each find is the first occurrence in the current version.
	convert := func(id, src string, edits []struct{ Find, Replace string }, expect []kit.StepExpectation) kit.QualCase {
		cur := []byte(src)
		var out []kit.Edit
		for _, e := range edits {
			find, repl := []byte(e.Find), []byte(e.Replace)
			at := len(cur)
			if len(find) > 0 {
				at = bytes.Index(cur, find)
			}
			if at < 0 {
				t.Fatalf("%s: edit target not found", id)
			}
			out = append(out, kit.Edit{StartByte: uint32(at), OldEndByte: uint32(at + len(find)), NewEndByte: uint32(at + len(repl)), Old: find, New: repl})
			cur = append(append(append([]byte{}, cur[:at]...), repl...), cur[at+len(find):]...)
		}
		if out == nil {
			out = []kit.Edit{}
		}
		return kit.QualCase{ID: id, Role: "requirement", Input: kit.NativeInput{SHA256: shaHex([]byte(src)), Bytes: uint64(len(src))}, Edits: out, Expect: expect,
			QueryExpect: []kit.QueryExpectation{}, Covers: map[string][]string{}}
	}
	type srcCase struct {
		id, src  string
		features []string
		edits    []struct{ Find, Replace string }
		expect   []kit.StepExpectation
	}
	load := func(p string) ([]srcCase, string) {
		var f caseFile
		decode(p, &f)
		var out []srcCase
		for _, c := range f.Cases {
			sc := srcCase{id: c.ID, src: c.Source, features: c.Features, expect: []kit.StepExpectation{}}
			for _, e := range c.Edits {
				sc.edits = append(sc.edits, struct{ Find, Replace string }{e.Find, e.Replace})
			}
			for _, e := range c.Expect {
				contains := e.Contains
				if contains == nil {
					contains = []string{}
				}
				sc.expect = append(sc.expect, kit.StepExpectation{Step: e.Step, Syntax: e.Syntax, Contains: contains})
			}
			out = append(out, sc)
		}
		return out, f.Format
	}
	cover := func(c *kit.QualCase, features []string, rows map[string][]string, kinds []string) {
		for _, f := range features {
			want, ok := rows[f]
			if !ok {
				continue // N461 role ids and free-text gap notes are not requirement rows
			}
			var ks []string
			for _, k := range kinds {
				if slices.Contains(want, k) && !slices.Contains(c.Covers[f], k) {
					ks = append(ks, k)
				}
			}
			if len(ks) > 0 {
				c.Covers[f] = append(c.Covers[f], ks...)
				slices.SortFunc(c.Covers[f], func(a, b string) int {
					return slices.Index([]string{"P", "N", "R", "E", "Q", "W"}, a) - slices.Index([]string{"P", "N", "R", "E", "Q", "W"}, b)
				})
			}
		}
	}
	derived := func(c kit.QualCase) []string {
		var ks []string
		for _, k := range []string{"P", "N", "R", "E", "Q"} {
			ok := false
			switch k {
			case "P":
				ok = slices.ContainsFunc(c.Expect, func(e kit.StepExpectation) bool { return e.Syntax == "NO_ERROR" && len(e.Contains) > 0 })
			case "N":
				ok = slices.ContainsFunc(c.Expect, func(e kit.StepExpectation) bool { return e.Syntax == "ERROR" })
			case "R":
				ok = slices.ContainsFunc(c.Expect, func(e kit.StepExpectation) bool { return e.Syntax == "ERROR" && len(e.Contains) > 0 })
			case "E":
				ok = len(c.Edits) > 0
			case "Q":
				ok = slices.ContainsFunc(c.QueryExpect, func(q kit.QueryExpectation) bool { return q.Captures != nil })
			}
			if ok {
				ks = append(ks, k)
			}
		}
		return ks
	}

	inv := kit.QualificationInventory{Schema: kit.QualificationInventorySchema, ID: "tsgk-c1-qualification-r1", Campaign: campaign.Campaign,
		Cells: len(campaign.Routes) * len(campaign.Platforms), CoverageRule: kit.CoverageRule, Kinds: []string{"P", "N", "R", "E", "Q", "W"}}
	for _, p := range campaign.Platforms {
		inv.Platforms = append(inv.Platforms, kit.QualPlatform{ID: p.ID, GOOS: p.GOOS, GOARCH: p.GOARCH})
	}
	var svc kit.QualWorkload
	for _, route := range campaign.Routes {
		rows := reqs[route]
		symbol, grammar := grammarOf(route)
		w := kit.QualWorkload{Set: "s06-" + route, Profile: "s06-" + route, Route: route, Operation: "native-query", Output: "tree", Symbol: symbol,
			Grammar: grammar, FactPack: packRef(route), API: true}
		for _, kind := range []string{"routes", "gaps", "n461"} {
			p := "src/testdata/native/" + kind + "/" + route + ".json"
			if _, err := os.Stat(filepath.Join(root, p)); err != nil {
				continue
			}
			cs, _ := load(p)
			for _, sc := range cs {
				c := convert(sc.id, sc.src, sc.edits, sc.expect)
				cover(&c, sc.features, rows, derived(c))
				w.Cases = append(w.Cases, c)
			}
		}
		var qf queryFile
		decode("src/testdata/native/queries/"+route+".json", &qf)
		routeCases, _ := load("src/testdata/native/routes/" + route + ".json")
		for _, q := range qf.Queries {
			w.Queries = append(w.Queries, kit.QualQuery{ID: q.ID, SHA256: shaHex([]byte(q.Source))})
		}
		w.Queries = append(w.Queries, packQueries(route, "")...)
		for _, q := range qf.Cases {
			i := slices.IndexFunc(routeCases, func(s srcCase) bool { return s.id == q.SourceCase })
			if i < 0 {
				t.Fatalf("%s: source case %s", q.ID, q.SourceCase)
			}
			sc := routeCases[i]
			c := convert(q.ID, sc.src, sc.edits, sc.expect)
			c.QueryExpect = q.QueryExpect
			src := sc.src
			c.Source = &src
			cover(&c, q.Features, rows, []string{"Q"}) // a query case covers its listed rows' Q only
			if !slices.Contains(derived(c), "Q") {
				t.Fatalf("%s: query case without capture expectations", q.ID)
			}
			w.Cases = append(w.Cases, c)
		}
		for _, f := range dyn.Files {
			if f.Route == route {
				w.Cases = append(w.Cases, kit.QualCase{ID: "dynamic-sql-" + route, Role: "support", Input: kit.NativeInput{SHA256: f.SHA256, Bytes: f.Bytes},
					Edits: []kit.Edit{}, Expect: []kit.StepExpectation{}, QueryExpect: []kit.QueryExpectation{}, Covers: map[string][]string{}})
			}
		}
		var req []kit.QualRequirement
		for _, row := range rows["#order"] {
			req = append(req, kit.QualRequirement{Row: row, Kinds: rows[row]})
		}
		inv.Routes = append(inv.Routes, kit.QualRoute{Route: route, Workload: w, Requirements: req})
		if route == "csharp" {
			cs, format := load("src/testdata/native/n461/svc.json")
			svc = kit.QualWorkload{Set: "s06-csharp-svc", Profile: "s06-csharp-svc", Route: "csharp", Operation: "native-query", Output: "tree", Symbol: symbol,
				Format: format, Grammar: grammar, Queries: packQueries("csharp", ""), API: true}
			for _, sc := range cs {
				c := convert(sc.id, sc.src, sc.edits, sc.expect)
				c.Role = "support"
				svc.Cases = append(svc.Cases, c)
			}
		}
	}
	// the NET461 large-file profile: windows/amd64 only (C1-REAL-WORLD-SOURCE-WINDOWS-R3)
	symbol, grammar := grammarOf(large.Route)
	lw := kit.QualWorkload{Set: "s06-large", Profile: "s06-large", Route: large.Route, Operation: "native-query-large", Output: "auto", Symbol: symbol,
		Grammar: grammar, Queries: packQueries(large.Route, "declarations"), FactPack: packRef(large.Route)}
	for _, f := range large.Fixtures {
		var ex []kit.StepExpectation
		for _, e := range f.Expect {
			ex = append(ex, kit.StepExpectation{Step: 0, Syntax: e.Syntax, Contains: []string{}, Declarations: e.Declarations})
		}
		lw.Cases = append(lw.Cases, kit.QualCase{ID: f.ID, Role: "support", Input: kit.NativeInput{SHA256: f.SHA256, Bytes: f.Bytes}, Edits: []kit.Edit{}, Expect: ex,
			QueryExpect: []kit.QueryExpectation{}, Covers: map[string][]string{}})
	}
	over := lw
	over.Set, over.Profile, over.Queries, over.FactPack = "s06-large-over", "s06-large-over", []kit.QualQuery{{ID: "all.nodes", SHA256: shaHex([]byte("_ @node\n"))}}, nil
	for _, c := range lw.Cases {
		if c.ID == "cs-large-8m-errors" {
			c.ID, c.ExpectStatus = "cs-large-8m-over-captures", kit.StatusResourceLimit
			over.Cases = []kit.QualCase{c}
		}
	}
	r3 := "NET461 workload is Windows-hosted (WinForms/.NET Framework 4.6.1)"
	inv.ExtraRoles = []kit.QualRole{
		{ID: "n461-svc", Role: "owned", Status: kit.RoleExecuted, Reason: "NET461 SVC-SERVICEHOST-r1 composite on the owned .svc fixtures (OWNED_FIXTURE, three hosts)",
			Platforms: []string{"windows-amd64", "linux-amd64", "darwin-arm64"}, NotApplicable: map[string]string{}, Workloads: []kit.QualWorkload{svc}},
		{ID: "n461-large", Role: "owned", Status: kit.RoleExecuted, Reason: "real-world-source-r3 / native-query-large synthetic large C# fixtures (OWNED_FIXTURE)",
			Platforms: []string{"windows-amd64"}, NotApplicable: map[string]string{"linux-amd64": r3, "darwin-arm64": r3}, Workloads: []kit.QualWorkload{lw, over}},
		{ID: "owned-native-fixtures", Role: "owned", Status: kit.RoleExternal, Reason: "owned scannerless and stateful-scanner fixtures: the foundation job's native tests on each host (src/internal/native), not this aggregation",
			Platforms: []string{}, NotApplicable: map[string]string{}},
		{ID: "brightscript-maintained", Role: "maintained", Status: kit.RoleNotRun, Reason: "BrightScript v0.1.4 maintained reference: no native route or workload is registered in this campaign",
			Platforms: []string{}, NotApplicable: map[string]string{}},
		{ID: "brightscript-historical", Role: "historical", Status: kit.RoleNotRun, Reason: "BrightScript v0.1.2 historical replay (S07-REPLAY-2ULP-r1): the verification bundle is not prepared; never a NEW_RUN",
			Platforms: []string{}, NotApplicable: map[string]string{}},
		{ID: "cooklang-audit", Role: "historical", Status: kit.RoleNotRun, Reason: "Cooklang legacy metadata audit: static S00 observation only, not executed",
			Platforms: []string{}, NotApplicable: map[string]string{}},
	}
	data, err := json.MarshalIndent(inv, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

// requirementRows returns route → row → required kinds for REQ rows, with "#order" keeping
// the rows in document order.
func requirementRows(t *testing.T, md string) map[string]map[string][]string {
	t.Helper()
	id := regexp.MustCompile(`^([a-z-]+)-[A-Z][0-9]+[a-z]*$`)
	out := map[string]map[string][]string{}
	for _, line := range strings.Split(md, "\n") {
		if !strings.HasPrefix(line, "| ") {
			continue
		}
		f := strings.Split(line, "|")
		if len(f) != 9 {
			continue
		}
		row := strings.TrimSpace(f[1])
		m := id.FindStringSubmatch(row)
		if m == nil || strings.TrimSpace(f[3]) != "REQ" {
			continue
		}
		if out[m[1]] == nil {
			out[m[1]] = map[string][]string{}
		}
		out[m[1]][row] = strings.Split(strings.TrimSpace(f[6]), ",")
		out[m[1]]["#order"] = append(out[m[1]]["#order"], row)
	}
	return out
}

// TestQualificationInventory checks the tracked inventory is exactly the one the adopted
// registries and registered cases produce, and that the kit accepts it with 78 cells.
func TestQualificationInventory(t *testing.T) {
	root := repository(t)
	want := buildInventory(t, root)
	if os.Getenv("TSGK_WRITE_INVENTORY") == "1" {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(inventoryPath)), want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(inventoryPath)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is not the inventory the registries produce; regenerate it (TSGK_WRITE_INVENTORY=1)", inventoryPath)
	}
	inv, err := kit.ParseQualificationInventory(got)
	if err != nil {
		t.Fatal(err)
	}
	var campaign campaignDefinition
	if err := json.Unmarshal(mustRead(t, filepath.Join(root, "src/contracts/campaign-01.json")), &campaign); err != nil {
		t.Fatal(err)
	}
	if inv.Cells != 78 || len(inv.Routes) != 26 || len(inv.Platforms) != 3 || inv.Cells != len(campaign.Routes)*len(campaign.Platforms) {
		t.Fatalf("cells %d routes %d platforms %d", inv.Cells, len(inv.Routes), len(inv.Platforms))
	}
	for i, r := range inv.Routes {
		if r.Route != campaign.Routes[i] {
			t.Fatalf("route order %d: %s != %s", i, r.Route, campaign.Routes[i])
		}
	}
	// the two SQL dialects stay separate routes with separate grammars and cases
	tsql, pg := inv.Routes[slices.IndexFunc(inv.Routes, func(r kit.QualRoute) bool { return r.Route == "tsql" })], inv.Routes[slices.IndexFunc(inv.Routes, func(r kit.QualRoute) bool { return r.Route == "postgresql-sql" })]
	if tsql.Workload.Grammar[0].SHA256 == pg.Workload.Grammar[0].SHA256 || tsql.Workload.Cases[0].Input.SHA256 == pg.Workload.Cases[0].Input.SHA256 {
		t.Fatal("SQL dialects collapsed")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
