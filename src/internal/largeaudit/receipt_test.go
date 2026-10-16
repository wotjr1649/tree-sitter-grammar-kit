package largeaudit

import (
	"os"
	"testing"
)

func TestRegisteredNumberedSources(t *testing.T) {
	data, e := os.ReadFile("../../contracts/native-large-fixtures.json")
	if e != nil {
		t.Fatal(e)
	}
	inv, e := LoadFixtures(data)
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range inv.Fixtures {
		t.Run(f.ID, func(t *testing.T) {
			s, e := Source(f)
			if e != nil || uint64(len(s)) != f.Bytes || digest(s) != f.SHA256 {
				t.Fatalf("registered source: %v", e)
			}
		})
	}
	f := inv.Fixtures[0]
	for name, change := range map[string]func(*Fixture){"bytes": func(f *Fixture) { f.Bytes-- }, "block": func(f *Fixture) { f.Block = "changed {N}" }, "hash": func(f *Fixture) { f.SHA256 = "" }, "repeat": func(f *Fixture) { f.Repeat-- }} {
		t.Run(name, func(t *testing.T) {
			bad := f
			change(&bad)
			if _, e := Source(bad); e == nil {
				t.Fatal("changed registered source accepted")
			}
		})
	}
}
