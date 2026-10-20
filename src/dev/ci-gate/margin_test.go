package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func marginFixture(t *testing.T, id string) (marginReport, []byte, []byte) {
	t.Helper()
	r := expectation()
	r.RunID = id
	r.Event = "push"
	r.HeadSHA = r.SHA
	report := marginReport{Schema: marginSchema, Run: r, CoreSHA: strings.Repeat("f", 64), Cutoff: "2026-10-16T04:00:00Z"}
	var jobs []marginAPIJob
	number, _ := strconv.ParseUint(id, 10, 64)
	for i, j := range marginJobs {
		host := r
		host.Job, host.GOOS, host.GOARCH = j.job, j.goos, j.arch
		host.RunnerOS = map[string]string{"windows": "Windows", "linux": "Linux", "darwin": "macOS"}[j.goos]
		host.RunnerArch = map[string]string{"amd64": "X64", "arm64": "ARM64"}[j.arch]
		host.Image, host.GoVersion = j.goos+"/image-v1", "go1.27.1"
		c := marginConfig{Schema: "tsgk-ci-config/r1", Run: host, Source: strings.Repeat("c", 64)}
		if j.compiler {
			c.Compiler = &fileDigest{strings.Repeat("d", 64), 100}
		}
		report.Configs = append(report.Configs, c)
		job := marginAPIJob{ID: uint64(i + 1), RunID: number, Attempt: 2, Head: r.HeadSHA,
			marginStep: marginStep{Name: j.name, Status: "completed", Conclusion: "success", Start: "2026-10-16T03:00:00Z", End: "2026-10-16T03:01:00Z"}}
		for name := range marginStepLimits(j) {
			job.Steps = append(job.Steps, marginStep{Name: name, Status: "completed", Conclusion: "success", Start: job.Start, End: job.End})
		}
		jobs = append(jobs, job)
	}
	jobs = append(jobs, marginAPIJob{marginStep: marginStep{Name: "integration gate", Status: "in_progress"}})
	jobsData := encoded(t, map[string]any{"total_count": len(jobs), "jobs": jobs})
	core := encoded(t, map[string]any{"assessment": "PASS", "run": r, "large": map[string]any{"max_page_wall_ms": 63000}})
	metrics, err := currentMargins(r, report.Configs, jobsData, core)
	if err != nil {
		t.Fatal(err)
	}
	report.Metrics = metrics
	return report, jobsData, core
}

func TestMarginJobsSettled(t *testing.T) {
	_, jobsData, _ := marginFixture(t, "100")
	var fixture struct {
		Jobs []marginAPIJob `json:"jobs"`
	}
	if decode(jobsData, &fixture) != nil {
		t.Fatal("fixture jobs")
	}
	dir := t.TempDir()
	expected := filepath.Join(dir, "expected.json")
	if err := os.WriteFile(expected, encoded(t, expectation()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "margin", "metadata"), 0700); err != nil {
		t.Fatal(err)
	}
	cli := func(data []byte) int {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "margin", "metadata", "jobs.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		return run([]string{"margin-settled", "-expected", expected, "-history", filepath.Join(dir, "margin")}, io.Discard)
	}
	// the gate itself is still running and is not measured
	if !marginJobsSettled(jobsData) || cli(jobsData) != 0 {
		t.Fatal("settled snapshot rejected")
	}
	for name, mutate := range map[string]func(jobs []marginAPIJob) []marginAPIJob{
		"job-running":  func(jobs []marginAPIJob) []marginAPIJob { jobs[7].Status, jobs[7].End = "in_progress", ""; return jobs },
		"job-no-end":   func(jobs []marginAPIJob) []marginAPIJob { jobs[8].End = ""; return jobs },
		"step-running": func(jobs []marginAPIJob) []marginAPIJob { jobs[7].Steps[0].Status = "in_progress"; return jobs },
		"step-missing": func(jobs []marginAPIJob) []marginAPIJob { jobs[7].Steps = nil; return jobs },
		"job-missing":  func(jobs []marginAPIJob) []marginAPIJob { return jobs[1:] },
	} {
		t.Run(name, func(t *testing.T) {
			jobs := slices.Clone(fixture.Jobs)
			for i := range jobs {
				jobs[i].Steps = slices.Clone(jobs[i].Steps)
			}
			jobs = mutate(jobs)
			data := encoded(t, map[string]any{"total_count": len(jobs), "jobs": jobs})
			if marginJobsSettled(data) || cli(data) != 3 {
				t.Fatalf("%s snapshot treated as settled", name)
			}
		})
	}
	if cli([]byte("{")) != 3 {
		t.Fatal("malformed snapshot treated as settled")
	}
}

