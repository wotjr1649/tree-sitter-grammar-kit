package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func qualificationProbePlan(t *testing.T, inventory []byte) qualSemanticPlan {
	t.Helper()
	plan := qualSemanticPlan{Schema: QualificationSemanticsPlanSchema, ID: "small-probes", InventorySHA256: digestHex(inventory), Context: "full-input-utf8/r2", Scope: qualSemanticScope}
	for _, platform := range []string{"windows-amd64", "linux-amd64"} {
		p := qualSemanticPlanTarget{ID: "probe-" + platform, Route: "fxa", Role: "primary", Set: "s06-fxa", Platform: platform, ProbeID: "eq-probe", ProfileSHA256: digestHex([]byte("workload")), RegistrationSHA256: digestHex([]byte("registration")), Queries: []QualQuery{{ID: "eq", SHA256: digestHex([]byte(`((word) @x (#eq? @x "x"))`))}}}
		p.Required = []qualSemanticTuple{{Case: "probe-case", Query: "eq", Step: 0, TreeRole: "incremental"}, {Case: "probe-case", Query: "eq", Step: 1, TreeRole: "incremental"}, {Case: "probe-case", Query: "eq", Step: 1, TreeRole: "fresh"}}
		plan.Targets = append(plan.Targets, p)
	}
	return plan
}

func qualificationProbeBaseline(t *testing.T) QualifyRequest {
	t.Helper()
	f := newQfx(t)
	f.inv.Routes[1].Requirements[0].Kinds = []string{"P", "E", "Q"}
	paths := f.all(t, nil)
	req := QualifyRequest{Inventory: f.invBytes(t), Candidate: qfxCandidate}
	for _, p := range f.inv.Platforms {
		req.Hosts = append(req.Hosts, QualifyHost{Platform: p.ID, Root: paths[p.ID]})
	}
	return req
}

