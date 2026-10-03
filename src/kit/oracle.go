package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Oracle schemas (Session 06): the profile, the per-case record and the completed set
// manifest written last.
const (
	OracleSchema         = "tsgk-oracle/r1"
	OracleRecordSchema   = "tsgk-oracle-record/r1"
	OracleManifestSchema = "tsgk-oracle-manifest/r1"
)

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

// OracleProfile is a decoded tsgk-oracle/r1 profile. Native holds the shared members (its
// Cases are the base of OracleCases, in the same order).
type OracleProfile struct {
	Native      IncrementalProfile `json:"-"`
	Queries     []OracleQuery      `json:"queries"`
	FactPack    *FactPackRef       `json:"fact_pack"`
	API         bool               `json:"api"`
	OracleCases []OracleCase       `json:"cases"`
}

var queryStatus = map[string]bool{StatusCompleted: true, StatusResourceLimit: true, "INVALID_QUERY": true, StatusFailed: true, "UNSUPPORTED": true, StatusNotRun: true}

// ParseOracleProfile strictly decodes a tsgk-oracle/r1 profile: the incremental profile
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
	var o OracleProfile
	n, x, e := parseNative(data, OracleSchema, "oracle", []string{"queries", "fact_pack", "api"}, []string{"query_expect", "dynamic_sql_expect"})
	if e != nil {
		return o, e
	}
	o.Native = n
	t := x.t
	qs, e := t.array(x.top["queries"])
	if e != nil {
		return o, e
	}
	if len(qs) > MaxQueries {
		return o, t.bad("QUERIES_COUNT_INVALID", x.top["queries"])
	}
	ids := map[string]bool{}
	for _, qv := range qs {
		qm, e := t.object(qv, []string{"id", "source"})
		if e != nil {
			return o, e
		}
		var q OracleQuery
		if q.ID, e = t.str(qm["id"]); e != nil {
			return o, e
		} else if !validID(q.ID) || ids[q.ID] {
			return o, t.bad("QUERY_ID_INVALID", qm["id"])
		}
		ids[q.ID] = true
		if q.Source, e = t.str(qm["source"]); e != nil {
			return o, e
		} else if q.Source == "" || len(q.Source) > MaxQueryBytes {
			return o, t.bad("QUERY_SOURCE_INVALID", qm["source"])
		}
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
			} else if !ids[q.Query] {
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
				qe.Offset = uint32(min(off, 1<<32-1))
				q.Error = &qe
			}
			if (q.Error != nil) != (q.Status == "INVALID_QUERY") || (q.Captures != nil && q.Status != StatusCompleted) {
				return o, t.bad("EXPECT_SHAPE_INVALID", ev)
			}
			oc.QueryExpect = append(oc.QueryExpect, q)
		}
		if dv := cm["dynamic_sql_expect"]; dv.kind != 'n' {
			d, e := parseDynamicSQLExpect(t, dv)
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

func parseDynamicSQLExpect(t typed, v *jv) (*DynamicSQLExpectation, *Error) {
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
		return Point{Row: uint32(min(r, 1<<32-1)), Column: uint32(min(c, 1<<32-1))}, e
	}
	span := func(m map[string]*jv) (uint32, uint32, *Error) {
		s, e := t.uint(m["start_byte"])
		if e != nil {
			return 0, 0, e
		}
		en, e := t.uint(m["end_byte"])
		return uint32(min(s, 1<<32-1)), uint32(min(en, 1<<32-1)), e
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
	Role            string `json:"role"` // record | raw
	Case            string `json:"case"`
	Bytes           uint64 `json:"bytes"`
	SHA256          string `json:"sha256"`
	InputBytes      uint64 `json:"input_bytes"`
	InputSHA256     string `json:"input_sha256"`
	ExecutionStatus string `json:"execution_status"`
}

// OracleManifest is the completeness inventory of a record set. It is written last; its
// final member complete is the set's completeness marker.
type OracleManifest struct {
	Schema          string            `json:"schema"`
	Workload        map[string]string `json:"workload"` // profile id and sha256, route, operation, output
	Producer        map[string]string `json:"producer"` // build identity, executable sha256, compiler, runtime, platform
	Policy          IdentityRef       `json:"policy"`
	Protocol        string            `json:"protocol"`
	Comparators     []string          `json:"comparators"`
	Queries         []IdentityRef     `json:"queries"`
	FactPack        *FactPackRef      `json:"fact_pack"`
	ExecutionStatus string            `json:"execution_status"`
	Assessment      string            `json:"assessment"`
	Records         int               `json:"records"`
	Cases           []string          `json:"cases"` // the profile's case ids in order
	Members         []OracleMember    `json:"members"`
	Complete        bool              `json:"complete"`
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

var memberPath = regexp.MustCompile(`^(records|raw)/[0-9]{5}-[A-Za-z0-9._-]{1,128}\.json$`)

// VerifyOracleSet verifies a published record set offline: the manifest decodes strictly
// and ends with complete: true, every listed member exists with its recorded bytes and
// sha256, no file is unlisted, the record count matches, and each record is a complete
// record of its case whose input and first step bind to the member's input identity. Any
// finding makes the set invalid; nothing is repaired.
func VerifyOracleSet(fsys fs.FS) OracleSetReport {
	r := OracleSetReport{Findings: []Finding{}}
	add := func(code, path, msg string) {
		r.Findings = append(r.Findings, Finding{Code: code, Severity: "error", Path: path, Message: msg})
	}
	data, err := fs.ReadFile(fsys, "manifest.json")
	if err != nil {
		add("MANIFEST_MISSING", "manifest.json", "완결 표시인 manifest가 없다")
		return r
	}
	var m OracleManifest
	if e := decodeOracleManifest(data, &m); e != nil {
		add(e.Code, e.Path, "manifest가 완결되지 않았거나 형식이 틀렸다")
		return r
	}
	r.ExecutionStatus, r.Assessment = m.ExecutionStatus, m.Assessment
	listed := map[string]bool{"manifest.json": true}
	records := 0
	perCase := map[string]int{}
	for _, mem := range m.Members {
		dir := map[string]string{"record": "records/", "raw": "raw/"}[mem.Role]
		if !memberPath.MatchString(mem.Path) || dir == "" || !strings.HasPrefix(mem.Path, dir) || listed[mem.Path] {
			add("MEMBER_INVALID", mem.Path, "member 경로·역할이 틀렸거나 중복이다")
			continue
		}
		listed[mem.Path] = true
		b, err := fs.ReadFile(fsys, mem.Path)
		if err != nil {
			add("MEMBER_MISSING", mem.Path, "member 파일이 없다")
			continue
		}
		if uint64(len(b)) != mem.Bytes || digestHex(b) != mem.SHA256 {
			add("MEMBER_MISMATCH", mem.Path, "member bytes 또는 sha256이 manifest와 다르다")
			continue
		}
		if mem.Role == "record" {
			records++
			perCase[mem.Case]++
			if code := checkRecord(b, mem); code != "" {
				add(code, mem.Path, "record가 완결되지 않았거나 member identity와 다르다")
			}
		}
	}
	if records != m.Records || len(m.Cases) != m.Records {
		add("RECORD_COUNT_MISMATCH", "manifest.json", "record 수가 manifest와 다르다")
	}
	for _, c := range m.Cases {
		if perCase[c] != 1 {
			add("CASE_RECORD_MISMATCH", "manifest.json", "사례마다 record가 정확히 하나여야 한다: "+c)
		}
	}
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
	r.Records = records
	r.Valid = len(r.Findings) == 0
	return r
}

func decodeOracleManifest(data []byte, m *OracleManifest) *Error {
	t := typed{doc: "manifest"}
	v, e := decodeStrict(t.doc, data, MaxDocumentBytes)
	if e != nil {
		return e
	}
	keys := []string{"schema", "workload", "producer", "policy", "protocol", "comparators", "queries", "fact_pack", "execution_status", "assessment", "records", "cases", "members", "complete"}
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
	} else if s != OracleManifestSchema {
		return t.bad("SCHEMA_UNSUPPORTED", mm["schema"])
	}
	for _, k := range []string{"workload", "producer"} {
		obj, e := t.object(mm[k], mm[k].keys)
		if e != nil {
			return e
		}
		for _, v := range obj {
			if _, e := t.str(v); e != nil {
				return e
			}
		}
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
	m.Complete = true
	return nil
}

// checkRecord checks a record's own completeness and binding: schema, the final complete
// member, the case id, the input identity and, when steps exist, step 0's source identity.
func checkRecord(data []byte, mem OracleMember) string {
	t := typed{doc: "record"}
	v, e := decodeStrict(t.doc, data, MaxDocumentBytes*4)
	if e != nil {
		return e.Code
	}
	if v.kind != '{' || len(v.keys) == 0 || v.keys[len(v.keys)-1] != "complete" || v.vals[len(v.vals)-1].kind != 't' {
		return "RECORD_INCOMPLETE"
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
	if str(get(v, "schema")) != OracleRecordSchema || str(get(v, "case")) != mem.Case {
		return "RECORD_IDENTITY_MISMATCH"
	}
	in := get(v, "input")
	if str(get(in, "sha256")) != mem.InputSHA256 || get(in, "bytes") == nil || get(in, "bytes").s != itoa(mem.InputBytes) {
		return "RECORD_INPUT_MISMATCH"
	}
	if str(get(v, "execution_status")) != mem.ExecutionStatus {
		return "RECORD_STATUS_MISMATCH"
	}
	if steps := get(v, "steps"); steps != nil && steps.kind == '[' && len(steps.vals) > 0 {
		s0 := steps.vals[0]
		if str(get(s0, "source_sha256")) != mem.InputSHA256 || get(s0, "source_bytes") == nil || get(s0, "source_bytes").s != itoa(mem.InputBytes) {
			return "RECORD_STEP_MISMATCH"
		}
	}
	return ""
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