func TestMarginThresholdAndIndependentRuns(t *testing.T) {
	for _, tc := range []struct {
		n, percent int
		state      string
		failed     bool
	}{
		{19, 71, "INSUFFICIENT_SAMPLE", false}, {20, 71, "FAIL", true}, {20, 69, "PASS", false}, {20, 70, "PASS", false},
		{1, 71, "INSUFFICIENT_SAMPLE", false},
		{1, 110, "INSUFFICIENT_SAMPLE", false}, {20, 110, "FAIL", true},
	} {
		t.Run(fmt.Sprintf("%d/%d", tc.n, tc.percent), func(t *testing.T) {
			r, _, _ := marginFixture(t, "900")
			for i := range r.Metrics {
				r.Metrics[i].Used = r.Metrics[i].Limit * int64(tc.percent) / 100
			}
			var past []marginReport
			for i := 1; i < tc.n; i++ {
				p, _, _ := marginFixture(t, strconv.Itoa(i))
				past = append(past, p)
			}
			failed, err := evaluateMargins(&r, past)
			if err != nil || failed != tc.failed {
				t.Fatalf("failed=%v err=%v", failed, err)
			}
			for _, m := range r.Metrics {
				if m.State != tc.state || m.Samples != tc.n || m.Exceeded != (tc.percent > 70) {
					t.Fatalf("%+v", m)
				}
			}
		})
	}
	r, _, _ := marginFixture(t, "900")
	a, _, _ := marginFixture(t, "1")
	b, _, _ := marginFixture(t, "1")
	b.Run.RunAttempt = "3"
	for i := range b.Configs {
		b.Configs[i].Run.RunAttempt = "3"
	}
	if _, err := evaluateMargins(&r, []marginReport{a, b}); err == nil {
		t.Fatal("rerun counted as another sample")
	}
	// Historical aggregates are never imported: one report with samples=999 is one run.
	for i := range a.Metrics {
		a.Metrics[i].Samples = 999
	}
	if _, err := evaluateMargins(&r, []marginReport{a}); err != nil {
		t.Fatal(err)
	}
	for _, m := range r.Metrics {
		if m.Samples != 2 {
			t.Fatal("transitive sample inflation")
		}
	}
}

func TestMarginConfigurationAndMalformedControls(t *testing.T) {
	r, jobs, core := marginFixture(t, "900")
	if len(r.Metrics) != 17 {
		t.Fatalf("metrics=%d", len(r.Metrics))
	}
	a, _, _ := marginFixture(t, "1")
	for i := range a.Configs {
		a.Configs[i].Run.Image += "-changed"
	}
	for i := range a.Metrics {
		for _, c := range a.Configs {
			if strings.HasPrefix(a.Metrics[i].ID, c.Run.Job+"/"+c.Run.GOOS+"/") {
				a.Metrics[i].Config = marginKey(c, a.Metrics[i].ID, a.Metrics[i].Limit)
			}
		}
	}
	if _, err := evaluateMargins(&r, []marginReport{a}); err != nil {
		t.Fatal(err)
	}
	for _, m := range r.Metrics {
		if m.Samples != 1 {
			t.Fatal("foreign image combined")
		}
	}
	for name, mutate := range map[string]func(*marginReport){
		"zero-limit":         func(r *marginReport) { r.Metrics[0].Limit = 0 },
		"negative":           func(r *marginReport) { r.Metrics[0].Used = -1 },
		"unsafe-observation": func(r *marginReport) { r.Metrics[0].Used = 24*time.Hour.Milliseconds() + 1 },
		"missing":            func(r *marginReport) { r.Metrics = r.Metrics[1:] },
		"duplicate":          func(r *marginReport) { r.Metrics[1] = r.Metrics[0] },
		"forged-key":         func(r *marginReport) { r.Metrics[0].Config = strings.Repeat("0", 64) },
		"compiler":           func(r *marginReport) { r.Configs[0].Compiler = nil },
		"toolchain":          func(r *marginReport) { r.Configs[0].Run.GoVersion = "go1.27.2" },
	} {
		t.Run(name, func(t *testing.T) {
			bad, _, _ := marginFixture(t, "900")
			mutate(&bad)
			if _, err := evaluateMargins(&bad, nil); err == nil {
				t.Fatal("invalid report accepted")
			}
		})
	}
	for _, bad := range [][]byte{[]byte("{"), []byte("null"), []byte("{}"),
		[]byte(strings.Replace(string(encoded(t, r)), `"used_ms":60000`, `"used_ms":null`, 1)),
		[]byte(strings.Replace(string(encoded(t, r)), `"used_ms":60000,`, "", 1)),
	} {
		var got marginReport
		if decodeMarginReport(bad, &got) == nil {
			t.Fatal("malformed metric accepted")
		}
	}
	var api map[string]any
	if err := json.Unmarshal(jobs, &api); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"started_at", "completed_at", "status", "conclusion", "run_attempt", "head_sha"} {
		var bad map[string]any
		json.Unmarshal(jobs, &bad)
		job := bad["jobs"].([]any)[0].(map[string]any)
		delete(job, field)
		if _, err := currentMargins(r.Run, r.Configs, encoded(t, bad), core); err == nil {
			t.Fatalf("missing API %s accepted", field)
		}
	}
	badCore := []byte(strings.Replace(string(core), `"max_page_wall_ms":63000`, `"max_page_wall_ms":null`, 1))
	if _, err := currentMargins(r.Run, r.Configs, jobs, badCore); err == nil {
		t.Fatal("missing page wall became zero")
	}
}

