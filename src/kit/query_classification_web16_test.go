package kit

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

const classificationWeb16NativeRoutesSHA = "7be36efe4cafcefa460c8841c8698a0d055cb25d6f86a5bed8561d601c093ad2"

type classificationEmptyGrammarFile struct {
	Path string
	Role string
}

type classificationEmptyDefinitionProfile struct {
	Name, AnchorFile, AnchorSHA string
	Route, Case, Workload       string
	QueryFile                   string
	QueryFilePin                classificationGoDefinitionQuerySourcePin
	Set, Symbol, RootType       string
	Language                    string
	QueryCount                  int
	Patterns                    int
	CaptureNameEntries          int
	UniqueCaptureNames          int
	ABI                         eligibilityProducerABI
	Counts                      classificationGoDefinitionSlotCounts
	QueryIDs                    []string
	GrammarShape                []classificationEmptyGrammarFile
	CompilerIncludeDirs         []string
	CompilerSourceFiles         []string

	// SourceUTF8 and Nodes pin a non-empty single-frame source and its full CST; zero values keep the empty-source root-only profile.
	SourceUTF8 string
	Nodes      []TreeNode
}

func classificationWeb16ProfileName(name string) bool {
	_, err := classificationWeb16ProfileForName(name)
	return err == nil
}

func classificationWeb16ProfileForName(name string) (classificationEmptyDefinitionProfile, error) {
	routes := map[string]string{
		"javascript4-empty": "javascript",
		"jsx3-empty":        "jsx",
		"typescript7-empty": "typescript",
		"tsx2-empty":        "tsx",
	}
	route := routes[name]
	if route == "" {
		return classificationEmptyDefinitionProfile{}, fmt.Errorf("unknown closed Web16 profile %q", name)
	}
	profile, ok := classificationWeb16ProfileForRoute(route)
	if !ok || profile.Name != name {
		return classificationEmptyDefinitionProfile{}, fmt.Errorf("closed Web16 profile binding is missing: %q", name)
	}
	return profile, nil
}

func classificationWeb16ProfileForRoute(route string) (classificationEmptyDefinitionProfile, bool) {
	abi := func(symbols, fields uint32) eligibilityProducerABI {
		return eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: symbols, FieldCount: fields}
	}
	commonHeaders := func(prefix string) []classificationEmptyGrammarFile {
		return []classificationEmptyGrammarFile{
			{Path: prefix + "src/tree_sitter/alloc.h", Role: "header"},
			{Path: prefix + "src/tree_sitter/array.h", Role: "header"},
			{Path: prefix + "src/tree_sitter/parser.h", Role: "header"},
		}
	}
	base := func(name, anchorFile, anchorSHA, caseName, workload, queryFile string, queryBytes uint64, querySHA, set, symbol, rootType, language string,
		queryIDs []string, grammar []classificationEmptyGrammarFile, includeDirs, sources []string, patternCount, captureEntries, uniqueCaptures int,
		symbolCount, fieldCount uint32, counts classificationGoDefinitionSlotCounts) classificationEmptyDefinitionProfile {
		return classificationEmptyDefinitionProfile{
			Name: name, AnchorFile: anchorFile, AnchorSHA: anchorSHA, Route: route, Case: caseName, Workload: workload,
			QueryFile: queryFile, QueryFilePin: classificationGoDefinitionQuerySourcePin{Path: queryFile, Bytes: queryBytes, SHA256: querySHA},
			Set: set, Symbol: symbol, RootType: rootType, Language: language, QueryCount: len(queryIDs), Patterns: patternCount,
			CaptureNameEntries: captureEntries, UniqueCaptureNames: uniqueCaptures, ABI: abi(symbolCount, fieldCount), Counts: counts,
			QueryIDs: slices.Clone(queryIDs), GrammarShape: slices.Clone(grammar), CompilerIncludeDirs: slices.Clone(includeDirs), CompilerSourceFiles: slices.Clone(sources),
		}
	}
	switch route {
	case "javascript":
		shape := append([]classificationEmptyGrammarFile{{Path: "src/parser.c", Role: "parser"}, {Path: "src/scanner.c", Role: "scanner"}}, commonHeaders("")...)
		return base("javascript4-empty", "javascript4-classification-anchors.json", "b49741060ebc83441982a46b5b4187ed00fdb836d472d8e89ae9a2d44c301c33",
			"javascript4-empty-definitions", "javascript4-empty-definition-single-frame", "src/testdata/native/queries/javascript.json", 6270,
			"f33a5feb0eae85a5f467d36b9cb42084799dc4a5b2d67b8dcfd9d267b8f5e505", "s06-javascript", "tree_sitter_javascript", "program", "JavaScript",
			[]string{"q.structure", "q.probe-v16", "q.probe-v20", "q.probe-v22"}, shape, []string{"src"}, []string{"src/parser.c", "src/scanner.c"},
			17, 20, 20, 291, 36, classificationGoDefinitionSlotCounts{Cases: 58, Edits: 40, CaseSteps: 98, TreeRoles: 138, PotentialQueryTreeRoles: 552}), true
	case "jsx":
		shape := append([]classificationEmptyGrammarFile{{Path: "src/parser.c", Role: "parser"}, {Path: "src/scanner.c", Role: "scanner"}}, commonHeaders("")...)
		return base("jsx3-empty", "jsx3-classification-anchors.json", "dfee37f1794e2128a46d01c3b5d8b4fd78809a3a4d8d9cfa92f45b19f1a8a979",
			"jsx3-empty-definitions", "jsx3-empty-definition-single-frame", "src/testdata/native/queries/jsx.json", 4366,
			"036457f6bba87231d75fb55d768adef9ab8f03c8e2d43231da093aa18f8ffb62", "s06-jsx", "tree_sitter_javascript", "program", "JSX",
			[]string{"q.structure", "q.probe-b02", "q.probe-b03"}, shape, []string{"src"}, []string{"src/parser.c", "src/scanner.c"},
			7, 7, 7, 291, 36, classificationGoDefinitionSlotCounts{Cases: 58, Edits: 28, CaseSteps: 86, TreeRoles: 114, PotentialQueryTreeRoles: 342}), true
	case "typescript":
		shape := []classificationEmptyGrammarFile{{Path: "common/scanner.h", Role: "header"}, {Path: "typescript/src/parser.c", Role: "parser"}, {Path: "typescript/src/scanner.c", Role: "scanner"}}
		shape = append(shape, commonHeaders("typescript/")...)
		return base("typescript7-empty", "typescript7-classification-anchors.json", "1f658bca74082dd8cfd387eea2c73777e00b2128d1f58a9e690c1b40d83e31e1",
			"typescript7-empty-definitions", "typescript7-empty-definition-single-frame", "src/testdata/native/queries/typescript.json", 7706,
			"240790a7f8dbfee7c1fc0f4fc1e73098d75a519f86ec7a3a213cf7f2d9672067", "s06-typescript", "tree_sitter_typescript", "program", "TypeScript",
			[]string{"q.structure", "q.probe-V28", "q.probe-V3", "q.probe-V40", "q.probe-V45", "q.probe-V50", "q.probe-V59"}, shape,
			[]string{"typescript/src", "common"}, []string{"typescript/src/parser.c", "typescript/src/scanner.c"},
			20, 20, 20, 388, 40, classificationGoDefinitionSlotCounts{Cases: 81, Edits: 26, CaseSteps: 107, TreeRoles: 133, PotentialQueryTreeRoles: 931}), true
	case "tsx":
		shape := []classificationEmptyGrammarFile{{Path: "common/scanner.h", Role: "header"}, {Path: "tsx/src/parser.c", Role: "parser"}, {Path: "tsx/src/scanner.c", Role: "scanner"}}
		shape = append(shape, commonHeaders("tsx/")...)
		return base("tsx2-empty", "tsx2-classification-anchors.json", "da0fc57ba374c6a937ab793e023f7aed8459f73ebeb0ea680f37336148060eab",
			"tsx2-empty-definitions", "tsx2-empty-definition-single-frame", "src/testdata/native/queries/tsx.json", 2814,
			"9ff203465bdcda7bcee4ab553e92aff327da72ed23d03ed559dd9b5d5fd72374", "s06-tsx", "tree_sitter_tsx", "program", "TSX",
			[]string{"q.structure", "q.probe-B02"}, shape, []string{"tsx/src", "common"}, []string{"tsx/src/parser.c", "tsx/src/scanner.c"},
			7, 7, 7, 408, 43, classificationGoDefinitionSlotCounts{Cases: 71, Edits: 18, CaseSteps: 89, TreeRoles: 107, PotentialQueryTreeRoles: 214}), true
	default:
		return classificationEmptyDefinitionProfile{}, false
	}
}

