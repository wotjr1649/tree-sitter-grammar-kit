package foundation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
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
		Integration             *struct {
			PRs          []int  `json:"prs"`
			MergeCommit  string `json:"merge_commit"`
			PostMergeRun int64  `json:"post_merge_run"`
		} `json:"integration"`
	}
}

var mergeCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

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
	counts := []int{19, 16, 18, 18, 22, 17, 16, 17}
	if len(c.Sessions) != 8 {
		return fmt.Errorf("campaign session count mismatch")
	}
	// INTEGRATED records a merged session with its PRs, merge commit and post-merge run. It
	// says nothing about qualification or support, which only the qualify CI receipt owns.
	lastPR, lastRun, merges := 0, int64(0), map[string]bool{}
	for i, s := range c.Sessions {
		switch s.ImplementationStatus {
		case "NOT_IMPLEMENTED":
			if s.Integration != nil {
				return fmt.Errorf("implementation status NOT_IMPLEMENTED carries integration evidence: %02d", i+1)
			}
		case "INTEGRATED":
			if i > 0 && c.Sessions[i-1].ImplementationStatus != "INTEGRATED" {
				return fmt.Errorf("implementation status INTEGRATED before its predecessor: %02d", i+1)
			}
			in := s.Integration
			if in == nil || len(in.PRs) == 0 || !mergeCommitPattern.MatchString(in.MergeCommit) || in.PostMergeRun <= 0 {
				return fmt.Errorf("implementation status INTEGRATED without PR, merge commit and post-merge run: %02d", i+1)
			}
			for _, pr := range in.PRs {
				if pr <= 0 {
					return fmt.Errorf("implementation status INTEGRATED with an invalid PR number: %02d", i+1)
				}
				if pr <= lastPR {
					return fmt.Errorf("implementation status INTEGRATED with evidence not after its predecessor: %02d", i+1)
				}
				lastPR = pr
			}
			if in.PostMergeRun <= lastRun || merges[in.MergeCommit] {
				return fmt.Errorf("implementation status INTEGRATED with evidence not after its predecessor: %02d", i+1)
			}
			lastRun, merges[in.MergeCommit] = in.PostMergeRun, true
		default:
			return fmt.Errorf("unknown implementation status %q: %02d", s.ImplementationStatus, i+1)
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
	SourceClosure       string              `json:"source_closure"`
	FeatureSupport      string              `json:"feature_support"`
	KnownGaps           []string            `json:"known_gaps"`
	Adoption            *sourceAdoption     `json:"adoption"`
	Regeneration        *sourceRegeneration `json:"regeneration"`
	C2Patch             *sourceC2Patch      `json:"c2_patch"`
	SupersededCandidate *struct {
		Repository, Commit string
	} `json:"superseded_candidate"`
}

type sourcePin struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// sourceRegeneration records a route regenerated from its pinned grammar (Campaign 02, #76):
// no adoption native run, no patches of its own, pinned outputs. A route's C2 patches live
// in its c2_patch record; the outputs are then those of the patched grammar.
type sourceRegeneration struct {
	Decision      string      `json:"decision"`
	Generation    string      `json:"generation"`
	Outputs       []sourcePin `json:"outputs"`
	PatchSubjects []string    `json:"patch_subjects"`
	PatchedFiles  []sourcePin `json:"patched_files"`
}

type sourceAdoption struct {
	Decision      string   `json:"decision"`
	Candidate     string   `json:"candidate"`
	PatchSubjects []string `json:"patch_subjects"`
	PatchedFiles  []struct {
		Path   string `json:"path"`
		Bytes  int64  `json:"bytes"`
		SHA256 string `json:"sha256"`
	} `json:"patched_files"`
	Generation            string   `json:"generation"`
	ExpectationAmendments []string `json:"expectation_amendments"`
	NativeEvidence        struct {
		RunID           int64  `json:"run_id"`
		Stage           string `json:"stage"`
		RegisteredRows  int    `json:"registered_rows"`
		RegisteredEdits int    `json:"registered_edits"`
		Platform        string `json:"platform"`
		Result          string `json:"result"`
	} `json:"native_evidence"`
}

type expectationAmendments struct {
	Schema     string `json:"schema"`
	Amendments []struct {
		ID, Route, Stage  string
		RegisteredSubject string                   `json:"registered_subject"`
		InputBytes        int                      `json:"input_bytes"`
		InputSHA256       string                   `json:"input_sha256"`
		OldWindow         struct{ Start, End int } `json:"old_error_window"`
		NewWindow         struct{ Start, End int } `json:"new_error_window"`
		ComparatorChanged bool                     `json:"comparator_changed"`
		OtherRowsChanged  bool                     `json:"other_rows_changed"`
	} `json:"amendments"`
}

// checkExpectationAmendments pins the single user-approved T-SQL window correction to its registered case.
func checkExpectationAmendments(e expectationAmendments) error {
	if e.Schema != "tsgk-p05-expectation-amendments/r1" || len(e.Amendments) != 1 {
		return fmt.Errorf("expectation amendment scope mismatch")
	}
	a := e.Amendments[0]
	if a.ID != "P05-MSSQL-TEMPORAL-BOUNDARY-NEGATIVE-r1" || a.Route != "tsql" || a.Stage != "mssql-patch-r1" || a.RegisteredSubject != "src/dev/prepare-p05/remedy-followup-r2.json" || a.InputBytes != 54 || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(a.InputSHA256) {
		return fmt.Errorf("expectation amendment identity mismatch")
	}
	if a.OldWindow.Start != 27 || a.OldWindow.End != 50 || a.NewWindow.Start != 27 || a.NewWindow.End != 52 || a.ComparatorChanged || a.OtherRowsChanged {
		return fmt.Errorf("expectation amendment window mismatch")
	}
	return nil
}

// adoptedRoutes are the routes whose remedied candidates the user adopted on 2026-10-02 (PREPARE #20).
var adoptedRoutes = map[string]bool{"csharp": true, "typescript": true, "tsx": true, "swift": true, "tsql": true, "postgresql-sql": true}

func checkAdoption(r sourceCandidate) error {
	a := r.Adoption
	if a == nil || strings.TrimSpace(a.Decision) == "" || strings.TrimSpace(a.Generation) == "" || !regexp.MustCompile(`^P05-[A-Z0-9-]+-r[0-9]+$`).MatchString(a.Candidate) || len(a.PatchSubjects) == 0 || len(a.PatchedFiles) == 0 {
		return fmt.Errorf("source adoption record mismatch: %s", r.RouteID)
	}
	for _, subject := range a.PatchSubjects {
		if !regexp.MustCompile(`^src/dev/prepare-p05/remedy-[a-z0-9-]+\.json$`).MatchString(subject) {
			return fmt.Errorf("source adoption patch subject mismatch: %s", r.RouteID)
		}
	}
	for _, file := range a.PatchedFiles {
		if !portableSourcePath(file.Path, false) || file.Bytes <= 0 || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(file.SHA256) {
			return fmt.Errorf("source adoption patched file mismatch: %s", r.RouteID)
		}
	}
	n := a.NativeEvidence
	if n.RunID <= 0 || strings.TrimSpace(n.Stage) == "" || n.RegisteredRows <= 0 || n.RegisteredEdits < 0 || n.Platform != "linux/amd64" || n.Result != "ALL_REGISTERED_ROWS_AND_EDITS_PASS" {
		return fmt.Errorf("source adoption native evidence mismatch: %s", r.RouteID)
	}
	wantAmendments := r.RouteID == "tsql"
	if (len(a.ExpectationAmendments) == 1 && a.ExpectationAmendments[0] == "src/dev/prepare-p05/remedy-expectation-amendments-r1.json") != wantAmendments || (!wantAmendments && len(a.ExpectationAmendments) != 0) {
		return fmt.Errorf("source adoption expectation amendment mismatch: %s", r.RouteID)
	}
	if r.RouteID == "tsql" && (r.SupersededCandidate == nil || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(r.SupersededCandidate.Commit) || r.SupersededCandidate.Repository == r.Repository) {
		return fmt.Errorf("source adoption superseded candidate mismatch: %s", r.RouteID)
	}
	return nil
}

// regenerationDecision names the first step (#89) of the patch path the user authorized
// for all 26 routes on #76: every route that is not adopted is regenerated by the pinned
// generator from its unpatched grammar, so later patches (C2-PATCH-r1) are measured
// without drift.
const regenerationDecision = "C2-REGENERATE-r1"

// regeneratedOutputs are the registered generator outputs, in registry order.
var regeneratedOutputs = []string{"grammar.json", "node-types.json", "parser.c", "tree_sitter/alloc.h", "tree_sitter/array.h", "tree_sitter/parser.h"}

func checkRegeneration(r sourceCandidate) error {
	g := r.Regeneration
	if g == nil || g.Decision != regenerationDecision || strings.TrimSpace(g.Generation) == "" {
		return fmt.Errorf("source regeneration record mismatch: %s", r.RouteID)
	}
	if len(g.PatchSubjects) != 0 || len(g.PatchedFiles) != 0 {
		return fmt.Errorf("source regeneration carries patches: %s", r.RouteID)
	}
	if len(g.Outputs) != len(regeneratedOutputs) {
		return fmt.Errorf("source regeneration output pin mismatch: %s", r.RouteID)
	}
	for i, o := range g.Outputs {
		if o.Path != regeneratedOutputs[i] || o.Bytes <= 0 || !hex64.MatchString(o.SHA256) {
			return fmt.Errorf("source regeneration output pin mismatch: %s", r.RouteID)
		}
	}
	return nil
}

// c2PatchDecision is the user's #76 decision that any of the 26 routes may carry project-local
// patches to reach FULL PASS. A C2 record names its patch subjects and the files the route's
// whole chain leaves (after the adoption chain on adopted routes); the adoption record keeps
// its P05 identity.
const c2PatchDecision = "C2-PATCH-r1"

type sourceC2Patch struct {
	Decision     string      `json:"decision"`
	Subjects     []string    `json:"subjects"`
	PatchedFiles []sourcePin `json:"patched_files"`
}

var c2SubjectPattern = regexp.MustCompile(`^src/dev/c2-patches/[a-z0-9-]+\.json$`)

func checkC2Patch(r sourceCandidate) error {
	p := r.C2Patch
	if p == nil {
		return nil
	}
	if p.Decision != c2PatchDecision || len(p.Subjects) == 0 || len(p.PatchedFiles) == 0 {
		return fmt.Errorf("source C2 patch record mismatch: %s", r.RouteID)
	}
	seen := map[string]bool{}
	for _, s := range p.Subjects {
		if !c2SubjectPattern.MatchString(s) || seen[s] {
			return fmt.Errorf("source C2 patch subject mismatch: %s", r.RouteID)
		}
		seen[s] = true
	}
	for _, f := range p.PatchedFiles {
		if !portableSourcePath(f.Path, false) || seen[f.Path] || f.Bytes <= 0 || !hex64.MatchString(f.SHA256) {
			return fmt.Errorf("source C2 patched file mismatch: %s", r.RouteID)
		}
		seen[f.Path] = true
	}
	return nil
}

// c2Created lists the files a route's C2 subjects create ("create": true): files absent at
// the pinned commit, such as go's automatic-semicolon scanner (#98).
type c2Created map[string][]c2CreatedFile

type c2CreatedFile struct {
	Target     string `json:"target"`
	Create     *bool  `json:"create"`
	Operations []struct {
		Before      string `json:"before"`
		After       string `json:"after"`
		Occurrences *int   `json:"occurrences"`
	} `json:"operations"`
}

// checkC2Created: "create" is true when present; a created file has exactly one operation, with an empty before, 0
// occurrences and the whole content; its source record does not list it among the upstream
// scanner and shared files; and its C2 record pins it.
func checkC2Created(registry sourceRegistry, created c2Created) error {
	for _, r := range registry.Routes {
		for _, f := range created[r.RouteID] {
			if f.Create == nil || !*f.Create {
				return fmt.Errorf("source C2 create flag is not true: %s %s", r.RouteID, f.Target)
			}
			if len(f.Operations) != 1 || f.Operations[0].Before != "" || f.Operations[0].Occurrences == nil || *f.Operations[0].Occurrences != 0 || f.Operations[0].After == "" {
				return fmt.Errorf("source C2 created file is not one whole-content operation: %s %s", r.RouteID, f.Target)
			}
			if slices.Contains(r.ScannerAndShared, f.Target) {
				return fmt.Errorf("source C2 created file is an upstream file: %s %s", r.RouteID, f.Target)
			}
			if r.C2Patch == nil || !slices.ContainsFunc(r.C2Patch.PatchedFiles, func(p sourcePin) bool { return p.Path == f.Target }) {
				return fmt.Errorf("source C2 created file is not pinned: %s %s", r.RouteID, f.Target)
			}
		}
	}
	return nil
}

func TestC2CreatedFiles(t *testing.T) {
	root := repository(t)
	load := func() sourceRegistry {
		data, err := os.ReadFile(filepath.Join(root, "src/contracts/language-sources.json"))
		if err != nil {
			t.Fatal(err)
		}
		var s sourceRegistry
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	registry := load()
	loadCreated := func() c2Created {
		created := c2Created{}
		for _, r := range registry.Routes {
			if r.C2Patch == nil {
				continue
			}
			for _, name := range r.C2Patch.Subjects {
				data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
				if err != nil {
					t.Fatal(err)
				}
				var doc struct {
					Files []c2CreatedFile `json:"files"`
				}
				if err := json.Unmarshal(data, &doc); err != nil {
					t.Fatal(err)
				}
				for _, f := range doc.Files {
					if f.Create != nil {
						created[r.RouteID] = append(created[r.RouteID], f)
					}
				}
			}
		}
		return created
	}
	created := loadCreated()
	if len(created) != 1 || len(created["go"]) != 1 || created["go"][0].Target != "src/scanner.c" {
		t.Fatalf("created files %v, want go's src/scanner.c only", created)
	}
	if err := checkC2Created(registry, created); err != nil {
		t.Fatal(err)
	}
	one, no := 1, false
	for _, tc := range []struct {
		name, diagnostic string
		mutate           func(*sourceRegistry, c2Created)
	}{
		{"created-file-listed-upstream", "created file is an upstream file", func(s *sourceRegistry, _ c2Created) {
			sourceRoute(s, "go").ScannerAndShared = append(sourceRoute(s, "go").ScannerAndShared, "src/scanner.c")
		}},
		{"created-file-without-pin", "created file is not pinned", func(s *sourceRegistry, _ c2Created) {
			p := sourceRoute(s, "go").C2Patch
			p.PatchedFiles = slices.DeleteFunc(p.PatchedFiles, func(f sourcePin) bool { return f.Path == "src/scanner.c" })
		}},
		{"created-file-two-operations", "not one whole-content operation", func(_ *sourceRegistry, c c2Created) {
			c["go"][0].Operations = append(c["go"][0].Operations, c["go"][0].Operations[0])
		}},
		{"created-file-with-before", "not one whole-content operation", func(_ *sourceRegistry, c c2Created) { c["go"][0].Operations[0].Before = "x" }},
		{"created-file-with-occurrences", "not one whole-content operation", func(_ *sourceRegistry, c c2Created) { c["go"][0].Operations[0].Occurrences = &one }},
		{"created-file-without-occurrences", "not one whole-content operation", func(_ *sourceRegistry, c c2Created) { c["go"][0].Operations[0].Occurrences = nil }},
		{"created-file-without-content", "not one whole-content operation", func(_ *sourceRegistry, c c2Created) { c["go"][0].Operations[0].After = "" }},
		{"create-false", "create flag is not true", func(_ *sourceRegistry, c c2Created) { c["go"][0].Create = &no }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := load()
			c := loadCreated()
			tc.mutate(&s, c)
			if err := checkC2Created(s, c); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("mutation not detected as %q: %v", tc.diagnostic, err)
			}
		})
	}
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
		wantFeasibility, wantClosure := "METADATA_OBSERVED_NATIVE_NOT_RUN", "CONTENT_REVIEW_PENDING"
		if adoptedRoutes[r.RouteID] {
			wantFeasibility, wantClosure = "ADOPTED_REMEDIED_CANDIDATE_REGISTERED_NATIVE_PASS", "ADOPTED_PINNED_PATCH_RECONSTRUCTION"
			if err := checkAdoption(r); err != nil {
				return err
			}
			if r.Regeneration != nil {
				return fmt.Errorf("source regeneration record on an adopted route: %s", r.RouteID)
			}
		} else if r.Adoption != nil || r.SupersededCandidate != nil {
			return fmt.Errorf("source observation status/closure mismatch: %s", r.RouteID)
		} else if err := checkRegeneration(r); err != nil {
			return err
		}
		if err := checkC2Patch(r); err != nil {
			return err
		}
		if r.Feasibility != wantFeasibility || r.SourceClosure != wantClosure || r.Submodules == nil || len(r.ScannerAndShared) == 0 {
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
		if adoptedRoutes[r.RouteID] {
			wantSupport = "REGISTERED_CASES_PASS_GAPS_RECORDED"
		}
		if r.FeatureSupport != wantSupport || ((adoptedRoutes[r.RouteID] || r.RouteID == "csharp" || r.RouteID == "swift" || r.RouteID == "tsql" || r.RouteID == "postgresql-sql") && len(r.KnownGaps) == 0) {
			return fmt.Errorf("source known gap/support mismatch: %s", r.RouteID)
		}
	}
	return nil
}

