package nativewire

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
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
		"sources-absent":          {func(_ *Response, e *Expect) { e.Sources = nil }, "RESPONSE_INVALID"},
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

func TestApprovedHistoricalRuntimeLoggerForwarding(t *testing.T) {
	historical := strings.Repeat("d", 64)
	producer := strings.Repeat("a", 64)
	source := []byte("x")
	sourcePin := sourceID(source)
	observation, err := json.Marshal(reuseobs.Observation{Schema: reuseobs.Schema, Policy: reuseobs.Policy,
		RuntimeManifestSHA256: historical, RequestID: "case", RunID: "run-1", ProducerSHA256: producer,
		Role: "initial", SourceSHA256: sourcePin.SHA256, SourceBytes: sourcePin.Bytes,
		Status: "COMPLETE", Events: 2, Bytes: 30, Counts: &reuseobs.Counts{NewParse: 1, Done: 1}})
	if err != nil {
		t.Fatal(err)
	}
	step := Step{Step: 0, SourceBytes: sourcePin.Bytes, SourceSHA256: sourcePin.SHA256,
		Incremental: oneNodeTree(1, StatusCompleted, ""), Observations: jsontext.Value("[" + string(observation) + "]")}
	step.Incremental.Queries = jsontext.Value("null")
	step.Incremental.API = jsontext.Value("null")
	api, predicates, query := "tsgk-api/r1", "NOT_EVALUATED", "tsgk-query/r1"
	symbols, fields := uint32(1), uint32(0)
	exit := 0
	r := Response{Protocol: ProtocolR4, ID: "case", Status: StatusCompleted,
		Producer: Producer{Query: query, API: &api, Predicates: &predicates, SymbolCount: &symbols, FieldCount: &fields,
			ReuseObservation: jsontext.Value(fmt.Sprintf(`{"policy":%q,"runtime_manifest_sha256":%q}`, reuseobs.Policy, historical))},
		SourceBytes: sourcePin.Bytes, StepsCompleted: 1, Steps: []Step{step}, Complete: true}
	e := Expect{Revision: ProtocolR4, ID: "case", Nodes: 10, SourceBytes: sourcePin.Bytes,
		Sources: []SourceID{sourcePin}, Exit: &exit,
		Observation: &Binding{Manifest: historical, RunID: "run-1", ProducerSHA256: producer}}
	if _, err := Check(r, e); err == nil {
		t.Fatal("legacy nativewire binding accepted an unlisted runtime")
	}
	pin, err := reuseobs.NewRuntimePin(historical, reuseobs.RuntimeLoggerReader)
	if err != nil {
		t.Fatal(err)
	}
	e.Observation.Runtime = &pin
	if _, err := Check(r, e); err != nil {
		t.Fatalf("historical nativewire binding rejected: %v", err)
	}
	wrong, err := reuseobs.NewRuntimePin(strings.Repeat("e", 64), reuseobs.RuntimeLoggerReader)
	if err != nil {
		t.Fatal(err)
	}
	e.Observation.Runtime = &wrong
	if _, err := Check(r, e); code(err) == "" {
		t.Fatal("nativewire accepted a capability outside the approved runtime pin")
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

func TestCheckStepIndex(t *testing.T) {
	r, e := editResponse()
	if _, err := CheckStep(-1, r.Steps[0], e); code(err) != "RESPONSE_INVALID" {
		t.Fatalf("negative step index: %v", err)
	}
}

func TestTreeDigestCancellation(t *testing.T) {
	tree := oneNodeTree(1, StatusCompleted, "")
	stop := errors.New("cancelled hashing")
	calls := 0
	beforeDigest := len(tree.Nodes) + len(tree.Fields)
	_, err := CheckTree(tree, 1, Expect{Nodes: 10, Check: func() error {
		calls++
		if calls > beforeDigest {
			return stop
		}
		return nil
	}})
	if !errors.Is(err, stop) || calls != beforeDigest+1 {
		t.Fatalf("digest checkpoint calls%d err%v", calls, err)
	}
	checked, err := CheckTree(tree, 1, Expect{Nodes: 10, Check: func() error { return nil }})
	if err != nil || TreeDigest(checked.Nodes) != tree.Digest {
		t.Fatalf("normal digest changed: %v", err)
	}
}

func TestPartialTableCancellation(t *testing.T) {
	for _, at := range []int{2, 4} {
		t.Run(fmt.Sprint(at), func(t *testing.T) {
			tree := oneNodeTree(1, StatusCompleted, "")
			summary := "summary"
			tree.Form = &summary
			tree.Nodes = nil
			tree.Errors = &WireErrors{}
			tree.Fields = []string{"a", "b"}
			tree.PartialTrees = make([]WirePartial, 3)
			stopped := errors.New("cancel partial table")
			calls := 0
			got, err := CheckTree(tree, 1, Expect{Nodes: 10, Check: func() error {
				calls++
				if calls == at {
					return stopped
				}
				return nil
			}})
			wantPartials := 0
			if at == 4 {
				wantPartials = 1
			}
			if err != stopped || calls != at || len(got.Partials) != wantPartials {
				t.Fatalf("partial cancellation err%v calls%d partials%d", err, calls, len(got.Partials))
			}
		})
	}
}
