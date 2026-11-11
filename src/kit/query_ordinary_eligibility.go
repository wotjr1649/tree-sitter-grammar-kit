package kit

import (
	"crypto/sha256"
	"encoding/binary"
	"io"
	"reflect"
)

const ordinaryEligibilityBindingSchema = "tsgk-query-ordinary-eligibility-binding/r1"

// ordinaryEligibilityBinding pins every ordinary caller document into the existing approval.
type ordinaryEligibilityBinding struct {
	Schema             string      `json:"schema"`
	Context            string      `json:"context"`
	ClosureEncoding    string      `json:"closure_encoding"`
	Profile            OrdinaryPin `json:"profile_pin"`
	PlanningProfile    OrdinaryPin `json:"planning_profile_pin"`
	Registration       OrdinaryPin `json:"registration_pin"`
	Assets             OrdinaryPin `json:"asset_registration_pin"`
	Plan               OrdinaryPin `json:"shard_plan_pin"`
	Commitment         OrdinaryPin `json:"plan_commitment_pin"`
	ReferenceSelection OrdinaryPin `json:"reference_selection_pin"`
	RunBinding         OrdinaryPin `json:"run_binding_pin"`
	Candidate          string      `json:"candidate"`
	SemanticCohort     string      `json:"semantic_cohort"`
	RunID              string      `json:"run_id"`
	ShardIDs           []string    `json:"selected_shard_ids"`
}

// ordinaryEligibilityScope exists only during the ordinary API call. It keeps the verified
// eligibility session alive so the ordinary reader can reuse its root, ledger and page authority.
type ordinaryEligibilityScope struct {
	Binding         ordinaryEligibilityBinding
	ClosureEncoding string
	TupleRequired   uint64
	session         *eligibilitySession
}

func validOrdinaryEligibilityBinding(b ordinaryEligibilityBinding) bool {
	if b.Schema != ordinaryEligibilityBindingSchema || b.Context != OrdinaryContext || !ordinaryClosure(b.ClosureEncoding) || b.Candidate == "" || b.SemanticCohort == "" || b.RunID == "" || b.ShardIDs == nil || len(b.ShardIDs) == 0 || len(b.ShardIDs) > NativeOperations()["native-query-batch"].MaxCases {
		return false
	}
	for _, p := range []OrdinaryPin{b.Profile, b.PlanningProfile, b.Registration, b.Assets, b.Plan, b.Commitment, b.ReferenceSelection, b.RunBinding} {
		if !ordinaryPin(p) {
			return false
		}
	}
	seen := make(map[string]bool, len(b.ShardIDs))
	for _, id := range b.ShardIDs {
		if id == "" || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func ordinaryEligibilityBindingMatches(attested, selected ordinaryEligibilityBinding) bool {
	return validOrdinaryEligibilityBinding(attested) && validOrdinaryEligibilityBinding(selected) && reflect.DeepEqual(attested, selected)
}

func ordinaryEligibilityScopeMatches(context string, attested *ordinaryEligibilityBinding, scope *ordinaryEligibilityScope) bool {
	if scope == nil {
		return context != OrdinaryContext && attested == nil
	}
	return context == OrdinaryContext && attested != nil && ordinaryEligibilityBindingMatches(*attested, scope.Binding) && attested.ClosureEncoding == scope.ClosureEncoding && scope.TupleRequired > 0
}

func ordinaryPagingDigest(binding ordinaryEligibilityBinding) [32]byte {
	h := sha256.New()
	write := func(value string) {
		var length [8]byte
		binary.LittleEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = h.Write(length[:])
		_, _ = io.WriteString(h, value)
	}
	for _, pin := range []OrdinaryPin{binding.Profile, binding.PlanningProfile, binding.Registration, binding.Assets, binding.Plan, binding.Commitment, binding.ReferenceSelection, binding.RunBinding} {
		write(pin.Path)
		var size [8]byte
		binary.LittleEndian.PutUint64(size[:], pin.Bytes)
		_, _ = h.Write(size[:])
		write(pin.SHA256)
	}
	for _, value := range []string{binding.Schema, binding.Context, binding.ClosureEncoding, binding.Candidate, binding.SemanticCohort, binding.RunID} {
		write(value)
	}
	var count [8]byte
	binary.LittleEndian.PutUint64(count[:], uint64(len(binding.ShardIDs)))
	_, _ = h.Write(count[:])
	for _, id := range binding.ShardIDs {
		write(id)
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

func ordinaryPagingEnabled(s *ordinaryEligibilityScope) bool {
	return s != nil && (s.ClosureEncoding == "parser-paged" || s.ClosureEncoding == "binary-paged")
}
