package native

import (
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// observeCase sets the case's mandatory observation claim (#217) and folds it into a
// completed case's assessment last, so every return path of a case, segment or batch frame
// keeps it: a FAIL replaces the code of a case it turns from another assessment to FAIL.
func (x Context) observeCase(c *CaseResult) {
	if x.Protocol != ProtocolR3 {
		return
	}
	steps := make([]reuseobs.Step, len(c.Steps))
	for i := range c.Steps {
		s := &c.Steps[i]
		native := s.Incremental != nil || s.Composite != nil && s.Composite.Inline != nil
		if s.Incremental == nil && s.Composite != nil && s.Composite.Inline == nil && len(s.Observations) == 0 {
			roles := []string{"initial"}
			if s.Step > 0 {
				roles = []string{"incremental", "fresh"}
			}
			for _, role := range roles {
				s.Observations = append(s.Observations, reuseobs.Observation{Schema: reuseobs.Schema,
					Policy: reuseobs.Policy, RuntimeManifestSHA256: reuseobs.RuntimeManifestSHA256,
					RequestID: c.ID, RunID: x.ObservationRunID, ProducerSHA256: x.ObservationProducerSHA256,
					Step: uint64(s.Step), Role: role, SourceSHA256: s.SourceSHA256, SourceBytes: s.SourceBytes,
					Status: "UNAVAILABLE", Reason: "NO_NATIVE_PARSE", Counts: &reuseobs.Counts{}})
			}
		}
		steps[i] = reuseobs.Step{Native: native, Edit: s.Step > 0 && !s.Restarted, Observations: s.Observations}
	}
	completed := c.ExecutionStatus == kit.StatusCompleted
	c.ReuseObservationClaim = reuseobs.CaseClaim(steps, completed)
	if !completed {
		return
	}
	switch c.ReuseObservationClaim {
	case ClaimFail:
		if c.Assessment != kit.AssessFail {
			c.Assessment, c.Code = kit.AssessFail, "REUSE_OBSERVATION_FAILED"
		}
	case ClaimBlocked:
		if c.Assessment == kit.AssessPass {
			c.Assessment, c.Code = kit.AssessBlocked, "REUSE_OBSERVATION_LIMIT"
		}
	}
}
