package kit

import (
	"slices"
	"testing"
)

// #266: only a process that exited 0 by itself (not by a signal) with verified cleanup and
// nothing after its last response vouches for its answers.
func TestOracleBatchAccepted(t *testing.T) {
	ok := OracleBatch{Status: "COMPLETED", Reason: "EXITED", CleanupVerified: true}
	if !ok.Accepted() {
		t.Fatal("a clean exit 0 is not accepted")
	}
	for name, change := range map[string]func(*OracleBatch){
		"signal":         func(b *OracleBatch) { b.Status = "FAILED" }, // exit -1 recorded as 0
		"nonzero":        func(b *OracleBatch) { b.Status, b.ExitCode = "FAILED", 5 },
		"fatal-exit-3":   func(b *OracleBatch) { b.Status, b.ExitCode = "FAILED", 3 },
		"frame-watchdog": func(b *OracleBatch) { b.Status, b.Reason = "RESOURCE_LIMIT", "FRAME_WATCHDOG" },
		"stderr-limit":   func(b *OracleBatch) { b.Status, b.Reason = "RESOURCE_LIMIT", "OUTPUT_LIMIT" },
		"lingered":       func(b *OracleBatch) { b.Status, b.Reason = "RESOURCE_LIMIT", "BATCH_EXIT_TIMEOUT" },
		"cancelled":      func(b *OracleBatch) { b.Status, b.Reason = "CANCELLED", "CANCELLED" },
		"cleanup":        func(b *OracleBatch) { b.CleanupVerified = false },
		"trailing":       func(b *OracleBatch) { b.TrailingBytes = 4 },
	} {
		b := ok
		change(&b)
		if b.Accepted() {
			t.Errorf("%s accepted", name)
		}
	}
}

// #266: the shared batch rules on records already decoded: a completed case needs an
// answered frame of an accepted batch with no unanswered sent frame and a raw response that
// answers it; a completed FAIL stays acceptable evidence of failure.
func TestCheckBatches(t *testing.T) {
	clean := OracleBatch{Index: 0, Cases: []string{"a", "b"}, Status: "COMPLETED", Reason: "EXITED", CleanupVerified: true}
	base := func() (OracleManifest, map[string]recordTransport, map[string]string, map[string]int, map[string]string) {
		return OracleManifest{Cases: []string{"a", "b"}, Batches: []OracleBatch{clean}},
			map[string]recordTransport{"a": {Sent: true, Answered: true}, "b": {Sent: true, Frame: 1, Answered: true}},
			map[string]string{"a": StatusCompleted, "b": StatusCompleted}, map[string]int{"a": 1, "b": 1}, map[string]string{"a": "a", "b": "b"}
	}
	run := func(m OracleManifest, tr map[string]recordTransport, st map[string]string, raws map[string]int, ids map[string]string) []string {
		var codes []string
		checkBatches(m, tr, st, raws, ids, func(code, _, _ string) { codes = append(codes, code) })
		return codes
	}
	if got := run(base()); len(got) != 0 {
		t.Fatalf("clean set: %v", got)
	}
	for name, c := range map[string]struct {
		change func(*OracleManifest, map[string]recordTransport, map[string]string, map[string]int, map[string]string)
		want   string
	}{
		"swapped-frame": {func(_ *OracleManifest, tr map[string]recordTransport, _ map[string]string, _ map[string]int, _ map[string]string) {
			tr["a"] = recordTransport{Sent: true, Frame: 1, Answered: true}
		}, "BATCH_FRAME_MISMATCH"},
		"raw-of-another-case": {func(_ *OracleManifest, _ map[string]recordTransport, _ map[string]string, _ map[string]int, ids map[string]string) {
			ids["a"] = "b"
		}, "BATCH_RAW_MISMATCH"},
		"unanswered-sent-frame": {func(_ *OracleManifest, tr map[string]recordTransport, st map[string]string, raws map[string]int, _ map[string]string) {
			tr["b"], st["b"], raws["b"] = recordTransport{Sent: true, Frame: 1}, StatusFailed, 0
		}, "BATCH_SUCCESS_UNACCEPTED"},
		"not-accepted": {func(m *OracleManifest, _ map[string]recordTransport, _ map[string]string, _ map[string]int, _ map[string]string) {
			m.Batches[0].Status = "FAILED"
		}, "BATCH_SUCCESS_UNACCEPTED"},
		"completed-never-sent": {func(_ *OracleManifest, tr map[string]recordTransport, _ map[string]string, raws map[string]int, _ map[string]string) {
			tr["a"], raws["a"] = recordTransport{}, 0
		}, "BATCH_SUCCESS_UNACCEPTED"},
	} {
		m, tr, st, raws, ids := base()
		m.Batches = slices.Clone(m.Batches)
		c.change(&m, tr, st, raws, ids)
		if got := run(m, tr, st, raws, ids); !slices.Contains(got, c.want) {
			t.Errorf("%s: %v, want %s", name, got, c.want)
		}
	}
	// a completed FAIL of a batch that is not accepted is kept as a failure
	m, tr, st, raws, ids := base()
	m.Batches = []OracleBatch{clean}
	m.Batches[0].Status = "FAILED"
	tr["a"] = recordTransport{Sent: true, Answered: true, Fail: true}
	st["b"] = StatusFailed
	if got := run(m, tr, st, raws, ids); len(got) != 0 {
		t.Fatalf("a kept FAIL was rejected: %v", got)
	}
}
