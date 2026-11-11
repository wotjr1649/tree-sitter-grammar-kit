package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/ordinarycommit"
)

func TestBoundedRootInput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "input.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readRoot(root, "input.json", 2)
	if err != nil || string(got) != "{}" {
		t.Fatalf("valid bounded input: got=%q err=%v", got, err)
	}
	if _, err := readRoot(root, "../input.json", 2); err == nil {
		t.Fatal("parent traversal accepted")
	}
	if _, err := readRoot(root, "input.json", 1); err == nil {
		t.Fatal("oversized input accepted")
	}
}

func TestWriteCommitmentArtifacts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "commitment")
	plan := []byte(`{"shards":[{"id":"a"},{"id":"b"}]}`)
	if err := writeCommitmentArtifacts(dir, plan); err != nil {
		t.Fatal(err)
	}
	commitmentBytes, err := os.ReadFile(filepath.Join(dir, "commitment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var commitment ordinarycommit.Commitment
	if err := json.Unmarshal(commitmentBytes, &commitment); err != nil {
		t.Fatal(err)
	}
	for count := 0; count < 2; count++ {
		proofBytes, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("proof-%06d.json", count)))
		if err != nil {
			t.Fatal(err)
		}
		if len(proofBytes) > 16<<20 {
			t.Fatalf("proof %d exceeds the physical file limit: %d", count, len(proofBytes))
		}
		var proof ordinarycommit.ShardProof
		if err := json.Unmarshal(proofBytes, &proof); err != nil {
			t.Fatal(err)
		}
		if err := ordinarycommit.Verify(commitment, proof.Shard, proof); err != nil {
			t.Fatalf("proof %d did not verify: %v", count, err)
		}
	}
	if err := writeCommitmentArtifacts(dir, plan); err == nil {
		t.Fatal("existing commitment directory was overwritten")
	}
}
