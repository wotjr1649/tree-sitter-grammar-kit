package kit

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"reflect"
	"slices"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

// typedJSONShape describes fixed rows once per document, never once per row.
type typedJSONShape struct {
	typ        reflect.Type
	kind       reflect.Kind
	raw        bool
	witness    bool
	exact      bool
	base64     bool
	shapeTakes uint64
	length     int
	elem       *typedJSONShape
	fields     map[string]typedJSONField
}

type typedJSONField struct {
	shape    *typedJSONShape
	optional bool
	nullable bool
	bit      uint64
}

func (b *eligibilityBudget) typedJSONShape(doc string, typ reflect.Type) (*typedJSONShape, *Error) {
	return b.typedJSONShapeFor(doc, typ, false, true)
}

func (b *eligibilityBudget) typedJSONShapeFor(doc string, typ reflect.Type, witness, exact bool) (*typedJSONShape, *Error) {
	if e := b.charge(1, 256, doc); e != nil {
		return nil, e
	}
	shapeTakes := uint64(1)
	if !witness && typ.Kind() != reflect.Struct && typ.Kind() != reflect.Map && typ.Kind() != reflect.Slice && typ.Kind() != reflect.Pointer {
		shapeTakes++
	}
	switch typ {
	case reflect.TypeFor[TreeNode](), reflect.TypeFor[Capture](), reflect.TypeFor[QueryPredicateStep](), reflect.TypeFor[QueryWitnessQuery](), reflect.TypeFor[QueryWitnessMetadata](), reflect.TypeFor[QueryWitnessTuple]():
		if !witness {
			shapeTakes++
		}
		witness = true
	}
	s := &typedJSONShape{typ: typ, kind: typ.Kind(), raw: typ == reflect.TypeFor[jsontext.Value](), witness: witness, exact: exact, shapeTakes: shapeTakes}
	if s.raw {
		return s, nil
	}
	switch s.kind {
	case reflect.String, reflect.Bool, reflect.Uint, reflect.Uint32, reflect.Uint64, reflect.Int, reflect.Int64:
	case reflect.Map, reflect.Slice, reflect.Pointer, reflect.Array:
		if s.kind == reflect.Slice && typ.Elem().Kind() == reflect.Uint8 {
			s.base64 = true
			return s, nil
		}
		if s.kind == reflect.Map && typ.Key().Kind() != reflect.String {
			return nil, fail(KindInvalidInput, "JSON_SHAPE", doc, nil)
		}
		var e *Error
		s.elem, e = b.typedJSONShapeFor(doc, typ.Elem(), witness || s.kind == reflect.Array, exact)
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
			if !witness && (options == "inline" || f.Anonymous && f.Tag.Get("json") == "" && f.Type.Kind() == reflect.Struct) {
				embedded, e := b.typedJSONShapeFor(doc, f.Type, false, exact)
				if e != nil {
					return nil, e
				}
				for name, field := range embedded.fields {
					field.bit = 1 << len(s.fields)
					s.fields[name] = field
				}
				continue
			}
			if f.Anonymous || !f.IsExported() || name == "" || name == "-" || options != "" && options != "omitempty" {
				return nil, fail(KindInvalidInput, "JSON_SHAPE", doc, nil)
			}
			field, e := b.typedJSONShapeFor(doc, f.Type, witness, exact)
			if e != nil {
				return nil, e
			}
			optional := options == "omitempty" && typ != reflect.TypeFor[nativewire.Producer]() && typ != reflect.TypeFor[nativewire.Step]()
			nullable := eligibilityNativeNullable(typ, name)
			if witness {
				optional = name == "encoding_source" || typ == reflect.TypeFor[NativeOperation]() && options == "omitempty"
				nullable = f.Type.Kind() == reflect.Slice && slices.Contains([]string{"structural", "expected", "captures", "capture_names", "predicates", "types", "fields", "nodes"}, name) && typ != reflect.TypeFor[QueryWitnessMetadata]() && typ != reflect.TypeFor[QueryWitnessRegistration]()
			}
			s.fields[name] = typedJSONField{shape: field, optional: optional, nullable: nullable, bit: 1 << len(s.fields)}
		}
		if len(s.fields) > 64 {
			return nil, fail(KindInvalidInput, "JSON_SHAPE", doc, nil)
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
	return b.decodeTypedDocument(doc, data, target, passes, false, true, false)
}

func (b *eligibilityBudget) decodeWitnessTyped(doc string, data []byte, target any, exact bool) *Error {
	return b.decodeTypedDocument(doc, data, target, 3, b.unmetered, exact, true)
}

func (b *eligibilityBudget) decodeTypedDocument(doc string, data []byte, target any, passes uint64, witness, exact, legacyShapeLimit bool) *Error {
	if e := b.charge(1, uint64(len(data)), doc); e != nil {
		return e
	}
	if e := b.reserveJSONContainers(doc, data, passes, 80); e != nil {
		return e
	}
	return b.decodeTypedShape(doc, data, target, witness, exact, legacyShapeLimit)
}

func (b *eligibilityBudget) decodeTypedShape(doc string, data []byte, target any, witness, exact, legacyShapeLimit bool) *Error {
	shape, e := b.typedJSONShapeFor(doc, reflect.TypeOf(target).Elem(), witness, exact)
	if e != nil {
		return e
	}
	dec := jsontext.NewDecoder(bytes.NewBuffer(data))
	var values uint64
	var visits *uint64
	if legacyShapeLimit {
		visits = new(uint64)
	}
	if e := b.readTypedJSONChecked(doc, dec, shape, &values, visits); e != nil {
		return e
	}
	if e := b.charge(values, 0, doc); e != nil {
		return e
	}
	if err := jsonv2.Unmarshal(data, target, jsonv2.RejectUnknownMembers(exact)); err != nil {
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
	return b.readTypedJSONChecked(doc, dec, s, values, nil)
}

// Preserve the legacy shape-value cap, including pointer and validator handoffs.
// These virtual visits do not charge work for AST traversals that no longer run.
func (b *eligibilityBudget) takeTypedShape(doc string, dec *jsontext.Decoder, s *typedJSONShape, visits *uint64, scalar bool) *Error {
	if visits == nil {
		return nil
	}
	if !b.unmetered {
		if e := b.r.check(); e != nil {
			return e
		}
	}
	*visits += s.shapeTakes
	if *visits > 2000000 {
		e := typedJSONError(doc, "JSON_VALUE_LIMIT", dec)
		e.Kind = KindResourceLimit
		return e
	}
	if scalar && s.kind == reflect.Pointer {
		return b.takeTypedShape(doc, dec, s.elem, visits, true)
	}
	return nil
}

func (b *eligibilityBudget) readTypedJSONChecked(doc string, dec *jsontext.Decoder, s *typedJSONShape, values, visits *uint64) *Error {
	if e := b.takeTypedShape(doc, dec, s, visits, false); e != nil {
		return e
	}
	if s.raw {
		return b.skipTypedJSON(doc, dec, values)
	}
	if s.kind == reflect.Pointer {
		if dec.PeekKind() == 'n' {
			if e := b.skipTypedJSON(doc, dec, values); e != nil {
				return e
			}
			if s.witness {
				return nil
			}
			return typedJSONError(doc, "JSON_SHAPE", dec)
		}
		return b.readTypedJSONChecked(doc, dec, s.elem, values, visits)
	}
	if s.kind == reflect.Array && dec.PeekKind() == '[' {
		return b.readTypedJSONArray(doc, dec, s, values, visits)
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
		if s.base64 {
			want = '"'
		}
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
					if s.exact {
						return typedJSONError(doc, "JSON_UNKNOWN_FIELD", dec)
					}
					if e := b.skipTypedJSON(doc, dec, values); e != nil {
						return e
					}
					continue
				}
				seen |= f.bit
				child = f.shape
				if (s.typ == reflect.TypeFor[nativewire.WireTree]() || s.typ == reflect.TypeFor[witnessRawTree]() || s.typ == reflect.TypeFor[witnessTreeProjection]()) && (fieldName == "status" || fieldName == "form") && dec.PeekKind() == '"' {
					if e := b.takeTypedShape(doc, dec, child, visits, true); e != nil {
						return e
					}
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
				if s.witness && fieldName == "encoding_source" && dec.PeekKind() == '"' {
					if e := b.takeTypedShape(doc, dec, child, visits, true); e != nil {
						return e
					}
					value, err := dec.ReadToken()
					if err != nil {
						return jsonError(doc, "", err)
					}
					if e := b.charge(1, 0, doc); e != nil {
						return e
					}
					*values++
					if value.String() != SourceDeclaration {
						return typedJSONError(doc, "JSON_SHAPE", dec)
					}
					continue
				}
				if f.nullable && dec.PeekKind() == 'n' {
					if s.witness {
						if e := b.takeTypedShape(doc, dec, child, visits, false); e != nil {
							return e
						}
					}
					if e := b.skipTypedJSON(doc, dec, values); e != nil {
						return e
					}
					continue
				}
			}
			if e := b.readTypedJSONChecked(doc, dec, child, values, visits); e != nil {
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
			if s.witness && s.typ == reflect.TypeFor[witnessTreeProjection]() && name == "tree" {
				optional = slices.Contains([]string{StatusFailed, StatusResourceLimit}, status) || slices.Contains([]string{"summary", "record"}, form)
			}
			if s.witness && s.typ == reflect.TypeFor[witnessRawTree]() {
				if slices.Contains([]string{StatusFailed, StatusResourceLimit}, status) {
					optional = slices.Contains([]string{"descendant_count", "has_error", "digest", "types", "fields", "nodes"}, name)
				} else if slices.Contains([]string{"summary", "record"}, form) {
					optional = slices.Contains([]string{"types", "fields", "nodes"}, name)
				}
			}
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
		if s.base64 {
			return nil
		}
		for dec.PeekKind() != ']' {
			if e := b.readTypedJSONChecked(doc, dec, s.elem, values, visits); e != nil {
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
func (b *eligibilityBudget) readTypedJSONArray(doc string, dec *jsontext.Decoder, s *typedJSONShape, values, visits *uint64) *Error {
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
		if e := b.readTypedJSONChecked(doc, inner, s.elem, values, visits); e != nil {
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
