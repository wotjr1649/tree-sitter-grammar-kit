package kit

import (
	"context"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

const (
	QueryWitnessRegistrationSchema = "tsgk-query-witness-registration/r1"
	QueryWitnessResultSchema       = "tsgk-query-witness-result/r1"
)

type QueryPredicate = querypolicy.Predicate
type QueryPredicateStep = querypolicy.Step

// QueryWitnessMetadata is independently registered compiled metadata. A producer's
// own metadata is compared to it, never used to replace a missing registration.
type QueryWitnessMetadata struct {
	Patterns     uint32           `json:"patterns"`
	CaptureNames []string         `json:"capture_names"`
	Predicates   []QueryPredicate `json:"predicates"`
}

// QueryWitnessQuery binds exact query bytes to independently selected metadata.
type QueryWitnessQuery struct {
	ID       string                `json:"id"`
	Source   string                `json:"source"`
	Metadata *QueryWitnessMetadata `json:"metadata"`
}

// QueryWitnessTuple fixes one obligation before results are read. Structural includes
// rejected matches. Nil anchors mean unavailable evidence; [] is a complete empty anchor.
type QueryWitnessTuple struct {
	Query      string    `json:"query"`
	Step       uint32    `json:"step"`
	TreeRole   string    `json:"tree_role"`
	Structural []Capture `json:"structural"`
	Expected   []Capture `json:"expected"`
}

// MarshalJSON preserves unavailable anchors under both JSON APIs; JSON/v2 otherwise
// emits nil slices as [], which would turn missing evidence into an empty expectation.
func (t QueryWitnessTuple) MarshalJSON() ([]byte, error) {
	type plain QueryWitnessTuple
	return jsonv2.Marshal(plain(t), jsonv2.FormatNilSliceAsNull(true))
}

// QueryWitnessRegistration is caller-trusted data outside the generated evidence set.
// Route, platform and role are part of the registration identity, not inferred from results.
type QueryWitnessRegistration struct {
	Schema   string              `json:"schema"`
	Policy   string              `json:"policy"`
	Case     string              `json:"case"`
	Route    string              `json:"route"`
	Platform string              `json:"platform"`
	Role     string              `json:"role"`
	Encoding string              `json:"encoding"`
	Input    NativeInput         `json:"input"`
	Edits    []Edit              `json:"edits"`
	Queries  []QueryWitnessQuery `json:"queries"`
	Required []QueryWitnessTuple `json:"required"`
}

// QueryWitnessRequest supplies bounded stored data, never executable code. Registration
// must be selected independently by the caller. Source is the initial source; edits are
// reapplied. Raw is the original r2 response, Projection the saved case projection.
type QueryWitnessRequest struct {
	Registration []byte
	Source       []byte
	Raw          []byte
	Projection   []byte
}

// QueryWitnessResult assesses stored capture evidence only. PASS does not establish
// producer execution, parser completeness on arbitrary input, or reference eligibility.
type QueryWitnessResult struct {
	Report
	ResultSchema       string `json:"result_schema"`
	RegistrationSHA256 string `json:"registration_sha256"`
	Required           int    `json:"required"`
	Verified           int    `json:"verified"`
	Evaluated          int    `json:"evaluated"`
}

type witnessRawTree struct {
	Status   string         `json:"status"`
	Code     string         `json:"code"`
	Form     *string        `json:"form"`
	Count    uint64         `json:"descendant_count"`
	HasError bool           `json:"has_error"`
	Digest   string         `json:"digest"`
	Types    []string       `json:"types"`  // witnessNullable
	Fields   []string       `json:"fields"` // witnessNullable
	Nodes    [][]int64      `json:"nodes"`  // witnessNullable
	Queries  jsontext.Value `json:"queries"`
}

type witnessEdit struct {
	StartByte   uint32    `json:"start_byte"`
	OldEndByte  uint32    `json:"old_end_byte"`
	NewEndByte  uint32    `json:"new_end_byte"`
	StartPoint  [2]uint32 `json:"start_point"`
	OldEndPoint [2]uint32 `json:"old_end_point"`
	NewEndPoint [2]uint32 `json:"new_end_point"`
}

type witnessRawStep struct {
	Step        uint32          `json:"step"`
	Bytes       uint64          `json:"source_bytes"`
	SHA256      string          `json:"source_sha256"`
	Edit        *witnessEdit    `json:"edit"`
	Incremental *witnessRawTree `json:"incremental"`
	Fresh       *witnessRawTree `json:"fresh"`
}

type witnessRaw struct {
	Protocol       string           `json:"protocol"`
	ID             string           `json:"id"`
	Status         string           `json:"status"`
	Code           string           `json:"code"`
	Bytes          uint64           `json:"source_bytes"`
	StepsCompleted uint64           `json:"steps_completed"`
	Complete       bool             `json:"complete"`
	Steps          []witnessRawStep `json:"steps"`
}

type witnessQueryProjection struct {
	ID           string                  `json:"id"`
	SHA256       string                  `json:"sha256"`
	Status       string                  `json:"status"`
	Code         string                  `json:"code"`
	Evaluation   string                  `json:"evaluation"`
	QueryMillis  uint64                  `json:"query_ms"`
	Error        *querywire.CompileError `json:"error"`
	Patterns     uint32                  `json:"patterns"`
	CaptureNames []string                `json:"capture_names"` // witnessNullable
	Predicates   []querypolicy.Predicate `json:"predicates"`    // witnessNullable
	Matches      uint64                  `json:"matches"`
	Partial      bool                    `json:"partial"`
	Captures     []Capture               `json:"captures"` // witnessNullable
}

type witnessTreeProjection struct {
	Status   string `json:"status"`
	Code     string `json:"code"`
	Form     string `json:"form"`
	Count    uint64 `json:"descendant_count"`
	HasError bool   `json:"has_error"`
	Digest   string `json:"digest"`
	Tree     *struct {
		Schema string     `json:"schema"`
		Input  rcInput    `json:"input"`
		Status string     `json:"status"`
		Nodes  []TreeNode `json:"nodes"`
	} `json:"tree"`
	Queries []witnessQueryProjection `json:"queries"`
}

type witnessProjectionStep struct {
	Step        uint32                 `json:"step"`
	Bytes       uint64                 `json:"source_bytes"`
	SHA256      string                 `json:"source_sha256"`
	Edit        *witnessEdit           `json:"edit"`
	Incremental *witnessTreeProjection `json:"incremental"`
	Fresh       *witnessTreeProjection `json:"fresh"`
}

type witnessProjection struct {
	ID       string                  `json:"id"`
	Input    NativeInput             `json:"input"`
	Encoding string                  `json:"encoding"`
	Status   string                  `json:"execution_status"`
	Code     string                  `json:"code"`
	Steps    []witnessProjectionStep `json:"steps"`
}

// VerifyQueryWitness recomputes raw -> evaluated -> independently expected full capture
// streams with the shared predicate policy and comparators. It starts no processes.
func VerifyQueryWitness(ctx context.Context, req QueryWitnessRequest) (QueryWitnessResult, error) {
	r, e := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		res := QueryWitnessResult{Report: newReport("query witness"), ResultSchema: QueryWitnessResultSchema}
		failReport(&res.Report, e)
		return res, e
	}
	defer r.cancel()
	return verifyQueryWitness(r, req)
}

