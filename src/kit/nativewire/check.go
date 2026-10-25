package nativewire

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"strconv"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// Expect is what a response must answer: the request and the caller's own computation of
// its sources and edit points. Live passes everything; a caller without a value passes
// the documented absent form and only the rule that needs it is skipped. A caller must not
// fill a value from the response itself.
type Expect struct {
	Check       func() error       // Optional cancellation checkpoint for repeated tree hashing.
	Revision    string             // the request revision ("" is r1)
	ID          string             // the request id
	Nodes       uint64             // the node limit of a completed tree
	Queries     []querywire.Source // r2/r4: the requested queries
	API         bool               // r2/r4: API observations requested
	Points      []uint32           // the byte of each requested API point
	SourceBytes uint64             // the request source length
	Edits       []EditSpan         // the request edits
	// Sources is the expected identity of step k (the source after k edits); a step without
	// an entry is rejected.
	Sources []SourceID
	// EditPoints are the edit points of each edit; nil skips only the point comparison.
	EditPoints []EditPoints
	// Exit is the driver exit code; nil skips only its agreement with the status.
	Exit *int
	// Observation binds r3/r4 observations; required for those revisions.
	Observation *Binding
}

// EditSpan is the byte span of one request edit.
type EditSpan struct {
	StartByte, OldEndByte, NewEndByte uint32
}

// SourceID is the length and sha256 of one source version.
type SourceID struct {
	Bytes  uint64
	SHA256 string
}

// Binding is the run binding of the observations: the runtime manifest the producer must
// declare, the observation run, the producer build and the history start.
type Binding struct {
	Manifest, RunID, ProducerSHA256 string
	HistoryStart                    uint64
}

func (e Expect) revision() string {
	if e.Revision == "" {
		return Protocol
	}
	return e.Revision
}

// queries reports whether the revision carries the query limits, queries and API switch (r2, r4).
func (e Expect) queries() bool { return e.revision() == ProtocolR2 || e.revision() == ProtocolR4 }

// observes reports whether the revision carries the bounded logger observations (r3, r4).
func (e Expect) observes() bool { return e.revision() == ProtocolR3 || e.revision() == ProtocolR4 }

// Tree is a validated tree of one step.
type Tree struct {
	Status   string
	Code     string
	Form     string // full | summary | record | "" when not completed
	Wire     WireTree
	Nodes    []TreeNode // full form only
	Partials [][]TreeNode
	Queries  []WireQuery // r2: one per requested query when the tree completed
	API      *WireAPI    // r2: requested API observations of a full tree
}

// Incomplete returns the status and code of the first part of t that did not complete:
// the tree itself, else a query cut by a limit or failed. An invalid query is an
// observation of the query input, not an incomplete tree.
func (t Tree) Incomplete() (string, string, bool) {
	if t.Status != StatusCompleted {
		return t.Status, t.Code, true
	}
	for _, q := range t.Queries {
		if q.Status == StatusResourceLimit || q.Status == StatusFailed {
			return q.Status, q.Code, true
		}
	}
	return "", "", false
}

func isNull(v jsontext.Value) bool { return string(v) == "null" }

// requireMembers rejects a decoded r2 object or array of objects that lacks a member: the
// strict decoder would otherwise leave an absent member at its zero value, and an absent
// predicate list must never read as "no predicates".
func requireMembers(raw jsontext.Value, array bool, names ...string) error {
	if err := querywire.RequireMembers(raw, array, names...); err != nil {
		return invalid(err.Code, err.Cause)
	}
	return nil
}

