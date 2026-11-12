package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestOrdinaryReplayFailureExitSurvivesCancellationAndOutputErrors(t *testing.T) {
	result := kit.ReplayResult{Report: kit.Report{Schema: kit.ReportSchema, Command: "replay", ExecutionStatus: kit.StatusCompleted, EvidenceMode: kit.ModeReplayedRaw, Assessment: kit.AssessFail},
		ResultSchema: kit.ReplayOrdinaryResultSchema, Reducer: kit.ReducerInfo{ID: "oracle-set-r9", Revision: "r9"}, Operation: "evidence-replay",
		OrdinaryQuerySemantics: &kit.OrdinaryQuerySemanticsResult{Report: kit.Report{Schema: kit.ReportSchema, Command: "ordinary query semantics", ExecutionStatus: kit.StatusCompleted, EvidenceMode: kit.ModeReplayedRaw, Assessment: kit.AssessFail},
			ResultSchema: kit.OrdinaryQuerySemanticsResultSchema, Code: "EXPECTATION_FAILED", Tuple: kit.QueryEligibilityCounts{Required: 1, Failed: 1}}}
	cancelled := &kit.Error{Kind: kit.KindCancelled, Code: "CANCELLED"}
	var out, stderr bytes.Buffer
	if code := finish(result, cancelled, "", "", &out, &stderr); code != exitFail {
		t.Fatalf("cancelled after known FAIL returned %d", code)
	}
	if code := finish(result, nil, "", "", failedWriter{}, &stderr); code != exitFail {
		t.Fatalf("stdout failure after known FAIL returned %d", code)
	}
	existing := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(existing, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := finish(result, nil, existing, "", &out, &stderr); code != exitFail {
		t.Fatalf("publication failure after known FAIL returned %d", code)
	}
	legacy := result
	legacy.ResultSchema, legacy.Reducer = kit.ReplaySemanticsResultSchema, kit.ReducerInfo{ID: "oracle-set-r4", Revision: "r4"}
	if code := finish(legacy, cancelled, "", "", &out, &stderr); code != exitCanceled {
		t.Fatalf("legacy replay cancellation changed to %d", code)
	}
}

func gateSet(t *testing.T, recomputedStatus string) (string, string) {
	t.Helper()
	root, dir := t.TempDir(), t.TempDir()
	doc := func(status string) []byte {
		b, _ := json.Marshal(map[string]any{"gate": "CANCEL", "status": status, "notes": []string{}, "points": []any{}})
		return b
	}
	files := map[string][]byte{"recorded/CANCEL.json": doc("PASS"), "recomputed/CANCEL.json": doc(recomputedStatus)}
	var members []any
	for _, p := range []string{"recomputed/CANCEL.json", "recorded/CANCEL.json"} {
		os.MkdirAll(filepath.Join(root, filepath.Dir(p)), 0o755)
		os.WriteFile(filepath.Join(root, filepath.FromSlash(p)), files[p], 0o644)
		s := sha256.Sum256(files[p])
		members = append(members, map[string]any{"path": p, "role": strings.Split(p, "/")[0] + "-gate", "bytes": len(files[p]), "sha256": hex.EncodeToString(s[:])})
	}
	reg, _ := json.Marshal(map[string]any{"schema": kit.ReplaySchema, "id": "cli", "reducer": "bs-gate-compare-r1", "operation": "evidence-replay",
		"subject":    map[string]any{"run": "1", "attempt": 1, "platform": "linux/amd64", "commit": "", "evidence_mode": "NEW_RUN", "execution_status": "COMPLETED", "assessment": "PASS"},
		"identities": map[string]string{}, "records": []string{"CANCEL"}, "members": members})
	prof := filepath.Join(dir, "replay.json")
	os.WriteFile(prof, reg, 0o644)
	return root, prof
}

// S07-A10/A11: the CLI runs the same core as the API (same bytes), refuses a registration
// inside the evidence root, maps UNRESOLVED to 3 and a failed check to 1, and never
// replaces an existing --out.
func TestReplayCLI(t *testing.T) {
	root, prof := gateSet(t, "PASS")
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"replay", "--input", root, "--profile", prof}, &out, &errb)
	if code != exitBlocked {
		t.Fatalf("unresolved replay exit %d: %s %s", code, out.String(), errb.String())
	}
	data, _ := os.ReadFile(prof)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	api, err := kit.Replay(ctx, kit.ReplayRequest{Root: root, Profile: data})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(api)
	if strings.TrimSuffix(out.String(), "\n") != string(want) {
		t.Fatalf("CLI and API differ:\n%s\n%s", out.String(), want)
	}
	root, prof = gateSet(t, "FAIL")
	if code := run(context.Background(), []string{"replay", "--input", root, "--profile", prof}, &out, &errb); code != exitFail {
		t.Fatalf("failed comparison exit %d", code)
	}
	inside := filepath.Join(root, "replay.json")
	os.WriteFile(inside, data, 0o644)
	errb.Reset()
	if code := run(context.Background(), []string{"replay", "--input", root, "--profile", inside}, &out, &errb); code != exitUsage || !strings.Contains(errb.String(), "PROFILE_INSIDE_INPUT") {
		t.Fatalf("profile inside input: %d %s", code, errb.String())
	}
	root, prof = gateSet(t, "PASS")
	exists := filepath.Join(t.TempDir(), "r.json")
	os.WriteFile(exists, []byte("keep"), 0o644)
	errb.Reset()
	if code := run(context.Background(), []string{"replay", "--input", root, "--profile", prof, "--out", exists}, &out, &errb); code != exitIO || !strings.Contains(errb.String(), "OUTPUT_EXISTS") {
		t.Fatalf("existing out: %d %s", code, errb.String())
	}
	if b, _ := os.ReadFile(exists); string(b) != "keep" {
		t.Fatal("existing --out was replaced")
	}
	// evidence verify of a malformed policy is an argument error
	pol := filepath.Join(t.TempDir(), "p.json")
	os.WriteFile(pol, []byte(`{"schema":"tsgk-evidence-policy/r1","schema":"x"}`), 0o644)
	errb.Reset()
	if code := run(context.Background(), []string{"evidence", "verify", "--input", root, "--profile", pol}, &out, &errb); code != exitUsage || !strings.Contains(errb.String(), "JSON_DUPLICATE_KEY") {
		t.Fatalf("evidence verify duplicate key: %d %s", code, errb.String())
	}
}

