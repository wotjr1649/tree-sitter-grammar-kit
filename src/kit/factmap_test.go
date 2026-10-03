package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type factMapping struct {
	Schema   string `json:"schema"`
	Revision string `json:"revision"`
	Routes   []struct {
		Route  string `json:"route"`
		Schema struct {
			Repository, Commit, Path, SHA256 string
			Bytes                            int
		} `json:"schema"`
		CandidateSchema string `json:"candidate_schema"`
		Facts           []struct {
			Fact, Node, Name, Value, Match, Status, Reason string
		} `json:"facts"`
	} `json:"routes"`
	DynamicSQL struct {
		Revision string `json:"revision"`
		Kinds    []struct {
			Kind, Route, Node, Match, Argument string
		} `json:"kinds"`
		NonDynamic []struct {
			Kind, Route, Node, Match string
		} `json:"non_dynamic"`
		ArgumentKinds []struct {
			Kind, TSQL, CSharp string
		} `json:"argument_kinds"`
		KnownMisses []struct{ Case, Reason string } `json:"known_misses"`
	} `json:"dynamic_sql"`
}

var factKinds = []string{"type_declaration", "member_declaration", "create_object", "static_exec_target", "command_text_site", "command_text_literal", "dynamic_sql_site"}

var locatorStep = regexp.MustCompile(`^(field|child|children):[A-Za-z_][A-Za-z0-9_]*$`)

func readFactMapping(t *testing.T) (factMapping, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "contracts", "fact-mapping.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, e := decodeStrict("fact-mapping", data, MaxDocumentBytes); e != nil {
		t.Fatal(e)
	}
	var m factMapping
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m, data
}

// S03-A15/A17: every fact kind of every mapped route is mapped to schema nodes with a
// well-formed locator or is UNSUPPORTED with a reason; dynamic SQL kinds are closed.
func TestFactMapping(t *testing.T) {
	m, _ := readFactMapping(t)
	if m.Schema != "tsgk-fact-mapping/r1" || m.Revision != "declaration-facts-r1" {
		t.Fatalf("revision %s %s", m.Schema, m.Revision)
	}
	var reg struct {
		Routes []struct {
			RouteID             string `json:"route_id"`
			Repository, Commit  string
			GrammarSubdirectory string `json:"grammar_subdirectory"`
		} `json:"routes"`
	}
	data, err := os.ReadFile(filepath.Join("..", "contracts", "language-sources.json"))
	if err != nil || json.Unmarshal(data, &reg) != nil {
		t.Fatalf("registry: %v", err)
	}
	routes := map[string]bool{}
	for _, r := range m.Routes {
		i := slices.IndexFunc(reg.Routes, func(x struct {
			RouteID             string `json:"route_id"`
			Repository, Commit  string
			GrammarSubdirectory string `json:"grammar_subdirectory"`
		}) bool {
			return x.RouteID == r.Route
		})
		if i < 0 || reg.Routes[i].Repository != r.Schema.Repository || reg.Routes[i].Commit != r.Schema.Commit {
			t.Fatalf("%s: schema source differs from the registry", r.Route)
		}
		want := "src/node-types.json"
		if sub := reg.Routes[i].GrammarSubdirectory; sub != "." {
			want = sub + "/" + want
		}
		if r.Schema.Path != want || len(r.Schema.SHA256) != 64 || r.Schema.Bytes <= 0 || !strings.HasPrefix(r.CandidateSchema, "NOT_AVAILABLE: ") {
			t.Fatalf("%s: schema binding %+v %q", r.Route, r.Schema, r.CandidateSchema)
		}
		routes[r.Route] = true
		seen := map[string]bool{}
		for _, f := range r.Facts {
			if !slices.Contains(factKinds, f.Fact) {
				t.Fatalf("%s: unknown fact kind %q", r.Route, f.Fact)
			}
			seen[f.Fact] = true
			if f.Status == "UNSUPPORTED" {
				if f.Reason == "" || f.Node != "" || f.Name != "" {
					t.Fatalf("%s %s: UNSUPPORTED needs a reason and no node", r.Route, f.Fact)
				}
				continue
			}
			if f.Status != "" || f.Node == "" || f.Reason != "" {
				t.Fatalf("%s %s: mapped fact %+v", r.Route, f.Fact, f)
			}
			for _, loc := range []string{f.Name, f.Value} {
				if loc == "" || loc == "node" {
					if loc == "" && f.Name == "" {
						t.Fatalf("%s %s: missing name locator", r.Route, f.Node)
					}
					continue
				}
				for _, step := range strings.Split(loc, "/") {
					if !locatorStep.MatchString(step) {
						t.Fatalf("%s %s: bad locator step %q", r.Route, f.Node, step)
					}
				}
			}
		}
		for _, k := range factKinds {
			if !seen[k] {
				t.Fatalf("%s: fact kind %s is neither mapped nor UNSUPPORTED", r.Route, k)
			}
		}
	}
	for _, r := range []string{"csharp", "tsql", "postgresql-sql"} {
		if !routes[r] {
			t.Fatalf("adopted declaration route %s is not mapped", r)
		}
	}
	d := m.DynamicSQL
	var kinds, args []string
	for _, k := range d.Kinds {
		kinds = append(kinds, k.Kind)
	}
	for _, a := range d.ArgumentKinds {
		args = append(args, a.Kind)
	}
	if d.Revision != "dynamic-sql-r1" || strings.Join(kinds, ",") != "EXEC_PAREN,EXEC_PAREN_AT,SP_EXECUTESQL,CSHARP_COMMAND" ||
		strings.Join(args, ",") != "literal_unicode,literal_char,variable,bare_word,concatenation,other" ||
		len(d.NonDynamic) != 1 || d.NonDynamic[0].Kind != "EXEC_MODULE_VARIABLE" || len(d.KnownMisses) != 3 {
		t.Fatalf("dynamic SQL contract %+v", d)
	}
	for i, miss := range []string{"AT DATA_SOURCE", "WITH RESULT SETS", "without EXEC"} {
		if !strings.Contains(d.KnownMisses[i].Case, miss) {
			t.Fatalf("known miss %d: %q", i, d.KnownMisses[i].Case)
		}
	}
}

