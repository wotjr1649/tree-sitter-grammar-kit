// Command ordinary-registration emits the source-only ordinary registration data for 26 routes and
// two hosts, the full shard plan and its commitment. It never runs native code and never writes an
// expected asset or an approval.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/ordinarycommit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

const fileLimit = int64(16 << 20)

// Output is every generated document of one run, keyed by its output-relative slash path.
type Output struct {
	PlanRequest kit.OrdinaryPlanRequest
	Files       map[string][]byte
	Plan        kit.OrdinaryShardPlan
	Commitment  ordinarycommit.Commitment
}

func readBounded(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > fileLimit {
		return nil, fmt.Errorf("input is not a regular file within %d bytes: %s", fileLimit, path)
	}
	return os.ReadFile(path)
}

// loadRequest reads the pinned contract files below root and the caller's P1 witness. Witness bytes
// are never trusted: the generator parses them against the pinned inventory.
func loadRequest(root, witnessPath, candidate, cohort string) (kit.OrdinaryGenerateRequest, error) {
	req := kit.OrdinaryGenerateRequest{RegistrySHA256: "ffd3552dee2d6a9b476b9de76195d2c118bb58de9079ab76dfb1e469d34aa3c8",
		Basis: map[string][]byte{}, QueryFiles: map[string][]byte{}, Candidate: candidate, SemanticCohort: cohort}
	read := func(rel string) ([]byte, error) { return readBounded(filepath.Join(root, filepath.FromSlash(rel))) }
	var err error
	if req.Registry, err = read(queryregistrycore.Path); err != nil {
		return req, err
	}
	if req.Qualification, err = read(queryregistrycore.QualificationPin.Path); err != nil {
		return req, err
	}
	if req.Slots, err = read(queryregistrycore.SlotPin.Path); err != nil {
		return req, err
	}
	for _, rel := range []string{"src/dev/query-reference/check.py", "src/internal/native/query_policy_test.go"} {
		if req.Basis[rel], err = read(rel); err != nil {
			return req, err
		}
	}
	if req.FactPack, err = read("src/contracts/fact-query-pack.json"); err != nil {
		return req, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "src", "testdata", "native", "queries"))
	if err != nil {
		return req, err
	}
	for _, entry := range entries {
		if name, ok := strings.CutSuffix(entry.Name(), ".json"); ok {
			if req.QueryFiles[name], err = read("src/testdata/native/queries/" + entry.Name()); err != nil {
				return req, err
			}
		}
	}
	if witnessPath == "" {
		return req, errors.New("-witness is required")
	}
	req.Witness, err = readBounded(witnessPath)
	return req, err
}

// generate builds every document and proves the full plan with the existing planner and commitment.
func generate(req kit.OrdinaryGenerateRequest) (Output, error) {
	out := Output{Files: map[string][]byte{}}
	g, err := kit.GenerateOrdinaryRegistration(req)
	if err != nil {
		return out, err
	}
	planReq := g.PlanRequest(req)
	out.PlanRequest = planReq
	if out.Plan, err = kit.BuildOrdinaryShardPlan(planReq); err != nil {
		return out, err
	}
	planBytes, err := kit.MarshalOrdinaryShardPlan(out.Plan)
	if err != nil {
		return out, err
	}
	var proofs []ordinarycommit.ShardProof
	if out.Commitment, proofs, err = ordinarycommit.Build(planBytes); err != nil {
		return out, err
	}
	if uint64(len(proofs)) != out.Commitment.ShardCount {
		return out, errors.New("proof count differs from the commitment shard count")
	}
	commitmentBytes, err := json.Marshal(out.Commitment)
	if err != nil {
		return out, err
	}
	out.Files["profile/tsgk-oracle-r4.json"], out.Files["profile/ordinary-assets-r1.json"] = g.Profile, g.Assets
	for key, data := range g.Registrations {
		out.Files["registrations/"+key+".json"] = data
	}
	for path, data := range g.Bodies {
		out.Files[path] = data
	}
	out.Files["plan.json"], out.Files["commitment.json"] = planBytes, append(commitmentBytes, '\n')
	return out, nil
}

// writeOutput creates dir (it must be new) and writes each file with O_EXCL.
func writeOutput(dir string, files map[string][]byte) error {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return fmt.Errorf("create new output directory: %w", err)
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		target := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err := f.Write(files[path]); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

func run(root, witness, candidate, cohort, outDir string, stderr io.Writer) error {
	if outDir == "" {
		return errors.New("-out is required")
	}
	req, err := loadRequest(root, witness, candidate, cohort)
	if err != nil {
		return err
	}
	out, err := generate(req)
	if err != nil {
		return err
	}
	if err := writeOutput(outDir, out.Files); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "shards=%d required=%d admitted=%d blocked=%d plan_sha256=%s commitment_root=%s\n", len(out.Plan.Shards), out.Plan.Required,
		out.Plan.Admitted, out.Plan.Blocked, queryregistrycore.Digest(out.Files["plan.json"]), out.Commitment.Root)
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	witness := flag.String("witness", "", "P1 source export file")
	candidate := flag.String("candidate", "", "candidate identity the documents bind to")
	cohort := flag.String("cohort", "", "semantic cohort identity the documents bind to")
	out := flag.String("out", "", "new output directory")
	flag.Parse()
	if err := run(*root, *witness, *candidate, *cohort, *out, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
