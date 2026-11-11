package kit

import (
	"errors"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
)

var ordinaryReplayRoles = []string{
	"workload-profile", "ordinary-selection", "ordinary-planning-profile", "ordinary-registration", "ordinary-assets",
	"ordinary-plan-commitment", "ordinary-shard-proof", "ordinary-registry", "qualification-inventory", "slot-inventory",
	"semantics-run-binding", "ordinary-reference-selection", "eligibility-approval", "eligibility-registration",
	"evidence-policy", "ordinary-evidence", "retained",
}

type ordinaryReplayScope struct {
	root              *os.Root
	walked            map[string]fs.FileInfo
	walkedDirectories map[string]fs.FileInfo
	snapshot          *evidenceSnapshot
	selected          []string
}

func replayOrdinaryQuerySemantics(x *replayEnv) (resultErr *Error) {
	var paths uint64
	for _, member := range x.prof.Members {
		paths += uint64(len(member.Path) + len(member.Role) + 1)
	}
	for path := range x.directories {
		paths += uint64(len(path) + 1)
	}
	if e := x.r.eligibility.charge(16*paths+1, 8*paths+256*uint64(len(x.files)+len(x.directories)+15), "ordinary-replay-scope"); e != nil {
		return e
	}
	var req OrdinaryQuerySemanticsRequest
	var selection []byte
	observed := make(map[string]string, 14)
	for _, input := range []struct {
		role string
		data *[]byte
	}{
		{"workload-profile", &req.Profile}, {"ordinary-selection", &selection}, {"ordinary-planning-profile", &req.PlanningProfile},
		{"ordinary-registration", &req.Registration}, {"ordinary-assets", &req.AssetRegistration},
		{"ordinary-plan-commitment", &req.PlanCommitment}, {"ordinary-registry", &req.Registry},
		{"qualification-inventory", &req.QualificationInventory}, {"slot-inventory", &req.SlotInventory},
		{"semantics-run-binding", &req.RunBinding}, {"ordinary-reference-selection", &req.ReferenceSelection},
		{"eligibility-approval", &req.EligibilityApproval}, {"eligibility-registration", &req.EligibilityRegistration},
		{"evidence-policy", &req.Evidence.Policy},
	} {
		member, e := x.oneMember(input.role)
		if e != nil {
			return e
		}
		if strings.HasPrefix(strings.ToLower(member.Path), "evidence/") {
			return fail(KindInvalidInput, "MEMBER_TRUST_DOCUMENT_INSIDE", member.Path, nil)
		}
		data, e := x.member(member.Path)
		if e != nil {
			return e
		}
		*input.data = data
		observed[input.role] = member.SHA256
	}
	if e := x.r.eligibility.decodeOrdinaryTyped("ordinary-selection", selection, &req.Selection); e != nil {
		return e
	}
	if !validOrdinaryEligibilityBinding(req.Selection) {
		return fail(KindInvalidInput, "ORDINARY_PLAN_SELECTION", "ordinary-selection", nil)
	}
	req.RegistrySHA256 = ordinaryRegistrySHA
	for _, member := range x.prof.Members {
		inside := strings.HasPrefix(member.Path, "evidence/")
		if strings.HasPrefix(strings.ToLower(member.Path), "evidence/") && !inside || (member.Role == "ordinary-evidence") != inside {
			return fail(KindInvalidInput, "MEMBER_EVIDENCE_SCOPE", member.Path, nil)
		}
		if member.Role == "ordinary-shard-proof" {
			if len(req.ShardProofs) >= NativeOperations()["native-query-batch"].MaxCases {
				return fail(KindResourceLimit, "ORDINARY_PLAN_SELECTION_LIMIT", member.Path, nil)
			}
			data, e := x.member(member.Path)
			if e != nil {
				return e
			}
			req.ShardProofs = append(req.ShardProofs, data)
		}
	}
	before, e := x.g.lstat("evidence")
	if e != nil {
		return e
	}
	if before == nil || kind(before) != "dir" || x.directories["evidence"] == nil || !os.SameFile(before, x.directories["evidence"]) {
		return fail(KindInvalidInput, "MEMBER_EVIDENCE_ROOT_INVALID", "evidence", nil)
	}
	root, err := x.g.root.OpenRoot("evidence")
	if err != nil {
		return fail(KindIO, "READ_FAILED", "evidence", err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			if resultErr == nil {
				resultErr = fail(KindIO, "ROOT_CLOSE_FAILED", "evidence", err)
			} else {
				resultErr.Cause = errors.Join(resultErr.Cause, err)
			}
		}
	}()
	if e := x.r.semanticEntries.bindChild(x.g.root, "evidence", root); e != nil {
		return e
	}
	scope := &ordinaryReplayScope{root: root, walked: map[string]fs.FileInfo{}, walkedDirectories: map[string]fs.FileInfo{}}
	for path, info := range x.files {
		if local, ok := strings.CutPrefix(path, "evidence/"); ok {
			scope.walked[local] = info
		}
	}
	for path, info := range x.directories {
		if local, ok := strings.CutPrefix(path, "evidence/"); ok {
			scope.walkedDirectories[local] = info
		}
	}
	res, err := verifyOrdinaryQuerySemanticsInRun(x.r.caller, req, x.r, scope)
	x.ordinaryQuerySemantics, x.recomp = &res, Verdict{res.ExecutionStatus, res.Assessment}
	if res.ProducerRecorded.ExecutionStatus != StatusNotRun {
		x.recorded = res.ProducerRecorded
	}
	// member verified these exact bytes before the ordinary core consumed them.
	for _, identity := range []struct{ role, input string }{
		{"workload", "workload-profile"}, {"ordinary-selection", "ordinary-selection"},
		{"ordinary-registration", "ordinary-registration"}, {"ordinary-assets", "ordinary-assets"},
		{"ordinary-plan-commitment", "ordinary-plan-commitment"}, {"registry", "ordinary-registry"},
		{"semantics-run-binding", "semantics-run-binding"}, {"evidence-policy", "evidence-policy"},
	} {
		x.actual[identity.role] = observed[identity.input]
	}
	if scope.snapshot != nil {
		if e := x.r.eligibility.charge(4*paths+256*uint64(len(scope.snapshot.nodes)+1), 256*uint64(len(scope.snapshot.verifiedMembers)+1), "ordinary-replay-outer-binding"); e != nil {
			return e
		}
		for path, pin := range scope.snapshot.verifiedMembers {
			outer, ok := x.members["evidence/"+path]
			outer.Path = path
			if ok && sameMemberBytes(outer, pin) {
				x.verified["evidence/"+path], x.consumed["evidence/"+path] = true, true
			} else {
				x.finding("ORDINARY_OUTER_MEMBER", path, "nested evidence member와 replay inventory가 다르다")
			}
		}
		for _, node := range scope.snapshot.nodes {
			if slices.ContainsFunc(node.Files, func(m ReplayMember) bool { return m.Path == "oracle/manifest.json" }) {
				if x.prof.Subject.Run != node.Identities["run"] || strconv.FormatUint(x.prof.Subject.Attempt, 10) != node.Identities["attempt"] ||
					x.prof.Subject.Platform != node.Identities["platform"] || x.prof.Subject.EvidenceMode != node.EvidenceMode {
					x.finding("ORDINARY_SUBJECT_MISMATCH", node.ID, "subject와 ordinary producer identity가 다르다")
				}
			}
		}
	}
	if err != nil {
		return asError(err)
	}
	if x.prof.Records == nil || !slices.Equal(x.prof.Records, scope.selected) {
		x.finding("RECORDS_WORKLOAD_MISMATCH", "", "record 목록은 선택 의무의 정확한 순서 있는 union이어야 한다")
	}
	x.cons = Consumption{Expected: len(scope.selected), Consumed: len(res.Obligations)}
	gate := x.gate("ordinary-query-semantics")
	switch res.Assessment {
	case AssessPass:
		gate.pass()
	case AssessFail:
		gate.fail("", res.Code, "ordinary 의미 검증이 실패했다")
	default:
		gate.recorded()
	}
	return nil
}
