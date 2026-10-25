// Package reuseobs validates the bounded public-logger observation shared by native
// execution and offline replay. Events are not unique nodes or performance evidence.
package reuseobs

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"strings"
)

const (
	Schema                = "tsgk-reuse-observation/r1"
	Policy                = "tsgk-reuse-log/r1"
	RuntimeManifestSHA256 = "1b962a519e6aece2140eb1c56589cb5ca88b501612921a0ce8dd4acd1a63d541"
	MaxEvents             = 16384
	MaxBytes              = 1048576
	MaxMessageBytes       = 1024
	MaxRecordBytes        = 4096
)

// VerifiedRuntimeManifest reports whether recorded observations bound to this runtime
// manifest are read under this policy: the current RuntimeManifestSHA256 (v0.27.1), which
// live runs must declare, and 659cda7's, whose parser log formats were checked identical (#213).
func VerifiedRuntimeManifest(sha string) bool {
	return sha == RuntimeManifestSHA256 || sha == "affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080"
}

// Counts contains only recognized parse events, never logger text or node addresses.
type Counts struct {
	NewParse        uint64 `json:"new_parse"`
	ParseAfterEdit  uint64 `json:"parse_after_edit"`
	ResumeParse     uint64 `json:"resume_parse"`
	Done            uint64 `json:"done"`
	Accepted        uint64 `json:"accepted"`
	Changed         uint64 `json:"changed"`
	Error           uint64 `json:"error"`
	Missing         uint64 `json:"missing"`
	Fragile         uint64 `json:"fragile"`
	IncludedRange   uint64 `json:"included_range"`
	ExternalScanner uint64 `json:"external_scanner"`
	Lookahead       uint64 `json:"lookahead"`
}

// Observation's status distinguishes completed zero from missing or partial data.
// OVERFLOW has null counts; other noncomplete statuses retain only the observed prefix.
type Observation struct {
	Schema                string  `json:"schema"`
	Policy                string  `json:"policy"`
	RuntimeManifestSHA256 string  `json:"runtime_manifest_sha256"`
	RequestID             string  `json:"request_id"`
	RunID                 string  `json:"run_id"`
	ProducerSHA256        string  `json:"producer_sha256"`
	HistoryStart          uint64  `json:"history_start"`
	Step                  uint64  `json:"step"`
	Role                  string  `json:"role"`
	SourceSHA256          string  `json:"source_sha256"`
	SourceBytes           uint64  `json:"source_bytes"`
	OldTree               bool    `json:"old_tree"`
	OldSourceSHA256       *string `json:"old_source_sha256"`
	Status                string  `json:"status"`
	Reason                string  `json:"reason"`
	Events                uint64  `json:"events"`
	Bytes                 uint64  `json:"bytes"`
	Counts                *Counts `json:"counts"`
}

// Binding comes from the request or its replayed, independently checked step.
// PreviousSourceSHA256 binds an actually supplied old tree; it does not assert use.
type Binding struct {
	HistoryStart                                                                                      uint64
	RuntimeManifestSHA256, RequestID, RunID, ProducerSHA256, Role, SourceSHA256, PreviousSourceSHA256 string
	Step, SourceBytes                                                                                 uint64
}

func hash(s string) bool { return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == "" }

func object(raw []byte, fields ...string) (map[string]jsontext.Value, error) {
	var values map[string]jsontext.Value
	if err := jsonv2.Unmarshal(raw, &values); err != nil || values == nil {
		return nil, errors.New("observation object")
	}
	if len(values) != len(fields) {
		return nil, errors.New("observation members")
	}
	for _, field := range fields {
		if _, ok := values[field]; !ok {
			return nil, errors.New("missing observation member")
		}
	}
	return values, nil
}

