package native

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

func predicateScopeAssets(t *testing.T) (asset, registry []byte, expected kit.PredicateExpected) {
	t.Helper()
	read := func(name string) []byte {
		data, err := os.ReadFile("../../contracts/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	asset, registry = read("query-predicate-expected-r1.json"), read("query-semantic-registry-r1.json")
	expected, err := kit.ParsePredicateExpected(asset, registry)
	if err != nil {
		t.Fatal(err)
	}
	return asset, registry, expected
}

// runPredicateTuple runs the owned plain grammar on a tuple's registered source and query and
// returns the raw response and case projection, as a stored witness would.
func runPredicateTuple(t *testing.T, b *Build, tuple kit.PredicateExpectedTuple) (raw, projection []byte) {
	t.Helper()
	registration, source := tuple.Registration("")
	c := kit.IncrementalCase{ID: "oracle", Encoding: kit.EncodingUTF8, Input: registration.Input, Edits: []kit.Edit{}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	result := b.RunCase(ctx, oracleContext(false, tuple.Query), c, source)
	projection, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return result.Raw, projection
}

func predicateHost() (id, subject string) {
	subject = runtime.GOOS + "/" + runtime.GOARCH
	return runtime.GOOS + "-" + runtime.GOARCH, subject
}

// otherQueryHost is the other registered host of the two-host query denominators; ok is false on a
// host the registry does not judge (no claim is made there).
func otherQueryHost(platform string) (other string, ok bool) {
	hosts := queryregistrycore.Platforms()
	for i, h := range hosts {
		if h.ID == platform && len(hosts) == 2 {
			return hosts[1-i].ID, true
		}
	}
	return "", false
}

// queryHostPlan is the one rule the two-host query tests share (#362): a host the registry does not
// judge skips, and the fan-in (collectOnly) runs on windows-amd64 as it always did but on another
// registered host only while the opt-in collection (TSGK_QUERY_SCOPE_OUT) asks for its judgments, so
// the required sanitizer and foundation native runs on Linux do not gain its time or its failures.
func queryHostPlan(platform string, collecting, collectOnly bool) (other, skip string) {
	other, ok := otherQueryHost(platform)
	switch {
	case !ok:
		return "", "the registry judges windows-amd64 and linux-amd64 only; " + platform + " is not registered"
	case collectOnly && platform != "windows-amd64" && !collecting:
		return "", "on " + platform + " this test runs only when " + kit.QueryScopeHostEnv + " collects its judgments"
	}
	return other, ""
}

// queryHostOrSkip applies queryHostPlan to this host.
func queryHostOrSkip(t *testing.T, collectOnly bool) (platform, subject, other string) {
	t.Helper()
	platform, subject = predicateHost()
	other, skip := queryHostPlan(platform, os.Getenv(kit.QueryScopeHostEnv) != "", collectOnly)
	if skip != "" {
		t.Skip(skip)
	}
	return platform, subject, other
}

// The 20 EVALUATED tuples run against the owned native with the shared witness verifier. The
// expected file was sealed before the first native run and holds no native output.
func TestNativePredicateScopeEvaluatedTuples(t *testing.T) {
	platform, subject, other := queryHostOrSkip(t, false)
	b := fixtureBuild(t, "plain")
	_, registry, expected := predicateScopeAssets(t)
	reg, err := queryregistrycore.Parse(registry)
	if err != nil {
		t.Fatal(err)
	}
	verified := map[string]bool{}
	for _, tuple := range expected.Tuples {
		t.Run(tuple.ID, func(t *testing.T) {
			raw, projection := runPredicateTuple(t, b, tuple)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			got, err := kit.VerifyPredicateTuple(ctx, tuple, subject, raw, projection)
			if err != nil || got.Assessment != kit.AssessPass || got.Required != 1 || got.Verified != 1 || got.Evaluated != 1 {
				t.Fatalf("%s: %+v %v", tuple.ID, got, err)
			}
			verified[tuple.ID] = true
		})
	}
	if len(verified) != 20 {
		t.Fatalf("verified tuples = %d", len(verified))
	}

	// The 47 host-policy rows come from the same registry, run through the host evaluator.
	controls := map[string]bool{}
	for _, p := range reg.Probes {
		if p.Kind != queryregistrycore.Control {
			continue
		}
		if err := kit.CheckPredicateControl(p, predicateControlOutcome(t, p)); err != nil {
			t.Errorf("%v", err)
			continue
		}
		controls[p.ID] = true
	}
	if len(controls) != 47 {
		t.Fatalf("verified rejection rows = %d", len(controls))
	}

	// Fold this host's adjudicated rows; the other registered host stays BLOCKED (not run).
	var judgments []queryregistrycore.Judgment
	for _, c := range queryregistrycore.Expand(nil, queryregistrycore.Platforms(), reg.Probes, nil) {
		if c.Platform != platform || !verified[c.Case] && !controls[c.Case] {
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
	coverage, err := kit.FoldPredicateScope(registry, queryregistrycore.Digest(registry), judgments, map[string]bool{other: true})
	if err != nil || coverage.FullScopeComplete {
		t.Fatalf("fold: %+v %v", coverage, err)
	}
	for _, c := range coverage.Counters {
		want := kit.PredicateScopeCounter{Kind: c.Kind, Platform: c.Platform, Required: c.Required}
		if c.Platform == platform {
			want.Verified = c.Required
		} else {
			want.Blocked = c.Required
		}
		if c != want || (c.Kind == queryregistrycore.Predicate) != (c.Required == 20) || (c.Kind == queryregistrycore.Control) != (c.Required == 47) {
			t.Fatalf("counter %+v, want %+v", c, want)
		}
	}
	t.Logf("scope coverage: %+v", coverage.Counters)
}

// predicateControlOutcome runs one registered rejection row through the host evaluator.
func predicateControlOutcome(t *testing.T, p queryregistrycore.Probe) kit.PredicateControlOutcome {
	t.Helper()
	var out QueryOut
	if p.ID == "not-run" {
		out = queryOuts(Tree{}, []QuerySource{{ID: "q"}}, p.Encoding, nil)[0]
	} else {
		var predicates []WirePredicate
		in := p.PolicyInput
		names := []string{"x", "y"}
		if in != nil {
			args := []PredStep{policyCapture(in.LeftCapture, in.LeftQuantifier)}
			if in.RightCapture != "" {
				args = append(args, policyCapture(in.RightCapture, in.RightQuantifier))
			}
			for _, literal := range in.Literals {
				args = append(args, policyString(literal))
			}
			predicates = []WirePredicate{policyPredicate(in.Operator, args...)}
			for _, name := range []string{in.LeftCapture, in.RightCapture} {
				if name != "" && !slices.Contains(names, name) {
					names = append(names, name)
				}
			}
		}
		q := policyWire(predicates...)
		q.CaptureNames = names
		if p.Status == kit.StatusResourceLimit {
			q.Status, q.Code, q.Partial = kit.StatusResourceLimit, p.Code, true
		}
		out = policyOut(t, q, p.Encoding)
	}
	o := kit.PredicateControlOutcome{Status: out.Status, Code: out.Code, Evaluation: out.Evaluation, Partial: out.Partial, Structural: len(out.structural)}
	switch {
	case out.Captures == nil:
		o.Captures = "NULL"
	case len(out.Captures) == 0:
		o.Captures = "EMPTY"
	default:
		o.Captures = "PRESENT"
		o.CapturesAreStructural = slices.Equal(out.Captures, out.structural)
	}
	return o
}

// Forged evidence must not become VERIFIED: EVALUATED called structural, a structural query
// called a predicate tuple, a forged expectation, null turned into [], a forged CST.
func TestNativePredicateScopeForgeries(t *testing.T) {
	b := fixtureBuild(t, "plain")
	_, _, expected := predicateScopeAssets(t)
	_, subject := predicateHost()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	tuple := func(id string) kit.PredicateExpectedTuple {
		tp, ok := expected.Tuple(id)
		if !ok {
			t.Fatal(id)
		}
		// deep copy: the forged variants must not alias the sealed values
		var copied kit.PredicateExpectedTuple
		data, _ := json.Marshal(tp)
		if err := json.Unmarshal(data, &copied); err != nil {
			t.Fatal(err)
		}
		return copied
	}
	verify := func(tp kit.PredicateExpectedTuple, raw, projection []byte) kit.QueryWitnessResult {
		got, err := kit.VerifyPredicateTuple(ctx, tp, subject, raw, projection)
		if err != nil {
			t.Fatalf("%s: %v", tp.ID, err)
		}
		return got
	}
	notVerified := func(name string, got kit.QueryWitnessResult) {
		t.Helper()
		if got.Assessment == kit.AssessPass {
			t.Errorf("%s: forged evidence VERIFIED: %+v", name, got)
		}
	}
	for _, id := range []string{"eq-0", "eq-1", "two-captures-rejected", "pattern-isolation", "evaluated-empty"} {
		base := tuple(id)
		raw, projection := runPredicateTuple(t, b, base)
		if got := verify(base, raw, projection); got.Assessment != kit.AssessPass {
			t.Fatalf("honest %s: %+v", id, got)
		}
		// EVALUATED projection relabeled STRUCTURAL
		relabeled := bytes.ReplaceAll(projection, []byte(`"evaluation":"EVALUATED"`), []byte(`"evaluation":"STRUCTURAL"`))
		if bytes.Equal(relabeled, projection) {
			t.Fatalf("%s: projection has no EVALUATED marker", id)
		}
		notVerified(id+" EVALUATED->STRUCTURAL", verify(base, raw, relabeled))
		// registered as structural-only metadata
		structuralOnly := tuple(id)
		structuralOnly.Metadata.Predicates = []kit.QueryPredicate{}
		notVerified(id+" predicates registered empty", verify(structuralOnly, raw, projection))
		// forged CST
		forged := tuple(id)
		forged.Tree[len(forged.Tree)-1].Type = "number"
		notVerified(id+" forged CST", verify(forged, raw, projection))
		// forged evaluated expectation: the opposite verdict, and null turned into []
		flipped := tuple(id)
		if len(flipped.Expected) == len(flipped.Structural) {
			flipped.Expected = []kit.Capture{}
		} else {
			flipped.Expected = flipped.Structural
		}
		if id != "evaluated-empty" { // the empty-source tuple has no opposite verdict
			notVerified(id+" forged evaluated stream", verify(flipped, raw, projection))
		}
		nulled := tuple(id)
		nulled.Expected = nil
		notVerified(id+" evaluated expectation unavailable", verify(nulled, raw, projection))
		// structural stream emptied
		if len(base.Structural) != 0 {
			emptied := tuple(id)
			emptied.Structural = []kit.Capture{}
			notVerified(id+" structural null->[]", verify(emptied, raw, projection))
		}
	}
	// A real structural-only query is a STRUCTURAL_ONLY result and is never a predicate tuple.
	structural := tuple("eq-0")
	structural.Query, structural.Metadata.Predicates = "((identifier) @x)", []kit.QueryPredicate{}
	structural.Expected = structural.Structural
	raw, projection := runPredicateTuple(t, b, structural)
	got := verify(structural, raw, projection)
	if got.Assessment != kit.AssessFail || !strings.Contains(findingCodes(got), "PREDICATE_TUPLE_NOT_EVALUATED") {
		t.Errorf("structural-only query counted as an EVALUATED predicate tuple: %+v", got)
	}
	// a stored run of another tuple's source cannot stand for this tuple
	a, c := tuple("eq-0"), tuple("eq-1")
	rawA, projectionA := runPredicateTuple(t, b, a)
	notVerified("source of another tuple", verify(c, rawA, projectionA))
}

func findingCodes(r kit.QueryWitnessResult) string {
	var codes []string
	for _, f := range r.Findings {
		codes = append(codes, f.Code)
	}
	return strings.Join(codes, ",")
}
