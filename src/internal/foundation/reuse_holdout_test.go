package foundation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
// each one's history (route, source, exact byte edits) and provenance (origin, sibling) are
// fixed here, so dropping, replacing, re-editing or re-sourcing an original fails even when
// the file stays self-consistent. A
// holdout never shares an ID with a registered case, its recorded steps lie in its history,
// a #198 original matches its preregistered hashes, and every version of a #170 original's
// protected sibling is the 29-byte prefix followed by the original's version.
func TestReuseHoldoutRegistry(t *testing.T) {
	pinned := []struct{ id, history string }{
		{"tsql-issue137-builtin-default-r1", "2ad13ee607cee027a37d5befd32cd3b0631e264e3cb457c283ee7b0dddf887be"},
		{"tsql-issue137-dynamic-payload-r1", "fbf8c284edcaaca71184ceb75136287df918fe65b8c1537b4f57292eacdfea82"},
		{"tsql-issue139-procedure-r1", "cd04860981964a5c095942af88687380aa5da96b6654207b2598a1d9f94c4b87"},
		{"tsql-issue139-function-r1", "c2aafc7ed41835a32261e26c0a952704c1f41dd1c9827621e325da94c81b648c"},
		{"tsql-issue139-output-r1", "5ceeaef45094a32ed425a7bba94a476f4b68aba5699d61b5b2925d88f23cb31e"},
		{"tsql-issue139-clr-r1", "a6c3ceb770428d44dc9c1591b73c262a1eb8f39b24422d3013241ad4367a8643"},
		{"tsql-issue138-leading-01473-r1", "4dd79a54f5e7989f630cf7ed0892451bb100746c0466a2772c105c804a5da286"},
		{"tsql-issue138-leading-02133-r1", "9413f3492cfb52f91cd2389016f9d41ae9731c40cee21e9133f99242e6f27acc"},
		{"tsql-issue138-leading-02657-r1", "10df1052447ed0bc97f27ade1d96edbe4226a7ce0b23755a96f976e4a779f9a7"},
		{"tsql-issue148-negative-pair-00-r1", "09bc82f500d6564fb4832a1a274397f477f7c47c21ceb882f931bc8a1d9f1aed"},
		{"tsql-issue148-negative-pair-01-r1", "99e269d48f44a5cc9688f517d402caf51f2710686a4ae6937ac4fcf3ac862c79"},
		{"tsql-issue198-unclosed-bracket", "16a81ed40e4047c134c45b30918eae4c31a1cf8e3076af7a8a0a28eadd8e5d3b"},
		{"json-repair-close", "fa64cce8c757b87ed579cb4b851f4d6e70d808f5fc066b27cbfb545e4d39caa8"},
		{"json-break-close", "97674f16d0dcc62788bd91b76f24aa4458d4e1c27062c9e3b4201d7758bdb909"},
		{"json-repair-colon", "5d7ca0aaa6d8b6458b9cc78bbb905d863adc8dd3eb3477c0ae3fa7fd30a36e39"},
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
			Route, Source, Sibling string
			Origin                 any
			Edits                  []kit.Edit
		}{h.Route, h.SourceUTF8, h.Sibling, h.Origin, edits})
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

// holdoutWorkflowSteps are the top-level statements of the native routes run step that check
// the ledger (#217), in their required order; the holdout run's exit and log checks follow
// it directly.
var holdoutWorkflowSteps = []string{
	"if ($routesExit -ne 0) { throw 'route run reported failures' }",
	"& src/dev/s05-native/check-dispositions.ps1 -Summary (Join-Path $out 'summary.json')",
	"foreach ($route in @('json', 'tsql')) {",
	"go test ./src/internal/native -run '^TestNativeReuseHoldouts$' -count=1 -v -timeout=300s > (Join-Path $out 'reuse-observation.log') 2>&1",
	"if ($LASTEXITCODE -ne 0) { throw 'reuse holdout disposition or omit-old control failed' }",
	`if (!(Select-String -LiteralPath (Join-Path $out 'reuse-observation.log') -CaseSensitive -Quiet -Pattern '^--- PASS: TestNativeReuseHoldouts \(')) { throw 'reuse holdouts did not run' }`,
}

// psTopStatements prints the first line of each top-level statement of a PowerShell script,
// from the PowerShell parser's own AST: comments are not statements and a statement nested
// in a conditional or loop is not top-level, whatever its indentation.
const psTopStatements = `param([string]$Script)
$ErrorActionPreference = 'Stop'
$errors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile($Script, [ref]$null, [ref]$errors)
if ($errors.Count) { throw "parse errors: $($errors[0].Message)" }
foreach ($s in $ast.EndBlock.Statements) { ($s.Extent.Text -split "\r?\n")[0] }
`

