package foundation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Model every possible completed/active/pending cohort, including jobs released
// at different times. A matrix contributes its configured maximum width.
func workflowPeak(text string) (int, error) {
	_, jobs, ok := strings.Cut(text, "\njobs:\n")
	if !ok {
		return 0, fmt.Errorf("missing jobs")
	}
	entries := regexp.MustCompile(`(?m)^  ([a-z][a-z-]*):\n`).FindAllStringSubmatchIndex(jobs, -1)
	names := make(map[string]int)
	width := make([]int, len(entries))
	needs := make([][]string, len(entries))
	for i, entry := range entries {
		name := jobs[entry[2]:entry[3]]
		names[name] = i
		end := len(jobs)
		if i+1 < len(entries) {
			end = entries[i+1][0]
		}
		body := jobs[entry[1]:end]
		width[i] = 1
		if name == "qualification" || name == "integration-gate" || name == "mode" {
			width[i] = 0 // Offline aggregation and the label lookup start no native/compiler workload.
		}
		if strings.Contains(body, "    strategy:\n") {
			m := regexp.MustCompile(`(?m)^      max-parallel: ([1-9][0-9]*)$`).FindStringSubmatch(body)
			if m == nil {
				return 0, fmt.Errorf("unbounded matrix: %s", name)
			}
			width[i], _ = strconv.Atoi(m[1])
		}
		if m := regexp.MustCompile(`(?m)^    needs: ([^\n]+)$`).FindStringSubmatch(body); m != nil {
			needs[i] = strings.FieldsFunc(m[1], func(r rune) bool { return r == '[' || r == ']' || r == ',' || r == ' ' })
		}
	}
	if len(names) != 8 {
		return 0, fmt.Errorf("review newly added or missing jobs")
	}
	for _, name := range []string{"foundation", "mode", "native-prepare", "native-routes", "large-api", "qualification", "race", "integration-gate"} {
		if _, ok := names[name]; !ok {
			return 0, fmt.Errorf("missing job: %s", name)
		}
	}
	for _, deps := range needs {
		for _, dep := range deps {
			if _, ok := names[dep]; !ok {
				return 0, fmt.Errorf("unknown dependency: %s", dep)
			}
		}
	}
	// Terminal deliberately covers success, failure, skipped and cancelled alike:
	// the width bound must hold even when an always/!cancelled job is released.
	states := make([]int, len(entries)) // pending, active, terminal
	peak := 0
	var visit func(int)
	visit = func(i int) {
		if i < len(states) {
			for states[i] = 0; states[i] < 3; states[i]++ {
				visit(i + 1)
			}
			return
		}
		total := 0
		for i, state := range states {
			if state == 0 {
				continue
			}
			for _, dep := range needs[i] {
				if states[names[dep]] != 2 {
					return
				}
			}
			if state == 1 {
				total += width[i]
			}
		}
		peak = max(peak, total)
	}
	visit(0)
	return peak, nil
}

func TestWorkflowSharedHeavyLane(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	if peak, err := workflowPeak(text); err != nil || peak != 3 {
		t.Fatalf("shared heavy lane peak=%d: %v", peak, err)
	}
	// Scope the mutation to native-routes; the foundation matrix also has width 2.
	marker := "      # Windows/Linux share the width-3 lane with large-api, which starts together.\n      max-parallel: 2\n"
	if strings.Count(text, marker) != 1 {
		t.Fatal("native route control scope is not unique")
	}
	// Each control edits one job only; the same needs line is shared by three jobs.
	inJob := func(name, old, replacement string) string {
		body := workflowJobBody(text, name)
		if strings.Count(body, old) != 1 {
			t.Fatalf("%s control anchor is not unique: %q", name, old)
		}
		return strings.Replace(text, body, strings.Replace(body, old, replacement, 1), 1)
	}
	controls := map[string]string{
		"excess_route_width":     strings.Replace(text, marker, strings.Replace(marker, "max-parallel: 2", "max-parallel: 3", 1), 1),
		"excess_race_width":      inJob("race", "      max-parallel: 1\n", "      max-parallel: 4\n"),
		"routes_skip_foundation": inJob("native-routes", "    needs: [foundation, native-prepare]\n", "    needs: native-prepare\n"),
		"large_waits_mode_only":  inJob("large-api", "    needs: [foundation, native-prepare]\n", "    needs: mode\n"),
		"early_diagnostics":      strings.ReplaceAll(text, "needs: [foundation, native-prepare, native-routes, large-api, qualification]", "needs: [foundation, native-prepare]"),
	}
	for name, broken := range controls {
		if broken == text {
			t.Fatalf("%s control did not mutate scheduler", name)
		}
		if peak, err := workflowPeak(broken); err != nil || peak <= 3 {
			t.Fatalf("%s overlap missed: peak=%d: %v", name, peak, err)
		}
	}
}

func workflowJobBody(text, name string) string {
	_, body, ok := strings.Cut(text, "\n  "+name+":\n")
	if !ok {
		return ""
	}
	if end := regexp.MustCompile(`(?m)^  [a-z][a-z-]*:\n`).FindStringIndex(body); end != nil {
		body = body[:end[0]]
	}
	return body
}

func routeSchedulePolicy(text string) error {
	// #332: macOS is out of CI; no route job targets darwin
	matrix := "          - os: windows-2025\n            goos: windows\n            goarch: amd64\n          - os: ubuntu-24.04\n            goos: linux\n            goarch: amd64\n"
	body := workflowJobBody(text, "native-routes")
	_, actual, ok := strings.Cut(body, "      matrix:\n        include:\n")
	actual, _, _ = strings.Cut(actual, "    runs-on:")
	if !ok || actual != matrix {
		return fmt.Errorf("route platform coverage")
	}
	for _, required := range []string{"    name: native routes (${{ matrix.os }})\n", "    needs: [foundation, native-prepare]\n", "      fail-fast: false\n", "    runs-on: ${{ matrix.os }}\n", "    timeout-minutes: 60\n", "    defaults:\n      run:\n        shell: pwsh\n"} {
		if !strings.Contains(body, required) {
			return fmt.Errorf("route execution contract")
		}
	}
	if strings.Contains(body, "continue-on-error:") || regexp.MustCompile(`(?m)^    (if|env|permissions):`).MatchString(body) {
		return fmt.Errorf("route job override")
	}
	if strings.Contains(text, "native-routes-macos") || strings.Contains(text, "macos-15") || strings.Contains(text, "native-route-steps") {
		return fmt.Errorf("macOS jobs must stay out of the workflow")
	}
	large := workflowJobBody(text, "large-api")
	if !strings.Contains(large, "    needs: [foundation, native-prepare]\n") || regexp.MustCompile(`(?m)^    if:`).MatchString(large) {
		return fmt.Errorf("large audit must wait for the foundation and successful preparation")
	}
	qualification := workflowJobBody(text, "qualification")
	if !strings.Contains(qualification, "    needs: [native-routes]\n") ||
		!strings.Contains(qualification, "    if: ${{ !cancelled() && needs.native-routes.result != 'skipped' }}\n") {
		return fmt.Errorf("qualification must wait for all route hosts")
	}
	return nil
}

func TestWorkflowRouteSchedulePolicy(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	if err := routeSchedulePolicy(text); err != nil {
		t.Fatal(err)
	}
	for name, broken := range map[string]string{
		"missing-linux":          strings.Replace(text, "          - os: ubuntu-24.04\n            goos: linux\n            goarch: amd64\n", "", 2),
		"mac-matrix":             strings.Replace(text, "    runs-on: ${{ matrix.os }}\n    timeout-minutes: 60\n", "          - os: macos-15\n            goos: darwin\n            goarch: arm64\n    runs-on: ${{ matrix.os }}\n    timeout-minutes: 60\n", 1),
		"mac-job":                text + "\n  native-routes-macos:\n    runs-on: macos-15\n",
		"large-skips-foundation": strings.Replace(text, "    name: large API (windows-2025)\n    needs: [foundation, native-prepare]\n", "    name: large API (windows-2025)\n    needs: native-prepare\n", 1),
		"large-waits-routes":     strings.Replace(text, "    name: large API (windows-2025)\n    needs: [foundation, native-prepare]\n", "    name: large API (windows-2025)\n    needs: [foundation, native-prepare, native-routes]\n", 1),
		"routes-skip-foundation": strings.Replace(text, "    name: native routes (${{ matrix.os }})\n    needs: [foundation, native-prepare]\n", "    name: native routes (${{ matrix.os }})\n    needs: native-prepare\n", 1),
		"partial-qualification":  strings.Replace(text, "needs: [native-routes]\n", "needs: native-prepare\n", 1),
		"qual-runs-skipped":      strings.Replace(text, " && needs.native-routes.result != 'skipped' }}", " }}", 1),
		"conditional-large":      strings.Replace(text, "    name: large API (windows-2025)\n", "    name: large API (windows-2025)\n    if: ${{ always() }}\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if broken == text || routeSchedulePolicy(broken) == nil {
				t.Fatal("route scheduling/coverage control not rejected")
			}
		})
	}
}

