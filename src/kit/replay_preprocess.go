package kit

import (
	"fmt"
	"reflect"
	"strings"
)

// replayPreprocess checks one symbol-mode case of native-result-r5 (#218) against the
// caller-registered original source witness: the evaluated steps are recomputed from the
// source and the registered edits and must equal the recorded ones, and a case the
// evaluator stopped must be the fixed not-run outcome. Stored evidence cannot show that the
// driver received the ranges (a source without directives parses alike either way); the
// native connection tests own that.
func (x *replayEnv) replayPreprocess(c *rcCase, index int, registered *IncrementalCase, pp *Preprocess) *Error {
	g := x.gate("preprocess")
	if registered == nil {
		g.fail(c.ID, "PREPROCESS_CASE_UNBOUND", "preprocess 사례의 workload가 없다")
		return nil
	}
	if c.Preprocess == nil {
		g.fail(c.ID, "PREPROCESS_MISSING", "사례에 preprocess 관측이 없다")
		return nil
	}
	path := fmt.Sprintf("sources/%05d-%s.bin", index, c.ID)
	if m, ok := x.members[path]; !ok || m.Role != "case-source" {
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
	if !g.check(err == nil, c.ID, "PREPROCESS_SOURCE_MISMATCH", "원 입력과 등록 edit로 step을 다시 만들 수 없다") {
		return nil
	}
	want := PreprocessSteps(versions, pp)
	g.check(reflect.DeepEqual(*c.Preprocess, want), c.ID, "PREPROCESS_MISMATCH", "기록된 preprocess 관측이 원 입력에서 다시 계산한 값과 다르다")
	last := want[len(want)-1]
	if last.Status == PreprocessOK {
		g.check(!strings.HasPrefix(c.Code, "PREPROCESS_"), c.ID, "PREPROCESS_OUTCOME_MISMATCH", "모든 step이 OK인데 사례가 preprocess로 멈췄다")
		return nil
	}
	exec, code := PreprocessOutcome(last.Status)
	_, raw := x.members[fmt.Sprintf("responses/%05d-%s.json", index, c.ID)]
	g.check(c.ExecutionStatus == exec && c.Assessment == AssessBlocked && c.Code == code && len(c.Steps) == 0 && !raw &&
		c.Claims.IncrementalEquality == claimNotClaimed && c.Claims.IncrementalRoute == claimNotClaimed && c.Claims.Expectations == claimNotClaimed,
		c.ID, "PREPROCESS_OUTCOME_MISMATCH", "preprocess가 멈춘 사례의 상태·claim·step이 고정 결과와 다르다")
	return nil
}

// checkPreprocessPolicy requires the run policy to be the tsgk-native-policy/r2 identity
// recomputed from the registered operation (a caller may only narrow its run wall), the
// profile's output form and its preprocess member.
func (x *replayEnv) checkPreprocessPolicy(top nativeResult, prof IncrementalProfile, resultPath string) {
	g := x.gate("preprocess")
	reg := NativeOperations()[prof.Operation]
	op := top.Operation
	narrowed := op
	narrowed.RunWall = reg.RunWall
	want := NativePolicy(op, prof.Output, prof.Preprocess)
	got := IdentityRef{}
	for _, id := range top.Identities {
		if id.Role == "policy" {
			got = id
		}
	}
	g.check(reflect.DeepEqual(narrowed, reg) && op.RunWall > 0 && op.RunWall <= reg.RunWall && got == want,
		resultPath, "PREPROCESS_POLICY_MISMATCH", "run policy가 등록 operation·출력·preprocess에서 다시 계산한 tsgk-native-policy/r2와 다르다")
}
