package kit

import (
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
)

// Gate comparison under the BrightScript v0.1.2 historical policy S07-REPLAY-2ULP-r1, a
// port of that workload's archived replay_compare.py. It belongs to the bs-gate-compare-r1
// reducer only and is not a kit-wide tolerance: everything is exact except the allowlisted
// log-derived exponent fields, which admit at most 2 ULP between finite normal values of
// the same sign, never straddle their original threshold within the 2 ULP corridor and
// keep the same threshold decision; zero and subnormal values are bit-exact.

// GateComparePolicy is the policy identifier of the historical comparison.
const GateComparePolicy = "S07-REPLAY-2ULP-r1"

const gateMaxULPs = 2

var bsGates = []string{"B5-01-MEMORY", "B5-02-LIFECYCLE", "A5-01-COST", "CANCEL", "CANCEL-OVERSHOOT",
	"MAX-CALLBACK-GAP", "CLEANUP-ALL", "LARGE-INPUT", "QUERY-MALFORMED", "VALID-PARSE",
	"SEM-PUBLIC", "INCREMENTAL-REPAIR", "RESUME-RESET", "SUPPORT", "REGRESSION-SWEEP",
	"ABS-MEMORY", "RECOVERY-LOCALITY"}

// GateDifference is one admitted rounding difference of a derived field.
type GateDifference struct {
	Gate       string  `json:"gate_id"`
	Point      string  `json:"point_id"`
	Path       string  `json:"path"`
	Recorded   float64 `json:"recorded"`
	Recomputed float64 `json:"recomputed"`
	ULPs       uint64  `json:"ulp_distance"`
	Threshold  float64 `json:"threshold"`
}

// GateComparison is the result of comparing one recorded and one recomputed gate.
type GateComparison struct {
	Status      string           `json:"status"` // BITWISE_EQUAL | REPLAY_EQUIVALENT_WITH_DECLARED_ROUNDING
	Policy      string           `json:"policy"`
	Gate        string           `json:"gate_id"`
	Verdict     string           `json:"verdict"`
	Differences []GateDifference `json:"differences"`
}

type gateMismatch struct{ code string }

func (g gateMismatch) Error() string { return g.code }

func mismatch(code string, path []string) error {
	if len(path) == 0 {
		return gateMismatch{code}
	}
	return gateMismatch{code + ": " + strings.Join(path, "/")}
}

// numeric kinds, as the archived verifier's json.loads produced them
func isFloatLiteral(lit string) bool { return strings.ContainsAny(lit, ".eE") }

func jvFloat(v *jv) (float64, error) {
	f, err := strconv.ParseFloat(v.s, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, gateMismatch{"NONFINITE_JSON"}
	}
	return f, nil
}

// typeName is the archived verifier's Python type of a value.
func typeName(v *jv) string {
	switch v.kind {
	case '{':
		return "dict"
	case '[':
		return "list"
	case '"':
		return "str"
	case 't', 'f':
		return "bool"
	case 'n':
		return "none"
	}
	if isFloatLiteral(v.s) {
		return "float"
	}
	return "int"
}

func jvMember(v *jv, k string) *jv {
	if v == nil || v.kind != '{' {
		return nil
	}
	if i := slices.Index(v.keys, k); i >= 0 {
		return v.vals[i]
	}
	return nil
}

func keySet(v *jv) []string {
	k := slices.Clone(v.keys)
	slices.Sort(k)
	return k
}

func pathKey(path []string) string { return strings.Join(path, "\x00") }

