package kit

import (
	"encoding/base64"
	"reflect"
	"slices"
	"strings"
)

type eligibilityProcessRequest struct {
	Path        string   `json:"Path"`
	Args        []string `json:"Args"`
	Dir         string   `json:"Dir"`
	Env         []string `json:"Env"`
	StdoutBytes uint64   `json:"StdoutBytes"`
	StderrBytes uint64   `json:"StderrBytes"`
	Wall        uint64   `json:"Wall"`
	Grace       uint64   `json:"Grace"`
	Memory      struct {
		Bytes uint64 `json:"Bytes"`
		Hard  bool   `json:"Hard"`
	} `json:"Memory"`
	CgroupParent string `json:"CgroupParent"`
}

type eligibilityExecution struct {
	Status          string `json:"status"`
	Reason          string `json:"reason"`
	ExitCode        int    `json:"exit_code"`
	StdoutBytes     uint64 `json:"stdout_bytes"`
	StderrBytes     uint64 `json:"stderr_bytes"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	WallMillis      uint64 `json:"wall_ms"`
	Cleanup         struct {
		Method            string `json:"method"`
		Scope             string `json:"scope"`
		ResidualAfterExit uint64 `json:"residual_after_exit"`
		Verified          bool   `json:"verified"`
		Detail            string `json:"detail,omitempty"`
	} `json:"cleanup"`
	Memory struct {
		Limit       uint64 `json:"limit_bytes"`
		Enforcement string `json:"enforcement"`
		Metric      string `json:"metric"`
		Peak        uint64 `json:"peak_bytes"`
		PeakKnown   bool   `json:"peak_known"`
		LimitHit    bool   `json:"limit_hit"`
	} `json:"memory"`
	Capabilities struct {
		OS                 string `json:"os"`
		Backend            string `json:"backend"`
		TreeCleanup        string `json:"tree_cleanup"`
		EscapedDescendants string `json:"escaped_descendants"`
		Memory             string `json:"memory"`
		MemoryMetric       string `json:"memory_metric"`
		PoliteTermination  bool   `json:"polite_termination"`
	} `json:"capabilities"`
}

type eligibilityReceipt struct {
	Schema       string               `json:"schema"`
	Execution    eligibilityExecution `json:"execution"`
	StdoutBase64 string               `json:"stdout_base64"`
	StderrBase64 string               `json:"stderr_base64"`
}

type eligibilityOutput struct {
	stdout, stderr []byte
	request        eligibilityProcessRequest
	execution      eligibilityExecution
	rows           [][]byte
	offsets        []uint64
	responses      []*instrumentedReference
}

func eligibilityLiteral(x string) bool { return eligibilityString(x) && !strings.ContainsRune(x, 0) }

func (s *eligibilitySession) request(inv *eligibilityInvocation) (eligibilityProcessRequest, bool, *Error) {
	var expected, actual eligibilityProcessRequest
	if e := s.b.decode(inv.ID+"/approved-request", inv.ApprovedRequest, &expected, true); e != nil {
		return actual, false, e
	}
	data, ok, e := s.member(inv.Request)
	if e != nil {
		return actual, false, e
	}
	if !ok {
		return actual, false, nil
	}
	if e := s.b.decode(inv.ID+"/request", data, &actual, true); e != nil {
		return actual, false, e
	}
	if e := s.b.charge(50, uint64(len(data)+len(inv.ApprovedRequest)), inv.ID); e != nil {
		return actual, false, e
	}
	if !reflect.DeepEqual(expected, actual) {
		s.finding("ELIGIBILITY_REQUEST_MISMATCH", inv.ID, true)
		return actual, false, nil
	}
	if e := eligibilityValidateRequest(actual, inv.ID); e != nil {
		return actual, false, e
	}
	return actual, true, nil
}

func eligibilityValidateRequest(request eligibilityProcessRequest, id string) *Error {
	if !eligibilityLiteral(request.Path) || !eligibilityLiteral(request.Dir) || request.StdoutBytes == 0 || request.StderrBytes == 0 || request.Wall == 0 || request.Grace == 0 || request.Memory.Bytes == 0 || request.StdoutBytes > maxJSONInteger || request.StderrBytes > maxJSONInteger || request.Wall > maxJSONInteger || request.Grace > maxJSONInteger || request.Memory.Bytes > maxJSONInteger {
		return fail(KindInvalidInput, "ELIGIBILITY_REQUEST_INVALID", id, nil)
	}
	for _, arg := range request.Args {
		if !eligibilityLiteral(arg) {
			return fail(KindInvalidInput, "ELIGIBILITY_ARGV_INVALID", id, nil)
		}
	}
	if !slices.IsSorted(request.Env) {
		return fail(KindInvalidInput, "ELIGIBILITY_ENV_INVALID", id, nil)
	}
	names := map[string]bool{}
	for _, env := range request.Env {
		name, value, ok := strings.Cut(env, "=")
		if !ok || name == "" || names[name] || !eligibilityLiteral(env) || strings.ContainsAny(name, "=\x00") {
			return fail(KindInvalidInput, "ELIGIBILITY_ENV_INVALID", id, nil)
		}
		_ = value
		names[name] = true
	}
	return nil
}

func (s *eligibilitySession) receipt(inv *eligibilityInvocation, request eligibilityProcessRequest, expectedExit int, truncated bool) (*eligibilityOutput, bool, *Error) {
	data, ok, e := s.member(inv.Receipt)
	if e != nil {
		return nil, false, e
	}
	if !ok {
		return nil, false, nil
	}
	var receipt eligibilityReceipt
	if e := s.b.decode(inv.ID+"/receipt", data, &receipt, true); e != nil {
		return nil, false, e
	}
	if receipt.Schema != "tsgk-reference-process/r1" {
		s.finding("ELIGIBILITY_RECEIPT_SCHEMA", inv.ID, true)
		return nil, false, nil
	}
	out := &eligibilityOutput{request: request, execution: receipt.Execution}
	for i, text := range []string{receipt.StdoutBase64, receipt.StderrBase64} {
		if e := s.b.charge(1, 4*uint64(len(text)), inv.ID+"/base64"); e != nil {
			return nil, false, e
		}
		raw, err := base64.StdEncoding.Strict().DecodeString(text)
		count, cut, limit := receipt.Execution.StdoutBytes, receipt.Execution.StdoutTruncated, request.StdoutBytes
		if i == 1 {
			count, cut, limit = receipt.Execution.StderrBytes, receipt.Execution.StderrTruncated, request.StderrBytes
		}
		if cut && uint64(len(raw)) != limit {
			s.finding("ELIGIBILITY_TRUNCATED_PREFIX", inv.ID, true)
			return nil, false, nil
		}
		if err != nil || base64.StdEncoding.EncodeToString(raw) != text || uint64(len(raw)) > limit || count > maxJSONInteger || count < uint64(len(raw)) || !cut && count != uint64(len(raw)) {
			s.finding("ELIGIBILITY_STREAM_INVALID", inv.ID, true)
			return nil, false, nil
		}
		if i == 0 {
			out.stdout = raw
		} else {
			out.stderr = raw
		}
	}
	ready := s.execution(inv.ID, receipt.Execution, request, s.cohorts[inv.CohortID].Platform, expectedExit, truncated)
	if inv.ExitAttestation == nil || inv.ExitAttestation.Value != expectedExit {
		s.finding("ELIGIBILITY_EXIT_ATTESTATION", inv.ID, true)
		ready = false
	}
	return out, ready, nil
}

func (s *eligibilitySession) execution(id string, x eligibilityExecution, request eligibilityProcessRequest, platform string, exit int, truncated bool) bool {
	good := true
	if x.Capabilities.OS != platform || !x.Cleanup.Verified || x.Cleanup.ResidualAfterExit != 0 || x.Memory.Limit != request.Memory.Bytes || x.StderrTruncated || x.Memory.LimitHit || x.Memory.PeakKnown && x.Memory.Peak > x.Memory.Limit {
		s.finding("ELIGIBILITY_EXECUTION_BINDING", id, true)
		good = false
	}
	if truncated {
		if request.StdoutBytes != 1024 || x.Status != StatusResourceLimit || x.Reason != "OUTPUT_LIMIT" || !x.StdoutTruncated {
			s.finding("ELIGIBILITY_TRUNCATION_CONTROL", id, true)
			good = false
		}
	} else {
		want := StatusCompleted
		if exit != 0 {
			want = StatusFailed
		}
		if x.Status != want || x.Reason != "EXITED" || x.ExitCode != exit || x.StdoutTruncated {
			s.finding("ELIGIBILITY_EXECUTION_INCOMPLETE", id, true)
			good = false
		}
	}
	backend, scope := "linux-cgroup-v2", "CGROUP_KILL"
	if platform == "windows/amd64" {
		backend, scope = "windows-job-object", "JOB_OBJECT"
	}
	if platform == "darwin/arm64" || !request.Memory.Hard || x.Memory.Enforcement != "HARD" || x.Capabilities.Memory != "HARD" {
		s.finding("ELIGIBILITY_HARD_MEMORY_UNAVAILABLE", id, false)
		good = false
	} else if x.Capabilities.Backend != backend || x.Capabilities.TreeCleanup != scope || x.Cleanup.Scope != scope || x.Cleanup.Method != scope || x.Capabilities.EscapedDescendants != "CONTAINED" || x.Capabilities.MemoryMetric != x.Memory.Metric || !eligibilityString(x.Memory.Metric) {
		s.finding("ELIGIBILITY_BACKEND_MISMATCH", id, true)
		good = false
	}
	return good
}

func (s *eligibilitySession) expand(value string, paths map[string]string, platform string) (string, bool) {
	sep := "/"
	if platform == "windows/amd64" {
		sep = "\\"
	}
	for _, key := range []string{"B", "C", "H", "S", "T", "R", "G", "L", "LH"} {
		literal, ok := paths[key]
		if strings.Contains(value, "{"+key+"}") && !ok {
			return "", false
		}
		value = strings.ReplaceAll(value, "{"+key+"}/", literal+sep)
		value = strings.ReplaceAll(value, "{"+key+"}", literal)
	}
	shared := "-shared"
	if platform == "darwin/arm64" {
		shared = "-dynamiclib"
	}
	return strings.ReplaceAll(value, "{SHARED}", shared), true
}

func (s *eligibilitySession) fixedRequest(inv *eligibilityInvocation, req eligibilityProcessRequest, rule eligibilityBuildRule, paths map[string]string) bool {
	c := s.cohorts[inv.CohortID]
	good := true
	dir, ok := s.expand(rule.Dir, paths, c.Platform)
	good = good && ok
	args := make([]string, len(rule.Args))
	for i, arg := range rule.Args {
		var found bool
		args[i], found = s.expand(arg, paths, c.Platform)
		good = good && found
	}
	if !good {
		s.finding("ELIGIBILITY_PATH_BINDING_MISSING", inv.ID, false)
		return false
	}
	if e := s.b.charge(uint64(len(args)+len(req.Args)+len(req.Env)+1), uint64(len(inv.ApprovedRequest))*2, inv.ID+"/request-compare"); e != nil {
		return false
	}
	if req.Dir != dir || !slices.Equal(req.Args, args) || req.Wall != rule.WallSeconds*1000000000 || req.Grace != rule.GraceSeconds*1000000000 || req.StdoutBytes != rule.StdoutBytes || req.StderrBytes != rule.StderrBytes || req.Memory.Bytes != rule.MemoryBytes {
		s.finding("ELIGIBILITY_CATALOGUE_REQUEST", inv.ID, true)
		return false
	}
	return true
}

func eligibilityJoin(base, leaf, platform string) string {
	sep := "/"
	if platform == "windows/amd64" {
		sep = "\\"
	}
	return base + sep + strings.ReplaceAll(leaf, "/", sep)
}

func eligibilityParent(path, platform string) string {
	sep := "/"
	if platform == "windows/amd64" {
		sep = "\\"
	}
	i := strings.LastIndex(path, sep)
	if i < 0 {
		return ""
	}
	return path[:i]
}

func (s *eligibilitySession) groupPaths(paths map[string]string, platform, purpose string) bool {
	keys := []string{"B", "H", "S", "T", "R"}
	if purpose == "control" {
		keys = []string{"C", "G", "L", "LH", "R"}
	}
	good := len(paths) == len(keys)
	for _, key := range keys {
		good = good && eligibilityLiteral(paths[key])
	}
	if purpose == "build" {
		ext := ""
		if platform == "windows/amd64" {
			ext = ".exe"
		}
		good = good && paths["S"] == eligibilityJoin(paths["B"], "source", platform) && paths["R"] == eligibilityJoin(paths["B"], "target/"+paths["T"]+"/release/tsgk-query-reference"+ext, platform)
	} else {
		library := "plain.so"
		if platform == "windows/amd64" {
			library = "plain.dll"
		}
		if platform == "darwin/arm64" {
			library = "plain.dylib"
		}
		good = good && paths["G"] == eligibilityJoin(paths["C"], "grammar", platform) && paths["L"] == eligibilityJoin(paths["C"], library, platform) && hex64.MatchString(paths["LH"])
	}
	if !good {
		s.finding("ELIGIBILITY_GROUP_PATHS", purpose, true)
	}
	return good
}

func (s *eligibilitySession) processEnv(inv *eligibilityInvocation, req eligibilityProcessRequest, paths, toolPaths map[string]string, build bool) bool {
	if e := s.b.charge(4*uint64(len(req.Env)+1)+100, 2*uint64(len(inv.ApprovedRequest)), inv.ID+"/env"); e != nil {
		return false
	}
	c := s.cohorts[inv.CohortID]
	platform := c.Platform
	env := map[string]string{}
	for _, item := range req.Env {
		k, v, _ := strings.Cut(item, "=")
		env[k] = v
	}
	root := paths["C"]
	if build {
		root = paths["B"]
	}
	expected := map[string]string{"TEMP": eligibilityJoin(root, "tmp", platform), "TMP": eligibilityJoin(root, "tmp", platform), "TMPDIR": eligibilityJoin(root, "tmp", platform)}
	system, sep := "/usr/bin:/bin", ":"
	if platform == "windows/amd64" {
		system, sep = "C:/Windows/System32;C:/Windows", ";"
	}
	expected["PATH"] = eligibilityParent(toolPaths["cc"], platform) + sep + system
	if build {
		expected["PATH"] = eligibilityParent(toolPaths["cc"], platform) + sep + eligibilityParent(toolPaths["rustc"], platform) + sep + system
		expected["CARGO_HOME"] = eligibilityJoin(root, "cargo-home", platform)
		expected["CARGO_TARGET_DIR"] = eligibilityJoin(root, "target", platform)
		expected["CARGO_NET_OFFLINE"], expected["CARGO_TERM_COLOR"], expected["CARGO_INCREMENTAL"] = "true", "never", "0"
		expected["RUSTC"] = toolPaths["rustc"]
		expected["GIT_CONFIG_NOSYSTEM"] = "1"
		expected["GIT_CONFIG_GLOBAL"] = "/dev/null"
		if platform == "windows/amd64" {
			expected["GIT_CONFIG_GLOBAL"] = "nul"
		}
		expected["GIT_CEILING_DIRECTORIES"] = root
		target := strings.ReplaceAll(paths["T"], "-", "_")
		expected["CC_"+target] = toolPaths["cc"]
		expected["AR_"+target] = toolPaths["ar"]
		expected["CARGO_TARGET_"+strings.ToUpper(target)+"_LINKER"] = toolPaths["cc"]
	}
	allowed := map[string]bool{}
	if platform == "windows/amd64" {
		for _, key := range []string{"SystemRoot", "WINDIR", "COMSPEC", "PATHEXT"} {
			allowed[key] = true
		}
		if build {
			allowed["SystemDrive"] = true
		}
	}
	good := true
	for key, value := range expected {
		if env[key] != value {
			good = false
		}
	}
	for key := range env {
		if _, known := expected[key]; !known && !allowed[key] {
			good = false
		}
	}
	if !good {
		s.finding("ELIGIBILITY_CONTROLLER_ENV", inv.ID, true)
	}
	return good
}
