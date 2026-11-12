package kit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func largeAssets(t *testing.T) (asset, registry, inventory, fixtures []byte) {
	t.Helper()
	read := func(name string) []byte {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	return read("../contracts/query-large-expected-r1.json"), read("../contracts/query-semantic-registry-r1.json"),
		read("../contracts/qualification-c1.json"), read("../contracts/native-large-fixtures.json")
}

func largeCode(t *testing.T, err error) string {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("not a kit error: %v", err)
	}
	return e.Code
}

func loadLarge(t *testing.T) (LargeExpected, []LargeTemplate) {
	t.Helper()
	asset, registry, inventory, fixtures := largeAssets(t)
	e, err := ParseLargeExpected(asset, registry, inventory, fixtures)
	if err != nil {
		t.Fatal(err)
	}
	templates, _, err := LargeTemplates(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	return e, templates
}

// The sealed asset: seal 2 (after the c.14.3.1 correction taken from the query text; seal 1 was
// f3ffa566...8226 and differed only in the 22m occurrence digest).
func TestLargeExpectedSealedAndBound(t *testing.T) {
	asset, registry, inventory, fixtures := largeAssets(t)
	if len(asset) != 4966 || digestHex(asset) != "7b2e7374ffbd85d8c35ae58c525e82cfce9994c1cd7aa150d14d9f9a3647b929" {
		t.Fatalf("sealed expected asset changed: %d/%s", len(asset), digestHex(asset))
	}
	e, err := ParseLargeExpected(asset, registry, inventory, fixtures)
	if err != nil || len(e.Cases) != 3 || e.Control.SemanticPass {
		t.Fatalf("asset: %v", err)
	}
	count := map[string]int{}
	for _, o := range e.Obligations() {
		count[o.Kind]++
	}
	want := map[string]int{LargeKindSource: 3, LargeKindStatus: 3, LargeKindCSTPage: 14, LargeKindCSTFooter: 3, LargeKindOccPage: 12, LargeKindOccFooter: 3, LargeKindControl: 1}
	for k, v := range want {
		if count[k] != v {
			t.Errorf("%s = %d, want %d", k, count[k], v)
		}
	}
	// Linux and macOS stay N/A with the original reason; the large role runs on Windows only
	for _, host := range []string{"linux-amd64", "darwin-arm64"} {
		if e.NotApplicable[host] != R3PlatformReason {
			t.Errorf("%s N/A reason = %q", host, e.NotApplicable[host])
		}
	}
}

// The expected values are the derivation of the fixture templates: recomputing them (no native
// output is involved) reproduces the sealed asset, and the error-recovery fixtures are refused.
func TestLargeTemplateReproducesAsset(t *testing.T) {
	t.Parallel()
	e, templates := loadLarge(t)
	c := e.Cases[0]
	cst, err := templates[0].CSTDigests(LargePageRows)
	if err != nil {
		t.Fatal(err)
	}
	occ, err := templates[0].Occurrences()
	if err != nil {
		t.Fatal(err)
	}
	if !sameDerivation(c, cst, occ) {
		t.Fatalf("template derivation differs from the sealed asset: %d nodes, %d captures", cst.Nodes, occ.Count)
	}
	// forged values (node count, a page digest, the footer digests, the occurrence digest or count) no longer match the derivation
	for name, edit := range map[string]func(*LargeExpectedFixture){
		"nodes": func(f *LargeExpectedFixture) { f.CST.Nodes++ },
		"page digest": func(f *LargeExpectedFixture) {
			f.CST.PageDigests = slices.Clone(f.CST.PageDigests)
			f.CST.PageDigests[3] = strings.Repeat("0", 64)
		},
		"all rows":       func(f *LargeExpectedFixture) { f.CST.AllRowsSHA256 = strings.Repeat("1", 64) },
		"row bytes":      func(f *LargeExpectedFixture) { f.CST.RowBytes++ },
		"occurrence sha": func(f *LargeExpectedFixture) { f.Occurrences.SHA256 = strings.Repeat("2", 64) },
		"occurrence n":   func(f *LargeExpectedFixture) { f.Occurrences.Total++ },
	} {
		forged := c
		cst2, occ2 := *c.CST, *c.Occurrences
		forged.CST, forged.Occurrences = &cst2, &occ2
		edit(&forged)
		if sameDerivation(forged, cst, occ) {
			t.Errorf("forged %s still matches the derivation", name)
		}
	}
	for _, i := range []int{1, 2} {
		_, err := templates[i].CSTDigests(LargePageRows)
		if err == nil || largeCode(t, err) != LargeNotDerivable {
			t.Errorf("%s must be refused as not derivable: %v", templates[i].ID, err)
		}
		if e.Cases[i].Derivation != LargeNotDerivableState || e.Cases[i].CST != nil {
			t.Errorf("%s registered as derivable", templates[i].ID)
		}
	}
}

// tinyTemplate is a small valid template whose rows are checked by hand below.
func tinyTemplate(t *testing.T, repeat int) LargeTemplate {
	t.Helper()
	x := LargeTemplate{ID: "tiny", Header: "// h\nusing System;\n\n", Repeat: repeat, Footer: "// end\n",
		Block: "namespace Gen{N}\n{\n  public class C{N}\n  {\n    private int f{N};\n    public int M{N}(int a)\n    {\n      int x = a;\n      x = x + 0;\n      return x;\n    }\n  }\n}\n"}
	var b strings.Builder
	b.WriteString(x.Header)
	for n := 0; n < repeat; n++ {
		b.WriteString(strings.ReplaceAll(x.Block, "{N}", strconv.Itoa(n)))
	}
	b.WriteString(x.Footer)
	sum := sha256.Sum256([]byte(b.String()))
	x.Bytes, x.SHA256 = uint64(b.Len()), hex.EncodeToString(sum[:])
	return x
}

func TestLargeTemplateRowsByHand(t *testing.T) {
	x := tinyTemplate(t, 1)
	var rows []LargeRow
	n, err := x.LargeWalk(func(r LargeRow) { rows = append(rows, r) }, nil)
	if err != nil || n != uint64(len(rows)) {
		t.Fatal(err)
	}
	// hand-derived: root, comment, using_directive(using, identifier, ;), then the namespace subtree
	want := []struct {
		typ, field string
		parent     int32
		start, end uint32
	}{
		{"compilation_unit", "", -1, 0, uint32(x.Bytes)},
		{"comment", "", 0, 0, 4},
		{"using_directive", "", 0, 5, 18},
		{"using", "", 2, 5, 10},
		{"identifier", "", 2, 11, 17},
		{";", "", 2, 17, 18},
		{"namespace_declaration", "", 0, 20, 20 + uint32(len(strings.ReplaceAll(x.Block, "{N}", "0"))) - 1},
		{"namespace", "", 6, 20, 29},
		{"identifier", "name", 6, 30, 34},
		{"declaration_list", "body", 6, 35, 20 + uint32(len(strings.ReplaceAll(x.Block, "{N}", "0"))) - 1},
		{"{", "", 9, 35, 36},
		{"class_declaration", "", 9, 39, 0},
	}
	for i, w := range want {
		r := rows[i]
		if r.Type != w.typ || r.Field != w.field || r.Parent != w.parent || r.StartByte != w.start || (w.end != 0 && r.EndByte != w.end) || r.Index != uint32(i) {
			t.Fatalf("row %d = %+v, want %+v", i, r, w)
		}
	}
	// hand count of the block: namespace header 5, class header 7, field 8, method 35 (parameter list 6,
	// body 24 = braces 2 + local declaration 8 + assignment 9 + return 4 + ... see the grammar rules), two closing braces 2
	if len(rows) != 1+1+4+57+1 {
		t.Fatalf("rows = %d", len(rows))
	}
	last := rows[len(rows)-1]
	if last.Type != "comment" || last.Parent != 0 || last.Flags != LargeRowNamed|LargeRowExtra {
		t.Fatalf("footer row = %+v", last)
	}
}

func TestLargeEncodingIsStable(t *testing.T) {
	r := LargeRow{Index: 1, Parent: -1, StartByte: 2, EndByte: 3, StartRow: 4, StartCol: 5, EndRow: 6, EndCol: 7, Flags: 3, Type: "ab", Field: "c"}
	want := "0000000" + "1" + "ffffffff" + "00000002" + "00000003" + "00000004" + "00000005" + "00000006" + "00000007" + "03" + "0002" + "6162" + "0001" + "63"
	if got := hex.EncodeToString(r.Append(nil)); got != want {
		t.Fatalf("row encoding %s, want %s", got, want)
	}
}

// goodPages builds the session a correct native process would send for the derivable fixture.
func goodPages(c LargeExpectedFixture, session string) []LargeCSTPage {
	exp := c.CST
	var pages []LargeCSTPage
	for i, d := range exp.PageDigests {
		start := uint64(i) * exp.PageRows
		end := min(start+exp.PageRows, exp.Nodes)
		p := LargeCSTPage{Schema: "tsgk-large-cst-page/r1", Session: session, Index: uint64(i), ParseCount: 1, Nodes: exp.Nodes, Start: start, End: end, Rows: end - start, Digest: d, Complete: true}
		if i == len(exp.PageDigests)-1 {
			p.SessionComplete = true
			p.Footer = &LargeCSTFooter{Schema: "tsgk-large-cst-footer/r1", Nodes: exp.Nodes, Pages: uint64(len(exp.PageDigests)), RowBytes: exp.RowBytes, MaxDepth: exp.MaxDepth, AllRows: exp.AllRowsSHA256, PagesSHA256: exp.PagesSHA256, HasError: exp.HasError}
		}
		pages = append(pages, p)
	}
	return pages
}

func failedCodes(js []LargeJudgment) map[string]bool {
	out := map[string]bool{}
	for _, j := range js {
		if j.State == "FAILED" {
			out[j.Code] = true
		}
	}
	return out
}

func TestLargeCSTCounterexamples(t *testing.T) {
	e, _ := loadLarge(t)
	c := e.Cases[0]
	const session = "0123456789abcdef0123456789abcdef"
	for _, j := range VerifyLargeCST(c, session, goodPages(c, session)) {
		if j.State != "VERIFIED" {
			t.Fatalf("a correct session must verify: %+v", j)
		}
	}
	clone := func() []LargeCSTPage { return goodPages(c, session) }
	last := func(p []LargeCSTPage) *LargeCSTPage { return &p[len(p)-1] }
	for _, tc := range []struct {
		name string
		edit func([]LargeCSTPage) []LargeCSTPage
		want string
	}{
		{"missing page", func(p []LargeCSTPage) []LargeCSTPage { return slices.Delete(p, 3, 4) }, "LARGE_PAGE_ORDER"},
		{"missing last page", func(p []LargeCSTPage) []LargeCSTPage { return p[:len(p)-1] }, "LARGE_PAGE_MISSING"},
		{"reordered", func(p []LargeCSTPage) []LargeCSTPage { p[2], p[3] = p[3], p[2]; return p }, "LARGE_PAGE_ORDER"},
		{"duplicated", func(p []LargeCSTPage) []LargeCSTPage { p[4] = p[3]; return p }, "LARGE_PAGE_ORDER"},
		{"forged digest", func(p []LargeCSTPage) []LargeCSTPage { p[5].Digest = strings.Repeat("0", 64); return p }, "LARGE_PAGE_DIGEST"},
		{"forged range", func(p []LargeCSTPage) []LargeCSTPage { p[1].End--; return p }, "LARGE_PAGE_RANGE"},
		{"forged footer count", func(p []LargeCSTPage) []LargeCSTPage { last(p).Footer.Nodes--; return p }, "LARGE_FOOTER_COUNT"},
		{"forged footer pages", func(p []LargeCSTPage) []LargeCSTPage { last(p).Footer.Pages++; return p }, "LARGE_FOOTER_COUNT"},
		{"forged footer row bytes", func(p []LargeCSTPage) []LargeCSTPage { last(p).Footer.RowBytes++; return p }, "LARGE_FOOTER_COUNT"},
		{"forged footer has_error", func(p []LargeCSTPage) []LargeCSTPage { last(p).Footer.HasError = true; return p }, "LARGE_FOOTER_COUNT"},
		{"forged footer digest", func(p []LargeCSTPage) []LargeCSTPage { last(p).Footer.AllRows = strings.Repeat("1", 64); return p }, "LARGE_FOOTER_DIGEST"},
		{"forged footer page chain", func(p []LargeCSTPage) []LargeCSTPage { last(p).Footer.PagesSHA256 = strings.Repeat("2", 64); return p }, "LARGE_FOOTER_DIGEST"},
		{"truncated disguised as complete", func(p []LargeCSTPage) []LargeCSTPage {
			f := last(p).Footer
			p = p[:len(p)-3]
			last(p).SessionComplete, last(p).Footer = true, f
			return p
		}, "LARGE_FOOTER_MISSING"},
		{"footer missing", func(p []LargeCSTPage) []LargeCSTPage { last(p).Footer = nil; return p }, "LARGE_PAGE_COMPLETION"},
		{"early completion", func(p []LargeCSTPage) []LargeCSTPage { p[2].SessionComplete = true; return p }, "LARGE_PAGE_COMPLETION"},
		{"extra page", func(p []LargeCSTPage) []LargeCSTPage { return append(p, p[0]) }, "LARGE_PAGE_EXTRA"},
		{"other session", func(p []LargeCSTPage) []LargeCSTPage { p[6].Session = "ffffffffffffffffffffffffffffffff"; return p }, "LARGE_PAGE_ENVELOPE"},
		{"incomplete page", func(p []LargeCSTPage) []LargeCSTPage { p[7].Complete = false; return p }, "LARGE_PAGE_ENVELOPE"},
		{"error page", func(p []LargeCSTPage) []LargeCSTPage { p[7].Error = "NODE_COUNT_MISMATCH"; return p }, "LARGE_PAGE_ENVELOPE"},
		{"second parse", func(p []LargeCSTPage) []LargeCSTPage { p[8].ParseCount = 2; return p }, "LARGE_PAGE_PARSE_COUNT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := failedCodes(VerifyLargeCST(c, session, tc.edit(clone())))
			if !got[tc.want] {
				t.Fatalf("%s not rejected with %s: %v", tc.name, tc.want, got)
			}
		})
	}
	if got := failedCodes(VerifyLargeCST(c, session, nil)); !got["LARGE_PAGE_MISSING"] || !got["LARGE_FOOTER_MISSING"] {
		t.Fatalf("an empty session must fail: %v", got)
	}
}

