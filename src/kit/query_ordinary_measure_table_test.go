package kit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// ordinaryMeasureCase is one case of the ordinary denominator with the numbers the capacity model needs.
type ordinaryMeasureCase struct {
	Route      string   `json:"route"`
	Case       string   `json:"case"`
	Steps      int      `json:"steps"`
	Edits      int      `json:"edits"`
	Bytes0     uint64   `json:"bytes0"`
	BytesMax   uint64   `json:"bytes_max"`
	BytesSum   uint64   `json:"bytes_sum"`
	Tuples     int      `json:"tuples"`
	FactTuples int      `json:"fact_tuples"`
	ErrorSteps int      `json:"error_steps"`
	TreeRoles  int      `json:"tree_roles"`
	NonASCII   bool     `json:"non_ascii"`
	QueryIDs   []string `json:"query_ids"`
	Derivable  bool     `json:"derivable"`
}

// TestOrdinaryMeasureCaseTable writes the per-case table (source size, steps, tuples) of every ordinary
// route. It needs no native run; it only reads the registry and the route fixtures.
func TestOrdinaryMeasureCaseTable(t *testing.T) {
	out := os.Getenv("TSGK_MEASURE_OUT")
	if out == "" {
		t.Skip("set TSGK_MEASURE_OUT to write the ordinary case table")
	}
	v, inventory := ordinaryP0ForPlanTest(t)
	platform := ordinaryHostPlatform()
	type key struct{ route, c string }
	tuples, facts := map[key]int{}, map[key]int{}
	roles := map[key]map[string]bool{}
	for _, coord := range v.Coordinates() {
		if coord.Kind != queryregistrycore.Structural || coord.Platform != platform {
			continue
		}
		k := key{coord.Route, coord.Case}
		tuples[k]++
		if len(coord.Query) > 6 && coord.Query[:6] == "facts." {
			facts[k]++
		}
		if roles[k] == nil {
			roles[k] = map[string]bool{}
		}
		roles[k][fmt.Sprintf("%d/%s", coord.Step, coord.TreeRole)] = true
	}
	var rows []ordinaryMeasureCase
	for _, route := range inventory.Routes {
		sources := ordinaryRouteSources(t, route.Route)
		var queryIDs []string
		for _, q := range route.Workload.Queries {
			queryIDs = append(queryIDs, q.ID)
		}
		for _, c := range route.Workload.Cases {
			k := key{route.Route, c.ID}
			if tuples[k] == 0 {
				continue
			}
			text, ok := sources[c.ID]
			if !ok {
				t.Fatalf("no source fixture for %s/%s", route.Route, c.ID)
			}
			versions, _, err := ApplyEdits(EncodingUTF8, []byte(text), c.Edits, 65536)
			if err != nil {
				t.Fatal(err)
			}
			row := ordinaryMeasureCase{Route: route.Route, Case: c.ID, Steps: len(versions), Edits: len(c.Edits), Bytes0: uint64(len(versions[0])),
				Tuples: tuples[k], FactTuples: facts[k], TreeRoles: len(roles[k]), QueryIDs: queryIDs}
			for _, version := range versions {
				row.BytesSum += uint64(len(version))
				row.BytesMax = max(row.BytesMax, uint64(len(version)))
				for _, b := range version {
					if b >= 0x80 {
						row.NonASCII = true
						break
					}
				}
			}
			for _, e := range c.Expect {
				if e.Syntax == "ERROR" {
					row.ErrorSteps++
				}
			}
			row.Derivable = row.ErrorSteps == 0
			rows = append(rows, row)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Route < rows[j].Route })
	data, err := json.MarshalIndent(rows, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "case-table.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d cases written", len(rows))
}
