package kit

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	ordinarySourceExportSchema = "tsgk-query-source-export/r1"
	ordinaryRegistryPath       = "src/contracts/query-semantic-registry-r1.json"
	ordinaryRegistryBytes      = uint64(82475)
	ordinaryRegistrySHA        = "91e8475fd6a207a1c64af50c63e2456601f59097445790f2523b39c265f2a06c"
	ordinaryInventoryPath      = "src/contracts/qualification-c1.json"
	ordinaryInventoryBytes     = uint64(8297072)
	ordinaryInventorySHA       = "cce474cac1f21f8a5ff8dc1fe25ac11840d4e1dcb8c174c049fa0f78672c06cb"
	ordinarySlotsPath          = "src/contracts/query-slot-inventory-r1.json"
	ordinarySlotsBytes         = uint64(60490)
	ordinarySlotsSHA           = "f24b4f0b4aad3fe165afd7950d2166fdc7e1d6a59d809faf670d5907c4f12789"
)

type ordinarySourceOrigin struct {
	File     OrdinaryPin `json:"file"`
	Selector string      `json:"selector"`
	Case     string      `json:"case"`
}

type ordinarySourceVersion struct {
	SHA256 string `json:"sha256"`
	Bytes  uint64 `json:"bytes"`
	Source []byte `json:"source_base64"`
}

type ordinarySourceCase struct {
	ID       string                 `json:"id"`
	Selector string                 `json:"selector"`
	Origins  []ordinarySourceOrigin `json:"origins"`
	Steps    []string               `json:"step_source_sha256"`
	Points   []EditPoints           `json:"edit_points"`
}

type ordinarySourceRoute struct {
	Route    string                  `json:"route"`
	Set      string                  `json:"set"`
	Encoding string                  `json:"encoding"`
	Context  string                  `json:"context"`
	Grammar  []NativeInput           `json:"grammar"`
	Queries  []QualQuery             `json:"queries"`
	Versions []ordinarySourceVersion `json:"source_versions"`
	Cases    []ordinarySourceCase    `json:"cases"`
}

type ordinarySourceCounts struct {
	Cases         int    `json:"cases"`
	Embedded      int    `json:"embedded"`
	Recovered     int    `json:"recovered"`
	Edits         int    `json:"edits"`
	Steps         int    `json:"steps"`
	TreeRoles     int    `json:"tree_roles"`
	Queries       int    `json:"queries"`
	InitialUnique int    `json:"initial_unique"`
	Versions      int    `json:"source_versions"`
	Bundles       int    `json:"query_source_bundles"`
	InitialBytes  uint64 `json:"initial_bytes"`
	StepBytes     uint64 `json:"step_bytes"`
	UniqueBytes   uint64 `json:"unique_version_bytes"`
}

type ordinarySourceExport struct {
	Schema        string                `json:"schema"`
	Registry      OrdinaryPin           `json:"registry"`
	Qualification OrdinaryPin           `json:"qualification_inventory"`
	Slots         OrdinaryPin           `json:"query_slot_inventory"`
	State         string                `json:"state"`
	Counts        ordinarySourceCounts  `json:"counts"`
	Routes        []ordinarySourceRoute `json:"routes"`
}

