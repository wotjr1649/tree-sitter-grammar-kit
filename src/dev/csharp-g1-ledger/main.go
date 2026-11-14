// Command csharp-g1-ledger derives src/contracts/csharp-g1-ledger-r1.json (#233 G1-1) from the
// pinned repository declarations alone. It never runs native code, Roslyn or dotnet, and never
// reads native output.
//
//	go run ./src/dev/csharp-g1-ledger -root . [-r2] [-write]
//
// With -r2 it derives src/contracts/csharp-g1-ledger-r2.json (#233 G1-3b) instead, which also reads the expected and executed assets of
// csharp-B03 and csharp-B04. Ledger r1 is frozen: the tool without -r2 only compares it.
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

func run(root string, write, r2 bool) error {
	read := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) }
	src := kit.CSharpG1Sources{Fixtures: map[string][]byte{}}
	var err error
	if src.Inventory, err = read(queryregistrycore.QualificationPin.Path); err != nil {
		return err
	}
	if src.Alternatives, err = read("src/contracts/feature-alternatives.json"); err != nil {
		return err
	}
	if src.FactMapping, err = read("src/contracts/fact-mapping.json"); err != nil {
		return err
	}
	for _, dir := range []string{"routes", "gaps", "n461", "queries"} {
		if src.Fixtures[dir], err = read("src/testdata/native/" + dir + "/csharp.json"); err != nil {
			return err
		}
	}
	// the r2 expected asset (r1 plus the post-native corrections), when present, turns its EXPECTED alternatives into SEALED
	if exp, err := read(kit.CSharpG1ExpectedPathR2); err == nil {
		src.Expected = exp
	}
	// the executed asset, when present, turns the compared SEALED alternatives into EXECUTED
	if exe, err := read(kit.CSharpG1ExecutedPathR2); err == nil {
		src.Executed = exe
	}
	derive, parse, outPath := kit.DeriveCSharpG1Ledger, kit.ParseCSharpG1Ledger, kit.CSharpG1LedgerPath
	if r2 {
		derive, parse, outPath = kit.DeriveCSharpG1LedgerR2, kit.ParseCSharpG1LedgerR2, kit.CSharpG1LedgerPathR2
		if src.ExpectedB34, err = read(kit.CSharpG1B34PathR2); err != nil {
			return err
		}
		// the executed asset of the two rows is optional like the one of the lexical rows: without it nothing is EXECUTED there
		if exe, err := read(kit.CSharpG1B34ExecutedPathR2); err == nil {
			src.ExecutedB34 = exe
		}
	}
	l, err := derive(src)
	if err != nil {
		return err
	}
	data, err := kit.MarshalCSharpG1Ledger(l)
	if err != nil {
		return err
	}
	if _, err := parse(data, src); err != nil {
		return fmt.Errorf("generated ledger does not parse: %w", err)
	}
	path := filepath.Join(root, filepath.FromSlash(outPath))
	if !write {
		old, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(old, data) {
			return errors.New("tracked ledger differs from the derivation")
		}
		return nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %d bytes %+v\n", outPath, len(data), l.Counts)
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	write := flag.Bool("write", false, "write the ledger instead of comparing")
	r2 := flag.Bool("r2", false, "derive ledger r2 (with the B03 and B04 assets) instead of r1")
	flag.Parse()
	if err := run(*root, *write, *r2); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
