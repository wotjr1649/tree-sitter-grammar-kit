package kit

import (
	"strings"
	"testing"
)

var ordinaryPlanHeaderMeasuring bool
var ordinaryPlanHeaderObserved uint64
var ordinaryPlanHeaderOverlayEnabled = false

func ordinaryPlanHeaderChargeForTest(t *testing.T, plan OrdinaryShardPlan) uint64 {
	t.Helper()
	b := ordinaryTestBudget(t)
	before := b.work
	if err := b.chargeOrdinaryPlanHeaderValidation(plan); err != nil {
		t.Fatal(err)
	}
	return b.work - before
}

func ordinaryPlanHeaderMeasureAndChargeForTest(t *testing.T, plan OrdinaryShardPlan) (uint64, uint64, error) {
	t.Helper()
	b := ordinaryTestBudget(t)
	ordinaryPlanHeaderObserved = 0
	ordinaryPlanHeaderMeasuring = true
	before := b.work
	chargeErr := b.chargeOrdinaryPlanHeaderValidation(plan)
	reserved := b.work - before
	if chargeErr != nil {
		ordinaryPlanHeaderMeasuring = false
		return reserved, ordinaryPlanHeaderObserved, chargeErr
	}
	err := validateOrdinaryPlanHeader(plan, false)
	ordinaryPlanHeaderMeasuring = false
	return reserved, ordinaryPlanHeaderObserved, err
}

func TestOrdinaryPlanHeaderReservationTracksVisitedData(t *testing.T) {
	v, inventory := ordinaryP0ForPlanTest(t)
	plan := ordinaryPlanForTest(v, inventory)
	base := ordinaryPlanHeaderChargeForTest(t, plan)

	plan.Budget.ReferenceModel = strings.Repeat("uninspected estimate note ", 4096)
	if err := validateOrdinaryPlanHeader(plan, false); err != nil {
		t.Fatal(err)
	}
	if got := ordinaryPlanHeaderChargeForTest(t, plan); got != base {
		t.Fatalf("uninspected budget note changed header-validation work: got %d, want %d", got, base)
	}

	plan.Routes[0].NativeQueryIDs = append(plan.Routes[0].NativeQueryIDs, strings.Repeat("q", 128))
	plan.Routes[0].ReferenceQueryGroups = ordinaryQueryGroups(plan.Routes[0].NativeQueryIDs)
	if err := validateOrdinaryPlanHeader(plan, false); err != nil {
		t.Fatal(err)
	}
	if got := ordinaryPlanHeaderChargeForTest(t, plan); got <= base {
		t.Fatalf("visited query ID did not increase header-validation work: base=%d got=%d", base, got)
	}
}

func TestOrdinaryPlanHeaderValidationAccountingOverlay(t *testing.T) {
	if !ordinaryPlanHeaderOverlayEnabled {
		t.Skip("requires the test-only plan-accounting overlay")
	}
	v, inventory := ordinaryP0ForPlanTest(t)
	measureAndCheck := func(plan OrdinaryShardPlan, wantValid bool) uint64 {
		t.Helper()
		reserved, actual, err := ordinaryPlanHeaderMeasureAndChargeForTest(t, plan)
		if (err == nil) != wantValid {
			t.Fatalf("header validity mismatch: wantValid=%t err=%v", wantValid, err)
		}
		if actual == 0 {
			t.Fatal("overlay did not observe reservation and validator/helper visits")
		}
		t.Logf("valid=%t reserved_work=%d observed_work=%d", wantValid, reserved, actual)
		if reserved < actual {
			t.Fatalf("header validation reservation below observed work: reserved=%d actual=%d", reserved, actual)
		}
		return actual
	}

	plan := ordinaryPlanForTest(v, inventory)
	base := measureAndCheck(plan, true)
	plan.Budget.ReferenceModel = strings.Repeat("uninspected estimate note ", 4096)
	if got := measureAndCheck(plan, true); got != base {
		t.Fatalf("uninspected budget note changed observed work: base=%d got=%d", base, got)
	}

	plan = ordinaryPlanForTest(v, inventory)
	plan.Routes[0].NativeQueryIDs = append(plan.Routes[0].NativeQueryIDs, strings.Repeat("q", 128))
	plan.Routes[0].ReferenceQueryGroups = ordinaryQueryGroups(plan.Routes[0].NativeQueryIDs)
	if got := measureAndCheck(plan, true); got <= base {
		t.Fatalf("visited query ID did not increase observed work: base=%d got=%d", base, got)
	}

	plan = ordinaryPlanForTest(v, inventory)
	plan.Bindings[0].Members = append(plan.Bindings[0].Members, OrdinaryMember{
		Owner: "test", Path: "capture/雪/data.json", Role: "grammar", CaseOrBatch: "case",
		Pin: OrdinaryPin{Path: "capture/雪/data.json", SHA256: strings.Repeat("a", 64)},
	})
	measureAndCheck(plan, true)

	plan = ordinaryPlanForTest(v, inventory)
	plan.Registry.Path = "registry/" + strings.Repeat("component/", 512) + "tail"
	measureAndCheck(plan, false)

	plan = ordinaryPlanForTest(v, inventory)
	plan.Bindings[0].Members = append(plan.Bindings[0].Members, OrdinaryMember{
		Owner: "test", Path: strings.Repeat("/", 4096), Role: "grammar", CaseOrBatch: "case",
		Pin: OrdinaryPin{Path: strings.Repeat("/", 4096), SHA256: strings.Repeat("a", 64)},
	})
	measureAndCheck(plan, false)
}