func integrationPolicy(text string) error {
	large := workflowJobBody(text, "large-api")
	config := strings.Index(large, "      - name: Margin 실행 구성 기록\n")
	upload := strings.Index(large, "      - name: Margin 실행 구성 보관\n")
	audit := strings.Index(large, "      - name: Windows 대용량 전체 API audit\n")
	if config < 0 || upload <= config || audit <= upload || !strings.Contains(large[upload:audit], "name: ci-config-large-api-windows-${{ github.run_id }}-${{ github.run_attempt }}") {
		return fmt.Errorf("large API configuration must be retained before the audit can fail")
	}
	needs := "    needs: [foundation, native-prepare, native-routes, large-api, qualification]\n"
	// #335: the gate also needs the mode, whose pr_verified output is the only way it may skip
	gateNeeds := "    needs: [mode, foundation, native-prepare, native-routes, large-api, qualification]\n"
	// #317: the race runs on nightly and dispatch only, also when a duplicate nightly skipped
	// the foundation.
	rules := map[string]string{
		"race": `    if: ${{ !cancelled() && contains(fromJSON('["schedule","workflow_dispatch"]'), github.event_name) }}` + "\n",
	}
	for name, eventRule := range rules {
		body := workflowJobBody(text, name)
		if !strings.Contains(body, needs) || !strings.Contains(body, eventRule) || !strings.Contains(body, "      - name: 진단 실패 전파\n        if: always()\n") || !strings.Contains(body, "if ($outcome -ne 'success') { throw '") {
			return fmt.Errorf("diagnostic release/failure policy: %s", name)
		}
	}
	body := workflowJobBody(text, "integration-gate")
	for _, required := range []string{gateNeeds, "    name: integration gate\n", "    if: ${{ always() && !(github.event_name == 'schedule' && needs.foundation.result == 'skipped') && !(github.event_name == 'push' && needs.mode.outputs.pr_verified == 'true') }}\n", "    timeout-minutes: 15\n", "      contents: read\n      actions: read\n", "PRODUCER_RESULTS: ${{ toJSON(needs) }}", "GH_TOKEN: ${{ github.token }}", "& src/dev/ci-gate/fetch.ps1 -Gate ", " verify -expected ", "if ($LASTEXITCODE -ne 0) { throw 'current-run artifact download failed' }", "if ($LASTEXITCODE -ne 0) { throw 'integration gate failed' }"} {
		if !strings.Contains(body, required) {
			return fmt.Errorf("integration gate policy missing: %s", required)
		}
	}
	if strings.Contains(body, "continue-on-error:") || strings.Contains(body, "actions/download-artifact@") || strings.Contains(body, "NEW_RUN_CACHE") {
		return fmt.Errorf("integration gate suppresses failures or bypasses digest fetch")
	}
	if strings.Count(body, "GH_TOKEN:") != 1 {
		return fmt.Errorf("credential scope changed")
	}
	if !strings.Contains(text, "  workflow_dispatch:\n  schedule:\n    - cron: '23 18 * * *'\n") {
		return fmt.Errorf("nightly/dispatch trigger missing")
	}
	return nil
}

func TestWorkflowIntegrationPolicy(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	if e := integrationPolicy(text); e != nil {
		t.Fatal(e)
	}
	large := workflowJobBody(text, "large-api")
	config := large[strings.Index(large, "      - name: Margin 실행 구성 기록\n"):strings.Index(large, "      - name: Windows 대용량 전체 API audit\n")]
	lateConfig := strings.Replace(large, config, "", 1) + config
	for name, broken := range map[string]string{
		"late-large-config":        strings.Replace(text, large, lateConfig, 1),
		"wrong-large-config-name":  strings.Replace(text, "name: ci-config-large-api-windows-${{ github.run_id }}-${{ github.run_attempt }}", "name: wrong-config", 1),
		"PR-race":                  strings.ReplaceAll(text, `'["schedule","workflow_dispatch"]'`, `'["pull_request","schedule","workflow_dispatch"]'`),
		"push-race":                strings.ReplaceAll(text, `'["schedule","workflow_dispatch"]'`, `'["push","schedule","workflow_dispatch"]'`),
		"race-skips-on-duplicate":  strings.ReplaceAll(text, `'["schedule","workflow_dispatch"]'), github.event_name) }}`, `'["schedule","workflow_dispatch"]'), github.event_name) && needs.foundation.result != 'skipped' }}`),
		"race-misses-dispatch":     strings.ReplaceAll(text, `'["schedule","workflow_dispatch"]'`, `'["schedule"]'`),
		"missing-large-dependency": strings.ReplaceAll(text, "native-routes, large-api, qualification]", "native-routes, qualification]"),
		"missing-route-dependency": strings.ReplaceAll(text, "native-prepare, native-routes, large-api", "native-prepare, large-api"),
		"gate-not-always":          strings.Replace(text, "    if: ${{ always() && !(", "    if: ${{ success() && !(", 1),
		"gate-skips-off-schedule":  strings.Replace(text, "github.event_name == 'schedule' && needs.foundation.result == 'skipped'", "needs.foundation.result == 'skipped'", 1),
		// #335: a skipped required gate reports success, so only a reused main push may skip it
		"gate-skips-on-pr-event":   strings.Replace(text, " && !(github.event_name == 'push' && needs.mode.outputs.pr_verified == 'true') }}", " && !(needs.mode.outputs.pr_verified == 'true') }}", 1),
		"gate-skips-on-light":      strings.Replace(text, "github.event_name == 'push' && needs.mode.outputs.pr_verified == 'true'", "github.event_name == 'push' && needs.mode.outputs.heavy != 'true'", 1),
		"gate-skips-any-push":      strings.Replace(text, "github.event_name == 'push' && needs.mode.outputs.pr_verified == 'true'", "github.event_name == 'push'", 1),
		"gate-without-mode-need":   strings.Replace(text, "    needs: [mode, foundation, native-prepare,", "    needs: [foundation, native-prepare,", 1),
		"swallowed-fetch":          strings.Replace(text, "if ($LASTEXITCODE -ne 0) { throw 'current-run artifact download failed' }", "Write-Warning 'download failed'", 1),
		"swallowed-diagnostic":     strings.ReplaceAll(text, "if ($outcome -ne 'success') { throw '", "if ($outcome -ne 'success') { Write-Warning '"),
		"diagnostics-success-only": strings.ReplaceAll(text, "!cancelled() && contains", "success() && contains"),
	} {
		t.Run(name, func(t *testing.T) {
			if broken == text {
				t.Fatal("control did not mutate")
			}
			if integrationPolicy(broken) == nil {
				t.Fatal("invalid scheduler policy accepted")
			}
		})
	}
}

// lightModePolicy is #252: native-prepare and the foundation consult the run-time mode, which is
// light only for a pull_request carrying ci:light and full otherwise or on a failed lookup; the
// integration gate reads it only to skip a reused main push (#335), so a light run cannot pass it.
func lightModePolicy(text string) error {
	mode := workflowJobBody(text, "mode")
	for _, required := range []string{"    name: CI mode (${{ github.sha }})\n", "    runs-on: ubuntu-24.04\n", "    timeout-minutes: 5\n", "    permissions:\n      pull-requests: read\n      actions: read\n      contents: read\n    outputs:\n",
		"      heavy: ${{ steps.mode.outputs.heavy }}\n", "          $heavy = 'true'\n", "          if ($env:EVENT -eq 'pull_request') {\n",
		"            if ($LASTEXITCODE -eq 0 -and $labels -contains 'ci:light') { $heavy = 'false' }\n", `          "heavy=$heavy" >> $env:GITHUB_OUTPUT` + "\n", "          exit 0 # a failed lookup already chose full"} {
		if !strings.Contains(mode, required) {
			return fmt.Errorf("mode policy missing: %q", required)
		}
	}
	// the labels come from the API at run time (a re-run reuses the event payload) and label
	// events are no triggers: a label run would only add an unneeded run to the shared lane
	for _, required := range []string{"          EVENT: ${{ github.event_name }}\n", "          PR: ${{ github.event.pull_request.number }}\n",
		`            $labels = @(gh api --paginate "repos/$env:REPOSITORY/issues/$env:PR/labels" --jq '.[].name')` + "\n"} {
		if !strings.Contains(mode, required) {
			return fmt.Errorf("mode must read the current labels: %q", required)
		}
	}
	if !strings.Contains(text, "on:\n  pull_request:\n    branches: [main]\n  push:\n") || strings.Contains(text, "labeled") {
		return fmt.Errorf("label events must not trigger the workflow")
	}
	if strings.Contains(mode, "actions/checkout@") || strings.Contains(mode, "continue-on-error:") || strings.Count(mode, "$heavy = 'false'") != 1 {
		return fmt.Errorf("mode checks out code, hides failures or has another light path")
	}
	prepare := workflowJobBody(text, "native-prepare")
	// #317: it starts beside the foundation and consults the duplicate nightly skip itself
	// #335: and not on a main push that reused same-tree PR evidence
	if !strings.Contains(prepare, "    needs: [mode]\n    if: ${{ needs.mode.outputs.heavy == 'true' && needs.mode.outputs.duplicate != 'true' && needs.mode.outputs.pr_verified != 'true' }}\n") {
		return fmt.Errorf("native-prepare must start only on a full mode, beside the foundation")
	}
	for _, name := range []string{"native-routes", "large-api", "qualification", "race"} {
		if strings.Contains(workflowJobBody(text, name), "mode") {
			return fmt.Errorf("only native-prepare and the foundation consult the mode: %s", name)
		}
	}
	// the gate names the mode exactly twice: its needs line and the push-only skip in its if
	if strings.Count(workflowJobBody(text, "integration-gate"), "mode") != 2 {
		return fmt.Errorf("the integration gate may consult the mode only to skip a reused push")
	}
	return nil
}