// CheckTree validates a tree object against the step's source length, the node limit and,
// for r2/r4, the requested queries and API observations.
func CheckTree(w WireTree, sourceBytes uint64, e Expect) (Tree, error) {
	t := Tree{Status: w.Status, Code: w.Code, Wire: w}
	r2 := e.queries()
	if r2 != (w.Queries != nil) || r2 != (w.API != nil) {
		return t, invalid("RESPONSE_INVALID", errors.New("query/api members do not match the revision"))
	}
	switch w.Status {
	case StatusCompleted:
		if w.Code != "" || w.Form == nil {
			return t, invalid("RESPONSE_INVALID", errors.New("completed tree without form"))
		}
	case StatusResourceLimit, StatusFailed:
		if w.Code == "" || w.Form != nil || w.Nodes != nil || (r2 && (!isNull(w.Queries) || !isNull(w.API))) {
			return t, invalid("RESPONSE_INVALID", errors.New("noncomplete tree with data"))
		}
		return t, nil
	default:
		return t, invalid("RESPONSE_INVALID", errors.New("tree status "+strconv.Quote(w.Status)))
	}
	t.Form = *w.Form
	if len(w.Digest) != 64 || w.DescendantCount == 0 || w.DescendantCount > e.Nodes {
		return t, invalid("RESPONSE_INVALID", errors.New("tree header"))
	}
	switch t.Form {
	case "full":
		ns, err := Nodes(w.Nodes, w.Types, w.Fields)
		if err != nil {
			return t, err
		}
		if err := ValidateTree(ns, sourceBytes); err != nil {
			return t, invalid("RESPONSE_INVALID", err)
		}
		if uint64(len(ns)) != w.DescendantCount || ns[0].HasError != w.HasError {
			return t, invalid("RESPONSE_INVALID", errors.New("count or root flags"))
		}
		digest, err := treeDigest(ns, e.Check)
		if err != nil {
			return t, err
		}
		if digest != w.Digest {
			return t, invalid("TREE_DIGEST_MISMATCH", nil)
		}
		t.Nodes = ns
	case "summary", "record":
		if w.Errors == nil || w.Nodes != nil || uint64(len(w.Errors.Items)) > w.Errors.Limit || w.Errors.Truncated != (w.Errors.Total > uint64(len(w.Errors.Items))) {
			return t, invalid("RESPONSE_INVALID", errors.New("summary errors"))
		}
		for _, it := range w.Errors.Items {
			if (it.Kind != "ERROR" && it.Kind != "MISSING") || it.StartByte > it.EndByte || uint64(it.EndByte) > sourceBytes {
				return t, invalid("RESPONSE_INVALID", errors.New("error item"))
			}
		}
		for _, p := range w.PartialTrees {
			ns, err := Nodes(p.Nodes, w.Types, w.Fields)
			if err != nil {
				return t, err
			}
			if len(ns) > 0 {
				if err := ValidateTree(ns, sourceBytes); err != nil {
					return t, invalid("RESPONSE_INVALID", err)
				}
			}
			t.Partials = append(t.Partials, ns)
		}
	default:
		return t, invalid("RESPONSE_INVALID", errors.New("form"))
	}
	if w.Declarations != nil {
		for _, d := range *w.Declarations {
			if d.StartByte > d.EndByte || uint64(d.EndByte) > sourceBytes || (d.Name != nil && (d.Name.StartByte > d.Name.EndByte || uint64(d.Name.EndByte) > sourceBytes)) {
				return t, invalid("RESPONSE_INVALID", errors.New("declaration range"))
			}
			switch d.Status {
			case "PASS", "NAME_MISSING", "HAS_ERROR":
			default:
				return t, invalid("RESPONSE_INVALID", errors.New("declaration status"))
			}
		}
	}
	if r2 {
		if err := t.checkQueries(e.Queries, sourceBytes); err != nil {
			return t, err
		}
		if err := t.checkAPI(e); err != nil {
			return t, err
		}
	}
	return t, nil
}

// checkQueries retains the full-node link check while sharing strict wire validation.
func (t *Tree) checkQueries(sources []querywire.Source, sourceBytes uint64) error {
	t.Queries = nil
	var queries []WireQuery
	var err *querywire.Error
	if t.Nodes != nil {
		queries, err = querywire.DecodeFull(t.Wire.Queries, sources, sourceBytes, t.Wire.DescendantCount, func(q WireQuery, row []int64) bool {
			if row[3] < 0 || row[3] >= int64(len(t.Nodes)) {
				return false
			}
			// the capture row's node fields (type, flags, span) as the public capture reads them
			n, f := t.Nodes[row[3]], row[5]
			return n.Type == q.Types[row[4]] && n.Named == (f&1 != 0) && n.Extra == (f&2 != 0) && n.IsError == (f&4 != 0) && n.HasError == (f&8 != 0) && n.IsMissing == (f&16 != 0) &&
				n.StartByte == uint32(row[6]) && n.EndByte == uint32(row[7]) && n.StartPoint == (Point{Row: uint32(row[8]), Column: uint32(row[9])}) && n.EndPoint == (Point{Row: uint32(row[10]), Column: uint32(row[11])})
		})
	} else {
		queries, err = querywire.DecodeSummary(t.Wire.Queries, sources, sourceBytes, t.Wire.DescendantCount)
	}
	if err != nil {
		return invalid(err.Code, err.Cause)
	}
	t.Queries = queries
	return nil
}

