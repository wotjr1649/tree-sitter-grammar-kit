package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// ResultSchema is the published incremental result.
const ResultSchema = "tsgk-incremental-result/r1"

// Capabilities a caller must grant to `tsgk incremental`.
const (
	AllowBuild = "BUILD_NATIVE"
	AllowExec  = "EXEC_NATIVE"
)

// IncrementalRequest is one `tsgk incremental` run. Paths are absolute.
type IncrementalRequest struct {
	Root         string // case inputs
	GrammarRoot  string // grammar files; empty means Root
	Profile      []byte
	Runtime      string
	Compiler     string
	Work         string
	Out          string
	Allow        []string
	CgroupParent string
	RunWall      time.Duration // optional, at most the operation's run wall
}

// Summary counts case outcomes.
type Summary struct {
	Cases       int            `json:"cases"`
	Statuses    map[string]int `json:"execution_statuses"`
	Assessments map[string]int `json:"assessments"`
	Codes       map[string]int `json:"codes"`
	HasError    int            `json:"has_error"`
	Batches     int            `json:"batches"`
	Requeued    int            `json:"requeued"`
}

// Result is the published tsgk-incremental-result/r1 document.
type Result struct {
	kit.Report
	ResultSchema  string              `json:"result_schema"`
	ProfileID     string              `json:"profile_id"`
	Route         string              `json:"route"`
	Operation     kit.NativeOperation `json:"operation"`
	Output        string              `json:"output"`
	Build         *Build              `json:"build"`
	BuildRemoved  string              `json:"build_removed"`
	Summary       Summary             `json:"summary"`
	Cases         []CaseResult        `json:"cases"`
	Batches       []BatchRecord       `json:"batches"`
	Capabilities  runner.Capabilities `json:"capabilities"`
	Platform      string              `json:"platform"`
	StartedAt     string              `json:"started_at"`
	WallMillis    int64               `json:"wall_ms"`
	ProfileSHA256 string              `json:"profile_sha256"`
}

// BatchRecord is one batch process of a batch operation.
type BatchRecord struct {
	Index         int           `json:"index"`
	Cases         []string      `json:"cases"`
	Process       runner.Result `json:"process"`
	TrailingBytes int64         `json:"trailing_bytes"`
	Fatal         string        `json:"fatal,omitempty"`
}

// buildFailure names the cause of a failed build: the refusal error and, for a failed
// compiler step, that step's stderr tail (already bounded to 4096 bytes per step).
func buildFailure(err error, b *Build) string {
	msg := "driver build이 완료되지 않았다: " + err.Error()
	if b != nil && len(b.Steps) > 0 {
		if s := b.Steps[len(b.Steps)-1]; s.Stderr != "" {
			msg += "\n" + s.Name + " stderr: " + s.Stderr
		}
	}
	return msg
}

func finding(code, severity, path, msg string) kit.Finding {
	return kit.Finding{Code: code, Severity: severity, Path: path, Message: msg}
}

