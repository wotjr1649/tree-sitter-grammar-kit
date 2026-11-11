package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func pinTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func pinTestApprove(t *testing.T, req *EvidencePinsRequest) {
	t.Helper()
	req.ApprovedPinSetSHA256 = digestHex(req.PinSet)
	req.Approval = eligibilityJSON(t, evidencePinApproval{Schema: EvidencePinApprovalSchema,
		PinSetSHA256: req.ApprovedPinSetSHA256, SubjectSHA256: digestHex(req.Subject), Reader: EvidencePinReader, Scope: EvidencePinScope})
	req.ApprovedApprovalSHA256 = digestHex(req.Approval)
}

func pinTestFixture(t *testing.T) EvidencePinsRequest {
	t.Helper()
	req := EvidencePinsRequest{Subject: []byte(`{"original":"subject A","recorded":"PASS"}`),
		Materials: map[string][]byte{"reference/tuple.json": []byte("rustc A; cargo A"), "native/runtime.json": []byte("runtime A")}}
	rows := []EvidencePinMaterial{}
	for _, item := range []struct{ name, role string }{{"reference/tuple.json", "reference-tuple"}, {"native/runtime.json", "runtime-manifest"}} {
		data := req.Materials[item.name]
		rows = append(rows, EvidencePinMaterial{Name: item.name, Role: item.role, Bytes: uint64(len(data)), SHA256: digestHex(data)})
	}
	req.PinSet = eligibilityJSON(t, evidencePinSet{Schema: EvidencePinSetSchema, Reader: EvidencePinReader, Materials: rows})
	pinTestApprove(t, &req)
	return req
}

