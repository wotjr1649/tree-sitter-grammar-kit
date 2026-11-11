package kit

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// Frozen from c6c7423 to preserve registration error precedence during scan fusion.
type legacyOnce_ordinaryRegistrationSelection struct {
	Registration OrdinaryRegistration
	CaseIndices  []int
	TotalCases   int
	TotalQueries int
}

type legacyOnce_ordinaryJSONSpan struct{ start, end int }

type legacyOnce_ordinaryRegistrationSpans struct {
	root   map[string]legacyOnce_ordinaryJSONSpan
	arrays map[string][]legacyOnce_ordinaryJSONSpan
}

type legacyOnce_ordinaryCaseHeader struct {
	ID string `json:"case_id"`
}

type legacyOnce_ordinaryQueryHeader struct {
	ID string `json:"id"`
}

type legacyOnce_ordinaryRequiredCoordinateHeader struct {
	Case     string `json:"case"`
	Query    string `json:"query"`
	Step     uint32 `json:"step"`
	TreeRole string `json:"tree_role"`
}

type legacyOnce_ordinaryRequiredHeader struct {
	ID         string                                      `json:"obligation_id"`
	Coordinate legacyOnce_ordinaryRequiredCoordinateHeader `json:"coordinate"`
}

type legacyOnce_ordinaryRegistrationTuple struct {
	caseID  string
	queryID string
	step    uint32
	role    string
}

