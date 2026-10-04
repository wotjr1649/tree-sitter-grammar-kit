package kit

import (
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
)

// qualifier is the state of one qualification: the shared run and the opened hosts.
type qualifier struct {
	r         *run
	lim       ReplayLimits
	inv       QualificationInventory
	candidate string
	hosts     map[string]*qhost
	findings  []Finding
}

func (q *qualifier) finding(code, path, msg string) {
	if len(q.findings) < 1000 {
		q.findings = append(q.findings, Finding{Code: code, Severity: "error", Path: path, Message: msg})
	}
}

func (q *qualifier) close() {
	for _, h := range q.hosts {
		h.x.g.root.Close()
	}
}

// qhost is one host directory: its walked files, its run identity and the files the
// workloads used; a rejected host contributes no cell.
type qhost struct {
	platform QualPlatform
	x        *replayEnv
	files    map[string]fs.FileInfo
	used     map[string]bool
	id       *RunIdentity
	idSHA    string
	rejected []Finding
	unused   int
	records  uint64
}

func (h *qhost) reject(code, path, msg string) {
	h.rejected = append(h.rejected, Finding{Code: code, Severity: "error", Path: h.platform.ID + "/" + path, Message: msg})
}

// openHost walks a host directory under the per-host limits and checks its run identity:
// the registered OS/architecture, a NEW_RUN of the candidate checkout.
func (q *qualifier) openHost(p QualPlatform, dir string) (*qhost, *Error) {
	root, _, e := openRoot(dir)
	if e != nil {
		return nil, e
	}
	// each host has its own byte budget; the wall and cancellation are shared
	hr := &run{caller: q.r.caller, wall: q.r.wall, cancel: q.r.cancel}
	// the records are new CI evidence: they are judged with the oracle-set-r2 S05 route rule
	x := &replayEnv{r: hr, g: newGuard(root), lim: q.lim, files: map[string]fs.FileInfo{}, members: map[string]ReplayMember{}, verified: map[string]bool{},
		consumed: map[string]bool{}, seen: map[string]map[string]bool{}, covered: map[string]bool{}, actual: map[string]string{}, errorTreeRoute: true}
	h := &qhost{platform: p, x: x, files: x.files, used: map[string]bool{}}
	var count uint64
	if e := x.walk(".", &count); e != nil {
		root.Close()
		return nil, e
	}
	const idPath = "run-identity.json"
	h.used[idPath] = true
	if f := x.files["summary.json"]; f != nil {
		h.used["summary.json"] = true // the helper's own summary: retained, not evidence
	}
	data, e := h.readWalked(idPath)
	if e != nil {
		if e.Kind == KindCancelled || e.Code == "TOTAL_BYTES_LIMIT" || e.Code == "WALL_LIMIT" {
			return nil, e
		}
		h.reject("RUN_IDENTITY_MISSING", idPath, "host 실행 identity가 없거나 읽지 못했다")
		return h, nil
	}
	var id RunIdentity
	if e := strictDoc(idPath, data, RunIdentitySchema); e != nil {
		h.reject("RUN_IDENTITY_INVALID", idPath, "host 실행 identity 형식이 틀렸다")
		return h, nil
	}
	if e := decodeTyped(idPath, data, &id); e != nil {
		h.reject("RUN_IDENTITY_INVALID", idPath, "host 실행 identity 형식이 틀렸다")
		return h, nil
	}
	h.id, h.idSHA = &id, digestHex(data)
	switch {
	case id.GOOS != p.GOOS || id.GOARCH != p.GOARCH:
		h.reject("PLATFORM_MISMATCH", idPath, fmt.Sprintf("host가 %s/%s에서 실행됐다(칸은 %s)", id.GOOS, id.GOARCH, p.pair()))
	case id.EvidenceMode != ModeNewRun:
		h.reject("ELIGIBILITY_REJECTED", idPath, "최종 qualification은 NEW_RUN만 받는다(replay·승계·기록은 자격이 없다)")
	case id.Checkout != q.candidate || id.SHA != q.candidate:
		h.reject("ELIGIBILITY_REJECTED", idPath, "다른 commit의 실행은 현재 후보의 칸을 채우지 못한다")
	case id.Repository == "" || id.Workflow == "" || id.RunID == "" || id.RunAttempt == "" || id.Event == "":
		h.reject("RUN_IDENTITY_INVALID", idPath, "run·attempt·event identity가 비었다")
	}
	return h, nil
}

// readWalked reads one walked file under the host limits without a trusted hash.
func (h *qhost) readWalked(path string) ([]byte, *Error) {
	info := h.files[path]
	if info == nil {
		return nil, fail(KindInvalidInput, "MEMBER_MISSING", path, nil)
	}
	if uint64(info.Size()) > h.x.lim.FileBytes {
		return nil, fail(KindResourceLimit, "FILE_BYTES_LIMIT", path, nil)
	}
	st, e := h.x.g.readFile(h.x.r, path, info, h.x.lim.FileBytes, h.x.lim.TotalBytes, uint64(info.Size())+1)
	if e != nil {
		return nil, e
	}
	h.used[path] = true
	return st.content, nil
}

