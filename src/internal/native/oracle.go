package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// OracleResultSchema is the stdout result of `tsgk oracle record`.
const OracleResultSchema = "tsgk-oracle-result/r1"

// QueryExpectResult is one evaluated query expectation.
type QueryExpectResult struct {
	kit.QueryExpectation
	Result string `json:"result"`
	Detail string `json:"detail,omitempty"`
}

// FactsOut are the facts derived from the pack queries on step 0's incremental tree.
type FactsOut struct {
	Pack         kit.FactPackRef            `json:"pack"`
	Declarations *DeclFacts                 `json:"declarations"`
	DynamicSQL   *kit.DynamicSQLFacts       `json:"dynamic_sql"`
	XML          map[string]int             `json:"xml_structure,omitempty"`
	Difference   string                     `json:"first_difference,omitempty"`
	Expected     *kit.DynamicSQLExpectation `json:"-"`
}

// DeclFacts are the declaration items the pack query reproduced and whether they equal the
// S05 traversal items of the same tree.
type DeclFacts struct {
	Items      []kit.DeclarationItem `json:"items"`
	Assessment string                `json:"assessment"`
	Reproduces bool                  `json:"reproduces_s05"`
}

// OracleRequest is one `tsgk oracle record` run. Paths are absolute.
type OracleRequest struct {
	Root         string
	GrammarRoot  string
	Profile      []byte
	FactPack     []byte // the pack file when the profile binds one
	Runtime      string
	Compiler     string
	Work         string
	Out          string
	Allow        []string
	CgroupParent string
	RunWall      time.Duration
}

// OracleResult is the stdout result of a record run.
type OracleResult struct {
	kit.Report
	ResultSchema  string               `json:"result_schema"`
	ProfileID     string               `json:"profile_id"`
	Route         string               `json:"route"`
	Operation     kit.NativeOperation  `json:"operation"`
	Build         *Build               `json:"build"`
	BuildRemoved  string               `json:"build_removed"`
	Summary       Summary              `json:"summary"`
	Cases         []CaseLine           `json:"cases"`
	Set           *kit.OracleSetReport `json:"set"`
	Platform      string               `json:"platform"`
	WallMillis    int64                `json:"wall_ms"`
	ProfileSHA256 string               `json:"profile_sha256"`
}

// CaseLine summarizes one case in the stdout result; the record holds the details.
type CaseLine struct {
	ID              string        `json:"id"`
	ExecutionStatus string        `json:"execution_status"`
	Assessment      string        `json:"assessment"`
	Code            string        `json:"code"`
	Claims          Claims        `json:"claims"`
	Oracle          *OracleClaims `json:"oracle_claims"`
}

// OracleRecord is one published case record; complete is its last member.
type OracleRecord struct {
	Schema   string `json:"schema"`
	Case     string `json:"case"`
	Workload string `json:"workload"`
	CaseResult
	Complete bool `json:"complete"`
}

// hostPlatform is this host as goos/goarch (a test may substitute another host).
var hostPlatform = runtime.GOOS + "/" + runtime.GOARCH

// platformScope refuses, before any build, an operation whose platform scope excludes this
// host (real-world-source-r3 and native-query-large run on windows/amd64 only).
func platformScope(op kit.NativeOperation) (kit.Finding, *Error) {
	if len(op.Platforms) == 0 || slices.Contains(op.Platforms, hostPlatform) {
		return kit.Finding{}, nil
	}
	return finding("OPERATION_PLATFORM_SCOPE", "error", "", op.Name+"는 "+fmt.Sprint(op.Platforms)+"에서만 실행한다: "+kit.R3PlatformReason),
		refuse(kit.KindUnsupported, "OPERATION_PLATFORM_SCOPE", nil)
}

