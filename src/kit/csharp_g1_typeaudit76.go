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

// The type audit of the 76 differential-oracle alternatives of the G1 expected asset (#233, G1TYPEAUDIT76).
//
// The position audit (csharp-g1-audit76-r1.json) compares the witness of each alternative with the specification and
// never the expected types, so that grammar knowledge stays out of it (the decision of 2026-11-13, R-4). This audit is
// the other half: for each of the 76 alternatives the node type that the pinned grammar.js gives the witness, derived by
// hand from the grammar text alone (rule names, aliases, fields, hidden rules, inline, supertypes, externals) and compared
// with the types (ts) of the r2 expected asset. The ts of these alternatives were translated from the kind map; the audit
// is the check that the grammar text agrees, and it must not read the kind map: its only input besides the rows is the
// r2 asset (a syntactic test, TestCSharpG1TypeAudit76NeverReadsTheKindMap, checks the file).
//
// A row states the class of the derivation: NODE (one visible node type), NO_OWN_NODE (the specification production has no
// node of its own; the type is the node of the same extent), ALIAS (the type is fixed by an alias), EXTERNAL_TOKEN (the
// token comes from the external scanner: its type name is in the grammar text, its extent is not, so the row audits the
// type only and the extent is in the position audit) and NOT_DERIVABLE (the grammar text leaves a conflict whose
// resolution is not in the text; the row keeps the candidate set). Coincident are further node types that have the same
// extent as the node, which the expected types may list as well. The verifier recomputes the comparison of the row with
// the r2 ts and the comparison with the ts before the ts corrections of r2 (Initial).

const (
	CSharpG1TypeAudit76Schema = "tsgk-csharp-g1-typeaudit76/r1"
	CSharpG1TypeAudit76Path   = "src/contracts/csharp-g1-typeaudit76-r1.json"

	G1TypeNode            = "NODE"
	G1TypeNoOwnNode       = "NO_OWN_NODE"
	G1TypeAlias           = "ALIAS"
	G1TypeExternal        = "EXTERNAL_TOKEN"
	G1TypeNotDerivable    = "NOT_DERIVABLE"
	G1TypeExtentElsewhere = "type only, extent in the position audit"

	G1TypeRelEqual      = "EQUAL"
	G1TypeRelCoincident = "WITH_COINCIDENT_NODES"
	G1TypeRelCandidates = "CANDIDATE_SET_EQUAL"
	G1TypeRelSuperset   = "EXPECTED_SUPERSET"
	G1TypeRelSubset     = "EXPECTED_SUBSET"
	G1TypeRelOverlap    = "OVERLAP"
	G1TypeRelDisjoint   = "DISJOINT"

	csharpG1TypeAudit76Max = 1 << 20
)

const csharpG1TypeAudit76Basis = "Hand derivation from the pinned grammar.js alone (the grammar.js the kind map pins) of the node type at the witness of each of the 76 differential-oracle alternatives of the r2 expected asset, written without reading the kind map, the expected types, the Roslyn kinds or any native output. A row states the class of the derivation, the derived types (the candidate set when the grammar text does not settle it), the node types of the same extent that the expected types may also list, and grammar quotes with their lines. A row of an external scanner token audits the type only: the grammar text does not give the extent of the token, which the position audit checks. The verifier recomputes the comparison with the ts of the r2 asset and the comparison with the ts before the ts corrections of r2 (initial). The audit covers the types only; the position of the witness is the position audit, and a derivation is what one reader derived from the grammar text, not a parser run."

var g1t76TypeName = regexp.MustCompile(`^(?:[a-z_][a-z0-9_]*|".+")$`)

// G1TypeAudit76Row is the type audit of one alternative.
type G1TypeAudit76Row struct {
	ID         string     `json:"id"`
	Class      string     `json:"class"`
	Types      []string   `json:"types"`
	Coincident []string   `json:"coincident,omitempty"`
	Extent     string     `json:"extent,omitempty"`
	Reason     string     `json:"reason"`
	Grounds    []G1Ground `json:"ground"`
	Result     string     `json:"result"`
	Relation   string     `json:"relation"`
	Initial    string     `json:"initial"`
}

type G1TypeAudit76Counts struct {
	Rows         int `json:"rows"`
	Match        int `json:"match"`
	Mismatch     int `json:"mismatch"`
	Node         int `json:"node"`
	NoOwnNode    int `json:"no_own_node"`
	Alias        int `json:"alias"`
	External     int `json:"external_token"`
	NotDerivable int `json:"not_derivable"`
	Corrected    int `json:"corrected"` // initial MISMATCH, now MATCH
}

type CSharpG1TypeAudit76 struct {
	Schema string              `json:"schema"`
	Basis  string              `json:"basis"`
	Counts G1TypeAudit76Counts `json:"counts"`
	Rows   []G1TypeAudit76Row  `json:"rows"`
}

