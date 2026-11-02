package native

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"os"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestQueryClassificationDartPHP12StaticProfilesAndPins(t *testing.T) {
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
	var definitions, patterns, entries int
	names := make(map[string]bool)
	for _, name := range []string{"dart7-empty", "php5-empty"} {
		t.Run(name, func(t *testing.T) {
			profile, ok := classificationDartPHP12ProfileFor(name)
			if !ok || !classificationGoCaptureEntrySkipsProfile(name) || classificationWeb16ProfileName(name) || classificationScript17ProfileName(name) {
				t.Fatal("closed DartPHP12 dispatch differs")
			}
			if _, ok := classificationSmall17ProfileFor(name); ok {
				t.Fatal("profile entered Small17")
			}
			raw, err := os.ReadFile("../../testdata/query-semantics/" + profile.anchorFile)
			if err != nil || len(raw) != profile.anchorBytes || sha(raw) != profile.anchorSHA {
				t.Fatal("anchor pin differs", err)
			}
			var anchors classificationLocalAnchors
			if err := jsonv2.Unmarshal(raw, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
				t.Fatal(err)
			}
			if err := classificationEmptyAnchorsValid(name, anchors); err != nil {
				t.Fatal(err)
			}
			if err := classificationEmptyQueryRegistryValid(name, anchors); err != nil {
				t.Fatal(err)
			}
			matches := 0
			var grammar []kit.NativeInput
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
			if err := classificationEmptyGrammarValid(name, registryData, profile.route, profile.symbol, grammar); err != nil {
				t.Fatal(err)
			}
			capture, err := classificationCaptureProfileFor(name)
			if err != nil || capture.anchorFile != profile.anchorFile || capture.anchorSHA != profile.anchorSHA || capture.route != profile.route ||
				capture.symbol != profile.symbol || capture.queryCount != len(profile.queries) || capture.grammarCount != profile.grammarCount || capture.paged {
				t.Fatal("capture dispatch differs", err)
			}
			bad := anchors
			bad.Source.UTF8 = ";"
			bad.Source.Bytes = 1
			bad.Source.SHA256 = sha([]byte(";"))
			if classificationEmptyAnchorsValid(name, bad) == nil {
				t.Fatal("nonempty source entered empty profile")
			}
			badGrammar := slices.Clone(grammar)
			badGrammar[0].Path = "src/parser.c"
			if profile.route == "php" && classificationEmptyGrammarValid(name, registryData, profile.route, profile.symbol, badGrammar) == nil {
				t.Fatal("PHP common header alias accepted")
			}
			if classificationEmptyGrammarValid(name, registryData, profile.route, profile.symbol, grammar[1:]) == nil {
				t.Fatal("incomplete grammar accepted")
			}
			definitions += len(profile.queries)
			for _, query := range profile.queries {
				patterns += int(query.patterns)
				entries += len(query.captures)
				for _, capture := range query.captures {
					names[capture] = true
				}
			}
		})
	}
	if definitions != 12 || patterns != 42 || entries != 43 || len(names) != 38 {
		t.Fatal("metadata denominator differs", definitions, patterns, entries, len(names))
	}
	for _, name := range []string{"", "positive-batch", "dart", "dart7-empty-alias", "php5-empty ", "powershell3-empty", "python8-empty", "javascript4-empty"} {
		if classificationDartPHP12ProfileName(name) {
			t.Fatal("non-DartPHP12 name accepted", name)
		}
	}
}

func TestQueryClassificationDartPHP12LocalCapture(t *testing.T) {
	if !classificationDartPHP12ProfileName(os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE")) {
		t.Skip("explicit closed DartPHP12 classification profile required")
	}
	classificationNativeLocalCapture(t)
}
