package kit

import (
	"cmp"
	"fmt"
	"math/rand"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// #427: the TSQL dynamic SQL charge is derived from the capture counts instead of 128n². These tests drive
// deriveTSQL with adversarial capture lists, count the steps it really takes (factStepHook), measure the
// cumulative allocation, and require work >= 64 x steps and logical >= allocation for every list.

type tsqlFactCase struct {
	caps []Capture
	src  []byte
}

// tsqlFactSource is a source whose bytes the captures point into; DATA_SOURCE sits at 8..19.
func tsqlFactSource(size int) []byte {
	src := make([]byte, size)
	for i := range src {
		src[i] = " ;EXEC  @x=1 "[i%13]
	}
	copy(src[8:], "DATA_SOURCE")
	return src
}

type tsqlFactBuilder struct {
	caps  []Capture
	match uint32
	node  int64
}

func (b *tsqlFactBuilder) next() int64 { b.node++; return b.node }

func (b *tsqlFactBuilder) add(match uint32, name string, node int64, typ string, named bool, start, end uint32) {
	b.caps = append(b.caps, Capture{Match: match, Name: name, Node: node, Type: typ, Named: named, StartByte: start, EndByte: end})
}

func (b *tsqlFactBuilder) newMatch() uint32 { b.match++; return b.match }

// tsqlFactDuplicateExec: E exec captures on one node whose children list has K entries, so every exec
// walks all K children (the exec x children product).
func tsqlFactDuplicateExec(execs, kids int) tsqlFactCase {
	var b tsqlFactBuilder
	m := b.newMatch()
	parent := b.next()
	b.add(m, "exec.parent", parent, "execute_statement", true, 0, 4)
	for i := 0; i < kids; i++ {
		b.add(m, "exec.child", b.next(), "identifier", true, 1, 2)
	}
	for i := 0; i < execs; i++ {
		b.add(b.newMatch(), "exec", parent, "execute_statement", true, 0, 4)
	}
	return tsqlFactCase{b.caps, tsqlFactSource(64)}
}

// tsqlFactKnownMisses: E distinct EXEC ... AT DATA_SOURCE statements (each a known miss, so each runs the
// end-of-statement search over the execs and the S semi captures) and W errwith captures that never match.
func tsqlFactKnownMisses(execs, semis, withErrors int) tsqlFactCase {
	var b tsqlFactBuilder
	for i := 0; i < execs; i++ {
		m := b.newMatch()
		x := b.next()
		start := uint32(30 + 2*i)
		b.add(m, "exec.parent", x, "execute_statement", true, start, start+1)
		b.add(m, "exec", x, "execute_statement", true, start, start+1)
		b.add(m, "exec.child", b.next(), "(", false, start, start+1)
		b.add(m, "exec.child", b.next(), "keyword_at", true, start, start+1)
		b.add(m, "exec.child", b.next(), "identifier", true, 8, 19)
	}
	for i := 0; i < semis; i++ {
		b.add(b.newMatch(), "semi", b.next(), ";", false, 1, 2)
	}
	for i := 0; i < withErrors; i++ {
		b.add(b.newMatch(), "errwith", b.next(), "ERROR", true, 0, 3)
	}
	return tsqlFactCase{b.caps, tsqlFactSource(2*execs + 64)}
}

// tsqlFactWide: M matches that each hold every parent and child name, so each of the nine pairs calls
// scans all captures twice and sorts M pairs.
func tsqlFactWide(matches int) tsqlFactCase {
	var b tsqlFactBuilder
	for i := 0; i < matches; i++ {
		m := b.newMatch()
		for _, pair := range tsqlFactPairs {
			b.add(m, pair[0], b.next(), "node", true, 1, 3)
			b.add(m, pair[1], b.next(), "node", true, 2, 4)
		}
	}
	return tsqlFactCase{b.caps, tsqlFactSource(64)}
}

// tsqlFactRandom draws names, nodes and spans at random over the full name set; a match has up to five captures.
func tsqlFactRandom(seed int64, captures int) tsqlFactCase {
	rng := rand.New(rand.NewSource(seed))
	names := []string{"exec", "semi", "errwith", "batch.child", "batch.program"}
	for _, pair := range tsqlFactPairs {
		names = append(names, pair[0], pair[1])
	}
	types := []string{"(", "keyword_at", "identifier", "exec_argument", "go_statement", "batch", "literal", "field", "binary_expression"}
	var b tsqlFactBuilder
	src := tsqlFactSource(256)
	for len(b.caps) < captures {
		m := b.newMatch()
		for i, n := 0, 1+rng.Intn(5); i < n && len(b.caps) < captures; i++ {
			start := uint32(rng.Intn(200))
			b.add(m, names[rng.Intn(len(names))], int64(rng.Intn(captures/2+1)), types[rng.Intn(len(types))], rng.Intn(2) == 0, start, start+uint32(rng.Intn(40)))
		}
	}
	return tsqlFactCase{b.caps, src}
}

// tsqlFactCounts reads the counts the way the reservation does.
func (c tsqlFactCase) counts() tsqlFactCounts {
	var counts tsqlFactCounts
	for _, capture := range c.caps {
		counts.add(capture.Name)
	}
	return counts
}

type tsqlFactRun struct {
	facts                                      DynamicSQLFacts
	steps, pairCalls, decoded, missSort, alloc uint64
}

// reserve records the actual ledger before derivation; unmetered permits adversarial lists beyond pass capacity.
func (c tsqlFactCase) reserve(t *testing.T) (work, logical, missSort uint64) {
	t.Helper()
	b := semanticTestBudget(t)
	b.unmetered = true
	previous := ordinaryChargeHook
	ordinaryChargeHook = func(_ *eligibilityBudget, w, l uint64, at string, _, _ uint64) {
		work += w
		logical += l
		if at == "ordinary-base-fact-known-miss-sort" {
			missSort += w
		}
	}
	defer func() { ordinaryChargeHook = previous }()
	if e := b.ordinaryBaseFactReservation(FactQuery{Facts: FactDynamicSQL}, FactPackRoute{Route: "tsql"}, ordinaryFactBudgetQuery(c.caps), c.src); e != nil {
		t.Fatal(e)
	}
	return work, logical, missSort
}

// derive measures the adapter, including decoded bytes and final-sort comparisons.
func (c tsqlFactCase) derive(t *testing.T, enc string) (r tsqlFactRun) {
	t.Helper()
	previous := factStepHook
	factStepHook = func(kind string, n uint64) {
		switch kind {
		case "pair-steps":
			r.pairCalls++
			r.steps += n
		case "index-steps", "tsql-steps":
			r.steps += n
		case "decoded-bytes":
			r.decoded += n
		case "known-miss-sort-steps":
			r.missSort += n
		}
	}
	defer func() { factStepHook = previous }()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	facts, err := DeriveDynamicSQLChecked("tsql", enc, c.caps, c.src)
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	r.facts, r.alloc = facts, after.TotalAlloc-before.TotalAlloc
	return r
}

func TestTSQLFactBoundCoversAdversarialCaptureLists(t *testing.T) {
	cases := map[string]tsqlFactCase{
		"duplicate exec 40x40":   tsqlFactDuplicateExec(40, 40),
		"duplicate exec 200x200": tsqlFactDuplicateExec(200, 200),
		"duplicate exec 400x10":  tsqlFactDuplicateExec(400, 10),
		"known misses 60/60/60":  tsqlFactKnownMisses(60, 60, 60),
		"known misses 150/150/0": tsqlFactKnownMisses(150, 150, 0),
		"known misses 5/400/400": tsqlFactKnownMisses(5, 400, 400),
		"wide 10":                tsqlFactWide(10),
		"wide 120":               tsqlFactWide(120),
		"empty":                  {nil, tsqlFactSource(8)},
	}
	for seed := int64(1); seed <= 24; seed++ {
		cases[fmt.Sprintf("random seed %d", seed)] = tsqlFactRandom(seed, 30+int(seed)*40)
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			work, logical, _ := c.reserve(t)
			r := c.derive(t, EncodingUTF8)
			if r.pairCalls != uint64(len(tsqlFactPairs)) {
				t.Fatalf("deriveTSQL made %d pairs calls, tsqlFactPairs lists %d", r.pairCalls, len(tsqlFactPairs))
			}
			if work < tsqlStepWork*(r.steps+r.decoded) {
				t.Fatalf("work %d < %d x %d real steps and decoded bytes", work, tsqlStepWork, r.steps+r.decoded)
			}
			t.Logf("captures %d: steps %d, decoded %d B, work %d, allocated %d B of logical %d B", len(c.caps), r.steps, r.decoded, work, r.alloc, logical)
			if r.alloc > logical {
				t.Fatalf("allocated %d B > pre-reserved logical %d B", r.alloc, logical)
			}
		})
	}
}

