package kit

import "fmt"

// Report of the specification sample audit by stratum (#233, R-4). The audit asset (r1, sealed) stores one
// estimate for all samples and, with no mismatch, the rule-of-three value 300/n. That value is an
// approximation that is larger than the exact one-sided 95% binomial (Clopper-Pearson) upper bound, so it
// is conservative; it stays in the sealed bytes. This file computes the exact bound and the strata, and
// changes no asset. A sample audit measures how often a hand derivation from the specification agrees
// with the expectation; it is not an error rate of Roslyn or of the expected asset as a whole, and a
// stratum of 4 samples bounds almost nothing.

// G1AuditUpper95 is the exact one-sided 95% upper bound, in percent, of the error rate p when k errors
// were seen in n samples: the p at which P(X <= k) = 0.05 for X ~ Binomial(n, p). It uses plain float
// arithmetic (no math package, so the derivation closure stays free of it) and is exact to 1e-12.
func G1AuditUpper95(k, n int) float64 {
	if n <= 0 || k < 0 {
		return 100
	}
	if k >= n {
		return 100
	}
	lo, hi := 0.0, 1.0
	for i := 0; i < 80; i++ {
		p := (lo + hi) / 2
		if g1BinomCDF(k, n, p) > 0.05 {
			lo = p
		} else {
			hi = p
		}
	}
	return 100 * hi
}

// g1BinomCDF is P(X <= k) for X ~ Binomial(n, p), summed term by term (n is the size of an audit sample).
func g1BinomCDF(k, n int, p float64) float64 {
	q := 1 - p
	term := 1.0
	for i := 0; i < n; i++ {
		term *= q
	}
	sum := term
	for i := 0; i < k; i++ {
		term *= float64(n-i) / float64(i+1) * p / q
		sum += term
	}
	return sum
}

// the strata of the audit samples
const (
	G1StratumSpecDerived  = "spec-derived"        // EXPECTED, basis spec-derived: the lexeme comes from the specification, Roslyn only locates it
	G1StratumDifferential = "differential-oracle" // EXPECTED, basis differential-oracle: the structure is what Roslyn observed
	G1StratumBlocked      = "blocked"             // the hand derivation says BLOCKED with a reason code
	G1StratumNoWitness    = "coverage-gap"        // the specification derives an expectation, no linked source exhibits it: nothing was compared
)

// G1AuditStratum is one stratum of the audit: its samples, its mismatches and the exact upper bound.
// Compared is false for the coverage gap: no value was compared there, so no bound is given.
type G1AuditStratum struct {
	Stratum    string `json:"stratum"`
	Samples    int    `json:"samples"`
	Mismatches int    `json:"mismatches"`
	Compared   bool   `json:"compared"`
	Upper95    string `json:"upper_95_percent,omitempty"` // exact one-sided binomial bound, percent with one decimal
	// LaterWrong counts the samples whose expected value a later correction made after the native result (r2, after_native) changed although
	// the audit had matched it; Upper95Later is the exact bound with those counted as errors too.
	LaterWrong   int    `json:"later_wrong"`
	Upper95Later string `json:"upper_95_percent_with_later_wrong,omitempty"`
}

// G1AuditStrata groups the samples of the audit of x by stratum and gives each stratum's exact upper
// bound, then two totals: the expected values that were compared with a hand derivation and all samples.
// The audit's own estimate is the all-samples row computed exactly.
func G1AuditStrata(x CSharpG1Expected) []G1AuditStratum {
	basis := map[string]string{}
	for _, a := range x.Alternatives {
		basis[a.ID] = a.Basis
	}
	order := []string{G1StratumSpecDerived, G1StratumDifferential, G1StratumBlocked, G1StratumNoWitness}
	byName := map[string]*G1AuditStratum{}
	var out []G1AuditStratum
	for _, name := range order {
		out = append(out, G1AuditStratum{Stratum: name, Compared: name != G1StratumNoWitness})
	}
	for i := range out {
		byName[out[i].Stratum] = &out[i]
	}
	corrected := map[string]bool{}
	for _, c := range x.Corrections {
		if c.AfterNative && c.Field == G1FieldTS {
			corrected[c.Item] = true
		}
	}
	for _, s := range x.Audit.Samples {
		name := G1StratumSpecDerived
		switch {
		case s.Outcome == "COVERAGE_GAP":
			name = G1StratumNoWitness
		case s.Hand.Status == G1ExpBlocked:
			name = G1StratumBlocked
		case basis[s.ID] == G1BasisDiff:
			name = G1StratumDifferential
		}
		st := byName[name]
		st.Samples++
		if s.Outcome == "MISMATCH" {
			st.Mismatches++
		} else if corrected[s.ID] {
			st.LaterWrong++
		}
	}
	values := G1AuditStratum{Stratum: "expected values compared (spec-derived and differential-oracle)", Compared: true}
	all := G1AuditStratum{Stratum: "all samples", Compared: true}
	for i := range out {
		if out[i].Compared && out[i].Samples > 0 {
			out[i].Upper95 = fmt.Sprintf("%.1f", G1AuditUpper95(out[i].Mismatches, out[i].Samples))
		}
		if out[i].Stratum == G1StratumSpecDerived || out[i].Stratum == G1StratumDifferential {
			values.Samples += out[i].Samples
			values.Mismatches += out[i].Mismatches
			values.LaterWrong += out[i].LaterWrong
		}
		all.Samples += out[i].Samples
		all.Mismatches += out[i].Mismatches
		all.LaterWrong += out[i].LaterWrong
	}
	for i := range out {
		if out[i].Compared && out[i].Samples > 0 {
			out[i].Upper95Later = fmt.Sprintf("%.1f", G1AuditUpper95(out[i].Mismatches+out[i].LaterWrong, out[i].Samples))
		}
	}
	values.Upper95Later = fmt.Sprintf("%.1f", G1AuditUpper95(values.Mismatches+values.LaterWrong, values.Samples))
	all.Upper95Later = fmt.Sprintf("%.1f", G1AuditUpper95(all.Mismatches+all.LaterWrong, all.Samples))
	values.Upper95 = fmt.Sprintf("%.1f", G1AuditUpper95(values.Mismatches, values.Samples))
	all.Upper95 = fmt.Sprintf("%.1f", G1AuditUpper95(all.Mismatches, all.Samples))
	return append(out, values, all)
}