// G1TypeAudit76Sources is what the type audit is compared with: the r2 expected asset, nothing else.
type G1TypeAudit76Sources struct {
	Expected []byte
}

func g1t76Fault(code string) error { return fail(KindInvalidInput, code, CSharpG1TypeAudit76Path, nil) }

func g1t76Sorted(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return out
}

// g1t76Compare compares the types of a row with the ts of the alternative. A derived set is equal to the ts, or the ts
// adds node types of the same extent that the row lists; a set of candidates must be equal to the ts.
func g1t76Compare(r G1TypeAudit76Row, ts []string) (result, relation string) {
	ts = g1t76Sorted(ts)
	in := func(s []string, x string) bool { return slices.Contains(s, x) }
	allIn := func(a, b []string) bool {
		for _, x := range a {
			if !in(b, x) {
				return false
			}
		}
		return true
	}
	switch {
	case len(ts) == 0:
		return G1AuditMismatch, G1TypeRelDisjoint
	case slices.Equal(ts, g1t76Sorted(r.Types)) && r.Class == G1TypeNotDerivable:
		return G1AuditMatch, G1TypeRelCandidates
	case slices.Equal(ts, g1t76Sorted(r.Types)):
		return G1AuditMatch, G1TypeRelEqual
	case r.Class != G1TypeNotDerivable && allIn(r.Types, ts) && allIn(ts, append(slices.Clone(r.Types), r.Coincident...)):
		return G1AuditMatch, G1TypeRelCoincident
	case allIn(ts, r.Types):
		return G1AuditMismatch, G1TypeRelSubset
	case allIn(r.Types, ts):
		return G1AuditMismatch, G1TypeRelSuperset
	case slices.ContainsFunc(ts, func(x string) bool { return in(r.Types, x) }):
		return G1AuditMismatch, G1TypeRelOverlap
	}
	return G1AuditMismatch, G1TypeRelDisjoint
}

// g1t76InitialTS is the ts of the alternative before the ts corrections of the r2 asset.
func g1t76InitialTS(item string, ts []string, cs []G1Correction) ([]string, error) {
	for _, c := range cs {
		if c.Item == item && c.Field == G1FieldTS {
			var old []string
			if err := json.Unmarshal([]byte(c.Old), &old); err != nil {
				return ts, g1t76Fault("G1_TYPEAUDIT76_CORRECTION")
			}
			ts = old
		}
	}
	return ts, nil
}

func g1t76Fill(r *G1TypeAudit76Row, alt G1ExpAlt, cs []G1Correction) error {
	r.Result, r.Relation = g1t76Compare(*r, alt.TS)
	initial, err := g1t76InitialTS(alt.ID, alt.TS, cs)
	if err != nil {
		return err
	}
	r.Initial, _ = g1t76Compare(*r, initial)
	return nil
}

func g1t76Counts(rows []G1TypeAudit76Row) G1TypeAudit76Counts {
	n := G1TypeAudit76Counts{Rows: len(rows)}
	for _, r := range rows {
		if r.Result == G1AuditMatch {
			n.Match++
		} else {
			n.Mismatch++
		}
		if r.Initial == G1AuditMismatch && r.Result == G1AuditMatch {
			n.Corrected++
		}
		switch r.Class {
		case G1TypeNode:
			n.Node++
		case G1TypeNoOwnNode:
			n.NoOwnNode++
		case G1TypeAlias:
			n.Alias++
		case G1TypeExternal:
			n.External++
		case G1TypeNotDerivable:
			n.NotDerivable++
		}
	}
	return n
}

// DeriveCSharpG1TypeAudit76 completes the authored rows (class, types, grounds) with their comparison against the r2 asset.
func DeriveCSharpG1TypeAudit76(authored []G1TypeAudit76Row, in G1TypeAudit76Sources) (CSharpG1TypeAudit76, error) {
	out := CSharpG1TypeAudit76{Schema: CSharpG1TypeAudit76Schema, Basis: csharpG1TypeAudit76Basis}
	x, alts, err := g1a76Differential(in.Expected)
	if err != nil {
		return out, err
	}
	rows := slices.Clone(authored)
	slices.SortFunc(rows, func(a, b G1TypeAudit76Row) int { return strings.Compare(a.ID, b.ID) })
	if len(rows) != len(alts) {
		return out, g1t76Fault("G1_TYPEAUDIT76_ROWS")
	}
	for i := range rows {
		if rows[i].ID != alts[i].ID {
			return out, g1t76Fault("G1_TYPEAUDIT76_ROWS")
		}
		if err := g1t76Fill(&rows[i], alts[i], x.Corrections); err != nil {
			return out, err
		}
	}
	out.Rows, out.Counts = rows, g1t76Counts(rows)
	return out, nil
}

