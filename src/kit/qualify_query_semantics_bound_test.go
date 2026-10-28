package kit

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	qualificationSemanticRoute = "owned-plain"
	qualificationSemanticSet   = "s06-owned-plain"
	qualificationSemanticHead  = "89abcdef0123456789abcdef0123456789abcdef"
)

type qualificationSemanticCoreFixture struct {
	id       string
	platform string
	req      QuerySemanticsRequest
	f        *eligibilityFixture
	binding  semanticRunBinding
}

type qualificationSemanticFixture struct {
	request QualifyRequest
	cores   map[string]*qualificationSemanticCoreFixture
}

func newQualificationSemanticFixture(t *testing.T) QualifyRequest {
	t.Helper()
	return newQualificationSemanticFixtureState(t).request
}

func newQualificationSemanticFixtureState(t *testing.T) *qualificationSemanticFixture {
	t.Helper()
	platforms := []QualPlatform{{ID: "windows-amd64", GOOS: "windows", GOARCH: "amd64"}, {ID: "linux-amd64", GOOS: "linux", GOARCH: "amd64"}}
	fx := newQfx(t)
	fx.inv.Platforms = slices.Clone(platforms)
	fx.inv.Cells = len(platforms)
	fx.inv.Routes = []QualRoute{{Route: qualificationSemanticRoute, Workload: qfxWorkload(qualificationSemanticSet, qualificationSemanticRoute, qfxCases(qualificationSemanticRoute)),
		Requirements: []QualRequirement{{Row: qualificationSemanticRoute + "-B01", Kinds: []string{"P", "E", "Q"}, AlternativesStatus: AlternativesComplete, Alternatives: []string{qualificationSemanticRoute + "-B01.a01"}}}}}
	fx.inv.ExtraRoles = []QualRole{}
	var hosts []QualifyHost
	for _, p := range platforms {
		hosts = append(hosts, QualifyHost{Platform: p.ID, Root: fx.host(t, p, qfxMut{})})
	}
	inventory := fx.invBytes(t)
	fixture := &qualificationSemanticFixture{cores: map[string]*qualificationSemanticCoreFixture{}}
	var plan qualSemanticPlan
	plan.Schema, plan.ID, plan.InventorySHA256, plan.Context, plan.Scope = QualificationSemanticsPlanSchema, "synthetic-small-fanin", digestHex(inventory), "full-input-utf8/r2", qualSemanticScope
	for _, p := range platforms {
		id := "semantic-" + p.ID[:strings.IndexByte(p.ID, '-')]
		core := newQualificationSemanticCoreFixture(t, id, p.pair())
		fixture.cores[p.ID] = core
		var reg semanticRegistration
		if err := json.Unmarshal(core.req.Registration, &reg); err != nil {
			t.Fatal(err)
		}
		target := qualSemanticPlanTarget{ID: id, Route: reg.Route, Role: "primary", Set: qualificationSemanticSet, Platform: p.ID, ProbeID: reg.Queries[0].ID,
			ProfileSHA256: digestHex(core.req.Profile), RegistrationSHA256: digestHex(core.req.Registration), Queries: []QualQuery{}, Required: []qualSemanticTuple{}}
		for _, query := range reg.Queries {
			target.Queries = append(target.Queries, QualQuery{ID: query.ID, SHA256: digestHex([]byte(query.Source))})
		}
		for _, tuple := range reg.Required {
			target.Required = append(target.Required, qualSemanticTuple{Case: reg.Case, Query: tuple.Query, Step: tuple.Step, TreeRole: tuple.TreeRole})
		}
		plan.Targets = append(plan.Targets, target)
	}
	planBytes := eligibilityJSON(t, plan)
	planSHA := digestHex(planBytes)
	inputs := make([]QualificationQuerySemanticsInput, 0, len(plan.Targets))
	for _, p := range platforms {
		core := fixture.cores[p.ID]
		inputs = append(inputs, qualificationSemanticInput(t, core, planSHA, core.f.reg.Graph.SHA256))
	}
	fixture.request = QualifyRequest{Inventory: inventory, Candidate: qfxCandidate, Hosts: hosts,
		QuerySemantics: &QualificationQuerySemanticsRequest{Plan: planBytes, Targets: inputs}}
	return fixture
}

