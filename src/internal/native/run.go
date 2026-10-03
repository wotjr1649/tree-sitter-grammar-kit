package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// Claim values.
const (
	ClaimPass       = "PASS"
	ClaimFail       = "FAIL"
	ClaimBlocked    = "BLOCKED"
	ClaimNotClaimed = "NOT_CLAIMED"
)

// Claims keep the three judgements of a case apart: incremental = fresh at every edit
// step, the observed incremental route, and the independent language expectations.
type Claims struct {
	IncrementalEquality string `json:"incremental_equality"`
	IncrementalRoute    string `json:"incremental_route"`
	Expectations        string `json:"expectations"`
}

// TreeInput is the input of a tree envelope: original bytes and the declared encoding.
type TreeInput struct {
	Bytes          uint64 `json:"bytes"`
	SHA256         string `json:"sha256"`
	Encoding       string `json:"encoding"`
	EncodingSource string `json:"encoding_source"`
}

// TreeEnvelope is the public tsgk-tree/r1 full tree.
type TreeEnvelope struct {
	Schema       string            `json:"schema"`
	Input        TreeInput         `json:"input"`
	Status       string            `json:"status"`
	Capabilities map[string]string `json:"capabilities"`
	Nodes        []kit.TreeNode    `json:"nodes"`
	Captures     []struct{}        `json:"captures"`
	Identities   []kit.IdentityRef `json:"identities"`
}

// SummaryEnvelope is the public tsgk-tree-summary/r1 summary.
type SummaryEnvelope struct {
	Schema          string            `json:"schema"`
	Input           TreeInput         `json:"input"`
	Status          string            `json:"status"`
	Identities      []kit.IdentityRef `json:"identities"`
	Reason          string            `json:"reason"`
	DescendantCount uint64            `json:"descendant_count"`
	Digest          SummaryDigest     `json:"digest"`
	Errors          SummaryErrors     `json:"errors"`
	Declarations    SummaryDecls      `json:"declarations"`
	PartialTrees    []PartialTree     `json:"partial_trees"`
}

// SummaryDigest names the digest canonicalization.
type SummaryDigest struct {
	Canonicalization string `json:"canonicalization"`
	SHA256           string `json:"sha256"`
}

// SummaryErrors is the capped ERROR/MISSING list with points as objects.
type SummaryErrors struct {
	Limit     uint64         `json:"limit"`
	Total     uint64         `json:"total"`
	Truncated bool           `json:"truncated"`
	Items     []SummaryError `json:"items"`
}

// SummaryError is one ERROR or MISSING node.
type SummaryError struct {
	Kind       string    `json:"kind"`
	Type       string    `json:"type"`
	StartByte  uint32    `json:"start_byte"`
	EndByte    uint32    `json:"end_byte"`
	StartPoint kit.Point `json:"start_point"`
	EndPoint   kit.Point `json:"end_point"`
}

// SummaryDecls is the declaration structure check.
type SummaryDecls struct {
	Mapping    string     `json:"mapping"`
	Route      string     `json:"route"`
	Assessment string     `json:"assessment"`
	Items      []DeclItem `json:"items"`
}

// PartialTree is one registered-point partial tree.
type PartialTree struct {
	Point     string         `json:"point"`
	Byte      uint32         `json:"byte"`
	Truncated bool           `json:"truncated"`
	Nodes     []kit.TreeNode `json:"nodes"`
}

// TreeOut is one step tree in the report.
type TreeOut struct {
	Status          string           `json:"status"`
	Code            string           `json:"code"`
	Form            string           `json:"form"`
	ParseMillis     uint64           `json:"parse_ms"`
	DescendantCount uint64           `json:"descendant_count"`
	MaxDepth        uint64           `json:"max_depth"`
	HasError        bool             `json:"has_error"`
	Digest          string           `json:"digest"`
	Tree            *TreeEnvelope    `json:"tree,omitempty"`
	Summary         *SummaryEnvelope `json:"summary,omitempty"`
}

// RouteOut is the route instrumentation with its judgement.
type RouteOut struct {
	Route
	Proven bool `json:"proven"`
}

// Comparison is the incremental/fresh comparison of one edit step.
type Comparison struct {
	Equal bool                `json:"equal"`
	First *kit.TreeDifference `json:"first_difference"`
}

