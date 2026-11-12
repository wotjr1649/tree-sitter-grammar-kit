package kit

import (
	"hash"
	"io/fs"
	"os"
)

// Only readFileContent creates these, retaining either a private slice or its streamed chain.
// Neither callbacks nor snapshot.files receive a mutable view of content.
type guardedPart struct {
	root    *os.Root
	path    string
	info    fs.FileInfo
	size    uint64
	sha256  string
	content []byte
	chain   *streamedChain
	ordinal int
}

func pagingPartRole(role string) bool {
	return role == "grammar-paged-part" || role == "driver-paged-part" || role == "library-paged-part"
}

type pagingBinding struct {
	graph                           ReplayMember
	approval, registration, witness string
	policy, context, invocation     string
	candidate                       eligibilityCandidate
	cohort                          eligibilityCohort
	ordinaryDigest                  [32]byte
	ordinaryShardCount              uint32
}

type verifiedPaging struct {
	owner    *eligibilitySession
	r        *run
	b        *eligibilityBudget
	snapshot *evidenceSnapshot
	root     *os.Root
	binding  pagingBinding
	records  map[string]verifiedLogical
	finished bool
}

type verifiedLogical struct {
	descriptor eligibilityMember
	data       string
	owner      string
	kind       string
	partPrefix string
	index      int
	input      semanticBuildInput
	logical    semanticBinaryLogical
	parts      []ReplayMember
	cohort     eligibilityCohort
}

func (s *eligibilitySession) pagingBinding() (pagingBinding, *Error) {
	x := pagingBinding{graph: s.reg.Graph, approval: s.res.ApprovalSHA256, registration: s.res.RegistrationSHA256,
		witness: s.res.WitnessSHA256, policy: s.approval.EvidencePolicySHA256, context: s.reg.Context, candidate: s.approval.Candidate}
	if s.ordinary != nil {
		ordinary := s.ordinary.Binding
		if s.approval.Ordinary == nil || !ordinaryEligibilityBindingMatches(*s.approval.Ordinary, ordinary) {
			return x, fail(KindInvalidInput, "ELIGIBILITY_PAGING_IDENTITY", "approval/ordinary", nil)
		}
		bytes := uint64(8*8*3 + 128 + 8*len(ordinary.ShardIDs))
		for _, pin := range []OrdinaryPin{ordinary.Profile, ordinary.PlanningProfile, ordinary.Registration, ordinary.Assets, ordinary.Plan, ordinary.Commitment, ordinary.ReferenceSelection, ordinary.RunBinding} {
			bytes += uint64(len(pin.Path) + len(pin.SHA256))
		}
		for _, value := range []string{ordinary.Schema, ordinary.Context, ordinary.ClosureEncoding, ordinary.Candidate, ordinary.SemanticCohort, ordinary.RunID} {
			bytes += uint64(len(value))
		}
		for _, id := range ordinary.ShardIDs {
			bytes += uint64(len(id))
		}
		if e := s.b.charge(bytes+uint64(16+len(ordinary.ShardIDs)), bytes, "ordinary-paging-binding"); e != nil {
			return x, e
		}
		x.ordinaryDigest = ordinaryPagingDigest(ordinary)
		x.ordinaryShardCount = uint32(len(ordinary.ShardIDs))
		for _, cohort := range s.approval.Cohorts {
			if cohort.Purpose == "current" {
				x.cohort = cohort
				break
			}
		}
	}
	if s.reg.Native != nil {
		x.invocation = s.reg.Native.InvocationID
		if inv := s.invocations[x.invocation]; inv != nil {
			x.cohort = s.cohorts[inv.CohortID]
		}
	}
	return x, nil
}

func (p *verifiedPaging) valid(s *eligibilitySession) (bool, *Error) {
	if p == nil || s == nil || s.snapshot == nil || s.b == nil || p.owner != s || p.snapshot != s.snapshot || s.snapshot.paging != p || !s.snapshot.graphBound || !s.bound || p.r != s.r || p.b != s.b || s.r != s.b.r || p.root == nil {
		return false, nil
	}
	binding, e := s.pagingBinding()
	if e != nil || p.binding != binding {
		return false, e
	}
	return true, nil
}

// memberPin validates ownership without exporting the retained bytes.
func (s *eligibilitySession) memberPin(pin *eligibilityMember) (bool, *Error) {
	return s.checkMemberPin(pin, true)
}

