package native

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// This captures the existing normal/omit runs after their historical assertions;
// it never launches a second holdout execution or changes their legacy verdicts.
type acceptedHoldoutCapture struct {
	output, responses, sources                   *outputDirectory
	profile                                      map[string]any
	cases                                        []kit.IncrementalCase
	result                                       Result
	responsesClosed, sourcesClosed, outputClosed bool
}

func registeredGrammar(t testing.TB, route string) (string, []kit.NativeInput) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "contracts", "native-routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reg struct {
		Routes []struct {
			Route, Symbol string
			Files         []kit.NativeInput
		}
	}
	if err := json.Unmarshal(b, &reg); err != nil {
		t.Fatal(err)
	}
	for _, r := range reg.Routes {
		if r.Route == route {
			return r.Symbol, r.Files
		}
	}
	t.Fatalf("registered grammar missing: %s", route)
	return "", nil
}

func captureAcceptedHoldout(t *testing.T, ctx context.Context, dest string, data []byte, ledger *kit.ReuseAcceptedLedger, captures map[string]*acceptedHoldoutCapture, omit bool, x Context, built *Build, c kit.IncrementalCase, source []byte, r CaseResult) {
	t.Helper()
	key := x.Route
	if omit {
		key += "-omit"
	}
	capture := captures[key]
	if capture == nil {
		set := "reuse-accepted-" + key
		output, err := newOutputDirectory(filepath.Join(dest, set), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if capture == nil || !capture.outputClosed {
				if err := output.Close(); err != nil {
					t.Error(err)
				}
			}
		})
		responses, err := createOutputDirectory(output.Root, "responses")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if capture == nil || !capture.responsesClosed {
				if err := responses.Close(); err != nil {
					t.Error(err)
				}
			}
		})
		sources, err := createOutputDirectory(output.Root, "sources")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if capture == nil || !capture.sourcesClosed {
				if err := sources.Close(); err != nil {
					t.Error(err)
				}
			}
		})
		if err := createExclusive(output.Root, "reuse-ledger.json", data); err != nil {
			t.Fatal(err)
		}
		symbol, grammar := registeredGrammar(t, x.Route)
		profile := map[string]any{"schema": kit.IncrementalSchema, "id": set, "route": x.Route, "operation": x.Op.Name, "symbol": symbol, "encoding": kit.EncodingUTF8, "output": kit.OutputTree, "compiler": built.Compiler, "grammar": grammar, "declarations": nil}
		capture = &acceptedHoldoutCapture{output: output, responses: responses, sources: sources, profile: profile,
			result: Result{Report: kit.Report{Schema: kit.ReportSchema, Command: "incremental", EvidenceMode: kit.ModeNewRun, Findings: []kit.Finding{}},
				ResultSchema: AcceptedResultSchema, ReuseAcceptedPolicy: kit.ReuseAcceptedSchema, ReuseAcceptedLedgerSHA256: kit.ReuseAcceptedLedgerSHA256, ReuseObservationPolicy: reuseobs.Policy, ObservationRunID: x.ObservationRunID,
				ProfileID: set, Route: x.Route, Operation: x.Op, Output: kit.OutputTree, Build: built, BuildRemoved: "KEPT_BY_TEST_CACHE", Platform: runtime.GOOS + "/" + runtime.GOARCH, Cases: []CaseResult{}, Batches: []BatchRecord{}}}
		captures[key] = capture
	}
	versions, points, err := kit.ApplyEdits(c.Encoding, source, c.Edits, x.Op.InputBytes)
	if err != nil {
		t.Fatal(err)
	}
	actual := x.request(c, source).expect(versions, points, &r.Process.ExitCode)
	r.acceptedExpected = &actual
	x.ReuseAccepted = ledger
	x.acceptCase(ctx, c, source, &r)
	want := ClaimPass
	if omit {
		want = ClaimFail
	}
	if r.ReuseAccepted == nil || r.ReuseAccepted.Result != want || !omit && (r.Assessment != kit.AssessPass || r.Claims.IncrementalRoute != ClaimPass) || omit && r.Assessment != kit.AssessFail {
		t.Fatalf("accepted %s/%s: proof=%+v status=%s assessment=%s code=%s", key, c.ID, r.ReuseAccepted, r.ExecutionStatus, r.Assessment, r.Code)
	}
	index := len(capture.cases)
	if err := writeAcceptedMembers(capture.responses.Root, capture.sources.Root, index, c.ID, r.Raw, source); err != nil {
		t.Fatal(err)
	}
	capture.cases = append(capture.cases, c)
	capture.result.Cases = append(capture.result.Cases, r)
}

func (c *acceptedHoldoutCapture) finish(t *testing.T) {
	t.Helper()
	expected := 12
	if c.result.Route == "json" {
		expected = 3
	}
	if len(c.cases) != expected {
		t.Fatalf("unexpected capture count: %d want %d", len(c.cases), expected)
	}
	err := errors.Join(c.responses.Close(), c.sources.Close())
	c.responsesClosed, c.sourcesClosed = true, true
	if err != nil {
		t.Fatal(err)
	}
	c.profile["cases"] = c.cases
	profile, err := json.Marshal(c.profile)
	if err != nil {
		t.Fatal(err)
	}
	prof, err := kit.ParseIncrementalProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	c.result.ProfileSHA256 = sha(profile)
	c.result.Identities = []kit.IdentityRef{{Role: "profile", Schema: prof.Schema, SHA256: sha(profile)}, kit.NativePolicy(c.result.Operation, kit.OutputTree, nil), {Role: "producer", Schema: BuildSchema, SHA256: c.result.Build.Identity}}
	c.result.ExecutionStatus, c.result.Assessment = aggregate(c.result.Cases)
	c.result.Summary = summarize(c.result.Cases, nil)
	if err := createExclusive(c.output.Root, "profile.json", profile); err != nil {
		t.Fatal(err)
	}
	result, err := json.Marshal(c.result)
	if err != nil {
		t.Fatal(err)
	}
	if err := createExclusive(c.output.Root, "result.json", append(result, '\n')); err != nil {
		t.Fatal(err)
	}
	t.Logf("accepted capture set=%s histories=%d bytes=%d cached_build=%s", c.result.ProfileID, len(c.cases), len(result), c.result.BuildRemoved)
	err = c.output.Close()
	c.outputClosed = true
	if err != nil {
		t.Fatal(err)
	}
}
