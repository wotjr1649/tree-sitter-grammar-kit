package kit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

func TestPreprocessCSTHashes(t *testing.T) {
	feff := []byte("#\uFEFFif A\nclass X {}\n#endif\n")
	if got, err := preprocessCSTHashes(feff, []TreeNode{{Type: "#if", StartByte: 0, EndByte: 6}, {Type: "#endif", StartByte: 20, EndByte: 26}}); err != nil || !reflect.DeepEqual(got, []uint32{0, 20}) {
		t.Fatalf("pinned feff-after-hash positions: %v %v", got, err)
	}
	if _, err := preprocessCSTHashes([]byte("#\nif"), []TreeNode{{Type: "#if", StartByte: 0, EndByte: 4}}); err == nil {
		t.Fatal("newline inside directive keyword prefix accepted")
	}
	source := []byte("\xef\xbb\xbf\r\n#\u2003if true\r\n#!x\n#:sdk x\n#endif")
	start := uint32(strings.Index(string(source), "#"))
	alias1 := uint32(strings.Index(string(source), "#!"))
	alias2 := uint32(strings.Index(string(source), "#:"))
	end := uint32(strings.Index(string(source), "#endif"))
	nodes := []TreeNode{
		{Type: "preproc_if", StartByte: start, EndByte: uint32(len(source))},
		{Type: "#endif", StartByte: end, EndByte: end + 6, IsError: true},
		{Type: "#if", StartByte: start, EndByte: start + 7},
		{Type: "#if", StartByte: start, EndByte: start + 7},
		{Type: "#else", StartByte: start, EndByte: start + 1, IsMissing: true},
		{Type: "#else", StartByte: start, EndByte: start},
		{Type: "shebang_directive", StartByte: alias1, EndByte: alias1 + 3},
		{Type: "file_based_app_directive", StartByte: alias2, EndByte: alias2 + 7},
	}
	want := []uint32{start, alias1, alias2, end}
	got, err := preprocessCSTHashes(source, nodes)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%v %v; want %v", got, err, want)
	}
	for _, n := range []TreeNode{{Type: "#if", StartByte: 0, EndByte: 3}, {Type: "#if", StartByte: start, EndByte: 1000}, {Type: "#else", StartByte: start, EndByte: start + 7}} {
		if _, err := preprocessCSTHashes(source, []TreeNode{n}); err == nil {
			t.Fatalf("malformed token accepted: %+v", n)
		}
	}
}

