// Command csharp-g1-executed derives src/contracts/csharp-g1-executed-b01b02-r2.json (r1 with -r1; #233 G1-3a native
// comparison) from the expected asset, the kind map, the Roslyn observation and the raw native
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

func load(root, rawPath string, r2 bool) (kit.CSharpG1ExecutedSources, error) {
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
	expPath, kmPath := kit.CSharpG1ExpectedPath, kit.CSharpG1KindMapPath
	if r2 {
		expPath, kmPath = kit.CSharpG1ExpectedPathR2, kit.CSharpG1KindMapPathR2
	}
	if in.Expected, err = read(expPath); err != nil {
		return in, err
	}
	if in.KindMap, err = read(kmPath); err != nil {
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

func run(root, rawPath string, write, r2 bool) error {
	in, err := load(root, rawPath, r2)
	if err != nil {
		return err
	}
	outPath := kit.CSharpG1ExecutedPath
	if r2 {
		outPath = kit.CSharpG1ExecutedPathR2
	}
	path := filepath.Join(root, filepath.FromSlash(outPath))
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
	fmt.Printf("wrote %s: %d bytes %+v\n", outPath, len(data), x.Counts)
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	raw := flag.String("native", "", "raw native observation written by csharp-g1-native")
	write := flag.Bool("write", false, "write the asset instead of comparing")
	r1 := flag.Bool("r1", false, "use the sealed r1 assets instead of the r2 ones")
	set := flag.String("set", "b01b02", "rows to compare: b01b02 (lexical rows) or b34 (B03 and B04)")
	flag.Parse()
	run := func() error { return run(*root, *raw, *write, !*r1) }
	if *set == "b34" {
		run = func() error { return runB34(*root, *raw, *write, *r1) }
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
