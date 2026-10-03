package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const (
	cliBase      = `[{"type":"program","named":true,"root":true,"fields":{"body":{"multiple":true,"required":false,"types":[{"type":"item","named":true}]}}},{"type":"item","named":true},{"type":"item","named":false}]`
	cliCandidate = `[{"type":"program","named":true,"root":true,"fields":{"body":{"multiple":true,"required":true,"types":[{"type":"item","named":true}]}}},{"type":"item","named":true},{"type":"item","named":false},{"type":"extra","named":true}]`
)

// writeSchemas puts each document in its own directory under the same base name, so only
// role and hash tell them apart (S03-A09).
func writeSchemas(t *testing.T, docs ...string) []string {
	t.Helper()
	var out []string
	for _, d := range docs {
		p := filepath.Join(t.TempDir(), "node-types.json")
		if err := os.WriteFile(p, []byte(d), 0o644); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func apiJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + "\n"
}

// S03-A10: the CLI renders exactly the API result and maps the outcome to the exit code;
// nothing beyond the two JSON files (no parser.c, compiler or network) is needed.
func TestSchemaCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	files := writeSchemas(t, cliBase, cliCandidate, `[{"type":"a","named":true},{"type":"a","named":true}]`, `[{"type":"a","named":true,"visible":true}]`)
	base, cand, invalid, unsupported := files[0], files[1], files[2], files[3]
	input := func(p string) kit.SchemaInput {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return kit.SchemaInput{Name: filepath.Base(p), Data: data}
	}
	for _, tc := range []struct {
		args    []string
		code    int
		errCode string
		api     func() (any, error)
	}{
		{[]string{"check", "--input", base}, 0, "", func() (any, error) {
			return kit.SchemaCheck(ctx, kit.SchemaCheckRequest{Input: input(base), Limits: kit.DefaultSchemaLimits()})
		}},
		{[]string{"check", "--input", invalid}, 1, "", func() (any, error) {
			return kit.SchemaCheck(ctx, kit.SchemaCheckRequest{Input: input(invalid), Limits: kit.DefaultSchemaLimits()})
		}},
		{[]string{"check", "--input", unsupported}, 3, "", func() (any, error) {
			return kit.SchemaCheck(ctx, kit.SchemaCheckRequest{Input: input(unsupported), Limits: kit.DefaultSchemaLimits()})
		}},
		{[]string{"diff", "--before", base, "--after", cand}, 1, "", func() (any, error) {
			return kit.SchemaDiff(ctx, kit.SchemaDiffRequest{Baseline: input(base), Candidate: input(cand), Limits: kit.DefaultSchemaLimits()})
		}},
		{[]string{"diff", "--before", cand, "--after", cand}, 0, "", func() (any, error) {
			return kit.SchemaDiff(ctx, kit.SchemaDiffRequest{Baseline: input(cand), Candidate: input(cand), Limits: kit.DefaultSchemaLimits()})
		}},
		{[]string{"diff", "--before", base, "--after", invalid}, 2, "SCHEMA_INVALID", func() (any, error) {
			return kit.SchemaDiff(ctx, kit.SchemaDiffRequest{Baseline: input(base), Candidate: input(invalid), Limits: kit.DefaultSchemaLimits()})
		}},
		{[]string{"diff", "--before", unsupported, "--after", base}, 3, "SCHEMA_KEY_UNSUPPORTED", func() (any, error) {
			return kit.SchemaDiff(ctx, kit.SchemaDiffRequest{Baseline: input(unsupported), Candidate: input(base), Limits: kit.DefaultSchemaLimits()})
		}},
	} {
		code, stdout, stderr := cli(t, ctx, append([]string{"schema"}, tc.args...)...)
		res, err := tc.api()
		if code != tc.code || stdout != apiJSON(t, res) {
			t.Fatalf("%v: exit %d (want %d) stderr %q\nCLI %sAPI %s", tc.args, code, tc.code, stderr, stdout, apiJSON(t, res))
		}
		var code2 string
		if ke, ok := err.(*kit.Error); ok {
			code2 = ke.Code
		} else if err != nil {
			t.Fatal(err)
		}
		if code2 != tc.errCode || (code2 != "" && !strings.Contains(stderr, code2)) {
			t.Fatalf("%v: API error %v vs CLI %q", tc.args, err, stderr)
		}
	}
	// Directional result keeps both same-named inputs apart by role and hash.
	_, stdout, _ := cli(t, ctx, "schema", "diff", "--before", base, "--after", cand)
	var d kit.SchemaDiffResult
	if err := json.Unmarshal([]byte(stdout), &d); err != nil {
		t.Fatal(err)
	}
	if d.Baseline.Name != d.Candidate.Name || d.Baseline.SHA256 == d.Candidate.SHA256 || len(d.Differences) != 2 ||
		d.Differences[0].Code != "NODE_ADDED" || d.Differences[1].Code != "FIELD_REQUIRED_CHANGED" {
		t.Fatalf("diff result %+v", d)
	}
	// An unreadable input is an I/O failure; a complete result can be published once.
	if code, _, stderr := cli(t, ctx, "schema", "check", "--input", filepath.Join(t.TempDir(), "missing.json")); code != 4 || !strings.Contains(stderr, "SCHEMA_UNREADABLE") {
		t.Fatalf("missing input: %d %q", code, stderr)
	}
	out := filepath.Join(t.TempDir(), "report.json")
	if code, _, stderr := cli(t, ctx, "schema", "diff", "--before", base, "--after", cand, "--out", out); code != 1 {
		t.Fatalf("publish: %d %q", code, stderr)
	}
	if data, err := os.ReadFile(out); err != nil || string(data) != stdout {
		t.Fatalf("published report differs: %v", err)
	}
	if code, _, stderr := cli(t, ctx, "schema", "diff", "--before", base, "--after", cand, "--out", out); code != 4 || !strings.Contains(stderr, "OUTPUT_EXISTS") {
		t.Fatalf("clobber: %d %q", code, stderr)
	}
	if code, _, stderr := cli(t, ctx, "schema", "check", "--input", base, "--out", base); code != 4 || !strings.Contains(stderr, "OUTPUT_EXISTS") {
		t.Fatalf("input overwrite: %d %q", code, stderr)
	}
}

