// Package runner is the single supervised external-process runner of the kit. It starts
// one executable by absolute path with an exact argv, working directory and environment
// (no shell, no PATH lookup, no inherited environment), bounds stdout/stderr, enforces a
// wall deadline and caller cancellation, and terminates and verifies the whole process
// tree through the current OS backend: a Windows Job Object, a Linux cgroup v2 leaf (or a
// process group when no delegated cgroup is given), or a macOS process group.
//
// Environment and path isolation is not containment of hostile native code: the backends
// bound and clean up trusted or approved tools. Only the cgroup and Job Object backends
// contain descendants that leave the process group (setsid); the process-group backends
// report that as EscapedDescendants=NOT_CONTAINED.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Result status values follow the kit report model.
const (
	StatusCompleted     = "COMPLETED"
	StatusFailed        = "FAILED"
	StatusCancelled     = "CANCELLED"
	StatusResourceLimit = "RESOURCE_LIMIT"
)

// Termination reasons. A reason is set once; the first cause wins.
const (
	ReasonExited    = "EXITED"
	ReasonWall      = "WALL_LIMIT"
	ReasonCancelled = "CANCELLED"
	ReasonOutput    = "OUTPUT_LIMIT"
	ReasonMemory    = "MEMORY_LIMIT"
)

// Capability values.
const (
	MemoryHard        = "HARD"
	MemorySampled     = "SAMPLED"
	MemoryUnsupported = "UNSUPPORTED"
	Contained         = "CONTAINED"
	NotContained      = "NOT_CONTAINED"
)

// SampleInterval is the sampled-memory backends' polling interval.
const SampleInterval = 100 * time.Millisecond

// Memory is the requested memory policy. Hard requires a kernel-enforced cap; without
// Hard a sampled backend may observe and terminate, labelled non-strict.
type Memory struct {
	Bytes uint64
	Hard  bool
}

// Spec is one external process request. Every limit must be positive.
type Spec struct {
	Path        string   // absolute executable path; never looked up on PATH
	Args        []string // argv[1:]; passed without shell interpretation
	Dir         string   // absolute working directory
	Env         []string // exact environment; nil means empty, never inherited
	Stdin       []byte   // Run only; nil connects the null device
	Interactive bool     // Start only: the caller writes Stdin and reads Stdout
	StdoutBytes int64
	StderrBytes int64
	Wall        time.Duration
	Grace       time.Duration // SIGTERM-to-SIGKILL delay and cleanup verification bound
	Memory      Memory
	// CgroupParent is a delegated cgroup v2 directory (Linux only). The runner creates and
	// removes one leaf below it per process; empty selects the process-group backend.
	CgroupParent string
}

// Capabilities describe what the selected backend enforces on this host.
type Capabilities struct {
	OS                 string `json:"os"`
	Backend            string `json:"backend"`
	TreeCleanup        string `json:"tree_cleanup"`
	EscapedDescendants string `json:"escaped_descendants"`
	Memory             string `json:"memory"`
	MemoryMetric       string `json:"memory_metric"`
	PoliteTermination  bool   `json:"polite_termination"`
}

// Cleanup records the process-tree termination and its verification.
type Cleanup struct {
	Method string `json:"method"`
	// Scope is what Verified covers: the whole tree (JOB_OBJECT, CGROUP) or only the
	// process group (PROCESS_GROUP); a descendant that left the group is not observed
	// there unless it still holds an output pipe.
	Scope             string `json:"scope"`
	ResidualAfterExit int    `json:"residual_after_exit"`
	Verified          bool   `json:"verified"`
	Detail            string `json:"detail,omitempty"`
}

// MemoryObservation keeps the backend's own metric and unit; metrics are never merged.
type MemoryObservation struct {
	Limit       uint64 `json:"limit_bytes"`
	Enforcement string `json:"enforcement"`
	Metric      string `json:"metric"`
	Peak        uint64 `json:"peak_bytes"`
	PeakKnown   bool   `json:"peak_known"`
	LimitHit    bool   `json:"limit_hit"`
}

