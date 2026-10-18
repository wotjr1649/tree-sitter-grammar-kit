package kit

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"slices"
	"unicode/utf8"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

const (
	QueryReferenceRegistrationSchema = "tsgk-query-reference-connection-registration/r1"
	QueryReferenceResultSchema       = "tsgk-query-reference-connection-result/r1"
	instrumentedReferenceSchema      = "tsgk-instrumented-query-reference/r1"
	instrumentedReferenceBuildSchema = "tsgk-instrumented-reference-build/r1"
	instrumentedReferenceCommit      = "10eb5ac119eec9cb4267a8e035066473983f5f3a"
)

// QueryReferenceNode fixes all eleven identity roles independently of the graph.
type QueryReferenceNode struct {
	ID         string            `json:"id"`
	Identities map[string]string `json:"identities"`
}

// QueryReferenceObservation registers a fresh response for one source version and query.
// The same observation may serve both tree roles at that version, counted separately.
type QueryReferenceObservation struct {
	Query        string       `json:"query"`
	Step         uint32       `json:"step"`
	SourceBytes  uint64       `json:"source_bytes"`
	SourceSHA256 string       `json:"source_sha256"`
	Response     ReplayMember `json:"response"`
}

// QueryReferenceRegistration is caller-trusted approval outside the generated evidence.
// Context is full-input-utf8/r1: the caller approves full input, no included ranges or
// composite format, and the build/tool/grammar/platform/run context behind these pins.
type QueryReferenceRegistration struct {
	Schema               string                      `json:"schema"`
	Context              string                      `json:"context"`
	EvidencePolicySHA256 string                      `json:"evidence_policy_sha256"`
	Native               QueryReferenceNode          `json:"native"`
	Reference            QueryReferenceNode          `json:"reference"`
	WitnessRegistration  ReplayMember                `json:"witness_registration"`
	Source               ReplayMember                `json:"source"`
	Raw                  ReplayMember                `json:"raw"`
	Projection           ReplayMember                `json:"projection"`
	Build                ReplayMember                `json:"build"`
	BinarySHA256         string                      `json:"binary_sha256"`
	LibrarySHA256        string                      `json:"library_sha256"`
	Symbol               string                      `json:"symbol"`
	Observations         []QueryReferenceObservation `json:"observations"`
}

type QueryReferenceConnectionRequest struct {
	Evidence            EvidenceRequest
	Registration        []byte
	WitnessRegistration []byte
}

// QueryReferenceConnectionResult describes stored data connections, not execution
// authenticity, receipt-closure approval or qualification eligibility.
type QueryReferenceConnectionResult struct {
	Report
	ResultSchema       string              `json:"result_schema"`
	RegistrationSHA256 string              `json:"registration_sha256"`
	Required           int                 `json:"required"`
	Verified           int                 `json:"verified"`
	NativeWitness      *QueryWitnessResult `json:"native_witness"`
}

type instrumentedReference struct {
	Schema   string `json:"schema"`
	Status   string `json:"status"`
	Code     string `json:"code"`
	Upstream string `json:"upstream_commit"`
	Input    struct {
		Encoding string `json:"encoding"`
		Bytes    uint64 `json:"bytes"`
		SHA256   string `json:"sha256"`
	} `json:"input"`
	Grammar struct {
		LibrarySHA256 string `json:"library_sha256"`
		Symbol        string `json:"symbol"`
	} `json:"grammar"`
	Tree struct {
		Nodes []TreeNode `json:"nodes"`
	} `json:"tree"`
	Query struct {
		ID           string           `json:"id"`
		SHA256       string           `json:"sha256"`
		Patterns     uint32           `json:"patterns"`
		CaptureNames []string         `json:"capture_names"`
		Predicates   []QueryPredicate `json:"predicates"`
		Matches      uint64           `json:"matches"`
		MatchMapping []struct {
			Match     uint32 `json:"match"`
			RuntimeID uint32 `json:"runtime_id"`
		} `json:"match_mapping"`
		Structural []Capture `json:"structural"`
		Evaluated  []Capture `json:"evaluated"`
	} `json:"query"`
	Limits struct {
		Nodes         uint64 `json:"nodes"`
		Captures      uint64 `json:"captures"`
		GroupCaptures uint64 `json:"group_captures"`
		Matches       uint64 `json:"matches"`
		WallMillis    uint64 `json:"wall_ms"`
	} `json:"limits"`
	CursorExhausted     bool `json:"cursor_exhausted"`
	DidExceedMatchLimit bool `json:"did_exceed_match_limit"`
	Complete            bool `json:"complete"`
}

