// Command csharp-g1-ledger derives src/contracts/csharp-g1-ledger-r1.json (#233 G1-1) from the
// pinned repository declarations alone. It never runs native code, Roslyn or dotnet, and never
// reads native output.
//
//	go run ./src/dev/csharp-g1-ledger -root . [-write]
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

func run(root string, write bool) error {
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
	l, err := kit.DeriveCSharpG1Ledger(src)
	if err != nil {
		return err
	}
	data, err := kit.MarshalCSharpG1Ledger(l)
	if err != nil {
		return err
	}
	if _, err := kit.ParseCSharpG1Ledger(data, src); err != nil {
		return fmt.Errorf("generated ledger does not parse: %w", err)
	}
	path := filepath.Join(root, filepath.FromSlash(kit.CSharpG1LedgerPath))
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
	fmt.Printf("wrote %s: %d bytes %+v\n", kit.CSharpG1LedgerPath, len(data), l.Counts)
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	write := flag.Bool("write", false, "write the ledger instead of comparing")
	flag.Parse()
	if err := run(*root, *write); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