// checkMemberPin is memberPin with optional finding records: the semantic set reuses a finished
// eligibility session and must not append findings to it.
func (s *eligibilitySession) checkMemberPin(pin *eligibilityMember, record bool) (bool, *Error) {
	if pin == nil {
		return false, nil
	}
	if e := s.b.charge(1, uint64(len(pin.Member.Path)+len(pin.Member.Role)+len(pin.Member.SHA256)), pin.Member.Path); e != nil {
		return false, e
	}
	expected, ok := s.owned[pin.Member.Path]
	if !ok || expected != *pin || s.owners[pin.Member.Path] != pin.NodeID {
		if record {
			s.finding("ELIGIBILITY_MEMBER_OWNER", pin.Member.Path, true)
		}
		return false, nil
	}
	actual, ok := s.snapshot.verifiedMembers[pin.Member.Path]
	if !ok || actual != pin.Member {
		if record {
			s.finding("ELIGIBILITY_MEMBER_UNVERIFIED", pin.Member.Path, true)
		}
		return false, nil
	}
	if _, ok := s.snapshot.files[pin.Member.Path]; !ok && s.snapshot.parts[pin.Member.Path] == nil && s.snapshot.identities[pin.Member.Path].owner == "" {
		if record {
			s.finding("ELIGIBILITY_MEMBER_MISSING", pin.Member.Path, true)
		}
		return false, nil
	}
	return true, nil
}

// part returns rejected=true only when the ownership check itself failed (and, if record, was
// already reported), so callers do not run the same check again and duplicate the finding.
func (p *verifiedPaging) part(s *eligibilitySession, pin eligibilityMember, record bool) (_ *guardedPart, ok bool, rejected bool, _ *Error) {
	valid, e := p.valid(s)
	if e != nil {
		return nil, false, false, e
	}
	if !valid {
		return nil, false, false, fail(KindInvalidInput, "ELIGIBILITY_PAGING_IDENTITY", pin.Member.Path, nil)
	}
	if p.snapshot.invalidNodes[pin.NodeID] {
		return nil, false, false, nil
	}
	ok, e = s.checkMemberPin(&pin, record)
	if e != nil || !ok {
		return nil, false, e == nil, e
	}
	part := p.snapshot.parts[pin.Member.Path]
	if part == nil {
		return nil, false, false, nil
	}
	info := p.snapshot.observed[pin.Member.Path]
	if !pagingPartRole(pin.Member.Role) || part.root != p.root || part.path != pin.Member.Path || info == nil ||
		!os.SameFile(info, part.info) || info.Size() != part.info.Size() || !info.ModTime().Equal(part.info.ModTime()) ||
		part.size != pin.Member.Bytes || part.sha256 != pin.Member.SHA256 || part.chain == nil && uint64(len(part.content)) != part.size {
		return nil, false, false, fail(KindInvalidInput, "ELIGIBILITY_PAGING_IDENTITY", pin.Member.Path, nil)
	}
	if part.chain != nil && (part.content != nil || part.ordinal < 0 || part.ordinal >= len(part.chain.pins) || part.chain.pins[part.ordinal] != pin.Member || part.ordinal >= part.chain.next || part.chain.next > len(part.chain.pins)) {
		// A chain cut short by a later part's file failure still serves the parts it verified; that later
		// part is rejected by its own pin check, and chainSum refuses any incomplete chain.
		return nil, false, false, fail(KindInvalidInput, "ELIGIBILITY_PAGING_IDENTITY", pin.Member.Path, nil)
	}
	return part, true, false, nil
}

type verifiedPageSource struct {
	paging     *verifiedPaging
	descriptor eligibilityMember
	prefix     string
	used       map[string]bool
	parts      []ReplayMember
	complete   bool
	set        map[string]OracleMember
	cohort     eligibilityCohort
	stream     *streamedChain
}

func (s *eligibilitySession) pageSource(pin eligibilityMember, prefix string, used map[string]bool) (*verifiedPageSource, *Error) {
	p := s.snapshot.paging
	if p == nil {
		return nil, nil
	}
	valid, e := p.valid(s)
	if e != nil {
		return nil, e
	}
	if !valid || s.snapshot.invalidNodes[pin.NodeID] {
		// Unbound input keeps the full callback/hash path and its original findings.
		return nil, nil
	}
	if e := s.b.charge(128, 512, "verified-page-source"); e != nil {
		return nil, e
	}
	if _, err := p.root.Stat("."); err != nil {
		return nil, fail(KindIO, "SOURCE_CHANGED", pin.Member.Path, err)
	}
	ok, e := s.memberPin(&pin)
	if e != nil || !ok {
		return nil, e
	}
	cohort := eligibilityCohort{}
	if inv := s.invocations[s.nodes[pin.NodeID].Identities["run"]]; inv != nil && inv.Node.ID == pin.NodeID {
		cohort = s.cohorts[inv.CohortID]
	}
	return &verifiedPageSource{paging: p, descriptor: pin, prefix: prefix, used: used, complete: true, cohort: cohort}, nil
}

func (v *verifiedPageSource) load(pin ReplayMember) (*guardedPart, bool, bool, *Error) {
	if v.set != nil {
		m, ok := v.set[pin.Path]
		if !ok || m.Case != "" || m.Role != pin.Role || m.Bytes != pin.Bytes || m.SHA256 != pin.SHA256 {
			return nil, false, false, nil
		}
	}
	pin.Path = v.prefix + pin.Path
	if v.used != nil {
		v.used[pin.Path] = true
	}
	part, ok, rejected, e := v.paging.part(v.paging.owner, eligibilityMember{NodeID: v.descriptor.NodeID, Member: pin}, v.set == nil)
	if e != nil || !ok {
		v.complete = false
		return nil, false, rejected, e
	}
	if e := v.paging.owner.b.charge(1, 256+uint64(len(pin.Path)+len(pin.Role)+len(pin.SHA256)), "verified-page-part"); e != nil {
		return nil, false, false, e
	}
	v.parts = append(v.parts, pin)
	return part, true, false, nil
}

