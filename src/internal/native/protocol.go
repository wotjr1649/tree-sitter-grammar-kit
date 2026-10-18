package native

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"strconv"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// Driver protocol revisions: r1 (S05), r2 (S06), r3 (opt-in logger observations). The
// driver answers each request in the revision it was asked in.
const (
	Protocol   = "tsgk-native/r1"
	ProtocolR2 = "tsgk-native/r2"
	ProtocolR3 = "tsgk-native/r3"
)

// MaxRequestFrame is the request frame payload ceiling.
const MaxRequestFrame = 50331648

// Limits are the per-request driver limits.
type Limits struct {
	InputBytes, Nodes, FullNodes, Depth, OutputBytes, ParseMillis, MemoryBytes, Errors, PartialNodes uint64
	Matches, Captures, QueryMillis                                                                   uint64 // r2 only
}

// LimitsFor returns the request limits of an operation.
func LimitsFor(op kit.NativeOperation) Limits {
	return Limits{op.InputBytes, op.Nodes, op.FullNodes, op.Depth, op.OutputBytes, op.ParseMillis, op.MemoryBytes, op.Errors, op.PartialNodes,
		op.Matches, op.Captures, op.QueryMillis}
}

// QuerySource is one r2 query: its id and UTF-8 source.
type QuerySource = querywire.Source

// Request is one driver request.
type Request struct {
	ID                        string
	Encoding                  string
	Output                    string
	Limits                    Limits
	Declarations              []kit.NativeDeclaration
	Points                    []kit.NativePoint
	Ranges                    [][]kit.Span // included ranges per step (SVC inline code); nil for whole input
	Source                    []byte
	Edits                     []kit.Edit
	Protocol                  string // "" is r1
	Queries                   []QuerySource
	API                       bool
	ObservationRunID          string
	ObservationProducerSHA256 string
	ObservationHistoryStart   uint64
}

// Revision is the request's protocol revision.
func (r Request) Revision() string {
	if r.Protocol == "" {
		return Protocol
	}
	return r.Protocol
}