func TestWorkflowLightModePolicy(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	if err := lightModePolicy(text); err != nil {
		t.Fatal(err)
	}
	for name, broken := range map[string]string{
		"unconditional-skip":        strings.Replace(text, "    if: ${{ needs.mode.outputs.heavy == 'true' && needs.mode.outputs.duplicate != 'true' && needs.mode.outputs.pr_verified != 'true' }}\n", "    if: ${{ false }}\n", 1),
		"prepare-after-foundation":  strings.Replace(text, "    needs: [mode]\n    if: ${{ needs.mode.outputs.heavy", "    needs: [foundation, mode]\n    if: ${{ needs.mode.outputs.heavy", 1),
		"prepare-ignores-duplicate": strings.Replace(text, " && needs.mode.outputs.duplicate != 'true' && needs.mode.outputs.pr_verified != 'true' }}\n    runs-on: ubuntu-24.04\n    timeout-minutes: 30\n", " && needs.mode.outputs.pr_verified != 'true' }}\n    runs-on: ubuntu-24.04\n    timeout-minutes: 30\n", 1),
		"prepare-ignores-reuse":     strings.Replace(text, " && needs.mode.outputs.pr_verified != 'true' }}\n    runs-on: ubuntu-24.04\n    timeout-minutes: 30\n", " }}\n    runs-on: ubuntu-24.04\n    timeout-minutes: 30\n", 1),
		"prepare-ignores-light":     strings.Replace(text, "    if: ${{ needs.mode.outputs.heavy == 'true' && needs.mode.outputs.duplicate", "    if: ${{ needs.mode.outputs.duplicate", 1),
		"light-on-main":             strings.Replace(text, "          if ($env:EVENT -eq 'pull_request') {\n", "          if ($true) {\n", 1),
		"default-light":             strings.Replace(text, "          $heavy = 'true'\n", "          $heavy = 'false'\n", 1),
		"failed-lookup-light":       strings.Replace(text, "if ($LASTEXITCODE -eq 0 -and $labels -contains 'ci:light')", "if ($LASTEXITCODE -ne 0 -or $labels -contains 'ci:light')", 1),
		"gate-skips-light":          strings.Replace(text, "    name: integration gate\n", "    name: integration gate\n    # mode\n", 1),
		"route-skips-light":         strings.Replace(text, "    name: large API (windows-2025)\n", "    name: large API (windows-2025)\n    # mode\n", 1),
		"lookup-exit-fails-mode":    strings.Replace(text, "          exit 0 # a failed lookup", "          # a failed lookup", 1),
		"label-trigger":             strings.Replace(text, "  pull_request:\n    branches: [main]\n", "  pull_request:\n    types: [opened, synchronize, reopened, labeled, unlabeled]\n    branches: [main]\n", 1),
		"payload-labels":            strings.Replace(text, `@(gh api --paginate "repos/$env:REPOSITORY/issues/$env:PR/labels" --jq '.[].name')`, `@($env:LABELS -split ',')`, 1),
		"mode-checkout":             strings.Replace(text, "    steps:\n      - id: mode\n", "    steps:\n      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1\n      - id: mode\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if broken == text || lightModePolicy(broken) == nil {
				t.Fatal("light mode control not rejected")
			}
		})
	}
}

// requiredRunJobs are the eight jobs of a complete earlier run (#317, #332): the two foundation
// legs, the preparation, the two route hosts, the large audit, the qualification and the
// gate. The diagnostics are not among them.
var requiredRunJobs = []string{"foundation (windows-2025)", "foundation (ubuntu-24.04)", "native prepare (ubuntu-24.04)",
	"native routes (windows-2025)", "native routes (ubuntu-24.04)", "large API (windows-2025)", "qualification (ubuntu-24.04)", "integration gate"}

// nightlyDuplicatePolicy is #257/#317: a schedule run skips the foundation, and with it every
// job but the diagnostics, only when a push or dispatch run of the same main commit finished
// in the last 24 hours and its latest attempt holds each required job once with conclusion
// success; the diagnostics' result does not count, a nightly run never counts and a failed
// lookup runs in full. The gate skips only on that skipped foundation, and only on a schedule,
// or on a reused main push (#335), so a pull_request run can never leave its required gate skipped.
func nightlyDuplicatePolicy(text string) error {
	mode := workflowJobBody(text, "mode")
	for _, required := range []string{"      duplicate: ${{ steps.mode.outputs.duplicate }}\n", "          SHA: ${{ github.sha }}\n",
		"          $duplicate = 'false'\n          $required = @('" + strings.Join(requiredRunJobs, "', '") + "')\n          if ($env:EVENT -eq 'schedule') {\n            try {\n",
		`/actions/workflows/foundation.yml/runs?head_sha=$env:SHA&status=completed&per_page=100" --jq '.workflow_runs[] | [.id, .run_attempt, .event, .head_branch, .updated_at] | @tsv')` + "\n",
		"              if ($LASTEXITCODE -eq 0) {\n                $since = [DateTimeOffset]::UtcNow.AddHours(-24)\n",
		// the fields bind in order; the job lookup of that attempt and the one duplicate
		// assignment sit inside the whole condition and the exact-once success check
		"                  $id, $attempt, $trigger, $branch, $updated = $row -split \"`t\"\n" +
			"                  if ($trigger -in @('push', 'workflow_dispatch') -and $branch -eq 'main' -and\n" +
			"                    [DateTimeOffset]::Parse($updated, [Globalization.CultureInfo]::InvariantCulture) -ge $since) {\n" +
			`                    $jobs = @(gh api "repos/$env:REPOSITORY/actions/runs/$id/attempts/$attempt/jobs?per_page=100" --jq '.jobs[] | [.name, .conclusion] | @tsv')` + "\n" +
			"                    $complete = $LASTEXITCODE -eq 0\n" +
			"                    foreach ($name in $required) {\n" +
			"                      $named = @($jobs | Where-Object { ($_ -split \"`t\")[0] -eq $name })\n" +
			"                      if ($named.Count -ne 1 -or ($named[0] -split \"`t\")[1] -ne 'success') { $complete = $false }\n" +
			"                    }\n" +
			"                    if ($complete) {\n" +
			"                      $duplicate = 'true'\n",
		"            } catch {\n              $duplicate = 'false'\n            }\n", `          "duplicate=$duplicate" >> $env:GITHUB_OUTPUT` + "\n"} {
		if !strings.Contains(mode, required) {
			return fmt.Errorf("nightly duplicate policy missing: %q", required)
		}
	}
	if strings.Count(mode, "$duplicate = 'true'") != 1 {
		return fmt.Errorf("mode has another duplicate path")
	}
	foundation := workflowJobBody(text, "foundation")
	if !strings.Contains(foundation, "    needs: mode\n    if: ${{ needs.mode.outputs.duplicate != 'true' }}\n") {
		return fmt.Errorf("the foundation must skip only on a duplicate nightly")
	}
	return integrationPolicy(text)
}

func TestWorkflowNightlyDuplicatePolicy(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	if err := nightlyDuplicatePolicy(text); err != nil {
		t.Fatal(err)
	}
	for name, broken := range map[string]string{
		"any-event":          strings.Replace(text, "          if ($env:EVENT -eq 'schedule') {\n            try {\n", "          if ($true) {\n            try {\n", 1),
		"nightly-counts":     strings.Replace(text, "@('push', 'workflow_dispatch')", "@('push', 'workflow_dispatch', 'schedule')", 1),
		"no-window":          strings.Replace(text, " -and\n                    [DateTimeOffset]::Parse($updated, [Globalization.CultureInfo]::InvariantCulture) -ge $since) {\n", ") {\n", 1),
		"any-branch":         strings.Replace(text, " -and $branch -eq 'main'", "", 1),
		"default-duplicate":  strings.Replace(text, "          $duplicate = 'false'\n          $required", "          $duplicate = 'true'\n          $required", 1),
		"failed-lookup":      strings.Replace(text, "              if ($LASTEXITCODE -eq 0) {\n                $since", "              if ($true) {\n                $since", 1),
		"error-duplicate":    strings.Replace(text, "            } catch {\n              $duplicate = 'false'\n", "            } catch {\n              $duplicate = 'true'\n", 1),
		"foundation-always":  strings.Replace(text, "    if: ${{ needs.mode.outputs.duplicate != 'true' }}\n", "    if: ${{ needs.mode.outputs.heavy == 'true' }}\n", 1),
		"gate-skips-on-pr":   strings.Replace(text, "github.event_name == 'schedule' && needs.foundation.result == 'skipped'", "needs.foundation.result == 'skipped'", 1),
		"event-constant":     strings.Replace(text, ".run_attempt, .event, .head_branch,", `.run_attempt, "push", .head_branch,`, 1),
		"branch-constant":    strings.Replace(text, ".event, .head_branch,", `.event, "main",`, 1),
		"fields-swapped":     strings.Replace(text, "$id, $attempt, $trigger, $branch, $updated =", "$id, $attempt, $branch, $trigger, $updated =", 1),
		"assignment-outside": strings.Replace(text, "                    if ($complete) {\n                      $duplicate = 'true'\n", "                    if ($complete) {\n                    }\n                    $duplicate = 'true'\n                    if ($true) {\n", 1),
		// #317: the run conclusion is not the criterion, the ten jobs of the latest attempt are
		"run-conclusion-only": strings.Replace(text, "&status=completed&per_page=100\"", "&status=success&per_page=100\"", 1),
		"no-job-check":        strings.Replace(text, "                    if ($complete) {\n", "                    if ($true) {\n", 1),
		"job-missing":         strings.Replace(text, ", 'integration gate')", ")", 1),
		"prepare-missing":     strings.Replace(text, " 'native prepare (ubuntu-24.04)',", "", 1),
		"no-conclusion":       strings.Replace(text, " -or ($named[0] -split \"`t\")[1] -ne 'success'", "", 1),
		"at-least-one":        strings.Replace(text, "$named.Count -ne 1 -or", "$named.Count -lt 1 -or", 1),
		"attempt-constant":    strings.Replace(text, "/attempts/$attempt/jobs", "/attempts/1/jobs", 1),
		"jobs-lookup-failed":  strings.Replace(text, "$complete = $LASTEXITCODE -eq 0", "$complete = $true", 1),
		"required-unchecked":  strings.Replace(text, "foreach ($name in $required) {", "foreach ($name in @()) {", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if broken == text || nightlyDuplicatePolicy(broken) == nil {
				t.Fatal("nightly duplicate control not rejected")
			}
		})
	}
}

// foundationBudget is #260. It orders the configured limits, not every possible wall time:
// each foundation step carries its own timeout, which outlasts the nominal deadlines it runs
// in sequence (each go test -timeout plus 30 seconds to build it, each timeout(1) wrapper and
// the nominal sum the scripts it calls are given here), and the job outlasts, on every host,
// the sum of the steps that host runs plus one minute of runner allowance. A go test over
// several packages applies its timeout per package, so its own wall time is not bounded by it,
// and the runner allowance is operational, not a measured bound. Only the canonical forms
// below are accepted outside whole-line comments, and a line that runs one of these commands
// may carry no inline comment, so a # inside a string can never hide code; any other form
// of these commands fails.
const cacheSaveCondition = "        if: github.event_name == 'push' && github.ref == 'refs/heads/main'"
const cacheRestoreCondition = "        if: github.event_name == 'pull_request' || github.event_name == 'push'"

