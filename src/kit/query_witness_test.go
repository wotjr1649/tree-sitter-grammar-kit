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
		{"raw-type-unused", func(_ *QueryWitnessRequest, _, raw, _ map[string]any) {
			raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["types"] = []string{"doc", "word", "unused"}
		}},
		{"raw-field-unused", func(_ *QueryWitnessRequest, _, raw, _ map[string]any) {
			raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["fields"] = []string{"unused"}
		}},
		{"raw-type-duplicate", func(_ *QueryWitnessRequest, _, raw, _ map[string]any) {
			tree := raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			tree["types"] = []string{"doc", "word", "word"}
			tree["nodes"].([][]int64)[1][1] = 2
		}},
		{"raw-type-order", func(_ *QueryWitnessRequest, _, raw, _ map[string]any) {
			tree := raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			tree["types"] = []string{"word", "doc"}
			tree["nodes"].([][]int64)[0][1], tree["nodes"].([][]int64)[1][1] = 1, 0
		}},
		{"missing-without-error", func(_ *QueryWitnessRequest, _, raw, p map[string]any) {
			nodes := fxTree("a", false)
			nodes[0].IsMissing = true
			rawTree := raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			rawTree["nodes"].([][]int64)[0][3], rawTree["digest"] = 1|16, TreeDigest(nodes)
			tree := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			tree["tree"].(map[string]any)["nodes"], tree["digest"] = nodes, TreeDigest(nodes)
		}},
		{"edit-summary-tree", func(_ *QueryWitnessRequest, _, raw, p map[string]any) {
			rawTree := raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			rawTree["form"] = "summary"
			delete(rawTree, "nodes")
			tree := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			tree["form"] = "summary"
			delete(tree, "tree")
		}},
		{"child-outside-parent", func(_ *QueryWitnessRequest, _, raw, p map[string]any) {
			nodes := fxTree("a", false)
			nodes[0].EndByte, nodes[0].EndPoint = 0, Point{}
			rawTree := raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			rawTree["nodes"].([][]int64)[0][5], rawTree["nodes"].([][]int64)[0][9], rawTree["digest"] = 0, 0, TreeDigest(nodes)
			tree := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			tree["tree"].(map[string]any)["nodes"], tree["digest"] = nodes, TreeDigest(nodes)
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
				q["partial"], pq["partial"], q["captures"], pq["evaluation"], q["types"] = false, false, nil, "", []string{}
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
			rawFresh, fresh := rawStep["fresh"], step["fresh"]
			rawStep["fresh"], step["fresh"] = nil, nil
			res, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
			if err != nil || res.Assessment != AssessBlocked || res.Verified != 1 {
				t.Fatalf("consistent incomplete native step: %+v %v", res, err)
			}
			// the driver writes a null fresh tree after an incomplete incremental one
			rawStep["fresh"], step["fresh"] = rawFresh, fresh
			for _, withRaw := range []bool{true, false} {
				req := marshalWitness(t, req, reg, raw, p)
				if !withRaw {
					req.Raw = nil
				}
				if res, err := VerifyQueryWitness(ctxT(t), req); err != nil || res.Assessment != AssessFail {
					t.Fatalf("fresh after incomplete incremental (raw %v): %+v %v", withRaw, res, err)
				}
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

func TestQueryWitnessReviewCompletedProjectionTrees(t *testing.T) {
	for _, step := range []int{0, 1} {
		req, reg, raw, p := witnessFixture(t)
		if step == 0 {
			reg["required"] = reg["required"].([]any)[1:]
		} else {
			reg["required"] = reg["required"].([]any)[:1]
		}
		s := p["steps"].([]any)[step].(map[string]any)
		s["incremental"], s["fresh"] = nil, nil
		res, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
		if err != nil || res.Assessment != AssessFail {
			t.Fatalf("completed projection omitted step %d trees: %+v %v", step, res, err)
		}
	}
}

func TestQueryWitnessReviewProjectionCaptureLinksWithoutRaw(t *testing.T) {
	for _, field := range []string{"intact", "node-negative", "node-range", "capture", "pattern", "name", "type", "flag", "byte", "point"} {
		t.Run(field, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			reg["required"] = reg["required"].([]any)[1:]
			q := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
			c := &q["captures"].([]Capture)[0]
			switch field {
			case "node-negative":
				c.Node = -1
			case "node-range":
				c.Node = 2
			case "capture":
				c.Capture = 1
			case "pattern":
				c.Pattern = 1
			case "name":
				c.Name = "other"
			case "type":
				c.Type = "other"
			case "flag":
				c.Named = false
			case "byte":
				c.EndByte = 2
			case "point":
				c.EndPoint.Column = 2
			}
			req = marshalWitness(t, req, reg, raw, p)
			req.Raw = nil
			res, err := VerifyQueryWitness(ctxT(t), req)
			want := AssessFail
			if field == "intact" {
				want = AssessBlocked
			}
			if err != nil || res.Assessment != want {
				t.Fatalf("unrequired projection capture %s: %+v %v", field, res, err)
			}
		})
	}
}

func TestQueryWitnessReviewInvalidQueryError(t *testing.T) {
	for _, mutation := range []string{"intact", "class", "point", "projection-only", "invalid-matches", "invalid-time", "invalid-names", "invalid-predicates", "raw-invalid-matches", "raw-invalid-time", "raw-invalid-names", "raw-invalid-predicates", "raw-invalid-types", "raw-legacy-null"} {
		t.Run(mutation, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			query := "\n(bogus)"
			reg["queries"] = append(reg["queries"].([]any), map[string]any{"id": "broken", "source": query, "metadata": nil})
			kind, point := "NODE_TYPE", [2]uint32{1, 1}
			if mutation == "class" {
				kind = "FORGED"
			}
			if mutation == "point" || mutation == "projection-only" {
				point = [2]uint32{9, 9}
			}
			for i, value := range raw["steps"].([]any) {
				step := value.(map[string]any)
				projected := p["steps"].([]any)[i].(map[string]any)
				for _, role := range []string{"incremental", "fresh"} {
					if step[role] == nil {
						continue
					}
					q := map[string]any{"id": "broken", "status": "INVALID_QUERY", "code": "QUERY_" + kind, "query_ms": 0, "error": map[string]any{"type": kind, "offset": 2, "point": point}, "patterns": 0, "capture_names": []string{}, "predicates": []any{}, "matches": 0, "partial": false, "types": []string{}, "captures": nil}
					switch strings.TrimPrefix(mutation, "raw-") {
					case "invalid-matches":
						q["matches"] = 1
					case "invalid-time":
						q["query_ms"] = 1
					case "invalid-names":
						q["capture_names"] = []string{"forged"}
					case "invalid-predicates":
						q["predicates"] = []any{map[string]any{"pattern": 0, "steps": []any{}}}
					case "invalid-types":
						q["types"] = []string{"forged"}
					case "legacy-null":
						q["capture_names"], q["predicates"], q["types"] = nil, nil, nil
					}
					tree := step[role].(map[string]any)
					tree["queries"] = append(tree["queries"].([]any), q)
					pq := map[string]any{}
					for key, value := range q {
						if key != "types" {
							pq[key] = value
						}
					}
					pq["sha256"], pq["evaluation"] = sum([]byte(query)), ""
					pt := projected[role].(map[string]any)
					pt["queries"] = append(pt["queries"].([]any), pq)
				}
			}
			req = marshalWitness(t, req, reg, raw, p)
			if mutation == "projection-only" || strings.HasPrefix(mutation, "invalid-") {
				req.Raw = nil
			}
			if strings.HasPrefix(mutation, "raw-") {
				req.Projection = nil
			}
			res, err := VerifyQueryWitness(ctxT(t), req)
			want := AssessFail
			if mutation == "intact" {
				want = AssessPass
			}
			if mutation == "raw-legacy-null" {
				want = AssessBlocked
			}
			if err != nil || res.Assessment != want {
				t.Fatalf("invalid query %s: %+v %v", mutation, res, err)
			}
		})
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
	q["predicates"], q["capture_names"], q["patterns"], q["types"] = nil, nil, 0, []string{}
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
	q["captures"], q["matches"], q["types"] = [][]int64{}, 0, []string{}
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

func TestQueryWitnessRejectsImpossibleCaptureNameTables(t *testing.T) {
	for _, names := range [][]string{{"x", "x"}, {"x", ""}} {
		req, reg, raw, p := witnessFixture(t)
		reg["queries"].([]any)[0].(map[string]any)["metadata"].(map[string]any)["capture_names"] = names
		for _, document := range []map[string]any{raw, p} {
			for _, step := range document["steps"].([]any) {
				for _, role := range []string{"incremental", "fresh"} {
					if tree, ok := step.(map[string]any)[role].(map[string]any); ok {
						tree["queries"].([]any)[0].(map[string]any)["capture_names"] = names
					}
				}
			}
		}
		r, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
		e, ok := err.(*Error)
		if !ok || e.Kind != KindInvalidInput || e.Code != "QUERY_REGISTRATION_QUERY_INVALID" || r.Assessment == AssessPass || r.Verified != 0 {
			t.Fatalf("capture names %q adopted: %+v %v", names, r, err)
		}
	}
}

func TestQueryWitnessRejectsUnknownPredicateCapture(t *testing.T) {
	req, reg, raw, p := witnessFixture(t)
	// every document shares the fixture's predicate maps
	reg["queries"].([]any)[0].(map[string]any)["metadata"].(map[string]any)["predicates"].([]any)[0].(map[string]any)["steps"].([]any)[1].(map[string]any)["value"] = "y"
	r, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
	e, ok := err.(*Error)
	if !ok || e.Kind != KindInvalidInput || e.Code != "QUERY_REGISTRATION_QUERY_INVALID" || r.Assessment == AssessPass || r.Verified != 0 {
		t.Fatalf("unknown predicate capture adopted: %+v %v", r, err)
	}
}

func TestQueryWitnessRejectsImpossibleProjectionCSTWithoutRaw(t *testing.T) {
	alias := fxTree("a", false)
	alias[0].Type = "" // the generator accepts alias(rule, "")
	if !witnessCST(alias) {
		t.Fatal("empty alias type rejected")
	}
	for name, mutate := range map[string]func([]TreeNode){
		"child-outside-parent": func(nodes []TreeNode) { nodes[0].EndByte, nodes[0].EndPoint = 0, Point{} },
	} {
		t.Run(name, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			tree := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			nodes := fxTree("a", false)
			mutate(nodes)
			tree["tree"].(map[string]any)["nodes"], tree["digest"] = nodes, TreeDigest(nodes)
			req = marshalWitness(t, req, reg, raw, p)
			req.Raw = nil
			res, err := VerifyQueryWitness(ctxT(t), req)
			if err != nil || res.Assessment != AssessFail {
				t.Fatalf("impossible projection CST: %+v %v", res, err)
			}
		})
	}
}

func TestQueryWitnessRejectsImpossibleRegistrationMetadata(t *testing.T) {
	pred := func(pattern int, steps []any) map[string]any {
		return map[string]any{"pattern": pattern, "steps": steps}
	}
	eq := []any{map[string]any{"kind": "string", "value": "eq?", "quantifier": nil}, map[string]any{"kind": "capture", "value": "x", "quantifier": "ONE"}, map[string]any{"kind": "string", "value": "a", "quantifier": nil}}
	for name, tc := range map[string]struct {
		patterns   int
		predicates []any
	}{
		"pattern-range": {1, []any{pred(1, eq)}},
		"empty-steps":   {1, []any{pred(0, []any{})}},
		"pattern-order": {2, []any{pred(1, eq), pred(0, eq)}},
	} {
		t.Run(name, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			reg["queries"].([]any)[0].(map[string]any)["metadata"] = map[string]any{"patterns": tc.patterns, "capture_names": []string{"x"}, "predicates": tc.predicates}
			r, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
			e, ok := err.(*Error)
			if !ok || e.Code != "QUERY_REGISTRATION_QUERY_INVALID" || r.Assessment == AssessPass {
				t.Fatalf("impossible metadata adopted: %+v %v", r, err)
			}
		})
	}
}

func TestQueryWitnessReviewImpossibleProjectionQueryWithoutRaw(t *testing.T) {
	for _, kind := range []string{"duplicate-names", "missing-without-error"} {
		t.Run(kind, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			reg["queries"].([]any)[0].(map[string]any)["metadata"] = nil
			tree := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			q := tree["queries"].([]any)[0].(map[string]any)
			if kind == "duplicate-names" {
				q["capture_names"] = []string{"x", "x"}
			} else {
				// a summary projection has no CST, so only the capture flags can expose it; only
				// a request without edits may yield one
				reg["edits"], reg["required"], p["steps"] = []Edit{}, reg["required"].([]any)[:1], p["steps"].([]any)[:1]
				tree["form"] = "summary"
				delete(tree, "tree")
				q["captures"].([]Capture)[0].IsMissing = true
				tuple := reg["required"].([]any)[0].(map[string]any)
				tuple["structural"].([]Capture)[0].IsMissing, tuple["expected"].([]Capture)[0].IsMissing = true, true
			}
			req = marshalWitness(t, req, reg, raw, p)
			req.Raw = nil
			if res, err := VerifyQueryWitness(ctxT(t), req); err != nil || res.Assessment != AssessFail {
				t.Fatalf("projection adopted: %+v %v", res, err)
			}
		})
	}
}

