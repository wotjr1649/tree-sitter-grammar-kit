package kit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"slices"
	"sort"
	"strings"
	"time"
)

// Replay schemas and the evidence modes and assessments S07 adds to E0.
const (
	ReplaySchema       = "tsgk-replay/r1"
	ReplayResultSchema = "tsgk-replay-result/r1"

	ModeReplayedRaw           = "REPLAYED_RAW"
	ModeCarriedForward        = "CARRIED_FORWARD"
	ModeRecordedNotRecomputed = "RECORDED_NOT_RECOMPUTED"

	AssessUnresolved    = "UNRESOLVED"
	AssessNotApplicable = "NOT_APPLICABLE"
)

// ReplayLimits are the finite bounds of a replay operation. Records bounds each record
// collection (cases, inventory records, ledger rows); RecordBytes bounds one decoded record
// value, so a large raw stream is read one record at a time.
type ReplayLimits struct {
	Files       uint64        `json:"files"`
	FileBytes   uint64        `json:"file_bytes"`
	TotalBytes  uint64        `json:"total_bytes"`
	Records     uint64        `json:"records"`
	RecordBytes uint64        `json:"record_bytes"`
	OutputBytes uint64        `json:"output_bytes"`
	Wall        time.Duration `json:"-"`
	WallMillis  int64         `json:"wall_ms"`
}

// ReplayOperations returns the registered replay operations: evidence-replay (the S07
// budget operation, three OS) and private-corpus-replay (NET461-PHASE2-LOCAL-r1 local raw:
// 26000 files, 2 GiB of records, 1800 s).
func ReplayOperations() map[string]ReplayLimits {
	return map[string]ReplayLimits{
		"evidence-replay":       {Files: 10000, FileBytes: 16777216, TotalBytes: 268435456, Records: 100000, RecordBytes: 16777216, OutputBytes: 16777216, Wall: 120 * time.Second, WallMillis: 120000},
		"private-corpus-replay": {Files: 26000, FileBytes: 2147483648, TotalBytes: 2147483648, Records: 26000, RecordBytes: 16777216, OutputBytes: 67108864, Wall: 1800 * time.Second, WallMillis: 1800000},
	}
}

