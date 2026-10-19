package kit

import "slices"

// This reducer deliberately has its own ID: legacy oracle replay retains its
// record-only claims, and a stored-capture PASS is not reference qualification.
func replayQueryWitness(x *replayEnv) *Error {
	var req QueryWitnessRequest
	for _, input := range []struct {
		role string
		dst  *[]byte
	}{
		{"query-registration", &req.Registration},
		{"case-source", &req.Source},
		{"raw", &req.Raw},
		{"projection", &req.Projection},
	} {
		m, e := x.oneMember(input.role)
		if e != nil {
			return e
		}
		b, e := x.member(m.Path)
		if e != nil {
			return e
		}
		if b == nil {
			b = []byte{} // a verified zero-byte member is present evidence
		}
		*input.dst = b
	}
	reg, e := parseQueryWitnessRegistration(req.Registration, x.r)
	if e != nil {
		if e.Kind != KindInvalidInput {
			return e
		}
		return fail(KindInvalidInput, "MEMBER_INVALID_"+e.Code, "query-registration", nil)
	}
	x.actual = map[string]string{
		"registration": digestHex(req.Registration), "source": reg.Input.SHA256,
		"route": reg.Route, "platform": reg.Platform, "role": reg.Role, "policy": reg.Policy,
	}
	if x.prof.Subject.Platform != reg.Platform {
		x.finding("QUERY_WITNESS_SUBJECT_PLATFORM_MISMATCH", "", "등록 query platform과 replay subject가 다르다")
	}
	if x.prof.Records != nil && !slices.Equal(x.prof.Records, []string{reg.Case}) {
		x.finding("RECORDS_WORKLOAD_MISMATCH", "", "등록 record 목록이 query registration과 다르다")
	}
	res, err := verifyQueryWitness(x.r, req, nil)
	if err != nil {
		return asError(err)
	}
	x.cons = Consumption{Expected: 1, Consumed: 1}
	g := x.gate("stored-query-witness")
	switch res.Assessment {
	case AssessPass:
		g.pass()
	case AssessFail:
		g.fail(reg.Case, "QUERY_WITNESS_CONTRADICTION", "source·raw CST·query capture·projection 또는 독립 anchor가 다르다")
	default:
		g.recorded()
	}
	for _, f := range res.Findings {
		if f.Severity == "error" {
			x.finding(f.Code, f.Path, f.Message)
		}
	}
	x.recomp = Verdict{res.ExecutionStatus, res.Assessment}
	return nil
}