func TestQualifySemanticMissingInputsAndLegacyBytes(t *testing.T) {
	req := qualificationProbeBaseline(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	legacy, err := Qualify(ctx, req)
	if err != nil || legacy.Assessment != AssessPass {
		t.Fatalf("baseline: %v %s", err, legacy.Assessment)
	}
	before := mustJSON(t, legacy)
	req.QuerySemantics = nil
	again, err := Qualify(ctx, req)
	if err != nil || !bytes.Equal(before, mustJSON(t, again)) {
		t.Fatal("nil opt-in changed legacy bytes", err)
	}
	plan := qualificationProbePlan(t, req.Inventory)
	req.QuerySemantics = &QualificationQuerySemanticsRequest{Plan: mustJSON(t, plan)}
	res, err := Qualify(ctx, req)
	if err != nil || res.ResultSchema != QualificationSemanticsResultSchema || res.Assessment != AssessBlocked || res.SupportClaim != "BLOCKED" || res.Baseline == nil || res.Baseline.Assessment != AssessPass || res.Baseline.SupportClaim != "SUPPORTED" {
		t.Fatalf("missing inputs: %v %+v", err, res.Baseline)
	}
	if !bytes.Equal(mustJSON(t, legacy.Cells), mustJSON(t, res.Cells)) || !bytes.Equal(mustJSON(t, legacy.Totals), mustJSON(t, res.Totals)) {
		t.Fatal("baseline rows changed")
	}
	for _, row := range res.QuerySemantics.Targets {
		if row.Tuple == nil || *row.Tuple != (QueryEligibilityCounts{Required: 3, Blocked: 3}) || row.Core != nil || row.Digests != (QualSemanticDigests{}) {
			t.Fatalf("lost fixed denominator/null: %+v", row)
		}
	}
	if res.BytesRead != legacy.BytesRead+uint64(len(req.QuerySemantics.Plan)) || res.QuerySemantics.BytesRead != uint64(len(req.QuerySemantics.Plan)) {
		t.Fatal("common plan read was lost or counted twice")
	}
	for _, claim := range res.PlatformClaims {
		if claim != "BLOCKED" {
			t.Fatal("partial scope promoted platform")
		}
	}
	for i := range plan.Targets {
		plan.Targets[i].Required = []qualSemanticTuple{}
	}
	req.QuerySemantics.Plan = mustJSON(t, plan)
	zero, err := Qualify(ctx, req)
	if err != nil || zero.Assessment != AssessBlocked {
		t.Fatal("zero denominator", err)
	}
	for _, row := range zero.QuerySemantics.Targets {
		if row.Tuple != nil || row.State != "UNREGISTERED" {
			t.Fatalf("zero became 0/0: %+v", row)
		}
	}
	req.QuerySemantics.Plan = nil
	missing, err := Qualify(ctx, req)
	if err == nil || asError(err).Code != "SEMANTICS_PLAN_REQUIRED" || missing.QuerySemantics.PlanSHA256 != nil || missing.QuerySemantics.BytesRead != 0 {
		t.Fatal("missing plan was represented by an empty document digest", err)
	}
}

func TestQualifySemanticKnownFailureBeforeCancellation(t *testing.T) {
	req := qualificationProbeBaseline(t)
	plan := qualificationProbePlan(t, req.Inventory)
	plan.InventorySHA256 = strings.Repeat("e", 64)
	req.QuerySemantics = &QualificationQuerySemanticsRequest{Plan: mustJSON(t, plan)}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	old := testHookOpen
	testHookOpen = func(name string) {
		if strings.HasPrefix(name, "records/") {
			cancel()
		}
	}
	defer func() { testHookOpen = old }()
	res, err := Qualify(ctx, req)
	if err == nil || asError(err).Kind != KindCancelled || res.ExecutionStatus != StatusCancelled || res.Assessment != AssessFail || res.QuerySemantics.Assessment != AssessFail || res.QuerySemantics.Code != "SEMANTICS_PLAN_INVENTORY_MISMATCH" {
		t.Fatalf("failure erased by cancellation: %v %+v", err, res.QuerySemantics)
	}
	for _, row := range res.QuerySemantics.Targets {
		if row.Assessment != AssessFail || row.Tuple == nil || row.Tuple.Required != 3 || row.Tuple.Failed != 3 {
			t.Fatalf("partial denominator: %+v", row)
		}
	}
}

func TestQualifySemanticMalformedBorrowedSelector(t *testing.T) {
	req := qualificationProbeBaseline(t)
	plan := qualificationProbePlan(t, req.Inventory)
	req.QuerySemantics = &QualificationQuerySemanticsRequest{Plan: mustJSON(t, plan), Targets: []QualificationQuerySemanticsInput{{ID: plan.Targets[0].ID, Replay: ReplayRequest{Root: t.TempDir(), Profile: []byte(`[]`)}}}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := Qualify(ctx, req)
	if err == nil || res.QuerySemantics == nil || res.QuerySemantics.Work == 0 || res.QuerySemantics.Targets[0].Core != nil || res.QuerySemantics.Targets[0].Digests.Profile == nil {
		t.Fatalf("selector escaped S: %v %+v", err, res.QuerySemantics)
	}
	for _, row := range res.QuerySemantics.Targets {
		if row.Tuple == nil || row.Tuple.Required != 3 || row.Tuple.Unprocessed != 3 {
			t.Fatalf("selector reset plan denominator: %+v", row)
		}
	}
}

func TestQualifySemanticHostRejectionSurvivesInterruption(t *testing.T) {
	req := qualificationProbeBaseline(t)
	for _, host := range req.Hosts {
		if host.Platform != "windows-amd64" {
			continue
		}
		path := filepath.Join(host.Root, "run-identity.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var id RunIdentity
		if err := json.Unmarshal(data, &id); err != nil {
			t.Fatal(err)
		}
		id.SHA, id.Checkout = strings.Repeat("b", 40), strings.Repeat("b", 40)
		if err := os.WriteFile(path, mustJSON(t, id), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan := qualificationProbePlan(t, req.Inventory)
	profile := qualificationProbeReplayHeader(t, "linux/amd64")
	rawPlan := mustJSON(t, plan)
	req.QuerySemantics = &QualificationQuerySemanticsRequest{Plan: rawPlan, Targets: []QualificationQuerySemanticsInput{{ID: plan.Targets[1].ID, Replay: ReplayRequest{Root: t.TempDir(), Profile: profile}}}}
	old := testHookReplayLimits
	defer func() { testHookReplayLimits = old }()
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.TotalBytes = uint64(len(rawPlan) + len(profile) - 1); return l }
	res, err := Qualify(ctxT(t), req)
	if err == nil || asError(err).Kind != KindResourceLimit || res.Assessment != AssessFail || res.Baseline == nil || res.Baseline.Assessment != AssessFail {
		t.Fatalf("host rejection was erased: err=%v result=%+v", err, res)
	}
	if !slices.ContainsFunc(res.Findings, func(f Finding) bool { return f.Code == "ELIGIBILITY_REJECTED" }) {
		t.Fatal("host rejection cause was lost")
	}
}

func TestQualifySemanticExpectedHostIdentity(t *testing.T) {
	req := qualificationProbeBaseline(t)
	other, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	info, e := openedRootInfo(other, "")
	if e != nil {
		t.Fatal(e)
	}
	req.Hosts[0].ExpectedRootIdentity = info
	res, err := Qualify(ctxT(t), req)
	if err == nil || asError(err).Kind != KindIO || asError(err).Code != "SOURCE_CHANGED" || res.BytesRead != 0 {
		t.Fatalf("a replaced admitted host was consumed: err=%v bytes=%d", err, res.BytesRead)
	}
}

func TestQualifySemanticPlatformIDRequiresActualPair(t *testing.T) {
	req := qualificationProbeBaseline(t)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(req.Inventory, &raw); err != nil {
		t.Fatal(err)
	}
	var platforms []QualPlatform
	if err := json.Unmarshal(raw["platforms"], &platforms); err != nil {
		t.Fatal(err)
	}
	for i := range platforms {
		if platforms[i].ID == "windows-amd64" {
			platforms[i].GOOS = "linux"
		}
	}
	raw["platforms"] = mustJSON(t, platforms)
	req.Inventory = mustJSON(t, raw)
	plan := qualificationProbePlan(t, req.Inventory)
	req.QuerySemantics = &QualificationQuerySemanticsRequest{Plan: mustJSON(t, plan)}
	res, err := Qualify(ctxT(t), req)
	row := qualificationSemanticRow(res, "windows-amd64")
	if err != nil || res.Assessment != AssessFail || row == nil || row.Code != "SEMANTICS_PLATFORM_PAIR_MISMATCH" {
		t.Fatalf("a Linux pair was presented as Windows: err=%v row=%+v", err, row)
	}
}

func qualificationProbeReplayHeader(t *testing.T, platform string) []byte {
	t.Helper()
	return mustJSON(t, map[string]any{"schema": ReplaySchema, "id": "probe-replay", "operation": "evidence-replay", "reducer": "oracle-set-r4",
		"subject":    ReplaySubject{Run: "500", Attempt: 1, Platform: platform, Commit: qfxCandidate, EvidenceMode: ModeNewRun, ExecutionStatus: StatusCompleted, Assessment: AssessBlocked},
		"identities": map[string]string{"workload": strings.Repeat("a", 64), "semantics-registration": strings.Repeat("b", 64), "semantics-run-binding": strings.Repeat("c", 64), "evidence-policy": strings.Repeat("d", 64)},
		"records":    []string{"probe-case"}, "members": []ReplayMember{{Path: "evidence/evidence.json", Role: "semantics-evidence", Bytes: 1, SHA256: strings.Repeat("e", 64)}}})
}

func TestQualifySemanticGlobalAndHostReadCeilings(t *testing.T) {
	req := qualificationProbeBaseline(t)
	plan := qualificationProbePlan(t, req.Inventory)
	rawPlan := mustJSON(t, plan)
	profiles := [][]byte{qualificationProbeReplayHeader(t, "windows/amd64"), qualificationProbeReplayHeader(t, "linux/amd64")}
	inputs := []QualificationQuerySemanticsInput{{ID: plan.Targets[0].ID, Replay: ReplayRequest{Root: t.TempDir(), Profile: profiles[0]}}, {ID: plan.Targets[1].ID, Replay: ReplayRequest{Root: t.TempDir(), Profile: profiles[1]}}}
	req.QuerySemantics = &QualificationQuerySemanticsRequest{Plan: rawPlan, Targets: inputs}
	t.Run("global-second-target", func(t *testing.T) {
		old := testHookReplayLimits
		defer func() { testHookReplayLimits = old }()
		testHookReplayLimits = func(l ReplayLimits) ReplayLimits {
			l.TotalBytes = uint64(len(rawPlan) + len(profiles[0]) + len(profiles[1]) - 1)
			return l
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := Qualify(ctx, req)
		if err == nil || asError(err).Code != "TOTAL_BYTES_LIMIT" || res.QuerySemantics.BytesRead != uint64(len(rawPlan)+len(profiles[0])) {
			t.Fatalf("global read reset/overread: %v %+v", err, res.QuerySemantics)
		}
		first, second := res.QuerySemantics.Targets[0], res.QuerySemantics.Targets[1]
		if first.Tuple == nil || first.Tuple.Blocked != 3 || first.BytesRead != uint64(len(profiles[0])) || second.Tuple == nil || second.Tuple.Unprocessed != 3 || second.BytesRead != 0 || second.Digests.Profile != nil {
			t.Fatalf("second-target partial state: %+v %+v", first, second)
		}
	})
	t.Run("host-remaining-before-consumption", func(t *testing.T) {
		id, err := os.ReadFile(filepath.Join(req.Hosts[0].Root, "run-identity.json"))
		if err != nil {
			t.Fatal(err)
		}
		old := testHookQualifyLimits
		defer func() { testHookQualifyLimits = old }()
		testHookQualifyLimits = func(l ReplayLimits) ReplayLimits { l.TotalBytes = uint64(len(id) + len(profiles[0]) - 1); return l }
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := Qualify(ctx, req)
		if err == nil || asError(err).Code != "TOTAL_BYTES_LIMIT" || res.QuerySemantics.Targets[0].Digests.Profile != nil || res.QuerySemantics.Targets[0].BytesRead != 0 {
			t.Fatalf("host limit applied after consumption: %v %+v", err, res.QuerySemantics)
		}
	})
}

// This is an isolated synthetic fold, not a Windows/Linux execution receipt.
func qualificationAllProbePass(t *testing.T) (QualificationResult, *qualifier) {
	t.Helper()
	axis := newQualSemanticsAxis()
	sha := strings.Repeat("a", 64)
	axis.PlanSHA256 = &sha
	axis.Assessment = AssessPass
	axis.ProcessingComplete = true
	q := &qualifier{}
	s := &qualSemanticRun{q: q, axis: axis, lim: axis.Limits, planSHA: sha, r: &run{eligibility: &eligibilityBudget{}, semanticEntries: newSemanticEntryBudget(10000)}}
	q.semantic = s
	for i := range axis.Targets {
		row := &axis.Targets[i]
		row.ID = "probe-" + row.Platform
		row.Route = "json"
		row.Role = "primary"
		row.Set = "s06-json"
		row.ProbeID = "eq"
		row.State = "REPLAYED"
		row.Assessment = AssessPass
		row.Code = ""
		row.ExpectedProfileSHA256 = &sha
		row.ExpectedRegistrationSHA256 = &sha
		row.Digests = QualSemanticDigests{Profile: &sha, Approval: &sha, Context: &sha}
		row.Tuple = &QueryEligibilityCounts{Required: 1, Verified: 1}
		evidence := &EvidenceResult{Report: Report{Schema: ReportSchema, ExecutionStatus: StatusCompleted, EvidenceMode: ModeNewRun, Assessment: AssessPass}, ResultSchema: EvidenceResultSchema}
		eligibility := &QueryReferenceEligibilityResult{Report: Report{Schema: ReportSchema, ExecutionStatus: StatusCompleted, EvidenceMode: ModeReplayedRaw, Assessment: AssessPass}, ResultSchema: QueryReferenceEligibilityResultSchema, Complete: true, Evidence: evidence, Tuple: *row.Tuple,
			Build: QueryEligibilityCounts{Required: 9, Verified: 9}, Unit: QueryEligibilityCounts{Required: 4, Verified: 4}, Control: QueryEligibilityCounts{Required: 53, Verified: 53}, Assertion: QueryEligibilityCounts{Required: 36, Verified: 36}}
		row.Core = &QuerySemanticsResult{Report: Report{Schema: ReportSchema, Assessment: AssessPass, ExecutionStatus: StatusCompleted, EvidenceMode: ModeReplayedRaw}, ResultSchema: QuerySemanticsResultSchema, ProfileSHA256: sha, RegistrationSHA256: sha, RunBindingSHA256: sha, Complete: true, BindingValid: true, ProducerFoldValid: true, ProducerRecorded: Verdict{StatusCompleted, AssessBlocked}, Semantic: AssessPass, BaseVerified: Verdict{StatusCompleted, AssessPass}, Evidence: evidence, Eligibility: eligibility, Tuple: *row.Tuple}
		for _, id := range []string{"case-binding", "case-status", "tree", "verdict", "incremental-equality", "incremental-route", "expectations", "query-equality", "api", "query-expectations", "fact-reproduction", "dynamic-sql"} {
			row.Core.Gates = append(row.Core.Gates, ReplayGate{ID: id, EvidenceMode: ModeReplayedRaw, Assessment: AssessPass, Recomputed: 1})
		}
		rr := ReplayResult{Report: Report{Schema: ReportSchema, ExecutionStatus: StatusCompleted, EvidenceMode: ModeReplayedRaw, Assessment: AssessPass}, ResultSchema: ReplaySemanticsResultSchema, Reducer: ReducerInfo{ID: "oracle-set-r4", Revision: "r4"}, Operation: "evidence-replay", EvidenceValid: true, QuerySemantics: row.Core, Subject: ReplaySubject{Run: "synthetic", ExecutionStatus: StatusCompleted, Assessment: AssessBlocked, EvidenceMode: ModeNewRun}, Recorded: row.Core.ProducerRecorded, Recomputed: Verdict{StatusCompleted, AssessPass}, Observed: map[string]string{"workload": sha, "semantics-registration": sha, "semantics-run-binding": sha}, Consumption: Consumption{Expected: 1, Consumed: 1}, Gates: []ReplayGate{{ID: "query-semantics", EvidenceMode: ModeReplayedRaw, Assessment: AssessPass, Recomputed: 1}}}
		if !validSemanticReplayResult(rr) {
			t.Fatal("the isolated fold must start from an internally valid semantic core")
		}
		p := qualSemanticPlanTarget{ID: row.ID, Route: row.Route, Role: row.Role, Set: row.Set, Platform: row.Platform, ProbeID: row.ProbeID, ProfileSHA256: sha, RegistrationSHA256: sha, Queries: []QualQuery{{ID: "eq", SHA256: sha}}, Required: []qualSemanticTuple{{Case: "case", Query: "eq", TreeRole: "incremental"}}}
		s.targets = append(s.targets, &qualSemanticTargetRun{owner: s, plan: p, row: row, replay: &rr})
	}
	res := QualificationResult{Report: Report{Assessment: AssessPass, ExecutionStatus: StatusCompleted}, ResultSchema: QualificationSemanticsResultSchema, QuerySemantics: axis, SupportClaim: "SUPPORTED", Completeness: AssessPass, MechanismGate: AssessPass, PlatformClaims: map[string]string{"windows-amd64": "SUPPORTED", "linux-amd64": "SUPPORTED", "darwin-arm64": "SUPPORTED"}, Cells: make([]QualCell, 78), Totals: QualTotals{Cells: 78, Obligations: QualCounts{Obligations: 2478, Pass: 2478}}}
	q.foldQuerySemantics(&res)
	return res, q
}

func TestQualifySemanticNestedRootIdentityAdmission(t *testing.T) {
	r, e := startRun(ctxT(t), time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	defer r.cancel()
	r.eligibility = &eligibilityBudget{r: r}
	r.semanticEntries = newSemanticEntryBudget(10000)
	path := t.TempDir()
	if err := os.Mkdir(filepath.Join(path, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	host, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	childInfo, err := host.Lstat("nested")
	if err != nil {
		t.Fatal(err)
	}
	h := &qhost{x: &replayEnv{g: newGuard(host), directories: map[string]os.FileInfo{"nested": childInfo}}}
	q := &qualifier{hosts: map[string]*qhost{"host": h}, lim: QualificationLimits()}
	s := &qualSemanticRun{q: q, r: r}
	target := &qualSemanticTargetRun{owner: s, host: h, plan: qualSemanticPlanTarget{ID: "target"}, input: QualificationQuerySemanticsInput{Replay: ReplayRequest{Root: t.TempDir()}}}
	s.targets = []*qualSemanticTargetRun{target}
	defer func() {
		for _, root := range s.roots {
			root.Close()
		}
	}()
	target.input.ExpectedRootIdentity, e = openedRootInfo(host, "")
	if e != nil {
		t.Fatal(e)
	}
	if e := target.openRoot(); e == nil || e.Code != "SOURCE_CHANGED" || r.totalRead != 0 {
		t.Fatal("a replaced admitted target was consumed", e)
	}
	target.input.ExpectedRootIdentity = nil
	if e := target.openRoot(); e != nil {
		t.Fatal(e)
	}
	if e := target.directory(host, "nested"); e == nil || e.Code != "SEMANTIC_ROOT_OVERLAP" {
		t.Fatal("nested protected identity was not retained for the target walk", e)
	}
	if e := target.directory(target.root, "."); e == nil || e.Code != "SEMANTIC_ROOT_OVERLAP" {
		t.Fatal("target root self-alias was accepted", e)
	}
	if err := target.root.Mkdir("visited", 0o700); err != nil {
		t.Fatal(err)
	}
	if e := target.directory(target.root, "visited"); e != nil {
		t.Fatal(e)
	}
	if e := target.directory(target.root, "visited"); e == nil || e.Code != "SEMANTIC_ROOT_OVERLAP" {
		t.Fatal("nested directory alias was accepted", e)
	}
}

func TestQualifySemanticMemberIdentityBudget(t *testing.T) {
	r, e := startRun(ctxT(t), time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	defer r.cancel()
	r.eligibility = &eligibilityBudget{r: r}
	for _, members := range [][]ReplayMember{nil, {{Path: "a", Bytes: 1}}, {{Path: "z", Bytes: math.MaxUint64}, {Path: "a/b"}, {Path: "a", Role: "role"}, {Path: "한글"}, {Path: "c"}}} {
		before := slices.Clone(members)
		text, err := qualificationMembersText(members, r.eligibility)
		if err != nil || text != membersText(members) || !slices.Equal(before, members) {
			t.Fatalf("canonical member identity changed: %v %q", err, text)
		}
	}
	r.eligibility.work = eligibilityWorkLimit - 1
	if _, e := qualificationMembersText([]ReplayMember{{Path: "a"}}, r.eligibility); e == nil || e.Kind != KindResourceLimit || r.eligibility.work != eligibilityWorkLimit {
		t.Fatal("member work escaped the shared ledger", e)
	}
}

func TestQualifySemanticSelectorBillsInvalidInput(t *testing.T) {
	r, e := startRun(ctxT(t), time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	defer r.cancel()
	r.eligibility = &eligibilityBudget{r: r}
	data := []byte(`{"reducer":"not-r4"}`)
	if _, e := parseQualificationReplay(data, r); e == nil || r.eligibility.work < 2*uint64(len(data))+1 || r.eligibility.logical < 2*uint64(len(data))+256 {
		t.Fatal("failed selector escaped S work/allocation", e)
	}
	r.eligibility.work = eligibilityWorkLimit
	if _, e := parseQualificationReplay(data, r); e == nil || e.Kind != KindResourceLimit {
		t.Fatal("selector ran with an exhausted ledger", e)
	}
}

func TestQualifySemanticCallerDigestUsesWorkBudget(t *testing.T) {
	for _, exhausted := range []bool{false, true} {
		r, e := startRun(ctxT(t), time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		defer r.cancel()
		r.eligibility = &eligibilityBudget{r: r}
		if exhausted {
			r.eligibility.work = eligibilityWorkLimit
		}
		s := &qualSemanticRun{r: r, lim: ReplayOperations()["evidence-replay"]}
		data := []byte("caller document")
		err := s.document("caller-document", data)
		if exhausted {
			if err == nil || err.Kind != KindResourceLimit || r.totalRead != 0 {
				t.Fatal("hash ran or bytes were consumed without available work", err)
			}
		} else if err != nil || r.eligibility.work != uint64(len(data))+1 || r.eligibility.logical < 256 || r.totalRead != uint64(len(data)) {
			t.Fatal("caller digest reservation was lost or counted twice", err)
		}
	}
}

func TestQualifySemanticPartialScopeAndEmitterInvariants(t *testing.T) {
	res, q := qualificationAllProbePass(t)
	if res.Assessment != AssessBlocked || res.SupportClaim != "BLOCKED" || res.Baseline.Assessment != AssessPass || res.QuerySemantics.Assessment != AssessPass || len(res.Cells) != 78 || res.Totals.Obligations.Pass != 2478 || !validQualificationSemantics(res, q.semantic) {
		t.Fatal("partial probe scope promoted or lost baseline")
	}
	failed, failedQ := qualificationAllProbePass(t)
	row := &failed.QuerySemantics.Targets[0]
	row.Assessment, row.State, row.Code = AssessFail, "REJECTED", "KNOWN_FAILURE"
	row.Core.Assessment, row.Core.Code = AssessFail, row.Code
	row.Core.Findings = []Finding{{Severity: "error", Code: row.Code}}
	failed.Assessment, failed.QuerySemantics.Assessment, failed.QuerySemantics.Code = AssessFail, AssessFail, row.Code
	rr := failedQ.semantic.targets[0].replay
	rr.Assessment, rr.EvidenceValid, rr.Recomputed = AssessFail, false, Verdict{StatusCompleted, AssessFail}
	rr.Gates = []ReplayGate{{ID: "query-semantics", EvidenceMode: ModeReplayedRaw, Assessment: AssessFail, Recomputed: 1, Failed: 1, Code: row.Code}}
	if !validQualificationSemantics(failed, failedQ.semantic) {
		t.Fatal("the completed FAIL control must pass before child-result mutation")
	}
	cases := map[string]func(*QualificationResult){
		"global-pass":      func(r *QualificationResult) { r.Assessment = AssessPass; r.SupportClaim = "SUPPORTED" },
		"platform-pass":    func(r *QualificationResult) { r.PlatformClaims["windows-amd64"] = "SUPPORTED" },
		"missing-platform": func(r *QualificationResult) { delete(r.PlatformClaims, "windows-amd64") },
		"raised-budget":    func(r *QualificationResult) { r.QuerySemantics.WorkLimit++ },
		"counter-rebind":   func(r *QualificationResult) { r.QuerySemantics.Work++ },
		"full-scope":       func(r *QualificationResult) { r.QuerySemantics.FullScopeComplete = true },
		"missing-target":   func(r *QualificationResult) { r.QuerySemantics.Targets = r.QuerySemantics.Targets[:1] },
		"duplicate-platform": func(r *QualificationResult) {
			r.QuerySemantics.Targets[1].Platform = r.QuerySemantics.Targets[0].Platform
		},
		"plan-rebind": func(r *QualificationResult) { s := strings.Repeat("b", 64); r.QuerySemantics.PlanSHA256 = &s },
		"shrunk-all-counters": func(r *QualificationResult) {
			x := &r.QuerySemantics.Targets[0]
			x.Tuple = &QueryEligibilityCounts{}
			x.Core.Tuple = *x.Tuple
		},
		"overflow":        func(r *QualificationResult) { r.QuerySemantics.Targets[0].Tuple.Failed = math.MaxUint64 },
		"incomplete-core": func(r *QualificationResult) { r.QuerySemantics.Targets[0].Core.Complete = false },
		"unrun-core":      func(r *QualificationResult) { r.QuerySemantics.Targets[0].Core.ExecutionStatus = StatusNotRun },
		"failed-core":     func(r *QualificationResult) { r.QuerySemantics.Targets[0].Core.Assessment = AssessFail },
		"failed-core-overflow": func(r *QualificationResult) {
			x := &r.QuerySemantics.Targets[0]
			x.Assessment, x.Core.Assessment = AssessFail, AssessFail
			r.Assessment, r.QuerySemantics.Assessment, r.QuerySemantics.Code = AssessFail, AssessFail, "KNOWN_FAILURE"
			x.Core.Tuple.Verified = math.MaxUint64
		},
		"failed-core-incomplete": func(r *QualificationResult) {
			x := &r.QuerySemantics.Targets[0]
			x.Assessment, x.Core.Assessment = AssessFail, AssessFail
			r.Assessment, r.QuerySemantics.Assessment, r.QuerySemantics.Code = AssessFail, AssessFail, "KNOWN_FAILURE"
			x.Core.ExecutionStatus = StatusResourceLimit
		},
		"failed-core-hidden-processing": func(r *QualificationResult) {
			x := &r.QuerySemantics.Targets[0]
			x.Assessment, x.Core.Assessment = AssessFail, AssessFail
			r.Assessment, r.QuerySemantics.Assessment, r.QuerySemantics.Code = AssessFail, AssessFail, "KNOWN_FAILURE"
			x.Core.Complete = false
		},
		"failed-core-missing-eligibility": func(r *QualificationResult) {
			x := &r.QuerySemantics.Targets[0]
			x.Assessment, x.Core.Assessment = AssessFail, AssessFail
			r.Assessment, r.QuerySemantics.Assessment, r.QuerySemantics.Code = AssessFail, AssessFail, "KNOWN_FAILURE"
			x.Core.Eligibility = nil
		},
		"failed-core-missing-evidence": func(r *QualificationResult) {
			x := &r.QuerySemantics.Targets[0]
			x.Assessment, x.Core.Assessment = AssessFail, AssessFail
			r.Assessment, r.QuerySemantics.Assessment, r.QuerySemantics.Code = AssessFail, AssessFail, "KNOWN_FAILURE"
			x.Core.Evidence = nil
		},
		"failed-core-interrupted-eligibility": func(r *QualificationResult) {
			x := &r.QuerySemantics.Targets[0]
			x.Assessment, x.Core.Assessment = AssessFail, AssessFail
			r.Assessment, r.QuerySemantics.Assessment, r.QuerySemantics.Code = AssessFail, AssessFail, "KNOWN_FAILURE"
			x.Core.Eligibility.ExecutionStatus = StatusResourceLimit
		},
		"core-profile": func(r *QualificationResult) { r.QuerySemantics.Targets[0].Core.ProfileSHA256 = strings.Repeat("b", 64) },
		"core-registration": func(r *QualificationResult) {
			r.QuerySemantics.Targets[0].Core.RegistrationSHA256 = strings.Repeat("b", 64)
		},
		"source-target": func(r *QualificationResult) { r.QuerySemantics.Targets[0].Route = "go" },
		"hidden-missing-scope": func(r *QualificationResult) {
			r.QuerySemantics.MissingScopes = slices.Delete(r.QuerySemantics.MissingScopes, 0, 1)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			base, witness := res, q.semantic
			if strings.HasPrefix(name, "failed-core-") {
				base, witness = failed, failedQ.semantic
			}
			var changed QualificationResult
			if err := json.Unmarshal(mustJSON(t, base), &changed); err != nil {
				t.Fatal(err)
			}
			changed.QuerySemantics.Limits.Wall = base.QuerySemantics.Limits.Wall
			if !validQualificationSemantics(changed, witness) {
				t.Fatal("unmodified clone must pass before mutation")
			}
			mutate(&changed)
			if validQualificationSemantics(changed, witness) {
				t.Fatal("forged result accepted")
			}
		})
	}
}
