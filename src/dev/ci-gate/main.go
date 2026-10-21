// ci-gate verifies one Foundation cohort; downloaded evidence is never executable input.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const repository = "wotjr1649/tree-sitter-grammar-kit"

var hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)
var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
var positive = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
var platforms = []struct{ os, id string }{{"windows", "windows-amd64"}, {"linux", "linux-amd64"}, {"darwin", "darwin-arm64"}}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func decode(data []byte, value any) error {
	d := jsontext.NewDecoder(bytes.NewReader(data))
	for n := 0; ; n++ {
		_, err := d.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || d.StackDepth() > 32 || n >= 1000000 {
			return errors.New("invalid or excessive JSON")
		}
	}
	if jsonv2.Unmarshal(data, value) != nil {
		return errors.New("invalid JSON shape")
	}
	return nil
}

func readFile(path string, limit int64) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, errors.New("required regular file unavailable")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("required file unavailable")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !os.SameFile(before, st) || !st.Mode().IsRegular() || st.Size() > limit {
		return nil, errors.New("invalid file or size")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, errors.New("file read limit")
	}
	return b, nil
}

func expectedIdentity(path string) (kit.RunIdentity, error) {
	var r kit.RunIdentity
	b, err := readFile(path, 65536)
	if err != nil {
		return r, err
	}
	if err = decode(b, &r); err != nil {
		return r, err
	}
	if r.Schema != kit.RunIdentitySchema || r.Repository != repository || r.Workflow != "Foundation" ||
		!positive.MatchString(r.RunID) || !positive.MatchString(r.RunAttempt) || !hex40.MatchString(r.SHA) ||
		!hex40.MatchString(r.HeadSHA) || r.Checkout != r.SHA || r.EvidenceMode != "NEW_RUN" || r.Job != "integration-gate" ||
		!slices.Contains([]string{"pull_request", "push", "schedule", "workflow_dispatch"}, r.Event) ||
		(r.Event != "pull_request" && r.HeadSHA != r.SHA) {
		return r, errors.New("invalid workflow expectation")
	}
	return r, nil
}

func sameRun(got *kit.RunIdentity, want kit.RunIdentity) bool {
	return got != nil && got.Schema == kit.RunIdentitySchema && got.Repository == want.Repository && got.Workflow == want.Workflow &&
		got.RunID == want.RunID && got.RunAttempt == want.RunAttempt && got.Event == want.Event && got.SHA == want.SHA &&
		got.HeadSHA == want.HeadSHA && got.Checkout == want.Checkout && got.EvidenceMode == "NEW_RUN"
}

func checkNeeds(data []byte) error {
	var needs map[string]struct {
		Result  string            `json:"result"`
		Outputs map[string]string `json:"outputs"`
	}
	if decode(data, &needs) != nil || len(needs) != 6 {
		return errors.New("required producer results unavailable")
	}
	for _, name := range []string{"foundation", "native-prepare", "native-routes", "native-routes-macos", "large-api", "qualification"} {
		if needs[name].Result != "success" {
			return errors.New("required producer did not succeed")
		}
	}
	return nil
}

type artifactSpec struct {
	name  string
	files int
	bytes int64
}

func artifacts(r kit.RunIdentity) []artifactSpec {
	var a []artifactSpec
	for _, p := range platforms {
		a = append(a, artifactSpec{fmt.Sprintf("s05-routes-%s-%s-%s", p.os, r.RunID, r.RunAttempt), 10000, int64(kit.QualificationLimits().TotalBytes)})
	}
	a = append(a, artifactSpec{"s08-qualification-" + r.RunID + "-" + r.RunAttempt, 1, 16777216})
	return append(a, artifactSpec{"large-api-" + r.RunID + "-" + r.RunAttempt, 2, 16842752})
}

func outside(path, root string) bool {
	p, e1 := filepath.Abs(path)
	r, e2 := filepath.Abs(root)
	if e1 != nil || e2 != nil {
		return false
	}
	rel, err := filepath.Rel(r, p)
	return err == nil && (rel == ".." || len(rel) > 3 && rel[:3] == ".."+string(os.PathSeparator))
}

