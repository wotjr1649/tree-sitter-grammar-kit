package kit

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Static node-types.json contract revisions. SchemaFormat is the adopted reading of the
// Tree-sitter node-types.json format; SchemaComparator is the directional difference model.
const (
	SchemaFormat     = "node-types-r1"
	SchemaComparator = "node-types-diff-r1"
	SchemaPolicyName = "tsgk-schema-policy/r1"
)

// SchemaLimits are the finite bounds of one schema operation; every field must be positive.
// Records counts node entries, fields and type references of each input; the decoder also
// stops at 8 JSON values per record (JSON_VALUE_LIMIT).
type SchemaLimits struct {
	DocumentBytes, Records, OutputBytes uint64
	Wall                                time.Duration
}

// DefaultSchemaLimits returns the adopted `schema` operation bounds.
func DefaultSchemaLimits() SchemaLimits {
	return SchemaLimits{DocumentBytes: 16777216, Records: 200000, OutputBytes: 16777216, Wall: 120 * time.Second}
}

func (l SchemaLimits) valid() bool {
	return l.DocumentBytes > 0 && l.Records > 0 && l.OutputBytes > 0 && l.Wall > 0
}

// SchemaInput is one caller-owned node-types.json snapshot. Name is a caller label used in
// results and finding paths; the kit never opens it. Data is read during the call only.
type SchemaInput struct {
	Name string
	Data []byte
}

// SchemaCheckRequest validates one schema.
type SchemaCheckRequest struct {
	Input  SchemaInput
	Limits SchemaLimits
}

// SchemaDiffRequest compares a baseline schema with a candidate schema.
type SchemaDiffRequest struct {
	Baseline, Candidate SchemaInput
	Limits              SchemaLimits
}

// SchemaPolicy makes the operation bounds visible; its text is the "policy" identity.
type SchemaPolicy struct {
	Operation     string `json:"operation"`
	Format        string `json:"format"`
	Comparator    string `json:"comparator,omitempty"`
	DocumentBytes uint64 `json:"document_bytes"`
	Records       uint64 `json:"records"`
	OutputBytes   uint64 `json:"output_bytes"`
	WallMillis    int64  `json:"wall_ms"`
}

func (p SchemaPolicy) ref() IdentityRef {
	text := fmt.Sprintf("%s\noperation=%s\nformat=%s\ncomparator=%s\ndocument_bytes=%d\nrecords=%d\noutput_bytes=%d\nwall_ms=%d\n",
		SchemaPolicyName, p.Operation, p.Format, p.Comparator, p.DocumentBytes, p.Records, p.OutputBytes, p.WallMillis)
	sum := sha256.Sum256([]byte(text))
	return IdentityRef{Role: "policy", Schema: SchemaPolicyName, SHA256: hex.EncodeToString(sum[:])}
}

// NodeRef is a node identity: the same spelling with a different named status is a
// different node.
type NodeRef struct {
	Type  string `json:"type"`
	Named bool   `json:"named"`
}

// SchemaSummary binds one input's raw bytes; Counts is nil unless the input is a valid schema.
type SchemaSummary struct {
	Role   string        `json:"role"`
	Name   string        `json:"name"`
	Bytes  uint64        `json:"bytes"`
	SHA256 string        `json:"sha256"`
	Counts *SchemaCounts `json:"counts"`
}

// SchemaCounts describes a valid schema.
type SchemaCounts struct {
	Nodes      uint64    `json:"nodes"`
	Named      uint64    `json:"named"`
	Anonymous  uint64    `json:"anonymous"`
	Supertypes uint64    `json:"supertypes"`
	Fields     uint64    `json:"fields"`
	References uint64    `json:"references"`
	Roots      []NodeRef `json:"roots"`
}

// SchemaCheckResult is the outcome of SchemaCheck: PASS (valid), FAIL (format violation)
// or BLOCKED (only keys this format revision does not interpret).
type SchemaCheckResult struct {
	Report
	Policy SchemaPolicy  `json:"policy"`
	Input  SchemaSummary `json:"input"`
}

// SchemaDifference is one directional static contract change. Before and After are the
// compared values (null when absent on that side; *_PRESENCE_CHANGED compares the whole
// member). Paths are JSON pointers into the original baseline and candidate documents.
type SchemaDifference struct {
	Code          string          `json:"code"`
	Risk          string          `json:"risk"`
	Node          NodeRef         `json:"node"`
	Field         string          `json:"field,omitempty"`
	Member        *NodeRef        `json:"member,omitempty"`
	Before        json.RawMessage `json:"before"`
	After         json.RawMessage `json:"after"`
	BaselinePath  string          `json:"baseline_path"`
	CandidatePath string          `json:"candidate_path"`
}

// SchemaDiffResult is the outcome of SchemaDiff: PASS means the registered static contract
// is equal; FAIL means Differences is non-empty. Neither is a safety or SemVer verdict.
type SchemaDiffResult struct {
	Report
	Policy      SchemaPolicy       `json:"policy"`
	Baseline    SchemaSummary      `json:"baseline"`
	Candidate   SchemaSummary      `json:"candidate"`
	Differences []SchemaDifference `json:"differences"`
}

