package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const qualificationInventoryPath = "src/contracts/qualification-c1.json"

// droppedPlatforms are the committed inventory's platforms that CI does not run (#332: macOS
// is excluded for now). Re-enabling macOS empties this list; derivedInventory then returns the
// committed bytes unchanged.
var droppedPlatforms = []string{"darwin-arm64"}

// derivedInventory returns the committed inventory restricted to its platforms outside drop.
// Only four things change: the platform list, qualification_cells, each extra role's
// platforms/not_applicable/deferred entries and the id (suffixed with the remaining hosts, so
// the result is never mistaken for the full-platform verdict). The committed file and its pins
// stay untouched. The qualification job and the gate build it with this same function, so the
// bytes they pass to kit.Qualify are equal.
func derivedInventory(data []byte, drop []string) ([]byte, error) {
	if len(drop) == 0 {
		return data, nil
	}
	orig, err := kit.ParseQualificationInventory(data)
	if err != nil {
		return nil, errors.New("qualification inventory invalid")
	}
	for _, id := range drop {
		if !slices.ContainsFunc(orig.Platforms, func(p kit.QualPlatform) bool { return p.ID == id }) {
			return nil, errors.New("dropped platform is not registered")
		}
	}
	var doc map[string]any
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber() // route bodies hold integers that must keep their exact text
	if d.Decode(&doc) != nil {
		return nil, errors.New("qualification inventory unreadable")
	}
	var kept []any
	var suffix []string
	rawPlatforms, _ := doc["platforms"].([]any)
	for _, p := range rawPlatforms {
		m, _ := p.(map[string]any)
		id, _ := m["id"].(string)
		if slices.Contains(drop, id) {
			continue
		}
		goos, _ := m["goos"].(string)
		kept = append(kept, p)
		suffix = append(suffix, goos)
	}
	roles, _ := doc["extra_roles"].([]any)
	for _, r := range roles {
		role, _ := r.(map[string]any)
		listed, ok := role["platforms"].([]any)
		if !ok {
			return nil, errors.New("qualification inventory role shape")
		}
		role["platforms"] = slices.DeleteFunc(listed, func(v any) bool { s, _ := v.(string); return slices.Contains(drop, s) })
		for _, key := range []string{"not_applicable", "deferred"} {
			entries, ok := role[key].(map[string]any)
			if !ok {
				return nil, errors.New("qualification inventory role shape")
			}
			for _, id := range drop {
				delete(entries, id)
			}
		}
	}
	routes, _ := doc["routes"].([]any)
	doc["platforms"] = kept
	doc["qualification_cells"] = json.Number(strconv.Itoa(len(kept) * len(routes)))
	baseID, _ := doc["id"].(string)
	doc["id"] = baseID + "-" + strings.Join(suffix, "-")
	if mentions(doc, drop) {
		return nil, errors.New("dropped platform remains in the inventory")
	}
	var out bytes.Buffer
	e := json.NewEncoder(&out)
	e.SetEscapeHTML(false)
	if e.Encode(doc) != nil {
		return nil, errors.New("derived inventory encoding failed")
	}
	derived, err := kit.ParseQualificationInventory(out.Bytes())
	if err != nil {
		return nil, errors.New("derived qualification inventory invalid")
	}
	wantPlatforms := slices.DeleteFunc(slices.Clone(orig.Platforms), func(p kit.QualPlatform) bool { return slices.Contains(drop, p.ID) })
	if !slices.Equal(derived.Platforms, wantPlatforms) || !reflect.DeepEqual(derived.Routes, orig.Routes) || len(derived.ExtraRoles) != len(orig.ExtraRoles) {
		return nil, errors.New("derived qualification inventory differs from the committed routes")
	}
	return out.Bytes(), nil
}

// mentions reports whether any JSON key or string value equals one of ids.
func mentions(v any, ids []string) bool {
	switch x := v.(type) {
	case string:
		return slices.Contains(ids, x)
	case []any:
		return slices.ContainsFunc(x, func(e any) bool { return mentions(e, ids) })
	case map[string]any:
		for k, e := range x {
			if slices.Contains(ids, k) || mentions(e, ids) {
				return true
			}
		}
	}
	return false
}

// writeDerivedInventory writes the CI inventory to a new file outside the evidence.
func writeDerivedInventory(out string) error {
	data, err := readFile(qualificationInventoryPath, 16777216)
	if err != nil {
		return err
	}
	derived, err := derivedInventory(data, droppedPlatforms)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("new result unavailable")
	}
	_, err = f.Write(derived)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}
