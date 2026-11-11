package native

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func captureFrame(payload []byte) []byte {
	out := make([]byte, 4, 4+len(payload))
	binary.BigEndian.PutUint32(out, uint32(len(payload)))
	return append(out, payload...)
}

func syntheticBatchCapture(t *testing.T) *batchCapture {
	t.Helper()
	frames := []runner.Frame{{ID: "a", Request: []byte(" { \"id\" : \"a\" } \n")}, {ID: "b", Request: []byte("{\"id\":\"b\"}")}}
	c := newBatchCapture(3, runner.Spec{Path: "driver", Args: []string{"batch"}, Dir: "work", StderrBytes: 65536, Grace: time.Second}, runner.DefaultBatchPolicy(), frames)
	c.result.Process = runner.Result{Status: runner.StatusCompleted, Reason: runner.ReasonExited, Cleanup: runner.Cleanup{Verified: true}, Stderr: []byte("diagnostic\n"), StderrBytes: 11}
	for _, f := range frames {
		response := []byte(fmt.Sprintf("{ \"id\" : %q }\n", f.ID))
		c.capture("stdin", captureFrame(f.Request))
		c.capture("stdout", captureFrame(response))
		c.result.Frames = append(c.result.Frames, runner.FrameResult{ID: f.ID, Status: runner.FrameCompleted, Response: response})
	}
	c.result.Process.StdoutBytes = int64(c.stdout.n)
	return c
}

func capturedWire(t *testing.T, c *batchCapture) (batchCaptureWire, []captureFile) {
	t.Helper()
	files, err := c.files("capture/batch-00003")
	if err != nil {
		t.Fatal(err)
	}
	var w batchCaptureWire
	if err := json.Unmarshal(files[len(files)-1].data, &w); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.pin.Bytes != uint64(len(f.data)) || f.pin.SHA256 != sha(f.data) || len(f.data) > kit.MaxDocumentBytes {
			t.Fatal("invalid physical member")
		}
	}
	return w, files
}

func TestBatchCaptureActualFrames(t *testing.T) {
	c := syntheticBatchCapture(t)
	w, files := capturedWire(t, c)
	if w.Schema != batchCaptureSchema || w.Owner != "D_batch_00003" || !w.Accepted || w.Code != "" || !w.Spec.Interactive || w.Spec.Stdin != nil || w.Stdin.SHA256 != sha(c.stdin.data) || w.Stdout.SHA256 != sha(c.stdout.data) {
		t.Fatalf("transcript %+v", w)
	}
	var stdin, stdout []byte
	processes, stderr := 0, 0
	for _, f := range files {
		switch f.pin.Role {
		case "stdin-frame":
			stdin = append(stdin, f.data...)
		case "stdout-frame":
			stdout = append(stdout, f.data...)
		case "process":
			processes++
		case "stderr":
			stderr++
		}
	}
	if !bytes.Equal(stdin, c.stdin.data) || !bytes.Equal(stdout, c.stdout.data) || processes != 1 || stderr != 1 {
		t.Fatal("actual bytes or unique process ownership lost")
	}
	for i, f := range w.Frames {
		if f.Index != i || f.Case != c.frames[i].ID || f.State != "ANSWERED" || !f.Stdin.Complete || !f.Stdout.Complete {
			t.Fatalf("frame %+v", f)
		}
		if i == 1 && (f.Stdin.Offset != w.Frames[0].Stdin.Bytes || f.Stdout.Offset != w.Frames[0].Stdout.Bytes) {
			t.Fatal("frame offsets")
		}
	}
}

