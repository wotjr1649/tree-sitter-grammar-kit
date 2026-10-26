package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/largeaudit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func testNativeSession(t *testing.T, ctx context.Context, b *native.Build, cc string, req native.Request, nodes uint64) {
	t.Helper()
	const pageSize = 4
	if nodes <= pageSize || nodes > 100 {
		t.Fatalf("multi-page fixture node count: %d", nodes)
	}
	exe := filepath.Join(b.Dir, "api-audit.exe")
	binaryBytes, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	var expected [][]byte
	for start := uint64(0); start < nodes; start += pageSize {
		obs := run(ctx, exe, digest(binaryBytes), b.Dir, req, start, min(start+pageSize, nodes), nodes)
		if obs.Error != "" {
			t.Fatal(obs.Error)
		}
		payload, err := native.SplitFrame(obs.Raw)
		if err != nil {
			t.Fatal(err)
		}
		expected = append(expected, payload)
	}
	source, err := os.ReadFile("session-audit.c")
	if err != nil {
		t.Fatal(err)
	}
	if err = writeExclusive(filepath.Join(b.Dir, "session-audit.c"), source); err != nil {
		t.Fatal(err)
	}
	exe, _, err = compile(ctx, b, cc, filepath.Join(b.Dir, "session-audit.c"), "")
	if err != nil {
		t.Fatal(err)
	}
	binaryBytes, err = os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	firstReceipt, err := largeaudit.Decode(expected[0], req, 0, pageSize, nodes)
	if err != nil {
		t.Fatal(err)
	}
	s, err := runSession(ctx, exe, digest(binaryBytes), b.Dir, req, nodes, pageSize, firstReceipt.Fields)
	if err != nil {
		t.Fatalf("actual session: %v %+v", err, s.Batch)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var replay largeaudit.Session
	if err = json.Unmarshal(data, &replay); err != nil {
		t.Fatal(err)
	}
	if err = largeaudit.CheckSession(replay, req, expected); err != nil {
		t.Fatal(err)
	}
	t.Logf("actual session: nodes=%d pages=%d process_wall_ms=%d parse_count=1; all legacy payloads identical", nodes, len(s.RawPages), s.Batch.Process.WallMillis)
	t.Run("session/single-page", func(t *testing.T) {
		legacy := run(ctx, filepath.Join(b.Dir, "api-audit.exe"), digest(mustSessionBinary(t, filepath.Join(b.Dir, "api-audit.exe"))), b.Dir, req, 0, nodes, nodes)
		payload, err := native.SplitFrame(legacy.Raw)
		if legacy.Error != "" || err != nil {
			t.Fatalf("single-page legacy: %s %v", legacy.Error, err)
		}
		single, err := runSession(ctx, exe, digest(binaryBytes), b.Dir, req, nodes, nodes, firstReceipt.Fields)
		if err != nil || largeaudit.CheckSession(single, req, [][]byte{payload}) != nil {
			t.Fatalf("single-page session: %v", err)
		}
	})
	for _, delayed := range []struct {
		name, anchor string
		wantComplete bool
	}{
		{"bulk-cleanup-delay", "      idmap_free(&audit.map);", true},
		{"post-frame-delay", "    buf_free(&out);", false},
	} {
		t.Run("session/"+delayed.name, func(t *testing.T) {
			if strings.Count(string(source), delayed.anchor) != 1 {
				t.Fatal("delay anchor missing or ambiguous")
			}
			path := filepath.Join(b.Dir, "session-"+delayed.name+".c")
			modified := strings.Replace(string(source), delayed.anchor, "    if (end == expected_nodes) Sleep(6000);\n"+delayed.anchor, 1)
			if err := writeExclusive(path, []byte(modified)); err != nil {
				t.Fatal(err)
			}
			variant, _, err := compile(ctx, b, cc, path, "")
			if err != nil {
				t.Fatal(err)
			}
			result, err := runSession(ctx, variant, digest(mustSessionBinary(t, variant)), b.Dir, req, nodes, pageSize, firstReceipt.Fields)
			if delayed.wantComplete {
				if err != nil || largeaudit.CheckSession(result, req, expected) != nil {
					t.Fatalf("bulk cleanup escaped frame deadline: %v", err)
				}
			} else if err == nil || result.Complete || result.Batch.Process.Reason != runner.ReasonBatchExit || !result.Batch.Process.Cleanup.Verified || result.Batch.Process.Cleanup.ResidualAfterExit != 0 {
				t.Fatalf("post-frame exit guard weakened: %v %+v", err, result.Batch.Process)
			}
		})
	}
	t.Run("session/interactive-extra-after-final", func(t *testing.T) {
		op := kit.NativeOperations()["native-query-large"]
		p, err := runner.Start(ctx, runner.Spec{Path: exe, Dir: b.Dir, Args: []string{strings.Repeat("0", 32), strconv.FormatUint(nodes, 10), strconv.FormatUint(nodes, 10)}, Interactive: true, StdoutBytes: 1 << 20, StderrBytes: 65536, Wall: 15 * time.Second, Grace: time.Second, Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: true}})
		if err != nil {
			t.Fatal(err)
		}
		done := false
		defer func() {
			if !done {
				p.Terminate(runner.ReasonCancelled)
				p.Wait()
			}
		}()
		if _, err := p.Stdin().Write(native.Frame(req.Encode())); err != nil {
			t.Fatal(err)
		}
		var header [4]byte
		if _, err := io.ReadFull(p.Stdout(), header[:]); err != nil {
			t.Fatal(err)
		}
		n := binary.BigEndian.Uint32(header[:])
		if n == 0 || n > largeaudit.SessionPageLimit {
			t.Fatal("invalid final frame size")
		}
		if _, err := io.ReadFull(p.Stdout(), make([]byte, n)); err != nil {
			t.Fatal(err)
		}
		if _, err := p.Stdin().Write([]byte{0}); err != nil {
			t.Fatal(err)
		}
		p.Stdin().Close()
		tail, readErr := io.ReadAll(p.Stdout())
		result := p.Wait()
		done = true
		if readErr != nil || len(tail) != 0 || result.ExitCode != 2 || result.Reason != runner.ReasonExited || !result.Cleanup.Verified || result.Cleanup.ResidualAfterExit != 0 {
			t.Fatalf("interactive extra input accepted: %v %+v", readErr, result)
		}
	})
	frames, err := sessionFrames(req, nodes, pageSize)
	if err != nil {
		t.Fatal(err)
	}
	var stream []byte
	for _, frame := range frames {
		stream = append(stream, native.Frame(frame.Request)...)
	}
	first := len(native.Frame(frames[0].Request))
	controls := map[string][]byte{
		"truncated-first-header": stream[:3], "truncated-first-payload": stream[:first-1],
		"early-eof": stream[:first], "truncated-command-header": stream[:first+3],
		"truncated-command-payload": stream[:first+11],
		"extra-after-last":          append(append([]byte{}, stream...), 0),
	}
	for name, command := range map[string][]byte{
		"seven-byte": make([]byte, 7), "nine-byte": make([]byte, 9),
		"duplicate": make([]byte, 8), "gap": make([]byte, 8), "empty-range": make([]byte, 8), "reversed-range": make([]byte, 8), "out-of-range": make([]byte, 8),
	} {
		switch name {
		case "gap":
			binary.BigEndian.PutUint32(command, 5)
			binary.BigEndian.PutUint32(command[4:], 9)
		case "empty-range":
			binary.BigEndian.PutUint32(command, 4)
			binary.BigEndian.PutUint32(command[4:], 4)
		case "reversed-range":
			binary.BigEndian.PutUint32(command, 8)
			binary.BigEndian.PutUint32(command[4:], 4)
		case "out-of-range":
			binary.BigEndian.PutUint32(command, 4)
			binary.BigEndian.PutUint32(command[4:], uint32(nodes+1))
		}
		controls[name] = append(append([]byte{}, stream[:first]...), native.Frame(command)...)
	}
	op := kit.NativeOperations()["native-query-large"]
	for name, input := range controls {
		t.Run("session/"+name, func(t *testing.T) {
			result, err := runner.Run(ctx, runner.Spec{Path: exe, Dir: b.Dir, Args: []string{strings.Repeat("0", 32), "4", strconv.FormatUint(nodes, 10)}, Stdin: input, StdoutBytes: 1 << 20, StderrBytes: 65536, Wall: 5 * time.Second, Grace: time.Second, Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: true}})
			if err != nil || result.ExitCode != 2 || result.Reason != runner.ReasonExited || !result.Cleanup.Verified || result.Cleanup.ResidualAfterExit != 0 {
				t.Fatalf("invalid session accepted or unsafe: %v %+v", err, result)
			}
		})
	}
	t.Run("session/late-cursor-fault", func(t *testing.T) {
		fault, _, err := compile(ctx, b, cc, filepath.Join(b.Dir, "session-audit.c"), "CURSOR")
		if err != nil {
			t.Fatal(err)
		}
		binaryBytes, err := os.ReadFile(fault)
		if err != nil {
			t.Fatal(err)
		}
		failed, err := runSession(ctx, fault, digest(binaryBytes), b.Dir, req, nodes, 1, firstReceipt.Fields)
		p := failed.Batch.Process
		if err == nil || len(failed.RawPages) != int(nodes) || p.ExitCode != 1 || p.Reason != runner.ReasonExited || !p.Cleanup.Verified || p.Cleanup.ResidualAfterExit != 0 || p.StdoutTruncated || p.StderrTruncated || p.Memory.Enforcement != runner.MemoryHard || p.Memory.LimitHit {
			t.Fatalf("later-page API fault accepted or unsafe: err=%v pages=%d process=%+v", err, len(failed.RawPages), p)
		}
		for i, raw := range failed.RawPages {
			if i >= 2 {
				if len(raw) != 0 || failed.Batch.Frames[i].Status == runner.FrameCompleted {
					t.Fatalf("page %d completed after actual API fault", i)
				}
				continue
			}
			if failed.Batch.Frames[i].Status != runner.FrameCompleted {
				t.Fatalf("actual response %d was not complete", i)
			}
			var page largeaudit.SessionPage
			if err := json.Unmarshal(raw, &page); err != nil {
				t.Fatal(err)
			}
			r, err := largeaudit.Decode(page.Receipt, req, uint64(i), uint64(i+1), nodes)
			if err != nil || r.Differences != uint64(i) || (i == 1 && (r.First[0] != 1 || r.First[1] != 11 || r.First[2] != 1 || r.First[3] != 2)) {
				t.Fatalf("cached model hid actual API fault on page %d: %v %+v", i, err, r)
			}
		}
	})
}

func mustSessionBinary(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSessionFramePlanBounded(t *testing.T) {
	req := native.Request{Limits: native.Limits{Nodes: math.MaxUint64}}
	for _, v := range [][2]uint64{{math.MaxUint64, 2}, {math.MaxUint64, 1000000}, {25000001, 1000000}, {1, 0}, {0, 1}, {1, 1000001}} {
		if _, err := sessionFrames(req, v[0], v[1]); err == nil {
			t.Fatalf("unbounded/invalid plan accepted: %v", v)
		}
	}
	frames, err := sessionFrames(req, 25000000, 1000000)
	if err != nil || len(frames) != 25 {
		t.Fatalf("bounded 25M plan: %d %v", len(frames), err)
	}
}
