package kit

import (
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"slices"
	"strings"
	"testing"
)

func witnessFixture(t *testing.T) (QueryWitnessRequest, map[string]any, map[string]any, map[string]any) {
	t.Helper()
	pred := []any{map[string]any{"pattern": 0, "steps": []any{
		map[string]any{"kind": "string", "value": "eq?", "quantifier": nil},
		map[string]any{"kind": "capture", "value": "x", "quantifier": "ONE"},
		map[string]any{"kind": "string", "value": "a", "quantifier": nil},
	}}}
	capture := Capture{Match: 0, Pattern: 0, Capture: 0, Name: "x", Node: 1, Type: "word", Named: true, EndByte: 1, EndPoint: Point{Column: 1}}
	input := NativeInput{Path: "case.txt", Role: "case", Bytes: 1, SHA256: sum([]byte("a"))}
	edit := Edit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, Old: []byte("a"), New: []byte("b")}
	pointEdit := map[string]any{"start_byte": 0, "old_end_byte": 1, "new_end_byte": 1,
		"start_point": []int{0, 0}, "old_end_point": []int{0, 1}, "new_end_point": []int{0, 1}}
	reg := map[string]any{"schema": QueryWitnessRegistrationSchema, "policy": "tsgk-predicates/r1", "case": "c1", "route": "owned-plain", "platform": "windows/amd64", "role": "owned",
		"encoding": EncodingUTF8, "input": input, "edits": []Edit{edit},
		"queries": []any{map[string]any{"id": "q", "source": `((word) @x (#eq? @x "a"))`, "metadata": map[string]any{"patterns": 1, "capture_names": []string{"x"}, "predicates": pred}}},
		"required": []any{
			map[string]any{"query": "q", "step": 0, "tree_role": "incremental", "structural": []Capture{capture}, "expected": []Capture{capture}},
			map[string]any{"query": "q", "step": 1, "tree_role": "incremental", "structural": []Capture{capture}, "expected": []Capture{}},
			map[string]any{"query": "q", "step": 1, "tree_role": "fresh", "structural": []Capture{capture}, "expected": []Capture{}},
		}}
	query := func() map[string]any {
		return map[string]any{"id": "q", "status": StatusCompleted, "code": "", "query_ms": 0, "error": nil,
			"patterns": 1, "capture_names": []string{"x"}, "predicates": pred, "matches": 1, "partial": false,
			"types": []string{"word"}, "captures": [][]int64{{0, 0, 0, 1, 0, 1, 0, 1, 0, 0, 0, 1}}}
	}
	rawTree := func(src string) map[string]any {
		return map[string]any{"status": StatusCompleted, "code": "", "form": "full", "descendant_count": 2, "has_error": false,
			"digest": TreeDigest(fxTree(src, false)), "types": []string{"doc", "word"}, "fields": []string{},
			"nodes": [][]int64{{-1, 0, -1, 1, 0, 1, 0, 0, 0, 1}, {0, 1, -1, 1, 0, 1, 0, 0, 0, 1}}, "queries": []any{query()}}
	}
	projectedTree := func(src string) map[string]any {
		caps := []Capture{}
		if src == "a" {
			caps = append(caps, capture)
		}
		return map[string]any{"status": StatusCompleted, "code": "", "form": "full", "descendant_count": 2, "has_error": false,
			"digest": TreeDigest(fxTree(src, false)), "tree": map[string]any{"schema": TreeSchema, "input": map[string]any{"encoding": EncodingUTF8, "bytes": 1, "sha256": sum([]byte(src))}, "status": StatusCompleted, "nodes": fxTree(src, false)},
			"queries": []any{map[string]any{"id": "q", "sha256": sum([]byte(`((word) @x (#eq? @x "a"))`)), "status": StatusCompleted, "code": "", "evaluation": "EVALUATED", "query_ms": 0, "error": nil,
				"patterns": 1, "capture_names": []string{"x"}, "predicates": pred, "matches": 1, "partial": false, "captures": caps}}}
	}
	rawSteps, projectedSteps := []any{}, []any{}
	for i, src := range []string{"a", "b"} {
		var e, fresh, projectedFresh any
		if i > 0 {
			e, fresh, projectedFresh = pointEdit, rawTree(src), projectedTree(src)
		}
		rawSteps = append(rawSteps, map[string]any{"step": i, "source_bytes": 1, "source_sha256": sum([]byte(src)), "edit": e, "incremental": rawTree(src), "fresh": fresh})
		projectedSteps = append(projectedSteps, map[string]any{"step": i, "source_bytes": 1, "source_sha256": sum([]byte(src)), "edit": e, "incremental": projectedTree(src), "fresh": projectedFresh})
	}
	raw := map[string]any{"protocol": "tsgk-native/r2", "id": "c1", "status": StatusCompleted, "code": "", "source_bytes": 1, "steps_completed": 2, "complete": true, "steps": rawSteps}
	projection := map[string]any{"id": "c1", "input": input, "encoding": EncodingUTF8, "execution_status": StatusCompleted, "code": "", "steps": projectedSteps}
	return QueryWitnessRequest{Source: []byte("a")}, reg, raw, projection
}

