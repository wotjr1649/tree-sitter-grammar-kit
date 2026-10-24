package kit

import (
	"encoding/json"
	"fmt"
)

func (x *replayEnv) replayCSTPrimary(c *rcCase, index int, registered *IncrementalCase, pp *Preprocess, source []byte, top nativeResult, primary *PreprocessPrimary) *Error {
	g := x.gate("preprocess-cst")
	var op NativeOperation
	if err := json.Unmarshal(top.operation, &op); err != nil {
		g.fail(c.ID, "PREPROCESS_CST_POLICY_INVALID", "primary operation을 읽을 수 없다")
		return nil
	}
	path := fmt.Sprintf("primary/responses/%05d-%s.json", index, c.ID)
	var raw []byte
	if m, ok := x.members[path]; ok {
		if m.Role != "primary-response" {
			g.fail(path, "PRIMARY_RESPONSE_ROLE_INVALID", "primary 원 응답 role이 다르다")
			return nil
		}
		var e *Error
		raw, e = x.member(path)
		if e != nil {
			return e
		}
	}
	build := ""
	if top.Build != nil {
		build = top.Build.Identity
	}
	result := CheckPreprocessCST(PreprocessCSTRequest{Case: *registered, Source: source, Preprocess: pp, Raw: raw, Primary: primary, Operation: op, Platform: x.prof.Subject.Platform, RunID: top.ObservationRunID, ProducerSHA256: build})
	x.currentCST = &result
	switch result.Result {
	case "FAIL":
		g.fail(c.ID, result.Code, "primary CST 검증 실패는 detector BLOCKED로 허용할 수 없다")
	case "UNVERIFIED":
		g.recorded()
	default:
		g.pass()
	}
	return nil
}
