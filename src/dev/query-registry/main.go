// Explicit source-only registry generation; normal validation never invokes this writer.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/queryregistry"
)

func generate(root string) ([]byte, error) {
	read := func(path string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(path))) }
	qualification, err := read(queryregistry.QualificationPin.Path)
	if err != nil {
		return nil, err
	}
	slots, err := read(queryregistry.SlotPin.Path)
	if err != nil {
		return nil, err
	}
	inv, extras, err := queryregistry.Sources(qualification, slots)
	if err != nil {
		return nil, err
	}
	reg := queryregistry.Registry{Schema: queryregistry.Schema, Policy: queryregistry.Policy, Qualification: queryregistry.QualificationPin, Slots: queryregistry.SlotPin,
		Platforms: queryregistry.Platforms(), Probes: queryregistry.Roster(), Extra: queryregistry.ExtraRoles(extras), Assets: queryregistry.MissingAssets(), MissingScopes: queryregistry.MissingScopes(), ContentState: "INCOMPLETE"}
	reg.Large = queryregistry.LargeWorkloads(inv)
	for _, path := range []string{"src/dev/query-reference/check.py", "src/internal/native/query_policy_test.go"} {
		data, err := read(path)
		if err != nil {
			return nil, err
		}
		reg.Basis = append(reg.Basis, queryregistry.FilePin(path, data))
	}
	for i, route := range inv.Routes {
		reg.Ordinary = append(reg.Ordinary, queryregistry.Select(route, i))
	}
	reg.Namespace = queryregistry.Namespace(reg)
	reg.RequiredSets = queryregistry.Sets(queryregistry.Expand(inv, reg.Platforms, reg.Probes))
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(data)+1 > queryregistry.MaxBytes {
		return nil, fmt.Errorf("registry exceeds 256 KiB")
	}
	return append(data, '\n'), nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	write := flag.Bool("write", false, "explicitly replace the fixed P0 registry from its original pins")
	flag.Parse()
	if !*write {
		fmt.Fprintln(os.Stderr, "generation requires -write; validation is read-only")
		os.Exit(2)
	}
	data, err := generate(*root)
	if err == nil {
		err = os.WriteFile(filepath.Join(*root, filepath.FromSlash(queryregistry.Path)), data, 0644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%s bytes=%d sha256=%s; content=INCOMPLETE full_scope_complete=false\n", queryregistry.Path, len(data), queryregistry.Digest(data))
}