// SvcInline is the inline C# part of a composite: its included ranges and tree.
type SvcInline struct {
	IncludedRanges []kit.Span    `json:"included_ranges"`
	Tree           *TreeEnvelope `json:"tree"`
}

// SvcComposite is the tsgk-svc-composite/r1 record of one step.
type SvcComposite struct {
	Schema     string             `json:"schema"`
	Format     string             `json:"format"`
	Input      TreeInput          `json:"input"`
	Identities []kit.IdentityRef  `json:"identities"`
	Directive  *kit.SvcDirective  `json:"directive"`
	Language   kit.SvcLanguage    `json:"language"`
	CodeBehind *kit.SvcCodeBehind `json:"code_behind"`
	Inline     *SvcInline         `json:"inline"`
	Coverage   kit.SvcCoverage    `json:"coverage"`
}

func composite(o kit.SvcObservation, in TreeInput, ids []kit.IdentityRef, tree *TreeEnvelope) *SvcComposite {
	c := &SvcComposite{Schema: kit.SvcCompositeSchema, Format: kit.SvcFormat, Input: in, Identities: ids, Directive: o.Directive, Language: o.Language, CodeBehind: o.CodeBehind, Coverage: o.Coverage}
	if o.IncludedRanges != nil {
		c.Inline = &SvcInline{IncludedRanges: o.IncludedRanges, Tree: tree}
	}
	return c
}

// StepResult is one step of a case.
type StepResult struct {
	Composite    *SvcComposite `json:"composite,omitempty"`
	Step         int           `json:"step"`
	SourceBytes  uint64        `json:"source_bytes"`
	SourceSHA256 string        `json:"source_sha256"`
	Edit         *StepEdit     `json:"edit"`
	Route        *RouteOut     `json:"route"`
	Comparison   *Comparison   `json:"comparison"`
	Incremental  TreeOut       `json:"incremental"`
	Fresh        *TreeOut      `json:"fresh"`
}

// ExpectationResult is one evaluated expectation.
type ExpectationResult struct {
	kit.StepExpectation
	Result string `json:"result"`
	Detail string `json:"detail,omitempty"`
}

// CaseResult is the outcome of one registered case.
type CaseResult struct {
	ID              string              `json:"id"`
	Input           kit.NativeInput     `json:"input"`
	Encoding        string              `json:"encoding"`
	ExecutionStatus string              `json:"execution_status"`
	Assessment      string              `json:"assessment"`
	Code            string              `json:"code"`
	Claims          Claims              `json:"claims"`
	ResponseStatus  string              `json:"response_status"`
	ResponseCode    string              `json:"response_code"`
	Producer        *Producer           `json:"producer"`
	Process         *runner.Result      `json:"process"`
	Steps           []StepResult        `json:"steps"`
	Expectations    []ExpectationResult `json:"expectations"`
	Raw             []byte              `json:"-"`
}

// Context is the per-profile information a case run needs.
type Context struct {
	Op           kit.NativeOperation
	Route        string
	Output       string
	Declarations *kit.Declarations
	Format       string
	CgroupParent string
	PolicyRef    kit.IdentityRef
}

// PolicyRef binds the operation limits, output form and encoding into a policy identity.
func PolicyRef(op kit.NativeOperation, output string) kit.IdentityRef {
	l := LimitsFor(op)
	text := fmt.Sprintf("tsgk-native-policy/r1\noperation=%s\noutput=%s\nlimits=%+v\nparse_wall=%s\nedit_wall=%s\nmax_edits=%d\nbatch=%t %d %d %s %s\n",
		op.Name, output, l, op.ParseWall, op.EditWall, op.MaxEdits, op.Batch, op.BatchFiles, op.BatchBytes, op.FrameWall, op.BatchWall)
	s := sha256.Sum256([]byte(text))
	return kit.IdentityRef{Role: "policy", Schema: "tsgk-native-policy/r1", SHA256: hex.EncodeToString(s[:])}
}

func (x Context) request(c kit.IncrementalCase, source []byte) Request {
	r := Request{ID: c.ID, Encoding: c.Encoding, Output: x.Output, Limits: LimitsFor(x.Op), Points: c.Points, Source: source, Edits: c.Edits}
	if x.Declarations != nil {
		r.Declarations = x.Declarations.Items
	}
	return r
}

