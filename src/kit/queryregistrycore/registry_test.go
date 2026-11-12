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
	if len(data) != 82475 || Digest(data) != "296511b2bbe91811f1afc2af68b8ea66050867dc4175fa3f4d922a52242b15d5" {
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
	if reg.Namespace != "2d1720e6f5e2d035491486cc80503c101f2b960f5c4d4298d7082d090ec29fbe" || len(reg.RequiredSets) != 58 || required != 106346 || reg.ContentState != "INCOMPLETE" || reg.FullScopeComplete {
		t.Fatalf("P0 registry contract changed: namespace=%s sets=%d required=%d content=%s complete=%t", reg.Namespace, len(reg.RequiredSets), required, reg.ContentState, reg.FullScopeComplete)
	}
}