func foundationBudget(text string, read func(string) (string, error)) error {
	uncomment := regexp.MustCompile(`(?m)^\s*#.*$`)
	// A witness pattern must match exactly count times in the uncommented script; its first
	// group, if any, adds factor times each captured number of seconds to the nominal sum.
	type witness struct {
		file, pattern string
		count, factor int
	}
	scripts := map[string]struct {
		args      string // the rest of a call's line must match this pattern
		witnesses []witness
	}{
		"src/dev/ci-gate/modules.ps1":            {"", nil},
		"src/dev/ci-gate/config.ps1":             {"", nil},
		"src/dev/s05-native/select-compiler.ps1": {"", nil},
		// no route is reproduced; Invoke-WebRequest -TimeoutSec bounds a connection, not a download
		"src/dev/s05-native/prepare-routes.ps1": {` -Routes none$`, nil},
		// two reproduce workspaces, each under the profile's one wall_seconds
		"src/dev/s04-reproduce/ci-owned.ps1": {"", []witness{{"src/dev/s04-reproduce/ci-owned.ps1", `wall_seconds`, 1, 0},
			{"src/dev/s04-reproduce/ci-owned.ps1", `wall_seconds = ([0-9]+);`, 1, 2}}},
		// the owned build and its cleanup waits, five owned process runs and, off Windows, two
		// evidence collections under the collector's one total limit
		"src/dev/prepare-p05/self-test.ps1": {"", []witness{{"src/dev/prepare-p05/self-test.ps1", `WaitForExit\(`, 2, 0},
			{"src/dev/prepare-p05/self-test.ps1", `WaitForExit\(([0-9]+)000\)`, 2, 1},
			{"src/dev/prepare-p05/self-test.ps1", `\bRun '`, 5, 0},
			{"src/dev/prepare-p05/self-test.ps1", `\bRun '[^']+' @\([^)]*\) ([0-9]+) `, 5, 1},
			{"src/dev/prepare-p05/self-test.ps1", `collect\.ps1`, 3, 0}, // two calls and one parse
			{"src/dev/prepare-p05/self-test.ps1", `& \(Join-Path \$PSScriptRoot 'collect\.ps1'\) -Root`, 2, 0},
			{"src/dev/prepare-p05/collect.ps1", `TotalSeconds -gt`, 1, 0},
			{"src/dev/prepare-p05/collect.ps1", `TotalSeconds -gt ([0-9]+)\b`, 1, 2}}},
	}
	body := workflowJobBody(text, "foundation")
	head, steps, ok := strings.Cut(body, "\n    steps:\n")
	m := regexp.MustCompile(`(?m)^    timeout-minutes: ([0-9]+)$`).FindStringSubmatch(head)
	if !ok || m == nil {
		return fmt.Errorf("foundation job limit missing")
	}
	job, _ := strconv.Atoi(m[1])
	steps = "\n" + steps
	if len(regexp.MustCompile(`\n      -(\s|$)`).FindAllString(steps, -1)) != len(regexp.MustCompile(`\n      - (name|uses): `).FindAllString(steps, -1)) {
		return fmt.Errorf("unrecognized foundation step form")
	}
	anyGoTest := regexp.MustCompile(`\bgo\s+test\b[^\n]*`)
	goTimeout := regexp.MustCompile(` -timeout ([1-9][0-9]*)s( |\)|$)`)
	wrapper := regexp.MustCompile(`(^|[ &])timeout --kill-after=([1-9][0-9]*)s ([1-9][0-9]*)s `)
	call := regexp.MustCompile(`& (src/[^ '"\n]+\.ps1)([^\n]*)`)
	host := regexp.MustCompile(`^        if: runner\.os == '(Windows|Linux)'$`)
	sum := map[string]int{"Windows": 1, "Linux": 1}
	for _, step := range strings.Split(steps, "\n      - ")[1:] {
		name, _, _ := strings.Cut(step, "\n")
		m := regexp.MustCompile(`(?m)^        timeout-minutes: ([0-9]+)$`).FindStringSubmatch(step)
		if m == nil {
			return fmt.Errorf("unbounded foundation step: %s", name)
		}
		minutes, _ := strconv.Atoi(m[1])
		code := uncomment.ReplaceAllString(step, "")
		for _, line := range strings.Split(code, "\n") {
			if strings.Contains(line, "#") && (anyGoTest.MatchString(line) || strings.Contains(line, ".ps1") || regexp.MustCompile(`(^|[ &])timeout\s`).MatchString(line)) {
				return fmt.Errorf("step %s: inline comment on a budget line: %s", name, line)
			}
		}
		inner := 0
		for _, line := range anyGoTest.FindAllString(code, -1) {
			inner += 30
			if strings.HasPrefix(line, "go test -c ") && !strings.Contains(line, "-timeout") {
				continue // a build only
			}
			t := goTimeout.FindAllStringSubmatch(line, -1)
			if !strings.HasPrefix(line, "go test ") || len(t) != 1 || strings.Count(line, "-timeout") != 1 {
				return fmt.Errorf("step %s: unrecognized go test timeout: %s", name, line)
			}
			n, _ := strconv.Atoi(t[0][1])
			inner += n
		}
		wrappers := wrapper.FindAllStringSubmatch(code, -1)
		if len(wrappers) != len(regexp.MustCompile(`(^|[ &])timeout\s`).FindAllString(code, -1)) {
			return fmt.Errorf("step %s: unrecognized timeout wrapper", name)
		}
		for _, w := range wrappers {
			k, _ := strconv.Atoi(w[2])
			n, _ := strconv.Atoi(w[3])
			inner += k + n
		}
		calls := call.FindAllStringSubmatch(code, -1)
		if len(calls) != strings.Count(code, ".ps1") {
			return fmt.Errorf("step %s: unrecognized script call", name)
		}
		for _, c := range calls {
			s, ok := scripts[c[1]]
			if !ok || !regexp.MustCompile(s.args).MatchString(c[2]) {
				return fmt.Errorf("step %s: script call without a declared nominal sum: %s%s", name, c[1], c[2])
			}
			for _, w := range s.witnesses {
				data, err := read(w.file)
				found := regexp.MustCompile(w.pattern).FindAllStringSubmatch(uncomment.ReplaceAllString(data, ""), -1)
				if err != nil || len(found) != w.count {
					return fmt.Errorf("step %s: deadline witness changed: %s %s", name, w.file, w.pattern)
				}
				for _, f := range found {
					if len(f) > 1 {
						n, _ := strconv.Atoi(f[1])
						inner += w.factor * n
					}
				}
			}
		}
		if inner > minutes*60 {
			return fmt.Errorf("step %s: inner deadlines %ds exceed %d minutes", name, inner, minutes)
		}
		conditions := regexp.MustCompile(`(?m)^        if: .*$`).FindAllString(step, -1)
		if len(conditions) > 1 || len(conditions) == 1 && !host.MatchString(conditions[0]) && conditions[0] != cacheSaveCondition && conditions[0] != cacheRestoreCondition {
			return fmt.Errorf("step %s: unrecognized step condition", name)
		}
		for h := range sum {
			// the cache steps run on every host of their events, so they count for each of them
			if len(conditions) == 0 || conditions[0] == cacheSaveCondition || conditions[0] == cacheRestoreCondition || host.FindStringSubmatch(conditions[0])[1] == h {
				sum[h] += minutes
			}
		}
	}
	for h, total := range sum {
		if total > job {
			return fmt.Errorf("foundation job %d minutes below the %s step sum %d", job, h, total)
		}
	}
	return nil
}