// Result is the outcome of one supervised process.
type Result struct {
	Status          string            `json:"status"`
	Reason          string            `json:"reason"`
	ExitCode        int               `json:"exit_code"`
	Stdout          []byte            `json:"-"`
	Stderr          []byte            `json:"-"`
	StdoutBytes     int64             `json:"stdout_bytes"`
	StderrBytes     int64             `json:"stderr_bytes"`
	StdoutTruncated bool              `json:"stdout_truncated"`
	StderrTruncated bool              `json:"stderr_truncated"`
	WallMillis      int64             `json:"wall_ms"`
	Cleanup         Cleanup           `json:"cleanup"`
	Memory          MemoryObservation `json:"memory"`
	Capabilities    Capabilities      `json:"capabilities"`
}

// Succeeded is true only for a zero exit within every limit with verified cleanup.
func (r *Result) Succeeded() bool {
	return r.Status == StatusCompleted && r.ExitCode == 0 && r.Cleanup.Verified
}

// Error is a refusal before launch (Blocked) or a launch failure.
type Error struct {
	Code    string
	Blocked bool
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Cause.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Cause }

func blocked(code string, cause error) *Error { return &Error{Code: code, Blocked: true, Cause: cause} }

// tree is one OS backend instance supervising a single process and its descendants.
type tree interface {
	prepare(cmd *exec.Cmd) error
	attach(p *os.Process) error
	stop(force bool)
	live() (int, error)
	limitHit() bool
	peak() (uint64, bool)
	sample() (uint64, error) // current memory for sampled backends
	notify() <-chan struct{} // hard-limit events delivered while running, or nil
	close() error
}

// Probe reports the capabilities the backend would provide for spec on this host.
func Probe(spec Spec) Capabilities {
	t, caps, err := newTree(&spec, true)
	if err == nil && t != nil {
		t.close()
	}
	return caps
}

func validate(s *Spec) *Error {
	if s.StdoutBytes <= 0 || s.StderrBytes <= 0 || s.Wall <= 0 || s.Grace <= 0 || s.Memory.Bytes == 0 {
		return blocked("LIMIT_INVALID", errors.New("every limit must be positive"))
	}
	if !filepath.IsAbs(s.Path) {
		return blocked("TOOL_PATH_NOT_ABSOLUTE", nil)
	}
	info, err := os.Stat(s.Path)
	if err != nil {
		return blocked("TOOL_MISSING", err)
	}
	if !info.Mode().IsRegular() {
		return blocked("TOOL_NOT_REGULAR", nil)
	}
	if !filepath.IsAbs(s.Dir) {
		return blocked("DIR_NOT_ABSOLUTE", nil)
	}
	if info, err := os.Stat(s.Dir); err != nil || !info.IsDir() {
		return blocked("DIR_INVALID", err)
	}
	return nil
}

// Process is a started supervised process.
type Process struct {
	spec    Spec
	cmd     *exec.Cmd
	tree    tree
	caps    Capabilities
	start   time.Time
	stdin   *os.File
	stdout  io.Reader
	outPipe *os.File
	errPipe *os.File
	out     *capture
	errs    *capture
	exited  chan struct{}
	superv  chan struct{} // closed when supervise returns
	waitErr error
	done    chan struct{}
	result  Result

	mu          sync.Mutex
	reason      string
	kill        chan struct{}
	sampledPeak uint64
	sampled     bool
}

// Run starts spec, feeds Stdin, collects bounded stdout/stderr and waits.
func Run(ctx context.Context, spec Spec) (Result, error) {
	if spec.Interactive {
		return Result{}, blocked("SPEC_INVALID", errors.New("Run is not interactive"))
	}
	p, err := Start(ctx, spec)
	if err != nil {
		return Result{}, err
	}
	return p.Wait(), nil
}

