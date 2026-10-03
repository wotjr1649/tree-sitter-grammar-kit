package kit

import (
	"context"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
)

// Evidence graph schemas (Session 07).
const (
	EvidenceSchema       = "tsgk-evidence/r1"
	EvidencePolicySchema = "tsgk-evidence-policy/r1"
	EvidenceResultSchema = "tsgk-evidence-result/r1"

	// CarryUnchangedDependency is the only adopted carry-forward relation: the carried
	// result's every dependency identity equals its origin's.
	CarryUnchangedDependency = "unchanged-dependency-r1"
)

// EvidenceRoles are the identity roles every node binds, as separate fields.
var EvidenceRoles = []string{"source", "tool", "input", "query", "policy", "comparator", "protocol", "workload", "platform", "run", "attempt"}

// dependencyRoles are the roles a carried result must share with its origin and a replay
// with its subject's raw (run and attempt name the operation itself).
var dependencyRoles = []string{"source", "tool", "input", "query", "policy", "comparator", "protocol", "workload", "platform"}

// EvidenceNode is one result in the graph: a new run, a replay of a subject run's raw, a
// carried result or a historical record not recomputed.
type EvidenceNode struct {
	ID                 string            `json:"id"`
	Kind               string            `json:"kind"` // run | replay | carry | record
	ExecutionStatus    string            `json:"execution_status"`
	EvidenceMode       string            `json:"evidence_mode"`
	Assessment         string            `json:"assessment"`
	RecordedAssessment string            `json:"recorded_assessment"`
	Identities         map[string]string `json:"identities"`
	Files              []ReplayMember    `json:"files"`
	Refs               []EvidenceRef     `json:"refs"`
}

// EvidenceRef links a node to another: subject (replay → its subject), carries (carry →
// origin), supersedes (a later run → the earlier result it does not relabel).
type EvidenceRef struct {
	Relation string `json:"relation"`
	Node     string `json:"node"`
}

// EvidenceResult is the tsgk-evidence-result/r1 document.
type EvidenceResult struct {
	Report
	ResultSchema string         `json:"result_schema"`
	PolicyID     string         `json:"policy_id"`
	Nodes        int            `json:"nodes"`
	Files        int            `json:"files"`
	Modes        map[string]int `json:"modes"`
	Explanation  []string       `json:"explanation"`
}

// EvidenceRequest names the evidence root (holding evidence.json and its files) and the
// caller-trusted policy bytes.
type EvidenceRequest struct {
	Root   string
	Policy []byte
}

type evidencePolicy struct {
	ID             string `json:"id"`
	EvidenceSHA256 string `json:"evidence_sha256"`
	Required       []struct {
		Node       string            `json:"node"`
		Kind       string            `json:"kind"`
		Identities map[string]string `json:"identities"`
	} `json:"required"`
	CarryForward []struct {
		Node         string `json:"node"`
		Origin       string `json:"origin"`
		Relation     string `json:"relation"`
		AuthorizedBy string `json:"authorized_by"`
	} `json:"carry_forward"`
	Eligibility *struct {
		Nodes []string `json:"nodes"`
		Modes []string `json:"modes"`
	} `json:"eligibility"`
}

var nodeKinds = map[string]string{"run": ModeNewRun, "replay": ModeReplayedRaw, "carry": ModeCarriedForward, "record": ModeRecordedNotRecomputed}

// strictDoc validates a document with the shared strict decoder (duplicate and unknown
// handling, exact integers) before typed decoding.
func strictDoc(doc string, data []byte, schema string) *Error {
	v, e := decodeStrict(doc, data, MaxDocumentBytes)
	if e != nil {
		return e
	}
	s := jvMember(v, "schema")
	if s == nil || s.kind != '"' {
		return fail(KindInvalidInput, "JSON_MISSING_FIELD", doc+"#/schema", nil)
	}
	if s.s != schema {
		return fail(KindUnsupported, "SCHEMA_UNSUPPORTED", doc+"#/schema", nil)
	}
	return nil
}

