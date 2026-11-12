// Command svc-inline-expected derives src/contracts/query-svc-inline-expected-r1.json (#362 scope 5
// extension) from the pinned SVC contract, fixture, inventory and fact pack alone. It never runs
// native code and never reads native output.
//
//	go run ./src/dev/svc-inline-expected -root . [-write]
package main

import (
	"bytes"
	"encoding/json"
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
	var in [5][]byte
	for i, rel := range []string{kit.SvcExpectedPath, queryregistrycore.Path, queryregistrycore.QualificationPin.Path, "src/testdata/native/n461/svc.json", "src/contracts/fact-query-pack.json"} {
		data, err := read(rel)
		if err != nil {
			return err
		}
		in[i] = data
	}
	e, err := kit.DeriveSvcInline(in[0], in[1], in[2], in[3], in[4])
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(e, "", " ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := kit.ParseSvcInlineExpected(data, in[0], in[1], in[2], in[3], in[4]); err != nil {
		return fmt.Errorf("generated asset does not parse: %w", err)
	}
	path := filepath.Join(root, filepath.FromSlash(kit.SvcInlinePath))
	if !write {
		old, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(old, data) {
			return errors.New("tracked asset differs from the derivation")
		}
		return nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %d bytes derived=%d blocked=%d\n", kit.SvcInlinePath, len(data), e.Derived, e.Blocked)
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	write := flag.Bool("write", false, "write the asset instead of comparing")
	flag.Parse()
	if err := run(*root, *write); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
