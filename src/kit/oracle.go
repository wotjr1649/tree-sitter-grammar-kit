package kit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"io"
	"io/fs"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Oracle schemas (Session 06): the profile, the per-case record and the completed set
// manifest written last. Profile r2 is r1 on tsgk-incremental/r2 (expectation anchors).
const (
	OracleSchema         = "tsgk-oracle/r2"
	OracleRecordSchema   = "tsgk-oracle-record/r1"
	OracleManifestSchema = "tsgk-oracle-manifest/r1"
	// OracleSourceManifestSchema is an opt-in set retaining exact initial source bytes.
	OracleSourceManifestSchema = "tsgk-oracle-manifest/r2"
	MaxOracleSourceBytes       = 32 << 20
	// A set run through batch driver processes (#266): each r2 record names its batch and
	// frame instead of a process, and the r3 manifest lists every batch process once.
	OracleBatchRecordSchema       = "tsgk-oracle-record/r2"
	OracleBatchManifestSchema     = "tsgk-oracle-manifest/r3"
	OracleAcceptedRecordSchema    = "tsgk-oracle-record/r3"
	OracleAcceptedManifestSchema  = "tsgk-oracle-manifest/r4"
	OracleSemanticsRecordSchema   = "tsgk-oracle-record/r4"
	OracleSemanticsManifestSchema = "tsgk-oracle-manifest/r5"
	OracleSemanticsSchema         = "tsgk-oracle/r3"
)

// oracleSchemaR1 is still decoded (without anchors) so earlier evidence replays.
const oracleSchemaR1 = "tsgk-oracle/r1"

// MaxQueryBytes bounds one query source; MaxQueries bounds the queries of a profile.
const (
	MaxQueryBytes = 65536
	MaxQueries    = 16
)

func digestHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// OracleQuery is one query of an oracle profile: id and UTF-8 source text.
type OracleQuery struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

// FactPackRef binds a profile's pack queries to the pack file and route they came from.
type FactPackRef struct {
	Revision string `json:"revision"`
	SHA256   string `json:"sha256"`
	Route    string `json:"route"`
}

// CaptureExpectation is one expected capture: name, node type and the exact source text.
type CaptureExpectation struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Text string `json:"text"`
}

// QueryErrorExpectation is an expected query rejection: class and byte offset.
type QueryErrorExpectation struct {
	Type   string `json:"type"`
	Offset uint32 `json:"offset"`
}

// QueryExpectation is the registered result of one query on one step's incremental tree:
// its status and code and, for a completed query, the whole evaluated capture stream in
// order (nil does not assert captures), or the expected rejection of an invalid query.
type QueryExpectation struct {
	Query    string                 `json:"query"`
	Step     int                    `json:"step"`
	Status   string                 `json:"status"`
	Code     string                 `json:"code"`
	Captures *[]CaptureExpectation  `json:"captures"`
	Error    *QueryErrorExpectation `json:"error"`
}

// DynamicSQLExpectation is the registered dynamic SQL result of a case's step 0: every
// fact and every known-miss range, in order.
type DynamicSQLExpectation struct {
	Facts       []DynamicSQLFact `json:"facts"`
	KnownMisses []ByteSpan       `json:"known_misses"`
}

// OracleCase is one oracle case: the shared native case and its query and dynamic SQL
// expectations.
type OracleCase struct {
	IncrementalCase
	QueryExpect []QueryExpectation     `json:"query_expect"`
	DynamicSQL  *DynamicSQLExpectation `json:"dynamic_sql_expect"`
}

// OracleProfile is a decoded Oracle r1/r2 or semantics-aware r3 profile. Native holds the shared members (its
// Cases are the base of OracleCases, in the same order).
type OracleProfile struct {
	QuerySemanticsRegistrationSHA256 string             `json:"query_semantics_registration_sha256,omitempty"`
	Native                           IncrementalProfile `json:"-"`
	Queries                          []OracleQuery      `json:"queries"`
	FactPack                         *FactPackRef       `json:"fact_pack"`
	API                              bool               `json:"api"`
	OracleCases                      []OracleCase       `json:"cases"`
}

var queryStatus = map[string]bool{StatusCompleted: true, StatusResourceLimit: true, "INVALID_QUERY": true, StatusFailed: true, "UNSUPPORTED": true, StatusNotRun: true}

// ParseOracleProfile strictly decodes an Oracle r1/r2 or semantics-aware r3 profile: the incremental profile
// members (with a query operation), the queries, the optional fact pack binding, the API
// switch and per case the query and dynamic SQL expectations.
func ParseOracleProfile(data []byte) (OracleProfile, error) {
	p, e := parseOracle(data)
	if e != nil {
		return OracleProfile{}, e
	}
	return p, nil
}

func parseOracle(data []byte) (OracleProfile, *Error) {
	v, e := decodeStrict("oracle", data, MaxDocumentBytes)
	if e != nil {
		return OracleProfile{}, e
	}
	return parseOracleValue(data, v)
}