func decodeTyped(doc string, data []byte, v any) *Error {
	if err := jsonv2.Unmarshal(data, v, jsonv2.RejectUnknownMembers(true)); err != nil {
		return fail(KindInvalidInput, "JSON_SHAPE", doc, err)
	}
	return nil
}

// VerifyEvidence validates an evidence graph offline against a caller-trusted policy:
// the graph document is the one the policy anchors, files are exactly the listed ones
// with their sizes and sha256, references resolve without cycles, each node's three axes
// fit its kind, a replay keeps its subject's dependencies and run apart, a carried result
// is authorized by the adopted unchanged-dependency relation only, superseding results do
// not relabel earlier runs, required nodes carry their exact identities and eligibility
// accepts only the listed evidence modes. Integrity is not authenticity: hashes bind
// bytes, not a publisher.
func VerifyEvidence(ctx context.Context, req EvidenceRequest) (EvidenceResult, error) {
	res := EvidenceResult{Report: newReport("evidence verify"), ResultSchema: EvidenceResultSchema, Modes: map[string]int{}, Explanation: []string{}}
	bad := func(e *Error) (EvidenceResult, error) {
		failReport(&res.Report, e)
		res.Explanation = append(res.Explanation, "evidence 검증을 끝내지 못했다: "+e.Code)
		return res, e
	}
	pol, e := parseEvidencePolicy(req.Policy)
	if e != nil {
		return bad(e)
	}
	res.PolicyID = pol.ID
	res.Identities = append(res.Identities, IdentityRef{Role: "evidence-policy", Schema: EvidencePolicySchema, SHA256: digestHex(req.Policy)})
	lim := ReplayOperations()["evidence-replay"]
	r, e := startRun(ctx, lim.Wall)
	if e != nil {
		return bad(e)
	}
	defer r.cancel()
	root, _, e := openRoot(req.Root)
	if e != nil {
		return bad(e)
	}
	defer root.Close()
	x := &replayEnv{r: r, g: newGuard(root), lim: lim, files: map[string]fs.FileInfo{}, verified: map[string]bool{}, covered: map[string]bool{}, actual: map[string]string{}}
	var count uint64
	if e := x.walk(".", &count); e != nil {
		return bad(e)
	}
	info := x.files["evidence.json"]
	if info == nil {
		return bad(fail(KindInvalidInput, "EVIDENCE_MISSING", "evidence.json", nil))
	}
	st, e := x.g.readFile(r, "evidence.json", info, MaxDocumentBytes, lim.TotalBytes, MaxDocumentBytes)
	if e != nil {
		return bad(e)
	}
	data := st.content
	res.Identities = append(res.Identities, IdentityRef{Role: "evidence", Schema: EvidenceSchema, SHA256: st.sha256})
	if e := strictDoc("evidence", data, EvidenceSchema); e != nil {
		return bad(e)
	}
	var doc struct {
		Schema string         `json:"schema"`
		Nodes  []EvidenceNode `json:"nodes"`
	}
	if e := decodeTyped("evidence", data, &doc); e != nil {
		return bad(e)
	}
	add := func(code, path, msg string) { x.finding(code, path, msg) }
	if st.sha256 != pol.EvidenceSHA256 {
		add("EVIDENCE_ANCHOR_MISMATCH", "evidence.json", "evidence 문서가 policy가 고정한 sha256과 다르다")
	}
	nodes := map[string]*EvidenceNode{}
	files := map[string]string{"evidence.json": ""}
	for i := range doc.Nodes {
		n := &doc.Nodes[i]
		if !validID(n.ID) {
			add("NODE_ID_INVALID", n.ID, "node id 형식이 틀렸다")
			continue
		}
		if nodes[n.ID] != nil {
			add("NODE_DUPLICATE", n.ID, "같은 node id가 두 번 있다")
			continue
		}
		nodes[n.ID] = n
		res.Modes[n.EvidenceMode]++
		mode, known := nodeKinds[n.Kind]
		switch {
		case !known:
			add("NODE_KIND_UNSUPPORTED", n.ID, "등록되지 않은 node 종류다")
		case n.EvidenceMode != mode || !executionStatuses[n.ExecutionStatus] || !assessmentValues[n.Assessment]:
			add("AXIS_INVALID", n.ID, "node 종류와 실행 상태·evidence mode·판정이 맞지 않는다")
		case n.Kind == "record" && (n.Assessment != AssessUnresolved || !assessmentValues[n.RecordedAssessment] || n.RecordedAssessment == AssessUnresolved):
			add("AXIS_INVALID", n.ID, "다시 계산하지 않은 기록은 현재 판정 UNRESOLVED와 원 판정을 따로 가진다")
		case n.Kind == "carry" && n.ExecutionStatus != StatusNotRun:
			add("AXIS_INVALID", n.ID, "승계 결과의 현재 실행은 NOT_RUN이다")
		case n.Kind != "record" && n.RecordedAssessment != "":
			add("AXIS_INVALID", n.ID, "recorded_assessment는 기록 node에만 있다")
		case n.ExecutionStatus != StatusCompleted && n.Assessment == AssessPass && n.Kind != "carry":
			add("AXIS_INVALID", n.ID, "완료되지 않은 실행은 PASS가 아니다")
		}
		for _, role := range sortedKeys(n.Identities) {
			if !slices.Contains(EvidenceRoles, role) {
				add("IDENTITY_ROLE_UNKNOWN", n.ID, "등록되지 않은 identity 역할이다: "+role)
			}
		}
		for _, role := range EvidenceRoles {
			if _, ok := n.Identities[role]; !ok {
				add("IDENTITY_ROLE_MISSING", n.ID, "identity 역할이 빠졌다: "+role)
			}
		}
		for _, f := range n.Files {
			if !portable(f.Path, false) || len(f.SHA256) != 64 {
				add("FILE_INVALID", n.ID, "파일 경로·hash 형식이 틀렸다")
				continue
			}
			if _, dup := files[f.Path]; dup {
				add("FILE_DUPLICATE", f.Path, "같은 파일을 두 node가 쓰거나 두 번 썼다")
				continue
			}
			files[f.Path] = n.ID
			if _, e := x.read(f.Path, f.Bytes, f.SHA256); e != nil {
				if e.Kind == KindCancelled || e.Kind == KindIO {
					return bad(e)
				}
				add("FILE_"+e.Code, f.Path, "파일이 없거나 bytes·sha256이 다르다")
			}
		}
	}
	for p := range x.files {
		if _, ok := files[p]; !ok {
			add("FILE_UNLISTED", p, "graph에 없는 파일이 있다")
		}
	}
	// references, relations and cycles
	allowed := map[string]map[string][]string{
		"run":    {"supersedes": {"run", "record", "carry"}},
		"record": {"supersedes": {"run", "record"}},
		"replay": {"subject": {"run", "record"}},
		"carry":  {"carries": {"run"}},
	}
	for _, id := range sortedKeys(nodes) {
		n := nodes[id]
		counts := map[string]int{}
		for _, ref := range n.Refs {
			t := nodes[ref.Node]
			switch {
			case t == nil:
				add("REF_UNRESOLVED", id, "참조한 node가 없다: "+ref.Node)
			case !slices.Contains(allowed[n.Kind][ref.Relation], t.Kind):
				add("REF_RELATION_INVALID", id, fmt.Sprintf("%s는 %s 관계로 %s를 참조할 수 없다", n.Kind, ref.Relation, t.Kind))
			default:
				counts[ref.Relation]++
			}
		}
		if n.Kind == "replay" && counts["subject"] != 1 {
			add("REPLAY_SUBJECT_REQUIRED", id, "replay는 subject를 정확히 하나 참조한다")
		}
		if n.Kind == "carry" && counts["carries"] != 1 {
			add("CARRY_ORIGIN_REQUIRED", id, "승계는 원 결과를 정확히 하나 참조한다")
		}
	}
	if cyc := findCycle(nodes); cyc != "" {
		add("GRAPH_CYCLE", cyc, "참조가 순환한다")
	}
	sameRun := func(a, b *EvidenceNode) bool {
		return a.Identities["run"] == b.Identities["run"] && a.Identities["attempt"] == b.Identities["attempt"] && a.Identities["platform"] == b.Identities["platform"]
	}
	ids := sortedKeys(nodes)
	for i, a := range ids {
		for _, b := range ids[i+1:] {
			na, nb := nodes[a], nodes[b]
			if na.Kind == "run" && nb.Kind == "run" && sameRun(na, nb) {
				add("RUN_IDENTITY_DUPLICATE", a+","+b, "같은 run·attempt·platform이 두 결과로 기록됐다(relabel)")
			}
		}
	}
	for _, id := range ids {
		n := nodes[id]
		for _, ref := range n.Refs {
			t := nodes[ref.Node]
			if t == nil {
				continue
			}
			switch ref.Relation {
			case "supersedes":
				if sameRun(n, t) {
					add("RUN_RELABELED", id, "새 결과가 이전 결과와 같은 run identity로 그 결과를 대체한다")
				}
			case "subject":
				if n.Identities["run"] == t.Identities["run"] && n.Identities["attempt"] == t.Identities["attempt"] {
					add("REPLAY_RELABELS_SUBJECT", id, "replay가 subject run과 같은 실행 identity를 쓴다")
				}
				for _, role := range []string{"source", "input", "workload", "platform"} {
					if n.Identities[role] != t.Identities[role] {
						add("REPLAY_SUBJECT_MISMATCH", id, "replay와 subject의 identity가 다르다: "+role)
					}
				}
				x.checkReplayFile(n, t, add)
			case "carries":
				x.checkCarry(n, t, pol, add)
			}
		}
		if n.Kind == "carry" {
			authorized := false
			for _, rule := range pol.CarryForward {
				authorized = authorized || rule.Node == id
			}
			if !authorized {
				add("CARRY_FORWARD_UNAUTHORIZED", id, "승계를 허가한 policy 규칙이 없다(기본 거부)")
			}
		}
	}
	for _, req := range pol.Required {
		n := nodes[req.Node]
		if n == nil || n.Kind != req.Kind {
			add("REQUIRED_NODE_MISSING", req.Node, "필수 node가 없거나 종류가 다르다")
			continue
		}
		for _, role := range sortedKeys(req.Identities) {
			if n.Identities[role] != req.Identities[role] {
				add("IDENTITY_MISMATCH", req.Node, "필수 node의 identity가 policy와 다르다(stale 또는 혼합): "+role)
			}
		}
	}
	if el := pol.Eligibility; el != nil {
		for _, id := range el.Nodes {
			n := nodes[id]
			if n == nil {
				add("ELIGIBILITY_NODE_MISSING", id, "자격 판정 대상 node가 없다")
			} else if !slices.Contains(el.Modes, n.EvidenceMode) {
				add("ELIGIBILITY_REJECTED", id, "이 자격은 "+fmt.Sprint(el.Modes)+" evidence만 받는다: "+n.EvidenceMode)
			}
		}
	}
	res.Nodes, res.Files = len(nodes), len(files)-1
	res.Findings = append(res.Findings, x.findings...)
	res.Assessment = AssessPass
	if len(x.findings) > 0 {
		res.Assessment = AssessFail
	}
	res.Coverage.Requested = []string{"anchor", "files", "graph", "axes", "replay", "carry-forward", "supersedes", "required", "eligibility"}
	res.Coverage.Observed = slices.Clone(res.Coverage.Requested)
	res.Coverage.Unsupported = []string{"authenticity"}
	res.Explanation = append(res.Explanation, fmt.Sprintf("node %d개, 파일 %d개를 검사했다. finding %d개 → %s.", res.Nodes, res.Files, len(res.Findings), res.Assessment),
		"hash 일치는 무결성일 뿐 게시자 인증이 아니다(authenticity는 지원하지 않음).")
	if e := sealOutput(res, lim.OutputBytes); e != nil {
		return bad(e)
	}
	if e := r.check(); e != nil {
		return bad(e)
	}
	return res, nil
}

