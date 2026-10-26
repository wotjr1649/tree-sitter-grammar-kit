package kit

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"reflect"
	"slices"
	"strconv"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// ReuseAcceptedResult belongs to its enclosing case and policy/ledger identity.
// It does not authenticate execution or change the public node identity route.
type ReuseAcceptedResult struct {
	Schema        string              `json:"schema"`
	HistorySHA256 string              `json:"history_sha256"`
	Result        string              `json:"result"`
	Code          string              `json:"code"`
	Steps         []ReuseAcceptedStep `json:"steps"`
}

// ReuseAcceptedStep is one fixed target, retained even when history completion fails.
type ReuseAcceptedStep struct {
	Step   int    `json:"step"`
	Result string `json:"result"`
	Code   string `json:"code"`
}

// ReuseAcceptedRequest contains caller-verified registration and run context. Expected
// must come from the request, never the response; raw is an unframed native payload.
// Nil source/raw/projection is unsubmitted evidence. Declared missing members must be
// rejected by the guarded reader before calling this pure checker.
type ReuseAcceptedRequest struct {
	Ledger                     *ReuseAcceptedLedger
	Route                      string
	Case                       IncrementalCase
	Source, Raw, Projection    []byte
	Expected                   nativewire.Expect
	Policy                     IdentityRef
	Platform, RecordedPlatform string
}

