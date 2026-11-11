package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/queryregistry"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func testInputs(t *testing.T) *inputs {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	in, err := load(root)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestWitnessReconstruction(t *testing.T) {
	in := testInputs(t)
	out, err := build(in)
	if err != nil {
		t.Fatal(err)
	}
	c := out.Counts
	if c.Cases != 3201 || c.Embedded != 154 || c.Recovered != 3047 || c.Edits != 1852 || c.Steps != 5053 || c.TreeRoles != 6905 || c.Queries != 130 || c.InitialUnique != 3008 || c.Versions != 3963 || c.Bundles != 31337 || c.InitialBytes != 1159599 {
		t.Fatalf("counts changed: %+v", c)
	}
	bundles := map[string]bool{}
	for ri, r := range out.Routes {
		versions := map[string][]byte{}
		for _, v := range r.Versions {
			if queryregistry.Digest(v.Source) != v.SHA256 || uint64(len(v.Source)) != v.Bytes {
				t.Fatal("version pin mismatch")
			}
			if _, ok := versions[v.SHA256]; ok {
				t.Fatal("duplicate version")
			}
			versions[v.SHA256] = v.Source
			for _, q := range r.Queries {
				key := r.Route + "/" + r.Encoding + "/" + r.Context + "/" + q.ID + "/" + q.SHA256 + "/" + v.SHA256
				if bundles[key] {
					t.Fatal("duplicate query/source bundle")
				}
				bundles[key] = true
			}
		}
		seen := map[string]bool{}
		for ci, wc := range r.Cases {
			c := in.inv.Routes[ri].Workload.Cases[ci]
			selector := fmt.Sprintf("qualification_inventory#/routes/%d/workload/cases/%d", ri, ci)
			if wc.ID != c.ID || wc.Selector != selector || seen[wc.ID] || len(wc.Steps) != 1+len(c.Edits) || len(wc.Points) != len(c.Edits) {
				t.Fatalf("case membership changed: %s", wc.ID)
			}
			seen[wc.ID] = true
			cur, ok := versions[wc.Steps[0]]
			if !ok || queryregistry.Digest(cur) != c.Input.SHA256 || uint64(len(cur)) != c.Input.Bytes {
				t.Fatalf("initial bytes/hash/length: %s", wc.ID)
			}
			for i, e := range c.Edits {
				start, end := int(e.StartByte), int(e.OldEndByte)
				if start > end || end > len(cur) || !bytes.Equal(cur[start:end], e.Old) || e.NewEndByte != e.StartByte+uint32(len(e.New)) {
					t.Fatal("independent edit check failed")
				}
				// Independent reconstruction does not call ApplyEdits or reconstruct.
				next := append([]byte{}, cur[:start]...)
				next = append(next, e.New...)
				next = append(next, cur[end:]...)
				actual, ok := versions[wc.Steps[i+1]]
				if !ok || !bytes.Equal(next, actual) || len(next) != len(actual) || queryregistry.Digest(next) != wc.Steps[i+1] {
					t.Fatalf("step hash/length/order: %s/%d", wc.ID, i+1)
				}
				point := func(s []byte, at int) kit.Point {
					before := s[:at]
					return kit.Point{Row: uint32(bytes.Count(before, []byte{'\n'})), Column: uint32(at - bytes.LastIndexByte(before, '\n') - 1)}
				}
				p := wc.Points[i]
				if p.Start != point(cur, start) || p.OldEnd != point(cur, end) || p.NewEnd != point(next, int(e.NewEndByte)) {
					t.Fatal("edit point mismatch")
				}
				cur = next
			}
			if len(wc.Origins) == 0 {
				t.Fatal("missing original provenance")
			}
			for _, o := range wc.Origins {
				data, err := in.read(o.File.Path)
				if err != nil || queryregistry.FilePin(o.File.Path, data) != o.File || o.Selector == "" || o.Case == "" {
					t.Fatal("origin pin mismatch")
				}
			}
		}
	}
	if len(bundles) != 31337 {
		t.Fatal("query/source factoring changed")
	}
	encoded, err := encode(out)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip export
	if err := json.Unmarshal(encoded, &roundtrip); err != nil || !reflect.DeepEqual(out, roundtrip) {
		t.Fatalf("source byte roundtrip failed: %v", err)
	}
	t.Logf("reconstructed cases=%d edits=%d versions=%d bundles=%d initial_bytes=%d step_bytes=%d unique_bytes=%d export_bytes=%d export_sha256=%s", c.Cases, c.Edits, c.Versions, c.Bundles, c.InitialBytes, c.StepBytes, c.UniqueBytes, len(encoded), queryregistry.Digest(encoded))
}

