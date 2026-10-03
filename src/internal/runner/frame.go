package runner

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"time"
)

// Frame statuses. A frame keeps its completed response; the in-flight frame is
// RESOURCE_LIMIT for its own limit (watchdog, response size, memory), REQUEUED for a
// batch-level limit (batch wall, batch stdout) or caller cancellation, and FAILED when the
// process ends on its own mid-frame (crash or protocol violation). Unsent frames are
// REQUEUED.
const (
	FrameCompleted     = "COMPLETED"
	FrameResourceLimit = "RESOURCE_LIMIT"
	FrameRequeued      = "REQUEUED"
	FrameFailed        = "FAILED"
)

// Frame-level termination reasons recorded on the process result.
const (
	ReasonFrameWatchdog = "FRAME_WATCHDOG"
	ReasonResponseBytes = "RESPONSE_BYTES_LIMIT"
	ReasonBatchExit     = "BATCH_EXIT_TIMEOUT"
)

// BatchPolicy bounds the generic length-prefixed frame envelope. Each frame on stdin and
// stdout is a 4-byte big-endian length followed by that many payload bytes.
type BatchPolicy struct {
	RequestBytes  uint32        // per request frame payload
	ResponseBytes uint32        // per response frame payload
	FrameWall     time.Duration // driver-side per-frame budget (S05 progress callback)
	FrameGrace    time.Duration // runner watchdog margin beyond FrameWall
	StdoutBytes   int64         // whole-batch stdout, headers included
	BatchWall     time.Duration // whole-batch process wall
}

// DefaultBatchPolicy is the adopted real-world resource policy (S04-A16): 48 MiB request
// and 16 MiB response per frame, 60 s + 5 s per-frame watchdog, 256 MiB batch stdout
// (coordinator-derived) and a 3600 s batch process wall.
func DefaultBatchPolicy() BatchPolicy {
	return BatchPolicy{RequestBytes: 50331648, ResponseBytes: 16777216, FrameWall: 60 * time.Second, FrameGrace: 5 * time.Second,
		StdoutBytes: 268435456, BatchWall: 3600 * time.Second}
}

// Frame is one request of a batch.
type Frame struct {
	ID      string
	Request []byte
}

// FrameResult is the outcome of one frame.
type FrameResult struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Response []byte `json:"-"`
}

// BatchResult keeps every frame outcome and the supervised process result.
type BatchResult struct {
	Frames  []FrameResult `json:"frames"`
	Process Result        `json:"process"`
}

// RunBatch sends frames one at a time to a single supervised process and reads one
// response frame per request. spec's Wall, StdoutBytes and Interactive come from pol.
func RunBatch(ctx context.Context, spec Spec, pol BatchPolicy, frames []Frame) (BatchResult, error) {
	if pol.RequestBytes == 0 || pol.ResponseBytes == 0 || pol.FrameWall <= 0 || pol.FrameGrace <= 0 || pol.StdoutBytes <= 0 || pol.BatchWall <= 0 {
		return BatchResult{}, blocked("LIMIT_INVALID", errors.New("every batch limit must be positive"))
	}
	spec.Wall, spec.StdoutBytes, spec.Interactive, spec.Stdin = pol.BatchWall, pol.StdoutBytes, true, nil
	p, err := Start(ctx, spec)
	if err != nil {
		return BatchResult{}, err
	}
	out := BatchResult{Frames: make([]FrameResult, len(frames))}
	for i, f := range frames {
		out.Frames[i] = FrameResult{ID: f.ID, Status: FrameRequeued, Detail: "not sent"}
	}
	inflight := -1
	for i, f := range frames {
		if uint64(len(f.Request)) > uint64(pol.RequestBytes) {
			out.Frames[i] = FrameResult{ID: f.ID, Status: FrameResourceLimit, Detail: "REQUEST_BYTES_LIMIT"}
			continue
		}
		inflight = i
		resp, err := exchange(p, pol, f.Request)
		if err != nil {
			break
		}
		out.Frames[i] = FrameResult{ID: f.ID, Status: FrameCompleted, Response: resp}
		inflight = -1
	}
	// A process that neither exits nor stops writing after its last frame is ended.
	p.Stdin().Close()
	linger := time.AfterFunc(pol.FrameGrace, func() { p.Terminate(ReasonBatchExit) })
	out.Process = p.Wait()
	linger.Stop()
	if inflight >= 0 {
		fr := &out.Frames[inflight]
		fr.Detail = out.Process.Reason
		switch out.Process.Reason {
		case ReasonFrameWatchdog, ReasonResponseBytes, ReasonMemory:
			fr.Status = FrameResourceLimit
		case ReasonWall, ReasonOutput, ReasonCancelled:
			fr.Status = FrameRequeued
		default: // the process ended or broke the protocol on its own
			fr.Status = FrameFailed
		}
	}
	return out, nil
}

// exchange writes one request frame and reads its response under the frame watchdog.
func exchange(p *Process, pol BatchPolicy, req []byte) ([]byte, error) {
	watchdog := time.AfterFunc(pol.FrameWall+pol.FrameGrace, func() { p.Terminate(ReasonFrameWatchdog) })
	defer watchdog.Stop()
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(req)))
	if _, err := p.Stdin().Write(hdr[:]); err != nil {
		return nil, err
	}
	if _, err := p.Stdin().Write(req); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(p.Stdout(), hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > pol.ResponseBytes {
		p.Terminate(ReasonResponseBytes)
		return nil, errors.New("response frame over limit")
	}
	resp := make([]byte, n)
	if _, err := io.ReadFull(p.Stdout(), resp); err != nil {
		return nil, err
	}
	return resp, nil
}
