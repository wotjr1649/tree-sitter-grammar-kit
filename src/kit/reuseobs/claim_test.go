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