func classificationEmptyAnchorsValid(profile classificationEmptyDefinitionProfile, a classificationAnchors) error {
	wantNodes := profile.Nodes
	if wantNodes == nil {
		wantNodes = []TreeNode{{Parent: -1, Type: profile.RootType, Named: true}}
	}
	// A nonempty source must name its full CST; the root-only default is valid for the empty source alone.
	if profile.SourceUTF8 != "" && profile.Nodes == nil {
		return fmt.Errorf("%s nonempty source lacks an explicit CST", profile.Name)
	}
	if a.sha256 != profile.AnchorSHA || a.Schema != "tsgk-query-classification-anchors/r1" || a.Route != profile.Route || a.Case != profile.Case || a.Workload != profile.Workload ||
		a.Operation != "native-query" || a.DriverMode != "single" || a.API || a.Source.UTF8 != profile.SourceUTF8 || a.Source.Bytes != uint64(len(profile.SourceUTF8)) ||
		a.Source.SHA256 != digestHex([]byte(profile.SourceUTF8)) || a.Edits == nil || len(a.Edits) != 0 || len(a.Queries) != profile.QueryCount || len(a.Required) != profile.QueryCount ||
		!reflect.DeepEqual(a.Nodes, wantNodes) || ValidateTree(a.Nodes, a.Source.Bytes) != nil ||
		!reflect.DeepEqual(a.Expect, []StepExpectation{{Step: 0, Syntax: "NO_ERROR"}}) || a.ABI != profile.ABI {
		return fmt.Errorf("%s fixed single-frame source or CST anchors differ", profile.Name)
	}
	seenIDs := make(map[string]bool, profile.QueryCount)
	seenNames := make(map[string]bool, profile.UniqueCaptureNames)
	patterns, captureEntries := 0, 0
	for i, query := range a.Queries {
		if query.ID != profile.QueryIDs[i] || seenIDs[query.ID] || query.SHA256 != digestHex([]byte(query.Source)) || !classificationGoDefinitionMetadataValid(query.Metadata) {
			return fmt.Errorf("%s anchor query %d has changed order, source pin, or complete metadata", profile.Name, i)
		}
		seenIDs[query.ID] = true
		patterns += int(query.Metadata.Patterns)
		captureEntries += len(query.Metadata.CaptureNames)
		for _, name := range query.Metadata.CaptureNames {
			seenNames[name] = true
		}
		tuple := a.Required[i]
		if tuple.Query != query.ID || tuple.Step != 0 || tuple.TreeRole != "incremental" || tuple.Structural == nil || len(tuple.Structural) != 0 || tuple.Expected == nil || len(tuple.Expected) != 0 {
			return fmt.Errorf("%s empty required query tuple %d differs", profile.Name, i)
		}
	}
	if patterns != profile.Patterns || captureEntries != profile.CaptureNameEntries || len(seenNames) != profile.UniqueCaptureNames {
		return fmt.Errorf("%s query metadata denominator differs", profile.Name)
	}
	return nil
}

func classificationEmptyNativeRoutePins(data []byte, profile classificationEmptyDefinitionProfile) ([]NativeInput, error) {
	if len(data) != 118237 || digestHex(data) != classificationWeb16NativeRoutesSHA {
		return nil, fmt.Errorf("empty-profile pinned native-routes registry differs")
	}
	var registry struct {
		Schema string `json:"schema"`
		Routes []struct {
			Route  string        `json:"route"`
			Symbol string        `json:"symbol"`
			Files  []NativeInput `json:"files"`
		} `json:"routes"`
	}
	if err := jsonv2.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("empty-profile native-routes shape: %w", err)
	}
	if registry.Schema != "tsgk-native-routes/r1" {
		return nil, fmt.Errorf("empty-profile native-routes schema differs")
	}
	var files []NativeInput
	var symbol string
	var routeCount int
	for _, row := range registry.Routes {
		if row.Route == profile.Route {
			routeCount++
			symbol = row.Symbol
			files = row.Files
		}
	}
	if routeCount != 1 || symbol != profile.Symbol || len(files) != len(profile.GrammarShape) {
		return nil, fmt.Errorf("%s native route symbol or exact input count differs", profile.Name)
	}
	for i, file := range files {
		want := profile.GrammarShape[i]
		if file.Path != want.Path || file.Role != want.Role || file.Bytes == 0 || !hex64.MatchString(file.SHA256) {
			return nil, fmt.Errorf("%s native route input %d changed", profile.Name, i)
		}
	}
	return slices.Clone(files), nil
}

func classificationEmptyCompilerArgs(profile classificationEmptyDefinitionProfile, grammarRoot, output string, grammar []NativeInput) ([]string, error) {
	if len(grammar) != len(profile.GrammarShape) || !filepath.IsAbs(grammarRoot) || !filepath.IsAbs(output) {
		return nil, fmt.Errorf("%s compiler grammar or path closure differs", profile.Name)
	}
	for i, pin := range grammar {
		shape := profile.GrammarShape[i]
		if pin.Path != shape.Path || pin.Role != shape.Role || pin.Bytes == 0 || !hex64.MatchString(pin.SHA256) {
			return nil, fmt.Errorf("%s compiler grammar input %d differs", profile.Name, i)
		}
	}
	args := []string{"-shared", "-O0", "-std=gnu11"}
	for _, dir := range profile.CompilerIncludeDirs {
		args = append(args, "-I", filepath.Join(grammarRoot, filepath.FromSlash(dir)))
	}
	for _, source := range profile.CompilerSourceFiles {
		args = append(args, filepath.Join(grammarRoot, filepath.FromSlash(source)))
	}
	return append(args, "-o", output), nil
}

