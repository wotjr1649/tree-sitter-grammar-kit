package kit

import (
	"cmp"
	"fmt"
	"runtime"
	"slices"
	"testing"
	"unicode/utf8"
)

// Adversarial inputs for the C# fact reservation (#438): every shape asserts that 64 x the
// counted steps stay inside the work terms and that the cumulative allocation stays inside
// the logical reservation (assertCSharpFactChargeCovers).

func advCap(match uint32, name string, node int64, typ string, start, end uint32) Capture {
	return Capture{Match: match, Name: name, Node: node, Type: typ, Named: true, StartByte: start, EndByte: end}
}

// advAntiPDQ returns a permutation of 0..n-1 that makes slices.SortFunc compare the most
// (McIlroy's adversary): it fixes the pivot values lazily as the sort asks for them.
func advAntiPDQ(n int) []int {
	val := make([]int, n)
	gas := n - 1
	for i := range val {
		val[i] = gas
	}
	ids := make([]int, n)
	for i := range ids {
		ids[i] = i
	}
	solid, candidate := 0, 0
	slices.SortFunc(ids, func(x, y int) int {
		if val[x] == gas && val[y] == gas {
			if x == candidate {
				val[x] = solid
			} else {
				val[y] = solid
			}
			solid++
		}
		if val[x] == gas {
			candidate = x
		} else if val[y] == gas {
			candidate = y
		}
		return cmp.Compare(val[x], val[y])
	})
	return val
}

// advSortShape is one match with one parent and n children of one name in adversarial order.
func advSortShape(n int, parent, child string) []Capture {
	caps := []Capture{advCap(1, parent, 1, "x", 0, 0)}
	for _, k := range advAntiPDQ(n) {
		caps = append(caps, advCap(1, child, int64(k+10), "x", 0, 0))
	}
	return caps
}

// advChainShape is `sites` object creations whose type is a qualified name chain of depth
// nodes; a ring closes the chain on its head so resolve never reaches an identifier.
func advChainShape(sites, depth int, ring bool) ([]Capture, []byte) {
	src := []byte("SqlCommand CommandText x")
	var caps []Capture
	m := uint32(0)
	for i := 1; i <= depth; i++ {
		m++
		next, typ := int64(i+1), "qualified_name"
		if i == depth {
			if ring {
				next = 1
			} else {
				typ = "identifier"
			}
		}
		caps = append(caps, advCap(m, "qn", int64(i), "qualified_name", 0, 0), advCap(m, "qn.name", next, typ, 0, 10))
	}
	for s := 0; s < sites; s++ {
		oc, a, e := int64(1000+s), int64(100000+s), int64(200000+s)
		m += 3
		caps = append(caps,
			advCap(m-2, "oc", oc, "object_creation_expression", 0, 0), advCap(m-2, "oc.type", 1, "qualified_name", 0, 10),
			advCap(m-1, "oc.a", oc, "object_creation_expression", 0, 0), advCap(m-1, "oc.arg", a, "argument", 0, 0),
			advCap(m, "arg", a, "argument", 0, 0), advCap(m, "arg.expr", e, "identifier", 11, 21))
	}
	return caps, src
}

// advManyArgs is one oc.a match holding n oc.arg children, each with its own arg match.
func advManyArgs(n int) ([]Capture, []byte) {
	src := []byte("SqlCommand CommandText x")
	caps := []Capture{advCap(1, "oc", 1, "object_creation_expression", 0, 0), advCap(1, "oc.type", 2, "identifier", 0, 10), advCap(1, "oc.a", 1, "object_creation_expression", 0, 0)}
	for i := 0; i < n; i++ {
		caps = append(caps, advCap(1, "oc.arg", int64(10+i), "argument", 0, 0))
	}
	for i := 0; i < n; i++ {
		caps = append(caps, advCap(uint32(2+i), "arg", int64(10+i), "argument", 0, 0), advCap(uint32(2+i), "arg.expr", int64(100000+i), "identifier", 11, 21))
	}
	return caps, src
}

