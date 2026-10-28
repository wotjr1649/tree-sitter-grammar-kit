package kit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// Synthetic approvals prove offline consistency, never actual process execution.
func semanticBoundFixture(t *testing.T, registrationPadding int, omittedGrammarRoles ...string) (QuerySemanticsRequest, *eligibilityFixture, semanticRunBinding) {
	t.Helper()
	return semanticBoundFixtureQueries(t, registrationPadding, true, omittedGrammarRoles...)
}

func semanticBoundFixtureQueries(t *testing.T, registrationPadding int, withQueries bool, omittedGrammarRoles ...string) (QuerySemanticsRequest, *eligibilityFixture, semanticRunBinding) {
	return semanticBoundFixtureMode(t, registrationPadding, withQueries, false, omittedGrammarRoles...)
}

func semanticBoundFixtureMode(t *testing.T, registrationPadding int, withQueries, paged bool, omittedGrammarRoles ...string) (QuerySemanticsRequest, *eligibilityFixture, semanticRunBinding) {
	t.Helper()
	runtimeRoot := os.Getenv("TSGK_NATIVE_RUNTIME")
	if runtimeRoot == "" {
		t.Skip("pinned runtime source closure is not configured")
	}
	f := newEligibilityFixture(t)
	var parserData []byte
	if paged {
		parserData = bytes.Repeat([]byte("p"), MaxDocumentBytes+1)
		old := f.reg.Native.Grammar["parser.c"]
		pin := old
		pin.Member.Bytes = uint64(len(parserData))
		pin.Member.SHA256 = digestHex(parserData)
		f.reg.Native.Grammar["parser.c"] = pin
		text := f.files[f.reg.Native.BuildManifest.Member.Path]
		text = bytes.Replace(text, []byte(fmt.Sprintf("file=grammar-parser parser.c %s %d\n", old.Member.SHA256, old.Member.Bytes)), []byte(fmt.Sprintf("file=grammar-parser parser.c %s %d\n", pin.Member.SHA256, pin.Member.Bytes)), 1)
		f.nativeManifestMutation(func(string) string { return string(text) })
		f.reg.Context = semanticPagedContext
	}
	f.witness.Input.Role = "case"
	op := NativeOperations()["native-query"]
	f.witness.Native.NodeLimit = op.Nodes
	caseID := f.witness.Case
	c := IncrementalCase{ID: caseID, Input: f.witness.Input, Encoding: EncodingUTF8, Edits: f.witness.Edits, Points: []NativePoint{}, Expect: []StepExpectation{{Step: 0, Syntax: "NO_ERROR", Contains: []string{}}, {Step: 1, Syntax: "NO_ERROR", Contains: []string{}}}}
	compiler := ToolIdentity{Name: "cc", Version: "synthetic-cc", SHA256: digestHex([]byte("synthetic cc")), Bytes: 3}
	profile := OracleProfile{Native: IncrementalProfile{Schema: OracleSemanticsSchema, ID: "semantic-bound-synthetic", Route: f.witness.Route, Operation: "native-query", Symbol: f.reg.Native.Symbol, Encoding: EncodingUTF8, Output: OutputTree, Compiler: compiler, Cases: []IncrementalCase{c}}, Queries: []OracleQuery{{ID: "q0", Source: f.witness.Queries[0].Source}}, OracleCases: []OracleCase{{IncrementalCase: c, QueryExpect: []QueryExpectation{}}}}
	if !withQueries {
		profile.Queries = []OracleQuery{}
	}
	grammar := f.reg.Native.Grammar
	names := sortedKeys(grammar)
	for _, name := range names {
		pin := grammar[name].Member
		if !slices.Contains(omittedGrammarRoles, pin.Role) {
			profile.Native.Grammar = append(profile.Native.Grammar, NativeInput{Path: name, Role: strings.TrimPrefix(pin.Role, "grammar-"), Bytes: pin.Bytes, SHA256: pin.SHA256})
		}
	}
	files, roles := map[string][]byte{}, map[string]string{}
	add := func(path, role string, data []byte) ReplayMember {
		t.Helper()
		if _, exists := files[path]; exists {
			t.Fatalf("duplicate semantic fixture path: %s", path)
		}
		files[path], roles[path] = data, role
		return ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)}
	}
	buildText := f.files[f.reg.Native.BuildManifest.Member.Path]
	build := semanticBuildDescription{Identity: digestHex(buildText), Compiler: compiler, CompilerVersion: compiler.Version, Symbol: profile.Native.Symbol, Closure: []semanticBuildInput{}, Steps: []semanticBuildStep{}}
	buildLines := strings.Split(strings.TrimSpace(string(buildText)), "\n")
	if paged {
		rank := func(line string) int {
			if strings.HasPrefix(line, "file=runtime ") {
				return 1
			}
			if strings.HasPrefix(line, "file=grammar-") {
				return 2
			}
			if strings.HasPrefix(line, "file=driver ") {
				return 3
			}
			return 0
		}
		slices.SortStableFunc(buildLines, func(a, b string) int { return rank(a) - rank(b) })
	}
	for _, line := range buildLines {
		if strings.HasPrefix(line, "runtime=") {
			build.Runtime = strings.TrimPrefix(line, "runtime=")
		}
		if strings.HasPrefix(line, "step=") {
			name, argv, ok := eligibilityBuildStep(strings.TrimPrefix(line, "step="))
			if !ok {
				t.Fatal("invalid fixed synthetic build step")
			}
			build.Steps = append(build.Steps, semanticBuildStep{Name: name, Argv: argv})
		}
		if !strings.HasPrefix(line, "file=") {
			continue
		}
		row := strings.Fields(strings.TrimPrefix(line, "file="))
		if len(row) != 4 {
			t.Fatal("invalid fixed synthetic build input")
		}
		size, err := strconv.ParseUint(row[3], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		input := semanticBuildInput{Role: row[0], Path: row[1], SHA256: row[2], Bytes: size}
		var data []byte
		switch {
		case input.Role == "runtime":
			data, err = os.ReadFile(filepath.Join(runtimeRoot, filepath.FromSlash(input.Path)))
		case input.Role == "driver" && input.Path == "driver/shim.c":
			data = []byte("#include <tree_sitter/api.h>\nconst TSLanguage *tree_sitter_tsgk_plain(void);\nconst TSLanguage *tsgk_language(void) { return tree_sitter_tsgk_plain(); }\n")
		case input.Role == "driver":
			data, err = drivers.NativeC.ReadFile("native-c/" + strings.TrimPrefix(input.Path, "driver/"))
		default:
			data = f.files[grammar[input.Path].Member.Path]
			if paged && input.Role == "grammar-parser" {
				data = parserData
			}
		}
		if err != nil || uint64(len(data)) != input.Bytes || digestHex(data) != input.SHA256 {
			t.Fatalf("pinned input differs: %s %s: %v", input.Role, input.Path, err)
		}
		path := fmt.Sprintf("capture/build-input-%05d.bin", len(build.Closure))
		var pin ReplayMember
		if paged && input.Role == "grammar-parser" {
			index := len(build.Closure)
			d := semanticParserDescriptor{Schema: "tsgk-query-parser-pages/r1", Owner: "D"}
			d.Logical.ClosureIndex = uint64(index)
			d.Logical.Path = input.Path
			d.Logical.Role = input.Role
			d.Logical.Bytes = input.Bytes
			d.Logical.SHA256 = input.SHA256
			for offset := 0; offset < len(data); {
				i := len(d.Parts)
				size := min(MaxDocumentBytes, len(data)-offset)
				part := data[offset : offset+size]
				p := add(fmt.Sprintf("capture/build-input-%05d.part-%05d.bin", index, i), "grammar-paged-part", part)
				d.Parts = append(d.Parts, struct {
					Index  uint64 `json:"index"`
					Offset uint64 `json:"offset"`
					Path   string `json:"path"`
					Bytes  uint64 `json:"bytes"`
					SHA256 string `json:"sha256"`
				}{uint64(i), uint64(offset), p.Path, p.Bytes, p.SHA256})
				offset += size
			}
			pin = add(fmt.Sprintf("capture/build-input-%05d.pages.json", index), "grammar-paged-descriptor", eligibilityJSON(t, d))
		} else {
			pin = add(path, semanticBuildMemberRole(input.Role), data)
		}
		if strings.HasPrefix(input.Role, "grammar-") {
			f.reg.Native.Grammar[input.Path] = eligibilityMember{NodeID: "native-current", Member: ReplayMember{Path: "oracle/" + pin.Path, Role: pin.Role, Bytes: pin.Bytes, SHA256: pin.SHA256}}
		}
		build.Closure = append(build.Closure, input)
	}
	exe := f.files["native/executable.bin"]
	build.ExecutableSHA256, build.ExecutableBytes = digestHex(exe), uint64(len(exe))
	add("capture/driver.bin", "driver", exe)
	add("capture/build.json", "build", eligibilityJSON(t, build))
	add("capture/build-manifest.txt", "build-manifest", buildText)
	add("source/00000-"+caseID+".bin", "source", f.files["native/source.bin"])
	var raw nativewire.Response
	if err := jsonv2.Unmarshal(f.files["native/response.json"], &raw); err != nil {
		t.Fatal(err)
	}
	raw.Steps[1].Route = &nativewire.Route{EditHasChanges: true, EditedRootEndByte: 6, ReusedNodes: 1}
	if !withQueries {
		for i := range raw.Steps {
			raw.Steps[i].Incremental.Queries = jsontext.Value("[]")
			if raw.Steps[i].Fresh != nil {
				raw.Steps[i].Fresh.Queries = jsontext.Value("[]")
			}
		}
	}
	rawData := eligibilityJSON(t, raw)
	add("raw/00000-"+caseID+".json", "raw", rawData)
	frame := func(data []byte) []byte {
		out := make([]byte, 4+len(data))
		binary.BigEndian.PutUint32(out, uint32(len(data)))
		copy(out[4:], data)
		return out
	}
	nativeRequest := semanticNativeRequest{Protocol: nativewire.ProtocolR4, ID: caseID, Encoding: EncodingUTF8, Output: OutputTree, Limits: semanticLimitsFor(op), Declarations: []NativeDeclaration{}, Points: []NativePoint{}, Ranges: [][]semanticRequestRange{}, Source: base64.StdEncoding.EncodeToString(f.files["native/source.bin"]), Edits: []semanticRequestEdit{{StartByte: 0, OldEndByte: 1, NewEndByte: 1, Old: base64.StdEncoding.EncodeToString([]byte("x")), New: base64.StdEncoding.EncodeToString([]byte("y"))}}, Queries: []semanticRequestQuery{{ID: "q0", Source: base64.StdEncoding.EncodeToString([]byte(f.witness.Queries[0].Source))}}, ReuseObservation: reuseobs.Policy, ObservationRunID: f.witness.Native.ObservationRunID, ObservationProducerSHA256: build.Identity}
	if !withQueries {
		nativeRequest.Queries = []semanticRequestQuery{}
	}
	stdin := add("capture/stdin.bin", "stdin-frame", frame(eligibilityJSON(t, nativeRequest)))
	stdout := add("capture/stdout.bin", "stdout-frame", frame(rawData))
	stderr := add("capture/stderr.bin", "stderr", []byte{})
	p := f.witness.Native.Process
	execution := eligibilityFixtureExecution(p, 0, false, frame(rawData), []byte{})
	process := add("capture/process.json", "process", eligibilityJSON(t, execution))
	spec := &semanticCaptureSpec{Path: p.Path, Args: p.Args, Dir: p.Dir, Env: p.Env, Stdin: &stdin, StdoutBytes: int64(p.StdoutBytes), StderrBytes: int64(p.StderrBytes), Wall: int64(p.Wall), Grace: int64(p.Grace), Memory: p.Memory, CgroupParent: p.CgroupParent}
	capture := add("capture/process-capture.json", "capture", eligibilityJSON(t, semanticProcessCapture{Schema: "tsgk-native-process-capture/r1", Stage: "RUN_RETURNED", Spec: spec, Process: &process, Stdout: &stdout, Stderr: &stderr}))
	steps := []any{}
	for i, step := range raw.Steps {
		projectTree := func(tree *nativewire.WireTree) any {
			if tree == nil {
				return nil
			}
			caps := []Capture{}
			if i == 0 {
				caps = slices.Clone(f.witness.Required[0].Expected)
			}
			q := map[string]any{"id": "q0", "sha256": digestHex([]byte(f.witness.Queries[0].Source)), "status": StatusCompleted, "code": "", "evaluation": "EVALUATED", "query_ms": 0, "error": nil, "patterns": 1, "capture_names": []string{"x"}, "predicates": f.witness.Queries[0].Metadata.Predicates, "matches": 1, "partial": false, "captures": caps}
			queries := []any{}
			if withQueries {
				queries = append(queries, q)
			}
			return map[string]any{"status": StatusCompleted, "code": "", "form": "full", "descendant_count": 7, "has_error": false, "digest": tree.Digest, "declarations": tree.Declarations, "tree": map[string]any{"schema": TreeSchema, "input": map[string]any{"encoding": EncodingUTF8, "encoding_source": SourceDeclaration, "bytes": step.SourceBytes, "sha256": step.SourceSHA256}, "status": StatusCompleted, "nodes": f.cat.Trees["tree-0"]}, "queries": queries}
		}
		var route, comparison, queryComparison any
		if i > 0 {
			route = map[string]any{"edit_has_changes": true, "edited_root_end_byte": 6, "reused_nodes": 1, "fresh_reused_nodes": 0, "proven": true}
			comparison = map[string]any{"equal": true, "first_difference": nil}
			if withQueries {
				queryComparison = map[string]any{"equal": true}
			}
		}
		steps = append(steps, map[string]any{"step": i, "source_bytes": step.SourceBytes, "source_sha256": step.SourceSHA256, "edit": step.Edit, "route": route, "comparison": comparison, "query_comparison": queryComparison, "incremental": projectTree(&step.Incremental), "fresh": projectTree(step.Fresh), "observations": step.Observations})
	}
	expectations := []semanticExpectationResult{}
	for _, expected := range c.Expect {
		expectations = append(expectations, semanticExpectationResult{Step: expected.Step, Syntax: expected.Syntax, Contains: expected.Contains, Declarations: expected.Declarations, Result: claimPass})
	}
	projection := map[string]any{"id": caseID, "input": c.Input, "encoding": EncodingUTF8, "execution_status": StatusCompleted, "assessment": AssessPass, "code": "", "claims": map[string]string{"incremental_equality": claimPass, "incremental_route": claimPass, "expectations": claimPass}, "oracle_claims": map[string]string{"query_equality": claimPass, "query_expectations": claimNotClaimed, "api": claimNotClaimed, "fact_reproduction": claimNotClaimed, "dynamic_sql": claimNotClaimed}, "response_status": StatusCompleted, "response_code": "", "producer": raw.Producer, "process": execution, "steps": steps, "expectations": expectations, "query_expectations": []semanticQueryExpectationResult{}, "reuse_observation_claim": claimPass}
	projectionPin := add("projection/00000-"+caseID+".json", "projection", eligibilityJSON(t, projection))
	roles["records/00000-"+caseID+".json"] = "record"
	roles["manifest.json"] = "manifest"
	reg := semanticRegistration{QueryWitnessRegistration: f.witness.QueryWitnessRegistration, Context: "full-input-utf8/r2", Profile: semanticMeaning(profile), NativePolicy: NativePolicy(op, OutputTree, nil), Limits: semanticLimitsFor(op), Classes: []string{"semantic-required", "semantic-required", "semantic-required"}, PlannedMembers: []semanticPlannedMember{}}
	if !withQueries {
		reg.Queries, reg.Required, reg.Classes = reg.Queries[:0], reg.Required[:0], reg.Classes[:0]
		projection["oracle_claims"].(map[string]string)["query_equality"] = claimNotClaimed
		files[projectionPin.Path] = eligibilityJSON(t, projection)
		projectionPin.Bytes, projectionPin.SHA256 = uint64(len(files[projectionPin.Path])), digestHex(files[projectionPin.Path])
	}
	if paged {
		reg.Context = semanticPagedContext
	}
	reg.Schema = querySemanticsRegistrationSchema
	for _, path := range sortedKeys(roles) {
		owner := "D"
		if slices.Contains([]string{"manifest", "record", "projection", "source"}, roles[path]) {
			owner = "O"
		}
		reg.PlannedMembers = append(reg.PlannedMembers, semanticPlannedMember{Owner: owner, Path: path, Role: roles[path]})
	}
	registration := eligibilityJSON(t, reg)
	if registrationPadding > 0 {
		if len(registration) >= registrationPadding {
			t.Fatal("registration padding is too small")
		}
		registration = append(registration, bytes.Repeat([]byte(" "), registrationPadding-len(registration))...)
	}
	profile.QuerySemanticsRegistrationSHA256 = digestHex(registration)
	var profileObject map[string]jsontext.Value
	if err := jsonv2.Unmarshal(eligibilityJSON(t, semanticMeaning(profile)), &profileObject); err != nil {
		t.Fatal(err)
	}
	profileObject["schema"], profileObject["query_semantics_registration_sha256"] = eligibilityJSON(t, OracleSemanticsSchema), eligibilityJSON(t, profile.QuerySemanticsRegistrationSHA256)
	profileData := eligibilityJSON(t, profileObject)
	record := map[string]any{}
	for key, value := range projection {
		record[key] = value
	}
	record["schema"], record["case"], record["workload"] = OracleSemanticsRecordSchema, caseID, profile.Native.ID
	record["assessment"], record["code"] = AssessBlocked, "EXTERNAL_APPROVAL_PENDING"
	record["query_semantics"] = OracleQuerySemantics{Phase: "PRODUCER_LOCAL", Claim: AssessBlocked, Code: "EXTERNAL_APPROVAL_PENDING", RegistrationSHA256: digestHex(registration), Capture: &capture, Projection: projectionPin}
	recordData := eligibilityJSON(t, record)
	recordData = append(recordData[:len(recordData)-1], []byte(`,"complete":true}`)...)
	add("records/00000-"+caseID+".json", "record", recordData)
	manifest := OracleManifest{Schema: OracleSemanticsManifestSchema, QuerySemanticsRegistrationSHA256: digestHex(registration), Operation: &op, Workload: map[string]string{"profile_sha256": digestHex(profileData), "profile_id": profile.Native.ID, "route": reg.Route, "operation": "native-query", "output": OutputTree}, Producer: map[string]string{"platform": reg.Platform, "build_identity": build.Identity, "executable_sha256": build.ExecutableSHA256, "compiler_sha256": compiler.SHA256, "compiler_version": compiler.Version, "runtime_commit": build.Runtime}, Policy: reg.NativePolicy, Protocol: nativewire.ProtocolR4, Comparators: []string{"ordered-cst/r1", "ordered-captures/r1"}, Queries: []IdentityRef{}, ExecutionStatus: StatusCompleted, Assessment: AssessBlocked, Records: 1, Cases: []string{caseID}, Members: []OracleMember{}, Complete: true}
	if paged {
		manifest.Schema = OraclePagedSemanticsManifestSchema
	}
	if withQueries {
		manifest.Queries = []IdentityRef{{Role: "query:q0", Schema: "tsgk-query-source/r1", SHA256: digestHex([]byte(profile.Queries[0].Source))}}
	}
	for _, path := range sortedKeys(files) {
		data, role := files[path], roles[path]
		member := OracleMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data), ExecutionStatus: StatusCompleted, InputBytes: uint64(len(data)), InputSHA256: digestHex(data)}
		if slices.Contains([]string{"record", "projection", "source", "raw", "capture", "stdin-frame", "process", "stdout-frame", "stderr"}, role) {
			member.Case, member.InputBytes, member.InputSHA256 = caseID, c.Input.Bytes, c.Input.SHA256
		}
		manifest.Members = append(manifest.Members, member)
	}
	manifestData := mustJSON(t, manifest)
	files["manifest.json"] = manifestData
	binding := semanticRunBinding{Schema: "tsgk-query-semantics-run-binding/r1", ProfileSHA256: digestHex(profileData), ManifestSHA256: digestHex(manifestData), RegistrationSHA256: digestHex(registration), DriverNode: "native-current", OracleNode: "oracle-current", InvocationID: "native-current", CohortID: "current", Members: []semanticBoundMember{}, References: []semanticReferenceBinding{}}
	pins := map[string]eligibilityMember{}
	for _, planned := range reg.PlannedMembers {
		owner := binding.DriverNode
		if planned.Owner == "O" {
			owner = binding.OracleNode
		}
		pin := f.member(owner, "oracle/"+planned.Path, planned.Role, files[planned.Path])
		pins[planned.Role] = pin
		binding.Members = append(binding.Members, semanticBoundMember{Owner: planned.Owner, Local: planned.Path, NodeID: owner, Member: pin.Member})
	}
	oracle := f.nodes[binding.OracleNode]
	oracle.Assessment = AssessBlocked
	f.nodes[oracle.ID] = oracle
	f.reg.Native.Source, f.reg.Native.BuildManifest = pins["source"], pins["build-manifest"]
	for i := range f.reg.Invocations {
		inv := &f.reg.Invocations[i]
		if inv.ID == "native-current" {
			wrapper := f.member(inv.Node.ID, "native/semantic-request.json", "approved-request", eligibilityJSON(t, f.witness.Native))
			inv.Request, inv.ApprovedRequest = &wrapper, eligibilityJSON(t, f.witness.Native)
			processPin, responsePin, executablePin, sourcePin := pins["process"], pins["raw"], pins["driver"], pins["source"]
			inv.Process, inv.Response, inv.Executable = &processPin, &responsePin, &executablePin
			inv.Inputs["source"] = eligibilityInput{Kind: "PINNED", MemberPin: &sourcePin, ContentBasis: "PRESERVED_ORIGINAL"}
			binding.Process = processPin
		} else if inv.CohortID == "current" {
			binding.References = append(binding.References, semanticReferenceBinding{InvocationID: inv.ID, CohortID: inv.CohortID, Receipt: *inv.Receipt})
		}
	}
	f.root = filepath.Join(t.TempDir(), "evidence")
	if err := os.Mkdir(f.root, 0700); err != nil {
		t.Fatal(err)
	}
	eligibility := f.request()
	binding.EligibilityApprovalSHA256, binding.EligibilityRegistrationSHA256, binding.WitnessRegistrationSHA256 = digestHex(eligibility.Approval), digestHex(eligibility.Registration), digestHex(eligibility.WitnessRegistration)
	binding.EvidencePolicySHA256, binding.EvidenceSHA256 = digestHex(eligibility.Evidence.Policy), f.reg.Graph.SHA256
	return QuerySemanticsRequest{Evidence: eligibility.Evidence, Profile: profileData, Registration: registration, RunBinding: eligibilityJSON(t, binding), EligibilityApproval: eligibility.Approval, EligibilityRegistration: eligibility.Registration, WitnessRegistration: eligibility.WitnessRegistration}, f, binding
}

