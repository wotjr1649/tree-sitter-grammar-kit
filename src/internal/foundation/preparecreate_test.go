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
