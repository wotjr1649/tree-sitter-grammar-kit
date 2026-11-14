package kit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func ordinaryReplayContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	t.Cleanup(cancel)
	return ctx
}

func ordinaryPartialReplayFixture(t *testing.T) (OrdinaryQuerySemanticsRequest, string, map[string]any) {
	t.Helper()
	req, _, _ := ordinarySQLCapacityRequest(t, "json")
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "evidence"), 0700); err != nil {
		t.Fatal(err)
	}
	selection := ordinaryJSON(t, req.Selection)
	members := []ReplayMember{}
	for _, input := range []struct {
		role string
		data []byte
	}{{"workload-profile", req.Profile}, {"ordinary-selection", selection}, {"ordinary-planning-profile", req.PlanningProfile},
		{"ordinary-registration", req.Registration}, {"ordinary-assets", req.AssetRegistration},
		{"ordinary-plan-commitment", req.PlanCommitment}, {"ordinary-registry", req.Registry},
		{"qualification-inventory", req.QualificationInventory}, {"slot-inventory", req.SlotInventory},
		{"semantics-run-binding", req.RunBinding}, {"ordinary-reference-selection", req.ReferenceSelection},
		{"eligibility-approval", req.EligibilityApproval}, {"eligibility-registration", req.EligibilityRegistration},
		{"evidence-policy", req.Evidence.Policy}, {"ordinary-shard-proof", req.ShardProofs[0]}} {
		path := input.role + ".json"
		if err := os.WriteFile(filepath.Join(root, path), input.data, 0600); err != nil {
			t.Fatal(err)
		}
		members = append(members, ReplayMember{Path: path, Role: input.role, Bytes: uint64(len(input.data)), SHA256: digestHex(input.data)})
	}
	api, err := VerifyOrdinaryQuerySemantics(ordinaryReplayContext(t), req)
	if err != nil {
		t.Fatal(err)
	}
	profile := map[string]any{"schema": ReplaySchema, "id": "ordinary-partial", "reducer": "oracle-set-r9", "operation": "evidence-replay",
		"subject": ReplaySubject{Run: req.Selection.RunID, Attempt: 1, Platform: ordinaryHostSubject(), EvidenceMode: ModeNewRun, ExecutionStatus: StatusCompleted, Assessment: AssessBlocked},
		"records": api.SelectedObligationIDs, "members": members,
		"identities": map[string]string{"workload": digestHex(req.Profile), "ordinary-selection": digestHex(selection),
			"ordinary-registration": digestHex(req.Registration), "ordinary-assets": digestHex(req.AssetRegistration),
			"ordinary-plan-commitment": digestHex(req.PlanCommitment), "registry": digestHex(req.Registry),
			"semantics-run-binding": digestHex(req.RunBinding), "evidence-policy": digestHex(req.Evidence.Policy)}}
	return req, root, profile
}

func TestOrdinaryReplayPartialAndStoredParity(t *testing.T) {
	t.Parallel()
	req, root, profile := ordinaryPartialReplayFixture(t)
	api, err := VerifyOrdinaryQuerySemantics(ordinaryReplayContext(t), req)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := Replay(ordinaryReplayContext(t), ReplayRequest{Root: root, Profile: ordinaryJSON(t, profile)})
	if err != nil || replayed.OrdinaryQuerySemantics == nil {
		t.Fatalf("ordinary replay: err=%v result=%+v", err, replayed)
	}
	p := replayed.OrdinaryQuerySemantics
	if p.Code != api.Code || p.Tuple != api.Tuple || !slices.Equal(p.SelectedObligationIDs, api.SelectedObligationIDs) || p.CoverageSHA256 != api.CoverageSHA256 ||
		!reflect.DeepEqual(p.Obligations, api.Obligations) || p.Complete || p.ProcessingComplete || p.FullScopeComplete || p.Assessment != AssessBlocked {
		t.Fatalf("partial API/replay semantics changed: api=%+v replay=%+v", api, p)
	}
	if p.BytesRead != replayed.BytesRead || p.BytesRead <= api.BytesRead || p.Work <= api.Work || p.LogicalBytes <= api.LogicalBytes {
		t.Fatalf("wrapping did not retain the same cumulative ledger: api=%+v replay=%+v", api, p)
	}
	decoded, err := DecodeOrdinaryQuerySemanticsResult(ordinaryJSON(t, p))
	if err != nil || !reflect.DeepEqual(decoded, *p) {
		t.Fatalf("stored partial changed: %v", err)
	}
	if !validOrdinaryReplayResult(replayed) {
		t.Fatalf("actual partial replay rejected by stored invariant: %+v", replayed)
	}
	t.Logf("API work=%d logical=%d read=%d; wrapping delta work=%d logical=%d read=%d", api.Work, api.LogicalBytes, api.BytesRead, p.Work-api.Work, p.LogicalBytes-api.LogicalBytes, p.BytesRead-api.BytesRead)
}

