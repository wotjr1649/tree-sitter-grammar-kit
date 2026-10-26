package foundation

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func acceptedRole(t *testing.T, root string, grammarOf func(string) (string, []kit.NativeInput)) kit.QualRole {
	t.Helper()
	data := mustRead(t, filepath.Join(root, "src/testdata/native/reuse-holdouts.json"))
	if _, err := kit.ParseReuseAcceptedLedger(data); err != nil {
		t.Fatal(err)
	}
	var rows struct {
		Holdouts []struct {
			ID, Route string
			Source    string `json:"source_utf8"`
			Edits     []struct {
				Start  uint32 `json:"start_byte"`
				OldEnd uint32 `json:"old_end_byte"`
				NewEnd uint32 `json:"new_end_byte"`
				Old    string `json:"old_utf8"`
				New    string `json:"new_utf8"`
			}
		}
	}
	var runtime struct{ Commit string }
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mustRead(t, filepath.Join(root, "src/drivers/native-c/runtime-manifest.json")), &runtime); err != nil {
		t.Fatal(err)
	}
	r := kit.QualRole{ID: "reuse-accepted", Role: "owned", Status: kit.RoleExecuted,
		Reason:    "고정 original history의 accepted 관측과 fresh 전체 CST 동등성; node identity·성능은 주장하지 않는다",
		Platforms: []string{"windows-amd64", "linux-amd64"}, NotApplicable: map[string]string{},
		Deferred: map[string]string{"darwin-arm64": "사용자 결정으로 macOS accepted claim 검증은 제외한다"}}
	for _, control := range []string{"normal", "omit-old-tree"} {
		for _, route := range []string{"tsql", "json"} {
			symbol, grammar := grammarOf(route)
			set := "reuse-accepted-" + route
			if control == "omit-old-tree" {
				set += "-omit"
			}
			w := kit.QualWorkload{Set: set, Profile: set, Route: route, Operation: "native-parse-edit", Output: kit.OutputTree,
				Symbol: symbol, Grammar: grammar, Queries: []kit.QualQuery{}, RuntimeCommit: runtime.Commit,
				ReuseAccepted: &kit.QualReuseAcceptedRegistration{Policy: kit.ReuseAcceptedSchema, LedgerSHA256: kit.ReuseAcceptedLedgerSHA256, Control: control}}
			for _, h := range rows.Holdouts {
				if h.Route != route {
					continue
				}
				source := h.Source
				c := kit.QualCase{ID: h.ID, Role: "support", Source: &source,
					Input: kit.NativeInput{Path: h.ID + ".txt", Role: "case", SHA256: shaHex([]byte(source)), Bytes: uint64(len(source))},
					Edits: []kit.Edit{}, Points: []kit.NativePoint{}, Expect: []kit.StepExpectation{}, QueryExpect: []kit.QueryExpectation{}, Covers: map[string][]string{}, Alternatives: []string{}}
				for _, e := range h.Edits {
					c.Edits = append(c.Edits, kit.Edit{StartByte: e.Start, OldEndByte: e.OldEnd, NewEndByte: e.NewEnd, Old: []byte(e.Old), New: []byte(e.New)})
				}
				w.Cases = append(w.Cases, c)
			}
			r.Workloads = append(r.Workloads, w)
		}
	}
	return r
}
