package foundation

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// Anchors are written in case files as {type, text, occurrence} and converted to byte
// ranges twice: by run-routes.ps1 (Convert-Cases, into the profiles the hosts run) and by
// the inventory generator (convertCase, into qualification-c1.json). qualify compares the
// two, so a disagreement fails every anchored case; these tests keep them equal.

// anchorFixture is a synthetic case file with hand-computed ranges: a multi-byte prefix,
// a default and a second (overlapping) occurrence, and anchors on an edited step whose
// offsets the edit shifted.
const anchorFixture = `{"schema": "tsgk-native-route-cases/r1", "route": "fixture", "cases": [
 {"id": "anchor-fixture", "kind": "feature", "features": [], "source_utf8": "é = aaa;\ncall(x);\ncall(y);\n",
  "edits": [{"find": "é", "replace": "ab"}],
  "expect": [
   {"step": 0, "syntax": "NO_ERROR", "contains": ["call"], "anchors": [
     {"type": "call", "text": "call(y)", "occurrence": 1},
     {"type": "run", "text": "aa", "occurrence": 2},
     {"type": "first", "text": "call"}]},
   {"step": 1, "syntax": "NO_ERROR", "contains": [], "anchors": [{"type": "call", "text": "call(y)"}]},
   {"step": 1, "syntax": "ANY", "contains": []}]}]}`

// anchorFixtureWant are the fixture's ranges: "é" is two bytes, so call(y) starts at 19 in
// step 0 and stays at 19 after "é" -> "ab" (same length); "aa" occurs at 5 and 6.
var anchorFixtureWant = [][]kit.ExpectAnchor{
	{{Type: "call", StartByte: 19, EndByte: 26}, {Type: "run", StartByte: 6, EndByte: 8}, {Type: "first", StartByte: 10, EndByte: 14}},
	{{Type: "call", StartByte: 19, EndByte: 26}},
	nil,
}

// referenceAnchors is the documented conversion written a second time, over strings: apply
// each edit at the first occurrence of its find text (an empty find appends), then take the
// occurrence-th offset (counting every offset) where the anchor text starts.
func referenceAnchors(src string, edits []struct{ Find, Replace string }, step int, as []anchorSource) ([]kit.ExpectAnchor, error) {
	versions := []string{src}
	for _, e := range edits {
		cur := versions[len(versions)-1]
		at := len(cur)
		if e.Find != "" {
			at = strings.Index(cur, e.Find)
		}
		if at < 0 {
			return nil, fmt.Errorf("edit target %q not found", e.Find)
		}
		versions = append(versions, cur[:at]+e.Replace+cur[at+len(e.Find):])
	}
	if step < 0 || step >= len(versions) {
		return nil, fmt.Errorf("step %d", step)
	}
	s := versions[step]
	var out []kit.ExpectAnchor
	for _, a := range as {
		occ := 1
		if a.Occurrence != nil {
			occ = *a.Occurrence
		}
		found, seen := -1, 0
		for i := 0; a.Type != "" && a.Text != "" && i+len(a.Text) <= len(s); i++ {
			if strings.HasPrefix(s[i:], a.Text) {
				if seen++; seen == occ {
					found = i
					break
				}
			}
		}
		if found < 0 {
			return nil, fmt.Errorf("anchor %s %q occurrence %d not found", a.Type, a.Text, occ)
		}
		out = append(out, kit.ExpectAnchor{Type: a.Type, StartByte: uint32(found), EndByte: uint32(found + len(a.Text))})
	}
	return out, nil
}

// convertedCase is the part of a Convert-Cases case the comparison reads.
type convertedCase struct {
	ID     string                `json:"id"`
	Edits  []kit.Edit            `json:"edits"`
	Expect []kit.StepExpectation `json:"expect"`
}

// psConvert is the pwsh harness: it defines run-routes.ps1's conversion functions from the
// script's own AST (the script itself runs a whole route build) and prints Convert-Cases.
const psConvert = `param([string]$Script, [string]$CaseFile, [string]$Root)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$utf8 = [Text.UTF8Encoding]::new($false, $true)
$ast = [Management.Automation.Language.Parser]::ParseFile($Script, [ref]$null, [ref]$null)
$defs = $ast.FindAll({ param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] }, $true)
foreach ($f in $defs) {
  if ($f.Name -in @('Get-ShaBytes', 'Find-Bytes', 'Resolve-Anchor', 'Convert-Cases')) { . ([scriptblock]::Create($f.Extent.Text)) }
}
ConvertTo-Json -InputObject @(Convert-Cases $CaseFile $Root) -Depth 12 -Compress
`

