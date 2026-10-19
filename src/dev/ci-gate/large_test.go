package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/largeaudit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func largeFixture(t *testing.T) (largeResult, largeaudit.Inventory) {
	t.Helper()
	runtime, e := native.LoadRuntimeManifest()
	if e != nil {
		t.Fatal(e)
	}
	op := kit.NativeOperations()["native-query-large"]
	process := runner.Result{Status: runner.StatusCompleted, Reason: runner.ReasonExited, Cleanup: runner.Cleanup{Verified: true}, Memory: runner.MemoryObservation{Limit: op.MemoryBytes, Enforcement: "HARD", PeakKnown: true, Peak: 100}, WallMillis: 10}
	build := []native.BuildStep{{Executed: true, Result: process}}
	result := largeResult{Schema: "tsgk-large-api-validation/r3", SessionSource: strings.Repeat("e", 64), SessionExecutable: strings.Repeat("f", 64), SessionCompile: []native.BuildStep{{Name: "audit", Executed: true, Result: process, Argv: []string{"-c", "-O2", "-std=gnu11", "-I", ".", "-I", "runtime/lib/include", "-I", "driver", "session-audit.c", "-o", "audit.o"}}, {Name: "audit", Executed: true, Result: process, Argv: []string{"lib.o", "parser.o", "scanner0.o", "audit.o", "shim.o", "-o", "api-session-audit.exe"}}}, Mode: "REGISTERED_LARGE_ALL_NODES", Complete: true, Pass: true, Inventory: largeaudit.FixtureInventorySHA, Registry: strings.Repeat("b", 64), Source: strings.Repeat("c", 64), Executable: strings.Repeat("d", 64), Compile: build, Build: &native.Build{Identity: strings.Repeat("a", 64), Runtime: runtime.Commit, Symbol: "tree_sitter_c_sharp", Steps: build}}
	var inv largeaudit.Inventory
	makeCase := func(id, source string, n uint64, full bool) native.CaseResult {
		inc := &native.TreeOut{Status: kit.StatusCompleted, DescendantCount: n}
		if full {
			tree := &native.TreeEnvelope{Schema: kit.TreeSchema, Status: kit.StatusCompleted, Input: native.TreeInput{Bytes: uint64(len(source)), SHA256: digest([]byte(source)), Encoding: kit.EncodingUTF8}, Nodes: []kit.TreeNode{{Parent: -1, Type: "compilation_unit", Named: true, EndByte: uint32(len(source)), EndPoint: kit.Point{Row: 1}}}}
			inc.Tree = tree
			inc.Digest = kit.TreeDigest(tree.Nodes)
			inc.API = &native.APIOut{Revision: "tsgk-api/r1", Consistent: true}
		}
		producer := &native.Producer{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, FieldCount: new(uint32(1))}
		if !full {
			producer.FieldCount = nil
			producer.Query = "UNSUPPORTED"
		}
		return native.CaseResult{ID: id, ExecutionStatus: kit.StatusCompleted, Assessment: kit.AssessPass, Process: &process, Producer: producer, Oracle: &native.OracleClaims{API: native.ClaimPass}, Steps: []native.StepResult{{SourceBytes: uint64(len(source)), SourceSHA256: digest([]byte(source)), Incremental: inc}}}
	}
	observation := func(id, source string, n, start, end, children uint64, points int) auditObservation {
		fields := uint64(1)
		lookups := 2 * (end - start) * fields
		pc := 3 * uint64(points+2)
		r := largeaudit.Receipt{Schema: "tsgk-large-api-audit/r1", ID: id, SourceSHA: digest([]byte(source)), SourceBytes: uint64(len(source)), Nodes: n, Start: start, End: end, Depth: 1, Fields: fields, ChildFields: children, FieldLookups: lookups, PointChecks: pc, Checks: 13*(end-start) + children + lookups + pc, First: []int64{0, 0, 0, 0}, FirstDivergence: []int64{0, 0, 0, 0}, ExpectedSHA: strings.Repeat("a", 64), ObservedSHA: strings.Repeat("a", 64), Complete: true}
		raw := native.Frame(encoded(t, r))
		p := process
		p.StdoutBytes = int64(len(raw))
		return auditObservation{Executable: result.Executable, Receipt: encoded(t, r), Raw: raw, Process: encoded(t, p)}
	}
	for i, s := range largeaudit.SmallSources() {
		id := fmt.Sprintf("small-%d", i)
		result.Small = append(result.Small, makeCase(id, s, 1, true))
		result.Observations = append(result.Observations, observation(id, s, 1, 0, 1, 0, 1))
	}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("large-%d", i)
		s := "class C {}\n"
		inv.Fixtures = append(inv.Fixtures, largeaudit.Fixture{ID: id, Block: s, Repeat: 1, Bytes: uint64(len(s)), SHA256: digest([]byte(s))})
		n := uint64(2)
		if i == 0 {
			n = 1000001
		}
		result.Baseline = append(result.Baseline, makeCase(id, s, n, false))
		session := largeaudit.Session{Schema: largeaudit.SessionSchema, ID: id, Session: strings.Repeat("0", 32), Nodes: n, PageSize: 1000000, Complete: true}
		req := native.Request{ID: id, Encoding: kit.EncodingUTF8, Output: kit.OutputAuto, Limits: native.LimitsFor(op), Source: []byte(s)}
		session.RequestSHA = digest(native.Frame(req.Encode()))
		session.Batch.Process = process

		for start := uint64(0); start < n; {
			end := min(start+1000000, n)
			children := uint64(0)
			if start == 0 {
				children = n - 1
			}
			result.Observations = append(result.Observations, observation(id, s, n, start, end, children, 0))
			page := largeaudit.SessionPage{Schema: largeaudit.SessionPageSchema, Session: session.Session, Index: uint64(len(session.RawPages)), Parses: 1, SessionComplete: end == n, Receipt: result.Observations[len(result.Observations)-1].Receipt, Complete: true}
			payload := encoded(t, page)
			session.RawPages = append(session.RawPages, payload)
			session.Batch.Frames = append(session.Batch.Frames, runner.FrameResult{ID: fmt.Sprintf("page-%d", len(session.Batch.Frames)), Status: runner.FrameCompleted, WallMillis: 1})
			session.Batch.Process.StdoutBytes += int64(4 + len(payload))

			start = end
		}
		session.BudgetMillis = int64(len(session.RawPages)) * op.ParseWall.Milliseconds()
		result.Sessions = append(result.Sessions, session)
	}
	for i, f := range []string{"PARENT", "SIBLING", "COUNT", "CURSOR", "FIELD_NAME", "FIELD_LOOKUP", "POINT", "POSITIVE_SKIP", "ZERO_SKIP"} {
		o := result.Observations[0]
		o.Fault = f
		o.ExpectedFault = true
		o.Executable = fmt.Sprintf("%064x", i+1)
		o.Compile = []native.BuildStep{{Executed: true, Argv: []string{"-DTSGK_AUDIT_FAULT_" + f}, Result: process}}
		p := process
		p.Status = runner.StatusFailed
		p.ExitCode = 1
		p.Reason = runner.ReasonExited
		var r largeaudit.Receipt
		if e := json.Unmarshal(o.Receipt, &r); e != nil {
			t.Fatal(e)
		}
		if f == "ZERO_SKIP" {
			o.Receipt = result.Observations[3].Receipt
			if e := json.Unmarshal(o.Receipt, &r); e != nil {
				t.Fatal(e)
			}
			r.Divergences = 1
			r.FirstDivergence = []int64{0, 1, 0, 1}
			p = process
		} else {
			r.Differences = 1
			r.ObservedSHA = strings.Repeat("b", 64)
			r.First = []int64{0, 0, 0, 1}
		}
		o.Receipt = encoded(t, r)
		o.Process = encoded(t, p)
		result.Faults = append(result.Faults, o)
	}
	p := process
	p.Status = runner.StatusFailed
	p.ExitCode = 3
	p.Reason = runner.ReasonExited
	req := largeaudit.ResourceRequest()
	frame := native.Frame([]byte(fmt.Sprintf(`{"protocol":"tsgk-native/r1","id":"small-0","status":"RESOURCE_LIMIT","code":"ALLOCATION_LIMIT","producer":{"language_version":15,"runtime_language_version":15,"runtime_min_compatible":13,"query":"UNSUPPORTED"},"source_bytes":%d,"steps_completed":0,"steps":[],"complete":true}`, len(req.Source))))
	p.StdoutBytes = int64(len(frame))
	capture, e := largeaudit.CaptureResource(req, native.Frame(req.Encode()), frame, p)
	if e != nil {
		t.Fatal(e)
	}
	result.Resources = []auditObservation{{Executable: result.Executable, Error: "RESOURCE_LIMIT:ALLOCATION_LIMIT", Process: encoded(t, p), Receipt: encoded(t, largeaudit.Receipt{}), Resource: capture}}
	return result, inv
}

