package kit

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"io"
	"reflect"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

type ordinaryRegistrationSelection struct {
	Registration OrdinaryRegistration
	CaseIndices  []int
	TotalCases   int
	TotalQueries int
}

type ordinaryJSONSpan struct{ start, end int }

type ordinaryRegistrationSpans struct {
	root   map[string]ordinaryJSONSpan
	arrays map[string][]ordinaryJSONSpan
}

type ordinaryCaseHeader struct {
	ID string `json:"case_id"`
}

type ordinaryQueryHeader struct {
	ID string `json:"id"`
}

type ordinaryRequiredCoordinateHeader struct {
	Case     string `json:"case"`
	Query    string `json:"query"`
	Step     uint32 `json:"step"`
	TreeRole string `json:"tree_role"`
}

type ordinaryRequiredHeader struct {
	ID         string                           `json:"obligation_id"`
	Coordinate ordinaryRequiredCoordinateHeader `json:"coordinate"`
}

type ordinaryRegistrationTuple struct {
	caseID  string
	queryID string
	step    uint32
	role    string
}

// decodeOrdinaryRegistrationSelection validates the complete registration token stream and
// its global ID union, then materializes only selected cases and required rows.
func (b *eligibilityBudget) decodeOrdinaryRegistrationSelection(data []byte, selected []OrdinaryPlanShard) (ordinaryRegistrationSelection, *Error) {
	var out ordinaryRegistrationSelection
	const doc = "ordinary-registration"
	if len(data) > int(OrdinaryPlanLimits().FileBytes) {
		return out, fail(KindResourceLimit, "ORDINARY_DOCUMENT_LIMIT", doc, nil)
	}
	if len(selected) == 0 {
		return out, fail(KindInvalidInput, "ORDINARY_REGISTRATION_SELECTION", doc, nil)
	}
	shape, e := b.typedJSONShapeFor(doc, reflect.TypeFor[OrdinaryRegistration](), false, true)
	if e != nil {
		return out, e
	}
	ordinaryTypedNulls(shape)
	if e = b.charge(1, uint64(len(data)), doc); e != nil {
		return out, e
	}
	if e = b.charge(1, 5*uint64(len(data))+32768, "ordinary-registration-index"); e != nil {
		return out, e
	}
	spans, visits, e := b.indexOrdinaryRegistration(data)
	if e != nil {
		return out, e
	}
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	var values uint64
	if e = b.readTypedJSON(doc, dec, shape, &values); e != nil {
		return out, e
	}
	if e = b.charge(values, 0, doc); e != nil {
		return out, e
	}
	if e = b.charge(visits, 0, "ordinary-registration-index"); e != nil {
		return out, e
	}
	for _, key := range []string{"cases", "queries", "required"} {
		if _, ok := spans.arrays[key]; !ok {
			return out, fail(KindInvalidInput, "ORDINARY_REGISTRATION_ARRAY", doc, nil)
		}
	}
	header, e := b.decodeOrdinaryRegistrationHeader(data, spans)
	if e != nil {
		return out, e
	}
	out.Registration = header
	out.TotalCases = len(spans.arrays["cases"])
	out.TotalQueries = len(spans.arrays["queries"])

	selectedIDCount := uint64(len(selected))
	for _, shard := range selected {
		selectedIDCount += uint64(len(shard.ObligationIDs))
	}
	if e := b.charge(selectedIDCount, 96*selectedIDCount, "ordinary-registration-selection-index"); e != nil {
		return out, e
	}
	selectedIDs := make(map[string]bool, len(selected))
	selectedObligationIDs := make(map[string]bool)
	for _, shard := range selected {
		if shard.Route != header.Route || shard.Platform != header.Platform || shard.Subject != header.Subject || shard.Set != header.Set || shard.Case == "" || selectedIDs[shard.Case] {
			return out, fail(KindInvalidInput, "ORDINARY_REGISTRATION_SELECTION", doc, nil)
		}
		selectedIDs[shard.Case] = true
		for _, id := range shard.ObligationIDs {
			if selectedObligationIDs[id] {
				return out, fail(KindInvalidInput, "ORDINARY_REGISTRATION_SELECTION", doc, nil)
			}
			selectedObligationIDs[id] = true
		}
	}
	if e := b.charge(uint64(out.TotalCases), 32*uint64(out.TotalCases), "ordinary-registration-case-index"); e != nil {
		return out, e
	}
	caseIDs := make(map[string]uint32, out.TotalCases)
	for i, span := range spans.arrays["cases"] {
		raw := ordinarySpanBytes(data, span)
		var c ordinaryCaseHeader
		if er := b.decodeOrdinaryRegistrationHeaderRow("ordinary-registration-case-index", raw, &c); er != nil {
			return out, er
		}
		steps, er := b.ordinaryRegistrationArrayLength(raw, "version_pins")
		if er != nil {
			return out, er
		}
		if c.ID == "" || steps == 0 || caseIDs[c.ID] != 0 {
			return out, fail(KindInvalidInput, "ORDINARY_CASE", doc, nil)
		}
		caseIDs[c.ID] = uint32(steps)
		if selectedIDs[c.ID] {
			var full OrdinaryCase
			if er := b.decodeOrdinaryTyped("ordinary-registration-selected-case", raw, &full); er != nil {
				return out, er
			}
			if full.ID != c.ID || len(full.VersionPins) != len(full.Edits)+1 || len(full.Points) != len(full.Edits) || full.OriginalSelector == "" || full.InitialSource != full.VersionPins[0] {
				return out, fail(KindInvalidInput, "ORDINARY_CASE", doc, nil)
			}
			for _, pin := range full.VersionPins {
				if !ordinaryPin(pin) || pin.Path != "source-witness/"+pin.SHA256+".utf8" {
					return out, fail(KindInvalidInput, "ORDINARY_SOURCE_BINDING", doc, nil)
				}
			}
			out.Registration.Cases = append(out.Registration.Cases, full)
			out.CaseIndices = append(out.CaseIndices, i)
		}
	}
	if len(out.Registration.Cases) != len(selected) {
		return out, fail(KindInvalidInput, "ORDINARY_REGISTRATION_SELECTION", doc, nil)
	}

	if e := b.charge(uint64(out.TotalQueries), 32*uint64(out.TotalQueries), "ordinary-registration-query-index"); e != nil {
		return out, e
	}
	queryIDs := make(map[string]bool, out.TotalQueries)
	for _, span := range spans.arrays["queries"] {
		raw := ordinarySpanBytes(data, span)
		var header ordinaryQueryHeader
		if er := b.decodeOrdinaryRegistrationHeaderRow("ordinary-registration-query-index", raw, &header); er != nil {
			return out, er
		}
		if header.ID == "" || queryIDs[header.ID] {
			return out, fail(KindInvalidInput, "ORDINARY_QUERY_BINDING", doc, nil)
		}
		queryIDs[header.ID] = true
		var query OrdinaryQuery
		if er := b.decodeOrdinaryTyped("ordinary-registration-selected-query", raw, &query); er != nil {
			return out, er
		}
		if query.ID != header.ID || !ordinaryPin(query.Source) || query.Kind != queryregistrycore.Structural || query.MetadataAsset == "" {
			return out, fail(KindInvalidInput, "ORDINARY_QUERY_BINDING", doc, nil)
		}
		out.Registration.Queries = append(out.Registration.Queries, query)
	}

	if len(out.Registration.Queries) == 0 {
		return out, fail(KindInvalidInput, "ORDINARY_QUERY_BINDING", doc, nil)
	}
	for _, shard := range selected {
		if len(shard.SourceHistory) == 0 || shard.Route == "" || shard.Platform == "" {
			return out, fail(KindInvalidInput, "ORDINARY_REGISTRATION_SELECTION", doc, nil)
		}
	}

	// Check the full case/query/step/role union without hashing unselected IDs.
	coordinateCount := uint64(0)
	for _, steps := range caseIDs {
		coordinateCount += (2*uint64(steps) - 1) * uint64(len(out.Registration.Queries))
	}
	if e := b.charge(coordinateCount, 64*coordinateCount, "ordinary-registration-coordinate-index"); e != nil {
		return out, e
	}
	if e := b.charge(uint64(len(selectedObligationIDs)), 128*uint64(len(selectedObligationIDs)), "ordinary-registration-selected-index"); e != nil {
		return out, e
	}
	expected := make(map[ordinaryRegistrationTuple]bool)
	selectedExpected := make(map[string]OrdinaryCoordinate, len(selectedObligationIDs))
	for caseID, steps := range caseIDs {
		for _, q := range out.Registration.Queries {
			for step := uint32(0); step < steps; step++ {
				for _, treeRole := range []string{"incremental", "fresh"} {
					if step == 0 && treeRole == "fresh" {
						continue
					}
					tuple := ordinaryRegistrationTuple{caseID: caseID, queryID: q.ID, step: step, role: treeRole}
					if expected[tuple] {
						return out, fail(KindInvalidInput, "ORDINARY_REQUIRED_COORDINATE", doc, nil)
					}
					expected[tuple] = true
					if selectedIDs[caseID] {
						coord := ordinarySelectedCoordinate(header, caseID, q.ID, step, treeRole)
						id, er := b.ordinaryCoordinateID(coord, header.Namespace)
						if er != nil {
							return out, er
						}
						if !selectedObligationIDs[id] {
							return out, fail(KindInvalidInput, "ORDINARY_REQUIRED_COORDINATE", doc, nil)
						}
						selectedExpected[id] = coord
					}
				}
			}
		}
	}
	if len(expected) == 0 {
		return out, fail(KindInvalidInput, "ORDINARY_REQUIRED_GAP", doc, nil)
	}
	if e := b.charge(uint64(len(spans.arrays["required"])), 32*uint64(len(spans.arrays["required"])), "ordinary-registration-required-index"); e != nil {
		return out, e
	}
	seenRequiredIDs := make(map[string]bool, len(spans.arrays["required"]))
	for _, span := range spans.arrays["required"] {
		raw := ordinarySpanBytes(data, span)
		var required ordinaryRequiredHeader
		if er := b.decodeOrdinaryRegistrationHeaderRow("ordinary-registration-required-index", raw, &required); er != nil {
			return out, er
		}
		c := required.Coordinate
		tuple := ordinaryRegistrationTuple{caseID: c.Case, queryID: c.Query, step: c.Step, role: c.TreeRole}
		if !expected[tuple] || seenRequiredIDs[required.ID] || !strings.HasPrefix(required.ID, "qsem1:") || len(required.ID) != len("qsem1:")+64 || !hex64.MatchString(strings.TrimPrefix(required.ID, "qsem1:")) {
			return out, fail(KindInvalidInput, "ORDINARY_REQUIRED_COORDINATE", doc, nil)
		}
		seenRequiredIDs[required.ID] = true
		delete(expected, tuple)
		if coord, selected := selectedExpected[required.ID]; selected {
			id, er := b.ordinaryCoordinateID(coord, header.Namespace)
			if er != nil {
				return out, er
			}
			if c.Case != coord.Case || c.Query != coord.Query || c.Step != coord.Step || c.TreeRole != coord.TreeRole || required.ID != id {
				return out, fail(KindInvalidInput, "ORDINARY_REQUIRED_COORDINATE", doc, nil)
			}
			var full OrdinaryRequired
			if er := b.decodeOrdinaryTyped("ordinary-registration-selected-required", raw, &full); er != nil {
				return out, er
			}
			if full.ID != id || full.Coordinate != coord || full.TreeAsset == "" || full.CaptureAsset == "" {
				return out, fail(KindInvalidInput, "ORDINARY_REQUIRED_COORDINATE", doc, nil)
			}
			out.Registration.Required = append(out.Registration.Required, full)
			delete(selectedExpected, required.ID)
		}
	}
	if len(expected) != 0 || len(selectedExpected) != 0 || len(selectedObligationIDs) != len(out.Registration.Required) || len(out.Registration.Required) == 0 {
		return out, fail(KindInvalidInput, "ORDINARY_REQUIRED_GAP", doc, nil)
	}
	return out, b.r.check()
}

