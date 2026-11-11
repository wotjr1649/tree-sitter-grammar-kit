package queryregistrycore

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func smallScope() *Validated {
	coordinates := []Coordinate{
		{Structural, "plain", "windows-amd64", "windows/amd64", "primary", "s", "a/b", "λ", 0, "incremental", "ctx"},
		{Structural, "plain", "windows-amd64", "windows/amd64", "primary", "s", "a/b", "λ", 1, "incremental", "ctx"},
		{Structural, "plain", "windows-amd64", "windows/amd64", "primary", "s", "a/b", "λ", 1, "fresh", "ctx"},
		{Predicate, "owned-plain", "windows-amd64", "windows/amd64", "predicate-probe", "predicate/r1", "evaluated-empty", "q0", 0, "incremental", "ctx"},
		{Control, "owned-plain", "windows-amd64", "windows/amd64", "predicate-probe", "predicate/r1", "partial-structural", "q0", 0, "incremental", "ctx"},
	}
	v := &Validated{registry: Registry{Namespace: strings.Repeat("1", 64), RequiredSets: Sets(coordinates), Probes: Roster()}, sha: strings.Repeat("2", 64), coordinates: coordinates, byID: map[string]Coordinate{}}
	for _, c := range coordinates {
		v.byID[c.ID(v.Namespace())] = c
	}
	return v
}

func shard(v *Validated) Shard {
	s := Shard{ID: "s1", RegistrySHA256: v.sha}
	for _, c := range v.coordinates {
		s.Judgments = append(s.Judgments, Judgment{ID: c.ID(v.Namespace()), Coordinate: c, State: "UNPROCESSED"})
	}
	return s
}

func TestCoordinateCanonicalAndAliases(t *testing.T) {
	c := smallScope().coordinates[0]
	want := `["STRUCTURAL_ONLY","plain","windows-amd64","windows/amd64","primary","s","a/b","λ",0,"incremental","ctx"]`
	if string(c.Canonical()) != want {
		t.Fatalf("canonical UTF-8: %s", c.Canonical())
	}
	namespace := strings.Repeat("1", 64)
	preimage := `["tsgk-query-obligation-id/r1","` + namespace + `","STRUCTURAL_ONLY","plain","windows-amd64","windows/amd64","primary","s","a/b","λ",0,"incremental","ctx"]`
	if c.ID(namespace) != "qsem1:"+Digest([]byte(preimage)) {
		t.Fatal("ID preimage changed")
	}
	for _, changed := range []Coordinate{
		{Structural, "plain", "windows-amd64", "windows/amd64", "primary", "s", "a", "b/λ", 0, "incremental", "ctx"},
		{Structural, "plain", "linux-amd64", "linux/amd64", "primary", "s", "a/b", "λ", 0, "incremental", "ctx"},
		{Structural, "plain", "windows-amd64", "windows/amd64", "primary", "s", "a/b", "λ", 0, "incremental", "other-history"},
	} {
		if changed.ID(namespace) == c.ID(namespace) {
			t.Fatal("coordinate alias")
		}
	}
	if c.ID(namespace) == c.ID(strings.Repeat("3", 64)) {
		t.Fatal("namespace ignored")
	}
}

func TestExactShardMembership(t *testing.T) {
	v := smallScope()
	s := shard(v)
	if err := v.VerifyExact([]Shard{s}); err != nil {
		t.Fatal(err)
	}
	split := []Shard{{"s1", v.sha, s.Judgments[:2]}, {"s2", v.sha, s.Judgments[2:]}}
	if err := v.VerifyExact(split); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"zero", "gap", "overlap", "duplicate-shard", "old-registry", "same-count-case", "host-subject", "initial-fresh", "edit-fresh-missing", "id-forgery", "extra", "history"} {
		t.Run(name, func(t *testing.T) {
			s := shard(v)
			shards := []Shard{s}
			switch name {
			case "zero":
				shards = nil
			case "gap":
				shards[0].Judgments = s.Judgments[1:]
			case "overlap":
				shards = append(shards, Shard{"s2", v.sha, s.Judgments[:1]})
			case "duplicate-shard":
				shards = append(shards, s)
			case "old-registry":
				shards[0].RegistrySHA256 = strings.Repeat("0", 64)
			case "same-count-case":
				shards[0].Judgments[0].Coordinate.Case = "another"
			case "host-subject":
				shards[0].Judgments[0].Coordinate.Subject = "linux/amd64"
			case "initial-fresh":
				shards[0].Judgments[0].Coordinate.TreeRole = "fresh"
			case "edit-fresh-missing":
				shards[0].Judgments[2] = shards[0].Judgments[1]
			case "id-forgery":
				shards[0].Judgments[0].ID = "qsem1:" + strings.Repeat("0", 64)
			case "extra":
				shards[0].Judgments = append(s.Judgments, s.Judgments[0])
			case "history":
				shards[0].Judgments[0].Coordinate.Context = "other-history"
			}
			if v.VerifyExact(shards) == nil {
				t.Fatal("invalid shard union accepted")
			}
		})
	}
}