// qset is a set evaluation with the per-case kind results and comparison summaries.
type qset struct {
	QualSet
	kinds     map[string]map[string]string // case → kind → PASS | FAIL | BLOCKED
	checks    map[string]string            // case → registered-check result
	summaries map[string]string            // case → canonical semantic summary
	order     []string
}

// qRecord is the part of a record the qualification reads beyond the S07 case view.
type qRecord struct {
	Process *struct {
		WallMS int64 `json:"wall_ms"`
		Memory struct {
			PeakBytes uint64 `json:"peak_bytes"`
		} `json:"memory"`
	} `json:"process"`
	Steps []struct {
		Incremental *qTree `json:"incremental"`
		Fresh       *qTree `json:"fresh"`
		Route       *struct {
			Proven bool `json:"proven"`
		} `json:"route"`
		Comparison *struct {
			Equal bool            `json:"equal"`
			First *TreeDifference `json:"first_difference"`
		} `json:"comparison"`
		QueryCompare *struct {
			Equal bool `json:"equal"`
		} `json:"query_comparison"`
		Composite jsontext.Value `json:"composite"`
	} `json:"steps"`
	Expectations []rcExpect `json:"expectations"`
}

type qTree struct {
	Status          string         `json:"status"`
	Code            string         `json:"code"`
	Form            string         `json:"form"`
	DescendantCount uint64         `json:"descendant_count"`
	HasError        bool           `json:"has_error"`
	Digest          string         `json:"digest"`
	ParseMS         int64          `json:"parse_ms"`
	API             jsontext.Value `json:"api"`
	Queries         []qQuery       `json:"queries"`
}

type qQuery struct {
	ID         string           `json:"id"`
	Status     string           `json:"status"`
	Code       string           `json:"code"`
	Evaluation string           `json:"evaluation"`
	Captures   []Capture        `json:"captures"`
	Error      *json.RawMessage `json:"error"`
}

// compositeSemantic is an SVC composite as platforms must agree on it: every field except
// the producer identities, which name the platform's native build (a host observation kept
// as build_identity and bound to the run on each host by checkSeen).
func compositeSemantic(raw jsontext.Value) any {
	if raw == nil {
		return nil
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw) // compared as recorded; an undecodable composite is not projected
	}
	var drop func(v any)
	drop = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, e := range t {
				if ids, ok := e.([]any); ok && k == "identities" {
					kept := []any{}
					for _, id := range ids {
						if m, ok := id.(map[string]any); !ok || m["role"] != "producer" {
							kept = append(kept, id)
						}
					}
					t[k] = kept
					continue
				}
				drop(e)
			}
		case []any:
			for _, e := range t {
				drop(e)
			}
		}
	}
	drop(v)
	return v
}

// semantic is the projection two platforms must agree on: tree identity and shape, query
// streams, API observations, comparisons and judgements; host timing, memory and paths are
// not part of it.
func (t *qTree) semantic() any {
	if t == nil {
		return nil
	}
	type qs struct {
		ID, Status, Code, Evaluation, Captures string
		Error                                  *json.RawMessage
	}
	var queries []qs
	for _, q := range t.Queries {
		caps, _ := json.Marshal(q.Captures)
		queries = append(queries, qs{q.ID, q.Status, q.Code, q.Evaluation, digestHex(caps), q.Error})
	}
	return struct {
		Status, Code, Form string
		Count              uint64
		HasError           bool
		Digest             string
		API                jsontext.Value
		Queries            []qs
	}{t.Status, t.Code, t.Form, t.DescendantCount, t.HasError, t.Digest, t.API, queries}
}

// workload evaluates one workload on every registered platform and compares the hosts.
func (q *qualifier) workload(w QualWorkload, errNodes []string, platforms []QualPlatform) (map[string]*qset, *QualComparison, *Error) {
	sets := map[string]*qset{}
	cmp := &QualComparison{Set: w.Set, Platforms: []string{}, Differences: []string{}}
	for _, p := range platforms {
		s, e := q.evalSet(w, errNodes, p)
		if e != nil {
			return nil, nil, e
		}
		sets[p.ID] = s
		if s.Evidence == ModeNewRun {
			cmp.Platforms = append(cmp.Platforms, p.ID)
		}
	}
	q.compare(w, sets, cmp)
	return sets, cmp, nil
}

