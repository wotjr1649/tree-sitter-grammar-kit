package kit

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json/jsontext"
	"io"
	"strings"
)

const ordinaryPlanCommitmentSchema = "tsgk-query-ordinary-plan-commitment/r1"
const ordinaryPlanCommitmentPath = "plan/ordinary.json"

// These private wire types mirror src/internal/ordinarycommit. Kit cannot import
// src/internal packages; keep the shared serialized contract covered by tests.
type ordinaryPlanCommitment struct {
	Schema     string         `json:"schema"`
	Plan       OrdinaryPin    `json:"complete_plan_pin"`
	Header     jsontext.Value `json:"header"`
	ShardCount uint64         `json:"shard_count"`
	Root       string         `json:"root_sha256"`
}

type ordinaryShardProof struct {
	Shard    jsontext.Value `json:"shard"`
	Index    uint64         `json:"index"`
	Siblings []string       `json:"siblings"`
}

func verifyOrdinaryCommitment(commitment ordinaryPlanCommitment, shard []byte, proof ordinaryShardProof) error {
	if commitment.Schema != ordinaryPlanCommitmentSchema || commitment.Plan.Path != ordinaryPlanCommitmentPath || commitment.Plan.Bytes == 0 || commitment.Plan.Bytes > 16<<20 || !hex64.MatchString(commitment.Plan.SHA256) ||
		commitment.ShardCount == 0 || commitment.ShardCount > 10000 || len(commitment.Header) == 0 || len(commitment.Header) > 16<<20 || len(shard) == 0 || len(shard) > 16<<20 || !bytes.Equal(shard, proof.Shard) ||
		proof.Index >= commitment.ShardCount || !hex64.MatchString(commitment.Root) {
		return ordinaryBad("ORDINARY_COMMITMENT_INPUT")
	}
	width, index := commitment.ShardCount, proof.Index
	wantLevels := 0
	for n := width; n > 1; n = (n + 1) / 2 {
		wantLevels++
	}
	if len(proof.Siblings) != wantLevels {
		return ordinaryBad("ORDINARY_COMMITMENT_PROOF_LENGTH")
	}
	current := ordinaryCommitHeader(commitment.Header)
	current = ordinaryCommitLeaf(commitment.Plan, current, commitment.ShardCount, proof.Index, shard)
	for _, text := range proof.Siblings {
		if len(text) != sha256.Size*2 || strings.ToLower(text) != text {
			return ordinaryBad("ORDINARY_COMMITMENT_SIBLING")
		}
		raw, err := hex.DecodeString(text)
		if err != nil {
			return ordinaryBad("ORDINARY_COMMITMENT_SIBLING")
		}
		var sibling [sha256.Size]byte
		copy(sibling[:], raw)
		if index%2 == 0 && index+1 >= width && sibling != current {
			return ordinaryBad("ORDINARY_COMMITMENT_DUPLICATED_SIBLING")
		}
		if index%2 == 0 {
			current = ordinaryCommitNode(current, sibling)
		} else {
			current = ordinaryCommitNode(sibling, current)
		}
		index /= 2
		width = (width + 1) / 2
	}
	if width != 1 || index != 0 || hex.EncodeToString(current[:]) != commitment.Root {
		return ordinaryBad("ORDINARY_COMMITMENT_PROOF")
	}
	return nil
}

func ordinaryCommitHeader(header []byte) [sha256.Size]byte {
	h := sha256.New()
	_, _ = io.WriteString(h, "tsgk-ordinary-plan-header/r1\x00")
	ordinaryCommitWriteBytes(h, header)
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

func ordinaryCommitLeaf(plan OrdinaryPin, header [sha256.Size]byte, count, index uint64, shard []byte) [sha256.Size]byte {
	h := sha256.New()
	_, _ = io.WriteString(h, "tsgk-ordinary-plan-leaf/r1\x00")
	ordinaryCommitWriteBytes(h, []byte(plan.Path))
	ordinaryCommitWriteUint(h, plan.Bytes)
	ordinaryCommitWriteBytes(h, []byte(plan.SHA256))
	_, _ = h.Write(header[:])
	ordinaryCommitWriteUint(h, count)
	ordinaryCommitWriteUint(h, index)
	ordinaryCommitWriteBytes(h, shard)
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

func ordinaryCommitNode(left, right [sha256.Size]byte) [sha256.Size]byte {
	h := sha256.New()
	_, _ = io.WriteString(h, "tsgk-ordinary-plan-node/r1\x00")
	_, _ = h.Write(left[:])
	_, _ = h.Write(right[:])
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

func ordinaryCommitWriteUint(w io.Writer, value uint64) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], value)
	_, _ = w.Write(size[:])
}

func ordinaryCommitWriteBytes(w io.Writer, value []byte) {
	ordinaryCommitWriteUint(w, uint64(len(value)))
	_, _ = w.Write(value)
}