// createExclusive is the single file-creation path of a record set: it never replaces an
// existing file (a test may substitute it to inject a write failure).
var createExclusive = func(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Oracle builds the driver for an oracle profile, records every case through the r2
// protocol and publishes the record set: the output directory is created exclusively,
// every member is created without replacement, and the manifest (whose last member marks
// completeness) is written only after every member was written; the set is then verified
// offline. A refusal before the build creates nothing.
func Oracle(ctx context.Context, req OracleRequest) (OracleResult, error) {
	start := time.Now()
	res := OracleResult{Report: kit.Report{Schema: kit.ReportSchema, Command: "oracle record", ExecutionStatus: kit.StatusNotRun, EvidenceMode: kit.ModeNotRun,
		Assessment: kit.AssessNotAssessed, Identities: []kit.IdentityRef{}, Findings: []kit.Finding{}, Coverage: kit.Coverage{Requested: []string{}, Observed: []string{}, Unsupported: []string{}}},
		ResultSchema: OracleResultSchema, Cases: []CaseLine{}, Platform: hostPlatform}
	stop := func(err *Error) (OracleResult, error) {
		res.Findings = append(res.Findings, finding(err.Code, "error", "", "실행 전 거부했다"))
		if err.Kind == kit.KindUnsupported {
			res.Assessment = kit.AssessBlocked
		}
		return res, err
	}
	granted := map[string]bool{}
	for _, a := range req.Allow {
		granted[a] = true
	}
	if !granted[AllowBuild] || !granted[AllowExec] {
		return stop(refuse(kit.KindUnsupported, "CAPABILITY_NOT_GRANTED", nil))
	}
	prof, err := kit.ParseOracleProfile(req.Profile)
	if err != nil {
		var ke *kit.Error
		errors.As(err, &ke)
		return stop(refuse(ke.Kind, ke.Code, ke))
	}
	n := prof.Native
	op := kit.NativeOperations()[n.Operation]
	res.ProfileID, res.Route, res.Operation, res.ProfileSHA256 = n.ID, n.Route, op, n.SHA256
	if f, err := platformScope(op); err != nil {
		res.Findings = append(res.Findings, f)
		res.Assessment = kit.AssessBlocked
		return res, err
	}
	var pack *kit.FactPack
	if prof.FactPack != nil {
		p, e := checkFactPack(prof, req.FactPack)
		if e != nil {
			return stop(e)
		}
		pack = &p
	}
	probe := runner.Probe(runner.Spec{Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: runtime.GOOS != "darwin"}, CgroupParent: req.CgroupParent})
	if runtime.GOOS != "darwin" && probe.Memory != runner.MemoryHard {
		return stop(refuse(kit.KindUnsupported, "MEMORY_HARD_CAP_UNSUPPORTED", nil))
	}
	// the exclusive directory is the only collision guard: an existing output or a
	// concurrent run that created it first makes this run stop before writing anything
	if err := os.Mkdir(req.Out, 0o755); err != nil {
		code := "OUTPUT_FAILED"
		if errors.Is(err, os.ErrExist) {
			code = "OUTPUT_EXISTS"
		}
		return stop(refuse(kit.KindInvalidInput, code, err))
	}
	for _, d := range []string{"records", "raw"} {
		if err := os.Mkdir(filepath.Join(req.Out, d), 0o755); err != nil {
			return stop(refuse(kit.KindIO, "OUTPUT_FAILED", err))
		}
	}
	if req.RunWall > 0 && req.RunWall < op.RunWall {
		op.RunWall = req.RunWall
		res.Operation = op
	}
	runCtx, cancel := context.WithTimeout(ctx, op.RunWall)
	defer cancel()
	groot := req.GrammarRoot
	if groot == "" {
		groot = req.Root
	}
	b, berr := NewBuild(runCtx, BuildRequest{Work: req.Work, Runtime: req.Runtime, GrammarRoot: groot, Grammar: n.Grammar, Symbol: n.Symbol,
		Compiler: req.Compiler, CompilerID: n.Compiler, CgroupParent: req.CgroupParent})
	res.Build = b
	res.ExecutionStatus, res.EvidenceMode = kit.StatusCompleted, kit.ModeNewRun
	policy := PolicyRef(op, n.Output)
	res.Identities = append(res.Identities, kit.IdentityRef{Role: "profile", Schema: n.Schema, SHA256: n.SHA256}, policy)
	var queries []QuerySource
	qids := make([]kit.IdentityRef, 0, len(prof.Queries))
	for _, q := range prof.Queries {
		queries = append(queries, QuerySource{ID: q.ID, Source: []byte(q.Source)})
		qids = append(qids, kit.IdentityRef{Role: "query:" + q.ID, Schema: "tsgk-query-source/r1", SHA256: sha([]byte(q.Source))})
	}
	writeFailed := false
	var members []kit.OracleMember
	var cases []CaseResult
	write := func(rel string, data []byte) bool {
		if err := createExclusive(filepath.Join(req.Out, filepath.FromSlash(rel)), data); err != nil {
			res.Findings = append(res.Findings, finding("EVIDENCE_WRITE_FAILED", "error", rel, "record set member를 쓰지 못했다"))
			writeFailed = true
			return false
		}
		return true
	}
	if berr != nil {
		var ne *Error
		errors.As(berr, &ne)
		res.ExecutionStatus, res.Assessment = kit.StatusFailed, kit.AssessNotAssessed
		if ne.Kind == kit.KindInvalidInput {
			res.ExecutionStatus = kit.StatusNotRun
		}
		res.Findings = append(res.Findings, finding(ne.Code, "error", "", buildFailure(berr, b)))
	} else {
		res.Identities = append(res.Identities, kit.IdentityRef{Role: "producer", Schema: BuildSchema, SHA256: b.Identity})
		x := Context{Op: op, Route: n.Route, Output: n.Output, Declarations: n.Declarations, Format: n.Format, SvcContext: n.SvcContext, CgroupParent: req.CgroupParent, PolicyRef: policy,
			Protocol: ProtocolR2, Queries: queries, API: prof.API}
		refs := b.runSvcReferences(runCtx, x, n.Cases, req.Root)
		for _, oc := range prof.OracleCases {
			if ref, found := refs[oc.ID]; found && ref.ExecutionStatus == kit.StatusCompleted {
				src, err := readCase(req.Root, oc.Input)
				if err != nil {
					ref.ExecutionStatus, ref.Assessment, ref.Code = kit.StatusNotRun, kit.AssessBlocked, err.(*Error).Code
				} else {
					judgeOracle(&ref, oc, prof, pack, x, src)
				}
				refs[oc.ID] = ref
			}
		}
		for i, oc := range prof.OracleCases {
			c := oc.IncrementalCase
			var cr CaseResult
			src, err := readCase(req.Root, c.Input)
			if err != nil {
				cr = CaseResult{ID: c.ID, Input: c.Input, Encoding: c.Encoding, ExecutionStatus: kit.StatusNotRun, Assessment: kit.AssessNotAssessed,
					Code: err.(*Error).Code, Claims: Claims{ClaimNotClaimed, ClaimNotClaimed, ClaimNotClaimed}, Steps: []StepResult{}, Expectations: []ExpectationResult{}}
			} else {
				var found bool
				cr, found = refs[c.ID]
				if !found {
					cr = b.RunCase(runCtx, x, c, src)
					linkSvcReferences(&cr, refs)
					judgeOracle(&cr, oc, prof, pack, x, src)
				}
			}
			cases = append(cases, cr)
			name := fmt.Sprintf("%05d-%s.json", i, c.ID)
			if cr.Raw != nil && !writeFailed {
				if write("raw/"+name, cr.Raw) {
					members = append(members, kit.OracleMember{Path: "raw/" + name, Role: "raw", Case: c.ID, Bytes: uint64(len(cr.Raw)), SHA256: sha(cr.Raw),
						InputBytes: c.Input.Bytes, InputSHA256: c.Input.SHA256, ExecutionStatus: cr.ExecutionStatus})
				}
			}
			if !writeFailed {
				data, err := json.Marshal(OracleRecord{Schema: kit.OracleRecordSchema, Case: c.ID, Workload: n.ID, CaseResult: cr, Complete: true})
				if err != nil {
					res.Findings = append(res.Findings, finding("EVIDENCE_WRITE_FAILED", "error", c.ID, "record를 직렬화하지 못했다"))
					writeFailed = true
				} else if write("records/"+name, append(data, '\n')) {
					members = append(members, kit.OracleMember{Path: "records/" + name, Role: "record", Case: c.ID, Bytes: uint64(len(data) + 1), SHA256: sha(append(data, '\n')),
						InputBytes: c.Input.Bytes, InputSHA256: c.Input.SHA256, ExecutionStatus: cr.ExecutionStatus})
				}
			}
			res.Cases = append(res.Cases, CaseLine{cr.ID, cr.ExecutionStatus, cr.Assessment, cr.Code, cr.Claims, cr.Oracle})
		}
		res.ExecutionStatus, res.Assessment = aggregate(cases)
	}
	if b != nil {
		res.BuildRemoved = "REMOVED"
		if err := b.Remove(); err != nil {
			res.BuildRemoved = "FAILED: " + err.Error()
			res.Findings = append(res.Findings, finding("BUILD_CLEANUP_FAILED", "error", "", "build 디렉터리를 지우지 못했다"))
			res.ExecutionStatus = kit.StatusFailed
		}
	}
	res.Summary = summarize(cases, nil)
	if writeFailed {
		// no manifest: the set has no completeness marker and is never a reference set
		res.ExecutionStatus = kit.StatusFailed
		if res.Assessment == kit.AssessPass {
			res.Assessment = kit.AssessNotAssessed
		}
	} else {
		m := kit.OracleManifest{Schema: kit.OracleManifestSchema, Policy: policy, Protocol: ProtocolR2, Queries: qids, FactPack: prof.FactPack, Cases: []string{},
			Comparators: []string{kit.TreeDigestScheme, kit.CaptureCompareScheme, APICapability, PredicatePolicy},
			Workload:    map[string]string{"profile_id": n.ID, "profile_sha256": n.SHA256, "route": n.Route, "operation": n.Operation, "output": n.Output},
			Producer:    map[string]string{"platform": res.Platform}, ExecutionStatus: res.ExecutionStatus, Assessment: res.Assessment, Members: members, Complete: true}
		if m.Members == nil {
			m.Members = []kit.OracleMember{}
		}
		if b != nil {
			m.Producer["build_identity"], m.Producer["executable_sha256"], m.Producer["runtime_commit"] = b.Identity, b.ExecutableSHA256, b.Runtime
			m.Producer["compiler_sha256"], m.Producer["compiler_version"] = b.Compiler.SHA256, b.CompilerVersion
		}
		for _, mem := range members {
			if mem.Role == "record" {
				m.Records++
			}
		}
		if berr == nil {
			// every profile case gets a record; one without is a CASE_RECORD_MISMATCH
			for _, oc := range prof.OracleCases {
				m.Cases = append(m.Cases, oc.ID)
			}
		}
		data, err := json.Marshal(m)
		if err == nil && write("manifest.json", append(data, '\n')) {
			set := kit.VerifyOracleSet(os.DirFS(req.Out))
			res.Set = &set
			if !set.Valid {
				res.Findings = append(res.Findings, finding("REFERENCE_SET_INVALID", "error", "", "발행한 record set이 검증을 통과하지 못했다"))
				res.ExecutionStatus = kit.StatusFailed
			}
		} else {
			res.ExecutionStatus = kit.StatusFailed
		}
	}
	res.WallMillis = time.Since(start).Milliseconds()
	res.Coverage.Requested = []string{"tree", "queries", "api", "facts"}
	if res.ExecutionStatus == kit.StatusCompleted {
		res.Coverage.Observed = res.Coverage.Requested
	}
	return res, nil
}