// holdoutWorkflowOrder finds the native routes step that runs run-routes.ps1 and checks that
// its top-level PowerShell statements hold the ledger checks once each, in order, with the
// last three adjacent.
func holdoutWorkflowOrder(pwsh, text string) error {
	body := workflowJobBody(text, "native-routes")
	at := strings.Index(body, "run-routes.ps1 -Prepared")
	start := strings.LastIndex(body[:max(at, 0)], "run: |\n")
	if at < 0 || start < 0 {
		return fmt.Errorf("native-routes run step not found")
	}
	var script []string
	indent := ""
	for _, line := range strings.Split(body[start+len("run: |\n"):], "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" {
			script = append(script, "")
			continue
		}
		if indent == "" {
			indent = line[:len(line)-len(trimmed)]
		}
		if !strings.HasPrefix(line, indent) {
			break // the block scalar ends at the first shallower non-blank line
		}
		script = append(script, strings.TrimPrefix(line, indent))
	}
	dir, err := os.MkdirTemp("", "tsgk-holdout-workflow-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	runPath, parsePath := filepath.Join(dir, "run.ps1"), filepath.Join(dir, "parse.ps1")
	if err := os.WriteFile(runPath, []byte(strings.Join(script, "\n")), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(parsePath, []byte(psTopStatements), 0o644); err != nil {
		return err
	}
	out, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", parsePath, "-Script", runPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("parse run step: %v: %s", err, out)
	}
	statements := strings.Split(strings.ReplaceAll(strings.TrimSpace(string(out)), "\r\n", "\n"), "\n")
	prev := -1
	for k, want := range holdoutWorkflowSteps {
		i := slices.Index(statements, want)
		if i < 0 || slices.Index(statements[i+1:], want) >= 0 {
			return fmt.Errorf("top-level %q not present exactly once", want)
		}
		if i <= prev || k >= 4 && i != prev+1 {
			return fmt.Errorf("%q out of order", want)
		}
		prev = i
	}
	return nil
}

// TestReuseHoldoutWorkflow keeps the native routes job checking the ledger (#217): the
// disposition gate reads the route run's summary before the holdouts run, and the holdouts
// must run in full. Controls comment out, guard or reorder a check; a leading whitespace-only
// line in the block keeps the workflow valid.
func TestReuseHoldoutWorkflow(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available: the run step's PowerShell AST is not parsed")
	}
	text := string(mustRead(t, filepath.Join(repository(t), ".github", "workflows", "foundation.yml")))
	if err := holdoutWorkflowOrder(pwsh, text); err != nil {
		t.Fatalf("foundation.yml: %v", err)
	}
	const pad = "          "
	gate, logCheck := pad+holdoutWorkflowSteps[1]+"\n", pad+holdoutWorkflowSteps[5]+"\n"
	body := workflowJobBody(text, "native-routes")
	runAt := strings.LastIndex(body[:strings.Index(body, "run-routes.ps1 -Prepared")], "run: |\n") + len("run: |\n")
	blank := strings.Replace(text, body, body[:runAt]+"  \n"+body[runAt:], 1)
	if blank == text || holdoutWorkflowOrder(pwsh, blank) != nil {
		t.Fatal("a leading whitespace-only line rejected the workflow")
	}
	// nextStep moves a line into the step after the run-routes step
	nextStep := func(s, line string) string {
		at := strings.Index(s, "run-routes.ps1 -Prepared")
		end := at + strings.Index(s[at:], "\n      - name: ") + 1
		return s[:end] + "      - name: moved\n        shell: pwsh\n        run: |\n" + line + s[end:]
	}
	for name, broken := range map[string]string{
		"block-comment":        strings.Replace(text, gate, pad+"<#\n"+gate+pad+"#>\n", 1),
		"line-comment":         strings.Replace(text, gate, pad+"# "+holdoutWorkflowSteps[1]+"\n", 1),
		"guarded":              strings.Replace(text, gate, pad+"if ($false) {\n"+gate+pad+"}\n", 1),
		"comment-then-guarded": strings.Replace(text, gate, pad+"<# skip #> if ($false) {\n"+gate+pad+"}\n", 1),
		"gate-late":            strings.Replace(strings.Replace(text, gate, "", 1), logCheck, logCheck+gate, 1),
		"log-early":            strings.Replace(strings.Replace(text, logCheck, "", 1), gate, gate+logCheck, 1),
		"gate-in-next-step":    nextStep(strings.Replace(text, gate, "", 1), gate),
	} {
		if broken == text {
			t.Fatalf("%s control did not mutate the workflow", name)
		}
		if err := holdoutWorkflowOrder(pwsh, broken); err == nil {
			t.Errorf("%s control accepted", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
}

// TestReuseHoldoutLogCheck runs the workflow's log check on go test logs: only the top-level
// PASS line of TestNativeReuseHoldouts counts as a run.
func TestReuseHoldoutLogCheck(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available: the log check is not executed (TestReuseHoldoutWorkflow still applies)")
	}
	for _, tc := range []struct {
		name, log string
		ran       bool
	}{
		{"parent-pass", "=== RUN   TestNativeReuseHoldouts\n--- PASS: TestNativeReuseHoldouts (64.07s)\nPASS\n", true},
		{"skip-only", "=== RUN   TestNativeReuseHoldouts\n--- SKIP: TestNativeReuseHoldouts (0.00s)\nPASS\n", false},
		{"child-pass-parent-skip", "    --- PASS: TestNativeReuseHoldouts/child (0.00s)\n--- SKIP: TestNativeReuseHoldouts (0.00s)\n", false},
		{"similar-name", "--- PASS: TestNativeReuseHoldoutsMeasure (0.00s)\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "o'brien") // a quote in the path stays a literal
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "reuse-observation.log"), []byte(tc.log), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference = 'Stop'; $out = '"+strings.ReplaceAll(dir, "'", "''")+"'; "+holdoutWorkflowSteps[5]).CombinedOutput()
			if (err == nil) != tc.ran {
				t.Fatalf("ran=%t, check err=%v:\n%s", tc.ran, err, out)
			}
		})
	}
}

