package kit

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestQueryClassificationPowerShell3Local(t *testing.T) {
	if !classificationPowerShell3ProfileName(os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE")) {
		t.Skip("explicit closed PowerShell3 classification profile required")
	}
	classificationLocal(t)
}

func classificationPowerShell3Clone(a classificationAnchors) classificationAnchors {
	a.Nodes = slices.Clone(a.Nodes)
	a.Queries = slices.Clone(a.Queries)
	a.Required = slices.Clone(a.Required)
	return a
}

func TestQueryClassificationPowerShell3ProfileAndClosure(t *testing.T) {
	const name = "powershell3-tiny"
	if !classificationGoLocalSkipsProfile(name) || !classificationGoDefinitionProfileAllowed(name) ||
		classificationWeb16ProfileName(name) || classificationScript17ProfileName(name) || classificationSmall17ProfileName(name) || classificationDartPHP12ProfileName(name) {
		t.Fatal("dedicated profile selection differs")
	}
	if _, err := classificationCJRProfileForName(name); err == nil {
		t.Fatal("profile entered CJR")
	}
	if _, ok := classificationPagedProfileForName(name); ok {
		t.Fatal("profile entered the paged family")
	}
	fixture := classificationEmptyStaticFixtureForTest(t, name)
	profile, anchors := fixture.profile, fixture.anchors
	if anchors.API || !fixture.inputs.workload.API || anchors.Source.UTF8 != ";" || anchors.Source.Bytes != 1 || len(anchors.Nodes) != 4 ||
		!reflect.DeepEqual(anchors.Nodes, classificationPowerShell3ExpectedNodes()) ||
		anchors.nativeContext() != "full-input-utf8/r2" || anchors.nativeSymbol() != profile.Symbol || anchors.libraryName() != "powershell.dll" ||
		classificationNativePrefix(anchors) != "current/native/" || !classificationPlanContextMatches(anchors, fixture.plan) {
		t.Fatal("one-byte anchor or ordinary workload differs")
	}
	if routeProfile, ok := classificationEmptyProfileForRoute(anchors.Route); !ok || routeProfile.Name != name {
		t.Fatal("guarded empty-profile route selection differs")
	}
	if fixture.inputs.coordinates.Definitions != 3 || fixture.inputs.coordinates.Cases != 39 || fixture.inputs.coordinates.Edits != 24 || fixture.inputs.coordinates.CaseSteps != 63 ||
		fixture.inputs.coordinates.TreeRoles != 87 || fixture.inputs.coordinates.PotentialQueryTreeRoles != 261 || fixture.inputs.slotGroup.Counts != profile.Counts {
		t.Fatal("query or original potential denominator differs", fixture.inputs.coordinates)
	}
	patterns, entries, names := 0, 0, make(map[string]bool)
	for _, q := range anchors.Queries {
		patterns += int(q.Metadata.Patterns)
		entries += len(q.Metadata.CaptureNames)
		for _, capture := range q.Metadata.CaptureNames {
			names[capture] = true
		}
	}
	if patterns != 6 || entries != 6 || len(names) != 6 {
		t.Fatal("query metadata denominator differs", patterns, entries, len(names))
	}
	output := filepath.Join(fixture.plan.LibraryRequest.Dir, "library", "powershell.dll")
	want := []string{"-shared", "-O0", "-std=gnu11", "-I", filepath.Join(fixture.grammarRoot, "src"),
		filepath.Join(fixture.grammarRoot, "src/parser.c"), filepath.Join(fixture.grammarRoot, "src/scanner.c"), "-o", output}
	args, err := classificationEmptyCompilerArgs(profile, fixture.grammarRoot, output, fixture.plan.Grammar)
	if err != nil || !reflect.DeepEqual(args, want) || !reflect.DeepEqual(args, fixture.plan.LibraryRequest.Args) {
		t.Fatal("compiler closure differs", err)
	}
	witness, native, references := classificationSmall17RowsFixture(anchors)
	rows, err := classificationEmptyDefinitionRows(anchors, fixture.inputs, fixture.sourceFiles, witness, native, references)
	if err != nil || len(rows) != 3 {
		t.Fatal("source-bound rows rejected", err)
	}
	for i, row := range rows {
		if row.Query != anchors.Queries[i].ID || row.SourceFile != profile.QueryFile || row.Class != "STRUCTURAL_ONLY" || row.Assessment != "NOT_ASSESSED" ||
			row.SemanticRequired != nil || row.SemanticEvaluated || row.InventoryClass != "STRUCTURAL_CANDIDATE" || row.CompiledMetadata != "UNVERIFIED" || row.PotentialTreeRoles != 87 {
			t.Fatal("static row promoted or source binding differs", i)
		}
	}
	if _, err := classificationEmptyDefinitionProjection(anchors, fixture.plan, nil, nil, QueryReferenceEligibilityResult{},
		actualJSONRead(t, "../contracts/query-slot-inventory-r1.json"), actualJSONRead(t, "../contracts/qualification-c1.json"), fixture.queryData, strings.Repeat("d", 40)); err == nil {
		t.Fatal("projection without live guarded session accepted")
	}
	inventory, qualification := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json"), actualJSONRead(t, "../contracts/qualification-c1.json")
	if _, _, err := classificationEmptyDefinitionLoadInputs(anchors, fixture.plan, inventory, qualification, append(slices.Clone(fixture.queryData), ' ')); err == nil {
		t.Fatal("changed raw query source accepted")
	}
}

