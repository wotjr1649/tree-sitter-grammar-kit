package runner

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// S04-A16: the adopted values are the defaults, not caller tuning.
func TestDefaultBatchPolicy(t *testing.T) {
	p := DefaultBatchPolicy()
	if p.RequestBytes != 50331648 || p.ResponseBytes != 16777216 || p.FrameWall != 60*time.Second || p.FrameGrace != 5*time.Second ||
		p.StdoutBytes != 268435456 || p.BatchWall != 3600*time.Second {
		t.Fatalf("batch policy drifted from the adopted values: %+v", p)
	}
}

// S04-A16: the single-request process policy is the adopted one and refuses a fifth edit
// before any launch.
func TestDefaultRequestPolicy(t *testing.T) {
	p := DefaultRequestPolicy()
	if p.ParseWall != 90*time.Second || p.EditWall != 300*time.Second || p.MaxEdits != 4 || p.Memory != 4294967296 || p.Grace != 5*time.Second {
		t.Fatalf("request policy drifted from the adopted values: %+v", p)
	}
	for edits, wall := range map[int]time.Duration{0: 90 * time.Second, 1: 300 * time.Second, 4: 300 * time.Second} {
		var s Spec
		if err := p.Apply(&s, edits); err != nil || s.Wall != wall || s.Memory.Bytes != 4294967296 || s.Grace != 5*time.Second {
			t.Fatalf("%d edits: %v %+v", edits, err, s)
		}
		if s.Memory.Hard != (runtime.GOOS != "darwin") {
			t.Fatalf("memory hardness on %s: %+v", runtime.GOOS, s.Memory)
		}
	}
	for _, edits := range []int{5, -1} {
		var s Spec
		var re *Error
		if err := p.Apply(&s, edits); !errors.As(err, &re) || re.Code != "EDIT_COUNT_LIMIT" || !re.Blocked {
			t.Fatalf("%d edits accepted: %v", edits, err)
		}
	}
}

// awaitStart keeps helper start-up out of the first frame's watchdog: the batch sends its
// first frame only after the helper reports that it reads frames (#65).
func awaitStart(t *testing.T, pids string) {
	prev := testHookBatchStarted
	testHookBatchStarted = func() {
		if !waitFor(func() bool { return exists(pids + ".ready") }) {
			t.Error("frame helper not ready")
		}
	}
	t.Cleanup(func() { testHookBatchStarted = prev })
}

// controlFrameTimers expires only frame at, after the helper has read that request.
// Other timers stop normally without competing with the limit under examination (#65).
func controlFrameTimers(t *testing.T, pids string, at int, duration time.Duration) {
	t.Helper()
	prev := testHookFrameTimer
	var pending sync.WaitGroup
	frame := 0
	testHookFrameTimer = func(d time.Duration, expire func()) func() {
		if d != duration {
			t.Errorf("frame watchdog duration %v, want %v", d, duration)
		}
		var active atomic.Bool
		active.Store(true)
		if frame == at {
			pending.Add(1)
			go func() {
				defer pending.Done()
				if !waitFor(func() bool { return exists(pids + ".at") }) {
					t.Error("watchdog frame not reached")
					return
				}
				if active.CompareAndSwap(true, false) {
					expire()
				}
			}()
		}
		frame++
		return func() { active.Store(false) }
	}
	t.Cleanup(func() {
		pending.Wait()
		testHookFrameTimer = prev
		if frame == 0 {
			t.Error("no frame watchdog was armed")
		}
	})
}

// The default timer still ends an unresponsive first frame. Readiness removes startup
// from its budget and no completed frame has to win a scheduling race with this timer.
func TestFrameWatchdogRealTimer(t *testing.T) {
	s, pids := helperSpec(t, "frames:hang@0")
	awaitStart(t, pids)
	pol := BatchPolicy{RequestBytes: 1024, ResponseBytes: 1024, FrameWall: 300 * time.Millisecond, FrameGrace: 200 * time.Millisecond,
		StdoutBytes: 1 << 20, BatchWall: 30 * time.Second}
	r, err := RunBatch(context.Background(), s, pol, []Frame{{ID: "a", Request: []byte("x")}, {ID: "b", Request: []byte("y")}})
	if err != nil || r.Process.Reason != ReasonFrameWatchdog || !r.Process.Cleanup.Verified ||
		r.Frames[0].Status != FrameResourceLimit || r.Frames[1].Status != FrameRequeued {
		t.Fatalf("real watchdog: %v %+v", err, r)
	}
}

