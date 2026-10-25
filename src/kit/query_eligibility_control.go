package kit

import (
	"bytes"
	"reflect"
	"slices"
	"strconv"
)

type eligibilityControlSummary struct {
	Kind           string                            `json:"kind"`
	Passed         []string                          `json:"passed"`
	BinarySHA256   string                            `json:"binary_sha256"`
	LibrarySHA256  string                            `json:"library_sha256"`
	GrammarInputs  map[string]eligibilitySnapshotRow `json:"grammar_inputs"`
	IdentitySHA256 string                            `json:"identity_sha256"`
	ControlInputs  map[string]string                 `json:"control_inputs"`
}

type eligibilityBatchFooter struct {
	Schema             string   `json:"schema"`
	Status             string   `json:"status"`
	Complete           bool     `json:"complete"`
	Queries            []string `json:"queries"`
	ResponsesCompleted uint64   `json:"responses_completed"`
}

type eligibilityFailureResponse struct {
	Schema   string `json:"schema"`
	Status   string `json:"status"`
	Code     string `json:"code"`
	Complete bool   `json:"complete"`
}

func (s *eligibilitySession) verifyControls() *Error {
	defer func() { s.failItem = nil }()
	group := s.reg.Control
	summary := eligibilityControlSummary{}
	groupReady := false
	paths := map[string]string{}
	cohort := ""
	if group != nil {
		paths = group.Paths
		cohort = group.CohortID
		c, found := s.cohorts[cohort]
		if !found || c.Purpose != "control" {
			return fail(KindInvalidInput, "ELIGIBILITY_CONTROL_COHORT", "control", nil)
		}
		data, ok := s.member(&group.Summary)
		groupReady = ok
		if ok {
			if e := s.b.decode("control-summary", data, &summary, true); e != nil {
				return e
			}
		}
		_, libraryReady := s.member(group.Library)
		if group.Library == nil {
			s.finding("ELIGIBILITY_CONTROL_LIBRARY_MISSING", "control", false)
		}
		groupReady = groupReady && libraryReady && s.groupPaths(paths, c.Platform, "control")
		assertions := make([]string, 0, len(s.catalogue.Assertions))
		for _, a := range s.catalogue.Assertions {
			assertions = append(assertions, a.ID)
		}
		if ok && (summary.Kind != "OWNED_MANUAL_CONTROLS_NOT_ALL_ROUTE_ELIGIBILITY" || !slices.Equal(summary.Passed, assertions) || group.Library != nil && summary.LibrarySHA256 != group.Library.Member.SHA256 || paths["LH"] != summary.LibrarySHA256 || !reflect.DeepEqual(summary.GrammarInputs, s.catalogue.GrammarInputs) || len(summary.ControlInputs) != 5 || summary.ControlInputs["check.py"] != s.catalogue.ControllerSHA256["check.py"] || summary.ControlInputs["checks.py"] != s.catalogue.ControllerSHA256["checks.py"]) {
			s.finding("ELIGIBILITY_CONTROL_SUMMARY", "control", true)
			groupReady = false
		}
		if s.reg.Build == nil || s.buildIdentity == nil {
			groupReady = false
		} else if ok && (summary.BinarySHA256 != s.buildIdentity.BinarySHA256 || summary.IdentitySHA256 != s.reg.Build.Identity.Member.SHA256) {
			s.finding("ELIGIBILITY_CONTROL_BUILD_BINDING", "control", true)
			groupReady = false
		}
		if len(group.Grammar) > len(s.catalogue.GrammarInputs) {
			s.finding("ELIGIBILITY_GRAMMAR_INPUT_SET", "control", true)
			groupReady = false
		}
		for name := range group.Grammar {
			if _, known := s.catalogue.GrammarInputs[name]; !known {
				s.finding("ELIGIBILITY_GRAMMAR_INPUT_UNKNOWN", name, true)
				groupReady = false
			}
		}
		for name, row := range s.catalogue.GrammarInputs {
			pin, found := group.Grammar[name]
			if !found {
				s.finding("ELIGIBILITY_GRAMMAR_INPUT_MISSING", name, false)
				groupReady = false
				continue
			}
			_, verified := s.member(&pin)
			if !found || !verified || pin.Member.Bytes != row.Bytes || pin.Member.SHA256 != row.SHA256 {
				s.finding("ELIGIBILITY_GRAMMAR_INPUT", name, true)
				groupReady = false
			}
		}
		for _, name := range []string{"cc", "supervisor", "python"} {
			matches := 0
			for id, t := range s.tools {
				if t.CohortID == cohort && t.ToolName == name {
					matches++
					if !s.tool(id, cohort, name, group.Summary, summary.ControlInputs, map[string]string{name: t.PathLiteral}) {
						groupReady = false
					}
				}
			}
			if matches != 1 {
				s.finding("ELIGIBILITY_CONTROL_TOOL_COUNT", name, matches > 1)
				groupReady = false
			}
		}
	}
	for _, rule := range s.catalogue.Controls {
		inv := s.catalogueInvocations["control/"+rule.ID]
		if inv == nil {
			s.control[rule.ID] = "blocked"
			continue
		}
		s.failItem = func() { s.control[rule.ID] = "failed" }
		before := s.failures
		req, requestOK, e := s.request(inv)
		if e != nil {
			return e
		}
		ready := requestOK && groupReady && s.ready[inv.ID] && inv.CohortID == cohort
		if requestOK {
			toolPaths := map[string]string{}
			for _, tool := range s.tools {
				if tool.CohortID == cohort {
					toolPaths[tool.ToolName] = tool.PathLiteral
				}
			}
			if group != nil && !s.processEnv(inv, req, paths, toolPaths, false) {
				ready = false
			}
			fixed := eligibilityBuildRule{ID: rule.ID, Tool: rule.Tool, Args: rule.Args, Dir: rule.Dir, WallSeconds: rule.WallSeconds, GraceSeconds: rule.GraceSeconds, MemoryBytes: rule.MemoryBytes, StdoutBytes: rule.StdoutBytes, StderrBytes: rule.StderrBytes}
			if s.fixedRequest(inv, req, fixed, paths) {
				if !s.fixedInputs(inv, req.Args, paths) {
					ready = false
				}
			} else {
				ready = false
			}
			if rule.ID == "compile" {
				if group == nil || !s.tool(inv.ToolID, cohort, "cc", group.Summary, summary.ControlInputs, map[string]string{"cc": req.Path}) {
					ready = false
				}
				if inv.Executable != nil {
					s.finding("ELIGIBILITY_COMPILER_DESCRIPTOR", inv.ID, true)
					ready = false
				}
			} else {
				_, executableReady := s.member(inv.Executable)
				if !executableReady {
					ready = false
				}
				if inv.ToolID != "" || inv.ExecutablePath != req.Path || req.Path != paths["R"] || inv.Executable != nil && inv.Executable.Member.SHA256 != summary.BinarySHA256 {
					s.finding("ELIGIBILITY_REFERENCE_EXECUTABLE", inv.ID, true)
					ready = false
				}
			}
			out, executionReady, e := s.receipt(inv, req, rule.ExpectedSupervisorExit, rule.ID == "batch-truncated")
			if e != nil {
				return e
			}
			ready = ready && executionReady
			if out != nil {
				s.outputs[inv.ID] = out
				if rule.ID != "compile" && rule.ID != "batch-truncated" {
					semanticReady, e := s.controlResponses(rule, out, paths)
					if e != nil {
						return e
					}
					ready = ready && semanticReady
				}
			}
		}
		state := "blocked"
		if s.failures > before {
			state = "failed"
		} else if ready {
			state = "verified"
		}
		s.control[rule.ID] = state
		if e := s.r.check(); e != nil {
			return e
		}
	}
	// Full parsed response parity retains opaque runtime IDs and all producer fields.
	for _, pair := range []struct {
		batch string
		count int
	}{{"batch-sixteen", 16}, {"batch-one", 1}} {
		b := s.catalogueInvocations["control/"+pair.batch]
		if b == nil {
			continue
		}
		out := s.outputs[b.ID]
		if out == nil {
			continue
		}
		for i := 0; i < pair.count; i++ {
			var single *eligibilityInvocation
			for _, inv := range s.reg.Invocations {
				if inv.CatalogueID == eligibilityBatchSingleID(i) && s.cohorts[inv.CohortID].Purpose == "control" {
					single = s.invocations[inv.ID]
				}
			}
			if single == nil || s.outputs[single.ID] == nil || len(s.outputs[single.ID].responses) != 1 || i >= len(out.responses) {
				if s.control[pair.batch] != "failed" {
					s.control[pair.batch] = "blocked"
				}
				continue
			}
			actual := s.outputs[single.ID].responses[0]
			if e := s.b.charge(1000, uint64(len(out.rows[i])+len(s.outputs[single.ID].stdout)), pair.batch+"/parity"); e != nil {
				return e
			}
			if !reflect.DeepEqual(out.responses[i], actual) {
				s.finding("ELIGIBILITY_BATCH_FULL_PARITY", pair.batch, true)
				s.control[pair.batch] = "failed"
			}
		}
	}
	for _, id := range []string{"batch-later-input-missing", "batch-later-input-oversized", "batch-later-input-utf8"} {
		inv, single := s.catalogueInvocations["control/"+id], s.catalogueInvocations["control/batch-single-0"]
		if inv == nil || single == nil {
			continue
		}
		out, one := s.outputs[inv.ID], s.outputs[single.ID]
		if out == nil || one == nil || len(out.responses) != 1 || len(one.responses) != 1 || out.responses[0] == nil || one.responses[0] == nil {
			continue
		}
		actual := *out.responses[0]
		actual.Query.ID = "b0"
		if !reflect.DeepEqual(&actual, one.responses[0]) {
			s.finding("ELIGIBILITY_NEGATIVE_PREFIX_PARITY", id, true)
			s.control[id] = "failed"
		}
	}
	return nil
}