func unsigned(v jsontext.Value) bool {
	if len(v) == 0 {
		return false
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Decode rejects omitted zero-valued fields, unknown/duplicate members, noncanonical
// integers and provenance drift before any status or count can support a claim.
func Decode(raw []byte, b Binding) (Observation, error) {
	o, err := Parse(raw)
	if err != nil {
		return o, err
	}
	return validate(o, b)
}

// Parse checks the record encoding without accepting its binding or claim.
func Parse(raw []byte) (Observation, error) {
	var o Observation
	if len(raw) > MaxRecordBytes {
		return o, errors.New("observation bytes limit")
	}
	values, err := object(raw, "schema", "policy", "runtime_manifest_sha256", "request_id", "run_id", "producer_sha256",
		"history_start", "step", "role", "source_sha256", "source_bytes", "old_tree", "old_source_sha256",
		"status", "reason", "events", "bytes", "counts")
	if err != nil {
		return o, err
	}
	for _, field := range []string{"history_start", "step", "source_bytes", "events", "bytes"} {
		if !unsigned(values[field]) {
			return o, errors.New("observation unsigned integer")
		}
	}
	for _, field := range []string{"schema", "policy", "runtime_manifest_sha256", "request_id", "run_id", "producer_sha256", "role", "source_sha256", "status", "reason"} {
		if len(values[field]) == 0 || values[field][0] != '"' {
			return o, errors.New("observation string member")
		}
	}
	if string(values["old_tree"]) != "true" && string(values["old_tree"]) != "false" {
		return o, errors.New("observation old-tree flag")
	}
	if string(values["counts"]) != "null" {
		counts, err := object(values["counts"], "new_parse", "parse_after_edit", "resume_parse",
			"done", "accepted", "changed", "error", "missing", "fragile", "included_range",
			"external_scanner", "lookahead")
		if err != nil {
			return o, err
		}
		for _, value := range counts {
			if !unsigned(value) {
				return o, errors.New("observation counter integer")
			}
		}
	}
	if err := jsonv2.Unmarshal(raw, &o, jsonv2.RejectUnknownMembers(true)); err != nil {
		return o, errors.New("observation encoding")
	}
	return o, nil
}

func validate(o Observation, b Binding) (Observation, error) {
	if o.Schema != Schema || o.Policy != Policy ||
		!VerifiedRuntimeManifest(b.RuntimeManifestSHA256) || o.RuntimeManifestSHA256 != b.RuntimeManifestSHA256 ||
		!hash(b.SourceSHA256) || o.SourceSHA256 != b.SourceSHA256 ||
		o.SourceBytes != b.SourceBytes || len(b.RequestID) == 0 || len(b.RequestID) > 128 ||
		len(b.RunID) == 0 || len(b.RunID) > 128 || o.RunID != b.RunID ||
		!hash(b.ProducerSHA256) || o.ProducerSHA256 != b.ProducerSHA256 ||
		o.HistoryStart != b.HistoryStart || b.HistoryStart > 4 || o.Step > 4-b.HistoryStart ||
		o.RequestID != b.RequestID || o.Step != b.Step ||
		o.Role != b.Role || (o.Role != "initial" && o.Role != "incremental" && o.Role != "fresh") {
		return o, errors.New("observation binding")
	}
	if o.OldTree {
		if o.Role == "initial" {
			return o, errors.New("observation initial old tree")
		}
		if o.OldSourceSHA256 == nil || !hash(b.PreviousSourceSHA256) ||
			*o.OldSourceSHA256 != b.PreviousSourceSHA256 {
			return o, errors.New("observation old-source binding")
		}
	} else if o.OldSourceSHA256 != nil {
		return o, errors.New("observation unexpected old source")
	}
	if o.Events > MaxEvents || o.Bytes > MaxBytes || o.Bytes > o.Events*(MaxMessageBytes-1) {
		return o, errors.New("observation collection limit")
	}
	if o.Counts != nil {
		c := o.Counts
		if !o.OldTree && c.ParseAfterEdit != 0 {
			return o, errors.New("observation branch without old tree")
		}
		total := uint64(0)
		for _, count := range []uint64{c.NewParse, c.ParseAfterEdit, c.ResumeParse, c.Done, c.Accepted,
			c.Changed, c.Error, c.Missing, c.Fragile, c.IncludedRange, c.ExternalScanner, c.Lookahead} {
			if count > o.Events-total {
				return o, errors.New("observation counter total")
			}
			total += count
		}
	}
	switch o.Status {
	case "COMPLETE":
		if o.Counts == nil || o.Reason != "" || o.Bytes == 0 || o.Counts.Done != 1 ||
			o.Counts.NewParse+o.Counts.ParseAfterEdit+o.Counts.ResumeParse != 1 {
			return o, errors.New("observation completeness")
		}
	case "UNAVAILABLE":
		if o.Counts == nil {
			return o, errors.New("observation missing prefix")
		}
		switch o.Reason {
		case "LOGGER_MISSING":
			if o.Events != 0 || o.Bytes != 0 {
				return o, errors.New("observation unavailable prefix")
			}
		case "PARSE_NOT_RUN", "NO_NATIVE_PARSE":
			if o.Events != 0 || o.Bytes != 0 || o.OldTree {
				return o, errors.New("observation unavailable prefix")
			}
		case "PARSE_INCOMPLETE", "FORMAT_UNKNOWN":
		default:
			return o, errors.New("observation unavailable reason")
		}
	case "TRUNCATED":
		if o.Counts == nil || (o.Reason != "EVENT_LIMIT" && o.Reason != "BYTE_LIMIT" && o.Reason != "MESSAGE_LIMIT") ||
			(o.Reason == "EVENT_LIMIT" && o.Events != MaxEvents) {
			return o, errors.New("observation truncation")
		}
		if o.Reason == "BYTE_LIMIT" && o.Bytes <= MaxBytes-(MaxMessageBytes-1) {
			return o, errors.New("observation byte-limit prefix")
		}
	case "OVERFLOW":
		if o.Counts != nil || o.Reason != "COUNTER_OVERFLOW" {
			return o, errors.New("observation overflow")
		}
	default:
		return o, errors.New("observation status")
	}
	return o, nil
}
