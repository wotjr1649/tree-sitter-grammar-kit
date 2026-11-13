package native

import (
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
)

// A batch is accepted only from a driver that exited by itself with nothing left behind.
// ResidualAfterExit counts a descendant the runner had to end after the driver exited: the
// cleanup is verified, but the driver was not clean, so the batch is rejected like an
// unverified one (the rule of the other consumers of a process result).
func TestBatchRejection(t *testing.T) {
	clean := func() runner.BatchResult {
		return runner.BatchResult{Process: runner.Result{Status: runner.StatusCompleted, Reason: runner.ReasonExited, Cleanup: runner.Cleanup{Verified: true}}}
	}
	for name, c := range map[string]struct {
		edit  func(*runner.BatchResult)
		fatal bool
		want  string
	}{
		"clean":              {func(*runner.BatchResult) {}, false, ""},
		"unverified":         {func(b *runner.BatchResult) { b.Process.Cleanup.Verified = false }, false, "PROCESS_CLEANUP_UNVERIFIED"},
		"residual":           {func(b *runner.BatchResult) { b.Process.Cleanup.ResidualAfterExit = 1 }, false, "PROCESS_CLEANUP_UNVERIFIED"},
		"residual-fatal":     {func(b *runner.BatchResult) { b.Process.Cleanup.ResidualAfterExit = 1 }, true, "PROCESS_CLEANUP_UNVERIFIED"},
		"lingering":          {func(b *runner.BatchResult) { b.Process.Reason = runner.ReasonBatchExit }, false, "BATCH_EXIT_TIMEOUT"},
		"trailing":           {func(b *runner.BatchResult) { b.TrailingBytes = 4 }, false, "RESPONSE_TRAILING_BYTES"},
		"nonzero":            {func(b *runner.BatchResult) { b.Process.ExitCode = 2 }, false, "BATCH_EXIT_NONZERO"},
		"nonzero-fatal-stop": {func(b *runner.BatchResult) { b.Process.ExitCode = 3 }, true, ""},
	} {
		t.Run(name, func(t *testing.T) {
			b := clean()
			c.edit(&b)
			if got := batchRejection(b, c.fatal); got != c.want {
				t.Fatalf("batchRejection = %q, want %q", got, c.want)
			}
		})
	}
	if !batchCleanupClean(runner.Result{Cleanup: runner.Cleanup{Verified: true}}) || batchCleanupClean(runner.Result{Cleanup: runner.Cleanup{Verified: true, ResidualAfterExit: 1}}) {
		t.Fatal("batchCleanupClean must require Verified and no residual")
	}
}
