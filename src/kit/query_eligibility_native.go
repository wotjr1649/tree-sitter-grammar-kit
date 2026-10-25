package kit

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"io/fs"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

func (s *eligibilitySession) verifyNative() *Error {
	defer func() { s.failItem = nil }()
	w := s.witness
	n := s.reg.Native
	var versions [][]byte
	var points []EditPoints
	nativeReady := false
	var inv *eligibilityInvocation
	if n != nil {
		inv = s.invocations[n.InvocationID]
		if inv == nil || inv.Kind != "NATIVE_DRIVER" || s.cohorts[inv.CohortID].Purpose != "current" {
			return fail(KindInvalidInput, "ELIGIBILITY_NATIVE_INVOCATION", "native", nil)
		}
		s.failItem = func() { s.failInvocation(inv) }
		nativeReady = s.ready[inv.ID] && eligibilityAllVerified(s.build, 9) && eligibilityAllVerified(s.units, 4) && eligibilityAllVerified(s.control, 53) && eligibilityAllVerified(s.assertions, 36)
		_, executableReady, e := s.member(inv.Executable)
		if e != nil {
			return e
		}
		nativeReady = nativeReady && executableReady
		if inv.Executable == nil || inv.Executable.Member.SHA256 != w.Native.ExecutableSHA256 || !hex64.MatchString(w.Native.ExecutableSHA256) {
			s.finding("ELIGIBILITY_NATIVE_EXECUTABLE", inv.ID, true)
			nativeReady = false
		}

		source, haveSource, e := s.member(&n.Source)

		if e != nil {
			return e
		}
		if haveSource {
			if n.Source.Member.Bytes != w.Input.Bytes || n.Source.Member.SHA256 != w.Input.SHA256 || !utf8.Valid(source) {
				s.finding("ELIGIBILITY_NATIVE_SOURCE", "native", true)
				nativeReady = false
			} else {
				total := w.Input.Bytes
				for _, edit := range w.Edits {
					total += uint64(len(edit.New))
					if total > MaxDocumentBytes {
						return fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", "native/source", nil)
					}
				}
				logical, e := eligibilityProduct("source-replay", total+1, uint64(len(w.Edits)+1), 8)
				if e != nil {
					return e
				}
				if e := s.b.charge(uint64(len(w.Edits)+1), logical, "source-replay"); e != nil {
					return e
				}
				var err error
				versions, points, err = ApplyEdits(EncodingUTF8, source, w.Edits, MaxDocumentBytes)
				if err != nil {
					s.finding("ELIGIBILITY_NATIVE_EDITS", "native", true)
					nativeReady = false
				}
			}
		} else {
			nativeReady = false
		}
		if built, e := s.nativeBuild(n, w.Native); e != nil {
			return e
		} else if !built {
			nativeReady = false
		}
		if e := s.r.check(); e != nil {
			return e
		}
		if ok, e := s.nativeInputs(inv, n, w); e != nil {
			return e
		} else if !ok {
			nativeReady = false
		}
	}
	if e := s.currentReferences(versions); e != nil {
		return e
	}
	var checked []nativewire.CheckedStep
	if inv != nil {
		s.failItem = func() { s.failInvocation(inv) }
		requestBytes, requestReady, e := s.member(inv.Request)
		if e != nil {
			return e
		}
		var approved eligibilityNativeExpected
		if e := s.b.decode(inv.ID+"/approved-request", inv.ApprovedRequest, &approved, true); e != nil {
			return e
		}
		if requestReady {
			var stored eligibilityNativeExpected
			if e := s.b.decode(inv.ID+"/request", requestBytes, &stored, true); e != nil {
				return e
			}
			if !reflect.DeepEqual(stored, approved) || !reflect.DeepEqual(approved, w.Native) {
				s.finding("ELIGIBILITY_NATIVE_REQUEST", inv.ID, true)
				nativeReady = false
			}
		} else {
			nativeReady = false
		}
		if e := eligibilityValidateRequest(w.Native.Process, inv.ID); e != nil {
			return e
		}
		if w.Native.Process.Path != inv.ExecutablePath {
			s.finding("ELIGIBILITY_NATIVE_EXECUTABLE_PATH", inv.ID, true)
			nativeReady = false
		}
		if w.Native.Protocol != nativewire.ProtocolR4 || !validID(w.Native.RequestID) || w.Native.RequestID != w.Case || w.Native.NodeLimit == 0 || w.Native.NodeLimit > 1000000 || !hex64.MatchString(w.Native.ProducerSHA256) || !reuseobs.VerifiedRuntimeManifest(w.Native.RuntimeManifestSHA256) || !validID(w.Native.ObservationRunID) || w.Native.HistoryStart > 4 || w.Native.HistoryStart+uint64(len(w.Edits)) > 4 {
			s.finding("ELIGIBILITY_R4_EXPECTATION", inv.ID, true)
			nativeReady = false
		}
		processBytes, haveProcess, e := s.member(inv.Process)
		if e != nil {
			return e
		}
		var process eligibilityExecution
		if haveProcess {
			if e := s.b.decode(inv.ID+"/process", processBytes, &process, true); e != nil {
				return e
			}
			if !s.execution(inv.ID, process, w.Native.Process, w.Platform, 0, false) {
				nativeReady = false
			}
		} else {
			nativeReady = false
		}
		data, haveRaw, e := s.member(inv.Response)
		if e != nil {
			return e
		}
		if haveRaw && len(versions) > 0 {
			if e := s.b.reserveJSON(inv.ID+"/native-response", data, 12); e != nil {
				return e
			}
			var raw nativewire.Response
			if err := jsonv2.Unmarshal(data, &raw, jsonv2.RejectUnknownMembers(true)); err != nil {
				return fail(KindInvalidInput, "JSON_SHAPE", inv.ID, err)
			}

			var capability struct {
				Policy                string `json:"policy"`
				RuntimeManifestSHA256 string `json:"runtime_manifest_sha256"`
			}
			if e := s.b.decode("native-observation-capability", raw.Producer.ReuseObservation, &capability, true); e != nil {
				return e
			}
			if e := s.nativeReservation(raw, versions); e != nil {
				return e
			}
			abi, p := w.Native.ProducerABI, raw.Producer
			// The approved current runtime manifest pins ABI compatibility to 13..15.
			if w.Native.RuntimeManifestSHA256 == reuseobs.RuntimeManifestSHA256 && (abi.RuntimeLanguageVersion != 15 || abi.RuntimeMinCompatible != 13) {
				s.finding("ELIGIBILITY_NATIVE_RUNTIME_ABI", inv.ID, true)
				nativeReady = false
			}
			if abi.LanguageVersion == 0 || abi.RuntimeMinCompatible == 0 || abi.RuntimeMinCompatible > abi.LanguageVersion || abi.LanguageVersion > abi.RuntimeLanguageVersion || abi.SymbolCount == 0 || p.LanguageVersion != abi.LanguageVersion || p.RuntimeLanguageVersion != abi.RuntimeLanguageVersion || p.RuntimeMinCompatible != abi.RuntimeMinCompatible || p.SymbolCount == nil || *p.SymbolCount != abi.SymbolCount || p.FieldCount == nil || *p.FieldCount != abi.FieldCount || p.Query != "tsgk-query/r1" || p.API == nil || *p.API != "tsgk-api/r1" {
				s.finding("ELIGIBILITY_NATIVE_PRODUCER_ABI", inv.ID, true)
				nativeReady = false
			}
			if haveProcess && (process.StdoutBytes != uint64(len(data))+4 || process.StdoutBytes > w.Native.Process.StdoutBytes || process.StderrBytes > w.Native.Process.StderrBytes) {
				s.finding("ELIGIBILITY_NATIVE_PROCESS_STREAM", inv.ID, true)
				nativeReady = false
			}
			expect := nativewire.Expect{Check: func() error {
				if e := s.r.check(); e != nil {
					return e
				}
				return nil
			}, Revision: nativewire.ProtocolR4, ID: w.Native.RequestID, Nodes: w.Native.NodeLimit, API: w.Native.API, Points: w.Native.Points, SourceBytes: w.Input.Bytes, Exit: &process.ExitCode, Observation: &nativewire.Binding{Manifest: w.Native.RuntimeManifestSHA256, RunID: w.Native.ObservationRunID, ProducerSHA256: w.Native.ProducerSHA256, HistoryStart: w.Native.HistoryStart}}
			for _, q := range w.Queries {
				expect.Queries = append(expect.Queries, querywire.Source{ID: q.ID, Source: []byte(q.Source)})
			}
			for _, edit := range w.Edits {
				expect.Edits = append(expect.Edits, nativewire.EditSpan{StartByte: edit.StartByte, OldEndByte: edit.OldEndByte, NewEndByte: edit.NewEndByte})
			}
			expect.EditPoints = points
			for _, version := range versions {
				expect.Sources = append(expect.Sources, nativewire.SourceID{Bytes: uint64(len(version)), SHA256: digestHex(version)})
			}
			stop, err := nativewire.CheckHeader(raw, len(raw.Steps), expect)
			if err != nil || stop {
				s.finding("ELIGIBILITY_R4_HEADER", inv.ID, true)
				nativeReady = false
			} else {
				tally := nativewire.Tally{}
				reuseSteps := []reuseobs.Step{}
				all := []reuseobs.Observation{}
				for i, step := range raw.Steps {
					cs, err := nativewire.CheckStep(i, step, expect)
					if e, ok := err.(*Error); ok {
						return e
					}
					if err != nil {
						s.finding("ELIGIBILITY_R4_STEP", fmt.Sprintf("%s/%d", inv.ID, i), true)
						nativeReady = false
						break
					}
					done := cs.Done(i)
					if err := nativewire.CheckOpen(i, len(raw.Steps), raw.Status, done); err != nil {
						s.finding("ELIGIBILITY_R4_OPEN", inv.ID, true)
						nativeReady = false
					}
					tally.Add(i, cs)
					checked = append(checked, cs)
					reuseSteps = append(reuseSteps, reuseobs.Step{Native: true, Edit: i > 0, Observations: cs.Observations})
					all = append(all, cs.Observations...)
				}
				if err := nativewire.CheckEnd(raw, tally, expect); err != nil {
					s.finding("ELIGIBILITY_R4_END", inv.ID, true)
					nativeReady = false
				}
				mandatory, full := reuseobs.CaseClaim(reuseSteps, raw.Status == StatusCompleted), reuseobs.Judge(all)
				if mandatory == "FAIL" || full == "FAIL" {
					s.finding("ELIGIBILITY_LOGGER_CONTRADICTION", inv.ID, true)
					nativeReady = false
				} else if mandatory != "PASS" || full != "PASS" {
					s.finding("ELIGIBILITY_LOGGER_INCOMPLETE", inv.ID, false)
					nativeReady = false
				}
				if raw.Status != StatusCompleted || !raw.Complete || len(checked) != len(versions) {
					s.finding("ELIGIBILITY_NATIVE_PARTIAL", inv.ID, false)
					nativeReady = false
				}
			}
		} else {
			nativeReady = false
		}
	}
	for i, tuple := range w.Required {
		s.failItem = func() { s.tuples[i] = "failed" }
		before := s.failures
		ready := nativeReady
		queryIndex := slices.IndexFunc(w.Queries, func(q QueryWitnessQuery) bool { return q.ID == tuple.Query })
		q := w.Queries[queryIndex]
		if q.Metadata == nil || tuple.Structural == nil || tuple.Expected == nil {
			s.finding("ELIGIBILITY_INDEPENDENT_ANCHOR_MISSING", tuple.Query, false)
			ready = false
		}
		var tree *nativewire.Tree
		if int(tuple.Step) < len(checked) {
			tree = &checked[tuple.Step].Incremental
			if tuple.TreeRole == "fresh" {
				tree = checked[tuple.Step].Fresh
			}
		}
		var nativeQuery *querywire.Query
		var evaluated []Capture
		if tree != nil && queryIndex < len(tree.Queries) {
			nativeQuery = &tree.Queries[queryIndex]
			if q.Metadata != nil && (nativeQuery.Patterns != q.Metadata.Patterns || !slices.Equal(nativeQuery.CaptureNames, q.Metadata.CaptureNames) || !reflect.DeepEqual(nativeQuery.Predicates, q.Metadata.Predicates)) {
				s.finding("ELIGIBILITY_NATIVE_METADATA", tuple.Query, true)
				ready = false
			}
			if int(tuple.Step) < len(versions) {
				reservation := instrumentedReference{}
				reservation.Tree.Nodes = tree.Nodes
				reservation.Query.Predicates = nativeQuery.Predicates
				reservation.Query.CaptureNames = nativeQuery.CaptureNames
				for _, row := range nativeQuery.Captures {
					reservation.Query.Structural = append(reservation.Query.Structural, witnessCapture(*nativeQuery, row))
				}
				reservation.Query.Evaluated = tuple.Expected
				if e := s.b.reserveReference(&reservation, [][]byte{versions[tuple.Step]}); e != nil {
					return e
				}
				value, err := witnessEvaluate(s.r, *nativeQuery, EncodingUTF8, versions[tuple.Step])
				if err != nil {
					return asError(err)
				}
				evaluated = value.captures
				structural := make([]Capture, 0, len(nativeQuery.Captures))
				for _, row := range nativeQuery.Captures {
					structural = append(structural, witnessCapture(*nativeQuery, row))
				}
				if tuple.Structural != nil && CompareCaptures(structural, tuple.Structural) != nil || tuple.Expected != nil && CompareCaptures(evaluated, tuple.Expected) != nil {
					s.finding("ELIGIBILITY_NATIVE_ANCHOR", tuple.Query, true)
					ready = false
				}
				if value.status != StatusCompleted || nativeQuery.Partial || tree.Form != "full" || !witnessCST(tree.Nodes) || witnessPoints(tree.Nodes, EncodingUTF8, versions[tuple.Step], s.r) != "" {
					s.finding("ELIGIBILITY_NATIVE_CST_INELIGIBLE", tuple.Query, false)
					ready = false
				}
			}
		} else {
			ready = false
		}
		observation, found := s.observation(tuple)
		if !found {
			s.finding("ELIGIBILITY_REFERENCE_OBSERVATION_MISSING", tuple.Query, false)
			ready = false
		} else {
			refInv := s.invocations[observation.InvocationID]
			out := s.outputs[observation.InvocationID]
			if refInv == nil || s.cohorts[refInv.CohortID].Purpose != "current" || refInv.Kind != "REFERENCE_PROCESS" {
				s.finding("ELIGIBILITY_OBSERVATION_INVOCATION", tuple.Query, true)
				ready = false
			} else if out == nil || observation.ResponseOrdinal >= uint64(len(out.responses)) || out.responses[observation.ResponseOrdinal] == nil {
				ready = false
			} else {
				k := observation.ResponseOrdinal
				ref := out.responses[k]
				if observation.ByteOffset != out.offsets[k] || observation.ByteLength != uint64(len(out.rows[k])) || observation.SHA256 != digestHex(out.rows[k]) || observation.QueryID != ref.Query.ID || observation.SourceSHA256 != ref.Input.SHA256 || observation.SourceBytes != ref.Input.Bytes {
					s.finding("ELIGIBILITY_OBSERVATION_SLICE", tuple.Query, true)
					ready = false
				}
				if int(tuple.Step) >= len(versions) {
					ready = false
				} else if observation.SourceSHA256 != digestHex(versions[tuple.Step]) || observation.SourceBytes != uint64(len(versions[tuple.Step])) {
					s.finding("ELIGIBILITY_OBSERVATION_SOURCE_VERSION", tuple.Query, true)
					ready = false
				}
				if q.Metadata != nil && (ref.Query.Patterns != q.Metadata.Patterns || !slices.Equal(ref.Query.CaptureNames, q.Metadata.CaptureNames) || !reflect.DeepEqual(ref.Query.Predicates, q.Metadata.Predicates)) {
					s.finding("ELIGIBILITY_REFERENCE_METADATA", tuple.Query, true)
					ready = false
				}
				if tuple.Structural != nil && CompareCaptures(ref.Query.Structural, tuple.Structural) != nil || tuple.Expected != nil && CompareCaptures(ref.Query.Evaluated, tuple.Expected) != nil {
					s.finding("ELIGIBILITY_REFERENCE_ANCHOR", tuple.Query, true)
					ready = false
				}
				if tree != nil {
					if CompareTrees(tree.Nodes, ref.Tree.Nodes) != nil {
						s.finding("ELIGIBILITY_CST_MISMATCH", tuple.Query, false)
						ready = false
					}
					if nativeQuery != nil && CompareCaptures(evaluated, ref.Query.Evaluated) != nil {
						s.finding("ELIGIBILITY_EVALUATED_MISMATCH", tuple.Query, true)
						ready = false
					}
				}
				ready = ready && s.ready[refInv.ID]
			}
		}
		state := "blocked"
		if s.failures > before || s.tuples[i] == "failed" {
			state = "failed"
		} else if ready {
			state = "verified"
		}
		s.tuples[i] = state
		if e := s.r.check(); e != nil {
			return e
		}
	}
	return nil
}

