package kit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const ordinaryReferenceSelectionSchema = "tsgk-query-ordinary-reference-selection/r1"

// ordinaryReferenceSelection pins one ordered response row per case, source version and query.
type ordinaryReferenceSelection struct {
	Schema string                          `json:"schema"`
	Rows   []ordinaryReferenceSelectionRow `json:"rows"`
}

type ordinaryReferenceSelectionRow struct {
	Case            string      `json:"case_id"`
	Step            uint32      `json:"step"`
	Query           string      `json:"query_id"`
	Source          OrdinaryPin `json:"source_pin"`
	Invocation      string      `json:"invocation_id"`
	ResponseOrdinal uint64      `json:"response_ordinal"`
	ByteOffset      uint64      `json:"byte_offset"`
	ByteLength      uint64      `json:"byte_length"`
	SHA256          string      `json:"sha256"`
}

type ordinaryReferenceRow struct {
	Selection ordinaryReferenceSelectionRow
	Response  *instrumentedReference
	Ready     bool
}

func parseOrdinaryReferenceSelection(data []byte) (ordinaryReferenceSelection, error) {
	var selection ordinaryReferenceSelection
	if err := ordinaryDecode(data, &selection); err != nil {
		return selection, err
	}
	return validateOrdinaryReferenceSelection(selection)
}

func validateOrdinaryReferenceSelection(selection ordinaryReferenceSelection) (ordinaryReferenceSelection, error) {
	if selection.Schema != ordinaryReferenceSelectionSchema || selection.Rows == nil {
		return selection, ordinaryBad("ORDINARY_REFERENCE_SELECTION")
	}
	seen := map[string]bool{}
	for _, row := range selection.Rows {
		key := fmt.Sprintf("%s/%d/%s", row.Case, row.Step, row.Query)
		if row.Case == "" || row.Query == "" || !validID(row.Invocation) || !ordinaryPin(row.Source) || row.ByteLength == 0 || row.ByteLength > MaxDocumentBytes || !hex64.MatchString(row.SHA256) || seen[key] {
			return ordinaryReferenceSelection{}, ordinaryBad("ORDINARY_REFERENCE_SELECTION_ROW")
		}
		seen[key] = true
	}
	return selection, nil
}

