package nativewire

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

// Driver protocol revisions: r1 (S05), r2 (S06), r3 (opt-in logger observations), r4 (the
// r2 queries and API with the r3 observations on the same parse, #217). The driver answers
// each request in the revision it was asked in.
const (
	Protocol   = "tsgk-native/r1"
	ProtocolR2 = "tsgk-native/r2"
	ProtocolR3 = "tsgk-native/r3"
	ProtocolR4 = "tsgk-native/r4"
)

// Execution statuses of a response, a tree and a query; the kit's constants of the same names.
const (
	StatusCompleted     = "COMPLETED"
	StatusResourceLimit = "RESOURCE_LIMIT"
	StatusFailed        = "FAILED"
)

// Error is a rejected response: the native code and its cause. A tree structure violation
// is a RESPONSE_INVALID with a *TreeError cause.
type Error struct {
	Code  string
	Cause error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Cause.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Cause }

func invalid(code string, cause error) *Error { return &Error{Code: code, Cause: cause} }

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

// Nodes converts compact node arrays to public nodes and checks the name-table indices.
func Nodes(rows [][]int64, types, fields []string) ([]TreeNode, error) {
	return nodes(rows, types, fields, nil)
}

func nodes(rows [][]int64, types, fields []string, check func() error) ([]TreeNode, error) {
	for _, field := range fields {
		if check != nil {
			if err := check(); err != nil {
				return nil, err
			}
		}
		if field == "" {
			return nil, invalid("RESPONSE_INVALID", errors.New("empty field name"))
		}
	}
	out := make([]TreeNode, 0, len(rows))
	for i, r := range rows {
		if check != nil {
			if err := check(); err != nil {
				return nil, err
			}
		}
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
		n := TreeNode{Parent: r[0], Type: types[r[1]], Named: r[3]&1 != 0, Extra: r[3]&2 != 0, IsError: r[3]&4 != 0, HasError: r[3]&8 != 0, IsMissing: r[3]&16 != 0,
			StartByte: uint32(r[4]), EndByte: uint32(r[5]), StartPoint: Point{Row: uint32(r[6]), Column: uint32(r[7])}, EndPoint: Point{Row: uint32(r[8]), Column: uint32(r[9])}}
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

// WireQuery is one r2 query result on one tree.
type WireQuery = querywire.Query

// WireAPI is the tsgk-api/r1 observation of one full tree.
type WireAPI = querywire.API
