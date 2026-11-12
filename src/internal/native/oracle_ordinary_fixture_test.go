package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// TestOracleOrdinaryOwnedFixture exports one real native-produced r8 for the kit reader test.
func TestOracleOrdinaryOwnedFixture(t *testing.T) {
	dir := os.Getenv("TSGK_ORDINARY_OWNED_FIXTURE")
	if dir == "" {
		dir = t.TempDir()
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	query := kit.OracleQuery{ID: "q0", Source: "(identifier) @id"}
	source := "a = f(1);\n" + strings.Repeat("b = 2;\n", 11) + "b = 2;"
	profile, root := oracleProfile(t, []kit.OracleQuery{query}, source)
	profile = withOperation(t, profile, "native-query-batch")
	p, err := kit.ParseOracleProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	edits := []kit.Edit{{StartByte: 6, OldEndByte: 7, NewEndByte: 7, Old: []byte("1"), New: []byte("2")}}
	p.Native.Cases[0].Edits, p.OracleCases[0].Edits = edits, edits
	var document map[string]any
	if err := json.Unmarshal(profile, &document); err != nil {
		t.Fatal(err)
	}
	document["cases"].([]any)[0].(map[string]any)["edits"] = edits
	profile, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	p, err = kit.ParseOracleProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	req := oracleRequest(t, profile, root, filepath.Join(dir, "r8"))
	// An empty cache makes this run a build cache miss; the kit reader test must accept that evidence.
	req.BuildCache = os.Getenv("TSGK_ORDINARY_OWNED_BUILD_CACHE")
	req.Ordinary = &OrdinaryRecordingRequest{
		Inputs:       kit.OrdinaryPlanRequest{Profile: []byte(`{"fixture":"planning-profile"}`), Assets: []byte(`{"fixture":"asset-registration"}`)},
		Registration: []byte(`{"fixture":"registration"}`),
		ShardPlan:    []byte(`{"fixture":"shard-plan"}`),
	}
	prepared := &ordinaryRecording{
		profile:      kit.OrdinaryProfile{Candidate: "owned-e2e", SemanticCohort: "owned-e2e-current"},
		registration: kit.OrdinaryRegistration{Route: "owned", Platform: "windows-amd64", Subject: "windows/amd64", ClosureEncoding: "plain"},
		sources:      [][]byte{[]byte(source)},
		shardIDs:     []string{"owned/windows-amd64/ca"},
	}
	initial := OracleResult{Report: kit.Report{Schema: kit.ReportSchema, Command: "oracle record", Findings: []kit.Finding{}, Coverage: kit.Coverage{Requested: []string{}, Observed: []string{}, Unsupported: []string{}}}, Cases: []CaseLine{}, Platform: hostPlatform}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	result, err := recordOrdinaryPrepared(ctx, req, p, nil, prepared, initial, time.Now())
	if err != nil || result.Set == nil || !result.Set.Valid || result.BuildRemoved != "REMOVED" {
		t.Fatalf("owned native fixture publication: err=%v set=%+v cleanup=%s", err, result.Set, result.BuildRemoved)
	}
	runBinding, err := json.Marshal(result.OrdinaryBinding)
	if err != nil {
		t.Fatal(err)
	}
	metadata := struct {
		Profile           []byte                   `json:"profile"`
		PlanningProfile   []byte                   `json:"planning_profile"`
		AssetRegistration []byte                   `json:"asset_registration"`
		Registration      kit.OrdinaryRegistration `json:"registration"`
		ShardPlan         []byte                   `json:"shard_plan"`
		ShardIDs          []string                 `json:"shard_ids"`
		RunBinding        []byte                   `json:"run_binding"`
		RunID             string                   `json:"run_id"`
	}{Profile: profile, PlanningProfile: req.Ordinary.Inputs.Profile, AssetRegistration: req.Ordinary.Inputs.Assets,
		Registration: prepared.registration, ShardPlan: req.Ordinary.ShardPlan, ShardIDs: prepared.shardIDs, RunBinding: runBinding, RunID: result.ObservationRunID}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "producer.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
