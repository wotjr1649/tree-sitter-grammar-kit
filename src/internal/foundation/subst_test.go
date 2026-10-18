package foundation

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise the workflow's event validator, without creating a device mapping.
func TestHostedSubstEvidenceGuard(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh unavailable: hosted subst event validator not executed")
	}
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	_, tail, ok := strings.Cut(text, "          $events = @($lines | ForEach-Object { $_ | ConvertFrom-Json -AsHashtable })\n")
	if !ok {
		t.Fatal("hosted subst event validator missing")
	}
	body := "$events = @($lines | ForEach-Object { $_ | ConvertFrom-Json -AsHashtable })\n"
	for _, line := range strings.Split(tail, "\n") {
		if !strings.HasPrefix(line, "          ") {
			break
		}
		body += strings.TrimPrefix(line, "          ") + "\n"
	}
	witness := "SUBST_INSIDE_REJECTED SUBST_OUTSIDE_PUBLISHED SUBST_REMOVED SUBST_REMOVED SUBST_DIFFERENT_TARGET_PRESERVED SUBST_DIFFERENT_TARGET_PRESERVED"
	encode := func(action, output string) string {
		b, _ := json.Marshal(map[string]string{"Action": action, "Test": "TestHostedSubstMembership", "Output": output})
		return string(b)
	}
	cases := []map[string]any{{"name": "complete", "lines": []string{encode("output", witness), encode("pass", "")}, "accept": true}}
	for _, name := range []string{"SUBST_INSIDE_REJECTED", "SUBST_OUTSIDE_PUBLISHED", "SUBST_REMOVED", "SUBST_DIFFERENT_TARGET_PRESERVED"} {
		cases = append(cases, map[string]any{"name": name, "lines": []string{encode("output", strings.Replace(witness, name, "", 1)), encode("pass", "")}, "accept": false})
	}
	cases = append(cases,
		map[string]any{"name": "skip", "lines": []string{encode("skip", "")}, "accept": false},
		map[string]any{"name": "zero-tests", "lines": []string{}, "accept": false},
		map[string]any{"name": "fail-with-witness", "lines": []string{encode("output", witness), encode("pass", ""), encode("fail", "")}, "accept": false})
	dir := t.TempDir()
	data, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string][]byte{"validate.ps1": []byte(body), "cases.json": data, "check.ps1": []byte(`param($Validator, $Cases)
$ErrorActionPreference = 'Stop'
$validate = [scriptblock]::Create([IO.File]::ReadAllText($Validator))
foreach ($case in ([IO.File]::ReadAllText($Cases) | ConvertFrom-Json -AsHashtable)) {
  $lines = @($case.lines)
  $accepted = $true
  try { & $validate } catch { $accepted = $false }
  if ($accepted -ne $case.accept) { throw ('incorrect event disposition: ' + $case.name) }
}
Write-Output 'EVENT_GUARD_CONTROLS_PASS'
`)} {
		if err := os.WriteFile(filepath.Join(dir, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, pwsh, "-NoProfile", "-NonInteractive", "-File", filepath.Join(dir, "check.ps1"), "-Validator", filepath.Join(dir, "validate.ps1"), "-Cases", filepath.Join(dir, "cases.json")).CombinedOutput()
	if err != nil || !strings.Contains(string(output), "EVENT_GUARD_CONTROLS_PASS") {
		t.Fatalf("hosted subst event controls: %v %s", err, output)
	}
}