func verifyQueryWitness(r *run, req QueryWitnessRequest) (res QueryWitnessResult, resultErr error) {
	res = QueryWitnessResult{Report: newReport("query witness"), ResultSchema: QueryWitnessResultSchema}
	defer func() {
		if resultErr != nil {
			var e *Error
			if errors.As(resultErr, &e) && e != nil {
				failReport(&res.Report, e)
			}
		}
	}()
	reg, e := parseQueryWitnessRegistration(req.Registration, r)
	if e != nil {
		return res, e
	}
	res.RegistrationSHA256 = digestHex(req.Registration)
	res.ExecutionStatus, res.EvidenceMode, res.Assessment = StatusCompleted, ModeReplayedRaw, AssessPass
	res.Required = len(reg.Required)
	contradictions := 0
	caseBound := true
	badSteps := map[int]bool{}
	bad := func(code, path string) {
		contradictions++
		if len(res.Findings) < 1000 {
			res.Findings = append(res.Findings, Finding{Severity: "error", Code: code, Path: path, Message: "저장된 query 근거가 독립 등록 또는 재계산과 다르다"})
		}
		res.Assessment = AssessFail
	}
	blocked := func(code, path string) {
		if res.Assessment != AssessFail {
			res.Assessment = AssessBlocked
		}
		if len(res.Findings) < 1000 {
			res.Findings = append(res.Findings, Finding{Severity: "warning", Code: code, Path: path, Message: "필수 query 근거를 확보하지 못했다"})
		}
	}
	if len(reg.Required) == 0 {
		blocked("QUERY_WITNESS_REQUIRED_ZERO", "registration")
	}
	if req.Source == nil {
		blocked("QUERY_WITNESS_SOURCE_MISSING", "source")
	}
	if uint64(len(req.Source)) > NativeOperations()["native-query-large"].InputBytes {
		return res, fail(KindResourceLimit, "INPUT_TOO_LARGE", "source", nil)
	}
	if req.Source != nil && (uint64(len(req.Source)) != reg.Input.Bytes || digestHex(req.Source) != reg.Input.SHA256) {
		bad("QUERY_WITNESS_SOURCE_MISMATCH", "source")
	}
	var versions [][]byte
	var points []EditPoints
	if req.Source != nil && res.Assessment != AssessFail {
		var err error
		versions, points, err = ApplyEdits(reg.Encoding, req.Source, reg.Edits, NativeOperations()["native-query-large"].InputBytes)
		if err != nil {
			bad("QUERY_WITNESS_EDITS_INVALID", "source")
		}
	}
	queryIndices := map[string]int{}
	sources := make([]querywire.Source, len(reg.Queries))
	for i, q := range reg.Queries {
		queryIndices[q.ID], sources[i] = i, querywire.Source{ID: q.ID, Source: []byte(q.Source)}
	}
	var raw witnessRaw
	var projection witnessProjection
	rawQueries := map[*witnessRawTree][]querywire.Query{}
	haveRaw, haveProjection := req.Raw != nil, req.Projection != nil
	if haveRaw {
		if e := decodeWitnessDocument("raw", req.Raw, &raw, false, r); e != nil {
			if e.Kind != KindInvalidInput {
				return res, e
			}
			bad("QUERY_WITNESS_RAW_INVALID", "raw")
			return res, nil
		}
		if raw.Protocol != "tsgk-native/r2" || raw.ID != reg.Case || raw.Bytes != reg.Input.Bytes || !raw.Complete || raw.StepsCompleted != uint64(len(raw.Steps)) || len(raw.Steps) > len(reg.Edits)+1 ||
			(raw.Status == StatusCompleted && (raw.Code != "" || len(raw.Steps) != len(reg.Edits)+1)) || !slices.Contains([]string{StatusCompleted, StatusFailed, StatusResourceLimit}, raw.Status) {
			bad("QUERY_WITNESS_RAW_ENVELOPE", "raw")
			caseBound = false
		}
		if raw.Status != StatusCompleted {
			blocked("QUERY_WITNESS_RAW_INCOMPLETE", "raw")
		}
		for i, s := range raw.Steps {
			if !witnessRegisteredStep(i, s.Step, s.Bytes, s.SHA256, s.Edit, reg) || (len(versions) != 0 && !witnessStepBinding(i, s.Step, s.Bytes, s.SHA256, s.Edit, versions, points, reg.Edits)) {
				bad("QUERY_WITNESS_STEP_MISMATCH", fmt.Sprintf("raw/steps/%d", i))
				badSteps[i] = true
			}
			for _, tree := range []*witnessRawTree{s.Incremental, s.Fresh} {
				if tree != nil && !witnessRawTreeHeader(*tree) {
					bad("QUERY_WITNESS_TREE_INVALID", fmt.Sprintf("raw/steps/%d", i))
				} else if tree != nil && tree.Status == StatusCompleted {
					qs, qe := querywire.DecodeSummary(tree.Queries, sources, s.Bytes, tree.Count)
					if qe != nil {
						bad(qe.Code, fmt.Sprintf("raw/steps/%d", i))
					} else {
						rawQueries[tree] = qs
						for qi, q := range qs {
							meta := reg.Queries[qi].Metadata
							if meta != nil && (q.Patterns != meta.Patterns || !slices.Equal(q.CaptureNames, meta.CaptureNames) || !reflect.DeepEqual(q.Predicates, meta.Predicates)) {
								bad("QUERY_WITNESS_METADATA_MISMATCH", fmt.Sprintf("raw/steps/%d", i))
							}
						}
					}
				}
			}
		}
	} else {
		blocked("QUERY_WITNESS_RAW_MISSING", "raw")
	}
	if haveProjection {
		if e := decodeWitnessDocument("projection", req.Projection, &projection, false, r); e != nil {
			if e.Kind != KindInvalidInput {
				return res, e
			}
			bad("QUERY_WITNESS_PROJECTION_INVALID", "projection")
			return res, nil
		}
		if projection.ID != reg.Case || projection.Input != reg.Input || projection.Encoding != reg.Encoding || len(projection.Steps) > len(reg.Edits)+1 ||
			(projection.Status == StatusCompleted && len(projection.Steps) != len(reg.Edits)+1) || !slices.Contains([]string{StatusCompleted, StatusFailed, StatusResourceLimit, StatusNotRun, StatusCancelled}, projection.Status) {
			bad("QUERY_WITNESS_PROJECTION_ENVELOPE", "projection")
			caseBound = false
		}
		if projection.Status != StatusCompleted {
			blocked("QUERY_WITNESS_PROJECTION_INCOMPLETE", "projection")
		}
		for i, s := range projection.Steps {
			if !witnessRegisteredStep(i, s.Step, s.Bytes, s.SHA256, s.Edit, reg) || (len(versions) != 0 && !witnessStepBinding(i, s.Step, s.Bytes, s.SHA256, s.Edit, versions, points, reg.Edits)) {
				bad("QUERY_WITNESS_STEP_MISMATCH", fmt.Sprintf("projection/steps/%d", i))
				badSteps[i] = true
			}
		}
	} else {
		blocked("QUERY_WITNESS_PROJECTION_MISSING", "projection")
	}
	// Check every available binding before requiring source bytes or a full CST.
	for i, step := range projection.Steps {
		for role, tree := range []*witnessTreeProjection{step.Incremental, step.Fresh} {
			if tree == nil {
				continue
			}
			if !witnessProjectionTreeHeader(*tree) {
				bad("QUERY_WITNESS_TREE_PROJECTION_INVALID", fmt.Sprintf("projection/steps/%d", i))
			}
			if len(tree.Queries) != len(reg.Queries) {
				bad("QUERY_WITNESS_QUERY_SET_MISMATCH", fmt.Sprintf("projection/steps/%d", i))
				continue
			}
			if i < len(raw.Steps) {
				rawTree := raw.Steps[i].Incremental
				if role == 1 {
					rawTree = raw.Steps[i].Fresh
				}
				if qs, ok := rawQueries[rawTree]; ok && len(qs) == len(tree.Queries) {
					for qi, q := range qs {
						if !witnessQueryPassthrough(tree.Queries[qi], q, reg.Encoding) {
							bad("QUERY_WITNESS_QUERY_PASSTHROUGH_MISMATCH", fmt.Sprintf("projection/steps/%d", i))
						}
					}
				}
			}
			for qi, q := range tree.Queries {
				if q.ID != reg.Queries[qi].ID || q.SHA256 != digestHex([]byte(reg.Queries[qi].Source)) {
					bad("QUERY_WITNESS_QUERY_IDENTITY_MISMATCH", fmt.Sprintf("projection/steps/%d", i))
				}
				meta := reg.Queries[qi].Metadata
				if !witnessProjectionQueryHeader(q, reg.Encoding) {
					bad("QUERY_WITNESS_QUERY_PROJECTION_INVALID", fmt.Sprintf("projection/steps/%d", i))
				}
				if meta != nil && q.Status == StatusCompleted && (q.Patterns != meta.Patterns || !slices.Equal(q.CaptureNames, meta.CaptureNames) || !reflect.DeepEqual(q.Predicates, meta.Predicates)) {
					bad("QUERY_WITNESS_METADATA_MISMATCH", fmt.Sprintf("projection/steps/%d", i))
				}
			}
		}
	}
	for i, tuple := range reg.Required {
		if e := r.check(); e != nil {
			return res, e
		}
		path := fmt.Sprintf("required/%d", i)
		contradictionsBefore := contradictions
		qi := queryIndices[tuple.Query]
		registeredQuery := reg.Queries[qi]
		meta := registeredQuery.Metadata
		if meta == nil {
			blocked("QUERY_WITNESS_METADATA_MISSING", path)
		}
		if tuple.Structural == nil {
			blocked("QUERY_WITNESS_STRUCTURAL_ANCHOR_MISSING", path)
		}
		if tuple.Expected == nil {
			blocked("QUERY_WITNESS_EXPECTED_MISSING", path)
		}
		step := int(tuple.Step)
		var p *witnessTreeProjection
		if haveProjection && step < len(projection.Steps) {
			p = projection.Steps[step].Incremental
			if tuple.TreeRole == "fresh" {
				p = projection.Steps[step].Fresh
			}
			if p != nil && qi < len(p.Queries) {
				q := p.Queries[qi]
				if q.Status == StatusCompleted && !q.Partial && q.Captures != nil && tuple.Expected != nil && CompareCaptures(q.Captures, tuple.Expected) != nil {
					bad("QUERY_WITNESS_EXPECTED_PROJECTION_MISMATCH", path)
				}
			}
		}
		if !haveRaw || step >= len(raw.Steps) {
			blocked("QUERY_WITNESS_TUPLE_MISSING", path)
			continue
		}
		tree := raw.Steps[step].Incremental
		if tuple.TreeRole == "fresh" {
			tree = raw.Steps[step].Fresh
		}
		if tree == nil || tree.Status != StatusCompleted || tree.Form == nil || *tree.Form != "full" {
			blocked("QUERY_WITNESS_FULL_TREE_MISSING", path)
			continue
		}
		var source []byte
		sourceBytes := raw.Steps[step].Bytes
		if len(versions) != 0 {
			source, sourceBytes = versions[step], uint64(len(versions[step]))
		}
		nodes, code := witnessNodes(*tree, reg.Encoding, source, sourceBytes, r)
		if e := r.check(); e != nil {
			return res, e
		}
		if code != "" {
			bad(code, path)
			continue
		}
		queries, qe := querywire.DecodeFull(tree.Queries, sources, sourceBytes, uint64(len(nodes)), func(q querywire.Query, row []int64) bool {
			c := witnessCapture(q, row)
			n := nodes[row[3]]
			return c.Type == n.Type && c.Named == n.Named && c.Extra == n.Extra && c.IsError == n.IsError && c.HasError == n.HasError && c.IsMissing == n.IsMissing && c.StartByte == n.StartByte && c.EndByte == n.EndByte && c.StartPoint == n.StartPoint && c.EndPoint == n.EndPoint
		})
		if qe != nil {
			bad(qe.Code, path)
			continue
		}
		q := queries[qi]
		if meta != nil && (meta.Patterns != q.Patterns || !slices.Equal(meta.CaptureNames, q.CaptureNames) || !reflect.DeepEqual(meta.Predicates, q.Predicates)) {
			bad("QUERY_WITNESS_METADATA_MISMATCH", path)
		}
		structural := make([]Capture, 0, len(q.Captures))
		views := make([]querypolicy.Capture, 0, len(q.Captures))
		for _, row := range q.Captures {
			c := witnessCapture(q, row)
			structural = append(structural, c)
			views = append(views, querypolicy.Capture{Match: c.Match, Pattern: c.Pattern, Name: c.Name, StartByte: c.StartByte, EndByte: c.EndByte})
		}
		if q.Status == StatusCompleted && !q.Partial && tuple.Structural != nil && CompareCaptures(structural, tuple.Structural) != nil {
			bad("QUERY_WITNESS_STRUCTURAL_MISMATCH", path)
		}
		if p == nil {
			blocked("QUERY_WITNESS_TUPLE_MISSING", path)
		} else if p.Tree == nil || p.Form != "full" {
			blocked("QUERY_WITNESS_FULL_TREE_MISSING", path)
		} else {
			if p.Status != tree.Status || p.Code != tree.Code || p.Count != tree.Count || p.HasError != tree.HasError || p.Digest != tree.Digest || p.Tree.Schema != TreeSchema || p.Tree.Status != StatusCompleted ||
				p.Tree.Input.Encoding != reg.Encoding || p.Tree.Input.Bytes != sourceBytes || p.Tree.Input.SHA256 != raw.Steps[step].SHA256 || CompareTrees(nodes, p.Tree.Nodes) != nil {
				bad("QUERY_WITNESS_TREE_PROJECTION_MISMATCH", path)
			}
			if len(p.Queries) != len(queries) || qi >= len(p.Queries) {
				bad("QUERY_WITNESS_QUERY_SET_MISMATCH", path)
			}
		}
		if source == nil {
			blocked("QUERY_WITNESS_SOURCE_MISSING", path)
			continue
		}
		indices, reason, filterErr := querypolicy.FilterChecked(func() error {
			if e := r.check(); e != nil {
				return e
			}
			return nil
		}, q.Patterns, q.Predicates, views, reg.Encoding, source)
		if filterErr != nil {
			return res, filterErr
		}
		status, evaluation, evaluationCode := q.Status, "STRUCTURAL", q.Code
		var evaluated []Capture
		switch {
		case q.Captures == nil:
			evaluation = ""
		case reason != "" && reason != querypolicy.AbsentCapture && reason != querypolicy.AmbiguousCapture && reason != querypolicy.InvalidCapture && reason != querypolicy.InvalidMetadata:
			status, evaluation, evaluationCode = "UNSUPPORTED", "UNSUPPORTED", reason
		case len(q.Predicates) > 0 && q.Partial:
			evaluation = "UNSUPPORTED"
		case reason != "":
			status, evaluation, evaluationCode = StatusFailed, "UNSUPPORTED", reason
		default:
			evaluated = make([]Capture, 0, len(indices))
			for _, index := range indices {
				evaluated = append(evaluated, structural[index])
			}
			if len(q.Predicates) > 0 {
				evaluation = "EVALUATED"
			}
		}
		if q.Status == StatusCompleted && !q.Partial && tuple.Expected != nil && evaluated != nil && CompareCaptures(evaluated, tuple.Expected) != nil {
			bad("QUERY_WITNESS_EXPECTED_MISMATCH", path)
		}
		projectionOK := false
		if p != nil && p.Tree != nil && p.Form == "full" && len(p.Queries) == len(queries) && qi < len(p.Queries) {
			v := p.Queries[qi]
			projectionOK = witnessProjectionQueryHeader(v, reg.Encoding) && witnessQueryPassthrough(v, q, reg.Encoding) && v.SHA256 == digestHex([]byte(registeredQuery.Source)) && v.Status == status && v.Code == evaluationCode && v.Evaluation == evaluation &&
				(v.Captures == nil) == (evaluated == nil) && CompareCaptures(v.Captures, evaluated) == nil
			if !projectionOK {
				bad("QUERY_WITNESS_QUERY_PROJECTION_MISMATCH", path)
			}
		}
		if status != StatusCompleted || q.Partial || evaluated == nil {
			blocked("QUERY_WITNESS_QUERY_INELIGIBLE", path)
		} else if caseBound && !badSteps[step] && contradictions == contradictionsBefore && meta != nil && tuple.Structural != nil && tuple.Expected != nil && projectionOK {
			res.Verified++
			if evaluation == "EVALUATED" {
				res.Evaluated++
			}
		}
	}
	if e := r.check(); e != nil {
		return res, e
	}
	return res, nil
}