func TestQueryWitnessRejectsNodeRowsOutsideFullForm(t *testing.T) {
	for _, form := range []string{"summary", "record"} {
		req, reg, raw, p := witnessFixture(t)
		// without edits the driver may serialize a non-full tree, which carries no node rows
		reg["edits"], reg["required"] = []Edit{}, reg["required"].([]any)[:1]
		raw["steps"], raw["steps_completed"], p["steps"] = raw["steps"].([]any)[:1], 1, p["steps"].([]any)[:1]
		raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["form"] = form
		tree := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
		tree["form"] = form
		delete(tree, "tree")
		res, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
		if err != nil || res.Assessment != AssessFail {
			t.Fatalf("%s node rows adopted: %+v %v", form, res, err)
		}
	}
}

func TestQueryWitnessReviewIncompleteProjectionCodeWithoutRaw(t *testing.T) {
	for _, code := range []string{"START_FAILED", ""} {
		req, reg, raw, p := witnessFixture(t)
		p["execution_status"], p["code"], p["steps"] = StatusFailed, code, []any{}
		req = marshalWitness(t, req, reg, raw, p)
		req.Raw = nil
		res, err := VerifyQueryWitness(ctxT(t), req)
		want := AssessBlocked
		if code == "" {
			want = AssessFail // every incomplete host case records its cause
		}
		if err != nil || res.Assessment != want {
			t.Fatalf("incomplete projection code %q: %+v %v", code, res, err)
		}
	}
}

