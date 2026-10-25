// Package querywire validates stored native query streams without starting processes.
package querywire

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
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

// ValidCompileError binds a runtime rejection class and byte point to its query source.
func ValidCompileError(e *CompileError, source []byte) bool {
	if !validCompileErrorClass(e) || uint64(e.Offset) > uint64(len(source)) {
		return false
	}
	prefix := source[:e.Offset]
	return e.Point == [2]uint32{uint32(bytes.Count(prefix, []byte{'\n'})), uint32(len(prefix) - bytes.LastIndexByte(prefix, '\n') - 1)}
}

func validCompileErrorClass(e *CompileError) bool {
	return e != nil && slices.Contains([]string{"SYNTAX", "NODE_TYPE", "FIELD", "CAPTURE", "STRUCTURE", "LANGUAGE"}, e.Type)
}

type Error struct {
	Code  string
	Cause error
}

func (e *Error) Error() string                { return e.Code + ": " + e.Cause.Error() }
func (e *Error) Unwrap() error                { return e.Cause }
func invalid(code string, cause error) *Error { return &Error{Code: code, Cause: cause} }
func isNull(v jsontext.Value) bool            { return bytes.Equal(bytes.Trim(v, " \t\r\n"), []byte("null")) }

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
	return decode(raw, sources, sourceBytes, descendantCount, link, true)
}

// DecodeSummary validates the wire stream only; it supplies no full-node link evidence.
// The caller bounds raw bytes and independently validates the tree summary and count.
func DecodeSummary(raw jsontext.Value, sources []Source, sourceBytes, descendantCount uint64) ([]Query, *Error) {
	return decode(raw, sources, sourceBytes, descendantCount, nil, true)
}

// DecodeIntrinsic checks status, metadata, accounting and optional node links without
// query source identities or compile-error byte/point proof. A nil link is summary-only.
// The caller bounds raw bytes and validates the declared tree byte/count bounds.
func DecodeIntrinsic(raw jsontext.Value, sourceBytes, descendantCount uint64, link func(Query, []int64) bool) ([]Query, *Error) {
	return decode(raw, nil, sourceBytes, descendantCount, link, false)
}

var queryLimitCodes = map[string]bool{"QUERY_TIME_LIMIT": true, "MATCH_LIMIT": true, "CAPTURE_LIMIT": true, "QUERY_MATCH_OVERFLOW": true}

