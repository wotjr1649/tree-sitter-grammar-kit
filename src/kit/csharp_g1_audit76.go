package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	jsonv2 "encoding/json/v2"
)

// The audit of the 76 differential-oracle alternatives of the G1 expected asset (#233, G1AUDIT76).
//
// The 76 alternatives (the lexemes of the interpolated strings and the type, pointer and parameter alternatives of
// csharp-B01 and csharp-B02) take their witness from the Roslyn observation, so the specification stood behind them
// only through the kind map. The audit derives, for each, by hand from the specification text alone: whether the
// witness source is accepted, the lexeme form, and the extent of every instance of that form in the source. The
// verifier then compares those instances with the witness of the r2 expected asset: a witness of match span must have
// the extent of an instance, a witness of match starts-at the start of one.
//
// The audit does not read the expected types (ts): the comparison takes the witness and the source text only, the
// pinned old value of a correction is read for its witness only, and the separation test checks that no function of this
// file names the field. An instance of an interpolation or alignment clause is the extent of the specification symbol
// (the expression between the braces); the grammar text puts the braces and the comma inside the node, so a row may
// state the terminals that the node adds (Node, with grammar grounds). A terminal that is an external scanner token has
// no extent in the grammar text: for such a row only the start of the node is derivable, and a span claim does not match.
//
// A row states its comparison (Result, Relation) and the comparison of the witness as it was before the after-native
// corrections of the r2 asset (Initial); the verifier recomputes both, so a row cannot disagree with the asset it audits.
// The expected asset is not pinned by this one: the comparison is made against whatever r2 asset is given, and the test
// that runs it fails when the asset and the audit part.

const (
	CSharpG1Audit76Schema = "tsgk-csharp-g1-audit76/r1"
	CSharpG1Audit76Path   = "src/contracts/csharp-g1-audit76-r1.json"

	G1AuditMatch    = "MATCH"
	G1AuditMismatch = "MISMATCH"

	G1AuditRelEqual        = "EQUAL_SPAN"
	G1AuditRelStartsAt     = "STARTS_AT_EQUAL"
	G1AuditRelNodeSpan     = "NODE_EXTENT_SPAN"
	G1AuditRelNodeStartsAt = "NODE_EXTENT_STARTS_AT"
	G1AuditRelNoInstance   = "NO_INSTANCE"
	G1AuditRelNoExtent     = "EXTENT_NOT_DERIVABLE"

	csharpG1Audit76Max = 1 << 20
)

const csharpG1Audit76Basis = "Hand derivation from the specification text (csharpstandard, the pinned commit) of the 76 differential-oracle alternatives of the r2 expected asset, written without reading the expected types: for each alternative the acceptance of its witness source, the lexeme form and the extent of every instance of that form in the source, with a verbatim quote of the specification production. Each row states its comparison with the witness of the r2 asset (a span witness must have the extent of an instance, a starts-at witness the start of one; a row that names the terminals its node adds, with grammar grounds, compares the extent with those terminals) and the comparison of the witness before the after-native corrections (initial); the verifier recomputes both. The comparison covers the position of the witness only: it does not compare the types (ts) of these alternatives, and an instance list is what one reader derived from the text, not a parse."

// G1Audit76Node names the terminals a node adds around the specification symbol.
type G1Audit76Node struct {
	Leading  string `json:"leading"`
	Trailing string `json:"trailing"`
	// External: a terminal is an external scanner token of the grammar text, whose extent the text does not give
	External bool       `json:"external"`
	Grounds  []G1Ground `json:"ground"`
}