func (b *eligibilityBudget) indexOrdinaryRegistration(data []byte) (ordinaryRegistrationSpans, uint64, *Error) {
	var result ordinaryRegistrationSpans
	result.root = map[string]ordinaryJSONSpan{}
	result.arrays = map[string][]ordinaryJSONSpan{}
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	var visits uint64
	if err := b.scanOrdinaryRegistrationValue(dec, 0, &visits, &result, ""); err != nil {
		return result, visits, err
	}
	if _, err := dec.ReadToken(); err != io.EOF {
		return result, visits, fail(KindInvalidInput, "ORDINARY_JSON", "ordinary-registration", err)
	}
	return result, visits, nil
}

func (b *eligibilityBudget) scanOrdinaryRegistrationValue(dec *jsontext.Decoder, depth int, visits *uint64, spans *ordinaryRegistrationSpans, rootKey string) *Error {
	const doc = "ordinary-registration-index"
	if e := b.r.check(); e != nil {
		return e
	}
	if depth > jsonMaxDepth {
		return fail(KindResourceLimit, "JSON_DEPTH_LIMIT", doc, nil)
	}
	tok, err := dec.ReadToken()
	if err != nil {
		return jsonError(doc, "", err)
	}
	*visits++
	if *visits > 2000000 {
		return fail(KindResourceLimit, "JSON_VALUE_LIMIT", doc, nil)
	}
	if e := b.charge(1, 16, doc); e != nil {
		return e
	}
	switch tok.Kind() {
	case '{':
		seen := map[string]bool{}
		for dec.PeekKind() != '}' {
			nameToken, err := dec.ReadToken()
			if err != nil {
				return jsonError(doc, "", err)
			}
			*visits++
			if e := b.charge(1, 16, doc); e != nil {
				return e
			}
			name := nameToken.String()
			if seen[name] {
				return fail(KindInvalidInput, "JSON_DUPLICATE_FIELD", doc, nil)
			}
			seen[name] = true
			valueStart := int(dec.InputOffset())
			if depth == 0 && (name == "cases" || name == "queries" || name == "required") {
				if dec.PeekKind() != '[' {
					return fail(KindInvalidInput, "ORDINARY_REGISTRATION_ARRAY", doc, nil)
				}
				_, _ = dec.ReadToken()
				*visits++
				if e := b.charge(1, 0, doc); e != nil {
					return e
				}
				for dec.PeekKind() != ']' {
					rowStart := int(dec.InputOffset())
					if e := b.scanOrdinaryRegistrationValue(dec, depth+1, visits, spans, name); e != nil {
						return e
					}
					if e := b.charge(0, 16, "ordinary-registration-index"); e != nil {
						return e
					}
					spans.arrays[name] = append(spans.arrays[name], ordinaryJSONSpan{rowStart, int(dec.InputOffset())})
				}
				_, err := dec.ReadToken()
				if err != nil {
					return jsonError(doc, "", err)
				}
				*visits++
				if e := b.charge(1, 0, doc); e != nil {
					return e
				}
			} else {
				if e := b.scanOrdinaryRegistrationValue(dec, depth+1, visits, spans, name); e != nil {
					return e
				}
			}
			if depth == 0 {
				spans.root[name] = ordinaryJSONSpan{valueStart, int(dec.InputOffset())}
			}
		}
		_, err = dec.ReadToken()
		if err != nil {
			return jsonError(doc, "", err)
		}
		*visits++
		if e := b.charge(1, 0, doc); e != nil {
			return e
		}
	case '[':
		for dec.PeekKind() != ']' {
			if e := b.scanOrdinaryRegistrationValue(dec, depth+1, visits, spans, rootKey); e != nil {
				return e
			}
		}
		_, err = dec.ReadToken()
		if err != nil {
			return jsonError(doc, "", err)
		}
		*visits++
		if e := b.charge(1, 0, doc); e != nil {
			return e
		}
	default:
		if tok.Kind() == '}' || tok.Kind() == ']' {
			return fail(KindInvalidInput, "ORDINARY_JSON", doc, nil)
		}
	}
	_ = rootKey
	return nil
}

