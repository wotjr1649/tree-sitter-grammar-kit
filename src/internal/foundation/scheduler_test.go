package foundation

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
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
	if len(names) != 10 {
		return 0, fmt.Errorf("review newly added or missing jobs")
	}
	for _, name := range []string{"foundation", "mode", "native-prepare", "native-routes", "native-routes-macos", "large-api", "qualification", "race", "macos-regenerate", "integration-gate"} {
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
	// Scope the mutation to native-routes; the race matrix also has width 2.
	marker := "      # Windows/Linux share the width-3 lane with Mac, then large-api after Mac.\n      max-parallel: 2\n"
	if strings.Count(text, marker) != 1 {
		t.Fatal("native route control scope is not unique")
	}
	controls := map[string]string{
		"excess_route_width": strings.Replace(text, marker, strings.Replace(marker, "max-parallel: 2", "max-parallel: 3", 1), 1),
		"early_large":        strings.Replace(text, "needs: [native-prepare, native-routes-macos]", "needs: native-prepare", 1),
		"early_diagnostics":  strings.ReplaceAll(text, "needs: [foundation, native-prepare, native-routes, native-routes-macos, large-api, qualification]", "needs: native-routes"),
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
	matrices := map[string]string{
		"native-routes":       "          - os: windows-2025\n            goos: windows\n            goarch: amd64\n          - os: ubuntu-24.04\n            goos: linux\n            goarch: amd64\n",
		"native-routes-macos": "          - os: macos-15\n            goos: darwin\n            goarch: arm64\n",
	}
	for name, matrix := range matrices {
		body := workflowJobBody(text, name)
		_, actual, ok := strings.Cut(body, "      matrix:\n        include:\n")
		actual, _, _ = strings.Cut(actual, "    runs-on:")
		if !ok || actual != matrix {
			return fmt.Errorf("route platform coverage: %s", name)
		}
		for _, required := range []string{"    name: native routes (${{ matrix.os }})\n", "    needs: native-prepare\n", "      fail-fast: false\n", "    runs-on: ${{ matrix.os }}\n", "    timeout-minutes: 60\n", "    defaults:\n      run:\n        shell: pwsh\n"} {
			if !strings.Contains(body, required) {
				return fmt.Errorf("route execution contract: %s", name)
			}
		}
		if strings.Contains(body, "continue-on-error:") || regexp.MustCompile(`(?m)^    (if|env|permissions):`).MatchString(body) {
			return fmt.Errorf("route job override: %s", name)
		}
	}
	if strings.Count(text, "    steps: &native-route-steps\n") != 1 ||
		!strings.Contains(workflowJobBody(text, "native-routes"), "    steps: &native-route-steps\n") ||
		!strings.HasSuffix(strings.TrimSpace(workflowJobBody(text, "native-routes-macos")), "steps: *native-route-steps") {
		return fmt.Errorf("route steps must share one definition")
	}
	large := workflowJobBody(text, "large-api")
	if !strings.Contains(large, "    needs: [native-prepare, native-routes-macos]\n") || regexp.MustCompile(`(?m)^    if:`).MatchString(large) {
		return fmt.Errorf("large audit must wait for successful preparation and Mac")
	}
	qualification := workflowJobBody(text, "qualification")
	if !strings.Contains(qualification, "    needs: [native-routes, native-routes-macos]\n") ||
		!strings.Contains(qualification, "    if: ${{ !cancelled() && needs.native-routes.result != 'skipped' && needs.native-routes-macos.result != 'skipped' }}\n") {
		return fmt.Errorf("qualification must wait for all route hosts")
	}
	return nil
}

func TestWorkflowRouteSchedulePolicy(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	if err := routeSchedulePolicy(text); err != nil {
		t.Fatal(err)
	}
	mac := workflowJobBody(text, "native-routes-macos")
	for name, broken := range map[string]string{
		"missing-linux":         strings.Replace(text, "          - os: ubuntu-24.04\n            goos: linux\n            goarch: amd64\n", "", 2),
		"duplicate-mac":         strings.Replace(text, mac, strings.Replace(mac, "os: macos-15\n            goos: darwin\n            goarch: arm64", "os: windows-2025\n            goos: windows\n            goarch: amd64", 1), 1),
		"wrong-alias":           strings.Replace(text, "steps: *native-route-steps", "steps: *other-steps", 1),
		"early-large":           strings.Replace(text, "needs: [native-prepare, native-routes-macos]", "needs: native-prepare", 1),
		"partial-qualification": strings.Replace(text, "needs: [native-routes, native-routes-macos]", "needs: native-routes", 1),
		"conditional-large":     strings.Replace(text, "    name: large API (windows-2025)\n", "    name: large API (windows-2025)\n    if: ${{ always() }}\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if broken == text || routeSchedulePolicy(broken) == nil {
				t.Fatal("route scheduling/coverage control not rejected")
			}
		})
	}
}

