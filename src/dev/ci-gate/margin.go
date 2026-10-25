package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const marginSchema = "tsgk-ci-margin/r1"

type marginConfig struct {
	Schema   string          `json:"schema"`
	Run      kit.RunIdentity `json:"run"`
	Source   string          `json:"source_sha256"`
	Compiler *fileDigest     `json:"compiler"`
}

type marginMetric struct {
	ID       string `json:"id"`
	Config   string `json:"config_sha256"`
	Used     int64  `json:"used_ms"`
	Limit    int64  `json:"limit_ms"`
	Samples  int    `json:"samples"`
	State    string `json:"state"`
	Exceeded bool   `json:"exceeded"`
}

type marginReport struct {
	Schema   string          `json:"schema"`
	Run      kit.RunIdentity `json:"run"`
	CoreSHA  string          `json:"core_result_sha256"`
	Cutoff   string          `json:"history_cutoff"`
	Configs  []marginConfig  `json:"configurations"`
	Metrics  []marginMetric  `json:"metrics"`
	Excluded []string        `json:"excluded_history"`
}

type marginJob struct {
	name, job, goos, arch string
	minutes               int64
	compiler              bool
}

var marginJobs = []marginJob{
	{"foundation (windows-2025)", "foundation", "windows", "amd64", 57, true},
	{"foundation (ubuntu-24.04)", "foundation", "linux", "amd64", 57, true},
	{"foundation (macos-15)", "foundation", "darwin", "arm64", 57, true},
	{"native prepare (ubuntu-24.04)", "native-prepare", "linux", "amd64", 30, false},
	{"native routes (windows-2025)", "native-routes", "windows", "amd64", 60, true},
	{"native routes (ubuntu-24.04)", "native-routes", "linux", "amd64", 60, true},
	{"native routes (macos-15)", "native-routes-macos", "darwin", "arm64", 60, true},
	{"large API (windows-2025)", "large-api", "windows", "amd64", 60, true},
	{"qualification (ubuntu-24.04)", "qualification", "linux", "amd64", 15, false},
}

func configName(j marginJob, r kit.RunIdentity) string {
	return "ci-config-" + j.job + "-" + j.goos + "-" + r.RunID + "-" + r.RunAttempt
}

