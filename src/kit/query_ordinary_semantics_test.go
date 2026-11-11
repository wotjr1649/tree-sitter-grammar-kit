package kit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOrdinaryFinishRechecksMutableSnapshotBeforeVerifiedSeal(t *testing.T) {
	f := newOrdinaryReaderFixture(t)
	path := "oracle/capture/response.bin"
	original := f.s.snapshot.files[path]
	if len(original) == 0 {
		t.Fatal("fixture response member is empty")
	}
	if e := f.b.ordinarySnapshotIdentity(f.root, f.s); e != nil {
		t.Fatalf("fixture identity should initially hold: %v", e)
	}
	parent := filepath.ToSlash(filepath.Dir(path))
	temp := filepath.ToSlash(filepath.Join(parent, "mutable-replacement.bin"))
	file, err := f.root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]byte(nil), original...)
	changed[len(changed)-1] ^= 1
	if _, err := file.Write(changed); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.root.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := f.root.Rename(temp, path); err != nil {
		t.Fatal(err)
	}
	if len(changed) != len(original) {
		t.Fatal("mutation must preserve the member length")
	}

	s := f.s
	s.res.Complete, s.res.Assessment = true, AssessPass
	res := newOrdinarySemanticsResult()
	res.BindingValid, res.ProcessingComplete = true, true
	res.BaseVerified = Verdict{ExecutionStatus: StatusCompleted, Assessment: AssessPass}
	res.Tuple = QueryEligibilityCounts{Required: 1, Verified: 1}
	res.Obligations = []OrdinaryQuerySemanticsTuple{{ID: "obligation", State: ordinaryTupleVerified}}
	if err := f.b.ordinaryFinish(f.root, s, &res); err == nil || asError(err).Code != "SOURCE_CHANGED" {
		t.Fatalf("final identity mutation was not rejected: %v", err)
	}
	if res.Semantic != AssessBlocked || res.BindingValid || res.Tuple.Verified != 0 || res.Tuple.Blocked != 1 || res.Obligations[0].State != ordinaryTupleBlocked || res.Complete {
		t.Fatalf("verified result survived final identity failure: %+v", res)
	}
}

func TestVerifyOrdinaryQuerySemanticsRejectsEmptyRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	defer cancel()
	res, err := VerifyOrdinaryQuerySemantics(ctx, OrdinaryQuerySemanticsRequest{})
	if err == nil || asError(err).Kind != KindInvalidInput || asError(err).Code != "JSON_TRUNCATED" {
		t.Fatalf("empty caller documents were not rejected by strict decoding: %v", err)
	}
	if res.Semantic == AssessPass || res.Complete || res.ProcessingComplete {
		t.Fatalf("empty request produced a completed result: %+v", res)
	}
}

func TestOrdinaryAssetResolverPinsAllIndependentMembers(t *testing.T) {
	f := newOrdinaryReaderFixture(t)
	if err := f.root.Mkdir("expected", 0700); err != nil {
		t.Fatal(err)
	}
	assets := map[string]OrdinaryAsset{}
	put := func(path, role string, value []byte) OrdinaryPin {
		t.Helper()
		file, err := f.root.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(value); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		pin := OrdinaryPin{Path: path, Bytes: uint64(len(value)), SHA256: digestHex(value)}
		owned := eligibilityMember{NodeID: "node-independent", Member: ReplayMember{Path: path, Role: role, Bytes: pin.Bytes, SHA256: pin.SHA256}}
		f.s.owned[path], f.s.owners[path] = owned, owned.NodeID
		f.s.snapshot.files[path] = value
		f.s.snapshot.verifiedMembers[path] = owned.Member
		f.s.snapshot.observed[path], err = f.root.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return pin
	}
	body := []byte("independent expected metadata")
	pin := put("expected/metadata.json", "metadata", body)
	basis := put("expected/basis.json", "authoring-basis", []byte("reviewed source"))
	review := put("expected/review.json", "independent-review", []byte("review record"))
	assets["metadata"] = OrdinaryAsset{ID: "metadata", Role: "metadata", State: "PINNED", Pin: &pin, AuthoringBasis: &basis, IndependentReview: &review}
	resolver := ordinaryAssetResolver{session: f.s, assets: assets, resolved: map[string][]byte{}}
	got, err := resolver.resolve("metadata", "metadata")
	if err != nil || string(got) != string(body) {
		t.Fatalf("pinned expected asset failed resolution: %q, %v", got, err)
	}
	bad := pin
	bad.Path = "oracle/capture/response.bin"
	assets["metadata"] = OrdinaryAsset{ID: "metadata", Role: "metadata", State: "PINNED", Pin: &bad, AuthoringBasis: &basis, IndependentReview: &review}
	resolver = ordinaryAssetResolver{session: f.s, assets: assets, resolved: map[string][]byte{}}
	if _, err := resolver.resolve("metadata", "metadata"); err == nil || asError(err).Code != "ORDINARY_ASSET_OWNER" {
		t.Fatalf("asset path aliased another graph owner: %v", err)
	}
}

