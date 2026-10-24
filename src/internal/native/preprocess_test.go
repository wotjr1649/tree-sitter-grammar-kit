package native

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// #218: the shared policy preimage keeps every recorded tsgk-native-policy/r1 identity: the
// text kit writes is the one this package wrote before, for every operation and output.
func TestPolicyRefPreimage(t *testing.T) {
	for name, op := range kit.NativeOperations() {
		for _, output := range op.Outputs {
			l := LimitsFor(op)
			old := fmt.Sprintf("tsgk-native-policy/r1\noperation=%s\noutput=%s\nlimits=%+v\nparse_wall=%s\nedit_wall=%s\nmax_edits=%d\nbatch=%t %d %d %s %s\nrun_wall=%s\n",
				op.Name, output, l, op.ParseWall, op.EditWall, op.MaxEdits, op.Batch, op.BatchFiles, op.BatchBytes, op.FrameWall, op.BatchWall, op.RunWall)
			s := sha256.Sum256([]byte(old))
			want := kit.IdentityRef{Role: "policy", Schema: "tsgk-native-policy/r1", SHA256: hex.EncodeToString(s[:])}
			if got := PolicyRef(op, output); got != want || kit.NativePolicyText(op, output) != old {
				t.Errorf("%s %s: %+v, want %+v", name, output, got, want)
			}
			pp := &kit.Preprocess{Mode: kit.PreprocessMode, Symbols: []string{"A"}}
			r3 := kit.NativePolicy(op, output, pp)
			other := kit.NativePolicy(op, output, &kit.Preprocess{Mode: kit.PreprocessMode, Symbols: []string{"B"}})
			if r3.Schema != kit.NativePolicySchemaR4 || r3.SHA256 == want.SHA256 || r3 == other {
				t.Errorf("%s %s: r3 %+v does not bind the preprocess member", name, output, r3)
			}
		}
	}
}