func (b *eligibilityBudget) pageHash(h hash.Hash, pin ReplayMember, part func(ReplayMember) ([]byte, bool, *Error), v *verifiedPageSource, at string) (bool, *Error) {
	if v != nil {
		p, ok, rejected, e := v.load(pin)
		if e != nil {
			return false, e
		}
		if rejected && v.set == nil {
			// The ownership finding is already recorded once; the callback would repeat it.
			return false, nil
		}
		if ok {
			if p.chain != nil {
				if v.stream == nil {
					v.stream = p.chain
				}
				if v.stream != p.chain || p.ordinal != len(v.parts)-1 || p.ordinal >= len(p.chain.pins) || p.chain.pins[p.ordinal] != v.parts[len(v.parts)-1] {
					return false, nil
				}
				return true, b.charge(1, 0, at)
			}
			if v.stream != nil {
				return false, nil
			}
			if e := b.charge(1, pin.Bytes+1, at); e != nil {
				return false, e
			}
			_, _ = h.Write(p.content)
			return true, nil
		}
	}
	data, ok, e := part(pin)
	if e != nil || !ok {
		return false, e
	}
	if e := b.charge(1, 2*(pin.Bytes+1), at); e != nil {
		return false, e
	}
	if uint64(len(data)) != pin.Bytes || digestHex(data) != pin.SHA256 {
		return false, nil
	}
	_, _ = h.Write(data)
	return true, nil
}

func (v *verifiedPageSource) remember(data []byte, owner, kind, partPrefix string, index int, input semanticBuildInput, logical semanticBinaryLogical) *Error {
	if v == nil || !v.complete || v.set != nil {
		return nil
	}
	if e := v.paging.owner.b.charge(1, 1024+2*(uint64(len(data))+1), "verified-logical-record"); e != nil {
		return e
	}
	if uint64(len(data)) != v.descriptor.Member.Bytes || digestHex(data) != v.descriptor.Member.SHA256 {
		return fail(KindInvalidInput, "ELIGIBILITY_PAGING_IDENTITY", v.descriptor.Member.Path, nil)
	}
	v.paging.records[v.descriptor.Member.Path] = verifiedLogical{descriptor: v.descriptor, data: string(data), owner: owner, kind: kind,
		partPrefix: partPrefix, index: index, input: input, logical: logical, parts: v.parts, cohort: v.cohort}
	return nil
}

func (v *verifiedPageSource) reuse(data []byte, owner, kind, partPrefix string, index int, input semanticBuildInput) (semanticBinaryLogical, bool, bool, *Error) {
	if v == nil || v.set == nil || !v.paging.finished {
		return semanticBinaryLogical{}, false, false, nil
	}
	r, found := v.paging.records[v.descriptor.Member.Path]
	if !found {
		return semanticBinaryLogical{}, false, false, nil
	}
	if e := v.paging.owner.b.charge(uint64(len(data))+1, 256, "verified-logical-rebind"); e != nil {
		return semanticBinaryLogical{}, true, false, e
	}
	valid, e := v.paging.valid(v.paging.owner)
	if e != nil {
		return semanticBinaryLogical{}, true, false, e
	}
	if !valid || r.descriptor != v.descriptor || r.data != string(data) || r.owner != owner || r.kind != kind || r.partPrefix != partPrefix || r.index != index || r.input != input || r.cohort != v.cohort {
		return semanticBinaryLogical{}, true, false, nil
	}
	for _, pin := range r.parts {
		pin.Path = pin.Path[len(v.prefix):]
		if _, ok, _, e := v.load(pin); e != nil || !ok {
			return semanticBinaryLogical{}, true, false, e
		}
	}
	return r.logical, true, true, nil
}

func (b *eligibilityBudget) setPageSource(state *semanticSetState, pin OracleMember, pins map[string]OracleMember) (*verifiedPageSource, *Error) {
	if state == nil || state.paging == nil {
		return nil, nil
	}
	s := state.paging.owner
	if s.b != b {
		return nil, fail(KindInvalidInput, "ELIGIBILITY_PAGING_IDENTITY", pin.Path, nil)
	}
	valid, e := state.paging.valid(s)
	if e != nil {
		return nil, e
	}
	if !valid {
		return nil, fail(KindInvalidInput, "ELIGIBILITY_PAGING_IDENTITY", pin.Path, nil)
	}
	p := eligibilityMember{NodeID: s.owners["oracle/"+pin.Path], Member: ReplayMember{Path: "oracle/" + pin.Path, Role: pin.Role, Bytes: pin.Bytes, SHA256: pin.SHA256}}
	v, e := s.pageSource(p, "oracle/", nil)
	if v != nil {
		v.set = pins
	}
	return v, e
}
