package native

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const (
	classificationWeb16NativeRoutesBytes = 118237
	classificationWeb16NativeRoutesSHA   = "7be36efe4cafcefa460c8841c8698a0d055cb25d6f86a5bed8561d601c093ad2"
)

type classificationWeb16QuerySpec struct {
	id, sha256 string
	patterns   uint32
	captures   []string
}

type classificationWeb16Profile struct {
	name, anchorFile, anchorSHA, route, symbol, caseID, workload string
	queryFile, querySHA                                          string
	anchorBytes, queryBytes, grammarCount                        int
	languageVersion, runtimeLanguageVersion, minCompatible       uint32
	symbolCount, fieldCount                                      uint32
	queries                                                      []classificationWeb16QuerySpec
}

func classificationWeb16ProfileFor(name string) (classificationWeb16Profile, bool) {
	abi := func(symbols, fields uint32) (uint32, uint32, uint32, uint32, uint32) {
		return 15, 15, 13, symbols, fields
	}
	switch name {
	case "javascript4-empty":
		language, runtime, compatible, symbols, fields := abi(291, 36)
		return classificationWeb16Profile{
			name: "javascript4-empty", anchorFile: "javascript4-classification-anchors.json", anchorSHA: "b49741060ebc83441982a46b5b4187ed00fdb836d472d8e89ae9a2d44c301c33",
			route: "javascript", symbol: "tree_sitter_javascript", caseID: "javascript4-empty-definitions", workload: "javascript4-empty-definition-single-frame",
			queryFile: "src/testdata/native/queries/javascript.json", queryBytes: 6270, querySHA: "f33a5feb0eae85a5f467d36b9cb42084799dc4a5b2d67b8dcfd9d267b8f5e505",
			anchorBytes: 2914, grammarCount: 5, languageVersion: language, runtimeLanguageVersion: runtime, minCompatible: compatible, symbolCount: symbols, fieldCount: fields,
			queries: []classificationWeb16QuerySpec{
				{id: "q.structure", sha256: "d5c47480683d350ed43669b6cd07d1aec567d585093237d5f96c1f405a35a2bd", patterns: 4, captures: []string{"source", "function", "class", "method"}},
				{id: "q.probe-v16", sha256: "68b529ff3343a0b6d713e6a37c50a335fb7e6bb46e51576dcd6d71d5aac4aa48", patterns: 5, captures: []string{"rest", "await", "base", "exponent", "catch_body", "spread"}},
				{id: "q.probe-v20", sha256: "f29a1a2a4be3ea9faff06653dca56216af779280e97939cf7a183ee2d8a75791", patterns: 4, captures: []string{"fallback", "big_name", "big", "target", "value", "meta"}},
				{id: "q.probe-v22", sha256: "1411fa645026d8454339cd78d8d171d7cc1ffebee4a6d38d65eab314e788abe8", patterns: 4, captures: []string{"attributes", "private_field", "public_field", "static_block"}},
			},
		}, true
	case "jsx3-empty":
		language, runtime, compatible, symbols, fields := abi(291, 36)
		return classificationWeb16Profile{
			name: "jsx3-empty", anchorFile: "jsx3-classification-anchors.json", anchorSHA: "dfee37f1794e2128a46d01c3b5d8b4fd78809a3a4d8d9cfa92f45b19f1a8a979",
			route: "jsx", symbol: "tree_sitter_javascript", caseID: "jsx3-empty-definitions", workload: "jsx3-empty-definition-single-frame",
			queryFile: "src/testdata/native/queries/jsx.json", queryBytes: 4366, querySHA: "036457f6bba87231d75fb55d768adef9ab8f03c8e2d43231da093aa18f8ffb62",
			anchorBytes: 2024, grammarCount: 5, languageVersion: language, runtimeLanguageVersion: runtime, minCompatible: compatible, symbolCount: symbols, fieldCount: fields,
			queries: []classificationWeb16QuerySpec{
				{id: "q.structure", sha256: "8704313cbd2b50ca5899acab82b2da3a193a0ba12c4efca916cb2581650f5416", patterns: 3, captures: []string{"open", "close", "self_closing"}},
				{id: "q.probe-b02", sha256: "99c375a77d619da22005303b03932f8e01007c4feb464756dbd8ea1de7c3c55d", patterns: 2, captures: []string{"attr_name", "spread_attr"}},
				{id: "q.probe-b03", sha256: "b4d71bd30038f1ca36248ea80ebb0d6947d8a9625ed62b52966ff59c93f5d4fd", patterns: 2, captures: []string{"spread_child", "entity"}},
			},
		}, true
	case "typescript7-empty":
		language, runtime, compatible, symbols, fields := abi(388, 40)
		return classificationWeb16Profile{
			name: "typescript7-empty", anchorFile: "typescript7-classification-anchors.json", anchorSHA: "1f658bca74082dd8cfd387eea2c73777e00b2128d1f58a9e690c1b40d83e31e1",
			route: "typescript", symbol: "tree_sitter_typescript", caseID: "typescript7-empty-definitions", workload: "typescript7-empty-definition-single-frame",
			queryFile: "src/testdata/native/queries/typescript.json", queryBytes: 7706, querySHA: "240790a7f8dbfee7c1fc0f4fc1e73098d75a519f86ec7a3a213cf7f2d9672067",
			anchorBytes: 3545, grammarCount: 6, languageVersion: language, runtimeLanguageVersion: runtime, minCompatible: compatible, symbolCount: symbols, fieldCount: fields,
			queries: []classificationWeb16QuerySpec{
				{id: "q.structure", sha256: "94b718ea5696901a19e30582397afe6037fe0ebf65855f6a765a83ba41022a2f", patterns: 4, captures: []string{"interface", "type", "enum", "method"}},
				{id: "q.probe-V28", sha256: "d9a7639fba1f7cf108bdca789ab6800702f8bc38b57c3c7c362865b6a6837255", patterns: 3, captures: []string{"conditional", "infer", "mapping"}},
				{id: "q.probe-V3", sha256: "a8b2f1c046976f9f6b5cc20776ed8b8377ec0421618ef1a8aa2c6a472a7ce9ef", patterns: 4, captures: []string{"tuple", "optional", "rest", "asserts"}},
				{id: "q.probe-V40", sha256: "47f632cb4424a761e3047d2107698843bf5285957af9e3278b8361963d0712ae", patterns: 2, captures: []string{"template", "override"}},
				{id: "q.probe-V45", sha256: "2b4202215cb85c8d4f2e03f979b67677c089c2ee7d063f9ed3b74d14e13d5136", patterns: 2, captures: []string{"instantiation", "satisfies"}},
				{id: "q.probe-V50", sha256: "65afc82d65c1525e8d7284e88dce96aa39fa56128960cd0176fc31f631f3556d", patterns: 2, captures: []string{"attributes", "decorator"}},
				{id: "q.probe-V59", sha256: "1baa54c41379035e935e4db7e43b3c9ab3c27251d29ce63ad548d77b7a64a901", patterns: 3, captures: []string{"deferred", "legacy_module", "namespace"}},
			},
		}, true
	case "tsx2-empty":
		language, runtime, compatible, symbols, fields := abi(408, 43)
		return classificationWeb16Profile{
			name: "tsx2-empty", anchorFile: "tsx2-classification-anchors.json", anchorSHA: "da0fc57ba374c6a937ab793e023f7aed8459f73ebeb0ea680f37336148060eab",
			route: "tsx", symbol: "tree_sitter_tsx", caseID: "tsx2-empty-definitions", workload: "tsx2-empty-definition-single-frame",
			queryFile: "src/testdata/native/queries/tsx.json", queryBytes: 2814, querySHA: "9ff203465bdcda7bcee4ab553e92aff327da72ed23d03ed559dd9b5d5fd72374",
			anchorBytes: 1721, grammarCount: 6, languageVersion: language, runtimeLanguageVersion: runtime, minCompatible: compatible, symbolCount: symbols, fieldCount: fields,
			queries: []classificationWeb16QuerySpec{
				{id: "q.structure", sha256: "9c7ac03567566d0b379fda0303f5db461cea457732bc8104e76101a118c44064", patterns: 4, captures: []string{"type", "interface", "function", "component"}},
				{id: "q.probe-B02", sha256: "29f60a58af9943e2c546263bebc105379ee5cd82d41c9709cc6c3d5fb9a9ae19", patterns: 3, captures: []string{"type_params", "as", "tag"}},
			},
		}, true
	default:
		return classificationWeb16Profile{}, false
	}
}

