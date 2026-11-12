package kit

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"errors"
	"slices"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// SVC inline C# trees and the two fact queries (#362 scope 5, extension). The contract
// query-svc-inline-expected-r1.json holds, per inline step of the SVC69 workload (81 composite steps
// with an included range and the one CodeBehind reference C# case), either the independent CST and
// capture expectation derived from the C# grammar rules and the query source (DERIVED) or the exact
// reason it cannot be derived without the real parser (BLOCKED). Nothing is copied from native output.

const (
	SvcInlineSchema = "tsgk-query-svc-inline-expected/r1"
	SvcInlinePath   = "src/contracts/query-svc-inline-expected-r1.json"
	// shortcut: the asset is under 100 KB today; raise only with a new expected revision.
	svcInlineMaxBytes = 256 * 1024

	SvcInlineDerived = "DERIVED"
	SvcInlineBlocked = "BLOCKED"

	// The fragment is not in the language: tree-sitter's error recovery decides its tree.
	SvcInlineErrorRecoveryCode = "SVC_INLINE_ERROR_RECOVERY_NOT_DERIVABLE"
	// The fragment has a preprocessor directive: the external scanner and the parse state decide its tokens.
	SvcInlinePreprocCode = "SVC_INLINE_PREPROC_SCANNER_NOT_DERIVED"
	// The fragment is outside the derived C# subset (valid or not).
	SvcInlineSubsetCode = "SVC_INLINE_SUBSET_NOT_DERIVABLE"
	// The query expectation needs the inline tree, which is BLOCKED.
	SvcInlineQueryTreeBlockedCode = "SVC_QUERY_INLINE_TREE_BLOCKED"
	// A derived expectation has no native evidence yet.
	SvcInlineNativeNotRunCode = "SVC_INLINE_NATIVE_NOT_RUN"

	// The tree role a view was parsed in: the incremental tree, or the fresh re-parse of an edited step.
	SvcInlineRoleIncremental = "incremental"
	SvcInlineRoleFresh       = "fresh"

	svcInlineDynamicID      = "facts.dynamic_sql"
	svcInlineGrammarVersion = "tree-sitter-c-sharp 0.23.5"
	// SHA-256 of the grammar.json the rules were read from (not part of the registered closure).
	svcInlineGrammarJSON = "704882b5d27ee79c69a1508d3d649e8d7c64d43c57e07ef0041cb9daf0669e16"
)

// SvcInlineGrammar pins the grammar the derivation follows.
type SvcInlineGrammar struct {
	Version       string `json:"version"`
	GrammarJSON   string `json:"grammar_json_sha256"`
	ParserSHA256  string `json:"parser_sha256"`
	ScannerSHA256 string `json:"scanner_sha256"`
}

// SvcInlineStep is one inline obligation unit: a composite step with an included range, or the
// CodeBehind reference case (step 0, the whole file).
type SvcInlineStep struct {
	Case     string            `json:"case"`
	Step     int               `json:"step"`
	State    string            `json:"state"`
	Code     string            `json:"code,omitempty"`
	Reason   string            `json:"reason,omitempty"`
	Fragment SvcInlineFragment `json:"fragment"`
	CST      *SvcInlineCST     `json:"cst,omitempty"`
	Queries  []SvcInlineQuery  `json:"queries,omitempty"`
}

// SvcInlineQueryPin binds one registered fact query by ID, source SHA-256 and pattern count.
type SvcInlineQueryPin struct {
	ID       string `json:"id"`
	SHA256   string `json:"sha256"`
	Patterns int    `json:"patterns"`
}

// SvcInlineExpected is the decoded tsgk-query-svc-inline-expected/r1 asset.
type SvcInlineExpected struct {
	Schema        string                `json:"schema"`
	SvcExpected   queryregistrycore.Pin `json:"svc_expected"`
	Registry      queryregistrycore.Pin `json:"registry"`
	Qualification queryregistrycore.Pin `json:"qualification"`
	Fixture       queryregistrycore.Pin `json:"fixture"`
	FactPack      queryregistrycore.Pin `json:"fact_pack"`
	Grammar       SvcInlineGrammar      `json:"grammar"`
	Queries       []SvcInlineQueryPin   `json:"queries"`
	Basis         string                `json:"basis"`
	Derived       int                   `json:"derived"`
	Blocked       int                   `json:"blocked"`
	Steps         []SvcInlineStep       `json:"steps"`
}

