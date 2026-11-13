// Command csharp-g1-correct derives the r2 kind map (src/contracts/csharp-g1-kind-map-r2.json) and the r2
// expected asset (src/contracts/csharp-g1-expected-b01b02-r2.json) of #233 G1-3a: the sealed r1 files
// plus the post-native corrections authored in kit/csharp_g1_correct.go. It runs no native code,
// reads no native output, and never changes an r1 file.
//
//	go run ./src/dev/csharp-g1-correct -root . [-write] [-grammar <tree-sitter-c-sharp checkout>] [-spec <dir with lexical-structure.md and expressions.md>]
//
// Without -write it compares the tracked r2 files with the derivation. With -grammar and -spec it also
// checks that every ground of every correction quotes the pinned grammar.js (its SHA-256 must be the
// kind map's pin) and the specification files.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

func load(root string) (kit.CSharpG1ExpectedSources, error) {
	read := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) }
	in := kit.CSharpG1ExpectedSources{Src: kit.CSharpG1Sources{Fixtures: map[string][]byte{}}}
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
	if in.Observation, err = read(kit.CSharpG1RoslynPath); err != nil {
		return in, err
	}
	in.KindMap, err = read(kit.CSharpG1KindMapPath)
	return in, err
}

func checkGrounds(grammarDir, specDir string, kmGrammarSHA string) error {
	g, err := os.ReadFile(filepath.Join(grammarDir, "grammar.js"))
	if err != nil {
		return err
	}
	if sum := sha256.Sum256(g); hex.EncodeToString(sum[:]) != kmGrammarSHA {
		return errors.New("grammar.js is not the pinned one")
	}
	spec := map[string]string{}
	for _, name := range []string{"lexical-structure.md", "expressions.md"} {
		d, err := os.ReadFile(filepath.Join(specDir, name))
		if err != nil {
			return err
		}
		spec[name] = string(d)
	}
	if err := kit.CheckG1Grounds(kit.G1ExpectedCorrections(), string(g), spec); err != nil {
		return err
	}
	return kit.CheckG1Grounds(kit.G1KindMapCorrections(), string(g), spec)
}

func run(root string, write bool, grammarDir, specDir string) error {
	in, err := load(root)
	if err != nil {
		return err
	}
	read := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) }
	prevExp, err := read(kit.CSharpG1ExpectedPath)
	if err != nil {
		return err
	}
	obs, err := kit.CSharpG1RoslynVocabulary(in.Observation)
	if err != nil {
		return err
	}
	km, err := kit.DeriveCSharpG1KindMapR2(kit.CSharpG1KindMapR2Sources{Previous: in.KindMap, Observed: obs})
	if err != nil {
		return err
	}
	if grammarDir != "" {
		if err := checkGrounds(grammarDir, specDir, km.Grammar.GrammarJS.SHA256); err != nil {
			return err
		}
	}
	kmData, err := kit.MarshalCSharpG1KindMap(km)
	if err != nil {
		return err
	}
	if _, err := kit.ParseCSharpG1KindMapR2(kmData, kit.CSharpG1KindMapR2Sources{Previous: in.KindMap, Observed: obs}); err != nil {
		return fmt.Errorf("derived kind map does not verify: %w", err)
	}
	r2in := kit.CSharpG1ExpectedR2Sources{R1: in, Previous: prevExp, KindMap: kmData}
	x, err := kit.DeriveCSharpG1ExpectedR2(r2in)
	if err != nil {
		return err
	}
	expData, err := kit.MarshalCSharpG1Expected(x)
	if err != nil {
		return err
	}
	if _, err := kit.ParseCSharpG1ExpectedR2(expData, r2in); err != nil {
		return fmt.Errorf("derived expected asset does not verify: %w", err)
	}
	for _, f := range []struct {
		rel  string
		data []byte
	}{{kit.CSharpG1KindMapPathR2, kmData}, {kit.CSharpG1ExpectedPathR2, expData}} {
		path := filepath.Join(root, filepath.FromSlash(f.rel))
		if !write {
			old, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(old, f.data) {
				return fmt.Errorf("tracked %s differs from the derivation", f.rel)
			}
			continue
		}
		if err := os.WriteFile(path, f.data, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s: %d bytes\n", f.rel, len(f.data))
	}
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	write := flag.Bool("write", false, "write the r2 files instead of comparing")
	grammar := flag.String("grammar", "", "pinned tree-sitter-c-sharp checkout (checks the grammar grounds)")
	spec := flag.String("spec", "", "directory holding the pinned lexical-structure.md and expressions.md (checks the spec grounds)")
	flag.Parse()
	if (*grammar == "") != (*spec == "") {
		fmt.Fprintln(os.Stderr, "-grammar and -spec go together")
		os.Exit(2)
	}
	if err := run(*root, *write, *grammar, *spec); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