func classificationWeb16ProfileForRoute(route string) (classificationWeb16Profile, bool) {
	switch route {
	case "javascript":
		return classificationWeb16ProfileFor("javascript4-empty")
	case "jsx":
		return classificationWeb16ProfileFor("jsx3-empty")
	case "typescript":
		return classificationWeb16ProfileFor("typescript7-empty")
	case "tsx":
		return classificationWeb16ProfileFor("tsx2-empty")
	default:
		return classificationWeb16Profile{}, false
	}
}

func classificationWeb16ProfileName(name string) bool {
	_, ok := classificationWeb16ProfileFor(name)
	return ok
}

func classificationWeb16GrammarPins(profile classificationWeb16Profile) []kit.NativeInput {
	sharedHeaders := func(prefix string) []kit.NativeInput {
		return []kit.NativeInput{
			{Path: prefix + "alloc.h", Role: "header", Bytes: 985, SHA256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
			{Path: prefix + "array.h", Role: "header", Bytes: 13453, SHA256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
			{Path: prefix + "parser.h", Role: "header", Bytes: 7624, SHA256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
		}
	}
	var files []kit.NativeInput
	switch profile.route {
	case "javascript", "jsx":
		files = []kit.NativeInput{
			{Path: "src/parser.c", Role: "parser", Bytes: 4395417, SHA256: "6b46785fd55c74dfed0d13929a0aaef8b6e5f3786b31dc5c5ab76e4bb615f05c"},
			{Path: "src/scanner.c", Role: "scanner", Bytes: 23108, SHA256: "59803ef7202faca8624465d4f2dde8a23b07195a19ac55f8d17664da1b6327f5"},
		}
		return append(files, sharedHeaders("src/tree_sitter/")...)
	case "typescript":
		files = []kit.NativeInput{
			{Path: "common/scanner.h", Role: "header", Bytes: 10097, SHA256: "da66ef2bd14a3f7ea743e25ba068c6c9aae2c3509db200ff80c4a0e6116e564c"},
			{Path: "typescript/src/parser.c", Role: "parser", Bytes: 8845184, SHA256: "d5a8cbe3c229abddae6fcc360ac39704bc46f1d39d40c0a1798da8eef96c3bfa"},
			{Path: "typescript/src/scanner.c", Role: "scanner", Bytes: 573, SHA256: "9125013b42cb888379d9be909f1d73dfb75a37626c2cdbf4122718a2b431a6d3"},
		}
		return append(files, sharedHeaders("typescript/src/tree_sitter/")...)
	case "tsx":
		files = []kit.NativeInput{
			{Path: "common/scanner.h", Role: "header", Bytes: 10097, SHA256: "da66ef2bd14a3f7ea743e25ba068c6c9aae2c3509db200ff80c4a0e6116e564c"},
			{Path: "tsx/src/parser.c", Role: "parser", Bytes: 9178103, SHA256: "5024cdeddb5833348203acc0c3bcc60347034064f0fb86e8d8abc8e0b2a8ad62"},
			{Path: "tsx/src/scanner.c", Role: "scanner", Bytes: 538, SHA256: "d563cd30b2f39718c9ae4292795c5ce03a2ad01954ba3a86ef84c2781a736673"},
		}
		return append(files, sharedHeaders("tsx/src/tree_sitter/")...)
	default:
		return nil
	}
}

func classificationWeb16AnchorsValid(profileName string, a classificationLocalAnchors) error {
	profile, ok := classificationWeb16ProfileFor(profileName)
	if !ok {
		return fmt.Errorf("unknown closed Web16 profile %q", profileName)
	}
	if a.Schema != "tsgk-query-classification-anchors/r1" || a.Route != profile.route || a.Case != profile.caseID || a.API ||
		a.Operation != "native-query" || a.DriverMode != "single" || a.Workload != profile.workload ||
		a.Source.UTF8 != "" || a.Source.Bytes != 0 || a.Source.SHA256 != sha(nil) || a.Edits == nil || len(a.Edits) != 0 ||
		len(a.Expect) != 1 || a.Expect[0].Step != 0 || a.Expect[0].Syntax != "NO_ERROR" ||
		len(a.Nodes) != 1 || len(a.Queries) != len(profile.queries) || len(a.Required) != len(profile.queries) {
		return fmt.Errorf("%s Web16 empty-source profile changed", profileName)
	}
	wantRoot := []kit.TreeNode{{Parent: -1, Type: "program", Named: true}}
	if !reflect.DeepEqual(a.Nodes, wantRoot) ||
		a.ABI.LanguageVersion != profile.languageVersion || a.ABI.RuntimeLanguageVersion != profile.runtimeLanguageVersion ||
		a.ABI.RuntimeMinCompatible != profile.minCompatible || a.ABI.SymbolCount != profile.symbolCount || a.ABI.FieldCount != profile.fieldCount {
		return fmt.Errorf("%s Web16 ABI or independent empty-source CST changed", profileName)
	}
	seen := make(map[string]bool, len(profile.queries))
	for i, q := range a.Queries {
		want := profile.queries[i]
		if q.ID != want.id || seen[q.ID] || q.SHA256 != want.sha256 || q.SHA256 != sha([]byte(q.Source)) ||
			q.Metadata.Patterns != want.patterns || q.Metadata.CaptureNames == nil || !reflect.DeepEqual(q.Metadata.CaptureNames, want.captures) ||
			q.Metadata.Predicates == nil || len(q.Metadata.Predicates) != 0 {
			return fmt.Errorf("%s Web16 query source or complete predicate-free metadata changed at %d", profileName, i)
		}
		seen[q.ID] = true
		tuple := a.Required[i]
		if tuple.Query != q.ID || tuple.Step != 0 || tuple.TreeRole != "incremental" || tuple.Structural == nil || len(tuple.Structural) != 0 || tuple.Expected == nil || len(tuple.Expected) != 0 {
			return fmt.Errorf("%s Web16 denominator or empty capture anchors changed at %d", profileName, i)
		}
	}
	return nil
}

func classificationWeb16QueryRegistryValid(profileName string, a classificationLocalAnchors) error {
	profile, ok := classificationWeb16ProfileFor(profileName)
	if !ok {
		return fmt.Errorf("unknown closed Web16 profile %q", profileName)
	}
	queryPath := filepath.Join("..", "..", filepath.FromSlash(strings.TrimPrefix(profile.queryFile, "src/")))
	data, err := os.ReadFile(queryPath)
	if err != nil {
		return fmt.Errorf("read pinned %s query registry: %w", profileName, err)
	}
	return classificationWeb16QueryRegistryBytesValid(profileName, a, data)
}

func classificationWeb16QueryRegistryBytesValid(profileName string, a classificationLocalAnchors, data []byte) error {
	profile, ok := classificationWeb16ProfileFor(profileName)
	if !ok {
		return fmt.Errorf("unknown closed Web16 profile %q", profileName)
	}
	if len(data) != profile.queryBytes || sha(data) != profile.querySHA || len(a.Queries) != len(profile.queries) {
		return fmt.Errorf("%s Web16 query registry file pin changed", profileName)
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
		return fmt.Errorf("decode %s Web16 query registry: %w", profileName, err)
	}
	if registry.Schema != "tsgk-native-query-cases/r1" || registry.Route != profile.route || len(registry.Queries) != len(profile.queries) {
		return fmt.Errorf("%s Web16 query registry profile changed", profileName)
	}
	for i, query := range registry.Queries {
		if query.ID != profile.queries[i].id || query.ID != a.Queries[i].ID || query.Source != a.Queries[i].Source || sha([]byte(query.Source)) != profile.queries[i].sha256 {
			return fmt.Errorf("%s Web16 query registry source/order differs at %d", profileName, i)
		}
	}
	return nil
}

func classificationWeb16GrammarValid(profileName string, registryData []byte, route, symbol string, grammar []kit.NativeInput) error {
	profile, ok := classificationWeb16ProfileFor(profileName)
	if !ok {
		return fmt.Errorf("unknown closed Web16 profile %q", profileName)
	}
	want := classificationWeb16GrammarPins(profile)
	parserCount := 0
	for _, file := range want {
		if file.Role == "parser" {
			parserCount++
			if file.Bytes == 0 || file.Bytes > 16<<20 {
				return fmt.Errorf("%s Web16 parser exceeds the plain-input bound", profileName)
			}
		}
	}
	if len(registryData) != classificationWeb16NativeRoutesBytes || sha(registryData) != classificationWeb16NativeRoutesSHA ||
		route != profile.route || symbol != profile.symbol || parserCount != 1 || len(want) != profile.grammarCount ||
		len(grammar) != len(want) || !reflect.DeepEqual(grammar, want) {
		return fmt.Errorf("%s exact registered Web16 native grammar closure changed", profileName)
	}
	return nil
}

func TestQueryClassificationWeb16StaticProfilesAndPins(t *testing.T) {
	names := []string{"javascript4-empty", "jsx3-empty", "typescript7-empty", "tsx2-empty"}
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
	var definitions, patterns, captureNames uint32
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			profile, ok := classificationWeb16ProfileFor(name)
			if !ok {
				t.Fatal("closed Web16 profile missing")
			}
			routeProfile, ok := classificationWeb16ProfileForRoute(profile.route)
			if !ok || routeProfile.name != profile.name {
				t.Fatal("Web16 route mapped to another profile")
			}
			anchorData, err := os.ReadFile("../../testdata/query-semantics/" + profile.anchorFile)
			if err != nil || len(anchorData) != profile.anchorBytes || sha(anchorData) != profile.anchorSHA {
				t.Fatal("pinned Web16 anchor changed", err)
			}
			var anchors classificationLocalAnchors
			if err := jsonv2.Unmarshal(anchorData, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
				t.Fatal(err)
			}
			if err := classificationWeb16AnchorsValid(name, anchors); err != nil {
				t.Fatal(err)
			}
			if err := classificationWeb16QueryRegistryValid(name, anchors); err != nil {
				t.Fatal(err)
			}
			var routeMatches int
			var grammar []kit.NativeInput
			var symbol string
			for _, row := range registry.Routes {
				if row.Route == profile.route {
					routeMatches++
					grammar, symbol = row.Files, row.Symbol
				}
			}
			if routeMatches != 1 {
				t.Fatal("Web16 native route missing or duplicated", routeMatches)
			}
			if err := classificationWeb16GrammarValid(name, registryData, profile.route, symbol, grammar); err != nil {
				t.Fatal(err)
			}
			capture, err := classificationCaptureProfileFor(name)
			if err != nil || capture.anchorFile != profile.anchorFile || capture.anchorSHA != profile.anchorSHA || capture.route != profile.route ||
				capture.symbol != profile.symbol || capture.queryCount != len(profile.queries) || capture.grammarCount != profile.grammarCount || capture.paged {
				t.Fatal("Web16 capture dispatch profile differs", err)
			}
			if _, ok := classificationSmall17ProfileFor(name); ok {
				t.Fatal("Web16 profile entered the closed Small17 set")
			}
			if _, ok := classificationCJRSpecFor(name); ok {
				t.Fatal("Web16 profile entered the closed CJR set")
			}
			definitions += uint32(len(profile.queries))
			for _, query := range profile.queries {
				patterns += query.patterns
				captureNames += uint32(len(query.captures))
			}

			wrongRoute := anchors
			wrongRoute.Route = map[string]string{"javascript": "jsx", "jsx": "javascript", "typescript": "tsx", "tsx": "typescript"}[profile.route]
			if classificationWeb16AnchorsValid(name, wrongRoute) == nil {
				t.Fatal("shared parser route alias accepted")
			}
			wrongOrder := anchors
			wrongOrder.Queries = slices.Clone(anchors.Queries)
			wrongOrder.Queries[0], wrongOrder.Queries[1] = wrongOrder.Queries[1], wrongOrder.Queries[0]
			if classificationWeb16AnchorsValid(name, wrongOrder) == nil {
				t.Fatal("reordered Web16 queries accepted")
			}
			wrongMetadata := anchors
			wrongMetadata.Queries = slices.Clone(anchors.Queries)
			wrongMetadata.Queries[0].Metadata.Predicates = nil
			if classificationWeb16AnchorsValid(name, wrongMetadata) == nil {
				t.Fatal("nil predicates accepted as the explicit empty predicate list")
			}
			wrongCaptureOrder := anchors
			wrongCaptureOrder.Queries = slices.Clone(anchors.Queries)
			wrongCaptureOrder.Queries[0].Metadata.CaptureNames = slices.Clone(anchors.Queries[0].Metadata.CaptureNames)
			wrongCaptureOrder.Queries[0].Metadata.CaptureNames[0], wrongCaptureOrder.Queries[0].Metadata.CaptureNames[1] =
				wrongCaptureOrder.Queries[0].Metadata.CaptureNames[1], wrongCaptureOrder.Queries[0].Metadata.CaptureNames[0]
			if classificationWeb16AnchorsValid(name, wrongCaptureOrder) == nil {
				t.Fatal("reordered Web16 capture names accepted")
			}
			nonemptyPredicates := anchors
			nonemptyPredicates.Queries = slices.Clone(anchors.Queries)
			nonemptyPredicates.Queries[0].Metadata.Predicates = []kit.QueryPredicate{{}}
			if classificationWeb16AnchorsValid(name, nonemptyPredicates) == nil {
				t.Fatal("nonempty Web16 predicates accepted")
			}
			wrongTuple := anchors
			wrongTuple.Required = slices.Clone(anchors.Required)
			wrongTuple.Required[0].Expected = nil
			if classificationWeb16AnchorsValid(name, wrongTuple) == nil {
				t.Fatal("nil expected tuple accepted as an explicit empty array")
			}
			queryPath := filepath.Join("..", "..", filepath.FromSlash(strings.TrimPrefix(profile.queryFile, "src/")))
			queryData, err := os.ReadFile(queryPath)
			if err != nil {
				t.Fatal(err)
			}
			if classificationWeb16QueryRegistryBytesValid(name, anchors, append(slices.Clone(queryData), ' ')) == nil {
				t.Fatal("changed Web16 query file accepted")
			}
			badRegistry := slices.Clone(registryData)
			badRegistry = append(badRegistry, ' ')
			if classificationWeb16GrammarValid(name, badRegistry, profile.route, profile.symbol, grammar) == nil {
				t.Fatal("changed native route registry accepted")
			}
			badGrammar := slices.Clone(grammar)
			badGrammar[0].SHA256 = strings.Repeat("0", 64)
			if classificationWeb16GrammarValid(name, registryData, profile.route, profile.symbol, badGrammar) == nil {
				t.Fatal("changed Web16 grammar closure accepted")
			}
		})
	}
	if definitions != 16 || patterns != 51 || captureNames != 54 {
		t.Fatalf("closed Web16 source denominator changed: definitions=%d patterns=%d capture_names=%d", definitions, patterns, captureNames)
	}
}