func TestQueryWitnessReviewUnrequiredEvaluation(t *testing.T) {
	req, reg, raw, p := witnessFixture(t)
	reg["required"] = reg["required"].([]any)[1:]
	p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)["captures"] = []Capture{}
	res, err := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
	if err != nil || res.Assessment != AssessFail {
		t.Fatalf("unrequired evaluated captures omitted: %+v %v", res, err)
	}
}

func TestQueryWitnessReviewCompletedProjectionChildStatusWithoutRaw(t *testing.T) {
	for _, status := range []string{StatusFailed, StatusResourceLimit} {
		t.Run(status, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			tree := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			tree["status"], tree["code"], tree["form"], tree["tree"] = status, "NODE_LIMIT", "", nil
			tree["descendant_count"], tree["digest"], tree["has_error"] = 0, "", false
			q := tree["queries"].([]any)[0].(map[string]any)
			q["status"], q["code"], q["evaluation"], q["captures"] = StatusNotRun, "TREE_NOT_COMPLETED", "", nil
			q["patterns"], q["capture_names"], q["predicates"], q["matches"] = 0, []string{}, []any{}, 0
			req = marshalWitness(t, req, reg, raw, p)
			req.Raw = nil
			res, err := VerifyQueryWitness(ctxT(t), req)
			if err != nil || res.Assessment != AssessFail {
				t.Fatalf("completed parent with %s child: %+v %v", status, res, err)
			}
		})
	}
}

func TestQueryWitnessReviewCompletedProjectionQueryStatusWithoutRaw(t *testing.T) {
	for _, status := range []string{StatusNotRun, StatusResourceLimit, StatusFailed} {
		t.Run(status, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			q := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
			q["status"], q["captures"] = status, nil
			switch status {
			case StatusNotRun:
				q["code"], q["evaluation"], q["patterns"] = "TREE_NOT_COMPLETED", "", 0
				q["capture_names"], q["predicates"], q["matches"] = []string{}, []any{}, 0
			case StatusResourceLimit:
				q["code"], q["evaluation"], q["partial"] = "CAPTURE_LIMIT", "UNSUPPORTED", true
			case StatusFailed:
				q["code"], q["evaluation"], q["patterns"] = "CAPTURE_NODE_UNMAPPED", "", 0
				q["capture_names"], q["predicates"], q["matches"] = nil, []any{}, 0
			}
			req = marshalWitness(t, req, reg, raw, p)
			req.Raw = nil
			res, err := VerifyQueryWitness(ctxT(t), req)
			if err != nil || res.Assessment != AssessFail {
				t.Fatalf("completed parent with %s query: %+v %v", status, res, err)
			}
		})
	}
}