// Native query projection copies these fields without evaluating source text.
func witnessQueryPassthrough(v witnessQueryProjection, q querywire.Query, encoding string) bool {
	predicates := q.Predicates
	if predicates == nil {
		predicates = []querypolicy.Predicate{} // queryOuts preserves a present empty metadata list
	}
	if v.ID != q.ID || v.QueryMillis != q.QueryMillis || !reflect.DeepEqual(v.Error, q.Error) || v.Patterns != q.Patterns ||
		(v.CaptureNames == nil) != (q.CaptureNames == nil) || !slices.Equal(v.CaptureNames, q.CaptureNames) || !reflect.DeepEqual(v.Predicates, predicates) || v.Matches != q.Matches || v.Partial != q.Partial {
		return false
	}
	status, code, evaluation := q.Status, q.Code, ""
	var captures []Capture
	switch {
	case q.Captures == nil:
	case len(q.Predicates) == 0:
		evaluation, captures = "STRUCTURAL", make([]Capture, 0, len(q.Captures))
		for _, row := range q.Captures {
			captures = append(captures, witnessCapture(q, row))
		}
	case querypolicy.Support(q.Patterns, q.Predicates, encoding) != "":
		status, code, evaluation = "UNSUPPORTED", querypolicy.Support(q.Patterns, q.Predicates, encoding), "UNSUPPORTED"
	case q.Partial:
		evaluation = "UNSUPPORTED"
	default:
		// Only predicate evaluation can change this completed raw query's state.
		return (v.Status == StatusCompleted && v.Code == "" && v.Evaluation == "EVALUATED" && v.Captures != nil) ||
			(v.Status == StatusFailed && slices.Contains([]string{querypolicy.InvalidMetadata, querypolicy.InvalidCapture, querypolicy.AbsentCapture, querypolicy.AmbiguousCapture}, v.Code) && v.Evaluation == "UNSUPPORTED" && v.Captures == nil)
	}
	return v.Status == status && v.Code == code && v.Evaluation == evaluation && (v.Captures == nil) == (captures == nil) && CompareCaptures(v.Captures, captures) == nil
}

