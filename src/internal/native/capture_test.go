package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func captureFiles(t *testing.T, capture *driverCapture) (map[string][]byte, processCapture) {
	t.Helper()
	files, err := capture.files("capture/c1")
	if err != nil {
		t.Fatal(err)
	}
	stored := map[string][]byte{}
	for _, file := range files {
		if stored[file.pin.Path] != nil || file.pin.Bytes != uint64(len(file.data)) || file.pin.SHA256 != sha(file.data) {
			t.Fatalf("bad capture pin %+v", file.pin)
		}
		stored[file.pin.Path] = file.data
	}
	last := files[len(files)-1]
	if last.pin.Role != "capture" {
		t.Fatal("capture was not last")
	}
	var wire processCapture
	if err := json.Unmarshal(last.data, &wire); err != nil {
		t.Fatal(err)
	}
	return stored, wire
}

func TestDriverCaptureSerialization(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "nil", true: "empty"}[empty], func(t *testing.T) {
			spec := runner.Spec{Path: `C:\driver.exe`, Dir: `C:\work`, Wall: -time.Nanosecond, Grace: 5 * time.Second, StdoutBytes: -1}
			result := runner.Result{ExitCode: -1, WallMillis: -1}
			if empty {
				spec.Args, spec.Env, spec.Stdin = []string{}, []string{}, []byte{}
				result.Stdout, result.Stderr = []byte{}, []byte{}
			}
			capture := &driverCapture{stage: "RUN_RETURNED", spec: &spec, result: &result, err: &runner.Error{Code: "START_FAILED", Cause: errors.New("not serialized")}}
			stored, wire := captureFiles(t, capture)
			if wire.Schema != "tsgk-native-process-capture/r1" || wire.Stage != "RUN_RETURNED" || wire.Spec == nil || wire.Process == nil || wire.Error == nil || wire.Error.Type != "runner" || wire.Error.Kind != nil || wire.Error.Blocked == nil || *wire.Error.Blocked {
				t.Fatalf("capture %+v", wire)
			}
			if (wire.Spec.Args != nil) != empty || (wire.Spec.Env != nil) != empty || (wire.Spec.Stdin != nil) != empty || (wire.Stdout != nil) != empty || (wire.Stderr != nil) != empty {
				t.Fatalf("nil/empty changed %+v", wire)
			}
			if wire.Spec.Wall != -1 || wire.Spec.Grace != 5*time.Second || wire.Spec.StdoutBytes != -1 || wire.Spec.Path != spec.Path {
				t.Fatalf("spec projection changed %+v", wire.Spec)
			}
			if bytes.Contains(stored["capture/c1.json"], []byte("not serialized")) || bytes.Contains(stored["capture/c1.json"], []byte("Cause")) {
				t.Fatal("error cause was serialized")
			}
			var process runner.Result
			if err := json.Unmarshal(stored[wire.Process.Path], &process); err != nil || process.ExitCode != -1 || process.WallMillis != -1 {
				t.Fatalf("plain process changed %+v %v", process, err)
			}
		})
	}
	result := &runner.Result{Stdout: []byte{0, 0xff, 0xc0}, Stderr: []byte{0xff}}
	capture := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: result}
	stored, wire := captureFiles(t, capture)
	if !bytes.Equal(stored[wire.Stdout.Path], result.Stdout) || !bytes.Equal(stored[wire.Stderr.Path], result.Stderr) {
		t.Fatal("binary stream changed")
	}
	stored[wire.Stdout.Path][0] ^= 0xff
	again, next := captureFiles(t, capture)
	if !bytes.Equal(again[next.Stdout.Path], result.Stdout) {
		t.Fatal("published file buffers mutated the internal capture")
	}
	for _, at := range []string{"path", "env", "cleanup"} {
		t.Run("invalid-utf8-"+at, func(t *testing.T) {
			c := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: &runner.Result{}}
			switch at {
			case "path":
				c.spec.Path = "\xff"
			case "env":
				c.spec.Env = []string{"KEY=\xff"}
			case "cleanup":
				c.result.Cleanup.Detail = "\xff"
			}
			if files, err := c.files("capture/c1"); err == nil || files != nil {
				t.Fatal("invalid UTF-8 replaced or partial serialization adopted")
			}
		})
	}
}