func TestQueryWitnessReviewProjectionCompletionWithoutRaw(t *testing.T) {
	for _, kind := range []string{"all-completed", "partial-unsupported-valid", "partial-unsupported-completed", "partial-unsupported-wrong-code"} {
		t.Run(kind, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			p["execution_status"], p["code"] = StatusResourceLimit, "NODE_LIMIT"
			if kind != "all-completed" {
				query := reg["queries"].([]any)[0].(map[string]any)
				source := `((word) @x (#match? @x "a"))`
				query["source"] = source
				predicate := query["metadata"].(map[string]any)["predicates"].([]any)[0].(map[string]any)
				predicate["steps"].([]any)[0].(map[string]any)["value"] = "match?"
				for _, value := range p["steps"].([]any) {
					step := value.(map[string]any)
					for _, role := range []string{"incremental", "fresh"} {
						if step[role] == nil {
							continue
						}
						q := step[role].(map[string]any)["queries"].([]any)[0].(map[string]any)
						q["sha256"], q["status"], q["code"], q["evaluation"], q["captures"] = sum([]byte(source)), "UNSUPPORTED", "QUERY_PREDICATE_UNSUPPORTED:match?", "UNSUPPORTED", nil
					}
				}
				p["steps"].([]any)[1].(map[string]any)["fresh"].(map[string]any)["queries"].([]any)[0].(map[string]any)["partial"] = true
				p["code"] = "CAPTURE_LIMIT"
				if kind == "partial-unsupported-completed" {
					p["execution_status"], p["code"] = StatusCompleted, ""
				}
				if kind == "partial-unsupported-wrong-code" {
					p["code"] = "NODE_LIMIT"
				}
			}
			req = marshalWitness(t, req, reg, raw, p)
			req.Raw = nil
			res, err := VerifyQueryWitness(ctxT(t), req)
			want := AssessFail
			if kind == "partial-unsupported-valid" {
				want = AssessBlocked
			}
			if err != nil || res.Assessment != want {
				t.Fatalf("%s: %+v %v", kind, res, err)
			}
		})
	}
}

func TestQueryWitnessReviewProjectionMetadataWithoutRaw(t *testing.T) {
	for _, kind := range []string{"unsupported", "host-failed", "resource-limit"} {
		for _, tamper := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "/intact", true: "/tampered"}[tamper], func(t *testing.T) {
				req, reg, raw, p := witnessFixture(t)
				if kind == "unsupported" {
					meta := reg["queries"].([]any)[0].(map[string]any)["metadata"].(map[string]any)
					meta["predicates"].([]any)[0].(map[string]any)["steps"].([]any)[0].(map[string]any)["value"] = "match?"
				}
				for _, value := range p["steps"].([]any) {
					step := value.(map[string]any)
					for _, role := range []string{"incremental", "fresh"} {
						if step[role] == nil {
							continue
						}
						q := step[role].(map[string]any)["queries"].([]any)[0].(map[string]any)
						q["captures"], q["evaluation"] = nil, "UNSUPPORTED"
						switch kind {
						case "unsupported":
							q["status"], q["code"] = "UNSUPPORTED", "QUERY_PREDICATE_UNSUPPORTED:match?"
						case "host-failed":
							q["status"], q["code"] = StatusFailed, "QUERY_PREDICATE_CAPTURE_ABSENT"
						}
					}
				}
				if kind == "resource-limit" {
					// Keep previous steps completed; only the last fresh query reaches its cap.
					req, reg, raw, p = witnessFixture(t)
					q := p["steps"].([]any)[1].(map[string]any)["fresh"].(map[string]any)["queries"].([]any)[0].(map[string]any)
					q["status"], q["code"], q["partial"], q["captures"], q["evaluation"] = StatusResourceLimit, "CAPTURE_LIMIT", true, nil, "UNSUPPORTED"
					p["execution_status"], p["code"] = StatusResourceLimit, "CAPTURE_LIMIT"
				}
				q := p["steps"].([]any)[1].(map[string]any)["fresh"].(map[string]any)["queries"].([]any)[0].(map[string]any)
				if tamper {
					q["capture_names"] = []string{"different"}
				}
				req = marshalWitness(t, req, reg, raw, p)
				req.Raw = nil
				res, err := VerifyQueryWitness(ctxT(t), req)
				want := AssessBlocked
				if tamper {
					want = AssessFail
				}
				if err != nil || res.Assessment != want {
					t.Fatalf("%s tamper=%v: %+v %v", kind, tamper, res, err)
				}
			})
		}
	}
}

