package kit

import "unicode/utf8"

// Encoding outcome values bound into identity (real-world source policy steps 1-6).
const (
	EncodingUTF8    = "UTF-8"
	EncodingUTF16LE = "UTF-16LE"
	EncodingUTF16BE = "UTF-16BE"
	EncodingCP949   = "CP949"

	SourceBOM         = "BOM"
	SourceValidation  = "VALIDATION"
	SourceDeclaration = "DECLARATION"
)

// EncodingOutcome is the per-file detection result. Assessment is PASS only when the
// encoding was determined and its content validated; BLOCKED and UNRESOLVED carry a code.
type EncodingOutcome struct {
	Assessment string `json:"assessment"`
	Encoding   string `json:"encoding,omitempty"`
	Source     string `json:"source,omitempty"`
	Code       string `json:"code,omitempty"`
}

// EncodingPolicy carries caller declarations. Profile "cp949" enables steps 4-5 for
// cp949; Files replace steps 4-5 for the named files only.
type EncodingPolicy struct {
	Profile string         `json:"profile,omitempty"`
	Files   []FileEncoding `json:"files,omitempty"`
}

// FileEncoding declares "utf-8" or "cp949" for one portable path.
type FileEncoding struct {
	Path     string `json:"path"`
	Encoding string `json:"encoding"`
}

// detector validates a byte stream in one pass with constant state.
type detector struct {
	n        uint64
	head     [4]byte
	headLen  int
	bom      string // decided once 4 bytes (or EOF) are seen
	bomLen   int
	decided  bool
	pending  []byte // bytes buffered until the BOM decision
	nul      bool
	nonASCII bool
	// UTF-8 validation state: up to 3 carried bytes of an incomplete sequence.
	u8carry []byte
	u8bad   bool
	// UTF-16 state.
	u16odd     int // -1 none, else carried byte
	u16high    bool
	u16bad     string
	u16started bool
	// cp949 state: a lead byte waiting for its trail, a structural violation, and a
	// well-formed pair without a code point in the pinned index-euc-kr.
	cpLead     bool
	cpLeadByte byte
	cpBad      bool
	cpUnmapped bool
}

func newDetector() *detector { return &detector{u16odd: -1} }

func (d *detector) Write(p []byte) (int, error) {
	if !d.decided {
		for _, b := range p {
			if d.headLen < 4 {
				d.head[d.headLen] = b
				d.headLen++
			}
		}
		d.pending = append(d.pending, p...)
		if d.headLen < 4 {
			return len(p), nil
		}
		d.decide()
		buf := d.pending
		d.pending = nil
		d.feed(buf)
		return len(p), nil
	}
	d.feed(p)
	return len(p), nil
}

func (d *detector) decide() {
	d.decided = true
	h := d.head[:d.headLen]
	switch {
	case len(h) >= 4 && ((h[0] == 0xFF && h[1] == 0xFE && h[2] == 0 && h[3] == 0) || (h[0] == 0 && h[1] == 0 && h[2] == 0xFE && h[3] == 0xFF)):
		d.bom, d.bomLen = "UTF-32", 4
	case len(h) >= 3 && h[0] == 0xEF && h[1] == 0xBB && h[2] == 0xBF:
		d.bom, d.bomLen = EncodingUTF8, 3
	case len(h) >= 2 && h[0] == 0xFF && h[1] == 0xFE:
		d.bom, d.bomLen = EncodingUTF16LE, 2
	case len(h) >= 2 && h[0] == 0xFE && h[1] == 0xFF:
		d.bom, d.bomLen = EncodingUTF16BE, 2
	}
}

func (d *detector) feed(p []byte) {
	skip := 0
	if d.n < uint64(d.bomLen) {
		skip = int(min(uint64(d.bomLen)-d.n, uint64(len(p))))
	}
	d.n += uint64(len(p))
	body := p[skip:]
	if d.bom == EncodingUTF16LE || d.bom == EncodingUTF16BE {
		d.feedUTF16(body)
		return
	}
	for _, b := range body {
		if b == 0 {
			d.nul = true
		}
		if b >= 0x80 {
			d.nonASCII = true
		}
		// cp949 structure (WHATWG euc-kr decoder shape): ASCII, or lead 0x81-0xFE + trail 0x41-0xFE.
		if d.cpLead {
			if b < 0x41 || b == 0xFF {
				d.cpBad = true
			} else if !CP949Pair(d.cpLeadByte, b) {
				d.cpUnmapped = true
			}
			d.cpLead = false
		} else if b >= 0x80 {
			if b == 0x80 || b == 0xFF {
				d.cpBad = true
			} else {
				d.cpLead, d.cpLeadByte = true, b
			}
		}
	}
	if !d.u8bad {
		buf := body
		if len(d.u8carry) > 0 {
			buf = append(d.u8carry, body...)
			d.u8carry = nil
		}
		for len(buf) > 0 {
			if buf[0] < utf8.RuneSelf {
				buf = buf[1:]
				continue
			}
			if !utf8.FullRune(buf) {
				d.u8carry = append([]byte(nil), buf...)
				break
			}
			r, size := utf8.DecodeRune(buf)
			if r == utf8.RuneError && size == 1 {
				d.u8bad = true
				break
			}
			buf = buf[size:]
		}
	}
}

