package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/largeaudit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// largeMeasure is the actual cost of one native run.
type largeMeasure struct {
	Case         string `json:"case"`
	Run          string `json:"run"`
	Status       string `json:"status"`
	Code         string `json:"code"`
	WallMillis   int64  `json:"wall_ms"`
	PeakBytes    uint64 `json:"peak_bytes"`
	LimitBytes   uint64 `json:"limit_bytes"`
	Enforcement  string `json:"enforcement"`
	StdoutBytes  int64  `json:"stdout_bytes"`
	InputBytes   int    `json:"input_bytes"`
	Nodes        uint64 `json:"nodes"`
	Pages        int    `json:"pages"`
	MaxPageWall  int64  `json:"max_page_wall_ms"`
	Captures     int    `json:"captures"`
	CleanupOK    bool   `json:"cleanup_verified"`
	ResidualProc int    `json:"residual_after_exit"`
}

func processMeasure(m *largeMeasure, p runner.Result) {
	m.WallMillis, m.PeakBytes, m.LimitBytes, m.Enforcement = p.WallMillis, p.Memory.Peak, p.Memory.Limit, p.Memory.Enforcement
	m.StdoutBytes, m.CleanupOK, m.ResidualProc = p.StdoutBytes, p.Cleanup.Verified, p.Cleanup.ResidualAfterExit
}

// largeRecord is what one subtest leaves behind: its measures and judgments, folded later.
type largeRecord struct {
	Measures  []largeMeasure      `json:"measures"`
	Judgments []kit.LargeJudgment `json:"judgments"`
}

func writeLargeRecord(t *testing.T, name string, r largeRecord) {
	t.Helper()
	dir := os.Getenv("TSGK_LARGE_MEASURE_DIR")
	if dir == "" {
		return
	}
	data, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// largeSession runs the CST page session wrapper: one process, one parse, consecutive pages.
func largeSession(ctx context.Context, exe, exeSHA, dir string, req native.Request, nodes uint64) ([]kit.LargeCSTPage, string, runner.BatchResult, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, "", runner.BatchResult{}, err
	}
	session := hex.EncodeToString(nonce[:])
	frames, err := sessionFrames(req, nodes, kit.LargePageRows)
	if err != nil {
		return nil, session, runner.BatchResult{}, err
	}
	data, err := os.ReadFile(exe)
	if err != nil || digest(data) != exeSHA {
		return nil, session, runner.BatchResult{}, os.ErrInvalid
	}
	op := kit.NativeOperations()["native-query-large"]
	wall := time.Duration(len(frames)) * op.ParseWall
	policy := runner.BatchPolicy{RequestBytes: runner.DefaultBatchPolicy().RequestBytes, ResponseBytes: largeaudit.SessionPageLimit, FrameWall: op.ParseWall - 5*time.Second, FrameGrace: 5 * time.Second, StdoutBytes: int64(op.OutputBytes), BatchWall: wall}
	spec := runner.Spec{Path: exe, Dir: dir, Args: []string{session, strconv.FormatUint(kit.LargePageRows, 10), strconv.FormatUint(nodes, 10)}, StderrBytes: 65536, Grace: 5 * time.Second, Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: true}}
	batch, err := runner.RunBatch(ctx, spec, policy, frames)
	var pages []kit.LargeCSTPage
	for _, f := range batch.Frames {
		p, derr := kit.DecodeLargeCSTPage(f.Response)
		if derr != nil {
			return pages, session, batch, derr
		}
		pages = append(pages, p)
	}
	return pages, session, batch, err
}

