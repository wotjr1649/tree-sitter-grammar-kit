package runner

import (
	"context"
	"fmt"
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
