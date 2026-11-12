package kit

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOrdinaryPagingCommitmentBytesAreCharged(t *testing.T) {
	pin := OrdinaryPin{Path: "caller/input.json", Bytes: 1, SHA256: digestHex([]byte("input"))}
	binding := ordinaryEligibilityBinding{Schema: ordinaryEligibilityBindingSchema, Context: OrdinaryContext, ClosureEncoding: "binary-paged",
		Profile: pin, PlanningProfile: pin, Registration: pin, Assets: pin, Plan: pin, Commitment: pin, ReferenceSelection: pin, RunBinding: pin,
		Candidate: "candidate", SemanticCohort: "cohort", RunID: "run", ShardIDs: []string{"shard"}}
	measure := func(binding ordinaryEligibilityBinding) (uint64, uint64) {
		budget := ordinaryTestBudget(t)
		s := &eligibilitySession{b: budget, res: &QueryReferenceEligibilityResult{}, ordinary: &ordinaryEligibilityScope{Binding: binding}, approval: eligibilityApproval{Ordinary: &binding}}
		if _, e := s.pagingBinding(); e != nil {
			t.Fatal(e)
		}
		return budget.work, budget.logical
	}
	w, l := measure(binding)
	binding.Commitment.Path += strings.Repeat("x", 4096)
	w2, l2 := measure(binding)
	if w2 < w+4096 || l2 < l+4096 {
		t.Fatal("commitment pin bytes were hashed without being charged")
	}
}

func TestOrdinaryEligibilityBindingPinsContextAndShard(t *testing.T) {
	pin := OrdinaryPin{Path: "caller/profile.json", Bytes: 7, SHA256: digestHex([]byte("profile"))}
	b := ordinaryEligibilityBinding{
		Schema: ordinaryEligibilityBindingSchema, Context: OrdinaryContext, ClosureEncoding: "parser-paged",
		Profile: pin, PlanningProfile: pin, Registration: pin, Assets: pin, Plan: pin, Commitment: pin, ReferenceSelection: pin, RunBinding: pin,
		Candidate: "candidate", SemanticCohort: "cohort", RunID: "run", ShardIDs: []string{"shard-1", "shard-2"},
	}
	if !ordinaryEligibilityBindingMatches(b, b) {
		t.Fatal("complete ordinary approval binding rejected")
	}
	for name, mutate := range map[string]func(*ordinaryEligibilityBinding){
		"context":            func(x *ordinaryEligibilityBinding) { x.Context = "full-input-utf8/r2" },
		"closure":            func(x *ordinaryEligibilityBinding) { x.ClosureEncoding = "unspecified" },
		"rehash-target":      func(x *ordinaryEligibilityBinding) { x.Profile.SHA256 = digestHex([]byte("replacement")) },
		"missing-commitment": func(x *ordinaryEligibilityBinding) { x.Commitment = OrdinaryPin{} },
		"commitment-pin":     func(x *ordinaryEligibilityBinding) { x.Commitment.SHA256 = digestHex([]byte("other")) },
		"missing-run-pin":    func(x *ordinaryEligibilityBinding) { x.RunBinding = OrdinaryPin{} },
		"missing-references": func(x *ordinaryEligibilityBinding) { x.ReferenceSelection = OrdinaryPin{} },
		"duplicate-shard":    func(x *ordinaryEligibilityBinding) { x.ShardIDs[1] = x.ShardIDs[0] },
		"empty-shards":       func(x *ordinaryEligibilityBinding) { x.ShardIDs = nil },
	} {
		t.Run(name, func(t *testing.T) {
			copy := b
			copy.ShardIDs = append([]string(nil), b.ShardIDs...)
			mutate(&copy)
			if ordinaryEligibilityBindingMatches(copy, b) {
				t.Fatal("invalid ordinary approval binding accepted")
			}
		})
	}
}

func TestOrdinaryVerifiedPagingKeepsSmallEligibilityCap(t *testing.T) {
	for _, tc := range []struct {
		encoding string
		paging   bool
	}{{"plain", false}, {"parser-paged", true}, {"binary-paged", true}} {
		t.Run(tc.encoding, func(t *testing.T) {
			s := &ordinaryEligibilityScope{ClosureEncoding: tc.encoding}
			if ordinaryPagingEnabled(s) != tc.paging || eligibilityPagedContext(OrdinaryContext) {
				t.Fatalf("encoding %q changed legacy cap or part authority", tc.encoding)
			}
		})
	}
}

func TestOrdinaryPagingSelectionKeepsBatchOperationMaximum(t *testing.T) {
	pin := OrdinaryPin{Path: "caller/profile.json", Bytes: 7, SHA256: digestHex([]byte("profile"))}
	b := ordinaryEligibilityBinding{Schema: ordinaryEligibilityBindingSchema, Context: OrdinaryContext, ClosureEncoding: "plain",
		Profile: pin, PlanningProfile: pin, Registration: pin, Assets: pin, Plan: pin, Commitment: pin, ReferenceSelection: pin, RunBinding: pin,
		Candidate: "candidate", SemanticCohort: "cohort", RunID: "run", ShardIDs: make([]string, NativeOperations()["native-query-batch"].MaxCases)}
	for i := range b.ShardIDs {
		b.ShardIDs[i] = "shard-" + strconv.Itoa(i)
	}
	if !validOrdinaryEligibilityBinding(b) {
		t.Fatal("full native-query-batch case maximum was reduced")
	}
	b.ShardIDs = append(b.ShardIDs, "one-too-many")
	if validOrdinaryEligibilityBinding(b) {
		t.Fatal("selected shards exceeded native-query-batch operation maximum")
	}
}

