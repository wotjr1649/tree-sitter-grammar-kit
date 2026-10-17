package largeaudit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func resourceFixture(t *testing.T) (*ResourceCapture, runner.Result) {
	t.Helper()
	req := ResourceRequest()
	payload := []byte(fmt.Sprintf(`{"protocol":"tsgk-native/r1","id":"small-0","status":"RESOURCE_LIMIT","code":"ALLOCATION_LIMIT","producer":{"language_version":15,"runtime_language_version":15,"runtime_min_compatible":13,"query":"UNSUPPORTED"},"source_bytes":%d,"steps_completed":0,"steps":[],"complete":true}`, len(req.Source)))
	stdout := native.Frame(payload)
	p := runner.Result{Status: runner.StatusFailed, Reason: runner.ReasonExited, ExitCode: 3, StdoutBytes: int64(len(stdout)), Cleanup: runner.Cleanup{Verified: true}, Memory: runner.MemoryObservation{Limit: kit.NativeOperations()["native-query-large"].MemoryBytes, Enforcement: runner.MemoryHard, PeakKnown: true}}
	c, err := CaptureResource(req, native.Frame(req.Encode()), stdout, p)
	if err != nil {
		t.Fatal(err)
	}
	return c, p
}

func TestResourceCaptureReplay(t *testing.T) {
	c, process := resourceFixture(t)
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var restored ResourceCapture
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if _, err = CheckResource(&restored, process); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(c.Stdout, restored.Stdout) {
		t.Fatal("stdout bytes changed")
	}
	for _, bad := range [][]byte{
		bytes.Replace(data, []byte(`"request_bytes":`), []byte(`"unknown":0,"request_bytes":`), 1),
		bytes.Replace(data, []byte(`"source_sha256":`), []byte(`"stdout_bytes":0,"source_sha256":`), 1),
		bytes.Replace(data, []byte(`"stdout":"`), []byte(`"stdout":123,"unused":"`), 1),
	} {
		if json.Unmarshal(bad, &restored) == nil {
			t.Fatal("invalid capture JSON shape accepted")
		}
	}
	if _, err = CheckResource(nil, process); err == nil {
		t.Fatal("missing capture accepted")
	}
	for _, field := range []string{"request_sha256", "request_bytes", "source_sha256", "stdout_sha256", "stdout_bytes", "stdout"} {
		for _, null := range []bool{false, true} {
			var object map[string]json.RawMessage
			if err = json.Unmarshal(data, &object); err != nil {
				t.Fatal(err)
			}
			if null {
				object[field] = []byte("null")
			} else {
				delete(object, field)
			}
			bad, _ := json.Marshal(object)
			if json.Unmarshal(bad, &restored) == nil {
				t.Fatalf("missing/null %s accepted", field)
			}
		}
	}
	for name, change := range map[string]func(*ResourceCapture, *runner.Result){
		"request-hash":     func(c *ResourceCapture, p *runner.Result) { c.RequestSHA = strings.Repeat("a", 64) },
		"request-length":   func(c *ResourceCapture, p *runner.Result) { c.RequestBytes++ },
		"source-hash":      func(c *ResourceCapture, p *runner.Result) { c.SourceSHA = strings.Repeat("a", 64) },
		"stdout-hash":      func(c *ResourceCapture, p *runner.Result) { c.StdoutSHA = strings.Repeat("a", 64) },
		"stdout-length":    func(c *ResourceCapture, p *runner.Result) { c.StdoutBytes++ },
		"process-length":   func(c *ResourceCapture, p *runner.Result) { p.StdoutBytes++ },
		"exit":             func(c *ResourceCapture, p *runner.Result) { p.ExitCode = 0 },
		"status":           func(c *ResourceCapture, p *runner.Result) { p.Status = runner.StatusCompleted },
		"reason":           func(c *ResourceCapture, p *runner.Result) { p.Reason = "" },
		"cleanup":          func(c *ResourceCapture, p *runner.Result) { p.Cleanup.Verified = false },
		"residual":         func(c *ResourceCapture, p *runner.Result) { p.Cleanup.ResidualAfterExit = 1 },
		"stdout-truncated": func(c *ResourceCapture, p *runner.Result) { p.StdoutTruncated = true },
		"stderr-truncated": func(c *ResourceCapture, p *runner.Result) { p.StderrTruncated = true },
		"os-limit":         func(c *ResourceCapture, p *runner.Result) { p.Memory.Limit = 1 },
		"memory-hit":       func(c *ResourceCapture, p *runner.Result) { p.Memory.LimitHit = true },
		"memory-peak":      func(c *ResourceCapture, p *runner.Result) { p.Memory.Peak = p.Memory.Limit + 1 },
		"memory-unknown":   func(c *ResourceCapture, p *runner.Result) { p.Memory.PeakKnown = false },
		"memory-soft":      func(c *ResourceCapture, p *runner.Result) { p.Memory.Enforcement = "SAMPLED" },
		"wall":             func(c *ResourceCapture, p *runner.Result) { p.WallMillis = 90001 },
		"stderr-budget":    func(c *ResourceCapture, p *runner.Result) { p.StderrBytes = 65537 },
	} {
		t.Run(name, func(t *testing.T) {
			bad, p := *c, process
			change(&bad, &p)
			if _, e := CheckResource(&bad, p); e == nil {
				t.Fatal("invalid capture accepted")
			}
		})
	}
}

