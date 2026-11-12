package kit

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

func svcAssets(t *testing.T) (asset, registry, inventory, fixture []byte) {
	t.Helper()
	read := func(name string) []byte {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	return read("../contracts/query-svc-expected-r1.json"), read("../contracts/query-semantic-registry-r1.json"),
		read("../contracts/qualification-c1.json"), read("../testdata/native/n461/svc.json")
}

func svcCode(t *testing.T, err error) string {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("not a kit error: %v", err)
	}
	return e.Code
}

func TestSvcExpectedSealedAndBound(t *testing.T) {
	asset, registry, inventory, fixture := svcAssets(t)
	if len(asset) != 196457 || digestHex(asset) != "bbe9116749abe78cc218a0455cad476d9d4b4b9044447ff35d10a181df7eb746" {
		t.Fatalf("sealed expected asset changed: %d/%s", len(asset), digestHex(asset))
	}
	e, err := ParseSvcExpected(asset, registry, inventory, fixture)
	if err != nil || len(e.Cases) != 69 || e.Workload.Edits != 41 || e.Workload.Steps != 110 || len(e.Workload.Queries) != 2 || e.InlineSteps != 81 {
		t.Fatalf("asset: %v %d", err, len(e.Cases))
	}
	count := map[string]int{}
	for _, o := range e.Obligations() {
		count[o.Kind]++
	}
	want := map[string]int{SvcKindHistory: 110, SvcKindEditPoint: 41, SvcKindSegment: 109, SvcKindInlineTree: 82, SvcKindQuery: 164, SvcKindCodeBehind: 2, SvcKindNegative: 1}
	for k, v := range want {
		if count[k] != v {
			t.Errorf("%s = %d, want %d", k, count[k], v)
		}
	}
}

// The independent expected values and the existing offline SVC path agree on every obligation
// that has expected values, and the rest stay BLOCKED with their reason.
func TestSvcExpectedAgainstExistingObservationPath(t *testing.T) {
	asset, registry, inventory, fixture := svcAssets(t)
	e, err := ParseSvcExpected(asset, registry, inventory, fixture)
	if err != nil {
		t.Fatal(err)
	}
	judgments, err := VerifySvcExpected(e, inventory)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]int{}
	for _, j := range judgments {
		kind := strings.Split(j.ID, "/")[1]
		states[kind+" "+j.State+" "+j.Code]++
		if j.State == "FAILED" {
			t.Errorf("%s: %s", j.ID, j.Code)
		}
	}
	for _, key := range []string{"SVC_HISTORY VERIFIED ", "SVC_EDIT_POINT VERIFIED ", "SVC_SEGMENT VERIFIED ", "SVC_CODEBEHIND VERIFIED ", "SVC_NEGATIVE VERIFIED ",
		"SVC_INLINE_TREE BLOCKED SVC_INLINE_TREE_GRAMMAR_UNAVAILABLE", "SVC_INLINE_QUERY BLOCKED SVC_QUERY_EXPECTATION_UNREGISTERED"} {
		if states[key] == 0 {
			t.Errorf("missing state %q in %v", key, states)
		}
	}
	t.Logf("states: %v", states)
}

