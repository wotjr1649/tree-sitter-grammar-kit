package kit

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
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
	// Codex A217F: a regular limit ending binds its raw response like live Check, and a
	// record beyond the read limits leaves its raw responses unverified, never unbound.
	early := func(c map[string]any, code string) {
		c["execution_status"], c["response_status"], c["response_code"], c["assessment"], c["code"] = StatusResourceLimit, StatusResourceLimit, code, AssessBlocked, code
		c["claims"] = map[string]string{"incremental_equality": claimNotClaimed, "incremental_route": claimNotClaimed, "expectations": claimNotClaimed}
		c["oracle_claims"] = map[string]string{"query_equality": claimNotClaimed, "query_expectations": claimNotClaimed, "api": claimNotClaimed, "fact_reproduction": claimNotClaimed, "dynamic_sql": claimNotClaimed}
	}
	queryLimited := func(r []map[string]any) { // a completed tree whose query hit a limit
		early(r[0], "CAPTURE_LIMIT")
		r[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"] = []any{map[string]any{"id": "q.x", "status": StatusResourceLimit,
			"code": "CAPTURE_LIMIT", "evaluation": "NOT_EVALUATED", "captures": nil}}
		r[0]["expectations"] = []any{}
		observations := r[0]["steps"].([]any)[0].(map[string]any)["observations"].([]reuseobs.Observation)
		r[0]["reuse_observation_claim"] = reuseobs.CaseClaim([]reuseobs.Step{{Native: true, Observations: observations}}, false)
	}
	t.Run("early-end-other-code", func(t *testing.T) { // record and raw agree on a code the case did not end with
		f := newQfx(t)
		f.inv.ExtraRoles[0].Workloads[0].Cases[0].ExpectStatus = StatusResourceLimit
		r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": {record: func(set string, records []map[string]any) {
			if set == "s06-fxh" {
				queryLimited(records)
				records[0]["response_code"] = "MATCH_LIMIT"
			}
		}}}))
		row := roleRow(r, "fx-history", "windows-amd64")
		if len(row.Sets) != 1 || !slices.ContainsFunc(row.Sets[0].Gates, func(g ReplayGate) bool {
			return g.ID == "reuse-observation" && g.Code == "OBSERVATION_RESPONSE_UNBOUND" && g.Failed > 0
		}) {
			t.Fatalf("an end the case did not record bound: %+v", row)
		}
	})
	for _, tc := range []struct {
		name   string
		record func(records []map[string]any)
	}{
		{"query-limited-step", queryLimited},
		{"stepless-limit", func(r []map[string]any) { // OUTPUT_LIMIT drops every step
			early(r[0], "OUTPUT_LIMIT")
			r[0]["steps"], r[0]["expectations"], r[0]["reuse_observation_claim"] = []any{}, []any{}, claimNotClaimed
		}},
		{"stepless-query-time-limit", func(r []map[string]any) { // the workload registers q.x
			early(r[0], "QUERY_TIME_LIMIT")
			r[0]["steps"], r[0]["expectations"], r[0]["reuse_observation_claim"] = []any{}, []any{}, claimNotClaimed
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newQfx(t)
			f.inv.ExtraRoles[0].Workloads[0].Cases[0].ExpectStatus = StatusResourceLimit // a registered limit ending
			r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": {record: func(set string, records []map[string]any) {
				if set == "s06-fxh" {
					tc.record(records)
				}
			}}}))
			row := roleRow(r, "fx-history", "windows-amd64")
			if len(row.Sets) != 1 {
				t.Fatalf("%s: %+v", tc.name, row)
			}
			for _, g := range row.Sets[0].Gates {
				if g.ID == "reuse-observation" && g.Failed > 0 {
					t.Fatalf("%s: %+v", tc.name, g)
				}
			}
			if codes := codes(row.Sets[0].Findings); slices.Contains(codes, "RESPONSE_UNBOUND") || row.Mechanism == AssessFail {
				t.Fatalf("%s: mechanism %s %v %+v", tc.name, row.Mechanism, codes, row.Sets[0].Gates)
			}
		})
	}
	t.Run("stepless-query-time-limit-without-queries", func(t *testing.T) { // live Check needs a query
		f := newQfx(t)
		f.inv.ExtraRoles[0].Workloads[0].Queries = nil
		f.inv.ExtraRoles[0].Workloads[0].Cases[0].ExpectStatus = StatusResourceLimit
		r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": {record: func(set string, records []map[string]any) {
			if set == "s06-fxh" {
				early(records[0], "QUERY_TIME_LIMIT")
				records[0]["steps"], records[0]["expectations"], records[0]["reuse_observation_claim"] = []any{}, []any{}, claimNotClaimed
			}
		}}}))
		row := roleRow(r, "fx-history", "windows-amd64")
		if len(row.Sets) != 1 || !slices.ContainsFunc(row.Sets[0].Gates, func(g ReplayGate) bool {
			return g.ID == "reuse-observation" && g.Code == "OBSERVATION_RESPONSE_UNBOUND" && g.Failed > 0
		}) {
			t.Fatalf("query time limit without queries bound: %+v", row)
		}
	})
	t.Run("record-beyond-limit", func(t *testing.T) {
		f := newQfx(t)
		limit := QualificationLimits().FileBytes
		var record string
		hosts := f.all(t, map[string]qfxMut{"windows-amd64": {manifest: func(set string, m map[string]any) {
			if set != "s06-fxa" {
				return
			}
			members := m["members"].([]OracleMember)
			i := slices.IndexFunc(members, func(mem OracleMember) bool { return mem.Role == "record" && mem.Case == "c2" })
			members[i].Bytes, record = limit+1, members[i].Path
		}}})
		if err := os.Truncate(filepath.Join(hosts["windows-amd64"], "records", "s06-fxa", filepath.FromSlash(record)), int64(limit+1)); err != nil {
			t.Fatal(err)
		}
		cell := cellOf(f.run(t, hosts), "fxa", "windows-amd64")
		if cell.Mechanism != AssessBlocked || slices.Contains(setCodes(cell), "RESPONSE_UNBOUND") {
			t.Fatalf("record beyond limit: mechanism %s %v", cell.Mechanism, setCodes(cell))
		}
	})
	for _, tc := range []struct {
		name, code string // code: the reuse-observation gate code, or a set finding
		record     func(records []map[string]any)
		manifest   func(m map[string]any)
		raw        func(raw map[string]any)
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
		{name: "stepless-completed-raw", code: "OBSERVATION_RESPONSE_UNBOUND", record: func(r []map[string]any) { // no limit response claims completion
			early(r[0], "OUTPUT_LIMIT")
			r[0]["steps"], r[0]["expectations"], r[0]["reuse_observation_claim"], r[0]["response_status"] = []any{}, []any{}, claimNotClaimed, StatusCompleted
		}},
		{name: "stepless-bogus-status", code: "OBSERVATION_RESPONSE_UNBOUND", record: func(r []map[string]any) { // no live end
			early(r[0], "OUTPUT_LIMIT")
			r[0]["steps"], r[0]["expectations"], r[0]["reuse_observation_claim"], r[0]["response_status"] = []any{}, []any{}, claimNotClaimed, "BOGUS"
		}},
		{name: "stepless-other-code", code: "OBSERVATION_RESPONSE_UNBOUND", record: func(r []map[string]any) { // a parse limit keeps its steps
			early(r[0], "PARSE_TIME_LIMIT")
			r[0]["steps"], r[0]["expectations"], r[0]["reuse_observation_claim"] = []any{}, []any{}, claimNotClaimed
		}},
		{name: "raw-other-end", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) { // a completed record, a failed raw response
			if raw["id"] == "c1" {
				raw["status"], raw["code"] = StatusFailed, "PARSE_NULL"
			}
		}},
		{name: "bogus-end-both", code: "OBSERVATION_RESPONSE_UNBOUND", record: func(r []map[string]any) { // record and raw agree on no live end
			r[0]["response_status"], r[0]["response_code"] = "BOGUS", "X"
		}},
		{name: "raw-step-failed", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) { // a completed response with an unfinished step
			if raw["id"] == "c2" {
				raw["steps"].([]any)[1].(map[string]any)["incremental"].(map[string]any)["status"] = StatusFailed
			}
		}},
		{name: "raw-query-limited", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "q1" {
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"] = []any{qfxWireQuery("q.x", StatusResourceLimit, "CAPTURE_LIMIT")}
			}
		}},
		// Codex A217J: the raw tree slots and query streams live Check accepts
		{name: "raw-trees-missing", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c2" {
				for _, s := range raw["steps"].([]any) {
					delete(s.(map[string]any), "incremental")
					delete(s.(map[string]any), "fresh")
				}
			}
		}},
		{name: "raw-fresh-missing", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c2" {
				delete(raw["steps"].([]any)[1].(map[string]any), "fresh")
			}
		}},
		{name: "raw-first-fresh", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c2" {
				first := raw["steps"].([]any)[0].(map[string]any)
				first["fresh"] = first["incremental"]
			}
		}},
		{name: "raw-query-invalid-code", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "q1" {
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"] = []any{qfxWireQuery("q.x", "INVALID_QUERY", "QUERY_BOGUS")}
			}
		}},
		{name: "raw-query-code-missing", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "q1" {
				q := qfxWireQuery("q.x", StatusCompleted, "")
				delete(q, "code")
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"] = []any{q}
			}
		}},
		{name: "raw-queries-missing", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				delete(raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any), "queries")
			}
		}},
		{name: "raw-queries-duplicated", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				q := qfxWireQuery("q.x", StatusCompleted, "")
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"] = []any{q, q}
			}
		}},
		{name: "raw-api-empty", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) { // review on #250
			if raw["id"] == "c1" {
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["api"] = map[string]any{}
			}
		}},
		{name: "raw-api-member-missing", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				delete(raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["api"].(map[string]any), "names")
			}
		}},
		{name: "raw-api-rows", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) { // a row per node
			if raw["id"] == "c1" {
				api := raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["api"].(map[string]any)
				api["nodes"] = api["nodes"].([]any)[1:]
			}
		}},
		// review on #250: the form, API tables, producer capabilities and request source live Check binds
		{name: "raw-form-bogus", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				tree := raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
				tree["form"], tree["api"] = "BOGUS", nil // no full tree, so no API payload is due
			}
		}},
		{name: "raw-api-child-fields", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["api"].(map[string]any)["child_fields"] = "x"
			}
		}},
		{name: "raw-api-field-index", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["api"].(map[string]any)["field_lookups"] = []any{[]int64{0, 0, 0}}
			}
		}},
		// review on #250: the members and first-step shape live DecodeResponse and Check accept
		{name: "raw-first-step-route", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["steps"].([]any)[0].(map[string]any)["route"] = map[string]any{"edit_has_changes": false, "edited_root_end_byte": 0, "reused_nodes": 0, "fresh_reused_nodes": 0}
			}
		}},
		{name: "raw-unknown-member", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["bogus"] = true
			}
		}},
		{name: "raw-unknown-producer-member", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["producer"].(map[string]any)["bogus"] = true
			}
		}},
		{name: "raw-unknown-step-member", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["steps"].([]any)[0].(map[string]any)["bogus"] = true
			}
		}},
		{name: "raw-unknown-tree-member", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["bogus"] = true
			}
		}},
		{name: "raw-producer-abi-type", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["producer"].(map[string]any)["language_version"] = "15"
			}
		}},
		{name: "raw-tree-scalar-type", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["has_error"] = "no"
			}
		}},
		{name: "raw-tree-nodes-type", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["nodes"] = "x"
			}
		}},
		{name: "raw-edit-route-type", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c2" {
				raw["steps"].([]any)[1].(map[string]any)["route"] = "x"
			}
		}},
		{name: "raw-edit-unknown-member", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c2" {
				raw["steps"].([]any)[1].(map[string]any)["edit"].(map[string]any)["bogus"] = true
			}
		}},
		{name: "historical-manifest", code: "OBSERVATION_CAPABILITY_MISMATCH", record: func(r []map[string]any) {
			for _, c := range r { // every record, raw and observation consistently on 659cda7's verified manifest
				c["producer"].(map[string]any)["reuse_observation"].(map[string]any)["runtime_manifest_sha256"] = qfxHistoricalManifest
				for _, s := range c["steps"].([]any) {
					if os, ok := s.(map[string]any)["observations"].([]reuseobs.Observation); ok {
						for i := range os {
							os[i].RuntimeManifestSHA256 = qfxHistoricalManifest
						}
					}
				}
			}
		}},
		{name: "raw-digest-short", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["digest"] = "00"
			}
		}},
		{name: "raw-producer-query", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "q1" {
				raw["producer"].(map[string]any)["query"] = "UNSUPPORTED"
			}
		}},
		{name: "raw-producer-api", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["producer"].(map[string]any)["api"] = "UNSUPPORTED"
			}
		}},
		{name: "raw-source-bytes", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				raw["source_bytes"] = 0
			}
		}},
		{name: "raw-response-code-missing", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) {
			if raw["id"] == "c1" {
				delete(raw, "code")
			}
		}},
		{name: "raw-query-bogus", code: "OBSERVATION_RESPONSE_UNBOUND", raw: func(raw map[string]any) { // no wire query status
			if raw["id"] == "q1" {
				raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["queries"] = []any{qfxWireQuery("q.x", "BOGUS", "")}
			}
		}},
		{name: "stepless-raw-removed", code: "OBSERVATION_RESPONSE_MISSING", record: func(r []map[string]any) {
			early(r[0], "OUTPUT_LIMIT")
			r[0]["steps"], r[0]["expectations"], r[0]["reuse_observation_claim"] = []any{}, []any{}, claimNotClaimed
		}, manifest: func(m map[string]any) { // the response leaves the set with its member
			m["members"] = slices.DeleteFunc(m["members"].([]OracleMember), func(mem OracleMember) bool { return mem.Role == "raw" && mem.Case == "c1" })
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
			}, raw: func(set string, raw map[string]any) {
				if set == "s06-fxa" && tc.raw != nil {
					tc.raw(raw)
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

// steplessEnds selects the raw responses that dropped every step: a plain case without
// steps by its response end, the last native segment of an early SVC case without a
// recorded step of it by the case's end; nothing for a case or segment with steps.
func TestSteplessEnds(t *testing.T) {
	segments := []rcSvcSegment{{StartStep: 0, EndStep: 1}, {StartStep: 1, EndStep: 2}}
	step := []observationStep{{}}
	for _, tc := range []struct {
		name   string
		c      rcCase
		groups map[int][]observationStep
		want   map[string][2]string
	}{
		{"plain", rcCase{ID: "c", ResponseStatus: StatusResourceLimit, ResponseCode: "OUTPUT_LIMIT"}, nil,
			map[string][2]string{"raw/00003-c.json": {StatusResourceLimit, "OUTPUT_LIMIT"}}},
		{"plain-with-steps", rcCase{ID: "c", Steps: []rcStep{{}}}, map[int][]observationStep{0: step}, map[string][2]string{}},
		{"svc-last-segment", rcCase{ID: "c", Segments: segments, ExecutionStatus: StatusResourceLimit, Code: "OUTPUT_LIMIT", Steps: []rcStep{{}}},
			map[int][]observationStep{0: step}, map[string][2]string{"raw/00003-c-segment-00001.json": {StatusResourceLimit, "OUTPUT_LIMIT"}}},
		{"svc-completed", rcCase{ID: "c", Segments: segments, ExecutionStatus: StatusCompleted}, map[int][]observationStep{0: step}, map[string][2]string{}},
		{"svc-last-segment-with-steps", rcCase{ID: "c", Segments: segments, ExecutionStatus: StatusResourceLimit}, map[int][]observationStep{0: step, 1: step}, map[string][2]string{}},
	} {
		if got := steplessEnds(&tc.c, tc.groups, "raw/", 3); !maps.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// An API payload is a full tree's answer to an API workload only: one on a workload without
// API observations is unrequested, as live checkAPI rejects it (review on #250).
func TestQualificationRawAPIUnrequested(t *testing.T) {
	f := newQfx(t)
	f.inv.Routes[0].Workload.API = false
	r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": {profile: func(set string, prof map[string]any) {
		if set == "s06-fxa" {
			prof["api"] = false
		}
	}}}))
	cell := cellOf(r, "fxa", "windows-amd64")
	if !slices.ContainsFunc(cell.Set.Gates, func(g ReplayGate) bool {
		return g.ID == "reuse-observation" && g.Code == "OBSERVATION_RESPONSE_UNBOUND" && g.Failed > 0
	}) {
		t.Fatalf("unrequested API payload bound: %s %v %+v", cell.Mechanism, setCodes(cell), cell.Set.Gates)
	}
}

