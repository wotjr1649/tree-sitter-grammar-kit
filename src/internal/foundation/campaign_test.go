package foundation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
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
	// Source registry uses portable JSON names, independent of Go field matching.
	var raw struct {
		Schema string
		Routes []struct {
			RouteID             string `json:"route_id"`
			Repository, Commit  string
			GrammarSubdirectory string `json:"grammar_subdirectory"`
		}
	}
	if err := json.Unmarshal(read("src/contracts/language-sources.json"), &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Schema != "tsgk-language-candidates/r1" || len(raw.Routes) != len(c.Routes) {
		t.Fatal("source registry scope mismatch")
	}
	for i, r := range raw.Routes {
		if r.RouteID != c.Routes[i] || !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(r.Repository) || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(r.Commit) || r.GrammarSubdirectory == "" {
			t.Fatalf("unresolved source identity at route %d", i)
		}
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
}