func TestAuditProcessLimits(t *testing.T) {
	op := kit.NativeOperations()["native-query-large"]
	good := runner.Result{Status: runner.StatusCompleted, Reason: runner.ReasonExited, Cleanup: runner.Cleanup{Verified: true}, Memory: runner.MemoryObservation{Limit: op.MemoryBytes, Enforcement: "HARD", PeakKnown: true}, WallMillis: 10}
	if _, e := processResult(encoded(t, good), runner.StatusCompleted, 0); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*runner.Result){
		"negative-output": func(p *runner.Result) { p.StdoutBytes = -1 },
		"output-cap":      func(p *runner.Result) { p.StdoutBytes = int64(op.OutputBytes) + 5 },
		"stderr-cap":      func(p *runner.Result) { p.StderrBytes = 65537 },
		"wall-cap":        func(p *runner.Result) { p.WallMillis = op.ParseWall.Milliseconds() + 1 },
		"memory-cap":      func(p *runner.Result) { p.Memory.Limit++ },
		"soft-memory":     func(p *runner.Result) { p.Memory.Enforcement = "SAMPLED" },
		"residual":        func(p *runner.Result) { p.Cleanup.ResidualAfterExit = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := good
			change(&bad)
			if _, e := processResult(encoded(t, bad), runner.StatusCompleted, 0); e == nil {
				t.Fatal("invalid process limits accepted")
			}
		})
	}
}

