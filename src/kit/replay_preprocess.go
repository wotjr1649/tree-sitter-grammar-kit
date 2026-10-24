package kit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// sourceReadCodes are the codes of a live run that could not read the case input; nothing
// of such a case is recomputable offline.
var sourceReadCodes = map[string]bool{"SOURCE_MISSING": true, "SOURCE_LINK_REJECTED": true, "SOURCE_UNREADABLE": true, "SOURCE_MISMATCH": true}

// replayPreprocess checks one symbol-mode case against the
// caller-registered original source witness: the evaluated steps are recomputed from the
// source and the registered edits and must equal the recorded ones; a case the evaluator
// stopped must be the fixed not-run outcome with no trace of a driver; and in a run case
// every non-empty leaf of every full tree starts and ends inside its step's active ranges.
// A source without directives parses alike with and without ranges, so stored evidence
// cannot show the driver received them there; the native connection tests own that.
func (x *replayEnv) replayPreprocess(c *rcCase, index int, registered *IncrementalCase, pp *Preprocess, top nativeResult) *Error {
	g := x.gate("preprocess")
	var cst *PreprocessCSTResult
	if x.preprocessCST {
		var err error
		cst, err = decodePreprocessCST(c.PreprocessCST)
		if err != nil {
			x.gate("preprocess-cst").fail(c.ID, "PREPROCESS_CST_INVALID", "primary CST 관측의 strict 형식이 잘못됐다")
			return nil
		}
		x.currentCST = &PreprocessCSTResult{Schema: PreprocessCSTSchema, Result: "NOT_APPLICABLE", Steps: []PreprocessCSTStep{}}
		defer func() {
			x.gate("preprocess-cst").check(reflect.DeepEqual(cst, x.currentCST), c.ID, "PREPROCESS_CST_MISMATCH", "primary CST 관측이 원 입력·원 응답에서 다시 계산한 값과 다르다")
			x.preprocessCSTResults = append(x.preprocessCSTResults, PreprocessCSTCase{ID: c.ID, Result: *x.currentCST})
		}()
	}
	if registered == nil {
		g.fail(c.ID, "PREPROCESS_CASE_UNBOUND", "preprocess 사례의 workload가 없다")
		return nil
	}
	_, raw := x.members[fmt.Sprintf("responses/%05d-%s.json", index, c.ID)]
	traceless := len(c.Steps) == 0 && !raw && (len(c.Process) == 0 || string(c.Process) == "null") && c.Producer == nil && c.ResponseStatus == "" && c.ResponseCode == ""
	early := c.Preprocess == nil && c.ExecutionStatus == StatusNotRun && c.Assessment == AssessNotAssessed && traceless
	if c.Preprocess == nil && !early {
		g.fail(c.ID, "PREPROCESS_MISSING", "사례에 preprocess 관측이 없다")
		return nil
	}
	path := fmt.Sprintf("sources/%05d-%s.bin", index, c.ID)
	m, witnessed := x.members[path]
	if early && sourceReadCodes[c.Code] {
		if x.currentCST != nil {
			x.currentCST.Code = c.Code
		}
		// the live run could not read its input: stopped before any evaluation, nothing to
		// recompute; a witness given anyway must still be the registered input
		if witnessed && m.Role == "case-source" {
			source, e := x.member(path)
			if e != nil {
				return e
			}
			g.check(digestHex(source) == registered.Input.SHA256 && uint64(len(source)) == registered.Input.Bytes, c.ID, "PREPROCESS_SOURCE_MISMATCH", "원 입력 witness가 등록 입력과 다르다")
		}
		g.recorded()
		return nil
	}
	if !witnessed || m.Role != "case-source" {
		g.fail(c.ID, "PREPROCESS_SOURCE_MISSING", "preprocess를 다시 계산할 원 입력 witness가 없다")
		return nil
	}
	source, e := x.member(path)
	if e != nil {
		return e
	}
	// a witness of the registered length and digest, the empty one included, is valid
	if digestHex(source) != registered.Input.SHA256 || uint64(len(source)) != registered.Input.Bytes {
		g.fail(c.ID, "PREPROCESS_SOURCE_MISMATCH", "원 입력 witness가 등록 입력과 다르다")
		return nil
	}
	versions, _, err := ApplyEdits(registered.Encoding, source, registered.Edits, x.nativeInputBytes)
	if early {
		if x.currentCST != nil && err != nil {
			x.currentCST.Code = asError(err).Code
		}
		// the edits were refused before any evaluation: the same refusal, recomputed
		var ke *Error
		g.check(errors.As(err, &ke) && ke.Code == c.Code, c.ID, "PREPROCESS_OUTCOME_MISMATCH", "preprocess 전에 멈춘 사례가 원 입력·등록 edit의 거부와 다르다")
		return nil
	}
	if !g.check(err == nil, c.ID, "PREPROCESS_SOURCE_MISMATCH", "원 입력과 등록 edit로 step을 다시 만들 수 없다") {
		return nil
	}
	want := PreprocessSteps(versions, pp)
	if same := g.check(reflect.DeepEqual(*c.Preprocess, want), c.ID, "PREPROCESS_MISMATCH", "기록된 preprocess 관측이 원 입력에서 다시 계산한 값과 다르다"); same && x.selectionRoute {
		x.selectionSteps = want
		x.selectionBytes = make([]uint64, len(versions))
		for i, v := range versions {
			x.selectionBytes[i] = uint64(len(v))
		}
	}
	last := want[len(want)-1]
	if last.Status != PreprocessOK {
		exec, code := PreprocessOutcome(last.Status)
		if x.currentCST != nil {
			x.currentCST.Code = code
		}
		g.check(c.ExecutionStatus == exec && c.Assessment == AssessBlocked && c.Code == code && traceless &&
			c.Claims.IncrementalEquality == claimNotClaimed && c.Claims.IncrementalRoute == claimNotClaimed && c.Claims.Expectations == claimNotClaimed,
			c.ID, "PREPROCESS_OUTCOME_MISMATCH", "preprocess가 멈춘 사례의 상태·claim이 고정 결과와 다르거나 driver 실행 흔적이 있다")
		return nil
	}
	if x.preprocessCST {
		if e := x.replayCSTPrimary(c, index, registered, pp, source, top, cst.Primary); e != nil {
			return e
		}
		if x.currentCST.Result != "PASS" {
			assess := AssessBlocked
			if x.currentCST.Result == "FAIL" {
				assess = AssessFail
			}
			x.gate("preprocess-cst").check(c.ExecutionStatus == StatusNotRun && c.Assessment == assess && c.Code == x.currentCST.Code && traceless && len(c.Expectations) == 0 && len(c.Segments) == 0 && len(c.References) == 0 && c.Oracle == nil &&
				c.Claims.IncrementalEquality == claimNotClaimed && c.Claims.IncrementalRoute == claimNotClaimed && c.Claims.Expectations == claimNotClaimed, c.ID, "PREPROCESS_CST_OUTCOME_MISMATCH", "primary 정지 뒤 active 실행 흔적이 있거나 고정 판정과 다르다")
			return nil
		}
	}
	g.check(!strings.HasPrefix(c.Code, "PREPROCESS_"), c.ID, "PREPROCESS_OUTCOME_MISMATCH", "모든 step이 OK인데 사례가 preprocess로 멈췄다")
	for _, s := range c.Steps {
		for _, t := range []*rcTree{s.Incremental, s.Fresh} {
			if t == nil || t.Tree == nil && t.Summary == nil && t.Status != StatusCompleted {
				continue // a slot a resource limit ended carries no tree and no identity
			}
			// every tree's policy is checked against the recomputed r2 once the run is read
			ids := []IdentityRef{}
			if t.Tree != nil {
				ids = t.Tree.Identities
				if s.Step >= 0 && s.Step < len(want) {
					g.check(leavesInside(t.Tree.Nodes, want[s.Step].Active), c.ID, "PREPROCESS_TREE_OUTSIDE_RANGES", "tree의 leaf가 그 step의 활성 범위 밖에 있다")
				}
			} else if t.Summary != nil {
				ids = t.Summary.Identities
			}
			x.preprocessTrees = append(x.preprocessTrees, preprocessTree{c.ID, ids})
		}
	}
	return nil
}

