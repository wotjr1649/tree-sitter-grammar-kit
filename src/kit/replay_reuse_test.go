package kit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

func reuseFixture(t *testing.T) *fxNative {
	t.Helper()
	return reuseFixtureFor(t, reuseobs.RuntimeManifestSHA256, reuseobs.RuntimeManifestSHA256)
}

// reuseFixtureFor declares producerManifest in the producer capability and binds each
// observation to observationManifest.
func reuseFixtureFor(t *testing.T, producerManifest, observationManifest string) *fxNative {
	t.Helper()
	f := newFxNative()
	f.extra["sources/00000-c1.bin"], f.extra["sources/00001-c2.bin"] = []byte("alpha"), []byte("beta")
	f.reducer = "native-result-r3"
	cap := map[string]any{"policy": reuseobs.Policy, "runtime_manifest_sha256": producerManifest}
	for _, c := range f.cases {
		var wireSteps []any
		var all []reuseobs.Observation
		previous := ""
		c["producer"] = map[string]any{"query": "UNSUPPORTED", "reuse_observation": cap}
		for _, value := range c["steps"].([]any) {
			s := value.(map[string]any)
			k := s["step"].(int)
			o := reuseobs.Observation{Schema: reuseobs.Schema, Policy: reuseobs.Policy, RuntimeManifestSHA256: observationManifest,
				RequestID: c["id"].(string), RunID: "fx-run", ProducerSHA256: fxProducer, Step: uint64(k), Role: "initial",
				SourceSHA256: s["source_sha256"].(string), SourceBytes: uint64(s["source_bytes"].(int)), Status: "COMPLETE", Events: 2, Bytes: 30, Counts: &reuseobs.Counts{NewParse: 1, Done: 1}}
			observations := []reuseobs.Observation{o}
			if k > 0 {
				s["edit"] = map[string]any{"start_byte": 3, "old_end_byte": 4, "new_end_byte": 4, "start_point": []int{0, 3}, "old_end_point": []int{0, 4}, "new_end_point": []int{0, 4}}
				o.Role, o.OldTree, o.Counts = "incremental", true, &reuseobs.Counts{ParseAfterEdit: 1, Done: 1}
				old := previous
				o.OldSourceSHA256 = &old
				fresh := observations[0]
				fresh.Role = "fresh"
				observations = []reuseobs.Observation{o, fresh}
			}
			s["observations"] = observations
			all = append(all, observations...)
			ws := map[string]any{"step": k, "source_bytes": s["source_bytes"], "source_sha256": s["source_sha256"], "observations": observations, "edit": s["edit"]}
			for _, slot := range []string{"incremental", "fresh"} {
				if t, ok := s[slot].(map[string]any); ok {
					ws[slot] = map[string]any{"status": t["status"], "code": t["code"], "form": t["form"], "descendant_count": t["descendant_count"]}
				}
			}
			wireSteps = append(wireSteps, ws)
			previous = o.SourceSHA256
		}
		c["reuse_observation_claim"] = reuseobs.Judge(all)
		path := "responses/00000-c1.json"
		if c["id"] == "c2" {
			path = "responses/00001-c2.json"
		}
		f.extra[path], _ = json.Marshal(map[string]any{"protocol": "tsgk-native/r3", "id": c["id"], "status": c["response_status"], "code": c["response_code"], "producer": c["producer"], "steps": wireSteps, "steps_completed": len(wireSteps), "complete": true})
	}
	return f
}