type acceptedProcess struct {
	Status          string `json:"status"`
	Reason          string `json:"reason"`
	ExitCode        int    `json:"exit_code"`
	StdoutBytes     uint64 `json:"stdout_bytes"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	Cleanup         struct {
		Verified bool `json:"verified"`
	} `json:"cleanup"`
}

// JudgeReuseAccepted recomputes accepted events and full CST equality for the exact
// original history. Hashes and caller-supplied context are bindings, not execution proof.
// Cancellation uses Expected.Check, including when replay shares its enclosing budget.
func JudgeReuseAccepted(req ReuseAcceptedRequest) (ReuseAcceptedResult, error) {
	return judgeReuseAccepted(req, false)
}

// Only qualification's registered negative control can use the alternate expectation.
func checkReuseOmitControl(req ReuseAcceptedRequest) (string, error) {
	proof, err := judgeReuseAccepted(req, true)
	return proof.Result, err
}

func judgeReuseAccepted(req ReuseAcceptedRequest, omitControl bool) (ReuseAcceptedResult, error) {
	h := req.Ledger.original(req.Case.ID)
	res := ReuseAcceptedResult{Schema: ReuseAcceptedSchema, Result: claimBlocked, Code: "REUSE_ACCEPTED_EVIDENCE_MISSING", Steps: []ReuseAcceptedStep{}}
	if h == nil {
		res.Result, res.Code = claimFail, "REUSE_ACCEPTED_HISTORY_MISMATCH"
		return res, nil
	}
	res.HistorySHA256 = h.historySHA256()
	for _, k := range h.BlockedSteps {
		res.Steps = append(res.Steps, ReuseAcceptedStep{Step: k, Result: claimBlocked, Code: "REUSE_ACCEPTED_EVIDENCE_MISSING"})
	}
	failures := 0
	fail := func(code string) {
		failures++
		if res.Result != claimFail {
			res.Code = code
		}
		res.Result = claimFail
	}
	if req.Source != nil && req.Raw != nil && req.Projection != nil {
		res.Code = "REUSE_ACCEPTED_INCOMPLETE"
	}
	check := req.Expected.Check
	var stopped error
	if check != nil {
		checkpoint := check
		check = func() error {
			if stopped == nil {
				stopped = checkpoint()
			}
			return stopped
		}
		if err := check(); err != nil {
			return res, err
		}
	}
	source := []byte(h.SourceUTF8)
	if err := req.Ledger.CheckCase(req.Route, req.Case, source); err != nil {
		fail("REUSE_ACCEPTED_HISTORY_MISMATCH")
	}
	if req.Source != nil && (len(req.Source) != len(source) || digestHex(req.Source) != digestHex(source)) {
		fail("REUSE_ACCEPTED_SOURCE_MISMATCH")
	}
	if !slices.Contains([]string{"windows/amd64", "linux/amd64"}, req.Platform) || req.RecordedPlatform != req.Platform {
		fail("REUSE_ACCEPTED_PLATFORM_MISMATCH")
	}
	versions, points, err := ApplyEdits(EncodingUTF8, source, h.edits(), NativeOperations()["native-parse-edit"].InputBytes)
	if err != nil {
		return res, err
	}
	e := req.Expected
	e.Check = check
	if req.Policy.Role != "policy" || req.Policy.Schema != NativePolicySchema || !hex64.MatchString(req.Policy.SHA256) {
		fail("REUSE_ACCEPTED_POLICY_MISMATCH")
	}
	hasExpected := e.Observation != nil || req.Raw != nil
	if hasExpected && ((e.Revision != nativewire.ProtocolR3 && e.Revision != nativewire.ProtocolR4) || e.ID != h.ID || e.Nodes != NativeOperations()["native-parse-edit"].Nodes ||
		e.API || len(e.Queries) != 0 || len(e.Points) != 0 || e.SourceBytes != uint64(len(source)) || e.Observation == nil ||
		e.Observation != nil && (e.Observation.Manifest != reuseobs.RuntimeManifestSHA256 || e.Observation.HistoryStart != 0 || !validID(e.Observation.RunID) || !hex64.MatchString(e.Observation.ProducerSHA256))) {
		fail("REUSE_ACCEPTED_REQUEST_MISMATCH")
	}
	sources := make([]nativewire.SourceID, len(versions))
	spans := make([]nativewire.EditSpan, len(h.Edits))
	for i, b := range versions {
		sources[i] = nativewire.SourceID{Bytes: uint64(len(b)), SHA256: digestHex(b)}
	}
	for i, ed := range h.Edits {
		spans[i] = nativewire.EditSpan{StartByte: ed.StartByte, OldEndByte: ed.OldEndByte, NewEndByte: ed.NewEndByte}
	}
	if hasExpected && (!reflect.DeepEqual(e.Sources, sources) || !reflect.DeepEqual(e.Edits, spans) || !reflect.DeepEqual(e.EditPoints, points)) {
		fail("REUSE_ACCEPTED_REQUEST_MISMATCH")
	}
	// Even an invalid enclosing expectation cannot hide contradictions in provided data.
	e.Sources, e.Edits, e.EditPoints = sources, spans, points
	var c rcCase
	if req.Projection != nil {
		if err := acceptedProjection(req.Projection, &c, check); err != nil {
			if stopped != nil {
				return res, stopped
			}
			if asError(err).Kind == KindResourceLimit {
				return res, err
			}
			fail("REUSE_ACCEPTED_PROJECTION_INVALID")
		}
		if c.ID != req.Case.ID || c.Input != req.Case.Input {
			fail("REUSE_ACCEPTED_PROJECTION_BINDING")
		}
		{
			for k, s := range c.Steps {
				if req.Raw != nil && k < len(versions) && !earlyProjectionAvailable(c) {
					continue
				}
				before := failures
				obs, ok := acceptedObservations(s.Observations, k, e)
				if !ok {
					fail("REUSE_ACCEPTED_OBSERVATION_INVALID")
				}
				for _, o := range obs {
					if acceptedArgumentContradiction(o, omitControl) {
						fail("REUSE_ACCEPTED_PARSE_ARGUMENT_MISMATCH")
					}
				}
				zero := req.Raw == nil && !omitControl && ok && failures == before && len(obs) == 2 && obs[0].Status == "COMPLETE" && obs[0].Counts != nil && obs[0].Counts.Accepted == 0
				for i := range res.Steps {
					target := &res.Steps[i]
					if target.Step != k {
						continue
					}
					if zero {
						fail("REUSE_ACCEPTED_ZERO")
						if target.Result != claimFail {
							target.Result, target.Code = claimFail, "REUSE_ACCEPTED_ZERO"
						}
					} else if failures > before && target.Result != claimFail {
						target.Result, target.Code = claimFail, res.Code
					}
				}
			}
		}
	}
	var process *acceptedProcess
	if req.Projection != nil && jsonv2.Unmarshal(c.Process, &process) != nil {
		fail("REUSE_ACCEPTED_PROCESS_MISMATCH")
	}
	processComplete := process != nil && process.Status == StatusCompleted && process.Reason == "EXITED" && process.ExitCode == 0 && process.Cleanup.Verified && !process.StdoutTruncated && !process.StderrTruncated
	earlyProjection := process != nil && c.ExecutionStatus != StatusCompleted && (!process.Cleanup.Verified && c.Code == "PROCESS_CLEANUP_UNVERIFIED" ||
		process.Status == StatusResourceLimit && c.ExecutionStatus == StatusResourceLimit && c.Code == process.Reason ||
		process.Status == StatusCancelled && c.ExecutionStatus == StatusCancelled && c.Code == "CANCELLED")
	if process != nil && process.Reason != "EXITED" {
		e.Exit = nil
	} // forced termination has no natural driver exit
	var raw nativewire.Response
	var rawOK bool
	if req.Raw != nil {
		if uint64(len(req.Raw)) > NativeOperations()["native-parse-edit"].OutputBytes {
			return res, &Error{Kind: KindResourceLimit, Code: "REUSE_ACCEPTED_OUTPUT_LIMIT"}
		}
		if err := acceptedRawShape(req.Raw, check); err != nil {
			if stopped != nil {
				return res, stopped
			}
			if asError(err).Kind == KindResourceLimit {
				return res, err
			}
			fail("REUSE_ACCEPTED_RAW_INVALID")
		}
		raw, err = nativewire.DecodeResponse(req.Raw)
		if err != nil {
			fail("REUSE_ACCEPTED_RAW_INVALID")
		} else {
			rawOK = true
			if req.Projection != nil && process == nil {
				fail("REUSE_ACCEPTED_PROCESS_MISSING")
			}
			if _, err := nativewire.CheckHeader(raw, len(raw.Steps), e); err != nil {
				fail("REUSE_ACCEPTED_RAW_INVALID")
			}
		}
	}
	if req.Projection != nil && rawOK {
		if (!earlyProjection || c.ResponseStatus != "") && (c.ResponseStatus != raw.Status || c.ResponseCode != raw.Code) ||
			(!earlyProjection && len(c.Steps) != len(raw.Steps)) || len(c.Steps) > len(raw.Steps) {
			fail("REUSE_ACCEPTED_RAW_PROJECTION_MISMATCH")
		}
		var top struct {
			Encoding string         `json:"encoding"`
			Producer jsontext.Value `json:"producer"`
		}
		if jsonv2.Unmarshal(req.Projection, &top) != nil || top.Encoding != EncodingUTF8 {
			fail("REUSE_ACCEPTED_PRODUCER_MISMATCH")
		}
		if !earlyProjection || len(top.Producer) != 0 && string(top.Producer) != "null" {
			var producer nativewire.Producer
			if jsonv2.Unmarshal(top.Producer, &producer) != nil || !acceptedProducerEqual(producer, raw.Producer) {
				fail("REUSE_ACCEPTED_PRODUCER_MISMATCH")
			}
		}
		if process == nil || req.Expected.Exit == nil || process != nil && req.Expected.Exit != nil && process.ExitCode != *req.Expected.Exit {
			fail("REUSE_ACCEPTED_PROCESS_MISMATCH")
		}
		if process != nil && process.StdoutBytes != uint64(len(req.Raw))+4 {
			fail("REUSE_ACCEPTED_TRANSPORT_MISMATCH")
		}
		if c.ExecutionStatus == StatusCompleted && !processComplete {
			fail("REUSE_ACCEPTED_PROCESS_MISMATCH")
		}
	}
	valid := make([]bool, len(versions))
	full := make([]bool, len(versions))
	complete := len(raw.Steps) == len(versions) && raw.Status == StatusCompleted && c.ExecutionStatus == StatusCompleted && processComplete
	var tally nativewire.Tally
	firstOmit := 0
	legacyCode := ""
	for k, s := range raw.Steps {
		if check != nil {
			if err := check(); err != nil {
				return res, err
			}
		}
		if k >= len(versions) {
			fail("REUSE_ACCEPTED_RAW_INVALID")
			continue
		}
		before := res.Result
		beforeFailures := failures
		// Inspect observation arguments independently of tree completeness and wire errors.
		observations, obsOK := acceptedObservations(s.Observations, k, e)
		if !obsOK {
			fail("REUSE_ACCEPTED_OBSERVATION_INVALID")
		}
		obsComplete := obsOK
		if omitControl && k > 0 && obsOK && !observations[0].OldTree && firstOmit == 0 {
			firstOmit = k
		}
		for _, o := range observations {
			if acceptedArgumentContradiction(o, omitControl) {
				fail("REUSE_ACCEPTED_PARSE_ARGUMENT_MISMATCH")
			}
			if o.Status != "COMPLETE" {
				obsComplete = false
			}
		}
		cs, wireErr := nativewire.CheckStep(k, s, e)
		if stopped != nil {
			return res, stopped
		}
		if wireErr != nil {
			fail("REUSE_ACCEPTED_RAW_INVALID")
		} else {
			if err := nativewire.CheckOpen(k, len(raw.Steps), raw.Status, cs.Done(k)); err != nil {
				fail("REUSE_ACCEPTED_RAW_INVALID")
			}
			tally.Add(k, cs)
		}
		allFull := wireErr == nil && cs.Incremental.Form == "full" && (k == 0 || cs.Fresh != nil && cs.Fresh.Form == "full")
		if wireErr == nil {
			for _, t := range []*nativewire.Tree{&cs.Incremental, cs.Fresh} {
				if t == nil || t.Form != "full" {
					continue
				}
				if !witnessCST(t.Nodes) {
					fail("REUSE_ACCEPTED_CST_INVALID")
				}
				pointsCode := witnessPointsChecked(t.Nodes, EncodingUTF8, versions[k], check)
				if stopped != nil {
					return res, stopped
				}
				if pointsCode != "" {
					fail("REUSE_ACCEPTED_CST_INVALID")
				}
			}
			if k > 0 && allFull && CompareTrees(cs.Incremental.Nodes, cs.Fresh.Nodes) != nil {
				fail("REUSE_ACCEPTED_CST_MISMATCH")
				if omitControl && legacyCode == "" {
					legacyCode = "INCREMENTAL_FRESH_MISMATCH_STEP_" + strconv.Itoa(k)
				}
			}
		}
		if req.Projection != nil && k < len(c.Steps) {
			p := c.Steps[k]
			if !acceptedStepProjection(p, cs, observations, wireErr == nil, e, req.Policy) {
				fail("REUSE_ACCEPTED_RAW_PROJECTION_MISMATCH")
			}
			if omitControl && k > 0 {
				r := s.Route
				proven := r != nil && r.EditHasChanges && r.ReusedNodes > 0 && r.FreshReusedNodes == 0
				v := routeStepClaim(proven, r != nil && r.EditHasChanges && r.ReusedNodes == 0 && r.FreshReusedNodes == 0, c.Steps[k-1].Incremental, p.Incremental)
				if r == nil {
					fail("REUSE_ACCEPTED_CONTROL_MISMATCH")
				}
				code := ""
				if v == claimFail {
					code = "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_" + strconv.Itoa(k)
				} else if v == claimBlocked && obsOK && reuseobs.CaseClaim([]reuseobs.Step{{Native: true, Edit: true, Observations: observations}}, true) == claimFail {
					code = "INCREMENTAL_ROUTE_OLD_TREE_UNVERIFIED_STEP_" + strconv.Itoa(k)
				}
				if legacyCode == "" {
					legacyCode = code
				}
			}
			if k > 0 && s.Route != nil {
				proven := s.Route != nil && s.Route.EditHasChanges && s.Route.ReusedNodes > 0 && s.Route.FreshReusedNodes == 0
				if p.Route == nil || p.Route.Proven != proven || s.Route == nil || !s.Route.EditHasChanges || s.Route.FreshReusedNodes != 0 {
					fail("REUSE_ACCEPTED_ROUTE_MISMATCH")
				}
				if allFull && k > 0 && full[k-1] && c.Steps[k-1].Incremental != nil && c.Steps[k-1].Incremental.Tree != nil && len(c.Steps[k-1].Incremental.Tree.Nodes) > 0 {
					oldEnd := c.Steps[k-1].Incremental.Tree.Nodes[0].EndByte
					ed := h.Edits[k-1]
					end := oldEnd
					if oldEnd >= ed.OldEndByte {
						end = oldEnd - ed.OldEndByte + ed.NewEndByte
					} else if oldEnd >= ed.StartByte {
						end = ed.NewEndByte
					}
					if s.Route != nil && s.Route.EditedRootEndByte != end {
						fail("REUSE_ACCEPTED_ROUTE_MISMATCH")
					}
				}
			}
		}
		complete = complete && obsComplete && allFull && wireErr == nil
		// Per-step proof does not erase later failures of the original history.
		valid[k], full[k] = obsComplete && allFull && wireErr == nil && k < len(c.Steps) && before != claimFail && res.Result != claimFail, allFull
		for i := range res.Steps {
			target := &res.Steps[i]
			if target.Step != k {
				continue
			}
			switch {
			case !omitControl && obsOK && len(observations) == 2 && observations[0].Status == "COMPLETE" && observations[0].Counts != nil && observations[0].Counts.Accepted == 0:
				if target.Result != claimFail {
					target.Result, target.Code = claimFail, "REUSE_ACCEPTED_ZERO"
				}
				fail("REUSE_ACCEPTED_ZERO")
			case failures > beforeFailures:
				target.Result, target.Code = claimFail, res.Code
			case valid[k] && k > 0 && valid[k-1] && req.Source != nil && req.Projection != nil:
				target.Result, target.Code = claimPass, ""
			}
		}
	}
	if rawOK {
		if err := nativewire.CheckEnd(raw, tally, e); err != nil {
			fail("REUSE_ACCEPTED_RAW_INVALID")
		}
	}
	if omitControl && req.Raw != nil && req.Projection != nil && complete {
		var legacy string
		if firstOmit != h.OmitOldFirst || c.ExecutionStatus != StatusCompleted || c.Assessment != AssessFail || c.Claims.IncrementalEquality != claimPass || c.Claims.IncrementalRoute != claimFail ||
			jsonv2.Unmarshal(c.ReuseObservationClaim, &legacy) != nil || legacy != claimFail || c.Code != legacyCode || legacyCode != "INCREMENTAL_ROUTE_OLD_TREE_UNVERIFIED_STEP_"+strconv.Itoa(h.OmitOldFirst) {
			fail("REUSE_ACCEPTED_CONTROL_MISMATCH")
		}
	}
	if req.Source != nil && req.Raw != nil && req.Projection != nil && complete && res.Result != claimFail && !slices.ContainsFunc(res.Steps, func(s ReuseAcceptedStep) bool { return s.Result != claimPass }) {
		res.Result, res.Code = claimPass, ""
	}
	if check != nil {
		if err := check(); err != nil {
			return res, err
		}
	}
	return res, nil
}

func acceptedRawShape(data []byte, check func() error) error {
	budget := &valueBudget{max: 2000000, check: func() *Error {
		if check != nil {
			if err := check(); err != nil {
				return asError(err)
			}
		}
		return nil
	}}
	v, e := decodeBounded("reuse-accepted-raw", data, NativeOperations()["native-parse-edit"].OutputBytes, budget)
	if e != nil {
		return e
	}
	shape := func(v *jv, typ reflect.Type) error {
		if v == nil {
			return fmt.Errorf("missing raw member")
		}
		if e := witnessShape(typed{doc: "reuse-accepted-raw"}, v, typ, false, false, budget); e != nil {
			return e
		}
		return nil
	}
	var header struct {
		Protocol  string           `json:"protocol"`
		ID        string           `json:"id"`
		Status    string           `json:"status"`
		Code      string           `json:"code"`
		Source    uint64           `json:"source_bytes"`
		Completed uint64           `json:"steps_completed"`
		Complete  bool             `json:"complete"`
		Steps     []jsontext.Value `json:"steps"`
		Producer  struct {
			Language    uint32         `json:"language_version"`
			Runtime     uint32         `json:"runtime_language_version"`
			Min         uint32         `json:"runtime_min_compatible"`
			Query       string         `json:"query"`
			Observation jsontext.Value `json:"reuse_observation"`
		} `json:"producer"`
	}
	if err := shape(v, reflect.TypeOf(header)); err != nil {
		return err
	}
	for _, s := range jvMember(v, "steps").vals {
		var step struct {
			Step         uint32               `json:"step"`
			Bytes        uint64               `json:"source_bytes"`
			SHA256       string               `json:"source_sha256"`
			Edit         *nativewire.StepEdit `json:"edit"`
			Route        *nativewire.Route    `json:"route"`
			Incremental  jsontext.Value       `json:"incremental"`
			Fresh        jsontext.Value       `json:"fresh"`
			Observations jsontext.Value       `json:"observations"`
		}
		if err := shape(s, reflect.TypeOf(step)); err != nil {
			return err
		}
		for _, slot := range []string{"incremental", "fresh"} {
			t := jvMember(s, slot)
			if t.kind == 'n' {
				continue
			}
			var base struct {
				Status string  `json:"status"`
				Code   string  `json:"code"`
				Form   *string `json:"form"`
			}
			if err := shape(t, reflect.TypeOf(base)); err != nil {
				return err
			}
			if form := jvMember(t, "form"); form.kind == '"' && form.s == "full" {
				var full struct {
					Count  uint64    `json:"descendant_count"`
					Error  bool      `json:"has_error"`
					Digest string    `json:"digest"`
					Types  []string  `json:"types"`
					Fields []string  `json:"fields"`
					Nodes  [][]int64 `json:"nodes"`
				}
				if err := shape(t, reflect.TypeOf(full)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func acceptedObservations(raw jsontext.Value, k int, e nativewire.Expect) ([]reuseobs.Observation, bool) {
	var values []jsontext.Value
	if e.Observation == nil || k >= len(e.Sources) || jsonv2.Unmarshal(raw, &values) != nil || len(values) != 1+min(k, 1) {
		return nil, false
	}
	out := make([]reuseobs.Observation, 0, len(values))
	for i, value := range values {
		role := "initial"
		if k > 0 {
			role = []string{"incremental", "fresh"}[i]
		}
		b := reuseobs.Binding{RuntimeManifestSHA256: e.Observation.Manifest, RequestID: e.ID, RunID: e.Observation.RunID, ProducerSHA256: e.Observation.ProducerSHA256, HistoryStart: e.Observation.HistoryStart, Step: uint64(k), Role: role, SourceSHA256: e.Sources[k].SHA256, SourceBytes: e.Sources[k].Bytes}
		if k > 0 {
			b.PreviousSourceSHA256 = e.Sources[k-1].SHA256
		}
		o, err := reuseobs.Decode(value, b)
		if err != nil {
			return out, false
		}
		out = append(out, o)
	}
	return out, true
}

func acceptedArgumentContradiction(o reuseobs.Observation, omitControl bool) bool {
	incremental := o.Role == "incremental" && !omitControl
	if o.Reason != "PARSE_NOT_RUN" && o.OldTree != incremental {
		return true
	}
	if o.Counts == nil {
		return o.Status == "COMPLETE"
	}
	c := o.Counts
	if c.ResumeParse > 0 || c.Done > 1 || c.NewParse > 1 || c.ParseAfterEdit > 1 ||
		incremental && c.NewParse > 0 || !incremental && (c.ParseAfterEdit > 0 || c.Accepted > 0) {
		return true
	}
	return o.Status == "COMPLETE" && (c.Done != 1 || incremental && (c.ParseAfterEdit != 1 || c.NewParse != 0) || !incremental && (c.NewParse != 1 || c.ParseAfterEdit != 0 || c.Accepted != 0))
}

func acceptedStepProjection(p rcStep, cs nativewire.CheckedStep, obs []reuseobs.Observation, checked bool, e nativewire.Expect, policy IdentityRef) bool {
	s := cs.Step
	if p.Step != s.Step || p.Restarted || p.Composite != nil || p.SourceBytes != s.SourceBytes || p.SourceSHA256 != s.SourceSHA256 {
		return false
	}
	var observations []reuseobs.Observation
	if jsonv2.Unmarshal(p.Observations, &observations) != nil || !reflect.DeepEqual(observations, obs) {
		return false
	}
	if (p.Edit == nil) != (s.Edit == nil) || (p.Route == nil) != (s.Route == nil) {
		return false
	}
	if s.Edit != nil && (p.Edit.StartByte == nil || p.Edit.OldEndByte == nil || p.Edit.NewEndByte == nil || *p.Edit.StartByte != s.Edit.StartByte || *p.Edit.OldEndByte != s.Edit.OldEndByte || *p.Edit.NewEndByte != s.Edit.NewEndByte || !slices.Equal(p.Edit.StartPoint, s.Edit.StartPoint[:]) || !slices.Equal(p.Edit.OldEndPoint, s.Edit.OldEndPoint[:]) || !slices.Equal(p.Edit.NewEndPoint, s.Edit.NewEndPoint[:])) {
		return false
	}
	if s.Route != nil && (p.Route.EditHasChanges != s.Route.EditHasChanges || p.Route.EditedRootEndByte != s.Route.EditedRootEndByte || p.Route.ReusedNodes != s.Route.ReusedNodes || p.Route.FreshReusedNodes != s.Route.FreshReusedNodes) {
		return false
	}
	if !checked {
		return true
	}
	tree := func(p *rcTree, t *nativewire.Tree) bool {
		if t == nil {
			return p == nil
		}
		if p == nil || p.Status != t.Status || p.Code != t.Code || p.Form != t.Form || p.DescendantCount != t.Wire.DescendantCount || p.HasError != t.Wire.HasError || p.Digest != t.Wire.Digest {
			return false
		}
		if t.Form != "full" {
			return p.Tree == nil
		}
		ids := []IdentityRef{{Role: "producer", Schema: "tsgk-native-build/r1", SHA256: e.Observation.ProducerSHA256}, {Role: "source", Schema: "tsgk-source-bytes/r1", SHA256: s.SourceSHA256}, policy}
		return p.Tree != nil && p.Tree.Schema == TreeSchema && p.Tree.Status == StatusCompleted && p.Tree.Input.Encoding == EncodingUTF8 && p.Tree.Input.Bytes == s.SourceBytes && p.Tree.Input.SHA256 == s.SourceSHA256 && reflect.DeepEqual(p.Tree.Identities, ids) && CompareTrees(p.Tree.Nodes, t.Nodes) == nil
	}
	if !tree(p.Incremental, &cs.Incremental) || !tree(p.Fresh, cs.Fresh) {
		return false
	}
	if s.Step > 0 && cs.Incremental.Form == "full" && cs.Fresh != nil && cs.Fresh.Form == "full" {
		d := CompareTrees(cs.Incremental.Nodes, cs.Fresh.Nodes)
		return p.Comparison != nil && p.Comparison.Equal == (d == nil) && reflect.DeepEqual(p.Comparison.First, d)
	}
	return p.Comparison == nil
}

func acceptedProjection(data []byte, c *rcCase, check func() error) error {
	if uint64(len(data)) > ReplayOperations()["evidence-replay"].FileBytes {
		return fail(KindResourceLimit, "REUSE_ACCEPTED_OUTPUT_LIMIT", "reuse-accepted-projection", nil)
	}
	budget := &valueBudget{max: 2000000, check: func() *Error {
		if check != nil {
			if err := check(); err != nil {
				return asError(err)
			}
		}
		return nil
	}}
	v, err := decodeBounded("reuse-accepted-projection", data, ReplayOperations()["evidence-replay"].FileBytes, budget)
	if err != nil {
		return err
	}
	// Check only the proof's projection fields; unrelated legacy extras remain data.
	shape := func(v *jv, typ reflect.Type) error {
		if v == nil {
			return fmt.Errorf("missing projection member")
		}
		if err := witnessShape(typed{doc: "reuse-accepted-projection"}, v, typ, false, false, budget); err != nil {
			return err
		}
		return nil
	}
	var header struct {
		ID             string           `json:"id"`
		Input          NativeInput      `json:"input"`
		Encoding       string           `json:"encoding"`
		Status         string           `json:"execution_status"`
		ResponseStatus string           `json:"response_status"`
		ResponseCode   string           `json:"response_code"`
		Producer       jsontext.Value   `json:"producer"`
		Process        jsontext.Value   `json:"process"`
		Steps          []jsontext.Value `json:"steps"`
	}
	if err := shape(v, reflect.TypeOf(header)); err != nil {
		return err
	}
	if process := jvMember(v, "process"); process.kind != 'n' {
		var processShape acceptedProcess
		if err := shape(process, reflect.TypeOf(processShape)); err != nil {
			return err
		}
	}
	steps := jvMember(v, "steps")
	if steps.kind != '[' {
		return fmt.Errorf("projection steps")
	}
	for _, s := range steps.vals {
		var stepShape struct {
			Step         uint32               `json:"step"`
			Bytes        uint64               `json:"source_bytes"`
			SHA256       string               `json:"source_sha256"`
			Observations jsontext.Value       `json:"observations"`
			Incremental  jsontext.Value       `json:"incremental"`
			Fresh        jsontext.Value       `json:"fresh"`
			Edit         *nativewire.StepEdit `json:"edit"`
			Route        *struct {
				Changes bool   `json:"edit_has_changes"`
				End     uint32 `json:"edited_root_end_byte"`
				Reused  uint64 `json:"reused_nodes"`
				Fresh   uint64 `json:"fresh_reused_nodes"`
				Proven  bool   `json:"proven"`
			} `json:"route"`
			Comparison *struct {
				Equal bool            `json:"equal"`
				First *TreeDifference `json:"first_difference"`
			} `json:"comparison"`
		}
		if err := shape(s, reflect.TypeOf(stepShape)); err != nil {
			return err
		}
		for _, slot := range []string{"incremental", "fresh"} {
			t := jvMember(s, slot)
			if t.kind == 'n' {
				continue
			}
			var treeShape struct {
				Status string `json:"status"`
				Code   string `json:"code"`
				Form   string `json:"form"`
				Count  uint64 `json:"descendant_count"`
				Error  bool   `json:"has_error"`
				Digest string `json:"digest"`
			}
			if err := shape(t, reflect.TypeOf(treeShape)); err != nil {
				return err
			}
			tree := jvMember(t, "tree")
			if tree != nil && tree.kind != 'n' {
				nodes := jvMember(tree, "nodes")
				if nodes == nil {
					return fmt.Errorf("missing nodes")
				}
				if err := witnessShape(typed{doc: "reuse-accepted-projection"}, nodes, reflect.TypeFor[[]TreeNode](), false, false, budget); err != nil {
					return err
				}
			}
		}
	}
	return jsonv2.Unmarshal(data, c)
}

func earlyProjectionAvailable(c rcCase) bool {
	var p *acceptedProcess
	return jsonv2.Unmarshal(c.Process, &p) == nil && p != nil && c.ExecutionStatus != StatusCompleted && (!p.Cleanup.Verified && c.Code == "PROCESS_CLEANUP_UNVERIFIED" ||
		p.Status == StatusResourceLimit && c.ExecutionStatus == StatusResourceLimit && c.Code == p.Reason ||
		p.Status == StatusCancelled && c.ExecutionStatus == StatusCancelled && c.Code == "CANCELLED")
}

func acceptedProducerEqual(a, b nativewire.Producer) bool {
	capability := func(raw jsontext.Value) (string, bool) {
		// The existing strict capability decoder rejects extra, duplicate and absent fields.
		m, ok := observationProducer(raw)
		return m, ok && m == reuseobs.RuntimeManifestSHA256
	}
	am, aok := capability(a.ReuseObservation)
	bm, bok := capability(b.ReuseObservation)
	a.ReuseObservation, b.ReuseObservation = nil, nil
	return aok && bok && am == bm && reflect.DeepEqual(a, b)
}
