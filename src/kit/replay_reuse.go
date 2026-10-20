package kit

import (
	"cmp"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// The metadata pass preserves JSON member-order independence without retaining CSTs.
func (x *replayEnv) readObservationTop(path string, top *nativeResult) *Error {
	m, e := x.stream(path)
	if e != nil {
		return e
	}
	e = x.streamTop(m, "cases", func(name string, raw jsontext.Value) *Error {
		switch name {
		case "result_schema", "reuse_observation_policy", "observation_run_id", "build":
			return decodeRecord(path, append(append([]byte(`{"`+name+`":`), raw...), '}'), top)
		}
		return nil
	}, func(_ jsontext.Value, _ int) *Error { return x.r.check() })
	if e != nil {
		return e
	}
	schema := "tsgk-incremental-result/r2"
	if x.mandatoryObservation {
		schema = "tsgk-incremental-result/r3"
	}
	if top.ResultSchema != schema || top.ReuseObservationPolicy != reuseobs.Policy || len(top.ObservationRunID) == 0 || len(top.ObservationRunID) > 128 {
		return fail(KindUnsupported, "OBSERVATION_SCHEMA_UNSUPPORTED", path, nil)
	}
	return nil
}

// observationProducer returns the runtime manifest a recorded producer declared and whether
// this policy reads observations bound to it.
func observationProducer(raw jsontext.Value) (string, bool) {
	var cap struct {
		Policy                string `json:"policy"`
		RuntimeManifestSHA256 string `json:"runtime_manifest_sha256"`
	}
	ok := jsonv2.Unmarshal(raw, &cap, jsonv2.RejectUnknownMembers(true)) == nil && cap.Policy == reuseobs.Policy && reuseobs.VerifiedRuntimeManifest(cap.RuntimeManifestSHA256)
	return cap.RuntimeManifestSHA256, ok
}

// recordManifest picks the runtime manifest a case's observations bind to: the one its
// producer declared, or for a case without native parse (no producer) the one its first
// host marker declared. declared reports a readable producer declaration; an unreadable one
// fails the capability check, and an unverified marker manifest fails each binding.
func recordManifest(c *rcCase) (manifest string, declared bool) {
	if c.Producer != nil {
		if m, ok := observationProducer(c.Producer.ReuseObservation); ok {
			return m, true
		}
		return reuseobs.RuntimeManifestSHA256, false
	}
	for _, s := range c.Steps {
		var markers []struct {
			RuntimeManifestSHA256 string `json:"runtime_manifest_sha256"`
		}
		if len(s.Observations) > 0 && jsonv2.Unmarshal(s.Observations, &markers) == nil && len(markers) > 0 {
			if reuseobs.VerifiedRuntimeManifest(markers[0].RuntimeManifestSHA256) {
				return markers[0].RuntimeManifestSHA256, false
			}
			break
		}
	}
	return reuseobs.RuntimeManifestSHA256, false
}

// responseManifest is the readable manifest a raw response declared, or "" if none.
func responseManifest(raw jsontext.Value) string {
	if m, ok := observationProducer(raw); ok {
		return m
	}
	return ""
}

func decodeObservations(raw jsontext.Value, b reuseobs.Binding, native bool) ([]reuseobs.Observation, error) {
	var values []jsontext.Value
	if err := jsonv2.Unmarshal(raw, &values); err != nil || len(values) != 1+int(min(b.Step, 1)) {
		return nil, fmt.Errorf("observation roles")
	}
	var observations []reuseobs.Observation
	for i, value := range values {
		b.Role = "initial"
		if b.Step > 0 {
			b.Role = "incremental"
			if i == 1 {
				b.Role = "fresh"
			}
		}
		o, err := reuseobs.Decode(value, b)
		if err != nil {
			return nil, err
		}
		if native == (o.Reason == "NO_NATIVE_PARSE") {
			return nil, fmt.Errorf("observation origin")
		}
		observations = append(observations, o)
	}
	return observations, nil
}

type observationStep struct {
	observations []reuseobs.Observation
	edit         *rcEdit
}

func (x *replayEnv) replayObservations(c *rcCase, index int, top nativeResult, registered *IncrementalCase) *Error {
	g := x.gate("reuse-observation")
	if registered == nil {
		g.fail(c.ID, "OBSERVATION_CASE_UNBOUND", "관측 case의 workload가 없다")
		return nil
	}
	g.check(c.ExecutionStatus != StatusCompleted || registered != nil && len(c.Steps) == len(registered.Edits)+1, c.ID, "OBSERVATION_STEPS_MISSING", "완료 사례에 등록 history의 관측 step이 없다")
	build := ""
	if top.Build != nil {
		build = top.Build.Identity
	}
	manifest, declared := recordManifest(c)
	var all []reuseobs.Observation
	groups := map[int][]observationStep{}
	previous := ""
	var versions [][]byte
	var points []EditPoints
	source := registered.SvcSource
	path := fmt.Sprintf("sources/%05d-%s.bin", index, c.ID)
	if member, ok := x.members[path]; ok && member.Role == "case-source" {
		var e *Error
		source, e = x.member(path)
		if e != nil {
			return e
		}
	}
	if source != nil {
		var err error
		if digestHex(source) == registered.Input.SHA256 && uint64(len(source)) == registered.Input.Bytes {
			versions, points, err = ApplyEdits(registered.Encoding, source, registered.Edits, x.nativeInputBytes)
		} else {
			err = fmt.Errorf("source witness identity")
		}
		g.check(err == nil, c.ID, "OBSERVATION_SOURCE_MISMATCH", "원 입력 witness가 등록 입력·edit와 다르다")
	}
	for _, s := range c.Steps {
		native := s.Incremental != nil || s.Composite != nil && s.Composite.Inline != nil
		editOK := s.Step == 0 && s.Edit == nil
		if s.Step > 0 && registered != nil && s.Step <= len(registered.Edits) && s.Edit != nil {
			e, got := registered.Edits[s.Step-1], s.Edit
			editOK = got.StartByte != nil && got.OldEndByte != nil && got.NewEndByte != nil &&
				*got.StartByte == e.StartByte && *got.OldEndByte == e.OldEndByte && *got.NewEndByte == e.NewEndByte &&
				len(got.StartPoint) == 2 && len(got.OldEndPoint) == 2 && len(got.NewEndPoint) == 2
		}
		g.check(editOK && s.SourceBytes <= x.nativeInputBytes, c.ID, "OBSERVATION_EDIT_UNBOUND", "관측 step의 edit가 등록 workload와 다르거나 입력 상한을 넘었다")
		if versions != nil {
			g.check(s.Step >= 0 && s.Step < len(versions) && s.SourceSHA256 == digestHex(versions[s.Step]) && s.SourceBytes == uint64(len(versions[s.Step])), c.ID, "OBSERVATION_SOURCE_MISMATCH", "관측 입력이 원 입력·등록 edit에서 다시 만든 bytes와 다르다")
		}
		if s.Step > 0 {
			pg := x.gate("edit-points")
			if versions == nil {
				pg.recorded()
			} else {
				valid := editOK && s.Step <= len(points)
				if valid {
					p := points[s.Step-1]
					valid = reflect.DeepEqual(s.Edit.StartPoint, []uint32{p.Start.Row, p.Start.Column}) && reflect.DeepEqual(s.Edit.OldEndPoint, []uint32{p.OldEnd.Row, p.OldEnd.Column}) && reflect.DeepEqual(s.Edit.NewEndPoint, []uint32{p.NewEnd.Row, p.NewEnd.Column})
				}
				pg.check(valid, c.ID, "OBSERVATION_EDIT_POINT_MISMATCH", "edit point가 원 입력·등록 edit의 재계산과 다르다")
			}
		}
		local, start := s.Step, 0
		if native && len(c.Segments) > 0 {
			found := false
			for _, segment := range c.Segments {
				if s.Step >= segment.StartStep && s.Step < segment.EndStep {
					start = segment.StartStep
					local -= start
					found = true
					break
				}
			}
			if !g.check(found, c.ID, "OBSERVATION_SEGMENT_UNBOUND", "관측의 native segment를 찾을 수 없다") {
				continue
			}
		}
		b := reuseobs.Binding{RuntimeManifestSHA256: manifest, RequestID: c.ID,
			HistoryStart: uint64(start),
			RunID:        top.ObservationRunID, ProducerSHA256: build, Step: uint64(local),
			SourceSHA256: s.SourceSHA256, SourceBytes: s.SourceBytes, PreviousSourceSHA256: previous}
		observations, err := decodeObservations(s.Observations, b, native)
		previous = s.SourceSHA256
		if !g.check(err == nil, c.ID, "OBSERVATION_INVALID", "관측의 상태·역할·입력·실행 결속이 유효하지 않다") {
			continue
		}
		all = append(all, observations...)
		if native {
			edit := s.Edit
			if local == 0 {
				edit = nil
			}
			groups[start] = append(groups[start], observationStep{observations, edit})
		}
	}
	var claim string
	want := reuseobs.Judge(all)
	if x.mandatoryObservation {
		want, _ = mandatoryObservations(c)
	}
	g.check(len(c.ReuseObservationClaim) > 0 && c.ReuseObservationClaim[0] == '"' && jsonv2.Unmarshal(c.ReuseObservationClaim, &claim) == nil && claim == want, c.ID, "OBSERVATION_CLAIM_MISMATCH", "관측 판정이 별도 축의 재계산과 다르다")
	if len(groups) > 0 {
		g.check(declared, c.ID, "OBSERVATION_CAPABILITY_MISMATCH", "producer 관측 정책이 다르다")
	}
	starts := slices.Sorted(maps.Keys(groups))
	dir := cmp.Or(x.responseDir, "responses/")
	for path, end := range steplessEnds(c, groups, dir, index) {
		if x.members[path].Role == "response" {
			if e := x.compareObservationResponse(path, c, nil, top, manifest, "", end); e != nil {
				return e
			}
		}
	}
	queries := x.observationWire == "tsgk-native/r4" // only r4 responses carry queries
	for i, start := range starts {
		steps := groups[start]
		path := fmt.Sprintf("%s%05d-%s.json", dir, index, c.ID)
		if len(c.Segments) > 0 {
			path = fmt.Sprintf("%s%05d-%s-segment-%05d.json", dir, index, c.ID, start)
		}
		open := "" // r4: the recorded unfinished last step of an early case, by its trees and queries
		if x.mandatoryObservation && c.ExecutionStatus != StatusCompleted && i == len(starts)-1 && len(c.Steps) > 0 {
			last := c.Steps[len(c.Steps)-1]
			for _, t := range []*rcTree{last.Incremental, last.Fresh} {
				if t != nil && open == "" {
					var statuses []string
					for _, q := range t.Queries {
						if queries {
							statuses = append(statuses, q.Status)
						}
					}
					open = openStatus(t.Status, statuses)
				}
			}
		}
		if e := x.compareObservationResponse(path, c, steps, top, manifest, open, [2]string{}); e != nil {
			return e
		}
	}
	return nil
}

// steplessEnds names each raw response of a case that dropped every step (Codex A217F/A217G)
// with the end it must record: for a case without SVC segments and steps, the recorded
// response status and code; for the last native segment of an early SVC case that recorded
// no step of it, the case's status and code, which the segment's run set.
func steplessEnds(c *rcCase, groups map[int][]observationStep, dir string, index int) map[string][2]string {
	out := map[string][2]string{}
	if len(c.Segments) == 0 {
		if len(c.Steps) == 0 {
			out[fmt.Sprintf("%s%05d-%s.json", dir, index, c.ID)] = [2]string{c.ResponseStatus, c.ResponseCode}
		}
		return out
	}
	if last := c.Segments[len(c.Segments)-1]; c.ExecutionStatus != StatusCompleted && groups[last.StartStep] == nil {
		out[fmt.Sprintf("%s%05d-%s-segment-%05d.json", dir, index, c.ID, last.StartStep)] = [2]string{c.ExecutionStatus, c.Code}
	}
	return out
}

// steplessEnd reports whether live Check accepts a response without steps that ends with
// status and code: an output, allocation or language limit, or a query time limit of an
// r4 request with registered queries.
func steplessEnd(status, code string, queries bool) bool {
	return (status == StatusResourceLimit || status == StatusFailed) &&
		(code == "OUTPUT_LIMIT" || code == "ALLOCATION_LIMIT" || code == "LANGUAGE_INCOMPATIBLE" || queries && code == "QUERY_TIME_LIMIT" && status == StatusResourceLimit)
}

// Only the versioned observation projection is decoded here; native trees are still
// judged from the recorded public trees by the existing twelve-field comparator. Under r4
// the last response of a case that ended early may keep its unfinished last step, which
// steps_completed omits: open is that step's recorded noncomplete tree status, "" otherwise.
// A response that dropped every step (want nil) must end with end, its recorded status and code.
func (x *replayEnv) compareObservationResponse(path string, c *rcCase, want []observationStep, top nativeResult, manifest, open string, end [2]string) *Error {
	id := c.ID
	g := x.gate("reuse-observation")
	if member, ok := x.members[path]; !ok || member.Role != "response" {
		g.fail(id, "OBSERVATION_RESPONSE_MISSING", "관측 원 응답이 등록되지 않았다")
		return nil
	}
	m, e := x.stream(path)
	if e != nil {
		return e
	}
	var wire struct {
		Status         string  `json:"status"`
		Code           string  `json:"code"`
		Protocol       string  `json:"protocol"`
		ID             string  `json:"id"`
		Complete       *bool   `json:"complete"`
		StepsCompleted *uint64 `json:"steps_completed"`
		Producer       struct {
			ReuseObservation jsontext.Value `json:"reuse_observation"`
		} `json:"producer"`
	}
	count, rawOpen := 0, ""
	e = x.streamTop(m, "steps", func(name string, raw jsontext.Value) *Error {
		switch name {
		case "protocol", "id", "producer", "complete", "steps_completed", "status", "code":
			return decodeRecord(path, append(append([]byte(`{"`+name+`":`), raw...), '}'), &wire)
		}
		return nil
	}, func(raw jsontext.Value, i int) *Error {
		count++
		var step struct {
			Step         int            `json:"step"`
			SourceBytes  uint64         `json:"source_bytes"`
			SourceSHA256 string         `json:"source_sha256"`
			Observations jsontext.Value `json:"observations"`
			Edit         *rcEdit        `json:"edit"`
			Incremental  *treeStatus    `json:"incremental"`
			Fresh        *treeStatus    `json:"fresh"`
		}
		if e := decodeRecord(path, raw, &step); e != nil {
			return e
		}
		if i >= len(want) {
			g.fail(id, "OBSERVATION_PROJECTION_MISMATCH", "원 응답에 추가 관측 step이 있다")
			return nil
		}
		b := reuseobs.Binding{RuntimeManifestSHA256: manifest, RequestID: id, RunID: top.ObservationRunID,
			HistoryStart:   want[i].observations[0].HistoryStart,
			ProducerSHA256: top.Build.Identity, Step: uint64(i), SourceSHA256: step.SourceSHA256, SourceBytes: step.SourceBytes}
		if i > 0 {
			b.PreviousSourceSHA256 = want[i-1].observations[0].SourceSHA256
		}
		rawOpen = ""
		for _, t := range []*treeStatus{step.Incremental, step.Fresh} {
			if t != nil && rawOpen == "" {
				var statuses []string
				for _, q := range t.Queries {
					if x.observationWire == "tsgk-native/r4" { // only r4 responses carry queries
						statuses = append(statuses, q.Status)
					}
				}
				rawOpen = openStatus(t.Status, statuses)
			}
		}
		got, err := decodeObservations(step.Observations, b, true)
		g.check(err == nil && step.Step == i && reflect.DeepEqual(got, want[i].observations) && reflect.DeepEqual(step.Edit, want[i].edit), id, "OBSERVATION_PROJECTION_MISMATCH", "관측 보고서·edit가 원 응답과 다르다")
		return x.r.check()
	})
	if e != nil {
		return e
	}
	counted := wire.StepsCompleted != nil && stepsCounted(*wire.StepsCompleted, count, wire.Status, open, rawOpen)
	if want == nil {
		// live Check leaves steps_completed of a response that dropped its steps unconstrained
		counted = wire.StepsCompleted != nil && [2]string{wire.Status, wire.Code} == end && steplessEnd(wire.Status, wire.Code, wire.Protocol == "tsgk-native/r4" && x.observationQueries)
	}
	g.check(count == len(want) && wire.Complete != nil && *wire.Complete && counted && wire.Protocol == cmp.Or(x.observationWire, "tsgk-native/r3") && wire.ID == id && responseManifest(wire.Producer.ReuseObservation) == manifest, id, "OBSERVATION_RESPONSE_UNBOUND", "원 응답의 완결성·revision·case·정책·관측 수가 다르다")
	x.observationResponses[path], x.consumed[path] = true, true
	return nil
}

// mandatoryObservations recomputes the r4 case claim (#217) and each step's claim from the
// recorded observations with the live rule (reuseobs.CaseClaim); their binding and agreement
// with the raw responses are the reuse-observation gate's (replayObservations). An unreadable
// step list reads as missing.
func mandatoryObservations(c *rcCase) (string, []string) {
	steps := make([]reuseobs.Step, len(c.Steps))
	claims := make([]string, len(c.Steps))
	for i, s := range c.Steps {
		var observations []reuseobs.Observation
		if len(s.Observations) > 0 && jsonv2.Unmarshal(s.Observations, &observations) != nil {
			observations = nil
		}
		steps[i] = reuseobs.Step{Native: s.Incremental != nil || s.Composite != nil && s.Composite.Inline != nil, Edit: s.Step > 0 && !s.Restarted, Observations: observations}
		claims[i] = reuseobs.CaseClaim(steps[i:i+1], true)
	}
	return reuseobs.CaseClaim(steps, c.ExecutionStatus == StatusCompleted), claims
}

// stepsCounted reports whether a raw response's steps_completed fits its count steps: all of
// them, or all but the last when that step is unfinished in both the record (open, allowed
// only for the last native response of an r4 case that ended early) and the raw response
// (rawOpen, its last step's first noncomplete tree status), each the response's RESOURCE_LIMIT
// or FAILED status, as live Check requires.
func stepsCounted(completed uint64, count int, status, open, rawOpen string) bool {
	return completed == uint64(count) || count > 0 && completed == uint64(count-1) &&
		(status == StatusResourceLimit || status == StatusFailed) && open == status && rawOpen == status
}

// treeStatus is the only member of a raw response tree, and of each of its r4 queries, the
// short-count check reads.
type treeStatus struct {
	Status  string `json:"status"`
	Queries []struct {
		Status string `json:"status"`
	} `json:"queries"`
}

// openStatus is a tree's first noncomplete status as live Check counts a step unfinished:
// the tree's own, else its first RESOURCE_LIMIT or FAILED query (r4); "" when finished.
func openStatus(tree string, queries []string) string {
	if tree != StatusCompleted {
		return tree
	}
	for _, q := range queries {
		if q == StatusResourceLimit || q == StatusFailed {
			return q
		}
	}
	return ""
}