// Rehash mutated fixture members, so these controls test semantic checks rather than
// failing earlier on a byte identity mismatch.
func writeReuseFixture(t *testing.T, f *fxNative, mutate func(map[string]any)) (string, []byte) {
	t.Helper()
	return f.write(t, func(root string, reg map[string]any) {
		path := filepath.Join(root, "result.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var top map[string]any
		if err := json.Unmarshal(raw, &top); err != nil {
			t.Fatal(err)
		}
		top["result_schema"], top["reuse_observation_policy"], top["observation_run_id"] = "tsgk-incremental-result/r2", reuseobs.Policy, "fx-run"
		if mutate != nil {
			mutate(top)
		}
		raw, err = json.Marshal(top)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		for _, v := range reg["members"].([]any) {
			m := v.(map[string]any)
			if m["path"] == "sources/00000-c1.bin" || m["path"] == "sources/00001-c2.bin" {
				m["role"] = "case-source"
			}
			if m["path"] == "result.json" {
				m["bytes"], m["sha256"] = len(raw), sum(raw)
			}
		}
	})
}

func TestReplayReuseObservations(t *testing.T) {
	root, reg := writeReuseFixture(t, reuseFixture(t), nil)
	t.Setenv("PATH", "")
	r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	if err != nil || r.Assessment != AssessPass || !r.EvidenceValid || gateOf(r, "reuse-observation").Failed != 0 {
		t.Fatalf("replay: %v %+v", err, r)
	}
}

// A record bound to the previous runtime manifest keeps its meaning under the same reducer
// revision; unknown or mixed manifests are refused (#213).
func TestReplayReuseRuntimeManifests(t *testing.T) {
	previous := "affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080"
	unknown := strings.Repeat("d", 64)
	for _, tc := range []struct {
		name, producer, observation string
		pass                        bool
	}{
		{"current", reuseobs.RuntimeManifestSHA256, reuseobs.RuntimeManifestSHA256, true},
		{"previous", previous, previous, true},
		{"unknown", unknown, unknown, false},
		{"mixed", previous, reuseobs.RuntimeManifestSHA256, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, reg := writeReuseFixture(t, reuseFixtureFor(t, tc.producer, tc.observation), nil)
			t.Setenv("PATH", "")
			r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
			if err != nil {
				t.Fatal(err)
			}
			passed := r.Assessment == AssessPass && r.EvidenceValid && gateOf(r, "reuse-observation").Failed == 0
			if passed != tc.pass {
				t.Fatalf("pass=%v want %v: %+v", passed, tc.pass, gateOf(r, "reuse-observation"))
			}
		})
	}
}

