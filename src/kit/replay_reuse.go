package kit

import (
	"bytes"
	"cmp"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
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
	if x.preprocess {
		schema = "tsgk-incremental-result/r4" // r3 with the symbol mode (#218)
	} else if x.mandatoryObservation {
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
	route        *wireRoute // the recorded route of an edit step, which r4 qualification binds to the raw step
}

// wireRoute is an edit step's route as live DecodeResponse reads it and the record keeps it.
type wireRoute struct {
	EditHasChanges    bool   `json:"edit_has_changes"`
	EditedRootEndByte uint32 `json:"edited_root_end_byte"`
	ReusedNodes       uint64 `json:"reused_nodes"`
	FreshReusedNodes  uint64 `json:"fresh_reused_nodes"`
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
			// r5 (#218): a case the run stopped on the same edit refusal, or before it could read
			// its input (the edits were never evaluated), has nothing to observe; r3 and r4 keep
			// their recorded meaning and reject it
			var ke *Error
			if x.preprocess && err != nil && c.ExecutionStatus == StatusNotRun && len(c.Steps) == 0 && (sourceReadCodes[c.Code] || errors.As(err, &ke) && ke.Code == c.Code) {
				err = nil
			}
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
			var route *wireRoute
			if s.Route != nil {
				route = &s.Route.wireRoute
			}
			groups[start] = append(groups[start], observationStep{observations, edit, route})
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
	// qualification evidence is a new run, which live Check binds to the current runtime
	// manifest; earlier verified manifests stay readable only for recorded replays (review on #250)
	g.check(x.observationWire != "tsgk-native/r4" || manifest == reuseobs.RuntimeManifestSHA256, c.ID, "OBSERVATION_CAPABILITY_MISMATCH", "qualification 관측은 현재 runtime manifest여야 한다")
	starts := slices.Sorted(maps.Keys(groups))
	dir := cmp.Or(x.responseDir, "responses/")
	// the source length and API point bytes of the request of the response starting at a
	// step, as live Check binds them: the registered input, else the recomputed or recorded version
	sourceAt := func(start int) *uint64 {
		if start == 0 {
			return &registered.Input.Bytes
		}
		if start > 0 && start < len(versions) {
			n := uint64(len(versions[start]))
			return &n
		}
		for _, s := range c.Steps {
			if s.Step == start {
				return &s.SourceBytes
			}
		}
		return nil
	}
	var apiPoints []uint32
	for _, p := range registered.Points {
		apiPoints = append(apiPoints, p.Byte)
	}
	// expect is the request of the r4 response starting at step start as live Check binds it
	// (#251): the registered edits of its segment, the recomputed versions and points or else
	// the recorded source identities; nil off r4 or when the recorded segment bounds or the
	// source length cannot form it
	expect := func(start int) *nativewire.Expect {
		end := len(registered.Edits) + 1
		if len(c.Segments) > 0 {
			end = -1
			for _, s := range c.Segments {
				if s.StartStep == start {
					end = s.EndStep
					break
				}
			}
		}
		source := sourceAt(start)
		if x.observationWire != "tsgk-native/r4" || source == nil || start < 0 || start >= end || end-1 > len(registered.Edits) || versions != nil && (end-1 > len(points) || end > len(versions)) {
			return nil
		}
		e := &nativewire.Expect{Revision: "tsgk-native/r4", ID: c.ID, Nodes: x.observationNodes, Queries: x.observationSources, API: x.observationAPI,
			Points: apiPoints, SourceBytes: *source, Observation: &nativewire.Binding{Manifest: manifest, RunID: top.ObservationRunID, ProducerSHA256: build, HistoryStart: uint64(start)}}
		for _, ed := range registered.Edits[start : end-1] {
			e.Edits = append(e.Edits, nativewire.EditSpan{StartByte: ed.StartByte, OldEndByte: ed.OldEndByte, NewEndByte: ed.NewEndByte})
		}
		if versions != nil {
			e.EditPoints = points[start : end-1]
		}
		for step := start; step < end; step++ {
			if versions != nil {
				e.Sources = append(e.Sources, nativewire.SourceID{Bytes: uint64(len(versions[step])), SHA256: digestHex(versions[step])})
				continue
			}
			// without a source witness the record binds the version, as edit-points stays recorded()
			k := slices.IndexFunc(c.Steps, func(s rcStep) bool { return s.Step == step })
			if k < 0 {
				break
			}
			e.Sources = append(e.Sources, nativewire.SourceID{Bytes: c.Steps[k].SourceBytes, SHA256: c.Steps[k].SourceSHA256})
		}
		return e
	}
	for path, end := range steplessEnds(c, groups, dir, index) {
		start := 0
		if len(c.Segments) > 0 {
			start = c.Segments[len(c.Segments)-1].StartStep
		}
		// a recorded stepless limit end came from a driver response, which must be present
		// by the recorded response end or the case's own, so erasing one of them does not skip it (review on #250)
		queries := x.observationWire == "tsgk-native/r4" && x.observationQueries
		if x.members[path].Role == "response" || steplessEnd(end[0], end[1], queries) || steplessEnd(c.ExecutionStatus, c.Code, queries) {
			if e := x.compareObservationResponse(path, c, nil, top, manifest, [2]string{}, end, expect(start)); e != nil {
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
		var open [2]string // r4: the recorded unfinished last step of an early case, by its trees and queries
		if x.mandatoryObservation && c.ExecutionStatus != StatusCompleted && i == len(starts)-1 && len(c.Steps) > 0 {
			last := c.Steps[len(c.Steps)-1]
			for _, t := range []*rcTree{last.Incremental, last.Fresh} {
				if t != nil && open == ([2]string{}) {
					open = recordedEnd(t, queries)
				}
			}
		}
		if e := x.compareObservationResponse(path, c, steps, top, manifest, open, responseEnd(c, start), expect(start)); e != nil {
			return e
		}
	}
	return nil
}

// recordedEnd is a recorded tree's first noncomplete status and code: the tree's own, else
// its first RESOURCE_LIMIT or FAILED query when the wire carries queries; zero when finished.
func recordedEnd(t *rcTree, queries bool) [2]string {
	if t.Status != StatusCompleted {
		return [2]string{t.Status, t.Code}
	}
	for _, q := range t.Queries {
		if queries && (q.Status == StatusResourceLimit || q.Status == StatusFailed) {
			return [2]string{q.Status, q.Code}
		}
	}
	return [2]string{}
}

// liveSteps reports whether a raw response's step ends fit live Check: every step but the
// last finished, steps_completed counting the finished steps, and an unfinished last step
// only in a response that ends with that step's first noncomplete status and code (Codex
// review on #250).
func liveSteps(opens [][2]string, completed uint64, end [2]string) bool {
	n := len(opens)
	for i := 0; i+1 < n; i++ {
		if opens[i] != ([2]string{}) {
			return false
		}
	}
	if n > 0 && opens[n-1] != ([2]string{}) {
		return completed == uint64(n-1) && end == opens[n-1]
	}
	return completed == uint64(n) && end == [2]string{StatusCompleted, ""}
}

// liveEnd reports whether a response end with steps is one live RunCase records: COMPLETED
// without a code, or a RESOURCE_LIMIT or FAILED end with a code that is the case's own end.
func liveEnd(end [2]string, c *rcCase) bool {
	if end == [2]string{StatusCompleted, ""} {
		return true
	}
	return (end[0] == StatusResourceLimit || end[0] == StatusFailed) && end[1] != "" && end == [2]string{c.ExecutionStatus, c.Code}
}

// steplessEnds names each raw response of a case that dropped every step (Codex A217F/A217G)
// with the end it must record: for a case without SVC segments and steps, the recorded
// response status and code; for the last native segment of an early SVC case that recorded
// no step of it, the case's status and code, which the segment's run set.
func steplessEnds(c *rcCase, groups map[int][]observationStep, dir string, index int) map[string][2]string {
	out := map[string][2]string{}
	if len(c.Segments) == 0 {
		if len(c.Steps) == 0 {
			out[fmt.Sprintf("%s%05d-%s.json", dir, index, c.ID)] = responseEnd(c, 0)
		}
		return out
	}
	if last := c.Segments[len(c.Segments)-1]; c.ExecutionStatus != StatusCompleted && groups[last.StartStep] == nil {
		out[fmt.Sprintf("%s%05d-%s-segment-%05d.json", dir, index, c.ID, last.StartStep)] = responseEnd(c, last.StartStep)
	}
	return out
}

// responseEnd is the status and code the native response starting at step start must
// record: a plain case's response status and code; an SVC segment's COMPLETED, except the
// last segment of a case that ended early, whose run set the case's status and code.
func responseEnd(c *rcCase, start int) [2]string {
	if len(c.Segments) == 0 {
		return [2]string{c.ResponseStatus, c.ResponseCode}
	}
	if c.ExecutionStatus != StatusCompleted && start == c.Segments[len(c.Segments)-1].StartStep {
		return [2]string{c.ExecutionStatus, c.Code}
	}
	return [2]string{StatusCompleted, ""}
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
// An r4 response is judged by the live wire rules (nativewire) against expect, its request
// as live Check binds it (nil when the record cannot form it).
func (x *replayEnv) compareObservationResponse(path string, c *rcCase, want []observationStep, top nativeResult, manifest string, open, end [2]string, expect *nativewire.Expect) *Error {
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
		Status         string   `json:"status"`
		Code           *string  `json:"code"`
		Protocol       string   `json:"protocol"`
		ID             string   `json:"id"`
		Complete       *bool    `json:"complete"`
		StepsCompleted *uint64  `json:"steps_completed"`
		SourceBytes    *uint64  `json:"source_bytes"`
		Producer       struct { // typed as the replay always read them, also for r3 (Codex A251C)
			Query            string         `json:"query"`
			API              *string        `json:"api"`
			Predicates       *string        `json:"predicates"`
			SymbolCount      *uint32        `json:"symbol_count"`
			FieldCount       *uint32        `json:"field_count"`
			ReuseObservation jsontext.Value `json:"reuse_observation"`
		} `json:"producer"`
	}
	r4 := x.observationWire == "tsgk-native/r4"
	// r4 (#251): the response as live DecodeResponse reads it, member by member; its first
	// strict decode failure; each checked step's completion until the first step failure,
	// after which no further step is checked; and whether every tree kept its code member,
	// which the strict decode reads as "" when absent (Codex PLAN251C)
	var h nativewire.Response
	var malformed, stepErr error
	var done []bool
	var tally nativewire.Tally
	codes := true
	count, rawOpen, wireEnds := 0, [2]string{}, true
	var opens [][2]string // r3: each raw step's first noncomplete tree status and code
	e = x.streamTop(m, "steps", func(name string, raw jsontext.Value) *Error {
		switch name {
		case "protocol", "id", "complete", "steps_completed", "source_bytes", "status", "code", "producer":
			member := append(append([]byte(`{"`+name+`":`), raw...), '}') // name is one of the fixed members above
			if r4 && malformed == nil {
				malformed = jsonv2.Unmarshal(member, &h, jsonv2.RejectUnknownMembers(true))
			}
			return decodeRecord(path, member, &wire)
		}
		if r4 && malformed == nil {
			malformed = fmt.Errorf("unknown member %q", name)
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
		var slots struct {
			Route jsontext.Value `json:"route"`
		}
		// the raw route the projection compares; a present route that does not read as one
		// never reads as absent (Codex A251C), and its strict shape is the live rules'
		var rawRoute *wireRoute
		routeRead := true
		if jsonv2.Unmarshal(raw, &slots) == nil && len(slots.Route) > 0 && string(slots.Route) != "null" {
			var route wireRoute
			rawRoute, routeRead = &route, jsonv2.Unmarshal(slots.Route, &route) == nil
		}
		if r4 {
			for _, t := range []*treeStatus{step.Incremental, step.Fresh} {
				codes = codes && (t == nil || t.Code != nil)
			}
			if malformed == nil {
				var s nativewire.Step
				if err := jsonv2.Unmarshal(raw, &s, jsonv2.RejectUnknownMembers(true)); err != nil {
					malformed = err
				} else if stepErr == nil && expect != nil {
					cs, err := nativewire.CheckStep(i, s, *expect)
					if err == nil {
						done = append(done, cs.Done(i))
						tally.Add(i, cs)
					}
					stepErr = err
				}
			}
		} else {
			var incEnd, freshEnd [2]string
			incOK, freshOK := step.Incremental != nil, step.Fresh == nil
			if step.Incremental != nil {
				incEnd, incOK = rawTreeEnd(step.Incremental)
			}
			if step.Fresh != nil {
				freshEnd, freshOK = rawTreeEnd(step.Fresh)
			}
			rawOpen = cmp.Or(incEnd, freshEnd)
			// the tree slots live Check requires: an incremental tree on every step, no fresh tree
			// on the first, and a fresh tree on an edit step unless its incremental tree ended it
			wireEnds = wireEnds && incOK && freshOK && (i == 0) == (step.Fresh == nil) || wireEnds && incOK && i > 0 && step.Fresh == nil && incEnd != ([2]string{})
			opens = append(opens, rawOpen)
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
		// r4: the raw route is the recorded one, which the route claim was judged from (review on #250)
		routeBound := !r4 || routeRead && reflect.DeepEqual(rawRoute, want[i].route)
		got, err := decodeObservations(step.Observations, b, true)
		g.check(err == nil && step.Step == i && reflect.DeepEqual(got, want[i].observations) && reflect.DeepEqual(step.Edit, want[i].edit) && routeBound, id, "OBSERVATION_PROJECTION_MISMATCH", "관측 보고서·edit·route가 원 응답과 다르다")
		return x.r.check()
	})
	if e != nil {
		return e
	}
	wireEnd := [2]string{wire.Status, ""}
	if wire.Code != nil {
		wireEnd[1] = *wire.Code
	} else {
		wireEnds = false // live Check requires the response code member
	}
	detail := ""
	if r4 {
		// the live order: decoding, the header, then each step and its completion, then the end
		shared := malformed
		if shared == nil && expect == nil {
			shared = fmt.Errorf("request unbound")
		}
		if shared == nil {
			stop, err := nativewire.CheckHeader(h, count, *expect)
			shared = err
			for k := 0; shared == nil && !stop && k < len(done); k++ {
				shared = nativewire.CheckOpen(k, count, h.Status, done[k])
			}
			if shared == nil && !stop {
				shared = stepErr
			}
			if shared == nil && !stop {
				shared = nativewire.CheckEnd(h, tally, *expect)
			}
		}
		if tally.LastOpen {
			rawOpen = [2]string{tally.LastStatus, tally.LastCode}
		}
		wireEnds = wireEnds && codes && shared == nil
		if shared != nil {
			var we *nativewire.Error
			if !errors.As(shared, &we) {
				we = &nativewire.Error{Code: "RESPONSE_MALFORMED"}
			}
			detail = ": " + we.Code
		}
	}
	counted := wire.StepsCompleted != nil && stepsCounted(*wire.StepsCompleted, count, wireEnd, open, rawOpen)
	if x.mandatoryObservation && want != nil { // r4: the response ends as live Check accepts and as recorded
		counted = counted && wireEnds && (r4 || liveSteps(opens, *wire.StepsCompleted, wireEnd)) && wireEnd == end && liveEnd(end, c)
	}
	if want == nil {
		// live Check leaves steps_completed of a response that dropped its steps unconstrained
		counted = wire.StepsCompleted != nil && wire.Code != nil && wireEnd == end && steplessEnd(wireEnd[0], wireEnd[1], wire.Protocol == "tsgk-native/r4" && x.observationQueries)
	}
	if r4 {
		// the members whose presence the strict decode loses, and the capabilities RunCase
		// requests: the workload's queries and API exactly (review on #250)
		p := wire.Producer
		counted = counted && wireEnds && wire.SourceBytes != nil &&
			(!x.observationQueries || p.Query == "tsgk-query/r1") && (!x.observationAPI || p.API != nil && *p.API == "tsgk-api/r1")
	}
	g.check(count == len(want) && wire.Complete != nil && *wire.Complete && counted && wire.Protocol == cmp.Or(x.observationWire, "tsgk-native/r3") && wire.ID == id && responseManifest(wire.Producer.ReuseObservation) == manifest, id, "OBSERVATION_RESPONSE_UNBOUND", "원 응답의 완결성·revision·case·정책·관측 수가 다르다"+detail)
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
// (rawOpen, its last step's first noncomplete tree or query status and code), each the
// response's RESOURCE_LIMIT or FAILED end, as live Check requires.
func stepsCounted(completed uint64, count int, end, open, rawOpen [2]string) bool {
	return completed == uint64(count) || count > 0 && completed == uint64(count-1) &&
		(end[0] == StatusResourceLimit || end[0] == StatusFailed) && open == end && rawOpen == end
}

// treeStatus is the shape of a raw response tree the r3 step checks read; its nodes stay
// judged from the record.
type treeStatus struct {
	Status          string         `json:"status"`
	Code            *string        `json:"code"`
	Form            jsontext.Value `json:"form"`
	DescendantCount uint64         `json:"descendant_count"`
	Digest          string         `json:"digest"`
	Queries         jsontext.Value `json:"queries"`
	API             jsontext.Value `json:"api"`
}

// rawTreeEnd is an r3 raw tree's noncomplete status and code as live Check counts a step
// unfinished; zero when finished. ok reports the tree shape live checkTree accepts: a code
// member, no query or API member, a full, summary or record form only on a COMPLETED tree
// without a code, and a code without a form on a RESOURCE_LIMIT or FAILED tree. An r4 tree
// is judged by the live rules themselves (nativewire, #251).
func rawTreeEnd(t *treeStatus) (end [2]string, ok bool) {
	null := func(v jsontext.Value) bool { return len(v) == 0 || string(bytes.TrimSpace(v)) == "null" }
	if t.Code == nil || t.Queries != nil || t.API != nil {
		return [2]string{}, false
	}
	switch t.Status {
	case StatusCompleted:
		var form string // the decoded form, as live DecodeResponse reads it (Codex A217L)
		return [2]string{}, *t.Code == "" && !null(t.Form) && jsonv2.Unmarshal(t.Form, &form) == nil && slices.Contains([]string{"full", "summary", "record"}, form)
	case StatusResourceLimit, StatusFailed:
		return [2]string{t.Status, *t.Code}, *t.Code != "" && null(t.Form)
	}
	return [2]string{}, false
}
