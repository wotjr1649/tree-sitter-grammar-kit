package kit

import (
	"fmt"
	"math/rand"
	"runtime"
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

// derive runs the adapter and returns the steps it took outside the decoded text, the number of pairs
// calls, the decoded bytes, the number of decode calls and the cumulative allocation.
func (c tsqlFactCase) derive(t *testing.T) (steps, pairCalls, decoded, decodeCalls, alloc uint64) {
	t.Helper()
	previous := factStepHook
	factStepHook = func(kind string, n uint64) {
		switch kind {
		case "pair-steps":
			pairCalls++
			steps += n
		case "index-steps", "tsql-steps":
			steps += n
		case "decoded-bytes":
			decoded += n
			decodeCalls++
		}
	}
	defer func() { factStepHook = previous }()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if _, err := DeriveDynamicSQLChecked("tsql", EncodingUTF8, c.caps, c.src); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	return steps, pairCalls, decoded, decodeCalls, after.TotalAlloc - before.TotalAlloc
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
			steps, pairCalls, decoded, decodeCalls, alloc := c.derive(t)
			if pairCalls != uint64(len(tsqlFactPairs)) {
				t.Fatalf("deriveTSQL made %d pairs calls, tsqlFactPairs lists %d", pairCalls, len(tsqlFactPairs))
			}
			work, logical, e := tsqlFactBound(c.counts())
			if e != nil {
				t.Fatal(e)
			}
			if work < tsqlStepWork*steps {
				t.Fatalf("work %d < %d x %d real steps", work, tsqlStepWork, steps)
			}
			// The decoded text is charged separately by span bytes (64(E+1)A and 64EWS); the strings it allocates are
			// at most the decoded bytes (rounded up to a 16 byte size class) and are left out of the capture-list term.
			slack := 2*decoded + 16*decodeCalls
			t.Logf("captures %d: steps %d, work bound %d (%.1fx real), allocated %d B of logical %d B", len(c.caps), steps, work, float64(work)/float64(tsqlStepWork*steps+1), alloc, logical)
			if alloc > logical+slack {
				t.Fatalf("allocated %d B > logical %d B (+%d decode slack)", alloc, logical, slack)
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
		n := uint64(captures) + 1
		if old := 128 * n * n; captures >= 150 && (work >= old || logical >= old) {
			t.Fatalf("captures %d: work %d logical %d not below the previous 128n² = %d", captures, work, logical, old)
		}
	}
	// A real product still dominates when EXEC captures repeat: 200 execs x 200 children.
	c := tsqlFactDuplicateExec(200, 200)
	steps, _, _, _, _ := c.derive(t)
	work, _, e := tsqlFactBound(c.counts())
	if e != nil {
		t.Fatal(e)
	}
	if steps < 200*200 || work < tsqlStepWork*200*200 {
		t.Fatalf("the exec x children product was lost: steps %d work %d", steps, work)
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
