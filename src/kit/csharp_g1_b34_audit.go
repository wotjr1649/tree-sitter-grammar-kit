package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	jsonv2 "encoding/json/v2"
)

// The position audit and the type audit of the G1-3b expectations of csharp-B03 and csharp-B04 (#233), made before the seal.
//
// Both audits derive, without the expected values, what the expectation says, from a source that is not the one the expectation was made
// from, and compare:
//
//   - The position audit (csharp-g1-audit-b03b04-r1.json) lists, from the specification productions, the instances of each B
//     alternative in its witness source, and compares the witness (a node span from the Roslyn observation) with them. For each A
//     alternative it checks the anchors only (decision D-1): a small lexer of the C# lexical grammar, which has nothing from Roslyn,
//     must find the same lexeme at each anchor span of the witness.
//   - The type audit (csharp-g1-typeaudit-b03b04-r1.json) derives, from grammar.js alone, the node type at the witness of each B
//     alternative and the type of each anchor lexeme of the A alternatives, and compares them with the expected types.
//
// Each audit takes a narrow input made from the expected asset by csharp_g1_b34_view.go: the position audit the witnesses and the
// source text, the type audit the types and no position. A finding is an alternative the first comparison did not match; the
// derivation was fixed (csharp_g1_b34_rules.go, kind map r3) before the seal, and the finding stays recorded in `findings`.

const (
	CSharpG1AuditB34Schema = "tsgk-csharp-g1-audit-b03b04/r1"
	CSharpG1AuditB34Path   = "src/contracts/csharp-g1-audit-b03b04-r1.json"

	CSharpG1TypeAuditB34Schema = "tsgk-csharp-g1-typeaudit-b03b04/r1"
	CSharpG1TypeAuditB34Path   = "src/contracts/csharp-g1-typeaudit-b03b04-r1.json"

	csharpG1AuditB34Max = 1 << 20

	g1b34GroupB = "B" // the group of a structure alternative (G1ClassStructure of the classification, which the audit does not reach)
)

// G1B34Finding is a disagreement the first comparison of an audit showed and the fix made before the seal.
type G1B34Finding struct {
	ID    string `json:"id"`
	Audit string `json:"audit"` // position or type
	What  string `json:"what"`
	Fix   string `json:"fix"`
}

// ---- the position audit --------------------------------------------------------------------------------------

