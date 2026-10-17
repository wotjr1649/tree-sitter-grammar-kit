package largeaudit

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const SessionSchema = "tsgk-large-api-session-evaluation/r1"
const SessionPageSchema = "tsgk-large-api-session-page/r1"

// Fixed scalar receipt fields, three hashes and the driver's 128-byte ID fit
// within this bound even with JSON escaping and the session envelope.
const SessionPageLimit = 4096
const SessionPages = 25

type SessionPage struct {
	Schema          string          `json:"schema"`
	Session         string          `json:"session"`
	Index           uint64          `json:"index"`
	Parses          uint64          `json:"parse_count"`
	SessionComplete bool            `json:"session_complete"`
	Receipt         json.RawMessage `json:"receipt"`
	Complete        bool            `json:"complete"`
}

// Session keeps whole-process evidence once and the actual response payloads separately.
type Session struct {
	Schema       string             `json:"schema"`
	ID           string             `json:"id"`
	Session      string             `json:"session"`
	RequestSHA   string             `json:"request_sha256"`
	Nodes        uint64             `json:"nodes"`
	PageSize     uint64             `json:"page_size"`
	BudgetMillis int64              `json:"budget_ms"`
	Batch        runner.BatchResult `json:"batch"`
	RawPages     [][]byte           `json:"raw_pages"`
	Complete     bool               `json:"complete"`
}

func (s *Session) UnmarshalJSON(data []byte) error {
	if len(data) > 1<<20 {
		return errors.New("session record exceeds limit")
	}
	object, err := required(data, "schema", "id", "session", "request_sha256", "nodes", "page_size", "budget_ms", "batch", "raw_pages", "complete")
	if err != nil {
		return err
	}
	var pages []json.RawMessage
	if err := jsonv2.Unmarshal(object["raw_pages"], &pages); err != nil || len(pages) > SessionPages {
		return errors.New("session page list invalid")
	}
	for _, page := range pages {
		if bytes.Equal(bytes.TrimSpace(page), []byte("null")) || len(page) > 2+4*((SessionPageLimit+2)/3) {
			return errors.New("session encoded page exceeds limit")
		}
	}
	batch, err := required(object["batch"], "frames", "process", "trailing_bytes")
	if err != nil {
		return err
	}
	var frames []json.RawMessage
	if err := jsonv2.Unmarshal(batch["frames"], &frames); err != nil || len(frames) > SessionPages {
		return errors.New("session frame list invalid")
	}
	for _, frame := range frames {
		if _, err := required(frame, "id", "status", "wall_ms"); err != nil {
			return err
		}
	}
	process, err := required(batch["process"], "status", "reason", "exit_code", "stdout_bytes", "stderr_bytes", "stdout_truncated", "stderr_truncated", "wall_ms", "cleanup", "memory")
	if err != nil {
		return err
	}
	if _, err := required(process["cleanup"], "verified", "residual_after_exit"); err != nil {
		return err
	}
	if _, err := required(process["memory"], "limit_bytes", "enforcement", "peak_bytes", "peak_known", "limit_hit"); err != nil {
		return err
	}
	type plain Session
	var value plain
	if err := jsonv2.Unmarshal(data, &value, jsonv2.RejectUnknownMembers(true)); err != nil {
		return err
	}
	for _, page := range value.RawPages {
		if len(page) > SessionPageLimit {
			return errors.New("session decoded page exceeds limit")
		}
	}
	*s = Session(value)
	return nil
}

// CheckSession compares every actual payload with an independently executed legacy page.
func CheckSession(s Session, req native.Request, expected [][]byte) error {
	op := kit.NativeOperations()["native-query-large"]
	if s.Schema != SessionSchema || s.ID != req.ID || len(s.Session) != 32 || s.RequestSHA != digest(native.Frame(req.Encode())) ||
		!s.Complete || s.Nodes == 0 || s.Nodes > req.Limits.Nodes || s.PageSize == 0 || s.PageSize > 1000000 {
		return errors.New("session identity or completion invalid")
	}
	for _, c := range s.Session {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return errors.New("session identifier invalid")
		}
	}
	count64 := 1 + (s.Nodes-1)/s.PageSize
	if count64 > SessionPages {
		return errors.New("session page count exceeds limit")
	}
	count := int(count64)
	if len(s.RawPages) != count || len(expected) != count || len(s.Batch.Frames) != count || s.Batch.TrailingBytes != 0 ||
		s.BudgetMillis <= 0 || s.BudgetMillis > int64(count)*op.ParseWall.Milliseconds() {
		return errors.New("session page count or budget invalid")
	}
	p := s.Batch.Process
	if !p.Succeeded() || p.Reason != runner.ReasonExited || !p.Cleanup.Verified || p.Cleanup.ResidualAfterExit != 0 ||
		p.StdoutTruncated || p.StderrTruncated || p.WallMillis < 0 || p.WallMillis > s.BudgetMillis || p.StderrBytes < 0 || p.StderrBytes > 65536 ||
		p.Memory.Enforcement != runner.MemoryHard || p.Memory.Limit != op.MemoryBytes || !p.Memory.PeakKnown || p.Memory.LimitHit || p.Memory.Peak > p.Memory.Limit {
		return errors.New("session process incomplete or outside limits")
	}
	var stdout int64
	var wall int64
	var children uint64
	for i, payload := range s.RawPages {
		frame := s.Batch.Frames[i]
		if frame.ID != fmt.Sprintf("page-%d", i) || frame.Status != runner.FrameCompleted || frame.Detail != "" || frame.WallMillis < 0 || frame.WallMillis > op.ParseWall.Milliseconds() || len(payload) == 0 || len(payload) > SessionPageLimit {
			return errors.New("session frame incomplete or outside limits")
		}
		if _, err := required(payload, "schema", "session", "index", "parse_count", "session_complete", "receipt", "complete"); err != nil {
			return err
		}
		var page SessionPage
		if err := jsonv2.Unmarshal(payload, &page, jsonv2.RejectUnknownMembers(true)); err != nil {
			return err
		}
		if page.Schema != SessionPageSchema || page.Session != s.Session || page.Index != uint64(i) || page.Parses != 1 || !page.Complete || page.SessionComplete != (i == count-1) || !bytes.Equal(page.Receipt, expected[i]) {
			return errors.New("session page binding or legacy payload differs")
		}
		start := uint64(i) * s.PageSize
		r, err := Decode(page.Receipt, req, start, min(start+s.PageSize, s.Nodes), s.Nodes)
		if err != nil || r.Differences != 0 || r.Divergences != 0 {
			return errors.New("session audit differs or is incomplete")
		}
		children += r.ChildFields
		stdout += int64(4 + len(payload))
		wall += frame.WallMillis
	}
	if children != s.Nodes-1 || stdout != p.StdoutBytes || stdout > int64(op.OutputBytes) || wall > p.WallMillis {
		return errors.New("session cumulative evidence invalid")
	}
	return nil
}