func (s *eligibilitySession) observation(tuple QueryWitnessTuple) (eligibilityObservation, bool) {
	for _, o := range s.reg.Observations {
		if o.Query == tuple.Query && o.Step == tuple.Step && o.TreeRole == tuple.TreeRole {
			return o, true
		}
	}
	return eligibilityObservation{}, false
}

func (s *eligibilitySession) nativeInputs(inv *eligibilityInvocation, n *eligibilityNative, w eligibilityWitness) (bool, *Error) {
	good := true
	allowed := map[string]bool{"source": true, "library": true}
	source, ok := inv.Inputs["source"]
	if !ok {
		s.finding("ELIGIBILITY_INPUT_MISSING", inv.ID+"/source", false)
		good = false
	} else if source.Kind != "PINNED" || source.MemberPin == nil || *source.MemberPin != n.Source || source.ArgvIndex != nil || source.PathLiteral != "" {
		s.finding("ELIGIBILITY_NATIVE_SOURCE_INPUT", inv.ID, true)
		good = false
	}
	if ok, e := s.input(inv, "library", "NOT_APPLICABLE", nil, "", "", nil); e != nil {
		return false, e
	} else if !ok {
		good = false
	}
	for _, q := range w.Queries {
		name := "query-" + q.ID
		allowed[name] = true
		input, ok := inv.Inputs[name]
		if !ok {
			s.finding("ELIGIBILITY_INPUT_MISSING", name, false)
			good = false
			continue
		}
		if input.Kind != "PINNED" || input.MemberPin == nil || input.ArgvIndex != nil || input.PathLiteral != "" || input.MemberPin.Member.SHA256 != digestHex([]byte(q.Source)) || input.MemberPin.Member.Bytes != uint64(len(q.Source)) {
			s.finding("ELIGIBILITY_NATIVE_QUERY_INPUT", name, true)
			good = false
		}
	}
	return s.inputSlots(inv, allowed) && good, nil
}

