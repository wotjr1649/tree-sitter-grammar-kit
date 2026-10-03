package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// schemaBase is an owned node-types fixture: one root, a supertype, fields, children, an
// extra node, the same spelling named and anonymous ("null"), and an anonymous "this".
const schemaBase = `[
 {"type":"program","named":true,"root":true,"fields":{},"children":{"multiple":true,"required":false,"types":[{"type":"statement","named":true}]}},
 {"type":"expression","named":true,"subtypes":[{"type":"call","named":true},{"type":"identifier","named":true}]},
 {"type":"statement","named":true,"fields":{"value":{"multiple":false,"required":true,"types":[{"type":";","named":false},{"type":"expression","named":true}]}}},
 {"type":"call","named":true,"fields":{"arguments":{"multiple":true,"required":false,"types":[{"type":"expression","named":true}]},"function":{"multiple":false,"required":true,"types":[{"type":"identifier","named":true}]}}},
 {"type":"identifier","named":true},
 {"type":"comment","named":true,"extra":true},
 {"type":"null","named":true},
 {"type":"null","named":false},
 {"type":"this","named":false},
 {"type":";","named":false}
]`

// schemaCandidate plants one change of every supported kind relative to schemaBase.
const schemaCandidate = `[
 {"type":"program","named":true,"fields":{},"children":{"multiple":false,"required":true,"types":[{"type":"expression","named":true},{"type":"statement","named":true}]}},
 {"type":"expression","named":true,"subtypes":[{"type":"identifier","named":true},{"type":"number","named":true}]},
 {"type":"statement","named":true,"fields":{"value":{"multiple":false,"required":true,"types":[{"type":"expression","named":true},{"type":"number","named":true}]}},"children":{"multiple":false,"required":false,"types":[{"type":"identifier","named":true}]}},
 {"type":"call","named":true,"fields":{"function":{"multiple":true,"required":false,"types":[{"type":"identifier","named":true}]},"receiver":{"multiple":false,"required":false,"types":[{"type":"expression","named":true}]}}},
 {"type":"identifier","named":true,"fields":{}},
 {"type":"number","named":true},
 {"type":"null","named":true,"extra":true},
 {"type":"null","named":false},
 {"type":"this","named":true},
 {"type":";","named":false}
]`

// Expected forward differences, written from the planted changes (not from the comparator).
var schemaForward = []string{
	"FIELD_REMOVED REMOVAL call:true arguments -",
	"FIELD_ADDED ADDITION call:true receiver -",
	"FIELD_REQUIRED_CHANGED CARDINALITY_WIDENED call:true function -",
	"FIELD_MULTIPLE_CHANGED CARDINALITY_WIDENED call:true function -",
	"NODE_REMOVED REMOVAL comment:true - -",
	"SUBTYPE_REMOVED REMOVAL expression:true - call:true",
	"SUBTYPE_ADDED ADDITION expression:true - number:true",
	"FIELDS_PRESENCE_CHANGED IDENTITY identifier:true - -",
	"EXTRA_CHANGED CLASSIFICATION null:true - -",
	"NODE_ADDED ADDITION number:true - -",
	"ROOT_CHANGED IDENTITY program:true - -",
	"CHILDREN_REQUIRED_CHANGED CARDINALITY_NARROWED program:true - -",
	"CHILDREN_MULTIPLE_CHANGED CARDINALITY_NARROWED program:true - -",
	"CHILDREN_TYPE_ADDED ADDITION program:true - expression:true",
	"FIELD_TYPE_REMOVED REMOVAL statement:true value ;:false",
	"FIELD_TYPE_ADDED ADDITION statement:true value number:true",
	"CHILDREN_PRESENCE_CHANGED ADDITION statement:true - -",
	"CHILDREN_TYPE_ADDED ADDITION statement:true - identifier:true",
	"NODE_NAMED_CHANGED IDENTITY this:false - -",
}

