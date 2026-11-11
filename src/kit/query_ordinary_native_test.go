package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestOrdinaryBuildMemberIdentityUsesSameSnapshotPin(t *testing.T) {
	f := newOrdinaryReaderFixture(t)
	path := "oracle/capture/driver.bin"
	data := []byte("driver")
	pin := ReplayMember{Path: path, Role: "driver", Bytes: uint64(len(data)), SHA256: digestHex(data)}
	owned := eligibilityMember{NodeID: "node-D_build", Member: pin}
	f.s.owned[path], f.s.owners[path] = owned, owned.NodeID
	f.s.snapshot.verifiedMembers[path] = pin
	if f.s.snapshot.identities == nil {
		f.s.snapshot.identities = map[string]verifiedMemberIdentity{}
	}
	f.s.snapshot.identities[path] = verifiedMemberIdentity{owner: owned.NodeID, member: pin}
	members := map[string]OrdinaryMember{"capture/driver.bin": {Owner: "D_build", Path: "capture/driver.bin", Role: "driver", CaseOrBatch: "build", Pin: OrdinaryPin{Path: "capture/driver.bin", Bytes: pin.Bytes, SHA256: pin.SHA256}}}
	got, ok, e := f.b.ordinaryBuildMemberIdentity(f.s, members, "capture/driver.bin", "driver")
	if e != nil || !ok || got.Pin.SHA256 != pin.SHA256 {
		t.Fatalf("valid identity-only driver pin rejected: ok=%v err=%v member=%+v", ok, e, got)
	}
	members["capture/driver.bin"] = OrdinaryMember{Owner: "O", Path: "capture/driver.bin", Role: "driver", CaseOrBatch: "case", Pin: OrdinaryPin{Path: "capture/driver.bin", Bytes: pin.Bytes, SHA256: pin.SHA256}}
	if _, ok, e := f.b.ordinaryBuildMemberIdentity(f.s, members, "capture/driver.bin", "driver"); e != nil || ok {
		t.Fatalf("foreign driver owner accepted: ok=%v err=%v", ok, e)
	}
}

func TestOrdinaryBuildPageHashAccountsForActualBytes(t *testing.T) {
	f := newOrdinaryReaderFixture(t)
	data := []byte("streamed ordinary build part")
	path := "capture/build-input-00000.part-00000.bin"
	full := "oracle/" + path
	pin := ReplayMember{Path: path, Role: "grammar-paged-part", Bytes: uint64(len(data)), SHA256: digestHex(data)}
	member := OrdinaryMember{Owner: "D_build", Path: path, Role: pin.Role, CaseOrBatch: "build", Pin: OrdinaryPin{Path: path, Bytes: pin.Bytes, SHA256: pin.SHA256}}
	f.s.owned[full] = eligibilityMember{NodeID: "node-D_build", Member: ReplayMember{Path: full, Role: pin.Role, Bytes: pin.Bytes, SHA256: pin.SHA256}}
	f.s.owners[full] = "node-D_build"
	f.s.snapshot.verifiedMembers[full] = f.s.owned[full].Member
	f.s.snapshot.files[full] = data
	beforeWork, beforeLogical := f.b.work, f.b.logical
	h := sha256.New()
	ok, e := f.b.ordinaryBuildPageHash(h, pin, nil, map[string]OrdinaryMember{path: member}, f.s)
	if e != nil || !ok || hex.EncodeToString(h.Sum(nil)) != pin.SHA256 {
		t.Fatalf("valid ordinary part hash failed: ok=%v err=%v", ok, e)
	}
	if f.b.work-beforeWork < uint64(len(data)) || f.b.logical-beforeLogical < uint64(2*len(data)) {
		t.Fatalf("part hash undercharged actual bytes: work=%d logical=%d bytes=%d", f.b.work-beforeWork, f.b.logical-beforeLogical, len(data))
	}
	pin.SHA256 = digestHex([]byte("different"))
	if ok, e := f.b.ordinaryBuildPageHash(sha256.New(), pin, nil, map[string]OrdinaryMember{path: member}, f.s); e != nil || ok {
		t.Fatalf("foreign part pin accepted: ok=%v err=%v", ok, e)
	}
}