func (s *eligibilitySession) nativeBuild(n *eligibilityNative, w eligibilityNativeExpected) (bool, *Error) {
	data, ok, e := s.member(&n.BuildManifest)
	if e != nil {
		return false, e
	}
	if !ok {
		return false, nil
	}
	if e := s.b.charge(8*(uint64(len(data))+1)+uint64(len(n.Grammar)), 40*uint64(len(data)), "native-build-manifest"); e != nil {
		return false, nil
	}
	if digestHex(data) != w.ProducerSHA256 || !utf8.Valid(data) || len(data) == 0 || data[len(data)-1] != '\n' {
		s.finding("ELIGIBILITY_NATIVE_BUILD_MANIFEST", "native", true)
		return false, nil
	}
	expectedFiles, runtimeCommit, e := s.nativeStaticClosure(n.Symbol)
	if e != nil {
		return false, e
	}
	if w.RuntimeManifestSHA256 != reuseobs.RuntimeManifestSHA256 {
		s.finding("ELIGIBILITY_NATIVE_HISTORICAL_RUNTIME", "native", false)
		return false, nil
	}
	lines := strings.Split(string(data[:len(data)-1]), "\n")
	if len(lines) < 7 || lines[0] != "tsgk-native-build/r1" || lines[1] != "symbol="+n.Symbol || lines[4] != "os="+s.witness.Platform || !strings.HasPrefix(lines[2], "compiler=") || !strings.HasPrefix(lines[3], "runtime=") {
		s.finding("ELIGIBILITY_NATIVE_BUILD_HEADER", "native", true)
		return false, nil
	}
	compiler := strings.Fields(strings.TrimPrefix(lines[2], "compiler="))
	if len(compiler) != 2 || !hex64.MatchString(compiler[0]) {
		s.finding("ELIGIBILITY_NATIVE_BUILD_COMPILER", "native", true)
		return false, nil
	}
	if _, err := strconv.ParseUint(compiler[1], 10, 53); err != nil || strings.TrimPrefix(lines[3], "runtime=") != runtimeCommit {
		s.finding("ELIGIBILITY_NATIVE_BUILD_HEADER", "native", true)
		return false, nil
	}
	good := true
	seen := map[string]bool{}
	grammar := map[string]bool{}
	steps := []string{}
	parsers, scanners := 0, 0
	previousFile := ""
	for _, line := range lines[5:] {
		if e := s.b.charge(1, uint64(len(line)), "native-build-manifest"); e != nil {
			return false, e
		}
		if strings.HasPrefix(line, "step=") {
			name, args, valid := eligibilityBuildStep(strings.TrimPrefix(line, "step="))
			if !valid || name == "version" && !slices.Equal(args, []string{"--version"}) {
				s.finding("ELIGIBILITY_NATIVE_BUILD_STEP", "native", true)
				good = false
			}
			steps = append(steps, name)
			continue
		}
		if !strings.HasPrefix(line, "file=") {
			s.finding("ELIGIBILITY_NATIVE_BUILD_LINE", "native", true)
			good = false
			continue
		}
		body := strings.TrimPrefix(line, "file=")
		role, rest, haveRole := strings.Cut(body, " ")
		last := strings.LastIndexByte(rest, ' ')
		fields := []string{}
		if haveRole && last >= 0 {
			sizeText := rest[last+1:]
			rest = rest[:last]
			hashAt := strings.LastIndexByte(rest, ' ')
			if hashAt >= 0 {
				fields = []string{role, rest[:hashAt], rest[hashAt+1:], sizeText}
			}
		}
		if len(fields) != 4 || !validID(fields[0]) || !portable(fields[1], false) || !hex64.MatchString(fields[2]) {
			s.finding("ELIGIBILITY_NATIVE_BUILD_FILE", "native", true)
			good = false
			continue
		}
		size, err := strconv.ParseUint(fields[3], 10, 53)
		key := fields[0] + " " + fields[1]
		if len(steps) != 0 || previousFile != "" && key <= previousFile {
			s.finding("ELIGIBILITY_NATIVE_BUILD_ORDER", "native", true)
			good = false
		}
		previousFile = key
		if strings.HasPrefix(fields[0], "grammar-") {
			switch fields[0] {
			case "grammar-parser":
				parsers++
			case "grammar-scanner":
				scanners++
			case "grammar-header":
			default:
				s.finding("ELIGIBILITY_NATIVE_GRAMMAR_ROLE", fields[1], true)
				good = false
			}
		}
		if err != nil || seen[key] {
			s.finding("ELIGIBILITY_NATIVE_BUILD_FILE", "native", true)
			good = false
		}
		if row, known := expectedFiles[key]; known {
			if row.SHA256 != fields[2] || row.Bytes != size {
				s.finding("ELIGIBILITY_NATIVE_RUNTIME_DRIVER", fields[1], true)
				good = false
			}
		} else if !strings.HasPrefix(fields[0], "grammar-") {
			s.finding("ELIGIBILITY_NATIVE_BUILD_FILE_UNKNOWN", fields[1], true)
			good = false
		}
		seen[key] = true
		if strings.HasPrefix(fields[0], "grammar-") {
			pin, found := n.Grammar[fields[1]]
			if !found {
				s.finding("ELIGIBILITY_NATIVE_GRAMMAR_MISSING", fields[1], false)
				good = false
				grammar[fields[1]] = true
				continue
			}
			_, verified, e := s.member(&pin)
			if e != nil {
				return false, e
			}
			if !found || !verified || pin.Member.Role != fields[0] || pin.Member.SHA256 != fields[2] || pin.Member.Bytes != size {
				s.finding("ELIGIBILITY_NATIVE_GRAMMAR", fields[1], true)
				good = false
			}
			grammar[fields[1]] = true
		}
	}
	for key := range expectedFiles {
		if !seen[key] {
			s.finding("ELIGIBILITY_NATIVE_BUILD_FILE_MISSING", key, true)
			good = false
		}
	}
	expectedSteps := []string{"version", "runtime", "parser"}
	for i := 0; i < scanners; i++ {
		expectedSteps = append(expectedSteps, "scanner")
	}
	expectedSteps = append(expectedSteps, "driver", "shim", "link")
	if parsers != 1 || len(grammar) != len(n.Grammar) || !slices.Equal(steps, expectedSteps) {
		s.finding("ELIGIBILITY_NATIVE_BUILD_CLOSURE", "native", true)
		good = false
	}
	return good, nil
}

