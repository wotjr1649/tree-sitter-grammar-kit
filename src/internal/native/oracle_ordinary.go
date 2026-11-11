package native

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// OrdinaryRecordingRequest binds source-only planning to an executable r1/r2 profile.
// Collection preserves CAPACITY_UNPROVEN; it does not admit a semantic verification pass.
type OrdinaryRecordingRequest struct {
	Inputs       kit.OrdinaryPlanRequest
	ShardPlan    []byte
	Registration []byte
	ShardIDs     []string
}

type ordinaryRecording struct {
	profile      kit.OrdinaryProfile
	registration kit.OrdinaryRegistration
	sources      [][]byte
	shardIDs     []string
}

func ordinaryPin(path string, data []byte) kit.OrdinaryPin {
	return kit.OrdinaryPin{Path: path, Bytes: uint64(len(data)), SHA256: sha(data)}
}

func prepareOrdinary(ctx context.Context, req OracleRequest, prof kit.OracleProfile) (*ordinaryRecording, error) {
	if req.Ordinary == nil {
		return nil, refuse(kit.KindInvalidInput, "ORDINARY_INPUT_REQUIRED", nil)
	}
	n := prof.Native
	if n.Operation != "native-query-batch" || n.Output != kit.OutputTree || n.Encoding != kit.EncodingUTF8 || n.Format != "" || n.Preprocess != nil || n.SvcContext != nil || !prof.API || len(prof.Queries) == 0 || req.ReuseAccepted != nil || req.QuerySemanticsRegistration != nil || req.RunWall != 0 || n.Schema == kit.OracleSemanticsSchema {
		return nil, refuse(kit.KindUnsupported, "ORDINARY_CONTEXT_UNSUPPORTED", nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	in := req.Ordinary
	p, err := kit.ParseOrdinaryProfile(in.Inputs.Profile)
	if err != nil {
		return nil, err
	}
	r, err := kit.ParseOrdinaryRegistration(in.Registration)
	if err != nil {
		return nil, err
	}
	plan, err := kit.ParseOrdinaryShardPlan(in.ShardPlan)
	if err != nil {
		return nil, err
	}
	if err := kit.ValidateOrdinaryShardPlan(plan, in.Inputs); err != nil {
		return nil, err
	}
	registered := false
	for _, raw := range in.Inputs.Registrations {
		registered = registered || bytes.Equal(raw, in.Registration)
	}
	if !registered || r.Route != n.Route || r.Subject != hostPlatform || r.Profile != ordinaryPin(r.Profile.Path, in.Inputs.Profile) || r.Assets != ordinaryPin(r.Assets.Path, in.Inputs.Assets) || r.Candidate != p.Candidate || r.SemanticCohort != p.SemanticCohort {
		return nil, refuse(kit.KindInvalidInput, "ORDINARY_EXECUTION_BINDING", nil)
	}
	if err := validateOrdinaryShardSelection(in.ShardIDs, plan, r, n.Cases); err != nil {
		return nil, err
	}
	prepared, err := bindOrdinaryExecution(ctx, req, prof, p, r)
	if err != nil {
		return nil, err
	}
	prepared.shardIDs = slices.Clone(in.ShardIDs)
	return prepared, nil
}

func validateOrdinaryShardSelection(ids []string, plan kit.OrdinaryShardPlan, r kit.OrdinaryRegistration, cases []kit.IncrementalCase) error {
	if len(ids) == 0 || len(ids) != len(cases) {
		return refuse(kit.KindInvalidInput, "ORDINARY_SHARD_SELECTION", nil)
	}
	shards := make(map[string]kit.OrdinaryPlanShard, len(plan.Shards))
	for _, shard := range plan.Shards {
		shards[shard.ID] = shard
	}
	seen := map[string]bool{}
	for i, id := range ids {
		shard, ok := shards[id]
		if !ok || seen[id] || shard.Route != r.Route || shard.Platform != r.Platform || shard.Subject != r.Subject || shard.Case != cases[i].ID {
			return refuse(kit.KindInvalidInput, "ORDINARY_SHARD_SELECTION", nil)
		}
		seen[id] = true
	}
	return nil
}

func bindOrdinaryExecution(ctx context.Context, req OracleRequest, prof kit.OracleProfile, p kit.OrdinaryProfile, r kit.OrdinaryRegistration) (*ordinaryRecording, error) {
	n := prof.Native
	var binding *kit.OrdinaryBinding
	for i := range p.Bindings {
		if p.Bindings[i].Route == r.Route && p.Bindings[i].Platform == r.Platform {
			binding = &p.Bindings[i]
			break
		}
	}
	runtimeManifest, err := drivers.NativeC.ReadFile("native-c/runtime-manifest.json")
	if err != nil {
		return nil, err
	}
	if binding == nil || !reflect.DeepEqual(binding.Grammar, n.Grammar) || binding.Runtime != ordinaryPin(binding.Runtime.Path, runtimeManifest) || len(r.Queries) != len(prof.Queries) {
		return nil, refuse(kit.KindInvalidInput, "ORDINARY_BUILD_BINDING", nil)
	}
	for i, q := range prof.Queries {
		if q.ID != r.Queries[i].ID || ordinaryPin(r.Queries[i].Source.Path, []byte(q.Source)) != r.Queries[i].Source {
			return nil, refuse(kit.KindInvalidInput, "ORDINARY_QUERY_BINDING", nil)
		}
	}
	var handle *os.Root
	if req.Inputs != nil {
		handle = req.Inputs.Cases
	}
	out := &ordinaryRecording{profile: p, registration: r}
	last := -1
	for _, c := range n.Cases {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		index := slices.IndexFunc(r.Cases, func(rc kit.OrdinaryCase) bool { return rc.ID == c.ID })
		if index <= last || c.Encoding != kit.EncodingUTF8 || len(c.SvcSource) != 0 {
			return nil, refuse(kit.KindInvalidInput, "ORDINARY_CASE_BINDING", nil)
		}
		last = index
		rc := r.Cases[index]
		if !reflect.DeepEqual(c.Edits, rc.Edits) && !(len(c.Edits) == 0 && len(rc.Edits) == 0) {
			return nil, refuse(kit.KindInvalidInput, "ORDINARY_HISTORY_BINDING", nil)
		}
		source, err := readCase(req.Root, c.Input, handle)
		if err != nil {
			return nil, err
		}
		versions, points, err := kit.ApplyEdits(kit.EncodingUTF8, source, c.Edits, 65536)
		if err != nil {
			return nil, err
		}
		if len(versions) != len(rc.VersionPins) || !reflect.DeepEqual(points, rc.Points) && !(len(points) == 0 && len(rc.Points) == 0) {
			return nil, refuse(kit.KindInvalidInput, "ORDINARY_HISTORY_BINDING", nil)
		}
		for step, version := range versions {
			if rc.VersionPins[step] != ordinaryPin(rc.VersionPins[step].Path, version) {
				return nil, refuse(kit.KindInvalidInput, "ORDINARY_SOURCE_BINDING", nil)
			}
		}
		out.sources = append(out.sources, source)
	}
	if len(out.sources) == 0 {
		return nil, refuse(kit.KindInvalidInput, "ORDINARY_CASE_BINDING", nil)
	}
	return out, nil
}

func ordinaryRecordError(ctx context.Context, res OracleResult, err error) (OracleResult, error) {
	var ne *Error
	var ke *kit.Error
	if ctx.Err() != nil {
		err = refuse(kit.KindCancelled, "CANCELLED", err)
	} else if errors.Is(err, context.DeadlineExceeded) {
		err = refuse(kit.KindResourceLimit, "WALL_LIMIT", err)
	} else if errors.As(err, &ke) {
		err = refuse(ke.Kind, ke.Code, ke)
	}
	if !errors.As(err, &ne) {
		err = refuse(kit.KindIO, "ORDINARY_RECORD_FAILED", err)
		errors.As(err, &ne)
	}
	res.Findings = append(res.Findings, finding(ne.Code, "error", "", "ordinary producer를 완결하지 못했다"))
	if ne.Kind == kit.KindCancelled {
		res.ExecutionStatus = kit.StatusCancelled
	} else if ne.Kind == kit.KindResourceLimit {
		res.ExecutionStatus = kit.StatusResourceLimit
	} else if res.ExecutionStatus == kit.StatusCompleted {
		res.ExecutionStatus = kit.StatusFailed
	}
	if res.Assessment != kit.AssessFail && (ne.Kind == kit.KindUnsupported || ne.Kind == kit.KindResourceLimit) {
		res.Assessment = kit.AssessBlocked
	} else if res.Assessment == kit.AssessPass {
		res.Assessment = kit.AssessNotAssessed
	}
	return res, err
}

func oracleOrdinary(ctx context.Context, req OracleRequest) (res OracleResult, resultErr error) {
	start := time.Now()
	res = OracleResult{Report: kit.Report{Schema: kit.ReportSchema, Command: "oracle record", ExecutionStatus: kit.StatusNotRun, EvidenceMode: kit.ModeNotRun, Assessment: kit.AssessNotAssessed, Identities: []kit.IdentityRef{}, Findings: []kit.Finding{}, Coverage: kit.Coverage{Requested: []string{}, Observed: []string{}, Unsupported: []string{}}}, ResultSchema: "tsgk-oracle-result/r4", Cases: []CaseLine{}, Platform: hostPlatform}
	fail := func(err error) (OracleResult, error) { return ordinaryRecordError(ctx, res, err) }
	if !slices.Contains(req.Allow, AllowBuild) || !slices.Contains(req.Allow, AllowExec) {
		return fail(refuse(kit.KindUnsupported, "CAPABILITY_NOT_GRANTED", nil))
	}
	prof, err := kit.ParseOracleProfile(req.Profile)
	if err != nil {
		return fail(err)
	}
	if prof.FactPack == nil && slices.ContainsFunc(prof.Queries, func(q kit.OracleQuery) bool { return strings.HasPrefix(q.ID, "facts.") }) {
		return fail(refuse(kit.KindInvalidInput, "ORDINARY_FACT_PACK_REQUIRED", nil))
	}
	if prof.FactPack != nil && prof.FactPack.Route != prof.Native.Route {
		return fail(refuse(kit.KindInvalidInput, "ORDINARY_FACT_ROUTE_BINDING", nil))
	}
	if err := req.Inputs.validate(); err != nil {
		return fail(err)
	}
	prepared, err := prepareOrdinary(ctx, req, prof)
	if err != nil {
		return fail(err)
	}
	var pack *kit.FactPack
	if prof.FactPack != nil {
		value, e := checkFactPack(prof, req.FactPack)
		if e != nil {
			return fail(e)
		}
		pack = &value
		if len(req.FactMapping) == 0 || len(req.FactMapping) > kit.MaxDocumentBytes {
			return fail(refuse(kit.KindInvalidInput, "ORDINARY_MAPPING_BINDING", nil))
		}
		if _, _, _, e := validateOrdinaryFactPins(prof, pack, req.FactMapping); e != nil {
			return fail(refuse(kit.KindInvalidInput, "ORDINARY_FACT_BINDING", e))
		}
	} else if req.FactPack != nil || req.FactMapping != nil {
		return fail(refuse(kit.KindInvalidInput, "FACT_PACK_BINDING", nil))
	}
	res, err = recordOrdinaryPrepared(ctx, req, prof, pack, prepared, res, start)
	if err != nil {
		return fail(err)
	}
	return res, nil
}

func recordOrdinaryPrepared(ctx context.Context, req OracleRequest, prof kit.OracleProfile, pack *kit.FactPack, prepared *ordinaryRecording, res OracleResult, start time.Time) (result OracleResult, resultErr error) {
	n := prof.Native
	op := kit.NativeOperations()[n.Operation]
	res.ProfileID, res.Route, res.Operation, res.ProfileSHA256 = n.ID, n.Route, op, n.SHA256
	res.ObservationRunID = rand.Text()
	if f, e := platformScope(op); e != nil {
		res.Findings = append(res.Findings, f)
		return res, e
	}
	if probe := runner.Probe(runner.Spec{Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: true}, CgroupParent: req.CgroupParent}); probe.Memory != runner.MemoryHard {
		return res, refuse(kit.KindUnsupported, "MEMORY_HARD_CAP_UNSUPPORTED", nil)
	}
	output, err := newOutputDirectory(req.Out, req.OutTarget)
	if err != nil {
		return res, refuse(kit.KindIO, "OUTPUT_FAILED", err)
	}
	defer func() { closeOutput(output, &res.Report, &resultErr); result = res }()
	roots := map[string]*os.Root{}
	for _, name := range []string{"capture", "source", "raw", "projection", "records", "facts"} {
		child, e := createOutputDirectory(output.Root, name)
		if e != nil {
			return res, e
		}
		defer closeOutput(child, &res.Report, &resultErr)
		roots[name] = child.Root
	}
	live, cancel := context.WithTimeout(ctx, op.RunWall)
	defer cancel()
	groot := req.GrammarRoot
	if groot == "" {
		groot = req.Root
	}
	var grammarHandle *os.Root
	if req.Inputs != nil {
		grammarHandle = req.Inputs.Grammar
	}
	b, err := NewBuild(live, BuildRequest{Work: req.Work, Runtime: req.Runtime, GrammarRoot: groot, GrammarHandle: grammarHandle, Grammar: n.Grammar, Symbol: n.Symbol, Compiler: req.Compiler, CompilerID: n.Compiler, CgroupParent: req.CgroupParent, CacheDir: req.BuildCache})
	res.Build = b
	removed := false
	if b != nil {
		defer func() {
			if !removed {
				if e := b.Remove(); e != nil {
					res.BuildRemoved = "FAILED"
					recordCleanupFailure(&res.Report, &resultErr, "BUILD_CLEANUP_FAILED", e)
				} else {
					res.BuildRemoved = "REMOVED"
				}
			}
		}()
	}
	if err != nil {
		return res, err
	}
	res.ExecutionStatus, res.EvidenceMode = kit.StatusCompleted, kit.ModeNewRun
	policy := PolicyRef(op, n.Output)
	x := Context{Op: op, Route: n.Route, Output: n.Output, Declarations: n.Declarations, PolicyRef: policy, Protocol: ProtocolR4, API: true, CgroupParent: req.CgroupParent, ObservationRunID: res.ObservationRunID, ObservationProducerSHA256: b.Identity}
	for _, q := range prof.Queries {
		x.Queries = append(x.Queries, QuerySource{ID: q.ID, Source: []byte(q.Source)})
	}
	members := []kit.OrdinaryMember{}
	var total uint64
	writeFile := func(file captureFile, owner, caseID string) error {
		lim := kit.OrdinaryPlanLimits()
		if file.pin.Bytes > lim.FileBytes || total > lim.ReadBytes-file.pin.Bytes || uint64(len(members)+2) > lim.Entries {
			return refuse(kit.KindResourceLimit, "ORDINARY_MEMBER_LIMIT", nil)
		}
		if err := live.Err(); err != nil {
			return err
		}
		root := roots[filepath.Dir(filepath.FromSlash(file.pin.Path))]
		if root == nil {
			return refuse(kit.KindInvalidInput, "ORDINARY_MEMBER_PATH", nil)
		}
		f, e := root.OpenFile(filepath.Base(file.pin.Path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if e != nil {
			return e
		}
		_, e = file.WriteTo(f)
		e = errors.Join(e, f.Close())
		if e != nil {
			return e
		}
		total += file.pin.Bytes
		members = append(members, kit.OrdinaryMember{Owner: owner, Path: file.pin.Path, Role: file.pin.Role, CaseOrBatch: caseID, Pin: ordinaryPinFromReplay(file.pin)})
		return nil
	}
	add := func(path, role, owner, caseID string, data []byte) (kit.ReplayMember, error) {
		pin := kit.ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: sha(data)}
		return pin, writeFile(captureFile{pin: pin, data: data}, owner, caseID)
	}
	if pack != nil {
		if _, e := add("facts/pack.json", "fact-pack", "O", "run", req.FactPack); e != nil {
			return res, e
		}
		if _, e := add("facts/mapping.json", "fact-mapping", "O", "run", req.FactMapping); e != nil {
			return res, e
		}
	}
	files, err := semanticBuildFilesIn(live, b, func() string {
		switch prepared.registration.ClosureEncoding {
		case "parser-paged":
			return semanticPagedContext
		case "binary-paged":
			return semanticPagedBinariesContext
		}
		return "full-input-utf8/r2"
	}())
	if err != nil {
		return res, err
	}
	for _, file := range files {
		file, err = ordinaryBuildOwner(file)
		if err != nil {
			return res, err
		}
		if err := writeFile(file, "D_build", "build"); err != nil {
			return res, err
		}
	}
	out := make([]CaseResult, len(n.Cases))
	reqs := make([]caseRequest, len(n.Cases))
	pending := []int{}
	for i, c := range n.Cases {
		out[i] = newCaseResult(c)
		if _, e := add(fmt.Sprintf("source/%05d-%s.bin", i, c.ID), "source", "O", c.ID, prepared.sources[i]); e != nil {
			return res, e
		}
		var done bool
		if reqs[i], done = b.prepareCase(live, x, c, prepared.sources[i], &out[i]); !done {
			pending = append(pending, i)
		}
	}
	_, captures, captureErr := b.runOracleBatchesCaptured(live, x, n.Cases, reqs, out, pending)
	defer func() {
		for i := range captures {
			if e := captures[i].close(); e != nil {
				recordCleanupFailure(&res.Report, &resultErr, "BATCH_CAPTURE_CLEANUP_FAILED", e)
			}
		}
	}()
	for i, oc := range prof.OracleCases {
		cr := &out[i]
		judgeOracle(cr, oc, prof, pack, x, prepared.sources[i])
		x.observeCase(cr)
		res.Cases = append(res.Cases, CaseLine{ID: cr.ID, ExecutionStatus: cr.ExecutionStatus, Assessment: cr.Assessment, Code: cr.Code, Claims: cr.Claims, Oracle: cr.Oracle, ReuseObservationClaim: cr.ReuseObservationClaim})
	}
	res.ExecutionStatus, res.Assessment = aggregate(out)
	res.Summary = summarize(out, nil)
	runBatches := []kit.OrdinaryRunBatch{}
	capturePins := map[int]kit.ReplayMember{}
	for i := range captures {
		capture := &captures[i]
		files, e := capture.files(fmt.Sprintf("capture/batch-%05d", i))
		if e != nil {
			return res, e
		}
		owner := fmt.Sprintf("D_batch_%05d", i)
		batch := kit.OrdinaryRunBatch{Index: i, Owner: owner, Cases: []string{}}
		for _, frame := range capture.frames {
			batch.Cases = append(batch.Cases, frame.ID)
		}
		for _, file := range files {
			if e := writeFile(file, owner, owner); e != nil {
				return res, e
			}
			if file.pin.Role == "native-batch-capture" {
				batch.Capture = file.pin
				capturePins[i] = file.pin
			}
		}
		runBatches = append(runBatches, batch)
	}
	for i := range prof.OracleCases {
		cr := &out[i]
		stepFacts := json.RawMessage("[]")
		if pack != nil {
			facts, e := ordinaryStepFacts(cr, prof, pack, x, prepared.sources[i], req.FactMapping)
			if e != nil {
				return res, e
			}
			stepFacts, e = json.Marshal(facts)
			if e != nil {
				return res, e
			}
		}
		name := fmt.Sprintf("%05d-%s.json", i, cr.ID)
		projection, e := json.Marshal(cr)
		if e != nil {
			return res, e
		}
		pin, e := add("projection/"+name, "projection", "O", cr.ID, projection)
		if e != nil {
			return res, e
		}
		var capturePin *kit.ReplayMember
		if cr.Transport != nil {
			if p, ok := capturePins[cr.Transport.Batch]; ok {
				capturePin = &p
			}
		}
		semanticLocalCase(cr, sha(req.Ordinary.Registration), capturePin, pin)
		if cr.Raw != nil {
			if _, e := add("raw/"+name, "raw", "O", cr.ID, cr.Raw); e != nil {
				return res, e
			}
		}
		transport := json.RawMessage("null")
		if cr.Transport != nil {
			transport, e = json.Marshal(cr.Transport)
			if e != nil {
				return res, e
			}
		}
		record := kit.OrdinaryOracleRecord{Schema: kit.OracleOrdinaryRecordSchema, Case: cr.ID, Workload: n.ID, BaseProjection: pin, QuerySemantics: cr.QuerySemantics, Transport: transport, StepFacts: stepFacts, Complete: true}
		data, e := json.Marshal(record)
		if e != nil {
			return res, e
		}
		if _, e := add("records/"+name, "record", "O", cr.ID, append(data, '\n')); e != nil {
			return res, e
		}
		res.Cases[i] = CaseLine{ID: cr.ID, ExecutionStatus: cr.ExecutionStatus, Assessment: cr.Assessment, Code: cr.Code, Claims: cr.Claims, Oracle: cr.Oracle, QuerySemantics: cr.QuerySemantics, ReuseObservationClaim: cr.ReuseObservationClaim}
	}
	res.ExecutionStatus, res.Assessment = aggregate(out)
	res.EvidenceMode = kit.ModeNewRun
	res.Summary = summarize(out, nil)
	res.WallMillis = time.Since(start).Milliseconds()
	if captureErr != nil {
		return res, captureErr
	}
	for i := range captures {
		if err := captures[i].close(); err != nil {
			return res, err
		}
	}
	captures = nil
	if err := b.Remove(); err != nil {
		res.BuildRemoved = "FAILED"
		return res, err
	}
	removed = true
	res.BuildRemoved = "REMOVED"
	manifest := kit.OrdinaryOracleManifest{Schema: kit.OracleOrdinaryManifestSchema, Context: kit.OrdinaryContext, ClosureEncoding: prepared.registration.ClosureEncoding, Profile: ordinaryPin("profile/executable.json", req.Profile), PlanningProfile: ordinaryPin("profile/tsgk-oracle-r4.json", req.Ordinary.Inputs.Profile), Registration: ordinaryPin("registrations/"+prepared.registration.Route+"/"+prepared.registration.Platform+".json", req.Ordinary.Registration), Assets: ordinaryPin("profile/ordinary-assets-r1.json", req.Ordinary.Inputs.Assets), Plan: ordinaryPin("plan/ordinary.json", req.Ordinary.ShardPlan), Candidate: prepared.profile.Candidate, SemanticCohort: prepared.profile.SemanticCohort, RunID: res.ObservationRunID, ProducerOwner: "O", Policy: policy, Operation: op, ExecutionStatus: res.ExecutionStatus, Assessment: res.Assessment, Cases: []string{}, Members: members, Batches: runBatches, Complete: true}
	manifest.Producer = map[string]string{"build_identity": b.Identity, "executable_sha256": b.ExecutableSHA256, "runtime_commit": b.Runtime, "compiler_sha256": b.Compiler.SHA256, "platform": hostPlatform}
	manifest.Producer["profile_id"] = n.ID
	manifest.ShardIDs = slices.Clone(prepared.shardIDs)
	for _, c := range n.Cases {
		manifest.Cases = append(manifest.Cases, c.ID)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return res, err
	}
	if uint64(len(data)+1) > kit.OrdinaryPlanLimits().FileBytes || total+uint64(len(data)+1) > kit.OrdinaryPlanLimits().ReadBytes {
		return res, refuse(kit.KindResourceLimit, "ORDINARY_MEMBER_LIMIT", nil)
	}
	if err := createExclusive(output.Root, "manifest.json", append(data, '\n')); err != nil {
		return res, err
	}
	binding := kit.OrdinaryRunBinding{Schema: "tsgk-query-semantics-run-binding/r2", Context: manifest.Context, Profile: manifest.Profile, PlanningProfile: manifest.PlanningProfile, Registration: manifest.Registration, Assets: manifest.Assets, Plan: manifest.Plan, Candidate: manifest.Candidate, SemanticCohort: manifest.SemanticCohort, RunID: manifest.RunID, ProducerOwner: manifest.ProducerOwner, Members: members, Batches: runBatches}
	binding.ShardIDs = slices.Clone(manifest.ShardIDs)
	res.OrdinaryBinding = &binding
	set := kit.VerifyOracleSet(output.FS())
	res.Set = &set
	if !set.Valid {
		res.ExecutionStatus = kit.StatusFailed
		return res, refuse(kit.KindInvalidInput, "REFERENCE_SET_INVALID", nil)
	}
	return res, nil
}

func ordinaryPinFromReplay(p kit.ReplayMember) kit.OrdinaryPin {
	return kit.OrdinaryPin{Path: p.Path, Bytes: p.Bytes, SHA256: p.SHA256}
}

func ordinaryBuildOwner(file captureFile) (captureFile, error) {
	var value any
	switch file.pin.Role {
	case "grammar-paged-descriptor":
		var descriptor semanticParserPages
		if err := json.Unmarshal(file.data, &descriptor); err != nil {
			return file, err
		}
		descriptor.Owner = "D_build"
		value = descriptor
	case "driver-paged-descriptor":
		var descriptor semanticBinaryPages
		if err := json.Unmarshal(file.data, &descriptor); err != nil {
			return file, err
		}
		descriptor.Owner = "D_build"
		value = descriptor
	default:
		return file, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return file, err
	}
	file.data = data
	file.pin.Bytes = uint64(len(data))
	file.pin.SHA256 = sha(data)
	return file, nil
}