func marshalWitness(t *testing.T, req QueryWitnessRequest, reg, raw, projection map[string]any) QueryWitnessRequest {
	t.Helper()
	var err error
	if req.Registration, err = json.Marshal(reg); err != nil {
		t.Fatal(err)
	}
	if req.Raw, err = json.Marshal(raw); err != nil {
		t.Fatal(err)
	}
	if req.Projection, err = json.Marshal(projection); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestQueryWitnessRecomputesInitialAndEdit(t *testing.T) {
	req, reg, raw, projection := witnessFixture(t)
	r, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, projection))
	if err != nil || r.Assessment != AssessPass || r.Required != 3 || r.Verified != 3 || r.Evaluated != 3 {
		t.Fatalf("witness: %+v %v", r, err)
	}
}

func TestQueryWitnessRejectsContradictions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*QueryWitnessRequest, map[string]any, map[string]any, map[string]any)
	}{
		{"source", func(req *QueryWitnessRequest, _, _, _ map[string]any) { req.Source = []byte("b") }},
		{"raw-step-source", func(_ *QueryWitnessRequest, _, raw, _ map[string]any) {
			raw["steps"].([]any)[1].(map[string]any)["source_sha256"] = sum([]byte("a"))
		}},
		{"projection-step-source", func(_ *QueryWitnessRequest, _, _, p map[string]any) {
			p["steps"].([]any)[1].(map[string]any)["source_sha256"] = sum([]byte("a"))
		}},
		{"raw-null-scalar", func(_ *QueryWitnessRequest, _, raw, _ map[string]any) {
			raw["steps"].([]any)[0].(map[string]any)["step"] = nil
		}},
		{"projection-null-scalar", func(_ *QueryWitnessRequest, _, _, p map[string]any) {
			p["steps"].([]any)[0].(map[string]any)["step"] = nil
		}},
		{"raw-field-table", func(_ *QueryWitnessRequest, _, raw, _ map[string]any) {
			delete(raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any), "fields")
		}},
		{"raw-field-index", func(_ *QueryWitnessRequest, _, raw, _ map[string]any) {
			raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["nodes"].([][]int64)[1][2] = -2
		}},
		{"raw-empty-rejected-stream", func(_ *QueryWitnessRequest, _, raw, _ map[string]any) {
			q := raw["steps"].([]any)[1].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
			q["captures"], q["matches"] = [][]int64{}, 0
		}},
		{"raw-predicate-deleted", func(_ *QueryWitnessRequest, _, raw, _ map[string]any) {
			raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)["predicates"] = []any{}
		}},
		{"projection-partial", func(_ *QueryWitnessRequest, _, _, p map[string]any) {
			p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)["partial"] = true
		}},
		{"projection-code", func(_ *QueryWitnessRequest, _, _, p map[string]any) {
			p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)["code"] = "QUERY_TIME_LIMIT"
		}},
		{"projection-null-captures", func(_ *QueryWitnessRequest, _, _, p map[string]any) {
			p["steps"].([]any)[1].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)["captures"] = nil
		}},
		{"projection-missing-zero", func(_ *QueryWitnessRequest, _, _, p map[string]any) {
			delete(p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any), "partial")
		}},
		{"raw-completion-code", func(_ *QueryWitnessRequest, _, raw, _ map[string]any) { raw["code"] = "QUERY_TIME_LIMIT" }},
		{"raw-step-duplicate", func(_ *QueryWitnessRequest, _, raw, _ map[string]any) {
			raw["steps"].([]any)[1].(map[string]any)["step"] = 0
		}},
		{"projection-step-duplicate", func(_ *QueryWitnessRequest, _, _, p map[string]any) {
			p["steps"].([]any)[1].(map[string]any)["step"] = 0
		}},
		{"projection-stale-filter", func(_ *QueryWitnessRequest, _, _, p map[string]any) {
			p["steps"].([]any)[1].(map[string]any)["incremental"] = p["steps"].([]any)[0].(map[string]any)["incremental"]
		}},
		{"projection-query-hash", func(_ *QueryWitnessRequest, _, _, p map[string]any) {
			p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)["sha256"] = fxPolicy
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, reg, raw, projection := witnessFixture(t)
			tc.mutate(&req, reg, raw, projection)
			r, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, projection))
			if err != nil || r.Assessment != AssessFail || len(r.Findings) == 0 {
				t.Fatalf("accepted contradiction: %+v %v", r, err)
			}
		})
	}
}