// Exec launches the driver on one framed request and returns the process result. It is
// the single launch path for cases and the protocol tests.
func (b *Build) Exec(ctx context.Context, mode string, stdin []byte, wall time.Duration, op kit.NativeOperation, cgroup string) (runner.Result, error) {
	if err := b.Verify(); err != nil {
		return runner.Result{}, err
	}
	spec := runner.Spec{Path: b.Executable, Args: []string{mode}, Dir: b.Dir, Env: nil, Stdin: stdin, StdoutBytes: int64(op.OutputBytes) + 4, StderrBytes: 65536,
		Wall: wall, Grace: 5 * time.Second, Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: runtime.GOOS != "darwin"}, CgroupParent: cgroup}
	return runner.Run(ctx, spec)
}

// RunCase validates the case, runs one driver process for it and assesses the result.
func (b *Build) RunCase(ctx context.Context, x Context, c kit.IncrementalCase, source []byte) CaseResult {
	out := CaseResult{ID: c.ID, Input: c.Input, Encoding: c.Encoding, Claims: Claims{ClaimNotClaimed, ClaimNotClaimed, ClaimNotClaimed}, Steps: []StepResult{}, Expectations: []ExpectationResult{}}
	notRun := func(status, assess, code string) CaseResult {
		out.ExecutionStatus, out.Assessment, out.Code = status, assess, code
		return out
	}
	versions, points, err := kit.ApplyEdits(c.Encoding, source, c.Edits, x.Op.InputBytes)
	if err != nil {
		var ke *kit.Error
		errors.As(err, &ke)
		return notRun(kit.StatusNotRun, kit.AssessNotAssessed, ke.Code)
	}
	req := x.request(c, source)
	var svc []kit.SvcObservation
	if x.Format == kit.SvcFormat {
		for _, v := range versions {
			svc = append(svc, kit.ObserveServiceHost(c.Encoding, v))
		}
		for _, o := range svc {
			req.Ranges = append(req.Ranges, o.IncludedRanges)
		}
		if slices.ContainsFunc(svc, func(o kit.SvcObservation) bool { return o.IncludedRanges == nil }) {
			// No C# inline code to parse in some step: the composite is the directive
			// observation alone and no driver process runs.
			for k, o := range svc {
				sum := sha256.Sum256(versions[k])
				in := TreeInput{Bytes: uint64(len(versions[k])), SHA256: hex.EncodeToString(sum[:]), Encoding: c.Encoding, EncodingSource: kit.SourceDeclaration}
				out.Steps = append(out.Steps, StepResult{Step: k, SourceBytes: in.Bytes, SourceSHA256: in.SHA256, Composite: composite(o, in, []kit.IdentityRef{x.PolicyRef}, nil)})
			}
			out.ExecutionStatus, out.Assessment = kit.StatusCompleted, kit.AssessPass
			if svc[0].IncludedRanges != nil || len(c.Expect) > 0 {
				out.Assessment, out.Code = kit.AssessBlocked, "SVC_INLINE_NOT_PARSED"
			}
			return out
		}
	}
	payload := req.Encode()
	if len(payload) > MaxRequestFrame {
		return notRun(kit.StatusResourceLimit, kit.AssessBlocked, "REQUEST_TOO_LARGE")
	}
	wall := x.Op.ParseWall
	if len(c.Edits) > 0 {
		wall = x.Op.EditWall
	}
	res, err := b.Exec(ctx, "single", Frame(payload), wall, x.Op, x.CgroupParent)
	if err != nil {
		var ne *Error
		if errors.As(err, &ne) {
			return notRun(kit.StatusNotRun, kit.AssessBlocked, ne.Code)
		}
		var re *runner.Error
		if errors.As(err, &re) {
			return notRun(kit.StatusNotRun, kit.AssessBlocked, re.Code)
		}
		return notRun(kit.StatusFailed, kit.AssessNotAssessed, "START_FAILED")
	}
	out.Process = &res
	switch {
	case res.Status == runner.StatusCancelled:
		return notRun(kit.StatusCancelled, kit.AssessNotAssessed, "CANCELLED")
	case res.Status == runner.StatusResourceLimit:
		return notRun(kit.StatusResourceLimit, kit.AssessBlocked, res.Reason)
	case !res.Cleanup.Verified:
		return notRun(kit.StatusFailed, kit.AssessNotAssessed, "PROCESS_CLEANUP_UNVERIFIED")
	}
	frame, err := SplitFrame(res.Stdout)
	if err != nil {
		return notRun(kit.StatusFailed, kit.AssessNotAssessed, err.(*Error).Code)
	}
	out.Raw = frame
	resp, err := DecodeResponse(frame)
	if err != nil {
		return notRun(kit.StatusFailed, kit.AssessNotAssessed, err.(*Error).Code)
	}
	checked, err := Check(resp, req, versions, points, res.ExitCode)
	if err != nil {
		return notRun(kit.StatusFailed, kit.AssessNotAssessed, err.(*Error).Code)
	}
	out.ResponseStatus, out.ResponseCode, out.Producer = resp.Status, resp.Code, &resp.Producer
	build := kit.IdentityRef{Role: "producer", Schema: BuildSchema, SHA256: b.Identity}
	for k, cs := range checked.Steps {
		sr := StepResult{Step: k, SourceBytes: cs.Step.SourceBytes, SourceSHA256: cs.Step.SourceSHA256, Edit: cs.Step.Edit}
		in := TreeInput{Bytes: cs.Step.SourceBytes, SHA256: cs.Step.SourceSHA256, Encoding: c.Encoding, EncodingSource: kit.SourceDeclaration}
		ids := []kit.IdentityRef{build, {Role: "source", Schema: "tsgk-source-bytes/r1", SHA256: cs.Step.SourceSHA256}, x.PolicyRef}
		sr.Incremental = x.treeOut(cs.Incremental, in, ids, c.Points)
		if cs.Fresh != nil {
			f := x.treeOut(*cs.Fresh, in, ids, c.Points)
			sr.Fresh = &f
		}
		if svc != nil {
			sr.Composite = composite(svc[k], in, ids, sr.Incremental.Tree)
		}
		if cs.Step.Route != nil {
			r := RouteOut{Route: *cs.Step.Route}
			r.Proven = r.EditHasChanges && r.ReusedNodes > 0 && r.FreshReusedNodes == 0
			sr.Route = &r
		}
		if k > 0 && cs.Fresh != nil && cs.Incremental.Form == "full" && cs.Fresh.Form == "full" {
			d := kit.CompareTrees(cs.Incremental.Nodes, cs.Fresh.Nodes)
			sr.Comparison = &Comparison{Equal: d == nil, First: d}
		}
		out.Steps = append(out.Steps, sr)
	}
	switch resp.Status {
	case "INVALID_REQUEST":
		// The kit validated the request first: a driver rejection is a disagreement.
		return notRun(kit.StatusFailed, kit.AssessNotAssessed, "DRIVER_REJECTED_"+resp.Code)
	case kit.StatusResourceLimit:
		return notRun(kit.StatusResourceLimit, kit.AssessBlocked, resp.Code)
	case kit.StatusFailed:
		return notRun(kit.StatusFailed, kit.AssessNotAssessed, resp.Code)
	}
	out.ExecutionStatus = kit.StatusCompleted
	if len(c.Edits) > 0 {
		out.Claims.IncrementalEquality, out.Claims.IncrementalRoute = ClaimPass, ClaimPass
		for _, s := range out.Steps[1:] {
			if s.Comparison == nil {
				out.Claims.IncrementalEquality = ClaimBlocked
			} else if !s.Comparison.Equal {
				out.Claims.IncrementalEquality = ClaimFail
				if out.Code == "" {
					out.Code = fmt.Sprintf("INCREMENTAL_FRESH_MISMATCH_STEP_%d", s.Step)
				}
			}
			if s.Route == nil || !s.Route.Proven {
				out.Claims.IncrementalRoute = ClaimFail
				if out.Code == "" {
					out.Code = fmt.Sprintf("INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_%d", s.Step)
				}
			}
		}
	}
	out.Expectations, out.Claims.Expectations = evaluate(c.Expect, checked.Steps)
	if out.Claims.Expectations == ClaimFail && out.Code == "" {
		out.Code = "EXPECTATION_FAILED"
	}
	out.Assessment = kit.AssessPass
	for _, cl := range []string{out.Claims.IncrementalEquality, out.Claims.IncrementalRoute, out.Claims.Expectations} {
		switch cl {
		case ClaimFail:
			out.Assessment = kit.AssessFail
		case ClaimBlocked:
			if out.Assessment == kit.AssessPass {
				out.Assessment = kit.AssessBlocked
			}
		}
	}
	return out
}

