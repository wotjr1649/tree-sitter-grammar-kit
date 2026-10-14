package kit

import "testing"

// The existing query/API qualification revision must reject an observation it
// cannot consume, even when its byte identities and a claimed PASS are consistent.
func TestQualificationRejectsUnconsumedReuseObservation(t *testing.T) {
	for _, field := range []string{"claim", "null-claim", "step", "capability"} {
		t.Run(field, func(t *testing.T) {
			f := newQfx(t)
			mut := qfxMut{record: func(set string, records []map[string]any) {
				if set != "s06-fxa" {
					return
				}
				c := records[0]
				switch field {
				case "claim":
					c["reuse_observation_claim"] = "PASS"
				case "null-claim":
					c["reuse_observation_claim"] = nil
				case "step":
					c["steps"].([]any)[0].(map[string]any)["observations"] = []any{}
				case "capability":
					c["producer"].(map[string]any)["reuse_observation"] = map[string]any{"policy": "tsgk-reuse-log/r1"}
				}
			}}
			r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": mut}))
			cell := cellOf(r, "fxa", "windows-amd64")
			if cell.Mechanism != AssessFail || r.MechanismGate != AssessFail || r.SupportClaim != "BLOCKED" {
				t.Fatalf("ignored %s: %+v", field, r)
			}
			found := false
			for _, g := range cell.Set.Gates {
				if g.ID == "reuse-observation" && g.Code == "OBSERVATION_SCHEMA_UNSUPPORTED" && g.Failed > 0 {
					found = true
				}
			}
			if !found {
				t.Fatal("missing explicit unsupported-observation disposition")
			}
		})
	}
}
