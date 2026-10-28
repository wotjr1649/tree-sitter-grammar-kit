package foundation

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const querySlotInventoryPath = "src/contracts/query-slot-inventory-r1.json"
const querySlotInventoryMaxBytes = 256 * 1024

type slotSourcePin struct {
	Path   string `json:"path"`
	Bytes  uint64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type slotCounts struct {
	Cases          uint64 `json:"cases"`
	Edits          uint64 `json:"edits"`
	CaseSteps      uint64 `json:"case_steps"`
	TreeRoles      uint64 `json:"tree_roles"`
	PotentialSlots uint64 `json:"potential_query_tree_roles"`
}

type slotQuery struct {
	ID               string `json:"id"`
	SHA256           string `json:"sha256"`
	SourceFile       string `json:"source_file"`
	Class            string `json:"class"`
	CompiledMetadata string `json:"compiled_metadata"`
}

type slotGroup struct {
	Route     string      `json:"route"`
	Set       string      `json:"set"`
	Profile   string      `json:"profile"`
	Operation string      `json:"operation"`
	Counts    slotCounts  `json:"counts"`
	Queries   []slotQuery `json:"queries"`
}

type slotExtraWorkload struct {
	Set       string          `json:"set"`
	Profile   string          `json:"profile"`
	Route     string          `json:"route"`
	Operation string          `json:"operation"`
	Format    string          `json:"format"`
	Cases     uint64          `json:"cases"`
	Edits     uint64          `json:"edits"`
	Queries   []kit.QualQuery `json:"queries"`
}

type slotExtra struct {
	ID               string              `json:"id"`
	Role             string              `json:"role"`
	RegisteredStatus string              `json:"registered_status"`
	Reason           string              `json:"reason"`
	Platforms        []string            `json:"platforms"`
	NotApplicable    map[string]string   `json:"not_applicable"`
	Deferred         map[string]string   `json:"deferred"`
	Class            string              `json:"class"`
	Workloads        []slotExtraWorkload `json:"workloads"`
}

type slotTreeRole struct {
	Step uint32
	Role string
}

func querySlotTreeRoles(edits int) []slotTreeRole {
	roles := []slotTreeRole{{0, "incremental"}}
	for step := 1; step <= edits; step++ {
		roles = append(roles, slotTreeRole{uint32(step), "incremental"}, slotTreeRole{uint32(step), "fresh"})
	}
	return roles
}

type querySlotInventory struct {
	Coordinate        []string        `json:"coordinate"`
	SemanticPlatforms []string        `json:"semantic_platform_scope"`
	MacOS             string          `json:"new_macos_validation"`
	Schema            string          `json:"schema"`
	Scope             string          `json:"scope"`
	Assessment        string          `json:"assessment"`
	SemanticRequired  *uint64         `json:"semantic_required"`
	ExpansionRule     string          `json:"expansion_rule"`
	Qualification     slotSourcePin   `json:"qualification_inventory"`
	QuerySources      []slotSourcePin `json:"query_sources"`
	Totals            slotCounts      `json:"ordinary_totals"`
	Ordinary          []slotGroup     `json:"ordinary"`
	Extra             []slotExtra     `json:"extra_roles"`
}

func querySlotSourceClass(q kit.QualQuery, text string) (class, code string) {
	if shaHex([]byte(text)) != q.SHA256 {
		return "", "QUERY_SOURCE_HASH"
	}
	// A new predicate requires explicit classification, not an automatic semantic upgrade.
	if strings.Contains(text, "#") {
		return "", "QUERY_CLASSIFICATION_REQUIRED"
	}
	// Only locate reserved predicate heads; compiled metadata remains unverified.
	afterOpen, quoted, escaped, comment := false, false, false, false
	for _, r := range text {
		if comment {
			if r == '\n' {
				comment = false
			}
			continue
		}
		if quoted {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
			} else if r == '"' {
				quoted = false
			}
			continue
		}
		if r == ';' {
			comment = true
			continue
		}
		if unicode.IsSpace(r) {
			continue
		}
		if afterOpen && r == '.' {
			return "", "QUERY_CLASSIFICATION_REQUIRED"
		}
		afterOpen = r == '('
		if r == '"' {
			quoted = true
		}
	}
	return "STRUCTURAL_CANDIDATE", ""
}