// checkAPI validates the shape of requested API observations: present exactly for a
// completed full tree of an API request, with every row's arity and table indices in
// range. Whether the values agree with the serialized tree is judged by the caller.
func (t *Tree) checkAPI(e Expect) error {
	bad := func(what string) error { return invalid("RESPONSE_INVALID", errors.New("api: "+what)) }
	want := e.API && t.Form == "full"
	if isNull(t.Wire.API) {
		if want {
			return bad("missing")
		}
		return nil
	}
	if !want {
		return bad("unrequested")
	}
	a, err := querywire.DecodeAPI(t.Wire.API, int64(len(t.Nodes)), e.Points)
	if err != nil {
		return invalid(err.Code, err.Cause)
	}
	t.API = a
	return nil
}

// Checked is a response validated against its Expect.
type Checked struct {
	Response Response
	Steps    []CheckedStep
}

// CheckedStep is one validated step.
type CheckedStep struct {
	Step         Step
	Incremental  Tree
	Fresh        *Tree
	Observations []reuseobs.Observation
}

// Open returns the status and code of the step's first noncomplete tree or query: the
// incremental tree's, else the fresh tree's.
func (cs CheckedStep) Open() (string, string, bool) {
	status, code, bad := cs.Incremental.Incomplete()
	if !bad && cs.Fresh != nil {
		status, code, bad = cs.Fresh.Incomplete()
	}
	return status, code, bad
}

// Done reports whether step k completed: its incremental tree and, on an edit step, its
// fresh tree.
func (cs CheckedStep) Done(k int) bool {
	_, _, incBad := cs.Incremental.Incomplete()
	freshBad := true
	if cs.Fresh != nil {
		_, _, freshBad = cs.Fresh.Incomplete()
	}
	return !incBad && (k == 0 || !freshBad)
}

