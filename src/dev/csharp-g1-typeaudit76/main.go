// Command csharp-g1-typeaudit76 builds and verifies src/contracts/csharp-g1-typeaudit76-r1.json, the type audit of the 76
// differential-oracle alternatives of the G1 expected asset (#233, G1TYPEAUDIT76). It reads the r2 expected asset and nothing
// else (no kind map, no native output) and runs no native code.
//
//	go run ./src/dev/csharp-g1-typeaudit76 -root . [-in <authored rows.json> -write] [-grammar <dir holding grammar.js>]
//
// Without -in it verifies the tracked asset. With -in it completes the authored rows with their comparison against the r2 asset,
// verifies the result and, with -write, writes it. With -grammar it also checks every ground against the pinned grammar.js.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func run(root, in string, write bool, grammarDir string) error {
	expected, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(kit.CSharpG1ExpectedPathR2)))
	if err != nil {
		return err
	}
	src := kit.G1TypeAudit76Sources{Expected: expected}
	path := filepath.Join(root, filepath.FromSlash(kit.CSharpG1TypeAudit76Path))
	var data []byte
	if in == "" {
		if data, err = os.ReadFile(path); err != nil {
			return err
		}
	} else {
		raw, err := os.ReadFile(in)
		if err != nil {
			return err
		}
		var rows []kit.G1TypeAudit76Row
		if err := jsonv2.Unmarshal(raw, &rows, jsonv2.RejectUnknownMembers(true)); err != nil {
			return err
		}
		x, err := kit.DeriveCSharpG1TypeAudit76(rows, src)
		if err != nil {
			return err
		}
		if data, err = kit.MarshalCSharpG1TypeAudit76(x); err != nil {
			return err
		}
	}
	x, err := kit.ParseCSharpG1TypeAudit76(data, src)
	if err != nil {
		return fmt.Errorf("the type audit does not verify: %w", err)
	}
	if grammarDir != "" {
		g, err := os.ReadFile(filepath.Join(grammarDir, "grammar.js"))
		if err != nil {
			return err
		}
		if err := kit.CheckG1TypeAudit76Grounds(x.Rows, string(g)); err != nil {
			return err
		}
	}
	if in != "" {
		if !write {
			old, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(old, data) {
				return fmt.Errorf("tracked %s differs from the derivation", kit.CSharpG1TypeAudit76Path)
			}
			return nil
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s: %d bytes %+v\n", kit.CSharpG1TypeAudit76Path, len(data), x.Counts)
		return nil
	}
	fmt.Printf("verified %s: %+v\n", kit.CSharpG1TypeAudit76Path, x.Counts)
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	in := flag.String("in", "", "authored rows (JSON list) to complete and verify")
	write := flag.Bool("write", false, "write the asset instead of comparing (with -in)")
	grammar := flag.String("grammar", "", "directory holding the pinned grammar.js (checks the grounds)")
	flag.Parse()
	if err := run(*root, *in, *write, *grammar); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