// Encode renders the canonical request payload (fixed key order, no whitespace). An r1
// request is exactly the S05 payload; r2 adds the query limits, queries and the API switch.
func (r Request) Encode() []byte {
	var b bytes.Buffer
	b64 := base64.StdEncoding.EncodeToString
	r2 := r.Revision() == ProtocolR2
	fmt.Fprintf(&b, `{"protocol":%q,"id":%q,"encoding":%q,"output":%q,`, r.Revision(), r.ID, r.Encoding, r.Output)
	l := r.Limits
	fmt.Fprintf(&b, `"limits":{"input_bytes":%d,"nodes":%d,"full_nodes":%d,"depth":%d,"output_bytes":%d,"parse_ms":%d,"memory_bytes":%d,"errors":%d,"partial_nodes":%d`,
		l.InputBytes, l.Nodes, l.FullNodes, l.Depth, l.OutputBytes, l.ParseMillis, l.MemoryBytes, l.Errors, l.PartialNodes)
	if r2 {
		fmt.Fprintf(&b, `,"matches":%d,"captures":%d,"query_ms":%d`, l.Matches, l.Captures, l.QueryMillis)
	}
	b.WriteString("},")
	b.WriteString(`"declarations":[`)
	for i, d := range r.Declarations {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"fact":%q,"node":%q,"name":%q}`, d.Fact, d.Node, d.Name)
	}
	b.WriteString(`],"points":[`)
	for i, p := range r.Points {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":%q,"byte":%d}`, p.ID, p.Byte)
	}
	b.WriteString(`],"ranges":[`)
	for k, step := range r.Ranges {
		if k > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('[')
		for i, g := range step {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"start_byte":%d,"end_byte":%d,"start_point":[%d,%d],"end_point":[%d,%d]}`, g.StartByte, g.EndByte, g.StartPoint.Row, g.StartPoint.Column, g.EndPoint.Row, g.EndPoint.Column)
		}
		b.WriteByte(']')
	}
	fmt.Fprintf(&b, `],"source":"%s","edits":[`, b64(r.Source))
	for i, e := range r.Edits {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"start_byte":%d,"old_end_byte":%d,"new_end_byte":%d,"old":"%s","new":"%s"}`, e.StartByte, e.OldEndByte, e.NewEndByte, b64(e.Old), b64(e.New))
	}
	b.WriteByte(']')
	if r2 {
		b.WriteString(`,"queries":[`)
		for i, q := range r.Queries {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"id":%q,"source":"%s"}`, q.ID, b64(q.Source))
		}
		fmt.Fprintf(&b, `],"api":%t`, r.API)
	}
	if r.Revision() == ProtocolR3 {
		fmt.Fprintf(&b, `,"reuse_observation":%q,"observation_run_id":%q,"observation_producer_sha256":%q,"observation_history_start":%d`, reuseobs.Policy, r.ObservationRunID, r.ObservationProducerSHA256, r.ObservationHistoryStart)
	}
	b.WriteByte('}')
	return b.Bytes()
}

// Frame prefixes a payload with its 4-byte big-endian length.
func Frame(payload []byte) []byte {
	out := make([]byte, 4, 4+len(payload))
	binary.BigEndian.PutUint32(out, uint32(len(payload)))
	return append(out, payload...)
}

// Response is a decoded driver response.
type Response struct {
	Protocol       string   `json:"protocol"`
	ID             string   `json:"id"`
	Status         string   `json:"status"`
	Code           string   `json:"code"`
	Producer       Producer `json:"producer"`
	SourceBytes    uint64   `json:"source_bytes"`
	StepsCompleted uint64   `json:"steps_completed"`
	Steps          []Step   `json:"steps"`
	Complete       bool     `json:"complete"`
}

// Producer is the runtime and grammar ABI the driver reports. An r2 producer also declares
// its capabilities (query and API revisions, predicate handling) and the language shape;
// those members are absent from r1.
type Producer struct {
	LanguageVersion        uint32         `json:"language_version"`
	RuntimeLanguageVersion uint32         `json:"runtime_language_version"`
	RuntimeMinCompatible   uint32         `json:"runtime_min_compatible"`
	Query                  string         `json:"query"`
	API                    *string        `json:"api,omitempty"`
	Predicates             *string        `json:"predicates,omitempty"`
	SymbolCount            *uint32        `json:"symbol_count,omitempty"`
	FieldCount             *uint32        `json:"field_count,omitempty"`
	ReuseObservation       jsontext.Value `json:"reuse_observation,omitempty"`
}

// Step is one parse step of a response.
type Step struct {
	Step         int            `json:"step"`
	SourceBytes  uint64         `json:"source_bytes"`
	SourceSHA256 string         `json:"source_sha256"`
	Edit         *StepEdit      `json:"edit"`
	Route        *Route         `json:"route"`
	Incremental  WireTree       `json:"incremental"`
	Fresh        *WireTree      `json:"fresh"`
	Observations jsontext.Value `json:"observations,omitempty"`
}

// StepEdit echoes an edit with the driver-computed points.
type StepEdit struct {
	StartByte   uint32    `json:"start_byte"`
	OldEndByte  uint32    `json:"old_end_byte"`
	NewEndByte  uint32    `json:"new_end_byte"`
	StartPoint  [2]uint32 `json:"start_point"`
	OldEndPoint [2]uint32 `json:"old_end_point"`
	NewEndPoint [2]uint32 `json:"new_end_point"`
}

// Route is the driver's tree-observed incremental route instrumentation.
type Route struct {
	EditHasChanges    bool   `json:"edit_has_changes"`
	EditedRootEndByte uint32 `json:"edited_root_end_byte"`
	ReusedNodes       uint64 `json:"reused_nodes"`
	FreshReusedNodes  uint64 `json:"fresh_reused_nodes"`
}

// WireTree is one serialized tree in any form.
type WireTree struct {
	Status          string         `json:"status"`
	Code            string         `json:"code"`
	ParseMillis     uint64         `json:"parse_ms"`
	Form            *string        `json:"form"`
	Reason          string         `json:"reason"`
	DescendantCount uint64         `json:"descendant_count"`
	MaxDepth        uint64         `json:"max_depth"`
	HasError        bool           `json:"has_error"`
	Digest          string         `json:"digest"`
	Types           []string       `json:"types"`
	Fields          []string       `json:"fields"`
	Nodes           [][]int64      `json:"nodes"`
	Errors          *WireErrors    `json:"errors"`
	Declarations    *[]DeclItem    `json:"declarations"`
	PartialTrees    []WirePartial  `json:"partial_trees"`
	Queries         jsontext.Value `json:"queries"` // r2: null or the query results; absent in r1
	API             jsontext.Value `json:"api"`     // r2: null or the API observations; absent in r1
}

// WireQuery is one r2 query result on one tree.
type WireQuery = querywire.Query

// QueryError is the runtime's query rejection class, byte offset and point.
type QueryError = querywire.CompileError

// WirePredicate is one predicate of a pattern as the runtime lists it.
type WirePredicate = querypolicy.Predicate

// PredStep is one predicate step: kind capture (with its quantifier) or string.
type PredStep = querypolicy.Step

// WireAPI is the tsgk-api/r1 observation of one full tree.
type WireAPI struct {
	Revision     string    `json:"revision"`
	Nodes        [][]int64 `json:"nodes"`
	ChildFields  [][]int64 `json:"child_fields"`
	FieldLookups [][]int64 `json:"field_lookups"`
	Points       [][]int64 `json:"points"`
	Names        []string  `json:"names"`
}

// WireErrors is the capped ERROR/MISSING list.
type WireErrors struct {
	Limit     uint64      `json:"limit"`
	Total     uint64      `json:"total"`
	Truncated bool        `json:"truncated"`
	Items     []WireError `json:"items"`
}

// WireError is one ERROR or MISSING node.
type WireError struct {
	Kind       string    `json:"kind"`
	Type       string    `json:"type"`
	StartByte  uint32    `json:"start_byte"`
	EndByte    uint32    `json:"end_byte"`
	StartPoint [2]uint32 `json:"start_point"`
	EndPoint   [2]uint32 `json:"end_point"`
}

// DeclItem is one declaration structure item.
type DeclItem struct {
	Fact      string    `json:"fact"`
	NodeType  string    `json:"node_type"`
	StartByte uint32    `json:"start_byte"`
	EndByte   uint32    `json:"end_byte"`
	Name      *NameSpan `json:"name"`
	Status    string    `json:"status"`
}

// NameSpan is a declaration name range.
type NameSpan struct {
	StartByte uint32 `json:"start_byte"`
	EndByte   uint32 `json:"end_byte"`
}

// WirePartial is one registered-point partial tree.
type WirePartial struct {
	Point     string    `json:"point"`
	Byte      uint32    `json:"byte"`
	Truncated bool      `json:"truncated"`
	Nodes     [][]int64 `json:"nodes"`
}

// DecodeResponse strictly decodes one response payload: unknown or duplicate members,
// invalid UTF-8 and trailing values are rejected.
func DecodeResponse(payload []byte) (Response, error) {
	var r Response
	if err := jsonv2.Unmarshal(payload, &r, jsonv2.RejectUnknownMembers(true)); err != nil {
		return r, invalid("RESPONSE_MALFORMED", err)
	}
	return r, nil
}

func invalid(code string, cause error) *Error { return refuse(kit.KindIO, code, cause) }

// SplitFrame returns the single response payload of out and rejects truncation and any
// trailing bytes.
func SplitFrame(out []byte) ([]byte, error) {
	if len(out) < 4 {
		return nil, invalid("RESPONSE_TRUNCATED", nil)
	}
	n := uint64(binary.BigEndian.Uint32(out))
	switch rest := uint64(len(out) - 4); {
	case rest < n:
		return nil, invalid("RESPONSE_TRUNCATED", nil)
	case rest > n:
		return nil, invalid("RESPONSE_TRAILING_BYTES", nil)
	}
	return out[4:], nil
}

// Nodes converts compact node arrays to public nodes and checks the name-table indices.
func Nodes(rows [][]int64, types, fields []string) ([]kit.TreeNode, error) {
	for _, field := range fields {
		if field == "" {
			return nil, invalid("RESPONSE_INVALID", errors.New("empty field name"))
		}
	}
	out := make([]kit.TreeNode, 0, len(rows))
	for i, r := range rows {
		bad := func(what string) error {
			return invalid("RESPONSE_INVALID", fmt.Errorf("node %d: %s", i, what))
		}
		if len(r) != 10 {
			return nil, bad("arity")
		}
		if i == 0 && r[0] != -1 {
			return nil, bad("root parent")
		}
		for k := 4; k < 10; k++ {
			if r[k] < 0 || r[k] > 0xffffffff {
				return nil, bad("range")
			}
		}
		if r[1] < 0 || r[1] >= int64(len(types)) || r[2] < -1 || r[2] >= int64(len(fields)) || r[3] < 0 || r[3] > 31 {
			return nil, bad("index or flags")
		}
		n := kit.TreeNode{Parent: r[0], Type: types[r[1]], Named: r[3]&1 != 0, Extra: r[3]&2 != 0, IsError: r[3]&4 != 0, HasError: r[3]&8 != 0, IsMissing: r[3]&16 != 0,
			StartByte: uint32(r[4]), EndByte: uint32(r[5]), StartPoint: kit.Point{Row: uint32(r[6]), Column: uint32(r[7])}, EndPoint: kit.Point{Row: uint32(r[8]), Column: uint32(r[9])}}
		if r[0] == -1 && r[2] >= 0 {
			return nil, bad("root field")
		}
		if r[2] >= 0 {
			f := fields[r[2]]
			n.Field = &f
		}
		out = append(out, n)
	}
	return out, nil
}

// Tree is a validated tree of one step.
type Tree struct {
	Status   string
	Code     string
	Form     string // full | summary | record | "" when not completed
	Wire     WireTree
	Nodes    []kit.TreeNode // full form only
	Partials [][]kit.TreeNode
	Queries  []WireQuery // r2: one per requested query when the tree completed
	API      *WireAPI    // r2: requested API observations of a full tree
}

// incomplete returns the status and code of the first part of t that did not complete:
// the tree itself, else a query cut by a limit or failed. An invalid query is an
// observation of the query input, not an incomplete tree.
func (t Tree) incomplete() (string, string, bool) {
	if t.Status != kit.StatusCompleted {
		return t.Status, t.Code, true
	}
	for _, q := range t.Queries {
		if q.Status == kit.StatusResourceLimit || q.Status == kit.StatusFailed {
			return q.Status, q.Code, true
		}
	}
	return "", "", false
}

func isNull(v jsontext.Value) bool { return string(v) == "null" }

// requireMembers rejects a decoded r2 object or array of objects that lacks a member: the
// strict decoder would otherwise leave an absent member at its zero value, and an absent
// predicate list must never read as "no predicates".
func requireMembers(raw jsontext.Value, array bool, names ...string) error {
	if err := querywire.RequireMembers(raw, array, names...); err != nil {
		return invalid(err.Code, err.Cause)
	}
	return nil
}

// checkTree validates a tree object against the step's source length and, for r2, the
// requested queries and API observations.
func checkTree(w WireTree, sourceBytes uint64, nodes uint64, req Request) (Tree, error) {
	t := Tree{Status: w.Status, Code: w.Code, Wire: w}
	r2 := req.Revision() == ProtocolR2
	if r2 != (w.Queries != nil) || r2 != (w.API != nil) {
		return t, invalid("RESPONSE_INVALID", errors.New("query/api members do not match the revision"))
	}
	switch w.Status {
	case kit.StatusCompleted:
		if w.Code != "" || w.Form == nil {
			return t, invalid("RESPONSE_INVALID", errors.New("completed tree without form"))
		}
	case kit.StatusResourceLimit, kit.StatusFailed:
		if w.Code == "" || w.Form != nil || w.Nodes != nil || (r2 && (!isNull(w.Queries) || !isNull(w.API))) {
			return t, invalid("RESPONSE_INVALID", errors.New("noncomplete tree with data"))
		}
		return t, nil
	default:
		return t, invalid("RESPONSE_INVALID", errors.New("tree status "+strconv.Quote(w.Status)))
	}
	t.Form = *w.Form
	if len(w.Digest) != 64 || w.DescendantCount == 0 || w.DescendantCount > nodes {
		return t, invalid("RESPONSE_INVALID", errors.New("tree header"))
	}
	switch t.Form {
	case "full":
		ns, err := Nodes(w.Nodes, w.Types, w.Fields)
		if err != nil {
			return t, err
		}
		if err := kit.ValidateTree(ns, sourceBytes); err != nil {
			return t, invalid("RESPONSE_INVALID", err)
		}
		if uint64(len(ns)) != w.DescendantCount || ns[0].HasError != w.HasError {
			return t, invalid("RESPONSE_INVALID", errors.New("count or root flags"))
		}
		if kit.TreeDigest(ns) != w.Digest {
			return t, invalid("TREE_DIGEST_MISMATCH", nil)
		}
		t.Nodes = ns
	case "summary", "record":
		if w.Errors == nil || w.Nodes != nil || uint64(len(w.Errors.Items)) > w.Errors.Limit || w.Errors.Truncated != (w.Errors.Total > uint64(len(w.Errors.Items))) {
			return t, invalid("RESPONSE_INVALID", errors.New("summary errors"))
		}
		for _, e := range w.Errors.Items {
			if (e.Kind != "ERROR" && e.Kind != "MISSING") || e.StartByte > e.EndByte || uint64(e.EndByte) > sourceBytes {
				return t, invalid("RESPONSE_INVALID", errors.New("error item"))
			}
		}
		for _, p := range w.PartialTrees {
			ns, err := Nodes(p.Nodes, w.Types, w.Fields)
			if err != nil {
				return t, err
			}
			if len(ns) > 0 {
				if err := kit.ValidateTree(ns, sourceBytes); err != nil {
					return t, invalid("RESPONSE_INVALID", err)
				}
			}
			t.Partials = append(t.Partials, ns)
		}
	default:
		return t, invalid("RESPONSE_INVALID", errors.New("form"))
	}
	if w.Declarations != nil {
		for _, d := range *w.Declarations {
			if d.StartByte > d.EndByte || uint64(d.EndByte) > sourceBytes || (d.Name != nil && (d.Name.StartByte > d.Name.EndByte || uint64(d.Name.EndByte) > sourceBytes)) {
				return t, invalid("RESPONSE_INVALID", errors.New("declaration range"))
			}
			switch d.Status {
			case "PASS", "NAME_MISSING", "HAS_ERROR":
			default:
				return t, invalid("RESPONSE_INVALID", errors.New("declaration status"))
			}
		}
	}
	if r2 {
		if err := t.checkQueries(req, sourceBytes); err != nil {
			return t, err
		}
		if err := t.checkAPI(req); err != nil {
			return t, err
		}
	}
	return t, nil
}

// checkQueries retains the full-node link check while sharing strict wire validation.
func (t *Tree) checkQueries(req Request, sourceBytes uint64) error {
	t.Queries = nil
	var queries []WireQuery
	var err *querywire.Error
	if t.Nodes != nil {
		queries, err = querywire.DecodeFull(t.Wire.Queries, req.Queries, sourceBytes, t.Wire.DescendantCount, func(q WireQuery, row []int64) bool {
			if row[3] < 0 || row[3] >= int64(len(t.Nodes)) {
				return false
			}
			n := t.Nodes[row[3]]
			c := captureOf(q, row)
			return n.Type == c.Type && n.Named == c.Named && n.Extra == c.Extra && n.IsError == c.IsError && n.HasError == c.HasError && n.IsMissing == c.IsMissing &&
				n.StartByte == c.StartByte && n.EndByte == c.EndByte && n.StartPoint == c.StartPoint && n.EndPoint == c.EndPoint
		})
	} else {
		queries, err = querywire.DecodeSummary(t.Wire.Queries, req.Queries, sourceBytes, t.Wire.DescendantCount)
	}
	if err != nil {
		return invalid(err.Code, err.Cause)
	}
	t.Queries = queries
	return nil
}

// captureOf converts one validated capture row to the public capture.
func captureOf(q WireQuery, row []int64) kit.Capture {
	f := row[5]
	return kit.Capture{Match: uint32(row[0]), Pattern: uint32(row[1]), Capture: uint32(row[2]), Name: q.CaptureNames[row[2]], Node: row[3], Type: q.Types[row[4]],
		Named: f&1 != 0, Extra: f&2 != 0, IsError: f&4 != 0, HasError: f&8 != 0, IsMissing: f&16 != 0,
		StartByte: uint32(row[6]), EndByte: uint32(row[7]), StartPoint: kit.Point{Row: uint32(row[8]), Column: uint32(row[9])}, EndPoint: kit.Point{Row: uint32(row[10]), Column: uint32(row[11])}}
}

// checkAPI validates the shape of requested API observations: present exactly for a
// completed full tree of an API request, with every row's arity and table indices in
// range. Whether the values agree with the serialized tree is judged by compareAPI.
func (t *Tree) checkAPI(req Request) error {
	bad := func(what string) error { return invalid("RESPONSE_INVALID", errors.New("api: "+what)) }
	want := req.API && t.Form == "full"
	if isNull(t.Wire.API) {
		if want {
			return bad("missing")
		}
		return nil
	}
	if !want {
		return bad("unrequested")
	}
	if err := requireMembers(t.Wire.API, false, "revision", "nodes", "child_fields", "field_lookups", "points", "names"); err != nil {
		return err
	}
	var a WireAPI
	if err := jsonv2.Unmarshal(t.Wire.API, &a, jsonv2.RejectUnknownMembers(true)); err != nil {
		return invalid("RESPONSE_MALFORMED", err)
	}
	n := int64(len(t.Nodes))
	if a.Revision != "tsgk-api/r1" || int64(len(a.Nodes)) != n || len(a.Points) != len(req.Points) {
		return bad("header")
	}
	for _, r := range a.Nodes {
		if len(r) != 13 {
			return bad("node row")
		}
	}
	for _, r := range a.ChildFields {
		if len(r) != 3 || r[0] < 0 || r[0] >= n || r[1] < 0 || r[2] < 0 || r[2] >= int64(len(a.Names)) {
			return bad("child field row")
		}
	}
	for _, r := range a.FieldLookups {
		if len(r) != 3 || r[0] < 0 || r[0] >= n || r[1] < 0 || r[1] >= int64(len(a.Names)) {
			return bad("field lookup row")
		}
	}
	for k, r := range a.Points {
		if len(r) != 4 || r[0] != int64(req.Points[k].Byte) {
			return bad("point row")
		}
	}
	t.API = &a
	return nil
}

// Checked is a response validated against its request and the kit's own intermediate
// sources and edit points.
type Checked struct {
	Response Response
	Steps    []CheckedStep
}

// CheckedStep is one validated step.
type CheckedStep struct {
	Step         Step
	Incremental  Tree
	Fresh        *Tree
	Observations []reuseobs.Observation
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Check validates a decoded response for req: protocol and id, status/exit agreement, the
// step sequence, each step's source identity and edit points against the kit's own
// computation, and every tree's structure.
func Check(r Response, req Request, versions [][]byte, points []kit.EditPoints, exitCode int) (Checked, error) {
	c := Checked{Response: r}
	if r.Protocol != req.Revision() {
		return c, invalid("RESPONSE_PROTOCOL_MISMATCH", fmt.Errorf("request %s response %s", req.Revision(), r.Protocol))
	}
	if !r.Complete {
		return c, invalid("RESPONSE_INVALID", errors.New("completeness"))
	}
	if r.ID != req.ID {
		return c, invalid("RESPONSE_INVALID", errors.New("id"))
	}
	want := map[string]int{kit.StatusCompleted: 0, "INVALID_REQUEST": 2, kit.StatusResourceLimit: 3, kit.StatusFailed: 4}
	code, ok := want[r.Status]
	if !ok || code != exitCode {
		return c, invalid("EXIT_STATUS_MISMATCH", fmt.Errorf("status %s exit %d", r.Status, exitCode))
	}
	if (r.Status == kit.StatusCompleted) != (r.Code == "") {
		return c, invalid("RESPONSE_INVALID", errors.New("code"))
	}
	p := r.Producer
	if req.Revision() != Protocol && req.Revision() != ProtocolR2 && req.Revision() != ProtocolR3 {
		return c, invalid("RESPONSE_PROTOCOL_MISMATCH", errors.New("unknown protocol"))
	}
	if req.Revision() == ProtocolR3 && (len(req.Queries) != 0 || req.API) {
		return c, invalid("RESPONSE_INVALID", errors.New("r3 query/API combination"))
	}
	if req.Revision() == Protocol || req.Revision() == ProtocolR3 {
		if p.Query != "UNSUPPORTED" || p.API != nil || p.Predicates != nil || p.SymbolCount != nil || p.FieldCount != nil {
			return c, invalid("RESPONSE_INVALID", errors.New("r1 producer"))
		}
	} else if p.API == nil || p.Predicates == nil || *p.Predicates != "NOT_EVALUATED" || p.SymbolCount == nil || p.FieldCount == nil || p.Query == "" {
		return c, invalid("RESPONSE_INVALID", errors.New("r2 producer capabilities"))
	}
	if req.Revision() == ProtocolR3 {
		if err := requireMembers(p.ReuseObservation, false, "policy", "runtime_manifest_sha256"); err != nil {
			return c, err
		}
		var capability struct {
			Policy                string `json:"policy"`
			RuntimeManifestSHA256 string `json:"runtime_manifest_sha256"`
		}
		if err := jsonv2.Unmarshal(p.ReuseObservation, &capability, jsonv2.RejectUnknownMembers(true)); err != nil ||
			capability.Policy != reuseobs.Policy || capability.RuntimeManifestSHA256 != reuseobs.RuntimeManifestSHA256 {
			return c, invalid("RESPONSE_INVALID", errors.New("r3 observation capability"))
		}
	} else if len(p.ReuseObservation) != 0 {
		return c, invalid("RESPONSE_INVALID", errors.New("unexpected observation capability"))
	}
	if r.Status == "INVALID_REQUEST" {
		if len(r.Steps) != 0 {
			return c, invalid("RESPONSE_INVALID", errors.New("steps on invalid request"))
		}
		return c, nil
	}
	if r.Status == kit.StatusCompleted && len(r.Steps) != len(req.Edits)+1 {
		return c, invalid("RESPONSE_INVALID", errors.New("step count"))
	}
	if len(r.Steps) > len(req.Edits)+1 || r.SourceBytes != uint64(len(req.Source)) {
		return c, invalid("RESPONSE_INVALID", errors.New("steps or source"))
	}
	completed := uint64(0)
	for k, s := range r.Steps {
		if s.Step != k || s.SourceBytes != uint64(len(versions[k])) || s.SourceSHA256 != sha(versions[k]) {
			return c, invalid("SOURCE_TRANSPORT_MISMATCH", fmt.Errorf("step %d", k))
		}
		if (k == 0) != (s.Edit == nil) || (k == 0 && (s.Route != nil || s.Fresh != nil)) {
			return c, invalid("RESPONSE_INVALID", fmt.Errorf("step %d shape", k))
		}
		if k > 0 {
			e, p := req.Edits[k-1], points[k-1]
			got := s.Edit
			if got.StartByte != e.StartByte || got.OldEndByte != e.OldEndByte || got.NewEndByte != e.NewEndByte ||
				got.StartPoint != [2]uint32{p.Start.Row, p.Start.Column} || got.OldEndPoint != [2]uint32{p.OldEnd.Row, p.OldEnd.Column} ||
				got.NewEndPoint != [2]uint32{p.NewEnd.Row, p.NewEnd.Column} {
				return c, invalid("EDIT_POINT_MISMATCH", fmt.Errorf("step %d", k))
			}
		}
		inc, err := checkTree(s.Incremental, s.SourceBytes, req.Limits.Nodes, req)
		if err != nil {
			return c, err
		}
		cs := CheckedStep{Step: s, Incremental: inc}
		if req.Revision() == ProtocolR3 {
			var raw []jsontext.Value
			if err := jsonv2.Unmarshal(s.Observations, &raw); err != nil || len(raw) != 1+min(k, 1) {
				return c, invalid("RESPONSE_INVALID", errors.New("observation roles"))
			}
			for index, value := range raw {
				role := "initial"
				if k > 0 {
					role = "incremental"
					if index == 1 {
						role = "fresh"
					}
				}
				binding := reuseobs.Binding{RuntimeManifestSHA256: reuseobs.RuntimeManifestSHA256, RequestID: req.ID,
					RunID: req.ObservationRunID, ProducerSHA256: req.ObservationProducerSHA256,
					HistoryStart: req.ObservationHistoryStart,
					Step:         uint64(k), Role: role, SourceSHA256: s.SourceSHA256, SourceBytes: s.SourceBytes}
				if k > 0 {
					binding.PreviousSourceSHA256 = sha(versions[k-1])
				}
				observation, err := reuseobs.Decode(value, binding)
				if err != nil {
					return c, invalid("RESPONSE_INVALID", err)
				}
				cs.Observations = append(cs.Observations, observation)
			}
		} else if len(s.Observations) != 0 {
			return c, invalid("RESPONSE_INVALID", errors.New("unexpected observations"))
		}
		if s.Fresh != nil {
			f, err := checkTree(*s.Fresh, s.SourceBytes, req.Limits.Nodes, req)
			if err != nil {
				return c, err
			}
			cs.Fresh = &f
		}
		_, _, incBad := inc.incomplete()
		freshBad := true
		if cs.Fresh != nil {
			_, _, freshBad = cs.Fresh.incomplete()
		}
		stepDone := !incBad && (k == 0 || !freshBad)
		if stepDone {
			completed++
		} else if k != len(r.Steps)-1 || r.Status == kit.StatusCompleted {
			return c, invalid("RESPONSE_INVALID", fmt.Errorf("step %d not completed", k))
		}
		c.Steps = append(c.Steps, cs)
	}
	// The response status and code are those of the first noncomplete tree or query; a
	// response that dropped its steps (OUTPUT_LIMIT, ALLOCATION_LIMIT, LANGUAGE_INCOMPATIBLE,
	// and in r2 a QUERY_TIME_LIMIT the runtime ran past) reports the count only.
	if r.Status != kit.StatusCompleted {
		if len(c.Steps) == 0 {
			if r.Code != "OUTPUT_LIMIT" && r.Code != "ALLOCATION_LIMIT" && r.Code != "LANGUAGE_INCOMPATIBLE" &&
				(r.Code != "QUERY_TIME_LIMIT" || req.Revision() != ProtocolR2 || len(req.Queries) == 0 || r.Status != kit.StatusResourceLimit) {
				return c, invalid("RESPONSE_INVALID", errors.New("noncomplete response without steps"))
			}
		} else {
			last := c.Steps[len(c.Steps)-1]
			status, code, bad := last.Incremental.incomplete()
			if !bad && last.Fresh != nil {
				status, code, bad = last.Fresh.incomplete()
			}
			if !bad || status != r.Status || code != r.Code {
				return c, invalid("RESPONSE_INVALID", errors.New("response status/code differ from the first noncomplete tree or query"))
			}
		}
	}
	if len(r.Steps) > 0 && completed != r.StepsCompleted {
		return c, invalid("RESPONSE_INVALID", errors.New("steps_completed"))
	}
	return c, nil
}
