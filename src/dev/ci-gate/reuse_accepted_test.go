package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func expectedAcceptedAxis(t *testing.T, workloads []kit.QualWorkload) *kit.QualReuseAcceptedAxis {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "native", "reuse-holdouts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kit.ParseReuseAcceptedLedger(b); err != nil {
		t.Fatal(err)
	}
	var ledger struct {
		Holdouts []struct {
			ID      string
			Targets []int `json:"blocked_steps"`
		}
	}
	if err := json.Unmarshal(b, &ledger); err != nil {
		t.Fatal(err)
	}
	a := &kit.QualReuseAcceptedAxis{Result: "PASS", Cases: []kit.QualReuseAcceptedCase{}}
	for _, w := range workloads {
		for _, c := range w.Cases {
			x := kit.QualReuseAcceptedCase{Set: w.Set, ID: c.ID, Control: w.ReuseAccepted.Control, Result: "PASS", Steps: []kit.ReuseAcceptedStep{}}
			if x.Control == "omit-old-tree" {
				a.Controls.Expected++
				a.Controls.Passed++
			} else {
				a.Originals.Expected++
				a.Originals.Passed++
				for _, h := range ledger.Holdouts {
					if h.ID == c.ID {
						for _, k := range h.Targets {
							x.Steps = append(x.Steps, kit.ReuseAcceptedStep{Step: k, Result: "PASS"})
							a.Targets.Expected++
							a.Targets.Passed++
						}
					}
				}
			}
			a.Cases = append(a.Cases, x)
		}
	}
	return a
}

func TestQualificationAcceptedAxis(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*kit.QualificationResult)
	}{
		{"valid", func(*kit.QualificationResult) {}},
		{"missing-axis", func(q *kit.QualificationResult) {
			q.ExtraRoles[roleRow(q, "reuse-accepted", "windows-amd64")].ReuseAccepted = nil
		}},
		{"shrunk-denominator", func(q *kit.QualificationResult) {
			a := q.ExtraRoles[roleRow(q, "reuse-accepted", "windows-amd64")].ReuseAccepted
			a.Originals.Expected--
			a.Originals.Passed--
		}},
		{"failed-target", func(q *kit.QualificationResult) {
			q.ExtraRoles[roleRow(q, "reuse-accepted", "windows-amd64")].Sets[0].ReuseAccepted.Cases[0].Steps[0].Result = "FAIL"
		}},
		{"missing-set", func(q *kit.QualificationResult) {
			r := &q.ExtraRoles[roleRow(q, "reuse-accepted", "windows-amd64")]
			r.Sets = r.Sets[:3]
		}},
		{"deferred-claim", func(q *kit.QualificationResult) {
			q.ExtraRoles[roleRow(q, "reuse-accepted", "darwin-arm64")].ReuseAccepted = &kit.QualReuseAcceptedAxis{Result: "PASS"}
		}},
		{"control-as-target", func(q *kit.QualificationResult) {
			q.ExtraRoles[roleRow(q, "reuse-accepted", "windows-amd64")].Sets[2].ReuseAccepted.Cases[0].Steps = []kit.ReuseAcceptedStep{{Step: 1, Result: "PASS"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, inv := qualificationFixture(t)
			tc.edit(&q)
			if err := checkQualification(q, expectation(), inv); (err == nil) != (tc.name == "valid") {
				t.Fatalf("gate %s: %v", tc.name, err)
			}
		})
	}
}