// checkFactPack binds the profile's pack queries to the pack file: same revision and
// sha256, the route present, at least one pack query of the route in the profile and every
// one present with the pack's source (a profile may select a subset, e.g. the declaration
// query alone for large inputs), and for the declaration query the route's items equal to
// the profile's declarations.
func checkFactPack(prof kit.OracleProfile, data []byte) (kit.FactPack, *Error) {
	bad := func(what string) (kit.FactPack, *Error) {
		return kit.FactPack{}, refuse(kit.KindInvalidInput, "FACT_PACK_MISMATCH", errors.New(what))
	}
	if data == nil {
		return bad("the profile binds a fact pack but none was given")
	}
	p, err := kit.ParseFactPack(data)
	if err != nil {
		var ke *kit.Error
		errors.As(err, &ke)
		return kit.FactPack{}, refuse(ke.Kind, ke.Code, ke)
	}
	ref := prof.FactPack
	if p.Revision != ref.Revision || p.SHA256 != ref.SHA256 {
		return bad("revision or sha256")
	}
	i := slices.IndexFunc(p.Routes, func(r kit.FactPackRoute) bool { return r.Route == ref.Route })
	if i < 0 {
		return bad("route")
	}
	r := p.Routes[i]
	selected := 0
	dynamicSelected := false
	for _, q := range r.Queries {
		j := slices.IndexFunc(prof.Queries, func(o kit.OracleQuery) bool { return o.ID == q.ID })
		if j < 0 {
			continue
		}
		selected++
		dynamicSelected = dynamicSelected || q.Facts == kit.FactDynamicSQL
		if prof.Queries[j].Source != q.Source {
			return bad("query " + q.ID)
		}
		if q.Facts == kit.FactDeclarations {
			var items []kit.NativeDeclaration
			if prof.Native.Declarations != nil {
				items = prof.Native.Declarations.Items
			}
			if !slices.Equal(items, r.Declarations) || q.Source != kit.DeclarationQuery(r.Declarations) {
				return bad("declarations")
			}
		}
	}
	if selected == 0 {
		return bad("no pack query selected")
	}
	if !dynamicSelected && slices.ContainsFunc(prof.OracleCases, func(c kit.OracleCase) bool { return c.DynamicSQL != nil }) {
		return bad("dynamic SQL expectation without a selected dynamic SQL query")
	}
	return p, nil
}

