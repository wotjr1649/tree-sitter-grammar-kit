package kit

import (
	"reflect"
	"testing"
)

func TestOrdinarySourceDocumentHonorsWorkAndLogicalBoundaries(t *testing.T) {
	data := ordinaryJSON(t, ordinaryProfileFixture())
	baseline := ordinaryTestBudget(t)
	var profile OrdinaryProfile
	if e := baseline.decodeOrdinaryTyped("ordinary-profile", data, &profile); e != nil {
		t.Fatal("baseline decode:", e)
	}
	workCost, logicalCost := baseline.work, baseline.logical
	if workCost == 0 || logicalCost == 0 || workCost >= eligibilityWorkLimit || logicalCost >= eligibilityLogicalLimit {
		t.Fatalf("unexpected source decode costs: work=%d logical=%d", workCost, logicalCost)
	}

	for _, tc := range []struct {
		name string
		seed func(*eligibilityBudget, uint64)
		cost uint64
		cap  uint64
	}{
		{"work", func(b *eligibilityBudget, n uint64) { b.work = n }, workCost, eligibilityWorkLimit},
		{"logical", func(b *eligibilityBudget, n uint64) { b.logical = n }, logicalCost, eligibilityLogicalLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, over := range []bool{false, true} {
				b := ordinaryTestBudget(t)
				seed := tc.cap - tc.cost
				if over {
					seed++
				}
				tc.seed(b, seed)
				var got OrdinaryProfile
				err := b.decodeOrdinaryTyped("ordinary-profile", data, &got)
				if !over && (err != nil || b.work != eligibilityWorkLimit && tc.name == "work" || b.logical != eligibilityLogicalLimit && tc.name == "logical") {
					t.Fatalf("exact cap rejected: work=%d logical=%d err=%v", b.work, b.logical, err)
				}
				if over && (err == nil || err.Code != "ELIGIBILITY_WORK_LIMIT") {
					t.Fatalf("one byte over cap accepted: work=%d logical=%d err=%v", b.work, b.logical, err)
				}
			}
		})
	}
}

func TestOrdinaryCallerDocumentsHonorReadBoundary(t *testing.T) {
	if OrdinaryPlanLimits().ReadBytes != ReplayOperations()["evidence-replay"].TotalBytes {
		t.Fatal("ordinary caller reads must inherit the existing evidence-replay read cap")
	}
	for _, over := range []bool{false, true} {
		b := ordinaryTestBudget(t)
		b.r.totalRead = OrdinaryPlanLimits().ReadBytes - 1
		err := ordinaryCallerRead(b.r, "ordinary-source", []byte{'x'})
		if !over && (err != nil || b.r.totalRead != OrdinaryPlanLimits().ReadBytes) {
			t.Fatalf("exact read cap rejected: bytes=%d err=%v", b.r.totalRead, err)
		}
		if over {
			err = ordinaryCallerRead(b.r, "ordinary-source", []byte{'y'})
			if err == nil || err.Code != "TOTAL_BYTES_LIMIT" || b.r.totalRead != OrdinaryPlanLimits().ReadBytes+1 {
				t.Fatalf("one byte over read cap accepted: bytes=%d err=%v", b.r.totalRead, err)
			}
		}
	}
}

func TestOrdinaryReducerDispatchStaysSeparateFromLegacySemanticReducers(t *testing.T) {
	table := reducerTable()
	ordinary, ok := table[ordinaryReplayReducerID]
	if !ok || ordinary.info.Revision != "r9" || ordinary.run == nil || reflect.ValueOf(ordinary.run).Pointer() == reflect.ValueOf(replayQuerySemantics).Pointer() {
		t.Fatalf("ordinary reducer is not separately registered: %+v", ordinary.info)
	}
	if semanticReducer(ordinaryReplayReducerID) {
		t.Fatal("ordinary reducer entered the legacy r4-r6 context decoder")
	}
	for _, tc := range []struct {
		id, revision string
	}{
		{"oracle-set-r4", "r4"}, {"oracle-set-r5", "r5"}, {"oracle-set-r6", "r6"},
	} {
		legacy, ok := table[tc.id]
		if !ok || legacy.info.Revision != tc.revision || legacy.run == nil || reflect.ValueOf(legacy.run).Pointer() != reflect.ValueOf(replayQuerySemantics).Pointer() || !semanticReducer(tc.id) {
			t.Errorf("legacy reducer dispatch changed for %s: %+v", tc.id, legacy.info)
		}
	}
}
