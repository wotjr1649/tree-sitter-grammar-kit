package native

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The current registered state through the completion fan-in: the large scope enters from the
// recorded #412 judgments when TSGK_LARGE_FOLD is set. The predicate scope is run on the
// owned native (this host), the SVC scope is judged offline against its independent expected
// values, and the ordinary scope has no stored result (and the large scope none without records), so
// they are absent. The gate
// must stay closed and the table shows which scope's obligations close it.
func TestNativeQueryCompletionCurrentState(t *testing.T) {
	read := func(path string) []byte {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	registry := read("../../contracts/query-semantic-registry-r1.json")
	predAsset, _, expected := predicateScopeAssets(t)
	svc, inventory := svcScopeAssets(t)
	svcAsset := read("../../contracts/query-svc-expected-r1.json")
	reg, err := queryregistrycore.Parse(registry)
	if err != nil {
		t.Fatal(err)
	}
	platform, subject := predicateHost()
	otherHost, registered := otherQueryHost(platform)
	if !registered {
		t.Skip("the registry judges windows-amd64 and linux-amd64 only; " + platform + " is not registered")
	}

	// predicate scope: 20 EVALUATED tuples on the owned native plus the 47 rejection rows
	b := fixtureBuild(t, "plain")
	ok := map[string]bool{}
	for _, tuple := range expected.Tuples {
		raw, projection := runPredicateTuple(t, b, tuple)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		got, err := kit.VerifyPredicateTuple(ctx, tuple, subject, raw, projection)
		cancel()
		ok[tuple.ID] = err == nil && got.Assessment == kit.AssessPass && got.Evaluated == 1
	}
	for _, p := range reg.Probes {
		if p.Kind == queryregistrycore.Control {
			ok[p.ID] = kit.CheckPredicateControl(p, predicateControlOutcome(t, p)) == nil
		}
	}
	var judgments []queryregistrycore.Judgment
	for _, c := range queryregistrycore.Expand(nil, queryregistrycore.Platforms(), reg.Probes, nil) {
		if c.Platform != platform {
			continue
		}
		j := queryregistrycore.Judgment{ID: c.ID(reg.Namespace), Coordinate: c, State: "VERIFIED"}
		if !ok[c.Case] {
			j.State, j.Code = "FAILED", "PREDICATE_NOT_VERIFIED"
		}
		if c.Kind == queryregistrycore.Control {
			for _, p := range reg.Probes {
				if p.ID == c.Case {
					j.Subject = &queryregistrycore.Subject{Status: p.Status, Code: p.Code, Evaluation: p.Evaluation, Partial: p.Partial, Structural: p.Structural, Expected: p.Expected}
				}
			}
		}
		judgments = append(judgments, j)
	}
	other := map[string]bool{otherHost: true}
	predicate, err := kit.PredicateScopeResult(registry, predAsset, judgments, other)
	if err != nil {
		t.Fatal(err)
	}

	// SVC scope: the offline obligations against the independent expected values
	svcJudgments, err := kit.VerifySvcExpected(svc, inventory)
	if err != nil {
		t.Fatal(err)
	}
	svcScope, err := kit.SvcScopeResult(svc, svcAsset, registry, platform, svcJudgments, other)
	if err != nil {
		t.Fatal(err)
	}
	// opt-in: keep this host's two judgments for the merge of both hosts (TSGK_QUERY_SCOPE_OUT)
	storeQueryScopeHost(t, platform, kit.QueryScopeHostFile(kit.QueryScopePredicate, platform), predicate)
	storeQueryScopeHost(t, platform, kit.QueryScopeHostFile(kit.QueryScopeSvc, platform), svcScope)

	// large scope: the judgments #412 recorded per case (TSGK_LARGE_FOLD, ';' separated record
	// files) and the HARD-cap receipt of the case that could not run in 4 GiB. No native large run
	// happens here; without records the large scope has no stored result and stays absent.
	largeAsset := read("../../contracts/query-large-expected-r1.json")
	largeFixtures := read("../../contracts/native-large-fixtures.json")
	scopes := []kit.QueryScopeResult{predicate, svcScope}
	pins := map[string]string{kit.QueryScopePredicate: predicate.ResultSHA256, kit.QueryScopeSvc: svcScope.ResultSHA256}
	wantVerified := uint64(67 + 263)
	if list := os.Getenv("TSGK_LARGE_FOLD"); list != "" {
		large, err := kit.ParseLargeExpected(largeAsset, registry, read("../../contracts/qualification-c1.json"), largeFixtures)
		if err != nil {
			t.Fatal(err)
		}
		largeScope, err := kit.LargeScopeResult(large, largeAsset, registry, recordedLargeJudgments(t, large, largeFixtures, list))
		if err != nil {
			t.Fatal(err)
		}
		scopes, pins[kit.QueryScopeLarge] = append(scopes, largeScope), largeScope.ResultSHA256
		wantVerified += 30 // #412: 30 VERIFIED, 9 BLOCKED
	}

	req := kit.QueryCompletionRequest{
		Registry: registry, RegistrySHA256: queryregistrycore.Digest(registry),
		Qualification: read("../../contracts/qualification-c1.json"), Slots: read("../../contracts/query-slot-inventory-r1.json"),
		Basis:       map[string][]byte{"src/dev/query-reference/check.py": read("../../dev/query-reference/check.py"), "src/internal/native/query_policy_test.go": read("query_policy_test.go")},
		SvcExpected: svcAsset, SvcFixture: read("../../testdata/native/n461/svc.json"), LargeExpected: largeAsset, LargeFixtures: largeFixtures,
		AssetPins: map[string][]queryregistrycore.Pin{
			kit.QueryScopePredicate: {queryregistrycore.FilePin(kit.PredicateExpectedPath, predAsset)},
			kit.QueryScopeSvc:       {queryregistrycore.FilePin(kit.SvcExpectedPath, svcAsset)},
			kit.QueryScopeLarge:     {queryregistrycore.FilePin(kit.LargeExpectedPath, largeAsset)},
		},
		ResultPins: pins,
		Scopes:     scopes,
	}
	res, err := kit.FoldQueryCompletion(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Counters {
		t.Logf("COUNTER %-9s %-24s %-14s required=%-6d verified=%-6d failed=%d blocked=%-6d unprocessed=%d", c.Scope, c.Kind, c.Platform, c.Required, c.Verified, c.Failed, c.Blocked, c.Unprocessed)
	}
	for _, h := range res.Hosts {
		t.Logf("HOST %-14s required=%d verified=%d failed=%d blocked=%d unprocessed=%d", h.Platform, h.Required, h.Verified, h.Failed, h.Blocked, h.Unprocessed)
	}
	t.Logf("TOTAL required=%d verified=%d failed=%d blocked=%d unprocessed=%d", res.Total.Required, res.Total.Verified, res.Total.Failed, res.Total.Blocked, res.Total.Unprocessed)
	for _, s := range res.Scopes {
		t.Logf("SCOPE %-9s present=%v result=%s not_applicable=%v", s.Scope, s.Present, s.ResultSHA256, s.NotApplicable)
	}
	t.Logf("VERDICT completion=%s code=%s complete=%v unregistered=%v", res.Completion, res.Code, res.Complete, res.UnregisteredScopes)
	if res.Complete || res.Completion == kit.QueryCompletionComplete || res.Total.Failed != 0 {
		t.Fatalf("the gate must stay closed without a FAIL: %+v", res)
	}
	if os.Getenv("TSGK_LARGE_FOLD") != "" {
		var large kit.QueryCompletionCounts
		for _, c := range res.Counters {
			if c.Scope == kit.QueryScopeLarge {
				large.Required, large.Verified, large.Blocked, large.Failed, large.Unprocessed = large.Required+c.Required, large.Verified+c.Verified, large.Blocked+c.Blocked, large.Failed+c.Failed, large.Unprocessed+c.Unprocessed
			}
		}
		if large != (kit.QueryCompletionCounts{Required: 39, Verified: 30, Blocked: 9}) {
			t.Fatalf("large %+v, want the #412 state 39 = 30 VERIFIED + 9 BLOCKED", large)
		}
	}
	if res.Total.Verified != wantVerified {
		t.Fatalf("verified %d, want %d (predicate 67 + SVC 263 + recorded large)", res.Total.Verified, wantVerified)
	}
}

// recordedLargeJudgments reads the per-case records of the large run (the #412 recording format)
// and, for a case that stopped at the task's HARD memory cap, the supervised receipt in
// TSGK_LARGE_CAP_RECEIPT ("case=path"): its source identity is checked offline and its status is
// BLOCKED, never PASS.
func recordedLargeJudgments(t *testing.T, e kit.LargeExpected, fixtures []byte, list string) []kit.LargeJudgment {
	t.Helper()
	var judgments []kit.LargeJudgment
	for _, name := range strings.Split(list, ";") {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var rec struct {
			Judgments []kit.LargeJudgment `json:"judgments"`
		}
		if err := json.Unmarshal(data, &rec); err != nil {
			t.Fatal(err)
		}
		judgments = append(judgments, rec.Judgments...)
	}
	if spec := os.Getenv("TSGK_LARGE_CAP_RECEIPT"); spec != "" {
		id, path, _ := strings.Cut(spec, "=")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var receipt struct {
			Execution struct {
				Status string `json:"status"`
				Reason string `json:"reason"`
				Memory struct {
					Limit       uint64 `json:"limit_bytes"`
					Enforcement string `json:"enforcement"`
					Hit         bool   `json:"limit_hit"`
				} `json:"memory"`
			} `json:"execution"`
		}
		if err := json.Unmarshal(data, &receipt); err != nil {
			t.Fatal(err)
		}
		x := receipt.Execution
		if x.Status != kit.StatusResourceLimit || x.Reason != "MEMORY_LIMIT" || x.Memory.Limit != 4<<30 || x.Memory.Enforcement != "HARD" || !x.Memory.Hit {
			t.Fatalf("receipt is not a HARD 4 GiB memory limit: %+v", x)
		}
		templates, _, err := kit.LargeTemplates(fixtures)
		if err != nil {
			t.Fatal(err)
		}
		for i, c := range e.Cases {
			if c.ID == id {
				source, err := templates[i].Source()
				if err != nil {
					t.Fatal(err)
				}
				judgments = append(judgments, kit.VerifyLargeSource(c, source), kit.LargeJudgment{ID: "LARGE_STATUS/csharp/" + c.ID + "/0", State: "BLOCKED", Code: "LARGE_MEMORY_CAP_4GIB_EXCEEDED"})
			}
		}
	}
	return judgments
}