func classificationEmptyLibraryRequestValid(profile classificationEmptyDefinitionProfile, plan classificationPlan, root, grammarRoot string) error {
	request := plan.LibraryRequest
	output := filepath.Join(root, "library", profile.Route+".dll")
	if plan.Schema != "tsgk-query-classification-plan/r1" || plan.AnchorsSHA256 != profile.AnchorSHA || plan.APIAnchorsSHA256 != "" || plan.NativeContext != "full-input-utf8/r2" ||
		!eligibilityRevision(plan.SourceRevision) || !validID(plan.RunID) || !hex64.MatchString(plan.RuntimeManifestSHA256) || !hex64.MatchString(plan.CompilerSHA256) ||
		len(plan.Grammar) != len(profile.GrammarShape) || !filepath.IsAbs(root) || !filepath.IsAbs(grammarRoot) || request.Dir != root ||
		!eligibilityAbsolutePath(request.Path, "windows/amd64") || eligibilityValidateRequest(request, "go-library") != nil ||
		request.StdoutBytes != 16<<20 || request.StderrBytes != 4<<20 || request.Wall != 120e9 || request.Grace != 2e9 ||
		request.Memory.Bytes != 4<<30 || !request.Memory.Hard || len(request.Env) != 9 || request.CgroupParent != "" {
		return fmt.Errorf("%s compiler request identity, ownership, or limits differ", profile.Name)
	}
	wantEnvNames := []string{"COMSPEC", "PATH", "PATHEXT", "SystemDrive", "SystemRoot", "TEMP", "TMP", "TMPDIR", "WINDIR"}
	for i, env := range request.Env {
		name, value, ok := strings.Cut(env, "=")
		if !ok || name != wantEnvNames[i] || value == "" {
			return fmt.Errorf("%s compiler environment differs from the explicit Windows allowlist", profile.Name)
		}
	}
	wantArgs, err := classificationEmptyCompilerArgs(profile, grammarRoot, output, plan.Grammar)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(request.Args, wantArgs) {
		return fmt.Errorf("%s exact compiler parser/scanner argv differs", profile.Name)
	}
	return nil
}

func classificationEmptyDefinitionLoadInputs(a classificationAnchors, plan classificationPlan, inventoryData, qualificationData, queryData []byte) (classificationGoDefinitionInputs, []string, error) {
	var out classificationGoDefinitionInputs
	profile, ok := classificationEmptyProfileForRoute(a.Route)
	if !ok {
		return out, nil, fmt.Errorf("unsupported closed empty-profile route %q", a.Route)
	}
	if err := classificationEmptyAnchorsValid(profile, a); err != nil {
		return out, nil, err
	}
	if plan.Schema != "tsgk-query-classification-plan/r1" || plan.AnchorsSHA256 != profile.AnchorSHA || plan.APIAnchorsSHA256 != "" || plan.NativeContext != "full-input-utf8/r2" ||
		!eligibilityRevision(plan.SourceRevision) || !validID(plan.RunID) || !hex64.MatchString(plan.RuntimeManifestSHA256) || !hex64.MatchString(plan.CompilerSHA256) ||
		len(plan.Grammar) != len(profile.GrammarShape) {
		return out, nil, fmt.Errorf("%s preregistration differs from its fixed full-input single-frame profile", profile.Name)
	}
	if digestHex(inventoryData) != classificationGoDefinitionInventorySHA || digestHex(qualificationData) != classificationGoDefinitionQualificationSHA ||
		uint64(len(queryData)) != profile.QueryFilePin.Bytes || digestHex(queryData) != profile.QueryFilePin.SHA256 {
		return out, nil, fmt.Errorf("%s query-slot, qualification, or raw query-source pin differs", profile.Name)
	}
	var slots classificationGoDefinitionSlotInventory
	if err := jsonv2.Unmarshal(inventoryData, &slots, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, nil, fmt.Errorf("query-slot inventory shape: %w", err)
	}
	if slots.Schema != "tsgk-query-slot-inventory/r1" || slots.ExpansionRule != "case-query-edit-tree-role/r1" ||
		!slices.Equal(slots.Coordinate, []string{"qualification_inventory.sha256", "set", "case", "query", "step", "tree_role"}) || len(slots.Ordinary) != 26 ||
		len(slots.SemanticRequired) == 0 || !bytes.Equal(bytes.TrimSpace(slots.SemanticRequired), []byte("null")) ||
		slots.Qualification.Path != "src/contracts/qualification-c1.json" || slots.Qualification.Bytes != uint64(len(qualificationData)) || slots.Qualification.SHA256 != classificationGoDefinitionQualificationSHA {
		return out, nil, fmt.Errorf("query-slot inventory r1 contract differs")
	}
	var queryFile classificationGoDefinitionQueryFile
	if err := jsonv2.Unmarshal(queryData, &queryFile, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, nil, fmt.Errorf("%s query-file shape: %w", profile.Name, err)
	}
	if queryFile.Schema != "tsgk-native-query-cases/r1" || queryFile.Route != profile.Route || len(queryFile.Queries) != profile.QueryCount {
		return out, nil, fmt.Errorf("%s raw query-file identity or ordered count differs", profile.Name)
	}
	qualification, err := ParseQualificationInventory(qualificationData)
	if err != nil {
		return out, nil, fmt.Errorf("qualification inventory: %w", err)
	}
	if qualification.SHA256 != classificationGoDefinitionQualificationSHA {
		return out, nil, fmt.Errorf("parsed qualification inventory pin differs")
	}
	var route *QualRoute
	var routeCount int
	for i := range qualification.Routes {
		if qualification.Routes[i].Route == profile.Route {
			routeCount++
			route = &qualification.Routes[i]
		}
	}
	if routeCount != 1 || route == nil {
		return out, nil, fmt.Errorf("%s qualification route is missing or duplicated", profile.Name)
	}
	workload := route.Workload
	// The ordinary qualification workload keeps API=true; the empty observation anchor uses API=false.
	if workload.Route != profile.Route || workload.Set != profile.Set || workload.Profile != profile.Set || workload.Operation != "native-query-batch" || workload.Output != "tree" || workload.Symbol != profile.Symbol ||
		!workload.API || workload.FactPack != nil || len(workload.Grammar) != len(plan.Grammar) || !reflect.DeepEqual(workload.Grammar, plan.Grammar) || len(workload.Queries) != profile.QueryCount {
		return out, nil, fmt.Errorf("%s qualification workload differs from the fixed profile", profile.Name)
	}
	for _, extra := range qualification.ExtraRoles {
		for _, extraWorkload := range extra.Workloads {
			if extraWorkload.Route == profile.Route && extraWorkload.Set == profile.Set {
				return out, nil, fmt.Errorf("%s workload is duplicated in an extra role", profile.Name)
			}
		}
	}
	var groupCount int
	for _, group := range slots.Ordinary {
		if (group.Route == profile.Route || group.Set == profile.Set) && (group.Route != profile.Route || group.Set != profile.Set || group.Profile != profile.Set) {
			return out, nil, fmt.Errorf("%s query-slot group appears under another route or profile", profile.Name)
		}
		if group.Route == profile.Route && group.Set == profile.Set {
			groupCount++
			out.slotGroup = group
		}
	}
	if groupCount != 1 || out.slotGroup.Profile != profile.Set || out.slotGroup.Operation != "native-query-batch" ||
		out.slotGroup.Counts != profile.Counts || len(out.slotGroup.Queries) != profile.QueryCount {
		return out, nil, fmt.Errorf("%s ordinary query-slot group or fixed coordinates differ", profile.Name)
	}
	var sourceCount int
	for _, source := range slots.QuerySources {
		if source.Path == profile.QueryFilePin.Path {
			sourceCount++
			if source != profile.QueryFilePin {
				return out, nil, fmt.Errorf("%s query-source inventory pin differs", profile.Name)
			}
		}
	}
	if sourceCount != 1 {
		return out, nil, fmt.Errorf("%s query-source inventory pin is missing or duplicated", profile.Name)
	}
	sourceFiles := make([]string, profile.QueryCount)
	for i := range sourceFiles {
		sourceFiles[i] = profile.QueryFile
	}
	queryIDs, err := classificationGoDefinitionQueryBindings(a, queryFile, workload, out.slotGroup, sourceFiles)
	if err != nil {
		return out, nil, err
	}
	coordinates, err := classificationGoDefinitionCoordinateProjection(qualification.SHA256, profile.Set, workload, queryIDs)
	if err != nil {
		return out, nil, err
	}
	coordinates.QuerySlotInventorySHA256 = classificationGoDefinitionInventorySHA
	coordinates.QuerySourceFiles = []classificationGoDefinitionQuerySourcePin{profile.QueryFilePin}
	if coordinates.Cases != profile.Counts.Cases || coordinates.Edits != profile.Counts.Edits || coordinates.CaseSteps != profile.Counts.CaseSteps ||
		coordinates.TreeRoles != profile.Counts.TreeRoles || coordinates.Definitions != uint64(profile.QueryCount) || coordinates.PotentialQueryTreeRoles != profile.Counts.PotentialQueryTreeRoles {
		return out, nil, fmt.Errorf("%s query-coordinate denominator differs", profile.Name)
	}
	out.qualification, out.workload, out.queryFile, out.coordinates = qualification, workload, queryFile, coordinates
	return out, sourceFiles, nil
}

