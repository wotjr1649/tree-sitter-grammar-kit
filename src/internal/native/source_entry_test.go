package native

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestInputRootsRefuseBeforeOutput(t *testing.T) {
	oracle, _ := oracleProfile(t, nil, "a = 1;\n")
	var profile map[string]any
	if err := json.Unmarshal(oracle, &profile); err != nil {
		t.Fatal(err)
	}
	profile["schema"], profile["operation"] = "tsgk-incremental/r2", "native-parse-edit"
	for _, key := range []string{"queries", "fact_pack", "api"} {
		delete(profile, key)
	}
	for _, item := range profile["cases"].([]any) {
		delete(item.(map[string]any), "query_expect")
		delete(item.(map[string]any), "dynamic_sql_expect")
	}
	incremental, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"incremental", "oracle"} {
		for _, mode := range []string{"missing-case", "missing-grammar", "closed-case", "closed-grammar"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				cases, err := os.OpenRoot(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				defer cases.Close()
				grammar, err := os.OpenRoot(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				defer grammar.Close()
				binding := &InputRoots{Cases: cases, Grammar: grammar}
				switch mode {
				case "missing-case":
					binding.Cases = nil
				case "missing-grammar":
					binding.Grammar = nil
				case "closed-case":
					if err := cases.Close(); err != nil {
						t.Fatal(err)
					}
				case "closed-grammar":
					if err := grammar.Close(); err != nil {
						t.Fatal(err)
					}
				}
				out := filepath.Join(t.TempDir(), "must-not-exist")
				allow := []string{AllowBuild, AllowExec}
				var runErr error
				var report kit.Report
				if kind == "incremental" {
					result, err := Incremental(context.Background(), IncrementalRequest{Profile: incremental, Inputs: binding, Out: out, Allow: allow})
					report, runErr = result.Report, err
				} else {
					result, err := Oracle(context.Background(), OracleRequest{Profile: oracle, Inputs: binding, Out: out, Allow: allow})
					report, runErr = result.Report, err
				}
				var e *Error
				if !errors.As(runErr, &e) || e.Code != "INPUT_ROOT_UNAVAILABLE" || report.ExecutionStatus != kit.StatusNotRun {
					t.Fatalf("partial/closed binding accepted: %v %+v", runErr, report)
				}
				if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("refusal created output: %v", err)
				}
			})
		}
	}
}

func TestOracleBoundInputsAndPredicates(t *testing.T) {
	source := "a = 1;\nb = 2;\n"
	profile, root := oracleProfile(t, []kit.OracleQuery{{ID: "filtered", Source: `((identifier) @x (#eq? @x "a"))`}}, source)
	out := filepath.Join(t.TempDir(), "set")
	req := oracleRequest(t, profile, root, out)
	cases, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer cases.Close()
	grammar, err := os.OpenRoot(req.GrammarRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer grammar.Close()
	req.Inputs = &InputRoots{Cases: cases, Grammar: grammar}
	req.Root, req.GrammarRoot = t.TempDir(), t.TempDir() // Valid empty paths cannot supply either input.
	result, err := runOracle(t, req)
	if err != nil || result.ExecutionStatus != kit.StatusCompleted || result.Set == nil || !result.Set.Valid || result.Set.Records != 1 {
		t.Fatalf("bound oracle did not complete: %v %+v", err, result)
	}
	var record OracleRecord
	data, err := os.ReadFile(filepath.Join(out, "records", "00000-ca.json"))
	if err != nil || json.Unmarshal(data, &record) != nil || !record.Complete || len(record.Steps) != 1 || record.Steps[0].Incremental == nil || len(record.Steps[0].Incremental.Queries) != 1 {
		t.Fatalf("bound oracle record: %v %s", err, data)
	}
	query := record.Steps[0].Incremental.Queries[0]
	if query.Status != kit.StatusCompleted || query.Evaluation != EvalEvaluated || len(query.Captures) != 1 {
		t.Fatalf("bound predicate not evaluated: %+v", query)
	}
	got := query.Captures[0]
	if got.Name != "x" || got.Type != "identifier" || got.StartByte != 0 || got.EndByte != 1 || source[got.StartByte:got.EndByte] != "a" || record.Steps[0].SourceSHA256 != record.Input.SHA256 {
		t.Fatalf("bound capture/source changed: %+v", got)
	}
	for _, borrowed := range []*os.Root{cases, grammar} {
		if _, err := borrowed.Stat("."); err != nil {
			t.Fatalf("native closed caller input root: %v", err)
		}
	}
}