func TestOrdinaryReplayRejectsRecordAndMemberDamage(t *testing.T) {
	t.Parallel()
	_, root, profile := ordinaryPartialReplayFixture(t)
	for _, name := range []string{"deleted-id", "duplicate-id", "missing-document", "file-underflow", "file-overflow"} {
		t.Run(name, func(t *testing.T) {
			var changed map[string]any
			if err := json.Unmarshal(ordinaryJSON(t, profile), &changed); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "deleted-id":
				changed["records"] = []string{}
			case "duplicate-id":
				ids := changed["records"].([]any)
				changed["records"] = append(ids, ids[0])
			case "missing-document":
				members := changed["members"].([]any)
				changed["members"] = members[1:]
			default:
				members := changed["members"].([]any)
				member := members[1].(map[string]any)
				if name == "file-underflow" {
					member["bytes"] = member["bytes"].(float64) - 1
				} else {
					member["bytes"] = member["bytes"].(float64) + 1
				}
			}
			result, err := Replay(ordinaryReplayContext(t), ReplayRequest{Root: root, Profile: ordinaryJSON(t, changed)})
			if err == nil && result.Assessment != AssessFail {
				t.Fatalf("accepted damaged ordinary replay %s: %+v", name, result)
			}
		})
	}
}

func TestOrdinaryReplayInheritsExistingLedger(t *testing.T) {
	t.Parallel()
	req, _, _ := ordinarySQLCapacityRequest(t, "json")
	b := ordinaryTestBudget(t)
	b.r.eligibility = b
	b.work = eligibilityWorkLimit - 1
	b.r.totalRead = 123
	result, err := verifyOrdinaryQuerySemanticsInRun(context.Background(), req, b.r, &ordinaryReplayScope{})
	if err == nil || asError(err).Code != "ELIGIBILITY_WORK_LIMIT" || b.r.eligibility != b || result.Work < eligibilityWorkLimit-1 || result.BytesRead != 123 {
		t.Fatalf("ordinary replay reset its inherited ledger: err=%v work=%d read=%d same=%t", err, result.Work, result.BytesRead, b.r.eligibility == b)
	}
}

func TestOrdinaryReplayCannotRaiseInheritedLogicalCap(t *testing.T) {
	t.Parallel()
	req, _, _ := ordinarySQLCapacityRequest(t, "json")
	b := ordinaryTestBudget(t)
	b.r.eligibility = b
	b.logical = (1 << 30) - 1
	result, err := verifyOrdinaryQuerySemanticsInRun(ordinaryReplayContext(t), req, b.r, &ordinaryReplayScope{})
	if err == nil || asError(err).Code != "ELIGIBILITY_WORK_LIMIT" || b.r.eligibility != b || b.logicalCap() != 1<<30 || result.Complete {
		t.Fatalf("ordinary replay raised its inherited logical cap: err=%v cap=%d result=%+v", err, b.logicalCap(), result)
	}
}

func TestOrdinaryReplayCapsStayAtRegisteredSmallLimits(t *testing.T) {
	want := OrdinaryLimits{FileBytes: 16 << 20, ReadBytes: 256 << 20, Work: 32_000_000, Logical: 1 << 30, Entries: 10_000, WallMillis: 120_000}
	if OrdinaryPlanLimits() != want || eligibilityWorkLimit != want.Work || eligibilityLogicalLimit != want.Logical {
		t.Fatalf("ordinary small limits changed: %+v", OrdinaryPlanLimits())
	}
	lim := ReplayOperations()["evidence-replay"]
	if lim.FileBytes != want.FileBytes || lim.TotalBytes != want.ReadBytes || lim.Files != want.Entries || lim.WallMillis != int64(want.WallMillis) {
		t.Fatalf("ordinary replay inherited another capacity policy: %+v", lim)
	}
}

func TestOrdinaryReplayDoesNotActivateLegacyQualification(t *testing.T) {
	t.Parallel()
	_, root, profile := ordinaryPartialReplayFixture(t)
	b := ordinaryTestBudget(t)
	b.r.eligibility = b
	data := ordinaryJSON(t, profile)
	if _, err := parseQualificationReplay(data, b.r); err == nil || err.Code != "SEMANTICS_REDUCER_UNSUPPORTED" {
		t.Fatalf("r10 admitted an ordinary replay: %v", err)
	}
	_, err := replayShared(ordinaryReplayContext(t), ReplayRequest{Root: root, Profile: data}, &replayBudget{r: b.r, total: ReplayOperations()["evidence-replay"].TotalBytes})
	if err == nil || asError(err).Code != "ORDINARY_QUALIFICATION_UNSUPPORTED" {
		t.Fatalf("legacy qualification started a separate ordinary pass: %v", err)
	}
}

func TestOrdinaryReaderKeepsExactMixedCasePaths(t *testing.T) {
	f := newOrdinaryReaderFixture(t, "Case-B01")
	set, err := f.b.ordinaryGuardedSet(f.root, f.s, f.registration, f.profile, f.manifest, f.runBinding)
	if err != nil || set == nil || !set.Complete || containsOrdinaryFinding(set.Findings, "MEMBER_UNLISTED") {
		t.Fatalf("valid mixed-case member lost exact membership: err=%v set=%+v", err, set)
	}
}

func TestOrdinaryReplayReducerRolesAreCallerOwned(t *testing.T) {
	first := reducerTable()["oracle-set-r9"]
	first.info.Roles[0] = "caller-modified-policy"
	second := reducerTable()["oracle-set-r9"]
	if !slices.Equal(second.info.Roles, ordinaryReplayRoles) || second.info.Roles[0] == first.info.Roles[0] {
		t.Fatal("caller result mutation changed the replay role registry")
	}
}
