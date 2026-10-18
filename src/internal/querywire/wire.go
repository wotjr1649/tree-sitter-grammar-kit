// Package querywire validates stored native query streams without starting processes.
package querywire

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"strconv"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/querypolicy"
)

// Source is the caller-selected query identity and bytes.
type Source struct {
	ID     string
	Source []byte
}

// Query is one r2 query result on one tree.
type Query struct {
	ID           string                  `json:"id"`
	Status       string                  `json:"status"`
	Code         string                  `json:"code"`
	QueryMillis  uint64                  `json:"query_ms"`
	Error        *CompileError           `json:"error"`
	Patterns     uint32                  `json:"patterns"`
	CaptureNames []string                `json:"capture_names"`
	Predicates   []querypolicy.Predicate `json:"predicates"`
	Matches      uint64                  `json:"matches"`
	Partial      bool                    `json:"partial"`
	Types        []string                `json:"types"`
	Captures     [][]int64               `json:"captures"`
}

// CompileError is the runtime's rejection of a query: class, byte offset and point in the
// query source.
type CompileError struct {
	Type   string    `json:"type"`
	Offset uint32    `json:"offset"`
	Point  [2]uint32 `json:"point"`
}

type Error struct {
	Code  string
	Cause error
}

func (e *Error) Error() string                { return e.Code + ": " + e.Cause.Error() }
func (e *Error) Unwrap() error                { return e.Cause }
func invalid(code string, cause error) *Error { return &Error{Code: code, Cause: cause} }
func isNull(v jsontext.Value) bool            { return string(v) == "null" }

// RequireMembers rejects missing members before typed decoding supplies zero values.
func RequireMembers(raw jsontext.Value, array bool, names ...string) *Error {
	var objs []map[string]jsontext.Value
	if array {
		if err := jsonv2.Unmarshal(raw, &objs); err != nil {
			return invalid("RESPONSE_MALFORMED", err)
		}
	} else {
		var one map[string]jsontext.Value
		if err := jsonv2.Unmarshal(raw, &one); err != nil {
			return invalid("RESPONSE_MALFORMED", err)
		}
		objs = append(objs, one)
	}
	for i, o := range objs {
		for _, n := range names {
			if _, ok := o[n]; !ok {
				return invalid("RESPONSE_INVALID", fmt.Errorf("object %d lacks %s", i, n))
			}
		}
	}
	return nil
}

// DecodeFull validates the wire stream and its links to an independently validated full
// tree. The caller bounds raw bytes and supplies the tree's exact descendant count;
// link compares every node type, flag, byte and point field for an in-range row.
// A nil link is an error, including when the stream is empty. No summary fallback occurs.
func DecodeFull(raw jsontext.Value, sources []Source, sourceBytes, descendantCount uint64, link func(Query, []int64) bool) ([]Query, *Error) {
	if link == nil {
		return nil, invalid("RESPONSE_INVALID", errors.New("full query validation requires node links"))
	}
	return decode(raw, sources, sourceBytes, descendantCount, link)
}

// DecodeSummary validates the wire stream only; it supplies no full-node link evidence.
// The caller bounds raw bytes and independently validates the tree summary and count.
func DecodeSummary(raw jsontext.Value, sources []Source, sourceBytes, descendantCount uint64) ([]Query, *Error) {
	return decode(raw, sources, sourceBytes, descendantCount, nil)
}

var queryLimitCodes = map[string]bool{"QUERY_TIME_LIMIT": true, "MATCH_LIMIT": true, "CAPTURE_LIMIT": true, "QUERY_MATCH_OVERFLOW": true}