func TestQueryClassificationWeb16LocalCapture(t *testing.T) {
	if !classificationWeb16ProfileName(os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE")) {
		t.Skip("explicit closed Web16 classification profile required")
	}
	classificationNativeLocalCapture(t)
}

func TestQueryClassificationWeb16GoCaptureEntryDispatch(t *testing.T) {
	for _, profile := range []string{"javascript4-empty", "jsx3-empty", "typescript7-empty", "tsx2-empty"} {
		if !classificationGoCaptureEntrySkipsProfile(profile) {
			t.Fatalf("Web16 profile %s reaches the Go capture entry", profile)
		}
	}
	for _, profile := range []string{"", "positive-batch"} {
		if classificationGoCaptureEntrySkipsProfile(profile) {
			t.Fatalf("Go profile %q was routed away from the Go capture entry", profile)
		}
	}
}

func TestQueryClassificationWeb16EmptyStderrCaptureMembers(t *testing.T) {
	for _, profile := range []string{"javascript4-empty", "jsx3-empty", "typescript7-empty", "tsx2-empty"} {
		t.Run(profile, func(t *testing.T) {
			processResult := &runner.Result{}
			ownerResult := &runner.Result{}
			result := &CaseResult{Process: processResult}
			capture := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult}
			classificationNormalizeCaptureStderr(profile, result, capture)
			for _, process := range []*runner.Result{processResult, ownerResult} {
				if process.StderrBytes != 0 || process.Stderr == nil || len(process.Stderr) != 0 {
					t.Fatal("Web16 zero-byte stderr was not made explicit in both process records")
				}
			}
			files, err := capture.files("capture/" + profile)
			if err != nil {
				t.Fatal(err)
			}
			stderrRows := 0
			for _, file := range files {
				if file.pin.Role == "stderr" {
					stderrRows++
					if file.pin.Bytes != 0 || file.pin.SHA256 != sha(nil) || file.data == nil || len(file.data) != 0 {
						t.Fatal("Web16 empty stderr member differs from the process")
					}
				}
			}
			if stderrRows != 1 {
				t.Fatal("Web16 empty stderr member was not serialized exactly once")
			}

			stderr := []byte{0x41, 0x42}
			processResult = &runner.Result{StderrBytes: int64(len(stderr)), Stderr: slices.Clone(stderr)}
			ownerResult = &runner.Result{StderrBytes: int64(len(stderr)), Stderr: slices.Clone(stderr)}
			result = &CaseResult{Process: processResult}
			capture = &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult}
			classificationNormalizeCaptureStderr(profile, result, capture)
			if !slices.Equal(processResult.Stderr, stderr) || !slices.Equal(ownerResult.Stderr, stderr) ||
				processResult.StderrBytes != int64(len(stderr)) || ownerResult.StderrBytes != int64(len(stderr)) {
				t.Fatal("Web16 nonempty stderr was rewritten")
			}
			files, err = capture.files("capture/" + profile + "-nonempty-stderr")
			if err != nil {
				t.Fatal(err)
			}
			stderrRows = 0
			for _, file := range files {
				if file.pin.Role == "stderr" {
					stderrRows++
					if file.pin.Bytes != uint64(len(stderr)) || file.pin.SHA256 != sha(stderr) || !slices.Equal(file.data, stderr) {
						t.Fatal("Web16 nonempty stderr member changed")
					}
				}
			}
			if stderrRows != 1 {
				t.Fatal("Web16 nonempty stderr member was not serialized exactly once")
			}
		})
	}
	for _, profile := range []string{"", "positive-batch"} {
		processResult := &runner.Result{}
		ownerResult := &runner.Result{}
		result := &CaseResult{Process: processResult}
		capture := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult}
		classificationNormalizeCaptureStderr(profile, result, capture)
		if processResult.Stderr != nil || ownerResult.Stderr != nil {
			t.Fatalf("legacy Go profile %q changed its nil zero-byte stderr representation", profile)
		}
		files, err := capture.files("capture/go-" + profile)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if file.pin.Role == "stderr" {
				t.Fatalf("legacy Go profile %q gained an empty stderr member", profile)
			}
		}
	}
}