// The recorded registered grammar supplies valid wire evidence; each mutation changes
// one trust boundary and keeps the rest, including raw/process byte binding, coherent.
func TestPreprocessCSTPrimary(t *testing.T) {
	base := filepath.Join("..", "testdata", "qualify-symbols", "s05-csharp-symbols-switch")
	read := func(path string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(base, path))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	profile, err := ParseIncrementalProfile(read("profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Platform, ObservationRunID string
		Build                      struct{ Identity string }
		Cases                      []struct {
			PreprocessCST *PreprocessCSTResult `json:"preprocess_cst"`
		}
	}
	// The external names have underscores, independent of Go's field-name matching.
	var rawTop map[string]json.RawMessage
	if err := json.Unmarshal(read("result.json"), &rawTop); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawTop["platform"], &result.Platform); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawTop["observation_run_id"], &result.ObservationRunID); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawTop["build"], &result.Build); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawTop["cases"], &result.Cases); err != nil {
		t.Fatal(err)
	}
	c := profile.Cases[0]
	stem := "00000-" + c.ID
	newRequest := func() PreprocessCSTRequest {
		b, _ := json.Marshal(result.Cases[0].PreprocessCST.Primary)
		var p PreprocessPrimary
		if err := json.Unmarshal(b, &p); err != nil {
			t.Fatal(err)
		}
		return PreprocessCSTRequest{Case: c, Source: read("sources/" + stem + ".bin"), Preprocess: profile.Preprocess,
			Raw: read("primary/responses/" + stem + ".json"), Primary: &p, Operation: NativeOperations()["native-parse-edit"],
			Platform: result.Platform, RunID: result.ObservationRunID, ProducerSHA256: result.Build.Identity}
	}
	changeWire := func(r *PreprocessCSTRequest, change func(*nativewire.Response)) {
		t.Helper()
		w, err := nativewire.DecodeResponse(r.Raw)
		if err != nil {
			t.Fatal(err)
		}
		change(&w)
		r.Raw, err = json.Marshal(w)
		if err != nil {
			t.Fatal(err)
		}
		// Protocol r3 omits these r2-only members; a Go nil raw value marshals as null.
		r.Raw = bytes.ReplaceAll(r.Raw, []byte(`,"queries":null,"api":null`), nil)
		if _, err := nativewire.DecodeResponse(r.Raw); err != nil {
			t.Fatal("mutated wire encoding", err)
		}
		r.Primary.Process.StdoutBytes = int64(len(r.Raw)) + 4
	}
	redigest := func(w *nativewire.WireTree) {
		t.Helper()
		nodes, err := nativewire.Nodes(w.Nodes, w.Types, w.Fields)
		if err != nil {
			t.Fatal(err)
		}
		w.Digest = nativewire.TreeDigest(nodes)
	}
	hideDirective := func(w *nativewire.Response) {
		t.Helper()
		for i, name := range w.Steps[0].Incremental.Types {
			if name == "#if" {
				w.Steps[0].Incremental.Types[i] = "hidden_directive"
				redigest(&w.Steps[0].Incremental)
				return
			}
		}
		t.Fatal("fixture lacks independent #if anchor")
	}
	for name, tc := range map[string]struct {
		want, code string
		change     func(*PreprocessCSTRequest)
	}{
		"complete": {"PASS", "", func(*PreprocessCSTRequest) {}},
		"linux hard scope": {"PASS", "", func(r *PreprocessCSTRequest) {
			r.Platform = "linux/amd64"
			r.Primary.Process.CleanupScope = "CGROUP_KILL"
		}},
		"linux generic scope refused": {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) { r.Platform = "linux/amd64"; r.Primary.Process.CleanupScope = "CGROUP" }},
		"linux sampled scope refused": {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) {
			r.Platform = "linux/amd64"
			r.Primary.Process.CleanupScope = "PROCESS_GROUP"
			r.Primary.Process.MemoryEnforcement = "SAMPLED"
		}},
		"darwin sampled scope": {"PASS", "", func(r *PreprocessCSTRequest) {
			r.Platform = "darwin/arm64"
			r.Primary.Process.CleanupScope = "PROCESS_GROUP"
			r.Primary.Process.MemoryEnforcement = "SAMPLED"
		}},
		"linux mismatch scope": {"MISMATCH", PreprocessCSTPrefix + "0", func(r *PreprocessCSTRequest) {
			r.Platform = "linux/amd64"
			r.Primary.Process.CleanupScope = "CGROUP_KILL"
			changeWire(r, hideDirective)
		}},
		"earliest mismatch": {"MISMATCH", PreprocessCSTPrefix + "0", func(r *PreprocessCSTRequest) { changeWire(r, hideDirective) }},
		"later equality overrides mismatch": {"FAIL", "PREPROCESS_CST_EQUALITY_FAILED", func(r *PreprocessCSTRequest) {
			changeWire(r, func(w *nativewire.Response) {
				hideDirective(w)
				f := w.Steps[1].Fresh
				f.Types[f.Nodes[0][1]] = "different_root"
				redigest(f)
			})
		}},
		"later source overrides mismatch": {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) {
			changeWire(r, func(w *nativewire.Response) { hideDirective(w); w.Steps[1].SourceSHA256 = strings.Repeat("f", 64) })
		}},
		"complete logger required": {"UNVERIFIED", "PREPROCESS_CST_PRIMARY_UNVERIFIED", func(r *PreprocessCSTRequest) {
			changeWire(r, func(w *nativewire.Response) {
				var obs []reuseobs.Observation
				if err := json.Unmarshal(w.Steps[1].Observations, &obs); err != nil {
					t.Fatal(err)
				}
				obs[0].Status, obs[0].Reason, obs[0].Counts = "TRUNCATED", "MESSAGE_LIMIT", &reuseobs.Counts{ParseAfterEdit: 1}
				if reuseobs.Mandatory(obs, true) != "PASS" || reuseobs.Judge(obs) != "BLOCKED" {
					t.Fatal("not a valid start-only prefix")
				}
				w.Steps[1].Observations, _ = json.Marshal(obs)
			})
		}},
		"summary is insufficient": {"UNVERIFIED", "PREPROCESS_CST_PRIMARY_UNVERIFIED", func(r *PreprocessCSTRequest) {
			changeWire(r, func(w *nativewire.Response) {
				tr := &w.Steps[0].Incremental
				form := "summary"
				tr.Form = &form
				tr.Nodes = nil
				tr.Errors = &nativewire.WireErrors{Limit: 16, Items: []nativewire.WireError{}}
			})
		}},
		"missing fresh fails": {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) { changeWire(r, func(w *nativewire.Response) { w.Steps[1].Fresh = nil }) }},
		"run identity":        {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) { r.RunID = "another-run" }},
		"producer identity":   {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) { r.ProducerSHA256 = strings.Repeat("f", 64) }},
		"source identity":     {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) { r.Source[0] ^= 1 }},
		"history identity":    {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) { r.Case.Edits = nil }},
		"role":                {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) { r.Primary.Role = "active" }},
		"policy":              {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) { r.Primary.Policy.SHA256 = strings.Repeat("f", 64) }},
		"frame header bytes":  {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) { r.Primary.Process.StdoutBytes -= 4 }},
		"cleanup unavailable": {"UNVERIFIED", "PREPROCESS_CST_PRIMARY_UNVERIFIED", func(r *PreprocessCSTRequest) { r.Primary.Process.CleanupVerified = false }},
		"foreign cleanup":     {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) { r.Primary.Process.CleanupScope = "PROCESS_GROUP" }},
		"memory hit":          {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) { r.Primary.Process.MemoryLimitHit = true }},
		"wall budget":         {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) { r.Primary.Process.WallBudgetMillis = 0 }},
		"partial timeout output": {"UNVERIFIED", "PREPROCESS_CST_PRIMARY_UNVERIFIED", func(r *PreprocessCSTRequest) {
			r.Raw = nil
			r.Primary.ExecutionStatus, r.Primary.Code = StatusResourceLimit, "WALL_LIMIT"
			r.Primary.Process.Status, r.Primary.Process.Reason = StatusResourceLimit, "WALL_LIMIT"
			r.Primary.Process.StdoutBytes = 17
		}},
		"output cap counts received bytes": {"UNVERIFIED", "PREPROCESS_CST_PRIMARY_UNVERIFIED", func(r *PreprocessCSTRequest) {
			r.Raw = nil
			r.Primary.ExecutionStatus, r.Primary.Code = StatusResourceLimit, "OUTPUT_LIMIT"
			r.Primary.Process.Status, r.Primary.Process.Reason = StatusResourceLimit, "OUTPUT_LIMIT"
			r.Primary.Process.StdoutBytes, r.Primary.Process.StdoutTruncated = int64(r.Operation.OutputBytes)+5, true
		}},
		"output count flag contradiction": {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) {
			r.Raw = nil
			r.Primary.ExecutionStatus, r.Primary.Code = StatusResourceLimit, "OUTPUT_LIMIT"
			r.Primary.Process.Status, r.Primary.Process.Reason = StatusResourceLimit, "OUTPUT_LIMIT"
			r.Primary.Process.StdoutBytes = int64(r.Operation.OutputBytes) + 5
		}},
		"output truncated below cap": {"FAIL", "PREPROCESS_CST_PRIMARY_INVALID", func(r *PreprocessCSTRequest) {
			r.Raw = nil
			r.Primary.ExecutionStatus, r.Primary.Code = StatusResourceLimit, "OUTPUT_LIMIT"
			r.Primary.Process.Status, r.Primary.Process.Reason = StatusResourceLimit, "OUTPUT_LIMIT"
			r.Primary.Process.StdoutBytes, r.Primary.Process.StdoutTruncated = 17, true
		}},
		"first terminal precedes memory flag": {"UNVERIFIED", "PREPROCESS_CST_PRIMARY_UNVERIFIED", func(r *PreprocessCSTRequest) {
			r.Raw = nil
			r.Primary.ExecutionStatus, r.Primary.Code = StatusResourceLimit, "WALL_LIMIT"
			r.Primary.Process.Status, r.Primary.Process.Reason = StatusResourceLimit, "WALL_LIMIT"
			r.Primary.Process.StdoutBytes, r.Primary.Process.MemoryLimitHit = 0, true
		}},
		"submillisecond wall receipt": {"UNVERIFIED", "PREPROCESS_CST_PRIMARY_UNVERIFIED", func(r *PreprocessCSTRequest) {
			r.Operation.RunWall = time.Nanosecond
			r.Primary.Policy = NativePolicy(r.Operation, OutputTree, nil)
			r.Raw = nil
			r.Primary.ExecutionStatus, r.Primary.Code = StatusResourceLimit, "WALL_LIMIT"
			r.Primary.Process.Status, r.Primary.Process.Reason = StatusResourceLimit, "WALL_LIMIT"
			r.Primary.Process.StdoutBytes, r.Primary.Process.WallBudgetMillis = 0, 1
		}},
		"cleanup precedes terminal cause": {"UNVERIFIED", "PREPROCESS_CST_PRIMARY_UNVERIFIED", func(r *PreprocessCSTRequest) {
			r.Raw = nil
			r.Primary.ExecutionStatus, r.Primary.Code = StatusFailed, "PROCESS_CLEANUP_UNVERIFIED"
			r.Primary.Process.Status, r.Primary.Process.Reason = StatusResourceLimit, "MEMORY_LIMIT"
			r.Primary.Process.CleanupVerified, r.Primary.Process.MemoryLimitHit = false, true
			r.Primary.Process.StdoutBytes = 0
		}},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRequest()
			tc.change(&r)
			got := CheckPreprocessCST(r)
			if got.Result != tc.want || got.Code != tc.code {
				t.Fatalf("%s/%s want %s/%s", got.Result, got.Code, tc.want, tc.code)
			}
			if tc.want == "MISMATCH" && (len(got.Steps) != 1 || len(got.Steps[0].HostOnly) == 0) {
				t.Fatal("first mismatch prefix lost")
			}
			b, _ := json.Marshal(got)
			if _, err := decodePreprocessCST(b); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPreprocessCSTStrictReceipt(t *testing.T) {
	base := PreprocessCSTResult{Schema: PreprocessCSTSchema, Result: "NOT_APPLICABLE", Steps: []PreprocessCSTStep{}}
	raw, _ := json.Marshal(base)
	if _, err := decodePreprocessCST(raw); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"steps":[]`, `"steps":null`, 1),
		strings.Replace(string(raw), `"code":"",`, "", 1),
		strings.Replace(string(raw), `"code":""`, `"code":null`, 1),
		strings.Replace(string(raw), `"schema":`, `"Schema":`, 1),
		strings.Replace(string(raw), `"schema":`, `"schema":"x","schema":`, 1),
		strings.Replace(string(raw), `"primary":null`, `"primary":{"primary":null}`, 1),
	} {
		if _, err := decodePreprocessCST([]byte(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