func parseOracleValue(data []byte, v *jv, checkpoint ...func() *Error) (OracleProfile, *Error) {
	var o OracleProfile
	topExtra := []string{"queries", "fact_pack", "api"}
	if schema := jvMember(v, "schema"); schema != nil && schema.s == OracleSemanticsSchema {
		topExtra = append(topExtra, "query_semantics_registration_sha256")
	}
	n, x, e := parseNativeValue(data, []string{OracleSchema, oracleSchemaR1, OracleSemanticsSchema}, "oracle", topExtra, []string{"query_expect", "dynamic_sql_expect"}, v, checkpoint...)
	if e != nil {
		return o, e
	}
	o.Native = n
	t := x.t
	if n.Schema == OracleSemanticsSchema {
		if o.QuerySemanticsRegistrationSHA256, e = hexDigest(t, x.top["query_semantics_registration_sha256"]); e != nil {
			return o, e
		}
	}
	qs, e := t.array(x.top["queries"])
	if e != nil {
		return o, e
	}
	if len(qs) > MaxQueries {
		return o, t.bad("QUERIES_COUNT_INVALID", x.top["queries"])
	}
	ids := map[string]string{}
	for _, qv := range qs {
		qm, e := t.object(qv, []string{"id", "source"})
		if e != nil {
			return o, e
		}
		var q OracleQuery
		if q.ID, e = t.str(qm["id"]); e != nil {
			return o, e
		} else if !validID(q.ID) || ids[q.ID] != "" {
			return o, t.bad("QUERY_ID_INVALID", qm["id"])
		}
		if q.Source, e = t.str(qm["source"]); e != nil {
			return o, e
		} else if q.Source == "" || len(q.Source) > MaxQueryBytes {
			return o, t.bad("QUERY_SOURCE_INVALID", qm["source"])
		}
		ids[q.ID] = q.Source
		o.Queries = append(o.Queries, q)
	}
	if fv := x.top["fact_pack"]; fv.kind != 'n' {
		fm, e := t.object(fv, []string{"revision", "sha256", "route"})
		if e != nil {
			return o, e
		}
		var f FactPackRef
		if f.Revision, e = t.str(fm["revision"]); e != nil {
			return o, e
		} else if !validID(f.Revision) {
			return o, t.bad("FACT_PACK_INVALID", fm["revision"])
		}
		if f.SHA256, e = hexDigest(t, fm["sha256"]); e != nil {
			return o, e
		}
		if f.Route, e = t.str(fm["route"]); e != nil {
			return o, e
		} else if f.Route != n.Route {
			return o, t.bad("FACT_PACK_ROUTE", fm["route"])
		}
		o.FactPack = &f
	}
	if o.API, e = t.boolean(x.top["api"]); e != nil {
		return o, e
	}
	for i, c := range n.Cases {
		oc := OracleCase{IncrementalCase: c}
		cm := x.cases[i]
		list, e := t.array(cm["query_expect"])
		if e != nil {
			return o, e
		}
		for _, ev := range list {
			em, e := t.object(ev, []string{"query", "step", "status", "code", "captures", "error"})
			if e != nil {
				return o, e
			}
			var q QueryExpectation
			if q.Query, e = t.str(em["query"]); e != nil {
				return o, e
			} else if ids[q.Query] == "" {
				return o, t.bad("EXPECT_QUERY_UNKNOWN", em["query"])
			}
			step, e := t.uint(em["step"])
			if e != nil {
				return o, e
			} else if step > uint64(len(c.Edits)) {
				return o, t.bad("EXPECT_STEP_INVALID", em["step"])
			}
			q.Step = int(step)
			if q.Status, e = t.str(em["status"]); e != nil {
				return o, e
			} else if !queryStatus[q.Status] {
				return o, t.bad("EXPECT_STATUS_INVALID", em["status"])
			}
			if q.Code, e = t.str(em["code"]); e != nil {
				return o, e
			}
			if cv := em["captures"]; cv.kind != 'n' {
				items, e := t.array(cv)
				if e != nil {
					return o, e
				}
				caps := []CaptureExpectation{}
				for _, iv := range items {
					im, e := t.object(iv, []string{"name", "type", "text"})
					if e != nil {
						return o, e
					}
					var ce CaptureExpectation
					for _, f := range []struct {
						dst  *string
						name string
					}{{&ce.Name, "name"}, {&ce.Type, "type"}, {&ce.Text, "text"}} {
						if *f.dst, e = t.str(im[f.name]); e != nil {
							return o, e
						}
					}
					caps = append(caps, ce)
				}
				q.Captures = &caps
			}
			if ev := em["error"]; ev.kind != 'n' {
				xm, e := t.object(ev, []string{"type", "offset"})
				if e != nil {
					return o, e
				}
				var qe QueryErrorExpectation
				if qe.Type, e = t.str(xm["type"]); e != nil {
					return o, e
				}
				off, e := t.uint(xm["offset"])
				if e != nil {
					return o, e
				}
				if off > uint64(len(ids[q.Query])) {
					return o, t.bad("EXPECT_QUERY_OFFSET_INVALID", xm["offset"])
				}
				qe.Offset = uint32(off)
				q.Error = &qe
			}
			if (q.Error != nil) != (q.Status == "INVALID_QUERY") || (q.Captures != nil && q.Status != StatusCompleted) {
				return o, t.bad("EXPECT_SHAPE_INVALID", ev)
			}
			oc.QueryExpect = append(oc.QueryExpect, q)
		}
		if dv := cm["dynamic_sql_expect"]; dv.kind != 'n' {
			d, e := parseDynamicSQLExpect(t, dv, c.Input.Bytes)
			if e != nil {
				return o, e
			}
			if o.FactPack == nil {
				return o, t.bad("EXPECT_DYNAMIC_SQL_UNMAPPED", dv)
			}
			oc.DynamicSQL = d
		}
		o.OracleCases = append(o.OracleCases, oc)
	}
	return o, nil
}

func parseDynamicSQLExpect(t typed, v *jv, inputBytes uint64) (*DynamicSQLExpectation, *Error) {
	m, e := t.object(v, []string{"facts", "known_misses"})
	if e != nil {
		return nil, e
	}
	d := &DynamicSQLExpectation{Facts: []DynamicSQLFact{}, KnownMisses: []ByteSpan{}}
	facts, e := t.array(m["facts"])
	if e != nil {
		return nil, e
	}
	point := func(v *jv) (Point, *Error) {
		pm, e := t.object(v, []string{"row", "column"})
		if e != nil {
			return Point{}, e
		}
		r, e := t.uint(pm["row"])
		if e != nil {
			return Point{}, e
		}
		c, e := t.uint(pm["column"])
		if e != nil {
			return Point{}, e
		}
		if r > 0xffffffff || c > 0xffffffff || r > inputBytes || c > inputBytes {
			return Point{}, t.bad("EXPECT_DYNAMIC_POINT_INVALID", v)
		}
		return Point{Row: uint32(r), Column: uint32(c)}, nil
	}
	span := func(m map[string]*jv) (uint32, uint32, *Error) {
		s, e := t.uint(m["start_byte"])
		if e != nil {
			return 0, 0, e
		}
		en, e := t.uint(m["end_byte"])
		if e != nil {
			return 0, 0, e
		}
		if s > en || en > 0xffffffff || en > inputBytes {
			return 0, 0, t.bad("EXPECT_DYNAMIC_RANGE_INVALID", m["end_byte"])
		}
		return uint32(s), uint32(en), nil
	}
	for _, fv := range facts {
		fm, e := t.object(fv, []string{"construct", "argument_kind", "start_byte", "end_byte", "start_point", "end_point", "variable", "heuristic"})
		if e != nil {
			return nil, e
		}
		var f DynamicSQLFact
		if f.Construct, e = t.str(fm["construct"]); e != nil {
			return nil, e
		}
		if f.ArgumentKind, e = t.str(fm["argument_kind"]); e != nil {
			return nil, e
		}
		if f.StartByte, f.EndByte, e = span(fm); e != nil {
			return nil, e
		}
		if f.StartPoint, e = point(fm["start_point"]); e != nil {
			return nil, e
		}
		if f.EndPoint, e = point(fm["end_point"]); e != nil {
			return nil, e
		}
		if f.StartPoint.Row > f.EndPoint.Row || (f.StartPoint.Row == f.EndPoint.Row && f.StartPoint.Column > f.EndPoint.Column) {
			return nil, t.bad("EXPECT_DYNAMIC_POINT_INVALID", fm["end_point"])
		}
		if vv := fm["variable"]; vv.kind != 'n' {
			s, e := t.str(vv)
			if e != nil {
				return nil, e
			}
			f.Variable = &s
		}
		if f.Heuristic, e = t.boolean(fm["heuristic"]); e != nil {
			return nil, e
		}
		d.Facts = append(d.Facts, f)
	}
	misses, e := t.array(m["known_misses"])
	if e != nil {
		return nil, e
	}
	for _, kv := range misses {
		km, e := t.object(kv, []string{"start_byte", "end_byte"})
		if e != nil {
			return nil, e
		}
		var s ByteSpan
		if s.StartByte, s.EndByte, e = span(km); e != nil {
			return nil, e
		}
		d.KnownMisses = append(d.KnownMisses, s)
	}
	return d, nil
}