func witnessRawTreeHeader(t witnessRawTree) bool {
	switch t.Status {
	case StatusCompleted:
		return t.Code == "" && t.Form != nil && slices.Contains([]string{"full", "summary", "record"}, *t.Form) && t.Count > 0 && hex64.MatchString(t.Digest)
	case StatusResourceLimit, StatusFailed:
		return t.Code != "" && t.Form == nil && t.Nodes == nil && t.Queries.Kind() == 'n'
	default:
		return false
	}
}

func witnessProjectionTreeHeader(t witnessTreeProjection) bool {
	switch t.Status {
	case StatusCompleted:
		return t.Code == "" && slices.Contains([]string{"full", "summary", "record"}, t.Form) && t.Count > 0 && hex64.MatchString(t.Digest)
	case StatusResourceLimit, StatusFailed:
		return t.Code != "" && t.Form == "" && t.Tree == nil
	default:
		return false
	}
}

func witnessProjectionQueryHeader(q witnessQueryProjection, encoding string) bool {
	switch q.Status {
	case StatusCompleted:
		return q.Code == "" && q.Error == nil && !q.Partial && q.Captures != nil && q.CaptureNames != nil && q.Predicates != nil && querypolicy.Support(q.Patterns, q.Predicates, encoding) == "" &&
			((len(q.Predicates) == 0 && q.Evaluation == "STRUCTURAL") || (len(q.Predicates) != 0 && q.Evaluation == "EVALUATED"))
	case "UNSUPPORTED":
		reason := querypolicy.Support(q.Patterns, q.Predicates, encoding)
		return reason != "" && q.Code == reason && q.Error == nil && q.Captures == nil && q.Evaluation == "UNSUPPORTED"
	case StatusResourceLimit:
		return slices.Contains([]string{"QUERY_TIME_LIMIT", "MATCH_LIMIT", "CAPTURE_LIMIT", "QUERY_MATCH_OVERFLOW"}, q.Code) && q.Error == nil && q.Partial &&
			((len(q.Predicates) == 0 && q.Evaluation == "STRUCTURAL" && q.Captures != nil) || (len(q.Predicates) != 0 && q.Evaluation == "UNSUPPORTED" && q.Captures == nil))
	case StatusFailed:
		return q.Code != "" && q.Error == nil && !q.Partial && q.Captures == nil && slices.Contains([]string{"", "UNSUPPORTED"}, q.Evaluation)
	case "INVALID_QUERY":
		return q.Error != nil && q.Code == "QUERY_"+q.Error.Type && !q.Partial && q.Captures == nil && q.Evaluation == "" && q.Patterns == 0
	case StatusNotRun:
		return q.Code == "TREE_NOT_COMPLETED" && q.Error == nil && !q.Partial && q.Captures == nil && q.Evaluation == "" && q.Patterns == 0
	default:
		return false
	}
}