// S03-A10/A14: an external module using only the public API gets the same schema results
// and error codes as the CLI binary run with an empty PATH.
func TestExternalConsumerSchema(t *testing.T) {
	bin, consumer := buildCLI(t), buildConsumer(t)
	offline := []string{"PATH=", "SystemRoot=" + os.Getenv("SystemRoot")}
	files := writeSchemas(t, cliBase, cliCandidate, `[]`)
	for _, pair := range [][2]string{{files[0], files[1]}, {files[1], files[0]}, {files[2], files[0]}} {
		_, lines, _ := runBin(t, offline, consumer, "schema", pair[0], pair[1])
		api := map[string][2]string{}
		for _, line := range strings.Split(strings.TrimSpace(lines), "\n") {
			parts := strings.SplitN(line, "\t", 3)
			api[parts[0]] = [2]string{parts[1], parts[2]}
		}
		for name, args := range map[string][]string{
			"schema-check": {"schema", "check", "--input", pair[0]},
			"schema-diff":  {"schema", "diff", "--before", pair[0], "--after", pair[1]},
		} {
			code, stdout, stderr := runBin(t, offline, bin, args...)
			if strings.TrimSuffix(stdout, "\n") != api[name][1] {
				t.Fatalf("%s: CLI and API differ\nCLI %s\nAPI %s", name, stdout, api[name][1])
			}
			if api[name][0] != "-" && (code != 2 || !strings.Contains(stderr, strings.Split(api[name][0], "/")[1])) {
				t.Fatalf("%s guard differs: API %s CLI %d %q", name, api[name][0], code, stderr)
			}
		}
	}
}