// compare checks that every judged host produced the same semantic summary per case.
func (q *qualifier) compare(w QualWorkload, sets map[string]*qset, cmp *QualComparison) {
	if len(cmp.Platforms) < 2 {
		cmp.Result = AssessNotAssessed
		return
	}
	cmp.Result = AssessPass
	base := sets[cmp.Platforms[0]]
	for _, pid := range cmp.Platforms[1:] {
		s := sets[pid]
		for _, k := range []string{"runtime", "protocol", "comparators", "policy", "queries", "fact_pack", "workload-route", "operation"} {
			if base.Identities[k] != s.Identities[k] {
				cmp.Result = AssessFail
				cmp.add(fmt.Sprintf("identity %s: %s %q vs %s %q", k, cmp.Platforms[0], base.Identities[k], pid, s.Identities[k]))
			}
		}
		for _, c := range w.Cases {
			a, b := base.summaries[c.ID], s.summaries[c.ID]
			if a == b {
				continue
			}
			cmp.Result = AssessFail
			cmp.add(fmt.Sprintf("case %s: %s vs %s: %s", c.ID, cmp.Platforms[0], pid, firstDiff(a, b)))
		}
	}
}

func (c *QualComparison) add(s string) {
	if len(c.Differences) < 20 {
		c.Differences = append(c.Differences, s)
	}
}

// firstDiff names the first differing top-level part of two summaries.
func firstDiff(a, b string) string {
	var x, y map[string]jsontext.Value
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return "summary missing on one platform"
	}
	for _, k := range sortedKeys(x) {
		if string(x[k]) != string(y[k]) {
			if k == "steps" {
				var sx, sy []jsontext.Value
				json.Unmarshal(x[k], &sx)
				json.Unmarshal(y[k], &sy)
				for i := range min(len(sx), len(sy)) {
					if string(sx[i]) != string(sy[i]) {
						return fmt.Sprintf("step %d: %s", i, partDiff(sx[i], sy[i]))
					}
				}
				return fmt.Sprintf("step count %d vs %d", len(sx), len(sy))
			}
			return k
		}
	}
	return "differs"
}

func partDiff(a, b jsontext.Value) string {
	var x, y map[string]jsontext.Value
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return "differs"
	}
	for _, k := range sortedKeys(x) {
		if string(x[k]) != string(y[k]) {
			if k == "incremental" || k == "fresh" {
				var tx, ty map[string]jsontext.Value
				json.Unmarshal(x[k], &tx)
				json.Unmarshal(y[k], &ty)
				for _, f := range sortedKeys(tx) {
					if string(tx[f]) != string(ty[f]) {
						return k + "." + f
					}
				}
			}
			return k
		}
	}
	return "differs"
}

// role evaluates one extra-role row on every platform.
func (q *qualifier) role(x QualRole, platforms []QualPlatform) ([]QualRoleRow, []QualComparison, *Error) {
	var rows []QualRoleRow
	var cmps []QualComparison
	byPlat := map[string][]*qset{}
	if x.Status == RoleExecuted {
		for _, w := range x.Workloads {
			var ps []QualPlatform
			for _, p := range platforms {
				if slices.Contains(x.Platforms, p.ID) {
					ps = append(ps, p)
				}
			}
			sets, cmp, e := q.workload(w, nil, ps)
			if e != nil {
				return nil, nil, e
			}
			cmps = append(cmps, *cmp)
			for pid, s := range sets {
				byPlat[pid] = append(byPlat[pid], s)
			}
			for _, s := range sets {
				if cmp.Result == AssessFail {
					s.Mechanism = AssessFail
					s.Findings = append(s.Findings, Finding{Code: "COMPARISON_FAILED", Severity: "error", Path: w.Set, Message: "세 platform의 의미 결과가 다르다"})
				}
			}
		}
	}
	for _, p := range platforms {
		row := QualRoleRow{ID: x.ID, Role: x.Role, Platform: p.ID, Reason: x.Reason, Sets: []QualSet{}}
		switch {
		case x.NotApplicable[p.ID] != "":
			row.Status, row.Reason = RoleNotApplicable, x.NotApplicable[p.ID]
		case !slices.Contains(x.Platforms, p.ID) && x.Status == RoleExecuted:
			continue // the role does not name this platform
		case x.Status != RoleExecuted:
			row.Status = x.Status
		default:
			row.Status, row.Mechanism, row.Checks = CellPass, AssessPass, claimPass
			missing := false
			for _, w := range x.Workloads {
				for _, s := range byPlat[p.ID] {
					if s.Set != w.Set {
						continue
					}
					row.Sets = append(row.Sets, s.QualSet)
					if s.Evidence == "MISSING" {
						missing = true
						continue
					}
					row.Mechanism = worseMechanism(row.Mechanism, s.Mechanism)
					v, _ := foldChecks(w.Cases, s)
					for _, d := range sortedKeys(s.Detectors) {
						v = worseClaim(v, s.Detectors[d])
					}
					row.Checks = worseClaim(row.Checks, v)
				}
			}
			switch {
			case missing:
				row.Status, row.Mechanism, row.Checks = CellMissing, AssessNotAssessed, ""
			case row.Mechanism == AssessFail || row.Checks == claimFail:
				row.Status = CellFail
			case row.Mechanism != AssessPass || row.Checks != claimPass:
				row.Status = CellIncomplete
			}
		}
		rows = append(rows, row)
	}
	return rows, cmps, nil
}

