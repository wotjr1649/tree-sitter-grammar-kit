package kit

import (
	"bytes"
	"encoding/json"
	"maps"
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
	decodes := map[string][2]int{}   // expected-asset decode uses per doc: {new decodes, shared uses}
	visits := map[string][2]uint64{} // query-independent checks that really ran per kind: {visits, runs}
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
	prevStage, prevCharge, prevDecode, prevVisit, prevGuard := ordinaryStageHook, ordinaryChargeHook, ordinaryDecodeHook, ordinaryVisitHook, guardLstatHook
	// The final-identity physical walk: the guard's path bytes (between the runtime-fold stage and the
	// final-identity stage) and physicalSameFileUnits per charged path, reported as the "physical" visits.
	inFinalIdentity := false
	guardLstatHook = func(step string, n int) {
		mu.Lock()
		defer mu.Unlock()
		if inFinalIdentity {
			v := visits["physical"]
			visits["physical"] = [2]uint64{v[0] + guardLstatUnits(step, n), v[1]}
		}
	}
	ordinaryVisitHook = func(kind string, n uint64) {
		mu.Lock()
		defer mu.Unlock()
		v := visits[kind]
		visits[kind] = [2]uint64{v[0] + n, v[1] + 1}
	}
	ordinaryDecodeHook = func(doc string, shared bool) {
		mu.Lock()
		defer mu.Unlock()
		v := decodes[doc]
		if shared {
			v[1]++
		} else {
			v[0]++
		}
		decodes[doc] = v
	}
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
		if at == "ordinary-final-physical-identity" {
			p := visits["physical"]
			visits["physical"] = [2]uint64{p[0] + physicalSameFileUnits, p[1] + 1}
		}
	}
	ordinaryStageHook = func(stage string, b *eligibilityBudget) {
		mu.Lock()
		defer mu.Unlock()
		inFinalIdentity = stage == "runtime-fold"
		record := map[string]any{"kind": "stage", "stage": stage, "logical": b.logical, "work": b.work, "read": b.r.totalRead,
			"remaining_logical": b.logicalCap() - b.logical, "remaining_work": eligibilityWorkLimit - b.work,
			"remaining_read": ReplayOperations()["evidence-replay"].TotalBytes - b.r.totalRead}
		if os.Getenv("TSGK_MEASURE_STAGE_LABELS") != "" && stage != "exit" {
			record["labels"] = maps.Clone(labels[b]) // cumulative per-label charges; consecutive records give the per-stage breakdown
		}
		if stage == "exit" {
			record["labels"] = labels[b]
			record["decodes"] = decodes
			record["visits"] = visits
			delete(labels, b)
		}
		appendRecord(record)
	}
	t.Cleanup(func() {
		ordinaryStageHook, ordinaryChargeHook, ordinaryDecodeHook, ordinaryVisitHook = prevStage, prevCharge, prevDecode, prevVisit
		guardLstatHook = prevGuard
	})
}

// ordinaryLedgerDecodes reads the expected-asset decode counts of the pass from its exit record.
func ordinaryLedgerDecodes(t *testing.T, path string) map[string][2]int {
	t.Helper()
	decodes, _, _ := ordinaryLedgerExit(t, path)
	return decodes
}

// ordinaryLedgerExit reads the decode counts, the per-label {work, logical, charges, rejections}
// and the real visits {visits, runs} per check kind from the exit record of the pass.
func ordinaryLedgerExit(t *testing.T, path string) (map[string][2]int, map[string][4]uint64, map[string][2]uint64) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		var record struct {
			Kind    string               `json:"kind"`
			Stage   string               `json:"stage"`
			Decodes map[string][2]int    `json:"decodes"`
			Labels  map[string][4]uint64 `json:"labels"`
			Visits  map[string][2]uint64 `json:"visits"`
		}
		if json.Unmarshal(line, &record) == nil && record.Kind == "stage" && record.Stage == "exit" {
			return record.Decodes, record.Labels, record.Visits
		}
	}
	t.Fatalf("%s has no exit record", path)
	return nil, nil, nil
}