// Byte counts, initial identity and edit byte ranges are known before source replay.
func witnessRegisteredStep(index int, step uint32, size uint64, sha string, edit *witnessEdit, reg QueryWitnessRegistration) bool {
	if int(step) != index || index > len(reg.Edits) {
		return false
	}
	n := reg.Input.Bytes
	for _, e := range reg.Edits[:index] {
		if e.StartByte > e.OldEndByte || uint64(e.OldEndByte) > n || e.NewEndByte < e.StartByte {
			return false
		}
		n = n - uint64(e.OldEndByte-e.StartByte) + uint64(e.NewEndByte-e.StartByte)
	}
	if size != n || !hex64.MatchString(sha) {
		return false
	}
	if index == 0 {
		return sha == reg.Input.SHA256 && edit == nil
	}
	e := reg.Edits[index-1]
	return edit != nil && edit.StartByte == e.StartByte && edit.OldEndByte == e.OldEndByte && edit.NewEndByte == e.NewEndByte
}

func witnessStepBinding(index int, step uint32, size uint64, sha string, edit *witnessEdit, versions [][]byte, points []EditPoints, edits []Edit) bool {
	if int(step) != index || index >= len(versions) || size != uint64(len(versions[index])) || sha != digestHex(versions[index]) {
		return false
	}
	if index == 0 {
		return edit == nil
	}
	if edit == nil {
		return false
	}
	e, p := edits[index-1], points[index-1]
	return edit.StartByte == e.StartByte && edit.OldEndByte == e.OldEndByte && edit.NewEndByte == e.NewEndByte && edit.StartPoint == [2]uint32{p.Start.Row, p.Start.Column} && edit.OldEndPoint == [2]uint32{p.OldEnd.Row, p.OldEnd.Column} && edit.NewEndPoint == [2]uint32{p.NewEnd.Row, p.NewEnd.Column}
}

