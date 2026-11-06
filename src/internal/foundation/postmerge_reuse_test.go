package foundation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// reuseBlock is the whole #335 decision of the mode job, pinned as one piece: a main push reuses
// the heavy evidence of a pull_request run only when that run's latest attempt holds the eight
// required jobs and its one CI mode job exactly once with conclusion success, and the synthetic
// merge named by that job has the tree of this merge commit and the PR head as second parent.
// Every failed lookup, exception, malformed answer and other event leaves the default 'false'.
const reuseBlock = "" +
	"          $prVerified = 'false'\n" +
	"          if ($env:EVENT -eq 'push' -and $env:REF -eq 'refs/heads/main') {\n" +
	"            try {\n" +
	"              $merge = @(gh api \"repos/$env:REPOSITORY/git/commits/$env:SHA\" --jq '.tree.sha, (.parents | length), .parents[1].sha')\n" +
	"              if ($LASTEXITCODE -eq 0 -and $merge.Count -eq 3 -and $merge[1] -eq '2') {\n" +
	"                $tree, $prHead = $merge[0], $merge[2]\n" +
	"                $rows = @(gh api \"repos/$env:REPOSITORY/actions/workflows/foundation.yml/runs?head_sha=$prHead&event=pull_request&status=completed&per_page=100\" --jq '.workflow_runs[] | [.id, .run_attempt, .event, .head_sha, .updated_at] | @tsv')\n" +
	"                if ($LASTEXITCODE -eq 0) {\n" +
	"                  $since = [DateTimeOffset]::UtcNow.AddHours(-24)\n" +
	"                  foreach ($row in $rows) {\n" +
	"                    $id, $attempt, $trigger, $runHead, $updated = $row -split \"`t\"\n" +
	"                    if ($trigger -ceq 'pull_request' -and $runHead -ceq $prHead -and\n" +
	"                      [DateTimeOffset]::Parse($updated, [Globalization.CultureInfo]::InvariantCulture) -ge $since) {\n" +
	"                      $jobs = @(gh api \"repos/$env:REPOSITORY/actions/runs/$id/attempts/$attempt/jobs?per_page=100\" --jq '.jobs[] | [.name, .conclusion] | @tsv')\n" +
	"                      $complete = $LASTEXITCODE -eq 0\n" +
	"                      foreach ($name in $required) {\n" +
	"                        $named = @($jobs | Where-Object { ($_ -split \"`t\")[0] -eq $name })\n" +
	"                        if ($named.Count -ne 1 -or ($named[0] -split \"`t\")[1] -ne 'success') { $complete = $false }\n" +
	"                      }\n" +
	"                      $modes = @($jobs | Where-Object { ($_ -split \"`t\")[0] -cmatch '^CI mode \\([0-9a-f]{40}\\)$' })\n" +
	"                      if ($complete -and $modes.Count -eq 1 -and ($modes[0] -split \"`t\")[1] -ceq 'success') {\n" +
	"                        $synthetic = [regex]::Match($modes[0], '^CI mode \\(([0-9a-f]{40})\\)').Groups[1].Value\n" +
	"                        $verified = @(gh api \"repos/$env:REPOSITORY/git/commits/$synthetic\" --jq '.tree.sha, (.parents | length), .parents[1].sha')\n" +
	"                        if ($LASTEXITCODE -eq 0 -and $verified.Count -eq 3 -and $verified[0] -ceq $tree -and $verified[1] -eq '2' -and $verified[2] -ceq $prHead) {\n" +
	"                          $prVerified = 'true'\n" +
	"                          \"CI mode: run $id attempt $attempt verified synthetic $synthetic with the tree $tree of $env:SHA\"\n" +
	"                        }\n" +
	"                      }\n" +
	"                    }\n" +
	"                  }\n" +
	"                }\n" +
	"              }\n" +
	"            } catch {\n" +
	"              $prVerified = 'false'\n" +
	"            }\n" +
	"          }\n" +
	"          \"heavy=$heavy\" >> $env:GITHUB_OUTPUT\n" +
	"          \"duplicate=$duplicate\" >> $env:GITHUB_OUTPUT\n" +
	"          \"pr_verified=$prVerified\" >> $env:GITHUB_OUTPUT\n" +
	"          \"CI mode: heavy=$heavy duplicate=$duplicate pr_verified=$prVerified\"\n"