func TestWorkflowFoundationBudget(t *testing.T) {
	root := repository(t)
	text := string(mustRead(t, filepath.Join(root, ".github/workflows/foundation.yml")))
	disk := func(p string) (string, error) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		return string(b), err
	}
	if err := foundationBudget(text, disk); err != nil {
		t.Fatal(err)
	}
	vet := "          go vet ./src/...\n"
	// a comment naming these forms changes nothing
	if err := foundationBudget(strings.Replace(text, vet, "          # timeout 30s, & src/dev/x.ps1, go test -timeout 1s\n"+vet, 1), disk); err != nil {
		t.Fatal("comment rejected: ", err)
	}
	for name, broken := range map[string]string{
		"job-below-sum": strings.Replace(text, "    timeout-minutes: 64\n", "    timeout-minutes: 15\n", 1),
		// Linux runs 63 minutes of steps plus the one minute allowance
		"job-one-below":     strings.Replace(text, "    timeout-minutes: 64\n", "    timeout-minutes: 63\n", 1),
		"cache-save-grows":  strings.Replace(text, "      - name: Go build cache 저장\n        if: github.event_name == 'push' && github.ref == 'refs/heads/main'\n        timeout-minutes: 2\n", "      - name: Go build cache 저장\n        if: github.event_name == 'push' && github.ref == 'refs/heads/main'\n        timeout-minutes: 4\n", 1),
		"cache-unbounded":   strings.Replace(text, "        timeout-minutes: 2\n        continue-on-error: true\n        env:", "        continue-on-error: true\n        env:", 1),
		"cache-save-always": strings.Replace(text, "        if: github.event_name == 'push' && github.ref == 'refs/heads/main'\n", "        if: github.event_name == 'push'\n", 1),
		"unbounded-step":    strings.Replace(text, "      - name: Vet와 CGO-free build\n        timeout-minutes: 3\n", "      - name: Vet와 CGO-free build\n", 1),
		"native-outlasts":   strings.Replace(text, "go test $nativePackage -count=1 -v -timeout 420s", "go test $nativePackage -count=1 -v -timeout 480s", 1),
		"frame-outlasts":    strings.Replace(text, "      - name: frame 상태와 native cancellation 반복 시험 (#65)\n        timeout-minutes: 6\n", "      - name: frame 상태와 native cancellation 반복 시험 (#65)\n        timeout-minutes: 4\n", 1),
		"wrapper-outlasts":  strings.Replace(text, "timeout --kill-after=5s 40s unshare", "timeout --kill-after=5s 100s unshare", 1),
		"linux-step-grows":  strings.Replace(text, "        if: runner.os == 'Linux'\n        timeout-minutes: 4\n", "        if: runner.os == 'Linux'\n        timeout-minutes: 7\n", 1),
		"p05-short":         strings.Replace(text, "      - name: P05 도구 경로 회귀 검사\n        timeout-minutes: 6\n", "      - name: P05 도구 경로 회귀 검사\n        timeout-minutes: 4\n", 1),
		"timeout-equals":    strings.Replace(text, "-failfast -timeout 200s", "-failfast -timeout=900s", 1),
		"timeout-minutes":   strings.Replace(text, "-failfast -timeout 200s", "-failfast -timeout 15m", 1),
		"timeout-zero":      strings.Replace(text, "-failfast -timeout 200s", "-failfast -timeout 0s", 1),
		"no-go-timeout":     strings.Replace(text, " -json -timeout 60s)", " -json)", 1),
		"commented-timeout": strings.Replace(text, vet, "          go test ./src/internal/runner # -timeout 30s\n"+vet, 1),
		"spaced-go-test":    strings.Replace(text, vet, "          go  test ./src/internal/runner -timeout 900s\n"+vet, 1),
		"other-wrapper":     strings.Replace(text, "& timeout --kill-after=5s 15s env", "& timeout 900s env", 1),
		"unknown-script":    strings.Replace(text, "          & src/dev/ci-gate/config.ps1\n", "          & src/dev/ci-gate/config.ps1\n          & src/dev/ci-gate/other.ps1\n", 1),
		"quoted-script":     strings.Replace(text, vet, "          & 'src/dev/ci-gate/other.ps1'\n"+vet, 1),
		"routes-reproduced": strings.Replace(text, " -Platform $env:PLATFORM -Routes none", " -Platform $env:PLATFORM", 1),
		"routes-listed":     strings.Replace(text, " -Platform $env:PLATFORM -Routes none", " -Platform $env:PLATFORM -Routes none,go", 1),
		"string-hides-code": strings.Replace(text, vet, "          Write-Output 'log #1'; go test ./src/internal/runner\n"+vet, 1),
		"bare-dash-step":    strings.Replace(text, "      - name: Verify clean validation output\n", "      -\n        name: extra\n        run: Start-Sleep 3600\n      - name: Verify clean validation output\n", 1),
		"host-case":         strings.Replace(text, "      - name: Verify clean validation output\n", "      - name: extra Linux work\n        if: runner.os == 'LINUX'\n        timeout-minutes: 3\n        run: Write-Output 'extra'\n      - name: Verify clean validation output\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if broken == text || foundationBudget(broken, disk) == nil {
				t.Fatal("foundation budget control not rejected")
			}
		})
	}
	for name, change := range map[string][3]string{
		"collection-grows":   {"src/dev/prepare-p05/collect.ps1", "TotalSeconds -gt 120", "TotalSeconds -gt 1200"},
		"reproduce-override": {"src/dev/s04-reproduce/ci-owned.ps1", "\n$profile", "\n$profile.limits.wall_seconds = 600\n$profile"},
		"reproduce-grows":    {"src/dev/s04-reproduce/ci-owned.ps1", "wall_seconds = 120;", "wall_seconds = 600;"},
		"reproduce-inline":   {"src/dev/s04-reproduce/ci-owned.ps1", "\n$profilePath", "\nWrite-Output 'S04 #260'; $profile.limits.wall_seconds = 240\n$profilePath"},
		"run-timer-grows":    {"src/dev/prepare-p05/self-test.ps1", "Run 'burst' @() 2 ", "Run 'burst' @() 400 "},
		"build-wait-grows":   {"src/dev/prepare-p05/self-test.ps1", "WaitForExit(30000)", "WaitForExit(300000)"},
	} {
		t.Run(name, func(t *testing.T) {
			changed := func(p string) (string, error) {
				s, err := disk(p)
				if p == change[0] {
					if !strings.Contains(s, change[1]) {
						t.Fatalf("control anchor missing: %s", change[1])
					}
					s = strings.Replace(s, change[1], change[2], 1)
				}
				return s, err
			}
			if foundationBudget(text, changed) == nil {
				t.Fatal("script deadline change not rejected")
			}
		})
	}
}

func workflowGoVersionPolicy(foundation, reference, module string) error {
	setupStep := regexp.MustCompile(`(?m)^      - uses: actions/setup-go@[^\n]*\n`)
	for _, workflow := range []struct {
		name, text string
		count      int
	}{{"foundation", foundation, 7}, {"reference-linux", reference, 1}} {
		name, text, count := workflow.name, workflow.text, workflow.count
		starts := setupStep.FindAllStringIndex(text, -1)
		if len(starts) != count {
			return fmt.Errorf("%s has %d setup-go steps, want %d", name, len(starts), count)
		}
		blocks := make([]string, len(starts))
		for i, start := range starts {
			end := len(text)
			if next := strings.Index(text[start[1]:], "\n      - "); next >= 0 {
				end = start[1] + next + 1
			}
			blocks[i] = text[start[1]:end]
			if !strings.Contains(blocks[i], "          go-version-file: go.mod\n") || regexp.MustCompile(`(?m)^\s*go-version\s*:`).MatchString(blocks[i]) || !strings.Contains(blocks[i], "          cache: false\n") {
				return fmt.Errorf("%s setup-go step %d must read go.mod without an inline version or cache", name, i+1)
			}
		}
		if name == "foundation" && (!strings.Contains(blocks[0], "        id: setup_go\n") || strings.Count(text, "id: setup_go") != 1) {
			return fmt.Errorf("foundation must expose the first setup-go output as setup_go")
		}
		if name == "reference-linux" && strings.Contains(text, "id: setup_go") {
			return fmt.Errorf("reference-linux must not add an unused setup_go id")
		}
	}
	if regexp.MustCompile(`(?m)^[ \t]*toolchain[ \t]+`).MatchString(module) {
		return fmt.Errorf("A policy uses the go directive, without a toolchain directive")
	}
	return nil
}

func TestWorkflowGoVersionPolicy(t *testing.T) {
	foundationPath := filepath.Join(repository(t), ".github/workflows/foundation.yml")
	referencePath := filepath.Join(repository(t), ".github/workflows/reference-linux.yml")
	foundation := string(mustRead(t, foundationPath))
	reference := string(mustRead(t, referencePath))
	module := string(mustRead(t, filepath.Join(repository(t), "go.mod")))
	if err := workflowGoVersionPolicy(foundation, reference, module); err != nil {
		t.Fatal(err)
	}
	for name, broken := range map[string][3]string{
		"foundation-literal": {strings.Replace(foundation, "          go-version-file: go.mod\n", "          go-version: '1.28.4'\n", 1), reference, module},
		"foundation-file":    {strings.Replace(foundation, "          go-version-file: go.mod\n", "          go-version-file: other.mod\n", 1), reference, module},
		"reference-literal":  {foundation, strings.Replace(reference, "          go-version-file: go.mod\n", "          go-version: '1.28.4'\n", 1), module},
		"missing-first-id":   {strings.Replace(foundation, "        id: setup_go\n", "", 1), reference, module},
		"toolchain-source":   {foundation, reference, module + "toolchain go1.28.4\n"},
	} {
		t.Run(name, func(t *testing.T) {
			if broken == [3]string{foundation, reference, module} || workflowGoVersionPolicy(broken[0], broken[1], broken[2]) == nil {
				t.Fatal("unsupported Go version policy was not rejected")
			}
		})
	}
}

func workflowStep(text, name string) string {
	_, step, ok := strings.Cut(text, "      - name: "+name+"\n")
	if !ok {
		return ""
	}
	step, _, _ = strings.Cut(step, "\n      - ")
	return step
}

func workflowRunCode(step string) string {
	_, code, ok := strings.Cut(step, "        run: |\n")
	if !ok {
		return ""
	}
	lines := strings.Split(code, "\n")
	for i := range lines {
		lines[i] = strings.TrimPrefix(lines[i], "          ")
	}
	return strings.Join(lines, "\n")
}

func runWorkflowPowerShell(t *testing.T, dir, script string) error {
	t.Helper()
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell 7 is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pwsh, "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pwsh failed: %w: %s", err, output)
	}
	return nil
}

// #352 keeps the native command, failure propagation and cgroup in the existing step.
func foundationNativeMarginPolicy(text, config string) error {
	step := workflowStep(workflowJobBody(text, "foundation"), "형식과 foundation 검사")
	code := workflowRunCode(step)
	for _, need := range []string{
		"        timeout-minutes: 15\n",
		"go test @otherPackages -count=1 -v -timeout 420s\nif ($LASTEXITCODE -ne 0) { throw 'test failed' }",
		`$nativePackage = 'github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native'`,
		`$packages = @(go list ./src/...)`,
		`if ($LASTEXITCODE -ne 0 -or $packages -notcontains $nativePackage) { throw 'native package enumeration failed' }`,
		`$otherPackages = @($packages | Where-Object { $_ -ne $nativePackage })`,
		`if ($otherPackages.Count -eq 0) { throw 'other packages missing' }`,
		"$nativeWallPath = Join-Path $env:RUNNER_TEMP \"ci-native-wall-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT.txt\"\n" +
			"if (Test-Path -LiteralPath $nativeWallPath) { throw 'native command wall output exists' }\n" +
			"$nativeWatch = [Diagnostics.Stopwatch]::StartNew()\n" +
			"go test $nativePackage -count=1 -v -timeout 420s\n" +
			"$nativeWatch.Stop()\nif ($LASTEXITCODE -ne 0) { throw 'native test failed' }\n" +
			"$nativeWallFile = [IO.File]::Open($nativeWallPath, [IO.FileMode]::CreateNew)\n" +
			"try {\n  $nativeWallBytes = [Text.Encoding]::ASCII.GetBytes($nativeWatch.ElapsedMilliseconds.ToString([Globalization.CultureInfo]::InvariantCulture))\n" +
			"  $nativeWallFile.Write($nativeWallBytes)\n} finally { $nativeWallFile.Dispose() }",
		`$base = "/sys/fs/cgroup/tsgk-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT"`,
		`sudo sh -c "echo $PID > '$base/supervisor/cgroup.procs'"`,
		`$env:TSGK_CGROUP_PARENT = "$base/jobs"`,
	} {
		if !strings.Contains(step, need) && !strings.Contains(code, need) {
			return fmt.Errorf("native margin collection missing: %q", need)
		}
	}
	if strings.Index(code, "go test @otherPackages") > strings.Index(code, "$nativeWatch =") {
		return fmt.Errorf("native command moved before other packages")
	}
	transfer := "if ($env:GITHUB_JOB -ceq 'foundation') {\n" +
		"  $taskConfigArgs += @('-native-wall', (Join-Path $env:RUNNER_TEMP \"ci-native-wall-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT.txt\"))\n}\ngo @taskConfigArgs"
	if !strings.Contains(config, transfer) || strings.Count(config, "'-native-wall'") != 1 {
		return fmt.Errorf("native wall must reach foundation config only")
	}
	return nil
}

