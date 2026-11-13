// Command csharp-g1-classify is the dry run of the G1-3b classification (#233): it sorts the production alternatives of csharp-B03 and
// csharp-B04 into the literal anchors (A), the Roslyn structures (B) and the blocked ones (C) from the pinned declarations, the Roslyn
// observation r3 and the kind map r2. It writes no asset, runs no native code, Roslyn or dotnet, and reads no native output.
//
//	go run ./src/dev/csharp-g1-classify -root . [-list] [-kinds]
//
// The counts are printed always; -list adds one line per alternative, -kinds the kind map entries the B rules name.
package main

import (
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
	if in.Observation, err = read(kit.CSharpG1RoslynPathR3); err != nil {
		return in, err
	}
	in.KindMap, err = read(kit.CSharpG1KindMapPathR2)
	return in, err
}

func run(root string, list, kinds bool) error {
	in, err := load(root)
	if err != nil {
		return err
	}
	view, err := kit.G1ClassifyView(in)
	if err != nil {
		return err
	}
	rep, err := kit.ClassifyCSharpG1(view)
	if err != nil {
		return err
	}
	c := rep.Counts
	fmt.Printf("alternatives %d: A %d (discriminating %d, weak %d), B %d (discriminating %d, weak %d), C %d\n", c.Alternatives, c.A, c.ADiscr, c.AWeak, c.B, c.BDiscr, c.BWeak, c.C)
	for _, r := range c.Rows {
		fmt.Printf("row %s: %d\n", r.Key, r.Count)
	}
	for _, b := range c.Blocked {
		fmt.Printf("blocked %s: %d\n", b.Key, b.Count)
	}
	if list {
		for _, r := range rep.Results {
			fmt.Println(r)
		}
	}
	if kinds {
		for _, k := range rep.Kinds {
			fmt.Printf("kind %s\t%s\t%s\t%s\t%v\n", k.Kind, k.Status, k.Map, k.Reason, k.Alternatives)
		}
	}
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	list := flag.Bool("list", false, "list the classification of every alternative")
	kinds := flag.Bool("kinds", false, "list the Roslyn kinds the B rules name with their kind map entry")
	flag.Parse()
	if err := run(*root, *list, *kinds); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
