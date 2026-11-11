package native

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const batchCaptureSchema = "tsgk-native-batch-capture/r1"

// shortcut: retain bounded stream prefixes in memory; stream to files when ordinary publication is wired.
type batchCaptureStream struct {
	data []byte
	n    uint64
	h    hash.Hash
}

type batchCapture struct {
	index            int
	spec             runner.Spec
	frames           []runner.Frame
	result           runner.BatchResult
	stdin, stdout    batchCaptureStream
	buildIdentity    string
	executableSHA256 string
	executableBytes  int64
}

type batchCaptureSpan struct {
	Offset   uint64             `json:"offset"`
	Bytes    uint64             `json:"bytes"`
	SHA256   string             `json:"sha256"`
	Complete bool               `json:"complete"`
	Members  []kit.ReplayMember `json:"members"`
}

type batchCaptureFrame struct {
	Index      int               `json:"index"`
	Case       string            `json:"case_id"`
	State      string            `json:"state"`
	Status     string            `json:"status"`
	Detail     string            `json:"detail"`
	WallMillis int64             `json:"wall_ms"`
	Stdin      *batchCaptureSpan `json:"stdin"`
	Stdout     *batchCaptureSpan `json:"stdout"`
}

type batchCaptureStreamPin struct {
	Bytes         uint64 `json:"bytes"`
	SHA256        string `json:"sha256"`
	RetainedBytes uint64 `json:"retained_bytes"`
}

// Owner is the sole process owner; cases only refer to its frame indexes.
type batchCaptureWire struct {
	Schema           string                `json:"schema"`
	Owner            string                `json:"owner"`
	Batch            int                   `json:"batch"`
	BuildIdentity    string                `json:"build_identity"`
	ExecutableSHA256 string                `json:"executable_sha256"`
	ExecutableBytes  int64                 `json:"executable_bytes"`
	Spec             captureSpec           `json:"spec"`
	Process          kit.ReplayMember      `json:"process"`
	Stderr           kit.ReplayMember      `json:"stderr"`
	Stdin            batchCaptureStreamPin `json:"stdin"`
	Stdout           batchCaptureStreamPin `json:"stdout"`
	Frames           []batchCaptureFrame   `json:"frames"`
	Suffix           *batchCaptureSpan     `json:"stdout_suffix"`
	TrailingBytes    int64                 `json:"trailing_bytes"`
	Accepted         bool                  `json:"accepted"`
	Code             string                `json:"code"`
	CaptureError     string                `json:"capture_error"`
}

func newBatchCapture(index int, spec runner.Spec, pol runner.BatchPolicy, frames []runner.Frame) *batchCapture {
	spec.Wall, spec.StdoutBytes, spec.Interactive, spec.Stdin = pol.BatchWall, pol.StdoutBytes, true, nil
	c := &batchCapture{index: index, spec: spec, frames: slices.Clone(frames)}
	c.stdin.h, c.stdout.h = sha256.New(), sha256.New()
	return c
}

func (c *batchCapture) capture(direction string, data []byte) error {
	s := &c.stdin
	if direction == "stdout" {
		s = &c.stdout
	} else if direction != "stdin" {
		return fmt.Errorf("unknown batch capture stream")
	}
	s.n += uint64(len(data))
	s.h.Write(data)
	// Reserve both metadata members' file caps and stderr within the existing physical limit.
	room := int(kit.ReplayOperations()["evidence-replay"].TotalBytes) - 2*kit.MaxDocumentBytes - 65536 - len(c.stdin.data) - len(c.stdout.data)
	s.data = append(s.data, data[:min(len(data), max(0, room))]...)
	if room < len(data) {
		return fmt.Errorf("BATCH_CAPTURE_BYTES_LIMIT")
	}
	return nil
}

func (s *batchCaptureStream) pin() batchCaptureStreamPin {
	return batchCaptureStreamPin{Bytes: s.n, SHA256: hex.EncodeToString(s.h.Sum(nil)), RetainedBytes: uint64(len(s.data))}
}

