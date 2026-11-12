package kit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func ordinaryFullPosVerify(t *testing.T, ctx context.Context, req OrdinaryQuerySemanticsRequest) (OrdinaryQuerySemanticsResult, error) {
	t.Helper()
	c, stop := context.WithTimeout(ctx, ReplayOperations()["evidence-replay"].Wall)
	defer stop()
	return VerifyOrdinaryQuerySemantics(c, req)
}

// ordinaryFullPosRequireAllVerified checks the public result, then that the r9 replay and `tsgk replay` agree with it.
func ordinaryFullPosRequireAllVerified(t *testing.T, ctx context.Context, name string, in ordinaryFullPosInput, req OrdinaryQuerySemanticsRequest, required int) {
	t.Helper()
	api, err := ordinaryFullPosVerify(t, ctx, req)
	if err != nil || api.Assessment != AssessPass || api.Semantic != AssessPass || api.BaseVerified.Assessment != AssessPass || !api.Complete || !api.ProcessingComplete || !api.BindingValid || api.FullScopeComplete ||
		api.Tuple.Required != uint64(required) || api.Tuple.Verified != uint64(required) || api.Tuple.Failed != 0 || api.Tuple.Blocked != 0 || api.Tuple.Unprocessed != 0 || len(api.SelectedObligationIDs) != required {
		t.Fatalf("%s: public ordinary API over the owned r8: err=%v result=%+v", name, err, api)
	}
	env := ordinaryFullPosBuildCLI(t)
	replay, cli := ordinaryFullPosParity(t, ctx, env, req, api)
	t.Logf("%s: shards=%v tuples=%+v coverage=%s", name, in.ShardIDs, api.Tuple, api.CoverageSHA256)
	t.Logf("%s: budget read/work/logical API=%d/%d/%d replay=%d/%d/%d cli=%d/%d/%d", name, api.BytesRead, api.Work, api.LogicalBytes,
		replay.BytesRead, replay.Work, replay.LogicalBytes, cli.BytesRead, cli.Work, cli.LogicalBytes)
}

// ordinaryFullPosRequireNotVerified runs a counterexample through the public API. It may stop with an error,
// block or fail some tuples, but it must never report every tuple VERIFIED, and its counters must add up.
func ordinaryFullPosRequireNotVerified(t *testing.T, ctx context.Context, name string, req OrdinaryQuerySemanticsRequest) OrdinaryQuerySemanticsResult {
	t.Helper()
	result, err := ordinaryFullPosVerify(t, ctx, req)
	code := result.Code
	if err != nil {
		code = asError(err).Code
	}
	if result.Semantic == AssessPass || result.Assessment == AssessPass || result.Complete && result.Tuple.Verified == result.Tuple.Required && result.Tuple.Required != 0 ||
		result.Tuple.Verified+result.Tuple.Failed+result.Tuple.Blocked+result.Tuple.Unprocessed > result.Tuple.Required {
		t.Fatalf("%s: the public API accepted the counterexample: err=%v result=%+v", name, err, result)
	}
	t.Logf("counterexample %-34s -> code=%s semantic=%s verified=%d/%d failed=%d blocked=%d unprocessed=%d complete=%t", name, code, result.Semantic,
		result.Tuple.Verified, result.Tuple.Required, result.Tuple.Failed, result.Tuple.Blocked, result.Tuple.Unprocessed, result.Complete)
	return result
}

func ordinaryFullPosDropReference(refs []ordinaryE2EReference, caseID string, step uint32) []ordinaryE2EReference {
	return slices.DeleteFunc(slices.Clone(refs), func(r ordinaryE2EReference) bool { return r.Case == caseID && r.Step == step })
}

func ordinaryFullPosSwapReferences(t *testing.T, refs []ordinaryE2EReference, caseA string, stepA uint32, caseB string, stepB uint32) []ordinaryE2EReference {
	t.Helper()
	out := slices.Clone(refs)
	i := slices.IndexFunc(out, func(r ordinaryE2EReference) bool { return r.Case == caseA && r.Step == stepA })
	j := slices.IndexFunc(out, func(r ordinaryE2EReference) bool { return r.Case == caseB && r.Step == stepB })
	if i < 0 || j < 0 || i == j {
		t.Fatalf("the swap needs two distinct reference rows: %d %d", i, j)
	}
	out[i], out[j] = out[j], out[i]
	return out
}

