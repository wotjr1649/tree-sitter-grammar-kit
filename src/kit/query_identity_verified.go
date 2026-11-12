package kit

import "strings"

type verifiedMemberIdentity struct {
	owner  string
	member ReplayMember
}

func identityOnlyRole(role string) bool {
	switch role {
	case "driver", "native-executable", "reference-executable", "runtime-library", "compiler":
		return true
	default:
		return false
	}
}

// Roles alone do not prove that an approved registration will never request the bytes.
func (s *eligibilitySession) retainedCandidates() (map[string]bool, *Error) {
	var retained map[string]bool
	var err *Error
	add := func(pin *eligibilityMember) {
		if err != nil || pin == nil {
			return
		}
		err = s.b.charge(1, 0, "member-retention-plan")
		if err != nil || !identityOnlyRole(pin.Member.Role) && !pagingPartRole(pin.Member.Role) || retained[pin.Member.Path] {
			return
		}
		err = s.b.charge(1, 256+uint64(len(pin.Member.Path)), "member-retention-record")
		if err == nil {
			if retained == nil {
				retained = map[string]bool{}
			}
			retained[pin.Member.Path] = true
		}
	}
	for _, tool := range s.reg.Tools {
		add(&tool.IdentityMember)
	}
	if g := s.reg.Build; g != nil {
		add(&g.Identity)
		add(&g.SourceSnapshot)
		add(&g.VendorSnapshot)
	}
	if g := s.reg.Control; g != nil {
		add(&g.Summary)
		for _, pin := range g.Grammar {
			add(&pin)
		}
	}
	if n := s.reg.Native; n != nil {
		add(&n.Source)
		add(&n.BuildManifest)
		for _, pin := range n.Grammar {
			add(&pin)
		}
	}
	for _, inv := range s.reg.Invocations {
		for _, pin := range []*eligibilityMember{inv.Request, inv.Receipt, inv.Response, inv.Process} {
			add(pin)
		}
		for key, input := range inv.Inputs {
			if key == "source" || strings.HasPrefix(key, "query-") {
				add(input.MemberPin)
			}
		}
	}
	return retained, err
}

func (s *eligibilitySession) memberIdentity(pin *eligibilityMember) (verifiedMemberIdentity, bool, *Error) {
	ok, e := s.memberPin(pin)
	if e != nil || !ok {
		return verifiedMemberIdentity{}, false, e
	}
	if identity, found := s.snapshot.identities[pin.Member.Path]; found {
		if !identityOnlyRole(identity.member.Role) || identity.owner != pin.NodeID || identity.member != pin.Member {
			return verifiedMemberIdentity{}, false, fail(KindInvalidInput, "ELIGIBILITY_MEMBER_IDENTITY", pin.Member.Path, nil)
		}
		return identity, true, nil
	}
	if member, found := s.snapshot.verifiedMembers[pin.Member.Path]; found && member == pin.Member {
		return verifiedMemberIdentity{owner: pin.NodeID, member: member}, true, nil
	}
	s.finding("ELIGIBILITY_MEMBER_MISSING", pin.Member.Path, true)
	return verifiedMemberIdentity{}, false, nil
}

func (s *eligibilitySession) memberAvailable(pin *eligibilityMember) (bool, *Error) {
	_, ok, e := s.memberIdentity(pin)
	return ok, e
}
