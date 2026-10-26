package kit

import (
	"slices"
	"unicode/utf8"
)

// QualReuseAcceptedRegistration selects the fixed original ledger and a separate control.
type QualReuseAcceptedRegistration struct {
	Policy       string `json:"policy"`
	LedgerSHA256 string `json:"ledger_sha256"`
	Control      string `json:"control"`
}

// QualReuseAcceptedCounts keeps missing evidence in the registered denominator.
type QualReuseAcceptedCounts struct {
	Expected int `json:"expected"`
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Blocked  int `json:"blocked"`
	Missing  int `json:"missing"`
}

// QualReuseAcceptedCase retains each normal proof or independently checked omit control.
type QualReuseAcceptedCase struct {
	Set     string              `json:"set"`
	ID      string              `json:"id"`
	Control string              `json:"control"`
	Result  string              `json:"result"`
	Code    string              `json:"code"`
	Steps   []ReuseAcceptedStep `json:"steps"`
}

// QualReuseAcceptedAxis separates normal histories and targets from negative controls.
type QualReuseAcceptedAxis struct {
	Result    string                  `json:"result"`
	Originals QualReuseAcceptedCounts `json:"original_histories"`
	Targets   QualReuseAcceptedCounts `json:"targets"`
	Controls  QualReuseAcceptedCounts `json:"controls"`
	Cases     []QualReuseAcceptedCase `json:"cases"`
}

func qualificationHasCST(schema string) bool {
	return schema == qualificationInventoryR7 || schema == QualificationInventorySchema
}

func qualAcceptedPins(route string) (start, end int) {
	if route == "tsql" {
		return 0, 12
	}
	if route == "json" {
		return 12, 15
	}
	return 0, 0
}

func qualAcceptedWorkload(w QualWorkload) bool {
	a := w.ReuseAccepted
	start, end := qualAcceptedPins(w.Route)
	if a == nil || start == end || a.Policy != ReuseAcceptedSchema || a.LedgerSHA256 != ReuseAcceptedLedgerSHA256 ||
		!slices.Contains([]string{"normal", "omit-old-tree"}, a.Control) || w.Operation != "native-parse-edit" || w.Output != OutputTree ||
		w.Preprocess != nil || w.SvcContext != nil || w.Format != "" || w.API || w.FactPack != nil || w.Declarations != nil || len(w.Queries) != 0 ||
		!hex40.MatchString(w.RuntimeCommit) || len(w.Cases) != end-start {
		return false
	}
	for i, c := range w.Cases {
		if c.ID != reuseAcceptedPins[start+i].id || c.Role != "support" || c.Source == nil || !utf8.ValidString(*c.Source) ||
			c.Input.SHA256 != digestHex([]byte(*c.Source)) || c.Input.Bytes != uint64(len(*c.Source)) || c.Input.Role != "case" ||
			len(c.Covers) != 0 || len(c.Alternatives) != 0 || c.Sample != nil || c.DynamicSQL != nil || len(c.QueryExpect) != 0 ||
			len(c.Points) != 0 || len(c.Expect) != 0 || c.ExpectStatus != "" || c.ExpectAssessment != "" || c.ExpectCode != "" {
			return false
		}
	}
	return true
}

func qualAcceptedRole(r QualRole, platforms []QualPlatform) bool {
	if r.ID != "reuse-accepted" || r.Role != "owned" || r.Status != RoleExecuted || len(r.Workloads) != 4 || len(r.NotApplicable) != 0 {
		return false
	}
	seen := map[string]bool{}
	for _, w := range r.Workloads {
		if w.ReuseAccepted == nil || !qualAcceptedWorkload(w) {
			return false
		}
		key := w.Route + "/" + w.ReuseAccepted.Control
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	for _, p := range platforms {
		execute := p.pair() == "windows/amd64" || p.pair() == "linux/amd64"
		if slices.Contains(r.Platforms, p.ID) != execute || !execute && r.Deferred[p.ID] == "" {
			return false
		}
	}
	return true
}

func qualAcceptedMissing(w QualWorkload) []QualReuseAcceptedCase {
	start, _ := qualAcceptedPins(w.Route)
	cases := []QualReuseAcceptedCase{}
	for i, c := range w.Cases {
		x := QualReuseAcceptedCase{Set: w.Set, ID: c.ID, Control: w.ReuseAccepted.Control, Result: CellMissing, Code: "REUSE_ACCEPTED_EVIDENCE_MISSING", Steps: []ReuseAcceptedStep{}}
		if x.Control == "normal" {
			for _, k := range reuseAcceptedPins[start+i].targets {
				x.Steps = append(x.Steps, ReuseAcceptedStep{Step: k, Result: CellMissing, Code: x.Code})
			}
		}
		cases = append(cases, x)
	}
	return cases
}

func qualAcceptedAxis(cases []QualReuseAcceptedCase) *QualReuseAcceptedAxis {
	a := &QualReuseAcceptedAxis{Result: claimPass, Cases: cases}
	count := func(c *QualReuseAcceptedCounts, result string) {
		c.Expected++
		switch result {
		case claimPass:
			c.Passed++
		case claimFail:
			c.Failed++
		case CellMissing:
			c.Missing++
		default:
			c.Blocked++
		}
		if result == claimFail {
			a.Result = claimFail
		} else if result != claimPass {
			a.Result = worseClaim(a.Result, claimBlocked)
		}
	}
	for _, c := range cases {
		if c.Control == "omit-old-tree" {
			count(&a.Controls, c.Result)
			continue
		}
		count(&a.Originals, c.Result)
		for _, step := range c.Steps {
			count(&a.Targets, step.Result)
		}
	}
	return a
}

func qualAcceptedUnverified(cases []QualReuseAcceptedCase, result, code string) *QualReuseAcceptedAxis {
	for i := range cases {
		if cases[i].Result != claimFail && (result == claimFail || cases[i].Result == CellMissing) {
			cases[i].Result, cases[i].Code = result, code
		}
		for j := range cases[i].Steps {
			if cases[i].Steps[j].Result == CellMissing {
				cases[i].Steps[j].Result, cases[i].Steps[j].Code = result, code
			}
		}
	}
	return qualAcceptedAxis(cases)
}

// Restore only the new accepted role; interrupted legacy qualification keeps its meaning.
func (q *qualifier) restoreAccepted(res *QualificationResult) {
	if q.inv.Schema != QualificationInventorySchema {
		return
	}
	for _, role := range q.inv.ExtraRoles {
		if role.ID != "reuse-accepted" {
			continue
		}
		byPlat := map[string][]*qset{}
		for _, p := range q.inv.Platforms {
			for _, w := range role.Workloads {
				s := q.acceptedSets[p.ID+"/"+w.Set]
				if s == nil {
					s = newQSet(w, p)
				}
				byPlat[p.ID] = append(byPlat[p.ID], s)
			}
		}
		for _, row := range q.roleRows(role, q.inv.Platforms, byPlat) {
			found := false
			for i, old := range res.ExtraRoles {
				if old.ID == row.ID && old.Platform == row.Platform {
					res.ExtraRoles[i], found = row, true
					break
				}
			}
			if !found {
				res.ExtraRoles = append(res.ExtraRoles, row)
			}
			if row.Status == CellFail || row.ReuseAccepted != nil && row.ReuseAccepted.Result == claimFail {
				res.Assessment = AssessFail
			}
		}
	}
	res.BytesRead = q.bytesRead()
}
