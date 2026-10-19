package querypolicy

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestFilterCancellationInsidePredicateScan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	caps := make([]Capture, 1024)
	for i := range caps {
		caps[i] = Capture{Name: "unused", EndByte: 1}
	}
	caps[0].Name = "x"
	preds := make([]Predicate, 128)
	for i := range preds {
		preds[i] = predicate("eq?", capture("x", "ONE"), text("a"))
	}
	checks := 0
	indices, code, err := FilterChecked(func() error {
		checks++
		if checks == 4 {
			cancel()
		}
		return ctx.Err()
	}, 1, preds, caps, "UTF-8", []byte("a"))
	if !errors.Is(err, context.Canceled) || code != "" || indices != nil || checks != 4 {
		t.Fatalf("mid-filter cancellation: %v %q %v checks=%d", indices, code, err, checks)
	}
	indices, code = Filter(1, preds, caps, "UTF-8", []byte("a"))
	if code != "" || len(indices) != len(caps) {
		t.Fatalf("original policy changed: %v %q", indices, code)
	}
}

func capture(name, quantifier string) Step {
	return Step{Kind: "capture", Value: name, Quantifier: &quantifier}
}
func text(value string) Step { return Step{Kind: "string", Value: value} }
func predicate(op string, args ...Step) Predicate {
	return Predicate{Steps: append([]Step{text(op)}, args...)}
}

func TestMalformedMetadataIsRejected(t *testing.T) {
	one := "ONE"
	for _, tc := range []struct {
		name  string
		value Predicate
	}{
		{"no-steps", Predicate{}},
		{"pattern", Predicate{Pattern: 1, Steps: []Step{text("eq?"), capture("x", "ONE"), text("a")}}},
		{"capture-operator", Predicate{Steps: []Step{capture("eq?", "ONE"), capture("x", "ONE"), text("a")}}},
		{"nil-quantifier", predicate("eq?", Step{Kind: "capture", Value: "x"}, text("a"))},
		{"unknown-quantifier", predicate("eq?", capture("x", "INVALID"), text("a"))},
		{"empty-capture-name", predicate("eq?", capture("", "ONE"), text("a"))},
		{"unknown-kind", predicate("eq?", Step{Kind: "other", Value: "x"}, text("a"))},
		{"string-quantifier", predicate("eq?", capture("x", "ONE"), Step{Kind: "string", Value: "a", Quantifier: &one})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Support(1, []Predicate{tc.value}, "UTF-8"); got != InvalidMetadata {
				t.Fatalf("metadata support=%q", got)
			}
			if kept, code := Filter(1, []Predicate{tc.value}, []Capture{}, "UTF-8", nil); code != InvalidMetadata || kept != nil {
				t.Fatalf("metadata filter=%v %q", kept, code)
			}
		})
	}
}

func TestUnsupportedOperands(t *testing.T) {
	x := capture("x", "ONE")
	for _, p := range []Predicate{
		predicate("eq?"), predicate("eq?", x), predicate("eq?", x, text("a"), text("b")),
		predicate("eq?", text("a"), text("a")), predicate("any-of?", x), predicate("any-of?", x, capture("y", "ONE")),
	} {
		want := "QUERY_PREDICATE_UNSUPPORTED:" + p.Steps[0].Value
		if got := Support(1, []Predicate{p}, "UTF-8"); got != want {
			t.Fatalf("operand support=%q want%q", got, want)
		}
	}
}