func witnessNodes(t witnessRawTree, encoding string, source []byte, sourceBytes uint64, r *run) ([]TreeNode, string) {
	if t.Code != "" || t.Types == nil || t.Fields == nil || t.Nodes == nil || t.Count != uint64(len(t.Nodes)) || len(t.Nodes) == 0 {
		return nil, "QUERY_WITNESS_TREE_INVALID"
	}
	for _, name := range append(slices.Clone(t.Types), t.Fields...) {
		if name == "" {
			return nil, "QUERY_WITNESS_TREE_INVALID"
		}
	}
	nodes := make([]TreeNode, 0, len(t.Nodes))
	for i, row := range t.Nodes {
		if i%1024 == 0 && r.check() != nil {
			return nil, ""
		}
		if len(row) != 10 || row[1] < 0 || row[1] >= int64(len(t.Types)) || row[2] < -1 || row[2] >= int64(len(t.Fields)) || row[3] < 0 || row[3] > 31 {
			return nil, "QUERY_WITNESS_TREE_INVALID"
		}
		for _, v := range row[4:] {
			if v < 0 || v > 0xffffffff {
				return nil, "QUERY_WITNESS_TREE_INVALID"
			}
		}
		n := TreeNode{Parent: row[0], Type: t.Types[row[1]], Named: row[3]&1 != 0, Extra: row[3]&2 != 0, IsError: row[3]&4 != 0, HasError: row[3]&8 != 0, IsMissing: row[3]&16 != 0,
			StartByte: uint32(row[4]), EndByte: uint32(row[5]), StartPoint: Point{Row: uint32(row[6]), Column: uint32(row[7])}, EndPoint: Point{Row: uint32(row[8]), Column: uint32(row[9])}}
		if row[2] >= 0 {
			if n.Parent == -1 {
				return nil, "QUERY_WITNESS_TREE_INVALID"
			}
			f := t.Fields[row[2]]
			n.Field = &f
		}
		nodes = append(nodes, n)
	}
	if ValidateTree(nodes, sourceBytes) != nil || nodes[0].HasError != t.HasError || TreeDigest(nodes) != t.Digest {
		return nil, "QUERY_WITNESS_TREE_INVALID"
	}
	if source == nil {
		return nodes, ""
	}
	offsets := make([]uint32, 0, len(nodes)*2)
	for _, n := range nodes {
		offsets = append(offsets, n.StartByte, n.EndByte)
	}
	slices.Sort(offsets)
	offsets = slices.Compact(offsets)
	positions := make(map[uint32]Point, len(offsets))
	previous, position := uint32(0), (Point{})
	for i, offset := range offsets {
		if i%1024 == 0 && r.check() != nil {
			return nil, ""
		}
		if (encoding == EncodingUTF16LE || encoding == EncodingUTF16BE) && offset%2 != 0 {
			return nil, "QUERY_WITNESS_TREE_POINTS"
		}
		part := PointAt(encoding, source[previous:offset], int(offset-previous))
		position.Row += part.Row
		if part.Row != 0 {
			position.Column = part.Column
		} else {
			position.Column += part.Column
		}
		positions[offset], previous = position, offset
	}
	for _, n := range nodes {
		if positions[n.StartByte] != n.StartPoint || positions[n.EndByte] != n.EndPoint {
			return nil, "QUERY_WITNESS_TREE_POINTS"
		}
	}
	return nodes, ""
}