func expectedQuerySlotInventory(t *testing.T) querySlotInventory {
	t.Helper()
	root := repository(t)
	read := func(path string) []byte { return mustRead(t, filepath.Join(root, filepath.FromSlash(path))) }
	pin := func(path string, data []byte) slotSourcePin {
		return slotSourcePin{path, uint64(len(data)), shaHex(data)}
	}
	raw := read(inventoryPath)
	inv, err := kit.ParseQualificationInventory(raw)
	if err != nil {
		t.Fatal(err)
	}
	result := querySlotInventory{Coordinate: []string{"qualification_inventory.sha256", "set", "case", "query", "step", "tree_role"}, SemanticPlatforms: []string{"windows/amd64", "linux/amd64"}, MacOS: "EXCLUDED", Schema: "tsgk-query-slot-inventory/r1", Scope: "STATIC_CLASSIFICATION", Assessment: kit.AssessNotAssessed, ExpansionRule: "case-query-edit-tree-role/r1", Qualification: pin(inventoryPath, raw), QuerySources: []slotSourcePin{}, Ordinary: []slotGroup{}, Extra: []slotExtra{}}
	type source struct{ text, path string }
	sources := map[string]source{}
	add := func(route, id, text, path string) {
		key := route + "/" + id
		if _, ok := sources[key]; ok {
			t.Fatal("duplicate query source", key)
		}
		sources[key] = source{text, path}
	}
	for _, route := range inv.Routes {
		path := "src/testdata/native/queries/" + route.Route + ".json"
		data := read(path)
		result.QuerySources = append(result.QuerySources, pin(path, data))
		var file queryFile
		if err := jsonv2.Unmarshal(data, &file); err != nil {
			t.Fatal(path, err)
		}
		for _, q := range file.Queries {
			add(route.Route, q.ID, q.Source, path)
		}
	}
	packPath := "src/contracts/fact-query-pack.json"
	packData := read(packPath)
	result.QuerySources = append(result.QuerySources, pin(packPath, packData))
	var pack struct {
		Routes []struct {
			Route   string `json:"route"`
			Queries []struct {
				ID     string `json:"id"`
				Source string `json:"source"`
			} `json:"queries"`
		} `json:"routes"`
	}
	if err := jsonv2.Unmarshal(packData, &pack); err != nil {
		t.Fatal(err)
	}
	for _, r := range pack.Routes {
		for _, q := range r.Queries {
			add(r.Route, q.ID, q.Source, packPath)
		}
	}
	used := map[string]bool{}
	for _, route := range inv.Routes {
		w := route.Workload
		group := slotGroup{Route: route.Route, Set: w.Set, Profile: w.Profile, Operation: w.Operation, Queries: []slotQuery{}}
		for _, q := range w.Queries {
			key := route.Route + "/" + q.ID
			s, ok := sources[key]
			if !ok || used[key] {
				t.Fatal("unbound/duplicate query source", key)
			}
			class, code := querySlotSourceClass(q, s.text)
			if code != "" {
				t.Fatal(code, key)
			}
			used[key] = true
			group.Queries = append(group.Queries, slotQuery{q.ID, q.SHA256, s.path, class, "UNVERIFIED"})
		}
		group.Counts.Cases = uint64(len(w.Cases))
		for _, c := range w.Cases {
			edits := uint64(len(c.Edits))
			group.Counts.Edits += edits
			group.Counts.CaseSteps += 1 + edits
			group.Counts.TreeRoles += uint64(len(querySlotTreeRoles(len(c.Edits))))
		}
		group.Counts.PotentialSlots = group.Counts.TreeRoles * uint64(len(w.Queries))
		result.Totals.Cases += group.Counts.Cases
		result.Totals.Edits += group.Counts.Edits
		result.Totals.CaseSteps += group.Counts.CaseSteps
		result.Totals.TreeRoles += group.Counts.TreeRoles
		result.Totals.PotentialSlots += group.Counts.PotentialSlots
		result.Ordinary = append(result.Ordinary, group)
	}
	if len(used) != len(sources) {
		t.Fatal("query source omitted from ordinary classification")
	}
	for _, r := range inv.ExtraRoles {
		extra := slotExtra{ID: r.ID, Role: r.Role, RegisteredStatus: r.Status, Reason: r.Reason, Platforms: r.Platforms, NotApplicable: r.NotApplicable, Deferred: r.Deferred, Class: "NO_REGISTERED_QUERIES"}
		if r.Workloads != nil {
			extra.Workloads = []slotExtraWorkload{}
		}
		for _, w := range r.Workloads {
			edits := uint64(0)
			for _, c := range w.Cases {
				edits += uint64(len(c.Edits))
			}
			extra.Workloads = append(extra.Workloads, slotExtraWorkload{w.Set, w.Profile, w.Route, w.Operation, w.Format, uint64(len(w.Cases)), edits, w.Queries})
			if len(w.Queries) > 0 {
				extra.Class = "CONTEXT_NOT_EXPANDED"
			}
		}
		result.Extra = append(result.Extra, extra)
	}
	return result
}

