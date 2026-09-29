package foundation

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

type campaignDefinition struct {
	Schema, Campaign, Cutoff string
	Routes                   []string
	Platforms                []struct{ ID, GOOS, GOARCH, Runner string }
	QualificationCells       int      `json:"qualification_cells"`
	FixtureRoles             []string `json:"fixture_roles"`
	Sessions                 []struct {
		ID, Predecessor, Branch string
		Issue, Milestone        int
		AcceptanceIDs           []string `json:"acceptance_ids"`
		ImplementationStatus    string   `json:"implementation_status"`
	}
}

// Validate tracked acceptance definitions, never infer product or preparation success.
func checkCampaignDefinition(c campaignDefinition, workload string) error {
	wantRoutes := strings.Fields("csharp go python javascript jsx typescript tsx java kotlin c cpp rust swift dart php ruby r bash powershell html css json yaml xml tsql postgresql-sql")
	if c.Schema != "tsgk-campaign-definition/r1" || c.Campaign != "TSGK-C1" || c.Cutoff != "2026-09-29" || !reflect.DeepEqual(c.Routes, wantRoutes) {
		return fmt.Errorf("campaign route/scope identity mismatch")
	}
	if !reflect.DeepEqual(c.FixtureRoles, []string{"mainstream", "maintained", "historical", "owned"}) || len(c.Platforms) != 3 || c.QualificationCells != 78 {
		return fmt.Errorf("campaign coverage dimensions mismatch")
	}
	wantPlatforms := []string{"windows-amd64/windows/amd64/windows-2025", "linux-amd64/linux/amd64/ubuntu-24.04", "darwin-arm64/darwin/arm64/macos-15"}
	for i, p := range c.Platforms {
		if strings.Join([]string{p.ID, p.GOOS, p.GOARCH, p.Runner}, "/") != wantPlatforms[i] {
			return fmt.Errorf("campaign platform mismatch")
		}
	}
	branches := strings.Fields("inventory-identity profile-security schema-contract reproducibility incremental native-oracle evidence-replay real-world-qualification")
	counts := []int{14, 14, 14, 15, 15, 14, 14, 15}
	if len(c.Sessions) != 8 {
		return fmt.Errorf("campaign session count mismatch")
	}
	for i, s := range c.Sessions {
		if s.ImplementationStatus != "NOT_IMPLEMENTED" {
			return fmt.Errorf("preparation implementation status must remain NOT_IMPLEMENTED")
		}
		id, predecessor := fmt.Sprintf("%02d", i+1), fmt.Sprintf("%02d", i)
		if i == 0 {
			predecessor = "PREPARE+00"
		}
		if s.ID != id || s.Predecessor != predecessor || s.Issue != i+3 || s.Milestone != i+2 || s.Branch != "session/"+id+"-"+branches[i] || len(s.AcceptanceIDs) != counts[i] {
			return fmt.Errorf("campaign session mapping mismatch: %s", id)
		}
		for j, acceptance := range s.AcceptanceIDs {
			if acceptance != fmt.Sprintf("S%s-A%02d", id, j+1) || strings.Count(workload, "| "+acceptance+" |") != 1 {
				return fmt.Errorf("campaign acceptance definition missing/duplicated: %s", acceptance)
			}
		}
	}
	return nil
}

type sourceRegistry struct {
	Schema string
	Routes []sourceCandidate
}

type sourceCandidate struct {
	RouteID             string `json:"route_id"`
	Repository, Commit  string
	GrammarSubdirectory string                       `json:"grammar_subdirectory"`
	License             string                       `json:"license_metadata"`
	GrammarJS           *bool                        `json:"grammar_js"`
	GrammarJSON         *bool                        `json:"grammar_json"`
	ParserC             *bool                        `json:"parser_c"`
	NodeTypes           *bool                        `json:"node_types"`
	ScannerAndShared    []string                     `json:"scanner_and_shared"`
	Submodules          []struct{ Path, SHA string } `json:"submodules"`
	Feasibility         string
	SourceClosure       string   `json:"source_closure"`
	FeatureSupport      string   `json:"feature_support"`
	KnownGaps           []string `json:"known_gaps"`
}

