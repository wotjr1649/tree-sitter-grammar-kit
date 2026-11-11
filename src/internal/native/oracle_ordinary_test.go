package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// The owned grammar exercises publication independently of the full P0 plan corpus.
func TestOracleOrdinaryPublication(t *testing.T) {
	queries := []kit.OracleQuery{{ID: "q0", Source: "(identifier) @id"}, {ID: "facts.xml", Source: "(identifier) @tag"}}
	firstSource := "a = f(1);\n" + strings.Repeat("b = 2;\n", 12)
	profile, root := oracleProfile(t, queries, firstSource, "b = 2;\n")
	profile = withOperation(t, profile, "native-query-batch")
	p, err := kit.ParseOracleProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	edits := []kit.Edit{{StartByte: 6, OldEndByte: 7, NewEndByte: 7, Old: []byte("1"), New: []byte("2")}, {StartByte: 6, OldEndByte: 7, NewEndByte: 7, Old: []byte("2"), New: []byte("3")}}
	p.Native.Cases[0].Edits = edits
	p.OracleCases[0].Edits = edits
	packBytes, err := json.Marshal(kit.FactPack{Schema: kit.FactPackSchema, Revision: "owned-r1", Mapping: "declaration-facts-r1", DynamicSQL: "dynamic-sql-r1", XML: "xml-structure-r1", Routes: []kit.FactPackRoute{{Route: "owned", Declarations: []kit.NativeDeclaration{}, Queries: []kit.FactQuery{{ID: "facts.xml", Facts: kit.FactXMLStructure, Source: queries[1].Source}}}}})
	if err != nil {
		t.Fatal(err)
	}
	pack, err := kit.ParseFactPack(packBytes)
	if err != nil {
		t.Fatal(err)
	}
	p.FactPack = &kit.FactPackRef{Revision: pack.Revision, SHA256: pack.SHA256, Route: "owned"}
	mapping, err := os.ReadFile("../../contracts/fact-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	req := oracleRequest(t, profile, root, filepath.Join(t.TempDir(), "set"))
	req.FactPack, req.FactMapping = packBytes, mapping
	req.Ordinary = &OrdinaryRecordingRequest{Inputs: kit.OrdinaryPlanRequest{Profile: []byte(`{"owned":"planning"}`), Assets: []byte(`{"owned":"assets"}`)}, Registration: []byte(`{"owned":"registration"}`), ShardPlan: []byte(`{"owned":"plan"}`)}
	prepared := &ordinaryRecording{profile: kit.OrdinaryProfile{Candidate: "owned-control", SemanticCohort: "owned-cohort"}, registration: kit.OrdinaryRegistration{Route: "owned", Platform: "windows-amd64", ClosureEncoding: "plain"}, sources: [][]byte{[]byte(firstSource), []byte("b = 2;\n")}}
	prepared.shardIDs = []string{"owned/windows-amd64/ca", "owned/windows-amd64/cb"}
	cache := t.TempDir()
	req.BuildCache = cache
	for _, baseFail := range []bool{false, true} {
		t.Run(map[bool]string{false: "pass", true: "base-fail"}[baseFail], func(t *testing.T) {
			if baseFail {
				p.OracleCases[0].QueryExpect = []kit.QueryExpectation{{Step: 0, Query: "q0", Status: "INVALID_QUERY"}}
			}
			var document map[string]any
			if e := json.Unmarshal(profile, &document); e != nil {
				t.Fatal(e)
			}
			document["cases"].([]any)[0].(map[string]any)["edits"] = edits
			document["cases"].([]any)[0].(map[string]any)["query_expect"] = p.OracleCases[0].QueryExpect
			document["fact_pack"] = p.FactPack
			req.Profile, err = json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			p.Native.SHA256 = sha(req.Profile)
			req.Out = filepath.Join(t.TempDir(), "set")
			initial := OracleResult{Report: kit.Report{Schema: kit.ReportSchema, Command: "oracle record", Findings: []kit.Finding{}, Coverage: kit.Coverage{Requested: []string{}, Observed: []string{}, Unsupported: []string{}}}, Cases: []CaseLine{}, Platform: hostPlatform}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			res, err := recordOrdinaryPrepared(ctx, req, p, &pack, prepared, initial, time.Now())
			if err != nil || res.Set == nil || !res.Set.Valid || len(res.Cases) != 2 || res.BuildRemoved != "REMOVED" {
				t.Fatalf("publication err=%v set=%+v cases=%+v", err, res.Set, res.Cases)
			}
			want := kit.AssessBlocked
			if baseFail {
				want = kit.AssessFail
			}
			if res.Assessment != want {
				t.Fatalf("assessment %s want %s", res.Assessment, want)
			}
			read := func(path string) []byte {
				t.Helper()
				data, e := os.ReadFile(filepath.Join(req.Out, filepath.FromSlash(path)))
				if e != nil {
					t.Fatal(e)
				}
				return data
			}
			var manifest kit.OrdinaryOracleManifest
			if err := json.Unmarshal(read("manifest.json"), &manifest); err != nil {
				t.Fatal(err)
			}
			if manifest.Schema != kit.OracleOrdinaryManifestSchema || len(manifest.Batches) != 1 || len(manifest.Batches[0].Cases) != 2 {
				t.Fatalf("batch manifest %+v", manifest.Batches)
			}
			if res.OrdinaryBinding == nil || !reflect.DeepEqual(res.OrdinaryBinding.Members, manifest.Members) || !reflect.DeepEqual(res.OrdinaryBinding.Batches, manifest.Batches) {
				t.Fatal("actual run binding mismatch")
			}
			processes, stderrs := 0, 0
			for _, member := range manifest.Members {
				if member.Role == "process" {
					processes++
					if member.Owner != manifest.Batches[0].Owner {
						t.Fatal("process owner")
					}
				}
				if member.Role == "stderr" {
					stderrs++
				}
				data := read(member.Path)
				if uint64(len(data)) != member.Pin.Bytes || sha(data) != member.Pin.SHA256 {
					t.Fatal("member pin")
				}
			}
			if processes != 1 || stderrs != 1 {
				t.Fatalf("process/stderr duplicated %d/%d", processes, stderrs)
			}
			var base CaseResult
			if err := json.Unmarshal(read("projection/00000-ca.json"), &base); err != nil {
				t.Fatal(err)
			}
			baseWant := kit.AssessPass
			if baseFail {
				baseWant = kit.AssessFail
			}
			if base.Assessment != baseWant || base.QuerySemantics != nil || len(base.Steps) != 3 || base.Steps[2].Fresh == nil {
				t.Fatalf("immutable base %s steps %d", base.Assessment, len(base.Steps))
			}
			var record kit.OrdinaryOracleRecord
			if err := json.Unmarshal(read("records/00000-ca.json"), &record); err != nil {
				t.Fatal(err)
			}
			if record.Schema != kit.OracleOrdinaryRecordSchema || record.QuerySemantics.Code != "EXTERNAL_APPROVAL_PENDING" || record.QuerySemantics.Capture == nil || *record.QuerySemantics.Capture != manifest.Batches[0].Capture {
				t.Fatal("pending/capture not bound")
			}
			var facts OrdinaryStepFacts
			if err := json.Unmarshal(record.StepFacts, &facts); err != nil || len(facts.Steps) != 5 || len(facts.FactQueries) != 1 || facts.Mapping.SHA256 != sha(mapping) {
				t.Fatalf("step facts: %v %+v", err, facts)
			}
			if _, e := os.Stat(res.Build.Dir); !os.IsNotExist(e) {
				t.Fatalf("build not removed %v", e)
			}
		})
	}
}

func TestOracleOrdinaryRefusesBeforePublication(t *testing.T) {
	profile, root := oracleProfile(t, []kit.OracleQuery{{ID: "q0", Source: "(identifier) @id"}}, "a = 1;\n")
	for _, mode := range []string{"legacy-single", "batch-without-documents", "missing-fact-pack"} {
		t.Run(mode, func(t *testing.T) {
			data := profile
			if mode != "legacy-single" {
				data = withOperation(t, data, "native-query-batch")
			}
			if mode == "missing-fact-pack" {
				var doc map[string]any
				if err := json.Unmarshal(data, &doc); err != nil {
					t.Fatal(err)
				}
				doc["queries"].([]any)[0].(map[string]any)["id"] = "facts.xml"
				data, _ = json.Marshal(doc)
			}
			req := oracleRequest(t, data, root, filepath.Join(t.TempDir(), "set"))
			req.Ordinary = &OrdinaryRecordingRequest{}
			result, err := Oracle(context.Background(), req)
			if err == nil {
				t.Fatal("ordinary accepted missing/context documents")
			}
			if mode == "missing-fact-pack" && (len(result.Findings) != 1 || result.Findings[0].Code != "ORDINARY_FACT_PACK_REQUIRED") {
				t.Fatalf("fact suffix accepted without pack: %v", err)
			}
			if _, e := os.Stat(req.Out); !os.IsNotExist(e) {
				t.Fatalf("refusal wrote output %v", e)
			}
		})
	}
}

func TestOrdinaryExecutionBindsSourcesHistoryQueriesAndRuntime(t *testing.T) {
	profile, root := oracleProfile(t, []kit.OracleQuery{{ID: "q0", Source: "(identifier) @id"}}, "a = 1;\n", "b = 2;\n")
	profile = withOperation(t, profile, "native-query-batch")
	prof, err := kit.ParseOracleProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	prof.Native.Cases[0].Edits = []kit.Edit{{StartByte: 4, OldEndByte: 5, NewEndByte: 5, Old: []byte("1"), New: []byte("3")}, {StartByte: 4, OldEndByte: 5, NewEndByte: 5, Old: []byte("3"), New: []byte("4")}}
	runtimeManifest, err := drivers.NativeC.ReadFile("native-c/runtime-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	p := kit.OrdinaryProfile{Bindings: []kit.OrdinaryBinding{{Route: prof.Native.Route, Platform: "windows-amd64", Grammar: prof.Native.Grammar, Runtime: ordinaryPin("runtime/manifest.json", runtimeManifest)}}}
	r := kit.OrdinaryRegistration{Route: prof.Native.Route, Platform: "windows-amd64", Queries: []kit.OrdinaryQuery{{ID: "q0", Source: ordinaryPin("queries/q0.scm", []byte(prof.Queries[0].Source))}}}
	for _, c := range prof.Native.Cases {
		source, err := os.ReadFile(filepath.Join(root, c.Input.Path))
		if err != nil {
			t.Fatal(err)
		}
		versions, points, err := kit.ApplyEdits(kit.EncodingUTF8, source, c.Edits, 65536)
		if err != nil {
			t.Fatal(err)
		}
		rc := kit.OrdinaryCase{ID: c.ID, Edits: c.Edits, Points: points}
		for _, version := range versions {
			rc.VersionPins = append(rc.VersionPins, ordinaryPin("source-witness/"+sha(version)+".utf8", version))
		}
		rc.InitialSource = rc.VersionPins[0]
		r.Cases = append(r.Cases, rc)
	}
	req := OracleRequest{Root: root}
	if got, err := bindOrdinaryExecution(context.Background(), req, prof, p, r); err != nil || len(got.sources) != 2 {
		t.Fatalf("binding %v", err)
	}
	for _, mode := range []string{"query-order", "query-source", "runtime", "source-version", "edit-history", "edit-points", "case-order"} {
		t.Run(mode, func(t *testing.T) {
			pb, _ := json.Marshal(p)
			rb, _ := json.Marshal(r)
			var changedP kit.OrdinaryProfile
			var changedR kit.OrdinaryRegistration
			if err := json.Unmarshal(pb, &changedP); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(rb, &changedR); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "query-order":
				changedR.Queries[0].ID = "another"
			case "query-source":
				changedR.Queries[0].Source.SHA256 = strings.Repeat("0", 64)
			case "runtime":
				changedP.Bindings[0].Runtime.SHA256 = strings.Repeat("0", 64)
			case "source-version":
				changedR.Cases[0].VersionPins[1].SHA256 = changedR.Cases[0].VersionPins[0].SHA256
			case "edit-history":
				changedR.Cases[0].Edits[0].New = []byte("2")
			case "edit-points":
				changedR.Cases[0].Points[0].NewEnd.Column++
			case "case-order":
				changedR.Cases[0], changedR.Cases[1] = changedR.Cases[1], changedR.Cases[0]
			}
			if _, err := bindOrdinaryExecution(context.Background(), req, prof, changedP, changedR); err == nil {
				t.Fatal("accepted execution mutation")
			}
		})
	}
}

func TestOrdinaryFailureKeepsKnownBaseFailure(t *testing.T) {
	for _, kind := range []string{kit.KindResourceLimit, kit.KindUnsupported, kit.KindCancelled} {
		t.Run(kind, func(t *testing.T) {
			before := OracleResult{Report: kit.Report{ExecutionStatus: kit.StatusCompleted, Assessment: kit.AssessFail, Findings: []kit.Finding{{Code: "KNOWN_BASE_FAIL"}}}, Cases: []CaseLine{{ID: "c", Assessment: kit.AssessFail, Code: "KNOWN_BASE_FAIL"}}}
			after, err := ordinaryRecordError(context.Background(), before, refuse(kind, "LATER_FAILURE", nil))
			if err == nil || after.Assessment != kit.AssessFail || after.Cases[0].Code != "KNOWN_BASE_FAIL" || after.Findings[0].Code != "KNOWN_BASE_FAIL" {
				t.Fatalf("known failure hidden: %+v %v", after, err)
			}
			if kind == kit.KindResourceLimit && after.ExecutionStatus != kit.StatusResourceLimit || kind == kit.KindCancelled && after.ExecutionStatus != kit.StatusCancelled {
				t.Fatal("interruption status lost")
			}
		})
	}
}

func TestOrdinarySelectedShardsBindExactCaseOrder(t *testing.T) {
	r := kit.OrdinaryRegistration{Route: "owned", Platform: "windows-amd64", Subject: "windows/amd64"}
	plan := kit.OrdinaryShardPlan{Shards: []kit.OrdinaryPlanShard{{ID: "first", Route: r.Route, Platform: r.Platform, Subject: r.Subject, Case: "c1"}, {ID: "second", Route: r.Route, Platform: r.Platform, Subject: r.Subject, Case: "c2"}, {ID: "linux", Route: r.Route, Platform: "linux-amd64", Subject: "linux/amd64", Case: "c1"}}}
	cases := []kit.IncrementalCase{{ID: "c1"}, {ID: "c2"}}
	if err := validateOrdinaryShardSelection([]string{"first", "second"}, plan, r, cases); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{nil, {"first"}, {"first", "first"}, {"second", "first"}, {"linux", "second"}, {"unknown", "second"}} {
		if err := validateOrdinaryShardSelection(ids, plan, r, cases); err == nil {
			t.Fatalf("accepted selected shard mutation %v", ids)
		}
	}
}

func TestOrdinaryPagedDescriptorUsesSharedBuildOwner(t *testing.T) {
	for _, role := range []string{"grammar-paged-descriptor", "driver-paged-descriptor"} {
		var data []byte
		if role == "grammar-paged-descriptor" {
			data, _ = json.Marshal(semanticParserPages{Schema: "tsgk-query-parser-pages/r1", Owner: "D", Parts: []semanticParserPart{{Index: 0, Bytes: 16 << 20}}})
		} else {
			data, _ = json.Marshal(semanticBinaryPages{Schema: "tsgk-query-binary-pages/r1", Owner: "D", Parts: []semanticParserPart{{Index: 0, Bytes: 16 << 20}}})
		}
		file := captureFile{pin: kit.ReplayMember{Path: "capture/descriptor.json", Role: role, Bytes: uint64(len(data)), SHA256: sha(data)}, data: data}
		changed, err := ordinaryBuildOwner(file)
		if err != nil {
			t.Fatal(err)
		}
		var descriptor struct {
			Owner string               `json:"owner"`
			Parts []semanticParserPart `json:"parts"`
		}
		if err := json.Unmarshal(changed.data, &descriptor); err != nil || descriptor.Owner != "D_build" || len(descriptor.Parts) != 1 || descriptor.Parts[0].Bytes != 16<<20 || changed.pin.SHA256 != sha(changed.data) || changed.pin.Role != role || changed.pin.Path != file.pin.Path {
			t.Fatalf("descriptor binding %s %v", role, err)
		}
		if sha(file.data) != file.pin.SHA256 {
			t.Fatal("legacy capture descriptor changed")
		}
	}
}
