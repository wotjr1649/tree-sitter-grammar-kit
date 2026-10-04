package foundation

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// run-routes.ps1 fails the native-routes job on the kit axis only. Query expectation
// claims are requirement results (Q, judged by `tsgk qualify`) and a route the grammar's
// error tree left unobservable is BLOCKED, not failed: both are recorded in
// requirement_results for disposition and never appended to failures. Everything else
// that fails the job today (set verification, error findings, a non-completed case,
// incremental equality FAIL/BLOCKED, incremental route FAIL, query equality, fact
// reproduction and dynamic SQL FAIL/BLOCKED, a build refusal) still does.

// psRouteRun is the pwsh harness: it defines run-routes.ps1's result functions from the
// script's own AST (the script itself builds every route) and feeds them synthetic runs.
const psRouteRun = `param([string]$Script, [string]$Runs)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$ast = [Management.Automation.Language.Parser]::ParseFile($Script, [ref]$null, [ref]$null)
$defs = $ast.FindAll({ param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] }, $true)
foreach ($f in $defs) {
  if ($f.Name -in @('Add-RequirementResult', 'Add-Result', 'Add-OracleResult')) { . ([scriptblock]::Create($f.Extent.Text)) }
}
$summary = [ordered]@{ routes = @(); oracle = @(); api_findings = @(); requirement_results = @(); failures = @() }
$in = Get-Content -LiteralPath $Runs -Raw | ConvertFrom-Json
foreach ($r in @($in.incremental)) { Add-Result $r.label $r.run | Out-Null }
foreach ($r in @($in.oracle)) { Add-OracleResult $r.label $r.run | Out-Null }
ConvertTo-Json -InputObject ([ordered]@{ failures = @($summary.failures); requirement_results = @($summary.requirement_results) }) -Depth 8 -Compress
`

type routeRunSummary struct {
	Failures     []string `json:"failures"`
	Requirements []struct {
		Case     string   `json:"case"`
		Claim    string   `json:"claim"`
		Result   string   `json:"result"`
		Code     string   `json:"code"`
		Failures []string `json:"failures"`
	} `json:"requirement_results"`
}

func rrCase(id, eq, route, code string, oracle map[string]string) map[string]any {
	c := map[string]any{"id": id, "execution_status": "COMPLETED", "assessment": "FAIL", "code": code,
		"claims":       map[string]string{"incremental_equality": eq, "incremental_route": route, "expectations": "NOT_CLAIMED"},
		"expectations": []any{}, "steps": []any{}}
	if oracle != nil {
		o := map[string]string{"query_equality": "NOT_CLAIMED", "query_expectations": "NOT_CLAIMED", "api": "PASS", "fact_reproduction": "NOT_CLAIMED", "dynamic_sql": "NOT_CLAIMED"}
		for k, v := range oracle {
			o[k] = v
		}
		c["oracle_claims"] = o
	}
	return c
}