// modeScript returns the run block of the mode job without any indentation (PowerShell does not
// read it, and the checks below then need not know the nesting).
func modeScript(text string) (string, error) {
	_, block, ok := strings.Cut(workflowJobBody(text, "mode"), "        run: |\n")
	if !ok {
		return "", fmt.Errorf("missing mode run block")
	}
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimLeft(line, " ")
	}
	return strings.Join(lines, "\n"), nil
}

const reuseMarker = "          $prVerified = 'false'\n"

// postMergeReusePolicy is #335: the heavy evidence of a main push is replaced by that of a
// finished pull_request run of the same tree, and only then. A skipped required check reports
// success, so the integration gate may skip on a push alone and never on a pull_request run (the
// gate policy and ci-gate verify pin the other two layers). The foundation is not touched and
// the nightly duplicate block never sees the push path.
func postMergeReusePolicy(text string) error {
	mode := workflowJobBody(text, "mode")
	for _, required := range []string{"    name: CI mode (${{ github.sha }})\n",
		"    permissions:\n      pull-requests: read\n      actions: read\n      contents: read\n    outputs:\n",
		"      pr_verified: ${{ steps.mode.outputs.pr_verified }}\n", "          REF: ${{ github.ref }}\n", reuseBlock} {
		if !strings.Contains(mode, required) {
			return fmt.Errorf("post-merge reuse policy missing: %q", required)
		}
	}
	if strings.Count(mode, "$prVerified = 'true'") != 1 || strings.Count(mode, "pr_verified=$prVerified") != 2 {
		return fmt.Errorf("mode has another reuse path")
	}
	before, block, found := strings.Cut(mode, reuseMarker)
	_, nightly, foundNightly := strings.Cut(before, "$duplicate = 'false'\n")
	reuse, _, foundOutput := strings.Cut(block, `"heavy=$heavy"`)
	if !found || !foundNightly || !foundOutput || strings.Contains(nightly, "prVerified") || strings.Contains(reuse, "$duplicate") {
		return fmt.Errorf("the nightly duplicate block and the reuse block must stay apart")
	}
	if strings.Contains(workflowJobBody(text, "foundation"), "pr_verified") || strings.Count(text, "needs.mode.outputs.pr_verified") != 2 {
		return fmt.Errorf("only native-prepare and the integration gate consult pr_verified")
	}
	if err := lightModePolicy(text); err != nil {
		return err
	}
	return nightlyDuplicatePolicy(text)
}