func decode(raw jsontext.Value, sources []Source, sourceBytes, descendantCount uint64, link func(Query, []int64) bool) ([]Query, *Error) {
	var queries []Query
	bad := func(i int, what string) *Error {
		return invalid("RESPONSE_INVALID", fmt.Errorf("query %d: %s", i, what))
	}
	if isNull(raw) {
		if len(sources) != 0 {
			return nil, bad(0, "missing")
		}
		return queries, nil
	}
	if err := RequireMembers(raw, true, "id", "status", "code", "query_ms", "error", "patterns", "capture_names", "predicates", "matches", "partial", "types", "captures"); err != nil {
		return nil, err
	}
	// the nested objects too: a predicate without its pattern or a step without its kind
	// must not default to pattern 0 or an empty step
	var nested []struct {
		Error      jsontext.Value `json:"error"`
		Predicates jsontext.Value `json:"predicates"`
	}
	if err := jsonv2.Unmarshal(raw, &nested); err != nil {
		return nil, invalid("RESPONSE_MALFORMED", err)
	}
	for _, q := range nested {
		if !isNull(q.Error) {
			if err := RequireMembers(q.Error, false, "type", "offset", "point"); err != nil {
				return nil, err
			}
		}
		if isNull(q.Predicates) {
			continue
		}
		if err := RequireMembers(q.Predicates, true, "pattern", "steps"); err != nil {
			return nil, err
		}
		var preds []struct {
			Steps jsontext.Value `json:"steps"`
		}
		if err := jsonv2.Unmarshal(q.Predicates, &preds); err != nil {
			return nil, invalid("RESPONSE_MALFORMED", err)
		}
		for _, p := range preds {
			if err := RequireMembers(p.Steps, true, "kind", "value", "quantifier"); err != nil {
				return nil, err
			}
		}
	}
	if err := jsonv2.Unmarshal(raw, &queries, jsonv2.RejectUnknownMembers(true)); err != nil {
		return nil, invalid("RESPONSE_MALFORMED", err)
	}
	if len(queries) != len(sources) {
		return nil, bad(len(queries), "count")
	}
	for i, q := range queries {
		if q.ID != sources[i].ID {
			return nil, bad(i, "id")
		}
		switch q.Status {
		case "COMPLETED":
			if q.Code != "" || q.Error != nil || q.Captures == nil || q.Partial || q.Predicates == nil || q.CaptureNames == nil || q.Types == nil {
				return nil, bad(i, "completed shape")
			}
		case "RESOURCE_LIMIT":
			if !queryLimitCodes[q.Code] || q.Error != nil || q.Captures == nil || !q.Partial || q.Predicates == nil || q.CaptureNames == nil || q.Types == nil {
				return nil, bad(i, "limit shape")
			}
		case "FAILED":
			if q.Code != "CAPTURE_NODE_UNMAPPED" || q.Error != nil || q.Captures != nil || q.Partial {
				return nil, bad(i, "failed shape")
			}
			continue
		case "INVALID_QUERY":
			if q.Error == nil || q.Code != "QUERY_"+q.Error.Type || q.Captures != nil || q.Patterns != 0 || q.Partial || uint64(q.Error.Offset) > uint64(len(sources[i].Source)) {
				return nil, bad(i, "invalid shape")
			}
			continue
		default:
			return nil, bad(i, "status "+strconv.Quote(q.Status))
		}
		for _, p := range q.Predicates {
			if p.Pattern >= q.Patterns || len(p.Steps) == 0 {
				return nil, bad(i, "predicate")
			}
			for _, st := range p.Steps {
				if (st.Kind != "capture" || st.Quantifier == nil) && (st.Kind != "string" || st.Quantifier != nil) {
					return nil, bad(i, "predicate step")
				}
			}
		}
		seen := int64(-1)
		patternOf := map[int64]int64{}
		for k, row := range q.Captures {
			if len(row) != 12 || row[0] < 0 || uint64(row[0]) >= q.Matches || row[1] < 0 || row[1] >= int64(q.Patterns) || row[2] < 0 || row[2] >= int64(len(q.CaptureNames)) ||
				row[3] < 0 || uint64(row[3]) >= descendantCount || row[4] < 0 || row[4] >= int64(len(q.Types)) || row[5] < 0 || row[5] > 31 {
				return nil, bad(i, fmt.Sprintf("capture %d indices", k))
			}
			for _, v := range row[6:] {
				if v < 0 || v > 0xffffffff {
					return nil, bad(i, fmt.Sprintf("capture %d range", k))
				}
			}
			if row[6] > row[7] || uint64(row[7]) > sourceBytes {
				return nil, bad(i, fmt.Sprintf("capture %d range", k))
			}
			// the runtime numbers a match when it first returns one of its captures
			if row[0] > seen+1 {
				return nil, bad(i, fmt.Sprintf("capture %d match order", k))
			}
			seen = max(seen, row[0])
			// one match belongs to one pattern
			if p, ok := patternOf[row[0]]; ok && p != row[1] {
				return nil, bad(i, fmt.Sprintf("capture %d pattern of match", k))
			}
			patternOf[row[0]] = row[1]
			if link != nil && !link(q, row) {
				return nil, invalid("CAPTURE_LINK_MISMATCH", fmt.Errorf("query %d capture %d node %d", i, k, row[3]))
			}
		}
		// the match count is the number of matches the stream returned
		if q.Matches != uint64(seen+1) {
			return nil, bad(i, "match count")
		}
	}
	return queries, nil
}
