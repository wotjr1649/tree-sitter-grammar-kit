package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// Caller-authored inputs are fixed before the actual producer runs; no reference is executed.
func TestOracleOrdinaryE2EFixture(t *testing.T) {
	input := os.Getenv("TSGK_ORDINARY_OWNED_INPUTS")
	if input == "" {
		t.Skip("explicit caller-authored ordinary fixture inputs required")
	}
	var in struct {
		Profile, PlanningProfile, AssetRegistration, ShardPlan []byte
		RegistrationBytes                                      []byte
		Registration                                           kit.OrdinaryRegistration
		ShardIDs                                               []string
		Sources                                                [][]byte
		GrammarRoot                                            string
		FactPack, FactMapping                                  []byte
	}
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &in); err != nil {
		t.Fatal(err)
	}
	dir := os.Getenv("TSGK_ORDINARY_OWNED_FIXTURE")
	if dir == "" {
		t.Fatal("TSGK_ORDINARY_OWNED_FIXTURE required")
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("ordinary fixture output must be an absolute path to a directory that does not exist yet")
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	p, err := kit.ParseOracleProfile(in.Profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Sources) != len(p.Native.Cases) {
		t.Fatal("source/case count mismatch")
	}
	root := t.TempDir()
	for i, c := range p.Native.Cases {
		path := filepath.Join(root, filepath.FromSlash(c.Input.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, in.Sources[i], 0600); err != nil {
			t.Fatal(err)
		}
	}
	rt, cc := nativeTools(t)
	registration := in.RegistrationBytes
	if len(registration) == 0 {
		registration, err = json.Marshal(in.Registration)
		if err != nil {
			t.Fatal(err)
		}
	}
	parsedRegistration, err := kit.ParseOrdinaryRegistration(registration)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsedRegistration, in.Registration) {
		t.Fatal("typed registration differs from the pinned caller bytes")
	}
	// Unselected plan rows are synthetic; the public call validates the selected commitment.
	var plan kit.OrdinaryShardPlan
	if err := json.Unmarshal(in.ShardPlan, &plan); err != nil {
		t.Fatal(err)
	}
	shardIDs, err := validateOrdinaryShardSelection(in.ShardIDs, plan, parsedRegistration, p.Native.Cases)
	if err != nil {
		t.Fatal(err)
	}
	req := OracleRequest{Root: root, GrammarRoot: in.GrammarRoot, Profile: in.Profile, Runtime: rt, Compiler: cc,
		Work: t.TempDir(), Out: filepath.Join(dir, "r8"), Allow: []string{AllowBuild, AllowExec},
		FactPack: in.FactPack, FactMapping: in.FactMapping, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT"),
		Ordinary: &OrdinaryRecordingRequest{Inputs: kit.OrdinaryPlanRequest{Profile: in.PlanningProfile, Assets: in.AssetRegistration},
			Registration: registration, ShardPlan: in.ShardPlan, ShardIDs: in.ShardIDs}}
	planning, err := kit.ParseOrdinaryProfile(in.PlanningProfile)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := bindOrdinaryExecution(context.Background(), req, p, planning, parsedRegistration)
	if err != nil {
		t.Fatal(err)
	}
	prepared.shardIDs = shardIDs
	var pack *kit.FactPack
	if len(in.FactPack) != 0 {
		value, err := kit.ParseFactPack(in.FactPack)
		if err != nil {
			t.Fatal(err)
		}
		pack = &value
	}
	initial := OracleResult{Report: kit.Report{Schema: kit.ReportSchema, Command: "oracle record", Findings: []kit.Finding{},
		Coverage: kit.Coverage{Requested: []string{}, Observed: []string{}, Unsupported: []string{}}}, Cases: []CaseLine{}, Platform: hostPlatform}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	result, produceErr := recordOrdinaryPrepared(ctx, req, p, pack, prepared, initial, time.Now())
	receipt, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "producer-result.json"), receipt, 0600); err != nil {
		t.Fatal(err)
	}
	if produceErr != nil || result.Set == nil || !result.Set.Valid || result.BuildRemoved != "REMOVED" {
		t.Fatalf("ordinary producer: err=%v set=%+v cleanup=%s", produceErr, result.Set, result.BuildRemoved)
	}
	if os.Getenv("TSGK_E2E_ENFORCE_NATIVE_LIMITS") == "1" {
		for _, step := range result.Build.Steps {
			if step.Result.Memory.Enforcement != runner.MemoryHard || step.Result.Memory.Limit != 4<<30 || !step.Result.Cleanup.Verified || step.Result.Cleanup.ResidualAfterExit != 0 {
				t.Fatalf("compile HARD receipt does not satisfy the task: %s %+v", step.Name, step.Result)
			}
		}
		var manifest kit.OrdinaryOracleManifest
		data, err := os.ReadFile(filepath.Join(req.Out, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}
		processes := 0
		for _, member := range manifest.Members {
			if member.Role != "process" {
				continue
			}
			processes++
			data, err := os.ReadFile(filepath.Join(req.Out, filepath.FromSlash(member.Path)))
			if err != nil {
				t.Fatal(err)
			}
			var process runner.Result
			if err := json.Unmarshal(data, &process); err != nil {
				t.Fatal(err)
			}
			if process.Memory.Enforcement != runner.MemoryHard || process.Memory.Limit != 1<<30 || process.WallMillis > 30000 || !process.Cleanup.Verified || process.Cleanup.ResidualAfterExit != 0 {
				t.Fatalf("native HARD receipt does not satisfy the task: %+v", process)
			}
		}
		if processes != len(manifest.Batches) {
			t.Fatal("native HARD process receipt count differs from the actual batches")
		}
		t.Logf("HARD receipts: compile=4GiB/120s native=1GiB/30s verified cleanup/residual0 batches=%d", processes)
	}
	runBinding, err := json.Marshal(result.OrdinaryBinding)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(struct {
		Profile    []byte `json:"profile"`
		RunBinding []byte `json:"run_binding"`
		RunID      string `json:"run_id"`
	}{in.Profile, runBinding, result.ObservationRunID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "producer.json"), metadata, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("actual ordinary r8 route=%s cases=%d cleanup=%s", p.Native.Route, len(p.Native.Cases), result.BuildRemoved)
}
