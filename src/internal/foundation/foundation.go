// Package foundation checks this repository's development contracts, not grammars.
package foundation

import (
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
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
	"docs/specs/public-go-api.md", "docs/validation/language-feature-scope.md",
	"docs/validation/language-feature-disposition.md", "docs/validation/source-feature-feasibility.md",
	"docs/validation/p05-source-closure.md",
	"docs/validation/net461-workload.md",
	"docs/reports/campaign-01-2026-09-29-preparation.md",
	"docs/reports/issue-109-tsql-reclassification.md",
	"docs/reports/issue-128-tsql-corpus-gaps.md",
	"docs/reports/issue-131-tsql-overacceptance.md",
	"docs/reports/issue-132-security-policy-erratum.md",
	"docs/reports/issue-134-assembly-file-name-erratum.md",
	"docs/reports/issues-137-139-tsql-token-boundaries.md",
	"docs/reports/issue-141-tsql-context-and-hints.md",
	"docs/reports/issue-149-tsql-corpus-context.md",
	"docs/reports/issue-150-tsql-identifier-xml.md", "docs/reports/issue-151-tsql-exec-output.md",
	"docs/reports/issues-157-166-residual-syntax.md",
	"docs/reports/issue-113-tsql-boundaries.md",
	"docs/reports/issue-121-tsql-engine-validation.md", "docs/reports/issue-122-123-tsql-boundaries.md", "docs/reports/issue-126-expanded-validation.md",
	"docs/design/decisions/0001-core-and-execution.md",
	"docs/provenance/upstream-sources.md", "docs/validation/validation.md",
	"docs/validation/workload-matrix.md", "docs/reports/session-00-foundation.md",
	"src/internal/foundation/foundation.go", "src/internal/foundation/foundation_test.go",
	"src/internal/foundation/campaign_test.go",
	"src/contracts/examples/profile-r1.json", "src/contracts/examples/expected-r1.json",
	"src/contracts/campaign-01.json", "src/contracts/language-sources.json",
	"src/contracts/fact-mapping.json", "src/contracts/examples/tree-summary-r1.json",
	".github/workflows/prepare-p05.yml", "src/dev/prepare-p05/inputs.json",
	"src/dev/prepare-p05/acquire.ps1", "src/dev/prepare-p05/run.ps1",
	"src/dev/prepare-p05/collect.ps1", "src/dev/prepare-p05/probe.c.in",
	"src/dev/prepare-p05/case-review.json",
	"src/dev/prepare-p05/approval.ps1",
	"src/dev/prepare-p05/self-test.ps1",
	".github/workflows/prepare-p05-remedy.yml", "src/dev/prepare-p05/remedy.ps1",
	"src/dev/prepare-p05/remedy-patches.json", "src/dev/prepare-p05/remedy-cases.json",
	"src/dev/prepare-p05/remedy-fact-oracles.json", "src/dev/prepare-p05/remedy-sources.json",
	"src/dev/prepare-p05/remedy-r2.json",
	"src/dev/prepare-p05/remedy-csharp-r5.json",
	"src/dev/prepare-p05/remedy-tsql-r6.json",
	"src/dev/prepare-p05/remedy-expectation-amendments-r1.json",
	"src/kit/kit.go", "src/cmd/tsgk/main.go",
	"src/testdata/consumer/main.go", "src/testdata/consumer/go.mod.tmpl",
	"docs/reports/session-01-inventory-identity.md",
	"docs/reports/session-03-schema-contract.md",
	"go.sum", "docs/reports/session-04-reproducibility.md", "src/contracts/reproduction-routes.json",
	"src/dev/s04-reproduce/ci-owned.ps1", "src/testdata/reproduce/expected.json", "src/testdata/reproduce/src/grammar.json",
	"src/drivers/native-c/driver.c", "src/drivers/native-c/runtime-manifest.json", "src/contracts/native-routes.json",
	"src/drivers/native-c/runtime-field-lookup.patch.json",
	"src/drivers/native-c/runtime-navigation.patch.json",
	"src/contracts/native-large-fixtures.json", "src/dev/s05-native/prepare-routes.ps1", "src/dev/s05-native/run-routes.ps1",
	"src/dev/s05-native/select-compiler.ps1", "src/dev/s05-native/run-corpus.ps1", "docs/reports/session-05-incremental.md",
	"src/contracts/fact-query-pack.json", "docs/reports/session-06-native-oracle.md",
	"src/contracts/examples/replay-r1.json", "src/contracts/examples/evidence-policy-r1.json", "docs/reports/session-07-evidence-replay.md",
	"src/contracts/qualification-c1.json", "docs/reports/session-08-qualification.md", "src/dev/s08-qualify/run-identity.ps1", "src/dev/s08-qualify/gate.ps1",
	"src/contracts/feature-alternatives.json", "src/testdata/native/samples/NOTICE.md",
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
	var goFiles []string
	// Validate entries before reading required files; callers own an unchanged tree.
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
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("link or special entry forbidden: %s", name)
		}
		if name == "cmd" || name == "internal" || name == "pkg" || (path.Base(name) == "go.mod" && name != "go.mod") {
			problems = append(problems, fmt.Errorf("repository boundary violation: %s", name))
		}
		if strings.HasSuffix(name, ".go") {
			if !strings.HasPrefix(name, "src/") {
				problems = append(problems, fmt.Errorf("Go source outside src: %s", name))
			} else {
				goFiles = append(goFiles, name)
			}
		}
		return nil
	})
	if err := errors.Join(append(problems, err)...); err != nil {
		return err
	}
	// Inspect repository imports regardless of GOOS/build tags, without executing cgo.
	for _, name := range goFiles {
		data, err := fs.ReadFile(root, name)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, data, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("Go import syntax: %s", name)
		}
		for _, spec := range file.Imports {
			value, err := strconv.Unquote(spec.Path.Value)
			if err != nil || value == "C" || value == "github.com/wotjr1649/go-treesitter" || strings.HasPrefix(value, "github.com/wotjr1649/go-treesitter/") {
				return fmt.Errorf("forbidden core import: %s", name)
			}
		}
	}
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
	links := regexp.MustCompile(`(!?)\[[^\]\n]*\]\(([^)\s]+)\)`)
	code := regexp.MustCompile("(?s)`{3}.*?`{3}|`[^`\n]*`")
	for name, data := range contents {
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		docMap := false
		for _, match := range links.FindAllStringSubmatch(code.ReplaceAllString(data, ""), -1) {
			target := match[2]
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
			} else if resolved == "docs/README.md" && match[1] == "" {
				docMap = true
			}
		}
		if name == "AGENTS.md" && !docMap {
			problems = append(problems, errors.New("AGENTS requires a link to docs/README.md"))
		}
	}
	return errors.Join(problems...)
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
	data, err := gitOutput(root, "ls-files", "--stage", "-z")
	if err != nil {
		return fmt.Errorf("Git index query: %w", err)
	}
	if err := checkIndex(data); err != nil {
		return err
	}
	for _, dir := range localDirs {
		if _, err := gitOutput(root, "check-ignore", "--no-index", "--quiet", "--", dir+"/foundation-probe"); err != nil {
			return fmt.Errorf("ignore ineffective: %s", dir)
		}
	}
	return nil
}

func checkIndex(data []byte) error {
	tracked := make(map[string]bool)
	for _, record := range strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00") {
		header, name, ok := strings.Cut(record, "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") || fields[2] != "0" {
			return errors.New("index requires regular files at stage zero")
		}
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
	return nil
}
