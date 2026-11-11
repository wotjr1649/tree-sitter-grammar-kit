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