// A raw API payload answers the registered case's API points, one row at each point's byte,
// and the tree form binds as decoded, as live Check reads them (Codex A217L).
func TestQualificationRawAPIPoints(t *testing.T) {
	answered := []any{[]int64{0, 0, 0, 0}}
	for _, tc := range []struct {
		name string
		rows []any
		form any
		bad  bool
	}{{"answered", answered, nil, false}, {"escaped-form", answered, json.RawMessage(`"\u0066ull"`), false},
		{"missing", []any{}, nil, true}, {"other-byte", []any{[]int64{1, 0, 0, 0}}, nil, true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newQfx(t)
			f.inv.Routes[0].Workload.Cases[0].Points = []NativePoint{{ID: "P0", Byte: 0}}
			r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": {raw: func(set string, raw map[string]any) {
				if set == "s06-fxa" && raw["id"] == "c1" {
					tree := raw["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
					tree["api"].(map[string]any)["points"] = tc.rows
					if tc.form != nil {
						tree["form"] = tc.form
					}
				}
			}}}))
			cell := cellOf(r, "fxa", "windows-amd64")
			unbound := slices.ContainsFunc(cell.Set.Gates, func(g ReplayGate) bool {
				return g.ID == "reuse-observation" && g.Code == "OBSERVATION_RESPONSE_UNBOUND" && g.Failed > 0
			})
			if unbound != tc.bad || !tc.bad && cell.Mechanism != "PASS" {
				t.Fatalf("API points %v: %s %v %+v", tc.rows, cell.Mechanism, setCodes(cell), cell.Set.Gates)
			}
		})
	}
}