// TestOrdinaryFullPositiveOwnedJSONZeroEditMultiCase records two zero-edit JSON shards in one owned-native run
// from tracked files only and requires the API, the r9 replay and `tsgk replay` to agree on 2/2 VERIFIED.
func TestOrdinaryFullPositiveOwnedJSONZeroEditMultiCase(t *testing.T) {
	t.Parallel() // owned-native run in its own temporary directories and producer process; no hook or environment write
	ordinaryFullPosNativeSetup(t)
	grammarRoot := ordinaryFullPosGrammarRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	caseIDs := []string{"alt-json-B01-05", "alt-json-B01-09"}
	in := ordinaryFullPosJSONInput(t, grammarRoot, caseIDs, ordinaryFullPosMutation{})
	req, producerDir := ordinaryFullPosProduce(t, in)
	ordinaryFullPosRequireAllVerified(t, ctx, "zero-edit multi-case", in, req, 2)

	// Case substitution: the two documents swap their independent expectations. Every pin stays consistent,
	// so only the comparison with the actual native tree can object.
	swapped := ordinaryFullPosJSONInput(t, grammarRoot, caseIDs, ordinaryFullPosMutation{Cases: func(cases []ordinaryFullPosCase) {
		cases[0].Trees, cases[1].Trees = cases[1].Trees, cases[0].Trees
		cases[0].Captures, cases[1].Captures = cases[1].Captures, cases[0].Captures
	}})
	swappedReq, _ := ordinaryFullPosProduce(t, swapped)
	result := ordinaryFullPosRequireNotVerified(t, ctx, "expected trees swapped between cases", swappedReq)
	if result.Tuple.Failed != 2 || result.Tuple.Verified != 0 {
		t.Fatalf("both substituted tuples must FAIL against the native tree: %+v", result.Tuple)
	}

	// Reference rows for the two cases in the opposite order: the selection is ordered by the registration.
	refs := ordinaryFullPosSwapReferences(t, in.References, caseIDs[0], 0, caseIDs[1], 0)
	ordinaryFullPosRequireNotVerified(t, ctx, "reference rows of the cases reordered",
		ordinaryE2EAssemble(t, in.Request, producerDir, refs, in.AssetBodies))
	ordinaryFullPosRequireNotVerified(t, ctx, "reference row of one case missing",
		ordinaryE2EAssemble(t, in.Request, producerDir, ordinaryFullPosDropReference(in.References, caseIDs[1], 0), in.AssetBodies))
}

