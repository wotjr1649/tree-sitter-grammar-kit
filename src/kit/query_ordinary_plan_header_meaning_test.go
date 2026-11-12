package kit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOrdinaryPlanHeaderGlobalMeaningAndPrecedence(t *testing.T) {
	t.Parallel()
	v, inventory := ordinaryP0ForPlanTest(t)
	original := ordinaryPlanForTest(v, inventory)
	for _, tc := range []struct {
		name   string
		change func(*OrdinaryShardPlan)
	}{
		{"nonselected-binding-path", func(p *OrdinaryShardPlan) {
			p.Bindings[len(p.Bindings)-1].Members = []OrdinaryMember{{Path: "../bad", Pin: OrdinaryPin{Path: "../bad", SHA256: strings.Repeat("a", 64)}}}
		}},
		{"nonselected-query-groups", func(p *OrdinaryShardPlan) { p.Routes[len(p.Routes)-1].ReferenceQueryGroups = [][]string{} }},
		{"nonselected-host-gap", func(p *OrdinaryShardPlan) { p.Bindings[len(p.Bindings)-1].Route = "foreign" }},
		{"capacity-notes", func(p *OrdinaryShardPlan) { p.CapacityUnknown[0] = "changed" }},
		{"sql-budget", func(p *OrdinaryShardPlan) { p.Budget.SQL[0].HistoricalCoreLogical++ }},
		{"route-budget", func(p *OrdinaryShardPlan) { p.Budget.ReferenceRoutes[len(p.Routes)-1].Dense.Density++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var plan OrdinaryShardPlan
			if err := json.Unmarshal(ordinaryJSON(t, original), &plan); err != nil {
				t.Fatal(err)
			}
			tc.change(&plan)
			want := asError(validateOrdinaryPlanHeader(plan, false))
			if want == nil {
				t.Fatal("mutation did not change header meaning")
			}
			data, selection := ordinarySelectionForTest(t, plan, []int{0})
			// A later malformed proof must not outrank a global header meaning error.
			selection.Proofs[0] = []byte("{")
			_, got := ordinaryTestBudget(t).ordinarySelectedPlan(data, selection)
			if got == nil || got.Kind != want.Kind || got.Code != want.Code || got.Path != want.Path {
				t.Fatalf("header priority changed: got %v want %v", got, want)
			}
		})
	}
}