func witnessCapture(q querywire.Query, row []int64) Capture {
	f := row[5]
	return Capture{Match: uint32(row[0]), Pattern: uint32(row[1]), Capture: uint32(row[2]), Name: q.CaptureNames[row[2]], Node: row[3], Type: q.Types[row[4]], Named: f&1 != 0, Extra: f&2 != 0, IsError: f&4 != 0, HasError: f&8 != 0, IsMissing: f&16 != 0,
		StartByte: uint32(row[6]), EndByte: uint32(row[7]), StartPoint: Point{Row: uint32(row[8]), Column: uint32(row[9])}, EndPoint: Point{Row: uint32(row[10]), Column: uint32(row[11])}}
}

func parseQueryWitnessRegistration(data []byte, r *run) (QueryWitnessRegistration, *Error) {
	var reg QueryWitnessRegistration
	if e := decodeWitnessDocument("query-registration", data, &reg, true, r); e != nil {
		return reg, e
	}
	bad := func(code string) (QueryWitnessRegistration, *Error) {
		return reg, fail(KindInvalidInput, code, "query-registration", nil)
	}
	if reg.Schema != QueryWitnessRegistrationSchema {
		return bad("SCHEMA_UNSUPPORTED")
	}
	if reg.Policy != querypolicy.Revision {
		return bad("QUERY_REGISTRATION_POLICY_UNSUPPORTED")
	}
	if !validID(reg.Case) || !validID(reg.Route) || !validID(reg.Role) || !slices.Contains([]string{"windows/amd64", "linux/amd64", "darwin/arm64"}, reg.Platform) || !portable(reg.Input.Path, false) || !hex64.MatchString(reg.Input.SHA256) || !encodingNames[reg.Encoding] || len(reg.Edits) > 4 || len(reg.Queries) > MaxQueries || len(reg.Required) > 1000 {
		return bad("QUERY_REGISTRATION_INVALID")
	}
	queries := map[string]bool{}
	for _, q := range reg.Queries {
		if !validID(q.ID) || queries[q.ID] || q.Source == "" || len(q.Source) > MaxQueryBytes {
			return bad("QUERY_REGISTRATION_QUERY_INVALID")
		}
		queries[q.ID] = true
	}
	seen := map[string]bool{}
	for _, tuple := range reg.Required {
		key := fmt.Sprintf("%s/%d/%s", tuple.Query, tuple.Step, tuple.TreeRole)
		if !queries[tuple.Query] || int(tuple.Step) > len(reg.Edits) || !slices.Contains([]string{"incremental", "fresh"}, tuple.TreeRole) || tuple.Step == 0 && tuple.TreeRole == "fresh" || seen[key] {
			return bad("QUERY_REGISTRATION_TUPLE_INVALID")
		}
		seen[key] = true
	}
	return reg, nil
}