func TestOrdinaryResultErrorPreservesConfirmedFailureAndPartialCountsAtFindingCap(t *testing.T) {
	res := newOrdinarySemanticsResult()
	res.Tuple = QueryEligibilityCounts{Required: 4, Unprocessed: 4}
	res.BaseVerified = Verdict{ExecutionStatus: StatusCompleted, Assessment: AssessFail}
	res.ordinaryRecordTuple("verified-id", ordinaryTupleJudgment{State: ordinaryTupleVerified})
	res.ordinaryRecordTuple("failed-id", ordinaryTupleJudgment{State: ordinaryTupleFailed, Code: "BASE_QUERY_FAILED"})
	for len(res.Findings) < 1000 {
		res.Findings = append(res.Findings, Finding{Severity: "warning", Code: "capped"})
	}
	ordinaryResultError(&res, fail(KindCancelled, "CALLER_CANCELLED", "ordinary", nil), 17, 23, 29)
	if res.Tuple.Required != 4 || res.Tuple.Verified != 1 || res.Tuple.Failed != 1 || res.Tuple.Blocked != 0 || res.Tuple.Unprocessed != 2 {
		t.Fatalf("cancel erased partial tuple counts: %+v", res.Tuple)
	}
	if res.Semantic != AssessFail || res.Assessment != AssessFail || res.Code != "BASE_QUERY_FAILED" || res.BaseVerified.Assessment != AssessFail {
		t.Fatalf("cancel replaced an earlier FAIL: %+v", res)
	}
	if res.ExecutionStatus != StatusCancelled || res.Complete || res.ProcessingComplete || len(res.Findings) != 1000 || res.BytesRead != 17 || res.Work != 23 || res.LogicalBytes != 29 {
		t.Fatalf("cancel/finding-cap state is inconsistent: %+v", res)
	}
}

func TestOrdinaryResultResourceLimitKeepsVerifiedTuplesUnprocessedTail(t *testing.T) {
	res := newOrdinarySemanticsResult()
	res.Tuple = QueryEligibilityCounts{Required: 3, Unprocessed: 3}
	res.BaseVerified = Verdict{ExecutionStatus: StatusCompleted, Assessment: AssessPass}
	res.ordinaryRecordTuple("verified-id", ordinaryTupleJudgment{State: ordinaryTupleVerified})
	ordinaryResultError(&res, fail(KindResourceLimit, "ELIGIBILITY_WORK_LIMIT", "ordinary", nil), 1, 32_000_000, 2)
	if res.Tuple.Required != res.Tuple.Verified+res.Tuple.Failed+res.Tuple.Blocked+res.Tuple.Unprocessed || res.Tuple.Verified != 1 || res.Tuple.Unprocessed != 2 {
		t.Fatalf("resource stop lost the processed/unprocessed partition: %+v", res.Tuple)
	}
	if res.ExecutionStatus != StatusResourceLimit || res.Semantic != AssessBlocked || res.Code != "ELIGIBILITY_WORK_LIMIT" || res.Complete || res.ProcessingComplete || res.BaseVerified.Assessment != AssessPass {
		t.Fatalf("resource stop changed a confirmed nonfailure or exposed completion: %+v", res)
	}
}