// Hash only workload/configuration inputs; documentation and run-specific paths
// are provenance, not a reason to merge or split the measured population.
func marginSource() (string, error) {
	names := []string{"go.mod", "go.sum", ".github/workflows/foundation.yml"}
	err := filepath.WalkDir("src", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			names = append(names, filepath.ToSlash(path))
		}
		if len(names) > 10000 {
			return errors.New("margin source file limit")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	slices.Sort(names)
	files := map[string]fileDigest{}
	var total int64
	for _, name := range names {
		b, err := readFile(name, 64<<20)
		if err != nil {
			return "", err
		}
		total += int64(len(b))
		if total > 128<<20 {
			return "", errors.New("margin source byte limit")
		}
		files[name] = fileDigest{digest(b), int64(len(b))}
	}
	b, err := json.Marshal(files)
	return digest(b), err
}

func saveMarginConfig(identity, compiler, out string) error {
	b, err := readFile(identity, 65536)
	var r kit.RunIdentity
	if err != nil || decode(b, &r) != nil {
		return errors.New("margin producer identity unavailable")
	}
	var selected *marginJob
	for _, j := range marginJobs {
		if r.Job == j.job && r.GOOS == j.goos && r.GOARCH == j.arch {
			selected = &j
		}
	}
	if selected == nil || r.Image == "" || strings.HasPrefix(r.Image, "/") || strings.HasSuffix(r.Image, "/") {
		return errors.New("margin producer configuration unavailable")
	}
	source, err := marginSource()
	if err != nil {
		return err
	}
	c := marginConfig{Schema: "tsgk-ci-config/r1", Run: r, Source: source}
	if selected.compiler {
		b, err := readFile(compiler, 256<<20)
		if err != nil {
			return errors.New("margin compiler unavailable")
		}
		c.Compiler = &fileDigest{digest(b), int64(len(b))}
	} else if compiler != "" {
		return errors.New("unexpected margin compiler")
	}
	return writeNewJSON(out, c, 65536)
}

// marginGo is the pinned CI toolchain; a historical report may also carry the former pin, a
// separate configuration by its key (#261).
const marginGo, marginFormerGo = "go1.27.2", "go1.27.1"

func marginToolchain(v string, historical bool) bool {
	return v == marginGo || historical && v == marginFormerGo
}

func checkMarginConfig(c marginConfig, j marginJob, expected kit.RunIdentity, historical bool) error {
	r := c.Run
	if c.Schema != "tsgk-ci-config/r1" || !sameRun(&r, expected) || r.Job != j.job || r.GOOS != j.goos || r.GOARCH != j.arch ||
		!marginToolchain(r.GoVersion, historical) || !hex64.MatchString(c.Source) ||
		len(r.Image) > 128 || r.RunnerOS != map[string]string{"windows": "Windows", "linux": "Linux", "darwin": "macOS"}[j.goos] ||
		r.RunnerArch != map[string]string{"amd64": "X64", "arm64": "ARM64"}[j.arch] ||
		!strings.Contains(r.Image, "/") || strings.HasPrefix(r.Image, "/") || strings.HasSuffix(r.Image, "/") ||
		(j.compiler && (c.Compiler == nil || !hex64.MatchString(c.Compiler.SHA256) || c.Compiler.Bytes <= 0 || c.Compiler.Bytes > 256<<20)) ||
		(!j.compiler && c.Compiler != nil) {
		return errors.New("margin configuration mismatch")
	}
	return nil
}

func decodeMarginConfig(b []byte, c *marginConfig) error {
	var object map[string]json.RawMessage
	if requiredFields(b, "schema", "run", "source_sha256") != nil || decode(b, &object) != nil || decode(b, c) != nil {
		return errors.New("margin configuration fields missing")
	}
	if _, ok := object["compiler"]; !ok {
		return errors.New("margin compiler declaration missing")
	}
	return nil
}

func decodeMarginReport(b []byte, r *marginReport) error {
	var raw struct {
		Configs []json.RawMessage `json:"configurations"`
		Metrics []json.RawMessage `json:"metrics"`
	}
	if requiredFields(b, "schema", "run", "core_result_sha256", "history_cutoff", "configurations", "metrics") != nil ||
		decode(b, &raw) != nil || decode(b, r) != nil {
		return errors.New("margin report required fields")
	}
	for _, b := range raw.Configs {
		var c marginConfig
		if err := decodeMarginConfig(b, &c); err != nil {
			return err
		}
	}
	for _, m := range raw.Metrics {
		if requiredFields(m, "id", "config_sha256", "used_ms", "limit_ms") != nil {
			return errors.New("margin metric required fields")
		}
	}
	return nil
}

func marginKey(c marginConfig, id string, limit int64) string {
	// Keep the exact image, compiler, toolchain and workload. Run/head/attempt and
	// temporary compiler/build paths must not manufacture distinct configurations.
	b, _ := json.Marshal([]any{marginSchema, c.Source, c.Run.Job, c.Run.GOOS, c.Run.GOARCH,
		c.Run.RunnerOS, c.Run.RunnerArch, c.Run.Image, c.Run.GoVersion, c.Compiler, id, limit})
	return digest(b)
}

type marginStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Start      string `json:"started_at"`
	End        string `json:"completed_at"`
}
type marginAPIJob struct {
	ID      uint64 `json:"id"`
	RunID   uint64 `json:"run_id"`
	Attempt uint64 `json:"run_attempt"`
	Head    string `json:"head_sha"`
	marginStep
	Steps []marginStep `json:"steps"`
}