func newQualificationSemanticCoreFixture(t *testing.T, id, platform string) *qualificationSemanticCoreFixture {
	t.Helper()
	// Rebind synthetic protocol data consistently; this fixture does not execute a platform driver.
	req, f, binding := semanticBoundFixture(t, 0)
	f.witness.Platform = platform
	for i := range f.reg.Invocations {
		inv := &f.reg.Invocations[i]
		if inv.CohortID != "current" {
			continue
		}
		node := f.nodes[inv.ID]
		node.Identities["source"], node.Identities["platform"], node.Identities["attempt"] = qfxCandidate, platform, "1"
		f.nodes[inv.ID] = node
		inv.Node.Identities = node.Identities
	}
	oracle := f.nodes[binding.OracleNode]
	oracle.Identities["source"], oracle.Identities["platform"], oracle.Identities["run"], oracle.Identities["attempt"] = qfxCandidate, platform, "500", "1"
	f.nodes[binding.OracleNode] = oracle
	if platform == "windows/amd64" {
		qualificationSemanticWindowsPlatform(t, f, &binding)
	}

	var reg semanticRegistration
	if err := json.Unmarshal(req.Registration, &reg); err != nil {
		t.Fatal(err)
	}
	reg.Platform = platform
	req.Registration = eligibilityJSON(t, reg)
	var profile map[string]json.RawMessage
	if err := json.Unmarshal(req.Profile, &profile); err != nil {
		t.Fatal(err)
	}
	profile["query_semantics_registration_sha256"] = mustJSON(t, digestHex(req.Registration))
	req.Profile = eligibilityJSON(t, profile)
	if platform == "windows/amd64" {
		qualificationSemanticRebindRecord(t, f, binding, req.Registration)
	} else {
		qualificationSemanticRegistrationDigest(t, f, binding, req.Registration)
	}
	var manifest OracleManifest
	if err := json.Unmarshal(f.files["oracle/manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Producer["platform"] = platform
	manifest.QuerySemanticsRegistrationSHA256 = digestHex(req.Registration)
	manifest.Workload["profile_sha256"] = digestHex(req.Profile)
	f.replace("oracle/manifest.json", mustJSON(t, manifest))
	binding.ProfileSHA256, binding.RegistrationSHA256 = digestHex(req.Profile), digestHex(req.Registration)
	semanticBoundReapprove(t, &req, f, &binding)
	return &qualificationSemanticCoreFixture{id: id, platform: platform, req: req, f: f, binding: binding}
}

func qualificationSemanticWindowsPlatform(t *testing.T, f *eligibilityFixture, binding *semanticRunBinding) {
	t.Helper()
	f.witness.Native.Process.Path, f.witness.Native.Process.Dir = `C:\current\native.exe`, `C:\current`
	f.witness.Native.Process.CgroupParent = ""
	for i := range f.reg.Invocations {
		inv := &f.reg.Invocations[i]
		if inv.CohortID != "current" {
			continue
		}
		if inv.Kind == "NATIVE_DRIVER" {
			inv.ApprovedRequest = eligibilityJSON(t, f.witness.Native)
			inv.ExecutablePath = f.witness.Native.Process.Path
			if inv.Request != nil {
				f.replace(inv.Request.Member.Path, eligibilityJSON(t, f.witness.Native))
			}
		} else {
			var request eligibilityProcessRequest
			if err := json.Unmarshal(inv.ApprovedRequest, &request); err != nil {
				t.Fatal(err)
			}
			qualificationSemanticWindowsRequest(&request)
			inv.ApprovedRequest, inv.ExecutablePath = eligibilityJSON(t, request), request.Path
			for name, index := range map[string]int{"library": 0, "source": 2, "query-0": 3} {
				input := inv.Inputs[name]
				input.PathLiteral = request.Args[index]
				inv.Inputs[name] = input
			}
			if inv.Request != nil {
				f.replace(inv.Request.Member.Path, eligibilityJSON(t, request))
			}
		}
		if inv.Process != nil {
			f.replace(inv.Process.Member.Path, qualificationSemanticWindowsExecution(t, f.files[inv.Process.Member.Path]))
			if inv.ID == "native-current" {
				binding.Process = *inv.Process
			}
		}
		if inv.Receipt != nil {
			var receipt eligibilityReceipt
			if err := json.Unmarshal(f.files[inv.Receipt.Member.Path], &receipt); err != nil {
				t.Fatal(err)
			}
			receipt.Execution = qualificationSemanticWindowsExecutionValue(receipt.Execution)
			f.replace(inv.Receipt.Member.Path, eligibilityJSON(t, receipt))
			for j := range binding.References {
				if binding.References[j].InvocationID == inv.ID {
					binding.References[j].Receipt = *inv.Receipt
				}
			}
		}
	}
	capturePath := qualificationSemanticMemberPath(t, *binding, "capture")
	var capture semanticProcessCapture
	if err := json.Unmarshal(f.files[capturePath], &capture); err != nil {
		t.Fatal(err)
	}
	capture.Spec.Path, capture.Spec.Dir = f.witness.Native.Process.Path, f.witness.Native.Process.Dir
	capture.Spec.Args, capture.Spec.Env = slices.Clone(f.witness.Native.Process.Args), slices.Clone(f.witness.Native.Process.Env)
	capture.Spec.CgroupParent = f.witness.Native.Process.CgroupParent
	if capture.Process != nil {
		processPath := "oracle/" + capture.Process.Path
		f.replace(processPath, qualificationSemanticWindowsExecution(t, f.files[processPath]))
		capture.Process.Bytes, capture.Process.SHA256 = uint64(len(f.files[processPath])), digestHex(f.files[processPath])
	}
	f.replace(capturePath, eligibilityJSON(t, capture))
	projectionPath := qualificationSemanticMemberPath(t, *binding, "projection")
	var projection map[string]json.RawMessage
	if err := json.Unmarshal(f.files[projectionPath], &projection); err != nil {
		t.Fatal(err)
	}
	var projectionExecution eligibilityExecution
	if err := json.Unmarshal(projection["process"], &projectionExecution); err != nil {
		t.Fatal(err)
	}
	projection["process"] = eligibilityJSON(t, qualificationSemanticWindowsExecutionValue(projectionExecution))
	f.replace(projectionPath, eligibilityJSON(t, projection))

	buildManifestPath := f.reg.Native.BuildManifest.Member.Path
	buildManifest := strings.Replace(string(f.files[buildManifestPath]), "os=linux/amd64\n", "os=windows/amd64\n", 1)
	if buildManifest == string(f.files[buildManifestPath]) {
		t.Fatal("synthetic native build manifest lost its original Linux platform")
	}
	f.replace(buildManifestPath, []byte(buildManifest))
	oldBuildIdentity, buildIdentity := f.witness.Native.ProducerSHA256, digestHex([]byte(buildManifest))
	f.witness.Native.ProducerSHA256 = buildIdentity
	for path, data := range f.files {
		if raw := string(data); strings.Contains(raw, oldBuildIdentity) {
			f.replace(path, []byte(strings.ReplaceAll(raw, oldBuildIdentity, buildIdentity)))
		}
	}
	for i := range f.reg.Invocations {
		inv := &f.reg.Invocations[i]
		if inv.ID == "native-current" {
			inv.ApprovedRequest = eligibilityJSON(t, f.witness.Native)
			f.replace(inv.Request.Member.Path, eligibilityJSON(t, f.witness.Native))
		}
	}
	capture = semanticProcessCapture{}
	if err := json.Unmarshal(f.files[capturePath], &capture); err != nil {
		t.Fatal(err)
	}
	if capture.Spec != nil && capture.Spec.Stdin != nil {
		stdinPath := "oracle/" + capture.Spec.Stdin.Path
		capture.Spec.Stdin.Bytes, capture.Spec.Stdin.SHA256 = uint64(len(f.files[stdinPath])), digestHex(f.files[stdinPath])
	}
	for _, pin := range []*ReplayMember{capture.Process, capture.Stdout, capture.Stderr} {
		if pin != nil {
			data := f.files["oracle/"+pin.Path]
			pin.Bytes, pin.SHA256 = uint64(len(data)), digestHex(data)
		}
	}
	f.replace(capturePath, eligibilityJSON(t, capture))
	buildPath := qualificationSemanticMemberPath(t, *binding, "build")
	var build semanticBuildDescription
	if err := json.Unmarshal(f.files[buildPath], &build); err != nil {
		t.Fatal(err)
	}
	build.Identity = digestHex([]byte(buildManifest))
	var buildRaw map[string]json.RawMessage
	if err := json.Unmarshal(f.files[buildPath], &buildRaw); err != nil {
		t.Fatal(err)
	}
	buildRaw["identity"] = mustJSON(t, build.Identity)
	f.replace(buildPath, mustJSON(t, buildRaw))
	var manifest OracleManifest
	if err := json.Unmarshal(f.files["oracle/manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Producer["platform"], manifest.Producer["build_identity"] = "windows/amd64", build.Identity
	f.replace("oracle/manifest.json", mustJSON(t, manifest))
}

func qualificationSemanticWindowsRequest(request *eligibilityProcessRequest) {
	request.Path = qualificationSemanticWindowsPath(request.Path)
	if strings.HasSuffix(request.Path, `\reference`) {
		request.Path += ".exe"
	}
	request.Dir = qualificationSemanticWindowsPath(request.Dir)
	for i := range request.Args {
		request.Args[i] = qualificationSemanticWindowsPath(request.Args[i])
	}
	if len(request.Args) >= 4 {
		request.Args[0] += ".dll"
		request.Args[2] += ".txt"
		request.Args[3] += ".scm"
	}
	request.CgroupParent = ""
}

func qualificationSemanticWindowsPath(path string) string {
	if strings.HasPrefix(path, "/current") {
		return `C:\current` + strings.ReplaceAll(strings.TrimPrefix(path, "/current"), "/", `\`)
	}
	return path
}

func qualificationSemanticWindowsExecution(t *testing.T, data []byte) []byte {
	t.Helper()
	var execution eligibilityExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		t.Fatal(err)
	}
	return eligibilityJSON(t, qualificationSemanticWindowsExecutionValue(execution))
}

func qualificationSemanticWindowsExecutionValue(execution eligibilityExecution) eligibilityExecution {
	execution.Cleanup.Method, execution.Cleanup.Scope = "JOB_OBJECT", "JOB_OBJECT"
	execution.Memory.Metric = "windows job-object process memory metric"
	execution.Capabilities.OS, execution.Capabilities.Backend = "windows/amd64", "windows-job-object"
	execution.Capabilities.TreeCleanup, execution.Capabilities.EscapedDescendants = "JOB_OBJECT", "CONTAINED"
	execution.Capabilities.Memory, execution.Capabilities.MemoryMetric = "HARD", execution.Memory.Metric
	return execution
}

func qualificationSemanticMemberPath(t *testing.T, binding semanticRunBinding, role string) string {
	t.Helper()
	for _, member := range binding.Members {
		if member.Member.Role == role {
			return member.Member.Path
		}
	}
	t.Fatalf("synthetic semantic fixture has no %q member", role)
	return ""
}

func qualificationSemanticRebindRecord(t *testing.T, f *eligibilityFixture, binding semanticRunBinding, registration []byte) {
	t.Helper()
	var recordPath, projectionPath, capturePath string
	for _, member := range binding.Members {
		switch member.Member.Role {
		case "record":
			recordPath = member.Member.Path
		case "projection":
			projectionPath = member.Member.Path
		case "capture":
			capturePath = member.Member.Path
		}
	}
	if recordPath == "" || projectionPath == "" || capturePath == "" {
		t.Fatal("synthetic semantic fixture is missing its record, projection or capture")
	}
	var record, projection, local map[string]json.RawMessage
	if err := json.Unmarshal(f.files[recordPath], &record); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(f.files[projectionPath], &projection); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(record["query_semantics"], &local); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"process", "steps", "producer"} {
		if value, ok := projection[key]; ok {
			record[key] = value
		}
	}
	local["registration_sha256"] = mustJSON(t, digestHex(registration))
	for key := range map[string]bool{"projection": true, "capture": true} {
		var pin ReplayMember
		if err := json.Unmarshal(local[key], &pin); err != nil {
			t.Fatal(err)
		}
		path := "oracle/" + pin.Path
		pin.Bytes, pin.SHA256 = uint64(len(f.files[path])), digestHex(f.files[path])
		local[key] = mustJSON(t, pin)
	}
	record["query_semantics"] = mustJSON(t, local)
	delete(record, "complete")
	data := mustJSON(t, record)
	data = append(data[:len(data)-1], []byte(`,"complete":true}`)...)
	f.replace(recordPath, data)
}

func qualificationSemanticRegistrationDigest(t *testing.T, f *eligibilityFixture, binding semanticRunBinding, registration []byte) {
	t.Helper()
	for _, member := range binding.Members {
		if member.Member.Role != "record" {
			continue
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal(f.files[member.Member.Path], &record); err != nil {
			t.Fatal(err)
		}
		var local struct {
			RegistrationSHA256 string `json:"registration_sha256"`
		}
		if err := json.Unmarshal(record["query_semantics"], &local); err != nil {
			t.Fatal(err)
		}
		if local.RegistrationSHA256 == digestHex(registration) {
			return
		}
		old := `"registration_sha256":"` + local.RegistrationSHA256 + `"`
		updated := strings.Replace(string(f.files[member.Member.Path]), old, `"registration_sha256":"`+digestHex(registration)+`"`, 1)
		if updated == string(f.files[member.Member.Path]) {
			t.Fatalf("synthetic local record lacks its registration digest %s", local.RegistrationSHA256)
		}
		f.replace(member.Member.Path, []byte(updated))
		return
	}
	t.Fatal("synthetic semantic fixture has no record member")
}

func qualificationSemanticInput(t *testing.T, core *qualificationSemanticCoreFixture, planSHA, graphSHA string) QualificationQuerySemanticsInput {
	t.Helper()
	ctx := qualSemanticContext{Schema: QualificationSemanticsContextSchema, PlanSHA256: planSHA, TargetID: core.id, Repository: "o/r", Workflow: "Foundation",
		WorkflowPath: ".github/workflows/foundation.yml", WorkflowRevision: strings.Repeat("d", 40), RunID: "500", Attempt: 1, Event: "pull_request",
		HeadSHA: qualificationSemanticHead, Checkout: qfxCandidate, ArtifactID: "synthetic-artifact-" + core.id,
		ArtifactSHA256: digestHex([]byte("synthetic-artifact-" + core.id)), GraphSHA256: graphSHA}
	ctxBytes := mustJSON(t, ctx)
	var approval eligibilityApproval
	if err := json.Unmarshal(core.req.EligibilityApproval, &approval); err != nil {
		t.Fatal(err)
	}
	approval.Candidate = eligibilityCandidate{SourceRevision: qfxCandidate, Platform: core.platform, Run: "500", Attempt: 1}
	var provenanceID string
	for i := range approval.Cohorts {
		if approval.Cohorts[i].Purpose == "current" {
			cohort := &approval.Cohorts[i]
			cohort.SourceRevision, cohort.Platform, cohort.Run, cohort.Attempt = qfxCandidate, core.platform, "500", 1
			provenanceID = cohort.ProvenanceID
		}
	}
	for i := range approval.Provenance {
		p := &approval.Provenance[i]
		if p.ID != provenanceID {
			continue
		}
		p.Kind, p.SourceRevision, p.ReviewedContextSHA256 = "reviewed-ci", qfxCandidate, digestHex(ctxBytes)
		p.Repository, p.WorkflowPath, p.WorkflowRevision = "o/r", ".github/workflows/foundation.yml", strings.Repeat("d", 40)
		p.RunID, p.Attempt, p.ArtifactID = "500", 1, ctx.ArtifactID
		p.ArtifactSHA256, p.SessionID = ctx.ArtifactSHA256, ""
	}
	core.req.EligibilityApproval = mustJSON(t, approval)
	core.binding.EligibilityApprovalSHA256 = digestHex(core.req.EligibilityApproval)
	core.req.RunBinding = mustJSON(t, core.binding)
	root, replayProfile := semanticBoundReplay(t, core.req, core.f, core.binding)
	var replay ReplayProfile
	if err := json.Unmarshal(replayProfile, &replay); err != nil {
		t.Fatal(err)
	}
	replay.Subject.Commit, replay.Subject.Run, replay.Subject.Attempt = qfxCandidate, "500", 1
	replay.Subject.Platform, replay.Subject.EvidenceMode = core.platform, ModeNewRun
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(replayProfile, &raw); err != nil {
		t.Fatal(err)
	}
	raw["subject"] = mustJSON(t, replay.Subject)
	return QualificationQuerySemanticsInput{ID: core.id, Replay: ReplayRequest{Root: root, Profile: mustJSON(t, raw)},
		EligibilityApproval: slices.Clone(core.req.EligibilityApproval), Context: ctxBytes}
}

func cloneQualificationSemanticRequest(t *testing.T, req QualifyRequest) QualifyRequest {
	t.Helper()
	copy := req
	copy.Inventory, copy.Hosts = slices.Clone(req.Inventory), slices.Clone(req.Hosts)
	if req.QuerySemantics == nil {
		return copy
	}
	semantics := *req.QuerySemantics
	semantics.Plan = slices.Clone(req.QuerySemantics.Plan)
	semantics.Targets = slices.Clone(req.QuerySemantics.Targets)
	for i := range semantics.Targets {
		semantics.Targets[i].Replay.Profile = slices.Clone(req.QuerySemantics.Targets[i].Replay.Profile)
		semantics.Targets[i].EligibilityApproval = slices.Clone(req.QuerySemantics.Targets[i].EligibilityApproval)
		semantics.Targets[i].Context = slices.Clone(req.QuerySemantics.Targets[i].Context)
	}
	copy.QuerySemantics = &semantics
	return copy
}

func qualificationSemanticRow(result QualificationResult, platform string) *QualQuerySemanticsTarget {
	if result.QuerySemantics == nil {
		return nil
	}
	for i := range result.QuerySemantics.Targets {
		if result.QuerySemantics.Targets[i].Platform == platform {
			return &result.QuerySemantics.Targets[i]
		}
	}
	return nil
}

func TestQualificationSmallSemanticFanIn(t *testing.T) {
	fixture := newQualificationSemanticFixtureState(t)
	t.Run("positive partial-scope fold", func(t *testing.T) {
		baselineRequest := cloneQualificationSemanticRequest(t, fixture.request)
		baselineRequest.QuerySemantics = nil
		baseline, err := Qualify(ctxT(t), baselineRequest)
		if err != nil || baseline.Assessment != AssessPass || baseline.SupportClaim != "SUPPORTED" || len(baseline.Cells) != 2 {
			t.Fatalf("synthetic Windows/Linux baseline: err=%v assessment=%s support=%s cells=%d", err, baseline.Assessment, baseline.SupportClaim, len(baseline.Cells))
		}
		got, err := Qualify(ctxT(t), fixture.request)
		if err != nil || got.Assessment != AssessBlocked || got.SupportClaim != "BLOCKED" || got.Baseline == nil || got.Baseline.Assessment != AssessPass || got.QuerySemantics == nil || got.QuerySemantics.Assessment != AssessPass || got.QuerySemantics.FullScopeComplete || !got.QuerySemantics.ProcessingComplete {
			t.Fatalf("partial semantic scope escaped conservative fold: err=%v assessment=%s support=%s baseline=%+v semantics=%s", err, got.Assessment, got.SupportClaim, got.Baseline, mustJSON(t, got.QuerySemantics))
		}
		for _, platform := range []string{"windows-amd64", "linux-amd64"} {
			row := qualificationSemanticRow(got, platform)
			if row == nil || row.Assessment != AssessPass || row.State != "REPLAYED" || row.Tuple == nil || row.Tuple.Required != 3 || row.Tuple.Verified != 3 || row.Tuple.Failed != 0 || row.Tuple.Blocked != 0 || row.Tuple.Unprocessed != 0 || row.Core == nil || row.Core.Assessment != AssessPass {
				t.Fatalf("synthetic %s target did not preserve its passing fixed denominator: %+v", platform, row)
			}
		}
	})
	t.Run("caller approval moved into bundle after admission", func(t *testing.T) {
		req := cloneQualificationSemanticRequest(t, fixture.request)
		outside := filepath.Join(t.TempDir(), "external-approval.json")
		if err := os.WriteFile(outside, req.QuerySemantics.Targets[0].EligibilityApproval, 0o600); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(outside)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := file.Stat()
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		req.QuerySemantics.ForbiddenFileIdentities = []fs.FileInfo{identity}
		if err := os.Rename(outside, filepath.Join(req.QuerySemantics.Targets[0].Replay.Root, "approval.json")); err != nil {
			t.Fatal(err)
		}
		got, err := Qualify(ctxT(t), req)
		row := qualificationSemanticRow(got, "windows-amd64")
		if err == nil || asError(err).Code != "TRUST_DOCUMENT_FILE_ALIAS" || row == nil || row.Core != nil {
			t.Fatalf("physical approval alias passed the API boundary: err=%v row=%+v", err, row)
		}
	})
	t.Run("caller mutations", func(t *testing.T) {
		for _, mutation := range []string{"subject-commit", "workflow", "artifact", "head", "local-approval", "missing-approval", "approval-copy", "plan-route"} {
			t.Run(mutation, func(t *testing.T) {
				req := cloneQualificationSemanticRequest(t, fixture.request)
				target := &req.QuerySemantics.Targets[0]
				switch mutation {
				case "subject-commit":
					var replay ReplayProfile
					if err := json.Unmarshal(target.Replay.Profile, &replay); err != nil {
						t.Fatal(err)
					}
					replay.Subject.Commit = strings.Repeat("b", 40)
					var raw map[string]json.RawMessage
					if err := json.Unmarshal(target.Replay.Profile, &raw); err != nil {
						t.Fatal(err)
					}
					raw["subject"] = mustJSON(t, replay.Subject)
					target.Replay.Profile = mustJSON(t, raw)
				case "workflow", "artifact", "head":
					var context qualSemanticContext
					if err := json.Unmarshal(target.Context, &context); err != nil {
						t.Fatal(err)
					}
					switch mutation {
					case "workflow":
						context.Workflow = "Untrusted"
					case "artifact":
						context.ArtifactID = "different-artifact"
					case "head":
						context.HeadSHA = strings.Repeat("c", 40)
					}
					target.Context = mustJSON(t, context)
				case "local-approval":
					var approval eligibilityApproval
					if err := json.Unmarshal(target.EligibilityApproval, &approval); err != nil {
						t.Fatal(err)
					}
					for i := range approval.Provenance {
						p := &approval.Provenance[i]
						if p.Kind != "reviewed-ci" {
							continue
						}
						p.Kind, p.SessionID = "approved-local", "synthetic-local-donor"
						p.Repository, p.WorkflowPath, p.WorkflowRevision = "", "", ""
						p.RunID, p.Attempt, p.ArtifactID, p.ArtifactSHA256 = "", 0, "", ""
					}
					target.EligibilityApproval = mustJSON(t, approval)
				case "missing-approval":
					target.EligibilityApproval = nil
				case "approval-copy":
					target.EligibilityApproval = append(target.EligibilityApproval, ' ')
				case "plan-route":
					var plan qualSemanticPlan
					if err := json.Unmarshal(req.QuerySemantics.Plan, &plan); err != nil {
						t.Fatal(err)
					}
					plan.Targets[0].Route = "unregistered-route"
					req.QuerySemantics.Plan = eligibilityJSON(t, plan)
				}
				got, err := Qualify(ctxT(t), req)
				if err != nil {
					t.Fatalf("qualify mutation %s: %v", mutation, err)
				}
				row := qualificationSemanticRow(got, "windows-amd64")
				if row == nil || row.Tuple == nil || row.Tuple.Required != 3 {
					t.Fatalf("%s lost the planned denominator: %+v", mutation, row)
				}
				switch mutation {
				case "local-approval", "missing-approval":
					if got.Assessment != AssessBlocked || row.Assessment != AssessBlocked || row.Tuple.Blocked != 3 || row.Tuple.Failed != 0 || row.Core != nil {
						t.Fatalf("%s was not a blocked fixed-denominator result: result=%s tuple=%+v row=%+v", mutation, got.Assessment, *row.Tuple, *row)
					}
				case "approval-copy":
					if got.Assessment != AssessFail || row.Assessment != AssessFail || row.Code != "SEMANTICS_CALLER_APPROVAL_MISMATCH" || row.Core == nil || row.Core.Assessment != AssessFail {
						t.Fatalf("caller approval copy mismatch was not preserved: %+v", row)
					}
				default:
					if got.Assessment != AssessFail || row.Assessment != AssessFail || row.Tuple.Failed != 3 || row.Core != nil {
						t.Fatalf("%s did not fail before semantic core completion: %+v", mutation, row)
					}
				}
			})
		}
	})
	t.Run("plan query mismatch after context rebinding", func(t *testing.T) {
		req := cloneQualificationSemanticRequest(t, fixture.request)
		var plan qualSemanticPlan
		if err := json.Unmarshal(req.QuerySemantics.Plan, &plan); err != nil {
			t.Fatal(err)
		}
		plan.Targets[0].Queries[0].SHA256 = strings.Repeat("0", 64)
		req.QuerySemantics.Plan = eligibilityJSON(t, plan)
		planSHA := digestHex(req.QuerySemantics.Plan)
		for _, p := range []string{"windows-amd64", "linux-amd64"} {
			core := fixture.cores[p]
			input := qualificationSemanticInput(t, core, planSHA, core.f.reg.Graph.SHA256)
			for i := range req.QuerySemantics.Targets {
				if req.QuerySemantics.Targets[i].ID == core.id {
					req.QuerySemantics.Targets[i] = input
				}
			}
		}
		got, err := Qualify(ctxT(t), req)
		if err != nil {
			t.Fatalf("qualify rebound plan query hash: %v", err)
		}
		row := qualificationSemanticRow(got, "windows-amd64")
		if row == nil || got.Assessment != AssessFail || row.Assessment != AssessFail || row.Code != "SEMANTICS_PLAN_QUERY_MISMATCH" || row.Core == nil || row.Core.Assessment != AssessFail {
			t.Fatalf("rebound plan query hash mismatch escaped the preregistration: %+v", row)
		}
	})
	t.Run("different planned tuple IDs do not borrow core counters", func(t *testing.T) {
		req := cloneQualificationSemanticRequest(t, fixture.request)
		var plan qualSemanticPlan
		if err := json.Unmarshal(req.QuerySemantics.Plan, &plan); err != nil {
			t.Fatal(err)
		}
		for i := range plan.Targets[0].Required {
			plan.Targets[0].Required[i].Case = "other-required-case"
		}
		req.QuerySemantics.Plan = eligibilityJSON(t, plan)
		for _, platform := range []string{"windows-amd64", "linux-amd64"} {
			core := fixture.cores[platform]
			input := qualificationSemanticInput(t, core, digestHex(req.QuerySemantics.Plan), core.f.reg.Graph.SHA256)
			for i := range req.QuerySemantics.Targets {
				if req.QuerySemantics.Targets[i].ID == core.id {
					req.QuerySemantics.Targets[i] = input
				}
			}
		}
		got, err := Qualify(ctxT(t), req)
		row := qualificationSemanticRow(got, "windows-amd64")
		if err != nil || row == nil || row.Code != "SEMANTICS_PLAN_TUPLE_MISMATCH" || row.Tuple == nil || row.Tuple.Required != 3 || row.Tuple.Verified != 0 || row.Tuple.Failed != 3 || row.Core == nil || row.Core.Tuple.Required != 3 || row.Core.Tuple.Verified != 3 || row.Core.Assessment != AssessFail {
			t.Fatalf("different IDs borrowed core counters: err=%v row=%+v", err, row)
		}
	})
	t.Run("Oracle source mismatch after graph and approval rebinding", func(t *testing.T) {
		core := fixture.cores["windows-amd64"]
		node := core.f.nodes[core.binding.OracleNode]
		node.Identities["source"] = strings.Repeat("b", 40)
		core.f.nodes[node.ID] = node
		semanticBoundReapprove(t, &core.req, core.f, &core.binding)
		req := cloneQualificationSemanticRequest(t, fixture.request)
		input := qualificationSemanticInput(t, core, digestHex(req.QuerySemantics.Plan), core.f.reg.Graph.SHA256)
		for i := range req.QuerySemantics.Targets {
			if req.QuerySemantics.Targets[i].ID == core.id {
				req.QuerySemantics.Targets[i] = input
			}
		}
		got, err := Qualify(ctxT(t), req)
		if err != nil {
			t.Fatalf("qualify rebound Oracle source: %v", err)
		}
		row := qualificationSemanticRow(got, "windows-amd64")
		if row == nil || got.Assessment != AssessFail || row.Assessment != AssessFail || row.Code != "SEMANTICS_ORACLE_SOURCE_MISMATCH" || row.Core == nil || row.Core.Assessment != AssessFail {
			t.Fatalf("rehashing the graph, policy, approval and context hid the source mismatch: %+v", row)
		}
		limit := uint64(len(req.QuerySemantics.Plan)) + row.BytesRead + uint64(len(req.QuerySemantics.Targets[1].Replay.Profile)) - 1
		oldLimits := testHookReplayLimits
		testHookReplayLimits = func(limits ReplayLimits) ReplayLimits { limits.TotalBytes = limit; return limits }
		t.Cleanup(func() { testHookReplayLimits = oldLimits })
		partial, interrupted := Qualify(ctxT(t), req)
		if interrupted == nil || asError(interrupted).Kind != KindResourceLimit || partial.ExecutionStatus != StatusResourceLimit || partial.Assessment != AssessFail || partial.QuerySemantics == nil || partial.QuerySemantics.Code != row.Code {
			t.Fatalf("later resource stop replaced the first failure: err=%v status=%s result=%s semantics=%+v", interrupted, partial.ExecutionStatus, partial.Assessment, partial.QuerySemantics)
		}
		first, second := qualificationSemanticRow(partial, "windows-amd64"), qualificationSemanticRow(partial, "linux-amd64")
		// Source binding fails the row without relabeling the core's observed tuple counters.
		if first == nil || first.Code != row.Code || first.Core == nil || first.Core.Assessment != AssessFail || first.Tuple == nil || first.Tuple.Required != 3 || *first.Tuple != *row.Tuple || first.Core.Tuple != row.Core.Tuple || second == nil || second.Core != nil || second.Tuple == nil || second.Tuple.Required != 3 || second.Tuple.Unprocessed != 3 {
			t.Fatalf("partial stop lost the failed core or planned second denominator: first=%+v second=%+v", first, second)
		}
	})
}