func TestWorkflowFoundationNativeMargin(t *testing.T) {
	root := repository(t)
	text := string(mustRead(t, filepath.Join(root, ".github/workflows/foundation.yml")))
	config := string(mustRead(t, filepath.Join(root, "src/dev/ci-gate/config.ps1")))
	if err := foundationNativeMarginPolicy(text, config); err != nil {
		t.Fatal(err)
	}
	for name, broken := range map[string]string{
		"record-removed":      strings.Replace(text, "$nativeWallFile.Write($nativeWallBytes)", "", 1),
		"exit-ignored":        strings.Replace(text, "if ($LASTEXITCODE -ne 0) { throw 'native test failed' }", "", 1),
		"clock-removed":       strings.Replace(text, "$nativeWatch.Stop()", "", 1),
		"collision-overwrite": strings.Replace(text, "[IO.FileMode]::CreateNew", "[IO.FileMode]::Create", 1),
		"no-precheck":         strings.Replace(text, "if (Test-Path -LiteralPath $nativeWallPath) { throw 'native command wall output exists' }", "", 1),
		"attempt-unbound":     strings.Replace(text, "ci-native-wall-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT.txt", "ci-native-wall.txt", 1),
		"native-json":         strings.Replace(text, "-v -timeout 420s", "-v -timeout 420s -json", 1),
		"cgroup-removed":      strings.Replace(text, `$env:TSGK_CGROUP_PARENT = "$base/jobs"`, "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if broken == text || foundationNativeMarginPolicy(broken, config) == nil {
				t.Fatal("workflow mutant accepted")
			}
		})
	}
	for _, broken := range []string{
		strings.Replace(config, "'-native-wall'", "'-other'", 1),
		strings.Replace(config, "-ceq 'foundation'", "-cne 'foundation'", 1),
		strings.Replace(config, "$env:GITHUB_RUN_ATTEMPT.txt", "1.txt", 1),
	} {
		if foundationNativeMarginPolicy(text, broken) == nil {
			t.Fatal("config transfer mutant accepted")
		}
	}
	code := workflowRunCode(workflowStep(workflowJobBody(text, "foundation"), "형식과 foundation 검사"))
	_, capture, ok := strings.Cut(code, "$nativeWallPath =")
	if !ok {
		t.Fatal("capture block missing")
	}
	capture = "$nativeWallPath =" + capture
	for _, scenario := range []string{"success", "failed", "existing", "race"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "ci-native-wall-900-2.txt")
			old := filepath.Join(dir, "ci-native-wall-900-1.txt")
			if err := os.WriteFile(old, []byte("earlier-attempt"), 0600); err != nil {
				t.Fatal(err)
			}
			if scenario == "existing" {
				if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			stub := "$ErrorActionPreference = 'Stop'\n$env:RUNNER_TEMP = (Get-Location).Path\n$env:GITHUB_RUN_ID = '900'\n$env:GITHUB_RUN_ATTEMPT = '2'\n$nativePackage = 'fixture'\nfunction go { $global:LASTEXITCODE = 0"
			if scenario == "failed" {
				stub += "; $global:LASTEXITCODE = 7"
			}
			if scenario == "race" {
				stub += "; [IO.File]::WriteAllText($nativeWallPath, 'keep')"
			}
			stub += " }\n"
			err := runWorkflowPowerShell(t, dir, stub+capture)
			if (err == nil) != (scenario == "success") {
				t.Fatalf("capture exit: %v", err)
			}
			b, readErr := os.ReadFile(path)
			switch scenario {
			case "success":
				ms, parseErr := strconv.ParseInt(string(b), 10, 64)
				if readErr != nil || parseErr != nil || ms < 0 {
					t.Fatalf("bad recorded wall: %q %v", b, readErr)
				}
			case "failed":
				if !os.IsNotExist(readErr) || !strings.Contains(err.Error(), "native test failed") {
					t.Fatal("failed command recorded or failure lost", err)
				}
			default:
				if readErr != nil || string(b) != "keep" {
					t.Fatal("existing observation overwritten")
				}
			}
			if b, err := os.ReadFile(old); err != nil || string(b) != "earlier-attempt" {
				t.Fatal("earlier attempt modified", err)
			}
		})
	}
}

func TestWorkflowGoVersionScripts(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("PowerShell 7 is unavailable")
	}
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	job := workflowJobBody(text, "foundation")
	cacheCode := workflowRunCode(workflowStep(job, "Go build cache 경로"))
	if cacheCode == "" {
		t.Fatal("Go cache script missing")
	}
	identityStep := workflowStep(job, "실행 identity 확인")
	identityLines := strings.Split(workflowRunCode(identityStep), "\n")
	identityCode := make([]string, 0, len(identityLines))
	for _, line := range identityLines {
		identityCode = append(identityCode, line)
		if strings.Contains(line, "Go host identity mismatch") {
			break
		}
	}
	if len(identityCode) == 0 || !strings.Contains(identityCode[len(identityCode)-1], "Go host identity mismatch") {
		t.Fatal("Go host identity check missing")
	}

	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	for _, tc := range []struct {
		name, moduleVersion, setupVersion string
		crlf, wantError                   bool
	}{
		{name: "matching version and CRLF go.mod", moduleVersion: "1.28.4", setupVersion: "1.28.4", crlf: true},
		{name: "old setup-go output after go.mod change", moduleVersion: "1.28.4", setupVersion: "1.27.2", wantError: true},
		{name: "duplicate directive", moduleVersion: "1.28.4\ngo 1.28.4", setupVersion: "1.28.4", wantError: true},
		{name: "minor-only directive", moduleVersion: "1.28", setupVersion: "1.28", wantError: true},
		{name: "empty directive", moduleVersion: "", setupVersion: "", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			module := "module example.test/candidate\ngo " + tc.moduleVersion + "\n"
			if tc.crlf {
				module = strings.ReplaceAll(module, "\n", "\r\n")
			}
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(module), 0o600); err != nil {
				t.Fatal(err)
			}
			outputPath := filepath.Join(dir, "github-output")
			stub := "function go { if ($args[0] -ceq 'env' -and $args[1] -ceq 'GOCACHE') { $global:LASTEXITCODE = 0; return '/fake/gocache' }; throw 'unexpected go invocation' }\n"
			prefix := "$env:SETUP_GO_VERSION = " + quote(tc.setupVersion) + "\n$env:GITHUB_OUTPUT = " + quote(outputPath) + "\n"
			err := runWorkflowPowerShell(t, dir, stub+prefix+cacheCode)
			if (err != nil) != tc.wantError {
				t.Fatalf("cache script error = %v, wantError %t", err, tc.wantError)
			}
			if !tc.wantError {
				written, readErr := os.ReadFile(outputPath)
				lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(written), "\r\n", "\n"), "\n"), "\n")
				if readErr != nil || len(lines) != 2 || lines[0] != "go-version="+tc.moduleVersion || lines[1] != "path=/fake/gocache" {
					t.Fatalf("cache outputs = %q, error %v", written, readErr)
				}
			}
		})
	}

	for _, tc := range []struct {
		name, identity string
		commandFails   bool
		wantError      bool
	}{
		{name: "matching host", identity: "go version go1.28.4 windows/amd64"},
		{name: "host version mismatch", identity: "go version go1.28.3 windows/amd64", wantError: true},
		{name: "host platform mismatch", identity: "go version go1.28.4 linux/amd64", wantError: true},
		{name: "go command failed", commandFails: true, wantError: true},
	} {
		t.Run("identity/"+tc.name, func(t *testing.T) {
			stub := "function go { $global:LASTEXITCODE = 0; return " + quote(tc.identity) + " }\n"
			if tc.commandFails {
				stub = "function go { $global:LASTEXITCODE = 1; return '' }\n"
			}
			prefix := "$env:ImageOS = 'windows'\n$env:ImageVersion = 'test'\n$env:EXPECTED_GO_VERSION = '1.28.4'\n$env:EXPECTED_GOOS = 'windows'\n$env:EXPECTED_GOARCH = 'amd64'\n"
			err := runWorkflowPowerShell(t, t.TempDir(), stub+prefix+strings.Join(identityCode, "\n"))
			if (err != nil) != tc.wantError {
				t.Fatalf("identity script error = %v, wantError %t", err, tc.wantError)
			}
		})
	}
}