// ParseQueries checks only the JSON shape, before semantic validation or capture linking.
// Callers bound input and repeated work before calling DecodeFull or DecodeSummary.
func ParseQueries(raw jsontext.Value) ([]Query, *Error) {
	var queries []Query
	if isNull(raw) {
		return nil, nil
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
	if err := rejectNullPrimitives(raw); err != nil {
		return nil, invalid("RESPONSE_MALFORMED", err)
	}
	return queries, nil
}

func decode(raw jsontext.Value, sources []Source, sourceBytes, descendantCount uint64, link func(Query, []int64) bool, sourceKnown bool) ([]Query, *Error) {
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
	var shapeError *Error
	queries, shapeError = ParseQueries(raw)
	if shapeError != nil {
		return nil, shapeError
	}
	if sourceKnown && len(queries) != len(sources) {
		return nil, bad(len(queries), "count")
	}
	for i, q := range queries {
		if sourceKnown && q.ID != sources[i].ID {
			return nil, bad(i, "id")
		}
		for _, p := range q.Predicates {
			for _, st := range p.Steps {
				if (st.Kind != "capture" || st.Quantifier == nil) && (st.Kind != "string" || st.Quantifier != nil) {
					return nil, bad(i, "predicate step")
				}
			}
		}
		if !ValidMetadata(q.Patterns, q.CaptureNames, q.Predicates) {
			return nil, bad(i, "predicate")
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
			// the producer skips the capture rows, so it never interns a type
			if q.Code != "CAPTURE_NODE_UNMAPPED" || q.Error != nil || q.Captures != nil || q.Partial || q.Types == nil || len(q.Types) != 0 {
				return nil, bad(i, "failed shape")
			}
			continue
		case "INVALID_QUERY":
			if !validCompileErrorClass(q.Error) || sourceKnown && !ValidCompileError(q.Error, sources[i].Source) || q.Code != "QUERY_"+q.Error.Type || q.Captures != nil || q.Patterns != 0 || q.Partial || q.QueryMillis != 0 || q.Matches != 0 || len(q.Types) != 0 {
				return nil, bad(i, "invalid shape")
			}
			continue
		default:
			return nil, bad(i, "status "+strconv.Quote(q.Status))
		}
		if !MatchAccounting(q.Matches, len(q.Captures), func(index int) (int64, int64) {
			row := q.Captures[index]
			if len(row) != 12 {
				return -1, -1
			}
			return row[0], row[1]
		}) {
			return nil, bad(i, "match accounting")
		}
		// the producer interns each emitted capture's type in first-use order; a MISSING node always has an error
		types := map[string]bool{}
		for k, row := range q.Captures {
			if len(row) != 12 || row[0] < 0 || uint64(row[0]) >= q.Matches || row[1] < 0 || row[1] >= int64(q.Patterns) || row[2] < 0 || row[2] >= int64(len(q.CaptureNames)) ||
				row[3] < 0 || uint64(row[3]) >= descendantCount || row[4] < 0 || row[4] >= int64(len(q.Types)) || row[5] < 0 || row[5] > 31 || row[5]&16 != 0 && row[5]&8 == 0 {
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
			if row[4] == int64(len(types)) {
				if types[q.Types[row[4]]] {
					return nil, bad(i, fmt.Sprintf("capture %d type table", k))
				}
				types[q.Types[row[4]]] = true
			} else if row[4] > int64(len(types)) {
				return nil, bad(i, fmt.Sprintf("capture %d type table", k))
			}
			if link != nil && !link(q, row) {
				return nil, invalid("CAPTURE_LINK_MISMATCH", fmt.Errorf("query %d capture %d node %d", i, k, row[3]))
			}
		}
		if len(types) != len(q.Types) {
			return nil, bad(i, "type table")
		}
	}
	return queries, nil
}

// ValidMetadata reports whether metadata is what a compiled query yields: unique non-empty
// capture names, and predicates in pattern order whose captures name one of them.
func ValidMetadata(patterns uint32, names []string, predicates []querypolicy.Predicate) bool {
	if patterns == 0 && len(names) != 0 { // the runtime parses captures only inside a pattern
		return false
	}
	seen := map[string]bool{}
	for _, name := range names {
		if name == "" || seen[name] {
			return false
		}
		seen[name] = true
	}
	for i, p := range predicates {
		if i > 0 && p.Pattern < predicates[i-1].Pattern {
			return false
		}
		for _, step := range p.Steps {
			if step.Kind == "capture" && !seen[step.Value] {
				return false
			}
		}
	}
	return querypolicy.Support(patterns, predicates, "UTF-8") != querypolicy.InvalidMetadata
}

// MatchAccounting checks an unfiltered structural stream without reordering captures.
// Caller bounds count and supplies match/pattern pairs; evaluated streams may have gaps.
func MatchAccounting(matches uint64, count int, at func(int) (int64, int64)) bool {
	return matchAccounting(matches, count, at, false)
}

// FilteredMatchAccounting permits rejected matches but preserves first appearance
// order and the one pattern per surviving match contract, including interleaving.
func FilteredMatchAccounting(matches uint64, count int, at func(int) (int64, int64)) bool {
	return matchAccounting(matches, count, at, true)
}

func matchAccounting(matches uint64, count int, at func(int) (int64, int64), filtered bool) bool {
	if count < 0 || at == nil || !filtered && matches > uint64(count) {
		return false
	}
	seen := int64(-1)
	patterns := map[int64]int64{}
	for index := 0; index < count; index++ {
		match, pattern := at(index)
		if match < 0 || pattern < 0 || uint64(match) >= matches {
			return false
		}
		if previous, ok := patterns[match]; ok {
			if previous != pattern {
				return false
			}
		} else if match <= seen || !filtered && match != seen+1 {
			return false
		}
		patterns[match] = pattern
		seen = max(seen, match)
	}
	return filtered || matches == uint64(seen+1)
}

// Typed JSON decoding accepts null as a scalar zero value. Only nullable schema
// members may contain it; typed decoding above already fixes their object locations.
func rejectNullPrimitives(raw jsontext.Value) error {
	d := jsontext.NewDecoder(bytes.NewReader(raw))
	var walk func(bool) error
	walk = func(nullable bool) error {
		token, err := d.ReadToken()
		if err != nil {
			return err
		}
		switch token.Kind() {
		case 'n':
			if !nullable {
				return errors.New("null primitive in query response")
			}
		case '[':
			for d.PeekKind() != ']' {
				if err := walk(false); err != nil {
					return err
				}
			}
			_, err = d.ReadToken()
		case '{':
			for d.PeekKind() != '}' {
				name, e := d.ReadToken()
				if e != nil {
					return e
				}
				allowNull := false
				switch name.String() {
				case "error", "capture_names", "predicates", "types", "captures", "quantifier":
					allowNull = true
				}
				if err := walk(allowNull); err != nil {
					return err
				}
			}
			_, err = d.ReadToken()
		}
		return err
	}
	return walk(false)
}

// API is the tsgk-api/r1 observation of one full tree.
type API struct {
	Revision     string    `json:"revision"`
	Nodes        [][]int64 `json:"nodes"`
	ChildFields  [][]int64 `json:"child_fields"`
	FieldLookups [][]int64 `json:"field_lookups"`
	Points       [][]int64 `json:"points"`
	Names        []string  `json:"names"`
}

// DecodeAPI validates the shape of the API observation of a full tree of nodes nodes for
// the requested point bytes: every member, the revision, one 13-value row per node, a row
// per point at its byte, and every table row's arity and indices in range. Whether the
// values agree with the tree is the caller's.
func DecodeAPI(raw jsontext.Value, nodes int64, points []uint32) (*API, *Error) {
	bad := func(what string) *Error { return invalid("RESPONSE_INVALID", errors.New("api: "+what)) }
	if err := RequireMembers(raw, false, "revision", "nodes", "child_fields", "field_lookups", "points", "names"); err != nil {
		return nil, err
	}
	var a API
	if err := jsonv2.Unmarshal(raw, &a, jsonv2.RejectUnknownMembers(true)); err != nil {
		return nil, invalid("RESPONSE_MALFORMED", err)
	}
	if a.Revision != "tsgk-api/r1" || int64(len(a.Nodes)) != nodes || len(a.Points) != len(points) {
		return nil, bad("header")
	}
	for _, r := range a.Nodes {
		if len(r) != 13 {
			return nil, bad("node row")
		}
	}
	for _, r := range a.ChildFields {
		if len(r) != 3 || r[0] < 0 || r[0] >= nodes || r[1] < 0 || r[2] < 0 || r[2] >= int64(len(a.Names)) {
			return nil, bad("child field row")
		}
	}
	for _, r := range a.FieldLookups {
		if len(r) != 3 || r[0] < 0 || r[0] >= nodes || r[1] < 0 || r[1] >= int64(len(a.Names)) {
			return nil, bad("field lookup row")
		}
	}
	for k, r := range a.Points {
		if len(r) != 4 || r[0] != int64(points[k]) {
			return nil, bad("point row")
		}
	}
	return &a, nil
}