// Expected reverse differences: additions and removals swap, narrowing becomes widening.
var schemaReverse = []string{
	"FIELD_REMOVED REMOVAL call:true receiver -",
	"FIELD_ADDED ADDITION call:true arguments -",
	"FIELD_REQUIRED_CHANGED CARDINALITY_NARROWED call:true function -",
	"FIELD_MULTIPLE_CHANGED CARDINALITY_NARROWED call:true function -",
	"NODE_ADDED ADDITION comment:true - -",
	"SUBTYPE_REMOVED REMOVAL expression:true - number:true",
	"SUBTYPE_ADDED ADDITION expression:true - call:true",
	"FIELDS_PRESENCE_CHANGED IDENTITY identifier:true - -",
	"EXTRA_CHANGED CLASSIFICATION null:true - -",
	"NODE_REMOVED REMOVAL number:true - -",
	"ROOT_CHANGED IDENTITY program:true - -",
	"CHILDREN_REQUIRED_CHANGED CARDINALITY_WIDENED program:true - -",
	"CHILDREN_MULTIPLE_CHANGED CARDINALITY_WIDENED program:true - -",
	"CHILDREN_TYPE_REMOVED REMOVAL program:true - expression:true",
	"FIELD_TYPE_REMOVED REMOVAL statement:true value number:true",
	"FIELD_TYPE_ADDED ADDITION statement:true value ;:false",
	"CHILDREN_PRESENCE_CHANGED REMOVAL statement:true - -",
	"CHILDREN_TYPE_REMOVED REMOVAL statement:true - identifier:true",
	"NODE_NAMED_CHANGED IDENTITY this:true - -",
}

func schemaCheck(t *testing.T, doc string) (SchemaCheckResult, error) {
	t.Helper()
	return SchemaCheck(testCtx(t), SchemaCheckRequest{Input: SchemaInput{Name: "node-types.json", Data: []byte(doc)}, Limits: DefaultSchemaLimits()})
}

func schemaDiff(t *testing.T, before, after string) (SchemaDiffResult, error) {
	t.Helper()
	return SchemaDiff(testCtx(t), SchemaDiffRequest{Baseline: SchemaInput{Name: "node-types.json", Data: []byte(before)},
		Candidate: SchemaInput{Name: "node-types.json", Data: []byte(after)}, Limits: DefaultSchemaLimits()})
}

func diffLines(ds []SchemaDifference) []string {
	out := []string{}
	for _, d := range ds {
		field, member := d.Field, "-"
		if field == "" {
			field = "-"
		}
		if d.Member != nil {
			member = fmt.Sprintf("%s:%v", d.Member.Type, d.Member.Named)
		}
		out = append(out, fmt.Sprintf("%s %s %s:%v %s %s", d.Code, d.Risk, d.Node.Type, d.Node.Named, field, member))
	}
	return out
}

func sameLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s differences\ngot:\n%s\nwant:\n%s", what, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// reorder reverses every array and re-encodes objects with sorted keys (encoding/json),
// independently of the kit decoder.
func reorder(t *testing.T, doc string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatal(err)
	}
	var walk func(any) any
	walk = func(x any) any {
		switch y := x.(type) {
		case []any:
			out := make([]any, len(y))
			for i := range y {
				out[len(y)-1-i] = walk(y[i])
			}
			return out
		case map[string]any:
			for k := range y {
				y[k] = walk(y[k])
			}
		}
		return x
	}
	data, err := json.Marshal(walk(v))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func withoutPaths(ds []SchemaDifference) string {
	cp := append([]SchemaDifference(nil), ds...)
	for i := range cp {
		cp[i].BaselinePath, cp[i].CandidatePath = "", ""
	}
	data, _ := json.Marshal(cp)
	return string(data)
}

