package native

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"os"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const classificationPowerShell3Profile = "powershell3-tiny"

func classificationPowerShell3ReadAnchors(t *testing.T, name string) (classificationEmptyProfile, []byte, classificationLocalAnchors) {
	t.Helper()
	profile, ok := classificationEmptyProfileFor(name)
	if !ok {
		t.Fatalf("closed empty-family profile %q missing", name)
	}
	data, err := os.ReadFile("../../testdata/query-semantics/" + profile.anchorFile)
	if err != nil || len(data) != profile.anchorBytes || sha(data) != profile.anchorSHA {
		t.Fatal("pinned anchor changed", err)
	}
	var anchors classificationLocalAnchors
	if err := jsonv2.Unmarshal(data, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	return profile, data, anchors
}

func classificationPowerShell3Clone(a classificationLocalAnchors) classificationLocalAnchors {
	a.Nodes = slices.Clone(a.Nodes)
	a.Queries = slices.Clone(a.Queries)
	a.Required = slices.Clone(a.Required)
	return a
}

func TestQueryClassificationPowerShell3StaticProfileAndPins(t *testing.T) {
	profile, _, anchors := classificationPowerShell3ReadAnchors(t, classificationPowerShell3Profile)
	if !classificationPowerShell3ProfileName(classificationPowerShell3Profile) || !classificationGoCaptureEntrySkipsProfile(classificationPowerShell3Profile) ||
		classificationWeb16ProfileName(classificationPowerShell3Profile) || classificationScript17ProfileName(classificationPowerShell3Profile) || classificationDartPHP12ProfileName(classificationPowerShell3Profile) {
		t.Fatal("closed PowerShell3 dispatch differs")
	}
	if _, ok := classificationSmall17ProfileFor(classificationPowerShell3Profile); ok || classificationCJRProfile(classificationPowerShell3Profile) {
		t.Fatal("PowerShell3 entered another closed family")
	}
	if _, ok := classificationCPPProfileSpecFor(classificationPowerShell3Profile); ok {
		t.Fatal("PowerShell3 entered the paged family")
	}
	if anchors.Source.UTF8 != ";" || anchors.Source.Bytes != 1 || len(anchors.Nodes) != 4 || profile.source != ";" || len(profile.nodes) != 4 {
		t.Fatal("one-byte source or static CST differs")
	}
	if err := classificationEmptyAnchorsValid(classificationPowerShell3Profile, anchors); err != nil {
		t.Fatal(err)
	}
	if err := classificationEmptyQueryRegistryValid(classificationPowerShell3Profile, anchors); err != nil {
		t.Fatal(err)
	}
	registryData, err := os.ReadFile("../../contracts/native-routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Routes []struct {
			Route, Symbol string
			Files         []kit.NativeInput
		}
	}
	if err := json.Unmarshal(registryData, &registry); err != nil {
		t.Fatal(err)
	}
	var grammar []kit.NativeInput
	matches := 0
	for _, row := range registry.Routes {
		if row.Route == profile.route {
			matches++
			grammar = row.Files
			if row.Symbol != profile.symbol {
				t.Fatal("symbol differs")
			}
		}
	}
	if matches != 1 {
		t.Fatal("route missing or duplicated")
	}
	if err := classificationEmptyGrammarValid(classificationPowerShell3Profile, registryData, profile.route, profile.symbol, grammar); err != nil {
		t.Fatal(err)
	}
	if classificationEmptyGrammarValid(classificationPowerShell3Profile, registryData, profile.route, profile.symbol, grammar[1:]) == nil {
		t.Fatal("incomplete grammar accepted")
	}
	badGrammar := slices.Clone(grammar)
	badGrammar[0], badGrammar[1] = badGrammar[1], badGrammar[0]
	if classificationEmptyGrammarValid(classificationPowerShell3Profile, registryData, profile.route, profile.symbol, badGrammar) == nil {
		t.Fatal("reordered grammar accepted")
	}
	capture, err := classificationCaptureProfileFor(classificationPowerShell3Profile)
	if err != nil || capture.anchorFile != profile.anchorFile || capture.anchorSHA != profile.anchorSHA || capture.route != "powershell" ||
		capture.symbol != "tree_sitter_powershell" || capture.queryCount != 3 || capture.grammarCount != 5 || capture.paged {
		t.Fatal("capture dispatch differs", err)
	}
	var patterns, entries int
	names := make(map[string]bool)
	for _, query := range profile.queries {
		patterns += int(query.patterns)
		entries += len(query.captures)
		for _, capture := range query.captures {
			names[capture] = true
		}
	}
	if patterns != 6 || entries != 6 || len(names) != 6 {
		t.Fatal("metadata denominator differs", patterns, entries, len(names))
	}
	queryData, err := os.ReadFile("../../testdata/native/queries/powershell.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := classificationEmptyQueryRegistryBytesValid(classificationPowerShell3Profile, anchors, append(slices.Clone(queryData), ' ')); err == nil {
		t.Fatal("query registry byte mutation accepted")
	}
	for _, name := range []string{"", "positive-batch", "powershell", "powershell3", "powershell3-tiny ", "POWERSHELL3-tiny", "powershell3-tiny-alias", "powershell3-empty", "dart7-empty", "bash2-empty"} {
		if classificationPowerShell3ProfileName(name) {
			t.Fatalf("non-PowerShell3 name accepted: %q", name)
		}
	}
	for _, name := range []string{"powershell", "powershell3", "powershell3-tiny ", "POWERSHELL3-tiny", "powershell3-tiny-alias", "powershell3-empty"} {
		if _, err := classificationCaptureProfileFor(name); err == nil {
			t.Fatalf("near-alias capture profile %q accepted", name)
		}
	}
}

