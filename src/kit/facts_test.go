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

// DeriveDeclarations follows the locator semantics: field:/child: take the first such node
// under the same path, children: keeps every node, names come in document order, a
// declaration without a name is NAME_MISSING and one with an error is HAS_ERROR.
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
		c(0, "decl.1", 10, 0, 50), // class with a name
		c(1, "owner.1", 10, 0, 50), c(1, "name.1", 12, 6, 7),
		c(2, "owner.1", 10, 0, 50), c(2, "name.1", 13, 8, 9), // a second name field child: not the first
		c(3, "decl.0", 20, 10, 30),
		// declarators a and b under the first variable_declaration (node 21); one under a
		// second variable_declaration (node 30) that child: must not take
		c(4, "owner.0", 20, 10, 30), c(4, "s0.0", 21, 12, 29), c(4, "s1.0", 24, 16, 17), c(4, "name.0", 25, 16, 17),
		c(5, "owner.0", 20, 10, 30), c(5, "s0.0", 21, 12, 29), c(5, "s1.0", 22, 13, 14), c(5, "name.0", 23, 13, 14),
		c(6, "owner.0", 20, 10, 30), c(6, "s0.0", 30, 25, 28), c(6, "s1.0", 31, 26, 27), c(6, "name.0", 32, 26, 27),
		c(7, "decl.1", 40, 31, 40), // class without a name
		c(8, "decl.2", 45, 41, 49),
	}
	for i := range caps { // the unnamed class has an error, which NAME_MISSING takes precedence over
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