// S03-A01/A02/A10: reordered keys, entries and unordered alternatives are the same static
// contract; the same spelling with different named status is two identities.
func TestSchemaEquivalence(t *testing.T) {
	res, err := schemaCheck(t, schemaBase)
	if err != nil || res.Assessment != AssessPass || res.ExecutionStatus != StatusCompleted {
		t.Fatalf("base check: %v %+v", err, res.Report)
	}
	c := res.Schema.Counts
	if c == nil || c.Nodes != 10 || c.Named != 7 || c.Anonymous != 3 || c.Supertypes != 1 || c.Fields != 3 || c.References != 7 || len(c.Roots) != 1 || c.Roots[0] != (NodeRef{"program", true}) {
		t.Fatalf("counts %+v", c)
	}
	if !hasCode(res.Findings, "SCHEMA_STATIC_ONLY") {
		t.Fatal("check must state that it is static only")
	}
	shuffled := reorder(t, schemaBase)
	if !strings.HasPrefix(shuffled, `[{"named":false,"type":";"}`) {
		t.Fatalf("fixture was not reordered: %.60s", shuffled)
	}
	d, err := schemaDiff(t, schemaBase, shuffled)
	if err != nil || d.Assessment != AssessPass || len(d.Differences) != 0 {
		t.Fatalf("reordered schema differs: %v %v", err, diffLines(d.Differences))
	}
	// Deterministic report: same bytes twice, and the same semantic findings for reordered inputs.
	a, _ := schemaDiff(t, schemaBase, schemaCandidate)
	b, _ := schemaDiff(t, schemaBase, schemaCandidate)
	if string(mustJSON(t, a)) != string(mustJSON(t, b)) {
		t.Fatal("same inputs gave different reports")
	}
	r, _ := schemaDiff(t, reorder(t, schemaBase), reorder(t, schemaCandidate))
	if withoutPaths(r.Differences) != withoutPaths(a.Differences) {
		t.Fatalf("reordered inputs changed the findings\n%v\n%v", diffLines(r.Differences), diffLines(a.Differences))
	}
}