func verify(ctx context.Context, expected kit.RunIdentity, root string) (map[string]any, error) {
	if err := checkDownloaded(root, expected); err != nil {
		return nil, err
	}
	var hosts []kit.QualifyHost
	for _, p := range platforms {
		hosts = append(hosts, kit.QualifyHost{Platform: p.id, Root: filepath.Join(root, "s05-routes-"+p.os+"-"+expected.RunID+"-"+expected.RunAttempt)})
	}
	inv, err := readFile("src/contracts/qualification-c1.json", 16777216)
	if err != nil {
		return nil, err
	}
	inventory, err := kit.ParseQualificationInventory(inv)
	if err != nil {
		return nil, errors.New("qualification inventory invalid")
	}
	q, qerr := kit.Qualify(ctx, kit.QualifyRequest{Inventory: inv, Candidate: expected.Checkout, Hosts: hosts})
	if qerr != nil {
		return nil, errors.New("qualification recomputation failed")
	}
	if err := checkQualification(q, expected, inventory); err != nil {
		return nil, err
	}
	hosted, err := readFile(filepath.Join(root, "s08-qualification-"+expected.RunID+"-"+expected.RunAttempt, "qualification.json"), 16777216)
	if err != nil {
		return nil, err
	}
	var shape kit.QualificationResult
	if decode(hosted, &shape) != nil {
		return nil, errors.New("hosted qualification JSON invalid")
	}
	canonical, err := json.Marshal(q)
	if err != nil || !bytes.Equal(append(canonical, '\n'), hosted) {
		return nil, errors.New("hosted qualification differs from raw evidence")
	}
	large, err := verifyLarge(root, expected)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, errors.New("integration verification deadline")
	}
	return map[string]any{"schema": "tsgk-ci-integration-gate/r1", "assessment": "PASS", "run": expected, "cells": len(q.Cells), "obligations": q.Totals.Obligations.Obligations, "qualification_sha256": digest(hosted), "large": large}, nil
}

func checkQualification(q kit.QualificationResult, expected kit.RunIdentity, inventory kit.QualificationInventory) error {
	want := map[string]map[string]bool{}
	obligationCount := 0
	for _, r := range inventory.Routes {
		for _, p := range inventory.Platforms {
			obligations := map[string]bool{}
			for _, requirement := range r.Requirements {
				for _, kind := range requirement.Kinds {
					obligations[requirement.Row+"/"+kind] = true
				}
			}
			want[r.Route+"/"+p.ID] = obligations
			obligationCount += len(obligations)
		}
	}
	if q.ResultSchema != kit.QualificationResultSchema || !sameRun(q.Run, expected) || q.Candidate != expected.Checkout ||
		q.Assessment != kit.AssessPass || q.Completeness != "PASS" || q.MechanismGate != "PASS" || q.SupportClaim != "SUPPORTED" || len(q.Findings) != 0 ||
		len(q.Cells) != 78 || q.Totals.Cells != 78 || len(q.Totals.Status) != 1 || q.Totals.Status["PASS"] != 78 ||
		len(want) != 78 || obligationCount <= 0 || q.Totals.Obligations.Obligations != obligationCount || q.Totals.Obligations.Pass != q.Totals.Obligations.Obligations ||
		q.Totals.Obligations.Fail != 0 || q.Totals.Obligations.Blocked != 0 || q.Totals.Obligations.NotCovered != 0 || len(q.PlatformClaims) != 3 {
		return errors.New("qualification gate incomplete or unsuccessful")
	}
	seen := map[string]bool{}
	for _, c := range q.Cells {
		key := c.Route + "/" + c.Platform
		expectedObligations, registered := want[key]
		if !registered || seen[key] || c.Status != "PASS" || c.Mechanism != "PASS" || c.Requirement != "PASS" || c.Comparison != "PASS" || c.Checks != "PASS" || len(c.CheckFailures) != 0 ||
			len(c.Obligations) != len(expectedObligations) || c.Counts.Obligations != len(expectedObligations) || c.Counts.Pass != len(expectedObligations) || c.Counts.Fail != 0 || c.Counts.Blocked != 0 || c.Counts.NotCovered != 0 {
			return errors.New("invalid qualification cell")
		}
		seen[key] = true
		seenObligations := map[string]bool{}
		for _, o := range c.Obligations {
			id := o.Row + "/" + o.Kind
			if o.Result != "PASS" || !expectedObligations[id] || seenObligations[id] || len(o.Cases) == 0 {
				return errors.New("unsatisfied qualification obligation")
			}
			seenObligations[id] = true
		}
	}
	for _, p := range platforms {
		if q.PlatformClaims[p.id] != "SUPPORTED" {
			return errors.New("unsupported platform")
		}
	}
	return nil
}