// worseMechanism orders kit-axis values: FAIL > BLOCKED/NOT_ASSESSED > PASS.
func worseMechanism(a, b string) string {
	rank := map[string]int{AssessPass: 0, AssessBlocked: 1, AssessNotAssessed: 1, AssessFail: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// evalSet verifies and judges one workload's record set on one host.
func (q *qualifier) evalSet(w QualWorkload, errNodes []string, p QualPlatform) (*qset, *Error) {
	s := &qset{QualSet: QualSet{Set: w.Set, Platform: p.ID, Evidence: "MISSING", Mechanism: AssessNotAssessed, Gates: []ReplayGate{}, Detectors: map[string]string{},
		Host: map[string]string{}, Identities: map[string]string{}, Findings: []Finding{}}, kinds: map[string]map[string]string{}, checks: map[string]string{}, summaries: map[string]string{}}
	h := q.hosts[p.ID]
	if h == nil {
		return s, nil
	}
	dir := "records/" + w.Set + "/"
	manPath := dir + "manifest.json"
	if h.files[manPath] == nil {
		return s, nil
	}
	markUsed(h, dir, "profiles/"+w.Profile+".json")
	if h.id == nil || len(h.rejected) > 0 {
		s.Evidence = "REJECTED"
		s.Mechanism = AssessFail
		s.Findings = append(s.Findings, h.rejected...)
		if h.id == nil {
			s.Findings = append(s.Findings, Finding{Code: "RUN_IDENTITY_MISSING", Severity: "error", Path: p.ID, Message: "host 실행 identity가 없어 칸을 채울 수 없다"})
		}
		markUsed(h, dir, "profiles/"+w.Profile+".json")
		return s, nil
	}
	s.Evidence = ModeNewRun
	add := func(code, path, msg string) {
		if len(s.Findings) < 200 {
			s.Findings = append(s.Findings, Finding{Code: code, Severity: "error", Path: p.ID + "/" + path, Message: msg})
		}
	}
	x := h.x
	x.gates, x.findings, x.seen = nil, nil, map[string]map[string]bool{}
	over := false
	done := func() (*qset, *Error) {
		for _, g := range x.gates {
			g.finish()
			s.Gates = append(s.Gates, *g)
		}
		for _, f := range x.findings {
			add(f.Code, f.Path, f.Message)
		}
		s.Mechanism = AssessPass
		for _, g := range s.Gates {
			if g.Failed > 0 {
				s.Mechanism = AssessFail
			}
		}
		if len(s.Findings) > 0 {
			s.Mechanism = AssessFail
		}
		if over && s.Mechanism == AssessPass {
			s.Mechanism = AssessBlocked // a member beyond the limits is not verified: never a pass
		}
		return s, nil
	}
	stop := func(e *Error) bool {
		return e != nil && (e.Kind == KindCancelled || e.Code == "WALL_LIMIT" || e.Code == "TOTAL_BYTES_LIMIT" || e.Kind == KindIO)
	}
	mdata, e := h.readWalked(manPath)
	if stop(e) {
		return nil, e
	}
	if e != nil {
		add(e.Code, manPath, "manifest를 읽지 못했다")
		return done()
	}
	s.Manifest = digestHex(mdata)
	var man OracleManifest
	if e := decodeOracleManifest(mdata, &man); e != nil {
		add("SET_INVALID_"+e.Code, manPath, "manifest가 완결되지 않았거나 형식이 틀렸다")
		markUsed(h, dir, "")
		return done()
	}
	// the workload profile the run used, matched field by field with the registration
	profPath := "profiles/" + w.Profile + ".json"
	pdata, e := h.readWalked(profPath)
	if stop(e) {
		return nil, e
	}
	if e != nil {
		add("PROFILE_MISSING", profPath, "set의 workload profile이 없다")
	} else {
		s.Profile = digestHex(pdata)
		if s.Profile != man.Workload["profile_sha256"] {
			add("WORKLOAD_MISMATCH", manPath, "set이 결속한 workload profile이 host의 profile과 다르다")
		}
		prof, pe := parseOracle(pdata)
		if pe != nil {
			add("PROFILE_INVALID_"+pe.Code, profPath, "workload profile이 계약에 맞지 않는다")
		} else if d := registrationDiff(prof, w); d != "" {
			add("REGISTRATION_MISMATCH", profPath, "host workload가 등록 inventory와 다르다: "+d)
		}
	}
	plat := man.Producer["platform"]
	if plat != p.pair() {
		add("PLATFORM_MISMATCH", manPath, fmt.Sprintf("set이 %s에서 만들어졌다(칸은 %s)", plat, p.pair()))
	}
	if man.Workload["route"] != w.Route || man.Workload["operation"] != w.Operation || man.Workload["output"] != w.Output {
		add("ROUTE_MISMATCH", manPath, "set의 route·연산·출력이 등록 workload와 다르다")
	}
	if man.Protocol != "tsgk-native/r2" {
		add("PROTOCOL_MISMATCH", manPath, "set protocol이 tsgk-native/r2가 아니다")
	}
	ids := make([]string, len(w.Cases))
	want := map[string]*QualCase{}
	for i := range w.Cases {
		ids[i] = w.Cases[i].ID
		want[w.Cases[i].ID] = &w.Cases[i]
	}
	if !slices.Equal(man.Cases, ids) {
		add("CASE_SET_MISMATCH", manPath, "set 사례 목록이 등록 workload와 다르다")
	}
	qs := ""
	for _, qq := range man.Queries {
		qs += qq.Role + "=" + qq.SHA256 + ";"
	}
	fp := ""
	if man.FactPack != nil {
		fp = man.FactPack.Revision + "=" + man.FactPack.SHA256
	}
	s.Identities = map[string]string{"runtime": man.Producer["runtime_commit"], "protocol": man.Protocol, "comparators": joinList(man.Comparators), "policy": man.Policy.SHA256,
		"queries": qs, "fact_pack": fp, "workload-route": man.Workload["route"], "operation": man.Workload["operation"]}
	for _, k := range []string{"build_identity", "executable_sha256", "compiler_sha256", "compiler_version", "platform"} {
		s.Host[k] = man.Producer[k]
	}
	// members: the shared set rules, read under the host limits
	listed := map[string]bool{manPath: true}
	perCase := map[string]int{}
	var maxWall int64
	var maxPeak uint64
	next := 0
	for i, mem := range man.Members {
		rd := map[string]string{"record": "records/", "raw": "raw/"}[mem.Role]
		full := dir + mem.Path
		if !memberPath.MatchString(mem.Path) || rd == "" || !strings.HasPrefix(mem.Path, rd) || listed[full] {
			add("MEMBER_INVALID", full, "member 경로·역할이 틀렸거나 중복이다")
			continue
		}
		listed[full] = true
		h.used[full] = true
		if mem.Role != "record" {
			if e := x.verifyFile(full, mem.Bytes, mem.SHA256); e != nil {
				if stop(e) {
					return nil, e
				}
				if e.Code == "FILE_BYTES_LIMIT" {
					over = true
					continue
				}
				add(e.Code, full, "raw 응답이 manifest와 다르다")
			}
			continue
		}
		b, e := x.read(full, mem.Bytes, mem.SHA256)
		if stop(e) {
			return nil, e
		}
		if e != nil && e.Code == "FILE_BYTES_LIMIT" {
			over = true
			s.Records++
			perCase[mem.Case]++
			next++
			continue
		}
		if e != nil {
			add(e.Code, full, "record를 확인하지 못했다")
			continue
		}
		s.Records++
		perCase[mem.Case]++
		if h.records++; h.records > uint64(q.lim.Records) {
			return nil, fail(KindResourceLimit, "RECORDS_LIMIT", full, nil)
		}
		if code := checkRecord(b, mem); code != "" {
			add(code, full, "record가 완결되지 않았거나 member identity와 다르다")
			continue
		}
		var c rcCase
		var extra qRecord
		if e := decodeRecord(full, b, &c); e != nil {
			add(e.Code, full, "record를 decode하지 못했다")
			continue
		}
		if err := jsonv2.Unmarshal(b, &extra); err != nil {
			add("RECORD_INVALID", full, "record를 decode하지 못했다")
			continue
		}
		qc := want[c.ID]
		if qc == nil || next >= len(ids) || ids[next] != c.ID {
			add("CASE_ORDER_MISMATCH", full, "record가 등록 사례 순서와 다르다")
			if qc == nil {
				continue
			}
		}
		next++
		reg := IncrementalCase{ID: qc.ID, Input: qc.Input, Edits: qc.Edits, Expect: qc.Expect}
		if c.Input.Path != "" {
			reg.Input.Path, reg.Input.Role = c.Input.Path, c.Input.Role
		}
		x.replayCase(&c, i, &reg, len(w.Queries) > 0)
		q.judgeCase(s, qc, errNodes, &c, &extra, add, full)
		if extra.Process != nil {
			maxWall = max(maxWall, extra.Process.WallMS)
			maxPeak = max(maxPeak, extra.Process.Memory.PeakBytes)
		}
		if e := x.r.check(); e != nil {
			return nil, e
		}
	}
	x.checkSeen(map[string]string{"producer": man.Producer["build_identity"], "policy": man.Policy.SHA256})
	if s.Records != man.Records || len(man.Cases) != man.Records {
		add("RECORD_COUNT_MISMATCH", manPath, "record 수가 manifest와 다르다")
	}
	for _, c := range man.Cases {
		if perCase[c] != 1 {
			add("CASE_RECORD_MISMATCH", manPath, "사례마다 record가 정확히 하나여야 한다: "+c)
		}
	}
	for _, f := range sortedKeys(h.files) {
		if strings.HasPrefix(f, dir) && !listed[f] {
			add("MEMBER_UNLISTED", f, "manifest에 없는 파일이 있다")
			h.used[f] = true
		}
	}
	s.Host["process_wall_ms_max"] = strconv.FormatInt(maxWall, 10)
	s.Host["memory_peak_bytes_max"] = strconv.FormatUint(maxPeak, 10)
	return done()
}

// markUsed marks a set's files and profile as consumed by its (rejected) cell.
func markUsed(h *qhost, dir, profile string) {
	for f := range h.files {
		if strings.HasPrefix(f, dir) || f == profile {
			h.used[f] = true
		}
	}
}

// judgeCase applies the mechanism rules to one recorded case and computes its kind results
// and semantic summary.
func (q *qualifier) judgeCase(s *qset, qc *QualCase, errNodes []string, c *rcCase, extra *qRecord, add func(code, path, msg string), path string) {
	wantStatus := qc.ExpectStatus
	if wantStatus == "" {
		wantStatus = StatusCompleted
	}
	if c.ExecutionStatus != wantStatus {
		add("CASE_STATUS_UNEXPECTED", path, fmt.Sprintf("사례 %s가 %s로 끝났다(등록 %s)", qc.ID, c.ExecutionStatus, wantStatus))
	}
	completed := c.ExecutionStatus == StatusCompleted
	kit := map[string]string{"incremental_equality": c.Claims.IncrementalEquality, "incremental_route": c.Claims.IncrementalRoute}
	if c.Oracle != nil {
		kit["query_equality"], kit["fact_reproduction"], kit["dynamic_sql"] = c.Oracle.QueryEquality, c.Oracle.FactReproduction, c.Oracle.DynamicSQL
		if c.Oracle.API == claimFail || c.Oracle.API == claimBlocked {
			s.APIFails++ // the runtime node API disagreeing with its cursor: an observation for disposition
		}
	}
	for _, k := range sortedKeys(kit) {
		// a BLOCKED route is a route the grammar's error tree left unobservable (recomputed
		// by the incremental-route gate): the E obligation is BLOCKED, the kit has not failed
		if k == "incremental_route" && kit[k] == claimBlocked {
			continue
		}
		if completed && (kit[k] == claimFail || kit[k] == claimBlocked) {
			add("KIT_CLAIM_FAILED", path, fmt.Sprintf("사례 %s의 %s가 %s다", qc.ID, k, kit[k]))
		}
	}
	// per-step expectation results from the registered expectations
	stepRes := make([]string, len(qc.Expect))
	for i, e := range qc.Expect {
		stepRes[i] = claimBlocked
		if completed && e.Step < len(c.Steps) {
			if r, ok := expectResult(e, c.Steps[e.Step].Incremental); ok {
				stepRes[i] = r
			}
		}
	}
	fold := func(pick func(StepExpectation) bool) string {
		out := ""
		for i, e := range qc.Expect {
			if pick(e) {
				if out == "" {
					out = stepRes[i]
				} else {
					out = worseClaim(out, stepRes[i])
				}
			}
		}
		return out
	}
	// query expectations recomputed from the registered source, edits and capture texts
	qe := ""
	var qeDetail []string
	if len(qc.QueryExpect) > 0 {
		qe = claimPass
		versions, _, err := ApplyEdits(EncodingUTF8, []byte(*qc.Source), qc.Edits, uint64(len(*qc.Source))+1<<20)
		for _, e := range qc.QueryExpect {
			r, d := claimPass, ""
			var t *qTree
			if err == nil && completed && e.Step < len(extra.Steps) {
				t = extra.Steps[e.Step].Incremental
			}
			i := -1
			if t != nil {
				i = slices.IndexFunc(t.Queries, func(x qQuery) bool { return x.ID == e.Query })
			}
			switch {
			case i < 0:
				r, d = claimBlocked, "step not recorded"
			case t.Queries[i].Status != e.Status || t.Queries[i].Code != e.Code:
				r, d = claimFail, "status"
			case e.Error != nil:
				var got QueryErrorExpectation
				if t.Queries[i].Error == nil || json.Unmarshal(*t.Queries[i].Error, &got) != nil || got.Type != e.Error.Type || got.Offset != e.Error.Offset {
					r, d = claimFail, "error"
				}
			case e.Captures != nil:
				r, d = captureTexts(*e.Captures, t.Queries[i].Captures, versions[e.Step])
			}
			if d != "" {
				qeDetail = append(qeDetail, e.Query+": "+d)
			}
			qe = worseClaim(qe, r)
		}
		recorded := ""
		if c.Oracle != nil {
			recorded = c.Oracle.QueryExpectations
		}
		g := s.gateRef(q, "query-expectations")
		g.check(!completed || recorded == qe, path, "QUERY_EXPECTATION_MISMATCH", "query 기대값을 다시 계산한 결과가 기록과 다르다")
	}
	kinds := map[string]string{}
	if completed {
		// a P or W step also needs a recorded tree without the route's error node types
		// (has_error does not see them): FAIL when one is there, BLOCKED when the step has
		// no full tree to check
		clean := func(k string) string {
			out := ""
			for i, e := range qc.Expect {
				if !stepCovers(e, k, errNodes) {
					continue
				}
				v := worseClaim(stepRes[i], errorNodesAbsent(c, e.Step, errNodes))
				if out == "" {
					out = v
				} else {
					out = worseClaim(out, v)
				}
			}
			return out
		}
		kinds["P"] = clean("P")
		for _, k := range []string{"N", "R"} {
			kinds[k] = fold(func(e StepExpectation) bool { return stepCovers(e, k, errNodes) })
		}
		if qc.Sample != nil {
			// a registered sample: its step 0 parse, within the sample node bound
			kinds["W"] = clean("W")
			if t := firstTree(extra); kinds["W"] != "" && (t == nil || t.DescendantCount > SampleMaxNodes) {
				kinds["W"] = worseClaim(kinds["W"], claimBlocked) // over the bound it is not a W sample
			}
		}
		if len(qc.Edits) > 0 {
			kinds["E"] = claimPass
			for _, v := range []string{c.Claims.IncrementalEquality, c.Claims.IncrementalRoute} {
				if v != claimPass {
					kinds["E"] = worseClaim(kinds["E"], map[bool]string{true: claimFail, false: claimBlocked}[v == claimFail])
				}
			}
			if c.Oracle != nil && c.Oracle.QueryEquality != claimPass && c.Oracle.QueryEquality != claimNotClaimed {
				kinds["E"] = worseClaim(kinds["E"], c.Oracle.QueryEquality)
			}
		}
		if qe != "" {
			kinds["Q"] = qe
		}
	} else {
		for _, k := range caseKinds {
			kinds[k] = claimBlocked
		}
	}
	s.kinds[qc.ID] = kinds
	switch {
	case c.ExecutionStatus != wantStatus:
		s.checks[qc.ID] = claimBlocked
	case !completed:
		s.checks[qc.ID] = claimPass // the registered non-completion (an over-limit input) happened
	case len(c.Steps) > 0 && !slices.ContainsFunc(c.Steps, func(st rcStep) bool { return st.Incremental != nil || st.Composite == nil }):
		// SVC observation-only: the S05 verdict the verdict gate recomputed (assessment and
		// code); a registered verdict must match it exactly
		if qc.ExpectAssessment != "" {
			s.checks[qc.ID] = map[bool]string{true: claimPass, false: claimFail}[c.Assessment == qc.ExpectAssessment && c.Code == qc.ExpectCode]
			break
		}
		s.checks[qc.ID] = map[string]string{AssessPass: claimPass, AssessFail: claimFail}[c.Assessment]
		if s.checks[qc.ID] == "" {
			s.checks[qc.ID] = claimBlocked
		}
	case qc.ExpectAssessment != "":
		// a registered verdict on a record that is not observation-only: its code was not
		// recomputed by the verdict gate, so it cannot confirm the registration
		s.checks[qc.ID] = claimFail
	default:
		v := claimPass
		for _, r := range stepRes {
			v = worseClaim(v, r)
		}
		if qe != "" {
			v = worseClaim(v, qe)
		}
		s.checks[qc.ID] = v
	}
	if qc.Role == "detector" {
		d := claimPass
		for _, r := range stepRes {
			d = worseClaim(d, r)
		}
		if !completed {
			d = claimBlocked
		}
		s.Detectors[qc.ID] = d
	}
	// the semantic summary compared across platforms
	type step struct {
		Incremental, Fresh any
		Proven             *bool
		Comparison         any
		QueryEqual         *bool
		Composite          any
	}
	steps := []step{}
	for _, st := range extra.Steps {
		x := step{Incremental: st.Incremental.semantic(), Fresh: st.Fresh.semantic(), Composite: compositeSemantic(st.Composite)}
		if st.Route != nil {
			x.Proven = &st.Route.Proven
		}
		if st.Comparison != nil {
			x.Comparison = st.Comparison
		}
		if st.QueryCompare != nil {
			x.QueryEqual = &st.QueryCompare.Equal
		}
		steps = append(steps, x)
	}
	sum, _ := json.Marshal(map[string]any{"status": c.ExecutionStatus, "assessment": c.Assessment, "code": c.Code, "claims": c.Claims, "oracle": c.Oracle,
		"expectations": stepRes, "query_expectations": qe, "query_detail": qeDetail, "steps": steps})
	s.summaries[qc.ID] = string(sum)
}

// errorNodesAbsent checks one step's recorded incremental tree for the route's error node
// types: PASS without any (or when the route registers none), FAIL when one is there and
// BLOCKED when the step has no full tree to check (an absent step or a summary form).
func errorNodesAbsent(c *rcCase, step int, errNodes []string) string {
	if len(errNodes) == 0 {
		return claimPass
	}
	if step >= len(c.Steps) || c.Steps[step].Incremental == nil {
		return claimBlocked
	}
	t := c.Steps[step].Incremental
	if t.Form != "full" || t.Tree == nil {
		return claimBlocked
	}
	for _, n := range t.Tree.Nodes {
		if slices.Contains(errNodes, n.Type) {
			return claimFail
		}
	}
	return claimPass
}

// firstTree is the recorded step 0 incremental tree, nil when absent.
func firstTree(r *qRecord) *qTree {
	if len(r.Steps) == 0 {
		return nil
	}
	return r.Steps[0].Incremental
}

// gateRef returns the host env's gate for this set evaluation.
func (s *qset) gateRef(q *qualifier, id string) *ReplayGate {
	return q.hosts[s.Platform].x.gate(id)
}

// captureTexts compares registered capture names, types and texts with a recorded stream
// in order (S06 rule): the text is the source bytes of the capture's range.
func captureTexts(want []CaptureExpectation, got []Capture, src []byte) (string, string) {
	if got == nil {
		return claimBlocked, "no evaluated captures"
	}
	for i := range min(len(want), len(got)) {
		t, err := CaptureText(src, got[i])
		if err != nil {
			return claimBlocked, err.Error()
		}
		if w := want[i]; w.Name != got[i].Name || w.Type != got[i].Type || w.Text != t {
			return claimFail, fmt.Sprintf("capture %d", i)
		}
	}
	if len(want) != len(got) {
		return claimFail, fmt.Sprintf("capture count %d != %d", len(got), len(want))
	}
	return claimPass, ""
}

// registrationDiff compares a host's workload profile with the registered workload: every
// semantic field except the host compiler and profile id; "" when they match.
func registrationDiff(p OracleProfile, w QualWorkload) string {
	n := p.Native
	switch {
	case n.Route != w.Route:
		return "route"
	case n.Operation != w.Operation:
		return "operation"
	case n.Output != w.Output:
		return "output"
	case n.Symbol != w.Symbol:
		return "symbol"
	case n.Format != w.Format:
		return "format"
	case n.Encoding != EncodingUTF8:
		return "encoding"
	case p.API != w.API:
		return "api"
	}
	if !jsonEqual(n.Grammar, w.Grammar) {
		return "grammar"
	}
	if !jsonEqual(n.Declarations, w.Declarations) {
		return "declarations"
	}
	if (p.FactPack == nil) != (w.FactPack == nil) || (p.FactPack != nil && *p.FactPack != *w.FactPack) {
		return "fact_pack"
	}
	if len(p.Queries) != len(w.Queries) {
		return "queries"
	}
	for i, qq := range p.Queries {
		if qq.ID != w.Queries[i].ID || digestHex([]byte(qq.Source)) != w.Queries[i].SHA256 {
			return "queries/" + qq.ID
		}
	}
	if len(n.Cases) != len(w.Cases) || len(p.OracleCases) != len(w.Cases) {
		return "cases"
	}
	for i, c := range n.Cases {
		r := w.Cases[i]
		if c.ID != r.ID || c.Input.SHA256 != r.Input.SHA256 || c.Input.Bytes != r.Input.Bytes || !slices.EqualFunc(c.Edits, r.Edits, sameEdit) ||
			!slices.EqualFunc(c.Expect, r.Expect, sameExpect) || (len(p.OracleCases[i].QueryExpect)+len(r.QueryExpect) > 0 && !jsonEqual(p.OracleCases[i].QueryExpect, r.QueryExpect)) ||
			(c.Encoding != "" && c.Encoding != EncodingUTF8) || (len(c.Points)+len(r.Points) > 0 && !slices.Equal(c.Points, r.Points)) ||
			!jsonEqual(p.OracleCases[i].DynamicSQL, r.DynamicSQL) {
			return "cases/" + c.ID
		}
	}
	return ""
}

func sameEdit(a, b Edit) bool {
	return a.StartByte == b.StartByte && a.OldEndByte == b.OldEndByte && a.NewEndByte == b.NewEndByte && string(a.Old) == string(b.Old) && string(a.New) == string(b.New)
}

func sameExpect(a, b StepExpectation) bool {
	return a.Step == b.Step && a.Syntax == b.Syntax && slices.Equal(a.Contains, b.Contains) && slices.Equal(a.Anchors, b.Anchors) && a.Declarations == b.Declarations
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// bytesRead is the total of every host's reads.
func (q *qualifier) bytesRead() uint64 {
	n := q.r.totalRead
	for _, h := range q.hosts {
		n += h.x.r.totalRead
	}
	return n
}