func TestQueryWitnessMissingEvidenceAndFailPrecedence(t *testing.T) {
	for _, missing := range []string{"source", "raw", "projection", "structural", "expected", "metadata", "denominator"} {
		t.Run(missing, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			switch missing {
			case "structural", "expected":
				for _, x := range reg["required"].([]any) {
					x.(map[string]any)[missing] = nil
				}
			case "metadata":
				reg["queries"].([]any)[0].(map[string]any)["metadata"] = nil
			case "denominator":
				reg["required"] = []any{}
			}
			req = marshalWitness(t, req, reg, raw, p)
			switch missing {
			case "source":
				req.Source = nil
			case "raw":
				req.Raw = nil
			case "projection":
				req.Projection = nil
			}
			r, err := VerifyQueryWitness(ctxT(t), req)
			if err != nil || r.Assessment != AssessBlocked {
				t.Fatalf("missing evidence: %+v %v", r, err)
			}
			if missing == "expected" {
				raw["code"] = "FORGED"
				req = marshalWitness(t, req, reg, raw, p)
				r, err = VerifyQueryWitness(ctxT(t), req)
				if err != nil || r.Assessment != AssessFail {
					t.Fatalf("missing hid contradiction: %+v %v", r, err)
				}
			}
		})
	}
}

func TestQueryWitnessReviewChecksUnrequiredFullTrees(t *testing.T) {
	for _, kind := range []string{"intact", "raw-nodes", "projection-nodes", "projection-only"} {
		t.Run(kind, func(t *testing.T) {
			req, reg, raw, projection := witnessFixture(t)
			reg["required"] = reg["required"].([]any)[:1]
			if kind == "raw-nodes" {
				raw["steps"].([]any)[1].(map[string]any)["fresh"].(map[string]any)["nodes"].([][]int64)[1][2] = -2
			}
			if kind == "projection-nodes" || kind == "projection-only" {
				projection["steps"].([]any)[1].(map[string]any)["fresh"].(map[string]any)["tree"].(map[string]any)["nodes"].([]TreeNode)[1].Type = "FORGED"
			}
			req = marshalWitness(t, req, reg, raw, projection)
			if kind == "projection-only" {
				req.Raw = nil
			}
			want := AssessFail
			if kind == "intact" {
				want = AssessPass
			}
			res, err := VerifyQueryWitness(ctxT(t), req)
			if err != nil || res.Assessment != want || res.Required != 1 {
				t.Fatalf("unrequired full tree: %+v %v", res, err)
			}
		})
	}
}

func TestQueryWitnessReviewProjectionEncodingProvenance(t *testing.T) {
	for _, value := range []any{SourceDeclaration, "", "FORGED", nil} {
		req, reg, raw, projection := witnessFixture(t)
		projection["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["tree"].(map[string]any)["input"].(map[string]any)["encoding_source"] = value
		res, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, projection))
		want := AssessFail
		if value == SourceDeclaration {
			want = AssessPass
		}
		if err != nil || res.Assessment != want {
			t.Fatalf("encoding_source=%v: %+v %v", value, res, err)
		}
	}
}

