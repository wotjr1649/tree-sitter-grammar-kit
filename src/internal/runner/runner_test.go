package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// hardBackend reports whether this host's backend enforces a kernel memory cap.
func hardBackend() bool {
	return runtime.GOOS == "windows" || (runtime.GOOS == "linux" && os.Getenv("TSGK_CGROUP_PARENT") != "")
}

func helperSpec(t *testing.T, mode string) (Spec, string) {
	t.Helper()
	pids := filepath.Join(t.TempDir(), "pids")
	return Spec{
		Path: self(), Dir: t.TempDir(),
		Env:         []string{"TSGK_RUNNER_HELPER=" + mode, "TSGK_PIDS=" + pids},
		StdoutBytes: 1 << 20, StderrBytes: 1 << 20, Wall: 30 * time.Second, Grace: 5 * time.Second,
		Memory:       Memory{Bytes: 1 << 30, Hard: hardBackend()},
		CgroupParent: os.Getenv("TSGK_CGROUP_PARENT"),
	}, pids
}

func run(t *testing.T, s Spec) Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, err := Run(ctx, s)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return r
}

func readPIDs(t *testing.T, path string) []int {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("pid record: %v", err)
	}
	var pids []int
	for _, f := range strings.Fields(string(data)) {
		n, _ := strconv.Atoi(f)
		pids = append(pids, n)
	}
	return pids
}

