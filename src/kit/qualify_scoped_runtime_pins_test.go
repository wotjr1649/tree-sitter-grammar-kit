package kit

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func qualifyScopedAccepted(t *testing.T, label string, pin *EvidencePinsRequest) (*qualifier, QualWorkload, QualPlatform) {
	t.Helper()
	root, _ := acceptedReplayFixture(t, nil)
	materials := loggerPinMaterials(t, label)
	tuple := loggerTuple(t, materials)
	var profile map[string]any
	registrationBytes, err := os.ReadFile(filepath.Join(root, "profile.json"))
	if err != nil || json.Unmarshal(registrationBytes, &profile) != nil {
		t.Fatal("accepted profile")
	}
	for _, name := range []string{"result.json", "responses/00000-" + acceptedFixtureID(t) + ".json"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if json.Unmarshal(data, &value) != nil {
			t.Fatalf("decode %s", name)
		}
		var rawFields map[string]json.RawMessage
		if name == "result.json" {
			if json.Unmarshal(data, &rawFields) != nil {
				t.Fatal("decode raw result")
			}
		}
		setScopedManifest(value, tuple.ManifestSHA256)
		if name == "result.json" {
			if build, ok := value["build"].(map[string]any); ok {
				build["runtime_commit"] = tupleManifestCommit(t, materials)
			}
		} else if process, ok := value["process"].(map[string]any); ok {
			process["stdout_bytes"] = len(jsonBytes(t, value)) + 4
		}
		encoded := jsonBytes(t, value)
		if name == "result.json" {
			_ = json.Unmarshal(encoded, &rawFields)
			rawFields["operation"] = rawFieldsFromOriginal(data)["operation"]
			encoded = jsonBytes(t, rawFields)
		}
		if name == "result.json" {
			var top map[string]any
			if json.Unmarshal(encoded, &top) != nil {
				t.Fatal("decode result")
			}
			if cases, ok := top["cases"].([]any); ok && len(cases) > 0 {
				if process, ok := cases[0].(map[string]any)["process"].(map[string]any); ok {
					responsePath := filepath.Join(root, "responses", "00000-"+acceptedFixtureID(t)+".json")
					response, e := os.ReadFile(responsePath)
					if e != nil {
						t.Fatal(e)
					}
					process["stdout_bytes"] = len(response) + 4
				}
				encoded = jsonBytes(t, top)
				var rawUpdated map[string]json.RawMessage
				_ = json.Unmarshal(encoded, &rawUpdated)
				rawUpdated["operation"] = rawFieldsFromOriginal(data)["operation"]
				encoded = jsonBytes(t, rawUpdated)
			}
		}
		if err := os.WriteFile(path, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
	host := t.TempDir()
	if err := os.CopyFS(filepath.Join(host, "incremental", "reader-control"), os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(host, "run-identity.json"), scopedRunIdentity(t), 0600); err != nil {
		t.Fatal(err)
	}
	q, w, p := acceptedReaderQualifier(t, filepath.Join(host, "incremental", "reader-control"), false)
	h := q.hosts[p.ID]
	if h.x.r.eligibility == nil {
		h.x.r.eligibility = &eligibilityBudget{r: h.x.r}
	}
	if pin == nil {
		raw, e := os.ReadFile(filepath.Join(host, "run-identity.json"))
		if e != nil {
			t.Fatal(e)
		}
		pin = scopedPinRequest(t, raw, materials, runtimeAcceptedReader)
	}
	h.runtimePins, h.runtimeSubject = pin, append([]byte(nil), pin.Subject...)
	_, runtime, e := verifyLoggerRuntime(pin, h.runtimeSubject, h.x.r.eligibility)
	if e != nil || runtime == nil {
		t.Fatalf("qualify logger base: %+v %v", pin, e)
	}
	h.loggerRuntime = runtime
	return q, w, p
}

func qualifyScopedCST(t *testing.T, label string, pin *EvidencePinsRequest) (*qualifier, QualWorkload, QualPlatform) {
	t.Helper()
	request := scopedCSTReplay(t, label)
	materials := loggerPinMaterials(t, label)
	tuple := loggerTuple(t, materials)
	invBytes, err := os.ReadFile(filepath.Join("..", "contracts", "qualification-c1.json"))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := ParseQualificationInventory(invBytes)
	if err != nil {
		t.Fatal(err)
	}
	var w QualWorkload
	for _, role := range inv.ExtraRoles {
		for _, workload := range role.Workloads {
			if workload.Set == "s05-csharp-symbols-switch" {
				w = workload
			}
		}
	}
	if w.Set == "" {
		t.Fatal("CST qualification registration missing")
	}
	profileBytes, err := os.ReadFile(filepath.Join(request.Root, "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := ParseIncrementalProfile(profileBytes)
	if err != nil {
		t.Fatal(err)
	}
	w.Grammar = profile.Grammar
	w.RuntimeCommit = tupleManifestCommit(t, materials)
	inv.Platforms, inv.Routes, inv.Cells = inv.Platforms[:2], inv.Routes[:1], 2
	inv.ExtraRoles = []QualRole{{ID: "csharp-symbols", Role: "owned", Status: RoleExecuted, Reason: "runtime reader fixture", Platforms: []string{inv.Platforms[0].ID}, NotApplicable: map[string]string{}, Deferred: map[string]string{inv.Platforms[1].ID: "deferred"}, Workloads: []QualWorkload{w}}}
	invBytes, err = json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	host := t.TempDir()
	setRoot := filepath.Join(host, "incremental", w.Set)
	if err := os.CopyFS(setRoot, os.DirFS(request.Root)); err != nil {
		t.Fatal(err)
	}
	var paths []string
	err = filepath.WalkDir(setRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if filepath.Ext(path) != ".json" || filepath.Base(path) == "profile.json" {
			continue
		}
		data, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		var value any
		if json.Unmarshal(data, &value) != nil {
			t.Fatalf("decode %s", path)
		}
		setScopedManifest(value, tuple.ManifestSHA256)
		if relative, _ := filepath.Rel(setRoot, path); relative == "result.json" {
			if top, ok := value.(map[string]any); ok {
				if build, ok := top["build"].(map[string]any); ok {
					build["runtime_commit"] = tupleManifestCommit(t, materials)
				}
			}
		}
		encoded := jsonBytes(t, value)
		if relative, _ := filepath.Rel(setRoot, path); relative == "result.json" {
			var original, updated map[string]json.RawMessage
			if json.Unmarshal(data, &original) != nil || json.Unmarshal(encoded, &updated) != nil {
				t.Fatal("decode CST result operation")
			}
			updated["operation"] = original["operation"]
			encoded = jsonBytes(t, updated)
		}
		if err := os.WriteFile(path, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
	updateCSTQualificationStdout(t, setRoot)
	id := scopedRunIdentity(t)
	if err := os.WriteFile(filepath.Join(host, "run-identity.json"), id, 0600); err != nil {
		t.Fatal(err)
	}
	run, e := startRun(ctxT(t), QualificationLimits().Wall)
	if e != nil {
		t.Fatal(e)
	}
	q := &qualifier{r: run, lim: QualificationLimits(), candidate: strings.Repeat("c", 40), inv: inv, hosts: map[string]*qhost{}}
	p := inv.Platforms[0]
	h, e := q.openHost(p, host, nil)
	if e != nil {
		t.Fatal(e)
	}
	q.hosts[p.ID] = h
	t.Cleanup(func() { q.close(); run.cancel() })
	if h.x.r.eligibility == nil {
		h.x.r.eligibility = &eligibilityBudget{r: h.x.r}
	}
	if pin == nil {
		pin = scopedPinRequest(t, id, materials, runtimePreprocessCSTReader)
	}
	h.runtimePins, h.runtimeSubject = pin, append([]byte(nil), id...)
	_, runtime, e := verifyLoggerRuntime(pin, id, h.x.r.eligibility)
	if e != nil || runtime == nil {
		t.Fatalf("qualification CST logger base: %v", e)
	}
	h.loggerRuntime = runtime
	return q, w, p
}

func updateCSTQualificationStdout(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "result.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]any
	var original map[string]json.RawMessage
	if json.Unmarshal(data, &original) != nil {
		t.Fatal("decode original CST operation")
	}
	if json.Unmarshal(data, &top) != nil {
		t.Fatal("decode result")
	}
	for _, item := range top["cases"].([]any) {
		c := item.(map[string]any)
		id := c["id"].(string)
		if process, ok := c["process"].(map[string]any); ok {
			raw, e := os.ReadFile(filepath.Join(root, "responses", "00000-"+id+".json"))
			if e != nil {
				t.Fatal(e)
			}
			process["stdout_bytes"] = len(raw) + 4
		}
		if cst, ok := c["preprocess_cst"].(map[string]any); ok {
			if receipt, ok := cst["primary"].(map[string]any); ok {
				if process, ok := receipt["process"].(map[string]any); ok {
					raw, e := os.ReadFile(filepath.Join(root, "primary", "responses", "00000-"+id+".json"))
					if e != nil {
						t.Fatal(e)
					}
					process["stdout_bytes"] = len(raw) + 4
				}
			}
		}
	}
	updated := jsonBytes(t, top)
	var rawUpdated map[string]json.RawMessage
	if json.Unmarshal(updated, &rawUpdated) != nil {
		t.Fatal("decode updated CST operation")
	}
	rawUpdated["operation"] = original["operation"]
	if err := os.WriteFile(path, jsonBytes(t, rawUpdated), 0600); err != nil {
		t.Fatal(err)
	}
}

func scopedRunIdentity(t *testing.T) []byte {
	t.Helper()
	candidate := strings.Repeat("c", 40)
	data, err := json.Marshal(RunIdentity{Schema: RunIdentitySchema, Repository: "o/r", Workflow: "fixture", RunID: "1", RunAttempt: "1", Event: "push", SHA: candidate, HeadSHA: candidate, Checkout: candidate, Job: "test", RunnerOS: "Windows", RunnerArch: "X64", GOOS: "windows", GOARCH: "amd64", GoVersion: "fixture", EvidenceMode: ModeNewRun})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func jsonBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func rawFieldsFromOriginal(data []byte) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	return fields
}

func acceptedFixtureID(t *testing.T) string {
	t.Helper()
	req, _, _ := acceptedFixture(t, 12)
	return req.Case.ID
}

func TestQualificationAcceptedAndCSTRuntimePinsProductionIndependence(t *testing.T) {
	t.Parallel()
	for _, reader := range []string{runtimeAcceptedReader, runtimePreprocessCSTReader} {
		t.Run(reader, func(t *testing.T) {
			build := func() QualSet {
				var q *qualifier
				var w QualWorkload
				var p QualPlatform
				if reader == runtimeAcceptedReader {
					q, w, p = qualifyScopedAccepted(t, "qualify-scoped-A", nil)
				} else {
					q, w, p = qualifyScopedCST(t, "qualify-scoped-A", nil)
				}
				got, err := q.evalSymbolSet(w, p)
				if err != nil {
					t.Fatalf("qualify set: %v", err)
				}
				wantReader := reader
				if got.RuntimePins == nil || got.RuntimePins.Reader != wantReader || got.RuntimePins.Assessment != AssessPass || !got.RuntimePins.Complete || got.Mechanism != AssessPass {
					t.Fatalf("historical qualification set: %+v", got)
				}
				return got.QualSet
			}
			want := build()
			for _, label := range []string{"B", "C"} {
				production := scopedPinRequest(t, scopedRunIdentity(t), loggerPinMaterials(t, "qualify-production-"+label), reader)
				selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": production.ApprovedPinSetSHA256})
				if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, production.PinSet); err != nil {
					t.Fatal(err)
				}
				if got := build(); !reflect.DeepEqual(got, want) {
					t.Fatalf("production %s changed historical qualification: %+v", label, got)
				}
			}
		})
	}
}

func TestQualificationScopedRuntimePublicAdmissionAndHostIsolation(t *testing.T) {
	req, materials := qualificationLoggerRequest(t, "qualify-r13", "valid")
	if len(req.Hosts) == 0 || req.Hosts[0].RuntimePins == nil {
		t.Fatal("logger fixture missing host opt-in")
	}
	subject := req.Hosts[0].RuntimePins.Subject
	req.Hosts[0].RuntimePins = scopedPinRequest(t, subject, materials, runtimeAcceptedReader)
	got, err := Qualify(pinTestContext(t), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResultSchema != QualificationScopedRuntimeResultSchema || len(got.RuntimePins) != 1 || got.RuntimePins["windows-amd64"].Assessment != AssessPass {
		t.Fatalf("r13 admission or host isolation: schema=%s runtime=%+v", got.ResultSchema, got.RuntimePins)
	}
}

func TestQualificationScopedRuntimeRejectsReaderTupleAfterLoggerAdmission(t *testing.T) {
	for _, reader := range []string{runtimeAcceptedReader, runtimePreprocessCSTReader} {
		t.Run(reader, func(t *testing.T) {
			id := scopedRunIdentity(t)
			materials := loggerPinMaterials(t, "qualify-scope-forgery")
			approved := scopedPinRequest(t, id, materials, reader)
			scopes := runtimeReaders{}
			if json.Unmarshal(approved.Materials[runtimeReadersName], &scopes) != nil {
				t.Fatal("decode readers")
			}
			scopes.TupleSHA256 = strings.Repeat("0", 64)
			materials = clonePinMaterials(approved.Materials)
			materials[runtimeReadersName] = eligibilityJSON(t, scopes)
			forged := scopedPinRequestWithMaterials(t, id, materials)
			var q *qualifier
			var workload QualWorkload
			var platform QualPlatform
			if reader == runtimeAcceptedReader {
				q, workload, platform = qualifyScopedAccepted(t, "qualify-scope-forgery", forged)
			} else {
				q, workload, platform = qualifyScopedCST(t, "qualify-scope-forgery", forged)
			}
			got, err := q.evalSymbolSet(workload, platform)
			if err != nil {
				t.Fatal(err)
			}
			if got.RuntimePins == nil || got.RuntimePins.Assessment != AssessFail || got.Mechanism != AssessFail {
				t.Fatalf("reader tuple forgery passed after logger admission: %+v", got)
			}
		})
	}
}