func witnessTreeLimitFixture(t *testing.T) (QueryWitnessRequest, map[string]any, map[string]any, map[string]any) {
	t.Helper()
	req, reg, raw, p := witnessFixture(t)
	raw["status"], raw["code"], raw["steps_completed"] = StatusResourceLimit, "NODE_LIMIT", 1
	p["execution_status"], p["code"] = StatusResourceLimit, "NODE_LIMIT"
	rstep := raw["steps"].([]any)[1].(map[string]any)
	rstep["incremental"] = map[string]any{"status": StatusResourceLimit, "code": "NODE_LIMIT", "form": nil, "queries": nil}
	rstep["fresh"] = nil
	step := p["steps"].([]any)[1].(map[string]any)
	query := map[string]any{"id": "q", "sha256": sum([]byte(`((word) @x (#eq? @x "a"))`)), "status": StatusNotRun, "code": "TREE_NOT_COMPLETED", "evaluation": "", "query_ms": 0, "error": nil, "patterns": 0, "capture_names": []string{}, "predicates": []any{}, "matches": 0, "partial": false, "captures": nil}
	step["incremental"] = map[string]any{"status": StatusResourceLimit, "code": "NODE_LIMIT", "form": "", "descendant_count": 0, "has_error": false, "digest": "", "tree": nil, "queries": []any{query}}
	step["fresh"] = nil
	return req, reg, raw, p
}

func TestQueryWitnessReviewIncompleteTreeEmptySummary(t *testing.T) {
	for _, field := range []string{"intact", "descendant_count", "has_error", "digest", "types", "fields"} {
		t.Run(field, func(t *testing.T) {
			req, reg, raw, p := witnessTreeLimitFixture(t)
			rt := raw["steps"].([]any)[1].(map[string]any)["incremental"].(map[string]any)
			pt := p["steps"].([]any)[1].(map[string]any)["incremental"].(map[string]any)
			switch field {
			case "descendant_count":
				rt[field], pt[field] = 1, 1
			case "has_error":
				rt[field], pt[field] = true, true
			case "digest":
				rt[field], pt[field] = sum([]byte("forged")), sum([]byte("forged"))
			case "types", "fields":
				rt[field] = []string{"forged"}
			}
			res, e := VerifyQueryWitness(ctxT(t), marshalWitness(t, req, reg, raw, p))
			want := AssessFail
			if field == "intact" {
				want = AssessBlocked
			}
			if e != nil || res.Assessment != want {
				t.Fatalf("%s: %+v %v", field, res, e)
			}
		})
	}
}

func TestQueryWitnessReviewUnmappedCaptureMetadataWithoutRaw(t *testing.T) {
	for _, field := range []string{"intact", "patterns", "capture_names", "predicates", "null-capture-names", "null-predicates"} {
		t.Run(field, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			p["execution_status"], p["code"] = StatusFailed, "CAPTURE_NODE_UNMAPPED"
			step := p["steps"].([]any)[1].(map[string]any)
			step["fresh"] = nil
			q := step["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
			q["status"], q["code"], q["evaluation"], q["captures"] = StatusFailed, "CAPTURE_NODE_UNMAPPED", "", nil
			switch field {
			case "patterns":
				q[field] = 2
			case "capture_names":
				q[field] = []string{"other"}
			case "predicates":
				q[field] = []any{}
			case "null-capture-names":
				q["capture_names"] = nil
			case "null-predicates":
				q["predicates"] = nil
			}
			req = marshalWitness(t, req, reg, raw, p)
			req.Raw = nil
			res, e := VerifyQueryWitness(ctxT(t), req)
			want := AssessFail
			if field == "intact" {
				want = AssessBlocked
			}
			if e != nil || res.Assessment != want {
				t.Fatalf("%s: %+v %v", field, res, e)
			}
		})
	}
}

func TestQueryWitnessReviewStructuralMatchAccountingWithoutRaw(t *testing.T) {
	for _, kind := range []string{"intact", "interleaved", "count", "skip", "mixed-pattern"} {
		t.Run(kind, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			reg["required"] = reg["required"].([]any)[1:]
			query := reg["queries"].([]any)[0].(map[string]any)
			query["source"] = "(word) @x\n(word) @x"
			meta := query["metadata"].(map[string]any)
			meta["patterns"], meta["predicates"] = 2, []any{}
			for _, value := range p["steps"].([]any) {
				step := value.(map[string]any)
				for _, role := range []string{"incremental", "fresh"} {
					if step[role] == nil {
						continue
					}
					q := step[role].(map[string]any)["queries"].([]any)[0].(map[string]any)
					q["sha256"], q["evaluation"], q["patterns"], q["predicates"] = sum([]byte(query["source"].(string))), "STRUCTURAL", 2, []any{}
					q["matches"] = len(q["captures"].([]Capture))
				}
			}
			q := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
			captures := q["captures"].([]Capture)
			switch kind {
			case "count":
				q["matches"] = 2
			case "skip":
				captures[0].Match = 1
				q["matches"] = 2
			case "mixed-pattern":
				other := captures[0]
				other.Pattern = 1
				q["captures"] = append(captures, other)
			case "interleaved":
				other := captures[0]
				other.Match, other.Pattern = 1, 1
				q["captures"] = []Capture{captures[0], other, captures[0]}
				q["matches"] = 2
			}
			req = marshalWitness(t, req, reg, raw, p)
			req.Raw = nil
			res, err := VerifyQueryWitness(ctxT(t), req)
			want := AssessFail
			if kind == "intact" || kind == "interleaved" {
				want = AssessBlocked
			}
			if err != nil || res.Assessment != want {
				t.Fatalf("%s: %+v %v", kind, res, err)
			}
		})
	}
}

func TestQueryWitnessReviewNotRunProjectionFieldsWithoutRaw(t *testing.T) {
	for _, field := range []string{"intact", "matches", "query_ms", "capture_names", "predicates", "null-capture-names", "null-predicates"} {
		t.Run(field, func(t *testing.T) {
			req, reg, raw, p := witnessTreeLimitFixture(t)
			q := p["steps"].([]any)[1].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
			switch field {
			case "matches", "query_ms":
				q[field] = 1
			case "capture_names":
				q[field] = []string{"forged"}
			case "predicates":
				q[field] = []any{map[string]any{"pattern": 0, "steps": []any{}}}
			case "null-capture-names":
				q["capture_names"] = nil
			case "null-predicates":
				q["predicates"] = nil
			}
			req = marshalWitness(t, req, reg, raw, p)
			req.Raw = nil
			res, err := VerifyQueryWitness(ctxT(t), req)
			want := AssessFail
			if field == "intact" {
				want = AssessBlocked
			}
			if err != nil || res.Assessment != want {
				t.Fatalf("%s: %+v %v", field, res, err)
			}
		})
	}
}

