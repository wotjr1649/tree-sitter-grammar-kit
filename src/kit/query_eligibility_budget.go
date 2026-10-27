package kit

import (
	"bytes"
	"encoding/hex"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

const eligibilityWorkLimit uint64 = 32000000
const eligibilityLogicalLimit uint64 = 1 << 30

// Legacy headers need no AST; reserve decoder/error buffers and typed values before decoding.
func (b *eligibilityBudget) reserveLegacyReplayResult(data []byte) *Error {
	if b == nil {
		return nil
	}
	if e := b.charge(uint64(len(data))+1, 32*uint64(len(data))+32768, "legacy-replay-buffers"); e != nil {
		return e
	}
	dec := jsontext.NewDecoder(bytes.NewBuffer(data), jsontext.AllowDuplicateNames(true))
	values := uint64(0)
	for {
		if e := b.r.check(); e != nil {
			return e
		}
		kind, count := dec.StackIndex(dec.StackDepth())
		name := kind == '{' && count%2 == 0
		token, err := dec.ReadToken()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			var syntax *jsontext.SyntacticError
			if errors.As(err, &syntax) && syntax.Err != nil && syntax.Err.Error() == "exceeded max depth" {
				return fail(KindResourceLimit, "JSON_DEPTH_LIMIT", "replay-result", nil)
			}
			return fail(KindInvalidInput, "JSON_SYNTAX", "replay-result", nil)
		}
		if !name && token.Kind() != '}' && token.Kind() != ']' {
			values++
			if values > 2000000 {
				return fail(KindResourceLimit, "JSON_VALUE_LIMIT", "replay-result", nil)
			}
			if e := b.charge(1, 448, "legacy-replay-values"); e != nil {
				return e
			}
		}
	}
}

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
	_, e := b.decodeValue(doc, data, target, exact)
	return e
}

func (b *eligibilityBudget) decodeValue(doc string, data []byte, target any, exact bool) (*jv, *Error) {
	if e := b.charge(1, 2*uint64(len(data)), doc); e != nil {
		return nil, e
	}
	if uint64(len(data)) > MaxDocumentBytes {
		return nil, fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", doc, nil)
	}
	if !utf8.Valid(data) {
		return nil, fail(KindInvalidInput, "JSON_INVALID_UTF8", doc, nil)
	}
	if e := b.reserveJSON(doc, data, 3); e != nil {
		return nil, e
	}
	values := &valueBudget{max: 2000000, check: b.r.check, charge: func() *Error { return b.charge(1, 0, doc) }}
	v, e := decodeBounded(doc, data, MaxDocumentBytes, values)
	if e != nil {
		return nil, e
	}
	shape := &valueBudget{max: 2000000, check: b.r.check, charge: func() *Error { return b.charge(1, 0, doc) }, reserve: func(n uint64) *Error { return b.charge(0, n, doc) }}
	if e := eligibilityShape(typed{doc: doc}, v, reflect.TypeOf(target).Elem(), exact, shape); e != nil {
		return nil, e
	}
	if e := b.charge(values.n, uint64(len(data)), doc); e != nil {
		return nil, e
	}
	if err := jsonv2.Unmarshal(data, target, jsonv2.RejectUnknownMembers(exact)); err != nil {
		return nil, fail(KindInvalidInput, "JSON_SHAPE", doc, err)
	}
	return v, nil
}

