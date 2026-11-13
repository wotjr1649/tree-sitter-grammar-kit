//go:build linux || darwin

package runner

import (
	"strings"
	"testing"
)

// A backend that does not contain descendants cannot end a detached one that holds stdout
// after the helper's clean exit. The batch must not read that as a clean cleanup: the
// interactive stdout is held past Grace, which is the evidence an undrained stderr pipe
// already gives (Verified=false, "output pipe still held"). The helper's own exit stays
// EXITED. It asks for the process-group backend, so it runs on Linux even where CI
// delegates a cgroup, and on macOS.
func TestBatchExitHolderEscaped(t *testing.T) {
	r, _ := exitHolderBatch(t, true)
	if r.Process.Capabilities.EscapedDescendants == Contained {
		t.Skip("covered by TestBatchExitHolderContained")
	}
	c := r.Process.Cleanup
	if c.Verified || !strings.Contains(c.Detail, "output pipe still held") {
		t.Fatalf("an escaped stdout holder after a clean exit passed as verified cleanup: %+v", c)
	}
}
