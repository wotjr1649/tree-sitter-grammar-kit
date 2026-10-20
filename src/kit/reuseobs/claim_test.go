package reuseobs

import (
	"strings"
	"testing"
)

func TestObservationClaimIndependent(t *testing.T) {
	o, _ := sample()
	inc := o
	inc.Role, inc.Step, inc.OldTree = "incremental", 1, true
	old := strings.Repeat("c", 64)
	inc.OldSourceSHA256 = &old
	inc.Counts = &Counts{ParseAfterEdit: 1, Done: 1}
	fresh := o
	fresh.Role, fresh.Step = "fresh", 1
	omit := fresh
	omit.Role = "incremental"
	freshOld := inc
	freshOld.Role = "fresh"
	missing := o
	missing.Status, missing.Reason, missing.Events, missing.Bytes, missing.Counts = "UNAVAILABLE", "LOGGER_MISSING", 0, 0, &Counts{}
	partial := o
	partial.Status, partial.Reason = "TRUNCATED", "MESSAGE_LIMIT"
	overflow := o
	overflow.Status, overflow.Reason, overflow.Counts = "OVERFLOW", "COUNTER_OVERFLOW", nil
	host := missing
	host.Reason = "NO_NATIVE_PARSE"
	freshAccept := fresh
	freshAccept.Events, freshAccept.Counts = 3, &Counts{NewParse: 1, Done: 1, Accepted: 1}
	for _, tc := range []struct {
		name         string
		observations []Observation
		want         string
	}{
		{"empty", nil, "NOT_CLAIMED"},
		{"initial", []Observation{o}, "PASS"},
		{"actual-old", []Observation{o, inc, fresh}, "PASS"},
		{"omit-old", []Observation{omit}, "FAIL"},
		{"fresh-old", []Observation{freshOld}, "FAIL"},
		{"fresh-accept", []Observation{freshAccept}, "FAIL"},
		{"missing", []Observation{missing}, "BLOCKED"},
		{"partial", []Observation{partial}, "BLOCKED"},
		{"overflow", []Observation{overflow}, "BLOCKED"},
		{"host-only", []Observation{host}, "NOT_CLAIMED"},
		{"host-and-native", []Observation{o, host}, "PASS"},
		{"fail-dominates", []Observation{omit, partial}, "FAIL"},
		{"unchecked-incomplete", []Observation{{Status: "COMPLETE", Role: "initial", Counts: &Counts{NewParse: 1}}}, "BLOCKED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Judge(tc.observations); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestMandatoryObservationClaim(t *testing.T) {
	o, _ := sample()
	inc := o
	inc.Role, inc.Step, inc.OldTree = "incremental", 1, true
	old := strings.Repeat("c", 64)
	inc.OldSourceSHA256 = &old
	inc.Counts = &Counts{ParseAfterEdit: 1, Done: 1}
	fresh := o
	fresh.Role, fresh.Step = "fresh", 1
	omit := fresh
	omit.Role = "incremental"
	freshOld := inc
	freshOld.Role = "fresh"
	freshAccept := fresh
	freshAccept.Events, freshAccept.Counts = 3, &Counts{NewParse: 1, Done: 1, Accepted: 1}
	missing := inc
	missing.Status, missing.Reason, missing.Events, missing.Bytes, missing.Counts = "UNAVAILABLE", "LOGGER_MISSING", 0, 0, &Counts{}
	missingNoOld := missing
	missingNoOld.OldTree, missingNoOld.OldSourceSHA256 = false, nil
	unknown := inc
	unknown.Status, unknown.Reason = "UNAVAILABLE", "FORMAT_UNKNOWN"
	notRun := missingNoOld
	notRun.Reason = "PARSE_NOT_RUN"
	partial := inc
	partial.Status, partial.Reason, partial.Counts = "TRUNCATED", "MESSAGE_LIMIT", &Counts{ParseAfterEdit: 1}
	partialNoStart := partial
	partialNoStart.Counts = &Counts{Done: 1}
	partialDone := partial
	partialDone.Events, partialDone.Counts = 3, &Counts{ParseAfterEdit: 1, Done: 2}
	partialFresh := fresh
	partialFresh.Status, partialFresh.Reason, partialFresh.Counts = "TRUNCATED", "MESSAGE_LIMIT", &Counts{NewParse: 1}
	partialFreshAccept := partialFresh
	partialFreshAccept.Events, partialFreshAccept.Counts = 3, &Counts{NewParse: 1, Accepted: 1}
	overflow := inc
	overflow.Status, overflow.Reason, overflow.Counts = "OVERFLOW", "COUNTER_OVERFLOW", nil
	host := o
	host.Status, host.Reason, host.Events, host.Bytes, host.Counts = "UNAVAILABLE", "NO_NATIVE_PARSE", 0, 0, &Counts{}
	for _, tc := range []struct {
		name             string
		observations     []Observation
		completed, early string
	}{
		{"empty", nil, "NOT_CLAIMED", "NOT_CLAIMED"},
		{"actual-old", []Observation{o, inc, fresh}, "PASS", "PASS"},
		{"omit-old", []Observation{omit}, "FAIL", "FAIL"},
		{"fresh-old", []Observation{freshOld}, "FAIL", "FAIL"},
		{"fresh-accept", []Observation{freshAccept}, "FAIL", "FAIL"},
		{"logger-missing", []Observation{missing}, "FAIL", "BLOCKED"},
		{"logger-missing-without-old-tree", []Observation{missingNoOld}, "FAIL", "FAIL"},
		{"format-unknown", []Observation{unknown}, "FAIL", "BLOCKED"},
		{"parse-not-run", []Observation{notRun}, "FAIL", "BLOCKED"},
		{"truncated-with-start", []Observation{partial, partialFresh}, "PASS", "PASS"},
		{"truncated-without-start", []Observation{partialNoStart}, "BLOCKED", "BLOCKED"},
		{"truncated-with-done", []Observation{partialDone}, "BLOCKED", "BLOCKED"},
		{"truncated-fresh-accept", []Observation{partialFreshAccept}, "FAIL", "FAIL"},
		{"overflow", []Observation{overflow}, "BLOCKED", "BLOCKED"},
		{"host-only", []Observation{host}, "NOT_CLAIMED", "NOT_CLAIMED"},
		{"invalid", []Observation{{Status: "COMPLETE", Role: "initial", Counts: &Counts{NewParse: 1}}}, "FAIL", "FAIL"},
		{"fail-dominates", []Observation{partialNoStart, omit}, "FAIL", "FAIL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Mandatory(tc.observations, true); got != tc.completed {
				t.Fatalf("completed: got %s want %s", got, tc.completed)
			}
			if got := Mandatory(tc.observations, false); got != tc.early {
				t.Fatalf("early: got %s want %s", got, tc.early)
			}
		})
	}
}