func decodeWitnessDocument(doc string, data []byte, target any, exact bool, r *run) *Error {
	v, e := decodeBounded(doc, data, MaxDocumentBytes, &valueBudget{max: 2000000, check: r.check})
	if e != nil {
		return e
	}
	if e := witnessShape(typed{doc: doc}, v, reflect.TypeOf(target).Elem(), exact, false, &valueBudget{max: 2000000, check: r.check}); e != nil {
		return e
	}
	if err := jsonv2.Unmarshal(data, target); err != nil {
		return fail(KindInvalidInput, "JSON_SHAPE", doc, err)
	}
	return nil
}

// New witness documents check presence and primitive types before typed decoding can
// turn null or absent members into zero values. Partial legacy projections allow extras.
func witnessShape(t typed, v *jv, typ reflect.Type, exact, nullable bool, budget *valueBudget) *Error {
	if e := budget.take(t.doc, v); e != nil {
		return e
	}
	if typ == reflect.TypeFor[jsontext.Value]() {
		return nil
	}
	if typ.Kind() == reflect.Pointer {
		if v.kind == 'n' {
			return nil
		}
		return witnessShape(t, v, typ.Elem(), exact, false, budget)
	}
	if nullable && v.kind == 'n' {
		return nil
	}
	switch typ.Kind() {
	case reflect.Struct:
		if e := t.want(v, '{'); e != nil {
			return e
		}
		fields := map[string]reflect.StructField{}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			name := f.Tag.Get("json")
			if j := strings.IndexByte(name, ','); j >= 0 {
				name = name[:j]
			}
			if name != "" && name != "-" {
				fields[name] = f
			}
		}
		for i, name := range v.keys {
			f, ok := fields[name]
			if !ok {
				if exact {
					return t.bad("JSON_UNKNOWN_FIELD", v.vals[i])
				}
				continue
			}
			allowNull := f.Type.Kind() == reflect.Slice && (name == "structural" || name == "expected" || name == "captures" || name == "capture_names" || name == "predicates" || name == "types" || name == "fields" || name == "nodes")
			// Registered metadata arrays, full CST nodes and capture fields are never nullable.
			if typ == reflect.TypeFor[QueryWitnessMetadata]() || typ == reflect.TypeFor[QueryWitnessRegistration]() {
				allowNull = false
			}
			if e := witnessShape(t, v.vals[i], f.Type, exact, allowNull, budget); e != nil {
				return e
			}
		}
		for name, f := range fields {
			if jvMember(v, name) == nil && !slices.Contains([]string{"encoding_source"}, name) && f.Tag.Get("json") != "-" {
				return fail(KindInvalidInput, "JSON_MISSING_FIELD", t.doc+"#"+v.cptr()+"/"+name, nil)
			}
		}
	case reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			return t.want(v, '"')
		}
		if e := t.want(v, '['); e != nil {
			return e
		}
		for _, item := range v.vals {
			if e := witnessShape(t, item, typ.Elem(), exact, false, budget); e != nil {
				return e
			}
		}
	case reflect.Array:
		if e := t.want(v, '['); e != nil {
			return e
		}
		if len(v.vals) != typ.Len() {
			return t.bad("JSON_SHAPE", v)
		}
		for _, item := range v.vals {
			if e := witnessShape(t, item, typ.Elem(), exact, false, budget); e != nil {
				return e
			}
		}
	case reflect.String:
		return t.want(v, '"')
	case reflect.Bool:
		_, e := t.boolean(v)
		return e
	case reflect.Uint, reflect.Uint32, reflect.Uint64:
		_, e := t.uint(v)
		return e
	case reflect.Int, reflect.Int64:
		if e := t.want(v, '0'); e != nil {
			return e
		}
		s := v.s
		if len(s) > 0 && s[0] == '-' {
			s = s[1:]
		}
		if s == "" {
			return t.bad("JSON_NUMBER_NOT_INTEGER", v)
		}
		for _, c := range s {
			if c < '0' || c > '9' {
				return t.bad("JSON_NUMBER_NOT_INTEGER", v)
			}
		}
	default:
		return t.bad("JSON_SHAPE", v)
	}
	return nil
}
