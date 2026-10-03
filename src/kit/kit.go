// Package kit is the public offline API of tree-sitter-grammar-kit. It reads only the
// caller's explicit root and selection: no Git, Node, shell, compiler, target code,
// network, stdout/stderr, os.Exit, chdir, environment change or file creation.
package kit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Report schema and E0 axis values.
const (
	ReportSchema = "tsgk-report/r1"

	StatusNotRun        = "NOT_RUN"
	StatusCompleted     = "COMPLETED"
	StatusFailed        = "FAILED"
	StatusCancelled     = "CANCELLED"
	StatusResourceLimit = "RESOURCE_LIMIT"

	ModeNewRun = "NEW_RUN"
	ModeNotRun = "NOT_RUN"

	AssessBlocked     = "BLOCKED"
	AssessNotAssessed = "NOT_ASSESSED"
)

// Error kinds.
const (
	KindInvalidInput  = "INVALID_INPUT"
	KindIO            = "IO"
	KindCancelled     = "CANCELLED"
	KindResourceLimit = "RESOURCE_LIMIT"
	KindUnsupported   = "UNSUPPORTED"
)

// Error is the typed failure of an operation. Kind and Code are stable machine values;
// the Error string is not a machine identity.
type Error struct {
	Kind, Code, Path string
	Cause            error
}

func (e *Error) Error() string {
	s := e.Kind + ": " + e.Code
	if e.Path != "" {
		s += ": " + e.Path
	}
	if e.Cause != nil {
		s += ": " + e.Cause.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Cause }

func fail(kind, code, path string, cause error) *Error {
	return &Error{Kind: kind, Code: code, Path: path, Cause: cause}
}

// Limits are finite operation bounds; every field must be positive. Wall is the
// kit-set duration: reaching it ends RESOURCE_LIMIT, while the caller context's own
// cancellation or deadline ends CANCELLED.
type Limits struct {
	Files, FileBytes, TotalBytes, Depth, OutputBytes uint64
	Wall                                             time.Duration
}

// DefaultLimits returns the adopted offline-inspect bounds used by the CLI.
func DefaultLimits() Limits {
	return Limits{Files: 10000, FileBytes: 16777216, TotalBytes: 268435456, Depth: 64, OutputBytes: 16777216, Wall: 120 * time.Second}
}

func (l Limits) valid() bool {
	return l.Files > 0 && l.FileBytes > 0 && l.TotalBytes > 0 && l.Depth > 0 && l.OutputBytes > 0 && l.Wall > 0
}

// Selection chooses the grammar directory and, optionally, an exact file list.
type Selection struct {
	Grammar string          // exact "." root sentinel, otherwise a portable relative directory
	Files   []FileSelection // nil: documented bounded discovery; empty non-nil: invalid
}

// FileSelection is one explicitly selected file.
type FileSelection struct {
	Path string `json:"path"`
	Role string `json:"role"`
}

// Report is the E0 envelope shared by every command.
type Report struct {
	Schema          string        `json:"schema"`
	Command         string        `json:"command"`
	ExecutionStatus string        `json:"execution_status"`
	EvidenceMode    string        `json:"evidence_mode"`
	Assessment      string        `json:"assessment"`
	Identities      []IdentityRef `json:"identities"`
	Findings        []Finding     `json:"findings"`
	Coverage        Coverage      `json:"coverage"`
}

// IdentityRef names one bound identity of the operation.
type IdentityRef struct {
	Role   string `json:"role"`
	Schema string `json:"schema"`
	SHA256 string `json:"sha256"`
}

// Finding is a stable machine code with a root-relative path and a Korean message.
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

// Coverage separates what was requested, observed, and not supported.
type Coverage struct {
	Requested   []string `json:"requested"`
	Observed    []string `json:"observed"`
	Unsupported []string `json:"unsupported"`
}

// Policy makes the scan policy and limits visible; its canonical text is hashed into
// the "policy" identity, so raised limits are a different policy identity.
type Policy struct {
	Operation        string   `json:"operation"`
	Discovery        string   `json:"discovery"`
	Files            uint64   `json:"files"`
	Records          uint64   `json:"records,omitempty"`
	FileBytes        uint64   `json:"file_bytes"`
	TotalBytes       uint64   `json:"total_bytes"`
	Depth            uint64   `json:"depth"`
	OutputBytes      uint64   `json:"output_bytes"`
	WallMillis       int64    `json:"wall_ms"`
	LargeFileProfile string   `json:"large_file_profile,omitempty"`
	EncodingPolicy   string   `json:"encoding_policy,omitempty"`
	ArchiveProfile   string   `json:"archive_profile,omitempty"`
	ArchiveEntries   uint64   `json:"archive_entries,omitempty"`
	ArchiveBytes     uint64   `json:"archive_bytes,omitempty"`
	ArchiveDepth     uint64   `json:"archive_depth,omitempty"`
	Exclusions       []string `json:"exclusions"`
}

func (p Policy) text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "tsgk-policy/r1\noperation=%s\ndiscovery=%s\nfiles=%d\nrecords=%d\nfile_bytes=%d\ntotal_bytes=%d\ndepth=%d\noutput_bytes=%d\nwall_ms=%d\nlarge_file_profile=%s\nencoding=%s\n",
		p.Operation, p.Discovery, p.Files, p.Records, p.FileBytes, p.TotalBytes, p.Depth, p.OutputBytes, p.WallMillis, p.LargeFileProfile, p.EncodingPolicy)
	if p.ArchiveProfile != "" { // archive lines exist only for archive operations
		fmt.Fprintf(&b, "archive_profile=%s\narchive_entries=%d\narchive_bytes=%d\narchive_depth=%d\n", p.ArchiveProfile, p.ArchiveEntries, p.ArchiveBytes, p.ArchiveDepth)
	}
	for _, x := range p.Exclusions {
		fmt.Fprintf(&b, "exclude=%s\n", x)
	}
	return b.String()
}