func TestCaseClaimRequiresRoles(t *testing.T) {
	o, _ := sample()
	inc := o
	inc.Role, inc.Step, inc.OldTree = "incremental", 1, true
	old := strings.Repeat("c", 64)
	inc.OldSourceSHA256 = &old
	inc.Counts = &Counts{ParseAfterEdit: 1, Done: 1}
	fresh := o
	fresh.Role, fresh.Step = "fresh", 1
	marker := fresh
	marker.Status, marker.Reason, marker.Events, marker.Bytes, marker.Counts = "UNAVAILABLE", "NO_NATIVE_PARSE", 0, 0, &Counts{}
	markerInc := marker
	markerInc.Role = "incremental"
	for _, tc := range []struct {
		name             string
		steps            []Step
		completed, early string
	}{
		{"native", []Step{{true, false, []Observation{o}}, {true, true, []Observation{inc, fresh}}}, "PASS", "PASS"},
		{"restart", []Step{{true, false, []Observation{o}}}, "PASS", "PASS"},
		{"gap", []Step{{true, false, []Observation{o}}, {false, true, []Observation{markerInc, marker}}}, "PASS", "PASS"},
		{"missing-role", []Step{{true, true, []Observation{fresh}}}, "FAIL", "BLOCKED"},
		{"swapped-roles", []Step{{true, true, []Observation{fresh, inc}}}, "FAIL", "BLOCKED"},
		{"missing-step", []Step{{true, true, nil}}, "FAIL", "BLOCKED"},
		{"marker-as-native", []Step{{true, true, []Observation{markerInc, marker}}}, "FAIL", "BLOCKED"},
		{"native-as-gap", []Step{{false, true, []Observation{inc, fresh}}}, "FAIL", "BLOCKED"},
		{"restart-as-edit", []Step{{true, true, []Observation{o}}}, "FAIL", "BLOCKED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CaseClaim(tc.steps, true); got != tc.completed {
				t.Fatalf("completed: got %s want %s", got, tc.completed)
			}
			if got := CaseClaim(tc.steps, false); got != tc.early {
				t.Fatalf("early: got %s want %s", got, tc.early)
			}
		})
	}
}