// files validates the transport binding and preserves actual bytes, including failed suffixes.
// Oversized frames are diagnostic fragments only; they never become ordinary small evidence.
func (c *batchCapture) files(prefix string) ([]captureFile, error) {
	if c.index < 0 || len(c.frames) != len(c.result.Frames) || c.result.Process.StdoutBytes < 0 || uint64(c.result.Process.StdoutBytes) != c.stdout.n {
		return nil, fmt.Errorf("BATCH_CAPTURE_BINDING")
	}
	p := c.result.Process
	if c.stdin.n == uint64(len(c.stdin.data)) && sha(c.stdin.data) != c.stdin.pin().SHA256 || c.stdout.n == uint64(len(c.stdout.data)) && sha(c.stdout.data) != c.stdout.pin().SHA256 {
		return nil, fmt.Errorf("BATCH_CAPTURE_STREAM_HASH")
	}
	if p.StderrBytes < int64(len(p.Stderr)) || (!p.StderrTruncated && p.StderrBytes != int64(len(p.Stderr))) || int64(len(p.Stderr)) > c.spec.StderrBytes {
		return nil, fmt.Errorf("BATCH_CAPTURE_STDERR")
	}
	w := batchCaptureWire{Schema: batchCaptureSchema, Owner: fmt.Sprintf("D_batch_%05d", c.index), Batch: c.index,
		BuildIdentity: c.buildIdentity, ExecutableSHA256: c.executableSHA256, ExecutableBytes: c.executableBytes, CaptureError: c.result.CaptureError,
		Spec: captureSpec{Path: c.spec.Path, Args: c.spec.Args, Dir: c.spec.Dir, Env: c.spec.Env, Interactive: true,
			StdoutBytes: c.spec.StdoutBytes, StderrBytes: c.spec.StderrBytes, Wall: c.spec.Wall, Grace: c.spec.Grace, Memory: c.spec.Memory, CgroupParent: c.spec.CgroupParent},
		Stdin: c.stdin.pin(), Stdout: c.stdout.pin(), Frames: []batchCaptureFrame{}, TrailingBytes: c.result.TrailingBytes,
		Accepted: p.Succeeded() && !p.StdoutTruncated && !p.StderrTruncated && c.result.TrailingBytes == 0}
	var files []captureFile
	add := func(path, role string, data []byte) kit.ReplayMember {
		pin := kit.ReplayMember{Path: prefix + path, Role: role, Bytes: uint64(len(data)), SHA256: sha(data)}
		files = append(files, captureFile{pin: pin, data: slices.Clone(data)})
		return pin
	}
	block := func(code string) {
		w.Accepted = false
		if w.Code == "" {
			w.Code = code
		}
	}
	if c.stdin.n != uint64(len(c.stdin.data)) || c.stdout.n != uint64(len(c.stdout.data)) || c.result.CaptureError != "" {
		block("BATCH_CAPTURE_BYTES_LIMIT")
	}
	span := func(direction string, index int, offset int, data []byte, complete bool) *batchCaptureSpan {
		if len(data) == 0 {
			return nil
		}
		s := &batchCaptureSpan{Offset: uint64(offset), Bytes: uint64(len(data)), SHA256: sha(data), Complete: complete, Members: []kit.ReplayMember{}}
		if len(data) > kit.MaxDocumentBytes {
			block("BATCH_CAPTURE_MEMBER_LIMIT")
		}
		for part, start := 0, 0; start < len(data); part++ {
			end := min(start+kit.MaxDocumentBytes, len(data))
			s.Members = append(s.Members, add(fmt.Sprintf("-frame-%05d-%s-%03d.bin", index, direction, part), direction+"-frame", data[start:end]))
			start = end
		}
		return s
	}
	requestOffset, responseOffset := 0, 0
	ended := false
	for i, fr := range c.result.Frames {
		if fr.ID != c.frames[i].ID {
			return nil, fmt.Errorf("BATCH_CAPTURE_FRAME_BINDING")
		}
		if fr.Status != runner.FrameCompleted && fr.Status != runner.FrameRequeued && fr.Status != runner.FrameResourceLimit && fr.Status != runner.FrameFailed {
			return nil, fmt.Errorf("BATCH_CAPTURE_FRAME_STATUS")
		}
		f := batchCaptureFrame{Index: i, Case: fr.ID, Status: fr.Status, Detail: fr.Detail, WallMillis: fr.WallMillis, State: "NOT_SENT"}
		notSent := fr.Status == runner.FrameRequeued && fr.Detail == "not sent" || fr.Status == runner.FrameResourceLimit && fr.Detail == "REQUEST_BYTES_LIMIT"
		if notSent {
			block("BATCH_CAPTURE_NOT_SENT")
		}
		if !notSent {
			if ended {
				return nil, fmt.Errorf("BATCH_CAPTURE_FRAME_ORDER")
			}
			f.State = "SENT"
			req := c.frames[i].Request
			var header [4]byte
			binary.BigEndian.PutUint32(header[:], uint32(len(req)))
			n := min(4+len(req), len(c.stdin.data)-requestOffset)
			actual := c.stdin.data[requestOffset : requestOffset+n]
			if !bytes.Equal(actual[:min(n, 4)], header[:min(n, 4)]) || n > 4 && !bytes.Equal(actual[4:], req[:n-4]) {
				return nil, fmt.Errorf("BATCH_CAPTURE_STDIN_BINDING")
			}
			f.Stdin = span("stdin", i, requestOffset, actual, n == 4+len(req))
			requestOffset += n
			if fr.Status == runner.FrameCompleted {
				if n != 4+len(req) && c.stdin.n == uint64(len(c.stdin.data)) {
					return nil, fmt.Errorf("BATCH_CAPTURE_STDIN_INCOMPLETE")
				}
				n = 4 + len(fr.Response)
				if n > len(c.stdout.data)-responseOffset {
					if c.stdout.n == uint64(len(c.stdout.data)) {
						return nil, fmt.Errorf("BATCH_CAPTURE_STDOUT_INCOMPLETE")
					}
					n = len(c.stdout.data) - responseOffset
				}
				actual = c.stdout.data[responseOffset : responseOffset+n]
				binary.BigEndian.PutUint32(header[:], uint32(len(fr.Response)))
				if !bytes.Equal(actual[:min(n, 4)], header[:min(n, 4)]) || n > 4 && !bytes.Equal(actual[4:], fr.Response[:n-4]) {
					return nil, fmt.Errorf("BATCH_CAPTURE_STDOUT_BINDING")
				}
				f.State, f.Stdout = "ANSWERED", span("stdout", i, responseOffset, actual, n == 4+len(fr.Response))
				responseOffset += n
			} else {
				actual = c.stdout.data[responseOffset:]
				complete := len(actual) >= 4 && uint64(binary.BigEndian.Uint32(actual[:4])) == uint64(len(actual)-4)
				f.Stdout = span("stdout", i, responseOffset, actual, complete)
				responseOffset += len(actual)
				ended = true
				block("BATCH_CAPTURE_UNANSWERED")
			}
		}
		w.Frames = append(w.Frames, f)
	}
	if requestOffset != len(c.stdin.data) {
		return nil, fmt.Errorf("BATCH_CAPTURE_STDIN_SUFFIX")
	}
	if responseOffset < len(c.stdout.data) {
		w.Suffix = span("stdout", len(c.frames), responseOffset, c.stdout.data[responseOffset:], false)
		block("BATCH_CAPTURE_STDOUT_SUFFIX")
	}
	if !p.Succeeded() || p.StdoutTruncated || p.StderrTruncated {
		block("BATCH_CAPTURE_PROCESS_FAILED")
	}
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if len(data) > kit.MaxDocumentBytes {
		return nil, fmt.Errorf("BATCH_CAPTURE_MEMBER_LIMIT")
	}
	w.Process = add("-process.json", "process", data)
	w.Stderr = add("-stderr.bin", "stderr", p.Stderr)
	var total uint64
	for _, f := range files {
		total += f.pin.Bytes
	}
	if total > kit.ReplayOperations()["evidence-replay"].TotalBytes {
		block("BATCH_CAPTURE_BYTES_LIMIT")
	}
	data, err = json.Marshal(w)
	if err != nil {
		return nil, err
	}
	if total+uint64(len(data)) > kit.ReplayOperations()["evidence-replay"].TotalBytes {
		block("BATCH_CAPTURE_BYTES_LIMIT")
		data, err = json.Marshal(w)
		if err != nil {
			return nil, err
		}
	}
	if len(data) > kit.MaxDocumentBytes {
		return nil, fmt.Errorf("BATCH_CAPTURE_MEMBER_LIMIT")
	}
	add(".json", "native-batch-capture", data)
	return files, nil
}
