// Command csharp-g1-b34 builds and verifies the G1-3b assets of the production rows csharp-B03 and csharp-B04 (#233): the expected
// values (csharp-g1-expected-b03b04-r1.json), the position audit and the type audit. It runs no native code and reads no native
// output.
//
//	go run ./src/dev/csharp-g1-b34 -root . [-list] [-write]
//	go run ./src/dev/csharp-g1-b34 -root . -audit pos|type [-in <authored rows.json> -write] [-pins] [-grammar <dir> -spec <dir>]
//
// Without -write the tracked assets are verified (the expected asset against its derivation, an audit against the expected asset).
// -list prints every alternative with the text of its witness, the material an audit author reads. With -audit and -in the authored
// rows are completed with their comparison and verified; -write writes the canonical asset. -pins prints the ground pin table of a
// tracked audit asset as Go source. With -grammar and -spec (the directory of the pinned documents) every ground is also checked
// against the documents.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	if in.Observation, err = read(kit.CSharpG1RoslynPathR3); err != nil {
		return in, err
	}
	in.KindMap, err = read(kit.CSharpG1B34KindMapPath)
	return in, err
}

func texts(in kit.CSharpG1ExpectedSources) (map[string][]byte, error) {
	all, err := kit.CSharpG1RoslynSources(in.Src)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, s := range all {
		out[s.ID] = s.Data
	}
	return out, nil
}

func list(x kit.CSharpG1B34, tx map[string][]byte) {
	shown := map[string]bool{}
	for _, a := range x.Alternatives {
		fmt.Printf("%s\t%s\t%s\tdiscriminating=%t\t%s\t%v\t%v\n", a.ID, a.Group, a.Status, a.Discriminating, a.Form, a.TS, a.Anchors)
		for _, w := range a.Witnesses {
			t := tx[w.Source]
			if a.Group == "B" && !shown[w.Source] {
				shown[w.Source] = true
				fmt.Printf("=== SOURCE %s\n%s\n=== END\n", w.Source, t)
			}
			if w.Match == "span" {
				fmt.Printf("\twitness %s %v %s: %q\n", w.Source, w.Span, w.Roslyn, string(t[w.Span[0]:w.Span[1]]))
			} else {
				var parts []string
				for _, k := range w.Tokens {
					parts = append(parts, fmt.Sprintf("%d:%q", k[0], string(t[k[0]:k[1]])))
				}
				fmt.Printf("\twitness %s tokens %s\n", w.Source, strings.Join(parts, " "))
			}
		}
		if a.Status == "BLOCKED" {
			fmt.Printf("\tblocked %s: %s\n", a.Block, a.Reason)
		}
	}
}

func runKindMap(root string, write bool) error {
	r2, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(kit.CSharpG1KindMapPathR2)))
	if err != nil {
		return err
	}
	x, err := kit.DeriveCSharpG1KindMapR3(r2)
	if err != nil {
		return err
	}
	data, err := kit.MarshalCSharpG1KindMapR3(x)
	if err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(kit.CSharpG1KindMapPathR3))
	if write {
		return os.WriteFile(path, data, 0o644)
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := kit.ParseCSharpG1KindMapR3(old, r2); err != nil {
		return err
	}
	fmt.Printf("verified %s: %d contextual entries\n", kit.CSharpG1KindMapPathR3, len(x.Contextual))
	return nil
}

func run(root string, doList, write bool, kindmap bool, r2 bool, audit, authored string, pins bool, grammarDir, specDir string) error {
	if kindmap {
		return runKindMap(root, write)
	}
	in, err := load(root)
	if err != nil {
		return err
	}
	if r2 {
		return runR2(root, in, write, grammarDir, specDir)
	}
	path := filepath.Join(root, filepath.FromSlash(kit.CSharpG1B34Path))
	if audit != "" {
		return runAudit(root, in, audit, authored, write, pins, grammarDir, specDir)
	}
	want, err := kit.DeriveCSharpG1B34(in)
	if err != nil {
		return err
	}
	data, err := kit.MarshalCSharpG1B34(want)
	if err != nil {
		return err
	}
	if doList {
		tx, err := texts(in)
		if err != nil {
			return err
		}
		list(want, tx)
	}
	if write {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s: %d bytes %+v\n", kit.CSharpG1B34Path, len(data), want.Counts)
		return nil
	}
	if doList {
		return nil
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(old, data) {
		return fmt.Errorf("tracked %s differs from the derivation", kit.CSharpG1B34Path)
	}
	x, err := kit.ParseCSharpG1B34(old, in)
	if err != nil {
		return fmt.Errorf("the asset does not verify: %w", err)
	}
	fmt.Printf("verified %s: %+v\n", kit.CSharpG1B34Path, x.Counts)
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	doList := flag.Bool("list", false, "print every alternative with the text of its witness")
	write := flag.Bool("write", false, "write the asset instead of comparing")
	kindmap := flag.Bool("kindmap", false, "write or verify the kind map r3 (derived from r2)")
	r2 := flag.Bool("r2", false, "write or verify the r2 expected asset (the sealed r1 asset with the post-seal corrections)")
	audit := flag.String("audit", "", "pos or type: build or verify an audit asset")
	in := flag.String("in", "", "authored audit rows (JSON) to complete and verify")
	pins := flag.Bool("pins", false, "print the ground pin table of the tracked audit asset as Go source")
	grammar := flag.String("grammar", "", "directory holding the pinned grammar.js (checks the grounds)")
	spec := flag.String("spec", "", "directory holding the pinned specification files (checks the grounds)")
	flag.Parse()
	if (*grammar == "") != (*spec == "") {
		fmt.Fprintln(os.Stderr, "-grammar and -spec go together")
		os.Exit(2)
	}
	if err := run(*root, *doList, *write, *kindmap, *r2, *audit, *in, *pins, *grammar, *spec); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runR2 builds (-write) or verifies the r2 asset: the sealed r1 asset with the post-seal corrections (csharp_g1_b34_correct.go).
func runR2(root string, in kit.CSharpG1ExpectedSources, write bool, grammarDir, specDir string) error {
	previous, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(kit.CSharpG1B34Path)))
	if err != nil {
		return err
	}
	src := kit.CSharpG1B34R2Sources{R1: in, Previous: previous}
	want, err := kit.DeriveCSharpG1B34R2(src)
	if err != nil {
		return err
	}
	data, err := kit.MarshalCSharpG1B34(want)
	if err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(kit.CSharpG1B34PathR2))
	if write {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s: %d bytes %+v post_seal %+v\n", kit.CSharpG1B34PathR2, len(data), want.Counts, *want.PostSeal)
		return nil
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := kit.ParseCSharpG1B34R2(old, src); err != nil {
		return fmt.Errorf("the r2 asset does not verify: %w", err)
	}
	if grammarDir != "" {
		g, spec, err := readSpec(grammarDir, specDir)
		if err != nil {
			return err
		}
		if err := kit.CheckG1Grounds(kit.G1B34Corrections(), g, spec); err != nil {
			return err
		}
	}
	fmt.Printf("verified %s: %+v post_seal %+v\n", kit.CSharpG1B34PathR2, want.Counts, *want.PostSeal)
	return nil
}
