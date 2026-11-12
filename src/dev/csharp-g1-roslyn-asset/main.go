// Command csharp-g1-roslyn-asset is the Go side of the #233 G1-2 Roslyn observation. It never runs
// dotnet, Roslyn or native code.
//
//	go run ./src/dev/csharp-g1-roslyn-asset -root . -input <out.json>      write the tool's input document
//	go run ./src/dev/csharp-g1-roslyn-asset -root . -verify <observed>     verify an observation against the tracked ledger
//	go run ./src/dev/csharp-g1-roslyn-asset -root . -regress <observed>    compare a run over src/testdata/csharp-pp/cases.json
//	                                                                       with the 157 observations of roslyn-observed.json
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

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

// regress compares the directive observation of a run over cases.json with roslyn-observed.json.
func regress(root, observed string) error {
	dir := filepath.Join(root, "src", "testdata", "csharp-pp")
	type casesDoc struct {
		Cases []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"cases"`
	}
	var cases casesDoc
	var old struct {
		Cases []map[string]any `json:"cases"`
	}
	var got struct {
		Sources []map[string]any `json:"sources"`
	}
	for _, f := range []struct {
		name string
		into any
	}{{filepath.Join(dir, "cases.json"), &cases}, {filepath.Join(dir, "roslyn-observed.json"), &old}, {observed, &got}} {
		data, err := os.ReadFile(f.name)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, f.into); err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
	}
	if len(cases.Cases) != len(old.Cases) || len(got.Sources) != len(old.Cases) {
		return fmt.Errorf("case counts differ: cases %d, observed %d, new %d", len(cases.Cases), len(old.Cases), len(got.Sources))
	}
	var diffs []string
	same := 0
	for i, o := range old.Cases {
		n := got.Sources[i]
		raw, err := base64.StdEncoding.DecodeString(cases.Cases[i].Source)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		if n["id"] != o["id"] || cases.Cases[i].ID != o["id"] || n["sha256"] != hex.EncodeToString(sum[:]) || int(n["bytes"].(float64)) != len(raw) {
			diffs = append(diffs, fmt.Sprintf("%v: id, source sha256 or size differs", o["id"]))
			continue
		}
		ok := true
		for _, k := range []string{"features", "directives", "disabled", "errors", "diagnostics", "oracle_error"} {
			if !reflect.DeepEqual(o[k], n[k]) {
				ok = false
				diffs = append(diffs, fmt.Sprintf("%v: %s differs", o["id"], k))
			}
		}
		if ok {
			same++
		}
	}
	fmt.Printf("regression: %d of %d observations identical in features, directives, disabled, errors and diagnostics\n", same, len(old.Cases))
	if len(diffs) > 0 {
		for _, d := range diffs {
			fmt.Println(d)
		}
		return errors.New("regression differs")
	}
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	input := flag.String("input", "", "write the tool input document to this path")
	verifyPath := flag.String("verify", "", "verify this observation asset")
	regressPath := flag.String("regress", "", "compare this run over cases.json with roslyn-observed.json")
	flag.Parse()
	var err error
	switch {
	case *input != "":
		err = writeInput(*root, *input)
	case *verifyPath != "":
		err = verify(*root, *verifyPath)
	case *regressPath != "":
		err = regress(*root, *regressPath)
	default:
		err = errors.New("one of -input, -verify, -regress is required")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
