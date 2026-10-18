package native

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestNativePredicatePolicy(t *testing.T) {
	b := fixtureBuild(t, "plain")
	src := "a = a;\nb = a;\nb = b;\n"
	single := func(text string, count int) []capText {
		out := []capText{}
		for range count {
			out = append(out, capText{"x", "identifier", text})
		}
		return out
	}
	for _, tc := range []struct {
		name, query string
		want        []capText
	}{
		{"eq", `((identifier) @x (#eq? @x "a"))`, single("a", 3)},
		{"not-eq", `((identifier) @x (#not-eq? @x "a"))`, single("b", 3)},
		{"any-of", `((identifier) @x (#any-of? @x "a" "c"))`, single("a", 3)},
		{"not-any-of", `((identifier) @x (#not-any-of? @x "a" "c"))`, single("b", 3)},
		{"eq-captures", `((assignment name: (identifier) @x value: (identifier) @y) (#eq? @x @y))`, []capText{{"x", "identifier", "a"}, {"y", "identifier", "a"}, {"x", "identifier", "b"}, {"y", "identifier", "b"}}},
		{"not-eq-captures", `((assignment name: (identifier) @x value: (identifier) @y) (#not-eq? @x @y))`, []capText{{"x", "identifier", "b"}, {"y", "identifier", "a"}}},
		{"multiple-predicates", `((assignment name: (identifier) @x value: (identifier) @y) (#eq? @y "a") (#not-eq? @x @y))`, []capText{{"x", "identifier", "b"}, {"y", "identifier", "a"}}},
		{"empty", `((identifier) @x (#eq? @x "none"))`, []capText{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runOracleCase(t, b, oracleContext(false, tc.query), src, nil)
			if result.ExecutionStatus != kit.StatusCompleted || len(result.Steps) != 1 {
				t.Fatalf("native query did not complete: %s %s", result.ExecutionStatus, result.Code)
			}
			out := result.Steps[0].Incremental.Queries[0]
			if out.Status != kit.StatusCompleted || out.Evaluation != EvalEvaluated || out.Captures == nil || !slices.Equal(streamOf(t, out, src), tc.want) {
				t.Fatalf("native evaluated stream: %+v expected %+v", out, tc.want)
			}
		})
	}
	t.Run("quantifier", func(t *testing.T) {
		query := `((source_file (assignment name: (identifier) @x)*) (#eq? @x "a"))`
		result := runOracleCase(t, b, oracleContext(false, query), src, nil)
		if result.ExecutionStatus != kit.StatusCompleted || len(result.Steps) != 1 {
			t.Fatalf("quantified query did not run: %s %s", result.ExecutionStatus, result.Code)
		}
		out := result.Steps[0].Incremental.Queries[0]
		if len(out.Predicates) != 1 || len(out.Predicates[0].Steps) != 3 || out.Predicates[0].Steps[1].Quantifier == nil || *out.Predicates[0].Steps[1].Quantifier != "ZERO_OR_MORE" || out.Code != "QUERY_PREDICATE_UNSUPPORTED:eq?" || out.Status != EvalUnsupported || out.Captures != nil {
			t.Fatalf("native quantified capture adopted: %+v", out)
		}
	})
	t.Run("partial", func(t *testing.T) {
		x := oracleContext(false, `((identifier) @x (#eq? @x "a"))`)
		x.Op.Captures = 1
		result := runOracleCase(t, b, x, src, nil)
		if result.ExecutionStatus != kit.StatusResourceLimit || result.Code != "CAPTURE_LIMIT" || len(result.Steps) != 1 {
			t.Fatalf("partial query did not stop: %s %s", result.ExecutionStatus, result.Code)
		}
		out := result.Steps[0].Incremental.Queries[0]
		if out.Status != kit.StatusResourceLimit || !out.Partial || out.Evaluation != EvalUnsupported || out.Captures != nil || len(out.structural) != 1 {
			t.Fatalf("native partial predicate evaluated: %+v", out)
		}
	})
	for _, encoding := range []string{kit.EncodingUTF16LE, kit.EncodingUTF16BE, kit.EncodingCP949} {
		t.Run(encoding, func(t *testing.T) {
			source := []byte(src) // ASCII is also valid CP949.
			if encoding != kit.EncodingCP949 {
				source = utf16(encoding, src)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			result := b.RunCase(ctx, oracleContext(false, `((identifier) @x (#eq? @x "a"))`), kit.IncrementalCase{ID: "encoding", Encoding: encoding}, source)
			if result.ExecutionStatus != kit.StatusCompleted || len(result.Steps) != 1 {
				t.Fatalf("encoded query did not run: %s %s", result.ExecutionStatus, result.Code)
			}
			out := result.Steps[0].Incremental.Queries[0]
			if out.Status != EvalUnsupported || out.Code != "QUERY_PREDICATE_ENCODING" || out.Evaluation != EvalUnsupported || out.Captures != nil {
				t.Fatalf("native encoding adopted: %+v", out)
			}
		})
	}
}