func svcInlineFault(code string) error { return fail(KindInvalidInput, code, SvcInlinePath, nil) }

// svcInlineQuerySources returns the registered csharp fact queries of the workload, in order.
func svcInlineQuerySources(e SvcExpected, pack FactPack) (ids []string, sources []string, err error) {
	var route *FactPackRoute
	for i := range pack.Routes {
		if pack.Routes[i].Route == "csharp" {
			route = &pack.Routes[i]
		}
	}
	if route == nil {
		return nil, nil, svcInlineFault("SVC_INLINE_FACT_PACK")
	}
	for _, q := range e.Workload.Queries {
		i := slices.IndexFunc(route.Queries, func(f FactQuery) bool { return f.ID == q.ID })
		if i < 0 || digestHex([]byte(route.Queries[i].Source)) != q.SHA256 {
			return nil, nil, svcInlineFault("SVC_INLINE_QUERY_SOURCE")
		}
		ids, sources = append(ids, q.ID), append(sources, route.Queries[i].Source)
	}
	if len(ids) != 2 {
		return nil, nil, svcInlineFault("SVC_INLINE_QUERY_SOURCE")
	}
	return ids, sources, nil
}

const svcInlineBasis = "Inline C# fragments (the included ranges of the SVC composite and the CodeBehind reference case) are derived from the C# grammar rules (grammar.json) by a hand-written parser of the exact C# subset the valid fragments use (using, namespace, class, interface, attribute list with named arguments, method, field, parameter, return, string literal with escapes, member access, binary +, line comments); the root spans from its first child to the end of the included range. The two fact queries are interpreted from their source text by a small matcher (node patterns, wildcards, fields, the first-child anchor, captures) over the derived tree and kept as an unordered multiset with the pattern index. Dynamic SQL facts follow the fact pack rule: a CSHARP_COMMAND fact needs a capture of the oc.arg, ini.as or ma.as pattern, so none means zero facts. Fragments with a preprocessor directive (external scanner) and fragments outside the language (error recovery) are BLOCKED and never guessed."