// foundationCachePolicy is #317: every foundation leg restores the content-addressed Go build
// cache and only a push to main saves it, from commit-pinned actions of one version. Tests keep
// -count=1, so no test result is ever reused.
func foundationCachePolicy(text string) error {
	body := workflowJobBody(text, "foundation")
	step := func(name string) string {
		_, s, _ := strings.Cut(body, "      - name: "+name+"\n")
		s, _, _ = strings.Cut(s, "\n      - ")
		return "\n" + s + "\n"
	}
	key := "go-build-v1-${{ runner.os }}-${{ runner.arch }}-go${{ steps.gocache.outputs.go-version }}-${{ hashFiles('go.sum') }}-"
	pins := regexp.MustCompile(`(?m)^        uses: actions/cache/(restore|save)@([0-9a-f]{40}) # (v[0-9]+\.[0-9]+\.[0-9]+)$`).FindAllStringSubmatch(text, -1)
	if strings.Count(text, "actions/cache") != 2 || len(pins) != 2 || pins[0][1] != "restore" || pins[1][1] != "save" || pins[0][2] != pins[1][2] || pins[0][3] != pins[1][3] {
		return fmt.Errorf("the build cache needs one commit-pinned restore and one save of the same version")
	}
	path := step("Go build cache 경로")
	restore, save := step("Go build cache 복원"), step("Go build cache 저장")
	for _, required := range []string{
		"        id: gocache\n",
		"          SETUP_GO_VERSION: ${{ steps.setup_go.outputs.go-version }}\n",
		"$goMod.Length -gt 65536",
		"$goModText = $goModText.Replace(\"`r`n\", \"`n\")",
		`$goLines = [regex]::Matches($goModText, '(?m)^[ \t]*go(?:[ \t].*)?$')`,
		`$goVersions = [regex]::Matches($goModText, '(?m)^[ \t]*go[ \t]+([1-9][0-9]*\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))[ \t]*(?://[^\r\n]*)?$')`,
		"$goLines.Count -ne 1 -or $goVersions.Count -ne 1",
		"$goVersion = $goVersions[0].Groups[1].Value",
		"$goVersion -cne $env:SETUP_GO_VERSION",
		"$path = go env GOCACHE",
		`"go-version=$goVersion" >> $env:GITHUB_OUTPUT`,
		`"path=$path" >> $env:GITHUB_OUTPUT`,
	} {
		if !strings.Contains(path, required) {
			return fmt.Errorf("cache version/path step missing: %q", required)
		}
	}
	ordered := []string{
		"$goLines = [regex]::Matches",
		"$goVersions = [regex]::Matches",
		"if ($goLines.Count -ne 1 -or $goVersions.Count -ne 1)",
		"$goVersion = $goVersions[0].Groups[1].Value",
		"$goVersion -cne $env:SETUP_GO_VERSION",
		"$path = go env GOCACHE",
		`"go-version=$goVersion" >> $env:GITHUB_OUTPUT`,
	}
	for i := 1; i < len(ordered); i++ {
		if strings.Index(path, ordered[i-1]) > strings.Index(path, ordered[i]) {
			return fmt.Errorf("the bounded go.mod parser and setup-go comparison must precede cache lookup and output")
		}
	}
	if strings.Index(body, "- name: Go build cache 경로") > strings.Index(body, "- name: Go build cache 복원") {
		return fmt.Errorf("the checked Go version must be produced before cache restore")
	}
	identity := step("실행 identity 확인")
	if !strings.Contains(identity, "EXPECTED_GO_VERSION: ${{ steps.gocache.outputs.go-version }}") ||
		!strings.Contains(identity, `$goIdentity -cne "go version go$env:EXPECTED_GO_VERSION $env:EXPECTED_GOOS/$env:EXPECTED_GOARCH"`) {
		return fmt.Errorf("host identity must compare the actual version against the checked go.mod expectation")
	}
	if !strings.Contains(step("Go build cache 복원"), "${{ steps.gocache.outputs.go-version }}") || !strings.Contains(step("Go build cache 저장"), "${{ steps.gocache.outputs.go-version }}") {
		return fmt.Errorf("cache restore and save must use the checked go.mod version")
	}
	// the cache is an optional optimization: only these two steps may fail without failing the job
	if strings.Count(body, "continue-on-error:") != 2 || strings.Count(restore, "\n        continue-on-error: true\n") != 1 || strings.Count(save, "\n        continue-on-error: true\n") != 1 {
		return fmt.Errorf("continue-on-error belongs to the two cache steps only")
	}
	with := "          path: ${{ steps.gocache.outputs.path }}\n          key: " + key + "${{ github.sha }}\n"
	if !strings.Contains(restore, "uses: actions/cache/restore@") || !strings.Contains(restore, with+"          restore-keys: |\n            "+key+"\n") ||
		!strings.Contains(restore, "\n"+cacheRestoreCondition+"\n") || !strings.Contains(restore, "\n        env:\n          SEGMENT_DOWNLOAD_TIMEOUT_MINS: '1'\n") {
		return fmt.Errorf("only pull_request and push restore the cache, with a bounded download, by the epoch and sha key and the sha-free prefix")
	}
	if !strings.Contains(save, "uses: actions/cache/save@") || !strings.Contains(save, "\n"+cacheSaveCondition+"\n") || !strings.Contains(save, with) || strings.Contains(save, "restore-keys") {
		return fmt.Errorf("only a push to main saves the cache by the epoch and sha key")
	}
	// every executed go test, except a build-only go test -c, bypasses the test result cache
	code := regexp.MustCompile(`(?m)^\s*#.*$`).ReplaceAllString(body, "")
	count := regexp.MustCompile(` -count=[1-9][0-9]*( |$)`)
	for _, line := range regexp.MustCompile(`\bgo\s+test\b[^\n]*`).FindAllString(code, -1) {
		if !strings.HasPrefix(line, "go test -c ") && !count.MatchString(line) {
			return fmt.Errorf("a go test line without -count keeps the test result cache: %s", line)
		}
	}
	for _, kept := range []string{"go test @otherPackages -count=1 -v -timeout 420s", "go test $nativePackage -count=1 -v -timeout 420s"} {
		if !strings.Contains(body, kept) {
			return fmt.Errorf("the test result cache must stay off: %q", kept)
		}
	}
	return nil
}

func TestWorkflowFoundationCachePolicy(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	if err := foundationCachePolicy(text); err != nil {
		t.Fatal(err)
	}
	pin := regexp.MustCompile(`actions/cache/save@[0-9a-f]{40} # v[0-9.]+`).FindString(text)
	runner := "          go test ./src/internal/runner -count=20 -failfast -timeout 180s -run"
	for name, broken := range map[string]string{
		"tag-pin":                         strings.Replace(text, pin, "actions/cache/save@v6 # v6.1.0", 1),
		"mixed-versions":                  strings.Replace(text, pin, pin[:len(pin)-1]+"9", 1),
		"combined-action":                 strings.Replace(text, "actions/cache/restore@", "actions/cache@", 1),
		"save-everywhere":                 strings.Replace(text, "\n"+cacheSaveCondition+"\n", "\n", 1),
		"save-on-pr":                      strings.Replace(text, "github.event_name == 'push' && github.ref", "github.event_name != 'x' && github.ref", 1),
		"restore-no-sha":                  strings.Replace(text, "${{ hashFiles('go.sum') }}-${{ github.sha }}\n          restore-keys", "${{ hashFiles('go.sum') }}\n          restore-keys", 1),
		"restore-any-hash":                strings.Replace(text, "          restore-keys: |\n            go-build-v1-${{ runner.os }}-${{ runner.arch }}-go${{ steps.gocache.outputs.go-version }}-${{ hashFiles('go.sum') }}-\n", "          restore-keys: |\n            go-build-\n", 1),
		"cache-version-literal":           strings.Replace(text, "go${{ steps.gocache.outputs.go-version }}-${{ hashFiles('go.sum') }}", "go1.28.4-${{ hashFiles('go.sum') }}", 1),
		"setup-output-mismatch-unchecked": strings.Replace(text, "$goVersion -cne $env:SETUP_GO_VERSION", "$goVersion -ceq $env:SETUP_GO_VERSION", 1),
		"host-expectation-from-runtime":   strings.Replace(text, "EXPECTED_GO_VERSION: ${{ steps.gocache.outputs.go-version }}", "EXPECTED_GO_VERSION: ${{ steps.setup_go.outputs.go-version }}", 1),
		"go-mod-parser-missing":           strings.Replace(text, "$goVersions = [regex]::Matches($goModText", "$goVersions = [regex]::Matches($goModText, '^$'", 1),
		"no-epoch":                        strings.ReplaceAll(text, "go-build-v1-", "go-build-"),
		"epoch-mismatch":                  strings.Replace(text, "            go-build-v1-", "            go-build-v2-", 1),
		"restore-always":                  strings.Replace(text, "\n"+cacheRestoreCondition+"\n", "\n", 1),
		"restore-on-nightly":              strings.Replace(text, "github.event_name == 'pull_request' || github.event_name == 'push'", "github.event_name != 'workflow_dispatch'", 1),
		"restore-unbounded":               strings.Replace(text, "          SEGMENT_DOWNLOAD_TIMEOUT_MINS: '1'\n", "          SEGMENT_DOWNLOAD_TIMEOUT_MINS: '60'\n", 1),
		"restore-fails-job":               strings.Replace(text, "        timeout-minutes: 2\n        continue-on-error: true\n        env:", "        timeout-minutes: 2\n        env:", 1),
		"save-fails-job":                  strings.Replace(text, "        timeout-minutes: 2\n        continue-on-error: true\n        uses: actions/cache/save@", "        timeout-minutes: 2\n        uses: actions/cache/save@", 1),
		"error-spreads":                   strings.Replace(text, "      - name: Vet와 CGO-free build\n        timeout-minutes: 3\n", "      - name: Vet와 CGO-free build\n        timeout-minutes: 3\n        continue-on-error: true\n", 1),
		"test-cache-on":                   strings.Replace(text, "go test @otherPackages -count=1 -v", "go test @otherPackages -v", 1),
		"count-dropped":                   strings.Replace(text, runner, "          go test ./src/internal/runner -failfast -timeout 180s -run", 1),
		"count-zero":                      strings.Replace(text, runner, "          go test ./src/internal/runner -count=0 -failfast -timeout 180s -run", 1),
		"added-cached-test":               strings.Replace(text, "          go vet ./src/...\n", "          go test ./src/kit -timeout 60s\n          go vet ./src/...\n", 1),
		"cache-elsewhere":                 strings.Replace(text, "  mode:\n", "  mode:\n    # uses: actions/cache/save@0000000000000000000000000000000000000000 # v6.1.0\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if broken == text || foundationCachePolicy(broken) == nil {
				t.Fatal("foundation cache control not rejected")
			}
		})
	}
}

// concurrencyPolicy is #252/#290: one workflow-level lane that never cancels a running run and
// keeps the queue instead of replacing the pending run; no job may set its own concurrency.
// supersede.yml (#334) is the one deliberate exception: it starts no heavy workload and must not
// wait behind the runs it cancels, so TestWorkflowSupersedePolicy pins its separate group instead.
func concurrencyPolicy(text string) error {
	policy := `(?m)^concurrency:\n  group: tsgk-campaign-heavy\n  cancel-in-progress: false\n  queue: max\n\n(?:env|jobs):\n`
	keys := regexp.MustCompile(`(?m)^[\t ]*(?:concurrency|'concurrency'|"concurrency")[\t ]*:`).FindAllStringIndex(text, -1)
	if strings.Count(text, "concurrency:") != 1 || len(keys) != 1 || !regexp.MustCompile(policy).MatchString(text) {
		return fmt.Errorf("the workflow must hold exactly one shared lane with a kept queue")
	}
	return nil
}