func TestWorkflowPostMergeReusePolicy(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	if err := postMergeReusePolicy(text); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, from, to string
		inBlock        bool // edit only the reuse block: the nightly block holds similar lines before it
	}{
		{"any-event", "if ($env:EVENT -eq 'push' -and $env:REF -eq 'refs/heads/main') {", "if ($true) {", true},
		{"pr-event", "$env:EVENT -eq 'push' -and", "$env:EVENT -eq 'pull_request' -and", true},
		{"any-ref", " -and $env:REF -eq 'refs/heads/main'", "", true},
		{"default-true", "$prVerified = 'false'\n          if", "$prVerified = 'true'\n          if", false},
		{"catch-true", "} catch {\n              $prVerified = 'false'", "} catch {\n              $prVerified = 'true'", true},
		{"single-parent", " -and $merge[1] -eq '2')", ")", true},
		{"run-lookup-ignored", "if ($LASTEXITCODE -eq 0) {\n                  $since", "if ($true) {\n                  $since", true},
		{"no-window", " -and\n                      [DateTimeOffset]::Parse($updated, [Globalization.CultureInfo]::InvariantCulture) -ge $since) {", ") {", true},
		{"event-unchecked", "$trigger -ceq 'pull_request' -and ", "", true},
		{"head-unchecked", "-and $runHead -ceq $prHead -and", "-and", true},
		{"attempt-constant", "/attempts/$attempt/jobs", "/attempts/1/jobs", true},
		{"jobs-lookup-failed", "$complete = $LASTEXITCODE -eq 0", "$complete = $true", true},
		{"required-unchecked", "foreach ($name in $required) {", "foreach ($name in @()) {", true},
		{"at-least-one", "$named.Count -ne 1 -or", "$named.Count -lt 1 -or", true},
		{"no-conclusion", " -or ($named[0] -split \"`t\")[1] -ne 'success'", "", true},
		{"mode-any", "$modes.Count -eq 1", "$modes.Count -ge 1", true},
		{"mode-unchecked", "$complete -and $modes.Count -eq 1 -and ($modes[0] -split \"`t\")[1] -ceq 'success'", "$complete", true},
		{"mode-conclusion", " -and ($modes[0] -split \"`t\")[1] -ceq 'success') {\n", ") {\n", true},
		{"case-blind-mode", "-cmatch '^CI mode", "-match '^CI mode", true},
		{"no-tree", " -and $verified[0] -ceq $tree", "", true},
		{"head-tree", "git/commits/$synthetic", "git/commits/$prHead", true},
		{"no-parent-bind", " -and $verified[2] -ceq $prHead", "", true},
		{"assignment-outside", "                        if ($LASTEXITCODE -eq 0 -and $verified.Count", "                        $prVerified = 'true'\n                        if ($LASTEXITCODE -eq 0 -and $verified.Count", true},
		{"foundation-skips", "    needs: mode\n    if: ${{ needs.mode.outputs.duplicate != 'true' }}\n", "    needs: mode\n    if: ${{ needs.mode.outputs.duplicate != 'true' && needs.mode.outputs.pr_verified != 'true' }}\n", false},
		{"prepare-ignores", " && needs.mode.outputs.pr_verified != 'true' }}\n    runs-on: ubuntu-24.04\n    timeout-minutes: 30\n", " }}\n    runs-on: ubuntu-24.04\n    timeout-minutes: 30\n", false},
		{"gate-skips-on-pr", "!(github.event_name == 'push' && needs.mode.outputs.pr_verified == 'true')", "!(needs.mode.outputs.pr_verified == 'true')", false},
		{"nightly-reuses", "if ($env:EVENT -eq 'schedule') {\n            try {", "if ($env:EVENT -eq 'schedule' -or $env:EVENT -eq 'push') {\n            try {", false},
		{"write-scope", "      contents: read\n    outputs:\n      heavy:", "      contents: read\n      actions: write\n    outputs:\n      heavy:", false},
		{"name-without-sha", "    name: CI mode (${{ github.sha }})\n", "    name: CI mode\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broken := text
			if tc.inBlock {
				head, tail, _ := strings.Cut(text, reuseMarker)
				if strings.Count(tail, tc.from) == 0 {
					t.Fatal("control anchor missing in the reuse block")
				}
				broken = head + reuseMarker + strings.Replace(tail, tc.from, tc.to, 1)
			} else {
				if strings.Count(text, tc.from) == 0 {
					t.Fatal("control anchor missing")
				}
				broken = strings.Replace(text, tc.from, tc.to, 1)
			}
			if broken == text || postMergeReusePolicy(broken) == nil {
				t.Fatal("post-merge reuse control not rejected")
			}
		})
	}
}