// parseEvidencePolicy decodes a tsgk-evidence-policy/r1 document: the shared strict
// decoder first (duplicate names, exact integers), then the typed shape with unknown members
// rejected.
func parseEvidencePolicy(data []byte) (evidencePolicy, *Error) {
	if e := strictDoc("policy", data, EvidencePolicySchema); e != nil {
		return evidencePolicy{}, e
	}
	var pd struct {
		Schema         string `json:"schema"`
		evidencePolicy `json:",inline"`
	}
	if e := decodeTyped("policy", data, &pd); e != nil {
		return evidencePolicy{}, e
	}
	if !validID(pd.ID) || len(pd.EvidenceSHA256) != 64 {
		return evidencePolicy{}, fail(KindInvalidInput, "POLICY_VALUE_INVALID", "policy", nil)
	}
	return pd.evidencePolicy, nil
}

// checkCarry applies the adopted relation: authorized for this node and origin, every
// dependency identity unchanged and non-empty where the origin binds one, the origin a
// completed new run, the assessment unchanged.
func (x *replayEnv) checkCarry(n, origin *EvidenceNode, pol evidencePolicy, add func(code, path, msg string)) {
	for _, rule := range pol.CarryForward {
		if rule.Node != n.ID {
			continue
		}
		if rule.Origin != origin.ID || rule.Relation != CarryUnchangedDependency || rule.AuthorizedBy == "" {
			add("CARRY_RELATION_UNSUPPORTED", n.ID, "승계 규칙의 원 결과·관계·허가가 채택 관계와 다르다")
		}
	}
	for _, role := range dependencyRoles {
		if n.Identities[role] != origin.Identities[role] {
			add("CARRY_DEPENDENCY_CHANGED", n.ID, "원 결과와 dependency identity가 다르다: "+role)
		}
	}
	if origin.ExecutionStatus != StatusCompleted || origin.EvidenceMode != ModeNewRun {
		add("CARRY_ORIGIN_INCOMPLETE", n.ID, "원 결과가 완료된 새 실행이 아니다")
	}
	if n.Assessment != origin.Assessment {
		add("CARRY_ASSESSMENT_CHANGED", n.ID, "승계가 원 결과의 판정을 바꿨다")
	}
}