// R1-06: a frame hung while an escaped descendant holds stdout still ends within the
// declared bounds on every backend.
func TestFrameEscapedStdoutHolder(t *testing.T) {
	s, pids := helperSpec(t, "frames:escape@1")
	s.Grace = time.Second
	awaitStart(t, pids)
	pol := BatchPolicy{RequestBytes: 1024, ResponseBytes: 1024, FrameWall: 300 * time.Millisecond, FrameGrace: 200 * time.Millisecond,
		StdoutBytes: 1 << 20, BatchWall: 30 * time.Second}
	controlFrameTimers(t, pids, 1, pol.FrameWall+pol.FrameGrace)
	start := time.Now()
	r, err := RunBatch(context.Background(), s, pol, []Frame{{ID: "a", Request: []byte("x")}, {ID: "b", Request: []byte("y")}, {ID: "c", Request: []byte("z")}})
	if err != nil {
		t.Fatal(err)
	}
	holders := readPIDs(t, pids)
	defer func() {
		for _, pid := range holders {
			killPID(pid)
		}
	}()
	if len(holders) == 0 {
		t.Fatal("no escaped stdout holder was recorded")
	}
	if el := time.Since(start); el > 15*time.Second {
		t.Fatalf("batch took %v", el)
	}
	if r.Frames[0].Status != FrameCompleted || r.Frames[1].Status != FrameResourceLimit || r.Frames[2].Status != FrameRequeued {
		t.Fatalf("%+v", r.Frames)
	}
	if r.Process.Reason != ReasonFrameWatchdog {
		t.Fatalf("escaped stdout holder ended by %s, want %s", r.Process.Reason, ReasonFrameWatchdog)
	}
}

// R1-06: a helper that crashes mid-frame while an escaped descendant holds stdout gives
// FAILED for that frame within bounds, without any limit firing first.
func TestFrameCrashWithStdoutHolder(t *testing.T) {
	s, pids := helperSpec(t, "frames:crash-holder@1")
	s.Grace = time.Second
	pol := BatchPolicy{RequestBytes: 1024, ResponseBytes: 1024, FrameWall: 20 * time.Second, FrameGrace: time.Second,
		StdoutBytes: 1 << 20, BatchWall: 60 * time.Second}
	type outcome struct {
		r   BatchResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		r, err := RunBatch(context.Background(), s, pol, []Frame{{ID: "a", Request: []byte("x")}, {ID: "b", Request: []byte("y")}, {ID: "c", Request: []byte("z")}})
		done <- outcome{r, err}
	}()
	var o outcome
	select {
	case o = <-done:
	case <-time.After(10 * time.Second):
		for _, pid := range readPIDs(t, pids) {
			killPID(pid) // lets the stuck read end so the goroutine can finish
		}
		t.Fatal("RunBatch still blocked 10 s after the helper crashed")
	}
	defer func() {
		for _, pid := range readPIDs(t, pids) {
			killPID(pid)
		}
	}()
	r, err := o.r, o.err
	if err != nil {
		t.Fatal(err)
	}
	if r.Frames[0].Status != FrameCompleted || r.Frames[1].Status != FrameFailed || r.Frames[2].Status != FrameRequeued {
		t.Fatalf("%+v %+v", r.Frames, r.Process)
	}
}