// VerifyQueryReferenceConnection uses only the guarded bytes verified by VerifyEvidence.
// Hash approval binds bytes; it never proves that a stored response was really executed.
func VerifyQueryReferenceConnection(ctx context.Context, req QueryReferenceConnectionRequest) (QueryReferenceConnectionResult, error) {
	r, e := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		res := QueryReferenceConnectionResult{Report: newReport("query reference connection"), ResultSchema: QueryReferenceResultSchema}
		failReport(&res.Report, e)
		return res, e
	}
	defer r.cancel()
	return verifyQueryReferenceConnection(r, req, nil)
}

func verifyQueryReferenceConnection(r *run, req QueryReferenceConnectionRequest, scope *referenceReplayScope) (res QueryReferenceConnectionResult, resultErr error) {
	res = QueryReferenceConnectionResult{Report: newReport("query reference connection"), ResultSchema: QueryReferenceResultSchema}
	defer func() {
		if resultErr != nil {
			knownFailure := scope != nil && res.Assessment == AssessFail
			failReport(&res.Report, asError(resultErr))
			if knownFailure {
				res.Assessment = AssessFail
			}
		}
	}()
	var reg QueryReferenceRegistration
	if e := decodeWitnessDocument("reference-registration", req.Registration, &reg, true, r); e != nil {
		return res, e
	}
	if e := validateReferenceRegistration(reg); e != nil {
		return res, e
	}
	if len(req.WitnessRegistration) == 0 {
		return res, fail(KindInvalidInput, "REFERENCE_WITNESS_REQUIRED", "witness-registration", nil)
	}
	witnessReg, e := parseQueryWitnessRegistration(req.WitnessRegistration, r)
	if e != nil {
		return res, e
	}
	if uint64(len(req.WitnessRegistration)) != reg.WitnessRegistration.Bytes || digestHex(req.WitnessRegistration) != reg.WitnessRegistration.SHA256 {
		return res, fail(KindInvalidInput, "REFERENCE_WITNESS_PIN_MISMATCH", "witness-registration", nil)
	}
	res.Required = len(witnessReg.Required)
	res.RegistrationSHA256 = digestHex(req.Registration)
	res.ExecutionStatus, res.EvidenceMode, res.Assessment = StatusCompleted, ModeReplayedRaw, AssessPass
	add := func(code, path string, bad bool) {
		severity := "warning"
		if bad {
			res.Assessment, severity = AssessFail, "error"
		} else if res.Assessment != AssessFail {
			res.Assessment = AssessBlocked
		}
		if len(res.Findings) < 1000 {
			res.Findings = append(res.Findings, Finding{Severity: severity, Code: code, Path: path, Message: "저장 reference 연결이 승인된 문맥과 일치하지 않거나 필수 근거가 없다"})
		}
	}
	contextBound := true
	if digestHex(req.Evidence.Policy) != reg.EvidencePolicySHA256 {
		contextBound = false
		add("REFERENCE_POLICY_MISMATCH", "policy", true)
	}
	if reg.Native.Identities["platform"] != reg.Reference.Identities["platform"] {
		contextBound = false
		add("REFERENCE_PLATFORM_MISMATCH", "registration", true)
	}
	wanted := map[string]bool{}
	for _, m := range referenceMembers(reg) {
		wanted[m.Path] = true
	}
	snapshot := &evidenceSnapshot{}
	var root *os.Root
	if scope != nil {
		snapshot, root = scope.snapshot, scope.root
		snapshot.approved = scope.pins
	}
	snapshot.wanted, snapshot.wantedNodes = wanted, map[string]bool{reg.Native.ID: true, reg.Reference.ID: true}
	evidence, err := verifyEvidenceInRun(r.caller, req.Evidence, snapshot, r, root)
	if err != nil {
		if scope != nil && evidence.Assessment == AssessFail {
			res.Findings = append(res.Findings, evidence.Findings...)
			res.Assessment = AssessFail
		}
		if e := r.check(); e != nil {
			return res, e
		}
		return res, err
	}
	res.Findings = append(res.Findings, evidence.Findings...)
	if !snapshot.graphBound || snapshot.invalidNodes[reg.Native.ID] || snapshot.invalidNodes[reg.Reference.ID] {
		contextBound = false
	}
	if evidence.Assessment == AssessFail {
		res.Assessment = AssessFail
	} else if evidence.Assessment != AssessPass {
		add("REFERENCE_EVIDENCE_BLOCKED", "evidence", false)
	}
	if e := r.check(); e != nil {
		return res, e
	}
	nodes := map[string]EvidenceNode{}
	nodeCounts := map[string]int{}
	owners := map[string]string{}
	fileCounts := map[string]int{}
	for _, node := range snapshot.nodes {
		nodeCounts[node.ID]++
		nodes[node.ID] = node
		for _, m := range node.Files {
			fileCounts[m.Path]++
			owners[m.Path] = node.ID
		}
	}
	bindNode := func(binding QueryReferenceNode) {
		n, ok := nodes[binding.ID]
		if !ok {
			contextBound = false
			add("REFERENCE_NODE_MISSING", binding.ID, false)
			return
		}
		if nodeCounts[binding.ID] != 1 {
			contextBound = false
			add("REFERENCE_NODE_AMBIGUOUS", binding.ID, true)
		}
		if !reflect.DeepEqual(n.Identities, binding.Identities) {
			contextBound = false
			add("REFERENCE_IDENTITY_MISMATCH", binding.ID, true)
		}
		if n.Assessment == AssessFail {
			add("REFERENCE_NODE_FAILED", binding.ID, true)
		}
		if n.Kind != "run" || n.EvidenceMode != ModeNewRun || n.ExecutionStatus != StatusCompleted || n.Assessment != AssessPass || n.RecordedAssessment != "" {
			contextBound = false
			add("REFERENCE_NODE_INCOMPLETE", binding.ID, false)
		}
	}
	bindNode(reg.Native)
	bindNode(reg.Reference)
	member := func(nodeID string, m ReplayMember) []byte {
		if fileCounts[m.Path] > 1 {
			add("REFERENCE_MEMBER_AMBIGUOUS", m.Path, true)
			return nil
		}
		if owner, declared := owners[m.Path]; declared && owner != nodeID {
			add("REFERENCE_MEMBER_OWNER_MISMATCH", m.Path, true)
			return nil
		}
		n, ok := nodes[nodeID]
		if !ok {
			return nil
		}
		for _, f := range n.Files {
			if f.Path != m.Path {
				continue
			}
			if f != m {
				add("REFERENCE_MEMBER_MISMATCH", m.Path, true)
				return nil
			}
			return snapshot.files[m.Path]
		}
		add("REFERENCE_MEMBER_MISSING", m.Path, false)
		return nil
	}
	witnessBytes := member(reg.Native.ID, reg.WitnessRegistration)
	source, raw, projection := member(reg.Native.ID, reg.Source), member(reg.Native.ID, reg.Raw), member(reg.Native.ID, reg.Projection)
	buildBytes := member(reg.Reference.ID, reg.Build)
	buildReady := false
	if buildBytes != nil {
		var build struct {
			Schema       string `json:"schema"`
			Upstream     string `json:"upstream_commit"`
			Target       string `json:"target"`
			BinarySHA256 string `json:"binary_sha256"`
			Complete     bool   `json:"complete"`
		}
		if e := decodeWitnessDocument("reference-build", buildBytes, &build, false, r); e != nil {
			if e.Kind != KindInvalidInput {
				return res, e
			}
			add("REFERENCE_BUILD_INVALID", reg.Build.Path, true)
		} else {
			target := map[string]string{"windows/amd64": "x86_64-pc-windows-gnu", "linux/amd64": "x86_64-unknown-linux-gnu", "darwin/arm64": "aarch64-apple-darwin"}[reg.Reference.Identities["platform"]]
			if build.Schema != instrumentedReferenceBuildSchema || build.Upstream != instrumentedReferenceCommit || target == "" || build.Target != target || build.BinarySHA256 != reg.BinarySHA256 {
				add("REFERENCE_BUILD_MISMATCH", reg.Build.Path, true)
			} else if !build.Complete {
				add("REFERENCE_BUILD_INCOMPLETE", reg.Build.Path, false)
			} else {
				buildReady = true
			}
		}
	}
	witnessBound := witnessBytes != nil && slices.Equal(witnessBytes, req.WitnessRegistration)
	var versions [][]byte
	if source != nil {
		if !SourceEncodingValid(EncodingUTF8, source) {
			add("REFERENCE_SOURCE_ENCODING_INVALID", reg.Source.Path, true)
		} else {
			versions = [][]byte{source}
		}
	}
	if reg.Source.Bytes != witnessReg.Input.Bytes || reg.Source.SHA256 != witnessReg.Input.SHA256 {
		add("REFERENCE_SOURCE_MEMBER_MISMATCH", reg.Source.Path, true)
	}
	if witnessReg.Encoding != EncodingUTF8 {
		add("REFERENCE_ENCODING_INELIGIBLE", reg.WitnessRegistration.Path, false)
	}
	if witnessReg.Platform != reg.Native.Identities["platform"] || witnessReg.Platform != reg.Reference.Identities["platform"] {
		contextBound = false
		add("REFERENCE_PLATFORM_MISMATCH", "registration", true)
	}
	if witnessBytes != nil && !slices.Equal(witnessBytes, req.WitnessRegistration) {
		add("REFERENCE_WITNESS_BYTES_MISMATCH", reg.WitnessRegistration.Path, true)
	}
	nativeVerified := make([]bool, len(witnessReg.Required))
	witness, witnessErr := verifyQueryWitness(r, QueryWitnessRequest{Registration: req.WitnessRegistration, Source: source, Raw: raw, Projection: projection}, nativeVerified)
	res.NativeWitness = &witness
	if witnessErr != nil {
		return res, witnessErr
	}
	if witness.Assessment == AssessFail {
		add("REFERENCE_NATIVE_WITNESS_FAILED", "native", true)
	} else if witness.Assessment != AssessPass {
		add("REFERENCE_NATIVE_WITNESS_BLOCKED", "native", false)
	}
	if source != nil && uint64(len(source)) == witnessReg.Input.Bytes && digestHex(source) == witnessReg.Input.SHA256 {
		edited, _, editErr := ApplyEdits(witnessReg.Encoding, source, witnessReg.Edits, NativeOperations()["native-query-large"].InputBytes)
		if editErr == nil {
			versions = edited
		}
	}
	if res.Required == 0 {
		add("REFERENCE_REQUIRED_ZERO", "registration", false)
	}
	queries := map[string]QueryWitnessQuery{}
	for _, q := range witnessReg.Queries {
		queries[q.ID] = q
	}
	var native witnessRaw
	if raw != nil {
		if e := decodeWitnessDocument("native-raw", raw, &native, false, r); e != nil {
			if e.Kind != KindInvalidInput {
				return res, e
			}
			add("REFERENCE_NATIVE_RAW_INVALID", reg.Raw.Path, true)
		}
		if native.Protocol != "tsgk-native/r2" || !validID(native.ID) || !native.Complete || !slices.Contains([]string{StatusCompleted, StatusResourceLimit, StatusFailed}, native.Status) || native.Bytes != reg.Source.Bytes {
			add("REFERENCE_NATIVE_RAW_INVALID", reg.Raw.Path, true)
		}
	}
	var saved witnessProjection
	if projection != nil {
		if e := decodeWitnessDocument("native-projection", projection, &saved, false, r); e != nil {
			if e.Kind != KindInvalidInput {
				return res, e
			}
			add("REFERENCE_NATIVE_PROJECTION_INVALID", reg.Projection.Path, true)
		}
		if !validID(saved.ID) || !encodingNames[saved.Encoding] || saved.Input.Bytes != reg.Source.Bytes || saved.Input.SHA256 != reg.Source.SHA256 {
			add("REFERENCE_NATIVE_PROJECTION_INVALID", reg.Projection.Path, true)
		}
		if raw != nil && saved.ID != native.ID {
			add("REFERENCE_NATIVE_CASE_MISMATCH", reg.Projection.Path, true)
		}
	}
	nativeEOF := map[*witnessRawTree]bool{}
	for _, step := range native.Steps {
		for _, tree := range []*witnessRawTree{step.Incremental, step.Fresh} {
			if tree == nil || tree.Status != StatusCompleted || tree.Form == nil || *tree.Form != "full" {
				continue
			}
			valid := len(tree.Nodes) > 0 && len(tree.Nodes[0]) == 10 && tree.Nodes[0][5] >= 0 && uint64(tree.Nodes[0][5]) == step.Bytes
			nativeEOF[tree] = valid
			if !valid {
				add("REFERENCE_NATIVE_TREE_EOF_MISMATCH", reg.Raw.Path, true)
			}
		}
	}
	for _, step := range saved.Steps {
		for _, tree := range []*witnessTreeProjection{step.Incremental, step.Fresh} {
			if tree != nil && tree.Status == StatusCompleted && tree.Form == "full" && tree.Tree != nil && (len(tree.Tree.Nodes) == 0 || uint64(tree.Tree.Nodes[0].EndByte) != step.Bytes) {
				add("REFERENCE_PROJECTION_TREE_EOF_MISMATCH", reg.Projection.Path, true)
			}
		}
	}
	type versionIdentity struct {
		size uint64
		sha  string
	}
	versionPins := map[uint32]versionIdentity{0: {reg.Source.Bytes, reg.Source.SHA256}}
	bindVersion := func(step uint32, size uint64, sha, path string) bool {
		if previous, ok := versionPins[step]; ok && (previous.size != size || previous.sha != sha) {
			add("REFERENCE_SOURCE_VERSION_MISMATCH", path, true)
			return false
		}
		versionPins[step] = versionIdentity{size, sha}
		return true
	}
	for _, s := range native.Steps {
		bindVersion(s.Step, s.Bytes, s.SHA256, reg.Raw.Path)
	}
	for _, s := range saved.Steps {
		bindVersion(s.Step, s.Bytes, s.SHA256, reg.Projection.Path)
	}
	observed := map[string]*instrumentedReference{}
	for _, obs := range reg.Observations {
		if e := r.check(); e != nil {
			return res, e
		}
		observationBound := bindVersion(obs.Step, obs.SourceBytes, obs.SourceSHA256, obs.Response.Path)
		q, queryKnown := queries[obs.Query]
		if !queryKnown || int(obs.Step) > len(witnessReg.Edits) {
			observationBound = false
			add("REFERENCE_OBSERVATION_UNREGISTERED", obs.Response.Path, true)
		}
		if int(obs.Step) < len(versions) && (uint64(len(versions[obs.Step])) != obs.SourceBytes || digestHex(versions[obs.Step]) != obs.SourceSHA256) {
			observationBound = false
			add("REFERENCE_SOURCE_VERSION_MISMATCH", obs.Response.Path, true)
		}
		if obs.Step == 0 && (obs.SourceBytes != witnessReg.Input.Bytes || obs.SourceSHA256 != witnessReg.Input.SHA256) {
			observationBound = false
			add("REFERENCE_SOURCE_VERSION_MISMATCH", obs.Response.Path, true)
		}
		if queryKnown && int(obs.Step) <= len(witnessReg.Edits) {
			size := witnessReg.Input.Bytes
			for _, edit := range witnessReg.Edits[:obs.Step] {
				size = size - uint64(len(edit.Old)) + uint64(len(edit.New))
			}
			if obs.SourceBytes != size {
				observationBound = false
				add("REFERENCE_SOURCE_VERSION_MISMATCH", obs.Response.Path, true)
			}
		}
		data := member(reg.Reference.ID, obs.Response)
		if data == nil {
			continue
		}
		ref, code, complete, e := decodeReferenceResponse(data, obs, reg, q, queryKnown, versions, r)
		if e != nil {
			return res, e
		}
		if code != "" {
			add(code, obs.Response.Path, true)
			continue
		}
		if !complete {
			add("REFERENCE_RESPONSE_INCOMPLETE", obs.Response.Path, false)
			continue
		}
		if queryKnown && q.Metadata != nil && !reflect.DeepEqual(*q.Metadata, QueryWitnessMetadata{Patterns: ref.Query.Patterns, CaptureNames: ref.Query.CaptureNames, Predicates: ref.Query.Predicates}) {
			add("REFERENCE_METADATA_MISMATCH", obs.Response.Path, true)
			continue
		}
		if observationBound {
			observed[referenceKey(obs.Query, obs.Step)] = ref
		}
	}
	for i, tuple := range witnessReg.Required {
		if e := r.check(); e != nil {
			return res, e
		}
		path := fmt.Sprintf("%s/%d/%s", tuple.Query, tuple.Step, tuple.TreeRole)
		ref := observed[referenceKey(tuple.Query, tuple.Step)]
		if ref == nil {
			add("REFERENCE_TUPLE_MISSING", path, false)
			continue
		}
		checkAnchors := func() bool {
			ready := tuple.Structural != nil && tuple.Expected != nil
			if !ready {
				add("REFERENCE_ANCHOR_MISSING", path, false)
			}
			if tuple.Structural != nil && CompareCaptures(tuple.Structural, ref.Query.Structural) != nil || tuple.Expected != nil && CompareCaptures(tuple.Expected, ref.Query.Evaluated) != nil {
				add("REFERENCE_CAPTURE_MISMATCH", path, true)
				return false
			}
			return ready
		}
		identityMismatch := tuple.Structural != nil && referenceAnchorIdentityMismatch(tuple.Structural, ref.Query.Structural) || tuple.Expected != nil && referenceAnchorIdentityMismatch(tuple.Expected, ref.Query.Evaluated)
		if identityMismatch {
			add("REFERENCE_CAPTURE_MISMATCH", path, true)
		}
		var tree *witnessRawTree
		if int(tuple.Step) < len(native.Steps) {
			step := native.Steps[tuple.Step]
			if tuple.TreeRole == "fresh" {
				tree = step.Fresh
			} else {
				tree = step.Incremental
			}
		}
		if tree == nil || tree.Status != StatusCompleted || tree.Form == nil || *tree.Form != "full" {
			checkAnchors()
			add("REFERENCE_NATIVE_FULL_TREE_MISSING", path, false)
			continue
		}
		if !nativeEOF[tree] {
			continue
		}
		var src []byte
		if int(tuple.Step) < len(versions) {
			src = versions[tuple.Step]
		}
		nativeNodes, code := witnessNodes(*tree, witnessReg.Encoding, src, native.Steps[tuple.Step].Bytes, r)
		if code != "" {
			add("REFERENCE_NATIVE_TREE_INVALID", path, true)
			continue
		}
		q := queries[tuple.Query]
		metadata := QueryWitnessMetadata{Patterns: ref.Query.Patterns, CaptureNames: ref.Query.CaptureNames, Predicates: ref.Query.Predicates}
		if q.Metadata == nil {
			add("REFERENCE_METADATA_MISSING", path, false)
		} else if !reflect.DeepEqual(*q.Metadata, metadata) {
			add("REFERENCE_METADATA_MISMATCH", path, true)
			continue
		}
		if CompareTrees(nativeNodes, ref.Tree.Nodes) != nil {
			add("REFERENCE_CST_INELIGIBLE", path, false)
			continue
		}
		if identityMismatch || !checkAnchors() {
			continue
		}
		if contextBound && witnessBound && nativeVerified[i] && buildReady && witnessReg.Encoding == EncodingUTF8 && q.Metadata != nil && tuple.Structural != nil && tuple.Expected != nil {
			res.Verified++
		}
	}
	if res.Verified != res.Required && res.Assessment == AssessPass {
		add("REFERENCE_CONNECTION_INCOMPLETE", "connection", false)
	}
	if e := r.check(); e != nil {
		return res, e
	}
	return res, nil
}

