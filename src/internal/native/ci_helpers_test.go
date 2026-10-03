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
	want, err := filepath.EvalSymlinks(cc)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{cc}
	// Same basename as the target: macOS /usr/bin/clang is an xcrun shim that picks the
	// tool by its invoked name, so a differently named link would not run clang at all.
	link := filepath.Join(t.TempDir(), filepath.Base(cc))
	if err := os.Symlink(cc, link); err == nil {
		paths = append(paths, link)
	} else {
		t.Logf("symlinks unavailable, linked case skipped: %v", err)
	}
	for _, p := range paths {
		out, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", script, "-Candidates", p).Output()
		if err != nil {
			t.Fatalf("%s: %v\n%s", p, err, out)
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		got := strings.TrimSpace(lines[len(lines)-1])
		same := got == want
		if runtime.GOOS == "windows" {
			same = strings.EqualFold(got, want)
		}
		if !same {
			t.Fatalf("%s: selected %q, want the resolved file %q", p, got, want)
		}
	}
}

// Static guards for lines no Windows run can reach: a native command piped into
// Select-Object -First (pipeline stopped early, $LASTEXITCODE unset or stale) in a CI
// helper, and a direct read of a root-only /proc/sys file in the workflow.
func TestCIScriptPatterns(t *testing.T) {
	cut := regexp.MustCompile(`&\s*\$\w+[^|\n]*\|\s*Select-Object\s+-First`)
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
	}
	wf, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "workflows", "foundation.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`Get-Content[^\n]*/proc/sys/`).Match(wf) {
		t.Error("workflow reads a root-only /proc/sys file as the runner user; use sudo sysctl -n")
	}
}
