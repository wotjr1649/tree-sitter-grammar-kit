package kit

import (
	"encoding/json"
	"testing"
)

// The wire form decides what null means: decode through the reader's own budgeted decoder.
func TestBuildDescriptionReuseWireForms(t *testing.T) {
	base, err := json.Marshal(semanticBuildDescription{Closure: []semanticBuildInput{}})
	if err != nil {
		t.Fatal(err)
	}
	step := `{"name":"version","argv":["cc"]}`
	cases := []struct {
		name, steps, reuse string
		wantOK             bool
	}{
		{"miss", `[` + step + `]`, `{"key":"k","hit":false,"source_steps":[]}`, true},
		{"hit", `[]`, `{"key":"k","hit":true,"source_steps":[` + step + `]}`, true},
		{"miss with source steps", `[` + step + `]`, `{"key":"k","hit":false,"source_steps":[` + step + `]}`, false},
		{"miss with null source steps", `[` + step + `]`, `{"key":"k","hit":false,"source_steps":null}`, false},
		{"miss without steps", `[]`, `{"key":"k","hit":false,"source_steps":[]}`, false},
		{"hit with steps", `[` + step + `]`, `{"key":"k","hit":true,"source_steps":[` + step + `]}`, false},
		{"hit with null source steps", `[]`, `{"key":"k","hit":true,"source_steps":null}`, false},
	}
	for _, c := range cases {
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(base, &doc); err != nil {
			t.Fatal(err)
		}
		doc["steps"], doc["reuse"] = json.RawMessage(c.steps), json.RawMessage(c.reuse)
		data, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		var build semanticBuildDescription
		if e := ordinaryTestBudget(t).decode("build.json", data, &build, false); e != nil {
			t.Fatalf("%s: decode: %v", c.name, e)
		}
		if _, ok := build.effectiveSteps(); ok != c.wantOK {
			t.Errorf("%s: ok=%v, want %v", c.name, ok, c.wantOK)
		}
	}
}
