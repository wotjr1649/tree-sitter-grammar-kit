package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/largeaudit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func sessionFrames(req native.Request, nodes, pageSize uint64) ([]runner.Frame, error) {
	if nodes == 0 || nodes > req.Limits.Nodes || pageSize == 0 || pageSize > 1000000 || 1+(nodes-1)/pageSize > largeaudit.SessionPages {
		return nil, errors.New("session page plan invalid")
	}
	frames := []runner.Frame{{ID: "page-0", Request: req.Encode()}}
	for start := pageSize; start < nodes; start += pageSize {
		command := make([]byte, 8)
		binary.BigEndian.PutUint32(command, uint32(start))
		binary.BigEndian.PutUint32(command[4:], uint32(min(start+pageSize, nodes)))
		frames = append(frames, runner.Frame{ID: fmt.Sprintf("page-%d", len(frames)), Request: command})
	}
	return frames, nil
}

func runSession(ctx context.Context, exe, executableSHA, dir string, req native.Request, nodes, pageSize uint64, expected [][]byte) (largeaudit.Session, error) {
	s := largeaudit.Session{Schema: largeaudit.SessionSchema, ID: req.ID, RequestSHA: digest(native.Frame(req.Encode())), Nodes: nodes, PageSize: pageSize, RawPages: [][]byte{}}
	frames, err := sessionFrames(req, nodes, pageSize)
	if err != nil {
		return s, err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return s, err
	}
	s.Session = hex.EncodeToString(nonce[:])
	data, err := os.ReadFile(exe)
	if err != nil || digest(data) != executableSHA {
		return s, errors.New("session executable identity mismatch")
	}
	op := kit.NativeOperations()["native-query-large"]
	wall := time.Duration(len(frames)) * op.ParseWall
	if deadline, ok := ctx.Deadline(); ok {
		wall = min(wall, time.Until(deadline)-5*time.Second)
	}
	if wall <= 0 {
		return s, errors.New("session has no remaining run budget")
	}
	s.BudgetMillis = wall.Milliseconds()
	policy := runner.BatchPolicy{RequestBytes: runner.DefaultBatchPolicy().RequestBytes, ResponseBytes: largeaudit.SessionPageLimit, FrameWall: op.ParseWall - 5*time.Second, FrameGrace: 5 * time.Second, StdoutBytes: int64(op.OutputBytes), BatchWall: wall}
	spec := runner.Spec{Path: exe, Dir: dir, Args: []string{s.Session, strconv.FormatUint(pageSize, 10), strconv.FormatUint(nodes, 10)}, StderrBytes: 65536, Grace: 5 * time.Second, Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: true}}
	s.Batch, err = runner.RunBatch(ctx, spec, policy, frames)
	for _, frame := range s.Batch.Frames {
		s.RawPages = append(s.RawPages, frame.Response)
	}
	if err != nil {
		return s, err
	}
	data, err = os.ReadFile(exe)
	if err != nil || digest(data) != executableSHA {
		return s, errors.New("session executable changed")
	}
	s.Complete = true
	if err := largeaudit.CheckSession(s, req, expected); err != nil {
		s.Complete = false
		return s, err
	}
	return s, nil
}
