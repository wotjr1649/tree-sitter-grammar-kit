package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const AcceptedResultSchema = "tsgk-incremental-result/r7"

func acceptedLedger(data []byte, profile kit.IncrementalProfile, oracle bool) (*kit.ReuseAcceptedLedger, error) {
	if data == nil {
		return nil, nil
	}
	if profile.Preprocess != nil || profile.SvcContext != nil || profile.Declarations != nil || profile.Format != "" || profile.Output != kit.OutputTree || profile.Encoding != kit.EncodingUTF8 ||
		(!oracle && profile.Operation != "native-parse-edit") || (oracle && profile.Operation != "native-query") || !slices.Contains([]string{"windows/amd64", "linux/amd64"}, runtime.GOOS+"/"+runtime.GOARCH) {
		return nil, refuse(kit.KindUnsupported, "REUSE_ACCEPTED_MODE_UNSUPPORTED", nil)
	}
	ledger, err := kit.ParseReuseAcceptedLedger(data)
	if err != nil {
		return nil, refuse(kit.KindInvalidInput, "REUSE_ACCEPTED_LEDGER_INVALID", err)
	}
	if err := ledger.CheckProfile(profile); err != nil {
		return nil, refuse(kit.KindInvalidInput, "REUSE_ACCEPTED_HISTORY_MISMATCH", err)
	}
	return ledger, nil
}

func (x Context) acceptCase(ctx context.Context, c kit.IncrementalCase, source []byte, out *CaseResult) {
	if x.ReuseAccepted == nil {
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		out.ReuseAccepted = &kit.ReuseAcceptedResult{Schema: kit.ReuseAcceptedSchema, Result: ClaimFail, Code: "REUSE_ACCEPTED_PROJECTION_FAILED", Steps: []kit.ReuseAcceptedStep{}}
		x.foldAccepted(out)
		return
	}
	req := kit.ReuseAcceptedRequest{Ledger: x.ReuseAccepted, Route: x.Route, Case: c, Source: source, Raw: out.Raw, Projection: raw, Policy: x.PolicyRef, Platform: runtime.GOOS + "/" + runtime.GOARCH, RecordedPlatform: runtime.GOOS + "/" + runtime.GOARCH}
	if out.acceptedExpected != nil {
		req.Expected = *out.acceptedExpected
	}
	req.Expected.Check = ctx.Err
	proof, err := kit.JudgeReuseAccepted(req)
	if err != nil && proof.Result != ClaimFail {
		proof.Result, proof.Code = ClaimBlocked, "REUSE_ACCEPTED_INCOMPLETE"
	}
	out.ReuseAccepted = &proof
	x.foldAccepted(out)
}

func (x Context) foldAccepted(c *CaseResult) {
	if x.ReuseAccepted == nil || c.ReuseAccepted == nil {
		return
	}
	switch c.ReuseAccepted.Result {
	case ClaimFail:
		if c.Assessment != kit.AssessFail {
			c.Assessment, c.Code = kit.AssessFail, c.ReuseAccepted.Code
		}
	case ClaimBlocked:
		if c.Assessment == kit.AssessPass {
			c.Assessment, c.Code = kit.AssessBlocked, c.ReuseAccepted.Code
		}
	case ClaimPass:
		if c.ExecutionStatus != kit.StatusCompleted || c.Assessment == kit.AssessFail {
			return
		}
		route := ClaimNotClaimed
		for k := 1; k < len(c.Steps); k++ {
			v := stepRoute(c.Steps[k-1], c.Steps[k])
			if v == ClaimBlocked && slices.ContainsFunc(c.ReuseAccepted.Steps, func(s kit.ReuseAcceptedStep) bool { return s.Step == k && s.Result == ClaimPass }) {
				v = ClaimPass
			}
			route = worse(route, v)
		}
		c.Claims.IncrementalRoute = route
		foldAssessment(c)
		if c.Assessment == kit.AssessPass {
			c.Code = ""
		}
	}
}

// Both live runs and holdout capture use held roots and exclusive members.
func writeAcceptedMembers(responses, sources *os.Root, index int, id string, raw, source []byte) error {
	name := fmt.Sprintf("%05d-%s", index, id)
	var err error
	if raw != nil {
		err = createExclusive(responses, name+".json", raw)
	}
	if source == nil {
		return errors.Join(err, errors.New("accepted source witness missing"))
	}
	return errors.Join(err, createExclusive(sources, name+".bin", source))
}
