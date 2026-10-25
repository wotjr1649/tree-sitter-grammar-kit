package kit

import (
	"encoding/hex"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"reflect"
	"strings"
)

const eligibilityWorkLimit uint64 = 32000000
const eligibilityLogicalLimit uint64 = 1 << 30

// One ledger covers repeated work, including conservative reservations for shared leaves.
type eligibilityBudget struct {
	r             *run
	work, logical uint64
	err           *Error
}

func (b *eligibilityBudget) charge(work, logical uint64, at string) *Error {
	if b == nil {
		return nil
	}
	if e := b.r.check(); e != nil {
		return e
	}
	if work > eligibilityWorkLimit-b.work || logical > eligibilityLogicalLimit-b.logical {
		b.err = fail(KindResourceLimit, "ELIGIBILITY_WORK_LIMIT", at, nil)
		return b.err
	}
	b.work += work
	b.logical += logical
	return nil
}

func eligibilityProduct(at string, values ...uint64) (uint64, *Error) {
	n := uint64(1)
	for _, v := range values {
		if v != 0 && n > ^uint64(0)/v {
			return 0, fail(KindResourceLimit, "ELIGIBILITY_WORK_LIMIT", at, nil)
		}
		n *= v
	}
	return n, nil
}

func (b *eligibilityBudget) decode(doc string, data []byte, target any, exact bool) *Error {
	if e := b.charge(1, 2*uint64(len(data)), doc); e != nil {
		return e
	}
	values := &valueBudget{max: 2000000, check: b.r.check, charge: func() *Error { return b.charge(1, 0, doc) }}
	v, e := decodeBounded(doc, data, MaxDocumentBytes, values)
	if e != nil {
		return e
	}
	shape := &valueBudget{max: 2000000, check: b.r.check, charge: func() *Error { return b.charge(1, 0, doc) }}
	if e := eligibilityShape(typed{doc: doc}, v, reflect.TypeOf(target).Elem(), exact, shape); e != nil {
		return e
	}
	if e := b.charge(values.n, uint64(len(data)), doc); e != nil {
		return e
	}
	if err := jsonv2.Unmarshal(data, target, jsonv2.RejectUnknownMembers(exact)); err != nil {
		return fail(KindInvalidInput, "JSON_SHAPE", doc, err)
	}
	return nil
}

// Optional fields are explicit omitempty fields; null never substitutes for an absent arm.
func eligibilityShape(t typed, v *jv, typ reflect.Type, exact bool, budget *valueBudget) *Error {
	if e := budget.take(t.doc, v); e != nil {
		return e
	}
	if typ == reflect.TypeFor[jsontext.Value]() {
		return nil
	}
	switch typ {
	case reflect.TypeFor[TreeNode](), reflect.TypeFor[Capture](), reflect.TypeFor[QueryPredicateStep](), reflect.TypeFor[QueryWitnessQuery](), reflect.TypeFor[QueryWitnessMetadata](), reflect.TypeFor[QueryWitnessTuple]():
		return witnessShape(t, v, typ, exact, false, budget)
	}

	if typ.Kind() == reflect.Pointer {
		if v.kind == 'n' {
			return t.bad("JSON_SHAPE", v)
		}
		return eligibilityShape(t, v, typ.Elem(), exact, budget)
	}
	switch typ.Kind() {
	case reflect.Struct:
		if e := t.want(v, '{'); e != nil {
			return e
		}
		fields := map[string]reflect.StructField{}
		var collect func(reflect.Type) *Error
		collect = func(st reflect.Type) *Error {
			if e := budget.charge(); e != nil {
				return e
			}
			for i := 0; i < st.NumField(); i++ {
				f := st.Field(i)
				tag := f.Tag.Get("json")
				name, _, _ := strings.Cut(tag, ",")
				if strings.Contains(tag, ",inline") {
					if e := collect(f.Type); e != nil {
						return e
					}
					continue
				}
				if name != "" && name != "-" {
					fields[name] = f
				}
			}
			return nil
		}
		if e := collect(typ); e != nil {
			return e
		}
		for i, name := range v.keys {
			f, ok := fields[name]
			if !ok {
				if exact {
					return t.bad("JSON_UNKNOWN_FIELD", v.vals[i])
				}
				continue
			}
			if e := eligibilityShape(t, v.vals[i], f.Type, exact, budget); e != nil {
				return e
			}
		}
		for name, f := range fields {
			if jvMember(v, name) == nil && !strings.Contains(f.Tag.Get("json"), ",omitempty") {
				return fail(KindInvalidInput, "JSON_MISSING_FIELD", t.doc+"#"+v.cptr()+"/"+name, nil)
			}
		}
	case reflect.Map:
		if typ.Key().Kind() != reflect.String {
			return t.bad("JSON_SHAPE", v)
		}
		if e := t.want(v, '{'); e != nil {
			return e
		}
		for _, item := range v.vals {
			if e := eligibilityShape(t, item, typ.Elem(), exact, budget); e != nil {
				return e
			}
		}
	case reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			return t.want(v, '"')
		}
		if e := t.want(v, '['); e != nil {
			return e
		}
		for _, item := range v.vals {
			if e := eligibilityShape(t, item, typ.Elem(), exact, budget); e != nil {
				return e
			}
		}
	default:
		return witnessShape(t, v, typ, exact, false, budget)
	}
	return nil
}