func eligibilityBatchSingleID(i int) string { return "batch-single-" + strconv.Itoa(i) }

func (s *eligibilitySession) controlResponses(rule eligibilityControlRule, out *eligibilityOutput, paths map[string]string) (bool, *Error) {
	batch := len(rule.Args) > 0 && rule.Args[0] == "--batch"
	expectedRows := len(rule.Responses)
	if rule.Negative {
		expectedRows++
	}
	if batch {
		expectedRows++
	}
	if batch {
		if len(out.stdout) == 0 || out.stdout[len(out.stdout)-1] != '\n' {
			s.finding("ELIGIBILITY_BATCH_EOF", rule.ID, true)
			return false, nil
		}
		out.rows = bytes.Split(out.stdout[:len(out.stdout)-1], []byte{'\n'})
	} else {
		out.rows = [][]byte{out.stdout}
	}
	if len(out.rows) != expectedRows {
		s.finding("ELIGIBILITY_RESPONSE_COUNT", rule.ID, true)
		return false, nil
	}
	offset := uint64(0)
	for _, row := range out.rows {
		out.offsets = append(out.offsets, offset)
		offset += uint64(len(row) + 1)
	}
	ready := true
	for i, id := range rule.Responses {
		expected := s.catalogue.Responses[id]
		ref, ok, e := s.manualResponse(out.rows[i], expected, paths["LH"])
		if e != nil {
			return false, e
		}
		ready = ready && ok
		out.responses = append(out.responses, ref)
	}
	if rule.Negative {
		var failed eligibilityFailureResponse
		if e := s.b.decode(rule.ID+"/failure", out.rows[len(rule.Responses)], &failed, true); e != nil {
			return false, e
		}
		if failed.Schema != instrumentedReferenceSchema || failed.Status != StatusFailed || failed.Code != rule.FailureCode || failed.Complete {
			s.finding("ELIGIBILITY_EXPECTED_FAILURE", rule.ID, true)
			ready = false
		}
	}
	if batch {
		var footer eligibilityBatchFooter
		if e := s.b.decode(rule.ID+"/footer", out.rows[len(out.rows)-1], &footer, true); e != nil {
			return false, e
		}
		status := StatusCompleted
		if rule.Negative {
			status = StatusFailed
		}
		if footer.Schema != "tsgk-instrumented-query-reference-batch/r1" || footer.Status != status || footer.Complete == rule.Negative || !slices.Equal(footer.Queries, rule.QueryIDs) || footer.ResponsesCompleted != uint64(len(rule.Responses)) {
			s.finding("ELIGIBILITY_BATCH_FOOTER", rule.ID, true)
			ready = false
		}
	}
	return ready, nil
}

