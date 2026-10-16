// Package largeaudit shares the registered large-API receipt contract between live capture and offline CI verification.
package largeaudit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"strconv"
	"strings"
)

type Receipt struct {
	Schema          string  `json:"schema"`
	ID              string  `json:"id"`
	SourceSHA       string  `json:"source_sha256"`
	SourceBytes     uint64  `json:"source_bytes"`
	Nodes           uint64  `json:"nodes"`
	Start           uint64  `json:"range_start"`
	End             uint64  `json:"range_end"`
	Depth           uint64  `json:"depth"`
	Fields          uint64  `json:"field_count"`
	Checks          uint64  `json:"checks"`
	ChildFields     uint64  `json:"child_fields"`
	FieldLookups    uint64  `json:"field_lookups"`
	PointChecks     uint64  `json:"point_checks"`
	Differences     uint64  `json:"differences"`
	Divergences     uint64  `json:"position_navigation_divergences"`
	FirstDivergence []int64 `json:"first_position_divergence"`
	First           []int64 `json:"first_difference"`
	ExpectedSHA     string  `json:"expected_sha256"`
	ObservedSHA     string  `json:"observed_sha256"`
	Complete        bool    `json:"complete"`
}

// This is the existing registered three-Fixture inventory, not a caller-selected
// replacement workload. Updating it requires an explicit contract change.
const FixtureInventorySHA = "b9febb20fc2f6ba1e5b8d6d4b71b30f391b9cb32bf71eb1c4a68a8bd6f2f89f3"

type Fixture struct {
	ID     string            `json:"id"`
	Header string            `json:"header"`
	Block  string            `json:"block"`
	Footer string            `json:"footer"`
	SHA256 string            `json:"sha256"`
	Repeat int               `json:"repeat"`
	Bytes  uint64            `json:"bytes"`
	Points []kit.NativePoint `json:"points"`
	Expect []json.RawMessage `json:"expect"`
}
type Inventory struct {
	Schema    string    `json:"schema"`
	Route     string    `json:"route"`
	Operation string    `json:"operation"`
	Note      string    `json:"note"`
	Fixtures  []Fixture `json:"fixtures"`
}

func LoadFixtures(data []byte) (Inventory, error) {
	var x Inventory
	if digest(data) != FixtureInventorySHA {
		return x, errors.New("registered fixture inventory identity mismatch")
	}
	if e := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); e != nil {
		return x, e
	}
	if x.Schema != "tsgk-native-large-fixtures/r1" || x.Route != "csharp" || len(x.Fixtures) != 3 {
		return x, errors.New("fixture inventory invalid")
	}
	seen := map[string]bool{}
	for _, f := range x.Fixtures {
		if seen[f.ID] {
			return x, errors.New("duplicate fixture id")
		}
		seen[f.ID] = true
	}
	return x, nil
}

