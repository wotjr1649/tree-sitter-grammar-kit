package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	// ReuseAcceptedSchema proves accepted events and fresh CST equivalence, never node identity.
	ReuseAcceptedSchema = "tsgk-reuse-accepted/r1"
	// ReuseAcceptedLedgerSHA256 binds the original order, histories, targets and omit-old steps.
	ReuseAcceptedLedgerSHA256 = "23b1c07240604df9bf92d1336034ed03178236771cfdbe3e7da92083f46e36fc"
)

// ReuseAcceptedLedger is a validated, immutable copy of the fixed fifteen original histories.
// Its identity is semantic; callers retain and hash the original document bytes separately.
type ReuseAcceptedLedger struct{ originals []reuseAcceptedOriginal }

// ParseReuseAcceptedLedger rejects missing fields and drift from the independently
// registered Go history serialization, target steps and original order.
func ParseReuseAcceptedLedger(data []byte) (*ReuseAcceptedLedger, error) {
	t := typed{doc: "reuse-accepted-ledger"}
	v, err := decodeStrict(t.doc, data, MaxDocumentBytes)
	if err != nil {
		return nil, err
	}
	top, err := t.object(v, []string{"schema", "holdouts"})
	if err != nil {
		return nil, err
	}
	schema, err := t.str(top["schema"])
	if err != nil {
		return nil, err
	}
	if schema != "tsgk-reuse-holdouts/r1" {
		return nil, t.bad("SCHEMA_UNSUPPORTED", top["schema"])
	}
	rows, err := t.array(top["holdouts"])
	if err != nil {
		return nil, err
	}
	if len(rows) != len(reuseAcceptedPins) {
		return nil, t.bad("REUSE_ACCEPTED_LEDGER_MISMATCH", top["holdouts"])
	}
	for _, row := range rows {
		m, e := t.object(row, []string{"id", "route", "origin", "source_utf8", "edits", "blocked_steps", "omit_old_tree_first_step"}, "sibling")
		if e != nil {
			return nil, e
		}
		for _, k := range []string{"id", "route", "sibling", "source_utf8"} {
			if k == "sibling" && m[k] == nil {
				continue
			}
			if _, e := t.str(m[k]); e != nil {
				return nil, e
			}
		}
		origin, e := t.object(m["origin"], []string{"issue"}, "file", "revision", "artifact", "artifact_sha256", "pointer", "case_id", "input_file", "input_file_sha256", "original_sha256", "repaired_sha256")
		if e != nil {
			return nil, e
		}
		for k, x := range origin {
			if k == "issue" {
				_, e = t.uint(x)
			} else {
				_, e = t.str(x)
			}
			if e != nil {
				return nil, e
			}
		}
		edits, e := t.array(m["edits"])
		if e != nil {
			return nil, e
		}
		for _, edit := range edits {
			em, e := t.object(edit, []string{"start_byte", "old_end_byte", "new_end_byte", "old_utf8", "new_utf8"})
			if e != nil {
				return nil, e
			}
			for k, x := range em {
				if strings.HasSuffix(k, "_byte") {
					_, e = t.uint(x)
				} else {
					_, e = t.str(x)
				}
				if e != nil {
					return nil, e
				}
			}
		}
		targets, e := t.array(m["blocked_steps"])
		if e != nil {
			return nil, e
		}
		for _, x := range append(targets, m["omit_old_tree_first_step"]) {
			if _, e := t.uint(x); e != nil {
				return nil, e
			}
		}
	}
	var doc struct {
		Schema   string                  `json:"schema"`
		Holdouts []reuseAcceptedOriginal `json:"holdouts"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	ledger := "tsgk-reuse-accepted-ledger/r1\n"
	for i, h := range doc.Holdouts {
		pin := reuseAcceptedPins[i]
		// This field order and encoding/json byte-slice encoding preserve the legacy pins.
		identity, err := json.Marshal(struct {
			Route, Source, Sibling string
			Origin                 any
			Edits                  []Edit
		}{h.Route, h.SourceUTF8, h.Sibling, h.Origin, h.edits()})
		if err != nil {
			return nil, err
		}
		if h.ID != pin.id || digestHex(identity) != pin.legacy || !slices.Equal(h.BlockedSteps, pin.targets) || h.OmitOldFirst != pin.omit {
			return nil, fmt.Errorf("REUSE_ACCEPTED_LEDGER_MISMATCH: original %d", i)
		}
		if _, _, err := ApplyEdits(EncodingUTF8, []byte(h.SourceUTF8), h.edits(), NativeOperations()["native-parse-edit"].InputBytes); err != nil {
			return nil, err
		}
		ledger += h.ID + "\t" + h.historySHA256() + "\n"
	}
	if digestHex([]byte(ledger)) != ReuseAcceptedLedgerSHA256 {
		return nil, fmt.Errorf("REUSE_ACCEPTED_LEDGER_MISMATCH: semantic digest")
	}
	return &ReuseAcceptedLedger{originals: doc.Holdouts}, nil
}

func (h reuseAcceptedOriginal) edits() []Edit {
	edits := make([]Edit, 0, len(h.Edits))
	for _, e := range h.Edits {
		edits = append(edits, Edit{StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte, Old: []byte(e.Old), New: []byte(e.New)})
	}
	return edits
}

func (h reuseAcceptedOriginal) historySHA256() string {
	targets := make([]string, len(h.BlockedSteps))
	for i, k := range h.BlockedSteps {
		targets[i] = strconv.Itoa(k)
	}
	i := slices.IndexFunc(reuseAcceptedPins, func(p struct {
		id, legacy string
		targets    []int
		omit       int
	}) bool {
		return p.id == h.ID
	})
	if i < 0 {
		return ""
	}
	return digestHex([]byte("tsgk-reuse-accepted-history/r1\n" + h.ID + "\n" + reuseAcceptedPins[i].legacy + "\n" + strings.Join(targets, ",") + "\n" + strconv.Itoa(h.OmitOldFirst) + "\n"))
}

func (l *ReuseAcceptedLedger) original(id string) *reuseAcceptedOriginal {
	if l != nil {
		for i := range l.originals {
			if l.originals[i].ID == id {
				return &l.originals[i]
			}
		}
	}
	return nil
}

// CheckCase binds the entire registered history and the witnessed original bytes.
// Profile declarations, operation and process mode are checked by the enclosing host.
func (l *ReuseAcceptedLedger) CheckCase(route string, c IncrementalCase, source []byte) error {
	h := l.original(c.ID)
	if h == nil || h.Route != route || c.Encoding != EncodingUTF8 || !bytes.Equal(source, []byte(h.SourceUTF8)) || len(c.SvcSource) != 0 || len(c.Points) != 0 || len(c.Expect) != 0 {
		return fmt.Errorf("REUSE_ACCEPTED_HISTORY_MISMATCH")
	}
	edits := h.edits()
	if len(edits) != len(c.Edits) {
		return fmt.Errorf("REUSE_ACCEPTED_HISTORY_MISMATCH")
	}
	for i, a := range edits {
		b := c.Edits[i]
		if a.StartByte != b.StartByte || a.OldEndByte != b.OldEndByte || a.NewEndByte != b.NewEndByte || !bytes.Equal(a.Old, b.Old) || !bytes.Equal(a.New, b.New) {
			return fmt.Errorf("REUSE_ACCEPTED_HISTORY_MISMATCH")
		}
	}
	if c.Input.SHA256 != digestHex(source) || c.Input.Bytes != uint64(len(source)) {
		return fmt.Errorf("REUSE_ACCEPTED_SOURCE_MISMATCH")
	}
	return nil
}

// CheckProfile rejects unregistered or reordered histories before any native build or output.
// A single-process profile may select a subset; qualification keeps the fixed full ledger.
func (l *ReuseAcceptedLedger) CheckProfile(p IncrementalProfile) error {
	previous := -1
	if len(p.Cases) == 0 {
		return fmt.Errorf("REUSE_ACCEPTED_HISTORY_MISMATCH")
	}
	for _, c := range p.Cases {
		h := l.original(c.ID)
		if h == nil {
			return fmt.Errorf("REUSE_ACCEPTED_HISTORY_MISMATCH")
		}
		i := slices.IndexFunc(l.originals, func(h reuseAcceptedOriginal) bool { return h.ID == c.ID })
		if i <= previous {
			return fmt.Errorf("REUSE_ACCEPTED_HISTORY_MISMATCH")
		}
		previous = i
		if err := l.CheckCase(p.Route, c, []byte(h.SourceUTF8)); err != nil {
			return err
		}
	}
	return nil
}

type reuseAcceptedOriginal struct {
	ID     string `json:"id"`
	Route  string `json:"route"`
	Origin struct {
		Issue           int    `json:"issue"`
		File            string `json:"file"`
		Revision        string `json:"revision"`
		Artifact        string `json:"artifact"`
		ArtifactSHA256  string `json:"artifact_sha256"`
		Pointer         string `json:"pointer"`
		CaseID          string `json:"case_id"`
		InputFile       string `json:"input_file"`
		InputFileSHA256 string `json:"input_file_sha256"`
		OriginalSHA256  string `json:"original_sha256"`
		RepairedSHA256  string `json:"repaired_sha256"`
	} `json:"origin"`
	Sibling    string `json:"sibling"`
	SourceUTF8 string `json:"source_utf8"`
	Edits      []struct {
		StartByte  uint32 `json:"start_byte"`
		OldEndByte uint32 `json:"old_end_byte"`
		NewEndByte uint32 `json:"new_end_byte"`
		Old        string `json:"old_utf8"`
		New        string `json:"new_utf8"`
	} `json:"edits"`
	BlockedSteps []int `json:"blocked_steps"`
	OmitOldFirst int   `json:"omit_old_tree_first_step"`
}

var reuseAcceptedPins = []struct {
	id, legacy string
	targets    []int
	omit       int
}{
	{"tsql-issue137-builtin-default-r1", "2ad13ee607cee027a37d5befd32cd3b0631e264e3cb457c283ee7b0dddf887be", []int{1, 2}, 1},
	{"tsql-issue137-dynamic-payload-r1", "fbf8c284edcaaca71184ceb75136287df918fe65b8c1537b4f57292eacdfea82", []int{1, 2}, 1},
	{"tsql-issue139-procedure-r1", "cd04860981964a5c095942af88687380aa5da96b6654207b2598a1d9f94c4b87", []int{1, 2}, 1},
	{"tsql-issue139-function-r1", "c2aafc7ed41835a32261e26c0a952704c1f41dd1c9827621e325da94c81b648c", []int{1, 2}, 1},
	{"tsql-issue139-output-r1", "5ceeaef45094a32ed425a7bba94a476f4b68aba5699d61b5b2925d88f23cb31e", []int{1, 2}, 1},
	{"tsql-issue139-clr-r1", "a6c3ceb770428d44dc9c1591b73c262a1eb8f39b24422d3013241ad4367a8643", []int{1, 2}, 1},
	{"tsql-issue138-leading-01473-r1", "4dd79a54f5e7989f630cf7ed0892451bb100746c0466a2772c105c804a5da286", []int{1, 2}, 1},
	{"tsql-issue138-leading-02133-r1", "9413f3492cfb52f91cd2389016f9d41ae9731c40cee21e9133f99242e6f27acc", []int{1, 2}, 1},
	{"tsql-issue138-leading-02657-r1", "10df1052447ed0bc97f27ade1d96edbe4226a7ce0b23755a96f976e4a779f9a7", []int{1, 2}, 1},
	{"tsql-issue148-negative-pair-00-r1", "09bc82f500d6564fb4832a1a274397f477f7c47c21ceb882f931bc8a1d9f1aed", []int{2}, 1},
	{"tsql-issue148-negative-pair-01-r1", "99e269d48f44a5cc9688f517d402caf51f2710686a4ae6937ac4fcf3ac862c79", []int{2}, 1},
	{"tsql-issue198-unclosed-bracket", "16a81ed40e4047c134c45b30918eae4c31a1cf8e3076af7a8a0a28eadd8e5d3b", []int{1, 2}, 1},
	{"json-repair-close", "fa64cce8c757b87ed579cb4b851f4d6e70d808f5fc066b27cbfb545e4d39caa8", []int{1}, 1},
	{"json-break-close", "97674f16d0dcc62788bd91b76f24aa4458d4e1c27062c9e3b4201d7758bdb909", []int{1}, 1},
	{"json-repair-colon", "5d7ca0aaa6d8b6458b9cc78bbb905d863adc8dd3eb3477c0ae3fa7fd30a36e39", []int{1}, 1},
}