// reuseStub replaces gh for the real mode run block. Like the supersede stub it answers with the
// projected rows and ignores --jq and the server-side filters, so the block's own conditions are
// the only thing between a run and the verdict. The knobs (environment) shape one case each; an
// unknown call fails with exit code 2, which the block must read as a failed lookup.
const reuseStub = `$ErrorActionPreference = 'Stop'
$tab = [char]9
$fxTree = '1' * 40; $fxHead = 'b' * 40; $fxSynth = 'c' * 40; $fxOther = '9' * 40
$fxJobs = @('foundation (windows-2025)', 'foundation (ubuntu-24.04)', 'native prepare (ubuntu-24.04)', 'native routes (windows-2025)', 'native routes (ubuntu-24.04)', 'large API (windows-2025)', 'qualification (ubuntu-24.04)', 'integration gate')
function Get-Fx($name, $default) { $v = [Environment]::GetEnvironmentVariable($name); if ($v) { $v } else { $default } }
function gh {
  $a = @($args)
  $url = @($a | Where-Object { "$_" -like 'repos/*' })[0]
  $path = $url -replace '^repos/[^/]+/[^/]+/', ''
  Add-Content -LiteralPath $env:CALLS -Value $path
  $global:LASTEXITCODE = 0
  if ($path -like 'issues/*/labels') { return }
  if ($path -eq "git/commits/$env:SHA") {
    if ($env:FAIL -eq 'merge') { $global:LASTEXITCODE = 1; return }
    $fxTree
    if ($env:PARENTS -eq '1') { '1'; 'null' } else { '2'; $fxHead }
    return
  }
  if ($path -eq "git/commits/$fxSynth") {
    if ($env:FAIL -eq 'synthetic') { $global:LASTEXITCODE = 1; return }
    Get-Fx 'SYNTH_TREE' $fxTree
    '2'
    Get-Fx 'SYNTH_PARENT' $fxHead
    return
  }
  if ($path -like 'actions/workflows/foundation.yml/runs?*') {
    if ($env:FAIL -eq 'runs') { $global:LASTEXITCODE = 1; return }
    $updated = [DateTimeOffset]::UtcNow.AddHours(-[double](Get-Fx 'RUN_AGE_HOURS' '1')).ToString('o')
    foreach ($id in (Get-Fx 'RUNS' '301').Split(',')) {
      @($id, '2', (Get-Fx 'RUN_EVENT' 'pull_request'), (Get-Fx 'RUN_HEAD' $fxHead), $updated) -join $tab
    }
    return
  }
  if ($path -match '^actions/runs/(\d+)/attempts/(\d+)/jobs') {
    if ($env:FAIL -eq 'jobs') { $global:LASTEXITCODE = 1; return }
    # only the latest attempt (2) exists; any other attempt is an unknown call
    if ($Matches[2] -ne '2') { $global:LASTEXITCODE = 2; return }
    $broken = (Get-Fx 'BREAK_RUNS' '301').Split(',') -contains $Matches[1]
    $rows = @($fxJobs | ForEach-Object { "$_${tab}success" })
    if ($broken -and $env:BREAK -eq 'failure') { $rows[0] = "$($fxJobs[0])${tab}failure" }
    if ($broken -and $env:BREAK -eq 'skipped') { $rows[7] = "$($fxJobs[7])${tab}skipped" }
    if ($broken -and $env:BREAK -eq 'duplicate') { $rows += $rows[0] }
    if ($broken -and $env:BREAK -eq 'missing') { $rows = @($rows[1..7]) }
    $mode = Get-Fx 'MODE_JOB' "CI mode ($fxSynth)"
    if ($mode -ne 'none') { $rows += "$mode${tab}$(Get-Fx 'MODE_CONCLUSION' 'success')" }
    if ($broken -and $env:BREAK -eq 'two-modes') { $rows += "CI mode ($fxOther)${tab}success" }
    $rows
    return
  }
  $global:LASTEXITCODE = 2
}
`