func TestLargeGateCoverageAndControls(t *testing.T) {
	good, inv := largeFixture(t)
	data := encoded(t, good)
	if _, e := checkLarge(data, inv, good.Registry, good.Source, good.SessionSource, "tsgk-large-api-validation/r3"); e != nil {
		t.Fatal(e)
	}
	controls := map[string]func(*largeResult){
		"missing-session":   func(r *largeResult) { r.Sessions = r.Sessions[:2] },
		"duplicate-session": func(r *largeResult) { r.Sessions[0] = r.Sessions[1] },
		"session-source":    func(r *largeResult) { r.SessionSource = strings.Repeat("a", 64) },
		"session-fault-compile": func(r *largeResult) {
			r.SessionCompile[0].Argv = append(r.SessionCompile[0].Argv, "-DTSGK_AUDIT_FAULT_PARENT")
		},
		"session-preprocessor-escape": func(r *largeResult) {
			r.SessionCompile[0].Argv = append(r.SessionCompile[0].Argv, "-Wp,-DTSGK_AUDIT_FAULT_PARENT")
		},
		"session-response-file":    func(r *largeResult) { r.SessionCompile[0].Argv = append(r.SessionCompile[0].Argv, "@extra-options") },
		"session-foreign-source":   func(r *largeResult) { r.SessionCompile[0].Argv[9] = "elsewhere/session-audit.c" },
		"session-foreign-object":   func(r *largeResult) { r.SessionCompile[1].Argv[3] = "other.o" },
		"session-wrong-output":     func(r *largeResult) { r.SessionCompile[1].Argv[6] = "other.exe" },
		"session-no-output-option": func(r *largeResult) { r.SessionCompile[1].Argv[5] = "api-session-audit.exe" },
		"session-incomplete":       func(r *largeResult) { r.Sessions[0].Complete = false },
		"missing-actual-frame":     func(r *largeResult) { r.Observations[0].Raw = nil },
		"actual-frame-mismatch":    func(r *largeResult) { r.Observations[0].Raw = r.Observations[1].Raw },
		"drop":                     func(r *largeResult) { r.Observations = r.Observations[:len(r.Observations)-1] },
		"duplicate":                func(r *largeResult) { r.Observations = append(r.Observations, r.Observations[4]) },
		"reorder":                  func(r *largeResult) { r.Observations[4], r.Observations[5] = r.Observations[5], r.Observations[4] },
		"missing-small":            func(r *largeResult) { r.Observations = r.Observations[1:] },
		"incomplete":               func(r *largeResult) { r.Complete = false },
		"foreign-source":           func(r *largeResult) { r.Source = strings.Repeat("f", 64) },
		"missing-fault":            func(r *largeResult) { r.Faults = r.Faults[1:] },
		"duplicate-fault":          func(r *largeResult) { r.Faults[0] = r.Faults[1] },
		"missing-resource":         func(r *largeResult) { r.Resources = nil },
		"historical-r1":            func(r *largeResult) { r.Schema = "tsgk-large-api-validation/r1" },
		"missing-resource-frame":   func(r *largeResult) { r.Resources[0].Resource = nil },
		"resource-request-sha":     func(r *largeResult) { r.Resources[0].Resource.RequestSHA = strings.Repeat("e", 64) },
		"resource-stdout-sha":      func(r *largeResult) { r.Resources[0].Resource.StdoutSHA = strings.Repeat("e", 64) },
		"resource-producer": func(r *largeResult) {
			c := r.Resources[0].Resource
			c.Stdout = native.Frame([]byte(strings.Replace(string(c.Stdout[4:]), `"language_version":15`, `"language_version":14`, 1)))
			c.StdoutSHA = digest(c.Stdout)
		},
		"resource-empty-object":  func(r *largeResult) { r.Resources[0].Receipt = []byte(`{}`) },
		"resource-wrong-receipt": func(r *largeResult) { r.Resources[0].Receipt = r.Observations[0].Receipt },
		"resource-null-scalar": func(r *largeResult) {
			r.Resources[0].Receipt = []byte(strings.Replace(string(r.Resources[0].Receipt), `"complete":false`, `"complete":null`, 1))
		},
		"fault-termination-reason": func(r *largeResult) {
			var p runner.Result
			if e := json.Unmarshal(r.Faults[0].Process, &p); e != nil {
				t.Fatal(e)
			}
			p.Reason = "CALLER_CANCELLED"
			r.Faults[0].Process = encoded(t, p)
		},
		"small-fields-absent":      func(r *largeResult) { r.Small[0].Producer.FieldCount = nil },
		"large-producer-ABI":       func(r *largeResult) { r.Baseline[0].Producer.LanguageVersion = 14 },
		"baseline-fields-mismatch": func(r *largeResult) { r.Baseline[0].Producer.FieldCount = new(uint32(2)) },
		"reduced-field-coverage": func(r *largeResult) {
			var page largeaudit.Receipt
			if e := json.Unmarshal(r.Observations[4].Receipt, &page); e != nil {
				t.Fatal(e)
			}
			page.Checks -= page.FieldLookups
			page.FieldLookups = 0
			page.Fields = 0
			r.Observations[4].Receipt = encoded(t, page)
		},
		"resource-success":   func(r *largeResult) { r.Resources[0].Process = r.Observations[0].Process },
		"fault-not-detected": func(r *largeResult) { r.Faults[0].Receipt = r.Observations[0].Receipt },
		"fault-build-mixed": func(r *largeResult) {
			r.Faults[0].Compile[0].Argv = append(r.Faults[0].Compile[0].Argv, "-DTSGK_AUDIT_FAULT_SIBLING")
		},
		"null-exit": func(r *largeResult) {
			r.Observations[0].Process = []byte(strings.Replace(string(r.Observations[0].Process), `"exit_code":0`, `"exit_code":null`, 1))
		},
		"normal-fault":     func(r *largeResult) { r.Observations[0].ExpectedFault = true },
		"tree-corruption":  func(r *largeResult) { r.Small[0].Steps[0].Incremental.Tree.Nodes[0].Type = "changed" },
		"api-inconsistent": func(r *largeResult) { r.Small[0].Steps[0].Incremental.API.Consistent = false },
	}
	for name, change := range controls {
		t.Run(name, func(t *testing.T) {
			var bad largeResult
			if e := json.Unmarshal(data, &bad); e != nil {
				t.Fatal(e)
			}
			change(&bad)
			if _, e := checkLarge(encoded(t, bad), inv, good.Registry, good.Source, good.SessionSource, "tsgk-large-api-validation/r3"); e == nil {
				t.Fatal("invalid large API gate accepted")
			}
		})
	}
	for name, bad := range map[string][]byte{
		"null-zero":       []byte(strings.Replace(string(data), `"range_start":0`, `"range_start":null`, 1)),
		"absent-zero":     []byte(strings.Replace(string(data), `"differences":0,`, "", 1)),
		"null-fault-flag": []byte(strings.Replace(string(data), `"expected_fault":false`, `"expected_fault":null`, 1)),

		"truncated": data[:len(data)-1],
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := checkLarge(bad, inv, good.Registry, good.Source, good.SessionSource, "tsgk-large-api-validation/r3"); e == nil {
				t.Fatal("malformed audit accepted")
			}
		})
	}
}

