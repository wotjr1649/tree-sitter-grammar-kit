package ordinarycommit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

func TestBuildEnforcesPlanByteLimitAtOneByteBoundary(t *testing.T) {
	small := planBytes("plan-a", 1)
	exact := append(append([]byte(nil), small...), bytes.Repeat([]byte{' '}, maxBytes-len(small))...)
	if len(exact) != maxBytes {
		t.Fatalf("fixture size=%d, want %d", len(exact), maxBytes)
	}
	if _, _, err := Build(exact); err != nil {
		t.Fatalf("plan exactly at file limit rejected: %v", err)
	}
	if _, _, err := Build(append(exact, ' ')); err == nil {
		t.Fatal("plan one byte over file limit accepted")
	}
}

func TestBuildEnforcesShardEntryLimit(t *testing.T) {
	if _, proofs, err := Build(manyEntryPlan(maxItems)); err != nil || len(proofs) != maxItems {
		t.Fatalf("plan at entry limit: proofs=%d err=%v", len(proofs), err)
	}
	if _, _, err := Build(manyEntryPlan(maxItems + 1)); err == nil {
		t.Fatal("plan one entry over limit accepted")
	}
}

func manyEntryPlan(count int) []byte {
	shards := make([]map[string]string, count)
	for i := range shards {
		shards[i] = map[string]string{"id": fmt.Sprintf("plan-a/%05d", i), "payload": "value"}
	}
	data, err := json.Marshal(struct {
		Schema string              `json:"schema"`
		Shards []map[string]string `json:"shards"`
	}{Schema: "plan/r1", Shards: shards})
	if err != nil {
		panic(err)
	}
	return data
}