// A completed raw tree counts its nodes within the operation's limit, as live checkTree
// requires; the control keeps the count (review on #250).
func TestQualificationRawCountLimit(t *testing.T) {
	for _, over := range []bool{false, true} {
		f := newQfx(t)
		f.inv.Routes[0].Workload.API = false // no API payload, so only the count bound judges it
		r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": {profile: func(set string, prof map[string]any) {
			if set == "s06-fxa" {
				prof["api"] = false
			}
		}, raw: func(set string, raw map[string]any) {
			if set != "s06-fxa" {
				return
			}
			for _, s := range raw["steps"].([]any) {
				for _, slot := range []string{"incremental", "fresh"} {
					if tree, ok := s.(map[string]any)[slot].(map[string]any); ok && tree["api"] != nil {
						tree["api"] = nil
						if over && raw["id"] == "c1" {
							tree["descendant_count"] = NativeOperations()["native-query"].Nodes + 1
						}
					}
				}
			}
		}}}))
		cell := cellOf(r, "fxa", "windows-amd64")
		unbound := slices.ContainsFunc(cell.Set.Gates, func(g ReplayGate) bool {
			return g.ID == "reuse-observation" && g.Code == "OBSERVATION_RESPONSE_UNBOUND" && g.Failed > 0
		})
		if unbound != over {
			t.Fatalf("count over limit %v: %s %v %+v", over, cell.Mechanism, setCodes(cell), cell.Set.Gates)
		}
	}
}

// qfxHistoricalManifest is 659cda7's runtime manifest, verified for recorded replays only.
const qfxHistoricalManifest = "affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080"
