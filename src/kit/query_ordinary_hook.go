package kit

// Test-only observation hooks. Production code never sets them; they only report
// the ledger and never change a charge, an error or an output.
var (
	// ordinaryStageHook receives each stage boundary of VerifyOrdinaryQuerySemantics.
	ordinaryStageHook func(stage string, b *eligibilityBudget)
	// ordinaryChargeHook receives every charge with the budget values from before it.
	ordinaryChargeHook func(b *eligibilityBudget, work, logical uint64, at string, beforeWork, beforeLogical uint64)
)

func ordinaryStage(stage string, b *eligibilityBudget) {
	if ordinaryStageHook != nil {
		ordinaryStageHook(stage, b)
	}
}

// ordinaryVisitHook receives each query-independent check that really runs (an actual-tree check or
// comparison on a miss of the share, or without one; the final-identity sort) with the visits it made:
// nodes, source bytes and API rows, or the sort's comparisons, compared bytes and moves.
var ordinaryVisitHook func(kind string, visits uint64)

func ordinaryVisitObserved(kind string, visits uint64) {
	if ordinaryVisitHook != nil {
		ordinaryVisitHook(kind, visits)
	}
}

// factStepHook receives the steps the dynamic SQL adapters really take: "pair-steps" (scans,
// sort comparisons and outputs of one capIndex.pairs call or of the site sort), "chain-hops"
// (lastIdent hops) and "decoded-bytes" (source bytes decoded by srcText.span). It is separate
// from ordinaryVisitHook because sort comparison counts follow map iteration order, so the
// ledger harness that records visits must not see them.
var factStepHook func(kind string, steps uint64)

func factStepObserved(kind string, steps uint64) {
	if factStepHook != nil {
		factStepHook(kind, steps)
	}
}

// ordinaryDecodeHook receives each use of an expected asset decode: shared is true when the
// decoded value came from the per-pass share instead of a new decode.
var ordinaryDecodeHook func(doc string, shared bool)

func ordinaryDecodeObserved(doc string, shared bool) {
	if ordinaryDecodeHook != nil {
		ordinaryDecodeHook(doc, shared)
	}
}
