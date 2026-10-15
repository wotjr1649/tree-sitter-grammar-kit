package foundation

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// prepare-routes.ps1 creates a C2 file ("create": true, #98) only from one operation with an
// empty before, 0 occurrences and the whole content, accepts an empty before nowhere else,
// and refuses to create an npm input, a file present at the pinned commit or a file an
// adoption step patches. This runs the script's own functions on synthetic subjects.

// psPrepareCreate is the pwsh harness: it defines prepare-routes.ps1's patch functions from
// the script's AST (the script itself downloads and builds routes) and runs each case.
const psPrepareCreate = `param([string]$Script, [string]$Root, [string]$Cases)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repo = $Root
$ast = [Management.Automation.Language.Parser]::ParseFile($Script, [ref]$null, [ref]$null)
$defs = $ast.FindAll({ param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] }, $true)
foreach ($f in $defs) {
  if ($f.Name -in @('Get-JsonMember', 'Get-StepNode', 'Test-Created', 'Get-PatchedText', 'Assert-Creatable')) { . ([scriptblock]::Create($f.Extent.Text)) }
}
$out = foreach ($c in (Get-Content -LiteralPath $Cases -Raw | ConvertFrom-Json)) {
  try {
    if ($c.kind -eq 'creatable') {
      Assert-Creatable 'r' $c.path $c.exists @($c.own)
      [ordered]@{ name = $c.name; ok = $true; text = '' }
    } else {
      $chain = @([ordered]@{ subject = $c.subject; pointer = '/files/0'; field = 'operations'; target = 't' })
      $created = Test-Created $chain 't'
      [ordered]@{ name = $c.name; ok = $true; text = (Get-PatchedText $c.text $chain 't'); created = $created }
    }
  } catch {
    [ordered]@{ name = $c.name; ok = $false; text = $_.Exception.Message }
  }
}
ConvertTo-Json -InputObject @($out) -Depth 6 -Compress
`