// Start validates spec, checks capabilities before any launch and starts the process.
func Start(ctx context.Context, spec Spec) (*Process, error) {
	if ctx == nil {
		return nil, blocked("NIL_CONTEXT", nil)
	}
	if err := validate(&spec); err != nil {
		return nil, err
	}
	t, caps, err := newTree(&spec, false)
	if err != nil {
		return nil, err
	}
	if spec.Memory.Hard && caps.Memory != MemoryHard {
		t.close()
		return nil, blocked("MEMORY_HARD_CAP_UNSUPPORTED", fmt.Errorf("%s backend memory is %s", caps.Backend, caps.Memory))
	}
	if caps.Memory == MemoryUnsupported {
		t.close()
		return nil, blocked("MEMORY_CONTROL_UNSUPPORTED", nil)
	}
	p := &Process{spec: spec, tree: t, caps: caps, exited: make(chan struct{}), superv: make(chan struct{}), done: make(chan struct{}), kill: make(chan struct{})}
	if err := p.launch(); err != nil {
		t.close()
		return nil, err
	}
	go p.supervise(ctx)
	return p, nil
}

func (p *Process) launch() error {
	s := &p.spec
	outR, outW, err := os.Pipe()
	if err != nil {
		return &Error{Code: "START_FAILED", Cause: err}
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		outR.Close()
		outW.Close()
		return &Error{Code: "START_FAILED", Cause: err}
	}
	var inR *os.File
	if s.Interactive || s.Stdin != nil {
		if inR, p.stdin, err = os.Pipe(); err != nil {
			outR.Close()
			outW.Close()
			errR.Close()
			errW.Close()
			return &Error{Code: "START_FAILED", Cause: err}
		}
	}
	env := s.Env
	if env == nil {
		env = []string{}
	}
	p.cmd = &exec.Cmd{Path: s.Path, Args: append([]string{s.Path}, s.Args...), Dir: s.Dir, Env: env, Stdout: outW, Stderr: errW}
	if inR != nil {
		p.cmd.Stdin = inR
	}
	closeAll := func() {
		for _, f := range []*os.File{outR, outW, errR, errW, inR, p.stdin} {
			if f != nil {
				f.Close()
			}
		}
	}
	if err := p.tree.prepare(p.cmd); err != nil {
		closeAll()
		return &Error{Code: "START_FAILED", Cause: err}
	}
	p.start = time.Now()
	if err := p.cmd.Start(); err != nil {
		closeAll()
		return &Error{Code: "START_FAILED", Cause: err}
	}
	if err := p.tree.attach(p.cmd.Process); err != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
		closeAll()
		return &Error{Code: "START_FAILED", Cause: err}
	}
	// The child holds its own ends now; the parent keeps only its sides of the pipes.
	outW.Close()
	errW.Close()
	if inR != nil {
		inR.Close()
	}
	p.outPipe, p.errPipe = outR, errR
	p.errs = newCapture(errR, s.StderrBytes, func() { p.Terminate(ReasonOutput) })
	if s.Interactive {
		p.stdout = &capReader{r: outR, limit: s.StdoutBytes, p: p}
	} else {
		p.out = newCapture(outR, s.StdoutBytes, func() { p.Terminate(ReasonOutput) })
		if s.Stdin != nil {
			go func(w *os.File, data []byte) {
				w.Write(data) // a child that exits without reading ends this with EPIPE
				w.Close()
			}(p.stdin, s.Stdin)
			p.stdin = nil
		}
	}
	go func() {
		p.waitErr = p.cmd.Wait()
		close(p.exited)
	}()
	return nil
}

// Stdin returns the interactive input pipe.
func (p *Process) Stdin() io.WriteCloser { return p.stdin }

// Stdout returns the interactive output; reading past StdoutBytes terminates the tree
// with OUTPUT_LIMIT and returns ErrOutputLimit.
func (p *Process) Stdout() io.Reader { return p.stdout }

