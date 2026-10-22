package foundation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

type reuseHoldout struct {
	ID     string `json:"id"`
	Route  string `json:"route"`
	Origin struct {
		Issue           int    `json:"issue"`
		File            string `json:"file"`
		Revision        string `json:"revision"`
		Artifact        string `json:"artifact"`
		ArtifactSHA256  string `json:"artifact_sha256"`
		Pointer         string `json:"pointer"`
		CaseID          string `json:"case_id"`
		InputFile       string `json:"input_file"`
		InputFileSHA256 string `json:"input_file_sha256"`
		OriginalSHA256  string `json:"original_sha256"`
		RepairedSHA256  string `json:"repaired_sha256"`
	} `json:"origin"`
	Sibling    string `json:"sibling"`
	SourceUTF8 string `json:"source_utf8"`
	Edits      []struct {
		StartByte  uint32 `json:"start_byte"`
		OldEndByte uint32 `json:"old_end_byte"`
		NewEndByte uint32 `json:"new_end_byte"`
		Old        string `json:"old_utf8"`
		New        string `json:"new_utf8"`
	} `json:"edits"`
	BlockedSteps []int `json:"blocked_steps"`
	OmitOldFirst int   `json:"omit_old_tree_first_step"`
}

// TestReuseHoldoutRegistry pins the reuse holdout ledger (#217). The set of originals and
// each one's history (route, source and exact byte edits) are fixed here, so dropping,
// replacing or re-editing an original fails even when the file stays self-consistent. A
// holdout never shares an ID with a registered case, its recorded steps lie in its history,
// a #198 original matches its preregistered hashes, and every version of a #170 original's
// protected sibling is the 29-byte prefix followed by the original's version.
func TestReuseHoldoutRegistry(t *testing.T) {
	pinned := []struct{ id, history string }{
		{"tsql-issue137-builtin-default-r1", "d28caf5802caf01a8163233f3012a5e2df753e3bedd552e7b794a5bb1940db08"},
		{"tsql-issue137-dynamic-payload-r1", "ebe3200d3f2be3834bb1069f9df7a1d43741567fc8ae058d460a28f18c2d82e1"},
		{"tsql-issue139-procedure-r1", "9afac1e6907742fe02a3738274f3c8d2282a3a05d1414c4371b335e4b2dfe836"},
		{"tsql-issue139-function-r1", "c8f3c83d79e96dd037df26361ba2466367471efc539cd49d3346da8a96cf6459"},
		{"tsql-issue139-output-r1", "0cf51c44802ce762c60b2c89afbeec5e7fa024ed29aaa28130c4a0f1f9df1fea"},
		{"tsql-issue139-clr-r1", "dfad423e945d9ed31d40cc0ca297c79a000b5724885e7e8c31238abbd858965b"},
		{"tsql-issue138-leading-01473-r1", "c9403507404c2ffecf997d402eb2fce972b020ee1ef994e006c84589156bfe00"},
		{"tsql-issue138-leading-02133-r1", "729cebcfe10a3ad2168f86bdbebf54b571c24a32db198ac6e461c8011e8b934c"},
		{"tsql-issue138-leading-02657-r1", "be5598edcd9081fdbc3f9b7648e26fcf7587c0b1c9e954ebfaa65b0f2d717649"},
		{"tsql-issue148-negative-pair-00-r1", "7cbabba62d4df695b9426f1a62ff56e6137453aa081258a84969c3399b274ad6"},
		{"tsql-issue148-negative-pair-01-r1", "dde297f65cca5dfd9cc55544c2eef2a3ccdac4ae1f0efaa6b3083cc24a9fed2c"},
		{"tsql-issue198-unclosed-bracket", "760e7cf137f08ed7fc96393338c78f2dbfec204b656504be9346196e8ee14ffe"},
		{"json-repair-close", "ef207e069b058e6c66bd8456bfbdf21b7306c54f2ed707d159c4dab91db53414"},
		{"json-break-close", "0484e09150af8aa30c80a83a64b2a9eff001d83a1739f9384c6c84714949e2aa"},
		{"json-repair-colon", "f8fd991b5b06019acb9443b48087302710cc15ba0d6984a5eb7b297ec231e4aa"},
	}
	root := repository(t)
	data, err := os.ReadFile(filepath.Join(root, "src", "testdata", "native", "reuse-holdouts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Schema   string         `json:"schema"`
		Holdouts []reuseHoldout `json:"holdouts"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil || f.Schema != "tsgk-reuse-holdouts/r1" || len(f.Holdouts) != len(pinned) {
		t.Fatalf("reuse holdouts: %v schema=%q holdouts=%d", err, f.Schema, len(f.Holdouts))
	}
	readCases := func(p string) []srcCase {
		data, err := os.ReadFile(p)
		var cf caseFile
		if err == nil {
			err = json.Unmarshal(data, &cf)
		}
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		return loadCases(cf)
	}
	registered := map[string]bool{}
	for _, kind := range []string{"routes", "gaps", "n461"} {
		paths, _ := filepath.Glob(filepath.Join(root, "src", "testdata", "native", kind, "*.json"))
		for _, p := range paths {
			for _, c := range readCases(p) {
				registered[c.id] = true
			}
		}
	}
	hex64, hex40 := regexp.MustCompile(`^[0-9a-f]{64}$`), regexp.MustCompile(`^[0-9a-f]{40}$`)
	prefix := []byte("SELECT NULL;\nSELECT NULL;\nGO\n")
	for i, h := range f.Holdouts {
		edits := []kit.Edit{}
		for _, e := range h.Edits {
			edits = append(edits, kit.Edit{StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte, Old: []byte(e.Old), New: []byte(e.New)})
		}
		identity, _ := json.Marshal(struct {
			Route, Source string
			Edits         []kit.Edit
		}{h.Route, h.SourceUTF8, edits})
		sum := sha256.Sum256(identity)
		if h.ID != pinned[i].id || hex.EncodeToString(sum[:]) != pinned[i].history {
			t.Errorf("holdout %d: %s history %x, pinned %s %s", i, h.ID, sum, pinned[i].id, pinned[i].history)
			continue
		}
		if registered[h.ID] || (h.Route != "tsql" && h.Route != "json") {
			t.Errorf("%s: registered case ID or route %q", h.ID, h.Route)
		}
		versions, _, err := kit.ApplyEdits(kit.EncodingUTF8, []byte(h.SourceUTF8), edits, 1<<20)
		if err != nil || len(edits) == 0 {
			t.Errorf("%s: history %v", h.ID, err)
			continue
		}
		steps := len(h.BlockedSteps) > 0 && h.BlockedSteps[0] >= 1 && h.BlockedSteps[len(h.BlockedSteps)-1] <= len(edits)
		for k := 1; k < len(h.BlockedSteps); k++ {
			steps = steps && h.BlockedSteps[k-1] < h.BlockedSteps[k]
		}
		if !steps || h.OmitOldFirst < 1 || h.OmitOldFirst > h.BlockedSteps[0] {
			t.Errorf("%s: blocked steps %v, omit-old first step %d", h.ID, h.BlockedSteps, h.OmitOldFirst)
		}
		o := h.Origin
		switch o.Issue {
		case 170, 197:
			if o.File == "" || !hex40.MatchString(o.Revision) || o.Artifact != "" || (o.Issue == 170) != (h.Sibling != "") {
				t.Errorf("%s: origin %+v sibling %q", h.ID, o, h.Sibling)
			}
		case 198:
			first, last := sha256.Sum256(versions[0]), sha256.Sum256(versions[1])
			if o.Artifact == "" || o.Pointer == "" || o.CaseID == "" || o.InputFile == "" || !hex64.MatchString(o.ArtifactSHA256) || !hex64.MatchString(o.InputFileSHA256) ||
				o.OriginalSHA256 != hex.EncodeToString(first[:]) || o.RepairedSHA256 != hex.EncodeToString(last[:]) || h.Sibling != "" {
				t.Errorf("%s: origin %+v", h.ID, o)
			}
		default:
			t.Errorf("%s: origin issue %d", h.ID, o.Issue)
		}
		if h.Sibling == "" {
			continue
		}
		gaps := readCases(filepath.Join(root, "src", "testdata", "native", "gaps", h.Route+".json"))
		sib := slices.IndexFunc(gaps, func(c srcCase) bool { return c.id == h.Sibling })
		if sib < 0 {
			t.Errorf("%s: sibling %s not registered", h.ID, h.Sibling)
			continue
		}
		sc := gaps[sib]
		qc, err := convertCase(sc.id, sc.src, sc.edits, sc.expect, sc.anchors)
		if err != nil {
			t.Fatal(err)
		}
		sv, _, err := kit.ApplyEdits(kit.EncodingUTF8, []byte(sc.src), qc.Edits, 1<<20)
		if err != nil || len(sv) != len(versions) {
			t.Errorf("%s: sibling %s versions %d/%d %v", h.ID, h.Sibling, len(sv), len(versions), err)
			continue
		}
		for k := range sv {
			if !bytes.Equal(sv[k], append(slices.Clone(prefix), versions[k]...)) {
				t.Errorf("%s: sibling %s version %d is not the prefix followed by the original", h.ID, h.Sibling, k)
			}
		}
	}
}

// TestReuseHoldoutWorkflow keeps the native routes job checking the ledger (#217): every
// registered case's error-tree route BLOCKED fails it as undisposed, and the holdouts run
// with the old tree passed and omitted.
func TestReuseHoldoutWorkflow(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github", "workflows", "foundation.yml")))
	for _, want := range []string{
		"$undisposed = @((Get-Content -Raw -LiteralPath (Join-Path $out 'summary.json') | ConvertFrom-Json).requirement_results | Where-Object { $_.claim -eq 'incremental_route' })",
		"if ($undisposed.Count -ne 0) { throw",
		"go test ./src/internal/native -run '^TestNativeReuseHoldouts$' -count=1 -v",
	} {
		if strings.Count(text, want) != 1 {
			t.Errorf("foundation.yml: %q not present exactly once", want)
		}
	}
}