// S03-A15: with TSGK_ROUTE_SCHEMAS naming a directory of <route>.json copies of the bound
// upstream schemas, every locator step resolves in the schema. The schemas are not vendored,
// so hosted CI skips this check.
func TestFactMappingAgainstSchemas(t *testing.T) {
	dir := os.Getenv("TSGK_ROUTE_SCHEMAS")
	if dir == "" {
		t.Skip("TSGK_ROUTE_SCHEMAS not set: upstream schemas are local inputs")
	}
	m, _ := readFactMapping(t)
	type set struct {
		Types []NodeRef `json:"types"`
	}
	type node struct {
		Type     string         `json:"type"`
		Named    bool           `json:"named"`
		Fields   map[string]set `json:"fields"`
		Children *set           `json:"children"`
	}
	for _, r := range m.Routes {
		data, err := os.ReadFile(filepath.Join(dir, r.Route+".json"))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != r.Schema.SHA256 || len(data) != r.Schema.Bytes {
			t.Fatalf("%s: local schema is not the bound schema", r.Route)
		}
		res, err := SchemaCheck(testCtx(t), SchemaCheckRequest{Input: SchemaInput{Name: r.Route, Data: data}, Limits: DefaultSchemaLimits()})
		if err != nil || res.Assessment != AssessPass {
			t.Fatalf("%s: schema check %v %s", r.Route, err, res.Assessment)
		}
		var nodes []node
		if err := json.Unmarshal(data, &nodes); err != nil {
			t.Fatal(err)
		}
		named := map[string]node{}
		for _, n := range nodes {
			if n.Named {
				named[n.Type] = n
			}
		}
		resolve := func(start, loc string) {
			cur, ok := named[start]
			if !ok {
				t.Fatalf("%s: node %s is not a named node of the schema", r.Route, start)
			}
			if loc == "" || loc == "node" {
				return
			}
			steps := strings.Split(loc, "/")
			for i, step := range steps {
				kind, name, _ := strings.Cut(step, ":")
				var types []NodeRef
				if kind == "field" {
					f, ok := cur.Fields[name]
					if !ok {
						t.Fatalf("%s %s: %s has no field %s", r.Route, loc, cur.Type, name)
					}
					types = f.Types
				} else {
					if cur.Children == nil || !slices.Contains(cur.Children.Types, NodeRef{name, true}) {
						t.Fatalf("%s %s: %s has no named child %s", r.Route, loc, cur.Type, name)
					}
					types = []NodeRef{{name, true}}
				}
				if i == len(steps)-1 {
					return
				}
				if len(types) != 1 {
					t.Fatalf("%s %s: step %s is ambiguous", r.Route, loc, step)
				}
				cur = named[types[0].Type]
			}
		}
		for _, f := range r.Facts {
			if f.Status == "" {
				resolve(f.Node, f.Name)
				resolve(f.Node, f.Value)
			}
		}
	}
}

