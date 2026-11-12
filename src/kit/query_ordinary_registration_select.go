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
	root       map[string]ordinaryJSONSpan
	arrays     map[string][]ordinaryRegistrationRow
	shapeError *Error
}

type ordinaryRegistrationRow struct {
	span         ordinaryJSONSpan
	id           string
	coordinate   ordinaryRegistrationTuple
	steps        uint32
	stepOverflow bool
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
	spans, e := b.scanOrdinaryRegistration(data)
	if e != nil {
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
	for i, row := range spans.arrays["cases"] {
		cID, steps := row.id, row.steps
		if cID == "" || steps == 0 || caseIDs[cID] != 0 {
			return out, fail(KindInvalidInput, "ORDINARY_CASE", doc, nil)
		}
		caseIDs[cID] = steps
		if selectedIDs[cID] {
			raw := ordinarySpanBytes(data, row.span)
			var full OrdinaryCase
			if er := b.decodeOrdinaryTyped("ordinary-registration-selected-case", raw, &full); er != nil {
				return out, er
			}
			if full.ID != cID || len(full.VersionPins) != len(full.Edits)+1 || len(full.Points) != len(full.Edits) || full.OriginalSelector == "" || full.InitialSource != full.VersionPins[0] {
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
	for _, row := range spans.arrays["queries"] {
		raw := ordinarySpanBytes(data, row.span)
		if row.id == "" || queryIDs[row.id] {
			return out, fail(KindInvalidInput, "ORDINARY_QUERY_BINDING", doc, nil)
		}
		queryIDs[row.id] = true
		var query OrdinaryQuery
		if er := b.decodeOrdinaryTyped("ordinary-registration-selected-query", raw, &query); er != nil {
			return out, er
		}
		if query.ID != row.id || !ordinaryPin(query.Source) || query.Kind != queryregistrycore.Structural || query.MetadataAsset == "" {
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
	if e := b.charge(uint64(len(spans.arrays["required"])), 96*uint64(len(spans.arrays["required"])), "ordinary-registration-required-index"); e != nil {
		return out, e
	}
	seenRequiredIDs := make(map[[64]byte]bool, len(spans.arrays["required"]))
	for _, row := range spans.arrays["required"] {
		if row.stepOverflow {
			return out, fail(KindInvalidInput, "JSON_SHAPE", "ordinary-registration-required-index", nil)
		}
		tuple := row.coordinate
		if e := b.charge(uint64(8+len(row.id)+len(tuple.caseID)+len(tuple.queryID)+len(tuple.role)), 0, "ordinary-registration-required-index"); e != nil {
			return out, e
		}
		idKey, validID := ordinaryRequiredIDKey(row.id)
		if !expected[tuple] || !validID || seenRequiredIDs[idKey] {
			return out, fail(KindInvalidInput, "ORDINARY_REQUIRED_COORDINATE", doc, nil)
		}
		seenRequiredIDs[idKey] = true
		delete(expected, tuple)
		if coord, selected := selectedExpected[row.id]; selected {
			id, er := b.ordinaryCoordinateID(coord, header.Namespace)
			if er != nil {
				return out, er
			}
			if tuple.caseID != coord.Case || tuple.queryID != coord.Query || tuple.step != coord.Step || tuple.role != coord.TreeRole || row.id != id {
				return out, fail(KindInvalidInput, "ORDINARY_REQUIRED_COORDINATE", doc, nil)
			}
			raw := ordinarySpanBytes(data, row.span)
			var full OrdinaryRequired
			if er := b.decodeOrdinaryTyped("ordinary-registration-selected-required", raw, &full); er != nil {
				return out, er
			}
			if full.ID != id || full.Coordinate != coord || full.TreeAsset == "" || full.CaptureAsset == "" {
				return out, fail(KindInvalidInput, "ORDINARY_REQUIRED_COORDINATE", doc, nil)
			}
			out.Registration.Required = append(out.Registration.Required, full)
			delete(selectedExpected, row.id)
		}
	}
	if len(expected) != 0 || len(selectedExpected) != 0 || len(selectedObligationIDs) != len(out.Registration.Required) || len(out.Registration.Required) == 0 {
		return out, fail(KindInvalidInput, "ORDINARY_REQUIRED_GAP", doc, nil)
	}
	return out, b.r.check()
}

// Fixed-width keys avoid rehashing ID strings; validation and key copying share one pass.
func ordinaryRequiredIDKey(id string) ([64]byte, bool) {
	var key [64]byte
	const prefix = "qsem1:"
	if len(id) != len(prefix)+len(key) || !strings.HasPrefix(id, prefix) {
		return key, false
	}
	for i := range key {
		c := id[len(prefix)+i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return key, false
		}
		key[i] = c
	}
	return key, true
}

// scanOrdinaryRegistration defers shape errors until syntax/depth/value checks finish.
// Cached headers are adjudicated in contract order, independently of JSON member order.
func (b *eligibilityBudget) scanOrdinaryRegistration(data []byte) (ordinaryRegistrationSpans, *Error) {
	result := ordinaryRegistrationSpans{root: map[string]ordinaryJSONSpan{}, arrays: map[string][]ordinaryRegistrationRow{}}
	const doc = "ordinary-registration"
	shape, e := b.typedJSONShapeFor(doc, reflect.TypeFor[OrdinaryRegistration](), false, true)
	if e != nil {
		return result, e
	}
	ordinaryTypedNulls(shape)
	// One streaming decoder, copied token strings and bounded error/name buffers; no typed row containers.
	if e := b.charge(1, 4*uint64(len(data))+32768, doc); e != nil {
		return result, e
	}
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	var visits uint64
	if e := b.scanOrdinaryRegistrationValue(dec, 0, &visits, shape, &result, nil, ""); e != nil {
		return result, e
	}
	if _, err := dec.ReadToken(); err != io.EOF {
		return result, fail(KindInvalidInput, "ORDINARY_JSON", doc, err)
	}
	return result, result.shapeError
}

func (b *eligibilityBudget) scanOrdinaryRegistrationValue(dec *jsontext.Decoder, depth int, visits *uint64, shape *typedJSONShape, spans *ordinaryRegistrationSpans, row *ordinaryRegistrationRow, field string) *Error {
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
	if *visits > 2000000 && !(depth == 0 && field != "") {
		return fail(KindResourceLimit, "JSON_VALUE_LIMIT", doc, nil)
	}
	if e := b.charge(1, 16, doc); e != nil {
		return e
	}
	shapeError := func(code string) {
		if spans.shapeError == nil {
			spans.shapeError = typedJSONError("ordinary-registration", code, dec)
		}
	}
	if shape != nil {
		want := byte('"')
		switch shape.kind {
		case reflect.Struct:
			want = '{'
		case reflect.Slice:
			want = '['
			if shape.base64 {
				want = '"'
			}
		case reflect.Uint, reflect.Uint32, reflect.Uint64, reflect.Int, reflect.Int64:
			want = '0'
		case reflect.Bool:
			want = 't'
			if tok.Kind() == 'f' {
				want = 'f'
			}
		}
		if byte(tok.Kind()) != want {
			code := "JSON_TYPE"
			if tok.Kind() == 'n' {
				code = "JSON_NULL"
			}
			shapeError(code)
			shape = nil
		}
	}
	switch tok.Kind() {
	case '{':
		var seen uint64
		for dec.PeekKind() != '}' {
			nameToken, err := dec.ReadToken()
			if err != nil {
				return jsonError(doc, "", err)
			}
			*visits++
			name := nameToken.String()
			var f typedJSONField
			var known bool
			if shape != nil {
				f, known = shape.fields[name]
			}
			nameLogical := uint64(16)
			if !known {
				// Unknown objects can grow the decoder's duplicate-name table without a schema bound.
				nameLogical = 256
			}
			if e := b.charge(1, nameLogical, doc); e != nil {
				return e
			}
			var child *typedJSONShape
			if shape != nil {
				if known {
					seen |= f.bit
					child = f.shape
					if f.nullable && dec.PeekKind() == 'n' {
						child = nil
					}
				} else {
					shapeError("JSON_UNKNOWN_FIELD")
				}
			}
			start := int(dec.InputOffset())
			rootArray := depth == 0 && (name == "cases" || name == "queries" || name == "required")
			childDepth := depth + 1
			if rootArray {
				if dec.PeekKind() != '[' {
					return fail(KindInvalidInput, "ORDINARY_REGISTRATION_ARRAY", doc, nil)
				}
				childDepth = depth
			}
			if e := b.scanOrdinaryRegistrationValue(dec, childDepth, visits, child, spans, row, name); e != nil {
				return e
			}
			if depth == 0 && shape != nil {
				if _, known := shape.fields[name]; known {
					spans.root[name] = ordinaryJSONSpan{start, int(dec.InputOffset())}
				}
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
		if shape != nil {
			if e := b.charge(uint64(len(shape.fields)), 0, doc); e != nil {
				return e
			}
			for name, f := range shape.fields {
				if seen&f.bit == 0 && !f.optional && spans.shapeError == nil {
					shapeError("JSON_MISSING_FIELD")
					spans.shapeError.Path += "/" + name
				}
			}
		}
	case '[':
		rootArray := depth == 0 && field != ""
		if rootArray {
			spans.arrays[field] = nil
		}
		var child *typedJSONShape
		if shape != nil {
			child = shape.elem
		}
		count := uint32(0)
		for dec.PeekKind() != ']' {
			start := int(dec.InputOffset())
			current := row
			var header ordinaryRegistrationRow
			if rootArray {
				current = &header
			}
			if e := b.scanOrdinaryRegistrationValue(dec, depth+1, visits, child, spans, current, ""); e != nil {
				return e
			}
			count++
			if rootArray {
				header.span = ordinaryJSONSpan{start, int(dec.InputOffset())}
				rows := spans.arrays[field]
				if len(rows) == cap(rows) {
					capacity := max(8, 2*cap(rows))
					if e := b.charge(uint64(len(rows))+1, uint64(capacity)*uint64(reflect.TypeFor[ordinaryRegistrationRow]().Size()), "ordinary-registration-headers"); e != nil {
						return e
					}
					grown := make([]ordinaryRegistrationRow, len(rows), capacity)
					copy(grown, rows)
					rows = grown
				}
				spans.arrays[field] = append(rows, header)
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
		if row != nil && field == "version_pins" {
			row.steps = count
		}
	default:
		if tok.Kind() == '}' || tok.Kind() == ']' {
			return fail(KindInvalidInput, "ORDINARY_JSON", doc, nil)
		}
		if shape != nil && (shape.kind == reflect.Uint || shape.kind == reflect.Uint32 || shape.kind == reflect.Uint64) {
			value, code := jsonUint(tok.String())
			if code != "" {
				shapeError(code)
			}
			if row != nil && field == "step" {
				row.coordinate.step = uint32(value)
				row.stepOverflow = value > uint64(^uint32(0))
			}
		}
		if row != nil && tok.Kind() == '"' {
			switch field {
			case "case_id", "id", "obligation_id":
				row.id = tok.String()
			case "case":
				row.coordinate.caseID = tok.String()
			case "query":
				row.coordinate.queryID = tok.String()
			case "tree_role":
				row.coordinate.role = tok.String()
			}
		}
	}
	return nil
}

func ordinarySpanBytes(data []byte, span ordinaryJSONSpan) []byte {
	return bytes.TrimSpace(bytes.Trim(data[span.start:span.end], " \t\r\n,:"))
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

func ordinarySelectedCoordinate(r OrdinaryRegistration, caseID, queryID string, step uint32, treeRole string) OrdinaryCoordinate {
	return OrdinaryCoordinate{Kind: queryregistrycore.Structural, Route: r.Route, Platform: r.Platform, Subject: r.Subject, Role: "primary", Set: r.Set, Case: caseID, Query: queryID, Step: step, TreeRole: treeRole, Context: r.MembershipContext}
}