// G1Audit76Row is the audit of one alternative.
type G1Audit76Row struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`
	Accepted  bool           `json:"accepted"`
	Form      string         `json:"form"`
	Instances [][2]int       `json:"instances"`
	Node      *G1Audit76Node `json:"node,omitempty"`
	Grounds   []G1Ground     `json:"ground"`
	Result    string         `json:"result"`
	Relation  string         `json:"relation"`
	Initial   string         `json:"initial"`
}

type G1Audit76Counts struct {
	Rows      int `json:"rows"`
	Match     int `json:"match"`
	Mismatch  int `json:"mismatch"`
	Corrected int `json:"corrected"` // initial MISMATCH, now MATCH
	Node      int `json:"node"`
}

type CSharpG1Audit76 struct {
	Schema string          `json:"schema"`
	Basis  string          `json:"basis"`
	Counts G1Audit76Counts `json:"counts"`
	Rows   []G1Audit76Row  `json:"rows"`
}

// G1Audit76Sources are what the audit is compared with: the r2 expected asset and the text of the sources it names.
type G1Audit76Sources struct {
	Expected []byte
	Texts    map[string][]byte
}

func g1a76Fault(code string) error { return fail(KindInvalidInput, code, CSharpG1Audit76Path, nil) }

// g1a76Compare compares one witness with the instances of a row. It sees the witness and the source text only.
func g1a76Compare(r G1Audit76Row, w G1ExpWitness, text []byte) (result, relation string) {
	relation = G1AuditRelNoInstance
	for _, in := range r.Instances {
		lo, hi := in[0], in[1]
		rel, relStart := G1AuditRelEqual, G1AuditRelStartsAt
		if n := r.Node; n != nil {
			lo, hi = in[0]-len(n.Leading), in[1]+len(n.Trailing)
			if lo < 0 || hi > len(text) || string(text[lo:in[0]]) != n.Leading || string(text[in[1]:hi]) != n.Trailing {
				continue
			}
			rel, relStart = G1AuditRelNodeSpan, G1AuditRelNodeStartsAt
		}
		switch {
		case w.Match == "span" && w.Start == lo && w.End == hi:
			if r.Node != nil && r.Node.External {
				relation = G1AuditRelNoExtent
				continue
			}
			return G1AuditMatch, rel
		case w.Match == "starts-at" && w.Start == lo:
			return G1AuditMatch, relStart
		}
	}
	return G1AuditMismatch, relation
}

// g1a76Initial is the witness of the alternative before the corrections of the r2 asset that moved or relaxed it.
func g1a76Initial(item string, w G1ExpWitness, cs []G1Correction) (G1ExpWitness, error) {
	for _, c := range cs {
		if c.Item != item {
			continue
		}
		switch c.Field {
		case G1FieldEvidence:
			var o g1cEvidence
			if err := json.Unmarshal([]byte(c.Old), &o); err != nil {
				return w, g1a76Fault("G1_AUDIT76_CORRECTION")
			}
			w = o.Witness
		case G1FieldWitnessMatch:
			var m string
			if err := json.Unmarshal([]byte(c.Old), &m); err != nil {
				return w, g1a76Fault("G1_AUDIT76_CORRECTION")
			}
			w.Match = m
		}
	}
	return w, nil
}

// g1a76Differential returns the r2 alternatives of the differential-oracle stratum, by id.
func g1a76Differential(expected []byte) (CSharpG1Expected, []G1ExpAlt, error) {
	x, err := DecodeCSharpG1Expected(expected)
	if err != nil {
		return x, nil, err
	}
	if x.Schema != CSharpG1ExpectedSchemaR2 {
		return x, nil, g1a76Fault("G1_AUDIT76_EXPECTED")
	}
	var alts []G1ExpAlt
	for _, a := range x.Alternatives {
		if a.Status == G1ExpExpected && a.Basis == G1BasisDiff {
			alts = append(alts, a)
		}
	}
	slices.SortFunc(alts, func(a, b G1ExpAlt) int { return strings.Compare(a.ID, b.ID) })
	return x, alts, nil
}

// g1a76Fill sets the comparison fields of a row from the asset (the inputs are the instances, the node and the grounds).
func g1a76Fill(r *G1Audit76Row, alt G1ExpAlt, cs []G1Correction, text []byte) error {
	if len(alt.Witnesses) != 1 || alt.Witnesses[0].Source != r.Source {
		return g1a76Fault("G1_AUDIT76_WITNESS")
	}
	r.Result, r.Relation = g1a76Compare(*r, alt.Witnesses[0], text)
	initial, err := g1a76Initial(alt.ID, alt.Witnesses[0], cs)
	if err != nil {
		return err
	}
	r.Initial, _ = g1a76Compare(*r, initial, text)
	return nil
}

func g1a76Counts(rows []G1Audit76Row) G1Audit76Counts {
	n := G1Audit76Counts{Rows: len(rows)}
	for _, r := range rows {
		if r.Result == G1AuditMatch {
			n.Match++
		} else {
			n.Mismatch++
		}
		if r.Initial == G1AuditMismatch && r.Result == G1AuditMatch {
			n.Corrected++
		}
		if r.Node != nil {
			n.Node++
		}
	}
	return n
}

// DeriveCSharpG1Audit76 completes the authored rows (instances, node, grounds) with their comparison against the r2 asset.
func DeriveCSharpG1Audit76(authored []G1Audit76Row, in G1Audit76Sources) (CSharpG1Audit76, error) {
	out := CSharpG1Audit76{Schema: CSharpG1Audit76Schema, Basis: csharpG1Audit76Basis}
	x, alts, err := g1a76Differential(in.Expected)
	if err != nil {
		return out, err
	}
	rows := slices.Clone(authored)
	slices.SortFunc(rows, func(a, b G1Audit76Row) int { return strings.Compare(a.ID, b.ID) })
	if len(rows) != len(alts) {
		return out, g1a76Fault("G1_AUDIT76_ROWS")
	}
	for i := range rows {
		if rows[i].ID != alts[i].ID {
			return out, g1a76Fault("G1_AUDIT76_ROWS")
		}
		if err := g1a76Fill(&rows[i], alts[i], x.Corrections, in.Texts[rows[i].Source]); err != nil {
			return out, err
		}
	}
	out.Rows, out.Counts = rows, g1a76Counts(rows)
	return out, nil
}

// DecodeCSharpG1Audit76 decodes the asset strictly (size bound, no unknown member, schema).
func DecodeCSharpG1Audit76(data []byte) (CSharpG1Audit76, error) {
	var x CSharpG1Audit76
	if len(data) > csharpG1Audit76Max {
		return x, fail(KindResourceLimit, "G1_AUDIT76_SIZE", CSharpG1Audit76Path, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_AUDIT76_JSON", CSharpG1Audit76Path, err)
	}
	if x.Schema != CSharpG1Audit76Schema || x.Basis == "" {
		return x, g1a76Fault("G1_AUDIT76_SCHEMA")
	}
	return x, nil
}

// MarshalCSharpG1Audit76 writes the canonical bytes: the header on a few lines, one row per line.
func MarshalCSharpG1Audit76(x CSharpG1Audit76) ([]byte, error) {
	var b bytes.Buffer
	put := func(v any) error {
		d, err := json.Marshal(v)
		b.Write(d)
		return err
	}
	b.WriteString("{\n \"schema\": ")
	err := put(x.Schema)
	b.WriteString(",\n \"basis\": ")
	err = cmpErr(err, put(x.Basis))
	b.WriteString(",\n \"counts\": ")
	err = cmpErr(err, put(x.Counts))
	b.WriteString(",\n \"rows\": [\n")
	for i, r := range x.Rows {
		b.WriteString("  ")
		err = cmpErr(err, put(r))
		if i < len(x.Rows)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(" ]\n}\n")
	return b.Bytes(), err
}

func cmpErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// g1a76CheckGrounds requires the grounds of a row: each checkable offline, at least one, of the document kind wanted.
func g1a76CheckGrounds(gs []G1Ground, doc string) bool {
	for _, g := range gs {
		if g.Doc != doc || !g1CheckGround(g) {
			return false
		}
	}
	return len(gs) > 0
}

// ParseCSharpG1Audit76 decodes the asset and verifies it: its rows are exactly the differential-oracle alternatives of the
// r2 asset, every row is well formed with checkable grounds, the comparison each row states is the comparison made now
// against the r2 witness (and against the witness before the corrections), and the bytes are canonical.
func ParseCSharpG1Audit76(data []byte, in G1Audit76Sources) (CSharpG1Audit76, error) {
	x, err := DecodeCSharpG1Audit76(data)
	if err != nil {
		return x, err
	}
	r2, alts, err := g1a76Differential(in.Expected)
	if err != nil {
		return x, err
	}
	if len(x.Rows) != len(alts) {
		return x, g1a76Fault("G1_AUDIT76_ROWS")
	}
	for i, r := range x.Rows {
		alt := alts[i]
		text, ok := in.Texts[r.Source]
		if r.ID != alt.ID || !ok {
			return x, g1a76Fault("G1_AUDIT76_ROWS")
		}
		if !r.Accepted || r.Form == "" || len(r.Instances) == 0 || !g1a76CheckGrounds(r.Grounds, G1DocSpec) {
			return x, g1a76Fault("G1_AUDIT76_ROW")
		}
		for j, in := range r.Instances {
			if in[0] < 0 || in[0] >= in[1] || in[1] > len(text) || j > 0 && !(r.Instances[j-1][0] < in[0] || r.Instances[j-1][0] == in[0] && r.Instances[j-1][1] < in[1]) {
				return x, g1a76Fault("G1_AUDIT76_INSTANCE")
			}
		}
		if n := r.Node; n != nil && (n.Leading == "" && n.Trailing == "" || !g1a76CheckGrounds(n.Grounds, G1DocGrammar)) {
			return x, g1a76Fault("G1_AUDIT76_ROW")
		}
		want := r
		if err := g1a76Fill(&want, alt, r2.Corrections, text); err != nil {
			return x, err
		}
		if want.Result != r.Result || want.Relation != r.Relation || want.Initial != r.Initial {
			return x, g1a76Fault("G1_AUDIT76_COMPARISON")
		}
	}
	if x.Counts != g1a76Counts(x.Rows) {
		return x, g1a76Fault("G1_AUDIT76_COUNTS")
	}
	if x.Basis != csharpG1Audit76Basis {
		return x, g1a76Fault("G1_AUDIT76_SCHEMA")
	}
	canonical, err := MarshalCSharpG1Audit76(x)
	if err != nil || !bytes.Equal(canonical, data) {
		return x, g1a76Fault("G1_AUDIT76_BYTES")
	}
	return x, nil
}

// CheckG1Audit76Grounds checks every ground of the rows against the pinned documents (the live counterpart of the offline check).
func CheckG1Audit76Grounds(rows []G1Audit76Row, grammar string, spec map[string]string) error {
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
