package kit

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"fmt"
	"reflect"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// Retain the same checked native execution; semantic consumers must not evaluate it again.
type semanticNativeCache struct {
	tuples    []QueryWitnessTuple
	record    eligibilityNativeRecord
	checked   []nativewire.CheckedStep
	versions  [][]byte
	points    []EditPoints
	values    []witnessEvaluation
	evaluated []bool
	ready     bool
}

func (b *eligibilityBudget) semanticNative(req QuerySemanticsRequest, profile OracleProfile, reg semanticRegistration, manifest OracleManifest, files map[string][]byte, state semanticSetState, session *eligibilitySession, cache *semanticNativeCache, res *QuerySemanticsResult, add func(string, string, string)) *Error {
	cache.tuples = reg.Required
	if state.capture == nil || state.capture.Spec == nil || state.capture.Spec.Stdin == nil || state.execution == nil {
		res.Tuple.Blocked, res.Tuple.Unprocessed = res.Tuple.Required, 0
		return nil
	}
	request, e := b.nativeRequestFrame("oracle/"+state.capture.Spec.Stdin.Path, files[state.capture.Spec.Stdin.Path])
	if e != nil {
		return e
	}
	source, rawData := []byte(nil), []byte(nil)
	for _, mem := range manifest.Members {
		if mem.Role == "source" {
			source = files[mem.Path]
		}
		if mem.Role == "raw" {
			rawData = files[mem.Path]
		}
	}
	before := res.failures
	bad := func(code, at string) {
		add(code, at, "실제 request·native 결과·projection이 등록된 의미와 다르다")
	}
	if request.ID != reg.Case || request.Encoding != reg.Encoding || request.Output != OutputTree || request.Limits != reg.Limits || request.API != profile.API || len(request.Ranges) != 0 || !bytes.Equal(request.source, source) || uint64(len(source)) != reg.Input.Bytes || digestHex(source) != reg.Input.SHA256 || !semanticEditsEqual(request.edits, reg.Edits) || !slices.Equal(request.Points, profile.Native.Cases[0].Points) || request.ReuseObservation != reuseobs.Policy || request.ObservationProducerSHA256 != state.build.Identity {
		bad("SEMANTICS_ACTUAL_REQUEST", "oracle/capture")
	}
	var declarations []NativeDeclaration
	if profile.Native.Declarations != nil {
		declarations = profile.Native.Declarations.Items
	}
	if !slices.Equal(request.Declarations, declarations) || len(request.Queries) != len(reg.Queries) {
		bad("SEMANTICS_ACTUAL_REQUEST", "oracle/capture")
	} else {
		for i, q := range reg.Queries {
			if request.queries[i].ID != q.ID || !bytes.Equal(request.queries[i].Source, []byte(q.Source)) {
				bad("SEMANTICS_ACTUAL_REQUEST", q.ID)
			}
		}
	}
	bound := len(req.EligibilityApproval) != 0
	if bound {
		a := cache.record.approved
		points := make([]uint32, len(request.Points))
		for i, point := range request.Points {
			points[i] = point.Byte
		}
		if request.Protocol != a.Protocol || request.ID != a.RequestID || request.Limits.Nodes != a.NodeLimit || request.API != a.API || !slices.Equal(points, a.Points) || request.ObservationRunID != a.ObservationRunID || request.ObservationProducerSHA256 != a.ProducerSHA256 || request.ObservationHistoryStart != a.HistoryStart || a.RuntimeManifestSHA256 != reuseobs.RuntimeManifestSHA256 {
			bad("SEMANTICS_ACTUAL_APPROVED_REQUEST", "oracle/capture")
		}
	} else {
		versions, points, err := b.semanticVersions(request, reg.Limits.InputBytes)
		if err != nil {
			return asError(err)
		}
		cache.versions, cache.points = versions, points
		if rawData != nil {
			if e := b.reserveJSON("semantic-native-response", rawData, 12); e != nil {
				return e
			}
			if e := b.decode("semantic-native-response", rawData, &cache.record.raw, true); e != nil {
				return e
			}
			cache.record.haveRaw = true
			cache.record.rawBytes = uint64(len(rawData))
			cache.record.approved.ProducerSHA256 = request.ObservationProducerSHA256
			if e := session.nativeReservation(cache.record.raw, versions); e != nil {
				return e
			}
			expect := nativewire.Expect{Check: func() error {
				if e := b.r.check(); e != nil {
					return e
				}
				return nil
			}, Revision: request.Protocol, ID: request.ID, Nodes: request.Limits.Nodes, Queries: request.queries, API: request.API, SourceBytes: uint64(len(source)), EditPoints: points, Exit: &state.execution.ExitCode,
				Observation: &nativewire.Binding{Manifest: reuseobs.RuntimeManifestSHA256, RunID: request.ObservationRunID, ProducerSHA256: request.ObservationProducerSHA256, HistoryStart: request.ObservationHistoryStart}}
			for _, point := range request.Points {
				expect.Points = append(expect.Points, point.Byte)
			}
			for _, edit := range request.edits {
				expect.Edits = append(expect.Edits, nativewire.EditSpan{StartByte: edit.StartByte, OldEndByte: edit.OldEndByte, NewEndByte: edit.NewEndByte})
			}
			for _, version := range versions {
				expect.Sources = append(expect.Sources, nativewire.SourceID{Bytes: uint64(len(version)), SHA256: digestHex(version)})
			}
			checked, err := nativewire.Check(cache.record.raw, expect)
			if e, ok := err.(*Error); ok {
				return e
			}
			cache.checked = checked.Steps
			if err != nil {
				bad("SEMANTICS_NATIVE_INVALID", "oracle/raw")
			}
			cache.ready = err == nil && cache.record.raw.Status == StatusCompleted && len(cache.checked) == len(versions)
		}
		cache.values, cache.evaluated = make([]witnessEvaluation, len(reg.Required)), make([]bool, len(reg.Required))
	}
	significant, sizeErr := b.significantJSONBytes(files[state.local.Projection.Path])
	if sizeErr != nil {
		return sizeErr
	}
	if e := b.charge(8*significant+uint64(len(reg.Required)), 16*uint64(len(files[state.local.Projection.Path]))+512*uint64(len(reg.Required)), "semantic-projection-typed"); e != nil {
		return e
	}
	var projection witnessProjection
	if err := jsonv2.Unmarshal(files[state.local.Projection.Path], &projection); err != nil {
		return fail(KindInvalidInput, "JSON_SHAPE", "projection", err)
	}
	if projection.ID != reg.Case || projection.Input != reg.Input || projection.Encoding != reg.Encoding || projection.Status != cache.record.raw.Status || len(projection.Steps) != len(cache.checked) {
		if cache.record.haveRaw {
			bad("SEMANTICS_PROJECTION_ENVELOPE", "oracle/projection")
		}
	}
	if cache.record.haveRaw {
		if e := b.semanticProjectionInputs(cache.record.raw, state.projection, cache.record.rawBytes, add); e != nil {
			return e
		}
	}
	for i, step := range projection.Steps {
		if e := b.charge(1, 0, "semantic-projection-step"); e != nil {
			return e
		}
		if !witnessRegisteredStep(i, step.Step, step.Bytes, step.SHA256, step.Edit, reg.QueryWitnessRegistration) || !witnessStepBinding(i, step.Step, step.Bytes, step.SHA256, step.Edit, cache.versions, cache.points, reg.Edits) {
			bad("SEMANTICS_PROJECTION_STEP", fmt.Sprintf("projection/steps/%d", i))
		}
		if i >= len(cache.checked) {
			continue
		}
		checked := cache.checked[i]
		for j, tree := range []*witnessTreeProjection{step.Incremental, step.Fresh} {
			native := &checked.Incremental
			if j == 1 {
				native = checked.Fresh
			}
			if tree == nil || native == nil {
				if (tree == nil) != (native == nil) {
					bad("SEMANTICS_PROJECTION_TREE", "projection")
				}
				continue
			}
			if native.Form != "full" || tree.Form != "full" || tree.Tree == nil || tree.Status != native.Status || tree.Code != native.Code || tree.Count != native.Wire.DescendantCount || tree.HasError != native.Wire.HasError || tree.Digest != native.Wire.Digest || tree.Tree.Schema != TreeSchema || tree.Tree.Status != native.Status || tree.Tree.Input.Encoding != reg.Encoding || tree.Tree.Input.Bytes != step.Bytes || tree.Tree.Input.SHA256 != step.SHA256 || CompareTrees(native.Nodes, tree.Tree.Nodes) != nil || len(tree.Queries) != len(native.Queries) {
				bad("SEMANTICS_PROJECTION_TREE", fmt.Sprintf("projection/steps/%d", i))
			}
			if !bound && native.Form == "full" && i < len(cache.versions) {
				if len(native.Nodes) == 0 || uint64(native.Nodes[0].EndByte) != uint64(len(cache.versions[i])) || !witnessCST(native.Nodes) || witnessPoints(native.Nodes, reg.Encoding, cache.versions[i], b.r) != "" {
					bad("SEMANTICS_NATIVE_CST", "oracle/raw")
				}
				if e := b.r.check(); e != nil {
					return e
				}
			}
		}
		if i < len(cache.record.raw.Steps) {
			observed, e := b.semanticAST("native-observations", cache.record.raw.Steps[i].Observations)
			if e != nil {
				return e
			}
			steps := jvMember(state.projection, "steps")
			same, e := b.semanticValuesEqual(observed, jvMember(steps.vals[i], "observations"))
			if e != nil {
				return e
			}
			if !same {
				bad("SEMANTICS_PROJECTION_OBSERVATIONS", "projection")
			}
		}
	}
	envelopeOK := before == res.failures
	for i, tuple := range reg.Required {
		if e := b.charge(1, 0, "semantic-tuple"); e != nil {
			return e
		}
		at := fmt.Sprintf("required/%d", i)
		failures := res.failures
		ready := envelopeOK && cache.ready && res.BindingValid && bound && session.tuples[i] == "verified"
		tupleErr := func() *Error {
			var tree *nativewire.Tree
			var projected *witnessTreeProjection
			if int(tuple.Step) < len(cache.checked) {
				tree = &cache.checked[tuple.Step].Incremental
				if tuple.TreeRole == "fresh" {
					tree = cache.checked[tuple.Step].Fresh
				}
			}
			if int(tuple.Step) < len(projection.Steps) {
				projected = projection.Steps[tuple.Step].Incremental
				if tuple.TreeRole == "fresh" {
					projected = projection.Steps[tuple.Step].Fresh
				}
			}
			qi := slices.IndexFunc(reg.Queries, func(q QueryWitnessQuery) bool { return q.ID == tuple.Query })
			if tree == nil || projected == nil || tree.Form != "full" || projected.Tree == nil || qi < 0 || qi >= len(tree.Queries) || qi >= len(projected.Queries) || int(tuple.Step) >= len(cache.versions) {
				ready = false
			} else {
				q, p := tree.Queries[qi], projected.Queries[qi]
				meta := reg.Queries[qi].Metadata
				if meta == nil || q.Patterns != meta.Patterns || !slices.Equal(q.CaptureNames, meta.CaptureNames) || !reflect.DeepEqual(q.Predicates, meta.Predicates) {
					bad("SEMANTICS_METADATA_MISMATCH", at)
				}
				if !bound {
					reservation := instrumentedReference{}
					reservation.Tree.Nodes, reservation.Query.Predicates, reservation.Query.CaptureNames = tree.Nodes, q.Predicates, q.CaptureNames
					for _, row := range q.Captures {
						reservation.Query.Structural = append(reservation.Query.Structural, witnessCapture(q, row))
					}
					reservation.Query.Evaluated = tuple.Expected
					if e := b.reserveReference(&reservation, [][]byte{cache.versions[tuple.Step]}); e != nil {
						return e
					}
					value, err := witnessEvaluate(b.r, q, reg.Encoding, cache.versions[tuple.Step])
					if err != nil {
						return asError(err)
					}
					cache.values[i], cache.evaluated[i] = value, true
				}
				structural := make([]Capture, 0, len(q.Captures))
				for _, row := range q.Captures {
					structural = append(structural, witnessCapture(q, row))
				}
				if CompareCaptures(structural, tuple.Structural) != nil {
					bad("SEMANTICS_STRUCTURAL_MISMATCH", at)
				}
				if i >= len(cache.evaluated) || !cache.evaluated[i] {
					ready = false
				} else {
					value := cache.values[i]
					if CompareCaptures(value.captures, tuple.Expected) != nil || !value.matches(p) || !witnessQueryPassthrough(p, q, reg.Encoding) || !witnessProjectionQueryHeader(p, reg.Encoding) || p.SHA256 != digestHex([]byte(reg.Queries[qi].Source)) {
						bad("SEMANTICS_EVALUATED_MISMATCH", at)
					}
					ready = ready && q.Status == StatusCompleted && !q.Partial && value.status == StatusCompleted && value.evaluation == "EVALUATED" && value.captures != nil && projection.Status == StatusCompleted
				}
			}
			return nil
		}()
		if tupleErr != nil {
			if res.failures > failures || bound && session.tuples[i] == "failed" {
				res.Tuple.Unprocessed--
				res.Tuple.Failed++
				res.Semantic = AssessFail
			}
			return tupleErr
		}

		res.Tuple.Unprocessed--
		switch {
		case res.failures > failures || bound && session.tuples[i] == "failed":
			res.Tuple.Failed++
			res.Semantic = AssessFail
		case ready:
			res.Tuple.Verified++
		default:
			res.Tuple.Blocked++
		}
	}
	return nil
}

