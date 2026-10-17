package largeaudit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func sessionFixture(t *testing.T) (Session, native.Request, [][]byte) {
	t.Helper()
	req := native.Request{ID: "session-unit", Encoding: kit.EncodingUTF8, Output: kit.OutputAuto, Source: []byte("a"), Limits: native.LimitsFor(kit.NativeOperations()["native-query-large"])}
	s := Session{Schema: SessionSchema, ID: req.ID, Session: strings.Repeat("a", 32), RequestSHA: digest(native.Frame(req.Encode())), Nodes: 2, PageSize: 1, BudgetMillis: 180000, Complete: true}
	s.Batch.Process = runner.Result{Status: runner.StatusCompleted, Reason: runner.ReasonExited, WallMillis: 10, Cleanup: runner.Cleanup{Verified: true}, Memory: runner.MemoryObservation{Limit: req.Limits.MemoryBytes, Enforcement: runner.MemoryHard, PeakKnown: true, Peak: 100}}
	var expected [][]byte
	for i := 0; i < 2; i++ {
		children := uint64(1 - i)
		r := Receipt{Schema: "tsgk-large-api-audit/r1", ID: req.ID, SourceSHA: digest(req.Source), SourceBytes: 1, Nodes: 2, Start: uint64(i), End: uint64(i + 1), Depth: 2, Fields: 1, Checks: 13 + children + 2 + 6, ChildFields: children, FieldLookups: 2, PointChecks: 6, First: []int64{0, 0, 0, 0}, FirstDivergence: []int64{0, 0, 0, 0}, ExpectedSHA: strings.Repeat("b", 64), ObservedSHA: strings.Repeat("b", 64), Complete: true}
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		expected = append(expected, data)
		payload, err := json.Marshal(SessionPage{Schema: SessionPageSchema, Session: s.Session, Index: uint64(i), Parses: 1, SessionComplete: i == 1, Receipt: data, Complete: true})
		if err != nil {
			t.Fatal(err)
		}
		s.RawPages = append(s.RawPages, payload)
		s.Batch.Frames = append(s.Batch.Frames, runner.FrameResult{ID: fmt.Sprintf("page-%d", i), Status: runner.FrameCompleted, WallMillis: 1})
		s.Batch.Process.StdoutBytes += int64(4 + len(payload))
	}
	return s, req, expected
}

func TestSessionOfflineBinding(t *testing.T) {
	good, req, expected := sessionFixture(t)
	if err := CheckSession(good, req, expected); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(good)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*Session){
		"missing-page":                func(s *Session) { s.RawPages = s.RawPages[:1] },
		"reorder":                     func(s *Session) { s.RawPages[0], s.RawPages[1] = s.RawPages[1], s.RawPages[0] },
		"session-id":                  func(s *Session) { s.Session = strings.Repeat("b", 32) },
		"request":                     func(s *Session) { s.RequestSHA = strings.Repeat("b", 64) },
		"nodes":                       func(s *Session) { s.Nodes++ },
		"incomplete":                  func(s *Session) { s.Complete = false },
		"bad-exit-after-last-receipt": func(s *Session) { s.Batch.Process.Status = runner.StatusFailed; s.Batch.Process.ExitCode = 2 },
		"wall":                        func(s *Session) { s.Batch.Process.WallMillis = s.BudgetMillis + 1 },
		"frame-wall":                  func(s *Session) { s.Batch.Frames[0].WallMillis = 90001 },
		"sum-wall":                    func(s *Session) { s.Batch.Frames[0].WallMillis = 20 },
		"cleanup":                     func(s *Session) { s.Batch.Process.Cleanup.Verified = false },
		"residual":                    func(s *Session) { s.Batch.Process.Cleanup.ResidualAfterExit = 1 },
		"memory":                      func(s *Session) { s.Batch.Process.Memory.LimitHit = true },
		"peak":                        func(s *Session) { s.Batch.Process.Memory.Peak = s.Batch.Process.Memory.Limit + 1 },
		"soft-memory":                 func(s *Session) { s.Batch.Process.Memory.Enforcement = "SAMPLED" },
		"stdout-count":                func(s *Session) { s.Batch.Process.StdoutBytes++ },
		"stderr-limit":                func(s *Session) { s.Batch.Process.StderrBytes = 65537 },
		"truncated":                   func(s *Session) { s.Batch.Process.StdoutTruncated = true },
		"trailing":                    func(s *Session) { s.Batch.TrailingBytes = 1 },
		"skipped":                     func(s *Session) { s.Batch.Frames[1].Status = runner.FrameRequeued },
		"wrong-frame-id":              func(s *Session) { s.Batch.Frames[1].ID = "page-0" },
	}
	for name, change := range mutations {
		t.Run(name, func(t *testing.T) {
			var bad Session
			if err := json.Unmarshal(data, &bad); err != nil {
				t.Fatal(err)
			}
			change(&bad)
			if _, err := ValidateSession(bad, req, good.Nodes, 1); err == nil {
				t.Fatal("invalid common session accepted")
			}
			if CheckSession(bad, req, expected) == nil {
				t.Fatal("invalid session accepted")
			}
		})
	}
	for name, change := range map[string]func(*SessionPage){
		"parse-count": func(p *SessionPage) { p.Parses = 2 }, "index": func(p *SessionPage) { p.Index = 1 },
		"foreign-session": func(p *SessionPage) { p.Session = strings.Repeat("b", 32) },
		"early-complete":  func(p *SessionPage) { p.SessionComplete = true },
		"incomplete-page": func(p *SessionPage) { p.Complete = false },
	} {
		t.Run(name, func(t *testing.T) {
			var bad Session
			if err := json.Unmarshal(data, &bad); err != nil {
				t.Fatal(err)
			}
			var page SessionPage
			if err := json.Unmarshal(bad.RawPages[0], &page); err != nil {
				t.Fatal(err)
			}
			change(&page)
			payload, err := json.Marshal(page)
			if err != nil {
				t.Fatal(err)
			}
			bad.Batch.Process.StdoutBytes += int64(len(payload) - len(bad.RawPages[0]))
			bad.RawPages[0] = payload
			if _, err := ValidateSession(bad, req, good.Nodes, 1); err == nil {
				t.Fatal("invalid common page accepted")
			}
			if CheckSession(bad, req, expected) == nil {
				t.Fatal("invalid page accepted")
			}
		})
	}
	expectedCopy := append([][]byte{}, expected...)
	expectedCopy[0] = append([]byte(" "), expectedCopy[0]...)
	if CheckSession(good, req, expectedCopy) == nil {
		t.Fatal("different expected payload accepted")
	}

	for name, bad := range map[string][]byte{
		"missing-zero":    bytes.Replace(data, []byte(`"trailing_bytes":0`), []byte(`"unused":0`), 1),
		"null-zero":       bytes.Replace(data, []byte(`"trailing_bytes":0`), []byte(`"trailing_bytes":null`), 1),
		"null-memory-hit": bytes.Replace(data, []byte(`"limit_hit":false`), []byte(`"limit_hit":null`), 1),
		"missing-wall":    bytes.Replace(data, []byte(`"wall_ms":1}`), []byte(`"detail":""}`), 1),
		"duplicate":       bytes.Replace(data, []byte(`"complete":true`), []byte(`"complete":true,"complete":true`), 1),
		"unknown":         append(append([]byte{}, data[:len(data)-1]...), []byte(`,"unknown":0}`)...),
		"truncated":       data[:len(data)-1],
	} {
		t.Run(name, func(t *testing.T) {
			if bytes.Equal(data, bad) {
				t.Fatal("mutation did not apply")
			}
			var badRecord Session
			if json.Unmarshal(bad, &badRecord) == nil {
				t.Fatal("malformed session accepted")
			}
		})
	}
}

