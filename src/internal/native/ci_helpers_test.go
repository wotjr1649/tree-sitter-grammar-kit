package native

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// The CI route and sanitizer steps call select-compiler.ps1 first thing in a fresh pwsh,
// where $LASTEXITCODE is unset and StrictMode refuses to read it. Run it exactly so, with
// the configured compiler and with a link to it, and require the resolved file back.
func TestSelectCompilerFreshShell(t *testing.T) {
	_, cc := nativeTools(t)
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not available")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "dev", "s05-native", "select-compiler.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T, p, target string) {
		t.Helper()
		want, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", script, "-Candidates", p).Output()
		if err != nil {
			t.Fatalf("%s: %v\n%s", p, err, out)
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		got := strings.TrimSpace(lines[len(lines)-1])
		// The selected path must name the target file itself, not a link to it. Compare by
		// file identity: a directory link earlier in the path (an Xcode.app alias) may be
		// kept by PowerShell and resolved by Go, which is the same file either way.
		fi, err := os.Lstat(got)
		if err != nil {
			t.Fatalf("%s: selected %q: %v", p, got, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 || !os.SameFile(fi, want) {
			t.Fatalf("%s: selected %q, want the resolved file %s", p, got, target)
		}
	}
	t.Run("regular", func(t *testing.T) { check(t, cc, cc) })

	// The linked case needs a real compiler binary as its target. macOS /usr/bin/clang is
	// an xcrun shim that dispatches on how and where it is invoked, and shims are not
	// supported as links (platform-support), so on darwin link the clang binary that
	// xcrun runs from the active developer directory instead.
	t.Run("linked", func(t *testing.T) {
		target := cc
		if runtime.GOOS == "darwin" {
			out, err := exec.Command("xcrun", "--find", "clang").Output()
			if err != nil {
				t.Skipf("darwin linked case skipped: xcrun --find clang: %v", err)
			}
			target = strings.TrimSpace(string(out))
		}
		link := filepath.Join(t.TempDir(), filepath.Base(target))
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks unavailable, linked case skipped: %v", err)
		}
		check(t, link, target)
	})
}

// Static guards for lines no Windows run can reach: a native command piped into
// Select-Object -First (pipeline stopped early, $LASTEXITCODE unset or stale) in a CI
// helper, an array built inside $(if ...) without the unary comma (the subexpression
// unrolls a one-element array to a scalar and an empty one to $null), and a direct read
// of a root-only /proc/sys file in the workflow.
func TestCIScriptPatterns(t *testing.T) {
	cut := regexp.MustCompile(`&\s*\$\w+[^|\n]*\|\s*Select-Object\s+-First`)
	unroll := regexp.MustCompile(`\$\(\s*if\b[^\n]*\{\s*@\(`)
	files, _ := filepath.Glob(filepath.Join("..", "..", "dev", "s05-native", "*.ps1"))
	if len(files) == 0 {
		t.Fatal("no CI helper scripts found")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if m := cut.Find(data); m != nil {
			t.Errorf("%s pipes a native command into Select-Object -First: %s", f, m)
		}
		if m := unroll.Find(data); m != nil {
			t.Errorf("%s builds an array inside $(if ...) without ,@(...): %s", f, m)
		}
	}
	wf, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "workflows", "foundation.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`Get-Content[^\n]*/proc/sys/`).Match(wf) {
		t.Error("workflow reads a root-only /proc/sys file as the runner user; use sudo sysctl -n")
	}
}