func main() { os.Exit(run(os.Args[1:], os.Stderr)) }

func run(args []string, stderr io.Writer) int {
	if len(args) == 0 || !slices.Contains([]string{"plan", "unpack", "verify", "config", "margin-settled", "margin-discover", "margin-plan", "margin"}, args[0]) {
		fmt.Fprintln(stderr, "ci-gate: expected plan, unpack, verify, config, margin-settled, margin-discover, margin-plan or margin")
		return 2
	}
	fs := flag.NewFlagSet("ci-gate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	expectPath := fs.String("expected", "", "workflow-owned run identity")
	root := fs.String("artifacts", "", "new download root / existing evidence root")
	needs := fs.String("needs", "", "workflow-owned needs JSON")
	out := fs.String("out", "", "new verification result outside evidence")
	metadata := fs.String("metadata", "", "original bounded GitHub metadata directory")
	plan := fs.String("plan", "", "workflow-owned download plan outside evidence")
	history := fs.String("history", "", "bounded margin metadata and ZIP root")
	core := fs.String("core", "", "successful current-run core result")
	compiler := fs.String("compiler", "", "selected producer compiler")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *expectPath == "" {
		return 2
	}
	if args[0] == "config" {
		if *out == "" {
			return 2
		}
		if err := saveMarginConfig(*expectPath, *compiler, *out); err != nil {
			fmt.Fprintln(stderr, "ci-gate:", err)
			return 1
		}
		return 0
	}
	expected, err := expectedIdentity(*expectPath)
	if err != nil {
		fmt.Fprintln(stderr, "ci-gate:", err)
		return 1
	}
	if args[0] == "margin-settled" {
		if *history == "" {
			return 2
		}
		jobs, err := readFile(filepath.Join(*history, "metadata/jobs.json"), 1048576)
		if err != nil {
			fmt.Fprintln(stderr, "ci-gate:", err)
			return 1
		}
		if !marginJobsSettled(jobs) {
			fmt.Fprintln(stderr, "ci-gate: jobs API still shows a measured job or step as running")
			return 3
		}
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 365*time.Second)
	defer cancel()
	if args[0] == "margin-discover" || args[0] == "margin-plan" {
		if *metadata == "" || *history == "" || *out == "" || !outside(*out, *metadata) || !outside(*out, *history) {
			return 2
		}
		if args[0] == "margin-discover" {
			var runs []marginRun
			runs, err = marginRuns(expected, *metadata, *history)
			if err == nil {
				err = writeNewJSON(*out, runs, 65536)
			}
		} else {
			var result marginPlan
			result, err = makeMarginPlan(expected, *metadata, *history)
			if err == nil {
				err = writeNewJSON(*out, result, 131072)
			}
		}
	} else if args[0] == "margin" {
		if *metadata == "" || *history == "" || *root == "" || *core == "" || *out == "" ||
			!outside(*out, *root) || !outside(*out, *metadata) || !outside(*out, *history) {
			return 2
		}
		err = verifyMargins(ctx, expected, *root, *metadata, *history, *core, *out)
	} else if args[0] == "plan" {
		if *metadata == "" || *out == "" || !outside(*out, *metadata) {
			return 2
		}
		var value downloadPlan
		value, err = planFiles(expected, *metadata)
		if err == nil {
			err = writeNewJSON(*out, value, 65536)
		}
	} else if *root == "" || !outside(*expectPath, *root) {
		return 2
	} else if args[0] == "unpack" {
		if *plan == "" || !outside(*plan, *root) {
			return 2
		}
		err = unpack(ctx, expected, *root, *plan)
	} else {
		if *needs == "" || *out == "" || !outside(*needs, *root) || !outside(*out, *root) {
			return 2
		}
		var data []byte
		data, err = readFile(*needs, 65536)
		if err == nil {
			err = checkNeeds(data)
		}
		var result map[string]any
		if err == nil {
			result, err = verify(ctx, expected, *root)
		}
		if err == nil {
			data, err = json.Marshal(result)
			if err == nil {
				var f *os.File
				f, err = os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
				if err == nil {
					_, err = f.Write(append(data, '\n'))
					closeErr := f.Close()
					if err == nil {
						err = closeErr
					}
				}
			}
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "ci-gate:", err)
		return 1
	}
	fmt.Fprintln(stderr, "ci-gate: PASS")
	return 0
}
