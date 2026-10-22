package native

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// Driver protocol revisions: r1 (S05), r2 (S06), r3 (opt-in logger observations), r4 (the
// r2 queries and API with the r3 observations on the same parse, #217). The driver answers
// each request in the revision it was asked in.
const (
	Protocol   = nativewire.Protocol
	ProtocolR2 = nativewire.ProtocolR2
	ProtocolR3 = nativewire.ProtocolR3
	ProtocolR4 = nativewire.ProtocolR4
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

// queries reports whether the revision carries the query limits, queries and API switch (r2, r4).
func (r Request) queries() bool { return r.Revision() == ProtocolR2 || r.Revision() == ProtocolR4 }

// observes reports whether the revision carries the bounded logger observations (r3, r4).
func (r Request) observes() bool { return r.Revision() == ProtocolR3 || r.Revision() == ProtocolR4 }

// Encode renders the canonical request payload (fixed key order, no whitespace). An r1
// request is exactly the S05 payload; r2 adds the query limits, queries and the API switch.
func (r Request) Encode() []byte {
	var b bytes.Buffer
	b64 := base64.StdEncoding.EncodeToString
	r2 := r.queries()
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
	if r.observes() {
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

// The response wire types and their validation are shared with the offline replay (#251).
type (
	Response    = nativewire.Response
	Producer    = nativewire.Producer
	Step        = nativewire.Step
	StepEdit    = nativewire.StepEdit
	Route       = nativewire.Route
	WireTree    = nativewire.WireTree
	WireErrors  = nativewire.WireErrors
	WireError   = nativewire.WireError
	DeclItem    = nativewire.DeclItem
	NameSpan    = nativewire.NameSpan
	WirePartial = nativewire.WirePartial
	Tree        = nativewire.Tree
	Checked     = nativewire.Checked
	CheckedStep = nativewire.CheckedStep
)

// WireQuery is one r2 query result on one tree.
type WireQuery = querywire.Query

// QueryError is the runtime's query rejection class, byte offset and point.
type QueryError = querywire.CompileError

// WirePredicate is one predicate of a pattern as the runtime lists it.
type WirePredicate = querypolicy.Predicate

// PredStep is one predicate step: kind capture (with its quantifier) or string.
type PredStep = querypolicy.Step

// WireAPI is the tsgk-api/r1 observation of one full tree.
type WireAPI = querywire.API

// DecodeResponse strictly decodes one response payload: unknown or duplicate members,
// invalid UTF-8 and trailing values are rejected.
func DecodeResponse(payload []byte) (Response, error) {
	r, err := nativewire.DecodeResponse(payload)
	return r, wireError(err)
}

func invalid(code string, cause error) *Error { return refuse(kit.KindIO, code, cause) }

// wireError is a nativewire rejection as the native error, its tree structure cause as the
// kit error it has always been.
func wireError(err error) error {
	var we *nativewire.Error
	if !errors.As(err, &we) {
		return err
	}
	cause := we.Cause
	if te, ok := cause.(*nativewire.TreeError); ok {
		cause = &kit.Error{Kind: kit.KindInvalidInput, Code: te.Code, Path: te.Path}
	}
	return invalid(we.Code, cause)
}

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
	ns, err := nativewire.Nodes(rows, types, fields)
	return ns, wireError(err)
}

// checkTree validates a tree object against the step's source length and, for r2, the
// requested queries and API observations.
func checkTree(w WireTree, sourceBytes uint64, nodes uint64, req Request) (Tree, error) {
	e := req.expect(nil, nil, nil)
	e.Nodes = nodes
	t, err := nativewire.CheckTree(w, sourceBytes, e)
	return t, wireError(err)
}

// captureOf converts one validated capture row to the public capture.
func captureOf(q WireQuery, row []int64) kit.Capture {
	f := row[5]
	return kit.Capture{Match: uint32(row[0]), Pattern: uint32(row[1]), Capture: uint32(row[2]), Name: q.CaptureNames[row[2]], Node: row[3], Type: q.Types[row[4]],
		Named: f&1 != 0, Extra: f&2 != 0, IsError: f&4 != 0, HasError: f&8 != 0, IsMissing: f&16 != 0,
		StartByte: uint32(row[6]), EndByte: uint32(row[7]), StartPoint: kit.Point{Row: uint32(row[8]), Column: uint32(row[9])}, EndPoint: kit.Point{Row: uint32(row[10]), Column: uint32(row[11])}}
}

// pointBytes is the byte of each requested API point.
func pointBytes(points []kit.NativePoint) []uint32 {
	out := make([]uint32, len(points))
	for k, p := range points {
		out[k] = p.Byte
	}
	return out
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// expect is what a response to r must answer: the kit's own sources (versions) and edit
// points, and the driver exit when known.
func (r Request) expect(versions [][]byte, points []kit.EditPoints, exit *int) nativewire.Expect {
	e := nativewire.Expect{Revision: r.Protocol, ID: r.ID, Nodes: r.Limits.Nodes, Queries: r.Queries, API: r.API, Points: pointBytes(r.Points),
		SourceBytes: uint64(len(r.Source)), EditPoints: points, Exit: exit}
	for _, ed := range r.Edits {
		e.Edits = append(e.Edits, nativewire.EditSpan{StartByte: ed.StartByte, OldEndByte: ed.OldEndByte, NewEndByte: ed.NewEndByte})
	}
	for _, v := range versions {
		e.Sources = append(e.Sources, nativewire.SourceID{Bytes: uint64(len(v)), SHA256: sha(v)})
	}
	if r.observes() {
		e.Observation = &nativewire.Binding{Manifest: reuseobs.RuntimeManifestSHA256, RunID: r.ObservationRunID,
			ProducerSHA256: r.ObservationProducerSHA256, HistoryStart: r.ObservationHistoryStart}
	}
	return e
}

// Check validates a decoded response for req: protocol and id, status/exit agreement, the
// step sequence, each step's source identity and edit points against the kit's own
// computation, and every tree's structure.
func Check(r Response, req Request, versions [][]byte, points []kit.EditPoints, exitCode int) (Checked, error) {
	return checkExit(r, req, versions, points, &exitCode)
}

// checkExit is Check where a nil exit (a batch frame) skips only the exit agreement.
func checkExit(r Response, req Request, versions [][]byte, points []kit.EditPoints, exit *int) (Checked, error) {
	c, err := nativewire.Check(r, req.expect(versions, points, exit))
	return c, wireError(err)
}
