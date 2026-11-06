package kit

import (
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestQueryClassificationScript17EntrySelectionIsDisjoint(t *testing.T) {
	for _, name := range []string{"python8-empty", "ruby4-empty", "r3-empty", "bash2-empty"} {
		if !classificationScript17ProfileName(name) || !classificationGoLocalSkipsProfile(name) || !classificationGoDefinitionProfileAllowed(name) {
			t.Fatal("Script17 definition or dedicated entry is unavailable", name)
		}
		if classificationWeb16ProfileName(name) || classificationSmall17ProfileName(name) {
			t.Fatal("Script17 profile entered another closed set", name)
		}
		if _, err := classificationCJRProfileForName(name); err == nil {
			t.Fatal("Script17 profile entered the closed CJR set")
		}
	}
	for _, name := range []string{"", "positive-batch", "javascript4-empty", "jsx3-empty", "json1-tiny", "csharp15-empty", "python", "python8-empty-alias", "ruby4-empty ", "r3", "bash2-empty2"} {
		if classificationScript17ProfileName(name) {
			t.Fatal("non-Script17 profile entered its closed entry", name)
		}
	}
}

func TestQueryClassificationScript17ProfilesAndCompilerClosure(t *testing.T) {
	t.Parallel()
	var definitions, patterns, captureEntries uint64
	var cases, edits, steps, roles, links uint64
	allCaptureNames := make(map[string]bool)
	for _, name := range []string{"python8-empty", "ruby4-empty", "r3-empty", "bash2-empty"} {
		t.Run(name, func(t *testing.T) {
			fixture := classificationEmptyStaticFixtureForTest(t, name)
			profile, anchors := fixture.profile, fixture.anchors
			if anchors.API || !fixture.inputs.workload.API || anchors.Source.UTF8 != "" || anchors.Source.Bytes != 0 || len(anchors.Nodes) != 1 ||
				anchors.Nodes[0].Type != profile.RootType || anchors.nativeContext() != "full-input-utf8/r2" || anchors.nativeSymbol() != profile.Symbol ||
				classificationNativePrefix(anchors) != "current/native/" || !classificationPlanContextMatches(anchors, fixture.plan) {
				t.Fatal("Script17 empty anchor or ordinary API workload binding differs")
			}
			if !reflect.DeepEqual(profile.CompilerIncludeDirs, []string{"src"}) || !reflect.DeepEqual(profile.CompilerSourceFiles, []string{"src/parser.c", "src/scanner.c"}) {
				t.Fatal("Script17 compiler closure differs")
			}
			output := filepath.Join(fixture.plan.LibraryRequest.Dir, "library", profile.Route+".dll")
			args, err := classificationEmptyCompilerArgs(profile, fixture.grammarRoot, output, fixture.plan.Grammar)
			wantArgs := []string{
				"-shared", "-O0", "-std=gnu11", "-I", filepath.Join(fixture.grammarRoot, "src"),
				filepath.Join(fixture.grammarRoot, "src/parser.c"), filepath.Join(fixture.grammarRoot, "src/scanner.c"), "-o", output,
			}
			if err != nil || !reflect.DeepEqual(args, wantArgs) || !reflect.DeepEqual(args, fixture.plan.LibraryRequest.Args) {
				t.Fatal("Script17 compiler argv differs", err)
			}
			wrongOutput := slices.Clone(args)
			wrongOutput[len(wrongOutput)-1] = filepath.Join(fixture.plan.LibraryRequest.Dir, "library", "other.dll")
			for _, badArgs := range [][]string{wrongOutput, append(slices.Clone(args), "-DUNAPPROVED")} {
				badPlan := fixture.plan
				badPlan.LibraryRequest.Args = badArgs
				if classificationEmptyLibraryRequestValid(profile, badPlan, fixture.plan.LibraryRequest.Dir, fixture.grammarRoot) == nil {
					t.Fatal("Script17 compiler output alias or unapproved argument accepted")
				}
			}
			badGrammar := slices.Clone(fixture.plan.Grammar)
			badGrammar[1].Role = "header"
			if _, err := classificationEmptyCompilerArgs(profile, fixture.grammarRoot, filepath.Join(fixture.plan.LibraryRequest.Dir, "library", profile.Route+".dll"), badGrammar); err == nil {
				t.Fatal("Script17 scanner role alias accepted")
			}
			badPlan := fixture.plan
			badPlan.LibraryRequest.Env = slices.Clone(fixture.plan.LibraryRequest.Env)
			badPlan.LibraryRequest.Env[0] = "COMSPEC="
			if classificationEmptyLibraryRequestValid(profile, badPlan, fixture.plan.LibraryRequest.Dir, fixture.grammarRoot) == nil {
				t.Fatal("empty Script17 compiler environment value accepted")
			}
			witness, native, references := classificationSmall17RowsFixture(anchors)
			rows, err := classificationEmptyDefinitionRows(anchors, fixture.inputs, fixture.sourceFiles, witness, native, references)
			if err != nil || len(rows) != profile.QueryCount {
				t.Fatal("Script17 source-bound static rows rejected", err)
			}
			for i, row := range rows {
				if row.Query != anchors.Queries[i].ID || row.SourceFile != profile.QueryFile || row.Class != "STRUCTURAL_ONLY" || row.Assessment != "NOT_ASSESSED" ||
					row.SemanticRequired != nil || row.SemanticEvaluated || row.InventoryClass != "STRUCTURAL_CANDIDATE" || row.CompiledMetadata != "UNVERIFIED" ||
					row.PotentialTreeRoles != profile.Counts.TreeRoles || row.Metadata.Predicates == nil || len(row.Metadata.Predicates) != 0 {
					t.Fatal("Script17 static row was promoted or lost its source binding", i)
				}
			}
			if _, err := classificationEmptyDefinitionProjection(anchors, fixture.plan, nil, nil, QueryReferenceEligibilityResult{},
				actualJSONRead(t, "../contracts/query-slot-inventory-r1.json"), actualJSONRead(t, "../contracts/qualification-c1.json"), fixture.queryData, strings.Repeat("d", 40)); err == nil {
				t.Fatal("Script17 projection without the live guarded session accepted")
			}
			definitions += uint64(profile.QueryCount)
			cases += fixture.inputs.coordinates.Cases
			edits += fixture.inputs.coordinates.Edits
			steps += fixture.inputs.coordinates.CaseSteps
			roles += fixture.inputs.coordinates.TreeRoles
			links += fixture.inputs.coordinates.PotentialQueryTreeRoles
			for _, query := range anchors.Queries {
				patterns += uint64(query.Metadata.Patterns)
				captureEntries += uint64(len(query.Metadata.CaptureNames))
				for _, name := range query.Metadata.CaptureNames {
					allCaptureNames[name] = true
				}
			}
		})
	}
	if definitions != 17 || patterns != 52 || captureEntries != 59 || len(allCaptureNames) != 47 ||
		cases != 127 || edits != 140 || steps != 267 || roles != 407 || links != 2097 {
		t.Fatal("Script17 query metadata or original potential-coordinate denominator differs",
			definitions, patterns, captureEntries, len(allCaptureNames), cases, edits, steps, roles, links)
	}
}

func TestQueryClassificationScript17RejectsSourceAliasesAndReusedReferences(t *testing.T) {
	t.Parallel()
	inventory := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
	qualification := actualJSONRead(t, "../contracts/qualification-c1.json")
	for _, name := range []string{"python8-empty", "ruby4-empty", "r3-empty", "bash2-empty"} {
		t.Run(name, func(t *testing.T) {
			fixture := classificationEmptyStaticFixtureForTest(t, name)
			anchors := fixture.anchors
			for _, mutate := range []func(*classificationAnchors){
				func(a *classificationAnchors) { a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0] },
				func(a *classificationAnchors) { a.Required[0], a.Required[1] = a.Required[1], a.Required[0] },
				func(a *classificationAnchors) { a.Queries[0].Metadata.Predicates = nil },
				func(a *classificationAnchors) { a.Queries[0].Metadata.Predicates = []QueryPredicate{{}} },
				func(a *classificationAnchors) { a.Queries[0].Metadata.CaptureNames = nil },
				func(a *classificationAnchors) { a.Nodes[0].Type = "wrong_root" },
				func(a *classificationAnchors) { a.ABI.FieldCount++ },
			} {
				bad := anchors
				bad.Queries = slices.Clone(anchors.Queries)
				bad.Required = slices.Clone(anchors.Required)
				bad.Nodes = slices.Clone(anchors.Nodes)
				mutate(&bad)
				if classificationEmptyAnchorsValid(fixture.profile, bad) == nil {
					t.Fatal("mutated Script17 anchor accepted")
				}
			}
			if _, _, err := classificationEmptyDefinitionLoadInputs(anchors, fixture.plan, inventory, qualification, append(slices.Clone(fixture.queryData), ' ')); err == nil {
				t.Fatal("changed Script17 raw query source accepted")
			}
			ids := make(map[string]string, len(anchors.Queries))
			for i, query := range anchors.Queries {
				ids[query.ID] = fmt.Sprintf("script17-reference-%d", i)
			}
			if ordered, err := classificationEmptyOrderedReferenceIDs(anchors, ids); err != nil || len(ordered) != fixture.profile.QueryCount {
				t.Fatal("unique ordered Script17 reference invocations rejected", err)
			}
			ids[anchors.Queries[1].ID] = ids[anchors.Queries[0].ID]
			if _, err := classificationEmptyOrderedReferenceIDs(anchors, ids); err == nil {
				t.Fatal("one reference invocation reused by two Script17 queries")
			}
			witness, native, references := classificationSmall17RowsFixture(anchors)
			witness[0].Metadata.Predicates = nil
			if _, err := classificationEmptyDefinitionRows(anchors, fixture.inputs, fixture.sourceFiles, witness, native, references); err == nil {
				t.Fatal("missing witness predicate metadata accepted")
			}
			witness, native, references = classificationSmall17RowsFixture(anchors)
			native[0].Predicates = []QueryPredicate{{}}
			if _, err := classificationEmptyDefinitionRows(anchors, fixture.inputs, fixture.sourceFiles, witness, native, references); err == nil {
				t.Fatal("nonempty compiled Script17 predicates accepted")
			}
		})
	}
}
