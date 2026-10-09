package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"
)

type tsqlContextCase struct {
	ID          string     `json:"id"`
	Source      string     `json:"source"`
	SHA256      string     `json:"sha256"`
	Nodes       []TreeNode `json:"nodes"`
	Codes       []string   `json:"codes"`
	SyntaxError bool       `json:"syntax_error"`
}

func tsqlContextCases(t *testing.T) []tsqlContextCase {
	t.Helper()
	b, err := os.ReadFile("../testdata/tsql-context/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ParserSHA256 string            `json:"parser_sha256"`
		Cases        []tsqlContextCase `json:"cases"`
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) != 726 {
		t.Fatal("missing native snapshot cases")
	}
	registryBytes, err := os.ReadFile("../contracts/native-routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Routes []struct {
			Route string `json:"route"`
			Files []struct {
				Role, SHA256 string
			} `json:"files"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(registryBytes, &registry); err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, route := range registry.Routes {
		for _, file := range route.Files {
			matched = matched || route.Route == "tsql" && file.Role == "parser" && file.SHA256 == fixture.ParserSHA256
		}
	}
	if !matched {
		t.Fatal("native snapshots do not match the registered T-SQL parser")
	}
	return fixture.Cases
}

func TestTSQLContextNativeSnapshots(t *testing.T) {
	for _, c := range tsqlContextCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			source := []byte(c.Source)
			hash := sha256.Sum256(source)
			if hex.EncodeToString(hash[:]) != c.SHA256 {
				t.Fatal("source snapshot changed")
			}
			before, err := json.Marshal(c.Nodes)
			if err != nil {
				t.Fatal(err)
			}
			got, err := CheckTSQLContext(c.Nodes, source, EncodingUTF8)
			if c.SyntaxError {
				if !hasErrCode(err, "TSQL_TREE_HAS_ERROR") || got != nil {
					t.Fatalf("syntax errors cannot yield a context success: %v %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			codes := []string{}
			previous := -1
			for _, d := range got {
				if d.Node < previous || d.Node >= len(c.Nodes) {
					t.Fatal("diagnostics are not in preorder")
				}
				previous = d.Node
				n := c.Nodes[d.Node]
				if d.Range != (Span{n.StartByte, n.EndByte, n.StartPoint, n.EndPoint}) {
					t.Fatal("diagnostic lost its original range")
				}
				codes = append(codes, d.Code)
			}
			if !slices.Equal(codes, c.Codes) {
				t.Fatalf("got %v; want %v", codes, c.Codes)
			}
			after, _ := json.Marshal(c.Nodes)
			if !reflect.DeepEqual(before, after) || string(source) != c.Source {
				t.Fatal("caller input changed")
			}
		})
	}
}

func TestTSQLContextEncodingsAndInputGuards(t *testing.T) {
	var sample tsqlContextCase
	samples := []tsqlContextCase{}
	ids := []string{"hint-comma-conflict", "issue151-literal", "issue151-local", "issue151-global", "issue151-remote-literal", "issue151-remote-local"}
	for _, c := range tsqlContextCases(t) {
		if slices.Contains(ids, c.ID) {
			samples = append(samples, c)
		}
		if c.ID == "hint-comma-conflict" {
			sample = c
		}
	}
	if sample.ID == "" || len(samples) != len(ids) {
		t.Fatal("missing encoding control")
	}
	for _, sample := range samples {
		for _, enc := range []string{EncodingUTF8, EncodingCP949, EncodingUTF16LE, EncodingUTF16BE} {
			nodes, source := slices.Clone(sample.Nodes), []byte(sample.Source)
			if enc == EncodingUTF16LE || enc == EncodingUTF16BE {
				source = nil
				for _, b := range []byte(sample.Source) {
					if enc == EncodingUTF16LE {
						source = append(source, b, 0)
					} else {
						source = append(source, 0, b)
					}
				}
				for i := range nodes {
					nodes[i].StartByte *= 2
					nodes[i].EndByte *= 2
					nodes[i].StartPoint.Column *= 2
					nodes[i].EndPoint.Column *= 2
				}
			}
			got, err := CheckTSQLContext(nodes, source, enc)
			codes := make([]string, len(got))
			for i, d := range got {
				codes[i] = d.Code
			}
			if err != nil || !slices.Equal(codes, sample.Codes) {
				t.Fatalf("%s %s: %v %v", sample.ID, enc, got, err)
			}
			if enc == EncodingUTF16LE {
				nodes[1].StartByte++
				if _, err := CheckTSQLContext(nodes, source, enc); !hasErrCode(err, "TREE_ENCODING_BOUNDARY") {
					t.Fatalf("odd UTF16 node accepted: %v", err)
				}
			}
		}
	}
	for _, tc := range []struct {
		name, code, encoding string
		change               func([]TreeNode) []TreeNode
	}{
		{"empty", "TREE_EMPTY", EncodingUTF8, func([]TreeNode) []TreeNode { return nil }},
		{"parent", "TREE_PARENT_INVALID", EncodingUTF8, func(n []TreeNode) []TreeNode { n[1].Parent = 1; return n }},
		{"range", "TREE_RANGE_INVALID", EncodingUTF8, func(n []TreeNode) []TreeNode { n[1].EndByte = 99999; return n }},
		{"error", "TSQL_TREE_HAS_ERROR", EncodingUTF8, func(n []TreeNode) []TreeNode { n[1].IsError = true; return n }},
		{"missing", "TSQL_TREE_HAS_ERROR", EncodingUTF8, func(n []TreeNode) []TreeNode { n[1].IsMissing = true; return n }},
		{"has-error", "TSQL_TREE_HAS_ERROR", EncodingUTF8, func(n []TreeNode) []TreeNode { n[1].HasError = true; return n }},
		{"encoding", "SOURCE_ENCODING_INVALID", "unknown", func(n []TreeNode) []TreeNode { return n }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CheckTSQLContext(tc.change(slices.Clone(sample.Nodes)), []byte(sample.Source), tc.encoding)
			if !hasErrCode(err, tc.code) || got != nil {
				t.Fatalf("input guard failed: %v %v", got, err)
			}
		})
	}
	if _, err := CheckTSQLContext(sample.Nodes, []byte{0x81}, EncodingCP949); !hasErrCode(err, "SOURCE_ENCODING_INVALID") {
		t.Fatalf("invalid CP949 accepted: %v", err)
	}
}