// TestWorkflowPostMergeReuseDecision runs the mode job's own run block against the stubbed gh:
// pr_verified is true for exactly one shape of evidence and false for everything else.
func TestWorkflowPostMergeReuseDecision(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available: the mode run block is not executed")
	}
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	script, err := modeScript(text)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "mode.ps1")
	if err := os.WriteFile(file, []byte(reuseStub+script), 0o600); err != nil {
		t.Fatal(err)
	}
	synth, tree := strings.Repeat("c", 40), strings.Repeat("1", 40)
	other := strings.Repeat("9", 40)
	for _, tc := range []struct {
		name  string
		env   []string
		want  string
		noGit bool // a case that must not even read a commit
		noRun bool // a case that must not list runs
	}{
		{"verified", nil, "true", false, false},
		{"synthetic-tree-differs", []string{"SYNTH_TREE=" + other}, "false", false, false},
		{"synthetic-parent-differs", []string{"SYNTH_PARENT=" + other}, "false", false, false},
		{"single-parent-merge", []string{"PARENTS=1"}, "false", false, true},
		{"old-run", []string{"RUN_AGE_HOURS=25"}, "false", false, false},
		{"required-job-failed", []string{"BREAK=failure"}, "false", false, false},
		{"required-job-skipped", []string{"BREAK=skipped"}, "false", false, false},
		{"required-job-twice", []string{"BREAK=duplicate"}, "false", false, false},
		{"required-job-missing", []string{"BREAK=missing"}, "false", false, false},
		{"mode-job-failed", []string{"MODE_CONCLUSION=failure"}, "false", false, false},
		{"mode-job-without-sha", []string{"MODE_JOB=CI mode"}, "false", false, false},
		{"mode-job-missing", []string{"MODE_JOB=none"}, "false", false, false},
		{"two-mode-jobs", []string{"BREAK=two-modes"}, "false", false, false},
		{"merge-lookup-fails", []string{"FAIL=merge"}, "false", false, true},
		{"run-lookup-fails", []string{"FAIL=runs"}, "false", false, false},
		{"jobs-lookup-fails", []string{"FAIL=jobs"}, "false", false, false},
		{"synthetic-lookup-fails", []string{"FAIL=synthetic"}, "false", false, false},
		{"run-of-another-event", []string{"RUN_EVENT=push"}, "false", false, false},
		{"run-of-another-head", []string{"RUN_HEAD=" + other}, "false", false, false},
		{"second-run-passes", []string{"RUNS=302,301", "BREAK=failure", "BREAK_RUNS=302"}, "true", false, false},
		{"later-failing-run-keeps-verdict", []string{"RUNS=301,302", "BREAK=failure", "BREAK_RUNS=302"}, "true", false, false},
		{"pull-request-event", []string{"EVENT=pull_request"}, "false", true, false},
		{"schedule-event", []string{"EVENT=schedule"}, "false", true, false},
		{"dispatch-event", []string{"EVENT=workflow_dispatch"}, "false", true, false},
		{"push-to-another-branch", []string{"REF=refs/heads/feature"}, "false", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			calls, output := filepath.Join(dir, "calls.log"), filepath.Join(dir, "output.txt")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, pwsh, "-NoProfile", "-NonInteractive", "-File", file)
			cmd.Env = append(os.Environ(), "GH_TOKEN=unused", "REPOSITORY=o/r", "EVENT=push", "REF=refs/heads/main", "PR=12", "SHA="+strings.Repeat("e", 40),
				"GITHUB_OUTPUT="+output, "CALLS="+calls)
			cmd.Env = append(cmd.Env, tc.env...)
			out, runErr := cmd.CombinedOutput()
			if runErr != nil {
				t.Fatalf("the mode job must not fail on any lookup: %v\n%s", runErr, out)
			}
			raw, _ := os.ReadFile(output)
			got := strings.ReplaceAll(string(raw), "\r\n", "\n") // pwsh on Windows writes CRLF to GITHUB_OUTPUT
			log, _ := os.ReadFile(calls)
			if !strings.Contains(got, "pr_verified="+tc.want+"\n") || strings.Count(got, "pr_verified=") != 1 {
				t.Fatalf("want pr_verified=%s:\n%s\n%s\n%s", tc.want, got, out, log)
			}
			if tc.noGit && strings.Contains(string(log), "git/commits/") || tc.noRun && strings.Contains(string(log), "actions/workflows/") {
				t.Fatalf("lookups that the case must not reach:\n%s", log)
			}
			verified := "CI mode: run 301 attempt 2 verified synthetic " + synth + " with the tree " + tree + " of " + strings.Repeat("e", 40)
			if tc.name == "verified" && !strings.Contains(string(out), verified) {
				t.Fatalf("the verdict must name run, attempt, synthetic merge and tree:\n%s", out)
			}
		})
	}
}
