package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// withOperation returns the profile with another operation.
func withOperation(t *testing.T, profile []byte, op string) []byte {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal(profile, &p); err != nil {
		t.Fatal(err)
	}
	p["operation"] = op
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// comparable drops what a batch record legitimately records differently from a single one:
// the process or frame, the record schema, every elapsed time, the observation run and the
// identities (the operation's policy differs; TestOracleBatchSet checks the binding apart).
func comparable(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if k != "process" && k != "transport" && k != "schema" && !strings.HasSuffix(k, "_ms") && k != "run_id" && k != "observation_run_id" && k != "identities" {
				out[k] = comparable(e)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = comparable(e)
		}
		return out
	}
	return v
}

// firstDiff is the path of the first difference between two decoded documents.
func firstDiff(a, b any, path string) string {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return path + "{}"
		}
		for k, v := range x {
			if d := firstDiff(v, y[k], path+"/"+k); d != "" {
				return d
			}
		}
		return ""
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return path + "[]"
		}
		for i := range x {
			if d := firstDiff(x[i], y[i], fmt.Sprintf("%s/%d", path, i)); d != "" {
				return d
			}
		}
		return ""
	}
	if !reflect.DeepEqual(a, b) {
		return path
	}
	return ""
}

func readDoc(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// rewriteMember changes one member of a published set and re-binds the manifest to it, so
// only the batch rules can reject the result.
func rewriteMember(t *testing.T, set, rel string, change func(map[string]any)) {
	t.Helper()
	v := readDoc(t, filepath.Join(set, rel))
	change(v)
	writeMember(t, set, rel, v)
}

func writeMember(t *testing.T, set, rel string, v map[string]any) {
	t.Helper()
	data := marshalOrdered(t, v)
	if err := os.WriteFile(filepath.Join(set, rel), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if rel == "manifest.json" {
		return
	}
	sum := sha256.Sum256(data)
	rewriteManifest(t, set, func(m map[string]any) {
		for _, mem := range m["members"].([]any) {
			if mm := mem.(map[string]any); mm["path"] == rel {
				mm["bytes"], mm["sha256"] = float64(len(data)), hex.EncodeToString(sum[:])
			}
		}
	})
}

func rewriteManifest(t *testing.T, set string, change func(map[string]any)) {
	t.Helper()
	m := readDoc(t, filepath.Join(set, "manifest.json"))
	change(m)
	writeMember(t, set, "manifest.json", m)
}

// marshalOrdered writes a decoded document back with "complete" last, as the producer does.
func marshalOrdered(t *testing.T, v map[string]any) []byte {
	t.Helper()
	complete, has := v["complete"]
	delete(v, "complete")
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		tail, _ := json.Marshal(complete)
		data = append(append(data[:len(data)-1], []byte(`,"complete":`)...), append(tail, '}', '\n')...)
	}
	return data
}

// #266: a native-query-batch set answers every case as the native-query set does (apart from
// process, frame and time), binds each record to a frame of an accepted batch, records a
// case whose input is gone as never sent, and the published set verifies.
func TestOracleBatchSet(t *testing.T) {
	t.Parallel()
	queries := []kit.OracleQuery{{ID: "q.ids", Source: "(identifier) @id\n"}, {ID: "q.numbers", Source: "(number) @n\n"}}
	profile, root := oracleProfile(t, queries, "a = f(1);\n", "b = 2;\n", "c = (;\n", "d = g(3, 4);\n")
	sets := map[string]string{}
	for _, op := range []string{"native-query", "native-query-batch"} {
		out := filepath.Join(t.TempDir(), "set")
		req := oracleRequest(t, withOperation(t, profile, op), root, out)
		req.ReuseObservation = true
		res, err := runOracle(t, req)
		if err != nil || res.Set == nil || !res.Set.Valid || len(res.Cases) != 4 {
			t.Fatalf("%s: %v %s %+v %+v", op, err, res.ExecutionStatus, res.Set, res.Findings)
		}
		sets[op] = out
	}
	set := sets["native-query-batch"]
	m := readDoc(t, filepath.Join(set, "manifest.json"))
	batches := m["batches"].([]any)
	if m["schema"] != kit.OracleBatchManifestSchema || len(batches) != 1 || batches[0].(map[string]any)["reason"] != "EXITED" {
		t.Fatalf("manifest %v %v", m["schema"], batches)
	}
	for k, id := range []string{"ca", "cb", "cc", "cd"} {
		name := filepath.Join("records", "0000"+string(rune('0'+k))+"-"+id+".json")
		b, s := readDoc(t, filepath.Join(set, name)), readDoc(t, filepath.Join(sets["native-query"], name))
		tr, _ := b["transport"].(map[string]any)
		if b["schema"] != kit.OracleBatchRecordSchema || b["process"] != nil || tr["batch"] != float64(0) || tr["frame"] != float64(k) || tr["state"] != "ANSWERED" {
			t.Fatalf("%s: schema %v process %v transport %v", id, b["schema"], b["process"], b["transport"])
		}
		ids := b["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["tree"].(map[string]any)["identities"].([]any)
		if policy := ids[2].(map[string]any); policy["sha256"] != m["policy"].(map[string]any)["sha256"] {
			t.Errorf("%s: tree policy %v is not the set's policy", id, policy)
		}
		if d := firstDiff(comparable(b), comparable(s), ""); d != "" {
			t.Errorf("%s: the batch record differs from the single record at %s", id, d)
		}
	}
	// a case whose input is gone is refused before any frame: NOT_RUN and never sent
	if err := os.Remove(filepath.Join(root, "caseb.txt")); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(t.TempDir(), "set")
	res, err := runOracle(t, oracleRequest(t, withOperation(t, profile, "native-query-batch"), root, gone))
	r := readDoc(t, filepath.Join(gone, "records", "00001-cb.json"))
	if err != nil || !res.Set.Valid || r["execution_status"] != kit.StatusNotRun || r["transport"] != nil {
		t.Fatalf("missing input: %v %+v %v %v", err, res.Set, r["execution_status"], r["transport"])
	}
	// no input at all: no process runs, and the set still verifies with an empty batch list
	for _, name := range []string{"casea.txt", "casec.txt", "cased.txt"} {
		if err := os.Remove(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	none := filepath.Join(t.TempDir(), "set")
	res, err = runOracle(t, oracleRequest(t, withOperation(t, profile, "native-query-batch"), root, none))
	if b, ok := readDoc(t, filepath.Join(none, "manifest.json"))["batches"].([]any); err != nil || !res.Set.Valid || !ok || len(b) != 0 {
		t.Fatalf("no input: %v %+v %+v", err, res.Set, res.Findings)
	}
}

// #266: the shared batch rules reject a frame bound to another case, a missing raw response,
// a completed case of a batch that did not exit 0 by itself, an r1 record in an r3 set and an
// r3 manifest without batches.
func TestOracleBatchSetControls(t *testing.T) {
	t.Parallel()
	profile, root := oracleProfile(t, []kit.OracleQuery{{ID: "q", Source: "(identifier) @id\n"}}, "a = f(1);\n", "b = 2;\n")
	fresh := func() string {
		out := filepath.Join(t.TempDir(), "set")
		res, err := runOracle(t, oracleRequest(t, withOperation(t, profile, "native-query-batch"), root, out))
		if err != nil || !res.Set.Valid {
			t.Fatalf("%v %+v", err, res.Set)
		}
		return out
	}
	for name, c := range map[string]struct {
		change func(t *testing.T, set string)
		code   string
	}{
		"frame-of-another-case": {func(t *testing.T, set string) {
			rewriteMember(t, set, "records/00000-ca.json", func(r map[string]any) { r["transport"].(map[string]any)["frame"] = float64(1) })
		}, "BATCH_FRAME_MISMATCH"},
		"raw-missing": {func(t *testing.T, set string) {
			os.Remove(filepath.Join(set, "raw", "00000-ca.json"))
			rewriteManifest(t, set, func(m map[string]any) {
				var keep []any
				for _, mem := range m["members"].([]any) {
					if mem.(map[string]any)["path"] != "raw/00000-ca.json" {
						keep = append(keep, mem)
					}
				}
				m["members"] = keep
			})
		}, "BATCH_RAW_MISMATCH"},
		"batch-not-accepted": {func(t *testing.T, set string) {
			rewriteManifest(t, set, func(m map[string]any) { m["batches"].([]any)[0].(map[string]any)["exit_code"] = float64(5) })
		}, "BATCH_SUCCESS_UNACCEPTED"},
		"batch-lingered": {func(t *testing.T, set string) {
			rewriteManifest(t, set, func(m map[string]any) { m["batches"].([]any)[0].(map[string]any)["reason"] = runner.ReasonBatchExit })
		}, "BATCH_SUCCESS_UNACCEPTED"},
		"r1-record": {func(t *testing.T, set string) {
			rewriteMember(t, set, "records/00001-cb.json", func(r map[string]any) { r["schema"] = kit.OracleRecordSchema; delete(r, "transport") })
		}, "RECORD_IDENTITY_MISMATCH"},
		"raw-of-another-case": {func(t *testing.T, set string) {
			other, err := os.ReadFile(filepath.Join(set, "raw", "00001-cb.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(set, "raw", "00000-ca.json"), other, 0o644); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(other)
			rewriteManifest(t, set, func(m map[string]any) {
				for _, mem := range m["members"].([]any) {
					if mm := mem.(map[string]any); mm["path"] == "raw/00000-ca.json" {
						mm["bytes"], mm["sha256"] = float64(len(other)), hex.EncodeToString(sum[:])
					}
				}
			})
		}, "BATCH_RAW_MISMATCH"},
		"batches-missing": {func(t *testing.T, set string) {
			rewriteManifest(t, set, func(m map[string]any) { delete(m, "batches") })
		}, "JSON_MISSING_FIELD"},
	} {
		t.Run(name, func(t *testing.T) {
			set := fresh()
			c.change(t, set)
			got := kit.VerifyOracleSet(os.DirFS(set))
			found := false
			for _, f := range got.Findings {
				found = found || f.Code == c.code
			}
			if got.Valid || !found {
				t.Fatalf("want %s: %+v", c.code, got.Findings)
			}
		})
	}
}

// #266: the frame after a fatal resource response was never read: it moves to a new process,
// the fatal case keeps its RESOURCE_LIMIT, and the earlier answered case of that process is
// not accepted (exit 3 is not a clean end). A leaking driver ends after its first answer: that
// answer is not accepted and the frame it never answered fails without being sent again.
func TestOracleBatchFailures(t *testing.T) {
	t.Parallel()
	x := oracleContext(false, "(identifier) @id\n")
	x.Op = kit.NativeOperations()["native-query-batch"]
	x.PolicyRef = PolicyRef(x.Op, x.Output)
	prepare := func(b *Build, n int) ([]kit.IncrementalCase, []caseRequest, []CaseResult, []int) {
		var cases []kit.IncrementalCase
		var reqs []caseRequest
		var out []CaseResult
		var pending []int
		for i := range n {
			c := kit.IncrementalCase{ID: fmt.Sprintf("c%02d", i), Encoding: kit.EncodingUTF8}
			res := newCaseResult(c)
			cr, done := b.prepareCase(context.Background(), x, c, []byte("a = f(1);\n"), &res)
			if done {
				t.Fatalf("case %d refused: %s", i, res.Code)
			}
			cases, reqs, out, pending = append(cases, c), append(reqs, cr), append(out, res), append(pending, i)
		}
		return cases, reqs, out, pending
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	t.Run("fatal", func(t *testing.T) {
		b := fixtureBuild(t, "plain")
		cases, reqs, out, pending := prepare(b, 3)
		reqs[1].req.Limits.MemoryBytes = 4096
		reqs[1].payload = reqs[1].req.Encode()
		batches := b.runOracleBatches(ctx, x, cases, reqs, out, pending)
		want := []struct {
			status, code string
			batch, frame int
		}{{kit.StatusFailed, "BATCH_PROCESS_FAILED", 0, 0}, {kit.StatusResourceLimit, "ALLOCATION_LIMIT", 0, 1}, {kit.StatusCompleted, "", 1, 0}}
		if len(batches) != 2 || batches[0].Accepted() || batches[0].ExitCode != 3 || !batches[1].Accepted() {
			t.Fatalf("batches %+v", batches)
		}
		for i, w := range want {
			tr := out[i].Transport
			if out[i].ExecutionStatus != w.status || out[i].Code != w.code || tr == nil || tr.Batch != w.batch || tr.Frame != w.frame || tr.State != "ANSWERED" {
				t.Errorf("case %d: %s %s %+v", i, out[i].ExecutionStatus, out[i].Code, tr)
			}
		}
		if out[0].Raw == nil || out[0].Claims != (Claims{ClaimNotClaimed, ClaimNotClaimed, ClaimNotClaimed}) || out[0].Oracle != nil {
			t.Errorf("a rejected answer keeps its raw and claims nothing: %+v %+v", out[0].Claims, out[0].Oracle)
		}
	})
	t.Run("linger", func(t *testing.T) {
		b := fixtureBuild(t, "plain", "TSGK_FAULT_LINGER")
		cases, reqs, out, pending := prepare(b, 1)
		batches := b.runOracleBatches(ctx, x, cases, reqs, out, pending)
		if len(batches) != 1 || batches[0].Reason != runner.ReasonBatchExit || batches[0].Accepted() || out[0].Code != "BATCH_PROCESS_FAILED" {
			t.Fatalf("a driver that does not exit after its last frame was accepted: %+v %s", batches, out[0].Code)
		}
		// the private-corpus batch rejects the same process (#266, it used to accept it)
		root := t.TempDir()
		src := []byte("x = 1;\n")
		os.WriteFile(filepath.Join(root, "a.txt"), src, 0o644)
		sum := sha256.Sum256(src)
		y := testContext("private-corpus-local")
		y.Output = kit.OutputRecord
		corpus, records := runBatches(ctx, b, y, root, []kit.IncrementalCase{{ID: "a", Encoding: kit.EncodingUTF8, Input: kit.NativeInput{Path: "a.txt", Role: "case", SHA256: hex.EncodeToString(sum[:]), Bytes: uint64(len(src))}}}, nil, nil)
		if corpus[0].ExecutionStatus != kit.StatusFailed || corpus[0].Code != "BATCH_EXIT_TIMEOUT" || records[0].Fatal != "BATCH_EXIT_TIMEOUT" {
			t.Fatalf("private corpus accepted a lingering driver: %s %s %+v", corpus[0].ExecutionStatus, corpus[0].Code, records)
		}
	})
	t.Run("split", func(t *testing.T) {
		b := fixtureBuild(t, "plain")
		cases, reqs, out, pending := prepare(b, 65)
		batches := b.runOracleBatches(ctx, x, cases, reqs, out, pending)
		if len(batches) != 2 || len(batches[0].Cases) != 64 || len(batches[1].Cases) != 1 || !batches[0].Accepted() || !batches[1].Accepted() {
			t.Fatalf("65 cases: %d batches", len(batches))
		}
		for i, c := range out {
			if c.ExecutionStatus != kit.StatusCompleted || c.Transport == nil || c.Transport.Batch != i/64 || c.Transport.Frame != i%64 || c.Transport.State != "ANSWERED" {
				t.Fatalf("case %d: %s %s %+v", i, c.ExecutionStatus, c.Code, c.Transport)
			}
		}
	})
	t.Run("leak", func(t *testing.T) {
		b := fixtureBuild(t, "plain", "TSGK_FAULT_LEAK")
		cases, reqs, out, pending := prepare(b, 2)
		batches := b.runOracleBatches(ctx, x, cases, reqs, out, pending)
		if len(batches) != 1 || batches[0].ExitCode != 5 || batches[0].Accepted() {
			t.Fatalf("batches %+v", batches)
		}
		if out[0].Code != "BATCH_PROCESS_FAILED" || out[0].Transport.State != "ANSWERED" || out[1].Code != "BATCH_FRAME_FAILED" || out[1].Transport.State != "SENT" {
			t.Fatalf("%s %+v, %s %+v", out[0].Code, out[0].Transport, out[1].Code, out[1].Transport)
		}
	})
}

// #266: a sent frame without a complete response keeps the cause that ended it.
func TestUnansweredFrame(t *testing.T) {
	for _, c := range []struct {
		fr   runner.FrameResult
		want [3]string
	}{
		{runner.FrameResult{Status: runner.FrameResourceLimit, Detail: runner.ReasonFrameWatchdog}, [3]string{kit.StatusResourceLimit, kit.AssessBlocked, runner.ReasonFrameWatchdog}},
		{runner.FrameResult{Status: runner.FrameResourceLimit, Detail: runner.ReasonResponseBytes}, [3]string{kit.StatusResourceLimit, kit.AssessBlocked, runner.ReasonResponseBytes}},
		{runner.FrameResult{Status: runner.FrameResourceLimit, Detail: runner.ReasonMemory}, [3]string{kit.StatusResourceLimit, kit.AssessBlocked, runner.ReasonMemory}},
		{runner.FrameResult{Status: runner.FrameRequeued, Detail: runner.ReasonWall}, [3]string{kit.StatusResourceLimit, kit.AssessBlocked, "BATCH_WALL_LIMIT"}},
		{runner.FrameResult{Status: runner.FrameRequeued, Detail: runner.ReasonOutput}, [3]string{kit.StatusResourceLimit, kit.AssessBlocked, "BATCH_OUTPUT_LIMIT"}},
		{runner.FrameResult{Status: runner.FrameRequeued, Detail: runner.ReasonCancelled}, [3]string{kit.StatusCancelled, kit.AssessNotAssessed, "CANCELLED"}},
		{runner.FrameResult{Status: runner.FrameFailed, Detail: runner.ReasonExited}, [3]string{kit.StatusFailed, kit.AssessNotAssessed, "BATCH_FRAME_FAILED"}},
	} {
		if s, a, code := unansweredFrame(c.fr); [3]string{s, a, code} != c.want {
			t.Errorf("%+v: %s %s %s", c.fr, s, a, code)
		}
	}
}
