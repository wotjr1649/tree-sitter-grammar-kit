package kit

import (
	"bytes"
	"encoding/base64"
	"encoding/json/jsontext"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
)

// refRowAlloc measures the bytes the Go runtime allocated while f ran. Other goroutines of the
// test binary are idle, so the largest of a few runs is a fair observation of f's own allocation.
func refRowAlloc(f func()) uint64 {
	var worst uint64
	for i := 0; i < 3; i++ {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		f()
		runtime.ReadMemStats(&after)
		worst = max(worst, after.TotalAlloc-before.TotalAlloc)
	}
	return worst
}

// refRowTokens counts the values a token walk visits, independently of the product scan.
func refRowTokens(data []byte) uint64 {
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	var values uint64
	for {
		kind, count := dec.StackIndex(dec.StackDepth())
		name := kind == '{' && count%2 == 0
		peek := dec.PeekKind()
		if !name && peek != '}' && peek != ']' && peek != 0 {
			values++
		}
		if _, err := dec.ReadToken(); err != nil {
			if err == io.EOF {
				return values
			}
			return values
		}
	}
}

// refRowTree is a root with n-1 one-byte leaves on one line; the source is n bytes long.
func refRowTree(n int, typ string) ([]TreeNode, []byte) {
	source := bytes.Repeat([]byte("a"), n)
	field := "f"
	nodes := []TreeNode{{Parent: -1, Type: "program", Named: true, EndByte: uint32(n), EndPoint: Point{Column: uint32(n)}}}
	for i := 0; i+1 < n; i++ {
		nodes = append(nodes, TreeNode{Parent: 0, Type: typ, Field: &field, Named: true, StartByte: uint32(i), EndByte: uint32(i + 1),
			StartPoint: Point{Column: uint32(i)}, EndPoint: Point{Column: uint32(i + 1)}})
	}
	return nodes, source
}

func refRowCaptures(count, nodes int, name string) []Capture {
	captures := make([]Capture, count)
	for i := range captures {
		node := 1 + i%(nodes-1)
		captures[i] = Capture{Match: uint32(i), Name: name, Node: int64(node), Type: "t", Named: true,
			StartByte: uint32(node - 1), EndByte: uint32(node), StartPoint: Point{Column: uint32(node - 1)}, EndPoint: Point{Column: uint32(node)}}
	}
	return captures
}

type refRowCase struct {
	name string
	data []byte
	// pass is true when the row is a well-formed reference response; error rows stop in the decode.
	pass   bool
	source []byte
}

func refRowCases(t *testing.T) []refRowCase {
	t.Helper()
	build := func(name string, n, captures int, typ, captureName, queryID string) refRowCase {
		nodes, source := refRowTree(n, typ)
		metadata := QueryWitnessMetadata{Patterns: 1, CaptureNames: []string{captureName}, Predicates: []QueryPredicate{}}
		return refRowCase{name: name, pass: true, source: source,
			data: ordinaryE2ESyntheticResponseWithExpected(t, queryID, "(program) @"+captureName, "tree_sitter_x", strings.Repeat("0", 64), source, nodes, metadata, refRowCaptures(captures, n, captureName))}
	}
	cases := []refRowCase{
		build("tiny", 4, 0, "t", "c", "q"),
		build("large-cst", 20000, 0, "t", "c", "q"),
		build("many-captures", 100, 12000, "t", "c", "q"),
		build("long-node-types", 40, 0, strings.Repeat("T", 50000), "c", "q"),
		build("long-capture-names", 40, 400, "t", strings.Repeat("C", 4000), "q"),
		build("long-query-id", 40, 0, "t", "c", strings.Repeat("Q", 1<<20)),
	}
	tiny := cases[0]
	swap := func(name, from, to string) refRowCase {
		if !bytes.Contains(tiny.data, []byte(from)) {
			t.Fatalf("tiny row has no %s", from)
		}
		return refRowCase{name: name, pass: true, source: tiny.source, data: bytes.Replace(tiny.data, []byte(from), []byte(to), 1)}
	}
	cases = append(cases,
		swap("many-capture-names", `"capture_names":["c"]`, `"capture_names":[`+strings.TrimSuffix(strings.Repeat(`"",`, 100000), ",")+`]`),
		swap("many-mapping-rows", `"match_mapping":[]`, `"match_mapping":[`+strings.TrimSuffix(strings.Repeat(`{"match":0,"runtime_id":0},`, 60000), ",")+`]`),
	)
	deep := strings.Repeat(`{"`+strings.Repeat("a", 30000)+`":`, 31) + `[1,}`
	cases = append(cases,
		refRowCase{name: "error-deep-long-names", data: []byte(deep)},
		refRowCase{name: "error-long-string-bad-escape", data: []byte(`{"schema":"` + strings.Repeat("x", 1<<20) + `\q"}`)},
		refRowCase{name: "error-duplicate-long-name", data: []byte(`{"` + strings.Repeat("d", 1<<19) + `":1,"` + strings.Repeat("d", 1<<19) + `":2}`)},
		refRowCase{name: "error-tiny-array", data: []byte(`{"schema":"x","query":{"structural":[` + strings.TrimSuffix(strings.Repeat(`{},`, 100000), ",") + `]}}`)},
		refRowCase{name: "error-wide-names", data: []byte(`{` + strings.Repeat(`"k`+strings.Repeat("n", 40)+`":0,`, 20000) + `"z":1}`)},
		refRowCase{name: "wrong-schema-long-string", data: []byte(`{"schema":"` + strings.Repeat("s", 1<<21) + `","status":"COMPLETED","code":"","complete":true}`)},
	)
	return cases
}