func classificationEmptyDefinitionRows(a classificationAnchors, inputs classificationGoDefinitionInputs, sourceFiles []string, witnessQueries []QueryWitnessQuery, nativeQueries []querywire.Query, references []*instrumentedReference) ([]classificationGoDefinition, error) {
	profile, ok := classificationEmptyProfileForRoute(a.Route)
	if !ok {
		return nil, fmt.Errorf("unsupported closed empty-profile definition route %q", a.Route)
	}
	if err := classificationEmptyAnchorsValid(profile, a); err != nil {
		return nil, err
	}
	if len(sourceFiles) != profile.QueryCount {
		return nil, fmt.Errorf("%s source-file binding count differs", profile.Name)
	}
	return classificationDefinitionRows(a, inputs, witnessQueries, nativeQueries, references, profile.Language, sourceFiles)
}

func classificationEmptyOrderedReferenceIDs(a classificationAnchors, referenceIDs map[string]string) ([]string, error) {
	profile, ok := classificationEmptyProfileForRoute(a.Route)
	if !ok || len(a.Queries) != profile.QueryCount || len(referenceIDs) != profile.QueryCount {
		return nil, fmt.Errorf("empty-profile reference invocation denominator differs")
	}
	ordered := make([]string, profile.QueryCount)
	seenIDs := make(map[string]bool, profile.QueryCount)
	for i, query := range a.Queries {
		if query.ID != profile.QueryIDs[i] || seenIDs[query.ID] {
			return nil, fmt.Errorf("empty-profile reference query order or identity differs: %s", query.ID)
		}
		seenIDs[query.ID] = true
		id := referenceIDs[query.ID]
		if !validID(id) {
			return nil, fmt.Errorf("empty-profile guarded reference invocation is missing: %s", query.ID)
		}
		ordered[i] = id
	}
	if len(seenIDs) != profile.QueryCount {
		return nil, fmt.Errorf("empty-profile reference query set is incomplete")
	}
	seenInvocations := make(map[string]bool, profile.QueryCount)
	for _, id := range ordered {
		if seenInvocations[id] {
			return nil, fmt.Errorf("empty-profile reference invocation is reused: %s", id)
		}
		seenInvocations[id] = true
	}
	return ordered, nil
}