func TestMarginSecondResolutionTimestamps(t *testing.T) {
	// GitHub reported the successful Linux bind-mount step at 05:22:12Z twice.
	s := marginStep{Status: "completed", Conclusion: "success", Start: "2026-10-16T05:22:12Z", End: "2026-10-16T05:22:12Z"}
	if ms, err := elapsed(s); err != nil || ms != 0 {
		t.Fatalf("valid second-resolution observation rejected: %d %v", ms, err)
	}
	for name, mutate := range map[string]func(*marginStep){
		"missing-start": func(s *marginStep) { s.Start = "" },
		"missing-end":   func(s *marginStep) { s.End = "" },
		"invalid":       func(s *marginStep) { s.End = "invalid" },
		"reversed":      func(s *marginStep) { s.End = "2026-10-16T05:22:11Z" },
		"skipped":       func(s *marginStep) { s.Conclusion = "skipped" },
		"unfinished":    func(s *marginStep) { s.Status = "in_progress" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := s
			mutate(&bad)
			if _, err := elapsed(bad); err == nil {
				t.Fatal("invalid timestamp observation accepted")
			}
		})
	}
	r, jobs, core := marginFixture(t, "900")
	var api struct {
		Total int            `json:"total_count"`
		Jobs  []marginAPIJob `json:"jobs"`
	}
	if err := json.Unmarshal(jobs, &api); err != nil {
		t.Fatal(err)
	}
	name := "isolated bind-mount output boundary (#196)"
	for i := range api.Jobs {
		if api.Jobs[i].Name == "foundation (ubuntu-24.04)" {
			for j := range api.Jobs[i].Steps {
				if api.Jobs[i].Steps[j].Name == name {
					api.Jobs[i].Steps[j].End = api.Jobs[i].Steps[j].Start
				}
			}
		}
	}
	metrics, err := currentMargins(r.Run, r.Configs, encoded(t, api), core)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range metrics {
		if m.ID == "foundation/linux/step/"+name && m.Used == 0 {
			return
		}
	}
	t.Fatal("zero-duration successful step metric absent")
}