// The ordinary reference-row decode must account at least what the runtime allocates and what
// the decode visits, for real-shaped rows and for adversarial ones.
func TestOrdinaryReferenceRowAccounting(t *testing.T) {
	for _, c := range refRowCases(t) {
		t.Run(c.name, func(t *testing.T) {
			b := ordinaryTestBudget(t)
			query := QueryWitnessQuery{ID: "q", Source: "(program) @c"}
			obs := QueryReferenceObservation{Query: "q", SourceBytes: uint64(len(c.source)), SourceSHA256: digestHex(c.source)}
			var work, logical uint64
			alloc := refRowAlloc(func() {
				b = ordinaryTestBudget(t)
				decodeReferenceResponseRow(c.data, obs, strings.Repeat("0", 64), "tree_sitter_x", query, true, [][]byte{c.source}, b.r, true)
				work, logical = b.work, b.logical
			})
			if logical < alloc {
				t.Fatalf("%s: row of %d bytes accounted logical %d below the %d bytes allocated", c.name, len(c.data), logical, alloc)
			}
			if c.pass {
				// preflight, shape walk and unmarshal of the head, then shape walk and unmarshal of the row
				walks := 5 * refRowTokens(c.data)
				hashed := uint64(len(query.Source))
				if work < walks+hashed {
					t.Fatalf("%s: accounted work %d below %d visited values and %d hashed bytes", c.name, work, walks, hashed)
				}
			}
			t.Logf("%s: row=%d alloc=%d logical=%d (%.1fx) work=%d", c.name, len(c.data), alloc, logical, float64(logical)/float64(max(alloc, 1)), work)
		})
	}
}

// Process receipts carry the whole stdout as one base64 string; their name-path reservation still
// has to cover the decode.
func TestOrdinaryReceiptDecodeAccounting(t *testing.T) {
	row := bytes.Repeat([]byte("r"), 3<<20)
	stderr := bytes.Repeat([]byte("e"), 4096)
	good := ordinaryJSON(t, eligibilityReceipt{Schema: "tsgk-reference-process/r1", StdoutBase64: base64.StdEncoding.EncodeToString(row), StderrBase64: base64.StdEncoding.EncodeToString(stderr)})
	deep := []byte(strings.Repeat(`{"`+strings.Repeat("a", 30000)+`":`, 31) + `[1,}`)
	for name, data := range map[string][]byte{"receipt": good, "deep-names": deep, "long-bad-escape": []byte(`{"stdout_base64":"` + strings.Repeat("x", 1<<20) + `\q"}`)} {
		t.Run(name, func(t *testing.T) {
			var logical uint64
			alloc := refRowAlloc(func() {
				b := ordinaryTestBudget(t)
				var receipt eligibilityReceipt
				_ = b.decodeWitnessTypedPathBound("receipt", data, &receipt, true)
				logical = b.logical
			})
			if logical < alloc {
				t.Fatalf("%s: receipt of %d bytes accounted logical %d below the %d bytes allocated", name, len(data), logical, alloc)
			}
			t.Logf("%s: bytes=%d alloc=%d logical=%d", name, len(data), alloc, logical)
		})
	}
}