func TestQueryWitnessReviewRetainedStepCount(t *testing.T) {
	for _, missingSource := range []bool{false, true} {
		req, reg, raw, p := witnessTreeLimitFixture(t)
		p["steps"] = []any{}
		req = marshalWitness(t, req, reg, raw, p)
		if missingSource {
			req.Source = nil
		}
		res, err := VerifyQueryWitness(ctxT(t), req)
		if err != nil || res.Assessment != AssessFail {
			t.Fatalf("retained projection omitted: %+v %v", res, err)
		}
	}
}

func TestQueryWitnessReviewStaticEditShapeWithoutSource(t *testing.T) {
	for _, kind := range []string{"old-size", "new-size", "range", "empty", "size-limit"} {
		t.Run(kind, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			edits := reg["edits"].([]Edit)
			switch kind {
			case "old-size":
				edits[0].Old = []byte{}
			case "new-size":
				edits[0].New = []byte{}
			case "range":
				edits[0].OldEndByte = 2
			case "empty":
				edits[0] = Edit{Old: []byte{}, New: []byte{}}
			case "size-limit":
				input := reg["input"].(NativeInput)
				input.Bytes = NativeOperations()["native-query-large"].InputBytes + 1
				reg["input"] = input
			}
			req = marshalWitness(t, req, reg, raw, p)
			req.Source = nil
			_, err := VerifyQueryWitness(ctxT(t), req)
			want := KindInvalidInput
			if kind == "size-limit" {
				want = KindResourceLimit
			}
			if e, ok := err.(*Error); !ok || e.Kind != want {
				t.Fatalf("%s malformed registration: %v", kind, err)
			}
		})
	}
}

func TestQueryWitnessReviewEditPointsWithoutSource(t *testing.T) {
	for _, mutation := range []string{"intact", "old-end", "new-end"} {
		t.Run(mutation, func(t *testing.T) {
			req, reg, raw, p := witnessFixture(t)
			for _, doc := range []map[string]any{raw, p} {
				edit := doc["steps"].([]any)[1].(map[string]any)["edit"].(map[string]any)
				if mutation == "old-end" {
					edit["old_end_point"] = []int{0, 2}
				}
				if mutation == "new-end" {
					edit["new_end_point"] = []int{1, 0}
				}
			}
			req = marshalWitness(t, req, reg, raw, p)
			req.Source = nil
			res, err := VerifyQueryWitness(ctxT(t), req)
			want := AssessFail
			if mutation == "intact" {
				want = AssessBlocked
			}
			if err != nil || res.Assessment != want {
				t.Fatalf("edit point %s: %+v %v", mutation, res, err)
			}
		})
	}
}

func TestQueryWitnessRegisteredEditPointDeltas(t *testing.T) {
	for _, tc := range []struct {
		name, encoding string
		old, new       []byte
		oldEnd, newEnd [2]uint32
	}{
		{"insert", EncodingUTF8, []byte{}, []byte("x"), [2]uint32{1, 2}, [2]uint32{1, 3}},
		{"delete", EncodingUTF8, []byte("x"), []byte{}, [2]uint32{1, 3}, [2]uint32{1, 2}},
		{"lf", EncodingUTF8, []byte("a\nb"), []byte("\r\n"), [2]uint32{2, 1}, [2]uint32{2, 0}},
		{"utf16le", EncodingUTF16LE, []byte{'a', 0, 10, 0, 'b', 0}, []byte{}, [2]uint32{2, 2}, [2]uint32{1, 2}},
		{"utf16be", EncodingUTF16BE, []byte{0, 'a', 0, 10, 0, 'b'}, []byte{}, [2]uint32{2, 2}, [2]uint32{1, 2}},
		{"cp949", EncodingCP949, []byte{0xb0, 0xa1}, []byte{}, [2]uint32{1, 4}, [2]uint32{1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Edit{StartByte: 4, OldEndByte: 4 + uint32(len(tc.old)), NewEndByte: 4 + uint32(len(tc.new)), Old: tc.old, New: tc.new}
			reg := QueryWitnessRegistration{Encoding: tc.encoding, Input: NativeInput{Bytes: 16}, Edits: []Edit{e}}
			edit := &witnessEdit{StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte, StartPoint: [2]uint32{1, 2}, OldEndPoint: tc.oldEnd, NewEndPoint: tc.newEnd}
			size := uint64(16 - len(tc.old) + len(tc.new))
			if !witnessRegisteredStep(1, 1, size, strings.Repeat("0", 64), edit, reg) {
				t.Fatal("valid relative edit points rejected")
			}
			edit.NewEndPoint[1]++
			if witnessRegisteredStep(1, 1, size, strings.Repeat("0", 64), edit, reg) {
				t.Fatal("forged relative edit point accepted")
			}
		})
	}
}