func TestOrdinaryBuildPageHashReusesVerifiedStreamedPart(t *testing.T) {
	f := newOrdinaryReaderFixture(t)
	data := []byte("verified streamed page")
	path := "capture/build-input-00000.part-00000.bin"
	full := "oracle/" + path
	file, err := f.root.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := f.root.Stat(full)
	if err != nil {
		t.Fatal(err)
	}
	pin := ReplayMember{Path: full, Role: "grammar-paged-part", Bytes: uint64(len(data)), SHA256: digestHex(data)}
	f.s.owned[full] = eligibilityMember{NodeID: "node-D_build", Member: pin}
	f.s.owners[full] = "node-D_build"
	f.s.snapshot.verifiedMembers[full] = pin
	f.s.snapshot.observed[full] = info
	chain := &streamedChain{Hash: sha256.New(), pins: []ReplayMember{pin}, next: 1}
	_, _ = chain.Write(data)
	if f.s.snapshot.parts == nil {
		f.s.snapshot.parts = map[string]*guardedPart{}
	}
	f.s.snapshot.parts[full] = &guardedPart{root: f.root, path: full, info: info, size: pin.Bytes, sha256: pin.SHA256, chain: chain, ordinal: 0}
	descriptorPath := "oracle/capture/build-input-00000.pages.json"
	descriptor := []byte(`{"complete":true}`)
	descriptorPin := ReplayMember{Path: descriptorPath, Role: "grammar-paged-descriptor", Bytes: uint64(len(descriptor)), SHA256: digestHex(descriptor)}
	f.s.owned[descriptorPath] = eligibilityMember{NodeID: "node-D_build", Member: descriptorPin}
	f.s.owners[descriptorPath] = "node-D_build"
	f.s.snapshot.verifiedMembers[descriptorPath] = descriptorPin
	f.s.snapshot.files[descriptorPath] = descriptor
	ordinary := f.s.ordinary.Binding
	f.s.approval.Ordinary = &ordinary
	candidate := eligibilityCandidate{SourceRevision: strings.Repeat("a", 40), Platform: "windows/amd64", Run: "run", Attempt: 1}
	cohort := eligibilityCohort{ID: ordinary.SemanticCohort, Purpose: "current", SourceRevision: candidate.SourceRevision, Platform: candidate.Platform, Run: candidate.Run, Attempt: candidate.Attempt}
	f.s.approval.Candidate = candidate
	f.s.approval.Cohorts = []eligibilityCohort{cohort}
	f.s.cohorts = map[string]eligibilityCohort{cohort.ID: cohort}
	paging := &verifiedPaging{owner: f.s, r: f.run, b: f.b, snapshot: f.s.snapshot, root: f.root, records: map[string]verifiedLogical{}}
	f.s.snapshot.paging = paging
	var bindingErr *Error
	paging.binding, bindingErr = f.s.pagingBinding()
	if bindingErr != nil {
		t.Fatal(bindingErr)
	}
	member := OrdinaryMember{Owner: "D_build", Path: "capture/build-input-00000.pages.json", Role: "grammar-paged-descriptor", CaseOrBatch: "build", Pin: OrdinaryPin{Path: "capture/build-input-00000.pages.json", Bytes: descriptorPin.Bytes, SHA256: descriptorPin.SHA256}}
	source, sourceErr := ordinaryBuildPageSource(f.s, member)
	if sourceErr != nil || source == nil {
		t.Fatalf("verified page source: source=%v err=%v", source, sourceErr)
	}
	startWork, startLogical := f.b.work, f.b.logical
	h := sha256.New()
	ok, e := f.b.ordinaryBuildPageHash(h, ReplayMember{Path: path, Role: pin.Role, Bytes: pin.Bytes, SHA256: pin.SHA256}, source, map[string]OrdinaryMember{}, f.s)
	if e != nil || !ok || source.chainSum(h) != pin.SHA256 || hex.EncodeToString(h.Sum(nil)) == pin.SHA256 {
		t.Fatalf("verified streamed part hash: ok=%v err=%v", ok, e)
	}
	if f.b.work == startWork || f.b.logical-startLogical < uint64(len(data)) {
		t.Fatalf("verified cached part was not charged: work=%d logical=%d bytes=%d", f.b.work-startWork, f.b.logical-startLogical, len(data))
	}
}