// Review-risk categories. They describe what to review; none means "safe".
const (
	RiskAddition       = "ADDITION"
	RiskRemoval        = "REMOVAL"
	RiskIdentity       = "IDENTITY"
	RiskClassification = "CLASSIFICATION"
	RiskNarrowed       = "CARDINALITY_NARROWED"
	RiskWidened        = "CARDINALITY_WIDENED"
)

// diffOrder fixes the report order of codes within one node.
var diffOrder = []string{"NODE_REMOVED", "NODE_ADDED", "NODE_NAMED_CHANGED", "ROOT_CHANGED", "EXTRA_CHANGED",
	"FIELDS_PRESENCE_CHANGED", "FIELD_REMOVED", "FIELD_ADDED", "FIELD_REQUIRED_CHANGED", "FIELD_MULTIPLE_CHANGED", "FIELD_TYPE_REMOVED", "FIELD_TYPE_ADDED",
	"CHILDREN_PRESENCE_CHANGED", "CHILDREN_REQUIRED_CHANGED", "CHILDREN_MULTIPLE_CHANGED", "CHILDREN_TYPE_REMOVED", "CHILDREN_TYPE_ADDED",
	"SUBTYPES_PRESENCE_CHANGED", "SUBTYPE_REMOVED", "SUBTYPE_ADDED"}

// SchemaCheck validates one node-types.json document without parsing any source.
func SchemaCheck(ctx context.Context, req SchemaCheckRequest) (SchemaCheckResult, error) {
	res := SchemaCheckResult{Report: newReport("schema check"), Policy: schemaPolicy("schema-check", "", req.Limits)}
	res.Input = summarize("input", req.Input, req.Limits)
	if e := schemaRun(ctx, req.Limits, &res.Report, func(r *run) *Error {
		res.Report.Identities = schemaIdentities(res.Policy, res.Input)
		doc, c, e := parseSchema(r, req.Input, req.Limits)
		if e != nil {
			return e
		}
		res.Report.Findings = append(res.Report.Findings, c.findings()...)
		res.Report.Findings = append(res.Report.Findings, Finding{Code: "SCHEMA_STATIC_ONLY", Severity: "info", Path: "",
			Message: "정적 node-types 계약 검사이며 runtime tree·parser·문법 정확성 판정이 아니다"})
		res.Report.Assessment = c.assessment()
		if doc != nil {
			res.Input.Counts = doc.counts()
		}
		res.Report.Coverage = schemaCoverage()
		return sealOutput(res, req.Limits.OutputBytes)
	}); e != nil {
		res.Input.Counts = nil
		if sealOutput(res, req.Limits.OutputBytes) != nil {
			keepFailureFinding(&res.Report)
		}
		return res, e
	}
	return res, nil
}

// SchemaDiff compares two valid schemas directionally (baseline → candidate). An invalid
// input is INVALID_INPUT/SCHEMA_INVALID (or UNSUPPORTED/SCHEMA_KEY_UNSUPPORTED) with that
// input's findings; it never yields a partial comparison.
func SchemaDiff(ctx context.Context, req SchemaDiffRequest) (SchemaDiffResult, error) {
	res := SchemaDiffResult{Report: newReport("schema diff"), Policy: schemaPolicy("schema-diff", SchemaComparator, req.Limits), Differences: []SchemaDifference{}}
	res.Baseline, res.Candidate = summarize("baseline", req.Baseline, req.Limits), summarize("candidate", req.Candidate, req.Limits)
	if e := schemaRun(ctx, req.Limits, &res.Report, func(r *run) *Error {
		res.Report.Identities = schemaIdentities(res.Policy, res.Baseline, res.Candidate)
		var docs [2]*schemaDoc
		for i, in := range []SchemaInput{req.Baseline, req.Candidate} {
			role := [2]string{"baseline", "candidate"}[i]
			doc, c, e := parseSchema(r, SchemaInput{Name: role + ":" + in.Name, Data: in.Data}, req.Limits)
			if e != nil {
				return e
			}
			res.Report.Findings = append(res.Report.Findings, c.findings()...)
			switch c.assessment() {
			case AssessFail:
				return fail(KindInvalidInput, "SCHEMA_INVALID", role+":"+in.Name, nil)
			case AssessBlocked:
				return fail(KindUnsupported, "SCHEMA_KEY_UNSUPPORTED", role+":"+in.Name, nil)
			}
			docs[i] = doc
		}
		res.Baseline.Counts, res.Candidate.Counts = docs[0].counts(), docs[1].counts()
		diffs, e := diffSchemas(r, docs[0], docs[1], req.Limits.OutputBytes)
		if e != nil {
			return e
		}
		res.Differences = diffs
		res.Report.Assessment = AssessPass
		if len(diffs) > 0 {
			res.Report.Assessment = AssessFail
		}
		res.Report.Findings = append(res.Report.Findings, Finding{Code: "SCHEMA_DIFF_DESCRIPTIVE", Severity: "info", Path: "",
			Message: "정적 node-types 계약의 방향성 차이 기록이며 안전성·SemVer·runtime 동작 판정이 아니다"})
		res.Report.Coverage = schemaCoverage()
		return sealOutput(res, req.Limits.OutputBytes)
	}); e != nil {
		res.Baseline.Counts, res.Candidate.Counts, res.Differences = nil, nil, []SchemaDifference{}
		if sealOutput(res, req.Limits.OutputBytes) != nil {
			keepFailureFinding(&res.Report)
		}
		return res, e
	}
	return res, nil
}

