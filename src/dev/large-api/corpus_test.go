package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func publicPage() corpusPage {
	return corpusPage{receipt: receipt{Schema: "tsgk-corpus-cst-api-page/r1", ID: "control", SourceSHA: digest([]byte("a")), SourceBytes: 1, Nodes: 2, End: 2, Depth: 2, Fields: 1, Checks: 37, ChildFields: 1, FieldLookups: 4, PointChecks: 6, First: []int64{0, 0, 0, 0}, FirstDivergence: []int64{0, 0, 0, 0}, ExpectedSHA: strings.Repeat("a", 64), ObservedSHA: strings.Repeat("a", 64), Complete: true}, Encoding: kit.EncodingUTF8, CST: cstPage{Types: []string{"root", "leaf"}, Fields: []string{"value"}, Nodes: [][]int64{{-1, 0, -1, 1, 0, 1, 0, 0, 0, 1}, {0, 1, 0, 1, 0, 1, 0, 0, 0, 1}}}}
}

func TestCorpusPagesRecomputeCanonicalDigest(t *testing.T) {
	p := publicPage()
	full := []kit.TreeNode{{Parent: -1, Type: "root", Named: true, EndByte: 1, EndPoint: kit.Point{Column: 1}}, {Parent: 0, Type: "leaf", Field: &p.CST.Fields[0], Named: true, EndByte: 1, EndPoint: kit.Point{Column: 1}}}
	tree := newCorpusTree()
	first := p
	first.End = 1
	first.CST.Nodes = p.CST.Nodes[:1]
	first.ChildFields = 1
	second := p
	second.Start = 1
	second.CST.Nodes = p.CST.Nodes[1:]
	second.ChildFields = 0
	if err := tree.consume(first, 1); err != nil {
		t.Fatal(err)
	}
	if err := tree.consume(second, 1); err != nil {
		t.Fatal(err)
	}
	if tree.next != 2 || tree.children != 1 || hex.EncodeToString(tree.h.Sum(nil)) != kit.TreeDigest(full) {
		t.Fatal("stream disagrees with canonical full tree")
	}
	for _, mutate := range []func(*corpusPage){func(p *corpusPage) { p.CST.Types[1] = "other" }, func(p *corpusPage) { p.CST.Fields[0] = "other" }, func(p *corpusPage) { p.CST.Nodes[1][3] |= 2 }, func(p *corpusPage) { p.CST.Nodes[1][3] |= 16 }, func(p *corpusPage) { p.CST.Nodes[1][7] = 1 }} {
		bad := publicPage()
		mutate(&bad)
		s := newCorpusTree()
		if err := s.consume(bad, 1); err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(s.h.Sum(nil)) == kit.TreeDigest(full) {
			t.Fatal("CST mutation did not affect independent canonical digest")
		}
	}
}

func TestCorpusPageRejectsGapAndInvalidStructure(t *testing.T) {
	changes := map[string]func(*corpusPage){
		"gap":        func(p *corpusPage) { p.Start = 1 },
		"navigation": func(p *corpusPage) { p.Divergences = 1 },
		"difference": func(p *corpusPage) { p.Differences = 1 },
		"row":        func(p *corpusPage) { p.CST.Nodes[1] = p.CST.Nodes[1][:9] },
		"root":       func(p *corpusPage) { p.CST.Nodes[0][0] = 0 },
		"parent":     func(p *corpusPage) { p.CST.Nodes[1][0] = 1 },
		"field":      func(p *corpusPage) { p.CST.Nodes[0][2] = 0 },
		"flags":      func(p *corpusPage) { p.CST.Nodes[1][3] = 32 },
		"range":      func(p *corpusPage) { p.CST.Nodes[1][5] = 2 },
		"point":      func(p *corpusPage) { p.CST.Nodes[1][6] = 2 },
		"catalog":    func(p *corpusPage) { p.CST.Nodes[1][1] = 2 },
		"root_error": func(p *corpusPage) { p.HasError = true },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			p := publicPage()
			change(&p)
			if newCorpusTree().consume(p, 1) == nil {
				t.Fatal("invalid CST/API page accepted")
			}
		})
	}
}