// legacyOnce_decodeOrdinaryRegistrationSelection validates the complete registration token stream and
// its global ID union, then materializes only selected cases and required rows.
func (b *eligibilityBudget) legacyOnce_decodeOrdinaryRegistrationSelection(data []byte, selected []OrdinaryPlanShard) (legacyOnce_ordinaryRegistrationSelection, *Error) {
	var out legacyOnce_ordinaryRegistrationSelection
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
	spans, visits, e := b.legacyOnce_indexOrdinaryRegistration(data)
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
	header, e := b.legacyOnce_decodeOrdinaryRegistrationHeader(data, spans)
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
		raw := legacyOnce_ordinarySpanBytes(data, span)
		var c legacyOnce_ordinaryCaseHeader
		if er := b.legacyOnce_decodeOrdinaryRegistrationHeaderRow("ordinary-registration-case-index", raw, &c); er != nil {
			return out, er
		}
		steps, er := b.legacyOnce_ordinaryRegistrationArrayLength(raw, "version_pins")
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
		raw := legacyOnce_ordinarySpanBytes(data, span)
		var header legacyOnce_ordinaryQueryHeader
		if er := b.legacyOnce_decodeOrdinaryRegistrationHeaderRow("ordinary-registration-query-index", raw, &header); er != nil {
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
	expected := make(map[legacyOnce_ordinaryRegistrationTuple]bool)
	selectedExpected := make(map[string]OrdinaryCoordinate, len(selectedObligationIDs))
	for caseID, steps := range caseIDs {
		for _, q := range out.Registration.Queries {
			for step := uint32(0); step < steps; step++ {
				for _, treeRole := range []string{"incremental", "fresh"} {
					if step == 0 && treeRole == "fresh" {
						continue
					}
					tuple := legacyOnce_ordinaryRegistrationTuple{caseID: caseID, queryID: q.ID, step: step, role: treeRole}
					if expected[tuple] {
						return out, fail(KindInvalidInput, "ORDINARY_REQUIRED_COORDINATE", doc, nil)
					}
					expected[tuple] = true
					if selectedIDs[caseID] {
						coord := legacyOnce_ordinarySelectedCoordinate(header, caseID, q.ID, step, treeRole)
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
		raw := legacyOnce_ordinarySpanBytes(data, span)
		var required legacyOnce_ordinaryRequiredHeader
		if er := b.legacyOnce_decodeOrdinaryRegistrationHeaderRow("ordinary-registration-required-index", raw, &required); er != nil {
			return out, er
		}
		c := required.Coordinate
		tuple := legacyOnce_ordinaryRegistrationTuple{caseID: c.Case, queryID: c.Query, step: c.Step, role: c.TreeRole}
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

func (b *eligibilityBudget) legacyOnce_indexOrdinaryRegistration(data []byte) (legacyOnce_ordinaryRegistrationSpans, uint64, *Error) {
	var result legacyOnce_ordinaryRegistrationSpans
	result.root = map[string]legacyOnce_ordinaryJSONSpan{}
	result.arrays = map[string][]legacyOnce_ordinaryJSONSpan{}
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	var visits uint64
	if err := b.legacyOnce_scanOrdinaryRegistrationValue(dec, 0, &visits, &result, ""); err != nil {
		return result, visits, err
	}
	if _, err := dec.ReadToken(); err != io.EOF {
		return result, visits, fail(KindInvalidInput, "ORDINARY_JSON", "ordinary-registration", err)
	}
	return result, visits, nil
}

func (b *eligibilityBudget) legacyOnce_scanOrdinaryRegistrationValue(dec *jsontext.Decoder, depth int, visits *uint64, spans *legacyOnce_ordinaryRegistrationSpans, rootKey string) *Error {
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
					if e := b.legacyOnce_scanOrdinaryRegistrationValue(dec, depth+1, visits, spans, name); e != nil {
						return e
					}
					if e := b.charge(0, 16, "ordinary-registration-index"); e != nil {
						return e
					}
					spans.arrays[name] = append(spans.arrays[name], legacyOnce_ordinaryJSONSpan{rowStart, int(dec.InputOffset())})
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
				if e := b.legacyOnce_scanOrdinaryRegistrationValue(dec, depth+1, visits, spans, name); e != nil {
					return e
				}
			}
			if depth == 0 {
				spans.root[name] = legacyOnce_ordinaryJSONSpan{valueStart, int(dec.InputOffset())}
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
			if e := b.legacyOnce_scanOrdinaryRegistrationValue(dec, depth+1, visits, spans, rootKey); e != nil {
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

func legacyOnce_ordinarySpanBytes(data []byte, span legacyOnce_ordinaryJSONSpan) []byte {
	return bytes.TrimSpace(bytes.Trim(data[span.start:span.end], " \t\r\n,:"))
}

func (b *eligibilityBudget) legacyOnce_ordinaryRegistrationArrayLength(data []byte, field string) (int, *Error) {
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
				if e := b.legacyOnce_skipOrdinaryRegistrationValue(dec, 0); e != nil {
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
		if e := b.legacyOnce_skipOrdinaryRegistrationValue(dec, 0); e != nil {
			return 0, e
		}
	}
	return 0, fail(KindInvalidInput, "JSON_MISSING_FIELD", "ordinary-registration-case-index", nil)
}

func (b *eligibilityBudget) legacyOnce_skipOrdinaryRegistrationValue(dec *jsontext.Decoder, depth int) *Error {
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
			if e := b.legacyOnce_skipOrdinaryRegistrationValue(dec, depth+1); e != nil {
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
			if e := b.legacyOnce_skipOrdinaryRegistrationValue(dec, depth+1); e != nil {
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

func (b *eligibilityBudget) legacyOnce_decodeOrdinaryRegistrationHeader(data []byte, spans legacyOnce_ordinaryRegistrationSpans) (OrdinaryRegistration, *Error) {
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
			doc = append(doc, legacyOnce_ordinarySpanBytes(data, span)...)
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

func (b *eligibilityBudget) legacyOnce_decodeOrdinaryRegistrationHeaderRow(doc string, data []byte, target any) *Error {
	if e := b.charge(uint64(len(data)), uint64(len(data)), doc); e != nil {
		return e
	}
	if err := jsonv2.Unmarshal(data, target); err != nil {
		return fail(KindInvalidInput, "JSON_SHAPE", doc, err)
	}
	return nil
}

func legacyOnce_ordinarySelectedCoordinate(r OrdinaryRegistration, caseID, queryID string, step uint32, treeRole string) OrdinaryCoordinate {
	return OrdinaryCoordinate{Kind: queryregistrycore.Structural, Route: r.Route, Platform: r.Platform, Subject: r.Subject, Role: "primary", Set: r.Set, Case: caseID, Query: queryID, Step: step, TreeRole: treeRole, Context: r.MembershipContext}
}

// ordinaryRegistrationSelectionLegacyOnceSnapshot is copied from HEAD so the
// differential oracle retains the pre-optimization traversal and error order.
func ordinaryRegistrationOnceFixture() (OrdinaryRegistration, []OrdinaryPlanShard) {
	r := OrdinaryRegistration{
		Schema:    "tsgk-query-semantics-registration/r2",
		Registry:  OrdinaryPin{Path: ordinaryRegistryPath, Bytes: ordinaryRegistryBytes, SHA256: ordinaryRegistrySHA},
		Namespace: strings.Repeat("a", 64),
		Inventory: OrdinaryPin{Path: "contracts/inventory.json", Bytes: 1, SHA256: strings.Repeat("b", 64)},
		Slots:     OrdinaryPin{Path: "contracts/slots.json", Bytes: 1, SHA256: strings.Repeat("c", 64)},
		Profile:   OrdinaryPin{Path: "profile/ordinary.json", Bytes: 1, SHA256: strings.Repeat("d", 64)},
		Assets:    OrdinaryPin{Path: "profile/assets.json", Bytes: 1, SHA256: strings.Repeat("e", 64)},
		Route:     "json", Set: "set_json", Platform: "windows-amd64", Subject: "windows/amd64",
		MembershipContext: "qualification/r8", Context: OrdinaryContext, ClosureEncoding: "plain",
		Candidate: "candidate", SemanticCohort: "cohort",
		Cases: []OrdinaryCase{}, Queries: []OrdinaryQuery{}, Required: []OrdinaryRequired{}, PlannedMembers: []OrdinaryMember{},
	}
	shaByCase := []string{strings.Repeat("f", 64), strings.Repeat("9", 64), strings.Repeat("8", 64)}
	caseIDs := []string{"case-selected", "case-other", "case-third"}
	for i, caseID := range caseIDs {
		pin := OrdinaryPin{Path: "source-witness/" + shaByCase[i] + ".utf8", Bytes: 1, SHA256: shaByCase[i]}
		r.Cases = append(r.Cases, OrdinaryCase{
			ID: caseID, OriginalSelector: "qualification_inventory#/routes/0/workload/cases/0",
			InitialSource: pin, Edits: []Edit{}, VersionPins: []OrdinaryPin{pin}, Points: []EditPoints{},
		})
	}
	for i, queryID := range []string{"q1", "q2"} {
		r.Queries = append(r.Queries, OrdinaryQuery{
			ID:            queryID,
			Source:        OrdinaryPin{Path: "queries/" + queryID + ".scm", Bytes: 1, SHA256: strings.Repeat(string(rune('1'+i)), 64)},
			MetadataAsset: "metadata-" + queryID, Kind: "STRUCTURAL_ONLY",
		})
	}
	var selected OrdinaryPlanShard
	for caseIndex, c := range r.Cases {
		for _, q := range r.Queries {
			coord := OrdinaryCoordinate{
				Kind: "STRUCTURAL_ONLY", Route: r.Route, Platform: r.Platform, Subject: r.Subject,
				Role: "primary", Set: r.Set, Case: c.ID, Query: q.ID, Step: 0,
				TreeRole: "incremental", Context: r.MembershipContext,
			}
			id := coord.ID(r.Namespace)
			r.Required = append(r.Required, OrdinaryRequired{ID: id, Coordinate: coord, TreeAsset: "tree", CaptureAsset: "captures"})
			if caseIndex == 0 {
				selected.ObligationIDs = append(selected.ObligationIDs, id)
			}
		}
	}
	selected.Route, selected.Set, selected.Platform, selected.Subject, selected.Case = r.Route, r.Set, r.Platform, r.Subject, r.Cases[0].ID
	selected.SourceHistory = []OrdinaryPlanHistoryStep{{SHA256: shaByCase[0], Bytes: 1}}
	return r, []OrdinaryPlanShard{selected}
}

func ordinaryRegistrationOnceCurrentFromLegacy(old legacyOnce_ordinaryRegistrationSelection) ordinaryRegistrationSelection {
	return ordinaryRegistrationSelection{
		Registration: old.Registration, CaseIndices: old.CaseIndices,
		TotalCases: old.TotalCases, TotalQueries: old.TotalQueries,
	}
}

func compareOrdinaryRegistrationOnce(t *testing.T, data []byte, selected []OrdinaryPlanShard) (ordinaryRegistrationSelection, *Error) {
	t.Helper()
	old, oldErr := ordinaryTestBudget(t).legacyOnce_decodeOrdinaryRegistrationSelection(data, selected)
	got, gotErr := ordinaryTestBudget(t).decodeOrdinaryRegistrationSelection(data, selected)
	if (oldErr == nil) != (gotErr == nil) {
		t.Fatalf("legacy error=%v; current error=%v", oldErr, gotErr)
	}
	if oldErr != nil {
		if oldErr.Kind != gotErr.Kind || oldErr.Code != gotErr.Code || oldErr.Path != gotErr.Path {
			t.Fatalf("legacy error=(%q,%q,%q); current error=(%q,%q,%q)", oldErr.Kind, oldErr.Code, oldErr.Path, gotErr.Kind, gotErr.Code, gotErr.Path)
		}
		return ordinaryRegistrationSelection{}, gotErr
	}
	want := ordinaryRegistrationOnceCurrentFromLegacy(old)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("legacy result and current result differ:\nlegacy=%#v\ncurrent=%#v", want, got)
	}
	return got, nil
}

func ordinaryRegistrationOnceSetRowField(t *testing.T, data []byte, array string, index int, field string, value json.RawMessage) []byte {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(root[array], &rows); err != nil {
		t.Fatal(err)
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(rows[index], &row); err != nil {
		t.Fatal(err)
	}
	row[field] = append(json.RawMessage(nil), value...)
	rows[index], _ = json.Marshal(row)
	root[array], _ = json.Marshal(rows)
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func ordinaryRegistrationOnceDeleteRowField(t *testing.T, data []byte, array string, index int, field string) []byte {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(root[array], &rows); err != nil {
		t.Fatal(err)
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(rows[index], &row); err != nil {
		t.Fatal(err)
	}
	delete(row, field)
	rows[index], _ = json.Marshal(row)
	root[array], _ = json.Marshal(rows)
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func ordinaryRegistrationOnceSetCoordinateStep(t *testing.T, data []byte, index int, value json.RawMessage) []byte {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(root["required"], &rows); err != nil {
		t.Fatal(err)
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(rows[index], &row); err != nil {
		t.Fatal(err)
	}
	var coordinate map[string]json.RawMessage
	if err := json.Unmarshal(row["coordinate"], &coordinate); err != nil {
		t.Fatal(err)
	}
	coordinate["step"] = append(json.RawMessage(nil), value...)
	row["coordinate"], _ = json.Marshal(coordinate)
	rows[index], _ = json.Marshal(row)
	root["required"], _ = json.Marshal(rows)
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func ordinaryRegistrationOnceSetRequiredCoordinate(t *testing.T, data []byte, index, from int) []byte {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(root["required"], &rows); err != nil {
		t.Fatal(err)
	}
	var source, target map[string]json.RawMessage
	if err := json.Unmarshal(rows[from], &source); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rows[index], &target); err != nil {
		t.Fatal(err)
	}
	target["coordinate"] = source["coordinate"]
	rows[index], _ = json.Marshal(target)
	root["required"], _ = json.Marshal(rows)
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func ordinaryRegistrationOnceSetCaseID(t *testing.T, data []byte, index int, value json.RawMessage) []byte {
	return ordinaryRegistrationOnceSetRowField(t, data, "cases", index, "case_id", value)
}

func ordinaryRegistrationOnceSetRootField(t *testing.T, data []byte, field string, value json.RawMessage) []byte {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	root[field] = append(json.RawMessage(nil), value...)
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func ordinaryRegistrationOnceReorderObject(t *testing.T, raw []byte, preferred []string) []byte {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	ordered := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, key := range preferred {
		if _, ok := fields[key]; ok && !seen[key] {
			ordered = append(ordered, key)
			seen[key] = true
		}
	}
	var rest []string
	for key := range fields {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	ordered = append(ordered, rest...)
	var out bytes.Buffer
	out.WriteByte('{')
	for i, key := range ordered {
		if i != 0 {
			out.WriteByte(',')
		}
		name, _ := json.Marshal(key)
		out.Write(name)
		out.WriteByte(':')
		out.Write(fields[key])
	}
	out.WriteByte('}')
	return out.Bytes()
}

func ordinaryRegistrationOnceReorderRows(t *testing.T, data []byte, array string, indices map[int][]string) []byte {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(root[array], &rows); err != nil {
		t.Fatal(err)
	}
	for i, preferred := range indices {
		rows[i] = ordinaryRegistrationOnceReorderObject(t, rows[i], preferred)
	}
	root[array], _ = json.Marshal(rows)
	return ordinaryRegistrationOnceReorderObject(t, mustMarshalOrdinaryRegistrationOnce(t, root), []string{"required", "queries", "cases", "context", "schema"})
}

func mustMarshalOrdinaryRegistrationOnce(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func ordinaryRegistrationOnceSetUnselectedPinOverflow(t *testing.T, data []byte, caseIndex int) []byte {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(root["cases"], &rows); err != nil {
		t.Fatal(err)
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(rows[caseIndex], &row); err != nil {
		t.Fatal(err)
	}
	var pins []json.RawMessage
	if err := json.Unmarshal(row["version_pins"], &pins); err != nil {
		t.Fatal(err)
	}
	var pin map[string]json.RawMessage
	if err := json.Unmarshal(pins[0], &pin); err != nil {
		t.Fatal(err)
	}
	pin["bytes"] = json.RawMessage("18446744073709551616")
	pins[0], _ = json.Marshal(pin)
	row["version_pins"], _ = json.Marshal(pins)
	rows[caseIndex], _ = json.Marshal(row)
	root["cases"], _ = json.Marshal(rows)
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func ordinaryRegistrationOnceSetSelectedCaseSourcePath(t *testing.T, data []byte, caseIndex int, path string) []byte {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(root["cases"], &rows); err != nil {
		t.Fatal(err)
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(rows[caseIndex], &row); err != nil {
		t.Fatal(err)
	}
	var initial OrdinaryPin
	if err := json.Unmarshal(row["initial_source_pin"], &initial); err != nil {
		t.Fatal(err)
	}
	initial.Path = path
	row["initial_source_pin"], _ = json.Marshal(initial)
	var pins []OrdinaryPin
	if err := json.Unmarshal(row["version_pins"], &pins); err != nil {
		t.Fatal(err)
	}
	pins[0].Path = path
	row["version_pins"], _ = json.Marshal(pins)
	rows[caseIndex], _ = json.Marshal(row)
	root["cases"], _ = json.Marshal(rows)
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOrdinaryRegistrationOnceDifferentialValidEncodings(t *testing.T) {
	registration, selected := ordinaryRegistrationOnceFixture()
	data := ordinaryJSON(t, registration)
	data = bytes.Replace(data, []byte("\"case_id\""), []byte("\"case_\\u0069d\""), 1)
	if !bytes.Contains(data, []byte("\"case_\\u0069d\"")) {
		t.Fatal("case member name was not escaped")
	}
	data = ordinaryRegistrationOnceReorderRows(t, data, "cases", map[int][]string{0: {"version_pins", "case_id", "edits"}, 2: {"initial_source_pin"}})
	data = ordinaryRegistrationOnceReorderRows(t, data, "queries", map[int][]string{1: {"kind", "id", "source_pin"}})
	data = ordinaryRegistrationOnceReorderRows(t, data, "required", map[int][]string{4: {"coordinate", "obligation_id"}})
	escapedCandidate := "\"candidate-" + string(rune(92)) + "u96ea\""
	data = ordinaryRegistrationOnceSetRootField(t, data, "candidate", json.RawMessage(escapedCandidate))
	if _, e := compareOrdinaryRegistrationOnce(t, data, selected); e != nil {
		t.Fatal(e)
	}

	nullRegistration := ordinaryJSON(t, registration)
	nullRegistration = ordinaryRegistrationOnceSetRowField(t, nullRegistration, "cases", 0, "edits", json.RawMessage("null"))
	nullRegistration = ordinaryRegistrationOnceSetRowField(t, nullRegistration, "cases", 0, "edit_points", json.RawMessage("null"))
	if _, e := compareOrdinaryRegistrationOnce(t, nullRegistration, selected); e != nil {
		t.Fatal(e)
	}
}

func TestOrdinaryRegistrationOnceDifferentialRejectsGlobalAndUnselectedMutations(t *testing.T) {
	registration, selected := ordinaryRegistrationOnceFixture()
	valid := ordinaryJSON(t, registration)
	cases := []struct {
		name string
		data []byte
	}{
		{"unknown-root-field", ordinaryRegistrationOnceSetRootField(t, valid, "rogue", json.RawMessage("true"))},
		{"duplicate-known-field", bytes.Replace(valid, []byte("\"case_id\":\"case-other\""), []byte("\"case_id\":\"case-other\",\"case_id\":\"case-other\""), 1)},
		{"duplicate-nonselected-case-id", ordinaryRegistrationOnceSetCaseID(t, valid, 2, json.RawMessage("\"case-other\""))},
		{"duplicate-query-id", ordinaryRegistrationOnceSetRowField(t, valid, "queries", 1, "id", json.RawMessage("\"q1\""))},
		{"duplicate-unselected-required-id", ordinaryRegistrationOnceSetRowField(t, valid, "required", 3, "obligation_id", json.RawMessage("\""+registration.Required[2].ID+"\""))},
		{"duplicate-unselected-required-coordinate", ordinaryRegistrationOnceSetRequiredCoordinate(t, valid, 3, 2)},
		{"bad-unselected-required-id", ordinaryRegistrationOnceSetRowField(t, valid, "required", 2, "obligation_id", json.RawMessage("\"bad-id\""))},
		{"unknown-unselected-case-field", ordinaryRegistrationOnceSetRowField(t, valid, "cases", 2, "rogue", json.RawMessage("true"))},
		{"missing-unselected-case-field", ordinaryRegistrationOnceDeleteRowField(t, valid, "cases", 2, "original_selector")},
		{"wrong-unselected-case-field-type", ordinaryRegistrationOnceSetCaseID(t, valid, 2, json.RawMessage("17"))},
		{"null-unselected-version-pins", ordinaryRegistrationOnceSetRowField(t, valid, "cases", 2, "version_pins", json.RawMessage("null"))},
		{"overflow-unselected-version-pin-bytes", ordinaryRegistrationOnceSetUnselectedPinOverflow(t, valid, 2)},
		{"unknown-unselected-query-field", ordinaryRegistrationOnceSetRowField(t, valid, "queries", 1, "rogue", json.RawMessage("true"))},
		{"missing-unselected-query-field", ordinaryRegistrationOnceDeleteRowField(t, valid, "queries", 1, "metadata_asset")},
		{"wrong-unselected-query-field-type", ordinaryRegistrationOnceSetRowField(t, valid, "queries", 1, "source_pin", json.RawMessage("\"bad\""))},
		{"unknown-unselected-required-field", ordinaryRegistrationOnceSetRowField(t, valid, "required", 2, "rogue", json.RawMessage("true"))},
		{"missing-unselected-required-field", ordinaryRegistrationOnceDeleteRowField(t, valid, "required", 2, "tree_asset")},
		{"selected-case-source-path", ordinaryRegistrationOnceSetSelectedCaseSourcePath(t, valid, 0, "other/"+registration.Cases[0].VersionPins[0].SHA256+".utf8")},
		{"selected-required-tree-asset-empty", ordinaryRegistrationOnceSetRowField(t, valid, "required", 0, "tree_asset", json.RawMessage("\"\""))},
		{"wrong-required-step-negative", ordinaryRegistrationOnceSetCoordinateStep(t, valid, 2, json.RawMessage("-1"))},
		{"wrong-required-step-fraction", ordinaryRegistrationOnceSetCoordinateStep(t, valid, 2, json.RawMessage("0.5"))},
		{"required-step-uint32-overflow", ordinaryRegistrationOnceSetCoordinateStep(t, valid, 2, json.RawMessage("4294967296"))},
		{"required-step-string", ordinaryRegistrationOnceSetCoordinateStep(t, valid, 2, json.RawMessage("\"0\""))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, e := compareOrdinaryRegistrationOnce(t, tc.data, selected)
			if e == nil {
				t.Fatal("accepted malformed or globally inconsistent registration")
			}
		})
	}
}

func TestOrdinaryRegistrationOnceDifferentialErrorPrecedence(t *testing.T) {
	registration, selected := ordinaryRegistrationOnceFixture()
	valid := ordinaryJSON(t, registration)
	earlyBadCase := ordinaryRegistrationOnceSetCaseID(t, valid, 0, json.RawMessage("\"\""))
	laterBadRequiredShape := ordinaryRegistrationOnceSetRowField(t, earlyBadCase, "required", len(registration.Required)-1, "rogue", json.RawMessage("true"))
	_, e := compareOrdinaryRegistrationOnce(t, laterBadRequiredShape, selected)
	if e == nil || e.Code != "JSON_UNKNOWN_FIELD" {
		t.Fatalf("later strict shape error did not retain priority over earlier case semantics: %v", e)
	}

	earlyBadShape := ordinaryRegistrationOnceSetRowField(t, valid, "cases", 0, "rogue", json.RawMessage("true"))
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"shape-before-late-json", append(append([]byte(nil), earlyBadShape...), []byte(" true")...)},
		{"semantic-before-late-json", append(append([]byte(nil), earlyBadCase...), []byte(" true")...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, e := compareOrdinaryRegistrationOnce(t, tc.data, selected)
			if e == nil || e.Code != "ORDINARY_JSON" {
				t.Fatalf("late JSON error did not retain priority: %v", e)
			}
		})
	}
}

func TestOrdinaryRegistrationOnceDuplicateUnselectedIDKillsGlobalUniquenessMutant(t *testing.T) {
	registration, selected := ordinaryRegistrationOnceFixture()
	data := ordinaryJSON(t, registration)
	data = ordinaryRegistrationOnceSetRowField(t, data, "required", 3, "obligation_id", json.RawMessage("\""+registration.Required[2].ID+"\""))
	if _, e := ordinaryTestBudget(t).decodeOrdinaryRegistrationSelection(data, selected); e == nil || e.Code != "ORDINARY_REQUIRED_COORDINATE" {
		t.Fatalf("duplicate IDs on two unselected required rows were not rejected: %v", e)
	}
}