// keepFailureFinding reduces a failure report that would exceed the output limit to the
// failure finding failReport appended last.
func keepFailureFinding(r *Report) {
	if n := len(r.Findings); n > 1 {
		r.Findings = r.Findings[n-1:]
	}
}

func schemaPolicy(op, comparator string, l SchemaLimits) SchemaPolicy {
	return SchemaPolicy{Operation: op, Format: SchemaFormat, Comparator: comparator, DocumentBytes: l.DocumentBytes, Records: l.Records, OutputBytes: l.OutputBytes, WallMillis: l.Wall.Milliseconds()}
}

// summarize binds the raw input. An input over DocumentBytes is not hashed (Bytes 0, SHA256
// ""), so a caller passing the whole file and the CLI passing its bounded read agree.
func summarize(role string, in SchemaInput, l SchemaLimits) SchemaSummary {
	s := SchemaSummary{Role: role, Name: in.Name}
	if uint64(len(in.Data)) <= l.DocumentBytes {
		sum := sha256.Sum256(in.Data)
		s.Bytes, s.SHA256 = uint64(len(in.Data)), hex.EncodeToString(sum[:])
	}
	return s
}

func schemaIdentities(p SchemaPolicy, inputs ...SchemaSummary) []IdentityRef {
	out := []IdentityRef{}
	for _, in := range inputs {
		if in.SHA256 != "" {
			role := in.Role
			if role == "input" {
				role = "schema"
			}
			out = append(out, IdentityRef{Role: role, Schema: SchemaFormat, SHA256: in.SHA256})
		}
	}
	return append(out, p.ref())
}

func schemaCoverage() Coverage {
	return Coverage{Requested: []string{SchemaFormat}, Observed: []string{SchemaFormat}, Unsupported: []string{"runtime-tree", "supertype-expansion"}}
}

// schemaRun applies the shared guards, runs body and records any failure on the report.
func schemaRun(ctx context.Context, l SchemaLimits, rep *Report, body func(*run) *Error) error {
	var e *Error
	if !l.valid() {
		e = fail(KindInvalidInput, "LIMITS_INVALID", "", nil)
	} else if r, se := startRun(ctx, l.Wall); se != nil {
		e = se
	} else {
		defer r.cancel()
		e = body(r)
	}
	if e != nil {
		if e.Code == "OUTPUT_LIMIT" {
			rep.Findings = []Finding{} // the failure report itself must stay small
		}
		failReport(rep, e)
		return e
	}
	return nil
}

// maxSchemaFindings bounds the findings kept per input; the assessment counts every one.
const maxSchemaFindings = 1000

// collector gathers one input's findings: violations make FAIL, uninterpreted keys alone
// make BLOCKED, warnings keep PASS.
type collector struct {
	name                   string
	list                   []Finding
	total                  int
	violation, unsupported bool
}

// maxFindingPath bounds a finding path; a member name can be as long as the document.
const maxFindingPath = 1024

// add records a finding at value at (plus a literal pointer suffix). The pointer is built,
// clipped, only for a finding that is kept.
func (c *collector) add(code, sev string, at *jv, suffix, msg string) {
	if c.keep(code, sev) {
		p, cut := at.ptrClip(maxFindingPath)
		if !cut {
			p, cut = clipPath(p+suffix, maxFindingPath)
		}
		if cut {
			p += "...(truncated)"
		}
		c.list = append(c.list, Finding{Code: code, Severity: sev, Path: c.name + "#" + p, Message: msg})
	}
}

func (c *collector) addPath(code, sev, path, msg string) {
	if c.keep(code, sev) {
		if p, cut := clipPath(path, len(c.name)+1+maxFindingPath); cut {
			path = p + "...(truncated)"
		}
		c.list = append(c.list, Finding{Code: code, Severity: sev, Path: path, Message: msg})
	}
}

// keep counts a finding toward the assessment and reports whether it is still recorded.
func (c *collector) keep(code, sev string) bool {
	if sev == "error" {
		if code == "SCHEMA_KEY_UNSUPPORTED" {
			c.unsupported = true
		} else {
			c.violation = true
		}
	}
	c.total++
	return c.total <= maxSchemaFindings
}

// clipPath cuts p to at most max bytes on a UTF-8 boundary.
func clipPath(p string, max int) (string, bool) {
	if len(p) <= max {
		return p, false
	}
	for max > 0 && !utf8.RuneStart(p[max]) {
		max--
	}
	return p[:max], true
}

// ptrClip builds v's JSON pointer from the root, stopping after max bytes, so a long member
// name is never copied in full for a finding.
func (v *jv) ptrClip(max int) (string, bool) {
	var chain []*jv
	for x := v; x != nil && x.up != nil; x = x.up {
		chain = append(chain, x)
	}
	var b strings.Builder
	for i := len(chain) - 1; i >= 0; i-- {
		x := chain[i]
		seg := strconv.Itoa(x.idx)
		if x.up.kind == '{' {
			name := x.name
			if len(name) > max {
				name = name[:max]
			}
			seg = pointerEscaper.Replace(name)
		}
		b.WriteByte('/')
		b.WriteString(seg)
		if b.Len() > max {
			return clipPath(b.String(), max)
		}
	}
	return b.String(), false
}

