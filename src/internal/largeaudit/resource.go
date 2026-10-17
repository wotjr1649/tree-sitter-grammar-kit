package largeaudit

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// fatal_limit uses a 1024-byte static payload buffer and a 4-byte frame header.
const ResourceFrameLimit = 4 + 1023

type ResourceCapture struct {
	RequestSHA   string `json:"request_sha256"`
	RequestBytes uint64 `json:"request_bytes"`
	SourceSHA    string `json:"source_sha256"`
	StdoutSHA    string `json:"stdout_sha256"`
	StdoutBytes  uint64 `json:"stdout_bytes"`
	Stdout       []byte `json:"stdout"`
}

func required(data []byte, keys ...string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := jsonv2.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	for _, key := range keys {
		value, ok := object[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, errors.New("resource member missing or null")
		}
	}
	return object, nil
}

// Bound encoded input before the base64 decoder can allocate the response bytes.
func (c *ResourceCapture) UnmarshalJSON(data []byte) error {
	if len(data) > 2048 {
		return errors.New("resource capture JSON exceeds limit")
	}
	object, err := required(data, "request_sha256", "request_bytes", "source_sha256", "stdout_sha256", "stdout_bytes", "stdout")
	if err != nil {
		return err
	}
	if len(object["stdout"]) > 2+1372 {
		return errors.New("resource encoded stdout exceeds limit")
	}
	type capture ResourceCapture
	var value capture
	if err = jsonv2.Unmarshal(data, &value, jsonv2.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if len(value.Stdout) < 4 || len(value.Stdout) > ResourceFrameLimit {
		return errors.New("resource stdout frame exceeds limit")
	}
	*c = ResourceCapture(value)
	return nil
}

// ResourceRequest reconstructs the fixed control, separately from the OS memory limit.
func ResourceRequest() native.Request {
	source := []byte(SmallSources()[0])
	req := native.Request{ID: "small-0", Encoding: kit.EncodingUTF8, Output: kit.OutputAuto, Limits: native.LimitsFor(kit.NativeOperations()["native-query-large"]), Source: source, Points: []kit.NativePoint{{ID: "middle", Byte: uint32(len(source) / 2)}}}
	req.Limits.MemoryBytes = 1
	return req
}

// CaptureResource retains the whole actual stdout and binds the actual stdin frame.
func CaptureResource(req native.Request, stdin, stdout []byte, process runner.Result) (*ResourceCapture, error) {
	if len(stdout) > ResourceFrameLimit || !bytes.Equal(stdin, native.Frame(req.Encode())) || !bytes.Equal(stdin, native.Frame(ResourceRequest().Encode())) {
		return nil, errors.New("resource request or output outside registered control")
	}
	c := &ResourceCapture{RequestSHA: digest(stdin), RequestBytes: uint64(len(stdin)), SourceSHA: digest(req.Source), StdoutSHA: digest(stdout), StdoutBytes: uint64(len(stdout)), Stdout: bytes.Clone(stdout)}
	if _, err := CheckResource(c, process); err != nil {
		return nil, err
	}
	return c, nil
}

// CheckResource replays preserved bytes; it does not prove OS enforcement or producer honesty.
func CheckResource(c *ResourceCapture, process runner.Result) (native.Producer, error) {
	bad := func() (native.Producer, error) { return native.Producer{}, errors.New("resource capture invalid") }
	req := ResourceRequest()
	stdin := native.Frame(req.Encode())
	op := kit.NativeOperations()["native-query-large"]
	if c == nil || len(c.Stdout) < 4 || len(c.Stdout) > ResourceFrameLimit || c.RequestSHA != digest(stdin) || c.RequestBytes != uint64(len(stdin)) || c.SourceSHA != digest(req.Source) || c.StdoutSHA != digest(c.Stdout) || c.StdoutBytes != uint64(len(c.Stdout)) || process.StdoutBytes != int64(len(c.Stdout)) ||
		process.Status != runner.StatusFailed || process.Reason != runner.ReasonExited || process.ExitCode != 3 || process.StdoutTruncated || process.StderrTruncated || !process.Cleanup.Verified || process.Cleanup.ResidualAfterExit != 0 || process.Memory.Enforcement != runner.MemoryHard || !process.Memory.PeakKnown || process.Memory.Limit != op.MemoryBytes || process.Memory.LimitHit || process.Memory.Peak > process.Memory.Limit || process.WallMillis < 0 || process.WallMillis > op.ParseWall.Milliseconds() || process.StderrBytes < 0 || process.StderrBytes > 65536 {
		return bad()
	}
	payload, err := native.SplitFrame(c.Stdout)
	if err != nil {
		return bad()
	}
	object, err := required(payload, "protocol", "id", "status", "code", "producer", "source_bytes", "steps_completed", "steps", "complete")
	if err != nil {
		return bad()
	}
	p, err := required(object["producer"], "language_version", "runtime_language_version", "runtime_min_compatible", "query")
	if err != nil || len(p) != 4 {
		return bad()
	}
	r, err := native.DecodeResponse(payload)
	if err != nil || r.Status != kit.StatusResourceLimit || r.Code != "ALLOCATION_LIMIT" || r.StepsCompleted != 0 || len(r.Steps) != 0 || r.Producer.LanguageVersion == 0 || r.Producer.RuntimeLanguageVersion < r.Producer.LanguageVersion || r.Producer.RuntimeMinCompatible == 0 || r.Producer.RuntimeMinCompatible > r.Producer.LanguageVersion {
		return bad()
	}
	if _, err = native.Check(r, req, [][]byte{req.Source}, nil, process.ExitCode); err != nil {
		return bad()
	}
	return r.Producer, nil
}