func checkQuerySlotInventory(data, want []byte) error {
	if len(data) > querySlotInventoryMaxBytes {
		return fmt.Errorf("slot inventory exceeds 256 KiB")
	}
	var typed querySlotInventory
	if err := jsonv2.Unmarshal(data, &typed, jsonv2.RejectUnknownMembers(true)); err != nil {
		return err
	}
	// Compare JSON shapes too: omitted zero/null fields must not equal present evidence.
	var got, expected any
	if err := jsonv2.Unmarshal(data, &got); err != nil {
		return err
	}
	if err := jsonv2.Unmarshal(want, &expected); err != nil {
		return err
	}
	if !reflect.DeepEqual(got, expected) {
		return fmt.Errorf("slot inventory differs from its pinned registration and classification")
	}
	return nil
}

func TestQuerySlotInventory(t *testing.T) {
	want := expectedQuerySlotInventory(t)
	data, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join(repository(t), filepath.FromSlash(querySlotInventoryPath))
	if os.Getenv("TSGK_WRITE_INVENTORY") == "1" {
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkQuerySlotInventory(mustRead(t, path), data); err != nil {
		t.Fatal(err)
	}
	if want.Totals != (slotCounts{3201, 1852, 5053, 6905, 53104}) || len(want.Ordinary) != 26 || len(want.Extra) != 8 || want.SemanticRequired != nil {
		t.Fatal("static scope changed; review its obligations", want.Totals)
	}
	t.Logf("static routes=%d cases=%d potential_slots=%d; semantic denominator unregistered, metadata unverified", len(want.Ordinary), want.Totals.Cases, want.Totals.PotentialSlots)
}

func TestQuerySlotInventoryControls(t *testing.T) {
	want, err := json.Marshal(expectedQuerySlotInventory(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing-route", "duplicate-route", "missing-query", "duplicate-query", "query-hash", "source-pin", "count", "false-class", "false-pass", "invented-denominator", "missing-null", "lost-deferred-reason", "unknown-field", "duplicate-field", "oversize"} {
		t.Run(name, func(t *testing.T) {
			var doc map[string]any
			if err := json.Unmarshal(want, &doc); err != nil {
				t.Fatal(err)
			}
			groups := doc["ordinary"].([]any)
			first := groups[0].(map[string]any)
			queries := first["queries"].([]any)
			switch name {
			case "missing-route":
				doc["ordinary"] = groups[1:]
			case "duplicate-route":
				groups[1] = groups[0]
			case "missing-query":
				first["queries"] = queries[1:]
			case "duplicate-query":
				queries[1] = queries[0]
			case "query-hash":
				queries[0].(map[string]any)["sha256"] = strings.Repeat("0", 64)
			case "source-pin":
				doc["qualification_inventory"].(map[string]any)["sha256"] = strings.Repeat("0", 64)
			case "count":
				first["counts"].(map[string]any)["potential_query_tree_roles"] = float64(0)
			case "false-class":
				queries[0].(map[string]any)["class"] = "EVALUATED"
			case "false-pass":
				doc["assessment"] = "PASS"
			case "invented-denominator":
				doc["semantic_required"] = float64(53104)
			case "missing-null":
				delete(doc, "semantic_required")
			case "lost-deferred-reason":
				changed := false
				for _, raw := range doc["extra_roles"].([]any) {
					extra := raw.(map[string]any)
					if extra["id"] == "csharp-symbols" {
						extra["deferred"].(map[string]any)["darwin-arm64"] = ""
						changed = true
					}
				}
				if !changed {
					t.Fatal("missing deferred symbol role")
				}
			case "unknown-field":
				doc["completion"] = "PASS"
			}
			data, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if name == "duplicate-field" {
				data = append([]byte(`{"schema":"tsgk-query-slot-inventory/r1",`), data[1:]...)
			}
			if name == "oversize" {
				data = append(data, []byte(strings.Repeat(" ", querySlotInventoryMaxBytes))...)
			}
			if err := checkQuerySlotInventory(data, want); err == nil {
				t.Fatal("unsafe inventory accepted", name)
			}
		})
	}
}

func TestQuerySlotTreeRoles(t *testing.T) {
	for _, test := range []struct {
		edits int
		want  []slotTreeRole
	}{
		{0, []slotTreeRole{{0, "incremental"}}},
		{1, []slotTreeRole{{0, "incremental"}, {1, "incremental"}, {1, "fresh"}}},
	} {
		if got := querySlotTreeRoles(test.edits); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("edits %d: %+v", test.edits, got)
		}
	}
}

func TestQuerySlotSourceClassification(t *testing.T) {
	for _, test := range []struct {
		name, text, class, code string
		wrongHash               bool
	}{
		{"plain", `(identifier) @name`, "STRUCTURAL_CANDIDATE", "", false},
		{"predicate", `((identifier) @name (#eq? @name "C"))`, "", "QUERY_CLASSIFICATION_REQUIRED", false},
		{"source-hash", `(identifier) @name`, "", "QUERY_SOURCE_HASH", true},
		{"dot-predicate", "((identifier) @name (.eq? @name \"C\"))", "", "QUERY_CLASSIFICATION_REQUIRED", false},
		{"dot-whitespace", "((identifier) @name (\n\t.eq? @name \"C\"))", "", "QUERY_CLASSIFICATION_REQUIRED", false},
		{"dot-comment", "((identifier) @name (; predicate\r\n ; second\n .eq? @name \"C\"))", "", "QUERY_CLASSIFICATION_REQUIRED", false},
		{"dot-unicode-space", "((identifier) @name ( .eq? @name \"C\"))", "", "QUERY_CLASSIFICATION_REQUIRED", false},
		{"dot-directive", "((identifier) @name (.set! test \"value\"))", "", "QUERY_CLASSIFICATION_REQUIRED", false},
		{"dot-malformed", "(. eq? @name \"C\")", "", "QUERY_CLASSIFICATION_REQUIRED", false},
		{"capture-dot", "(identifier) @context.name", "STRUCTURAL_CANDIDATE", "", false},
		{"sibling-anchor", "(array . (identifier) @name)", "STRUCTURAL_CANDIDATE", "", false},
		{"trailing-anchor", "(array (identifier) @name .)", "STRUCTURAL_CANDIDATE", "", false},
		{"group-anchor", "((comment) @a . (comment) @b)", "STRUCTURAL_CANDIDATE", "", false},
		{"quoted-dot", "\"(.eq?)\" @token", "STRUCTURAL_CANDIDATE", "", false},
		{"escaped-quote", "\"\\\"(.eq?)\" @token", "STRUCTURAL_CANDIDATE", "", false},
		{"comment-dot", "; (.eq? fake)\n(identifier) @name", "STRUCTURAL_CANDIDATE", "", false},
		{"even-backslashes", "(\"x\\\\\" @name (.eq? @name \"C\"))", "", "QUERY_CLASSIFICATION_REQUIRED", false},
		{"hash-in-string", "\"#\" @token", "", "QUERY_CLASSIFICATION_REQUIRED", false},
		{"hash-in-comment", "; # marker\n(identifier) @name", "", "QUERY_CLASSIFICATION_REQUIRED", false},
		{"source-hash-dot", "((identifier) @name (.eq? @name \"C\"))", "", "QUERY_SOURCE_HASH", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			q := kit.QualQuery{ID: "test", SHA256: shaHex([]byte(test.text))}
			if test.wrongHash {
				q.SHA256 = strings.Repeat("0", 64)
			}
			class, code := querySlotSourceClass(q, test.text)
			if class != test.class || code != test.code {
				t.Fatalf("class=%q code=%q expected class=%q code=%q", class, code, test.class, test.code)
			}
		})
	}
}
