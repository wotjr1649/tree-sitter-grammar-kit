package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

func loggerPinMaterials(t *testing.T, label string) map[string][]byte {
	t.Helper()
	all := runtimePinMaterials(t)
	m := map[string][]byte{}
	for _, row := range runtimePinRolesR1 {
		m[row.name] = slices.Clone(all[row.name])
	}
	var manifest eligibilityRuntimeManifest
	if err := json.Unmarshal(m["native/runtime-manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Commit, manifest.Note = fxRuntime, "synthetic logger runtime "+label
	m["native/runtime-manifest.json"] = eligibilityJSON(t, manifest)
	runtimePinTuple(t, m, func(tuple *eligibilityRuntimeTuple) {
		tuple.ManifestSHA256 = digestHex(m["native/runtime-manifest.json"])
	})
	return m
}

func loggerPinRequest(t *testing.T, subject []byte, materials map[string][]byte) *EvidencePinsRequest {
	t.Helper()
	set := evidencePinSet{Schema: EvidencePinSetSchema, Reader: EvidencePinReader}
	for _, row := range runtimePinRolesR1 {
		if data, ok := materials[row.name]; ok {
			set.Materials = append(set.Materials, EvidencePinMaterial{Name: row.name, Role: row.role, Bytes: uint64(len(data)), SHA256: digestHex(data)})
		}
	}
	req := &EvidencePinsRequest{Subject: slices.Clone(subject), Materials: materials, PinSet: eligibilityJSON(t, set)}
	pinTestApprove(t, req)
	return req
}

func loggerTuple(t *testing.T, m map[string][]byte) eligibilityRuntimeTuple {
	t.Helper()
	var tuple eligibilityRuntimeTuple
	if err := json.Unmarshal(m["native/runtime-tuple.json"], &tuple); err != nil {
		t.Fatal(err)
	}
	return tuple
}

func loggerProducer(p map[string]any, tuple eligibilityRuntimeTuple) {
	p["reuse_observation"] = map[string]any{"policy": reuseobs.Policy, "runtime_manifest_sha256": tuple.ManifestSHA256}
	p["language_version"], p["runtime_language_version"], p["runtime_min_compatible"] = tuple.RuntimeLanguageVersion, tuple.RuntimeLanguageVersion, tuple.RuntimeMinCompatible
}

func loggerReplayFixture(t *testing.T, mandatory bool, mutate func(*fxNative, map[string]any), materialChange func(map[string][]byte)) ReplayRequest {
	t.Helper()
	m := loggerPinMaterials(t, "A")
	tuple := loggerTuple(t, m)
	if materialChange != nil {
		materialChange(m)
	}
	if _, ok := m["native/runtime-tuple.json"]; ok {
		tuple = loggerTuple(t, m)
	}
	f := reuseFixtureFor(t, tuple.ManifestSHA256, tuple.ManifestSHA256)
	f.reducer = "native-result-r9"
	if mandatory {
		f.reducer = "native-result-r10"
	}
	for _, c := range f.cases {
		loggerProducer(c["producer"].(map[string]any), tuple)
		c["reuse_observation_claim"] = claimPass
	}
	for path, data := range f.extra {
		if !strings.HasPrefix(path, "responses/") {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatal(err)
		}
		loggerProducer(raw["producer"].(map[string]any), tuple)
		f.extra[path] = eligibilityJSON(t, raw)
	}
	root, profile := writeReuseFixture(t, f, func(top map[string]any) {
		if mandatory {
			top["result_schema"] = "tsgk-incremental-result/r3"
		}
		if mutate != nil {
			mutate(f, top)
		}
	})
	return ReplayRequest{Root: root, Profile: profile, RuntimePins: loggerPinRequest(t, profile, m)}
}

func TestReplayLoggerPinsProductionIndependence(t *testing.T) {
	for _, mandatory := range []bool{false, true} {
		t.Run(map[bool]string{false: "optional", true: "mandatory"}[mandatory], func(t *testing.T) {
			req := loggerReplayFixture(t, mandatory, nil, nil)
			want, err := Replay(pinTestContext(t), req)
			if err != nil || want.Assessment != AssessPass || !want.EvidenceValid || !validLoggerReplayResult(want) {
				t.Fatalf("historical logger replay: %v %+v", err, want)
			}
			if want.RuntimePins.RuntimePin.ManifestSHA256 == reuseobs.RuntimeManifestSHA256 {
				t.Fatal("fixture must be historical")
			}
			node := &EvidenceNode{ID: "replay", EvidenceMode: want.EvidenceMode, Assessment: want.Assessment, Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
			subject := &EvidenceNode{ExecutionStatus: want.Subject.ExecutionStatus, Assessment: want.Subject.Assessment, EvidenceMode: want.Subject.EvidenceMode, Identities: map[string]string{"run": want.Subject.Run, "attempt": "1", "platform": want.Subject.Platform}}
			env := &replayEnv{}
			if e := env.checkReplayFile(node, subject, map[string][]byte{"result.json": eligibilityJSON(t, want)}, func(code, path, msg string) { env.finding(code, path, msg) }); e != nil || len(env.findings) != 0 {
				t.Fatalf("stored logger result reader: %v %+v", e, env.findings)
			}
			for _, field := range []string{"status", "assessment", "mode"} {
				forged := want
				switch field {
				case "status":
					forged.Subject.ExecutionStatus = StatusFailed
				case "assessment":
					forged.Subject.Assessment = AssessBlocked
				case "mode":
					forged.Subject.EvidenceMode = ModeCarriedForward
				}
				forged.Recorded = Verdict{forged.Subject.ExecutionStatus, forged.Subject.Assessment}
				env := &replayEnv{}
				if e := env.checkReplayFile(node, subject, map[string][]byte{"result.json": eligibilityJSON(t, forged)}, func(code, path, msg string) { env.finding(code, path, msg) }); e != nil || len(env.findings) == 0 {
					t.Fatalf("stored logger graph subject forgery accepted: %s %v", field, e)
				}
			}
			for _, label := range []string{"B", "C"} {
				candidate := loggerPinRequest(t, req.Profile, loggerPinMaterials(t, label))
				selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": candidate.ApprovedPinSetSHA256})
				if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, candidate.PinSet); err != nil {
					t.Fatal(err)
				}
				got, err := Replay(pinTestContext(t), req)
				if err != nil || !reflect.DeepEqual(want, got) {
					t.Fatalf("production %s changed A: %v", label, err)
				}
			}
			bad := want
			bad.RuntimePins = nil
			if validLoggerReplayResult(bad) {
				t.Fatal("stored PASS without runtime approval accepted")
			}
			for _, required := range runtimePinRolesR1 {
				bad := want
				p := *want.RuntimePins
				pins := *p.Pins
				p.Pins = &pins
				bad.RuntimePins = &p
				pins.Materials = slices.DeleteFunc(slices.Clone(pins.Materials), func(m EvidencePinMaterial) bool { return m.Name == required.name })
				pins.Required--
				pins.Verified--
				if validLoggerReplayResult(bad) {
					t.Fatal("stored runtime descriptor reduction accepted", required.name)
				}
			}
		})
	}
}

func TestReplayLoggerPinsRejectForgery(t *testing.T) {
	for _, name := range []string{"tuple-driver", "unapproved", "subject", "producer", "projection-abi", "raw-abi", "raw-manifest", "build-runtime", "observation", "missing-tuple", "reduced-inventory", "missing-patch"} {
		t.Run(name, func(t *testing.T) {
			req := loggerReplayFixture(t, false, func(f *fxNative, top map[string]any) {
				c := top["cases"].([]any)[0].(map[string]any)
				p := c["producer"].(map[string]any)
				switch name {
				case "producer":
					p["reuse_observation"].(map[string]any)["runtime_manifest_sha256"] = reuseobs.RuntimeManifestSHA256
				case "projection-abi":
					p["runtime_language_version"] = 99
				case "build-runtime":
					top["build"].(map[string]any)["runtime_commit"] = strings.Repeat("f", 40)
				case "observation":
					c["steps"].([]any)[0].(map[string]any)["observations"].([]any)[0].(map[string]any)["runtime_manifest_sha256"] = reuseobs.RuntimeManifestSHA256
				}
			}, func(m map[string][]byte) {
				switch name {
				case "tuple-driver":
					runtimePinTuple(t, m, func(tuple *eligibilityRuntimeTuple) { tuple.DriverSHA256 = strings.Repeat("f", 64) })
				case "missing-tuple":
					delete(m, "native/runtime-tuple.json")
				case "missing-patch":
					delete(m, runtimePinRolesR1[5].name)
				case "reduced-inventory":
					var manifest eligibilityRuntimeManifest
					json.Unmarshal(m["native/runtime-manifest.json"], &manifest)
					manifest.Files = manifest.Files[:len(manifest.Files)-1]
					m["native/runtime-manifest.json"] = eligibilityJSON(t, manifest)
					runtimePinTuple(t, m, func(tuple *eligibilityRuntimeTuple) {
						tuple.ManifestSHA256 = digestHex(m["native/runtime-manifest.json"])
					})
				}
			})
			if name == "raw-abi" || name == "raw-manifest" {
				path := "responses/00000-c1.json"
				data, err := os.ReadFile(filepath.Join(req.Root, path))
				if err != nil {
					t.Fatal(err)
				}
				var raw map[string]any
				json.Unmarshal(data, &raw)
				p := raw["producer"].(map[string]any)
				if name == "raw-abi" {
					p["runtime_min_compatible"] = 99
				} else {
					p["reuse_observation"].(map[string]any)["runtime_manifest_sha256"] = reuseobs.RuntimeManifestSHA256
				}
				data = eligibilityJSON(t, raw)
				if err := os.WriteFile(filepath.Join(req.Root, path), data, 0644); err != nil {
					t.Fatal(err)
				}
				var prof map[string]any
				json.Unmarshal(req.Profile, &prof)
				for _, item := range prof["members"].([]any) {
					m := item.(map[string]any)
					if m["path"] == path {
						m["bytes"], m["sha256"] = len(data), digestHex(data)
					}
				}
				req.Profile = eligibilityJSON(t, prof)
				req.RuntimePins = loggerPinRequest(t, req.Profile, req.RuntimePins.Materials)
			}
			if name == "unapproved" {
				req.RuntimePins.ApprovedPinSetSHA256 = ""
			}
			if name == "subject" {
				req.RuntimePins.Subject = []byte("another subject")
				pinTestApprove(t, req.RuntimePins)
			}
			got, err := Replay(pinTestContext(t), req)
			if got.Assessment == AssessPass || got.EvidenceValid {
				t.Fatalf("logger runtime forgery accepted: %s err=%v %+v", name, err, got)
			}
		})
	}
}

func TestReplayLoggerPinsAdmission(t *testing.T) {
	req := loggerReplayFixture(t, false, nil, nil)
	missing := req
	missing.RuntimePins = nil
	if _, err := Replay(pinTestContext(t), missing); asError(err).Code != "REPLAY_RUNTIME_PINS_REDUCER_MISMATCH" {
		t.Fatal(err)
	}
	for _, reducer := range []string{"native-result-r3", "native-result-r8", "native-result-r7", "oracle-set-r3"} {
		var prof map[string]any
		json.Unmarshal(req.Profile, &prof)
		prof["reducer"] = reducer
		test := req
		test.Profile = eligibilityJSON(t, prof)
		if _, err := Replay(pinTestContext(t), test); asError(err).Code != "REPLAY_RUNTIME_PINS_REDUCER_MISMATCH" {
			t.Fatal(reducer, err)
		}
	}
}

func TestLoggerPinsHostMarkers(t *testing.T) {
	materials := loggerPinMaterials(t, "host-markers")
	tuple := loggerTuple(t, materials)
	for _, name := range []string{"approved", "current-marker", "missing-marker"} {
		t.Run(name, func(t *testing.T) {
			r, e := startRun(pinTestContext(t), ReplayOperations()["evidence-replay"].Wall)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			b := &eligibilityBudget{r: r}
			subject := []byte("host marker subject")
			_, pin, e := verifyLoggerRuntime(loggerPinRequest(t, subject, materials), subject, b)
			if e != nil || pin == nil {
				t.Fatal(e)
			}
			source := []byte("svc")
			o := reuseobs.Observation{Schema: reuseobs.Schema, Policy: reuseobs.Policy, RuntimeManifestSHA256: tuple.ManifestSHA256,
				RequestID: "svc", RunID: "run", ProducerSHA256: fxProducer, SourceSHA256: digestHex(source), SourceBytes: 3,
				Role: "initial", Status: "UNAVAILABLE", Reason: "NO_NATIVE_PARSE", Counts: &reuseobs.Counts{}}
			if name == "current-marker" {
				o.RuntimeManifestSHA256 = reuseobs.RuntimeManifestSHA256
			}
			raw := eligibilityJSON(t, []reuseobs.Observation{o})
			if name == "missing-marker" {
				raw = nil
			}
			c := &rcCase{ID: "svc", ExecutionStatus: StatusCompleted, ReuseObservationClaim: eligibilityJSON(t, claimNotClaimed), Steps: []rcStep{{Step: 0, SourceSHA256: digestHex(source), SourceBytes: 3, Observations: raw}}}
			x := &replayEnv{r: r, nativeInputBytes: 1024, members: map[string]ReplayMember{}, loggerRuntime: pin, observationRuntime: &pin.logger, observationManifest: tuple.ManifestSHA256}
			e = x.replayObservations(c, 0, nativeResult{ObservationRunID: "run", Build: &nativeBuild{Identity: fxProducer, RuntimeCommit: fxRuntime}}, &IncrementalCase{ID: "svc", Input: NativeInput{Bytes: 3, SHA256: digestHex(source)}})
			if e != nil {
				t.Fatal(e)
			}
			g := x.gate("reuse-observation")
			if (g.Failed == 0) != (name == "approved") {
				t.Fatalf("marker approval bypass: %s %+v", name, g)
			}
		})
	}
}

func TestLoggerPinsSharedBudget(t *testing.T) {
	b := semanticTestBudget(t)
	prior := &eligibilityRuntimePins{}
	b.runtime = prior
	subject := []byte("logger shared budget subject")
	req := loggerPinRequest(t, subject, loggerPinMaterials(t, "shared"))
	first, runtime, e := verifyLoggerRuntime(req, subject, b)
	if e != nil || runtime == nil || !first.Complete || b.runtime != prior || first.BytesRead != b.r.totalRead || first.Work != b.work || first.LogicalBytes != b.logical {
		t.Fatalf("runtime budget or authority escaped: %+v %v", first, e)
	}
	read, work, logical := b.r.totalRead, b.work, b.logical
	second, runtime, e := verifyLoggerRuntime(req, []byte("another subject"), b)
	if e != nil || runtime != nil || second.Assessment != AssessFail || b.runtime != prior || second.BytesRead+read != b.r.totalRead || second.Work+work != b.work || second.LogicalBytes+logical != b.logical {
		t.Fatalf("subject authority or counters changed: %+v %v", second, e)
	}
	b.work = eligibilityWorkLimit
	limited, runtime, e := verifyLoggerRuntime(req, subject, b)
	if e == nil || e.Code != "ELIGIBILITY_WORK_LIMIT" || runtime != nil || limited.Complete || b.runtime != prior {
		t.Fatalf("shared work budget bypassed: %+v %v", limited, e)
	}
}