func TestPrepareRoutesCreate(t *testing.T) {
	root := repository(t)
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available: prepare-routes.ps1 create guards not executed")
	}
	dir := t.TempDir()
	type subjectFile = map[string]any
	op := func(before, after string, occ any) map[string]any {
		m := map[string]any{"after": after}
		if before != "\x00" {
			m["before"] = before
		}
		if occ != nil {
			m["occurrences"] = occ
		}
		return m
	}
	subjects := map[string]subjectFile{
		"ok":         {"target": "t", "create": true, "operations": []any{op("", "content", 0)}},
		"two-ops":    {"target": "t", "create": true, "operations": []any{op("", "content", 0), op("", "more", 0)}},
		"no-ops":     {"target": "t", "create": true, "operations": []any{}},
		"no-before":  {"target": "t", "create": true, "operations": []any{op("\x00", "content", 0)}},
		"no-occ":     {"target": "t", "create": true, "operations": []any{op("", "content", nil)}},
		"occ-1":      {"target": "t", "create": true, "operations": []any{op("", "content", 1)}},
		"occ-string": {"target": "t", "create": true, "operations": []any{op("", "content", "0")}},
		"before-x":   {"target": "t", "create": true, "operations": []any{op("x", "content", 0)}},
		"no-content": {"target": "t", "create": true, "operations": []any{op("", "", 0)}},
		"not-create": {"target": "t", "operations": []any{op("", "content", 0)}},
		"string-yes": {"target": "t", "create": "true", "operations": []any{op("", "content", 0)}},
		"edit":       {"target": "t", "operations": []any{op("a", "b", 1)}},
	}
	for name, doc := range subjects {
		b, _ := json.Marshal(map[string]any{"files": []any{doc}})
		if err := os.WriteFile(filepath.Join(dir, name+".json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const format = "a created file needs exactly one operation with an empty before, 0 occurrences and the whole content"
	const emptyBefore = "an empty before only creates a declared file"
	type want struct {
		ok   bool
		text string
	}
	cases := []struct {
		name, kind, subject, text, path string
		exists                          bool
		own                             []map[string]string
		want                            want
	}{
		{name: "created", subject: "ok.json", want: want{true, "content"}},
		{name: "edit of an existing file", subject: "edit.json", text: "a", want: want{true, "b"}},
		{name: "two operations", subject: "two-ops.json", want: want{false, format}},
		{name: "no operation", subject: "no-ops.json", want: want{false, format}},
		{name: "missing before", subject: "no-before.json", want: want{false, format}},
		{name: "missing occurrences", subject: "no-occ.json", want: want{false, format}},
		{name: "occurrences 1", subject: "occ-1.json", want: want{false, format}},
		{name: "occurrences as a string", subject: "occ-string.json", want: want{false, format}},
		{name: "non-empty before", subject: "before-x.json", want: want{false, format}},
		{name: "no content", subject: "no-content.json", want: want{false, format}},
		{name: "empty before without create", subject: "not-create.json", want: want{false, emptyBefore}},
		{name: "create as a string", subject: "string-yes.json", want: want{false, emptyBefore}},
		{name: "creatable", kind: "creatable", path: "src/scanner.c", want: want{true, ""}},
		{name: "npm input", kind: "creatable", path: "node_modules/tree-sitter-c/grammar.js", want: want{false, "an npm input file"}},
		{name: "present upstream", kind: "creatable", path: "src/scanner.c", exists: true, want: want{false, "which exists at the pinned commit"}},
		{name: "adoption target", kind: "creatable", path: "src/scanner.c", own: []map[string]string{{"target": "src/scanner.c"}}, want: want{false, "which an adoption step patches"}},
	}
	type input struct {
		Name    string              `json:"name"`
		Kind    string              `json:"kind"`
		Subject string              `json:"subject"`
		Text    string              `json:"text"`
		Path    string              `json:"path"`
		Exists  bool                `json:"exists"`
		Own     []map[string]string `json:"own"`
	}
	var in []input
	for _, c := range cases {
		own := c.own
		if own == nil {
			own = []map[string]string{}
		}
		in = append(in, input{c.name, c.kind, c.subject, c.text, c.path, c.exists, own})
	}
	b, _ := json.Marshal(in)
	caseFile := filepath.Join(dir, "cases.json")
	harness := filepath.Join(dir, "create.ps1")
	if err := os.WriteFile(caseFile, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(harness, []byte(psPrepareCreate), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "src", "dev", "s05-native", "prepare-routes.ps1")
	out, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harness, "-Script", script, "-Root", dir, "-Cases", caseFile).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	var got []struct {
		Name string `json:"name"`
		OK   bool   `json:"ok"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(out, &got); err != nil || len(got) != len(cases) {
		t.Fatalf("harness output: %v\n%s", err, out)
	}
	for i, c := range cases {
		g := got[i]
		if g.Name != c.name || g.OK != c.want.ok || (c.want.ok && g.Text != c.want.text) || (!c.want.ok && !strings.Contains(g.Text, c.want.text)) {
			t.Errorf("%s: ok=%v %q, want ok=%v %q", c.name, g.OK, g.Text, c.want.ok, c.want.text)
		}
	}
}

// psRecordedRun defines prepare-routes.ps1's Test-RecordedRun from the script's AST and
// reports, per synthetic reproduce result, whether -RecordOutputs would accept it.
const psRecordedRun = `param([string]$Script, [string]$Cases)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$ast = [Management.Automation.Language.Parser]::ParseFile($Script, [ref]$null, [ref]$null)
$f = $ast.FindAll({ param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq 'Test-RecordedRun' }, $true)
. ([scriptblock]::Create(@($f)[0].Extent.Text))
$out = foreach ($c in (Get-Content -LiteralPath $Cases -Raw | ConvertFrom-Json)) { [ordered]@{ name = $c.name; ok = [bool](Test-RecordedRun $c.rep) } }
ConvertTo-Json -InputObject @($out) -Compress
`

// -RecordOutputs keeps only completed, deterministic runs whose sole gap is the absent
// reference; a resource or cleanup failure with passing observation claims is refused.
func TestPrepareRoutesRecordedRun(t *testing.T) {
	root := repository(t)
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available: prepare-routes.ps1 record guard not executed")
	}
	rep := func(mut func(map[string]any)) map[string]any {
		r := map[string]any{"execution_status": "COMPLETED", "assessment": "BLOCKED", "findings": []any{},
			"claims":      map[string]any{"generator_ran": "PASS", "deterministic": "PASS", "reference_match": "NOT_CLAIMED"},
			"comparisons": []any{map[string]any{"path": "parser.c", "across_runs": "EQUAL", "reference": "REFERENCE_ABSENT"}}}
		if mut != nil {
			mut(r)
		}
		return r
	}
	want := map[string]bool{"absent-reference": true}
	cases := []any{map[string]any{"name": "absent-reference", "rep": rep(nil)}}
	for name, mut := range map[string]func(map[string]any){
		"storage-limit-b":   func(r map[string]any) { r["execution_status"] = "RESOURCE_LIMIT" },
		"cleanup-failed":    func(r map[string]any) { r["execution_status"] = "FAILED" },
		"not-deterministic": func(r map[string]any) { r["claims"].(map[string]any)["deterministic"] = "FAIL" },
		"generator-failed":  func(r map[string]any) { r["claims"].(map[string]any)["generator_ran"] = "FAIL" },
		"reference-present": func(r map[string]any) {
			r["assessment"], r["claims"].(map[string]any)["reference_match"] = "PASS", "PASS"
		},
		"outputs-differ": func(r map[string]any) {
			r["comparisons"] = []any{map[string]any{"path": "parser.c", "across_runs": "DIFFERENT", "reference": "REFERENCE_ABSENT"}}
		},
		"no-comparisons":      func(r map[string]any) { r["comparisons"] = []any{} },
		"unregistered-output": func(r map[string]any) { r["findings"] = []any{map[string]any{"code": "UNREGISTERED_OUTPUT"}} },
	} {
		want[name] = false
		cases = append(cases, map[string]any{"name": name, "rep": rep(mut)})
	}
	dir := t.TempDir()
	b, _ := json.Marshal(cases)
	casesPath, harness := filepath.Join(dir, "cases.json"), filepath.Join(dir, "harness.ps1")
	if err := os.WriteFile(casesPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(harness, []byte(psRecordedRun), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harness, "-Script", filepath.Join(root, "src/dev/s05-native/prepare-routes.ps1"), "-Cases", casesPath).CombinedOutput()
	if err != nil {
		t.Fatalf("harness: %v: %s", err, out)
	}
	var got []struct {
		Name string
		OK   bool
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &got); err != nil || len(got) != len(want) {
		t.Fatalf("harness output %v: %s", err, out)
	}
	for _, g := range got {
		if g.OK != want[g.Name] {
			t.Errorf("%s: accepted=%v want %v", g.Name, g.OK, want[g.Name])
		}
	}
}