// judgeOracle evaluates a recorded case's query expectations and pack facts and folds
// them into the assessment.
func judgeOracle(cr *CaseResult, oc kit.OracleCase, prof kit.OracleProfile, pack *kit.FactPack, x Context, src []byte) {
	if cr.Oracle == nil {
		return // not run, refused or failed before the observations were accepted
	}
	stepQueries := func(step int) []QueryOut {
		if step < len(cr.Steps) && cr.Steps[step].Incremental != nil {
			return cr.Steps[step].Incremental.Queries
		}
		return nil
	}
	if len(oc.QueryExpect) > 0 {
		cr.Oracle.QueryExpectations = ClaimPass
		versions, _, _ := kit.ApplyEdits(oc.Encoding, src, oc.Edits, x.Op.InputBytes)
		for _, e := range oc.QueryExpect {
			r := QueryExpectResult{QueryExpectation: e, Result: ClaimPass}
			qs := stepQueries(e.Step)
			i := slices.IndexFunc(qs, func(q QueryOut) bool { return q.ID == e.Query })
			switch {
			case i < 0:
				r.Result, r.Detail = ClaimBlocked, "step not recorded"
			case qs[i].Status != e.Status || qs[i].Code != e.Code:
				r.Result, r.Detail = ClaimFail, "status "+qs[i].Status+" "+qs[i].Code
			case e.Error != nil && (qs[i].Error == nil || qs[i].Error.Type != e.Error.Type || qs[i].Error.Offset != e.Error.Offset):
				r.Result, r.Detail = ClaimFail, "error"
			case e.Captures != nil:
				r.Result, r.Detail = compareCaptureText(*e.Captures, qs[i].Captures, versions[e.Step])
			}
			cr.Oracle.QueryExpectations = worse(cr.Oracle.QueryExpectations, r.Result)
			cr.QueryExpect = append(cr.QueryExpect, r)
		}
	}
	if pack != nil && cr.ExecutionStatus == kit.StatusCompleted && (len(cr.Steps) == 0 || cr.Steps[0].Incremental == nil) {
		cr.Oracle.FactReproduction = ClaimBlocked
		if oc.DynamicSQL != nil {
			cr.Oracle.DynamicSQL = ClaimBlocked
		}
	}
	if pack != nil && cr.ExecutionStatus == kit.StatusCompleted && len(cr.Steps) > 0 && cr.Steps[0].Incremental != nil {
		f := &FactsOut{Pack: *prof.FactPack}
		setDifference := func(detail string) {
			if f.Difference == "" {
				f.Difference = detail
			}
		}
		r := pack.Routes[slices.IndexFunc(pack.Routes, func(r kit.FactPackRoute) bool { return r.Route == prof.FactPack.Route })]
		t := cr.Steps[0].Incremental
		for _, q := range r.Queries {
			if !slices.ContainsFunc(prof.Queries, func(o kit.OracleQuery) bool { return o.ID == q.ID }) {
				continue // not selected by this profile
			}
			i := slices.IndexFunc(t.Queries, func(o QueryOut) bool { return o.ID == q.ID })
			if i < 0 || t.Queries[i].Status != kit.StatusCompleted {
				setDifference("pack query " + q.ID + " not completed")
				cr.Oracle.FactReproduction = worse(cr.Oracle.FactReproduction, ClaimBlocked)
				continue
			}
			caps := t.Queries[i].Captures
			switch q.Facts {
			case kit.FactDeclarations:
				items := kit.DeriveDeclarations(r.Declarations, caps)
				var s05 []DeclItem
				if t.Summary != nil {
					s05 = t.Summary.Declarations.Items
				} else if t.Declarations != nil {
					s05 = *t.Declarations
				}
				d := &DeclFacts{Items: items, Assessment: declAssessment(items)}
				d.Reproduces = len(items) == len(s05)
				for k := 0; d.Reproduces && k < len(items); k++ {
					a, b := items[k], s05[k]
					var an, bn *NameSpan
					if a.Name != nil {
						an = &NameSpan{a.Name.StartByte, a.Name.EndByte}
					}
					bn = b.Name
					d.Reproduces = a.Fact == b.Fact && a.NodeType == b.NodeType && a.StartByte == b.StartByte && a.EndByte == b.EndByte && a.Status == b.Status &&
						(an == nil) == (bn == nil) && (an == nil || *an == *bn)
					if !d.Reproduces {
						setDifference(fmt.Sprintf("declaration item %d", k))
					}
				}
				if !d.Reproduces && f.Difference == "" {
					setDifference(fmt.Sprintf("declaration items %d != %d", len(items), len(s05)))
				}
				f.Declarations = d
				if d.Reproduces {
					cr.Oracle.FactReproduction = worse(cr.Oracle.FactReproduction, ClaimPass)
				} else {
					cr.Oracle.FactReproduction = ClaimFail
				}
			case kit.FactDynamicSQL:
				ds, err := kit.DeriveDynamicSQLChecked(prof.FactPack.Route, oc.Encoding, caps, src)
				if err != nil {
					setDifference(err.Error())
					cr.Oracle.DynamicSQL = worse(cr.Oracle.DynamicSQL, ClaimBlocked)
					continue
				}
				f.DynamicSQL = &ds
				if oc.DynamicSQL != nil {
					if d := compareDynamicSQL(*oc.DynamicSQL, ds); d != "" {
						setDifference(d)
						cr.Oracle.DynamicSQL = ClaimFail
					} else {
						cr.Oracle.DynamicSQL = worse(cr.Oracle.DynamicSQL, ClaimPass)
					}
				}
			case kit.FactXMLStructure:
				f.XML = map[string]int{}
				for _, c := range caps {
					f.XML[c.Name]++
				}
			}
		}
		cr.Facts = f
	}
	if cr.ExecutionStatus == kit.StatusCompleted {
		foldAssessment(cr)
		if cr.Assessment == kit.AssessFail && (cr.Code == "" || strings.HasPrefix(cr.Code, routeUnobservablePrefix)) {
			// a blocked route code names the case only when nothing fails
			cr.Code = "ORACLE_CLAIM_FAILED"
		}
	}
}