func TestBatchCaptureBindingControls(t *testing.T) {
	for name, mutate := range map[string]func(*batchCapture){
		"frame-index": func(c *batchCapture) {
			c.result.Frames[0].ID, c.result.Frames[1].ID = c.result.Frames[1].ID, c.result.Frames[0].ID
		},
		"response-exchange": func(c *batchCapture) { c.result.Frames[0], c.result.Frames[1] = c.result.Frames[1], c.result.Frames[0] },
		"case-id":           func(c *batchCapture) { c.result.Frames[1].ID = "a" },
		"same-length-stdin": func(c *batchCapture) {
			c.frames[0].Request = bytes.ReplaceAll(c.frames[0].Request, []byte("a"), []byte("z"))
		},
		"same-length-response": func(c *batchCapture) {
			c.result.Frames[0].Response = bytes.ReplaceAll(c.result.Frames[0].Response, []byte("a"), []byte("z"))
		},
		"request-header":           func(c *batchCapture) { c.stdin.data[3]++; c.stdin.h.Reset(); c.stdin.h.Write(c.stdin.data) },
		"response-header":          func(c *batchCapture) { c.stdout.data[3]++; c.stdout.h.Reset(); c.stdout.h.Write(c.stdout.data) },
		"actual-stream-hash":       func(c *batchCapture) { c.stdout.data[5]++ },
		"process-stdout-count":     func(c *batchCapture) { c.result.Process.StdoutBytes++ },
		"stderr-truncation-hidden": func(c *batchCapture) { c.result.Process.StderrBytes++ },
		"stdin-suffix":             func(c *batchCapture) { c.capture("stdin", []byte("x")) },
	} {
		t.Run(name, func(t *testing.T) {
			c := syntheticBatchCapture(t)
			mutate(c)
			if _, err := c.files("capture/batch"); err == nil {
				t.Fatal("accepted binding mutation")
			}
		})
	}
}

func TestBatchCapturePartialAndProcessFailure(t *testing.T) {
	for _, n := range []int{0, 1, 3, 4, 7} {
		t.Run(fmt.Sprintf("response-prefix-%d", n), func(t *testing.T) {
			c := syntheticBatchCapture(t)
			first := 4 + len(c.result.Frames[0].Response)
			c.stdout.data = c.stdout.data[:first+n]
			c.stdout.h.Reset()
			c.stdout.h.Write(c.stdout.data)
			c.stdout.n = uint64(len(c.stdout.data))
			c.result.Process.StdoutBytes = int64(c.stdout.n)
			c.result.Frames[1].Status, c.result.Frames[1].Response = runner.FrameFailed, nil
			c.frames = append(c.frames, runner.Frame{ID: "tail", Request: []byte("unwritten")})
			c.result.Frames = append(c.result.Frames, runner.FrameResult{ID: "tail", Status: runner.FrameRequeued, Detail: "not sent"})
			w, _ := capturedWire(t, c)
			if w.Accepted || w.Frames[0].State != "ANSWERED" || w.Frames[1].State != "SENT" || w.Frames[2].State != "NOT_SENT" || w.Frames[2].Stdin != nil || n == 0 && w.Frames[1].Stdout != nil || n > 0 && (w.Frames[1].Stdout.Complete || w.Frames[1].Stdout.Bytes != uint64(n)) {
				t.Fatalf("partial transcript %+v", w)
			}
		})
	}
	for _, failure := range []string{"cleanup", "stdout-truncated", "stderr-truncated", "batch-cap", "capture", "trailing"} {
		t.Run(failure, func(t *testing.T) {
			c := syntheticBatchCapture(t)
			switch failure {
			case "cleanup":
				c.result.Process.Cleanup.Verified = false
			case "stdout-truncated":
				c.result.Process.StdoutTruncated = true
			case "stderr-truncated":
				c.result.Process.StderrTruncated = true
			case "batch-cap":
				c.result.Process.Status, c.result.Process.Reason = runner.StatusResourceLimit, runner.ReasonOutput
			case "capture":
				c.result.CaptureError = "sink failed"
			case "trailing":
				c.capture("stdout", []byte("tail"))
				c.result.Process.StdoutBytes += 4
				c.result.TrailingBytes = 4
			}
			w, _ := capturedWire(t, c)
			if w.Accepted || w.Frames[0].State != "ANSWERED" || c.result.Frames[0].Status != runner.FrameCompleted {
				t.Fatal("batch failure changed an answered frame or was accepted")
			}
		})
	}
}

