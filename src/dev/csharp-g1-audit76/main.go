// Command csharp-g1-audit76 builds and verifies src/contracts/csharp-g1-audit76-r1.json, the audit of the 76
// differential-oracle alternatives of the r2 expected asset (#233 G1AUDIT76). It runs no native code and reads no
// native output.
//
//	go run ./src/dev/csharp-g1-audit76 -root . [-in <authored rows.json> -write] [-grammar <dir> -spec <dir>]
//
// Without -in it verifies the tracked asset against the tracked r2 expected asset and the pinned sources. With -in
// it completes the authored rows (instances, node, grounds) with their comparison against r2, verifies the result,
// and with -write writes the canonical asset. With -grammar and -spec (the directory of the pinned grammar.js and
// the pinned specification files) it also checks every ground against the documents.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

func sources(root string) (kit.G1Audit76Input, error) {
	read := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) }
	var out kit.G1Audit76Input
	src := kit.CSharpG1Sources{Fixtures: map[string][]byte{}}
	var err error
	if src.Inventory, err = read(queryregistrycore.QualificationPin.Path); err != nil {
		return out, err
	}
	if src.Alternatives, err = read("src/contracts/feature-alternatives.json"); err != nil {
		return out, err
	}
	if src.FactMapping, err = read("src/contracts/fact-mapping.json"); err != nil {
		return out, err
	}
	for _, dir := range []string{"routes", "gaps", "n461", "queries"} {
		if src.Fixtures[dir], err = read("src/testdata/native/" + dir + "/csharp.json"); err != nil {
			return out, err
		}
	}
	expected, err := read(kit.CSharpG1ExpectedPathR2)
	if err != nil {
		return out, err
	}
	all, err := kit.CSharpG1RoslynSources(src)
	if err != nil {
		return out, err
	}
	texts := map[string][]byte{}
	for _, s := range all {
		texts[s.ID] = s.Data
	}
	return kit.G1Audit76View(expected, texts)
}

func run(root, in string, write, pins bool, grammarDir, specDir string) error {
	path := filepath.Join(root, filepath.FromSlash(kit.CSharpG1Audit76Path))
	if pins {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		x, err := kit.DecodeCSharpG1Audit76(data)
		if err != nil {
			return err
		}
		fmt.Print(kit.G1GroundPinSource("g1a76GroundPins", kit.G1Audit76GroundPins(x.Rows)))
		return nil
	}
	src, err := sources(root)
	if err != nil {
		return err
	}
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
		var rows []kit.G1Audit76Row
		if err := jsonv2.Unmarshal(raw, &rows, jsonv2.RejectUnknownMembers(true)); err != nil {
			return err
		}
		x, err := kit.DeriveCSharpG1Audit76(rows, src)
		if err != nil {
			return err
		}
		if data, err = kit.MarshalCSharpG1Audit76(x); err != nil {
			return err
		}
	}
	x, err := kit.ParseCSharpG1Audit76(data, src)
	if err != nil {
		return fmt.Errorf("the audit does not verify: %w", err)
	}
	if grammarDir != "" {
		g, err := os.ReadFile(filepath.Join(grammarDir, "grammar.js"))
		if err != nil {
			return err
		}
		spec := map[string]string{}
		for _, name := range kit.G1SpecFiles {
			d, err := os.ReadFile(filepath.Join(specDir, name))
			if err != nil {
				return err
			}
			spec[name] = string(d)
		}
		if err := kit.CheckG1Audit76Grounds(x.Rows, string(g), spec); err != nil {
			return err
		}
	}
	if in != "" {
		if !write {
			old, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(old, data) {
				return fmt.Errorf("tracked %s differs from the derivation", kit.CSharpG1Audit76Path)
			}
			return nil
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s: %d bytes %+v\n", kit.CSharpG1Audit76Path, len(data), x.Counts)
		return nil
	}
	fmt.Printf("verified %s: %+v\n", kit.CSharpG1Audit76Path, x.Counts)
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	in := flag.String("in", "", "authored rows (JSON list) to complete and verify")
	write := flag.Bool("write", false, "write the asset instead of comparing (with -in)")
	pins := flag.Bool("pins", false, "print the ground pin table of the tracked asset (Go source for csharp_g1_ground_pins.go) and stop")
	grammar := flag.String("grammar", "", "directory holding the pinned grammar.js (checks the grounds)")
	spec := flag.String("spec", "", "directory holding the pinned specification files (checks the grounds)")
	flag.Parse()
	if (*grammar == "") != (*spec == "") {
		fmt.Fprintln(os.Stderr, "-grammar and -spec go together")
		os.Exit(2)
	}
	if err := run(*root, *in, *write, *pins, *grammar, *spec); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
