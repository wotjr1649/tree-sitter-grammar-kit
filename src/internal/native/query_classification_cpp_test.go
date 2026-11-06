package native

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

type classificationCPPQuerySpec struct {
	id, sha256 string
	patterns   uint32
	captures   []string
}

type classificationCPPGrammarFile struct {
	path, role, sha256 string
	bytes              uint64
}

type classificationCPPProfileSpec struct {
	label, registryLabel                 string
	anchorFile, anchorSHA, route, symbol string
	caseID, workload, rootType           string
	queryFile, querySHA                  string
	queryBytes, grammarCount             int
	languageVersion, runtimeVersion      int
	minCompatible, symbolCount           int
	fieldCount                           int
	factQueries                          int // trailing querySpecs whose source is in the fact query pack, not queryFile
	querySpecs                           []classificationCPPQuerySpec
	grammarFiles                         []classificationCPPGrammarFile
}

// Closed paged-parser profiles share these validators; each name keeps its own pins.
func classificationCPPProfileSpecFor(name string) (classificationCPPProfileSpec, bool) {
	switch name {
	case "cpp4-empty":
		return classificationCPPProfileSpec{
			label: "CPP4", registryLabel: "CPP",
			anchorFile:      "cpp4-classification-anchors.json",
			anchorSHA:       "e807345cb9a528facbaddaae3b914027acc4c99c8148f65f365e83f4b58ee5b4",
			route:           "cpp",
			symbol:          "tree_sitter_cpp",
			caseID:          "cpp4-empty-definitions",
			workload:        "cpp4-empty-definition-single-frame",
			rootType:        "translation_unit",
			queryFile:       "cpp.json",
			queryBytes:      4734,
			querySHA:        "fc90268c86c9133e2801b2fb7a20bda479137aacc0445911fa5046f11fd65766",
			languageVersion: 15, runtimeVersion: 15, minCompatible: 13,
			symbolCount: 621, fieldCount: 53, grammarCount: 5,
			querySpecs: []classificationCPPQuerySpec{
				{id: "q.structure", sha256: "2a57e6e5cb3c6136878e5d27ced7c7a090d491ec2e9e5e3d51f6305c6227945e", patterns: 4, captures: []string{"namespace", "struct", "concept", "class"}},
				{id: "q.probe-cpp-V14", sha256: "1077f4c6e42da4eb404eb4a819b5338f3c2aeeabbfeb1605678e3a4dd3d28f31", patterns: 3, captures: []string{"init", "vartemplate", "num"}},
				{id: "q.probe-cpp-V17", sha256: "29e660f47b5bb900c4321ea914fe6ea74f94d4662912773075e3aac7bf744c49", patterns: 2, captures: []string{"ns", "binding"}},
				{id: "q.probe-cpp-V23", sha256: "fde3be27ac5b9a89fa6e366ed03a24f035e1f21d0ea4434a80e1d666aef3a467", patterns: 2, captures: []string{"self", "index"}},
			},
			grammarFiles: []classificationCPPGrammarFile{
				{path: "src/parser.c", role: "parser", bytes: 49598509, sha256: "3856d51ead4b6ecf5d4e1ec3fd39c52c9ba0cb535e69ea1785822280c45b75fc"},
				{path: "src/scanner.c", role: "scanner", bytes: 6772, sha256: "55b66d60ce5551e354112bc0c050237e786402764d59893d0296ac5f88176546"},
				{path: "src/tree_sitter/alloc.h", role: "header", bytes: 985, sha256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
				{path: "src/tree_sitter/array.h", role: "header", bytes: 13453, sha256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
				{path: "src/tree_sitter/parser.h", role: "header", bytes: 7624, sha256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
			},
		}, true
	case "kotlin5-empty":
		return classificationCPPProfileSpec{
			label: "Kotlin5", registryLabel: "Kotlin5",
			anchorFile:      "kotlin5-classification-anchors.json",
			anchorSHA:       "ffd6d41c2a784900d328e0d6b34c3608ca57262a65f987972c40f889019ce591",
			route:           "kotlin",
			symbol:          "tree_sitter_kotlin",
			caseID:          "kotlin5-empty-definitions",
			workload:        "kotlin5-empty-definition-single-frame",
			rootType:        "source_file",
			queryFile:       "kotlin.json",
			queryBytes:      7135,
			querySHA:        "a32e41d67ec2c3f9b7eb50274a740408edf22d962927fa9735670619b13ced55",
			languageVersion: 15, runtimeVersion: 15, minCompatible: 13,
			symbolCount: 304, fieldCount: 9, grammarCount: 5,
			querySpecs: []classificationCPPQuerySpec{
				{id: "q.structure", sha256: "3948d9b3b28919dc7d78644ed99c383b8a0d0287b499393fe87719c08026a2cc", patterns: 2, captures: []string{"class", "function"}},
				{id: "q.probe-b02", sha256: "22d934886244d3623cabdf879e9dd38c1be8e93496a25a60a4f639bc86ae3104", patterns: 2, captures: []string{"condition", "catch_var"}},
				{id: "q.probe-v14", sha256: "ea6b48fdac43474737477653de739b92c41d28282adf63e711c7763429f8af19", patterns: 4, captures: []string{"class", "object", "dnn", "range"}},
				{id: "q.probe-v22", sha256: "01e3fc670a0c7656a4e5c516e7cf150ce2a2018c2bc7529a9288c436a5d0c916", patterns: 3, captures: []string{"class", "object", "alias"}},
				{id: "q.probe-v24", sha256: "d4f39e572fa9004ae8b3e7f0eb62c68a511e143e013127ca0617076baa99036e", patterns: 3, captures: []string{"class", "function", "property"}},
			},
			grammarFiles: []classificationCPPGrammarFile{
				{path: "src/parser.c", role: "parser", bytes: 33870017, sha256: "c13a776638ead65def356f8feeda4f612121cc96485fb4c78eed80757d906097"},
				{path: "src/scanner.c", role: "scanner", bytes: 25985, sha256: "8b51cea2bd9560201c1d681f56a329f3e637495eac2aaafba3622600c8abdf83"},
				{path: "src/tree_sitter/alloc.h", role: "header", bytes: 985, sha256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
				{path: "src/tree_sitter/array.h", role: "header", bytes: 13453, sha256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
				{path: "src/tree_sitter/parser.h", role: "header", bytes: 7624, sha256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
			},
		}, true
	case "swift8-empty":
		return classificationCPPProfileSpec{
			label: "Swift8", registryLabel: "Swift8",
			anchorFile:      "swift8-classification-anchors.json",
			anchorSHA:       "d5a5ef3d5189a2e7e0e6c2dbb18431ab8a66144e4824392374facf8b480e43b6",
			route:           "swift",
			symbol:          "tree_sitter_swift",
			caseID:          "swift8-empty-definitions",
			workload:        "swift8-empty-definition-single-frame",
			rootType:        "source_file",
			queryFile:       "swift.json",
			queryBytes:      9390,
			querySHA:        "29ae544ed6d729af5abf6d72653ab2df3829eec0cf8dfcfaf1fe8ddfb342c12c",
			languageVersion: 15, runtimeVersion: 15, minCompatible: 13,
			symbolCount: 596, fieldCount: 48, grammarCount: 5,
			querySpecs: []classificationCPPQuerySpec{
				{id: "q.structure", sha256: "6d88d7735d57a8811212c5b2a1530bb38edba699c2410fe88e776df4c5bb1459", patterns: 4, captures: []string{"type", "protocol", "requirement", "function"}},
				{id: "q.probe-b02", sha256: "633bb081a17c16f86332a6342d67f80fa14a1d6800ca6e5f1e118ff094dc490b", patterns: 2, captures: []string{"capture", "keypath"}},
				{id: "q.probe-v51", sha256: "593535bc4f0c3366d2cbfca37ae26365609b21e0f8e0fa51a595e6258d03728e", patterns: 3, captures: []string{"opaque", "raw", "trailing_label"}},
				{id: "q.probe-v55", sha256: "be22a5c1e67ccf88e9c21ff5ec8a9d3e87fede0c98d7e2107ff56724f95618db", patterns: 2, captures: []string{"actor", "existential"}},
				{id: "q.probe-v59", sha256: "3fd8897fa2284fb0e46e430ed8eb25e4db523756ec5dfa49ae53d43a6a0c2088", patterns: 3, captures: []string{"macro", "thrown", "expansion"}},
				{id: "q.probe-v61", sha256: "1053aec87985bcc9739a03cc87e6d5f2d54e9a77432d345d0b2ea401a6ef2018", patterns: 3, captures: []string{"extension", "tparam", "capture"}},
				{id: "q.probe-v62", sha256: "ff3f2f1a325e9f09f80b6ef9a6184e764ddbb132f2eff2708f7ad58eaa7d5a37", patterns: 2, captures: []string{"attr", "unsafe"}},
				{id: "q.probe-v64", sha256: "aa4d6b32c8c84ce015800e49dc64f07132b5fe695ce8096c537112c6eff44745", patterns: 2, captures: []string{"optional", "attr"}},
			},
			grammarFiles: []classificationCPPGrammarFile{
				{path: "src/parser.c", role: "parser", bytes: 30636167, sha256: "512ac3affb50c25a5f7b3106c4c1577aaa28e03e68f268fe4f037759af9e0ae4"},
				{path: "src/scanner.c", role: "scanner", bytes: 39994, sha256: "aee1af6476556021d0ff0cbad064818e21fde836eea3a8b919f31a2a326cdd9d"},
				{path: "src/tree_sitter/alloc.h", role: "header", bytes: 985, sha256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
				{path: "src/tree_sitter/array.h", role: "header", bytes: 13453, sha256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
				{path: "src/tree_sitter/parser.h", role: "header", bytes: 7624, sha256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
			},
		}, true
	case "postgresql9-empty":
		return classificationCPPProfileSpec{
			label: "PostgreSQL9", registryLabel: "PostgreSQL9",
			anchorFile:      "postgresql9-classification-anchors.json",
			anchorSHA:       "9a6f0bd033a757e761d6321e58a9361397b97f3defeee93f844ef1b55ca2b9a6",
			route:           "postgresql-sql",
			symbol:          "tree_sitter_postgres",
			caseID:          "postgresql9-empty-definitions",
			workload:        "postgresql9-empty-definition-single-frame",
			rootType:        "source_file",
			queryFile:       "postgresql-sql.json",
			queryBytes:      8110,
			querySHA:        "2ed90fffb67dda344fd85b6f01c1647d1dad4dce1d6befd2a05e5960676edebd",
			languageVersion: 15, runtimeVersion: 15, minCompatible: 13,
			symbolCount: 1280, fieldCount: 0, grammarCount: 5, factQueries: 1,
			querySpecs: []classificationCPPQuerySpec{
				{id: "q.structure", sha256: "35e55e9d63eafae23dfd6fd23e9f52018574905884d99d78227fa7abad0a0a4e", patterns: 4, captures: []string{"table", "column", "cte", "function"}},
				{id: "q.probe-b01-strings", sha256: "9004103236ba4fdcb76b71442a6c2ba5a3b14ec031416f313633b974e92253fa", patterns: 4, captures: []string{"dollar", "escape", "bit", "hex"}},
				{id: "q.probe-b04-utility", sha256: "b4264eb4db18dd8c652f890dce53b1b07a82b628b80c583b0a73c8b4d7e080c7", patterns: 3, captures: []string{"listen_channel", "notify_channel", "prepared_name"}},
				{id: "q.probe-v10-partition", sha256: "01d1260c7c90dd7c0ee0286f4f6bbb850a84a903314ba94db7207dd10ca54da2", patterns: 2, captures: []string{"partition_spec", "partition_bound"}},
				{id: "q.probe-v13-merge", sha256: "b5e10f700059e1cfc7712a3dd2ca091ea3b76f152dc50d23894015946239c1ad", patterns: 1, captures: []string{"merge_when"}},
				{id: "q.probe-v16-json-table", sha256: "37e8d705152701242987e07600c6aff590be3f2b6ae9b75f907e179999175f8f", patterns: 1, captures: []string{"jt_column"}},
				{id: "q.probe-v18-returning", sha256: "0fbedd7084e35b58bd9f50c787e3015d3049c8b62dc145cdb548ef632199a6f3", patterns: 1, captures: []string{"returning_alias"}},
				{id: "q.probe-l01-oids", sha256: "b72a313c2f8e47b0fa1022d89a26983808000413e8634fb5bffc9143667d334d", patterns: 1, captures: []string{"opt_with"}},
				{id: "facts.declarations", sha256: "1f2284ae901d05903ec6fc8418420a9a459806e2e15ea9d1e45033aeeba528aa", patterns: 16, captures: []string{
					"c.0.0.0", "c.0.1.1", "c.0.1.0", "c.1.0.0", "c.1.1.1", "c.1.1.0", "c.2.0.0", "c.2.1.1", "c.2.1.0", "c.3.0.0", "c.3.1.1", "c.3.1.0",
					"c.4.0.0", "c.4.1.1", "c.4.1.0", "c.5.0.0", "c.5.1.1", "c.5.1.0", "c.6.0.0", "c.6.1.1", "c.6.1.0", "c.7.0.0", "c.7.1.1", "c.7.1.0"}},
			},
			grammarFiles: []classificationCPPGrammarFile{
				{path: "postgres/src/parser.c", role: "parser", bytes: 104285731, sha256: "0db8885c7e1f6c57d3c9dabf066f1397b4d3ec41e934079d02aa74998fc2882e"},
				{path: "postgres/src/scanner.c", role: "scanner", bytes: 4855, sha256: "ccbf351fc4e04def876cd2ae07ff081c860f4c2f53265723855d1c36729849a7"},
				{path: "postgres/src/tree_sitter/alloc.h", role: "header", bytes: 985, sha256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
				{path: "postgres/src/tree_sitter/array.h", role: "header", bytes: 13453, sha256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
				{path: "postgres/src/tree_sitter/parser.h", role: "header", bytes: 7624, sha256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
			},
		}, true
	default:
		return classificationCPPProfileSpec{}, false
	}
}

func classificationCPPAnchorsValid(a classificationLocalAnchors) error {
	return classificationPagedAnchorsValid("cpp4-empty", a)
}

func classificationPagedAnchorsValid(name string, a classificationLocalAnchors) error {
	spec, ok := classificationCPPProfileSpecFor(name)
	if !ok {
		return fmt.Errorf("unknown closed paged profile %q", name)
	}
	if a.Schema != "tsgk-query-classification-anchors/r1" || a.Route != spec.route || a.Case != spec.caseID || a.API ||
		a.Operation != "native-query" || a.DriverMode != "single" || a.Workload != spec.workload ||
		a.Source.UTF8 != "" || a.Source.Bytes != 0 || a.Source.SHA256 != sha(nil) ||
		a.Edits == nil || len(a.Edits) != 0 || len(a.Expect) != 1 || a.Expect[0].Step != 0 || a.Expect[0].Syntax != "NO_ERROR" ||
		len(a.Queries) != len(spec.querySpecs) || len(a.Required) != len(spec.querySpecs) {
		return fmt.Errorf("%s empty-source profile changed", spec.label)
	}
	wantRoot := []kit.TreeNode{{Parent: -1, Type: spec.rootType, Named: true}}
	if !reflect.DeepEqual(a.Nodes, wantRoot) ||
		a.ABI.LanguageVersion != uint32(spec.languageVersion) ||
		a.ABI.RuntimeLanguageVersion != uint32(spec.runtimeVersion) ||
		a.ABI.RuntimeMinCompatible != uint32(spec.minCompatible) ||
		a.ABI.SymbolCount != uint32(spec.symbolCount) ||
		a.ABI.FieldCount != uint32(spec.fieldCount) {
		return fmt.Errorf("%s ABI or independent empty-source CST changed", spec.label)
	}
	seen := make(map[string]bool, len(a.Queries))
	for i, q := range a.Queries {
		want := spec.querySpecs[i]
		if q.ID != want.id || seen[q.ID] || q.SHA256 != want.sha256 || q.SHA256 != sha([]byte(q.Source)) ||
			q.Metadata.Patterns != want.patterns || q.Metadata.CaptureNames == nil ||
			!reflect.DeepEqual(q.Metadata.CaptureNames, want.captures) ||
			q.Metadata.Predicates == nil || len(q.Metadata.Predicates) != 0 {
			return fmt.Errorf("%s query identity or predicate-free metadata changed at %d", spec.label, i)
		}
		seen[q.ID] = true
		tuple := a.Required[i]
		if tuple.Query != q.ID || tuple.Step != 0 || tuple.TreeRole != "incremental" ||
			tuple.Structural == nil || len(tuple.Structural) != 0 ||
			tuple.Expected == nil || len(tuple.Expected) != 0 {
			return fmt.Errorf("%s empty capture tuple changed at %d", spec.label, i)
		}
	}
	return nil
}

func classificationCPPQueryRegistryValid(a classificationLocalAnchors) error {
	return classificationPagedQueryRegistryValid("cpp4-empty", a)
}

func classificationPagedQueryRegistryValid(name string, a classificationLocalAnchors) error {
	spec, ok := classificationCPPProfileSpecFor(name)
	if !ok {
		return fmt.Errorf("unknown closed paged profile %q", name)
	}
	data, err := os.ReadFile("../../testdata/native/queries/" + spec.queryFile)
	if err != nil {
		return err
	}
	if err := classificationPagedQueryRegistryBytesValid(name, a, data); err != nil || spec.factQueries == 0 {
		return err
	}
	factData, err := os.ReadFile(classificationSmall17FactPackPath)
	if err != nil {
		return err
	}
	return classificationPagedFactPackValid(name, a, factData)
}

// classificationPagedFactPackValid binds the trailing fact anchors to the pinned fact query pack.
func classificationPagedFactPackValid(name string, a classificationLocalAnchors, data []byte) error {
	spec, ok := classificationCPPProfileSpecFor(name)
	if !ok || spec.factQueries == 0 {
		return fmt.Errorf("profile %q has no fact query pack", name)
	}
	if len(data) != classificationSmall17FactPackBytes || sha(data) != classificationSmall17FactPackSHA {
		return fmt.Errorf("%s fact query pack pin changed", spec.registryLabel)
	}
	var pack struct {
		Schema string `json:"schema"`
		Routes []struct {
			Route   string `json:"route"`
			Queries []struct {
				ID     string `json:"id"`
				Facts  string `json:"facts"`
				Source string `json:"source"`
			} `json:"queries"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(data, &pack); err != nil {
		return fmt.Errorf("decode %s fact query pack: %w", spec.registryLabel, err)
	}
	if pack.Schema != "tsgk-fact-query-pack/r1" || len(a.Queries) != len(spec.querySpecs) {
		return fmt.Errorf("%s fact query pack identity changed", spec.registryLabel)
	}
	ordinary, routes := len(spec.querySpecs)-spec.factQueries, 0
	for _, route := range pack.Routes {
		if route.Route != spec.route {
			continue
		}
		routes++
		if len(route.Queries) != spec.factQueries {
			return fmt.Errorf("%s fact query route is reordered or incomplete", spec.registryLabel)
		}
		for j, query := range route.Queries {
			want, anchor := spec.querySpecs[ordinary+j], a.Queries[ordinary+j]
			if query.ID != want.id || query.ID != anchor.ID || query.Facts != strings.TrimPrefix(query.ID, "facts.") || query.Source != anchor.Source || sha([]byte(query.Source)) != want.sha256 {
				return fmt.Errorf("%s fact query source/order differs at %d", spec.registryLabel, ordinary+j)
			}
		}
	}
	if routes != 1 {
		return fmt.Errorf("%s fact query route is missing or duplicated", spec.registryLabel)
	}
	return nil
}

func classificationCPPQueryRegistryBytesValid(a classificationLocalAnchors, data []byte) error {
	return classificationPagedQueryRegistryBytesValid("cpp4-empty", a, data)
}

func classificationPagedQueryRegistryBytesValid(name string, a classificationLocalAnchors, data []byte) error {
	spec, ok := classificationCPPProfileSpecFor(name)
	if !ok {
		return fmt.Errorf("unknown closed paged profile %q", name)
	}
	if len(data) != spec.queryBytes || sha(data) != spec.querySHA {
		return fmt.Errorf("%s query registry file pin changed", spec.registryLabel)
	}
	var registry struct {
		Schema  string `json:"schema"`
		Route   string `json:"route"`
		Queries []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"queries"`
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return fmt.Errorf("decode %s query registry: %w", spec.registryLabel, err)
	}
	if registry.Schema != "tsgk-native-query-cases/r1" || registry.Route != spec.route ||
		len(registry.Queries) != len(spec.querySpecs)-spec.factQueries || len(a.Queries) != len(spec.querySpecs) {
		return fmt.Errorf("%s query registry profile changed", spec.registryLabel)
	}
	for i, query := range registry.Queries {
		if query.ID != spec.querySpecs[i].id || query.ID != a.Queries[i].ID ||
			query.Source != a.Queries[i].Source || sha([]byte(query.Source)) != spec.querySpecs[i].sha256 {
			return fmt.Errorf("%s query registry source/order differs at %d", spec.registryLabel, i)
		}
	}
	return nil
}

func classificationCPPGrammarValid(files []kit.NativeInput) bool {
	return classificationPagedGrammarValid("cpp4-empty", files)
}

func classificationPagedGrammarValid(name string, files []kit.NativeInput) bool {
	spec, ok := classificationCPPProfileSpecFor(name)
	if !ok || len(files) != len(spec.grammarFiles) || len(files) != spec.grammarCount {
		return false
	}
	for i, file := range files {
		want := spec.grammarFiles[i]
		if file.Path != want.path || file.Role != want.role || file.Bytes != want.bytes || file.SHA256 != want.sha256 {
			return false
		}
	}
	return true
}

func classificationCPPRouteFiles(t *testing.T, route string) (string, []kit.NativeInput) {
	t.Helper()
	data, err := os.ReadFile("../../contracts/native-routes.json")
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
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	var symbol string
	var files []kit.NativeInput
	matches := 0
	for _, row := range registry.Routes {
		if row.Route == route {
			matches++
			symbol, files = row.Symbol, row.Files
		}
	}
	if matches != 1 {
		t.Fatalf("native route %q missing or duplicated: %d", route, matches)
	}
	return symbol, files
}

func classificationCPPReadAnchors(t *testing.T) ([]byte, classificationLocalAnchors) {
	t.Helper()
	return classificationPagedReadAnchors(t, "cpp4-empty")
}

func classificationPagedReadAnchors(t *testing.T, name string) ([]byte, classificationLocalAnchors) {
	t.Helper()
	spec, ok := classificationCPPProfileSpecFor(name)
	if !ok {
		t.Fatalf("unknown closed paged profile %q", name)
	}
	data, err := os.ReadFile("../../testdata/query-semantics/" + spec.anchorFile)
	if err != nil {
		t.Fatal(err)
	}
	if sha(data) != spec.anchorSHA {
		t.Fatalf("pinned %s anchors changed", spec.label)
	}
	var anchors classificationLocalAnchors
	if err := jsonv2.Unmarshal(data, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	return data, anchors
}

func TestQueryClassificationCPPStaticProfile(t *testing.T) {
	spec, ok := classificationCPPProfileSpecFor("cpp4-empty")
	if !ok {
		t.Fatal("closed CPP4 profile missing")
	}
	profile, err := classificationCaptureProfileFor("cpp4-empty")
	if err != nil || profile.anchorFile != spec.anchorFile || profile.anchorSHA != spec.anchorSHA ||
		profile.route != spec.route || profile.symbol != spec.symbol ||
		profile.queryCount != len(spec.querySpecs) || profile.grammarCount != len(spec.grammarFiles) || !profile.paged {
		t.Fatalf("CPP4 profile is not closed and paged: %#v, %v", profile, err)
	}
	for _, name := range []string{"", "positive-batch", "c4-empty", "java5-empty", "rust4-empty", "csharp15-empty"} {
		other, err := classificationCaptureProfileFor(name)
		wantPaged := name == "csharp15-empty"
		if err != nil || other.paged != wantPaged {
			t.Fatalf("CPP paging changed another profile: %q, %#v, %v", name, other, err)
		}
	}
	if _, err := classificationCaptureProfileFor("unknown-cpp"); err == nil {
		t.Fatal("unknown CPP profile accepted")
	}
	_, anchors := classificationCPPReadAnchors(t)
	if err := classificationCPPAnchorsValid(anchors); err != nil {
		t.Fatal("pinned CPP4 anchor rejected", err)
	}
	queryData, err := os.ReadFile("../../testdata/native/queries/" + spec.queryFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := classificationCPPQueryRegistryBytesValid(anchors, queryData); err != nil {
		t.Fatal("pinned CPP query registry rejected", err)
	}
	if err := classificationCPPQueryRegistryValid(anchors); err != nil {
		t.Fatal("on-disk CPP query registry rejected", err)
	}
	if _, ok := classificationCJRSpecFor("cpp4-empty"); ok || classificationCJRProfile("cpp4-empty") {
		t.Fatal("CPP profile entered the closed C/Java/Rust set")
	}
}

func TestQueryClassificationCPPAnchorGuards(t *testing.T) {
	data, _ := classificationCPPReadAnchors(t)
	clone := func() classificationLocalAnchors {
		t.Helper()
		var anchors classificationLocalAnchors
		if err := jsonv2.Unmarshal(data, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
			t.Fatal(err)
		}
		return anchors
	}
	for _, test := range []struct {
		name   string
		change func(*classificationLocalAnchors)
	}{
		{"route-context", func(a *classificationLocalAnchors) { a.Route = "other" }},
		{"case-workload-mode", func(a *classificationLocalAnchors) { a.Case = "other"; a.Workload = "other"; a.DriverMode = "batch" }},
		{"api-operation", func(a *classificationLocalAnchors) { a.API = true; a.Operation = "native-query-batch" }},
		{"source", func(a *classificationLocalAnchors) {
			a.Source.UTF8 = "x"
			a.Source.Bytes = 1
			a.Source.SHA256 = sha([]byte("x"))
		}},
		{"root", func(a *classificationLocalAnchors) { a.Nodes[0].Type = "other_root" }},
		{"abi", func(a *classificationLocalAnchors) { a.ABI.FieldCount++ }},
		{"query-order", func(a *classificationLocalAnchors) { a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0] }},
		{"query-source", func(a *classificationLocalAnchors) {
			a.Queries[0].Source += "\n"
			a.Queries[0].SHA256 = sha([]byte(a.Queries[0].Source))
		}},
		{"metadata-patterns", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Patterns++ }},
		{"metadata-captures-null", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.CaptureNames = nil }},
		{"metadata-captures-empty", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.CaptureNames = []string{} }},
		{"metadata-predicates-null", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = nil }},
		{"metadata-predicate", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = []kit.QueryPredicate{{}} }},
		{"required-count", func(a *classificationLocalAnchors) { a.Required = a.Required[:3] }},
		{"required-null", func(a *classificationLocalAnchors) { a.Required[0].Expected = nil }},
		{"required-role", func(a *classificationLocalAnchors) { a.Required[0].TreeRole = "fresh" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			anchors := clone()
			test.change(&anchors)
			if err := classificationCPPAnchorsValid(anchors); err == nil {
				t.Fatal("mutated CPP4 context, source, CST, ABI, query metadata, or tuple accepted")
			}
		})
	}
}

func TestQueryClassificationCPPQueryRegistryGuards(t *testing.T) {
	_, anchors := classificationCPPReadAnchors(t)
	data, err := os.ReadFile("../../testdata/native/queries/cpp.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := classificationCPPQueryRegistryBytesValid(anchors, append(append([]byte(nil), data...), ' ')); err == nil {
		t.Fatal("query registry bytes outside the exact pin accepted")
	}
	changed := anchors
	changed.Queries[0].Source += "\n"
	if err := classificationCPPQueryRegistryBytesValid(changed, data); err == nil {
		t.Fatal("anchor query source detached from the pinned registry accepted")
	}
}

func TestQueryClassificationCPPGrammarGuards(t *testing.T) {
	spec, _ := classificationCPPProfileSpecFor("cpp4-empty")
	symbol, files := classificationCPPRouteFiles(t, spec.route)
	if symbol != spec.symbol || !classificationCPPGrammarValid(files) {
		t.Fatal("pinned CPP route grammar closure rejected")
	}
	for _, changed := range [][]kit.NativeInput{
		files[:len(files)-1],
		func() []kit.NativeInput {
			copy := append([]kit.NativeInput(nil), files...)
			copy[1].Role = "header"
			return copy
		}(),
		func() []kit.NativeInput {
			copy := append([]kit.NativeInput(nil), files...)
			copy[0].SHA256 = strings.Repeat("0", 64)
			return copy
		}(),
		func() []kit.NativeInput {
			copy := append([]kit.NativeInput(nil), files...)
			copy[0].Bytes--
			return copy
		}(),
	} {
		if classificationCPPGrammarValid(changed) {
			t.Fatal("incomplete or modified CPP grammar closure accepted")
		}
	}
	if classificationCJRGrammarValid("cpp4-empty", files) {
		t.Fatal("CPP parser escaped the CJR closed-profile rejection")
	}
	c4Symbol, c4Files := classificationCPPRouteFiles(t, "c")
	if c4Symbol != "tree_sitter_c" || !classificationCJRGrammarValid("c4-empty", c4Files) {
		t.Fatal("C route symbol changed")
	}
	c4WithLargeParser := append([]kit.NativeInput(nil), c4Files...)
	c4WithLargeParser[0] = files[0]
	if classificationCJRGrammarValid("c4-empty", c4WithLargeParser) {
		t.Fatal("C4 profile accepted a CPP parser substitution")
	}
	_, anchors := classificationCPPReadAnchors(t)
	if err := classificationCJRAnchorsValid("cpp4-empty", anchors); err == nil {
		t.Fatal("CPP anchors entered the CJR validator")
	}
	if err := classificationCJRQueryRegistryBytesValid("cpp4-empty", anchors, nil); err == nil {
		t.Fatal("CPP query registry entered the CJR validator")
	}
}

func TestQueryClassificationCPPEmptyStderr(t *testing.T) {
	spec, _ := classificationCPPProfileSpecFor("cpp4-empty")
	result := &runner.Result{}
	classificationCSharpEmptyStderr(result)
	capture := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: result}
	files, err := capture.files("capture/" + spec.caseID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range files {
		if file.pin.Role == "stderr" {
			found = file.pin.Bytes == 0 && file.pin.SHA256 == sha(nil) && file.data != nil && len(file.data) == 0
		}
	}
	if !found {
		t.Fatal("zero-byte CPP stderr was omitted from the serialized capture")
	}
	missing := &runner.Result{StderrBytes: 1}
	classificationCSharpEmptyStderr(missing)
	if missing.Stderr != nil {
		t.Fatal("missing nonempty CPP stderr was replaced with an empty member")
	}
}