// CheckHeader validates everything of a response but its steps, given their number n:
// protocol and id, status/exit agreement, the producer capabilities, the step count and
// the source length. stop reports an INVALID_REQUEST response, which has no steps and
// nothing more to check.
func CheckHeader(r Response, n int, e Expect) (stop bool, err error) {
	if r.Protocol != e.revision() {
		return false, invalid("RESPONSE_PROTOCOL_MISMATCH", fmt.Errorf("request %s response %s", e.revision(), r.Protocol))
	}
	if !r.Complete {
		return false, invalid("RESPONSE_INVALID", errors.New("completeness"))
	}
	if r.ID != e.ID {
		return false, invalid("RESPONSE_INVALID", errors.New("id"))
	}
	want := map[string]int{StatusCompleted: 0, "INVALID_REQUEST": 2, StatusResourceLimit: 3, StatusFailed: 4}
	code, ok := want[r.Status]
	if !ok || e.Exit != nil && code != *e.Exit {
		exit := "unknown"
		if e.Exit != nil {
			exit = strconv.Itoa(*e.Exit)
		}
		return false, invalid("EXIT_STATUS_MISMATCH", fmt.Errorf("status %s exit %s", r.Status, exit))
	}
	if (r.Status == StatusCompleted) != (r.Code == "") {
		return false, invalid("RESPONSE_INVALID", errors.New("code"))
	}
	p := r.Producer
	if v := e.revision(); v != Protocol && v != ProtocolR2 && v != ProtocolR3 && v != ProtocolR4 {
		return false, invalid("RESPONSE_PROTOCOL_MISMATCH", errors.New("unknown protocol"))
	}
	if e.revision() == ProtocolR3 && (len(e.Queries) != 0 || e.API) {
		return false, invalid("RESPONSE_INVALID", errors.New("r3 query/API combination"))
	}
	if !e.queries() {
		if p.Query != "UNSUPPORTED" || p.API != nil || p.Predicates != nil || p.SymbolCount != nil || p.FieldCount != nil {
			return false, invalid("RESPONSE_INVALID", errors.New("r1 producer"))
		}
	} else if p.API == nil || p.Predicates == nil || *p.Predicates != "NOT_EVALUATED" || p.SymbolCount == nil || p.FieldCount == nil || p.Query == "" {
		return false, invalid("RESPONSE_INVALID", errors.New("r2 producer capabilities"))
	}
	if e.observes() {
		if err := requireMembers(p.ReuseObservation, false, "policy", "runtime_manifest_sha256"); err != nil {
			return false, err
		}
		var capability struct {
			Policy                string `json:"policy"`
			RuntimeManifestSHA256 string `json:"runtime_manifest_sha256"`
		}
		if err := jsonv2.Unmarshal(p.ReuseObservation, &capability, jsonv2.RejectUnknownMembers(true)); err != nil || e.Observation == nil ||
			capability.Policy != reuseobs.Policy || capability.RuntimeManifestSHA256 != e.Observation.Manifest {
			return false, invalid("RESPONSE_INVALID", errors.New("r3 observation capability"))
		}
	} else if len(p.ReuseObservation) != 0 {
		return false, invalid("RESPONSE_INVALID", errors.New("unexpected observation capability"))
	}
	if r.Status == "INVALID_REQUEST" {
		if n != 0 {
			return false, invalid("RESPONSE_INVALID", errors.New("steps on invalid request"))
		}
		return true, nil
	}
	if r.Status == StatusCompleted && n != len(e.Edits)+1 {
		return false, invalid("RESPONSE_INVALID", errors.New("step count"))
	}
	if n > len(e.Edits)+1 || r.SourceBytes != e.SourceBytes {
		return false, invalid("RESPONSE_INVALID", errors.New("steps or source"))
	}
	return false, nil
}

// CheckStep validates step k (within the count CheckHeader accepted): its source identity,
// shape and edit, then its incremental tree, its observations and its fresh tree.
func CheckStep(k int, s Step, e Expect) (CheckedStep, error) {
	cs := CheckedStep{Step: s}
	if k < 0 || k >= len(e.Sources) || k > 0 && k > len(e.Edits) {
		return cs, invalid("RESPONSE_INVALID", fmt.Errorf("step %d has no expectation", k))
	}
	want := e.Sources[k]
	if s.Step != k || s.SourceBytes != want.Bytes || s.SourceSHA256 != want.SHA256 {
		return cs, invalid("SOURCE_TRANSPORT_MISMATCH", fmt.Errorf("step %d", k))
	}
	if (k == 0) != (s.Edit == nil) || (k == 0 && (s.Route != nil || s.Fresh != nil)) {
		return cs, invalid("RESPONSE_INVALID", fmt.Errorf("step %d shape", k))
	}
	if k > 0 {
		ed, got := e.Edits[k-1], s.Edit
		bad := got.StartByte != ed.StartByte || got.OldEndByte != ed.OldEndByte || got.NewEndByte != ed.NewEndByte
		if e.EditPoints != nil {
			if k > len(e.EditPoints) {
				return cs, invalid("RESPONSE_INVALID", fmt.Errorf("step %d has no edit points", k))
			}
			p := e.EditPoints[k-1]
			bad = bad || got.StartPoint != [2]uint32{p.Start.Row, p.Start.Column} || got.OldEndPoint != [2]uint32{p.OldEnd.Row, p.OldEnd.Column} ||
				got.NewEndPoint != [2]uint32{p.NewEnd.Row, p.NewEnd.Column}
		}
		if bad {
			return cs, invalid("EDIT_POINT_MISMATCH", fmt.Errorf("step %d", k))
		}
	}
	inc, err := CheckTree(s.Incremental, s.SourceBytes, e)
	if err != nil {
		return cs, err
	}
	cs.Incremental = inc
	if e.observes() {
		var raw []jsontext.Value
		if err := jsonv2.Unmarshal(s.Observations, &raw); err != nil || len(raw) != 1+min(k, 1) || e.Observation == nil {
			return cs, invalid("RESPONSE_INVALID", errors.New("observation roles"))
		}
		for index, value := range raw {
			role := "initial"
			if k > 0 {
				role = "incremental"
				if index == 1 {
					role = "fresh"
				}
			}
			b := e.Observation
			binding := reuseobs.Binding{RuntimeManifestSHA256: b.Manifest, RequestID: e.ID,
				RunID: b.RunID, ProducerSHA256: b.ProducerSHA256, HistoryStart: b.HistoryStart,
				Step: uint64(k), Role: role, SourceSHA256: s.SourceSHA256, SourceBytes: s.SourceBytes}
			if k > 0 {
				binding.PreviousSourceSHA256 = e.Sources[k-1].SHA256
			}
			observation, err := reuseobs.Decode(value, binding)
			if err != nil {
				return cs, invalid("RESPONSE_INVALID", err)
			}
			cs.Observations = append(cs.Observations, observation)
		}
	} else if len(s.Observations) != 0 {
		return cs, invalid("RESPONSE_INVALID", errors.New("unexpected observations"))
	}
	if s.Fresh != nil {
		f, err := CheckTree(*s.Fresh, s.SourceBytes, e)
		if err != nil {
			return cs, err
		}
		cs.Fresh = &f
	}
	return cs, nil
}

