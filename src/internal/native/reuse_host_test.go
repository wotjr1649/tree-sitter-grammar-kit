package native

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

func TestReuseHostRequiresEveryRole(t *testing.T) {
	x := testContext("native-parse-edit")
	x.Protocol, x.ObservationRunID, x.ObservationProducerSHA256 = ProtocolR3, "host-run", sha([]byte("producer"))
	o := reuseobs.Observation{Schema: reuseobs.Schema, Policy: reuseobs.Policy, RuntimeManifestSHA256: reuseobs.RuntimeManifestSHA256,
		RequestID: "case", RunID: x.ObservationRunID, ProducerSHA256: x.ObservationProducerSHA256, Role: "initial", SourceSHA256: sha([]byte("a")), SourceBytes: 1,
		Status: "COMPLETE", Events: 2, Bytes: 20, Counts: &reuseobs.Counts{NewParse: 1, Done: 1}}
	for _, missing := range []string{"all-edit-roles", "incremental-role", "native-no-parse-marker", "injected-no-parse-marker", "injected-inline-marker"} {
		t.Run(missing, func(t *testing.T) {
			c := CaseResult{ID: "case", Steps: []StepResult{{Incremental: &TreeOut{}, Observations: []reuseobs.Observation{o}}, {Step: 1, Incremental: &TreeOut{}}}}
			if missing == "incremental-role" {
				fresh := o
				fresh.Role, fresh.Step = "fresh", 1
				c.Steps[1].Observations = []reuseobs.Observation{fresh}
			}
			if missing == "native-no-parse-marker" {
				c.Steps[1].Incremental = nil
				c.Steps[1].Composite = &SvcComposite{Inline: &SvcInline{}}
			}
			if missing == "injected-no-parse-marker" || missing == "injected-inline-marker" {
				for _, role := range []string{"incremental", "fresh"} {
					marker := o
					marker.Role, marker.Step, marker.Status, marker.Reason = role, 1, "UNAVAILABLE", "NO_NATIVE_PARSE"
					marker.Events, marker.Bytes, marker.Counts = 0, 0, &reuseobs.Counts{}
					c.Steps[1].Observations = append(c.Steps[1].Observations, marker)
				}
				if missing == "injected-inline-marker" {
					c.Steps[1].Incremental = nil
					c.Steps[1].Composite = &SvcComposite{Inline: &SvcInline{}}
				}
			}
			x.observeCase(&c)
			if c.ReuseObservationClaim != ClaimBlocked {
				t.Fatalf("missing role %s became %s", missing, c.ReuseObservationClaim)
			}
			if missing == "native-no-parse-marker" && len(c.Steps[1].Observations) != 0 {
				t.Fatal("native attempt labelled as host-only")
			}
		})
	}
}

func TestNativeReuseBatchProjection(t *testing.T) {
	b := fixtureBuild(t, "plain")
	x := testContext("private-corpus-local")
	x.Output = kit.OutputRecord
	x.PolicyRef = PolicyRef(x.Op, x.Output)
	x.Protocol, x.ObservationRunID = ProtocolR3, "batch-reuse-run"
	root := t.TempDir()
	var cases []kit.IncrementalCase
	for _, id := range []string{"a", "b"} {
		source := []byte(id + " = 1;")
		name := id + ".txt"
		if err := os.WriteFile(filepath.Join(root, name), source, 0o644); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, kit.IncrementalCase{ID: id, Encoding: kit.EncodingUTF8, Input: kit.NativeInput{Path: name, Role: "case", SHA256: sha(source), Bytes: uint64(len(source))}})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	outDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(outDir, "responses"), 0o755); err != nil {
		t.Fatal(err)
	}
	responses, err := os.OpenRoot(filepath.Join(outDir, "responses"))
	if err != nil {
		t.Fatal(err)
	}
	defer responses.Close()
	results, batches := runBatches(ctx, b, x, root, cases, responses, nil)
	if len(results) != 2 || len(batches) != 1 {
		t.Fatalf("batch accounting %d/%d", len(results), len(batches))
	}
	for i, c := range results {
		if c.ExecutionStatus != kit.StatusCompleted || c.ReuseObservationClaim != ClaimPass || len(c.Steps) != 1 || len(c.Steps[0].Observations) != 1 {
			t.Fatalf("batch %s status=%s code=%s claim=%s", c.ID, c.ExecutionStatus, c.Code, c.ReuseObservationClaim)
		}
		o := c.Steps[0].Observations[0]
		raw, err := os.ReadFile(filepath.Join(outDir, "responses", fmt.Sprintf("%05d-%s.json", i, c.ID)))
		if err != nil {
			t.Fatal(err)
		}
		response, err := DecodeResponse(raw)
		if err != nil || response.ID != c.ID || response.Protocol != ProtocolR3 || len(response.Steps) != 1 {
			t.Fatalf("batch raw %s: %v", c.ID, err)
		}
		if o.RequestID != cases[i].ID || o.RunID != x.ObservationRunID || o.ProducerSHA256 != b.Identity || o.Counts.Accepted != 0 || o.Counts.NewParse != 1 {
			t.Fatalf("batch payload isolation: %+v", o)
		}
	}
	unavailable, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := unavailable.Close(); err != nil {
		t.Fatal(err)
	}
	failed, _ := runBatches(ctx, b, x, root, cases, unavailable, nil)
	for _, c := range failed {
		if c.ExecutionStatus != kit.StatusFailed || c.Code != "EVIDENCE_WRITE_FAILED" {
			t.Fatalf("raw write failure hidden: %s %s", c.ExecutionStatus, c.Code)
		}
	}
}
