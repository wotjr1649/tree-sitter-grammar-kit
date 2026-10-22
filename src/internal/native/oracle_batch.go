package native

import (
	"context"
	"errors"
	"os"
	"runtime"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// FrameTransport is the frame an r2 oracle record was sent in (#266): its batch, its index
// in that batch, SENT or ANSWERED, and the request-write-to-response-read time.
type FrameTransport struct {
	Batch           int    `json:"batch"`
	Frame           int    `json:"frame"`
	State           string `json:"state"`
	FrameWallMillis int64  `json:"frame_wall_ms"`
}

// fatalFrameCodes end the driver right after their typed response (exit 3), so it never
// reads the frame sent after them.
var fatalFrameCodes = map[string]bool{"ALLOCATION_LIMIT": true, "QUERY_TIME_LIMIT": true}

// unansweredFrame is the result of a sent frame without a complete response: its own
// limit (frame watchdog, response size, memory), the batch limit or cancellation that ended
// the process while it was in flight, or a crash or protocol break.
func unansweredFrame(fr runner.FrameResult) (status, assess, code string) {
	switch {
	case fr.Status == runner.FrameResourceLimit:
		return kit.StatusResourceLimit, kit.AssessBlocked, fr.Detail
	case fr.Status == runner.FrameRequeued && fr.Detail == runner.ReasonWall:
		return kit.StatusResourceLimit, kit.AssessBlocked, "BATCH_WALL_LIMIT"
	case fr.Status == runner.FrameRequeued && fr.Detail == runner.ReasonOutput:
		return kit.StatusResourceLimit, kit.AssessBlocked, "BATCH_OUTPUT_LIMIT"
	case fr.Status == runner.FrameRequeued && fr.Detail == runner.ReasonCancelled:
		return kit.StatusCancelled, kit.AssessNotAssessed, "CANCELLED"
	}
	return kit.StatusFailed, kit.AssessNotAssessed, "BATCH_FRAME_FAILED"
}

// runOracleBatches runs the prepared cases (index pending into cases, reqs and out) through
// batch driver processes of at most BatchFiles cases and BatchBytes framed request bytes,
// judging every answered frame like a single process response. A frame that was sent is
// never sent again: only frames a process never received move to a new process, and only
// after that process disposed of at least one frame with verified cleanup. Answered cases
// of a batch whose process is not kit.OracleBatch.Accepted are not accepted.
func (b *Build) runOracleBatches(ctx context.Context, x Context, cases []kit.IncrementalCase, reqs []caseRequest, out []CaseResult, pending []int) []kit.OracleBatch {
	batches := []kit.OracleBatch{}
	queue := pending
	stop := ""
	for len(queue) > 0 {
		if stop == "" && ctx.Err() != nil {
			stop = "CANCELLED"
		}
		deadline, bounded := ctx.Deadline()
		if stop == "" && bounded && time.Until(deadline) <= 0 {
			stop = "RUN_WALL_LIMIT"
		}
		if stop != "" {
			break
		}
		var idx []int
		var frames []runner.Frame
		var bytes, stdout uint64
		var walls time.Duration
		for len(queue) > 0 && len(idx) < x.Op.BatchFiles {
			i := queue[0]
			n := uint64(len(reqs[i].payload) + 4)
			if len(idx) > 0 && bytes+n > x.Op.BatchBytes {
				break
			}
			idx, frames, queue = append(idx, i), append(frames, runner.Frame{ID: cases[i].ID, Request: reqs[i].payload}), queue[1:]
			bytes, stdout, walls = bytes+n, stdout+x.Op.OutputBytes+4, walls+reqs[i].wall
		}
		if err := b.Verify(); err != nil {
			for _, i := range idx {
				finishEarly(&out[i], kit.StatusNotRun, kit.AssessBlocked, "EXECUTABLE_MISMATCH")
			}
			stop = "EXECUTABLE_MISMATCH"
			break
		}
		wall := min(x.Op.BatchWall, walls) // no more than its cases' own walls together
		if bounded {
			wall = min(wall, time.Until(deadline))
		}
		pol := runner.BatchPolicy{RequestBytes: MaxRequestFrame, ResponseBytes: uint32(x.Op.OutputBytes), FrameWall: x.Op.FrameWall, FrameGrace: 5 * time.Second,
			StdoutBytes: int64(stdout), BatchWall: wall}
		spec := runner.Spec{Path: b.Executable, Args: []string{"batch"}, Dir: b.Dir, StderrBytes: 65536, Grace: 5 * time.Second,
			Memory: runner.Memory{Bytes: x.Op.MemoryBytes, Hard: runtime.GOOS != "darwin"}, CgroupParent: x.CgroupParent}
		br, err := runner.RunBatch(ctx, spec, pol, frames)
		if err != nil {
			code := "BATCH_START_FAILED"
			var re *runner.Error
			if errors.As(err, &re) {
				code = re.Code
			}
			for _, i := range idx {
				finishEarly(&out[i], kit.StatusNotRun, kit.AssessBlocked, code)
			}
			stop = code
			break
		}
		ob := kit.OracleBatch{Index: len(batches), Status: br.Process.Status, Reason: br.Process.Reason, ExitCode: max(br.Process.ExitCode, 0),
			WallMillis: br.Process.WallMillis, PeakBytes: br.Process.Memory.Peak, CleanupVerified: br.Process.Cleanup.Verified, TrailingBytes: br.TrailingBytes}
		var requeue, answered []int
		fatal, progress, unanswered := false, false, false
		for k, fr := range br.Frames {
			i := idx[k]
			ob.Cases = append(ob.Cases, cases[i].ID)
			if fatal || fr.Status == runner.FrameRequeued && fr.Detail == "not sent" {
				requeue = append(requeue, i) // never read by a driver
				continue
			}
			c := &out[i]
			c.Transport, progress = &FrameTransport{Batch: ob.Index, Frame: k, State: "SENT", FrameWallMillis: fr.WallMillis}, true
			switch fr.Status {
			case runner.FrameCompleted:
				c.Transport.State = "ANSWERED"
				b.judgeFrame(x, cases[i], reqs[i], fr.Response, nil, c)
				answered = append(answered, i)
				fatal = c.ResponseStatus == kit.StatusResourceLimit && fatalFrameCodes[c.ResponseCode]
			default:
				status, assess, code := unansweredFrame(fr)
				finishEarly(c, status, assess, code)
				unanswered = true
			}
		}
		if !ob.Accepted() || unanswered {
			for _, i := range answered {
				// a completed FAIL stays: a failure is never a support claim
				if out[i].ExecutionStatus == kit.StatusCompleted && out[i].Assessment != kit.AssessFail {
					// the response stays as diagnostic raw; nothing of it is claimed
					raw, tr := out[i].Raw, out[i].Transport
					out[i] = newCaseResult(cases[i])
					out[i].Raw, out[i].Transport = raw, tr
					finishEarly(&out[i], kit.StatusFailed, kit.AssessNotAssessed, "BATCH_PROCESS_FAILED")
				}
			}
		}
		batches = append(batches, ob)
		switch {
		case !ob.CleanupVerified:
			stop = "PROCESS_CLEANUP_UNVERIFIED"
		case !progress && len(requeue) > 0:
			stop = "BATCH_NO_PROGRESS"
		}
		queue = append(requeue, queue...)
	}
	for _, i := range queue {
		finishEarly(&out[i], kit.StatusNotRun, kit.AssessBlocked, stop)
	}
	return batches
}

// oracleBatch reads and prepares every case of a batch set and runs the prepared ones; it
// returns each case's result before the oracle judgement and the source of each case read.
func (b *Build) oracleBatch(ctx context.Context, x Context, cases []kit.IncrementalCase, root string, handle *os.Root) ([]CaseResult, [][]byte, []kit.OracleBatch) {
	x.ObservationProducerSHA256 = b.Identity
	out := make([]CaseResult, len(cases))
	sources := make([][]byte, len(cases))
	reqs := make([]caseRequest, len(cases))
	var pending []int
	for i, c := range cases {
		out[i] = newCaseResult(c)
		src, err := readCase(root, c.Input, handle)
		if err != nil {
			finishEarly(&out[i], kit.StatusNotRun, kit.AssessNotAssessed, err.(*Error).Code)
			continue
		}
		sources[i] = src
		var done bool
		if reqs[i], done = b.prepareCase(ctx, x, c, src, &out[i]); !done {
			pending = append(pending, i)
		}
	}
	return out, sources, b.runOracleBatches(ctx, x, cases, reqs, out, pending)
}
