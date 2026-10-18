package kit

import (
	"encoding/json"
	"testing"
)

// A rehashed replay report from another host must not satisfy the graph's subject.
func TestEvidenceReplaySubjectPlatform(t *testing.T) {
	for _, tc := range []struct {
		name     string
		platform any
		omit     bool
	}{
		{"same", "windows/amd64", false},
		{"other-platform", "linux/amd64", false},
		{"empty", "", false},
		{"missing", nil, true},
		{"null", nil, false},
		{"both-empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newFxGraph()
			subject := map[string]any{"run": "10", "attempt": 1}
			if !tc.omit {
				subject["platform"] = tc.platform
			}
			if tc.name == "both-empty" {
				for _, i := range []int{0, 2} {
					g.nodes[i]["identities"].(map[string]string)["platform"] = ""
				}
			}
			data, err := json.Marshal(map[string]any{
				"result_schema": ReplayResultSchema, "evidence_mode": ModeReplayedRaw, "assessment": AssessFail,
				"subject": subject,
			})
			if err != nil {
				t.Fatal(err)
			}
			g.files["replay/result.json"] = data
			g.nodes[2]["files"] = []any{map[string]any{"path": "replay/result.json", "role": "replay-result", "bytes": len(data), "sha256": sum(data)}}
			if tc.name == "same" {
				if r := g.run(t, nil); r.Assessment != AssessPass {
					t.Fatalf("same subject: %+v", r.Findings)
				}
			} else {
				evidenceFails(t, g, nil, "REPLAY_RESULT_MISMATCH")
			}
		})
	}
}
