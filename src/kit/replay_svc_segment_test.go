package kit

import (
	"encoding/json"
	"fmt"
	"testing"
)

func svcSegmentCase(t *testing.T, failure string, position int) rcCase {
	t.Helper()
	f := newFxNative()
	data, _ := json.Marshal(f.cases[1])
	var base rcCase
	if err := json.Unmarshal(data, &base); err != nil {
		t.Fatal(err)
	}
	c := base
	c.Steps = nil
	segments := []map[string]any{}
	for i := range 3 {
		var part rcCase
		if err := json.Unmarshal(data, &part); err != nil {
			t.Fatal(err)
		}
		start := len(c.Steps)
		for k := range part.Steps {
			part.Steps[k].Step = start + k
			part.Steps[k].Composite = &rcComposite{Directive: &SvcDirective{Close: &Span{}}, Coverage: SvcCoverage{Inline: "OBSERVED"}}
		}
		part.Steps[0].Restarted = i > 0
		claims := map[string]string{"incremental_equality": claimPass, "incremental_route": claimPass, "expectations": claimNotClaimed}
		step := &part.Steps[1]
		if failure == "query-equality" {
			for _, tree := range []*rcTree{step.Incremental, step.Fresh} {
				if err := json.Unmarshal([]byte(`{"queries":[{"id":"q","captures":[]}]}`), tree); err != nil {
					t.Fatal(err)
				}
			}
			if err := json.Unmarshal([]byte(`{"query_comparison":{"equal":true}}`), step); err != nil {
				t.Fatal(err)
			}
		}
		if i == position {
			switch failure {
			case "equality":
				step.Fresh.Tree.Nodes[1].Type = "different"
				step.Fresh.Digest = TreeDigest(step.Fresh.Tree.Nodes)
				step.Comparison.Equal = false
				step.Comparison.First = CompareTrees(step.Incremental.Tree.Nodes, step.Fresh.Tree.Nodes)
				claims["incremental_equality"] = claimFail
			case "route", "blocked-route":
				step.Route.ReusedNodes, step.Route.Proven = 0, false
				claims["incremental_route"] = claimFail
				if failure == "blocked-route" {
					claims["incremental_route"] = claimBlocked
					for _, tree := range []*rcTree{part.Steps[0].Incremental, step.Incremental, step.Fresh} {
						tree.HasError, tree.Tree.Nodes[0].HasError = true, true
						tree.Digest = TreeDigest(tree.Tree.Nodes)
					}
				}
			case "query-equality":
				if err := json.Unmarshal([]byte(`{"queries":[{"id":"q","captures":[{"name":"different"}]}]}`), step.Fresh); err != nil {
					t.Fatal(err)
				}
				step.QueryCompare.Equal = false
			}
		}
		c.Steps = append(c.Steps, part.Steps...)
		segments = append(segments, map[string]any{"start_step": start, "end_step": len(c.Steps), "claims": claims,
			"process": map[string]any{"status": StatusCompleted, "exit_code": 0, "cleanup": map[string]bool{"verified": true}}})
		if i < 2 {
			c.Steps = append(c.Steps, rcStep{Step: len(c.Steps), Composite: &rcComposite{}})
		}
	}
	c.Claims.IncrementalEquality, c.Claims.IncrementalRoute, c.Claims.Expectations = claimNotClaimed, claimNotClaimed, claimPass
	c.Expectations = []rcExpect{{Step: 2, Syntax: "ERROR", Result: claimPass}}
	c.Assessment = AssessPass
	if failure != "" {
		c.Assessment = AssessFail
		if failure == "blocked-route" {
			c.Assessment = AssessBlocked
		}
	}
	data, _ = json.Marshal(c)
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	raw["segments"] = segments
	if failure == "query-equality" {
		raw["oracle_claims"] = map[string]string{"query_equality": claimFail, "query_expectations": claimNotClaimed, "api": claimNotClaimed, "fact_reproduction": claimNotClaimed, "dynamic_sql": claimNotClaimed}
	}
	data, _ = json.Marshal(raw)
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSvcQualificationKeepsRecordedSegmentFailures(t *testing.T) {
	for _, failure := range []string{"equality", "route", "blocked-route", "query-equality"} {
		for position := range 3 {
			t.Run(fmt.Sprintf("%s/%d", failure, position), func(t *testing.T) {
				c := svcSegmentCase(t, failure, position)
				x := &replayEnv{svcFormat: SvcLegacyFormat, errorTreeRoute: true, seen: map[string]map[string]bool{}}
				out := x.replayCase(&c, 0, nil, failure == "query-equality")
				for _, g := range x.gates {
					if g.Failed > 0 {
						t.Fatalf("valid recorded failure rejected: %+v", g)
					}
				}
				if out.assess != c.Assessment {
					t.Fatalf("replay=%s recorded=%s", out.assess, c.Assessment)
				}
				s := &qset{kinds: map[string]map[string]string{}, checks: map[string]string{}, summaries: map[string]string{}}
				qc := &QualCase{ID: c.ID, Expect: []StepExpectation{{Step: 2, Syntax: "ERROR"}}}
				kitFailed := false
				q := &qualifier{}
				q.judgeCase(s, qc, nil, &c, &qRecord{}, out, func(code, _, _ string) { kitFailed = kitFailed || code == "KIT_CLAIM_FAILED" }, "synthetic", true, false)
				if s.checks[c.ID] != c.Assessment || kitFailed != (failure != "blocked-route") {
					t.Fatalf("failure hidden: check=%s kitFailed=%v", s.checks[c.ID], kitFailed)
				}
			})
		}
	}
}

func TestSvcReplayRejectsSegmentClaimTampering(t *testing.T) {
	for _, bad := range []string{"missing", "null", "false-pass", "unknown", "partial"} {
		t.Run(bad, func(t *testing.T) {
			c := svcSegmentCase(t, "equality", 1)
			data, _ := json.Marshal(c)
			var raw map[string]any
			_ = json.Unmarshal(data, &raw)
			seg := raw["segments"].([]any)[1].(map[string]any)
			switch bad {
			case "missing":
				delete(seg, "claims")
			case "null":
				seg["claims"] = nil
			default:
				claims := map[string]any{"incremental_equality": claimFail, "incremental_route": claimPass, "expectations": claimNotClaimed}
				seg["claims"] = claims
				if bad == "false-pass" {
					claims["incremental_equality"] = claimPass
				} else if bad == "unknown" {
					claims["incremental_equality"] = "UNKNOWN"
				} else {
					delete(claims, "expectations")
				}
			}
			data, _ = json.Marshal(raw)
			c = rcCase{}
			_ = json.Unmarshal(data, &c)
			x := &replayEnv{svcFormat: SvcLegacyFormat, errorTreeRoute: true, seen: map[string]map[string]bool{}}
			x.replayCase(&c, 0, nil, false)
			failed := false
			for _, g := range x.gates {
				failed = failed || g.Failed > 0
			}
			if !failed {
				t.Fatal("tampered segment claim accepted")
			}
		})
	}
}

func TestSvcSegmentExpectationsAndSingletons(t *testing.T) {
	for _, variant := range []string{"duplicate", "gap-only", "diagnostics", "implicit-gap", "unobserved-gap", "singleton", "no-gap", "unexpected-no-gap-segment"} {
		t.Run(variant, func(t *testing.T) {
			c := svcSegmentCase(t, "", 0)
			want := AssessPass
			switch variant {
			case "duplicate":
				c.Expectations = append([]rcExpect{{Step: 0, Syntax: "NO_ERROR", Result: claimPass}, {Step: 0, Syntax: "ERROR", Result: claimFail}}, c.Expectations...)
				c.Segments[0].Claims.Expectations = claimFail
				c.Claims.Expectations, c.Assessment, want = claimFail, AssessFail, AssessFail
			case "gap-only":
				c.Expectations = []rcExpect{{Step: 2, Syntax: "ANY", Contains: []string{"class_declaration"}, Result: claimBlocked}}
				c.Claims.Expectations, c.Assessment, want = claimBlocked, AssessBlocked, AssessBlocked
			case "diagnostics":
				c.Steps[0].Composite.Directive.Diagnostics = []string{"ATTRIBUTE_UNKNOWN"}
				c.Segments[0].Claims.Expectations = claimBlocked
				c.Assessment, want = AssessBlocked, AssessBlocked
			case "implicit-gap", "unobserved-gap":
				c.Expectations = nil
				c.Claims.Expectations, c.Assessment, want = claimBlocked, AssessBlocked, AssessBlocked
				if variant == "unobserved-gap" {
					c.Claims.Expectations = claimNotClaimed
					for _, i := range []int{2, 5} {
						c.Steps[i].Composite = &rcComposite{Directive: &SvcDirective{Close: &Span{}}, Coverage: SvcCoverage{Inline: "UNRESOLVED"}}
					}
				}
			case "singleton":
				c.Steps = []rcStep{c.Steps[0], c.Steps[2]}
				c.Steps[1].Step = 1
				c.Expectations[0].Step = 1
				c.Segments = c.Segments[:1]
				c.Segments[0].EndStep = 1
				*c.Segments[0].Claims = rcClaims{claimNotClaimed, claimNotClaimed, claimNotClaimed}
			case "no-gap", "unexpected-no-gap-segment":
				c.Steps, c.Expectations = c.Steps[:1], nil
				if variant == "no-gap" {
					c.Segments = nil
				}
				c.Claims.Expectations = claimNotClaimed
			}
			x := &replayEnv{svcFormat: SvcLegacyFormat, errorTreeRoute: true, seen: map[string]map[string]bool{}}
			out := x.replayCase(&c, 0, nil, false)
			if variant == "unexpected-no-gap-segment" {
				if x.gate("case-binding").Code != "SVC_SEGMENT_MISMATCH" {
					t.Fatal("unexpected single segment accepted")
				}
				return
			}
			for _, g := range x.gates {
				if g.Failed > 0 {
					t.Fatalf("valid boundary rejected: %+v", g)
				}
			}
			if out.assess != want || len(c.Segments) > 0 && !out.segmentClaimsValid {
				t.Fatalf("%+v", out)
			}
			s := &qset{kinds: map[string]map[string]string{}, checks: map[string]string{}, summaries: map[string]string{}}
			qc := &QualCase{ID: c.ID}
			for _, e := range c.Expectations {
				qc.Expect = append(qc.Expect, StepExpectation{Step: e.Step, Syntax: e.Syntax, Contains: e.Contains})
			}
			q := &qualifier{}
			q.judgeCase(s, qc, nil, &c, &qRecord{}, out, func(_, _, _ string) {}, "synthetic", true, false)
			if s.checks[c.ID] != want {
				t.Fatalf("check=%s want=%s", s.checks[c.ID], want)
			}
			if c.Claims.IncrementalEquality != claimNotClaimed || c.Claims.IncrementalRoute != claimNotClaimed {
				t.Fatal("whole-history claim changed")
			}
		})
	}
}

func TestSvcObservationOnlyRejectsUnexpectedSegments(t *testing.T) {
	for _, format := range []string{SvcFormat, SvcLegacyFormat, ""} {
		x := &replayEnv{svcFormat: format, seen: map[string]map[string]bool{}}
		source := []byte(`<%@ ServiceHost Service="S" %>`)
		o := ObserveServiceHost(EncodingUTF8, source)
		c := rcCase{ID: "synthetic", ExecutionStatus: StatusCompleted, Assessment: AssessPass,
			Input: NativeInput{Bytes: uint64(len(source)), SHA256: digestHex(source)},
			Steps: []rcStep{{SourceBytes: uint64(len(source)), SourceSHA256: digestHex(source), Composite: &rcComposite{
				Schema: SvcCompositeSchema, Format: SvcFormat, Input: rcInput{Bytes: uint64(len(source)), SHA256: digestHex(source), Encoding: EncodingUTF8},
				Directive: o.Directive, Language: o.Language, Coverage: o.Coverage,
				Identities: []IdentityRef{{"producer", "tsgk-native-build/r1", fxProducer}, {"source", "tsgk-source-bytes/r1", digestHex(source)}, {"policy", "tsgk-native-policy/r1", fxPolicy}},
			}}}, Segments: []rcSvcSegment{{StartStep: 0, EndStep: 1}}}
		c.Claims = rcClaims{claimNotClaimed, claimNotClaimed, claimNotClaimed}
		x.replayCase(&c, 0, &IncrementalCase{ID: c.ID, Input: c.Input, SvcSource: source, Encoding: EncodingUTF8}, false)
		code := "SVC_SEGMENT_MISMATCH"
		if format == "" {
			code = "SVC_SEGMENT_UNEXPECTED"
		}
		if x.gate("case-binding").Code != code {
			t.Fatalf("unexpected segment accepted: %s", format)
		}
	}
}
