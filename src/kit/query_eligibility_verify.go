package kit

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

type eligibilitySession struct {
	r                                 *run
	b                                 *eligibilityBudget
	res                               *QueryReferenceEligibilityResult
	approval                          eligibilityApproval
	reg                               eligibilityRegistration
	witness                           eligibilityWitness
	catalogue                         eligibilityCatalogue
	snapshot                          *evidenceSnapshot
	cohorts                           map[string]eligibilityCohort
	provenance                        map[string]eligibilityProvenance
	invocations                       map[string]*eligibilityInvocation
	catalogueInvocations              map[string]*eligibilityInvocation
	tools                             map[string]eligibilityTool
	owned                             map[string]eligibilityMember
	nodes                             map[string]EvidenceNode
	owners                            map[string]string
	ready                             map[string]bool
	build, units, control, assertions map[string]string
	tuples                            map[int]string
	outputs                           map[string]*eligibilityOutput
	activeTuple                       *int
	failures                          uint64
	buildIdentity                     *eligibilityBuildIdentity
	bound                             bool
}

func (s *eligibilitySession) finding(code, at string, bad bool) {
	severity := "warning"
	if bad {
		s.failures++
		if s.activeTuple != nil {
			s.tuples[*s.activeTuple] = "failed"
		}
		s.res.Assessment = AssessFail
		severity = "error"
	} else if s.res.Assessment != AssessFail {
		s.res.Assessment = AssessBlocked
	}
	if len(s.res.Findings) < 1000 {
		s.res.Findings = append(s.res.Findings, Finding{Severity: severity, Code: code, Path: at, Message: "승인된 eligibility 문맥과 모순되거나 필수 저장 근거가 없다"})
	}
}

