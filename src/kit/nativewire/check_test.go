package nativewire

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

// oneNodeTree is a completed full r1 tree of one root node over n bytes.
func oneNodeTree(n uint32, status, code string) WireTree {
	if status != StatusCompleted {
		return WireTree{Status: status, Code: code}
	}
	form := "full"
	root := []TreeNode{{Parent: -1, Type: "x", Named: true, EndByte: n, EndPoint: Point{Column: n}}}
	return WireTree{Status: status, Form: &form, DescendantCount: 1, Digest: TreeDigest(root), Types: []string{"x"},
		Nodes: [][]int64{{-1, 0, -1, 1, 0, int64(n), 0, 0, 0, int64(n)}}}
}

func sourceID(b []byte) SourceID {
	s := sha256.Sum256(b)
	return SourceID{Bytes: uint64(len(b)), SHA256: hex.EncodeToString(s[:])}
}

// editResponse is an r1 response to one edit of "a" into "ab" and its expectation.
func editResponse() (Response, Expect) {
	v0, v1 := []byte("a"), []byte("ab")
	exit := 0
	e := Expect{ID: "c", Nodes: 10, SourceBytes: 1, Edits: []EditSpan{{StartByte: 1, OldEndByte: 1, NewEndByte: 2}},
		Sources: []SourceID{sourceID(v0), sourceID(v1)}, EditPoints: []EditPoints{{Start: Point{Column: 1}, OldEnd: Point{Column: 1}, NewEnd: Point{Column: 2}}}, Exit: &exit}
	fresh := oneNodeTree(2, StatusCompleted, "")
	r := Response{Protocol: Protocol, ID: "c", Status: StatusCompleted, Producer: Producer{Query: "UNSUPPORTED"}, SourceBytes: 1, StepsCompleted: 2, Complete: true,
		Steps: []Step{
			{Step: 0, SourceBytes: 1, SourceSHA256: e.Sources[0].SHA256, Incremental: oneNodeTree(1, StatusCompleted, "")},
			{Step: 1, SourceBytes: 2, SourceSHA256: e.Sources[1].SHA256, Edit: &StepEdit{StartByte: 1, OldEndByte: 1, NewEndByte: 2, StartPoint: [2]uint32{0, 1}, OldEndPoint: [2]uint32{0, 1}, NewEndPoint: [2]uint32{0, 2}},
				Incremental: oneNodeTree(2, StatusCompleted, ""), Fresh: &fresh},
		}}
	return r, e
}

func code(err error) string {
	var we *Error
	if errors.As(err, &we) {
		return we.Code
	}
	return ""
}

func TestCheckAbsentExpectations(t *testing.T) {
	r, e := editResponse()
	if _, err := Check(r, e); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	for name, tc := range map[string]struct {
		edit func(*Response, *Expect)
		want string
	}{
		"exit-mismatch":           {func(_ *Response, e *Expect) { four := 4; e.Exit = &four }, "EXIT_STATUS_MISMATCH"},
		"exit-absent":             {func(_ *Response, e *Expect) { e.Exit = nil }, ""},
		"exit-absent-bad-status":  {func(r *Response, e *Expect) { e.Exit, r.Status = nil, "BOGUS" }, "EXIT_STATUS_MISMATCH"},
		"points-absent":           {func(_ *Response, e *Expect) { e.EditPoints = nil }, ""},
		"points-absent-byte-span": {func(r *Response, e *Expect) { e.EditPoints = nil; r.Steps[1].Edit.NewEndByte = 3 }, "EDIT_POINT_MISMATCH"},
		"points-wrong":            {func(r *Response, _ *Expect) { r.Steps[1].Edit.NewEndPoint = [2]uint32{0, 3} }, "EDIT_POINT_MISMATCH"},
		"points-short":            {func(_ *Response, e *Expect) { e.EditPoints = e.EditPoints[:0] }, "RESPONSE_INVALID"},
		"sources-short":           {func(_ *Response, e *Expect) { e.Sources = e.Sources[:1] }, "RESPONSE_INVALID"},
		// an unfinished step that is not the last fails before the later step's source mismatch
		"open-before-later-step": {func(r *Response, e *Expect) {
			three := 3
			r.Status, r.Code, e.Exit = StatusResourceLimit, "NODE_LIMIT", &three
			r.Steps[0].Incremental = oneNodeTree(0, StatusResourceLimit, "NODE_LIMIT")
			r.Steps[1].SourceSHA256 = "0"
		}, "RESPONSE_INVALID"},
	} {
		t.Run(name, func(t *testing.T) {
			r, e := editResponse()
			tc.edit(&r, &e)
			_, err := Check(r, e)
			if code(err) != tc.want || (tc.want == "") != (err == nil) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCheckStreamedEqualsCheck(t *testing.T) {
	// a caller checking the steps one by one (the offline replay) reaches the same verdict
	r, e := editResponse()
	three := 3
	r.Status, r.Code, r.StepsCompleted, e.Exit = StatusResourceLimit, "NODE_LIMIT", 1, &three
	fresh := oneNodeTree(0, StatusResourceLimit, "NODE_LIMIT")
	r.Steps[1].Fresh = &fresh
	if _, err := Check(r, e); err != nil {
		t.Fatalf("early end: %v", err)
	}
	var tally Tally
	for k, s := range r.Steps {
		cs, err := CheckStep(k, s, e)
		if err != nil {
			t.Fatal(err)
		}
		if err := CheckOpen(k, len(r.Steps), r.Status, cs.Done(k)); err != nil {
			t.Fatal(err)
		}
		tally.Add(k, cs)
	}
	header := r
	header.Steps = nil
	if stop, err := CheckHeader(header, tally.Steps, e); err != nil || stop {
		t.Fatalf("header: %v %v", stop, err)
	}
	if err := CheckEnd(header, tally, e); err != nil {
		t.Fatal(err)
	}
	header.StepsCompleted = 2
	if code(CheckEnd(header, tally, e)) != "RESPONSE_INVALID" {
		t.Fatal("steps_completed of the unfinished step accepted")
	}
}

func TestValidateTreeCause(t *testing.T) {
	w := oneNodeTree(1, StatusCompleted, "")
	w.Nodes[0][5] = 5 // past the source
	_, err := CheckTree(w, 1, Expect{Nodes: 10})
	var te *TreeError
	if code(err) != "RESPONSE_INVALID" || !errors.As(err, &te) || te.Code != "TREE_RANGE_INVALID" || te.Path != "#/nodes/0" {
		t.Fatalf("tree cause: %v", err)
	}
}