// OracleMember is one file of a record set: a case record or the raw driver response of a
// case, with its own identity and the case input identity it is bound to.
type OracleMember struct {
	Path            string `json:"path"`
	Role            string `json:"role"` // record | raw | source (manifest r2 only)
	Case            string `json:"case"`
	Bytes           uint64 `json:"bytes"`
	SHA256          string `json:"sha256"`
	InputBytes      uint64 `json:"input_bytes"`
	InputSHA256     string `json:"input_sha256"`
	ExecutionStatus string `json:"execution_status"`
}

// OracleManifest is the completeness inventory of a record set. It is written last; its
// final member complete is the set's completeness marker.
// OracleQuerySemantics preserves producer-local pending evidence, never external approval.
type OracleQuerySemantics struct {
	Phase              string        `json:"phase"`
	Claim              string        `json:"claim"`
	Code               string        `json:"code"`
	RegistrationSHA256 string        `json:"registration_sha256"`
	Capture            *ReplayMember `json:"capture"`
	Projection         ReplayMember  `json:"projection"`
}

type OracleManifest struct {
	QuerySemanticsRegistrationSHA256 string            `json:"query_semantics_registration_sha256,omitempty"`
	Operation                        *NativeOperation  `json:"operation,omitempty"`
	ReuseAcceptedPolicy              string            `json:"reuse_accepted_policy,omitempty"`
	ReuseAcceptedLedgerSHA256        string            `json:"reuse_accepted_ledger_sha256,omitempty"`
	Schema                           string            `json:"schema"`
	Workload                         map[string]string `json:"workload"` // profile id and sha256, route, operation, output
	Producer                         map[string]string `json:"producer"` // build identity, executable sha256, compiler, runtime, platform
	Policy                           IdentityRef       `json:"policy"`
	Protocol                         string            `json:"protocol"`
	Comparators                      []string          `json:"comparators"`
	Queries                          []IdentityRef     `json:"queries"`
	FactPack                         *FactPackRef      `json:"fact_pack"`
	ExecutionStatus                  string            `json:"execution_status"`
	Assessment                       string            `json:"assessment"`
	Records                          int               `json:"records"`
	Cases                            []string          `json:"cases"` // the profile's case ids in order
	Members                          []OracleMember    `json:"members"`
	Batches                          []OracleBatch     `json:"batches,omitzero"` // manifest r3 only: [] when no process ran
	Complete                         bool              `json:"complete"`
}

// OracleBatch is one batch driver process of a manifest r3 set: the cases sent to it in
// frame order and the facts of the process. ExitCode is the natural exit code, 0 when a
// limit or a signal ended the process (Status and Reason say which).
type OracleBatch struct {
	Index           int      `json:"index"`
	Cases           []string `json:"cases"`
	Status          string   `json:"status"`
	Reason          string   `json:"reason"`
	ExitCode        int      `json:"exit_code"`
	WallMillis      int64    `json:"wall_ms"`
	PeakBytes       uint64   `json:"peak_bytes"`
	CleanupVerified bool     `json:"cleanup_verified"`
	TrailingBytes   int64    `json:"trailing_bytes"`
}

// Accepted reports whether the process may vouch for its answered frames: it exited 0 by
// itself (status COMPLETED, so not a signal), cleanup is verified and nothing followed the
// last response. Any other end accepts none: a limit, cancellation or crash, STATE_LEAK, the
// exit 3 of a fatal resource response, or a process that did not exit after its last frame.
// A batch that left a sent frame unanswered accepts none either (checkBatches, live run).
func (b OracleBatch) Accepted() bool {
	return b.Status == "COMPLETED" && b.Reason == "EXITED" && b.ExitCode == 0 && b.CleanupVerified && b.TrailingBytes == 0
}

// recordTransport is what an r2 record says about its frame; Sent is false for a case that
// no batch process received. Fail is a completed case's own FAIL, kept even when its batch
// is not accepted: a failure is never a support claim.
type recordTransport struct {
	Semantics    *OracleQuerySemantics
	Sent         bool
	Batch, Frame uint64
	Answered     bool
	Fail         bool
}

// checkBatches checks a manifest r3 set's records against its batches: every sent case
// names a frame of a listed batch that holds that case, a raw response exists exactly for
// the answered cases, and a completed case that is not a FAIL was answered by an accepted
// batch without an unanswered sent frame, with a raw response that answers that case.
// rawIDs maps a case to the id its raw response answers; nil skips that comparison (the r4
// qualification binds the responses through the observations).
func checkBatches(m OracleManifest, transports map[string]recordTransport, statuses map[string]string, raws map[string]int, rawIDs map[string]string, add func(code, path, msg string)) {
	for i, b := range m.Batches {
		if b.Index != i || len(b.Cases) == 0 || slices.ContainsFunc(b.Cases, func(c string) bool { return !slices.Contains(m.Cases, c) }) {
			add("BATCH_INVALID", "manifest.json", fmt.Sprintf("batch %d의 순번이나 사례 목록이 틀렸다", i))
		}
	}
	unanswered := map[uint64]bool{}
	for _, t := range transports {
		if t.Sent && !t.Answered {
			unanswered[t.Batch] = true
		}
	}
	for _, c := range m.Cases {
		t, known := transports[c]
		if !known {
			continue // the record itself is reported missing
		}
		if t.Sent && (t.Batch >= uint64(len(m.Batches)) || t.Frame >= uint64(len(m.Batches[t.Batch].Cases)) || m.Batches[t.Batch].Cases[t.Frame] != c) {
			add("BATCH_FRAME_MISMATCH", "manifest.json", "record의 batch frame이 manifest batch와 다르다: "+c)
			continue
		}
		if (raws[c] == 1) != t.Answered || raws[c] > 1 {
			add("BATCH_RAW_MISMATCH", "manifest.json", "응답한 frame에만 raw 응답이 하나 있어야 한다: "+c)
		}
		if statuses[c] == StatusCompleted && !t.Fail && !(t.Answered && m.Batches[t.Batch].Accepted() && !unanswered[t.Batch]) {
			add("BATCH_SUCCESS_UNACCEPTED", "manifest.json", "완료 사례는 정상 종료한 batch의 응답이어야 한다: "+c)
		}
		if statuses[c] == StatusCompleted && rawIDs != nil && rawIDs[c] != c {
			add("BATCH_RAW_MISMATCH", "manifest.json", "완료 사례의 raw 응답이 그 사례의 응답이 아니다: "+c)
		}
	}
}