// This checks registry data, not filesystem authorization or source closure.
func portableSourcePath(value string, rootSentinel bool) bool {
	if value == "." {
		return rootSentinel
	}
	if !utf8.ValidString(value) || !fs.ValidPath(value) || strings.ContainsAny(value, "\\:") {
		return false
	}
	device := regexp.MustCompile(`^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$`)
	for _, segment := range strings.Split(value, "/") {
		if strings.TrimRight(segment, " .") != segment || device.MatchString(strings.Split(strings.ToUpper(segment), ".")[0]) {
			return false
		}
		for _, r := range segment {
			if r < 32 || r == 127 {
				return false
			}
		}
	}
	return true
}

func checkSources(registry sourceRegistry, routes []string, scope string) error {
	if registry.Schema != "tsgk-language-candidates/r1" || len(registry.Routes) != len(routes) {
		return fmt.Errorf("source registry scope mismatch")
	}
	rows := regexp.MustCompile("(?m)^\\| `([a-z-]+)` \\| ([^|]+) \\| ([^|]+) \\|$").FindAllStringSubmatch(scope, -1)
	if len(rows) != len(routes) {
		return fmt.Errorf("feature scope table count mismatch")
	}
	for i, r := range registry.Routes {
		if r.RouteID != routes[i] || rows[i][1] != routes[i] || strings.TrimSpace(rows[i][2]) == "" || strings.TrimSpace(rows[i][3]) == "" {
			return fmt.Errorf("feature scope route mismatch")
		}
		if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(r.Repository) || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(r.Commit) || !portableSourcePath(r.GrammarSubdirectory, true) {
			return fmt.Errorf("source identity/path mismatch: %s", r.RouteID)
		}
		if (r.License != "MIT" && r.License != "BSD-2-Clause" && r.License != "BSD-3-Clause") || r.GrammarJS == nil || !*r.GrammarJS || r.GrammarJSON == nil || !*r.GrammarJSON || r.NodeTypes == nil || !*r.NodeTypes || r.ParserC == nil || *r.ParserC != (r.RouteID != "swift") {
			return fmt.Errorf("observed source metadata mismatch: %s", r.RouteID)
		}
		if r.Feasibility != "METADATA_OBSERVED_NATIVE_NOT_RUN" || r.SourceClosure != "CONTENT_REVIEW_PENDING" || r.Submodules == nil || len(r.ScannerAndShared) == 0 {
			return fmt.Errorf("source observation status/closure mismatch: %s", r.RouteID)
		}
		for _, submodule := range r.Submodules {
			if !portableSourcePath(submodule.Path, false) || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(submodule.SHA) {
				return fmt.Errorf("source submodule identity mismatch: %s", r.RouteID)
			}
		}
		seen := map[string]bool{}
		for _, name := range r.ScannerAndShared {
			if !portableSourcePath(name, false) || seen[name] {
				return fmt.Errorf("source shared path mismatch: %s", r.RouteID)
			}
			seen[name] = true
		}
		wantSupport := "UNRESOLVED"
		if r.RouteID == "csharp" {
			wantSupport = "KNOWN_REQUIRED_SUPPORT_GAP"
		}
		if r.RouteID == "tsql" {
			wantSupport = "STATIC_REQUIRED_SOURCE_GAP"
		}
		if r.FeatureSupport != wantSupport || ((r.RouteID == "csharp" || r.RouteID == "swift" || r.RouteID == "tsql" || r.RouteID == "postgresql-sql") && len(r.KnownGaps) == 0) {
			return fmt.Errorf("source known gap/support mismatch: %s", r.RouteID)
		}
	}
	return nil
}

