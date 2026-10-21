package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// Fixtures are synthetic public data. Their construction is outside measured loops.
func benchmarkTree(n int) []TreeNode {
	field := "item"
	nodes := make([]TreeNode, n+1)
	nodes[0] = TreeNode{Parent: -1, Type: "document", Named: true, EndByte: uint32(n), EndPoint: Point{Row: 0, Column: uint32(n)}}
	for i := 1; i <= n; i++ {
		nodes[i] = TreeNode{Parent: 0, Type: "identifier", Field: &field, Named: true,
			StartByte: uint32(i - 1), EndByte: uint32(i), StartPoint: Point{Row: 0, Column: uint32(i - 1)}, EndPoint: Point{Row: 0, Column: uint32(i)}}
	}
	return nodes
}

func TestBenchmarkFixtures(t *testing.T) {
	for _, n := range []int{64, 1024, 16384} {
		a := benchmarkTree(n)
		if err := ValidateTree(a, uint64(n)); err != nil {
			t.Fatal(err)
		}
		if CompareTrees(a, slices.Clone(a)) != nil || TreeDigest(a) == "" {
			t.Fatal("invalid equal-tree fixture")
		}
		b := slices.Clone(a)
		b[n].Type = "different"
		if d := CompareTrees(a, b); d == nil || d.Node != n || d.Field != "type" {
			t.Fatalf("last-node negative control: %+v", d)
		}
		if TreeDigest(a) == TreeDigest(b) {
			t.Fatal("digest did not detect last-node mutation")
		}
		data, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		var decoded []TreeNode
		if err := json.Unmarshal(data, &decoded); err != nil || !reflect.DeepEqual(a, decoded) {
			t.Fatalf("tree JSON round trip: %v", err)
		}
	}
	ctx := testCtx(t)
	for _, n := range []int{64, 1024, 4096} {
		input := SchemaInput{Name: "baseline", Data: benchmarkSchema(n)}
		check, err := SchemaCheck(ctx, SchemaCheckRequest{Input: input, Limits: DefaultSchemaLimits()})
		if err != nil || check.Assessment != AssessPass {
			t.Fatalf("schema fixture: %v %s", err, check.Assessment)
		}
		diff, err := SchemaDiff(ctx, SchemaDiffRequest{Baseline: input, Candidate: SchemaInput{Name: "candidate", Data: benchmarkSchema(n + 1)}, Limits: DefaultSchemaLimits()})
		if err != nil || diff.Assessment != AssessFail || len(diff.Differences) != 1 || diff.Differences[0].Code != "NODE_ADDED" {
			t.Fatalf("schema negative control: %v %s", err, diff.Assessment)
		}
	}
	root := t.TempDir()
	file := filepath.Join(root, "grammar.js")
	req := IdentityRequest{Root: root, Selection: Selection{Grammar: ".", Files: []FileSelection{{Path: "grammar.js", Role: "grammar"}}}, Limits: DefaultLimits()}
	if err := os.WriteFile(file, []byte("aaaa"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := Identity(ctx, req)
	if err != nil || before.ExecutionStatus != StatusCompleted || before.Assessment != AssessNotAssessed {
		t.Fatalf("identity fixture: %v", err)
	}
	if err := os.WriteFile(file, []byte("bbbb"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := Identity(ctx, req)
	if err != nil || before.SetSHA256 == after.SetSHA256 || len(after.Manifest.Files) != 1 || after.Manifest.Files[0].Size != 4 {
		t.Fatalf("same-size identity mutation: %v", err)
	}
}

func BenchmarkCompareTrees(b *testing.B) {
	for _, n := range []int{64, 1024, 16384} {
		for _, different := range []bool{false, true} {
			b.Run(fmt.Sprintf("nodes=%d/last-difference=%t", n+1, different), func(b *testing.B) {
				a := benchmarkTree(n)
				c := slices.Clone(a)
				if different {
					c[n].Type = "different"
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					d := CompareTrees(a, c)
					if (d != nil) != different {
						b.Fatal("comparison changed")
					}
				}
				b.ReportMetric(float64(len(a)), "nodes/op")
			})
		}
	}
}

func BenchmarkTreeDigest(b *testing.B) {
	for _, n := range []int{64, 1024, 16384} {
		b.Run(fmt.Sprintf("nodes=%d", n+1), func(b *testing.B) {
			a := benchmarkTree(n)
			want := TreeDigest(a)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if TreeDigest(a) != want {
					b.Fatal("digest changed")
				}
			}
			b.ReportMetric(float64(len(a)), "nodes/op")
		})
	}
}

func benchmarkSchema(n int) []byte {
	var out strings.Builder
	out.WriteByte('[')
	for i := 0; i < n; i++ {
		if i != 0 {
			out.WriteByte(',')
		}
		fmt.Fprintf(&out, `{"type":"node_%d","named":true}`, i)
	}
	out.WriteByte(']')
	return []byte(out.String())
}

func BenchmarkSchema(b *testing.B) {
	for _, n := range []int{64, 1024, 4096} {
		data := benchmarkSchema(n)
		candidate := benchmarkSchema(n + 1)
		for _, diff := range []bool{false, true} {
			b.Run(fmt.Sprintf("nodes=%d/diff=%t", n, diff), func(b *testing.B) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				input := SchemaInput{Name: "baseline", Data: data}
				b.ReportAllocs()
				b.SetBytes(int64(len(data))) // Baseline document bytes, not source parsing throughput.
				b.ResetTimer()
				for b.Loop() {
					if diff {
						result, err := SchemaDiff(ctx, SchemaDiffRequest{Baseline: input, Candidate: SchemaInput{Name: "candidate", Data: candidate}, Limits: DefaultSchemaLimits()})
						if err != nil || result.Assessment != AssessFail || len(result.Differences) != 1 {
							b.Fatalf("schema diff: %v %+v", err, result)
						}
					} else {
						result, err := SchemaCheck(ctx, SchemaCheckRequest{Input: input, Limits: DefaultSchemaLimits()})
						if err != nil || result.Assessment != AssessPass {
							b.Fatalf("schema check: %v %+v", err, result)
						}
					}
				}
			})
		}
	}
}

func BenchmarkIdentity(b *testing.B) {
	for _, size := range []int{4096, 262144, 4194304} {
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
			root := b.TempDir()
			data := []byte(strings.Repeat("x", size))
			if err := os.WriteFile(filepath.Join(root, "grammar.js"), data, 0600); err != nil {
				b.Fatal(err)
			}
			req := IdentityRequest{Root: root, Selection: Selection{Grammar: ".", Files: []FileSelection{{Path: "grammar.js", Role: "grammar"}}}, Limits: DefaultLimits()}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			want, err := Identity(ctx, req)
			if err != nil || want.ExecutionStatus != StatusCompleted || want.EvidenceMode != ModeNewRun || want.Assessment != AssessNotAssessed || len(want.Manifest.Files) != 1 || want.Manifest.Files[0].Size != uint64(size) {
				b.Fatalf("identity baseline: %v", err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for b.Loop() {
				got, err := Identity(ctx, req)
				if err != nil || got.ExecutionStatus != StatusCompleted || got.EvidenceMode != ModeNewRun || got.Assessment != AssessNotAssessed || got.SetSHA256 != want.SetSHA256 {
					b.Fatalf("identity changed: %v", err)
				}
			}
		})
	}
}

func BenchmarkTreeJSON(b *testing.B) {
	for _, n := range []int{64, 1024, 16384} {
		b.Run(fmt.Sprintf("nodes=%d", n+1), func(b *testing.B) {
			nodes := benchmarkTree(n)
			want, err := json.Marshal(nodes)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(want)))
			b.ResetTimer()
			for b.Loop() {
				got, err := json.Marshal(nodes)
				if err != nil || len(got) != len(want) {
					b.Fatalf("serialization changed: %v", err)
				}
			}
		})
	}
}
