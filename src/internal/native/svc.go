package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// SvcReferenceResult binds a separately parsed registered C# case to the composite.
type SvcReferenceResult struct {
	Case            string          `json:"case"`
	Input           kit.NativeInput `json:"input"`
	ExecutionStatus string          `json:"execution_status"`
	Assessment      string          `json:"assessment"`
	HasError        bool            `json:"has_error"`
}

func (b *Build) runSvcReferences(ctx context.Context, x Context, cases []kit.IncrementalCase, root string) map[string]CaseResult {
	out := map[string]CaseResult{}
	if x.SvcContext == nil {
		return out
	}
	declared := x.SvcContext.CodeBehind
	x.Format = ""
	x.SvcContext = nil
	for _, r := range declared {
		if _, found := out[r.Case]; found {
			continue
		}
		for _, c := range cases {
			if c.ID != r.Case {
				continue
			}
			src, err := readCase(root, c.Input)
			if err != nil {
				out[c.ID] = CaseResult{ID: c.ID, Input: c.Input, Encoding: c.Encoding, ExecutionStatus: kit.StatusNotRun, Assessment: kit.AssessBlocked, Code: err.(*Error).Code, Claims: Claims{ClaimNotClaimed, ClaimNotClaimed, ClaimNotClaimed}}
				break
			}
			o := kit.ObserveServiceHost(c.Encoding, src)
			if o.HasDirectivePrefix(c.Encoding, src) {
				out[c.ID] = CaseResult{ID: c.ID, Input: c.Input, Encoding: c.Encoding, ExecutionStatus: kit.StatusNotRun, Assessment: kit.AssessBlocked, Code: "SVC_REFERENCE_INVALID", Claims: Claims{ClaimNotClaimed, ClaimNotClaimed, ClaimNotClaimed}}
				break
			}
			out[c.ID] = b.RunCase(ctx, x, c, src)
			break
		}
	}
	return out
}

