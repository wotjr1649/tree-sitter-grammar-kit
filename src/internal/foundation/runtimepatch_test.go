package foundation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const psRuntimePatch = `param([string]$Script, [string]$Root, [string]$Cases)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repo = $Root
$ast = [Management.Automation.Language.Parser]::ParseFile($Script, [ref]$null, [ref]$null)
foreach ($f in $ast.FindAll({ param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] }, $true)) {
  if ($f.Name -in @('Get-Sha', 'Assert-File', 'Get-JsonMember', 'Get-StepNode', 'Get-PatchedText', 'Set-RuntimePatches')) { . ([scriptblock]::Create($f.Extent.Text)) }
}
$out = foreach ($c in (Get-Content -LiteralPath $Cases -Raw | ConvertFrom-Json)) {
  $p = Join-Path $Root 'node.c'
  [IO.File]::WriteAllText($p, $c.text, [Text.UTF8Encoding]::new($false))
  try {
    Set-RuntimePatches $p 'lib/src/node.c' ([pscustomobject]@{ patches = @($c.step) })
    Assert-File $p $c.after_sha256 $c.after_bytes 'runtime file'
    [ordered]@{ ok = $true; message = '' }
  } catch { [ordered]@{ ok = $false; message = $_.Exception.Message } }
}
ConvertTo-Json -InputObject @($out) -Compress
`

func TestRuntimePatch(t *testing.T) {
	root := repository(t)
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var manifest struct {
		Commit  string
		Patches []map[string]any `json:"patches"`
		Files   []struct {
			Path, SHA256 string
			Bytes        int
		}
	}
	if err := json.Unmarshal(read("src/drivers/native-c/runtime-manifest.json"), &manifest); err != nil || len(manifest.Patches) != 2 || len(manifest.Files) != 83 || manifest.Commit != "659cda7c7f86ebe31cc825dc5da59e9add172dc7" {
		t.Fatalf("runtime patch registration: %v", err)
	}
	var nodePin bool
	for _, f := range manifest.Files {
		if f.Path == "lib/src/node.c" {
			nodePin = f.SHA256 == "ef9c9e15b6dec11646416207db3421c58054bef7cb5209df10d36c5f872b2f01" && f.Bytes == 23700
		}
	}
	if !nodePin {
		t.Fatal("patched node.c must remain bound by the runtime input closure")
	}
	for i, tc := range []struct {
		issue, subject, beforeSHA string
		beforeBytes               int
	}{
		{"118", "src/drivers/native-c/runtime-field-lookup.patch.json", "fb0b5eecacb6d7e324f60914893801c0d147f413dd0af73a19ef270d341a77b5", 25151},
		{"120", "src/drivers/native-c/runtime-navigation.patch.json", "4ffa3a64675b95316ae92e11cbfc9754f908bb151c5499c73fa6371a93358740", 23579},
	} {
		t.Run(tc.issue, func(t *testing.T) {
			checkRuntimePatch(t, root, manifest.Patches[i], tc.issue, tc.subject, tc.beforeSHA, tc.beforeBytes)
		})
	}
}

func checkRuntimePatch(t *testing.T, root string, step map[string]any, issue, expectedSubject, beforeSHA string, beforeBytes int) {
	t.Helper()
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	removal, _ := step["remove_when"].(string)
	if step["origin"] != "LOCAL" || step["tracking_issue"] != "https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/"+issue || step["license"] != "MIT" || step["target"] != "lib/src/node.c" || step["pointer"] != "/files/0" || step["field"] != "operations" || strings.TrimSpace(removal) == "" {
		t.Fatal("runtime patch provenance/target/removal policy changed without review")
	}
	if step["before_sha256"] != beforeSHA || step["before_bytes"] != float64(beforeBytes) {
		t.Fatal("patch input node.c identity changed")
	}
	subject, ok := step["subject"].(string)
	if !ok || subject != expectedSubject {
		t.Fatal("unexpected patch subject")
	}
	raw := read(subject)
	digest := func(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
	if step["sha256"] != digest(raw) || step["bytes"] != float64(len(raw)) {
		t.Fatal("patch subject identity mismatch")
	}
	var patch struct {
		Schema string
		Files  []struct {
			Target     string
			Operations []struct {
				Before, After string
				Occurrences   int
			}
		}
	}
	if err := json.Unmarshal(raw, &patch); err != nil || patch.Schema != "tsgk-native-runtime-patch/r1" || len(patch.Files) != 1 || patch.Files[0].Target != step["target"] || len(patch.Files[0].Operations) != 1 || patch.Files[0].Operations[0].Occurrences != 1 {
		t.Fatalf("patch literal contract: %v", err)
	}
	op := patch.Files[0].Operations[0]
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh unavailable: runtime preparation guards unrun; metadata checked")
	}
	dir := t.TempDir()
	write := func(name string, b []byte) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(subject, raw)
	step = maps.Clone(step)
	step["before_sha256"], step["before_bytes"] = digest([]byte(op.Before)), len(op.Before)
	type input struct {
		Text        string         `json:"text"`
		Step        map[string]any `json:"step"`
		AfterSHA256 string         `json:"after_sha256"`
		AfterBytes  int            `json:"after_bytes"`
	}
	var cases []input
	var wants []string
	add := func(text, after, diagnostic string, mutate func(map[string]any)) {
		s := maps.Clone(step)
		if mutate != nil {
			mutate(s)
		}
		cases = append(cases, input{text, s, digest([]byte(after)), len(after)})
		wants = append(wants, diagnostic)
	}
	add(op.Before, op.After, "", nil)
	add("modified", op.After, "runtime patch input identity mismatch", nil)
	add(op.Before, op.After, "runtime patch subject identity mismatch", func(s map[string]any) { s["sha256"] = strings.Repeat("0", 64) })
	add(op.Before+op.Before, op.After, "2 occurrences, want 1", func(s map[string]any) {
		s["before_sha256"], s["before_bytes"] = digest([]byte(op.Before+op.Before)), 2*len(op.Before)
	})
	add("absent", op.After, "0 occurrences, want 1", func(s map[string]any) {
		s["before_sha256"], s["before_bytes"] = digest([]byte("absent")), len("absent")
	})
	add(op.Before, "wrong output", "runtime file identity mismatch", nil)
	b, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	write("cases.json", b)
	write("test.ps1", []byte(psRuntimePatch))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, pwsh, "-NoProfile", "-NonInteractive", "-File", filepath.Join(dir, "test.ps1"), "-Script", filepath.Join(root, "src/dev/s05-native/prepare-routes.ps1"), "-Root", dir, "-Cases", filepath.Join(dir, "cases.json")).CombinedOutput()
	if err != nil {
		t.Fatalf("runtime patch harness: %v %s", err, out)
	}
	var got []struct {
		OK      bool
		Message string
	}
	if err := json.Unmarshal(out, &got); err != nil || len(got) != len(wants) {
		t.Fatalf("runtime patch harness output: %v %s", err, out)
	}
	for i, want := range wants {
		if got[i].OK != (want == "") || !strings.Contains(got[i].Message, want) {
			t.Errorf("case %d: %+v, want %q", i, got[i], want)
		}
	}
}
