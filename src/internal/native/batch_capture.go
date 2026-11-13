package native

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const batchCaptureSchema = "tsgk-native-batch-capture/r1"

type batchCaptureStream struct {
	file        *os.File
	path        string
	n           uint64
	retained    uint64
	h           hash.Hash
	retainedSum hash.Hash
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
	c.stdin.retainedSum, c.stdout.retainedSum = sha256.New(), sha256.New()
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
	room := int(kit.ReplayOperations()["evidence-replay"].TotalBytes) - 2*kit.MaxDocumentBytes - 65536 - int(c.stdin.retained) - int(c.stdout.retained)
	keep := min(len(data), max(0, room))
	if keep > 0 {
		if s.file == nil {
			f, err := os.CreateTemp("", "tsgk-batch-capture-*")
			if err != nil {
				return err
			}
			s.file, s.path = f, f.Name()
		}
		n, err := s.file.Write(data[:keep])
		if n > 0 {
			s.retained += uint64(n)
			s.retainedSum.Write(data[:n])
		}
		if err != nil {
			return err
		}
		if n != keep {
			return io.ErrShortWrite
		}
	}
	if keep < len(data) {
		return fmt.Errorf("BATCH_CAPTURE_BYTES_LIMIT")
	}
	return nil
}

func (s *batchCaptureStream) pin() batchCaptureStreamPin {
	return batchCaptureStreamPin{Bytes: s.n, SHA256: hex.EncodeToString(s.h.Sum(nil)), RetainedBytes: s.retained}
}

func (s *batchCaptureStream) close() error {
	var err error
	if s.file != nil {
		err = s.file.Close()
		s.file = nil
	}
	if s.path != "" {
		if removeErr := os.Remove(s.path); removeErr != nil && !os.IsNotExist(removeErr) {
			err = errors.Join(err, removeErr)
		}
		s.path = ""
	}
	return err
}

func (c *batchCapture) close() error {
	return errors.Join(c.stdin.close(), c.stdout.close())
}

