package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/queryregistry"
)

func TestGenerateSourceOnly(t *testing.T) {
	root := "../../.."
	got, err := generate(root)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := os.ReadFile(filepath.Join(root, queryregistry.Path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, retained) {
		t.Fatal("explicit generator is not deterministic for the fixed original inputs")
	}
	reg, err := queryregistry.Parse(got)
	if err != nil || reg.FullScopeComplete || len(reg.Ordinary) != 26 || len(reg.Large) != 4 {
		t.Fatal("generator scope", err)
	}
	// A changed source is refused before any write or expectation generation.
	badRoot := t.TempDir()
	path := filepath.Join(badRoot, queryregistry.QualificationPin.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	slots, err := os.ReadFile(filepath.Join(root, queryregistry.SlotPin.Path))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badRoot, queryregistry.SlotPin.Path), slots, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := generate(badRoot); err == nil {
		t.Fatal("changed original accepted")
	}
}