func ordinarySpanBytes(data []byte, span ordinaryJSONSpan) []byte {
	return bytes.TrimSpace(bytes.Trim(data[span.start:span.end], " \t\r\n,:"))
}

func (b *eligibilityBudget) ordinaryRegistrationArrayLength(data []byte, field string) (int, *Error) {
	if e := b.charge(uint64(len(data)), 0, "ordinary-registration-case-index"); e != nil {
		return 0, e
	}
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	if _, err := dec.ReadToken(); err != nil {
		return 0, jsonError("ordinary-registration-case-index", "", err)
	}
	for dec.PeekKind() != '}' {
		name, err := dec.ReadToken()
		if err != nil {
			return 0, jsonError("ordinary-registration-case-index", "", err)
		}
		if name.String() == field {
			if dec.PeekKind() != '[' {
				return 0, fail(KindInvalidInput, "JSON_TYPE", "ordinary-registration-case-index", nil)
			}
			_, _ = dec.ReadToken()
			count := 0
			for dec.PeekKind() != ']' {
				if e := b.skipOrdinaryRegistrationValue(dec, 0); e != nil {
					return 0, e
				}
				count++
			}
			_, err = dec.ReadToken()
			if err != nil {
				return 0, jsonError("ordinary-registration-case-index", "", err)
			}
			return count, nil
		}
		if e := b.skipOrdinaryRegistrationValue(dec, 0); e != nil {
			return 0, e
		}
	}
	return 0, fail(KindInvalidInput, "JSON_MISSING_FIELD", "ordinary-registration-case-index", nil)
}

