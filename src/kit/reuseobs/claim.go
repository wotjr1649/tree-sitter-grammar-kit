package reuseobs

// Judge is only the completed logger/parse-argument axis, never CST equality or
// node identity reuse. Host-only markers do not count as native observations.
func Judge(observations []Observation) string {
	claim, native := "PASS", false
	for _, o := range observations {
		previous := ""
		if o.OldSourceSHA256 != nil {
			previous = *o.OldSourceSHA256
		}
		_, err := validate(o, Binding{RuntimeManifestSHA256: o.RuntimeManifestSHA256, RequestID: o.RequestID, RunID: o.RunID, ProducerSHA256: o.ProducerSHA256,
			HistoryStart: o.HistoryStart, Step: o.Step, Role: o.Role, SourceSHA256: o.SourceSHA256, SourceBytes: o.SourceBytes, PreviousSourceSHA256: previous})
		if err != nil {
			native = true
			if claim != "FAIL" {
				claim = "BLOCKED"
			}
			continue
		}
		if o.Reason == "NO_NATIVE_PARSE" {
			continue
		}
		native = true
		if o.Status != "COMPLETE" {
			if claim != "FAIL" {
				claim = "BLOCKED"
			}
		} else if o.Counts == nil || (o.Role == "incremental" && (!o.OldTree || o.Counts.ParseAfterEdit != 1)) ||
			((o.Role == "initial" || o.Role == "fresh") && (o.OldTree || o.Counts.NewParse != 1 || o.Counts.Accepted != 0)) {
			claim = "FAIL"
		}
	}
	if !native {
		return "NOT_CLAIMED"
	}
	return claim
}
