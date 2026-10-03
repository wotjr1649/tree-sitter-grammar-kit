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