// checkReplayFile reads a replay node's replay-result file, when it lists one, and binds
// its evidence mode, assessment and subject run to the graph.
func (x *replayEnv) checkReplayFile(n, subject *EvidenceNode, add func(code, path, msg string)) {
	for _, f := range n.Files {
		if f.Role != "replay-result" {
			continue
		}
		data, e := x.read(f.Path, f.Bytes, f.SHA256)
		if e != nil {
			return // already a FILE_ finding
		}
		var rr struct {
			ResultSchema string `json:"result_schema"`
			EvidenceMode string `json:"evidence_mode"`
			Assessment   string `json:"assessment"`
			Subject      struct {
				Run     string `json:"run"`
				Attempt uint64 `json:"attempt"`
			} `json:"subject"`
		}
		if err := jsonv2.Unmarshal(data, &rr); err != nil || rr.ResultSchema != ReplayResultSchema {
			add("REPLAY_RESULT_INVALID", f.Path, "replay 결과 문서가 tsgk-replay-result/r1이 아니다")
			continue
		}
		if rr.EvidenceMode != n.EvidenceMode || rr.Assessment != n.Assessment {
			add("REPLAY_RESULT_MISMATCH", n.ID, "replay 결과의 evidence mode·판정이 graph와 다르다")
		}
		if rr.Subject.Run != subject.Identities["run"] || strconv.FormatUint(rr.Subject.Attempt, 10) != subject.Identities["attempt"] {
			add("REPLAY_RESULT_MISMATCH", n.ID, "replay 결과의 subject run이 graph의 subject와 다르다")
		}
	}
}

// findCycle returns a node on a reference cycle, or "".
func findCycle(nodes map[string]*EvidenceNode) string {
	state := map[string]int{}
	var visit func(id string) string
	visit = func(id string) string {
		switch state[id] {
		case 1:
			return id
		case 2:
			return ""
		}
		state[id] = 1
		if n := nodes[id]; n != nil {
			for _, r := range n.Refs {
				if nodes[r.Node] != nil {
					if c := visit(r.Node); c != "" {
						return c
					}
				}
			}
		}
		state[id] = 2
		return ""
	}
	for _, id := range sortedKeys(nodes) {
		if c := visit(id); c != "" {
			return c
		}
	}
	return ""
}