func TestQueryClassificationPowerShell3RejectsSourceAndCSTMutations(t *testing.T) {
	fixture := classificationEmptyStaticFixtureForTest(t, "powershell3-tiny")
	field := "value"
	for _, mutate := range []struct {
		name string
		edit func(*classificationAnchors)
	}{
		{"empty-source", func(a *classificationAnchors) { a.Source.UTF8, a.Source.Bytes, a.Source.SHA256 = "", 0, digestHex(nil) }},
		{"trailing-newline", func(a *classificationAnchors) {
			a.Source.UTF8, a.Source.Bytes, a.Source.SHA256 = ";\n", 2, digestHex([]byte(";\n"))
		}},
		{"leading-space", func(a *classificationAnchors) {
			a.Source.UTF8, a.Source.Bytes, a.Source.SHA256 = " ;", 2, digestHex([]byte(" ;"))
		}},
		{"stale-source-pin", func(a *classificationAnchors) { a.Source.UTF8 = ";;" }},
		{"stale-source-bytes", func(a *classificationAnchors) { a.Source.Bytes = 2 }},
		{"stale-source-sha", func(a *classificationAnchors) { a.Source.SHA256 = digestHex(nil) }},
		{"root-only", func(a *classificationAnchors) { a.Nodes = a.Nodes[:1] }},
		{"missing-leaf", func(a *classificationAnchors) { a.Nodes = a.Nodes[:3] }},
		{"missing-middle", func(a *classificationAnchors) { a.Nodes = append(a.Nodes[:1:1], a.Nodes[2:]...) }},
		{"extra-node", func(a *classificationAnchors) { a.Nodes = append(a.Nodes, a.Nodes[3]) }},
		{"zero-span", func(a *classificationAnchors) { a.Nodes[3].EndByte, a.Nodes[3].EndPoint.Column = 0, 0 }},
		{"root-span", func(a *classificationAnchors) { a.Nodes[0].EndByte, a.Nodes[0].EndPoint.Column = 0, 0 }},
		{"field-set", func(a *classificationAnchors) { a.Nodes[3].Field = &field }},
		{"named-flip-statement", func(a *classificationAnchors) { a.Nodes[2].Named = false }},
		{"named-flip-token", func(a *classificationAnchors) { a.Nodes[3].Named = true }},
		{"extra-flag", func(a *classificationAnchors) { a.Nodes[1].Extra = true }},
		{"error-flag", func(a *classificationAnchors) { a.Nodes[2].HasError = true }},
		{"missing-flag", func(a *classificationAnchors) { a.Nodes[3].IsMissing = true }},
		{"parent", func(a *classificationAnchors) { a.Nodes[3].Parent = 1 }},
		{"type", func(a *classificationAnchors) { a.Nodes[2].Type = "_statement" }},
		{"root-type", func(a *classificationAnchors) { a.Nodes[0].Type = "script_file" }},
		{"abi-symbols", func(a *classificationAnchors) { a.ABI.SymbolCount-- }},
		{"abi-fields", func(a *classificationAnchors) { a.ABI.FieldCount++ }},
		{"query-order", func(a *classificationAnchors) { a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0] }},
		{"required-order", func(a *classificationAnchors) { a.Required[1], a.Required[2] = a.Required[2], a.Required[1] }},
		{"query-source", func(a *classificationAnchors) { a.Queries[2].Source += " " }},
		{"patterns", func(a *classificationAnchors) { a.Queries[0].Metadata.Patterns = 3 }},
		// The kit validator pins capture counts, not names (native pins names); a count change is the nominal rejection.
		{"captures", func(a *classificationAnchors) { a.Queries[1].Metadata.CaptureNames = []string{"chain_op", "chain"} }},
		{"predicates-nil", func(a *classificationAnchors) { a.Queries[0].Metadata.Predicates = nil }},
		{"required-structural-nil", func(a *classificationAnchors) { a.Required[0].Structural = nil }},
		{"required-expected-nil", func(a *classificationAnchors) { a.Required[0].Expected = nil }},
		{"expect", func(a *classificationAnchors) { a.Expect = nil }},
		{"api", func(a *classificationAnchors) { a.API = true }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			bad := classificationPowerShell3Clone(fixture.anchors)
			mutate.edit(&bad)
			if classificationEmptyAnchorsValid(fixture.profile, bad) == nil {
				t.Fatal("mutated PowerShell3 anchor accepted")
			}
		})
	}
}