// derivedFields is the allowlist of log-derived fields per gate and point shape, with
// each field's unchanged threshold.
func derivedFields(gate string, p *jv) (map[string]float64, error) {
	keys := keySet(p)
	is := func(want ...string) bool { slices.Sort(want); return slices.Equal(keys, want) }
	str := func(k string) string {
		if v := jvMember(p, k); v != nil && v.kind == '"' {
			return v.s
		}
		return ""
	}
	switch {
	case gate == "B5-01-MEMORY" && is("family", "sizes", "exponent", "pass"):
		return map[string]float64{pathKey([]string{"exponent"}): 1.5}, nil
	case gate == "B5-02-LIFECYCLE" && str("check") == "call exponent 1,000 -> 4,000" && is("check", "exponent", "pass"):
		return map[string]float64{pathKey([]string{"exponent"}): 1.5}, nil
	case gate == "A5-01-COST" && is("family", "op", "sizes", "exponents", "pass"):
		return map[string]float64{pathKey([]string{"exponents", "0"}): 1.5, pathKey([]string{"exponents", "1"}): 1.5}, nil
	case gate == "QUERY-MALFORMED" && jvMember(p, "family") != nil && jvMember(p, "exponent") != nil:
		return map[string]float64{pathKey([]string{"exponent"}): 1.5}, nil
	case gate == "VALID-PARSE" && is("family", "largest_pair_exponents", "pass"):
		return map[string]float64{pathKey([]string{"largest_pair_exponents", "0"}): 1.2, pathKey([]string{"largest_pair_exponents", "1"}): 1.2}, nil
	case gate == "REGRESSION-SWEEP" && jvMember(p, "completed") != nil && jvMember(p, "completed").kind == 't':
		ex := jvMember(p, "exponents")
		if ex == nil || ex.kind != '{' || !slices.Equal(keySet(ex), []string{"400-20000", "4000-20000"}) {
			return nil, gateMismatch{"UNKNOWN_DERIVED_SCHEMA"}
		}
		return map[string]float64{pathKey([]string{"exponents", "400-20000"}): 1.5, pathKey([]string{"exponents", "4000-20000"}): 1.5, pathKey([]string{"exponent"}): 1.5}, nil
	}
	return map[string]float64{}, nil
}

func ulpCorridor(v float64) (float64, float64) {
	lo, hi := v, v
	for range gateMaxULPs {
		lo, hi = math.Nextafter(lo, math.Inf(-1)), math.Nextafter(hi, math.Inf(1))
	}
	return lo, hi
}

// compareGateValue compares a recorded and a recomputed value exactly, except allowed
// derived float fields.
func compareGateValue(a, b *jv, path []string, allowed map[string]float64, changes *[]GateDifference) error {
	ta, tb := typeName(a), typeName(b)
	if ta != tb {
		return mismatch("TYPE_DIFFERENCE", path)
	}
	switch ta {
	case "dict":
		if !slices.Equal(keySet(a), keySet(b)) {
			return mismatch("KEY_DIFFERENCE", path)
		}
		for i, k := range a.keys {
			if err := compareGateValue(a.vals[i], jvMember(b, k), append(slices.Clone(path), k), allowed, changes); err != nil {
				return err
			}
		}
	case "list":
		if len(a.vals) != len(b.vals) {
			return mismatch("CARDINALITY_DIFFERENCE", path)
		}
		for i := range a.vals {
			if err := compareGateValue(a.vals[i], b.vals[i], append(slices.Clone(path), strconv.Itoa(i)), allowed, changes); err != nil {
				return err
			}
		}
	case "float":
		x, err := jvFloat(a)
		if err != nil {
			return mismatch("NONFINITE", path)
		}
		y, err := jvFloat(b)
		if err != nil {
			return mismatch("NONFINITE", path)
		}
		ba, bb := math.Float64bits(x), math.Float64bits(y)
		limit, has := allowed[pathKey(path)]
		normal := math.Abs(x) >= 0x1p-1022 && math.Abs(y) >= 0x1p-1022 && ba>>63 == bb>>63
		if !has || !normal {
			if ba != bb {
				return mismatch("EXACT_FLOAT_DIFFERENCE", path)
			}
			return nil
		}
		for _, v := range []float64{x, y} {
			lo, hi := ulpCorridor(v)
			if math.IsInf(lo, 0) || math.IsInf(hi, 0) || (lo <= limit && limit < hi) {
				return mismatch("NUMERIC_BOUNDARY_INDETERMINATE", path)
			}
		}
		if (x <= limit) != (y <= limit) {
			return mismatch("THRESHOLD_DECISION_DIFFERENCE", path)
		}
		d := ba - bb
		if bb > ba {
			d = bb - ba
		}
		if d > gateMaxULPs {
			return mismatch("ULP_LIMIT_EXCEEDED", path)
		}
		if d != 0 {
			*changes = append(*changes, GateDifference{Path: strings.Join(path, "/"), Recorded: x, Recomputed: y, ULPs: d, Threshold: limit})
		}
	case "int":
		var x, y big.Int
		if _, ok := x.SetString(a.s, 10); !ok {
			return mismatch("VALUE_DIFFERENCE", path)
		}
		if _, ok := y.SetString(b.s, 10); !ok || x.Cmp(&y) != 0 {
			return mismatch("VALUE_DIFFERENCE", path)
		}
	default: // str, bool, none
		if a.kind != b.kind || a.s != b.s {
			return mismatch("VALUE_DIFFERENCE", path)
		}
	}
	return nil
}

