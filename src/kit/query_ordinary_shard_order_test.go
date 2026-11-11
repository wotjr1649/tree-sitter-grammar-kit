package kit

import (
	"slices"
	"testing"
)

// The manifest lists cases in profile (registration) order and selected shard IDs in plan order. They
// are bound by ID, so an order that differs between the two is valid while any other binding is not.
func TestOrdinaryManifestBindsShardIDsToCasesByID(t *testing.T) {
	registration := OrdinaryRegistration{Route: "route", Platform: "windows-amd64"}
	cases := []string{"c-feature", "a-alt", "b-alt"} // registration order
	key := func(c string) string { return ordinaryPlanShardKey(registration.Route, registration.Platform, c) }
	profile := OracleProfile{}
	for _, c := range cases {
		profile.Native.Cases = append(profile.Native.Cases, IncrementalCase{ID: c})
	}
	sorted := []string{key("a-alt"), key("b-alt"), key("c-feature")} // plan order
	check := func(manifestCases, shardIDs []string) (string, int) {
		findings := 0 // findings of the order check only; the member checks that follow are not under test
		m := OrdinaryOracleManifest{Cases: manifestCases, ShardIDs: shardIDs}
		e := ordinaryCheckManifestMembers(m, registration, profile, nil, func(code, _, _ string) {
			if code == "ORDINARY_CASE_ORDER" {
				findings++
			}
		})
		if e == nil {
			return "", findings
		}
		return e.Code, findings
	}
	if code, findings := check(cases, sorted); code != "" || findings != 0 {
		t.Fatalf("registration-ordered cases with plan-ordered shard IDs were refused: %q %d", code, findings)
	}
	for name, test := range map[string]struct {
		cases, shards []string
		code          string
	}{
		"shard IDs in case order":       {cases, []string{key("c-feature"), key("a-alt"), key("b-alt")}, "ORDINARY_CASE_ORDER"},
		"shard IDs swapped":             {cases, []string{sorted[1], sorted[0], sorted[2]}, "ORDINARY_CASE_ORDER"},
		"shard IDs descending":          {cases, []string{sorted[2], sorted[1], sorted[0]}, "ORDINARY_CASE_ORDER"},
		"duplicated shard ID":           {cases, []string{sorted[0], sorted[0], sorted[2]}, "ORDINARY_CASE_ORDER"},
		"substituted shard ID":          {cases, []string{sorted[0], sorted[1], key("d-other")}, "ORDINARY_CASE_ORDER"},
		"foreign platform shard":        {cases, []string{sorted[0], sorted[1], "route/linux-amd64/c-feature"}, "ORDINARY_CASE_ORDER"},
		"omitted shard ID":              {cases, sorted[:2], "ORDINARY_CASE_COUNT"},
		"extra shard ID":                {cases, append(slices.Clone(sorted), key("z-extra")), "ORDINARY_CASE_COUNT"},
		"cases differ from the profile": {[]string{"a-alt", "c-feature", "b-alt"}, sorted, "ORDINARY_CASE_ORDER"},
		"duplicated case":               {[]string{"c-feature", "c-feature", "b-alt"}, sorted, "ORDINARY_CASE_ORDER"},
	} {
		if code, findings := check(test.cases, test.shards); code != test.code || (test.code == "ORDINARY_CASE_ORDER") != (findings > 0) {
			t.Errorf("%s: code %q findings %d, want %q", name, code, findings, test.code)
		}
	}
}