func TestQueryWitnessReviewDroppedStatusPair(t *testing.T) {
	for _, code := range []string{"OUTPUT_LIMIT", "ALLOCATION_LIMIT", "LANGUAGE_INCOMPATIBLE"} {
		for _, status := range []string{StatusResourceLimit, StatusFailed} {
			req, reg, raw, p := witnessFixture(t)
			raw["status"], raw["code"], raw["steps"], raw["steps_completed"] = status, code, []any{}, 0
			req = marshalWitness(t, req, reg, raw, p)
			req.Projection = nil
			got, err := VerifyQueryWitness(ctxT(t), req)
			want := AssessFail
			if (code == "LANGUAGE_INCOMPATIBLE") == (status == StatusFailed) {
				want = AssessBlocked
			}
			if err != nil || got.Assessment != want {
				t.Fatalf("%s/%s: %s %v", status, code, got.Assessment, err)
			}
		}
	}
}

func TestQueryWitnessImpossibleStartPoint(t *testing.T) {
	for _, tc := range []struct {
		encoding string
		start    uint32
		point    [2]uint32
		payload  []byte
	}{
		{EncodingUTF8, 2, [2]uint32{0, 0}, []byte("x")},
		{EncodingUTF16LE, 2, [2]uint32{2, 0}, []byte{'x', 0}},
		{EncodingUTF16BE, 4, [2]uint32{1, 1}, []byte{0, 'x'}},
	} {
		e := Edit{StartByte: tc.start, OldEndByte: tc.start + uint32(len(tc.payload)), NewEndByte: tc.start + uint32(len(tc.payload)), Old: tc.payload, New: tc.payload}
		reg := QueryWitnessRegistration{Encoding: tc.encoding, Input: NativeInput{Bytes: 16}, Edits: []Edit{e}}
		end := tc.point
		end[1] += uint32(len(tc.payload))
		edit := &witnessEdit{StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte, StartPoint: tc.point, OldEndPoint: end, NewEndPoint: end}
		if witnessRegisteredStep(1, 1, 16, strings.Repeat("0", 64), edit, reg) {
			t.Fatalf("impossible %s start point accepted: %v at %d", tc.encoding, tc.point, tc.start)
		}
	}
}

func TestQueryWitnessReviewIncompleteTreeCodes(t *testing.T) {
	for _, code := range []string{"PARSE_TIME_LIMIT", "NODE_LIMIT", "DEPTH_LIMIT", "PARSE_NULL", "DESCENDANT_COUNT_MISMATCH", "FORGED"} {
		for _, status := range []string{StatusResourceLimit, StatusFailed} {
			for _, keep := range []string{"both", "raw", "projection"} {
				req, reg, raw, p := witnessTreeLimitFixture(t)
				raw["status"], raw["code"], p["execution_status"], p["code"] = status, code, status, code
				for _, doc := range []map[string]any{raw, p} {
					tree := doc["steps"].([]any)[1].(map[string]any)["incremental"].(map[string]any)
					tree["status"], tree["code"] = status, code
				}
				req = marshalWitness(t, req, reg, raw, p)
				if keep == "raw" {
					req.Projection = nil
				} else if keep == "projection" {
					req.Raw = nil
				}
				got, err := VerifyQueryWitness(ctxT(t), req)
				want := AssessFail
				resource := slices.Contains([]string{"PARSE_TIME_LIMIT", "NODE_LIMIT", "DEPTH_LIMIT"}, code)
				failed := slices.Contains([]string{"PARSE_NULL", "DESCENDANT_COUNT_MISMATCH"}, code)
				if status == StatusResourceLimit && resource || status == StatusFailed && failed {
					want = AssessBlocked
				}
				if err != nil || got.Assessment != want {
					t.Fatalf("%s %s/%s: %s %v", keep, status, code, got.Assessment, err)
				}
			}
		}
	}
}

func TestQueryWitnessReviewUTF16RegistrationWithoutSource(t *testing.T) {
	for _, encoding := range []string{EncodingUTF16LE, EncodingUTF16BE} {
		for _, mutation := range []string{"initial-size", "start", "old-end", "new-end", "old-null", "new-surrogate"} {
			req, reg, raw, p := witnessFixture(t)
			reg["encoding"] = encoding
			input := reg["input"].(NativeInput)
			input.Bytes = 4
			e := Edit{StartByte: 0, OldEndByte: 2, NewEndByte: 2, Old: []byte{0, 'a'}, New: []byte{0, 'b'}}
			switch mutation {
			case "initial-size":
				input.Bytes = 3
			case "start":
				e.StartByte, e.OldEndByte, e.NewEndByte = 1, 3, 3
			case "old-end":
				e.OldEndByte, e.Old = 1, []byte{'a'}
			case "new-end":
				e.NewEndByte, e.New = 1, []byte{'b'}
			case "old-null":
				e.Old = []byte{0, 0}
			case "new-surrogate":
				e.New = []byte{0, 0xd8}
				if encoding == EncodingUTF16BE {
					e.New = []byte{0xd8, 0}
				}
			}
			reg["input"], reg["edits"] = input, []Edit{e}
			req = marshalWitness(t, req, reg, raw, p)
			req.Source = nil
			_, err := VerifyQueryWitness(ctxT(t), req)
			if e, ok := err.(*Error); !ok || e.Kind != KindInvalidInput {
				t.Fatalf("%s/%s registration: %v", encoding, mutation, err)
			}
		}
	}
}