// TestCheckDispositions runs the disposition gate on route run summaries: a registered
// case's or SVC segment's incremental route BLOCKED fails, so does a summary that cannot
// show one; a query expectation result alone passes.
func TestCheckDispositions(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available: check-dispositions.ps1 not executed (TestReuseHoldoutWorkflow still applies)")
	}
	script := filepath.Join(repository(t), "src", "dev", "s05-native", "check-dispositions.ps1")
	route := func(label string) string {
		return `{"case": "` + label + `", "claim": "incremental_route", "result": "BLOCKED", "code": "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1", "failures": []}`
	}
	query := `{"case": "oracle r/q", "claim": "query_expectations", "result": "BLOCKED", "code": "ORACLE_CLAIM_FAILED", "failures": []}`
	for _, tc := range []struct {
		name, summary, want string
	}{
		{"empty", `{"schema": "tsgk-s05-route-run/r1", "requirement_results": []}`, "undisposed_route_blocked=0"},
		{"query-only", `{"schema": "tsgk-s05-route-run/r1", "requirement_results": [` + query + `]}`, "requirement_results=1 undisposed_route_blocked=0"},
		{"s05-route", `{"schema": "tsgk-s05-route-run/r1", "requirement_results": [` + query + `, ` + route("tsql/a") + `]}`, "!undisposed route BLOCKED (#217): tsql/a"},
		{"s06-route", `{"schema": "tsgk-s05-route-run/r1", "requirement_results": [` + route("oracle tsql/a") + `]}`, "!undisposed route BLOCKED (#217): oracle tsql/a"},
		{"segment-route", `{"schema": "tsgk-s05-route-run/r1", "requirement_results": [` + route("csharp-svc/s segment [0,2)") + `]}`, "!undisposed route BLOCKED (#217): csharp-svc/s segment [0,2)"},
		{"missing", `{"schema": "tsgk-s05-route-run/r1"}`, "!requirement_results missing or not an array"},
		{"null", `{"schema": "tsgk-s05-route-run/r1", "requirement_results": null}`, "!requirement_results missing or not an array"},
		{"object", `{"schema": "tsgk-s05-route-run/r1", "requirement_results": ` + route("tsql/a") + `}`, "!requirement_results missing or not an array"},
		{"no-claim", `{"schema": "tsgk-s05-route-run/r1", "requirement_results": [{"case": "tsql/a"}]}`, "!requirement result without a known claim"},
		{"empty-claim", `{"schema": "tsgk-s05-route-run/r1", "requirement_results": [{"case": "tsql/a", "claim": "", "result": "BLOCKED"}]}`, "!requirement result without a known claim"},
		{"padded-claim", `{"schema": "tsgk-s05-route-run/r1", "requirement_results": [{"case": "tsql/a", "claim": "incremental_route ", "result": "BLOCKED"}]}`, "!requirement result without a known claim"},
		{"unknown-claim", `{"schema": "tsgk-s05-route-run/r1", "requirement_results": [{"case": "tsql/a", "claim": "Incremental_Route", "result": "BLOCKED"}]}`, "!requirement result without a known claim"},
		{"schema", `{"schema": "tsgk-s05-route-run/r0", "requirement_results": []}`, "!not a tsgk-s05-route-run/r1 summary"},
		{"top-level-array", `[{"schema": "tsgk-s05-route-run/r1", "requirement_results": []}]`, "!not a tsgk-s05-route-run/r1 summary"},
		{"schema-empty-array", `{"schema": [], "requirement_results": []}`, "!not a tsgk-s05-route-run/r1 summary"},
		{"schema-array", `{"schema": ["tsgk-s05-route-run/r1"], "requirement_results": []}`, "!not a tsgk-s05-route-run/r1 summary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "summary.json")
			if err := os.WriteFile(path, []byte(tc.summary), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", script, "-Summary", path).CombinedOutput()
			fail, want := strings.HasPrefix(tc.want, "!"), strings.TrimPrefix(tc.want, "!")
			if (err != nil) != fail || !strings.Contains(string(out), want) {
				t.Fatalf("err=%v, want failure=%t with %q:\n%s", err, fail, want, out)
			}
		})
	}
}
