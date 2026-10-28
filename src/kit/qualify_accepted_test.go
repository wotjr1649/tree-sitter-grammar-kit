package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// The hook cancels a real context at a deterministic checkpoint, without timing sleeps.
type acceptedCheckWall struct {
	context.Context
	calls  atomic.Int64
	at     int64
	expire context.CancelFunc
}

func (w *acceptedCheckWall) Err() error {
	if w.calls.Add(1) == w.at {
		w.expire()
	}
	return w.Context.Err()
}

func acceptedWall(t *testing.T, at int64) *acceptedCheckWall {
	t.Helper()
	ctx, expire := context.WithCancel(context.Background())
	w := &acceptedCheckWall{Context: ctx, at: at, expire: expire}
	t.Cleanup(expire)
	testHookWall = func() (context.Context, context.CancelFunc) { return w, func() {} }
	t.Cleanup(func() { testHookWall = nil })
	return w
}

func TestQualifyReuseAcceptedInterruptedReader(t *testing.T) {
	for _, prior := range []string{"normal", "gate-fail", "legacy-fail", "omit-control"} {
		for _, offset := range []int64{0, 1} {
			t.Run(fmt.Sprintf("%s/last-minus-%d", prior, offset), func(t *testing.T) {
				root, _ := acceptedReplayFixture(t, func(files map[string][]byte, top map[string]any) {
					switch prior {
					case "gate-fail":
						c := top["cases"].([]any)[0].(map[string]any)
						proof := c["reuse_accepted"].(ReuseAcceptedResult)
						proof.HistorySHA256 = strings.Repeat("0", 64)
						c["reuse_accepted"] = proof
					case "legacy-fail":
						acceptedLegacyRouteFailure(t, files, top)
					case "omit-control":
						acceptedOmitRouteControl(t, files, top)
					}
				})
				baselineWall := acceptedWall(t, 0)
				baseline, w, p := acceptedReaderQualifier(t, root, false)
				if prior == "omit-control" {
					w.ReuseAccepted.Control = "omit-old-tree"
				}
				if _, e := baseline.evalSymbolSet(w, p); e != nil {
					t.Fatal(e)
				}
				at := baselineWall.calls.Load() - offset
				acceptedWall(t, at)
				q, w, p := acceptedReaderQualifier(t, root, false)
				if prior == "omit-control" {
					w.ReuseAccepted.Control = "omit-old-tree"
				}
				_, e := q.evalSymbolSet(w, p)
				if e == nil || e.Code != "WALL_LIMIT" {
					t.Fatalf("checkpoint %d did not stop: %v", at, e)
				}
				s := q.acceptedSets[p.ID+"/"+w.Set]
				a := s.ReuseAccepted
				switch prior {
				case "gate-fail":
					if s.Mechanism != AssessFail || a.Originals.Failed != 1 {
						t.Fatalf("lost evidence failure: %+v %+v", s, a)
					}
				case "legacy-fail":
					if a.Originals.Failed != 1 || a.Targets.Passed != 1 || a.Cases[0].Code != "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_1" {
						t.Fatalf("lost legacy FAIL/target PASS: %+v", a)
					}
				case "omit-control":
					if s.Mechanism != AssessBlocked || a.Controls.Passed != 1 || a.Controls.Failed != 0 || a.Originals.Expected != 0 {
						t.Fatalf("expected omit FAIL poisoned qualification: %+v %+v", s, a)
					}
				default:
					if s.Mechanism != AssessBlocked || a.Originals.Failed != 0 || a.Targets.Failed != 0 {
						t.Fatalf("honest limit called FAIL: %+v %+v", s, a)
					}
				}
			})
		}
	}
}