func numberValue(v *jv) (float64, bool) {
	if v == nil || v.kind != '0' {
		return 0, false
	}
	f, err := strconv.ParseFloat(v.s, 64)
	return f, err == nil && !math.IsInf(f, 0)
}

// pointID is the explanatory semantic id of a point, never a list offset.
func pointID(p *jv) *jv {
	out := &jv{kind: '{'}
	for _, k := range []string{"case", "family", "op", "sizes", "check", "budget"} {
		if v := jvMember(p, k); v != nil {
			out.keys = append(out.keys, k)
			out.vals = append(out.vals, v)
		}
	}
	return out
}

func gatePointText(p *jv) string {
	var parts []string
	for i, k := range p.keys {
		parts = append(parts, k+"="+p.vals[i].s)
	}
	return strings.Join(parts, ",")
}

// CompareGates compares one recorded and one recomputed BrightScript gate document under
// S07-REPLAY-2ULP-r1. Any difference outside the policy is an error whose text starts with
// the archived verifier's code (TYPE_DIFFERENCE, ULP_LIMIT_EXCEEDED, ...).
func CompareGates(recorded, recomputed []byte) (GateComparison, error) {
	a, e := decodeStrict("recorded", recorded, MaxDocumentBytes)
	if e != nil {
		return GateComparison{}, e
	}
	b, e := decodeStrict("recomputed", recomputed, MaxDocumentBytes)
	if e != nil {
		return GateComparison{}, e
	}
	return compareGateDocs(a, b)
}

func compareGateDocs(a, b *jv) (GateComparison, error) {
	want := []string{"gate", "notes", "points", "status"}
	if a.kind != '{' || b.kind != '{' || !slices.Equal(keySet(a), want) || !slices.Equal(keySet(b), want) {
		return GateComparison{}, gateMismatch{"UNKNOWN_GATE_SCHEMA"}
	}
	name := jvMember(a, "gate")
	if name.kind != '"' || !slices.Contains(bsGates, name.s) || jvMember(b, "gate").kind != '"' || jvMember(b, "gate").s != name.s {
		return GateComparison{}, gateMismatch{"UNKNOWN_GATE_ID"}
	}
	var changes []GateDifference // status and notes admit no rounding
	none := map[string]float64{}
	if err := compareGateValue(jvMember(a, "status"), jvMember(b, "status"), nil, none, &changes); err != nil {
		return GateComparison{}, err
	}
	if err := compareGateValue(jvMember(a, "notes"), jvMember(b, "notes"), nil, none, &changes); err != nil {
		return GateComparison{}, err
	}
	pa, pb := jvMember(a, "points"), jvMember(b, "points")
	if pa.kind != '[' || pb.kind != '[' {
		return GateComparison{}, gateMismatch{"POINT_LIST_REQUIRED"}
	}
	if len(pa.vals) != len(pb.vals) {
		return GateComparison{}, gateMismatch{"POINT_CARDINALITY_DIFFERENCE"}
	}
	out := GateComparison{Policy: GateComparePolicy, Gate: name.s, Verdict: jvMember(a, "status").s, Differences: []GateDifference{}}
	for i := range pa.vals {
		old, nw := pa.vals[i], pb.vals[i]
		if old.kind != '{' || nw.kind != '{' {
			return out, gateMismatch{"UNKNOWN_DERIVED_SCHEMA"}
		}
		if err := compareGateValue(pointID(old), pointID(nw), nil, none, &changes); err != nil {
			return out, err
		}
		allow, err := derivedFields(name.s, old)
		if err != nil {
			return out, err
		}
		if c := jvMember(old, "completed"); name.s == "REGRESSION-SWEEP" && c != nil && c.kind == 't' {
			var winners [2][]string
			for j, p := range []*jv{old, nw} {
				ex, top := jvMember(p, "exponents"), jvMember(p, "exponent")
				t, ok := numberValue(top)
				if ex == nil || ex.kind != '{' || !ok {
					return out, gateMismatch{"UNKNOWN_DERIVED_SCHEMA"}
				}
				best := math.Inf(-1)
				for _, v := range ex.vals {
					f, ok := numberValue(v)
					if !ok {
						return out, gateMismatch{"UNKNOWN_DERIVED_SCHEMA"}
					}
					best = max(best, f)
				}
				if t != best {
					return out, gateMismatch{"AGGREGATE_VALUE_DIFFERENCE"}
				}
				for k, v := range ex.vals {
					if f, _ := numberValue(v); f == t {
						winners[j] = append(winners[j], ex.keys[k])
					}
				}
				slices.Sort(winners[j])
			}
			if !slices.Equal(winners[0], winners[1]) {
				return out, gateMismatch{"AGGREGATE_SOURCE_CHANGED"}
			}
		}
		var rows []GateDifference
		if err := compareGateValue(old, nw, nil, allow, &rows); err != nil {
			return out, err
		}
		for _, r := range rows {
			r.Gate, r.Point = name.s, gatePointText(pointID(old))
			out.Differences = append(out.Differences, r)
		}
	}
	out.Status = "BITWISE_EQUAL"
	if len(out.Differences) > 0 {
		out.Status = "REPLAY_EQUIVALENT_WITH_DECLARED_ROUNDING"
	}
	return out, nil
}

