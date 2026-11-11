package kit

import (
	"reflect"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The approval pins the commitment separately from the complete plan retained by r8/r2.
type ordinaryPlanSelection struct {
	Binding ordinaryEligibilityBinding
	Proofs  [][]byte
}

func (b *eligibilityBudget) ordinarySelectedPlan(data []byte, selection ordinaryPlanSelection) (OrdinaryShardPlan, *Error) {
	var plan OrdinaryShardPlan
	bad := func(code string) (OrdinaryShardPlan, *Error) {
		return plan, fail(KindInvalidInput, code, "ordinary-plan-selection", nil)
	}
	binding := selection.Binding
	if !validOrdinaryEligibilityBinding(binding) || len(selection.Proofs) != len(binding.ShardIDs) {
		return bad("ORDINARY_PLAN_SELECTION")
	}
	if e := b.charge(1, uint64(len(data)), "ordinary-plan-commitment-pin"); e != nil {
		return plan, e
	}
	if binding.Commitment != queryregistrycore.FilePin(binding.Commitment.Path, data) {
		return bad("ORDINARY_PLAN_COMMITMENT_PIN")
	}
	var commitment ordinaryPlanCommitment
	if e := b.decodeOrdinaryTyped("ordinary-plan-commitment", data, &commitment); e != nil {
		return plan, e
	}
	if commitment.Schema != ordinaryPlanCommitmentSchema || commitment.Plan.Path != binding.Plan.Path || commitment.Plan.Bytes != binding.Plan.Bytes || commitment.Plan.SHA256 != binding.Plan.SHA256 || commitment.ShardCount != 6402 || !hex64.MatchString(commitment.Root) {
		return bad("ORDINARY_PLAN_COMMITMENT")
	}
	if e := b.decodeOrdinaryTyped("ordinary-plan-header", commitment.Header, &plan); e != nil {
		return plan, e
	}
	if e := b.charge(32*uint64(len(commitment.Header)), 4*uint64(len(commitment.Header)), "ordinary-plan-header-validation"); e != nil {
		return plan, e
	}
	if err := validateOrdinaryPlanHeader(plan, false); err != nil {
		return plan, asError(err)
	}
	if len(plan.Shards) != 0 || plan.Profile != binding.PlanningProfile || plan.Assets != binding.Assets || plan.Candidate != binding.Candidate || plan.SemanticCohort != binding.SemanticCohort {
		return bad("ORDINARY_PLAN_APPROVAL_BINDING")
	}
	var previous uint64
	for i, raw := range selection.Proofs {
		var proof ordinaryShardProof
		if e := b.decodeOrdinaryTyped("ordinary-plan-proof", raw, &proof); e != nil {
			return plan, e
		}
		if i > 0 && proof.Index <= previous {
			return bad("ORDINARY_PLAN_PROOF_ORDER")
		}
		previous = proof.Index
		// Verify hashes the exact raw shard, context and one path; no complete-plan bytes are read.
		if e := b.charge(uint64(len(commitment.Header)+len(proof.Shard))+512*uint64(len(proof.Siblings))+2048, 8*uint64(len(commitment.Header)+len(proof.Shard))+256*uint64(len(proof.Siblings))+4096, "ordinary-plan-proof-hash"); e != nil {
			return plan, e
		}
		if err := verifyOrdinaryCommitment(commitment, proof.Shard, proof); err != nil {
			return bad("ORDINARY_PLAN_INCLUSION")
		}
		var shard OrdinaryPlanShard
		if e := b.decodeOrdinaryTyped("ordinary-plan-selected-shard", proof.Shard, &shard); e != nil {
			return plan, e
		}
		if shard.ID != binding.ShardIDs[i] {
			return bad("ORDINARY_PLAN_SELECTION")
		}
		if e := b.ordinarySelectedShard(plan, shard); e != nil {
			return plan, e
		}
		plan.Shards = append(plan.Shards, shard)
	}
	return plan, b.r.check()
}

func (b *eligibilityBudget) ordinarySelectedShard(plan OrdinaryShardPlan, shard OrdinaryPlanShard) *Error {
	bad := func() *Error {
		return fail(KindInvalidInput, "ORDINARY_PLAN_COORDINATE", "ordinary-plan-selected-shard", nil)
	}
	if shard.ID != ordinaryPlanShardKey(shard.Route, shard.Platform, shard.Case) || shard.Case == "" || !ordinaryHost(shard.Platform, shard.Subject) || len(shard.SourceHistory) == 0 || len(shard.SourceHistory) > 5 || len(shard.EditPoints)+1 != len(shard.SourceHistory) || !reflect.DeepEqual(shard.ReferenceSteps, ordinaryReferenceSteps(uint32(len(shard.SourceHistory)-1))) || shard.Admission != (OrdinaryPlanAdmission{State: "BLOCKED", Code: "CAPACITY_UNPROVEN"}) {
		return bad()
	}
	ri := slices.IndexFunc(plan.Routes, func(r OrdinaryPlanRoute) bool { return r.Route == shard.Route })
	bi := slices.IndexFunc(plan.Bindings, func(v OrdinaryBinding) bool { return v.Route+"/"+v.Platform == shard.BindingID })
	if ri < 0 || bi < 0 || shard.BindingID != shard.Route+"/"+shard.Platform || shard.Set != plan.Routes[ri].Set || shard.Subject != plan.Bindings[bi].Subject || shard.Capacity != ordinaryShardCapacity(shard.SourceHistory, plan.Bindings[bi].Members) {
		return bad()
	}
	for _, step := range shard.SourceHistory {
		if !hex64.MatchString(step.SHA256) || step.Bytes > 65536 {
			return bad()
		}
	}
	queries := plan.Routes[ri].NativeQueryIDs
	if len(queries) == 0 || len(queries) > MaxQueries {
		return bad()
	}
	if e := b.charge(32*uint64(len(shard.ObligationIDs)+len(plan.Routes)+len(plan.Bindings)), 4096*uint64(len(shard.ObligationIDs)+1), "ordinary-plan-selected-coordinate"); e != nil {
		return e
	}
	index := 0
	for _, query := range queries {
		for step := range shard.SourceHistory {
			for _, role := range []string{"incremental", "fresh"} {
				if step == 0 && role == "fresh" {
					continue
				}
				c := OrdinaryCoordinate{Kind: queryregistrycore.Structural, Route: shard.Route, Platform: shard.Platform, Subject: shard.Subject, Role: "primary", Set: shard.Set, Case: shard.Case, Query: query, Step: uint32(step), TreeRole: role, Context: "qualification/r8"}
				id, e := b.ordinaryCoordinateID(c, plan.Namespace)
				if e != nil {
					return e
				}
				if index >= len(shard.ObligationIDs) || shard.ObligationIDs[index] != id {
					return bad()
				}
				index++
			}
		}
	}
	if index != len(shard.ObligationIDs) || ordinaryIDDigest(shard.ObligationIDs) != shard.ObligationIDSHA256 {
		return bad()
	}
	return b.r.check()
}

func (b *eligibilityBudget) ordinaryCoordinateID(c OrdinaryCoordinate, namespace string) (string, *Error) {
	// JSON may escape each input byte to six bytes; reserve before canonicalizing and hashing.
	n := uint64(len(namespace) + len(c.Kind) + len(c.Route) + len(c.Platform) + len(c.Subject) + len(c.Role) + len(c.Set) + len(c.Case) + len(c.Query) + len(c.TreeRole) + len(c.Context))
	bound := 6*n + 512
	if e := b.charge(bound, 8*bound+1024, "ordinary-selected-coordinate-hash"); e != nil {
		return "", e
	}
	return c.ID(namespace), nil
}
