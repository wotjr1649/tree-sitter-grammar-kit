package kit

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// This file derives the independent expectations for the ordinary full-positive JSON regression
// from the JSON grammar only. It never reads a native or reference output.
//
// Grammar facts used (from the pinned tracked parser.c, src/testdata/ordinary-json/parser.c):
//   - extras are the whitespace characters ' ', \t, \n, \r; `document` is exactly one value
//     (there is no document repeat symbol), so the root spans the whole input including trailing
//     whitespace, as in the PostgreSQL DO expectation.
//   - object  = "{" [pair {"," pair}] "}"          (aux_sym_object_repeat1)
//     pair    = key:string ":" value:_value        (field_key child 0, field_value child 2)
//     array   = "[" [_value {"," _value}] "]"      (aux_sym_array_repeat1)
//     string  = "\"" {string_content | escape_sequence} "\""
//   - string_content is a maximal run that starts with ' ' or a character above ' ', continues
//     over characters above 0x1f, and stops at '"' or '\\' (lex states 31/32).
//   - escape_sequence is '\\' followed by one of "/\\bfnrt, or by 'u' and four hex digits (state 12..21).
//   - number is -?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)? (states 3, 15..17, 34..37).
//   - true, false and null are named leaf nodes.
type ordinaryJSONExpectedNode struct {
	typ        string
	named      bool
	field      string
	start, end int
	kids       []*ordinaryJSONExpectedNode
}

type ordinaryJSONExpectedParser struct {
	src []byte
	pos int
	err error
}

func (p *ordinaryJSONExpectedParser) fail(format string, args ...any) {
	if p.err == nil {
		p.err = fmt.Errorf("offset %d: "+format, append([]any{p.pos}, args...)...)
	}
}

func (p *ordinaryJSONExpectedParser) skipSpace() {
	for p.pos < len(p.src) && strings.IndexByte(" \t\r\n", p.src[p.pos]) >= 0 {
		p.pos++
	}
}

func (p *ordinaryJSONExpectedParser) token(text string, named bool, typ string) *ordinaryJSONExpectedNode {
	p.skipSpace()
	if p.pos+len(text) > len(p.src) || string(p.src[p.pos:p.pos+len(text)]) != text {
		p.fail("expected %q", text)
		return &ordinaryJSONExpectedNode{typ: typ, named: named, start: p.pos, end: p.pos}
	}
	n := &ordinaryJSONExpectedNode{typ: typ, named: named, start: p.pos, end: p.pos + len(text)}
	p.pos += len(text)
	return n
}

func (p *ordinaryJSONExpectedParser) punct(text string) *ordinaryJSONExpectedNode {
	return p.token(text, false, text)
}

