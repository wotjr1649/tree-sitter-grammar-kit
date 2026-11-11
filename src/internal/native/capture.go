package native

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// One capture belongs to one invocation, including runner errors before CaseResult accepts a process.
type driverCapture struct {
	stage  string
	spec   *runner.Spec
	result *runner.Result
	err    error
}

type captureSpec struct {
	Path         string            `json:"Path"`
	Args         []string          `json:"Args"`
	Dir          string            `json:"Dir"`
	Env          []string          `json:"Env"`
	Stdin        *kit.ReplayMember `json:"Stdin"`
	Interactive  bool              `json:"Interactive"`
	StdoutBytes  int64             `json:"StdoutBytes"`
	StderrBytes  int64             `json:"StderrBytes"`
	Wall         time.Duration     `json:"Wall"`
	Grace        time.Duration     `json:"Grace"`
	Memory       runner.Memory     `json:"Memory"`
	CgroupParent string            `json:"CgroupParent"`
}

type captureError struct {
	Type    string  `json:"type"`
	Code    string  `json:"code"`
	Kind    *string `json:"kind"`
	Blocked *bool   `json:"blocked"`
}

type processCapture struct {
	Schema  string            `json:"schema"`
	Stage   string            `json:"stage"`
	Spec    *captureSpec      `json:"spec"`
	Process *kit.ReplayMember `json:"process"`
	Stdout  *kit.ReplayMember `json:"stdout"`
	Stderr  *kit.ReplayMember `json:"stderr"`
	Error   *captureError     `json:"error"`
}

type captureFile struct {
	pin    kit.ReplayMember
	data   []byte
	path   string
	offset int64
}

// WriteTo verifies a captured member while copying it to the publication writer.
func (f captureFile) WriteTo(dst io.Writer) (written int64, retErr error) {
	if f.pin.Bytes > kit.MaxDocumentBytes {
		return 0, fmt.Errorf("capture member limit")
	}
	var src io.Reader
	if f.path != "" {
		file, err := os.Open(f.path)
		if err != nil {
			return 0, err
		}
		defer func() { retErr = errors.Join(retErr, file.Close()) }()
		info, err := file.Stat()
		if err != nil || f.offset < 0 || f.pin.Bytes > uint64(max(int64(0), info.Size()-f.offset)) {
			return 0, fmt.Errorf("capture member range")
		}
		src = io.NewSectionReader(file, f.offset, int64(f.pin.Bytes))
	} else {
		if uint64(len(f.data)) != f.pin.Bytes {
			return 0, fmt.Errorf("capture member length")
		}
		src = bytes.NewReader(f.data)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, h), src)
	if err != nil {
		return n, err
	}
	if uint64(n) != f.pin.Bytes || hex.EncodeToString(h.Sum(nil)) != f.pin.SHA256 {
		return n, fmt.Errorf("capture member pin mismatch")
	}
	return n, nil
}

func projectCaptureError(err error) *captureError {
	if err == nil {
		return nil
	}
	var ne *Error
	if errors.As(err, &ne) {
		return &captureError{Type: "native", Code: ne.Code, Kind: &ne.Kind}
	}
	var re *runner.Error
	if errors.As(err, &re) {
		return &captureError{Type: "runner", Code: re.Code, Blocked: &re.Blocked}
	}
	return &captureError{Type: "other"}
}

// The final capture references earlier files; no member refers to a future manifest or graph.
func (c *driverCapture) files(prefix string) ([]captureFile, error) {
	wire := processCapture{Schema: "tsgk-native-process-capture/r1", Stage: c.stage, Error: projectCaptureError(c.err)}
	var files []captureFile
	add := func(suffix, role string, data []byte) *kit.ReplayMember {
		pin := kit.ReplayMember{Path: prefix + suffix, Role: role, Bytes: uint64(len(data)), SHA256: sha(data)}
		files = append(files, captureFile{pin: pin, data: slices.Clone(data)})
		return &pin
	}
	marshal := func(value any) ([]byte, error) {
		return jsonv2.Marshal(value, jsonv2.FormatNilSliceAsNull(true), json.FormatDurationAsNano(true))
	}
	switch c.stage {
	case "VERIFY_REFUSED":
		if c.spec != nil || c.result != nil || c.err == nil {
			return nil, fmt.Errorf("invalid refused process capture")
		}
	case "RUN_RETURNED":
		if c.spec == nil || c.result == nil || wire.Error != nil && wire.Error.Type == "native" {
			return nil, fmt.Errorf("incomplete returned process capture")
		}
		s := c.spec
		wire.Spec = &captureSpec{Path: s.Path, Args: s.Args, Dir: s.Dir, Env: s.Env, Interactive: s.Interactive,
			StdoutBytes: s.StdoutBytes, StderrBytes: s.StderrBytes, Wall: s.Wall, Grace: s.Grace, Memory: s.Memory, CgroupParent: s.CgroupParent}
		if s.Stdin != nil {
			wire.Spec.Stdin = add("-stdin.bin", "stdin-frame", s.Stdin)
		}
		process, err := marshal(c.result)
		if err != nil {
			return nil, err
		}
		wire.Process = add("-process.json", "process", process)
		if c.result.Stdout != nil {
			wire.Stdout = add("-stdout.bin", "stdout-frame", c.result.Stdout)
		}
		if c.result.Stderr != nil {
			wire.Stderr = add("-stderr.bin", "stderr", c.result.Stderr)
		}
	default:
		return nil, fmt.Errorf("process invocation was not captured")
	}
	data, err := marshal(wire)
	if err != nil {
		return nil, err
	}
	add(".json", "capture", data)
	return files, nil
}