func pinTestCode(res EvidencePinsResult, code string) bool {
	for _, f := range res.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

func TestEvidencePinsProductionSelectionDoesNotChangeHistory(t *testing.T) {
	ctx := pinTestContext(t)
	a := pinTestFixture(t)
	before := append([]byte(nil), a.Subject...)
	want, err := ResolveEvidencePins(ctx, a)
	if err != nil || want.Assessment != AssessPass || !want.Complete || want.Verified != 2 {
		t.Fatalf("%+v %v", want, err)
	}
	for _, generation := range []string{"A", "B", "C"} {
		current := pinTestFixture(t)
		current.Subject = []byte("new subject " + generation)
		var set evidencePinSet
		if err := json.Unmarshal(current.PinSet, &set); err != nil {
			t.Fatal(err)
		}
		data := []byte("rustc " + generation + "; cargo " + generation)
		current.Materials[set.Materials[0].Name] = data
		set.Materials[0].Bytes, set.Materials[0].SHA256 = uint64(len(data)), digestHex(data)
		current.PinSet = eligibilityJSON(t, set)
		pinTestApprove(t, &current)
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": current.ApprovedPinSetSHA256})
		selected, err := SelectEvidenceProductionPin(ctx, selection, current.PinSet)
		if err != nil || selected != current.ApprovedPinSetSHA256 {
			t.Fatal(selected, err)
		}
		got, err := ResolveEvidencePins(ctx, a)
		if err != nil || !reflect.DeepEqual(got, want) || !bytes.Equal(a.Subject, before) {
			t.Fatalf("historical result changed at %s: %+v %v", generation, got, err)
		}
		got, err = ResolveEvidencePins(ctx, current)
		if err != nil || got.Assessment != AssessPass {
			t.Fatal(got, err)
		}
	}
}

func TestEvidencePinsRejectForgery(t *testing.T) {
	for _, mode := range []string{"unapproved-set", "pin-substitution", "self-rehash", "sidecar-forgery", "sidecar-reuse", "material", "mixed-tuple", "approval-missing"} {
		t.Run(mode, func(t *testing.T) {
			req := pinTestFixture(t)
			code, assessment := "PIN_APPROVAL_MISMATCH", AssessFail
			switch mode {
			case "unapproved-set":
				req.ApprovedPinSetSHA256 = strings.Repeat("a", 64)
			case "pin-substitution":
				req.PinSet = append(req.PinSet, ' ')
			case "self-rehash":
				var set evidencePinSet
				if err := json.Unmarshal(req.PinSet, &set); err != nil {
					t.Fatal(err)
				}
				req.Materials[set.Materials[0].Name] = []byte("forged identity")
				set.Materials[0].Bytes, set.Materials[0].SHA256 = 15, digestHex([]byte("forged identity"))
				req.PinSet = eligibilityJSON(t, set)
				var approval evidencePinApproval
				if err := json.Unmarshal(req.Approval, &approval); err != nil {
					t.Fatal(err)
				}
				approval.PinSetSHA256 = digestHex(req.PinSet)
				req.Approval = eligibilityJSON(t, approval)
			case "sidecar-forgery":
				req.Approval = append(req.Approval, ' ')
			case "sidecar-reuse":
				req.Subject = []byte("different graph/profile/candidate")
				code = "PIN_SUBJECT_MISMATCH"
			case "material":
				req.Materials["native/runtime.json"] = []byte("runtime B")
				code = "PIN_MATERIAL_MISMATCH"
			case "mixed-tuple":
				req.Materials["reference/tuple.json"] = []byte("rustc A; cargo B")
				code = "PIN_MATERIAL_MISMATCH"
			case "approval-missing":
				req.ApprovedPinSetSHA256 = ""
				code, assessment = "PIN_APPROVAL_MISSING", AssessBlocked
			}
			got, err := ResolveEvidencePins(pinTestContext(t), req)
			if err != nil || got.Assessment != assessment || !pinTestCode(got, code) {
				t.Fatalf("forgery accepted: %+v %v", got, err)
			}
		})
	}
}

func TestEvidencePinsStrictBoundaries(t *testing.T) {
	for _, mode := range []string{"duplicate", "unknown", "missing", "null", "depth", "document-bytes", "unsupported-reader", "unsupported-schema", "unsupported-scope", "unknown-role", "empty", "alias", "traversal", "name-case", "unused", "declared-bytes", "missing-material"} {
		t.Run(mode, func(t *testing.T) {
			req := pinTestFixture(t)
			var set evidencePinSet
			if err := json.Unmarshal(req.PinSet, &set); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "duplicate":
				req.PinSet = []byte(`{"schema":"a","schema":"b"}`)
			case "unknown":
				req.PinSet = bytes.Replace(req.PinSet, []byte(`"reader":`), []byte(`"unknown":0,"reader":`), 1)
			case "missing":
				req.PinSet = []byte(`{"schema":"tsgk-evidence-pin-set/r1","reader":"tsgk-evidence-materials/r1"}`)
			case "null":
				req.PinSet = []byte(`{"schema":"tsgk-evidence-pin-set/r1","reader":"tsgk-evidence-materials/r1","materials":null}`)
			case "depth":
				req.PinSet = []byte(strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34))
			case "document-bytes":
				req.PinSet = bytes.Repeat([]byte(" "), int(MaxDocumentBytes)+1)
			case "unsupported-reader":
				set.Reader = "future-reader"
				req.PinSet = eligibilityJSON(t, set)
			case "unsupported-schema":
				set.Schema = "future-schema"
				req.PinSet = eligibilityJSON(t, set)
			case "unknown-role":
				set.Materials[0].Role = "arbitrary-code"
				req.PinSet = eligibilityJSON(t, set)
			case "empty":
				set.Materials = []EvidencePinMaterial{}
				req.PinSet = eligibilityJSON(t, set)
			case "alias":
				set.Materials = append(set.Materials, set.Materials[0])
				set.Materials[2].Name = strings.ToUpper(set.Materials[0].Name)
				req.PinSet = eligibilityJSON(t, set)
			case "traversal":
				set.Materials[0].Name = "../outside"
				req.PinSet = eligibilityJSON(t, set)
			case "name-case":
				req.Materials["REFERENCE/tuple.json"] = req.Materials["reference/tuple.json"]
				delete(req.Materials, "reference/tuple.json")
			case "unused":
				req.Materials["extra"] = []byte("unapproved")
			case "declared-bytes":
				set.Materials[0].Bytes = 1<<24 + 1
				req.PinSet = eligibilityJSON(t, set)
			case "missing-material":
				delete(req.Materials, "native/runtime.json")
			}
			pinTestApprove(t, &req)
			switch mode {
			case "unknown-role", "empty", "alias", "traversal", "declared-bytes":
				selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": req.ApprovedPinSetSHA256})
				if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, req.PinSet); err == nil {
					t.Fatal("malformed production descriptor accepted")
				}
			}
			if mode == "unsupported-scope" {
				var a evidencePinApproval
				_ = json.Unmarshal(req.Approval, &a)
				a.Scope = "full-semantic"
				req.Approval = eligibilityJSON(t, a)
				req.ApprovedApprovalSHA256 = digestHex(req.Approval)
			}
			got, err := ResolveEvidencePins(pinTestContext(t), req)
			if mode == "missing-material" {
				if err != nil || got.Assessment != AssessBlocked || !got.Complete || got.Verified != 1 || got.Required != 2 {
					t.Fatal(got, err)
				}
			} else if err == nil || got.Assessment == AssessPass || got.Complete {
				t.Fatalf("invalid input accepted: %+v %v", got, err)
			}
		})
	}
	for _, selection := range []string{`{"schema":"tsgk-evidence-pin-selection/r1","pin_set_sha256":"bad"}`, `{"schema":"future","pin_set_sha256":"` + strings.Repeat("a", 64) + `"}`, `{"schema":"tsgk-evidence-pin-selection/r1","pin_set_sha256":"` + strings.Repeat("a", 64) + `","other":"pin"}`} {
		if _, err := SelectEvidenceProductionPin(pinTestContext(t), []byte(selection), pinTestFixture(t).PinSet); err == nil {
			t.Fatal("invalid selection accepted")
		}
	}
}

