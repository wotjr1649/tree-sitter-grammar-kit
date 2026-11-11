package queryregistry

import (
	"errors"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestValidateKeepsRegistryPrecedenceAndKitErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		sha  string
		want string
		kind string
	}{
		{"caller-pin-before-registry-json", []byte("{"), strings.Repeat("0", 64), "REGISTRY_CALLER_PIN", kit.KindInvalidInput},
		{"registry-json-before-source-pin", []byte("{"), "", "REGISTRY_JSON", kit.KindInvalidInput},
		{"registry-size-before-source-pin", make([]byte, MaxBytes+1), "", "REGISTRY_SIZE", kit.KindResourceLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sha := tc.sha
			if sha == "" {
				sha = Digest(tc.data)
			}
			_, err := Validate(tc.data, sha, []byte("bad qualification"), []byte("bad slots"), nil)
			var typed *kit.Error
			if !errors.As(err, &typed) || typed.Code != tc.want || typed.Kind != tc.kind || typed.Path != Path {
				t.Fatalf("Validate error = %#v, want %s/%s at %s", err, tc.kind, tc.want, Path)
			}
		})
	}
}