// S03-A16: the summary example has exactly the fixed fields and value rules, carries ranges
// only (no extracted text) and its partial tree uses the ordered tree node shape.
func TestTreeSummaryExample(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "contracts", "examples", "tree-summary-r1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, e := decodeStrict("summary", data, MaxDocumentBytes); e != nil {
		t.Fatal(e)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	keys := func(where string, v any, want ...string) map[string]any {
		t.Helper()
		m, ok := v.(map[string]any)
		got := []string{}
		for k := range m {
			got = append(got, k)
		}
		slices.Sort(got)
		slices.Sort(want)
		if !ok || !slices.Equal(got, want) {
			t.Fatalf("%s keys %v, want %v", where, got, want)
		}
		return m
	}
	point := func(where string, v any) { keys(where, v, "row", "column") }
	span := func(where string, m map[string]any) {
		point(where+".start_point", m["start_point"])
		point(where+".end_point", m["end_point"])
		if m["start_byte"].(float64) > m["end_byte"].(float64) {
			t.Fatalf("%s: inverted range", where)
		}
	}
	keys("summary", doc, "schema", "input", "status", "identities", "reason", "descendant_count", "digest", "errors", "declarations", "partial_trees")
	if doc["schema"] != "tsgk-tree-summary/r1" || !slices.Contains([]string{"DESCENDANT_LIMIT", "OUTPUT_LIMIT"}, doc["reason"].(string)) ||
		!slices.Contains([]string{"COMPLETED", "CANCELLED", "RESOURCE_LIMIT", "FAILED"}, doc["status"].(string)) {
		t.Fatalf("summary header %v %v %v", doc["schema"], doc["reason"], doc["status"])
	}
	in := keys("input", doc["input"], "bytes", "sha256", "encoding", "encoding_source")
	if !slices.Contains([]string{EncodingUTF8, "UTF-16LE", "UTF-16BE", "CP949"}, in["encoding"].(string)) ||
		!slices.Contains([]string{"BOM", "VALIDATION", SourceDeclaration}, in["encoding_source"].(string)) {
		t.Fatalf("input encoding %v", in)
	}
	for _, id := range doc["identities"].([]any) {
		keys("identity", id, "role", "schema", "sha256")
	}
	if d := keys("digest", doc["digest"], "canonicalization", "sha256"); d["canonicalization"] != "tsgk-tree-digest/r1" {
		t.Fatalf("digest %v", d)
	}
	errs := keys("errors", doc["errors"], "limit", "total", "truncated", "items")
	items := errs["items"].([]any)
	if errs["limit"].(float64) != 1000 || float64(len(items)) > errs["limit"].(float64) || errs["truncated"].(bool) != (errs["total"].(float64) > float64(len(items))) {
		t.Fatalf("errors cap %v", errs)
	}
	for _, it := range items {
		m := keys("error item", it, "kind", "type", "start_byte", "end_byte", "start_point", "end_point")
		if m["kind"] != "ERROR" && m["kind"] != "MISSING" {
			t.Fatalf("error kind %v", m["kind"])
		}
		span("error item", m)
	}
	decl := keys("declarations", doc["declarations"], "mapping", "route", "assessment", "items")
	all := true
	for _, it := range decl["items"].([]any) {
		m := keys("declaration", it, "fact", "node_type", "start_byte", "end_byte", "name", "status")
		if !slices.Contains(factKinds[:3], m["fact"].(string)) || !slices.Contains([]string{"PASS", "NAME_MISSING", "HAS_ERROR"}, m["status"].(string)) {
			t.Fatalf("declaration item %v", m)
		}
		if m["name"] != nil {
			keys("declaration name", m["name"], "start_byte", "end_byte")
		}
		all = all && m["status"] == "PASS"
	}
	if decl["mapping"] != "declaration-facts-r1" || (decl["assessment"] == "PASS") != all {
		t.Fatalf("declaration assessment %v", decl["assessment"])
	}
	for _, pt := range doc["partial_trees"].([]any) {
		p := keys("partial tree", pt, "point", "byte", "truncated", "nodes")
		for i, n := range p["nodes"].([]any) {
			m := keys("partial node", n, "parent", "type", "field", "named", "extra", "is_error", "has_error", "is_missing", "start_byte", "end_byte", "start_point", "end_point")
			if parent := int(m["parent"].(float64)); parent >= i || (i == 0) != (parent == -1) {
				t.Fatalf("partial node %d parent %d", i, parent)
			}
			span("partial node", m)
		}
	}
	// Ranges only: no key may carry source, XML value or name text.
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				if slices.Contains([]string{"text", "value", "content", "source_text", "name_text"}, k) {
					t.Fatalf("summary carries text in %q", k)
				}
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(doc)
}
