package kit

import (
	"bytes"
	_ "embed"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// index-euc-kr is the pinned WHATWG table (see data/NOTICE-index-euc-kr.md).
//
//go:embed data/index-euc-kr.txt
var euckrIndex []byte

// EUCKRIndexSHA256 and EUCKRIdentifier pin the embedded table.
const (
	EUCKRIndexSHA256 = "89af20dd867c84cefb710b1790229786cfef2bf11916361a210d81b90381e267"
	EUCKRIdentifier  = "1d97134cbf187263585bc8f593ca4196654ed4c7a673f5672eaad4f5d9fdc4ba"
	euckrPointers    = 126 * 190
)

// euckrTable maps pointer (lead-0x81)*190 + (trail-0x41) to its code point; 0 is absent.
var euckrTable = sync.OnceValue(func() *[euckrPointers]uint16 {
	var t [euckrPointers]uint16
	for line := range strings.SplitSeq(string(euckrIndex), "\n") {
		if line == "" || line[0] == '#' {
			continue
		}
		cols := strings.Split(line, "\t")
		p, err1 := strconv.Atoi(strings.TrimSpace(cols[0]))
		cp, err2 := strconv.ParseUint(strings.TrimPrefix(cols[1], "0x"), 16, 16)
		if err1 != nil || err2 != nil || p < 0 || p >= euckrPointers {
			panic("embedded index-euc-kr is malformed")
		}
		t[p] = uint16(cp)
	}
	return &t
})

// CP949Pair reports whether lead+trail is a two-byte character with a code point in the
// pinned index-euc-kr (Windows-only extensions outside the index are not).
func CP949Pair(lead, trail byte) bool {
	if lead < 0x81 || lead == 0xff || trail < 0x41 || trail == 0xff {
		return false
	}
	return euckrTable()[int(lead-0x81)*190+int(trail-0x41)] != 0
}

// CP949Rune decodes the pair with the pinned table, or returns false.
func CP949Rune(lead, trail byte) (rune, bool) {
	if !CP949Pair(lead, trail) {
		return 0, false
	}
	return rune(euckrTable()[int(lead-0x81)*190+int(trail-0x41)]), true
}

func unit16(enc string, s []byte) uint32 {
	if enc == EncodingUTF16LE {
		return uint32(s[0]) | uint32(s[1])<<8
	}
	return uint32(s[0])<<8 | uint32(s[1])
}

// SourceEncodingValid applies the tsgk-native/r1 source rule of the declared encoding:
// UTF-8 accepts any bytes; UTF-16 needs even length, paired surrogates and no U+0000;
// CP949 needs ASCII or table pairs only.
func SourceEncodingValid(enc string, s []byte) bool {
	switch enc {
	case EncodingUTF8:
		return true
	case EncodingCP949:
		for i := 0; i < len(s); i++ {
			if s[i] < 0x80 {
				continue
			}
			if i+1 >= len(s) || !CP949Pair(s[i], s[i+1]) {
				return false
			}
			i++
		}
		return true
	case EncodingUTF16LE, EncodingUTF16BE:
		if len(s)%2 != 0 {
			return false
		}
		high := false
		for i := 0; i < len(s); i += 2 {
			u := unit16(enc, s[i:])
			switch {
			case u == 0:
				return false
			case u >= 0xd800 && u <= 0xdbff:
				if high {
					return false
				}
				high = true
			case u >= 0xdc00 && u <= 0xdfff:
				if !high {
					return false
				}
				high = false
			default:
				if high {
					return false
				}
			}
		}
		return !high
	}
	return false
}

// boundaryCode returns the violation code when off is not an allowed edit boundary.
func boundaryCode(enc string, s []byte, off int) string {
	if enc == EncodingUTF16LE || enc == EncodingUTF16BE {
		if off%2 != 0 {
			return "EDIT_ODD_UTF16"
		}
		if off >= 2 && off+2 <= len(s) {
			a, b := unit16(enc, s[off-2:]), unit16(enc, s[off:])
			if a >= 0xd800 && a <= 0xdbff && b >= 0xdc00 && b <= 0xdfff {
				return "EDIT_SPLITS_CHARACTER"
			}
		}
		return ""
	}
	for i := 0; i < off; {
		size := 1
		if enc == EncodingCP949 {
			if s[i] >= 0x80 {
				size = 2
			}
		} else if r, n := utf8.DecodeRune(s[i:]); !(r == utf8.RuneError && n == 1) {
			size = n // a well-formed character; an invalid byte is its own unit
		}
		if i+size > off {
			return "EDIT_SPLITS_CHARACTER"
		}
		i += size
	}
	return ""
}

// PointAt computes the 0-based row (LF count) and byte column of off under enc; it never
// counts Unicode scalar values.
func PointAt(enc string, s []byte, off int) Point {
	var row, line int
	if enc == EncodingUTF16LE || enc == EncodingUTF16BE {
		for i := 0; i+2 <= off; i += 2 {
			if unit16(enc, s[i:]) == 0x0a {
				row, line = row+1, i+2
			}
		}
	} else {
		for i := 0; i < off; i++ {
			if s[i] == '\n' {
				row, line = row+1, i+1
			}
		}
	}
	return Point{Row: uint32(row), Column: uint32(off - line)}
}

// Edit is one byte edit: Old must equal the current [StartByte, OldEndByte) and New
// replaces it, so NewEndByte = StartByte + len(New).
type Edit struct {
	StartByte  uint32 `json:"start_byte"`
	OldEndByte uint32 `json:"old_end_byte"`
	NewEndByte uint32 `json:"new_end_byte"`
	Old        []byte `json:"old"`
	New        []byte `json:"new"`
}

// EditPoints are the native edit points of one applied edit.
type EditPoints struct {
	Start  Point `json:"start_point"`
	OldEnd Point `json:"old_end_point"`
	NewEnd Point `json:"new_end_point"`
}

// ApplyEdits validates and applies edits in order under the declared encoding, returning
// every intermediate source (versions[0] is source) and the computed edit points. Any
// violation rejects the whole sequence before anything is parsed; nothing is repaired.
func ApplyEdits(enc string, source []byte, edits []Edit, maxBytes uint64) ([][]byte, []EditPoints, error) {
	bad := func(code string, i int) error {
		return fail(KindInvalidInput, code, "#/edits/"+strconv.Itoa(i), nil)
	}
	if enc != EncodingUTF8 && enc != EncodingUTF16LE && enc != EncodingUTF16BE && enc != EncodingCP949 {
		return nil, nil, fail(KindInvalidInput, "ENCODING_UNSUPPORTED", "", nil)
	}
	if uint64(len(source)) > maxBytes {
		return nil, nil, fail(KindInvalidInput, "INPUT_TOO_LARGE", "", nil)
	}
	if !SourceEncodingValid(enc, source) {
		return nil, nil, fail(KindInvalidInput, "SOURCE_ENCODING_INVALID", "", nil)
	}
	versions := [][]byte{source}
	var points []EditPoints
	for i, e := range edits {
		cur := versions[len(versions)-1]
		if e.StartByte > e.OldEndByte || uint64(e.OldEndByte) > uint64(len(cur)) {
			return nil, nil, bad("EDIT_RANGE", i)
		}
		if !bytes.Equal(cur[e.StartByte:e.OldEndByte], e.Old) {
			return nil, nil, bad("EDIT_OLD_MISMATCH", i)
		}
		if uint64(e.NewEndByte) != uint64(e.StartByte)+uint64(len(e.New)) {
			return nil, nil, bad("EDIT_LENGTH_INCONSISTENT", i)
		}
		if len(e.Old) == 0 && len(e.New) == 0 {
			return nil, nil, bad("EDIT_EMPTY", i)
		}
		size := uint64(len(cur)) - uint64(len(e.Old)) + uint64(len(e.New))
		if size > maxBytes {
			return nil, nil, bad("INPUT_TOO_LARGE", i)
		}
		for _, off := range []uint32{e.StartByte, e.OldEndByte} {
			if c := boundaryCode(enc, cur, int(off)); c != "" {
				return nil, nil, bad(c, i)
			}
		}
		next := make([]byte, 0, size)
		next = append(next, cur[:e.StartByte]...)
		next = append(next, e.New...)
		next = append(next, cur[e.OldEndByte:]...)
		if !SourceEncodingValid(enc, next) {
			return nil, nil, bad("EDIT_ENCODING_INVALID", i)
		}
		for _, off := range []uint32{e.StartByte, e.NewEndByte} {
			if c := boundaryCode(enc, next, int(off)); c != "" {
				return nil, nil, bad(c, i)
			}
		}
		points = append(points, EditPoints{Start: PointAt(enc, cur, int(e.StartByte)), OldEnd: PointAt(enc, cur, int(e.OldEndByte)), NewEnd: PointAt(enc, next, int(e.NewEndByte))})
		versions = append(versions, next)
	}
	return versions, points, nil
}
