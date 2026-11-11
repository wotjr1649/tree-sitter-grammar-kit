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

const classificationSmall17FactQueryPackSHA = "f86e4c8072d30480bfdd7a7a77a3c6d92fb52090b910deeb38b9137fa4b4e21a"
const classificationSmall17NativeRoutesSHA = "7be36efe4cafcefa460c8841c8698a0d055cb25d6f86a5bed8561d601c093ad2"

type classificationSmall17GrammarFile struct {
	Path string
	Role string
}

type classificationSmall17DefinitionProfile struct {
	Name, AnchorFile, AnchorSHA string
	Route, Case, Workload       string
	QueryFile, QuerySHA         string
	Set, Symbol, RootType       string
	Language                    string
	QueryCount                  int
	NodeCount                   int
	SourceUTF8                  string
	ABI                         eligibilityProducerABI
	Counts                      classificationGoDefinitionSlotCounts
	QueryIDs                    []string
	QuerySourcePins             []classificationGoDefinitionQuerySourcePin
	GrammarShape                []classificationSmall17GrammarFile
	CompilerIncludeDirs         []string
	CompilerSourceFiles         []string
}

func classificationSmall17ProfileName(name string) bool {
	_, err := classificationSmall17ProfileForName(name)
	return err == nil
}

func classificationSmall17ProfileForName(name string) (classificationSmall17DefinitionProfile, error) {
	route := map[string]string{
		"json1-tiny":  "json",
		"yaml1-empty": "yaml",
		"xml2-tiny":   "xml",
		"html2-empty": "html",
		"css11-empty": "css",
	}[name]
	if route == "" {
		return classificationSmall17DefinitionProfile{}, fmt.Errorf("unknown closed Small17 profile %q", name)
	}
	profile, ok := classificationSmall17ProfileForRoute(route)
	if !ok || profile.Name != name {
		return classificationSmall17DefinitionProfile{}, fmt.Errorf("closed Small17 profile binding is missing: %q", name)
	}
	return profile, nil
}

func classificationSmall17ProfileForRoute(route string) (classificationSmall17DefinitionProfile, bool) {
	abi := func(symbols, fields uint32) eligibilityProducerABI {
		return eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: symbols, FieldCount: fields}
	}
	queryPin := func(path string, size uint64, sha string) classificationGoDefinitionQuerySourcePin {
		return classificationGoDefinitionQuerySourcePin{Path: path, Bytes: size, SHA256: sha}
	}
	const (
		allocHeader  = "src/tree_sitter/alloc.h"
		arrayHeader  = "src/tree_sitter/array.h"
		parserHeader = "src/tree_sitter/parser.h"
	)
	commonHeaders := func(prefix string) []classificationSmall17GrammarFile {
		return []classificationSmall17GrammarFile{
			{Path: prefix + allocHeader, Role: "header"},
			{Path: prefix + arrayHeader, Role: "header"},
			{Path: prefix + parserHeader, Role: "header"},
		}
	}
	withCommonHeaders := func(prefix string, files ...classificationSmall17GrammarFile) []classificationSmall17GrammarFile {
		out := slices.Clone(files)
		return append(out, commonHeaders(prefix)...)
	}
	switch route {
	case "json":
		return classificationSmall17DefinitionProfile{
			Name: "json1-tiny", AnchorFile: "json1-classification-anchors.json", AnchorSHA: "40498d8392c3da1437b88b10e8ee06aaf082e6de35ae4bd14eead1d40a2cc1fd",
			Route: "json", Case: "json1-tiny-definitions", Workload: "json1-tiny-definition-single-frame",
			QueryFile: "src/testdata/native/queries/json.json", QuerySHA: "3b1c4cac01ee08056098e6a5a7fa463158222bc0b3c345155925d1cc1e4373c3",
			Set: "s06-json", Symbol: "tree_sitter_json", RootType: "document", Language: "JSON",
			QueryCount: 1, NodeCount: 2, SourceUTF8: "null", ABI: abi(23, 2),
			QueryIDs:            []string{"q.structure"},
			QuerySourcePins:     []classificationGoDefinitionQuerySourcePin{queryPin("src/testdata/native/queries/json.json", 2321, "3b1c4cac01ee08056098e6a5a7fa463158222bc0b3c345155925d1cc1e4373c3")},
			GrammarShape:        withCommonHeaders("", classificationSmall17GrammarFile{Path: "src/parser.c", Role: "parser"}),
			CompilerIncludeDirs: []string{"src"}, CompilerSourceFiles: []string{"src/parser.c"},
			Counts: classificationGoDefinitionSlotCounts{Cases: 23, Edits: 24, CaseSteps: 47, TreeRoles: 71, PotentialQueryTreeRoles: 71},
		}, true
	case "yaml":
		return classificationSmall17DefinitionProfile{
			Name: "yaml1-empty", AnchorFile: "yaml1-classification-anchors.json", AnchorSHA: "57c5d0961dc1be0016340dd5b229cf44a748362b1d9089769a3e95e420ad0c1e",
			Route: "yaml", Case: "yaml1-empty-definitions", Workload: "yaml1-empty-definition-single-frame",
			QueryFile: "src/testdata/native/queries/yaml.json", QuerySHA: "008d4d04351a7e9a4eb39e38f1820de313fcb2c7f6bc0527caf4efc4695e79b1",
			Set: "s06-yaml", Symbol: "tree_sitter_yaml", RootType: "stream", Language: "YAML",
			QueryCount: 1, NodeCount: 1, SourceUTF8: "", ABI: abi(301, 2),
			QueryIDs:        []string{"q.structure"},
			QuerySourcePins: []classificationGoDefinitionQuerySourcePin{queryPin("src/testdata/native/queries/yaml.json", 2426, "008d4d04351a7e9a4eb39e38f1820de313fcb2c7f6bc0527caf4efc4695e79b1")},
			GrammarShape: withCommonHeaders("",
				classificationSmall17GrammarFile{Path: "src/parser.c", Role: "parser"},
				classificationSmall17GrammarFile{Path: "src/scanner.c", Role: "scanner"},
				classificationSmall17GrammarFile{Path: "src/schema.core.c", Role: "header"},
				classificationSmall17GrammarFile{Path: "src/schema.json.c", Role: "header"},
				classificationSmall17GrammarFile{Path: "src/schema.legacy.c", Role: "header"}),
			CompilerIncludeDirs: []string{"src"}, CompilerSourceFiles: []string{"src/parser.c", "src/scanner.c"},
			Counts: classificationGoDefinitionSlotCounts{Cases: 27, Edits: 22, CaseSteps: 49, TreeRoles: 71, PotentialQueryTreeRoles: 71},
		}, true
	case "xml":
		return classificationSmall17DefinitionProfile{
			Name: "xml2-tiny", AnchorFile: "xml2-classification-anchors.json", AnchorSHA: "ef7ea68d923b5a551fe2805184cfd949aca2cf57254679844f090de1234f238f",
			Route: "xml", Case: "xml2-tiny-definitions", Workload: "xml2-tiny-definition-single-frame",
			QueryFile: "src/testdata/native/queries/xml.json", QuerySHA: "32e54336d7a2189cbf497644717ccee3d6ccfb2d35db2686c6c4b21ef0f12847",
			Set: "s06-xml", Symbol: "tree_sitter_xml", RootType: "document", Language: "XML",
			QueryCount: 2, NodeCount: 6, SourceUTF8: "<r/>", ABI: abi(142, 2),
			QueryIDs: []string{"q.structure", "facts.xml_structure"},
			QuerySourcePins: []classificationGoDefinitionQuerySourcePin{
				queryPin("src/testdata/native/queries/xml.json", 2409, "32e54336d7a2189cbf497644717ccee3d6ccfb2d35db2686c6c4b21ef0f12847"),
				queryPin("src/contracts/fact-query-pack.json", 9702, classificationSmall17FactQueryPackSHA),
			},
			GrammarShape: withCommonHeaders("xml/",
				classificationSmall17GrammarFile{Path: "common/scanner.h", Role: "header"},
				classificationSmall17GrammarFile{Path: "xml/src/parser.c", Role: "parser"},
				classificationSmall17GrammarFile{Path: "xml/src/scanner.c", Role: "scanner"}),
			CompilerIncludeDirs: []string{"xml/src", "common"}, CompilerSourceFiles: []string{"xml/src/parser.c", "xml/src/scanner.c"},
			Counts: classificationGoDefinitionSlotCounts{Cases: 30, Edits: 34, CaseSteps: 64, TreeRoles: 98, PotentialQueryTreeRoles: 196},
		}, true
	case "html":
		return classificationSmall17DefinitionProfile{
			Name: "html2-empty", AnchorFile: "html2-classification-anchors.json", AnchorSHA: "e61754ca5f6873df5e0be601171b75b012d607f4e28e71a4d1f213e29c7685d7",
			Route: "html", Case: "html2-empty-definitions", Workload: "html2-empty-definition-single-frame",
			QueryFile: "src/testdata/native/queries/html.json", QuerySHA: "dca8b1397376a0f013d8180b439d1c5c0cc639971b53c9abd0c4769d0bb59955",
			Set: "s06-html", Symbol: "tree_sitter_html", RootType: "document", Language: "HTML",
			QueryCount: 2, NodeCount: 1, SourceUTF8: "", ABI: abi(48, 0),
			QueryIDs:        []string{"q.structure", "q.source"},
			QuerySourcePins: []classificationGoDefinitionQuerySourcePin{queryPin("src/testdata/native/queries/html.json", 5003, "dca8b1397376a0f013d8180b439d1c5c0cc639971b53c9abd0c4769d0bb59955")},
			GrammarShape: withCommonHeaders("",
				classificationSmall17GrammarFile{Path: "src/parser.c", Role: "parser"},
				classificationSmall17GrammarFile{Path: "src/scanner.c", Role: "scanner"},
				classificationSmall17GrammarFile{Path: "src/tag.h", Role: "header"}),
			CompilerIncludeDirs: []string{"src"}, CompilerSourceFiles: []string{"src/parser.c", "src/scanner.c"},
			Counts: classificationGoDefinitionSlotCounts{Cases: 34, Edits: 8, CaseSteps: 42, TreeRoles: 50, PotentialQueryTreeRoles: 100},
		}, true
	case "css":
		return classificationSmall17DefinitionProfile{
			Name: "css11-empty", AnchorFile: "css11-classification-anchors.json", AnchorSHA: "aa271590746efbf2ade6b36d0ae4cc4b555dbfeeb558e86358a10c8cc20770b5",
			Route: "css", Case: "css11-empty-definitions", Workload: "css11-empty-definition-single-frame",
			QueryFile: "src/testdata/native/queries/css.json", QuerySHA: "92de676cf8ebf4234ee97dcebe531edd4f5cbc569c50e4dc2fe6d51d20e4bb21",
			Set: "s06-css", Symbol: "tree_sitter_css", RootType: "stylesheet", Language: "CSS",
			QueryCount: 11, NodeCount: 1, SourceUTF8: "", ABI: abi(204, 0),
			QueryIDs:        []string{"q.structure", "q.property", "q.value", "q.call", "q.selectors", "q.import", "q.calc", "q.at", "q.media", "q.grid", "q.supports"},
			QuerySourcePins: []classificationGoDefinitionQuerySourcePin{queryPin("src/testdata/native/queries/css.json", 36030, "92de676cf8ebf4234ee97dcebe531edd4f5cbc569c50e4dc2fe6d51d20e4bb21")},
			GrammarShape: withCommonHeaders("",
				classificationSmall17GrammarFile{Path: "src/parser.c", Role: "parser"},
				classificationSmall17GrammarFile{Path: "src/scanner.c", Role: "scanner"}),
			CompilerIncludeDirs: []string{"src"}, CompilerSourceFiles: []string{"src/parser.c", "src/scanner.c"},
			Counts: classificationGoDefinitionSlotCounts{Cases: 246, Edits: 30, CaseSteps: 276, TreeRoles: 306, PotentialQueryTreeRoles: 3366},
		}, true
	default:
		return classificationSmall17DefinitionProfile{}, false
	}
}

