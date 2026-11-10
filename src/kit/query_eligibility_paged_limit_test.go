package kit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func pagedLimitCharge(t *testing.T, b *eligibilityBudget, logical uint64) *Error {
	t.Helper()
	return b.charge(0, logical, "paged-limit-test")
}

// The cap is a fixed, per-ledger constant: ordinary ledgers stay at 1 GiB and only an explicitly selected ledger reaches 1.5 GiB.
func TestEligibilityPagedLogicalLimitBoundary(t *testing.T) {
	if eligibilityLogicalLimit != 1<<30 || eligibilityPagedLogicalLimit != 1610612736 || !eligibilityPagedContext(semanticPagedContext) || !eligibilityPagedContext(semanticPagedBinariesContext) || eligibilityPagedContext("full-input-utf8-paged-binaries/r2") || eligibilityPagedContext("full-input-utf8/r2") || eligibilityPagedContext("") {
		t.Fatal("limit constants or approved-context predicate changed")
	}
	for _, tc := range []struct {
		name      string
		limit     uint64
		cap       uint64
		okAtCap   bool
		failAbove bool
	}{{"ordinary", 0, eligibilityLogicalLimit, true, true}, {"paged", eligibilityPagedLogicalLimit, eligibilityPagedLogicalLimit, true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			r, e := startRun(ctxT(t), 30*time.Second)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			b := &eligibilityBudget{r: r, logicalLimit: tc.limit}
			if b.logicalCap() != tc.cap {
				t.Fatalf("cap %d", b.logicalCap())
			}
			if e := pagedLimitCharge(t, b, tc.cap); e != nil || b.logical != tc.cap {
				t.Fatalf("exact cap rejected: %v %d", e, b.logical)
			}
			if e := pagedLimitCharge(t, b, 1); e == nil || e.Kind != KindResourceLimit || e.Code != "ELIGIBILITY_WORK_LIMIT" || b.logical != tc.cap {
				t.Fatalf("cap+1 accepted: %v %d", e, b.logical)
			}
		})
	}
	// Just above 1 GiB: fails in an ordinary ledger, passes in an approved paged ledger; just above 1.5 GiB fails there.
	r, e := startRun(ctxT(t), 30*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	defer r.cancel()
	ordinary, paged := &eligibilityBudget{r: r}, &eligibilityBudget{r: r, logicalLimit: eligibilityPagedLogicalLimit}
	if e := pagedLimitCharge(t, ordinary, eligibilityLogicalLimit+1); e == nil || e.Code != "ELIGIBILITY_WORK_LIMIT" {
		t.Fatalf("ordinary ledger accepted 1 GiB+1: %v", e)
	}
	if e := pagedLimitCharge(t, paged, eligibilityLogicalLimit+1); e != nil {
		t.Fatalf("paged ledger rejected 1 GiB+1: %v", e)
	}
	if e := pagedLimitCharge(t, paged, eligibilityPagedLogicalLimit-eligibilityLogicalLimit); e == nil || e.Code != "ELIGIBILITY_WORK_LIMIT" {
		t.Fatalf("paged ledger accepted 1.5 GiB+1: %v", e)
	}
}

