package kit

import (
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// #217: an edit workload's set is recorded through tsgk-native/r4 and the qualification
// consumes its mandatory observation like native-result-r4. A forged or unbound
// observation fails the kit axis, and so does an omitted old tree recorded honestly; a
// collection limit leaves the kit axis BLOCKED.
func TestQualificationMandatoryObservation(t *testing.T) {
	c2 := func(records []map[string]any) map[string]any {
		return records[slices.IndexFunc(records, func(c map[string]any) bool { return c["id"] == "c2" })]
	}
	omitOld := func(c map[string]any) {
		step := c["steps"].([]any)[1].(map[string]any)
		observations := step["observations"].([]reuseobs.Observation)
		observations[0].OldTree, observations[0].OldSourceSHA256, observations[0].Counts = false, nil, &reuseobs.Counts{NewParse: 1, Done: 1}
		c["reuse_observation_claim"] = claimFail
	}
	limited := func(r []map[string]any) { // c2's incremental prefix holds no start event
		c := c2(r)
		o := &c["steps"].([]any)[1].(map[string]any)["observations"].([]reuseobs.Observation)[0]
		o.Status, o.Reason, o.Events, o.Counts = "TRUNCATED", "EVENT_LIMIT", reuseobs.MaxEvents, &reuseobs.Counts{}
		c["reuse_observation_claim"], c["assessment"], c["code"] = claimBlocked, AssessBlocked, "REUSE_OBSERVATION_LIMIT"
	}
	t.Run("collection-limit", func(t *testing.T) {
		f := newQfx(t)
		r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": {record: func(set string, records []map[string]any) {
			if set == "s06-fxa" {
				limited(records)
			}
		}}}))
		if cell := cellOf(r, "fxa", "windows-amd64"); cell.Mechanism != AssessBlocked || len(setCodes(cell)) != 0 {
			t.Fatalf("collection limit: mechanism %s %v", cell.Mechanism, setCodes(cell))
		}
	})
	for _, tc := range []struct {
		name, code string // code: the reuse-observation gate code, or a set finding
		record     func(records []map[string]any)
		manifest   func(m map[string]any)
	}{
		{name: "claim", code: "OBSERVATION_CLAIM_MISMATCH", record: func(r []map[string]any) { r[0]["reuse_observation_claim"] = "FAIL" }},
		{name: "null-claim", code: "OBSERVATION_CLAIM_MISMATCH", record: func(r []map[string]any) { r[0]["reuse_observation_claim"] = nil }},
		{name: "step", code: "OBSERVATION_INVALID", record: func(r []map[string]any) { r[0]["steps"].([]any)[0].(map[string]any)["observations"] = []any{} }},
		{name: "capability", code: "OBSERVATION_CAPABILITY_MISMATCH", record: func(r []map[string]any) {
			r[0]["producer"].(map[string]any)["reuse_observation"] = map[string]any{"policy": "tsgk-reuse-log/r1"}
		}},
		{name: "omitted-old-tree-kept-pass", code: "OBSERVATION_CLAIM_MISMATCH", record: func(r []map[string]any) {
			omitOld(c2(r))
			c2(r)["reuse_observation_claim"] = claimPass
		}},
		{name: "omitted-old-tree", code: "REUSE_OBSERVATION_FAILED", record: func(r []map[string]any) {
			c := c2(r)
			omitOld(c)
			c["assessment"], c["code"] = AssessFail, "REUSE_OBSERVATION_FAILED"
		}},
		{name: "protocol-r2", code: "PROTOCOL_MISMATCH", manifest: func(m map[string]any) { m["protocol"] = "tsgk-native/r2" }},
		{name: "other-run", code: "OBSERVATION_INVALID", manifest: func(m map[string]any) { m["producer"].(map[string]string)["observation_run_id"] = "other-run" }},
		{name: "unbound-raw", code: "RESPONSE_UNBOUND", manifest: func(m map[string]any) { // never read, so never verified
			members := m["members"].([]OracleMember)
			extra := members[len(members)-1]
			extra.Path = "raw/00009-" + extra.Case + ".json"
			m["members"] = append(members, extra)
		}},
		{name: "no-run", code: "OBSERVATION_RUN_UNBOUND", manifest: func(m map[string]any) { delete(m["producer"].(map[string]string), "observation_run_id") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newQfx(t)
			mut := qfxMut{record: func(set string, records []map[string]any) {
				if set == "s06-fxa" && tc.record != nil {
					tc.record(records)
				}
			}, manifest: func(set string, m map[string]any) {
				if set == "s06-fxa" && tc.manifest != nil {
					tc.manifest(m)
				}
			}}
			r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": mut}))
			cell := cellOf(r, "fxa", "windows-amd64")
			if cell.Status == CellPass || r.SupportClaim != "BLOCKED" {
				t.Fatalf("%s passed: %s %s", tc.name, cell.Status, r.SupportClaim)
			}
			found := slices.Contains(setCodes(cell), tc.code)
			for _, g := range cell.Set.Gates {
				found = found || g.ID == "reuse-observation" && g.Code == tc.code && g.Failed > 0
			}
			if cell.Mechanism != AssessFail || !found {
				t.Fatalf("%s: mechanism %s, want %s in %v %+v", tc.name, cell.Mechanism, tc.code, setCodes(cell), cell.Set.Gates)
			}
		})
	}
}