// Capabilities returns the backend capabilities used for this process.
func (p *Process) Capabilities() Capabilities { return p.caps }

// ErrOutputLimit is returned by an interactive Stdout read past the cap.
var ErrOutputLimit = errors.New("stdout limit exceeded")

// Terminate stops the whole tree and records reason unless a reason is already set.
func (p *Process) Terminate(reason string) {
	p.mu.Lock()
	if p.reason == "" {
		p.reason = reason
		close(p.kill)
	}
	p.mu.Unlock()
}

func (p *Process) currentReason() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reason
}

func (p *Process) supervise(ctx context.Context) {
	defer close(p.superv)
	wall := time.NewTimer(p.spec.Wall)
	defer wall.Stop()
	var tick <-chan time.Time
	if p.caps.Memory == MemorySampled {
		ticker := time.NewTicker(SampleInterval)
		defer ticker.Stop()
		tick = ticker.C
	}
	for {
		select {
		case <-p.exited:
			return
		case <-ctx.Done():
			p.Terminate(ReasonCancelled)
		case <-wall.C:
			p.Terminate(ReasonWall)
		case <-p.tree.notify():
			p.Terminate(ReasonMemory)
		case <-tick:
			if used, err := p.tree.sample(); err == nil {
				p.mu.Lock()
				if used > p.sampledPeak || !p.sampled {
					p.sampledPeak, p.sampled = used, true
				}
				p.mu.Unlock()
				if used > p.spec.Memory.Bytes {
					p.Terminate(ReasonMemory)
				}
			}
		case <-p.kill:
			select {
			case <-p.exited: // already reaped: Wait handles any residual tree
				return
			default:
			}
			p.stopTree()
			<-p.exited
			if p.spec.Interactive {
				// R1-06: a descendant outside the backend's reach may still hold stdout;
				// unblock a pending interactive read after the grace period.
				time.AfterFunc(p.spec.Grace, func() { p.outPipe.Close() })
			}
			return
		}
	}
}

// stopTree asks politely, waits at most Grace for the tree to leave, then forces.
func (p *Process) stopTree() {
	p.tree.stop(!p.caps.PoliteTermination)
	if p.caps.PoliteTermination {
		if !p.waitEmpty(p.spec.Grace) {
			p.tree.stop(true)
		}
	}
}