// Before CST equivalence, only query identity and stream cardinality are comparable.
func referenceAnchorIdentityMismatch(expected, actual []Capture) bool {
	if len(expected) != len(actual) {
		return true
	}
	for i, x := range expected {
		y := actual[i]
		if x.Match != y.Match || x.Pattern != y.Pattern || x.Capture != y.Capture || x.Name != y.Name {
			return true
		}
	}
	return false
}

func referenceKey(query string, step uint32) string { return fmt.Sprintf("%s/%d", query, step) }

func referenceMembers(reg QueryReferenceRegistration) []ReplayMember {
	ms := []ReplayMember{reg.WitnessRegistration, reg.Source, reg.Raw, reg.Projection, reg.Build}
	for _, obs := range reg.Observations {
		ms = append(ms, obs.Response)
	}
	return ms
}

func validateReferenceRegistration(reg QueryReferenceRegistration) *Error {
	bad := func() *Error {
		return fail(KindInvalidInput, "REFERENCE_REGISTRATION_INVALID", "reference-registration", nil)
	}
	if reg.Schema != QueryReferenceRegistrationSchema || reg.Context != "full-input-utf8/r1" || !hex64.MatchString(reg.EvidencePolicySHA256) || !hex64.MatchString(reg.BinarySHA256) || !hex64.MatchString(reg.LibrarySHA256) || !ValidLanguageSymbol(reg.Symbol) || reg.Native.ID == reg.Reference.ID || len(reg.Observations) > 1000 {
		return bad()
	}
	for _, n := range []QueryReferenceNode{reg.Native, reg.Reference} {
		if !validID(n.ID) || len(n.Identities) != 11 {
			return bad()
		}
		for _, role := range []string{"source", "tool", "input", "query", "policy", "comparator", "protocol", "workload", "platform", "run", "attempt"} {
			if _, ok := n.Identities[role]; !ok {
				return bad()
			}
		}
	}
	paths, keys := map[string]bool{}, map[string]bool{}
	for _, m := range referenceMembers(reg) {
		if !portable(m.Path, false) || m.Path == "evidence.json" || m.Role == "" || !hex64.MatchString(m.SHA256) || m.Bytes > ReplayOperations()["evidence-replay"].FileBytes || paths[m.Path] {
			return bad()
		}
		paths[m.Path] = true
	}
	for _, obs := range reg.Observations {
		key := referenceKey(obs.Query, obs.Step)
		if !validID(obs.Query) || obs.Step > 4 || obs.SourceBytes > 32*1024*1024 || !hex64.MatchString(obs.SourceSHA256) || keys[key] {
			return bad()
		}
		keys[key] = true
	}
	return nil
}

