package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// Platform neutrality of the ordinary inputs (#427 P1, plan section 3). The independent expectations of the
// ordinary tuples are host-neutral only if the inputs are: the same bytes on both hosts (line endings, encoding)
// and no scanner whose answer on a non-ASCII input depends on the host's C library. These two tests pin what the
// inputs are today, so that a change has to be looked at.

// neutralityWant is the per-route shape of the derived source versions: how many there are, how many carry a byte
// above 0x7f and how many carry a CR. A route with non-ASCII input is exposed to a host-dependent scanner (see
// TestOrdinaryScannerLibcExposure); a version with a CR is exposed to any line-ending conversion.
var neutralityWant = map[string][3]int{
	"bash": {37, 0, 0}, "c": {64, 1, 0}, "cpp": {80, 1, 0}, "csharp": {407, 30, 3}, "css": {241, 1, 1},
	"dart": {58, 0, 2}, "go": {71, 27, 2}, "html": {36, 3, 1}, "java": {47, 1, 1}, "javascript": {71, 5, 2},
	"json": {34, 1, 2}, "jsx": {67, 5, 2}, "kotlin": {45, 1, 1}, "php": {59, 1, 1}, "postgresql-sql": {559, 0, 0},
	"powershell": {45, 2, 1}, "python": {60, 1, 1}, "r": {24, 0, 0}, "ruby": {38, 3, 0}, "rust": {37, 2, 1},
	"swift": {95, 2, 1}, "tsql": {1534, 44, 16}, "tsx": {80, 1, 0}, "typescript": {89, 2, 0}, "xml": {48, 5, 2},
	"yaml": {37, 7, 1},
}

// rawOrigins are the only origin files of ordinary sources that are not JSON: a JSON file carries a CR or a
// non-ASCII character as an escape or as UTF-8 text inside a string, which no checkout converts, while a raw
// source file is the bytes themselves and would change under a line-ending conversion.
var rawOrigins = []string{"src/testdata/native/dynamic-sql/csharp-command.cs", "src/testdata/native/dynamic-sql/tsql-dynamic.sql"}

func TestOrdinarySourcesAreByteIdenticalOnBothHosts(t *testing.T) {
	in := testInputs(t)
	out, err := build(in)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][3]int{}
	for _, r := range out.Routes {
		var row [3]int
		row[0] = len(r.Versions)
		for _, v := range r.Versions {
			var nonASCII, cr bool
			for _, b := range v.Source {
				nonASCII, cr = nonASCII || b >= 0x80, cr || b == '\r'
			}
			if nonASCII {
				row[1]++
			}
			if cr {
				row[2]++
			}
		}
		got[r.Route] = row
	}
	for route, want := range neutralityWant {
		if got[route] != want {
			t.Errorf("route %s: versions, non-ASCII, CR = %v, pinned %v", route, got[route], want)
		}
	}
	if len(got) != len(neutralityWant) {
		t.Errorf("routes %d, pinned %d", len(got), len(neutralityWant))
	}
	// where the bytes come from: JSON files, and two raw files that carry no CR for a line-ending conversion to change
	origins := map[string]bool{}
	for _, r := range out.Routes {
		for _, c := range r.Cases {
			for _, o := range c.Origins {
				origins[o.File.Path] = true
			}
		}
	}
	var raw []string
	for path := range origins {
		if filepath.Ext(path) != ".json" {
			raw = append(raw, path)
		}
	}
	sort.Strings(raw)
	if !slices.Equal(raw, rawOrigins) {
		t.Fatalf("raw origin files %v, pinned %v", raw, rawOrigins)
	}
	for _, path := range raw {
		data, err := in.read(path)
		if err != nil || strings.Contains(string(data), "\r") {
			t.Errorf("%s: a raw origin must hold no CR (err %v)", path, err)
		}
	}
}

// libcClasses are the C library calls whose answer depends on the host's locale or wide-character tables: the
// narrow and wide classification and case functions, the multibyte conversions and setlocale itself.
var libcClasses = regexp.MustCompile(`\b(isw[a-z]+|tow(?:upper|lower)|is(?:alpha|alnum|digit|space|upper|lower|punct|xdigit|print|graph|cntrl|blank)|to(?:upper|lower)|wcs[a-z]+|wcw[a-z]+|wcr[a-z]+|mbr[a-z]+|setlocale)\s*\(`)

var cComments = regexp.MustCompile(`(?s)/\*.*?\*/|//[^\n]*`)