// The charge follows the real counts: a long capture list without repeated EXEC captures costs far less than 128n².
func TestTSQLFactBoundIsNotQuadraticInCaptures(t *testing.T) {
	for _, captures := range []int{50, 150, 500, 2000} {
		c := tsqlFactRandom(7, captures)
		work, logical, e := tsqlFactBound(c.counts())
		if e != nil {
			t.Fatal(e)
		}
		_, _, sortWork := c.reserve(t)
		work += sortWork
		n := uint64(captures) + 1
		if old := 128 * n * n; captures >= 150 && (work >= old || logical >= old) {
			t.Fatalf("captures %d: work %d logical %d not below the previous 128n² = %d", captures, work, logical, old)
		}
	}
	// A real product still dominates when EXEC captures repeat: 200 execs x 200 children.
	c := tsqlFactDuplicateExec(200, 200)
	steps := c.derive(t, EncodingUTF8).steps
	work, _, e := tsqlFactBound(c.counts())
	if e != nil {
		t.Fatal(e)
	}
	if steps < 200*200 || work < tsqlStepWork*200*200 {
		t.Fatalf("the exec x children product was lost: steps %d work %d", steps, work)
	}
}

func tsqlRepeatedErrorParent(enc string) tsqlFactCase {
	source := []byte("sp_executesql " + strings.Repeat(" ", 32768-len("sp_executesql ")))
	identifierEnd := uint32(13)
	if enc == EncodingUTF16LE {
		var wide []byte
		for _, b := range source {
			wide = append(wide, b, 0)
		}
		source = wide
		identifierEnd *= 2
	} else if enc == EncodingCP949 {
		// Invalid lead bytes expand to U+FFFD; exercise a larger decoder allocation than UTF-8.
		for i := 13; i < len(source); i++ {
			source[i] = 0xff
		}
	}
	var b tsqlFactBuilder
	match := b.newMatch()
	b.add(match, "err", b.next(), "ERROR", true, 0, uint32(len(source)))
	for i := 0; i < 256; i++ {
		b.add(match, "err.first", b.next(), "identifier", true, 0, identifierEnd)
	}
	b.add(b.newMatch(), "batch.program", b.next(), "program", true, 0, uint32(len(source)))
	return tsqlFactCase{b.caps, source}
}

