package kit

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// jsonMaxDepth bounds object/array nesting of every strict document.
const jsonMaxDepth = 32

// maxJSONInteger is 2^53-1, the largest integer every consumer can round-trip exactly.
const maxJSONInteger = 1<<53 - 1

// jv is one strictly decoded JSON value. Numbers keep their exact literal; objects keep
// member order. Duplicate names (including escape-equivalent spellings), invalid UTF-8,
// lone surrogates, trailing values and nesting beyond jsonMaxDepth never reach jv.
// A value keeps only its parent link and member name or index; its JSON pointer is built on
// demand, so a long member name is never copied once per descendant value.
type jv struct {
	kind byte // '{' '[' '"' '0' 't' 'f' 'n'
	s    string
	keys []string
	vals []*jv
	up   *jv    // containing value; nil for the document root
	name string // member name when up is an object
	idx  int    // element index when up is an array
	plen int    // memoized ptrLen()+1; 0 when not computed
}

var pointerEscaper = strings.NewReplacer("~", "~0", "/", "~1")

// cptr is the error-path pointer: built from the root and clipped to maxFindingPath bytes,
// so an error report stays bounded whatever the member names.
func (v *jv) cptr() string {
	p, cut := v.ptrClip(maxFindingPath)
	if cut {
		p += "...(truncated)"
	}
	return p
}

// ptrLen returns len(v.ptr()) without building it; each value computes it once.
func (v *jv) ptrLen() int {
	if v == nil || v.up == nil {
		return 0
	}
	if v.plen == 0 {
		seg := len(strconv.Itoa(v.idx))
		if v.up.kind == '{' {
			seg = len(v.name) + strings.Count(v.name, "~") + strings.Count(v.name, "/")
		}
		v.plen = v.up.ptrLen() + 1 + seg + 1
	}
	return v.plen - 1
}

// ptr returns the RFC 6901 pointer of v ("" for the root).
func (v *jv) ptr() string {
	if v == nil || v.up == nil {
		return ""
	}
	seg := strconv.Itoa(v.idx)
	if v.up.kind == '{' {
		seg = pointerEscaper.Replace(v.name)
	}
	return v.up.ptr() + "/" + seg
}

// decodeStrict is the single strict decoder shared by CLI and API for profile and expected
// documents. doc names the document in error paths ("profile#/files/0/path").
func decodeStrict(doc string, data []byte, maxBytes uint64) (*jv, *Error) {
	return decodeBounded(doc, data, maxBytes, nil)
}

// valueBudget bounds the decoded value count (so a small-valued document cannot allocate far
// beyond its byte size) and runs a cancellation checkpoint every 4096 values.
type valueBudget struct {
	max, n uint64
	check  func() *Error
}

func (b *valueBudget) take(doc string, at *jv) *Error {
	if b == nil {
		return nil
	}
	if b.n++; b.n > b.max {
		return fail(KindResourceLimit, "JSON_VALUE_LIMIT", doc+"#"+at.cptr(), nil)
	}
	if b.n%4096 == 0 {
		return b.check()
	}
	return nil
}

func decodeBounded(doc string, data []byte, maxBytes uint64, b *valueBudget) (*jv, *Error) {
	if uint64(len(data)) > maxBytes {
		return nil, fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", doc, nil)
	}
	if !utf8.Valid(data) {
		return nil, fail(KindInvalidInput, "JSON_INVALID_UTF8", doc, nil)
	}
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	v, e := readJSON(dec, doc, &jv{}, 0, b)
	if e != nil {
		return nil, e
	}
	if _, err := dec.ReadToken(); err != io.EOF {
		if err == nil {
			return nil, fail(KindInvalidInput, "JSON_TRAILING_VALUE", doc, nil)
		}
		return nil, jsonError(doc, "", err)
	}
	return v, nil
}

