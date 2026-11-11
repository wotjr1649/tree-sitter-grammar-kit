package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This test asks the native package to make the r8, then consumes those exact files with the
// ordinary reader and handwritten expectations in this package.
func TestOrdinaryOwnedNativeR8ReaderExpectedVerified(t *testing.T) {
	if os.Getenv("TSGK_NATIVE_RUNTIME") == "" || os.Getenv("TSGK_NATIVE_CC") == "" {
		t.Skip("ordinary owned native integration requires the pinned runtime and compiler")
	}
	fixtureDir := filepath.Join(t.TempDir(), "fixture")
	rootDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "./src/internal/native", "-run", "^TestOracleOrdinaryOwnedFixture$", "-count=1", "-timeout=180s")
	cmd.Dir = rootDir
	cmd.Env = ordinaryOwnedNativeEnv(fixtureDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native owned r8 producer failed: %v\n%s", err, output)
	}
	var producer struct {
		Profile    []byte `json:"profile"`
		RunBinding []byte `json:"run_binding"`
		RunID      string `json:"run_id"`
	}
	if err := decodeJSONFile(filepath.Join(fixtureDir, "producer.json"), &producer); err != nil {
		t.Fatal(err)
	}
	profile, err := ParseOracleProfile(producer.Profile)
	if err != nil {
		t.Fatal(err)
	}
	setDir := filepath.Join(fixtureDir, "r8")
	manifestBytes, err := os.ReadFile(filepath.Join(setDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest OrdinaryOracleManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != OracleOrdinaryManifestSchema || manifest.Assessment != AssessBlocked || !manifest.Complete || len(manifest.Cases) != 1 {
		t.Fatalf("producer r8 is not a complete pending set: assessment=%s complete=%t cases=%v", manifest.Assessment, manifest.Complete, manifest.Cases)
	}
	fixture := newOwnedNativeReaderFixture(t, setDir, manifest, manifestBytes, producer.RunBinding, profile)
	set, e := fixture.b.ordinaryGuardedSet(fixture.root, fixture.s, fixture.registration, fixture.profile, fixture.manifest, fixture.runBinding)
	if e != nil || set == nil || !set.Complete || set.Assessment != AssessBlocked {
		if set == nil {
			t.Fatalf("ordinary r8 reader: error=%v; no set", e)
		}
		t.Fatalf("ordinary r8 reader: error=%v assessment=%s complete=%t code=%s findings=%+v", e, set.Assessment, set.Complete, set.Code, set.Findings)
	}
	testOrdinaryNativeClosureOwnedFixture(t, fixture, set)
	cache := set.Cases[profile.Native.Cases[0].ID]
	if cache == nil || !cache.ready || len(cache.checked) != 2 {
		batch := set.Batches["D_batch_00000"]
		var frames []string
		if batch != nil {
			for _, frame := range batch.Capture.Frames {
				frames = append(frames, frame.Case+"/"+frame.State)
			}
		}
		t.Fatalf("reader did not accept both native versions: cache=%+v profileCase=%s regCases=%d manifestCases=%v batches=%v batchFrames=%v frameCache=%v records=%v assessment=%s findings=%+v", cache, profile.Native.Cases[0].ID, len(fixture.registration.Cases), set.Manifest.Cases, mapKeys(set.Batches), frames, batchKeys(batch), mapKeys(set.Records), set.Assessment, set.Findings)
	}
	base, baseErr := fixture.b.recomputeOrdinaryBase(profile, 0, cache)
	if baseErr != nil || base.Verdict.Assessment != AssessPass {
		t.Fatalf("owned native raw base recomputation: err=%v verdict=%+v gates=%+v findings=%+v", baseErr, base.Verdict, base.Gates, base.Findings)
	}
	caseReg := fixture.registration.Cases[0]
	id := ordinaryTupleIdentity{Case: caseReg.ID, Query: profile.Queries[0].ID, Step: 1, TreeRole: "incremental", Source: caseReg.VersionPins[1]}
	metadata := QueryWitnessMetadata{Patterns: 1, CaptureNames: []string{"id"}, Predicates: []QueryPredicate{}}
	version := cache.versions[id.Step]
	expectedTree, captures := ordinaryOwnedExpected(id, version)
	judgment, e := adjudicateOrdinaryTuple(fixture.b, id, cache.checked[id.Step].Incremental, version, ordinaryJSON(t, metadata), ordinaryJSON(t, expectedTree), ordinaryJSON(t, captures), nil, nil)
	if e != nil || judgment.State != ordinaryTupleVerified || judgment.Code != "" || len(judgment.Findings) != 0 {
		t.Fatalf("owned native tuple vs independent expectation: err=%v judgment=%+v", e, judgment)
	}
}

func mapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func batchKeys(batch *ordinaryBatchCache) []string {
	if batch == nil {
		return nil
	}
	return mapKeys(batch.Frames)
}

func ordinaryOwnedExpected(id ordinaryTupleIdentity, source []byte) (ordinaryExpectedTree, ordinaryExpectedCaptures) {
	lines := strings.Split(string(source), "\n")
	lastLine := lines[len(lines)-1]
	tree := ordinaryExpectedTree{Schema: ordinaryExpectedTreeSchema, Case: id.Case, Step: id.Step, TreeRole: id.TreeRole, Source: id.Source,
		Tree: []TreeNode{{Parent: -1, Type: "source_file", Named: true, EndByte: uint32(len(source)), EndPoint: Point{Row: uint32(len(lines) - 1), Column: uint32(len(lastLine))}}}}
	captures := ordinaryExpectedCaptures{Schema: ordinaryExpectedCaptureSchema, Case: id.Case, Query: id.Query, Step: id.Step, TreeRole: id.TreeRole, Source: id.Source, Ordered: []Capture{}}
	appendNode := func(parent int64, typ string, field *string, named bool, start, end, row, startColumn, endColumn uint32) int64 {
		tree.Tree = append(tree.Tree, TreeNode{Parent: parent, Type: typ, Field: field, Named: named, StartByte: start, EndByte: end, StartPoint: Point{Row: row, Column: startColumn}, EndPoint: Point{Row: row, Column: endColumn}})
		return int64(len(tree.Tree) - 1)
	}
	appendCapture := func(node int64, start, end, row, startColumn, endColumn uint32) {
		captures.Ordered = append(captures.Ordered, Capture{Match: uint32(len(captures.Ordered)), Pattern: 0, Capture: 0, Name: "id", Node: node, Type: "identifier", Named: true, StartByte: start, EndByte: end, StartPoint: Point{Row: row, Column: startColumn}, EndPoint: Point{Row: row, Column: endColumn}})
	}
	offset := uint32(0)
	for i, line := range lines {
		if line == "" {
			continue
		}
		row, lineBytes := uint32(i), uint32(len(line))
		end := offset + lineBytes
		assignment := appendNode(0, "assignment", nil, true, offset, end, row, 0, lineBytes)
		nameField, valueField := "name", "value"
		if i == 0 {
			left := appendNode(assignment, "identifier", &nameField, true, offset, offset+1, row, 0, 1)
			appendCapture(left, offset, offset+1, row, 0, 1)
			appendNode(assignment, "=", nil, false, offset+2, offset+3, row, 2, 3)
			call := appendNode(assignment, "call", &valueField, true, offset+4, offset+8, row, 4, 8)
			function := appendNode(call, "identifier", stringField("function"), true, offset+4, offset+5, row, 4, 5)
			appendCapture(function, offset+4, offset+5, row, 4, 5)
			appendNode(call, "(", nil, false, offset+5, offset+6, row, 5, 6)
			appendNode(call, "number", nil, true, offset+6, offset+7, row, 6, 7)
			appendNode(call, ")", nil, false, offset+7, offset+8, row, 7, 8)
			appendNode(assignment, ";", nil, false, offset+8, offset+9, row, 8, 9)
		} else {
			name := appendNode(assignment, "identifier", &nameField, true, offset, offset+1, row, 0, 1)
			appendCapture(name, offset, offset+1, row, 0, 1)
			appendNode(assignment, "=", nil, false, offset+2, offset+3, row, 2, 3)
			appendNode(assignment, "number", &valueField, true, offset+4, offset+5, row, 4, 5)
			appendNode(assignment, ";", nil, false, offset+5, offset+6, row, 5, 6)
		}
		offset = end + 1
	}
	captures.Matches = uint64(len(captures.Ordered))
	return tree, captures
}

func stringField(value string) *string { return &value }

func ordinaryOwnedNativeEnv(dir string) []string {
	values := map[string]string{
		"TSGK_ORDINARY_OWNED_FIXTURE": dir,
		"TSGK_NATIVE_RUNTIME":         `D:\AIDEV\tree-sitter-grammar-kit\.work\352-data\s05\runtime`,
		"TSGK_NATIVE_CC":              `C:\msys64\ucrt64\bin\gcc.exe`,
		"TSGK_NATIVE_REQUIRED":        "1",
	}
	env := make([]string, 0, len(os.Environ())+len(values))
	for _, entry := range os.Environ() {
		key := entry
		if i := strings.IndexByte(entry, '='); i >= 0 {
			key = entry[:i]
		}
		if _, replace := values[strings.ToUpper(key)]; !replace {
			env = append(env, entry)
		}
	}
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}

type ordinaryOwnedReaderFixture struct {
	root         *os.Root
	b            *eligibilityBudget
	s            *eligibilitySession
	registration OrdinaryRegistration
	profile      OracleProfile
	manifest     []byte
	runBinding   []byte
}

func newOwnedNativeReaderFixture(t *testing.T, setDir string, manifest OrdinaryOracleManifest, manifestBytes, runBinding []byte, profile OracleProfile) ordinaryOwnedReaderFixture {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "oracle"), 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	copyErr := filepath.WalkDir(setDir, func(source string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(setDir, source)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, "oracle", rel)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0600)
	})
	if copyErr != nil {
		t.Fatal(copyErr)
	}
	if string(profile.Native.Operation) != "native-query-batch" || len(profile.Native.Cases) != 1 || len(profile.Queries) != 1 {
		t.Fatalf("unexpected producer profile: operation=%s cases=%d queries=%d", profile.Native.Operation, len(profile.Native.Cases), len(profile.Queries))
	}
	caseProfile := profile.Native.Cases[0]
	initial, err := os.ReadFile(filepath.Join(setDir, "source", fmt.Sprintf("%05d-%s.bin", 0, caseProfile.ID)))
	if err != nil {
		t.Fatal(err)
	}
	versions, points, err := ApplyEdits(EncodingUTF8, initial, caseProfile.Edits, 65536)
	if err != nil {
		t.Fatal(err)
	}
	casePins := make([]OrdinaryPin, len(versions))
	for i, version := range versions {
		casePins[i] = OrdinaryPin{Path: "source-witness/" + digestHex(version) + ".utf8", Bytes: uint64(len(version)), SHA256: digestHex(version)}
	}
	registration := OrdinaryRegistration{Namespace: "ordinary-owned-test/r1", Route: profile.Native.Route, Platform: "windows-amd64", Subject: "windows/amd64", Context: OrdinaryContext, ClosureEncoding: "plain",
		Cases:   []OrdinaryCase{{ID: caseProfile.ID, InitialSource: casePins[0], Edits: caseProfile.Edits, VersionPins: casePins, Points: points}},
		Queries: []OrdinaryQuery{{ID: profile.Queries[0].ID, Source: OrdinaryPin{Path: "queries/q0.scm", Bytes: uint64(len(profile.Queries[0].Source)), SHA256: digestHex([]byte(profile.Queries[0].Source))}}}}
	ordinary := ordinaryEligibilityBinding{Schema: ordinaryEligibilityBindingSchema, Context: manifest.Context, ClosureEncoding: manifest.ClosureEncoding,
		Profile: manifest.Profile, PlanningProfile: manifest.PlanningProfile, Registration: manifest.Registration, Assets: manifest.Assets, Plan: manifest.Plan,
		Commitment:         OrdinaryPin{Path: "caller/plan-commitment.json", Bytes: 1, SHA256: digestHex([]byte("c"))},
		ReferenceSelection: OrdinaryPin{Path: "caller/reference-selection.json", Bytes: 1, SHA256: digestHex([]byte("r"))},
		RunBinding:         OrdinaryPin{Path: "caller/run-binding.json", Bytes: uint64(len(runBinding)), SHA256: digestHex(runBinding)},
		Candidate:          manifest.Candidate, SemanticCohort: manifest.SemanticCohort, RunID: manifest.RunID, ShardIDs: manifest.ShardIDs}
	b := ordinaryTestBudget(t)
	snapshot := &evidenceSnapshot{graphBound: true, invalidNodes: map[string]bool{}, files: map[string][]byte{}, verifiedMembers: map[string]ReplayMember{}, observed: map[string]os.FileInfo{}, directories: map[string]os.FileInfo{"oracle": nil}}
	ordinaryDir, err := root.Stat("oracle")
	if err != nil {
		t.Fatal(err)
	}
	snapshot.directories["oracle"] = ordinaryDir
	owned, owners := map[string]eligibilityMember{}, map[string]string{}
	addMember := func(owner, path, role string, pin OrdinaryPin) {
		full := "oracle/" + path
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(full)))
		if err != nil {
			t.Fatal(err)
		}
		member := ReplayMember{Path: full, Role: role, Bytes: pin.Bytes, SHA256: pin.SHA256}
		owned[full], owners[full] = eligibilityMember{NodeID: "node-" + owner, Member: member}, "node-"+owner
		snapshot.verifiedMembers[full], snapshot.files[full] = member, data
		info, err := root.Stat(full)
		if err != nil {
			t.Fatal(err)
		}
		snapshot.observed[full] = info
	}
	for _, member := range manifest.Members {
		addMember(member.Owner, member.Path, member.Role, member.Pin)
	}
	manifestPin := OrdinaryPin{Path: "oracle/manifest.json", Bytes: uint64(len(manifestBytes)), SHA256: digestHex(manifestBytes)}
	addMember("O", "manifest.json", "manifest", manifestPin)
	session := &eligibilitySession{r: b.r, b: b, snapshot: snapshot, owned: owned, owners: owners, bound: true,
		res: &QueryReferenceEligibilityResult{Report: newReport("ordinary owned fixture"), Complete: true}}
	session.res.Assessment = AssessPass
	session.ordinary = &ordinaryEligibilityScope{Binding: ordinary, ClosureEncoding: "plain", TupleRequired: 1, session: session}
	return ordinaryOwnedReaderFixture{root: root, b: b, s: session, registration: registration, profile: profile, manifest: manifestBytes, runBinding: runBinding}
}

func decodeJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
