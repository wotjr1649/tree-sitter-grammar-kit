package native

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const (
	classificationSmall17NativeRoutesBytes = 118237
	classificationSmall17NativeRoutesSHA   = "7be36efe4cafcefa460c8841c8698a0d055cb25d6f86a5bed8561d601c093ad2"
	classificationSmall17FactPackPath      = "../../contracts/fact-query-pack.json"
	classificationSmall17FactPackBytes     = 9702
	classificationSmall17FactPackSHA       = "f86e4c8072d30480bfdd7a7a77a3c6d92fb52090b910deeb38b9137fa4b4e21a"
)

type classificationSmall17QuerySpec struct {
	id, sha256 string
	patterns   uint32
	captures   []string
}

type classificationSmall17Profile struct {
	name, anchorFile, anchorSHA, queryFile, querySHA string
	route, symbol, caseID                            string
	workload, source, rootType                       string
	sourceBytes                                      uint64
	anchorBytes, queryBytes, nodeCount               int
	languageVersion                                  uint32
	runtimeLanguageVersion                           uint32
	minCompatible                                    uint32
	symbolCount                                      uint32
	fieldCount                                       uint32
	queries                                          []classificationSmall17QuerySpec
}

func classificationSmall17ProfileFor(name string) (classificationSmall17Profile, bool) {
	switch name {
	case "json1-tiny":
		return classificationSmall17Profile{
			name: "json1-tiny", anchorFile: "json1-classification-anchors.json", anchorSHA: "40498d8392c3da1437b88b10e8ee06aaf082e6de35ae4bd14eead1d40a2cc1fd",
			queryFile: "src/testdata/native/queries/json.json", queryBytes: 2321, querySHA: "3b1c4cac01ee08056098e6a5a7fa463158222bc0b3c345155925d1cc1e4373c3",
			route: "json", symbol: "tree_sitter_json", caseID: "json1-tiny-definitions", workload: "json1-tiny-definition-single-frame",
			source: "null", sourceBytes: 4, rootType: "document", nodeCount: 2, anchorBytes: 1443,
			languageVersion: 15, runtimeLanguageVersion: 15, minCompatible: 13, symbolCount: 23, fieldCount: 2,
			queries: []classificationSmall17QuerySpec{{id: "q.structure", sha256: "1862753c69e51b6365315be1d0e788969e3a899a7e05a22cdbbea3e096ff0f41", patterns: 3, captures: []string{"array_key", "object_key", "key", "value"}}},
		}, true
	case "yaml1-empty":
		return classificationSmall17Profile{
			name: "yaml1-empty", anchorFile: "yaml1-classification-anchors.json", anchorSHA: "57c5d0961dc1be0016340dd5b229cf44a748362b1d9089769a3e95e420ad0c1e",
			queryFile: "src/testdata/native/queries/yaml.json", queryBytes: 2426, querySHA: "008d4d04351a7e9a4eb39e38f1820de313fcb2c7f6bc0527caf4efc4695e79b1",
			route: "yaml", symbol: "tree_sitter_yaml", caseID: "yaml1-empty-definitions", workload: "yaml1-empty-definition-single-frame",
			rootType: "stream", nodeCount: 1, anchorBytes: 1273,
			languageVersion: 15, runtimeLanguageVersion: 15, minCompatible: 13, symbolCount: 301, fieldCount: 2,
			queries: []classificationSmall17QuerySpec{{id: "q.structure", sha256: "7f2d2c9782a06629b02bd0213e13de72334ced48e686fe2ee9637a009d7689f1", patterns: 4, captures: []string{"block_key", "anchor", "alias", "flow_key"}}},
		}, true
	case "xml2-tiny":
		return classificationSmall17Profile{
			name: "xml2-tiny", anchorFile: "xml2-classification-anchors.json", anchorSHA: "ef7ea68d923b5a551fe2805184cfd949aca2cf57254679844f090de1234f238f",
			queryFile: "src/testdata/native/queries/xml.json", queryBytes: 2409, querySHA: "32e54336d7a2189cbf497644717ccee3d6ccfb2d35db2686c6c4b21ef0f12847",
			route: "xml", symbol: "tree_sitter_xml", caseID: "xml2-tiny-definitions", workload: "xml2-tiny-definition-single-frame",
			source: "<r/>", sourceBytes: 4, rootType: "document", nodeCount: 6, anchorBytes: 3863,
			languageVersion: 15, runtimeLanguageVersion: 15, minCompatible: 13, symbolCount: 142, fieldCount: 2,
			queries: []classificationSmall17QuerySpec{
				{id: "q.structure", sha256: "bc3851a4b2a54ae9bfdb6b4f910479c955cae7cf38f31722f7ab1abff16ff227", patterns: 4, captures: []string{"entity", "root", "child_tag", "attribute"}},
				{id: "facts.xml_structure", sha256: "5aa16c3d6107e93005486a5e720fb06c2fef9c279fdcd85267942cdc14de19d6", patterns: 3, captures: []string{"element.name", "element", "attribute.name", "attribute.value", "attribute"}},
			},
		}, true
	case "html2-empty":
		return classificationSmall17Profile{
			name: "html2-empty", anchorFile: "html2-classification-anchors.json", anchorSHA: "e61754ca5f6873df5e0be601171b75b012d607f4e28e71a4d1f213e29c7685d7",
			queryFile: "src/testdata/native/queries/html.json", queryBytes: 5003, querySHA: "dca8b1397376a0f013d8180b439d1c5c0cc639971b53c9abd0c4769d0bb59955",
			route: "html", symbol: "tree_sitter_html", caseID: "html2-empty-definitions", workload: "html2-empty-definition-single-frame",
			rootType: "document", nodeCount: 1, anchorBytes: 1727,
			languageVersion: 15, runtimeLanguageVersion: 15, minCompatible: 13, symbolCount: 48, fieldCount: 0,
			queries: []classificationSmall17QuerySpec{
				{id: "q.structure", sha256: "a2ec6d274471dae0d094c63445488f0463f0f7593fe82020f909f40aabbc5a18", patterns: 4, captures: []string{"attribute", "raw_text_tag", "self_closing"}},
				{id: "q.source", sha256: "a3502f6c81bdbccb69b3f08a337201d23eb87539f3cba989bd53a8bb1075cd20", patterns: 4, captures: []string{"tag", "attribute", "unquoted", "quoted"}},
			},
		}, true
	case "css11-empty":
		return classificationSmall17Profile{
			name: "css11-empty", anchorFile: "css11-classification-anchors.json", anchorSHA: "aa271590746efbf2ade6b36d0ae4cc4b555dbfeeb558e86358a10c8cc20770b5",
			queryFile: "src/testdata/native/queries/css.json", queryBytes: 36030, querySHA: "92de676cf8ebf4234ee97dcebe531edd4f5cbc569c50e4dc2fe6d51d20e4bb21",
			route: "css", symbol: "tree_sitter_css", caseID: "css11-empty-definitions", workload: "css11-empty-definition-single-frame",
			rootType: "stylesheet", nodeCount: 1, anchorBytes: 4945,
			languageVersion: 15, runtimeLanguageVersion: 15, minCompatible: 13, symbolCount: 204, fieldCount: 0,
			queries: []classificationSmall17QuerySpec{
				{id: "q.structure", sha256: "aa9dedcc031ca2bf19d116d552647078ebb0b864bf7f176729662c58fc9d874d", patterns: 4, captures: []string{"namespace", "property", "feature", "keyframes"}},
				{id: "q.property", sha256: "c13e3ce0363023caa52de753923408d6916f25e8d870dcb7dfc6d84b00523c56", patterns: 1, captures: []string{"property"}},
				{id: "q.value", sha256: "51fdcb6dd0235bf191f1c54bbc836aeab687ba0167fdfd39016eee1a7ddf48c3", patterns: 1, captures: []string{"value"}},
				{id: "q.call", sha256: "793d2aeb5b2600e6c4188b7d6e65b9a07fd7cc388dc6203c0129465af160e1b3", patterns: 2, captures: []string{"function", "arg"}},
				{id: "q.selectors", sha256: "08c73efedba5fd7eb31c88d0819d5b7d99ffec0765e3f653b013f69ae40c461a", patterns: 4, captures: []string{"class", "attribute", "pseudo_class", "pseudo_element"}},
				{id: "q.import", sha256: "b107565df1c734022b1007cfb48d814addb229381ec96641acfadbdb0ec3f077", patterns: 2, captures: []string{"function", "media_type"}},
				{id: "q.calc", sha256: "6b01dffaa02b5e6785dd84e8a02716ffc178cad130db72535555caee371f70b1", patterns: 1, captures: []string{"operand"}},
				{id: "q.at", sha256: "523ef692141f6cbd74188feee0a0852b178341a91fbf1be1d9d72f4c187efe5e", patterns: 2, captures: []string{"at_keyword", "prelude"}},
				{id: "q.media", sha256: "55bed83b0e371591bb6a02d1262d7f9ed04801b6984c557e8f64174adfa3fd7c", patterns: 2, captures: []string{"condition", "feature"}},
				{id: "q.grid", sha256: "d9aab723cca20750c5f32ac7a7ce95b78f941e4664194271cca704355f877902", patterns: 2, captures: []string{"line_names", "function"}},
				{id: "q.supports", sha256: "429b5b96421b1ad469caf87a89c7301ac41b9d3e800d1a947ac2be6ff12d6509", patterns: 1, captures: []string{"selector"}},
			},
		}, true
	default:
		return classificationSmall17Profile{}, false
	}
}

