package kit

import (
	"bytes"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

// unsafeBody is the verified body a share entry was keyed by.
func unsafeBody(k ordinaryDecodedKey) []byte { return unsafe.Slice(k.body, k.size) }

// expDecodeDocs are real-shaped and adversarial expected assets; kind selects the decode target.
type expDecodeDoc struct {
	name string
	kind string // tree, captures, metadata, facts
	data []byte
	ok   bool // a well-formed asset; error inputs stop inside the decode
}

func expDecodeTarget(kind string) any {
	switch kind {
	case "tree":
		return new(ordinaryExpectedTree)
	case "captures":
		return new(ordinaryExpectedCaptures)
	case "metadata":
		return new(QueryWitnessMetadata)
	}
	return new(ordinaryExpectedFact)
}

func expDecodeDocs(t *testing.T) []expDecodeDoc {
	t.Helper()
	pin := OrdinaryPin{Path: "source-witness/x.utf8", Bytes: 1, SHA256: strings.Repeat("a", 64)}
	tree := func(n int, typ string) []byte {
		nodes, _ := refRowTree(n, typ)
		return ordinaryJSON(t, ordinaryExpectedTree{Schema: ordinaryExpectedTreeSchema, Case: "c", TreeRole: "incremental", Source: pin, Tree: nodes})
	}
	captures := func(count int, name string) []byte {
		return ordinaryJSON(t, ordinaryExpectedCaptures{Schema: ordinaryExpectedCaptureSchema, Case: "c", Query: "q", TreeRole: "incremental", Source: pin, Matches: uint64(count), Ordered: refRowCaptures(count, 1000, name)})
	}
	metadata := func(names []string) []byte {
		return ordinaryJSON(t, QueryWitnessMetadata{Patterns: 1, CaptureNames: names, Predicates: []QueryPredicate{}})
	}
	items := make([]DeclarationItem, 5000)
	for i := range items {
		items[i] = DeclarationItem{Fact: "create_object", NodeType: "create_table", Name: &ByteSpan{StartByte: 0, EndByte: 1}, Status: AssessPass}
	}
	facts := ordinaryJSON(t, ordinaryExpectedFact{Schema: ordinaryExpectedFactSchema, Case: "c", Query: "q", TreeRole: "incremental", Source: pin, Status: AssessPass,
		Declarations: &ordinaryExpectedDeclarations{Items: items, Assessment: AssessPass}})
	emptyNames := strings.TrimSuffix(strings.Repeat(`"",`, 100000), ",")
	return []expDecodeDoc{
		{"tree-tiny", "tree", tree(4, "t"), true},
		{"tree-large", "tree", tree(20000, "t"), true},
		{"tree-long-types", "tree", tree(40, strings.Repeat("T", 50000)), true},
		{"captures-many", "captures", captures(12000, "c"), true},
		{"captures-long-names", "captures", captures(400, strings.Repeat("C", 4000)), true},
		{"metadata-small", "metadata", metadata([]string{"a", "b"}), true},
		{"metadata-wide", "metadata", metadata(slices.Repeat([]string{strings.Repeat("n", 40)}, 20000)), true},
		{"metadata-empty-names", "metadata", []byte(`{"patterns":1,"capture_names":[` + emptyNames + `],"predicates":[]}`), true},
		{"facts-many-items", "facts", facts, true},
		{"error-deep-long-names", "tree", []byte(strings.Repeat(`{"`+strings.Repeat("a", 30000)+`":`, 31) + `[1,}`), false},
		{"error-long-string-bad-escape", "tree", []byte(`{"schema":"` + strings.Repeat("x", 1<<20) + `\q"}`), false},
		{"error-duplicate-long-name", "tree", []byte(`{"` + strings.Repeat("d", 1<<19) + `":1,"` + strings.Repeat("d", 1<<19) + `":2}`), false},
		{"error-tiny-array", "captures", []byte(`{"ordered_captures":[` + strings.TrimSuffix(strings.Repeat(`{},`, 100000), ",") + `]}`), false},
		{"error-unknown-wide-names", "metadata", []byte(`{` + strings.Repeat(`"k`+strings.Repeat("n", 40)+`":0,`, 20000) + `"z":1}`), false},
	}
}

// The ordinary expected-asset decode must account at least what the runtime allocates and the
// values it visits, for real-shaped assets and for adversarial ones.
func TestOrdinaryExpectedDecodeAccounting(t *testing.T) {
	for _, d := range expDecodeDocs(t) {
		t.Run(d.name, func(t *testing.T) {
			var work, logical uint64
			var e *Error
			alloc := refRowAlloc(func() {
				b := ordinaryTestBudget(t)
				_, e = b.decodeOrdinaryExpected("doc", d.data, expDecodeTarget(d.kind))
				work, logical = b.work, b.logical
			})
			if d.ok && e != nil {
				t.Fatalf("well-formed asset rejected: %v", e)
			}
			if !d.ok && (e == nil || e.Kind != KindInvalidInput) {
				t.Fatalf("error input was not rejected as invalid input: %v", e)
			}
			if logical < alloc {
				t.Fatalf("%s: %d bytes accounted logical %d below the %d bytes allocated", d.name, len(d.data), logical, alloc)
			}
			if d.ok {
				// preflight, shape walk and unmarshal
				if walks := 3 * refRowTokens(d.data); work < walks {
					t.Fatalf("%s: accounted work %d below %d visited values", d.name, work, walks)
				}
			}
			t.Logf("%s: bytes=%d alloc=%d logical=%d (%.1fx) work=%d", d.name, len(d.data), alloc, logical, float64(logical)/float64(max(alloc, 1)), work)
		})
	}
}

// The share's own retention is priced: the heap a kept decode holds is at most what the cache entry is charged.
func TestOrdinaryExpectedCacheRetentionPriced(t *testing.T) {
	for _, d := range expDecodeDocs(t) {
		if !d.ok || d.kind == "facts" {
			continue
		}
		t.Run(d.name, func(t *testing.T) {
			var charged uint64
			prev := ordinaryChargeHook
			ordinaryChargeHook = func(_ *eligibilityBudget, _, logical uint64, at string, _, _ uint64) {
				if at == "ordinary-expected-cache" {
					charged += logical
				}
			}
			t.Cleanup(func() { ordinaryChargeHook = prev })
			var worst uint64
			// The first run only warms the per-type decode shape caches, which no entry owns.
			for i := 0; i < 4; i++ {
				charged = 0
				b := ordinaryTestBudget(t)
				b.decoded = newOrdinaryDecodedAssets()
				var before, after runtime.MemStats
				runtime.GC()
				runtime.GC() // the second collection also drops the sync.Pool victims of the decoder
				runtime.ReadMemStats(&before)
				var e *Error
				switch d.kind {
				case "tree":
					_, _, e = ordinaryExpectedDecode[ordinaryExpectedTree](b, "doc", d.data)
				case "captures":
					_, _, e = ordinaryExpectedDecode[ordinaryExpectedCaptures](b, "doc", d.data)
				default:
					_, _, e = ordinaryExpectedDecode[QueryWitnessMetadata](b, "doc", d.data)
				}
				if e != nil {
					t.Fatal(e)
				}
				runtime.GC()
				runtime.GC()
				runtime.ReadMemStats(&after)
				runtime.KeepAlive(b.decoded)
				if i > 0 && after.HeapAlloc > before.HeapAlloc {
					worst = max(worst, after.HeapAlloc-before.HeapAlloc)
				}
			}
			if charged == 0 || charged < worst {
				t.Fatalf("%s: cache entry charged %d below the %d bytes it retains", d.name, charged, worst)
			}
			t.Logf("%s: retained=%d charged=%d (%.1fx)", d.name, worst, charged, float64(charged)/float64(max(worst, 1)))
		})
	}
}

// expSeq is one tuple of a sequence: the tuple's inputs and its reference row.
type expSeq struct {
	id            ordinaryTupleIdentity
	actual        nativewire.Tree
	source        []byte
	meta, tree    []byte
	captures      []byte
	ref           *instrumentedReference
	skipReference bool
}

// expJudge mirrors the coordinator's per-tuple steps: judgment, then the reference comparison.
func expJudge(t *testing.T, b *eligibilityBudget, c expSeq) ordinaryTupleJudgment {
	t.Helper()
	j := &ordinaryJudgmentBuilder{}
	judgment, e := adjudicateOrdinaryTuple(b, c.id, c.actual, c.source, c.meta, c.tree, c.captures, nil, nil)
	ordinaryMergeJudgment(j, judgment)
	if e == nil && !c.skipReference {
		judgment, e = b.ordinaryReferenceExpected(c.id, c.ref, c.meta, c.tree, c.captures)
		ordinaryMergeJudgment(j, judgment)
	}
	if e != nil {
		if e.Kind != KindInvalidInput {
			t.Fatalf("tuple %s/%s stopped: %v", c.id.Case, c.id.Query, e)
		}
		j.fail(e.Code, "asset")
	}
	return j.value()
}

// expTuples builds tuples over one shared tree and metadata: two queries on the same step, then
// controls. The bodies of one asset are one slice, as the asset resolver returns them.
func expTuples(t *testing.T) []expSeq {
	t.Helper()
	id, actual, source, metadata, tree, captures := ordinaryAdjudicatorFixture(t)
	metaBody, treeBody := ordinaryJSON(t, metadata), ordinaryJSON(t, tree)
	build := func(query string, treeBody []byte) expSeq {
		a := actual
		a.Queries = slices.Clone(actual.Queries)
		a.Queries[0].ID = query
		cap := captures
		cap.Query = query
		ref := &instrumentedReference{}
		ref.Query.Patterns, ref.Query.CaptureNames, ref.Query.Predicates = 1, []string{"x"}, []QueryPredicate{}
		ref.Query.Structural, ref.Query.Matches, ref.Tree.Nodes = cap.Ordered, cap.Matches, tree.Tree
		tid := id
		tid.Query = query
		return expSeq{id: tid, actual: a, source: source, meta: metaBody, tree: treeBody, captures: ordinaryJSON(t, cap), ref: ref}
	}
	tuples := []expSeq{build("q1", treeBody), build("q2", treeBody), build("q3", treeBody)}
	// A tuple bound to another case reads the same tree body: only it must fail the binding.
	other := build("q4", treeBody)
	other.id.Case = "case-other"
	// One tuple with its own tampered tree body.
	tampered := tree
	tampered.Tree = slices.Clone(tree.Tree)
	tampered.Tree[0].Type = "tampered" // not a captured node, so the tree comparison reports first
	bad := build("q5", ordinaryJSON(t, tampered))
	// One tuple whose captures fail strict decode.
	broken := build("q6", treeBody)
	broken.captures = bytes.Replace(broken.captures, []byte(`"matches"`), []byte(`"unknown_member"`), 1)
	// One tuple whose tree fails strict decode: the shared decode error is replayed.
	badTree := build("q7", bytes.Replace(treeBody, []byte(`"schema"`), []byte(`"schema_x"`), 1))
	return []expSeq{tuples[0], other, tuples[1], bad, broken, tuples[2], badTree, build("q8", treeBody)}
}

func expRun(t *testing.T, share bool, tuples []expSeq) ([]ordinaryTupleJudgment, *eligibilityBudget, map[string]int, map[string]int) {
	t.Helper()
	b := ordinaryTestBudget(t)
	if share {
		b.decoded = newOrdinaryDecodedAssets()
	}
	decodes := map[string]int{}
	uses, charges := 0, map[string]int{}
	prev, prevCharge := ordinaryDecodeHook, ordinaryChargeHook
	ordinaryDecodeHook = func(doc string, shared bool) {
		uses++
		if !shared {
			decodes[doc]++
		}
	}
	ordinaryChargeHook = func(_ *eligibilityBudget, _, _ uint64, at string, _, _ uint64) {
		charges[at]++
	}
	t.Cleanup(func() { ordinaryDecodeHook, ordinaryChargeHook = prev, prevCharge })
	var got []ordinaryTupleJudgment
	for _, c := range tuples {
		got = append(got, expJudge(t, b, c))
	}
	ordinaryDecodeHook, ordinaryChargeHook = prev, prevCharge
	// Every use of the share pays its lookup; a pass without a share pays none.
	if lookups := charges["ordinary-expected-cache-lookup"]; share && lookups != uses || !share && lookups != 0 {
		t.Fatalf("share=%t: %d lookups charged for %d uses", share, lookups, uses)
	}
	return got, b, decodes, charges
}

// Sharing changes the ledger, never the judgments: states, first FAIL code, findings and their
// order equal those of a pass without a share, and a tampered tuple fails alone.
func TestOrdinaryExpectedShareKeepsJudgments(t *testing.T) {
	tuples := expTuples(t)
	plain, plainBudget, plainDecodes, plainCharges := expRun(t, false, tuples)
	shared, sharedBudget, sharedDecodes, sharedCharges := expRun(t, true, tuples)
	if !reflect.DeepEqual(plain, shared) {
		t.Fatalf("sharing changed the judgments:\nplain  %+v\nshared %+v", plain, shared)
	}
	want := []struct{ state, code string }{
		{ordinaryTupleVerified, ""},
		{ordinaryTupleFailed, "ORDINARY_EXPECTED_TREE_BINDING"},
		{ordinaryTupleVerified, ""},
		{ordinaryTupleFailed, "ORDINARY_EXPECTED_TREE_MISMATCH"},
		{ordinaryTupleFailed, "JSON_UNKNOWN_FIELD"},
		{ordinaryTupleVerified, ""},
		{ordinaryTupleFailed, "JSON_UNKNOWN_FIELD"},
		{ordinaryTupleVerified, ""},
	}
	for i, w := range want {
		if shared[i].State != w.state || shared[i].Code != w.code {
			t.Errorf("tuple %d: %s %s, want %s %s (%+v)", i, shared[i].State, shared[i].Code, w.state, w.code, shared[i].Findings)
		}
	}
	// Eight tuples use 3 distinct tree bodies plus one broken tree; each (type, body) decodes once.
	if sharedDecodes["ordinary-expected-tree"]+sharedDecodes["ordinary-reference-expected-tree"] != 3 ||
		sharedDecodes["ordinary-expected-metadata"]+sharedDecodes["ordinary-reference-metadata"] != 1 {
		t.Fatalf("shared decodes %v", sharedDecodes)
	}
	if plainDecodes["ordinary-expected-tree"] < 7 || plainDecodes["ordinary-reference-expected-tree"] < 5 {
		t.Fatalf("plain decodes %v", plainDecodes)
	}
	// Six tuples reach the structure check, of two distinct (tree, source) pairs: it runs twice and the
	// other uses pay only the lookup. Without a share every tuple runs it.
	if sharedCharges["ordinary-expected-tree-check"] != 2 || sharedCharges["ordinary-expected-tree-check-lookup"] != 6 ||
		plainCharges["ordinary-expected-tree-check"] != 6 || plainCharges["ordinary-expected-tree-check-lookup"] != 0 {
		t.Fatalf("tree check charges: shared %v plain %v", sharedCharges, plainCharges)
	}
	if sharedBudget.logical >= plainBudget.logical || sharedBudget.work >= plainBudget.work {
		t.Fatalf("sharing did not reduce the ledger: logical %d/%d work %d/%d", sharedBudget.logical, plainBudget.logical, sharedBudget.work, plainBudget.work)
	}
}

// The shared value is never written: after every tuple of the sequence each entry still equals a fresh decode.
func TestOrdinaryExpectedShareValuesStayUnchanged(t *testing.T) {
	tuples := expTuples(t)
	_, b, _, _ := expRun(t, true, tuples)
	checked := 0
	for key, entry := range b.decoded.entries {
		if entry.err != nil {
			continue
		}
		fresh := reflect.New(key.typ).Interface()
		if _, e := ordinaryTestBudget(t).decodeOrdinaryExpected("fresh", unsafeBody(key), fresh); e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(entry.value, fresh) {
			t.Fatalf("shared %v changed after the sequence", key.typ)
		}
		checked++
	}
	if checked < 4 {
		t.Fatalf("only %d shared values checked", checked)
	}
}

// With a share the whole sequence stays within its own accounting: retained values and every
// decode are priced at least at the runtime allocation.
func TestOrdinaryExpectedShareAccountingCoversAllocation(t *testing.T) {
	tuples := expTuples(t)
	var logical uint64
	alloc := refRowAlloc(func() {
		_, b, _, _ := expRun(t, true, tuples)
		logical = b.logical
	})
	if logical < alloc {
		t.Fatalf("shared sequence accounted logical %d below the %d bytes allocated", logical, alloc)
	}
}

// One asset ID is verified against one pin: a later use that claims another pin, or a different
// length, is rejected instead of being served the verified body.
func TestOrdinaryAssetResolverRejectsOtherPinForSharedAsset(t *testing.T) {
	f := newOrdinaryReaderFixture(t)
	if err := f.root.Mkdir("expected", 0700); err != nil {
		t.Fatal(err)
	}
	put := func(path, role string, value []byte) OrdinaryPin {
		t.Helper()
		file, err := f.root.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(value); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		pin := OrdinaryPin{Path: path, Bytes: uint64(len(value)), SHA256: digestHex(value)}
		owned := eligibilityMember{NodeID: "node-" + path, Member: ReplayMember{Path: path, Role: role, Bytes: pin.Bytes, SHA256: pin.SHA256}}
		f.s.owned[path], f.s.owners[path] = owned, owned.NodeID
		f.s.snapshot.files[path] = value
		f.s.snapshot.verifiedMembers[path] = owned.Member
		f.s.snapshot.observed[path], err = f.root.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return pin
	}
	pinA := put("expected/a.json", "expected-tree", []byte(`{"tree":"a"}`))
	pinB := put("expected/b.json", "expected-tree", []byte(`{"tree":"b"}`))
	basis := put("expected/basis.json", "authoring-basis", []byte("reviewed source"))
	review := put("expected/review.json", "independent-review", []byte("review record"))
	asset := func(pin OrdinaryPin) OrdinaryAsset {
		return OrdinaryAsset{ID: "tree", Role: "expected-tree", State: "PINNED", Pin: &pin, AuthoringBasis: &basis, IndependentReview: &review}
	}
	assets := map[string]OrdinaryAsset{"tree": asset(pinA)}
	resolver := ordinaryAssetResolver{session: f.s, assets: assets, resolved: map[string][]byte{}}
	first, err := resolver.resolve("tree", "expected-tree")
	if err != nil {
		t.Fatal(err)
	}
	again, err := resolver.resolve("tree", "expected-tree")
	if err != nil || len(again) == 0 || &again[0] != &first[0] {
		t.Fatalf("an unchanged pin must reuse the verified body: %v", err)
	}
	// A second claim on the same asset ID with a different pin, even one that is itself a valid member.
	assets["tree"] = asset(pinB)
	if _, err := resolver.resolve("tree", "expected-tree"); err == nil || asError(err).Code != "ORDINARY_ASSET_PIN" {
		t.Fatalf("a different pin for a shared asset was served the verified body: %v", err)
	}
	// The same pin with another length is not the verified body either.
	short := pinA
	short.Bytes--
	assets["tree"] = asset(short)
	if _, err := resolver.resolve("tree", "expected-tree"); err == nil || asError(err).Code != "ORDINARY_ASSET_PIN" {
		t.Fatalf("a pin of another length was served the verified body: %v", err)
	}
}

// Different bodies never share a decoded value, even with the same type and length.
func TestOrdinaryExpectedShareKeysByVerifiedBody(t *testing.T) {
	b := ordinaryTestBudget(t)
	b.decoded = newOrdinaryDecodedAssets()
	one := ordinaryJSON(t, QueryWitnessMetadata{Patterns: 1, CaptureNames: []string{"a"}, Predicates: []QueryPredicate{}})
	two := ordinaryJSON(t, QueryWitnessMetadata{Patterns: 1, CaptureNames: []string{"b"}, Predicates: []QueryPredicate{}})
	if len(one) != len(two) {
		t.Fatal("bodies must have the same length")
	}
	first, _, e := ordinaryExpectedDecode[QueryWitnessMetadata](b, "doc", one)
	if e != nil {
		t.Fatal(e)
	}
	second, _, e := ordinaryExpectedDecode[QueryWitnessMetadata](b, "doc", two)
	if e != nil || second.CaptureNames[0] != "b" || first.CaptureNames[0] != "a" {
		t.Fatalf("a body of the same type and length reused another asset's value: %v %+v", e, second)
	}
	again, _, e := ordinaryExpectedDecode[QueryWitnessMetadata](b, "doc", one)
	if e != nil || again != first {
		t.Fatalf("the same body must reuse its decode: %v", e)
	}
}

// The real TSQL expected assets of the selected shard (metadata, trees, captures, facts), with the
// product accounting: every decode is priced at least at its allocation and visited values.
func TestOrdinaryExpectedDecodeAccountingSQL(t *testing.T) {
	if os.Getenv("TSGK_ORDINARY_SQL_E2E") != "1" {
		t.Skip("set TSGK_ORDINARY_SQL_E2E=1 for the selected TSQL expected assets")
	}
	_, profileBytes, source, _, _, _, _, _, _, _, _, _, _ := ordinarySQLOwnedInputs(t, "tsql", nil)
	_, bodies, _, _ := ordinaryTSQLAssets(t, "alt-tsql-B01-r001", source, profileBytes, nil)
	type total struct{ n, bytes, alloc, logical, work uint64 }
	totals := map[string]*total{}
	for path, body := range bodies {
		name := path[strings.LastIndex(path, "/")+1:]
		if !strings.HasSuffix(name, ".json") { // authoring basis and review records are text
			continue
		}
		kind := map[bool]string{true: "metadata"}[strings.HasPrefix(name, "meta-")]
		for prefix, k := range map[string]string{"tsql-tree-": "tree", "tsql-captures-": "captures", "tsql-facts-": "facts"} {
			if strings.HasPrefix(name, prefix) {
				kind = k
			}
		}
		if kind == "" {
			continue
		}
		var logical, work uint64
		alloc := refRowAlloc(func() {
			b := ordinaryTestBudget(t)
			if _, e := b.decodeOrdinaryExpected(name, body, expDecodeTarget(kind)); e != nil {
				t.Fatalf("%s: %v", name, e)
			}
			logical, work = b.logical, b.work
		})
		if logical < alloc || work < 3*refRowTokens(body) {
			t.Fatalf("%s: logical %d alloc %d work %d", name, logical, alloc, work)
		}
		sum := totals[kind]
		if sum == nil {
			sum = &total{}
			totals[kind] = sum
		}
		sum.n, sum.bytes, sum.alloc, sum.logical, sum.work = sum.n+1, sum.bytes+uint64(len(body)), sum.alloc+alloc, sum.logical+logical, sum.work+work
	}
	for kind, sum := range totals {
		t.Logf("%s: %d assets, %d bytes, alloc %d, logical %d (%.1fx), work %d", kind, sum.n, sum.bytes, sum.alloc, sum.logical, float64(sum.logical)/float64(max(sum.alloc, 1)), sum.work)
	}
	if len(totals) != 4 {
		t.Fatalf("expected tree, captures, facts and metadata assets, got %d kinds", len(totals))
	}
}
