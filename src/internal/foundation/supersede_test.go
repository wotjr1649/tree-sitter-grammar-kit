package foundation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// supersedeScript returns the one run block of supersede.yml without any indentation (PowerShell
// does not read it, and the checks below then need not know the nesting).
func supersedeScript(text string) (string, error) {
	_, block, ok := strings.Cut(text, "        run: |\n")
	if !ok {
		return "", fmt.Errorf("missing run block")
	}
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimLeft(line, " ")
	}
	return strings.Join(lines, "\n"), nil
}

// supersedePolicy is #334: a newer head of a pull request cancels that PR's own unfinished
// pull_request runs of the Foundation workflow and nothing else. The workflow is outside the
// heavy lane (it would wait behind the runs it cancels), holds the only actions: write grant of
// the repository at workflow level, runs no checkout, action or repository script, and calls the
// API with GET and one POST .../cancel. Every selection condition and the recheck right before
// the request are pinned, so a main push, a schedule or dispatch run, another PR's run, a fork's
// run, a finished run and the PR's current head run can never be cancelled.
func supersedePolicy(text string) error {
	const head = "name: Supersede\n\non:\n  pull_request:\n    types: [synchronize]\n    branches: [main]\n\npermissions:\n  actions: write\n  pull-requests: read\n\n"
	if !strings.HasPrefix(text, head) || strings.Count(text, "permissions:") != 1 {
		return fmt.Errorf("supersede must start on pull_request synchronize with one workflow-level actions: write and pull-requests: read")
	}
	const lane = "\nconcurrency:\n  group: tsgk-supersede-${{ github.event.pull_request.number }}\n  cancel-in-progress: true\n\njobs:\n"
	if strings.Count(text, "concurrency:") != 1 || strings.Count(text, lane) != 1 || strings.Contains(text, "tsgk-campaign-heavy") {
		return fmt.Errorf("supersede must hold its own per-PR group outside the heavy lane")
	}
	if len(regexp.MustCompile(`(?m)^  [a-z][a-z-]*:\n`).FindAllString(text, -1)) != 1 {
		return fmt.Errorf("supersede must hold exactly one job")
	}
	job := workflowJobBody(text, "cancel")
	for _, required := range []string{
		"    if: ${{ github.event.pull_request.head.repo.full_name == github.repository }}\n    runs-on: ubuntu-24.04\n    timeout-minutes: 25\n",
		"    defaults:\n      run:\n        shell: pwsh\n    steps:\n      - name: 같은 PR의 이전 head run 취소\n        env:\n" +
			"          GH_TOKEN: ${{ github.token }}\n          REPOSITORY: ${{ github.repository }}\n          REPOSITORY_ID: ${{ github.repository_id }}\n" +
			"          PR: ${{ github.event.pull_request.number }}\n          HEAD_REF: ${{ github.event.pull_request.head.ref }}\n        run: |\n",
	} {
		if !strings.Contains(job, required) {
			return fmt.Errorf("supersede job missing: %q", required)
		}
	}
	if strings.Count(job, "\n      - ") != 1 {
		return fmt.Errorf("supersede must hold exactly one step")
	}
	for _, forbidden := range []string{"uses:", "continue-on-error", "checkout", "src/", "pull_request_target", "workflow_run:", "push:", "schedule:", "workflow_dispatch:",
		"force-cancel", "rerun", "dispatches", "/caches", "/artifacts", "/logs", "-X DELETE", "-X PATCH", "-X PUT", "--method", "--input", "--field", "--raw-field", " -f ", " -F ", "Invoke-", "curl"} {
		if strings.Contains(text, forbidden) {
			return fmt.Errorf("supersede must not contain %q", forbidden)
		}
	}
	script, err := supersedeScript(text)
	if err != nil {
		return err
	}
	if strings.Contains(script, "${{") {
		return fmt.Errorf("the script may read workflow values only through env")
	}
	// the API surface: six calls, one write, and the recheck contiguous with that write
	if strings.Count(script, "gh api") != 6 || strings.Count(script, "-X ") != 1 || strings.Count(script, `/cancel"`) != 1 {
		return fmt.Errorf("supersede must call the API six times with one cancel request")
	}
	for _, required := range []string{
		"if ($LASTEXITCODE -ne 0 -or $head -notmatch '^[0-9a-f]{40}$') { throw 'PR lookup failed' }\nif ($state -cne 'open') { \"PR $env:PR is $state; nothing cancelled\"; exit 0 }\n",
		"$branch = [uri]::EscapeDataString($env:HEAD_REF)\n",
		"$rows = @(gh api --paginate \"repos/$env:REPOSITORY/actions/workflows/foundation.yml/runs?event=pull_request&branch=$branch&per_page=100\" --jq '.workflow_runs[] | [.id, .event, .status, .head_repository.id, .head_branch, .head_sha, ([.pull_requests[].number | tostring] | join(\",\"))] | @tsv')\n" +
			"if ($LASTEXITCODE -ne 0) { throw 'run lookup failed' }\n",
		"$id, $trigger, $status, $repo, $runBranch, $runHead, $prs = $row -split \"`t\"\n",
		"if ($trigger -cne 'pull_request' -or $status -ceq 'completed' -or $repo -cne $env:REPOSITORY_ID -or\n" +
			"$runBranch -cne $env:HEAD_REF -or ($prs -split ',') -notcontains $env:PR -or $runHead -ceq $head) { continue }\n",
		"$now = gh api \"repos/$env:REPOSITORY/pulls/$env:PR\" --jq '.head.sha'\nif ($LASTEXITCODE -ne 0) { throw 'PR recheck failed' }\n" +
			"$trigger, $status, $runHead = (gh api \"repos/$env:REPOSITORY/actions/runs/$id\" --jq '[.event, .status, .head_sha] | @tsv') -split \"`t\"\n" +
			"if ($LASTEXITCODE -ne 0) { throw \"run $id recheck failed\" }\n" +
			"if ($trigger -cne 'pull_request' -or $status -ceq 'completed' -or $runHead -ceq $now) { \"run $id left as is\"; continue }\n" +
			"gh api -X POST \"repos/$env:REPOSITORY/actions/runs/$id/cancel\" | Out-Null\n" +
			"if ($LASTEXITCODE -ne 0) { throw \"run $id cancel request failed\" }\n",
		"$deadline = [DateTimeOffset]::UtcNow.AddMinutes(20)\n",
		"if ($status -ceq 'completed') { \"run $id completed: $conclusion\"; break }\n" +
			"if ([DateTimeOffset]::UtcNow -gt $deadline) { throw \"run $id not terminal within 20 minutes; cancel it manually\" }\n",
	} {
		if !strings.Contains(script, required) {
			return fmt.Errorf("supersede script missing: %q", required)
		}
	}
	return nil
}

