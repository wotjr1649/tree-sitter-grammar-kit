package kit

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

func predicateAssets(t *testing.T) (asset, registry []byte) {
	t.Helper()
	read := func(name string) []byte {
		data, err := os.ReadFile("../contracts/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	return read("query-predicate-expected-r1.json"), read("query-semantic-registry-r1.json")
}

func predicateCode(t *testing.T, err error) string {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("not a kit error: %v", err)
	}
	return e.Code
}

func TestPredicateExpectedAssetBindsRegistryAndRoster(t *testing.T) {
	asset, registry := predicateAssets(t)
	if len(asset) != 80893 || queryregistrycore.Digest(asset) != "1f057de4a4dd870f1bddef5b1ef780343b0996d9c473d3b937807d464280b61c" {
		t.Fatalf("sealed expected asset changed: %d/%s", len(asset), queryregistrycore.Digest(asset))
	}
	e, err := ParsePredicateExpected(asset, registry)
	if err != nil || len(e.Tuples) != 20 || e.Registry.SHA256 != "ffd3552dee2d6a9b476b9de76195d2c118bb58de9079ab76dfb1e469d34aa3c8" {
		t.Fatalf("asset: %v %d", err, len(e.Tuples))
	}
	for _, id := range []string{"pattern-isolation", "evaluated-empty"} {
		tuple, _ := e.Tuple(id)
		if id == "evaluated-empty" && (len(tuple.Structural) != 0 || len(tuple.Expected) != 0 || tuple.Expected == nil) {
			t.Fatalf("completed empty stream is not []: %+v", tuple)
		}
		if id == "pattern-isolation" && (len(tuple.Structural) != 2 || len(tuple.Expected) != 1 || tuple.Expected[0].Name != "n") {
			t.Fatalf("pattern isolation: %+v", tuple)
		}
	}
	if tuple, _ := e.Tuple("eq-1"); len(tuple.Structural) != 1 || len(tuple.Expected) != 0 {
		t.Fatal("a rejected match must stay in the structural stream and leave the evaluated one")
	}
}

func TestPredicateExpectedRejectsForgeries(t *testing.T) {
	asset, registry := predicateAssets(t)
	mutate := func(f func(e *PredicateExpected)) []byte {
		var e PredicateExpected
		if err := jsonv2.Unmarshal(asset, &e); err != nil {
			t.Fatal(err)
		}
		f(&e)
		out, err := jsonv2.Marshal(e, jsonv2.FormatNilSliceAsNull(true))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	index := func(e *PredicateExpected, id string) *PredicateExpectedTuple {
		for i := range e.Tuples {
			if e.Tuples[i].ID == id {
				return &e.Tuples[i]
			}
		}
		t.Fatal(id)
		return nil
	}
	for _, tc := range []struct {
		name, code string
		data       []byte
		registry   []byte
	}{
		{"deleted obligation", "PREDICATE_EXPECTED_MEMBERSHIP", mutate(func(e *PredicateExpected) { e.Tuples = e.Tuples[:19] }), registry},
		{"swapped order", "PREDICATE_EXPECTED_MEMBERSHIP", mutate(func(e *PredicateExpected) { e.Tuples[0], e.Tuples[1] = e.Tuples[1], e.Tuples[0] }), registry},
		{"duplicate replaces one", "PREDICATE_EXPECTED_MEMBERSHIP", mutate(func(e *PredicateExpected) { e.Tuples[1] = e.Tuples[0] }), registry},
		{"other source", "PREDICATE_EXPECTED_MEMBERSHIP", mutate(func(e *PredicateExpected) { index(e, "eq-1").Source = "x;\n1;\n" }), registry},
		{"other query", "PREDICATE_EXPECTED_MEMBERSHIP", mutate(func(e *PredicateExpected) { index(e, "eq-0").Query = "(identifier) @x" }), registry},
		{"evaluated null", "PREDICATE_EXPECTED_NULL", mutate(func(e *PredicateExpected) { index(e, "evaluated-empty").Expected = nil }), registry},
		{"structural null", "PREDICATE_EXPECTED_NULL", mutate(func(e *PredicateExpected) { index(e, "eq-0").Structural = nil }), registry},
		{"metadata predicates null", "PREDICATE_EXPECTED_NULL", mutate(func(e *PredicateExpected) { index(e, "eq-0").Metadata.Predicates = nil }), registry},
		{"structural-only relabel", "PREDICATE_EXPECTED_STRUCTURAL_ONLY", mutate(func(e *PredicateExpected) { index(e, "eq-0").Metadata.Predicates = []QueryPredicate{} }), registry},
		{"split match", "PREDICATE_EXPECTED_SUBSEQUENCE", mutate(func(e *PredicateExpected) {
			tuple := index(e, "two-captures-kept")
			tuple.Expected = tuple.Expected[:1]
		}), registry},
		{"unknown capture name", "PREDICATE_EXPECTED_CAPTURE", mutate(func(e *PredicateExpected) { index(e, "eq-0").Expected[0].Name = "z" }), registry},
		{"foreign tree", "PREDICATE_EXPECTED_TREE", mutate(func(e *PredicateExpected) { index(e, "eq-0").Tree[0].EndByte = 5 }), registry},
		{"other schema", "PREDICATE_EXPECTED_SCHEMA", mutate(func(e *PredicateExpected) { e.Schema += "x" }), registry},
		{"unknown member", "PREDICATE_EXPECTED_JSON", bytes.Replace(asset, []byte(`"basis":`), []byte(`"extra":1,"basis":`), 1), registry},
		{"registry changed", "PREDICATE_EXPECTED_REGISTRY_PIN", asset, append(slices.Clone(registry), '\n')},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParsePredicateExpected(tc.data, tc.registry)
			if err == nil || predicateCode(t, err) != tc.code {
				t.Fatalf("accepted or wrong code: %v, want %s", err, tc.code)
			}
		})
	}
}