func TestQueryWitnessReviewSummaryPredicateEvaluation(t *testing.T) {
	for _, form := range []string{"summary", "record"} {
		for _, mutation := range []string{"intact", "evaluated", "raw-point", "raw-only-point", "projection-point", "projection-name", "projection-range", "no-source-intact", "no-source-range", "no-source-reversed"} {
			req, reg, raw, p := witnessFixture(t)
			// the driver serializes only full trees for a request with edits
			reg["edits"], reg["required"] = []Edit{}, reg["required"].([]any)[:1]
			raw["steps"], raw["steps_completed"], p["steps"] = raw["steps"].([]any)[:1], 1, p["steps"].([]any)[:1]
			for _, value := range raw["steps"].([]any) {
				for _, role := range []string{"incremental", "fresh"} {
					if tree, ok := value.(map[string]any)[role].(map[string]any); ok {
						tree["form"] = form
						delete(tree, "nodes")
						delete(tree, "types")
						delete(tree, "fields")
					}
				}
			}
			for _, value := range p["steps"].([]any) {
				for _, role := range []string{"incremental", "fresh"} {
					if tree, ok := value.(map[string]any)[role].(map[string]any); ok {
						tree["form"] = form
						delete(tree, "tree")
					}
				}
			}
			query := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
			capture := &query["captures"].([]Capture)[0]
			switch mutation {
			case "evaluated":
				query["captures"] = []Capture{}
			case "raw-point", "raw-only-point", "projection-point":
				capture.StartPoint = Point{Row: 1}
				if mutation == "raw-point" || mutation == "raw-only-point" {
					row := raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)["captures"].([][]int64)[0]
					row[8], row[9] = 1, 0
				}
			case "projection-name":
				capture.Name = "forged"
			case "projection-range", "no-source-range":
				capture.EndByte = 2
			case "no-source-reversed":
				capture.StartByte, capture.EndByte = 1, 0
			}
			req = marshalWitness(t, req, reg, raw, p)
			if strings.HasPrefix(mutation, "projection-") {
				req.Raw = nil
			}
			if mutation == "raw-only-point" {
				req.Projection = nil
			}
			if strings.HasPrefix(mutation, "no-source-") {
				req.Source, req.Raw = nil, nil
			}
			got, err := VerifyQueryWitness(ctxT(t), req)
			want := AssessBlocked
			if mutation != "intact" && mutation != "no-source-intact" {
				want = AssessFail
			}
			if err != nil || got.Assessment != want || mutation == "evaluated" && !slices.Contains(codes(got.Findings), "QUERY_WITNESS_QUERY_PROJECTION_MISMATCH") {
				t.Fatalf("%s %s: %s %v %v", form, mutation, got.Assessment, err, codes(got.Findings))
			}
		}
	}
}

func TestQueryWitnessReviewCancelledCodeWithoutRaw(t *testing.T) {
	for _, code := range []string{"CANCELLED", "FORGED"} {
		req, reg, raw, p := witnessFixture(t)
		p["execution_status"], p["code"], p["steps"] = StatusCancelled, code, []any{}
		req = marshalWitness(t, req, reg, raw, p)
		req.Raw = nil
		r, e := VerifyQueryWitness(ctxT(t), req)
		want := AssessBlocked
		if code == "FORGED" {
			want = AssessFail
		}
		if e != nil || r.Assessment != want {
			t.Fatalf("cancel code %s: %+v %v", code, r, e)
		}
	}
}

func TestQueryWitnessReviewUnsupportedPartialWithoutRaw(t *testing.T) {
	req, reg, raw, p := witnessFixture(t)
	query := reg["queries"].([]any)[0].(map[string]any)
	query["source"] = `((word) @x (#match? @x "a"))`
	meta := query["metadata"].(map[string]any)
	preds := meta["predicates"].([]any)
	preds[0].(map[string]any)["steps"].([]any)[0].(map[string]any)["value"] = "match?"
	p["execution_status"], p["code"] = StatusResourceLimit, "MATCH_LIMIT"
	p["steps"] = p["steps"].([]any)[:1]
	q := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
	q["sha256"], q["predicates"], q["status"], q["code"], q["evaluation"], q["partial"], q["captures"] = sum([]byte(query["source"].(string))), preds, StatusResourceLimit, "MATCH_LIMIT", "UNSUPPORTED", true, nil
	req = marshalWitness(t, req, reg, raw, p)
	req.Raw = nil
	r, e := VerifyQueryWitness(ctxT(t), req)
	if e != nil || r.Assessment != AssessFail {
		t.Fatalf("unsupported partial status: %+v %v", r, e)
	}
}

func TestQueryWitnessReviewEvaluatedMatchAccountingWithoutRaw(t *testing.T) {
	for _, kind := range []string{"gaps", "interleaved", "pattern", "first-order"} {
		req, reg, raw, p := witnessFixture(t)
		reg["required"] = []any{}
		qreg := reg["queries"].([]any)[0].(map[string]any)
		qreg["source"] = "((word) @x (#eq? @x \"a\"))\n(word) @x"
		meta := qreg["metadata"].(map[string]any)
		meta["patterns"] = 2
		for _, v := range p["steps"].([]any) {
			for _, role := range []string{"incremental", "fresh"} {
				if tree, ok := v.(map[string]any)[role].(map[string]any); ok {
					q := tree["queries"].([]any)[0].(map[string]any)
					q["sha256"], q["patterns"] = sum([]byte(qreg["source"].(string))), 2
				}
			}
		}
		q := p["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"].([]any)[0].(map[string]any)
		first := q["captures"].([]Capture)[0]
		first.Match = 1
		second := first
		second.Match = 3
		second.Pattern = 1
		q["matches"] = 4
		switch kind {
		case "gaps":
			q["captures"] = []Capture{first, second}
		case "interleaved":
			q["captures"] = []Capture{first, second, first}
		case "pattern":
			second.Match = first.Match
			q["captures"] = []Capture{first, second}
		case "first-order":
			q["captures"] = []Capture{second, first}
		}
		req = marshalWitness(t, req, reg, raw, p)
		req.Raw = nil
		r, e := VerifyQueryWitness(ctxT(t), req)
		want := AssessBlocked
		if kind == "pattern" || kind == "first-order" {
			want = AssessFail
		}
		if e != nil || r.Assessment != want {
			t.Fatalf("evaluated %s: %+v %v", kind, r, e)
		}
	}
}
