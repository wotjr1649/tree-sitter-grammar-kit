package kit

import (
	jsonv2 "encoding/json/v2"
	"slices"
)

func (b *eligibilityBudget) semanticBase(profile OracleProfile, data []byte, cache *semanticNativeCache, res *QuerySemanticsResult, add func(string, string, string)) *Error {
	significant, sizeErr := b.significantJSONBytes(data)
	if sizeErr != nil {
		return sizeErr
	}
	if e := b.charge(16*significant+1, 16*uint64(len(data)), "semantic-base-replay"); e != nil {
		return e
	}
	var c rcCase
	if err := jsonv2.Unmarshal(data, &c); err != nil {
		return fail(KindInvalidInput, "JSON_SHAPE", "projection", err)
	}
	missingOracle := c.Oracle == nil && (c.ExecutionStatus == StatusCompleted || len(profile.Queries) != 0 || profile.API)
	if missingOracle {
		add("ORACLE_CLAIMS_MISSING", c.ID, "등록된 query/API에 필요한 기존 Oracle claim이 없다")
	}
	x := &replayEnv{r: b.r, lim: ReplayOperations()["evidence-replay"], seen: map[string]map[string]bool{},
		reuseObservation: true, mandatoryObservation: true, errorTreeRoute: true}
	claim, computed, e := b.semanticQueryExpectations(profile, cache)
	if claim == claimFail {
		res.BaseVerified = Verdict{StatusCompleted, AssessFail}
		if res.Assessment != AssessFail {
			code := c.Code
			if code == "" {
				code = "QUERY_EXPECTATION_FAILED"
			}
			add(code, c.ID, "중단 전에 확인한 query 기대값 모순을 보존한다")
		}
	}
	if e != nil {
		return e
	}
	if computed || claim == claimFail {
		x.recomputedOracle = map[string]string{"query-expectations": claim}
	}
	out := x.replayCase(&c, 0, &profile.Native.Cases[0], len(profile.Queries) != 0)
	if cache.record.haveRaw {
		x.checkSeen(map[string]string{"producer": cache.record.approved.ProducerSHA256, "policy": NativePolicy(NativeOperations()[profile.Native.Operation], profile.Native.Output, nil).SHA256})
	}
	res.BaseVerified = Verdict{out.status, out.assess}
	if missingOracle {
		res.BaseVerified.Assessment = AssessFail
	}
	if !executionStatuses[out.status] {
		res.BaseVerified = Verdict{StatusFailed, AssessFail}
		add("SEMANTICS_PROJECTION_VERDICT", c.ID, "허용되지 않은 producer 실행 상태를 실패로 정규화한다")
	}
	if !out.complete && res.BaseVerified.Assessment != AssessFail {
		res.BaseVerified.Assessment = AssessBlocked
	}
	for _, gate := range x.gates {
		gate.finish()
		res.Gates = append(res.Gates, *gate)
		if gate.Failed > 0 {
			res.BaseVerified.Assessment = AssessFail
			add(gate.Code, gate.Record, gate.Detail)
		} else if (gate.NotRecomputed > 0 || gate.Recomputed == 0) && res.BaseVerified.Assessment != AssessFail {
			res.BaseVerified.Assessment = AssessBlocked
		}
	}
	if out.assess == AssessFail && res.Assessment != AssessFail {
		code := c.Code
		if code == "" {
			code = "SEMANTICS_BASE_FAILED"
		}
		add(code, c.ID, "기존 claim의 재계산 판정이 FAIL이다")
	}
	if c.Assessment == AssessFail && res.Assessment != AssessFail {
		code := c.Code
		if code == "" {
			code = "SEMANTICS_PRODUCER_FAILED"
		}
		add(code, c.ID, "producer에 보존된 FAIL을 미재계산 claim 때문에 지우지 않는다")
	}
	if !computed || c.ExecutionStatus != StatusCompleted || !cache.ready {
		if res.BaseVerified.Assessment != AssessFail {
			res.BaseVerified.Assessment = AssessBlocked
		}
	}
	return b.r.check()
}

func (b *eligibilityBudget) semanticQueryExpectations(profile OracleProfile, cache *semanticNativeCache) (string, bool, *Error) {
	claim := claimNotClaimed
	computed := true
	for _, expected := range profile.OracleCases[0].QueryExpect {
		if e := b.charge(uint64(len(profile.Queries)+len(cache.tuples))+1, 0, "semantic-query-expectation"); e != nil {
			return claim, false, e
		}
		value := claimBlocked
		qi := slices.IndexFunc(profile.Queries, func(q OracleQuery) bool { return q.ID == expected.Query })
		if expected.Step >= 0 && expected.Step < len(cache.checked) && qi >= 0 && qi < len(cache.checked[expected.Step].Incremental.Queries) {
			q := cache.checked[expected.Step].Incremental.Queries[qi]
			index := -1
			// The cache uses the independently registered tuple order, not query order.
			for i, tuple := range cache.tuples {
				if tuple.Query == expected.Query && int(tuple.Step) == expected.Step && tuple.TreeRole == "incremental" {
					index = i
					break
				}
			}
			if index < 0 || index >= len(cache.evaluated) || !cache.evaluated[index] {
				claim = worseClaim(claim, claimBlocked)
				computed = false
				continue
			}
			evaluated := cache.values[index]

			if evaluated.status != expected.Status || evaluated.code != expected.Code {
				value = claimFail
			} else if expected.Error != nil && (q.Error == nil || q.Error.Type != expected.Error.Type || q.Error.Offset != expected.Error.Offset) {
				value = claimFail
			} else if expected.Captures == nil {
				value = claimPass
			} else {
				if index >= 0 && index < len(cache.evaluated) && cache.evaluated[index] && cache.values[index].captures != nil && expected.Step < len(cache.versions) {
					captures := cache.values[index].captures
					value = claimPass
					for i := range min(len(*expected.Captures), len(captures)) {
						if e := b.charge(uint64(len(cache.versions[expected.Step]))+1, uint64(len(cache.versions[expected.Step])), "semantic-capture-text"); e != nil {
							return claim, false, e
						}
						text, err := CaptureText(cache.versions[expected.Step], captures[i])
						if err != nil {
							value = claimBlocked
							break
						}
						want := (*expected.Captures)[i]
						if want.Name != captures[i].Name || want.Type != captures[i].Type || want.Text != text {
							value = claimFail
							break
						}
					}
					if value == claimPass && len(*expected.Captures) != len(captures) {
						value = claimFail
					}
				}
			}
		} else {
			computed = false
		}
		claim = worseClaim(claim, value)
	}
	return claim, computed, nil
}