func TestLargeSessionGate(t *testing.T) {
	good, inv := largeFixture(t)
	good.Schema = "tsgk-large-api-validation/r4"
	good.Mode = "REGISTERED_LARGE_SINGLE_SESSION"
	good.Observations = good.Observations[:4]
	good.Sessions[1].Batch.Process.Memory.Peak = 500 // the session, not a page, holds the peak
	data := encoded(t, good)
	checked, err := checkLarge(data, inv, good.Registry, good.Source, good.SessionSource, "tsgk-large-api-validation/r4")
	if err != nil || checked["pages"] != 4 || checked["session_fixtures"] != 3 {
		t.Fatalf("promoted gate: %v %v", checked, err)
	}
	op := kit.NativeOperations()["native-query-large"]
	if checked["max_memory_bytes"] != uint64(500) || checked["memory_peak_source"] != good.Sessions[1].ID || checked["memory_limit_bytes"] != op.MemoryBytes ||
		checked["memory_headroom_bytes"] != op.MemoryBytes-500 || checked["memory_metric"] != "" {
		t.Fatalf("memory observation: %v", checked)
	}
	for name, change := range map[string]func(*largeResult){
		"historical-evaluation-fallback": func(r *largeResult) { r.Schema = "tsgk-large-api-validation/r3"; r.Mode = "REGISTERED_LARGE_ALL_NODES" },
		"missing-session":                func(r *largeResult) { r.Sessions = r.Sessions[:2] },
		"missing-page":                   func(r *largeResult) { r.Sessions[0].RawPages = r.Sessions[0].RawPages[:1] },
		"reordered-pages":                func(r *largeResult) { s := &r.Sessions[0]; s.RawPages[0], s.RawPages[1] = s.RawPages[1], s.RawPages[0] },
		"duplicate-session":              func(r *largeResult) { r.Sessions[0] = r.Sessions[1] },
		"incomplete":                     func(r *largeResult) { r.Sessions[0].Complete = false },
		"bad-exit-after-receipt": func(r *largeResult) {
			r.Sessions[0].Batch.Process.Status = runner.StatusFailed
			r.Sessions[0].Batch.Process.ExitCode = 2
		},
		"cleanup":             func(r *largeResult) { r.Sessions[0].Batch.Process.Cleanup.Verified = false },
		"memory":              func(r *largeResult) { r.Sessions[0].Batch.Process.Memory.LimitHit = true },
		"mixed-memory-metric": func(r *largeResult) { r.Sessions[0].Batch.Process.Memory.Metric = "other" },
		"wall":                func(r *largeResult) { r.Sessions[0].Batch.Frames[0].WallMillis = 90001 },
		"trailing":            func(r *largeResult) { r.Sessions[0].Batch.TrailingBytes = 1 },
		"missing-small":       func(r *largeResult) { r.Small = r.Small[:3] },
		"missing-fault":       func(r *largeResult) { r.Faults = r.Faults[:8] },
		"missing-resource":    func(r *largeResult) { r.Resources = nil },
		"copied-page-process": func(r *largeResult) { r.Observations = append(r.Observations, r.Observations[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			var bad largeResult
			if err := json.Unmarshal(data, &bad); err != nil {
				t.Fatal(err)
			}
			change(&bad)
			if _, err := checkLarge(encoded(t, bad), inv, good.Registry, good.Source, good.SessionSource, "tsgk-large-api-validation/r4"); err == nil {
				t.Fatal("invalid promoted gate accepted")
			}
		})
	}
	for _, kind := range []string{"nodes", "fields"} {
		t.Run("self-consistent-fake-"+kind, func(t *testing.T) {
			var bad largeResult
			if err := json.Unmarshal(data, &bad); err != nil {
				t.Fatal(err)
			}
			session := &bad.Sessions[0]
			if kind == "nodes" {
				session.Nodes++
			}
			for i, raw := range session.RawPages {
				var page largeaudit.SessionPage
				if err := json.Unmarshal(raw, &page); err != nil {
					t.Fatal(err)
				}
				var receipt largeaudit.Receipt
				if err := json.Unmarshal(page.Receipt, &receipt); err != nil {
					t.Fatal(err)
				}
				if kind == "nodes" {
					receipt.Nodes++
					if i == 0 {
						receipt.ChildFields++
					}
					if i == len(session.RawPages)-1 {
						receipt.End++
					}
				} else {
					receipt.Fields++
				}
				receipt.FieldLookups = 2 * (receipt.End - receipt.Start) * receipt.Fields
				receipt.Checks = 13*(receipt.End-receipt.Start) + receipt.ChildFields + receipt.FieldLookups + receipt.PointChecks
				page.Receipt = encoded(t, receipt)
				payload := encoded(t, page)
				session.Batch.Process.StdoutBytes += int64(len(payload) - len(raw))
				session.RawPages[i] = payload
			}
			f := inv.Fixtures[0]
			source, err := largeaudit.Source(f)
			if err != nil {
				t.Fatal(err)
			}
			req := native.Request{ID: f.ID, Encoding: kit.EncodingUTF8, Output: kit.OutputAuto, Limits: native.LimitsFor(kit.NativeOperations()["native-query-large"]), Source: source, Points: f.Points}
			claimedFields := uint64(1)
			if kind == "fields" {
				claimedFields = 2
			}
			// The altered packet is internally consistent, but disagrees with independent evidence.
			if _, err := largeaudit.ValidateSession(*session, req, session.Nodes, claimedFields); err != nil {
				t.Fatalf("fake control malformed: %v", err)
			}
			if _, err := checkLarge(encoded(t, bad), inv, good.Registry, good.Source, good.SessionSource, "tsgk-large-api-validation/r4"); err == nil {
				t.Fatal("self-consistent claim replaced independent baseline/field inventory")
			}
		})
	}
}
