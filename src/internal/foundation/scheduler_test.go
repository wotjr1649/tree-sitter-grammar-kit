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
		if name == "qualification" || name == "integration-gate" {
			width[i] = 0 // Offline aggregation does not start a native/compiler workload.
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
	for _, name := range []string{"foundation", "native-prepare", "native-routes", "large-api", "qualification", "race", "macos-regenerate", "integration-gate"} {
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
	marker := "      # Two route hosts plus the independent large-api job share the width-3 lane.\n      max-parallel: 2\n"
	if strings.Count(text, marker) != 1 {
		t.Fatal("native route control scope is not unique")
	}
	controls := map[string]string{
		"three_routes_plus_large": strings.Replace(text, marker, strings.Replace(marker, "max-parallel: 2", "max-parallel: 3", 1), 1),
		"early_diagnostics":       strings.ReplaceAll(text, "needs: [foundation, native-prepare, native-routes, large-api, qualification]", "needs: native-routes"),
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

func integrationPolicy(text string) error {
	needs := "    needs: [foundation, native-prepare, native-routes, large-api, qualification]\n"
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
		"missing-large-dependency": strings.ReplaceAll(text, "native-routes, large-api, qualification]", "native-routes, qualification]"),
		"gate-not-always":          strings.Replace(text, "    if: ${{ always() }}\n", "    if: ${{ success() }}\n", 1),
		"swallowed-fetch":          strings.Replace(text, "if ($LASTEXITCODE -ne 0) { throw 'current-run artifact download failed' }", "Write-Warning 'download failed'", 1),
		"swallowed-diagnostic":     strings.ReplaceAll(text, "if ($outcome -ne 'success') { throw '", "if ($outcome -ne 'success') { Write-Warning '"),
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
