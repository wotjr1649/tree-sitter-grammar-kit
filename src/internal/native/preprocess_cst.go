package native

import (
	"context"
	"runtime"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// runPreprocessPrimary isolates the structural request from active expectations and ranges.
// Its full tree exists only in raw; the outer result keeps the small supervision receipt.
func (b *Build) runPreprocessPrimary(ctx context.Context, x Context, c kit.IncrementalCase, source []byte, out *CaseResult) bool {
	primaryCase := kit.IncrementalCase{ID: c.ID, Input: c.Input, Encoding: c.Encoding, Edits: c.Edits}
	primaryContext := Context{Op: x.Op, Route: x.Route, Output: kit.OutputTree, CgroupParent: x.CgroupParent,
		PolicyRef: kit.NativePolicy(x.Op, kit.OutputTree, nil), Protocol: ProtocolR3,
		ObservationRunID: x.ObservationRunID, ObservationProducerSHA256: b.Identity}
	wall := x.Op.ParseWall
	if len(c.Edits) > 0 {
		wall = x.Op.EditWall
	}
	wall = min(wall, x.Op.RunWall)
	if deadline, ok := ctx.Deadline(); ok {
		wall = min(wall, max(time.Nanosecond, time.Until(deadline)))
	}
	primary := b.RunCase(ctx, primaryContext, primaryCase, source)
	p := &kit.PreprocessPrimary{Schema: kit.PreprocessPrimarySchema, Role: "primary-full-source", Policy: primaryContext.PolicyRef,
		ExecutionStatus: primary.ExecutionStatus, Code: primary.Code}
	if len(primary.Raw) > 0 && primary.ResponseStatus != "" {
		p.ExecutionStatus, p.Code = primary.ResponseStatus, primary.ResponseCode
	}
	if r := primary.Process; r != nil {
		p.Process = &kit.PreprocessProcess{Status: r.Status, Reason: r.Reason, ExitCode: r.ExitCode,
			CleanupVerified: r.Cleanup.Verified, CleanupScope: r.Cleanup.Scope, StdoutBytes: r.StdoutBytes, StdoutTruncated: r.StdoutTruncated,
			WallMillis: r.WallMillis, WallBudgetMillis: max(int64(1), (wall + time.Millisecond - 1).Milliseconds()), MemoryLimitBytes: r.Memory.Limit,
			MemoryEnforcement: r.Memory.Enforcement, PeakBytes: r.Memory.Peak, PeakKnown: r.Memory.PeakKnown, MemoryLimitHit: r.Memory.LimitHit}
	}
	out.PrimaryRaw = primary.Raw
	primary.Steps, primary.Process = nil, nil
	result := kit.CheckPreprocessCST(kit.PreprocessCSTRequest{Case: primaryCase, Source: source, Preprocess: x.Preprocess, Raw: out.PrimaryRaw, Primary: p,
		Operation: x.Op, Platform: runtime.GOOS + "/" + runtime.GOARCH, RunID: x.ObservationRunID, ProducerSHA256: b.Identity})
	out.PreprocessCST = &result
	switch result.Result {
	case "PASS":
		return true
	case "FAIL":
		finishEarly(out, kit.StatusNotRun, kit.AssessFail, result.Code)
	default:
		finishEarly(out, kit.StatusNotRun, kit.AssessBlocked, result.Code)
	}
	return false
}
