package kit

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"reflect"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

// typedJSONShape describes fixed rows once per document, never once per row.
type typedJSONShape struct {
	typ    reflect.Type
	kind   reflect.Kind
	raw    bool
	length int
	elem   *typedJSONShape
	fields map[string]typedJSONField
}

type typedJSONField struct {
	shape    *typedJSONShape
	optional bool
	nullable bool
	bit      uint64
}

func (b *eligibilityBudget) typedJSONShape(doc string, typ reflect.Type) (*typedJSONShape, *Error) {
	if e := b.charge(1, 256, doc); e != nil {
		return nil, e
	}
	s := &typedJSONShape{typ: typ, kind: typ.Kind(), raw: typ == reflect.TypeFor[jsontext.Value]()}
	if s.raw {
		return s, nil
	}
	switch s.kind {
	case reflect.String, reflect.Bool, reflect.Uint, reflect.Uint32, reflect.Uint64, reflect.Int, reflect.Int64:
	case reflect.Map, reflect.Slice, reflect.Pointer, reflect.Array:
		if s.kind == reflect.Map && typ.Key().Kind() != reflect.String {
			return nil, fail(KindInvalidInput, "JSON_SHAPE", doc, nil)
		}
		var e *Error
		s.elem, e = b.typedJSONShape(doc, typ.Elem())
		if e != nil {
			return nil, e
		}
		if s.kind == reflect.Array {
			s.length = typ.Len()
		}
	case reflect.Struct:
		// Explicit tags keep wire names independent of Go field-name matching.
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
			optional := options == "omitempty" && typ != reflect.TypeFor[nativewire.Producer]() && typ != reflect.TypeFor[nativewire.Step]()
			s.fields[name] = typedJSONField{shape: field, optional: optional, nullable: eligibilityNativeNullable(typ, name), bit: 1 << i}
		}
	default:
		return nil, fail(KindInvalidInput, "JSON_SHAPE", doc, nil)
	}
	return s, nil
}

// decodeTyped preflights the whole syntax before shape errors, as decodeValue does.
// No jv tree is built: the shape pass retains only a depth-bounded call stack.
func (b *eligibilityBudget) decodeTyped(doc string, data []byte, target any) *Error {
	return b.decodeTypedPasses(doc, data, target, 3)
}

