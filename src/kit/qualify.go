package kit

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Qualification schemas (Session 08).
const (
	QualificationInventorySchema = "tsgk-qualification-inventory/r1"
	QualificationResultSchema    = "tsgk-qualification-result/r2"
	RunIdentitySchema            = "tsgk-run-identity/r1"
	// CoverageRule names how a registered case covers a requirement row's case kinds:
	// P a step expecting NO_ERROR with named structure, N a step expecting ERROR, R a step
	// expecting ERROR with named structure kept, E at least one edit (incremental/fresh
	// comparison), Q a query case with registered capture expectations. W (licensed
	// real-world sample) has no registered producer and is never covered.
	CoverageRule = "tsgk-coverage-rule/r1"
)

// Cell, obligation and role row values of a qualification result.
const (
	CellPass       = "PASS"
	CellFail       = "FAIL"
	CellIncomplete = "INCOMPLETE" // every executed check passed, a required obligation has no case
	CellMissing    = "MISSING"    // no evidence for the cell

	ObligationNotCovered = "NOT_COVERED"

	RoleExecuted      = "EXECUTED"
	RoleNotRun        = "NOT_RUN"
	RoleExternal      = "EXTERNAL" // checked by another gate (named in the reason), not by this aggregation
	RoleNotApplicable = "NOT_APPLICABLE"
)

var caseKinds = []string{"P", "N", "R", "E", "Q", "W"}

// QualificationLimits are the bounds of one `qualification` operation (S08): per host
// 10000 files, 64 MiB per file (the windows-only NET461 large records reach 56 MiB),
// 512 MiB read and 100000 records; one wall of 360 s and a 16 MiB result for the whole
// aggregation.
func QualificationLimits() ReplayLimits {
	return ReplayLimits{Files: 10000, FileBytes: 67108864, TotalBytes: 536870912, Records: 100000, RecordBytes: 67108864, OutputBytes: 16777216, Wall: 360 * time.Second, WallMillis: 360000}
}

// QualPlatform is one registered OS/architecture of the inventory.
type QualPlatform struct {
	ID     string `json:"id"`
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

func (p QualPlatform) pair() string { return p.GOOS + "/" + p.GOARCH }

// QualRequirement is one required feature row and the case kinds it requires.
type QualRequirement struct {
	Row   string   `json:"row"`
	Kinds []string `json:"kinds"`
}

// QualQuery binds one registered query source by sha256.
type QualQuery struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}

// QualCase is one registered case of a workload: its exact input, edits and expectations,
// the status it must end with ("" is COMPLETED), its role (requirement, detector or
// support) and the requirement rows and kinds it covers under CoverageRule. Source is the
// UTF-8 input, kept only when query expectations are recomputed from it.
type QualCase struct {
	ID           string                 `json:"id"`
	Role         string                 `json:"role"`
	Input        NativeInput            `json:"input"`
	Edits        []Edit                 `json:"edits"`
	Expect       []StepExpectation      `json:"expect"`
	QueryExpect  []QueryExpectation     `json:"query_expect"`
	Points       []NativePoint          `json:"points"`
	DynamicSQL   *DynamicSQLExpectation `json:"dynamic_sql_expect"`
	ExpectStatus string                 `json:"expect_status"`
	Covers       map[string][]string    `json:"covers"`
	Source       *string                `json:"source"`
}

// QualWorkload is the registered workload a host runs for one cell or role: the record set
// directory under records/, the workload profile under profiles/ and the semantic profile
// fields the host profile must match exactly (host compiler and profile id excluded).
type QualWorkload struct {
	Set          string        `json:"set"`
	Profile      string        `json:"profile"`
	Route        string        `json:"route"`
	Operation    string        `json:"operation"`
	Output       string        `json:"output"`
	Symbol       string        `json:"symbol"`
	Format       string        `json:"format"`
	Grammar      []NativeInput `json:"grammar"`
	Declarations *Declarations `json:"declarations"`
	Queries      []QualQuery   `json:"queries"`
	FactPack     *FactPackRef  `json:"fact_pack"`
	API          bool          `json:"api"`
	Cases        []QualCase    `json:"cases"`
}

// QualRoute is one mandatory syntax route: its workload and required feature rows.
type QualRoute struct {
	Route        string            `json:"route"`
	Workload     QualWorkload      `json:"workload"`
	Requirements []QualRequirement `json:"requirements"`
}