// A case without native parse has no producer; its host markers declare the manifest they
// bind to, and that manifest must be a verified one shared by every marker (#213).
func TestRecordManifestForHostMarkers(t *testing.T) {
	previous := "affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080"
	src := strings.Repeat("a", 64)
	marker := func(manifest string, step int) []byte {
		raw, err := json.Marshal([]reuseobs.Observation{{Schema: reuseobs.Schema, Policy: reuseobs.Policy, RuntimeManifestSHA256: manifest,
			RequestID: "svc", RunID: "run", ProducerSHA256: fxProducer, Step: uint64(step), Role: "initial",
			SourceSHA256: src, SourceBytes: 3, Status: "UNAVAILABLE", Reason: "NO_NATIVE_PARSE", Counts: &reuseobs.Counts{}}})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	for _, tc := range []struct {
		name   string
		steps  []string
		decode []bool
	}{
		{"current", []string{reuseobs.RuntimeManifestSHA256}, []bool{true}},
		{"previous", []string{previous, previous}, []bool{true, true}},
		{"unknown", []string{strings.Repeat("d", 64)}, []bool{false}},
		{"mixed", []string{previous, reuseobs.RuntimeManifestSHA256}, []bool{true, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &rcCase{}
			for _, m := range tc.steps {
				c.Steps = append(c.Steps, rcStep{Step: 0, Observations: marker(m, 0)})
			}
			manifest, declared := recordManifest(c)
			if declared {
				t.Fatal("a host-only case has no producer declaration")
			}
			for k, s := range c.Steps {
				b := reuseobs.Binding{RuntimeManifestSHA256: manifest, RequestID: "svc", RunID: "run", ProducerSHA256: fxProducer,
					SourceSHA256: src, SourceBytes: 3}
				_, err := decodeObservations(s.Observations, b, false)
				if (err == nil) != tc.decode[k] {
					t.Fatalf("step %d manifest %s: err=%v want ok=%v", k, manifest[:8], err, tc.decode[k])
				}
			}
		})
	}
}

func TestReplayReuseRejectsRehashedMutations(t *testing.T) {
	for _, name := range []string{"run", "policy", "producer", "missing-role", "missing-zero", "old-source", "claim", "null-claim", "raw-drift", "missing-raw", "legacy-result", "legacy-reducer", "edit-point-both", "source-witness"} {
		t.Run(name, func(t *testing.T) {
			f := reuseFixture(t)
			if name == "source-witness" {
				f.extra["sources/00001-c2.bin"] = []byte("bad!")
			}
			if name == "edit-point-both" {
				var wire map[string]any
				if err := json.Unmarshal(f.extra["responses/00001-c2.json"], &wire); err != nil {
					t.Fatal(err)
				}
				wire["steps"].([]any)[1].(map[string]any)["edit"].(map[string]any)["start_point"] = []int{9, 9}
				f.extra["responses/00001-c2.json"], _ = json.Marshal(wire)
			}
			if name == "raw-drift" {
				f.extra["responses/00001-c2.json"] = []byte(`{"protocol":"tsgk-native/r3","id":"c2","steps":[],"complete":true}`)
			}
			if name == "missing-raw" {
				delete(f.extra, "responses/00001-c2.json")
			}
			if name == "legacy-reducer" {
				f.reducer = "native-result-r2"
			}
			root, reg := writeReuseFixture(t, f, func(top map[string]any) {
				c := top["cases"].([]any)[1].(map[string]any)
				s := c["steps"].([]any)[1].(map[string]any)
				o := s["observations"].([]any)[0].(map[string]any)
				switch name {
				case "run":
					top["observation_run_id"] = "other-run"
				case "policy":
					top["reuse_observation_policy"] = "other-policy"
				case "producer":
					o["producer_sha256"] = fxExe
				case "missing-role":
					s["observations"] = s["observations"].([]any)[:1]
				case "missing-zero":
					delete(o["counts"].(map[string]any), "accepted")
				case "old-source":
					o["old_source_sha256"] = fxExe
				case "claim":
					c["reuse_observation_claim"] = "NOT_CLAIMED"
				case "null-claim":
					c["reuse_observation_claim"] = nil
				case "legacy-result":
					top["result_schema"] = "tsgk-incremental-result/r1"
				case "edit-point-both":
					s["edit"].(map[string]any)["start_point"] = []int{9, 9}
				}
			})
			r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
			if err == nil && r.EvidenceValid {
				t.Fatalf("accepted %s: %+v", name, r)
			}
		})
	}
}

func TestReplayReuseWithoutSourceKeepsPointsUnresolved(t *testing.T) {
	f := reuseFixture(t)
	delete(f.extra, "sources/00000-c1.bin")
	delete(f.extra, "sources/00001-c2.bin")
	root, reg := writeReuseFixture(t, f, nil)
	r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	points := gateOf(r, "edit-points")
	if err != nil || !r.EvidenceValid || r.Assessment != AssessUnresolved || points.NotRecomputed != 1 || points.Assessment != AssessUnresolved || gateOf(r, "reuse-observation").Failed != 0 {
		t.Fatalf("missing witness: %v assessment=%s points=%+v", err, r.Assessment, points)
	}
}

func TestReplayReuseWithoutWitnessBindsInitialWorkload(t *testing.T) {
	f := reuseFixture(t)
	delete(f.extra, "sources/00000-c1.bin")
	delete(f.extra, "sources/00001-c2.bin")
	var wire map[string]any
	if err := json.Unmarshal(f.extra["responses/00000-c1.json"], &wire); err != nil {
		t.Fatal(err)
	}
	step := wire["steps"].([]any)[0].(map[string]any)
	step["source_sha256"], step["source_bytes"] = fxExe, 9
	o := step["observations"].([]any)[0].(map[string]any)
	o["source_sha256"], o["source_bytes"] = fxExe, 9
	f.extra["responses/00000-c1.json"], _ = json.Marshal(wire)
	root, reg := writeReuseFixture(t, f, func(top map[string]any) {
		c := top["cases"].([]any)[0].(map[string]any)
		s := c["steps"].([]any)[0].(map[string]any)
		s["source_sha256"], s["source_bytes"] = fxExe, 9
		o := s["observations"].([]any)[0].(map[string]any)
		o["source_sha256"], o["source_bytes"] = fxExe, 9
	})
	r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	if err != nil || r.EvidenceValid || gateOf(r, "case-binding").Failed == 0 {
		t.Fatalf("accepted initial workload substitution: %+v", r)
	}
}

func TestLegacyReplayRejectsPresentObservationMetadata(t *testing.T) {
	for _, field := range []string{"reuse_observation_policy", "observation_run_id"} {
		for _, state := range []string{"absent", "empty", "null"} {
			t.Run(field+"/"+state, func(t *testing.T) {
				f := newFxNative()
				f.reducer = "native-result-r2"
				root, reg := writeReuseFixture(t, f, func(top map[string]any) {
					top["result_schema"] = "tsgk-incremental-result/r1"
					delete(top, "reuse_observation_policy")
					delete(top, "observation_run_id")
					if state == "empty" {
						top[field] = ""
					} else if state == "null" {
						top[field] = nil
					}
				})
				r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
				if state == "absent" {
					if err != nil || !r.EvidenceValid || r.Assessment != AssessPass {
						t.Fatalf("legacy baseline rejected: %v assessment=%s", err, r.Assessment)
					}
				} else {
					var ke *Error
					if !errors.As(err, &ke) || ke.Code != "OBSERVATION_SCHEMA_UNSUPPORTED" {
						t.Fatalf("legacy accepted present %s %s: %v", field, state, err)
					}
				}
			})
		}
	}
}