func classificationEmptyDefinitionProjection(anchors classificationAnchors, plan classificationPlan, s *eligibilitySession, cache *semanticNativeCache, core QueryReferenceEligibilityResult, inventoryData, qualificationData, queryData []byte, readerRevision string) (classificationGoDefinitionProjectionResult, error) {
	var result classificationGoDefinitionProjectionResult
	profile, ok := classificationEmptyProfileForRoute(anchors.Route)
	if !ok {
		return result, fmt.Errorf("unsupported closed empty-profile route %q", anchors.Route)
	}
	inputs, sourceFiles, err := classificationEmptyDefinitionLoadInputs(anchors, plan, inventoryData, qualificationData, queryData)
	if err != nil {
		return result, err
	}
	if !hex40.MatchString(readerRevision) {
		return result, fmt.Errorf("separately pinned reader revision is invalid")
	}
	if s == nil || cache == nil || s.res == nil || s.r == nil || s.semantic != cache || !s.bound || !cache.ready || !cache.record.haveRaw {
		return result, fmt.Errorf("empty-profile definition projection requires the live guarded eligibility session")
	}
	if err := s.r.check(); err != nil {
		return result, err
	}
	wantTuple := QueryEligibilityCounts{Required: uint64(profile.QueryCount), Verified: uint64(profile.QueryCount)}
	if core.ResultSchema != QueryReferenceEligibilityResultSchema || core.ExecutionStatus != StatusCompleted || core.Assessment != AssessPass || !core.Complete || core.Tuple != wantTuple ||
		s.res.ResultSchema != core.ResultSchema || s.res.ExecutionStatus != core.ExecutionStatus || s.res.Assessment != core.Assessment || s.res.Complete != core.Complete || s.res.Tuple != wantTuple ||
		s.res.ApprovalSHA256 != core.ApprovalSHA256 || s.res.RegistrationSHA256 != core.RegistrationSHA256 || s.res.WitnessSHA256 != core.WitnessSHA256 ||
		s.res.CatalogueSHA256 != core.CatalogueSHA256 || s.res.GraphSHA256 != core.GraphSHA256 {
		return result, fmt.Errorf("empty-profile projection requires the same completed guarded tuple")
	}
	if !classificationLibraryMatches(s) {
		return result, fmt.Errorf("empty-profile reference library differs from guarded compiler output")
	}
	nativeID := "native-" + anchors.Case
	if plan.NativeContext != anchors.nativeContext() || plan.NativeContext != "full-input-utf8/r2" || s.reg.Context != plan.NativeContext || s.reg.Native == nil ||
		s.reg.Native.Symbol != profile.Symbol || s.reg.Native.InvocationID != nativeID ||
		s.witness.Schema != queryEligibilityWitnessSchema || s.witness.Policy != "tsgk-predicates/r1" || s.witness.Case != anchors.Case ||
		s.witness.Route != profile.Route || s.witness.Role != "owned" || s.witness.Encoding != EncodingUTF8 || s.witness.Platform != "windows/amd64" ||
		s.witness.Input.Path != "source.bin" || s.witness.Input.Bytes != anchors.Source.Bytes || s.witness.Input.SHA256 != anchors.Source.SHA256 || s.witness.Input.Role != "case" ||
		s.witness.Native.RequestID != anchors.Case || s.witness.Native.API || s.witness.Native.ProducerABI != anchors.ABI ||
		s.witness.Native.RuntimeManifestSHA256 != plan.RuntimeManifestSHA256 || s.witness.Native.ObservationRunID != plan.RunID || len(s.witness.Native.Points) != 0 ||
		plan.SourceRevision != s.approval.Candidate.SourceRevision || plan.RunID != s.approval.Candidate.Run || s.approval.Candidate.Platform != "windows/amd64" || s.approval.Candidate.Attempt != 1 ||
		!classificationGoWitnessMatches(anchors, s.witness.QueryWitnessRegistration) || len(s.witness.Queries) != profile.QueryCount || len(s.reg.Native.Grammar) != len(plan.Grammar) {
		return result, fmt.Errorf("empty-profile candidate, source frame, witness, or grammar context differs from the fixed profile")
	}
	if err := classificationGrammarContext(s, anchors, plan); err != nil {
		return result, err
	}
	grammarRoot := filepath.Join(os.Getenv("TSGK_BENCH_PREPARED"), "routes", anchors.Route)
	if err := classificationEmptyLibraryRequestValid(profile, plan, plan.LibraryRequest.Dir, grammarRoot); err != nil {
		return result, fmt.Errorf("empty-profile guarded compiler request differs: %w", err)
	}
	if len(cache.checked) != 1 || cache.checked[0].Step.Step != 0 || cache.checked[0].Step.SourceBytes != anchors.Source.Bytes ||
		cache.checked[0].Step.SourceSHA256 != anchors.Source.SHA256 || cache.checked[0].Step.Edit != nil || cache.checked[0].Fresh != nil ||
		CompareTrees(cache.checked[0].Incremental.Nodes, anchors.Nodes) != nil || len(cache.checked[0].Incremental.Queries) != profile.QueryCount {
		return result, fmt.Errorf("empty-profile guarded source frame CST or query count differs")
	}
	references := make(map[string]*instrumentedReference, profile.QueryCount)
	referenceIDs := make(map[string]string, profile.QueryCount)
	var referenceExecutableSHA string
	for _, tuple := range anchors.Required {
		observation, found := s.observation(tuple)
		if !found || observation.Query != tuple.Query || observation.QueryID != tuple.Query || observation.Step != 0 || observation.TreeRole != "incremental" ||
			observation.SourceBytes != anchors.Source.Bytes || observation.SourceSHA256 != anchors.Source.SHA256 {
			return result, fmt.Errorf("empty-profile guarded reference observation differs: %s", tuple.Query)
		}
		out := s.outputs[observation.InvocationID]
		if out == nil || observation.ResponseOrdinal >= uint64(len(out.responses)) || observation.ResponseOrdinal >= uint64(len(out.rows)) || out.responses[observation.ResponseOrdinal] == nil {
			return result, fmt.Errorf("empty-profile guarded reference response is missing: %s", tuple.Query)
		}
		response, row := out.responses[observation.ResponseOrdinal], out.rows[observation.ResponseOrdinal]
		if response.Schema != instrumentedReferenceSchema || response.Status != StatusCompleted || response.Code != "" || response.Input.Encoding != EncodingUTF8 ||
			response.Input.Bytes != anchors.Source.Bytes || response.Input.SHA256 != anchors.Source.SHA256 || response.Grammar.Symbol != profile.Symbol ||
			CompareTrees(response.Tree.Nodes, anchors.Nodes) != nil || uint64(len(row)) != observation.ByteLength || digestHex(row) != observation.SHA256 {
			return result, fmt.Errorf("empty-profile guarded reference source frame or complete CST differs: %s", tuple.Query)
		}
		invocation := s.invocations[observation.InvocationID]
		if invocation == nil || invocation.Executable == nil {
			return result, fmt.Errorf("empty-profile reference executable is missing: %s", tuple.Query)
		}
		if referenceExecutableSHA == "" {
			referenceExecutableSHA = invocation.Executable.Member.SHA256
		} else if referenceExecutableSHA != invocation.Executable.Member.SHA256 {
			return result, fmt.Errorf("empty-profile reference executable bindings differ")
		}
		if !hex64.MatchString(referenceExecutableSHA) {
			return result, fmt.Errorf("empty-profile reference executable pin is invalid")
		}
		references[tuple.Query], referenceIDs[tuple.Query] = response, observation.InvocationID
	}
	orderedReferenceIDs, err := classificationEmptyOrderedReferenceIDs(anchors, referenceIDs)
	if err != nil {
		return result, err
	}
	orderedReferences := make([]*instrumentedReference, profile.QueryCount)
	for i, query := range anchors.Queries {
		orderedReferences[i] = references[query.ID]
		if orderedReferences[i] == nil {
			return result, fmt.Errorf("empty-profile guarded reference query is missing: %s", query.ID)
		}
	}
	definitions, err := classificationEmptyDefinitionRows(anchors, inputs, sourceFiles, s.witness.Queries, cache.checked[0].Incremental.Queries, orderedReferences)
	if err != nil {
		return result, err
	}
	nativeInvocation, err := classificationNativeInvocation(s, anchors)
	if err != nil {
		return result, err
	}
	libraryPin, libraryOK := s.owned["current/go.dll"]
	if nativeInvocation.Executable == nil || nativeInvocation.Executable.Member.SHA256 != s.witness.Native.ExecutableSHA256 || !hex64.MatchString(nativeInvocation.Executable.Member.SHA256) {
		return result, fmt.Errorf("empty-profile guarded native driver executable is missing or mismatched")
	}
	if !libraryOK || libraryPin.NodeID != "go-library-build" || libraryPin.Member.Role != "runtime-library" || !hex64.MatchString(libraryPin.Member.SHA256) ||
		s.reg.Build == nil || s.reg.Native.ReferenceLibrary == nil || !hex64.MatchString(s.reg.Native.ReferenceLibrary.Member.SHA256) {
		return result, fmt.Errorf("empty-profile guarded build or route library binding is missing")
	}
	libraryNode, found := s.nodes["go-library-build"]
	if !found || !classificationGoLibraryNodeMatches(libraryNode, plan, anchors.Workload) || libraryNode.Identities["tool"] != plan.CompilerSHA256 {
		return result, fmt.Errorf("empty-profile guarded compiler node differs from preregistration")
	}
	for _, pin := range []string{s.reg.Build.Identity.Member.SHA256, s.reg.Build.SourceSnapshot.Member.SHA256, s.reg.Build.VendorSnapshot.Member.SHA256} {
		if !hex64.MatchString(pin) {
			return result, fmt.Errorf("empty-profile guarded build identity pin is invalid")
		}
	}
	result = classificationGoDefinitionProjectionResult{
		Scope: fmt.Sprintf("conditional structural classification of %d predicate-free %s definitions and %d potential %s links; one guarded native-query frame is the guard, not per-case or semantic completion", profile.QueryCount, profile.Language, inputs.coordinates.PotentialQueryTreeRoles, profile.Set),
		Guard: classificationGoDefinitionGuardBinding{
			ReaderRevision: readerRevision, NativeContext: plan.NativeContext, ObservationSourceRevision: plan.SourceRevision, ObservationRunID: plan.RunID,
			ObservationAttempt: s.approval.Candidate.Attempt, ObservationPlatform: s.approval.Candidate.Platform,
			Case: anchors.Case, Route: anchors.Route, Workload: anchors.Workload, Operation: anchors.Operation,
			NativeInvocationID: nativeInvocation.ID, ReferenceInvocationIDs: orderedReferenceIDs,
			CaseSourceSHA256: anchors.Source.SHA256, AnchorsSHA256: anchors.sha256, RuntimeManifestSHA256: plan.RuntimeManifestSHA256,
			ProducerABI: s.witness.Native.ProducerABI, DriverSHA256: nativeInvocation.Executable.Member.SHA256, ReferenceExecutableSHA256: referenceExecutableSHA,
			ReferenceLibrarySHA256: s.reg.Native.ReferenceLibrary.Member.SHA256, CurrentLibrarySHA256: libraryPin.Member.SHA256, CompilerSHA256: libraryNode.Identities["tool"],
			BuildIdentitySHA256: s.reg.Build.Identity.Member.SHA256, SourceSnapshotSHA256: s.reg.Build.SourceSnapshot.Member.SHA256, VendorSnapshotSHA256: s.reg.Build.VendorSnapshot.Member.SHA256,
			Grammar: slices.Clone(plan.Grammar), ApprovalSHA256: core.ApprovalSHA256, RegistrationSHA256: core.RegistrationSHA256, WitnessSHA256: core.WitnessSHA256,
			CatalogueSHA256: core.CatalogueSHA256, GraphSHA256: core.GraphSHA256, Provenance: slices.Clone(s.approval.Provenance), NativeObservationCreated: false,
			EligibilityResultSchema: core.ResultSchema, EligibilityExecutionStatus: core.ExecutionStatus, EligibilityAssessment: core.Assessment, EligibilityComplete: core.Complete, EligibilityTuple: core.Tuple,
		},
		Inventory: inputs.coordinates, Definitions: definitions,
	}
	return result, nil
}

