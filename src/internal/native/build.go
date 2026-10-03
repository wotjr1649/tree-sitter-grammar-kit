// Package native builds the generic tsgk-native/r1 driver with an external compiler and
// runs it through the S04 runner. It is CLI-side: the public offline API never builds or
// starts a process. Contract: docs/specs/tree-and-adapter-protocol.md "S05 구현".
package native

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// BuildSchema names the canonical build closure text hashed into the build identity.
const BuildSchema = "tsgk-native-build/r1"

// c-build operation limits (resource-budget r10).
const (
	buildWall   = 120 * time.Second
	buildOutput = 8388608
	buildMemory = 4294967296
	buildGrace  = 2 * time.Second
)

// Error is a refusal before any launch or a build/run failure with a stable code.
type Error struct {
	Kind, Code string
	Cause      error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Kind + ": " + e.Code + ": " + e.Cause.Error()
	}
	return e.Kind + ": " + e.Code
}

func (e *Error) Unwrap() error { return e.Cause }

func refuse(kind, code string, cause error) *Error {
	return &Error{Kind: kind, Code: code, Cause: cause}
}

// RuntimeManifest is the pinned runtime source closure (src/drivers/native-c).
type RuntimeManifest struct {
	Schema     string `json:"schema"`
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	URL        string `json:"url"`
	Note       string `json:"note"`
	Files      []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Bytes  int64  `json:"bytes"`
	} `json:"files"`
}

// LoadRuntimeManifest returns the embedded runtime manifest.
func LoadRuntimeManifest() (RuntimeManifest, error) {
	var m RuntimeManifest
	data, err := drivers.NativeC.ReadFile("native-c/runtime-manifest.json")
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(data, &m)
	return m, err
}

// BuildRequest describes one driver build. Paths are absolute; Work is an existing
// caller-owned directory where a new build directory is created.
type BuildRequest struct {
	Work         string
	Runtime      string // runtime root holding the manifest files (LICENSE, lib/...)
	GrammarRoot  string
	Grammar      []kit.NativeInput
	Symbol       string
	Compiler     string
	CompilerID   kit.ToolIdentity
	Defines      []string // owned fault controls for the kit's tests only; recorded in the closure
	Sanitize     bool     // diagnostic build with AddressSanitizer/UBSan (gcc/clang on Linux and macOS)
	CgroupParent string
}