func semanticBoundReplay(t *testing.T, req QuerySemanticsRequest, f *eligibilityFixture, binding semanticRunBinding) (string, []byte) {
	t.Helper()
	root := filepath.Dir(f.root)
	members := []ReplayMember{}
	for _, doc := range []struct {
		path, role string
		data       []byte
	}{{"profile.json", "workload-profile", req.Profile}, {"registration.json", "semantics-registration", req.Registration}, {"binding.json", "semantics-run-binding", req.RunBinding}, {"approval.json", "eligibility-approval", req.EligibilityApproval}, {"eligibility.json", "eligibility-registration", req.EligibilityRegistration}, {"witness.json", "query-registration", req.WitnessRegistration}, {"policy.json", "evidence-policy", req.Evidence.Policy}} {
		if err := os.WriteFile(filepath.Join(root, doc.path), doc.data, 0600); err != nil {
			t.Fatal(err)
		}
		members = append(members, ReplayMember{Path: doc.path, Role: doc.role, Bytes: uint64(len(doc.data)), SHA256: digestHex(doc.data)})
	}
	for _, path := range sortedKeys(f.files) {
		members = append(members, ReplayMember{Path: "evidence/" + path, Role: "semantics-evidence", Bytes: uint64(len(f.files[path])), SHA256: digestHex(f.files[path])})
	}
	graph, err := os.ReadFile(filepath.Join(f.root, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	members = append(members, ReplayMember{Path: "evidence/evidence.json", Role: "semantics-evidence", Bytes: uint64(len(graph)), SHA256: digestHex(graph)})
	o := f.nodes[binding.OracleNode]
	var mode struct {
		Context string `json:"context"`
	}
	if err := json.Unmarshal(req.Registration, &mode); err != nil {
		t.Fatal(err)
	}
	reducer := "oracle-set-r4"
	if mode.Context == semanticPagedContext {
		reducer = "oracle-set-r5"
	}
	profile := eligibilityJSON(t, map[string]any{"schema": ReplaySchema, "id": "semantic-bound-replay", "reducer": reducer, "operation": "evidence-replay", "subject": ReplaySubject{Run: o.Identities["run"], Attempt: 1, Platform: "linux/amd64", EvidenceMode: ModeNewRun, ExecutionStatus: StatusCompleted, Assessment: AssessBlocked}, "identities": map[string]string{"workload": digestHex(req.Profile), "semantics-registration": digestHex(req.Registration), "semantics-run-binding": digestHex(req.RunBinding), "evidence-policy": digestHex(req.Evidence.Policy)}, "records": []string{f.witness.Case}, "members": members})
	return root, profile
}

func TestSemanticBoundPublicAPIAndReplay(t *testing.T) {
	req, f, binding := semanticBoundFixture(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	absent := req
	absent.EligibilityApproval, absent.EligibilityRegistration, absent.WitnessRegistration = nil, nil, nil
	absentBinding := binding
	absentBinding.EligibilityApprovalSHA256, absentBinding.EligibilityRegistrationSHA256, absentBinding.WitnessRegistrationSHA256 = digestHex(nil), digestHex(nil), digestHex(nil)
	absentBinding.InvocationID, absentBinding.CohortID, absentBinding.Process, absentBinding.References = "", "", eligibilityMember{}, []semanticReferenceBinding{}
	absent.RunBinding = eligibilityJSON(t, absentBinding)
	blocked, err := VerifyQuerySemantics(ctx, absent)
	if err != nil || !blocked.Complete || blocked.Assessment != AssessBlocked || blocked.BaseVerified.Assessment != AssessPass || blocked.Tuple != (QueryEligibilityCounts{Required: 3, Blocked: 3}) {
		t.Fatalf("same stored data without approval: %v %+v", err, blocked)
	}
	passed, err := VerifyQuerySemantics(ctx, req)
	if err != nil || !passed.Complete || passed.Assessment != AssessPass || passed.Semantic != AssessPass || passed.BaseVerified.Assessment != AssessPass || !passed.BindingValid || passed.Tuple != (QueryEligibilityCounts{Required: 3, Verified: 3}) || passed.ProducerRecorded != (Verdict{StatusCompleted, AssessBlocked}) || passed.Eligibility == nil || passed.Eligibility.Assessment != AssessPass {
		t.Fatalf("caller-approved stored consistency: %v %+v eligibility=%+v", err, passed, passed.Eligibility)
	}
	root, profile := semanticBoundReplay(t, req, f, binding)
	replayed, err := Replay(ctx, ReplayRequest{Root: root, Profile: profile})
	if err != nil || replayed.Assessment != AssessPass || !replayed.EvidenceValid || replayed.QuerySemantics == nil || replayed.QuerySemantics.Tuple != passed.Tuple || replayed.QuerySemantics.ProducerRecorded != passed.ProducerRecorded || !validSemanticReplayResult(replayed) {
		t.Fatalf("same-core BOUND replay: %v %+v", err, replayed)
	}
	if replayed.BytesRead != passed.BytesRead || replayed.QuerySemantics.BytesRead != passed.BytesRead {
		t.Fatalf("shared ledger changed byte accounting: API=%d replay=%d core=%d", passed.BytesRead, replayed.BytesRead, replayed.QuerySemantics.BytesRead)
	}
	for _, mutation := range []string{"normal", "evidence-blocked", "evidence-mode", "eligibility-mode", "nested-evidence-fail", "nested-evidence-different", "overall-fail-nested-pass", "subject-assessment", "subject-mode", "graph-subject-assessment", "graph-subject-mode", "producer-pass", "inner-fail", "inner-negative", "inner-mode", "inner-duplicate", "inner-missing", "inner-recorded-binding", "inner-zero-status", "inner-missing-tree", "inner-missing-api", "pass-code", "reported-error", "outer-reported-error", "evidence-reported-error", "eligibility-reported-error"} {
		t.Run("stored-"+mutation, func(t *testing.T) {
			var stored ReplayResult
			if err := json.Unmarshal(mustJSON(t, replayed), &stored); err != nil {
				t.Fatal(err)
			}
			subject := &EvidenceNode{ExecutionStatus: replayed.Subject.ExecutionStatus, EvidenceMode: replayed.Subject.EvidenceMode, Assessment: replayed.Subject.Assessment, Identities: map[string]string{"run": replayed.Subject.Run, "attempt": "1", "platform": replayed.Subject.Platform}}
			switch mutation {
			case "evidence-blocked":
				stored.QuerySemantics.Evidence.Assessment = AssessBlocked
			case "evidence-mode":
				stored.QuerySemantics.Evidence.EvidenceMode = "FORGED"
			case "eligibility-mode":
				stored.QuerySemantics.Eligibility.EvidenceMode = "FORGED"
			case "nested-evidence-fail":
				stored.QuerySemantics.Eligibility.Evidence.Assessment = AssessFail
			case "nested-evidence-different":
				stored.QuerySemantics.Eligibility.Evidence.PolicyID = "different-snapshot"
			case "overall-fail-nested-pass":
				stored.QuerySemantics.Evidence.Assessment = AssessFail
				stored.QuerySemantics.Eligibility.Evidence.Assessment = AssessFail
				stored.QuerySemantics.Assessment, stored.Assessment = AssessFail, AssessFail
				stored.Recomputed.Assessment = AssessFail
				stored.EvidenceValid = false
				stored.Gates[0].Failed = 1
				stored.Gates[0].finish()
			case "inner-fail":
				if len(stored.QuerySemantics.Gates) == 0 {
					t.Fatal("positive fixture has no base gates")
				}
				g := &stored.QuerySemantics.Gates[0]
				if g.Recomputed == 0 {
					g.Recomputed = 1
				}
				g.Failed = 1
				g.Code = "FORGED_BASE_FAILURE"
				g.finish()
			case "inner-negative":
				stored.QuerySemantics.Gates[0].Recomputed = -1
			case "inner-mode":
				stored.QuerySemantics.Gates[0].EvidenceMode = ModeCarriedForward
			case "inner-duplicate":
				stored.QuerySemantics.Gates = append(stored.QuerySemantics.Gates, stored.QuerySemantics.Gates[0])
			case "inner-recorded-binding", "inner-zero-status":
				id := "case-binding"
				if mutation == "inner-zero-status" {
					id = "case-status"
				}
				for i := range stored.QuerySemantics.Gates {
					g := &stored.QuerySemantics.Gates[i]
					if g.ID == id {
						g.Recomputed = 0
						g.NotRecomputed = 0
						if mutation == "inner-recorded-binding" {
							g.NotRecomputed = 1
						}
						g.finish()
					}
				}
			case "inner-missing-tree", "inner-missing-api":
				id := "tree"
				if mutation == "inner-missing-api" {
					id = "api"
				}
				stored.QuerySemantics.Gates = slices.DeleteFunc(stored.QuerySemantics.Gates, func(g ReplayGate) bool { return g.ID == id })
			case "inner-missing":
				stored.QuerySemantics.Gates = nil
			case "reported-error":
				stored.QuerySemantics.Findings = []Finding{{Severity: "error", Code: "SEMANTICS_PROJECTION_BASE_INPUT"}}
			case "outer-reported-error":
				stored.Findings = []Finding{{Severity: "error", Code: "RECORDED_VERDICT_MISMATCH"}}
			case "evidence-reported-error":
				stored.QuerySemantics.Evidence.Findings = []Finding{{Severity: "error", Code: "EVIDENCE_FAILED"}}
				stored.QuerySemantics.Eligibility.Evidence.Findings = slices.Clone(stored.QuerySemantics.Evidence.Findings)
			case "eligibility-reported-error":
				stored.QuerySemantics.Eligibility.Findings = []Finding{{Severity: "error", Code: "ELIGIBILITY_WITNESS_CONTEXT"}}
			case "pass-code":
				stored.QuerySemantics.Code = "FORGED_FAILURE"
			case "producer-pass":
				stored.QuerySemantics.ProducerRecorded.Assessment = AssessPass
				stored.Recorded.Assessment, stored.Subject.Assessment, subject.Assessment = AssessPass, AssessPass, AssessPass
			case "subject-assessment":
				stored.Subject.Assessment = AssessPass
			case "subject-mode":
				stored.Subject.EvidenceMode = ModeCarriedForward
			case "graph-subject-assessment":
				subject.Assessment = AssessPass
			case "graph-subject-mode":
				subject.EvidenceMode = ModeCarriedForward
			}
			data := mustJSON(t, stored)
			node := &EvidenceNode{EvidenceMode: stored.EvidenceMode, Assessment: stored.Assessment, Files: []ReplayMember{{Path: "stored.json", Role: "replay-result", Bytes: uint64(len(data)), SHA256: digestHex(data)}}}
			x := &replayEnv{r: semanticTestBudget(t).r}
			if e := x.checkReplayFile(node, subject, map[string][]byte{"stored.json": data}, x.finding); e != nil || (len(x.findings) == 0) != (mutation == "normal") {
				t.Fatalf("stored BOUND mutation %s: %v %+v", mutation, e, x.findings)
			}
		})
	}
}

func TestSemanticBoundRehashedProjectionCannotReplaceRaw(t *testing.T) {
	for _, mutation := range []string{"route", "declarations", "producer", "summary-tree", "record-claim", "projection-verdict", "projection-code"} {
		t.Run(mutation, func(t *testing.T) {
			req, f, binding := semanticBoundFixture(t, 0)
			projectionPath := "oracle/projection/00000-" + f.witness.Case + ".json"
			recordPath := "oracle/records/00000-" + f.witness.Case + ".json"
			var projection, record map[string]any
			for path, target := range map[string]any{projectionPath: &projection, recordPath: &record} {
				if err := json.Unmarshal(f.files[path], target); err != nil {
					t.Fatal(err)
				}
			}
			step := projection["steps"].([]any)[1].(map[string]any)
			switch mutation {
			case "route":
				step["route"].(map[string]any)["reused_nodes"] = 999
			case "declarations":
				step["incremental"].(map[string]any)["declarations"] = []any{map[string]any{"fact": "type", "node_type": "identifier", "start_byte": 0, "end_byte": 1, "name": nil, "status": "PASS"}}
			case "producer":
				projection["producer"].(map[string]any)["language_version"] = 14
			case "summary-tree":
				initial := projection["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
				initial["form"], initial["tree"] = "record", nil
			case "record-claim":
				record["claims"].(map[string]any)["expectations"] = claimFail
			case "projection-verdict":
				projection["execution_status"] = "FORGED"
			case "projection-code":
				projection["code"] = "FORGED"
			}
			projectionData := mustJSON(t, projection)
			record["steps"], record["producer"] = projection["steps"], projection["producer"]
			pin := record["query_semantics"].(map[string]any)["projection"].(map[string]any)
			pin["bytes"], pin["sha256"] = len(projectionData), digestHex(projectionData)
			delete(record, "complete")
			recordData := mustJSON(t, record)
			recordData = append(recordData[:len(recordData)-1], []byte(`,"complete":true}`)...)
			f.files[projectionPath], f.files[recordPath] = projectionData, recordData
			semanticBoundReapprove(t, &req, f, &binding)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := VerifyQuerySemantics(ctx, req)
			badFold := strings.HasPrefix(mutation, "record-") || strings.HasPrefix(mutation, "projection-")
			expectedCode := "SEMANTICS_PROJECTION_BASE_INPUT"
			if mutation == "summary-tree" {
				expectedCode = "SEMANTICS_PROJECTION_TREE"
			}
			if err != nil || !result.Complete || !result.BindingValid || result.ProducerFoldValid == badFold || result.Eligibility == nil || result.Eligibility.Assessment != AssessPass || result.Assessment != AssessFail || !badFold && result.Code != expectedCode {
				t.Fatalf("reapproved %s projection replaced native facts: %v %+v eligibility=%+v", mutation, err, result, result.Eligibility)
			}
			if mutation == "summary-tree" && result.BaseVerified.Assessment != AssessBlocked {
				t.Fatalf("unrecomputed base gate became PASS: %+v", result)
			}
			if mutation == "producer" || mutation == "summary-tree" {
				root, profile := semanticBoundReplay(t, req, f, binding)
				replayed, err := Replay(ctx, ReplayRequest{Root: root, Profile: profile})
				if err != nil || replayed.Assessment != AssessFail || replayed.QuerySemantics == nil || replayed.QuerySemantics.Eligibility.Assessment != AssessPass || replayed.QuerySemantics.Eligibility.Evidence.Assessment != AssessPass || !validSemanticReplayResult(replayed) {
					t.Fatalf("actual semantic failure lost valid eligibility/evidence PASS: %v %+v", err, replayed)
				}
				if mutation == "summary-tree" && (replayed.QuerySemantics.BaseVerified.Assessment != AssessBlocked || replayed.QuerySemantics.Code != expectedCode) {
					t.Fatalf("summary mismatch lost BLOCKED base or first failure: %+v", replayed.QuerySemantics)
				}
				data := mustJSON(t, replayed)
				node := &EvidenceNode{EvidenceMode: replayed.EvidenceMode, Assessment: replayed.Assessment, Files: []ReplayMember{{Path: "failed.json", Role: "replay-result", Bytes: uint64(len(data)), SHA256: digestHex(data)}}}
				subject := f.nodes[binding.OracleNode]
				x := &replayEnv{r: semanticTestBudget(t).r}
				if e := x.checkReplayFile(node, &subject, map[string][]byte{"failed.json": data}, x.finding); e != nil || len(x.findings) != 0 {
					t.Fatalf("actual FAIL output rejected by stored reader: %v %+v", e, x.findings)
				}
				originalCode := replayed.QuerySemantics.Code
				replayed.QuerySemantics.Code = "FORGED_FIRST_CAUSE"
				if originalCode == "" || validSemanticReplayResult(replayed) {
					t.Fatal("stored semantic failure replaced its first cause independently of the gate")
				}
				replayed.QuerySemantics.Code, replayed.Gates[0].Code = "", ""
				if validSemanticReplayResult(replayed) {
					t.Fatal("stored semantic failure erased its first cause from both payload and gate")
				}
			}
		})
	}
}

func TestSemanticProducerFoldRejectsMatchingInvalidVerdict(t *testing.T) {
	for _, status := range []string{StatusFailed, "FORGED"} {
		t.Run(status, func(t *testing.T) {
			_, f, _ := semanticBoundFixture(t, 0)
			name := "00000-" + f.witness.Case + ".json"
			projectionPath, recordPath := "projection/"+name, "records/"+name
			var projection, record map[string]any
			var manifest OracleManifest
			for path, target := range map[string]any{projectionPath: &projection, recordPath: &record, "manifest.json": &manifest} {
				if err := json.Unmarshal(f.files["oracle/"+path], target); err != nil {
					t.Fatal(err)
				}
			}
			projection["execution_status"], projection["assessment"], projection["code"] = status, AssessPass, ""
			record["execution_status"], record["assessment"], record["code"] = status, AssessPass, ""
			manifest.ExecutionStatus, manifest.Assessment = status, AssessPass
			projectionData := mustJSON(t, projection)
			pin := record["query_semantics"].(map[string]any)["projection"].(map[string]any)
			pin["bytes"], pin["sha256"] = len(projectionData), digestHex(projectionData)
			delete(record, "complete")
			recordData := mustJSON(t, record)
			recordData = append(recordData[:len(recordData)-1], []byte(`,"complete":true}`)...)
			files := map[string][]byte{}
			for path, data := range f.files {
				if name, ok := strings.CutPrefix(path, "oracle/"); ok {
					files[name] = data
				}
			}
			files[projectionPath], files[recordPath] = projectionData, recordData
			for i := range manifest.Members {
				m := &manifest.Members[i]
				m.Bytes, m.SHA256 = uint64(len(files[m.Path])), digestHex(files[m.Path])
				if m.Case != "" {
					m.ExecutionStatus = status
				}
			}
			b := semanticTestBudget(t)
			ast, e := b.semanticAST(recordPath, recordData)
			if e != nil {
				t.Fatal(e)
			}
			state, invalid := semanticSetState{}, false
			e = b.checkSemanticSet(manifest, files, ast, &state, func(code, _, _ string) { invalid = invalid || code == "SEMANTICS_PROJECTION_VERDICT" })
			if e != nil || state.producerVerified || !invalid {
				t.Fatalf("matching invalid status/assessment was accepted as a fold: %v %+v", e, state)
			}
		})
	}
}

func TestSemanticBoundPartialReplayPreservesRecordedAndMismatch(t *testing.T) {
	for _, kind := range []string{KindCancelled, KindResourceLimit} {
		t.Run(kind, func(t *testing.T) {
			req, fixture, binding := semanticBoundFixture(t, 0)
			root, profile := semanticBoundReplay(t, req, fixture, binding)
			var registered map[string]any
			if err := json.Unmarshal(profile, &registered); err != nil {
				t.Fatal(err)
			}
			registered["subject"].(map[string]any)["assessment"] = AssessPass
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			hits := 0
			testHookSemanticProducerFold = func(b *eligibilityBudget) {
				hits++
				if kind == KindCancelled {
					cancel()
				} else {
					b.work = eligibilityWorkLimit
				}
			}
			t.Cleanup(func() { testHookSemanticProducerFold = nil })
			result, err := Replay(ctx, ReplayRequest{Root: root, Profile: mustJSON(t, registered)})
			if hits != 1 || err == nil || asError(err).Kind != kind || kind == KindCancelled && !errors.Is(err, context.Canceled) || result.Recorded != (Verdict{StatusCompleted, AssessBlocked}) || result.QuerySemantics == nil || !result.QuerySemantics.ProducerFoldValid || result.Recorded != result.QuerySemantics.ProducerRecorded || result.Assessment != AssessFail {
				t.Fatalf("partial producer/mismatch/error lost: hits=%d %v %+v", hits, err, result)
			}
			found := false
			for _, finding := range result.Findings {
				found = found || finding.Code == "RECORDED_VERDICT_MISMATCH"
			}
			if !found || !validSemanticReplayResult(result) {
				t.Fatalf("partial mismatch finding or stored result rejected: %+v", result)
			}
		})
	}
}

func TestSemanticBoundRejectsUnregisteredGrammar(t *testing.T) {
	req, _, _ := semanticBoundFixture(t, 0, "grammar-header")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := VerifyQuerySemantics(ctx, req)
	if err != nil || !result.Complete || result.BindingValid || result.Assessment != AssessFail || result.Code != "SEMANTICS_BUILD_PROFILE" || result.Eligibility == nil || result.Eligibility.Assessment != AssessPass {
		t.Fatalf("approved extra grammar input escaped the profile: %v %+v", err, result)
	}
}

func semanticBoundReapprove(t *testing.T, req *QuerySemanticsRequest, f *eligibilityFixture, binding *semanticRunBinding) {
	t.Helper()
	var manifest OracleManifest
	if err := json.Unmarshal(f.files["oracle/manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	for i := range manifest.Members {
		p := &manifest.Members[i]
		data := f.files["oracle/"+p.Path]
		p.Bytes, p.SHA256 = uint64(len(data)), digestHex(data)
		if p.Case == "" {
			p.InputBytes, p.InputSHA256 = p.Bytes, p.SHA256
		}
	}
	f.files["oracle/manifest.json"] = mustJSON(t, manifest)
	for i := range f.reg.OwnedMembers {
		p := &f.reg.OwnedMembers[i].Member
		p.Bytes, p.SHA256 = uint64(len(f.files[p.Path])), digestHex(f.files[p.Path])
	}
	for id, node := range f.nodes {
		for i := range node.Files {
			p := &node.Files[i]
			p.Bytes, p.SHA256 = uint64(len(f.files[p.Path])), digestHex(f.files[p.Path])
		}
		f.nodes[id] = node
	}
	for i := range binding.Members {
		p := &binding.Members[i].Member
		p.Bytes, p.SHA256 = uint64(len(f.files[p.Path])), digestHex(f.files[p.Path])
	}
	eligibility := f.request()
	req.Evidence, req.EligibilityApproval, req.EligibilityRegistration, req.WitnessRegistration = eligibility.Evidence, eligibility.Approval, eligibility.Registration, eligibility.WitnessRegistration
	binding.ManifestSHA256 = digestHex(f.files["oracle/manifest.json"])
	binding.EligibilityApprovalSHA256, binding.EligibilityRegistrationSHA256, binding.WitnessRegistrationSHA256 = digestHex(eligibility.Approval), digestHex(eligibility.Registration), digestHex(eligibility.WitnessRegistration)
	binding.EvidencePolicySHA256, binding.EvidenceSHA256 = digestHex(eligibility.Evidence.Policy), f.reg.Graph.SHA256
	req.RunBinding = mustJSON(t, binding)
}

func TestSemanticBoundRejectsBorrowedGrammarPin(t *testing.T) {
	for _, borrowControl := range []bool{true, false} {
		t.Run(strconv.FormatBool(borrowControl), func(t *testing.T) {
			req, f, binding := semanticBoundFixture(t, 0)
			name := sortedKeys(f.reg.Native.Grammar)[0]
			original := f.reg.Native.Grammar[name]
			if borrowControl {
				f.reg.Native.Grammar[name] = f.reg.Control.Grammar[name]
			} else {
				f.reg.Native.Grammar[name] = f.member(binding.DriverNode, "native/grammar-alias.bin", original.Member.Role, f.files[original.Member.Path])
			}
			if f.reg.Native.Grammar[name].Member.SHA256 != original.Member.SHA256 {
				t.Fatal("mutation changed grammar bytes")
			}
			semanticBoundReapprove(t, &req, f, &binding)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := VerifyQuerySemantics(ctx, req)
			if err != nil || !result.Complete || result.BindingValid || result.Assessment != AssessFail || result.Code != "SEMANTICS_NATIVE_PIN" || result.Eligibility == nil || result.Eligibility.Assessment != AssessPass {
				t.Fatalf("same-hash borrowed grammar pin escaped binding: %v %+v", err, result)
			}
		})
	}
}

func TestSemanticBoundRejectsDeclaredCompilerContradiction(t *testing.T) {
	for _, field := range []string{"name", "version"} {
		t.Run(field, func(t *testing.T) {
			req, f, binding := semanticBoundFixture(t, 0)
			var build semanticBuildDescription
			if err := json.Unmarshal(f.files["oracle/capture/build.json"], &build); err != nil {
				t.Fatal(err)
			}
			observed := build.CompilerVersion
			if field == "name" {
				build.Compiler.Name = "forged-compiler"
			} else {
				build.Compiler.Version = "forged-declared-version"
			}
			f.files["oracle/capture/build.json"] = mustJSON(t, build)
			semanticBoundReapprove(t, &req, f, &binding)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := VerifyQuerySemantics(ctx, req)
			if err != nil || !result.Complete || result.BindingValid || result.Assessment != AssessFail || result.Code != "SEMANTICS_BUILD_PROFILE" || result.Eligibility == nil || result.Eligibility.Assessment != AssessPass || observed != build.CompilerVersion {
				t.Fatalf("declared compiler mismatch escaped profile binding: %v %+v", err, result)
			}
		})
	}
}

func TestSemanticBoundSparseRegistrationAndStoredResult(t *testing.T) {
	req, f, binding := semanticBoundFixture(t, 5<<20)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := VerifyQuerySemantics(ctx, req)
	if err != nil || !result.Complete || result.Assessment != AssessPass {
		t.Fatalf("5MiB sparse registration rejected by API: %v %+v", err, result)
	}
	root, profile := semanticBoundReplay(t, req, f, binding)
	replayed, err := Replay(ctx, ReplayRequest{Root: root, Profile: profile})
	if err != nil || replayed.Assessment != AssessPass || !validSemanticReplayResult(replayed) {
		t.Fatalf("sparse registration replay: %v %+v", err, replayed)
	}
	data := mustJSON(t, replayed)
	if len(data) >= 5<<20 {
		t.Fatal("stored result fixture too large")
	}
	data = append(data, bytes.Repeat([]byte(" "), (5<<20)-len(data))...)
	node := &EvidenceNode{EvidenceMode: replayed.EvidenceMode, Assessment: replayed.Assessment, Files: []ReplayMember{{Path: "stored.json", Role: "replay-result", Bytes: uint64(len(data)), SHA256: digestHex(data)}}}
	subject := f.nodes[binding.OracleNode]
	x := &replayEnv{r: semanticTestBudget(t).r}
	if e := x.checkReplayFile(node, &subject, map[string][]byte{"stored.json": data}, x.finding); e != nil || len(x.findings) != 0 {
		t.Fatalf("5MiB sparse stored r3 rejected: %v %+v", e, x.findings)
	}
}

func TestSemanticBoundSparseProjection(t *testing.T) {
	req, f, binding := semanticBoundFixture(t, 0)
	projectionPath := "oracle/projection/00000-" + f.witness.Case + ".json"
	recordPath := "oracle/records/00000-" + f.witness.Case + ".json"
	data := f.files[projectionPath]
	if len(data) >= 8<<20 {
		t.Fatal("projection fixture too large")
	}
	data = append(data, bytes.Repeat([]byte(" "), (8<<20)-len(data))...)
	f.files[projectionPath] = data
	var record map[string]any
	if err := json.Unmarshal(f.files[recordPath], &record); err != nil {
		t.Fatal(err)
	}
	pin := record["query_semantics"].(map[string]any)["projection"].(map[string]any)
	pin["bytes"], pin["sha256"] = len(data), digestHex(data)
	delete(record, "complete")
	encoded := mustJSON(t, record)
	f.files[recordPath] = append(encoded[:len(encoded)-1], []byte(`,"complete":true}`)...)
	semanticBoundReapprove(t, &req, f, &binding)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := VerifyQuerySemantics(ctx, req)
	if err != nil || !result.Complete || result.Assessment != AssessPass {
		t.Fatalf("8MiB sparse projection rejected by publicAPI: %v %+v", err, result)
	}
	files := fstest.MapFS{}
	for path, data := range f.files {
		if strings.HasPrefix(path, "oracle/") {
			files[strings.TrimPrefix(path, "oracle/")] = &fstest.MapFile{Data: data}
		}
	}
	set := VerifyOracleSet(files)
	if !set.Valid {
		t.Fatalf("8MiB sparse projection rejected by set verifier: %+v", set)
	}
}

func TestSemanticEmptyQueriesClaimsAPIReplayStored(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(strconv.FormatBool(missing), func(t *testing.T) {
			req, f, binding := semanticBoundFixtureQueries(t, 0, false)
			if missing {
				for _, role := range []string{"projection", "records"} {
					path := "oracle/" + role + "/00000-" + f.witness.Case + ".json"
					var record map[string]any
					if err := json.Unmarshal(f.files[path], &record); err != nil {
						t.Fatal(err)
					}
					delete(record, "oracle_claims")
					if role == "projection" {
						f.files[path] = mustJSON(t, record)
					} else {
						delete(record, "complete")
						data := mustJSON(t, record)
						f.files[path] = append(data[:len(data)-1], []byte(`,"complete":true}`)...)
					}
				}
				recordPath := "oracle/records/00000-" + f.witness.Case + ".json"
				var record map[string]any
				if err := json.Unmarshal(f.files[recordPath], &record); err != nil {
					t.Fatal(err)
				}
				data := f.files["oracle/projection/00000-"+f.witness.Case+".json"]
				pin := record["query_semantics"].(map[string]any)["projection"].(map[string]any)
				pin["bytes"], pin["sha256"] = len(data), digestHex(data)
				delete(record, "complete")
				encoded := mustJSON(t, record)
				f.files[recordPath] = append(encoded[:len(encoded)-1], []byte(`,"complete":true}`)...)
				semanticBoundReapprove(t, &req, f, &binding)
			}
			req.EligibilityApproval, req.EligibilityRegistration, req.WitnessRegistration = nil, nil, nil
			binding.EligibilityApprovalSHA256, binding.EligibilityRegistrationSHA256, binding.WitnessRegistrationSHA256 = digestHex(nil), digestHex(nil), digestHex(nil)
			binding.InvocationID, binding.CohortID, binding.Process, binding.References = "", "", eligibilityMember{}, []semanticReferenceBinding{}
			req.RunBinding = mustJSON(t, binding)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := VerifyQuerySemantics(ctx, req)
			want, base := AssessBlocked, AssessPass
			if missing {
				want, base = AssessFail, AssessFail
			}
			if err != nil || !result.Complete || result.Assessment != want || result.BaseVerified.Assessment != base || result.Tuple.Required != 0 || missing && result.Code != "ORACLE_CLAIMS_MISSING" {
				t.Fatalf("empty query API missing=%v: %v %+v", missing, err, result)
			}
			root, profile := semanticBoundReplay(t, req, f, binding)
			replayed, err := Replay(ctx, ReplayRequest{Root: root, Profile: profile})
			if err != nil || replayed.Assessment != want || replayed.QuerySemantics == nil || replayed.QuerySemantics.BaseVerified.Assessment != base || !validSemanticReplayResult(replayed) {
				t.Fatalf("empty query replay/stored missing=%v: %v %+v", missing, err, replayed)
			}
		})
	}
}

func TestSemanticProducerFailureCodeSurvivesPartialReplay(t *testing.T) {
	for _, kind := range []string{KindCancelled, KindResourceLimit} {
		t.Run(kind, func(t *testing.T) {
			req, f, binding := semanticBoundFixture(t, 0)
			projectionPath, recordPath := "oracle/projection/00000-"+f.witness.Case+".json", "oracle/records/00000-"+f.witness.Case+".json"
			var projection, record map[string]any
			if err := json.Unmarshal(f.files[projectionPath], &projection); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(f.files[recordPath], &record); err != nil {
				t.Fatal(err)
			}
			projection["assessment"], projection["code"] = AssessFail, "EXPECTATION_FAILED"
			projection["claims"].(map[string]any)["expectations"] = claimFail
			projection["expectations"].([]any)[0].(map[string]any)["result"] = claimFail
			data := mustJSON(t, projection)
			f.files[projectionPath] = data
			for _, key := range []string{"assessment", "code", "claims", "expectations"} {
				record[key] = projection[key]
			}
			pin := record["query_semantics"].(map[string]any)["projection"].(map[string]any)
			pin["bytes"], pin["sha256"] = len(data), digestHex(data)
			delete(record, "complete")
			data = mustJSON(t, record)
			f.files[recordPath] = append(data[:len(data)-1], []byte(`,"complete":true}`)...)
			var manifest OracleManifest
			if err := json.Unmarshal(f.files["oracle/manifest.json"], &manifest); err != nil {
				t.Fatal(err)
			}
			manifest.Assessment = AssessFail
			f.files["oracle/manifest.json"] = mustJSON(t, manifest)
			semanticBoundReapprove(t, &req, f, &binding)
			root, profile := semanticBoundReplay(t, req, f, binding)
			var registered map[string]any
			if err := json.Unmarshal(profile, &registered); err != nil {
				t.Fatal(err)
			}
			registered["subject"].(map[string]any)["assessment"] = AssessFail
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			hits := 0
			testHookSemanticProducerFold = func(b *eligibilityBudget) {
				hits++
				if kind == KindCancelled {
					cancel()
				} else {
					b.work = eligibilityWorkLimit
				}
			}
			defer func() { testHookSemanticProducerFold = nil }()
			result, err := Replay(ctx, ReplayRequest{Root: root, Profile: mustJSON(t, registered)})
			if hits != 1 || err == nil || asError(err).Kind != kind || result.QuerySemantics == nil || result.QuerySemantics.Complete || result.QuerySemantics.Assessment != AssessFail || result.QuerySemantics.Code != "EXPECTATION_FAILED" || result.Recorded.Assessment != AssessFail || !validSemanticReplayResult(result) {
				t.Fatalf("producer FAIL partial lost: hits=%d %v %+v", hits, err, result)
			}
			originalFindings := result.QuerySemantics.Findings
			result.QuerySemantics.Findings = nil
			result.QuerySemantics.Code = "FORGED_FIRST_CAUSE"
			if validSemanticReplayResult(result) {
				t.Fatal("missing semantic failure findings hid original cause")
			}
			result.QuerySemantics.Findings, result.QuerySemantics.Code = originalFindings, "EXPECTATION_FAILED"
			result.QuerySemantics.Findings = append([]Finding{{Severity: "error", Code: ""}}, result.QuerySemantics.Findings...)
			if validSemanticReplayResult(result) {
				t.Fatal("empty error finding masked preserved first cause")
			}
			result.QuerySemantics.Findings = result.QuerySemantics.Findings[1:]
			result.QuerySemantics.Code = "FORGED_FIRST_CAUSE"
			if validSemanticReplayResult(result) {
				t.Fatal("partial first cause changed independently of preserved finding")
			}
			result.QuerySemantics.Eligibility.Assessment = AssessFail
			for _, warnings := range []int{0, 999} {
				result.QuerySemantics.Eligibility.Findings = make([]Finding, warnings)
				for i := range result.QuerySemantics.Eligibility.Findings {
					result.QuerySemantics.Eligibility.Findings[i] = Finding{Severity: "warning", Code: "PRIOR_WARNING"}
				}
				if validSemanticReplayResult(result) {
					t.Fatal("unsaturated eligibility FAIL hid visible original cause")
				}
			}
		})
	}
}