// TestLargeScopeNative runs the registered large workloads on the registered C# grammar (a
// directory from TSGK_LARGE_CSHARP_GRAMMAR whose files match the qualification closure byte for
// byte). One subtest per case keeps one native process and its Go-side data at a time, so each
// case can run in its own memory-limited job: -run 'TestLargeScopeNative/^cs-large-22m$'.
func TestLargeScopeNative(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("the large workloads run on windows/amd64 only")
	}
	// The prepared C# grammar is not tracked, so this run is opt-in even under TSGK_NATIVE_REQUIRED=1;
	// a set but mismatching grammar still fails against the registered closure below.
	grammarRoot := os.Getenv("TSGK_LARGE_CSHARP_GRAMMAR")
	if grammarRoot == "" {
		t.Skip("set TSGK_LARGE_CSHARP_GRAMMAR to the prepared registered C# grammar directory to run the large-scope native regression (the grammar is not tracked)")
	}
	rt, cc := os.Getenv("TSGK_NATIVE_RUNTIME"), os.Getenv("TSGK_NATIVE_CC")
	if rt == "" || cc == "" {
		if os.Getenv("TSGK_NATIVE_REQUIRED") == "1" {
			t.Fatal("TSGK_LARGE_CSHARP_GRAMMAR is set but the required native runtime or compiler is absent")
		}
		t.Skip("native runtime or compiler not configured")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	read := func(rel string) []byte {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	registry, inventory, fixtures, asset := read(queryregistrycore.Path), read(queryregistrycore.QualificationPin.Path), read(kit.LargeFixturesPath), read(kit.LargeExpectedPath)
	e, err := kit.ParseLargeExpected(asset, registry, inventory, fixtures)
	if err != nil {
		t.Fatal(err)
	}
	templates, _, err := kit.LargeTemplates(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := kit.ParseQualificationInventory(inventory)
	if err != nil {
		t.Fatal(err)
	}
	var role kit.QualRole
	for _, r := range inv.ExtraRoles {
		if r.ID == "n461-large" {
			role = r
		}
	}
	grammar := role.Workloads[0].Grammar
	for _, f := range grammar { // the registered closure, byte for byte
		data, err := os.ReadFile(filepath.Join(grammarRoot, filepath.FromSlash(f.Path)))
		if err != nil || uint64(len(data)) != f.Bytes || digest(data) != f.SHA256 {
			t.Fatalf("registered C# grammar file %s differs: %v", f.Path, err)
		}
	}
	declSource := factQuerySource(t, read("src/contracts/fact-query-pack.json"), "csharp", "facts.declarations")
	if digest(declSource) != role.Workloads[0].Queries[0].SHA256 {
		t.Fatal("facts.declarations source differs from the registered query hash")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	compiler, err := os.ReadFile(cc)
	if err != nil {
		t.Fatal(err)
	}
	b, err := native.NewBuild(ctx, native.BuildRequest{Work: t.TempDir(), Runtime: rt, GrammarRoot: grammarRoot, Grammar: grammar, Symbol: "tree_sitter_c_sharp", Compiler: cc,
		CompilerID: kit.ToolIdentity{Name: "cc", Version: "host", SHA256: digest(compiler), Bytes: uint64(len(compiler))}})
	if err != nil {
		t.Fatal(err)
	}
	wrapper, err := os.ReadFile("session-cst.c")
	if err != nil {
		t.Fatal(err)
	}
	if err = writeExclusive(filepath.Join(b.Dir, "session-cst.c"), wrapper); err != nil {
		t.Fatal(err)
	}
	exe, _, err := compile(ctx, b, cc, filepath.Join(b.Dir, "session-cst.c"), "")
	if err != nil {
		t.Fatal(err)
	}
	exeBytes, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	exeSHA := digest(exeBytes)

	op := kit.NativeOperations()["native-query-large"]
	runCase := func(id string, source []byte, points []kit.NativePoint, queries ...native.QuerySource) native.CaseResult {
		x := native.Context{Op: op, Route: "csharp", Output: kit.OutputAuto, Protocol: native.ProtocolR2, Queries: queries}
		return b.RunCase(ctx, x, kit.IncrementalCase{ID: id, Encoding: kit.EncodingUTF8, Points: points}, source)
	}
	for i, c := range e.Cases {
		t.Run(c.ID, func(t *testing.T) {
			var rec largeRecord
			defer func() { writeLargeRecord(t, c.ID, rec) }()
			source, err := templates[i].Source()
			if err != nil {
				t.Fatal(err)
			}
			rec.Judgments = append(rec.Judgments, kit.VerifyLargeSource(c, source))
			points := role.Workloads[0].Cases[i].Points
			res := runCase(c.ID, source, points, native.QuerySource{ID: "facts.declarations", Source: declSource})
			m := largeMeasure{Case: c.ID, Run: "product-query", Status: res.ExecutionStatus, Code: res.Code, InputBytes: len(source)}
			if res.Process != nil {
				processMeasure(&m, *res.Process)
			}
			var tree *native.TreeOut
			if len(res.Steps) == 1 {
				tree = res.Steps[0].Incremental
			}
			if tree == nil {
				// no tree: the status obligation fails with the process outcome; nothing else can be judged
				t.Errorf("%s: no tree: %s %s", c.ID, res.ExecutionStatus, res.Code)
				rec.Measures = append(rec.Measures, m)
				rec.Judgments = append(rec.Judgments, kit.LargeJudgment{ID: "LARGE_STATUS/csharp/" + c.ID + "/0", State: "FAILED", Code: "LARGE_STATUS_" + res.ExecutionStatus})
				return
			}
			m.Nodes = tree.DescendantCount
			rec.Judgments = append(rec.Judgments, kit.VerifyLargeStatus(c, res.ExecutionStatus, tree.HasError))
			if c.CST != nil {
				if tree.DescendantCount != c.CST.Nodes {
					t.Errorf("%s: product node count %d, independent %d", c.ID, tree.DescendantCount, c.CST.Nodes)
				}
				if len(tree.Queries) != 1 || tree.Queries[0].Status != kit.StatusCompleted {
					t.Errorf("%s: declarations query did not complete: %d queries", c.ID, len(tree.Queries))
				} else {
					m.Captures = len(tree.Queries[0].Captures)
					rec.Judgments = append(rec.Judgments, kit.VerifyLargeOccurrences(c, tree.Queries[0].Captures)...)
				}
			}
			rec.Measures = append(rec.Measures, m)
			if c.CST == nil {
				return
			}
			req := native.Request{ID: c.ID, Encoding: kit.EncodingUTF8, Output: kit.OutputAuto, Limits: native.LimitsFor(op), Source: source, Points: points}
			pages, session, batch, err := largeSession(ctx, exe, exeSHA, b.Dir, req, c.CST.Nodes)
			sm := largeMeasure{Case: c.ID, Run: "cst-session", Status: batch.Process.Status, Pages: len(pages), Nodes: c.CST.Nodes, InputBytes: len(source)}
			processMeasure(&sm, batch.Process)
			for _, f := range batch.Frames {
				sm.MaxPageWall = max(sm.MaxPageWall, f.WallMillis)
			}
			rec.Measures = append(rec.Measures, sm)
			if err != nil {
				t.Errorf("%s: CST session: %v (frames %d)", c.ID, err, len(batch.Frames))
			}
			rec.Judgments = append(rec.Judgments, kit.VerifyLargeCST(c, session, pages)...)
			// forgeries of the real native pages and captures must not verify
			caps := tree.Queries[0].Captures
			lastPage := len(pages) - 1
			forgedFooter := func(edit func(*kit.LargeCSTFooter)) []kit.LargeCSTPage {
				cp := slices.Clone(pages)
				f := *cp[lastPage].Footer
				edit(&f)
				cp[lastPage].Footer = &f
				return cp
			}
			for name, forged := range map[string][]kit.LargeCSTPage{
				"missing page":  slices.Delete(slices.Clone(pages), 4, 5),
				"missing last":  pages[:lastPage],
				"swapped":       func() []kit.LargeCSTPage { cp := slices.Clone(pages); cp[1], cp[2] = cp[2], cp[1]; return cp }(),
				"duplicated":    func() []kit.LargeCSTPage { cp := slices.Clone(pages); cp[3] = cp[2]; return cp }(),
				"footer count":  forgedFooter(func(f *kit.LargeCSTFooter) { f.Nodes-- }),
				"footer digest": forgedFooter(func(f *kit.LargeCSTFooter) { f.AllRows = strings.Repeat("0", 64) }),
				"truncated as done": func() []kit.LargeCSTPage {
					cp := slices.Clone(pages[:lastPage-2])
					cp[len(cp)-1].SessionComplete = true
					cp[len(cp)-1].Footer = pages[lastPage].Footer
					return cp
				}(),
			} {
				if allVerified(kit.VerifyLargeCST(c, session, forged)) {
					t.Errorf("forged CST session %q verified", name)
				}
			}
			for name, forged := range map[string][]kit.Capture{
				"dropped":   slices.Delete(slices.Clone(caps), 100, 101),
				"duplicate": append(slices.Clone(caps), caps[0]),
				"truncated": caps[:len(caps)-1],
				"swapped":   func() []kit.Capture { cp := slices.Clone(caps); cp[10], cp[5000] = cp[5000], cp[10]; return cp }(),
			} {
				if allVerified(kit.VerifyLargeOccurrences(c, forged)) {
					t.Errorf("forged occurrence stream %q verified", name)
				}
			}
			for _, j := range rec.Judgments {
				if j.State == "FAILED" {
					t.Errorf("FAILED %s: %s", j.ID, j.Code)
				}
			}
		})
	}
	// the over-limit control: an exact RESOURCE_LIMIT and code, never a semantic PASS
	t.Run("control", func(t *testing.T) {
		var rec largeRecord
		defer func() { writeLargeRecord(t, "control", rec) }()
		source, err := templates[1].Source()
		if err != nil {
			t.Fatal(err)
		}
		over := runCase(e.Control.ID, source, nil, native.QuerySource{ID: e.Control.Query, Source: []byte("_ @node\n")})
		cm := largeMeasure{Case: e.Control.ID, Run: "control-query", Status: over.ExecutionStatus, Code: over.Code, InputBytes: len(source)}
		if over.Process != nil {
			processMeasure(&cm, *over.Process)
		}
		rec.Measures = append(rec.Measures, cm)
		j := kit.VerifyLargeControl(e.Control, over.ExecutionStatus, over.Code, over.Assessment, false)
		rec.Judgments = append(rec.Judgments, j)
		if j.State != "VERIFIED" {
			t.Errorf("control %s: %s (status %s code %s assessment %s)", j.ID, j.Code, over.ExecutionStatus, over.Code, over.Assessment)
		}
	})
}

// TestLargeScopeFold folds the judgments the per-case runs left in TSGK_LARGE_FOLD (a ';'
// separated list of record files) over the full denominator.
func TestLargeScopeFold(t *testing.T) {
	list := os.Getenv("TSGK_LARGE_FOLD")
	if list == "" {
		t.Skip("no recorded judgments to fold")
	}
	root, _ := filepath.Abs("../../..")
	read := func(rel string) []byte {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	e, err := kit.ParseLargeExpected(read(kit.LargeExpectedPath), read(queryregistrycore.Path), read(queryregistrycore.QualificationPin.Path), read(kit.LargeFixturesPath))
	if err != nil {
		t.Fatal(err)
	}
	var judgments []kit.LargeJudgment
	for _, name := range strings.Split(list, ";") {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var rec largeRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			t.Fatal(err)
		}
		judgments = append(judgments, rec.Judgments...)
	}
	// a case whose job ended on the task's HARD memory cap has no record: the supervised receipt is
	// its evidence, the source identity is checked offline and the status is BLOCKED, never PASS
	if spec := os.Getenv("TSGK_LARGE_CAP_RECEIPT"); spec != "" {
		id, path, _ := strings.Cut(spec, "=")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var receipt struct {
			Execution struct {
				Status string `json:"status"`
				Reason string `json:"reason"`
				Memory struct {
					Limit       uint64 `json:"limit_bytes"`
					Enforcement string `json:"enforcement"`
					Peak        uint64 `json:"peak_bytes"`
					Hit         bool   `json:"limit_hit"`
				} `json:"memory"`
			} `json:"execution"`
		}
		if err := json.Unmarshal(data, &receipt); err != nil {
			t.Fatal(err)
		}
		x := receipt.Execution
		if x.Status != kit.StatusResourceLimit || x.Reason != "MEMORY_LIMIT" || x.Memory.Limit != 4<<30 || x.Memory.Enforcement != "HARD" || !x.Memory.Hit {
			t.Fatalf("receipt is not a HARD 4 GiB memory limit: %+v", x)
		}
		templates, _, err := kit.LargeTemplates(read(kit.LargeFixturesPath))
		if err != nil {
			t.Fatal(err)
		}
		for i, c := range e.Cases {
			if c.ID == id {
				source, err := templates[i].Source()
				if err != nil {
					t.Fatal(err)
				}
				judgments = append(judgments, kit.VerifyLargeSource(c, source), kit.LargeJudgment{ID: "LARGE_STATUS/csharp/" + c.ID + "/0", State: "BLOCKED", Code: "LARGE_MEMORY_CAP_4GIB_EXCEEDED"})
				t.Logf("%s: job stopped at the 4 GiB cap (peak %d bytes); registered large budget is %d bytes", id, x.Memory.Peak, kit.RealWorldR3MemoryBytes)
			}
		}
	}
	coverage, err := kit.FoldLargeScope(e, judgments)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range coverage.Counters {
		t.Log(c.String())
	}
	t.Logf("completion %s full_scope_complete %t not_applicable %v", coverage.Completion, coverage.FullScopeComplete, coverage.NotApplicable)
}

// factQuerySource returns the registered source text of one fact-pack query of a route.
func factQuerySource(t *testing.T, data []byte, route, id string) []byte {
	t.Helper()
	var pack struct {
		Routes []struct {
			Route   string `json:"route"`
			Queries []struct {
				ID     string `json:"id"`
				Source string `json:"source"`
			} `json:"queries"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(data, &pack); err != nil {
		t.Fatal(err)
	}
	for _, r := range pack.Routes {
		for _, q := range r.Queries {
			if r.Route == route && q.ID == id {
				return []byte(q.Source)
			}
		}
	}
	t.Fatalf("fact query %s/%s absent", route, id)
	return nil
}

func allVerified(js []kit.LargeJudgment) bool {
	for _, j := range js {
		if j.State != "VERIFIED" {
			return false
		}
	}
	return true
}