// Base replay consumes these projection fields, so bind them to the retained raw too.
func (b *eligibilityBudget) semanticProjectionInputs(raw nativewire.Response, projection *jv, rawBytes uint64, add func(string, string, string)) *Error {
	status := jvMember(projection, "execution_status")
	if raw.Status != StatusCompleted || status == nil || status.s != StatusCompleted {
		return nil
	}
	if e := b.charge(16*rawBytes+1, 16*rawBytes, "semantic-base-inputs"); e != nil {
		return e
	}
	compare := func(actual any, recorded *jv, path string) *Error {
		data, err := jsonv2.Marshal(actual)
		if err != nil {
			return fail(KindInvalidInput, "JSON_SHAPE", path, err)
		}
		v, e := b.semanticAST(path, data)
		if e != nil {
			return e
		}
		if v.kind == 'n' && recorded == nil {
			return nil // optional null declarations are omitted by the producer
		}
		same, e := b.semanticValuesEqual(v, recorded)
		if e != nil {
			return e
		}
		if !same {
			add("SEMANTICS_PROJECTION_BASE_INPUT", path, "기본 판정 입력이 실제 native 응답과 다르다")
		}
		return nil
	}
	for _, field := range []struct {
		key   string
		value any
	}{{"producer", raw.Producer}, {"response_status", raw.Status}, {"response_code", raw.Code}} {
		if e := compare(field.value, jvMember(projection, field.key), "projection/"+field.key); e != nil {
			return e
		}
	}
	steps := jvMember(projection, "steps")
	for i, step := range raw.Steps {
		if steps == nil || i >= len(steps.vals) {
			break // the envelope check reports a different step count
		}
		p := steps.vals[i]
		var route any
		if step.Route != nil {
			route = struct {
				nativewire.Route
				Proven bool `json:"proven"`
			}{*step.Route, step.Route.EditHasChanges && step.Route.ReusedNodes > 0 && step.Route.FreshReusedNodes == 0}
		}
		at := fmt.Sprintf("projection/steps/%d", i)
		if e := compare(route, jvMember(p, "route"), at+"/route"); e != nil {
			return e
		}
		if restarted := jvMember(p, "restarted"); restarted != nil && restarted.kind != 'f' {
			add("SEMANTICS_PROJECTION_BASE_INPUT", at+"/restarted", "단일 native 실행에는 restart가 없다")
		}
		for j, tree := range []*nativewire.WireTree{&step.Incremental, step.Fresh} {
			role := "incremental"
			if j == 1 {
				role = "fresh"
			}
			if tree != nil {
				if e := compare(tree.Declarations, jvMember(jvMember(p, role), "declarations"), at+"/"+role+"/declarations"); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
