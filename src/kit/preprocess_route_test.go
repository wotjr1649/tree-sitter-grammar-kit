package kit

import "testing"

func TestPreprocessSelectionDisjoint(t *testing.T) {
	span := func(a, b uint32) Span { return Span{StartByte: a, EndByte: b} }
	step := func(active, inactive []Span) PreprocessStep {
		return PreprocessStep{Status: PreprocessOK, Active: active, Inactive: inactive}
	}
	old := step([]Span{span(9, 20)}, []Span{span(26, 37)})
	cur := step([]Span{span(29, 40)}, []Span{span(12, 23)})
	edit := Edit{StartByte: 4, OldEndByte: 8, NewEndByte: 11}
	for _, tc := range []struct {
		name     string
		old, cur PreprocessStep
		edit     Edit
		a, b     uint64
		want     bool
	}{
		{"switch", old, cur, edit, 44, 47, true},
		{"same-selection-shift", old, step([]Span{span(18, 29)}, []Span{span(35, 46)}), Edit{StartByte: 4, OldEndByte: 8, NewEndByte: 17}, 44, 53, false},
		{"same-selection-shift-into-old-inactive", old, step([]Span{span(26, 37)}, []Span{span(43, 54)}), Edit{StartByte: 4, OldEndByte: 8, NewEndByte: 25}, 44, 61, false},
		{"common-trailer", step([]Span{span(6, 17), span(41, 52)}, []Span{span(23, 34)}), step([]Span{span(23, 34), span(41, 52)}, []Span{span(6, 17)}), Edit{StartByte: 4, OldEndByte: 5, NewEndByte: 5}, 52, 52, false},
		{"full-replacement-no-switch", step([]Span{span(0, 10)}, nil), step([]Span{span(0, 10)}, nil), Edit{OldEndByte: 10, NewEndByte: 10}, 10, 10, false},
		{"insert-prefix", old, step([]Span{span(12, 23)}, []Span{span(29, 40)}), Edit{NewEndByte: 3}, 44, 47, false},
		{"delete-prefix", old, step([]Span{span(6, 17)}, []Span{span(23, 34)}), Edit{OldEndByte: 3}, 44, 41, false},
		{"wrong-length", old, cur, edit, 44, 48, false},
		{"bad-edit", old, cur, Edit{StartByte: 9, OldEndByte: 8, NewEndByte: 11}, 44, 47, false},
		{"empty-active", step(nil, nil), cur, edit, 44, 47, false},
		{"zero-width", step([]Span{span(9, 9)}, nil), cur, edit, 44, 47, false},
		{"range-outside-source", old, step([]Span{span(29, 48)}, nil), edit, 44, 47, false},
		{"overlapping-selection", old, step([]Span{span(29, 40)}, []Span{span(29, 40)}), edit, 44, 47, false},
		{"unsorted", step([]Span{span(20, 21), span(9, 10)}, nil), cur, edit, 44, 47, false},
		{"source-limit", old, cur, edit, 1 << 32, 1 << 32, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := PreprocessSelectionDisjoint(tc.old, tc.cur, tc.edit, tc.a, tc.b); got != tc.want {
				t.Fatalf("got %t, want %t", got, tc.want)
			}
		})
	}
	bad := old
	bad.Status = PreprocessMalformed
	if PreprocessSelectionDisjoint(bad, cur, edit, 44, 47) {
		t.Fatal("non-OK selection accepted")
	}
	// A source-independent r2 pin from the pre-change live probe remains replayable.
	pp := &Preprocess{Mode: PreprocessMode, Symbols: []string{"A"}}
	legacy := nativePolicy(NativeOperations()["native-parse-edit"], "tree", pp, false)
	if legacy.Schema != NativePolicySchemaR2 || legacy.SHA256 != "13bab3289f8629ce6063c13ed0beade458464418b7519419980a87cd42ec7517" {
		t.Fatalf("legacy policy changed: %+v", legacy)
	}
	current := NativePolicy(NativeOperations()["native-parse-edit"], "tree", pp)
	r3 := nativePolicy(NativeOperations()["native-parse-edit"], "tree", pp, true)
	if r3.Schema != NativePolicySchemaR3 || r3.SHA256 == legacy.SHA256 || current.Schema != NativePolicySchemaR4 || current.SHA256 == legacy.SHA256 || current.SHA256 == r3.SHA256 {
		t.Fatalf("selection rule unbound: %+v", current)
	}
}
