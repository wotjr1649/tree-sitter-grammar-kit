// Package scopeobligations is the artifact-layer companion of a host's SVC scope results (#362).
//
// The stored tsgk-query-scope-host/r1 results count obligations per kind and host; they do not say which
// obligation was judged how. A merge that compares those counts cannot see an exchange that keeps them
// (an offline FAILED turned VERIFIED while another obligation turns FAILED). The companion file keeps
// the judgment of every SVC obligation, by ID, for the host's offline run and for its inline run. The
// merge recomputes both stored results from it (the stored format stays r1) and checks per obligation
// that the inline run changed nothing but offline BLOCKED inline obligations.
package scopeobligations

import (
	jsonv2 "encoding/json/v2"
	"slices"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const (
	Schema = "tsgk-query-scope-svc-obligations/r1"

	// CodeShape: the file is not a well formed companion of this denominator. CodeInline: the inline
	// judgments change something the offline run decided.
	CodeShape  = "QUERY_SCOPE_SVC_OBLIGATIONS"
	CodeInline = "QUERY_SCOPE_SVC_INLINE_MISMATCH"

	maxBytes = 4 << 20
)

// FileName is the artifact file of a host's companion.
func FileName(host string) string { return "query-scope-svc-obligations-" + host + ".json" }

// Judgment is the stored form of a kit.SvcJudgment.
type Judgment struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Code  string `json:"code,omitempty"`
}

// Document holds every SVC obligation's judgment, in obligation order, twice: Offline is the judgment
// before the real C# grammar ran, Inline the final judgment after it.
type Document struct {
	Schema  string     `json:"schema"`
	Host    string     `json:"host"`
	Offline []Judgment `json:"offline"`
	Inline  []Judgment `json:"inline"`
}

func stored(js []kit.SvcJudgment) []Judgment {
	out := make([]Judgment, 0, len(js))
	for _, j := range js {
		out = append(out, Judgment{j.ID, j.State, j.Code})
	}
	return out
}

func judgments(js []Judgment) []kit.SvcJudgment {
	out := make([]kit.SvcJudgment, 0, len(js))
	for _, j := range js {
		out = append(out, kit.SvcJudgment{ID: j.ID, State: j.State, Code: j.Code})
	}
	return out
}

// New is the companion of a host's offline judgments and the final judgments after the inline run.
func New(host string, offline, inline []kit.SvcJudgment) Document {
	return Document{Schema: Schema, Host: host, Offline: stored(offline), Inline: stored(inline)}
}

func Encode(d Document) ([]byte, error) { return jsonv2.Marshal(d) }

// Decode strictly decodes a companion: exact member names, no unknown or repeated member, no trailing
// data.
func Decode(data []byte) (Document, error) {
	var d Document
	if len(data) > maxBytes || jsonv2.Unmarshal(data, &d, jsonv2.RejectUnknownMembers(true)) != nil || d.Schema != Schema {
		return Document{}, errShape
	}
	return d, nil
}

type fault string

func (f fault) Error() string { return string(f) }

const errShape = fault(CodeShape)

// Offline and Inline return the judgments in the form the scope folds take.
func (d Document) OfflineJudgments() []kit.SvcJudgment { return judgments(d.Offline) }
func (d Document) InlineJudgments() []kit.SvcJudgment  { return judgments(d.Inline) }

// Check returns "" when both judgment lists are exactly the obligations of e, in order, with known
// states, and the inline list refines the offline one per obligation: an obligation the offline run
// judged VERIFIED or FAILED keeps that judgment and code, and only the inline obligations (SVC_INLINE_TREE,
// SVC_INLINE_QUERY) the offline run left BLOCKED may change. inline is the committed inline expected asset's
// InlineCodes: the offline run has no C# grammar, so it must leave every inline obligation BLOCKED with its
// registered code, and the inline run may judge (VERIFIED or FAILED) only the obligations the asset derives;
// every other inline obligation stays BLOCKED. Otherwise it returns CodeShape or CodeInline.
func (d Document) Check(e kit.SvcExpected, inline map[string]string) string {
	obligations := e.Obligations()
	if len(d.Offline) != len(obligations) || len(d.Inline) != len(obligations) {
		return CodeShape
	}
	known := []string{"VERIFIED", "FAILED", "BLOCKED"}
	for i, o := range obligations {
		off, in := d.Offline[i], d.Inline[i]
		if off.ID != o.ID || in.ID != o.ID || !slices.Contains(known, off.State) || !slices.Contains(known, in.State) {
			return CodeShape
		}
		offlineCode, inlineKind := offlineInlineCode(o.Kind)
		if !inlineKind {
			if in != off {
				return CodeInline
			}
			continue
		}
		blockedCode, ok := inline[o.ID]
		if !ok {
			return CodeShape
		}
		if off.State != "BLOCKED" || off.Code != offlineCode {
			return CodeInline
		}
		switch {
		case blockedCode == "": // derivable: judged, left as the offline run had it, or reported as not run
			if in.State == "BLOCKED" && in != off && in.Code != kit.SvcInlineNativeNotRunCode {
				return CodeInline
			}
		case in.State != "BLOCKED" || in.Code != blockedCode && in != off: // not derivable: it stays BLOCKED
			return CodeInline
		}
	}
	return ""
}

// offlineInlineCode is the BLOCKED code the offline run gives an inline kind, and whether the kind is one.
func offlineInlineCode(kind string) (string, bool) {
	switch kind {
	case kit.SvcKindInlineTree:
		return kit.SvcInlineTreeUnavailableCode, true
	case kit.SvcKindQuery:
		return kit.SvcQueryUnregisteredCode, true
	}
	return "", false
}

// Unjudged counts the inline obligations the asset derives that the inline list leaves BLOCKED: an inline
// run that judged less than it could. Call it on a document Check accepted.
func (d Document) Unjudged(inline map[string]string) int {
	n := 0
	for _, j := range d.Inline {
		if code, ok := inline[j.ID]; ok && code == "" && j.State == "BLOCKED" {
			n++
		}
	}
	return n
}