// Carries PR #326 review S-1: the seven PostgreSQL9 parser parts cost 2*(bytes+1) each in the part-hash ledger, which fits only the paged cap.
func TestEligibilityPagedLogicalLimitPostgreSQL9PartArithmetic(t *testing.T) {
	parts := []uint64{16777216, 16777216, 16777216, 16777216, 16777216, 16777216, 3622435}
	var bytes, charge uint64
	for _, n := range parts {
		bytes += n
		charge += 2 * (n + 1)
	}
	const observedPreHashBound = 1056664398 // ledger at the recorded failure; part hashes were not yet (all) charged
	if bytes != 104285731 || charge != 208571476 {
		t.Fatalf("parts %d charge %d", bytes, charge)
	}
	if observedPreHashBound+charge <= eligibilityLogicalLimit || observedPreHashBound+charge > eligibilityPagedLogicalLimit {
		t.Fatalf("sum %d outside (1 GiB, 1.5 GiB]", observedPreHashBound+charge)
	}
	r, e := startRun(ctxT(t), 30*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	defer r.cancel()
	for name, limit := range map[string]uint64{"ordinary": 0, "paged": eligibilityPagedLogicalLimit} {
		b := &eligibilityBudget{r: r, logical: observedPreHashBound, logicalLimit: limit}
		var failed *Error
		for _, n := range parts {
			if failed = b.charge(1, 2*(n+1), "paged-limit-part"); failed != nil {
				break
			}
		}
		if (name == "ordinary") != (failed != nil) {
			t.Fatalf("%s: part ledger result %v", name, failed)
		}
	}
}

func pagedLimitCap(t *testing.T, mutate func(*eligibilityFixture), edit func(*QueryReferenceEligibilityRequest), preset *eligibilityBudget) uint64 {
	t.Helper()
	f := newEligibilityFixture(t)
	if mutate != nil {
		mutate(f)
	}
	req := f.request()
	if edit != nil {
		edit(&req)
	}
	r, e := startRun(ctxT(t), 30*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	defer r.cancel()
	if preset != nil {
		preset.r = r
		r.eligibility = preset
	}
	scope := &eligibilityScope{r: r}
	if _, err := verifyQueryReferenceEligibilityInRun(r.caller, req, scope); err != nil && preset == nil {
		t.Logf("core returned %v", err)
	}
	if scope.session == nil || r.eligibility == nil {
		t.Fatal("verify did not reach a session")
	}
	return r.eligibility.logicalCap()
}

// The selection table: only an owning, approval-bound, approved paged-parser request is raised, and only once.
func TestEligibilityPagedLogicalLimitSelection(t *testing.T) {
	t.Parallel()
	paged := func(f *eligibilityFixture) { f.reg.Context = semanticPagedContext }
	binaries := func(f *eligibilityFixture) { f.reg.Context = semanticPagedBinariesContext }
	for _, tc := range []struct {
		name   string
		mutate func(*eligibilityFixture)
		edit   func(*QueryReferenceEligibilityRequest)
		preset *eligibilityBudget
		want   uint64
	}{
		{"ordinary route owner bound", nil, nil, nil, eligibilityLogicalLimit},
		{"paged owner bound", paged, nil, nil, eligibilityPagedLogicalLimit},
		{"paged owner unbound", paged, func(r *QueryReferenceEligibilityRequest) { r.Registration = append(r.Registration, '\n') }, nil, eligibilityLogicalLimit},
		{"paged inherited ledger", paged, nil, &eligibilityBudget{}, eligibilityLogicalLimit},
		{"ordinary inherited ledger", nil, nil, &eligibilityBudget{}, eligibilityLogicalLimit},
		{"binaries owner bound", binaries, nil, nil, eligibilityPagedLogicalLimit},
		{"binaries owner unbound", binaries, func(r *QueryReferenceEligibilityRequest) { r.Registration = append(r.Registration, '\n') }, nil, eligibilityLogicalLimit},
		{"binaries inherited ledger", binaries, nil, &eligibilityBudget{}, eligibilityLogicalLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pagedLimitCap(t, tc.mutate, tc.edit, tc.preset); got != tc.want {
				t.Fatalf("cap %d want %d", got, tc.want)
			}
		})
	}
	// An inherited ledger is not rewritten, even when it is already partly charged.
	b := &eligibilityBudget{logical: 7}
	pagedLimitCap(t, paged, nil, b)
	if b.logicalLimit != 0 {
		t.Fatalf("inherited ledger cap rewritten to %d", b.logicalLimit)
	}
}

// Structural pins: the cap has exactly one assignment site in product code, and it sits after validation and before any evidence read.
func TestEligibilityPagedLogicalLimitSelectionSite(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatal("no sources", err)
	}
	sites := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		sites += strings.Count(string(data), ".logicalLimit =") + strings.Count(string(data), "logicalLimit:")
	}
	if sites != 1 {
		t.Fatalf("logicalLimit assigned at %d product sites", sites)
	}
	data, err := os.ReadFile("query_eligibility_verify.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	validate, set, evidence := strings.Index(src, "s.validate(req)"), strings.Index(src, "b.logicalLimit = eligibilityPagedLogicalLimit"), strings.Index(src, "verifyEvidenceInRun(ctx, req.Evidence")
	if validate < 0 || set < 0 || evidence < 0 || !(validate < set && set < evidence) {
		t.Fatalf("selection order validate=%d set=%d evidence=%d", validate, set, evidence)
	}
	if !strings.Contains(src, "if owner && s.bound && eligibilityPagedContext(s.reg.Context) {") {
		t.Fatal("selection condition changed")
	}
}
