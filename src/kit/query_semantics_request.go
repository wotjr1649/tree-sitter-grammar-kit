package kit

import (
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

type semanticRequestLimits struct {
	InputBytes   uint64 `json:"input_bytes"`
	Nodes        uint64 `json:"nodes"`
	FullNodes    uint64 `json:"full_nodes"`
	Depth        uint64 `json:"depth"`
	OutputBytes  uint64 `json:"output_bytes"`
	ParseMillis  uint64 `json:"parse_ms"`
	MemoryBytes  uint64 `json:"memory_bytes"`
	Errors       uint64 `json:"errors"`
	PartialNodes uint64 `json:"partial_nodes"`
	Matches      uint64 `json:"matches"`
	Captures     uint64 `json:"captures"`
	QueryMillis  uint64 `json:"query_ms"`
}

type semanticRequestEdit struct {
	StartByte  uint32 `json:"start_byte"`
	OldEndByte uint32 `json:"old_end_byte"`
	NewEndByte uint32 `json:"new_end_byte"`
	Old        string `json:"old"`
	New        string `json:"new"`
}

type semanticRequestQuery struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

type semanticRequestRange struct {
	StartByte  uint32    `json:"start_byte"`
	EndByte    uint32    `json:"end_byte"`
	StartPoint [2]uint32 `json:"start_point"`
	EndPoint   [2]uint32 `json:"end_point"`
}

// Base64 stays text until its allocation and canonicalization have been reserved.
type semanticNativeRequest struct {
	Protocol                  string                   `json:"protocol"`
	ID                        string                   `json:"id"`
	Encoding                  string                   `json:"encoding"`
	Output                    string                   `json:"output"`
	Limits                    semanticRequestLimits    `json:"limits"`
	Declarations              []NativeDeclaration      `json:"declarations"`
	Points                    []NativePoint            `json:"points"`
	Ranges                    [][]semanticRequestRange `json:"ranges"`
	Source                    string                   `json:"source"`
	Edits                     []semanticRequestEdit    `json:"edits"`
	Queries                   []semanticRequestQuery   `json:"queries"`
	API                       bool                     `json:"api"`
	ReuseObservation          string                   `json:"reuse_observation,omitempty"`
	ObservationRunID          string                   `json:"observation_run_id,omitempty"`
	ObservationProducerSHA256 string                   `json:"observation_producer_sha256,omitempty"`
	ObservationHistoryStart   uint64                   `json:"observation_history_start,omitempty"`
	source                    []byte
	edits                     []Edit
	queries                   []querywire.Source
}

func semanticSum(at string, values ...uint64) (uint64, *Error) {
	var n uint64
	for _, v := range values {
		if v > ^uint64(0)-n {
			return 0, fail(KindResourceLimit, "ELIGIBILITY_WORK_LIMIT", at, nil)
		}
		n += v
	}
	return n, nil
}

func semanticBase64Cost(at string, length uint64) (work, logical uint64, err *Error) {
	decoded := length / 4 * 3
	padded, e := semanticSum(at, decoded, 2)
	if e != nil {
		return 0, 0, e
	}
	encoded, e := eligibilityProduct(at, padded/3, 4)
	if e != nil {
		return 0, 0, e
	}
	decodedCopies, e := eligibilityProduct(at, decoded, 2)
	if e != nil {
		return 0, 0, e
	}
	encodedCopies, e := eligibilityProduct(at, encoded, 2)
	if e != nil {
		return 0, 0, e
	}
	inputScans, e := eligibilityProduct(at, length, 3)
	if e != nil {
		return 0, 0, e
	}
	logical, e = semanticSum(at, length, decodedCopies, encodedCopies)
	if e != nil {
		return 0, 0, e
	}
	work, e = semanticSum(at, 1, inputScans, decodedCopies, encoded)
	return work, logical, e
}

func (b *eligibilityBudget) semanticBase64(t typed, v *jv) ([]byte, *Error) {
	text, e := t.str(v)
	if e != nil {
		return nil, e
	}
	work, logical, e := semanticBase64Cost(t.doc, uint64(len(text)))
	if e != nil {
		return nil, e
	}
	if e := b.charge(work, logical, t.doc); e != nil {
		return nil, e
	}
	return strictBase64(t, v)
}

func (b *eligibilityBudget) nativeRequest(doc string, data []byte) (semanticNativeRequest, *Error) {
	var request semanticNativeRequest
	v, e := b.decodeValue(doc, data, &request, true)
	if e != nil {
		return request, e
	}
	t := typed{doc: doc}
	if request.Protocol != nativewire.ProtocolR2 && request.Protocol != nativewire.ProtocolR4 {
		return request, t.bad("SEMANTICS_REQUEST_PROTOCOL", jvMember(v, "protocol"))
	}
	for _, name := range []string{"reuse_observation", "observation_run_id", "observation_producer_sha256", "observation_history_start"} {
		member := jvMember(v, name)
		if request.Protocol == nativewire.ProtocolR2 && member != nil {
			return request, t.bad("SEMANTICS_REQUEST_REVISION", member)
		}
		if request.Protocol == nativewire.ProtocolR4 && member == nil {
			return request, fail(KindInvalidInput, "JSON_MISSING_FIELD", doc+"#/"+name, nil)
		}
	}
	if request.Protocol == nativewire.ProtocolR4 && (request.ReuseObservation != reuseobs.Policy || !validID(request.ObservationRunID) || !hex64.MatchString(request.ObservationProducerSHA256)) {
		return request, t.bad("SEMANTICS_REQUEST_OBSERVATION", v)
	}
	if e := b.charge(uint64(len(request.Edits)+len(request.Queries)+1), uint64(len(request.Edits))*64+uint64(len(request.Queries))*40, doc+"/decoded-inputs"); e != nil {
		return request, e
	}
	if request.source, e = b.semanticBase64(t, jvMember(v, "source")); e != nil {
		return request, e
	}
	request.edits = make([]Edit, len(request.Edits))
	for i, edit := range request.Edits {
		value := jvMember(v, "edits").vals[i]
		old, e := b.semanticBase64(t, jvMember(value, "old"))
		if e != nil {
			return request, e
		}
		next, e := b.semanticBase64(t, jvMember(value, "new"))
		if e != nil {
			return request, e
		}
		request.edits[i] = Edit{StartByte: edit.StartByte, OldEndByte: edit.OldEndByte, NewEndByte: edit.NewEndByte, Old: old, New: next}
	}
	request.queries = make([]querywire.Source, len(request.Queries))
	for i, query := range request.Queries {
		source, e := b.semanticBase64(t, jvMember(jvMember(v, "queries").vals[i], "source"))
		if e != nil {
			return request, e
		}
		request.queries[i] = querywire.Source{ID: query.ID, Source: source}
	}
	return request, nil
}

func (b *eligibilityBudget) semanticVersions(request semanticNativeRequest, maxBytes uint64) ([][]byte, []EditPoints, error) {
	upper := uint64(len(request.source))
	work := upper
	var bytes uint64
	for _, edit := range request.edits {
		if e := b.charge(1, 0, "request/versions"); e != nil {
			return nil, nil, e
		}
		// A successful next version is bounded individually, even when total growth exceeds the cap.
		next, e := semanticSum("request/versions", upper, uint64(len(edit.New)))
		if e != nil {
			return nil, nil, e
		}
		next = min(next, maxBytes)
		bytes, e = semanticSum("request/versions", bytes, next)
		if e != nil {
			return nil, nil, e
		}
		// Encoding/boundary checks and three PointAt scans may revisit the entire version.
		scanBytes, e := semanticSum("request/versions", upper, next, uint64(len(edit.Old)), uint64(len(edit.New)), 1)
		if e != nil {
			return nil, nil, e
		}
		scans, e := eligibilityProduct("request/versions", 12, scanBytes)
		if e != nil {
			return nil, nil, e
		}
		work, e = semanticSum("request/versions", work, scans)
		if e != nil {
			return nil, nil, e
		}
		upper = next
	}
	// Append may temporarily retain old and new backing arrays for versions and edit points.
	structure, e := eligibilityProduct("request/versions", uint64(len(request.edits)+1), 256)
	if e != nil {
		return nil, nil, e
	}
	logical, e := semanticSum("request/versions", bytes, structure)
	if e != nil {
		return nil, nil, e
	}
	work, e = semanticSum("request/versions", work, 1)
	if e != nil {
		return nil, nil, e
	}
	if e := b.charge(work, logical, "request/versions"); e != nil {
		return nil, nil, e
	}
	return ApplyEdits(request.Encoding, request.source, request.edits, maxBytes)
}
