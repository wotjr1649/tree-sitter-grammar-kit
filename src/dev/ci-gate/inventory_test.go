package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func committedInventory(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "contracts", "qualification-c1.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// #332: the CI inventory is the committed one minus darwin-arm64; the committed file is never rewritten.
func TestDerivedInventory(t *testing.T) {
	data := committedInventory(t)
	full, err := kit.ParseQualificationInventory(data)
	if err != nil {
		t.Fatal(err)
	}
	derived, err := derivedInventory(data, droppedPlatforms)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := kit.ParseQualificationInventory(derived)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Platforms) != 2 || inv.Platforms[0].ID != "windows-amd64" || inv.Platforms[1].ID != "linux-amd64" ||
		inv.Cells != 52 || inv.Cells != len(inv.Routes)*len(inv.Platforms) || inv.ID != full.ID+"-windows-linux" || inv.SHA256 == full.SHA256 {
		t.Fatalf("derived header: platforms=%v cells=%d id=%s", inv.Platforms, inv.Cells, inv.ID)
	}
	if len(inv.ExtraRoles) != len(full.ExtraRoles) {
		t.Fatal("extra role set changed")
	}
	for _, role := range inv.ExtraRoles {
		if slices.Contains(role.Platforms, "darwin-arm64") || role.NotApplicable["darwin-arm64"] != "" || role.Deferred["darwin-arm64"] != "" {
			t.Fatalf("darwin remains in role %s", role.ID)
		}
	}
	// the host list the gate reads is exactly the derived inventory's
	if err := checkGateHosts(inv); err != nil {
		t.Fatal(err)
	}
	if checkGateHosts(full) == nil {
		t.Fatal("full three-platform inventory accepted by the two-host gate")
	}
	// the qualification job and the gate build the same bytes
	again, err := derivedInventory(data, droppedPlatforms)
	if err != nil || !bytes.Equal(derived, again) {
		t.Fatal("derived inventory is not deterministic", err)
	}
	// no drop returns the committed bytes, the re-enabling path
	same, err := derivedInventory(data, nil)
	if err != nil || !bytes.Equal(same, data) {
		t.Fatal("empty drop must return the committed bytes", err)
	}
}

func TestDerivedInventoryRejects(t *testing.T) {
	data := committedInventory(t)
	for name, drop := range map[string][]string{
		"unregistered":        {"plan9-386"},
		"one-registered-only": {"darwin-arm64", "plan9-386"},
		// n461-large executes on Windows only: dropping it leaves a role that executes nowhere
		"sole-executor": {"windows-amd64"},
		"all":           {"windows-amd64", "linux-amd64", "darwin-arm64"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := derivedInventory(data, drop); err == nil {
				t.Fatal("invalid drop accepted")
			}
		})
	}
	if _, err := derivedInventory([]byte(`{}`), droppedPlatforms); err == nil {
		t.Fatal("unreadable inventory accepted")
	}
	if !mentions(map[string]any{"a": []any{map[string]any{"darwin-arm64": "x"}}}, droppedPlatforms) ||
		!mentions([]any{"darwin-arm64"}, droppedPlatforms) || mentions(map[string]any{"a": "windows-amd64"}, droppedPlatforms) {
		t.Fatal("leftover platform scan is wrong")
	}
}

// the gate stays platform-generic: it rejects a result of the other inventory
func TestQualificationRejectsOtherInventoryResult(t *testing.T) {
	two, inv2 := qualificationFixture(t)
	three, inv3 := fullQualificationFixture(t)
	if checkQualification(two, expectation(), inv2) != nil || checkQualification(three, expectation(), inv3) != nil {
		t.Fatal("matching inventory rejected")
	}
	if checkQualification(three, expectation(), inv2) == nil || checkQualification(two, expectation(), inv3) == nil {
		t.Fatal("result of another platform set accepted")
	}
}

// the registered deferred rows of the full inventory still bind the gate
func TestQualificationDeferredRowsOfFullInventory(t *testing.T) {
	for name, mutate := range map[string]func(*kit.QualificationResult){
		"deferred_executed": func(q *kit.QualificationResult) {
			q.ExtraRoles[roleRow(q, "csharp-symbols", "darwin-arm64")].Status = "PASS"
		},
		"deferred_axis": func(q *kit.QualificationResult) {
			q.ExtraRoles[roleRow(q, "csharp-symbols", "darwin-arm64")].Mechanism = "PASS"
		},
	} {
		t.Run(name, func(t *testing.T) {
			q, inv := fullQualificationFixture(t)
			mutate(&q)
			if checkQualification(q, expectation(), inv) == nil {
				t.Fatal("forged deferred row accepted")
			}
		})
	}
}

func TestInventoryCommandArguments(t *testing.T) {
	for name, args := range map[string][]string{
		"no-out":       {"inventory"},
		"with-expect":  {"inventory", "-expected", "x", "-out", "y"},
		"other-needs":  {"verify", "-out", "y"},
		"extra-arg":    {"inventory", "-out", "y", "z"},
		"unknown-flag": {"inventory", "-nope"},
	} {
		if code := run(args, io.Discard); code != 2 {
			t.Fatalf("%s: exit %d, want 2", name, code)
		}
	}
}
