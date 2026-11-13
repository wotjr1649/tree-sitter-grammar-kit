// Command csharp-g1-expected derives src/contracts/csharp-g1-expected-b01b02-r1.json (#233 G1-3a)
// from the pinned repository declarations, the pinned Roslyn observation and the kind map. It runs no
// native code, Roslyn or dotnet, and reads no native output.
//
//	go run ./src/dev/csharp-g1-expected -root . [-write] [-picks] [-summary]
//	go run ./src/dev/csharp-g1-expected -root . -metamorphic-input <file>
//	go run ./src/dev/csharp-g1-expected -root . -metamorphic-results <Roslyn output> [-write]
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	jsonv2 "encoding/json/v2"

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

func run(root string, write, picks, summary bool, metaIn, metaRes string) error {
	in, err := load(root)
	if err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(kit.CSharpG1ExpectedPath))
	if picks {
		s, err := kit.CSharpG1AuditSamples(in)
		if err != nil {
			return err
		}
		for _, p := range s {
			fmt.Printf("%s\t%s\n", p[0], p[1])
		}
		return nil
	}
	if metaIn != "" {
		vs, err := kit.G1MetaVariantSources(in)
		if err != nil {
			return err
		}
		data, err := kit.MarshalCSharpG1RoslynInput(vs)
		if err != nil {
			return err
		}
		return os.WriteFile(metaIn, data, 0o644)
	}
	// the sealed digests come from the tracked asset, or from a fresh Roslyn output
	if old, err := os.ReadFile(path); err == nil {
		var prev kit.CSharpG1Expected
		if err := jsonv2.Unmarshal(old, &prev); err == nil {
			in.Sealed = prev.Metamorphic.Results()
		}
	}
	if metaRes != "" {
		data, err := os.ReadFile(metaRes)
		if err != nil {
			return err
		}
		var out kit.CSharpG1RoslynObserved
		if err := jsonv2.Unmarshal(data, &out); err != nil {
			return err
		}
		in.Sealed = kit.G1ExpMetaResults{}
		for _, s := range out.Sources {
			in.Sealed[s.ID] = kit.CSharpG1StructureDigest(out.Kinds, s)
		}
	}
	x, err := kit.DeriveCSharpG1Expected(in)
	if err != nil {
		return err
	}
	data, err := kit.MarshalCSharpG1Expected(x)
	if err != nil {
		return err
	}
	if summary {
		b, _ := json.MarshalIndent(struct {
			Counts kit.G1ExpCounts
			Cross  []kit.G1ExpMismatch
			Audit  kit.G1ExpAuditEstimate
		}{x.Counts, x.Cross.Mismatches, x.Audit.Estimate}, "", " ")
		fmt.Println(string(b))
		blocked := map[string]int{}
		for _, a := range x.Alternatives {
			if a.Status == kit.G1ExpBlocked {
				blocked[a.Row+" "+a.Block]++
			}
		}
		fmt.Println(blocked)
	}
	if write {
		if _, err := kit.ParseCSharpG1Expected(data, in); err != nil {
			return fmt.Errorf("generated asset does not parse: %w", err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s: %d bytes %+v\n", kit.CSharpG1ExpectedPath, len(data), x.Counts)
		return nil
	}
	if !summary {
		old, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(old, data) {
			return errors.New("tracked expected asset differs from the derivation")
		}
	}
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	write := flag.Bool("write", false, "write the asset instead of comparing")
	picks := flag.Bool("picks", false, "list the audit sample alternatives")
	summary := flag.Bool("summary", false, "print the counts and mismatches without comparing")
	metaIn := flag.String("metamorphic-input", "", "write the Roslyn tool input of the metamorphic variants")
	metaRes := flag.String("metamorphic-results", "", "seal the structure digests of a Roslyn run over the variants")
	flag.Parse()
	if err := run(*root, *write, *picks, *summary, *metaIn, *metaRes); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