func linkSvcReferences(out *CaseResult, refs map[string]CaseResult) {
	seen := map[string]bool{}
	for i := range out.Steps {
		composite := out.Steps[i].Composite
		if composite == nil || composite.CodeBehind == nil || composite.CodeBehind.Case == "" {
			continue
		}
		id := composite.CodeBehind.Case
		r, found := refs[id]
		if !found {
			continue
		}
		composite.CodeBehind.Resolution = "PARSED"
		if r.ExecutionStatus != kit.StatusCompleted {
			composite.CodeBehind.Resolution = "NOT_PARSED"
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		hasError := false
		for _, s := range r.Steps {
			if s.Incremental != nil && s.Incremental.HasError {
				hasError = true
			}
		}
		assessment := r.Assessment
		if r.ExecutionStatus != kit.StatusCompleted {
			assessment = kit.AssessBlocked
		}
		out.References = append(out.References, SvcReferenceResult{Case: id, Input: r.Input, ExecutionStatus: r.ExecutionStatus, Assessment: assessment, HasError: hasError})
	}
	if len(out.References) > 0 && out.ExecutionStatus == kit.StatusCompleted {
		if out.Assessment == kit.AssessBlocked && out.Code != "" {
			out.Claims.Expectations = worse(out.Claims.Expectations, ClaimBlocked)
		}
		foldAssessment(out)
	}
}

// SvcSegment is one contiguous native history; no reuse crosses its start boundary.
type SvcSegment struct {
	StartStep int            `json:"start_step"`
	EndStep   int            `json:"end_step"`
	Process   *runner.Result `json:"process"`
	Claims    Claims         `json:"claims"`
}

func (b *Build) runSvcSegments(ctx context.Context, x Context, c kit.IncrementalCase, versions [][]byte, points []kit.EditPoints, svc []kit.SvcObservation, out CaseResult) CaseResult {
	expectations := map[int]ExpectationResult{}
	for k := 0; k < len(versions); {
		if svc[k].IncludedRanges == nil {
			sum := sha256.Sum256(versions[k])
			in := TreeInput{Bytes: uint64(len(versions[k])), SHA256: hex.EncodeToString(sum[:]), Encoding: c.Encoding, EncodingSource: kit.SourceDeclaration}
			step := StepResult{Step: k, SourceBytes: in.Bytes, SourceSHA256: in.SHA256, Composite: composite(svc[k], in, []kit.IdentityRef{{Role: "producer", Schema: BuildSchema, SHA256: b.Identity}, {Role: "source", Schema: "tsgk-source-bytes/r1", SHA256: in.SHA256}, x.PolicyRef}, nil)}
			if k > 0 {
				step.Edit = svcStepEdit(c.Edits[k-1], points[k-1])
			}
			out.Steps = append(out.Steps, step)
			for _, e := range c.Expect {
				if e.Step == k {
					local := e
					local.Step = 0
					res, _ := evaluateSvc([]kit.StepExpectation{local}, nil, svc[k:k+1])
					res[0].Step = k
					expectations[k] = res[0]
				}
			}
			k++
			continue
		}
		end := k + 1
		for end < len(versions) && svc[end].IncludedRanges != nil {
			end++
		}
		part := c
		part.Edits = c.Edits[k : end-1]
		part.Expect = nil
		for _, e := range c.Expect {
			if e.Step >= k && e.Step < end {
				e.Step -= k
				part.Expect = append(part.Expect, e)
			}
		}
		result := b.RunCase(ctx, x, part, versions[k])
		out.Segments = append(out.Segments, SvcSegment{StartStep: k, EndStep: end, Process: result.Process, Claims: result.Claims})
		if result.ExecutionStatus != kit.StatusCompleted {
			out.ExecutionStatus, out.Assessment, out.Code = result.ExecutionStatus, result.Assessment, result.Code
			return out
		}
		out.Producer = result.Producer
		for _, s := range result.Steps {
			s.Step += k
			if s.Step == k && k > 0 {
				s.Restarted = true
				s.Edit = svcStepEdit(c.Edits[k-1], points[k-1])
			}
			out.Steps = append(out.Steps, s)
		}
		for _, e := range result.Expectations {
			e.Step += k
			expectations[e.Step] = e
		}
		if result.Oracle != nil {
			if out.Oracle == nil {
				v := *result.Oracle
				out.Oracle = &v
			} else {
				out.Oracle.QueryEquality = worse(out.Oracle.QueryEquality, result.Oracle.QueryEquality)
				out.Oracle.API = worse(out.Oracle.API, result.Oracle.API)
			}
		}
		k = end
	}
	out.ExecutionStatus = kit.StatusCompleted
	out.Claims.Expectations = ClaimNotClaimed
	if len(c.Expect) > 0 {
		out.Claims.Expectations = ClaimPass
	}
	for _, e := range c.Expect {
		r := expectations[e.Step]
		out.Expectations = append(out.Expectations, r)
		out.Claims.Expectations = worse(out.Claims.Expectations, r.Result)
	}
	// Whole-history reuse/equality is not claimed across an unparsed boundary.
	out.Claims.IncrementalEquality, out.Claims.IncrementalRoute = ClaimNotClaimed, ClaimNotClaimed
	if out.Oracle != nil {
		out.Oracle.QueryEquality = ClaimNotClaimed
	}
	foldAssessment(&out)
	if out.Claims.Expectations == ClaimFail {
		out.Code = "EXPECTATION_FAILED"
	}
	if out.Claims.Expectations == ClaimBlocked {
		out.Code = "SVC_EXPECTATION_UNASSESSABLE"
	}
	if len(c.Expect) == 0 {
		out.Assessment, out.Code = svcObservationOnly(svc, false)
		if out.Code == "SVC_INLINE_NOT_PARSED" {
			out.Code = "SVC_PARTIAL_HISTORY"
		}
	}
	return out
}

func svcStepEdit(edit kit.Edit, point kit.EditPoints) *StepEdit {
	return &StepEdit{StartByte: edit.StartByte, OldEndByte: edit.OldEndByte, NewEndByte: edit.NewEndByte, StartPoint: [2]uint32{point.Start.Row, point.Start.Column}, OldEndPoint: [2]uint32{point.OldEnd.Row, point.OldEnd.Column}, NewEndPoint: [2]uint32{point.NewEnd.Row, point.NewEnd.Column}}
}