// replayGateCompare pairs recorded and recomputed gate documents by gate id. The
// recomputed documents come from a separately authorized run of the archived verifier;
// the kit compares them, it does not recompute the gates from raw.
func replayGateCompare(x *replayEnv) *Error {
	if x.prof.Records == nil {
		return fail(KindInvalidInput, "RECORDS_REQUIRED", "replay#/records", nil)
	}
	load := func(role string) (map[string]*jv, *Error) {
		out := map[string]*jv{}
		for _, m := range x.membersByRole(role) {
			data, e := x.member(m.Path)
			if e != nil {
				return nil, e
			}
			v, e := decodeStrict(m.Path, data, MaxDocumentBytes)
			if e != nil {
				return nil, fail(KindInvalidInput, "RECORD_INVALID_"+e.Code, m.Path, nil)
			}
			g := jvMember(v, "gate")
			if g == nil || g.kind != '"' {
				return nil, fail(KindInvalidInput, "RECORD_INVALID", m.Path, nil)
			}
			if out[g.s] != nil {
				x.cons.Duplicate++
				if x.cons.First == "" {
					x.cons.First = "duplicate " + g.s
				}
				continue
			}
			out[g.s] = v
		}
		return out, nil
	}
	rec, e := load("recorded-gate")
	if e != nil {
		return e
	}
	re, e := load("recomputed-gate")
	if e != nil {
		return e
	}
	cons := newConsumer(x, x.prof.Records)
	gc := x.gate("gate-compare")
	order := slices.Clone(x.prof.Records)
	for _, id := range sortedKeys(rec) {
		if !slices.Contains(order, id) {
			order = append(order, id)
		}
	}
	for i, id := range order {
		if rec[id] == nil || !cons.take(id, i) {
			continue
		}
		other := re[id]
		if other == nil {
			gc.fail(id, "RECOMPUTED_GATE_MISSING", "다시 계산한 gate 문서가 없다")
			continue
		}
		res, err := compareGateDocs(rec[id], other)
		if err != nil {
			code, detail, _ := strings.Cut(err.Error(), ": ")
			gc.fail(id, code, fmt.Sprintf("%s %s", GateComparePolicy, detail))
			continue
		}
		gc.pass()
		x.gate("raw-recompute").recorded()
		if res.Status != "BITWISE_EQUAL" {
			x.gate("rounding").recorded() // admitted differences stay visible
		}
	}
	for id := range re {
		if rec[id] == nil {
			x.cons.Unused++
			if x.cons.First == "" {
				x.cons.First = "unused recomputed " + id
			}
		}
	}
	cons.close()
	x.recomp = Verdict{StatusCompleted, AssessUnresolved}
	return nil
}