// scannerLibcCalls are the distinct libc classification calls in the hand-written (not generated) C and C++ files
// of a prepared grammar: its scanner and the shared headers it includes.
func scannerLibcCalls(t *testing.T, routeDir string) []string {
	t.Helper()
	var files []string
	for _, pattern := range []string{"src/*.c", "src/*.cc", "src/*.h", "common/*.c", "common/*.cc", "common/*.h"} {
		matches, err := filepath.Glob(filepath.Join(routeDir, filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, matches...)
	}
	seen := map[string]bool{}
	for _, file := range files {
		if filepath.Base(file) == "parser.c" { // generated tables, no scanner code
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range libcClasses.FindAllStringSubmatch(cComments.ReplaceAllString(string(data), " "), -1) {
			seen[m[1]] = true
		}
	}
	var calls []string
	for call := range seen {
		calls = append(calls, call)
	}
	sort.Strings(calls)
	return calls
}

// libcScannerRoutes are the routes whose prepared scanner calls a locale-dependent libc function, and
// selectedRoutes the routes whose shards P1 collects: none of them may be among the former.
var (
	libcScannerRoutes = []string{"bash", "cpp", "csharp", "css", "dart", "html", "javascript", "jsx", "kotlin", "php", "powershell", "r", "ruby", "rust", "swift", "tsx", "typescript"}
	// libcExposedRoutes are those of them that also have non-ASCII ordinary input.
	libcExposedRoutes = []string{"cpp", "csharp", "css", "html", "javascript", "jsx", "kotlin", "php", "powershell", "ruby", "rust", "swift", "tsx", "typescript"}
	selectedRoutes    = []string{"json", "postgresql-sql", "tsql"}
)

// TestOrdinaryScannerLibcExposure names the routes whose scanner asks the host's C library about a character and
// that have non-ASCII ordinary input: on those, a tuple's expected value rests on one libc's answer, so it is not
// host-neutral evidence. The shards P1 collects (JSON, PostgreSQL, TSQL) are outside the set; the rest are listed,
// and a prepared grammar whose scanner changes the list fails here. It reads the prepared grammar closures that the
// CI preparation hands to the route jobs (TSGK_ORDINARY_GRAMMAR_ROOT) and skips without them.
func TestOrdinaryScannerLibcExposure(t *testing.T) {
	grammars := os.Getenv("TSGK_ORDINARY_GRAMMAR_ROOT")
	if grammars == "" {
		t.Skip("set TSGK_ORDINARY_GRAMMAR_ROOT to the prepared grammar routes directory")
	}
	out, err := build(testInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	var libc, exposed []string
	for _, r := range out.Routes {
		calls := scannerLibcCalls(t, filepath.Join(grammars, r.Route))
		if len(calls) == 0 {
			continue
		}
		libc = append(libc, r.Route)
		for _, v := range r.Versions {
			if slices.ContainsFunc(v.Source, func(b byte) bool { return b >= 0x80 }) {
				exposed = append(exposed, r.Route)
				break
			}
		}
		t.Logf("route %s: scanner calls %v", r.Route, calls)
	}
	sort.Strings(libc)
	if !slices.Equal(libc, libcScannerRoutes) {
		t.Errorf("routes with a libc-dependent scanner: %v, pinned %v", libc, libcScannerRoutes)
	}
	for _, route := range selectedRoutes {
		if slices.Contains(libc, route) {
			t.Errorf("selected route %s has a libc-dependent scanner: its collected shards are not host-neutral", route)
		}
	}
	sort.Strings(exposed)
	if !slices.Equal(exposed, libcExposedRoutes) {
		t.Errorf("routes whose non-ASCII ordinary input meets a libc-dependent scanner: %v, pinned %v", exposed, libcExposedRoutes)
	}
	t.Logf("not host-neutral evidence until a host-neutral expectation exists: %s", fmt.Sprint(exposed))
}

// The scan sees a call in a scanner and its shared header, ignores comments and the generated parser, and an
// all-ASCII scanner (like the TSQL one after #171) is clean.
func TestScannerLibcCallsReadOnlyHandWrittenCode(t *testing.T) {
	dir := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/parser.c", "int x = iswalpha(c);\n")
	write("src/scanner.c", "// iswupper(c) in a comment\n/* tolower(c) */\nbool f(int32_t c) { return c < 128 && is_ascii_alpha(c); }\n")
	if calls := scannerLibcCalls(t, dir); len(calls) != 0 {
		t.Fatalf("a clean scanner reported %v", calls)
	}
	write("common/scanner.h", "static bool g(int32_t c) { return iswspace (c); }\n")
	write("src/scanner.c", "bool f(int32_t c) { return iswalnum(c) || towlower(c) == c; }\n")
	if calls := scannerLibcCalls(t, dir); !slices.Equal(calls, []string{"iswalnum", "iswspace", "towlower"}) {
		t.Fatalf("calls %v", calls)
	}
}
