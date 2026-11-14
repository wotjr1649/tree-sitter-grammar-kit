package kit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The type audit r2 of the G1-3b anchors of csharp-B03 and csharp-B04 (#233, after the native comparison).
//
// The type audit r1 (csharp_g1_b34_audit.go) compares the types of an anchor lexeme with the union of the types the expected asset gives
// that lexeme over all alternatives. For a lexeme whose type depends on its place (the audit's class NOT_DERIVABLE: var, _, new, ref, the
// contextual keywords, ...) it keeps the candidate set and the comparison asks only that the union equals it, so an alternative whose own
// types are wrong cannot show: another alternative fills the union. The four `var` anchors of csharp-B03.a184, a236 and csharp-B04.a028,
// a029 are that case. r2 compares each anchor of such a lexeme with the types the pinned grammar text gives the lexeme in the place of its
// alternative:
//
//   - roles: for each lexeme of NOT_DERIVABLE class of r1, the places the grammar text knows (a role: its name, the node types the
//     grammar gives the lexeme there, a reason and grammar grounds pinned to the row);
//   - contexts: for every anchor of such a lexeme in the EXPECTED A alternatives, the role of the alternative's production (the place the
//     production names, not the place of the witness) and the comparison of the types of that anchor with the types of the role:
//     EQUAL, EXPECTED_SUPERSET (the expected types hold the role's types and others the grammar does not give there), EXPECTED_SUBSET, OVERLAP (all
//     MATCH) or DISJOINT (MISMATCH: the expected types name nothing the grammar gives in that place).
//
// The audit is made after the native comparison and from grammar.js alone; the roles name no Roslyn kind, the kind map or any native
// output. r1 is frozen: it audits the sealed expected asset r1 and r2 audits the expected asset r2. Everything the corrections did not
// touch is r1's audit of the same alternatives.

const (
	CSharpG1TypeAuditB34SchemaR2 = "tsgk-csharp-g1-typeaudit-b03b04/r2"
	CSharpG1TypeAuditB34PathR2   = "src/contracts/csharp-g1-typeaudit-b03b04-r2.json"

	// the relations are those of the type audits of G1-3a and of r1: WIDER is the expected types holding the role's types and others (EXPECTED_SUPERSET)
	g1b34RelEqual    = G1TypeRelEqual
	g1b34RelWider    = G1TypeRelSuperset
	g1b34RelNarrower = G1TypeRelSubset
	g1b34RelOverlap  = G1TypeRelOverlap
	g1b34RelDisjoint = G1TypeRelDisjoint
)

// G1B34Role is a place of an anchor lexeme in the grammar text and the node types the grammar gives the lexeme there.
type G1B34Role struct {
	Lexeme  string     `json:"lexeme"`
	Role    string     `json:"role"`
	Types   []string   `json:"types"`
	Reason  string     `json:"reason"`
	Grounds []G1Ground `json:"ground"`
}

// G1B34Context is the anchor Index of the alternative Alt, its role and the comparison of its types with the types of the role.
type G1B34Context struct {
	Alt      string   `json:"alt"`
	Index    int      `json:"index"`
	Text     string   `json:"text"`
	Role     string   `json:"role"`
	Place    string   `json:"place"`
	Types    []string `json:"types"`
	Result   string   `json:"result"`
	Relation string   `json:"relation"`
}

type G1B34ContextCounts struct {
	Roles    int `json:"roles"`
	Contexts int `json:"contexts"`
	Match    int `json:"match"`
	Mismatch int `json:"mismatch"`
	Equal    int `json:"equal"`
	Wider    int `json:"wider"`
	Narrower int `json:"narrower"`
	Overlap  int `json:"overlap"`
	Findings int `json:"findings"`
}

type CSharpG1TypeAuditB34R2 struct {
	Schema   string                `json:"schema"`
	Basis    string                `json:"basis"`
	Previous queryregistrycore.Pin `json:"previous"`
	Counts   G1B34ContextCounts    `json:"counts"`
	Findings []G1B34Finding        `json:"findings"`
	Roles    []G1B34Role           `json:"roles"`
	Contexts []G1B34Context        `json:"contexts"`
}

const csharpG1TypeAuditB34R2Basis = "Hand derivation from the pinned grammar.js alone (the grammar.js the kind map pins), after the native comparison, of the node types the grammar text gives each anchor lexeme of class NOT_DERIVABLE of the type audit r1 in each place it occurs (a role), and the role of the production of every anchor of such a lexeme in the EXPECTED A alternatives of csharp-B03 and csharp-B04 (a context; the production names the place, not the witness). The verifier recomputes, for every context, the comparison of the expected types of that anchor with the types of its role: EQUAL, WIDER, NARROWER and OVERLAP are MATCH, DISJOINT is MISMATCH. It audits the expected asset it is given; r1 (frozen) compared the types of a lexeme with the union over all alternatives, which cannot show a wrong alternative when another alternative fills the union. Written without reading the Roslyn kinds, the kind map or any native output; the author knew the result of the native comparison. The audit covers the types of the anchors of those lexemes only: the B alternatives and the other anchor lexemes are the audit r1, which this audit does not repeat, and a derivation is what one reader derived from the grammar text, not a parser run."

// g1b34ContextFindings are the findings of this audit: the alternatives its comparison did not match against the sealed expected asset r1. The
// four anchors were corrected in the expected asset r2 (csharp_g1_b34_correct.go).
var g1b34ContextFindings = []G1B34Finding{
	{ID: "csharp-B03.a184", Audit: "context", What: "anchor var: the sealed types identifier and implicit_parameter name nothing the grammar gives a type position (implicit_type and \"var\"); the audit r1 compared with the union over all alternatives, which a011 and others fill", Fix: "the expected asset r2 sets the types of the anchor to \"var\" and implicit_type"},
	{ID: "csharp-B03.a236", Audit: "context", What: "anchor var: the same disagreement as a184", Fix: "the expected asset r2 sets the types of the anchor to \"var\" and implicit_type"},
	{ID: "csharp-B04.a028", Audit: "context", What: "anchor var: the same disagreement as a184", Fix: "the expected asset r2 sets the types of the anchor to \"var\" and implicit_type"},
	{ID: "csharp-B04.a029", Audit: "context", What: "anchor var: the same disagreement as a184", Fix: "the expected asset r2 sets the types of the anchor to \"var\" and implicit_type"},
}

func g1b34xFault(code string) error {
	return fail(KindInvalidInput, code, CSharpG1TypeAuditB34PathR2, nil)
}

// G1B34AnchorPair is one anchor of an EXPECTED A alternative: the alternative, the position of the anchor in it, its lexeme and its types.
// There is no member for the witness, the Roslyn kind or the kind map.
type G1B34AnchorPair struct {
	Alt   string
	Index int
	Text  string
	TS    []string
}

// g1b34ContextCompare compares the types of an anchor with the types of its role.
func g1b34ContextCompare(role, ts []string) (string, string) {
	both := 0
	for _, t := range ts {
		if slices.Contains(role, t) {
			both++
		}
	}
	switch {
	case both == 0:
		return G1AuditMismatch, g1b34RelDisjoint
	case both == len(ts) && both == len(role):
		return G1AuditMatch, g1b34RelEqual
	case both == len(role):
		return G1AuditMatch, g1b34RelWider
	case both == len(ts):
		return G1AuditMatch, g1b34RelNarrower
	}
	return G1AuditMatch, g1b34RelOverlap
}

func g1b34ContextCounts(x CSharpG1TypeAuditB34R2) G1B34ContextCounts {
	c := G1B34ContextCounts{Roles: len(x.Roles), Contexts: len(x.Contexts), Findings: len(x.Findings)}
	for _, r := range x.Contexts {
		if r.Result == G1AuditMatch {
			c.Match++
		} else {
			c.Mismatch++
		}
		switch r.Relation {
		case g1b34RelEqual:
			c.Equal++
		case g1b34RelWider:
			c.Wider++
		case g1b34RelNarrower:
			c.Narrower++
		case g1b34RelOverlap:
			c.Overlap++
		}
	}
	return c
}

// g1b34ContextPin is the SHA-256 of the role and place of a context: a role assignment that was changed later fails without the documents.
func g1b34ContextPin(c G1B34Context) string {
	s := sha256.Sum256([]byte(strings.Join([]string{c.Alt, fmt.Sprint(c.Index), c.Text, c.Role, c.Place}, "\x1f")))
	return hex.EncodeToString(s[:])
}

func g1b34RoleKey(lexeme, role string) string { return lexeme + "/" + role }

// DeriveCSharpG1TypeAuditB34R2 completes the authored roles and contexts with the comparison of every context against the anchors of the
// expected asset (pairs) and returns the audit r2. previous is the sealed type audit r1 file the audit pins.
func DeriveCSharpG1TypeAuditB34R2(roles []G1B34Role, contexts []G1B34Context, previous []byte, pairs []G1B34AnchorPair) (CSharpG1TypeAuditB34R2, error) {
	out := CSharpG1TypeAuditB34R2{Schema: CSharpG1TypeAuditB34SchemaR2, Basis: csharpG1TypeAuditB34R2Basis, Findings: slices.Clone(g1b34ContextFindings),
		Previous: queryregistrycore.FilePin(CSharpG1TypeAuditB34Path, previous)}
	roles, contexts = slices.Clone(roles), slices.Clone(contexts)
	slices.SortFunc(roles, func(a, b G1B34Role) int {
		return strings.Compare(g1b34RoleKey(a.Lexeme, a.Role), g1b34RoleKey(b.Lexeme, b.Role))
	})
	nd, err := g1b34NotDerivable(previous)
	if err != nil {
		return out, err
	}
	var want []G1B34AnchorPair
	for _, p := range pairs {
		if nd[p.Text] != nil {
			want = append(want, p)
		}
	}
	if len(contexts) != len(want) {
		return out, g1b34xFault("G1_TYPEAUDITB34R2_ROWS")
	}
	types := map[string][]string{}
	for _, r := range roles {
		types[g1b34RoleKey(r.Lexeme, r.Role)] = r.Types
	}
	for i := range contexts {
		c, p := &contexts[i], want[i]
		rt, ok := types[g1b34RoleKey(p.Text, c.Role)]
		if c.Alt != p.Alt || c.Index != p.Index || c.Text != p.Text || !ok {
			return out, g1b34xFault("G1_TYPEAUDITB34R2_ROWS")
		}
		c.Types = rt
		c.Result, c.Relation = g1b34ContextCompare(rt, p.TS)
	}
	out.Roles, out.Contexts = roles, contexts
	out.Counts = g1b34ContextCounts(out)
	return out, nil
}

// g1b34NotDerivable returns the candidate set of each anchor lexeme the type audit r1 classifies NOT_DERIVABLE.
func g1b34NotDerivable(previous []byte) (map[string][]string, error) {
	r1, err := DecodeCSharpG1TypeAuditB34(previous)
	if err != nil {
		return nil, err
	}
	nd := map[string][]string{}
	for _, a := range r1.Anchors {
		if a.Class == G1TypeNotDerivable {
			nd[a.Text] = a.Types
		}
	}
	return nd, nil
}

func DecodeCSharpG1TypeAuditB34R2(data []byte) (CSharpG1TypeAuditB34R2, error) {
	var x CSharpG1TypeAuditB34R2
	if len(data) > csharpG1AuditB34Max {
		return x, fail(KindResourceLimit, "G1_TYPEAUDITB34R2_SIZE", CSharpG1TypeAuditB34PathR2, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_TYPEAUDITB34R2_JSON", CSharpG1TypeAuditB34PathR2, err)
	}
	if x.Schema != CSharpG1TypeAuditB34SchemaR2 || x.Basis == "" {
		return x, g1b34xFault("G1_TYPEAUDITB34R2_SCHEMA")
	}
	return x, nil
}

func MarshalCSharpG1TypeAuditB34R2(x CSharpG1TypeAuditB34R2) ([]byte, error) {
	return g1b34Write([]g1b34Member{{"schema", x.Schema}, {"basis", x.Basis}, {"previous", x.Previous}, {"counts", x.Counts}},
		[]g1b34List{
			{"findings", len(x.Findings), func(i int) any { return x.Findings[i] }},
			{"roles", len(x.Roles), func(i int) any { return x.Roles[i] }},
			{"contexts", len(x.Contexts), func(i int) any { return x.Contexts[i] }},
		})
}

// ParseCSharpG1TypeAuditB34R2 decodes the asset and verifies it against the sealed r1 audit file and the anchors of the expected asset it
// audits: it pins that r1 file; its roles are well formed (types of the lexeme's r1 candidate set, a reason, grammar grounds pinned to the
// row) and each is used; its contexts are exactly the anchors of the NOT_DERIVABLE lexemes, in order, each with a role of its lexeme, the
// types of that role and the comparison made now; the findings are the authored ones.
func ParseCSharpG1TypeAuditB34R2(data []byte, previous []byte, pairs []G1B34AnchorPair) (CSharpG1TypeAuditB34R2, error) {
	x, err := DecodeCSharpG1TypeAuditB34R2(data)
	if err != nil {
		return x, err
	}
	if x.Previous != queryregistrycore.FilePin(CSharpG1TypeAuditB34Path, previous) {
		return x, g1b34xFault("G1_TYPEAUDITB34R2_PIN")
	}
	nd, err := g1b34NotDerivable(previous)
	if err != nil {
		return x, err
	}
	used := map[string]bool{}
	for i, r := range x.Roles {
		cand, ok := nd[r.Lexeme]
		if !ok || r.Role == "" || r.Reason == "" || !g1b34RoleTypes(r.Types, cand) || !g1b34CheckGrounds(r.Grounds, G1DocGrammar) ||
			i > 0 && g1b34RoleKey(x.Roles[i-1].Lexeme, x.Roles[i-1].Role) >= g1b34RoleKey(r.Lexeme, r.Role) {
			return x, g1b34xFault("G1_TYPEAUDITB34R2_ROLE")
		}
		if G1GroundPin(r.Grounds) != g1b34RolePins[g1b34RoleKey(r.Lexeme, r.Role)] {
			return x, g1b34xFault("G1_TYPEAUDITB34R2_GROUND")
		}
		used[g1b34RoleKey(r.Lexeme, r.Role)] = false
	}
	for _, c := range x.Contexts {
		if _, ok := used[g1b34RoleKey(c.Text, c.Role)]; !ok || c.Place == "" {
			return x, g1b34xFault("G1_TYPEAUDITB34R2_ROW")
		}
		if g1b34ContextPin(c) != g1b34ContextPins[c.Alt+"#"+fmt.Sprint(c.Index)] {
			return x, g1b34xFault("G1_TYPEAUDITB34R2_GROUND")
		}
		used[g1b34RoleKey(c.Text, c.Role)] = true
	}
	for _, u := range used {
		if !u {
			return x, g1b34xFault("G1_TYPEAUDITB34R2_ROLE")
		}
	}
	want, err := DeriveCSharpG1TypeAuditB34R2(x.Roles, x.Contexts, previous, pairs)
	if err != nil {
		return x, err
	}
	if !g1b34Same(x.Roles, want.Roles) || !g1b34Same(x.Contexts, want.Contexts) {
		return x, g1b34xFault("G1_TYPEAUDITB34R2_COMPARISON")
	}
	if !g1b34Same(x.Findings, want.Findings) {
		return x, g1b34xFault("G1_TYPEAUDITB34R2_FINDINGS")
	}
	if x.Counts != g1b34ContextCounts(x) || x.Basis != csharpG1TypeAuditB34R2Basis {
		return x, g1b34xFault("G1_TYPEAUDITB34R2_COUNTS")
	}
	canonical, err := MarshalCSharpG1TypeAuditB34R2(x)
	if err != nil || !bytes.Equal(canonical, data) {
		return x, g1b34xFault("G1_TYPEAUDITB34R2_BYTES")
	}
	return x, nil
}

// g1b34RoleTypes: the types of a role are sorted, without duplicate, named, and in the candidate set r1 keeps for the lexeme.
func g1b34RoleTypes(types, candidates []string) bool {
	if len(types) == 0 || !slices.IsSorted(types) || len(slices.Compact(slices.Clone(types))) != len(types) {
		return false
	}
	for _, t := range types {
		if !g1b34TypeName.MatchString(t) || !slices.Contains(candidates, t) {
			return false
		}
	}
	return true
}

// G1B34ContextGroundPins are the pins of the roles ("<lexeme>/<role>") of the audit r2.
func G1B34ContextGroundPins(x CSharpG1TypeAuditB34R2) map[string]string {
	pins := map[string]string{}
	for _, r := range x.Roles {
		pins[g1b34RoleKey(r.Lexeme, r.Role)] = G1GroundPin(r.Grounds)
	}
	return pins
}

// G1B34ContextRowPins are the pins of the contexts ("<alt>#<index>") of the audit r2.
func G1B34ContextRowPins(x CSharpG1TypeAuditB34R2) map[string]string {
	pins := map[string]string{}
	for _, c := range x.Contexts {
		pins[c.Alt+"#"+fmt.Sprint(c.Index)] = g1b34ContextPin(c)
	}
	return pins
}

// CheckG1TypeAuditB34R2Grounds checks every ground of the roles against the pinned grammar.js (the live counterpart of the offline check).
func CheckG1TypeAuditB34R2Grounds(x CSharpG1TypeAuditB34R2, grammar string) error {
	for _, r := range x.Roles {
		if err := CheckG1GroundList(g1b34RoleKey(r.Lexeme, r.Role), r.Grounds, grammar, nil); err != nil {
			return fmt.Errorf("type audit r2 role %w", err)
		}
	}
	return nil
}