// advUnit encodes one rune of a span: ASCII names and a padding that decodes long. "zwsp" is a
// format character the identifier normalisation drops, so the name still matches and every
// decode of a site happens; "ascii" and "invalid" (an unpaired surrogate, a lone high byte or
// an invalid lead byte, each decoded to a 3-byte U+FFFD) never match but allocate more.
func advUnit(enc, kind string) []byte {
	switch enc + "/" + kind {
	case EncodingUTF8 + "/zwsp":
		return utf8.AppendRune(nil, 0x200B)
	case EncodingUTF8 + "/ascii":
		return []byte{'a'}
	case EncodingUTF8 + "/invalid":
		return []byte{0xFF}
	case EncodingUTF16LE + "/zwsp":
		return []byte{0x0B, 0x20}
	case EncodingUTF16LE + "/ascii":
		return []byte{'a', 0}
	case EncodingUTF16LE + "/invalid":
		return []byte{0x00, 0xD8}
	case EncodingUTF16BE + "/zwsp":
		return []byte{0x20, 0x0B}
	case EncodingUTF16BE + "/ascii":
		return []byte{0, 'a'}
	case EncodingUTF16BE + "/invalid":
		return []byte{0xD8, 0x00}
	case EncodingCP949 + "/ascii":
		return []byte{'a'}
	case EncodingCP949 + "/invalid":
		return []byte{0x80}
	}
	return nil // no such padding (CP949 has no zero-width space)
}

func advRegion(enc, name string, size int, pad []byte) []byte {
	var b []byte
	for i := 0; i < len(name); i++ {
		switch enc {
		case EncodingUTF16LE:
			b = append(b, name[i], 0)
		case EncodingUTF16BE:
			b = append(b, 0, name[i])
		default:
			b = append(b, name[i])
		}
	}
	for len(b) < size {
		b = append(b, pad...)
	}
	return b
}

// advHugeSpans is `sites` command sites and assignments whose identifiers, argument and
// right-hand side all span `size` bytes of one region (the command type and the property name).
func advHugeSpans(enc, kind string, size, sites int) ([]Capture, []byte) {
	pad := advUnit(enc, kind)
	r0, r1 := advRegion(enc, "SqlCommand", size, pad), advRegion(enc, "CommandText", size, pad)
	src := append(slices.Clone(r0), r1...)
	e0, e1 := uint32(len(r0)), uint32(len(src))
	var caps []Capture
	m := uint32(0)
	for s := 0; s < sites; s++ {
		oc, a, e, as := int64(1000+s), int64(100000+s), int64(200000+s), int64(300000+s)
		caps = append(caps,
			advCap(m+1, "oc", oc, "object_creation_expression", 0, 0), advCap(m+1, "oc.type", 1, "identifier", 0, e0),
			advCap(m+2, "oc.a", oc, "object_creation_expression", 0, 0), advCap(m+2, "oc.arg", a, "argument", 0, 0),
			advCap(m+3, "arg", a, "argument", 0, 0), advCap(m+3, "arg.expr", e, "identifier", 0, e0),
			advCap(m+4, "ma.as", as, "assignment_expression", 0, 0), advCap(m+4, "ma.name", int64(400000+s), "identifier", e0, e1),
			advCap(m+5, "ma.as", as, "assignment_expression", 0, 0), advCap(m+5, "ma.right", int64(500000+s), "identifier", 0, e0))
		m += 5
	}
	return caps, src
}

type csharpAdversarialCase struct {
	label string
	caps  []Capture
	src   []byte
	enc   string
}

