package reuseobs

// Judge is only the completed logger/parse-argument axis, never CST equality or
// node identity reuse. Host-only markers do not count as native observations.
func Judge(observations []Observation) string {
	claim, native := "PASS", false
	for _, o := range observations {
		if _, err := validate(o, selfBinding(o)); err != nil {
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

// Mandatory is the claim of the revision that requires every native parse to be observed
// (#217). It is the axis of the parse call's start and old-tree argument, not of a complete
// logger collection. A parse is PASS when its observation shows the role's start with its
// arguments: an old tree for incremental with one parse after edit, none for initial and
// fresh with one new parse and no accepted node. A complete observation shows it with
// its done; a TRUNCATED prefix (the cap keeps what came before, and the start precedes
// every lex callback) shows it with exactly that one start and no done, and then proves
// nothing past the cap. An invalid observation, an old-tree argument contradicting the role
// (in any status of a parse that ran), a complete one with other arguments or an accepted
// node in a prefix without old tree is FAIL; any other prefix and an OVERFLOW (no counts)
// are BLOCKED. Any other noncomplete status
// is FAIL in a completed case, whose parses all ran, and BLOCKED in one that ended early.
// Host NO_NATIVE_PARSE markers are not native observations.
func Mandatory(observations []Observation, completed bool) string {
	claim := "NOT_CLAIMED"
	for _, o := range observations {
		v := "PASS"
		_, err := validate(o, selfBinding(o))
		switch {
		case err != nil:
			v = "FAIL"
		case o.Reason == "NO_NATIVE_PARSE":
			continue
		case o.Reason != "PARSE_NOT_RUN" && o.OldTree != (o.Role == "incremental"):
			v = "FAIL"
		case o.Status == "COMPLETE":
			if o.Role == "incremental" && o.Counts.ParseAfterEdit != 1 || o.Role != "incremental" && (o.Counts.NewParse != 1 || o.Counts.Accepted != 0) {
				v = "FAIL"
			}
		case o.Status == "TRUNCATED" && o.Role != "incremental" && o.Counts.Accepted != 0:
			v = "FAIL"
		case o.Status == "TRUNCATED" && o.Counts.ResumeParse == 0 && o.Counts.Done == 0 && (o.Role == "incremental" && o.Counts.ParseAfterEdit == 1 && o.Counts.NewParse == 0 ||
			o.Role != "incremental" && o.Counts.NewParse == 1 && o.Counts.ParseAfterEdit == 0):
			// the prefix holds the parse's one start event with the role's arguments
		case o.Status == "TRUNCATED" || o.Status == "OVERFLOW":
			v = "BLOCKED"
		case completed:
			v = "FAIL"
		default:
			v = "BLOCKED"
		}
		if rank[v] > rank[claim] {
			claim = v
		}
	}
	return claim
}

var rank = map[string]int{"NOT_CLAIMED": 0, "PASS": 1, "BLOCKED": 2, "FAIL": 3}

// selfBinding checks an observation's internal consistency only; callers bind it to the
// request or replayed step separately.
func selfBinding(o Observation) Binding {
	previous := ""
	if o.OldSourceSHA256 != nil {
		previous = *o.OldSourceSHA256
	}
	return Binding{RuntimeManifestSHA256: o.RuntimeManifestSHA256, RequestID: o.RequestID, RunID: o.RunID, ProducerSHA256: o.ProducerSHA256,
		HistoryStart: o.HistoryStart, Step: o.Step, Role: o.Role, SourceSHA256: o.SourceSHA256, SourceBytes: o.SourceBytes, PreviousSourceSHA256: previous}
}

// Step is one public step of a case: Native says a native parse ran in it and Edit that the
// parse continued an old tree (an edit step that is not a segment restart).
type Step struct {
	Native, Edit bool
	Observations []Observation
}

// CaseClaim is Mandatory over a case's steps, where a step without exactly its roles in order
// (initial, or incremental then fresh) or whose host NO_NATIVE_PARSE marker contradicts its
// native parse is missing: FAIL in a completed case, BLOCKED in one that ended early. Live
// execution and replay share it so both read the same steps the same way.
func CaseClaim(steps []Step, completed bool) string {
	var all []Observation
	missing := false
	for _, s := range steps {
		roles := []string{"initial"}
		if s.Edit {
			roles = []string{"incremental", "fresh"}
		}
		missing = missing || len(s.Observations) != len(roles)
		for j, o := range s.Observations {
			missing = missing || j >= len(roles) || o.Role != roles[j] || s.Native == (o.Reason == "NO_NATIVE_PARSE")
		}
		all = append(all, s.Observations...)
	}
	claim := Mandatory(all, completed)
	if v := map[bool]string{true: "FAIL", false: "BLOCKED"}[completed]; missing && rank[v] > rank[claim] {
		claim = v
	}
	return claim
}