func (x Context) treeOut(t Tree, in TreeInput, ids []kit.IdentityRef, points []kit.NativePoint) TreeOut {
	w := t.Wire
	o := TreeOut{Status: t.Status, Code: t.Code, Form: t.Form, ParseMillis: w.ParseMillis, DescendantCount: w.DescendantCount, MaxDepth: w.MaxDepth, HasError: w.HasError, Digest: w.Digest}
	switch t.Form {
	case "full":
		o.Tree = &TreeEnvelope{Schema: kit.TreeSchema, Input: in, Status: t.Status, Capabilities: map[string]string{"query": "UNSUPPORTED"}, Nodes: t.Nodes, Identities: ids}
	case "summary":
		s := &SummaryEnvelope{Schema: kit.TreeSummarySchema, Input: in, Status: t.Status, Identities: ids, Reason: w.Reason, DescendantCount: w.DescendantCount,
			Digest: SummaryDigest{kit.TreeDigestScheme, w.Digest}, Errors: summaryErrors(w.Errors), Declarations: x.decls(w.Declarations), PartialTrees: []PartialTree{}}
		for i, p := range w.PartialTrees {
			s.PartialTrees = append(s.PartialTrees, PartialTree{Point: p.Point, Byte: p.Byte, Truncated: p.Truncated, Nodes: t.Partials[i]})
		}
		o.Summary = s
	}
	return o
}