func (d *detector) feedUTF16(body []byte) {
	for _, b := range body {
		if d.u16odd < 0 {
			d.u16odd = int(b)
			continue
		}
		var u uint16
		if d.bom == EncodingUTF16LE {
			u = uint16(d.u16odd) | uint16(b)<<8
		} else {
			u = uint16(d.u16odd)<<8 | uint16(b)
		}
		d.u16odd = -1
		if d.u16bad != "" {
			continue
		}
		switch {
		case u == 0:
			d.u16bad = "UTF16_NUL"
		case u >= 0xD800 && u <= 0xDBFF:
			if d.u16high {
				d.u16bad = "UTF16_UNPAIRED_SURROGATE"
			}
			d.u16high = true
		case u >= 0xDC00 && u <= 0xDFFF:
			if !d.u16high {
				d.u16bad = "UTF16_UNPAIRED_SURROGATE"
			}
			d.u16high = false
		default:
			if d.u16high {
				d.u16bad = "UTF16_UNPAIRED_SURROGATE"
			}
		}
	}
}

func blocked(enc, source, code string) EncodingOutcome {
	return EncodingOutcome{Assessment: "BLOCKED", Encoding: enc, Source: source, Code: code}
}

// result applies steps 1-6. declared is "", "utf-8" or "cp949" for this file.
func (d *detector) result(profileCP949 bool, declared string) EncodingOutcome {
	if !d.decided {
		d.decide()
		buf := d.pending
		d.pending = nil
		d.feed(buf)
	}
	switch d.bom {
	case "UTF-32": // step 1
		return blocked("", SourceBOM, "UTF32_BOM")
	case EncodingUTF8: // step 2: validate under the BOM encoding, never fall back
		if d.u8bad || len(d.u8carry) > 0 {
			return blocked(EncodingUTF8, SourceBOM, "BOM_CONTENT_INVALID")
		}
		return EncodingOutcome{Assessment: "PASS", Encoding: EncodingUTF8, Source: SourceBOM}
	case EncodingUTF16LE, EncodingUTF16BE:
		switch {
		case d.u16odd >= 0:
			return blocked(d.bom, SourceBOM, "UTF16_ODD_LENGTH")
		case d.u16bad != "":
			return blocked(d.bom, SourceBOM, d.u16bad)
		case d.u16high:
			return blocked(d.bom, SourceBOM, "UTF16_UNPAIRED_SURROGATE")
		}
		return EncodingOutcome{Assessment: "PASS", Encoding: d.bom, Source: SourceBOM}
	}
	if d.nul { // step 3
		return blocked("", "", "NUL_WITHOUT_BOM")
	}
	utf8OK := !d.u8bad && len(d.u8carry) == 0
	cpPossible := !d.cpBad && !d.cpLead    // structure of the WHATWG euc-kr decoder
	cpValid := cpPossible && !d.cpUnmapped // and every pair has a code point in the pinned table
	switch declared {                      // per-file declarations replace steps 4-5 only
	case "utf-8":
		if utf8OK {
			return EncodingOutcome{Assessment: "PASS", Encoding: EncodingUTF8, Source: SourceDeclaration}
		}
		return blocked(EncodingUTF8, SourceDeclaration, "DECLARED_ENCODING_INVALID")
	case "cp949":
		if !cpValid {
			return blocked(EncodingCP949, SourceDeclaration, "DECLARED_ENCODING_INVALID")
		}
		return EncodingOutcome{Assessment: "PASS", Encoding: EncodingCP949, Source: SourceDeclaration}
	}
	if utf8OK { // step 4
		if profileCP949 && d.nonASCII && cpValid {
			return blocked("", "", "AMBIGUOUS_ENCODING")
		}
		return EncodingOutcome{Assessment: "PASS", Encoding: EncodingUTF8, Source: SourceValidation}
	}
	if profileCP949 && cpPossible { // step 5
		if d.cpUnmapped { // Windows-only extensions and other pairs outside the table
			return blocked(EncodingCP949, SourceValidation, "CP949_UNMAPPED")
		}
		return EncodingOutcome{Assessment: "PASS", Encoding: EncodingCP949, Source: SourceValidation}
	}
	return blocked("", "", "UNDETERMINED_ENCODING") // step 6
}
