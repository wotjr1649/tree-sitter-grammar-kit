package kit

import (
	"bytes"
	"encoding/json/jsontext"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

type eligibilityNativeRecord struct {
	approved eligibilityNativeExpected
	process  eligibilityExecution
	raw      nativewire.Response
	haveRaw  bool
	ready    bool
}

func (s *eligibilitySession) nativeInvocation(inv *eligibilityInvocation, versions [][]byte) (eligibilityNativeRecord, *Error) {
	s.failItem = func() { s.failInvocation(inv) }
	defer func() { s.failItem = nil }()
	before := s.failures
	record := eligibilityNativeRecord{ready: s.ready[inv.ID]}
	if e := s.b.decode(inv.ID+"/approved-request", inv.ApprovedRequest, &record.approved, true); e != nil {
		return record, e
	}
	request, haveRequest, e := s.member(inv.Request)
	if e != nil {
		return record, e
	}
	if e := s.b.charge(8*uint64(len(request)+len(inv.ApprovedRequest)), 4*uint64(len(request)+len(inv.ApprovedRequest)), "native-request-compare"); e != nil {
		return record, e
	}
	if haveRequest {
		var stored eligibilityNativeExpected
		if e := s.b.decode(inv.ID+"/request", request, &stored, true); e != nil {
			return record, e
		}
		if e := eligibilityValidateRequest(stored.Process, inv.ID); e != nil {
			return record, e
		}
		if _, e := s.processPaths(inv.ID, stored.Process, s.cohorts[inv.CohortID].Platform); e != nil {
			return record, e
		}
		if !reflect.DeepEqual(stored, record.approved) {
			s.finding("ELIGIBILITY_NATIVE_REQUEST", inv.ID, true)
		}
	}
	if e := eligibilityValidateRequest(record.approved.Process, inv.ID); e != nil {
		return record, e
	}
	if _, e := s.processPaths(inv.ID, record.approved.Process, s.cohorts[inv.CohortID].Platform); e != nil {
		return record, e
	}
	a := record.approved
	if a.Protocol != nativewire.ProtocolR4 || !validID(a.RequestID) || a.NodeLimit == 0 || a.NodeLimit > 1000000 || !hex64.MatchString(a.ProducerSHA256) || !reuseobs.VerifiedRuntimeManifest(a.RuntimeManifestSHA256) || !validID(a.ObservationRunID) || a.HistoryStart > 4 {
		s.finding("ELIGIBILITY_R4_EXPECTATION", inv.ID, true)
	}
	_, haveExecutable, e := s.member(inv.Executable)
	if e != nil {
		return record, e
	}
	if inv.Executable == nil || inv.Executable.Member.SHA256 != a.ExecutableSHA256 || !hex64.MatchString(a.ExecutableSHA256) {
		s.finding("ELIGIBILITY_NATIVE_EXECUTABLE", inv.ID, true)
	}
	if a.Process.Path != inv.ExecutablePath {
		s.finding("ELIGIBILITY_NATIVE_EXECUTABLE_PATH", inv.ID, true)
	}
	process, haveProcess, e := s.member(inv.Process)
	if e != nil {
		return record, e
	}
	if haveProcess {
		if e := s.b.decode(inv.ID+"/process", process, &record.process, true); e != nil {
			return record, e
		}
		if !s.execution(inv.ID, record.process, a.Process, s.cohorts[inv.CohortID].Platform, 0, false) {
			record.ready = false
		}
		if record.process.StdoutBytes > a.Process.StdoutBytes || record.process.StderrBytes > a.Process.StderrBytes {
			s.finding("ELIGIBILITY_NATIVE_PROCESS_STREAM", inv.ID, true)
		}
	}
	record.raw, record.haveRaw, e = s.nativeResponse(inv, versions)
	if e != nil {
		return record, e
	}
	if record.haveRaw {
		if haveProcess && record.process.StdoutBytes != inv.Response.Member.Bytes+4 {
			s.finding("ELIGIBILITY_NATIVE_PROCESS_STREAM", inv.ID, true)
		}
		expect := nativewire.Expect{Revision: a.Protocol, ID: a.RequestID, Observation: &nativewire.Binding{Manifest: a.RuntimeManifestSHA256, RunID: a.ObservationRunID, ProducerSHA256: a.ProducerSHA256, HistoryStart: a.HistoryStart}}
		if haveProcess {
			expect.Exit = &record.process.ExitCode
		}
		if _, err := nativewire.CheckEnvelope(record.raw, len(record.raw.Steps), expect); err != nil {
			s.finding("ELIGIBILITY_R4_HEADER", inv.ID, true)
		}
		abi, p := a.ProducerABI, record.raw.Producer
		if a.RuntimeManifestSHA256 == reuseobs.RuntimeManifestSHA256 && (abi.RuntimeLanguageVersion != 15 || abi.RuntimeMinCompatible != 13) {
			s.finding("ELIGIBILITY_NATIVE_RUNTIME_ABI", inv.ID, true)
		}
		if abi.LanguageVersion == 0 || abi.RuntimeMinCompatible == 0 || abi.RuntimeMinCompatible > abi.LanguageVersion || abi.LanguageVersion > abi.RuntimeLanguageVersion || abi.SymbolCount == 0 || p.LanguageVersion != abi.LanguageVersion || p.RuntimeLanguageVersion != abi.RuntimeLanguageVersion || p.RuntimeMinCompatible != abi.RuntimeMinCompatible || p.SymbolCount == nil || *p.SymbolCount != abi.SymbolCount || p.FieldCount == nil || *p.FieldCount != abi.FieldCount || p.Query != "tsgk-query/r1" || p.API == nil || *p.API != "tsgk-api/r1" {
			s.finding("ELIGIBILITY_NATIVE_PRODUCER_ABI", inv.ID, true)
		}
		if ok, e := s.nativeIndependent(inv, record); e != nil {
			return record, e
		} else if !ok {
			record.ready = false
		}
	}
	record.ready = record.ready && haveRequest && haveExecutable && haveProcess && record.haveRaw && before == s.failures
	return record, nil
}

func (s *eligibilitySession) nativeIndependent(inv *eligibilityInvocation, record eligibilityNativeRecord) (bool, *Error) {
	a, raw := record.approved, record.raw
	before := s.failures
	good := true
	sources := map[string][]byte{}
	knownQueries := map[string]bool{}
	var initial []byte
	haveInitial := false
	initialSHA := ""
	for key, input := range inv.Inputs {
		if e := s.b.charge(1, uint64(len(key)), "native-independent-input"); e != nil {
			return false, e
		}
		if key == "library" {
			if input.Kind != "NOT_APPLICABLE" {
				s.finding("ELIGIBILITY_INPUT_APPLICABILITY", inv.ID+"/library", true)
			}
			continue
		}
		if key != "source" && (!strings.HasPrefix(key, "query-") || !validID(strings.TrimPrefix(key, "query-"))) {
			s.finding("ELIGIBILITY_INPUT_UNKNOWN", inv.ID+"/"+key, true)
			continue
		}
		if key != "source" {
			knownQueries[strings.TrimPrefix(key, "query-")] = true
		}
		if input.Kind != "PINNED" || input.MemberPin == nil || input.ArgvIndex != nil || input.PathLiteral != "" {
			s.finding("ELIGIBILITY_NATIVE_FRAME_INPUT", inv.ID+"/"+key, true)
			continue
		}
		data, have, e := s.member(input.MemberPin)
		if e != nil {
			return false, e
		}
		if !have {
			good = false
			continue
		}
		if key == "source" {
			initial, haveInitial, initialSHA = data, true, input.MemberPin.Member.SHA256
			if e := s.b.charge(uint64(len(data))+1, 0, "native-initial-utf8"); e != nil {
				return false, e
			}
			if !utf8.Valid(data) {
				s.finding("ELIGIBILITY_NATIVE_SOURCE_ENCODING", inv.ID, true)
			}
		} else {
			if len(data) == 0 || len(data) > 65536 {
				s.finding("ELIGIBILITY_NATIVE_QUERY_SOURCE", inv.ID+"/"+key, true)
			}
			sources[strings.TrimPrefix(key, "query-")] = data
		}
	}
	for _, key := range []string{"source", "library"} {
		if _, ok := inv.Inputs[key]; !ok {
			s.finding("ELIGIBILITY_INPUT_MISSING", inv.ID+"/"+key, false)
			good = false
		}
	}
	if haveInitial && raw.SourceBytes != uint64(len(initial)) {
		s.finding("ELIGIBILITY_NATIVE_SOURCE_TRANSPORT", inv.ID, true)
	}
	if a.HistoryStart > 4 || uint64(len(raw.Steps)) > 5-min(a.HistoryStart, 5) || raw.StepsCompleted > 5-min(a.HistoryStart, 5) || raw.Status == StatusCompleted && len(raw.Steps) == 0 {
		s.finding("ELIGIBILITY_NATIVE_HISTORY_RANGE", inv.ID, true)
	}
	expect := nativewire.Expect{Revision: a.Protocol, Nodes: a.NodeLimit, API: a.API, Points: a.Points, Check: func() error {
		if e := s.r.check(); e != nil {
			return e
		}
		return nil
	}}
	var tally nativewire.Tally
	var steps []reuseobs.Step
	var all []reuseobs.Observation
	for k, step := range raw.Steps {
		if e := s.b.charge(1, 0, "native-independent-step"); e != nil {
			return false, e
		}
		path := fmt.Sprintf("%s/%d", inv.ID, k)
		if step.Step != k || !hex64.MatchString(step.SourceSHA256) || (k == 0) != (step.Edit == nil) || k == 0 && (step.Route != nil || step.Fresh != nil) {
			s.finding("ELIGIBILITY_NATIVE_STEP_SHAPE", path, true)
		}
		if k == 0 && haveInitial && (step.SourceBytes != uint64(len(initial)) || step.SourceSHA256 != initialSHA) {
			s.finding("ELIGIBILITY_NATIVE_SOURCE_TRANSPORT", path, true)
		}
		if k == 0 && step.SourceBytes != raw.SourceBytes {
			s.finding("ELIGIBILITY_NATIVE_SOURCE_TRANSPORT", path, true)
		}
		if k > 0 && step.Edit != nil {
			valid := nativeIndependentEdit(*step.Edit, raw.Steps[k-1].SourceBytes, step.SourceBytes)
			if !valid {
				s.finding("ELIGIBILITY_NATIVE_EDIT_RANGE", path, true)
			}
			if valid && k == 1 && haveInitial {
				ed := step.Edit
				if uint64(ed.OldEndByte) > uint64(len(initial)) {
					s.finding("ELIGIBILITY_NATIVE_EDIT_RANGE", path, true)
				} else {
					if e := s.b.charge(2*uint64(len(initial))+1, 0, "native-independent-edit-points"); e != nil {
						return false, e
					}
					start, oldEnd := PointAt(EncodingUTF8, initial, int(ed.StartByte)), PointAt(EncodingUTF8, initial, int(ed.OldEndByte))
					if ed.StartPoint != [2]uint32{start.Row, start.Column} || ed.OldEndPoint != [2]uint32{oldEnd.Row, oldEnd.Column} {
						s.finding("ELIGIBILITY_NATIVE_EDIT_POINTS", path, true)
					}
				}
			}
		}
		cs := nativewire.CheckedStep{Step: step}
		for j, wire := range []*nativewire.WireTree{&step.Incremental, step.Fresh} {
			if wire == nil {
				continue
			}
			if e := s.nativeIndependentTreeBudget(*wire, initial, k == 0 && haveInitial); e != nil {
				return false, e
			}
			// This bound is the recorded step's own claim, not actual source proof.
			tree, err := nativewire.CheckTreeStructure(*wire, step.SourceBytes, expect)
			if e := s.nativeIndependentError(path, "ELIGIBILITY_NATIVE_TREE", err); e != nil {
				return false, e
			}
			if err == nil && wire.Status == StatusCompleted {
				if tree.Form == "full" && (len(tree.Nodes) == 0 || uint64(tree.Nodes[0].EndByte) != step.SourceBytes) {
					s.finding("ELIGIBILITY_NATIVE_TREE_EOF", path, true)
				}
				if e := s.nativeIndependentQueries(inv, &tree, sources, knownQueries, step.SourceBytes, expect.Check); e != nil {
					return false, e
				}
				if e := s.nativeIndependentError(path, "ELIGIBILITY_NATIVE_API", tree.CheckAPI(expect)); e != nil {
					return false, e
				}
				if k == 0 && haveInitial && tree.Form == "full" {
					if _, e := s.nativeTreeSource(path, tree.Nodes, initial); e != nil {
						return false, e
					}
				}
			}
			if j == 0 {
				cs.Incremental = tree
			} else {
				cs.Fresh = &tree
			}
		}
		if e := s.nativeIndependentError(path, "ELIGIBILITY_NATIVE_STEP_STATUS", nativewire.CheckOpen(k, len(raw.Steps), raw.Status, cs.Done(k))); e != nil {
			return false, e
		}
		tally.Add(k, cs)
		if e := s.b.reserveJSON("native-independent-logger", step.Observations, 8); e != nil {
			return false, e
		}
		var values []jsontext.Value
		if e := s.b.decode("native-independent-logger", step.Observations, &values, true); e != nil {
			return false, e
		}
		var observations []reuseobs.Observation
		for j, value := range values {
			o, err := reuseobs.Parse(value)
			if err != nil {
				return false, fail(KindInvalidInput, "JSON_SHAPE", path, err)
			}
			role := "initial"
			if k > 0 {
				role = "incremental"
				if j == 1 {
					role = "fresh"
				}
			}
			if o.RequestID != a.RequestID || o.RunID != a.ObservationRunID || o.ProducerSHA256 != a.ProducerSHA256 || o.RuntimeManifestSHA256 != a.RuntimeManifestSHA256 || o.HistoryStart != a.HistoryStart || o.Step != uint64(k) || o.Role != role {
				s.finding("ELIGIBILITY_NATIVE_LOGGER_BINDING", path, true)
			}
			// Cross-record consistency does not prove a later version's actual bytes.
			if o.SourceSHA256 != step.SourceSHA256 || o.SourceBytes != step.SourceBytes {
				s.finding("ELIGIBILITY_NATIVE_LOGGER_SOURCE", path, true)
			}
			if o.OldTree && k > 0 {
				previous := raw.Steps[k-1].SourceSHA256
				if k == 1 && haveInitial {
					previous = initialSHA
				}
				if o.OldSourceSHA256 == nil || *o.OldSourceSHA256 != previous {
					s.finding("ELIGIBILITY_NATIVE_LOGGER_OLD_SOURCE", path, true)
				}
			}
			observations = append(observations, o)
		}
		steps = append(steps, reuseobs.Step{Native: true, Edit: k > 0, Observations: observations})
		all = append(all, observations...)
	}
	if e := s.nativeIndependentError(inv.ID, "ELIGIBILITY_NATIVE_END", nativewire.CheckEnd(raw, tally, expect)); e != nil {
		return false, e
	}
	mandatory, full := reuseobs.CaseClaim(steps, raw.Status == StatusCompleted), reuseobs.Judge(all)
	if mandatory == "FAIL" || full == "FAIL" {
		s.finding("ELIGIBILITY_LOGGER_CONTRADICTION", inv.ID, true)
	} else if mandatory != "PASS" || full != "PASS" {
		s.finding("ELIGIBILITY_LOGGER_INCOMPLETE", inv.ID, false)
		good = false
	}
	return good && before == s.failures, nil
}

func (s *eligibilitySession) nativeIndependentError(path, code string, err error) *Error {
	if e, ok := err.(*Error); ok {
		return e
	}
	if err != nil {
		s.finding(code, path, true)
	}
	return s.r.check()
}

func (s *eligibilitySession) nativeIndependentQueries(inv *eligibilityInvocation, tree *nativewire.Tree, sources map[string][]byte, known map[string]bool, sourceBytes uint64, check func() error) *Error {
	if e := s.b.reserveJSON("native-independent-queries", tree.Wire.Queries, 24); e != nil {
		return e
	}
	var values []jsontext.Value
	if !bytes.Equal(bytes.Trim(tree.Wire.Queries, " \t\r\n"), []byte("null")) {
		if e := s.b.decode("native-independent-queries", tree.Wire.Queries, &values, true); e != nil {
			return e
		}
	}
	seen := map[string]bool{}
	for _, value := range values {
		if e := s.b.charge(1, uint64(len(value))+2, "native-independent-query-record"); e != nil {
			return e
		}
		data := make([]byte, len(value)+2)
		data[0], data[len(data)-1] = '[', ']'
		copy(data[1:], value)
		queries, err := querywire.ParseQueries(data)
		if err != nil {
			return fail(KindInvalidInput, "JSON_SHAPE", inv.ID+"/query", err)
		}
		q := queries[0]
		if seen[q.ID] {
			s.finding("ELIGIBILITY_NATIVE_QUERY_DUPLICATE", inv.ID+"/"+q.ID, true)
		}
		seen[q.ID] = true
		source, have := sources[q.ID]
		one := *tree
		one.Wire.Queries = data
		var queryError error
		if !have {
			s.finding("ELIGIBILITY_INPUT_MISSING", inv.ID+"/query-"+q.ID, false)
			queryError = one.CheckQueryStructure(sourceBytes, check)
		} else {
			if e := s.b.charge(2*uint64(len(source))+1, 0, "native-independent-query-source"); e != nil {
				return e
			}
			queryError = one.CheckQueries([]querywire.Source{{ID: q.ID, Source: source}}, sourceBytes, check)
		}
		if e := s.nativeIndependentError(inv.ID+"/"+q.ID, "ELIGIBILITY_NATIVE_QUERY", queryError); e != nil {
			return e
		}
		tree.Queries = append(tree.Queries, one.Queries...)
	}
	for id := range known {
		if e := s.b.charge(1, 0, "native-independent-query-set"); e != nil {
			return e
		}
		if !seen[id] {
			s.finding("ELIGIBILITY_NATIVE_QUERY_MISSING", inv.ID+"/"+id, true)
		}
	}
	return nil
}

func (s *eligibilitySession) nativeIndependentTreeBudget(tree nativewire.WireTree, source []byte, actualSource bool) *Error {
	tableWork, e := eligibilityProduct("native-independent-fields", uint64(len(tree.Fields)), uint64(len(tree.PartialTrees))+1)
	if e != nil {
		return e
	}
	if e := s.b.charge(tableWork, 0, "native-independent-fields"); e != nil {
		return e
	}
	reserve := func(rows [][]int64) *Error {
		for _, row := range rows {
			logical := uint64(256)
			if len(row) == 10 {
				for _, x := range []struct {
					index int64
					names []string
				}{{row[1], tree.Types}, {row[2], tree.Fields}} {
					if x.index >= 0 && x.index < int64(len(x.names)) {
						logical += 4 * uint64(len(x.names[x.index]))
					}
				}
			}
			if e := s.b.charge(256, logical, "native-independent-node"); e != nil {
				return e
			}
		}
		return nil
	}
	if e := reserve(tree.Nodes); e != nil {
		return e
	}
	for _, partial := range tree.PartialTrees {
		if e := s.b.charge(1, 0, "native-independent-partial"); e != nil {
			return e
		}
		if e := reserve(partial.Nodes); e != nil {
			return e
		}
	}
	if actualSource {
		return s.b.charge(5*uint64(len(source))+1, 16*uint64(len(source)), "native-independent-points")
	}
	return nil
}

// The byte and point relations do not establish the edited source's identity.
func nativeIndependentEdit(ed nativewire.StepEdit, oldBytes, newBytes uint64) bool {
	if ed.StartByte > ed.OldEndByte || ed.StartByte > ed.NewEndByte || uint64(ed.OldEndByte) > oldBytes || uint64(ed.NewEndByte) > newBytes ||
		oldBytes-uint64(ed.OldEndByte) != newBytes-uint64(ed.NewEndByte) || ed.OldEndByte == ed.StartByte && ed.NewEndByte == ed.StartByte {
		return false
	}
	validPoint := func(b uint32, p [2]uint32) bool {
		return uint64(p[0])+uint64(p[1]) <= uint64(b) && (p[0] != 0 || p[1] == b)
	}
	after := func(b uint32, p [2]uint32) bool {
		if !validPoint(b, p) || p[0] < ed.StartPoint[0] {
			return false
		}
		if p[0] == ed.StartPoint[0] {
			return p[1] >= ed.StartPoint[1] && b-ed.StartByte == p[1]-ed.StartPoint[1]
		}
		return uint64(p[0]-ed.StartPoint[0])+uint64(p[1]) <= uint64(b-ed.StartByte)
	}
	return validPoint(ed.StartByte, ed.StartPoint) && after(ed.OldEndByte, ed.OldEndPoint) && after(ed.NewEndByte, ed.NewEndPoint)
}

// Record a definite source contradiction before returning an interruption.
func (s *eligibilitySession) nativeTreeSource(path string, nodes []TreeNode, source []byte) (bool, *Error) {
	code := ""
	if !utf8.Valid(source) {
		code = "ELIGIBILITY_NATIVE_SOURCE_ENCODING"
	} else if len(nodes) == 0 || uint64(nodes[0].EndByte) != uint64(len(source)) {
		code = "ELIGIBILITY_NATIVE_TREE_EOF"
	} else if !witnessCST(nodes) {
		code = "ELIGIBILITY_NATIVE_TREE_CST"
	} else if witnessPoints(nodes, EncodingUTF8, source, s.r) != "" {
		code = "ELIGIBILITY_NATIVE_TREE_POINTS"
	}
	if code != "" {
		s.finding(code, path, true)
	}
	return code == "", s.r.check()
}
