package kit

import (
	"strings"
	"testing"
)

func TestOrdinaryReplayVerifiedIdentityReuse(t *testing.T) {
	t.Parallel()
	_, root, profile := ordinaryPartialReplayFixture(t)
	selection := "ordinary-selection"
	members := profile["members"].([]ReplayMember)
	for i := range members {
		if members[i].Role == selection {
			// Forge the member and outer identity together; the byte hash must still reject it.
			members[i].SHA256 = strings.Repeat("a", 64)
			profile["identities"].(map[string]string)[selection] = members[i].SHA256
		}
	}
	got, err := Replay(ordinaryReplayContext(t), ReplayRequest{Root: root, Profile: ordinaryJSON(t, profile)})
	if err != nil || got.Assessment != AssessFail || !containsOrdinaryFinding(got.Findings, "MEMBER_MISMATCH") {
		t.Fatalf("verified pin reuse bypassed the physical byte hash: %v", err)
	}
}

func TestOrdinaryReplayObservedIdentityStillCompared(t *testing.T) {
	t.Parallel()
	_, root, profile := ordinaryPartialReplayFixture(t)
	for _, role := range []string{"workload", "ordinary-selection", "ordinary-registration", "ordinary-assets", "ordinary-plan-commitment", "registry", "semantics-run-binding", "evidence-policy"} {
		t.Run(role, func(t *testing.T) {
			identities := profile["identities"].(map[string]string)
			original := identities[role]
			identities[role] = strings.Repeat("a", 64)
			defer func() { identities[role] = original }()
			got, err := Replay(ordinaryReplayContext(t), ReplayRequest{Root: root, Profile: ordinaryJSON(t, profile)})
			if err != nil || got.Assessment != AssessFail || !containsOrdinaryFinding(got.Findings, "IDENTITY_MISMATCH") {
				t.Fatalf("outer identity mismatch disappeared: %v %+v", err, got.Findings)
			}
		})
	}
}