// TestOrdinaryFullPositiveOwnedJSONHistoryMultiCase records json-feature-r1 and json-query-r1: two cases with the
// same three-edit, four-version history, so seven tuples each (incremental step 0, incremental and fresh for
// steps 1..3), all expected from the grammar. It checks 14/14 VERIFIED through the API, the r9 replay and
// `tsgk replay`, then the version-order, step and case counterexamples.
func TestOrdinaryFullPositiveOwnedJSONHistoryMultiCase(t *testing.T) {
	t.Parallel() // owned-native run in its own temporary directories and producer process; no hook or environment write
	ordinaryFullPosNativeSetup(t)
	grammarRoot := ordinaryFullPosGrammarRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	caseIDs := []string{"json-feature-r1", "json-query-r1"}
	in := ordinaryFullPosJSONInput(t, grammarRoot, caseIDs, ordinaryFullPosMutation{})
	if len(in.Cases[0].Versions) != 4 || len(in.Cases[1].Versions) != 4 {
		t.Fatalf("the history cases must have four versions: %d %d", len(in.Cases[0].Versions), len(in.Cases[1].Versions))
	}
	req, producerDir := ordinaryFullPosProduce(t, in)
	ordinaryFullPosRequireAllVerified(t, ctx, "edit-history multi-case", in, req, 14)

	// Step missing: the step-2 reference row of the first case is absent.
	ordinaryFullPosRequireNotVerified(t, ctx, "step 2 reference row missing",
		ordinaryE2EAssemble(t, in.Request, producerDir, ordinaryFullPosDropReference(in.References, caseIDs[0], 2), in.AssetBodies))
	// Version order: the step-1 and step-2 rows swap places.
	ordinaryFullPosRequireNotVerified(t, ctx, "reference rows of steps 1 and 2 swapped",
		ordinaryE2EAssemble(t, in.Request, producerDir, ordinaryFullPosSwapReferences(t, in.References, caseIDs[0], 1, caseIDs[0], 2), in.AssetBodies))
	// Case substitution by order: a row of the second case takes the first case's place.
	ordinaryFullPosRequireNotVerified(t, ctx, "reference rows of the cases swapped",
		ordinaryE2EAssemble(t, in.Request, producerDir, ordinaryFullPosSwapReferences(t, in.References, caseIDs[0], 0, caseIDs[1], 0), in.AssetBodies))

	// Version order in the expectation: steps 1 and 2 of the first case exchange their trees and captures.
	versionSwapped := ordinaryFullPosJSONInput(t, grammarRoot, caseIDs, ordinaryFullPosMutation{Cases: func(cases []ordinaryFullPosCase) {
		cases[0].Trees[1], cases[0].Trees[2] = cases[0].Trees[2], cases[0].Trees[1]
		cases[0].Captures[1], cases[0].Captures[2] = cases[0].Captures[2], cases[0].Captures[1]
	}})
	versionReq, _ := ordinaryFullPosProduce(t, versionSwapped)
	result := ordinaryFullPosRequireNotVerified(t, ctx, "expected versions 1 and 2 swapped", versionReq)
	if result.Tuple.Failed != 4 || result.Tuple.Verified != 10 || result.Tuple.Blocked != 0 {
		t.Fatalf("exactly steps 1 and 2 of the first case (two roles each) must FAIL and the other ten tuples verify: %+v", result.Tuple)
	}

	// Case substitution in the registration: the second case's required rows name the first case's assets.
	substituted := ordinaryFullPosJSONInput(t, grammarRoot, caseIDs, ordinaryFullPosMutation{Required: func(required *OrdinaryRequired) {
		if required.Coordinate.Case == caseIDs[1] && required.TreeAsset != "t" {
			required.TreeAsset = ordinaryFullPosTreeAsset(caseIDs[0], int(required.Coordinate.Step), required.Coordinate.TreeRole)
			required.CaptureAsset = ordinaryFullPosCaptureAsset(caseIDs[0], int(required.Coordinate.Step), required.Coordinate.TreeRole)
		}
	}})
	substitutedReq, _ := ordinaryFullPosProduce(t, substituted)
	result = ordinaryFullPosRequireNotVerified(t, ctx, "required rows use another case's assets", substitutedReq)
	if result.Tuple.Failed != 7 || result.Tuple.Verified != 7 || result.Tuple.Blocked != 0 {
		t.Fatalf("exactly the seven tuples of the substituted case must FAIL: %+v", result.Tuple)
	}
}

// ordinaryFullPosSelect rewrites the selected shard IDs in the caller selection and in the approval so that both
// stay equal. The plan commitment, the r8 manifest and the run binding keep the original selection.
func ordinaryFullPosSelect(t *testing.T, req OrdinaryQuerySemanticsRequest, change func(ids []string) []string, proofs func(proofs [][]byte) [][]byte) OrdinaryQuerySemanticsRequest {
	t.Helper()
	var approval eligibilityApproval
	if err := json.Unmarshal(req.EligibilityApproval, &approval); err != nil || approval.Ordinary == nil {
		t.Fatalf("the assembled approval has no ordinary binding: %v", err)
	}
	ids := change(slices.Clone(req.Selection.ShardIDs))
	req.Selection.ShardIDs = ids
	approval.Ordinary.ShardIDs = slices.Clone(ids)
	req.EligibilityApproval = eligibilityJSON(t, approval)
	if proofs != nil {
		req.ShardProofs = proofs(slices.Clone(req.ShardProofs))
	}
	return req
}