// Check public registry shape and references, not linguistic completeness or adoption.
func checkFeatureDisposition(routes []string, inventory, risks string) error {
	features := map[string]bool{}
	counts := map[string]map[string]int{}
	want := map[string]bool{}
	for _, route := range routes {
		want[route] = true
		counts[route] = map[string]int{}
		if strings.Count(inventory, "\n## "+route+"\n") != 1 {
			return fmt.Errorf("feature route section mismatch: %s", route)
		}
	}
	rowID := regexp.MustCompile(`^([a-z-]+)-[A-Z][0-9]+[a-z]*$`)
	for _, line := range strings.Split(inventory, "\n") {
		if !strings.HasPrefix(line, "| ") {
			continue
		}
		fields := strings.Split(line, "|")
		id := strings.TrimSpace(fields[1])
		match := rowID.FindStringSubmatch(id)
		if match == nil {
			for _, route := range routes {
				if strings.HasPrefix(id, route+"-") {
					return fmt.Errorf("invalid feature identity: %s", id)
				}
			}
			continue
		}
		if len(fields) != 8 || !want[match[1]] || features[id] {
			return fmt.Errorf("feature identity/columns mismatch: %s", id)
		}
		features[id] = true
		for i := 2; i <= 6; i++ {
			fields[i] = strings.TrimSpace(fields[i])
			if fields[i] == "" {
				return fmt.Errorf("empty feature field: %s", id)
			}
		}
		disposition := fields[3]
		if disposition != "REQ" && disposition != "SEM" && disposition != "RUN" && disposition != "EXT" {
			return fmt.Errorf("unresolved feature disposition: %s", id)
		}
		counts[match[1]][disposition]++
		if disposition == "REQ" {
			seen := map[string]bool{}
			for _, kind := range strings.Split(fields[6], ",") {
				if !strings.Contains("PNREQW", kind) || len(kind) != 1 || seen[kind] {
					return fmt.Errorf("invalid feature case kind: %s", id)
				}
				seen[kind] = true
				counts[match[1]][kind]++
			}
			if !seen["P"] {
				return fmt.Errorf("required feature without positive case: %s", id)
			}
		} else if fields[6] != "-" {
			return fmt.Errorf("excluded feature has parser cases: %s", id)
		}
	}
	for _, route := range routes {
		for _, key := range []string{"REQ", "SEM", "RUN", "EXT", "P", "N", "R", "E", "Q", "W"} {
			if counts[route][key] == 0 {
				return fmt.Errorf("feature coverage missing: %s/%s", route, key)
			}
		}
		prefix := "| " + route + " | "
		if strings.Count(risks, prefix) != 1 {
			return fmt.Errorf("source risk route mismatch: %s", route)
		}
		for _, line := range strings.Split(risks, "\n") {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			fields := strings.Split(line, "|")
			if len(fields) != 7 {
				return fmt.Errorf("source risk columns mismatch: %s", route)
			}
			kind := strings.TrimSpace(fields[2])
			if kind != "UPSTREAM_DECLARED" && kind != "STATIC_SOURCE_OBSERVATION" && kind != "UNVERIFIED_SUPPORT" {
				return fmt.Errorf("unobserved source risk claim: %s", route)
			}
			for _, id := range strings.Split(strings.TrimSpace(fields[3]), ",") {
				if !features[id] || !strings.HasPrefix(id, route+"-") {
					return fmt.Errorf("unknown source risk feature: %s", id)
				}
			}
		}
	}
	return nil
}

func TestFeatureDisposition(t *testing.T) {
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(repository(t), filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	var campaign campaignDefinition
	if err := json.Unmarshal(read("src/contracts/campaign-01.json"), &campaign); err != nil {
		t.Fatal(err)
	}
	inventory := string(read("docs/validation/language-feature-disposition.md"))
	risks := string(read("docs/validation/source-feature-feasibility.md"))
	if err := checkFeatureDisposition(campaign.Routes, inventory, risks); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, inventory, risks, diagnostic string }{
		{"missing-route", strings.Replace(inventory, "## tsx\n", "## removed\n", 1), risks, "route section"},
		{"unresolved-scope", strings.Replace(inventory, "| REQ |", "| UNRESOLVED |", 1), risks, "unresolved feature"},
		{"missing-positive", strings.Replace(inventory, "| P,N,R,E |", "| N,R,E |", 1), risks, "without positive"},
		{"invented-reproduction", inventory, strings.Replace(risks, "| UPSTREAM_DECLARED |", "| REPRODUCED_FAILURE |", 1), "unobserved source"},
		{"unknown-feature", inventory, strings.Replace(risks, "csharp-B01,csharp-V14c", "csharp-MISSING", 1), "unknown source risk"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkFeatureDisposition(campaign.Routes, tc.inventory, tc.risks)
			if err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("mutation not detected as %q: %v", tc.diagnostic, err)
			}
		})
	}
}