func csharpAdversarialCases() (sorts, chains, decodes []csharpAdversarialCase) {
	for _, n := range []int{100, 2000, 20000} {
		for _, pc := range [][2]string{{"oc.a", "oc.arg"}, {"qn", "qn.name"}, {"arg", "arg.expr"}, {"oc", "oc.type"}, {"ma.as", "ma.right"}} {
			sorts = append(sorts, csharpAdversarialCase{fmt.Sprintf("anti-pdqsort %s x%d", pc[1], n), advSortShape(n, pc[0], pc[1]), nil, EncodingUTF8})
		}
	}
	for _, ring := range []bool{false, true} {
		for _, ak := range [][2]int{{1, 500}, {500, 1}, {10, 500}, {200, 200}} {
			caps, src := advChainShape(ak[0], ak[1], ring)
			chains = append(chains, csharpAdversarialCase{fmt.Sprintf("chain ring=%v sites=%d depth=%d", ring, ak[0], ak[1]), caps, src, EncodingUTF8})
		}
	}
	caps, src := advManyArgs(3000)
	chains = append(chains, csharpAdversarialCase{"one match with 3000 oc.arg", caps, src, EncodingUTF8})
	for _, enc := range []string{EncodingUTF8, EncodingUTF16LE, EncodingUTF16BE, EncodingCP949} {
		for _, kind := range []string{"zwsp", "ascii", "invalid"} {
			if advUnit(enc, kind) == nil {
				continue
			}
			for _, shape := range [][2]int{{1 << 17, 8}, {1 << 10, 100}} {
				caps, src := advHugeSpans(enc, kind, shape[0], shape[1])
				decodes = append(decodes, csharpAdversarialCase{fmt.Sprintf("huge spans %s/%s size=%d sites=%d", enc, kind, shape[0], shape[1]), caps, src, enc})
			}
		}
	}
	return sorts, chains, decodes
}

func TestCSharpFactChargeCoversAdversarialInputs(t *testing.T) {
	sorts, chains, decodes := csharpAdversarialCases()
	var worst csharpFactRatio
	for _, c := range slices.Concat(sorts, chains, decodes) {
		_, ratio := assertCSharpFactChargeCovers(t, c.label, c.caps, c.src, c.enc)
		t.Logf("%-46s captures=%-6d used %+v", c.label, len(c.caps), ratio)
		worst.widen(ratio)
	}
	t.Logf("worst use of each reservation: %+v", worst)
	// The inputs must come close to every reservation the code could drop or shrink: a chain or
	// decode term cut in half, or the sorted passes cut from 12 to 1 or 2, is crossed here.
	if worst.chain < 0.5 || worst.decode < 0.5 || worst.sort < 0.1 || worst.logical < 0.2 {
		t.Fatalf("the adversarial inputs no longer exercise the reservations: %+v", worst)
	}
}

// One decode (srcText.span and the identifier normalisation of its text) allocates at most
// csharpDecodeLogical bytes per source byte in every encoding and for every kind of text.
func TestCSharpDecodeAllocationStaysInsideItsCoefficient(t *testing.T) {
	const size = 1 << 20
	var peak float64
	for _, enc := range []string{EncodingUTF8, EncodingUTF16LE, EncodingUTF16BE, EncodingCP949} {
		for _, kind := range []string{"zwsp", "ascii", "invalid"} {
			pad := advUnit(enc, kind)
			if pad == nil {
				continue
			}
			s := srcText{advRegion(enc, "SqlCommand", size, pad), enc}
			var text string
			alloc := csharpAllocated(func() { text = s.span(0, uint32(len(s.b))) }) + csharpAllocated(func() { _ = csharpIdentifierName(text) })
			per := float64(alloc) / float64(len(s.b))
			peak = max(peak, per)
			if per > csharpDecodeLogical {
				t.Errorf("%s/%s: %.2f bytes allocated per source byte exceed the coefficient %d", enc, kind, per, csharpDecodeLogical)
			}
		}
	}
	t.Logf("largest allocation per decoded byte: %.2f (coefficient %d)", peak, csharpDecodeLogical)
	if peak < csharpDecodeLogical/4 {
		t.Fatalf("no input allocates near the coefficient (%.2f of %d): the measurement lost its worst case", peak, csharpDecodeLogical)
	}
}

// csharpAllocated returns the bytes f allocates, cumulatively (not the live heap).
func csharpAllocated(f func()) uint64 {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}