func (c *collector) assessment() string {
	switch {
	case c.violation: // a definite violation outranks an uninterpreted key
		return AssessFail
	case c.unsupported:
		return AssessBlocked
	}
	return AssessPass
}

func (c *collector) findings() []Finding {
	out := append([]Finding{}, c.list...)
	if c.total > maxSchemaFindings {
		out = append(out, Finding{Code: "FINDINGS_TRUNCATED", Severity: "info", Path: c.name,
			Message: fmt.Sprintf("finding %d건 중 앞의 %d건만 기록했다", c.total, maxSchemaFindings)})
	}
	return out
}

type schemaRef struct {
	typ   string
	named bool
}

func (r schemaRef) public() NodeRef { return NodeRef{Type: r.typ, Named: r.named} }

func compareRefs(a, b schemaRef) int {
	if c := strings.Compare(a.typ, b.typ); c != 0 {
		return c
	}
	if a.named == b.named {
		return 0
	}
	if !a.named {
		return -1
	}
	return 1
}

// schemaSet is a children or field description; types is an unordered set (ref → member
// value). Values are kept as links; pointers are built only when reported.
type schemaSet struct {
	at                 *jv
	required, multiple bool
	types              map[schemaRef]*jv
}

type schemaNode struct {
	ref                   schemaRef
	at                    *jv
	root, extra           *bool
	fields                map[string]*schemaSet // nil: key absent (not the same as {})
	fieldsAt              *jv
	children              *schemaSet
	subtypes              map[schemaRef]*jv // nil: key absent
	subtypesAt            *jv
	hasFields, hasSubtype bool
}

type schemaDoc struct {
	nodes   map[schemaRef]*schemaNode
	records uint64
}

// parseSchema decodes and validates one document. Format violations are findings; only
// limits, cancellation and invalid requests are errors.
func parseSchema(r *run, in SchemaInput, l SchemaLimits) (*schemaDoc, *collector, *Error) {
	c := &collector{name: in.Name}
	add := c.add
	values := uint64(math.MaxUint64) // 8 JSON values per record, saturating
	if l.Records <= math.MaxUint64/8 {
		values = l.Records * 8
	}
	top, e := decodeBounded(in.Name, in.Data, l.DocumentBytes, &valueBudget{max: values, check: r.check})
	if e != nil {
		if e.Kind != KindInvalidInput {
			return nil, nil, e
		}
		c.addPath(e.Code, "error", e.Path, "JSON 문서가 엄격한 형식 규칙을 위반한다")
		return nil, c, nil
	}
	if top.kind != '[' {
		add(jsonKindCode(top), "error", top, "", "최상위 값은 node 배열이어야 한다")
		return nil, c, nil
	}
	if len(top.vals) == 0 {
		add("SCHEMA_EMPTY", "error", top, "", "node가 하나도 없는 schema는 유효한 빈 schema가 아니다")
		return nil, c, nil
	}
	doc := &schemaDoc{nodes: map[schemaRef]*schemaNode{}}
	count := func() *Error {
		if doc.records++; doc.records > l.Records {
			return fail(KindResourceLimit, "SCHEMA_RECORD_LIMIT", in.Name, nil)
		}
		if doc.records%1024 == 0 {
			return r.check()
		}
		return nil
	}
	var roots int
	for _, ev := range top.vals {
		if e := count(); e != nil {
			return nil, nil, e
		}
		n, ok, e := parseNode(ev, add, count)
		if e != nil {
			return nil, nil, e
		}
		if !ok {
			continue
		}
		if n.root != nil && *n.root {
			if roots++; roots > 1 {
				add("ROOT_MULTIPLE", "error", n.at, "/root", "root로 표시된 node가 둘 이상이다")
			}
		}
		if prev := doc.nodes[n.ref]; prev != nil {
			add("NODE_DUPLICATE", "error", n.at, "", "같은 (type, named) node가 "+prev.at.ptr()+"에도 있다")
			continue
		}
		doc.nodes[n.ref] = n
	}
	// References: a supertype's subtypes must be declared; an undeclared field/children
	// alternative is reported but is not a format violation (generator aliases do this).
	for _, n := range sortedNodes(doc) {
		for _, ref := range sortedRefs(n.subtypes) {
			if doc.nodes[ref] == nil {
				add("REFERENCE_UNRESOLVED", "error", n.subtypes[ref], "", "subtype이 선언된 node를 가리키지 않는다")
			}
		}
		for _, set := range n.sets() {
			for _, ref := range sortedRefs(set.types) {
				if doc.nodes[ref] == nil {
					add("REFERENCE_UNDECLARED", "warning", set.types[ref], "", "허용 type이 schema에 별도 node로 선언되지 않았다")
				}
			}
		}
	}
	if e := supertypeCycles(r, doc, add); e != nil {
		return nil, nil, e
	}
	if c.assessment() != AssessPass {
		return nil, c, nil
	}
	return doc, c, nil
}

func (n *schemaNode) sets() []*schemaSet {
	var out []*schemaSet
	for _, name := range sortedKeys(n.fields) {
		out = append(out, n.fields[name])
	}
	if n.children != nil {
		out = append(out, n.children)
	}
	return out
}