func classificationSmall17NativeRoutePins(data []byte, profile classificationSmall17DefinitionProfile) ([]NativeInput, error) {
	if len(data) != 118237 || digestHex(data) != classificationSmall17NativeRoutesSHA {
		return nil, fmt.Errorf("pinned native-routes registry differs")
	}
	var registry struct {
		Schema string `json:"schema"`
		Routes []struct {
			Route string        `json:"route"`
			Files []NativeInput `json:"files"`
		} `json:"routes"`
	}
	if err := jsonv2.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("native-routes shape: %w", err)
	}
	if registry.Schema != "tsgk-native-routes/r1" {
		return nil, fmt.Errorf("native-routes schema differs")
	}
	var files []NativeInput
	var routeCount int
	for _, route := range registry.Routes {
		if route.Route == profile.Route {
			routeCount++
			files = route.Files
		}
	}
	if routeCount != 1 || len(files) != len(profile.GrammarShape) {
		return nil, fmt.Errorf("%s native route is missing, duplicated, or has the wrong input count", profile.Name)
	}
	for i, file := range files {
		want := profile.GrammarShape[i]
		if file.Path != want.Path || file.Role != want.Role || file.Bytes == 0 || !hex64.MatchString(file.SHA256) {
			return nil, fmt.Errorf("%s native route input %d changed", profile.Name, i)
		}
	}
	return slices.Clone(files), nil
}