// DeriveSvcInline derives the asset from the pinned sources alone: the SVC contract (included ranges),
// the fixture (sources and edits), the qualification inventory and the fact pack. It reads no native output.
func DeriveSvcInline(svcContract, registry, inventory, fixture, factPack []byte) (SvcInlineExpected, error) {
	var out SvcInlineExpected
	e, err := ParseSvcExpected(svcContract, registry, inventory, fixture)
	if err != nil {
		return out, err
	}
	wl, err := svcInventoryWorkload(inventory)
	if err != nil {
		return out, err
	}
	pack, err := ParseFactPack(factPack)
	if err != nil {
		return out, err
	}
	ids, sources, err := svcInlineQuerySources(e, pack)
	if err != nil {
		return out, err
	}
	var patterns [][]siQPattern
	for _, s := range sources {
		p, err := siParseQuery(s)
		if err != nil || len(p) == 0 {
			return out, svcInlineFault("SVC_INLINE_QUERY_SYNTAX")
		}
		patterns = append(patterns, p)
	}
	out = SvcInlineExpected{
		Schema:        SvcInlineSchema,
		SvcExpected:   queryregistrycore.FilePin(SvcExpectedPath, svcContract),
		Registry:      queryregistrycore.FilePin(queryregistrycore.Path, registry),
		Qualification: queryregistrycore.FilePin(queryregistrycore.QualificationPin.Path, inventory),
		Fixture:       queryregistrycore.FilePin("src/testdata/native/n461/svc.json", fixture),
		FactPack:      queryregistrycore.FilePin("src/contracts/fact-query-pack.json", factPack),
		Grammar:       SvcInlineGrammar{Version: svcInlineGrammarVersion, GrammarJSON: svcInlineGrammarJSON},
		Basis:         svcInlineBasis,
		Steps:         []SvcInlineStep{},
	}
	for q, id := range ids {
		out.Queries = append(out.Queries, SvcInlineQueryPin{ID: id, SHA256: digestHex([]byte(sources[q])), Patterns: len(patterns[q])})
	}
	for _, g := range wl.Grammar {
		switch g.Role {
		case "parser":
			out.Grammar.ParserSHA256 = g.SHA256
		case "scanner":
			out.Grammar.ScannerSHA256 = g.SHA256
		}
	}
	for i, c := range e.Cases {
		versions, _, err := ApplyEdits(EncodingUTF8, []byte(*wl.Cases[i].Source), wl.Cases[i].Edits, NativeOperations()["native-query-large"].InputBytes)
		if err != nil || len(versions) != len(c.Steps) {
			return out, svcInlineFault("SVC_INLINE_HISTORY")
		}
		for k, s := range c.Steps {
			var f SvcInlineFragment
			switch {
			case c.Reference:
				text := versions[k]
				f = SvcInlineFragment{EndByte: uint32(len(text)), EndPoint: [2]uint32{uint32(bytes.Count(text, []byte("\n"))), uint32(len(text) - 1 - bytes.LastIndexByte(text, '\n'))}}
			case s.Observation != nil && s.Observation.IncludedRanges != nil:
				if len(s.Observation.IncludedRanges) != 1 {
					return out, svcInlineFault("SVC_INLINE_RANGES")
				}
				r := s.Observation.IncludedRanges[0]
				f = SvcInlineFragment{StartByte: r.StartByte, EndByte: r.EndByte, StartPoint: [2]uint32{r.StartPoint.Row, r.StartPoint.Column}, EndPoint: [2]uint32{r.EndPoint.Row, r.EndPoint.Column}}
			default:
				continue
			}
			if uint64(f.EndByte) > uint64(len(versions[k])) || f.StartByte > f.EndByte {
				return out, svcInlineFault("SVC_INLINE_RANGES")
			}
			text := versions[k][f.StartByte:f.EndByte]
			f.Bytes, f.SHA256 = uint64(len(text)), digestHex(text)
			step := SvcInlineStep{Case: c.ID, Step: k, Fragment: f}
			tree, derr := siDerive(text, f)
			var se *siError
			switch {
			case derr == nil:
				step.State = SvcInlineDerived
				cst := siRowsDigest(tree.rows)
				step.CST = &cst
				for q, id := range ids {
					caps := siRunQuery(patterns[q], tree.root)
					sum := siSummarize(id, len(patterns[q]), caps)
					if id == svcInlineDynamicID {
						// the fact pack rule: a command fact needs a match of oc.a, ini.as or ma.as
						for _, c := range caps {
							if c.Name == "oc.arg" || c.Name == "ini.as" || c.Name == "ma.as" {
								return out, svcInlineFault("SVC_INLINE_DYNAMIC_FACT_NOT_DERIVED")
							}
						}
						zero := uint64(0)
						sum.Facts = &zero
					}
					step.Queries = append(step.Queries, sum)
				}
				out.Derived++
			case errors.As(derr, &se):
				step.State, step.Reason, step.Code = SvcInlineBlocked, se.Error(), svcInlineCode(se)
				out.Blocked++
			default:
				return out, derr
			}
			out.Steps = append(out.Steps, step)
		}
	}
	return out, nil
}

// svcInlineCode is the BLOCKED code of a refused fragment: a token no grammar rule accepts means the
// real parser recovers from an error, a preprocessor directive belongs to the external scanner, and
// anything else is outside the derived subset.
func svcInlineCode(e *siError) string {
	switch e.class {
	case siPreproc:
		return SvcInlinePreprocCode
	case siSyntax, siLexical:
		return SvcInlineErrorRecoveryCode
	}
	return SvcInlineSubsetCode
}

// ParseSvcInlineExpected decodes the asset and requires it to equal the derivation from the pinned
// sources: pins, grammar, membership (the 82 inline steps in order), every state and digest.
func ParseSvcInlineExpected(data, svcContract, registry, inventory, fixture, factPack []byte) (SvcInlineExpected, error) {
	var x SvcInlineExpected
	if len(data) > svcInlineMaxBytes {
		return x, fail(KindResourceLimit, "SVC_INLINE_EXPECTED_SIZE", SvcInlinePath, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "SVC_INLINE_EXPECTED_JSON", SvcInlinePath, err)
	}
	if x.Schema != SvcInlineSchema || x.Basis == "" {
		return x, svcInlineFault("SVC_INLINE_EXPECTED_SCHEMA")
	}
	want, err := DeriveSvcInline(svcContract, registry, inventory, fixture, factPack)
	if err != nil {
		return x, err
	}
	if x.SvcExpected != want.SvcExpected || x.Registry != want.Registry || x.Qualification != want.Qualification || x.Fixture != want.Fixture || x.FactPack != want.FactPack {
		return x, svcInlineFault("SVC_INLINE_EXPECTED_PIN")
	}
	a, errA := jsonv2.Marshal(x)
	b, errB := jsonv2.Marshal(want)
	if errA != nil || errB != nil || !bytes.Equal(a, b) {
		return x, svcInlineFault("SVC_INLINE_EXPECTED_DERIVATION")
	}
	if x.Derived+x.Blocked != 82 || len(x.Steps) != 82 {
		return x, svcInlineFault("SVC_INLINE_EXPECTED_COUNTS")
	}
	return x, nil
}