// parseNode reads one entry. Every violation is a finding; the returned bool reports only
// whether the (type, named) identity is readable, so duplicates are found even in entries
// that are invalid for another reason.
func parseNode(v *jv, add func(code, sev string, at *jv, suffix, msg string), count func() *Error) (*schemaNode, bool, *Error) {
	if v.kind != '{' {
		add(jsonKindCode(v), "error", v, "", "node 항목은 object여야 한다")
		return nil, false, nil
	}
	n := &schemaNode{at: v}
	typeOK, namedOK := false, false
	for i, key := range v.keys {
		val := v.vals[i]
		switch key {
		case "type":
			if val.kind != '"' {
				add(jsonKindCode(val), "error", val, "", "type은 문자열이어야 한다")
			} else if n.ref.typ = val.s; val.s == "" {
				add("NODE_TYPE_EMPTY", "error", val, "", "type이 비어 있다")
			} else {
				typeOK = true
			}
		case "named":
			n.ref.named, namedOK = boolOf(val, add, "named는 boolean이어야 한다")
		case "root", "extra":
			if b, good := boolOf(val, add, key+"는 boolean이어야 한다"); good && key == "root" {
				n.root = &b
			} else if good {
				n.extra = &b
			}
		case "fields":
			n.hasFields, n.fieldsAt = true, val
			if val.kind != '{' {
				add(jsonKindCode(val), "error", val, "", "fields는 object여야 한다")
				continue
			}
			n.fields = map[string]*schemaSet{}
			for j, name := range val.keys {
				if e := count(); e != nil {
					return nil, false, e
				}
				if name == "" {
					add("FIELD_NAME_EMPTY", "error", val.vals[j], "", "field 이름이 비어 있다")
					continue
				}
				set, e := parseSet(val.vals[j], add, count)
				if e != nil {
					return nil, false, e
				}
				if set != nil {
					n.fields[name] = set
				}
			}
		case "children":
			set, e := parseSet(val, add, count)
			if e != nil {
				return nil, false, e
			}
			n.children = set
		case "subtypes":
			n.hasSubtype, n.subtypesAt = true, val
			refs, good, e := parseRefs(val, add, count)
			if e != nil {
				return nil, false, e
			}
			n.subtypes = refs
			if good && len(refs) == 0 {
				add("SUBTYPES_EMPTY", "error", val, "", "subtypes가 비어 있다")
			}
		default:
			add("SCHEMA_KEY_UNSUPPORTED", "error", val, "", "이 형식 revision이 해석하지 않는 key다")
		}
	}
	for _, key := range []string{"type", "named"} {
		if !v.has(key) {
			add("JSON_MISSING_FIELD", "error", v, "/"+key, key+"가 없다")
		}
	}
	if n.hasSubtype && (n.hasFields || n.children != nil) {
		add("SUPERTYPE_SHAPE", "error", v, "", "supertype은 subtypes 외에 fields·children을 가질 수 없다")
	}
	return n, typeOK && namedOK, nil
}

func parseSet(v *jv, add func(code, sev string, at *jv, suffix, msg string), count func() *Error) (*schemaSet, *Error) {
	if v.kind != '{' {
		add(jsonKindCode(v), "error", v, "", "field·children 설명은 object여야 한다")
		return nil, nil
	}
	s := &schemaSet{at: v}
	for i, key := range v.keys {
		val := v.vals[i]
		switch key {
		case "required":
			s.required, _ = boolOf(val, add, "required는 boolean이어야 한다")
		case "multiple":
			s.multiple, _ = boolOf(val, add, "multiple은 boolean이어야 한다")
		case "types":
			refs, good, e := parseRefs(val, add, count)
			if e != nil {
				return nil, e
			}
			s.types = refs
			if good && len(refs) == 0 {
				add("TYPES_EMPTY", "error", val, "", "허용 type 목록이 비어 있다")
			}
		default:
			add("SCHEMA_KEY_UNSUPPORTED", "error", val, "", "이 형식 revision이 해석하지 않는 key다")
		}
	}
	for _, key := range []string{"multiple", "required", "types"} {
		if !v.has(key) {
			add("JSON_MISSING_FIELD", "error", v, "/"+key, key+"가 없다")
		}
	}
	return s, nil
}

