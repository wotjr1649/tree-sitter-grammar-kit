package kit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestOrdinaryRequiredIDKey(t *testing.T) {
	valid := "qsem1:" + strings.Repeat("0123456789abcdef", 4)
	key, ok := ordinaryRequiredIDKey(valid)
	if !ok || string(key[:]) != valid[6:] {
		t.Fatal("valid ID did not preserve its exact suffix")
	}
	for i := range len(valid) {
		for c := 0; c < 256; c++ {
			changed := []byte(valid)
			changed[i] = byte(c)
			want := strings.HasPrefix(string(changed), "qsem1:") && hex64.MatchString(string(changed[6:]))
			_, got := ordinaryRequiredIDKey(string(changed))
			if got != want {
				t.Fatalf("byte %d value %d: valid=%t want=%t", i, c, got, want)
			}
		}
	}
	for _, bad := range []string{"", valid[:len(valid)-1], valid + "0"} {
		if _, ok := ordinaryRequiredIDKey(bad); ok {
			t.Fatalf("accepted length %d", len(bad))
		}
	}
}

func TestOrdinaryRequiredIDUnselectedFormatAndPrecedence(t *testing.T) {
	registration, selected := ordinaryRegistrationOnceFixture()
	valid := ordinaryJSON(t, registration)
	for _, id := range []string{"qsem1:" + strings.Repeat("g", 64), "Qsem1:" + strings.Repeat("a", 64), "qsem1:" + strings.Repeat("A", 64), "qsem1:" + strings.Repeat("a", 63), "qsem1:" + strings.Repeat("a", 65)} {
		data := ordinaryRegistrationOnceSetRowField(t, valid, "required", 2, "obligation_id", json.RawMessage("\""+id+"\""))
		if _, e := compareOrdinaryRegistrationOnce(t, data, selected); e == nil || e.Code != "ORDINARY_REQUIRED_COORDINATE" {
			t.Fatalf("unselected malformed ID accepted: %v", e)
		}
		syntax := append(bytes.Clone(data), []byte(" true")...)
		if _, e := compareOrdinaryRegistrationOnce(t, syntax, selected); e == nil || e.Code != "ORDINARY_JSON" {
			t.Fatalf("late syntax priority changed: %v", e)
		}
	}
}