func TestSessionDecodeBounds(t *testing.T) {
	s, _, _ := sessionFixture(t)
	// Valid payload plus whitespace isolates the pre-decode size guard.
	s.RawPages[0] = append(s.RawPages[0], bytes.Repeat([]byte(" "), SessionPageLimit+1-len(s.RawPages[0]))...)
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var out Session
	if json.Unmarshal(data, &out) == nil {
		t.Fatal("oversized encoded payload accepted")
	}
	good, _, _ := sessionFixture(t)
	data, err = json.Marshal(good)
	if err != nil {
		t.Fatal(err)
	}
	data = append(append(append([]byte{}, data[:len(data)-1]...), bytes.Repeat([]byte(" "), (1<<20)+1-len(data))...), byte('}'))
	if json.Unmarshal(data, &out) == nil {
		t.Fatal("oversized session object accepted")
	}
}

func TestSessionPageCountOverflow(t *testing.T) {
	s, req, _ := sessionFixture(t)
	s.Nodes = math.MaxUint64
	s.PageSize = 2
	req.Limits.Nodes = math.MaxUint64
	s.RequestSHA = digest(native.Frame(req.Encode()))
	if _, err := ValidateSession(s, req, math.MaxUint64, 1); err == nil {
		t.Fatal("overflowed page count accepted")
	}
}

func TestSessionIndependentMetadata(t *testing.T) {
	s, req, legacy := sessionFixture(t)
	payloads, err := ValidateSession(s, req, 2, 1)
	if err != nil || len(payloads) != 2 {
		t.Fatalf("common session: %v", err)
	}
	if _, err := ValidateSession(s, req, 3, 1); err == nil {
		t.Fatal("node baseline mismatch accepted")
	}
	if _, err := ValidateSession(s, req, 2, 2); err == nil {
		t.Fatal("field baseline mismatch accepted")
	}
	if CheckSession(s, req, nil) == nil {
		t.Fatal("evaluation silently skipped legacy comparison")
	}
	var receipt Receipt
	if err := json.Unmarshal(legacy[0], &receipt); err != nil {
		t.Fatal(err)
	}
	receipt.ExpectedSHA = strings.Repeat("c", 64)
	receipt.ObservedSHA = receipt.ExpectedSHA
	legacy[0], err = json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(legacy[0], req, 0, 1, 2); err != nil {
		t.Fatalf("legacy mismatch control malformed: %v", err)
	}
	if CheckSession(s, req, legacy) == nil {
		t.Fatal("evaluation accepted valid but different independent legacy result")
	}
}
