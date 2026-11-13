package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// authoredTypes is the JSON the type audit is authored in: the rows of the B alternatives and the rows of the anchor lexemes.
type authoredTypes struct {
	Rows    []kit.G1B34TypeRow       `json:"rows"`
	Anchors []kit.G1B34AnchorTypeRow `json:"anchors"`
}

func readSpec(grammarDir, specDir string) (string, map[string]string, error) {
	g, err := os.ReadFile(filepath.Join(grammarDir, "grammar.js"))
	if err != nil {
		return "", nil, err
	}
	spec := map[string]string{}
	for _, name := range kit.G1SpecFiles {
		d, err := os.ReadFile(filepath.Join(specDir, name))
		if err != nil {
			return "", nil, err
		}
		spec[name] = string(d)
	}
	return string(g), spec, nil
}

// runAudit builds (-in, -write) or verifies the position audit (pos) or the type audit (type).
func runAudit(root string, in kit.CSharpG1ExpectedSources, audit, authored string, write, pins bool, grammarDir, specDir string) error {
	read := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) }
	expected, err := read(kit.CSharpG1B34Path)
	if err != nil {
		return err
	}
	var raw []byte
	if authored != "" {
		if raw, err = os.ReadFile(authored); err != nil {
			return err
		}
	}
	switch audit {
	case "pos":
		return runPos(root, in, expected, raw, write, pins, grammarDir, specDir)
	case "type":
		return runType(root, expected, raw, write, pins, grammarDir)
	}
	return fmt.Errorf("-audit is pos or type")
}

func emit(path, rel string, data, old []byte, write bool) error {
	if write {
		return os.WriteFile(path, data, 0o644)
	}
	if !bytes.Equal(old, data) {
		return fmt.Errorf("tracked %s differs from the derivation", rel)
	}
	return nil
}

func runPos(root string, in kit.CSharpG1ExpectedSources, expected, raw []byte, write, pins bool, grammarDir, specDir string) error {
	path := filepath.Join(root, filepath.FromSlash(kit.CSharpG1AuditB34Path))
	if pins {
		var rows []kit.G1B34PosRow
		if raw != nil {
			if err := jsonv2.Unmarshal(raw, &rows, jsonv2.RejectUnknownMembers(true)); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			x, err := kit.DecodeCSharpG1AuditB34(data)
			if err != nil {
				return err
			}
			rows = x.Rows
		}
		fmt.Print(kit.G1GroundPinSource("g1b34PosPins", kit.G1B34PosGroundPins(rows)))
		return nil
	}
	tx, err := texts(in)
	if err != nil {
		return err
	}
	view, err := kit.G1B34AuditView(expected, tx)
	if err != nil {
		return err
	}
	var data []byte
	if raw != nil {
		var rows []kit.G1B34PosRow
		if err := jsonv2.Unmarshal(raw, &rows, jsonv2.RejectUnknownMembers(true)); err != nil {
			return err
		}
		x, err := kit.DeriveCSharpG1AuditB34(rows, view)
		if err != nil {
			return err
		}
		if data, err = kit.MarshalCSharpG1AuditB34(x); err != nil {
			return err
		}
	} else if data, err = os.ReadFile(path); err != nil {
		return err
	}
	x, err := kit.ParseCSharpG1AuditB34(data, view)
	if err != nil {
		return fmt.Errorf("the position audit does not verify: %w", err)
	}
	if grammarDir != "" {
		g, spec, err := readSpec(grammarDir, specDir)
		if err != nil {
			return err
		}
		if err := kit.CheckG1AuditB34Grounds(x.Rows, g, spec); err != nil {
			return err
		}
	}
	if raw != nil {
		old, _ := os.ReadFile(path)
		if err := emit(path, kit.CSharpG1AuditB34Path, data, old, write); err != nil {
			return err
		}
		fmt.Printf("%s %s: %d bytes %+v\n", map[bool]string{true: "wrote", false: "matches"}[write], kit.CSharpG1AuditB34Path, len(data), x.Counts)
		return nil
	}
	fmt.Printf("verified %s: %+v\n", kit.CSharpG1AuditB34Path, x.Counts)
	return nil
}

func runType(root string, expected, raw []byte, write, pins bool, grammarDir string) error {
	path := filepath.Join(root, filepath.FromSlash(kit.CSharpG1TypeAuditB34Path))
	if pins {
		var a authoredTypes
		if raw != nil {
			if err := jsonv2.Unmarshal(raw, &a, jsonv2.RejectUnknownMembers(true)); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			x, err := kit.DecodeCSharpG1TypeAuditB34(data)
			if err != nil {
				return err
			}
			a.Rows, a.Anchors = x.Rows, x.Anchors
		}
		fmt.Print(kit.G1GroundPinSource("g1b34TypePins", kit.G1B34TypeGroundPins(kit.CSharpG1TypeAuditB34{Rows: a.Rows, Anchors: a.Anchors})))
		return nil
	}
	view, err := kit.G1B34TypeView(expected)
	if err != nil {
		return err
	}
	var data []byte
	if raw != nil {
		var a authoredTypes
		if err := jsonv2.Unmarshal(raw, &a, jsonv2.RejectUnknownMembers(true)); err != nil {
			return err
		}
		x, err := kit.DeriveCSharpG1TypeAuditB34(a.Rows, a.Anchors, view)
		if err != nil {
			return err
		}
		if data, err = kit.MarshalCSharpG1TypeAuditB34(x); err != nil {
			return err
		}
	} else if data, err = os.ReadFile(path); err != nil {
		return err
	}
	x, err := kit.ParseCSharpG1TypeAuditB34(data, view)
	if err != nil {
		return fmt.Errorf("the type audit does not verify: %w", err)
	}
	if grammarDir != "" {
		g, err := os.ReadFile(filepath.Join(grammarDir, "grammar.js"))
		if err != nil {
			return err
		}
		if err := kit.CheckG1TypeAuditB34Grounds(x, string(g)); err != nil {
			return err
		}
		km, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(kit.CSharpG1KindMapPathR3)))
		if err != nil {
			return err
		}
		r2, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(kit.CSharpG1KindMapPathR2)))
		if err != nil {
			return err
		}
		k3, err := kit.ParseCSharpG1KindMapR3(km, r2)
		if err != nil {
			return err
		}
		if err := kit.CheckG1KindMapR3Grounds(k3, string(g)); err != nil {
			return err
		}
	}
	if raw != nil {
		old, _ := os.ReadFile(path)
		if err := emit(path, kit.CSharpG1TypeAuditB34Path, data, old, write); err != nil {
			return err
		}
		fmt.Printf("%s %s: %d bytes %+v\n", map[bool]string{true: "wrote", false: "matches"}[write], kit.CSharpG1TypeAuditB34Path, len(data), x.Counts)
		return nil
	}
	fmt.Printf("verified %s: %+v\n", kit.CSharpG1TypeAuditB34Path, x.Counts)
	return nil
}