func TestDriverCaptureRefusalAndError(t *testing.T) {
	capture := &driverCapture{}
	b := &Build{Executable: filepath.Join(t.TempDir(), "missing.exe")}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, original := b.exec(ctx, "single", nil, time.Second, kit.NativeOperations()["native-query"], "", capture)
	if original == nil || capture.err != original || capture.stage != "VERIFY_REFUSED" || capture.spec != nil || capture.result != nil {
		t.Fatalf("verify refusal %+v %v", capture, original)
	}
	_, wire := captureFiles(t, capture)
	if wire.Spec != nil || wire.Process != nil || wire.Stdout != nil || wire.Stderr != nil || wire.Error == nil || wire.Error.Type != "native" || wire.Error.Code != "EXECUTABLE_MISMATCH" || wire.Error.Kind == nil || wire.Error.Blocked != nil {
		t.Fatalf("refusal became execution evidence %+v", wire)
	}
	for _, c := range []*driverCapture{{}, {stage: "VERIFY_REFUSED"}, {stage: "RUN_RETURNED", spec: &runner.Spec{}}, {stage: "RUN_RETURNED", spec: &runner.Spec{}, result: &runner.Result{}, err: refuse(kit.KindInvalidInput, "EXECUTABLE_MISMATCH", nil)}} {
		if files, err := c.files("capture/c1"); err == nil || files != nil {
			t.Fatal("incomplete capture accepted")
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sum, size := digestOf(t, executable)
	b = &Build{Executable: executable, ExecutableSHA256: sum, ExecutableBytes: int64(size), Dir: filepath.Join(t.TempDir(), "missing")}
	capture = &driverCapture{}
	_, original = b.exec(ctx, "single", []byte{0, 1}, time.Second, kit.NativeOperations()["native-query"], "", capture)
	if original == nil || capture.err != original || capture.stage != "RUN_RETURNED" || capture.spec == nil || capture.result == nil {
		t.Fatalf("runner refusal lost %+v %v", capture, original)
	}
	_, wire = captureFiles(t, capture)
	if wire.Spec == nil || wire.Process == nil || wire.Error == nil || wire.Error.Type != "runner" {
		t.Fatalf("runner error became verify refusal %+v", wire)
	}
	other := projectCaptureError(errors.New("private detail"))
	if other.Type != "other" || other.Code != "" || other.Kind != nil || other.Blocked != nil {
		t.Fatalf("other error %+v", other)
	}
	capture = &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: &runner.Result{Stdout: []byte("previous case")}}
	x := Context{Op: kit.NativeOperations()["native-query"]}
	c := kit.IncrementalCase{ID: "invalid", Encoding: kit.EncodingUTF8, Edits: []kit.Edit{{StartByte: 9, OldEndByte: 10}}}
	out := b.runCase(ctx, x, c, []byte("a"), capture)
	if out.ExecutionStatus != kit.StatusNotRun || out.Code != "EDIT_RANGE" || !reflect.DeepEqual(*capture, driverCapture{}) {
		t.Fatalf("uninvoked case inherited earlier capture %+v %+v", out, capture)
	}
}

func TestDriverCaptureActualInvocation(t *testing.T) {
	b := fixtureBuild(t, "plain")
	x := oracleContext(false, "((number) @n (#eq? @n \"1\"))")
	request := x.request(kit.IncrementalCase{ID: "c1", Encoding: kit.EncodingUTF8}, []byte("a = 1;\n"))
	stdin := Frame(request.Encode())
	before := bytes.Clone(stdin)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	capture := &driverCapture{}
	result, err := b.exec(ctx, "single", stdin, 30*time.Second, x.Op, os.Getenv("TSGK_CGROUP_PARENT"), capture)
	if err != nil || !result.Succeeded() || capture.result == nil || capture.err != nil {
		t.Fatalf("captured run %+v %v", result, err)
	}
	legacy, err := b.Exec(ctx, "single", before, 30*time.Second, x.Op, os.Getenv("TSGK_CGROUP_PARENT"))
	if err != nil || !legacy.Succeeded() || !bytes.Equal(normalFrame(legacy.Stdout), normalFrame(result.Stdout)) {
		t.Fatalf("legacy path changed %+v %v", legacy, err)
	}
	stdout := bytes.Clone(result.Stdout)
	stdin[0] ^= 0xff
	result.Stdout[0] ^= 0xff
	if !bytes.Equal(capture.spec.Stdin, before) || !bytes.Equal(capture.result.Stdout, stdout) {
		t.Fatal("capture retained caller-owned mutable bytes")
	}
	stored, wire := captureFiles(t, capture)
	if !bytes.Equal(stored[wire.Spec.Stdin.Path], before) || !bytes.Equal(stored[wire.Stdout.Path], stdout) || wire.Spec.Env != nil || !reflect.DeepEqual(wire.Spec.Args, []string{"single"}) {
		t.Fatal("actual request/streams not bound")
	}
	if !strings.Contains(string(stored[wire.Process.Path]), `"verified":true`) {
		t.Fatal("plain process cleanup missing")
	}
}