func TestBatchCaptureMemberLimit(t *testing.T) {
	c := syntheticBatchCapture(t)
	// A header counts toward the physical file cap; fragments are failure diagnostics.
	c.frames[1].Request = bytes.Repeat([]byte("x"), kit.MaxDocumentBytes-3)
	c.stdin.data = nil
	c.stdin.n = 0
	c.stdin.h.Reset()
	for _, f := range c.frames {
		c.capture("stdin", captureFrame(f.Request))
	}
	w, _ := capturedWire(t, c)
	if w.Accepted || w.Code != "BATCH_CAPTURE_MEMBER_LIMIT" || len(w.Frames[1].Stdin.Members) != 2 {
		t.Fatal("oversized frame admitted")
	}
	c = syntheticBatchCapture(t)
	c.result.Process.Cleanup.Detail = strings.Repeat("x", kit.MaxDocumentBytes)
	if _, err := c.files("capture/batch"); err == nil {
		t.Fatal("oversized process metadata emitted")
	}
}

func TestBatchCaptureRetainedPrefix(t *testing.T) {
	c := syntheticBatchCapture(t)
	actualHash, actualBytes := c.stdout.pin().SHA256, c.stdout.n
	c.stdout.data = c.stdout.data[:5]
	c.result.CaptureError = "BATCH_CAPTURE_BYTES_LIMIT"
	w, _ := capturedWire(t, c)
	if w.Accepted || w.CaptureError == "" || w.Stdout.Bytes != actualBytes || w.Stdout.SHA256 != actualHash || w.Stdout.RetainedBytes != 5 || w.Frames[0].State != "ANSWERED" || w.Frames[0].Stdout.Complete || w.Frames[1].State != "ANSWERED" || w.Frames[1].Stdout != nil {
		t.Fatalf("lost retention failure %+v", w)
	}
}

func TestBatchCapturePartialStdin(t *testing.T) {
	for _, n := range []int{0, 1, 3, 4, 6} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			c := syntheticBatchCapture(t)
			first := 4 + len(c.frames[0].Request)
			c.stdin.data = c.stdin.data[:first+n]
			c.stdin.h.Reset()
			c.stdin.h.Write(c.stdin.data)
			c.stdin.n = uint64(len(c.stdin.data))
			c.stdout.data = c.stdout.data[:4+len(c.result.Frames[0].Response)]
			c.stdout.h.Reset()
			c.stdout.h.Write(c.stdout.data)
			c.stdout.n = uint64(len(c.stdout.data))
			c.result.Process.StdoutBytes = int64(c.stdout.n)
			c.result.Frames[1].Status, c.result.Frames[1].Response = runner.FrameFailed, nil
			w, _ := capturedWire(t, c)
			if w.Accepted || w.Frames[0].State != "ANSWERED" || w.Frames[1].State != "SENT" || n == 0 && w.Frames[1].Stdin != nil || n > 0 && (w.Frames[1].Stdin.Complete || w.Frames[1].Stdin.Bytes != uint64(n)) {
				t.Fatalf("partial write %+v", w)
			}
		})
	}
}

