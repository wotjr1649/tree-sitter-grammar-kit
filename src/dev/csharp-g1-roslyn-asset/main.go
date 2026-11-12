// Command csharp-g1-roslyn-asset is the Go side of the #233 G1-2 Roslyn observation. It never runs
// dotnet, Roslyn or native code.
//
//	go run ./src/dev/csharp-g1-roslyn-asset -root . -input <out.json>      write the tool's input document
//	go run ./src/dev/csharp-g1-roslyn-asset -root . -verify <observed>     verify an observation against the tracked ledger
//	go run ./src/dev/csharp-g1-roslyn-asset -root . -pp-check <generated>  compare the pp-r2 mode's run over src/testdata/csharp-pp/cases.json
//	                                                                       byte for byte with roslyn-observed.json and its approved pin
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

func sources(root string) (kit.CSharpG1Sources, error) {
	read := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) }
	src := kit.CSharpG1Sources{Fixtures: map[string][]byte{}}
	var err error
	if src.Inventory, err = read(queryregistrycore.QualificationPin.Path); err != nil {
		return src, err
	}
	if src.Alternatives, err = read("src/contracts/feature-alternatives.json"); err != nil {
		return src, err
	}
	if src.FactMapping, err = read("src/contracts/fact-mapping.json"); err != nil {
		return src, err
	}
	for _, dir := range []string{"routes", "gaps", "n461", "queries"} {
		if src.Fixtures[dir], err = read("src/testdata/native/" + dir + "/csharp.json"); err != nil {
			return src, err
		}
	}
	return src, nil
}

func writeInput(root, out string) error {
	src, err := sources(root)
	if err != nil {
		return err
	}
	list, err := kit.CSharpG1RoslynSources(src)
	if err != nil {
		return err
	}
	data, err := kit.MarshalCSharpG1RoslynInput(list)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	fmt.Printf("input: %d sources, %d bytes, sha256 %s\n", len(list), len(data), hex.EncodeToString(sum[:]))
	return os.WriteFile(out, data, 0o644)
}

func verify(root, observed string) error {
	src, err := sources(root)
	if err != nil {
		return err
	}
	ledger, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(kit.CSharpG1LedgerPath)))
	if err != nil {
		return err
	}
	data, err := os.ReadFile(observed)
	if err != nil {
		return err
	}
	x, err := kit.ParseCSharpG1RoslynObserved(data, ledger, src)
	if err != nil {
		return err
	}
	fmt.Printf("verified: %d sources, %d bytes\n", len(x.Sources), len(data))
	return nil
}

// ppCheck requires the pp-r2 run over cases.json to be the preserved observation: the same bytes as
// roslyn-observed.json, and a SHA-256 that an approved legacy-observation tuple (a content-addressed evidence-pin
// object) names. The Roslyn tool is the only producer; nothing here runs dotnet.
func ppCheck(root, generated string) error {
	got, err := os.ReadFile(generated)
	if err != nil {
		return err
	}
	want, err := os.ReadFile(filepath.Join(root, "src", "testdata", "csharp-pp", "roslyn-observed.json"))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(got)
	hexSum := hex.EncodeToString(sum[:])
	if !bytes.Equal(got, want) {
		at := 0
		for at < len(got) && at < len(want) && got[at] == want[at] {
			at++
		}
		return fmt.Errorf("pp-r2 output differs from roslyn-observed.json: %d bytes against %d, first difference at byte %d (line %d)", len(got), len(want), at, 1+bytes.Count(want[:min(at, len(want))], []byte("\n")))
	}
	objects := filepath.Join(root, "src", "contracts", "evidence-pins", "objects")
	entries, err := os.ReadDir(objects)
	if err != nil {
		return err
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(objects, e.Name()))
		if err != nil || len(data) > 4096 {
			continue // a tuple is a few hundred bytes
		}
		var tuple struct {
			Schema            string `json:"schema"`
			ObservationSHA256 string `json:"observation_sha256"`
			Scope             string `json:"scope"`
		}
		if json.Unmarshal(data, &tuple) == nil && tuple.Schema == "tsgk-roslyn-pin-tuple/r1" && tuple.Scope == "legacy-observation" && tuple.ObservationSHA256 == hexSum {
			fmt.Printf("pp-r2: %d bytes, sha256 %s: identical to roslyn-observed.json and to observation_sha256 of approved tuple %s\n", len(got), hexSum, e.Name())
			return nil
		}
	}
	return fmt.Errorf("no approved legacy-observation tuple names sha256 %s", hexSum)
}

func main() {
	root := flag.String("root", ".", "repository root")
	input := flag.String("input", "", "write the tool input document to this path")
	verifyPath := flag.String("verify", "", "verify this observation asset")
	ppPath := flag.String("pp-check", "", "compare this pp-r2 run over cases.json with roslyn-observed.json and its approved pin")
	flag.Parse()
	var err error
	switch {
	case *input != "":
		err = writeInput(*root, *input)
	case *verifyPath != "":
		err = verify(*root, *verifyPath)
	case *ppPath != "":
		err = ppCheck(*root, *ppPath)
	default:
		err = errors.New("one of -input, -verify, -pp-check is required")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