func TestQualifyReuseAcceptedEarlierSetBeforeLimit(t *testing.T) {
	for _, knownFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(knownFailure), func(t *testing.T) {
			f := newQfx(t)
			hosts := f.all(t, nil)
			role := acceptedQualRole(t)
			f.inv.ExtraRoles = append(f.inv.ExtraRoles, role)
			for i, w := range role.Workloads[:2] {
				if i == 0 && !knownFailure {
					continue
				}
				dir := filepath.Join(hosts["windows-amd64"], "incremental", w.Set)
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			wall, expire := context.WithCancel(context.Background())
			defer expire()
			testHookWall = func() (context.Context, context.CancelFunc) { return wall, func() {} }
			testHookOpen = func(path string) {
				if path == "incremental/"+role.Workloads[1].Set+"/profile.json" {
					expire()
				}
			}
			defer func() { testHookWall = nil; testHookOpen = nil }()
			var hs []QualifyHost
			for _, p := range qfxPlatforms {
				hs = append(hs, QualifyHost{Platform: p.ID, Root: hosts[p.ID]})
			}
			r, err := Qualify(ctxT(t), QualifyRequest{Inventory: f.invBytes(t), Candidate: qfxCandidate, Hosts: hs})
			assessment := AssessBlocked
			if knownFailure {
				assessment = AssessFail
			}
			if err == nil || asError(err).Code != "WALL_LIMIT" || r.Assessment != assessment || r.ExecutionStatus != StatusResourceLimit || r.SupportClaim != "BLOCKED" || len(r.PlatformClaims) != 0 {
				t.Fatalf("error assembly: %+v %v", r, err)
			}
			for _, p := range qfxPlatforms {
				row := roleRow(r, role.ID, p.ID)
				if p.GOOS == "darwin" {
					if row.Status != RoleDeferred || row.ReuseAccepted != nil {
						t.Fatalf("mac axis: %+v", row)
					}
					continue
				}
				a := row.ReuseAccepted
				if a == nil || a.Originals.Expected != 15 || a.Targets.Expected != 25 || a.Controls.Expected != 15 || a.Controls.Missing != 15 {
					t.Fatalf("lost fixed denominator: %+v", row)
				}
				if p.ID == "windows-amd64" && (a.Originals.Blocked != 3 || knownFailure && (a.Originals.Failed != 12 || a.Targets.Failed != 22)) {
					t.Fatalf("lost prior FAIL or current BLOCKED: %+v", a)
				}
			}
		})
	}
}

func TestQualifyReuseAcceptedRestoreIsIdempotent(t *testing.T) {
	f := newQfx(t)
	role := acceptedQualRole(t)
	f.inv.ExtraRoles = append(f.inv.ExtraRoles, role)
	w, p := role.Workloads[0], qfxPlatforms[0]
	s := newQSet(w, p)
	s.Evidence, s.Mechanism = ModeNewRun, AssessPass
	cases := s.ReuseAccepted.Cases
	cases[0].Result, cases[0].Code = claimFail, "KNOWN_ACCEPTED_FAILURE"
	cases[0].Steps[0].Result, cases[0].Steps[0].Code = claimPass, ""
	s.ReuseAccepted = qualAcceptedAxis(cases)
	s.checks[cases[0].ID] = claimFail
	q := &qualifier{r: &run{}, inv: f.inv, hosts: map[string]*qhost{}, acceptedSets: map[string]*qset{p.ID + "/" + w.Set: s}}
	r := QualificationResult{Report: newReport("qualify"), ExtraRoles: []QualRoleRow{}, SupportClaim: "BLOCKED", PlatformClaims: map[string]string{}}
	failReport(&r.Report, fail(KindResourceLimit, "OUTPUT_LIMIT", "", nil))
	q.restoreAccepted(&r)
	q.restoreAccepted(&r)
	count := 0
	for _, row := range r.ExtraRoles {
		if row.ID == role.ID {
			count++
		}
	}
	a := roleRow(r, role.ID, p.ID).ReuseAccepted
	if count != 3 || r.Assessment != AssessFail || r.ExecutionStatus != StatusResourceLimit || r.SupportClaim != "BLOCKED" || len(r.PlatformClaims) != 0 || a.Originals.Expected != 15 || a.Originals.Failed != 1 || a.Targets.Expected != 25 || a.Targets.Passed != 1 || a.Controls.Expected != 15 {
		t.Fatalf("restoration duplicated rows or lost progress: %+v", r)
	}
}

func TestQualifyReuseAcceptedIntrinsicResource(t *testing.T) {
	for _, member := range []string{"raw", "projection"} {
		for _, mismatch := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/source-mismatch=%t", member, mismatch), func(t *testing.T) {
				root, _ := acceptedReplayFixture(t, func(files map[string][]byte, top map[string]any) {
					deep := json.RawMessage(strings.Repeat("[", jsonMaxDepth+1) + "0" + strings.Repeat("]", jsonMaxDepth+1))
					c := top["cases"].([]any)[0].(map[string]any)
					if member == "projection" {
						c["retained_extra"] = deep
					}
					for path, data := range files {
						if member == "raw" && strings.HasPrefix(path, "responses/") {
							files[path] = deep
							c["process"].(map[string]any)["stdout_bytes"] = len(deep) + 4
						}
						if mismatch && strings.HasPrefix(path, "sources/") {
							files[path] = []byte(strings.Repeat("x", len(data)))
						}
					}
				})
				q, w, p := acceptedReaderQualifier(t, root, false)
				_, err := q.evalSymbolSet(w, p)
				if err == nil || err.Kind != KindResourceLimit || err.Code != "JSON_DEPTH_LIMIT" {
					t.Fatalf("intrinsic limit lost: %v", err)
				}
				s := q.acceptedSets[p.ID+"/"+w.Set]
				a := s.ReuseAccepted
				if s.Mechanism != AssessBlocked || a.Originals.Expected != 1 || a.Targets.Expected != 1 || a.Targets.Blocked != 1 {
					t.Fatalf("limit became evidence FAIL or shrank targets: %+v %+v", s, a)
				}
				if mismatch {
					if a.Originals.Failed != 1 || a.Cases[0].Code != "REUSE_ACCEPTED_SOURCE_MISMATCH" {
						t.Fatalf("known source FAIL lost: %+v", a)
					}
				} else if a.Originals.Failed != 0 || a.Originals.Blocked != 1 {
					t.Fatalf("honest limit called FAIL: %+v", a)
				}
			})
		}
	}
}

