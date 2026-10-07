package main

import (
	"bytes"
	"encoding/json"
	"go/build"
	"os"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestStandaloneCExcludedFromGoPackage(t *testing.T) {
	ctx := build.Default
	ctx.CgoEnabled = true
	pkg, err := ctx.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.CFiles) != 0 || len(pkg.CgoFiles) != 0 {
		t.Fatalf("standalone native audit entered Go package: C=%v cgo=%v", pkg.CFiles, pkg.CgoFiles)
	}
}

func TestReceiptRejectsTampering(t *testing.T) {
	req := native.Request{ID: "control", Source: []byte("a"), Limits: native.LimitsFor(kit.NativeOperations()["native-query-large"])}
	r := receipt{Schema: "tsgk-large-api-audit/r1", ID: req.ID, SourceSHA: digest(req.Source), SourceBytes: 1, Nodes: 2, End: 2, Depth: 2, Fields: 1, Checks: 37, ChildFields: 1, FieldLookups: 4, PointChecks: 6, First: []int64{0, 0, 0, 0}, FirstDivergence: []int64{0, 0, 0, 0}, ExpectedSHA: strings.Repeat("a", 64), ObservedSHA: strings.Repeat("a", 64), Complete: true}
	marshal := func(v receipt) []byte {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	good := marshal(r)
	if _, err := decode(native.Frame(good), req, 0, 2, 2); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*receipt){
		"source":      func(r *receipt) { r.SourceSHA = strings.Repeat("b", 64) },
		"identity":    func(r *receipt) { r.ID = "other" },
		"completion":  func(r *receipt) { r.Complete = false },
		"node_count":  func(r *receipt) { r.Nodes++ },
		"fields":      func(r *receipt) { r.FieldLookups-- },
		"children":    func(r *receipt) { r.ChildFields-- },
		"points":      func(r *receipt) { r.PointChecks-- },
		"checks":      func(r *receipt) { r.Checks-- },
		"range_start": func(r *receipt) { r.Start = 1 },
		"range_end":   func(r *receipt) { r.End = 1 },
		"digest":      func(r *receipt) { r.ObservedSHA = strings.Repeat("b", 64) },
		"difference":  func(r *receipt) { r.Differences = 1 },
		"first":       func(r *receipt) { r.First = []int64{0, 0, 0, 1} },
	}
	for name, change := range mutations {
		t.Run(name, func(t *testing.T) {
			bad := r
			change(&bad)
			if _, err := decode(native.Frame(marshal(bad)), req, 0, 2, 2); err == nil {
				t.Fatal("tampered receipt accepted")
			}
		})
	}
	for name, bad := range map[string][]byte{
		"duplicate": bytes.Replace(good, []byte(`"complete":true`), []byte(`"complete":false,"complete":true`), 1),
		"unknown":   bytes.Replace(good, []byte(`"complete":true`), []byte(`"unknown":1,"complete":true`), 1),
		"trailing":  append(append([]byte{}, good...), []byte(`{}`)...),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decode(native.Frame(bad), req, 0, 2, 2); err == nil {
				t.Fatal("invalid JSON accepted")
			}
		})
	}
	for _, first := range [][]int64{{1 << 62, 2, 1, 2}, {0, 999999, 1, 2}, {0, 1000004, 1, 2}, {0, 2000003, 1, 2}, {2, 2000000, 1, 2}} {
		bad := r
		bad.Differences = 1
		bad.ObservedSHA = strings.Repeat("b", 64)
		bad.First = first
		if _, err := decode(native.Frame(marshal(bad)), req, 0, 2, 2); err == nil {
			t.Fatal("invalid first difference coordinates accepted")
		}
	}
	frame := native.Frame(good)
	for _, bad := range [][]byte{frame[:len(frame)-1], append(append([]byte{}, frame...), 0)} {
		if _, err := decode(bad, req, 0, 2, 2); err == nil {
			t.Fatal("invalid frame accepted")
		}
	}
}

func TestFixtureInventoryRejectsReplacement(t *testing.T) {
	data, e := os.ReadFile("../../contracts/native-large-fixtures.json")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = loadFixtures(data); e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"block", "sha256", "id", "points", "repeat"} {
		var x map[string]any
		if e = json.Unmarshal(data, &x); e != nil {
			t.Fatal(e)
		}
		f := x["fixtures"].([]any)[0].(map[string]any)
		f[key] = "changed"
		bad, e := json.Marshal(x)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = loadFixtures(bad); e == nil {
			t.Fatal("replacement accepted: " + key)
		}
	}
	for _, bad := range [][]byte{append([]byte(" "), data...), bytes.Replace(data, []byte(`"schema":`), []byte(`"unknown":1,"schema":`), 1), bytes.Replace(data, []byte(`"schema":`), []byte(`"schema":"duplicate","schema":`), 1)} {
		if _, e = loadFixtures(bad); e == nil {
			t.Fatal("inventory mutation accepted")
		}
	}
}