// QualRole is one separately named extra-role row: never one of the mandatory cells.
type QualRole struct {
	ID            string            `json:"id"`
	Role          string            `json:"role"` // maintained | historical | owned
	Status        string            `json:"status"`
	Reason        string            `json:"reason"`
	Platforms     []string          `json:"platforms"`
	NotApplicable map[string]string `json:"not_applicable"`
	Workloads     []QualWorkload    `json:"workloads"`
}

// QualificationInventory is a decoded tsgk-qualification-inventory/r1 document: the exact
// route × platform cell set and the registered workloads, from the adopted registries.
type QualificationInventory struct {
	SHA256       string         `json:"-"`
	Schema       string         `json:"schema"`
	ID           string         `json:"id"`
	Campaign     string         `json:"campaign"`
	Cells        int            `json:"qualification_cells"`
	CoverageRule string         `json:"coverage_rule"`
	Kinds        []string       `json:"kinds"`
	Platforms    []QualPlatform `json:"platforms"`
	Routes       []QualRoute    `json:"routes"`
	ExtraRoles   []QualRole     `json:"extra_roles"`
}

var (
	hex64   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hex40   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	rowName = regexp.MustCompile(`^[a-z][a-z-]*-[A-Z][0-9]+[a-z]*$`)
)

// ParseQualificationInventory strictly decodes and checks an inventory: the declared cell
// count is the exact route × platform product, identifiers are unique, every coverage
// claim names a required row and kind and obeys CoverageRule, and recomputed sources match
// their inputs.
func ParseQualificationInventory(data []byte) (QualificationInventory, error) {
	inv, e := parseInventory(data)
	if e != nil {
		return inv, e
	}
	return inv, nil
}