// tinyOccurrenceFixture derives a fixture and a correct capture stream with more than one page.
func tinyOccurrenceFixture(t *testing.T) (LargeExpectedFixture, []Capture) {
	t.Helper()
	x := tinyTemplate(t, 700)
	occ, err := x.Occurrences()
	if err != nil {
		t.Fatal(err)
	}
	var caps []Capture
	_, err = x.LargeWalk(func(LargeRow) {}, func(n *ltNode, row func(*ltNode) LargeRow) {
		for p, names := range largeDeclarations[n.typ] {
			for i, node := range largeCaptureNodes(n, p) {
				r := row(node)
				caps = append(caps, Capture{Name: names[i], Type: node.typ, StartByte: r.StartByte, EndByte: r.EndByte,
					StartPoint: Point{Row: r.StartRow, Column: r.StartCol}, EndPoint: Point{Row: r.EndRow, Column: r.EndCol}})
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.SortStableFunc(caps, func(a, b Capture) int { return int(a.StartByte) - int(b.StartByte) })
	c := LargeExpectedFixture{ID: "tiny", Occurrences: &LargeExpectedOccurrences{PageCaptures: LargeOccurrencePage, Total: occ.Count, Pages: 1 + (occ.Count-1)/LargeOccurrencePage, ByName: occ.ByName, SHA256: occ.Digest}}
	return c, caps
}

func TestLargeOccurrenceCounterexamples(t *testing.T) {
	c, caps := tinyOccurrenceFixture(t)
	if c.Occurrences.Pages != 2 || c.Occurrences.Total != 700*15 {
		t.Fatalf("fixture shape: %+v", c.Occurrences)
	}
	for _, j := range VerifyLargeOccurrences(c, caps) {
		if j.State != "VERIFIED" {
			t.Fatalf("a correct stream must verify: %+v", j)
		}
	}
	dup := func(cs []Capture) []Capture { return slices.Clone(cs) }
	for _, tc := range []struct {
		name string
		edit func([]Capture) []Capture
		want string
	}{
		{"missing capture", func(cs []Capture) []Capture { return slices.Delete(cs, 5, 6) }, "LARGE_OCC_TOTAL"},
		{"truncated stream", func(cs []Capture) []Capture { return cs[:len(cs)-10] }, "LARGE_OCC_TOTAL"},
		{"truncated first page", func(cs []Capture) []Capture { return cs[:5] }, "LARGE_OCC_TOTAL"},
		{"duplicated capture", func(cs []Capture) []Capture { cs[7] = cs[6]; return cs }, "LARGE_OCC_BY_NAME"},
		{"extra capture", func(cs []Capture) []Capture { return append(cs, cs[0]) }, "LARGE_OCC_EXTRA"},
		{"order broken", func(cs []Capture) []Capture { cs[20], cs[200] = cs[200], cs[20]; return cs }, "LARGE_OCC_ORDER"},
		{"forged type", func(cs []Capture) []Capture { cs[9].Type = "identifier"; return cs }, "LARGE_OCC_DIGEST"},
		{"forged range", func(cs []Capture) []Capture { cs[11].EndByte++; return cs }, "LARGE_OCC_DIGEST"},
		{"forged point", func(cs []Capture) []Capture { cs[12].StartPoint.Column++; return cs }, "LARGE_OCC_DIGEST"},
		{"unknown name", func(cs []Capture) []Capture { cs[13].Name = "c.99.0.0"; return cs }, "LARGE_OCC_NAME"},
		{"empty stream", func(cs []Capture) []Capture { return nil }, "LARGE_OCC_TOTAL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := failedCodes(VerifyLargeOccurrences(c, tc.edit(dup(caps))))
			if !got[tc.want] {
				t.Fatalf("%s not rejected with %s: %v", tc.name, tc.want, got)
			}
		})
	}
}

func TestLargeControlNeverBecomesPass(t *testing.T) {
	e, _ := loadLarge(t)
	c := e.Control
	if j := VerifyLargeControl(c, StatusResourceLimit, "OUTPUT_LIMIT", AssessBlocked, true); j.State != "VERIFIED" {
		t.Fatalf("the exact rejection verifies: %+v", j)
	}
	for _, tc := range []struct{ name, status, code, assess, want string }{
		{"completed", StatusCompleted, "", AssessPass, "LARGE_CONTROL_PASSED"},
		{"completed without assessment", StatusCompleted, "OUTPUT_LIMIT", "", "LARGE_CONTROL_PASSED"},
		{"limit turned into pass", StatusResourceLimit, "OUTPUT_LIMIT", AssessPass, "LARGE_CONTROL_PASSED"},
		{"other status", StatusFailed, "OUTPUT_LIMIT", AssessBlocked, "LARGE_CONTROL_STATUS"},
		{"other code", StatusResourceLimit, "MATCH_LIMIT", AssessBlocked, "LARGE_CONTROL_CODE"},
		{"no code", StatusResourceLimit, "", AssessBlocked, "LARGE_CONTROL_CODE"},
	} {
		if j := VerifyLargeControl(c, tc.status, tc.code, tc.assess, false); j.State != "FAILED" || j.Code != tc.want {
			t.Errorf("%s = %+v, want %s", tc.name, j, tc.want)
		}
	}
	// a rejection is never counted as a semantic pass: the fold keeps it in its own kind
	cov, err := FoldLargeScope(e, []LargeJudgment{VerifyLargeControl(c, StatusResourceLimit, "OUTPUT_LIMIT", AssessBlocked, true)})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range cov.Counters {
		if k.Kind != LargeKindControl && k.Verified != 0 {
			t.Errorf("%s verified %d", k.Kind, k.Verified)
		}
	}
}

func TestLargeStatusAndSource(t *testing.T) {
	e, templates := loadLarge(t)
	c := e.Cases[1]
	if j := VerifyLargeStatus(c, StatusCompleted, true); j.State != "VERIFIED" {
		t.Fatalf("%+v", j)
	}
	for _, tc := range []struct {
		status string
		err    bool
		want   string
	}{{StatusResourceLimit, true, "LARGE_STATUS_RESOURCE_LIMIT"}, {StatusCompleted, false, "LARGE_SYNTAX_MISMATCH"}, {StatusFailed, true, "LARGE_STATUS_FAILED"}} {
		if j := VerifyLargeStatus(c, tc.status, tc.err); j.State != "FAILED" || j.Code != tc.want {
			t.Errorf("%+v", j)
		}
	}
	src, err := templates[1].Source()
	if err != nil || VerifyLargeSource(c, src).State != "VERIFIED" {
		t.Fatal(err)
	}
	if VerifyLargeSource(c, src[:len(src)-1]).State != "FAILED" || VerifyLargeSource(c, bytes.ToUpper(src)).State != "FAILED" {
		t.Fatal("a changed source must fail")
	}
}

func TestLargeFold(t *testing.T) {
	e, _ := loadLarge(t)
	cov, err := FoldLargeScope(e, nil)
	if err != nil {
		t.Fatal(err)
	}
	var required, blocked, unprocessed, verified uint64
	for _, k := range cov.Counters {
		required += k.Required
		blocked += k.Blocked
		unprocessed += k.Unprocessed
		verified += k.Verified
	}
	// nothing judged: the registered-BLOCKED obligations (error-recovery fixtures) are BLOCKED, the rest UNPROCESSED
	if required != 39 || blocked != 8 || unprocessed != 31 || verified != 0 || cov.Completion != "BLOCKED" || cov.FullScopeComplete {
		t.Fatalf("empty fold: required %d blocked %d unprocessed %d verified %d %+v", required, blocked, unprocessed, verified, cov)
	}
	id := largeID(LargeKindSource, e.Cases[0].ID, 0)
	for _, tc := range []struct {
		name string
		js   []LargeJudgment
		want string
	}{
		{"undeclared", []LargeJudgment{{ID: "LARGE_SOURCE/csharp/none/0", State: "VERIFIED"}}, "LARGE_SCOPE_UNDECLARED"},
		{"overlap", []LargeJudgment{{ID: id, State: "VERIFIED"}, {ID: id, State: "VERIFIED"}}, "LARGE_SCOPE_OVERLAP"},
		{"verified with code", []LargeJudgment{{ID: id, State: "VERIFIED", Code: "X"}}, "LARGE_SCOPE_VERIFIED_CODE"},
		{"failed without code", []LargeJudgment{{ID: id, State: "FAILED"}}, "LARGE_SCOPE_CODE"},
		{"unknown state", []LargeJudgment{{ID: id, State: "PASS"}}, "LARGE_SCOPE_STATE"},
		{"blocked obligation verified", []LargeJudgment{{ID: largeID(LargeKindCSTPage, e.Cases[1].ID, 0), State: "VERIFIED"}}, "LARGE_SCOPE_BLOCKED_VERIFIED"},
	} {
		if _, err := FoldLargeScope(e, tc.js); err == nil || largeCode(t, err) != tc.want {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.want)
		}
	}
	// FAIL wins over BLOCKED and keeps the first code
	cov, err = FoldLargeScope(e, []LargeJudgment{{ID: id, State: "FAILED", Code: "LARGE_SOURCE_IDENTITY"}})
	if err != nil || cov.Completion != "FAIL" {
		t.Fatalf("%v %+v", err, cov)
	}
	// deleting a registered obligation from a verified set shows up as UNPROCESSED, not as a smaller denominator
	var all []LargeJudgment
	for _, o := range e.Obligations() {
		if o.Blocked == "" {
			all = append(all, LargeJudgment{ID: o.ID, State: "VERIFIED"})
		}
	}
	full, err := FoldLargeScope(e, all)
	if err != nil || full.Completion != "BLOCKED" || full.FullScopeComplete {
		t.Fatalf("all derivable verified is still BLOCKED by the registered not-derivable obligations: %v %+v", err, full)
	}
	short, err := FoldLargeScope(e, all[1:])
	if err != nil {
		t.Fatal(err)
	}
	var u uint64
	for _, k := range short.Counters {
		u += k.Unprocessed
	}
	if u != 1 {
		t.Fatalf("a deleted judgment must be UNPROCESSED: %d", u)
	}
}

func TestLargeExpectedForgeries(t *testing.T) {
	t.Parallel()
	asset, registry, inventory, fixtures := largeAssets(t)
	replace := func(old, new string) []byte {
		if !bytes.Contains(asset, []byte(old)) {
			t.Fatalf("anchor %q absent", old)
		}
		return bytes.Replace(asset, []byte(old), []byte(new), 1)
	}
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"semantic pass", replace(`"semantic_pass": false`, `"semantic_pass": true`), "LARGE_EXPECTED_CONTROL"},
		{"control code removed", replace(`"code": "OUTPUT_LIMIT"`, `"code": ""`), "LARGE_EXPECTED_CONTROL"},
		{"control status", replace(`"status": "RESOURCE_LIMIT"`, `"status": "COMPLETED"`), "LARGE_EXPECTED_CONTROL"},
		{"derivable made blocked", replace(`"derivation": "TEMPLATE_RULES"`, `"derivation": "NOT_DERIVABLE"`), "LARGE_EXPECTED_DERIVATION"},
		{"blocked made derivable", replace(`"cst": null`, `"cst": {}`), "LARGE_EXPECTED_DERIVATION"},
		{"page count", replace(`"pages": 10`, `"pages": 9`), "LARGE_EXPECTED_DERIVED"},
		{"syntax", replace(`"syntax": "NO_ERROR"`, `"syntax": "ERROR"`), "LARGE_EXPECTED_MEMBERSHIP"},
		{"case size", replace(`"bytes": 21997719`, `"bytes": 21997718`), "LARGE_EXPECTED_MEMBERSHIP"},
		{"registry pin", replace(`"bytes": 82475`, `"bytes": 82476`), "LARGE_EXPECTED_PIN"},
		{"n/a reason", replace(`"linux-amd64": "NET461 workload is Windows-hosted (WinForms/.NET Framework 4.6.1)"`, `"linux-amd64": "other"`), "LARGE_EXPECTED_NOT_APPLICABLE"},
		{"n/a removed", replace(`"darwin-arm64": "NET461 workload is Windows-hosted (WinForms/.NET Framework 4.6.1)",`, ``), "LARGE_EXPECTED_PLATFORM"},
		{"unknown member", replace(`"basis":`, `"extra": 1, "basis":`), "LARGE_EXPECTED_JSON"},
		{"schema", replace(`tsgk-query-large-expected/r1`, `tsgk-query-large-expected/r2`), "LARGE_EXPECTED_SCHEMA"},
	} {
		if _, err := ParseLargeExpected(tc.data, registry, inventory, fixtures); err == nil || largeCode(t, err) != tc.want {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.want)
		}
	}
	// the fixture inventory pin: any other fixture bytes are refused
	if _, err := ParseLargeExpected(asset, registry, inventory, append(slices.Clone(fixtures), ' ')); err == nil || largeCode(t, err) != "LARGE_EXPECTED_PIN" {
		t.Errorf("fixture pin: %v", err)
	}
}

