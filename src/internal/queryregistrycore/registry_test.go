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
	if len(data) != 82475 || Digest(data) != "91e8475fd6a207a1c64af50c63e2456601f59097445790f2523b39c265f2a06c" {
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
	if reg.Namespace != "673312167ae0becb7807b7daf6ee854f08f67a0cf89e6dae411422f3cc2b2148" || len(reg.RequiredSets) != 58 || required != 106346 || reg.ContentState != "INCOMPLETE" || reg.FullScopeComplete {
		t.Fatalf("P0 registry contract changed: namespace=%s sets=%d required=%d content=%s complete=%t", reg.Namespace, len(reg.RequiredSets), required, reg.ContentState, reg.FullScopeComplete)
	}
}