// ordinaryReferenceRows validates selected actual process output under the existing eligibility
// session and ledger. Reference rows are returned only for comparison with independent assets.
func (s *eligibilitySession) ordinaryReferences(reg OrdinaryRegistration, selectionData []byte, sources map[string][][]byte) (map[string]ordinaryReferenceRow, *Error) {
	result := map[string]ordinaryReferenceRow{}
	addFailure := func(code, path string) { s.finding(code, path, true) }
	if s == nil || s.ordinary == nil || s.ordinary.session != s || s.snapshot == nil || !s.snapshot.graphBound || len(selectionData) == 0 {
		return result, fail(KindInvalidInput, "ELIGIBILITY_ORDINARY_SCOPE", "reference-selection", nil)
	}
	pinnedSelection := s.ordinary.Binding.ReferenceSelection
	if (OrdinaryPin{Path: pinnedSelection.Path, Bytes: uint64(len(selectionData)), SHA256: digestHex(selectionData)}) != pinnedSelection {
		s.finding("ORDINARY_REFERENCE_SELECTION_PIN", "reference-selection", true)
		return result, nil
	}
	var selection ordinaryReferenceSelection
	if e := s.b.decodeOrdinaryTyped("ordinary-reference-selection", selectionData, &selection); e != nil {
		return result, e
	}
	var err error
	if selection, err = validateOrdinaryReferenceSelection(selection); err != nil {
		return result, asError(err)
	}
	// Fix the full source/query denominator before consulting observed rows.
	casePins := map[string][]OrdinaryPin{}
	queryPins := map[string]OrdinaryPin{}
	selectedCases := []OrdinaryCase{}
	for _, c := range reg.Cases {
		if versions, selected := sources[c.ID]; selected {
			casePins[c.ID] = c.VersionPins
			if len(versions) != len(c.VersionPins) {
				return result, fail(KindInvalidInput, "ORDINARY_REFERENCE_SOURCE_HISTORY", c.ID, nil)
			}
			selectedCases = append(selectedCases, c)
		}
	}
	for _, q := range reg.Queries {
		queryPins[q.ID] = q.Source
	}
	expectedOrder := ordinaryReferenceKeys(reg, sources)
	expected := make(map[string]bool, len(expectedOrder))
	for _, key := range expectedOrder {
		expected[key] = true
	}
	if len(selection.Rows) != len(expected) {
		addFailure("ORDINARY_REFERENCE_SELECTION_GAP", "reference-selection")
		return result, nil
	}
	if !ordinaryReferenceSelectionOrdered(selection, expectedOrder) {
		addFailure("ORDINARY_REFERENCE_ORDER", "reference-selection")
		return result, nil
	}
	byInvocation := map[string][]ordinaryReferenceSelectionRow{}
	invocationOrder := []string{}
	for i, row := range selection.Rows {
		key := ordinaryReferenceKey(row.Case, row.Step, row.Query)
		if i >= len(expectedOrder) || key != expectedOrder[i] || !expected[key] || result[key].Selection.Case != "" || int(row.Step) >= len(casePins[row.Case]) || casePins[row.Case][row.Step] != row.Source || queryPins[row.Query].SHA256 == "" {
			addFailure("ORDINARY_REFERENCE_SELECTION_BINDING", key)
			continue
		}
		result[key] = ordinaryReferenceRow{Selection: row}
		if _, found := byInvocation[row.Invocation]; !found {
			invocationOrder = append(invocationOrder, row.Invocation)
		}
		byInvocation[row.Invocation] = append(byInvocation[row.Invocation], row)
		delete(expected, key)
	}
	if len(expected) != 0 {
		addFailure("ORDINARY_REFERENCE_SELECTION_GAP", "reference-selection")
		return result, nil
	}
	for _, invocation := range invocationOrder {
		beforeInvocation := s.failures
		rows := byInvocation[invocation]
		inv := s.invocations[invocation]
		if inv == nil || inv.Kind != "REFERENCE_PROCESS" || s.cohorts[inv.CohortID].Purpose != "current" {
			addFailure("ORDINARY_REFERENCE_INVOCATION", invocation)
			continue
		}
		if !slices.IsSortedFunc(rows, func(a, b ordinaryReferenceSelectionRow) int {
			if a.ResponseOrdinal < b.ResponseOrdinal {
				return -1
			}
			if a.ResponseOrdinal > b.ResponseOrdinal {
				return 1
			}
			return 0
		}) {
			addFailure("ORDINARY_REFERENCE_ORDER", invocation)
			continue
		}
		first := rows[0]
		for i, row := range rows {
			if row.Case != first.Case || row.Step != first.Step || row.Source != first.Source || row.ResponseOrdinal != uint64(i) {
				addFailure("ORDINARY_REFERENCE_SHARED_SOURCE", invocation)
			}
		}
		if s.ready[invocation] == false {
			continue
		}
		req, requestOK, e := s.request(inv)
		if e != nil {
			return result, e
		}
		if !requestOK || inv.ToolID != "" || inv.Executable == nil || req.Path != inv.ExecutablePath || s.reg.Build == nil || s.reg.Build.ReferenceBinary == nil || inv.Executable.Member.SHA256 != s.reg.Build.ReferenceBinary.Member.SHA256 {
			addFailure("ORDINARY_REFERENCE_BUILD_BINDING", invocation)
			continue
		}
		if ok, e := s.memberAvailable(inv.Executable); e != nil {
			return result, e
		} else if !ok {
			continue
		}
		if req.Memory.Bytes != 1<<30 || req.StdoutBytes > 64<<20 || req.StderrBytes > 1<<20 || req.Wall > 50000000000 || req.Grace != 2000000000 {
			addFailure("ORDINARY_REFERENCE_REQUEST_LIMIT", invocation)
			continue
		}
		batch := len(req.Args) > 0 && req.Args[0] == "--batch"
		libraryIndex, sourceIndex, queryStart, queryIDIndex, hashIndex, symbolIndex := 0, 2, 3, 4, 5, 1
		if batch {
			libraryIndex, sourceIndex, queryStart, queryIDIndex, hashIndex, symbolIndex = 1, 3, 6, 5, 4, 2
		}
		pairs := 1
		if batch {
			if len(req.Args) < 7 || (len(req.Args)-5)%2 != 0 {
				addFailure("ORDINARY_REFERENCE_ARGV", invocation)
				continue
			}
			pairs = (len(req.Args) - 5) / 2
		}
		if pairs != len(rows) || pairs > 15 || !batch && len(req.Args) != 6 || batch && len(req.Args) != 5+2*pairs {
			addFailure("ORDINARY_REFERENCE_ARGV", invocation)
			continue
		}
		platform := s.cohorts[inv.CohortID].Platform
		for _, i := range []int{libraryIndex, sourceIndex} {
			if !eligibilityLiteral(req.Args[i]) || !eligibilityReferenceAbsolutePath(req.Args[i], platform) {
				addFailure("ORDINARY_REFERENCE_PATH", invocation)
			}
		}
		libInput, hasLibrary := inv.Inputs["library"]
		sourceInput, hasSource := inv.Inputs["source"]
		if !hasLibrary || libInput.MemberPin == nil || !hasSource || sourceInput.MemberPin == nil || libInput.PathLiteral != req.Args[libraryIndex] || sourceInput.PathLiteral != req.Args[sourceIndex] {
			addFailure("ORDINARY_REFERENCE_INPUT", invocation)
			continue
		}
		if libInput.MemberPin.Member.Role != "reference-library" && libInput.MemberPin.Member.Role != "library-paged-descriptor" {
			addFailure("ORDINARY_REFERENCE_LIBRARY_OWNER", invocation)
			continue
		}
		plannedLibrary := false
		for _, member := range reg.PlannedMembers {
			plannedLibrary = plannedLibrary || member.Owner == "D_build" && member.Role == libInput.MemberPin.Member.Role && member.Pin.Bytes == libInput.MemberPin.Member.Bytes && member.Pin.SHA256 == libInput.MemberPin.Member.SHA256 && strings.TrimPrefix(libInput.MemberPin.Member.Path, "oracle/") == member.Path
		}
		if !plannedLibrary {
			addFailure("ORDINARY_REFERENCE_LIBRARY_PROFILE", invocation)
			continue
		}
		sourceBytes, haveSource, e := s.member(sourceInput.MemberPin)
		if e != nil {
			return result, e
		}
		if !haveSource || digestHex(sourceBytes) != first.Source.SHA256 || uint64(len(sourceBytes)) != first.Source.Bytes || !bytes.Equal(sourceBytes, sources[first.Case][first.Step]) {
			addFailure("ORDINARY_REFERENCE_SOURCE", invocation)
			continue
		}
		if _, e := s.input(inv, "source", "PINNED", eligibilityIndex(sourceIndex), req.Args[sourceIndex], first.Source.SHA256, &first.Source.Bytes); e != nil {
			return result, e
		}
		allowed := map[string]bool{"source": true, "library": true}
		queries := make([]QueryWitnessQuery, pairs)
		queryIDs := make([]string, pairs)
		for i, row := range rows {
			qindex := queryStart
			idIndex := queryIDIndex
			inputKey := "query-0"
			if batch {
				qindex += 2 * i
				idIndex += 2 * i
				inputKey = "query-" + strconv.Itoa(i)
			}
			q, queryOK := queryPins[row.Query]
			input, inputOK := inv.Inputs[inputKey]
			if !queryOK || !inputOK || input.MemberPin == nil || req.Args[idIndex] != row.Query || req.Args[qindex] != input.PathLiteral {
				addFailure("ORDINARY_REFERENCE_QUERY_BINDING", invocation)
				continue
			}
			queryBytes, have, e := s.member(input.MemberPin)
			if e != nil {
				return result, e
			}
			if !have || digestHex(queryBytes) != q.SHA256 || uint64(len(queryBytes)) != q.Bytes {
				addFailure("ORDINARY_REFERENCE_QUERY_SOURCE", row.Query)
				continue
			}
			if ok, e := s.input(inv, inputKey, "PINNED", eligibilityIndex(qindex), req.Args[qindex], q.SHA256, &q.Bytes); e != nil {
				return result, e
			} else if !ok {
				continue
			}
			queries[i] = QueryWitnessQuery{ID: row.Query, Source: string(queryBytes)}
			queryIDs[i] = row.Query
			allowed[inputKey] = true
		}
		if !s.inputSlots(inv, allowed) || req.Args[symbolIndex] == "" || req.Args[hashIndex] == "" {
			addFailure("ORDINARY_REFERENCE_INPUT_SET", invocation)
			continue
		}
		librarySHA, libraryReady, e := s.ordinaryReferenceLibrary(libInput.MemberPin)
		if e != nil {
			return result, e
		}
		if !libraryReady || req.Args[hashIndex] != librarySHA || req.Args[symbolIndex] == "" {
			addFailure("ORDINARY_REFERENCE_LIBRARY_IDENTITY", invocation)
			continue
		}
		if ok, e := s.input(inv, "library", "PINNED", eligibilityIndex(libraryIndex), req.Args[libraryIndex], libInput.MemberPin.Member.SHA256, &libInput.MemberPin.Member.Bytes); e != nil {
			return result, e
		} else if !ok {
			continue
		}
		out, executionReady, e := s.receipt(inv, req, 0, false)
		if e != nil {
			return result, e
		}
		if out == nil || !executionReady {
			continue
		}
		if batch {
			if len(out.stdout) == 0 || out.stdout[len(out.stdout)-1] != '\n' {
				addFailure("ORDINARY_REFERENCE_FOOTER", invocation)
				continue
			}
			out.rows = bytes.SplitN(out.stdout[:len(out.stdout)-1], []byte{'\n'}, pairs+2)
			if len(out.rows) != pairs+1 {
				addFailure("ORDINARY_REFERENCE_ROW_COUNT", invocation)
				continue
			}
			var footer eligibilityBatchFooter
			if e := s.b.decodeWitnessTyped(invocation+"/footer", out.rows[pairs], &footer, true); e != nil {
				return result, e
			}
			if footer.Schema != "tsgk-instrumented-query-reference-batch/r1" || footer.Status != StatusCompleted || !footer.Complete || !slices.Equal(footer.Queries, queryIDs) || footer.ResponsesCompleted != uint64(pairs) {
				addFailure("ORDINARY_REFERENCE_FOOTER", invocation)
				continue
			}
		} else {
			if len(out.stdout) == 0 || out.stdout[len(out.stdout)-1] != '\n' {
				addFailure("ORDINARY_REFERENCE_ROW_TERMINATOR", invocation)
				continue
			}
			out.rows = [][]byte{out.stdout[:len(out.stdout)-1]}
		}
		out.responses = make([]*instrumentedReference, pairs)
		offset := uint64(0)
		for i, row := range rows {
			actual := out.rows[i]
			if uint64(len(actual)) != row.ByteLength || digestHex(actual) != row.SHA256 || row.ByteOffset != offset || row.ResponseOrdinal != uint64(i) {
				addFailure("ORDINARY_REFERENCE_ROW_PIN", fmt.Sprintf("%s/%d", invocation, i))
				offset += uint64(len(actual) + 1)
				continue
			}
			out.offsets = append(out.offsets, offset)
			obs := QueryReferenceObservation{Query: row.Query, Step: 0, SourceBytes: uint64(len(sourceBytes)), SourceSHA256: digestHex(sourceBytes)}
			ref, code, complete, e := decodeReferenceResponseRow(actual, obs, librarySHA, req.Args[symbolIndex], s.catalogue.UpstreamCommit, queries[i], true, [][]byte{sourceBytes}, s.r, true)
			if e != nil {
				return result, e
			}
			out.responses[i] = ref
			if code != "" || !complete || ref == nil {
				addFailure("ORDINARY_REFERENCE_RESPONSE", invocation+"/"+code)
				offset += uint64(len(actual) + 1)
				continue
			}
			key := ordinaryReferenceKey(row.Case, row.Step, row.Query)
			selected := result[key]
			selected.Response, selected.Ready = ref, s.ready[invocation] && s.failures == beforeInvocation
			result[key] = selected
			offset += uint64(len(actual) + 1)
		}
		s.outputs[invocation] = out
	}
	return result, s.r.check()
}