// DecodeCSharpG1TypeAudit76 decodes the asset strictly (size bound, no unknown member, schema).
func DecodeCSharpG1TypeAudit76(data []byte) (CSharpG1TypeAudit76, error) {
	var x CSharpG1TypeAudit76
	if len(data) > csharpG1TypeAudit76Max {
		return x, fail(KindResourceLimit, "G1_TYPEAUDIT76_SIZE", CSharpG1TypeAudit76Path, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_TYPEAUDIT76_JSON", CSharpG1TypeAudit76Path, err)
	}
	if x.Schema != CSharpG1TypeAudit76Schema || x.Basis == "" {
		return x, g1t76Fault("G1_TYPEAUDIT76_SCHEMA")
	}
	return x, nil
}

// MarshalCSharpG1TypeAudit76 writes the canonical bytes: the header on a few lines, one row per line.
func MarshalCSharpG1TypeAudit76(x CSharpG1TypeAudit76) ([]byte, error) {
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

// g1t76CheckRow is the form of an authored row: the class, the type names (sorted, distinct, node names or the quoted
// anonymous tokens of ts), the extent mark of an external row and only an external row, a reason and grammar grounds.
func g1t76CheckRow(r G1TypeAudit76Row) bool {
	names := func(s []string) bool {
		for i, x := range s {
			if !g1t76TypeName.MatchString(x) || i > 0 && s[i-1] >= x {
				return false
			}
		}
		return true
	}
	switch r.Class {
	case G1TypeNode, G1TypeNoOwnNode, G1TypeAlias, G1TypeExternal:
		if len(r.Types) != 1 {
			return false
		}
	case G1TypeNotDerivable:
		if len(r.Types) < 2 || len(r.Coincident) != 0 {
			return false
		}
	default:
		return false
	}
	if !names(r.Types) || !names(r.Coincident) || r.Reason == "" || !g1a76CheckGrounds(r.Grounds, G1DocGrammar) {
		return false
	}
	for _, c := range r.Coincident {
		if slices.Contains(r.Types, c) {
			return false
		}
	}
	return (r.Class == G1TypeExternal) == (r.Extent == G1TypeExtentElsewhere) && (r.Class == G1TypeExternal || r.Extent == "")
}

// ParseCSharpG1TypeAudit76 decodes the asset and verifies it: its rows are exactly the differential-oracle alternatives of the
// r2 asset, every row is well formed with checkable grammar grounds, the comparison each row states is the comparison made
// now against the ts of the r2 asset (and against the ts before the corrections), and the bytes are canonical.
func ParseCSharpG1TypeAudit76(data []byte, in G1TypeAudit76Sources) (CSharpG1TypeAudit76, error) {
	x, err := DecodeCSharpG1TypeAudit76(data)
	if err != nil {
		return x, err
	}
	r2, alts, err := g1a76Differential(in.Expected)
	if err != nil {
		return x, err
	}
	if len(x.Rows) != len(alts) {
		return x, g1t76Fault("G1_TYPEAUDIT76_ROWS")
	}
	for i, r := range x.Rows {
		if r.ID != alts[i].ID {
			return x, g1t76Fault("G1_TYPEAUDIT76_ROWS")
		}
		if !g1t76CheckRow(r) {
			return x, g1t76Fault("G1_TYPEAUDIT76_ROW")
		}
		want := r
		if err := g1t76Fill(&want, alts[i], r2.Corrections); err != nil {
			return x, err
		}
		if want.Result != r.Result || want.Relation != r.Relation || want.Initial != r.Initial {
			return x, g1t76Fault("G1_TYPEAUDIT76_COMPARISON")
		}
	}
	if x.Counts != g1t76Counts(x.Rows) {
		return x, g1t76Fault("G1_TYPEAUDIT76_COUNTS")
	}
	if x.Basis != csharpG1TypeAudit76Basis {
		return x, g1t76Fault("G1_TYPEAUDIT76_SCHEMA")
	}
	canonical, err := MarshalCSharpG1TypeAudit76(x)
	if err != nil || !bytes.Equal(canonical, data) {
		return x, g1t76Fault("G1_TYPEAUDIT76_BYTES")
	}
	return x, nil
}

// CheckG1TypeAudit76Grounds checks every ground of the rows against the pinned grammar.js (the live counterpart of the offline check).
func CheckG1TypeAudit76Grounds(rows []G1TypeAudit76Row, grammar string) error {
	for _, r := range rows {
		if err := CheckG1GroundList(r.ID, r.Grounds, grammar, nil); err != nil {
			return fmt.Errorf("type audit row %w", err)
		}
	}
	return nil
}