// ClosureFile is one input of the build closure.
type ClosureFile struct {
	Role   string `json:"role"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// BuildStep is one compiler process.
type BuildStep struct {
	Name     string        `json:"name"`
	Argv     []string      `json:"argv"`
	Result   runner.Result `json:"result"`
	Stderr   string        `json:"stderr,omitempty"`
	Executed bool          `json:"executed"`
}

// Build is a completed driver build.
type Build struct {
	Dir              string           `json:"-"`
	Executable       string           `json:"-"`
	Identity         string           `json:"identity"`
	ExecutableSHA256 string           `json:"executable_sha256"`
	ExecutableBytes  int64            `json:"executable_bytes"`
	Compiler         kit.ToolIdentity `json:"compiler"`
	CompilerVersion  string           `json:"compiler_version"`
	Runtime          string           `json:"runtime_commit"`
	Symbol           string           `json:"symbol"`
	Defines          []string         `json:"defines"`
	Sanitize         bool             `json:"sanitize"`
	EnvNames         []string         `json:"env_names"`
	Closure          []ClosureFile    `json:"closure"`
	Steps            []BuildStep      `json:"steps"`
	env              []string
	cgroup           string
}

func fileDigest(p string) (string, int64, error) {
	info, err := os.Lstat(p)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, errors.New("not a regular file")
	}
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// copyVerified copies root/rel to dst after rejecting links in every component and
// checking the declared identity of the bytes actually copied.
func copyVerified(root, rel, dst, sha string, size int64) error {
	cur := root
	for _, seg := range strings.Split(rel, "/") {
		cur = filepath.Join(cur, seg)
		info, err := os.Lstat(cur)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (info.Mode()&os.ModeType != 0 && !info.IsDir()) {
			return errors.New("link or special file in path")
		}
	}
	data, err := os.ReadFile(cur)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != sha || int64(len(data)) != size {
		return errors.New("identity mismatch")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// buildEnv is the exact compiler environment: PATH limited to the compiler directory
// (and the system tool directories on Unix), a temp directory inside the build, and the
// named host variables a toolchain needs to find its own SDK. Values are not secrets;
// only names are reported.
func buildEnv(compiler, tmp string) []string {
	pathList := []string{filepath.Dir(compiler)}
	if runtime.GOOS != "windows" {
		pathList = append(pathList, "/usr/bin", "/bin")
	}
	env := []string{"PATH=" + strings.Join(pathList, string(os.PathListSeparator)), "TMP=" + tmp, "TEMP=" + tmp, "TMPDIR=" + tmp}
	for _, name := range []string{"SystemRoot", "WINDIR", "ProgramData", "ProgramFiles", "ProgramFiles(x86)", "PROCESSOR_ARCHITECTURE", "DEVELOPER_DIR", "SDKROOT"} {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

func exeName() string {
	if runtime.GOOS == "windows" {
		return "tsgk-native-driver.exe"
	}
	return "tsgk-native-driver"
}

// NewBuild verifies every input, materializes a new build directory and compiles the
// driver. A failed build keeps its steps and is returned with the error.
func NewBuild(ctx context.Context, req BuildRequest) (*Build, error) {
	if !kit.ValidLanguageSymbol(req.Symbol) {
		return nil, refuse(kit.KindInvalidInput, "SYMBOL_INVALID", nil)
	}
	for _, d := range req.Defines {
		if !strings.HasPrefix(d, "TSGK_") || strings.ContainsFunc(d, func(r rune) bool { return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') }) {
			return nil, refuse(kit.KindInvalidInput, "DEFINE_INVALID", nil)
		}
	}
	for _, p := range []string{req.Work, req.Runtime, req.GrammarRoot, req.Compiler} {
		if !filepath.IsAbs(p) {
			return nil, refuse(kit.KindInvalidInput, "PATH_NOT_ABSOLUTE", nil)
		}
	}
	sum, size, err := fileDigest(req.Compiler)
	if err != nil {
		return nil, refuse(kit.KindUnsupported, "TOOL_MISSING", err)
	}
	if sum != req.CompilerID.SHA256 || uint64(size) != req.CompilerID.Bytes {
		return nil, refuse(kit.KindInvalidInput, "TOOL_IDENTITY_MISMATCH", fmt.Errorf("compiler %s %d bytes", sum, size))
	}
	man, err := LoadRuntimeManifest()
	if err != nil {
		return nil, refuse(kit.KindIO, "RUNTIME_MANIFEST_INVALID", err)
	}
	dir, err := os.MkdirTemp(req.Work, "tsgk-native-")
	if err != nil {
		return nil, refuse(kit.KindIO, "BUILD_DIR_FAILED", err)
	}
	b := &Build{Dir: dir, Compiler: req.CompilerID, Runtime: man.Commit, Symbol: req.Symbol, Defines: slices.Clone(req.Defines), Sanitize: req.Sanitize, cgroup: req.CgroupParent}
	slices.Sort(b.Defines)
	fail := func(code string, cause error) (*Build, error) {
		return b, refuse(kit.KindIO, code, cause)
	}
	for _, f := range man.Files {
		if err := copyVerified(req.Runtime, f.Path, filepath.Join(dir, "runtime", filepath.FromSlash(f.Path)), f.SHA256, f.Bytes); err != nil {
			return b, refuse(kit.KindInvalidInput, "RUNTIME_MISMATCH", fmt.Errorf("%s: %w", f.Path, err))
		}
		b.Closure = append(b.Closure, ClosureFile{"runtime", f.Path, f.SHA256, f.Bytes})
	}
	var parser string
	var scanners []string
	for _, g := range req.Grammar {
		if err := copyVerified(req.GrammarRoot, g.Path, filepath.Join(dir, "grammar", filepath.FromSlash(g.Path)), g.SHA256, int64(g.Bytes)); err != nil {
			return b, refuse(kit.KindInvalidInput, "SOURCE_MISMATCH", fmt.Errorf("%s: %w", g.Path, err))
		}
		b.Closure = append(b.Closure, ClosureFile{"grammar-" + g.Role, g.Path, g.SHA256, int64(g.Bytes)})
		switch g.Role {
		case "parser":
			parser = g.Path
		case "scanner":
			scanners = append(scanners, g.Path)
		}
	}
	shim := fmt.Sprintf("#include <tree_sitter/api.h>\nconst TSLanguage *%s(void);\nconst TSLanguage *tsgk_language(void) { return %s(); }\n", req.Symbol, req.Symbol)
	files := map[string][]byte{"driver/shim.c": []byte(shim)}
	for _, name := range []string{"driver.c", "cp949_table.h"} {
		data, err := drivers.NativeC.ReadFile("native-c/" + name)
		if err != nil {
			return fail("DRIVER_SOURCE_MISSING", err)
		}
		files["driver/"+name] = data
	}
	for _, rel := range slices.Sorted(func(yield func(string) bool) {
		for k := range files {
			if !yield(k) {
				return
			}
		}
	}) {
		if err := os.MkdirAll(filepath.Join(dir, "driver"), 0o755); err != nil {
			return fail("BUILD_DIR_FAILED", err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), files[rel], 0o644); err != nil {
			return fail("BUILD_DIR_FAILED", err)
		}
		s := sha256.Sum256(files[rel])
		b.Closure = append(b.Closure, ClosureFile{"driver", rel, hex.EncodeToString(s[:]), int64(len(files[rel]))})
	}
	tmp := filepath.Join(dir, "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return fail("BUILD_DIR_FAILED", err)
	}
	b.env = buildEnv(req.Compiler, tmp)
	for _, kv := range b.env {
		name, _, _ := strings.Cut(kv, "=")
		b.EnvNames = append(b.EnvNames, name)
	}
	var defs, san []string
	for _, d := range b.Defines {
		defs = append(defs, "-D"+d)
	}
	if req.Sanitize {
		san = []string{"-fsanitize=address,undefined", "-fno-omit-frame-pointer", "-fno-sanitize-recover=undefined"}
	}
	grammarInc := path.Join("grammar", path.Dir(parser))
	rtInc, rtSrc := "runtime/lib/include", "runtime/lib/src"
	steps := []struct {
		name string
		argv []string
	}{
		{"version", []string{"--version"}},
		{"runtime", []string{"-c", "-O2", "-std=c11", "-D_POSIX_C_SOURCE=200112L", "-D_DEFAULT_SOURCE", "-I", rtInc, "-I", rtSrc, rtSrc + "/lib.c", "-o", "lib.o"}},
		{"parser", []string{"-c", "-O0", "-std=gnu11", "-DTREE_SITTER_REUSE_ALLOCATOR", "-I", grammarInc, "grammar/" + parser, "-o", "parser.o"}},
	}
	objs := []string{"lib.o", "parser.o"}
	for i, s := range scanners {
		obj := fmt.Sprintf("scanner%d.o", i)
		argv := append([]string{"-c", "-O2", "-std=gnu11", "-DTREE_SITTER_REUSE_ALLOCATOR", "-I", path.Join("grammar", path.Dir(s)), "-I", grammarInc, "-I", rtInc}, defs...)
		steps = append(steps, struct {
			name string
			argv []string
		}{"scanner", append(argv, "grammar/"+s, "-o", obj)})
		objs = append(objs, obj)
	}
	steps = append(steps,
		struct {
			name string
			argv []string
		}{"driver", append(append([]string{"-c", "-O2", "-std=gnu11", "-I", rtInc, "-I", "driver"}, defs...), "driver/driver.c", "-o", "driver.o")},
		struct {
			name string
			argv []string
		}{"shim", []string{"-c", "-O2", "-std=gnu11", "-I", rtInc, "driver/shim.c", "-o", "shim.o"}},
	)
	objs = append(objs, "driver.o", "shim.o")
	steps = append(steps, struct {
		name string
		argv []string
	}{"link", append(objs, "-o", exeName())})
	// The identity binds every input byte, the compiler content, every argv and the define set.
	var text strings.Builder
	fmt.Fprintf(&text, "%s\nsymbol=%s\ncompiler=%s %d\nruntime=%s\nos=%s/%s\n", BuildSchema, req.Symbol, req.CompilerID.SHA256, req.CompilerID.Bytes, man.Commit, runtime.GOOS, runtime.GOARCH)
	cl := slices.Clone(b.Closure)
	slices.SortFunc(cl, func(x, y ClosureFile) int { return strings.Compare(x.Role+" "+x.Path, y.Role+" "+y.Path) })
	for _, f := range cl {
		fmt.Fprintf(&text, "file=%s %s %s %d\n", f.Role, f.Path, f.SHA256, f.Bytes)
	}
	for i := range steps {
		if san != nil && steps[i].name != "version" {
			steps[i].argv = append(slices.Clone(san), steps[i].argv...)
		}
		fmt.Fprintf(&text, "step=%s %q\n", steps[i].name, steps[i].argv)
	}
	idsum := sha256.Sum256([]byte(text.String()))
	b.Identity = hex.EncodeToString(idsum[:])
	for _, s := range steps {
		spec := runner.Spec{Path: req.Compiler, Args: s.argv, Dir: dir, Env: b.env, StdoutBytes: buildOutput, StderrBytes: buildOutput,
			Wall: buildWall, Grace: buildGrace, Memory: runner.Memory{Bytes: buildMemory, Hard: runtime.GOOS != "darwin"}, CgroupParent: req.CgroupParent}
		res, err := runner.Run(ctx, spec)
		step := BuildStep{Name: s.name, Argv: s.argv, Result: res, Executed: err == nil}
		if len(res.Stderr) > 0 {
			step.Stderr = tail(res.Stderr, 4096)
		}
		b.Steps = append(b.Steps, step)
		if err != nil {
			return fail("BUILD_START_FAILED", err)
		}
		if !res.Succeeded() {
			code := "BUILD_FAILED"
			switch res.Status {
			case runner.StatusResourceLimit:
				code = "BUILD_RESOURCE_LIMIT"
			case runner.StatusCancelled:
				return b, refuse(kit.KindCancelled, "CANCELLED", nil)
			}
			return fail(code, fmt.Errorf("%s exit %d %s", s.name, res.ExitCode, res.Status))
		}
		if s.name == "version" {
			line, _, _ := bytes.Cut(res.Stdout, []byte("\n"))
			b.CompilerVersion = strings.TrimSpace(string(line))
		}
	}
	b.Executable = filepath.Join(dir, exeName())
	if b.ExecutableSHA256, b.ExecutableBytes, err = fileDigest(b.Executable); err != nil {
		return fail("EXECUTABLE_MISSING", err)
	}
	return b, nil
}

// Verify re-hashes the executable before a launch: a replaced or modified driver is
// rejected instead of being run under the recorded identity.
func (b *Build) Verify() error {
	sum, size, err := fileDigest(b.Executable)
	if err != nil || sum != b.ExecutableSHA256 || size != b.ExecutableBytes {
		return refuse(kit.KindInvalidInput, "EXECUTABLE_MISMATCH", err)
	}
	return nil
}

// Remove deletes the build directory and confirms it is gone.
func (b *Build) Remove() error {
	if err := os.RemoveAll(b.Dir); err != nil {
		return err
	}
	if _, err := os.Lstat(b.Dir); !errors.Is(err, os.ErrNotExist) {
		return errors.New("build directory still present")
	}
	return nil
}

func tail(b []byte, n int) string {
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return strings.ToValidUTF8(string(b), "?")
}