func TestBatchCaptureNative(t *testing.T) {
	b := fixtureBuild(t, "plain")
	x := oracleContext(true, "(identifier) @id\n")
	x.Op, x.Protocol = kit.NativeOperations()["native-query-batch"], ProtocolR4
	x.PolicyRef, x.ObservationRunID, x.ObservationProducerSHA256 = PolicyRef(x.Op, x.Output), strings.Repeat("a", 32), b.Identity
	var cases []kit.IncrementalCase
	var reqs []caseRequest
	var out []CaseResult
	for i, src := range []string{"a = 1;\n", "b = 2;\n"} {
		c := kit.IncrementalCase{ID: fmt.Sprintf("c%d", i), Encoding: kit.EncodingUTF8}
		if i == 0 {
			c.Edits = editsFor(src, [3]any{4, 5, "3"}, [3]any{4, 5, "4"})
		}
		r := newCaseResult(c)
		q, done := b.prepareCase(context.Background(), x, c, []byte(src), &r)
		if done {
			t.Fatalf("prepare %s: %s", c.ID, r.Code)
		}
		cases, reqs, out = append(cases, c), append(reqs, q), append(out, r)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	batches, captures, err := b.runOracleBatchesCaptured(ctx, x, cases, reqs, out, []int{0, 1})
	if err != nil || len(batches) != 1 || !batches[0].Accepted() || len(captures) != 1 {
		t.Fatalf("%v %+v", err, batches)
	}
	w, _ := capturedWire(t, &captures[0])
	if !w.Accepted || len(w.Frames) != 2 || len(out[0].Steps) != 3 || out[0].Steps[2].Fresh == nil || out[0].Steps[2].Incremental.API == nil {
		t.Fatalf("native capture %+v, case %s %s", w, out[0].ExecutionStatus, out[0].Code)
	}
	for i, f := range w.Frames {
		if !bytes.Equal(captures[0].stdout.data[f.Stdout.Offset+4:f.Stdout.Offset+f.Stdout.Bytes], out[i].Raw) {
			t.Fatal("raw response differs from actual frame")
		}
	}
}

func TestBatchCaptureNativeFatalDoesNotResend(t *testing.T) {
	b := fixtureBuild(t, "plain")
	x := oracleContext(true, "(identifier) @id\n")
	x.Op, x.Protocol = kit.NativeOperations()["native-query-batch"], ProtocolR4
	x.PolicyRef, x.ObservationRunID, x.ObservationProducerSHA256 = PolicyRef(x.Op, x.Output), strings.Repeat("b", 32), b.Identity
	var cases []kit.IncrementalCase
	var reqs []caseRequest
	var out []CaseResult
	for i := range 3 {
		c := kit.IncrementalCase{ID: fmt.Sprintf("c%d", i), Encoding: kit.EncodingUTF8}
		r := newCaseResult(c)
		q, done := b.prepareCase(context.Background(), x, c, []byte("a = 1;\n"), &r)
		if done {
			t.Fatal(r.Code)
		}
		if i == 1 {
			q.req.Limits.MemoryBytes = 4096
			q.payload = q.req.Encode()
		}
		cases, reqs, out = append(cases, c), append(reqs, q), append(out, r)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	batches, captures, err := b.runOracleBatchesCaptured(ctx, x, cases, reqs, out, []int{0, 1, 2})
	if err != nil || len(batches) != 1 || len(captures) != 1 {
		t.Fatalf("attempted frame resent: %v %+v", err, batches)
	}
	w, _ := capturedWire(t, &captures[0])
	if w.Accepted || w.Frames[0].State != "ANSWERED" || w.Frames[1].State != "ANSWERED" || w.Frames[2].State != "SENT" || out[1].Code != "ALLOCATION_LIMIT" || out[2].Transport == nil || out[2].Transport.Batch != 0 || out[2].Transport.Frame != 2 || out[2].Transport.State != "SENT" {
		t.Fatalf("fatal transcript %+v, cases %+v", w, out)
	}
	if out[0].ExecutionStatus != kit.StatusCompleted || out[0].Assessment != kit.AssessPass || len(out[0].Steps) != 1 || out[0].Raw == nil {
		t.Fatal("batch failure overwrote an answered case's base observations")
	}
}

func TestBatchCaptureScope(t *testing.T) {
	for _, x := range []Context{oracleContext(true), {Op: kit.NativeOperations()["native-query-batch"], Protocol: ProtocolR4, Output: kit.OutputTree, API: false}} {
		if _, _, err := (*Build)(nil).runOracleBatchesCaptured(context.Background(), x, nil, nil, nil, nil); err == nil {
			t.Fatal("nonordinary capture admitted")
		}
	}
	x := oracleContext(true)
	x.Op, x.Protocol = kit.NativeOperations()["native-query-batch"], ProtocolR4
	if _, _, err := (*Build)(nil).runOracleBatchesCaptured(context.Background(), x, nil, []caseRequest{{payload: make([]byte, MaxRequestFrame+1)}}, nil, []int{0}); err == nil {
		t.Fatal("oversized request passed capture preflight")
	}
}
