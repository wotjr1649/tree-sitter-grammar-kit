package ordinarycommit

import (
	"encoding/json"
	"fmt"
	"testing"
)

func planBytes(prefix string, count int) []byte {
	return planWith(prefix, prefix, count)
}

func planWith(header, shardPrefix string, count int) []byte {
	shards := make([]map[string]string, count)
	for i := range shards {
		shards[i] = map[string]string{"id": fmt.Sprintf("%s/%02d", shardPrefix, i), "payload": fmt.Sprintf("value-%d", i)}
	}
	doc := struct {
		Schema string              `json:"schema"`
		Value  string              `json:"value"`
		Shards []map[string]string `json:"shards"`
	}{Schema: "plan/r1", Value: header, Shards: shards}
	data, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return data
}

func TestBuildAndVerifyProofs(t *testing.T) {
	plan := planBytes("plan-a", 5)
	commitment, proofs, err := Build(plan)
	if err != nil {
		t.Fatal(err)
	}
	if commitment.Plan.Bytes != uint64(len(plan)) || commitment.Plan.SHA256 == "" || commitment.ShardCount != 5 || len(proofs) != 5 {
		t.Fatalf("unexpected commitment summary: %#v", commitment)
	}
	for i, proof := range proofs {
		if proof.Index != uint64(i) {
			t.Fatalf("proof %d has index %d", i, proof.Index)
		}
		if err := Verify(commitment, proof.Shard, proof); err != nil {
			t.Fatalf("proof %d: %v", i, err)
		}
	}
}

func TestProofBindsPlanHeaderShardAndPosition(t *testing.T) {
	commitA, proofsA, err := Build(planBytes("plan-a", 5))
	if err != nil {
		t.Fatal(err)
	}
	_, proofsB, err := Build(planWith("plan-b", "plan-a", 5))
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		commit Commitment
		proof  ShardProof
	}{
		"other-plan-header":       {commitA, proofsB[0]},
		"replacement-at-position": {commitA, func() ShardProof { p := proofsA[0]; p.Shard = proofsA[1].Shard; return p }()},
		"swapped-index":           {commitA, func() ShardProof { p := proofsA[0]; p.Index = 1; return p }()},
		"missing-sibling":         {commitA, func() ShardProof { p := proofsA[0]; p.Siblings = p.Siblings[:len(p.Siblings)-1]; return p }()},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if err := Verify(tc.commit, tc.proof.Shard, tc.proof); err == nil {
				t.Fatal("accepted a proof outside its committed plan position")
			}
		})
	}
	proof := proofsA[0]
	proof.Siblings[0] = proofsA[0].Siblings[0]
	proof.Shard = append(proof.Shard[:len(proof.Shard)-1], ' ')
	if err := Verify(commitA, proof.Shard, proof); err == nil {
		t.Fatal("accepted modified shard bytes")
	}
}

func TestOddTreeRequiresDuplicatedLastSibling(t *testing.T) {
	commitment, proofs, err := Build(planBytes("plan-a", 3))
	if err != nil {
		t.Fatal(err)
	}
	proof := proofs[2]
	if err := Verify(commitment, proof.Shard, proof); err != nil {
		t.Fatal(err)
	}
	proof.Siblings[0] = proofs[0].Siblings[0]
	if err := Verify(commitment, proof.Shard, proof); err == nil {
		t.Fatal("accepted a noncanonical sibling for the odd final leaf")
	}
}

func TestBuildRejectsDuplicateUnsortedAndAmbiguousPlan(t *testing.T) {
	for name, plan := range map[string][]byte{
		"duplicate-id":  []byte(`{"schema":"r1","shards":[{"id":"a"},{"id":"a"}]}`),
		"unsorted-id":   []byte(`{"schema":"r1","shards":[{"id":"b"},{"id":"a"}]}`),
		"duplicate-key": []byte(`{"schema":"r1","schema":"r2","shards":[{"id":"a"}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Build(plan); err == nil {
				t.Fatal("accepted invalid commitment input")
			}
		})
	}
}