// readCase reads one declared case input below root, rejecting links and identity drift.
func readCase(root string, in kit.NativeInput) ([]byte, error) {
	cur := root
	for _, seg := range strings.Split(in.Path, "/") {
		cur = filepath.Join(cur, seg)
		info, err := os.Lstat(cur)
		if err != nil {
			return nil, refuse(kit.KindInvalidInput, "SOURCE_MISSING", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || (info.Mode()&os.ModeType != 0 && !info.IsDir()) {
			return nil, refuse(kit.KindInvalidInput, "SOURCE_LINK_REJECTED", nil)
		}
	}
	data, err := os.ReadFile(cur)
	if err != nil {
		return nil, refuse(kit.KindIO, "SOURCE_UNREADABLE", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != in.SHA256 || uint64(len(data)) != in.Bytes {
		return nil, refuse(kit.KindInvalidInput, "SOURCE_MISMATCH", nil)
	}
	return data, nil
}

// Incremental builds the driver for the profile and runs every case. A refusal before
// the build returns an error and creates nothing; later failures are recorded per case.
func Incremental(ctx context.Context, req IncrementalRequest) (Result, error) {
	start := time.Now()
	res := Result{Report: kit.Report{Schema: kit.ReportSchema, Command: "incremental", ExecutionStatus: kit.StatusNotRun, EvidenceMode: kit.ModeNotRun,
		Assessment: kit.AssessNotAssessed, Identities: []kit.IdentityRef{}, Findings: []kit.Finding{}, Coverage: kit.Coverage{Requested: []string{}, Observed: []string{}, Unsupported: []string{"query"}}},
		ResultSchema: ResultSchema, Cases: []CaseResult{}, Batches: []BatchRecord{}, Platform: runtime.GOOS + "/" + runtime.GOARCH, StartedAt: start.UTC().Format(time.RFC3339)}
	stop := func(err *Error) (Result, error) {
		res.Findings = append(res.Findings, finding(err.Code, "error", "", "실행 전 거부했다"))
		if err.Kind == kit.KindUnsupported {
			res.Assessment = kit.AssessBlocked
		}
		return res, err
	}
	granted := map[string]bool{}
	for _, a := range req.Allow {
		granted[a] = true
	}
	if !granted[AllowBuild] || !granted[AllowExec] {
		return stop(refuse(kit.KindUnsupported, "CAPABILITY_NOT_GRANTED", nil))
	}
	prof, err := kit.ParseIncrementalProfile(req.Profile)
	if err != nil {
		var ke *kit.Error
		errors.As(err, &ke)
		return stop(refuse(ke.Kind, ke.Code, ke))
	}
	op := kit.NativeOperations()[prof.Operation]
	res.ProfileID, res.Route, res.Operation, res.Output, res.ProfileSHA256 = prof.ID, prof.Route, op, prof.Output, prof.SHA256
	if _, err := os.Lstat(req.Out); err == nil {
		return stop(refuse(kit.KindInvalidInput, "OUTPUT_EXISTS", nil))
	}
	probe := runner.Probe(runner.Spec{Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: runtime.GOOS != "darwin"}, CgroupParent: req.CgroupParent})
	res.Capabilities = probe
	if runtime.GOOS != "darwin" && probe.Memory != runner.MemoryHard {
		return stop(refuse(kit.KindUnsupported, "MEMORY_HARD_CAP_UNSUPPORTED", nil))
	}
	if err := os.MkdirAll(filepath.Join(req.Out, "responses"), 0o755); err != nil {
		return stop(refuse(kit.KindIO, "OUTPUT_FAILED", err))
	}
	if req.RunWall > 0 && req.RunWall < op.RunWall {
		op.RunWall = req.RunWall // a caller may only narrow the run wall; recorded in operation and policy
		res.Operation = op
	}
	wall := op.RunWall
	writeFailed := false
	runCtx, cancel := context.WithTimeout(ctx, wall)
	defer cancel()
	groot := req.GrammarRoot
	if groot == "" {
		groot = req.Root
	}
	b, berr := NewBuild(runCtx, BuildRequest{Work: req.Work, Runtime: req.Runtime, GrammarRoot: groot, Grammar: prof.Grammar, Symbol: prof.Symbol,
		Compiler: req.Compiler, CompilerID: prof.Compiler, CgroupParent: req.CgroupParent})
	res.Build = b
	res.ExecutionStatus, res.EvidenceMode = kit.StatusCompleted, kit.ModeNewRun
	policy := PolicyRef(op, prof.Output)
	res.Identities = append(res.Identities, kit.IdentityRef{Role: "profile", Schema: kit.IncrementalSchema, SHA256: prof.SHA256}, policy)
	if berr != nil {
		var ne *Error
		errors.As(berr, &ne)
		res.ExecutionStatus, res.Assessment = kit.StatusFailed, kit.AssessNotAssessed
		if ne.Kind == kit.KindInvalidInput {
			res.ExecutionStatus = kit.StatusNotRun
		}
		res.Findings = append(res.Findings, finding(ne.Code, "error", "", buildFailure(berr, b)))
	} else {
		res.Identities = append(res.Identities, kit.IdentityRef{Role: "producer", Schema: BuildSchema, SHA256: b.Identity})
		x := Context{Op: op, Route: prof.Route, Output: prof.Output, Declarations: prof.Declarations, Format: prof.Format, CgroupParent: req.CgroupParent, PolicyRef: policy}
		if op.Batch {
			res.Cases, res.Batches = runBatches(runCtx, b, x, req.Root, prof.Cases)
		} else {
			for _, c := range prof.Cases {
				src, err := readCase(req.Root, c.Input)
				if err != nil {
					res.Cases = append(res.Cases, CaseResult{ID: c.ID, Input: c.Input, Encoding: c.Encoding, ExecutionStatus: kit.StatusNotRun, Assessment: kit.AssessNotAssessed,
						Code: err.(*Error).Code, Claims: Claims{ClaimNotClaimed, ClaimNotClaimed, ClaimNotClaimed}, Steps: []StepResult{}, Expectations: []ExpectationResult{}})
					continue
				}
				cr := b.RunCase(runCtx, x, c, src)
				if cr.Raw != nil {
					// The case order makes the name unique on case-insensitive filesystems too.
					name := fmt.Sprintf("%05d-%s.json", len(res.Cases), c.ID)
					if err := os.WriteFile(filepath.Join(req.Out, "responses", name), cr.Raw, 0o644); err != nil {
						res.Findings = append(res.Findings, finding("EVIDENCE_WRITE_FAILED", "error", c.ID, "원 응답을 보존하지 못했다"))
						writeFailed = true
					}
				}
				res.Cases = append(res.Cases, cr)
			}
		}
		res.ExecutionStatus, res.Assessment = aggregate(res.Cases)
		if writeFailed {
			res.ExecutionStatus = kit.StatusFailed
			if res.Assessment == kit.AssessPass {
				res.Assessment = kit.AssessNotAssessed
			}
		}
	}
	if b != nil {
		res.BuildRemoved = "REMOVED"
		if err := b.Remove(); err != nil {
			res.BuildRemoved = "FAILED: " + err.Error()
			res.Findings = append(res.Findings, finding("BUILD_CLEANUP_FAILED", "error", "", "build 디렉터리를 지우지 못했다"))
			res.ExecutionStatus = kit.StatusFailed
			if res.Assessment == kit.AssessPass {
				res.Assessment = kit.AssessNotAssessed
			}
		}
	}
	res.Summary = summarize(res.Cases, res.Batches)
	res.WallMillis = time.Since(start).Milliseconds()
	res.Coverage.Requested = []string{"edit-steps", "fresh-comparison", "route-instrumentation", "expectations"}
	res.Coverage.Observed = []string{}
	if res.ExecutionStatus == kit.StatusCompleted {
		res.Coverage.Observed = res.Coverage.Requested
	}
	data, err := json.Marshal(res)
	if err == nil {
		err = os.WriteFile(filepath.Join(req.Out, "result.json"), append(data, '\n'), 0o644)
	}
	if err != nil {
		res.Findings = append(res.Findings, finding("EVIDENCE_WRITE_FAILED", "error", "", "결과를 쓰지 못했다"))
		return res, refuse(kit.KindIO, "EVIDENCE_WRITE_FAILED", err)
	}
	return res, nil
}

// aggregate: any case not run or not completed makes the run noncomplete; the
// assessment is the worst case assessment (FAIL > BLOCKED > NOT_ASSESSED > PASS).
func aggregate(cases []CaseResult) (string, string) {
	status, assess := kit.StatusCompleted, kit.AssessPass
	rank := map[string]int{kit.AssessPass: 0, kit.AssessNotAssessed: 1, kit.AssessBlocked: 2, kit.AssessFail: 3}
	for _, c := range cases {
		switch c.ExecutionStatus {
		case kit.StatusCompleted:
		case kit.StatusCancelled:
			status = kit.StatusCancelled
		case kit.StatusResourceLimit:
			if status == kit.StatusCompleted {
				status = kit.StatusResourceLimit
			}
		default:
			if status == kit.StatusCompleted || status == kit.StatusResourceLimit {
				status = kit.StatusFailed
			}
		}
		if rank[c.Assessment] > rank[assess] {
			assess = c.Assessment
		}
	}
	if len(cases) == 0 {
		status, assess = kit.StatusNotRun, kit.AssessNotAssessed
	}
	return status, assess
}

func summarize(cases []CaseResult, batches []BatchRecord) Summary {
	s := Summary{Cases: len(cases), Statuses: map[string]int{}, Assessments: map[string]int{}, Codes: map[string]int{}, Batches: len(batches)}
	for _, c := range cases {
		s.Statuses[c.ExecutionStatus]++
		s.Assessments[c.Assessment]++
		if c.Code != "" {
			s.Codes[c.Code]++
		}
		if len(c.Steps) > 0 && c.Steps[0].Incremental != nil && c.Steps[0].Incremental.HasError {
			s.HasError++
		}
	}
	return s
}

// runBatches runs record requests through length-prefixed frames, at most BatchFiles
// cases or BatchBytes input bytes per driver process. Frames a batch-level limit or a
// fatal frame left unanswered are re-queued into a new process (not a retry); a crash
// mid-frame is FAILED; files never reached before the run wall are NOT_RUN.
func runBatches(ctx context.Context, b *Build, x Context, root string, cases []kit.IncrementalCase) ([]CaseResult, []BatchRecord) {
	out := make([]CaseResult, len(cases))
	for i, c := range cases {
		out[i] = CaseResult{ID: c.ID, Input: c.Input, Encoding: c.Encoding, ExecutionStatus: kit.StatusNotRun, Assessment: kit.AssessNotAssessed, Code: "NOT_RUN",
			Claims: Claims{ClaimNotClaimed, ClaimNotClaimed, ClaimNotClaimed}, Steps: []StepResult{}, Expectations: []ExpectationResult{}}
	}
	batches := []BatchRecord{}
	queue := make([]int, 0, len(cases))
	for i := range cases {
		queue = append(queue, i)
	}
	pol := runner.BatchPolicy{RequestBytes: MaxRequestFrame, ResponseBytes: uint32(x.Op.OutputBytes), FrameWall: x.Op.FrameWall, FrameGrace: 5 * time.Second,
		StdoutBytes: 268435456, BatchWall: x.Op.BatchWall}
	for len(queue) > 0 && ctx.Err() == nil {
		var idx []int
		var frames []runner.Frame
		var reqs []Request
		var total uint64
		for len(queue) > 0 && len(idx) < x.Op.BatchFiles {
			i := queue[0]
			if len(idx) > 0 && total+cases[i].Input.Bytes > x.Op.BatchBytes {
				break
			}
			queue = queue[1:]
			c := cases[i]
			src, err := readCase(root, c.Input)
			if err == nil {
				if _, _, e := kit.ApplyEdits(c.Encoding, src, nil, x.Op.InputBytes); e != nil {
					var ke *kit.Error
					errors.As(e, &ke)
					err = refuse(kit.KindInvalidInput, ke.Code, nil)
				}
			}
			if err != nil {
				out[i].Code = err.(*Error).Code
				out[i].Assessment = kit.AssessBlocked
				continue
			}
			total += c.Input.Bytes
			r := x.request(c, src)
			r.Points, r.Edits = nil, nil
			if x.Format == kit.SvcFormat {
				o := kit.ObserveServiceHost(c.Encoding, src)
				sum := sha256.Sum256(src)
				in := TreeInput{Bytes: uint64(len(src)), SHA256: hex.EncodeToString(sum[:]), Encoding: c.Encoding, EncodingSource: kit.SourceDeclaration}
				out[i].Steps = []StepResult{{Step: 0, SourceBytes: in.Bytes, SourceSHA256: in.SHA256, Composite: composite(o, in, []kit.IdentityRef{x.PolicyRef}, nil)}}
				if o.IncludedRanges == nil { // directive observation only: no frame
					out[i].ExecutionStatus = kit.StatusCompleted
					out[i].Assessment, out[i].Code = svcObservationOnly([]kit.SvcObservation{o}, false)
					continue
				}
				r.Ranges = [][]kit.Span{o.IncludedRanges}
			}
			idx = append(idx, i)
			reqs = append(reqs, r)
			frames = append(frames, runner.Frame{ID: c.ID, Request: r.Encode()})
		}
		if len(frames) == 0 {
			continue
		}
		if err := b.Verify(); err != nil {
			for _, i := range idx {
				out[i].Code, out[i].Assessment = "EXECUTABLE_MISMATCH", kit.AssessBlocked
			}
			continue
		}
		spec := runner.Spec{Path: b.Executable, Args: []string{"batch"}, Dir: b.Dir, StderrBytes: 65536, Grace: 5 * time.Second,
			Memory: runner.Memory{Bytes: x.Op.MemoryBytes, Hard: runtime.GOOS != "darwin"}, CgroupParent: x.CgroupParent}
		br, err := runner.RunBatch(ctx, spec, pol, frames)
		rec := BatchRecord{Index: len(batches), Process: br.Process, TrailingBytes: br.TrailingBytes}
		for _, i := range idx {
			rec.Cases = append(rec.Cases, cases[i].ID)
		}
		if err != nil {
			for _, i := range idx {
				out[i].Code = "BATCH_START_FAILED"
			}
			rec.Fatal = "BATCH_START_FAILED"
			batches = append(batches, rec)
			break
		}
		fatal := false
		var requeue []int
		for k, fr := range br.Frames {
			i := idx[k]
			c := &out[i]
			switch {
			case fatal:
				requeue = append(requeue, i) // the driver ended deliberately before this frame
				continue
			case fr.Status == runner.FrameRequeued:
				requeue = append(requeue, i)
				continue
			case fr.Status == runner.FrameResourceLimit:
				c.ExecutionStatus, c.Assessment, c.Code = kit.StatusResourceLimit, kit.AssessBlocked, fr.Detail
				continue
			case fr.Status == runner.FrameFailed:
				c.ExecutionStatus, c.Code = kit.StatusFailed, "BATCH_FRAME_FAILED"
				continue
			}
			resp, err := DecodeResponse(fr.Response)
			if err == nil {
				want := map[string]int{kit.StatusCompleted: 0, "INVALID_REQUEST": 2, kit.StatusResourceLimit: 3, kit.StatusFailed: 4}
				var checked Checked
				checked, err = Check(resp, reqs[k], [][]byte{reqs[k].Source}, nil, want[resp.Status])
				if err == nil {
					*c = recordResult(*c, resp, checked, x)
					if resp.Code == "ALLOCATION_LIMIT" {
						fatal = true
						rec.Fatal = "ALLOCATION_LIMIT"
					}
				}
			}
			if err != nil {
				c.ExecutionStatus, c.Assessment, c.Code = kit.StatusFailed, kit.AssessNotAssessed, err.(*Error).Code
			}
		}
		if br.TrailingBytes > 0 || !br.Process.Cleanup.Verified || (br.Process.ExitCode != 0 && !fatal && br.Process.Status == runner.StatusCompleted) {
			// The batch broke the protocol or cleanup: none of its frames is accepted.
			code := "RESPONSE_TRAILING_BYTES"
			if !br.Process.Cleanup.Verified {
				code = "PROCESS_CLEANUP_UNVERIFIED"
			} else if br.TrailingBytes == 0 {
				code = "BATCH_EXIT_NONZERO"
			}
			for _, i := range idx {
				if out[i].ExecutionStatus == kit.StatusCompleted {
					out[i].ExecutionStatus, out[i].Assessment, out[i].Code = kit.StatusFailed, kit.AssessNotAssessed, code
				}
			}
			rec.Fatal = code
		}
		batches = append(batches, rec)
		queue = append(requeue, queue...)
	}
	return out, batches
}

// recordResult keeps only the retained record of a corpus file: status, has_error,
// descendant count, digest, the capped ERROR/MISSING list and the encoding.
func recordResult(c CaseResult, resp Response, checked Checked, x Context) CaseResult {
	var comp *SvcComposite
	if len(c.Steps) == 1 {
		comp = c.Steps[0].Composite // the directive observation made before the frame
	}
	c.Steps = nil
	c.ResponseStatus, c.ResponseCode, c.Producer = resp.Status, resp.Code, &resp.Producer
	switch resp.Status {
	case kit.StatusCompleted:
		c.ExecutionStatus, c.Assessment, c.Code = kit.StatusCompleted, kit.AssessPass, ""
	case kit.StatusResourceLimit:
		c.ExecutionStatus, c.Assessment, c.Code = kit.StatusResourceLimit, kit.AssessBlocked, resp.Code
	case "INVALID_REQUEST":
		c.ExecutionStatus, c.Assessment, c.Code = kit.StatusFailed, kit.AssessNotAssessed, "DRIVER_REJECTED_"+resp.Code
	default:
		c.ExecutionStatus, c.Assessment, c.Code = kit.StatusFailed, kit.AssessNotAssessed, resp.Code
	}
	for k, cs := range checked.Steps {
		w := cs.Incremental.Wire
		t := TreeOut{Status: cs.Incremental.Status, Code: cs.Incremental.Code, Form: cs.Incremental.Form, ParseMillis: w.ParseMillis,
			DescendantCount: w.DescendantCount, MaxDepth: w.MaxDepth, HasError: w.HasError, Digest: w.Digest}
		if w.Errors != nil {
			s := &SummaryEnvelope{Schema: "tsgk-tree-record/r1", Errors: summaryErrors(w.Errors), Declarations: x.decls(w.Declarations), Digest: SummaryDigest{kit.TreeDigestScheme, w.Digest},
				DescendantCount: w.DescendantCount, Status: cs.Incremental.Status, PartialTrees: []PartialTree{}, Identities: []kit.IdentityRef{}}
			t.Summary = s
		}
		c.Steps = append(c.Steps, StepResult{Step: k, SourceBytes: cs.Step.SourceBytes, SourceSHA256: cs.Step.SourceSHA256, Incremental: &t, Composite: comp})
	}
	return c
}