func (b *eligibilityBudget) skipOrdinaryRegistrationValue(dec *jsontext.Decoder, depth int) *Error {
	if depth > jsonMaxDepth {
		return fail(KindResourceLimit, "JSON_DEPTH_LIMIT", "ordinary-registration-case-index", nil)
	}
	tok, err := dec.ReadToken()
	if err != nil {
		return jsonError("ordinary-registration-case-index", "", err)
	}
	if e := b.charge(1, 0, "ordinary-registration-case-index"); e != nil {
		return e
	}
	switch tok.Kind() {
	case '{':
		seen := map[string]bool{}
		for dec.PeekKind() != '}' {
			name, err := dec.ReadToken()
			if err != nil {
				return jsonError("ordinary-registration-case-index", "", err)
			}
			if seen[name.String()] {
				return fail(KindInvalidInput, "JSON_DUPLICATE_FIELD", "ordinary-registration-case-index", nil)
			}
			seen[name.String()] = true
			if e := b.charge(1, 0, "ordinary-registration-case-index"); e != nil {
				return e
			}
			if e := b.skipOrdinaryRegistrationValue(dec, depth+1); e != nil {
				return e
			}
		}
		_, err = dec.ReadToken()
		if err != nil {
			return jsonError("ordinary-registration-case-index", "", err)
		}
		return b.charge(1, 0, "ordinary-registration-case-index")
	case '[':
		for dec.PeekKind() != ']' {
			if e := b.skipOrdinaryRegistrationValue(dec, depth+1); e != nil {
				return e
			}
		}
		_, err = dec.ReadToken()
		if err != nil {
			return jsonError("ordinary-registration-case-index", "", err)
		}
		return b.charge(1, 0, "ordinary-registration-case-index")
	default:
		return nil
	}
}