func (s *eligibilitySession) nativeReservation(raw nativewire.Response, versions [][]byte) *Error {
	work := uint64(0)
	logical := uint64(0)
	for i, step := range raw.Steps {
		if e := s.b.charge(1, 0, "native-bound"); e != nil {
			return e
		}
		if raw.Protocol == nativewire.ProtocolR4 || len(step.Observations) > 0 {
			if e := s.b.reserveJSON("native-observations", step.Observations, 4); e != nil {
				return e
			}
			var observations []jsontext.Value
			if e := s.b.decode("native-observations", step.Observations, &observations, true); e != nil {
				return e
			}
			for _, record := range observations {
				if _, err := reuseobs.Parse(record); err != nil {
					return fail(KindInvalidInput, "JSON_SHAPE", "native-observation", err)
				}
			}
		}
		for _, tree := range []*nativewire.WireTree{&step.Incremental, step.Fresh} {
			if tree == nil {
				continue
			}
			work += 256 * uint64(len(tree.Nodes)+len(tree.PartialTrees)+1)
			for _, name := range tree.Types {
				logical += 4 * uint64(len(name))
			}
			for _, name := range tree.Fields {
				logical += 4 * uint64(len(name))
			}
			reserveRows := func(rows [][]int64) *Error {
				for _, row := range rows {
					repeated := uint64(256)
					if len(row) == 10 {
						if row[1] >= 0 && row[1] < int64(len(tree.Types)) {
							repeated += 4 * uint64(len(tree.Types[row[1]]))
						}
						if row[2] >= 0 && row[2] < int64(len(tree.Fields)) {
							repeated += 4 * uint64(len(tree.Fields[row[2]]))
						}
					}
					if e := s.b.charge(256, repeated, "native-node-strings"); e != nil {
						return e
					}
				}
				return nil
			}
			if e := reserveRows(tree.Nodes); e != nil {
				return e
			}
			for _, partial := range tree.PartialTrees {
				if e := s.b.charge(1, 0, "native-partial"); e != nil {
					return e
				}
				if e := reserveRows(partial.Nodes); e != nil {
					return e
				}
			}
			if e := s.b.reserveJSON("native-query-bound", tree.Queries, 18); e != nil {
				return e
			}
			queries, err := querywire.ParseQueries(tree.Queries)
			if err != nil {
				return fail(KindInvalidInput, "JSON_SHAPE", "native-queries", err)
			}
			// Compact captures may repeatedly reference one long type string.
			for _, q := range queries {
				for _, row := range q.Captures {
					repeated := uint64(256)
					if len(row) == 12 {
						if row[4] >= 0 && row[4] < int64(len(q.Types)) {
							repeated += 4 * uint64(len(q.Types[row[4]]))
						}
						if row[3] >= 0 && row[3] < int64(len(tree.Nodes)) {
							node := tree.Nodes[row[3]]
							if len(node) == 10 && node[1] >= 0 && node[1] < int64(len(tree.Types)) {
								repeated += 4 * uint64(len(tree.Types[node[1]]))
							}
						}
					}
					if e := s.b.charge(256, repeated, "native-capture-strings"); e != nil {
						return e
					}
				}
			}
			if len(tree.API) > 0 && !bytes.Equal(bytes.TrimSpace(tree.API), []byte("null")) {
				var api querywire.API
				if e := s.b.decode("native-api", tree.API, &api, true); e != nil {
					return e
				}
			}
			// Reserve strict preflight, validation and query traversal before CheckStep.
			work += 4 * uint64(len(tree.Queries))
			logical += 24 * uint64(len(tree.Queries))
		}
		if i < len(versions) {
			logical += 16 * uint64(len(versions[i]))
		}
	}
	return s.b.charge(work, logical, "native-bound")
}

