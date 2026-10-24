package main

import (
	"errors"
	"reflect"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// expectedCSTAxis is owned by the integration verifier, derived only from its candidate's
// trusted inventory. Producer aggregates and its claimed support never choose this set.
func expectedCSTAxis(workloads []kit.QualWorkload) kit.QualPreprocessCSTAxis {
	a := kit.QualPreprocessCSTAxis{Result: kit.AssessBlocked, Cases: []kit.QualPreprocessCSTCase{}}
	for _, w := range workloads {
		for _, c := range w.Cases {
			x := kit.QualPreprocessCSTCase{Set: w.Set, Case: c.ID, Result: "NOT_APPLICABLE", Steps: []kit.PreprocessCSTStep{}}
			if len(c.PreprocessCSTExpect) == 0 {
				x.Code = c.ExpectCode
				a.Counts.NotApplicable++
			} else {
				x.Steps = c.PreprocessCSTExpect
				a.ComparedSteps += len(x.Steps)
				last := x.Steps[len(x.Steps)-1]
				if last.Result == "MISMATCH" {
					x.Result, x.Code = "MISMATCH", last.Code
					a.Counts.Mismatch++
				} else {
					x.Result = "PASS"
					a.Counts.Pass++
				}
			}
			a.Cases = append(a.Cases, x)
		}
	}
	if a.Counts.Pass > 0 && a.Counts.Mismatch == 0 {
		a.Result = kit.AssessPass
	}
	return a
}

// checkCSTAxes permits BLOCKED only for the exact outside-registered CST axis, while all
// historical row mechanisms/checks and 78-cell obligations remain mandatory PASS.
func checkCSTAxes(q kit.QualificationResult, inv kit.QualificationInventory) (map[string]bool, error) {
	blocked := map[string]bool{}
	bad := func() (map[string]bool, error) {
		return nil, errors.New("qualification primary CST axis differs from trusted inventory")
	}
	for _, r := range q.ExtraRoles {
		var registered *kit.QualRole
		for i := range inv.ExtraRoles {
			if inv.ExtraRoles[i].ID == r.ID {
				registered = &inv.ExtraRoles[i]
			}
		}
		if registered == nil {
			return bad()
		}
		var workloads []kit.QualWorkload
		for _, w := range registered.Workloads {
			if w.Preprocess != nil {
				workloads = append(workloads, w)
			}
		}
		executed := registered.Status == kit.RoleExecuted && slices.Contains(registered.Platforms, r.Platform)
		if len(workloads) == 0 || !executed {
			if r.PreprocessCST != nil {
				return bad()
			}
			for _, s := range r.Sets {
				if s.PreprocessCST != nil {
					return bad()
				}
			}
			continue
		}
		if len(r.Sets) != len(workloads) {
			return bad()
		}
		want := expectedCSTAxis(workloads)
		if r.PreprocessCST == nil || !reflect.DeepEqual(*r.PreprocessCST, want) {
			return bad()
		}
		for i, w := range workloads {
			s := r.Sets[i]
			setWant := expectedCSTAxis([]kit.QualWorkload{w})
			if s.Set != w.Set || s.Platform != r.Platform || s.PreprocessCST == nil || !reflect.DeepEqual(*s.PreprocessCST, setWant) {
				return bad()
			}
		}
		blocked[r.Platform] = blocked[r.Platform] || want.Result == kit.AssessBlocked
	}
	return blocked, nil
}
