package kit

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"reflect"
	"strings"
)

// typedJSONShape describes fixed rows once per document, never once per row.
// Native/witness nullable and conditional arms still use their existing validators.
type typedJSONShape struct {
	kind   reflect.Kind
	elem   *typedJSONShape
	fields map[string]typedJSONField
}

type typedJSONField struct {
	shape    *typedJSONShape
	optional bool
	bit      uint64
}

func (b *eligibilityBudget) typedJSONShape(doc string, typ reflect.Type) (*typedJSONShape, *Error) {
	if e := b.charge(1, 256, doc); e != nil {
		return nil, e
	}
	s := &typedJSONShape{kind: typ.Kind()}
	switch s.kind {
	case reflect.String, reflect.Uint64:
	case reflect.Map, reflect.Slice:
		if s.kind == reflect.Map && typ.Key().Kind() != reflect.String {
			return nil, fail(KindInvalidInput, "JSON_SHAPE", doc, nil)
		}
		var e *Error
		s.elem, e = b.typedJSONShape(doc, typ.Elem())
		if e != nil {
			return nil, e
		}
	case reflect.Struct:
		// Only flat, explicitly tagged, fixed-shape rows are supported in this stage.
		if typ.NumField() > 64 {
			return nil, fail(KindInvalidInput, "JSON_SHAPE", doc, nil)
		}
		if e := b.charge(uint64(typ.NumField()), 512*uint64(typ.NumField()), doc); e != nil {
			return nil, e
		}
		s.fields = make(map[string]typedJSONField, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			name, options, _ := strings.Cut(f.Tag.Get("json"), ",")
			if f.Anonymous || !f.IsExported() || name == "" || name == "-" || options != "" && options != "omitempty" {
				return nil, fail(KindInvalidInput, "JSON_SHAPE", doc, nil)
			}
			field, e := b.typedJSONShape(doc, f.Type)
			if e != nil {
				return nil, e
			}
			s.fields[name] = typedJSONField{shape: field, optional: options == "omitempty", bit: 1 << i}
		}
	default:
		return nil, fail(KindInvalidInput, "JSON_SHAPE", doc, nil)
	}
	return s, nil
}

// decodeTyped preflights the whole syntax before shape errors, as decodeValue does.
// No jv tree is built: the shape pass retains only a depth-bounded call stack.
func (b *eligibilityBudget) decodeTyped(doc string, data []byte, target any) *Error {
	if e := b.charge(1, uint64(len(data)), doc); e != nil {
		return e
	}
	if e := b.reserveJSONContainers(doc, data, 3, 80); e != nil {
		return e
	}
	shape, e := b.typedJSONShape(doc, reflect.TypeOf(target).Elem())
	if e != nil {
		return e
	}
	dec := jsontext.NewDecoder(bytes.NewBuffer(data))
	var values uint64
	if e := b.readTypedJSON(doc, dec, shape, &values); e != nil {
		return e
	}
	if e := b.charge(values, 0, doc); e != nil {
		return e
	}
	if err := jsonv2.Unmarshal(data, target, jsonv2.RejectUnknownMembers(true)); err != nil {
		return fail(KindInvalidInput, "JSON_SHAPE", doc, err)
	}
	return nil
}

func typedJSONError(doc, code string, dec *jsontext.Decoder) *Error {
	ptr, cut := clipPath(string(dec.StackPointer()), maxFindingPath)
	if cut {
		ptr += "...(truncated)"
	}
	return fail(KindInvalidInput, code, doc+"#"+ptr, nil)
}

func (b *eligibilityBudget) readTypedJSON(doc string, dec *jsontext.Decoder, s *typedJSONShape, values *uint64) *Error {
	if e := b.charge(1, 0, doc); e != nil {
		return e
	}
	*values++
	tok, err := dec.ReadToken()
	if err != nil {
		return jsonError(doc, "", err)
	}
	kind := byte(tok.Kind())
	want := byte('"')
	switch s.kind {
	case reflect.Struct, reflect.Map:
		want = '{'
	case reflect.Slice:
		want = '['
	case reflect.Uint64:
		want = '0'
	}
	if kind != want {
		code := "JSON_TYPE"
		if kind == 'n' {
			code = "JSON_NULL"
		}
		return typedJSONError(doc, code, dec)
	}
	switch s.kind {
	case reflect.Struct, reflect.Map:
		if s.kind == reflect.Struct {
			// Keep the existing per-row container/validation allowance (1280 B for snapshots).
			if e := b.charge(0, 256+512*uint64(len(s.fields)), doc); e != nil {
				return e
			}
		}
		var seen uint64
		for dec.PeekKind() != '}' {
			if e := b.charge(1, 0, doc); e != nil {
				return e
			}
			name, err := dec.ReadToken()
			if err != nil {
				return jsonError(doc, "", err)
			}
			child := s.elem
			if s.kind == reflect.Struct {
				f, ok := s.fields[name.String()]
				if !ok {
					return typedJSONError(doc, "JSON_UNKNOWN_FIELD", dec)
				}
				seen |= f.bit
				child = f.shape
			}
			if e := b.readTypedJSON(doc, dec, child, values); e != nil {
				return e
			}
		}
		if e := b.charge(uint64(len(s.fields)), 0, doc); e != nil {
			return e
		}
		if _, err := dec.ReadToken(); err != nil {
			return jsonError(doc, "", err)
		}
		for name, f := range s.fields {
			if seen&f.bit == 0 && !f.optional {
				e := typedJSONError(doc, "JSON_MISSING_FIELD", dec)
				e.Path += "/" + name
				return e
			}
		}
	case reflect.Slice:
		for dec.PeekKind() != ']' {
			if e := b.readTypedJSON(doc, dec, s.elem, values); e != nil {
				return e
			}
		}
		if _, err := dec.ReadToken(); err != nil {
			return jsonError(doc, "", err)
		}
	case reflect.Uint64:
		if _, code := jsonUint(tok.String()); code != "" {
			return typedJSONError(doc, code, dec)
		}
	}
	return nil
}
