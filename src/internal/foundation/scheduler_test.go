package foundation

import (
	"fmt"
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
	eventRule := `    if: ${{ !cancelled() && contains(fromJSON('["push","schedule","workflow_dispatch"]'), github.event_name) }}` + "\n"
	for _, name := range []string{"race", "macos-regenerate"} {
		body := workflowJobBody(text, name)
		if !strings.Contains(body, needs) || !strings.Contains(body, eventRule) || !strings.Contains(body, "      - name: 진단 실패 전파\n        if: always()\n") || !strings.Contains(body, "if ($outcome -ne 'success') { throw '") {
			return fmt.Errorf("diagnostic release/failure policy: %s", name)
		}
	}
	body := workflowJobBody(text, "integration-gate")
	for _, required := range []string{needs, "    name: integration gate\n", "    if: ${{ always() }}\n", "    timeout-minutes: 15\n", "      contents: read\n      actions: read\n", "PRODUCER_RESULTS: ${{ toJSON(needs) }}", "GH_TOKEN: ${{ github.token }}", "& src/dev/ci-gate/fetch.ps1 -Gate ", " verify -expected ", "if ($LASTEXITCODE -ne 0) { throw 'current-run artifact download failed' }", "if ($LASTEXITCODE -ne 0) { throw 'integration gate failed' }"} {
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
		"gate-not-always":          strings.Replace(text, "    if: ${{ always() }}\n", "    if: ${{ success() }}\n", 1),
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
	for _, required := range []string{"    name: CI mode\n", "    runs-on: ubuntu-24.04\n", "    timeout-minutes: 5\n", "    permissions:\n      pull-requests: read\n    outputs:\n",
		"      heavy: ${{ steps.mode.outputs.heavy }}\n", "          $heavy = 'true'\n", "          if ($env:EVENT -eq 'pull_request') {\n",
		"            if ($LASTEXITCODE -eq 0 -and $labels -contains 'ci:light') { $heavy = 'false' }\n", `          "heavy=$heavy" >> $env:GITHUB_OUTPUT` + "\n", "          exit 0 # a failed lookup already chose full"} {
		if !strings.Contains(mode, required) {
			return fmt.Errorf("mode policy missing: %q", required)
		}
	}
	if strings.Contains(mode, "actions/checkout@") || strings.Contains(mode, "continue-on-error:") || strings.Count(mode, "$heavy = 'false'") != 1 {
		return fmt.Errorf("mode checks out code, hides failures or has another light path")
	}
	prepare := workflowJobBody(text, "native-prepare")
	if !strings.Contains(prepare, "    needs: [foundation, mode]\n    if: ${{ needs.mode.outputs.heavy == 'true' }}\n") {
		return fmt.Errorf("native-prepare must start only on a full mode")
	}
	for _, name := range []string{"foundation", "native-routes", "native-routes-macos", "large-api", "qualification", "integration-gate", "race", "macos-regenerate"} {
		if strings.Contains(workflowJobBody(text, name), "mode") {
			return fmt.Errorf("only native-prepare consults the mode: %s", name)
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
		"mode-checkout":          strings.Replace(text, "    steps:\n      - id: mode\n", "    steps:\n      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1\n      - id: mode\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if broken == text || lightModePolicy(broken) == nil {
				t.Fatal("light mode control not rejected")
			}
		})
	}
}
