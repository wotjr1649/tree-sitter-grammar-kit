package foundation

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// stepSyntax is one registered expectation's step and syntax class.
type stepSyntax struct {
	Step   int
	Syntax string
}

// cleanEditCase reports whether a registered case is a clean-tree edit case: it has at
// least one edit, every step from 0 through the last edit has an expectation, and every
// expectation is syntax NO_ERROR.
func cleanEditCase(edits int, expect []stepSyntax) bool {
	if edits == 0 || len(expect) == 0 {
		return false
	}
	covered := map[int]bool{}
	for _, e := range expect {
		if e.Syntax != "NO_ERROR" {
			return false
		}
		covered[e.Step] = true
	}
	for s := 0; s <= edits; s++ {
		if !covered[s] {
			return false
		}
	}
	return true
}

// routesWithoutCleanEditCase reads the route and gap case files (n461 excluded) of every
// route under root and returns the routes none of whose cases is a clean-tree edit case.
func routesWithoutCleanEditCase(t *testing.T, root string, routes []string) []string {
	t.Helper()
	var missing []string
	for _, r := range routes {
		found := false
		for _, kind := range []string{"routes", "gaps"} {
			data, err := os.ReadFile(filepath.Join(root, "src", "testdata", "native", kind, r+".json"))
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			var f caseFile
			if err := json.Unmarshal(data, &f); err != nil {
				t.Fatalf("%s/%s: %v", kind, r, err)
			}
			for _, c := range f.Cases {
				var expect []stepSyntax
				for _, e := range c.Expect {
					expect = append(expect, stepSyntax{e.Step, e.Syntax})
				}
				found = found || cleanEditCase(len(c.Edits), expect)
			}
		}
		if !found {
			missing = append(missing, r)
		}
	}
	return missing
}

// A route whose registered edit cases all sit on error trees could hide a real
// incremental route defect behind the error-tree BLOCKED rule. Every registered route
// therefore keeps, in its route or gap case files (n461 excluded), at least one edit case
// whose every step expects NO_ERROR: on those clean trees an unproven route is a FAIL.
func TestEveryRouteHasCleanEditCase(t *testing.T) {
	root := repository(t)
	var reg struct {
		Routes []struct{ Route string }
	}
	if err := json.Unmarshal(mustRead(t, filepath.Join(root, "src", "contracts", "native-routes.json")), &reg); err != nil {
		t.Fatal(err)
	}
	var routes []string
	for _, r := range reg.Routes {
		routes = append(routes, r.Route)
	}
	if len(routes) != 26 {
		t.Fatalf("registered routes %d, want 26", len(routes))
	}
	if missing := routesWithoutCleanEditCase(t, root, routes); len(missing) > 0 {
		t.Errorf("routes without a clean-tree edit case (an edit case expecting NO_ERROR on every step): %v", missing)
	}
}

// The check itself: an edit case counts only with NO_ERROR expected on every step, and a
// route whose only edit case has an error step (or whose clean case is in n461) is reported.
func TestCleanEditCaseCheck(t *testing.T) {
	for name, tc := range map[string]struct {
		edits  int
		expect []stepSyntax
		want   bool
	}{
		"clean":          {1, []stepSyntax{{0, "NO_ERROR"}, {1, "NO_ERROR"}}, true},
		"no-edit":        {0, []stepSyntax{{0, "NO_ERROR"}}, false},
		"error-step":     {1, []stepSyntax{{0, "ERROR"}, {1, "NO_ERROR"}}, false},
		"uncovered-step": {2, []stepSyntax{{0, "NO_ERROR"}, {2, "NO_ERROR"}}, false},
		"any-step":       {1, []stepSyntax{{0, "NO_ERROR"}, {1, "ANY"}}, false},
		"no-expectation": {1, nil, false},
	} {
		if got := cleanEditCase(tc.edits, tc.expect); got != tc.want {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
	root := t.TempDir()
	write := func(kind, route, body string) {
		p := filepath.Join(root, "src", "testdata", "native", kind, route+".json")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const errorOnly = `{"cases": [{"id": "e", "source_utf8": "x(", "edits": [{"find": "(", "replace": "()"}], "expect": [{"step": 0, "syntax": "ERROR"}, {"step": 1, "syntax": "NO_ERROR"}]}]}`
	const clean = `{"cases": [{"id": "c", "source_utf8": "x;", "edits": [{"find": ";", "replace": "; "}], "expect": [{"step": 0, "syntax": "NO_ERROR"}, {"step": 1, "syntax": "NO_ERROR"}]}]}`
	write("routes", "hidden", errorOnly)
	write("n461", "hidden", clean) // n461 cases do not count
	write("routes", "covered", errorOnly)
	write("gaps", "covered", clean)
	missing := routesWithoutCleanEditCase(t, root, []string{"hidden", "covered", "absent"})
	if len(missing) != 2 || missing[0] != "hidden" || missing[1] != "absent" {
		t.Fatalf("missing %v, want [hidden absent]", missing)
	}
}