func TestWorkflowConcurrencyPolicy(t *testing.T) {
	for _, workflow := range []string{"foundation.yml", "reference-linux.yml", "prepare-p05.yml", "prepare-p05-remedy.yml"} {
		t.Run(workflow, func(t *testing.T) {
			text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows", workflow)))
			if err := concurrencyPolicy(text); err != nil {
				t.Fatal(err)
			}
			block := "concurrency:\n  group: tsgk-campaign-heavy\n  cancel-in-progress: false\n  queue: max\n"
			jobLane := "\njobs:\n  queue-policy-control:\n    concurrency:\n      group: tsgk-campaign-heavy\n      cancel-in-progress: false\n      queue: max\n"
			for name, broken := range map[string]string{
				"single-queue":           strings.Replace(text, "  queue: max\n", "  queue: single\n", 1),
				"default-queue":          strings.Replace(text, "  queue: max\n", "", 1),
				"cancel-in-progress":     strings.Replace(text, "  cancel-in-progress: false\n", "  cancel-in-progress: true\n", 1),
				"per-pr-group":           strings.Replace(text, "  group: tsgk-campaign-heavy\n", "  group: tsgk-${{ github.event.pull_request.number || github.ref }}\n", 1),
				"no-lane":                strings.Replace(text, block, "", 1),
				"job-level-lane":         strings.Replace(strings.Replace(text, block, "", 1), "\njobs:\n", jobLane, 1),
				"extra-job-lane":         strings.Replace(text, "\njobs:\n", jobLane, 1),
				"spaced-job-lane":        strings.Replace(text, "\njobs:\n", strings.Replace(jobLane, "concurrency:", "concurrency :", 1), 1),
				"single-quoted-job-lane": strings.Replace(text, "\njobs:\n", strings.Replace(jobLane, "concurrency:", "'concurrency':", 1), 1),
				"double-quoted-job-lane": strings.Replace(text, "\njobs:\n", strings.Replace(jobLane, "concurrency:", "\"concurrency\":", 1), 1),
				"flow-job-lane":          strings.Replace(text, "\njobs:\n", "\njobs:\n  queue-policy-control: {runs-on: ubuntu-24.04, concurrency: other, steps: [{run: echo control}]}\n", 1),
				"duplicate-queue":        strings.Replace(text, "  queue: max\n", "  queue: max\n  queue: single\n", 1),
			} {
				t.Run(name, func(t *testing.T) {
					if broken == text || concurrencyPolicy(broken) == nil {
						t.Fatal("concurrency control not rejected")
					}
				})
			}
		})
	}
}

// queryScopeEvidencePolicy pins the gate-external collection and merge of the scope judgments
// (#362 L2): no new job (the gate's margin counts the run's jobs), no foundation step (its Linux
// budget has no room), no required check. The producers store files without failing, the
// qualification job merges them with steps that cannot fail it, and neither the gate's needs nor its
// artifact set name the evidence.
func queryScopeEvidencePolicy(text, gateSource string) error {
	routes := workflowJobBody(text, "native-routes")
	for _, required := range []string{
		"TSGK_QUERY_SCOPE_OUT = $scopeOut", "-run '^TestNativeQueryCompletionCurrentState$'",
		"      - name: scope 판정 근거 보관 (host별, gate 밖)\n        if: always()\n        timeout-minutes: 2\n",
		"name: query-scope-${{ matrix.goos }}-${{ github.run_id }}-${{ github.run_attempt }}", "query-scope-${{ github.run_id }}-${{ github.run_attempt }}\n          if-no-files-found: ignore\n",
		"$global:LASTEXITCODE = 0\n",
	} {
		if !strings.Contains(routes, required) {
			return fmt.Errorf("native-routes scope collection missing: %s", required)
		}
	}
	// the producer is a required check: its collection may only warn
	if !strings.Contains(routes, "Write-Output \"::warning::query scope collection failed: $_\"") || strings.Contains(workflowJobBody(text, "foundation"), "query-scope") {
		return fmt.Errorf("scope collection must warn in native-routes and stay out of the foundation job")
	}
	qual := workflowJobBody(text, "qualification")
	_, tail, ok := strings.Cut(qual, "      # #362 L2: gate-external evidence.")
	if !ok {
		return fmt.Errorf("qualification scope merge steps missing")
	}
	steps := strings.Split("\n"+tail, "\n      - ")[1:]
	if len(steps) < 5 {
		return fmt.Errorf("qualification scope merge steps incomplete")
	}
	merged := 0
	for _, step := range steps[:5] {
		name, _, _ := strings.Cut(step, "\n")
		if !strings.Contains(name, "(gate 밖)") || !strings.Contains(step, "\n        if: ${{ !cancelled()") || !regexp.MustCompile(`(?m)^        timeout-minutes: [1-9][0-9]*$`).MatchString(step) {
			return fmt.Errorf("scope merge step must be bounded and skip only on cancellation: %s", name)
		}
		// an action step cannot catch its own failure; a script step ends with LASTEXITCODE 0
		if strings.Contains(step, "\n        uses: ") != strings.Contains(step, "\n        continue-on-error: true\n") {
			return fmt.Errorf("scope merge action steps (and only they) must continue on error: %s", name)
		}
		if strings.Contains(step, "go run ./src/dev/query-scope-merge @cliArgs") {
			merged++
			if !strings.Contains(step, "$global:LASTEXITCODE = 0") || !strings.Contains(step, "} catch {") || !strings.Contains(step, "::warning::query scope merge: $_") {
				return fmt.Errorf("the merge step must only warn")
			}
		}
	}
	if merged != 1 {
		return fmt.Errorf("one merge step expected")
	}
	for _, pattern := range []string{"pattern: query-scope-*-${{ github.run_id }}-${{ github.run_attempt }}", "name: scope-merge-${{ github.run_id }}-${{ github.run_attempt }}"} {
		if !strings.Contains(qual, pattern) {
			return fmt.Errorf("current run and attempt binding missing: %s", pattern)
		}
	}
	// not a required check: the gate neither waits for nor reads this evidence
	if strings.Contains(workflowJobBody(text, "integration-gate"), "scope") || strings.Contains(gateSource, "query-scope") || strings.Contains(gateSource, "scope-merge") {
		return fmt.Errorf("the scope evidence entered the integration gate")
	}
	return nil
}

func TestWorkflowQueryScopeEvidenceIsGateExternal(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	var gateSource strings.Builder
	for _, name := range []string{"main.go", "fetch.go", "margin.go"} {
		gateSource.Write(mustRead(t, filepath.Join(repository(t), "src/dev/ci-gate", name)))
	}
	gate := gateSource.String()
	if err := queryScopeEvidencePolicy(text, gate); err != nil {
		t.Fatal(err)
	}
	download := "      - name: scope 판정 host 근거 받기 (gate 밖)\n        if: ${{ !cancelled() }}\n        timeout-minutes: 2\n        continue-on-error: true\n"
	merge := "      - name: 두 host scope 판정 병합과 fold (gate 밖)\n        if: ${{ !cancelled() }}\n        timeout-minutes: 4\n"
	for name, broken := range map[string]string{
		"download-fails-job":   strings.Replace(text, download, strings.Replace(download, "        continue-on-error: true\n", "", 1), 1),
		"download-unbounded":   strings.Replace(text, download, strings.Replace(download, "        timeout-minutes: 2\n", "", 1), 1),
		"merge-unbounded":      strings.Replace(text, merge, strings.Replace(merge, "        timeout-minutes: 4\n", "", 1), 1),
		"merge-fails-job":      strings.Replace(text, "          $global:LASTEXITCODE = 0\n      - name: scope 병합 결과 보관", "      - name: scope 병합 결과 보관", 1),
		"merge-throws":         strings.Replace(text, "Write-Output \"::warning::query scope merge: $_\"", "throw $_", 1),
		"merge-skips-failure":  strings.Replace(text, merge, strings.Replace(merge, "!cancelled()", "success()", 1), 1),
		"collection-throws":    strings.Replace(text, "Write-Output \"::warning::query scope collection failed: $_\"", "throw $_", 1),
		"collection-fails-job": strings.Replace(text, "          $env:TSGK_QUERY_SCOPE_OUT = $null\n          $global:LASTEXITCODE = 0\n", "          $env:TSGK_QUERY_SCOPE_OUT = $null\n", 1),
		"upload-requires":      strings.Replace(text, "query-scope-${{ github.run_id }}-${{ github.run_attempt }}\n          if-no-files-found: ignore\n", "query-scope-${{ github.run_id }}-${{ github.run_attempt }}\n          if-no-files-found: error\n", 1),
		"previous-attempt":     strings.ReplaceAll(text, "pattern: query-scope-*-${{ github.run_id }}-${{ github.run_attempt }}", "pattern: query-scope-*"),
		"gate-waits":           strings.ReplaceAll(text, "needs: [mode, foundation, native-prepare, native-routes, large-api, qualification]", "needs: [mode, foundation, native-prepare, native-routes, large-api, qualification, scope-merge]"),
		"foundation-step":      strings.Replace(text, "      - name: Verify clean validation output\n        timeout-minutes: 1\n        run: |\n          $treeState = git status --porcelain=v1 --untracked-files=all --ignored=matching\n          if ($LASTEXITCODE -ne 0) { throw 'CI worktree query failed' }\n          if ($treeState) { throw 'CI requires a clean worktree including untracked and ignored files' }\n\n  race:", "      - name: scope 판정 query-scope\n        timeout-minutes: 1\n        run: Write-Output 'x'\n\n  race:", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if broken == text || queryScopeEvidencePolicy(broken, gate) == nil {
				t.Fatal("scope evidence control not rejected")
			}
		})
	}
	if queryScopeEvidencePolicy(text, gate+"\nartifactSpec{\"query-scope-\"") == nil {
		t.Fatal("gate artifact set naming the evidence was not rejected")
	}
}
