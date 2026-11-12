package queryregistrycore

import (
	"os"
	"path/filepath"
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