// parseOrdinarySourceExport checks the derived P1 source witness against the caller's
// already parsed pinned inventory. The export remains unreviewed source material.
func parseOrdinarySourceExport(data []byte, inv QualificationInventory, registrySHA string) (ordinarySourceExport, error) {
	var out ordinarySourceExport
	if err := ordinaryDecode(data, &out); err != nil {
		return out, err
	}
	bad := func() (ordinarySourceExport, error) {
		return ordinarySourceExport{}, ordinaryBad("ORDINARY_SOURCE_EXPORT")
	}
	if inv.Schema != QualificationInventorySchema || inv.SHA256 != ordinaryInventorySHA ||
		registrySHA != ordinaryRegistrySHA || out.Schema != ordinarySourceExportSchema || out.State != "DERIVED_UNREVIEWED" ||
		out.Registry != (OrdinaryPin{Path: ordinaryRegistryPath, Bytes: ordinaryRegistryBytes, SHA256: registrySHA}) ||
		out.Qualification != (OrdinaryPin{Path: ordinaryInventoryPath, Bytes: ordinaryInventoryBytes, SHA256: ordinaryInventorySHA}) ||
		out.Slots != (OrdinaryPin{Path: ordinarySlotsPath, Bytes: ordinarySlotsBytes, SHA256: ordinarySlotsSHA}) ||
		len(out.Routes) != len(inv.Routes) {
		return bad()
	}

	var counts ordinarySourceCounts
	for ri, route := range inv.Routes {
		x := out.Routes[ri]
		w := route.Workload
		if x.Route != route.Route || x.Set != w.Set || x.Encoding != EncodingUTF8 || x.Context != "qualification/r8/full-input" ||
			!reflect.DeepEqual(x.Grammar, w.Grammar) || !reflect.DeepEqual(x.Queries, w.Queries) || len(x.Cases) != len(w.Cases) ||
			x.Cases == nil || x.Queries == nil || x.Grammar == nil || x.Versions == nil {
			return bad()
		}
		counts.Queries += len(x.Queries)
		versions := make(map[string][]byte, len(x.Versions))
		for _, v := range x.Versions {
			if len(v.Source) == 0 && v.Bytes != 0 || uint64(len(v.Source)) != v.Bytes || v.SHA256 != digestHex(v.Source) || !utf8.Valid(v.Source) {
				return bad()
			}
			if _, exists := versions[v.SHA256]; exists {
				return bad()
			}
			versions[v.SHA256] = v.Source
			counts.Versions++
			counts.UniqueBytes += uint64(len(v.Source))
		}
		seenVersions := map[string][]byte{}
		versionOrder := []string{}
		initial := map[string]bool{}
		for ci, c := range w.Cases {
			wc := x.Cases[ci]
			selector := fmt.Sprintf("qualification_inventory#/routes/%d/workload/cases/%d", ri, ci)
			if wc.ID != c.ID || wc.Selector != selector || len(wc.Steps) != len(c.Edits)+1 || len(wc.Points) != len(c.Edits) ||
				!ordinarySourceOrigins(wc.Origins, route.Route, ci, c) {
				return bad()
			}
			initialSource, ok := versions[c.Input.SHA256]
			if !ok || uint64(len(initialSource)) != c.Input.Bytes || digestHex(initialSource) != c.Input.SHA256 ||
				c.Source != nil && string(initialSource) != *c.Source {
				return bad()
			}
			versionsSeen, points, err := ApplyEdits(EncodingUTF8, initialSource, c.Edits, 65536)
			if err != nil || len(versionsSeen) != len(wc.Steps) || !reflect.DeepEqual(points, wc.Points) {
				return bad()
			}
			counts.Cases++
			counts.InitialBytes += c.Input.Bytes
			if c.Source != nil {
				counts.Embedded++
			} else {
				counts.Recovered++
			}
			counts.Edits += len(c.Edits)
			counts.Steps += len(versionsSeen)
			counts.TreeRoles += 1 + 2*len(c.Edits)
			initial[c.Input.SHA256] = true
			for i, source := range versionsSeen {
				sha := digestHex(source)
				if wc.Steps[i] != sha {
					return bad()
				}
				if got, ok := versions[sha]; !ok || !reflect.DeepEqual(got, source) {
					return bad()
				}
				if old, ok := seenVersions[sha]; ok && !reflect.DeepEqual(old, source) {
					return bad()
				}
				if _, ok := seenVersions[sha]; !ok {
					versionOrder = append(versionOrder, sha)
				}
				seenVersions[sha] = source
				counts.StepBytes += uint64(len(source))
			}
		}
		if len(seenVersions) != len(versions) || len(versionOrder) != len(x.Versions) {
			return bad()
		}
		for i, version := range x.Versions {
			if version.SHA256 != versionOrder[i] {
				return bad()
			}
		}
		counts.InitialUnique += len(initial)
		counts.Bundles += len(versions) * len(x.Queries)
	}
	if !reflect.DeepEqual(out.Counts, counts) {
		return bad()
	}
	return out, nil
}

func ordinarySourceOrigins(origins []ordinarySourceOrigin, route string, caseIndex int, c QualCase) bool {
	if len(origins) == 0 || len(origins) > 2 {
		return false
	}
	for _, origin := range origins {
		if origin.Case == "" || !ordinaryPin(origin.File) || origin.File.Bytes == 0 {
			return false
		}
	}
	first := origins[0]
	if c.Source == nil && c.ID == "dynamic-sql-"+route {
		if len(origins) != 2 || first.Selector != "#" || !strings.HasPrefix(first.File.Path, "src/testdata/native/dynamic-sql/") ||
			!ordinaryPath(first.File.Path) || first.Case != c.ID || origins[1].Case != c.ID ||
			origins[1].File.Path != "src/testdata/native/dynamic-sql/expected.json" || !ordinaryIndexedSelector(origins[1].Selector, "#/files/") {
			return false
		}
		return true
	}
	fixture := first.File.Path == "src/testdata/native/routes/"+route+".json" || first.File.Path == "src/testdata/native/gaps/"+route+".json" || first.File.Path == "src/testdata/native/n461/"+route+".json"
	if !fixture || !strings.HasPrefix(first.Selector, "#/cases/") {
		return false
	}
	selectorParts := strings.Split(strings.TrimPrefix(first.Selector, "#/cases/"), "/")
	if len(selectorParts) != 2 || selectorParts[1] != "source_utf8" && selectorParts[1] != "source_case" {
		return false
	}
	if !ordinaryCanonicalIndex(selectorParts[0]) {
		return false
	}
	if c.Source == nil {
		return len(origins) == 1 && selectorParts[1] == "source_utf8" && first.Case == c.ID
	}
	if selectorParts[1] != "source_utf8" || !strings.Contains(first.File.Path, "/routes/") || len(origins) != 2 {
		return false
	}
	second := origins[1]
	return second.Case == c.ID && second.File.Path == "src/testdata/native/queries/"+route+".json" && ordinaryIndexedPathSelector(second.Selector, "#/cases/", "source_case")
}

func ordinaryIndexedSelector(selector, prefix string) bool {
	if !strings.HasPrefix(selector, prefix) {
		return false
	}
	return ordinaryCanonicalIndex(strings.TrimPrefix(selector, prefix))
}

func ordinaryIndexedPathSelector(selector, prefix, suffix string) bool {
	if !strings.HasPrefix(selector, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(selector, prefix), "/")
	return len(parts) == 2 && ordinaryCanonicalIndex(parts[0]) && parts[1] == suffix
}

func ordinaryCanonicalIndex(s string) bool {
	n, err := strconv.ParseUint(s, 10, 32)
	return err == nil && strconv.FormatUint(n, 10) == s
}