func ordinaryReferenceKey(caseID string, step uint32, query string) string {
	return fmt.Sprintf("%s/%d/%s", caseID, step, query)
}

func ordinaryReferenceKeys(reg OrdinaryRegistration, sources map[string][][]byte) []string {
	selected := make(map[string]bool, len(sources))
	for caseID := range sources {
		selected[caseID] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, required := range reg.Required {
		c := required.Coordinate
		key := ordinaryReferenceKey(c.Case, c.Step, c.Query)
		if selected[c.Case] && !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	return out
}

func ordinaryReferenceSelectionOrdered(selection ordinaryReferenceSelection, keys []string) bool {
	if len(selection.Rows) != len(keys) {
		return false
	}
	for i, row := range selection.Rows {
		if ordinaryReferenceKey(row.Case, row.Step, row.Query) != keys[i] {
			return false
		}
	}
	return true
}

// ordinaryReferenceLibrary hashes binary-paged libraries from the #366 verified chain,
// without assembling the library into a second byte slice.
func (s *eligibilitySession) ordinaryReferenceLibrary(pin *eligibilityMember) (string, bool, *Error) {
	data, ok, e := s.member(pin)
	if e != nil || !ok {
		return "", false, e
	}
	if pin.Member.Role != "library-paged-descriptor" {
		return pin.Member.SHA256, pin.Member.Role == "reference-library" && digestHex(data) == pin.Member.SHA256, nil
	}
	if !ordinaryPagingEnabled(s.ordinary) {
		return "", false, nil
	}
	var d semanticBinaryDescriptor
	if e := s.b.decodeOrdinaryTyped("ordinary-reference-library-descriptor", data, &d); e != nil {
		return "", false, e
	}
	physicalPath, found := strings.CutSuffix(pin.Member.Path, ".pages.json")
	path := strings.TrimPrefix(physicalPath, "oracle/")
	if !found || path == physicalPath || d.Schema != semanticBinaryPagesSchema || d.Owner != pin.NodeID || d.Logical.Kind != "library" || d.Logical.Path != path || d.Logical.Bytes <= MaxDocumentBytes || d.Logical.Bytes > semanticBinaryLogicalMax || !hex64.MatchString(d.Logical.SHA256) || uint64(len(d.Parts)) != (d.Logical.Bytes+MaxDocumentBytes-1)/MaxDocumentBytes {
		return "", false, nil
	}
	used := map[string]bool{pin.Member.Path: true}
	verified, e := s.pageSource(*pin, "oracle/", used)
	if e != nil || verified == nil {
		return "", false, e
	}
	h := sha256.New()
	var chain *streamedChain
	for i, item := range d.Parts {
		wantPath := fmt.Sprintf("%s.part-%05d.bin", path, i)
		wantBytes := min(MaxDocumentBytes, d.Logical.Bytes-uint64(i)*MaxDocumentBytes)
		if item.Index != uint64(i) || item.Offset != uint64(i)*MaxDocumentBytes || item.Bytes != wantBytes || item.Path != wantPath || !hex64.MatchString(item.SHA256) {
			return "", false, nil
		}
		part, ok, rejected, e := verified.load(ReplayMember{Path: item.Path, Role: "library-paged-part", Bytes: item.Bytes, SHA256: item.SHA256})
		if e != nil {
			return "", false, e
		}
		if rejected || !ok {
			return "", false, nil
		}
		if part.chain != nil {
			if chain == nil {
				chain = part.chain
			}
			if chain != part.chain || part.ordinal != i || i >= len(chain.pins) || chain.pins[i].Path != "oracle/"+item.Path || chain.pins[i].Role != "library-paged-part" || chain.pins[i].Bytes != item.Bytes || chain.pins[i].SHA256 != item.SHA256 {
				return "", false, nil
			}
			continue
		}
		if chain != nil || uint64(len(part.content)) != item.Bytes {
			return "", false, nil
		}
		if e := s.b.charge(1, 2*(item.Bytes+1), "ordinary-reference-library-part-hash"); e != nil {
			return "", false, e
		}
		_, _ = h.Write(part.content)
	}
	if chain != nil {
		if len(chain.pins) != len(d.Parts) || chain.next != len(d.Parts) || hex.EncodeToString(chain.Sum(nil)) != d.Logical.SHA256 {
			return "", false, nil
		}
	} else if hex.EncodeToString(h.Sum(nil)) != d.Logical.SHA256 {
		return "", false, nil
	}
	return d.Logical.SHA256, verified.complete, nil
}