func TestResourceResponseAndBounds(t *testing.T) {
	c, p := resourceFixture(t)
	check := func(frame []byte) error {
		bad, process := *c, p
		bad.Stdout = frame
		bad.StdoutBytes = uint64(len(frame))
		bad.StdoutSHA = digest(frame)
		process.StdoutBytes = int64(len(frame))
		_, e := CheckResource(&bad, process)
		return e
	}
	payload := c.Stdout[4:]
	for name, frame := range map[string][]byte{
		"truncated-header": c.Stdout[:3], "truncated": c.Stdout[:len(c.Stdout)-1],
		"two-frames": append(bytes.Clone(c.Stdout), c.Stdout...), "trailing-byte": append(bytes.Clone(c.Stdout), 0),
		"oversize":      native.Frame(bytes.Repeat([]byte(" "), 1024)),
		"trailing-json": native.Frame(append(bytes.Clone(payload), []byte(`{}`)...)),
		"invalid-utf8":  native.Frame(bytes.Replace(payload, []byte("ALLOCATION_LIMIT"), []byte{0xff}, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if check(frame) == nil {
				t.Fatal("invalid frame accepted")
			}
		})
	}
	for _, pair := range [][2]string{
		{`"protocol":"tsgk-native/r1"`, `"protocol":"tsgk-native/r2"`},
		{`"id":"small-0"`, `"id":"other"`},
		{`"status":"RESOURCE_LIMIT"`, `"status":"COMPLETED"`},
		{`"code":"ALLOCATION_LIMIT"`, `"code":"NODE_LIMIT"`},
		{fmt.Sprintf(`"source_bytes":%d`, len(ResourceRequest().Source)), fmt.Sprintf(`"source_bytes":%d`, len(ResourceRequest().Source)+1)},
		{`"steps_completed":0`, `"steps_completed":1`},
		{`"steps":[]`, `"steps":[{}]`},
		{`"complete":true`, `"complete":false`},
		{`"language_version":15`, `"language_version":16`},
		{`"runtime_min_compatible":13`, `"runtime_min_compatible":16`},
		{`"query":"UNSUPPORTED"`, `"query":"SUPPORTED"`},
		{`"query":"UNSUPPORTED"`, `"query":"UNSUPPORTED","api":null`},
		{`"complete":true`, `"complete":true,"complete":true`},
		{`"complete":true`, `"complete":true,"unknown":0`},
	} {
		bad := bytes.Replace(payload, []byte(pair[0]), []byte(pair[1]), 1)
		if bytes.Equal(payload, bad) {
			t.Fatalf("mutation did not change payload: %s", pair[0])
		}
		if check(native.Frame(bad)) == nil {
			t.Fatalf("invalid response accepted: %s", pair[1])
		}
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		t.Fatal(err)
	}
	for key := range object {
		for _, null := range []bool{false, true} {
			var bad map[string]json.RawMessage
			_ = json.Unmarshal(payload, &bad)
			if null {
				bad[key] = []byte("null")
			} else {
				delete(bad, key)
			}
			b, _ := json.Marshal(bad)
			if check(native.Frame(b)) == nil {
				t.Fatalf("missing/null response %s accepted", key)
			}
		}
	}
	for _, key := range []string{"language_version", "runtime_language_version", "runtime_min_compatible", "query"} {
		for _, null := range []bool{false, true} {
			var producer map[string]json.RawMessage
			_ = json.Unmarshal(object["producer"], &producer)
			if null {
				producer[key] = []byte("null")
			} else {
				delete(producer, key)
			}
			badProducer, _ := json.Marshal(producer)
			var bad map[string]json.RawMessage
			_ = json.Unmarshal(payload, &bad)
			bad["producer"] = badProducer
			b, _ := json.Marshal(bad)
			if check(native.Frame(b)) == nil {
				t.Fatalf("missing/null producer %s accepted", key)
			}
		}
	}
	maxFrame := native.Frame(append(bytes.Clone(payload), bytes.Repeat([]byte(" "), 1023-len(payload))...))
	if len(maxFrame) != ResourceFrameLimit || check(maxFrame) != nil {
		t.Fatal("maximum valid frame rejected")
	}
	maxCapture := *c
	maxCapture.Stdout = maxFrame
	maxCapture.StdoutSHA = digest(maxFrame)
	maxCapture.StdoutBytes = uint64(len(maxFrame))
	b, _ := json.Marshal(maxCapture)
	var decoded ResourceCapture
	if json.Unmarshal(b, &decoded) != nil {
		t.Fatal("maximum encoded frame rejected")
	}
	pretty, err := json.MarshalIndent(maxCapture, "      ", "  ")
	if err != nil || json.Unmarshal(pretty, &decoded) != nil {
		t.Fatal("indented production capture rejected")
	}
	malformed := bytes.Clone(b)
	malformed[bytes.Index(malformed, []byte(`"stdout":"`))+len(`"stdout":"`)] = '!'
	if len(malformed) != len(b) || json.Unmarshal(malformed, &decoded) == nil {
		t.Fatal("same-length malformed base64 accepted")
	}
	for _, bad := range [][]byte{bytes.Replace(b, []byte(`"request_sha256":"`), []byte(`"request_sha256":"`+strings.Repeat("a", 2049)), 1), bytes.Replace(b, []byte(`"stdout":"`), []byte(`"stdout":"AAAA`), 1)} {
		if json.Unmarshal(bad, &decoded) == nil {
			t.Fatal("excessive or malformed encoded capture accepted")
		}
	}
	oversize := maxCapture
	oversize.Stdout = append(bytes.Clone(maxFrame), ' ')
	tooLarge, _ := json.Marshal(oversize)
	if json.Unmarshal(tooLarge, &decoded) == nil {
		t.Fatal("1028 decoded bytes with 1372 encoded characters accepted")
	}
	for _, change := range []func(*native.Request){func(r *native.Request) { r.Limits.MemoryBytes = 2 }, func(r *native.Request) { r.Source = append(bytes.Clone(r.Source), ' ') }, func(r *native.Request) { r.Points[0].Byte++ }, func(r *native.Request) { r.Protocol = native.ProtocolR2 }} {
		req := ResourceRequest()
		change(&req)
		if _, err := CaptureResource(req, native.Frame(req.Encode()), c.Stdout, p); err == nil {
			t.Fatal("changed registered request accepted")
		}
	}
	if _, err := CaptureResource(ResourceRequest(), []byte("different stdin"), c.Stdout, p); err == nil {
		t.Fatal("actual stdin mismatch accepted")
	}
}
