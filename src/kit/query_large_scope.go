package kit

import (
	"crypto/sha256"
	"encoding/hex"
	jsonv2 "encoding/json/v2"
	"fmt"
	"slices"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The large scope (#362 scope 6) registers the denominator that the P0 registry leaves open for
// the Windows large workloads (`large-node-occurrence-page-footer` is a missing scope there): per
// registered fixture the source, the status, the full-CST pages and footer and the query
// occurrence pages and footer, plus the over-limit control. The independent expected values come
// from the fixture template (query_large_template.go); a fixture whose tree depends on parser
// error recovery has no derivable CST and its page obligations are registered BLOCKED. The P0
// registry, its IDs and the large operation limits are not changed.

const (
	LargeExpectedSchema = "tsgk-query-large-expected/r1"
	LargeExpectedPath   = "src/contracts/query-large-expected-r1.json"
	LargeFixturesPath   = "src/contracts/native-large-fixtures.json"
	// shortcut: the asset holds digests only (a few KB); raise only with a new expected revision.
	largeExpectedMaxBytes = 64 * 1024
	// LargePageRows is the CST page size (the existing large session page size, 1,000,000 nodes).
	LargePageRows = 1000000
	// LargeOccurrencePage is the occurrence page size in captures.
	LargeOccurrencePage = 10000
	largeHost           = "windows-amd64"
)

// Obligation kinds of the large denominator.
const (
	LargeKindSource    = "LARGE_SOURCE"
	LargeKindStatus    = "LARGE_STATUS"
	LargeKindCSTPage   = "LARGE_CST_PAGE"
	LargeKindCSTFooter = "LARGE_CST_FOOTER"
	LargeKindOccPage   = "LARGE_OCC_PAGE"
	LargeKindOccFooter = "LARGE_OCC_FOOTER"
	LargeKindControl   = "LARGE_CONTROL"
)

// Derivation values of a fixture's expected asset.
const (
	LargeDerivedByTemplate = "TEMPLATE_RULES"
	LargeNotDerivableState = "NOT_DERIVABLE"
)

type LargeExpectedCST struct {
	PageRows      uint64   `json:"page_rows"`
	Nodes         uint64   `json:"nodes"`
	PageDigests   []string `json:"page_digests"`
	PagesSHA256   string   `json:"pages_sha256"`
	AllRowsSHA256 string   `json:"all_rows_sha256"`
	RowBytes      uint64   `json:"row_bytes"`
	MaxDepth      uint32   `json:"max_depth"`
	HasError      bool     `json:"has_error"`
}

type LargeExpectedOccurrences struct {
	PageCaptures uint64            `json:"page_captures"`
	Total        uint64            `json:"total"`
	Pages        uint64            `json:"pages"`
	ByName       map[string]uint64 `json:"by_name"`
	SHA256       string            `json:"multiset_sha256"`
}

type LargeExpectedFixture struct {
	ID          string                    `json:"id"`
	Set         string                    `json:"set"`
	Bytes       uint64                    `json:"bytes"`
	SHA256      string                    `json:"sha256"`
	Repeat      int                       `json:"repeat"`
	Syntax      string                    `json:"syntax"`
	Derivation  string                    `json:"derivation"`
	Code        string                    `json:"code"`
	CST         *LargeExpectedCST         `json:"cst"`
	Occurrences *LargeExpectedOccurrences `json:"occurrences"`
}

// LargeExpectedControl is the over-limit control: the exact status and code are the expectation and
// SemanticPass stays false (a resource limit is never a semantic PASS).
type LargeExpectedControl struct {
	ID           string `json:"id"`
	Set          string `json:"set"`
	SourceCase   string `json:"source_case"`
	Query        string `json:"query"`
	QuerySHA256  string `json:"query_sha256"`
	Status       string `json:"status"`
	Code         string `json:"code"`
	SemanticPass bool   `json:"semantic_pass"`
	Basis        string `json:"basis"`
}

// LargeExpected is the strict decoded tsgk-query-large-expected/r1 asset.
type LargeExpected struct {
	Schema        string                 `json:"schema"`
	Registry      queryregistrycore.Pin  `json:"registry"`
	Qualification queryregistrycore.Pin  `json:"qualification"`
	Fixtures      queryregistrycore.Pin  `json:"fixture_inventory"`
	Grammar       string                 `json:"grammar_basis"`
	Basis         string                 `json:"basis"`
	NotApplicable map[string]string      `json:"not_applicable"`
	Cases         []LargeExpectedFixture `json:"cases"`
	Control       LargeExpectedControl   `json:"control"`
}

type largeFixtureFile struct {
	Schema    string `json:"schema"`
	Route     string `json:"route"`
	Operation string `json:"operation"`
	Note      string `json:"note"`
	Fixtures  []struct {
		ID     string        `json:"id"`
		Header string        `json:"header"`
		Block  string        `json:"block"`
		Repeat int           `json:"repeat"`
		Footer string        `json:"footer"`
		Bytes  uint64        `json:"bytes"`
		SHA256 string        `json:"sha256"`
		Points []NativePoint `json:"points"`
		Expect []struct {
			Syntax       string `json:"syntax"`
			Declarations string `json:"declarations"`
		} `json:"expect"`
	} `json:"fixtures"`
}

// LargeTemplates decodes the registered fixture inventory into generation rules, in registry order,
// and the registered syntax expectation of each.
func LargeTemplates(fixtures []byte) ([]LargeTemplate, []string, error) {
	var f largeFixtureFile
	if err := jsonv2.Unmarshal(fixtures, &f, jsonv2.RejectUnknownMembers(true)); err != nil || f.Schema != "tsgk-native-large-fixtures/r1" || f.Route != "csharp" {
		return nil, nil, fail(KindInvalidInput, "LARGE_FIXTURES_JSON", LargeFixturesPath, err)
	}
	var out []LargeTemplate
	var syntax []string
	for _, x := range f.Fixtures {
		if len(x.Expect) != 1 {
			return nil, nil, fail(KindInvalidInput, "LARGE_FIXTURES_EXPECT", LargeFixturesPath, nil)
		}
		out = append(out, LargeTemplate{ID: x.ID, Header: x.Header, Block: x.Block, Footer: x.Footer, Repeat: x.Repeat, Bytes: x.Bytes, SHA256: x.SHA256})
		syntax = append(syntax, x.Expect[0].Syntax)
	}
	return out, syntax, nil
}

// largeInventoryRole returns the n461-large role of the pinned qualification inventory.
func largeInventoryRole(inventory []byte) (QualRole, error) {
	inv, err := ParseQualificationInventory(inventory)
	if err != nil {
		return QualRole{}, err
	}
	for _, role := range inv.ExtraRoles {
		if role.ID == "n461-large" && len(role.Workloads) == 2 {
			return role, nil
		}
	}
	return QualRole{}, largeFault("LARGE_EXPECTED_WORKLOAD")
}

// ParseLargeExpected decodes the asset and binds it to the exact pinned registry, qualification
// inventory and fixture inventory bytes: the three positive cases and the control in registry
// order, their sizes, hashes, syntax expectation, queries and the Linux/macOS N/A reasons.
func ParseLargeExpected(data, registry, inventory, fixtures []byte) (LargeExpected, error) {
	var e LargeExpected
	if len(data) > largeExpectedMaxBytes {
		return e, fail(KindResourceLimit, "LARGE_EXPECTED_SIZE", LargeExpectedPath, nil)
	}
	if err := jsonv2.Unmarshal(data, &e, jsonv2.RejectUnknownMembers(true)); err != nil {
		return e, fail(KindInvalidInput, "LARGE_EXPECTED_JSON", LargeExpectedPath, err)
	}
	if e.Schema != LargeExpectedSchema || e.Basis == "" || e.Grammar == "" {
		return e, largeFault("LARGE_EXPECTED_SCHEMA")
	}
	if e.Registry != queryregistrycore.FilePin(queryregistrycore.Path, registry) ||
		e.Qualification != queryregistrycore.FilePin(queryregistrycore.QualificationPin.Path, inventory) ||
		e.Fixtures != queryregistrycore.FilePin(LargeFixturesPath, fixtures) {
		return e, largeFault("LARGE_EXPECTED_PIN")
	}
	role, err := largeInventoryRole(inventory)
	if err != nil {
		return e, err
	}
	templates, syntax, err := LargeTemplates(fixtures)
	if err != nil {
		return e, err
	}
	positive, over := role.Workloads[0], role.Workloads[1]
	if len(positive.Cases) != 3 || len(over.Cases) != 1 || len(templates) != 3 || len(e.Cases) != 3 || len(positive.Queries) != 1 || len(over.Queries) != 1 {
		return e, largeFault("LARGE_EXPECTED_MEMBERSHIP")
	}
	if !slices.Equal(role.Platforms, []string{largeHost}) || len(e.NotApplicable) != len(role.NotApplicable) {
		return e, largeFault("LARGE_EXPECTED_PLATFORM")
	}
	for host, reason := range role.NotApplicable {
		if e.NotApplicable[host] != reason || host == largeHost {
			return e, largeFault("LARGE_EXPECTED_NOT_APPLICABLE")
		}
	}
	for i, c := range e.Cases {
		q, t := positive.Cases[i], templates[i]
		if c.ID != q.ID || c.ID != t.ID || c.Set != positive.Set || c.Bytes != q.Input.Bytes || c.Bytes != t.Bytes || c.SHA256 != q.Input.SHA256 || c.SHA256 != t.SHA256 ||
			c.Repeat != t.Repeat || c.Syntax != syntax[i] || len(q.Expect) != 1 || q.Expect[0].Syntax != c.Syntax {
			return e, largeFault("LARGE_EXPECTED_MEMBERSHIP")
		}
		switch {
		case c.Derivation == LargeDerivedByTemplate && c.Code == "" && c.CST != nil && c.Occurrences != nil:
			cst, occ := c.CST, c.Occurrences
			if cst.PageRows != LargePageRows || cst.Nodes == 0 || uint64(len(cst.PageDigests)) != 1+(cst.Nodes-1)/cst.PageRows || len(cst.PagesSHA256) != 64 || len(cst.AllRowsSHA256) != 64 ||
				cst.HasError != (c.Syntax == "ERROR") || occ.PageCaptures != LargeOccurrencePage || occ.Total == 0 || occ.Pages != 1+(occ.Total-1)/occ.PageCaptures || len(occ.SHA256) != 64 {
				return e, largeFault("LARGE_EXPECTED_DERIVED")
			}
			var sum uint64
			for _, n := range occ.ByName {
				sum += n
			}
			if sum != occ.Total {
				return e, largeFault("LARGE_EXPECTED_DERIVED")
			}
			for _, d := range cst.PageDigests {
				if len(d) != 64 {
					return e, largeFault("LARGE_EXPECTED_DERIVED")
				}
			}
		case c.Derivation == LargeNotDerivableState && c.Code == LargeNotDerivable && c.CST == nil && c.Occurrences == nil:
		default:
			return e, largeFault("LARGE_EXPECTED_DERIVATION")
		}
	}
	c, q := e.Control, over.Cases[0]
	if c.ID != q.ID || c.Set != over.Set || c.Query != over.Queries[0].ID || c.QuerySHA256 != over.Queries[0].SHA256 || c.Status != q.ExpectStatus || c.Status != StatusResourceLimit ||
		c.Code == "" || c.SemanticPass || c.Basis == "" || c.SourceCase != "cs-large-8m-errors" || e.Cases[1].ID != c.SourceCase || q.Input != e.Cases[1].inputOf() {
		return e, largeFault("LARGE_EXPECTED_CONTROL")
	}
	return e, nil
}

func (c LargeExpectedFixture) inputOf() NativeInput {
	return NativeInput{SHA256: c.SHA256, Bytes: c.Bytes}
}

// LargeObligation is one obligation of the large denominator. Blocked is the code of an
// obligation that is registered but cannot be judged (no independent expected value).
type LargeObligation struct {
	ID      string
	Kind    string
	Case    string
	Blocked string
}

func largeID(kind, c string, index int) string {
	return fmt.Sprintf("%s/%s/%s/%d", kind, "csharp", c, index)
}

// Obligations enumerates the denominator in deterministic order.
func (e LargeExpected) Obligations() []LargeObligation {
	var out []LargeObligation
	add := func(kind, c string, index int, blocked string) {
		out = append(out, LargeObligation{ID: largeID(kind, c, index), Kind: kind, Case: c, Blocked: blocked})
	}
	for _, c := range e.Cases {
		add(LargeKindSource, c.ID, 0, "")
		add(LargeKindStatus, c.ID, 0, "")
		if c.CST != nil {
			for i := range c.CST.PageDigests {
				add(LargeKindCSTPage, c.ID, i, "")
			}
			add(LargeKindCSTFooter, c.ID, 0, "")
			for i := uint64(0); i < c.Occurrences.Pages; i++ {
				add(LargeKindOccPage, c.ID, int(i), "")
			}
			add(LargeKindOccFooter, c.ID, 0, "")
			continue
		}
		// page count unknown: one registered placeholder per kind; never a zero denominator
		add(LargeKindCSTPage, c.ID, 0, c.Code)
		add(LargeKindCSTFooter, c.ID, 0, c.Code)
		add(LargeKindOccPage, c.ID, 0, c.Code)
		add(LargeKindOccFooter, c.ID, 0, c.Code)
	}
	add(LargeKindControl, e.Control.ID, 0, "")
	return out
}

// LargeJudgment is the state of one obligation.
type LargeJudgment struct {
	ID    string
	State string // VERIFIED, FAILED, BLOCKED
	Code  string
}

func largeVerdict(id string, ok bool, code string) LargeJudgment {
	if ok {
		return LargeJudgment{ID: id, State: "VERIFIED"}
	}
	return LargeJudgment{ID: id, State: "FAILED", Code: code}
}

// VerifyLargeSource judges that the observed source is the registered fixture (bytes and SHA-256
// of the input the native process received).
func VerifyLargeSource(c LargeExpectedFixture, source []byte) LargeJudgment {
	sum := sha256.Sum256(source)
	return largeVerdict(largeID(LargeKindSource, c.ID, 0), uint64(len(source)) == c.Bytes && hex.EncodeToString(sum[:]) == c.SHA256, "LARGE_SOURCE_IDENTITY")
}

// VerifyLargeStatus judges the product execution: the process completed and its syntax verdict is
// the registered one. A resource limit or any other status is not a pass.
func VerifyLargeStatus(c LargeExpectedFixture, status string, hasError bool) LargeJudgment {
	id := largeID(LargeKindStatus, c.ID, 0)
	switch {
	case status != StatusCompleted:
		return LargeJudgment{ID: id, State: "FAILED", Code: "LARGE_STATUS_" + status}
	case hasError != (c.Syntax == "ERROR"):
		return LargeJudgment{ID: id, State: "FAILED", Code: "LARGE_SYNTAX_MISMATCH"}
	}
	return LargeJudgment{ID: id, State: "VERIFIED"}
}

// LargeCSTFooter is the complete footer of a CST page session.
type LargeCSTFooter struct {
	Schema      string `json:"schema"`
	Nodes       uint64 `json:"nodes"`
	Pages       uint64 `json:"pages"`
	RowBytes    uint64 `json:"row_bytes"`
	MaxDepth    uint32 `json:"max_depth"`
	AllRows     string `json:"all_rows_sha256"`
	PagesSHA256 string `json:"pages_sha256"`
	HasError    bool   `json:"has_error"`
}

// LargeCSTPage is one native tsgk-large-cst-page/r1 payload.
type LargeCSTPage struct {
	Schema          string          `json:"schema"`
	Session         string          `json:"session"`
	Index           uint64          `json:"index"`
	ParseCount      uint64          `json:"parse_count"`
	Nodes           uint64          `json:"nodes"`
	Start           uint64          `json:"start"`
	End             uint64          `json:"end"`
	Rows            uint64          `json:"rows"`
	RowBytes        uint64          `json:"row_bytes"`
	Digest          string          `json:"digest"`
	SessionComplete bool            `json:"session_complete"`
	Footer          *LargeCSTFooter `json:"footer"`
	Error           string          `json:"error"`
	Complete        bool            `json:"complete"`
}

// DecodeLargeCSTPage strictly decodes one page payload (unknown and duplicate members rejected).
func DecodeLargeCSTPage(raw []byte) (LargeCSTPage, error) {
	var p LargeCSTPage
	if err := jsonv2.Unmarshal(raw, &p, jsonv2.RejectUnknownMembers(true)); err != nil {
		return p, fail(KindInvalidInput, "LARGE_PAGE_JSON", LargeExpectedPath, err)
	}
	return p, nil
}

// VerifyLargeCST judges the CST pages and the footer of one derivable fixture against the
// independent expected values. pages are the received payloads in order; every page, in order and
// without gap, duplicate or extra page, must carry the expected digest, and the footer of the last
// page must carry the complete counts and digests. A missing page, a reordered or repeated page,
// a forged digest or footer, and a truncated session that still claims completion are FAILED.
func VerifyLargeCST(c LargeExpectedFixture, session string, pages []LargeCSTPage) []LargeJudgment {
	exp := c.CST
	var out []LargeJudgment
	seq := ""
	for i, want := range exp.PageDigests {
		id := largeID(LargeKindCSTPage, c.ID, i)
		code := ""
		start := uint64(i) * exp.PageRows
		end := min(start+exp.PageRows, exp.Nodes)
		switch {
		case i >= len(pages):
			code = "LARGE_PAGE_MISSING"
		default:
			p := pages[i]
			switch {
			case p.Schema != "tsgk-large-cst-page/r1" || !p.Complete || p.Error != "" || p.Session != session || p.Session == "":
				code = "LARGE_PAGE_ENVELOPE"
			case p.Index != uint64(i):
				code = "LARGE_PAGE_ORDER"
			case p.ParseCount != 1:
				code = "LARGE_PAGE_PARSE_COUNT"
			case p.Start != start || p.End != end || p.Rows != end-start:
				code = "LARGE_PAGE_RANGE"
			case p.Digest != want:
				code = "LARGE_PAGE_DIGEST"
			case p.SessionComplete != (i == len(exp.PageDigests)-1) || (p.Footer != nil) != p.SessionComplete:
				code = "LARGE_PAGE_COMPLETION"
			}
		}
		out = append(out, largeVerdict(id, code == "", code))
		if i < len(pages) {
			seq += pages[i].Digest + "\n"
		}
	}
	fid := largeID(LargeKindCSTFooter, c.ID, 0)
	fcode := ""
	switch {
	case len(pages) > len(exp.PageDigests):
		fcode = "LARGE_PAGE_EXTRA"
	case len(pages) != len(exp.PageDigests) || pages[len(pages)-1].Footer == nil:
		fcode = "LARGE_FOOTER_MISSING"
	default:
		f := *pages[len(pages)-1].Footer
		sum := sha256.Sum256([]byte(seq))
		switch {
		case f.Schema != "tsgk-large-cst-footer/r1":
			fcode = "LARGE_FOOTER_ENVELOPE"
		case f.Nodes != exp.Nodes || f.Pages != uint64(len(exp.PageDigests)) || f.RowBytes != exp.RowBytes || f.MaxDepth != exp.MaxDepth || f.HasError != exp.HasError:
			fcode = "LARGE_FOOTER_COUNT"
		case f.AllRows != exp.AllRowsSHA256 || f.PagesSHA256 != exp.PagesSHA256 || f.PagesSHA256 != hex.EncodeToString(sum[:]):
			fcode = "LARGE_FOOTER_DIGEST"
		}
	}
	return append(out, largeVerdict(fid, fcode == "", fcode))
}

// VerifyLargeOccurrences judges the query occurrence pages and footer of one derivable fixture.
// captures is the complete native capture stream in runtime order; pages of LargeOccurrencePage
// captures must be full except the last, keep a non-decreasing start byte (the runtime's
// documented order) and use only the registered capture names; the footer (total, per-name counts
// and the multiset digest of the canonical records) is compared with the independent values.
func VerifyLargeOccurrences(c LargeExpectedFixture, captures []Capture) []LargeJudgment {
	exp := c.Occurrences
	var out []LargeJudgment
	lastStart := uint32(0)
	byName := map[string]uint64{}
	var records [][]byte
	pageCode := make([]string, exp.Pages)
	for i, cap := range captures {
		page := uint64(i) / exp.PageCaptures
		if page >= exp.Pages {
			break
		}
		if cap.StartByte < lastStart && pageCode[page] == "" {
			pageCode[page] = "LARGE_OCC_ORDER"
		}
		lastStart = cap.StartByte
		if _, ok := exp.ByName[cap.Name]; !ok && pageCode[page] == "" {
			pageCode[page] = "LARGE_OCC_NAME"
		}
	}
	for _, cap := range captures {
		byName[cap.Name]++
		records = append(records, LargeCapture{Name: cap.Name, Type: cap.Type, StartByte: cap.StartByte, EndByte: cap.EndByte,
			StartRow: cap.StartPoint.Row, StartCol: cap.StartPoint.Column, EndRow: cap.EndPoint.Row, EndCol: cap.EndPoint.Column}.Record())
	}
	for i := uint64(0); i < exp.Pages; i++ {
		first := i * exp.PageCaptures
		code := pageCode[i]
		want := min(exp.PageCaptures, exp.Total-first)
		if code == "" && (first >= uint64(len(captures)) || min(uint64(len(captures))-first, exp.PageCaptures) != want) {
			code = "LARGE_OCC_PAGE_COUNT"
		}
		out = append(out, largeVerdict(largeID(LargeKindOccPage, c.ID, int(i)), code == "", code))
	}
	fcode := ""
	switch {
	case uint64(len(captures)) > exp.Total:
		fcode = "LARGE_OCC_EXTRA"
	case uint64(len(captures)) != exp.Total:
		fcode = "LARGE_OCC_TOTAL"
	case !equalCounts(byName, exp.ByName):
		fcode = "LARGE_OCC_BY_NAME"
	case LargeMultisetDigest(records) != exp.SHA256:
		fcode = "LARGE_OCC_DIGEST"
	}
	return append(out, largeVerdict(largeID(LargeKindOccFooter, c.ID, 0), fcode == "", fcode))
}

func equalCounts(a, b map[string]uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// VerifyLargeControl judges the over-limit control. Only the exact registered status and code
// verify it; a completed or PASS outcome is FAILED, and the result never counts as a semantic
// PASS (the fold keeps it in its own kind).
func VerifyLargeControl(c LargeExpectedControl, status, code, assessment string, partial bool) LargeJudgment {
	id := largeID(LargeKindControl, c.ID, 0)
	switch {
	case status == StatusCompleted || assessment == AssessPass:
		return LargeJudgment{ID: id, State: "FAILED", Code: "LARGE_CONTROL_PASSED"}
	case status != c.Status:
		return LargeJudgment{ID: id, State: "FAILED", Code: "LARGE_CONTROL_STATUS"}
	case code != c.Code:
		return LargeJudgment{ID: id, State: "FAILED", Code: "LARGE_CONTROL_CODE"}
	}
	return LargeJudgment{ID: id, State: "VERIFIED"}
}

type LargeScopeCounter struct {
	Kind        string `json:"kind"`
	Platform    string `json:"platform"`
	Required    uint64 `json:"required"`
	Verified    uint64 `json:"verified"`
	Failed      uint64 `json:"failed"`
	Blocked     uint64 `json:"blocked"`
	Unprocessed uint64 `json:"unprocessed"`
}

// LargeScopeCoverage is scope-local. Completion stays BLOCKED while any obligation is not
// VERIFIED; the registered Linux/macOS N/A reasons are carried unchanged and are not counted.
type LargeScopeCoverage struct {
	Counters          []LargeScopeCounter `json:"counters"`
	NotApplicable     map[string]string   `json:"not_applicable"`
	Completion        string              `json:"completion"`
	FullScopeComplete bool                `json:"full_scope_complete"`
}

// FoldLargeScope folds the Windows judgments over the full enumerated denominator. An undeclared or
// duplicate judgment, an unknown state, a VERIFIED with a code, a FAILED/BLOCKED without one, and a
// VERIFIED for an obligation that is registered BLOCKED are errors. A missing judgment is BLOCKED
// for a registered-BLOCKED obligation and UNPROCESSED otherwise.
func FoldLargeScope(e LargeExpected, judgments []LargeJudgment) (LargeScopeCoverage, error) {
	out := LargeScopeCoverage{Completion: "COMPLETE", NotApplicable: e.NotApplicable}
	byID := map[string]LargeObligation{}
	for _, o := range e.Obligations() {
		if _, dup := byID[o.ID]; dup {
			return out, largeFault("LARGE_SCOPE_DUPLICATE_OBLIGATION")
		}
		byID[o.ID] = o
	}
	seen := map[string]LargeJudgment{}
	for _, j := range judgments {
		o, ok := byID[j.ID]
		if !ok {
			return out, largeFault("LARGE_SCOPE_UNDECLARED")
		}
		if _, dup := seen[j.ID]; dup {
			return out, largeFault("LARGE_SCOPE_OVERLAP")
		}
		if j.State == "VERIFIED" && o.Blocked != "" {
			return out, largeFault("LARGE_SCOPE_BLOCKED_VERIFIED")
		}
		seen[j.ID] = j
	}
	index := map[string]int{}
	for _, o := range e.Obligations() {
		if _, ok := index[o.Kind]; !ok {
			index[o.Kind] = len(out.Counters)
			out.Counters = append(out.Counters, LargeScopeCounter{Kind: o.Kind, Platform: largeHost})
		}
		c := &out.Counters[index[o.Kind]]
		c.Required++
		j, ok := seen[o.ID]
		state := "UNPROCESSED"
		if ok {
			state = j.State
		}
		switch state {
		case "VERIFIED":
			if j.Code != "" {
				return out, largeFault("LARGE_SCOPE_VERIFIED_CODE")
			}
			c.Verified++
		case "FAILED", "BLOCKED":
			if j.Code == "" {
				return out, largeFault("LARGE_SCOPE_CODE")
			}
			if state == "FAILED" {
				c.Failed++
			} else {
				c.Blocked++
			}
		case "UNPROCESSED":
			if o.Blocked != "" {
				c.Blocked++
			} else {
				c.Unprocessed++
			}
		default:
			return out, largeFault("LARGE_SCOPE_STATE")
		}
	}
	for _, c := range out.Counters {
		if c.Required != c.Verified+c.Failed+c.Blocked+c.Unprocessed {
			return out, largeFault("LARGE_SCOPE_SUM")
		}
		if c.Failed > 0 {
			out.Completion = "FAIL"
		} else if c.Verified != c.Required && out.Completion != "FAIL" {
			out.Completion = "BLOCKED"
		}
	}
	return out, nil
}

func (c LargeScopeCounter) String() string {
	return strings.Join([]string{c.Kind, c.Platform, fmt.Sprint(c.Required, "=", c.Verified, "+", c.Failed, "+", c.Blocked, "+", c.Unprocessed)}, " ")
}