func summaryErrors(w *WireErrors) SummaryErrors {
	s := SummaryErrors{Limit: w.Limit, Total: w.Total, Truncated: w.Truncated, Items: []SummaryError{}}
	for _, e := range w.Items {
		s.Items = append(s.Items, SummaryError{e.Kind, e.Type, e.StartByte, e.EndByte, kit.Point{Row: e.StartPoint[0], Column: e.StartPoint[1]}, kit.Point{Row: e.EndPoint[0], Column: e.EndPoint[1]}})
	}
	return s
}

// DeclAssessment applies the summary rule: no items NOT_APPLICABLE, all PASS PASS, else FAIL.
func DeclAssessment(items []DeclItem) string {
	if len(items) == 0 {
		return "NOT_APPLICABLE"
	}
	for _, it := range items {
		if it.Status != "PASS" {
			return kit.AssessFail
		}
	}
	return kit.AssessPass
}

func (x Context) decls(items *[]DeclItem) SummaryDecls {
	if x.Declarations == nil || items == nil {
		return SummaryDecls{Route: x.Route, Assessment: kit.AssessNotAssessed, Items: []DeclItem{}}
	}
	return SummaryDecls{Mapping: x.Declarations.Mapping, Route: x.Route, Assessment: DeclAssessment(*items), Items: slices.Clone(*items)}
}

func evaluate(expect []kit.StepExpectation, steps []CheckedStep) ([]ExpectationResult, string) {
	out := []ExpectationResult{}
	claim := ClaimNotClaimed
	if len(expect) > 0 {
		claim = ClaimPass
	}
	for _, e := range expect {
		r := ExpectationResult{StepExpectation: e, Result: ClaimPass}
		t := steps[e.Step].Incremental
		switch {
		case e.Syntax == "NO_ERROR" && t.Wire.HasError:
			r.Result, r.Detail = ClaimFail, "root has_error"
		case e.Syntax == "ERROR" && !t.Wire.HasError:
			r.Result, r.Detail = ClaimFail, "root has no error"
		}
		if r.Result == ClaimPass && len(e.Contains) > 0 {
			if t.Form != "full" {
				r.Result, r.Detail = ClaimBlocked, "contains needs a full tree"
			} else {
				seen := map[string]bool{}
				for _, n := range t.Nodes {
					if n.Named {
						seen[n.Type] = true
					}
				}
				for _, ty := range e.Contains {
					if !seen[ty] {
						r.Result, r.Detail = ClaimFail, "missing named node "+ty
						break
					}
				}
			}
		}
		if r.Result == ClaimPass && e.Declarations != "" {
			if t.Wire.Declarations == nil {
				r.Result, r.Detail = ClaimBlocked, "no declarations"
			} else if got := DeclAssessment(*t.Wire.Declarations); got != e.Declarations {
				r.Result, r.Detail = ClaimFail, "declarations "+got
			}
		}
		switch r.Result {
		case ClaimFail:
			claim = ClaimFail
		case ClaimBlocked:
			if claim == ClaimPass {
				claim = ClaimBlocked
			}
		}
		out = append(out, r)
	}
	return out, claim
}