// S04-A16: an owned helper terminated at frame k keeps completed frames and attributes the
// in-flight frame by the limit that ended it.
func TestFrameStatusMapping(t *testing.T) {
	frames := func(n int) []Frame {
		f := make([]Frame, n)
		for i := range f {
			f[i] = Frame{ID: fmt.Sprintf("file-%d", i), Request: fmt.Appendf(nil, "req-%d", i)}
		}
		return f
	}
	small := BatchPolicy{RequestBytes: 1024, ResponseBytes: 1024, FrameWall: 300 * time.Millisecond, FrameGrace: 200 * time.Millisecond,
		StdoutBytes: 1 << 20, BatchWall: 30 * time.Second}
	cases := []struct {
		name   string
		action string
		pol    func(*BatchPolicy)
		mem    uint64
		wallAt bool
		want   []string
		detail string
	}{
		{"all", "echo", nil, 0, false, []string{FrameCompleted, FrameCompleted, FrameCompleted, FrameCompleted}, ""},
		{"own-watchdog", "hang@2", nil, 0, false, []string{FrameCompleted, FrameCompleted, FrameResourceLimit, FrameRequeued}, ReasonFrameWatchdog},
		{"own-response-cap", "big@2", nil, 0, false, []string{FrameCompleted, FrameCompleted, FrameResourceLimit, FrameRequeued}, ReasonResponseBytes},
		// The frame watchdog outlasts the sampling interval so a sampled backend (macOS ps
		// RSS) cannot lose the race to it; the helper holds 1 GiB until it is ended.
		{"own-memory", "alloc@2", func(p *BatchPolicy) { p.FrameWall = 20 * time.Second }, 192 << 20,
			false, []string{FrameCompleted, FrameCompleted, FrameResourceLimit, FrameRequeued}, ReasonMemory},
		{"batch-stdout", "fat@2", func(p *BatchPolicy) { p.StdoutBytes = 500 }, 0, false, []string{FrameCompleted, FrameCompleted, FrameRequeued, FrameRequeued}, ReasonOutput},
		{"batch-wall", "hang@2", func(p *BatchPolicy) { p.FrameWall, p.BatchWall = 20*time.Second, 1500*time.Millisecond }, 0,
			true, []string{FrameCompleted, FrameCompleted, FrameRequeued, FrameRequeued}, ReasonWall},
		{"crash", "crash@2", nil, 0, false, []string{FrameCompleted, FrameCompleted, FrameFailed, FrameRequeued}, ReasonExited},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, pids := helperSpec(t, "frames:"+c.action)
			awaitStart(t, pids)
			if c.wallAt {
				// The batch wall expires only once frame 2 is in flight, after frames 0 and 1.
				testHookWall = func() {
					if !waitFor(func() bool { return exists(pids + ".at") }) {
						t.Error("frame 2 not reached before the batch wall")
					}
				}
				t.Cleanup(func() { testHookWall = nil })
			}
			if c.mem != 0 {
				s.Memory.Bytes = c.mem
			}
			pol := small
			if c.pol != nil {
				c.pol(&pol)
			}
			at := -1
			if c.detail == ReasonFrameWatchdog {
				at = 2
			}
			controlFrameTimers(t, pids, at, pol.FrameWall+pol.FrameGrace)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			r, err := RunBatch(ctx, s, pol, frames(4))
			if err != nil {
				t.Fatal(err)
			}
			for i, fr := range r.Frames {
				if fr.Status != c.want[i] {
					t.Fatalf("frame %d: %s (%s), want %s; process %+v", i, fr.Status, fr.Detail, c.want[i], r.Process)
				}
				if fr.Status == FrameCompleted && string(fr.Response) != fmt.Sprintf("ok:req-%d", i) {
					t.Fatalf("frame %d response %q", i, fr.Response)
				}
			}
			if c.detail != "" && r.Frames[2].Detail != c.detail {
				t.Fatalf("in-flight detail %q, want %q", r.Frames[2].Detail, c.detail)
			}
			if !r.Process.Cleanup.Verified {
				t.Fatalf("cleanup not verified: %+v", r.Process.Cleanup)
			}
			if r.TrailingBytes != 0 {
				t.Fatalf("trailing bytes %d", r.TrailingBytes)
			}
		})
	}
	// S05-A09: bytes after the last response are counted, never silently dropped.
	s, pids := helperSpec(t, "frames:tail")
	awaitStart(t, pids)
	controlFrameTimers(t, pids, -1, small.FrameWall+small.FrameGrace)
	r, err := RunBatch(context.Background(), s, small, frames(2))
	if err != nil || r.TrailingBytes != 4 || r.Frames[1].Status != FrameCompleted {
		t.Fatalf("trailing stdout: %v %d %+v", err, r.TrailingBytes, r.Frames)
	}
	t.Run("request-cap", func(t *testing.T) {
		s, pids := helperSpec(t, "frames:echo")
		awaitStart(t, pids)
		controlFrameTimers(t, pids, -1, small.FrameWall+small.FrameGrace)
		f := frames(3)
		f[1].Request = make([]byte, small.RequestBytes+1)
		r, err := RunBatch(context.Background(), s, small, f)
		if err != nil {
			t.Fatal(err)
		}
		if r.Frames[0].Status != FrameCompleted || r.Frames[1].Status != FrameResourceLimit || r.Frames[2].Status != FrameCompleted {
			t.Fatalf("%+v", r.Frames)
		}
	})
}
