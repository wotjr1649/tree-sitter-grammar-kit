package queryregistry

import "strings"

// Probe is a result-independent declaration of a required predicate example
// or a closed host-policy control.
type Probe struct {
	ID          string       `json:"id"`
	Kind        string       `json:"kind"`
	Source      string       `json:"source"`
	Query       string       `json:"query"`
	PolicyInput *PolicyInput `json:"policy_input"`
	Encoding    string       `json:"encoding"`
	Basis       string       `json:"basis"`
	Status      string       `json:"status"`
	Code        string       `json:"code"`
	Evaluation  string       `json:"evaluation"`
	Partial     bool         `json:"partial"`
	Structural  string       `json:"structural"`
	Expected    string       `json:"expected"`
}

// PolicyInput records the synthetic predicate metadata passed to the host
// policy fixture. Its Query field is a fixture placeholder, not query syntax.
type PolicyInput struct {
	Operator        string   `json:"operator"`
	LeftCapture     string   `json:"left_capture"`
	LeftQuantifier  string   `json:"left_quantifier"`
	RightCapture    string   `json:"right_capture"`
	RightQuantifier string   `json:"right_quantifier"`
	Literals        []string `json:"literals"`
}

const (
	predicateRequired = "PREDICATE_REQUIRED"
	expectedRejection = "EXPECTED_REJECTION"
	utf8              = "UTF-8"
)

