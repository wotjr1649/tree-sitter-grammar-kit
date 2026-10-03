package runner

import (
	"context"
	"errors"
	"fmt"
	"runtime"
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

// R1-06: a frame hung while an escaped descendant holds stdout still ends within the
// declared bounds on every backend.
func TestFrameEscapedStdoutHolder(t *testing.T) {
	s, pids := helperSpec(t, "frames:escape@1")
	s.Grace = time.Second
	pol := BatchPolicy{RequestBytes: 1024, ResponseBytes: 1024, FrameWall: 300 * time.Millisecond, FrameGrace: 200 * time.Millisecond,
		StdoutBytes: 1 << 20, BatchWall: 30 * time.Second}
	start := time.Now()
	r, err := RunBatch(context.Background(), s, pol, []Frame{{ID: "a", Request: []byte("x")}, {ID: "b", Request: []byte("y")}, {ID: "c", Request: []byte("z")}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, pid := range readPIDs(t, pids) {
			killPID(pid)
		}
	}()
	if el := time.Since(start); el > 15*time.Second {
		t.Fatalf("batch took %v", el)
	}
	if r.Frames[0].Status != FrameCompleted || r.Frames[1].Status != FrameResourceLimit || r.Frames[2].Status != FrameRequeued {
		t.Fatalf("%+v", r.Frames)
	}
}

// R1-06: a helper that crashes mid-frame while an escaped descendant holds stdout gives
// FAILED for that frame within bounds, without any limit firing first.
func TestFrameCrashWithStdoutHolder(t *testing.T) {
	s, pids := helperSpec(t, "frames:crash-holder@1")
	s.Grace = time.Second
	pol := BatchPolicy{RequestBytes: 1024, ResponseBytes: 1024, FrameWall: 20 * time.Second, FrameGrace: time.Second,
		StdoutBytes: 1 << 20, BatchWall: 60 * time.Second}
	start := time.Now()
	r, err := RunBatch(context.Background(), s, pol, []Frame{{ID: "a", Request: []byte("x")}, {ID: "b", Request: []byte("y")}, {ID: "c", Request: []byte("z")}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, pid := range readPIDs(t, pids) {
			killPID(pid)
		}
	}()
	if el := time.Since(start); el > 10*time.Second {
		t.Fatalf("batch took %v; the crash was not released before the watchdog", el)
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
		want   []string
		detail string
	}{
		{"all", "echo", nil, 0, []string{FrameCompleted, FrameCompleted, FrameCompleted, FrameCompleted}, ""},
		{"own-watchdog", "hang@2", nil, 0, []string{FrameCompleted, FrameCompleted, FrameResourceLimit, FrameRequeued}, ReasonFrameWatchdog},
		{"own-response-cap", "big@2", nil, 0, []string{FrameCompleted, FrameCompleted, FrameResourceLimit, FrameRequeued}, ReasonResponseBytes},
		{"own-memory", "alloc@2", nil, 192 << 20, []string{FrameCompleted, FrameCompleted, FrameResourceLimit, FrameRequeued}, ReasonMemory},
		{"batch-stdout", "fat@2", func(p *BatchPolicy) { p.StdoutBytes = 500 }, 0, []string{FrameCompleted, FrameCompleted, FrameRequeued, FrameRequeued}, ReasonOutput},
		{"batch-wall", "hang@2", func(p *BatchPolicy) { p.FrameWall, p.BatchWall = 20*time.Second, 1500*time.Millisecond }, 0,
			[]string{FrameCompleted, FrameCompleted, FrameRequeued, FrameRequeued}, ReasonWall},
		{"crash", "crash@2", nil, 0, []string{FrameCompleted, FrameCompleted, FrameFailed, FrameRequeued}, ReasonExited},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := helperSpec(t, "frames:"+c.action)
			if c.mem != 0 {
				s.Memory.Bytes = c.mem
			}
			pol := small
			if c.pol != nil {
				c.pol(&pol)
			}
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
		})
	}
	t.Run("request-cap", func(t *testing.T) {
		s, _ := helperSpec(t, "frames:echo")
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
