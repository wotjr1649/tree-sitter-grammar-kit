package kit

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestQueryClassificationDartPHP12Local(t *testing.T) {
	if !classificationDartPHP12ProfileName(os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE")) {
		t.Skip("explicit closed DartPHP12 classification profile required")
	}
	classificationLocal(t)
}

func TestQueryClassificationDartPHP12ProfilesAndClosure(t *testing.T) {
	var definitions, patterns, entries, cases, edits, steps, roles, links uint64
	names := make(map[string]bool)
	for _, name := range []string{"dart7-empty", "php5-empty"} {
		t.Run(name, func(t *testing.T) {
			if !classificationGoLocalSkipsProfile(name) || !classificationGoDefinitionProfileAllowed(name) ||
				classificationWeb16ProfileName(name) || classificationScript17ProfileName(name) || classificationSmall17ProfileName(name) {
				t.Fatal("dedicated profile selection differs")
			}
			if _, err := classificationCJRProfileForName(name); err == nil {
				t.Fatal("profile entered CJR")
			}
			fixture := classificationEmptyStaticFixtureForTest(t, name)
			profile, anchors := fixture.profile, fixture.anchors
			if anchors.API || !fixture.inputs.workload.API || anchors.Source.UTF8 != "" || anchors.Source.Bytes != 0 || len(anchors.Nodes) != 1 ||
				anchors.nativeContext() != "full-input-utf8/r2" || anchors.nativeSymbol() != profile.Symbol || classificationNativePrefix(anchors) != "current/native/" ||
				!classificationPlanContextMatches(anchors, fixture.plan) {
				t.Fatal("empty anchor or ordinary workload differs")
			}
			routeProfile, ok := classificationEmptyProfileForRoute(anchors.Route)
			if !ok || routeProfile.Name != name {
				t.Fatal("guarded empty-profile route selection differs")
			}
			prefix := ""
			includes := []string{"src"}
			if profile.Route == "php" {
				prefix = "php/"
				includes = []string{"php/src", "common"}
			}
			want := []string{"-shared", "-O0", "-std=gnu11"}
			for _, dir := range includes {
				want = append(want, "-I", filepath.Join(fixture.grammarRoot, dir))
			}
			output := filepath.Join(fixture.plan.LibraryRequest.Dir, "library", profile.Route+".dll")
			want = append(want, filepath.Join(fixture.grammarRoot, prefix+"src/parser.c"), filepath.Join(fixture.grammarRoot, prefix+"src/scanner.c"), "-o", output)
			args, err := classificationEmptyCompilerArgs(profile, fixture.grammarRoot, output, fixture.plan.Grammar)
			if err != nil || !reflect.DeepEqual(args, want) || !reflect.DeepEqual(args, fixture.plan.LibraryRequest.Args) {
				t.Fatal("compiler closure differs", err)
			}
			if profile.Route == "php" {
				if len(fixture.plan.Grammar) != 6 || fixture.plan.Grammar[0].Path != "common/scanner.h" || fixture.plan.Grammar[0].Role != "header" {
					t.Fatal("PHP sixth closure role missing")
				}
				for _, index := range []int{0, 1, 2} {
					bad := slices.Clone(fixture.plan.Grammar)
					bad[index].Path = strings.TrimPrefix(bad[index].Path, "php/") + ".alias"
					if _, err := classificationEmptyCompilerArgs(profile, fixture.grammarRoot, output, bad); err == nil {
						t.Fatal("PHP nested/common alias accepted", index)
					}
				}
				badPlan := fixture.plan
				badPlan.LibraryRequest.Args = slices.Clone(args)
				badPlan.LibraryRequest.Args[4] = filepath.Join(fixture.grammarRoot, "src")
				if classificationEmptyLibraryRequestValid(profile, badPlan, fixture.plan.LibraryRequest.Dir, fixture.grammarRoot) == nil {
					t.Fatal("flattened PHP include accepted")
				}
			}
			witness, native, references := classificationSmall17RowsFixture(anchors)
			rows, err := classificationEmptyDefinitionRows(anchors, fixture.inputs, fixture.sourceFiles, witness, native, references)
			if err != nil || len(rows) != profile.QueryCount {
				t.Fatal("source-bound rows rejected", err)
			}
			for i, row := range rows {
				if row.Query != anchors.Queries[i].ID || row.SourceFile != profile.QueryFile || row.Class != "STRUCTURAL_ONLY" || row.Assessment != "NOT_ASSESSED" ||
					row.SemanticRequired != nil || row.SemanticEvaluated || row.InventoryClass != "STRUCTURAL_CANDIDATE" || row.CompiledMetadata != "UNVERIFIED" || row.PotentialTreeRoles != profile.Counts.TreeRoles {
					t.Fatal("static row promoted or source binding differs", i)
				}
			}
			if _, err := classificationEmptyDefinitionProjection(anchors, fixture.plan, nil, nil, QueryReferenceEligibilityResult{},
				actualJSONRead(t, "../contracts/query-slot-inventory-r1.json"), actualJSONRead(t, "../contracts/qualification-c1.json"), fixture.queryData, strings.Repeat("d", 40)); err == nil {
				t.Fatal("projection without live guarded session accepted")
			}
			definitions += uint64(profile.QueryCount)
			cases += fixture.inputs.coordinates.Cases
			edits += fixture.inputs.coordinates.Edits
			steps += fixture.inputs.coordinates.CaseSteps
			roles += fixture.inputs.coordinates.TreeRoles
			links += fixture.inputs.coordinates.PotentialQueryTreeRoles
			for _, q := range anchors.Queries {
				patterns += uint64(q.Metadata.Patterns)
				entries += uint64(len(q.Metadata.CaptureNames))
				for _, capture := range q.Metadata.CaptureNames {
					names[capture] = true
				}
			}
		})
	}
	if definitions != 12 || patterns != 42 || entries != 43 || len(names) != 38 || cases != 101 || edits != 70 || steps != 171 || roles != 241 || links != 1407 {
		t.Fatal("query or original potential denominator differs", definitions, patterns, entries, len(names), cases, edits, steps, roles, links)
	}
	for _, name := range []string{"", "dart", "dart7-empty-alias", "php5-empty ", "powershell3-empty", "python8-empty", "javascript4-empty"} {
		if classificationDartPHP12ProfileName(name) {
			t.Fatal("unknown dedicated profile accepted", name)
		}
	}
}