type classificationEmptyStaticFixture struct {
	profile     classificationEmptyDefinitionProfile
	anchors     classificationAnchors
	plan        classificationPlan
	grammarRoot string
	inputs      classificationGoDefinitionInputs
	sourceFiles []string
	queryData   []byte
}

func classificationEmptyStaticFixtureForTest(t *testing.T, name string) classificationEmptyStaticFixture {
	t.Helper()
	profile, err := classificationEmptyProfileForName(name)
	if err != nil {
		t.Fatal(err)
	}
	anchorData := actualJSONRead(t, "../testdata/query-semantics/"+profile.AnchorFile)
	if len(anchorData) == 0 || digestHex(anchorData) != profile.AnchorSHA {
		t.Fatalf("%s anchor pin differs", profile.Name)
	}
	var anchors classificationAnchors
	if err := jsonv2.Unmarshal(anchorData, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	anchors.sha256 = profile.AnchorSHA
	if err := classificationEmptyAnchorsValid(profile, anchors); err != nil {
		t.Fatal(err)
	}
	nativeRoutesData := actualJSONRead(t, "../contracts/native-routes.json")
	grammar, err := classificationEmptyNativeRoutePins(nativeRoutesData, profile)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "classification-root")
	grammarRoot := filepath.Join(t.TempDir(), "prepared", "routes", profile.Route)
	output := filepath.Join(root, "library", profile.Route+".dll")
	args, err := classificationEmptyCompilerArgs(profile, grammarRoot, output, grammar)
	if err != nil {
		t.Fatal(err)
	}
	plan := classificationPlan{
		Schema: "tsgk-query-classification-plan/r1", SourceRevision: strings.Repeat("a", 40), RunID: "empty-static-plan",
		AnchorsSHA256: profile.AnchorSHA, RuntimeManifestSHA256: strings.Repeat("b", 64), CompilerSHA256: strings.Repeat("c", 64),
		NativeContext: "full-input-utf8/r2", Grammar: grammar,
		LibraryRequest: eligibilityProcessRequest{Path: filepath.FromSlash("C:/toolchain/cc.exe"), Args: args, Dir: root, Env: []string{
			"COMSPEC=C:/Windows/System32/cmd.exe",
			"PATH=C:/Windows/System32;C:/Windows",
			"PATHEXT=.COM;.EXE;.BAT;.CMD",
			"SystemDrive=C:",
			"SystemRoot=C:/Windows",
			"TEMP=C:/Temp",
			"TMP=C:/Temp",
			"TMPDIR=C:/Temp",
			"WINDIR=C:/Windows",
		}, StdoutBytes: 16 << 20, StderrBytes: 4 << 20, Wall: 120e9, Grace: 2e9},
	}
	plan.LibraryRequest.Memory.Bytes, plan.LibraryRequest.Memory.Hard = 4<<30, true
	if err := classificationEmptyLibraryRequestValid(profile, plan, root, grammarRoot); err != nil {
		t.Fatal(err)
	}
	queryFilePath := filepath.Join("..", filepath.FromSlash(strings.TrimPrefix(profile.QueryFile, "src/")))
	queryData := actualJSONRead(t, queryFilePath)
	inventoryData := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
	qualificationData := actualJSONRead(t, "../contracts/qualification-c1.json")
	inputs, sourceFiles, err := classificationEmptyDefinitionLoadInputs(anchors, plan, inventoryData, qualificationData, queryData)
	if err != nil {
		t.Fatal(err)
	}
	return classificationEmptyStaticFixture{profile: profile, anchors: anchors, plan: plan, grammarRoot: grammarRoot, inputs: inputs, sourceFiles: sourceFiles, queryData: queryData}
}