func TestQueryWitnessReviewMissingRawRejectsFailedCode(t *testing.T) {
	req, reg, raw, projection := witnessFixture(t)
	q := projection["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
	q["status"], q["code"], q["evaluation"], q["captures"] = StatusFailed, "FORGED", "UNSUPPORTED", nil
	req = marshalWitness(t, req, reg, raw, projection)
	req.Raw = nil
	res, err := VerifyQueryWitness(ctxT(t), req)
	if err != nil || res.Assessment != AssessFail {
		t.Fatalf("missing raw hid invalid failed code: %+v %v", res, err)
	}
}

func TestQueryWitnessReviewMissingSourceChecksStepPairs(t *testing.T) {
	for _, field := range []string{"source_sha256", "edit-point"} {
		req, reg, raw, projection := witnessFixture(t)
		step := projection["steps"].([]any)[1].(map[string]any)
		if field == "source_sha256" {
			step[field] = fxPolicy
		} else {
			point := step["edit"].(map[string]any)
			// Copy: the fixture shares this edit object with raw.
			copy := map[string]any{}
			for key, value := range point {
				copy[key] = value
			}
			copy["start_point"] = []int{1, 0}
			step["edit"] = copy
		}
		req = marshalWitness(t, req, reg, raw, projection)
		req.Source = nil
		res, err := VerifyQueryWitness(ctxT(t), req)
		if err != nil || res.Assessment != AssessFail {
			t.Fatalf("missing source hid %s contradiction: %+v %v", field, res, err)
		}
	}
}

func TestQueryWitnessReviewIncompleteStepCount(t *testing.T) {
	for _, kind := range []string{"query-limit", "query-failed", "tree-limit"} {
		t.Run(kind, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			rawStep := raw["steps"].([]any)[1].(map[string]any)
			step := p["steps"].([]any)[1].(map[string]any)
			rawTree := rawStep["incremental"].(map[string]any)
			tree := step["incremental"].(map[string]any)
			status, code := StatusResourceLimit, "QUERY_TIME_LIMIT"
			q := rawTree["queries"].([]any)[0].(map[string]any)
			pq := tree["queries"].([]any)[0].(map[string]any)
			q["partial"], pq["partial"] = true, true
			pq["captures"], pq["evaluation"] = nil, "UNSUPPORTED"
			if kind == "query-failed" {
				status, code = StatusFailed, "CAPTURE_NODE_UNMAPPED"
				q["partial"], pq["partial"], q["captures"], pq["evaluation"] = false, false, nil, ""
			}
			q["status"], q["code"], pq["status"], pq["code"] = status, code, status, code
			if kind == "tree-limit" {
				code = "NODE_LIMIT"
				for _, key := range []string{"form", "types", "fields", "nodes", "queries"} {
					rawTree[key] = nil
				}
				rawTree["status"], rawTree["code"], rawTree["descendant_count"], rawTree["digest"] = status, code, 0, ""
				tree["status"], tree["code"], tree["descendant_count"], tree["digest"], tree["form"], tree["tree"] = status, code, 0, "", "", nil
				pq["status"], pq["code"], pq["evaluation"], pq["partial"], pq["patterns"], pq["matches"] = StatusNotRun, "TREE_NOT_COMPLETED", "", false, 0, 0
				pq["capture_names"], pq["predicates"] = []string{}, []any{}
			}
			raw["status"], raw["code"], raw["steps_completed"] = status, code, 1
			p["execution_status"], p["code"] = status, code
			rawStep["fresh"], step["fresh"] = nil, nil
			res, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
			if err != nil || res.Assessment != AssessBlocked || res.Verified != 1 {
				t.Fatalf("consistent incomplete native step: %+v %v", res, err)
			}
		})
	}
}

func TestQueryWitnessReviewTreeRolePlacement(t *testing.T) {
	for _, kind := range []string{"initial-fresh", "edit-fresh-missing", "projection-initial-fresh"} {
		req, reg, raw, p := witnessFixture(t)
		reg["required"] = reg["required"].([]any)[:1]
		rawSteps, steps := raw["steps"].([]any), p["steps"].([]any)
		if kind == "edit-fresh-missing" {
			rawSteps[1].(map[string]any)["fresh"], steps[1].(map[string]any)["fresh"] = nil, nil
		} else {
			steps[0].(map[string]any)["fresh"] = steps[0].(map[string]any)["incremental"]
			if kind == "initial-fresh" {
				rawSteps[0].(map[string]any)["fresh"] = rawSteps[0].(map[string]any)["incremental"]
			}
		}
		res, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
		if err != nil || res.Assessment != AssessFail {
			t.Fatalf("%s: %+v %v", kind, res, err)
		}
	}
}

func TestQueryWitnessReviewProjectionStatusPair(t *testing.T) {
	for _, status := range []string{StatusFailed, StatusResourceLimit, StatusCancelled} {
		req, reg, raw, p := witnessFixture(t)
		p["execution_status"] = status
		res, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
		if err != nil || res.Assessment != AssessFail {
			t.Fatalf("projection status %s contradicts completed raw: %+v %v", status, res, err)
		}
	}
}

func TestQueryWitnessReviewNoncompleteNeedsIncompleteTree(t *testing.T) {
	for _, status := range []string{StatusFailed, StatusResourceLimit} {
		req, reg, raw, p := witnessFixture(t)
		raw["status"], raw["code"] = status, "NODE_LIMIT"
		p["execution_status"], p["code"] = status, "NODE_LIMIT"
		res, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
		if err != nil || res.Assessment != AssessFail {
			t.Fatalf("all retained steps completed: %+v %v", res, err)
		}
	}
}

