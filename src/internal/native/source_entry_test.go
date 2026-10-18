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
