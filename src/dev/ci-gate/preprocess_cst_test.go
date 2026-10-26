package main

import (
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"slices"
	"testing"
)

// Keep the original four-detector counterexamples after the current producer is corrected.
func historicalCSTFixture(t *testing.T) (kit.QualificationResult, kit.QualificationInventory) {
	q, inv := qualificationFixture(t)
	anchors := map[string]uint32{"directive-string-no-escape": 25, "razor-comment-lines": 9, "dollars-two-quotes-raw": 37, "nested-string-open-no-runaway": 62}
	for i := range inv.ExtraRoles {
		r := &inv.ExtraRoles[i]
		if r.ID != "csharp-symbols" {
			continue
		}
		for j := range r.Workloads {
			for k := range r.Workloads[j].Cases {
				c := &r.Workloads[j].Cases[k]
				at, found := anchors[c.ID]
				if !found {
					continue
				}
				st := &c.PreprocessCSTExpect[0]
				st.Result, st.Code = "MISMATCH", kit.PreprocessCSTPrefix+"0"
				st.CSTHashes = append(slices.Clone(st.HostHashes), at)
				slices.Sort(st.CSTHashes)
				st.CSTOnly = []uint32{at}
				c.ExpectStatus, c.ExpectAssessment, c.ExpectCode = kit.StatusNotRun, kit.AssessBlocked, st.Code
				c.Expect = nil
			}
		}
		for j := range q.ExtraRoles {
			row := &q.ExtraRoles[j]
			if row.ID != r.ID || !slices.Contains(r.Platforms, row.Platform) {
				continue
			}
			axis := expectedCSTAxis(r.Workloads)
			row.PreprocessCST = &axis
			for k, w := range r.Workloads {
				setAxis := expectedCSTAxis([]kit.QualWorkload{w})
				row.Sets[k].PreprocessCST = &setAxis
			}
			q.PlatformClaims[row.Platform] = "BLOCKED"
		}
	}
	q.Assessment, q.SupportClaim = kit.AssessBlocked, "BLOCKED"
	return q, inv
}

func TestQualificationCSTCorrectedIsExact(t *testing.T) {
	q, inv := qualificationFixture(t)
	a := q.ExtraRoles[roleRow(&q, "csharp-symbols", "windows-amd64")].PreprocessCST
	if a == nil || a.Result != kit.AssessPass || a.Counts.Pass != 26 || a.Counts.Mismatch != 0 || a.Counts.NotApplicable != 4 || a.ComparedSteps != 29 || len(a.Cases) != 30 {
		t.Fatalf("corrected 30-case denominator/29-step prefix: %+v", a)
	}
	if err := checkQualification(q, expectation(), inv); err != nil {
		t.Fatal(err)
	}
	a.Result = kit.AssessBlocked
	if checkQualification(q, expectation(), inv) == nil {
		t.Fatal("unregistered CST aggregate accepted")
	}
}

func TestQualificationCSTBlockedIsExact(t *testing.T) {
	q, inv := historicalCSTFixture(t)
	index := roleRow(&q, "csharp-symbols", "windows-amd64")
	a := q.ExtraRoles[index].PreprocessCST
	if a == nil || a.Counts.Pass != 22 || a.Counts.Mismatch != 4 || a.Counts.NotApplicable != 4 || a.Counts.Fail != 0 || a.Counts.Unverified != 0 || a.ComparedSteps != 29 || len(a.Cases) != 30 {
		t.Fatalf("trusted 30-case denominator/29-step prefix changed: %+v", a)
	}
	if q.Assessment != kit.AssessBlocked || q.SupportClaim != "BLOCKED" || q.PlatformClaims["darwin-arm64"] != "SUPPORTED" {
		t.Fatal("CST mismatch promoted support or changed deferred scope")
	}
	if err := checkQualification(q, expectation(), inv); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*kit.QualRoleRow, *kit.QualificationResult){
		"support-promotion": func(_ *kit.QualRoleRow, q *kit.QualificationResult) {
			q.Assessment = kit.AssessPass
			q.SupportClaim = "SUPPORTED"
			q.PlatformClaims["windows-amd64"] = "SUPPORTED"
			q.PlatformClaims["linux-amd64"] = "SUPPORTED"
		},
		"axis-omission":     func(r *kit.QualRoleRow, _ *kit.QualificationResult) { r.PreprocessCST = nil },
		"set-axis-omission": func(r *kit.QualRoleRow, _ *kit.QualificationResult) { r.Sets[0].PreprocessCST = nil },
		"denominator-shrink": func(r *kit.QualRoleRow, _ *kit.QualificationResult) {
			r.PreprocessCST.Cases = r.PreprocessCST.Cases[:29]
			r.PreprocessCST.Counts.Mismatch--
		},
		"case-duplicate": func(r *kit.QualRoleRow, _ *kit.QualificationResult) {
			r.PreprocessCST.Cases[0] = r.PreprocessCST.Cases[1]
		},
		"positive-blocked": func(r *kit.QualRoleRow, _ *kit.QualificationResult) {
			r.PreprocessCST.Cases[0].Result = "UNVERIFIED"
			r.PreprocessCST.Counts.Pass--
			r.PreprocessCST.Counts.Unverified++
		},
		"step-prefix-shrink": func(r *kit.QualRoleRow, _ *kit.QualificationResult) {
			r.PreprocessCST.Cases[3].Steps = r.PreprocessCST.Cases[3].Steps[:1]
			r.PreprocessCST.ComparedSteps--
		},
		"mismatch-promotion": func(r *kit.QualRoleRow, _ *kit.QualificationResult) {
			r.PreprocessCST.Cases[29].Result = "PASS"
			r.PreprocessCST.Cases[29].Code = ""
			r.PreprocessCST.Counts.Mismatch--
			r.PreprocessCST.Counts.Pass++
		},
		"false-aggregate": func(r *kit.QualRoleRow, _ *kit.QualificationResult) { r.PreprocessCST.Result = "PASS" },
		"foreign-set":     func(r *kit.QualRoleRow, _ *kit.QualificationResult) { r.Sets[0].Set = "foreign" },
		"extra-set":       func(r *kit.QualRoleRow, _ *kit.QualificationResult) { r.Sets = append(r.Sets, r.Sets[0]) },
		"deferred-axis": func(_ *kit.QualRoleRow, q *kit.QualificationResult) {
			i := roleRow(q, "csharp-symbols", "darwin-arm64")
			q.ExtraRoles[i].PreprocessCST = &kit.QualPreprocessCSTAxis{Result: "PASS"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			fresh, _ := historicalCSTFixture(t)
			i := roleRow(&fresh, "csharp-symbols", "windows-amd64")
			mutate(&fresh.ExtraRoles[i], &fresh)
			if checkQualification(fresh, expectation(), inv) == nil {
				t.Fatal("forged CST support accepted")
			}
		})
	}
}
