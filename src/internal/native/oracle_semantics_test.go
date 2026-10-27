package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestOracleSemanticsProducer(t *testing.T) {
	if os.Getenv("TSGK_BENCH_PREPARED") == "" {
		t.Skip("pinned JSON grammar not configured")
	}
	if runtime.GOOS == "darwin" {
		t.Skip("new semantics claim is deferred on macOS")
	}
	rt, cc := nativeTools(t)
	var anchors struct {
		Queries []kit.OracleQuery `json:"queries"`
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "query-semantics", "small-json-anchors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &anchors); err != nil {
		t.Fatal(err)
	}
	registration := []byte(`{"producer_control":"not_external_approval"}`)
	source := []byte(`{"a":[1,1]}`)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "c1.json"), source, 0600); err != nil {
		t.Fatal(err)
	}
	input := kit.NativeInput{Path: "c1.json", Role: "case", Bytes: uint64(len(source)), SHA256: sha(source)}
	symbol, grammar := registeredGrammar(t, "json")
	hash, size := digestOf(t, cc)
	c := map[string]any{"id": "c1", "input": input, "encoding": kit.EncodingUTF8, "edits": []kit.Edit{{StartByte: 8, OldEndByte: 9, NewEndByte: 9, Old: []byte("1"), New: []byte("2")}}, "points": []any{}, "expect": []any{}, "query_expect": []any{}, "dynamic_sql_expect": nil}
	doc := map[string]any{"schema": kit.OracleSemanticsSchema, "id": "semantics-producer-control", "route": "json", "operation": "native-query", "symbol": symbol, "encoding": kit.EncodingUTF8, "output": kit.OutputTree, "compiler": kit.ToolIdentity{Name: "cc", Version: "control", SHA256: hash, Bytes: size}, "grammar": grammar, "declarations": nil, "queries": anchors.Queries, "fact_pack": nil, "api": false, "cases": []any{c}, "query_semantics_registration_sha256": sha(registration)}
	profile, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "set")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	req := OracleRequest{Root: root, GrammarRoot: filepath.Join(os.Getenv("TSGK_BENCH_PREPARED"), "routes", "json"), Profile: profile, Runtime: rt, Compiler: cc, Work: t.TempDir(), Out: out, Allow: []string{AllowBuild, AllowExec}, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT"), QuerySemanticsRegistration: registration, BuildCache: t.TempDir()}
	refusal := req
	refusal.QuerySemanticsRegistration = nil
	if _, err := Oracle(ctx, refusal); err == nil {
		t.Fatal("profile3 silently fell back without capture opt-in")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("refusal created output: %v", err)
	}
	result, err := Oracle(ctx, req)
	if err != nil || result.ResultSchema != "tsgk-oracle-result/r3" || result.ExecutionStatus != kit.StatusCompleted || result.Assessment != kit.AssessBlocked || result.Set == nil || !result.Set.Valid || len(result.Cases) != 1 {
		t.Fatalf("producer result err=%v status=%s assessment=%s set=%+v findings=%+v", err, result.ExecutionStatus, result.Assessment, result.Set, result.Findings)
	}
	local := result.Cases[0].QuerySemantics
	if local == nil || local.Claim != ClaimBlocked || local.Capture == nil || local.Code != "EXTERNAL_APPROVAL_PENDING" {
		t.Fatalf("local pending leaf %+v", local)
	}
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	var capture processCapture
	if err := json.Unmarshal(read(local.Capture.Path), &capture); err != nil {
		t.Fatal(err)
	}
	if capture.Stage != "RUN_RETURNED" || capture.Error != nil || capture.Spec == nil || capture.Spec.Stdin == nil || capture.Process == nil || capture.Stdout == nil {
		t.Fatalf("actual capture %+v", capture)
	}
	raw, err := SplitFrame(read(capture.Stdout.Path))
	if err != nil || !bytes.Equal(raw, read("raw/00000-c1.json")) {
		t.Fatalf("actual raw frame binding: %v", err)
	}
	request, err := SplitFrame(read(capture.Spec.Stdin.Path))
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Protocol string `json:"protocol"`
		ID       string `json:"id"`
	}
	if err := json.Unmarshal(request, &wire); err != nil || wire.Protocol != ProtocolR4 || wire.ID != "c1" {
		t.Fatalf("actual request: %+v %v", wire, err)
	}
	var projection CaseResult
	if err := json.Unmarshal(read(local.Projection.Path), &projection); err != nil {
		t.Fatal(err)
	}
	if projection.QuerySemantics != nil || projection.Assessment != kit.AssessPass || projection.Process == nil || !projection.Process.Succeeded() {
		t.Fatalf("semantic-free projection %+v", projection)
	}
	var stored OracleRecord
	if err := json.Unmarshal(read("records/00000-c1.json"), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Schema != kit.OracleSemanticsRecordSchema || stored.Assessment != kit.AssessBlocked || stored.Code != local.Code || !reflect.DeepEqual(stored.QuerySemantics, local) {
		t.Fatal("record/stdout producer fold mismatch")
	}
	var man kit.OracleManifest
	if err := json.Unmarshal(read("manifest.json"), &man); err != nil {
		t.Fatal(err)
	}
	if man.Schema != kit.OracleSemanticsManifestSchema || man.Assessment != kit.AssessBlocked || man.QuerySemanticsRegistrationSHA256 != sha(registration) || result.BuildRemoved != "REMOVED" {
		t.Fatalf("manifest/build lifecycle %+v", man)
	}
	roles := map[string]int{}
	for _, mem := range man.Members {
		roles[mem.Role]++
		actual := read(mem.Path)
		if uint64(len(actual)) != mem.Bytes || sha(actual) != mem.SHA256 {
			t.Fatalf("manifest member %s", mem.Path)
		}
	}
	if roles["driver"] != 1 || roles["build"] != 1 || roles["build-input"]+roles["grammar-parser"]+roles["grammar-scanner"]+roles["grammar-header"] != len(result.Build.Closure) || roles["projection"] != 1 || roles["capture"] != 1 || roles["process"] != 1 {
		t.Fatalf("incomplete capture roles: %v", roles)
	}
	semanticProducerCounterexamples(t, man, read)
	t.Run("public-api-absent", func(t *testing.T) { semanticAPIControl(t, req, doc, data, man, false) })
	t.Run("public-api-base-failure", func(t *testing.T) { semanticAPIControl(t, req, doc, data, man, true) })
	t.Run("public-api-empty-queries", func(t *testing.T) {
		empty := maps.Clone(doc)
		empty["queries"] = []kit.OracleQuery{}
		semanticAPIControl(t, req, empty, []byte(`{"queries":[],"required":[]}`), man, false)
	})
	t.Run("cache-hit", func(t *testing.T) {
		req.Out = filepath.Join(t.TempDir(), "set")
		req.Work = t.TempDir()
		cached, err := Oracle(ctx, req)
		if err != nil || cached.Set == nil || !cached.Set.Valid || cached.Build.Reuse == nil || !cached.Build.Reuse.Hit {
			t.Fatalf("cached producer: %v set=%+v", err, cached.Set)
		}
		cachedRead := func(path string) []byte {
			t.Helper()
			data, err := os.ReadFile(filepath.Join(req.Out, filepath.FromSlash(path)))
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
		var cachedManifest kit.OracleManifest
		if err := json.Unmarshal(cachedRead("manifest.json"), &cachedManifest); err != nil {
			t.Fatal(err)
		}
		semanticProducerCounterexamples(t, cachedManifest, cachedRead)
	})
	t.Run("empty-queries", func(t *testing.T) {
		doc["queries"] = []kit.OracleQuery{}
		emptyProfile, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		req.Profile, req.Out, req.Work = emptyProfile, filepath.Join(t.TempDir(), "set"), t.TempDir()
		empty, err := Oracle(ctx, req)
		if err != nil || empty.Set == nil || !empty.Set.Valid || empty.ExecutionStatus != kit.StatusCompleted || empty.Assessment != kit.AssessBlocked {
			t.Fatalf("empty query producer: %v status=%s set=%+v", err, empty.ExecutionStatus, empty.Set)
		}
	})
	t.Logf("producer BLOCKED; base PASS; actual request/process/raw retained; %d build inputs preserved; this control has no external eligibility approval", roles["build-input"])
}

func TestOracleSemanticsLocalFold(t *testing.T) {
	for _, assessment := range []string{kit.AssessPass, kit.AssessFail, kit.AssessBlocked, kit.AssessNotAssessed} {
		c := CaseResult{ExecutionStatus: kit.StatusCompleted, Assessment: assessment, Code: "original"}
		semanticLocalCase(&c, "registration", nil, kit.ReplayMember{})
		if assessment == kit.AssessPass {
			if c.Assessment != kit.AssessBlocked || c.Code != "EXTERNAL_APPROVAL_PENDING" {
				t.Fatal("PASS not blocked pending approval")
			}
		} else if c.Assessment != assessment || c.Code != "original" {
			t.Fatal("prior verdict or first code changed")
		}
	}
	c := CaseResult{ExecutionStatus: kit.StatusNotRun, Assessment: kit.AssessNotAssessed}
	semanticLocalCase(&c, "registration", nil, kit.ReplayMember{})
	if c.Assessment != kit.AssessNotAssessed || c.Code != "" {
		t.Fatal("not-run verdict changed")
	}
}

type semanticCountingFS struct {
	fs.FS
	opens int
}

func (s *semanticCountingFS) Open(name string) (fs.File, error) { s.opens++; return s.FS.Open(name) }

func semanticProducerCounterexamples(t *testing.T, original kit.OracleManifest, read func(string) []byte) {
	t.Helper()
	initial := fstest.MapFS{}
	for _, pin := range original.Members {
		initial[pin.Path] = &fstest.MapFile{Data: read(pin.Path), Mode: 0600}
	}
	cases := []struct{ name, code string }{
		{"manifest-pass", "SEMANTICS_PRODUCER_VERDICT"}, {"record-pass", "SEMANTICS_PRODUCER_VERDICT"}, {"projection-code", "SEMANTICS_PROJECTION_VERDICT"},
		{"capture-null", "SEMANTICS_CAPTURE_MISSING"}, {"process-missing", "SEMANTICS_CAPTURE_MEMBER"}, {"stdout-pin", "SEMANTICS_CAPTURE_MEMBER"},
		{"driver-missing", "SEMANTICS_MEMBER_COUNT"}, {"build-input-missing", "SEMANTICS_BUILD_BINDING"}, {"build-manifest-missing", "SEMANTICS_MEMBER_COUNT"},
		{"closure-drop", "SEMANTICS_BUILD_BINDING"}, {"raw-frame", "SEMANTICS_FRAME_MISMATCH"}, {"process-pair", "SEMANTICS_PROCESS_MISMATCH"},
		{"member-limit", "SEMANTICS_SET_LIMIT"}, {"total-limit", "SEMANTICS_SET_LIMIT"}, {"count-limit", "SEMANTICS_SET_LIMIT"},
		{"incomplete-pass", "SEMANTICS_PROJECTION_VERDICT"}, {"projection-alias", "JSON_UNKNOWN_FIELD"}, {"raw-missing", "SEMANTICS_RAW_MISSING"},
		{"process-false", "SEMANTICS_PROJECTION_SHAPE"}, {"process-length", "SEMANTICS_PROCESS_STREAM"}, {"stderr-truncation", "SEMANTICS_PROCESS_STREAM"}, {"stdin-frame", "SEMANTICS_FRAME_LENGTH"},
		{"runtime-producer", "SEMANTICS_BUILD_PRODUCER"}, {"compiler-producer", "SEMANTICS_BUILD_PRODUCER"}, {"version-producer", "SEMANTICS_BUILD_PRODUCER"}, {"build-argv", "SEMANTICS_BUILD_STEPS"},
		{"big-record", "ELIGIBILITY_WORK_LIMIT"}, {"big-manifest", "ELIGIBILITY_WORK_LIMIT"}, {"unlisted-limit", "FILES_LIMIT"}, {"depth-limit", "DEPTH_LIMIT"},
		{"producer-null", "SEMANTICS_PRODUCER_MISSING"}, {"producer-empty", "JSON_MISSING_FIELD"}, {"producer-field-type", "JSON_TYPE"}, {"producer-capability", "SEMANTICS_PRODUCER_INVALID"},
		{"expectation-false", "JSON_TYPE"}, {"query-expectation-false", "JSON_TYPE"}, {"orphan-raw", "SEMANTICS_CAPTURE_MISSING"}, {"schema-probe-error", "HEADER_REJECTED"},
	}
	var savedBuild Build
	if err := json.Unmarshal(initial["capture/build.json"].Data, &savedBuild); err != nil {
		t.Fatal(err)
	}
	if savedBuild.Reuse != nil && savedBuild.Reuse.Hit {
		cases = []struct{ name, code string }{{"build-argv", "SEMANTICS_BUILD_STEPS"}}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := fstest.MapFS{}
			for path, file := range initial {
				files[path] = file
			}
			man := original
			man.Members = slices.Clone(original.Members)
			man.Producer = maps.Clone(original.Producer)
			decode := func(path string) map[string]any {
				t.Helper()
				d := json.NewDecoder(bytes.NewReader(files[path].Data))
				d.UseNumber()
				var v map[string]any
				if err := d.Decode(&v); err != nil {
					t.Fatal(err)
				}
				return v
			}
			store := func(path string, v map[string]any) {
				data := marshalOrdered(t, v)
				files[path] = &fstest.MapFile{Data: data, Mode: 0600}
				for i := range man.Members {
					if man.Members[i].Path == path {
						man.Members[i].Bytes, man.Members[i].SHA256 = uint64(len(data)), sha(data)
						if man.Members[i].Case == "" {
							man.Members[i].InputBytes, man.Members[i].InputSHA256 = man.Members[i].Bytes, man.Members[i].SHA256
						}
					}
				}
			}
			remove := func(path string) {
				delete(files, path)
				man.Members = slices.DeleteFunc(man.Members, func(pin kit.OracleMember) bool { return pin.Path == path })
			}
			recordPath, projectionPath, capturePath := "records/00000-c1.json", "projection/00000-c1.json", "capture/00000-c1.json"
			record, projection, capture := decode(recordPath), decode(projectionPath), decode(capturePath)
			local := record["query_semantics"].(map[string]any)
			setBytes := func(path string, data []byte) {
				files[path] = &fstest.MapFile{Data: data, Mode: 0600}
				for i := range man.Members {
					if man.Members[i].Path == path {
						man.Members[i].Bytes, man.Members[i].SHA256 = uint64(len(data)), sha(data)
					}
				}
			}
			updateProcess := func(change func(map[string]any)) {
				path := capture["process"].(map[string]any)["path"].(string)
				process := decode(path)
				change(process)
				store(path, process)
				record["process"], projection["process"] = process, process
				capture["process"] = kit.ReplayMember{Path: path, Role: "process", Bytes: uint64(len(files[path].Data)), SHA256: sha(files[path].Data)}
			}
			switch tc.name {
			case "producer-null":
				record["producer"], projection["producer"] = nil, nil
			case "producer-empty":
				record["producer"], projection["producer"] = map[string]any{}, map[string]any{}
			case "producer-field-type":
				producer := projection["producer"].(map[string]any)
				producer["language_version"] = false
				record["producer"] = producer
			case "producer-capability":
				producer := projection["producer"].(map[string]any)
				producer["reuse_observation"].(map[string]any)["policy"] = "forged"
				record["producer"] = producer
			case "expectation-false":
				record["expectations"], projection["expectations"] = []any{false}, []any{false}
			case "query-expectation-false":
				record["query_expectations"], projection["query_expectations"] = []any{false}, []any{false}
			case "schema-probe-error":
			case "orphan-raw":
				for _, v := range []map[string]any{record, projection} {
					v["execution_status"], v["assessment"], v["code"], v["process"], v["steps"] = kit.StatusFailed, kit.AssessNotAssessed, "START_FAILED", nil, []any{}
				}
				man.ExecutionStatus, man.Assessment = kit.StatusFailed, kit.AssessNotAssessed
				for i := range man.Members {
					if man.Members[i].Case != "" {
						man.Members[i].ExecutionStatus = kit.StatusFailed
					}
				}
				local["capture"] = nil
				man.Members = slices.DeleteFunc(man.Members, func(pin kit.OracleMember) bool {
					if slices.Contains([]string{"capture", "stdin-frame", "process", "stdout-frame", "stderr"}, pin.Role) {
						delete(files, pin.Path)
						return true
					}
					return false
				})
			case "incomplete-pass":
				projection["execution_status"], projection["assessment"] = kit.StatusFailed, kit.AssessPass
				record["execution_status"], record["assessment"] = kit.StatusFailed, kit.AssessPass
				man.ExecutionStatus, man.Assessment = kit.StatusFailed, kit.AssessPass
				for i := range man.Members {
					if man.Members[i].Case != "" {
						man.Members[i].ExecutionStatus = kit.StatusFailed
					}
				}
			case "projection-alias":
				delete(projection, "code")
				projection["schema"] = kit.OracleSemanticsRecordSchema
			case "raw-missing":
				remove("raw/00000-c1.json")
			case "process-false":
				record["process"], projection["process"] = false, false
			case "process-length":
				updateProcess(func(process map[string]any) { process["stdout_bytes"] = json.Number("1") })
			case "stderr-truncation":
				updateProcess(func(process map[string]any) { process["stderr_truncated"] = true })
			case "stdin-frame":
				path := capture["spec"].(map[string]any)["Stdin"].(map[string]any)["path"].(string)
				data := bytes.Clone(files[path].Data)
				clear(data[:4])
				setBytes(path, data)
				capture["spec"].(map[string]any)["Stdin"] = kit.ReplayMember{Path: path, Role: "stdin-frame", Bytes: uint64(len(data)), SHA256: sha(data)}
			case "runtime-producer":
				man.Producer["runtime_commit"] = strings.Repeat("f", 40)
			case "compiler-producer":
				man.Producer["compiler_sha256"] = strings.Repeat("f", 64)
			case "version-producer":
				man.Producer["compiler_version"] = "forged"
			case "build-argv":
				build := decode("capture/build.json")
				steps := build["steps"].([]any)
				if len(steps) == 0 {
					steps = build["reuse"].(map[string]any)["source_steps"].([]any)
				}
				steps[1].(map[string]any)["argv"].([]any)[0] = "forged-flag"
				store("capture/build.json", build)
			case "big-record":
				record["padding"] = json.RawMessage("[" + strings.Repeat("0,", 6000000) + "0]")
			case "big-manifest":
			case "unlisted-limit":
				for i := 0; i < 10000; i++ {
					files[fmt.Sprintf("extra%05d", i)] = &fstest.MapFile{Data: []byte{}, Mode: 0600}
				}
			case "depth-limit":
				files[strings.Repeat("d/", 65)+"x"] = &fstest.MapFile{Data: []byte{}, Mode: 0600}
			case "manifest-pass":
				man.Assessment = kit.AssessPass
			case "record-pass":
				record["assessment"] = kit.AssessPass
			case "projection-code":
				projection["code"] = "forged"
			case "capture-null":
				local["capture"] = nil
				man.Members = slices.DeleteFunc(man.Members, func(pin kit.OracleMember) bool {
					if slices.Contains([]string{"capture", "stdin-frame", "process", "stdout-frame", "stderr"}, pin.Role) {
						delete(files, pin.Path)
						return true
					}
					return false
				})
			case "process-missing":
				remove(capture["process"].(map[string]any)["path"].(string))
			case "stdout-pin":
				capture["stdout"].(map[string]any)["sha256"] = sha([]byte("wrong stdout"))
			case "driver-missing":
				remove("capture/driver.bin")
			case "build-input-missing":
				remove("capture/build-input-00000.bin")
			case "build-manifest-missing":
				remove("capture/build-manifest.txt")
			case "closure-drop":
				build := decode("capture/build.json")
				build["closure"] = build["closure"].([]any)[1:]
				store("capture/build.json", build)
			case "raw-frame":
				files["raw/00000-c1.json"] = &fstest.MapFile{Data: []byte("{}"), Mode: 0600}
				for i := range man.Members {
					if man.Members[i].Role == "raw" {
						man.Members[i].Bytes, man.Members[i].SHA256 = 2, sha([]byte("{}"))
					}
				}
			case "process-pair":
				projection["process"].(map[string]any)["wall_ms"] = json.Number("1234567")
				record["process"] = projection["process"]
			case "member-limit":
				man.Members[0].Bytes = kit.MaxDocumentBytes + 1
			case "total-limit":
				for i := range man.Members {
					man.Members[i].Bytes = kit.MaxDocumentBytes
				}
			case "count-limit":
				for len(man.Members) < 10000 {
					man.Members = append(man.Members, man.Members[0])
				}
			}
			// Keep enclosing hashes valid so the relationship check, rather than a stale hash, rejects the mutation.
			store(projectionPath, projection)
			for _, pin := range man.Members {
				if pin.Path == projectionPath {
					local["projection"] = kit.ReplayMember{Path: pin.Path, Role: pin.Role, Bytes: pin.Bytes, SHA256: pin.SHA256}
				}
			}
			if local["capture"] != nil {
				store(capturePath, capture)
				for _, pin := range man.Members {
					if pin.Path == capturePath {
						local["capture"] = kit.ReplayMember{Path: pin.Path, Role: pin.Role, Bytes: pin.Bytes, SHA256: pin.SHA256}
					}
				}
			}
			store(recordPath, record)
			data, err := json.Marshal(man)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "big-manifest" {
				var fields map[string]any
				d := json.NewDecoder(bytes.NewReader(data))
				d.UseNumber()
				if err := d.Decode(&fields); err != nil {
					t.Fatal(err)
				}
				fields["padding"] = json.RawMessage("[" + strings.Repeat("0,", 6000000) + "0]")
				data = marshalOrdered(t, fields)
			}
			if tc.name == "schema-probe-error" {
				data = []byte(`{"schema":0,"padding":[` + strings.Repeat("0,", 6000000) + `0],"schema":"tsgk-oracle-manifest/r5"}`)
			}
			files["manifest.json"] = &fstest.MapFile{Data: data, Mode: 0600}
			input := &semanticCountingFS{FS: files}
			result := kit.VerifyOracleSet(input)
			found := slices.ContainsFunc(result.Findings, func(f kit.Finding) bool { return f.Code == tc.code })
			if tc.name == "schema-probe-error" {
				found = len(result.Findings) == 1 && slices.Contains([]string{"JSON_TYPE", "JSON_DUPLICATE_KEY"}, result.Findings[0].Code)
			}

			if len(result.Findings) > 1000 {
				t.Fatalf("finding cap exceeded: %d", len(result.Findings))
			}
			if result.Valid || !found {
				t.Fatalf("mutation %s accepted or wrong finding: %+v", tc.name, result)
			}
			if slices.Contains([]string{"member-limit", "total-limit", "count-limit", "big-manifest", "schema-probe-error"}, tc.name) && input.opens != 1 {
				t.Fatalf("budget rejection opened %d files", input.opens)
			}
		})
	}
}