// parseRefs reads a list of {type, named} references as a set; a repeated member is
// MEMBER_DUPLICATE, never silently merged.
func parseRefs(v *jv, add func(code, sev string, at *jv, suffix, msg string), count func() *Error) (map[schemaRef]*jv, bool, *Error) {
	if v.kind != '[' {
		add(jsonKindCode(v), "error", v, "", "type 목록은 배열이어야 한다")
		return nil, false, nil
	}
	out := map[schemaRef]*jv{}
	ok := true
	for _, m := range v.vals {
		if e := count(); e != nil {
			return nil, false, e
		}
		if m.kind != '{' {
			add(jsonKindCode(m), "error", m, "", "type 참조는 object여야 한다")
			ok = false
			continue
		}
		var ref schemaRef
		good, hasType, hasNamed := true, false, false
		for i, key := range m.keys {
			val := m.vals[i]
			switch key {
			case "type":
				hasType = true
				if val.kind != '"' {
					add(jsonKindCode(val), "error", val, "", "type은 문자열이어야 한다")
					good = false
				} else if ref.typ = val.s; val.s == "" {
					add("NODE_TYPE_EMPTY", "error", val, "", "type이 비어 있다")
					good = false
				}
			case "named":
				hasNamed = true
				b, g := boolOf(val, add, "named는 boolean이어야 한다")
				ref.named, good = b, good && g
			default:
				add("SCHEMA_KEY_UNSUPPORTED", "error", val, "", "이 형식 revision이 해석하지 않는 key다")
				good = false
			}
		}
		if !hasType {
			add("JSON_MISSING_FIELD", "error", m, "/type", "type이 없다")
			good = false
		}
		if !hasNamed {
			add("JSON_MISSING_FIELD", "error", m, "/named", "named가 없다")
			good = false
		}
		if !good {
			ok = false
			continue
		}
		if _, dup := out[ref]; dup {
			add("MEMBER_DUPLICATE", "error", m, "", "같은 type 참조가 이 목록에 이미 있다")
			ok = false
			continue
		}
		out[ref] = m
	}
	return out, ok, nil
}

func (v *jv) has(key string) bool { return slices.Contains(v.keys, key) }

func boolOf(v *jv, add func(code, sev string, at *jv, suffix, msg string), msg string) (bool, bool) {
	if v.kind == 't' || v.kind == 'f' {
		return v.kind == 't', true
	}
	add(jsonKindCode(v), "error", v, "", msg)
	return false, false
}

func jsonKindCode(v *jv) string {
	if v.kind == 'n' {
		return "JSON_NULL" // null is never an absent value
	}
	return "JSON_TYPE"
}

// supertypeCycles walks the supertype → subtype graph iteratively (no recursion), visiting
// each node once, and reports every subtype edge that closes a cycle.
func supertypeCycles(r *run, doc *schemaDoc, add func(code, sev string, at *jv, suffix, msg string)) *Error {
	const (
		white = iota
		grey
		black
	)
	color := map[schemaRef]int{}
	type frame struct {
		ref  schemaRef
		next []schemaRef
	}
	steps := 0
	for _, start := range sortedNodes(doc) {
		if start.subtypes == nil || color[start.ref] != white {
			continue
		}
		stack := []frame{{start.ref, sortedRefs(start.subtypes)}}
		color[start.ref] = grey
		for len(stack) > 0 {
			if steps++; steps%1024 == 0 {
				if e := r.check(); e != nil {
					return e
				}
			}
			top := &stack[len(stack)-1]
			if len(top.next) == 0 {
				color[top.ref] = black
				stack = stack[:len(stack)-1]
				continue
			}
			child := top.next[0]
			top.next = top.next[1:]
			cn := doc.nodes[child]
			if cn == nil || cn.subtypes == nil {
				continue
			}
			switch color[child] {
			case grey:
				add("SUPERTYPE_CYCLE", "error", doc.nodes[top.ref].subtypes[child], "", "supertype 관계가 순환한다")
			case white:
				color[child] = grey
				stack = append(stack, frame{child, sortedRefs(cn.subtypes)})
			}
		}
	}
	return nil
}

func (d *schemaDoc) counts() *SchemaCounts {
	c := &SchemaCounts{Roots: []NodeRef{}}
	for _, n := range sortedNodes(d) {
		c.Nodes++
		if n.ref.named {
			c.Named++
		} else {
			c.Anonymous++
		}
		if n.subtypes != nil {
			c.Supertypes++
			c.References += uint64(len(n.subtypes))
		}
		c.Fields += uint64(len(n.fields))
		for _, s := range n.sets() {
			c.References += uint64(len(s.types))
		}
		if n.root != nil && *n.root {
			c.Roots = append(c.Roots, n.ref.public())
		}
	}
	return c
}

func sortedNodes(d *schemaDoc) []*schemaNode {
	out := make([]*schemaNode, 0, len(d.nodes))
	for _, n := range d.nodes {
		out = append(out, n)
	}
	slices.SortFunc(out, func(a, b *schemaNode) int { return compareRefs(a.ref, b.ref) })
	return out
}

func sortedRefs[V any](m map[schemaRef]V) []schemaRef {
	out := make([]schemaRef, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.SortFunc(out, compareRefs)
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Canonical JSON of compared values: members sorted by (type, named), map keys sorted.
type setJSON struct {
	Multiple bool      `json:"multiple"`
	Required bool      `json:"required"`
	Types    []NodeRef `json:"types"`
}

type nodeJSON struct {
	Type     string               `json:"type"`
	Named    bool                 `json:"named"`
	Root     *bool                `json:"root,omitempty"`
	Extra    *bool                `json:"extra,omitempty"`
	Fields   *map[string]*setJSON `json:"fields,omitempty"` // pointer: {} stays distinct from absent
	Children *setJSON             `json:"children,omitempty"`
	Subtypes []NodeRef            `json:"subtypes,omitempty"`
}

func refList(m map[schemaRef]*jv) []NodeRef {
	out := []NodeRef{}
	for _, r := range sortedRefs(m) {
		out = append(out, r.public())
	}
	return out
}

func (s *schemaSet) canon() *setJSON {
	if s == nil {
		return nil
	}
	return &setJSON{Multiple: s.multiple, Required: s.required, Types: refList(s.types)}
}

func (n *schemaNode) canon() nodeJSON {
	j := nodeJSON{Type: n.ref.typ, Named: n.ref.named, Root: n.root, Extra: n.extra, Children: n.children.canon()}
	if n.fields != nil {
		fields := map[string]*setJSON{}
		for k, s := range n.fields {
			fields[k] = s.canon()
		}
		j.Fields = &fields
	}
	if n.subtypes != nil {
		j.Subtypes = refList(n.subtypes)
	}
	return j
}

// raw encodes a compared value; a nil pointer/map is the absent value null.
func raw(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err) // only strings, bools and slices/maps of them
	}
	return data
}

