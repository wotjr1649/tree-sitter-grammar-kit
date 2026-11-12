package kit

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func ordinaryMiniWitness(t *testing.T) ([]byte, QualificationInventory) {
	t.Helper()
	initial := []byte("ab")
	firstEdits := []Edit{
		{StartByte: 1, OldEndByte: 2, NewEndByte: 2, Old: []byte("b"), New: []byte("c")},
		{StartByte: 1, OldEndByte: 2, NewEndByte: 1, Old: []byte("c"), New: []byte{}},
	}
	secondEdits := []Edit{{StartByte: 1, OldEndByte: 2, NewEndByte: 2, Old: []byte("b"), New: []byte("x")}}
	queries := []QualQuery{{ID: "q0", SHA256: strings.Repeat("b", 64)}, {ID: "q1", SHA256: strings.Repeat("c", 64)}}
	input := func() NativeInput { return NativeInput{Bytes: uint64(len(initial)), SHA256: digestHex(initial)} }
	cases := []QualCase{{ID: "case-a", Input: input(), Edits: firstEdits}, {ID: "case-b", Input: input(), Edits: secondEdits}, {ID: "case-no-edit", Input: input(), Edits: []Edit{}}}
	inv := QualificationInventory{SHA256: ordinaryInventorySHA, Schema: QualificationInventorySchema, Routes: []QualRoute{{Route: "mini", Workload: QualWorkload{Set: "mini-set", Grammar: []NativeInput{}, Queries: queries, Cases: cases}}}}

	versions := []ordinarySourceVersion{}
	versionSet := map[string]bool{}
	var counts ordinarySourceCounts
	counts.Queries = len(queries)
	for ci, c := range cases {
		all, points, err := ApplyEdits(EncodingUTF8, initial, c.Edits, 65536)
		if err != nil {
			t.Fatal(err)
		}
		for _, source := range all {
			h := digestHex(source)
			counts.StepBytes += uint64(len(source))
			if !versionSet[h] {
				versionSet[h] = true
				versions = append(versions, ordinarySourceVersion{SHA256: h, Bytes: uint64(len(source)), Source: source})
				counts.UniqueBytes += uint64(len(source))
			}
		}
		counts.Cases++
		counts.Recovered++
		counts.InitialBytes += c.Input.Bytes
		counts.Edits += len(c.Edits)
		counts.Steps += len(all)
		counts.TreeRoles += 1 + 2*len(c.Edits)
		if ci == 0 {
			counts.InitialUnique++
		}
		_ = points
	}
	counts.Versions = len(versions)

	routes := []ordinarySourceRoute{{
		Route: "mini", Set: "mini-set", Encoding: EncodingUTF8, Context: "qualification/r8/full-input",
		Grammar: []NativeInput{}, Queries: queries, Versions: versions,
		Cases: []ordinarySourceCase{},
	}}
	for ci, c := range cases {
		all, points, err := ApplyEdits(EncodingUTF8, initial, c.Edits, 65536)
		if err != nil {
			t.Fatal(err)
		}
		steps := make([]string, len(all))
		for i, source := range all {
			steps[i] = digestHex(source)
		}
		path := "src/testdata/native/routes/mini.json"
		origin := ordinarySourceOrigin{File: OrdinaryPin{Path: path, Bytes: 1, SHA256: strings.Repeat("a", 64)}, Selector: "#/cases/" + strconv.Itoa(ci) + "/source_utf8", Case: c.ID}
		routes[0].Cases = append(routes[0].Cases, ordinarySourceCase{ID: c.ID, Selector: "qualification_inventory#/routes/0/workload/cases/" + strconv.Itoa(ci), Origins: []ordinarySourceOrigin{origin}, Steps: steps, Points: points})
	}
	// The version bundle count grows as versions are first encountered by each case.
	counts.Bundles = len(versions) * len(queries)
	out := ordinarySourceExport{
		Schema:        ordinarySourceExportSchema,
		Registry:      OrdinaryPin{Path: ordinaryRegistryPath, Bytes: ordinaryRegistryBytes, SHA256: ordinaryRegistrySHA},
		Qualification: OrdinaryPin{Path: ordinaryInventoryPath, Bytes: ordinaryInventoryBytes, SHA256: ordinaryInventorySHA},
		Slots:         OrdinaryPin{Path: ordinarySlotsPath, Bytes: ordinarySlotsBytes, SHA256: ordinarySlotsSHA},
		State:         "DERIVED_UNREVIEWED", Counts: counts, Routes: routes,
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return data, inv
}

func TestOrdinarySourceExportBindsCaseHistoryAndPins(t *testing.T) {
	data, inv := ordinaryMiniWitness(t)
	out, err := parseOrdinarySourceExport(data, inv, ordinaryRegistrySHA)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Routes) != 1 || len(out.Routes[0].Cases) != 3 || len(out.Routes[0].Versions) != 4 || out.Counts.Edits != 3 || out.Routes[0].Cases[2].Points != nil {
		t.Fatalf("unexpected source witness shape: routes=%d cases=%d versions=%d edits=%d", len(out.Routes), len(out.Routes[0].Cases), len(out.Routes[0].Versions), out.Counts.Edits)
	}
	if out.State != "DERIVED_UNREVIEWED" {
		t.Fatalf("source export state = %q", out.State)
	}

	var valid any
	if err := jsonv2.Unmarshal(data, &valid); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"case selector swapped", func(root map[string]any) {
			rows := root["routes"].([]any)[0].(map[string]any)["cases"].([]any)
			rows[0].(map[string]any)["id"], rows[1].(map[string]any)["id"] = rows[1].(map[string]any)["id"], rows[0].(map[string]any)["id"]
		}},
		{"edit points changed", func(root map[string]any) {
			points := root["routes"].([]any)[0].(map[string]any)["cases"].([]any)[0].(map[string]any)["edit_points"].([]any)
			points[0].(map[string]any)["new_end_point"].(map[string]any)["column"] = float64(99)
		}},
		{"origin pin substituted", func(root map[string]any) {
			origin := root["routes"].([]any)[0].(map[string]any)["cases"].([]any)[0].(map[string]any)["origins"].([]any)[0].(map[string]any)
			origin["file"].(map[string]any)["path"] = "src/testdata/native/routes/other.json"
		}},
		{"required id removed", func(root map[string]any) {
			rows := root["routes"].([]any)[0].(map[string]any)["cases"].([]any)
			root["routes"].([]any)[0].(map[string]any)["cases"] = rows[:1]
		}},
		{"registry pin substituted", func(root map[string]any) {
			root["registry"].(map[string]any)["sha256"] = strings.Repeat("d", 64)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var clone map[string]any
			if err := jsonv2.Unmarshal(data, &clone); err != nil {
				t.Fatal(err)
			}
			tc.mutate(clone)
			mutated, err := jsonv2.Marshal(clone)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseOrdinarySourceExport(mutated, inv, ordinaryRegistrySHA); err == nil {
				t.Fatal("tampered witness was accepted")
			}
		})
	}
	if _, err := parseOrdinarySourceExport(data, QualificationInventory{Schema: inv.Schema, SHA256: strings.Repeat("e", 64), Routes: inv.Routes}, ordinaryRegistrySHA); err == nil {
		t.Fatal("unpinned qualification inventory was accepted")
	}
	if _, err := parseOrdinarySourceExport(data, inv, strings.Repeat("d", 64)); err == nil {
		t.Fatal("unselected registry hash was accepted")
	}
	changedHistory := inv
	changedHistory.Routes = slices.Clone(inv.Routes)
	changedHistory.Routes[0].Workload.Cases = slices.Clone(inv.Routes[0].Workload.Cases)
	changedHistory.Routes[0].Workload.Cases[1].Edits = slices.Clone(inv.Routes[0].Workload.Cases[1].Edits)
	changedHistory.Routes[0].Workload.Cases[1].Edits[0].New = []byte("y")
	if _, err := parseOrdinarySourceExport(data, changedHistory, ordinaryRegistrySHA); err == nil {
		t.Fatal("same-source case with a different edit history was accepted")
	}
}

func TestOrdinarySourceExportStrictShape(t *testing.T) {
	data, inv := ordinaryMiniWitness(t)
	var root map[string]any
	if err := jsonv2.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	delete(root, "state")
	mutated, err := jsonv2.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseOrdinarySourceExport(mutated, inv, ordinaryRegistrySHA); err == nil {
		t.Fatal("missing field was accepted")
	}
	if _, err := parseOrdinarySourceExport(append(data, []byte(" {}")...), inv, ordinaryRegistrySHA); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
	duplicate := strings.Replace(string(data), "{", `{"schema":"duplicate",`, 1)
	if _, err := parseOrdinarySourceExport([]byte(duplicate), inv, ordinaryRegistrySHA); err == nil {
		t.Fatal("duplicate member was accepted")
	}
}