func TestQueryClassificationPowerShell3AnchorGuards(t *testing.T) {
	field := "value"
	for _, mutate := range []struct {
		name string
		edit func(*classificationLocalAnchors)
	}{
		{"route", func(a *classificationLocalAnchors) { a.Route = "bash" }},
		{"case", func(a *classificationLocalAnchors) { a.Case = "powershell3-empty-definitions" }},
		{"workload", func(a *classificationLocalAnchors) { a.Workload = "powershell3-empty-definition-single-frame" }},
		{"api", func(a *classificationLocalAnchors) { a.API = true }},
		{"empty-source", func(a *classificationLocalAnchors) { a.Source.UTF8, a.Source.Bytes, a.Source.SHA256 = "", 0, sha(nil) }},
		{"trailing-newline", func(a *classificationLocalAnchors) {
			a.Source.UTF8, a.Source.Bytes, a.Source.SHA256 = ";\n", 2, sha([]byte(";\n"))
		}},
		{"leading-space", func(a *classificationLocalAnchors) {
			a.Source.UTF8, a.Source.Bytes, a.Source.SHA256 = " ;", 2, sha([]byte(" ;"))
		}},
		{"stale-source-pin", func(a *classificationLocalAnchors) { a.Source.UTF8 = ";;" }},
		{"stale-source-bytes", func(a *classificationLocalAnchors) { a.Source.Bytes = 2 }},
		{"stale-source-sha", func(a *classificationLocalAnchors) { a.Source.SHA256 = sha(nil) }},
		{"root-only", func(a *classificationLocalAnchors) { a.Nodes = a.Nodes[:1] }},
		{"missing-leaf", func(a *classificationLocalAnchors) { a.Nodes = a.Nodes[:3] }},
		{"missing-middle", func(a *classificationLocalAnchors) { a.Nodes = append(a.Nodes[:1:1], a.Nodes[2:]...) }},
		{"extra-node", func(a *classificationLocalAnchors) { a.Nodes = append(a.Nodes, a.Nodes[3]) }},
		{"zero-span", func(a *classificationLocalAnchors) { a.Nodes[3].EndByte, a.Nodes[3].EndPoint.Column = 0, 0 }},
		{"field-set", func(a *classificationLocalAnchors) { a.Nodes[3].Field = &field }},
		{"named-flip-statement", func(a *classificationLocalAnchors) { a.Nodes[2].Named = false }},
		{"named-flip-token", func(a *classificationLocalAnchors) { a.Nodes[3].Named = true }},
		{"extra-flag", func(a *classificationLocalAnchors) { a.Nodes[1].Extra = true }},
		{"error-flag", func(a *classificationLocalAnchors) { a.Nodes[2].HasError = true }},
		{"missing-flag", func(a *classificationLocalAnchors) { a.Nodes[3].IsMissing = true }},
		{"parent", func(a *classificationLocalAnchors) { a.Nodes[3].Parent = 1 }},
		{"type", func(a *classificationLocalAnchors) { a.Nodes[2].Type = "_statement" }},
		{"root-type", func(a *classificationLocalAnchors) { a.Nodes[0].Type = "script_file" }},
		{"abi-symbols", func(a *classificationLocalAnchors) { a.ABI.SymbolCount-- }},
		{"abi-fields", func(a *classificationLocalAnchors) { a.ABI.FieldCount++ }},
		{"query-order", func(a *classificationLocalAnchors) { a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0] }},
		{"query-source", func(a *classificationLocalAnchors) { a.Queries[2].Source += " " }},
		{"patterns", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Patterns = 3 }},
		{"captures", func(a *classificationLocalAnchors) { a.Queries[1].Metadata.CaptureNames = []string{"chain"} }},
		{"predicates-nil", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = nil }},
		{"query-count", func(a *classificationLocalAnchors) { a.Queries = a.Queries[:2] }},
		{"required", func(a *classificationLocalAnchors) { a.Required = a.Required[:2] }},
		{"required-structural-nil", func(a *classificationLocalAnchors) { a.Required[0].Structural = nil }},
		{"expect", func(a *classificationLocalAnchors) { a.Expect = nil }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			_, _, anchors := classificationPowerShell3ReadAnchors(t, classificationPowerShell3Profile)
			bad := classificationPowerShell3Clone(anchors)
			mutate.edit(&bad)
			if classificationEmptyAnchorsValid(classificationPowerShell3Profile, bad) == nil {
				t.Fatal("PowerShell3 anchor mutation accepted")
			}
		})
	}
}