func (s *eligibilitySession) manualResponse(data []byte, want eligibilityExpected, library string) (*instrumentedReference, bool, *Error) {
	source := s.catalogue.Inputs[want.Source]
	query := s.catalogue.Inputs[want.Query]
	sourceBytes, e := s.fixedBytes(source)
	if e != nil {
		return nil, false, e
	}
	queryBytes, e := s.fixedBytes(query)
	if e != nil {
		return nil, false, e
	}
	q := QueryWitnessQuery{ID: want.ID, Source: string(queryBytes), Metadata: &QueryWitnessMetadata{Patterns: want.Patterns, CaptureNames: want.CaptureNames, Predicates: want.Predicates}}
	obs := QueryReferenceObservation{Query: want.ID, Step: 0, SourceBytes: source.Bytes, SourceSHA256: source.SHA256}
	ref, code, complete, e := decodeReferenceResponseBound(data, obs, library, "tree_sitter_tsgk_plain", q, true, [][]byte{sourceBytes}, s.r)
	if e != nil {
		return nil, false, e
	}
	if code != "" || !complete || ref == nil {
		s.finding("ELIGIBILITY_MANUAL_RESPONSE", want.ID+"/"+code, true)
		return ref, false, nil
	}
	structural, evaluated := make([]Capture, 0, len(want.Structural)), make([]Capture, 0, len(want.Evaluated))
	for _, id := range want.Structural {
		structural = append(structural, s.catalogue.Captures[id])
	}
	for _, id := range want.Evaluated {
		evaluated = append(evaluated, s.catalogue.Captures[id])
	}
	if e := s.b.charge(uint64(len(ref.Tree.Nodes)+len(structural)+len(evaluated)+1)*128, 4*uint64(len(data)), want.ID+"/manual-compare"); e != nil {
		return nil, false, e
	}
	if CompareTrees(ref.Tree.Nodes, s.catalogue.Trees[want.Tree]) != nil || ref.Query.Patterns != want.Patterns || !slices.Equal(ref.Query.CaptureNames, want.CaptureNames) || !reflect.DeepEqual(ref.Query.Predicates, want.Predicates) || CompareCaptures(ref.Query.Structural, structural) != nil || CompareCaptures(ref.Query.Evaluated, evaluated) != nil {
		s.finding("ELIGIBILITY_MANUAL_EXPECTATION", want.ID, true)
		return ref, false, nil
	}
	return ref, true, nil
}
