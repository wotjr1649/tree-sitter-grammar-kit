package kit

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	"reflect"
	"slices"
	"strconv"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The SVC scope (#362 scope 5) registers the extended denominator that the P0 registry leaves as
// `required:null`/UNREGISTERED for the n461-svc role: per case and step the source history, the
// edit points, the directive/segment/range/CodeBehind observation, the C# inline tree and the
// two queries. Independent expected values exist for the offline facts; the inline tree and
// query obligations are registered and stay BLOCKED until their evidence exists. The P0 registry
// and its ID namespace are not changed.

const (
	SvcExpectedSchema = "tsgk-query-svc-expected/r1"
	SvcExpectedPath   = "src/contracts/query-svc-expected-r1.json"
	// shortcut: the asset is 196 KB today; raise only with a new expected revision.
	svcExpectedMaxBytes = 512 * 1024
	svcRoleID           = "n461-svc"
)

// Obligation kinds of the extended denominator.
const (
	SvcKindHistory    = "SVC_HISTORY"
	SvcKindEditPoint  = "SVC_EDIT_POINT"
	SvcKindSegment    = "SVC_SEGMENT"
	SvcKindCodeBehind = "SVC_CODEBEHIND"
	SvcKindInlineTree = "SVC_INLINE_TREE"
	SvcKindQuery      = "SVC_INLINE_QUERY"
	SvcKindNegative   = "SVC_NEGATIVE"
)

type SvcExpectedEdit struct {
	StartByte   uint32    `json:"start_byte"`
	OldEndByte  uint32    `json:"old_end_byte"`
	NewEndByte  uint32    `json:"new_end_byte"`
	StartPoint  [2]uint32 `json:"start_point"`
	OldEndPoint [2]uint32 `json:"old_end_point"`
	NewEndPoint [2]uint32 `json:"new_end_point"`
}

type SvcExpectedStep struct {
	Step        int             `json:"step"`
	Bytes       uint64          `json:"bytes"`
	SHA256      string          `json:"sha256"`
	Observation *SvcObservation `json:"observation"`
}

type SvcExpectedNegative struct {
	Assessment string `json:"assessment"`
	Code       string `json:"code"`
	Process    *int   `json:"process"`
	InlineTree *int   `json:"inline_tree"`
}

type SvcExpectedCase struct {
	ID                   string               `json:"id"`
	Reference            bool                 `json:"reference"`
	Bytes                uint64               `json:"bytes"`
	SHA256               string               `json:"sha256"`
	Edits                []SvcExpectedEdit    `json:"edits"`
	Steps                []SvcExpectedStep    `json:"steps"`
	Negative             *SvcExpectedNegative `json:"negative,omitempty"`
	TreeExpectationSteps int                  `json:"tree_expectation_steps"`
}

type SvcExpectedWorkload struct {
	Set     string      `json:"set"`
	Route   string      `json:"route"`
	Format  string      `json:"format"`
	Cases   int         `json:"cases"`
	Edits   int         `json:"edits"`
	Steps   int         `json:"steps"`
	Queries []QualQuery `json:"queries"`
	Context SvcContext  `json:"context"`
}

// SvcExpected is the strict decoded tsgk-query-svc-expected/r1 asset.
type SvcExpected struct {
	Schema        string                `json:"schema"`
	Registry      queryregistrycore.Pin `json:"registry"`
	Qualification queryregistrycore.Pin `json:"qualification"`
	Fixture       queryregistrycore.Pin `json:"fixture"`
	Basis         string                `json:"basis"`
	Workload      SvcExpectedWorkload   `json:"workload"`
	InlineSteps   int                   `json:"inline_steps"`
	Cases         []SvcExpectedCase     `json:"cases"`
}

func svcFault(code string) error { return fail(KindInvalidInput, code, SvcExpectedPath, nil) }

// svcInventoryWorkload returns the n461-svc workload of the pinned qualification inventory.
func svcInventoryWorkload(inventory []byte) (QualWorkload, error) {
	inv, err := ParseQualificationInventory(inventory)
	if err != nil {
		return QualWorkload{}, err
	}
	for _, role := range inv.ExtraRoles {
		if role.ID == svcRoleID && len(role.Workloads) == 1 {
			return role.Workloads[0], nil
		}
	}
	return QualWorkload{}, svcFault("SVC_EXPECTED_WORKLOAD")
}

// ParseSvcExpected decodes the asset and binds it to the exact pinned registry, qualification
// inventory and fixture bytes, the inventory's 69 cases in order, 41 edits and 110 steps, and
// its two queries and CodeBehind declarations. null observations are only legal for the
// CodeBehind reference case; every other step needs one.
func ParseSvcExpected(data, registry, inventory, fixture []byte) (SvcExpected, error) {
	var e SvcExpected
	if len(data) > svcExpectedMaxBytes {
		return e, fail(KindResourceLimit, "SVC_EXPECTED_SIZE", SvcExpectedPath, nil)
	}
	if err := jsonv2.Unmarshal(data, &e, jsonv2.RejectUnknownMembers(true)); err != nil {
		return e, fail(KindInvalidInput, "SVC_EXPECTED_JSON", SvcExpectedPath, err)
	}
	if e.Schema != SvcExpectedSchema || e.Basis == "" {
		return e, svcFault("SVC_EXPECTED_SCHEMA")
	}
	if e.Registry != queryregistrycore.FilePin(queryregistrycore.Path, registry) ||
		e.Qualification != queryregistrycore.FilePin(queryregistrycore.QualificationPin.Path, inventory) ||
		e.Fixture != queryregistrycore.FilePin("src/testdata/native/n461/svc.json", fixture) {
		return e, svcFault("SVC_EXPECTED_PIN")
	}
	wl, err := svcInventoryWorkload(inventory)
	if err != nil {
		return e, err
	}
	w := e.Workload
	if w.Set != wl.Set || w.Route != wl.Route || w.Format != wl.Format || !reflect.DeepEqual(w.Queries, wl.Queries) || wl.SvcContext == nil || !reflect.DeepEqual(w.Context, *wl.SvcContext) ||
		w.Cases != len(wl.Cases) || len(e.Cases) != len(wl.Cases) {
		return e, svcFault("SVC_EXPECTED_WORKLOAD")
	}
	references := map[string]bool{}
	for _, r := range w.Context.CodeBehind {
		references[r.Case] = true
	}
	edits, steps, inline := 0, 0, 0
	for i, c := range e.Cases {
		q := wl.Cases[i]
		if c.ID != q.ID || c.Reference != references[c.ID] || c.Bytes != q.Input.Bytes || c.SHA256 != q.Input.SHA256 || len(c.Edits) != len(q.Edits) || len(c.Steps) != len(q.Edits)+1 ||
			c.TreeExpectationSteps != len(q.Expect) || (c.Negative != nil) != (q.ExpectAssessment != "" || q.ExpectCode != "") {
			return e, svcFault("SVC_EXPECTED_MEMBERSHIP")
		}
		if c.Negative != nil && (c.Negative.Assessment != q.ExpectAssessment || c.Negative.Code != q.ExpectCode || c.Negative.Process != nil || c.Negative.InlineTree != nil) {
			return e, svcFault("SVC_EXPECTED_NEGATIVE")
		}
		edits, steps = edits+len(c.Edits), steps+len(c.Steps)
		for k, s := range c.Steps {
			if s.Step != k || len(s.SHA256) != 64 || (s.Observation == nil) != c.Reference {
				return e, svcFault("SVC_EXPECTED_STEP")
			}
			if o := s.Observation; o != nil {
				if o.Directive == nil && o.AdditionalDirectives == nil && o.Coverage.Directive != "ABSENT" {
					return e, svcFault("SVC_EXPECTED_STEP")
				}
				if o.IncludedRanges != nil {
					inline++
				}
			}
		}
	}
	if edits != w.Edits || steps != w.Steps || inline != e.InlineSteps || w.Cases != 69 || w.Edits != 41 || w.Steps != 110 {
		return e, svcFault("SVC_EXPECTED_COUNTS")
	}
	return e, nil
}

// SvcObligation is one obligation of the extended denominator.
type SvcObligation struct {
	ID    string
	Kind  string
	Case  string
	Step  int
	Query string
}

func svcID(kind, c string, step int, extra string) string {
	return "svc1/" + kind + "/" + c + "/" + strconv.Itoa(step) + "/" + extra
}

// Obligations enumerates the extended denominator from the registered asset, in a fixed order:
// per case and step the history, edit point, segment, inline tree and its two queries, then the
// CodeBehind declarations and the VB negative. Counts: history 110, edit point 41, segment 109,
// inline tree 82 (81 composite steps plus the one CodeBehind reference C# case), query 164,
// CodeBehind 2, negative 1.
func (e SvcExpected) Obligations() []SvcObligation {
	var out []SvcObligation
	add := func(kind, c string, step int, query string) {
		out = append(out, SvcObligation{ID: svcID(kind, c, step, query), Kind: kind, Case: c, Step: step, Query: query})
	}
	for _, c := range e.Cases {
		for _, s := range c.Steps {
			add(SvcKindHistory, c.ID, s.Step, "")
			if s.Step > 0 {
				add(SvcKindEditPoint, c.ID, s.Step, "")
			}
			if s.Observation != nil {
				add(SvcKindSegment, c.ID, s.Step, "")
			}
			if c.Reference || s.Observation != nil && s.Observation.IncludedRanges != nil {
				add(SvcKindInlineTree, c.ID, s.Step, "")
				for _, q := range e.Workload.Queries {
					add(SvcKindQuery, c.ID, s.Step, q.ID)
				}
			}
		}
	}
	for _, r := range e.Workload.Context.CodeBehind {
		add(SvcKindCodeBehind, r.Case, 0, r.Name)
	}
	for _, c := range e.Cases {
		if c.Negative != nil {
			add(SvcKindNegative, c.ID, 0, "")
		}
	}
	return out
}

// SvcJudgment is the adjudicated state of one obligation on one host.
type SvcJudgment struct {
	ID    string
	State string // VERIFIED, FAILED, BLOCKED
	Code  string
}

// VerifySvcExpected recomputes the offline obligations from the pinned inventory and judges them
// against the independent expected values: source history (version bytes and hash), edit points,
// the segment observation through the existing ObserveServiceHostWithContext, the CodeBehind
// declarations and the unsupported-language negative through the existing svcVerdict. Inline tree
// and query obligations are BLOCKED here: the inventory registers authored tree expectations
// only for steps that need the real C# grammar, and no query expectation at all.
func VerifySvcExpected(e SvcExpected, inventory []byte) ([]SvcJudgment, error) {
	wl, err := svcInventoryWorkload(inventory)
	if err != nil {
		return nil, err
	}
	if len(wl.Cases) != len(e.Cases) {
		return nil, svcFault("SVC_EXPECTED_MEMBERSHIP")
	}
	state := map[string]SvcJudgment{}
	set := func(kind, c string, step int, extra string, ok bool, code string) {
		id := svcID(kind, c, step, extra)
		if ok {
			state[id] = SvcJudgment{ID: id, State: "VERIFIED"}
		} else {
			state[id] = SvcJudgment{ID: id, State: "FAILED", Code: code}
		}
	}
	declared := map[string]bool{}
	for i, c := range e.Cases {
		q := wl.Cases[i]
		if q.Source == nil || c.ID != q.ID {
			return nil, svcFault("SVC_EXPECTED_MEMBERSHIP")
		}
		versions, points, err := ApplyEdits(EncodingUTF8, []byte(*q.Source), q.Edits, NativeOperations()["native-query-large"].InputBytes)
		if err != nil || len(versions) != len(c.Steps) {
			set(SvcKindHistory, c.ID, 0, "", false, "SVC_HISTORY_EDITS")
			continue
		}
		for k, s := range c.Steps {
			set(SvcKindHistory, c.ID, k, "", uint64(len(versions[k])) == s.Bytes && digestHex(versions[k]) == s.SHA256, "SVC_HISTORY_MISMATCH")
			if k > 0 {
				ex, p := c.Edits[k-1], points[k-1]
				ed := q.Edits[k-1]
				set(SvcKindEditPoint, c.ID, k, "", ex.StartByte == ed.StartByte && ex.OldEndByte == ed.OldEndByte && ex.NewEndByte == ed.NewEndByte &&
					ex.StartPoint == [2]uint32{p.Start.Row, p.Start.Column} && ex.OldEndPoint == [2]uint32{p.OldEnd.Row, p.OldEnd.Column} && ex.NewEndPoint == [2]uint32{p.NewEnd.Row, p.NewEnd.Column}, "SVC_EDIT_POINT_MISMATCH")
			}
			if s.Observation != nil {
				got := ObserveServiceHostWithContext(EncodingUTF8, versions[k], &e.Workload.Context)
				set(SvcKindSegment, c.ID, k, "", reflect.DeepEqual(got, *s.Observation), "SVC_SEGMENT_MISMATCH")
				if cb := got.CodeBehind; cb != nil && cb.Resolution == "DECLARED" {
					declared[cb.Case+"\x00"+svcCodeBehindName(versions[k], cb)] = true
				}
			}
		}
		if c.Negative != nil {
			// the existing replay contract decides the unsupported-language verdict from the observation
			var steps []rcStep
			for _, s := range c.Steps {
				o := s.Observation
				steps = append(steps, rcStep{Composite: &rcComposite{Directive: o.Directive, Coverage: o.Coverage}})
			}
			assessment, code := svcVerdict(steps, false)
			o := c.Steps[0].Observation
			set(SvcKindNegative, c.ID, 0, "", assessment == c.Negative.Assessment && code == c.Negative.Code && o.IncludedRanges == nil && o.Language.Status == "UNSUPPORTED_LANGUAGE" && o.Coverage.Inline == "UNSUPPORTED", "SVC_NEGATIVE_CHANGED")
		}
	}
	for _, r := range e.Workload.Context.CodeBehind {
		set(SvcKindCodeBehind, r.Case, 0, r.Name, declared[r.Case+"\x00"+r.Name], "SVC_CODEBEHIND_UNOBSERVED")
	}
	var out []SvcJudgment
	for _, o := range e.Obligations() {
		j, ok := state[o.ID]
		switch {
		case ok:
		case o.Kind == SvcKindInlineTree:
			j = SvcJudgment{ID: o.ID, State: "BLOCKED", Code: "SVC_INLINE_TREE_GRAMMAR_UNAVAILABLE"}
		case o.Kind == SvcKindQuery:
			j = SvcJudgment{ID: o.ID, State: "BLOCKED", Code: "SVC_QUERY_EXPECTATION_UNREGISTERED"}
		default:
			j = SvcJudgment{ID: o.ID, State: "FAILED", Code: "SVC_OBLIGATION_UNJUDGED"}
		}
		out = append(out, j)
	}
	return out, nil
}

func svcCodeBehindName(src []byte, cb *SvcCodeBehind) string {
	return svcString(EncodingUTF8, src[cb.Value.StartByte:cb.Value.EndByte])
}

// SvcScopeCounter keeps required = verified + failed + blocked + unprocessed per kind and host.
type SvcScopeCounter struct {
	Kind        string `json:"kind"`
	Platform    string `json:"platform"`
	Required    uint64 `json:"required"`
	Verified    uint64 `json:"verified"`
	Failed      uint64 `json:"failed"`
	Blocked     uint64 `json:"blocked"`
	Unprocessed uint64 `json:"unprocessed"`
}

// SvcScopeCoverage is scope-local. Completion stays BLOCKED while any obligation is not VERIFIED,
// and the P0 root (INCOMPLETE, full_scope_complete=false) is never changed.
type SvcScopeCoverage struct {
	Counters          []SvcScopeCounter `json:"counters"`
	Completion        string            `json:"completion"`
	FullScopeComplete bool              `json:"full_scope_complete"`
}

// FoldSvcScope folds one host's judgments over the full enumerated denominator of both registered
// hosts. A judgment for an obligation that is not in the enumeration, a duplicate, an unknown
// state, a VERIFIED with a code, or a FAILED/BLOCKED without one is an error; a missing judgment
// is BLOCKED on a host listed in blocked and UNPROCESSED elsewhere. judgments belong to platform.
func FoldSvcScope(e SvcExpected, platform string, judgments []SvcJudgment, blocked map[string]bool) (SvcScopeCoverage, error) {
	out := SvcScopeCoverage{Completion: "COMPLETE"}
	obligations := e.Obligations()
	byID := map[string]SvcObligation{}
	for _, o := range obligations {
		if _, dup := byID[o.ID]; dup {
			return out, svcFault("SVC_SCOPE_DUPLICATE_OBLIGATION")
		}
		byID[o.ID] = o
	}
	seen := map[string]SvcJudgment{}
	for _, j := range judgments {
		if _, ok := byID[j.ID]; !ok {
			return out, svcFault("SVC_SCOPE_UNDECLARED")
		}
		if _, dup := seen[j.ID]; dup {
			return out, svcFault("SVC_SCOPE_OVERLAP")
		}
		seen[j.ID] = j
	}
	hosts := []string{"windows-amd64", "linux-amd64"}
	if !slices.Contains(hosts, platform) {
		return out, svcFault("SVC_SCOPE_PLATFORM")
	}
	index := map[string]int{}
	for _, host := range hosts {
		for _, o := range obligations {
			key := o.Kind + "/" + host
			if _, ok := index[key]; !ok {
				index[key] = len(out.Counters)
				out.Counters = append(out.Counters, SvcScopeCounter{Kind: o.Kind, Platform: host})
			}
			c := &out.Counters[index[key]]
			c.Required++
			state := "UNPROCESSED"
			var j SvcJudgment
			if host == platform {
				if v, ok := seen[o.ID]; ok {
					j, state = v, v.State
				}
			}
			switch state {
			case "VERIFIED":
				if j.Code != "" {
					return out, svcFault("SVC_SCOPE_VERIFIED_CODE")
				}
				c.Verified++
			case "FAILED", "BLOCKED":
				if j.Code == "" {
					return out, svcFault("SVC_SCOPE_CODE")
				}
				if state == "FAILED" {
					c.Failed++
				} else {
					c.Blocked++
				}
			case "UNPROCESSED":
				if blocked[host] {
					c.Blocked++
				} else {
					c.Unprocessed++
				}
			default:
				return out, svcFault("SVC_SCOPE_STATE")
			}
		}
	}
	for _, c := range out.Counters {
		if !countsSumTo(c.Required, c.Verified, c.Failed, c.Blocked, c.Unprocessed) {
			return out, svcFault("SVC_SCOPE_SUM")
		}
		if c.Failed > 0 {
			out.Completion = "FAIL"
		} else if c.Verified != c.Required && out.Completion != "FAIL" {
			out.Completion = "BLOCKED"
		}
	}
	return out, nil
}

// String keeps counters readable in test logs.
func (c SvcScopeCounter) String() string {
	return fmt.Sprintf("%s/%s req=%d ok=%d fail=%d blocked=%d unproc=%d", c.Kind, c.Platform, c.Required, c.Verified, c.Failed, c.Blocked, c.Unprocessed)
}
