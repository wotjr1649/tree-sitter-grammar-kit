// Package foundation checks this repository's development contracts, not grammars.
package foundation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var required = []string{
	"go.mod", "AGENTS.md", "README.md", "LICENSE", "CHANGELOG.md", "SECURITY.md",
	".gitignore", ".gitattributes", ".github/workflows/foundation.yml",
	".github/ISSUE_TEMPLATE/task.md", ".github/PULL_REQUEST_TEMPLATE.md",
	"docs/README.md", "docs/roadmap.md", "docs/specs/scope.md",
	"docs/specs/cli-and-profile.md", "docs/specs/identity-and-evidence.md",
	"docs/specs/trust-and-execution.md", "docs/specs/tree-and-adapter-protocol.md",
	"docs/specs/platform-support.md", "docs/design/architecture.md",
	"docs/design/decisions/0001-core-and-execution.md",
	"docs/provenance/upstream-sources.md", "docs/validation/validation.md",
	"docs/validation/workload-matrix.md", "docs/reports/session-00-foundation.md",
	"src/internal/foundation/foundation.go", "src/internal/foundation/foundation_test.go",
	"src/contracts/examples/profile-r0.json",
}

var localDirs = []string{"docs/prompts", "docs/plans", "artifacts", "_ref", ".work", "bin", "dist", "coverage"}

func local(name string) bool {
	for _, dir := range localDirs {
		if name == dir || strings.HasPrefix(name, dir+"/") {
			return true
		}
	}
	return name == "go.work" || name == "go.work.sum"
}

// CheckFiles never invokes Git or searches a parent directory.
func CheckFiles(root fs.FS) error {
	var problems []error
	contents := make(map[string]string)
	for _, name := range required {
		data, err := fs.ReadFile(root, name)
		if err != nil {
			problems = append(problems, fmt.Errorf("required file %s: %w", name, err))
			continue
		}
		if len(data) == 0 || !utf8.Valid(data) || strings.Contains(string(data), "\r") {
			problems = append(problems, fmt.Errorf("expected nonempty UTF-8/LF file: %s", name))
		}
		contents[name] = string(data)
	}
	if !strings.HasPrefix(contents["go.mod"], "module github.com/wotjr1649/tree-sitter-grammar-kit\n") {
		problems = append(problems, errors.New("root module identity mismatch"))
	}
	agents := contents["AGENTS.md"]
	lines := 0
	for _, line := range strings.Split(agents, "\n") {
		if strings.TrimSpace(line) != "" {
			lines++
		}
	}
	if lines > 60 {
		problems = append(problems, errors.New("AGENTS exceeds 60 nonblank lines"))
	}
	for _, entry := range []string{"docs/prompts/", "docs/README.md", "handoff", "src/", "CGO_ENABLED=0", "PowerShell 7", "pwsh"} {
		if !strings.Contains(agents, entry) {
			problems = append(problems, fmt.Errorf("AGENTS entry missing: %s", entry))
		}
	}
	for _, dir := range localDirs {
		if !strings.Contains("\n"+contents[".gitignore"], "\n/"+dir+"/\n") {
			problems = append(problems, fmt.Errorf("root ignore missing: %s", dir))
		}
	}
	for _, file := range []string{"go.work", "go.work.sum"} {
		if !strings.Contains("\n"+contents[".gitignore"], "\n/"+file+"\n") {
			problems = append(problems, fmt.Errorf("root ignore missing: %s", file))
		}
	}
	for _, dir := range []string{"bytes", "archives", "malformed"} {
		if !strings.Contains(contents[".gitattributes"], "src/testdata/"+dir+"/** -text\n") {
			problems = append(problems, fmt.Errorf("byte preservation missing: %s", dir))
		}
	}
	// ponytail: plain inline file links only; use a Markdown parser if canonical syntax expands.
	links := regexp.MustCompile(`\[[^\]\n]*\]\(([^)\s]+)\)`)
	code := regexp.MustCompile("(?s)`{3}.*?`{3}|`[^`\n]*`")
	for name, data := range contents {
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		for _, match := range links.FindAllStringSubmatch(code.ReplaceAllString(data, ""), -1) {
			target := match[1]
			if strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "#") {
				continue
			}
			target, _, _ = strings.Cut(target, "#")
			resolved := path.Clean(path.Join(path.Dir(name), target))
			if strings.Contains(target, ":") || strings.Contains(target, "\\") || strings.HasPrefix(target, "/") || !fs.ValidPath(resolved) || local(resolved) {
				problems = append(problems, fmt.Errorf("noncanonical link in %s", name))
				continue
			}
			if _, err := fs.Stat(root, resolved); err != nil {
				problems = append(problems, fmt.Errorf("broken link in %s: %s", name, target))
			}
		}
	}
	err := fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == ".git" || local(name) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if name == "cmd" || name == "internal" || name == "pkg" || (path.Base(name) == "go.mod" && name != "go.mod") {
			problems = append(problems, fmt.Errorf("repository boundary violation: %s", name))
		}
		if strings.HasSuffix(name, ".go") && !strings.HasPrefix(name, "src/") {
			problems = append(problems, fmt.Errorf("Go source outside src: %s", name))
		}
		return nil
	})
	return errors.Join(append(problems, err)...)
}

func gitOutput(root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	return cmd.Output()
}

// CheckGit requires the explicit checkout's marker before any Git invocation.
func CheckGit(root string) error {
	marker, err := os.Lstat(filepath.Join(root, ".git"))
	if err != nil || marker.Mode()&os.ModeSymlink != 0 {
		return errors.New("explicit checkout .git marker required")
	}
	top, err := gitOutput(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("Git root query: %w", err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	want, err = filepath.Abs(want)
	if err != nil {
		return err
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	if err != nil {
		return err
	}
	if filepath.Clean(want) != filepath.Clean(got) {
		return errors.New("Git root differs from explicit checkout")
	}
	data, err := gitOutput(root, "ls-files", "-z")
	if err != nil {
		return fmt.Errorf("Git index query: %w", err)
	}
	tracked := make(map[string]bool)
	for _, name := range strings.Split(string(data), "\x00") {
		if local(name) {
			return fmt.Errorf("local material tracked: %s", name)
		}
		tracked[name] = true
	}
	for _, name := range required {
		if !tracked[name] {
			return fmt.Errorf("required file untracked: %s", name)
		}
	}
	for _, dir := range localDirs {
		if _, err := gitOutput(root, "check-ignore", "--no-index", "--quiet", "--", dir+"/foundation-probe"); err != nil {
			return fmt.Errorf("ignore ineffective: %s", dir)
		}
	}
	return nil
}
