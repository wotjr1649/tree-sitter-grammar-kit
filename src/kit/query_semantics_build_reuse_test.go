package kit

import (
	"os"
	"path/filepath"
	"testing"
)

// The semantic set reader must accept a build cache miss and a hit, and reject every other reuse shape.
func TestSemanticSetBuildReuseShapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		build  func(*semanticBuildDescription)
		reject bool
	}{
		{"no reuse", func(b *semanticBuildDescription) {}, false},
		{"miss", func(b *semanticBuildDescription) { b.Reuse = &semanticBuildReuse{SourceSteps: []semanticBuildStep{}} }, false},
		{"hit", func(b *semanticBuildDescription) {
			b.Reuse, b.Steps = &semanticBuildReuse{Hit: true, SourceSteps: b.Steps}, []semanticBuildStep{}
		}, false},
		{"miss with source steps", func(b *semanticBuildDescription) { b.Reuse = &semanticBuildReuse{SourceSteps: b.Steps} }, true},
		{"miss with null source steps", func(b *semanticBuildDescription) { b.Reuse = &semanticBuildReuse{} }, true},
		{"miss without steps", func(b *semanticBuildDescription) {
			b.Reuse, b.Steps = &semanticBuildReuse{SourceSteps: []semanticBuildStep{}}, []semanticBuildStep{}
		}, true},
		{"hit with steps", func(b *semanticBuildDescription) { b.Reuse = &semanticBuildReuse{Hit: true, SourceSteps: b.Steps} }, true},
		{"hit with null source steps", func(b *semanticBuildDescription) {
			b.Reuse, b.Steps = &semanticBuildReuse{Hit: true}, []semanticBuildStep{}
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, f, _ := semanticBoundFixtureWith(t, 0, true, semanticBoundOptions{Build: c.build})
			set := VerifyOracleSet(os.DirFS(filepath.Join(f.root, "oracle")))
			found := false
			for _, finding := range set.Findings {
				found = found || finding.Code == "SEMANTICS_BUILD_STEPS" && finding.Path == "capture/build.json"
			}
			if c.reject && (set.Valid || !found) || !c.reject && (!set.Valid || found) {
				t.Fatalf("valid=%v steps finding=%v, reject=%v: %+v", set.Valid, found, c.reject, set.Findings)
			}
		})
	}
}