func (p *Process) waitEmpty(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if n, err := p.tree.live(); err == nil && n == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Wait waits for the main process, terminates and verifies any remaining descendants,
// collects the bounded output and returns the result. It is safe to call once.
func (p *Process) Wait() Result {
	<-p.exited
	<-p.superv
	r := &p.result
	r.WallMillis = time.Since(p.start).Milliseconds()
	r.Capabilities = p.caps
	r.Cleanup.Method = p.caps.TreeCleanup
	r.Cleanup.Scope = p.caps.TreeCleanup
	r.ExitCode = -1
	if p.cmd.ProcessState != nil {
		r.ExitCode = p.cmd.ProcessState.ExitCode()
	}
	if p.stdin != nil {
		p.stdin.Close()
	}
	// Descendants left after the main process exits are terminated, never adopted.
	if n, err := p.tree.live(); err != nil {
		r.Cleanup.Detail = "tree query failed: " + err.Error()
	} else if n > 0 {
		r.Cleanup.ResidualAfterExit = n
		p.tree.stop(true)
	}
	verified := p.waitEmpty(p.spec.Grace)
	// Output pipes reach EOF only when every holder is gone.
	pipes := make(chan struct{})
	go func() {
		if p.out != nil {
			<-p.out.done
		}
		<-p.errs.done
		close(pipes)
	}()
	select {
	case <-pipes:
	case <-time.After(p.spec.Grace):
		verified = false
		r.Cleanup.Detail = joinDetail(r.Cleanup.Detail, "output pipe still held after grace")
		p.outPipe.Close()
		p.errPipe.Close()
	}
	r.Cleanup.Verified = verified && r.Cleanup.Detail == ""
	if !verified && r.Cleanup.Detail == "" {
		r.Cleanup.Detail = "process tree not empty after grace"
	}
	if p.out != nil {
		r.Stdout, r.StdoutBytes, r.StdoutTruncated = p.out.result()
	} else if cr, ok := p.stdout.(*capReader); ok {
		r.StdoutBytes, r.StdoutTruncated = cr.n, cr.n > cr.limit
	}
	r.Stderr, r.StderrBytes, r.StderrTruncated = p.errs.result()
	p.outPipe.Close()
	p.errPipe.Close()
	r.Memory.Limit = p.spec.Memory.Bytes
	r.Memory.Enforcement = p.caps.Memory
	r.Memory.Metric = p.caps.MemoryMetric
	if peak, ok := p.tree.peak(); ok {
		r.Memory.Peak, r.Memory.PeakKnown = peak, true
	} else {
		p.mu.Lock()
		r.Memory.Peak, r.Memory.PeakKnown = p.sampledPeak, p.sampled
		p.mu.Unlock()
	}
	if p.tree.limitHit() {
		r.Memory.LimitHit = true
		p.Terminate(ReasonMemory)
	}
	if err := p.tree.close(); err != nil {
		r.Cleanup.Verified = false
		r.Cleanup.Detail = joinDetail(r.Cleanup.Detail, "backend release failed: "+err.Error())
	}
	reason := p.currentReason()
	if reason == ReasonMemory {
		r.Memory.LimitHit = true
	}
	switch reason {
	case "":
		r.Reason = ReasonExited
		r.Status = StatusCompleted
		if r.ExitCode != 0 {
			r.Status = StatusFailed
		}
	case ReasonCancelled:
		r.Reason, r.Status = reason, StatusCancelled
	case ReasonWall, ReasonOutput, ReasonMemory:
		r.Reason, r.Status = reason, StatusResourceLimit
	default: // a caller-defined reason such as a protocol watchdog
		r.Reason, r.Status = reason, StatusResourceLimit
	}
	close(p.done)
	return *r
}

func joinDetail(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// capture reads one pipe to EOF, keeping at most limit bytes.
type capture struct {
	buf   bytes.Buffer
	limit int64
	n     int64
	done  chan struct{}
	mu    sync.Mutex
}

func newCapture(r io.Reader, limit int64, over func()) *capture {
	c := &capture{limit: limit, done: make(chan struct{})}
	go func() {
		defer close(c.done)
		chunk := make([]byte, 32*1024)
		signalled := false
		for {
			k, err := r.Read(chunk)
			if k > 0 {
				c.mu.Lock()
				if room := c.limit - int64(c.buf.Len()); room > 0 {
					c.buf.Write(chunk[:min(int64(k), room)])
				}
				c.n += int64(k)
				exceeded := c.n > c.limit
				c.mu.Unlock()
				if exceeded && !signalled {
					signalled = true
					over()
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return c
}

func (c *capture) result() ([]byte, int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return bytes.Clone(c.buf.Bytes()), c.n, c.n > c.limit
}

// capReader enforces the interactive stdout cap.
type capReader struct {
	r     io.Reader
	limit int64
	n     int64
	p     *Process
}

func (c *capReader) Read(b []byte) (int, error) {
	if c.n >= c.limit {
		// Probe one byte so a stream that ends exactly at the cap is not a violation.
		var one [1]byte
		k, err := c.r.Read(one[:])
		if k == 0 {
			return 0, err
		}
		c.n += int64(k)
		c.p.Terminate(ReasonOutput)
		return 0, ErrOutputLimit
	}
	if int64(len(b)) > c.limit-c.n {
		b = b[:c.limit-c.n]
	}
	k, err := c.r.Read(b)
	c.n += int64(k)
	return k, err
}

func baseCaps() Capabilities { return Capabilities{OS: runtime.GOOS + "/" + runtime.GOARCH} }