func integrationPolicy(text string) error {
	needs := "    needs: [foundation, native-prepare, native-routes, native-routes-macos, large-api, qualification]\n"
	eventRule := `    if: ${{ !cancelled() && contains(fromJSON('["push","schedule","workflow_dispatch"]'), github.event_name) && needs.foundation.result != 'skipped' }}` + "\n"
	for _, name := range []string{"race", "macos-regenerate"} {
		body := workflowJobBody(text, name)
		if !strings.Contains(body, needs) || !strings.Contains(body, eventRule) || !strings.Contains(body, "      - name: 진단 실패 전파\n        if: always()\n") || !strings.Contains(body, "if ($outcome -ne 'success') { throw '") {
			return fmt.Errorf("diagnostic release/failure policy: %s", name)
		}
	}
	body := workflowJobBody(text, "integration-gate")
	for _, required := range []string{needs, "    name: integration gate\n", "    if: ${{ always() && !(github.event_name == 'schedule' && needs.foundation.result == 'skipped') }}\n", "    timeout-minutes: 15\n", "      contents: read\n      actions: read\n", "PRODUCER_RESULTS: ${{ toJSON(needs) }}", "GH_TOKEN: ${{ github.token }}", "& src/dev/ci-gate/fetch.ps1 -Gate ", " verify -expected ", "if ($LASTEXITCODE -ne 0) { throw 'current-run artifact download failed' }", "if ($LASTEXITCODE -ne 0) { throw 'integration gate failed' }"} {
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
	for name, broken := range map[string]string{
		"PR-diagnostic":            strings.ReplaceAll(text, `["push","schedule","workflow_dispatch"]`, `["pull_request","push","schedule","workflow_dispatch"]`),
		"PR-target-diagnostic":     strings.ReplaceAll(text, `["push","schedule","workflow_dispatch"]`, `["pull_request_target","push","schedule","workflow_dispatch"]`),
		"missing-large-dependency": strings.ReplaceAll(text, "native-routes-macos, large-api, qualification]", "native-routes-macos, qualification]"),
		"missing-Mac-dependency":   strings.ReplaceAll(text, "native-routes, native-routes-macos, large-api", "native-routes, large-api"),
		"gate-not-always":          strings.Replace(text, "    if: ${{ always() && !(", "    if: ${{ success() && !(", 1),
		"gate-skips-off-schedule":  strings.Replace(text, "github.event_name == 'schedule' && needs.foundation.result == 'skipped'", "needs.foundation.result == 'skipped'", 1),
		"diagnostics-on-duplicate": strings.ReplaceAll(text, " && needs.foundation.result != 'skipped' }}", " }}"),
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

// lightModePolicy is #252: only native-prepare consults the run-time mode, which is light
// only for a pull_request carrying ci:light and full otherwise or on a failed lookup; the
// integration gate neither needs mode nor skips, so a light run cannot pass it.
func lightModePolicy(text string) error {
	mode := workflowJobBody(text, "mode")
	for _, required := range []string{"    name: CI mode\n", "    runs-on: ubuntu-24.04\n", "    timeout-minutes: 5\n", "    permissions:\n      pull-requests: read\n      actions: read\n    outputs:\n",
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
	if !strings.Contains(prepare, "    needs: [foundation, mode]\n    if: ${{ needs.mode.outputs.heavy == 'true' }}\n") {
		return fmt.Errorf("native-prepare must start only on a full mode")
	}
	for _, name := range []string{"native-routes", "native-routes-macos", "large-api", "qualification", "integration-gate", "race", "macos-regenerate"} {
		if strings.Contains(workflowJobBody(text, name), "mode") {
			return fmt.Errorf("only native-prepare and the foundation consult the mode: %s", name)
		}
	}
	return nil
}

func TestWorkflowLightModePolicy(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	if err := lightModePolicy(text); err != nil {
		t.Fatal(err)
	}
	for name, broken := range map[string]string{
		"unconditional-skip":     strings.Replace(text, "    if: ${{ needs.mode.outputs.heavy == 'true' }}\n", "    if: ${{ false }}\n", 1),
		"light-on-main":          strings.Replace(text, "          if ($env:EVENT -eq 'pull_request') {\n", "          if ($true) {\n", 1),
		"default-light":          strings.Replace(text, "          $heavy = 'true'\n", "          $heavy = 'false'\n", 1),
		"failed-lookup-light":    strings.Replace(text, "if ($LASTEXITCODE -eq 0 -and $labels -contains 'ci:light')", "if ($LASTEXITCODE -ne 0 -or $labels -contains 'ci:light')", 1),
		"gate-skips-light":       strings.Replace(text, "    name: integration gate\n", "    name: integration gate\n    # mode\n", 1),
		"route-skips-light":      strings.Replace(text, "    name: large API (windows-2025)\n", "    name: large API (windows-2025)\n    # mode\n", 1),
		"lookup-exit-fails-mode": strings.Replace(text, "          exit 0 # a failed lookup", "          # a failed lookup", 1),
		"label-trigger":          strings.Replace(text, "  pull_request:\n    branches: [main]\n", "  pull_request:\n    types: [opened, synchronize, reopened, labeled, unlabeled]\n    branches: [main]\n", 1),
		"payload-labels":         strings.Replace(text, `@(gh api --paginate "repos/$env:REPOSITORY/issues/$env:PR/labels" --jq '.[].name')`, `@($env:LABELS -split ',')`, 1),
		"mode-checkout":          strings.Replace(text, "    steps:\n      - id: mode\n", "    steps:\n      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1\n      - id: mode\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if broken == text || lightModePolicy(broken) == nil {
				t.Fatal("light mode control not rejected")
			}
		})
	}
}

// nightlyDuplicatePolicy is #257: a schedule run skips the foundation, and with it every
// job, only when a successful push or dispatch run of the same main commit finished in the
// last 24 hours; a nightly success never counts and a failed lookup runs in full. The gate
// and the diagnostics skip only on that skipped foundation, and the gate only on a schedule,
// so a pull_request or push run can never leave its required gate skipped.
func nightlyDuplicatePolicy(text string) error {
	mode := workflowJobBody(text, "mode")
	for _, required := range []string{"      duplicate: ${{ steps.mode.outputs.duplicate }}\n", "          SHA: ${{ github.sha }}\n",
		"          $duplicate = 'false'\n          if ($env:EVENT -eq 'schedule') {\n            try {\n",
		`/actions/workflows/foundation.yml/runs?head_sha=$env:SHA&status=success&per_page=100" --jq '.workflow_runs[] | [.id, .event, .head_branch, .conclusion, .updated_at] | @tsv')` + "\n",
		"              if ($LASTEXITCODE -eq 0) {\n                $since = [DateTimeOffset]::UtcNow.AddHours(-24)\n",
		// the fields bind in order and the one duplicate assignment sits inside the whole condition
		"                  $id, $trigger, $branch, $conclusion, $updated = $row -split \"`t\"\n" +
			"                  if ($trigger -in @('push', 'workflow_dispatch') -and $branch -eq 'main' -and $conclusion -eq 'success' -and\n" +
			"                    [DateTimeOffset]::Parse($updated, [Globalization.CultureInfo]::InvariantCulture) -ge $since) {\n" +
			"                    $duplicate = 'true'\n",
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
		"no-window":          strings.Replace(text, " -and $conclusion -eq 'success' -and\n                    [DateTimeOffset]::Parse($updated, [Globalization.CultureInfo]::InvariantCulture) -ge $since) {\n", ") {\n", 1),
		"any-branch":         strings.Replace(text, " -and $branch -eq 'main'", "", 1),
		"default-duplicate":  strings.Replace(text, "          $duplicate = 'false'\n          if ($env:EVENT", "          $duplicate = 'true'\n          if ($env:EVENT", 1),
		"failed-lookup":      strings.Replace(text, "              if ($LASTEXITCODE -eq 0) {\n                $since", "              if ($true) {\n                $since", 1),
		"error-duplicate":    strings.Replace(text, "            } catch {\n              $duplicate = 'false'\n", "            } catch {\n              $duplicate = 'true'\n", 1),
		"foundation-always":  strings.Replace(text, "    if: ${{ needs.mode.outputs.duplicate != 'true' }}\n", "    if: ${{ needs.mode.outputs.heavy == 'true' }}\n", 1),
		"gate-skips-on-pr":   strings.Replace(text, "github.event_name == 'schedule' && needs.foundation.result == 'skipped'", "needs.foundation.result == 'skipped'", 1),
		"event-constant":     strings.Replace(text, "[.id, .event, .head_branch,", `[.id, "push", .head_branch,`, 1),
		"branch-constant":    strings.Replace(text, "[.id, .event, .head_branch,", `[.id, .event, "main",`, 1),
		"fields-swapped":     strings.Replace(text, "$id, $trigger, $branch, $conclusion, $updated =", "$id, $branch, $trigger, $conclusion, $updated =", 1),
		"assignment-outside": strings.Replace(text, " -ge $since) {\n                    $duplicate = 'true'\n", " -ge $since) {\n                  }\n                  $duplicate = 'true'\n                  {\n", 1),
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
	host := regexp.MustCompile(`^        if: runner\.os == '(Windows|Linux|macOS)'$`)
	sum := map[string]int{"Windows": 1, "Linux": 1, "macOS": 1}
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
		if len(conditions) > 1 || len(conditions) == 1 && !host.MatchString(conditions[0]) {
			return fmt.Errorf("step %s: unrecognized step condition", name)
		}
		for h := range sum {
			if len(conditions) == 0 || host.FindStringSubmatch(conditions[0])[1] == h {
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
		"job-below-sum":     strings.Replace(text, "    timeout-minutes: 57\n", "    timeout-minutes: 15\n", 1),
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

// concurrencyPolicy is #252/#290: one workflow-level lane that never cancels a running run and
// keeps the queue instead of replacing the pending run; no job may set its own concurrency.
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