type G1B34PosRow struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`
	Form      string         `json:"form"`
	Instances [][2]int       `json:"instances"`
	Node      *G1Audit76Node `json:"node,omitempty"`
	Grounds   []G1Ground     `json:"ground"`
	Result    string         `json:"result"`
	Relation  string         `json:"relation"`
}

// G1B34AnchorRow is the anchor check of an A alternative: whether the lexer finds each anchor lexeme at the witness span.
type G1B34AnchorRow struct {
	ID     string   `json:"id"`
	Source string   `json:"source"`
	Texts  []string `json:"texts"`
	Tokens [][2]int `json:"tokens"`
	Result string   `json:"result"`
}

type G1B34AuditCounts struct {
	Rows           int `json:"rows"`
	Match          int `json:"match"`
	Mismatch       int `json:"mismatch"`
	Node           int `json:"node"`
	Anchors        int `json:"anchors"`
	AnchorMatch    int `json:"anchors_match"`
	AnchorMismatch int `json:"anchors_mismatch"`
	Findings       int `json:"findings"`
}

type CSharpG1AuditB34 struct {
	Schema   string           `json:"schema"`
	Basis    string           `json:"basis"`
	Counts   G1B34AuditCounts `json:"counts"`
	Findings []G1B34Finding   `json:"findings"`
	Rows     []G1B34PosRow    `json:"rows"`
	Anchors  []G1B34AnchorRow `json:"anchors"`
}

const csharpG1AuditB34Basis = "Hand derivation from the specification text (csharpstandard, the pinned commit) of the instances of each B alternative of csharp-B03 and csharp-B04 in its witness source, written without reading the expected types: the extent of every instance of the production, with a verbatim quote of the specification production, compared with the witness (a Roslyn node span) of the expected asset; a row that names the terminals the grammar text adds to the production's extent (node, with grammar grounds) compares the extent with those terminals. For each A alternative only the anchors are audited (decision D-1): a lexer of the C# lexical grammar that reads no Roslyn output must find the anchor lexeme at each anchor span of the witness. The findings are the alternatives the first comparison did not match; the derivation was fixed before the seal and each fix is stated. The comparison covers the position of the witness only, not its types; an instance list is what one reader derived from the text, not a parse, and the anchor check does not show that the anchors belong to the construct."

// G1B34AuditAlt is one alternative as the position audit sees it: the witness of a B alternative (a span), or the anchors of an A
// alternative. There is no member for the expected types, the Roslyn kind or the kind map.
type G1B34AuditAlt struct {
	ID      string
	Group   string
	Witness G1Audit76Witness
	Texts   []string // A: the anchor lexemes
	Tokens  [][2]int // A: the anchor spans
}

type G1B34AuditInput struct {
	Alts  []G1B34AuditAlt // the EXPECTED alternatives, sorted by id
	Texts map[string][]byte
}

func g1b34aFault(code string) error { return fail(KindInvalidInput, code, CSharpG1AuditB34Path, nil) }

// g1b34Lexemes lexes C# source text by the lexical grammar (comments, strings, characters, numbers, identifiers, punctuators) and
// returns the span of each lexeme with its text. A string literal is one opaque lexeme that is not returned; `>` of a shift
// operator is returned both as the shift and as the single characters, as a generic type argument list closes with single ones.
func g1b34Lexemes(src []byte) map[[2]int]string {
	out := map[[2]int]string{}
	puncts := []string{"<<=", ">>=", ">>>", "??=", "...", "->", "++", "--", "&&", "||", "==", "!=", "<=", ">=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<", ">>", "=>", "??", "::", ".."}
	n := len(src)
	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '/' && i+1 < n && src[i+1] == '/':
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			j := bytes.Index(src[i+2:], []byte("*/"))
			if j < 0 {
				return out
			}
			i += j + 4
		case c == '#' && g1b34LineStart(src, i):
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '"' || c == '@' && i+1 < n && src[i+1] == '"' || c == '$' && i+1 < n && (src[i+1] == '"' || src[i+1] == '@') || c == '@' && i+2 < n && src[i+1] == '$' && src[i+2] == '"':
			verbatim := false
			for i < n && src[i] != '"' {
				verbatim = verbatim || src[i] == '@'
				i++
			}
			i++
			for i < n {
				if src[i] == '"' && verbatim && i+1 < n && src[i+1] == '"' {
					i += 2
				} else if src[i] == '"' {
					i++
					break
				} else if src[i] == '\\' && !verbatim {
					i += 2
				} else {
					i++
				}
			}
		case c == '\'':
			i++
			for i < n && src[i] != '\'' {
				if src[i] == '\\' {
					i++
				}
				i++
			}
			i++
		case c >= '0' && c <= '9':
			j := i
			for j < n && (src[j] >= '0' && src[j] <= '9' || src[j] >= 'a' && src[j] <= 'z' || src[j] >= 'A' && src[j] <= 'Z' || src[j] == '_' || src[j] == '.' && j+1 < n && src[j+1] >= '0' && src[j+1] <= '9') {
				j++
			}
			out[[2]int{i, j}] = string(src[i:j])
			i = j
		case c == '_' || c == '@' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i + 1
			for j < n && (src[j] == '_' || src[j] >= 'a' && src[j] <= 'z' || src[j] >= 'A' && src[j] <= 'Z' || src[j] >= '0' && src[j] <= '9') {
				j++
			}
			out[[2]int{i, j}] = string(src[i:j])
			i = j
		default:
			w := 1
			for _, p := range puncts {
				if bytes.HasPrefix(src[i:], []byte(p)) {
					w = len(p)
					break
				}
			}
			out[[2]int{i, i + w}] = string(src[i : i+w])
			if w > 1 && src[i] == '>' {
				for k := 0; k < w; k++ {
					if src[i+k] == '>' {
						out[[2]int{i + k, i + k + 1}] = ">"
					}
				}
			}
			i += w
		}
	}
	return out
}

func g1b34LineStart(src []byte, i int) bool {
	for j := i - 1; j >= 0 && src[j] != '\n'; j-- {
		if src[j] != ' ' && src[j] != '\t' {
			return false
		}
	}
	return true
}

// g1b34AnchorResult is the anchor check: each anchor span is a lexeme of the text, with the anchor's text, and the spans are in order.
func g1b34AnchorResult(texts []string, tokens [][2]int, lex map[[2]int]string) string {
	if len(texts) == 0 || len(texts) != len(tokens) {
		return G1AuditMismatch
	}
	for i, t := range tokens {
		if lex[t] != texts[i] || i > 0 && t[0] < tokens[i-1][1] {
			return G1AuditMismatch
		}
	}
	return G1AuditMatch
}

// g1b34PosCompare compares the witness span of a row with its instances.
func g1b34PosCompare(r G1B34PosRow, w G1Audit76Witness, text []byte) (string, string) {
	return g1a76Compare(G1Audit76Row{Instances: r.Instances, Node: r.Node}, w, text)
}

func g1b34AuditCounts(x CSharpG1AuditB34) G1B34AuditCounts {
	c := G1B34AuditCounts{Rows: len(x.Rows), Anchors: len(x.Anchors), Findings: len(x.Findings)}
	for _, r := range x.Rows {
		if r.Result == G1AuditMatch {
			c.Match++
		} else {
			c.Mismatch++
		}
		if r.Node != nil {
			c.Node++
		}
	}
	for _, a := range x.Anchors {
		if a.Result == G1AuditMatch {
			c.AnchorMatch++
		} else {
			c.AnchorMismatch++
		}
	}
	return c
}

// DeriveCSharpG1AuditB34 completes the authored rows (instances, node, grounds) with their comparison against the expected asset's
// witnesses, and derives the anchor rows.
func DeriveCSharpG1AuditB34(authored []G1B34PosRow, in G1B34AuditInput) (CSharpG1AuditB34, error) {
	out := CSharpG1AuditB34{Schema: CSharpG1AuditB34Schema, Basis: csharpG1AuditB34Basis, Findings: slices.Clone(g1b34Findings("position"))}
	rows := slices.Clone(authored)
	slices.SortFunc(rows, func(a, b G1B34PosRow) int { return strings.Compare(a.ID, b.ID) })
	var b []G1B34AuditAlt
	lexes := map[string]map[[2]int]string{}
	for _, a := range in.Alts {
		if a.Group == g1b34GroupB {
			b = append(b, a)
			continue
		}
		lex := lexes[a.Witness.Source]
		if lex == nil {
			lex = g1b34Lexemes(in.Texts[a.Witness.Source])
			lexes[a.Witness.Source] = lex
		}
		out.Anchors = append(out.Anchors, G1B34AnchorRow{ID: a.ID, Source: a.Witness.Source, Texts: a.Texts, Tokens: a.Tokens, Result: g1b34AnchorResult(a.Texts, a.Tokens, lex)})
	}
	if len(rows) != len(b) {
		return out, g1b34aFault("G1_AUDITB34_ROWS")
	}
	for i := range rows {
		if rows[i].ID != b[i].ID || rows[i].Source != b[i].Witness.Source {
			return out, g1b34aFault("G1_AUDITB34_ROWS")
		}
		rows[i].Result, rows[i].Relation = g1b34PosCompare(rows[i], b[i].Witness, in.Texts[rows[i].Source])
	}
	out.Rows = rows
	out.Counts = g1b34AuditCounts(out)
	return out, nil
}

func DecodeCSharpG1AuditB34(data []byte) (CSharpG1AuditB34, error) {
	var x CSharpG1AuditB34
	if len(data) > csharpG1AuditB34Max {
		return x, fail(KindResourceLimit, "G1_AUDITB34_SIZE", CSharpG1AuditB34Path, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_AUDITB34_JSON", CSharpG1AuditB34Path, err)
	}
	if x.Schema != CSharpG1AuditB34Schema || x.Basis == "" {
		return x, g1b34aFault("G1_AUDITB34_SCHEMA")
	}
	return x, nil
}

// g1b34Write writes the canonical form of an audit asset: the header members on one line each, then one line per row.
func g1b34Write(members []struct {
	name string
	v    any
}, lists []struct {
	name string
	n    int
	at   func(int) any
}) ([]byte, error) {
	var b bytes.Buffer
	var werr error
	put := func(v any) {
		d, err := json.Marshal(v)
		if err != nil {
			werr = err
		}
		b.Write(d)
	}
	b.WriteString("{\n")
	for _, m := range members {
		fmt.Fprintf(&b, " %q: ", m.name)
		put(m.v)
		b.WriteString(",\n")
	}
	for k, l := range lists {
		fmt.Fprintf(&b, " %q: [\n", l.name)
		for i := 0; i < l.n; i++ {
			b.WriteString("  ")
			put(l.at(i))
			if i < l.n-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(" ]")
		if k < len(lists)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return b.Bytes(), werr
}

type g1b34Member = struct {
	name string
	v    any
}
type g1b34List = struct {
	name string
	n    int
	at   func(int) any
}

func MarshalCSharpG1AuditB34(x CSharpG1AuditB34) ([]byte, error) {
	return g1b34Write([]g1b34Member{{"schema", x.Schema}, {"basis", x.Basis}, {"counts", x.Counts}},
		[]g1b34List{
			{"findings", len(x.Findings), func(i int) any { return x.Findings[i] }},
			{"rows", len(x.Rows), func(i int) any { return x.Rows[i] }},
			{"anchors", len(x.Anchors), func(i int) any { return x.Anchors[i] }},
		})
}

func g1b34CheckGrounds(gs []G1Ground, doc string) bool { return g1a76CheckGrounds(gs, doc) }

// ParseCSharpG1AuditB34 decodes the asset and verifies it against the narrow input: the rows are exactly the B alternatives and the
// anchor rows the A alternatives of the expected asset, every row is well formed with checkable grounds pinned to the row, every
// instance is a text of the source that the rule of its row accepts, and the comparisons are the comparisons made now.
func ParseCSharpG1AuditB34(data []byte, in G1B34AuditInput) (CSharpG1AuditB34, error) {
	x, err := DecodeCSharpG1AuditB34(data)
	if err != nil {
		return x, err
	}
	var b []G1B34AuditAlt
	for _, a := range in.Alts {
		if a.Group == g1b34GroupB {
			b = append(b, a)
		}
	}
	if len(x.Rows) != len(b) || len(g1b34PosRules) != len(g1b34PosRuleTexts) {
		return x, g1b34aFault("G1_AUDITB34_ROWS")
	}
	for i, r := range x.Rows {
		alt := b[i]
		text, ok := in.Texts[r.Source]
		if r.ID != alt.ID || !ok || alt.Witness.Source != r.Source {
			return x, g1b34aFault("G1_AUDITB34_ROWS")
		}
		if r.Form == "" || len(r.Instances) == 0 || !g1b34CheckGrounds(r.Grounds, G1DocSpec) {
			return x, g1b34aFault("G1_AUDITB34_ROW")
		}
		if n := r.Node; n != nil && (n.Leading == "" && n.Trailing == "" || !g1b34CheckGrounds(n.Grounds, G1DocGrammar)) {
			return x, g1b34aFault("G1_AUDITB34_ROW")
		}
		if g1b34PosPins[r.ID] != G1GroundPin(r.Grounds) || r.Node != nil && g1b34PosPins[r.ID+"#node"] != G1GroundPin(r.Node.Grounds) || r.Node == nil && g1b34PosPins[r.ID+"#node"] != "" {
			return x, g1b34aFault("G1_AUDITB34_GROUND")
		}
		for j, in := range r.Instances {
			if in[0] < 0 || in[0] >= in[1] || in[1] > len(text) || j > 0 && !(r.Instances[j-1][0] < in[0] || r.Instances[j-1][0] == in[0] && r.Instances[j-1][1] < in[1]) {
				return x, g1b34aFault("G1_AUDITB34_INSTANCE")
			}
			if !g1b34PosRules[r.ID].accepts(string(text[in[0]:in[1]])) {
				return x, g1b34aFault("G1_AUDITB34_RULE")
			}
		}
		res, rel := g1b34PosCompare(r, alt.Witness, text)
		if res != r.Result || rel != r.Relation {
			return x, g1b34aFault("G1_AUDITB34_COMPARISON")
		}
	}
	want, err := DeriveCSharpG1AuditB34(x.Rows, in)
	if err != nil {
		return x, err
	}
	if !g1b34Same(x.Anchors, want.Anchors) {
		return x, g1b34aFault("G1_AUDITB34_ANCHORS")
	}
	if !g1b34Same(x.Findings, want.Findings) {
		return x, g1b34aFault("G1_AUDITB34_FINDINGS")
	}
	if x.Counts != g1b34AuditCounts(x) || x.Basis != csharpG1AuditB34Basis {
		return x, g1b34aFault("G1_AUDITB34_COUNTS")
	}
	canonical, err := MarshalCSharpG1AuditB34(x)
	if err != nil || !bytes.Equal(canonical, data) {
		return x, g1b34aFault("G1_AUDITB34_BYTES")
	}
	return x, nil
}

// CheckG1AuditB34Grounds checks every ground of the rows against the pinned documents (the live counterpart of the offline check).
func CheckG1AuditB34Grounds(rows []G1B34PosRow, grammar string, spec map[string]string) error {
	for _, r := range rows {
		gs := slices.Clone(r.Grounds)
		if r.Node != nil {
			gs = append(gs, r.Node.Grounds...)
		}
		if err := CheckG1GroundList(r.ID, gs, grammar, spec); err != nil {
			return fmt.Errorf("audit row %w", err)
		}
	}
	return nil
}

// G1B34PosGroundPins are the pins of the rows of the position audit: one per row and one per node of a row ("<id>#node").
func G1B34PosGroundPins(rows []G1B34PosRow) map[string]string {
	pins := map[string]string{}
	for _, r := range rows {
		pins[r.ID] = G1GroundPin(r.Grounds)
		if r.Node != nil {
			pins[r.ID+"#node"] = G1GroundPin(r.Node.Grounds)
		}
	}
	return pins
}

// ---- the type audit -------------------------------------------------------------------------------------------

type G1B34TypeRow struct {
	ID         string     `json:"id"`
	Class      string     `json:"class"`
	Types      []string   `json:"types"`
	Coincident []string   `json:"coincident,omitempty"`
	Reason     string     `json:"reason"`
	Grounds    []G1Ground `json:"ground"`
	Result     string     `json:"result"`
	Relation   string     `json:"relation"`
}

// G1B34AnchorTypeRow is the type of one anchor lexeme of the A alternatives, derived from grammar.js.
type G1B34AnchorTypeRow struct {
	Text     string     `json:"text"`
	Class    string     `json:"class"`
	Types    []string   `json:"types"`
	Reason   string     `json:"reason"`
	Grounds  []G1Ground `json:"ground"`
	Result   string     `json:"result"`
	Relation string     `json:"relation"`
}

type G1B34TypeCounts struct {
	Rows           int `json:"rows"`
	Match          int `json:"match"`
	Mismatch       int `json:"mismatch"`
	Anchors        int `json:"anchors"`
	AnchorMatch    int `json:"anchors_match"`
	AnchorMismatch int `json:"anchors_mismatch"`
	Findings       int `json:"findings"`
}

type CSharpG1TypeAuditB34 struct {
	Schema   string               `json:"schema"`
	Basis    string               `json:"basis"`
	Counts   G1B34TypeCounts      `json:"counts"`
	Findings []G1B34Finding       `json:"findings"`
	Rows     []G1B34TypeRow       `json:"rows"`
	Anchors  []G1B34AnchorTypeRow `json:"anchors"`
}

const csharpG1TypeAuditB34Basis = "Hand derivation from the pinned grammar.js alone (the grammar.js the kind map pins) of the node type at the witness of each B alternative of csharp-B03 and csharp-B04, and of the type of each anchor lexeme of the A alternatives, written without reading the kind map, the expected types, the Roslyn kinds or any native output. A row states the class of the derivation, the derived types, the node types of the same extent that the expected types may also list, and grammar quotes. The verifier recomputes the comparison with the expected types. The findings are the alternatives the first comparison did not match; the derivation was fixed before the seal and each fix is stated. The audit covers the types only; the position of the witness is the position audit, and a derivation is what one reader derived from the grammar text, not a parser run."

// G1B34TypeAlt is one alternative as the type audit sees it: the expected types of a B alternative. There is no member for the
// witness, the kind map or the Roslyn kind.
type G1B34TypeAlt struct {
	ID string
	TS []string
}

// G1B34AnchorTS is the types the expected asset gives an anchor lexeme (the union over the alternatives that have it).
type G1B34AnchorTS struct {
	Text string
	TS   []string
}

type G1B34TypeInput struct {
	Alts    []G1B34TypeAlt // the EXPECTED B alternatives, sorted by id
	Anchors []G1B34AnchorTS
}

func g1b34tFault(code string) error {
	return fail(KindInvalidInput, code, CSharpG1TypeAuditB34Path, nil)
}

func g1b34TypeCompare(class string, types, coincident, ts []string) (string, string) {
	return g1t76Compare(G1TypeAudit76Row{Class: class, Types: types, Coincident: coincident}, ts)
}

func g1b34TypeCounts(x CSharpG1TypeAuditB34) G1B34TypeCounts {
	c := G1B34TypeCounts{Rows: len(x.Rows), Anchors: len(x.Anchors), Findings: len(x.Findings)}
	for _, r := range x.Rows {
		if r.Result == G1AuditMatch {
			c.Match++
		} else {
			c.Mismatch++
		}
	}
	for _, a := range x.Anchors {
		if a.Result == G1AuditMatch {
			c.AnchorMatch++
		} else {
			c.AnchorMismatch++
		}
	}
	return c
}

// DeriveCSharpG1TypeAuditB34 completes the authored rows with their comparison against the expected types.
func DeriveCSharpG1TypeAuditB34(rows []G1B34TypeRow, anchors []G1B34AnchorTypeRow, in G1B34TypeInput) (CSharpG1TypeAuditB34, error) {
	out := CSharpG1TypeAuditB34{Schema: CSharpG1TypeAuditB34Schema, Basis: csharpG1TypeAuditB34Basis, Findings: slices.Clone(g1b34Findings("type"))}
	rows, anchors = slices.Clone(rows), slices.Clone(anchors)
	slices.SortFunc(rows, func(a, b G1B34TypeRow) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(anchors, func(a, b G1B34AnchorTypeRow) int { return strings.Compare(a.Text, b.Text) })
	if len(rows) != len(in.Alts) || len(anchors) != len(in.Anchors) {
		return out, g1b34tFault("G1_TYPEAUDITB34_ROWS")
	}
	for i := range rows {
		if rows[i].ID != in.Alts[i].ID {
			return out, g1b34tFault("G1_TYPEAUDITB34_ROWS")
		}
		rows[i].Result, rows[i].Relation = g1b34TypeCompare(rows[i].Class, rows[i].Types, rows[i].Coincident, in.Alts[i].TS)
	}
	for i := range anchors {
		if anchors[i].Text != in.Anchors[i].Text {
			return out, g1b34tFault("G1_TYPEAUDITB34_ROWS")
		}
		anchors[i].Result, anchors[i].Relation = g1b34TypeCompare(anchors[i].Class, anchors[i].Types, nil, in.Anchors[i].TS)
	}
	out.Rows, out.Anchors = rows, anchors
	out.Counts = g1b34TypeCounts(out)
	return out, nil
}

func DecodeCSharpG1TypeAuditB34(data []byte) (CSharpG1TypeAuditB34, error) {
	var x CSharpG1TypeAuditB34
	if len(data) > csharpG1AuditB34Max {
		return x, fail(KindResourceLimit, "G1_TYPEAUDITB34_SIZE", CSharpG1TypeAuditB34Path, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_TYPEAUDITB34_JSON", CSharpG1TypeAuditB34Path, err)
	}
	if x.Schema != CSharpG1TypeAuditB34Schema || x.Basis == "" {
		return x, g1b34tFault("G1_TYPEAUDITB34_SCHEMA")
	}
	return x, nil
}

func MarshalCSharpG1TypeAuditB34(x CSharpG1TypeAuditB34) ([]byte, error) {
	return g1b34Write([]g1b34Member{{"schema", x.Schema}, {"basis", x.Basis}, {"counts", x.Counts}},
		[]g1b34List{
			{"findings", len(x.Findings), func(i int) any { return x.Findings[i] }},
			{"rows", len(x.Rows), func(i int) any { return x.Rows[i] }},
			{"anchors", len(x.Anchors), func(i int) any { return x.Anchors[i] }},
		})
}

var g1b34TypeName = regexp.MustCompile(`^(?:[a-z_][a-z0-9_]*|".+")$`)

func g1b34TypeForm(class string, types []string) bool {
	if !slices.IsSorted(types) || len(types) == 0 || slices.Compact(slices.Clone(types)) == nil || len(slices.Compact(slices.Clone(types))) != len(types) {
		return false
	}
	for _, t := range types {
		if !g1b34TypeName.MatchString(t) {
			return false
		}
	}
	switch class {
	case G1TypeNode, G1TypeNoOwnNode, G1TypeAlias:
		return len(types) == 1
	case G1TypeNotDerivable:
		return len(types) > 1
	}
	return false
}

// ParseCSharpG1TypeAuditB34 decodes the asset and verifies it against the narrow input: its rows are exactly the B alternatives of
// the expected asset and its anchor rows the anchor lexemes, each row has well formed types and grammar grounds pinned to the row,
// and the comparisons are the comparisons made now.
func ParseCSharpG1TypeAuditB34(data []byte, in G1B34TypeInput) (CSharpG1TypeAuditB34, error) {
	x, err := DecodeCSharpG1TypeAuditB34(data)
	if err != nil {
		return x, err
	}
	if len(x.Rows) != len(in.Alts) || len(x.Anchors) != len(in.Anchors) {
		return x, g1b34tFault("G1_TYPEAUDITB34_ROWS")
	}
	check := func(id, class string, types, coincident []string, reason string, gs []G1Ground, pin string) error {
		if !g1b34TypeForm(class, types) || reason == "" || !g1b34CheckGrounds(gs, G1DocGrammar) || (len(coincident) > 0 && (!slices.IsSorted(coincident) || slices.ContainsFunc(coincident, func(c string) bool { return slices.Contains(types, c) || !g1b34TypeName.MatchString(c) }))) {
			return g1b34tFault("G1_TYPEAUDITB34_ROW")
		}
		if G1GroundPin(gs) != pin {
			return g1b34tFault("G1_TYPEAUDITB34_GROUND")
		}
		return nil
	}
	for i, r := range x.Rows {
		if r.ID != in.Alts[i].ID {
			return x, g1b34tFault("G1_TYPEAUDITB34_ROWS")
		}
		if err := check(r.ID, r.Class, r.Types, r.Coincident, r.Reason, r.Grounds, g1b34TypePins[r.ID]); err != nil {
			return x, err
		}
	}
	for i, r := range x.Anchors {
		if r.Text != in.Anchors[i].Text {
			return x, g1b34tFault("G1_TYPEAUDITB34_ROWS")
		}
		if err := check(r.Text, r.Class, r.Types, nil, r.Reason, r.Grounds, g1b34TypePins["anchor:"+r.Text]); err != nil {
			return x, err
		}
	}
	want, err := DeriveCSharpG1TypeAuditB34(x.Rows, x.Anchors, in)
	if err != nil {
		return x, err
	}
	if !g1b34Same(x.Rows, want.Rows) || !g1b34Same(x.Anchors, want.Anchors) {
		return x, g1b34tFault("G1_TYPEAUDITB34_COMPARISON")
	}
	if !g1b34Same(x.Findings, want.Findings) {
		return x, g1b34tFault("G1_TYPEAUDITB34_FINDINGS")
	}
	if x.Counts != g1b34TypeCounts(x) || x.Basis != csharpG1TypeAuditB34Basis {
		return x, g1b34tFault("G1_TYPEAUDITB34_COUNTS")
	}
	canonical, err := MarshalCSharpG1TypeAuditB34(x)
	if err != nil || !bytes.Equal(canonical, data) {
		return x, g1b34tFault("G1_TYPEAUDITB34_BYTES")
	}
	return x, nil
}

// G1B34TypeGroundPins are the pins of the rows of the type audit ("anchor:<lexeme>" for an anchor row).
func G1B34TypeGroundPins(x CSharpG1TypeAuditB34) map[string]string {
	pins := map[string]string{}
	for _, r := range x.Rows {
		pins[r.ID] = G1GroundPin(r.Grounds)
	}
	for _, r := range x.Anchors {
		pins["anchor:"+r.Text] = G1GroundPin(r.Grounds)
	}
	return pins
}

// g1b34Same reports whether two values have the same JSON (the audit compares its rows with the rows it derives).
func g1b34Same(a, b any) bool {
	x, e1 := json.Marshal(a)
	y, e2 := json.Marshal(b)
	return e1 == nil && e2 == nil && bytes.Equal(x, y)
}

// CheckG1TypeAuditB34Grounds checks every ground of the type audit against the pinned grammar.js (the live counterpart of the offline check).
func CheckG1TypeAuditB34Grounds(x CSharpG1TypeAuditB34, grammar string) error {
	for _, r := range x.Rows {
		if err := CheckG1GroundList(r.ID, r.Grounds, grammar, nil); err != nil {
			return fmt.Errorf("type audit row %w", err)
		}
	}
	for _, r := range x.Anchors {
		if err := CheckG1GroundList("anchor "+r.Text, r.Grounds, grammar, nil); err != nil {
			return fmt.Errorf("type audit anchor %w", err)
		}
	}
	return nil
}