func classificationSmall17AnchorsValid(profile classificationSmall17DefinitionProfile, a classificationAnchors) error {
	if a.sha256 != profile.AnchorSHA || a.Schema != "tsgk-query-classification-anchors/r1" || a.Route != profile.Route || a.Case != profile.Case || a.Workload != profile.Workload ||
		a.Operation != "native-query" || a.DriverMode != "single" || a.API || a.Source.UTF8 != profile.SourceUTF8 || a.Source.Bytes != uint64(len(profile.SourceUTF8)) ||
		a.Source.SHA256 != digestHex([]byte(profile.SourceUTF8)) || a.Edits == nil || len(a.Edits) != 0 || len(a.Queries) != profile.QueryCount ||
		len(a.Required) != profile.QueryCount || len(a.Nodes) != profile.NodeCount || a.Nodes[0].Type != profile.RootType ||
		!reflect.DeepEqual(a.Expect, []StepExpectation{{Step: 0, Syntax: "NO_ERROR"}}) || a.ABI != profile.ABI {
		return fmt.Errorf("%s fixed single-frame anchor changed", profile.Name)
	}
	if ValidateTree(a.Nodes, a.Source.Bytes) != nil || !reflect.DeepEqual(a.Nodes, classificationSmall17ExpectedNodes(profile.Route)) {
		return fmt.Errorf("%s independently pinned source CST changed", profile.Name)
	}
	seen := make(map[string]bool, profile.QueryCount)
	for i, query := range a.Queries {
		if query.ID != profile.QueryIDs[i] || !validID(query.ID) || seen[query.ID] || query.SHA256 != digestHex([]byte(query.Source)) || !classificationGoDefinitionMetadataValid(query.Metadata) {
			return fmt.Errorf("%s definition anchor %d has invalid or reordered identity/metadata", profile.Name, i)
		}
		seen[query.ID] = true
		tuple := a.Required[i]
		if tuple.Query != query.ID || tuple.Step != 0 || tuple.TreeRole != "incremental" || tuple.Structural == nil || tuple.Expected == nil {
			return fmt.Errorf("%s required query tuple %d is incomplete or reordered", profile.Name, i)
		}
		if profile.Route == "xml" && query.ID == "facts.xml_structure" {
			want := classificationSmall17XMLFactCaptures()
			if !reflect.DeepEqual(tuple.Structural, want) || !reflect.DeepEqual(tuple.Expected, want) {
				return fmt.Errorf("XML fact query must retain the fixed EmptyElemTag-before-Name capture stream")
			}
		} else if len(tuple.Structural) != 0 || len(tuple.Expected) != 0 {
			return fmt.Errorf("%s query %s is not the fixed empty-capture obligation", profile.Name, query.ID)
		}
	}
	return nil
}

func classificationSmall17ExpectedNodes(route string) []TreeNode {
	node := func(parent int64, typ string, field *string, named bool, start, end uint32) TreeNode {
		return TreeNode{Parent: parent, Type: typ, Field: field, Named: named, StartByte: start, EndByte: end, StartPoint: Point{Column: start}, EndPoint: Point{Column: end}}
	}
	switch route {
	case "json":
		return []TreeNode{node(-1, "document", nil, true, 0, 4), node(0, "null", nil, true, 0, 4)}
	case "yaml":
		return []TreeNode{node(-1, "stream", nil, true, 0, 0)}
	case "xml":
		root := "root"
		return []TreeNode{
			node(-1, "document", nil, true, 0, 4),
			node(0, "element", &root, true, 0, 4),
			node(1, "EmptyElemTag", nil, true, 0, 4),
			node(2, "<", nil, false, 0, 1),
			node(2, "Name", nil, true, 1, 2),
			node(2, "/>", nil, false, 2, 4),
		}
	case "html":
		return []TreeNode{node(-1, "document", nil, true, 0, 0)}
	case "css":
		return []TreeNode{node(-1, "stylesheet", nil, true, 0, 0)}
	default:
		return nil
	}
}

func classificationSmall17XMLFactCaptures() []Capture {
	return []Capture{
		{Match: 0, Pattern: 1, Capture: 1, Name: "element", Node: 2, Type: "EmptyElemTag", Named: true, StartByte: 0, EndByte: 4, StartPoint: Point{}, EndPoint: Point{Column: 4}},
		{Match: 0, Pattern: 1, Capture: 0, Name: "element.name", Node: 4, Type: "Name", Named: true, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}},
	}
}