func TestQualifyReuseAcceptedHistoricalErrorUnchanged(t *testing.T) {
	for _, schema := range []string{qualificationInventoryR5, qualificationInventoryR6, qualificationInventoryR7} {
		t.Run(schema, func(t *testing.T) {
			f := newQfx(t)
			f.inv.Schema = schema
			f.inv.ExtraRoles[0].ID = "reuse-accepted"
			inv, err := ParseQualificationInventory(f.invBytes(t))
			if err != nil {
				t.Fatal(err)
			}
			q := &qualifier{r: &run{}, inv: inv, hosts: map[string]*qhost{}}
			r := QualificationResult{Report: newReport("qualify"), ExtraRoles: []QualRoleRow{}}
			failReport(&r.Report, fail(KindResourceLimit, "WALL_LIMIT", "", nil))
			q.restoreAccepted(&r)
			if len(r.ExtraRoles) != 0 || r.Assessment != AssessBlocked {
				t.Fatalf("historical role ID activated new recovery: %+v", r.ExtraRoles)
			}
		})
	}
}

func acceptedQualRole(t *testing.T) QualRole {
	t.Helper()
	_, ledger := acceptedLedgerFixture(t)
	r := QualRole{ID: "reuse-accepted", Role: "owned", Status: RoleExecuted, Reason: "fixed accepted controls",
		Platforms: []string{"windows-amd64", "linux-amd64"}, NotApplicable: map[string]string{}, Deferred: map[string]string{"darwin-arm64": "excluded"}}
	for _, control := range []string{"normal", "omit-old-tree"} {
		for _, route := range []string{"tsql", "json"} {
			w := QualWorkload{Set: "accepted-" + route + "-" + control, Profile: "accepted-" + route + "-" + control, Route: route,
				Operation: "native-parse-edit", Output: OutputTree, Symbol: "tree_sitter_" + route,
				Grammar: []NativeInput{{Path: "src/parser.c", Role: "parser", Bytes: 1, SHA256: fxCompiler}}, RuntimeCommit: fxRuntime,
				ReuseAccepted: &QualReuseAcceptedRegistration{Policy: ReuseAcceptedSchema, LedgerSHA256: ReuseAcceptedLedgerSHA256, Control: control}}
			for _, h := range ledger.originals {
				if h.Route == route {
					source := h.SourceUTF8
					c := acceptedOriginalCase(h)
					c.Input.Role = "case"
					w.Cases = append(w.Cases, QualCase{ID: c.ID, Role: "support", Input: c.Input, Edits: c.Edits, Source: &source})
				}
			}
			r.Workloads = append(r.Workloads, w)
		}
	}
	return r
}

func TestQualifyReuseAcceptedRegistration(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*QualRole)
	}{
		{"valid", func(*QualRole) {}},
		{"missing-original", func(r *QualRole) { r.Workloads[0].Cases = r.Workloads[0].Cases[1:] }},
		{"reordered-original", func(r *QualRole) { slices.Reverse(r.Workloads[0].Cases) }},
		{"missing-control", func(r *QualRole) { r.Workloads = r.Workloads[:3] }},
		{"duplicate-control", func(r *QualRole) { r.Workloads[2] = r.Workloads[0] }},
		{"changed-ledger", func(r *QualRole) { r.Workloads[0].ReuseAccepted.LedgerSHA256 = strings.Repeat("0", 64) }},
		{"mac-claimed", func(r *QualRole) {
			r.Platforms = append(r.Platforms, "darwin-arm64")
			delete(r.Deferred, "darwin-arm64")
		}},
		{"source-mismatch", func(r *QualRole) { source := "wrong"; r.Workloads[0].Cases[0].Source = &source }},
		{"api", func(r *QualRole) { r.Workloads[0].API = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newQfx(t)
			r := acceptedQualRole(t)
			tc.edit(&r)
			f.inv.ExtraRoles = append(f.inv.ExtraRoles, r)
			_, err := ParseQualificationInventory(f.invBytes(t))
			if (err == nil) != (tc.name == "valid") {
				t.Fatalf("registration %s: %v", tc.name, err)
			}
		})
	}
}