func digest(data []byte) string { x := sha256.Sum256(data); return hex.EncodeToString(x[:]) }
func Decode(data []byte, req native.Request, start, end, expectedNodes uint64) (Receipt, error) {
	var r Receipt
	var object map[string]json.RawMessage
	if jsonv2.Unmarshal(data, &object) != nil {
		return r, errors.New("audit object invalid")
	}
	for _, key := range []string{"schema", "id", "source_sha256", "source_bytes", "nodes", "range_start", "range_end", "depth", "field_count", "checks", "child_fields", "field_lookups", "point_checks", "differences", "position_navigation_divergences", "first_position_divergence", "first_difference", "expected_sha256", "observed_sha256", "complete"} {
		value, ok := object[key]
		if !ok || strings.TrimSpace(string(value)) == "null" {
			return r, errors.New("audit field missing or null")
		}
	}
	if err := jsonv2.Unmarshal(data, &r, jsonv2.RejectUnknownMembers(true)); err != nil {
		return r, err
	}
	for _, s := range []string{r.SourceSHA, r.ExpectedSHA, r.ObservedSHA} {
		x, e := hex.DecodeString(s)
		if e != nil || len(x) != 32 || strings.ToLower(s) != s {
			return r, errors.New("audit digest invalid")
		}
	}
	if r.Schema != "tsgk-large-api-audit/r1" || r.ID != req.ID || r.SourceBytes != uint64(len(req.Source)) || r.SourceSHA != digest(req.Source) || !r.Complete || r.Nodes == 0 || r.Nodes > req.Limits.Nodes || r.Depth == 0 || r.Depth > req.Limits.Depth || r.Fields > 65535 || len(r.First) != 4 || len(r.FirstDivergence) != 4 {
		return r, errors.New("audit identity or completion invalid")
	}
	if (expectedNodes != 0 && r.Nodes != expectedNodes) || r.Start != start || r.End != min(end, r.Nodes) || r.End <= r.Start || r.ChildFields > r.Nodes-1 || (r.Start == 0 && r.End == r.Nodes && r.ChildFields != r.Nodes-1) || r.FieldLookups != 2*(r.End-r.Start)*r.Fields || r.PointChecks != 3*uint64(len(req.Points)+2) || r.Checks != 13*(r.End-r.Start)+r.ChildFields+r.FieldLookups+r.PointChecks {
		return r, errors.New("audit coverage counts invalid")
	}
	if (r.Differences == 0) != (r.ExpectedSHA == r.ObservedSHA) || r.Differences > r.Checks || r.Divergences > 4*r.Nodes {
		return r, errors.New("audit comparison invalid")
	}
	if r.Differences == 0 {
		for _, v := range r.First {
			if v != 0 {
				return r, errors.New("unexpected first difference")
			}
		}
	} else if !((r.First[1] >= 0 && uint64(r.First[1]) < 13+r.ChildFields) || (r.First[1] >= 1000002 && uint64(r.First[1]) <= 1000001+2*r.Fields) || (r.First[1] >= 2000000 && r.First[1] <= 2000002)) || r.First[2] == r.First[3] || r.First[0] < 0 || r.First[1] < 0 || (r.First[1] < 2000000 && (uint64(r.First[0]) < r.Start || uint64(r.First[0]) >= r.End)) || (r.First[1] >= 2000000 && (r.First[1] > 2000002 || uint64(r.First[0]) >= uint64(len(req.Points)+2))) {
		return r, errors.New("missing first difference")
	}
	if r.Divergences == 0 {
		for _, v := range r.FirstDivergence {
			if v != 0 {
				return r, errors.New("unexpected positional divergence")
			}
		}
	} else if r.FirstDivergence[0] < 0 || uint64(r.FirstDivergence[0]) < r.Start || uint64(r.FirstDivergence[0]) >= r.End || r.FirstDivergence[1] < 1 || r.FirstDivergence[1] > 4 || r.FirstDivergence[2] == r.FirstDivergence[3] {
		return r, errors.New("positional divergence coordinates invalid")
	}
	return r, nil
}

// SmallSources returns the existing four registered controls.
func SmallSources() []string {
	return []string{"    class C { int M(int a) { return a + 1; } }\n", "class C { int M(int a) { return a + ; } }\n", "class C { void M() { F(1; } }\n", "class C { int M() { return 1 } }\n"}
}

// Source reproduces the registered numbered blocks within their existing size cap.
func Source(f Fixture) ([]byte, error) {
	if f.Bytes == 0 || f.Bytes > 33554432 || f.Repeat < 0 || f.Repeat > 10000000 || f.Block == "" {
		return nil, errors.New("registered fixture size invalid")
	}
	var b bytes.Buffer
	b.Grow(int(f.Bytes))
	write := func(s string) bool {
		if uint64(b.Len())+uint64(len(s)) > f.Bytes {
			return false
		}
		b.WriteString(s)
		return true
	}
	if !write(f.Header) {
		return nil, errors.New("registered fixture size exceeded")
	}
	for n := 0; n < f.Repeat; n++ {
		if !write(strings.ReplaceAll(f.Block, "{N}", strconv.Itoa(n))) {
			return nil, errors.New("registered fixture size exceeded")
		}
	}
	if !write(f.Footer) || uint64(b.Len()) != f.Bytes || digest(b.Bytes()) != f.SHA256 {
		return nil, errors.New("registered fixture identity mismatch")
	}
	return b.Bytes(), nil
}