func TestWitnessRejectsMutants(t *testing.T) {
	in := testInputs(t)
	route := in.inv.Routes[0]
	sources, err := in.sources(route.Route)
	if err != nil {
		t.Fatal(err)
	}
	var c kit.QualCase
	var s recovered
	for _, candidate := range route.Workload.Cases {
		if len(candidate.Edits) > 1 {
			c = candidate
			s = sources[c.ID]
			break
		}
	}
	if len(s.data) == 0 {
		t.Fatal("mutant fixture not selected")
	}
	if _, _, err := reconstruct(c, s); err != nil {
		t.Fatal(err)
	}
	t.Run("source-byte", func(t *testing.T) {
		bad := s
		bad.data = bytes.Clone(s.data)
		bad.data[0] ^= 1
		if _, _, err := reconstruct(c, bad); err == nil {
			t.Fatal("changed source byte accepted")
		}
	})
	t.Run("edit-order", func(t *testing.T) {
		bad := c
		bad.Edits = append([]kit.Edit{}, c.Edits...)
		bad.Edits[0], bad.Edits[1] = bad.Edits[1], bad.Edits[0]
		if _, _, err := reconstruct(bad, s); err == nil {
			t.Fatal("changed edit order accepted")
		}
	})
	t.Run("same-hash-alias", func(t *testing.T) {
		delete(sources, c.ID)
		var qf queryFile
		if _, err := in.recover(route.Route, c, sources, qf, ""); err == nil {
			t.Fatal("missing original case accepted by content alias")
		}
	})
}

func TestExpectationRuleLedger(t *testing.T) {
	in := testInputs(t)
	ledger, err := buildRules(in)
	if err != nil {
		t.Fatal(err)
	}
	data, err := encode(ledger)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := in.read(rulesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, data) {
		t.Fatal("rule ledger drift; explicitly regenerate with -rules")
	}
	if len(data) > maxRules {
		t.Fatal("rule ledger exceeds 256 KiB")
	}
	if len(ledger.Routes) != 26 || len(ledger.Rules) != 130 {
		t.Fatal("rule ledger membership")
	}
	i := 0
	for ri, r := range in.inv.Routes {
		if !reflect.DeepEqual(ledger.Routes[ri].Grammar, r.Workload.Grammar) {
			t.Fatal("grammar basis mismatch")
		}
		for _, q := range r.Workload.Queries {
			row := ledger.Rules[i]
			i++
			if row.Route != r.Route || row.Query != q.ID || row.SHA256 != q.SHA256 || queryregistry.Digest([]byte(row.Declaration)) != q.SHA256 || row.GrammarRef != fmt.Sprintf("#/routes/%d", ri) || row.Exceptions.ManualCount != nil || row.Review != nil || row.Exceptions.Items == nil {
				t.Fatal("rule membership/pin or incomplete state changed")
			}
			if row.Source.Selector == "" || row.Rule == "" || row.Automation == "" || row.Scope == "" {
				t.Fatal("missing rule disposition")
			}
		}
	}
}

