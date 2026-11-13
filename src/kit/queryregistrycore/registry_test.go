package queryregistrycore

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPinnedP0RegistryIdentitySurvivesCoreExtraction(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "contracts", "query-semantic-registry-r1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 82475 || Digest(data) != "ffd3552dee2d6a9b476b9de76195d2c118bb58de9079ab76dfb1e469d34aa3c8" {
		t.Fatalf("P0 registry bytes/hash changed: %d/%s", len(data), Digest(data))
	}
	reg, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var required uint64
	for _, set := range reg.RequiredSets {
		required += set.Required
	}
	if reg.Namespace != "4059089557e10dfaa3fe68c379d87fbd6c6c8507fa71afe0ea89f6e15407a21b" || len(reg.RequiredSets) != 58 || required != 106346 || reg.ContentState != "INCOMPLETE" || reg.FullScopeComplete {
		t.Fatalf("P0 registry contract changed: namespace=%s sets=%d required=%d content=%s complete=%t", reg.Namespace, len(reg.RequiredSets), required, reg.ContentState, reg.FullScopeComplete)
	}
}

// The reflection-free preimage must equal the Canonical preimage for every shape, including
// the strings it does not handle itself.
func TestCoordinateIDFastPathMatchesCanonical(t *testing.T) {
	plain := Coordinate{Structural, "route-a", "windows-amd64", "windows/amd64", "primary", "set.x", "case-1", "q0", 4294967295, "fresh", "qualification/r8"}
	odd := []string{"", `a"b`, `a\b`, "a\nb", "a\x00b", "a\x7fb", "a<b>&c", "é", "a\u2028b", "a\xffb", "日本語"}
	cases := []Coordinate{plain}
	for i := 0; i < 11; i++ {
		for _, s := range odd {
			c := plain
			f := []*string{&c.Kind, &c.Route, &c.Platform, &c.Subject, &c.Role, &c.Set, &c.Case, &c.Query, &c.TreeRole, &c.Context}
			*f[i%len(f)] = s
			cases = append(cases, c)
		}
	}
	for _, c := range cases {
		for _, ns := range []string{"4059089557e10dfaa3fe68c379d87fbd6c6c8507fa71afe0ea89f6e15407a21b", "", `n"s`} {
			if got, want := c.ID(ns), c.slowID(ns); got != want {
				t.Fatalf("ID %+v ns=%q: %s != %s", c, ns, got, want)
			}
		}
		if got, want := c.Canonical(), c.slowCanonical(); !bytes.Equal(got, want) {
			t.Fatalf("Canonical %+v: %s != %s", c, got, want)
		}
	}
	if _, ok := appendCoordinate(nil, plain); !ok {
		t.Fatal("plain ASCII coordinate must take the fast path")
	}
	// Every single byte, so a loosened or tightened range bound shows up; the printable edges must stay fast.
	for b := 0; b < 256; b++ {
		c := plain
		c.Kind = "a" + string([]byte{byte(b)}) + "b"
		if got, want := c.ID(""), c.slowID(""); got != want {
			t.Fatalf("ID with byte %#02x: %s != %s", b, got, want)
		}
		if _, ok := appendCoordinate(nil, c); ok != (b >= 0x20 && b <= 0x7e && b != '"' && b != '\\') {
			t.Fatalf("byte %#02x: fast path taken=%v", b, ok)
		}
	}
	for _, step := range []uint32{0, 1, 9, 10, 99, 100} {
		c := plain
		c.Step = step
		if got, want := c.ID(""), c.slowID(""); got != want {
			t.Fatalf("ID with step %d: %s != %s", step, got, want)
		}
	}
}

// The fast path and slowCanonical list the fields by hand; a new, removed or reordered field must fail here first.
func TestCoordinateFieldsMatchTheHandWrittenEncoding(t *testing.T) {
	want := []string{"Kind", "Route", "Platform", "Subject", "Role", "Set", "Case", "Query", "Step", "TreeRole", "Context"}
	typ := reflect.TypeOf(Coordinate{})
	if typ.NumField() != len(want) {
		t.Fatalf("Coordinate has %d fields, the hand-written encoding has %d", typ.NumField(), len(want))
	}
	for i, name := range want {
		if typ.Field(i).Name != name {
			t.Fatalf("field %d is %s, the hand-written encoding expects %s", i, typ.Field(i).Name, name)
		}
	}
	if typ.Field(8).Type.Kind() != reflect.Uint32 {
		t.Fatalf("Step is %s, the hand-written encoding formats an unsigned integer", typ.Field(8).Type)
	}
}
