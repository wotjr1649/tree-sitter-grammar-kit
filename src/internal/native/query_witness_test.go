package native

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// Anchors below come from the owned plain grammar and the literal query/source,
// never from the producer's metadata, structural stream or evaluated output.
func TestNativeStoredQueryWitness(t *testing.T) {
	b := fixtureBuild(t, "plain")
	one := "ONE"
	x := kit.QueryPredicateStep{Kind: "capture", Value: "x", Quantifier: &one}
	literal := func(value string) kit.QueryPredicateStep { return kit.QueryPredicateStep{Kind: "string", Value: value} }
	for _, tc := range []struct {
		name, clause string
		predicates   []kit.QueryPredicate
		keep         [2]bool
		blocked      bool
	}{
		{"structural", "", []kit.QueryPredicate{}, [2]bool{true, true}, false},
		{"eq", ` (#eq? @x "x")`, []kit.QueryPredicate{{Steps: []kit.QueryPredicateStep{literal("eq?"), x, literal("x")}}}, [2]bool{true, false}, false},
		{"not-eq", ` (#not-eq? @x "x")`, []kit.QueryPredicate{{Steps: []kit.QueryPredicateStep{literal("not-eq?"), x, literal("x")}}}, [2]bool{false, true}, false},
		{"any-of", ` (#any-of? @x "x" "z")`, []kit.QueryPredicate{{Steps: []kit.QueryPredicateStep{literal("any-of?"), x, literal("x"), literal("z")}}}, [2]bool{true, false}, false},
		{"not-any-of", ` (#not-any-of? @x "x" "z")`, []kit.QueryPredicate{{Steps: []kit.QueryPredicateStep{literal("not-any-of?"), x, literal("x"), literal("z")}}}, [2]bool{false, true}, false},
		{"capture-pair", ` (#eq? @x @x)`, []kit.QueryPredicate{{Steps: []kit.QueryPredicateStep{literal("eq?"), x, x}}}, [2]bool{true, true}, false},
		{"multiple", ` (#eq? @x "x") (#not-eq? @x "z")`, []kit.QueryPredicate{{Steps: []kit.QueryPredicateStep{literal("eq?"), x, literal("x")}}, {Steps: []kit.QueryPredicateStep{literal("not-eq?"), x, literal("z")}}}, [2]bool{true, false}, false},
		{"unsupported", ` (#match? @x "^x")`, []kit.QueryPredicate{{Steps: []kit.QueryPredicateStep{literal("match?"), x, literal("^x")}}}, [2]bool{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte("x;\n1;\n")
			query := "((identifier) @x" + tc.clause + ")"
			c := kit.IncrementalCase{ID: "oracle", Encoding: kit.EncodingUTF8,
				Input: kit.NativeInput{Path: "case.txt", Role: "case", Bytes: uint64(len(source)), SHA256: sha(source)},
				Edits: []kit.Edit{{StartByte: 0, OldEndByte: 1, NewEndByte: 1, Old: []byte("x"), New: []byte("y")}}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			result := b.RunCase(ctx, oracleContext(false, query), c, source)
			projection, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			anchor := kit.Capture{Match: 0, Pattern: 0, Capture: 0, Name: "x", Node: 2, Type: "identifier", Named: true, EndByte: 1, EndPoint: kit.Point{Column: 1}}
			registration := kit.QueryWitnessRegistration{Schema: kit.QueryWitnessRegistrationSchema, Policy: PredicatePolicy, Case: c.ID,
				Route: "owned-plain", Platform: runtime.GOOS + "/" + runtime.GOARCH, Role: "owned", Encoding: c.Encoding, Input: c.Input, Edits: c.Edits,
				Queries: []kit.QueryWitnessQuery{{ID: "q0", Source: query, Metadata: &kit.QueryWitnessMetadata{Patterns: 1, CaptureNames: []string{"x"}, Predicates: tc.predicates}}}, Required: []kit.QueryWitnessTuple{}}
			for _, tuple := range []struct {
				step uint32
				role string
			}{{0, "incremental"}, {1, "incremental"}, {1, "fresh"}} {
				expected := []kit.Capture{}
				if tc.keep[tuple.step] {
					expected = append(expected, anchor)
				}
				registration.Required = append(registration.Required, kit.QueryWitnessTuple{Query: "q0", Step: tuple.step, TreeRole: tuple.role, Structural: []kit.Capture{anchor}, Expected: expected})
			}
			registered, err := json.Marshal(registration)
			if err != nil {
				t.Fatal(err)
			}
			got, err := kit.VerifyQueryWitness(ctx, kit.QueryWitnessRequest{Registration: registered, Source: source, Raw: result.Raw, Projection: projection})
			assessment, verified, evaluated := kit.AssessPass, 3, 3
			if tc.name == "structural" {
				evaluated = 0
			}
			if tc.blocked {
				assessment, verified, evaluated = kit.AssessBlocked, 0, 0
			}
			if err != nil || got.Assessment != assessment || got.Verified != verified || got.Evaluated != evaluated {
				t.Fatalf("native witness %s: %+v %v; producer %s/%s/%s", tc.name, got, err, result.ExecutionStatus, result.Assessment, result.Code)
			}
		})
	}
	for _, limit := range []string{"capture-limit", "tree-limit", "summary"} {
		t.Run(limit, func(t *testing.T) {
			source := []byte("x;\n")
			query := "(identifier) @x"
			c := kit.IncrementalCase{ID: "limited", Encoding: kit.EncodingUTF8,
				Input: kit.NativeInput{Path: "case.txt", Role: "case", Bytes: uint64(len(source)), SHA256: sha(source)},
				Edits: []kit.Edit{{StartByte: 3, OldEndByte: 3, NewEndByte: 6, Old: []byte{}, New: []byte("y;\n")}}}
			x := oracleContext(false, query)
			if limit == "capture-limit" {
				x.Op.Captures = 1
			} else if limit == "summary" {
				x.Output, x.Op.FullNodes = kit.OutputAuto, 1
				c.Edits = []kit.Edit{}
			} else {
				x.Op.Nodes, x.Op.FullNodes = 4, 4
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			result := b.RunCase(ctx, x, c, source)
			response, err := DecodeResponse(result.Raw)
			status, completed, verified := kit.StatusResourceLimit, uint64(1), 1
			if limit == "summary" {
				status, completed, verified = kit.StatusCompleted, 1, 0
			}
			if err != nil || result.ExecutionStatus != status || len(response.Steps) != len(c.Edits)+1 || response.StepsCompleted != completed {
				t.Fatalf("real retained incomplete step: %+v %v", response, err)
			}
			projection, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			anchor := kit.Capture{Match: 0, Pattern: 0, Capture: 0, Name: "x", Node: 2, Type: "identifier", Named: true, EndByte: 1, EndPoint: kit.Point{Column: 1}}
			required := []kit.QueryWitnessTuple{{Query: "q0", TreeRole: "incremental", Structural: []kit.Capture{anchor}, Expected: []kit.Capture{anchor}}}
			if len(c.Edits) != 0 {
				required = append(required, kit.QueryWitnessTuple{Query: "q0", Step: 1, TreeRole: "incremental"}, kit.QueryWitnessTuple{Query: "q0", Step: 1, TreeRole: "fresh"})
			}
			registration, err := json.Marshal(kit.QueryWitnessRegistration{Schema: kit.QueryWitnessRegistrationSchema, Policy: PredicatePolicy, Case: c.ID,
				Route: "owned-plain", Platform: runtime.GOOS + "/" + runtime.GOARCH, Role: "owned", Encoding: c.Encoding, Input: c.Input, Edits: c.Edits,
				Queries:  []kit.QueryWitnessQuery{{ID: "q0", Source: query, Metadata: &kit.QueryWitnessMetadata{Patterns: 1, CaptureNames: []string{"x"}, Predicates: []kit.QueryPredicate{}}}},
				Required: required})
			if err != nil {
				t.Fatal(err)
			}
			got, err := kit.VerifyQueryWitness(ctx, kit.QueryWitnessRequest{Registration: registration, Source: source, Raw: result.Raw, Projection: projection})
			if err != nil || got.Assessment != kit.AssessBlocked || got.Verified != verified {
				t.Fatalf("real incomplete witness: %+v %v", got, err)
			}
		})
	}
	t.Run("empty-source", func(t *testing.T) {
		source := []byte{}
		query := "(source_file) @x"
		c := kit.IncrementalCase{ID: "oracle", Encoding: kit.EncodingUTF8, Edits: []kit.Edit{}, Input: kit.NativeInput{Path: "empty.txt", Role: "case", SHA256: sha(source)}}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		result := b.RunCase(ctx, oracleContext(false, query), c, source)
		projection, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		registration, err := json.Marshal(kit.QueryWitnessRegistration{Schema: kit.QueryWitnessRegistrationSchema, Policy: PredicatePolicy, Case: c.ID,
			Route: "owned-plain", Platform: runtime.GOOS + "/" + runtime.GOARCH, Role: "owned", Encoding: c.Encoding, Input: c.Input, Edits: c.Edits,
			Queries: []kit.QueryWitnessQuery{{ID: "q0", Source: query, Metadata: &kit.QueryWitnessMetadata{Patterns: 1, CaptureNames: []string{"x"}, Predicates: []kit.QueryPredicate{}}}},
			// Pinned query.c excludes a capture ending at the default start byte0.
			Required: []kit.QueryWitnessTuple{{Query: "q0", TreeRole: "incremental", Structural: []kit.Capture{}, Expected: []kit.Capture{}}}})
		if err != nil {
			t.Fatal(err)
		}
		got, err := kit.VerifyQueryWitness(ctx, kit.QueryWitnessRequest{Registration: registration, Source: source, Raw: result.Raw, Projection: projection})
		if err != nil || got.Assessment != kit.AssessPass || got.Verified != 1 {
			t.Fatalf("present empty source: %+v %v; captures %+v; nodes %+v", got, err, result.Steps[0].Incremental.Queries[0].Captures, result.Steps[0].Incremental.Tree.Nodes)
		}
	})
}