func TestCorpusDecoderBindsEncodingAndAllPageMembers(t *testing.T) {
	req := native.Request{ID: "control", Encoding: kit.EncodingUTF8, Source: []byte("a"), Limits: native.LimitsFor(kit.NativeOperations()["native-query-large"])}
	p := publicPage()
	good, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = decodeCorpusPage(native.Frame(good), req, 0, 2, 2); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{bytes.Replace(good, []byte(`"encoding":"UTF-8"`), []byte(`"encoding":"CP949"`), 1), bytes.Replace(good, []byte(`"has_error":false`), []byte(`"unknown":false`), 1), bytes.Replace(good, []byte(`"encoding":"UTF-8"`), []byte(`"encoding":"UTF-8","encoding":"UTF-8"`), 1)} {
		if _, err = decodeCorpusPage(native.Frame(bad), req, 0, 2, 2); err == nil {
			t.Fatal("page binding tamper accepted")
		}
	}
}

func TestCorpusPinnedReadRejectsDriftLinksAndLimits(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "input")
	if err := os.WriteFile(p, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	good := pinnedFile{Path: "input", SHA: digest([]byte("abc")), Bytes: 3}
	if b, err := readPinned(root, good, 3); err != nil || string(b) != "abc" {
		t.Fatalf("exact limit: %v", err)
	}
	for _, bad := range []pinnedFile{{Path: "../input", SHA: good.SHA, Bytes: 3}, {Path: "input", SHA: good.SHA, Bytes: 2}, {Path: "input", SHA: strings.Repeat("0", 64), Bytes: 3}} {
		if _, err := readPinned(root, bad, 3); err == nil {
			t.Fatal("drift or traversal accepted")
		}
	}
	if _, err := readPinned(root, good, 2); err == nil {
		t.Fatal("one-byte-over accepted")
	}
	if err := os.Link(p, filepath.Join(dir, "alias")); err == nil {
		if _, err := readPinned(root, good, 3); err == nil {
			t.Fatal("hardlink accepted")
		}
	}
}

func TestCorpusEvidenceStorageExactAndOneByteOver(t *testing.T) {
	q := &corpusQuota{limit: 3}
	var b bytes.Buffer
	w := corpusQuotaWriter{q, &b}
	if n, err := w.Write([]byte("abc")); err != nil || n != 3 || q.used != 3 {
		t.Fatalf("exact storage: %d %v", n, err)
	}
	if n, err := w.Write([]byte("x")); err == nil || n != 0 || q.used != 3 || b.String() != "abc" {
		t.Fatal("one-byte-over changed evidence")
	}
}

func TestCorpusEnvelopeOnlyLowersLimitsAndBindsPolicy(t *testing.T) {
	for _, p := range []corpusPlan{{}, {Storage: corpusStorageBytes, Wall: 3600}, {Storage: 3, Wall: 1}} {
		s, w, err := corpusEnvelope(p)
		if err != nil || s == 0 || s > corpusStorageBytes || w == 0 || w > 3600 {
			t.Fatalf("valid envelope refused: %v", err)
		}
	}
	for _, p := range []corpusPlan{{Storage: corpusStorageBytes + 1}, {Wall: 3601}} {
		if _, _, err := corpusEnvelope(p); err == nil {
			t.Fatal("raised limit accepted")
		}
	}
	base := corpusPolicySHA(corpusStorageBytes, 3600)
	if base == corpusPolicySHA(corpusStorageBytes-1, 3600) || base == corpusPolicySHA(corpusStorageBytes, 3599) {
		t.Fatal("lowered limits did not change policy identity")
	}
}
