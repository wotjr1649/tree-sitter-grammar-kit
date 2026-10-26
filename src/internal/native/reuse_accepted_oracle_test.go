package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// A separate small r4 Oracle control costs three parse observations; the 404
// structural holdout observations are captured from their existing execution.
func TestReuseAcceptedOracle(t *testing.T) {
	if os.Getenv("TSGK_BENCH_PREPARED") == "" {
		t.Skip("registered prepared closure not configured")
	}
	if runtime.GOOS == "darwin" {
		t.Skip("new accepted claim is deferred on macOS")
	}
	rt, cc := nativeTools(t)
	ledger, err := os.ReadFile(filepath.Join("..", "..", "testdata", "native", "reuse-holdouts.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Keep original edit bytes and positions rather than deriving an equivalent final input.
	var rows struct{ Holdouts []json.RawMessage }
	if err := json.Unmarshal(ledger, &rows); err != nil {
		t.Fatal(err)
	}
	var original struct {
		ID     string
		Source string `json:"source_utf8"`
		Edits  []struct {
			Start  uint32 `json:"start_byte"`
			OldEnd uint32 `json:"old_end_byte"`
			NewEnd uint32 `json:"new_end_byte"`
			Old    string `json:"old_utf8"`
			New    string `json:"new_utf8"`
		}
	}
	for _, row := range rows.Holdouts {
		var id struct{ ID string }
		_ = json.Unmarshal(row, &id)
		if id.ID == "json-repair-close" {
			if err := json.Unmarshal(row, &original); err != nil {
				t.Fatal(err)
			}
		}
	}
	if original.ID == "" {
		t.Fatal("registered original missing")
	}
	root := t.TempDir()
	source := []byte(original.Source)
	input := kit.NativeInput{Path: original.ID + ".txt", Role: "case", SHA256: sha(source), Bytes: uint64(len(source))}
	if err := os.WriteFile(filepath.Join(root, input.Path), source, 0o644); err != nil {
		t.Fatal(err)
	}
	edits := []kit.Edit{}
	for _, e := range original.Edits {
		edits = append(edits, kit.Edit{StartByte: e.Start, OldEndByte: e.OldEnd, NewEndByte: e.NewEnd, Old: []byte(e.Old), New: []byte(e.New)})
	}
	cs := map[string]any{"id": original.ID, "input": input, "edits": edits, "points": []any{}, "expect": []any{}, "query_expect": []any{}, "dynamic_sql_expect": nil}
	hash, n := digestOf(t, cc)
	symbol, grammar := registeredGrammar(t, "json")
	doc := map[string]any{"schema": kit.OracleSchema, "id": "reuse-accepted-oracle-control", "route": "json", "operation": "native-query", "symbol": symbol, "encoding": kit.EncodingUTF8, "output": kit.OutputTree,
		"compiler": kit.ToolIdentity{Name: "cc", Version: "accepted-control", SHA256: hash, Bytes: n}, "grammar": grammar, "declarations": nil, "queries": []any{}, "api": false, "fact_pack": nil, "cases": []any{cs}}
	profile, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "set")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := Oracle(ctx, OracleRequest{Root: root, GrammarRoot: filepath.Join(os.Getenv("TSGK_BENCH_PREPARED"), "routes", "json"), Profile: profile, Runtime: rt, Compiler: cc, Work: t.TempDir(), Out: out, Allow: []string{AllowBuild, AllowExec}, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT"), ReuseAccepted: ledger})
	if err != nil || result.Assessment != kit.AssessPass || result.Set == nil || !result.Set.Valid || len(result.Cases) != 1 || result.Cases[0].ReuseAccepted == nil || result.Cases[0].ReuseAccepted.Result != ClaimPass {
		if b, readErr := os.ReadFile(filepath.Join(out, "manifest.json")); readErr == nil {
			var m struct {
				Operation json.RawMessage `json:"operation"`
			}
			_ = json.Unmarshal(b, &m)
			t.Logf("actual operation: %s", m.Operation)
		}
		t.Fatalf("Oracle accepted: err=%v assessment=%s set=%+v findings=%+v cases=%+v", err, result.Assessment, result.Set, result.Findings, result.Cases)
	}
	manBytes, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var man kit.OracleManifest
	if err := json.Unmarshal(manBytes, &man); err != nil || man.Schema != kit.OracleAcceptedManifestSchema || man.Protocol != ProtocolR4 || man.Operation == nil {
		t.Fatalf("manifest %+v %v", man, err)
	}
	replayRoot := t.TempDir()
	files := map[string][]byte{"manifest.json": manBytes, "profile.json": profile}
	roles := map[string]string{"manifest.json": "manifest", "profile.json": "workload-profile"}
	for _, m := range man.Members {
		b, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(m.Path)))
		if err != nil {
			t.Fatal(err)
		}
		files[m.Path] = b
		roles[m.Path] = map[string]string{"raw": "raw", "record": "record", "source": "case-source", "reuse-ledger": "reuse-ledger"}[m.Role]
	}
	members := []kit.ReplayMember{}
	for path, b := range files {
		full := filepath.Join(replayRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, b, 0o644); err != nil {
			t.Fatal(err)
		}
		members = append(members, kit.ReplayMember{Path: path, Role: roles[path], Bytes: uint64(len(b)), SHA256: sha(b)})
	}
	reg := kit.ReplayProfile{ID: "accepted-oracle-replay", Reducer: "oracle-set-r3", Operation: "evidence-replay", Subject: kit.ReplaySubject{Run: "1", Attempt: 1, Commit: "control", Platform: runtime.GOOS + "/" + runtime.GOARCH, EvidenceMode: kit.ModeNewRun, ExecutionStatus: kit.StatusCompleted, Assessment: kit.AssessPass}, Members: members, Records: []string{original.ID},
		Identities: map[string]string{"workload": sha(profile), "operation": "native-query", "producer": man.Producer["build_identity"], "executable": man.Producer["executable_sha256"], "compiler": man.Producer["compiler_sha256"], "runtime": man.Producer["runtime_commit"], "platform": man.Producer["platform"], "policy": man.Policy.SHA256, "protocol": man.Protocol, "comparators": strings.Join(man.Comparators, ","), "queries": "", "fact_pack": ""}}
	encoded, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	var regDoc map[string]any
	_ = json.Unmarshal(encoded, &regDoc)
	regDoc["schema"] = kit.ReplaySchema
	encoded, _ = json.Marshal(regDoc)
	replayed, err := kit.Replay(ctx, kit.ReplayRequest{Root: replayRoot, Profile: encoded})
	if err != nil || !replayed.EvidenceValid || replayed.Assessment != kit.AssessPass {
		t.Fatalf("Oracle accepted replay: %v %+v", err, replayed)
	}
	t.Log(fmt.Sprintf("accepted Oracle actual protocol=%s record=%s histories=1 parse_observations=3 replay=%s", man.Protocol, kit.OracleAcceptedRecordSchema, replayed.Assessment))
}