func checkCounters(t *testing.T, c Coverage, required, verified, failed, blocked, unprocessed uint64) {
	t.Helper()
	var r, v, f, b, u uint64
	for _, row := range c.Counters {
		if row.Required != row.Verified+row.Failed+row.Blocked+row.Unprocessed {
			t.Fatal("counter conservation", row)
		}
		r += row.Required
		v += row.Verified
		f += row.Failed
		b += row.Blocked
		u += row.Unprocessed
	}
	if r != required || v != verified || f != failed || b != blocked || u != unprocessed || c.FullScopeComplete {
		t.Fatalf("coverage: %+v", c)
	}
}

func TestTypedCoverageMissingPartialAndFailure(t *testing.T) {
	v := smallScope()
	out, err := v.Fold(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	checkCounters(t, out, 5, 0, 0, 5, 0)
	out, err = v.Fold(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	checkCounters(t, out, 5, 0, 0, 0, 5)
	s := shard(v)
	s.Judgments = s.Judgments[:3]
	s.Judgments[0].State = "VERIFIED"
	s.Judgments[1].State, s.Judgments[1].Code = "FAILED", "METADATA_MISMATCH"
	s.Judgments[2].State, s.Judgments[2].Code = "FAILED", "LATER_FAILURE"
	// Consumption order differs from coordinate order: the first confirmed failure survives.
	s.Judgments[1], s.Judgments[2] = s.Judgments[2], s.Judgments[1]
	out, err = v.Fold([]Shard{s}, false)
	if err != nil {
		t.Fatal(err)
	}
	checkCounters(t, out, 5, 1, 2, 0, 2)
	if out.Verdict != "FAIL" || out.Code != "LATER_FAILURE" {
		t.Fatal("first failure lost", out)
	}
	out, err = v.Fold([]Shard{s}, true)
	if err != nil {
		t.Fatal(err)
	}
	checkCounters(t, out, 5, 1, 2, 2, 0)
	s.Judgments = s.Judgments[1:]
	out, err = v.Fold([]Shard{s}, true)
	if err != nil || out.Verdict != "FAIL" || out.Code != "LATER_FAILURE" {
		t.Fatal("missing input masked failure", out, err)
	}
	checkCounters(t, out, 5, 0, 2, 3, 0)
}

func TestTypedControlSubjectAndIncompleteAssets(t *testing.T) {
	v := smallScope()
	s := shard(v)
	for i := range s.Judgments {
		s.Judgments[i].State = "VERIFIED"
	}
	s.Judgments[4].Subject = &Subject{"RESOURCE_LIMIT", "CAPTURE_LIMIT", "STRUCTURAL", true, "PRESERVED_4", "PRESERVED_STRUCTURAL"}
	out, err := v.Fold([]Shard{s}, false)
	if err != nil {
		t.Fatal(err)
	}
	checkCounters(t, out, 5, 5, 0, 0, 0)
	for _, row := range out.Counters {
		if row.Kind == Predicate && row.Verified != 1 {
			t.Fatal("controls counted as predicate positives")
		}
	}
	for _, name := range []string{"null-empty", "evaluation", "partial", "code", "subject-status", "reclassify", "unavailable", "forged-evaluated"} {
		t.Run(name, func(t *testing.T) {
			v := smallScope()
			s := shard(v)
			j := &s.Judgments[4]
			j.State = "VERIFIED"
			j.Subject = &Subject{"RESOURCE_LIMIT", "CAPTURE_LIMIT", "STRUCTURAL", true, "PRESERVED_4", "PRESERVED_STRUCTURAL"}
			switch name {
			case "null-empty":
				j.Subject.Expected = "[]"
			case "evaluation":
				j.Subject.Evaluation = "EVALUATED"
			case "partial":
				j.Subject.Partial = false
			case "code":
				j.Subject.Code = ""
			case "subject-status":
				j.Subject.Status = "COMPLETED"
			case "reclassify":
				s.Judgments[0].State = "VERIFIED"
				s.Judgments[0].Subject = j.Subject
			case "unavailable":
				v.registry.Assets = MissingAssets()
				s.Judgments[0].State = "VERIFIED"
			case "forged-evaluated":
				s.Judgments[0].State = "EVALUATED"
			}
			if _, err := v.Fold([]Shard{s}, false); err == nil {
				t.Fatal("invalid coverage accepted")
			}
		})
	}
	v.registry.Assets = MissingAssets()
	out, err = v.Fold(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	checkCounters(t, out, 5, 0, 0, 4, 1)
	out, err = v.Fold([]Shard{shard(v)}, false)
	if err != nil {
		t.Fatal(err)
	}
	checkCounters(t, out, 5, 0, 0, 4, 1)
}

func TestParseStrictBoundaries(t *testing.T) {
	for _, data := range [][]byte{
		[]byte(`{}`), []byte(`null`), []byte(`[]`), []byte(`{"schema":1}`),
		bytes.Repeat([]byte{' '}, MaxBytes+1),
		[]byte(`{"schema":"x","schema":"y"}`),
		[]byte(`{"schema":` + strings.Repeat("[", 65) + `0` + strings.Repeat("]", 65) + `}`),
	} {
		if _, err := Parse(data); err == nil {
			t.Fatalf("invalid shape accepted: %.80s", data)
		} else {
			var typed *Fault
			if !errors.As(err, &typed) || typed.Code == "" || typed.Path != Path {
				t.Fatal("untyped registry failure", err)
			}
		}
	}
}

// A later contradiction must not erase a failure already confirmed by a well-formed judgment,
// and tampering alone is a FAIL with its own code (#362 review M2).
func TestFoldKeepsConfirmedFailureOnContradiction(t *testing.T) {
	v := smallScope()
	first := shard(v)
	first.Judgments = first.Judgments[:1]
	first.Judgments[0].State, first.Judgments[0].Code = "FAILED", "METADATA_MISMATCH"
	overlap := Shard{ID: "s2", RegistrySHA256: v.sha, Judgments: []Judgment{first.Judgments[0]}}
	out, err := v.Fold([]Shard{first, overlap}, false)
	if err == nil || out.Verdict != "FAIL" || out.Code != "METADATA_MISMATCH" || out.FullScopeComplete {
		t.Fatalf("confirmed failure lost on overlap: %+v %v", out, err)
	}
	bad := shard(v)
	bad.Judgments[1].State = "FAILED" // no code: the contradiction itself
	out, err = v.Fold([]Shard{bad}, false)
	if err == nil || out.Verdict != "FAIL" || out.Code != "REGISTRY_FAILURE_CODE" {
		t.Fatalf("tampering not reported as FAIL: %+v %v", out, err)
	}
	forged := shard(v)
	forged.Judgments[0].ID = strings.Repeat("f", 64)
	forged.Judgments[0].State, forged.Judgments[0].Code = "FAILED", "FORGED_CODE" // a forged code is never confirmed
	later := shard(v)
	later.ID = "s3"
	later.Judgments[2].State, later.Judgments[2].Code = "FAILED", "AFTER_DEFECT"
	out, err = v.Fold([]Shard{forged, later}, false)
	if err == nil || out.Verdict != "FAIL" || out.Code != "REGISTRY_UNDECLARED_COORDINATE" {
		t.Fatalf("failure after a defect counted as confirmed: %+v %v", out, err)
	}
}