// preprocessTree is the identity list of one symbol-mode tree.
type preprocessTree struct {
	record string
	ids    []IdentityRef
}

// leavesInside reports whether every non-empty leaf starts and ends inside the active
// ranges: a parent may span a skipped section and a token may continue across a range
// boundary, but no token is read from skipped bytes.
func leavesInside(nodes []TreeNode, active []Span) bool {
	parent := make([]bool, len(nodes))
	for _, n := range nodes {
		if n.Parent >= 0 && n.Parent < int64(len(nodes)) {
			parent[n.Parent] = true
		}
	}
	in := func(b uint32) bool {
		for _, s := range active {
			if b >= s.StartByte && b < s.EndByte {
				return true
			}
		}
		return false
	}
	for i, n := range nodes {
		if !parent[i] && n.EndByte > n.StartByte && (!in(n.StartByte) || !in(n.EndByte-1)) {
			return false
		}
	}
	return true
}

// checkPreprocessPolicy requires the run policy and the one policy of every tree to be the
// tsgk-native-policy/r2 identity recomputed from the registered operation (a caller may
// only narrow its run wall), the profile's output form and its preprocess member.
func (x *replayEnv) checkPreprocessPolicy(top nativeResult, prof IncrementalProfile, resultPath string) {
	g := x.gate("preprocess")
	reg := NativeOperations()[prof.Operation]
	// the recorded operation keeps its walls in nanoseconds, which encoding/json reads; the
	// bytes must be its own canonical form, so no case-folded or repeated member hides
	var op NativeOperation
	if err := json.Unmarshal(top.operation, &op); err != nil {
		g.fail(resultPath, "PREPROCESS_POLICY_MISMATCH", "기록된 operation을 읽을 수 없다")
		return
	}
	if canon, err := json.Marshal(op); err != nil || !bytes.Equal(canon, top.operation) {
		g.fail(resultPath, "PREPROCESS_POLICY_MISMATCH", "기록된 operation이 정규 형식이 아니다")
		return
	}
	narrowed := op
	narrowed.RunWall = reg.RunWall
	want := nativePolicy(op, prof.Output, prof.Preprocess, x.selectionRoute)
	if x.preprocessCST {
		want = NativePolicy(op, prof.Output, prof.Preprocess)
	}
	got := IdentityRef{}
	for _, id := range top.Identities {
		if id.Role == "policy" {
			got = id
		}
	}
	g.check(reflect.DeepEqual(narrowed, reg) && op.RunWall > 0 && op.RunWall <= reg.RunWall && got == want,
		resultPath, "PREPROCESS_POLICY_MISMATCH", "run policy가 등록 operation·출력·preprocess에서 다시 계산한 tsgk-native-policy/r2와 다르다")
	for _, t := range x.preprocessTrees {
		var policies []IdentityRef
		for _, id := range t.ids {
			if id.Role == "policy" {
				policies = append(policies, id)
			}
		}
		g.check(len(policies) == 1 && policies[0] == want, t.record, "PREPROCESS_POLICY_MISMATCH", "tree의 policy가 하나가 아니거나 다시 계산한 tsgk-native-policy/r2와 다르다")
	}
}
