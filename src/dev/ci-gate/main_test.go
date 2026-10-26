package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func expectation() kit.RunIdentity {
	return kit.RunIdentity{Schema: kit.RunIdentitySchema, Repository: repository, Workflow: "Foundation", RunID: "123", RunAttempt: "2", Event: "pull_request", SHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), Checkout: strings.Repeat("a", 40), EvidenceMode: "NEW_RUN", Job: "integration-gate"}
}
func encoded(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestStrictJSON(t *testing.T) {
	for _, b := range []string{`{"a":1,"a":2}`, `{"a":1,"\u0061":2}`, `{"a":1} {}`, `{"a":`, strings.Repeat("[", 33) + "0" + strings.Repeat("]", 33), "\"\xff\""} {
		var v any
		if decode([]byte(b), &v) == nil {
			t.Fatalf("invalid JSON accepted: %q", b)
		}
	}
	var v map[string]string
	if decode([]byte(`{"result":true}`), &v) == nil {
		t.Fatal("wrong type accepted")
	}
}

func TestNeedsFailClosed(t *testing.T) {
	good := map[string]any{}
	for _, name := range []string{"foundation", "native-prepare", "native-routes", "native-routes-macos", "large-api", "qualification"} {
		good[name] = map[string]any{"result": "success", "outputs": map[string]string{}}
	}
	if e := checkNeeds(encoded(t, good)); e != nil {
		t.Fatal(e)
	}
	for producer := range good {
		for _, status := range []any{"failure", "skipped", "cancelled", "", nil, true, "unknown"} {
			bad := map[string]any{}
			for k, v := range good {
				bad[k] = v
			}
			bad[producer] = map[string]any{"result": status}
			if checkNeeds(encoded(t, bad)) == nil {
				t.Fatalf("%s status %v accepted", producer, status)
			}
		}
		bad := map[string]any{}
		for k, v := range good {
			if k != producer {
				bad[k] = v
			}
		}
		if checkNeeds(encoded(t, bad)) == nil {
			t.Fatalf("missing %s accepted", producer)
		}
	}
	good["unexpected"] = map[string]any{"result": "success"}
	if checkNeeds(encoded(t, good)) == nil || checkNeeds([]byte(`{}`)) == nil {
		t.Fatal("unexpected or empty producer set accepted")
	}
}

func qualificationFixture(t *testing.T) (kit.QualificationResult, kit.QualificationInventory) {
	t.Helper()
	data, e := os.ReadFile(filepath.Join("..", "..", "contracts", "qualification-c1.json"))
	if e != nil {
		t.Fatal(e)
	}
	inv, e := kit.ParseQualificationInventory(data)
	if e != nil {
		t.Fatal(e)
	}
	r := expectation()
	q := kit.QualificationResult{ResultSchema: kit.QualificationResultSchema, Run: &r, Candidate: r.Checkout, Completeness: "PASS", MechanismGate: "PASS", SupportClaim: "SUPPORTED", PlatformClaims: map[string]string{}}
	q.Assessment = kit.AssessPass
	q.Totals.Cells = 78
	q.Totals.Status = map[string]int{"PASS": 78}
	for _, route := range inv.Routes {
		for _, p := range inv.Platforms {
			cell := kit.QualCell{Route: route.Route, Platform: p.ID, Status: "PASS", Mechanism: "PASS", Requirement: "PASS", Comparison: "PASS", Checks: "PASS"}
			for _, req := range route.Requirements {
				for _, kind := range req.Kinds {
					cell.Obligations = append(cell.Obligations, kit.QualObligation{Row: req.Row, Kind: kind, Result: "PASS", Cases: []string{"observed"}})
				}
			}
			cell.Counts.Obligations = len(cell.Obligations)
			cell.Counts.Pass = len(cell.Obligations)
			q.Cells = append(q.Cells, cell)
			q.Totals.Obligations.Obligations += len(cell.Obligations)
			q.PlatformClaims[p.ID] = "SUPPORTED"
		}
	}
	q.Totals.Obligations.Pass = q.Totals.Obligations.Obligations
	for _, x := range inv.ExtraRoles {
		for _, p := range inv.Platforms {
			row := kit.QualRoleRow{ID: x.ID, Role: x.Role, Platform: p.ID, Status: x.Status}
			switch {
			case x.Status != kit.RoleExecuted:
			case slices.Contains(x.Platforms, p.ID):
				row.Status, row.Mechanism, row.Checks, row.Sets = "PASS", "PASS", "PASS", []kit.QualSet{{}}
				if x.ID == "csharp-symbols" {
					axis := expectedCSTAxis(x.Workloads)
					row.PreprocessCST = &axis
					row.Sets = nil
					for _, w := range x.Workloads {
						setAxis := expectedCSTAxis([]kit.QualWorkload{w})
						row.Sets = append(row.Sets, kit.QualSet{Set: w.Set, Platform: p.ID, PreprocessCST: &setAxis})
					}
					if axis.Result == kit.AssessBlocked {
						q.Assessment, q.SupportClaim = kit.AssessBlocked, "BLOCKED"
						q.PlatformClaims[p.ID] = "BLOCKED"
					}
				}
			case x.NotApplicable[p.ID] != "":
				row.Status = kit.RoleNotApplicable
			case x.Deferred[p.ID] != "":
				row.Status = kit.RoleDeferred
			}
			q.ExtraRoles = append(q.ExtraRoles, row)
		}
	}
	return q, inv
}

// roleRow is the index of one extra-role row of the fixture.
func roleRow(q *kit.QualificationResult, id, platform string) int {
	return slices.IndexFunc(q.ExtraRoles, func(r kit.QualRoleRow) bool { return r.ID == id && r.Platform == platform })
}

func TestQualificationRejectsIncompleteAndForeignEvidence(t *testing.T) {
	q, inv := qualificationFixture(t)
	if e := checkQualification(q, expectation(), inv); e != nil {
		t.Fatal(e)
	}
	mutations := map[string]func(*kit.QualificationResult){
		"sha":                  func(q *kit.QualificationResult) { q.Candidate = strings.Repeat("c", 40) },
		"run":                  func(q *kit.QualificationResult) { q.Run.RunID = "124" },
		"attempt":              func(q *kit.QualificationResult) { q.Run.RunAttempt = "1" },
		"missing_cell":         func(q *kit.QualificationResult) { q.Cells = q.Cells[:77] },
		"duplicate_cell":       func(q *kit.QualificationResult) { q.Cells[0] = q.Cells[1] },
		"foreign_cell":         func(q *kit.QualificationResult) { q.Cells[0].Route = "unregistered" },
		"internal_gate":        func(q *kit.QualificationResult) { q.MechanismGate = "FAIL" },
		"obligation":           func(q *kit.QualificationResult) { q.Cells[0].Obligations = q.Cells[0].Obligations[1:] },
		"duplicate_obligation": func(q *kit.QualificationResult) { q.Cells[0].Obligations[0] = q.Cells[0].Obligations[1] },
		"empty_case":           func(q *kit.QualificationResult) { q.Cells[0].Obligations[0].Cases = nil },
		"zero_obligations":     func(q *kit.QualificationResult) { q.Totals.Obligations.Obligations = 0; q.Totals.Obligations.Pass = 0 },
		"finding":              func(q *kit.QualificationResult) { q.Findings = append(q.Findings, kit.Finding{Code: "CONTROL"}) },
		// #218: the symbol role rows, executed on Windows and Linux and deferred on darwin
		"role_row_missing": func(q *kit.QualificationResult) { q.ExtraRoles = slices.Delete(q.ExtraRoles, 0, 1) },
		"role_row_failed": func(q *kit.QualificationResult) {
			q.ExtraRoles[roleRow(q, "csharp-symbols", "linux-amd64")].Status = "FAIL"
		},
		"role_row_axis": func(q *kit.QualificationResult) {
			q.ExtraRoles[roleRow(q, "csharp-symbols", "windows-amd64")].Checks = "BLOCKED"
		},
		"deferred_executed": func(q *kit.QualificationResult) {
			q.ExtraRoles[roleRow(q, "csharp-symbols", "darwin-arm64")].Status = "PASS"
		},
		"deferred_axis": func(q *kit.QualificationResult) {
			q.ExtraRoles[roleRow(q, "csharp-symbols", "darwin-arm64")].Mechanism = "PASS"
		},
		"role_row_twice": func(q *kit.QualificationResult) { q.ExtraRoles = append(q.ExtraRoles, q.ExtraRoles[0]) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fresh, _ := qualificationFixture(t)
			mutate(&fresh)
			if checkQualification(fresh, expectation(), inv) == nil {
				t.Fatal("bad qualification accepted")
			}
		})
	}
}

func TestExpectedIdentityAndOutputBoundaries(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "expected.json")
	write := func(v any) {
		t.Helper()
		if e := os.WriteFile(p, encoded(t, v), 0600); e != nil {
			t.Fatal(e)
		}
	}
	good := expectation()
	write(good)
	if _, e := expectedIdentity(p); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*kit.RunIdentity){func(r *kit.RunIdentity) { r.Repository = "other/repo" }, func(r *kit.RunIdentity) { r.Event = "pull_request_target" }, func(r *kit.RunIdentity) { r.SHA = "bad" }, func(r *kit.RunIdentity) { r.RunAttempt = "0" }, func(r *kit.RunIdentity) { r.EvidenceMode = "CACHED" }} {
		bad := good
		mutate(&bad)
		write(bad)
		if _, e := expectedIdentity(p); e == nil {
			t.Fatal("bad expectation accepted")
		}
	}
	if outside(filepath.Join(dir, "artifacts", "expected.json"), filepath.Join(dir, "artifacts")) || !outside(p, filepath.Join(dir, "artifacts")) {
		t.Fatal("output/evidence boundary incorrect")
	}
}