// loc is a reported location: a value link plus a literal pointer suffix.
type loc struct {
	at     *jv
	suffix string
}

func (l loc) size() int {
	if l.at == nil {
		return 0
	}
	return l.at.ptrLen() + len(l.suffix)
}

func (l loc) path() string {
	if l.at == nil {
		return ""
	}
	return l.at.ptr() + l.suffix
}

type pendingDiff struct {
	d          SchemaDifference
	base, cand loc
}

type emitFunc func(d SchemaDifference, base, cand loc)

// diffSchemas reports every static contract change from a to b. Absent fields, children and
// subtypes compare as empty member sets; their presence change is reported separately.
// Paths are built only after the whole report, paths included, is known to fit limit.
func diffSchemas(r *run, a, b *schemaDoc, limit uint64) ([]SchemaDifference, *Error) {
	var pending []pendingDiff
	var size uint64
	emit := func(d SchemaDifference, base, cand loc) {
		if size > limit {
			return
		}
		size += uint64(160 + len(d.Code) + len(d.Risk) + len(d.Node.Type) + len(d.Field) + len(d.Before) + len(d.After))
		if d.Member != nil {
			size += uint64(len(d.Member.Type))
		}
		pending = append(pending, pendingDiff{d, base, cand})
	}
	overLimit := fail(KindResourceLimit, "OUTPUT_LIMIT", "", nil)
	// Group identities by spelling to recognize a named-status change of one spelling.
	removed, added := map[string][]*schemaNode{}, map[string][]*schemaNode{}
	for ref, n := range a.nodes {
		if b.nodes[ref] == nil {
			removed[ref.typ] = append(removed[ref.typ], n)
		}
	}
	for ref, n := range b.nodes {
		if a.nodes[ref] == nil {
			added[ref.typ] = append(added[ref.typ], n)
		}
	}
	for typ, rs := range removed {
		if as := added[typ]; len(rs) == 1 && len(as) == 1 {
			emit(SchemaDifference{Code: "NODE_NAMED_CHANGED", Risk: RiskIdentity, Node: rs[0].ref.public(),
				Before: raw(rs[0].canon()), After: raw(as[0].canon())}, loc{rs[0].at, ""}, loc{as[0].at, ""})
			delete(added, typ)
			continue
		}
		for _, n := range rs {
			emit(SchemaDifference{Code: "NODE_REMOVED", Risk: RiskRemoval, Node: n.ref.public(), Before: raw(n.canon()), After: raw(nil)}, loc{n.at, ""}, loc{})
		}
	}
	for _, as := range added {
		for _, n := range as {
			emit(SchemaDifference{Code: "NODE_ADDED", Risk: RiskAddition, Node: n.ref.public(), Before: raw(nil), After: raw(n.canon())}, loc{}, loc{n.at, ""})
		}
	}
	steps := 0
	for _, x := range sortedNodes(a) {
		y := b.nodes[x.ref]
		if y == nil {
			continue
		}
		if size > limit {
			return nil, overLimit
		}
		if steps++; steps%1024 == 0 {
			if e := r.check(); e != nil {
				return nil, e
			}
		}
		node := x.ref.public()
		if !sameFlag(x.root, y.root) {
			emit(SchemaDifference{Code: "ROOT_CHANGED", Risk: RiskIdentity, Node: node, Before: raw(x.root), After: raw(y.root)}, flagLoc(x, x.root, "root"), flagLoc(y, y.root, "root"))
		}
		if !sameFlag(x.extra, y.extra) {
			emit(SchemaDifference{Code: "EXTRA_CHANGED", Risk: RiskClassification, Node: node, Before: raw(x.extra), After: raw(y.extra)}, flagLoc(x, x.extra, "extra"), flagLoc(y, y.extra, "extra"))
		}
		if x.hasFields != y.hasFields {
			emit(SchemaDifference{Code: "FIELDS_PRESENCE_CHANGED", Risk: RiskIdentity, Node: node, Before: raw(x.canon().Fields), After: raw(y.canon().Fields)}, loc{x.fieldsAt, ""}, loc{y.fieldsAt, ""})
		}
		for _, name := range unionKeys(x.fields, y.fields) {
			fx, fy := x.fields[name], y.fields[name]
			switch {
			case fy == nil:
				emit(SchemaDifference{Code: "FIELD_REMOVED", Risk: RiskRemoval, Node: node, Field: name, Before: raw(fx.canon()), After: raw(nil)}, loc{fx.at, ""}, loc{})
			case fx == nil:
				emit(SchemaDifference{Code: "FIELD_ADDED", Risk: RiskAddition, Node: node, Field: name, Before: raw(nil), After: raw(fy.canon())}, loc{}, loc{fy.at, ""})
			default:
				diffSet(emit, "FIELD", node, name, fx, fy)
			}
		}
		if (x.children == nil) != (y.children == nil) {
			risk := RiskAddition
			if y.children == nil {
				risk = RiskRemoval
			}
			emit(SchemaDifference{Code: "CHILDREN_PRESENCE_CHANGED", Risk: risk, Node: node, Before: raw(x.children.canon()), After: raw(y.children.canon())}, setLoc(x.children), setLoc(y.children))
		}
		diffSet(emit, "CHILDREN", node, "", x.children, y.children)
		if x.hasSubtype != y.hasSubtype {
			risk := RiskAddition
			if !y.hasSubtype {
				risk = RiskRemoval
			}
			emit(SchemaDifference{Code: "SUBTYPES_PRESENCE_CHANGED", Risk: risk, Node: node, Before: raw(x.canon().Subtypes), After: raw(y.canon().Subtypes)}, loc{x.subtypesAt, ""}, loc{y.subtypesAt, ""})
		}
		diffMembers(emit, "SUBTYPE", node, "", x.subtypes, y.subtypes)
	}
	if size > limit {
		return nil, overLimit
	}
	for _, p := range pending {
		if size += uint64(p.base.size() + p.cand.size()); size > limit {
			return nil, overLimit
		}
	}
	rank := map[string]int{}
	for i, c := range diffOrder {
		rank[c] = i
	}
	slices.SortFunc(pending, func(p, q pendingDiff) int {
		return cmp.Or(compareRefs(schemaRef{p.d.Node.Type, p.d.Node.Named}, schemaRef{q.d.Node.Type, q.d.Node.Named}),
			cmp.Compare(rank[p.d.Code], rank[q.d.Code]), strings.Compare(p.d.Field, q.d.Field), compareMember(p.d.Member, q.d.Member))
	})
	out := make([]SchemaDifference, 0, len(pending))
	for _, p := range pending {
		p.d.BaselinePath, p.d.CandidatePath = p.base.path(), p.cand.path()
		out = append(out, p.d)
	}
	return out, nil
}

