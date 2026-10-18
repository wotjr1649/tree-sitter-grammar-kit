package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

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
