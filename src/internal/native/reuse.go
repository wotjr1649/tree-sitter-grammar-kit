package native

import "github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"

// observeCase keeps the logger judgement separate from all existing case gates.
func (x Context) observeCase(c *CaseResult) {
	if x.Protocol != ProtocolR3 {
		return
	}
	var all []reuseobs.Observation
	missing := false
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
		roles := []string{"initial"}
		if s.Step > 0 && !s.Restarted {
			roles = []string{"incremental", "fresh"}
		}
		if len(s.Observations) != len(roles) {
			missing = true
		} else {
			for j, role := range roles {
				if s.Observations[j].Role != role || native == (s.Observations[j].Reason == "NO_NATIVE_PARSE") {
					missing = true
				}
			}
		}
		all = append(all, s.Observations...)
	}
	c.ReuseObservationClaim = reuseobs.Judge(all)
	if missing && c.ReuseObservationClaim != ClaimFail {
		c.ReuseObservationClaim = ClaimBlocked
	}
}