// OracleSetReport is the result of VerifyOracleSet. Valid means the set is complete and
// intact; whether its run succeeded is ExecutionStatus and Assessment, so a valid set of a
// failed run is a complete record of that failure, not a reference of passing results.
type OracleSetReport struct {
	Valid           bool      `json:"valid"`
	Records         int       `json:"records"`
	ExecutionStatus string    `json:"execution_status"`
	Assessment      string    `json:"assessment"`
	Findings        []Finding `json:"findings"`
}

// memberPath names a record or raw response by case position and id; an r4 SVC native
// segment's raw response adds its start step after the full-length id (#217).
var memberPath = regexp.MustCompile(`^(?:(?:records|raw)/[0-9]{5}-[A-Za-z0-9._-]{1,128}|raw/[0-9]{5}-[A-Za-z0-9._-]{1,128}-segment-[0-9]{5})\.json$`)
var sourceMemberPath = regexp.MustCompile(`^source/[0-9]{5}-[A-Za-z0-9._-]{1,128}\.bin$`)

// VerifyOracleSet verifies a published record set offline: the manifest decodes strictly
// and ends with complete: true, every listed member exists with its recorded bytes and
// sha256, no file is unlisted, the record count matches, and each record is a complete
// record of its case whose input and first step bind to the member's input identity. Any
// finding makes the set invalid; nothing is repaired.
func VerifyOracleSet(fsys fs.FS) OracleSetReport {
	r := OracleSetReport{Findings: []Finding{}}
	var semanticB *eligibilityBudget
	limitError := errors.New("oracle member bytes limit")
	read := func(path string, limit int64) ([]byte, error) {
		f, err := fsys.Open(path)
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(f, limit+1))
		err = errors.Join(err, f.Close())
		if int64(len(b)) > limit {
			return nil, errors.Join(err, limitError)
		}
		return b, err
	}
	add := func(code, path, msg string) {
		if semanticB != nil && len(r.Findings) >= 1000 {
			r.Findings[999] = Finding{Code: code, Severity: "error", Path: path, Message: msg}
			return
		}
		r.Findings = append(r.Findings, Finding{Code: code, Severity: "error", Path: path, Message: msg})
	}
	manifestStart := time.Now()
	data, err := read("manifest.json", MaxDocumentBytes)
	if err != nil {
		code := "MANIFEST_MISSING"
		if errors.Is(err, limitError) {
			code = "MANIFEST_BYTES_LIMIT"
		}
		add(code, "manifest.json", "완결 표시인 manifest를 제한 안에서 읽지 못했다")
		return r
	}
	var semanticFiles map[string][]byte
	var semanticRecord *jv
	var m OracleManifest
	if !utf8.Valid(data) {
		add("JSON_INVALID_UTF8", "manifest", "manifest가 UTF-8이 아니다")
		return r
	}
	selected, selectionErr := selectJSONRevision(data, nil, "manifest", "schema", OracleSemanticsManifestSchema, nil)
	if selectionErr != nil && selectionErr.Kind != KindInvalidInput {
		add(selectionErr.Code, selectionErr.Path, "manifest revision을 선택하지 못했다")
		return r
	}
	if selected == OracleSemanticsManifestSchema {
		lim := ReplayOperations()["evidence-replay"]
		ctx, cancel := context.WithTimeout(context.Background(), lim.Wall+time.Second)
		defer cancel()
		run, e := startRun(ctx, lim.Wall-time.Since(manifestStart))
		if e != nil {
			add(e.Code, e.Path, "검증 예산을 시작하지 못했다")
			return r
		}
		defer run.cancel()
		semanticB = &eligibilityBudget{r: run}
		run.eligibility = semanticB
		v, e := semanticB.semanticAST("manifest", data)
		if e != nil {
			add(e.Code, e.Path, "manifest 해석 예산을 넘었다")
			return r
		}
		significant, sizeErr := semanticB.significantJSONBytes(data)
		if sizeErr != nil {
			add(sizeErr.Code, sizeErr.Path, "manifest 해석을 중단했다")
			return r
		}
		if e := semanticB.charge(8*significant, 16*uint64(len(data)), "semantic-manifest-typed"); e != nil {
			add(e.Code, e.Path, "manifest 해석 예산을 넘었다")
			return r
		}
		if e := decodeOracleManifestValue(data, &m, v, typed{doc: "manifest", check: run.check}, OracleSemanticsManifestSchema); e != nil {
			add(e.Code, e.Path, "manifest 형식이 틀렸다")
			return r
		}
	} else if e := decodeOracleManifestVersion(data, &m, OracleManifestSchema, OracleSourceManifestSchema, OracleBatchManifestSchema, OracleAcceptedManifestSchema); e != nil {
		add(e.Code, e.Path, "manifest가 완결되지 않았거나 형식이 틀렸다")
		return r
	}
	if semanticB != nil {
		lim := ReplayOperations()["evidence-replay"]
		var total = uint64(len(data))
		if uint64(len(m.Members))+1 > lim.Files {
			add("SEMANTICS_SET_LIMIT", "manifest.json", "member 수 한도를 넘었다")
			return r
		}
		for _, mem := range m.Members {
			if mem.Bytes > lim.FileBytes || total > lim.TotalBytes-mem.Bytes {
				add("SEMANTICS_SET_LIMIT", mem.Path, "member 또는 전체 bytes 한도를 넘었다")
				return r
			}
			total += mem.Bytes
		}
		if e := semanticB.charge(uint64(len(m.Members))+1, 512*uint64(len(m.Members))+8*uint64(len(data)), "semantics-set-members"); e != nil {
			add(e.Code, e.Path, "검증 예산을 넘었다")
			return r
		}
		semanticFiles = map[string][]byte{}
	}
	r.ExecutionStatus, r.Assessment = m.ExecutionStatus, m.Assessment
	listed := map[string]bool{"manifest.json": true}
	records := 0
	perCase := map[string]int{}
	sources := map[string]int{}
	var accepted *ReuseAcceptedLedger
	recordInputs := map[string]OracleMember{}
	transports, statuses, raws, rawIDs := map[string]recordTransport{}, map[string]string{}, map[string]int{}, map[string]string{}
	caseOrdinals := map[string]int{}
	for i, c := range m.Cases {
		caseOrdinals[c] = i
	}
	for _, mem := range m.Members {
		if m.Schema == OracleAcceptedManifestSchema && mem.Role == "reuse-ledger" {
			if accepted != nil || mem.Path != "reuse-ledger.json" || mem.Case != "" || mem.Bytes > MaxDocumentBytes || mem.InputBytes != mem.Bytes || mem.InputSHA256 != mem.SHA256 || mem.ExecutionStatus != StatusCompleted {
				add("MEMBER_INVALID", mem.Path, "accepted 원장 member 결속이 틀렸다")
				continue
			}
			listed[mem.Path] = true
			b, err := read(mem.Path, int64(mem.Bytes))
			if err != nil || uint64(len(b)) != mem.Bytes || digestHex(b) != mem.SHA256 {
				add("MEMBER_MISMATCH", mem.Path, "accepted 원장 bytes가 다르다")
				continue
			}
			accepted, err = ParseReuseAcceptedLedger(b)
			if err != nil {
				add("REUSE_ACCEPTED_LEDGER_INVALID", mem.Path, "원장이 고정 등록과 다르다")
			}
			continue
		}
		dir := map[string]string{"record": "records/", "raw": "raw/"}[mem.Role]
		validPath := memberPath.MatchString(mem.Path)
		if m.Schema == OracleSourceManifestSchema || m.Schema == OracleAcceptedManifestSchema || m.Schema == OracleSemanticsManifestSchema {
			// the producer names every member by its case's position in cases
			ordinal, known := caseOrdinals[mem.Case]
			if mem.Role == "source" {
				dir, validPath = "source/", known && sourceMemberPath.MatchString(mem.Path) && mem.Path == fmt.Sprintf("source/%05d-%s.bin", ordinal, mem.Case)
			} else {
				validPath = validPath && known && mem.Path == fmt.Sprintf("%s%05d-%s.json", dir, ordinal, mem.Case)
			}
		}
		if m.Schema == OracleSemanticsManifestSchema {
			switch mem.Role {
			case "capture", "stdin-frame", "process", "stdout-frame", "stderr":
				dir, validPath = "capture/", portable(mem.Path, false) && caseOrdinals[mem.Case] == 0 && len(m.Cases) == 1 && mem.Case == m.Cases[0]
			case "projection":
				dir, validPath = "projection/", mem.Case == m.Cases[0] && mem.Path == "projection/00000-"+mem.Case+".json"
			case "driver", "build-input", "grammar-parser", "grammar-scanner", "grammar-header", "build", "build-manifest":
				dir, validPath = "capture/", portable(mem.Path, false) && mem.Case == "" && mem.InputBytes == mem.Bytes && mem.InputSHA256 == mem.SHA256 && mem.ExecutionStatus == StatusCompleted
			}
		}
		if !validPath || dir == "" || !strings.HasPrefix(mem.Path, dir) || listed[mem.Path] {
			add("MEMBER_INVALID", mem.Path, "member 경로·역할이 틀렸거나 중복이다")
			continue
		}
		listed[mem.Path] = true
		limit := uint64(MaxDocumentBytes * 4)
		if m.Schema == OracleSemanticsManifestSchema {
			limit = MaxDocumentBytes
		}
		if mem.Role == "source" {
			limit = MaxOracleSourceBytes
			if mem.Bytes > MaxOracleSourceBytes {
				add("SOURCE_BYTES_LIMIT", mem.Path, "source witness 크기 한도를 넘었다")
				continue
			}
		}
		if mem.Bytes > limit {
			add("MEMBER_BYTES_LIMIT", mem.Path, "member 크기 한도를 넘었다")
			continue
		}
		if semanticB != nil {
			if e := semanticB.charge(1, mem.Bytes, mem.Path); e != nil {
				add(e.Code, e.Path, "검증 예산을 넘었다")
				return r
			}
		}
		b, err := read(mem.Path, int64(mem.Bytes))
		if err != nil {
			code := "MEMBER_MISSING"
			if errors.Is(err, limitError) {
				code = "MEMBER_BYTES_LIMIT"
			}
			add(code, mem.Path, "member 파일을 제한 안에서 읽지 못했다")
			continue
		}
		if uint64(len(b)) != mem.Bytes || digestHex(b) != mem.SHA256 {
			add("MEMBER_MISMATCH", mem.Path, "member bytes 또는 sha256이 manifest와 다르다")
			continue
		}
		if semanticFiles != nil {
			semanticFiles[mem.Path] = b
		}
		if mem.Role == "record" {
			records++
			perCase[mem.Case]++
			recordInputs[mem.Case] = mem
			var tr recordTransport
			var code string
			if semanticB != nil {
				v, e := semanticB.semanticAST(mem.Path, b)
				if e != nil {
					add(e.Code, e.Path, "record 해석 예산을 넘었다")
					return r
				}
				semanticRecord = v
				tr, code = checkRecordValue(typed{doc: mem.Path, check: semanticB.r.check}, v, mem, m.Schema)
			} else {
				tr, code = checkRecord(b, mem, m.Schema)
			}
			if code != "" {
				add(code, mem.Path, "record가 완결되지 않았거나 member identity와 다르다")
			} else {
				transports[mem.Case], statuses[mem.Case] = tr, mem.ExecutionStatus
			}
		}
		if mem.Role == "raw" {
			raws[mem.Case]++
			if m.Schema != OracleSemanticsManifestSchema {
				rawIDs[mem.Case] = responseID(b)
			}
		}
		if mem.Role == "source" {
			sources[mem.Case]++
			if mem.Bytes != mem.InputBytes || mem.SHA256 != mem.InputSHA256 {
				add("SOURCE_INPUT_MISMATCH", mem.Path, "source witness와 사례 입력 identity가 다르다")
			}
		}
	}
	if m.Schema == OracleSemanticsManifestSchema {
		pins := map[string]OracleMember{}
		for _, mem := range m.Members {
			pins[mem.Path] = mem
		}
		for id, tr := range transports {
			if tr.Semantics == nil || tr.Semantics.RegistrationSHA256 != m.QuerySemanticsRegistrationSHA256 {
				add("SEMANTICS_LOCAL_BINDING", id, "local semantic 등록 결속이 다르다")
				continue
			}
			refs := []ReplayMember{tr.Semantics.Projection}
			if tr.Semantics.Capture != nil {
				refs = append(refs, *tr.Semantics.Capture)
			}
			for _, pin := range refs {
				mem, ok := pins[pin.Path]
				if !ok || mem.Case != id || mem.Role != pin.Role || mem.Bytes != pin.Bytes || mem.SHA256 != pin.SHA256 {
					add("SEMANTICS_LOCAL_PIN", pin.Path, "local semantic pin의 소유 member가 다르다")
				}
			}
		}
	}
	if semanticB != nil {
		if e := semanticB.checkSemanticSet(m, semanticFiles, semanticRecord, nil, add); e != nil {
			add(e.Code, e.Path, "semantic set 검증을 끝내지 못했다")
		}
	}
	if records != m.Records || len(m.Cases) != m.Records {
		add("RECORD_COUNT_MISMATCH", "manifest.json", "record 수가 manifest와 다르다")
	}
	for _, c := range m.Cases {
		if perCase[c] != 1 {
			add("CASE_RECORD_MISMATCH", "manifest.json", "사례마다 record가 정확히 하나여야 한다: "+c)
		}
		if (m.Schema == OracleSourceManifestSchema || m.Schema == OracleAcceptedManifestSchema || m.Schema == OracleSemanticsManifestSchema) && sources[c] != 1 {
			add("CASE_SOURCE_MISMATCH", "manifest.json", "사례마다 source witness가 정확히 하나여야 한다: "+c)
		}
	}
	if m.Schema == OracleBatchManifestSchema {
		checkBatches(m, transports, statuses, raws, rawIDs, add)
	}
	if m.Schema == OracleAcceptedManifestSchema && accepted == nil {
		add("REUSE_ACCEPTED_LEDGER_MISSING", "manifest.json", "고정 원장 member가 없다")
	}
	if m.Schema == OracleSourceManifestSchema || m.Schema == OracleAcceptedManifestSchema || m.Schema == OracleSemanticsManifestSchema {
		if len(m.Cases) == 0 {
			add("CASE_SOURCE_EMPTY", "manifest.json", "source witness set에 사례가 없다")
		}
		for _, mem := range m.Members {
			if m.Schema == OracleSemanticsManifestSchema && mem.Case == "" {
				continue
			}
			if m.Schema == OracleAcceptedManifestSchema && mem.Role == "reuse-ledger" {
				continue
			}
			rec, ok := recordInputs[mem.Case]
			if !slices.Contains(m.Cases, mem.Case) || !ok || mem.InputBytes != rec.InputBytes || mem.InputSHA256 != rec.InputSHA256 || mem.ExecutionStatus != rec.ExecutionStatus {
				add("CASE_MEMBER_MISMATCH", mem.Path, "member의 사례·입력·상태가 record와 다르다")
			}
		}
	}
	if semanticB != nil {
		if e := semanticB.walkSemanticSet(fsys, listed, add); e != nil {
			add(e.Code, e.Path, "set 순회 예산을 넘었거나 읽을 수 없다")
		}
	} else {
		_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				add("SET_UNREADABLE", path, "set을 읽지 못했다")
				return nil
			}
			if !d.IsDir() && !listed[path] {
				add("MEMBER_UNLISTED", path, "manifest에 없는 파일이 있다")
			}
			return nil
		})
	}

	r.Records = records
	r.Valid = len(r.Findings) == 0
	return r
}

