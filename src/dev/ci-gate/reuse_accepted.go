package main

import (
	"errors"
	"reflect"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// Raw qualification is recomputed before this gate; require its complete fixed axis too.
func checkAcceptedAxes(q kit.QualificationResult, inventory kit.QualificationInventory) error {
	var role *kit.QualRole
	for i := range inventory.ExtraRoles {
		if inventory.ExtraRoles[i].ID == "reuse-accepted" {
			role = &inventory.ExtraRoles[i]
		}
	}
	if role == nil {
		return errors.New("accepted role registration missing")
	}
	for _, row := range q.ExtraRoles {
		if row.ID != role.ID {
			if row.ReuseAccepted != nil {
				return errors.New("accepted axis on unrelated role")
			}
			continue
		}
		if role.Deferred[row.Platform] != "" {
			if row.ReuseAccepted != nil {
				return errors.New("accepted deferred platform claimed")
			}
			continue
		}
		if len(row.Sets) != len(role.Workloads) || row.ReuseAccepted == nil {
			return errors.New("accepted sets or row axis missing")
		}
		all := []kit.QualReuseAcceptedCase{}
		for i, w := range role.Workloads {
			s := row.Sets[i]
			if w.ReuseAccepted == nil {
				return errors.New("accepted workload config missing")
			}
			if s.Set != w.Set || s.Platform != row.Platform || s.ReuseAccepted == nil || len(s.ReuseAccepted.Cases) != len(w.Cases) {
				return errors.New("accepted workload mismatch")
			}
			originals, targets, controls := 0, 0, 0
			for j, c := range s.ReuseAccepted.Cases {
				registered := w.Cases[j]
				if c.Set != w.Set || c.ID != registered.ID || c.Control != w.ReuseAccepted.Control || c.Result != "PASS" || c.Code != "" {
					return errors.New("accepted original or control incomplete")
				}
				if c.Control == "omit-old-tree" {
					controls++
					if len(c.Steps) != 0 {
						return errors.New("accepted control entered target counts")
					}
					continue
				}
				originals++
				previous := 0
				for _, step := range c.Steps {
					if step.Result != "PASS" || step.Code != "" || step.Step <= previous || step.Step > len(registered.Edits) {
						return errors.New("accepted target incomplete")
					}
					previous = step.Step
					targets++
				}
			}
			if !acceptedCounts(s.ReuseAccepted, originals, targets, controls) {
				return errors.New("accepted set counters differ")
			}
			all = append(all, s.ReuseAccepted.Cases...)
		}
		if !acceptedCounts(row.ReuseAccepted, 15, 25, 15) || !reflect.DeepEqual(row.ReuseAccepted.Cases, all) {
			return errors.New("accepted fixed denominator incomplete")
		}
	}
	return nil
}

func acceptedCounts(a *kit.QualReuseAcceptedAxis, originals, targets, controls int) bool {
	return a.Result == "PASS" && a.Originals == (kit.QualReuseAcceptedCounts{Expected: originals, Passed: originals}) &&
		a.Targets == (kit.QualReuseAcceptedCounts{Expected: targets, Passed: targets}) && a.Controls == (kit.QualReuseAcceptedCounts{Expected: controls, Passed: controls})
}
