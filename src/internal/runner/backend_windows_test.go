package runner

import (
	"context"
	"io"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestJobActiveProcesses(t *testing.T) {
	for _, c := range []struct {
		name          string
		active, total uint32
		rootDone      bool
		want          int
	}{
		{"empty", 0, 1, true, 0},
		{"root-unconfirmed", 1, 1, false, 1},
		{"confirmed-sole-root", 1, 1, true, 0},
		{"child-history", 1, 2, true, 1},
		{"multiple-active", 2, 2, true, 2},
		{"inconsistent-zero-total", 1, 0, true, 1},
		{"inconsistent-active-count", 2, 1, true, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			acc := jobAccounting{ActiveProcesses: c.active, TotalProcesses: c.total}
			if got := jobActiveProcesses(acc, c.rootDone); got != c.want {
				t.Fatalf("count %d, want %d", got, c.want)
			}
		})
	}
}

func TestJobRootExitNotification(t *testing.T) {
	for _, interactive := range []bool{false, true} {
		for _, code := range []string{"0", "3"} {
			s, _ := helperSpec(t, "exit:"+code)
			s.Interactive = interactive
			p, err := Start(context.Background(), s)
			if err != nil {
				t.Fatal(err)
			}
			job := p.tree.(*jobTree)
			if interactive {
				if _, err := io.ReadAll(p.Stdout()); err != nil {
					t.Fatal(err)
				}
			}
			r := p.Wait()
			if !job.rootDone.Load() || !r.Cleanup.Verified || r.Cleanup.ResidualAfterExit != 0 {
				t.Fatalf("root notification/cleanup: %+v", r)
			}
			if (code == "0") != r.Succeeded() || (code == "3" && (r.Status != StatusFailed || r.ExitCode != 3)) {
				t.Fatalf("exit status changed: %+v", r)
			}
		}
	}
}

func TestJobAccountingQueryError(t *testing.T) {
	for _, rootDone := range []bool{false, true} {
		job := &jobTree{job: windows.InvalidHandle}
		job.rootDone.Store(rootDone)
		if _, err := job.live(); err == nil {
			t.Fatal("invalid job handle was classified as an empty tree")
		}
	}
}

func TestJobRootTermination(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		s, pids := helperSpec(t, "sleep")
		ctx, cancel := context.WithCancel(context.Background())
		if !cancelled {
			s.Wall = 200 * time.Millisecond
			testHookWall = func() {
				if !waitFor(func() bool { return countPIDs(pids) == 1 }) {
					t.Error("root not recorded before wall expiry")
				}
			}
		}
		p, err := Start(ctx, s)
		if err != nil {
			cancel()
			testHookWall = nil
			t.Fatal(err)
		}
		if cancelled {
			if !waitFor(func() bool { return countPIDs(pids) == 1 }) {
				t.Error("root not recorded before cancellation")
			}
			cancel()
		}
		r := p.Wait()
		cancel()
		testHookWall = nil
		want, reason := StatusResourceLimit, ReasonWall
		if cancelled {
			want, reason = StatusCancelled, ReasonCancelled
		}
		if !p.tree.(*jobTree).rootDone.Load() || !r.Cleanup.Verified || r.Succeeded() || r.Status != want || r.Reason != reason {
			t.Fatalf("root termination status/cleanup: %+v", r)
		}
		requireDead(t, readPIDs(t, pids), 1)
	}
}
