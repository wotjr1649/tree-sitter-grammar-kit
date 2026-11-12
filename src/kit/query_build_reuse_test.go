package kit

import "testing"

func TestBuildDescriptionEffectiveStepsReuseShapes(t *testing.T) {
	steps := []semanticBuildStep{{Name: "version", Argv: []string{"cc"}}}
	other := []semanticBuildStep{{Name: "link", Argv: []string{"cc"}}}
	empty := []semanticBuildStep{}
	cases := []struct {
		name   string
		build  semanticBuildDescription
		want   []semanticBuildStep
		wantOK bool
	}{
		{"no reuse", semanticBuildDescription{Steps: steps}, steps, true},
		{"miss", semanticBuildDescription{Steps: steps, Reuse: &semanticBuildReuse{SourceSteps: empty}}, steps, true},
		{"hit", semanticBuildDescription{Steps: empty, Reuse: &semanticBuildReuse{Hit: true, SourceSteps: steps}}, steps, true},
		{"miss with source steps", semanticBuildDescription{Steps: steps, Reuse: &semanticBuildReuse{SourceSteps: other}}, steps, false},
		{"miss with null source steps", semanticBuildDescription{Steps: steps, Reuse: &semanticBuildReuse{}}, steps, false},
		{"miss without steps", semanticBuildDescription{Steps: empty, Reuse: &semanticBuildReuse{SourceSteps: empty}}, empty, false},
		{"miss with null steps", semanticBuildDescription{Reuse: &semanticBuildReuse{SourceSteps: empty}}, nil, false},
		{"hit with steps", semanticBuildDescription{Steps: other, Reuse: &semanticBuildReuse{Hit: true, SourceSteps: steps}}, steps, false},
		{"hit with null source steps", semanticBuildDescription{Steps: empty, Reuse: &semanticBuildReuse{Hit: true}}, empty, false},
	}
	for _, c := range cases {
		got, ok := c.build.effectiveSteps()
		if ok != c.wantOK || len(got) != len(c.want) {
			t.Errorf("%s: ok=%v steps=%v, want ok=%v steps=%v", c.name, ok, got, c.wantOK, c.want)
		}
	}
}
