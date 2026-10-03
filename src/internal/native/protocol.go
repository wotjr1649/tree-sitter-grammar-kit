package native

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"strconv"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// Protocol is the driver protocol revision.
const Protocol = "tsgk-native/r1"

// MaxRequestFrame is the request frame payload ceiling.
const MaxRequestFrame = 50331648

// Limits are the per-request driver limits.
type Limits struct {
	InputBytes, Nodes, FullNodes, Depth, OutputBytes, ParseMillis, MemoryBytes, Errors, PartialNodes uint64
}

// LimitsFor returns the request limits of an operation.
func LimitsFor(op kit.NativeOperation) Limits {
	return Limits{op.InputBytes, op.Nodes, op.FullNodes, op.Depth, op.OutputBytes, op.ParseMillis, op.MemoryBytes, op.Errors, op.PartialNodes}
}

// Request is one driver request.
type Request struct {
	ID           string
	Encoding     string
	Output       string
	Limits       Limits
	Declarations []kit.NativeDeclaration
	Points       []kit.NativePoint
	Source       []byte
	Edits        []kit.Edit
}

// Encode renders the canonical request payload (fixed key order, no whitespace).
func (r Request) Encode() []byte {
	var b bytes.Buffer
	b64 := base64.StdEncoding.EncodeToString
	fmt.Fprintf(&b, `{"protocol":%q,"id":%q,"encoding":%q,"output":%q,`, Protocol, r.ID, r.Encoding, r.Output)
	l := r.Limits
	fmt.Fprintf(&b, `"limits":{"input_bytes":%d,"nodes":%d,"full_nodes":%d,"depth":%d,"output_bytes":%d,"parse_ms":%d,"memory_bytes":%d,"errors":%d,"partial_nodes":%d},`,
		l.InputBytes, l.Nodes, l.FullNodes, l.Depth, l.OutputBytes, l.ParseMillis, l.MemoryBytes, l.Errors, l.PartialNodes)
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
	fmt.Fprintf(&b, `],"source":"%s","edits":[`, b64(r.Source))
	for i, e := range r.Edits {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"start_byte":%d,"old_end_byte":%d,"new_end_byte":%d,"old":"%s","new":"%s"}`, e.StartByte, e.OldEndByte, e.NewEndByte, b64(e.Old), b64(e.New))
	}
	b.WriteString("]}")
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

// Producer is the runtime and grammar ABI the driver reports.
type Producer struct {
	LanguageVersion        uint32 `json:"language_version"`
	RuntimeLanguageVersion uint32 `json:"runtime_language_version"`
	RuntimeMinCompatible   uint32 `json:"runtime_min_compatible"`
	Query                  string `json:"query"`
}

// Step is one parse step of a response.
type Step struct {
	Step         int       `json:"step"`
	SourceBytes  uint64    `json:"source_bytes"`
	SourceSHA256 string    `json:"source_sha256"`
	Edit         *StepEdit `json:"edit"`
	Route        *Route    `json:"route"`
	Incremental  WireTree  `json:"incremental"`
	Fresh        *WireTree `json:"fresh"`
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
	Status          string        `json:"status"`
	Code            string        `json:"code"`
	ParseMillis     uint64        `json:"parse_ms"`
	Form            *string       `json:"form"`
	Reason          string        `json:"reason"`
	DescendantCount uint64        `json:"descendant_count"`
	MaxDepth        uint64        `json:"max_depth"`
	HasError        bool          `json:"has_error"`
	Digest          string        `json:"digest"`
	Types           []string      `json:"types"`
	Fields          []string      `json:"fields"`
	Nodes           [][]int64     `json:"nodes"`
	Errors          *WireErrors   `json:"errors"`
	Declarations    *[]DeclItem   `json:"declarations"`
	PartialTrees    []WirePartial `json:"partial_trees"`
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
	out := make([]kit.TreeNode, 0, len(rows))
	for i, r := range rows {
		bad := func(what string) error {
			return invalid("RESPONSE_INVALID", fmt.Errorf("node %d: %s", i, what))
		}
		if len(r) != 10 {
			return nil, bad("arity")
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
}

// checkTree validates a tree object against the step's source length.
func checkTree(w WireTree, sourceBytes uint64, nodes uint64) (Tree, error) {
	t := Tree{Status: w.Status, Code: w.Code, Wire: w}
	switch w.Status {
	case kit.StatusCompleted:
		if w.Code != "" || w.Form == nil {
			return t, invalid("RESPONSE_INVALID", errors.New("completed tree without form"))
		}
	case kit.StatusResourceLimit, kit.StatusFailed:
		if w.Code == "" || w.Form != nil || w.Nodes != nil {
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
			if len(ns) > 0 && !p.Truncated {
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
	return t, nil
}

// Checked is a response validated against its request and the kit's own intermediate
// sources and edit points.
type Checked struct {
	Response Response
	Steps    []CheckedStep
}

// CheckedStep is one validated step.
type CheckedStep struct {
	Step        Step
	Incremental Tree
	Fresh       *Tree
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
	if r.Protocol != Protocol || !r.Complete {
		return c, invalid("RESPONSE_INVALID", errors.New("protocol or completeness"))
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
	if r.Producer.Query != "UNSUPPORTED" {
		return c, invalid("RESPONSE_INVALID", errors.New("query capability"))
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
		inc, err := checkTree(s.Incremental, s.SourceBytes, req.Limits.Nodes)
		if err != nil {
			return c, err
		}
		cs := CheckedStep{Step: s, Incremental: inc}
		if s.Fresh != nil {
			f, err := checkTree(*s.Fresh, s.SourceBytes, req.Limits.Nodes)
			if err != nil {
				return c, err
			}
			cs.Fresh = &f
		}
		stepDone := inc.Status == kit.StatusCompleted && (k == 0 || (cs.Fresh != nil && cs.Fresh.Status == kit.StatusCompleted))
		if stepDone {
			completed++
		} else if k != len(r.Steps)-1 || r.Status == kit.StatusCompleted {
			return c, invalid("RESPONSE_INVALID", fmt.Errorf("step %d not completed", k))
		}
		c.Steps = append(c.Steps, cs)
	}
	// A response that dropped its steps (OUTPUT_LIMIT, ALLOCATION_LIMIT) reports the count only.
	if len(r.Steps) > 0 && completed != r.StepsCompleted {
		return c, invalid("RESPONSE_INVALID", errors.New("steps_completed"))
	}
	return c, nil
}
