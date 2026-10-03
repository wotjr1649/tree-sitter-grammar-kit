package kit

import (
	"bytes"
	"testing"
)

// S01-A15/A16: steps 1-6 of the real-world source policy, in order, fed in one chunk
// and byte by byte so streaming state across chunk boundaries is exercised.
func TestEncodingSteps(t *testing.T) {
	han := []byte("\xED\x95\x9C\xEA\xB8\x80")  // two Hangul syllables in UTF-8; also cp949-shaped
	cp := []byte("\xC7\xD1")                   // cp949-shaped, invalid UTF-8
	both := []byte("\xEA\xB0\x81\xEA\xB0\x81") // valid UTF-8 whose cp949 pairs are all in index-euc-kr
	unmapped := []byte("\xFE\xA1")             // invalid UTF-8; cp949-shaped user-defined row absent from index-euc-kr
	cases := []struct {
		name     string
		data     []byte
		cp949    bool
		declared string
		want     EncodingOutcome
	}{
		{"utf32le-bom", []byte("\xFF\xFE\x00\x00a\x00\x00\x00"), false, "", EncodingOutcome{"BLOCKED", "", "BOM", "UTF32_BOM"}},
		{"utf32be-bom", []byte("\x00\x00\xFE\xFF\x00\x00\x00a"), false, "", EncodingOutcome{"BLOCKED", "", "BOM", "UTF32_BOM"}},
		{"utf32-bom-beats-declaration", []byte("\xFF\xFE\x00\x00"), false, "utf-8", EncodingOutcome{"BLOCKED", "", "BOM", "UTF32_BOM"}},
		{"utf8-bom-valid", append([]byte("\xEF\xBB\xBFa"), han...), false, "", EncodingOutcome{"PASS", "UTF-8", "BOM", ""}},
		{"utf8-bom-invalid-no-fallback", []byte("\xEF\xBB\xBFa\xC7\xD1"), true, "", EncodingOutcome{"BLOCKED", "UTF-8", "BOM", "BOM_CONTENT_INVALID"}},
		{"utf8-bom-truncated", []byte("\xEF\xBB\xBF\xED\x95"), false, "", EncodingOutcome{"BLOCKED", "UTF-8", "BOM", "BOM_CONTENT_INVALID"}},
		{"utf8-bom-beats-cp949-declaration", []byte("\xEF\xBB\xBFa"), false, "cp949", EncodingOutcome{"PASS", "UTF-8", "BOM", ""}},
		{"utf16le-valid", []byte("\xFF\xFEa\x00\x3D\xD8\x00\xDE"), false, "", EncodingOutcome{"PASS", "UTF-16LE", "BOM", ""}},
		{"utf16be-valid", []byte("\xFE\xFF\x00a\xD8\x3D\xDE\x00"), false, "", EncodingOutcome{"PASS", "UTF-16BE", "BOM", ""}},
		{"utf16le-odd", []byte("\xFF\xFEa\x00b"), false, "", EncodingOutcome{"BLOCKED", "UTF-16LE", "BOM", "UTF16_ODD_LENGTH"}},
		{"utf16be-odd", []byte("\xFE\xFF\x00"), false, "", EncodingOutcome{"BLOCKED", "UTF-16BE", "BOM", "UTF16_ODD_LENGTH"}},
		{"utf16le-lone-high", []byte("\xFF\xFE\x3D\xD8a\x00"), false, "", EncodingOutcome{"BLOCKED", "UTF-16LE", "BOM", "UTF16_UNPAIRED_SURROGATE"}},
		{"utf16le-high-at-eof", []byte("\xFF\xFEa\x00\x3D\xD8"), false, "", EncodingOutcome{"BLOCKED", "UTF-16LE", "BOM", "UTF16_UNPAIRED_SURROGATE"}},
		{"utf16be-lone-low", []byte("\xFE\xFF\xDE\x00"), false, "", EncodingOutcome{"BLOCKED", "UTF-16BE", "BOM", "UTF16_UNPAIRED_SURROGATE"}},
		{"utf16le-nul", []byte("\xFF\xFEa\x00\x00\x00"), false, "", EncodingOutcome{"BLOCKED", "UTF-16LE", "BOM", "UTF16_NUL"}},
		{"utf16-bom-beats-declaration", []byte("\xFF\xFEa"), false, "cp949", EncodingOutcome{"BLOCKED", "UTF-16LE", "BOM", "UTF16_ODD_LENGTH"}},
		{"nul-without-bom", []byte("ab\x00c"), false, "", EncodingOutcome{"BLOCKED", "", "", "NUL_WITHOUT_BOM"}},
		{"nul-beats-declaration", []byte("a\x00"), false, "utf-8", EncodingOutcome{"BLOCKED", "", "", "NUL_WITHOUT_BOM"}},
		{"empty", nil, false, "", EncodingOutcome{"PASS", "UTF-8", "VALIDATION", ""}},
		{"ascii", []byte("int x;\r\n"), false, "", EncodingOutcome{"PASS", "UTF-8", "VALIDATION", ""}},
		{"ascii-under-cp949-profile", []byte("int x;\n"), true, "", EncodingOutcome{"PASS", "UTF-8", "VALIDATION", ""}},
		{"strict-utf8", append([]byte("a"), han...), false, "", EncodingOutcome{"PASS", "UTF-8", "VALIDATION", ""}},
		{"utf8-surrogate-is-invalid", []byte("\xED\xA0\x80"), false, "", EncodingOutcome{"BLOCKED", "", "", "UNDETERMINED_ENCODING"}},
		{"utf8-overlong-is-invalid", []byte("\xC0\xAF"), false, "", EncodingOutcome{"BLOCKED", "", "", "UNDETERMINED_ENCODING"}},
		{"ambiguous", append([]byte("a"), both...), true, "", EncodingOutcome{"BLOCKED", "", "", "AMBIGUOUS_ENCODING"}},
		{"utf8-with-unmapped-cp949-pairs", append([]byte("a"), han...), true, "", EncodingOutcome{"PASS", "UTF-8", "VALIDATION", ""}},
		{"utf8-not-cp949-shaped", []byte("\xE2\x82\xAC"), true, "", EncodingOutcome{"PASS", "UTF-8", "VALIDATION", ""}},
		{"cp949-profile-valid", cp, true, "", EncodingOutcome{"PASS", "CP949", "VALIDATION", ""}},
		{"cp949-profile-unmapped", unmapped, true, "", EncodingOutcome{"BLOCKED", "CP949", "VALIDATION", "CP949_UNMAPPED"}},
		{"cp949-profile-structurally-invalid", []byte("\xC7\x20"), true, "", EncodingOutcome{"BLOCKED", "", "", "UNDETERMINED_ENCODING"}},
		{"cp949-truncated-lead", []byte("a\xC7"), true, "", EncodingOutcome{"BLOCKED", "", "", "UNDETERMINED_ENCODING"}},
		{"no-profile-no-cp949", cp, false, "", EncodingOutcome{"BLOCKED", "", "", "UNDETERMINED_ENCODING"}},
		{"declared-utf8-valid", append([]byte("a"), han...), true, "utf-8", EncodingOutcome{"PASS", "UTF-8", "DECLARATION", ""}},
		{"declared-utf8-invalid", cp, true, "utf-8", EncodingOutcome{"BLOCKED", "UTF-8", "DECLARATION", "DECLARED_ENCODING_INVALID"}},
		{"declared-cp949-valid", cp, false, "cp949", EncodingOutcome{"PASS", "CP949", "DECLARATION", ""}},
		{"declared-cp949-unmapped", unmapped, false, "cp949", EncodingOutcome{"BLOCKED", "CP949", "DECLARATION", "DECLARED_ENCODING_INVALID"}},
		{"declared-cp949-ascii", []byte("abc"), false, "cp949", EncodingOutcome{"PASS", "CP949", "DECLARATION", ""}},
		{"declared-cp949-invalid", []byte("\xFF"), false, "cp949", EncodingOutcome{"BLOCKED", "CP949", "DECLARATION", "DECLARED_ENCODING_INVALID"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			whole := newDetector()
			whole.Write(tc.data)
			if got := whole.result(tc.cp949, tc.declared); got != tc.want {
				t.Fatalf("whole: got %+v want %+v", got, tc.want)
			}
			split := newDetector()
			for i := range tc.data {
				split.Write(tc.data[i : i+1])
			}
			if got := split.result(tc.cp949, tc.declared); got != tc.want {
				t.Fatalf("byte-by-byte: got %+v want %+v", got, tc.want)
			}
		})
	}
}

// The detector must not alter the bytes it observes (raw hash/size stay exact).
func TestEncodingDoesNotMutateInput(t *testing.T) {
	data := []byte("\xEF\xBB\xBFa\r\nb\x00")
	keep := bytes.Clone(data)
	d := newDetector()
	d.Write(data)
	d.result(true, "")
	if !bytes.Equal(data, keep) {
		t.Fatal("input mutated")
	}
}
