package kit

import "strconv"

// CaptureCompareScheme names the single capture-stream comparator.
const CaptureCompareScheme = "tsgk-capture-compare/r1"

// Capture is one query capture in the order the pinned native runtime returned it (Session
// 06). Match is the runtime's match number (assigned when a match first returns a capture),
// Pattern and Capture are the query's pattern and capture indices, Name is the capture
// name, and Node is the captured node's preorder index in the tree it was taken from; the
// node's type, flags and range are repeated so a summary record is self-describing.
type Capture struct {
	Match      uint32 `json:"match"`
	Pattern    uint32 `json:"pattern"`
	Capture    uint32 `json:"capture"`
	Name       string `json:"name"`
	Node       int64  `json:"node"`
	Type       string `json:"type"`
	Named      bool   `json:"named"`
	Extra      bool   `json:"extra"`
	IsError    bool   `json:"is_error"`
	HasError   bool   `json:"has_error"`
	IsMissing  bool   `json:"is_missing"`
	StartByte  uint32 `json:"start_byte"`
	EndByte    uint32 `json:"end_byte"`
	StartPoint Point  `json:"start_point"`
	EndPoint   Point  `json:"end_point"`
}

// CaptureDifference is the first difference CompareCaptures finds: the stream position, the
// field ("capture_count" when one stream is a prefix of the other) and both values.
type CaptureDifference struct {
	Index int    `json:"index"`
	Field string `json:"field"`
	Left  string `json:"left"`
	Right string `json:"right"`
}

// CompareCaptures is the single capture-stream comparator: every field of every capture in
// stream order, with no sorting, deduplication or span adjustment, so duplicates and ties
// keep their producer order. It returns nil when the streams are equal.
func CompareCaptures(a, b []Capture) *CaptureDifference {
	u := func(v uint32) string { return strconv.FormatUint(uint64(v), 10) }
	bl := strconv.FormatBool
	for i := range min(len(a), len(b)) {
		x, y := a[i], b[i]
		for _, f := range []struct{ name, l, r string }{
			{"match", u(x.Match), u(y.Match)},
			{"pattern", u(x.Pattern), u(y.Pattern)},
			{"capture", u(x.Capture), u(y.Capture)},
			{"name", strconv.Quote(x.Name), strconv.Quote(y.Name)},
			{"node", strconv.FormatInt(x.Node, 10), strconv.FormatInt(y.Node, 10)},
			{"type", strconv.Quote(x.Type), strconv.Quote(y.Type)},
			{"named", bl(x.Named), bl(y.Named)},
			{"extra", bl(x.Extra), bl(y.Extra)},
			{"is_error", bl(x.IsError), bl(y.IsError)},
			{"has_error", bl(x.HasError), bl(y.HasError)},
			{"is_missing", bl(x.IsMissing), bl(y.IsMissing)},
			{"start_byte", u(x.StartByte), u(y.StartByte)},
			{"end_byte", u(x.EndByte), u(y.EndByte)},
			{"start_point", pointText(x.StartPoint), pointText(y.StartPoint)},
			{"end_point", pointText(x.EndPoint), pointText(y.EndPoint)},
		} {
			if f.l != f.r {
				return &CaptureDifference{Index: i, Field: f.name, Left: f.l, Right: f.r}
			}
		}
	}
	if len(a) != len(b) {
		return &CaptureDifference{Index: min(len(a), len(b)), Field: "capture_count", Left: strconv.Itoa(len(a)), Right: strconv.Itoa(len(b))}
	}
	return nil
}