// SvcInlineQueryResult is one native query result on one tree.
type SvcInlineQueryResult struct {
	Status   string
	Captures []Capture
}

// SvcInlineView is one parse of a step as the native producer reported it: its role, the public CST
// and the result of every query by ID. A step the native run reached by editing the previous inline
// step has an incremental and a fresh view and both must be given and match; a step with no inline
// step before it (step 0, or the start of a segment) has the incremental view only.
type SvcInlineView struct {
	Role    string
	Nodes   []TreeNode
	Queries map[string]SvcInlineQueryResult
}

// SvcInlineObserved is the native evidence of one inline step. Source is the step's source version.
type SvcInlineObserved struct {
	Case   string
	Step   int
	Source []byte
	Views  []SvcInlineView
}

func svcInlineID(kind string, s SvcInlineStep, query string) string {
	return svcID(kind, s.Case, s.Step, query)
}

// JudgeSvcInline judges the inline tree and query obligations: a DERIVED step against the native
// evidence (VERIFIED or FAILED with the first mismatch code, BLOCKED SVC_INLINE_NATIVE_NOT_RUN without
// evidence), a BLOCKED step with its registered code. Evidence for a BLOCKED, unknown or repeated
// step is an error: a blocked step never becomes VERIFIED through native output. Evidence that lacks
// a view role its step needs is never VERIFIED: it is BLOCKED unless a view it does have already
// mismatches, which stays FAILED.
func JudgeSvcInline(x SvcInlineExpected, observed []SvcInlineObserved) ([]SvcJudgment, error) {
	byStep := map[string]SvcInlineObserved{}
	for _, o := range observed {
		key := svcID("", o.Case, o.Step, "")
		if _, dup := byStep[key]; dup {
			return nil, svcInlineFault("SVC_INLINE_OBSERVED_REPEATED")
		}
		byStep[key] = o
	}
	inline := map[string]bool{}
	for _, s := range x.Steps {
		inline[svcID("", s.Case, s.Step, "")] = true
	}
	var out []SvcJudgment
	used := 0
	for _, s := range x.Steps {
		treeID := svcInlineID(SvcKindInlineTree, s, "")
		o, seen := byStep[svcID("", s.Case, s.Step, "")]
		if s.State != SvcInlineDerived {
			if seen {
				return nil, svcInlineFault("SVC_INLINE_OBSERVED_FOR_BLOCKED")
			}
			out = append(out, SvcJudgment{ID: treeID, State: "BLOCKED", Code: s.Code})
			for _, q := range x.queryIDs() {
				out = append(out, SvcJudgment{ID: svcInlineID(SvcKindQuery, s, q), State: "BLOCKED", Code: SvcInlineQueryTreeBlockedCode})
			}
			continue
		}
		if !seen || len(o.Views) == 0 {
			out = append(out, SvcJudgment{ID: treeID, State: "BLOCKED", Code: SvcInlineNativeNotRunCode})
			for _, q := range x.queryIDs() {
				out = append(out, SvcJudgment{ID: svcInlineID(SvcKindQuery, s, q), State: "BLOCKED", Code: SvcInlineNativeNotRunCode})
			}
			continue
		}
		used++
		complete, err := svcInlineViewsComplete(o, s.Step > 0 && inline[svcID("", s.Case, s.Step-1, "")])
		if err != nil {
			return nil, err
		}
		judge := func(id, code string) SvcJudgment {
			if code == "" && !complete {
				return SvcJudgment{ID: id, State: "BLOCKED", Code: SvcInlineNativeNotRunCode}
			}
			return svcInlineJudge(id, code)
		}
		out = append(out, judge(treeID, svcInlineTreeCode(s, o)))
		for _, q := range s.Queries {
			out = append(out, judge(svcInlineID(SvcKindQuery, s, q.ID), svcInlineQueryCode(q, o)))
		}
	}
	if used != len(byStep) {
		return nil, svcInlineFault("SVC_INLINE_OBSERVED_UNKNOWN")
	}
	return out, nil
}