type pinCancellationContext struct {
	context.Context
	cancel           context.CancelFunc
	checks, cancelAt int
}

func (c *pinCancellationContext) Err() error {
	c.checks++
	if c.cancelAt > 0 && c.checks == c.cancelAt {
		c.cancel()
	}
	return c.Context.Err()
}

func TestEvidencePinsInterruptionPreservesFailure(t *testing.T) {
	req := pinTestFixture(t)
	req.Materials["reference/tuple.json"] = []byte("forged")
	req.Materials["native/runtime.json"] = make([]byte, (1<<24)+1)
	got, err := ResolveEvidencePins(pinTestContext(t), req)
	if err == nil || asError(err).Kind != KindResourceLimit || got.Assessment != AssessFail || got.Complete || !pinTestCode(got, "PIN_MATERIAL_MISMATCH") || got.Work == 0 {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	cancel()
	got, err = ResolveEvidencePins(ctx, pinTestFixture(t))
	if err == nil || asError(err).Kind != KindCancelled || got.Complete {
		t.Fatal(got, err)
	}
	req = pinTestFixture(t)
	req.Materials["reference/tuple.json"] = []byte("forged")
	counted := &pinCancellationContext{Context: pinTestContext(t)}
	baseline, err := ResolveEvidencePins(counted, req)
	if err != nil || baseline.Assessment != AssessFail || !baseline.Complete {
		t.Fatal(baseline, err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	interrupted := &pinCancellationContext{Context: ctx, cancel: cancel, cancelAt: counted.checks}
	got, err = ResolveEvidencePins(interrupted, req)
	if err == nil || asError(err).Kind != KindCancelled || got.Assessment != AssessFail || !pinTestCode(got, "PIN_MATERIAL_MISMATCH") || got.Complete {
		t.Fatal(got, err)
	}
}

func TestEvidencePinSelectionBindsCandidate(t *testing.T) {
	req := pinTestFixture(t)
	selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": digestHex(req.PinSet)})
	for _, candidate := range [][]byte{nil, []byte("other pin set"), append(append([]byte(nil), req.PinSet...), ' ')} {
		if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, candidate); err == nil {
			t.Fatal("missing/substituted candidate accepted")
		}
	}
	malformed := []byte(`{"schema":"tsgk-evidence-pin-set/r1","reader":"tsgk-evidence-materials/r1","materials":[]}`)
	selection = eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": digestHex(malformed)})
	if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, malformed); err == nil {
		t.Fatal("empty candidate accepted")
	}
	var set evidencePinSet
	if err := json.Unmarshal(req.PinSet, &set); err != nil {
		t.Fatal(err)
	}
	set.Materials = make([]EvidencePinMaterial, ReplayOperations()["evidence-replay"].Files+1)
	tooMany := eligibilityJSON(t, set)
	selection = eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": digestHex(tooMany)})
	if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, tooMany); err == nil || asError(err).Code != "FILE_COUNT_LIMIT" {
		t.Fatal("material count boundary", err)
	}
}

func TestEvidencePinsArchive(t *testing.T) {
	root := filepath.Join("..", "contracts", "evidence-pins")
	selection, err := os.ReadFile(filepath.Join(root, "selection.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pointer struct {
		PinSetSHA256 string `json:"pin_set_sha256"`
	}
	if err := json.Unmarshal(selection, &pointer); err != nil {
		t.Fatal(err)
	}
	selectedSet, err := os.ReadFile(filepath.Join(root, "sets", pointer.PinSetSHA256+".json"))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectEvidenceProductionPin(pinTestContext(t), selection, selectedSet)
	if err != nil {
		t.Fatal(err)
	}
	sets, err := filepath.Glob(filepath.Join(root, "sets", "*.json"))
	if err != nil || len(sets) == 0 {
		t.Fatal(sets, err)
	}
	found := false
	for _, path := range sets {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sha := digestHex(data)
		if filepath.Base(path) != sha+".json" {
			t.Fatal("pin-set address mismatch", path)
		}
		if sha == selected {
			found = true
		}
		var set evidencePinSet
		if err := json.Unmarshal(data, &set); err != nil {
			t.Fatal(err)
		}
		req := EvidencePinsRequest{PinSet: data, Subject: []byte("independently reviewed test subject"), Materials: map[string][]byte{}}
		for _, pin := range set.Materials {
			b, err := os.ReadFile(filepath.Join(root, "objects", pin.SHA256))
			if err != nil {
				t.Fatal(err)
			}
			req.Materials[pin.Name] = b
		}
		pinTestApprove(t, &req)
		got, err := ResolveEvidencePins(pinTestContext(t), req)
		if err != nil || got.Assessment != AssessPass || !got.Complete || got.Verified != uint64(len(set.Materials)) {
			t.Fatal(got, err)
		}
	}
	if !found {
		t.Fatal("selected pin set not archived")
	}
}