// CheckOpen rejects an unfinished step k of n that is not the last one, or the last one of
// a COMPLETED response.
func CheckOpen(k, n int, status string, done bool) error {
	if !done && (k != n-1 || status == StatusCompleted) {
		return invalid("RESPONSE_INVALID", fmt.Errorf("step %d not completed", k))
	}
	return nil
}

// Tally is the end state of the checked steps: their number, how many completed and the
// last step's first noncomplete status and code.
type Tally struct {
	Steps                int
	Completed            uint64
	LastStatus, LastCode string
	LastOpen             bool
}

// Add counts step k.
func (t *Tally) Add(k int, cs CheckedStep) {
	t.Steps++
	if cs.Done(k) {
		t.Completed++
	}
	t.LastStatus, t.LastCode, t.LastOpen = cs.Open()
}

// CheckEnd validates the response end against the checked steps: the status and code are
// those of the first noncomplete tree or query, and steps_completed counts the finished
// steps. A response that dropped its steps (OUTPUT_LIMIT, ALLOCATION_LIMIT,
// LANGUAGE_INCOMPATIBLE, and in r2 a QUERY_TIME_LIMIT the runtime ran past) reports the
// count only.
func CheckEnd(r Response, t Tally, e Expect) error {
	if r.Status != StatusCompleted {
		if t.Steps == 0 {
			if r.Code != "OUTPUT_LIMIT" && r.Code != "ALLOCATION_LIMIT" && r.Code != "LANGUAGE_INCOMPATIBLE" &&
				(r.Code != "QUERY_TIME_LIMIT" || !e.queries() || len(e.Queries) == 0 || r.Status != StatusResourceLimit) {
				return invalid("RESPONSE_INVALID", errors.New("noncomplete response without steps"))
			}
		} else if !t.LastOpen || t.LastStatus != r.Status || t.LastCode != r.Code {
			return invalid("RESPONSE_INVALID", errors.New("response status/code differ from the first noncomplete tree or query"))
		}
	}
	if t.Steps > 0 && t.Completed != r.StepsCompleted {
		return invalid("RESPONSE_INVALID", errors.New("steps_completed"))
	}
	return nil
}

// Check validates a decoded response against e: CheckHeader, then each step in order with
// CheckStep and CheckOpen, then CheckEnd.
func Check(r Response, e Expect) (Checked, error) {
	c := Checked{Response: r}
	if stop, err := CheckHeader(r, len(r.Steps), e); err != nil || stop {
		return c, err
	}
	var t Tally
	for k, s := range r.Steps {
		cs, err := CheckStep(k, s, e)
		if err != nil {
			return c, err
		}
		if err := CheckOpen(k, len(r.Steps), r.Status, cs.Done(k)); err != nil {
			return c, err
		}
		t.Add(k, cs)
		c.Steps = append(c.Steps, cs)
	}
	return c, CheckEnd(r, t, e)
}