// sourceRoute selects a route by name so negative controls keep their meaning when the
// registry order changes.
func sourceRoute(s *sourceRegistry, id string) *sourceCandidate {
	for i := range s.Routes {
		if s.Routes[i].RouteID == id {
			return &s.Routes[i]
		}
	}
	panic("source route missing: " + id)
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
		if len(fields) != 9 || !want[match[1]] || features[id] {
			return fmt.Errorf("feature identity/columns mismatch: %s", id)
		}
		features[id] = true
		for i := 2; i <= 7; i++ {
			fields[i] = strings.TrimSpace(fields[i])
			if fields[i] == "" {
				return fmt.Errorf("empty feature field: %s", id)
			}
		}
		if !strings.Contains(fields[7], "](https://") {
			return fmt.Errorf("missing feature primary reference: %s", id)
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
		{"missing-reference", strings.Replace(inventory, "[spec Lexical structure](", "section(", 1), risks, "primary reference"},
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
	strict := func(b []byte, c *campaignDefinition) error {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		return dec.Decode(c)
	}
	var c campaignDefinition
	if err := strict(data, &c); err != nil {
		t.Fatal(err)
	}
	if err := strict(bytes.Replace(data, []byte(`"implementation_status"`), []byte(`"qualification": "PASS", "implementation_status"`), 1), new(campaignDefinition)); err == nil {
		t.Fatal("unknown campaign field not rejected")
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
	var amendments expectationAmendments
	if err := json.Unmarshal(read("src/dev/prepare-p05/remedy-expectation-amendments-r1.json"), &amendments); err != nil {
		t.Fatal(err)
	}
	if err := checkExpectationAmendments(amendments); err != nil {
		t.Fatal(err)
	}
	widened := amendments
	widened.Amendments = append(widened.Amendments[:0:0], amendments.Amendments...)
	widened.Amendments[0].ComparatorChanged = true
	if err := checkExpectationAmendments(widened); err == nil || !strings.Contains(err.Error(), "window") {
		t.Fatalf("comparator change not detected: %v", err)
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
		{"qualified-status", "implementation status", func(c *campaignDefinition, _ *string) { c.Sessions[7].ImplementationStatus = "QUALIFIED" }},
		{"integrated-without-evidence", "without PR", func(c *campaignDefinition, _ *string) { c.Sessions[2].Integration = nil }},
		{"integrated-short-merge", "without PR", func(c *campaignDefinition, _ *string) { c.Sessions[3].Integration.MergeCommit = "5aee01d" }},
		{"integrated-no-run", "without PR", func(c *campaignDefinition, _ *string) { c.Sessions[4].Integration.PostMergeRun = 0 }},
		{"integrated-bad-pr", "invalid PR", func(c *campaignDefinition, _ *string) { c.Sessions[5].Integration.PRs = []int{0} }},
		{"integrated-out-of-order", "before its predecessor", func(c *campaignDefinition, _ *string) {
			c.Sessions[5].ImplementationStatus, c.Sessions[5].Integration = "NOT_IMPLEMENTED", nil
		}},
		{"copied-evidence", "not after its predecessor", func(c *campaignDefinition, _ *string) { c.Sessions[3].Integration = c.Sessions[2].Integration }},
		{"reused-merge-commit", "not after its predecessor", func(c *campaignDefinition, _ *string) {
			c.Sessions[7].Integration.MergeCommit = c.Sessions[6].Integration.MergeCommit
		}},
		{"not-implemented-with-evidence", "carries integration", func(c *campaignDefinition, _ *string) { c.Sessions[7].ImplementationStatus = "NOT_IMPLEMENTED" }},
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
		{"missing-schema-metadata", "metadata", func(s *sourceRegistry, _ *string) { sourceRoute(s, "csharp").NodeTypes = nil }},
		{"changed-parser-presence", "metadata", func(s *sourceRegistry, _ *string) { *sourceRoute(s, "csharp").ParserC = false }},
		{"changed-json-presence", "metadata", func(s *sourceRegistry, _ *string) { *sourceRoute(s, "csharp").GrammarJSON = false }},
		{"missing-shared", "closure", func(s *sourceRegistry, _ *string) { sourceRoute(s, "csharp").ScannerAndShared = nil }},
		{"unsafe-shared", "shared path", func(s *sourceRegistry, _ *string) { sourceRoute(s, "csharp").ScannerAndShared[0] = "../outside" }},
		{"unsafe-grammar", "identity/path", func(s *sourceRegistry, _ *string) { sourceRoute(s, "csharp").GrammarSubdirectory = "../outside" }},
		{"false-closure", "closure", func(s *sourceRegistry, _ *string) { sourceRoute(s, "csharp").SourceClosure = "VERIFIED" }},
		{"false-support", "gap/support", func(s *sourceRegistry, _ *string) { sourceRoute(s, "csharp").FeatureSupport = "SUPPORTED" }},
		{"missing-gap", "gap/support", func(s *sourceRegistry, _ *string) { sourceRoute(s, "csharp").KnownGaps = nil }},
		{"adoption-on-unadopted-route", "closure", func(s *sourceRegistry, _ *string) {
			sourceRoute(s, "go").Feasibility = sourceRoute(s, "csharp").Feasibility
		}},
		{"missing-adoption-record", "adoption", func(s *sourceRegistry, _ *string) { sourceRoute(s, "csharp").Adoption = nil }},
		{"missing-native-run", "adoption native", func(s *sourceRegistry, _ *string) { sourceRoute(s, "csharp").Adoption.NativeEvidence.RunID = 0 }},
		{"partial-native-result", "adoption native", func(s *sourceRegistry, _ *string) {
			sourceRoute(s, "csharp").Adoption.NativeEvidence.Result = "PARTIAL"
		}},
		{"unsafe-patch-subject", "patch subject", func(s *sourceRegistry, _ *string) {
			sourceRoute(s, "csharp").Adoption.PatchSubjects[0] = "../remedy.json"
		}},
		{"bad-patched-hash", "patched file", func(s *sourceRegistry, _ *string) { sourceRoute(s, "csharp").Adoption.PatchedFiles[0].SHA256 = "00" }},
		{"missing-superseded-tsql", "superseded", func(s *sourceRegistry, _ *string) { sourceRoute(s, "tsql").SupersededCandidate = nil }},
		{"adoption-record-on-unadopted-route", "closure", func(s *sourceRegistry, _ *string) {
			sourceRoute(s, "go").Adoption = sourceRoute(s, "csharp").Adoption
		}},
		{"missing-tsql-amendment", "expectation amendment", func(s *sourceRegistry, _ *string) {
			sourceRoute(s, "tsql").Adoption.ExpectationAmendments = nil
		}},
		{"amendment-on-other-route", "expectation amendment", func(s *sourceRegistry, _ *string) {
			sourceRoute(s, "csharp").Adoption.ExpectationAmendments = []string{"src/dev/prepare-p05/remedy-expectation-amendments-r1.json"}
		}},
		// regenerated-only routes (class b) and routes with neither record (class c)
		{"adoption-replaces-regeneration", "closure", func(s *sourceRegistry, _ *string) {
			g := sourceRoute(s, "go")
			g.Adoption, g.Regeneration = sourceRoute(s, "csharp").Adoption, nil
		}},
		{"neither-adoption-nor-regeneration", "regeneration record", func(s *sourceRegistry, _ *string) { sourceRoute(s, "go").Regeneration = nil }},
		{"regeneration-on-adopted-route", "regeneration record on an adopted", func(s *sourceRegistry, _ *string) {
			sourceRoute(s, "csharp").Regeneration = sourceRoute(s, "go").Regeneration
		}},
		{"other-regeneration-decision", "regeneration record", func(s *sourceRegistry, _ *string) {
			sourceRoute(s, "go").Regeneration.Decision = "P05-GO-REMEDY-r1"
		}},
		{"regeneration-with-patched-files", "carries patches", func(s *sourceRegistry, _ *string) {
			sourceRoute(s, "go").Regeneration.PatchedFiles = []sourcePin{{Path: "grammar.js", Bytes: 1, SHA256: strings.Repeat("0", 64)}}
		}},
		{"regeneration-with-patch-subject", "carries patches", func(s *sourceRegistry, _ *string) {
			sourceRoute(s, "go").Regeneration.PatchSubjects = []string{"src/dev/prepare-p05/remedy-patches.json"}
		}},
		{"missing-output-pin", "output pin", func(s *sourceRegistry, _ *string) {
			g := sourceRoute(s, "go").Regeneration
			g.Outputs = g.Outputs[:len(g.Outputs)-1]
		}},
		{"bad-output-pin", "output pin", func(s *sourceRegistry, _ *string) { sourceRoute(s, "go").Regeneration.Outputs[2].SHA256 = "00" }},
		// C2 patch records (C2-PATCH-r1), on the patched yaml route
		{"other-c2-decision", "C2 patch record", func(s *sourceRegistry, _ *string) { sourceRoute(s, "yaml").C2Patch.Decision = regenerationDecision }},
		{"c2-record-without-subject", "C2 patch record", func(s *sourceRegistry, _ *string) { sourceRoute(s, "yaml").C2Patch.Subjects = nil }},
		{"c2-record-without-files", "C2 patch record", func(s *sourceRegistry, _ *string) { sourceRoute(s, "yaml").C2Patch.PatchedFiles = nil }},
		{"unsafe-c2-subject", "C2 patch subject", func(s *sourceRegistry, _ *string) {
			sourceRoute(s, "yaml").C2Patch.Subjects[0] = "src/dev/prepare-p05/remedy-patches.json"
		}},
		{"bad-c2-patched-hash", "C2 patched file", func(s *sourceRegistry, _ *string) { sourceRoute(s, "yaml").C2Patch.PatchedFiles[0].SHA256 = "00" }},
		{"unsafe-c2-patched-path", "C2 patched file", func(s *sourceRegistry, _ *string) {
			sourceRoute(s, "yaml").C2Patch.PatchedFiles[0].Path = "../scanner.c"
		}},
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

type reproductionRoutes struct {
	Schema    string
	Generator struct {
		Version       string
		ABI           int
		ReleaseAssets map[string]struct {
			Asset  string
			Bytes  int64
			SHA256 string
		} `json:"release_assets"`
	}
	JSRuntime struct {
		Version       string
		ReleaseAssets map[string]struct{ Asset, SHA256 string } `json:"release_assets"`
	} `json:"js_runtime"`
	Routes []struct {
		Route, Mode, Entry, Decision string
		ReferenceBasis               string `json:"reference_basis"`
		PrepareGenerated             map[string]struct {
			SHA256 string
			Bytes  int64
		} `json:"prepare_generated"`
		UpstreamParserC string   `json:"upstream_parser_c"`
		NPM             []string `json:"npm_dependencies"`
	}
	BuildPortabilityPatch struct{ State, Reason string } `json:"build_portability_patch"`
}

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// checkReproductionRoutes binds the S04 registry to the source registry: every route once,
// in order, with the source path it actually has (S04-A15) and the tool digests that gate
// generator runs per OS (S04-A18).
func checkReproductionRoutes(r reproductionRoutes, sources sourceRegistry) error {
	if r.Schema != "tsgk-reproduction-routes/r1" || r.Generator.Version != "0.27.0" || r.Generator.ABI != 15 || r.JSRuntime.Version != "24.21.0" {
		return errors.New("reproduction registry identity")
	}
	for _, os := range []string{"windows/amd64", "linux/amd64", "darwin/arm64"} {
		if a := r.Generator.ReleaseAssets[os]; !hex64.MatchString(a.SHA256) || a.Asset == "" || a.Bytes <= 0 {
			return fmt.Errorf("generator digest missing for %s", os)
		}
		if a := r.JSRuntime.ReleaseAssets[os]; !hex64.MatchString(a.SHA256) || a.Asset == "" {
			return fmt.Errorf("node digest missing for %s", os)
		}
	}
	if len(r.Routes) != len(sources.Routes) {
		return fmt.Errorf("route count %d, sources %d", len(r.Routes), len(sources.Routes))
	}
	for i, src := range sources.Routes {
		e := r.Routes[i]
		if e.Route != src.RouteID {
			return fmt.Errorf("route %d is %q, sources %q", i, e.Route, src.RouteID)
		}
		prefix := ""
		if src.GrammarSubdirectory != "." {
			prefix = src.GrammarSubdirectory + "/"
		}
		if e.Mode != "js" || src.GrammarJS == nil || !*src.GrammarJS || e.Entry != prefix+"grammar.js" {
			return fmt.Errorf("%s: JS regeneration claimed without grammar.js at %q", e.Route, e.Entry)
		}
		adopted, regenerated := src.Adoption != nil, src.Regeneration != nil
		switch {
		case adopted == regenerated:
			return fmt.Errorf("%s: a route is either adopted or regenerated-only", e.Route)
		case adopted && (e.ReferenceBasis != "PREPARE_GENERATED" || !hex64.MatchString(e.PrepareGenerated["parser.c"].SHA256) || e.Decision != ""):
			return fmt.Errorf("%s: adopted route needs its PREPARE generated parser.c reference", e.Route)
		case regenerated && (e.ReferenceBasis != "PREPARE_GENERATED" || e.Decision != regenerationDecision || len(e.PrepareGenerated) != len(src.Regeneration.Outputs)):
			return fmt.Errorf("%s: regenerated-only route needs its generated output pins", e.Route)
		}
		if regenerated {
			for _, o := range src.Regeneration.Outputs {
				if pin, ok := e.PrepareGenerated[o.Path]; !ok || pin.SHA256 != o.SHA256 || pin.Bytes != o.Bytes {
					return fmt.Errorf("%s: output pin %s disagrees with the source regeneration record", e.Route, o.Path)
				}
			}
		}
		// A C2-patched route, adopted or not, pins every output in its reproduction reference.
		if src.C2Patch != nil {
			if len(e.PrepareGenerated) != len(regeneratedOutputs) {
				return fmt.Errorf("%s: C2-patched route needs all %d output pins", e.Route, len(regeneratedOutputs))
			}
			for _, path := range regeneratedOutputs {
				if pin, ok := e.PrepareGenerated[path]; !ok || !hex64.MatchString(pin.SHA256) || pin.Bytes <= 0 {
					return fmt.Errorf("%s: C2-patched route needs all %d output pins", e.Route, len(regeneratedOutputs))
				}
			}
		}
		if (src.ParserC != nil && !*src.ParserC) != (e.UpstreamParserC == "ABSENT") {
			return fmt.Errorf("%s: upstream parser.c presence disagrees with the source registry", e.Route)
		}
	}
	if r.BuildPortabilityPatch.State != "NOT_APPLICABLE" || r.BuildPortabilityPatch.Reason != "no patch needed" {
		return errors.New("build portability patch state")
	}
	return nil
}

func TestReproductionRoutes(t *testing.T) {
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(repository(t), filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	var sources sourceRegistry
	if err := json.Unmarshal(read("src/contracts/language-sources.json"), &sources); err != nil {
		t.Fatal(err)
	}
	data := read("src/contracts/reproduction-routes.json")
	var r reproductionRoutes
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	if err := checkReproductionRoutes(r, sources); err != nil {
		t.Fatal(err)
	}
	at := func(x *reproductionRoutes, route string) int {
		for i, e := range x.Routes {
			if e.Route == route {
				return i
			}
		}
		panic("reproduction route missing: " + route)
	}
	for name, mutate := range map[string]func(*reproductionRoutes){
		"omitted route":     func(x *reproductionRoutes) { x.Routes = x.Routes[1:] },
		"json path claimed": func(x *reproductionRoutes) { x.Routes[at(x, "javascript")].Mode = "json" },
		"wrong entry":       func(x *reproductionRoutes) { x.Routes[at(x, "typescript")].Entry = "grammar.js" },
		"adopted basis":     func(x *reproductionRoutes) { x.Routes[at(x, "csharp")].ReferenceBasis = "UPSTREAM_CHECKED_IN" },
		"adopted decision":  func(x *reproductionRoutes) { x.Routes[at(x, "csharp")].Decision = regenerationDecision },
		"swift parser.c":    func(x *reproductionRoutes) { x.Routes[at(x, "swift")].UpstreamParserC = "" },
		"missing digest":    func(x *reproductionRoutes) { delete(x.Generator.ReleaseAssets, "darwin/arm64") },
		// regenerated-only routes
		"upstream basis with a regeneration": func(x *reproductionRoutes) { x.Routes[at(x, "go")].ReferenceBasis = "UPSTREAM_CHECKED_IN" },
		"missing regeneration decision":      func(x *reproductionRoutes) { x.Routes[at(x, "go")].Decision = "" },
		"missing output pin":                 func(x *reproductionRoutes) { delete(x.Routes[at(x, "go")].PrepareGenerated, "tree_sitter/array.h") },
		"output pin of another route": func(x *reproductionRoutes) {
			x.Routes[at(x, "go")].PrepareGenerated["parser.c"] = x.Routes[at(x, "python")].PrepareGenerated["parser.c"]
		},
		"output pin bytes": func(x *reproductionRoutes) {
			pin := x.Routes[at(x, "go")].PrepareGenerated["parser.c"]
			pin.Bytes++
			x.Routes[at(x, "go")].PrepareGenerated["parser.c"] = pin
		},
	} {
		var m reproductionRoutes
		json.Unmarshal(data, &m)
		mutate(&m)
		if checkReproductionRoutes(m, sources) == nil {
			t.Errorf("mutation not detected: %s", name)
		}
	}
	sourceData := read("src/contracts/language-sources.json")
	for name, mutate := range map[string]func(*sourceRegistry){
		"neither adoption nor regeneration": func(s *sourceRegistry) { sourceRoute(s, "go").Regeneration = nil },
		"adoption and regeneration":         func(s *sourceRegistry) { sourceRoute(s, "go").Adoption = sourceRoute(s, "csharp").Adoption },
		"regeneration on an adopted route":  func(s *sourceRegistry) { sourceRoute(s, "csharp").Regeneration = sourceRoute(s, "go").Regeneration },
		"source output pin changed":         func(s *sourceRegistry) { sourceRoute(s, "go").Regeneration.Outputs[2].SHA256 = strings.Repeat("0", 64) },
	} {
		var s sourceRegistry
		json.Unmarshal(sourceData, &s)
		mutate(&s)
		if checkReproductionRoutes(r, s) == nil {
			t.Errorf("source mutation not detected: %s", name)
		}
	}
	// A C2 record on the adopted csharp route demands all six output pins; csharp's reference
	// pins parser.c and parser.h only. With six well-formed pins it is accepted.
	adoptedC2 := func(pins func(*reproductionRoutes)) error {
		var m reproductionRoutes
		var s sourceRegistry
		json.Unmarshal(data, &m)
		json.Unmarshal(sourceData, &s)
		sourceRoute(&s, "csharp").C2Patch = &sourceC2Patch{Decision: c2PatchDecision, Subjects: []string{"src/dev/c2-patches/csharp.json"}}
		pins(&m)
		return checkReproductionRoutes(m, s)
	}
	six := func(x *reproductionRoutes) {
		x.Routes[at(x, "csharp")].PrepareGenerated = maps.Clone(x.Routes[at(x, "go")].PrepareGenerated)
	}
	if err := adoptedC2(six); err != nil {
		t.Fatalf("C2-patched adopted route with six pins: %v", err)
	}
	for name, pins := range map[string]func(*reproductionRoutes){
		"C2-patched adopted route with its parser.c pin only": func(*reproductionRoutes) {},
		"C2-patched adopted route with a malformed pin": func(x *reproductionRoutes) {
			six(x)
			pin := x.Routes[at(x, "csharp")].PrepareGenerated["node-types.json"]
			pin.SHA256 = "00"
			x.Routes[at(x, "csharp")].PrepareGenerated["node-types.json"] = pin
		},
	} {
		if err := adoptedC2(pins); err == nil || !strings.Contains(err.Error(), "C2-patched route needs all 6 output pins") {
			t.Errorf("%s: mutation not detected: %v", name, err)
		}
	}
}