func parseInventory(data []byte) (QualificationInventory, *Error) {
	var inv QualificationInventory
	const doc = "inventory"
	if e := strictDoc(doc, data, QualificationInventorySchema); e != nil {
		return inv, e
	}
	if e := decodeTyped(doc, data, &inv); e != nil {
		return inv, e
	}
	inv.SHA256 = digestHex(data)
	bad := func(code, path string) (QualificationInventory, *Error) {
		return inv, fail(KindInvalidInput, code, doc+"#/"+path, nil)
	}
	if !validID(inv.ID) || inv.Campaign == "" || inv.CoverageRule != CoverageRule || !slices.Equal(inv.Kinds, caseKinds) {
		return bad("INVENTORY_HEADER_INVALID", "id")
	}
	if len(inv.Platforms) == 0 || len(inv.Routes) == 0 || inv.Cells != len(inv.Platforms)*len(inv.Routes) {
		return bad("CELL_COUNT_MISMATCH", "qualification_cells")
	}
	plat := map[string]bool{}
	for i, p := range inv.Platforms {
		if !validID(p.ID) || p.GOOS == "" || p.GOARCH == "" || plat[p.ID] {
			return bad("PLATFORM_INVALID", fmt.Sprintf("platforms/%d", i))
		}
		plat[p.ID] = true
	}
	sets, routes := map[string]bool{}, map[string]bool{}
	checkWorkload := func(w QualWorkload, path string, reqs map[string][]string, role bool) *Error {
		badW := func(code, sub string) *Error { return fail(KindInvalidInput, code, doc+"#/"+path+sub, nil) }
		if !validID(w.Set) || !validID(w.Profile) || sets[w.Set] || !validID(w.Route) || w.Operation == "" || w.Output == "" || !ValidLanguageSymbol(w.Symbol) {
			return badW("WORKLOAD_INVALID", "")
		}
		sets[w.Set] = true
		if len(w.Grammar) == 0 {
			return badW("WORKLOAD_INVALID", "/grammar")
		}
		for _, g := range w.Grammar {
			if !hex64.MatchString(g.SHA256) || g.Path == "" {
				return badW("WORKLOAD_INVALID", "/grammar")
			}
		}
		qs := map[string]bool{}
		for _, q := range w.Queries {
			if !validID(q.ID) || qs[q.ID] || !hex64.MatchString(q.SHA256) {
				return badW("WORKLOAD_INVALID", "/queries")
			}
			qs[q.ID] = true
		}
		ids := map[string]bool{}
		for i, c := range w.Cases {
			cp := fmt.Sprintf("/cases/%d", i)
			if !validID(c.ID) || ids[c.ID] || !hex64.MatchString(c.Input.SHA256) {
				return badW("CASE_INVALID", cp)
			}
			ids[c.ID] = true
			switch c.Role {
			case "requirement":
			case "support":
				if len(c.Covers) > 0 {
					return badW("SUPPORT_COVERS", cp)
				}
			case "detector":
				if len(c.Covers) > 0 {
					return badW("DETECTOR_COVERS", cp) // a historical detector never satisfies a mainstream requirement
				}
				if !role {
					return badW("DETECTOR_IN_ROUTE", cp) // detectors belong to a separately named role row
				}
			default:
				return badW("CASE_INVALID", cp+"/role")
			}
			for _, e := range c.Expect {
				if e.Step < 0 || e.Step > len(c.Edits) {
					return badW("CASE_STEP_INVALID", cp)
				}
			}
			for _, q := range c.QueryExpect {
				if q.Step < 0 || q.Step > len(c.Edits) {
					return badW("CASE_STEP_INVALID", cp)
				}
			}
			if c.ExpectStatus != "" && (c.ExpectStatus != StatusResourceLimit || len(c.Covers) > 0) {
				return badW("CASE_INVALID", cp+"/expect_status") // an over-limit case judges no row
			}
			for _, q := range c.QueryExpect {
				if !qs[q.Query] {
					return badW("CASE_QUERY_UNKNOWN", cp)
				}
			}
			if len(c.QueryExpect) > 0 {
				if c.Source == nil || !utf8.ValidString(*c.Source) || digestHex([]byte(*c.Source)) != c.Input.SHA256 || uint64(len(*c.Source)) != c.Input.Bytes {
					return badW("CASE_SOURCE_MISMATCH", cp)
				}
			}
			for _, row := range sortedKeys(c.Covers) {
				want, ok := reqs[row]
				if !ok {
					return badW("COVERS_UNKNOWN_ROW", cp)
				}
				for _, k := range c.Covers[row] {
					if !slices.Contains(want, k) || !coverageHolds(c, k) {
						return badW("COVERAGE_RULE_VIOLATION", cp)
					}
				}
			}
		}
		return nil
	}
	for i, r := range inv.Routes {
		if !validID(r.Route) || routes[r.Route] || r.Workload.Route != r.Route {
			return bad("ROUTE_INVALID", fmt.Sprintf("routes/%d", i))
		}
		routes[r.Route] = true
		reqs := map[string][]string{}
		for j, q := range r.Requirements {
			if !rowName.MatchString(q.Row) || !strings.HasPrefix(q.Row, r.Route+"-") || reqs[q.Row] != nil || len(q.Kinds) == 0 {
				return bad("REQUIREMENT_INVALID", fmt.Sprintf("routes/%d/requirements/%d", i, j))
			}
			for _, k := range q.Kinds {
				if !slices.Contains(caseKinds, k) {
					return bad("REQUIREMENT_INVALID", fmt.Sprintf("routes/%d/requirements/%d", i, j))
				}
			}
			reqs[q.Row] = q.Kinds
		}
		if len(r.Requirements) == 0 {
			return bad("REQUIREMENT_INVALID", fmt.Sprintf("routes/%d/requirements", i)) // a route without rows would pass vacuously
		}
		if e := checkWorkload(r.Workload, fmt.Sprintf("routes/%d/workload", i), reqs, false); e != nil {
			return inv, e
		}
	}
	roleIDs := map[string]bool{}
	for i, x := range inv.ExtraRoles {
		p := fmt.Sprintf("extra_roles/%d", i)
		if !validID(x.ID) || roleIDs[x.ID] || routes[x.ID] || (x.Role != "maintained" && x.Role != "historical" && x.Role != "owned") || x.Reason == "" {
			return bad("ROLE_INVALID", p)
		}
		roleIDs[x.ID] = true
		for _, pl := range x.Platforms {
			if !plat[pl] {
				return bad("ROLE_INVALID", p+"/platforms")
			}
		}
		for pl := range x.NotApplicable {
			if !plat[pl] || slices.Contains(x.Platforms, pl) {
				return bad("ROLE_INVALID", p+"/not_applicable")
			}
		}
		switch x.Status {
		case RoleExecuted:
			if len(x.Workloads) == 0 || len(x.Platforms) == 0 {
				return bad("ROLE_INVALID", p)
			}
			for j, w := range x.Workloads {
				if e := checkWorkload(w, fmt.Sprintf("%s/workloads/%d", p, j), map[string][]string{}, true); e != nil {
					return inv, e
				}
			}
		case RoleNotRun, RoleExternal:
			if len(x.Workloads) > 0 {
				return bad("ROLE_INVALID", p)
			}
		default:
			return bad("ROLE_INVALID", p+"/status")
		}
	}
	return inv, nil
}