// diffSet compares two children/field descriptions; cardinality is compared only when both
// exist, while alternatives compare against an empty set when one side is absent.
func diffSet(emit emitFunc, prefix string, node NodeRef, field string, x, y *schemaSet) {
	if x != nil && y != nil {
		if x.required != y.required {
			emit(SchemaDifference{Code: prefix + "_REQUIRED_CHANGED", Risk: cardinalityRisk(y.required), Node: node, Field: field,
				Before: raw(x.required), After: raw(y.required)}, loc{x.at, "/required"}, loc{y.at, "/required"})
		}
		if x.multiple != y.multiple {
			emit(SchemaDifference{Code: prefix + "_MULTIPLE_CHANGED", Risk: cardinalityRisk(!y.multiple), Node: node, Field: field,
				Before: raw(x.multiple), After: raw(y.multiple)}, loc{x.at, "/multiple"}, loc{y.at, "/multiple"})
		}
	}
	var xt, yt map[schemaRef]*jv
	if x != nil {
		xt = x.types
	}
	if y != nil {
		yt = y.types
	}
	diffMembers(emit, prefix+"_TYPE", node, field, xt, yt)
}

func diffMembers(emit emitFunc, prefix string, node NodeRef, field string, x, y map[schemaRef]*jv) {
	for _, ref := range sortedRefs(x) {
		if _, ok := y[ref]; !ok {
			m := ref.public()
			emit(SchemaDifference{Code: prefix + "_REMOVED", Risk: RiskRemoval, Node: node, Field: field, Member: &m, Before: raw(m), After: raw(nil)}, loc{x[ref], ""}, loc{})
		}
	}
	for _, ref := range sortedRefs(y) {
		if _, ok := x[ref]; !ok {
			m := ref.public()
			emit(SchemaDifference{Code: prefix + "_ADDED", Risk: RiskAddition, Node: node, Field: field, Member: &m, Before: raw(nil), After: raw(m)}, loc{}, loc{y[ref], ""})
		}
	}
}

// cardinalityRisk: a stricter constraint (now required, or no longer multiple) narrows.
func cardinalityRisk(narrowed bool) string {
	if narrowed {
		return RiskNarrowed
	}
	return RiskWidened
}

func sameFlag(a, b *bool) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

func flagLoc(n *schemaNode, v *bool, key string) loc {
	if v == nil {
		return loc{}
	}
	return loc{n.at, "/" + key}
}

func setLoc(s *schemaSet) loc {
	if s == nil {
		return loc{}
	}
	return loc{s.at, ""}
}

func unionKeys(a, b map[string]*schemaSet) []string {
	seen := map[string]*schemaSet{}
	for k, v := range a {
		seen[k] = v
	}
	for k, v := range b {
		seen[k] = v
	}
	return sortedKeys(seen)
}

func compareMember(a, b *NodeRef) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return compareRefs(schemaRef{a.Type, a.Named}, schemaRef{b.Type, b.Named})
}