func TestCampaignDefinitions(t *testing.T) {
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(repository(t), filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	data := read("src/contracts/campaign-01.json")
	workload := string(read("docs/validation/workload-matrix.md"))
	var c campaignDefinition
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if err := checkCampaignDefinition(c, workload); err != nil {
		t.Fatal(err)
	}
	sourceData := read("src/contracts/language-sources.json")
	scope := string(read("docs/validation/language-feature-scope.md"))
	var raw sourceRegistry
	if err := json.Unmarshal(sourceData, &raw); err != nil {
		t.Fatal(err)
	}
	if err := checkSources(raw, c.Routes, scope); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, diagnostic string
		mutate           func(*campaignDefinition, *string)
	}{
		{"missing-route", "route/scope", func(c *campaignDefinition, _ *string) { c.Routes = c.Routes[:25] }},
		{"collapsed-sql", "route/scope", func(c *campaignDefinition, _ *string) { c.Routes[25] = "tsql" }},
		{"wrong-arch", "platform", func(c *campaignDefinition, _ *string) { c.Platforms[2].GOARCH = "amd64" }},
		{"wrong-predecessor", "session mapping", func(c *campaignDefinition, _ *string) { c.Sessions[4].Predecessor = "06" }},
		{"missing-acceptance", "acceptance", func(_ *campaignDefinition, w *string) { *w = strings.Replace(*w, "| S05-A04 |", "| REMOVED |", 1) }},
		{"false-implementation", "implementation status", func(c *campaignDefinition, _ *string) { c.Sessions[0].ImplementationStatus = "IMPLEMENTED" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var candidate campaignDefinition
			if err := json.Unmarshal(data, &candidate); err != nil {
				t.Fatal(err)
			}
			text := workload
			tc.mutate(&candidate, &text)
			if err := checkCampaignDefinition(candidate, text); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("mutation not detected as %q: %v", tc.diagnostic, err)
			}
		})
	}
	for _, tc := range []struct {
		name, diagnostic string
		mutate           func(*sourceRegistry, *string)
	}{
		{"missing-schema-metadata", "metadata", func(s *sourceRegistry, _ *string) { s.Routes[0].NodeTypes = nil }},
		{"changed-parser-presence", "metadata", func(s *sourceRegistry, _ *string) { *s.Routes[0].ParserC = false }},
		{"changed-json-presence", "metadata", func(s *sourceRegistry, _ *string) { *s.Routes[0].GrammarJSON = false }},
		{"missing-shared", "closure", func(s *sourceRegistry, _ *string) { s.Routes[0].ScannerAndShared = nil }},
		{"unsafe-shared", "shared path", func(s *sourceRegistry, _ *string) { s.Routes[0].ScannerAndShared[0] = "../outside" }},
		{"unsafe-grammar", "identity/path", func(s *sourceRegistry, _ *string) { s.Routes[0].GrammarSubdirectory = "../outside" }},
		{"false-closure", "closure", func(s *sourceRegistry, _ *string) { s.Routes[0].SourceClosure = "VERIFIED" }},
		{"false-support", "gap/support", func(s *sourceRegistry, _ *string) { s.Routes[0].FeatureSupport = "SUPPORTED" }},
		{"missing-gap", "gap/support", func(s *sourceRegistry, _ *string) { s.Routes[0].KnownGaps = nil }},
		{"missing-feature-row", "scope table", func(_ *sourceRegistry, text *string) { *text = strings.Replace(*text, "| `csharp` |", "| csharp |", 1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var candidate sourceRegistry
			if err := json.Unmarshal(sourceData, &candidate); err != nil {
				t.Fatal(err)
			}
			text := scope
			tc.mutate(&candidate, &text)
			if err := checkSources(candidate, c.Routes, text); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("mutation not detected as %q: %v", tc.diagnostic, err)
			}
		})
	}
}

func TestSourceRegistryPaths(t *testing.T) {
	for _, value := range []string{".", "typescript", "common/scanner.h", "dir/한글.c"} {
		if !portableSourcePath(value, true) {
			t.Errorf("valid selection rejected: %q", value)
		}
	}
	for _, value := range []string{"", "./x", "x/.", "x/../y", "../x", "/root", "C:/x", `\\host\share`, `a\b`, "a//b", "CON.txt", "x. ", "a\x00b"} {
		if portableSourcePath(value, true) {
			t.Errorf("invalid selection accepted: %q", value)
		}
	}
	if portableSourcePath(".", false) {
		t.Fatal("root sentinel accepted as a file path")
	}
}
