package kit

import "testing"

func TestScopedRuntimePinsSharedBudget(t *testing.T) {
	for _, reader := range []string{runtimeAcceptedReader, runtimePreprocessCSTReader} {
		t.Run(reader, func(t *testing.T) {
			b := semanticTestBudget(t)
			prior := &eligibilityRuntimePins{}
			b.runtime = prior
			subject := []byte("scoped runtime subject")
			req := scopedPinRequest(t, subject, loggerPinMaterials(t, "shared"), reader)
			res, runtime, err := verifyScopedRuntime(req, subject, b, reader)
			if err != nil || runtime == nil || !validPassedScopedRuntime(&res, digestHex(subject), reader) || b.runtime != prior || res.BytesRead != b.r.totalRead || res.Work != b.work || res.LogicalBytes != b.logical {
				t.Fatalf("scope, ledger or authority escaped: %+v %v", res, err)
			}
			read, work, logical := b.r.totalRead, b.work, b.logical
			res, runtime, err = verifyScopedRuntime(req, []byte("another subject"), b, reader)
			if err != nil || runtime != nil || res.Assessment != AssessFail || res.Complete || b.runtime != prior || res.BytesRead+read != b.r.totalRead || res.Work+work != b.work || res.LogicalBytes+logical != b.logical {
				t.Fatalf("subject or ledger mismatch: %+v %v", res, err)
			}
			b.work = eligibilityWorkLimit
			res, runtime, err = verifyScopedRuntime(req, subject, b, reader)
			if err == nil || err.Code != "ELIGIBILITY_WORK_LIMIT" || runtime != nil || res.Complete || b.runtime != prior {
				t.Fatalf("scope bypassed work limit: %+v %v", res, err)
			}
		})
	}
}