// marginJobsSettled reports whether the jobs API already shows every measured job and timed
// step as completed. The API can trail the finished jobs this gate needs, so the collector
// fetches a new snapshot instead of relaxing currentMargins.
func marginJobsSettled(jobsData []byte) bool {
	var jobs struct {
		Jobs []marginAPIJob `json:"jobs"`
	}
	if decode(jobsData, &jobs) != nil {
		return false
	}
	settled := func(s marginStep) bool { return s.Status == "completed" && s.Start != "" && s.End != "" }
	for _, j := range marginJobs {
		found := false
		for _, job := range jobs.Jobs {
			if job.Name != j.name {
				continue
			}
			found = true
			if !settled(job.marginStep) {
				return false
			}
			for name := range marginStepLimits(j) {
				if !slices.ContainsFunc(job.Steps, func(s marginStep) bool { return s.Name == name && settled(s) }) {
					return false
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func elapsed(s marginStep) (int64, error) {
	start, e1 := time.Parse(time.RFC3339, s.Start)
	end, e2 := time.Parse(time.RFC3339, s.End)
	if e1 != nil || e2 != nil || s.Status != "completed" || s.Conclusion != "success" || end.Before(start) {
		return 0, errors.New("margin observation incomplete or invalid")
	}
	ms := end.Sub(start).Milliseconds()
	if ms > 24*time.Hour.Milliseconds() {
		return 0, errors.New("margin timestamp range")
	}
	return ms, nil
}

func marginStepLimits(j marginJob) map[string]int64 {
	limits := map[string]int64{}
	if j.job == "foundation" {
		limits["frame 상태와 native cancellation 반복 시험 (#65)"] = 6
		if j.goos == "linux" {
			limits["isolated bind-mount output boundary (#196)"] = 2
			limits["S04 runner tree 정리 반복 시험 (Linux cgroup v2)"] = 4
		}
	}
	if j.job == "native-routes" && j.goos == "linux" {
		limits["S05 driver sanitizer (Linux, ASan/UBSan)"] = 8
	}
	if j.job == "large-api" {
		limits["Windows 대용량 전체 API audit"] = 45
	}
	return limits
}

func currentMargins(expected kit.RunIdentity, configs []marginConfig, jobsData, core []byte) ([]marginMetric, error) {
	var jobs struct {
		Total int            `json:"total_count"`
		Jobs  []marginAPIJob `json:"jobs"`
	}
	var gate struct {
		Assessment string          `json:"assessment"`
		Run        kit.RunIdentity `json:"run"`
		Large      struct {
			Wall int64 `json:"max_page_wall_ms"`
		} `json:"large"`
	}
	var rawCore struct {
		Large json.RawMessage `json:"large"`
	}
	if decode(core, &rawCore) != nil || requiredFields(rawCore.Large, "max_page_wall_ms") != nil ||
		decode(jobsData, &jobs) != nil || jobs.Total != len(jobs.Jobs) || jobs.Total < 11 || jobs.Total > 14 ||
		decode(core, &gate) != nil || gate.Assessment != "PASS" || !sameRun(&gate.Run, expected) ||
		len(configs) != len(marginJobs) {
		return nil, errors.New("margin current evidence invalid")
	}
	var metrics []marginMetric
	for i, j := range marginJobs {
		c := configs[i]
		if checkMarginConfig(c, j, expected, false) != nil {
			return nil, errors.New("margin current configuration invalid")
		}
		var found []marginAPIJob
		for _, job := range jobs.Jobs {
			if job.Name == j.name {
				found = append(found, job)
			}
		}
		if len(found) != 1 {
			return nil, errors.New("margin required job missing or duplicate")
		}
		job := found[0]
		if job.ID == 0 || strconv.FormatUint(job.RunID, 10) != expected.RunID ||
			strconv.FormatUint(job.Attempt, 10) != expected.RunAttempt || job.Head != expected.HeadSHA {
			return nil, errors.New("margin job run binding mismatch")
		}
		add := func(id string, used, limit int64) {
			metrics = append(metrics, marginMetric{ID: id, Config: marginKey(c, id, limit), Used: used, Limit: limit})
		}
		wall, err := elapsed(job.marginStep)
		if err != nil {
			return nil, err
		}
		add(j.job+"/"+j.goos+"/job", wall, j.minutes*60000)
		for name, minutes := range marginStepLimits(j) {
			var found []marginStep
			for _, step := range job.Steps {
				if step.Name == name {
					found = append(found, step)
				}
			}
			if len(found) != 1 {
				return nil, errors.New("margin timeout step missing or duplicate")
			}
			wall, err := elapsed(found[0])
			if err != nil {
				return nil, err
			}
			start, _ := time.Parse(time.RFC3339, found[0].Start)
			end, _ := time.Parse(time.RFC3339, found[0].End)
			jobStart, _ := time.Parse(time.RFC3339, job.Start)
			jobEnd, _ := time.Parse(time.RFC3339, job.End)
			if start.Before(jobStart) || end.After(jobEnd) {
				return nil, errors.New("margin step outside job")
			}
			add(j.job+"/"+j.goos+"/step/"+name, wall, minutes*60000)
		}
		if j.job == "large-api" {
			add("large-api/windows/page-max", gate.Large.Wall, kit.NativeOperations()["native-query-large"].ParseWall.Milliseconds())
		}
	}
	slices.SortFunc(metrics, func(a, b marginMetric) int { return strings.Compare(a.ID, b.ID) })
	return metrics, nil
}

func validateMarginReport(r marginReport, historical bool) error {
	if r.Schema != marginSchema || !hex64.MatchString(r.CoreSHA) || len(r.Configs) != len(marginJobs) || !marginToolchain(r.Run.GoVersion, historical) {
		return errors.New("margin report shape")
	}
	expected := map[string]string{}
	limits := map[string]int64{}
	configs := map[string]marginConfig{}
	for i, j := range marginJobs {
		c := r.Configs[i]
		// Historical r1 reports retain the former Mac matrix job and its exact keys.
		if historical && j.job == "native-routes-macos" && c.Run.Job == "native-routes" {
			j.job = "native-routes"
		}
		if checkMarginConfig(c, j, r.Run, historical) != nil || c.Source != r.Configs[0].Source {
			return errors.New("margin report configuration")
		}
		id := j.job + "/" + j.goos + "/job"
		limits[id] = j.minutes * 60000
		expected[id] = marginKey(c, id, limits[id])
		configs[id] = c
		for name, minutes := range marginStepLimits(j) {
			id := j.job + "/" + j.goos + "/step/" + name
			limits[id] = minutes * 60000
			expected[id] = marginKey(c, id, limits[id])
			configs[id] = c
		}
		if j.job == "large-api" {
			id := "large-api/windows/page-max"
			limits[id] = kit.NativeOperations()["native-query-large"].ParseWall.Milliseconds()
			expected[id] = marginKey(c, id, limits[id])
			configs[id] = c
		}
	}
	if len(r.Metrics) != len(expected) {
		return errors.New("margin metric coverage")
	}
	for _, m := range r.Metrics {
		key, ok := expected[m.ID]
		// #260: a past report keeps the limit it ran under; its key binds that limit, so it is
		// another configuration and never adds a sample to a metric with the current limit
		if ok && historical && m.Limit != limits[m.ID] && m.Limit > 0 && m.Limit <= 24*time.Hour.Milliseconds() {
			key = marginKey(configs[m.ID], m.ID, m.Limit)
			limits[m.ID] = m.Limit
		}
		if !ok || m.Config != key || m.Limit != limits[m.ID] || m.Used < 0 || m.Used > 24*time.Hour.Milliseconds() {
			return errors.New("margin invalid metric or configuration key")
		}
		delete(expected, m.ID)
	}
	return nil
}

func evaluateMargins(current *marginReport, history []marginReport) (bool, error) {
	if err := validateMarginReport(*current, false); err != nil {
		return false, err
	}
	seen := map[string]bool{current.Run.RunID: true}
	counts := map[string]int{}
	limits := map[string]int64{}
	for _, m := range current.Metrics {
		limits[m.ID] = m.Limit
	}
	for _, r := range history {
		if seen[r.Run.RunID] {
			return false, errors.New("margin duplicate run sample")
		}
		seen[r.Run.RunID] = true
		if err := validateMarginReport(r, true); err != nil {
			return false, err
		}
		for _, c := range r.Configs {
			if c.Run.Job == "native-routes" && c.Run.GOOS == "darwin" && c.Source == current.Configs[0].Source {
				return false, errors.New("legacy margin identity claims current configuration")
			}
		}
		// #260: the source binds the workflow and the limit table, so one source has one limit set
		for _, m := range r.Metrics {
			if r.Configs[0].Source == current.Configs[0].Source && m.Limit != limits[m.ID] {
				return false, errors.New("former margin limit claims current configuration")
			}
		}
		for _, m := range r.Metrics {
			counts[m.Config]++
		}
	}
	failed := false
	for i := range current.Metrics {
		m := &current.Metrics[i]
		m.Samples = counts[m.Config] + 1
		m.Exceeded = m.Used*100 > m.Limit*70
		m.State = "INSUFFICIENT_SAMPLE"
		if m.Samples >= 20 {
			m.State = "PASS"
			if m.Exceeded {
				m.State = "FAIL"
				failed = true
			}
		}
	}
	return failed, nil
}

func bindMarginHost(c marginConfig, root string, expected kit.RunIdentity) error {
	var path string
	if c.Run.Job == "native-routes" || c.Run.Job == "native-routes-macos" {
		path = filepath.Join(root, "s05-routes-"+c.Run.GOOS+"-"+expected.RunID+"-"+expected.RunAttempt, "run-identity.json")
	} else if c.Run.Job == "large-api" {
		path = filepath.Join(root, "large-api-"+expected.RunID+"-"+expected.RunAttempt, "large-api-identity-"+expected.RunID+"-"+expected.RunAttempt, "run-identity.json")
	}
	if path != "" {
		b, err := readFile(path, 65536)
		var r kit.RunIdentity
		if err != nil || decode(b, &r) != nil || r != c.Run {
			return errors.New("margin host identity differs from verified evidence")
		}
	}
	if c.Run.Job == "large-api" {
		b, err := readFile(filepath.Join(root, "large-api-"+expected.RunID+"-"+expected.RunAttempt, "large-api-"+expected.RunID+"-"+expected.RunAttempt+".json"), 16777216)
		var large struct {
			Build *native.Build `json:"base_build"`
		}
		if err != nil || decode(b, &large) != nil || large.Build == nil || c.Compiler == nil ||
			large.Build.Compiler.SHA256 != c.Compiler.SHA256 || int64(large.Build.Compiler.Bytes) != c.Compiler.Bytes {
			return errors.New("margin audit compiler differs from verified evidence")
		}
	}
	if c.Run.Job == "native-routes" || c.Run.Job == "native-routes-macos" {
		b, err := readFile(filepath.Join(root, "s05-routes-"+c.Run.GOOS+"-"+expected.RunID+"-"+expected.RunAttempt, "summary.json"), 16777216)
		var summary struct {
			Compiler fileDigest `json:"compiler"`
		}
		if err != nil || decode(b, &summary) != nil || c.Compiler == nil || summary.Compiler != *c.Compiler {
			return errors.New("margin route compiler differs from verified evidence")
		}
	}
	return nil
}

func verifyMargins(ctx context.Context, expected kit.RunIdentity, coreRoot, metadata, historyRoot, corePath, out string) error {
	plan, err := makeMarginPlan(expected, metadata, filepath.Join(historyRoot, "metadata"))
	if err != nil {
		return err
	}
	source, err := marginSource()
	if err != nil {
		return err
	}
	report := marginReport{Schema: marginSchema, Run: expected, Excluded: plan.Excluded}
	cutoff, err := readFile(filepath.Join(historyRoot, "metadata/cutoff.json"), 4096)
	var snapshot struct {
		Cutoff string `json:"cutoff"`
	}
	if err != nil || decode(cutoff, &snapshot) != nil {
		return errors.New("margin cutoff unavailable")
	}
	report.Cutoff = snapshot.Cutoff
	if err := os.Mkdir(filepath.Join(historyRoot, "unpacked"), 0700); err != nil {
		return err
	}
	for i, a := range plan.Current {
		b, err := readMarginZIP(ctx, a, filepath.Join(historyRoot, "zips"), filepath.Join(historyRoot, "unpacked"), "config.json")
		var c marginConfig
		if err != nil || decodeMarginConfig(b, &c) != nil || checkMarginConfig(c, marginJobs[i], expected, false) != nil || c.Source != source {
			return errors.New("margin producer configuration invalid")
		}
		if err := bindMarginHost(c, coreRoot, expected); err != nil {
			return err
		}
		report.Configs = append(report.Configs, c)
	}
	core, err := readFile(corePath, 65536)
	if err != nil {
		return err
	}
	jobs, err := readFile(filepath.Join(historyRoot, "metadata/jobs.json"), 1048576)
	if err != nil {
		return err
	}
	report.CoreSHA = digest(core)
	report.Metrics, err = currentMargins(expected, report.Configs, jobs, core)
	if err != nil {
		return err
	}
	var past []marginReport
	for _, h := range plan.History {
		b, err := readMarginZIP(ctx, h.Artifact, filepath.Join(historyRoot, "zips"), filepath.Join(historyRoot, "unpacked"), "margin.json")
		var r marginReport
		if err != nil || decodeMarginReport(b, &r) != nil || !sameRun(&r.Run, h.Run) || r.Run.Job != "integration-gate" {
			return errors.New("margin historical report binding invalid")
		}
		past = append(past, r)
	}
	failed, err := evaluateMargins(&report, past)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeNewJSON(out, report, 262144); err != nil {
		return err
	}
	for _, m := range report.Metrics {
		if m.Exceeded || m.State == "INSUFFICIENT_SAMPLE" {
			fmt.Printf("::warning::CI margin %s: %d/%d ms; n=%d; %s\n", m.ID, m.Used, m.Limit, m.Samples, m.State)
		}
	}
	if failed {
		return errors.New("CI margin exceeded 70 percent with at least 20 same-configuration runs")
	}
	return nil
}
