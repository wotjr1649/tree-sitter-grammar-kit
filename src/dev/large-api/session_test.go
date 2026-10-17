package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
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
	s, err := runSession(ctx, exe, digest(binaryBytes), b.Dir, req, nodes, pageSize, expected)
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