func TestWorkflowSupersedePolicy(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/supersede.yml")))
	if err := supersedePolicy(text); err != nil {
		t.Fatal(err)
	}
	// The one exception to TestWorkflowConcurrencyPolicy: this workflow is not in the heavy lane.
	if concurrencyPolicy(text) == nil {
		t.Fatal("supersede.yml must stay outside the shared heavy lane")
	}
	recheck := "            if ($trigger -cne 'pull_request' -or $status -ceq 'completed' -or $runHead -ceq $now) { \"run $id left as is\"; continue }\n"
	for _, tc := range []struct{ name, from, to string }{
		{"push-trigger", "on:\n  pull_request:", "on:\n  push:\n  pull_request:"},
		{"target-trigger", "  pull_request:\n    types", "  pull_request_target:\n    types"},
		{"all-types", "    types: [synchronize]\n", ""},
		{"write-all", "permissions:\n  actions: write\n  pull-requests: read\n", "permissions: write-all\n"},
		{"extra-scope", "  actions: write\n", "  actions: write\n  contents: write\n"},
		{"job-permissions", "    runs-on: ubuntu-24.04\n", "    permissions:\n      contents: write\n    runs-on: ubuntu-24.04\n"},
		{"heavy-lane", "group: tsgk-supersede-${{ github.event.pull_request.number }}", "group: tsgk-campaign-heavy"},
		{"keep-running", "cancel-in-progress: true", "cancel-in-progress: false"},
		{"checkout", "    steps:\n", "    steps:\n      - uses: actions/checkout@v4\n"},
		{"repo-script", "Start-Sleep -Seconds 20\n", "Start-Sleep -Seconds 20\n            & src/dev/x.ps1\n"},
		{"inline-ref", "EscapeDataString($env:HEAD_REF)", "EscapeDataString(${{ github.event.pull_request.head.ref }})"},
		{"any-event", "if ($trigger -cne 'pull_request' -or $status -ceq 'completed' -or $repo -cne", "if ($status -ceq 'completed' -or $repo -cne"},
		{"any-pr", "($prs -split ',') -notcontains $env:PR -or ", ""},
		{"any-branch", "$runBranch -cne $env:HEAD_REF -or ", ""},
		{"any-repo", " -or $repo -cne $env:REPOSITORY_ID -or\n", " -or\n"},
		{"current-head", " -or $runHead -ceq $head) { continue }", ") { continue }"},
		{"completed-too", "-or $status -ceq 'completed' -or $repo -cne", "-or $repo -cne"},
		{"no-recheck", recheck, ""},
		{"stale-recheck", "$runHead -ceq $now)", "$runHead -ceq $head)"},
		{"force", `/cancel"`, `/force-cancel"`},
		{"swallowed", "    steps:\n      - name: 같은 PR의 이전 head run 취소\n", "    steps:\n      - name: 같은 PR의 이전 head run 취소\n        continue-on-error: true\n"},
		{"no-poll", "throw \"run $id not terminal within 20 minutes; cancel it manually\"", "break"},
		{"fork-runs", "    if: ${{ github.event.pull_request.head.repo.full_name == github.repository }}\n", ""},
		{"case-blind", "$runBranch -cne $env:HEAD_REF", "$runBranch -ne $env:HEAD_REF"},
		{"other-workflow", "workflows/foundation.yml/runs", "workflows/reference-linux.yml/runs"},
		{"second-write", "Start-Sleep -Seconds 20\n", "Start-Sleep -Seconds 20\n            gh api -X POST \"repos/$env:REPOSITORY/actions/runs/$id/rerun\"\n"},
		{"delete-method", "-X POST", "-X DELETE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.Replace(text, tc.from, tc.to, 1)
			if strings.Count(text, tc.from) == 0 || broken == text || supersedePolicy(broken) == nil {
				t.Fatal("supersede control not rejected")
			}
		})
	}
}

