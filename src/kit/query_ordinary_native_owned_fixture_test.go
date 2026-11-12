package kit

import (
	"encoding/json"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// This uses an actual r8 from the owned native producer and supplies only a test-local
// eligibility approval envelope; it does not claim an externally approved ordinary run.
func testOrdinaryNativeClosureOwnedFixture(t *testing.T, fixture ordinaryOwnedReaderFixture, set *ordinaryGuardedSet) {
	t.Helper()
	manifest := set.Manifest
	fixture.registration.Candidate = manifest.Candidate
	fixture.registration.SemanticCohort = manifest.SemanticCohort
	ordinary := fixture.s.ordinary.Binding
	fixture.s.approval.Ordinary = &ordinary
	candidate := eligibilityCandidate{SourceRevision: strings.Repeat("a", 40), Platform: manifest.Producer["platform"], Run: manifest.RunID, Attempt: 1}
	current := eligibilityCohort{ID: ordinary.SemanticCohort, Purpose: "current", SourceRevision: candidate.SourceRevision, Platform: candidate.Platform, Run: candidate.Run, Attempt: candidate.Attempt}
	fixture.s.approval.Candidate = candidate
	fixture.s.approval.Cohorts = []eligibilityCohort{current}
	fixture.s.cohorts = map[string]eligibilityCohort{current.ID: current}
	compiler := fixture.profile.Native.Compiler
	identityBytes := []byte("approved test compiler identity summary")
	identityPin := ReplayMember{Path: "approval/current/compiler-identity.json", Role: "compiler", Bytes: uint64(len(identityBytes)), SHA256: digestHex(identityBytes)}
	identityOwner := eligibilityMember{NodeID: "node-current-compiler", Member: identityPin}
	fixture.s.owned[identityPin.Path] = identityOwner
	fixture.s.owners[identityPin.Path] = identityOwner.NodeID
	fixture.s.snapshot.verifiedMembers[identityPin.Path] = identityPin
	if fixture.s.snapshot.identities == nil {
		fixture.s.snapshot.identities = map[string]verifiedMemberIdentity{}
	}
	fixture.s.snapshot.identities[identityPin.Path] = verifiedMemberIdentity{owner: identityOwner.NodeID, member: identityPin}
	fixture.s.reg.Tools = []eligibilityTool{{ID: "current-compiler", CohortID: current.ID, ToolName: compiler.Name, SubjectSHA256: compiler.SHA256, IdentityMember: identityOwner}}
	runtimeManifest, err := fs.ReadFile(drivers.NativeC, "native-c/runtime-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	policyText := []byte(NativePolicyText(NativeOperations()["native-query-batch"], OutputTree))
	plan := OrdinaryProfile{Candidate: manifest.Candidate, SemanticCohort: manifest.SemanticCohort,
		NativePolicy: queryregistrycore.FilePin("policy/native-query-batch.txt", policyText),
		Bindings: []OrdinaryBinding{{Route: fixture.registration.Route, Platform: fixture.registration.Platform, Subject: fixture.registration.Subject,
			ClosureEncoding: fixture.registration.ClosureEncoding, Grammar: fixture.profile.Native.Grammar,
			Runtime: queryregistrycore.FilePin("runtime/runtime-manifest.json", runtimeManifest), Members: fixture.registration.PlannedMembers}},
	}
	in := ordinaryVerifiedInputs{profile: plan, registration: fixture.registration,
		route: QualRoute{Workload: QualWorkload{Grammar: fixture.profile.Native.Grammar}}}
	check := func(candidateSet *ordinaryGuardedSet) (bool, *Error, []Finding) {
		findings := []Finding{}
		passed, e := fixture.b.ordinaryNativeClosure(fixture.root, fixture.s, in, fixture.profile, candidateSet, func(code, path, message string) {
			findings = append(findings, Finding{Severity: "error", Code: code, Path: path, Message: message})
		})
		return passed, e, findings
	}
	passed, e, findings := check(set)
	if e != nil || !passed || len(findings) != 0 {
		t.Fatalf("owned r8 native closure: pass=%v err=%v findings=%+v", passed, e, findings)
	}
	testOrdinaryNativeClosureBuildReuse(t, fixture, set, check)
	approvedCohort := fixture.s.cohorts[ordinary.SemanticCohort]
	mutated := approvedCohort
	mutated.SourceRevision = strings.Repeat("b", 40)
	fixture.s.cohorts[approvedCohort.ID] = mutated
	passed, e, findings = check(set)
	fixture.s.cohorts[approvedCohort.ID] = approvedCohort
	if e != nil || passed || !containsOrdinaryFinding(findings, "ORDINARY_BUILD_COHORT") {
		t.Fatalf("current cohort mutation was accepted: pass=%v err=%v findings=%+v", passed, e, findings)
	}
	compilerSHA := fixture.s.reg.Tools[0].SubjectSHA256
	fixture.s.reg.Tools[0].SubjectSHA256 = digestHex([]byte("wrong compiler"))
	passed, e, findings = check(set)
	fixture.s.reg.Tools[0].SubjectSHA256 = compilerSHA
	if e != nil || passed || !containsOrdinaryFinding(findings, "ORDINARY_BUILD_COMPILER") {
		t.Fatalf("compiler approval mutation was accepted: pass=%v err=%v findings=%+v", passed, e, findings)
	}
	badSet := *set
	badSet.Manifest.Members = append([]OrdinaryMember(nil), set.Manifest.Members...)
	changed := false
	for i := range badSet.Manifest.Members {
		member := &badSet.Manifest.Members[i]
		if member.Owner == "D_build" && member.Role == "grammar-parser" {
			member.Pin.SHA256 = digestHex([]byte("different grammar"))
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("owned r8 has no directly pinned grammar input")
	}
	passed, e, findings = check(&badSet)
	if e != nil || passed || !containsOrdinaryFinding(findings, "ORDINARY_BUILD_INPUT") {
		t.Fatalf("grammar closure pin mutation was accepted: pass=%v err=%v findings=%+v", passed, e, findings)
	}
}

func containsOrdinaryFinding(findings []Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

// testOrdinaryNativeClosureBuildReuse replaces the producer's build cache record in the owned r8 with each
// reuse shape and requires the reader to accept exactly the miss, hit and no-reuse shapes.
func testOrdinaryNativeClosureBuildReuse(t *testing.T, fixture ordinaryOwnedReaderFixture, set *ordinaryGuardedSet, check func(*ordinaryGuardedSet) (bool, *Error, []Finding)) {
	t.Helper()
	const full = "oracle/capture/build.json"
	original := fixture.s.snapshot.files[full]
	ownedBuild, originalVerified := fixture.s.owned[full], fixture.s.snapshot.verifiedMembers[full]
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(original, &doc); err != nil {
		t.Fatal(err)
	}
	steps := string(doc["steps"])
	var parts []json.RawMessage
	if err := json.Unmarshal(doc["steps"], &parts); err != nil || len(parts) < 2 {
		t.Fatalf("producer build steps: err=%v steps=%d", err, len(parts))
	}
	parts[0], parts[1] = parts[1], parts[0]
	reordered, err := json.Marshal(parts)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, steps, reuse string
		reject             bool
	}{
		{"miss", steps, `{"key":"k","hit":false,"source_steps":[]}`, false},
		{"hit", `[]`, `{"key":"k","hit":true,"source_steps":` + steps + `}`, false},
		{"miss with reordered steps", string(reordered), `{"key":"k","hit":false,"source_steps":[]}`, true},
		{"miss with source steps", steps, `{"key":"k","hit":false,"source_steps":` + steps + `}`, true},
		{"miss with null source steps", steps, `{"key":"k","hit":false,"source_steps":null}`, true},
		{"miss without steps", `[]`, `{"key":"k","hit":false,"source_steps":[]}`, true},
		{"hit with steps", steps, `{"key":"k","hit":true,"source_steps":` + steps + `}`, true},
		{"hit with null source steps", `[]`, `{"key":"k","hit":true,"source_steps":null}`, true},
	}
	for _, c := range cases {
		mutated := maps.Clone(doc)
		mutated["steps"], mutated["reuse"] = json.RawMessage(c.steps), json.RawMessage(c.reuse)
		data, err := json.Marshal(mutated)
		if err != nil {
			t.Fatal(err)
		}
		pin := ReplayMember{Path: full, Role: "build", Bytes: uint64(len(data)), SHA256: digestHex(data)}
		fixture.s.owned[full] = eligibilityMember{NodeID: ownedBuild.NodeID, Member: pin}
		fixture.s.snapshot.verifiedMembers[full], fixture.s.snapshot.files[full] = pin, data
		candidate := *set
		candidate.Manifest.Members = slices.Clone(set.Manifest.Members)
		for i := range candidate.Manifest.Members {
			if candidate.Manifest.Members[i].Path == "capture/build.json" {
				candidate.Manifest.Members[i].Pin = OrdinaryPin{Path: "capture/build.json", Bytes: pin.Bytes, SHA256: pin.SHA256}
			}
		}
		passed, e, findings := check(&candidate)
		if e != nil || c.reject != containsOrdinaryFinding(findings, "ORDINARY_BUILD_STEPS") || c.reject == passed || (!c.reject && len(findings) != 0) {
			t.Errorf("%s: reject=%v pass=%v err=%v findings=%+v", c.name, c.reject, passed, e, findings)
		}
	}
	fixture.s.owned[full] = ownedBuild
	fixture.s.snapshot.verifiedMembers[full], fixture.s.snapshot.files[full] = originalVerified, original
}
