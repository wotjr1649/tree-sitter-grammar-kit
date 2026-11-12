// Command large-expected derives src/contracts/query-large-expected-r1.json (#362 scope 6) from the
// pinned fixture template alone. It never runs native code and never reads native output.
//
//	go run ./src/dev/large-expected -root . [-write]
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

const grammarBasis = "tree-sitter-c-sharp 0.23.5 grammar.json (parser.c sha256 b1a77b75e034af978ebbde75be71e362cae447f67ec25dcf297702cdca9a1dbb, scanner.c sha256 90c63e1fefc5a04070785bbbaec9c265392db8520e81a8d690d0e30d40891ae0), the registered qualification closure of the n461-large role"

const basis = "Rows are derived from the fixture template by a hand-written parser of the exact C# subset the valid template uses (namespace, class, field, method, local declaration, assignment of a binary sum, return), following the grammar.json rules; the two fixtures with `x = x + ;` depend on parser error recovery and are NOT_DERIVABLE. Occurrences are the facts.declarations captures of the same derived tree as an unordered multiset."

const controlBasis = "all.nodes (`_ @node`) on cs-large-8m-errors captures every node. The tree has more than the 1,000,000 captures the native-query-large operation allows, so the query stops RESOURCE_LIMIT/MATCH_LIMIT after 1,000,000 captures; each retained capture row needs at least 25 encoded bytes, so 1,000,000 rows exceed the 16,777,216-byte output limit and the driver replaces the whole response with RESOURCE_LIMIT/OUTPUT_LIMIT. The case-level code is therefore OUTPUT_LIMIT."

func run(root string, write bool) error {
	read := func(rel string) []byte {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			panic(err)
		}
		return data
	}
	registry, inventory, fixtures := read(queryregistrycore.Path), read(queryregistrycore.QualificationPin.Path), read(kit.LargeFixturesPath)
	templates, syntax, err := kit.LargeTemplates(fixtures)
	if err != nil {
		return err
	}
	inv, err := kit.ParseQualificationInventory(inventory)
	if err != nil {
		return err
	}
	var role kit.QualRole
	for _, r := range inv.ExtraRoles {
		if r.ID == "n461-large" {
			role = r
		}
	}
	if len(role.Workloads) != 2 {
		return errors.New("n461-large role missing")
	}
	e := kit.LargeExpected{
		Schema:        kit.LargeExpectedSchema,
		Registry:      queryregistrycore.FilePin(queryregistrycore.Path, registry),
		Qualification: queryregistrycore.FilePin(queryregistrycore.QualificationPin.Path, inventory),
		Fixtures:      queryregistrycore.FilePin(kit.LargeFixturesPath, fixtures),
		Grammar:       grammarBasis,
		Basis:         basis,
		NotApplicable: role.NotApplicable,
	}
	for i, t := range templates {
		c := kit.LargeExpectedFixture{ID: t.ID, Set: role.Workloads[0].Set, Bytes: t.Bytes, SHA256: t.SHA256, Repeat: t.Repeat, Syntax: syntax[i]}
		cst, cerr := t.CSTDigests(kit.LargePageRows)
		var kerr *kit.Error
		switch {
		case cerr == nil:
			occ, oerr := t.Occurrences()
			if oerr != nil {
				return oerr
			}
			c.Derivation = kit.LargeDerivedByTemplate
			c.CST = &kit.LargeExpectedCST{PageRows: kit.LargePageRows, Nodes: cst.Nodes, PageDigests: cst.Pages, PagesSHA256: cst.PagesChain,
				AllRowsSHA256: cst.AllRows, RowBytes: cst.RowBytes, MaxDepth: cst.MaxDepth, HasError: syntax[i] == "ERROR"}
			c.Occurrences = &kit.LargeExpectedOccurrences{PageCaptures: kit.LargeOccurrencePage, Total: occ.Count, Pages: 1 + (occ.Count-1)/kit.LargeOccurrencePage,
				ByName: occ.ByName, SHA256: occ.Digest}
		case errors.As(cerr, &kerr) && kerr.Code == kit.LargeNotDerivable:
			c.Derivation, c.Code = kit.LargeNotDerivableState, kit.LargeNotDerivable
		default:
			return cerr
		}
		e.Cases = append(e.Cases, c)
	}
	over := role.Workloads[1]
	e.Control = kit.LargeExpectedControl{ID: over.Cases[0].ID, Set: over.Set, SourceCase: "cs-large-8m-errors", Query: over.Queries[0].ID, QuerySHA256: over.Queries[0].SHA256,
		Status: over.Cases[0].ExpectStatus, Code: "OUTPUT_LIMIT", SemanticPass: false, Basis: controlBasis}
	data, err := json.MarshalIndent(e, "", " ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := kit.ParseLargeExpected(data, registry, inventory, fixtures); err != nil {
		return fmt.Errorf("generated asset does not parse: %w", err)
	}
	path := filepath.Join(root, filepath.FromSlash(kit.LargeExpectedPath))
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
	fmt.Printf("wrote %s: %d bytes\n", kit.LargeExpectedPath, len(data))
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