func predicateHonest(p queryregistrycore.Probe) PredicateControlOutcome {
	o := PredicateControlOutcome{Status: p.Status, Code: p.Code, Evaluation: p.Evaluation, Partial: p.Partial, Captures: "NULL"}
	if p.Expected == "PRESERVED_STRUCTURAL" {
		o.Captures, o.CapturesAreStructural = "PRESENT", true
	}
	if p.Structural == "PRESERVED_4" {
		o.Structural = 4
	}
	return o
}

func TestPredicateControlRowsRejectForgedOutcomes(t *testing.T) {
	rows := 0
	for _, p := range queryregistrycore.Roster() {
		if p.Kind != queryregistrycore.Control {
			continue
		}
		rows++
		if err := CheckPredicateControl(p, predicateHonest(p)); err != nil {
			t.Fatalf("honest %s: %v", p.ID, err)
		}
		forged := map[string]func(o *PredicateControlOutcome){
			"status to pass":  func(o *PredicateControlOutcome) { o.Status = "COMPLETED" },
			"other code":      func(o *PredicateControlOutcome) { o.Code += "X" },
			"evaluated":       func(o *PredicateControlOutcome) { o.Evaluation = "EVALUATED" },
			"partial flipped": func(o *PredicateControlOutcome) { o.Partial = !o.Partial },
			"null to []":      func(o *PredicateControlOutcome) { o.Captures = "EMPTY" },
			"streams adopted": func(o *PredicateControlOutcome) { o.Captures, o.CapturesAreStructural = "PRESENT", true },
			"structural lost": func(o *PredicateControlOutcome) { o.Structural = (o.Structural + 1) % 5 },
		}
		for name, f := range forged {
			o := predicateHonest(p)
			f(&o)
			if name == "streams adopted" && p.Expected == "PRESERVED_STRUCTURAL" {
				continue // the honest outcome of that row already is this value
			}
			if CheckPredicateControl(p, o) == nil {
				t.Errorf("%s: forged %q accepted", p.ID, name)
			}
		}
	}
	if rows != 47 {
		t.Fatalf("control rows = %d", rows)
	}
	if CheckPredicateControl(queryregistrycore.Roster()[0], PredicateControlOutcome{}) == nil {
		t.Fatal("a positive row was checked as a rejection")
	}
}