// requireDead checks, independently of the backend, that every recorded process is gone.
func requireDead(t *testing.T, pids []int, want int) {
	t.Helper()
	if len(pids) < want {
		t.Fatalf("expected at least %d recorded processes, got %v", want, pids)
	}
	deadline := time.Now().Add(5 * time.Second)
	for _, pid := range pids {
		for alive(pid) {
			if time.Now().After(deadline) {
				killPID(pid)
				t.Fatalf("process %d of the tree survived termination", pid)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// S04-A10: literal spaces, quotes, metacharacters and Unicode reach the tool unchanged.
func TestArgsDirEnvExact(t *testing.T) {
	s, _ := helperSpec(t, "args")
	dir := filepath.Join(t.TempDir(), "작업 dir 'q' \"x\"")
	if runtime.GOOS == "windows" {
		dir = filepath.Join(t.TempDir(), "작업 dir 'q' x")
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s.Dir = dir
	s.Args = []string{"a b", `"quoted"`, `back\slash\`, `tail\\"`, "", "%PATH%", "$HOME", "a;b&c|d>e", "한글 ü", "*"}
	s.Env = append(s.Env, "TSGK_EXTRA=v w")
	r := run(t, s)
	if !r.Succeeded() {
		t.Fatalf("result %+v stderr %s", r, r.Stderr)
	}
	var got struct {
		Args []string
		Dir  string
		Env  []string
	}
	if err := json.Unmarshal(r.Stdout, &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Args, s.Args) {
		t.Fatalf("args changed:\n got %q\nwant %q", got.Args, s.Args)
	}
	if a, b := evalDir(t, got.Dir), evalDir(t, dir); a != b {
		t.Fatalf("dir %q != %q", a, b)
	}
	for _, e := range got.Env {
		name, _, _ := strings.Cut(e, "=")
		switch strings.ToUpper(name) {
		case "TSGK_RUNNER_HELPER", "TSGK_PIDS", "TSGK_EXTRA":
		case "SYSTEMROOT": // os/exec adds this critical variable on Windows
			if runtime.GOOS != "windows" {
				t.Fatalf("unexpected %s", e)
			}
		default:
			t.Fatalf("environment leaked into the tool: %q", name)
		}
	}
}

func evalDir(t *testing.T, p string) string {
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ToLower(r)
}

// S04-A07: a nonzero exit is FAILED with its partial output, never success.
func TestNonzeroExit(t *testing.T) {
	s, _ := helperSpec(t, "exit:3")
	r := run(t, s)
	if r.Status != StatusFailed || r.ExitCode != 3 || string(r.Stdout) != "partial" || r.Succeeded() {
		t.Fatalf("%+v %q", r, r.Stdout)
	}
}

// S04-A07/A11: floods end in OUTPUT_LIMIT with the kept prefix bounded; equal passes.
func TestOutputLimits(t *testing.T) {
	for _, mode := range []string{"flood", "stderr-flood"} {
		s, _ := helperSpec(t, mode)
		r := run(t, s)
		if r.Status != StatusResourceLimit || r.Reason != ReasonOutput || !r.Cleanup.Verified {
			t.Fatalf("%s: %+v", mode, r)
		}
		if int64(len(r.Stdout)) > s.StdoutBytes || int64(len(r.Stderr)) > s.StderrBytes {
			t.Fatalf("%s kept more than the cap", mode)
		}
	}
	const limit = 4096
	for _, c := range []struct {
		n    int
		want string
	}{{limit, StatusCompleted}, {limit + 1, StatusResourceLimit}} {
		s, _ := helperSpec(t, "exact:"+strconv.Itoa(c.n))
		s.StdoutBytes = limit
		r := run(t, s)
		if r.Status != c.want || r.StdoutBytes != int64(c.n) || len(r.Stdout) != min(c.n, limit) {
			t.Fatalf("exact %d: %+v", c.n, r)
		}
	}
}

// S04-A08: wall expiry and caller cancellation terminate parent, child and grandchild.
func TestTreeTermination(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		s, pids := helperSpec(t, "tree")
		ctx, cancel := context.WithCancel(context.Background())
		if cancelled {
			time.AfterFunc(1500*time.Millisecond, cancel)
		} else {
			s.Wall = 1500 * time.Millisecond
		}
		start := time.Now()
		r, err := Run(ctx, s)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		want, reason := StatusResourceLimit, ReasonWall
		if cancelled {
			want, reason = StatusCancelled, ReasonCancelled
		}
		if r.Status != want || r.Reason != reason || !r.Cleanup.Verified {
			t.Fatalf("cancelled=%v: %+v", cancelled, r)
		}
		if el := time.Since(start); el > 1500*time.Millisecond+s.Grace+5*time.Second {
			t.Fatalf("termination took %v", el)
		}
		requireDead(t, readPIDs(t, pids), 3)
	}
}

// S04-A08: a grandchild holding the stdout pipe after the parent exits is terminated, so
// the result does not wait for it and is not reported as an unbounded success.
func TestPipeHoldingDescendant(t *testing.T) {
	s, pids := helperSpec(t, "orphan")
	start := time.Now()
	r := run(t, s)
	if r.Status != StatusCompleted || r.ExitCode != 0 || r.Cleanup.ResidualAfterExit < 1 || !r.Cleanup.Verified {
		t.Fatalf("%+v", r)
	}
	if !bytes.Equal(r.Stdout, []byte("parent-done")) || time.Since(start) > 15*time.Second {
		t.Fatalf("stdout %q after %v", r.Stdout, time.Since(start))
	}
	requireDead(t, readPIDs(t, pids), 2)
}

// S04-A08/A13: a descendant that leaves the process group is contained only by the Job
// Object and cgroup backends; the process-group backends must not claim verified cleanup.
func TestEscapedDescendant(t *testing.T) {
	s, pids := helperSpec(t, "escape")
	s.Grace = 2 * time.Second
	r := run(t, s)
	escaped := readPIDs(t, pids)
	if r.Capabilities.EscapedDescendants == Contained {
		if !r.Cleanup.Verified {
			t.Fatalf("%+v", r)
		}
		requireDead(t, escaped, 1)
		return
	}
	for _, pid := range escaped {
		killPID(pid)
	}
	if r.Cleanup.Verified {
		t.Fatalf("process-group backend claimed verified cleanup with an escaped pipe holder: %+v", r)
	}
	t.Logf("NOT_CONTAINED backend reported: %s", r.Cleanup.Detail)
}

// S04-A16: the memory cap maps to RESOURCE_LIMIT/MEMORY_LIMIT; hard where the kernel
// enforces it, sampled (non-strict) otherwise.
func TestMemoryLimit(t *testing.T) {
	s, pids := helperSpec(t, "memory:768")
	s.Memory.Bytes = 192 << 20
	r := run(t, s)
	if r.Status != StatusResourceLimit || r.Reason != ReasonMemory || !r.Memory.LimitHit || !r.Cleanup.Verified {
		t.Fatalf("%+v stderr %.300s", r, r.Stderr)
	}
	want := MemorySampled
	if hardBackend() {
		want = MemoryHard
	}
	if r.Memory.Enforcement != want {
		t.Fatalf("enforcement %s, want %s", r.Memory.Enforcement, want)
	}
	if _, err := os.Stat(pids); err == nil {
		requireDead(t, readPIDs(t, pids), 0)
	}
	t.Logf("memory: %+v", r.Memory)
}

// S04-A06/A11: a missing or relative tool, invalid limits and an unavailable required
// control are refused before any launch.
func TestBlockedBeforeLaunch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	base, _ := helperSpec(t, "args")
	cases := map[string]func(*Spec){
		"TOOL_MISSING":           func(s *Spec) { s.Path = filepath.Join(t.TempDir(), "absent") },
		"TOOL_PATH_NOT_ABSOLUTE": func(s *Spec) { s.Path = "tree-sitter" },
		"TOOL_NOT_REGULAR":       func(s *Spec) { s.Path = t.TempDir() },
		"DIR_INVALID":            func(s *Spec) { s.Dir = filepath.Join(t.TempDir(), "absent") },
		"LIMIT_INVALID":          func(s *Spec) { s.Wall = 0 },
	}
	switch runtime.GOOS {
	case "linux":
		cases["CGROUP_UNAVAILABLE"] = func(s *Spec) { s.CgroupParent = filepath.Join(t.TempDir(), "not-a-cgroup") }
		if !hardBackend() {
			cases["MEMORY_HARD_CAP_UNSUPPORTED"] = func(s *Spec) { s.Memory.Hard = true }
		}
	case "darwin":
		cases["MEMORY_HARD_CAP_UNSUPPORTED"] = func(s *Spec) { s.Memory.Hard = true }
	}
	for code, mut := range cases {
		s := base
		mut(&s)
		_, err := Run(ctx, s)
		var re *Error
		if !errors.As(err, &re) || re.Code != code || !re.Blocked {
			t.Fatalf("%s: got %v", code, err)
		}
	}
}

// Interactive streams carry exact bytes both ways.
func TestInteractiveEcho(t *testing.T) {
	s, _ := helperSpec(t, "echo")
	s.Interactive = true
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	p, err := Start(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("frame\x00\xff"), 10000)
	go func() {
		p.Stdin().Write(payload)
		p.Stdin().Close()
	}()
	got, err := io.ReadAll(p.Stdout())
	if err != nil {
		t.Fatal(err)
	}
	r := p.Wait()
	if !bytes.Equal(got, payload) || !r.Succeeded() {
		t.Fatalf("echo mismatch or %+v", r)
	}
}

// S04-A13: the host capability receipt. CI on Linux must provide a delegated cgroup so the
// hard-cap backend is exercised rather than silently replaced by the sampled one.
func TestCapabilityReceipt(t *testing.T) {
	s, _ := helperSpec(t, "args")
	caps := Probe(s)
	data, _ := json.Marshal(caps)
	t.Logf("TSGK_CAPABILITY_RECEIPT %s", data)
	if runtime.GOOS == "linux" && os.Getenv("CI") != "" && caps.Memory != MemoryHard {
		t.Fatalf("hosted Linux run without the delegated cgroup backend: %s", data)
	}
	if caps.Backend == "" || caps.TreeCleanup == "" || caps.Memory == "" || caps.MemoryMetric == "" {
		t.Fatalf("incomplete capability receipt %s", data)
	}
}