func TestJSONIndependentAnchor(t *testing.T) {
	in := testInputs(t)
	var anchors struct {
		Cases []struct {
			ID          string `json:"id"`
			SourceCase  string `json:"source_case"`
			QueryExpect []struct {
				Query    string           `json:"query"`
				Step     int              `json:"step"`
				Status   string           `json:"status"`
				Code     string           `json:"code"`
				Error    *json.RawMessage `json:"error"`
				Captures []struct {
					Name string `json:"name"`
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"captures"`
			} `json:"query_expect"`
		} `json:"cases"`
	}
	if err := in.decode("src/testdata/native/queries/json.json", &anchors); err != nil {
		t.Fatal(err)
	}
	var source []byte
	for _, r := range in.inv.Routes {
		if r.Route == "json" {
			for _, c := range r.Workload.Cases {
				if c.ID == "json-query-r1" {
					source = []byte(*c.Source)
				}
			}
		}
	}
	got, err := jsonExpect(source)
	if err != nil {
		t.Fatal(err)
	}
	caseIndex := -1
	for i, c := range anchors.Cases {
		if c.ID == "json-query-r1" {
			if caseIndex >= 0 {
				t.Fatal("duplicate manual anchor case")
			}
			caseIndex = i
		}
	}
	if caseIndex < 0 || anchors.Cases[caseIndex].SourceCase != "json-feature-r1" || len(anchors.Cases[caseIndex].QueryExpect) != 1 {
		t.Fatal("manual anchor case/source_case mismatch")
	}
	sources, err := in.sources("json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sources[anchors.Cases[caseIndex].SourceCase].data, source) {
		t.Fatal("manual anchor source_case bytes mismatch")
	}
	anchor := anchors.Cases[caseIndex].QueryExpect[0]
	if anchor.Query != "q.structure" || anchor.Step != 0 || anchor.Status != "COMPLETED" || anchor.Code != "" || anchor.Error != nil || len(anchor.Captures) != 13 || len(got) != len(anchor.Captures) {
		t.Fatal("manual anchor membership changed")
	}
	for i, want := range anchor.Captures {
		c := got[i]
		if c.Name != want.Name || c.Type != want.Type || c.Text != want.Text || c.Text != string(source[c.StartByte:c.EndByte]) {
			t.Fatalf("independent capture %d = %+v, anchor=%+v", i, c, want)
		}
	}
	t.Logf("independent JSON rule agrees with all %d ordered manual name/type/text anchors; source_sha256=%s", len(got), queryregistry.Digest(source))
}

func TestJSONIndependentEdges(t *testing.T) {
	source := []byte("{\n\"λ\":\"가\\n\\\"\",\"λ\":[],\"x\":{\"x\":\"y\"}}")
	got, err := jsonExpect(source)
	if err != nil {
		t.Fatal(err)
	}
	want := []jsonCapture{
		{"key", "string", "\"λ\"", 2, 6, 2, 0, 0},
		{"value", "string", "\"가\\n\\\"\"", 7, 16, 2, 0, 1},
		{"array_key", "string", "\"λ\"", 17, 21, 0, 0, 0},
		{"object_key", "string", "\"x\"", 25, 28, 1, 0, 0},
		{"key", "string", "\"x\"", 30, 33, 2, 1, 0},
		{"value", "string", "\"y\"", 34, 37, 2, 1, 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("independent Unicode/duplicate/nested spans\ngot  %+v\nwant %+v", got, want)
	}
	for _, source := range []string{"null", "[]", "{}", "{\"n\":3,\"b\":true,\"z\":null}"} {
		got, err := jsonExpect([]byte(source))
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("expected explicit empty captures: %s %v", source, err)
		}
	}
	for _, source := range []string{"", "{", "{} {}", "{\"x\":1,}", "/*c*/null", "\"\\x\"", string([]byte{'"', 0xff, '"'}), strings.Repeat("[", 66) + "0" + strings.Repeat("]", 66), strings.Repeat(" ", maxSource) + "0"} {
		if _, err := jsonExpect([]byte(source)); err == nil {
			t.Fatal("unsupported/malformed JSON accepted")
		}
	}
}