func decodeReferenceResponse(data []byte, obs QueryReferenceObservation, reg QueryReferenceRegistration, q QueryWitnessQuery, queryKnown bool, versions [][]byte, r *run) (*instrumentedReference, string, bool, *Error) {
	var head struct {
		Schema   string `json:"schema"`
		Status   string `json:"status"`
		Code     string `json:"code"`
		Complete bool   `json:"complete"`
	}
	invalid := func(e *Error) (*instrumentedReference, string, bool, *Error) {
		if e.Kind != KindInvalidInput {
			return nil, "", false, e
		}
		return nil, "REFERENCE_RESPONSE_INVALID", false, nil
	}
	if e := decodeWitnessDocument("reference-response", data, &head, false, r); e != nil {
		return invalid(e)
	}
	if head.Schema != instrumentedReferenceSchema {
		return nil, "REFERENCE_RESPONSE_SCHEMA", false, nil
	}
	if head.Status == StatusFailed && !head.Complete && head.Code != "" {
		if e := decodeWitnessDocument("reference-response", data, &head, true, r); e != nil {
			return invalid(e)
		}
		if !slices.Contains([]string{
			"ABSOLUTE_REGULAR_INPUT_REQUIRED", "CAPTURE_DUPLICATED", "CAPTURE_ID", "CAPTURE_INDEX", "CAPTURE_LIMIT", "CAPTURE_NODE_FIELDS_DIFFER", "CAPTURE_NODE_UNMAPPED",
			"CST_LIMIT", "CST_NODE_ID_DUPLICATE", "IDENTIFIER_INVALID", "INPUT_LIMIT", "INPUT_OPEN", "INPUT_READ", "INPUT_STAT", "LANGUAGE_ABI", "LANGUAGE_NULL", "LANGUAGE_SYMBOL",
			"LIBRARY_CHANGED", "LIBRARY_LOAD", "LIBRARY_SHA256_MISMATCH", "LIBRARY_SHA256_UTF8", "MATCH_GROUP_CHANGED", "MATCH_GROUP_INCOMPLETE", "MATCH_GROUP_LIMIT", "NODE_OBJECT",
			"PARSE_INCOMPLETE", "PARSE_TIME_LIMIT", "PREDICATE_CAPTURE", "PREDICATE_EMPTY", "PREDICATE_OPERANDS", "PREDICATE_OPERATOR", "PREDICATE_OUTSIDE_REFERENCE_SUBSET",
			"PREDICATE_POINTER", "PREDICATE_QUANTIFIER_OUTSIDE_REFERENCE_SUBSET", "PREDICATE_STEP_KIND", "PREDICATE_STEP_LIMIT", "PREDICATE_STRING", "PREDICATE_UNTERMINATED", "PREDICATE_UTF8",
			"QUERY_COMPILE", "QUERY_ID_UTF8", "QUERY_INCOMPLETE", "QUERY_UTF8", "SOURCE_UTF8", "SYMBOL_UTF8", "USAGE_LIBRARY_SYMBOL_SOURCE_QUERY_QUERY_ID_LIBRARY_SHA256",
		}, head.Code) {
			return nil, "REFERENCE_FAILURE_CODE_INVALID", false, nil
		}
		return nil, "", false, nil
	}
	var ref instrumentedReference
	if e := decodeWitnessDocument("reference-response", data, &ref, true, r); e != nil {
		return invalid(e)
	}
	if ref.Status != StatusCompleted || ref.Code != "" || !ref.Complete || !ref.CursorExhausted || ref.DidExceedMatchLimit {
		return nil, "REFERENCE_COMPLETION_INVALID", false, nil
	}
	if int(obs.Step) < len(versions) && !utf8.Valid(versions[obs.Step]) {
		return nil, "REFERENCE_SOURCE_ENCODING_INVALID", false, nil
	}
	if ref.Upstream != instrumentedReferenceCommit || ref.Input.Encoding != EncodingUTF8 || ref.Input.Bytes != obs.SourceBytes || ref.Input.SHA256 != obs.SourceSHA256 || ref.Grammar.LibrarySHA256 != reg.LibrarySHA256 || ref.Grammar.Symbol != reg.Symbol || ref.Query.ID != obs.Query || !hex64.MatchString(ref.Query.SHA256) || queryKnown && ref.Query.SHA256 != digestHex([]byte(q.Source)) {
		return nil, "REFERENCE_CONTEXT_MISMATCH", false, nil
	}
	l := ref.Limits
	if l.Nodes != 100000 || l.Captures != 50000 || l.GroupCaptures != 200000 || l.Matches != 100000 || l.WallMillis != 3000 || uint64(len(ref.Tree.Nodes)) > l.Nodes || uint64(len(ref.Query.Structural)) > l.Captures || uint64(len(ref.Query.Evaluated)) > l.Captures || ref.Query.Matches > l.Matches || len(ref.Query.MatchMapping) != int(ref.Query.Matches) {
		return nil, "REFERENCE_LIMITS_INVALID", false, nil
	}
	if ref.Tree.Nodes == nil || ref.Query.CaptureNames == nil || ref.Query.Predicates == nil || ref.Query.Structural == nil || ref.Query.Evaluated == nil || ref.Query.MatchMapping == nil || ValidateTree(ref.Tree.Nodes, ref.Input.Bytes) != nil {
		return nil, "REFERENCE_TREE_INVALID", false, nil
	}
	// A full parse ends at EOF; its root may start after legal leading padding.
	if len(ref.Tree.Nodes) == 0 || uint64(ref.Tree.Nodes[0].EndByte) != ref.Input.Bytes {
		return nil, "REFERENCE_TREE_EOF_MISMATCH", false, nil
	}
	if querypolicy.Support(ref.Query.Patterns, ref.Query.Predicates, EncodingUTF8) != "" {
		return nil, "REFERENCE_METADATA_INVALID", false, nil
	}
	names := map[string]bool{}
	for _, name := range ref.Query.CaptureNames {
		if name == "" || names[name] {
			return nil, "REFERENCE_METADATA_INVALID", false, nil
		}
		names[name] = true
	}
	for _, predicate := range ref.Query.Predicates {
		for _, step := range predicate.Steps {
			if step.Kind == "capture" && !names[step.Value] {
				return nil, "REFERENCE_METADATA_INVALID", false, nil
			}
		}
	}
	for i, n := range ref.Tree.Nodes {
		if i%1024 == 0 {
			if e := r.check(); e != nil {
				return nil, "", false, e
			}
		}
		if n.Type == "" {
			return nil, "REFERENCE_TREE_INVALID", false, nil
		}
	}
	if int(obs.Step) < len(versions) && witnessPoints(ref.Tree.Nodes, EncodingUTF8, versions[obs.Step], r) != "" {
		return nil, "REFERENCE_TREE_POINTS", false, nil
	}
	if !querywire.MatchAccounting(ref.Query.Matches, len(ref.Query.Structural), func(i int) (int64, int64) { c := ref.Query.Structural[i]; return int64(c.Match), int64(c.Pattern) }) {
		return nil, "REFERENCE_MATCH_ACCOUNTING", false, nil
	}
	ids := map[uint32]bool{}
	for i, m := range ref.Query.MatchMapping {
		if m.Match != uint32(i) || ids[m.RuntimeID] {
			return nil, "REFERENCE_MATCH_MAPPING", false, nil
		}
		ids[m.RuntimeID] = true
	}
	for _, stream := range [][]Capture{ref.Query.Structural, ref.Query.Evaluated} {
		for i, c := range stream {
			if i%1024 == 0 {
				if e := r.check(); e != nil {
					return nil, "", false, e
				}
			}
			if uint64(c.Match) >= ref.Query.Matches || c.Pattern >= ref.Query.Patterns || uint64(c.Capture) >= uint64(len(ref.Query.CaptureNames)) || c.Name != ref.Query.CaptureNames[c.Capture] || !witnessCaptureNode(c, ref.Tree.Nodes) {
				return nil, "REFERENCE_CAPTURE_INVALID", false, nil
			}
		}
	}
	// Evaluated output is a whole-match subsequence; preserve occurrences and interleaving.
	kept := map[uint32]bool{}
	for _, c := range ref.Query.Evaluated {
		kept[c.Match] = true
	}
	predicated := map[uint32]bool{}
	for _, p := range ref.Query.Predicates {
		predicated[p.Pattern] = true
	}
	for _, c := range ref.Query.Structural {
		if !predicated[c.Pattern] && !kept[c.Match] {
			return nil, "REFERENCE_EVALUATION_MISMATCH", false, nil
		}
	}
	filtered := make([]Capture, 0, len(ref.Query.Evaluated))
	for _, c := range ref.Query.Structural {
		if kept[c.Match] {
			filtered = append(filtered, c)
		}
	}
	if CompareCaptures(filtered, ref.Query.Evaluated) != nil {
		return nil, "REFERENCE_EVALUATED_MAPPING", false, nil
	}
	if len(ref.Query.Predicates) == 0 {
		if CompareCaptures(ref.Query.Structural, ref.Query.Evaluated) != nil {
			return nil, "REFERENCE_EVALUATION_MISMATCH", false, nil
		}
	} else if int(obs.Step) < len(versions) {
		views := make([]querypolicy.Capture, 0, len(ref.Query.Structural))
		for _, c := range ref.Query.Structural {
			views = append(views, querypolicy.Capture{Match: c.Match, Pattern: c.Pattern, Name: c.Name, StartByte: c.StartByte, EndByte: c.EndByte})
		}
		indices, reason, err := querypolicy.FilterChecked(func() error {
			if e := r.check(); e != nil {
				return e
			}
			return nil
		}, ref.Query.Patterns, ref.Query.Predicates, views, EncodingUTF8, versions[obs.Step])
		if err != nil {
			return nil, "", false, asError(err)
		}
		if reason != "" {
			return nil, "REFERENCE_EVALUATION_MISMATCH", false, nil
		}
		expected := make([]Capture, 0, len(indices))
		for _, index := range indices {
			expected = append(expected, ref.Query.Structural[index])
		}
		if CompareCaptures(expected, ref.Query.Evaluated) != nil {
			return nil, "REFERENCE_EVALUATION_MISMATCH", false, nil
		}
	}
	if !slices.IsSortedFunc(ref.Query.Predicates, func(a, b QueryPredicate) int {
		if a.Pattern < b.Pattern {
			return -1
		}
		if a.Pattern > b.Pattern {
			return 1
		}
		return 0
	}) {
		return nil, "REFERENCE_METADATA_INVALID", false, nil
	}
	return &ref, "", true, nil
}