func (b *eligibilityBudget) decodeOrdinaryRegistrationHeader(data []byte, spans ordinaryRegistrationSpans) (OrdinaryRegistration, *Error) {
	var out OrdinaryRegistration
	var doc []byte
	doc = append(doc, '{')
	first := true
	for key, span := range spans.root {
		if !first {
			doc = append(doc, ',')
		}
		first = false
		quoted, _ := jsonv2.Marshal(key)
		doc = append(doc, quoted...)
		doc = append(doc, ':')
		if key == "cases" || key == "queries" || key == "required" {
			doc = append(doc, "[]"...)
		} else {
			doc = append(doc, ordinarySpanBytes(data, span)...)
		}
	}
	doc = append(doc, '}')
	if e := b.decodeOrdinaryTyped("ordinary-registration-header", doc, &out); e != nil {
		return out, e
	}
	out.Cases = []OrdinaryCase{}
	out.Queries = []OrdinaryQuery{}
	out.Required = []OrdinaryRequired{}
	if out.Schema != "tsgk-query-semantics-registration/r2" || out.Context != OrdinaryContext || out.MembershipContext != "qualification/r8" || !ordinaryClosure(out.ClosureEncoding) || !ordinaryHost(out.Platform, out.Subject) || out.Route == "" || out.Set == "" || out.Candidate == "" || out.SemanticCohort == "" || !hex64.MatchString(out.Namespace) {
		return out, fail(KindInvalidInput, "ORDINARY_REGISTRATION", "ordinary-registration-header", nil)
	}
	for _, pin := range []OrdinaryPin{out.Registry, out.Inventory, out.Slots, out.Profile, out.Assets} {
		if !ordinaryPin(pin) {
			return out, fail(KindInvalidInput, "ORDINARY_REGISTRATION_PIN", "ordinary-registration-header", nil)
		}
	}
	if err := ordinaryMembers(out.PlannedMembers, out.ClosureEncoding); err != nil {
		return out, asError(err)
	}
	return out, nil
}

func (b *eligibilityBudget) decodeOrdinaryRegistrationHeaderRow(doc string, data []byte, target any) *Error {
	if e := b.charge(uint64(len(data)), uint64(len(data)), doc); e != nil {
		return e
	}
	if err := jsonv2.Unmarshal(data, target); err != nil {
		return fail(KindInvalidInput, "JSON_SHAPE", doc, err)
	}
	return nil
}

func ordinarySelectedCoordinate(r OrdinaryRegistration, caseID, queryID string, step uint32, treeRole string) OrdinaryCoordinate {
	return OrdinaryCoordinate{Kind: queryregistrycore.Structural, Route: r.Route, Platform: r.Platform, Subject: r.Subject, Role: "primary", Set: r.Set, Case: caseID, Query: queryID, Step: step, TreeRole: treeRole, Context: r.MembershipContext}
}
