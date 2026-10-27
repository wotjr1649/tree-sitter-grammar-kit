package kit

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"unicode/utf8"
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
	return verifyEvidence(ctx, req, nil)
}

// Requested bytes come from the same guarded pass that hashes every graph member.
type evidenceSnapshot struct {
	observed          map[string]fs.FileInfo
	directories       map[string]fs.FileInfo
	wanted            map[string]bool
	wantedNodes       map[string]bool
	files             map[string][]byte
	nodes             []EvidenceNode
	graphBound        bool
	invalidNodes      map[string]bool
	approved          map[string]ReplayMember
	verifiedMembers   map[string]ReplayMember
	walked            map[string]fs.FileInfo // the caller's walk of the same files; nil when none
	walkedDirectories map[string]fs.FileInfo
}

func verifyEvidence(ctx context.Context, req EvidenceRequest, snapshot *evidenceSnapshot) (EvidenceResult, error) {
	return verifyEvidenceInRun(ctx, req, snapshot, nil, nil)
}

// Borrowed state keeps nested verification under the caller's root and budgets.
func verifyEvidenceInRun(ctx context.Context, req EvidenceRequest, snapshot *evidenceSnapshot, sharedRun *run, heldRoot *os.Root) (EvidenceResult, error) {
	if snapshot != nil {
		snapshot.files, snapshot.nodes = map[string][]byte{}, nil
		snapshot.observed = nil
		snapshot.directories = nil
		snapshot.graphBound, snapshot.invalidNodes = false, map[string]bool{}
		snapshot.verifiedMembers = map[string]ReplayMember{}
	}
	res := EvidenceResult{Report: newReport("evidence verify"), ResultSchema: EvidenceResultSchema, Modes: map[string]int{}, Explanation: []string{}}
	var x *replayEnv
	findingsCopied := false
	bad := func(e *Error) (EvidenceResult, error) {
		known := x != nil && len(x.findings) > 0
		if known && !findingsCopied {
			res.Findings = append(res.Findings, x.findings...)
			findingsCopied = true
		}
		failReport(&res.Report, e)
		if known {
			res.Assessment = AssessFail
		}
		if len(res.Findings) > 1000 {
			last := res.Findings[len(res.Findings)-1]
			res.Findings = res.Findings[:1000]
			res.Findings[999] = last
		}
		res.Explanation = append(res.Explanation, "evidence 검증을 끝내지 못했다: "+e.Code)
		return res, e
	}

	if sharedRun != nil {
		if e := sharedRun.eligibility.reserveJSON("policy", req.Policy, 3); e != nil {
			return bad(e)
		}
	}
	pol, e := parseEvidencePolicy(req.Policy)
	if e != nil {
		return bad(e)
	}
	res.PolicyID = pol.ID
	res.Identities = append(res.Identities, IdentityRef{Role: "evidence-policy", Schema: EvidencePolicySchema, SHA256: digestHex(req.Policy)})
	lim := ReplayOperations()["evidence-replay"]
	if testHookReplayLimits != nil {
		lim = testHookReplayLimits(lim)
	}
	r := sharedRun
	if r == nil {
		r, e = startRun(ctx, lim.Wall)
		if e != nil {
			return bad(e)
		}
		defer r.cancel()
	}
	root := heldRoot
	if root == nil {
		root, _, e = openRoot(req.Root)
		if e != nil {
			return bad(e)
		}
		defer root.Close()
	}
	x = &replayEnv{r: r, g: newGuard(root), lim: lim, files: map[string]fs.FileInfo{}, verified: map[string]bool{}, covered: map[string]bool{}, actual: map[string]string{}}
	if snapshot != nil {
		x.directories = map[string]fs.FileInfo{}
	}
	var count uint64
	if e := x.walk(".", &count); e != nil {
		return bad(e)
	}
	if snapshot != nil {
		snapshot.observed = x.files
		snapshot.directories = x.directories
	}
	if snapshot != nil && snapshot.walked != nil {
		// a file swapped between the caller's walk and this one must not inherit its verification
		for p, inner := range x.files {
			outer := snapshot.walked[p]
			if outer == nil || !os.SameFile(outer, inner) || outer.Size() != inner.Size() || !outer.ModTime().Equal(inner.ModTime()) {
				return bad(fail(KindIO, "SOURCE_CHANGED", p, nil))
			}
		}
	}
	if snapshot != nil && snapshot.walkedDirectories != nil {
		if len(x.directories) != len(snapshot.walkedDirectories) {
			return bad(fail(KindIO, "SOURCE_CHANGED", ".", nil))
		}
		for path, inner := range x.directories {
			if e := r.eligibility.charge(uint64(len(path))+1, 0, "snapshot-directory"); e != nil {
				return bad(e)
			}
			outer := snapshot.walkedDirectories[path]
			if outer == nil || !os.SameFile(outer, inner) {
				return bad(fail(KindIO, "SOURCE_CHANGED", path, nil))
			}
		}
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
	if e := r.eligibility.reserveJSON("evidence", data, 3); e != nil {
		return bad(e)
	}
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
	if snapshot != nil {
		snapshot.nodes = doc.Nodes
	}
	if e := r.eligibility.reserveGraph(doc.Nodes, pol); e != nil {
		return bad(e)
	}
	add := func(code, path, msg string) { x.finding(code, path, msg) }
	markNode := func(id string) {
		if snapshot != nil && snapshot.wantedNodes[id] {
			snapshot.invalidNodes[id] = true
		}
	}
	nodeBad := func(code, id, msg string) {
		markNode(id)
		add(code, id, msg)
	}
	if snapshot != nil {
		snapshot.graphBound = st.sha256 == pol.EvidenceSHA256
		graph := ReplayMember{Path: "evidence.json", Bytes: st.size, SHA256: st.sha256}
		if snapshot.approved != nil && !sameMemberBytes(snapshot.approved[graph.Path], graph) {
			snapshot.graphBound = false
			add("EVIDENCE_APPROVAL_MISMATCH", graph.Path, "graph bytes가 외부 replay 등록과 다르다")
		} else {
			snapshot.verifiedMembers[graph.Path] = graph
		}
	}
	if st.sha256 != pol.EvidenceSHA256 {
		add("EVIDENCE_ANCHOR_MISMATCH", "evidence.json", "evidence 문서가 policy가 고정한 sha256과 다르다")
	}
	nodes := map[string]*EvidenceNode{}
	files := map[string]string{"evidence.json": ""}
	kept := map[string][]byte{}
	for i := range doc.Nodes {
		n := &doc.Nodes[i]
		if !validID(n.ID) {
			nodeBad("NODE_ID_INVALID", n.ID, "node id 형식이 틀렸다")
			continue
		}
		if nodes[n.ID] != nil {
			nodeBad("NODE_DUPLICATE", n.ID, "같은 node id가 두 번 있다")
			continue
		}
		nodes[n.ID] = n
		res.Modes[n.EvidenceMode]++
		mode, known := nodeKinds[n.Kind]
		switch {
		case !known:
			nodeBad("NODE_KIND_UNSUPPORTED", n.ID, "등록되지 않은 node 종류다")
		case n.EvidenceMode != mode || !executionStatuses[n.ExecutionStatus] || !assessmentValues[n.Assessment]:
			nodeBad("AXIS_INVALID", n.ID, "node 종류와 실행 상태·evidence mode·판정이 맞지 않는다")
		case n.Kind == "record" && (n.Assessment != AssessUnresolved || !assessmentValues[n.RecordedAssessment] || n.RecordedAssessment == AssessUnresolved):
			nodeBad("AXIS_INVALID", n.ID, "다시 계산하지 않은 기록은 현재 판정 UNRESOLVED와 원 판정을 따로 가진다")
		case n.Kind == "carry" && n.ExecutionStatus != StatusNotRun:
			nodeBad("AXIS_INVALID", n.ID, "승계 결과의 현재 실행은 NOT_RUN이다")
		case n.Kind != "record" && n.RecordedAssessment != "":
			nodeBad("AXIS_INVALID", n.ID, "recorded_assessment는 기록 node에만 있다")
		case n.ExecutionStatus != StatusCompleted && n.Assessment == AssessPass && n.Kind != "carry":
			nodeBad("AXIS_INVALID", n.ID, "완료되지 않은 실행은 PASS가 아니다")
		}
		for _, role := range sortedKeys(n.Identities) {
			if !slices.Contains(EvidenceRoles, role) {
				nodeBad("IDENTITY_ROLE_UNKNOWN", n.ID, "등록되지 않은 identity 역할이다: "+role)
			}
		}
		for _, role := range EvidenceRoles {
			if _, ok := n.Identities[role]; !ok {
				nodeBad("IDENTITY_ROLE_MISSING", n.ID, "identity 역할이 빠졌다: "+role)
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
			if snapshot != nil && snapshot.approved != nil && !sameMemberBytes(snapshot.approved[f.Path], f) {
				add("FILE_APPROVAL_MISMATCH", f.Path, "graph member가 외부 replay 등록과 다르다")
				continue
			}
			var e *Error
			if f.Role == "replay-result" {
				// read once and kept for the replay binding below
				var b []byte
				if b, e = x.read(f.Path, f.Bytes, f.SHA256); e == nil {
					kept[f.Path] = b
					if snapshot != nil && snapshot.wanted[f.Path] {
						if b == nil {
							b = []byte{}
						}
						snapshot.files[f.Path] = b
					}
				}
			} else if snapshot != nil && snapshot.wanted[f.Path] {
				var b []byte
				if b, e = x.verifyFileBytes(f.Path, f.Bytes, f.SHA256, true); e == nil {
					if b == nil {
						b = []byte{}
					}
					snapshot.files[f.Path] = b
				}
			} else {
				e = x.verifyFile(f.Path, f.Bytes, f.SHA256)
			}
			if e != nil {
				if e.Kind == KindCancelled || e.Kind == KindIO || e.Kind == KindResourceLimit {
					return bad(e) // a file beyond the limits is a limit, not damaged evidence
				}
				add("FILE_"+e.Code, f.Path, "파일이 없거나 bytes·sha256이 다르다")
			} else if snapshot != nil {
				snapshot.verifiedMembers[f.Path] = f
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
				nodeBad("REF_UNRESOLVED", id, "참조한 node가 없다: "+ref.Node)
			case !slices.Contains(allowed[n.Kind][ref.Relation], t.Kind):
				nodeBad("REF_RELATION_INVALID", id, fmt.Sprintf("%s는 %s 관계로 %s를 참조할 수 없다", n.Kind, ref.Relation, t.Kind))
			default:
				counts[ref.Relation]++
			}
		}
		if n.Kind == "replay" && counts["subject"] != 1 {
			nodeBad("REPLAY_SUBJECT_REQUIRED", id, "replay는 subject를 정확히 하나 참조한다")
		}
		if n.Kind == "carry" && counts["carries"] != 1 {
			nodeBad("CARRY_ORIGIN_REQUIRED", id, "승계는 원 결과를 정확히 하나 참조한다")
		}
	}
	if cyc := findCycle(nodes); cyc != "" {
		add("GRAPH_CYCLE", cyc, "참조가 순환한다")
		if snapshot != nil {
			for id := range snapshot.wantedNodes {
				if findCycleFrom(nodes, []string{id}) != "" {
					markNode(id)
				}
			}
		}
	}
	sameRun := func(a, b *EvidenceNode) bool {
		return a.Identities["run"] == b.Identities["run"] && a.Identities["attempt"] == b.Identities["attempt"] && a.Identities["platform"] == b.Identities["platform"]
	}
	ids := sortedKeys(nodes)
	for i, a := range ids {
		for _, b := range ids[i+1:] {
			na, nb := nodes[a], nodes[b]
			// a run and a historical record of the same run identity are one execution
			// labelled twice, as are two runs
			if na.Kind != "replay" && na.Kind != "carry" && nb.Kind != "replay" && nb.Kind != "carry" && sameRun(na, nb) {
				markNode(a)
				markNode(b)
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
					nodeBad("RUN_RELABELED", id, "새 결과가 이전 결과와 같은 run identity로 그 결과를 대체한다")
				}
			case "subject":
				if n.Identities["run"] == t.Identities["run"] && n.Identities["attempt"] == t.Identities["attempt"] {
					nodeBad("REPLAY_RELABELS_SUBJECT", id, "replay가 subject run과 같은 실행 identity를 쓴다")
				}
				for _, role := range []string{"source", "input", "workload", "platform"} {
					if n.Identities[role] != t.Identities[role] {
						nodeBad("REPLAY_SUBJECT_MISMATCH", id, "replay와 subject의 identity가 다르다: "+role)
					}
				}
				if e := x.checkReplayFile(n, t, kept, add); e != nil {
					return bad(e)
				}
			case "carries":
				x.checkCarry(n, t, pol, nodeBad)
			}
		}
		if n.Kind == "carry" {
			authorized := false
			for _, rule := range pol.CarryForward {
				authorized = authorized || rule.Node == id
			}
			if !authorized {
				nodeBad("CARRY_FORWARD_UNAUTHORIZED", id, "승계를 허가한 policy 규칙이 없다(기본 거부)")
			}
		}
	}
	for _, req := range pol.Required {
		n := nodes[req.Node]
		if n == nil || n.Kind != req.Kind {
			nodeBad("REQUIRED_NODE_MISSING", req.Node, "필수 node가 없거나 종류가 다르다")
			continue
		}
		for _, role := range sortedKeys(req.Identities) {
			if n.Identities[role] != req.Identities[role] {
				nodeBad("IDENTITY_MISMATCH", req.Node, "필수 node의 identity가 policy와 다르다(stale 또는 혼합): "+role)
			}
		}
	}
	if el := pol.Eligibility; el != nil {
		for _, id := range el.Nodes {
			n := nodes[id]
			if n == nil {
				nodeBad("ELIGIBILITY_NODE_MISSING", id, "자격 판정 대상 node가 없다")
			} else if !slices.Contains(el.Modes, n.EvidenceMode) {
				nodeBad("ELIGIBILITY_REJECTED", id, "이 자격은 "+fmt.Sprint(el.Modes)+" evidence만 받는다: "+n.EvidenceMode)
			}
		}
	}
	res.Nodes, res.Files = len(nodes), len(files)-1
	res.Findings = append(res.Findings, x.findings...)
	findingsCopied = true
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
func (x *replayEnv) checkReplayFile(n, subject *EvidenceNode, kept map[string][]byte, add func(code, path, msg string)) *Error {
	for _, f := range n.Files {
		if f.Role != "replay-result" {
			continue
		}
		data, ok := kept[f.Path]
		if !ok {
			continue // not verified: already a FILE_ finding
		}
		var rr struct {
			ResultSchema string `json:"result_schema"`
			EvidenceMode string `json:"evidence_mode"`
			Assessment   string `json:"assessment"`
			Reducer      struct {
				ID string `json:"id"`
			} `json:"reducer"`
			QueryReference jsontext.Value `json:"query_reference"`
			QuerySemantics jsontext.Value `json:"query_semantics"`
			Subject        struct {
				Run      string `json:"run"`
				Attempt  uint64 `json:"attempt"`
				Platform string `json:"platform"`
			} `json:"subject"`
		}
		if x.r != nil {
			if e := x.r.check(); e != nil {
				return e
			}
			if b := x.r.eligibility; b != nil {
				if e := b.charge(uint64(len(data))+1, 0, "replay-result-probe"); e != nil {
					return e
				}
			}
		}
		selection, e := replayResultRevision(data, x.r)
		if e != nil {
			if e.Kind == KindCancelled || e.Kind == KindResourceLimit || e.Kind == KindIO {
				return e
			}
			add("REPLAY_RESULT_INVALID", f.Path, "replay 결과 문서가 올바르지 않다")
			continue
		}
		valid := false
		if selection == ReplaySemanticsResultSchema {
			full, e := decodeSemanticReplayResult(data, x.r)
			if e != nil && (e.Kind == KindCancelled || e.Kind == KindResourceLimit || e.Kind == KindIO) {
				return e
			}
			valid = e == nil && validSemanticReplayResult(full)
			if valid && full.ExecutionStatus == StatusCompleted && full.EvidenceValid && (full.Subject.ExecutionStatus != subject.ExecutionStatus || full.Subject.Assessment != subject.Assessment || full.Subject.EvidenceMode != subject.EvidenceMode) {
				valid = false
			}
			rr.ResultSchema, rr.EvidenceMode, rr.Assessment = full.ResultSchema, full.EvidenceMode, full.Assessment
			rr.Subject.Run, rr.Subject.Attempt, rr.Subject.Platform = full.Subject.Run, full.Subject.Attempt, full.Subject.Platform
		} else {
			if x.r != nil {
				if e := x.r.eligibility.reserveLegacyReplayResult(data); e != nil {
					if e.Kind == KindCancelled || e.Kind == KindResourceLimit || e.Kind == KindIO {
						return e
					}
					add("REPLAY_RESULT_INVALID", f.Path, "replay 결과 문서가 올바르지 않다")
					continue
				}
			}
			if err := jsonv2.Unmarshal(data, &rr); err != nil {
				add("REPLAY_RESULT_INVALID", f.Path, "replay 결과 문서가 올바르지 않다")
				continue
			}
			referenceAbsent := len(rr.QueryReference) == 0 || bytes.Equal(bytes.TrimSpace(rr.QueryReference), []byte("null"))
			valid = rr.ResultSchema == ReplayResultSchema && referenceAbsent && len(rr.QuerySemantics) == 0 && rr.Reducer.ID != "query-reference-connection-r1" && rr.Reducer.ID != "oracle-set-r4"
			if rr.ResultSchema == ReplayReferenceResultSchema {
				var full ReplayResult
				valid = len(rr.QuerySemantics) == 0 && decodeTyped("replay-result", data, &full) == nil && validReferenceReplayResult(full)
			}
		}

		if !valid {
			add("REPLAY_RESULT_INVALID", f.Path, "replay 결과 revision과 payload가 일치하지 않는다")
			continue
		}
		if rr.EvidenceMode != n.EvidenceMode || rr.Assessment != n.Assessment {
			add("REPLAY_RESULT_MISMATCH", n.ID, "replay 결과의 evidence mode·판정이 graph와 다르다")
		}
		if rr.Subject.Run != subject.Identities["run"] || strconv.FormatUint(rr.Subject.Attempt, 10) != subject.Identities["attempt"] || rr.Subject.Platform == "" || rr.Subject.Platform != subject.Identities["platform"] {
			add("REPLAY_RESULT_MISMATCH", n.ID, "replay 결과의 subject run·attempt·platform이 graph의 subject와 다르다")
		}
	}
	return nil
}

// This selector scans delimiters and bounded schema strings only; the selected decoder validates JSON.
func replayResultRevision(data []byte, r *run) (string, *Error) {
	return selectJSONRevision(data, r, "replay-result", "result_schema", ReplaySemanticsResultSchema, []string{ReplayResultSchema, ReplayReferenceResultSchema, ReplaySemanticsResultSchema})
}

func selectJSONRevision(data []byte, r *run, doc, name, early string, allowed []string) (selected string, resultErr *Error) {
	defer func() {
		if resultErr != nil && resultErr.Kind == KindInvalidInput && !utf8.Valid(data) {
			resultErr = fail(KindInvalidInput, "JSON_INVALID_UTF8", doc, nil)
		}
	}()
	if uint64(len(data)) > MaxDocumentBytes {
		return "", fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", doc, nil)
	}
	if r != nil {
		if e := r.check(); e != nil {
			return "", e
		}
	}
	pos, nextCheck := 0, 0
	checkpoint := func() *Error {
		if pos < nextCheck || r == nil {
			return nil
		}
		nextCheck = pos + 4096
		return r.check()
	}
	space := func() *Error {
		for pos < len(data) && (data[pos] == ' ' || data[pos] == '\r' || data[pos] == '\n' || data[pos] == '\t') {
			if e := checkpoint(); e != nil {
				return e
			}
			pos++
		}
		return nil
	}
	invalid := func() *Error {
		code := "JSON_SYNTAX"
		if pos >= len(data) && allowed == nil {
			code = "JSON_TRUNCATED"
		}
		return fail(KindInvalidInput, code, doc, nil)
	}
	quoted := func() ([]byte, *Error) {
		start := pos
		if pos >= len(data) || data[pos] != '"' {
			return nil, invalid()
		}
		pos++
		for pos < len(data) {
			if e := checkpoint(); e != nil {
				return nil, e
			}
			c := data[pos]
			pos++
			if c == '"' {
				return data[start:pos], nil
			}
			if c == '\\' {
				pos++ // Escape validity belongs to the selected decoder.
			}
		}
		return nil, invalid()
	}
	if e := space(); e != nil {
		return "", e
	}
	if pos >= len(data) || data[pos] != '{' {
		if allowed == nil {
			return "", nil
		} // A non-object cannot select the new revision.
		return "", invalid()
	}
	pos++
	revision, seen := "", false
	for {
		if e := space(); e != nil {
			return "", e
		}
		if pos < len(data) && data[pos] == '}' {
			pos++
			break
		}
		key, e := quoted()
		if e != nil {
			return "", e
		}
		isSchema := jsonASCIIString(key, name)
		if e := space(); e != nil {
			return "", e
		}
		if pos >= len(data) || data[pos] != ':' {
			return "", invalid()
		}
		pos++
		if e := space(); e != nil {
			return "", e
		}
		if isSchema {
			if seen {
				return "", fail(KindInvalidInput, "JSON_DUPLICATE_KEY", doc, nil)
			}
			seen = true
			if pos < len(data) && data[pos] != '"' {
				// Read one aliased token only; no nested name index or number String allocation.
				dec := jsontext.NewDecoder(bytes.NewBuffer(data[pos:]), jsontext.AllowDuplicateNames(true))
				token, err := dec.ReadToken()
				if err != nil {
					e := jsonError(doc, "", err)
					e.Path = doc + "#/" + name
					return "", e
				}
				if token.Kind() != '{' && token.Kind() != '[' {
					pos += int(dec.InputOffset())
					if e := space(); e != nil {
						return "", e
					}
					if pos == len(data) {
						return "", fail(KindInvalidInput, "JSON_TRUNCATED", doc+"#/"+name, nil)
					}
					if data[pos] != ',' && data[pos] != '}' {
						return "", fail(KindInvalidInput, "JSON_SYNTAX", doc+"#/"+name, nil)
					}
				}
				code := "JSON_TYPE"
				if token.Kind() == 'n' {
					code = "JSON_NULL"
				}
				return "", fail(KindInvalidInput, code, doc+"#/"+name, nil)
			}
			value, e := quoted()
			if e != nil {
				return "", e
			}
			if len(value) > 6*128+2 {
				if allowed != nil {
					return "", fail(KindInvalidInput, "JSON_SHAPE", doc+"#/"+name, nil)
				}
				revision = "" // A non-target legacy value belongs to its existing decoder.
			} else if decodeErr := jsonv2.Unmarshal(value, &revision); decodeErr != nil {
				if allowed == nil {
					e := jsonError(doc, "", decodeErr)
					e.Path = doc + "#/" + name
					return "", e
				}
				return "", fail(KindInvalidInput, "JSON_SHAPE", doc+"#/"+name, nil)
			}
			if revision == early {
				return revision, nil // Its bounded decoder owns all following syntax and duplicates.
			}
		} else {
			start, depth := pos, 0
			for pos < len(data) {
				if e := checkpoint(); e != nil {
					return "", e
				}
				c := data[pos]
				if depth == 0 && (c == ',' || c == '}') {
					break
				}
				if c == '"' {
					if _, e := quoted(); e != nil {
						return "", e
					}
					continue
				}
				pos++
				switch c {
				case '{', '[':
					depth++
				case '}', ']':
					if depth > 0 {
						depth--
					}
				}
			}
			if pos == start || depth != 0 {
				return "", invalid()
			}
		}
		if e := space(); e != nil {
			return "", e
		}
		if pos >= len(data) {
			return "", invalid()
		}
		if data[pos] == '}' {
			pos++
			break
		}
		if data[pos] != ',' {
			return "", invalid()
		}
		pos++
		if e := space(); e != nil {
			return "", e
		}
		if pos >= len(data) || data[pos] == '}' {
			return "", invalid()
		}
	}
	if e := space(); e != nil {
		return "", e
	}
	if pos != len(data) {
		return "", invalid()
	}
	if allowed != nil && (!seen || !slices.Contains(allowed, revision)) {
		return "", fail(KindInvalidInput, "SCHEMA_UNSUPPORTED", doc, nil)
	}
	return revision, nil
}

func jsonASCIIString(key []byte, name string) bool {
	if len(key) < len(name)+2 || len(key) > 6*len(name)+2 {
		return false
	}
	pos := 1
	for i := range len(name) {
		if pos >= len(key)-1 {
			return false
		}
		value := uint16(key[pos])
		pos++
		if value == '\\' {
			if pos+5 > len(key)-1 || key[pos] != 'u' {
				return false
			}
			pos++
			value = 0
			for range 4 {
				c := key[pos]
				pos++
				switch {
				case c >= '0' && c <= '9':
					value = value*16 + uint16(c-'0')
				case c >= 'a' && c <= 'f':
					value = value*16 + uint16(c-'a'+10)
				case c >= 'A' && c <= 'F':
					value = value*16 + uint16(c-'A'+10)
				default:
					return false
				}
			}
		}
		if value != uint16(name[i]) {
			return false
		}
	}
	return pos == len(key)-1 && key[pos] == '"'
}

// findCycle returns a node on a reference cycle, or "".
func findCycle(nodes map[string]*EvidenceNode) string {
	return findCycleFrom(nodes, sortedKeys(nodes))
}

func findCycleFrom(nodes map[string]*EvidenceNode, roots []string) string {
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
	for _, id := range roots {
		if c := visit(id); c != "" {
			return c
		}
	}
	return ""
}