// responseID is the request id a raw driver response answers ("" when it has none).
func responseID(raw []byte) string {
	var r struct {
		ID string `json:"id"`
	}
	if jsonv2.Unmarshal(raw, &r) != nil {
		return ""
	}
	return r.ID
}

func decodeOracleManifest(data []byte, m *OracleManifest) *Error {
	// Legacy replay and qualification do not acquire source-witness semantics implicitly; they
	// read batch sets (#266) through the shared batch checks.
	return decodeOracleManifestVersion(data, m, OracleManifestSchema, OracleBatchManifestSchema)
}

func decodeOracleManifestVersion(data []byte, m *OracleManifest, allowed ...string) *Error {
	t := typed{doc: "manifest"}
	v, e := decodeStrict(t.doc, data, MaxDocumentBytes)
	if e != nil {
		return e
	}
	return decodeOracleManifestValue(data, m, v, t, allowed...)
}

func decodeOracleManifestValue(data []byte, m *OracleManifest, v *jv, t typed, allowed ...string) *Error {
	var e *Error
	keys := []string{"schema", "workload", "producer", "policy", "protocol", "comparators", "queries", "fact_pack", "execution_status", "assessment", "records", "cases", "members", "complete"}
	if s := jvMember(v, "schema"); s != nil && s.kind == '"' && s.s == OracleAcceptedManifestSchema {
		keys = slices.Insert(keys, len(keys)-1, "reuse_accepted_policy", "reuse_accepted_ledger_sha256", "operation")
	}
	if s := jvMember(v, "schema"); s != nil && s.kind == '"' && s.s == OracleSemanticsManifestSchema {
		keys = slices.Insert(keys, len(keys)-1, "query_semantics_registration_sha256", "operation")
	}
	if s := jvMember(v, "schema"); s != nil && s.kind == '"' && s.s == OracleBatchManifestSchema {
		keys = slices.Insert(keys, len(keys)-1, "batches")
	}
	mm, e := t.object(v, keys)
	if e != nil {
		return e
	}
	// the completeness marker is the last member, written after everything it covers
	if len(v.keys) != len(keys) || v.keys[len(v.keys)-1] != "complete" || mm["complete"].kind != 't' {
		return t.bad("SET_INCOMPLETE", v)
	}
	if s, e := t.str(mm["schema"]); e != nil {
		return e
	} else if !slices.Contains(allowed, s) {
		return t.bad("SCHEMA_UNSUPPORTED", mm["schema"])
	} else {
		m.Schema = s
	}
	if m.Schema == OracleSemanticsManifestSchema {
		if m.QuerySemanticsRegistrationSHA256, e = hexDigest(t, mm["query_semantics_registration_sha256"]); e != nil {
			return e
		}
		if e := witnessShape(t, mm["operation"], reflect.TypeFor[NativeOperation](), true, false, nil); e != nil {
			return e
		}
		var extra struct {
			Operation *NativeOperation `json:"operation"`
		}
		if err := json.Unmarshal(data, &extra); err != nil || extra.Operation == nil {
			return t.bad("SEMANTICS_OPERATION_INVALID", mm["operation"])
		}
		m.Operation = extra.Operation
		if !reflect.DeepEqual(*m.Operation, NativeOperations()["native-query"]) {
			return t.bad("SEMANTICS_OPERATION_INVALID", mm["operation"])
		}
	}
	if m.Schema == OracleAcceptedManifestSchema {
		if e := witnessShape(t, mm["operation"], reflect.TypeFor[NativeOperation](), true, false, nil); e != nil {
			return e
		}
		var extra struct {
			Operation *NativeOperation `json:"operation"`
		}
		if err := json.Unmarshal(data, &extra); err != nil || extra.Operation == nil {
			return t.bad("REUSE_ACCEPTED_OPERATION_INVALID", mm["operation"])
		}
		m.Operation = extra.Operation
		reg := NativeOperations()["native-query"]
		op := *m.Operation
		op.RunWall = reg.RunWall
		if !reflect.DeepEqual(op, reg) || m.Operation.RunWall <= 0 || m.Operation.RunWall > reg.RunWall {
			return t.bad("REUSE_ACCEPTED_OPERATION_INVALID", mm["operation"])
		}
		if m.ReuseAcceptedPolicy, e = t.str(mm["reuse_accepted_policy"]); e != nil {
			return e
		}
		if m.ReuseAcceptedLedgerSHA256, e = t.str(mm["reuse_accepted_ledger_sha256"]); e != nil {
			return e
		}
		if m.ReuseAcceptedPolicy != ReuseAcceptedSchema || m.ReuseAcceptedLedgerSHA256 != ReuseAcceptedLedgerSHA256 {
			return t.bad("REUSE_ACCEPTED_POLICY_MISMATCH", v)
		}
	}
	for _, k := range []string{"workload", "producer"} {
		obj, e := t.object(mm[k], mm[k].keys)
		if e != nil {
			return e
		}
		vals := map[string]string{}
		for name, v := range obj {
			if vals[name], e = t.str(v); e != nil {
				return e
			}
		}
		if k == "workload" {
			m.Workload = vals
		} else {
			m.Producer = vals
		}
	}
	ref := func(v *jv) (IdentityRef, *Error) {
		rm, e := t.object(v, []string{"role", "schema", "sha256"})
		if e != nil {
			return IdentityRef{}, e
		}
		var r IdentityRef
		for _, f := range []struct {
			dst  *string
			name string
		}{{&r.Role, "role"}, {&r.Schema, "schema"}, {&r.SHA256, "sha256"}} {
			if *f.dst, e = t.str(rm[f.name]); e != nil {
				return r, e
			}
		}
		return r, nil
	}
	if m.Policy, e = ref(mm["policy"]); e != nil {
		return e
	}
	cs, e := t.array(mm["comparators"])
	if e != nil {
		return e
	}
	for _, cv := range cs {
		c, e := t.str(cv)
		if e != nil {
			return e
		}
		m.Comparators = append(m.Comparators, c)
	}
	qs, e := t.array(mm["queries"])
	if e != nil {
		return e
	}
	for _, qv := range qs {
		q, e := ref(qv)
		if e != nil {
			return e
		}
		m.Queries = append(m.Queries, q)
	}
	if fv := mm["fact_pack"]; fv.kind != 'n' {
		fm, e := t.object(fv, []string{"revision", "sha256", "route"})
		if e != nil {
			return e
		}
		var f FactPackRef
		for _, x := range []struct {
			dst  *string
			name string
		}{{&f.Revision, "revision"}, {&f.SHA256, "sha256"}, {&f.Route, "route"}} {
			if *x.dst, e = t.str(fm[x.name]); e != nil {
				return e
			}
		}
		m.FactPack = &f
	}
	for _, f := range []struct {
		dst  *string
		name string
	}{{&m.Protocol, "protocol"}, {&m.ExecutionStatus, "execution_status"}, {&m.Assessment, "assessment"}} {
		if *f.dst, e = t.str(mm[f.name]); e != nil {
			return e
		}
	}
	n, e := t.uint(mm["records"])
	if e != nil {
		return e
	}
	m.Records = int(min(n, 1<<31))
	ids, e := t.array(mm["cases"])
	if e != nil {
		return e
	}
	for _, iv := range ids {
		s, e := t.str(iv)
		if e != nil {
			return e
		}
		m.Cases = append(m.Cases, s)
	}
	list, e := t.array(mm["members"])
	if e != nil {
		return e
	}
	for _, iv := range list {
		im, e := t.object(iv, []string{"path", "role", "case", "bytes", "sha256", "input_bytes", "input_sha256", "execution_status"})
		if e != nil {
			return e
		}
		var mem OracleMember
		for _, f := range []struct {
			dst  *string
			name string
		}{{&mem.Path, "path"}, {&mem.Role, "role"}, {&mem.Case, "case"}, {&mem.ExecutionStatus, "execution_status"}} {
			if *f.dst, e = t.str(im[f.name]); e != nil {
				return e
			}
		}
		if mem.SHA256, e = hexDigest(t, im["sha256"]); e != nil {
			return e
		}
		if mem.InputSHA256, e = hexDigest(t, im["input_sha256"]); e != nil {
			return e
		}
		if mem.Bytes, e = t.uint(im["bytes"]); e != nil {
			return e
		}
		if mem.InputBytes, e = t.uint(im["input_bytes"]); e != nil {
			return e
		}
		m.Members = append(m.Members, mem)
	}
	if m.Schema == OracleBatchManifestSchema {
		bs, e := t.array(mm["batches"])
		if e != nil {
			return e
		}
		for _, bv := range bs {
			bm, e := t.object(bv, []string{"index", "cases", "status", "reason", "exit_code", "wall_ms", "peak_bytes", "cleanup_verified", "trailing_bytes"})
			if e != nil {
				return e
			}
			var b OracleBatch
			nums := map[string]uint64{}
			for _, name := range []string{"index", "exit_code", "wall_ms", "peak_bytes", "trailing_bytes"} {
				if nums[name], e = t.uint(bm[name]); e != nil {
					return e
				}
			}
			b.Index, b.ExitCode, b.WallMillis, b.PeakBytes, b.TrailingBytes = int(min(nums["index"], 1<<31)), int(min(nums["exit_code"], 1<<31)), int64(nums["wall_ms"]), nums["peak_bytes"], int64(nums["trailing_bytes"])
			if b.Status, e = t.str(bm["status"]); e != nil {
				return e
			}
			if b.Reason, e = t.str(bm["reason"]); e != nil {
				return e
			}
			if b.CleanupVerified, e = t.boolean(bm["cleanup_verified"]); e != nil {
				return e
			}
			cs, e := t.array(bm["cases"])
			if e != nil {
				return e
			}
			for _, cv := range cs {
				c, e := t.str(cv)
				if e != nil {
					return e
				}
				b.Cases = append(b.Cases, c)
			}
			m.Batches = append(m.Batches, b)
		}
	}
	if m.Schema == OracleAcceptedManifestSchema {
		if m.Protocol != nativewire.ProtocolR4 || m.Workload["operation"] != "native-query" || m.Workload["output"] != OutputTree || len(m.Queries) != 0 || m.FactPack != nil ||
			!slices.Contains([]string{"windows/amd64", "linux/amd64"}, m.Producer["platform"]) || m.Policy != NativePolicy(*m.Operation, OutputTree, nil) {
			return t.bad("REUSE_ACCEPTED_MANIFEST_INVALID", v)
		}
	}
	if m.Schema == OracleSemanticsManifestSchema {
		if m.Protocol != nativewire.ProtocolR4 || m.Workload["operation"] != "native-query" || m.Workload["output"] != OutputTree || len(m.Cases) != 1 || m.FactPack != nil || !slices.Contains([]string{"windows/amd64", "linux/amd64"}, m.Producer["platform"]) || m.Policy != NativePolicy(*m.Operation, OutputTree, nil) {
			return t.bad("SEMANTICS_MANIFEST_INVALID", v)
		}
	}
	m.Complete = true
	return nil
}