func TestQueryWitnessReviewCapabilityRefusal(t *testing.T) {
	for _, code := range []string{"QUERY_CAPABILITY_MISSING", "API_CAPABILITY_MISSING", "OTHER"} {
		for _, retainSteps := range []bool{false, true} {
			req, reg, raw, p := witnessFixture(t)
			p["execution_status"], p["code"] = StatusNotRun, code
			if !retainSteps {
				p["steps"] = []any{}
			}
			res, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
			want := AssessFail
			if code != "OTHER" && !retainSteps {
				want = AssessBlocked
			}
			if err != nil || res.Assessment != want || res.Verified != 0 {
				t.Fatalf("%s retain=%v: %+v %v", code, retainSteps, res, err)
			}
		}
	}
}

func TestQueryWitnessReviewDroppedQueryTimeout(t *testing.T) {
	for _, status := range []string{StatusFailed, StatusResourceLimit} {
		for _, queries := range []bool{false, true} {
			req, reg, raw, p := witnessFixture(t)
			raw["status"], raw["code"], raw["steps"], raw["steps_completed"] = status, "QUERY_TIME_LIMIT", []any{}, 0
			if !queries {
				reg["queries"], reg["required"] = []any{}, []any{}
			}
			req = marshalWitness(t, req, reg, raw, p)
			req.Projection = nil
			res, err := VerifyQueryWitness(ctxT(t), req)
			want := AssessFail
			if status == StatusResourceLimit && queries {
				want = AssessBlocked
			}
			if err != nil || res.Assessment != want {
				t.Fatalf("%s queries=%v: %+v %v", status, queries, res, err)
			}
		}
	}
}

func TestQueryWitnessRegistrationAndCancellation(t *testing.T) {
	req, reg, raw, p := witnessFixture(t)
	reg["required"] = append(reg["required"].([]any), reg["required"].([]any)[0])
	if _, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p)); err == nil {
		t.Fatal("duplicate trusted tuple accepted")
	}
	req, reg, raw, p = witnessFixture(t)
	ctx, cancel := context.WithCancel(ctxT(t))
	cancel()
	if _, err := VerifyQueryWitness(ctx, marshalWitness(t, req, reg, raw, p)); err == nil {
		t.Fatal("cancelled request accepted")
	}
}

func TestQueryWitnessTypedAnchorRoundTrip(t *testing.T) {
	req, reg, raw, p := witnessFixture(t)
	req = marshalWitness(t, req, reg, raw, p)
	var registration QueryWitnessRegistration
	if err := jsonv2.Unmarshal(req.Registration, &registration); err != nil {
		t.Fatal(err)
	}
	registration.Required[0].Structural = nil
	b, err := jsonv2.Marshal(registration)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip QueryWitnessRegistration
	if err := jsonv2.Unmarshal(b, &roundTrip); err != nil || roundTrip.Required[0].Structural != nil || roundTrip.Required[1].Expected == nil {
		t.Fatalf("lost missing/empty anchor distinction: %s %v", b, err)
	}
}

func TestQueryWitnessMissingSourceStillChecksEnvelope(t *testing.T) {
	req, reg, raw, p := witnessFixture(t)
	raw["code"] = "FORGED"
	req = marshalWitness(t, req, reg, raw, p)
	req.Source = nil
	r, err := VerifyQueryWitness(ctxT(t), req)
	if err != nil || r.Assessment != AssessFail {
		t.Fatalf("missing source hid contradiction: %+v %v", r, err)
	}
}

func TestQueryWitnessMissingEvidenceStillChecksPassthrough(t *testing.T) {
	for _, missing := range []string{"source", "projection-tree"} {
		for _, tamper := range []bool{false, true} {
			t.Run(missing+map[bool]string{false: "/intact", true: "/tampered"}[tamper], func(t *testing.T) {
				req, reg, raw, p := witnessFixture(t)
				tree := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
				if missing == "source" {
					req.Source = nil
				} else {
					tree["tree"] = nil
				}
				want := AssessBlocked
				if tamper {
					tree["queries"].([]any)[0].(map[string]any)["matches"] = 2
					want = AssessFail
				}
				r, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
				if err != nil || r.Assessment != want || (tamper && !slices.Contains(codes(r.Findings), "QUERY_WITNESS_QUERY_PASSTHROUGH_MISMATCH")) {
					t.Fatalf("available evidence comparison: %+v %v", r, err)
				}
			})
		}
	}
}