// coverageHolds applies CoverageRule to one registered case and kind.
func coverageHolds(c QualCase, kind string) bool {
	switch kind {
	case "P":
		return slices.ContainsFunc(c.Expect, func(e StepExpectation) bool { return e.Syntax == "NO_ERROR" && len(e.Contains) > 0 })
	case "N":
		return slices.ContainsFunc(c.Expect, func(e StepExpectation) bool { return e.Syntax == "ERROR" })
	case "R":
		return slices.ContainsFunc(c.Expect, func(e StepExpectation) bool { return e.Syntax == "ERROR" && len(e.Contains) > 0 })
	case "E":
		return len(c.Edits) > 0
	case "Q":
		return slices.ContainsFunc(c.QueryExpect, func(q QueryExpectation) bool { return q.Captures != nil })
	}
	return false // W: no registered producer
}

// RunIdentity is a host's tsgk-run-identity/r1 document, written by the run itself: the
// repository run and attempt, the checkout, the actual Go host and the evidence mode.
type RunIdentity struct {
	Schema       string `json:"schema"`
	Repository   string `json:"repository"`
	Workflow     string `json:"workflow"`
	RunID        string `json:"run_id"`
	RunAttempt   string `json:"run_attempt"`
	Event        string `json:"event"`
	SHA          string `json:"sha"`
	HeadSHA      string `json:"head_sha"`
	Checkout     string `json:"checkout"`
	Job          string `json:"job"`
	RunnerOS     string `json:"runner_os"`
	RunnerArch   string `json:"runner_arch"`
	Image        string `json:"image"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	GoVersion    string `json:"go_version"`
	EvidenceMode string `json:"evidence_mode"`
}

// cohort is the part of the run identity every host of one qualification shares.
func (r RunIdentity) cohort() string {
	return strings.Join([]string{r.Repository, r.Workflow, r.RunID, r.RunAttempt, r.Event, r.SHA, r.HeadSHA}, "\x00")
}

// QualifyHost names one host's evidence directory (run-identity.json, profiles/,
// records/ and the retained summary.json) by registered platform id.
type QualifyHost struct {
	Platform string
	Root     string
}

// QualifyRequest is the inventory bytes, the candidate commit every host must have run and
// the host directories.
type QualifyRequest struct {
	Inventory []byte
	Candidate string
	Hosts     []QualifyHost
}

// QualObligation is one required row × kind of a cell with its result and covering cases.
type QualObligation struct {
	Row    string   `json:"row"`
	Kind   string   `json:"kind"`
	Result string   `json:"result"` // PASS | FAIL | BLOCKED | NOT_COVERED
	Cases  []string `json:"cases"`
}

// QualCounts counts a cell's obligations by result.
type QualCounts struct {
	Obligations int `json:"obligations"`
	Pass        int `json:"pass"`
	Fail        int `json:"fail"`
	Blocked     int `json:"blocked"`
	NotCovered  int `json:"not_covered"`
}

// QualSet is the evaluation of one workload's record set on one host. Mechanism is the kit
// axis (set integrity, registration, S07 gates, kit claims and expected statuses); host
// observations are retained apart and never compared.
type QualSet struct {
	Set        string            `json:"set"`
	Platform   string            `json:"platform"`
	Evidence   string            `json:"evidence"` // NEW_RUN | MISSING | REJECTED
	Mechanism  string            `json:"mechanism"`
	Manifest   string            `json:"manifest_sha256"`
	Profile    string            `json:"profile_sha256"`
	Records    int               `json:"records"`
	Gates      []ReplayGate      `json:"gates"`
	APIFails   int               `json:"api_claim_failures"`
	Detectors  map[string]string `json:"detectors"`
	Host       map[string]string `json:"host_observations"`
	Identities map[string]string `json:"identities"`
	Findings   []Finding         `json:"findings"`
}

// QualCell is one of the mandatory route × platform cells with its two axes and the
// route's cross-platform comparison.
type QualCell struct {
	Route       string           `json:"route"`
	Platform    string           `json:"platform"`
	Status      string           `json:"status"`
	Mechanism   string           `json:"mechanism"`
	Requirement string           `json:"requirement"`
	Comparison  string           `json:"comparison"`
	Counts      QualCounts       `json:"counts"`
	Obligations []QualObligation `json:"obligations"`
	// Checks folds every registered expectation of the cell's cases (also steps and cases
	// that cover no row kind): a FAIL fails the requirement axis, a BLOCKED leaves it
	// INCOMPLETE. CheckFailures names the cases that did not pass.
	Checks        string   `json:"registered_checks"`
	CheckFailures []string `json:"check_failures"`
	Set           QualSet  `json:"set"`
}

// QualRoleRow is one extra-role row on one platform.
type QualRoleRow struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Platform string `json:"platform"`
	Status   string `json:"status"` // PASS | FAIL | INCOMPLETE | MISSING | NOT_RUN | EXTERNAL | NOT_APPLICABLE
	Reason   string `json:"reason"`
	// Mechanism is the worst kit axis of the row's sets; Checks folds their registered
	// expectations and detector results ("" when the row executes nothing).
	Mechanism string    `json:"mechanism"`
	Checks    string    `json:"registered_checks"`
	Sets      []QualSet `json:"sets"`
}

// QualComparison is the cross-platform semantic comparison of one workload.
type QualComparison struct {
	Set         string   `json:"set"`
	Platforms   []string `json:"platforms"`
	Result      string   `json:"result"` // PASS | FAIL | NOT_ASSESSED (fewer than two hosts)
	Differences []string `json:"differences"`
}

// QualTotals summarizes the cells.
type QualTotals struct {
	Cells            int            `json:"cells"`
	Status           map[string]int `json:"status"`
	MechanismPass    int            `json:"mechanism_pass"`
	RequirementPass  int            `json:"requirement_pass"`
	ComparisonFail   int            `json:"comparison_fail"`
	Obligations      QualCounts     `json:"obligations"`
	APIClaimFailures int            `json:"api_claim_failures"`
	NotCoveredByKind map[string]int `json:"not_covered_by_kind"`
}

// QualificationResult is the tsgk-qualification-result/r2 document: the E0 envelope plus
// the candidate, the run cohort, every mandatory cell, every extra-role row, the
// comparisons, the support claim, which is SUPPORTED only when every cell passes, and the
// per-platform claims (r2 adds platform_claims to r1).
type QualificationResult struct {
	Report
	ResultSchema string            `json:"result_schema"`
	InventoryID  string            `json:"inventory_id"`
	Candidate    string            `json:"candidate"`
	Limits       ReplayLimits      `json:"limits"`
	Run          *RunIdentity      `json:"run"`
	Hosts        map[string]string `json:"hosts"` // platform → run identity sha256
	Completeness string            `json:"completeness"`
	Cells        []QualCell        `json:"cells"`
	ExtraRoles   []QualRoleRow     `json:"extra_roles"`
	Comparisons  []QualComparison  `json:"comparisons"`
	Totals       QualTotals        `json:"totals"`
	// MechanismGate is PASS when every kit-side check passed: completeness, cohort and
	// eligibility, every cell's mechanism and comparison, every executed extra-role row.
	// Requirement failures and uncovered obligations (grammar gaps, missing cases) do not
	// change it; they keep SupportClaim BLOCKED.
	MechanismGate string `json:"mechanism_gate"`
	SupportClaim  string `json:"support_claim"`
	// PlatformClaims is the support claim of each registered platform (SUPPORTED or
	// BLOCKED): global evidence integrity (completeness PASS, no finding), every route's
	// single cell of the platform with mechanism and requirement PASS, and every executed
	// extra-role row of the platform with mechanism and status PASS. The cross-platform
	// comparison stays in SupportClaim only.
	PlatformClaims map[string]string `json:"platform_claims"`
	BytesRead      uint64            `json:"bytes_read"`
	Explanation    []string          `json:"explanation"`
}

// testHookQualifyLimits lets a test lower the qualification limits.
var testHookQualifyLimits func(ReplayLimits) ReplayLimits

// Qualify aggregates one candidate's host runs into the inventory's mandatory cells and
// extra-role rows. It reads only the host directories, starts no process and never
// fills a cell from another platform, attempt, candidate or replay. Every host must be a
// NEW_RUN of the candidate commit in one run cohort, on its registered OS/architecture.
// Each record set is verified (manifest, members, registration against the inventory,
// the S07 record gates) before its cases are judged; a cell passes only when its kit
// mechanism, every required obligation and the cross-platform comparison pass.
func Qualify(ctx context.Context, req QualifyRequest) (QualificationResult, error) {
	res := QualificationResult{Report: newReport("qualify"), ResultSchema: QualificationResultSchema, Hosts: map[string]string{}, Cells: []QualCell{},
		ExtraRoles: []QualRoleRow{}, Comparisons: []QualComparison{}, Explanation: []string{}, SupportClaim: "BLOCKED", MechanismGate: AssessFail, PlatformClaims: map[string]string{}}
	res.Assessment = AssessNotAssessed
	bad := func(e *Error) (QualificationResult, error) {
		failReport(&res.Report, e)
		res.PlatformClaims = map[string]string{} // a run that did not complete claims no platform
		res.Explanation = append(res.Explanation, explainFailure(e))
		return res, e
	}
	inv, e := parseInventory(req.Inventory)
	if e != nil {
		return bad(e)
	}
	if !hex40.MatchString(req.Candidate) {
		return bad(fail(KindInvalidInput, "CANDIDATE_INVALID", "candidate", nil))
	}
	res.InventoryID, res.Candidate = inv.ID, req.Candidate
	lim := QualificationLimits()
	if testHookQualifyLimits != nil {
		lim = testHookQualifyLimits(lim)
	}
	res.Limits = lim
	res.Identities = append(res.Identities, IdentityRef{Role: "inventory", Schema: QualificationInventorySchema, SHA256: inv.SHA256},
		IdentityRef{Role: "policy", Schema: "tsgk-qualification-policy/r1", SHA256: digestHex([]byte(replayPolicyText("qualification", lim)))})
	res.Coverage.Requested = []string{"completeness", "run-cohort", "eligibility", "set-integrity", "registration", "record-gates", "mechanism", "requirements", "comparison"}
	r, e := startRun(ctx, lim.Wall)
	if e != nil {
		return bad(e)
	}
	defer r.cancel()
	q := &qualifier{r: r, lim: lim, inv: inv, candidate: req.Candidate, hosts: map[string]*qhost{}}
	defer q.close()
	completeness := true
	for i, h := range req.Hosts {
		idx := slices.IndexFunc(inv.Platforms, func(p QualPlatform) bool { return p.ID == h.Platform })
		switch {
		case idx < 0:
			q.finding("HOST_UNREGISTERED", fmt.Sprintf("hosts/%d", i), "inventory에 없는 platform의 host다")
			completeness = false
			continue
		case q.hosts[h.Platform] != nil:
			q.finding("CELL_DUPLICATE", h.Platform, "같은 platform의 host가 둘 이상이다(중복 행)")
			completeness = false
			continue
		}
		hx, e := q.openHost(inv.Platforms[idx], h.Root)
		if e != nil {
			res.BytesRead = q.bytesRead()
			return bad(e)
		}
		q.hosts[h.Platform] = hx
	}
	// one run cohort: the same repository run, attempt, event, checkout and head on every host
	var first *qhost
	for _, eligible := range []bool{true, false} {
		for _, p := range inv.Platforms {
			if hx := q.hosts[p.ID]; first == nil && hx != nil && hx.id != nil && (len(hx.rejected) == 0) == eligible {
				first = hx
			}
		}
	}
	if first != nil {
		res.Run = first.id
	}
	for _, p := range inv.Platforms {
		hx := q.hosts[p.ID]
		if hx == nil || hx.id == nil {
			continue
		}
		res.Hosts[p.ID] = hx.idSHA
		if hx != first && hx.id.cohort() != first.id.cohort() {
			hx.reject("COHORT_MISMATCH", "run-identity.json", "다른 run·attempt·event·checkout의 host를 한 qualification에 섞었다")
		}
	}
	if e := r.check(); e != nil {
		res.BytesRead = q.bytesRead()
		return bad(e)
	}
	comparisons := map[string]*QualComparison{}
	for _, route := range inv.Routes {
		sets, cmp, e := q.workload(route.Workload, inv.Platforms)
		if e != nil {
			res.BytesRead = q.bytesRead()
			return bad(e)
		}
		comparisons[route.Workload.Set] = cmp
		res.Comparisons = append(res.Comparisons, *cmp)
		for _, p := range inv.Platforms {
			res.Cells = append(res.Cells, q.cell(route, p, sets[p.ID], cmp))
		}
	}
	for _, x := range inv.ExtraRoles {
		rows, cmps, e := q.role(x, inv.Platforms)
		if e != nil {
			res.BytesRead = q.bytesRead()
			return bad(e)
		}
		res.ExtraRoles = append(res.ExtraRoles, rows...)
		res.Comparisons = append(res.Comparisons, cmps...)
	}
	// completeness: exactly the inventory cells, each with one evidence set
	cells := map[string]int{}
	for _, c := range res.Cells {
		cells[c.Route+"@"+c.Platform]++
		if c.Status == CellMissing {
			completeness = false
		}
	}
	if len(res.Cells) != inv.Cells || len(cells) != inv.Cells {
		completeness = false
	}
	for _, x := range res.ExtraRoles {
		if x.Status == CellMissing {
			completeness = false
		}
	}
	for _, pl := range inv.Platforms {
		hx := q.hosts[pl.ID]
		if hx == nil {
			continue
		}
		for _, p := range sortedKeys(hx.files) {
			if !hx.used[p] {
				if hx.unused++; hx.unused <= 10 {
					q.finding("HOST_FILE_UNREGISTERED", hx.platform.ID+"/"+p, "inventory workload에 속하지 않는 파일이 host 근거에 있다")
				}
				completeness = false
			}
		}
	}
	res.Completeness = map[bool]string{true: AssessPass, false: AssessFail}[completeness]
	if !completeness {
		q.finding("COMPLETENESS_FAILED", "", "필수 칸이 빠졌거나 중복됐거나 등록되지 않은 근거가 있다")
	}
	res.Totals = totals(res.Cells)
	failed := !completeness || len(q.findings) > 0
	res.PlatformClaims = platformClaims(inv, res.Cells, res.ExtraRoles, !failed)
	gate := !failed
	allPass := true
	for _, c := range res.Cells {
		failed = failed || c.Status == CellFail
		allPass = allPass && c.Status == CellPass
		gate = gate && c.Mechanism == AssessPass && c.Comparison == AssessPass
	}
	for _, x := range res.ExtraRoles {
		failed = failed || x.Status == CellFail
		if x.Mechanism != "" {
			gate = gate && x.Mechanism == AssessPass
			allPass = allPass && x.Status == CellPass
		}
	}
	res.MechanismGate = map[bool]string{true: AssessPass, false: AssessFail}[gate]
	allPass = allPass && gate // SUPPORTED needs every kit-side check, not only the cells
	switch {
	case failed:
		res.Assessment = AssessFail
	case allPass:
		res.Assessment, res.SupportClaim = AssessPass, "SUPPORTED"
	default:
		res.Assessment = AssessBlocked
	}
	res.Findings = append(res.Findings, q.findings...)
	res.Coverage.Observed = slices.Clone(res.Coverage.Requested)
	res.BytesRead = q.bytesRead()
	res.Explanation = explainQualification(res, inv.Platforms)
	if e := sealOutput(res, lim.OutputBytes); e != nil {
		return bad(e)
	}
	if e := r.check(); e != nil {
		return bad(e)
	}
	return res, nil
}

// platformClaims decides each platform's support claim. integrity is global (completeness
// PASS and no finding): a finding means the evidence set itself cannot be trusted. Each
// route must have exactly one cell of the platform with mechanism and requirement PASS,
// and every executed extra-role row of the platform (non-empty mechanism) must have
// mechanism and status PASS; NOT_RUN, EXTERNAL and NOT_APPLICABLE rows do not block. The
// cross-platform comparison is not part of it.
func platformClaims(inv QualificationInventory, cells []QualCell, roles []QualRoleRow, integrity bool) map[string]string {
	out := map[string]string{}
	for _, p := range inv.Platforms {
		ok := integrity
		for _, r := range inv.Routes {
			n := 0
			for _, c := range cells {
				if c.Route == r.Route && c.Platform == p.ID {
					n++
					ok = ok && c.Mechanism == AssessPass && c.Requirement == AssessPass
				}
			}
			ok = ok && n == 1
		}
		for _, x := range roles {
			if x.Platform == p.ID && x.Mechanism != "" {
				ok = ok && x.Mechanism == AssessPass && x.Status == CellPass
			}
		}
		out[p.ID] = map[bool]string{true: "SUPPORTED", false: "BLOCKED"}[ok]
	}
	return out
}

// cell folds one route × platform evaluation into the cell's axes.
func (q *qualifier) cell(route QualRoute, p QualPlatform, s *qset, cmp *QualComparison) QualCell {
	c := QualCell{Route: route.Route, Platform: p.ID, Comparison: cmp.Result, Obligations: []QualObligation{}, CheckFailures: []string{}}
	c.Set = s.QualSet
	if c.Set.Evidence == "MISSING" {
		c.Status, c.Mechanism, c.Requirement = CellMissing, AssessNotAssessed, AssessNotAssessed
		return c
	}
	c.Mechanism = s.Mechanism
	notCovered, failedOb, blocked := 0, 0, 0
	for _, req := range route.Requirements {
		for _, k := range req.Kinds {
			ob := QualObligation{Row: req.Row, Kind: k, Result: ObligationNotCovered, Cases: []string{}}
			for _, qc := range route.Workload.Cases {
				if qc.Role != "requirement" || !slices.Contains(qc.Covers[req.Row], k) {
					continue
				}
				ob.Cases = append(ob.Cases, qc.ID)
				v := s.kinds[qc.ID][k]
				if v == "" {
					v = claimBlocked // the case's record was not judged (rejected or absent)
				}
				if ob.Result == ObligationNotCovered {
					ob.Result = v
				} else {
					ob.Result = worseClaim(ob.Result, v)
				}
			}
			switch ob.Result {
			case claimPass:
				c.Counts.Pass++
			case claimFail:
				c.Counts.Fail++
				failedOb++
			case claimBlocked:
				c.Counts.Blocked++
				blocked++
			default:
				c.Counts.NotCovered++
				notCovered++
			}
			c.Counts.Obligations++
			c.Obligations = append(c.Obligations, ob)
		}
	}
	c.Checks, c.CheckFailures = foldChecks(route.Workload.Cases, s)
	switch {
	case failedOb > 0 || c.Checks == claimFail:
		c.Requirement = AssessFail
	case notCovered+blocked > 0 || c.Checks != claimPass:
		c.Requirement = CellIncomplete
	default:
		c.Requirement = AssessPass
	}
	switch {
	case c.Mechanism == AssessFail || c.Requirement == AssessFail || c.Comparison == AssessFail:
		c.Status = CellFail
	case c.Mechanism != AssessPass || c.Requirement == CellIncomplete || c.Comparison != AssessPass:
		c.Status = CellIncomplete
	default:
		c.Status = CellPass
	}
	return c
}

// foldChecks folds the registered-check result of every non-detector case of a workload;
// a case without a judged record is BLOCKED.
func foldChecks(cases []QualCase, s *qset) (string, []string) {
	out, bad := claimPass, []string{}
	for _, qc := range cases {
		if qc.Role == "detector" {
			continue
		}
		v := s.checks[qc.ID]
		if v == "" {
			v = claimBlocked
		}
		if v != claimPass && len(bad) < 50 {
			bad = append(bad, qc.ID+"="+v)
		}
		out = worseClaim(out, v)
	}
	return out, bad
}

func totals(cells []QualCell) QualTotals {
	t := QualTotals{Cells: len(cells), Status: map[string]int{}, NotCoveredByKind: map[string]int{}}
	for _, c := range cells {
		t.Status[c.Status]++
		if c.Mechanism == AssessPass {
			t.MechanismPass++
		}
		if c.Requirement == AssessPass {
			t.RequirementPass++
		}
		if c.Comparison == AssessFail {
			t.ComparisonFail++
		}
		t.Obligations.Obligations += c.Counts.Obligations
		t.Obligations.Pass += c.Counts.Pass
		t.Obligations.Fail += c.Counts.Fail
		t.Obligations.Blocked += c.Counts.Blocked
		t.Obligations.NotCovered += c.Counts.NotCovered
		t.APIClaimFailures += c.Set.APIFails
		for _, o := range c.Obligations {
			if o.Result == ObligationNotCovered {
				t.NotCoveredByKind[o.Kind]++
			}
		}
	}
	return t
}

func explainQualification(res QualificationResult, platforms []QualPlatform) []string {
	t := res.Totals
	out := []string{
		fmt.Sprintf("필수 칸 %d개: PASS %d, FAIL %d, INCOMPLETE %d, MISSING %d. 완결성 %s.", t.Cells, t.Status[CellPass], t.Status[CellFail], t.Status[CellIncomplete], t.Status[CellMissing], res.Completeness),
		fmt.Sprintf("kit 축 PASS %d칸, 요구 축 PASS %d칸, 세 platform 비교 FAIL %d칸.", t.MechanismPass, t.RequirementPass, t.ComparisonFail),
		fmt.Sprintf("필수 의무 %d개: PASS %d, FAIL %d, BLOCKED %d, 사례 없음 %d.", t.Obligations.Obligations, t.Obligations.Pass, t.Obligations.Fail, t.Obligations.Blocked, t.Obligations.NotCovered),
		fmt.Sprintf("kit 검사 gate %s(완결성·cohort·자격·칸별 kit 축·비교·실행된 추가 역할 행의 kit 축).", res.MechanismGate),
	}
	if res.SupportClaim != "SUPPORTED" {
		out = append(out, "지원 claim은 BLOCKED다: 모든 필수 칸과 실행된 추가 역할 행이 PASS이고 kit 검사 gate가 PASS일 때만 SUPPORTED다. 추가 역할 행은 필수 칸을 대신하지 않는다.")
	}
	var pc []string
	for _, p := range platforms {
		pc = append(pc, p.ID+" "+res.PlatformClaims[p.ID])
	}
	out = append(out, "platform별 claim: "+strings.Join(pc, ", ")+". 근거 무결성(완결성 PASS, finding 없음)과 그 platform의 모든 칸 kit 축·요구 축 PASS, 실행된 추가 역할 행 PASS일 때만 SUPPORTED이며 platform 간 비교는 넣지 않는다.")
	return out
}