// checkRecord checks a record's own completeness and binding: schema, the final complete
// member, the case id, the input identity and, when steps exist, step 0's source identity.
func checkRecord(data []byte, mem OracleMember, manifestSchema string) (recordTransport, string) {
	var tr recordTransport
	t := typed{doc: "record"}
	v, e := decodeStrict(t.doc, data, MaxDocumentBytes*4)
	if e != nil {
		return tr, e.Code
	}
	return checkRecordValue(t, v, mem, manifestSchema)
}

func checkRecordValue(t typed, v *jv, mem OracleMember, manifestSchema string) (recordTransport, string) {
	tr, e := checkRecordValueError(t, v, mem, manifestSchema)
	if e != nil {
		return tr, e.Code
	}
	return tr, ""
}

func checkRecordValueError(t typed, v *jv, mem OracleMember, manifestSchema string) (recordTransport, *Error) {
	var tr recordTransport
	var e *Error
	if v.kind != '{' || len(v.keys) == 0 || v.keys[len(v.keys)-1] != "complete" || v.vals[len(v.vals)-1].kind != 't' {
		return tr, fail(KindInvalidInput, "RECORD_INCOMPLETE", t.doc, nil)
	}
	get := func(o *jv, k string) *jv {
		if o == nil || o.kind != '{' {
			return nil
		}
		if i := slices.Index(o.keys, k); i >= 0 {
			return o.vals[i]
		}
		return nil
	}
	str := func(o *jv) string {
		if o == nil || o.kind != '"' {
			return ""
		}
		return o.s
	}
	schema := OracleRecordSchema
	if manifestSchema == OracleBatchManifestSchema {
		schema = OracleBatchRecordSchema
	}
	if manifestSchema == OracleAcceptedManifestSchema {
		schema = OracleAcceptedRecordSchema
	}
	if manifestSchema == OracleSemanticsManifestSchema {
		schema = OracleSemanticsRecordSchema
	}
	if schema == OracleSemanticsRecordSchema {
		if get(v, "query_semantics") == nil {
			return tr, fail(KindInvalidInput, "JSON_MISSING_FIELD", t.doc, nil)
		}
		if tr.Semantics, e = semanticLocalValue(t, get(v, "query_semantics")); e != nil {
			return tr, e
		}
	} else if get(v, "query_semantics") != nil {
		return tr, fail(KindInvalidInput, "SEMANTICS_SCHEMA_UNSUPPORTED", t.doc, nil)
	}
	if schema != OracleAcceptedRecordSchema {
		for _, name := range []string{"reuse_accepted", "reuse_accepted_policy", "reuse_accepted_ledger_sha256"} {
			if get(v, name) != nil {
				return tr, fail(KindInvalidInput, "REUSE_ACCEPTED_SCHEMA_UNSUPPORTED", t.doc, nil)
			}
		}
	}
	if schema == OracleAcceptedRecordSchema {
		if get(v, "transport") != nil || get(v, "process") == nil {
			return tr, fail(KindInvalidInput, "RECORD_TRANSPORT_INVALID", t.doc, nil)
		}
		if e := checkAcceptedResultShape(t, get(v, "reuse_accepted")); e != nil {
			return tr, e
		}
	}
	if str(get(v, "schema")) != schema || str(get(v, "case")) != mem.Case {
		return tr, fail(KindInvalidInput, "RECORD_IDENTITY_MISMATCH", t.doc, nil)
	}
	in := get(v, "input")
	if str(get(in, "sha256")) != mem.InputSHA256 || get(in, "bytes") == nil || get(in, "bytes").s != itoa(mem.InputBytes) {
		return tr, fail(KindInvalidInput, "RECORD_INPUT_MISMATCH", t.doc, nil)
	}
	if str(get(v, "execution_status")) != mem.ExecutionStatus {
		return tr, fail(KindInvalidInput, "RECORD_STATUS_MISMATCH", t.doc, nil)
	}
	if steps := get(v, "steps"); steps != nil && steps.kind == '[' && len(steps.vals) > 0 {
		s0 := steps.vals[0]
		if str(get(s0, "source_sha256")) != mem.InputSHA256 || get(s0, "source_bytes") == nil || get(s0, "source_bytes").s != itoa(mem.InputBytes) {
			return tr, fail(KindInvalidInput, "RECORD_STEP_MISMATCH", t.doc, nil)
		}
	}
	// r2: a batch frame instead of a process; a case no batch received has a null transport
	transport := get(v, "transport")
	if schema == OracleRecordSchema || schema == OracleAcceptedRecordSchema || schema == OracleSemanticsRecordSchema {
		if transport != nil {
			return tr, fail(KindInvalidInput, "RECORD_TRANSPORT_INVALID", t.doc, nil)
		}
		return tr, nil
	}
	if p := get(v, "process"); p == nil || p.kind != 'n' || transport == nil {
		return tr, fail(KindInvalidInput, "RECORD_TRANSPORT_INVALID", t.doc, nil)
	}
	if transport.kind == 'n' {
		return tr, nil
	}
	tm, e := t.object(transport, []string{"batch", "frame", "state", "frame_wall_ms"})
	if e != nil {
		return tr, fail(KindInvalidInput, "RECORD_TRANSPORT_INVALID", t.doc, nil)
	}
	state, se := t.str(tm["state"])
	var be, fe, we *Error
	tr.Batch, be = t.uint(tm["batch"])
	tr.Frame, fe = t.uint(tm["frame"])
	_, we = t.uint(tm["frame_wall_ms"])
	if se != nil || be != nil || fe != nil || we != nil || state != "SENT" && state != "ANSWERED" {
		return tr, fail(KindInvalidInput, "RECORD_TRANSPORT_INVALID", t.doc, nil)
	}
	tr.Sent, tr.Answered, tr.Fail = true, state == "ANSWERED", str(get(v, "assessment")) == AssessFail
	return tr, nil
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// CaptureText is the exact source text of a capture, or an error when it is not UTF-8 (a
// capture expectation compares text and so applies to UTF-8 sources only).
func CaptureText(src []byte, c Capture) (string, error) {
	if int(c.EndByte) > len(src) || c.StartByte > c.EndByte {
		return "", errors.New("capture outside the source")
	}
	s := src[c.StartByte:c.EndByte]
	if !utf8.Valid(s) {
		return "", errors.New("capture text is not UTF-8")
	}
	return string(s), nil
}