func TestQueryWitnessFailedQueryMetadataNormalization(t *testing.T) {
	req, reg, raw, p := witnessFixture(t)
	raw["status"], raw["code"], raw["steps_completed"] = StatusFailed, "CAPTURE_NODE_UNMAPPED", 0
	p["execution_status"], p["code"] = StatusFailed, "CAPTURE_NODE_UNMAPPED"
	raw["steps"], p["steps"] = raw["steps"].([]any)[:1], p["steps"].([]any)[:1]
	reg["queries"].([]any)[0].(map[string]any)["metadata"] = nil
	q := raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
	q["status"], q["code"], q["captures"], q["matches"] = StatusFailed, "CAPTURE_NODE_UNMAPPED", nil, 0
	q["predicates"], q["capture_names"], q["patterns"], q["types"] = nil, nil, 0, nil
	pq := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
	pq["status"], pq["code"], pq["captures"], pq["matches"] = StatusFailed, "CAPTURE_NODE_UNMAPPED", nil, 0
	pq["predicates"], pq["capture_names"], pq["patterns"], pq["evaluation"] = []any{}, nil, 0, ""
	tree := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
	fullTree := tree["tree"]
	for _, missing := range []string{"none", "source", "projection-tree"} {
		for _, tamper := range []bool{false, true} {
			t.Run(missing+map[bool]string{false: "/intact", true: "/tampered"}[tamper], func(t *testing.T) {
				runReq := req
				if missing == "source" {
					runReq.Source = nil
				}
				tree["tree"] = fullTree
				if missing == "projection-tree" {
					tree["tree"] = nil
				}
				want := AssessBlocked
				pq["code"] = "CAPTURE_NODE_UNMAPPED"
				if tamper {
					pq["code"], want = "OTHER", AssessFail
				}
				r, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, runReq, reg, raw, p))
				if err != nil || r.Assessment != want {
					t.Fatalf("failed-query passthrough: %+v %v", r, err)
				}
			})
		}
	}
}

func TestQueryWitnessMissingEvidenceRejectsImpossibleTransition(t *testing.T) {
	for _, missing := range []string{"source", "projection-tree"} {
		t.Run(missing, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			tree := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			if missing == "source" {
				req.Source = nil
			} else {
				tree["tree"] = nil
			}
			q := tree["queries"].([]any)[0].(map[string]any)
			q["status"], q["code"], q["evaluation"], q["captures"] = StatusFailed, "CAPTURE_NODE_UNMAPPED", "", nil
			r, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
			if err != nil || r.Assessment != AssessFail || !slices.Contains(codes(r.Findings), "QUERY_WITNESS_QUERY_PASSTHROUGH_MISMATCH") {
				t.Fatalf("impossible state hidden by missing evidence: %+v %v", r, err)
			}
		})
	}
}

func TestQueryWitnessPartialIsIneligible(t *testing.T) {
	req, reg, raw, p := witnessFixture(t)
	raw["status"], raw["code"], raw["steps_completed"] = StatusResourceLimit, "CAPTURE_LIMIT", 0
	p["execution_status"], p["code"] = StatusResourceLimit, "CAPTURE_LIMIT"
	raw["steps"], p["steps"] = raw["steps"].([]any)[:1], p["steps"].([]any)[:1]
	q := raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
	q["status"], q["code"], q["partial"] = StatusResourceLimit, "CAPTURE_LIMIT", true
	q["captures"], q["matches"] = [][]int64{}, 0
	pq := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
	pq["status"], pq["code"], pq["partial"] = StatusResourceLimit, "CAPTURE_LIMIT", true
	pq["evaluation"], pq["captures"], pq["matches"] = "UNSUPPORTED", nil, 0
	r, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
	if err != nil || r.Assessment != AssessBlocked {
		t.Fatalf("partial stream treated as complete contradiction: %+v %v", r, err)
	}
}

func TestQueryWitnessMissingRawStillChecksQueryIdentity(t *testing.T) {
	req, reg, raw, p := witnessFixture(t)
	p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)["sha256"] = fxPolicy
	req = marshalWitness(t, req, reg, raw, p)
	req.Raw = nil
	r, err := VerifyQueryWitness(ctxT(t), req)
	if err != nil || r.Assessment != AssessFail {
		t.Fatalf("missing raw hid query identity contradiction: %+v %v", r, err)
	}
}

func TestQueryWitnessCountsOnlyVerifiedTuples(t *testing.T) {
	req, reg, raw, p := witnessFixture(t)
	reg["required"].([]any)[0].(map[string]any)["expected"] = []Capture{}
	r, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
	if err != nil || r.Assessment != AssessFail || r.Verified != 2 || r.Evaluated != 2 {
		t.Fatalf("failed tuple counted as verified: %+v %v", r, err)
	}
}