func TestMalformedCaptureIsRejected(t *testing.T) {
	pred := []Predicate{predicate("eq?", capture("x", "ONE"), text("a"))}
	for _, tc := range []struct {
		name     string
		patterns uint32
		values   []Capture
	}{
		{"missing", 1, nil},
		{"reverse-range", 1, []Capture{{Name: "x", StartByte: 2, EndByte: 1}}},
		{"past-source", 1, []Capture{{Name: "x", EndByte: 4}}},
		{"overflow-range", 1, []Capture{{Name: "x", EndByte: ^uint32(0)}}},
		{"pattern", 1, []Capture{{Name: "x", Pattern: 1, EndByte: 1}}},
		{"no-pattern", 0, []Capture{{Name: "x", EndByte: 1}}},
		{"empty-name", 1, []Capture{{EndByte: 1}}},
		{"mixed-match-pattern", 2, []Capture{{Name: "x", EndByte: 1}, {Name: "y", Pattern: 1, StartByte: 2, EndByte: 3}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			predicates := pred
			want := InvalidCapture
			if tc.patterns == 0 {
				predicates = nil
			}
			if kept, code := Filter(tc.patterns, predicates, tc.values, "UTF-8", []byte("a b")); code != want || kept != nil {
				t.Fatalf("capture filter=%v %q", kept, code)
			}
		})
	}
	// An unsupported operator must not hide an independently observed bad range.
	bad := []Capture{{Name: "x", EndByte: 4}}
	if kept, code := Filter(1, []Predicate{predicate("match?", capture("x", "ONE"), text("a"))}, bad, "UTF-8", []byte("a")); code != InvalidCapture || kept != nil {
		t.Fatalf("invalid evidence downgraded: %v %q", kept, code)
	}
}

func TestOperandMustBePresentAndUnambiguous(t *testing.T) {
	source := []byte("a b")
	for _, tc := range []struct {
		name string
		p    Predicate
		caps []Capture
		code string
	}{
		{"left-absent", predicate("eq?", capture("missing", "ONE"), text("a")), []Capture{{Name: "x", EndByte: 1}}, AbsentCapture},
		{"right-absent", predicate("eq?", capture("x", "ONE"), capture("y", "ONE")), []Capture{{Name: "x", EndByte: 1}}, AbsentCapture},
		{"left-ambiguous", predicate("eq?", capture("x", "ONE"), text("a")), []Capture{{Name: "x", EndByte: 1}, {Name: "x", StartByte: 2, EndByte: 3}}, AmbiguousCapture},
		{"right-ambiguous", predicate("eq?", capture("x", "ONE"), capture("y", "ONE")), []Capture{{Name: "x", EndByte: 1}, {Name: "y", EndByte: 1}, {Name: "y", StartByte: 2, EndByte: 3}}, AmbiguousCapture},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if kept, code := Filter(1, []Predicate{tc.p}, tc.caps, "UTF-8", source); code != tc.code || kept != nil {
				t.Fatalf("operand filter=%v %q", kept, code)
			}
		})
	}
}

func TestOriginalIndicesAndEmptyPresence(t *testing.T) {
	pred := []Predicate{predicate("eq?", capture("x", "ONE"), text("a"))}
	caps := []Capture{{Name: "x", EndByte: 1}, {Match: 1, Name: "x", StartByte: 2, EndByte: 3}, {Name: "y", StartByte: 4, EndByte: 5}, {Match: 1, Name: "y", StartByte: 6, EndByte: 7}}
	if kept, code := Filter(1, pred, caps, "UTF-8", []byte("a b b a")); code != "" || !slices.Equal(kept, []int{0, 2}) {
		t.Fatalf("original order lost: %v %q", kept, code)
	}
	if kept, code := Filter(1, pred, []Capture{}, "UTF-8", nil); code != "" || kept == nil || len(kept) != 0 {
		t.Fatalf("completed empty absent: %v %q", kept, code)
	}
	// An existing zero-width capture has a value even when the source is empty.
	empty := []Predicate{predicate("eq?", capture("x", "ONE"), text(""))}
	if kept, code := Filter(1, empty, []Capture{{Name: "x"}}, "UTF-8", nil); code != "" || !slices.Equal(kept, []int{0}) {
		t.Fatalf("zero-width capture lost: %v %q", kept, code)
	}
}
