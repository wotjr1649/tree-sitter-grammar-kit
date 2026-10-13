package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

type holdoutCase struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Valid  bool   `json:"valid"`
}

func jsonHoldout(t *testing.T) ([]holdoutCase, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "native", "json-holdout.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []holdoutCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 36 {
		t.Fatal("holdout inventory changed; revise the evaluation scope")
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if c.ID == "" || seen[c.ID] || json.Valid([]byte(c.Source)) != c.Valid {
			t.Fatalf("independent label/control: %s", c.ID)
		}
		seen[c.ID] = true
	}
	sum := sha256.Sum256(data)
	return cases, hex.EncodeToString(sum[:])
}

// Labels are fixed and checked by the standard library before native observation.
func TestJSONHoldoutLabels(t *testing.T) { jsonHoldout(t) }

func TestNativeJSONHoldout(t *testing.T) {
	cases, inventory := jsonHoldout(t)
	if os.Getenv("TSGK_BENCH_PREPARED") == "" {
		t.Skip("JSON holdout prepared closure not configured")
	}
	built := benchmarkJSONBuild(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	tp, tn, fp, fn := 0, 0, 0, 0
	for _, c := range cases {
		run := testContext("native-parse-edit")
		result := built.RunCase(ctx, run, kit.IncrementalCase{ID: c.ID, Encoding: kit.EncodingUTF8}, []byte(c.Source))
		if result.ExecutionStatus != kit.StatusCompleted || result.Process == nil || !result.Process.Succeeded() || len(result.Steps) != 1 || result.Steps[0].Incremental == nil {
			t.Fatalf("observation unavailable %s: %s %s", c.ID, result.ExecutionStatus, result.Code)
		}
		step := result.Steps[0]
		if step.Incremental.Tree == nil {
			t.Fatal("full recovery tree missing")
		}
		if err := kit.ValidateTree(step.Incremental.Tree.Nodes, uint64(len(c.Source))); err != nil {
			t.Fatalf("recovery tree %s: %v", c.ID, err)
		}
		sum := sha256.Sum256([]byte(c.Source))
		if step.SourceSHA256 != hex.EncodeToString(sum[:]) || step.SourceBytes != uint64(len(c.Source)) {
			t.Fatal("holdout identity drift")
		}
		accepted := !step.Incremental.HasError
		switch {
		case c.Valid && accepted:
			tp++
		case !c.Valid && !accepted:
			tn++
		case !c.Valid && accepted:
			fp++
		case c.Valid && !accepted:
			fn++
		}
		t.Logf("holdout id=%s oracle_valid=%t native_accepted=%t input_sha256=%s", c.ID, c.Valid, accepted, step.SourceSHA256)
	}
	// A disagreement is evaluation data, not permission to relabel or weaken a gate.
	t.Logf("holdout inventory_sha256=%s cases=%d TP=%d TN=%d FP=%d FN=%d", inventory, len(cases), tp, tn, fp, fn)
	for _, c := range []struct {
		id, before, after, route string
		edit                     kit.Edit
	}{
		{"repair-close", "[1", "[1]", ClaimBlocked, kit.Edit{StartByte: 2, OldEndByte: 2, NewEndByte: 3, New: []byte("]")}},
		{"break-close", "[1]", "[1", ClaimBlocked, kit.Edit{StartByte: 2, OldEndByte: 3, NewEndByte: 2, Old: []byte("]")}},
		{"replace-number", "[12345,2]", "[9,2]", ClaimPass, kit.Edit{StartByte: 1, OldEndByte: 6, NewEndByte: 2, Old: []byte("12345"), New: []byte("9")}},
		{"repair-colon", "{\"a\" 1}", "{\"a\":1}", ClaimBlocked, kit.Edit{StartByte: 4, OldEndByte: 5, NewEndByte: 5, Old: []byte(" "), New: []byte(":")}},
	} {
		result := built.RunCase(ctx, testContext("native-parse-edit"), kit.IncrementalCase{ID: c.id, Encoding: kit.EncodingUTF8, Edits: []kit.Edit{c.edit}}, []byte(c.before))
		if result.ExecutionStatus != kit.StatusCompleted || result.Process == nil || !result.Process.Succeeded() || result.Claims.IncrementalEquality != ClaimPass || len(result.Steps) != 2 {
			t.Fatalf("holdout edit %s: %s %s", c.id, result.Assessment, result.Code)
		}
		// Pin these observed cases explicitly. Error trees can reuse nodes in other cases.
		if result.Claims.IncrementalRoute != c.route {
			t.Fatalf("route expectation changed %s: %s", c.id, result.Claims.IncrementalRoute)
		}
		if c.route == ClaimPass {
			if result.Assessment != kit.AssessPass {
				t.Fatal("proven route assessment mismatch")
			}
		} else {
			route := result.Steps[1].Route
			if result.Assessment != kit.AssessBlocked || result.Code != "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1" || result.Claims.IncrementalRoute != ClaimBlocked || route == nil || !route.EditHasChanges || route.ReusedNodes != 0 || route.FreshReusedNodes != 0 || (json.Valid([]byte(c.before)) && json.Valid([]byte(c.after))) {
				t.Fatalf("unexpected route classification %s: %s %s", c.id, result.Assessment, result.Code)
			}
		}
		for i, source := range []string{c.before, c.after} {
			step := result.Steps[i]
			sum := sha256.Sum256([]byte(source))
			if step.SourceSHA256 != hex.EncodeToString(sum[:]) || step.SourceBytes != uint64(len(source)) || step.Incremental == nil || step.Incremental.HasError == json.Valid([]byte(source)) {
				t.Fatalf("independent edit syntax %s step %d", c.id, i)
			}
		}
		t.Logf("holdout_edit id=%s independent_syntax_agreement=true incremental_fresh_equal=true route=%s assessment=%s", c.id, result.Claims.IncrementalRoute, result.Assessment)
	}
}