// TestOrdinaryFullPositiveOwnedJSONShardOrderDiffers selects shards whose plan order (ascending shard ID,
// the commitment order) differs from their registration order (the P0 workload order the producer follows):
// the zero-edit alt-json-B01-05 sorts before json-feature-r1 but is registered after it. The r8 must keep the
// cases in registration order and the selected shard IDs in plan order, bound by ID, and the public API, the
// r9 replay and `tsgk replay` must agree on 8/8 VERIFIED.
func TestOrdinaryFullPositiveOwnedJSONShardOrderDiffers(t *testing.T) {
	t.Parallel() // owned-native run in its own temporary directories and producer process; no hook or environment write
	ordinaryFullPosNativeSetup(t)
	grammarRoot := ordinaryFullPosGrammarRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	caseIDs := []string{"alt-json-B01-05", "json-feature-r1"}
	in := ordinaryFullPosJSONInput(t, grammarRoot, caseIDs, ordinaryFullPosMutation{})
	registered := []string{in.Cases[0].ID, in.Cases[1].ID}
	byPlan := []string{ordinaryPlanShardKey("json", "windows-amd64", caseIDs[0]), ordinaryPlanShardKey("json", "windows-amd64", caseIDs[1])}
	if !slices.Equal(registered, []string{"json-feature-r1", "alt-json-B01-05"}) || !slices.Equal(in.ShardIDs, byPlan) || !slices.IsSorted(in.ShardIDs) {
		t.Fatalf("the premise changed: the registration order must be the reverse of the plan order: cases=%v shards=%v", registered, in.ShardIDs)
	}
	req, producerDir := ordinaryFullPosProduce(t, in)

	var manifest OrdinaryOracleManifest
	data, err := os.ReadFile(filepath.Join(producerDir, "r8", "manifest.json"))
	if err != nil || json.Unmarshal(data, &manifest) != nil {
		t.Fatalf("reading the owned r8 manifest: %v", err)
	}
	if !slices.Equal(manifest.Cases, registered) || !slices.Equal(manifest.ShardIDs, byPlan) {
		t.Fatalf("the r8 must list cases in registration order and shard IDs in plan order: cases=%v shards=%v", manifest.Cases, manifest.ShardIDs)
	}
	ordinaryFullPosRequireAllVerified(t, ctx, "shard order differs from registration order", in, req, 8)

	// Same shard-ID set, different binding: every variant must be refused by the public API.
	swap := func(ids []string) []string { ids[0], ids[1] = ids[1], ids[0]; return ids }
	reverse := func(proofs [][]byte) [][]byte { proofs[0], proofs[1] = proofs[1], proofs[0]; return proofs }
	ordinaryFullPosRequireNotVerified(t, ctx, "selected IDs in registration order", ordinaryFullPosSelect(t, req, swap, nil))
	ordinaryFullPosRequireNotVerified(t, ctx, "IDs and proofs in registration order", ordinaryFullPosSelect(t, req, swap, reverse))
	ordinaryFullPosRequireNotVerified(t, ctx, "duplicated shard ID", ordinaryFullPosSelect(t, req, func(ids []string) []string { ids[1] = ids[0]; return ids }, nil))
	ordinaryFullPosRequireNotVerified(t, ctx, "one shard ID omitted", ordinaryFullPosSelect(t, req, func(ids []string) []string { return ids[:1] }, func(p [][]byte) [][]byte { return p[:1] }))
	ordinaryFullPosRequireNotVerified(t, ctx, "shard ID of another case substituted", ordinaryFullPosSelect(t, req,
		func(ids []string) []string {
			ids[1] = ordinaryPlanShardKey("json", "windows-amd64", "alt-json-B01-09")
			return ids
		}, nil))
}

// TestOrdinaryBatchStdoutBudgetCountsEveryFrame pins the multi-case rule: the producer budgets one
// OutputBytes+4 response frame per case, so a batch of two or more cases must not be held to a single frame.
func TestOrdinaryBatchStdoutBudgetCountsEveryFrame(t *testing.T) {
	const output = uint64(16 << 20)
	for frames, want := range map[int]uint64{1: output + 4, 2: 2 * (output + 4), 15: 15 * (output + 4)} {
		if got := ordinaryBatchStdoutBudget(output, frames); got != want {
			t.Fatalf("%d frames: budget %d, want %d", frames, got, want)
		}
	}
}