func TestOracleSemanticsBuildCaptureBounds(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "driver.exe"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	b := &Build{Dir: root, Executable: filepath.Join(root, "driver.exe"), ExecutableBytes: 1, ExecutableSHA256: sha([]byte("x"))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := semanticBuildFiles(ctx, b); !errors.Is(err, context.Canceled) {
		t.Fatalf("capture cancellation: %v", err)
	}
	b.ExecutableBytes = kit.MaxDocumentBytes + 1
	if files, err := semanticBuildFiles(context.Background(), b); err == nil || files != nil {
		t.Fatal("oversized executable was captured")
	}
	b.ExecutableBytes = 1
	b.Closure = []ClosureFile{{Bytes: kit.MaxDocumentBytes + 1}}
	if files, err := semanticBuildFiles(context.Background(), b); err == nil || files != nil {
		t.Fatal("oversized closure was captured")
	}
}

func TestOracleSemanticsFailureClassification(t *testing.T) {
	caller, cancel := context.WithCancel(context.Background())
	resource := refuse(kit.KindResourceLimit, "SEMANTICS_MEMBER_LIMIT", nil)
	status, assessment, err := semanticFailure(caller, caller, resource)
	if status != kit.StatusResourceLimit || assessment != kit.AssessBlocked || err != resource {
		t.Fatalf("typed resource lost: %s/%s %v", status, assessment, err)
	}
	cancelled := refuse(kit.KindCancelled, "CANCELLED", context.Canceled)
	cancel()
	status, assessment, err = semanticFailure(caller, caller, cancelled)
	if status != kit.StatusCancelled || assessment != kit.AssessNotAssessed || err != cancelled {
		t.Fatal("original cancellation lost")
	}
	live, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	status, assessment, err = semanticFailure(context.Background(), live, cancelled)
	var ne *Error
	if !errors.As(err, &ne) || status != kit.StatusResourceLimit || assessment != kit.AssessBlocked || ne.Kind != kit.KindResourceLimit || ne.Code != "WALL_LIMIT" {
		t.Fatalf("orchestration wall lost: %s/%s %v", status, assessment, err)
	}
	buildLimit := refuse(kit.KindIO, "BUILD_RESOURCE_LIMIT", nil)
	status, assessment, err = semanticFailure(context.Background(), context.Background(), buildLimit)
	if !errors.As(err, &ne) || status != kit.StatusResourceLimit || assessment != kit.AssessBlocked || ne.Kind != kit.KindResourceLimit || ne.Code != buildLimit.Code || !errors.Is(err, buildLimit) {
		t.Fatal("compiler resource boundary lost")
	}
}

func TestOracleSemanticsProducerIncompatibleABI(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("new semantics producer is deferred on macOS")
	}
	profile, root := oracleProfile(t, []kit.OracleQuery{{ID: "q", Source: "(identifier) @id"}}, "a = 1;")
	var doc map[string]any
	if err := json.Unmarshal(profile, &doc); err != nil {
		t.Fatal(err)
	}
	grammarRoot := t.TempDir()
	copyTree(t, fixtureRoot(t, "plain"), grammarRoot)
	parser := filepath.Join(grammarRoot, "src", "parser.c")
	data, err := os.ReadFile(parser)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(data, []byte("#define LANGUAGE_VERSION 15"), []byte("#define LANGUAGE_VERSION 99"), 1)
	if bytes.Equal(changed, data) {
		t.Fatal("ABI control did not change grammar")
	}
	if err := os.WriteFile(parser, changed, 0600); err != nil {
		t.Fatal(err)
	}
	grammar := fixtureGrammar(t, "plain")
	grammar[0].SHA256, grammar[0].Bytes = digestOf(t, parser)
	registration := []byte(`{"producer_control":"not_external_approval"}`)
	doc["schema"], doc["query_semantics_registration_sha256"], doc["grammar"] = kit.OracleSemanticsSchema, sha(registration), grammar
	profile, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "set")
	req := oracleRequest(t, profile, root, out)
	req.GrammarRoot, req.QuerySemanticsRegistration = grammarRoot, registration
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := Oracle(ctx, req)
	if err != nil || result.ExecutionStatus != kit.StatusFailed || result.Set == nil || !result.Set.Valid || len(result.Cases) != 1 || result.Cases[0].Code != "LANGUAGE_INCOMPATIBLE" {
		t.Fatalf("honest incompatible ABI rejected: %v status=%s set=%+v cases=%+v", err, result.ExecutionStatus, result.Set, result.Cases)
	}
	record, err := os.ReadFile(filepath.Join(out, "records", "00000-ca.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved OracleRecord
	if err := json.Unmarshal(record, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Producer == nil || saved.Producer.LanguageVersion != 99 || saved.Code != "LANGUAGE_INCOMPATIBLE" || saved.QuerySemantics == nil || saved.QuerySemantics.Claim != ClaimBlocked {
		t.Fatal("observed ABI or original failure was replaced")
	}
}