// runConvertCases runs run-routes.ps1's Convert-Cases on a case file; ok is false when pwsh
// is not installed.
func runConvertCases(t *testing.T, root, caseFile string) ([]convertedCase, bool, error) {
	t.Helper()
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		return nil, false, nil
	}
	dir := t.TempDir()
	harness := filepath.Join(dir, "convert.ps1")
	if err := os.WriteFile(harness, []byte(psConvert), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "src", "dev", "s05-native", "run-routes.ps1")
	out, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harness, "-Script", script, "-CaseFile", caseFile, "-Root", filepath.Join(dir, "root")).CombinedOutput()
	if err != nil {
		return nil, true, fmt.Errorf("%v: %s", err, out)
	}
	var cs []convertedCase
	if err := json.Unmarshal(out, &cs); err != nil {
		t.Fatalf("Convert-Cases output: %v\n%s", err, out)
	}
	return cs, true, nil
}

// TestAnchorConversion: the fixture's hand-computed ranges are what the inventory
// generator, the reference conversion and run-routes.ps1 produce; an anchor text that is
// not in its step's source, an empty text and occurrence 0 are refused by all three.
func TestAnchorConversion(t *testing.T) {
	root := repository(t)
	var f caseFile
	if err := json.Unmarshal([]byte(anchorFixture), &f); err != nil {
		t.Fatal(err)
	}
	sc := loadCases(f)[0]
	c, err := convertCase(sc.id, sc.src, sc.edits, sc.expect, sc.anchors)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range c.Expect {
		if !slices.Equal(e.Anchors, anchorFixtureWant[i]) {
			t.Fatalf("generator expect %d anchors %v, want %v", i, e.Anchors, anchorFixtureWant[i])
		}
		ref, err := referenceAnchors(sc.src, sc.edits, e.Step, sc.anchors[i])
		if err != nil || !slices.Equal(ref, anchorFixtureWant[i]) {
			t.Fatalf("reference expect %d anchors %v (%v), want %v", i, ref, err, anchorFixtureWant[i])
		}
	}
	file := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(file, []byte(anchorFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	ps, ok, err := runConvertCases(t, root, file)
	if !ok {
		t.Log("pwsh not available: run-routes.ps1 conversion not executed (TestRunRoutesAnchorGuard still applies)")
	} else if err != nil {
		t.Fatal(err)
	} else {
		if len(ps) != 1 || len(ps[0].Expect) != len(anchorFixtureWant) {
			t.Fatalf("Convert-Cases: %+v", ps)
		}
		for i, e := range ps[0].Expect {
			if !slices.Equal(e.Anchors, anchorFixtureWant[i]) {
				t.Fatalf("run-routes.ps1 expect %d anchors %v, want %v", i, e.Anchors, anchorFixtureWant[i])
			}
		}
		if !slices.EqualFunc(ps[0].Edits, c.Edits, func(a, b kit.Edit) bool { return fmt.Sprint(a) == fmt.Sprint(b) }) {
			t.Fatalf("run-routes.ps1 edits %v, generator %v", ps[0].Edits, c.Edits)
		}
	}
	for name, bad := range map[string]string{
		"not-found":    `{"type": "call", "text": "call(z)"}`,
		"third":        `{"type": "call", "text": "call(", "occurrence": 3}`,
		"empty-text":   `{"type": "call", "text": ""}`,
		"occurrence-0": `{"type": "call", "text": "call", "occurrence": 0}`,
		// #83: a member that is not exactly type, text or occurrence, or an occurrence that
		// is not an integer, never falls back to the default occurrence 1
		"occurrence-null": `{"type": "call", "text": "call", "occurrence": null}`,
		"occurrence-real": `{"type": "call", "text": "call", "occurrence": 1.5}`,
		"occurrence-text": `{"type": "call", "text": "call", "occurrence": "2"}`,
		"misspelt-member": `{"type": "call", "text": "call", "occurence": 2}`,
		"other-case-name": `{"type": "call", "Text": "call"}`,
		"missing-type":    `{"text": "call"}`,
		"empty-type":      `{"type": "", "text": "call"}`,
		"text-number":     `{"type": "call", "text": 5}`,
		"type-number":     `{"type": 5, "text": "call"}`,
	} {
		doc := strings.Replace(anchorFixture, `{"type": "first", "text": "call"}`, bad, 1)
		// a decoding error is already the generator's refusal (buildInventory stops on it)
		var bf caseFile
		if err := json.Unmarshal([]byte(doc), &bf); err == nil {
			b := loadCases(bf)[0]
			if _, err := convertCase(b.id, b.src, b.edits, b.expect, b.anchors); err == nil {
				t.Errorf("%s: generator accepted the anchor", name)
			}
			if _, err := referenceAnchors(b.src, b.edits, b.expect[0].Step, b.anchors[0]); err == nil {
				t.Errorf("%s: reference accepted the anchor", name)
			}
		}
		if ok {
			p := filepath.Join(t.TempDir(), "bad.json")
			if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, _, err := runConvertCases(t, root, p); err == nil {
				t.Errorf("%s: run-routes.ps1 accepted the anchor", name)
			}
		}
	}
}

// TestRegisteredAnchors: every anchor of a registered case is found in its step's source,
// and the inventory carries exactly the ranges the reference conversion computes (and, when
// pwsh is installed, the ones run-routes.ps1 writes into the profile).
func TestRegisteredAnchors(t *testing.T) {
	root := repository(t)
	inv, err := kit.ParseQualificationInventory(mustRead(t, filepath.Join(root, filepath.FromSlash(inventoryPath))))
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string][]kit.QualCase{}
	add := func(w kit.QualWorkload) {
		for _, c := range w.Cases {
			registered[c.ID] = append(registered[c.ID], c)
		}
	}
	for _, r := range inv.Routes {
		add(r.Workload)
	}
	for _, x := range inv.ExtraRoles {
		for _, w := range x.Workloads {
			add(w)
		}
	}
	// a query case is its source case with its own id
	ids := map[string][]string{}
	entries, err := filepath.Glob(filepath.Join(root, "src", "testdata", "native", "queries", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range entries {
		var qf queryFile
		if err := json.Unmarshal(mustRead(t, p), &qf); err != nil {
			t.Fatal(err)
		}
		for _, q := range qf.Cases {
			ids[q.SourceCase] = append(ids[q.SourceCase], q.ID)
		}
	}
	anchored := 0
	for _, kind := range []string{"routes", "gaps", "n461"} {
		files, err := filepath.Glob(filepath.Join(root, "src", "testdata", "native", kind, "*.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range files {
			var f caseFile
			if err := json.Unmarshal(mustRead(t, p), &f); err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			has := false
			for _, sc := range loadCases(f) {
				want := make([][]kit.ExpectAnchor, len(sc.expect))
				for i, e := range sc.expect {
					if len(sc.anchors[i]) == 0 {
						continue
					}
					has = true
					if want[i], err = referenceAnchors(sc.src, sc.edits, e.Step, sc.anchors[i]); err != nil {
						t.Fatalf("%s %s expect %d: %v", p, sc.id, i, err)
					}
				}
				for _, id := range append([]string{sc.id}, ids[sc.id]...) {
					cs := registered[id]
					if len(cs) == 0 {
						t.Fatalf("%s: case %s not in the inventory", p, id)
					}
					for _, c := range cs {
						for i, e := range c.Expect {
							if !slices.Equal(e.Anchors, want[i]) {
								t.Fatalf("%s: inventory expect %d anchors %v, reference %v", id, i, e.Anchors, want[i])
							}
							if want[i] != nil {
								anchored++
							}
						}
					}
				}
			}
			if !has {
				continue
			}
			ps, ok, err := runConvertCases(t, root, p)
			if !ok {
				continue
			}
			if err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			for _, c := range ps {
				reg := registered[c.ID]
				for i, e := range c.Expect {
					if !slices.Equal(e.Anchors, reg[0].Expect[i].Anchors) {
						t.Fatalf("%s: run-routes.ps1 expect %d anchors %v, inventory %v", c.ID, i, e.Anchors, reg[0].Expect[i].Anchors)
					}
				}
			}
		}
	}
	t.Logf("anchored registered expectations: %d", anchored)
}

// TestRunRoutesAnchorGuard is the static half for hosts without pwsh: run-routes.ps1
// writes the profile revisions the kit decodes (r2 takes anchors) and converts anchors in
// Convert-Cases through Resolve-Anchor; run-corpus.ps1 writes the same incremental revision.
func TestRunRoutesAnchorGuard(t *testing.T) {
	root := repository(t)
	routes := string(mustRead(t, filepath.Join(root, "src", "dev", "s05-native", "run-routes.ps1")))
	corpus := string(mustRead(t, filepath.Join(root, "src", "dev", "s05-native", "run-corpus.ps1")))
	for _, want := range []string{"schema = '" + kit.IncrementalSchema + "'", "schema = '" + kit.OracleSchema + "'", "function Resolve-Anchor(", "Resolve-Anchor $src $a $c.id"} {
		if !strings.Contains(routes, want) {
			t.Errorf("run-routes.ps1 lacks %q", want)
		}
	}
	if !strings.Contains(corpus, "schema = '"+kit.IncrementalSchema+"'") {
		t.Errorf("run-corpus.ps1 does not write %s", kit.IncrementalSchema)
	}
	convert := routes[strings.Index(routes, "function Convert-Cases("):]
	convert = convert[:strings.Index(convert, "\n}\n")]
	if !strings.Contains(convert, "$x.anchors = @(") || !strings.Contains(convert, "$versions.Add($cur)") {
		t.Error("Convert-Cases does not convert anchors on each step's source as an array")
	}
}
