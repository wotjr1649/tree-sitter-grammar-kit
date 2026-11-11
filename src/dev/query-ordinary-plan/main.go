// Command query-ordinary-plan emits a source-only ordinary shard admission plan.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

const inputLimit = int64(16 << 20)

type pathsFlag []string

func (p *pathsFlag) String() string { return strings.Join(*p, ",") }
func (p *pathsFlag) Set(value string) error {
	if value == "" {
		return errors.New("path must not be empty")
	}
	*p = append(*p, value)
	return nil
}

func readFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("input is not a regular file within %d bytes: %s", limit, path)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("input exceeds %d bytes: %s", limit, path)
	}
	return data, nil
}

func readRoot(root, path string, limit int64) ([]byte, error) {
	if filepath.IsAbs(path) || strings.ContainsAny(path, "\\:") {
		return nil, fmt.Errorf("invalid root-relative input path: %s", path)
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return nil, fmt.Errorf("invalid root-relative input path: %s", path)
		}
	}
	rootAbs, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	rootAbs, err = filepath.Abs(rootAbs)
	if err != nil {
		return nil, err
	}
	pathAbs, err := filepath.EvalSymlinks(filepath.Join(rootAbs, filepath.FromSlash(path)))
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("input escapes root: %s", path)
	}
	return readFile(pathAbs, limit)
}

func run(root, witnessPath, profilePath, assetsPath, registrySHA string, registrationPaths []string, stdout, stderr io.Writer) error {
	if witnessPath == "" || profilePath == "" || assetsPath == "" || registrySHA == "" {
		return errors.New("-witness, -profile, -assets and -registry-sha256 are required")
	}
	var totalRead uint64
	readWithinBudget := func(read func(string, int64) ([]byte, error), path string) ([]byte, error) {
		remaining := uint64(256<<20) - totalRead
		limit := inputLimit
		if remaining < uint64(limit) {
			limit = int64(remaining)
		}
		data, err := read(path, limit)
		if err != nil {
			return nil, err
		}
		totalRead += uint64(len(data))
		return data, nil
	}
	readContract := func(path string) ([]byte, error) {
		return readWithinBudget(func(p string, limit int64) ([]byte, error) { return readRoot(root, p, limit) }, path)
	}
	registry, err := readContract(queryregistrycore.Path)
	if err != nil {
		return err
	}
	qualification, err := readContract(queryregistrycore.QualificationPin.Path)
	if err != nil {
		return err
	}
	slots, err := readContract(queryregistrycore.SlotPin.Path)
	if err != nil {
		return err
	}
	basis := make(map[string][]byte, 2)
	for _, path := range []string{"src/dev/query-reference/check.py", "src/internal/native/query_policy_test.go"} {
		data, err := readContract(path)
		if err != nil {
			return err
		}
		basis[path] = data
	}
	witness, err := readWithinBudget(readFile, witnessPath)
	if err != nil {
		return err
	}
	profile, err := readContract(profilePath)
	if err != nil {
		return err
	}
	assetsData, err := readContract(assetsPath)
	if err != nil {
		return err
	}
	assets, err := kit.ParseOrdinaryAssets(assetsData)
	if err != nil {
		return err
	}
	profileDoc, err := kit.ParseOrdinaryProfile(profile)
	if err != nil {
		return err
	}
	bodies := map[string][]byte{}
	readOptional := func(pin kit.OrdinaryPin) error {
		if _, ok := bodies[pin.Path]; ok {
			return nil
		}
		data, err := readContract(pin.Path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		bodies[pin.Path] = data
		return nil
	}
	for _, asset := range assets.Assets {
		if asset.State == "PINNED" {
			for _, pin := range []*kit.OrdinaryPin{asset.Pin, asset.AuthoringBasis, asset.IndependentReview} {
				if err := readOptional(*pin); err != nil {
					return err
				}
			}
		}
	}
	for _, binding := range profileDoc.Bindings {
		for _, member := range binding.Members {
			if err := readOptional(member.Pin); err != nil {
				return err
			}
		}
	}
	registrations := make([][]byte, 0, len(registrationPaths))
	for _, path := range registrationPaths {
		data, err := readContract(path)
		if err != nil {
			return err
		}
		registration, err := kit.ParseOrdinaryRegistration(data)
		if err != nil {
			return err
		}
		registrations = append(registrations, data)
		for _, query := range registration.Queries {
			if err := readOptional(query.Source); err != nil {
				return err
			}
		}
	}
	plan, err := kit.BuildOrdinaryShardPlan(kit.OrdinaryPlanRequest{
		Registry: registry, Qualification: qualification, Slots: slots, Witness: witness,
		Profile: profile, Assets: assetsData, RegistrySHA256: registrySHA, Basis: basis,
		Registrations: registrations, Bodies: bodies,
	})
	if err != nil {
		return err
	}
	data, err := kit.MarshalOrdinaryShardPlan(plan)
	if err != nil {
		return err
	}
	_, err = stdout.Write(data)
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "required=%d admitted=%d blocked=%d sql=BLOCKED/CAPACITY_UNPROVEN\n", plan.Required, plan.Admitted, plan.Blocked)
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	witness := flag.String("witness", "", "P1 source export file")
	profile := flag.String("profile", "", "ordinary profile path relative to root")
	assets := flag.String("assets", "", "ordinary asset registration path relative to root")
	registrySHA := flag.String("registry-sha256", "", "independently selected P0 registry SHA-256")
	var registrationPaths pathsFlag
	flag.Var(&registrationPaths, "registration", "ordinary registration path relative to root (repeatable)")
	flag.Parse()
	if err := run(*root, *witness, *profile, *assets, *registrySHA, registrationPaths, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
