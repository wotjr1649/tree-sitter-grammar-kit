package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// S06-A15: the versioned fact query pack is bound to the declaration-facts mapping: per
// route its declaration items are exactly the mapping's type/member/create items with a
// node, in mapping order, and its declaration query is DeclarationQuery of those items.
func TestFactQueryPack(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "contracts", "fact-query-pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	pack, err := ParseFactPack(data)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Revision != "fact-queries-r1" || pack.Mapping != "declaration-facts-r1" || pack.DynamicSQL != "dynamic-sql-r1" || pack.XML != "xml-structure-r1" {
		t.Fatalf("pack identity %+v", pack)
	}
	mdata, err := os.ReadFile(filepath.Join("..", "contracts", "fact-mapping.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []struct {
			Route string `json:"route"`
			Facts []struct {
				Fact string `json:"fact"`
				Node string `json:"node"`
				Name string `json:"name"`
			} `json:"facts"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(mdata, &m); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"csharp": {"facts.declarations", "facts.dynamic_sql"}, "tsql": {"facts.declarations", "facts.dynamic_sql"}, "postgresql-sql": {"facts.declarations"}, "xml": {"facts.xml_structure"}}
	if len(pack.Routes) != len(want) {
		t.Fatalf("routes %d", len(pack.Routes))
	}
	for _, r := range pack.Routes {
		var ids []string
		for _, q := range r.Queries {
			ids = append(ids, q.ID)
		}
		if !slices.Equal(ids, want[r.Route]) {
			t.Fatalf("%s queries %v", r.Route, ids)
		}
		var items []NativeDeclaration
		for _, mr := range m.Routes {
			if mr.Route != r.Route {
				continue
			}
			for _, f := range mr.Facts {
				if f.Node != "" && (f.Fact == "type_declaration" || f.Fact == "member_declaration" || f.Fact == "create_object") {
					items = append(items, NativeDeclaration{f.Fact, f.Node, f.Name})
				}
			}
		}
		if !slices.Equal(items, r.Declarations) {
			t.Fatalf("%s declarations differ from the mapping", r.Route)
		}
		if len(items) > 0 && r.Queries[0].Source != DeclarationQuery(items) {
			t.Fatalf("%s declaration query is not DeclarationQuery of its items", r.Route)
		}
	}
	// a changed byte is a different pack identity
	if p2, err := ParseFactPack(append(data, ' ')); err != nil || p2.SHA256 == pack.SHA256 {
		t.Fatalf("pack identity does not bind its bytes: %v", err)
	}
}

// DeriveDeclarations follows the S05 locator walk: field:/child: take the first candidate
// below the current node whether or not deeper levels exist below it, children: keeps
// every candidate, names come in document order, a declaration without a name is
// NAME_MISSING (taking precedence over HAS_ERROR), and items follow declaration preorder.
func TestDeriveDeclarations(t *testing.T) {
	items := []NativeDeclaration{
		{"member_declaration", "field_declaration", "child:variable_declaration/children:variable_declarator/field:name"},
		{"type_declaration", "class_declaration", "field:name"},
		{"member_declaration", "indexer_declaration", "node"},
	}
	c := func(match uint32, name string, node int64, start, end uint32) Capture {
		return Capture{Match: match, Name: name, Node: node, StartByte: start, EndByte: end}
	}
	caps := []Capture{
		// class with two name-field children: the first is the name
		c(0, "c.1.0.0", 10, 0, 9),
		c(1, "c.1.1.0", 10, 0, 9), c(1, "c.1.1.1", 12, 6, 7),
		c(2, "c.1.1.0", 10, 0, 9), c(2, "c.1.1.1", 13, 8, 9),
		// field: the first declaration (21) has two declarators; a second declaration (30) is not taken
		c(3, "c.0.0.0", 20, 10, 30),
		c(4, "c.0.1.0", 20, 10, 30), c(4, "c.0.1.1", 21, 12, 24),
		c(5, "c.0.1.0", 20, 10, 30), c(5, "c.0.1.1", 30, 25, 29),
		c(6, "c.0.3.1", 21, 12, 24), c(6, "c.0.3.2", 22, 13, 14), c(6, "c.0.3.3", 23, 13, 14),
		c(7, "c.0.3.1", 21, 12, 24), c(7, "c.0.3.2", 24, 16, 17), c(7, "c.0.3.3", 25, 16, 17),
		c(8, "c.0.3.1", 30, 25, 29), c(8, "c.0.3.2", 31, 26, 27), c(8, "c.0.3.3", 32, 26, 27),
		// class without a name, with an error
		c(9, "c.1.0.0", 40, 31, 40),
		c(10, "c.2.0.0", 45, 41, 49),
		// a field whose first declaration has no named declarator: S05 stops there
		c(11, "c.0.0.0", 50, 50, 70),
		c(12, "c.0.1.0", 50, 50, 70), c(12, "c.0.1.1", 51, 52, 55),
		c(13, "c.0.1.0", 50, 50, 70), c(13, "c.0.1.1", 53, 56, 69),
		c(14, "c.0.3.1", 53, 56, 69), c(14, "c.0.3.2", 54, 57, 58), c(14, "c.0.3.3", 55, 57, 58),
	}
	for i := range caps {
		caps[i].HasError = caps[i].Node == 40
	}
	got := DeriveDeclarations(items, caps)
	want := []struct {
		node   string
		start  uint32
		name   *ByteSpan
		status string
	}{
		{"class_declaration", 0, &ByteSpan{6, 7}, "PASS"},
		{"field_declaration", 10, &ByteSpan{13, 14}, "PASS"},
		{"field_declaration", 10, &ByteSpan{16, 17}, "PASS"},
		{"class_declaration", 31, nil, "NAME_MISSING"},
		{"indexer_declaration", 41, nil, "PASS"},
		{"field_declaration", 50, nil, "NAME_MISSING"},
	}
	if len(got) != len(want) {
		t.Fatalf("items %+v", got)
	}
	for i, w := range want {
		g := got[i]
		if g.NodeType != w.node || g.StartByte != w.start || g.Status != w.status || (g.Name == nil) != (w.name == nil) || (g.Name != nil && *g.Name != *w.name) {
			t.Fatalf("item %d: %+v, want %+v", i, g, w)
		}
	}
}

// One match can hold two nodes for one level: the runtime returned both names of
// `CREATE TABLE m_2024 PARTITION OF m` (nodes 99 and 104) in a single match. The name is
// the first in document order, as when the nodes come in separate matches.
func TestDeriveDeclarationsSameLevelInOneMatch(t *testing.T) {
	items := []NativeDeclaration{{"create_object", "CreateStmt", "child:qualified_name"}}
	c := func(match uint32, name string, node int64, start, end uint32) Capture {
		return Capture{Match: match, Name: name, Node: node, StartByte: start, EndByte: end}
	}
	for _, order := range [][2]int64{{99, 104}, {104, 99}} {
		span := map[int64][2]uint32{99: {170, 176}, 104: {190, 191}}
		caps := []Capture{
			c(2, "c.0.0.0", 96, 157, 240),
			c(3, "c.0.1.0", 96, 157, 240),
			c(3, "c.0.1.1", order[0], span[order[0]][0], span[order[0]][1]),
			c(3, "c.0.1.1", order[1], span[order[1]][0], span[order[1]][1]),
		}
		got := DeriveDeclarations(items, caps)
		if len(got) != 1 || got[0].Name == nil || *got[0].Name != (ByteSpan{170, 176}) || got[0].Status != "PASS" {
			t.Fatalf("capture order %v: %+v", order, got)
		}
	}
	// Two sibling parents on one level of a match (children: walks both): each candidate
	// links only to its own parent. A1 [0,10) has B1 [4,5) and the zero-width B0 at the
	// shared boundary 10 (node 13 < A2's node 14, so A1's); A2 [10,20) has none, so it
	// yields no name. Linking by range alone would give A2 the name of B0, without any
	// rule A2 would also get B1.
	items = []NativeDeclaration{{"member_declaration", "field_declaration", "children:variable_declarator/field:name"}}
	caps := []Capture{
		c(0, "c.0.0.0", 10, 0, 20),
		c(1, "c.0.2.0", 10, 0, 20),
		c(1, "c.0.2.1", 11, 0, 10), c(1, "c.0.2.1", 14, 10, 20),
		c(1, "c.0.2.2", 12, 4, 5), c(1, "c.0.2.2", 13, 10, 10),
	}
	got := DeriveDeclarations(items, caps)
	if len(got) != 1 || got[0].Name == nil || *got[0].Name != (ByteSpan{4, 5}) {
		t.Fatalf("sibling parents: %+v", got)
	}
}