func classificationSmall17GrammarPins(route string) ([]kit.NativeInput, bool) {
	shared := []kit.NativeInput{
		{Path: "src/tree_sitter/alloc.h", Role: "header", Bytes: 985, SHA256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
		{Path: "src/tree_sitter/array.h", Role: "header", Bytes: 13453, SHA256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
		{Path: "src/tree_sitter/parser.h", Role: "header", Bytes: 7624, SHA256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
	}
	switch route {
	case "json":
		return append([]kit.NativeInput{{Path: "src/parser.c", Role: "parser", Bytes: 23126, SHA256: "86971e7cb4e6dae9c5395bedf51abf525b2b662c6bcb632d33b72e4519aaab83"}}, shared...), true
	case "yaml":
		files := []kit.NativeInput{
			{Path: "src/parser.c", Role: "parser", Bytes: 1289038, SHA256: "02dc8c5f297936b198d1371874b8f5226f8dd35b75f6a82894d12a2cbc219eba"},
			{Path: "src/scanner.c", Role: "scanner", Bytes: 52836, SHA256: "548ec6777a474b9d9b6d61779e8da4977863535a264260499be056eb36a81402"},
			{Path: "src/schema.core.c", Role: "header", Bytes: 6630, SHA256: "91e56c3f3ae6fad1803b739ce4eb4568a782a9aec710f733db2f3e38d407a4d5"},
			{Path: "src/schema.json.c", Role: "header", Bytes: 3065, SHA256: "3a18e4406aa8cdaea2806483e11c50e1a392fd72cf8f54ab7f189b3728836949"},
			{Path: "src/schema.legacy.c", Role: "header", Bytes: 18624, SHA256: "4f7ca12361e848214beceaa95ae615048317039ea4736c6eec956e3e8da4694c"},
		}
		return append(files, shared...), true
	case "xml":
		xmlShared := make([]kit.NativeInput, len(shared))
		for i, file := range shared {
			file.Path = "xml/" + file.Path
			xmlShared[i] = file
		}
		return append([]kit.NativeInput{
			{Path: "common/scanner.h", Role: "header", Bytes: 5012, SHA256: "55023e3643dc84b54ff52a967f0a5f15152d8e10c880c12e94a94ca9148693d3"},
			{Path: "xml/src/parser.c", Role: "parser", Bytes: 231195, SHA256: "129f6c0d2f4c422098e83f93c75f4abe496a5dc74a6b7ddeaca7e06b33ae39e2"},
			{Path: "xml/src/scanner.c", Role: "scanner", Bytes: 8457, SHA256: "8f9bede7fa8a2936012844f584176c68cdf2733d85d2d64c1564648b28fba736"},
		}, xmlShared...), true
	case "html":
		return append([]kit.NativeInput{
			{Path: "src/parser.c", Role: "parser", Bytes: 80228, SHA256: "317e913c913010533392e292bae2c18e5948c09ecfb13c0d2e566809cb905f39"},
			{Path: "src/scanner.c", Role: "scanner", Bytes: 23782, SHA256: "f72e0d923bf00dd8d871872f16a5db4eaeefeb84b2fd1f6901a3c0015922977a"},
			{Path: "src/tag.h", Role: "header", Bytes: 9180, SHA256: "9a0aabd64e992987f3cfa87c342e246544135d9c6159536506268ece38f95b9b"},
		}, shared...), true
	case "css":
		return append([]kit.NativeInput{
			{Path: "src/parser.c", Role: "parser", Bytes: 970839, SHA256: "9010ec18975e886f2c274139a58fb3c2ae0f39aec02bb9d754857b089f44a2b0"},
			{Path: "src/scanner.c", Role: "scanner", Bytes: 8438, SHA256: "e7f2c01186235209f16f83624a44b154ca62ce22e2f9de3276250b321b67cbdd"},
		}, shared...), true
	default:
		return nil, false
	}
}

func classificationSmall17GrammarValid(profileName string, registryData []byte, route, symbol string, grammar []kit.NativeInput) error {
	profile, ok := classificationSmall17ProfileFor(profileName)
	want, knownRoute := classificationSmall17GrammarPins(profile.route)
	if !ok || !knownRoute || len(registryData) != classificationSmall17NativeRoutesBytes || sha(registryData) != classificationSmall17NativeRoutesSHA ||
		route != profile.route || symbol != profile.symbol || len(grammar) != len(want) || !reflect.DeepEqual(grammar, want) {
		return fmt.Errorf("%s exact registered native grammar closure changed", profileName)
	}
	return nil
}

func classificationSmall17AnchorsValid(profileName string, a classificationLocalAnchors) error {
	profile, ok := classificationSmall17ProfileFor(profileName)
	if !ok {
		return fmt.Errorf("unknown closed Small17 profile %q", profileName)
	}
	if a.Schema != "tsgk-query-classification-anchors/r1" || a.Route != profile.route || a.Case != profile.caseID ||
		a.Workload != profile.workload || a.Operation != "native-query" || a.DriverMode != "single" || a.API ||
		a.Source.UTF8 != profile.source || a.Source.Bytes != profile.sourceBytes || a.Source.SHA256 != sha([]byte(profile.source)) ||
		a.Edits == nil || len(a.Edits) != 0 || len(a.Nodes) != profile.nodeCount || len(a.Expect) != 1 ||
		!reflect.DeepEqual(a.Expect, []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR"}}) ||
		len(a.Queries) != len(profile.queries) || len(a.Required) != len(profile.queries) {
		return fmt.Errorf("%s anchor identity, source, or denominator changed", profileName)
	}
	if !reflect.DeepEqual(a.Nodes, classificationSmall17ExpectedNodes(profile)) ||
		a.ABI.LanguageVersion != profile.languageVersion || a.ABI.RuntimeLanguageVersion != profile.runtimeLanguageVersion ||
		a.ABI.RuntimeMinCompatible != profile.minCompatible || a.ABI.SymbolCount != profile.symbolCount || a.ABI.FieldCount != profile.fieldCount {
		return fmt.Errorf("%s independent source CST or expected producer ABI changed", profileName)
	}
	seen := make(map[string]bool, len(profile.queries))
	for i, query := range a.Queries {
		want := profile.queries[i]
		if query.ID != want.id || seen[query.ID] || query.SHA256 != want.sha256 || query.SHA256 != sha([]byte(query.Source)) ||
			query.Metadata.Patterns != want.patterns || query.Metadata.CaptureNames == nil || !reflect.DeepEqual(query.Metadata.CaptureNames, want.captures) ||
			query.Metadata.Predicates == nil || len(query.Metadata.Predicates) != 0 {
			return fmt.Errorf("%s query identity, source order, or complete predicate-free metadata changed at %d", profileName, i)
		}
		seen[query.ID] = true
		tuple := a.Required[i]
		wantCaptures := []kit.Capture{}
		if profileName == "xml2-tiny" && query.ID == "facts.xml_structure" {
			wantCaptures = classificationSmall17XMLFactCaptures()
		}
		if tuple.Query != query.ID || tuple.Step != 0 || tuple.TreeRole != "incremental" || tuple.Structural == nil ||
			tuple.Expected == nil || !reflect.DeepEqual(tuple.Structural, wantCaptures) || !reflect.DeepEqual(tuple.Expected, wantCaptures) {
			return fmt.Errorf("%s required structural tuple changed at %d", profileName, i)
		}
	}
	return nil
}

func classificationSmall17ExpectedNodes(profile classificationSmall17Profile) []kit.TreeNode {
	node := func(parent int64, typeName string, field *string, named bool, start, end uint32) kit.TreeNode {
		return kit.TreeNode{
			Parent: parent, Type: typeName, Field: field, Named: named, StartByte: start, EndByte: end,
			StartPoint: kit.Point{Column: start}, EndPoint: kit.Point{Column: end},
		}
	}
	if profile.name == "json1-tiny" {
		return []kit.TreeNode{node(-1, "document", nil, true, 0, 4), node(0, "null", nil, true, 0, 4)}
	}
	if profile.name == "xml2-tiny" {
		root := "root"
		return []kit.TreeNode{
			node(-1, "document", nil, true, 0, 4),
			node(0, "element", &root, true, 0, 4),
			node(1, "EmptyElemTag", nil, true, 0, 4),
			node(2, "<", nil, false, 0, 1),
			node(2, "Name", nil, true, 1, 2),
			node(2, "/>", nil, false, 2, 4),
		}
	}
	return []kit.TreeNode{node(-1, profile.rootType, nil, true, 0, 0)}
}

func classificationSmall17XMLFactCaptures() []kit.Capture {
	return []kit.Capture{
		{Match: 0, Pattern: 1, Capture: 1, Name: "element", Node: 2, Type: "EmptyElemTag", Named: true, StartByte: 0, EndByte: 4, StartPoint: kit.Point{}, EndPoint: kit.Point{Column: 4}},
		{Match: 0, Pattern: 1, Capture: 0, Name: "element.name", Node: 4, Type: "Name", Named: true, StartByte: 1, EndByte: 2, StartPoint: kit.Point{Column: 1}, EndPoint: kit.Point{Column: 2}},
	}
}

func classificationSmall17QuerySourcesValid(profileName string, a classificationLocalAnchors) error {
	profile, ok := classificationSmall17ProfileFor(profileName)
	if !ok {
		return fmt.Errorf("unknown closed Small17 profile %q", profileName)
	}
	queryPath := filepath.Join("..", "..", filepath.FromSlash(strings.TrimPrefix(profile.queryFile, "src/")))
	queryData, err := os.ReadFile(queryPath)
	if err != nil {
		return err
	}
	var factData []byte
	if profileName == "xml2-tiny" {
		factData, err = os.ReadFile(classificationSmall17FactPackPath)
		if err != nil {
			return err
		}
	}
	return classificationSmall17QuerySourceDataValid(profile, a, queryData, factData)
}

func classificationSmall17QuerySourceDataValid(profile classificationSmall17Profile, a classificationLocalAnchors, queryData, factData []byte) error {
	if len(queryData) != profile.queryBytes || sha(queryData) != profile.querySHA || len(a.Queries) != len(profile.queries) {
		return fmt.Errorf("%s ordinary query source pin changed", profile.name)
	}
	var queryFile struct {
		Schema  string `json:"schema"`
		Route   string `json:"route"`
		Queries []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"queries"`
	}
	if err := json.Unmarshal(queryData, &queryFile); err != nil {
		return fmt.Errorf("%s ordinary query source: %w", profile.name, err)
	}
	if queryFile.Schema != "tsgk-native-query-cases/r1" || queryFile.Route != profile.route {
		return fmt.Errorf("%s ordinary query source route changed", profile.name)
	}
	lastIndex := -1
	for _, anchor := range a.Queries {
		if profile.name == "xml2-tiny" && anchor.ID == "facts.xml_structure" {
			continue
		}
		matches := 0
		for index, query := range queryFile.Queries {
			if query.ID != anchor.ID {
				continue
			}
			matches++
			if query.Source != anchor.Source || index <= lastIndex {
				return fmt.Errorf("%s ordinary query %s source or order differs", profile.name, anchor.ID)
			}
			lastIndex = index
		}
		if matches != 1 {
			return fmt.Errorf("%s ordinary query %s is missing or duplicated", profile.name, anchor.ID)
		}
	}
	if profile.name != "xml2-tiny" {
		if factData != nil {
			return fmt.Errorf("%s has an unexpected fact query source", profile.name)
		}
		return nil
	}
	if len(factData) != classificationSmall17FactPackBytes || sha(factData) != classificationSmall17FactPackSHA {
		return fmt.Errorf("XML fact query source pin changed")
	}
	var factPack struct {
		Schema       string `json:"schema"`
		XMLStructure string `json:"xml_structure"`
		Routes       []struct {
			Route   string `json:"route"`
			Queries []struct {
				ID     string `json:"id"`
				Facts  string `json:"facts"`
				Source string `json:"source"`
			} `json:"queries"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(factData, &factPack); err != nil {
		return fmt.Errorf("XML fact query pack source: %w", err)
	}
	if factPack.Schema != "tsgk-fact-query-pack/r1" || factPack.XMLStructure != "xml-structure-r1" {
		return fmt.Errorf("XML fact query pack identity changed")
	}
	xmlRoutes := 0
	for _, route := range factPack.Routes {
		if route.Route != "xml" {
			continue
		}
		xmlRoutes++
		if len(route.Queries) != 1 || route.Queries[0].ID != "facts.xml_structure" || route.Queries[0].Facts != "xml_structure" ||
			route.Queries[0].ID != a.Queries[1].ID || route.Queries[0].Source != a.Queries[1].Source {
			return fmt.Errorf("facts.xml_structure source binding changed")
		}
	}
	if xmlRoutes != 1 {
		return fmt.Errorf("XML fact route is missing or duplicated")
	}
	return nil
}

func TestQueryClassificationSmall17ProfilesAndGrammarPins(t *testing.T) {
	registryData, err := os.ReadFile("../../contracts/native-routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Routes []struct {
			Route  string            `json:"route"`
			Symbol string            `json:"symbol"`
			Files  []kit.NativeInput `json:"files"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(registryData, &registry); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"json1-tiny", "yaml1-empty", "xml2-tiny", "html2-empty", "css11-empty"} {
		t.Run(name, func(t *testing.T) {
			profile, ok := classificationSmall17ProfileFor(name)
			if !ok {
				t.Fatal("closed Small17 profile missing")
			}
			wantGrammar, ok := classificationSmall17GrammarPins(profile.route)
			if !ok {
				t.Fatal("closed Small17 grammar pins missing")
			}
			got, err := classificationCaptureProfileFor(name)
			wantCapture := classificationCaptureProfile{profile.anchorFile, profile.anchorSHA, profile.route, profile.symbol, len(profile.queries), len(wantGrammar), false}
			if err != nil || got != wantCapture {
				t.Fatalf("local capture profile differs: %#v, %v", got, err)
			}
			if profile.queryFile != "src/testdata/native/queries/"+profile.route+".json" {
				t.Fatal("ordinary query source path differs")
			}
			anchorData, err := os.ReadFile("../../testdata/query-semantics/" + profile.anchorFile)
			if err != nil || len(anchorData) != profile.anchorBytes || sha(anchorData) != profile.anchorSHA {
				t.Fatal("static expected anchor pin changed", err)
			}
			var anchors classificationLocalAnchors
			if err := jsonv2.Unmarshal(anchorData, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
				t.Fatal(err)
			}
			if err := classificationSmall17AnchorsValid(name, anchors); err != nil {
				t.Fatal(err)
			}
			if err := classificationSmall17QuerySourcesValid(name, anchors); err != nil {
				t.Fatal(err)
			}
			var grammar []kit.NativeInput
			var symbol string
			var routeMatches int
			for _, row := range registry.Routes {
				if row.Route == profile.route {
					routeMatches++
					grammar, symbol = row.Files, row.Symbol
				}
			}
			if routeMatches != 1 {
				t.Fatalf("native route count differs: %d", routeMatches)
			}
			if err := classificationSmall17GrammarValid(name, registryData, profile.route, symbol, grammar); err != nil {
				t.Fatal(err)
			}
			for _, mutate := range []struct {
				name   string
				change func([]kit.NativeInput) []kit.NativeInput
			}{
				{"path", func(f []kit.NativeInput) []kit.NativeInput { f[0].Path += ".changed"; return f }},
				{"role", func(f []kit.NativeInput) []kit.NativeInput { f[0].Role = "wrong"; return f }},
				{"bytes", func(f []kit.NativeInput) []kit.NativeInput { f[0].Bytes++; return f }},
				{"sha", func(f []kit.NativeInput) []kit.NativeInput { f[0].SHA256 = strings.Repeat("0", 64); return f }},
			} {
				t.Run("grammar-"+mutate.name, func(t *testing.T) {
					changed := append([]kit.NativeInput(nil), grammar...)
					changed = mutate.change(changed)
					if err := classificationSmall17GrammarValid(name, registryData, profile.route, symbol, changed); err == nil {
						t.Fatal("changed grammar closure accepted")
					}
				})
			}
			if err := classificationSmall17GrammarValid(name, append(registryData, ' '), profile.route, symbol, grammar); err == nil {
				t.Fatal("changed native-routes source accepted")
			}
		})
	}
	if _, err := classificationCaptureProfileFor("small17-unknown"); err == nil {
		t.Fatal("unknown Small17 profile accepted")
	}
	for _, name := range []string{"json1-tiny", "yaml1-empty", "xml2-tiny", "html2-empty", "css11-empty"} {
		if _, ok := classificationCJRSpecFor(name); ok {
			t.Fatalf("Small17 profile entered the closed C/Java/Rust set: %s", name)
		}
	}
}

func TestQueryClassificationSmall17AnchorAndQueryBindingGuards(t *testing.T) {
	for _, name := range []string{"json1-tiny", "yaml1-empty", "xml2-tiny", "html2-empty", "css11-empty"} {
		t.Run(name, func(t *testing.T) {
			profile, _ := classificationSmall17ProfileFor(name)
			data, err := os.ReadFile("../../testdata/query-semantics/" + profile.anchorFile)
			if err != nil {
				t.Fatal(err)
			}
			queryPath := filepath.Join("..", "..", filepath.FromSlash(strings.TrimPrefix(profile.queryFile, "src/")))
			queryData, err := os.ReadFile(queryPath)
			if err != nil {
				t.Fatal(err)
			}
			var factData []byte
			if name == "xml2-tiny" {
				factData, err = os.ReadFile(classificationSmall17FactPackPath)
				if err != nil {
					t.Fatal(err)
				}
			}
			var base classificationLocalAnchors
			if err := jsonv2.Unmarshal(data, &base, jsonv2.RejectUnknownMembers(true)); err != nil {
				t.Fatal(err)
			}
			for _, mutate := range []struct {
				name   string
				change func(*classificationLocalAnchors)
			}{
				{"source", func(a *classificationLocalAnchors) { a.Source.UTF8 += " "; a.Source.Bytes++ }},
				{"root", func(a *classificationLocalAnchors) { a.Nodes[0].Type = "wrong_root" }},
				{"abi", func(a *classificationLocalAnchors) { a.ABI.FieldCount++ }},
				{"query-order", func(a *classificationLocalAnchors) {
					if len(a.Queries) > 1 {
						a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0]
					} else {
						a.Queries[0].ID = "wrong-order"
					}
				}},
				{"metadata-capture-nil", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.CaptureNames = nil }},
				{"metadata-predicate-nil", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = nil }},
				{"tuple-query", func(a *classificationLocalAnchors) { a.Required[0].Query = "wrong-query" }},
				{"tuple-step", func(a *classificationLocalAnchors) { a.Required[0].Step++ }},
				{"tuple-structural-nil", func(a *classificationLocalAnchors) { a.Required[0].Structural = nil }},
				{"tuple-expected-nil", func(a *classificationLocalAnchors) { a.Required[0].Expected = nil }},
			} {
				t.Run(mutate.name, func(t *testing.T) {
					changed := base
					changed.Queries = append(changed.Queries[:0:0], base.Queries...)
					changed.Required = append(changed.Required[:0:0], base.Required...)
					changed.Nodes = append(changed.Nodes[:0:0], base.Nodes...)
					mutate.change(&changed)
					if err := classificationSmall17AnchorsValid(name, changed); err == nil {
						t.Fatal("changed source, CST, ABI, query metadata, order, or tuple accepted")
					}
				})
			}
			changedQuerySource := base
			changedQuerySource.Queries = append(changedQuerySource.Queries[:0:0], base.Queries...)
			changedQuerySource.Queries[0].Source += " "
			if err := classificationSmall17QuerySourceDataValid(profile, changedQuerySource, queryData, factData); err == nil {
				t.Fatal("anchor query not bound to its pinned source file")
			}
			if err := classificationSmall17QuerySourceDataValid(profile, base, append(queryData, ' '), factData); err == nil {
				t.Fatal("changed ordinary query file accepted")
			}
			if name == "xml2-tiny" {
				if err := classificationSmall17QuerySourceDataValid(profile, base, queryData, append(factData, ' ')); err == nil {
					t.Fatal("changed XML fact pack accepted")
				}
				changedFact := base
				changedFact.Queries = append(changedFact.Queries[:0:0], base.Queries...)
				changedFact.Queries[1].Source = changedFact.Queries[0].Source
				if err := classificationSmall17QuerySourceDataValid(profile, changedFact, queryData, factData); err == nil {
					t.Fatal("ordinary XML query accepted as the fact query")
				}
			}
		})
	}
}