// Roster returns the fixed P0 predicate denominator in contract order.
// Probe expectations describe policy outcomes; they are not captured results.
func Roster() []Probe {
	probes := make([]Probe, 0, 67)
	positiveBasis := "src/dev/query-reference/check.py#owned-query-cases"
	addPositive := func(id, source, query string) {
		probes = append(probes, Probe{ID: id, Kind: predicateRequired, Source: source, Query: query, Encoding: utf8, Basis: positiveBasis,
			Status: "COMPLETED", Evaluation: "EVALUATED", Structural: "REGISTER_EXPECTED", Expected: "REGISTER_EVALUATED_EXPECTED"})
	}
	for _, group := range []struct {
		name, op string
	}{
		{"eq", "eq?"}, {"not-eq", "not-eq?"}, {"any-of", "any-of?"}, {"not-any-of", "not-any-of?"}, {"capture-pair", "eq?"}, {"multiple", "multiple"},
	} {
		for _, tc := range []struct{ suffix, source, query string }{
			{"0", "x;\n1;\n", "((identifier) @x (#" + group.op + " @x \"x\"))"},
			{"1", "y;\n1;\n", "((identifier) @x (#" + group.op + " @x \"x\"))"},
		} {
			q := tc.query
			if group.name == "any-of" || group.name == "not-any-of" {
				q = "((identifier) @x (#" + group.op + " @x \"x\" \"z\"))"
			}
			if group.name == "capture-pair" {
				q = "((identifier) @x (#eq? @x @x))"
			}
			if group.name == "multiple" {
				q = "((identifier) @x (#eq? @x \"x\") (#not-eq? @x \"z\"))"
			}
			addPositive(group.name+"-"+tc.suffix, tc.source, q)
		}
	}
	addPositive("two-captures-kept", "x;\n1;\n", "((identifier) @x @y (#eq? @x \"x\"))")
	addPositive("two-captures-rejected", "y;\n1;\n", "((identifier) @x @y (#eq? @x \"x\"))")
	for _, op := range []string{"eq?", "not-eq?"} {
		for _, tc := range []struct {
			suffix, source string
		}{
			{"equal", "x;\nx;\n"}, {"different", "x;\ny;\n"},
		} {
			addPositive("distinct-captures-"+strings.TrimSuffix(op, "?")+"-"+tc.suffix, tc.source,
				"(source_file (expression_statement (identifier) @x) (expression_statement (identifier) @y) (#"+op+" @x @y))")
		}
	}
	addPositive("pattern-isolation", "x;\n1;\n", "((identifier) @x (#eq? @x \"z\"))\n(number) @n")
	probes[len(probes)-1].Basis = "registry-policy/r1#pattern-isolation-proposal"
	probes[len(probes)-1].Expected = "REGISTER_STRUCTURAL_X_N_EVALUATED_N"
	addPositive("evaluated-empty", "", "((identifier) @x (#eq? @x \"x\"))")
	probes[len(probes)-1].Basis = "registry-policy/r1#evaluated-empty-proposal"
	probes[len(probes)-1].Expected = "REGISTER_EMPTY_EVALUATED_ARRAY"
	addControl := func(id, query, encoding, status, code, evaluation, structural, expected string, partial bool, basis string, input *PolicyInput) {
		source := "a b b b"
		if id == "not-run" {
			source = ""
		}
		probes = append(probes, Probe{ID: id, Kind: expectedRejection, Source: source, Query: query, PolicyInput: input, Encoding: encoding, Basis: basis,
			Status: status, Code: code, Evaluation: evaluation, Partial: partial, Structural: structural, Expected: expected})
	}
	placeholder := "owned host-policy fixture"
	policyBasis := "src/internal/native/query_policy_test.go#TestPredicatePolicyFailClosed"
	operators := []string{"match?", "not-match?", "any-eq?", "any-not-eq?", "any-match?", "any-not-match?", "is?", "is-not?", "set!", "select-adjacent!", "strip!", "unknown?"}
	for _, op := range operators {
		addControl("unsupported-"+strings.TrimRight(op, "?!"), placeholder, utf8,
			"UNSUPPORTED", "QUERY_PREDICATE_UNSUPPORTED:"+op, "UNSUPPORTED", "PRESERVED_4", "NULL", false,
			policyBasis+"/"+op, &PolicyInput{Operator: op, LeftCapture: "x", LeftQuantifier: "ONE", Literals: []string{"a"}})
	}
	for _, op := range []string{"eq?", "not-eq?", "any-of?", "not-any-of?"} {
		for _, q := range []struct{ id, quantifier string }{
			{"zero", "ZERO"}, {"zero-or-one", "ZERO_OR_ONE"}, {"zero-or-more", "ZERO_OR_MORE"}, {"one-or-more", "ONE_OR_MORE"},
		} {
			addControl("left-"+strings.TrimSuffix(op, "?")+"-"+q.id, placeholder, utf8,
				"UNSUPPORTED", "QUERY_PREDICATE_UNSUPPORTED:"+op, "UNSUPPORTED", "PRESERVED_4", "NULL", false,
				policyBasis+"/"+op+"/"+q.quantifier,
				&PolicyInput{Operator: op, LeftCapture: "x", LeftQuantifier: q.quantifier, Literals: []string{"a"}})
		}
	}
	for _, op := range []string{"eq?", "not-eq?"} {
		addControl("right-"+strings.TrimSuffix(op, "?")+"-zero-or-one", placeholder, utf8,
			"UNSUPPORTED", "QUERY_PREDICATE_UNSUPPORTED:"+op, "UNSUPPORTED", "PRESERVED_4", "NULL", false,
			policyBasis+"/"+op+"/right-quantifier", &PolicyInput{Operator: op, LeftCapture: "x", LeftQuantifier: "ONE", RightCapture: "y", RightQuantifier: "ZERO_OR_ONE"})
	}
	for _, op := range []string{"eq?", "not-eq?", "any-of?", "not-any-of?"} {
		for _, enc := range []struct{ id, value string }{{"utf16le", "UTF-16LE"}, {"utf16be", "UTF-16BE"}, {"cp949", "CP949"}} {
			addControl("encoding-"+strings.TrimSuffix(op, "?")+"-"+enc.id, placeholder, enc.value,
				"UNSUPPORTED", "QUERY_PREDICATE_ENCODING", "UNSUPPORTED", "PRESERVED_4", "NULL", false,
				policyBasis+"/"+op+"/"+enc.value, &PolicyInput{Operator: op, LeftCapture: "x", LeftQuantifier: "ONE", Literals: []string{"a"}})
		}
	}
	addControl("absent-left-capture", placeholder, utf8,
		"FAILED", "QUERY_PREDICATE_CAPTURE_ABSENT", "UNSUPPORTED", "PRESERVED_4", "NULL", false,
		"src/internal/native/query_policy_test.go#TestPredicatePolicyAbsentCapture", &PolicyInput{Operator: "eq?", LeftCapture: "absent", LeftQuantifier: "ONE", Literals: []string{"a"}})
	addControl("absent-right-capture", placeholder, utf8,
		"FAILED", "QUERY_PREDICATE_CAPTURE_ABSENT", "UNSUPPORTED", "PRESERVED_4", "NULL", false,
		"src/internal/native/query_policy_test.go#TestPredicatePolicyAbsentCapture", &PolicyInput{Operator: "not-eq?", LeftCapture: "x", LeftQuantifier: "ONE", RightCapture: "absent", RightQuantifier: "ONE"})
	addControl("partial-predicate", placeholder, utf8,
		"RESOURCE_LIMIT", "CAPTURE_LIMIT", "UNSUPPORTED", "PRESERVED_4", "NULL", true,
		"src/internal/native/query_policy_test.go#TestPredicatePolicyIncompleteAndEmptyStreams/partial-predicate", &PolicyInput{Operator: "eq?", LeftCapture: "x", LeftQuantifier: "ONE", Literals: []string{"a"}})
	addControl("partial-structural", placeholder, utf8,
		"RESOURCE_LIMIT", "CAPTURE_LIMIT", "STRUCTURAL", "PRESERVED_4", "PRESERVED_STRUCTURAL", true,
		"src/internal/native/query_policy_test.go#TestPredicatePolicyIncompleteAndEmptyStreams/partial-structural", nil)
	addControl("not-run", "", utf8,
		"NOT_RUN", "TREE_NOT_COMPLETED", "", "NULL", "NULL", false,
		"src/internal/native/query_policy_test.go#TestPredicatePolicyIncompleteAndEmptyStreams/not-run", nil)
	for i := range probes {
		if p := probes[i].PolicyInput; p != nil && p.Literals == nil {
			p.Literals = []string{}
		}
	}
	return probes
}
