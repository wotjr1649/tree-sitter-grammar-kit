package kit

import (
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"reflect"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The predicate scope (#362 scope 3) is the 20 PREDICATE_REQUIRED tuples and the 47 host-policy
// EXPECTED_REJECTION rows per host that the P0 registry fixes. The ordinary reducers never accept
// these coordinates (they require Kind STRUCTURAL_ONLY and an empty predicate list); this file is
// the separate typed route: independent expected values, the shared witness verifier for the
// EVALUATED tuples, an outcome check for the rejection rows, and a coverage fold per host.

const (
	PredicateExpectedSchema = "tsgk-query-predicate-expected/r1"
	PredicateExpectedPath   = "src/contracts/query-predicate-expected-r1.json"
	// shortcut: the asset is 81 KB today; raise only with a new expected revision.
	predicateExpectedMaxBytes = 256 * 1024
	predicateQueryID          = "q0"
)

// PredicateExpectedTuple is one independently authored tuple. Nil arrays are rejected: a
// complete empty stream is [] and an unavailable one is never registered.
type PredicateExpectedTuple struct {
	ID         string               `json:"id"`
	Source     string               `json:"source"`
	Query      string               `json:"query"`
	Encoding   string               `json:"encoding"`
	Step       uint32               `json:"step"`
	TreeRole   string               `json:"tree_role"`
	Metadata   QueryWitnessMetadata `json:"metadata"`
	Tree       []TreeNode           `json:"tree"`
	Structural []Capture            `json:"structural"`
	Expected   []Capture            `json:"expected"`
}

// PredicateExpected is the strict decoded tsgk-query-predicate-expected/r1 asset.
type PredicateExpected struct {
	Schema   string                   `json:"schema"`
	Registry queryregistrycore.Pin    `json:"registry"`
	Grammar  queryregistrycore.Pin    `json:"grammar"`
	Basis    string                   `json:"basis"`
	Tuples   []PredicateExpectedTuple `json:"tuples"`
}

func predicateFault(code string) error {
	return fail(KindInvalidInput, code, PredicateExpectedPath, nil)
}

// ParsePredicateExpected decodes the asset and binds it to the exact P0 registry bytes and the
// fixed roster: same IDs in roster order, the registered source, query and encoding, a nonempty
// predicate list (an empty list is STRUCTURAL_ONLY, never a predicate tuple), non-nil arrays,
// and an expected stream made of whole matches of the structural stream.
func ParsePredicateExpected(data, registry []byte) (PredicateExpected, error) {
	var e PredicateExpected
	if len(data) > predicateExpectedMaxBytes {
		return e, fail(KindResourceLimit, "PREDICATE_EXPECTED_SIZE", PredicateExpectedPath, nil)
	}
	if err := jsonv2.Unmarshal(data, &e, jsonv2.RejectUnknownMembers(true)); err != nil {
		return e, fail(KindInvalidInput, "PREDICATE_EXPECTED_JSON", PredicateExpectedPath, err)
	}
	if e.Schema != PredicateExpectedSchema || e.Basis == "" {
		return e, predicateFault("PREDICATE_EXPECTED_SCHEMA")
	}
	if e.Registry != queryregistrycore.FilePin(queryregistrycore.Path, registry) {
		return e, predicateFault("PREDICATE_EXPECTED_REGISTRY_PIN")
	}
	reg, err := queryregistrycore.Parse(registry)
	if err != nil || !reflect.DeepEqual(reg.Probes, queryregistrycore.Roster()) {
		return e, predicateFault("PREDICATE_EXPECTED_REGISTRY")
	}
	var want []queryregistrycore.Probe
	for _, p := range reg.Probes {
		if p.Kind == queryregistrycore.Predicate {
			want = append(want, p)
		}
	}
	if len(e.Tuples) != len(want) {
		return e, predicateFault("PREDICATE_EXPECTED_MEMBERSHIP")
	}
	for i, t := range e.Tuples {
		p := want[i]
		if t.ID != p.ID || t.Source != p.Source || t.Query != p.Query || t.Encoding != p.Encoding || t.Step != 0 || t.TreeRole != "incremental" {
			return e, predicateFault("PREDICATE_EXPECTED_MEMBERSHIP")
		}
		if t.Tree == nil || t.Structural == nil || t.Expected == nil || t.Metadata.CaptureNames == nil || t.Metadata.Predicates == nil || len(t.Tree) == 0 {
			return e, predicateFault("PREDICATE_EXPECTED_NULL")
		}
		if len(t.Metadata.Predicates) == 0 || t.Metadata.Patterns == 0 {
			return e, predicateFault("PREDICATE_EXPECTED_STRUCTURAL_ONLY")
		}
		if t.Tree[0].Parent != -1 || uint64(t.Tree[0].EndByte) != uint64(len(t.Source)) {
			return e, predicateFault("PREDICATE_EXPECTED_TREE")
		}
		for _, c := range append(slices.Clone(t.Structural), t.Expected...) {
			if c.Node < 0 || c.Node >= int64(len(t.Tree)) || c.Pattern >= t.Metadata.Patterns || int(c.Capture) >= len(t.Metadata.CaptureNames) || t.Metadata.CaptureNames[c.Capture] != c.Name {
				return e, predicateFault("PREDICATE_EXPECTED_CAPTURE")
			}
		}
		if !predicateWholeMatches(t.Structural, t.Expected) {
			return e, predicateFault("PREDICATE_EXPECTED_SUBSEQUENCE")
		}
	}
	return e, nil
}

// predicateWholeMatches reports whether kept is the structural stream with whole matches removed:
// a predicate keeps or drops a match, never one of its captures, and never renumbers.
func predicateWholeMatches(structural, kept []Capture) bool {
	i := 0
	for i < len(structural) {
		j := i
		for j < len(structural) && structural[j].Match == structural[i].Match {
			j++
		}
		if len(kept) >= j-i && CompareCaptures(structural[i:j], kept[:j-i]) == nil {
			kept = kept[j-i:]
		}
		i = j
	}
	return len(kept) == 0
}

// Tuple returns the registered tuple for a roster ID.
func (e PredicateExpected) Tuple(id string) (PredicateExpectedTuple, bool) {
	for _, t := range e.Tuples {
		if t.ID == id {
			return t, true
		}
	}
	return PredicateExpectedTuple{}, false
}

// Registration returns the witness registration of a tuple and its input source. The platform is
// the registration identity of the host that observed the run, never inferred from results.
func (t PredicateExpectedTuple) Registration(platform string) (QueryWitnessRegistration, []byte) {
	source := []byte(t.Source)
	input := NativeInput{Path: "case.txt", Role: "case", Bytes: uint64(len(source)), SHA256: digestHex(source)}
	return QueryWitnessRegistration{Schema: QueryWitnessRegistrationSchema, Policy: "tsgk-predicates/r1", Case: "oracle",
		Route: "owned-plain", Platform: platform, Role: "owned", Encoding: t.Encoding, Input: input, Edits: []Edit{},
		Queries:  []QueryWitnessQuery{{ID: predicateQueryID, Source: t.Query, Metadata: &t.Metadata}},
		Required: []QueryWitnessTuple{{Query: predicateQueryID, Step: t.Step, TreeRole: t.TreeRole, Structural: t.Structural, Expected: t.Expected}}}, source
}

// VerifyPredicateTuple checks one stored native run of a tuple. The shared witness verifier
// recomputes raw -> evaluated streams and compares them to the independent structural and
// evaluated expectations; this adds the independent full CST and requires an EVALUATED stream.
// A projection that calls a predicate tuple structural, or a tuple registered without
// predicates, cannot be counted: Evaluated must be exactly one.
func VerifyPredicateTuple(ctx context.Context, t PredicateExpectedTuple, platform string, raw, projection []byte) (QueryWitnessResult, error) {
	registration, source := t.Registration(platform)
	encoded, err := json.Marshal(registration)
	if err != nil {
		return QueryWitnessResult{}, err
	}
	got, err := VerifyQueryWitness(ctx, QueryWitnessRequest{Registration: encoded, Source: source, Raw: raw, Projection: projection})
	if err != nil {
		return got, err
	}
	if got.Assessment == AssessPass && (got.Required != 1 || got.Verified != 1 || got.Evaluated != 1) {
		got.Assessment = AssessFail
		got.Findings = append(got.Findings, Finding{Severity: "error", Code: "PREDICATE_TUPLE_NOT_EVALUATED", Path: t.ID, Message: "predicate tuple는 EVALUATED로 검증되어야 한다"})
		return got, nil
	}
	var p witnessProjection
	if got.Assessment == AssessPass {
		if err := json.Unmarshal(projection, &p); err != nil || len(p.Steps) != 1 || p.Steps[0].Incremental == nil || p.Steps[0].Incremental.Tree == nil ||
			!reflect.DeepEqual(p.Steps[0].Incremental.Tree.Nodes, t.Tree) {
			got.Assessment = AssessFail
			got.Findings = append(got.Findings, Finding{Severity: "error", Code: "PREDICATE_TUPLE_CST", Path: t.ID, Message: "실제 full CST가 독립 기대 CST와 다르다"})
		}
	}
	return got, nil
}

// PredicateControlOutcome is what a host evaluation produced for one rejection row.
type PredicateControlOutcome struct {
	Status, Code, Evaluation string
	Partial                  bool
	// Captures is "NULL" (no stream), "EMPTY" (completed empty stream) or "PRESENT".
	Captures string
	// Structural is the number of structural captures kept next to the outcome.
	Structural int
	// CapturesAreStructural is true when the kept stream equals the structural stream.
	CapturesAreStructural bool
}

// CheckPredicateControl compares an outcome with the registered expectation of an
// EXPECTED_REJECTION row. A rejection is never turned into a pass: its status and code are
// exact, its evaluated stream stays null (not []), and the structural stream is preserved.
func CheckPredicateControl(p queryregistrycore.Probe, o PredicateControlOutcome) error {
	bad := func(field string) error { return fmt.Errorf("PREDICATE_CONTROL_%s: %s", field, p.ID) }
	if p.Kind != queryregistrycore.Control {
		return bad("KIND")
	}
	if o.Status != p.Status {
		return bad("STATUS")
	}
	if o.Code != p.Code {
		return bad("CODE")
	}
	if o.Evaluation != p.Evaluation || o.Partial != p.Partial {
		return bad("EVALUATION")
	}
	switch p.Expected {
	case "NULL":
		if o.Captures != "NULL" {
			return bad("CAPTURES_NOT_NULL")
		}
	case "PRESERVED_STRUCTURAL":
		if o.Captures != "PRESENT" || !o.CapturesAreStructural {
			return bad("STRUCTURAL_STREAM")
		}
	default:
		return bad("EXPECTED")
	}
	switch p.Structural {
	case "PRESERVED_4":
		if o.Structural != 4 {
			return bad("STRUCTURAL_LOST")
		}
	case "NULL":
		if o.Structural != 0 {
			return bad("STRUCTURAL_PRESENT")
		}
	default:
		return bad("STRUCTURAL")
	}
	return nil
}

// PredicateScopeCounter keeps required = verified + failed + blocked + unprocessed per kind and host.
type PredicateScopeCounter struct {
	Kind        string `json:"kind"`
	Platform    string `json:"platform"`
	Required    uint64 `json:"required"`
	Verified    uint64 `json:"verified"`
	Failed      uint64 `json:"failed"`
	Blocked     uint64 `json:"blocked"`
	Unprocessed uint64 `json:"unprocessed"`
}

// PredicateScopeCoverage is a scope-local result. It never changes the P0 root: the registry
// stays INCOMPLETE and full_scope_complete stays false.
type PredicateScopeCoverage struct {
	Counters          []PredicateScopeCounter `json:"counters"`
	FullScopeComplete bool                    `json:"full_scope_complete"`
}

// FoldPredicateScope folds adjudicated judgments of the predicate scope. It enumerates the 40
// positive and 94 rejection coordinates from the pinned registry, so a deleted, extra, duplicated
// or renamed obligation is an error. An absent judgment is BLOCKED on a platform listed in
// blocked (registered but not run there) and UNPROCESSED elsewhere. VERIFIED needs an empty code;
// a rejection row must carry its registered subject; a positive row carries none.
func FoldPredicateScope(registry []byte, registrySHA string, judgments []queryregistrycore.Judgment, blocked map[string]bool) (PredicateScopeCoverage, error) {
	var out PredicateScopeCoverage
	if err := queryregistrycore.Preflight(registry, registrySHA); err != nil {
		return out, err
	}
	reg, err := queryregistrycore.Parse(registry)
	if err != nil {
		return out, err
	}
	if !reflect.DeepEqual(reg.Probes, queryregistrycore.Roster()) || reg.Namespace != queryregistrycore.Namespace(reg) {
		return out, predicateFault("PREDICATE_SCOPE_REGISTRY")
	}
	coordinates := queryregistrycore.Expand(nil, queryregistrycore.Platforms(), reg.Probes, nil)
	byID := make(map[string]queryregistrycore.Coordinate, len(coordinates))
	for _, c := range coordinates {
		byID[c.ID(reg.Namespace)] = c
	}
	seen := map[string]queryregistrycore.Judgment{}
	for _, j := range judgments {
		c, ok := byID[j.ID]
		if !ok || c != j.Coordinate {
			return out, predicateFault("PREDICATE_SCOPE_UNDECLARED")
		}
		if _, dup := seen[j.ID]; dup {
			return out, predicateFault("PREDICATE_SCOPE_OVERLAP")
		}
		seen[j.ID] = j
	}
	index := map[string]int{}
	for _, c := range coordinates {
		key := c.Kind + "/" + c.Platform
		if _, ok := index[key]; !ok {
			index[key] = len(out.Counters)
			out.Counters = append(out.Counters, PredicateScopeCounter{Kind: c.Kind, Platform: c.Platform})
		}
		counter := &out.Counters[index[key]]
		counter.Required++
		j, ok := seen[c.ID(reg.Namespace)]
		state := "UNPROCESSED"
		if ok {
			state = j.State
		}
		switch state {
		case "VERIFIED":
			if j.Code != "" {
				return out, predicateFault("PREDICATE_SCOPE_VERIFIED_CODE")
			}
			if c.Kind == queryregistrycore.Control {
				p := reg.Probes[slices.IndexFunc(reg.Probes, func(p queryregistrycore.Probe) bool { return p.ID == c.Case })]
				if j.Subject == nil || *j.Subject != (queryregistrycore.Subject{Status: p.Status, Code: p.Code, Evaluation: p.Evaluation, Partial: p.Partial, Structural: p.Structural, Expected: p.Expected}) {
					return out, predicateFault("PREDICATE_SCOPE_CONTROL_SUBJECT")
				}
			} else if j.Subject != nil {
				return out, predicateFault("PREDICATE_SCOPE_POSITIVE_RECLASSIFIED")
			}
			counter.Verified++
		case "FAILED", "BLOCKED":
			if j.Code == "" {
				return out, predicateFault("PREDICATE_SCOPE_CODE")
			}
			if state == "FAILED" {
				counter.Failed++
			} else {
				counter.Blocked++
			}
		case "UNPROCESSED":
			if blocked[c.Platform] {
				counter.Blocked++
			} else {
				counter.Unprocessed++
			}
		default:
			return out, predicateFault("PREDICATE_SCOPE_STATE")
		}
	}
	for _, c := range out.Counters {
		if c.Required != c.Verified+c.Failed+c.Blocked+c.Unprocessed {
			return out, predicateFault("PREDICATE_SCOPE_SUM")
		}
	}
	return out, nil
}
