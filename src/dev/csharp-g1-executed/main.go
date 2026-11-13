// Command csharp-g1-executed derives src/contracts/csharp-g1-executed-b01b02-r1.json (#233 G1-3a native
// comparison) from the sealed expected asset, the kind map, the Roslyn observation and the raw native
// observation written by csharp-g1-native. It runs no native code and changes none of its inputs.
//
//	go run ./src/dev/csharp-g1-executed -root . -native <raw.json> [-write]
//
// Without -write it compares the tracked asset with the derivation. With -native omitted it only
// verifies the tracked asset offline (pins, sealed expectations, evidence, dispositions, bytes).
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

func load(root, rawPath string) (kit.CSharpG1ExecutedSources, error) {
	read := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) }
	var in kit.CSharpG1ExecutedSources
	in.Src = kit.CSharpG1Sources{Fixtures: map[string][]byte{}}
	var err error
	if in.Src.Inventory, err = read(queryregistrycore.QualificationPin.Path); err != nil {
		return in, err
	}
	if in.Src.Alternatives, err = read("src/contracts/feature-alternatives.json"); err != nil {
		return in, err
	}
	if in.Src.FactMapping, err = read("src/contracts/fact-mapping.json"); err != nil {
		return in, err
	}
	for _, dir := range []string{"routes", "gaps", "n461", "queries"} {
		if in.Src.Fixtures[dir], err = read("src/testdata/native/" + dir + "/csharp.json"); err != nil {
			return in, err
		}
	}
	if in.Expected, err = read(kit.CSharpG1ExpectedPath); err != nil {
		return in, err
	}
	if in.KindMap, err = read(kit.CSharpG1KindMapPath); err != nil {
		return in, err
	}
	if in.Observation, err = read(kit.CSharpG1RoslynPath); err != nil {
		return in, err
	}
	if rawPath != "" {
		if in.Raw, err = os.ReadFile(rawPath); err != nil {
			return in, err
		}
	}
	return in, nil
}

func run(root, rawPath string, write bool) error {
	in, err := load(root, rawPath)
	if err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(kit.CSharpG1ExecutedPath))
	if rawPath == "" {
		if write {
			return errors.New("-write needs -native")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = kit.ParseCSharpG1Executed(data, in)
		return err
	}
	x, err := kit.DeriveCSharpG1Executed(in)
	if err != nil {
		return err
	}
	data, err := kit.MarshalCSharpG1Executed(x)
	if err != nil {
		return err
	}
	if _, err := kit.ParseCSharpG1Executed(data, in); err != nil {
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
	fmt.Printf("wrote %s: %d bytes %+v\n", kit.CSharpG1ExecutedPath, len(data), x.Counts)
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	raw := flag.String("native", "", "raw native observation written by csharp-g1-native")
	write := flag.Bool("write", false, "write the asset instead of comparing")
	flag.Parse()
	if err := run(*root, *raw, *write); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