func TestReplayCLIRuntimePinsEnvelopeForwarding(t *testing.T) {
	root := t.TempDir()
	profile, err := json.Marshal(map[string]any{
		"schema": kit.ReplaySchema, "id": "runtime-pins-cli", "reducer": "native-result-r9", "operation": "evidence-replay",
		"subject":    map[string]any{"run": "run", "attempt": 1, "platform": "linux/amd64", "commit": "", "evidence_mode": kit.ModeNewRun, "execution_status": kit.StatusCompleted, "assessment": kit.AssessPass},
		"identities": map[string]string{}, "records": []string{}, "members": []kit.ReplayMember{},
	})
	if err != nil {
		t.Fatal(err)
	}
	pinSet := []byte(`{"schema":"` + kit.EvidencePinSetSchema + `","reader":"` + kit.EvidencePinReader + `","materials":[{"name":"native/runtime-tuple.json","role":"runtime-tuple","bytes":5,"sha256":"` + fmt.Sprintf("%x", sha256.Sum256([]byte("tuple"))) + `"}]}`)
	pinInput, err := json.Marshal(map[string]any{
		"schema": cliEvidencePinsInputSchema, "subject_base64": base64.StdEncoding.EncodeToString(profile),
		"approved_approval_sha256": strings.Repeat("0", 64), "approval_base64": base64.StdEncoding.EncodeToString([]byte(`{}`)),
		"approved_pin_set_sha256": strings.Repeat("0", 64), "pin_set_base64": base64.StdEncoding.EncodeToString(pinSet),
		"materials_base64": map[string]string{"native/runtime-tuple.json": base64.StdEncoding.EncodeToString([]byte("tuple"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	profilePath, pinsPath := filepath.Join(t.TempDir(), "profile.json"), filepath.Join(t.TempDir(), "pins.json")
	if err := os.WriteFile(profilePath, profile, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pinsPath, pinInput, 0600); err != nil {
		t.Fatal(err)
	}
	request, err := decodeCLIEvidencePins(pinInput, profile)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	want, wantErr := kit.Replay(ctx, kit.ReplayRequest{Root: root, Profile: profile, RuntimePins: request})
	if want.RuntimePins == nil || want.ResultSchema != kit.ReplayLoggerPinsResultSchema {
		t.Fatalf("API did not exercise runtime pin path: schema=%s runtime=%+v err=%v", want.ResultSchema, want.RuntimePins, wantErr)
	}
	var out, errb bytes.Buffer
	code := run(ctx, []string{"replay", "--input", root, "--profile", profilePath, "--runtime-pins", pinsPath}, &out, &errb)
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSuffix(out.String(), "\n") != string(wantJSON) || code != replayExit(want.Assessment) {
		t.Fatalf("CLI did not forward the bounded runtime pin envelope: exit=%d want=%d stderr=%s\n%s\n%s", code, replayExit(want.Assessment), errb.String(), out.String(), wantJSON)
	}
}

func TestReplayCLIPinsEnvelopeForwarding(t *testing.T) {
	root := t.TempDir()
	profile, err := json.Marshal(map[string]any{
		"schema": kit.ReplaySchema, "id": "reference-pins-cli", "reducer": "native-result-r9", "operation": "evidence-replay",
		"subject":    map[string]any{"run": "run", "attempt": 1, "platform": "linux/amd64", "commit": "", "evidence_mode": kit.ModeNewRun, "execution_status": kit.StatusCompleted, "assessment": kit.AssessPass},
		"identities": map[string]string{}, "records": []string{}, "members": []kit.ReplayMember{},
	})
	if err != nil {
		t.Fatal(err)
	}
	pinInput, err := json.Marshal(map[string]any{
		"schema": cliEvidencePinsInputSchema, "subject_base64": base64.StdEncoding.EncodeToString([]byte("graph approval subject")),
		"approved_approval_sha256": strings.Repeat("a", 64), "approval_base64": base64.StdEncoding.EncodeToString([]byte(`{}`)),
		"approved_pin_set_sha256": strings.Repeat("b", 64), "pin_set_base64": base64.StdEncoding.EncodeToString([]byte(`{}`)),
		"materials_base64": map[string]string{"reference/catalogue.json": base64.StdEncoding.EncodeToString([]byte("catalogue"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	profilePath, pinsPath := filepath.Join(t.TempDir(), "profile.json"), filepath.Join(t.TempDir(), "pins.json")
	if err := os.WriteFile(profilePath, profile, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pinsPath, pinInput, 0600); err != nil {
		t.Fatal(err)
	}
	pins, err := decodeCLIEvidencePins(pinInput, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	want, wantErr := kit.Replay(ctx, kit.ReplayRequest{Root: root, Profile: profile, Pins: pins})
	var ke *kit.Error
	if !errors.As(wantErr, &ke) || ke.Code != "REPLAY_PINS_REDUCER_MISMATCH" {
		t.Fatalf("API did not receive Pins: err=%v", wantErr)
	}
	var out, errb bytes.Buffer
	code := run(ctx, []string{"replay", "--input", root, "--profile", profilePath, "--pins", pinsPath}, &out, &errb)
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSuffix(out.String(), "\n") != string(wantJSON) || code != exitUsage || !strings.Contains(errb.String(), ke.Code) {
		t.Fatalf("CLI did not forward Pins envelope: exit=%d stderr=%s\n%s\n%s", code, errb.String(), out.String(), wantJSON)
	}
}

func TestQueryWitnessReplayCLI(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "missing-anchor"}[blocked], func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/query-witness.json")
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				Registration kit.QueryWitnessRegistration `json:"registration"`
				Source       string                       `json:"source"`
				Raw          json.RawMessage              `json:"raw"`
				Projection   json.RawMessage              `json:"projection"`
			}
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatal(err)
			}
			if blocked {
				fixture.Registration.Required[0].Expected = nil
			}
			regBytes, err := json.Marshal(fixture.Registration)
			if err != nil {
				t.Fatal(err)
			}
			root, outside := t.TempDir(), t.TempDir()
			sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
			files := map[string][]byte{"registration.json": regBytes, "source.txt": []byte(fixture.Source), "raw.json": fixture.Raw, "projection.json": fixture.Projection}
			roles := map[string]string{"registration.json": "query-registration", "source.txt": "case-source", "raw.json": "raw", "projection.json": "projection"}
			members := []kit.ReplayMember{}
			for _, name := range []string{"registration.json", "source.txt", "raw.json", "projection.json"} {
				if err := os.WriteFile(filepath.Join(root, name), files[name], 0644); err != nil {
					t.Fatal(err)
				}
				members = append(members, kit.ReplayMember{Path: name, Role: roles[name], Bytes: uint64(len(files[name])), SHA256: sum(files[name])})
			}
			profile, err := json.Marshal(map[string]any{"schema": kit.ReplaySchema, "id": "cli-query-witness", "reducer": "query-witness-r1", "operation": "evidence-replay",
				"subject":    kit.ReplaySubject{Run: "1", Attempt: 1, Platform: "windows/amd64", Commit: "", EvidenceMode: kit.ModeNewRun, ExecutionStatus: kit.StatusCompleted, Assessment: kit.AssessPass},
				"identities": map[string]string{"registration": sum(regBytes), "source": sum(files["source.txt"]), "route": fixture.Registration.Route, "platform": fixture.Registration.Platform, "role": fixture.Registration.Role, "policy": fixture.Registration.Policy},
				"records":    []string{fixture.Registration.Case}, "members": members})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(outside, "replay.json")
			if err := os.WriteFile(path, profile, 0644); err != nil {
				t.Fatal(err)
			}
			code, stdout, stderr := cli(t, context.Background(), "replay", "--input", root, "--profile", path)
			wantCode := exitOK
			if blocked {
				wantCode = exitBlocked
			}
			if code != wantCode {
				t.Fatalf("exit=%d want=%d: %s %s", code, wantCode, stdout, stderr)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			api, err := kit.Replay(ctx, kit.ReplayRequest{Root: root, Profile: profile})
			if err != nil {
				t.Fatal(err)
			}
			want, err := json.Marshal(api)
			if err != nil || strings.TrimSuffix(stdout, "\n") != string(want) {
				t.Fatalf("CLI/API mismatch: %v", err)
			}
		})
	}
}

func TestEvidenceReplayPlatformCLI(t *testing.T) {
	for _, platform := range []string{"windows/amd64", "linux/amd64"} {
		t.Run(platform, func(t *testing.T) {
			root := t.TempDir()
			write := func(path string, value any) []byte {
				t.Helper()
				data, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
				return data
			}
			hash := func(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
			report := write(filepath.Join(root, "replay.json"), map[string]any{
				"result_schema": kit.ReplayResultSchema, "evidence_mode": kit.ModeReplayedRaw, "assessment": kit.AssessFail,
				"subject": map[string]any{"run": "10", "attempt": 1, "platform": platform},
			})
			node := func(id, kind, mode, runID string) kit.EvidenceNode {
				ids := map[string]string{}
				for _, role := range kit.EvidenceRoles {
					ids[role] = role
				}
				ids["run"], ids["attempt"], ids["platform"] = runID, "1", "windows/amd64"
				return kit.EvidenceNode{ID: id, Kind: kind, ExecutionStatus: kit.StatusCompleted, EvidenceMode: mode, Assessment: kit.AssessFail, Identities: ids}
			}
			subject := node("subject", "run", kit.ModeNewRun, "10")
			replay := node("replay", "replay", kit.ModeReplayedRaw, "11")
			replay.Refs = []kit.EvidenceRef{{Relation: "subject", Node: "subject"}}
			replay.Files = []kit.ReplayMember{{Path: "replay.json", Role: "replay-result", Bytes: uint64(len(report)), SHA256: hash(report)}}
			graph := write(filepath.Join(root, "evidence.json"), map[string]any{"schema": kit.EvidenceSchema, "nodes": []kit.EvidenceNode{subject, replay}})
			policy := filepath.Join(t.TempDir(), "policy.json")
			write(policy, map[string]any{"schema": kit.EvidencePolicySchema, "id": "cli-platform", "evidence_sha256": hash(graph)})
			var out, errb bytes.Buffer
			code := run(context.Background(), []string{"evidence", "verify", "--input", root, "--profile", policy}, &out, &errb)
			var result kit.EvidenceResult
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatalf("exit %d, stderr %s: %v", code, errb.String(), err)
			}
			if platform == "windows/amd64" {
				if code != exitOK || result.Assessment != kit.AssessPass {
					t.Fatalf("matching FAIL report is valid evidence: %d %s", code, out.String())
				}
			} else if code != exitFail || result.Assessment != kit.AssessFail || len(result.Findings) != 1 || result.Findings[0].Code != "REPLAY_RESULT_MISMATCH" {
				t.Fatalf("cross-platform report accepted: %d %s", code, out.String())
			}
		})
	}
}

func TestQueryReferenceReplayCLI(t *testing.T) {
	for _, state := range []string{"pass", "blocked", "fail-blocked", "graph-witness-missing"} {
		t.Run(state, func(t *testing.T) {
			req, _ := queryReferenceCLISet(t, state)
			root, outside := t.TempDir(), t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "evidence"), 0700); err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{"connection.json": req.Registration, "query-registration.json": req.WitnessRegistration, "policy.json": req.Evidence.Policy}
			entries, err := os.ReadDir(req.Evidence.Root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				b, err := os.ReadFile(filepath.Join(req.Evidence.Root, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				files["evidence/"+entry.Name()] = b
			}
			sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
			members := []kit.ReplayMember{}
			for name, data := range files {
				role := "reference-evidence"
				switch name {
				case "connection.json":
					role = "reference-registration"
				case "query-registration.json":
					role = "query-registration"
				case "policy.json":
					role = "reference-policy"
				}
				if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), data, 0600); err != nil {
					t.Fatal(err)
				}
				members = append(members, kit.ReplayMember{Path: name, Role: role, Bytes: uint64(len(data)), SHA256: sum(data)})
			}
			var witness kit.QueryWitnessRegistration
			if err := json.Unmarshal(req.WitnessRegistration, &witness); err != nil {
				t.Fatal(err)
			}
			profile, err := json.Marshal(map[string]any{"schema": kit.ReplaySchema, "id": "cli-reference-replay", "reducer": "query-reference-connection-r1", "operation": "evidence-replay",
				"subject":    kit.ReplaySubject{Run: "native-run", Attempt: 1, Platform: witness.Platform, Commit: "", EvidenceMode: kit.ModeNewRun, ExecutionStatus: kit.StatusCompleted, Assessment: kit.AssessPass},
				"identities": map[string]string{"registration": sum(req.Registration), "witness": sum(req.WitnessRegistration), "evidence-policy": sum(req.Evidence.Policy), "source": witness.Input.SHA256, "route": witness.Route, "platform": witness.Platform, "role": witness.Role, "policy": witness.Policy},
				"records":    []string{witness.Case}, "members": members})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(outside, "replay.json")
			if err := os.WriteFile(path, profile, 0600); err != nil {
				t.Fatal(err)
			}
			code, out, errout := cli(t, context.Background(), "replay", "--input", root, "--profile", path)
			want := map[string]int{"pass": exitOK, "blocked": exitBlocked, "fail-blocked": exitFail, "graph-witness-missing": exitBlocked}[state]
			if code != want {
				t.Fatalf("exit %d != %d: %s %s", code, want, out, errout)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			api, err := kit.Replay(ctx, kit.ReplayRequest{Root: root, Profile: profile})
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(api)
			if err != nil || strings.TrimSuffix(out, "\n") != string(data) || api.ResultSchema != kit.ReplayReferenceResultSchema || api.QueryReference == nil {
				t.Fatalf("r2 CLI/API mismatch: %v %s", err, out)
			}
		})
	}
}
