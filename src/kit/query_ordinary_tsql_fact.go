package kit

import "math/bits"

// tsqlFactPairs are the capIndex.pairs calls deriveTSQL makes, as (parent, child) capture names. The
// charge below reads its counts from these names; a test counts the real calls and keeps the two equal.
var tsqlFactPairs = [...][2]string{
	{"exec.parent", "exec.child"}, {"proc.exec", "proc"}, {"ref", "ref.name"}, {"ref.s", "ref.schema"},
	{"arg", "arg.name"}, {"arg.v", "arg.value"}, {"bin", "bin.op"}, {"field", "field.name"}, {"err", "err.first"},
}

// tsqlFactCounts are the capture counts by name that bound deriveTSQL's loops.
type tsqlFactCounts struct {
	captures          uint64
	parents, children [len(tsqlFactPairs)]uint64 // per tsqlFactPairs row
	execs, semis      uint64                     // "exec", "semi"
	withErrors        uint64                     // "errwith"
	context           uint64                     // "batch.child"
}

func (c *tsqlFactCounts) add(name string) {
	c.captures++
	for i, p := range tsqlFactPairs {
		if name == p[0] {
			c.parents[i]++
		}
		if name == p[1] {
			c.children[i]++
		}
	}
	switch name {
	case "exec":
		c.execs++
	case "semi":
		c.semis++
	case "errwith":
		c.withErrors++
	case "batch.child":
		c.context++
	}
}

const (
	// A step is one element touched once: a loop iteration, a map access or a comparator call. It costs
	// work 64, the unit of the shared capture conversion charge.
	tsqlStepWork = 64
	// Logical per capture, for the cumulative allocation deriveTSQL makes in proportion to the capture list.
	// A Capture is 104 bytes and a growing slice allocates at most about five times its final size in all.
	// Per capture: the capture index holds it in two slices (2·5·104 = 1040 B); the nine pairs outputs hold at
	// most one 208 B pair per child capture (the child names are distinct), 5·208 = 1040 B; the per-node maps
	// take one entry (about 300 B with growth); the exec and context copies add at most 5·104 each (520 B each).
	// That sums to about 3.4 KB, so 4 KB is an upper bound; the test measures at most about 1.1 KB.
	tsqlLogicalPerCapture = 4 * 1024
	// Logical per (exec, child) pair: each exec rebuilds its argument slice from its children.
	tsqlLogicalPerExecChild = 1024
	// Logical per call, independent of the capture list: the two regexps deriveTSQL compiles (about 14 KB
	// measured with no captures at all) and the empty maps and slices.
	tsqlLogicalFixed = 32 * 1024
)

// tsqlFactBound is the work and logical charge for deriveTSQL's scans over the capture list. It counts,
// from the capture names only, the steps of every loop outside the decoded text (which the callers
// charge separately by span bytes):
//
//   - indexCaptures: two appends per capture.
//   - each of the nine pairs calls: one scan of all captures, a second one when its parent capture
//     exists, the sort of its pairs (at most 3 comparisons per element and bit of the count, pdqsort's
//     bound) and its output; the loop over the output is one more step per pair, two for exec.child
//     (the children map, then the comment filter) and one extra for err.first (its own loop).
//   - exec: the copy, the sort and, per exec, one step, one scan of the errwith captures, at most three
//     scans of its children, and the end-of-statement search over the execs and semi captures.
//   - batch.child: the copy, the sort and one walk. A known miss list is at most one per exec and sorted.
//
// The only products are exec x (errwith, children, exec, semi): they are the loops that can really be
// quadratic when many EXEC captures are repeated, so they use the real counts rather than the capture count.
func tsqlFactBound(c tsqlFactCounts) (work, logical uint64, e *Error) {
	const at = "ordinary-base-fact-derive"
	var steps uint64
	overflow := false
	add := func(v uint64) {
		if steps > ^uint64(0)-v {
			overflow = true
		}
		steps += v
	}
	mul := func(values ...uint64) uint64 {
		v, err := eligibilityProduct(at, values...)
		if err != nil {
			overflow = true
		}
		return v
	}
	sortSteps := func(k uint64) uint64 { return mul(3, k, uint64(bits.Len64(k))) }
	m := c.captures
	add(mul(2, m))
	for i := range tsqlFactPairs {
		scans := m
		if c.parents[i] > 0 {
			scans = mul(2, m)
		}
		add(scans)
		add(sortSteps(c.children[i]))
		add(mul(2, c.children[i])) // the pair outputs and the loop over them
	}
	add(c.children[0])                 // exec.child: the comment filter pass
	add(c.children[len(c.children)-1]) // err.first: the loop body's own step
	execs, kids := c.execs, c.children[0]
	add(execs)
	add(sortSteps(execs))
	add(mul(execs, 1+c.withErrors+mul(3, kids)+execs+c.semis))
	add(mul(2, c.context))
	add(sortSteps(c.context))
	add(sortSteps(execs)) // known misses
	if overflow {
		return 0, 0, fail(KindResourceLimit, "ELIGIBILITY_WORK_LIMIT", at, nil)
	}
	work = mul(tsqlStepWork, steps)
	logical = mul(tsqlLogicalPerCapture, m+1)
	logical += tsqlLogicalFixed + mul(tsqlLogicalPerExecChild, execs, kids)
	if overflow {
		return 0, 0, fail(KindResourceLimit, "ELIGIBILITY_WORK_LIMIT", at, nil)
	}
	return work, logical, nil
}