// supersedeStub replaces gh and Start-Sleep for the real run block. The listing deliberately
// holds every kind of run a repository has, ignoring the server-side event and branch filters, so
// that the block's own conditions are the only thing between a run and its cancellation.
const supersedeStub = `$ErrorActionPreference = 'Stop'
$tab = [char]9
$H0 = 'a' * 40; $H1 = 'b' * 40; $H2 = 'c' * 40; $H3 = 'd' * 40; $M = 'e' * 40; $F1 = 'f' * 40
$table = @"
201 pull_request in_progress 100 feat/x $H1 12
202 pull_request pending 100 feat/x $H0 12
203 pull_request in_progress 100 feat/x $H2 12
204 pull_request completed 100 feat/x $H1 12
205 push in_progress 100 main $M -
206 pull_request in_progress 100 feat/x $H1 13
207 pull_request in_progress 999 feat/x $F1 -
208 pull_request queued 100 Feat/X $H1 12
209 pull_request in_progress 100 feat/x $H1 12
210 pull_request in_progress 100 feat/x $H3 12
211 schedule queued 100 main $M -
212 workflow_dispatch in_progress 100 feat/x $H1 12
"@
$lines = @($table.Trim().Split([char]10) | ForEach-Object { $_.Trim() })
function Start-Sleep { param([int]$Seconds) }
function gh {
  $a = @($args)
  $url = @($a | Where-Object { "$_" -like 'repos/*' })[0]
  $i = [array]::IndexOf($a, '--jq')
  $jq = if ($i -ge 0) { $a[$i + 1] } else { '' }
  $path = $url -replace '^repos/[^/]+/[^/]+/', ''
  $verb = if ($a -contains 'POST') { 'POST' } else { 'GET' }
  Add-Content -LiteralPath $env:CALLS -Value "$verb $path"
  $global:LASTEXITCODE = 0
  if ($verb -eq 'POST') { if ($env:POSTFAIL -eq '1') { $global:LASTEXITCODE = 1 }; return }
  if ($path -eq "pulls/$env:PR") {
    # the PR head moves to $H3 between the first lookup and the recheck
    if ($jq -like '*state*') { "$($env:PRSTATE)$tab$H2" } else { $H3 }
    return
  }
  if ($path -like 'actions/workflows/*') {
    if ($env:LISTFAIL -eq '1') { $global:LASTEXITCODE = 1; return }
    foreach ($line in $lines) { $f = $line -split ' '; if ($f[6] -eq '-') { $f[6] = '' }; $f -join $tab }
    return
  }
  if ($path -match '^actions/runs/(\d+)$') {
    if ($jq -like '*.event*') {
      $f = @($lines | Where-Object { $_ -like "$($Matches[1]) *" })[0] -split ' '
      # run 209 finishes between the listing and the recheck
      $status = if ($f[0] -eq '209') { 'completed' } else { $f[2] }
      "$($f[1])$tab$status$tab$($f[5])"
    } else { "completed${tab}cancelled" }
    return
  }
  $global:LASTEXITCODE = 2
}
`