func TestMarginSourceIdentity(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"src/p.go", "go.mod", "go.sum", ".github/workflows/foundation.yml", "docs/note.md"} {
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("owned\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a, err := marginSource()
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile("docs/note.md", []byte("documentation changed"), 0600)
	b, err := marginSource()
	if err != nil || a != b {
		t.Fatal("documentation split workload", err)
	}
	os.WriteFile("src/p.go", []byte("workload changed"), 0600)
	c, err := marginSource()
	if err != nil || a == c {
		t.Fatal("workload change not separated", err)
	}
}

func TestMarginHistoryDiscoveryTrustAndAttempt(t *testing.T) {
	dir := t.TempDir()
	meta := filepath.Join(dir, "current")
	hist := filepath.Join(dir, "history")
	os.Mkdir(meta, 0700)
	os.Mkdir(hist, 0700)
	expected := expectation()
	expected.Event = "push"
	expected.HeadSHA = expected.SHA
	current := marginRun{ID: 123, Attempt: 2, Event: "push", Head: expected.HeadSHA, Path: ".github/workflows/foundation.yml", Created: "2026-10-16T01:00:00Z"}
	current.Repository.ID = 42
	current.Repository.Name = repository
	past := current
	past.ID = 100
	past.Attempt = 3
	past.Status = "completed"
	past.Branch = "main"
	past.HeadRepository.ID = 42
	write := func(name string, value any) {
		t.Helper()
		if err := os.WriteFile(name, encoded(t, value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(meta, "run.json"), current)
	write(filepath.Join(hist, "cutoff.json"), map[string]string{"cutoff": "2026-10-16T02:00:00Z"})
	list := func(r marginRun) {
		write(filepath.Join(hist, "runs-1.json"), map[string]any{"total_count": 1, "workflow_runs": []marginRun{r}})
	}
	list(past)
	runs, err := marginRuns(expected, meta, hist)
	if err != nil || len(runs) != 1 || runs[0].Attempt != 3 {
		t.Fatalf("%+v %v", runs, err)
	}
	for _, raw := range []string{"{}", `{"total_count":0}`, `{"total_count":0,"workflow_runs":null}`} {
		if err := os.WriteFile(filepath.Join(hist, "runs-1.json"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := marginRuns(expected, meta, hist); err == nil {
			t.Fatal("absent/null workflow_runs became empty history")
		}
	}
	write(filepath.Join(hist, "runs-1.json"), map[string]any{"total_count": 0, "workflow_runs": []marginRun{}})
	if runs, err := marginRuns(expected, meta, hist); err != nil || runs == nil || len(runs) != 0 {
		t.Fatal("valid empty workflow_runs rejected", err)
	}
	for _, raw := range []string{"{}", `{"total_count":0}`, `{"total_count":0,"artifacts":null}`, `{"total_count":0,"artifacts":[]}`} {
		path := filepath.Join(hist, "artifact-shape.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := marginListing(path)
		valid := strings.Contains(raw, "[]")
		if valid != (err == nil) {
			t.Fatal("artifact listing required array", raw, err)
		}
	}
	for name, mutate := range map[string]func(*marginRun){
		"fork":        func(r *marginRun) { r.HeadRepository.ID = 43 },
		"workflow":    func(r *marginRun) { r.Path = "other.yml" },
		"branch":      func(r *marginRun) { r.Branch = "other" },
		"bad-created": func(r *marginRun) { r.Created = "bad" },
	} {
		t.Run(name, func(t *testing.T) {
			r := past
			mutate(&r)
			list(r)
			if _, err := marginRuns(expected, meta, hist); err == nil {
				t.Fatal("untrusted history accepted")
			}
		})
	}
	pending := past
	pending.Status = "in_progress"
	list(pending)
	if runs, err := marginRuns(expected, meta, hist); err != nil || len(runs) != 0 {
		t.Fatal("unfinished latest attempt counted")
	}
	forkPR := past
	forkPR.Event = "pull_request"
	forkPR.HeadRepository.ID = 99
	list(forkPR)
	if runs, err := marginRuns(expected, meta, hist); err != nil || runs == nil || len(runs) != 0 {
		t.Fatal("fork PR poisoned history or empty list became null", err)
	}
	old := past
	old.Created = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	list(old)
	if runs, err := marginRuns(expected, meta, hist); err != nil || len(runs) != 0 {
		t.Fatal("expired window counted")
	}
	write(filepath.Join(hist, "runs-1.json"), map[string]any{"total_count": 1001, "workflow_runs": []marginRun{past}})
	if _, err := marginRuns(expected, meta, hist); err == nil {
		t.Fatal("truncated window became insufficient sample")
	}
	write(filepath.Join(hist, "runs-1.json"), map[string]any{"total_count": 2, "workflow_runs": []marginRun{past, past}})
	if _, err := marginRuns(expected, meta, hist); err == nil {
		t.Fatal("duplicate API run counted")
	}
	a := artifactMetadata{ID: 1, Name: "ci-margin-100-2", Size: 1, Digest: "sha256:" + strings.Repeat("a", 64), Expired: new(false)}
	a.Run.ID = 100
	a.Run.Head = past.Head
	if _, ok, err := selectMarginArtifact([]artifactMetadata{a}, "ci-margin-100-3", marginIdentity(past)); err != nil || ok {
		t.Fatal("fell back to previous attempt")
	}
}

func TestMarginWorkflowLimitsAndCollection(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "workflows", "foundation.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	body := func(name string) string {
		_, s, ok := strings.Cut(text, "\n  "+name+":\n")
		if !ok {
			t.Fatalf("missing job %s", name)
		}
		if m := regexp.MustCompile("(?m)^  [a-z][a-z-]*:\n").FindStringIndex(s); m != nil {
			s = s[:m[0]]
		}
		return s
	}
	for _, j := range marginJobs {
		s := body(j.job)
		m := regexp.MustCompile("(?m)^    timeout-minutes: ([0-9]+)$").FindStringSubmatch(s)
		if len(m) != 2 || m[1] != strconv.FormatInt(j.minutes, 10) {
			t.Fatalf("job limit drift %s", j.name)
		}
		if j.job == "native-routes-macos" {
			if !strings.Contains(s, "    steps: *native-route-steps\n") {
				t.Fatal("Mac route step alias missing")
			}
			s = body("native-routes")
		}
		configName := "name: ci-config-" + j.job + "-"
		if j.job == "native-routes" || j.job == "native-routes-macos" {
			configName = "name: ci-config-${{ github.job }}-"
			if !strings.Contains(s, "path: ${{ runner.temp }}/ci-config-${{ github.job }}-${{ runner.os }}-") {
				t.Fatal("route configuration path lost actual job identity")
			}
		}
		if !strings.Contains(s, "& src/dev/ci-gate/config.ps1") || !strings.Contains(s, configName) {
			t.Fatalf("configuration capture missing %s", j.name)
		}
		for name, minutes := range marginStepLimits(j) {
			_, step, ok := strings.Cut(s, "      - name: "+name+"\n")
			if !ok {
				t.Fatalf("step not found %s", name)
			}
			if end := strings.Index(step, "\n      - "); end >= 0 {
				step = step[:end]
			}
			m := regexp.MustCompile("(?m)^        timeout-minutes: ([0-9]+)$").FindStringSubmatch(step)
			if len(m) != 2 || m[1] != strconv.FormatInt(minutes, 10) {
				t.Fatalf("step limit drift %s", name)
			}
		}
	}
	s := body("integration-gate")
	for _, need := range []string{" margin -expected ", "if ($LASTEXITCODE -ne 0) { throw 'CI margin failed' }",
		"if: $" + "{{ always() && steps.margin.outcome != 'skipped' }}", "retention-days: 30", "name: ci-margin-$" + "{{ github.run_id }}-$" + "{{ github.run_attempt }}"} {
		if !strings.Contains(s, need) {
			t.Fatalf("margin workflow policy missing %s", need)
		}
	}
	if strings.Count(s, "GH_TOKEN:") != 1 || strings.Contains(s, "continue-on-error:") {
		t.Fatal("margin credential/failure scope changed")
	}
}

func TestMarginOfflineArtifactFlowAndFailedSamplePreservation(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)
	for _, n := range []int{0, 19} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			base := t.TempDir()
			meta := filepath.Join(base, "metadata")
			hist := filepath.Join(base, "margin")
			hm := filepath.Join(hist, "metadata")
			root := filepath.Join(base, "core-artifacts")
			put := func(path string, b []byte) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			source, err := marginSource()
			if err != nil {
				t.Fatal(err)
			}
			current, jobs, core := marginFixture(t, "900")
			for i := range current.Configs {
				current.Configs[i].Source = source
			}
			core = []byte(strings.Replace(string(core), `"max_page_wall_ms":63000`, `"max_page_wall_ms":63900`, 1))
			put(filepath.Join(hm, "jobs.json"), jobs)
			put(filepath.Join(hm, "cutoff.json"), encoded(t, map[string]string{"cutoff": current.Cutoff}))
			api := marginRun{ID: 900, Attempt: 2, Event: "push", Head: current.Run.HeadSHA, Path: ".github/workflows/foundation.yml", Created: "2026-10-16T01:00:00Z"}
			api.Repository.ID = 42
			api.Repository.Name = repository
			api.HeadRepository.ID = 42
			api.Branch = "main"
			put(filepath.Join(meta, "run.json"), encoded(t, api))
			var rows []artifactMetadata
			makeArtifact := func(id uint64, name string, r kit.RunIdentity, z []byte) artifactMetadata {
				a := artifactMetadata{ID: id, Name: name, Size: int64(len(z)), Digest: "sha256:" + digest(z), Expired: new(false)}
				a.Run.ID, _ = strconv.ParseUint(r.RunID, 10, 64)
				a.Run.Head = r.HeadSHA
				put(filepath.Join(hist, "zips", name+".zip"), z)
				return a
			}
			for i, s := range artifacts(current.Run) {
				rows = append(rows, makeArtifact(uint64(i+1), s.name, current.Run, []byte("unused core ZIP in isolated margin test")))
			}
			for i, c := range current.Configs {
				j := marginJobs[i]
				z := zipBytes(t, zipEntry{"config.json", string(encoded(t, c)), 0600})
				rows = append(rows, makeArtifact(uint64(i+10), configName(j, current.Run), current.Run, z))
				if j.job == "native-routes" || j.job == "native-routes-macos" {
					dir := filepath.Join(root, "s05-routes-"+j.goos+"-900-2")
					put(filepath.Join(dir, "run-identity.json"), encoded(t, c.Run))
					put(filepath.Join(dir, "summary.json"), encoded(t, map[string]any{"compiler": c.Compiler}))
				}
				if j.job == "large-api" {
					dir := filepath.Join(root, "large-api-900-2")
					put(filepath.Join(dir, "large-api-identity-900-2/run-identity.json"), encoded(t, c.Run))
					put(filepath.Join(dir, "large-api-900-2.json"), encoded(t, map[string]any{"base_build": map[string]any{"compiler": map[string]any{"sha256": c.Compiler.SHA256, "bytes": c.Compiler.Bytes}}}))
				}
			}
			put(filepath.Join(meta, "listing-1.json"), encoded(t, map[string]any{"total_count": len(rows), "artifacts": rows}))
			pastAPI := []marginRun{}
			for i := 1; i <= n; i++ {
				p, jobs, core := marginFixture(t, strconv.Itoa(i))
				for j := range p.Configs {
					p.Configs[j].Source = source
				}
				p.Metrics, err = currentMargins(p.Run, p.Configs, jobs, core)
				if err != nil {
					t.Fatal(err)
				}
				p.CoreSHA = digest(core)
				r := api
				r.ID = uint64(i)
				r.Status = "completed"
				r.Created = "2026-10-16T00:00:00Z"
				pastAPI = append(pastAPI, r)
				name := "ci-margin-" + p.Run.RunID + "-2"
				a := makeArtifact(uint64(100+i), name, p.Run, zipBytes(t, zipEntry{"margin.json", string(encoded(t, p)), 0600}))
				put(filepath.Join(hm, fmt.Sprintf("artifacts-%d.json", i)), encoded(t, map[string]any{"total_count": 1, "artifacts": []artifactMetadata{a}}))
			}
			put(filepath.Join(hm, "runs-1.json"), encoded(t, map[string]any{"total_count": len(pastAPI), "workflow_runs": pastAPI}))
			corePath := filepath.Join(base, "result.json")
			put(corePath, core)
			out := filepath.Join(base, "margin.json")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			err = verifyMargins(ctx, current.Run, root, meta, hist, corePath, out)
			if n == 0 && err != nil || n == 19 && (err == nil || !strings.Contains(err.Error(), "exceeded 70")) {
				t.Fatalf("n=%d err=%v", n, err)
			}
			b, err := os.ReadFile(out)
			if err != nil {
				t.Fatal("valid failed sample was not preserved", err)
			}
			var got marginReport
			if err := decodeMarginReport(b, &got); err != nil {
				t.Fatal(err)
			}
			for _, m := range got.Metrics {
				if m.Samples != n+1 {
					t.Fatalf("sample count %+v", m)
				}
				if m.ID == "large-api/windows/page-max" && (m.Exceeded != true || (n == 19 && m.State != "FAIL") || (n == 0 && m.State != "INSUFFICIENT_SAMPLE")) {
					t.Fatalf("%+v", m)
				}
			}
		})
	}
}
