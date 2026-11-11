// Package ordinarycommit builds and verifies source-only Merkle commitments for
// already validated ordinary shard plans. It does not validate plan semantics.
package ordinarycommit

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"strings"
)

const (
	Schema   = "tsgk-query-ordinary-plan-commitment/r1"
	PlanPath = "plan/ordinary.json"
	maxBytes = 16 << 20
	maxItems = 10000
	maxNodes = 2000000
	maxDepth = 64
)

type Pin struct {
	Path   string `json:"path"`
	Bytes  uint64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// Commitment binds the exact full plan pin and its common header to one ordered
// Merkle root. Header is the full plan JSON with only its shards array replaced by [].
type Commitment struct {
	Schema     string         `json:"schema"`
	Plan       Pin            `json:"complete_plan_pin"`
	Header     jsontext.Value `json:"header"`
	ShardCount uint64         `json:"shard_count"`
	Root       string         `json:"root_sha256"`
}

// ShardProof carries the exact source shard bytes and its canonical inclusion path.
type ShardProof struct {
	Shard    jsontext.Value `json:"shard"`
	Index    uint64         `json:"index"`
	Siblings []string       `json:"siblings"`
}

// Build commits a plan emitted and semantically validated by the ordinary planner.
// The commitment verifies the document structure needed for safe selection but does
// not replace the planner's registry, coordinate, or exact-union validation.
func Build(plan []byte) (Commitment, []ShardProof, error) {
	var empty Commitment
	if len(plan) == 0 || len(plan) > maxBytes || !validJSON(plan) {
		return empty, nil, fmt.Errorf("ordinary commitment: invalid or oversized plan")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(plan, &fields); err != nil || fields == nil {
		return empty, nil, fmt.Errorf("ordinary commitment: plan must be an object")
	}
	rawShards, ok := fields["shards"]
	if !ok {
		return empty, nil, fmt.Errorf("ordinary commitment: shards are missing")
	}
	var shards []json.RawMessage
	if err := json.Unmarshal(rawShards, &shards); err != nil || shards == nil || len(shards) == 0 || len(shards) > maxItems {
		return empty, nil, fmt.Errorf("ordinary commitment: invalid shard list")
	}
	ids := make([]string, len(shards))
	for i, raw := range shards {
		var shard struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &shard); err != nil || shard.ID == "" {
			return empty, nil, fmt.Errorf("ordinary commitment: shard %d has no ID", i)
		}
		ids[i] = shard.ID
		if i > 0 && ids[i-1] >= ids[i] {
			return empty, nil, fmt.Errorf("ordinary commitment: shard IDs are duplicated or unsorted")
		}
	}
	fields["shards"] = json.RawMessage("[]")
	header, err := json.Marshal(fields)
	if err != nil || len(header) > maxBytes {
		return empty, nil, fmt.Errorf("ordinary commitment: invalid header")
	}
	planDigest := sha256.Sum256(plan)
	planPin := Pin{Path: PlanPath, Bytes: uint64(len(plan)), SHA256: hex.EncodeToString(planDigest[:])}
	headerDigest := hashHeader(header)
	leaves := make([][32]byte, len(shards))
	for i, raw := range shards {
		leaves[i] = hashLeaf(planPin, headerDigest, uint64(len(shards)), uint64(i), raw)
	}
	levels := [][][32]byte{leaves}
	for len(levels[len(levels)-1]) > 1 {
		previous := levels[len(levels)-1]
		next := make([][32]byte, (len(previous)+1)/2)
		for i := range next {
			left := previous[2*i]
			right := left
			if 2*i+1 < len(previous) {
				right = previous[2*i+1]
			}
			next[i] = hashNode(left, right)
		}
		levels = append(levels, next)
	}
	proofs := make([]ShardProof, len(leaves))
	for index, raw := range shards {
		proof := ShardProof{Shard: append(jsontext.Value(nil), raw...), Index: uint64(index), Siblings: make([]string, 0, len(levels)-1)}
		at := index
		for level := 0; level < len(levels)-1; level++ {
			row := levels[level]
			sibling := at ^ 1
			if sibling >= len(row) {
				sibling = at
			}
			proof.Siblings = append(proof.Siblings, hex.EncodeToString(row[sibling][:]))
			at /= 2
		}
		proofs[index] = proof
	}
	commitment := Commitment{
		Schema: Schema, Plan: planPin, Header: jsontext.Value(header),
		ShardCount: uint64(len(leaves)), Root: hex.EncodeToString(levels[len(levels)-1][0][:]),
	}
	return commitment, proofs, nil
}

// Verify checks the shard bytes and the unique canonical path for their index.
func Verify(commitment Commitment, shard []byte, proof ShardProof) error {
	if commitment.Schema != Schema || commitment.Plan.Path != PlanPath || commitment.Plan.Bytes == 0 || commitment.Plan.Bytes > maxBytes || !validDigest(commitment.Plan.SHA256) ||
		commitment.ShardCount == 0 || commitment.ShardCount > maxItems || len(shard) == 0 || len(shard) > maxBytes || !bytes.Equal(shard, proof.Shard) ||
		proof.Index >= commitment.ShardCount || !validDigest(commitment.Root) || !validJSON(commitment.Header) {
		return fmt.Errorf("ordinary commitment: invalid proof input")
	}
	var header map[string]json.RawMessage
	if json.Unmarshal(commitment.Header, &header) != nil || header == nil || string(header["shards"]) != "[]" {
		return fmt.Errorf("ordinary commitment: invalid header")
	}
	var shardValue struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(shard, &shardValue) != nil || shardValue.ID == "" {
		return fmt.Errorf("ordinary commitment: invalid shard")
	}
	width, index := commitment.ShardCount, proof.Index
	wantLevels := 0
	for n := width; n > 1; n = (n + 1) / 2 {
		wantLevels++
	}
	if len(proof.Siblings) != wantLevels {
		return fmt.Errorf("ordinary commitment: noncanonical proof length")
	}
	headerDigest := hashHeader(commitment.Header)
	current := hashLeaf(commitment.Plan, headerDigest, commitment.ShardCount, proof.Index, shard)
	for _, siblingText := range proof.Siblings {
		siblingBytes, err := hex.DecodeString(siblingText)
		if err != nil || len(siblingBytes) != sha256.Size || strings.ToLower(siblingText) != siblingText {
			return fmt.Errorf("ordinary commitment: invalid sibling digest")
		}
		var sibling [32]byte
		copy(sibling[:], siblingBytes)
		if index%2 == 0 && index+1 >= width && sibling != current {
			return fmt.Errorf("ordinary commitment: noncanonical duplicated sibling")
		}
		if index%2 == 0 {
			current = hashNode(current, sibling)
		} else {
			current = hashNode(sibling, current)
		}
		index /= 2
		width = (width + 1) / 2
	}
	if width != 1 || index != 0 || hex.EncodeToString(current[:]) != commitment.Root {
		return fmt.Errorf("ordinary commitment: inclusion proof mismatch")
	}
	return nil
}

func hashHeader(header []byte) [32]byte {
	h := sha256.New()
	h.Write([]byte("tsgk-ordinary-plan-header/r1\x00"))
	writeBytes(h, header)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func hashLeaf(plan Pin, header [32]byte, count, index uint64, shard []byte) [32]byte {
	h := sha256.New()
	h.Write([]byte("tsgk-ordinary-plan-leaf/r1\x00"))
	writeBytes(h, []byte(plan.Path))
	writeUint(h, plan.Bytes)
	writeBytes(h, []byte(plan.SHA256))
	h.Write(header[:])
	writeUint(h, count)
	writeUint(h, index)
	writeBytes(h, shard)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func hashNode(left, right [32]byte) [32]byte {
	h := sha256.New()
	h.Write([]byte("tsgk-ordinary-plan-node/r1\x00"))
	h.Write(left[:])
	h.Write(right[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func writeUint(w io.Writer, value uint64) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], value)
	_, _ = w.Write(size[:])
}

func writeBytes(w io.Writer, value []byte) {
	writeUint(w, uint64(len(value)))
	_, _ = w.Write(value)
}

func validDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validJSON(data []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	nodes := 0
	if !scanValue(decoder, 0, &nodes) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

func scanValue(decoder *json.Decoder, depth int, nodes *int) bool {
	*nodes++
	if *nodes > maxNodes || depth > maxDepth {
		return false
	}
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			key, ok := keyToken.(string)
			if err != nil || !ok || seen[key] {
				return false
			}
			seen[key] = true
			if !scanValue(decoder, depth+1, nodes) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for decoder.More() {
			if !scanValue(decoder, depth+1, nodes) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}