func TestOrdinaryEligibilityCannotUseLegacyUnboundPath(t *testing.T) {
	pin := OrdinaryPin{Path: "caller/profile.json", Bytes: 7, SHA256: digestHex([]byte("profile"))}
	binding := ordinaryEligibilityBinding{
		Schema: ordinaryEligibilityBindingSchema, Context: OrdinaryContext, ClosureEncoding: "binary-paged",
		Profile: pin, PlanningProfile: pin, Registration: pin, Assets: pin, Plan: pin, Commitment: pin, ReferenceSelection: pin, RunBinding: pin,
		Candidate: "candidate", SemanticCohort: "cohort", RunID: "run", ShardIDs: []string{"shard"},
	}
	scope := &ordinaryEligibilityScope{Binding: binding, ClosureEncoding: binding.ClosureEncoding, TupleRequired: 1}
	for _, tc := range []struct {
		name    string
		context string
		block   *ordinaryEligibilityBinding
		scope   *ordinaryEligibilityScope
		want    bool
	}{
		{"legacy-context-no-block", "full-input-utf8/r2", nil, nil, true},
		{"ordinary-context-unbound", OrdinaryContext, nil, nil, false},
		{"legacy-context-with-ordinary-block", "full-input-utf8/r2", &binding, nil, false},
		{"ordinary-context-bound", OrdinaryContext, &binding, scope, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ordinaryEligibilityScopeMatches(tc.context, tc.block, tc.scope); got != tc.want {
				t.Fatalf("scope match=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestOrdinaryVerifiedPagingBindsCallerSelection(t *testing.T) {
	pin := OrdinaryPin{Path: "caller/profile.json", Bytes: 7, SHA256: digestHex([]byte("profile"))}
	newSession := func(t *testing.T) (*eligibilitySession, *verifiedPaging) {
		t.Helper()
		root, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = root.Close() })
		r, e := startRun(ctxT(t), time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(r.cancel)
		binding := ordinaryEligibilityBinding{
			Schema: ordinaryEligibilityBindingSchema, Context: OrdinaryContext, ClosureEncoding: "binary-paged",
			Profile: pin, PlanningProfile: pin, Registration: pin, Assets: pin, Plan: pin, Commitment: pin, ReferenceSelection: pin, RunBinding: pin,
			Candidate: "candidate", SemanticCohort: "cohort", RunID: "run", ShardIDs: []string{"shard-1", "shard-2"},
		}
		scope := &ordinaryEligibilityScope{Binding: binding, ClosureEncoding: binding.ClosureEncoding, TupleRequired: 1}
		snapshot := &evidenceSnapshot{graphBound: true}
		s := &eligibilitySession{r: r, b: &eligibilityBudget{r: r}, snapshot: snapshot, ordinary: scope, bound: true,
			reg: eligibilityRegistration{Context: OrdinaryContext}, res: &QueryReferenceEligibilityResult{ApprovalSHA256: "approval", RegistrationSHA256: "registration", WitnessSHA256: "witness"},
			approval: eligibilityApproval{EvidencePolicySHA256: "policy", Candidate: eligibilityCandidate{Run: "candidate-run", Attempt: 1}, Cohorts: []eligibilityCohort{{ID: "cohort", Purpose: "current", Run: "candidate-run", Attempt: 1}}, Ordinary: &binding}}
		scope.session = s
		pageBinding, e := s.pagingBinding()
		if e != nil {
			t.Fatal(e)
		}
		p := &verifiedPaging{owner: s, r: r, b: s.b, snapshot: snapshot, root: root, binding: pageBinding}
		snapshot.paging = p
		return s, p
	}
	for _, tc := range []struct {
		name   string
		mutate func(*eligibilitySession)
	}{
		{"commitment-pin", func(s *eligibilitySession) { s.ordinary.Binding.Commitment.SHA256 = digestHex([]byte("other")) }},
		{"profile-pin", func(s *eligibilitySession) { s.ordinary.Binding.Profile.SHA256 = digestHex([]byte("other")) }},
		{"registration-pin", func(s *eligibilitySession) { s.ordinary.Binding.Registration.Path = "caller/other.json" }},
		{"reference-selection-pin", func(s *eligibilitySession) { s.ordinary.Binding.ReferenceSelection.Bytes++ }},
		{"closure", func(s *eligibilitySession) { s.ordinary.Binding.ClosureEncoding = "parser-paged" }},
		{"candidate", func(s *eligibilitySession) { s.ordinary.Binding.Candidate = "other" }},
		{"semantic-cohort", func(s *eligibilitySession) { s.ordinary.Binding.SemanticCohort = "other" }},
		{"run-id", func(s *eligibilitySession) { s.ordinary.Binding.RunID = "other" }},
		{"shard-order", func(s *eligibilitySession) {
			s.ordinary.Binding.ShardIDs[0], s.ordinary.Binding.ShardIDs[1] = s.ordinary.Binding.ShardIDs[1], s.ordinary.Binding.ShardIDs[0]
		}},
		{"current-cohort", func(s *eligibilitySession) { s.approval.Cohorts[0].Run = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p := newSession(t)
			if valid, e := p.valid(s); e != nil || !valid {
				t.Fatal("fresh ordinary paging authority rejected")
			}
			tc.mutate(s)
			if valid, e := p.valid(s); e == nil && valid {
				t.Fatal("mutated ordinary identity reused verified paging authority")
			}
		})
	}
}
