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
	if len(names) != 7 {
		return 0, fmt.Errorf("review newly added or missing jobs")
	}
	for _, deps := range needs {
		for _, dep := range deps {
			if _, ok := names[dep]; !ok {
				return 0, fmt.Errorf("unknown dependency: %s", dep)
			}
		}
	}
	states := make([]int, len(entries)) // pending, active, completed
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
		"early_diagnostics":       strings.ReplaceAll(text, "needs: [native-routes, large-api, qualification]", "needs: native-routes"),
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