func TestQualifyReuseAcceptedMissingAndHistory(t *testing.T) {
	f := newQfx(t)
	f.inv.ExtraRoles = append(f.inv.ExtraRoles, acceptedQualRole(t))
	r, err := Qualify(ctxT(t), QualifyRequest{Inventory: f.invBytes(t), Candidate: strings.Repeat("c", 40)})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range qfxPlatforms {
		row := roleRow(r, "reuse-accepted", p.ID)
		if p.GOOS == "darwin" {
			if row.Status != RoleDeferred || row.ReuseAccepted != nil {
				t.Fatalf("mac row %+v", row)
			}
			continue
		}
		a := row.ReuseAccepted
		if row.Status != CellMissing || a == nil || a.Originals.Expected != 15 || a.Originals.Missing != 15 || a.Targets.Expected != 25 || a.Targets.Missing != 25 || a.Controls.Expected != 15 || a.Controls.Missing != 15 {
			t.Fatalf("missing denominator %+v %+v", row, a)
		}
	}
	for schema, want := range map[string]string{qualificationInventoryR5: qualificationResultR6, qualificationInventoryR6: qualificationResultR7, qualificationInventoryR7: qualificationResultR8, QualificationInventorySchema: QualificationResultSchema} {
		f := newQfx(t)
		f.inv.Schema = schema
		r, err := Qualify(ctxT(t), QualifyRequest{Inventory: f.invBytes(t), Candidate: strings.Repeat("c", 40)})
		if err != nil || r.ResultSchema != want {
			t.Fatalf("historical mapping %s: %s %v", schema, r.ResultSchema, err)
		}
		if schema != QualificationInventorySchema {
			for _, value := range []any{nil, map[string]any{}, ""} {
				var doc map[string]any
				_ = json.Unmarshal(f.invBytes(t), &doc)
				doc["routes"].([]any)[0].(map[string]any)["workload"].(map[string]any)["reuse_accepted"] = value
				b, _ := json.Marshal(doc)
				if _, err := ParseQualificationInventory(b); err == nil {
					t.Fatalf("old schema admitted new member: %s %#v", schema, value)
				}
			}
		}
	}
}

func TestQualifyReuseAcceptedCountsPreserveFailure(t *testing.T) {
	a := qualAcceptedAxis([]QualReuseAcceptedCase{
		{Control: "normal", Result: claimFail, Steps: []ReuseAcceptedStep{{Result: claimPass}, {Result: claimBlocked}}},
		{Control: "normal", Result: CellMissing, Steps: []ReuseAcceptedStep{{Result: CellMissing}}},
		{Control: "omit-old-tree", Result: claimPass},
	})
	if a.Result != claimFail || a.Originals.Expected != 2 || a.Originals.Failed != 1 || a.Originals.Missing != 1 || a.Targets.Expected != 3 || a.Targets.Passed != 1 || a.Targets.Blocked != 1 || a.Targets.Missing != 1 || a.Controls.Passed != 1 {
		t.Fatalf("lost failure or denominator: %+v", a)
	}
}

func TestQualifyRoleFailureSurvivesMissingSet(t *testing.T) {
	f := newQfx(t)
	w := f.inv.ExtraRoles[0].Workloads[0]
	w.Set, w.Profile = "missing-history", "missing-history"
	f.inv.ExtraRoles[0].Workloads = append(f.inv.ExtraRoles[0].Workloads, w)
	hosts := f.all(t, map[string]qfxMut{"windows-amd64": {
		skip: func(set string) bool { return set == "missing-history" },
		manifest: func(set string, man map[string]any) {
			if set == "s06-fxh" {
				man["policy"] = IdentityRef{Role: "policy", Schema: NativePolicySchema, SHA256: strings.Repeat("0", 64)}
			}
		},
	}})
	r := f.run(t, hosts)
	row := roleRow(r, "fx-history", "windows-amd64")
	if row.Status != CellFail || row.Mechanism != AssessFail || len(row.Sets) != 2 || row.Sets[1].Evidence != "MISSING" {
		t.Fatalf("known failure hidden by missing set: %+v", row)
	}
}

