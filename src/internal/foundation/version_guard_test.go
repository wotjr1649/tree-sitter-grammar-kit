package foundation

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoHardcodedToolVersions keeps tool upgrades from blocking finished verification (#360).
// The Go release comes only from go.mod and history is accepted by format, so a Go release
// literal outside tests is a new pin that a bump would have to chase. PowerShell is checked
// as a lower bound: an exact major comparison would fail every job on the next major.
func TestNoHardcodedToolVersions(t *testing.T) {
	root := repository(t)
	goRelease := regexp.MustCompile(`\bgo1\.[0-9]+\.[0-9]+\b`)
	exactPwsh := regexp.MustCompile(`PSVersion\.Major\s*-(?:ne|eq|gt|ge|le)\b`)
	checked := 0
	for _, dir := range []string{"src", ".github"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
			if d.IsDir() {
				if rel == "src/testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".ps1", ".py", ".yml", ".yaml":
			default:
				return nil
			}
			if rel == "src/internal/foundation/version_guard_test.go" { // holds the patterns as self-check samples
				return nil
			}
			text := string(mustRead(t, path))
			checked++
			if loc := exactPwsh.FindStringIndex(text); loc != nil {
				t.Errorf("%s: exact PowerShell major check %q; use -lt 7", rel, text[loc[0]:loc[1]])
			}
			// Tests use synthetic releases to exercise the rules; only shipped code and workflows count.
			if !strings.HasSuffix(path, "_test.go") {
				if m := goRelease.FindString(text); m != "" {
					t.Errorf("%s: Go release literal %q; read it from go.mod", rel, m)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked == 0 {
		t.Fatal("no files scanned")
	}
	// The guard must notice the patterns it exists for.
	for _, bad := range []string{`if ($PSVersionTable.PSVersion.Major -ne 7) {}`, `$PSVersionTable.PSVersion.Major -eq 7`} {
		if !exactPwsh.MatchString(bad) {
			t.Fatalf("guard misses %q", bad)
		}
	}
	if exactPwsh.MatchString(`$PSVersionTable.PSVersion.Major -lt 7`) || !goRelease.MatchString(`"go1.27.4"`) || goRelease.MatchString(`go 1.27.4`) {
		t.Fatal("guard patterns drifted")
	}
}
