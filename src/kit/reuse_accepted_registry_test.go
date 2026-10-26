package kit

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func acceptedLedgerFixture(t *testing.T) ([]byte, *ReuseAcceptedLedger) {
	t.Helper()
	raw, err := os.ReadFile("../testdata/native/reuse-holdouts.json")
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := ParseReuseAcceptedLedger(raw)
	if err != nil {
		t.Fatal(err)
	}
	return raw, ledger
}

func acceptedOriginalCase(h reuseAcceptedOriginal) IncrementalCase {
	return IncrementalCase{ID: h.ID, Encoding: EncodingUTF8, Input: NativeInput{Path: h.ID + ".txt", Role: "source", Bytes: uint64(len(h.SourceUTF8)), SHA256: digestHex([]byte(h.SourceUTF8))}, Edits: h.edits()}
}

func TestReuseAcceptedLedger(t *testing.T) {
	raw, l := acceptedLedgerFixture(t)
	if len(l.originals) != 15 {
		t.Fatal(len(l.originals))
	}
	edits, targets, tsql := 0, 0, 0
	for _, h := range l.originals {
		edits += len(h.Edits)
		targets += len(h.BlockedSteps)
		if h.Route == "tsql" {
			tsql++
		}
		if err := l.CheckCase(h.Route, acceptedOriginalCase(h), []byte(h.SourceUTF8)); err != nil {
			t.Fatal(h.ID, err)
		}
	}
	if edits != 43 || targets != 25 || tsql != 12 {
		t.Fatalf("edits=%d targets=%d tsql=%d", edits, targets, tsql)
	}
	compact := new(bytes.Buffer)
	if err := json.Compact(compact, raw); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{compact.Bytes(), bytes.ReplaceAll(raw, []byte("\n"), []byte("\r\n"))} {
		if _, err := ParseReuseAcceptedLedger(data); err != nil {
			t.Fatal("format-independent identity", err)
		}
	}
	for name, mutate := range map[string]func(map[string]any){
		"drop-original": func(d map[string]any) { d["holdouts"] = d["holdouts"].([]any)[1:] },
		"reorder":       func(d map[string]any) { a := d["holdouts"].([]any); a[0], a[1] = a[1], a[0] },
		"targets":       func(d map[string]any) { d["holdouts"].([]any)[0].(map[string]any)["blocked_steps"] = []int{1} },
		"omit-step":     func(d map[string]any) { d["holdouts"].([]any)[0].(map[string]any)["omit_old_tree_first_step"] = 2 },
		"origin": func(d map[string]any) {
			d["holdouts"].([]any)[0].(map[string]any)["origin"].(map[string]any)["issue"] = 197
		},
		"sibling": func(d map[string]any) { d["holdouts"].([]any)[0].(map[string]any)["sibling"] = "" },
		"edit": func(d map[string]any) {
			d["holdouts"].([]any)[0].(map[string]any)["edits"].([]any)[0].(map[string]any)["old_utf8"] = "b"
		},
		"missing-field": func(d map[string]any) { delete(d["holdouts"].([]any)[0].(map[string]any), "source_utf8") },
		"null-field":    func(d map[string]any) { d["holdouts"].([]any)[0].(map[string]any)["source_utf8"] = nil },
		"unknown":       func(d map[string]any) { d["accepted_claim"] = "PASS" },
	} {
		t.Run(name, func(t *testing.T) {
			var d map[string]any
			if err := json.Unmarshal(raw, &d); err != nil {
				t.Fatal(err)
			}
			mutate(d)
			b, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseReuseAcceptedLedger(b); err == nil {
				t.Fatal("accepted altered registration")
			}
		})
	}
	for name, data := range map[string][]byte{
		"duplicate": bytes.Replace(raw, []byte(`"schema":`), []byte(`"schema":"tsgk-reuse-holdouts/r1","schema":`), 1),
		"exponent":  bytes.Replace(raw, []byte(`"start_byte": 13`), []byte(`"start_byte": 13e0`), 1),
		"trailing":  append(slices.Clone(raw), []byte(`{}`)...),
		"utf8":      []byte("{\"schema\":\"\xff\"}"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseReuseAcceptedLedger(data); err == nil {
				t.Fatal("accepted malformed registration")
			}
		})
	}
	h := l.originals[0]
	for name, mutate := range map[string]func(*IncrementalCase){
		"sibling-id":                   func(c *IncrementalCase) { c.ID = h.Sibling },
		"different-history-same-final": func(c *IncrementalCase) { c.Edits = c.Edits[2:] },
		"encoding":                     func(c *IncrementalCase) { c.Encoding = EncodingCP949 },
		"source-hash":                  func(c *IncrementalCase) { c.Input.SHA256 = strings.Repeat("0", 64) },
		"extra-point":                  func(c *IncrementalCase) { c.Points = []NativePoint{{ID: "p", Byte: 0}} },
	} {
		t.Run(name, func(t *testing.T) {
			c := acceptedOriginalCase(h)
			mutate(&c)
			if err := l.CheckCase(h.Route, c, []byte(h.SourceUTF8)); err == nil {
				t.Fatal("accepted different case")
			}
		})
	}
}