func (p Policy) ref() IdentityRef {
	sum := sha256.Sum256([]byte(p.text()))
	return IdentityRef{Role: "policy", Schema: "tsgk-policy/r1", SHA256: hex.EncodeToString(sum[:])}
}

func newReport(command string) Report {
	return Report{Schema: ReportSchema, Command: command, ExecutionStatus: StatusCompleted, EvidenceMode: ModeNewRun, Assessment: AssessNotAssessed,
		Identities: []IdentityRef{}, Findings: []Finding{}, Coverage: Coverage{Requested: []string{}, Observed: []string{}, Unsupported: []string{}}}
}

// failReport records an error on the report; a failed report is never COMPLETED or PASS.
func failReport(r *Report, err *Error) {
	switch err.Kind {
	case KindInvalidInput:
		r.ExecutionStatus, r.EvidenceMode, r.Assessment = StatusNotRun, ModeNotRun, AssessNotAssessed
	case KindUnsupported:
		r.ExecutionStatus, r.EvidenceMode, r.Assessment = StatusNotRun, ModeNotRun, AssessBlocked
	case KindCancelled:
		r.ExecutionStatus, r.Assessment = StatusCancelled, AssessNotAssessed
	case KindResourceLimit:
		r.ExecutionStatus, r.Assessment = StatusResourceLimit, AssessBlocked
	default:
		r.ExecutionStatus, r.Assessment = StatusFailed, AssessNotAssessed
	}
	r.Findings = append(r.Findings, Finding{Code: err.Code, Severity: "error", Path: err.Path, Message: messageFor(err)})
}

func messageFor(err *Error) string {
	switch err.Kind {
	case KindInvalidInput:
		return "입력이 계약을 위반해 실행하지 않았다"
	case KindUnsupported:
		return "이 build가 지원하지 않는 기능이다"
	case KindCancelled:
		return "호출자가 취소했거나 호출자 deadline이 지났다"
	case KindResourceLimit:
		return "kit 한도에 도달해 결과를 완성하지 않았다"
	}
	return "입출력 실패로 결과를 완성하지 않았다"
}

// run holds per-call state: caller context, kit wall, limits accounting.
type run struct {
	caller    context.Context
	wall      context.Context
	cancel    context.CancelFunc
	totalRead uint64
}

var errWall = errors.New("kit wall limit")

func startRun(ctx context.Context, wall time.Duration) (*run, *Error) {
	if ctx == nil {
		return nil, fail(KindInvalidInput, "NIL_CONTEXT", "", nil)
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, fail(KindInvalidInput, "DEADLINE_REQUIRED", "", nil)
	}
	wctx, cancel := context.WithTimeoutCause(context.Background(), wall, errWall)
	r := &run{caller: ctx, wall: wctx, cancel: cancel}
	if err := r.check(); err != nil {
		cancel()
		return nil, err
	}
	return r, nil
}

// check is the cancellation checkpoint: caller cancellation wins over the kit wall.
func (r *run) check() *Error {
	if err := r.caller.Err(); err != nil {
		return fail(KindCancelled, "CANCELLED", "", err)
	}
	if r.wall.Err() != nil {
		return fail(KindResourceLimit, "WALL_LIMIT", "", context.Cause(r.wall))
	}
	return nil
}

// sealOutput enforces OutputBytes on the exact JSON encoding the CLI writes.
func sealOutput(v any, limit uint64) *Error {
	data, err := json.Marshal(v)
	if err != nil {
		return fail(KindIO, "ENCODE_FAILED", "", err)
	}
	if uint64(len(data))+1 > limit {
		return fail(KindResourceLimit, "OUTPUT_LIMIT", "", nil)
	}
	return nil
}

func asError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return fail(KindIO, "IO_FAILED", "", err)
}