func TestQueryClassificationPowerShell3SourceFixingDoesNotWeakenEmptyProfiles(t *testing.T) {
	powershell := classificationEmptyStaticFixtureForTest(t, "powershell3-tiny")
	for _, name := range []string{"dart7-empty", "php5-empty", "javascript4-empty", "python8-empty"} {
		t.Run(name, func(t *testing.T) {
			empty := classificationEmptyStaticFixtureForTest(t, name)
			if empty.profile.SourceUTF8 != "" || empty.profile.Nodes != nil {
				t.Fatal("existing empty profile gained a source or CST pin")
			}
			for _, source := range []string{";", " ", "\n", "x"} {
				bad := classificationPowerShell3Clone(empty.anchors)
				bad.Source.UTF8, bad.Source.Bytes, bad.Source.SHA256 = source, uint64(len(source)), digestHex([]byte(source))
				if classificationEmptyAnchorsValid(empty.profile, bad) == nil {
					t.Fatalf("source %q entered the empty-source profile", source)
				}
			}
			bad := classificationPowerShell3Clone(empty.anchors)
			bad.Nodes = classificationPowerShell3ExpectedNodes()
			if classificationEmptyAnchorsValid(empty.profile, bad) == nil {
				t.Fatal("PowerShell CST entered the empty-source profile")
			}
			if classificationEmptyAnchorsValid(powershell.profile, empty.anchors) == nil || classificationEmptyAnchorsValid(empty.profile, powershell.anchors) == nil {
				t.Fatal("cross-profile anchors accepted")
			}
		})
	}
}

func TestQueryClassificationPowerShell3ClosedNamesRoutesAndReferences(t *testing.T) {
	for _, name := range []string{"", "powershell", "powershell3", "powershell3-tiny ", "POWERSHELL3-tiny", "powershell3-tiny-alias", "powershell3-empty", "dart7-empty", "bash2-empty"} {
		if classificationPowerShell3ProfileName(name) {
			t.Fatalf("near-alias PowerShell3 profile %q accepted", name)
		}
	}
	for _, name := range []string{"", "powershell", "powershell3", "powershell3-tiny ", "POWERSHELL3-tiny", "powershell3-tiny-alias", "powershell3-empty"} {
		if classificationGoDefinitionProfileAllowed(name) || classificationGoLocalSkipsProfile(name) {
			t.Fatalf("near-alias definition profile %q accepted", name)
		}
	}
	for _, route := range []string{"", "PowerShell", "powershell ", "powershell3", "pwsh", "bash"} {
		if profile, ok := classificationPowerShell3ProfileForRoute(route); ok {
			t.Fatalf("route %q entered the closed PowerShell3 route: %s", route, profile.Name)
		}
	}
	fixture := classificationEmptyStaticFixtureForTest(t, "powershell3-tiny")
	ids := make(map[string]string, len(fixture.anchors.Queries))
	for i, query := range fixture.anchors.Queries {
		ids[query.ID] = fmt.Sprintf("powershell3-reference-%d", i)
	}
	if ordered, err := classificationEmptyOrderedReferenceIDs(fixture.anchors, ids); err != nil || len(ordered) != 3 {
		t.Fatal("unique ordered reference invocations rejected", err)
	}
	ids[fixture.anchors.Queries[1].ID] = ids[fixture.anchors.Queries[0].ID]
	if _, err := classificationEmptyOrderedReferenceIDs(fixture.anchors, ids); err == nil {
		t.Fatal("one reference invocation reused by two queries")
	}
}

func TestQueryClassificationPowerShell3StderrBinding(t *testing.T) {
	a := classificationAnchors{Route: "powershell"}
	if err := classificationCaptureStderr(a, semanticProcessCapture{}, eligibilityExecution{}, nil); err == nil {
		t.Fatal("PowerShell3 missing zero-byte stderr member accepted")
	}
	empty := ReplayMember{Role: "stderr", Bytes: 0, SHA256: digestHex(nil)}
	read := func(pin *ReplayMember) ([]byte, error) {
		if pin.Role != empty.Role || pin.Bytes != empty.Bytes || pin.SHA256 != empty.SHA256 {
			return nil, fmt.Errorf("stderr pin changed")
		}
		return []byte{}, nil
	}
	if err := classificationCaptureStderr(a, semanticProcessCapture{Stderr: &empty}, eligibilityExecution{StderrBytes: 0}, read); err != nil {
		t.Fatalf("PowerShell3 explicit empty stderr rejected: %v", err)
	}
	if err := classificationCaptureStderr(a, semanticProcessCapture{Stderr: &empty}, eligibilityExecution{StderrBytes: 1}, read); err == nil {
		t.Fatal("PowerShell3 stderr byte count mismatch accepted")
	}
}