// Registration optionals use omitempty; native wire nullable arms are explicit.
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
		if e := budget.allocate(256 + 256*uint64(typ.NumField())); e != nil {
			return e
		}
		fields := map[string]reflect.StructField{}
		var collect func(reflect.Type) *Error
		collect = func(st reflect.Type) *Error {
			if e := budget.allocate(256 * uint64(st.NumField())); e != nil {
				return e
			}
			if e := budget.charge(); e != nil {
				return e
			}
			for i := 0; i < st.NumField(); i++ {
				f := st.Field(i)
				tag := f.Tag.Get("json")
				name, _, _ := strings.Cut(tag, ",")
				if strings.Contains(tag, ",inline") || f.Anonymous && tag == "" && f.Type.Kind() == reflect.Struct {
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
			if v.vals[i].kind == 'n' && eligibilityNativeNullable(typ, name) {
				continue
			}
			if e := eligibilityShape(t, v.vals[i], f.Type, exact, budget); e != nil {
				return e
			}
		}
		for name, f := range fields {
			if jvMember(v, name) == nil && !eligibilityNativeOptional(typ, name, v) && (!strings.Contains(f.Tag.Get("json"), ",omitempty") || typ == reflect.TypeFor[nativewire.Producer]() || typ == reflect.TypeFor[nativewire.Step]()) {
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

// Nullable wire arms do not permit null primitives or null compact-array elements.
func eligibilityNativeNullable(typ reflect.Type, name string) bool {
	switch typ {
	case reflect.TypeFor[QuerySemanticsResult]():
		return name == "evidence" || name == "eligibility"
	case reflect.TypeFor[QueryReferenceEligibilityResult]():
		return name == "evidence"
	case reflect.TypeFor[nativewire.Step]():
		return name == "edit" || name == "route" || name == "fresh"
	case reflect.TypeFor[nativewire.WireTree]():
		return name == "form" || name == "types" || name == "fields" || name == "nodes" || name == "errors" || name == "declarations" || name == "partial_trees"
	case reflect.TypeFor[nativewire.DeclItem]():
		return name == "name"
	case reflect.TypeFor[semanticProcessCapture]():
		return name == "spec" || name == "process" || name == "stdout" || name == "stderr" || name == "error"
	case reflect.TypeFor[semanticCaptureSpec]():
		return name == "Args" || name == "Env" || name == "Stdin"
	case reflect.TypeFor[semanticCaptureError]():
		return name == "kind" || name == "blocked"
	case reflect.TypeFor[semanticExpectationResult]():
		return name == "contains"
	case reflect.TypeFor[semanticQueryExpectationResult]():
		return name == "captures" || name == "error"
	case reflect.TypeFor[semanticBuildReuse]():
		return name == "source_steps"
	case reflect.TypeFor[semanticOracleMeaning]():
		return name == "declarations" || name == "fact_pack"
	case reflect.TypeFor[semanticOracleCase]():
		return name == "dynamic_sql_expect"
	case reflect.TypeFor[QueryExpectation]():
		return name == "captures" || name == "error"
	}
	return false
}

func (b *eligibilityBudget) significantJSONBytes(data []byte) (uint64, *Error) {
	var significant uint64
	quoted, escaped := false, false
	for i, c := range data {
		if i%4096 == 0 {
			if e := b.r.check(); e != nil {
				return 0, e
			}
		}
		if quoted {
			significant++
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
		} else if c == '"' {
			quoted = true
			significant++
		} else if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			significant++
		}
	}
	return significant, nil
}

// Token preflight reserves decoder/error buffers first, then AST and typed containers before any AST pass.
func (b *eligibilityBudget) reserveJSON(doc string, data []byte, passes uint64) *Error {
	if b == nil {
		return nil
	}
	if e := b.r.check(); e != nil {
		return e
	}
	if e := b.charge(1, uint64(len(data)), doc); e != nil {
		return e
	}
	if uint64(len(data)) > MaxDocumentBytes {
		return fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", doc, nil)
	}
	if !utf8.Valid(data) {
		return fail(KindInvalidInput, "JSON_INVALID_UTF8", doc, nil)
	}
	significant, sizeErr := b.significantJSONBytes(data)
	if sizeErr != nil {
		return sizeErr
	}
	// bytes.Buffer aliases the input; only strings and non-whitespace syntax can grow names/error paths.
	if e := b.charge(0, 32*significant+32768, doc); e != nil {
		return e
	}
	dec := jsontext.NewDecoder(bytes.NewBuffer(data))
	values := uint64(0)
	for {
		if e := b.r.check(); e != nil {
			return e
		}
		kind, count := dec.StackIndex(dec.StackDepth())
		if dec.StackDepth() == 0 && count > 0 {
			if _, err := dec.ReadToken(); err == io.EOF {
				break
			} else if err != nil {
				return jsonError(doc, "", err)
			}
			return fail(KindInvalidInput, "JSON_TRAILING_VALUE", doc, nil)
		}
		name := kind == '{' && count%2 == 0
		peek := dec.PeekKind()
		if !name && peek != '}' && peek != ']' && peek != 0 {
			values++
			if values > 2000000 {
				return fail(KindResourceLimit, "JSON_VALUE_LIMIT", doc, nil)
			}
			// 448 covers one jv, geometric child/key growth, and subsequent typed containers.
			if e := b.charge(1, 448, doc); e != nil {
				return e
			}
			if (peek == '{' || peek == '[') && dec.StackDepth() >= jsonMaxDepth {
				return fail(KindResourceLimit, "JSON_DEPTH_LIMIT", doc, nil)
			}
		}
		if _, err := dec.ReadToken(); err == io.EOF {
			break
		} else if err != nil {
			return jsonError(doc, "", err)
		}
	}
	work, e := eligibilityProduct(doc, values, passes)
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

// Full trees omit summary-only arms in the actual native/r4 producer.
func eligibilityNativeOptional(typ reflect.Type, name string, v *jv) bool {
	if typ != reflect.TypeFor[nativewire.WireTree]() {
		return false
	}
	status, form := jvMember(v, "status"), jvMember(v, "form")
	if status != nil && status.kind == '"' && status.s != StatusCompleted {
		return name != "status" && name != "code" && name != "parse_ms" && name != "form" && name != "queries" && name != "api"
	}
	return form != nil && form.kind == '"' && form.s == "full" && (name == "reason" || name == "errors" || name == "partial_trees")
}