func classificationSmall17DefinitionLoadInputs(a classificationAnchors, plan classificationPlan, inventoryData, qualificationData, queryFileData, factPackData []byte) (classificationGoDefinitionInputs, []string, error) {
	var out classificationGoDefinitionInputs
	profile, ok := classificationSmall17ProfileForRoute(a.Route)
	if !ok {
		return out, nil, fmt.Errorf("unsupported closed Small17 route %q", a.Route)
	}
	if err := classificationSmall17AnchorsValid(profile, a); err != nil {
		return out, nil, err
	}
	if plan.Schema != "tsgk-query-classification-plan/r1" || plan.AnchorsSHA256 != profile.AnchorSHA || plan.APIAnchorsSHA256 != "" || plan.NativeContext != "full-input-utf8/r2" ||
		!eligibilityRevision(plan.SourceRevision) || !validID(plan.RunID) || !hex64.MatchString(plan.RuntimeManifestSHA256) || !hex64.MatchString(plan.CompilerSHA256) || len(plan.Grammar) != len(profile.GrammarShape) {
		return out, nil, fmt.Errorf("%s preregistration differs from its pinned full-input single-frame profile", profile.Name)
	}
	if digestHex(inventoryData) != classificationGoDefinitionInventorySHA || digestHex(qualificationData) != classificationGoDefinitionQualificationSHA {
		return out, nil, fmt.Errorf("query-slot or qualification inventory pin differs")
	}
	for _, pin := range profile.QuerySourcePins {
		data := queryFileData
		if pin.Path == "src/contracts/fact-query-pack.json" {
			data = factPackData
		}
		if uint64(len(data)) != pin.Bytes || digestHex(data) != pin.SHA256 {
			return out, nil, fmt.Errorf("%s exact query source pin differs: %s", profile.Name, pin.Path)
		}
	}
	if profile.Route != "xml" && factPackData != nil {
		return out, nil, fmt.Errorf("%s does not accept an XML fact-pack source", profile.Name)
	}
	var slots classificationGoDefinitionSlotInventory
	if err := jsonv2.Unmarshal(inventoryData, &slots, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, nil, fmt.Errorf("query-slot inventory shape: %w", err)
	}
	if slots.Schema != "tsgk-query-slot-inventory/r1" || slots.ExpansionRule != "case-query-edit-tree-role/r1" ||
		!slices.Equal(slots.Coordinate, []string{"qualification_inventory.sha256", "set", "case", "query", "step", "tree_role"}) || len(slots.Ordinary) != 26 ||
		len(slots.SemanticRequired) == 0 || !bytes.Equal(bytes.TrimSpace(slots.SemanticRequired), []byte("null")) {
		return out, nil, fmt.Errorf("query-slot inventory r1 contract differs")
	}
	if slots.Qualification.Path != "src/contracts/qualification-c1.json" || slots.Qualification.Bytes != uint64(len(qualificationData)) || slots.Qualification.SHA256 != classificationGoDefinitionQualificationSHA {
		return out, nil, fmt.Errorf("query-slot qualification pin differs")
	}
	var queryFile classificationGoDefinitionQueryFile
	if err := jsonv2.Unmarshal(queryFileData, &queryFile, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, nil, fmt.Errorf("%s query-file shape: %w", profile.Name, err)
	}
	queryFileCount := profile.QueryCount
	if profile.Route == "xml" {
		queryFileCount--
	}
	if queryFile.Schema != "tsgk-native-query-cases/r1" || queryFile.Route != profile.Route || len(queryFile.Queries) != queryFileCount {
		return out, nil, fmt.Errorf("%s raw query-file identity differs", profile.Name)
	}
	combined := slices.Clone(queryFile.Queries)
	if profile.Route == "xml" {
		var pack classificationCSharpDefinitionFactQueryPack
		if err := jsonv2.Unmarshal(factPackData, &pack, jsonv2.RejectUnknownMembers(true)); err != nil {
			return out, nil, fmt.Errorf("XML fact query pack shape: %w", err)
		}
		if pack.Schema != "tsgk-fact-query-pack/r1" || pack.Revision != "fact-queries-r1" || pack.Mapping != "declaration-facts-r1" || pack.DynamicSQL != "dynamic-sql-r1" || pack.XMLStructure != "xml-structure-r1" || len(pack.Routes) != 4 {
			return out, nil, fmt.Errorf("XML fact query pack contract differs")
		}
		var xmlRouteCount int
		for _, route := range pack.Routes {
			if route.Route != "xml" {
				continue
			}
			xmlRouteCount++
			if len(route.Declarations) != 0 || len(route.Queries) != 1 || route.Queries[0].ID != "facts.xml_structure" || route.Queries[0].Facts != "xml_structure" {
				return out, nil, fmt.Errorf("XML fact route is missing, aliased, or reordered")
			}
			combined = append(combined, OracleQuery{ID: route.Queries[0].ID, Source: route.Queries[0].Source})
		}
		if xmlRouteCount != 1 {
			return out, nil, fmt.Errorf("XML fact route is missing or duplicated")
		}
	}
	if len(combined) != profile.QueryCount {
		return out, nil, fmt.Errorf("%s combined raw source query count differs", profile.Name)
	}
	var qualification QualificationInventory
	parsed, err := ParseQualificationInventory(qualificationData)
	if err != nil {
		return out, nil, fmt.Errorf("qualification inventory: %w", err)
	}
	qualification = parsed
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
	w := route.Workload
	if w.Set != profile.Set || w.Profile != profile.Set || w.Operation != "native-query-batch" || w.Output != "tree" || w.Symbol != profile.Symbol ||
		len(w.Grammar) != len(profile.GrammarShape) || len(w.Queries) != profile.QueryCount || !reflect.DeepEqual(w.Grammar, plan.Grammar) {
		return out, nil, fmt.Errorf("%s qualification workload differs from the fixed profile", profile.Name)
	}
	if profile.Route == "xml" {
		if w.FactPack == nil || w.FactPack.Route != "xml" || w.FactPack.Revision != "fact-queries-r1" || w.FactPack.SHA256 != classificationSmall17FactQueryPackSHA {
			return out, nil, fmt.Errorf("XML qualification fact pack binding differs")
		}
	} else if w.FactPack != nil {
		return out, nil, fmt.Errorf("%s unexpected qualification fact pack binding", profile.Name)
	}
	for _, extra := range qualification.ExtraRoles {
		for _, workload := range extra.Workloads {
			if workload.Route == profile.Route && workload.Set == profile.Set {
				return out, nil, fmt.Errorf("%s workload is duplicated in an extra role", profile.Name)
			}
		}
	}
	groupCount := 0
	for _, group := range slots.Ordinary {
		if (group.Set == profile.Set || group.Route == profile.Route) && (group.Route != profile.Route || group.Set != profile.Set || group.Profile != profile.Set) {
			return out, nil, fmt.Errorf("%s query-slot group appears under another route or profile", profile.Name)
		}
		if group.Route == profile.Route && group.Set == profile.Set {
			groupCount++
			out.slotGroup = group
		}
	}
	if groupCount != 1 || out.slotGroup.Profile != profile.Set || out.slotGroup.Operation != "native-query-batch" || len(out.slotGroup.Queries) != profile.QueryCount || out.slotGroup.Counts != profile.Counts {
		return out, nil, fmt.Errorf("%s ordinary query-slot group or fixed coordinates differ", profile.Name)
	}
	for _, expected := range profile.QuerySourcePins {
		var matches int
		for _, pin := range slots.QuerySources {
			if pin.Path == expected.Path {
				matches++
				if pin != expected {
					return out, nil, fmt.Errorf("%s query-source inventory pin differs: %s", profile.Name, expected.Path)
				}
			}
		}
		if matches != 1 {
			return out, nil, fmt.Errorf("%s query-source inventory pin is missing or duplicated: %s", profile.Name, expected.Path)
		}
	}
	sourceFiles := make([]string, profile.QueryCount)
	for i, anchor := range a.Queries {
		sourceFiles[i] = profile.QueryFile
		if profile.Route == "xml" && anchor.ID == "facts.xml_structure" {
			sourceFiles[i] = "src/contracts/fact-query-pack.json"
		}
	}
	combinedQueryFile := classificationGoDefinitionQueryFile{Schema: "tsgk-native-query-cases/r1", Route: profile.Route, Queries: combined}
	queryIDs, err := classificationGoDefinitionQueryBindings(a, combinedQueryFile, w, out.slotGroup, sourceFiles)
	if err != nil {
		return out, nil, fmt.Errorf("%s query source binding: %w", profile.Name, err)
	}
	coordinates, err := classificationGoDefinitionCoordinateProjection(qualification.SHA256, profile.Set, w, queryIDs)
	if err != nil {
		return out, nil, err
	}
	coordinates.QuerySlotInventorySHA256 = classificationGoDefinitionInventorySHA
	coordinates.QuerySourceFiles = slices.Clone(profile.QuerySourcePins)
	if coordinates.Cases != profile.Counts.Cases || coordinates.Edits != profile.Counts.Edits || coordinates.CaseSteps != profile.Counts.CaseSteps ||
		coordinates.TreeRoles != profile.Counts.TreeRoles || coordinates.Definitions != uint64(profile.QueryCount) ||
		coordinates.PotentialQueryTreeRoles != profile.Counts.PotentialQueryTreeRoles {
		return out, nil, fmt.Errorf("%s query-coordinate denominator differs", profile.Name)
	}
	out.qualification, out.workload, out.coordinates = qualification, w, coordinates
	out.queryFile = combinedQueryFile
	return out, sourceFiles, nil
}

func classificationSmall17DefinitionRows(a classificationAnchors, inputs classificationGoDefinitionInputs, sourceFiles []string, witnessQueries []QueryWitnessQuery, nativeQueries []querywire.Query, references []*instrumentedReference) ([]classificationGoDefinition, error) {
	profile, ok := classificationSmall17ProfileForRoute(a.Route)
	if !ok {
		return nil, fmt.Errorf("unsupported closed Small17 definition route %q", a.Route)
	}
	if err := classificationSmall17AnchorsValid(profile, a); err != nil {
		return nil, err
	}
	if len(sourceFiles) != profile.QueryCount {
		return nil, fmt.Errorf("%s source-file binding count differs", profile.Name)
	}
	return classificationDefinitionRows(a, inputs, witnessQueries, nativeQueries, references, profile.Language, sourceFiles)
}

