package native

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// edit describes one replacement of the first occurrence of find (or an insertion
// at the end when find is empty) in the current source.
type edit struct{ find, repl string }

func editsBy(src string, ops ...edit) []kit.Edit {
	cur := src
	var out []kit.Edit
	for _, o := range ops {
		start := len(cur)
		if o.find != "" {
			start = strings.Index(cur, o.find)
			if start < 0 {
				panic("edit target not found: " + o.find)
			}
		}
		end := start + len(o.find)
		out = append(out, kit.Edit{StartByte: uint32(start), OldEndByte: uint32(end), NewEndByte: uint32(start + len(o.repl)), Old: []byte(cur[start:end]), New: []byte(o.repl)})
		cur = cur[:start] + o.repl + cur[end:]
	}
	return out
}

// rawExec sends one framed payload to the driver and decodes its single response.
func rawExec(t *testing.T, b *Build, stdin []byte, op string) (runner.Result, Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	o := kit.NativeOperations()[op]
	res, err := b.Exec(ctx, "single", stdin, 90*time.Second, o, os.Getenv("TSGK_CGROUP_PARENT"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Cleanup.Verified {
		t.Fatalf("cleanup not verified: %+v", res.Cleanup)
	}
	payload, err := SplitFrame(res.Stdout)
	if err != nil {
		return res, Response{}, err
	}
	r, err := DecodeResponse(payload)
	return res, r, err
}

func baseRequest(src string, op string) Request {
	return Request{ID: "raw", Encoding: kit.EncodingUTF8, Output: kit.OutputTree, Limits: LimitsFor(kit.NativeOperations()[op]), Source: []byte(src)}
}

// S05-A05: EOF insertion, missing zero-width nodes with duplicated spans, and insertion
// into an empty source. Order, ranges and flags are preserved without span dedup.
func TestEdgeStructures(t *testing.T) {
	b := fixtureBuild(t, "plain")
	t.Run("eof-insertion", func(t *testing.T) {
		src := "z = [1, 2];\na = 1;" // the first statement stays reusable; the edit is at EOF
		r := runCase(t, b, "eof", kit.EncodingUTF8, src, editsBy(src, edit{"", "\nb = 2;"}, edit{"", "\nc = 3;"}))
		if r.Assessment != kit.AssessPass {
			t.Fatalf("%s %s %+v", r.Assessment, r.Code, r.Claims)
		}
	})
	t.Run("missing-zero-width", func(t *testing.T) {
		src := "z = [1, 2];\na = (1;\nb = 2;\n"
		r := runCase(t, b, "missing", kit.EncodingUTF8, src, editsBy(src, edit{"b = 2", "b = 3"}, edit{"(1", "(1)"}),
			kit.StepExpectation{Step: 0, Syntax: "ERROR"}, kit.StepExpectation{Step: 2, Syntax: "NO_ERROR"})
		if r.Assessment != kit.AssessPass {
			t.Fatalf("%s %s %+v", r.Assessment, r.Code, r.Claims)
		}
		var zero []kit.TreeNode
		for _, n := range r.Steps[1].Incremental.Tree.Nodes {
			if n.StartByte == n.EndByte {
				zero = append(zero, n)
			}
		}
		if len(zero) == 0 || !zero[0].IsMissing {
			t.Fatalf("expected a zero-width MISSING node, got %+v", zero)
		}
	})
	t.Run("duplicated-span", func(t *testing.T) {
		// The root and its only statement share [0,2); both stay in preorder.
		r := runCase(t, b, "dup", kit.EncodingUTF8, "x;", nil)
		nodes := r.Steps[0].Incremental.Tree.Nodes
		same := 0
		for _, n := range nodes {
			if n.StartByte == 0 && n.EndByte == 2 {
				same++
			}
		}
		if r.Assessment != kit.AssessPass || same < 2 {
			t.Fatalf("%s %s same-span nodes %d", r.Assessment, r.Code, same)
		}
	})
	t.Run("empty-source-insertion", func(t *testing.T) {
		// With nothing to reuse the route cannot be observed: equality holds but the route
		// claim honestly fails instead of being assumed.
		r := runCase(t, b, "empty", kit.EncodingUTF8, "", editsBy("", edit{"", "a = 1;"}))
		if r.Claims.IncrementalEquality != ClaimPass || r.Claims.IncrementalRoute != ClaimFail || r.Steps[0].Incremental.DescendantCount != 1 {
			t.Fatalf("%+v %+v", r.Claims, r.Steps[0].Incremental)
		}
	})
}

func utf16(enc string, s string) []byte {
	var out []byte
	put := func(u uint16) {
		if enc == kit.EncodingUTF16LE {
			out = binary.LittleEndian.AppendUint16(out, u)
		} else {
			out = binary.BigEndian.AppendUint16(out, u)
		}
	}
	put(0xfeff)
	for _, r := range s {
		if r > 0xffff {
			r -= 0x10000
			put(uint16(0xd800 + r>>10))
			put(uint16(0xdc00 + r&0x3ff))
			continue
		}
		put(uint16(r))
	}
	return out
}

// S05-A06/A16/A17: exact transport and byte points for CRLF, BOM, NUL, non-ASCII and
// invalid UTF-8, UTF-16LE/BE and CP949 through the table decode callback; boundary
// violations are rejected by the kit and by the driver.
func TestEncodingsAndPoints(t *testing.T) {
	b := fixtureBuild(t, "plain")
	t.Run("utf8-bytes", func(t *testing.T) {
		src := "\xEF\xBB\xBFa = \"\xED\x95\x9C\";\r\nb = \"\xff\";\r\nc\x00 = 1;\r\n"
		edits := editsBy(src, edit{"\xff", "\xfe\xfd"}, edit{"\xfe\xfd", "\xff"})
		r := runCase(t, b, "utf8", kit.EncodingUTF8, src, edits, kit.StepExpectation{Step: 0, Syntax: "ANY", Contains: []string{"string"}})
		if r.Assessment != kit.AssessPass {
			t.Fatalf("%s %s %+v", r.Assessment, r.Code, r.Claims)
		}
		// row 1 starts after "\r\n"; the invalid byte sits at byte column 5 of that row.
		if r.Steps[1].Edit.StartPoint != [2]uint32{1, 5} || r.Steps[2].SourceSHA256 != r.Steps[0].SourceSHA256 {
			t.Fatalf("point %v", r.Steps[1].Edit.StartPoint)
		}
	})
	for _, enc := range []string{kit.EncodingUTF16LE, kit.EncodingUTF16BE} {
		t.Run(enc, func(t *testing.T) {
			src := utf16(enc, "a = \"한\U0001F600\";\nb = 2;\n")
			two := bytes.Index(src, utf16(enc, "2")[2:])
			e := kit.Edit{StartByte: uint32(two), OldEndByte: uint32(two + 2), NewEndByte: uint32(two + 4), Old: src[two : two+2], New: utf16(enc, "34")[2:]}
			c := kit.IncrementalCase{ID: "u16", Encoding: enc, Edits: []kit.Edit{e}, Expect: []kit.StepExpectation{{Step: 1, Syntax: "NO_ERROR", Contains: []string{"string", "number"}}}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			r := b.RunCase(ctx, testContext("native-parse-edit"), c, src)
			if r.Assessment != kit.AssessPass || r.Steps[1].Edit.StartPoint != [2]uint32{1, 8} {
				t.Fatalf("%s %s %+v %v", r.Assessment, r.Code, r.Claims, r.Steps)
			}
			odd := e
			odd.StartByte, odd.OldEndByte, odd.NewEndByte, odd.Old = e.StartByte+1, e.OldEndByte+1, e.StartByte+1+4, src[two+1:two+3]
			if _, _, err := kit.ApplyEdits(enc, src, []kit.Edit{odd}, 65536); !hasCode(err, "EDIT_ODD_UTF16") {
				t.Fatalf("odd offset: %v", err)
			}
			req := baseRequest("", "native-parse-edit")
			req.Encoding, req.Source, req.Edits = enc, src, []kit.Edit{odd}
			_, resp, err := rawExec(t, b, Frame(req.Encode()), "native-parse-edit")
			if err != nil || resp.Status != "INVALID_REQUEST" || resp.Code != "EDIT_ODD_UTF16" {
				t.Fatalf("driver odd offset: %v %s %s", err, resp.Status, resp.Code)
			}
		})
	}
	t.Run("cp949", func(t *testing.T) {
		src := []byte("a = \"\xC7\xD1\xB1\xDB\";\nb = 2;\n")
		c := kit.IncrementalCase{ID: "cp949", Encoding: kit.EncodingCP949, Edits: editsBy(string(src), edit{"2", "34"}),
			Expect: []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR", Contains: []string{"string"}}}}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		r := b.RunCase(ctx, testContext("native-parse-edit"), c, src)
		if r.Assessment != kit.AssessPass {
			t.Fatalf("%s %s %+v", r.Assessment, r.Code, r.Claims)
		}
		for _, n := range r.Steps[0].Incremental.Tree.Nodes {
			if n.Type == "string" && (n.StartByte != 4 || n.EndByte != 10 || n.EndPoint.Column != 10) {
				t.Fatalf("cp949 string range %+v", n)
			}
		}
		// A cp949 identifier: only a correct two-byte decode keeps both letters in one token.
		ident := []byte("\xB0\xA1\xB3\xAA = 1;\nb = 2;\n")
		ic := kit.IncrementalCase{ID: "cp949-ident", Encoding: kit.EncodingCP949, Edits: editsBy(string(ident), edit{"2", "3"}),
			Expect: []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR", Contains: []string{"assignment"}}}}
		ir := b.RunCase(ctx, testContext("native-parse-edit"), ic, ident)
		if ir.Assessment != kit.AssessPass || ir.Steps[0].Incremental.Tree.Nodes[2].Type != "identifier" || ir.Steps[0].Incremental.Tree.Nodes[2].EndByte != 4 {
			t.Fatalf("cp949 identifier: %s %s %+v", ir.Assessment, ir.Code, ir.Steps[0].Incremental.Tree.Nodes[:3])
		}
		split := kit.Edit{StartByte: 6, OldEndByte: 6, NewEndByte: 7, New: []byte("x")}
		if _, _, err := kit.ApplyEdits(kit.EncodingCP949, src, []kit.Edit{split}, 65536); !hasCode(err, "EDIT_SPLITS_CHARACTER") {
			t.Fatalf("cp949 split: %v", err)
		}
		req := baseRequest("", "native-parse-edit")
		req.Encoding, req.Source, req.Edits = kit.EncodingCP949, src, []kit.Edit{split}
		if _, resp, err := rawExec(t, b, Frame(req.Encode()), "native-parse-edit"); err != nil || resp.Code != "EDIT_SPLITS_CHARACTER" {
			t.Fatalf("driver cp949 split: %v %s", err, resp.Code)
		}
		req.Edits, req.Source = nil, []byte("a = \"\xFE\xA1\";")
		if _, resp, err := rawExec(t, b, Frame(req.Encode()), "native-parse-edit"); err != nil || resp.Code != "SOURCE_ENCODING_INVALID" {
			t.Fatalf("driver unmapped cp949: %v %s", err, resp.Code)
		}
	})
}

func hasCode(err error, code string) bool {
	var ke *kit.Error
	return errors.As(err, &ke) && ke.Code == code
}

// S05-A07: invalid edits are rejected before any native state exists, by the kit and
// again by the driver (which returns no step at all).
func TestEditRejections(t *testing.T) {
	b := fixtureBuild(t, "plain")
	src := "a = 1;\nb = 2;\n"
	good := editsBy(src, edit{"1", "11"})[0]
	cases := map[string]kit.Edit{
		"EDIT_RANGE":               {StartByte: 10, OldEndByte: 40, NewEndByte: 10, Old: nil},
		"EDIT_OLD_MISMATCH":        {StartByte: good.StartByte, OldEndByte: good.OldEndByte, NewEndByte: good.NewEndByte, Old: []byte("9"), New: good.New},
		"EDIT_LENGTH_INCONSISTENT": {StartByte: good.StartByte, OldEndByte: good.OldEndByte, NewEndByte: good.NewEndByte + 1, Old: good.Old, New: good.New},
		"EDIT_EMPTY":               {StartByte: 3, OldEndByte: 3, NewEndByte: 3},
	}
	inverted := good
	inverted.StartByte, inverted.OldEndByte = good.OldEndByte, good.StartByte
	cases["EDIT_RANGE/inverted"] = inverted
	for name, e := range cases {
		code, _, _ := strings.Cut(name, "/")
		t.Run(name, func(t *testing.T) {
			if _, _, err := kit.ApplyEdits(kit.EncodingUTF8, []byte(src), []kit.Edit{good, e}, 65536); !hasCode(err, code) {
				t.Fatalf("kit: %v", err)
			}
			req := baseRequest(src, "native-parse-edit")
			req.Edits = []kit.Edit{good, e}
			res, resp, err := rawExec(t, b, Frame(req.Encode()), "native-parse-edit")
			if err != nil || resp.Status != "INVALID_REQUEST" || resp.Code != code || len(resp.Steps) != 0 || res.ExitCode != 2 {
				t.Fatalf("driver: %v %s %s steps=%d exit=%d", err, resp.Status, resp.Code, len(resp.Steps), res.ExitCode)
			}
		})
	}
	t.Run("utf8-split", func(t *testing.T) {
		s := "a = \"\xED\x95\x9C\";"
		e := kit.Edit{StartByte: 6, OldEndByte: 6, NewEndByte: 7, New: []byte("x")}
		if _, _, err := kit.ApplyEdits(kit.EncodingUTF8, []byte(s), []kit.Edit{e}, 65536); !hasCode(err, "EDIT_SPLITS_CHARACTER") {
			t.Fatalf("kit: %v", err)
		}
		// next to an invalid byte the boundary is accepted (one-byte unit, never repaired)
		bad := "a = \"\xED\x95\";"
		ok := kit.Edit{StartByte: 6, OldEndByte: 6, NewEndByte: 7, New: []byte("x")}
		if _, _, err := kit.ApplyEdits(kit.EncodingUTF8, []byte(bad), []kit.Edit{ok}, 65536); err != nil {
			t.Fatalf("invalid-byte boundary: %v", err)
		}
	})
}

// S05-A09: malformed, oversized, truncated and extra request frames end in a bounded
// INVALID_REQUEST; malformed responses are never accepted by the kit.
func TestFrames(t *testing.T) {
	b := fixtureBuild(t, "plain")
	valid := baseRequest("a = 1;", "native-parse-edit").Encode()
	big := make([]byte, 4)
	binary.BigEndian.PutUint32(big, MaxRequestFrame+1)
	for name, tc := range map[string]struct {
		stdin []byte
		code  string
	}{
		"short-header":     {[]byte{0, 0}, "FRAME_TRUNCATED"},
		"short-payload":    {Frame(valid)[:20], "FRAME_TRUNCATED"},
		"too-large":        {big, "FRAME_TOO_LARGE"},
		"extra-frame":      {append(Frame(valid), Frame(valid)...), "FRAME_EXTRA"},
		"extra-byte":       {append(Frame(valid), 'x'), "FRAME_EXTRA"},
		"malformed":        {Frame([]byte(`{"protocol":"tsgk-native/r1","id":"x"}`)), "REQUEST_MALFORMED"},
		"whitespace":       {Frame(bytes.Replace(valid, []byte(`,"id"`), []byte(`, "id"`), 1)), "REQUEST_MALFORMED"},
		"protocol":         {Frame(bytes.Replace(valid, []byte("tsgk-native/r1"), []byte("tsgk-native/r3"), 1)), "PROTOCOL_MISMATCH"},
		"r1-body-as-r2":    {Frame(bytes.Replace(valid, []byte("tsgk-native/r1"), []byte("tsgk-native/r2"), 1)), "REQUEST_MALFORMED"}, // S06-A01: no silent reinterpretation
		"base64":           {Frame(bytes.Replace(valid, []byte(`"source":"YSA9IDE7"`), []byte(`"source":"YSA9IDE"`), 1)), "BASE64_INVALID"},
		"trailing-json":    {Frame(append(append([]byte(nil), valid...), ' ')), "REQUEST_MALFORMED"},
		"limit-zero":       {Frame(bytes.Replace(valid, []byte(`"errors":1000`), []byte(`"errors":0`), 1)), "LIMIT_INVALID"},
		"edits-not-tree":   {Frame(bytes.Replace(editRequest(), []byte(`"output":"tree"`), []byte(`"output":"auto"`), 1)), "EDITS_NOT_ALLOWED"},
		"too-many-edits":   {Frame(manyEdits(5)), "EDIT_COUNT_LIMIT"},
		"locator":          {Frame(declRequest("field:")), "LOCATOR_INVALID"},
		"encoding-unknown": {Frame(bytes.Replace(valid, []byte(`"UTF-8"`), []byte(`"UTF-32"`), 1)), "ENCODING_UNSUPPORTED"},
		"ranges-count":     {Frame(rangeRequest(2, 1)), "RANGES_INVALID"},
		"ranges-outside":   {Frame(rangeRequest(1, 99)), "RANGES_INVALID"},
		"ranges-huge":      {Frame(rangeRequest(1, 4000000000)), "RANGES_INVALID"}, // R1 B-1: checked before any read
	} {
		t.Run(name, func(t *testing.T) {
			res, resp, err := rawExec(t, b, tc.stdin, "native-parse-edit")
			if err != nil || resp.Status != "INVALID_REQUEST" || resp.Code != tc.code || res.ExitCode != 2 || len(resp.Steps) != 0 || !resp.Complete {
				t.Fatalf("%v %s %s exit=%d", err, resp.Status, resp.Code, res.ExitCode)
			}
		})
	}
	// Kit-side response validation.
	req := baseRequest("a = 1;", "native-parse-edit")
	res, resp, err := rawExec(t, b, Frame(req.Encode()), "native-parse-edit")
	if err != nil || resp.Status != kit.StatusCompleted {
		t.Fatalf("valid run: %v %+v", err, resp)
	}
	payload, _ := SplitFrame(res.Stdout)
	versions := [][]byte{req.Source}
	if _, err := Check(resp, req, versions, nil, 0); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}
	for name, f := range map[string]func() error{
		"truncated": func() error { _, err := SplitFrame(res.Stdout[:len(res.Stdout)-1]); return err },
		"trailing":  func() error { _, err := SplitFrame(append(append([]byte(nil), res.Stdout...), 0)); return err },
		"unknown": func() error {
			_, err := DecodeResponse(bytes.Replace(payload, []byte(`"complete":true`), []byte(`"complete":true,"extra":1`), 1))
			return err
		},
		"duplicate": func() error {
			_, err := DecodeResponse(bytes.Replace(payload, []byte(`"complete":true`), []byte(`"complete":true,"complete":true`), 1))
			return err
		},
		"incomplete": func() error {
			r, _ := DecodeResponse(bytes.Replace(payload, []byte(`"complete":true`), []byte(`"complete":false`), 1))
			_, err := Check(r, req, versions, nil, 0)
			return err
		},
		"exit-mismatch": func() error { _, err := Check(resp, req, versions, nil, 1); return err },
		"code-mismatch": func() error {
			lim := baseRequest(plainSource, "native-parse-edit")
			lim.Limits.Nodes, lim.Limits.FullNodes = 5, 5
			res2, r2, err := rawExec(t, b, Frame(lim.Encode()), "native-parse-edit")
			if err != nil || r2.Code != "NODE_LIMIT" {
				return nil
			}
			r2.Code = "DEPTH_LIMIT"
			_, err = Check(r2, lim, [][]byte{lim.Source}, nil, res2.ExitCode)
			return err
		},
		"step-missing": func() error {
			r := resp
			r.Steps = nil
			_, err := Check(r, req, versions, nil, 0)
			return err
		},
		"digest": func() error {
			r, _ := DecodeResponse(payload)
			r.Steps[0].Incremental.Digest = strings.Repeat("0", 64)
			_, err := Check(r, req, versions, nil, 0)
			return err
		},
		"node-graph": func() error {
			r, _ := DecodeResponse(payload)
			r.Steps[0].Incremental.Nodes[2][0] = 5
			_, err := Check(r, req, versions, nil, 0)
			return err
		},
		"transport": func() error { _, err := Check(resp, req, [][]byte{[]byte("a = 2;")}, nil, 0); return err },
	} {
		if err := f(); err == nil {
			t.Errorf("%s: malformed response accepted", name)
		}
	}
}

func rangeRequest(steps int, end uint32) []byte {
	r := baseRequest("a = 1;", "native-parse-edit")
	for i := 0; i < steps; i++ {
		r.Ranges = append(r.Ranges, []kit.Span{{StartByte: 0, EndByte: end, EndPoint: kit.Point{Column: end}}})
	}
	return r.Encode()
}

func editRequest() []byte {
	r := baseRequest("a = 1;", "native-parse-edit")
	r.Edits = editsBy("a = 1;", edit{"1", "2"})
	return r.Encode()
}

func manyEdits(n int) []byte {
	r := baseRequest("a = 1;", "native-parse-edit")
	src := "a = 1;"
	var ops []edit
	for i := 0; i < n; i++ {
		ops = append(ops, edit{"", ";"})
	}
	r.Edits = editsBy(src, ops...)
	return r.Encode()
}

func declRequest(loc string) []byte {
	r := baseRequest("a = 1;", "native-parse-edit")
	r.Declarations = []kit.NativeDeclaration{{Fact: "type_declaration", Node: "assignment", Name: loc}}
	return r.Encode()
}

func repeatSource(stmt string, n int) string { return strings.Repeat(stmt, n) }

// S05-A10/A13/A19: node and depth caps, parse timeout through the progress callback,
// a null tree, caller cancellation, output overflow and the allocator hook end in their
// documented noncomplete statuses with verified cleanup; deep nesting within the cap
// completes without stack failure.
func TestLimitsAndCancellation(t *testing.T) {
	b := fixtureBuild(t, "plain")
	run := func(t *testing.T, req Request, op string) (runner.Result, Response) {
		t.Helper()
		res, resp, err := rawExec(t, b, Frame(req.Encode()), op)
		if err != nil {
			t.Fatal(err)
		}
		return res, resp
	}
	expect := func(t *testing.T, res runner.Result, resp Response, status, code string, exit int) {
		t.Helper()
		if resp.Status != status || resp.Code != code || res.ExitCode != exit {
			t.Fatalf("got %s %s exit %d, want %s %s %d", resp.Status, resp.Code, res.ExitCode, status, code, exit)
		}
	}
	t.Run("node-cap", func(t *testing.T) {
		req := baseRequest(plainSource, "native-parse-edit")
		req.Limits.Nodes, req.Limits.FullNodes = 5, 5
		res, resp := run(t, req, "native-parse-edit")
		expect(t, res, resp, kit.StatusResourceLimit, "NODE_LIMIT", 3)
		if resp.Steps[0].Incremental.Form != nil || resp.Steps[0].Incremental.Nodes != nil {
			t.Fatal("a limited tree must not carry nodes")
		}
	})
	t.Run("depth-cap", func(t *testing.T) {
		req := baseRequest(plainSource, "native-parse-edit")
		req.Limits.Depth = 3
		res, resp := run(t, req, "native-parse-edit")
		expect(t, res, resp, kit.StatusResourceLimit, "DEPTH_LIMIT", 3)
	})
	t.Run("output-limit", func(t *testing.T) {
		req := baseRequest(plainSource, "native-parse-edit")
		req.Limits.OutputBytes = 400
		res, resp := run(t, req, "native-parse-edit")
		expect(t, res, resp, kit.StatusResourceLimit, "OUTPUT_LIMIT", 3)
		if len(resp.Steps) != 0 {
			t.Fatal("steps over the output limit must be dropped")
		}
	})
	t.Run("parse-timeout", func(t *testing.T) {
		req := baseRequest(repeatSource("a = f(1, [2, 3], (4));\n", 200000), "real-world-source-r2")
		req.Output, req.Limits.ParseMillis = kit.OutputAuto, 1
		res, resp := run(t, req, "real-world-source-r2")
		expect(t, res, resp, kit.StatusResourceLimit, "PARSE_TIME_LIMIT", 3)
	})
	t.Run("allocation-limit", func(t *testing.T) {
		req := baseRequest(plainSource, "native-parse-edit")
		req.Limits.MemoryBytes = 2048
		res, resp := run(t, req, "native-parse-edit")
		expect(t, res, resp, kit.StatusResourceLimit, "ALLOCATION_LIMIT", 3)
	})
	t.Run("null-tree", func(t *testing.T) {
		nb := fixtureBuild(t, "plain", "TSGK_FAULT_NULL_TREE")
		req := baseRequest(plainSource, "native-parse-edit")
		req.Edits = editsBy(plainSource, edit{"1", "2"})
		res, resp, err := rawExec(t, nb, Frame(req.Encode()), "native-parse-edit")
		if err != nil {
			t.Fatal(err)
		}
		expect(t, res, resp, kit.StatusFailed, "PARSE_NULL", 4)
		if len(resp.Steps) != 2 || resp.Steps[1].Incremental.Form != nil || resp.StepsCompleted != 1 {
			t.Fatalf("null tree must stay distinct from an empty tree: %+v", resp.Steps)
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		req := baseRequest(repeatSource("a = f(1, [2, 3], (4));\n", 1400000), "real-world-source-r2")
		req.Output = kit.OutputAuto
		// The caller cancels a running driver that has read all but the last request byte and
		// waits for it, so it cannot answer first; a timer raced the parse time (#65). The
		// spec is Exec's, made interactive to hold back that byte.
		if err := b.Verify(); err != nil {
			t.Fatal(err)
		}
		op, frame := kit.NativeOperations()["real-world-source-r2"], Frame(req.Encode())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p, err := runner.Start(ctx, runner.Spec{Path: b.Executable, Args: []string{"single"}, Dir: b.Dir, Interactive: true,
			StdoutBytes: int64(op.OutputBytes) + 4, StderrBytes: 65536, Wall: 90 * time.Second, Grace: 5 * time.Second,
			Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: runtime.GOOS != "darwin"}, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Stdin().Write(frame[:len(frame)-1]); err != nil {
			t.Fatal(err)
		}
		cancel()
		if res := p.Wait(); res.Status != runner.StatusCancelled || !res.Cleanup.Verified {
			t.Fatalf("cancel: %s %+v", res.Status, res.Cleanup)
		}
	})
	t.Run("deep-nesting", func(t *testing.T) {
		deep := func(n int) string { return "a = " + strings.Repeat("(", n) + "1" + strings.Repeat(")", n) + ";\n" }
		req := baseRequest(deep(12000), "real-world-source-r2")
		req.Output = kit.OutputAuto
		res, resp := run(t, req, "real-world-source-r2")
		expect(t, res, resp, kit.StatusCompleted, "", 0)
		if resp.Steps[0].Incremental.MaxDepth < 10000 {
			t.Fatalf("max depth %d", resp.Steps[0].Incremental.MaxDepth)
		}
		req = baseRequest(deep(100001), "real-world-source-r2")
		req.Output = kit.OutputAuto
		res, resp = run(t, req, "real-world-source-r2")
		expect(t, res, resp, kit.StatusResourceLimit, "DEPTH_LIMIT", 3)
	})
	t.Run("separate-processes", func(t *testing.T) {
		var first string
		for i := 0; i < 10; i++ {
			r := runCase(t, b, "repeat", kit.EncodingUTF8, plainSource, editsBy(plainSource, edit{"b", "bb"}))
			if r.Assessment != kit.AssessPass || !r.Process.Cleanup.Verified {
				t.Fatalf("run %d: %s %s", i, r.Assessment, r.Code)
			}
			if first == "" {
				first = r.Steps[1].Incremental.Digest
			} else if r.Steps[1].Incremental.Digest != first {
				t.Fatal("same request in a new process must give the same tree")
			}
		}
	})
}

// S05-A18 (owned part): the auto gate switches to a summary whose digest equals the full
// tree digest of the same bytes; errors are capped; declarations come from a node-type
// traversal; registered points give partial trees; oversized requests never launch.
func TestSummaryGate(t *testing.T) {
	b := fixtureBuild(t, "plain")
	src := repeatSource("x = f(1, [2, 3]);\n", 3000) + repeatSource("y = (;\n", 1200)
	req := baseRequest(src, "real-world-source-r2")
	req.Output = kit.OutputAuto
	req.Declarations = []kit.NativeDeclaration{{Fact: "type_declaration", Node: "assignment", Name: "field:name"}}
	req.Points = []kit.NativePoint{{ID: "P01", Byte: 9}, {ID: "P02", Byte: uint32(len(src) + 5)}, {ID: "P03", Byte: 4}}
	res, resp, err := rawExec(t, b, Frame(req.Encode()), "real-world-source-r2")
	if err != nil || resp.Status != kit.StatusCompleted {
		t.Fatalf("%v %s %s", err, resp.Status, resp.Code)
	}
	sum := resp.Steps[0].Incremental
	if *sum.Form != "summary" || sum.Reason != "DESCENDANT_LIMIT" || sum.Errors.Total <= 1000 || !sum.Errors.Truncated || len(sum.Errors.Items) != 1000 {
		t.Fatalf("summary %v %s total=%d", *sum.Form, sum.Reason, sum.Errors.Total)
	}
	if len(*sum.Declarations) < 3000 || len(sum.PartialTrees) != 3 || len(sum.PartialTrees[0].Nodes) == 0 || len(sum.PartialTrees[1].Nodes) != 0 {
		t.Fatalf("declarations %d partial %d", len(*sum.Declarations), len(sum.PartialTrees))
	}
	checked, err := Check(resp, req, [][]byte{req.Source}, nil, res.ExitCode)
	if err != nil {
		t.Fatal(err)
	}
	// R1 MINOR-3: the deepest node of a partial tree keeps its field (identifier `f`, field function).
	p3 := checked.Steps[0].Incremental.Partials[2]
	if deepest := p3[len(p3)-1]; deepest.Type != "identifier" || deepest.Field == nil || *deepest.Field != "function" {
		t.Fatalf("partial deepest node %+v", deepest)
	}
	full := req
	full.Limits.FullNodes = full.Limits.Nodes
	_, fr, err := rawExec(t, b, Frame(full.Encode()), "real-world-source-r2")
	if err != nil || *fr.Steps[0].Incremental.Form != "full" || fr.Steps[0].Incremental.Digest != sum.Digest {
		t.Fatalf("full and summary digests must match: %v", err)
	}
	// An encoded request over 50331648 bytes is refused before any launch.
	huge := bytes.Repeat([]byte("a;"), 33554432/2)
	c := kit.IncrementalCase{ID: "huge", Encoding: kit.EncodingUTF8, Edits: []kit.Edit{{StartByte: 0, OldEndByte: 33554432, NewEndByte: 2, Old: huge, New: []byte("b;")}}}
	ctx := testContext("real-world-source-r2")
	r := b.RunCase(context.Background(), ctx, c, huge)
	if r.ExecutionStatus != kit.StatusResourceLimit || r.Code != "REQUEST_TOO_LARGE" || r.Process != nil {
		t.Fatalf("oversized request: %s %s", r.ExecutionStatus, r.Code)
	}
}