// Simple facts are also derived here with plain string operations, independently of both the
// asset and ObserveServiceHost: the directive range, the closing delimiter and the inline start.
func TestSvcExpectedSimpleAnchors(t *testing.T) {
	asset, registry, inventory, fixture := svcAssets(t)
	e, err := ParseSvcExpected(asset, registry, inventory, fixture)
	if err != nil {
		t.Fatal(err)
	}
	wl, err := svcInventoryWorkload(inventory)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for i, c := range e.Cases {
		versions, _, err := ApplyEdits(EncodingUTF8, []byte(*wl.Cases[i].Source), wl.Cases[i].Edits, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		for k, s := range c.Steps {
			o := s.Observation
			text := string(versions[k])
			open := strings.Index(text, "<%")
			// a quoted "%>" is inside the attribute value, not the delimiter: a plain string search cannot anchor that case
			if o == nil || o.Directive == nil || o.Directive.Close == nil || len(o.AdditionalDirectives) != 0 || strings.Contains(c.ID, "quoted-close") {
				continue
			}
			end := strings.Index(text[open:], "%>")
			if end < 0 || o.Directive.Close.StartByte != uint32(open+end) || o.Directive.Close.EndByte != uint32(open+end+2) || o.Directive.Range.StartByte != uint32(open) ||
				o.Directive.Range.EndByte != uint32(open+end+2) || o.Directive.Open.StartByte != uint32(open) {
				t.Errorf("%s step %d: directive span %+v vs text offsets %d/%d", c.ID, k, o.Directive.Range, open, end)
			}
			if o.IncludedRanges != nil && (o.IncludedRanges[0].StartByte != o.Directive.Range.EndByte || o.IncludedRanges[0].EndByte != uint32(len(text))) {
				t.Errorf("%s step %d: included range %+v", c.ID, k, o.IncludedRanges[0])
			}
			checked++
		}
	}
	if checked < 60 {
		t.Fatalf("only %d single-directive steps anchored", checked)
	}
}

func TestSvcExpectedRejectsForgeries(t *testing.T) {
	asset, registry, inventory, fixture := svcAssets(t)
	mutate := func(f func(e *SvcExpected)) []byte {
		var e SvcExpected
		if err := jsonv2.Unmarshal(asset, &e); err != nil {
			t.Fatal(err)
		}
		f(&e)
		out, err := jsonv2.Marshal(e, jsonv2.FormatNilSliceAsNull(true))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	byID := func(e *SvcExpected, id string) *SvcExpectedCase {
		for i := range e.Cases {
			if e.Cases[i].ID == id {
				return &e.Cases[i]
			}
		}
		t.Fatal(id)
		return nil
	}
	for _, tc := range []struct {
		name, code string
		data       []byte
		inventory  []byte
	}{
		{"deleted case", "SVC_EXPECTED_WORKLOAD", mutate(func(e *SvcExpected) { e.Cases = e.Cases[:68] }), inventory},
		{"swapped cases", "SVC_EXPECTED_MEMBERSHIP", mutate(func(e *SvcExpected) { e.Cases[0], e.Cases[1] = e.Cases[1], e.Cases[0] }), inventory},
		{"deleted edit", "SVC_EXPECTED_MEMBERSHIP", mutate(func(e *SvcExpected) { c := byID(e, "n461-svc-inline-pe-r1"); c.Edits = c.Edits[:1] }), inventory},
		{"deleted step", "SVC_EXPECTED_MEMBERSHIP", mutate(func(e *SvcExpected) { c := byID(e, "n461-svc-inline-pe-r1"); c.Steps = c.Steps[:2] }), inventory},
		{"other source hash", "SVC_EXPECTED_MEMBERSHIP", mutate(func(e *SvcExpected) { byID(e, "n461-svc-directive-p-r1").SHA256 = strings.Repeat("0", 64) }), inventory},
		{"observation null", "SVC_EXPECTED_STEP", mutate(func(e *SvcExpected) { byID(e, "n461-svc-directive-p-r1").Steps[0].Observation = nil }), inventory},
		{"reference with observation", "SVC_EXPECTED_STEP", mutate(func(e *SvcExpected) {
			r, d := byID(e, "n461-svc-reference-p-r2"), byID(e, "n461-svc-directive-p-r1")
			r.Steps[0].Observation = d.Steps[0].Observation
		}), inventory},
		{"VB negative turned into pass", "SVC_EXPECTED_NEGATIVE", mutate(func(e *SvcExpected) {
			c := byID(e, "n461-svc-language-vb-n-r1")
			c.Negative.Assessment, c.Negative.Code = "PASS", ""
		}), inventory},
		{"VB negative deleted", "SVC_EXPECTED_MEMBERSHIP", mutate(func(e *SvcExpected) { byID(e, "n461-svc-language-vb-n-r1").Negative = nil }), inventory},
		{"VB negative gains a tree", "SVC_EXPECTED_NEGATIVE", mutate(func(e *SvcExpected) {
			one := 1
			byID(e, "n461-svc-language-vb-n-r1").Negative.InlineTree = &one
		}), inventory},
		{"wrong query", "SVC_EXPECTED_WORKLOAD", mutate(func(e *SvcExpected) { e.Workload.Queries[1].SHA256 = strings.Repeat("1", 64) }), inventory},
		{"codebehind declaration changed", "SVC_EXPECTED_WORKLOAD", mutate(func(e *SvcExpected) { e.Workload.Context.CodeBehind[0].Name = "Other.svc.cs" }), inventory},
		{"counts changed", "SVC_EXPECTED_COUNTS", mutate(func(e *SvcExpected) { e.InlineSteps++ }), inventory},
		{"unknown member", "SVC_EXPECTED_JSON", bytes.Replace(asset, []byte(`"basis":`), []byte(`"extra":1,"basis":`), 1), inventory},
		{"other fixture", "SVC_EXPECTED_PIN", asset, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := fixture
			if tc.inventory == nil {
				fx = append(slices.Clone(fixture), '\n')
			}
			_, err := ParseSvcExpected(tc.data, registry, inventory, fx)
			if err == nil || svcCode(t, err) != tc.code {
				t.Fatalf("accepted or wrong code: %v, want %s", err, tc.code)
			}
		})
	}
}

// A forged value that still parses (same shape, wrong content) is caught when the obligation is
// recomputed from the pinned inventory: history, edit point, segment, range, point and CodeBehind.
func TestSvcExpectedForgedContentFailsRecompute(t *testing.T) {
	asset, registry, inventory, fixture := svcAssets(t)
	base, err := ParseSvcExpected(asset, registry, inventory, fixture)
	if err != nil {
		t.Fatal(err)
	}
	clone := func() SvcExpected {
		var e SvcExpected
		data, _ := jsonv2.Marshal(base, jsonv2.FormatNilSliceAsNull(true))
		if err := jsonv2.Unmarshal(data, &e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	caseOf := func(e *SvcExpected, id string) *SvcExpectedCase {
		for i := range e.Cases {
			if e.Cases[i].ID == id {
				return &e.Cases[i]
			}
		}
		t.Fatal(id)
		return nil
	}
	for _, tc := range []struct {
		name, kind, code string
		forge            func(e *SvcExpected)
	}{
		{"history hash", SvcKindHistory, "SVC_HISTORY_MISMATCH", func(e *SvcExpected) { caseOf(e, "n461-svc-inline-pe-r1").Steps[1].SHA256 = strings.Repeat("a", 64) }},
		{"history length", SvcKindHistory, "SVC_HISTORY_MISMATCH", func(e *SvcExpected) { caseOf(e, "n461-svc-inline-pe-r1").Steps[2].Bytes++ }},
		{"edit point", SvcKindEditPoint, "SVC_EDIT_POINT_MISMATCH", func(e *SvcExpected) { caseOf(e, "n461-svc-inline-pe-r1").Edits[0].NewEndPoint[1]++ }},
		{"edit byte", SvcKindEditPoint, "SVC_EDIT_POINT_MISMATCH", func(e *SvcExpected) { caseOf(e, "n461-svc-inline-pe-r1").Edits[1].OldEndByte++ }},
		{"directive range", SvcKindSegment, "SVC_SEGMENT_MISMATCH", func(e *SvcExpected) {
			caseOf(e, "n461-svc-inline-pe-r1").Steps[0].Observation.Directive.Range.EndByte++
		}},
		{"directive point", SvcKindSegment, "SVC_SEGMENT_MISMATCH", func(e *SvcExpected) {
			caseOf(e, "n461-svc-inline-pe-r1").Steps[0].Observation.Directive.Close.EndPoint.Column++
		}},
		{"included range after edit", SvcKindSegment, "SVC_SEGMENT_MISMATCH", func(e *SvcExpected) {
			o := caseOf(e, "n461-svc-boundary-pe-r1").Steps[1].Observation
			o.IncludedRanges[0].StartByte = caseOf(e, "n461-svc-boundary-pe-r1").Steps[0].Observation.IncludedRanges[0].StartByte
		}},
		{"included range null to empty", SvcKindSegment, "SVC_SEGMENT_MISMATCH", func(e *SvcExpected) {
			caseOf(e, "n461-svc-directive-p-r1").Steps[0].Observation.IncludedRanges = []Span{}
		}},
		{"language status", SvcKindSegment, "SVC_SEGMENT_MISMATCH", func(e *SvcExpected) {
			caseOf(e, "n461-svc-language-vb-n-r1").Steps[0].Observation.Language.Status = "CSHARP"
		}},
		{"code behind resolution", SvcKindSegment, "SVC_SEGMENT_MISMATCH", func(e *SvcExpected) {
			caseOf(e, "n461-svc-codebehind-p-r1").Steps[0].Observation.CodeBehind.Resolution = "NOT_RESOLVED"
		}},
		{"code behind coverage", SvcKindSegment, "SVC_SEGMENT_MISMATCH", func(e *SvcExpected) {
			caseOf(e, "n461-svc-codebehind-p-r1").Steps[0].Observation.Coverage.CodeBehind = "ABSENT"
		}},
		{"code behind declaration never observed", SvcKindCodeBehind, "SVC_CODEBEHIND_UNOBSERVED", func(e *SvcExpected) {
			e.Workload.Context.CodeBehind = append(e.Workload.Context.CodeBehind, SvcReference{Name: "Never.svc.cs", Case: "n461-svc-reference-p-r2"})
		}},
		{"VB negative expected to pass", SvcKindNegative, "SVC_NEGATIVE_CHANGED", func(e *SvcExpected) {
			caseOf(e, "n461-svc-language-vb-n-r1").Negative.Code = "SVC_INLINE_UNRESOLVED"
		}},
		{"VB observation says inline observed", SvcKindNegative, "SVC_NEGATIVE_CHANGED", func(e *SvcExpected) {
			caseOf(e, "n461-svc-language-vb-n-r1").Steps[0].Observation.Coverage.Inline = "OBSERVED"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := clone()
			tc.forge(&e)
			judgments, err := VerifySvcExpected(e, inventory)
			if err != nil {
				t.Fatal(err)
			}
			failed := 0
			for _, j := range judgments {
				if j.State == "FAILED" && strings.Contains(j.ID, "/"+tc.kind+"/") && j.Code == tc.code {
					failed++
				}
			}
			if failed == 0 {
				t.Fatalf("forged %s was not FAILED with %s", tc.name, tc.code)
			}
		})
	}
}

func TestSvcScopeFold(t *testing.T) {
	asset, registry, inventory, fixture := svcAssets(t)
	e, err := ParseSvcExpected(asset, registry, inventory, fixture)
	if err != nil {
		t.Fatal(err)
	}
	judgments, err := VerifySvcExpected(e, inventory)
	if err != nil {
		t.Fatal(err)
	}
	got, err := FoldSvcScope(e, "windows-amd64", judgments, map[string]bool{"linux-amd64": true})
	if err != nil || got.FullScopeComplete || got.Completion != "BLOCKED" {
		t.Fatalf("fold: %+v %v", got, err)
	}
	want := map[string][4]uint64{ // required, verified, blocked(windows), blocked(linux)
		SvcKindHistory: {110, 110, 0, 110}, SvcKindEditPoint: {41, 41, 0, 41}, SvcKindSegment: {109, 109, 0, 109}, SvcKindCodeBehind: {2, 2, 0, 2},
		SvcKindNegative: {1, 1, 0, 1}, SvcKindInlineTree: {82, 0, 82, 82}, SvcKindQuery: {164, 0, 164, 164},
	}
	for _, c := range got.Counters {
		w := want[c.Kind]
		if c.Required != w[0] || c.Platform == "windows-amd64" && (c.Verified != w[1] || c.Blocked != w[2]) || c.Platform == "linux-amd64" && (c.Verified != 0 || c.Blocked != w[3]) || c.Failed != 0 || c.Unprocessed != 0 {
			t.Errorf("counter %s, want %v", c, w)
		}
	}
	// a deleted judgment is not hidden
	got, err = FoldSvcScope(e, "windows-amd64", judgments[1:], nil)
	if err != nil || got.Counters[0].Unprocessed == 0 && got.Counters[0].Verified != got.Counters[0].Required-1 {
		t.Fatalf("deleted judgment: %+v %v", got.Counters[0], err)
	}
	for name, mutate := range map[string]func(j []SvcJudgment) []SvcJudgment{
		"duplicate":            func(j []SvcJudgment) []SvcJudgment { return append(j, j[0]) },
		"undeclared":           func(j []SvcJudgment) []SvcJudgment { j[0].ID += "x"; return j },
		"verified with code":   func(j []SvcJudgment) []SvcJudgment { j[0].Code = "X"; return j },
		"failed without code":  func(j []SvcJudgment) []SvcJudgment { j[0].State, j[0].Code = "FAILED", ""; return j },
		"unknown state":        func(j []SvcJudgment) []SvcJudgment { j[0].State = "PASS"; return j },
		"blocked without code": func(j []SvcJudgment) []SvcJudgment { j[0].State, j[0].Code = "BLOCKED", ""; return j },
	} {
		if _, err := FoldSvcScope(e, "windows-amd64", mutate(slices.Clone(judgments)), nil); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := FoldSvcScope(e, "darwin-arm64", judgments, nil); err == nil {
		t.Error("unregistered host accepted")
	}
	// a FAILED obligation turns the completion into FAIL
	failed := slices.Clone(judgments)
	failed[0].State, failed[0].Code = "FAILED", "SVC_HISTORY_MISMATCH"
	if got, err := FoldSvcScope(e, "windows-amd64", failed, nil); err != nil || got.Completion != "FAIL" {
		t.Fatalf("failed fold: %+v %v", got, err)
	}
}