func predicateJudgments(t *testing.T, registry []byte, platform string) (judgments []queryregistrycore.Judgment, namespace string) {
	t.Helper()
	reg, err := queryregistrycore.Parse(registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range queryregistrycore.Expand(nil, queryregistrycore.Platforms(), reg.Probes, nil) {
		if c.Platform != platform {
			continue
		}
		j := queryregistrycore.Judgment{ID: c.ID(reg.Namespace), Coordinate: c, State: "VERIFIED"}
		if c.Kind == queryregistrycore.Control {
			for _, p := range reg.Probes {
				if p.ID == c.Case {
					j.Subject = &queryregistrycore.Subject{Status: p.Status, Code: p.Code, Evaluation: p.Evaluation, Partial: p.Partial, Structural: p.Structural, Expected: p.Expected}
				}
			}
		}
		judgments = append(judgments, j)
	}
	return judgments, reg.Namespace
}

func TestPredicateScopeFoldWindowsVerifiedLinuxBlocked(t *testing.T) {
	_, registry := predicateAssets(t)
	sha := queryregistrycore.Digest(registry)
	judgments, _ := predicateJudgments(t, registry, "windows-amd64")
	if len(judgments) != 67 {
		t.Fatalf("windows coordinates = %d", len(judgments))
	}
	got, err := FoldPredicateScope(registry, sha, judgments, map[string]bool{"linux-amd64": true})
	want := []PredicateScopeCounter{
		{Kind: "PREDICATE_REQUIRED", Platform: "windows-amd64", Required: 20, Verified: 20},
		{Kind: "PREDICATE_REQUIRED", Platform: "linux-amd64", Required: 20, Blocked: 20},
		{Kind: "EXPECTED_REJECTION", Platform: "windows-amd64", Required: 47, Verified: 47},
		{Kind: "EXPECTED_REJECTION", Platform: "linux-amd64", Required: 47, Blocked: 47},
	}
	if err != nil || got.FullScopeComplete || !slices.Equal(got.Counters, want) {
		t.Fatalf("fold: %+v %v", got, err)
	}
	// A deleted judgment is not hidden: without a blocked host it stays unprocessed.
	got, err = FoldPredicateScope(registry, sha, judgments[1:], nil)
	if err != nil || got.Counters[0].Verified != 19 || got.Counters[0].Unprocessed != 1 || got.Counters[1].Unprocessed != 20 {
		t.Fatalf("deleted judgment: %+v %v", got, err)
	}
}

func TestPredicateScopeFoldRejectsForgeries(t *testing.T) {
	_, registry := predicateAssets(t)
	sha := queryregistrycore.Digest(registry)
	base, _ := predicateJudgments(t, registry, "windows-amd64")
	for _, tc := range []struct {
		name   string
		mutate func(j []queryregistrycore.Judgment) []queryregistrycore.Judgment
		sha    string
	}{
		{"positive with a rejection subject", func(j []queryregistrycore.Judgment) []queryregistrycore.Judgment {
			j[0].Subject = &queryregistrycore.Subject{}
			return j
		}, sha},
		{"rejection row turned into a pass", func(j []queryregistrycore.Judgment) []queryregistrycore.Judgment {
			j[30].Subject = &queryregistrycore.Subject{Status: "COMPLETED", Evaluation: "EVALUATED", Expected: "PRESERVED_STRUCTURAL"}
			return j
		}, sha},
		{"rejection row without subject", func(j []queryregistrycore.Judgment) []queryregistrycore.Judgment {
			j[30].Subject = nil
			return j
		}, sha},
		{"verified with a code", func(j []queryregistrycore.Judgment) []queryregistrycore.Judgment {
			j[0].Code = "X"
			return j
		}, sha},
		{"failed without a code", func(j []queryregistrycore.Judgment) []queryregistrycore.Judgment {
			j[0].State = "FAILED"
			return j
		}, sha},
		{"unknown state", func(j []queryregistrycore.Judgment) []queryregistrycore.Judgment {
			j[0].State = "PASS"
			return j
		}, sha},
		{"duplicate", func(j []queryregistrycore.Judgment) []queryregistrycore.Judgment { return append(j, j[0]) }, sha},
		{"undeclared id", func(j []queryregistrycore.Judgment) []queryregistrycore.Judgment {
			j[0].ID += "0"
			return j
		}, sha},
		{"coordinate swapped", func(j []queryregistrycore.Judgment) []queryregistrycore.Judgment {
			j[0].Coordinate = j[1].Coordinate
			return j
		}, sha},
		{"other registry hash", func(j []queryregistrycore.Judgment) []queryregistrycore.Judgment { return j }, queryregistrycore.Digest(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := FoldPredicateScope(registry, tc.sha, tc.mutate(slices.Clone(base)), nil); err == nil {
				t.Fatal("forged judgment set accepted")
			}
		})
	}
}