func TestQueryClassificationWeb16EntrySelectionIsDisjoint(t *testing.T) {
	webProfiles := []string{"javascript4-empty", "jsx3-empty", "typescript7-empty", "tsx2-empty"}
	for _, profile := range webProfiles {
		if !classificationWeb16ProfileName(profile) || !classificationGoLocalSkipsProfile(profile) {
			t.Fatalf("Web16 profile %q is not routed exclusively through its explicit local entry", profile)
		}
		if classificationSmall17ProfileName(profile) {
			t.Fatalf("Web16 profile %q was admitted by the Small17 entry", profile)
		}
		if _, err := classificationCJRProfileForName(profile); err == nil {
			t.Fatalf("Web16 profile %q was admitted by the CJR entry", profile)
		}
	}
	for _, profile := range []string{"csharp15-empty", "c4-empty", "java5-empty", "rust4-empty", "cpp4-empty", "json1-tiny", "yaml1-empty", "xml2-tiny", "html2-empty", "css11-empty"} {
		if !classificationGoLocalSkipsProfile(profile) || classificationWeb16ProfileName(profile) {
			t.Fatalf("non-Web16 profile %q crossed the broad Go/Web16 entry boundary", profile)
		}
	}
	if classificationGoLocalSkipsProfile("") || classificationGoLocalSkipsProfile("positive-batch") || classificationWeb16ProfileName("javascript") {
		t.Fatal("default Go or positive-batch profile was taken by the Web16-only selection")
	}
}

func TestQueryClassificationWeb16ProfilesAndCompilerClosure(t *testing.T) {
	names := []string{"javascript4-empty", "jsx3-empty", "typescript7-empty", "tsx2-empty"}
	var totalDefinitions, totalPatterns, totalCaptureEntries uint64
	allCaptureNames := make(map[string]bool)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			fixture := classificationEmptyStaticFixtureForTest(t, name)
			profile, anchors := fixture.profile, fixture.anchors
			if !fixture.inputs.workload.API || anchors.API || anchors.Source.Bytes != 0 || anchors.Source.UTF8 != "" || len(anchors.Nodes) != 1 || anchors.Nodes[0].Type != "program" ||
				anchors.nativeContext() != "full-input-utf8/r2" || anchors.nativeSymbol() != profile.Symbol || anchors.libraryName() != profile.Route+".dll" || classificationNativePrefix(anchors) != "current/native/" ||
				!classificationPlanContextMatches(anchors, fixture.plan) {
				t.Fatalf("%s empty full-input source or native route binding differs", profile.Name)
			}
			totalDefinitions += uint64(len(anchors.Queries))
			for _, query := range anchors.Queries {
				totalPatterns += uint64(query.Metadata.Patterns)
				totalCaptureEntries += uint64(len(query.Metadata.CaptureNames))
				for _, captureName := range query.Metadata.CaptureNames {
					allCaptureNames[captureName] = true
				}
			}
			if fixture.inputs.coordinates.Definitions != uint64(profile.QueryCount) || fixture.inputs.coordinates.Cases != profile.Counts.Cases ||
				fixture.inputs.coordinates.Edits != profile.Counts.Edits || fixture.inputs.coordinates.CaseSteps != profile.Counts.CaseSteps ||
				fixture.inputs.coordinates.TreeRoles != profile.Counts.TreeRoles || fixture.inputs.coordinates.PotentialQueryTreeRoles != profile.Counts.PotentialQueryTreeRoles ||
				fixture.inputs.slotGroup.Counts != profile.Counts || len(fixture.inputs.queryFile.Queries) != profile.QueryCount || fixture.inputs.workload.Symbol != profile.Symbol {
				t.Fatalf("%s exact query-coordinate denominator differs: %#v", profile.Name, fixture.inputs.coordinates)
			}
			wantIncludeDirs, wantSources := []string{"src"}, []string{"src/parser.c", "src/scanner.c"}
			if profile.Route == "typescript" {
				wantIncludeDirs, wantSources = []string{"typescript/src", "common"}, []string{"typescript/src/parser.c", "typescript/src/scanner.c"}
			} else if profile.Route == "tsx" {
				wantIncludeDirs, wantSources = []string{"tsx/src", "common"}, []string{"tsx/src/parser.c", "tsx/src/scanner.c"}
			}
			if !reflect.DeepEqual(profile.CompilerIncludeDirs, wantIncludeDirs) || !reflect.DeepEqual(profile.CompilerSourceFiles, wantSources) {
				t.Fatalf("%s compiler parser/scanner and include closure differs", profile.Name)
			}
			args, err := classificationEmptyCompilerArgs(profile, fixture.grammarRoot, filepath.Join(fixture.plan.LibraryRequest.Dir, "library", profile.Route+".dll"), fixture.plan.Grammar)
			if err != nil || !reflect.DeepEqual(args, fixture.plan.LibraryRequest.Args) {
				t.Fatalf("%s exact compiler argv changed: %v", profile.Name, err)
			}
			badGrammar := slices.Clone(fixture.plan.Grammar)
			badGrammar[0].Path += ".alias"
			if _, err := classificationEmptyCompilerArgs(profile, fixture.grammarRoot, filepath.Join(fixture.plan.LibraryRequest.Dir, "library", profile.Route+".dll"), badGrammar); err == nil {
				t.Fatal("grammar path alias accepted")
			}
			badGrammar = slices.Clone(fixture.plan.Grammar)
			if badGrammar[0].Role == "parser" {
				badGrammar[0].Role = "scanner"
			} else {
				badGrammar[0].Role = "parser"
			}
			if _, err := classificationEmptyCompilerArgs(profile, fixture.grammarRoot, filepath.Join(fixture.plan.LibraryRequest.Dir, "library", profile.Route+".dll"), badGrammar); err == nil {
				t.Fatal("grammar role alias accepted")
			}
			witness, native, references := classificationSmall17RowsFixture(anchors)
			rows, err := classificationEmptyDefinitionRows(anchors, fixture.inputs, fixture.sourceFiles, witness, native, references)
			if err != nil || len(rows) != profile.QueryCount {
				t.Fatalf("%s predicate-free static rows rejected: %v", profile.Name, err)
			}
			for i, row := range rows {
				if row.Query != anchors.Queries[i].ID || row.SourceFile != profile.QueryFile || row.Class != "STRUCTURAL_ONLY" || row.Assessment != "NOT_ASSESSED" ||
					row.SemanticRequired != nil || row.SemanticEvaluated || row.InventoryClass != "STRUCTURAL_CANDIDATE" || row.CompiledMetadata != "UNVERIFIED" ||
					row.PotentialTreeRoles != profile.Counts.TreeRoles || row.Metadata.Predicates == nil || len(row.Metadata.Predicates) != 0 {
					t.Fatalf("%s definition row %d was promoted or lost its exact source binding: %#v", profile.Name, i, row)
				}
			}
		})
	}
	if totalDefinitions != 16 || totalPatterns != 51 || totalCaptureEntries != 54 || len(allCaptureNames) != 48 {
		t.Fatalf("closed Web16 metadata denominator changed: definitions=%d patterns=%d capture_entries=%d unique_names=%d", totalDefinitions, totalPatterns, totalCaptureEntries, len(allCaptureNames))
	}
}