func (s *eligibilitySession) currentReferences(versions [][]byte) *Error {
	defer func() { s.failItem = nil }()
	sourceIDs := make([]nativewire.SourceID, len(versions))
	for i, version := range versions {
		if e := s.b.charge(1, uint64(len(version)), "reference-source-identity"); e != nil {
			return e
		}
		sourceIDs[i] = nativewire.SourceID{Bytes: uint64(len(version)), SHA256: digestHex(version)}
	}
	for _, inv := range s.invocations {
		s.failItem = nil
		if inv.Kind != "REFERENCE_PROCESS" || s.cohorts[inv.CohortID].Purpose != "current" {
			continue
		}
		s.failItem = func() { s.failInvocation(inv) }
		before := s.failures
		ready := s.ready[inv.ID]
		req, ok, e := s.request(inv)
		if e != nil {
			return e
		}
		if !ok {
			s.ready[inv.ID] = false
			continue
		}
		if inv.ToolID != "" || inv.Executable != nil && req.Path != inv.ExecutablePath {
			s.finding("ELIGIBILITY_CURRENT_EXECUTABLE", inv.ID, true)
			ready = false
		}
		_, exeOK, e := s.member(inv.Executable)
		if e != nil {
			return e
		}
		ready = ready && exeOK
		if s.reg.Build == nil || s.reg.Build.ReferenceBinary == nil {
			ready = false
		} else if inv.Executable != nil && inv.Executable.Member.SHA256 != s.reg.Build.ReferenceBinary.Member.SHA256 {
			s.finding("ELIGIBILITY_CURRENT_BUILD", inv.ID, true)
			ready = false
		}
		if req.Memory.Bytes != 1<<30 || req.StdoutBytes > 64<<20 || req.StderrBytes > 1<<20 || req.Wall > 50000000000 || req.Grace != 2000000000 {
			s.finding("ELIGIBILITY_CURRENT_REQUEST_LIMIT", inv.ID, true)
			ready = false
		}
		batch := len(req.Args) > 0 && req.Args[0] == "--batch"
		libraryIndex, sourceIndex, queryStart, queryIDIndex := 0, 2, 3, 4
		pairs := 1
		if batch {
			libraryIndex, sourceIndex, queryStart, queryIDIndex = 1, 3, 6, 5
			pairs = (len(req.Args) - 5) / 2
		}
		if !batch && len(req.Args) != 6 || batch && (len(req.Args) < 7 || (len(req.Args)-5)%2 != 0 || pairs > 16) {
			s.finding("ELIGIBILITY_CURRENT_ARGV", inv.ID, true)
			s.ready[inv.ID] = false
			continue
		}
		symbolIndex := 1
		if batch {
			symbolIndex = 2
		}
		if s.reg.Native == nil {
			ready = false
		} else if req.Args[symbolIndex] != s.reg.Native.Symbol {
			s.finding("ELIGIBILITY_CURRENT_SYMBOL", inv.ID, true)
			ready = false
		}
		library, found := inv.Inputs["library"]
		if !found {
			s.finding("ELIGIBILITY_INPUT_MISSING", inv.ID+"/library", false)
			ready = false
		} else {
			if library.Kind != "PINNED" || library.ArgvIndex == nil || *library.ArgvIndex != uint64(libraryIndex) || library.PathLiteral != req.Args[libraryIndex] || library.MemberPin == nil {
				s.finding("ELIGIBILITY_CURRENT_LIBRARY_INPUT", inv.ID, true)
				ready = false
			}
			if s.reg.Native == nil || s.reg.Native.ReferenceLibrary == nil {
				ready = false
			} else if library.MemberPin == nil || *library.MemberPin != *s.reg.Native.ReferenceLibrary {
				s.finding("ELIGIBILITY_CURRENT_LIBRARY", inv.ID, true)
				ready = false
			}
		}
		source, sourceFound := inv.Inputs["source"]
		var sourceBytes []byte
		haveSource := false
		if !sourceFound {
			s.finding("ELIGIBILITY_INPUT_MISSING", inv.ID+"/source", false)
			ready = false
		} else {
			if source.Kind != "PINNED" || source.MemberPin == nil || source.ArgvIndex == nil || *source.ArgvIndex != uint64(sourceIndex) || source.PathLiteral != req.Args[sourceIndex] {
				s.finding("ELIGIBILITY_CURRENT_SOURCE_INPUT", inv.ID, true)
				ready = false
			} else {
				sourceBytes, haveSource, e = s.member(source.MemberPin)
				if e != nil {
					return e
				}
			}
		}
		ids := make([]string, pairs)
		seen := map[string]bool{}
		allowed := map[string]bool{"library": true, "source": true}
		queries := make([]QueryWitnessQuery, pairs)
		observations := make([]QueryReferenceObservation, pairs)
		knownStep := make([]bool, pairs)
		for i := 0; i < pairs; i++ {
			qi, ii := queryStart, queryIDIndex
			if batch {
				qi += 2 * i
				ii += 2 * i
			}
			ids[i] = req.Args[ii]
			if !validID(ids[i]) || seen[ids[i]] {
				s.finding("ELIGIBILITY_CURRENT_QUERY_ID", inv.ID, true)
				ready = false
			}
			seen[ids[i]] = true
			queryKey := "query-" + strconv.Itoa(i)
			allowed[queryKey] = true
			var selected *eligibilityObservation
			for j := range s.reg.Observations {
				o := &s.reg.Observations[j]
				if o.InvocationID == inv.ID && o.QueryID == ids[i] {
					if selected != nil && (selected.Query != o.Query || selected.Step != o.Step || selected.SourceBytes != o.SourceBytes || selected.SourceSHA256 != o.SourceSHA256) {
						s.finding("ELIGIBILITY_SHARED_INVOCATION_SOURCE", inv.ID, true)
						ready = false
					}
					selected = o
				}
			}
			if selected == nil {
				s.finding("ELIGIBILITY_CURRENT_QUERY_UNREFERENCED", inv.ID, false)
				queries[i].ID = ids[i]
				observations[i].Query = ids[i]
				if source.MemberPin != nil {
					observations[i].SourceSHA256 = source.MemberPin.Member.SHA256
					observations[i].SourceBytes = source.MemberPin.Member.Bytes
				}
				input, found := inv.Inputs[queryKey]
				if !found {
					s.finding("ELIGIBILITY_INPUT_MISSING", inv.ID+"/"+queryKey, false)
					ready = false
				} else if input.MemberPin == nil {
					s.finding("ELIGIBILITY_CURRENT_QUERY_INPUT", inv.ID, true)
				} else {
					data, have, e := s.member(input.MemberPin)
					if e != nil {
						return e
					}
					size := input.MemberPin.Member.Bytes
					if ok, e := s.input(inv, queryKey, "PINNED", eligibilityIndex(qi), req.Args[qi], input.MemberPin.Member.SHA256, &size); e != nil {
						return e
					} else if !ok {
						ready = false
					}
					if !have {
						ready = false
					}
					if have {
						if !utf8.Valid(data) {
							s.finding("ELIGIBILITY_CURRENT_QUERY_ENCODING", inv.ID, true)
						} else {
							queries[i].Source = string(data)
						}
					}
				}
				continue
			}
			knownStep[i] = true
			index := slices.IndexFunc(s.witness.Queries, func(q QueryWitnessQuery) bool { return q.ID == selected.Query })
			if index < 0 {
				s.finding("ELIGIBILITY_CURRENT_QUERY_UNKNOWN", inv.ID, true)
				ready = false
				continue
			}
			queries[i] = s.witness.Queries[index]
			queries[i].ID = ids[i]
			observations[i] = QueryReferenceObservation{Query: ids[i], Step: selected.Step, SourceBytes: selected.SourceBytes, SourceSHA256: selected.SourceSHA256}
			sha := digestHex([]byte(queries[i].Source))
			size := uint64(len(queries[i].Source))
			if ok, e := s.input(inv, queryKey, "PINNED", eligibilityIndex(qi), req.Args[qi], sha, &size); e != nil {
				return e
			} else if !ok {
				ready = false
			}
			if source.MemberPin != nil && (selected.SourceSHA256 != source.MemberPin.Member.SHA256 || selected.SourceBytes != source.MemberPin.Member.Bytes) {
				s.finding("ELIGIBILITY_REFERENCE_SOURCE_OBSERVATION", inv.ID, true)
				ready = false
			}
			if int(selected.Step) >= len(versions) {
				ready = false
			} else if selected.SourceSHA256 != sourceIDs[selected.Step].SHA256 || selected.SourceBytes != sourceIDs[selected.Step].Bytes || haveSource && !bytes.Equal(sourceBytes, versions[selected.Step]) {
				s.finding("ELIGIBILITY_REFERENCE_SOURCE_VERSION", inv.ID, true)
				ready = false
			}
		}
		if !s.inputSlots(inv, allowed) {
			ready = false
		}
		out, executionReady, e := s.receipt(inv, req, 0, false)
		if e != nil {
			return e
		}
		ready = ready && executionReady
		if out == nil {
			s.ready[inv.ID] = false
			continue
		}
		s.outputs[inv.ID] = out
		if batch {
			if len(out.stdout) == 0 || out.stdout[len(out.stdout)-1] != '\n' {
				s.finding("ELIGIBILITY_CURRENT_BATCH_EOF", inv.ID, true)
				s.ready[inv.ID] = false
				continue
			}
			out.rows = bytes.SplitN(out.stdout[:len(out.stdout)-1], []byte{'\n'}, pairs+2)
			if len(out.rows) != pairs+1 {
				s.finding("ELIGIBILITY_CURRENT_BATCH_COUNT", inv.ID, true)
				s.ready[inv.ID] = false
				continue
			}
			var footer eligibilityBatchFooter
			if e := s.b.decode(inv.ID+"/footer", out.rows[pairs], &footer, true); e != nil {
				return e
			}
			if footer.Schema != "tsgk-instrumented-query-reference-batch/r1" || footer.Status != StatusCompleted || !footer.Complete || !slices.Equal(footer.Queries, ids) || footer.ResponsesCompleted != uint64(pairs) {
				s.finding("ELIGIBILITY_CURRENT_BATCH_FOOTER", inv.ID, true)
				ready = false
			}
		} else {
			out.rows = [][]byte{out.stdout}
		}
		offset := uint64(0)
		for i := 0; i < pairs; i++ {
			out.offsets = append(out.offsets, offset)
			offset += uint64(len(out.rows[i]) + 1)
			hashArg := 5
			if batch {
				hashArg = 4
			}
			librarySHA := req.Args[hashArg]
			if !hex64.MatchString(librarySHA) || library.MemberPin != nil && librarySHA != library.MemberPin.Member.SHA256 || s.reg.Native != nil && s.reg.Native.ReferenceLibrary != nil && librarySHA != s.reg.Native.ReferenceLibrary.Member.SHA256 {
				s.finding("ELIGIBILITY_CURRENT_LIBRARY_ARGV", inv.ID, true)
				ready = false
			}
			symbol := req.Args[symbolIndex]
			obs := observations[i]
			responseVersions := versions
			if !knownStep[i] {
				responseVersions = nil
				if haveSource {
					responseVersions = [][]byte{sourceBytes}
					obs.Step = 0
				}
			}
			ref, code, complete, e := decodeReferenceResponseBound(out.rows[i], obs, librarySHA, symbol, queries[i], queries[i].Source != "", responseVersions, s.r)
			if e != nil {
				return e
			}
			out.responses = append(out.responses, ref)
			if code != "" {
				s.finding("ELIGIBILITY_CURRENT_RESPONSE", inv.ID+"/"+code, true)
				ready = false
			} else if !complete {
				s.finding("ELIGIBILITY_CURRENT_RESPONSE_PARTIAL", inv.ID, false)
				ready = false
			}
		}
		s.ready[inv.ID] = ready && s.failures == before
		if e := s.r.check(); e != nil {
			return e
		}
	}
	return nil
}

