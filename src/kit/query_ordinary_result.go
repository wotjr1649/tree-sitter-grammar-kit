package kit

func ordinaryResultError(res *OrdinaryQuerySemanticsResult, resultErr error, read, work, logical uint64) {
	res.BytesRead, res.Work, res.LogicalBytes = read, work, logical
	if resultErr == nil {
		return
	}
	known, firstCode := res.Assessment == AssessFail, res.Code
	e := asError(resultErr)
	failReport(&res.Report, e)
	if len(res.Findings) > 1000 {
		res.Findings = res.Findings[:1000]
	}
	res.Complete, res.ProcessingComplete = false, false
	res.Semantic = AssessBlocked
	if res.Tuple.Failed > 0 {
		res.Semantic = AssessFail
	}
	if known {
		res.Assessment, res.Code = AssessFail, firstCode
	} else {
		res.Code = e.Code
	}
}
