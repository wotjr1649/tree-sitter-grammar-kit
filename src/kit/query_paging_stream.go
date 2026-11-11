package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"slices"
	"strings"
)

// The graph fixes the read order; descriptors still validate every pin and logical identity.
type streamedChain struct {
	hash.Hash
	pins []ReplayMember
	next int
}

func (s *evidenceSnapshot) streamPlan(n EvidenceNode, start int, nodes []EvidenceNode, pol evidencePolicy) (*streamedChain, *Error) {
	if s.paging == nil || !s.graphBound || s.invalidNodes[n.ID] {
		return nil, nil
	}
	f := n.Files[start]
	prefix, ok := strings.CutSuffix(f.Path, ".part-00000.bin")
	if !ok || !pagingPartRole(f.Role) {
		return nil, nil
	}
	// Later graph checks must not invalidate a node whose fallback bytes were omitted.
	if n.Kind != "run" || len(n.Refs) != 0 {
		return nil, nil
	}
	count := 0
	for _, other := range nodes {
		if e := s.paging.b.charge(128, 0, "streamed-chain-plan"); e != nil {
			return nil, e
		}
		if other.ID == n.ID {
			count++
			continue
		}
		if other.Kind != "replay" && other.Kind != "carry" && other.Identities["run"] == n.Identities["run"] && other.Identities["attempt"] == n.Identities["attempt"] && other.Identities["platform"] == n.Identities["platform"] {
			return nil, nil
		}
	}
	if count != 1 {
		return nil, nil
	}
	for _, req := range pol.Required {
		if e := s.paging.b.charge(128, 0, "streamed-chain-plan"); e != nil {
			return nil, e
		}
		if req.Node == n.ID {
			if req.Kind != n.Kind {
				return nil, nil
			}
			for role, value := range req.Identities {
				if n.Identities[role] != value {
					return nil, nil
				}
			}
		}
	}
	if pol.Eligibility != nil {
		for _, id := range pol.Eligibility.Nodes {
			if e := s.paging.b.charge(128, 0, "streamed-chain-plan"); e != nil {
				return nil, e
			}
			if id == n.ID && !slices.Contains(pol.Eligibility.Modes, n.EvidenceMode) {
				return nil, nil
			}
		}
	}
	end := start
	for end < len(n.Files) {
		p := n.Files[end]
		if e := s.paging.b.charge(2*uint64(len(p.Path)+len(prefix))+128, 0, "streamed-chain-plan"); e != nil {
			return nil, e
		}
		if p.Role != f.Role || !strings.HasPrefix(p.Path, prefix+".part-") {
			break
		}
		if p.Path != fmt.Sprintf("%s.part-%05d.bin", prefix, end-start) || s.approved[p.Path] != p || !portable(p.Path, false) || s.retained[p.Path] {
			return nil, nil
		}
		end++
	}
	if end-start < 2 {
		return nil, nil
	}
	// Reject split groups as well as reordered or duplicate members, retaining their fallback bytes.
	for _, other := range nodes {
		for i, p := range other.Files {
			if e := s.paging.b.charge(uint64(len(p.Path)+len(prefix))+1, 0, "streamed-chain-plan"); e != nil {
				return nil, e
			}
			if strings.HasPrefix(p.Path, prefix+".part-") && (other.ID != n.ID || i < start || i >= end) {
				return nil, nil
			}
		}
	}
	logical := uint64(1024)
	for _, p := range n.Files[start:end] {
		logical += 128 + uint64(len(p.Path)+len(p.Role)+len(p.SHA256))
	}
	if e := s.paging.b.charge(1, logical, "streamed-chain-record"); e != nil {
		return nil, e
	}
	return &streamedChain{Hash: sha256.New(), pins: append([]ReplayMember(nil), n.Files[start:end]...)}, nil
}

func (v *verifiedPageSource) chainSum(h hash.Hash) string {
	if v != nil && v.stream != nil {
		if v.stream.next != len(v.stream.pins) || len(v.parts) != len(v.stream.pins) {
			return ""
		}
		return hex.EncodeToString(v.stream.Sum(nil))
	}
	return hex.EncodeToString(h.Sum(nil))
}
