package kit

const (
	runtimeAcceptedReader      = "tsgk-runtime-accepted/r1"
	runtimePreprocessCSTReader = "tsgk-runtime-preprocess-cst/r1"
	runtimeReadersName         = "native/runtime-readers.json"
)

// Reader scopes are independently approved material, never inferred from logger support.
type runtimeReaders struct {
	Schema              string `json:"schema"`
	TupleSHA256         string `json:"tuple_sha256"`
	AcceptedReader      string `json:"accepted_reader,omitempty"`
	PreprocessCSTReader string `json:"preprocess_cst_reader,omitempty"`
}

func verifyScopedRuntime(req *EvidencePinsRequest, subject []byte, b *eligibilityBudget, reader string) (res RuntimeEvidenceResult, runtime *eligibilityRuntimePins, resultErr *Error) {
	read, work, logical := b.r.totalRead, b.work, b.logical
	defer func() {
		res.Reader = reader
		res.BytesRead, res.Work, res.LogicalBytes = b.r.totalRead-read, b.work-work, b.logical-logical
		if resultErr != nil {
			known := res.Assessment == AssessFail
			failReport(&res.Report, resultErr)
			if known {
				res.Assessment = AssessFail
			}
			res.Complete = false
			runtime = nil
		}
	}()
	res, runtime, resultErr = verifyLoggerRuntime(req, subject, b)
	if resultErr != nil || runtime == nil {
		return
	}
	var descriptor *EvidencePinMaterial
	for i, m := range res.Pins.Materials {
		if e := b.charge(1, 0, "runtime-reader-index"); e != nil {
			return res, nil, e
		}
		if m.Role == "runtime-readers" && m.Name != runtimeReadersName {
			return res, nil, fail(KindInvalidInput, "PIN_RUNTIME_READER_ROLE", m.Name, nil)
		}
		if m.Name == runtimeReadersName {
			descriptor = &res.Pins.Materials[i]
		}
	}
	if descriptor == nil {
		res.Assessment, res.Complete = AssessBlocked, false
		res.Findings = append(res.Findings, Finding{Code: "PIN_RUNTIME_READER_MISSING", Severity: "warning", Path: runtimeReadersName, Message: "요청한 판독 범위의 독립 승인 자료가 없다"})
		return res, nil, nil
	}
	if descriptor.Role != "runtime-readers" {
		return res, nil, fail(KindInvalidInput, "PIN_RUNTIME_READER_ROLE", runtimeReadersName, nil)
	}
	var scopes runtimeReaders
	if e := b.decode(runtimeReadersName, req.Materials[runtimeReadersName], &scopes, true); e != nil {
		return res, nil, e
	}
	if scopes.Schema != "tsgk-runtime-readers/r1" {
		return res, nil, fail(KindUnsupported, "PIN_RUNTIME_READER_UNSUPPORTED", runtimeReadersName, nil)
	}
	if scopes.TupleSHA256 != res.RuntimePin.TupleSHA256 {
		res.Assessment, res.Complete = AssessFail, false
		res.Findings = append(res.Findings, Finding{Code: "PIN_RUNTIME_READER_BINDING", Severity: "error", Path: runtimeReadersName, Message: "판독 범위의 승인 tuple이 다르다"})
		return res, nil, nil
	}
	var selected string
	switch reader {
	case runtimeAcceptedReader:
		selected = scopes.AcceptedReader
	case runtimePreprocessCSTReader:
		selected = scopes.PreprocessCSTReader
	default:
		return res, nil, fail(KindUnsupported, "PIN_RUNTIME_READER_UNSUPPORTED", reader, nil)
	}
	if selected == "" {
		res.Assessment, res.Complete = AssessBlocked, false
		res.Findings = append(res.Findings, Finding{Code: "PIN_RUNTIME_READER_MISSING", Severity: "warning", Path: runtimeReadersName, Message: "요청한 판독 범위가 승인되지 않았다"})
		return res, nil, nil
	}
	if selected != reader {
		return res, nil, fail(KindUnsupported, "PIN_RUNTIME_READER_UNSUPPORTED", runtimeReadersName, nil)
	}
	return
}

func validPassedScopedRuntime(p *RuntimeEvidenceResult, subject, reader string) bool {
	if !validPassedLoggerRuntime(p, subject) || p.Reader != reader {
		return false
	}
	for _, m := range p.Pins.Materials {
		if m.Name == runtimeReadersName {
			return m.Role == "runtime-readers" && m.Bytes > 0
		}
	}
	return false
}

func scopedRuntimeReader(reducer string) string {
	switch reducer {
	case "native-result-r11", "oracle-set-r8":
		return runtimeAcceptedReader
	case "native-result-r12":
		return runtimePreprocessCSTReader
	}
	return ""
}