func eligibilityString(x string) bool { return x != "" && len(x) <= 1024 && utf8.ValidString(x) }
func eligibilityRevision(x string) bool {
	if len(x) != 40 && len(x) != 64 {
		return false
	}
	for _, c := range x {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func verifyQueryReferenceEligibility(ctx context.Context, req QueryReferenceEligibilityRequest) (res QueryReferenceEligibilityResult, resultErr error) {
	res = QueryReferenceEligibilityResult{Report: newReport("query reference eligibility"), ResultSchema: QueryReferenceEligibilityResultSchema,
		Build: QueryEligibilityCounts{Required: 9, Unprocessed: 9}, Unit: QueryEligibilityCounts{Required: 4, Unprocessed: 4}, Control: QueryEligibilityCounts{Required: 53, Unprocessed: 53}, Assertion: QueryEligibilityCounts{Required: 36, Unprocessed: 36}}
	r, e := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		failReport(&res.Report, e)
		return res, e
	}
	defer r.cancel()
	b := &eligibilityBudget{r: r}
	r.eligibility = b
	s := &eligibilitySession{r: r, b: b, res: &res, bound: true, cohorts: map[string]eligibilityCohort{}, provenance: map[string]eligibilityProvenance{}, invocations: map[string]*eligibilityInvocation{}, catalogueInvocations: map[string]*eligibilityInvocation{}, tools: map[string]eligibilityTool{}, owned: map[string]eligibilityMember{}, nodes: map[string]EvidenceNode{}, owners: map[string]string{}, ready: map[string]bool{}, build: map[string]string{}, units: map[string]string{}, control: map[string]string{}, assertions: map[string]string{}, tuples: map[int]string{}, outputs: map[string]*eligibilityOutput{}}
	res.ExecutionStatus, res.EvidenceMode, res.Assessment = StatusCompleted, ModeReplayedRaw, AssessPass
	defer func() {
		res.BytesRead, res.Work, res.LogicalBytes = r.totalRead, b.work, b.logical
		if resultErr != nil {
			known := res.Assessment == AssessFail
			failReport(&res.Report, asError(resultErr))
			if known {
				res.Assessment = AssessFail
			}
			res.Complete = false
		}
		s.counts(res.Complete)
	}()
	docs := []struct {
		name string
		data []byte
	}{{"approval", req.Approval}, {"registration", req.Registration}, {"witness-registration", req.WitnessRegistration}, {"policy", req.Evidence.Policy}}
	for _, doc := range docs {
		if uint64(len(doc.data)) > MaxDocumentBytes {
			return res, fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", doc.name, nil)
		}
		if uint64(len(doc.data)) > ReplayOperations()["evidence-replay"].TotalBytes-r.totalRead {
			return res, fail(KindResourceLimit, "TOTAL_BYTES_LIMIT", doc.name, nil)
		}
		r.totalRead += uint64(len(doc.data))
	}
	if len(req.Approval) == 0 {
		s.finding("ELIGIBILITY_APPROVAL_MISSING", "approval", false)
		return res, nil
	}
	if e := b.decode("approval", req.Approval, &s.approval, true); e != nil {
		return res, e
	}
	if e := b.decode("registration", req.Registration, &s.reg, true); e != nil {
		return res, e
	}
	if e := b.decode("witness-registration", req.WitnessRegistration, &s.witness, true); e != nil {
		return res, e
	}
	if _, e := validateQueryWitnessRegistration(s.witness.QueryWitnessRegistration, queryEligibilityWitnessSchema); e != nil {
		return res, e
	}
	res.Tuple.Required = uint64(len(s.witness.Required))
	res.Tuple.Unprocessed = res.Tuple.Required
	if e := b.charge(4, uint64(len(req.Approval)+len(req.Registration)+len(req.WitnessRegistration)+len(eligibilityCatalogueJSON)), "caller-digests"); e != nil {
		return res, e
	}
	res.ApprovalSHA256, res.RegistrationSHA256, res.WitnessSHA256 = digestHex(req.Approval), digestHex(req.Registration), digestHex(req.WitnessRegistration)
	res.CatalogueSHA256 = digestHex([]byte(eligibilityCatalogueJSON))
	if e := b.decode("catalogue", []byte(eligibilityCatalogueJSON), &s.catalogue, true); e != nil {
		return res, e
	}
	if e := s.validate(req); e != nil {
		return res, e
	}
	wanted, approved, wantedNodes := map[string]bool{}, map[string]ReplayMember{}, map[string]bool{}
	approved[s.reg.Graph.Path] = s.reg.Graph
	for path, pin := range s.owned {
		wanted[path] = true
		approved[path] = pin.Member
		wantedNodes[pin.NodeID] = true
	}
	s.snapshot = &evidenceSnapshot{wanted: wanted, wantedNodes: wantedNodes, approved: approved}
	evidence, err := verifyEvidenceInRun(ctx, req.Evidence, s.snapshot, r, nil)
	res.Evidence = &evidence
	for _, identity := range evidence.Identities {
		if identity.Role == "evidence" {
			res.GraphSHA256 = identity.SHA256
		}
	}
	if evidence.Assessment == AssessFail {
		s.finding("ELIGIBILITY_EVIDENCE_FAILED", "evidence", true)
	}
	if err != nil {
		return res, err
	}
	s.bound = s.bound && s.snapshot.graphBound
	for _, node := range s.snapshot.nodes {
		s.nodes[node.ID] = node
		for _, member := range node.Files {
			s.owners[member.Path] = node.ID
		}
	}
	for _, inv := range s.reg.Invocations {
		s.bindInvocation(inv)
	}
	if e := s.verifyBuild(); e != nil {
		return res, e
	}
	if e := s.verifyControls(); e != nil {
		return res, e
	}
	s.verifyAssertions()
	if e := s.verifyNative(); e != nil {
		return res, e
	}
	if res.Tuple.Required == 0 {
		s.finding("ELIGIBILITY_REQUIRED_ZERO", "witness-registration", false)
	}
	if e := r.check(); e != nil {
		return res, e
	}
	res.Complete = true
	s.counts(true)
	if res.Build.Verified != 9 || res.Unit.Verified != 4 || res.Control.Verified != 53 || res.Assertion.Verified != 36 || res.Tuple.Verified != res.Tuple.Required {
		s.finding("ELIGIBILITY_INCOMPLETE", "counts", false)
	}
	if e := b.charge(20000, 16<<20, "result-encoding"); e != nil {
		return res, e
	}
	if e := sealOutput(res, ReplayOperations()["evidence-replay"].OutputBytes); e != nil {
		return res, e
	}
	return res, nil
}

func (s *eligibilitySession) counts(complete bool) {
	count := func(required uint64, states []string) QueryEligibilityCounts {
		x := QueryEligibilityCounts{Required: required}
		for _, state := range states {
			switch state {
			case "verified":
				x.Verified++
			case "failed":
				x.Failed++
			case "blocked":
				x.Blocked++
			}
		}
		left := required - x.Verified - x.Failed - x.Blocked
		if complete {
			x.Blocked += left
		} else {
			x.Unprocessed = left
		}
		return x
	}
	values := func(ids []string, states map[string]string) []string {
		a := make([]string, 0, len(ids))
		for _, id := range ids {
			a = append(a, states[id])
		}
		return a
	}
	builds, controls, assertions := []string{}, []string{}, []string{}
	for _, v := range s.catalogue.Builds {
		builds = append(builds, v.ID)
	}
	for _, v := range s.catalogue.Controls {
		controls = append(controls, v.ID)
	}
	for _, v := range s.catalogue.Assertions {
		assertions = append(assertions, v.ID)
	}
	s.res.Build = count(9, values(builds, s.build))
	s.res.Unit = count(4, values(s.catalogue.Units, s.units))
	s.res.Control = count(53, values(controls, s.control))
	s.res.Assertion = count(36, values(assertions, s.assertions))
	tupleStates := make([]string, 0, len(s.tuples))
	for _, state := range s.tuples {
		tupleStates = append(tupleStates, state)
	}
	s.res.Tuple = count(s.res.Tuple.Required, tupleStates)
}

func (s *eligibilitySession) validate(req QueryReferenceEligibilityRequest) *Error {
	a, g := s.approval, s.reg
	bad := func(at string) *Error { return fail(KindInvalidInput, "ELIGIBILITY_REGISTRATION_INVALID", at, nil) }
	if a.Schema != QueryReferenceEligibilityApprovalSchema || g.Schema != QueryReferenceEligibilityRegistrationSchema || g.Context != "full-input-utf8/r2" {
		return fail(KindUnsupported, "SCHEMA_UNSUPPORTED", "eligibility", nil)
	}
	if !eligibilityRevision(a.Candidate.SourceRevision) || !slices.Contains([]string{"windows/amd64", "linux/amd64", "darwin/arm64"}, a.Candidate.Platform) || !eligibilityString(a.Candidate.Run) || a.Candidate.Attempt == 0 || a.Candidate.Attempt > maxJSONInteger {
		return bad("candidate")
	}
	for _, h := range []string{a.RegistrationSHA256, a.WitnessSHA256, a.EvidencePolicySHA256, a.CatalogueSHA256, g.CatalogueSHA256} {
		if !hex64.MatchString(h) {
			return bad("approval-digest")
		}
	}
	if a.RegistrationSHA256 != s.res.RegistrationSHA256 || a.WitnessSHA256 != s.res.WitnessSHA256 || a.CatalogueSHA256 != s.res.CatalogueSHA256 || g.CatalogueSHA256 != s.res.CatalogueSHA256 || a.EvidencePolicySHA256 != digestHex(req.Evidence.Policy) {
		s.bound = false
		s.finding("ELIGIBILITY_APPROVAL_MISMATCH", "approval", true)
	}
	if g.Graph.Path != "evidence.json" || g.Graph.Role != "evidence-graph" || g.Graph.Bytes > MaxDocumentBytes || !hex64.MatchString(g.Graph.SHA256) {
		return bad("graph")
	}
	if len(a.Cohorts) > 3 || len(a.Provenance) > 3 {
		return bad("cohorts")
	}
	purposes := map[string]bool{}
	for _, c := range a.Cohorts {
		if !validID(c.ID) || s.cohorts[c.ID].ID != "" || !slices.Contains([]string{"build", "control", "current"}, c.Purpose) || purposes[c.Purpose] || !eligibilityRevision(c.SourceRevision) || !slices.Contains([]string{"windows/amd64", "linux/amd64", "darwin/arm64"}, c.Platform) || !eligibilityString(c.Run) || c.Attempt == 0 || c.Attempt > maxJSONInteger || !validID(c.ProvenanceID) {
			return bad("cohort/" + c.ID)
		}
		purposes[c.Purpose] = true
		s.cohorts[c.ID] = c
		if c.Purpose == "current" && (c.SourceRevision != a.Candidate.SourceRevision || c.Platform != a.Candidate.Platform || c.Run != a.Candidate.Run || c.Attempt != a.Candidate.Attempt) {
			s.bound = false
			s.finding("ELIGIBILITY_CURRENT_MISMATCH", c.ID, true)
		}
	}
	for _, p := range a.Provenance {
		if !validID(p.ID) || s.provenance[p.ID].ID != "" || !eligibilityRevision(p.SourceRevision) || !slices.Contains([]string{"reviewed-ci", "approved-local"}, p.Kind) || len(p.InvocationIDs) == 0 {
			return bad("provenance/" + p.ID)
		}
		for _, h := range []string{p.RunnerSHA256, p.ControllerSourcesSHA256, p.ReviewedContextSHA256, p.ReviewRecordSHA256, p.EvidenceSHA256} {
			if !hex64.MatchString(h) {
				return bad("provenance-digest")
			}
		}
		switch p.Kind {
		case "reviewed-ci":
			if !eligibilityString(p.Repository) || !portable(p.WorkflowPath, false) || !eligibilityRevision(p.WorkflowRevision) || !eligibilityString(p.RunID) || p.Attempt == 0 || p.Attempt > maxJSONInteger || !eligibilityString(p.ArtifactID) || !hex64.MatchString(p.ArtifactSHA256) || p.SessionID != "" {
				return bad("ci-provenance")
			}
		case "approved-local":
			if !eligibilityString(p.SessionID) || p.Repository != "" || p.WorkflowPath != "" || p.WorkflowRevision != "" || p.RunID != "" || p.Attempt != 0 || p.ArtifactID != "" || p.ArtifactSHA256 != "" {
				return bad("local-provenance")
			}
		}
		ids := map[string]bool{}
		for _, id := range p.InvocationIDs {
			if !validID(id) || ids[id] {
				return bad("provenance-invocation")
			}
			ids[id] = true
		}
		s.provenance[p.ID] = p
	}
	foldedMembers := map[string]bool{}
	for _, pin := range g.OwnedMembers {
		if !validID(pin.NodeID) || !portable(pin.Member.Path, false) || pin.Member.Path == "evidence.json" || !validID(pin.Member.Role) || !hex64.MatchString(pin.Member.SHA256) || pin.Member.Bytes > maxJSONInteger {
			return bad("owned-member")
		}
		if _, ok := s.owned[pin.Member.Path]; ok {
			return bad("owned-member-duplicate")
		}
		folded := strings.ToLower(pin.Member.Path)
		if foldedMembers[folded] {
			return bad("owned-member-case-collision")
		}
		foldedMembers[folded] = true
		s.owned[pin.Member.Path] = pin
	}
	for i := range s.reg.Invocations {
		inv := &s.reg.Invocations[i]
		c, ok := s.cohorts[inv.CohortID]
		if !validID(inv.ID) || s.invocations[inv.ID] != nil || !ok || !validID(inv.CatalogueID) || !validID(inv.Node.ID) || len(inv.Node.Identities) != len(EvidenceRoles) || !slices.Contains([]string{"REFERENCE_PROCESS", "NATIVE_DRIVER"}, inv.Kind) {
			return bad("invocation")
		}
		for _, role := range EvidenceRoles {
			if !eligibilityString(inv.Node.Identities[role]) {
				return bad("invocation-identity")
			}
		}
		key := c.Purpose + "/" + inv.CatalogueID
		if s.catalogueInvocations[key] != nil {
			return bad("catalogue-duplicate")
		}
		if c.Purpose == "current" && inv.CatalogueID != inv.ID {
			return bad("current-invocation")
		}
		if c.Purpose != "current" && inv.Kind != "REFERENCE_PROCESS" {
			return bad("invocation-kind")
		}
		known := c.Purpose == "current"
		if c.Purpose == "build" {
			for _, r := range s.catalogue.Builds {
				known = known || r.ID == inv.CatalogueID
			}
		}
		if c.Purpose == "control" {
			for _, r := range s.catalogue.Controls {
				known = known || r.ID == inv.CatalogueID
			}
		}
		if !known {
			return bad("catalogue-unknown")
		}
		if inv.Executable != nil && !eligibilityLiteral(inv.ExecutablePath) || inv.Executable == nil && inv.ExecutablePath != "" {
			return bad("executable-path")
		}
		s.invocations[inv.ID] = inv
		s.catalogueInvocations[key] = inv
		for name, input := range inv.Inputs {
			if !validID(name) {
				return bad("input-name")
			}
			if e := s.validateInput(input); e != nil {
				return e
			}
		}
		if inv.Kind == "REFERENCE_PROCESS" {
			if inv.Request == nil || inv.Receipt == nil || inv.Response != nil || inv.Process != nil || inv.ExitAttestation == nil || inv.ExitAttestation.Basis != "reviewed-inline/r1" || inv.ExitAttestation.ProvenanceID != c.ProvenanceID {
				return bad("reference-shape")
			}
		} else if c.Purpose != "current" || inv.Request == nil || inv.Receipt != nil || inv.Response == nil || inv.Process == nil || inv.Executable == nil || inv.ToolID != "" || inv.ExitAttestation != nil {
			return bad("native-shape")
		}
	}
	referenced := map[string]bool{}
	claimedInvocations := map[string]bool{}
	for _, c := range a.Cohorts {
		p, ok := s.provenance[c.ProvenanceID]
		if !ok {
			s.finding("ELIGIBILITY_PROVENANCE_MISSING", c.ID, false)
			continue
		}
		referenced[p.ID] = true
		if p.SourceRevision != c.SourceRevision || p.EvidenceSHA256 != g.Graph.SHA256 || p.Kind == "reviewed-ci" && (p.RunID != c.Run || p.Attempt != c.Attempt) {
			s.bound = false
			s.finding("ELIGIBILITY_PROVENANCE_MISMATCH", c.ID, true)
		}
		for _, id := range p.InvocationIDs {
			inv := s.invocations[id]
			if inv == nil {
				return bad("provenance-unknown-invocation")
			}
			ic := s.cohorts[inv.CohortID]
			if ic.ProvenanceID != p.ID {
				return bad("provenance-cross-cohort")
			}
			claimedInvocations[id] = true
		}
	}
	for id := range s.provenance {
		if !referenced[id] {
			return bad("provenance-unreferenced")
		}
	}
	for _, inv := range s.invocations {
		if !claimedInvocations[inv.ID] {
			s.finding("ELIGIBILITY_INVOCATION_UNAPPROVED", inv.ID, false)
		}
	}
	for _, tool := range g.Tools {
		c, ok := s.cohorts[tool.CohortID]
		if !validID(tool.ID) || s.tools[tool.ID].ID != "" || !ok || !slices.Contains([]string{"cargo", "rustc", "cc", "ar", "git", "supervisor", "python"}, tool.ToolName) || !eligibilityString(tool.PathLiteral) || !hex64.MatchString(tool.SubjectSHA256) || tool.ProvenanceID != c.ProvenanceID {
			return bad("tool")
		}
		s.tools[tool.ID] = tool
	}
	requiredTuples := map[string]bool{}
	for _, t := range s.witness.Required {
		requiredTuples[fmt.Sprintf("%s/%d/%s", t.Query, t.Step, t.TreeRole)] = true
	}
	seen := map[string]bool{}
	for _, o := range g.Observations {
		key := fmt.Sprintf("%s/%d/%s", o.Query, o.Step, o.TreeRole)
		if !validID(o.Query) || !validID(o.QueryID) || !validID(o.InvocationID) || !hex64.MatchString(o.SourceSHA256) || !hex64.MatchString(o.SHA256) || !slices.Contains([]string{"incremental", "fresh"}, o.TreeRole) || seen[key] || o.Step > 4 || o.Step == 0 && o.TreeRole == "fresh" {
			return bad("observation")
		}
		if !requiredTuples[key] {
			return bad("observation-unreferenced")
		}
		seen[key] = true
	}
	if s.witness.Encoding != EncodingUTF8 || s.witness.Platform != a.Candidate.Platform {
		s.bound = false
		s.finding("ELIGIBILITY_WITNESS_CONTEXT", "witness-registration", true)
	}
	return nil
}

func (s *eligibilitySession) validateInput(x eligibilityInput) *Error {
	invalid := func() *Error { return fail(KindInvalidInput, "ELIGIBILITY_INPUT_UNION", "registration", nil) }
	switch x.Kind {
	case "PINNED":
		if x.MemberPin == nil || !slices.Contains([]string{"PRESERVED_ORIGINAL", "APPROVED_EXPECTED_BYTES"}, x.ContentBasis) {
			return invalid()
		}
		if (x.ArgvIndex == nil) != (x.PathLiteral == "") {
			return invalid()
		}
	case "EXPECTED_ABSENT":
		if x.MemberPin != nil || x.ContentBasis != "" || x.ArgvIndex == nil || !eligibilityString(x.PathLiteral) {
			return invalid()
		}
	case "NOT_APPLICABLE":
		if x.MemberPin != nil || x.ContentBasis != "" || x.ArgvIndex != nil || x.PathLiteral != "" {
			return invalid()
		}
	default:
		return invalid()
	}
	if x.MemberPin != nil {
		p := x.MemberPin
		if !validID(p.NodeID) || !portable(p.Member.Path, false) || !validID(p.Member.Role) || !hex64.MatchString(p.Member.SHA256) || p.Member.Bytes > maxJSONInteger {
			return invalid()
		}
	}
	if x.ArgvIndex != nil && *x.ArgvIndex > maxJSONInteger {
		return invalid()
	}
	return nil
}

func (s *eligibilitySession) member(pin *eligibilityMember) ([]byte, bool) {
	if pin == nil {
		return nil, false
	}
	if e := s.b.charge(1, uint64(len(pin.Member.Path)+len(pin.Member.Role)+len(pin.Member.SHA256)), pin.Member.Path); e != nil {
		return nil, false
	}
	expected, ok := s.owned[pin.Member.Path]
	if !ok || expected != *pin || s.owners[pin.Member.Path] != pin.NodeID {
		s.finding("ELIGIBILITY_MEMBER_OWNER", pin.Member.Path, true)
		return nil, false
	}
	actual, ok := s.snapshot.verifiedMembers[pin.Member.Path]
	if !ok || actual != pin.Member {
		s.finding("ELIGIBILITY_MEMBER_UNVERIFIED", pin.Member.Path, true)
		return nil, false
	}
	data, ok := s.snapshot.files[pin.Member.Path]
	if !ok {
		s.finding("ELIGIBILITY_MEMBER_MISSING", pin.Member.Path, true)
		return nil, false
	}
	return data, true
}

func (s *eligibilitySession) bindInvocation(inv eligibilityInvocation) {
	c := s.cohorts[inv.CohortID]
	node, ok := s.nodes[inv.Node.ID]
	p, approved := s.provenance[c.ProvenanceID]
	ready := s.bound && ok && !s.snapshot.invalidNodes[inv.Node.ID] && approved && slices.Contains(p.InvocationIDs, inv.ID)
	if !ok {
		s.finding("ELIGIBILITY_NODE_MISSING", inv.ID, false)
	} else {
		if !reflect.DeepEqual(node.Identities, inv.Node.Identities) || node.Identities["source"] != c.SourceRevision || node.Identities["platform"] != c.Platform || node.Identities["run"] != inv.ID || node.Identities["attempt"] != strconv.FormatUint(c.Attempt, 10) {
			s.finding("ELIGIBILITY_NODE_BINDING", inv.ID, true)
			ready = false
		}
		if node.Kind != "run" || node.EvidenceMode != ModeNewRun || node.RecordedAssessment != "" {
			s.finding("ELIGIBILITY_NODE_MODE", inv.ID, true)
			ready = false
		}
		negative := false
		status := StatusCompleted
		if c.Purpose == "control" {
			for _, rule := range s.catalogue.Controls {
				if rule.ID == inv.CatalogueID {
					negative = rule.Negative
					status = rule.ExecutionStatus
				}
			}
		}
		if node.ExecutionStatus != status || negative && node.Assessment != AssessNotAssessed || !negative && node.Assessment != AssessPass {
			ready = false
			s.finding("ELIGIBILITY_NODE_AXIS", inv.ID, true)
		}
	}
	for _, pin := range []*eligibilityMember{inv.Request, inv.Receipt, inv.Response, inv.Process} {
		if pin != nil {
			if _, ok := s.member(pin); !ok {
				ready = false
			}
		}
		if pin != nil && pin.NodeID != inv.Node.ID {
			s.finding("ELIGIBILITY_INVOCATION_OWNER", inv.ID, true)
			ready = false
		}
	}
	for _, input := range inv.Inputs {
		if input.MemberPin != nil {
			if _, ok := s.member(input.MemberPin); !ok {
				ready = false
			}
		}
	}
	s.ready[inv.ID] = ready
}

func (s *eligibilitySession) verifyAssertions() {
	for _, a := range s.catalogue.Assertions {
		state := "verified"
		for _, id := range a.RequiredInvocations {
			switch s.control[id] {
			case "failed":
				state = "failed"
			case "verified":
			default:
				if state != "failed" {
					state = "blocked"
				}
			}
		}
		s.assertions[a.ID] = state
	}
}