func TestQueryWitnessIndependentBindingFailures(t *testing.T) {
	for _, kind := range []string{"raw-sha", "raw-bytes", "raw-edit", "projection-sha", "projection-bytes", "projection-edit", "unsupported-captures", "raw-tree-status"} {
		t.Run(kind, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			s := raw["steps"].([]any)[0].(map[string]any)
			if len(kind) >= 10 && kind[:10] == "projection" {
				s = p["steps"].([]any)[0].(map[string]any)
			}
			switch kind {
			case "raw-sha", "projection-sha":
				s["source_sha256"] = fxPolicy
			case "raw-bytes", "projection-bytes":
				s["source_bytes"] = 2
			case "raw-edit", "projection-edit":
				s["edit"] = raw["steps"].([]any)[1].(map[string]any)["edit"]
			case "raw-tree-status":
				s["incremental"].(map[string]any)["status"] = "BOGUS"
			case "unsupported-captures":
				p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)["status"] = "UNSUPPORTED"
			}
			req = marshalWitness(t, req, reg, raw, p)
			if kind == "unsupported-captures" {
				req.Raw = nil
			} else if kind != "raw-tree-status" {
				req.Source = nil
			}
			r, err := VerifyQueryWitness(ctxT(t), req)
			if err != nil || r.Assessment != AssessFail {
				t.Fatalf("independent contradiction hidden: %+v %v", r, err)
			}
		})
	}
}

func TestQueryWitnessStepBindingExcludesOnlyAffectedTuple(t *testing.T) {
	req, reg, raw, p := witnessFixture(t)
	raw["steps"].([]any)[0].(map[string]any)["source_sha256"] = fxPolicy
	r, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
	if err != nil || r.Assessment != AssessFail || r.Verified != 2 || r.Evaluated != 2 {
		t.Fatalf("invalid step binding counted: %+v %v", r, err)
	}
}

func TestReplayQueryWitness(t *testing.T) {
	for _, kind := range []string{"complete", "blocked", "forged-projection", "stale-platform", "wrong-subject", "wrong-records", "old-reducer", "unlisted", "registration-value-limit", "registration-malformed", "empty-source-mismatch"} {
		t.Run(kind, func(t *testing.T) {
			req, witness, raw, projection := witnessFixture(t)
			if kind == "blocked" {
				witness["required"].([]any)[0].(map[string]any)["expected"] = nil
			}
			if kind == "forged-projection" {
				projection["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)["sha256"] = fxPolicy
			}
			req = marshalWitness(t, req, witness, raw, projection)
			if kind == "registration-value-limit" {
				req.Registration = []byte(`{"required":[` + strings.Repeat("0,", 2000000) + `0]}`)
			}
			if kind == "registration-malformed" {
				req.Registration = []byte("{")
			}
			if kind == "empty-source-mismatch" {
				req.Source = []byte{}
			}
			root := t.TempDir()
			files := map[string][]byte{"registration.json": req.Registration, "case.txt": req.Source, "raw.json": req.Raw, "projection.json": req.Projection}
			roles := map[string]string{"registration.json": "query-registration", "case.txt": "case-source", "raw.json": "raw", "projection.json": "projection"}
			members := writeSet(t, root, files, func(p string) string { return roles[p] })
			ids := map[string]string{"registration": sum(req.Registration), "source": sum(req.Source), "route": "owned-plain", "platform": "windows/amd64", "role": "owned", "policy": "tsgk-predicates/r1"}
			reg := registration("query-witness-r1", "evidence-replay", AssessPass, ids, []string{"c1"}, members)
			switch kind {
			case "stale-platform":
				ids["platform"] = "linux/amd64"
			case "wrong-subject":
				reg["subject"].(map[string]any)["platform"] = "linux/amd64"
			case "wrong-records":
				reg["records"] = []string{"other"}
			case "old-reducer":
				reg["reducer"] = "oracle-set-r2"
			case "unlisted":
				writeSet(t, root, map[string][]byte{"extra.json": []byte("{}")}, func(string) string { return "retained" })
			}
			if kind == "registration-value-limit" {
				profile, err := json.Marshal(reg)
				if err != nil {
					t.Fatal(err)
				}
				r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: profile})
				e, ok := err.(*Error)
				if !ok || e.Kind != KindResourceLimit || e.Code != "JSON_VALUE_LIMIT" || r.ExecutionStatus != StatusResourceLimit {
					t.Fatalf("registration resource kind lost: %+v %v", r, err)
				}
				return
			}
			r := runReg(t, root, reg)
			switch kind {
			case "complete":
				if r.Assessment != AssessPass || !r.EvidenceValid || r.Consumption.Consumed != 1 || r.Subject.Assessment != AssessPass || r.Reducer.ID != "query-witness-r1" {
					t.Fatalf("complete witness: %+v", r)
				}
			case "blocked":
				if r.Assessment != AssessBlocked || !r.EvidenceValid || r.Recomputed.Assessment != AssessBlocked {
					t.Fatalf("missing anchor promoted or made corrupt: %+v", r)
				}
			default:
				if r.Assessment != AssessFail || r.EvidenceValid {
					t.Fatalf("accepted %s: %+v", kind, r)
				}
				if kind == "wrong-subject" && !slices.Contains(codes(r.Findings), "QUERY_WITNESS_SUBJECT_PLATFORM_MISMATCH") {
					t.Fatalf("subject platform binding not checked: %+v", r.Findings)
				}
				if kind == "empty-source-mismatch" && !slices.Contains(codes(r.Findings), "QUERY_WITNESS_SOURCE_MISMATCH") {
					t.Fatalf("zero-byte file confused with missing source: %+v", r.Findings)
				}
			}
		})
	}
}