func (b *eligibilityBudget) decodeTypedPasses(doc string, data []byte, target any, passes uint64) *Error {
	if e := b.charge(1, uint64(len(data)), doc); e != nil {
		return e
	}
	if e := b.reserveJSONContainers(doc, data, passes, 80); e != nil {
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
	if s.raw {
		return b.skipTypedJSON(doc, dec, values)
	}
	if s.kind == reflect.Pointer {
		if dec.PeekKind() == 'n' {
			if e := b.skipTypedJSON(doc, dec, values); e != nil {
				return e
			}
			return typedJSONError(doc, "JSON_SHAPE", dec)
		}
		return b.readTypedJSON(doc, dec, s.elem, values)
	}
	if s.kind == reflect.Array && dec.PeekKind() == '[' {
		return b.readTypedJSONArray(doc, dec, s, values)
	}
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
	case reflect.Slice, reflect.Array:
		want = '['
	case reflect.Uint, reflect.Uint32, reflect.Uint64, reflect.Int, reflect.Int64:
		want = '0'
	case reflect.Bool:
		want = 't'
		if kind == 'f' {
			want = 'f'
		}
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
		var status, form string
		var haveStatus bool
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
				fieldName := name.String()
				f, ok := s.fields[fieldName]
				if !ok {
					return typedJSONError(doc, "JSON_UNKNOWN_FIELD", dec)
				}
				seen |= f.bit
				child = f.shape
				if s.typ == reflect.TypeFor[nativewire.WireTree]() && (fieldName == "status" || fieldName == "form") && dec.PeekKind() == '"' {
					value, err := dec.ReadToken()
					if err != nil {
						return jsonError(doc, "", err)
					}
					if e := b.charge(1, 0, doc); e != nil {
						return e
					}
					*values++
					if fieldName == "status" {
						status = value.String()
						haveStatus = true
					} else {
						form = value.String()
					}
					continue
				}
				if f.nullable && dec.PeekKind() == 'n' {
					if e := b.skipTypedJSON(doc, dec, values); e != nil {
						return e
					}
					continue
				}
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
			optional := f.optional
			if s.typ == reflect.TypeFor[nativewire.WireTree]() {
				if haveStatus && status != StatusCompleted {
					optional = name != "status" && name != "code" && name != "parse_ms" && name != "form" && name != "queries" && name != "api"
				} else if form == "full" {
					optional = name == "reason" || name == "errors" || name == "partial_trees"
				}
			}
			if seen&f.bit == 0 && !optional {
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
	case reflect.Uint, reflect.Uint32, reflect.Uint64:
		if _, code := jsonUint(tok.String()); code != "" {
			return typedJSONError(doc, code, dec)
		}
	case reflect.Int, reflect.Int64:
		number := strings.TrimPrefix(tok.String(), "-")
		for _, c := range number {
			if c < '0' || c > '9' {
				return typedJSONError(doc, "JSON_NUMBER_NOT_INTEGER", dec)
			}
		}
	}
	return nil
}

// Raw arms are syntax-checked by the preflight and still counted in the typed pass.
func (b *eligibilityBudget) skipTypedJSON(doc string, dec *jsontext.Decoder, values *uint64) *Error {
	if e := b.charge(1, 0, doc); e != nil {
		return e
	}
	*values++
	tok, err := dec.ReadToken()
	if err != nil {
		return jsonError(doc, "", err)
	}
	end := byte(0)
	if tok.Kind() == '{' {
		end = '}'
	} else if tok.Kind() == '[' {
		end = ']'
	}
	if end == 0 {
		return nil
	}
	for byte(dec.PeekKind()) != end {
		if end == '}' {
			if e := b.charge(1, 0, doc); e != nil {
				return e
			}
			if _, err := dec.ReadToken(); err != nil {
				return jsonError(doc, "", err)
			}
		}
		if e := b.skipTypedJSON(doc, dec, values); e != nil {
			return e
		}
	}
	_, err = dec.ReadToken()
	if err != nil {
		return jsonError(doc, "", err)
	}
	return nil
}

// The AST validator checks fixed-array length before any element shape error.
func (b *eligibilityBudget) readTypedJSONArray(doc string, dec *jsontext.Decoder, s *typedJSONShape, values *uint64) *Error {
	data, err := dec.ReadValue()
	if err != nil {
		return jsonError(doc, "", err)
	}
	if e := b.charge(1, uint64(len(data))+2048, doc); e != nil {
		return e
	}
	count := jsontext.NewDecoder(bytes.NewBuffer(data))
	_, _ = count.ReadToken()
	var length int
	var counted uint64
	for count.PeekKind() != ']' {
		if e := b.skipTypedJSON(doc, count, &counted); e != nil {
			return e
		}
		length++
	}
	if e := b.charge(counted+1, 0, doc); e != nil {
		return e
	}
	if length != s.length {
		return typedJSONError(doc, "JSON_SHAPE", dec)
	}
	inner := jsontext.NewDecoder(bytes.NewBuffer(data))
	_, _ = inner.ReadToken()
	*values++
	for i := 0; i < length; i++ {
		if e := b.readTypedJSON(doc, inner, s.elem, values); e != nil {
			if e.Kind != KindInvalidInput {
				return e
			}
			// Keep the outer document pointer for fixed-array element errors.
			base := typedJSONError(doc, e.Code, dec)
			base.Path += string(inner.StackPointer())
			return base
		}
	}
	return nil
}

// Check retains its independent 12-value/byte traversal reservation, without a second AST.
func (b *eligibilityBudget) decodeNativeResponse(doc string, data []byte, target *nativewire.Response) *Error {
	return b.decodeTypedPasses(doc, data, target, 15)
}