// The existing graph/policy decoders run only after this bounded strict pass and reservation.
func (b *eligibilityBudget) reserveJSON(doc string, data []byte, passes uint64) *Error {
	if b == nil {
		return nil
	}
	if e := b.charge(1, uint64(len(data)), doc); e != nil {
		return e
	}
	values := &valueBudget{max: 2000000, check: b.r.check, charge: func() *Error { return b.charge(1, 0, doc) }}
	if _, e := decodeBounded(doc, data, MaxDocumentBytes, values); e != nil {
		return e
	}
	work, e := eligibilityProduct(doc, values.n, passes)
	if e != nil {
		return e
	}
	logical, e := eligibilityProduct(doc, uint64(len(data)), passes)
	if e != nil {
		return e
	}
	return b.charge(work, logical, doc)
}

func (b *eligibilityBudget) reserveGraph(nodes []EvidenceNode, pol evidencePolicy) *Error {
	if b == nil {
		return nil
	}
	var edges, fields uint64
	for _, n := range nodes {
		if e := b.charge(1, 0, "graph-bound"); e != nil {
			return e
		}
		edges += uint64(len(n.Refs) + len(n.Files) + len(n.Identities))
		fields += uint64(len(n.ID) + len(n.Kind) + len(n.ExecutionStatus) + len(n.EvidenceMode) + len(n.Assessment) + len(n.RecordedAssessment))
		for k, v := range n.Identities {
			fields += uint64(len(k) + len(v))
		}
		for _, f := range n.Files {
			fields += uint64(len(f.Path) + len(f.Role) + len(f.SHA256))
		}
		for _, r := range n.Refs {
			fields += uint64(len(r.Relation) + len(r.Node))
		}
	}
	n := uint64(len(nodes))
	// Includes all pair comparisons, per-root cycle walks, carry rules and owner scans.
	work, e := eligibilityProduct("graph-bound", 64, n+1, n+edges+uint64(len(pol.Required)+len(pol.CarryForward))+1)
	if e != nil {
		return e
	}
	logical, e := eligibilityProduct("graph-bound", fields+1, n+uint64(len(pol.Required)+len(pol.CarryForward))+1)
	if e != nil {
		return e
	}
	return b.charge(work, logical, "graph-bound")
}

func (b *eligibilityBudget) reserveReference(ref *instrumentedReference, versions [][]byte) *Error {
	c, p, t := uint64(len(ref.Query.Structural)), uint64(len(ref.Query.Predicates)), uint64(0)
	var text uint64
	for _, v := range versions {
		text += uint64(len(v))
	}
	for _, pred := range ref.Query.Predicates {
		if e := b.charge(1, 0, "reference-bound"); e != nil {
			return e
		}
		t += uint64(len(pred.Steps))
		for _, step := range pred.Steps {
			text += uint64(len(step.Kind) + len(step.Value))
			if step.Quantifier != nil {
				text += uint64(len(*step.Quantifier))
			}
		}
	}
	product, e := eligibilityProduct("reference-bound", c, p+t)
	if e != nil {
		return e
	}
	work, e := eligibilityProduct("reference-bound", 16, 1+c+p+t+product)
	if e != nil {
		return e
	}
	work += 128 * uint64(len(ref.Tree.Nodes)+len(ref.Query.Structural)+len(ref.Query.Evaluated)+len(ref.Query.MatchMapping)+len(ref.Query.CaptureNames)+1)
	logical, e := eligibilityProduct("reference-bound", 2, text+1, 1+c*(p+t+1))
	if e != nil {
		return e
	}
	for _, n := range ref.Tree.Nodes {
		if e := b.charge(1, 0, "reference-bound"); e != nil {
			return e
		}
		logical += 2 * uint64(len(n.Type))
		if n.Field != nil {
			logical += 2 * uint64(len(*n.Field))
		}
	}
	for _, stream := range [][]Capture{ref.Query.Structural, ref.Query.Evaluated} {
		for _, capture := range stream {
			if e := b.charge(1, 0, "reference-bound"); e != nil {
				return e
			}
			logical += 4 * uint64(len(capture.Name)+len(capture.Type))
		}
	}
	return b.charge(work, logical, "reference-bound")
}

func (s *eligibilitySession) fixedBytes(input eligibilityFixedInput) ([]byte, *Error) {
	if e := s.b.charge(1, input.Bytes*3, "catalogue-input"); e != nil {
		return nil, e
	}
	if input.Bytes > MaxDocumentBytes {
		return nil, fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", "catalogue-input", nil)
	}
	var data []byte
	var err error
	if input.RepeatByte != "" {
		v, e := hex.DecodeString(input.RepeatByte)
		if e != nil || len(v) != 1 {
			return nil, fail(KindInvalidInput, "CATALOGUE_INVALID", "catalogue-input", nil)
		}
		data = make([]byte, input.Bytes)
		for i := range data {
			data[i] = v[0]
		}
	} else {
		data, err = hex.DecodeString(input.Hex)
	}
	if err != nil || uint64(len(data)) != input.Bytes || digestHex(data) != input.SHA256 {
		return nil, fail(KindInvalidInput, "CATALOGUE_INVALID", "catalogue-input", err)
	}
	return data, nil
}
