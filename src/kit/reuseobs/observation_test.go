package reuseobs

import (
	"encoding/json"
	"strings"
	"testing"
)

func sample() (Observation, Binding) {
	b := Binding{RuntimeManifestSHA256: RuntimeManifestSHA256, RequestID: "case",
		RunID: "run-1", ProducerSHA256: strings.Repeat("a", 64),
		Role: "initial", SourceSHA256: strings.Repeat("b", 64), SourceBytes: 3}
	return Observation{Schema: Schema, Policy: Policy, RuntimeManifestSHA256: b.RuntimeManifestSHA256,
		RequestID: b.RequestID, RunID: b.RunID, ProducerSHA256: b.ProducerSHA256,
		Role: b.Role, SourceSHA256: b.SourceSHA256, SourceBytes: b.SourceBytes,
		Status: "COMPLETE", Events: 2, Bytes: 30, Counts: &Counts{NewParse: 1, Done: 1}}, b
}

func encode(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestObservationStates(t *testing.T) {
	for _, state := range []string{"COMPLETE", "LOGGER_MISSING", "PARSE_NOT_RUN", "NO_NATIVE_PARSE",
		"PARSE_INCOMPLETE", "FORMAT_UNKNOWN", "EVENT_LIMIT", "BYTE_LIMIT", "MESSAGE_LIMIT", "COUNTER_OVERFLOW"} {
		t.Run(state, func(t *testing.T) {
			o, b := sample()
			switch state {
			case "COMPLETE": // a completed zero accepted-event count is meaningful
			case "LOGGER_MISSING", "PARSE_NOT_RUN", "NO_NATIVE_PARSE":
				o.Status, o.Reason, o.Events, o.Bytes, o.Counts = "UNAVAILABLE", state, 0, 0, &Counts{}
			case "PARSE_INCOMPLETE", "FORMAT_UNKNOWN":
				o.Status, o.Reason = "UNAVAILABLE", state
			case "EVENT_LIMIT", "BYTE_LIMIT", "MESSAGE_LIMIT":
				o.Status, o.Reason = "TRUNCATED", state
				if state == "EVENT_LIMIT" {
					o.Events = MaxEvents
				}
				if state == "BYTE_LIMIT" {
					o.Bytes = MaxBytes
					o.Events = (MaxBytes + MaxMessageBytes - 2) / (MaxMessageBytes - 1)
				}
			case "COUNTER_OVERFLOW":
				o.Status, o.Reason, o.Counts = "OVERFLOW", state, nil
			}
			got, err := Decode(encode(t, o), b)
			if err != nil || got.Status != o.Status || got.Reason != o.Reason {
				t.Fatalf("%s: %v", state, err)
			}
		})
	}
	o, b := sample()
	b.Role, b.Step, b.PreviousSourceSHA256 = "incremental", 1, strings.Repeat("c", 64)
	o.Role, o.Step, o.OldTree, o.OldSourceSHA256 = b.Role, b.Step, true, &b.PreviousSourceSHA256
	o.Counts.NewParse, o.Counts.ParseAfterEdit = 0, 1
	if _, err := Decode(encode(t, o), b); err != nil {
		t.Fatal(err)
	}
	o.Status, o.Reason, o.Events, o.Bytes, o.Counts = "UNAVAILABLE", "LOGGER_MISSING", 0, 0, &Counts{}
	if _, err := Decode(encode(t, o), b); err != nil {
		t.Fatal("missing logger erased actual old-tree argument:", err)
	}
}

func TestObservationRejectsAmbiguousOrUnboundData(t *testing.T) {
	base, b := sample()
	raw := string(encode(t, base))
	for name, bad := range map[string]string{
		"null-object":             "null",
		"array-object":            "[]",
		"missing-events":          strings.Replace(raw, `"events":2,`, "", 1),
		"missing-run":             strings.Replace(raw, `"run_id":"run-1",`, "", 1),
		"missing-history":         strings.Replace(raw, `"history_start":0,`, "", 1),
		"history-drift":           strings.Replace(raw, `"history_start":0`, `"history_start":2`, 1),
		"history-negative-zero":   strings.Replace(raw, `"history_start":0`, `"history_start":-0`, 1),
		"run-drift":               strings.Replace(raw, `"run_id":"run-1"`, `"run_id":"run-2"`, 1),
		"producer-drift":          strings.Replace(raw, b.ProducerSHA256, strings.Repeat("e", 64), 1),
		"empty-complete-bytes":    strings.Replace(raw, `"bytes":30`, `"bytes":0`, 1),
		"old-branch-without-tree": strings.Replace(raw, `"new_parse":1,"parse_after_edit":0`, `"new_parse":0,"parse_after_edit":1`, 1),
		"missing-zero-counter":    strings.Replace(raw, `"accepted":0,`, "", 1),
		"unknown-member":          strings.Replace(raw, `"schema":`, `"extra":0,"schema":`, 1),
		"duplicate":               strings.Replace(raw, `"events":2`, `"events":2,"events":2`, 1),
		"negative-zero":           strings.Replace(raw, `"accepted":0`, `"accepted":-0`, 1),
		"exponent":                strings.Replace(raw, `"events":2`, `"events":2e0`, 1),
		"overflow-integer":        strings.Replace(raw, `"accepted":0`, `"accepted":18446744073709551616`, 1),
		"null-bool":               strings.Replace(raw, `"old_tree":false`, `"old_tree":null`, 1),
		"null-empty-reason":       strings.Replace(raw, `"reason":""`, `"reason":null`, 1),
		"impossible-byte-prefix":  strings.Replace(strings.Replace(raw, `"status":"COMPLETE"`, `"status":"TRUNCATED"`, 1), `"reason":""`, `"reason":"BYTE_LIMIT"`, 1),
		"bytes-without-events":    strings.Replace(raw, `"events":2`, `"events":0`, 1),
		"null-complete-counts":    strings.Replace(raw, string(encode(t, base.Counts)), "null", 1),
		"unfinished":              strings.Replace(raw, `"done":1`, `"done":0`, 1),
		"missing-branch":          strings.Replace(raw, `"new_parse":1`, `"new_parse":0`, 1),
		"counter-total":           strings.Replace(raw, `"accepted":0`, `"accepted":3`, 1),
		"event-cap":               strings.Replace(raw, `"events":2`, `"events":16385`, 1),
		"byte-cap":                strings.Replace(raw, `"bytes":30`, `"bytes":1048577`, 1),
		"role":                    strings.Replace(raw, `"role":"initial"`, `"role":"fresh"`, 1),
		"runtime":                 strings.Replace(raw, b.RuntimeManifestSHA256, strings.Repeat("d", 64), 1),
		"source":                  strings.Replace(raw, b.SourceSHA256, strings.Repeat("d", 64), 1),
		"old-tree-without-source": strings.Replace(raw, `"old_tree":false`, `"old_tree":true`, 1),
		"status":                  strings.Replace(raw, "COMPLETE", "MAYBE", 1),
		"schema":                  strings.Replace(raw, Schema, "tsgk-reuse-observation/r9", 1),
		"invalid-utf8":            strings.Replace(raw, `"case"`, string([]byte{34, 255, 34}), 1),
		"oversize":                strings.Repeat(" ", MaxRecordBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if bad == raw {
				t.Fatal("negative control did not mutate input")
			}
			if _, err := Decode([]byte(bad), b); err == nil {
				t.Fatal("accepted ambiguous or unbound observation")
			}
		})
	}
}
