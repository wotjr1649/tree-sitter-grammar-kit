package native

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The current registered state through the completion fan-in: the predicate scope is run on the
// owned native (this host), the SVC scope is judged offline against its independent expected
// values, and the ordinary and large scopes have no stored result, so they are absent. The gate
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
	if platform != "windows-amd64" {
		t.Skip("the registered current state is judged on windows-amd64")
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
	other := map[string]bool{"linux-amd64": true}
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

	req := kit.QueryCompletionRequest{
		Registry: registry, RegistrySHA256: queryregistrycore.Digest(registry),
		Qualification: read("../../contracts/qualification-c1.json"), Slots: read("../../contracts/query-slot-inventory-r1.json"),
		Basis:       map[string][]byte{"src/dev/query-reference/check.py": read("../../dev/query-reference/check.py"), "src/internal/native/query_policy_test.go": read("query_policy_test.go")},
		SvcExpected: svcAsset, SvcFixture: read("../../testdata/native/n461/svc.json"),
		AssetPins: map[string][]queryregistrycore.Pin{
			kit.QueryScopePredicate: {queryregistrycore.FilePin(kit.PredicateExpectedPath, predAsset)},
			kit.QueryScopeSvc:       {queryregistrycore.FilePin(kit.SvcExpectedPath, svcAsset)},
		},
		ResultPins: map[string]string{kit.QueryScopePredicate: predicate.ResultSHA256, kit.QueryScopeSvc: svcScope.ResultSHA256},
		Scopes:     []kit.QueryScopeResult{predicate, svcScope},
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
		t.Logf("SCOPE %-9s present=%v result=%s", s.Scope, s.Present, s.ResultSHA256)
	}
	t.Logf("VERDICT completion=%s code=%s complete=%v unregistered=%v", res.Completion, res.Code, res.Complete, res.UnregisteredScopes)
	if res.Complete || res.Completion == kit.QueryCompletionComplete || res.Total.Failed != 0 {
		t.Fatalf("the gate must stay closed without a FAIL: %+v", res)
	}
	if res.Total.Verified != 67+263 {
		t.Fatalf("verified %d, want predicate 67 + SVC 263", res.Total.Verified)
	}
}