func (s *batchCaptureStream) verify() error {
	if s.retained == 0 {
		if s.path != "" || s.file != nil {
			return fmt.Errorf("BATCH_CAPTURE_STREAM_FILE")
		}
		return nil
	}
	if s.path == "" {
		return fmt.Errorf("BATCH_CAPTURE_STREAM_FILE")
	}
	f, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || uint64(info.Size()) != s.retained {
		return fmt.Errorf("BATCH_CAPTURE_STREAM_LENGTH")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if !bytes.Equal(h.Sum(nil), s.retainedSum.Sum(nil)) || s.retained > s.n {
		return fmt.Errorf("BATCH_CAPTURE_STREAM_HASH")
	}
	if s.retained == s.n && !bytes.Equal(h.Sum(nil), s.h.Sum(nil)) {
		return fmt.Errorf("BATCH_CAPTURE_STREAM_HASH")
	}
	return nil
}

func (s *batchCaptureStream) equal(offset int64, expected []byte) bool {
	if offset < 0 || uint64(offset)+uint64(len(expected)) > s.retained || s.path == "" {
		return false
	}
	f, err := os.Open(s.path)
	if err != nil {
		return false
	}
	defer f.Close()
	var buf [32 * 1024]byte
	for len(expected) > 0 {
		n := min(len(expected), len(buf))
		read, err := f.ReadAt(buf[:n], offset)
		if err != nil && err != io.EOF || read != n {
			return false
		}
		if !bytes.Equal(buf[:n], expected[:n]) {
			return false
		}
		offset += int64(n)
		expected = expected[n:]
	}
	return true
}

func (s *batchCaptureStream) readAt(offset int64, dst []byte) error {
	if offset < 0 || uint64(offset)+uint64(len(dst)) > s.retained || s.path == "" {
		return io.ErrUnexpectedEOF
	}
	f, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := f.ReadAt(dst, offset)
	if err != nil && !(err == io.EOF && n == len(dst)) {
		return err
	}
	if n != len(dst) {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func hashRange(path string, offset, length int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.NewSectionReader(f, offset, length))
	if err != nil {
		return "", err
	}
	if n != length {
		return "", io.ErrUnexpectedEOF
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// files validates the transport binding and preserves actual bytes, including failed suffixes.
// Oversized frames are diagnostic fragments only; they never become ordinary small evidence.
func (c *batchCapture) files(prefix string) ([]captureFile, error) {
	if c.index < 0 || len(c.frames) != len(c.result.Frames) || c.result.Process.StdoutBytes < 0 || uint64(c.result.Process.StdoutBytes) != c.stdout.n {
		return nil, fmt.Errorf("BATCH_CAPTURE_BINDING")
	}
	p := c.result.Process
	if c.stdin.verify() != nil || c.stdout.verify() != nil {
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
		Accepted: p.Succeeded() && p.Cleanup.ResidualAfterExit == 0 && !p.StdoutTruncated && !p.StderrTruncated && c.result.TrailingBytes == 0}
	var files []captureFile
	add := func(path, role string, data []byte) kit.ReplayMember {
		pin := kit.ReplayMember{Path: prefix + path, Role: role, Bytes: uint64(len(data)), SHA256: sha(data)}
		files = append(files, captureFile{pin: pin, data: slices.Clone(data)})
		return pin
	}
	addStream := func(path, role string, stream *batchCaptureStream, offset, length int64) (kit.ReplayMember, error) {
		digest, err := hashRange(stream.path, offset, length)
		if err != nil {
			return kit.ReplayMember{}, err
		}
		member := captureFile{path: stream.path, offset: offset}
		member.pin = kit.ReplayMember{Path: prefix + path, Role: role, Bytes: uint64(length), SHA256: digest}
		files = append(files, member)
		return member.pin, nil
	}
	block := func(code string) {
		w.Accepted = false
		if w.Code == "" {
			w.Code = code
		}
	}
	if c.stdin.n != c.stdin.retained || c.stdout.n != c.stdout.retained || c.result.CaptureError != "" {
		block("BATCH_CAPTURE_BYTES_LIMIT")
	}
	span := func(direction string, index int, stream *batchCaptureStream, offset int64, length int64, complete bool) (*batchCaptureSpan, error) {
		if length == 0 {
			return nil, nil
		}
		digest, err := hashRange(stream.path, offset, length)
		if err != nil {
			return nil, err
		}
		s := &batchCaptureSpan{Offset: uint64(offset), Bytes: uint64(length), SHA256: digest, Complete: complete, Members: []kit.ReplayMember{}}
		if length > kit.MaxDocumentBytes {
			block("BATCH_CAPTURE_MEMBER_LIMIT")
		}
		for part, start := 0, int64(0); start < length; part++ {
			end := min(start+kit.MaxDocumentBytes, length)
			member, err := addStream(fmt.Sprintf("-frame-%05d-%s-%03d.bin", index, direction, part), direction+"-frame", stream, offset+start, end-start)
			if err != nil {
				return nil, err
			}
			s.Members = append(s.Members, member)
			start = end
		}
		return s, nil
	}
	requestOffset, responseOffset := int64(0), int64(0)
	var err error
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
			n := min(4+len(req), int(c.stdin.retained)-int(requestOffset))
			if n < 0 {
				n = 0
			}
			if !c.stdin.equal(requestOffset, header[:min(n, 4)]) || n > 4 && !c.stdin.equal(requestOffset+4, req[:n-4]) {
				return nil, fmt.Errorf("BATCH_CAPTURE_STDIN_BINDING")
			}
			f.Stdin, err = span("stdin", i, &c.stdin, requestOffset, int64(n), n == 4+len(req))
			if err != nil {
				return nil, err
			}
			requestOffset += int64(n)
			if fr.Status == runner.FrameCompleted {
				if n != 4+len(req) && c.stdin.n == c.stdin.retained {
					return nil, fmt.Errorf("BATCH_CAPTURE_STDIN_INCOMPLETE")
				}
				n = 4 + len(fr.Response)
				if n > int(c.stdout.retained)-int(responseOffset) {
					if c.stdout.n == c.stdout.retained {
						return nil, fmt.Errorf("BATCH_CAPTURE_STDOUT_INCOMPLETE")
					}
					n = max(0, int(c.stdout.retained)-int(responseOffset))
				}
				binary.BigEndian.PutUint32(header[:], uint32(len(fr.Response)))
				if !c.stdout.equal(responseOffset, header[:min(n, 4)]) || n > 4 && !c.stdout.equal(responseOffset+4, fr.Response[:n-4]) {
					return nil, fmt.Errorf("BATCH_CAPTURE_STDOUT_BINDING")
				}
				f.State = "ANSWERED"
				f.Stdout, err = span("stdout", i, &c.stdout, responseOffset, int64(n), n == 4+len(fr.Response))
				if err != nil {
					return nil, err
				}
				responseOffset += int64(n)
			} else {
				length := int64(c.stdout.retained) - responseOffset
				complete := false
				if length >= 4 {
					var responseHeader [4]byte
					if c.stdout.readAt(responseOffset, responseHeader[:]) == nil {
						complete = uint64(binary.BigEndian.Uint32(responseHeader[:])) == uint64(length-4)
					}
				}
				f.Stdout, err = span("stdout", i, &c.stdout, responseOffset, length, complete)
				if err != nil {
					return nil, err
				}
				responseOffset += length
				ended = true
				block("BATCH_CAPTURE_UNANSWERED")
			}
		}
		w.Frames = append(w.Frames, f)
	}
	if requestOffset != int64(c.stdin.retained) {
		return nil, fmt.Errorf("BATCH_CAPTURE_STDIN_SUFFIX")
	}
	if responseOffset < int64(c.stdout.retained) {
		w.Suffix, err = span("stdout", len(c.frames), &c.stdout, responseOffset, int64(c.stdout.retained)-responseOffset, false)
		if err != nil {
			return nil, err
		}
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
