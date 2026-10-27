package kit

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

func TestSemanticEligibilityBorrowedScope(t *testing.T) {
	fixture := newEligibilityFixture(t)
	req := fixture.request()
	b := semanticTestBudget(t)
	if e := b.charge(7, 128, "outer-consumer"); e != nil {
		t.Fatal(e)
	}
	root, err := os.OpenRoot(req.Evidence.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	cache := &semanticNativeCache{}
	scope := &eligibilityScope{r: b.r, root: root, wanted: map[string]bool{}, native: cache}
	result, err := verifyQueryReferenceEligibilityInRun(b.r.caller, req, scope)
	if err != nil || result.Assessment != AssessPass || !result.Complete || scope.session == nil || scope.session.r != b.r || scope.session.b != b || result.Work != b.work || result.LogicalBytes != b.logical {
		t.Fatalf("borrowed eligibility reset or failed: %v %+v", err, result)
	}
	if e := b.r.check(); e != nil {
		t.Fatalf("borrowed operation cancelled outer run: %v", e)
	}
	if scope.session.snapshot == nil || len(scope.session.snapshot.observed) == 0 || !cache.ready || !cache.record.haveRaw || cache.record.rawBytes != uint64(len(fixture.files["native/response.json"])) || len(cache.checked) != 2 || len(cache.versions) != 2 || len(cache.points) != 1 || len(cache.values) != len(scope.session.witness.Required) {
		t.Fatal("shared checked native evidence was discarded")
	}
	for i, tuple := range scope.session.witness.Required {
		if !cache.evaluated[i] || cache.values[i].evaluation != "EVALUATED" || CompareCaptures(cache.values[i].captures, tuple.Expected) != nil {
			t.Fatalf("retained tuple %d lost actual evaluation", i)
		}
	}
}

func TestSemanticProjectionBindsBaseInputsToRaw(t *testing.T) {
	api, predicates := "tsgk-api/r1", "NOT_EVALUATED"
	symbols, fields := uint32(25), uint32(3)
	decls := []nativewire.DeclItem{{Fact: "type", NodeType: "identifier", StartByte: 0, EndByte: 1, Name: &nativewire.NameSpan{StartByte: 0, EndByte: 1}, Status: "PASS"}}
	raw := nativewire.Response{Status: StatusCompleted, Producer: nativewire.Producer{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, Query: "tsgk-query/r1", API: &api, Predicates: &predicates, SymbolCount: &symbols, FieldCount: &fields}, Steps: []nativewire.Step{{Route: &nativewire.Route{EditHasChanges: true, EditedRootEndByte: 3, ReusedNodes: 2}, Incremental: nativewire.WireTree{Declarations: &decls}, Fresh: &nativewire.WireTree{Declarations: &decls}}}}
	control := map[string]any{"execution_status": StatusCompleted, "response_status": StatusCompleted, "response_code": "", "producer": raw.Producer, "steps": []any{map[string]any{"route": map[string]any{"edit_has_changes": true, "edited_root_end_byte": 3, "reused_nodes": 2, "fresh_reused_nodes": 0, "proven": true}, "incremental": map[string]any{"declarations": decls}, "fresh": map[string]any{"declarations": decls}}}}
	for _, mutation := range []string{"none", "language", "query", "symbols", "route-count", "route-proof", "restart", "response-status", "response-code", "declaration-status", "declaration-name", "fresh-declaration"} {
		t.Run(mutation, func(t *testing.T) {
			var p map[string]any
			if err := json.Unmarshal(mustJSON(t, control), &p); err != nil {
				t.Fatal(err)
			}
			step := p["steps"].([]any)[0].(map[string]any)
			producer := p["producer"].(map[string]any)
			route := step["route"].(map[string]any)
			decl := step["incremental"].(map[string]any)["declarations"].([]any)[0].(map[string]any)
			switch mutation {
			case "language":
				producer["language_version"] = 14
			case "query":
				producer["query"] = "FORGED"
			case "symbols":
				producer["symbol_count"] = 26
			case "route-count":
				route["reused_nodes"] = 3
			case "route-proof":
				route["proven"] = false
			case "restart":
				step["restarted"] = true
			case "response-status":
				p["response_status"] = StatusFailed
			case "response-code":
				p["response_code"] = "FORGED"
			case "declaration-status":
				decl["status"] = "FAIL"
			case "declaration-name":
				decl["name"].(map[string]any)["end_byte"] = 2
			case "fresh-declaration":
				step["fresh"].(map[string]any)["declarations"] = []any{}
			}
			b := semanticTestBudget(t)
			data := mustJSON(t, p)
			projection, e := b.semanticAST("projection", data)
			if e != nil {
				t.Fatal(e)
			}
			findings := 0
			e = b.semanticProjectionInputs(raw, projection, uint64(len(mustJSON(t, raw))), func(code, _, _ string) {
				if code != "SEMANTICS_PROJECTION_BASE_INPUT" {
					t.Fatalf("unexpected finding: %s", code)
				}
				findings++
			})
			if e != nil || (findings == 0) != (mutation == "none") {
				t.Fatalf("mutation %s findings=%d error=%v", mutation, findings, e)
			}
		})
	}
}