func eligibilityAllVerified(states map[string]string, n int) bool {
	if len(states) != n {
		return false
	}
	for _, state := range states {
		if state != "verified" {
			return false
		}
	}
	return true
}

func (s *eligibilitySession) nativeStaticClosure(symbol string) (map[string]eligibilitySnapshotRow, string, *Error) {
	files := map[string]eligibilitySnapshotRow{}
	read := func(name string) ([]byte, *Error) {
		info, err := fs.Stat(drivers.NativeC, "native-c/"+name)
		if err != nil || info.Size() < 0 || info.Size() > 4<<20 {
			return nil, fail(KindInvalidInput, "NATIVE_STATIC_CLOSURE", name, err)
		}
		if e := s.b.charge(1, 4*uint64(info.Size()), "native-static"); e != nil {
			return nil, e
		}
		data, err := drivers.NativeC.ReadFile("native-c/" + name)
		if err != nil {
			return nil, fail(KindInvalidInput, "NATIVE_STATIC_CLOSURE", name, err)
		}
		return data, nil
	}
	manifest, e := read("runtime-manifest.json")
	if e != nil {
		return nil, "", e
	}
	var runtime struct {
		Commit string `json:"commit"`
		Files  []struct {
			Path   string `json:"path"`
			Bytes  uint64 `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if e := s.b.reserveJSON("embedded-runtime-manifest", manifest, 3); e != nil {
		return nil, "", e
	}
	if err := jsonv2.Unmarshal(manifest, &runtime); err != nil {
		return nil, "", fail(KindInvalidInput, "NATIVE_STATIC_CLOSURE", "manifest", err)
	}
	if digestHex(manifest) != reuseobs.RuntimeManifestSHA256 {
		return nil, "", fail(KindInvalidInput, "NATIVE_STATIC_CLOSURE", "manifest", nil)
	}
	for _, row := range runtime.Files {
		files["runtime "+row.Path] = eligibilitySnapshotRow{Bytes: row.Bytes, SHA256: row.SHA256}
	}
	for _, name := range []string{"driver.c", "cp949_table.h"} {
		data, e := read(name)
		if e != nil {
			return nil, "", e
		}
		files["driver driver/"+name] = eligibilitySnapshotRow{Bytes: uint64(len(data)), SHA256: digestHex(data)}
	}
	shim := []byte(fmt.Sprintf("#include <tree_sitter/api.h>\nconst TSLanguage *%s(void);\nconst TSLanguage *tsgk_language(void) { return %s(); }\n", symbol, symbol))
	if e := s.b.charge(1, 2*uint64(len(shim)), "native-shim"); e != nil {
		return nil, "", e
	}
	files["driver driver/shim.c"] = eligibilitySnapshotRow{Bytes: uint64(len(shim)), SHA256: digestHex(shim)}
	return files, runtime.Commit, nil
}

// Native build identities use fmt's canonical quoted argv list, not shell syntax.
func eligibilityBuildStep(text string) (string, []string, bool) {
	name, list, found := strings.Cut(text, " ")
	if !found || !validID(name) || len(list) < 2 || list[0] != '[' || list[len(list)-1] != ']' {
		return "", nil, false
	}
	rest := list[1 : len(list)-1]
	args := []string{}
	for rest != "" {
		quoted, err := strconv.QuotedPrefix(rest)
		if err != nil {
			return "", nil, false
		}
		arg, err := strconv.Unquote(quoted)
		if err != nil || !utf8.ValidString(arg) || strings.ContainsRune(arg, 0) {
			return "", nil, false
		}
		args = append(args, arg)
		rest = rest[len(quoted):]
		if rest != "" {
			if rest[0] != ' ' {
				return "", nil, false
			}
			rest = rest[1:]
		}
	}
	return name, args, len(args) > 0 && fmt.Sprintf("%q", args) == list
}