// One original isolates the guarded reader; public registration separately fixes all 15.
func TestQualifyReuseAcceptedReader(t *testing.T) {
	for _, name := range []string{"complete", "legacy-route-fail", "profile-missing", "stored-proof-mismatch", "file-limit", "host-rejected"} {
		t.Run(name, func(t *testing.T) {
			root, _ := acceptedReplayFixture(t, func(files map[string][]byte, top map[string]any) {
				if name == "stored-proof-mismatch" {
					top["reuse_accepted_ledger_sha256"] = strings.Repeat("0", 64)
				}
				if name == "legacy-route-fail" {
					acceptedLegacyRouteFailure(t, files, top)
				}
			})
			q, w, p := acceptedReaderQualifier(t, root, name == "profile-missing")
			h := q.hosts[p.ID]
			if name == "file-limit" {
				h.x.lim.FileBytes = uint64(h.files["incremental/"+w.Set+"/profile.json"].Size() - 1)
			}
			if name == "host-rejected" {
				h.reject("FIXTURE_REJECTED", "", "test")
			}
			s, e := q.evalSymbolSet(w, p)
			if e != nil {
				t.Fatal(e)
			}
			a := s.ReuseAccepted
			switch name {
			case "complete":
				if s.Mechanism != AssessPass || a.Result != claimPass || a.Originals.Passed != 1 || a.Targets.Passed != 1 {
					t.Fatalf("positive reader %+v %+v", s, a)
				}
			case "file-limit":
				if s.Mechanism != AssessBlocked || a.Result != claimBlocked || a.Originals.Blocked != 1 || a.Originals.Missing != 0 {
					t.Fatalf("limit called missing or pass: %+v %+v", s, a)
				}
			case "legacy-route-fail":
				if a.Result != claimFail || a.Originals.Failed != 1 || a.Targets.Passed != 1 || a.Cases[0].Code != "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_1" || s.checks[w.Cases[0].ID] != claimFail {
					t.Fatalf("accepted proof masked legacy failure or lost passing target: %+v %+v", s, a)
				}
			default:
				if s.Mechanism != AssessFail || a.Result != claimFail || a.Originals.Failed != 1 || a.Originals.Missing != 0 {
					t.Fatalf("invalid evidence called missing: %+v %+v", s, a)
				}
			}
		})
	}
}

func acceptedReaderQualifier(t *testing.T, root string, omitProfile bool) (*qualifier, QualWorkload, QualPlatform) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	prof, err := ParseIncrementalProfile(b)
	if err != nil {
		t.Fatal(err)
	}
	w := QualWorkload{Set: "reader-control", Profile: prof.ID, Route: prof.Route, Operation: prof.Operation, Output: prof.Output, Symbol: prof.Symbol,
		Grammar: prof.Grammar, RuntimeCommit: fxRuntime, ReuseAccepted: &QualReuseAcceptedRegistration{Policy: ReuseAcceptedSchema, LedgerSHA256: ReuseAcceptedLedgerSHA256, Control: "normal"}}
	c := prof.Cases[0]
	w.Cases = []QualCase{{ID: c.ID, Role: "support", Input: c.Input, Edits: c.Edits}}
	host := t.TempDir()
	if err := os.CopyFS(filepath.Join(host, "incremental", w.Set), os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	if omitProfile {
		if err := os.Remove(filepath.Join(host, "incremental", w.Set, "profile.json")); err != nil {
			t.Fatal(err)
		}
	}
	candidate := strings.Repeat("c", 40)
	id, _ := json.Marshal(RunIdentity{Schema: RunIdentitySchema, Repository: "o/r", Workflow: "fixture", RunID: "1", RunAttempt: "1", Event: "push", SHA: candidate, HeadSHA: candidate, Checkout: candidate, Job: "test", RunnerOS: "Windows", GOOS: "windows", GOARCH: "amd64", GoVersion: "fixture", EvidenceMode: ModeNewRun})
	if err := os.WriteFile(filepath.Join(host, "run-identity.json"), id, 0o600); err != nil {
		t.Fatal(err)
	}
	run, e := startRun(ctxT(t), QualificationLimits().Wall)
	if e != nil {
		t.Fatal(e)
	}
	q := &qualifier{r: run, lim: QualificationLimits(), candidate: candidate, inv: QualificationInventory{Schema: QualificationInventorySchema}, hosts: map[string]*qhost{}}
	p := qfxPlatforms[0]
	h, e := q.openHost(p, host, nil)
	if e != nil {
		t.Fatal(e)
	}
	q.hosts[p.ID] = h
	t.Cleanup(func() { q.close(); run.cancel() })
	return q, w, p
}
