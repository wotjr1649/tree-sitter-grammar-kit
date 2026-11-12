package foundation

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/queryregistry"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func queryRegistryInputs(t *testing.T) ([]byte, []byte, []byte, map[string][]byte) {
	t.Helper()
	root := repository(t)
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	qualification := read(queryregistry.QualificationPin.Path)
	slots := read(queryregistry.SlotPin.Path)
	registry := read(queryregistry.Path)
	basis := map[string][]byte{
		"src/dev/query-reference/check.py":         read("src/dev/query-reference/check.py"),
		"src/internal/native/query_policy_test.go": read("src/internal/native/query_policy_test.go"),
	}
	return registry, qualification, slots, basis
}

func TestQuerySemanticRegistry(t *testing.T) {
	data, qualification, slots, basis := queryRegistryInputs(t)
	got, err := queryregistry.Validate(data, queryregistry.Digest(data), qualification, slots, basis)
	if err != nil {
		t.Fatal(err)
	}
	if got.SHA256() != queryregistry.Digest(data) || got.Namespace() == "" {
		t.Fatal("registry caller binding or namespace is empty")
	}

	inv, err := kit.ParseQualificationInventory(qualification)
	if err != nil {
		t.Fatal(err)
	}
	roster := queryregistry.Roster()
	positive, controls := 0, 0
	for _, probe := range roster {
		switch probe.Kind {
		case queryregistry.Predicate:
			positive++
		case queryregistry.Control:
			controls++
		}
	}
	if positive != 20 {
		t.Fatalf("predicate roster = %d, want 20", positive)
	}
	if controls != 47 {
		t.Fatalf("control roster = %d, want 47", controls)
	}
	platforms := queryregistry.Platforms()
	want := make(map[string]queryregistry.Coordinate, 106342)
	counts := map[string]int{}
	// This expansion is intentionally independent from queryregistry.Expand.
	for _, route := range inv.Routes {
		for _, platform := range platforms {
			for _, workloadCase := range route.Workload.Cases {
				for _, query := range route.Workload.Queries {
					for step := 0; step <= len(workloadCase.Edits); step++ {
						roles := []string{"incremental"}
						if step > 0 {
							roles = append(roles, "fresh")
						}
						for _, treeRole := range roles {
							coord := queryregistry.Coordinate{Kind: queryregistry.Structural, Route: route.Route, Platform: platform.ID, Subject: platform.Subject,
								Role: "primary", Set: route.Workload.Set, Case: workloadCase.ID, Query: query.ID, Step: uint32(step), TreeRole: treeRole, Context: "qualification/r8"}
							id := coord.ID(got.Namespace())
							if _, exists := want[id]; exists {
								t.Fatalf("duplicate independently expanded ID %s", id)
							}
							want[id], counts[coord.Kind] = coord, counts[coord.Kind]+1
						}
					}
				}
			}
		}
	}
	for _, probe := range roster {
		for _, platform := range platforms {
			coord := queryregistry.Coordinate{Kind: probe.Kind, Route: "owned-plain", Platform: platform.ID, Subject: platform.Subject,
				Role: "predicate-probe", Set: "predicate/r1", Case: probe.ID, Query: "q0", Step: 0, TreeRole: "incremental", Context: probe.Basis}
			id := coord.ID(got.Namespace())
			if _, exists := want[id]; exists {
				t.Fatalf("duplicate probe ID %s", id)
			}
			want[id], counts[coord.Kind] = coord, counts[coord.Kind]+1
		}
	}
	large := 0
	var expectedLarge []queryregistry.LargeWorkload
	for roleIndex, role := range inv.ExtraRoles {
		if role.ID != "n461-large" {
			continue
		}
		for workloadIndex, workload := range role.Workloads {
			for caseIndex, workloadCase := range workload.Cases {
				kind, status := "LARGE_WORKLOAD_POSITIVE", "COMPLETED"
				if workloadCase.ExpectStatus != "" && workloadCase.ExpectStatus != "COMPLETED" {
					kind, status = "LARGE_WORKLOAD_CONTROL", workloadCase.ExpectStatus
				}
				if kind == "LARGE_WORKLOAD_CONTROL" && (status != "RESOURCE_LIMIT" || workloadCase.ID != "cs-large-8m-over-captures") {
					t.Fatalf("large control expectation changed: %s/%s", workloadCase.ID, status)
				}
				for _, query := range workload.Queries {
					ref := fmt.Sprintf("qualification_inventory#/extra_roles/%d/workloads/%d/cases/%d", roleIndex, workloadIndex, caseIndex)
					expectedLarge = append(expectedLarge, queryregistry.LargeWorkload{Set: workload.Set, Case: workloadCase.ID, Query: query.ID, SourceRef: ref, Kind: kind, Status: status})
					coord := queryregistry.Coordinate{Kind: kind, Route: "csharp", Platform: "windows-amd64", Subject: "windows/amd64",
						Role: "n461-large", Set: workload.Set, Case: workloadCase.ID, Query: query.ID, Step: 0, TreeRole: "incremental",
						Context: "qualification/r8-large-subledger-unregistered"}
					id := coord.ID(got.Namespace())
					if _, exists := want[id]; exists {
						t.Fatalf("duplicate large obligation %s", id)
					}
					want[id], counts[kind] = coord, counts[kind]+1
					large++
				}
			}
		}
	}
	if large != 4 || counts["LARGE_WORKLOAD_POSITIVE"] != 3 || counts["LARGE_WORKLOAD_CONTROL"] != 1 {
		t.Fatalf("large workload denominator = %d, counts=%v", large, counts)
	}
	var registered struct {
		Large []queryregistry.LargeWorkload `json:"large_workloads"`
	}
	if err := json.Unmarshal(data, &registered); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(registered.Large, expectedLarge) {
		t.Fatalf("large workload registrations differ from independent selectors: got=%+v want=%+v", registered.Large, expectedLarge)
	}
	if counts[queryregistry.Structural] != 106208 || counts[queryregistry.Predicate] != 40 || counts[queryregistry.Control] != 94 {
		t.Fatalf("independent required counts = %v", counts)
	}
	actual := got.Coordinates()
	if len(actual) != len(want) {
		t.Fatalf("registered coordinates=%d, independently expanded=%d", len(actual), len(want))
	}
	for _, coord := range actual {
		id := coord.ID(got.Namespace())
		if expected, ok := want[id]; !ok || expected != coord {
			t.Fatalf("undeclared or changed required coordinate: %+v", coord)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("%d independently required IDs are missing", len(want))
	}
	// Rebuild typed set counters and their length-prefixed membership digest without
	// calling the production Sets or setKey helpers.
	type group struct{ kind, scope, platform string }
	rows := make(map[group][]string)
	for _, coord := range actual {
		key := group{coord.Kind, coord.Route, coord.Platform}
		canonical, err := independentCanonicalCoordinate(coord)
		if err != nil {
			t.Fatal(err)
		}
		rows[key] = append(rows[key], string(canonical))
	}
	groups := make([]group, 0, len(rows))
	for key := range rows {
		groups = append(groups, key)
	}
	sort.Slice(groups, func(i, j int) bool {
		left, right := groups[i], groups[j]
		return left.kind+"/"+left.scope+"/"+left.platform < right.kind+"/"+right.scope+"/"+right.platform
	})
	expectedSets := make([]queryregistry.RequiredSet, 0, len(groups))
	for _, key := range groups {
		members := rows[key]
		sort.Strings(members)
		h := sha256.New()
		for _, member := range members {
			var length [8]byte
			binary.BigEndian.PutUint64(length[:], uint64(len(member)))
			h.Write(length[:])
			h.Write([]byte(member))
		}
		expectedSets = append(expectedSets, queryregistry.RequiredSet{Kind: key.kind, Scope: key.scope, Platform: key.platform,
			Registration: "REGISTERED", Required: uint64(len(members)), Digest: hex.EncodeToString(h.Sum(nil))})
	}
	if len(expectedSets) != 58 {
		t.Fatalf("typed scope sets=%d, want 58", len(expectedSets))
	}
	var registeredSets []queryregistry.RequiredSet
	var manifest struct {
		RequiredSets []queryregistry.RequiredSet `json:"required_sets"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	registeredSets = manifest.RequiredSets
	if !reflect.DeepEqual(registeredSets, expectedSets) {
		t.Fatalf("required set rows differ from independent fold: got=%d want=%d", len(registeredSets), len(expectedSets))
	}
	setCounts := map[string]uint64{}
	for _, set := range expectedSets {
		setCounts[set.Kind] += set.Required
	}
	for kind, count := range map[string]uint64{
		queryregistry.Structural: 106208, queryregistry.Predicate: 40, queryregistry.Control: 94,
		"LARGE_WORKLOAD_POSITIVE": 3, "LARGE_WORKLOAD_CONTROL": 1,
	} {
		if setCounts[kind] != count {
			t.Fatalf("typed set total %s=%d, want %d", kind, setCounts[kind], count)
		}
	}
	if len(basis) != 2 {
		t.Fatal("predicate/control source basis is not fully pinned")
	}
	coverage, err := got.Fold(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	var required, blocked uint64
	for _, counter := range coverage.Counters {
		required += counter.Required
		blocked += counter.Blocked
	}
	if required != 106346 || blocked != required || coverage.FullScopeComplete || coverage.Verdict != "BLOCKED" {
		t.Fatalf("P0 empty-input fold = %+v", coverage)
	}
}

func independentCanonicalCoordinate(c queryregistry.Coordinate) ([]byte, error) {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode([]any{c.Kind, c.Route, c.Platform, c.Subject, c.Role, c.Set, c.Case, c.Query, c.Step, c.TreeRole, c.Context}); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'}), nil
}

func TestQuerySemanticRegistryRejectsMutants(t *testing.T) {
	data, qualification, slots, basis := queryRegistryInputs(t)
	var original map[string]any
	if err := json.Unmarshal(data, &original); err != nil {
		t.Fatal(err)
	}
	clone := func() map[string]any {
		raw, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	mutants := []struct {
		name   string
		change func(map[string]any)
	}{
		{"missing-route", func(d map[string]any) { d["ordinary"] = d["ordinary"].([]any)[1:] }},
		{"zero-required", func(d map[string]any) { d["required_sets"].([]any)[0].(map[string]any)["required"] = float64(0) }},
		{"same-count-other-route", func(d map[string]any) {
			rows := d["ordinary"].([]any)
			rows[1].(map[string]any)["route"] = rows[0].(map[string]any)["route"]
		}},
		{"host-subject", func(d map[string]any) { d["platforms"].([]any)[0].(map[string]any)["subject"] = "linux/amd64" }},
		{"case-selector", func(d map[string]any) {
			d["ordinary"].([]any)[0].(map[string]any)["cases_ref"] = "qualification_inventory#/routes/0/workload/cases/1:"
		}},
		{"large-selector", func(d map[string]any) {
			d["large_workloads"].([]any)[0].(map[string]any)["source_ref"] = "qualification_inventory#/extra_roles/0/workloads/0/cases/1"
		}},
		{"large-status", func(d map[string]any) {
			for _, raw := range d["large_workloads"].([]any) {
				row := raw.(map[string]any)
				if row["kind"] == "LARGE_WORKLOAD_CONTROL" {
					row["expected_status"] = "COMPLETED"
				}
			}
		}},
		{"large-denominator-zero", func(d map[string]any) { d["large_workloads"] = []any{} }},
		{"initial-fresh", func(d map[string]any) { p := d["predicate"].([]any)[0].(map[string]any); p["id"] = "eq-1" }},
		{"extra-reason", func(d map[string]any) {
			d["extra_roles"].([]any)[0].(map[string]any)["original"].(map[string]any)["reason"] = ""
		}},
		{"svc-zero", func(d map[string]any) {
			for _, raw := range d["extra_roles"].([]any) {
				row := raw.(map[string]any)
				if row["original"].(map[string]any)["id"] == "n461-svc" {
					row["registration"], row["required"] = "REGISTERED", float64(0)
				}
			}
		}},
		{"unavailable-pinned", func(d map[string]any) {
			a := d["assets"].([]any)[0].(map[string]any)
			a["state"], a["pin"] = "PINNED", map[string]any{"path": "missing.json", "bytes": float64(1), "sha256": queryregistry.Digest([]byte("x"))}
		}},
		{"namespace", func(d map[string]any) { d["membership_namespace_sha256"] = queryregistry.Digest([]byte("spoof")) }},
		{"full-complete", func(d map[string]any) { d["full_scope_complete"] = true }},
		{"empty-to-null", func(d map[string]any) { d["predicate"] = nil }},
	}
	for _, tc := range mutants {
		t.Run(tc.name, func(t *testing.T) {
			mutant := clone()
			tc.change(mutant)
			bad, err := json.Marshal(mutant)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := queryregistry.Validate(bad, queryregistry.Digest(bad), qualification, slots, basis); err == nil {
				t.Fatal("registry mutant accepted")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"qualification-source", func(b []byte) []byte { return bytes.Replace(b, []byte("csharp"), []byte("fsharp"), 1) }},
		{"slot-source", func(b []byte) []byte { return bytes.Replace(b, []byte("csharp"), []byte("fsharp"), 1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, s := qualification, slots
			if tc.name == "qualification-source" {
				q = tc.mutate(q)
			} else {
				s = tc.mutate(s)
			}
			if _, err := queryregistry.Validate(data, queryregistry.Digest(data), q, s, basis); err == nil {
				t.Fatal("rehashed source substitution accepted")
			}
		})
	}
	t.Run("source-rehashed-with-registry", func(t *testing.T) {
		q := bytes.Replace(qualification, []byte("csharp"), []byte("fsharp"), 1)
		reg, err := queryregistry.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		reg.Qualification = queryregistry.FilePin(queryregistry.QualificationPin.Path, q)
		reg.Namespace = queryregistry.Namespace(reg)
		mutant, err := jsonv2.Marshal(reg)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := queryregistry.Validate(mutant, queryregistry.Digest(mutant), q, slots, basis); err == nil {
			t.Fatal("source and its registry pin were both rehashed")
		}
	})
	for _, field := range []string{"source", "query"} {
		t.Run("predicate-"+field+"-namespace-rehashed", func(t *testing.T) {
			reg, err := queryregistry.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			if field == "source" {
				reg.Probes[0].Source = "changed source"
			} else {
				reg.Probes[0].Query = "(changed) @x"
			}
			reg.Namespace = queryregistry.Namespace(reg)
			mutant, err := jsonv2.Marshal(reg)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := queryregistry.Validate(mutant, queryregistry.Digest(mutant), qualification, slots, basis); err == nil {
				t.Fatalf("predicate %s and namespace both substituted", field)
			}
		})
	}
}

func TestQuerySemanticRegistryStrictJSON(t *testing.T) {
	data, _, _, _ := queryRegistryInputs(t)
	if _, err := queryregistry.Parse(data); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		input []byte
	}{
		{"duplicate-member", append([]byte(`{"schema":"bad",`), data[1:]...)},
		{"unknown-member", bytes.Replace(data, []byte(`"schema":`), []byte(`"unknown":1,"schema":`), 1)},
		{"invalid-utf8", bytes.Replace(data, []byte(`"schema": "`), append([]byte(`"schema": "`), 0xff), 1)},
		{"invalid-surrogate", bytes.Replace(data, []byte(`"tsgk-query-semantic-registry/r1"`), []byte(`"\ud800"`), 1)},
		{"truncated", bytes.TrimSpace(data)[:len(bytes.TrimSpace(data))-1]},
		{"deep", []byte(fmt.Sprintf(`{"schema":%s}`, string(bytes.Repeat([]byte{'['}, 70))+`0`+string(bytes.Repeat([]byte{']'}, 70))))},
		{"trailing-value", append(append([]byte(nil), data...), []byte(` {}`)...)},
		{"oversize", append(append([]byte(nil), data...), bytes.Repeat([]byte{' '}, queryregistry.MaxBytes)...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := queryregistry.Parse(tc.input); err == nil {
				t.Fatal("invalid strict JSON accepted")
			}
		})
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing-required", func(d map[string]any) { delete(d["required_sets"].([]any)[0].(map[string]any), "required") }},
		{"null-required", func(d map[string]any) { d["required_sets"].([]any)[0].(map[string]any)["required"] = nil }},
		{"string-required", func(d map[string]any) { d["required_sets"].([]any)[0].(map[string]any)["required"] = "1" }},
		{"null-predicate", func(d map[string]any) { d["predicate"] = nil }},
		{"missing-full-scope", func(d map[string]any) { delete(d, "full_scope_complete") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			var mutant map[string]any
			if err := json.Unmarshal(b, &mutant); err != nil {
				t.Fatal(err)
			}
			tc.mutate(mutant)
			b, err = json.Marshal(mutant)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := queryregistry.Parse(b); err == nil {
				t.Fatal("wrong member presence or primitive type accepted")
			}
		})
	}
}