func (p *ordinaryJSONExpectedParser) peek() byte {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

func (p *ordinaryJSONExpectedParser) value() *ordinaryJSONExpectedNode {
	switch c := p.peek(); {
	case c == '{':
		n := &ordinaryJSONExpectedNode{typ: "object", named: true}
		n.kids = append(n.kids, p.punct("{"))
		n.start = n.kids[0].start
		if p.peek() != '}' {
			for p.err == nil {
				n.kids = append(n.kids, p.pair())
				if p.peek() != ',' {
					break
				}
				n.kids = append(n.kids, p.punct(","))
			}
		}
		n.kids = append(n.kids, p.punct("}"))
		n.end = n.kids[len(n.kids)-1].end
		return n
	case c == '[':
		n := &ordinaryJSONExpectedNode{typ: "array", named: true}
		n.kids = append(n.kids, p.punct("["))
		n.start = n.kids[0].start
		if p.peek() != ']' {
			for p.err == nil {
				n.kids = append(n.kids, p.value())
				if p.peek() != ',' {
					break
				}
				n.kids = append(n.kids, p.punct(","))
			}
		}
		n.kids = append(n.kids, p.punct("]"))
		n.end = n.kids[len(n.kids)-1].end
		return n
	case c == '"':
		return p.str()
	case c == '-' || c >= '0' && c <= '9':
		return p.number()
	case strings.HasPrefix(string(p.src[p.pos:]), "true"):
		return p.token("true", true, "true")
	case strings.HasPrefix(string(p.src[p.pos:]), "false"):
		return p.token("false", true, "false")
	case strings.HasPrefix(string(p.src[p.pos:]), "null"):
		return p.token("null", true, "null")
	}
	p.fail("unexpected input; the expectation covers error-free JSON only")
	return &ordinaryJSONExpectedNode{typ: "ERROR", start: p.pos, end: p.pos}
}

func (p *ordinaryJSONExpectedParser) pair() *ordinaryJSONExpectedNode {
	n := &ordinaryJSONExpectedNode{typ: "pair", named: true}
	if p.peek() != '"' {
		p.fail("pair key must be a string")
		return n
	}
	key := p.str()
	key.field = "key"
	n.kids = append(n.kids, key, p.punct(":"))
	value := p.value()
	value.field = "value"
	n.kids = append(n.kids, value)
	n.start, n.end = key.start, value.end
	return n
}

func (p *ordinaryJSONExpectedParser) number() *ordinaryJSONExpectedNode {
	p.skipSpace()
	start := p.pos
	digits := func() int {
		n := 0
		for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			p.pos++
			n++
		}
		return n
	}
	if p.pos < len(p.src) && p.src[p.pos] == '-' {
		p.pos++
	}
	switch {
	case p.pos < len(p.src) && p.src[p.pos] == '0':
		p.pos++
	case p.pos < len(p.src) && p.src[p.pos] >= '1' && p.src[p.pos] <= '9':
		digits()
	default:
		p.fail("malformed number")
	}
	if p.pos+1 < len(p.src) && p.src[p.pos] == '.' && p.src[p.pos+1] >= '0' && p.src[p.pos+1] <= '9' {
		p.pos++
		digits()
	}
	if p.pos < len(p.src) && (p.src[p.pos] == 'e' || p.src[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.src) && (p.src[p.pos] == '+' || p.src[p.pos] == '-') {
			p.pos++
		}
		if digits() == 0 {
			p.fail("malformed exponent")
		}
	}
	return &ordinaryJSONExpectedNode{typ: "number", named: true, start: start, end: p.pos}
}

func (p *ordinaryJSONExpectedParser) str() *ordinaryJSONExpectedNode {
	open := p.punct("\"")
	n := &ordinaryJSONExpectedNode{typ: "string", named: true, start: open.start, kids: []*ordinaryJSONExpectedNode{open}}
	for p.err == nil {
		if p.pos >= len(p.src) {
			p.fail("unterminated string")
			break
		}
		c := p.src[p.pos]
		switch {
		case c == '"':
			closeQuote := &ordinaryJSONExpectedNode{typ: "\"", start: p.pos, end: p.pos + 1}
			p.pos++
			n.kids = append(n.kids, closeQuote)
			n.end = closeQuote.end
			return n
		case c == '\\':
			end := p.pos + 2
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == 'u' {
				end = p.pos + 6
			} else if p.pos+1 >= len(p.src) || strings.IndexByte("\"/\\bfnrt", p.src[p.pos+1]) < 0 {
				p.fail("unsupported escape")
				return n
			}
			if end > len(p.src) {
				p.fail("short escape")
				return n
			}
			if p.src[p.pos+1] == 'u' && strings.Trim(string(p.src[p.pos+2:end]), "0123456789abcdefABCDEF") != "" {
				p.fail("unicode escape needs four hex digits")
				return n
			}
			n.kids = append(n.kids, &ordinaryJSONExpectedNode{typ: "escape_sequence", named: true, start: p.pos, end: end})
			p.pos = end
		case c == '\t' || c == '\r' || c == '\n' || c < 0x20:
			p.fail("control character inside a string is outside this expectation")
			return n
		default:
			start := p.pos
			for p.pos < len(p.src) && p.src[p.pos] > 0x1f && p.src[p.pos] != '"' && p.src[p.pos] != '\\' {
				p.pos++
			}
			n.kids = append(n.kids, &ordinaryJSONExpectedNode{typ: "string_content", named: true, start: start, end: p.pos})
		}
	}
	return n
}