// ReplayMember is one expected file of the evidence set: the profile's independent
// inventory, never derived from the set itself.
type ReplayMember struct {
	Path   string `json:"path"`
	Role   string `json:"role"`
	Bytes  uint64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// ReplaySubject is the historical subject run the replay refers to. The replay never
// changes it: its recorded evidence mode stays as recorded.
type ReplaySubject struct {
	Run             string `json:"run"`
	Attempt         uint64 `json:"attempt"`
	Platform        string `json:"platform"`
	Commit          string `json:"commit"`
	EvidenceMode    string `json:"evidence_mode"`
	ExecutionStatus string `json:"execution_status"`
	Assessment      string `json:"assessment"`
}

// ReplayProfile is a decoded tsgk-replay/r1 registration.
type ReplayProfile struct {
	SHA256     string            `json:"-"`
	ID         string            `json:"id"`
	Reducer    string            `json:"reducer"`
	Operation  string            `json:"operation"`
	Subject    ReplaySubject     `json:"subject"`
	Identities map[string]string `json:"identities"`
	Records    []string          `json:"records"` // nil: the reducer takes the expected records from the workload
	Members    []ReplayMember    `json:"members"`
}

// Verdict is one execution status and assessment pair.
type Verdict struct {
	ExecutionStatus string `json:"execution_status"`
	Assessment      string `json:"assessment"`
}

// ReplayGate is one recomputed or recorded check of a reducer over its records.
// EvidenceMode is REPLAYED_RAW only when every record of the gate was recomputed.
type ReplayGate struct {
	ID            string `json:"id"`
	EvidenceMode  string `json:"evidence_mode"`
	Assessment    string `json:"assessment"`
	Recomputed    int    `json:"recomputed"`
	NotRecomputed int    `json:"not_recomputed"`
	Failed        int    `json:"failed"`
	Code          string `json:"code"`
	Record        string `json:"record"`
	Detail        string `json:"detail"`
}

// Consumption is the exact raw-use account of the expected records.
type Consumption struct {
	Expected   int    `json:"expected"`
	Consumed   int    `json:"consumed"`
	Missing    int    `json:"missing"`
	Duplicate  int    `json:"duplicate"`
	Unused     int    `json:"unused"`
	OutOfOrder int    `json:"out_of_order"`
	Excluded   int    `json:"excluded"` // records of another cohort, counted and not consumed
	First      string `json:"first"`
}

// ReplayResult is the tsgk-replay-result/r1 document: the E0 envelope of the replay
// operation plus the unchanged subject, its recorded verdict, the recomputed verdict, the
// gates, the consumption account and Korean explanation lines.
type ReplayResult struct {
	Report
	ResultSchema  string            `json:"result_schema"`
	ProfileID     string            `json:"profile_id"`
	Reducer       ReducerInfo       `json:"reducer"`
	Operation     string            `json:"operation"`
	Limits        ReplayLimits      `json:"limits"`
	Subject       ReplaySubject     `json:"subject"`
	Recorded      Verdict           `json:"recorded"`
	Recomputed    Verdict           `json:"recomputed"`
	EvidenceValid bool              `json:"evidence_valid"`
	Observed      map[string]string `json:"observed_identities"`
	Gates         []ReplayGate      `json:"gates"`
	Consumption   Consumption       `json:"consumption"`
	Members       int               `json:"members"`
	Retained      int               `json:"retained_members"` // registered and hashed, not evidence of this reducer
	BytesRead     uint64            `json:"bytes_read"`
	Explanation   []string          `json:"explanation"`
}

// ReplayRequest names the evidence set root and the replay registration bytes.
type ReplayRequest struct {
	Root    string
	Profile []byte
}

// ParseReplayProfile strictly decodes a tsgk-replay/r1 registration.
func ParseReplayProfile(data []byte) (ReplayProfile, error) {
	p, e := parseReplay(data)
	if e != nil {
		return ReplayProfile{}, e
	}
	return p, nil
}

var evidenceModes = map[string]bool{ModeNewRun: true, ModeReplayedRaw: true, ModeCarriedForward: true, ModeRecordedNotRecomputed: true, ModeNotRun: true}
var executionStatuses = map[string]bool{StatusNotRun: true, StatusCompleted: true, StatusFailed: true, StatusCancelled: true, StatusResourceLimit: true}
var assessmentValues = map[string]bool{AssessPass: true, AssessFail: true, AssessBlocked: true, AssessNotApplicable: true, AssessUnresolved: true, AssessNotAssessed: true}

func parseReplay(data []byte) (ReplayProfile, *Error) {
	var p ReplayProfile
	t := typed{doc: "replay"}
	v, e := decodeStrict(t.doc, data, MaxDocumentBytes)
	if e != nil {
		return p, e
	}
	m, e := t.object(v, []string{"schema", "id", "reducer", "operation", "subject", "identities", "records", "members"})
	if e != nil {
		return p, e
	}
	if s, e := t.str(m["schema"]); e != nil {
		return p, e
	} else if s != ReplaySchema {
		return p, fail(KindUnsupported, "SCHEMA_UNSUPPORTED", "replay#/schema", nil)
	}
	for _, f := range []struct {
		dst  *string
		name string
	}{{&p.ID, "id"}, {&p.Reducer, "reducer"}, {&p.Operation, "operation"}} {
		if *f.dst, e = t.str(m[f.name]); e != nil {
			return p, e
		} else if !validID(*f.dst) {
			return p, t.bad("ID_INVALID", m[f.name])
		}
	}
	sm, e := t.object(m["subject"], []string{"run", "attempt", "platform", "commit", "evidence_mode", "execution_status", "assessment"})
	if e != nil {
		return p, e
	}
	s := &p.Subject
	for _, f := range []struct {
		dst  *string
		name string
		ok   map[string]bool
	}{{&s.Run, "run", nil}, {&s.Platform, "platform", nil}, {&s.Commit, "commit", nil},
		{&s.EvidenceMode, "evidence_mode", evidenceModes}, {&s.ExecutionStatus, "execution_status", executionStatuses}, {&s.Assessment, "assessment", assessmentValues}} {
		if *f.dst, e = t.str(sm[f.name]); e != nil {
			return p, e
		}
		if f.ok != nil && !f.ok[*f.dst] {
			return p, t.bad("SUBJECT_VALUE_INVALID", sm[f.name])
		}
	}
	if s.Run == "" || len(s.Run) > 128 {
		return p, t.bad("SUBJECT_VALUE_INVALID", sm["run"])
	}
	if s.Attempt, e = t.uint(sm["attempt"]); e != nil {
		return p, e
	}
	im, e := t.object(m["identities"], m["identities"].keys)
	if e != nil {
		return p, e
	}
	p.Identities = map[string]string{}
	for k, iv := range im {
		if p.Identities[k], e = t.str(iv); e != nil {
			return p, e
		}
	}
	if rv := m["records"]; rv.kind != 'n' {
		list, e := t.array(rv)
		if e != nil {
			return p, e
		}
		p.Records = []string{}
		for _, x := range list {
			id, e := t.str(x)
			if e != nil {
				return p, e
			} else if id == "" || len(id) > 4096 {
				return p, t.bad("RECORD_ID_INVALID", x)
			}
			p.Records = append(p.Records, id)
		}
	}
	list, e := t.array(m["members"])
	if e != nil {
		return p, e
	}
	seen := map[string]bool{}
	for _, x := range list {
		mm, e := t.object(x, []string{"path", "role", "bytes", "sha256"})
		if e != nil {
			return p, e
		}
		var mem ReplayMember
		if mem.Path, e = t.str(mm["path"]); e != nil {
			return p, e
		} else if !portable(mem.Path, false) {
			return p, t.bad("MEMBER_PATH_INVALID", mm["path"])
		}
		folded := strings.ToLower(mem.Path)
		if seen[folded] {
			return p, t.bad("MEMBER_DUPLICATE", mm["path"])
		}
		seen[folded] = true
		if mem.Role, e = t.str(mm["role"]); e != nil {
			return p, e
		} else if mem.Role == "" || len(mem.Role) > 128 {
			return p, t.bad("MEMBER_ROLE_INVALID", mm["role"])
		}
		if mem.Bytes, e = t.uint(mm["bytes"]); e != nil {
			return p, e
		}
		if mem.SHA256, e = hexDigest(t, mm["sha256"]); e != nil {
			return p, e
		}
		p.Members = append(p.Members, mem)
	}
	p.SHA256 = digestHex(data)
	return p, nil
}

// ReducerInfo describes one registered reducer: the raw inputs it reads, the gates it
// recomputes and the recorded gates it does not recompute. The registry is code, not a
// plugin language; an unknown reducer is UNSUPPORTED.
type ReducerInfo struct {
	ID          string   `json:"id"`
	Revision    string   `json:"revision"`
	Operation   string   `json:"operation"`
	Inputs      []string `json:"inputs"`
	Roles       []string `json:"member_roles"` // registrable member roles; a role ending in ':' takes a route group suffix
	Gates       []string `json:"gates"`
	Recorded    []string `json:"recorded_not_recomputed"`
	Description string   `json:"description"`
}

type reducer struct {
	info ReducerInfo
	run  func(*replayEnv) *Error
}

func reducerTable() map[string]reducer {
	t := map[string]reducer{
		"native-result-r1": {ReducerInfo{ID: "native-result-r1", Revision: "r1", Operation: "evidence-replay",
			Inputs:      []string{"tsgk-incremental-result/r1 result.json", "tsgk-incremental/r2 or r1 workload profile", "responses/ members"},
			Roles:       []string{"workload-profile", "result", "response", "retained"},
			Gates:       nativeGates,
			Recorded:    []string{"driver response payloads (tsgk-native protocol decoding is not part of the offline API)", "expectations with declarations on r1 full trees", "digests of record and summary form trees"},
			Description: "S05 incremental result: exact case consumption against the workload profile, tree digest and structure, incremental/fresh comparison, route proof, expectations and verdict fold recomputed from the recorded trees"}, replayNativeResult},
		"oracle-set-r1": {ReducerInfo{ID: "oracle-set-r1", Revision: "r1", Operation: "evidence-replay",
			Inputs:      []string{"tsgk-oracle-manifest/r1 manifest.json", "tsgk-oracle-record/r1 records/", "raw/ driver responses", "tsgk-oracle/r2 or r1 workload profile"},
			Roles:       []string{"workload-profile", "manifest", "record", "raw", "retained"},
			Gates:       append(slices.Clone(nativeGates), "query-equality"),
			Recorded:    []string{"api (tsgk-api/r1 observations stay in the raw response)", "query_expectations, fact_reproduction, dynamic_sql (need the case source bytes)", "runtime structural capture streams of UNSUPPORTED queries"},
			Description: "S06 record set: set completeness through the shared record checks, then the S05 gates and the incremental/fresh query comparison per record"}, replayOracleSet},
		"private-corpus-r1": {ReducerInfo{ID: "private-corpus-r1", Revision: "r1", Operation: "private-corpus-replay",
			Inputs:      []string{"tsgk-report/r1 corpus inventory", "tsgk-incremental/r2 or r1 route profiles", "tsgk-incremental-result/r1 route results", "per-file record projection (jsonl)", "local-run summary"},
			Roles:       []string{"inventory", "summary", "projection", "workload-profile:", "result:", "retained"},
			Gates:       []string{"local-run-identity", "corpus-binding", "case-status", "verdict", "record-projection", "summary"},
			Recorded:    []string{"tree digests of record-form trees (no nodes are kept)"},
			Description: "NET461-PHASE2-LOCAL-r1: every routed inventory record consumed by exactly one route case, case verdicts, the per-file projection and the summary counts recomputed; paths are never reported"}, replayPrivateCorpus},
		"prepare-native-r1": {ReducerInfo{ID: "prepare-native-r1", Revision: "r1", Operation: "evidence-replay",
			Inputs:      []string{"PREPARE evidence-manifest.json", "records/case-ledger.json", "raw/case-*.stdout probe output"},
			Roles:       []string{"inventory", "ledger", "retained"},
			Gates:       []string{"inventory", "ledger-binding", "raw-binding", "exit", "syntax"},
			Recorded:    []string{"fact-checks (PREPARE review helper, not ported)", "edit-recovery (PREPARE review helper, not ported)", "inventory entries checked by size only (not read) and absent raw"},
			Description: "PREPARE native probe evidence: bundle inventory, exact registered rows of one producer, raw stdout binding and the original-stage syntax class recomputed; registered fact and edit checks stay recorded"}, replayPrepareNative},
		"bs-gate-compare-r1": {ReducerInfo{ID: "bs-gate-compare-r1", Revision: "r1", Operation: "evidence-replay",
			Inputs:      []string{"recorded gate documents", "recomputed gate documents (from a separately authorized EXEC_ADAPTER run)"},
			Roles:       []string{"recorded-gate", "recomputed-gate", "retained"},
			Gates:       []string{"gate-compare"},
			Recorded:    []string{"raw-recompute (BrightScript gates.py is an archived Python verifier; the kit compares only)"},
			Description: "BrightScript v0.1.2 historical policy S07-REPLAY-2ULP-r1, scoped to this workload adapter: exact types, keys, order, identities, verdicts and thresholds; <= 2 ULP only on the allowlisted log-derived exponents, zero and subnormal bit-exact, no threshold straddle, unchanged max winner"}, replayGateCompare},
	}
	for _, id := range []string{"native-result", "oracle-set", "private-corpus"} {
		t[id+"-r2"] = errorTreeRevision(t[id+"-r1"], id+"-r2")
	}
	return t
}

// errorTreeRevision is the r2 revision of a reducer that judges S05 cases: the same inputs
// and gates with the S05 route rule that an edit step reusing no node next to a full error
// tree is BLOCKED (unobservable). The r1 revision keeps every unproven route a FAIL, so
// evidence recorded before the rule replays as it did.
func errorTreeRevision(r reducer, id string) reducer {
	info := r.info
	info.ID, info.Revision = id, "r2"
	info.Inputs, info.Roles = slices.Clone(info.Inputs), slices.Clone(info.Roles)
	info.Gates, info.Recorded = slices.Clone(info.Gates), slices.Clone(info.Recorded)
	info.Description += "; r2: an edit step whose instrumented edit with changes reused no node, between full trees one of which has an error, is an unobservable route (BLOCKED, not FAIL)"
	run := r.run
	return reducer{info, func(x *replayEnv) *Error {
		x.errorTreeRoute = true
		return run(x)
	}}
}

// Reducers returns the registered reducer inventory in id order.
func Reducers() []ReducerInfo {
	var out []ReducerInfo
	for _, r := range reducerTable() {
		out = append(out, r.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// replayEnv is the state of one replay: the bounded root, the walked file set and the
// reducer's accounting.
type replayEnv struct {
	svcContext     *SvcContext
	svcFormat      string
	svcEvidence    map[string]svcCaseEvidence
	svcReferences  []svcReferenceLink
	r              *run
	g              *guard
	lim            ReplayLimits
	prof           ReplayProfile
	files          map[string]fs.FileInfo
	members        map[string]ReplayMember
	verified       map[string]bool
	consumed       map[string]bool            // members the reducer read or bound
	seen           map[string]map[string]bool // producer and policy values named by trees
	covered        map[string]bool            // files a reducer inventory lists besides the profile members
	gates          []*ReplayGate
	cons           Consumption
	actual         map[string]string
	recorded       Verdict
	recomp         Verdict
	findings       []Finding
	redact         bool
	over           bool   // a needed or registered member exceeds the operation limits: recorded, not recomputed
	overAt         string // the member that did
	needOver       bool   // a member the reducer needed was beyond the limits: it stopped early
	noRecomp       bool   // the reducer recomputed no subject outcome (absent raw)
	errorTreeRoute bool   // the r2 S05 route rule: no reuse next to a full error tree is BLOCKED
}

func (x *replayEnv) finding(code, path, msg string) {
	if len(x.findings) < 1000 {
		x.findings = append(x.findings, Finding{Code: code, Severity: "error", Path: path, Message: msg})
	}
}

// gate returns the named gate, creating it in first-use order.
func (x *replayEnv) gate(id string) *ReplayGate {
	for _, g := range x.gates {
		if g.ID == id {
			return g
		}
	}
	g := &ReplayGate{ID: id}
	x.gates = append(x.gates, g)
	return g
}

func (x *replayEnv) name(id string, index int) string {
	if x.redact {
		return fmt.Sprintf("#%d", index)
	}
	return id
}

func (g *ReplayGate) pass() { g.Recomputed++ }

func (g *ReplayGate) fail(record, code, detail string) {
	g.Recomputed++
	g.Failed++
	if g.Code == "" {
		g.Code, g.Record, g.Detail = code, record, detail
	}
}

func (g *ReplayGate) recorded() { g.NotRecomputed++ }

// check records a recomputed comparison: ok passes, otherwise the first failure is kept.
func (g *ReplayGate) check(ok bool, record, code, detail string) bool {
	if ok {
		g.pass()
	} else {
		g.fail(record, code, detail)
	}
	return ok
}

func (g *ReplayGate) finish() {
	switch {
	case g.Failed > 0:
		g.Assessment = AssessFail
	case g.NotRecomputed > 0 || g.Recomputed == 0:
		g.Assessment = AssessUnresolved
	default:
		g.Assessment = AssessPass
	}
	g.EvidenceMode = ModeRecordedNotRecomputed
	if g.Recomputed > 0 && g.NotRecomputed == 0 {
		g.EvidenceMode = ModeReplayedRaw
	}
}

// member returns the bytes of a profile member, verified against its expected size and
// sha256 and bounded by the operation limits. A member over the per-file limit is not
// read: the replay records it instead of recomputing.
func (x *replayEnv) member(path string) ([]byte, *Error) {
	mem, ok := x.members[path]
	if !ok {
		return nil, fail(KindInvalidInput, "MEMBER_NOT_REGISTERED", path, nil)
	}
	b, e := x.read(path, mem.Bytes, mem.SHA256)
	if e == nil {
		x.consumed[path] = true
	}
	return b, e
}

// verifyFile hashes one walked file in a bounded stream against a trusted size and sha256
// without keeping its bytes; only the per-file and total limits apply.
func (x *replayEnv) verifyFile(path string, size uint64, sum string) *Error {
	info := x.files[path]
	if info == nil {
		return fail(KindInvalidInput, "MEMBER_MISSING", path, nil)
	}
	if uint64(info.Size()) != size {
		return fail(KindInvalidInput, "MEMBER_MISMATCH", path, nil)
	}
	if size > x.lim.FileBytes {
		x.over, x.overAt = true, path
		return fail(KindResourceLimit, "FILE_BYTES_LIMIT", path, nil)
	}
	st, e := x.g.readFile(x.r, path, info, x.lim.FileBytes, x.lim.TotalBytes, 0)
	if e != nil {
		return e
	}
	if st.sha256 != sum {
		return fail(KindInvalidInput, "MEMBER_MISMATCH", path, nil)
	}
	x.verified[path] = true
	return nil
}

// read reads one walked file whose identity comes from a trusted inventory.
func (x *replayEnv) read(path string, size uint64, sum string) ([]byte, *Error) {
	info := x.files[path]
	if info == nil {
		return nil, fail(KindInvalidInput, "MEMBER_MISSING", path, nil)
	}
	if uint64(info.Size()) != size {
		return nil, fail(KindInvalidInput, "MEMBER_MISMATCH", path, nil)
	}
	if size > x.lim.FileBytes || size > x.lim.RecordBytes {
		x.over, x.overAt = true, path
		return nil, fail(KindResourceLimit, "FILE_BYTES_LIMIT", path, nil)
	}
	st, e := x.g.readFile(x.r, path, info, x.lim.FileBytes, x.lim.TotalBytes, size+1)
	if e != nil {
		return nil, e
	}
	if st.sha256 != sum {
		return nil, fail(KindInvalidInput, "MEMBER_MISMATCH", path, nil)
	}
	x.verified[path] = true
	return st.content, nil
}

// stream opens a profile member for one bounded streaming pass; finish verifies the bytes
// read were the whole member with the expected sha256.
func (x *replayEnv) stream(path string) (*memberReader, *Error) {
	mem, ok := x.members[path]
	if !ok {
		return nil, fail(KindInvalidInput, "MEMBER_NOT_REGISTERED", path, nil)
	}
	info := x.files[path]
	if info == nil {
		return nil, fail(KindInvalidInput, "MEMBER_MISSING", path, nil)
	}
	if uint64(info.Size()) != mem.Bytes {
		return nil, fail(KindInvalidInput, "MEMBER_MISMATCH", path, nil)
	}
	if mem.Bytes > x.lim.FileBytes {
		x.over, x.overAt = true, path
		return nil, fail(KindResourceLimit, "FILE_BYTES_LIMIT", path, nil)
	}
	f, err := x.g.root.Open(path)
	if err != nil {
		return nil, fail(KindIO, "READ_FAILED", path, err)
	}
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) || opened.Size() != info.Size() || !opened.ModTime().Equal(info.ModTime()) {
		f.Close()
		return nil, fail(KindIO, "SOURCE_CHANGED", path, err)
	}
	if e := checkLinks(f, path); e != nil {
		f.Close()
		return nil, e
	}
	return &memberReader{x: x, f: f, path: path, mem: mem, h: sha256.New(), info: opened}, nil
}

// memberReader is a bounded, hashing, cancellable reader over one member. limit, when
// positive, is the read position a single record value may not pass.
type memberReader struct {
	x     *replayEnv
	f     *os.File
	path  string
	mem   ReplayMember
	h     hash.Hash
	info  fs.FileInfo
	pos   int64
	limit int64
	err   *Error
}

var errRecordLimit = errors.New("record bytes limit")

// testHookReplayLimits lets a test lower the operation limits to exercise them cheaply.
var testHookReplayLimits func(ReplayLimits) ReplayLimits

func (m *memberReader) Read(p []byte) (int, error) {
	if e := m.x.r.check(); e != nil {
		m.err = e
		return 0, e
	}
	if m.limit > 0 {
		if m.pos >= m.limit {
			return 0, errRecordLimit
		}
		if room := m.limit - m.pos; int64(len(p)) > room {
			p = p[:room]
		}
	}
	if len(p) > readChunk {
		p = p[:readChunk]
	}
	n, err := m.f.Read(p)
	if n > 0 {
		m.pos += int64(n)
		m.h.Write(p[:n])
		m.x.r.totalRead += uint64(n)
		if m.x.r.totalRead > m.x.lim.TotalBytes {
			m.err = fail(KindResourceLimit, "TOTAL_BYTES_LIMIT", m.path, nil)
			return n, m.err
		}
	}
	return n, err
}

// finish drains the rest, closes the file and checks size, sha256 and stability.
func (m *memberReader) finish() *Error {
	defer m.f.Close()
	if m.err != nil {
		return m.err
	}
	m.limit = 0
	if _, err := io.Copy(io.Discard, m); err != nil {
		if m.err != nil {
			return m.err
		}
		return fail(KindIO, "READ_FAILED", m.path, err)
	}
	after, err := m.f.Stat()
	if err != nil || after.Size() != m.info.Size() || !after.ModTime().Equal(m.info.ModTime()) || m.pos != m.info.Size() {
		return fail(KindIO, "SOURCE_CHANGED", m.path, err)
	}
	if hex.EncodeToString(m.h.Sum(nil)) != m.mem.SHA256 {
		return fail(KindInvalidInput, "MEMBER_MISMATCH", m.path, nil)
	}
	m.x.verified[m.path] = true
	m.x.consumed[m.path] = true
	return nil
}

// walk lists every entry below the root once, bounded by the file limit; links and special
// files are rejected, never followed.
func (x *replayEnv) walk(dir string, count *uint64) *Error {
	entries, e := x.g.readDir(x.r, dir, count, x.lim.Files)
	if e != nil {
		return e
	}
	for _, d := range entries {
		p := join(dir, d.Name())
		info, e := x.g.lstat(p)
		if e != nil {
			return e
		}
		if info == nil {
			return fail(KindIO, "SOURCE_CHANGED", p, nil)
		}
		switch kind(info) {
		case "dir":
			if depthOf(p) > 64 {
				return fail(KindResourceLimit, "DEPTH_LIMIT", p, nil)
			}
			if e := x.walk(p, count); e != nil {
				return e
			}
		case "file":
			x.files[p] = info
		default:
			return fail(KindInvalidInput, "LINK_OR_SPECIAL_REJECTED", p, nil)
		}
	}
	return nil
}

// Replay runs one registered data-only reducer over an evidence set root. It reads only
// the root, starts no process and imports no code from the evidence; an unknown reducer or
// schema is UNSUPPORTED, never a replayed success.
func Replay(ctx context.Context, req ReplayRequest) (ReplayResult, error) {
	res := ReplayResult{Report: newReport("replay"), ResultSchema: ReplayResultSchema, Gates: []ReplayGate{}, Observed: map[string]string{}, Explanation: []string{}}
	res.EvidenceMode = ModeReplayedRaw
	res.Assessment = AssessNotAssessed
	bad := func(e *Error) (ReplayResult, error) {
		failReport(&res.Report, e)
		res.Explanation = append(res.Explanation, explainFailure(e))
		return res, e
	}
	prof, e := parseReplay(req.Profile)
	if e != nil {
		return bad(e)
	}
	res.ProfileID, res.Subject, res.Operation = prof.ID, prof.Subject, prof.Operation
	res.Recorded = Verdict{prof.Subject.ExecutionStatus, prof.Subject.Assessment}
	res.Identities = append(res.Identities, IdentityRef{Role: "replay-profile", Schema: ReplaySchema, SHA256: prof.SHA256})
	red, ok := reducerTable()[prof.Reducer]
	if !ok {
		return bad(fail(KindUnsupported, "REDUCER_UNSUPPORTED", "replay#/reducer", nil))
	}
	res.Reducer = red.info
	lim, ok := ReplayOperations()[prof.Operation]
	if !ok {
		return bad(fail(KindUnsupported, "OPERATION_UNSUPPORTED", "replay#/operation", nil))
	}
	if prof.Operation != red.info.Operation {
		return bad(fail(KindInvalidInput, "OPERATION_REDUCER_MISMATCH", "replay#/operation", nil))
	}
	if testHookReplayLimits != nil {
		lim = testHookReplayLimits(lim)
	}
	res.Limits = lim
	res.Identities = append(res.Identities, IdentityRef{Role: "reducer", Schema: red.info.ID, SHA256: digestHex([]byte(reducerText(red.info)))},
		IdentityRef{Role: "policy", Schema: "tsgk-replay-policy/r1", SHA256: digestHex([]byte(replayPolicyText(prof.Operation, lim)))},
		IdentityRef{Role: "evidence-set", Schema: "tsgk-replay-members/r1", SHA256: digestHex([]byte(membersText(prof.Members)))})
	res.Coverage.Requested = slices.Clone(red.info.Gates)
	r, e := startRun(ctx, lim.Wall)
	if e != nil {
		return bad(e)
	}
	defer r.cancel()
	root, _, e := openRoot(req.Root)
	if e != nil {
		return bad(e)
	}
	defer root.Close()
	x := &replayEnv{r: r, g: newGuard(root), lim: lim, prof: prof, files: map[string]fs.FileInfo{}, members: map[string]ReplayMember{},
		verified: map[string]bool{}, consumed: map[string]bool{}, seen: map[string]map[string]bool{}, covered: map[string]bool{}, actual: map[string]string{}}
	for _, m := range prof.Members {
		x.members[m.Path] = m
	}
	var count uint64
	if e := x.walk(".", &count); e != nil {
		return bad(e)
	}
	if e := red.run(x); e != nil {
		switch {
		case e.Kind == KindResourceLimit && x.over:
			x.needOver = true
			// a needed raw member is beyond the operation limits: it is not read and the
			// subject stays as recorded; everything found so far still counts
		case e.Kind == KindInvalidInput && (strings.HasPrefix(e.Code, "MEMBER_") || strings.HasPrefix(e.Code, "RECORD_") || strings.HasPrefix(e.Code, "JSON_")):
			// damaged or incomplete evidence is a completed check that failed, not an argument error
			x.finding(e.Code, e.Path, "evidence가 등록과 다르거나 불완전해 소비를 끝내지 못했다")
		default:
			res.BytesRead = r.totalRead
			return bad(e)
		}
	}
	// every registered member has a role of the reducer, is verified, and is consumed by the
	// reducer unless it is registered as retained; every walked file is registered or listed
	for _, m := range prof.Members {
		known := false
		for _, role := range red.info.Roles {
			known = known || m.Role == role || (strings.HasSuffix(role, ":") && strings.HasPrefix(m.Role, role) && validID(strings.TrimPrefix(m.Role, role)))
		}
		if !known {
			x.finding("MEMBER_ROLE_UNKNOWN", m.Path, "reducer가 모르는 member 역할이다")
			continue
		}
		if !x.verified[m.Path] {
			if e := x.verifyFile(m.Path, m.Bytes, m.SHA256); e != nil {
				switch {
				case e.Kind == KindCancelled || e.Kind == KindIO || e.Code == "TOTAL_BYTES_LIMIT":
					res.BytesRead = r.totalRead
					return bad(e)
				case e.Kind != KindResourceLimit:
					x.finding(e.Code, m.Path, "등록된 member를 확인하지 못했다")
				case x.overAt != m.Path:
					// grew past the limit while read: unverified, not a pass
					res.BytesRead = r.totalRead
					return bad(e)
				}
				if !x.consumed[m.Path] && m.Role != "retained" && !x.needOver {
					x.finding("MEMBER_UNUSED", m.Path, "reducer가 소비하지 않은 등록 member다(evidence가 아니면 retained로 등록한다)")
				}
				continue
			}
		}
		if !x.consumed[m.Path] && m.Role != "retained" && !x.needOver {
			x.finding("MEMBER_UNUSED", m.Path, "reducer가 소비하지 않은 등록 member다(evidence가 아니면 retained로 등록한다)")
		}
	}
	unlisted := 0
	for p := range x.files {
		if _, ok := x.members[p]; !ok && !x.covered[p] {
			if unlisted++; unlisted <= 10 {
				x.finding("MEMBER_UNLISTED", p, "등록 inventory에 없는 파일이 있다")
			}
		}
	}
	// the profile binds every identity the reducer exposes, exactly
	roles := map[string]bool{}
	for k := range x.actual {
		roles[k] = true
	}
	for k := range prof.Identities {
		roles[k] = true
	}
	for _, k := range sortedKeys(roles) {
		want, wok := prof.Identities[k]
		got, gok := x.actual[k]
		switch {
		case !wok:
			x.finding("IDENTITY_UNBOUND", k, "reducer가 관측한 identity를 등록이 결속하지 않았다")
		case !gok && x.needOver:
			// not observed: the raw that would show it was beyond the limits
		case !gok:
			x.finding("IDENTITY_UNKNOWN", k, "reducer가 관측하지 않는 identity를 등록했다")
		case want != got:
			x.finding("IDENTITY_MISMATCH", k, "등록 identity와 raw의 identity가 다르다(stale 또는 혼합)")
		}
	}
	if x.recorded.ExecutionStatus != "" {
		if x.recorded != res.Recorded {
			x.finding("RECORDED_VERDICT_MISMATCH", "", "raw에 기록된 판정이 등록한 subject 판정과 다르다")
		}
		res.Recorded = x.recorded // what the raw records; the registration's stays in subject
	}
	for _, g := range x.gates {
		g.finish()
		res.Gates = append(res.Gates, *g)
		res.Coverage.Observed = append(res.Coverage.Observed, g.ID)
		if g.EvidenceMode != ModeReplayedRaw {
			res.Coverage.Unsupported = append(res.Coverage.Unsupported, g.ID)
		}
	}
	res.Consumption = x.cons
	res.Observed = x.actual
	if c := x.cons; c.Missing+c.Duplicate+c.Unused+c.OutOfOrder > 0 {
		x.finding("CONSUMPTION_INCOMPLETE", c.First, "expected record의 누락·중복·미사용·순서 오류가 있다")
	}
	res.Findings = append(res.Findings, x.findings...)
	if x.over {
		res.Findings = append(res.Findings, Finding{Code: "RAW_OVER_LIMIT", Severity: "warning", Path: x.overAt, Message: "raw가 연산 한도를 넘어 읽지 않았다. 다시 계산하지 않고 기록 판정만 보존한다"})
	}
	for _, m := range prof.Members {
		if m.Role == "retained" {
			res.Retained++
		}
	}
	res.Members, res.BytesRead = len(prof.Members), r.totalRead
	res.Recomputed = x.recomp
	failed := false
	for _, g := range res.Gates {
		failed = failed || g.Failed > 0
	}
	res.EvidenceValid = len(x.findings) == 0 && !failed
	recomputedAny := false
	for _, g := range res.Gates {
		recomputedAny = recomputedAny || g.Recomputed > 0
	}
	if !recomputedAny {
		res.EvidenceMode = ModeRecordedNotRecomputed
	}
	if (x.over || x.noRecomp) && res.EvidenceValid {
		res.EvidenceMode, res.Recomputed = ModeRecordedNotRecomputed, Verdict{StatusNotRun, AssessUnresolved}
	}
	switch {
	case !res.EvidenceValid:
		res.Assessment = AssessFail
	case res.Recomputed.Assessment == "":
		res.Assessment = AssessUnresolved
	default:
		res.Assessment = res.Recomputed.Assessment
	}
	res.Explanation = append(res.Explanation, explainReplay(res)...)
	if e := sealOutput(res, lim.OutputBytes); e != nil {
		return bad(e)
	}
	if e := r.check(); e != nil {
		return bad(e)
	}
	return res, nil
}

func reducerText(i ReducerInfo) string {
	return "tsgk-reducer/r1\nid=" + i.ID + "\nrevision=" + i.Revision + "\noperation=" + i.Operation + "\ngates=" + strings.Join(i.Gates, ",") + "\nrecorded=" + strings.Join(i.Recorded, ",") + "\n"
}

func replayPolicyText(op string, l ReplayLimits) string {
	return fmt.Sprintf("tsgk-replay-policy/r1\noperation=%s\nfiles=%d\nfile_bytes=%d\ntotal_bytes=%d\nrecords=%d\nrecord_bytes=%d\noutput_bytes=%d\nwall_ms=%d\n",
		op, l.Files, l.FileBytes, l.TotalBytes, l.Records, l.RecordBytes, l.OutputBytes, l.WallMillis)
}

func membersText(ms []ReplayMember) string {
	sorted := slices.Clone(ms)
	slices.SortFunc(sorted, func(a, b ReplayMember) int { return strings.Compare(a.Path, b.Path) })
	var b strings.Builder
	b.WriteString("tsgk-replay-members/r1\n")
	for _, m := range sorted {
		fmt.Fprintf(&b, "%s\x00%s\x00%d\x00%s\n", m.Path, m.Role, m.Bytes, m.SHA256)
	}
	return b.String()
}

func explainFailure(e *Error) string {
	switch e.Kind {
	case KindUnsupported:
		return "등록되지 않은 reducer·schema·연산이라 replay를 실행하지 않았다. 지원하지 않는 판정을 성공으로 만들지 않는다."
	case KindInvalidInput:
		return "등록 문서나 입력이 계약을 어겨 replay를 실행하지 않았다."
	case KindCancelled:
		return "호출자가 취소해 replay 결과가 완결되지 않았다."
	case KindResourceLimit:
		return "replay 한도에 닿아 결과가 완결되지 않았다."
	}
	return "입출력 실패로 replay 결과가 완결되지 않았다."
}

func explainReplay(res ReplayResult) []string {
	out := []string{fmt.Sprintf("subject run %s(attempt %d, %s)의 기록 판정은 %s/%s이며 이 replay가 바꾸지 않는다.",
		res.Subject.Run, res.Subject.Attempt, res.Subject.Platform, res.Recorded.ExecutionStatus, res.Recorded.Assessment)}
	c := res.Consumption
	out = append(out, fmt.Sprintf("expected record %d개 중 %d개를 한 번씩 소비했다(누락 %d, 중복 %d, 미사용 %d, 순서 오류 %d, 다른 cohort 제외 %d).",
		c.Expected, c.Consumed, c.Missing, c.Duplicate, c.Unused, c.OutOfOrder, c.Excluded))
	for _, g := range res.Gates {
		switch g.EvidenceMode {
		case ModeReplayedRaw:
			out = append(out, fmt.Sprintf("gate %s: raw에서 다시 계산했다(%d건, 실패 %d) → %s.", g.ID, g.Recomputed, g.Failed, g.Assessment))
		default:
			out = append(out, fmt.Sprintf("gate %s: 다시 계산하지 않은 기록 %d건, 다시 계산 %d건(실패 %d) → %s.", g.ID, g.NotRecomputed, g.Recomputed, g.Failed, g.Assessment))
		}
	}
	if !res.EvidenceValid {
		out = append(out, "evidence가 등록·raw와 맞지 않아 이 replay는 FAIL이다. 첫 finding을 확인한다.")
	} else {
		out = append(out, fmt.Sprintf("다시 계산한 현재 판정은 %s/%s다.", res.Recomputed.ExecutionStatus, res.Recomputed.Assessment))
	}
	return out
}