// S03-A03/A04/A05: every planted change is reported directionally with its field, member,
// before/after evidence and locations.
func TestSchemaDiffKinds(t *testing.T) {
	res, err := schemaDiff(t, schemaBase, schemaCandidate)
	if err != nil || res.Assessment != AssessFail || res.ExecutionStatus != StatusCompleted {
		t.Fatalf("diff: %v %+v", err, res.Report)
	}
	sameLines(t, "forward", diffLines(res.Differences), schemaForward)
	for _, d := range res.Differences {
		if d.BaselinePath == "" && d.CandidatePath == "" {
			t.Fatalf("%s has no location", d.Code)
		}
	}
	by := map[string]SchemaDifference{}
	for _, d := range res.Differences {
		by[d.Code+" "+d.Node.Type+" "+d.Field] = d
	}
	checks := []struct{ key, before, after, bpath, cpath string }{
		{"FIELD_REQUIRED_CHANGED call function", "true", "false", "/3/fields/function/required", "/3/fields/function/required"},
		{"ROOT_CHANGED program ", "true", "null", "/0/root", ""},
		{"FIELDS_PRESENCE_CHANGED identifier ", "null", "{}", "", "/4/fields"},
		{"NODE_REMOVED comment ", `{"type":"comment","named":true,"extra":true}`, "null", "/5", ""},
		{"NODE_NAMED_CHANGED this ", `{"type":"this","named":false}`, `{"type":"this","named":true}`, "/8", "/8"},
		{"FIELD_TYPE_REMOVED statement value", `{"type":";","named":false}`, "null", "/2/fields/value/types/0", ""},
		{"CHILDREN_PRESENCE_CHANGED statement ", "null", `{"multiple":false,"required":false,"types":[{"type":"identifier","named":true}]}`, "", "/2/children"},
	}
	for _, c := range checks {
		d, ok := by[c.key]
		if !ok || string(d.Before) != c.before || string(d.After) != c.after || d.BaselinePath != c.bpath || d.CandidatePath != c.cpath {
			t.Fatalf("%s: got %s→%s at %q→%q", c.key, d.Before, d.After, d.BaselinePath, d.CandidatePath)
		}
	}
	// A supertype gained or lost entirely.
	small := `[{"type":"a","named":true},{"type":"b","named":true}%s]`
	s, err := schemaDiff(t, fmt.Sprintf(small, ""), fmt.Sprintf(small, `,{"type":"s","named":true,"subtypes":[{"type":"a","named":true},{"type":"b","named":true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	sameLines(t, "supertype added", diffLines(s.Differences), []string{"NODE_ADDED ADDITION s:true - -"})
	s, err = schemaDiff(t, `[{"type":"a","named":true},{"type":"s","named":true}]`, `[{"type":"a","named":true},{"type":"s","named":true,"subtypes":[{"type":"a","named":true}]}]`)
	if err != nil {
		t.Fatal(err)
	}
	sameLines(t, "becomes supertype", diffLines(s.Differences), []string{"SUBTYPES_PRESENCE_CHANGED ADDITION s:true - -", "SUBTYPE_ADDED ADDITION s:true - a:true"})
}

// S03-A09: swapping inputs inverts additions/removals and keeps both raw identities, even
// when both inputs carry the same name.
func TestSchemaReverse(t *testing.T) {
	f, err := schemaDiff(t, schemaBase, schemaCandidate)
	if err != nil {
		t.Fatal(err)
	}
	r, err := schemaDiff(t, schemaCandidate, schemaBase)
	if err != nil {
		t.Fatal(err)
	}
	sameLines(t, "reverse", diffLines(r.Differences), schemaReverse)
	if f.Baseline.SHA256 == f.Candidate.SHA256 || f.Baseline.Name != f.Candidate.Name {
		t.Fatal("fixture inputs must share a name and differ in bytes")
	}
	if r.Baseline.SHA256 != f.Candidate.SHA256 || r.Candidate.SHA256 != f.Baseline.SHA256 || r.Baseline.Role != "baseline" {
		t.Fatalf("reverse identities: %+v %+v", r.Baseline, r.Candidate)
	}
	ids := map[string]string{}
	for _, id := range f.Identities {
		ids[id.Role] = id.SHA256
	}
	if ids["baseline"] != f.Baseline.SHA256 || ids["candidate"] != f.Candidate.SHA256 || ids["policy"] == "" {
		t.Fatalf("identities %+v", f.Identities)
	}
	if string(r.Differences[0].Before) != string(f.Differences[1].After) {
		t.Fatal("reverse before/after not swapped")
	}
	// Equal inputs are equal whichever way round.
	e, err := schemaDiff(t, schemaCandidate, schemaCandidate)
	if err != nil || e.Assessment != AssessPass || len(e.Differences) != 0 {
		t.Fatalf("self diff: %v %v", err, diffLines(e.Differences))
	}
}

// S03-A06/A07: invalid or unsupported schemas are typed outcomes, never a partial PASS;
// null, missing and empty are not defaulted to a valid empty value.
func TestSchemaInvalid(t *testing.T) {
	sub := func(extra string) string { return `[{"type":"a","named":true` + extra + `}]` }
	set := `{"multiple":false,"required":true,"types":[{"type":"a","named":true}]}`
	cases := []struct {
		name, doc, assess string
		codes             []string
	}{
		{"not-array", `{"type":"a","named":true}`, AssessFail, []string{"JSON_TYPE"}},
		{"null-document", `null`, AssessFail, []string{"JSON_NULL"}},
		{"zero-nodes", `[]`, AssessFail, []string{"SCHEMA_EMPTY"}},
		{"entry-not-object", `[1]`, AssessFail, []string{"JSON_TYPE"}},
		{"named-not-bool", `[{"type":"a","named":"yes"}]`, AssessFail, []string{"JSON_TYPE"}},
		{"type-not-string", `[{"type":7,"named":true}]`, AssessFail, []string{"JSON_TYPE"}},
		{"type-empty", `[{"type":"","named":true}]`, AssessFail, []string{"NODE_TYPE_EMPTY"}},
		{"missing-type", `[{"named":true}]`, AssessFail, []string{"JSON_MISSING_FIELD"}},
		{"missing-named", `[{"type":"a"}]`, AssessFail, []string{"JSON_MISSING_FIELD"}},
		{"duplicate-node", `[{"type":"a","named":true},{"type":"a","named":true}]`, AssessFail, []string{"NODE_DUPLICATE"}},
		{"duplicate-node-other-shape", `[{"type":"?","named":false},{"type":"?","named":false,"fields":{}}]`, AssessFail, []string{"NODE_DUPLICATE"}},
		{"duplicate-field", sub(`,"fields":{"f":` + set + `,"f":` + set + `}`), AssessFail, []string{"JSON_DUPLICATE_KEY"}},
		{"escaped-duplicate-field", sub(`,"fields":{"f":` + set + `,"f":` + set + `}`), AssessFail, []string{"JSON_DUPLICATE_KEY"}},
		{"duplicate-member", sub(`,"children":{"multiple":false,"required":true,"types":[{"type":"a","named":true},{"type":"a","named":true}]}`), AssessFail, []string{"MEMBER_DUPLICATE"}},
		{"unsupported-key", sub(`,"visible":true`), AssessBlocked, []string{"SCHEMA_KEY_UNSUPPORTED"}},
		{"unsupported-member-key", sub(`,"children":{"multiple":false,"required":true,"types":[{"type":"a","named":true,"alias":true}]}`), AssessBlocked, []string{"SCHEMA_KEY_UNSUPPORTED"}},
		{"unsupported-and-invalid", `[{"type":"a","named":true,"visible":true},{"type":"a","named":true}]`, AssessFail, []string{"SCHEMA_KEY_UNSUPPORTED", "NODE_DUPLICATE"}},
		{"broken-subtype", `[{"type":"s","named":true,"subtypes":[{"type":"missing","named":true}]}]`, AssessFail, []string{"REFERENCE_UNRESOLVED"}},
		{"subtype-named-mismatch", `[{"type":"a","named":false},{"type":"s","named":true,"subtypes":[{"type":"a","named":true}]}]`, AssessFail, []string{"REFERENCE_UNRESOLVED"}},
		{"cycle", `[{"type":"s","named":true,"subtypes":[{"type":"t","named":true}]},{"type":"t","named":true,"subtypes":[{"type":"s","named":true}]}]`, AssessFail, []string{"SUPERTYPE_CYCLE"}},
		{"self-cycle", `[{"type":"s","named":true,"subtypes":[{"type":"s","named":true}]}]`, AssessFail, []string{"SUPERTYPE_CYCLE"}},
		{"types-empty", sub(`,"fields":{"f":{"multiple":false,"required":true,"types":[]}}`), AssessFail, []string{"TYPES_EMPTY"}},
		{"subtypes-empty", sub(`,"subtypes":[]`), AssessFail, []string{"SUBTYPES_EMPTY"}},
		{"supertype-with-fields", `[{"type":"a","named":true},{"type":"s","named":true,"fields":{},"subtypes":[{"type":"a","named":true}]}]`, AssessFail, []string{"SUPERTYPE_SHAPE"}},
		{"two-roots", `[{"type":"a","named":true,"root":true},{"type":"b","named":true,"root":true}]`, AssessFail, []string{"ROOT_MULTIPLE"}},
		{"field-name-empty", sub(`,"fields":{"":` + set + `}`), AssessFail, []string{"FIELD_NAME_EMPTY"}},
		{"fields-null", sub(`,"fields":null`), AssessFail, []string{"JSON_NULL"}},
		{"children-null", sub(`,"children":null`), AssessFail, []string{"JSON_NULL"}},
		{"root-null", sub(`,"root":null`), AssessFail, []string{"JSON_NULL"}},
		{"set-missing-required", sub(`,"children":{"multiple":false,"types":[{"type":"a","named":true}]}`), AssessFail, []string{"JSON_MISSING_FIELD"}},
		{"set-empty-object", sub(`,"children":{}`), AssessFail, []string{"JSON_MISSING_FIELD"}},
		{"trailing-value", sub(``) + ` []`, AssessFail, []string{"JSON_TRAILING_VALUE"}},
		{"truncated", sub(``)[:10], AssessFail, []string{"JSON_TRUNCATED"}},
		{"invalid-utf8", "[{\"type\":\"\xff\",\"named\":true}]", AssessFail, []string{"JSON_INVALID_UTF8"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := schemaCheck(t, c.doc)
			if err != nil {
				t.Fatalf("check error %v", err)
			}
			if res.Assessment != c.assess || res.ExecutionStatus != StatusCompleted || res.Schema.Counts != nil {
				t.Fatalf("assessment %s status %s counts %v", res.Assessment, res.ExecutionStatus, res.Schema.Counts)
			}
			for _, code := range c.codes {
				if !hasCode(res.Findings, code) {
					t.Fatalf("missing %s in %+v", code, res.Findings)
				}
			}
			// A diff never compares an invalid input.
			d, err := schemaDiff(t, schemaBase, c.doc)
			want := "SCHEMA_INVALID"
			kind := KindInvalidInput
			if c.assess == AssessBlocked {
				want, kind = "SCHEMA_KEY_UNSUPPORTED", KindUnsupported
			}
			kindOf(t, err, kind, want)
			if d.Assessment == AssessPass || d.ExecutionStatus == StatusCompleted || len(d.Differences) != 0 || d.Candidate.Counts != nil || !hasCode(d.Findings, c.codes[0]) {
				t.Fatalf("invalid diff input: %+v", d)
			}
		})
	}
	// Valid conventions: same spelling named/anonymous, undeclared field alternative (reported),
	// nested supertypes, anonymous entries with fields, empty fields object.
	valid := `[{"type":"a","named":true},{"type":"a","named":false,"fields":{}},{"type":"t","named":true,"subtypes":[{"type":"a","named":true}]},
		{"type":"s","named":true,"subtypes":[{"type":"t","named":true},{"type":"a","named":false}]},
		{"type":"x","named":true,"fields":{"f":{"multiple":false,"required":false,"types":[{"type":"alias_only","named":true}]}}}]`
	res, err := schemaCheck(t, valid)
	if err != nil || res.Assessment != AssessPass || !hasFinding(res.Findings, "REFERENCE_UNDECLARED", "node-types.json#/4/fields/f/types/0") {
		t.Fatalf("valid conventions: %v %+v", err, res.Report)
	}
	// Absent fields and an explicitly empty fields object are different contracts.
	d, err := schemaDiff(t, `[{"type":"a","named":true}]`, `[{"type":"a","named":true,"fields":{}}]`)
	if err != nil {
		t.Fatal(err)
	}
	sameLines(t, "fields presence", diffLines(d.Differences), []string{"FIELDS_PRESENCE_CHANGED IDENTITY a:true - -"})
}

// S03-A08: limits at and one over each bound, deep supertype chains without recursion,
// and typed cancellation.
func TestSchemaLimits(t *testing.T) {
	base := []byte(schemaBase)
	ok, _ := schemaCheck(t, schemaBase)
	c := ok.Schema.Counts
	records := c.Nodes + c.Fields + c.References
	check := func(l SchemaLimits, data []byte) (SchemaCheckResult, error) {
		return SchemaCheck(testCtx(t), SchemaCheckRequest{Input: SchemaInput{Name: "n", Data: data}, Limits: l})
	}
	l := DefaultSchemaLimits()
	l.Records = records
	if _, err := check(l, base); err != nil {
		t.Fatalf("records at limit: %v", err)
	}
	l.Records = records - 1
	r, err := check(l, base)
	kindOf(t, err, KindResourceLimit, "SCHEMA_RECORD_LIMIT")
	if r.ExecutionStatus != StatusResourceLimit || r.Assessment != AssessBlocked || r.Schema.Counts != nil {
		t.Fatalf("limit report %+v", r.Report)
	}
	l = DefaultSchemaLimits()
	l.DocumentBytes = uint64(len(base))
	if _, err := check(l, base); err != nil {
		t.Fatalf("bytes at limit: %v", err)
	}
	l.DocumentBytes--
	_, err = check(l, base)
	kindOf(t, err, KindResourceLimit, "DOCUMENT_BYTES_LIMIT")
	// The report repeats output_bytes, so find a 4-digit limit equal to the exact output size.
	full, _ := check(DefaultSchemaLimits(), base)
	defaultLen := uint64(len(mustJSON(t, full)))
	out := defaultLen - uint64(len(fmt.Sprint(DefaultSchemaLimits().OutputBytes))) + 4 + 1
	if len(fmt.Sprint(out)) != 4 {
		t.Fatalf("output size %d is not 4 digits", out)
	}
	l = DefaultSchemaLimits()
	l.OutputBytes = out
	if _, err := check(l, base); err != nil {
		t.Fatalf("output at limit: %v", err)
	}
	l.OutputBytes = out - 1
	_, err = check(l, base)
	kindOf(t, err, KindResourceLimit, "OUTPUT_LIMIT")
	l = DefaultSchemaLimits()
	l.Records = 2 // 16 JSON values
	_, err = check(l, []byte("[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1]"))
	kindOf(t, err, KindResourceLimit, "JSON_VALUE_LIMIT")
	_, err = check(DefaultSchemaLimits(), []byte(strings.Repeat("[", 40)+strings.Repeat("]", 40)))
	kindOf(t, err, KindResourceLimit, "JSON_DEPTH_LIMIT")
	l = DefaultSchemaLimits()
	l.Wall = 0
	_, err = check(l, base)
	kindOf(t, err, KindInvalidInput, "LIMITS_INVALID")

	// A 20000-level supertype chain is walked iteratively; closing it reports one cycle.
	chain := func(closed bool) []byte {
		var b strings.Builder
		b.WriteString("[")
		const n = 20000
		for i := 0; i < n; i++ {
			next := fmt.Sprintf("s%d", i+1)
			if i == n-1 {
				next = "leaf"
				if closed {
					next = "s0"
				}
			}
			fmt.Fprintf(&b, `{"type":"s%d","named":true,"subtypes":[{"type":%q,"named":true}]},`, i, next)
		}
		b.WriteString(`{"type":"leaf","named":true}]`)
		return []byte(b.String())
	}
	deep, err := check(DefaultSchemaLimits(), chain(false))
	if err != nil || deep.Assessment != AssessPass || deep.Schema.Counts.Supertypes != 20000 {
		t.Fatalf("deep chain: %v %+v", err, deep.Report)
	}
	cyc, err := check(DefaultSchemaLimits(), chain(true))
	if err != nil || cyc.Assessment != AssessFail || len(cyc.Findings) != 2 || cyc.Findings[0].Code != "SUPERTYPE_CYCLE" {
		t.Fatalf("closed chain: %v %+v", err, cyc.Findings)
	}

	// Cancellation before the call and in the middle of decoding / walking.
	dead, cancel := context.WithTimeout(context.Background(), time.Minute)
	cancel()
	r, err = SchemaCheck(dead, SchemaCheckRequest{Input: SchemaInput{Name: "n", Data: base}, Limits: DefaultSchemaLimits()})
	kindOf(t, err, KindCancelled, "CANCELLED")
	if r.ExecutionStatus != StatusCancelled || r.Schema.Counts != nil {
		t.Fatalf("cancel report %+v", r.Report)
	}
	// Count the checkpoints of a full run, then cancel at the first, middle and last one
	// (decode, record accounting and the supertype walk all query the context).
	parent, stop := context.WithTimeout(context.Background(), time.Minute)
	defer stop()
	probe := &countingCtx{Context: parent}
	probe.left.Store(1 << 40)
	if _, err := SchemaCheck(probe, SchemaCheckRequest{Input: SchemaInput{Name: "n", Data: chain(false)}, Limits: DefaultSchemaLimits()}); err != nil {
		t.Fatal(err)
	}
	total := 1<<40 - probe.left.Load()
	if total < 20 {
		t.Fatalf("too few checkpoints: %d", total)
	}
	for _, at := range []int64{1, total / 2, total - 1} {
		ctx := &countingCtx{Context: parent}
		ctx.left.Store(at)
		_, err = SchemaCheck(ctx, SchemaCheckRequest{Input: SchemaInput{Name: "n", Data: chain(false)}, Limits: DefaultSchemaLimits()})
		kindOf(t, err, KindCancelled, "CANCELLED")
	}
	_, err = SchemaCheck(context.Background(), SchemaCheckRequest{Input: SchemaInput{Data: base}, Limits: DefaultSchemaLimits()})
	kindOf(t, err, KindInvalidInput, "DEADLINE_REQUIRED")
}

// S03-A12: set normalization stays inside schema alternatives. The shared strict decoder
// keeps an ordered-tree document's sibling order and duplicates exactly, and a repeated
// schema member is reported, not silently merged.
func TestSchemaScopeOrderedTree(t *testing.T) {
	tree := `{"schema":"tsgk-tree/r0","nodes":[{"parent":-1,"type":"program"},{"parent":0,"type":"b"},{"parent":0,"type":"a"},{"parent":0,"type":"a"}],
		"captures":["name","name","call","body"]}`
	v, e := decodeStrict("tree", []byte(tree), MaxDocumentBytes)
	if e != nil {
		t.Fatal(e)
	}
	var types, names []string
	for _, n := range v.vals[1].vals {
		types = append(types, n.vals[1].s)
	}
	for _, n := range v.vals[2].vals {
		names = append(names, n.s)
	}
	if strings.Join(types, ",") != "program,b,a,a" || strings.Join(names, ",") != "name,name,call,body" {
		t.Fatalf("ordered document changed: %v %v", types, names)
	}
	res, err := schemaCheck(t, `[{"type":"a","named":true,"children":{"multiple":true,"required":true,"types":[{"type":"b","named":true},{"type":"a","named":true},{"type":"a","named":true}]}},{"type":"b","named":true}]`)
	if err != nil || res.Assessment != AssessFail || !hasFinding(res.Findings, "MEMBER_DUPLICATE", "node-types.json#/0/children/types/2") {
		t.Fatalf("duplicate member: %v %+v", err, res.Findings)
	}
}
