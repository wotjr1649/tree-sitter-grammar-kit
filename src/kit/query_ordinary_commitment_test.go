package kit

import (
	"encoding/json/jsontext"
	"testing"
)

func TestOrdinaryCommitmentWireVector(t *testing.T) {
	commitment := ordinaryPlanCommitment{
		Schema: ordinaryPlanCommitmentSchema,
		Plan:   OrdinaryPin{Path: ordinaryPlanCommitmentPath, Bytes: 243, SHA256: "8d56380b804c53caa0bbbada3f2ad97c01d05587ad3253351a2a392c04866a16"},
		Header: jsontext.Value(`{"schema":"plan/r1","shards":[],"value":"plan-a"}`), ShardCount: 5,
		Root: "93b457ea10c62200926643a9327f90ab4ce5d1608f30ac5b67bdb483403fdb5e",
	}
	proof := ordinaryShardProof{
		Shard: jsontext.Value(`{"id":"plan-a/00","payload":"value-0"}`), Index: 0,
		Siblings: []string{
			"ce5f986c40ecb71f206d10a008ccfef8ffaffc20986202fefe21103a89ddb563",
			"580014f2c8b5bf95cc06ba903d15d1112d952e5446e39e98bf46b861de255285",
			"e3b02bdee27e2e840f930c3f61ff4cdb53825bca49a92ac804693f96d90d3559",
		},
	}
	if err := verifyOrdinaryCommitment(commitment, proof.Shard, proof); err != nil {
		t.Fatalf("builder/verifier wire vector mismatch: %v", err)
	}
	proof.Siblings[0] = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := verifyOrdinaryCommitment(commitment, proof.Shard, proof); err == nil {
		t.Fatal("accepted a modified Merkle path")
	}
}