func TestQueryWitnessSourceIndependentWireAndPolicy(t *testing.T) {
	for _, kind := range []string{"raw-query-id", "unsupported-evaluated", "unsupported-correct", "other-case-failure"} {
		t.Run(kind, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			if kind == "raw-query-id" {
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)["id"] = "other"
			} else if strings.HasPrefix(kind, "unsupported") {
				reg["encoding"], p["encoding"] = EncodingCP949, EncodingCP949
				for _, step := range p["steps"].([]any) {
					for _, role := range []string{"incremental", "fresh"} {
						tree, ok := step.(map[string]any)[role].(map[string]any)
						if !ok {
							continue
						}
						tree["tree"].(map[string]any)["input"].(map[string]any)["encoding"] = EncodingCP949
						if kind == "unsupported-correct" {
							q := tree["queries"].([]any)[0].(map[string]any)
							q["status"], q["code"], q["evaluation"], q["captures"] = "UNSUPPORTED", "QUERY_PREDICATE_ENCODING", "UNSUPPORTED", nil
						}
					}
				}
			} else {
				p["code"] = "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_1"
				p["assessment"] = AssessFail
			}
			req = marshalWitness(t, req, reg, raw, p)
			want := AssessFail
			switch kind {
			case "raw-query-id":
				req.Source = nil
			case "unsupported-evaluated", "unsupported-correct":
				req.Raw = nil
				if kind == "unsupported-correct" {
					want = AssessBlocked
				}
			default:
				want = AssessPass // other case claims are outside this API's capture claim
			}
			r, err := VerifyQueryWitness(ctxT(t), req)
			if err != nil || r.Assessment != want {
				t.Fatalf("source-independent check %s: %+v %v", kind, r, err)
			}
		})
	}
}

func TestQueryWitnessEmptyCaptureNamesAreNotNull(t *testing.T) {
	req, reg, raw, p := witnessFixture(t)
	querySource := "(doc)"
	q := reg["queries"].([]any)[0].(map[string]any)
	q["source"] = querySource
	q["metadata"] = map[string]any{"patterns": 1, "capture_names": []string{}, "predicates": []any{}}
	for _, tuple := range reg["required"].([]any) {
		tuple.(map[string]any)["structural"], tuple.(map[string]any)["expected"] = []Capture{}, []Capture{}
	}
	for _, document := range []map[string]any{raw, p} {
		for _, step := range document["steps"].([]any) {
			for _, role := range []string{"incremental", "fresh"} {
				tree, ok := step.(map[string]any)[role].(map[string]any)
				if !ok {
					continue
				}
				q := tree["queries"].([]any)[0].(map[string]any)
				q["capture_names"], q["predicates"], q["matches"] = []string{}, []any{}, 0
				if _, projection := q["sha256"]; projection {
					q["sha256"], q["evaluation"], q["captures"] = sum([]byte(querySource)), "STRUCTURAL", []Capture{}
				} else {
					q["types"], q["captures"] = []string{}, [][]int64{}
				}
			}
		}
	}
	r, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
	if err != nil || r.Assessment != AssessPass || r.Verified != 3 || r.Evaluated != 0 {
		t.Fatalf("complete no-capture query: %+v %v", r, err)
	}
	p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)["capture_names"] = nil
	r, err = VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
	if err != nil || r.Assessment != AssessFail || r.Verified != 2 {
		t.Fatalf("null capture names accepted or counted: %+v %v", r, err)
	}
}