func sameDerivation(c LargeExpectedFixture, cst LargeCSTDigests, occ LargeOccurrences) bool {
	if cst.Nodes != c.CST.Nodes || !slices.Equal(cst.Pages, c.CST.PageDigests) || cst.PagesChain != c.CST.PagesSHA256 || cst.AllRows != c.CST.AllRowsSHA256 || cst.RowBytes != c.CST.RowBytes || cst.MaxDepth != c.CST.MaxDepth {
		return false
	}
	if occ.Count != c.Occurrences.Total || occ.Digest != c.Occurrences.SHA256 || len(occ.ByName) != len(c.Occurrences.ByName) {
		return false
	}
	for k, v := range occ.ByName {
		if c.Occurrences.ByName[k] != v {
			return false
		}
	}
	return true
}

// Occurrence and CST counts are bounded by the native-query-large operation (captures and nodes) at
// parse time, so Obligations and the page judgments never expand an unbounded count; a per-name
// count above the total (whose sum could wrap) is refused too.
func TestLargeExpectedBoundsCountsBeforeExpansion(t *testing.T) {
	asset, registry, inventory, fixtures := largeAssets(t)
	op := NativeOperations()["native-query-large"]
	edit := func(f func(c *LargeExpectedFixture)) []byte {
		var e LargeExpected
		if err := json.Unmarshal(asset, &e); err != nil {
			t.Fatal(err)
		}
		f(&e.Cases[0])
		return mustJSON(t, e)
	}
	occ := func(total uint64, byName map[string]uint64) func(c *LargeExpectedFixture) {
		return func(c *LargeExpectedFixture) {
			c.Occurrences.Total, c.Occurrences.Pages, c.Occurrences.ByName = total, 1+(total-1)/LargeOccurrencePage, byName
		}
	}
	for name, data := range map[string][]byte{
		"huge occurrence total":       edit(occ(^uint64(0), map[string]uint64{"c.0.0.0": ^uint64(0)})),
		"occurrence total over limit": edit(occ(op.Captures+1, map[string]uint64{"c.0.0.0": op.Captures + 1})),
		"by-name sum that wraps":      edit(occ(1, map[string]uint64{"c.0.0.0": ^uint64(0), "c.0.1.0": 2})),
		"cst nodes over the node limit": edit(func(c *LargeExpectedFixture) {
			c.CST.Nodes = op.Nodes + 1
			for uint64(len(c.CST.PageDigests)) < 1+(c.CST.Nodes-1)/c.CST.PageRows {
				c.CST.PageDigests = append(c.CST.PageDigests, c.CST.PageDigests[0])
			}
		}),
	} {
		_, err := ParseLargeExpected(data, registry, inventory, fixtures)
		if err == nil || largeCode(t, err) != "LARGE_EXPECTED_DERIVED" {
			t.Errorf("%s: %v, want LARGE_EXPECTED_DERIVED", name, err)
		}
	}
	if _, err := ParseLargeExpected(edit(occ(op.Captures, map[string]uint64{"c.0.0.0": op.Captures})), registry, inventory, fixtures); err != nil {
		t.Errorf("occurrence total at the limit: %v", err)
	}
}
