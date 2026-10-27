package kit

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

const semanticRefusedCapture = `{"schema":"tsgk-native-process-capture/r1","stage":"VERIFY_REFUSED","spec":null,"process":null,"stdout":null,"stderr":null,"error":{"type":"native","code":"EXECUTABLE_MISMATCH","kind":"INVALID_INPUT","blocked":null}}`

func semanticReturnedCapture(t *testing.T) []byte {
	t.Helper()
	c := semanticProcessCapture{Schema: "tsgk-native-process-capture/r1", Stage: "RUN_RETURNED", Spec: &semanticCaptureSpec{Path: `C:\driver.exe`, Dir: `C:\work`, Args: []string{}, Env: []string{}, Wall: -1, Grace: 5000000000, StdoutBytes: -1}}
	c.Process = &ReplayMember{Path: "capture/c1-process.json", Role: "process", Bytes: 123, SHA256: strings.Repeat("a", 64)}
	c.Spec.Stdin = &ReplayMember{Path: "capture/c1-stdin.bin", Role: "stdin-frame", Bytes: 0, SHA256: digestHex(nil)}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSemanticCaptureDecode(t *testing.T) {
	refused, e := semanticTestBudget(t).processCapture("capture", []byte(semanticRefusedCapture))
	if e != nil || refused.Spec != nil || refused.Process != nil || refused.Error == nil || refused.Error.Code != "EXECUTABLE_MISMATCH" {
		t.Fatalf("refused %+v %v", refused, e)
	}
	data := semanticReturnedCapture(t)
	returned, e := semanticTestBudget(t).processCapture("capture", data)
	if e != nil || returned.Spec.Args == nil || returned.Spec.Env == nil || returned.Spec.Wall != -1 || returned.Spec.Grace != 5000000000 || returned.Spec.StdoutBytes != -1 || returned.Spec.Stdin == nil {
		t.Fatalf("returned %+v %v", returned, e)
	}
	null := strings.ReplaceAll(strings.ReplaceAll(string(data), `"Args":[]`, `"Args":null`), `"Env":[]`, `"Env":null`)
	returned, e = semanticTestBudget(t).processCapture("capture", []byte(null))
	if e != nil || returned.Spec.Args != nil || returned.Spec.Env != nil {
		t.Fatalf("nil/empty collapsed %+v %v", returned, e)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	for key := range top {
		copy := make(map[string]json.RawMessage, len(top))
		for k, v := range top {
			copy[k] = v
		}
		delete(copy, key)
		mutation, _ := json.Marshal(copy)
		if _, e := semanticTestBudget(t).processCapture("capture", mutation); e == nil {
			t.Fatalf("missing field %s accepted", key)
		}
	}
	for _, mutation := range []struct{ data, old, next string }{
		{semanticRefusedCapture, `"stage":"VERIFY_REFUSED"`, `"stage":"RUN_RETURNED"`},
		{semanticRefusedCapture, `"type":"native"`, `"type":"runner"`},
		{semanticRefusedCapture, `"blocked":null`, `"blocked":true`},
		{semanticRefusedCapture, `"error":{"type":"native","code":"EXECUTABLE_MISMATCH","kind":"INVALID_INPUT","blocked":null}`, `"error":null`},
		{string(data), `"process":{"path":"capture/c1-process.json"`, `"process":{"path":"../c1-process.json"`},
		{string(data), `"role":"process"`, `"role":"stdout-frame"`},
		{string(data), `"Args":[]`, `"Args":[null]`},
		{string(data), `"Wall":-1`, `"Wall":-1.5`},
		{string(data), `"Wall":-1`, `"Wall":9223372036854775808`},
		{string(data), `"Interactive":false`, `"Interactive":null`},
		{string(data), `"error":null`, `"error":{"type":"native","code":"EXECUTABLE_MISMATCH","kind":"INVALID_INPUT","blocked":null}`},
	} {
		changed := strings.Replace(mutation.data, mutation.old, mutation.next, 1)
		if changed == mutation.data {
			t.Fatalf("mutation did not change bytes: %s", mutation.old)
		}
		if _, e := semanticTestBudget(t).processCapture("capture", []byte(changed)); e == nil {
			t.Fatalf("capture mutation accepted %s", mutation.next)
		}
	}
}

func TestSemanticFrames(t *testing.T) {
	payload := []byte(semanticRequestFixture)
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame, uint32(len(payload)))
	copy(frame[4:], payload)
	actual, e := semanticTestBudget(t).semanticFrame("frame", frame)
	if e != nil || &actual[0] != &frame[4] || string(actual) != semanticRequestFixture {
		t.Fatalf("frame copied or changed %v", e)
	}
	if _, e := semanticTestBudget(t).nativeRequestFrame("frame", frame); e == nil || e.Code != "SEMANTICS_REQUEST_REVISION" {
		t.Fatalf("non-observing request qualified %v", e)
	}
	for _, invalid := range [][]byte{nil, {0}, {0, 0, 0}, frame[:len(frame)-1], append(append([]byte{}, frame...), 0), {0xff, 0xff, 0xff, 0xff}} {
		if _, e := semanticTestBudget(t).semanticFrame("frame", invalid); e == nil || e.Code != "SEMANTICS_FRAME_LENGTH" {
			t.Fatalf("bad frame accepted length%d error %v", len(invalid), e)
		}
	}
	binary.BigEndian.PutUint32(frame, 0)
	if _, e := semanticTestBudget(t).semanticFrame("frame", frame); e == nil || e.Code != "SEMANTICS_FRAME_LENGTH" {
		t.Fatalf("header mutation %v", e)
	}
	limit := int(ReplayOperations()["evidence-replay"].FileBytes)
	boundary := make([]byte, limit+1)
	binary.BigEndian.PutUint32(boundary, uint32(limit-4))
	if _, e := semanticTestBudget(t).semanticFrame("boundary", boundary[:limit]); e != nil {
		t.Fatalf("maximum file frame rejected: %v", e)
	}
	binary.BigEndian.PutUint32(boundary, uint32(limit-3))
	if _, e := semanticTestBudget(t).semanticFrame("boundary", boundary); e == nil || e.Kind != KindResourceLimit {
		t.Fatalf("over-limit file frame accepted: %v", e)
	}
}
