package kit

import (
	"encoding/json"
	"os"
	"sync"
	"testing"
)

// ordinaryInstallLedgerHooks appends stage, rejected-charge and per-label records to path
// for budgets created while the test runs. It only observes; it never changes a charge.
func ordinaryInstallLedgerHooks(t *testing.T, path string) {
	t.Helper()
	var mu sync.Mutex
	labels := map[*eligibilityBudget]map[string][4]uint64{}
	appendRecord := func(v any) {
		data, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}
		if _, err = f.Write(append(data, '\n')); err != nil {
			panic(err)
		}
		if err = f.Close(); err != nil {
			panic(err)
		}
	}
	prevStage, prevCharge := ordinaryStageHook, ordinaryChargeHook
	ordinaryChargeHook = func(b *eligibilityBudget, work, logical uint64, at string, beforeWork, beforeLogical uint64) {
		if b.r == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if labels[b] == nil {
			labels[b] = map[string][4]uint64{}
		}
		v := labels[b][at]
		v[0], v[1], v[2] = v[0]+work, v[1]+logical, v[2]+1
		if b.work-beforeWork != work || b.logical-beforeLogical != logical {
			v[3]++
			appendRecord(map[string]any{"kind": "rejected", "label": at, "logical": beforeLogical, "work": beforeWork, "read": b.r.totalRead,
				"requested_work": work, "requested_logical": logical,
				"remaining_work": eligibilityWorkLimit - beforeWork, "remaining_logical": b.logicalCap() - beforeLogical,
				"work_deficit":    max(work, eligibilityWorkLimit-beforeWork) - (eligibilityWorkLimit - beforeWork),
				"logical_deficit": max(logical, b.logicalCap()-beforeLogical) - (b.logicalCap() - beforeLogical)})
		}
		labels[b][at] = v
	}
	ordinaryStageHook = func(stage string, b *eligibilityBudget) {
		mu.Lock()
		defer mu.Unlock()
		record := map[string]any{"kind": "stage", "stage": stage, "logical": b.logical, "work": b.work, "read": b.r.totalRead,
			"remaining_logical": b.logicalCap() - b.logical, "remaining_work": eligibilityWorkLimit - b.work,
			"remaining_read": ReplayOperations()["evidence-replay"].TotalBytes - b.r.totalRead}
		if stage == "exit" {
			record["labels"] = labels[b]
			delete(labels, b)
		}
		appendRecord(record)
	}
	t.Cleanup(func() { ordinaryStageHook, ordinaryChargeHook = prevStage, prevCharge })
}
