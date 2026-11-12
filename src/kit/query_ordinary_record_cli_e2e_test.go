package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func ordinaryCLIReplayProfile(t *testing.T, req OrdinaryQuerySemanticsRequest, result OrdinaryQuerySemanticsResult) (string, []byte) {
	t.Helper()
	root := t.TempDir()
	evidenceRoot := filepath.Join(root, "evidence")
	if err := os.Mkdir(evidenceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	var members []ReplayMember
	put := func(path, role string, data []byte) {
		t.Helper()
		target := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			t.Fatal(err)
		}
		members = append(members, ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)})
	}
	selection, err := json.Marshal(req.Selection)
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range []struct {
		path, role string
		data       []byte
	}{
		{"inputs/profile.json", "workload-profile", req.Profile},
		{"inputs/selection.json", "ordinary-selection", selection},
		{"inputs/planning.json", "ordinary-planning-profile", req.PlanningProfile},
		{"inputs/registration.json", "ordinary-registration", req.Registration},
		{"inputs/assets.json", "ordinary-assets", req.AssetRegistration},
		{"inputs/commitment.json", "ordinary-plan-commitment", req.PlanCommitment},
		{"inputs/registry.json", "ordinary-registry", req.Registry},
		{"inputs/qualification.json", "qualification-inventory", req.QualificationInventory},
		{"inputs/slots.json", "slot-inventory", req.SlotInventory},
		{"inputs/run-binding.json", "semantics-run-binding", req.RunBinding},
		{"inputs/references.json", "ordinary-reference-selection", req.ReferenceSelection},
		{"inputs/approval.json", "eligibility-approval", req.EligibilityApproval},
		{"inputs/eligibility-registration.json", "eligibility-registration", req.EligibilityRegistration},
		{"inputs/evidence-policy.json", "evidence-policy", req.Evidence.Policy},
	} {
		put(doc.path, doc.role, doc.data)
	}
	for i, proof := range req.ShardProofs {
		put(fmt.Sprintf("inputs/proof-%06d.json", i), "ordinary-shard-proof", proof)
	}
	var nested []ReplayMember
	err = filepath.WalkDir(req.Evidence.Root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || path == req.Evidence.Root {
			return walkErr
		}
		rel, err := filepath.Rel(req.Evidence.Root, path)
		if err != nil {
			return err
		}
		target := filepath.Join(evidenceRoot, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			return err
		}
		nested = append(nested, ReplayMember{Path: filepath.ToSlash(filepath.Join("evidence", rel)), Role: "ordinary-evidence", Bytes: uint64(len(data)), SHA256: digestHex(data)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(nested, func(a, b ReplayMember) int { return strings.Compare(a.Path, b.Path) })
	members = append(members, nested...)
	profile := ReplayProfile{
		ID: "ordinary-record-cli-replay", Reducer: "oracle-set-r9", Operation: "evidence-replay",
		Subject: ReplaySubject{Run: req.Selection.RunID, Attempt: 1, Platform: "windows/amd64", EvidenceMode: ModeNewRun, ExecutionStatus: StatusCompleted, Assessment: AssessBlocked},
		Identities: map[string]string{
			"workload": req.Selection.Profile.SHA256, "ordinary-selection": digestHex(selection), "ordinary-registration": req.Selection.Registration.SHA256,
			"ordinary-assets": req.Selection.Assets.SHA256, "ordinary-plan-commitment": req.Selection.Commitment.SHA256, "registry": req.RegistrySHA256,
			"semantics-run-binding": req.Selection.RunBinding.SHA256, "evidence-policy": digestHex(req.Evidence.Policy),
		},
		Records: slices.Clone(result.SelectedObligationIDs), Members: members,
	}
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	document["schema"] = ReplaySchema
	if data, err = json.Marshal(document); err != nil {
		t.Fatal(err)
	}
	return root, data
}

func ordinaryCLIRun(t *testing.T, ctx context.Context, exe string, args ...string) (stdout, stderr []byte, code int) {
	t.Helper()
	cmd := exec.CommandContext(ctx, exe, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if exit, ok := err.(*exec.ExitError); ok {
		return out.Bytes(), errOut.Bytes(), exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("%s: %v", exe, err)
	}
	return out.Bytes(), errOut.Bytes(), 0
}

// TestOrdinaryRecordCLIOwnedPostgreSQLPublicParity records the owned PostgreSQL case through `tsgk oracle
// record` with the real source-only inputs, then verifies the CLI-produced r8 through the public API and
// through the r9 replay (in process and through `tsgk replay`) and compares their results.
func TestOrdinaryRecordCLIOwnedPostgreSQLPublicParity(t *testing.T) {
	if os.Getenv("TSGK_ORDINARY_CLI_E2E") != "1" {
		t.Skip("set TSGK_ORDINARY_CLI_E2E=1 (Job-limited Windows native) to run the record CLI end to end")
	}
	runtimeRoot, compiler := ordinaryNativeTools(t)
	bundle := os.Getenv("TSGK_ORDINARY_CLI_BUNDLE")
	witnessPath := os.Getenv("TSGK_ORDINARY_P1_WITNESS")
	if bundle == "" || witnessPath == "" {
		t.Fatal("TSGK_ORDINARY_CLI_BUNDLE and TSGK_ORDINARY_P1_WITNESS are required")
	}
	witness, err := os.ReadFile(witnessPath)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err = filepath.Abs(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(bundle, 0o700); err != nil {
		t.Fatalf("the bundle directory must be new: %v", err)
	}
	in := ordinaryCLIRealInputs(t, witness)
	ordinaryCLIWriteBundle(t, bundle, in, witness)
	var inputBytes uint64
	for _, root := range []string{"ordinary"} {
		err := filepath.WalkDir(filepath.Join(bundle, root), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr == nil && !entry.IsDir() {
				info, err := entry.Info()
				if err != nil {
					return err
				}
				inputBytes += uint64(info.Size())
			}
			return walkErr
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	inputBytes += uint64(len(witness)) + uint64(len(in.PlanBytes)) // the two trusted documents outside the ordinary root

	exe := filepath.Join(bundle, "tsgk.exe")
	build := exec.Command("go", "build", "-o", exe, "./src/cmd/tsgk")
	build.Dir = repo
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the CLI: %v\n%s", err, output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	producerDir := filepath.Join(bundle, "producer")
	if err := os.Mkdir(producerDir, 0o700); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(bundle, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	record := []string{"oracle", "record", "--root", filepath.Join(bundle, "cases"), "--grammar-root", in.GrammarRoot, "--profile", filepath.Join(bundle, "oracle-profile.json"),
		"--fact-pack", filepath.Join(bundle, "fact-pack.json"), "--ordinary-root", filepath.Join(bundle, "ordinary"), "--ordinary-witness", filepath.Join(bundle, "witness.json"),
		"--ordinary-plan", filepath.Join(bundle, "plan.json"), "--ordinary-profile", in.Plan.Profile.Path, "--ordinary-assets", in.Plan.Assets.Path,
		"--ordinary-registry-sha256", ordinaryRegistrySHA, "--ordinary-registration", in.Plan.Registrations[0].Path, "--ordinary-shard", in.ShardID,
		"--runtime", runtimeRoot, "--tool", "cc=" + compiler, "--work", work,
		"--out", filepath.Join(producerDir, "r8"), "--allow", "BUILD_NATIVE", "--allow", "EXEC_NATIVE"}

	// Negative: a plan one byte over the 16 MiB file limit stops the loader before any native work.
	oversize := filepath.Join(bundle, "oversize.json")
	if err := os.WriteFile(oversize, bytes.Repeat([]byte{' '}, 16<<20+1), 0o600); err != nil {
		t.Fatal(err)
	}
	over := slices.Clone(record)
	over[slices.Index(over, "--ordinary-plan")+1] = oversize
	over[slices.Index(over, "--out")+1] = filepath.Join(producerDir, "never")
	if _, stderr, code := ordinaryCLIRun(t, ctx, exe, over...); code != 3 || !bytes.Contains(stderr, []byte("ORDINARY_FILE_LIMIT")) {
		t.Fatalf("a 16 MiB + 1 plan must be blocked by the loader: exit=%d stderr=%s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(producerDir, "never")); err == nil {
		t.Fatal("a refused loader must not publish a result")
	}

	stdout, stderr, code := ordinaryCLIRun(t, ctx, exe, record...)
	if err := os.WriteFile(filepath.Join(bundle, "record-stdout.json"), stdout, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "record-stderr.txt"), stderr, 0o600); err != nil {
		t.Fatal(err)
	}
	var produced struct {
		Binding         json.RawMessage `json:"ordinary_run_binding"`
		RunID           string          `json:"observation_run_id"`
		BuildRemoved    string          `json:"build_removed"`
		ExecutionStatus string          `json:"execution_status"`
		Assessment      string          `json:"assessment"`
		Summary         struct {
			Codes map[string]int `json:"codes"`
		} `json:"summary"`
		Set *struct {
			Valid bool `json:"valid"`
		} `json:"set"`
	}
	// A record only collects: the producer-local verdict is BLOCKED with EXTERNAL_APPROVAL_PENDING (exit 3).
	if err := json.Unmarshal(stdout, &produced); err != nil || code != 3 || len(produced.Binding) == 0 || produced.RunID == "" || produced.BuildRemoved != "REMOVED" ||
		produced.ExecutionStatus != StatusCompleted || produced.Assessment != AssessBlocked || produced.Summary.Codes["EXTERNAL_APPROVAL_PENDING"] != 1 || produced.Set == nil || !produced.Set.Valid {
		t.Fatalf("tsgk oracle record did not produce a complete pending ordinary r8: err=%v exit=%d\nstderr=%s\nstdout=%.2000s", err, code, stderr, stdout)
	}
	loader := regexp.MustCompile(`ORDINARY_LOADER read=(\d+) work=(\d+) logical=(\d+) entries=(\d+) wall_ms=(\d+)`).FindSubmatch(stderr)
	if loader == nil {
		t.Fatalf("the loader ledger is not reported: %s", stderr)
	}
	var ledger [5]uint64
	for i := range ledger {
		ledger[i], _ = strconv.ParseUint(string(loader[i+1]), 10, 64)
	}
	// The ledger covers the first read and the re-open of every file: at least twice the bytes on disk.
	if ledger[0] < 2*inputBytes || ledger[2] < 2*inputBytes || ledger[1] == 0 || ledger[3] < 2*uint64(len(in.PlanReq.Bodies)) {
		t.Fatalf("loader ledger is below what was read: ledger=%v input-bytes=%d", ledger, inputBytes)
	}
	metadata, err := json.Marshal(struct {
		Profile    []byte `json:"profile"`
		RunBinding []byte `json:"run_binding"`
		RunID      string `json:"run_id"`
	}{in.Profile, produced.Binding, produced.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(producerDir, "producer.json"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}

	references := ordinarySQLSyntheticReferences(t, in.Profile, in.Source, in.Registration, in.ExpectedTree, in.Metadata)
	req := ordinaryE2EAssemble(t, in.Request, producerDir, references, in.AssetBodies)
	api, err := VerifyOrdinaryQuerySemantics(ctx, req)
	if err != nil || api.Assessment != AssessPass || api.Semantic != AssessPass || !api.Complete || !api.ProcessingComplete || api.FullScopeComplete ||
		api.Tuple.Required == 0 || api.Tuple.Verified != api.Tuple.Required || api.Tuple.Failed != 0 || api.Tuple.Blocked != 0 {
		t.Fatalf("public ordinary API over the CLI-recorded r8: err=%v result=%+v", err, api)
	}
	root, profile := ordinaryCLIReplayProfile(t, req, api)
	replayed, err := Replay(ctx, ReplayRequest{Root: root, Profile: profile})
	if err != nil || replayed.OrdinaryQuerySemantics == nil || replayed.Assessment != api.Assessment {
		t.Fatalf("r9 replay over the CLI-recorded r8: err=%v result=%+v", err, replayed)
	}
	got := replayed.OrdinaryQuerySemantics
	if got.Semantic != api.Semantic || got.Code != api.Code || got.Tuple != api.Tuple || got.CoverageSHA256 != api.CoverageSHA256 ||
		!slices.Equal(got.SelectedObligationIDs, api.SelectedObligationIDs) || !validOrdinaryReplayResult(replayed) {
		t.Fatalf("API/replay parity changed: api=%+v replay=%+v", api, replayed)
	}
	profilePath := filepath.Join(bundle, "replay-profile.json")
	if err := os.WriteFile(profilePath, profile, 0o600); err != nil {
		t.Fatal(err)
	}
	cliOut, cliErr, cliCode := ordinaryCLIRun(t, ctx, exe, "replay", "--input", root, "--profile", profilePath)
	var cli ReplayResult
	if cliCode != 0 || json.Unmarshal(cliOut, &cli) != nil || cli.OrdinaryQuerySemantics == nil {
		t.Fatalf("tsgk replay: exit=%d stderr=%s stdout=%.1000s", cliCode, cliErr, cliOut)
	}
	cv := cli.OrdinaryQuerySemantics
	if cli.Assessment != replayed.Assessment || cv.Semantic != api.Semantic || cv.Code != api.Code || cv.Tuple != api.Tuple || cv.CoverageSHA256 != api.CoverageSHA256 ||
		!slices.Equal(cv.SelectedObligationIDs, api.SelectedObligationIDs) || cv.BytesRead != got.BytesRead || cv.Work != got.Work || cv.LogicalBytes != got.LogicalBytes {
		t.Fatalf("CLI replay differs from the API result: cli=%+v api=%+v", cv, api)
	}
	t.Logf("record CLI -> API/replay parity: selected=%d typed=%+v coverage=%s", len(api.SelectedObligationIDs), api.Tuple, api.CoverageSHA256)
	t.Logf("record loader ledger (wrapper cost of recording): read=%d work=%d logical=%d entries=%d wall_ms=%d input-bytes=%d", ledger[0], ledger[1], ledger[2], ledger[3], ledger[4], inputBytes)
	t.Logf("API budget read/work/logical=%d/%d/%d replay=%d/%d/%d wrapper-delta=%d/%d/%d", api.BytesRead, api.Work, api.LogicalBytes,
		got.BytesRead, got.Work, got.LogicalBytes, got.BytesRead-api.BytesRead, got.Work-api.Work, got.LogicalBytes-api.LogicalBytes)
}