func TestQueryClassificationPowerShell3SourceFixingDoesNotWeakenEmptyProfiles(t *testing.T) {
	_, _, powershell := classificationPowerShell3ReadAnchors(t, classificationPowerShell3Profile)
	for _, name := range []string{"dart7-empty", "php5-empty", "javascript4-empty", "python8-empty", "bash2-empty"} {
		t.Run(name, func(t *testing.T) {
			profile, _, anchors := classificationPowerShell3ReadAnchors(t, name)
			if profile.source != "" || profile.nodes != nil {
				t.Fatal("existing empty profile gained a source or CST pin")
			}
			for _, source := range []string{";", " ", "\n", "x"} {
				bad := classificationPowerShell3Clone(anchors)
				bad.Source.UTF8, bad.Source.Bytes, bad.Source.SHA256 = source, uint64(len(source)), sha([]byte(source))
				if classificationEmptyAnchorsValid(name, bad) == nil {
					t.Fatalf("source %q entered the empty-source profile", source)
				}
			}
			bad := classificationPowerShell3Clone(anchors)
			bad.Nodes = classificationPowerShell3ExpectedNodes()
			if classificationEmptyAnchorsValid(name, bad) == nil {
				t.Fatal("PowerShell CST entered the empty-source profile")
			}
			if classificationEmptyAnchorsValid(classificationPowerShell3Profile, anchors) == nil || classificationEmptyAnchorsValid(name, powershell) == nil {
				t.Fatal("cross-profile anchors accepted")
			}
		})
	}
}

func TestQueryClassificationPowerShell3EmptyStderrCaptureMember(t *testing.T) {
	processResult, ownerResult := &runner.Result{}, &runner.Result{}
	result := &CaseResult{Process: processResult}
	capture := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult}
	classificationNormalizeCaptureStderr(classificationPowerShell3Profile, result, capture)
	for _, process := range []*runner.Result{processResult, ownerResult} {
		if process.StderrBytes != 0 || process.Stderr == nil || len(process.Stderr) != 0 {
			t.Fatal("PowerShell3 zero-byte stderr was not made explicit in both process records")
		}
	}
	stderr := []byte{0x41}
	processResult = &runner.Result{StderrBytes: 1, Stderr: slices.Clone(stderr)}
	ownerResult = &runner.Result{StderrBytes: 1, Stderr: slices.Clone(stderr)}
	classificationNormalizeCaptureStderr(classificationPowerShell3Profile, &CaseResult{Process: processResult}, &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult})
	if !slices.Equal(processResult.Stderr, stderr) || !slices.Equal(ownerResult.Stderr, stderr) {
		t.Fatal("PowerShell3 nonempty stderr was rewritten")
	}
}

func TestQueryClassificationPowerShell3LocalCapture(t *testing.T) {
	if !classificationPowerShell3ProfileName(os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE")) {
		t.Skip("explicit closed PowerShell3 classification profile required")
	}
	classificationNativeLocalCapture(t)
}
