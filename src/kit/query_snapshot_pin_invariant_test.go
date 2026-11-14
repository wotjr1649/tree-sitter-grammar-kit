package kit

import (
	"os"
	"path/filepath"
	"testing"
)

// #427: ordinaryNativeClosure no longer rehashes the build inputs it reads from the verified snapshot. That is
// only sound while the snapshot holds a member's bytes only after they matched the approved pin, so this test
// pins the invariant at its source: every retained byte slice is exactly the bytes of its verified member, and a
// member whose bytes differ on disk never reaches the snapshot.
func TestEligibilitySnapshotHoldsOnlyPinVerifiedBytes(t *testing.T) {
	f := newEligibilityFixture(t)
	req := f.request()
	s, result := verifyIdentityEligibility(t, req)
	if result.Assessment != AssessPass {
		t.Fatalf("verification: %+v", result.Findings)
	}
	retained := 0
	for path, data := range s.snapshot.files {
		pin, ok := s.snapshot.verifiedMembers[path]
		if !ok || uint64(len(data)) != pin.Bytes || digestHex(data) != pin.SHA256 {
			t.Fatalf("retained %s is not the bytes of its verified member: verified=%v pin=%+v len=%d", path, ok, pin, len(data))
		}
		retained++
	}
	if retained == 0 {
		t.Fatal("the fixture retained no member bytes")
	}

	source := s.reg.Native.Source
	path := filepath.Join(req.Evidence.Root, filepath.FromSlash(source.Member.Path))
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		t.Fatalf("read retained source fixture: %v", err)
	}
	data[len(data)-1] ^= 1 // same length, different bytes: only the hash can tell
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	s, result = verifyIdentityEligibility(t, req)
	if result.Assessment != AssessFail || result.Evidence == nil || !evidenceFinding(*result.Evidence, "FILE_MEMBER_MISMATCH") {
		t.Fatalf("changed retained bytes were not reported: eligibility=%+v evidence=%+v", result.Findings, result.Evidence)
	}
	if _, ok := s.snapshot.files[source.Member.Path]; ok {
		t.Fatal("changed bytes entered the snapshot")
	}
	if _, ok := s.snapshot.verifiedMembers[source.Member.Path]; ok {
		t.Fatal("changed bytes entered the verified members")
	}
	if _, ok, e := s.member(&source); ok {
		t.Fatalf("changed bytes were served as a verified member: err=%v", e)
	}
}