func classificationSmall17OrderedReferenceIDs(a classificationAnchors, referenceIDs map[string]string) ([]string, error) {
	profile, ok := classificationSmall17ProfileForRoute(a.Route)
	if !ok || len(a.Queries) != profile.QueryCount || len(referenceIDs) != profile.QueryCount {
		return nil, fmt.Errorf("Small17 reference invocation denominator differs")
	}
	ordered := make([]string, profile.QueryCount)
	seen := make(map[string]bool, profile.QueryCount)
	queries := make(map[string]bool, profile.QueryCount)
	for i, query := range a.Queries {
		if !validID(query.ID) || queries[query.ID] {
			return nil, fmt.Errorf("Small17 reference query identity is missing or duplicated: %s", query.ID)
		}
		queries[query.ID] = true
		id := referenceIDs[query.ID]
		if !validID(id) || seen[id] {
			return nil, fmt.Errorf("Small17 reference invocation is missing, invalid, or reused: %s", query.ID)
		}
		seen[id] = true
		ordered[i] = id
	}
	return ordered, nil
}

func classificationSmall17LibraryRequestValid(profile classificationSmall17DefinitionProfile, plan classificationPlan, root, grammarRoot string) error {
	request := plan.LibraryRequest
	output := filepath.Join(root, "library", profile.Route+".dll")
	if plan.Schema != "tsgk-query-classification-plan/r1" || plan.AnchorsSHA256 != profile.AnchorSHA || plan.APIAnchorsSHA256 != "" || plan.NativeContext != "full-input-utf8/r2" ||
		!eligibilityRevision(plan.SourceRevision) || !validID(plan.RunID) || !hex64.MatchString(plan.RuntimeManifestSHA256) || !hex64.MatchString(plan.CompilerSHA256) ||
		!filepath.IsAbs(root) || !filepath.IsAbs(grammarRoot) || request.Dir != root || !eligibilityAbsolutePath(request.Path, "windows/amd64") ||
		eligibilityValidateRequest(request, "go-library") != nil || request.StdoutBytes != 16<<20 || request.StderrBytes != 4<<20 || request.Wall != 120e9 || request.Grace != 2e9 ||
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
	wantArgs, err := classificationGoCompilerArgs(profile.Route, grammarRoot, output, plan.Grammar)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(request.Args, wantArgs) {
		return fmt.Errorf("%s exact compiler parser/scanner argv differs", profile.Name)
	}
	return nil
}