func TestRunRoutesFailureAxes(t *testing.T) {
	root := repository(t)
	script := filepath.Join(root, "src", "dev", "s05-native", "run-routes.ps1")
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available: run-routes.ps1 result functions not executed (TestRunRoutesFailureAxesGuard still applies)")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "oracle-r")
	// the record of the failing query expectation, read by Add-OracleResult
	rec := `{"query_expectations": [{"query": "q.x", "step": 0, "result": "FAIL", "detail": "capture 0"}]}`
	if err := os.MkdirAll(filepath.Join(out, "records"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "records", "00000-q-fail.json"), []byte(rec), 0o644); err != nil {
		t.Fatal(err)
	}
	const blocked = "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1"
	runs := map[string]any{
		"incremental": []any{map[string]any{"label": "r", "run": map[string]any{"code": 1, "out": filepath.Join(dir, "r"), "res": map[string]any{
			"execution_status": "COMPLETED", "assessment": "FAIL", "build": nil, "findings": []any{}, "cases": []any{
				rrCase("route-fail", "PASS", "FAIL", "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_1", nil),
				rrCase("route-blocked", "PASS", "BLOCKED", blocked, nil),
				rrCase("eq-blocked", "BLOCKED", "PASS", "", nil),
			}}}}},
		"oracle": []any{
			map[string]any{"label": "r", "run": map[string]any{"code": 1, "out": out, "res": map[string]any{
				"execution_status": "COMPLETED", "assessment": "FAIL", "set": map[string]any{"valid": true}, "build": nil, "findings": []any{}, "cases": []any{
					rrCase("q-fail", "NOT_CLAIMED", "NOT_CLAIMED", "ORACLE_CLAIM_FAILED", map[string]string{"query_expectations": "FAIL"}),
					rrCase("q-eq-fail", "PASS", "PASS", "ORACLE_CLAIM_FAILED", map[string]string{"query_equality": "FAIL"}),
					rrCase("dyn-blocked", "NOT_CLAIMED", "NOT_CLAIMED", "", map[string]string{"dynamic_sql": "BLOCKED"}),
					rrCase("route-fail", "PASS", "FAIL", "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_1", map[string]string{"query_equality": "PASS"}),
					rrCase("route-blocked", "PASS", "BLOCKED", blocked, map[string]string{"query_equality": "PASS"}),
				}}}},
			map[string]any{"label": "bad", "run": map[string]any{"code": 1, "out": filepath.Join(dir, "bad"), "res": map[string]any{
				"execution_status": "FAILED", "assessment": "NOT_ASSESSED", "set": map[string]any{"valid": false}, "build": nil,
				"findings": []any{map[string]any{"code": "SET_BROKEN", "severity": "error"}}, "cases": []any{}}}},
		},
	}
	data, _ := json.Marshal(runs)
	in := filepath.Join(dir, "runs.json")
	harness := filepath.Join(dir, "route-run.ps1")
	if err := os.WriteFile(in, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(harness, []byte(psRouteRun), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harness, "-Script", script, "-Runs", in).CombinedOutput()
	if err != nil {
		t.Fatalf("harness: %v: %s", err, raw)
	}
	var s routeRunSummary
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("harness output: %v\n%s", err, raw)
	}
	has := func(prefix string) bool {
		return slices.ContainsFunc(s.Failures, func(f string) bool { return strings.HasPrefix(f, prefix) })
	}
	for _, want := range []string{
		"r/route-fail: incremental PASS/FAIL", "r/eq-blocked: incremental BLOCKED/PASS",
		"oracle r/q-eq-fail: query_equality FAIL", "oracle r/dyn-blocked: dynamic_sql BLOCKED", "oracle r/route-fail: incremental PASS/FAIL",
		"oracle bad: SET_BROKEN", "oracle bad: record set not verified", "oracle bad: FAILED build or refusal",
	} {
		if !has(want) {
			t.Errorf("kit-axis failure %q not recorded: %q", want, s.Failures)
		}
	}
	for _, f := range s.Failures {
		if strings.Contains(f, "query_expectations") || strings.Contains(f, "route-blocked") || strings.Contains(f, "r/q-fail:") {
			t.Errorf("requirement-axis result failed the job: %q", f)
		}
	}
	got := map[string]string{}
	for _, r := range s.Requirements {
		got[r.Case+" "+r.Claim] = r.Result + " " + r.Code + " " + strings.Join(r.Failures, ";")
	}
	want := map[string]string{
		"oracle r/q-fail query_expectations":       "FAIL ORACLE_CLAIM_FAILED q.x step 0: FAIL capture 0",
		"r/route-blocked incremental_route":        "BLOCKED " + blocked + " ",
		"oracle r/route-blocked incremental_route": "BLOCKED " + blocked + " ",
	}
	if len(got) != len(want) {
		t.Errorf("requirement_results %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("requirement_results[%s] = %q, want %q", k, got[k], v)
		}
	}
}

// TestRunRoutesFailureAxesGuard is the static half for hosts without pwsh: the oracle
// claim loop that appends to failures leaves out query_expectations, and a route claim
// fails the job only when it is FAIL.
func TestRunRoutesFailureAxesGuard(t *testing.T) {
	root := repository(t)
	routes := string(mustRead(t, filepath.Join(root, "src", "dev", "s05-native", "run-routes.ps1")))
	if !strings.Contains(routes, "foreach ($k in @('query_equality', 'fact_reproduction', 'dynamic_sql'))") {
		t.Error("the oracle failure loop does not list exactly query_equality, fact_reproduction and dynamic_sql")
	}
	if strings.Contains(routes, "incremental_route -in @('FAIL', 'BLOCKED')") || strings.Count(routes, "$c.claims.incremental_route -eq 'FAIL'") != 2 {
		t.Error("an incremental route claim must fail the job when FAIL, and only then, in both result functions")
	}
	if strings.Count(routes, "Add-RequirementResult ") != 2 {
		t.Error("both result functions must record requirement results")
	}
}