// svcInlineViewsComplete reports whether the views are exactly the roles the step needs: incremental,
// and fresh too when edited. An unknown or repeated role, or a fresh view for a step that has none,
// is an error; a missing role is incomplete evidence.
func svcInlineViewsComplete(o SvcInlineObserved, edited bool) (bool, error) {
	var incremental, fresh int
	for _, v := range o.Views {
		switch v.Role {
		case SvcInlineRoleIncremental:
			incremental++
		case SvcInlineRoleFresh:
			fresh++
		default:
			return false, svcInlineFault("SVC_INLINE_VIEW_ROLE")
		}
	}
	if incremental > 1 || fresh > 1 || fresh == 1 && !edited {
		return false, svcInlineFault("SVC_INLINE_VIEW_ROLE")
	}
	return incremental == 1 && (fresh == 1) == edited, nil
}

func (x SvcInlineExpected) queryIDs() []string {
	ids := make([]string, len(x.Queries))
	for i, q := range x.Queries {
		ids[i] = q.ID
	}
	return ids
}

func svcInlineJudge(id, code string) SvcJudgment {
	if code == "" {
		return SvcJudgment{ID: id, State: "VERIFIED"}
	}
	return SvcJudgment{ID: id, State: "FAILED", Code: code}
}

func svcInlineTreeCode(s SvcInlineStep, o SvcInlineObserved) string {
	for _, v := range o.Views {
		if len(v.Nodes) == 0 {
			return "SVC_INLINE_CST_EMPTY"
		}
		got := siRowsDigest(siRowsOfNodes(v.Nodes))
		switch {
		case got.Nodes != s.CST.Nodes:
			return "SVC_INLINE_CST_NODES"
		case got.HasError != s.CST.HasError || v.Nodes[0].HasError != s.CST.HasError:
			return "SVC_INLINE_CST_ERROR"
		case got.RowsSHA256 != s.CST.RowsSHA256 || got.MaxDepth != s.CST.MaxDepth:
			return "SVC_INLINE_CST_DIGEST"
		}
	}
	return ""
}

func svcInlineQueryCode(q SvcInlineQuery, o SvcInlineObserved) string {
	for _, v := range o.Views {
		r, ok := v.Queries[q.ID]
		if !ok || r.Status != StatusCompleted {
			return "SVC_INLINE_QUERY_NOT_COMPLETED"
		}
		for i := 1; i < len(r.Captures); i++ {
			if r.Captures[i].StartByte < r.Captures[i-1].StartByte {
				return "SVC_INLINE_QUERY_ORDER"
			}
		}
		got := siSummarize(q.ID, q.Patterns, siCapturesOf(r.Captures))
		switch {
		case got.Captures != q.Captures:
			return "SVC_INLINE_QUERY_COUNT"
		case !slices.Equal(got.ByName, q.ByName):
			return "SVC_INLINE_QUERY_NAMES"
		case got.SHA256 != q.SHA256:
			return "SVC_INLINE_QUERY_DIGEST"
		}
		if q.Facts != nil {
			facts, err := DeriveDynamicSQLChecked("csharp", EncodingUTF8, r.Captures, o.Source)
			if err != nil || uint64(len(facts.Items)) != *q.Facts || len(facts.KnownMisses) != 0 || !strings.HasPrefix(facts.Mapping, "dynamic-sql-") {
				return "SVC_INLINE_DYNAMIC_FACTS"
			}
		}
	}
	return ""
}

// MergeSvcJudgments replaces the judgment of every obligation that over names; an obligation that
// base does not have is an error.
func MergeSvcJudgments(base, over []SvcJudgment) ([]SvcJudgment, error) {
	out := slices.Clone(base)
	for _, j := range over {
		i := slices.IndexFunc(out, func(b SvcJudgment) bool { return b.ID == j.ID })
		if i < 0 {
			return nil, svcFault("SVC_SCOPE_UNDECLARED")
		}
		out[i] = j
	}
	return out, nil
}
