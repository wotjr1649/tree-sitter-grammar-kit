package native

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

var batchTiming = regexp.MustCompile(`"[a-z_]*_ms":[0-9]+`)

// runDriverBatch sends the requests to one batch process and returns its response frames.
func runDriverBatch(t *testing.T, b *Build, op kit.NativeOperation, reqs []Request) ([][]byte, runner.Result) {
	t.Helper()
	var stdin bytes.Buffer
	for _, r := range reqs {
		stdin.Write(Frame(r.Encode()))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := runner.Run(ctx, runner.Spec{Path: b.Executable, Args: []string{"batch"}, Dir: b.Dir, Stdin: stdin.Bytes(), StdoutBytes: 1 << 28, StderrBytes: 65536,
		Wall: time.Minute, Grace: 5 * time.Second, Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: runtime.GOOS != "darwin"}, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")})
	if err != nil {
		t.Fatal(err)
	}
	var frames [][]byte
	for out := res.Stdout; len(out) > 0; {
		if len(out) < 4 || uint64(len(out)-4) < uint64(binary.BigEndian.Uint32(out)) {
			t.Fatalf("truncated batch stdout after %d frames", len(frames))
		}
		n := binary.BigEndian.Uint32(out)
		frames, out = append(frames, out[4:4+n]), out[4+n:]
	}
	return frames, res
}

// #266: a batch process compiles a query list once and reuses it only for a frame with the
// same ids and source bytes; every frame answers exactly as its own single process would,
// including invalid queries, list changes and an allocation limit with the list cached.
func TestBatchQueryCache(t *testing.T) {
	t.Parallel()
	src := []byte("a = f(1);\nb = a;\n")
	x := oracleContext(true, "(identifier) @x", "(number) @n")
	lists := map[string][]QuerySource{
		"A":        x.Queries,
		"byte":     {{ID: "q0", Source: []byte("(identifier) @y")}, x.Queries[1]},
		"id":       {{ID: "qz", Source: x.Queries[0].Source}, x.Queries[1]},
		"invalid":  {x.Queries[0], {ID: "q1", Source: []byte("(assignment")}},
		"count":    x.Queries[:1],
		"no-query": nil,
	}
	var reqs []Request
	for i, name := range []string{"A", "A", "byte", "A", "id", "invalid", "A", "count", "A", "no-query", "A"} {
		y := x
		y.Queries = lists[name]
		reqs = append(reqs, y.request(kit.IncrementalCase{ID: fmt.Sprintf("c%d-%s", i, name), Encoding: kit.EncodingUTF8}, src))
	}
	small := x.request(kit.IncrementalCase{ID: "small-memory", Encoding: kit.EncodingUTF8}, src)
	small.Limits.MemoryBytes = 4096 // fatal ALLOCATION_LIMIT: the last frame of the batch
	reqs = append(reqs, small)
	for _, build := range []struct {
		defines []string
		differ  []int // frames a broken cache answers differently
	}{{nil, nil}, {[]string{"TSGK_FAULT_CACHE_ANY"}, []int{2, 5}}} {
		b := fixtureBuild(t, "plain", build.defines...)
		frames, res := runDriverBatch(t, b, x.Op, reqs)
		if len(frames) != len(reqs) || res.ExitCode != 3 || !res.Cleanup.Verified || !strings.Contains(string(frames[len(frames)-1]), `"code":"ALLOCATION_LIMIT"`) {
			t.Fatalf("%v: %d frames for %d requests, exit %d, cleanup %v", build.defines, len(frames), len(reqs), res.ExitCode, res.Cleanup.Verified)
		}
		var differ []int
		for i, r := range reqs {
			if !bytes.Equal(normalFrame(frames[i]), singleFrame(t, b, x.Op, r)) {
				differ = append(differ, i)
			}
		}
		if fmt.Sprint(differ) != fmt.Sprint(build.differ) {
			t.Errorf("%v: frames differing from their single process %v, want %v", build.defines, differ, build.differ)
		}
	}
}

func normalFrame(f []byte) []byte { return batchTiming.ReplaceAll(f, []byte(`"ms":0`)) }

// singleFrame runs one request in its own single process and returns its normalised response.
func singleFrame(t *testing.T, b *Build, op kit.NativeOperation, r Request) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	res, err := b.Exec(ctx, "single", Frame(r.Encode()), op.ParseWall, op, os.Getenv("TSGK_CGROUP_PARENT"))
	if err != nil || !res.Cleanup.Verified || res.Reason != runner.ReasonExited {
		t.Fatalf("single %s: %v %s cleanup %v", r.ID, err, res.Reason, res.Cleanup.Verified)
	}
	body, err := SplitFrame(res.Stdout)
	if err != nil {
		t.Fatalf("single %s: %v", r.ID, err)
	}
	return normalFrame(body)
}

// #266: a request with another query list frees the kept list before its own memory limit
// applies, so a limit its single process fits is not exceeded by the previous list.
func TestBatchQueryCacheMissFreesFirst(t *testing.T) {
	t.Parallel()
	b := fixtureBuild(t, "plain")
	src := []byte("a = f(1);\nb = a;\n")
	big := oracleContext(false)
	for i := range 4 {
		big.Queries = append(big.Queries, QuerySource{ID: fmt.Sprintf("big%d", i), Source: []byte(strings.Repeat("(identifier) @x\n", 2000))})
	}
	bare := oracleContext(false)
	limited := func(x Context, limit uint64) Request {
		r := x.request(kit.IncrementalCase{ID: "limited", Encoding: kit.EncodingUTF8}, src)
		r.Limits.MemoryBytes = limit
		return r
	}
	completed := []byte(`"status":"COMPLETED"`)
	limit := uint64(65536)
	for !bytes.Contains(singleFrame(t, b, bare.Op, limited(bare, limit)), completed) {
		if limit *= 2; limit > 1<<26 {
			t.Fatal("no limit fits the query-free request")
		}
	}
	if bytes.Contains(singleFrame(t, b, big.Op, limited(big, limit)), completed) {
		t.Fatalf("the big list fits %d bytes: it cannot show a kept list counted against the next limit", limit)
	}
	next := limited(bare, limit)
	frames, res := runDriverBatch(t, b, big.Op, []Request{big.request(kit.IncrementalCase{ID: "big", Encoding: kit.EncodingUTF8}, src), next})
	if len(frames) != 2 || res.ExitCode != 0 || !res.Cleanup.Verified || !bytes.Equal(normalFrame(frames[1]), singleFrame(t, b, bare.Op, next)) {
		t.Fatalf("after a kept list: %d frames, exit %d, limit %d, frame %.200s", len(frames), res.ExitCode, limit, frames[len(frames)-1])
	}
}

// #266: a kept list serves r4 requests with their own source, edit history, API switch and
// observation run, each answering as its own single process, and the batch ends cleanly.
func TestBatchQueryCacheHistory(t *testing.T) {
	t.Parallel()
	b := fixtureBuild(t, "plain")
	x := oracleContext(true, "(call function: (identifier) @fn)\n(number) @n\n")
	x.Protocol, x.ObservationProducerSHA256 = ProtocolR4, b.Identity
	var reqs []Request
	for i, c := range []struct {
		src, run string
		api      bool
		edits    []edit
	}{
		{"z = [1, 2];\na = f(1);\n", "run-a", true, []edit{{"f(1)", "g(1, 2)"}, {"[1, 2]", "[1]"}}},
		{"b = h(3);\n", "run-b", false, []edit{{"h(3)", "k(3, 4)"}}},
		{"z = [1, 2];\na = f(1);\n", "run-a", true, []edit{{"[1, 2]", "[]"}}},
	} {
		y := x
		y.API, y.ObservationRunID = c.api, c.run
		reqs = append(reqs, y.request(kit.IncrementalCase{ID: fmt.Sprintf("h%d", i), Encoding: kit.EncodingUTF8, Edits: editsBy(c.src, c.edits...)}, []byte(c.src)))
	}
	frames, res := runDriverBatch(t, b, x.Op, reqs)
	if len(frames) != len(reqs) || res.ExitCode != 0 || !res.Cleanup.Verified {
		t.Fatalf("%d frames for %d requests, exit %d, cleanup %v", len(frames), len(reqs), res.ExitCode, res.Cleanup.Verified)
	}
	for i, r := range reqs {
		if !bytes.Equal(normalFrame(frames[i]), singleFrame(t, b, x.Op, r)) {
			t.Errorf("frame %d differs from its single process", i)
		}
	}
}

// #266: a frame that leaves an allocation behind ends the batch with exit 5 and no further
// frame, and no answered frame of that process is accepted.
func TestBatchStateLeak(t *testing.T) {
	t.Parallel()
	b := fixtureBuild(t, "plain", "TSGK_FAULT_LEAK")
	x := oracleContext(false)
	src := []byte("x = 1;\n")
	frames, res := runDriverBatch(t, b, x.Op, []Request{x.request(kit.IncrementalCase{ID: "a", Encoding: kit.EncodingUTF8}, src), x.request(kit.IncrementalCase{ID: "b", Encoding: kit.EncodingUTF8}, src)})
	if len(frames) != 1 || res.ExitCode != 5 || !strings.Contains(string(res.Stderr), "STATE_LEAK") {
		t.Fatalf("leak: %d frames, exit %d, stderr %q", len(frames), res.ExitCode, res.Stderr)
	}
	root := t.TempDir()
	var cases []kit.IncrementalCase
	for _, id := range []string{"a", "b"} {
		os.WriteFile(filepath.Join(root, id+".txt"), src, 0o644)
		s := sha256.Sum256(src)
		cases = append(cases, kit.IncrementalCase{ID: id, Encoding: kit.EncodingUTF8, Input: kit.NativeInput{Path: id + ".txt", Role: "case", SHA256: hex.EncodeToString(s[:]), Bytes: uint64(len(src))}})
	}
	y := testContext("private-corpus-local")
	y.Output = kit.OutputRecord
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, batches := runBatches(ctx, b, y, root, cases, nil, nil)
	// a: answered before the leak was seen; b: sent to a process that ended without answering
	if out[0].ExecutionStatus != kit.StatusFailed || out[0].Code != "BATCH_EXIT_NONZERO" || out[1].ExecutionStatus != kit.StatusFailed || out[1].Code != "BATCH_FRAME_FAILED" {
		t.Errorf("leaking batch accepted: %s %s, %s %s", out[0].ExecutionStatus, out[0].Code, out[1].ExecutionStatus, out[1].Code)
	}
	if len(batches) != 1 || batches[0].Process.ExitCode != 5 || batches[0].Fatal != "BATCH_EXIT_NONZERO" {
		t.Errorf("batches %+v", batches)
	}
}