// readJSON reads one value into v, which already carries its parent link and name or index.
func readJSON(dec *jsontext.Decoder, doc string, v *jv, depth int, b *valueBudget) (*jv, *Error) {
	tok, err := dec.ReadToken()
	if err != nil {
		return nil, jsonError(doc, v.cptr(), err)
	}
	if e := b.take(doc, v); e != nil {
		return nil, e
	}
	v.kind = byte(tok.Kind())
	switch v.kind {
	case '{', '[':
		if depth >= jsonMaxDepth {
			return nil, fail(KindResourceLimit, "JSON_DEPTH_LIMIT", doc+"#"+v.cptr(), nil)
		}
		end := jsontext.Kind('}')
		if v.kind == '[' {
			end = ']'
		}
		for dec.PeekKind() != end {
			child := &jv{up: v, idx: len(v.vals)}
			if v.kind == '{' {
				name, err := dec.ReadToken()
				if err != nil {
					return nil, jsonError(doc, v.cptr(), err)
				}
				child.name = name.String()
				v.keys = append(v.keys, child.name)
			}
			c, e := readJSON(dec, doc, child, depth+1, b)
			if e != nil {
				return nil, e
			}
			v.vals = append(v.vals, c)
		}
		if _, err := dec.ReadToken(); err != nil {
			return nil, jsonError(doc, v.cptr(), err)
		}
	default:
		v.s = tok.String()
	}
	return v, nil
}

func jsonError(doc, ptr string, err error) *Error {
	var se *jsontext.SyntacticError
	if errors.As(err, &se) {
		if p, cut := clipPath(string(se.JSONPointer), maxFindingPath); cut {
			ptr = p + "...(truncated)"
		} else {
			ptr = p
		}
	}
	code := "JSON_SYNTAX"
	switch {
	case errors.Is(err, jsontext.ErrDuplicateName):
		code = "JSON_DUPLICATE_KEY"
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		code = "JSON_TRUNCATED"
	}
	return fail(KindInvalidInput, code, doc+"#"+ptr, err)
}

// typed reads strict values out of one document; every error carries doc#pointer.
type typed struct{ doc string }

func (t typed) bad(code string, v *jv) *Error {
	return fail(KindInvalidInput, code, t.doc+"#"+v.cptr(), nil)
}

// want rejects null distinctly from a wrong type: null is never an absent value.
func (t typed) want(v *jv, kind byte) *Error {
	switch {
	case v.kind == kind:
		return nil
	case v.kind == 'n':
		return t.bad("JSON_NULL", v)
	}
	return t.bad("JSON_TYPE", v)
}

// object returns the members of an object whose names are exactly from required and
// optional; a case-different or unknown name is JSON_UNKNOWN_FIELD.
func (t typed) object(v *jv, required []string, optional ...string) (map[string]*jv, *Error) {
	if e := t.want(v, '{'); e != nil {
		return nil, e
	}
	out := map[string]*jv{}
	for i, k := range v.keys {
		known := false
		for _, name := range append(required, optional...) {
			known = known || k == name
		}
		if !known {
			return nil, fail(KindInvalidInput, "JSON_UNKNOWN_FIELD", t.doc+"#"+v.vals[i].cptr(), nil)
		}
		out[k] = v.vals[i]
	}
	for _, name := range required {
		if out[name] == nil {
			return nil, fail(KindInvalidInput, "JSON_MISSING_FIELD", t.doc+"#"+v.cptr()+"/"+name, nil)
		}
	}
	return out, nil
}

func (t typed) str(v *jv) (string, *Error) {
	if e := t.want(v, '"'); e != nil {
		return "", e
	}
	return v.s, nil
}

func (t typed) boolean(v *jv) (bool, *Error) {
	if v.kind == 't' || v.kind == 'f' {
		return v.kind == 't', nil
	}
	return false, t.want(v, 't')
}

func (t typed) array(v *jv) ([]*jv, *Error) {
	if e := t.want(v, '['); e != nil {
		return nil, e
	}
	return v.vals, nil
}

// uint accepts only a canonical decimal 0..2^53-1: no sign, fraction, exponent or
// leading zero, so nothing is rounded, truncated or defaulted.
func (t typed) uint(v *jv) (uint64, *Error) {
	if e := t.want(v, '0'); e != nil {
		return 0, e
	}
	s := v.s
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, t.bad("JSON_NUMBER_NOT_INTEGER", v)
		}
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, t.bad("JSON_NUMBER_NOT_INTEGER", v)
	}
	if len(s) > 16 {
		return 0, t.bad("JSON_INTEGER_RANGE", v)
	}
	var n uint64
	for _, c := range s {
		n = n*10 + uint64(c-'0')
	}
	if n > maxJSONInteger {
		return 0, t.bad("JSON_INTEGER_RANGE", v)
	}
	return n, nil
}