func declAssessment(items []kit.DeclarationItem) string {
	if len(items) == 0 {
		return "NOT_APPLICABLE"
	}
	for _, it := range items {
		if it.Status != "PASS" {
			return kit.AssessFail
		}
	}
	return kit.AssessPass
}

// compareCaptureText compares a registered capture list with the evaluated stream by
// (name, node type, exact source text), in order, without sorting or deduplication.
func compareCaptureText(want []kit.CaptureExpectation, got []kit.Capture, src []byte) (string, string) {
	if got == nil {
		return ClaimBlocked, "no evaluated captures"
	}
	for i := range min(len(want), len(got)) {
		t, err := kit.CaptureText(src, got[i])
		if err != nil {
			return ClaimBlocked, err.Error()
		}
		if w := want[i]; w.Name != got[i].Name || w.Type != got[i].Type || w.Text != t {
			return ClaimFail, fmt.Sprintf("capture %d: %s %s %q", i, got[i].Name, got[i].Type, t)
		}
	}
	if len(want) != len(got) {
		return ClaimFail, fmt.Sprintf("capture count %d != %d", len(got), len(want))
	}
	return ClaimPass, ""
}

// compareDynamicSQL compares derived dynamic SQL facts and known-miss ranges with the
// registered expectation, in order; it returns the first difference or "".
func compareDynamicSQL(want kit.DynamicSQLExpectation, got kit.DynamicSQLFacts) string {
	for i := range min(len(want.Facts), len(got.Items)) {
		a, b := want.Facts[i], got.Items[i]
		if a.Construct != b.Construct || a.ArgumentKind != b.ArgumentKind || a.StartByte != b.StartByte || a.EndByte != b.EndByte || a.StartPoint != b.StartPoint ||
			a.EndPoint != b.EndPoint || a.Heuristic != b.Heuristic || (a.Variable == nil) != (b.Variable == nil) || (a.Variable != nil && *a.Variable != *b.Variable) {
			return fmt.Sprintf("fact %d: %+v", i, b)
		}
	}
	if len(want.Facts) != len(got.Items) {
		return fmt.Sprintf("fact count %d != %d", len(got.Items), len(want.Facts))
	}
	if len(want.KnownMisses) != len(got.KnownMisses) {
		return fmt.Sprintf("known miss count %d != %d", len(got.KnownMisses), len(want.KnownMisses))
	}
	for i, k := range got.KnownMisses {
		if k.StartByte != want.KnownMisses[i].StartByte || k.EndByte != want.KnownMisses[i].EndByte {
			return fmt.Sprintf("known miss %d: %s [%d,%d)", i, k.Case, k.StartByte, k.EndByte)
		}
	}
	return ""
}