func TestQueryClassificationWeb16JavaScriptAndJSXShareSymbolWithoutRouteExchange(t *testing.T) {
	javascript := classificationEmptyStaticFixtureForTest(t, "javascript4-empty")
	jsx := classificationEmptyStaticFixtureForTest(t, "jsx3-empty")
	if javascript.profile.Symbol != "tree_sitter_javascript" || jsx.profile.Symbol != javascript.profile.Symbol ||
		javascript.anchors.nativeSymbol() != javascript.profile.Symbol || jsx.anchors.nativeSymbol() != javascript.profile.Symbol ||
		javascript.inputs.workload.Symbol != javascript.profile.Symbol || jsx.inputs.workload.Symbol != javascript.profile.Symbol {
		t.Fatal("the pinned native-routes and qualification rows must agree on the shared JavaScript parser symbol")
	}
	if javascript.profile.Route == jsx.profile.Route || javascript.profile.AnchorSHA == jsx.profile.AnchorSHA ||
		javascript.profile.QueryFile == jsx.profile.QueryFile || javascript.profile.QueryFilePin.SHA256 == jsx.profile.QueryFilePin.SHA256 {
		t.Fatal("JavaScript and JSX route, anchor, and query-source identities were conflated")
	}
	if err := classificationEmptyAnchorsValid(javascript.profile, jsx.anchors); err == nil {
		t.Fatal("JSX anchor accepted under the JavaScript route")
	}
	if err := classificationEmptyAnchorsValid(jsx.profile, javascript.anchors); err == nil {
		t.Fatal("JavaScript anchor accepted under the JSX route")
	}
	inventoryData := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
	qualificationData := actualJSONRead(t, "../contracts/qualification-c1.json")
	if _, _, err := classificationEmptyDefinitionLoadInputs(javascript.anchors, javascript.plan, inventoryData, qualificationData, jsx.queryData); err == nil {
		t.Fatal("JSX query bytes accepted under the JavaScript route")
	}
	if _, _, err := classificationEmptyDefinitionLoadInputs(jsx.anchors, jsx.plan, inventoryData, qualificationData, javascript.queryData); err == nil {
		t.Fatal("JavaScript query bytes accepted under the JSX route")
	}
}

func TestQueryClassificationWeb16RejectsSourceAliasesAndReusedReferences(t *testing.T) {
	fixture := classificationEmptyStaticFixtureForTest(t, "typescript7-empty")
	anchors := fixture.anchors
	for _, name := range []string{"javascript", "javascript4-empty-alias", "jsx3-empty ", "typescript7-empty2", "tsx2", ""} {
		if classificationWeb16ProfileName(name) {
			t.Fatalf("unknown or near-alias Web16 profile %q accepted", name)
		}
	}
	for _, mutate := range []func(*classificationAnchors){
		func(a *classificationAnchors) { a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0] },
		func(a *classificationAnchors) { a.Required[0], a.Required[1] = a.Required[1], a.Required[0] },
		func(a *classificationAnchors) { a.Queries[0].Metadata.Predicates = nil },
		func(a *classificationAnchors) { a.Queries[0].Metadata.Predicates = []QueryPredicate{{}} },
		func(a *classificationAnchors) { a.Queries[0].Metadata.CaptureNames = nil },
	} {
		bad := anchors
		bad.Queries = slices.Clone(anchors.Queries)
		bad.Required = slices.Clone(anchors.Required)
		mutate(&bad)
		if err := classificationEmptyAnchorsValid(fixture.profile, bad); err == nil {
			t.Fatal("mutated Web16 anchor order, tuple, or complete metadata accepted")
		}
	}
	ids := make(map[string]string, len(anchors.Queries))
	for i, query := range anchors.Queries {
		ids[query.ID] = fmt.Sprintf("reference-%d", i)
	}
	ordered, err := classificationEmptyOrderedReferenceIDs(anchors, ids)
	if err != nil || len(ordered) != fixture.profile.QueryCount {
		t.Fatalf("unique ordered reference invocations rejected: %v", err)
	}
	ids[anchors.Queries[1].ID] = ids[anchors.Queries[0].ID]
	if _, err := classificationEmptyOrderedReferenceIDs(anchors, ids); err == nil {
		t.Fatal("one reused reference invocation was accepted for two query definitions")
	}
	ids = make(map[string]string, len(anchors.Queries))
	for i, query := range anchors.Queries {
		ids[query.ID] = fmt.Sprintf("reference-%d", i)
	}
	delete(ids, anchors.Queries[0].ID)
	if _, err := classificationEmptyOrderedReferenceIDs(anchors, ids); err == nil {
		t.Fatal("missing query reference invocation accepted")
	}
	queryJSX := actualJSONRead(t, "../testdata/native/queries/jsx.json")
	if _, _, err := classificationEmptyDefinitionLoadInputs(anchors, fixture.plan,
		actualJSONRead(t, "../contracts/query-slot-inventory-r1.json"), actualJSONRead(t, "../contracts/qualification-c1.json"), queryJSX); err == nil {
		t.Fatal("JSX query bytes accepted as the TypeScript route source")
	}
	witness, native, references := classificationSmall17RowsFixture(anchors)
	witness[0].Metadata.Predicates = nil
	if _, err := classificationEmptyDefinitionRows(anchors, fixture.inputs, fixture.sourceFiles, witness, native, references); err == nil {
		t.Fatal("nil witness predicates accepted as explicit empty predicate metadata")
	}
	witness, native, references = classificationSmall17RowsFixture(anchors)
	native[0].Predicates = []QueryPredicate{{}}
	if _, err := classificationEmptyDefinitionRows(anchors, fixture.inputs, fixture.sourceFiles, witness, native, references); err == nil {
		t.Fatal("nonempty compiled predicates accepted")
	}
	witness, native, references = classificationSmall17RowsFixture(anchors)
	references[0].Query.CaptureNames = nil
	if _, err := classificationEmptyDefinitionRows(anchors, fixture.inputs, fixture.sourceFiles, witness, native, references); err == nil {
		t.Fatal("missing reference capture names accepted")
	}
}
