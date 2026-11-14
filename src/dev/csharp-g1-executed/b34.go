package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// runB34 derives or verifies src/contracts/csharp-g1-executed-b03b04-r2.json (#233 G1-3b: the expected asset r2 against the raw native
// observation), or with r1 the frozen r1 asset (the sealed expected asset), like run does for the lexical rows.
func runB34(root, rawPath string, write, r1 bool) error {
	read := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) }
	var in kit.CSharpG1B34ExecutedSources
	in.Src = kit.CSharpG1Sources{Fixtures: map[string][]byte{}}
	var err error
	if in.Src.Inventory, err = read(queryregistrycore.QualificationPin.Path); err != nil {
		return err
	}
	if in.Src.Alternatives, err = read("src/contracts/feature-alternatives.json"); err != nil {
		return err
	}
	if in.Src.FactMapping, err = read("src/contracts/fact-mapping.json"); err != nil {
		return err
	}
	for _, dir := range []string{"routes", "gaps", "n461", "queries"} {
		if in.Src.Fixtures[dir], err = read("src/testdata/native/" + dir + "/csharp.json"); err != nil {
			return err
		}
	}
	expectedPath, executedPath := kit.CSharpG1B34PathR2, kit.CSharpG1B34ExecutedPathR2
	if r1 {
		expectedPath, executedPath = kit.CSharpG1B34Path, kit.CSharpG1B34ExecutedPath
	}
	in.R2 = !r1
	if in.Expected, err = read(expectedPath); err != nil {
		return err
	}
	if in.Observation, err = read(kit.CSharpG1RoslynPathR3); err != nil {
		return err
	}
	if rawPath != "" {
		if in.Raw, err = os.ReadFile(rawPath); err != nil {
			return err
		}
	}
	path := filepath.Join(root, filepath.FromSlash(executedPath))
	if rawPath == "" {
		if write {
			return errors.New("-write needs -native")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = kit.ParseCSharpG1B34Executed(data, in)
		return err
	}
	x, err := kit.DeriveCSharpG1B34Executed(in)
	if err != nil {
		return err
	}
	data, err := kit.MarshalCSharpG1B34Executed(x)
	if err != nil {
		return err
	}
	if _, err := kit.ParseCSharpG1B34Executed(data, in); err != nil {
		return fmt.Errorf("derived asset does not verify: %w", err)
	}
	if !write {
		old, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(old, data) {
			return errors.New("tracked executed asset differs from the derivation")
		}
		return nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %d bytes %+v\n", executedPath, len(data), x.Counts)
	return nil
}