// This projection consumes only the original guarded eligibility session. It adds no native,
// reference, case, or semantic observation of its own.
func classificationSmall17DefinitionProjection(anchors classificationAnchors, plan classificationPlan, s *eligibilitySession, cache *semanticNativeCache, core QueryReferenceEligibilityResult, inventoryData, qualificationData, queryFileData, factPackData []byte, readerRevision string) (classificationGoDefinitionProjectionResult, error) {
	var result classificationGoDefinitionProjectionResult
	profile, ok := classificationSmall17ProfileForRoute(anchors.Route)
	if !ok {
		return result, fmt.Errorf("unsupported closed Small17 route %q", anchors.Route)
	}
	inputs, sourceFiles, err := classificationSmall17DefinitionLoadInputs(anchors, plan, inventoryData, qualificationData, queryFileData, factPackData)
	if err != nil {
		return result, err
	}
	if !hex40.MatchString(readerRevision) {
		return result, fmt.Errorf("separately pinned reader revision is invalid")
	}
	if s == nil || cache == nil || s.res == nil || s.r == nil || s.semantic != cache || !s.bound || !cache.ready || !cache.record.haveRaw {
		return result, fmt.Errorf("Small17 definition projection requires the live guarded eligibility session")
	}
	if err := s.r.check(); err != nil {
		return result, err
	}
	wantTuple := QueryEligibilityCounts{Required: uint64(profile.QueryCount), Verified: uint64(profile.QueryCount)}
	if !classificationEligibilityResultMatches(plan, core) || core.ExecutionStatus != StatusCompleted || core.Assessment != AssessPass || !core.Complete || core.Tuple != wantTuple ||
		s.res.ResultSchema != core.ResultSchema || s.res.ExecutionStatus != core.ExecutionStatus || s.res.Assessment != core.Assessment || s.res.Complete != core.Complete || s.res.Tuple != wantTuple ||
		s.res.ApprovalSHA256 != core.ApprovalSHA256 || s.res.RegistrationSHA256 != core.RegistrationSHA256 || s.res.WitnessSHA256 != core.WitnessSHA256 ||
		s.res.CatalogueSHA256 != core.CatalogueSHA256 || s.res.GraphSHA256 != core.GraphSHA256 {
		return result, fmt.Errorf("Small17 projection requires the same completed guarded tuple")
	}
	if !classificationLibraryMatches(s) {
		return result, fmt.Errorf("Small17 reference library differs from guarded compiler output")
	}
	if plan.NativeContext != anchors.nativeContext() || plan.NativeContext != "full-input-utf8/r2" || s.reg.Context != plan.NativeContext || s.reg.Native == nil ||
		s.reg.Native.Symbol != profile.Symbol || s.reg.Native.InvocationID != "native-"+anchors.Case ||
		s.witness.Schema != queryEligibilityWitnessSchema || s.witness.Policy != "tsgk-predicates/r1" || s.witness.Case != anchors.Case ||
		s.witness.Route != profile.Route || s.witness.Role != "owned" || s.witness.Encoding != EncodingUTF8 || s.witness.Platform != "windows/amd64" ||
		s.witness.Input.Path != "source.bin" || s.witness.Input.Bytes != anchors.Source.Bytes || s.witness.Input.SHA256 != anchors.Source.SHA256 || s.witness.Input.Role != "case" ||
		s.witness.Native.RequestID != anchors.Case || s.witness.Native.API || s.witness.Native.ProducerABI != anchors.ABI ||
		s.witness.Native.RuntimeManifestSHA256 != plan.RuntimeManifestSHA256 || s.witness.Native.ObservationRunID != plan.RunID || len(s.witness.Native.Points) != 0 ||
		plan.SourceRevision != s.approval.Candidate.SourceRevision || plan.RunID != s.approval.Candidate.Run || s.approval.Candidate.Platform != "windows/amd64" || s.approval.Candidate.Attempt != 1 ||
		!reflect.DeepEqual(s.witness.Required, anchors.Required) || !reflect.DeepEqual(s.witness.Edits, []Edit{}) || len(s.witness.Queries) != profile.QueryCount ||
		len(s.reg.Native.Grammar) != len(plan.Grammar) {
		return result, fmt.Errorf("Small17 candidate, source frame, witness, or grammar context differs from the fixed profile")
	}
	if err := classificationGrammarContext(s, anchors, plan); err != nil {
		return result, fmt.Errorf("Small17 guarded grammar pin differs: %w", err)
	}
	if err := classificationSmall17LibraryRequestValid(profile, plan, plan.LibraryRequest.Dir, filepath.Join(os.Getenv("TSGK_BENCH_PREPARED"), "routes", anchors.Route)); err != nil {
		return result, fmt.Errorf("Small17 guarded compiler request differs: %w", err)
	}
	if len(cache.checked) != 1 || cache.checked[0].Step.Step != 0 || cache.checked[0].Step.SourceBytes != anchors.Source.Bytes ||
		cache.checked[0].Step.SourceSHA256 != anchors.Source.SHA256 || cache.checked[0].Step.Edit != nil || cache.checked[0].Fresh != nil ||
		CompareTrees(cache.checked[0].Incremental.Nodes, anchors.Nodes) != nil || len(cache.checked[0].Incremental.Queries) != profile.QueryCount {
		return result, fmt.Errorf("Small17 guarded single-frame source CST or query count differs")
	}
	references := make(map[string]*instrumentedReference, profile.QueryCount)
	referenceIDs := make(map[string]string, profile.QueryCount)
	var referenceExecutableSHA string
	for _, tuple := range anchors.Required {
		observation, found := s.observation(tuple)
		if !found || observation.Query != tuple.Query || observation.QueryID != tuple.Query || observation.Step != 0 || observation.TreeRole != "incremental" ||
			observation.SourceBytes != anchors.Source.Bytes || observation.SourceSHA256 != anchors.Source.SHA256 {
			return result, fmt.Errorf("Small17 guarded reference observation differs: %s", tuple.Query)
		}
		out := s.outputs[observation.InvocationID]
		if out == nil || observation.ResponseOrdinal >= uint64(len(out.responses)) || observation.ResponseOrdinal >= uint64(len(out.rows)) || out.responses[observation.ResponseOrdinal] == nil {
			return result, fmt.Errorf("Small17 guarded reference response is missing: %s", tuple.Query)
		}
		response, row := out.responses[observation.ResponseOrdinal], out.rows[observation.ResponseOrdinal]
		if response.Schema != instrumentedReferenceSchema || response.Status != StatusCompleted || response.Code != "" || response.Input.Encoding != EncodingUTF8 ||
			response.Input.Bytes != anchors.Source.Bytes || response.Input.SHA256 != anchors.Source.SHA256 || response.Grammar.Symbol != profile.Symbol ||
			CompareTrees(response.Tree.Nodes, anchors.Nodes) != nil || uint64(len(row)) != observation.ByteLength || digestHex(row) != observation.SHA256 {
			return result, fmt.Errorf("Small17 guarded reference source frame or complete CST differs: %s", tuple.Query)
		}
		invocation := s.invocations[observation.InvocationID]
		if invocation == nil || invocation.Executable == nil {
			return result, fmt.Errorf("Small17 reference executable is missing: %s", tuple.Query)
		}
		if referenceExecutableSHA == "" {
			referenceExecutableSHA = invocation.Executable.Member.SHA256
		} else if referenceExecutableSHA != invocation.Executable.Member.SHA256 {
			return result, fmt.Errorf("Small17 reference executable bindings differ")
		}
		if !hex64.MatchString(referenceExecutableSHA) {
			return result, fmt.Errorf("Small17 reference executable pin is invalid")
		}
		references[tuple.Query], referenceIDs[tuple.Query] = response, observation.InvocationID
	}
	orderedReferenceIDs, err := classificationSmall17OrderedReferenceIDs(anchors, referenceIDs)
	if err != nil {
		return result, err
	}
	orderedReferences := make([]*instrumentedReference, profile.QueryCount)
	for i, query := range anchors.Queries {
		orderedReferences[i] = references[query.ID]
		if orderedReferences[i] == nil {
			return result, fmt.Errorf("Small17 guarded reference query is missing: %s", query.ID)
		}
	}
	definitions, err := classificationSmall17DefinitionRows(anchors, inputs, sourceFiles, s.witness.Queries, cache.checked[0].Incremental.Queries, orderedReferences)
	if err != nil {
		return result, err
	}
	nativeInvocation, err := classificationNativeInvocation(s, anchors)
	if err != nil {
		return result, err
	}
	libraryPin, libraryOK := s.owned["current/go.dll"]
	if nativeInvocation.Executable == nil || nativeInvocation.Executable.Member.SHA256 != s.witness.Native.ExecutableSHA256 || !hex64.MatchString(nativeInvocation.Executable.Member.SHA256) {
		return result, fmt.Errorf("Small17 guarded native driver executable is missing or mismatched")
	}
	if !libraryOK || libraryPin.NodeID != "go-library-build" || libraryPin.Member.Role != "runtime-library" || !hex64.MatchString(libraryPin.Member.SHA256) ||
		s.reg.Build == nil || s.reg.Native.ReferenceLibrary == nil || !hex64.MatchString(s.reg.Native.ReferenceLibrary.Member.SHA256) {
		return result, fmt.Errorf("Small17 guarded build or route library binding is missing")
	}
	libraryNode, found := s.nodes["go-library-build"]
	if !found || !classificationGoLibraryNodeMatches(libraryNode, plan, anchors.Workload) || libraryNode.Identities["tool"] != plan.CompilerSHA256 {
		return result, fmt.Errorf("Small17 guarded compiler node differs from preregistration")
	}
	for _, pin := range []string{s.reg.Build.Identity.Member.SHA256, s.reg.Build.SourceSnapshot.Member.SHA256, s.reg.Build.VendorSnapshot.Member.SHA256} {
		if !hex64.MatchString(pin) {
			return result, fmt.Errorf("Small17 guarded build identity pin is invalid")
		}
	}
	result = classificationGoDefinitionProjectionResult{
		Scope: fmt.Sprintf("conditional historical classification of %d predicate-free %s definitions and %d potential %s links; one guarded native-query frame is the guard, not per-case or semantic completion", profile.QueryCount, profile.Language, inputs.coordinates.PotentialQueryTreeRoles, profile.Set),
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

type classificationSmall17StaticFixture struct {
	profile     classificationSmall17DefinitionProfile
	anchors     classificationAnchors
	plan        classificationPlan
	grammarRoot string
	inputs      classificationGoDefinitionInputs
	sourceFiles []string
	queryFile   []byte
	factPack    []byte
}

func classificationSmall17StaticFixtureForTest(t *testing.T, name string) classificationSmall17StaticFixture {
	t.Helper()
	profile, err := classificationSmall17ProfileForName(name)
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
	if err := classificationSmall17AnchorsValid(profile, anchors); err != nil {
		t.Fatal(err)
	}
	nativeRoutesData := actualJSONRead(t, "../contracts/native-routes.json")
	grammar, err := classificationSmall17NativeRoutePins(nativeRoutesData, profile)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "classification-root")
	grammarRoot := filepath.Join(t.TempDir(), "prepared", "routes", profile.Route)
	output := filepath.Join(root, "library", profile.Route+".dll")
	args, err := classificationGoCompilerArgs(profile.Route, grammarRoot, output, grammar)
	if err != nil {
		t.Fatal(err)
	}
	plan := classificationPlan{
		Schema: "tsgk-query-classification-plan/r1", SourceRevision: strings.Repeat("a", 40), RunID: "small17-static-plan",
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
	if err := classificationSmall17LibraryRequestValid(profile, plan, root, grammarRoot); err != nil {
		t.Fatal(err)
	}
	queryFilePath := filepath.Join("..", filepath.FromSlash(strings.TrimPrefix(profile.QueryFile, "src/")))
	queryFile := actualJSONRead(t, queryFilePath)
	var factPack []byte
	if profile.Route == "xml" {
		factPack = actualJSONRead(t, "../contracts/fact-query-pack.json")
	}
	inventoryData := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
	qualificationData := actualJSONRead(t, "../contracts/qualification-c1.json")
	inputs, sourceFiles, err := classificationSmall17DefinitionLoadInputs(anchors, plan, inventoryData, qualificationData, queryFile, factPack)
	if err != nil {
		t.Fatal(err)
	}
	return classificationSmall17StaticFixture{profile: profile, anchors: anchors, plan: plan, grammarRoot: grammarRoot, inputs: inputs, sourceFiles: sourceFiles, queryFile: queryFile, factPack: factPack}
}

func classificationSmall17RowsFixture(a classificationAnchors) ([]QueryWitnessQuery, []querywire.Query, []*instrumentedReference) {
	witness := make([]QueryWitnessQuery, len(a.Queries))
	native := make([]querywire.Query, len(a.Queries))
	references := make([]*instrumentedReference, len(a.Queries))
	for i, query := range a.Queries {
		metadata := classificationGoDefinitionCloneMetadata(query.Metadata)
		witness[i] = QueryWitnessQuery{ID: query.ID, Source: query.Source, Metadata: &metadata}
		native[i] = querywire.Query{ID: query.ID, Status: StatusCompleted, Patterns: query.Metadata.Patterns,
			CaptureNames: slices.Clone(query.Metadata.CaptureNames), Predicates: slices.Clone(query.Metadata.Predicates)}
		reference := &instrumentedReference{Status: StatusCompleted}
		reference.Query.ID, reference.Query.SHA256 = query.ID, query.SHA256
		reference.Query.Patterns = query.Metadata.Patterns
		reference.Query.CaptureNames = slices.Clone(query.Metadata.CaptureNames)
		reference.Query.Predicates = slices.Clone(query.Metadata.Predicates)
		references[i] = reference
	}
	return witness, native, references
}

func TestQueryClassificationSmall17ProfilesAndCompilerClosure(t *testing.T) {
	t.Parallel()
	names := []string{"json1-tiny", "yaml1-empty", "xml2-tiny", "html2-empty", "css11-empty"}
	var totalDefinitions, totalPatterns, totalCaptureNames uint64
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			fixture := classificationSmall17StaticFixtureForTest(t, name)
			p, a := fixture.profile, fixture.anchors
			if a.nativeContext() != "full-input-utf8/r2" || a.nativeSymbol() != p.Symbol || a.libraryName() != p.Route+".dll" || classificationNativePrefix(a) != "current/native/" {
				t.Fatal("Small17 context, symbol, library, or capture namespace differs")
			}
			if !classificationPlanContextMatches(a, fixture.plan) {
				t.Fatal("Small17 plan did not independently select its full-input context")
			}
			for _, query := range a.Queries {
				totalDefinitions++
				totalPatterns += uint64(query.Metadata.Patterns)
				totalCaptureNames += uint64(len(query.Metadata.CaptureNames))
			}
			if fixture.inputs.coordinates.Definitions != uint64(p.QueryCount) || fixture.inputs.coordinates.Cases != p.Counts.Cases ||
				fixture.inputs.coordinates.Edits != p.Counts.Edits || fixture.inputs.coordinates.CaseSteps != p.Counts.CaseSteps ||
				fixture.inputs.coordinates.TreeRoles != p.Counts.TreeRoles || fixture.inputs.coordinates.PotentialQueryTreeRoles != p.Counts.PotentialQueryTreeRoles ||
				fixture.inputs.slotGroup.Counts != p.Counts {
				t.Fatalf("%s pinned query-coordinate denominator differs: %#v", p.Name, fixture.inputs.coordinates)
			}
			var wantIncludeDirs, wantSources []string
			switch p.Route {
			case "json":
				wantIncludeDirs, wantSources = []string{"src"}, []string{"src/parser.c"}
			case "yaml":
				wantIncludeDirs, wantSources = []string{"src"}, []string{"src/parser.c", "src/scanner.c"}
			case "xml":
				wantIncludeDirs, wantSources = []string{"xml/src", "common"}, []string{"xml/src/parser.c", "xml/src/scanner.c"}
			case "html", "css":
				wantIncludeDirs, wantSources = []string{"src"}, []string{"src/parser.c", "src/scanner.c"}
			default:
				t.Fatal("unexpected Small17 compiler route", p.Route)
			}
			if !reflect.DeepEqual(p.CompilerIncludeDirs, wantIncludeDirs) || !reflect.DeepEqual(p.CompilerSourceFiles, wantSources) {
				t.Fatalf("%s compiler source layout differs from parser/scanner roles", p.Name)
			}
			wantArgs := []string{"-shared", "-O0", "-std=gnu11"}
			grammarRoot := filepath.Join(t.TempDir(), "prepared", "routes", p.Route)
			output := filepath.Join(t.TempDir(), "library", p.Route+".dll")
			for _, dir := range p.CompilerIncludeDirs {
				wantArgs = append(wantArgs, "-I", filepath.Join(grammarRoot, filepath.FromSlash(dir)))
			}
			for _, source := range p.CompilerSourceFiles {
				wantArgs = append(wantArgs, filepath.Join(grammarRoot, filepath.FromSlash(source)))
			}
			wantArgs = append(wantArgs, "-o", output)
			gotArgs, err := classificationGoCompilerArgs(p.Route, grammarRoot, output, fixture.plan.Grammar)
			if err != nil || !reflect.DeepEqual(gotArgs, wantArgs) {
				t.Fatalf("%s exact compiler argv differs: got=%v want=%v err=%v", p.Name, gotArgs, wantArgs, err)
			}
			badGrammar := slices.Clone(fixture.plan.Grammar)
			badGrammar[0].Path += ".alias"
			if _, err := classificationGoCompilerArgs(p.Route, grammarRoot, output, badGrammar); err == nil {
				t.Fatal("grammar path alias accepted")
			}
			badGrammar = slices.Clone(fixture.plan.Grammar)
			badGrammar[0].Role = "scanner"
			if _, err := classificationGoCompilerArgs(p.Route, grammarRoot, output, badGrammar); err == nil {
				t.Fatal("grammar role alias accepted")
			}
			if p.Route == "yaml" && slices.Contains(gotArgs, filepath.Join(grammarRoot, "src", "schema.core.c")) {
				t.Fatal("schema header was compiled as an extra source input")
			}
			if p.Route == "xml" && (!slices.Contains(gotArgs, filepath.Join(grammarRoot, "xml", "src", "parser.c")) || !slices.Contains(gotArgs, filepath.Join(grammarRoot, "common"))) {
				t.Fatal("XML nested parser/common include closure differs")
			}
			witness, native, references := classificationSmall17RowsFixture(a)
			rows, err := classificationSmall17DefinitionRows(a, fixture.inputs, fixture.sourceFiles, witness, native, references)
			if err != nil || len(rows) != p.QueryCount {
				t.Fatalf("%s predicate-free rows rejected: %v", p.Name, err)
			}
			for i, row := range rows {
				if row.Query != a.Queries[i].ID || row.SourceFile != fixture.sourceFiles[i] || row.Class != "STRUCTURAL_ONLY" || row.Assessment != "NOT_ASSESSED" || row.SemanticRequired != nil || row.SemanticEvaluated || row.PotentialTreeRoles != p.Counts.TreeRoles {
					t.Fatalf("%s row %d promoted or lost its static source binding: %#v", p.Name, i, row)
				}
			}
		})
	}
	if totalDefinitions != 17 || totalPatterns != 44 || totalCaptureNames != 46 {
		t.Fatalf("closed Small17 static metadata denominator changed: definitions=%d patterns=%d capture_names=%d", totalDefinitions, totalPatterns, totalCaptureNames)
	}
}

