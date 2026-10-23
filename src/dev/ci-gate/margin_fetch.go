package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

type marginRun struct {
	ID         uint64 `json:"id"`
	Attempt    uint64 `json:"run_attempt"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Head       string `json:"head_sha"`
	Branch     string `json:"head_branch"`
	Path       string `json:"path"`
	Created    string `json:"created_at"`
	Repository struct {
		ID   uint64 `json:"id"`
		Name string `json:"full_name"`
	} `json:"repository"`
	HeadRepository struct {
		ID uint64 `json:"id"`
	} `json:"head_repository"`
}
type marginHistory struct {
	Run      kit.RunIdentity  `json:"run"`
	Artifact artifactMetadata `json:"artifact"`
}
type marginPlan struct {
	Current  []artifactMetadata `json:"current"`
	History  []marginHistory    `json:"history"`
	Excluded []string           `json:"excluded"`
}

func marginRuns(expected kit.RunIdentity, metadata, history string) ([]marginRun, error) {
	b, err := readFile(filepath.Join(metadata, "run.json"), 1048576)
	var current marginRun
	if err != nil || decode(b, &current) != nil || current.Repository.ID == 0 ||
		current.Repository.Name != repository || strconv.FormatUint(current.ID, 10) != expected.RunID ||
		strconv.FormatUint(current.Attempt, 10) != expected.RunAttempt || current.Head != expected.HeadSHA ||
		current.Event != expected.Event || current.Path != ".github/workflows/foundation.yml" {
		return nil, errors.New("margin current API identity invalid")
	}
	b, err = readFile(filepath.Join(history, "cutoff.json"), 4096)
	var snapshot struct {
		Cutoff string `json:"cutoff"`
	}
	if err != nil || decode(b, &snapshot) != nil {
		return nil, errors.New("margin collection cutoff missing")
	}
	cutoff, err := time.Parse(time.RFC3339, snapshot.Cutoff)
	created, createdErr := time.Parse(time.RFC3339, current.Created)
	if err != nil || createdErr != nil || cutoff.Before(created) || cutoff.After(time.Now().UTC().Add(time.Minute)) {
		return nil, errors.New("margin cutoff timestamp invalid")
	}
	runs := []marginRun{}
	var total, listed uint64
	seen := map[uint64]bool{}
	for page := 1; page <= 20; page++ {
		b, err := readFile(filepath.Join(history, fmt.Sprintf("runs-%d.json", page)), 1048576)
		var listing struct {
			Total *uint64     `json:"total_count"`
			Runs  []marginRun `json:"workflow_runs"`
		}
		if err != nil || requiredFields(b, "workflow_runs") != nil || decode(b, &listing) != nil || listing.Total == nil || *listing.Total > 1000 ||
			page > 1 && *listing.Total != total {
			return nil, errors.New("HISTORY_WINDOW_INCOMPLETE: workflow run listing")
		}
		total = *listing.Total
		if listed > total || uint64(len(listing.Runs)) != min(uint64(50), total-listed) {
			return nil, errors.New("HISTORY_WINDOW_INCOMPLETE: workflow run page")
		}
		listed += uint64(len(listing.Runs))
		for _, r := range listing.Runs {
			if r.ID == 0 || seen[r.ID] {
				return nil, errors.New("duplicate history API run")
			}
			seen[r.ID] = true
			if r.Repository.ID != current.Repository.ID || r.Repository.Name != repository ||
				r.Path != ".github/workflows/foundation.yml" {
				return nil, errors.New("untrusted margin history workflow")
			}
			// exclude_pull_requests trims PR references, not workflow-run events.
			if !slices.Contains([]string{"push", "schedule", "workflow_dispatch"}, r.Event) {
				continue
			}
			if r.HeadRepository.ID != current.Repository.ID || r.Branch != "main" {
				return nil, errors.New("untrusted margin history source")
			}
			created, err := time.Parse(time.RFC3339, r.Created)
			if err != nil || !hex40.MatchString(r.Head) || r.Attempt == 0 {
				return nil, errors.New("margin history run malformed")
			}
			if !slices.Contains([]string{"completed", "queued", "in_progress", "waiting", "requested", "pending"}, r.Status) {
				return nil, errors.New("margin history run status invalid")
			}
			if r.ID != current.ID && r.Status == "completed" && !created.After(cutoff) &&
				!created.Before(cutoff.Add(-30*24*time.Hour)) {
				runs = append(runs, r)
			}
		}
		if listed == total {
			break
		}
	}
	if listed != total {
		return nil, errors.New("HISTORY_WINDOW_INCOMPLETE")
	}
	slices.SortFunc(runs, func(a, b marginRun) int {
		if a.Created != b.Created {
			return strings.Compare(b.Created, a.Created)
		}
		if a.ID > b.ID {
			return -1
		}
		if a.ID < b.ID {
			return 1
		}
		return 0
	})
	return runs[:min(60, len(runs))], nil
}

func marginIdentity(r marginRun) kit.RunIdentity {
	return kit.RunIdentity{Schema: kit.RunIdentitySchema, Repository: repository, Workflow: "Foundation",
		RunID: strconv.FormatUint(r.ID, 10), RunAttempt: strconv.FormatUint(r.Attempt, 10),
		Event: r.Event, SHA: r.Head, HeadSHA: r.Head, Checkout: r.Head, Job: "integration-gate", EvidenceMode: "NEW_RUN"}
}

func marginListing(path string) ([]artifactMetadata, error) {
	b, err := readFile(path, 1048576)
	var listing struct {
		Total     *uint64            `json:"total_count"`
		Artifacts []artifactMetadata `json:"artifacts"`
	}
	if err != nil || requiredFields(b, "artifacts") != nil || decode(b, &listing) != nil || listing.Total == nil ||
		*listing.Total != uint64(len(listing.Artifacts)) || *listing.Total > 100 {
		return nil, errors.New("margin artifact listing incomplete")
	}
	return listing.Artifacts, nil
}

func selectMarginArtifact(rows []artifactMetadata, name string, expected kit.RunIdentity) (artifactMetadata, bool, error) {
	var found []artifactMetadata
	for _, a := range rows {
		if a.Name == name {
			found = append(found, a)
		}
	}
	if len(found) > 1 {
		return artifactMetadata{}, false, errors.New("duplicate margin artifact")
	}
	if len(found) == 0 {
		return artifactMetadata{}, false, nil
	}
	a := found[0]
	if a.Expired != nil && *a.Expired {
		return a, false, nil
	}
	if !validArtifact(a, expected) || a.Size > 262144 {
		return a, false, errors.New("margin artifact metadata invalid")
	}
	return a, true, nil
}

func makeMarginPlan(expected kit.RunIdentity, metadata, history string) (marginPlan, error) {
	result := marginPlan{Current: []artifactMetadata{}, History: []marginHistory{}, Excluded: []string{}}
	// Reuse full current-run listing validation, including all pages and 5 core ZIPs.
	corePlan, err := planFiles(expected, metadata)
	if err != nil {
		return result, err
	}
	runs, err := marginRuns(expected, metadata, history)
	if err != nil {
		return result, err
	}
	var rows []artifactMetadata
	for page := 1; page <= 10; page++ {
		b, err := readFile(filepath.Join(metadata, fmt.Sprintf("listing-%d.json", page)), 1048576)
		if err != nil {
			break
		} // planFiles above already required every original page.
		var listing struct {
			Artifacts []artifactMetadata `json:"artifacts"`
		}
		if decode(b, &listing) != nil {
			return result, errors.New("margin current listing invalid")
		}
		rows = append(rows, listing.Artifacts...)
	}
	ids := map[uint64]bool{}
	for _, a := range corePlan.Artifacts {
		ids[a.ID] = true
	}
	for _, j := range marginJobs {
		a, ok, err := selectMarginArtifact(rows, configName(j, expected), expected)
		if err != nil || !ok || ids[a.ID] {
			return result, errors.New("margin producer artifact missing or duplicate")
		}
		ids[a.ID] = true
		result.Current = append(result.Current, a)
	}
	for _, r := range runs {
		rows, err := marginListing(filepath.Join(history, fmt.Sprintf("artifacts-%d.json", r.ID)))
		if err != nil {
			return result, err
		}
		expected := marginIdentity(r)
		name := "ci-margin-" + expected.RunID + "-" + expected.RunAttempt
		a, ok, err := selectMarginArtifact(rows, name, expected)
		if err != nil {
			return result, err
		}
		if !ok {
			reason := "NO_LATEST_ATTEMPT_REPORT"
			if a.Expired != nil && *a.Expired {
				reason = "EXPIRED_REPORT"
			}
			result.Excluded = append(result.Excluded, expected.RunID+":"+reason)
			continue
		}
		if ids[a.ID] {
			return result, errors.New("margin artifact ID reused")
		}
		ids[a.ID] = true
		result.History = append(result.History, marginHistory{expected, a})
	}
	return result, nil
}

func readMarginZIP(ctx context.Context, a artifactMetadata, root, out, name string) ([]byte, error) {
	b, err := readFile(filepath.Join(root, a.Name+".zip"), 262144)
	if err != nil || int64(len(b)) != a.Size || "sha256:"+digest(b) != a.Digest {
		return nil, errors.New("margin ZIP digest mismatch")
	}
	dir := filepath.Join(out, a.Name)
	files, err := extract(ctx, filepath.Join(root, a.Name+".zip"), dir, artifactSpec{a.Name, 1, 262144})
	if err != nil || len(files) != 1 {
		return nil, errors.New("margin ZIP shape invalid")
	}
	if _, ok := files[name]; !ok {
		return nil, errors.New("margin ZIP file absent")
	}
	return readFile(filepath.Join(dir, name), 262144)
}