func ordinaryJSONExpectedPoint(src []byte, offset int) Point {
	var point Point
	for _, value := range src[:offset] {
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}

// ordinaryJSONExpectedTree returns the preorder tree of an error-free JSON document.
func ordinaryJSONExpectedTree(src []byte) ([]TreeNode, error) {
	p := &ordinaryJSONExpectedParser{src: src}
	if len(src) == 0 || strings.IndexByte(" \t\r\n", src[0]) >= 0 {
		return nil, fmt.Errorf("leading whitespace changes the root start; outside this expectation")
	}
	root := &ordinaryJSONExpectedNode{typ: "document", named: true, start: 0, end: len(src)}
	root.kids = []*ordinaryJSONExpectedNode{p.value()}
	p.skipSpace()
	if p.err == nil && p.pos != len(src) {
		p.fail("a second value after the document value is an ERROR in this grammar")
	}
	if p.err != nil {
		return nil, p.err
	}
	var out []TreeNode
	var walk func(n *ordinaryJSONExpectedNode, parent int64)
	walk = func(n *ordinaryJSONExpectedNode, parent int64) {
		node := TreeNode{Parent: parent, Type: n.typ, Named: n.named, StartByte: uint32(n.start), EndByte: uint32(n.end),
			StartPoint: ordinaryJSONExpectedPoint(src, n.start), EndPoint: ordinaryJSONExpectedPoint(src, n.end)}
		if n.field != "" {
			field := n.field
			node.Field = &field
		}
		index := int64(len(out))
		out = append(out, node)
		for _, kid := range n.kids {
			walk(kid, index)
		}
	}
	walk(root, -1)
	return out, nil
}

// ordinaryJSONExpectedCaptures evaluates q.structure over an expected tree. The three patterns
// are (pair key: (string) @array_key value: (array)), (... @object_key value: (object)) and
// (... @key value: (string) @value). A pair has exactly one value, so at most one pattern matches
// it; matches are numbered by first appearance and captures follow the start byte of the node,
// which is pair preorder because a matching pair's captures end before the next pair starts.
func ordinaryJSONExpectedCaptures(tree []TreeNode) []Capture {
	children := map[int64][]int{}
	for i, node := range tree {
		children[node.Parent] = append(children[node.Parent], i)
	}
	out := []Capture{}
	capture := func(match, pattern, index uint32, name string, node int) Capture {
		n := tree[node]
		return Capture{Match: match, Pattern: pattern, Capture: index, Name: name, Node: int64(node), Type: n.Type, Named: n.Named,
			StartByte: n.StartByte, EndByte: n.EndByte, StartPoint: n.StartPoint, EndPoint: n.EndPoint}
	}
	var matches uint32
	for i, node := range tree {
		if node.Type != "pair" {
			continue
		}
		kids := children[int64(i)]
		key, value := kids[0], kids[2]
		switch tree[value].Type {
		case "array":
			out = append(out, capture(matches, 0, 0, "array_key", key))
		case "object":
			out = append(out, capture(matches, 1, 1, "object_key", key))
		case "string":
			out = append(out, capture(matches, 2, 2, "key", key), capture(matches, 2, 3, "value", value))
		default:
			continue
		}
		matches++
	}
	return out
}

func ordinaryJSONExpectedMatches(captures []Capture) uint64 {
	if len(captures) == 0 {
		return 0
	}
	return uint64(captures[len(captures)-1].Match) + 1
}

// TestOrdinaryJSONExpectedBuilderAgainstFixedAnchors checks the grammar-derived builder against
// independent fixed artifacts that predate it: the tracked JSON anchor tree for `null`, literal
// hand-written trees of small documents, and the tracked json-query-r1 capture list.
func TestOrdinaryJSONExpectedBuilderAgainstFixedAnchors(t *testing.T) {
	var anchors struct {
		Nodes []TreeNode `json:"nodes_each_version"`
	}
	data, err := os.ReadFile("../testdata/query-semantics/json1-classification-anchors.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &anchors); err != nil {
		t.Fatal(err)
	}
	tree, err := ordinaryJSONExpectedTree([]byte("null"))
	if err != nil || !ordinaryJSONSame(tree, anchors.Nodes) {
		t.Fatalf("null tree differs from the fixed anchor: err=%v\n got %s\nwant %s", err, ordinaryJSON(t, tree), ordinaryJSON(t, anchors.Nodes))
	}

	// `{"a": []}` + LF, written node by node.
	key := "key"
	pt := func(column uint32) Point { return Point{Column: column} }
	want := []TreeNode{
		{Parent: -1, Type: "document", Named: true, StartByte: 0, EndByte: 10, StartPoint: pt(0), EndPoint: Point{Row: 1}},
		{Parent: 0, Type: "object", Named: true, StartByte: 0, EndByte: 9, StartPoint: pt(0), EndPoint: pt(9)},
		{Parent: 1, Type: "{", StartByte: 0, EndByte: 1, StartPoint: pt(0), EndPoint: pt(1)},
		{Parent: 1, Type: "pair", Named: true, StartByte: 1, EndByte: 8, StartPoint: pt(1), EndPoint: pt(8)},
		{Parent: 3, Type: "string", Field: &key, Named: true, StartByte: 1, EndByte: 4, StartPoint: pt(1), EndPoint: pt(4)},
		{Parent: 4, Type: "\"", StartByte: 1, EndByte: 2, StartPoint: pt(1), EndPoint: pt(2)},
		{Parent: 4, Type: "string_content", Named: true, StartByte: 2, EndByte: 3, StartPoint: pt(2), EndPoint: pt(3)},
		{Parent: 4, Type: "\"", StartByte: 3, EndByte: 4, StartPoint: pt(3), EndPoint: pt(4)},
		{Parent: 3, Type: ":", StartByte: 4, EndByte: 5, StartPoint: pt(4), EndPoint: pt(5)},
		{Parent: 3, Type: "array", Named: true, StartByte: 6, EndByte: 8, StartPoint: pt(6), EndPoint: pt(8)},
		{Parent: 9, Type: "[", StartByte: 6, EndByte: 7, StartPoint: pt(6), EndPoint: pt(7)},
		{Parent: 9, Type: "]", StartByte: 7, EndByte: 8, StartPoint: pt(7), EndPoint: pt(8)},
		{Parent: 1, Type: "}", StartByte: 8, EndByte: 9, StartPoint: pt(8), EndPoint: pt(9)},
	}
	value := "value"
	want[9].Field = &value
	tree, err = ordinaryJSONExpectedTree([]byte("{\"a\": []}\n"))
	if err != nil || !ordinaryJSONSame(tree, want) {
		t.Fatalf("hand-written object tree differs: err=%v\n got %s\nwant %s", err, ordinaryJSON(t, tree), ordinaryJSON(t, want))
	}
	captures := ordinaryJSONExpectedCaptures(tree)
	if len(captures) != 1 || captures[0].Name != "array_key" || captures[0].Node != 4 || captures[0].Match != 0 || captures[0].Pattern != 0 || captures[0].Capture != 0 || captures[0].StartByte != 1 || captures[0].EndByte != 4 {
		t.Fatalf("hand-written capture expectation differs: %+v", captures)
	}

	// The fixed json-query-r1 capture list (earlier campaign) must equal the derived stream for
	// its source, including the shared match numbers of key/value pairs.
	var cases struct {
		Cases []struct {
			ID     string `json:"id"`
			Source string `json:"source_utf8"`
		} `json:"cases"`
	}
	routeData, err := os.ReadFile("../testdata/native/routes/json.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(routeData, &cases); err != nil {
		t.Fatal(err)
	}
	var source string
	for _, c := range cases.Cases {
		if c.ID == "json-feature-r1" {
			source = c.Source
		}
	}
	var queries struct {
		Cases []struct {
			ID          string `json:"id"`
			QueryExpect []struct {
				Step     int `json:"step"`
				Captures []struct {
					Name string `json:"name"`
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"captures"`
			} `json:"query_expect"`
		} `json:"cases"`
	}
	queryData, err := os.ReadFile("../testdata/native/queries/json.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(queryData, &queries); err != nil || len(queries.Cases) != 1 || len(queries.Cases[0].QueryExpect) != 1 {
		t.Fatalf("fixed json-query-r1 list is malformed: %v", err)
	}
	tree, err = ordinaryJSONExpectedTree([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	derived := ordinaryJSONExpectedCaptures(tree)
	fixed := queries.Cases[0].QueryExpect[0].Captures
	if len(derived) != len(fixed) || len(fixed) != 13 {
		t.Fatalf("derived capture count %d differs from the fixed list %d", len(derived), len(fixed))
	}
	for i, c := range derived {
		if c.Name != fixed[i].Name || c.Type != fixed[i].Type || source[c.StartByte:c.EndByte] != fixed[i].Text {
			t.Fatalf("capture %d: derived %s %s %q, fixed %s %s %q", i, c.Name, c.Type, source[c.StartByte:c.EndByte], fixed[i].Name, fixed[i].Type, fixed[i].Text)
		}
	}
}

func ordinaryJSONSame(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
