package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// CI 37106058599: cgroup.procs is empty while the kernel has not yet released a killed,
// orphaned task (cgroup.events "populated 1"), and rmdir of the leaf fails with EBUSY.
// The tree must count as non-empty until populated 0. The leaf files are simulated in a
// plain directory so the state is exact; the real backend runs in TestEscapedQuietDescendant.
func TestCgroupLiveUntilReleased(t *testing.T) {
	dir := t.TempDir()
	c := &cgroupTree{dir: dir}
	write := func(name, data string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		procs, events string
		want          int
		err           bool
	}{
		{"101\n102\n", "populated 1\nfrozen 0\n", 2, false},
		{"", "populated 1\nfrozen 0\n", 1, false}, // exiting task not yet released: busy
		{"", "populated 0\nfrozen 0\n", 0, false},
		{"", "frozen 0\n", 0, true},
	}
	for _, tc := range cases {
		write("cgroup.procs", tc.procs)
		write("cgroup.events", tc.events)
		n, err := c.live()
		if n != tc.want || (err != nil) != tc.err {
			t.Fatalf("procs %q events %q: live %d err %v, want %d err %v", tc.procs, tc.events, n, err, tc.want, tc.err)
		}
	}
}
