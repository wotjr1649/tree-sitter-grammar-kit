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
	single := func(b *Build, r Request) []byte {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		res, err := b.Exec(ctx, "single", Frame(r.Encode()), x.Op.ParseWall, x.Op, os.Getenv("TSGK_CGROUP_PARENT"))
		if err != nil || len(res.Stdout) < 4 {
			t.Fatalf("single %s: %v %d bytes", r.ID, err, len(res.Stdout))
		}
		return batchTiming.ReplaceAll(res.Stdout[4:], []byte(`"ms":0`))
	}
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
			if !bytes.Equal(batchTiming.ReplaceAll(frames[i], []byte(`"ms":0`)), single(b, r)) {
				differ = append(differ, i)
			}
		}
		if fmt.Sprint(differ) != fmt.Sprint(build.differ) {
			t.Errorf("%v: frames differing from their single process %v, want %v", build.defines, differ, build.differ)
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
