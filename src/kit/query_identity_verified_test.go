package kit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQueryEligibilityIdentityOnlyLeavesReferenceBytes(t *testing.T) {
	f := newReferenceFixture(t)
	f.registration.Source.Role = "compiler"
	for i := range f.nodes {
		for j, member := range f.nodes[i].Files {
			if member.Path == f.registration.Source.Path {
				f.nodes[i].Files[j] = f.registration.Source
			}
		}
	}
	got, err := VerifyQueryReferenceConnection(ctxT(t), f.request(t))
	if err != nil || got.Assessment != AssessPass {
		t.Fatalf("legacy reference snapshot lost source bytes: %v %+v", err, got)
	}
}

func TestQueryEligibilityIdentityRoleStillRetainsSource(t *testing.T) {
	for _, role := range []string{"driver", "native-executable", "reference-executable", "runtime-library", "compiler"} {
		t.Run(role, func(t *testing.T) {
			f := newEligibilityFixture(t)
			old := f.reg.Native.Source
			pin := old
			pin.Member.Role = role
			reg := bytes.ReplaceAll(eligibilityJSON(t, f.reg), eligibilityJSON(t, old), eligibilityJSON(t, pin))
			if err := json.Unmarshal(reg, &f.reg); err != nil {
				t.Fatal(err)
			}
			for id, node := range f.nodes {
				for i, member := range node.Files {
					if member.Path == old.Member.Path {
						node.Files[i] = pin.Member
					}
				}
				f.nodes[id] = node
			}
			s, result := verifyIdentityEligibility(t, f.request())
			if result.Assessment != AssessPass {
				t.Fatalf("role changed source behavior: %+v", result.Findings)
			}
			data, ok, e := s.member(&pin)
			if e != nil || !ok || !bytes.Equal(data, f.files[pin.Member.Path]) {
				t.Fatalf("source bytes omitted: %v %v", ok, e)
			}
		})
	}
}

func verifyIdentityEligibility(t *testing.T, req QueryReferenceEligibilityRequest) (*eligibilitySession, QueryReferenceEligibilityResult) {
	t.Helper()
	root, err := os.OpenRoot(req.Evidence.Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	r, e := startRun(ctxT(t), time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(r.cancel)
	r.eligibility = &eligibilityBudget{r: r}
	scope := &eligibilityScope{r: r, root: root}
	result, err := verifyQueryReferenceEligibilityInRun(ctxT(t), req, scope)
	if err != nil || scope.session == nil {
		t.Fatalf("identity verification: %v %+v", err, result)
	}
	return scope.session, result
}

func TestQueryEligibilityIdentityOnlyMembers(t *testing.T) {
	f := newEligibilityFixture(t)
	req := f.request()
	s, result := verifyIdentityEligibility(t, req)
	if result.Assessment != AssessPass {
		t.Fatalf("identity-only verification: %+v", result)
	}
	pins := []eligibilityMember{*s.reg.Build.ReferenceBinary, *s.reg.Control.Library}
	for _, inv := range s.reg.Invocations {
		if inv.Executable != nil && identityOnlyRole(inv.Executable.Member.Role) {
			pins = append(pins, *inv.Executable)
		}
	}
	seen := map[string]bool{}
	for _, pin := range pins {
		if seen[pin.Member.Path] {
			continue
		}
		seen[pin.Member.Path] = true
		identity, ok, e := s.memberIdentity(&pin)
		if e != nil || !ok || identity.owner != pin.NodeID || identity.member != pin.Member {
			t.Fatalf("verified identity %s: ok=%v err=%v identity=%+v", pin.Member.Path, ok, e, identity)
		}
		if _, retained := s.snapshot.files[pin.Member.Path]; retained {
			t.Fatalf("identity-only bytes retained: %s", pin.Member.Path)
		}
		if _, ok, e := s.member(&pin); e == nil || e.Code != "ELIGIBILITY_MEMBER_BYTES_UNAVAILABLE" || ok {
			t.Fatalf("identity-only bytes request %s: ok=%v err=%v", pin.Member.Path, ok, e)
		}
	}
	pin := pins[0]
	original := s.snapshot.identities[pin.Member.Path]
	for _, tc := range []struct {
		name   string
		mutate func(*verifiedMemberIdentity)
	}{
		{"owner", func(v *verifiedMemberIdentity) { v.owner = "another-owner" }},
		{"role", func(v *verifiedMemberIdentity) { v.member.Role = "text" }},
		{"bytes", func(v *verifiedMemberIdentity) { v.member.Bytes++ }},
		{"sha256", func(v *verifiedMemberIdentity) { v.member.SHA256 = digestHex([]byte("other")) }},
	} {
		t.Run("identity-mutation/"+tc.name, func(t *testing.T) {
			mutated := original
			tc.mutate(&mutated)
			s.snapshot.identities[pin.Member.Path] = mutated
			defer func() { s.snapshot.identities[pin.Member.Path] = original }()
			if _, ok, e := s.memberIdentity(&pin); ok || e == nil {
				t.Fatalf("mutated identity was trusted: ok=%v err=%v", ok, e)
			}
		})
	}
	if data, ok, e := s.member(&s.reg.Native.Source); e != nil || !ok || len(data) == 0 {
		t.Fatalf("source bytes were not retained: ok=%v err=%v", ok, e)
	}
}

func TestQueryEligibilityIdentityOnlyRejectsChangedBytes(t *testing.T) {
	f := newEligibilityFixture(t)
	req := f.request()
	pin := *f.reg.Build.ReferenceBinary
	path := filepath.Join(req.Evidence.Root, filepath.FromSlash(pin.Member.Path))
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		t.Fatalf("read binary fixture: %v", err)
	}
	data[0] ^= 1
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s, result := verifyIdentityEligibility(t, req)
	if result.Assessment != AssessFail || result.Evidence == nil || !evidenceFinding(*result.Evidence, "FILE_MEMBER_MISMATCH") {
		t.Fatalf("changed identity-only bytes did not preserve the evidence finding: eligibility=%+v evidence=%+v", result.Findings, result.Evidence)
	}
	if _, ok := s.snapshot.identities[pin.Member.Path]; ok {
		t.Fatal("changed bytes produced a trusted identity-only record")
	}
	if _, ok := s.snapshot.verifiedMembers[pin.Member.Path]; ok {
		t.Fatal("changed bytes entered verified members")
	}
	if _, ok, e := s.memberIdentity(&pin); e != nil || ok {
		t.Fatalf("changed bytes identity lookup: ok=%v err=%v", ok, e)
	}
}

func evidenceFinding(result EvidenceResult, code string) bool {
	for _, finding := range result.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
