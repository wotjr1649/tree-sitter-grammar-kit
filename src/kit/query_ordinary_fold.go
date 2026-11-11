package kit

// This private fold accepts only the fixed selected IDs and adjudicator results.
// It does not reinterpret P0's unavailable assets or open its completion gate.
type ordinaryRuntimeCoverage struct {
	Tuple              QueryEligibilityCounts
	Assessment         string
	Code               string
	ProcessingComplete bool
	FullScopeComplete  bool
}

func (b *eligibilityBudget) foldOrdinaryRuntime(reg OrdinaryRegistration, selected []string, judgments []ordinaryTupleJudgment) (ordinaryRuntimeCoverage, *Error) {
	out := ordinaryRuntimeCoverage{Tuple: QueryEligibilityCounts{Required: uint64(len(selected)), Unprocessed: uint64(len(selected))}, Assessment: AssessBlocked}
	if e := b.charge(uint64(len(reg.Required)+len(selected)+len(judgments))+1, 512*uint64(len(reg.Required)+len(selected)), "ordinary-runtime-fold"); e != nil {
		return out, e
	}
	known := map[string]OrdinaryCoordinate{}
	for _, required := range reg.Required {
		if _, exists := known[required.ID]; exists || required.Coordinate.ID(reg.Namespace) != required.ID {
			return out, fail(KindInvalidInput, "ORDINARY_RUNTIME_MEMBERSHIP", required.ID, nil)
		}
		known[required.ID] = required.Coordinate
	}
	seen := map[string]bool{}
	for _, id := range selected {
		if _, exists := known[id]; !exists || seen[id] {
			return out, fail(KindInvalidInput, "ORDINARY_RUNTIME_SELECTION", id, nil)
		}
		seen[id] = true
	}
	if len(selected) == 0 || len(judgments) > len(selected) {
		return out, fail(KindInvalidInput, "ORDINARY_RUNTIME_SELECTION", "ordinary", nil)
	}
	for i, judgment := range judgments {
		switch judgment.State {
		case ordinaryTupleVerified:
			if judgment.Code != "" {
				return out, fail(KindInvalidInput, "ORDINARY_RUNTIME_STATE", selected[i], nil)
			}
			out.Tuple.Verified++
		case ordinaryTupleFailed:
			if judgment.Code == "" {
				return out, fail(KindInvalidInput, "ORDINARY_RUNTIME_STATE", selected[i], nil)
			}
			out.Tuple.Failed++
			if out.Assessment != AssessFail {
				out.Assessment, out.Code = AssessFail, judgment.Code
			}
		case ordinaryTupleBlocked:
			if judgment.Code == "" {
				return out, fail(KindInvalidInput, "ORDINARY_RUNTIME_STATE", selected[i], nil)
			}
			out.Tuple.Blocked++
		default:
			return out, fail(KindInvalidInput, "ORDINARY_RUNTIME_STATE", selected[i], nil)
		}
		out.Tuple.Unprocessed--
	}
	out.ProcessingComplete = out.Tuple.Unprocessed == 0
	if out.Tuple.Verified == out.Tuple.Required {
		out.Assessment = AssessPass
	} else if out.Code == "" {
		out.Code = "ORDINARY_INCOMPLETE"
	}
	return out, nil
}
