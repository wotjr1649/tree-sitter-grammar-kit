package kit

import (
	"encoding/json"
	"fmt"

	jsonv2 "encoding/json/v2"
)

// The raw native observation of the G1 sources (#233 G1-3a, native comparison).
//
// The capture command (src/dev/csharp-g1-native) parses each source the sealed expectations name
// with the registered tree-sitter-c-sharp build and writes this document. The comparison
// (csharp_g1_executed.go) reads it and the sealed assets; nothing here feeds back into the expected
// asset or the kind map. The document is a local artifact: the tracked executed asset keeps the
// evidence the comparison used and the digest of each tree, not the trees.

const (
	CSharpG1NativeRawSchema = "tsgk-csharp-g1-native-raw/r1"
	// shortcut: the raw document is a local artifact of about 20 MiB; raise with the corpus.
	csharpG1NativeRawMax = 256 << 20

	G1NativeCompleted = "COMPLETED"

	// node flags of G1NativeSource.Nodes
	G1NodeNamed   = 1
	G1NodeExtra   = 2
	G1NodeError   = 4
	G1NodeMissing = 8
)

// G1NativeProvenance names the build and the limits the capture ran under.
type G1NativeProvenance struct {
	Route           string         `json:"route"`
	Symbol          string         `json:"symbol"`
	Platform        string         `json:"platform"`
	Grammar         []G1NativeFile `json:"grammar"`
	DriverIdentity  string         `json:"driver_identity"`
	Compiler        string         `json:"compiler"`
	CompilerSHA256  string         `json:"compiler_sha256"`
	CompilerVersion string         `json:"compiler_version"`
	RuntimeCommit   string         `json:"runtime_commit"`
	Operation       string         `json:"operation"`
	NativeMemory    uint64         `json:"native_memory_bytes"`
	NativeWallMS    int64          `json:"native_wall_ms"`
	BuildMemory     uint64         `json:"build_memory_bytes"`
	BuildWallMS     int64          `json:"build_wall_ms"`
}

// G1NativeFile is one input of the native build closure.
type G1NativeFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// G1NativeSource is the native parse of one source version. Nodes are in preorder:
// [type index into Types, flags, start byte, end byte, parent index (-1 for the root)].
type G1NativeSource struct {
	ID         string   `json:"id"`
	SHA256     string   `json:"sha256"`
	Bytes      int      `json:"bytes"`
	Status     string   `json:"status"`
	Code       string   `json:"code"`
	HasError   bool     `json:"has_error"`
	TreeDigest string   `json:"tree_digest"`
	Types      []string `json:"types"`
	Nodes      [][5]int `json:"nodes"`
}

// CSharpG1NativeRaw is the decoded tsgk-csharp-g1-native-raw/r1 document.
type CSharpG1NativeRaw struct {
	Schema     string             `json:"schema"`
	Provenance G1NativeProvenance `json:"provenance"`
	Sources    []G1NativeSource   `json:"sources"`
}

func g1nFault(code string) error { return fail(KindInvalidInput, code, "csharp-g1-native-raw", nil) }

// DecodeCSharpG1NativeRaw decodes the document strictly and checks its structure: every source id
// once, node indexes and spans in range, parents before children.
func DecodeCSharpG1NativeRaw(data []byte) (CSharpG1NativeRaw, error) {
	var x CSharpG1NativeRaw
	if len(data) > csharpG1NativeRawMax {
		return x, fail(KindResourceLimit, "G1_NATIVE_SIZE", "csharp-g1-native-raw", nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_NATIVE_JSON", "csharp-g1-native-raw", err)
	}
	if x.Schema != CSharpG1NativeRawSchema {
		return x, g1nFault("G1_NATIVE_SCHEMA")
	}
	seen := map[string]bool{}
	for _, s := range x.Sources {
		if s.ID == "" || seen[s.ID] || len(s.SHA256) != 64 || s.Bytes < 0 {
			return x, g1nFault("G1_NATIVE_SOURCE")
		}
		seen[s.ID] = true
		if s.Status != G1NativeCompleted {
			if len(s.Nodes) != 0 || s.Code == "" {
				return x, g1nFault("G1_NATIVE_SOURCE")
			}
			continue
		}
		if len(s.Nodes) == 0 || s.Nodes[0][4] != -1 || s.TreeDigest == "" {
			return x, g1nFault("G1_NATIVE_TREE")
		}
		for i, n := range s.Nodes {
			if n[0] < 0 || n[0] >= len(s.Types) || n[2] < 0 || n[3] < n[2] || n[3] > s.Bytes || n[4] >= i || (i > 0 && n[4] < 0) {
				return x, g1nFault("G1_NATIVE_TREE")
			}
		}
		if G1NativeNodesDigest(s) != s.TreeDigest {
			return x, g1nFault("G1_NATIVE_TREE")
		}
	}
	return x, nil
}

// G1NativeNodesDigest binds the compact node list of a source.
func G1NativeNodesDigest(s G1NativeSource) string {
	b, _ := json.Marshal(struct {
		T []string `json:"t"`
		N [][5]int `json:"n"`
	}{s.Types, s.Nodes})
	return digestHex(b)
}

// MarshalCSharpG1NativeRaw writes the document: one line per source.
func MarshalCSharpG1NativeRaw(x CSharpG1NativeRaw) ([]byte, error) {
	var out []byte
	head, err := json.Marshal(struct {
		Schema     string             `json:"schema"`
		Provenance G1NativeProvenance `json:"provenance"`
	}{x.Schema, x.Provenance})
	if err != nil {
		return nil, err
	}
	out = append(out, head[:len(head)-1]...)
	out = append(out, `,"sources":[`...)
	for i, s := range x.Sources {
		b, err := json.Marshal(s)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, '\n')
		out = append(out, b...)
	}
	out = append(out, "\n]}\n"...)
	return out, nil
}

// CSharpG1NativeSourceIDs lists, in order, the source versions the native comparison needs: the
// witness sources of every EXPECTED alternative and every source version of a P step the ledger links
// to such an alternative ("<case>#<step>").
func CSharpG1NativeSourceIDs(ledger CSharpG1Ledger, exp CSharpG1Expected) []string {
	links := map[string][]G1Link{}
	for _, a := range ledger.Alternatives {
		links[a.ID] = a.Links
	}
	set := map[string]bool{}
	for _, a := range exp.Alternatives {
		if a.Status != G1ExpExpected {
			continue
		}
		for _, w := range a.Witnesses {
			set[w.Source] = true
		}
		for _, l := range links[a.ID] {
			for _, step := range l.Steps {
				set[fmt.Sprintf("%s#%d", l.Case, step)] = true
			}
		}
	}
	return g1SortedKeys(set)
}