func TestTSQLRepeatedErrorParentAllocation(t *testing.T) {
	for _, enc := range []string{EncodingUTF8, EncodingUTF16LE, EncodingCP949} {
		t.Run(enc, func(t *testing.T) {
			c := tsqlRepeatedErrorParent(enc)
			work, logical, _ := c.reserve(t)
			r := c.derive(t, enc)
			if len(r.facts.KnownMisses) != 256 {
				t.Fatalf("repeated children lost verdicts: %d known misses", len(r.facts.KnownMisses))
			}
			for _, miss := range r.facts.KnownMisses {
				if miss != (KnownMiss{MissBatchFirstNoExe, 0, uint32(len(c.src))}) {
					t.Fatalf("known-miss payload changed: %+v", miss)
				}
			}
			t.Logf("work=%d logical=%d allocated=%d decoded=%d", work, logical, r.alloc, r.decoded)
			if r.alloc > logical {
				t.Fatalf("allocated %d B > pre-reserved logical %d B", r.alloc, logical)
			}
		})
	}
}

func TestTSQLRepeatedErrorParentDecodedWork(t *testing.T) {
	c := tsqlRepeatedErrorParent(EncodingUTF8)
	work, _, _ := c.reserve(t)
	r := c.derive(t, EncodingUTF8)
	t.Logf("work=%d steps=%d decoded=%d", work, r.steps, r.decoded)
	if used := 64 * (r.steps + r.decoded); used > work {
		t.Fatalf("real work %d > pre-reserved work %d", used, work)
	}
}

func TestTSQLKnownMissSortWithoutExec(t *testing.T) {
	const batches = 512
	var b tsqlFactBuilder
	var source []byte
	var expected []KnownMiss
	rng := rand.New(rand.NewSource(427))
	for _, i := range rng.Perm(batches) {
		start := uint32(i * len("sp_executesql\nGO\n"))
		end := start + uint32(len("sp_executesql"))
		m := b.newMatch()
		node := b.next()
		b.add(m, "err", node, "ERROR", true, start, end)
		b.add(m, "err.first", b.next(), "identifier", true, start, end)
		b.add(b.newMatch(), "batch.child", node-1, "go_statement", true, start, start)
		expected = append(expected, KnownMiss{MissBatchFirstNoExe, start, end})
		source = append(source, "sp_executesql\nGO\n"...)
	}
	b.add(b.newMatch(), "batch.program", b.next(), "program", true, 0, uint32(len(source)))
	c := tsqlFactCase{b.caps, source}
	work, logical, sortWork := c.reserve(t)
	r := c.derive(t, EncodingUTF8)
	slices.SortFunc(expected, func(a, b KnownMiss) int { return cmp.Compare(a.StartByte, b.StartByte) })
	if !slices.Equal(r.facts.KnownMisses, expected) || c.counts().execs != 0 {
		t.Fatal("multi-batch known misses changed")
	}
	t.Logf("misses=%d comparisons=%d sort work=%d total work=%d logical=%d alloc=%d", len(expected), r.missSort, sortWork, work, logical, r.alloc)
	if r.missSort == 0 || 64*r.missSort > sortWork {
		t.Fatalf("sort work %d does not cover %d comparisons", sortWork, r.missSort)
	}
	if 64*(r.steps+r.decoded) > work || r.alloc > logical {
		t.Fatal("measured work/allocation exceeds the pre-reserved ledger")
	}
}

func TestTSQLFactBoundOverflowIsALimit(t *testing.T) {
	var c tsqlFactCounts
	c.captures, c.execs, c.withErrors, c.semis = 1<<62, 1<<62, 1<<62, 1<<62
	c.children[0] = 1 << 62
	if _, _, e := tsqlFactBound(c); e == nil || e.Code != "ELIGIBILITY_WORK_LIMIT" {
		t.Fatalf("overflowing counts: %v", e)
	}
}