// TestWorkflowSupersedeSelection runs the workflow's own run block against a stubbed gh. Only
// stale unfinished pull_request runs of the PR's own branch and repository may be cancelled.
func TestWorkflowSupersedeSelection(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available: the supersede run block is not executed")
	}
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/supersede.yml")))
	script, err := supersedeScript(text)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "supersede.ps1")
	if err := os.WriteFile(file, []byte(supersedeStub+script), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		env     []string
		failure bool
		posts   []int
	}{
		{"stale-runs-only", nil, false, []int{201, 202}},
		{"list-fails", []string{"LISTFAIL=1"}, true, nil},
		{"cancel-fails", []string{"POSTFAIL=1"}, true, []int{201}},
		{"closed-pr", []string{"PRSTATE=closed"}, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := filepath.Join(t.TempDir(), "calls.log")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, pwsh, "-NoProfile", "-NonInteractive", "-File", file)
			cmd.Env = append(os.Environ(), "GH_TOKEN=unused", "REPOSITORY=o/r", "REPOSITORY_ID=100", "PR=12", "HEAD_REF=feat/x", "PRSTATE=open", "CALLS="+calls)
			cmd.Env = append(cmd.Env, tc.env...)
			out, runErr := cmd.CombinedOutput()
			if (runErr != nil) != tc.failure {
				t.Fatalf("failure=%t, err=%v:\n%s", tc.failure, runErr, out)
			}
			log, _ := os.ReadFile(calls)
			var posts []int
			for _, line := range strings.Split(string(log), "\n") {
				if m := regexp.MustCompile(`^POST actions/runs/(\d+)/cancel\s*$`).FindStringSubmatch(line); m != nil {
					id, _ := strconv.Atoi(m[1])
					posts = append(posts, id)
				} else if strings.HasPrefix(line, "POST ") {
					t.Fatalf("unexpected write %q", line)
				}
			}
			if !slices.Equal(posts, tc.posts) {
				t.Fatalf("cancelled %v, want %v:\n%s\n%s", posts, tc.posts, out, log)
			}
			if tc.name == "stale-runs-only" && !strings.Contains(string(log), "GET actions/workflows/foundation.yml/runs?event=pull_request&branch=feat%2Fx&per_page=100") {
				t.Fatalf("the listing was not narrowed to the PR branch:\n%s", log)
			}
			if tc.name == "closed-pr" && strings.Contains(string(log), "actions/workflows/") {
				t.Fatalf("a closed PR must not list runs:\n%s", log)
			}
		})
	}
}
