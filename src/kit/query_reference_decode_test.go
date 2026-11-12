package kit

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Pinned B4 ordering: non-exact head, schema, exact FAILED head or full response.
func referenceRowTwoPass(data []byte, r *run) (*instrumentedReference, *Error) {
	var head struct {
		Schema   string `json:"schema"`
		Status   string `json:"status"`
		Code     string `json:"code"`
		Complete bool   `json:"complete"`
	}
	if e := decodeWitnessDocument("reference-response", data, &head, false, r); e != nil {
		return nil, e
	}
	if head.Schema != instrumentedReferenceSchema || head.Status == StatusFailed && !head.Complete && head.Code != "" {
		if head.Schema == instrumentedReferenceSchema {
			if e := decodeWitnessDocument("reference-response", data, &head, true, r); e != nil {
				return nil, e
			}
		}
		return &instrumentedReference{Schema: head.Schema, Status: head.Status, Code: head.Code, Complete: head.Complete}, nil
	}
	var ref instrumentedReference
	if e := decodeWitnessDocument("reference-response", data, &ref, true, r); e != nil {
		return nil, e
	}
	return &ref, nil
}

func referenceRowParity(t *testing.T, data []byte) {
	t.Helper()
	for _, metered := range []bool{false, true} {
		old, oldErr := referenceRowTwoPass(data, witnessTestRun(t, metered))
		got, gotErr := decodeReferenceRow(data, witnessTestRun(t, metered))
		if (oldErr == nil) != (gotErr == nil) || oldErr != nil && (oldErr.Kind != gotErr.Kind || oldErr.Code != gotErr.Code || oldErr.Code != "JSON_MISSING_FIELD" && oldErr.Path != gotErr.Path) {
			t.Fatalf("two-pass/single reference parity metered=%t: old=%v single=%v input=%q", metered, oldErr, gotErr, data[:min(len(data), 200)])
		}
		if oldErr == nil && !reflect.DeepEqual(old, got) {
			t.Fatalf("two-pass/single reference value mismatch metered=%t", metered)
		}
	}
}

func TestReferenceRowDifferential(t *testing.T) {
	paths, err := filepath.Glob("../testdata/query-reference/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("stored references: %v %v", paths, err)
	}
	rng := rand.New(rand.NewPCG(362, 5))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			referenceRowParity(t, data)
			for i := 0; i < 32; i++ {
				t.Run(fmt.Sprintf("truncate-%02d", i), func(t *testing.T) {
					referenceRowParity(t, data[:i*len(data)/32])
				})
			}
			for i := 0; i < 32; i++ {
				t.Run(fmt.Sprintf("mutation-%02d", i), func(t *testing.T) {
					mutated := bytes.Clone(data)
					mutated[rng.IntN(len(data))] = byte(rng.Uint32())
					referenceRowParity(t, mutated)
				})
			}
		})
	}
}

func TestReferenceRowErrorPriority(t *testing.T) {
	const head = `"schema":"tsgk-instrumented-query-reference/r1","status":"FAILED","code":"CST_LIMIT","complete":false`
	cases := []string{
		`{` + head + `}`, `{` + head + `,"unknown":{"schema":0}}`,
		`{"unknown":null,` + head + `}`, `{` + head + `,"unknown":0,"code":0}`,
		`{"tree":null,"schema":"wrong","status":"COMPLETED","code":"","complete":true}`,
		`{"tree":null,"schema":"wrong","status":"COMPLETED","code":"","complete":null}`,
		`{"unknown":0,"schema":false,"status":"FAILED","code":"CST_LIMIT","complete":false}`,
		`{"schema":"wrong","status":{},"code":"","complete":true}`,
		`{"schema":"wrong","status":"FAILED","code":[],"complete":false}`,
		`{"schema":"wrong","status":"FAILED","code":"x","complete":0}`,
		`{"schema":"wrong","status":"COMPLETED","code":""}`,
		`{"schema":"wrong","status":"FAILED","code":"x","complete":false,"\u0073chema":"x"}`,
		`{"schema":null,"unknown":`, `{` + head + `} {}`, `{` + head + `} x`,
		`{"\u0073chema":"wrong","status":"FAILED","code":"x","complete":false}`,
		`{"schema":"wrong","status":"FAILED","code":"x","complete":false,"unknown":` + strings.Repeat("[", 33) + strings.Repeat("]", 33) + `}`,
		`null`, `false`, `[]`, `["x"]`, `0`, `"x"`, `{}`, ``,
	}
	for i, data := range cases {
		t.Run(fmt.Sprintf("case-%02d", i), func(t *testing.T) { referenceRowParity(t, []byte(data)) })
	}
	data, err := os.ReadFile("../testdata/query-reference/eq-0.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, replacement := range []string{`"complete":null`, `"complete":0`, `"complete":[]`, `"complete":{}`} {
		t.Run(replacement, func(t *testing.T) {
			// Earlier full-shape failure must not hide the later non-exact head error.
			bad := bytes.Replace(data, []byte(`"nodes":[`), []byte(`"unknown":0,"nodes":[`), 1)
			bad = bytes.Replace(bad, []byte(`"complete":true`), []byte(replacement), 1)
			referenceRowParity(t, bad)
		})
	}
	// A later head error beats schema mismatch and the earlier full-shape failure.
	bad := bytes.Replace(data, []byte(instrumentedReferenceSchema), []byte("wrong"), 1)
	bad = bytes.Replace(bad, []byte(`"complete":true`), []byte(`"complete":null`), 1)
	referenceRowParity(t, bad)
}

func TestReferenceRowLedger(t *testing.T) {
	data, err := os.ReadFile("../testdata/query-reference/eq-0.json")
	if err != nil {
		t.Fatal(err)
	}
	oldRun, newRun := witnessTestRun(t, true), witnessTestRun(t, true)
	if _, e := referenceRowTwoPass(data, oldRun); e != nil {
		t.Fatal(e)
	}
	if _, e := decodeReferenceRow(data, newRun); e != nil {
		t.Fatal(e)
	}
	if newRun.eligibility.logical >= oldRun.eligibility.logical || newRun.eligibility.work >= oldRun.eligibility.work {
		t.Fatalf("duplicate row pass retained: old=%+v new=%+v", oldRun.eligibility, newRun.eligibility)
	}
	// The shared ledger must still reject before the combined decode exceeds either cap.
	for _, work := range []bool{false, true} {
		r := witnessTestRun(t, true)
		if work {
			r.eligibility.work = eligibilityWorkLimit - newRun.eligibility.work + 1
		} else {
			r.eligibility.logical = eligibilityLogicalLimit - newRun.eligibility.logical + 1
		}
		if _, e := decodeReferenceRow(data, r); e == nil || e.Kind != KindResourceLimit || e.Code != "ELIGIBILITY_WORK_LIMIT" {
			t.Fatalf("combined row exceeded ledger work=%t: %v", work, e)
		}
	}
}

func FuzzReferenceRow(f *testing.F) {
	data, err := os.ReadFile("../testdata/query-reference/eq-0.json")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte(`{"schema":"tsgk-instrumented-query-reference/r1","status":"FAILED","code":"CST_LIMIT","complete":false}`))
	f.Add([]byte(`{"schema":"wrong","status":null,"code":"","complete":false}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) <= MaxDocumentBytes+1 {
			referenceRowParity(t, data)
		}
	})
}