func TestQueryClassificationDefinitionProfileAdmission(t *testing.T) {
	for _, profile := range []string{"positive-batch", "csharp15-empty", "c4-empty", "java5-empty", "rust4-empty", "cpp4-empty", "json1-tiny", "yaml1-empty", "xml2-tiny", "html2-empty", "css11-empty", "javascript4-empty", "jsx3-empty", "typescript7-empty", "tsx2-empty"} {
		if !classificationGoDefinitionProfileAllowed(profile) {
			t.Fatalf("exact reviewed definition profile %q rejected", profile)
		}
	}
	for _, profile := range []string{"", "json", "json1-tiny-", "json1-tiny ", "yaml1-empty-alias", "xml2-tin", "html2-empty2", "css11", "javascript", "javascript4-empty-alias", "jsx3-empty ", "typescript7-empty2", "tsx2"} {
		if classificationGoDefinitionProfileAllowed(profile) {
			t.Fatalf("unknown or near-alias definition profile %q accepted", profile)
		}
	}
}

func TestQueryClassificationSmall17RejectsAliasAndEmptyEvidence(t *testing.T) {
	t.Parallel()
	fixture := classificationSmall17StaticFixtureForTest(t, "xml2-tiny")
	a := fixture.anchors
	if _, err := classificationSmall17OrderedReferenceIDs(a, map[string]string{"q.structure": "ref-0", "facts.xml_structure": "ref-0"}); err == nil {
		t.Fatal("one reused reference invocation accepted for two XML queries")
	}
	ordered, err := classificationSmall17OrderedReferenceIDs(a, map[string]string{"q.structure": "ref-1", "facts.xml_structure": "ref-0"})
	if err != nil || !reflect.DeepEqual(ordered, []string{"ref-1", "ref-0"}) {
		t.Fatalf("ordered distinct XML reference IDs changed: %v %v", ordered, err)
	}
	for name, mutate := range map[string]func(*classificationAnchors){
		"empty_required":  func(a *classificationAnchors) { a.Required = nil },
		"duplicate_query": func(a *classificationAnchors) { a.Queries[1].ID = a.Queries[0].ID },
		"source_cst":      func(a *classificationAnchors) { a.Nodes[0].Type = "wrong_root" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := a
			bad.Queries = slices.Clone(a.Queries)
			bad.Required = slices.Clone(a.Required)
			bad.Nodes = slices.Clone(a.Nodes)
			mutate(&bad)
			if err := classificationSmall17AnchorsValid(fixture.profile, bad); err == nil {
				t.Fatal("mutated XML source or required-query binding accepted")
			}
		})
	}
	if _, _, err := classificationSmall17DefinitionLoadInputs(a, fixture.plan,
		actualJSONRead(t, "../contracts/query-slot-inventory-r1.json"), actualJSONRead(t, "../contracts/qualification-c1.json"), fixture.queryFile, fixture.queryFile); err == nil {
		t.Fatal("XML raw query file aliased into the pinned fact-pack input")
	}
	if _, _, err := classificationSmall17DefinitionLoadInputs(a, fixture.plan,
		actualJSONRead(t, "../contracts/query-slot-inventory-r1.json"), actualJSONRead(t, "../contracts/qualification-c1.json"), fixture.factPack, fixture.factPack); err == nil {
		t.Fatal("XML fact-pack source aliased into the native query-file input")
	}
	witness, native, references := classificationSmall17RowsFixture(a)
	witness[0].Metadata.Predicates = nil
	if _, err := classificationSmall17DefinitionRows(a, fixture.inputs, fixture.sourceFiles, witness, native, references); err == nil {
		t.Fatal("nil witness predicates accepted as an explicit empty predicate list")
	}
	witness, native, references = classificationSmall17RowsFixture(a)
	native[0].Predicates = []QueryPredicate{{}}
	if _, err := classificationSmall17DefinitionRows(a, fixture.inputs, fixture.sourceFiles, witness, native, references); err == nil {
		t.Fatal("unexpected native predicate accepted")
	}
	witness, native, references = classificationSmall17RowsFixture(a)
	references[1].Query.Predicates = []QueryPredicate{{}}
	if _, err := classificationSmall17DefinitionRows(a, fixture.inputs, fixture.sourceFiles, witness, native, references); err == nil {
		t.Fatal("unexpected reference predicate accepted")
	}
	badPlan := fixture.plan
	badPlan.LibraryRequest.Args = slices.Clone(fixture.plan.LibraryRequest.Args)
	badPlan.LibraryRequest.Args[len(badPlan.LibraryRequest.Args)-1] += ".alias"
	if err := classificationSmall17LibraryRequestValid(fixture.profile, fixture.plan, fixture.plan.LibraryRequest.Dir, fixture.grammarRoot); err != nil {
		t.Fatalf("pinned compiler plan should pass the admission helper before mutation: %v", err)
	}
	if err := classificationSmall17LibraryRequestValid(fixture.profile, badPlan, badPlan.LibraryRequest.Dir, fixture.grammarRoot); err == nil {
		t.Fatal("compiler output alias accepted")
	}
	badCompilerPath := fixture.plan
	badCompilerPath.LibraryRequest.Path = "compiler.exe"
	if err := classificationSmall17LibraryRequestValid(fixture.profile, badCompilerPath, badCompilerPath.LibraryRequest.Dir, fixture.grammarRoot); err == nil {
		t.Fatal("relative compiler path accepted")
	}
	for _, test := range []struct {
		name   string
		mutate func([]string) []string
	}{
		{name: "empty", mutate: func([]string) []string { return []string{} }},
		{name: "unknown-name", mutate: func(env []string) []string { env[len(env)-1] = "ZZZ=C:/Temp"; return env }},
		{name: "missing-PATH", mutate: func(env []string) []string { return append(env[:1], env[2:]...) }},
		{name: "duplicate-name", mutate: func(env []string) []string { env[len(env)-1] = env[len(env)-2]; return env }},
		{name: "case-variant", mutate: func(env []string) []string {
			env[0] = strings.Replace(env[0], "COMSPEC", "ComSpec", 1)
			slices.Sort(env)
			return env
		}},
		{name: "empty-value", mutate: func(env []string) []string { env[1] = "PATH="; return env }},
	} {
		t.Run("compiler-env-"+test.name, func(t *testing.T) {
			badPlan := fixture.plan
			badPlan.LibraryRequest.Env = test.mutate(slices.Clone(fixture.plan.LibraryRequest.Env))
			if err := classificationSmall17LibraryRequestValid(fixture.profile, badPlan, badPlan.LibraryRequest.Dir, fixture.grammarRoot); err == nil {
				t.Fatal("compiler environment outside the explicit nine-name allowlist accepted")
			}
		})
	}
	registry := actualJSONRead(t, "../contracts/native-routes.json")
	registry[0] ^= 1
	if _, err := classificationSmall17NativeRoutePins(registry, fixture.profile); err == nil {
		t.Fatal("native-routes raw source mutation accepted")
	}
}
