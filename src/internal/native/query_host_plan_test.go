package native

import "testing"

// The two-host query tests skip on an unregistered host, and the fan-in runs on Linux only while the
// opt-in collection asks for it (#362 F2, F7). No native tool is needed.
func TestQueryHostPlan(t *testing.T) {
	for _, c := range []struct {
		platform              string
		collecting, collectOn bool
		other                 string
		skip                  bool
	}{
		{"windows-amd64", false, true, "linux-amd64", false},
		{"windows-amd64", true, true, "linux-amd64", false},
		{"linux-amd64", false, true, "", true}, // required sanitizer and foundation runs: no fan-in
		{"linux-amd64", true, true, "windows-amd64", false},
		{"linux-amd64", false, false, "windows-amd64", false}, // the predicate and inline tests keep running
		{"darwin-arm64", true, true, "", true},
		{"darwin-arm64", false, false, "", true},
		{"windows-arm64", true, false, "", true},
	} {
		other, skip := queryHostPlan(c.platform, c.collecting, c.collectOn)
		if other != c.other || (skip != "") != c.skip {
			t.Errorf("%s collecting=%v collectOnly=%v: other %q skip %q", c.platform, c.collecting, c.collectOn, other, skip)
		}
	}
}