// Shared callers keep the original reservation; only the ordinary row path is cheaper.
func TestReferenceRowSharedDecodeUnchanged(t *testing.T) {
	nodes, source := refRowTree(200, "t")
	data := ordinaryE2ESyntheticResponseWithExpected(t, "q", "(program) @c", "tree_sitter_x", strings.Repeat("0", 64), source, nodes,
		QueryWitnessMetadata{Patterns: 1, CaptureNames: []string{"c"}, Predicates: []QueryPredicate{}}, refRowCaptures(20, 200, "c"))
	obs := QueryReferenceObservation{Query: "q", SourceBytes: uint64(len(source)), SourceSHA256: digestHex(source)}
	query := QueryWitnessQuery{ID: "q", Source: "(program) @c"}
	shared, ordinary := ordinaryTestBudget(t), ordinaryTestBudget(t)
	got, code, _, e := decodeReferenceResponseBound(data, obs, strings.Repeat("0", 64), "tree_sitter_x", query, true, [][]byte{source}, shared.r)
	want, wantCode, _, wantErr := decodeReferenceResponseRow(data, obs, strings.Repeat("0", 64), "tree_sitter_x", query, true, [][]byte{source}, ordinary.r, true)
	if e != nil || wantErr != nil || code != "" || wantCode != "" || got == nil || want == nil || len(got.Tree.Nodes) != len(want.Tree.Nodes) {
		t.Fatalf("shared %v %q, ordinary %v %q", e, code, wantErr, wantCode)
	}
	// Reference value of the unchanged shared reservation: two full decodes plus reserveReference.
	var head struct {
		Schema   string `json:"schema"`
		Status   string `json:"status"`
		Code     string `json:"code"`
		Complete bool   `json:"complete"`
	}
	legacy := ordinaryTestBudget(t)
	var decoded instrumentedReference
	if err := legacy.decodeWitnessTyped("reference-response", data, &head, false); err != nil {
		t.Fatal(err)
	}
	if err := legacy.decodeWitnessTyped("reference-response", data, &decoded, true); err != nil {
		t.Fatal(err)
	}
	if err := legacy.reserveReference(&decoded, [][]byte{source}); err != nil {
		t.Fatal(err)
	}
	if err := legacy.charge(uint64(len(source)), 0, "reference-points"); err != nil {
		t.Fatal(err)
	}
	if shared.logical != legacy.logical || shared.work != legacy.work {
		t.Fatalf("shared reference decode changed its ledger: logical %d/%d work %d/%d", shared.logical, legacy.logical, shared.work, legacy.work)
	}
	if ordinary.logical >= shared.logical {
		t.Fatalf("ordinary row did not get cheaper: %d >= %d", ordinary.logical, shared.logical)
	}
}

// The real TSQL and PostgreSQL rows of the selected SQL shards, with the product accounting.
func TestOrdinaryReferenceRowsAccountingSQL(t *testing.T) {
	if os.Getenv("TSGK_ORDINARY_SQL_E2E") != "1" {
		t.Skip("set TSGK_ORDINARY_SQL_E2E=1 for the SQL selected-shard rows")
	}
	for _, route := range []string{"tsql", "postgresql-sql"} {
		_, profileBytes, source, registration, _, _, _, _, _, _, _, expected, metadata := ordinarySQLOwnedInputs(t, route, nil)
		profile, err := ParseOracleProfile(profileBytes)
		if err != nil {
			t.Fatal(err)
		}
		versions, _, err := ApplyEdits(EncodingUTF8, source, profile.Native.Cases[0].Edits, 65536)
		if err != nil {
			t.Fatal(err)
		}
		queries := map[string]string{}
		for _, q := range profile.Queries {
			queries[q.ID] = q.Source
		}
		librarySHA := digestHex([]byte("synthetic reference library; never loaded"))
		var rows, bytesTotal, allocTotal, logicalTotal, workTotal uint64
		for _, ref := range ordinarySQLSyntheticReferences(t, profileBytes, source, registration, expected, metadata) {
			obs := QueryReferenceObservation{Query: ref.Query, SourceBytes: uint64(len(versions[ref.Step])), SourceSHA256: digestHex(versions[ref.Step])}
			query := QueryWitnessQuery{ID: ref.Query, Source: queries[ref.Query]}
			var logical, work uint64
			var code string
			alloc := refRowAlloc(func() {
				b := ordinaryTestBudget(t)
				var e *Error
				_, code, _, e = decodeReferenceResponseRow(ref.Response, obs, librarySHA, profile.Native.Symbol, query, true, [][]byte{versions[ref.Step]}, b.r, true)
				if e != nil {
					t.Fatal(e)
				}
				logical, work = b.logical, b.work
			})
			if code != "" || logical < alloc || work < 5*refRowTokens(ref.Response)+uint64(len(query.Source)) {
				t.Fatalf("%s %s/%d: code %q logical %d alloc %d work %d", route, ref.Query, ref.Step, code, logical, alloc, work)
			}
			rows, bytesTotal, allocTotal, logicalTotal, workTotal = rows+1, bytesTotal+uint64(len(ref.Response)), allocTotal+alloc, logicalTotal+logical, workTotal+work
		}
		t.Logf("%s: %d real rows, %d bytes, alloc %d, logical %d, work %d", route, rows, bytesTotal, allocTotal, logicalTotal, workTotal)
	}
}
