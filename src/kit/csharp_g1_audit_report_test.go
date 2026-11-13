package kit

import (
	"math"
	"strconv"
	"testing"
)

// The exact one-sided 95% binomial upper bound: P(X <= k) at the bound is 0.05, it is the closed form
// 1-0.05^(1/n) with no error, it grows with k, and it is below the rule of three that the sealed r1 asset
// records (so that value stays a safe, conservative one).
func TestCSharpG1AuditUpperIsExact(t *testing.T) {
	for _, n := range []int{1, 4, 5, 16, 20, 25, 26, 100} {
		got := G1AuditUpper95(0, n) / 100
		if want := 1 - math.Pow(0.05, 1/float64(n)); math.Abs(got-want) > 1e-9 {
			t.Errorf("0/%d: %.9f, closed form %.9f", n, got, want)
		}
		if rule := 300 / float64(n); G1AuditUpper95(0, n) >= rule {
			t.Errorf("0/%d: exact %.2f is not below the rule of three %.2f", n, G1AuditUpper95(0, n), rule)
		}
	}
	for _, c := range []struct{ k, n int }{{1, 4}, {1, 26}, {1, 16}, {2, 26}, {3, 26}, {5, 40}} {
		p := G1AuditUpper95(c.k, c.n) / 100
		if cdf := g1BinomCDF(c.k, c.n, p); math.Abs(cdf-0.05) > 1e-9 {
			t.Errorf("%d/%d: P(X<=k) at the bound is %.9f, want 0.05", c.k, c.n, cdf)
		}
		if G1AuditUpper95(c.k, c.n) <= G1AuditUpper95(c.k-1, c.n) {
			t.Errorf("%d/%d: the bound does not grow with k", c.k, c.n)
		}
		// a point estimate k/n is not an upper bound: the bound is above it
		if G1AuditUpper95(c.k, c.n) <= 100*float64(c.k)/float64(c.n) {
			t.Errorf("%d/%d: bound %.2f is not above the point estimate", c.k, c.n, G1AuditUpper95(c.k, c.n))
		}
	}
	// values the documents quote
	for _, c := range []struct {
		k, n int
		want string
	}{{0, 26, "10.9"}, {0, 20, "13.9"}, {0, 16, "17.1"}, {0, 4, "52.7"}, {0, 5, "45.1"}, {1, 16, "26.4"}, {1, 20, "21.6"}, {1, 26, "17.0"}} {
		if got := strconv.FormatFloat(G1AuditUpper95(c.k, c.n), 'f', 1, 64); got != c.want {
			t.Errorf("%d/%d: %s, want %s", c.k, c.n, got, c.want)
		}
	}
	if G1AuditUpper95(3, 3) != 100 || G1AuditUpper95(0, 0) != 100 {
		t.Error("no information must give 100")
	}
}

// The audit's own estimate, with mismatches, must be the exact bound, not the point estimate (the r1 bug).
func TestCSharpG1AuditEstimateWithMismatches(t *testing.T) {
	a := g1ExpLoad(t)
	_, ctx, _, err := g1ExpContext(a.in)
	if err != nil {
		t.Fatal(err)
	}
	recs := mustRuleSrcs(t, a.in)
	prod := map[string]string{}
	for _, r := range recs {
		prod[r.ID] = r.Production
	}
	id := "csharp-B01.a002"
	old := csharpG1AuditHand[id]
	e := old
	e.TS = []string{"block"}
	csharpG1AuditHand[id] = e
	au, err := g1Audit(ctx, a.want.Alternatives, prod)
	csharpG1AuditHand[id] = old
	if err != nil {
		t.Fatal(err)
	}
	if au.Estimate.Mismatches != 1 || au.Estimate.Samples != 26 || au.Estimate.Upper95 != "17.0" {
		t.Errorf("1 mismatch in 26: %+v, want upper bound 17.0 (point estimate 3.8)", au.Estimate)
	}
}

// The strata of the tracked audit: 16 spec-derived, 4 differential-oracle, 5 blocked, 1 coverage gap.
// The exact bounds replace the one 3/26 figure, and a335, matched by the audit and wrong in r1, shows
// in the spec-derived stratum as a later-wrong sample.
func TestCSharpG1AuditStrata(t *testing.T) {
	r2, err := DecodeCSharpG1Expected(mustRead(t, "../contracts/csharp-g1-expected-b01b02-r2.json"))
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		samples, mismatches, later int
		upper, upperLater          string
	}
	want := map[string]row{
		G1StratumSpecDerived:  {16, 0, 1, "17.1", "26.4"},
		G1StratumDifferential: {4, 0, 0, "52.7", "52.7"},
		G1StratumBlocked:      {5, 0, 0, "45.1", "45.1"},
		G1StratumNoWitness:    {1, 0, 0, "", ""},
		"expected values compared (spec-derived and differential-oracle)": {20, 0, 1, "13.9", "21.6"},
		"all samples": {26, 0, 1, "10.9", "17.0"},
	}
	got := G1AuditStrata(r2, nil)
	if len(got) != len(want) {
		t.Fatalf("%d rows: %+v", len(got), got)
	}
	for _, g := range got {
		w, ok := want[g.Stratum]
		if !ok || g.Samples != w.samples || g.Mismatches != w.mismatches || g.LaterWrong != w.later || g.Upper95 != w.upper || g.Upper95Later != w.upperLater {
			t.Errorf("%s: %+v, want %+v", g.Stratum, g, w)
		}
	}
	// the sealed r1 asset has no correction: nothing is later wrong there
	r1, err := DecodeCSharpG1Expected(mustRead(t, "../contracts/csharp-g1-expected-b01b02-r1.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range G1AuditStrata(r1, nil) {
		if g.LaterWrong != 0 {
			t.Errorf("r1 %s: later wrong %d", g.Stratum, g.LaterWrong)
		}
	}
	// a mismatch moves the bound of its own stratum only
	m := r2
	m.Audit.Samples = append([]G1ExpAuditSample(nil), r2.Audit.Samples...)
	for i := range m.Audit.Samples {
		if m.Audit.Samples[i].ID == "csharp-B02.a045" { // a differential-oracle sample
			m.Audit.Samples[i].Outcome = "MISMATCH"
		}
	}
	for _, g := range G1AuditStrata(m, nil) {
		switch g.Stratum {
		case G1StratumDifferential:
			if g.Mismatches != 1 || g.Upper95 != strconv.FormatFloat(G1AuditUpper95(1, 4), 'f', 1, 64) {
				t.Errorf("differential stratum after a mismatch: %+v", g)
			}
		case G1StratumSpecDerived:
			if g.Mismatches != 0 || g.Upper95 != "17.1" {
				t.Errorf("spec-derived stratum moved: %+v", g)
			}
		}
	}
}
