// Package reproduce runs one registered generator profile in two independent, initially
// empty workspaces through the kit runner and compares every registered output with the
// other workspace and, separately, with its reference artifact. The source root is read
// only; outputs are published to a new caller-owned directory without clobbering.
package reproduce

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// Schema of the reproduction result and claim values.
const (
	ResultSchema = "tsgk-reproduce-result/r1"
	ClaimPass    = "PASS"
	ClaimFail    = "FAIL"
	ClaimNone    = "NOT_CLAIMED"

	CapExecGenerator = "EXEC_GENERATOR"
)

// Request is one reproduction. Root, Work and Out are caller-owned; Work must exist and
// Out must not. Tools maps a profile tool name to its executable path.
type Request struct {
	Root         string
	Profile      []byte
	Tools        map[string]string
	Work         string
	Out          string
	Allow        []string // granted capabilities; EXEC_GENERATOR is required
	CgroupParent string
	Grace        time.Duration
}

// Claims are independent: a generator run, within-run determinism across the two
// workspaces, equality with the reference artifacts, and the closure kind proven.
type Claims struct {
	GeneratorRan     string `json:"generator_ran"`
	Deterministic    string `json:"deterministic"`
	ReferenceMatch   string `json:"reference_match"`
	JSReproduction   string `json:"js_reproduction"`
	JSONRegeneration string `json:"json_regeneration"`
}

// FileState is one observed file.
type FileState struct {
	Path   string `json:"path"`
	State  string `json:"state"` // PRESENT or MISSING
	SHA256 string `json:"sha256,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
}

// Run is one workspace execution.
type Run struct {
	Workspace     string         `json:"workspace"`
	State         string         `json:"state"` // EXECUTED, NOT_RUN
	Process       *runner.Result `json:"process,omitempty"`
	StdoutSHA256  string         `json:"stdout_sha256,omitempty"`
	StderrSHA256  string         `json:"stderr_sha256,omitempty"`
	Outputs       []FileState    `json:"outputs"`
	Unregistered  []FileState    `json:"unregistered"`
	SourceWritten []string       `json:"source_written"`
	StorageBytes  int64          `json:"storage_bytes"`
}

// Comparison is the per-output verdict.
type Comparison struct {
	Path      string `json:"path"`
	AcrossRun string `json:"across_runs"` // EQUAL, DIFFERENT, MISSING, NOT_RUN
	Reference string `json:"reference"`   // MATCH, MISMATCH, REFERENCE_ABSENT, OUTPUT_MISSING, NOT_RUN
}

// Result is the reproduction report.
type Result struct {
	kit.Report
	ResultSchema string              `json:"result_schema"`
	ProfileID    string              `json:"profile_id"`
	Route        string              `json:"route"`
	Mode         string              `json:"mode"`
	RunIdentity  string              `json:"run_identity"`
	Argv         []string            `json:"argv"`
	EnvNames     []string            `json:"env_names"`
	Tools        []kit.ToolIdentity  `json:"tools"`
	Claims       Claims              `json:"claims"`
	Runs         []Run               `json:"runs"`
	Comparisons  []Comparison        `json:"comparisons"`
	SourceBefore string              `json:"source_before"`
	SourceAfter  string              `json:"source_after"`
	Workspaces   string              `json:"workspaces"` // REMOVED or the failure
	Capabilities runner.Capabilities `json:"capabilities"`
}

// Error is a refusal before any generator launch or an evidence failure.
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

// hooks for fault injection in tests (S04-A09).
var (
	removeAll = os.RemoveAll
	writeFile = func(name string, data []byte) error {
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		_, werr := f.Write(data)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		return werr
	}
)

// Reproduce validates the profile, the snapshot, tools and capabilities before any launch,
// runs the generator in workspace A then B, compares, publishes and removes workspaces.
// A refusal before launch returns a NOT_RUN report and an *Error; a completed or failed
// execution returns its report with a nil error unless publication failed.
func Reproduce(ctx context.Context, req Request) (Result, error) {
	res := Result{Report: newReport(), ResultSchema: ResultSchema, Runs: []Run{}, Comparisons: []Comparison{}, Tools: []kit.ToolIdentity{}, EnvNames: []string{}}
	block := func(e *Error) (Result, error) {
		res.ExecutionStatus, res.EvidenceMode = kit.StatusNotRun, kit.ModeNotRun
		res.Assessment = kit.AssessBlocked
		if e.Kind == kit.KindInvalidInput {
			res.Assessment = kit.AssessNotAssessed
		}
		res.Findings = append(res.Findings, kit.Finding{Code: e.Code, Severity: "error", Message: "실행 전 거부: 생성기를 실행하지 않았다"})
		return res, e
	}
	prof, err := kit.ParseReproduceProfile(req.Profile)
	if err != nil {
		var ke *kit.Error
		errors.As(err, &ke)
		return block(refuse(kit.KindInvalidInput, ke.Code, err))
	}
	res.ProfileID, res.Route, res.Mode = prof.ID, prof.Route, prof.Mode
	if !slices.Contains(req.Allow, CapExecGenerator) {
		return block(refuse(kit.KindUnsupported, "CAPABILITY_NOT_GRANTED", errors.New("EXEC_GENERATOR was not granted")))
	}
	for _, p := range []string{req.Root, req.Work, req.Out} {
		if !filepath.IsAbs(p) {
			return block(refuse(kit.KindInvalidInput, "PATH_NOT_ABSOLUTE", fmt.Errorf("%q", p)))
		}
	}
	if info, err := os.Stat(req.Work); err != nil || !info.IsDir() {
		return block(refuse(kit.KindInvalidInput, "WORK_INVALID", err))
	}
	if _, err := os.Lstat(req.Out); err == nil {
		return block(refuse(kit.KindInvalidInput, "OUTPUT_EXISTS", nil))
	}
	snap, e := snapshot(req.Root, prof)
	if e != nil {
		return block(e)
	}
	res.SourceBefore = snap.digest
	// Tools: exact content identity, copied once into a private tools directory and executed
	// from there, so a replaced original after the check cannot run.
	toolDir := filepath.Join(req.Work, "tsgk-tools-"+nonce())
	if err := os.Mkdir(toolDir, 0o700); err != nil {
		return block(refuse(kit.KindIO, "WORK_INVALID", err))
	}
	toolsRemoved := false
	defer func() {
		if !toolsRemoved { // refusals before launch still remove the copies
			removeAll(toolDir)
		}
	}()
	wanted := []kit.ToolIdentity{prof.Generator}
	if prof.JSRuntime != nil {
		wanted = append(wanted, *prof.JSRuntime)
	}
	paths := map[string]string{}
	for _, tool := range wanted {
		p, e := pinTool(tool, req.Tools[tool.Name], toolDir)
		if e != nil {
			return block(e)
		}
		paths[tool.Name] = p
		res.Tools = append(res.Tools, tool)
	}
	hard := runtime.GOOS != "darwin" // Windows Job Object and Linux cgroup caps are required
	grace := req.Grace
	if grace <= 0 {
		grace = 2 * time.Second
	}
	base := runner.Spec{Path: paths[prof.Generator.Name], StdoutBytes: int64(prof.Limits.OutputBytes), StderrBytes: int64(prof.Limits.OutputBytes),
		Wall: time.Duration(prof.Limits.WallSeconds) * time.Second, Grace: grace,
		Memory: runner.Memory{Bytes: prof.Limits.MemoryBytes, Hard: hard}, CgroupParent: req.CgroupParent}
	res.Capabilities = runner.Probe(base)
	if hard && res.Capabilities.Memory != runner.MemoryHard {
		return block(refuse(kit.KindUnsupported, "MEMORY_HARD_CAP_UNSUPPORTED", fmt.Errorf("backend %s", res.Capabilities.Backend)))
	}
	if prof.Mode == kit.ModeJS {
		if leak := jsLeak(req.Work); leak != "" {
			return block(refuse(kit.KindUnsupported, "JS_CLOSURE_LEAK", errors.New(leak)))
		}
	}
	res.Argv = argv(prof, "<workspace>/out", "<tools>/"+filepath.Base(paths[nodeName(prof)]))
	res.EnvNames = envNames()
	res.RunIdentity = runIdentity(prof, snap.digest, res.Argv, res.EnvNames, res.Capabilities)
	res.Identities = append(res.Identities,
		kit.IdentityRef{Role: "profile", Schema: kit.ReproduceSchema, SHA256: prof.SHA256},
		kit.IdentityRef{Role: "source_snapshot", Schema: "tsgk-snapshot/r1", SHA256: snap.digest},
		kit.IdentityRef{Role: "run", Schema: "tsgk-reproduce-run/r1", SHA256: res.RunIdentity})
	res.Coverage.Requested = []string{"generator_ran", "deterministic", "reference_match"}

	// EXECUTE: workspace A then B, serially (local heavy work is one at a time).
	stamp := nonce()
	var wsDirs []string
	for _, name := range []string{"a", "b"} {
		dir := filepath.Join(req.Work, "tsgk-ws-"+stamp+"-"+name)
		wsDirs = append(wsDirs, dir)
		run := execute(ctx, base, prof, snap, dir, name, paths)
		res.Runs = append(res.Runs, run)
		if run.Process == nil || !run.Process.Succeeded() || run.State != "EXECUTED" || run.StorageBytes > int64(prof.Limits.StorageBytes) {
			break
		}
	}
	for len(res.Runs) < 2 {
		res.Runs = append(res.Runs, Run{Workspace: "b", State: "NOT_RUN", Outputs: []FileState{}, Unregistered: []FileState{}, SourceWritten: []string{}})
	}
	compare(&res, prof)
	after, e := snapshot(req.Root, prof)
	if e != nil {
		res.SourceAfter = "UNREADABLE"
		res.Findings = append(res.Findings, kit.Finding{Code: "SOURCE_CHANGED", Severity: "error", Message: "실행 후 source를 다시 확인하지 못했다"})
	} else {
		res.SourceAfter = after.digest
		if after.digest != snap.digest {
			res.Findings = append(res.Findings, kit.Finding{Code: "SOURCE_CHANGED", Severity: "error", Message: "실행 중 source root가 바뀌었다"})
		}
	}
	status(&res, prof.Limits.StorageBytes)
	// PUBLISH, then remove the workspaces; a failure of either is never a completed success.
	perr := publish(req.Out, &res, wsDirs)
	res.Workspaces = "REMOVED"
	for _, d := range wsDirs {
		if err := removeAll(d); err != nil {
			res.Workspaces = "CLEANUP_FAILED: " + err.Error()
			res.Findings = append(res.Findings, kit.Finding{Code: "WORKSPACE_CLEANUP_FAILED", Severity: "error", Path: filepath.Base(d), Message: "작업 공간을 지우지 못해 완료로 보고하지 않는다"})
			res.ExecutionStatus = kit.StatusFailed
		}
	}
	toolsRemoved = true
	if err := removeAll(toolDir); err != nil {
		res.Workspaces = "CLEANUP_FAILED: " + err.Error()
		res.Findings = append(res.Findings, kit.Finding{Code: "TOOL_CLEANUP_FAILED", Severity: "error", Path: filepath.Base(toolDir), Message: "도구 사본을 지우지 못해 완료로 보고하지 않는다"})
		res.ExecutionStatus = kit.StatusFailed
	}
	settle(&res)
	if perr != nil {
		res.ExecutionStatus = kit.StatusFailed
		res.Findings = append(res.Findings, kit.Finding{Code: "EVIDENCE_WRITE_FAILED", Severity: "error", Message: "결과 publication이 실패했다"})
		settle(&res)
		writeResult(req.Out, &res) // a partial publication keeps its failure report when it can
		return res, refuse(kit.KindIO, "EVIDENCE_WRITE_FAILED", perr)
	}
	if err := writeResult(req.Out, &res); err != nil {
		res.ExecutionStatus = kit.StatusFailed
		settle(&res)
		return res, refuse(kit.KindIO, "EVIDENCE_WRITE_FAILED", err)
	}
	return res, nil
}

// settle keeps an execution that did not complete (source change, storage limit, cleanup
// or evidence failure) from carrying an earlier PASS assessment or mode claim; the
// observational claims (generator_ran, deterministic, reference_match) stay as observed.
func settle(res *Result) {
	if res.ExecutionStatus != kit.StatusCompleted {
		for _, c := range []*string{&res.Claims.JSReproduction, &res.Claims.JSONRegeneration} {
			if *c == ClaimPass {
				*c = ClaimNone
			}
		}
	}
	switch {
	case res.ExecutionStatus == kit.StatusResourceLimit:
		res.Assessment = kit.AssessBlocked
	case res.ExecutionStatus != kit.StatusCompleted && res.Assessment == kit.AssessPass:
		res.Assessment = kit.AssessNotAssessed
	}
}

// jsLeak names a module location Node would search from a workspace below work although
// the declared snapshot does not contain it: node_modules in work or any ancestor, and the
// global folder <prefix>/lib/node of the copied runtime (<work>/tsgk-tools-*/node).
func jsLeak(work string) string {
	chains := []string{work}
	if real, err := filepath.EvalSymlinks(work); err == nil && real != work {
		chains = append(chains, real) // Node resolves from real paths (macOS /var -> /private/var)
	}
	for _, w := range chains {
		if _, err := os.Stat(filepath.Join(w, "lib", "node")); err == nil {
			return filepath.Join(w, "lib", "node")
		}
		for d := w; ; {
			if _, err := os.Stat(filepath.Join(d, "node_modules")); err == nil {
				return filepath.Join(d, "node_modules")
			}
			up := filepath.Dir(d)
			if up == d {
				break
			}
			d = up
		}
	}
	return ""
}

func newReport() kit.Report {
	return kit.Report{Schema: kit.ReportSchema, Command: "reproduce", ExecutionStatus: kit.StatusCompleted, EvidenceMode: kit.ModeNewRun, Assessment: kit.AssessNotAssessed,
		Identities: []kit.IdentityRef{}, Findings: []kit.Finding{}, Coverage: kit.Coverage{Requested: []string{}, Observed: []string{}, Unsupported: []string{}}}
}

func nonce() string {
	var b [6]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func nodeName(p kit.ReproduceProfile) string {
	if p.JSRuntime != nil {
		return p.JSRuntime.Name
	}
	return ""
}

// argv is the fixed generator command; profiles cannot add arbitrary arguments.
func argv(p kit.ReproduceProfile, out, jsRuntime string) []string {
	a := []string{"generate", "--abi", strconv.FormatUint(p.ABI, 10)}
	if !p.Optimize {
		a = append(a, "--disable-optimization")
	}
	a = append(a, "--output", out)
	if p.Mode == kit.ModeJS {
		a = append(a, "--js-runtime", jsRuntime)
	}
	return append(a, p.Grammar)
}

// env isolates every per-user configuration, cache and temporary location in the
// workspace; PATH is empty, so a JSON run cannot reach any JavaScript runtime.
func env(ws string) []string {
	home := filepath.Join(ws, "home")
	tmp := filepath.Join(ws, "tmp")
	cache := filepath.Join(ws, "cache")
	return []string{"PATH=", "HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"), "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_CACHE_HOME=" + cache, "TMP=" + tmp, "TEMP=" + tmp, "TMPDIR=" + tmp,
		"TREE_SITTER_DIR=" + filepath.Join(home, ".tree-sitter"), "TREE_SITTER_LIBDIR=" + filepath.Join(cache, "lib")}
}

func envNames() []string {
	var names []string
	for _, kv := range env("ws") {
		n, _, _ := strings.Cut(kv, "=")
		names = append(names, n)
	}
	return names
}

// runIdentity binds every input of the experiment: profile semantics, tool contents,
// snapshot, argv, environment template and backend; any change is a new identity.
func runIdentity(p kit.ReproduceProfile, snap string, argv, envNames []string, caps runner.Capabilities) string {
	body, _ := json.Marshal(struct {
		Schema  string
		Profile kit.ReproduceProfile
		Snap    string
		Argv    []string
		Env     []string
		Backend runner.Capabilities
	}{"tsgk-reproduce-run/r1", p, snap, argv, envNames, caps})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

type snap struct {
	files  map[string][]byte
	digest string
}

// snapshot reads exactly the declared files below root, rejecting links and special
// entries on every component, and checks each against its declared size and digest.
func snapshot(root string, p kit.ReproduceProfile) (*snap, *Error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, refuse(kit.KindInvalidInput, "ROOT_INVALID", err)
	}
	s := &snap{files: map[string][]byte{}}
	h := sha256.New()
	for _, in := range p.Inputs {
		cur := root
		for _, seg := range strings.Split(in.Path, "/") {
			cur = filepath.Join(cur, seg)
			fi, err := os.Lstat(cur)
			if err != nil {
				return nil, refuse(kit.KindInvalidInput, "SOURCE_MISSING", fmt.Errorf("%s: %w", in.Path, err))
			}
			if fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular|fs.ModeDevice|fs.ModeNamedPipe|fs.ModeSocket) != 0 {
				return nil, refuse(kit.KindInvalidInput, "SOURCE_LINK_REJECTED", errors.New(in.Path))
			}
		}
		f, err := os.Open(cur)
		if err != nil {
			return nil, refuse(kit.KindIO, "SOURCE_UNREADABLE", err)
		}
		data, err := io.ReadAll(io.LimitReader(f, int64(p.Limits.FileBytes)+1))
		f.Close()
		if err != nil {
			return nil, refuse(kit.KindIO, "SOURCE_UNREADABLE", err)
		}
		sum := sha256.Sum256(data)
		if uint64(len(data)) != in.Bytes || hex.EncodeToString(sum[:]) != in.SHA256 {
			return nil, refuse(kit.KindInvalidInput, "SOURCE_MISMATCH", errors.New(in.Path))
		}
		s.files[in.Path] = data
		fmt.Fprintf(h, "%s\x00%s\x00%d\n", in.Path, in.SHA256, in.Bytes)
	}
	s.digest = hex.EncodeToString(h.Sum(nil))
	return s, nil
}

// pinTool checks the executable's exact content and copies it into dir; the copy runs.
func pinTool(tool kit.ToolIdentity, path, dir string) (string, *Error) {
	if path == "" {
		return "", refuse(kit.KindUnsupported, "TOOL_MISSING", errors.New(tool.Name))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", refuse(kit.KindUnsupported, "TOOL_MISSING", err)
	}
	fi, err := os.Stat(abs)
	if err != nil || !fi.Mode().IsRegular() {
		return "", refuse(kit.KindUnsupported, "TOOL_MISSING", fmt.Errorf("%s: %v", tool.Name, err))
	}
	if uint64(fi.Size()) != tool.Bytes {
		return "", refuse(kit.KindUnsupported, "TOOL_IDENTITY_MISMATCH", fmt.Errorf("%s size", tool.Name))
	}
	src, err := os.Open(abs)
	if err != nil {
		return "", refuse(kit.KindUnsupported, "TOOL_MISSING", err)
	}
	defer src.Close()
	name := tool.Name
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dst := filepath.Join(dir, name)
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return "", refuse(kit.KindIO, "TOOL_COPY_FAILED", err)
	}
	h := sha256.New()
	_, cerr := io.Copy(io.MultiWriter(out, h), io.LimitReader(src, int64(tool.Bytes)+1))
	if err := out.Close(); cerr == nil {
		cerr = err
	}
	if cerr != nil {
		return "", refuse(kit.KindIO, "TOOL_COPY_FAILED", cerr)
	}
	if hex.EncodeToString(h.Sum(nil)) != tool.SHA256 {
		return "", refuse(kit.KindUnsupported, "TOOL_IDENTITY_MISMATCH", fmt.Errorf("%s sha256", tool.Name))
	}
	return dst, nil
}

// execute creates one new empty workspace, materializes the snapshot and runs the generator.
func execute(ctx context.Context, base runner.Spec, p kit.ReproduceProfile, s *snap, dir, name string, tools map[string]string) Run {
	run := Run{Workspace: name, State: "NOT_RUN", Outputs: []FileState{}, Unregistered: []FileState{}, SourceWritten: []string{}}
	// A pre-existing directory (stale or poisoned) is never reused.
	if err := os.Mkdir(dir, 0o700); err != nil {
		run.State = "WORKSPACE_REFUSED"
		return run
	}
	src := filepath.Join(dir, "src")
	for _, d := range []string{src, filepath.Join(dir, "out"), filepath.Join(dir, "home"), filepath.Join(dir, "tmp"), filepath.Join(dir, "cache")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			run.State = "WORKSPACE_REFUSED"
			return run
		}
	}
	for _, in := range p.Inputs {
		dst := filepath.Join(src, filepath.FromSlash(in.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			run.State = "WORKSPACE_REFUSED"
			return run
		}
		if err := writeFile(dst, s.files[in.Path]); err != nil {
			run.State = "WORKSPACE_REFUSED"
			return run
		}
	}
	spec := base
	spec.Dir = src
	spec.Env = env(dir)
	spec.Args = argv(p, filepath.Join(dir, "out"), tools[nodeName(p)])
	r, err := runner.Run(ctx, spec)
	if err != nil {
		run.State = "START_FAILED"
		return run
	}
	run.State = "EXECUTED"
	run.Process = &r
	run.StdoutSHA256, run.StderrSHA256 = digest(r.Stdout), digest(r.Stderr)
	// Collect every file in out/: registered outputs and anything else the tool wrote.
	produced := map[string]FileState{}
	filepath.WalkDir(filepath.Join(dir, "out"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(filepath.Join(dir, "out"), path)
		rel = filepath.ToSlash(rel)
		produced[rel] = fileState(rel, path, d)
		return nil
	})
	for _, o := range p.Outputs {
		if fsx, ok := produced[o.Path]; ok {
			run.Outputs = append(run.Outputs, fsx)
			delete(produced, o.Path)
		} else {
			run.Outputs = append(run.Outputs, FileState{Path: o.Path, State: "MISSING"})
		}
	}
	for _, k := range sortedKeys(produced) {
		run.Unregistered = append(run.Unregistered, produced[k])
	}
	// Writes into the materialized source tree are observed, never silently accepted.
	filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		rel = filepath.ToSlash(rel)
		fsx := fileState(rel, path, d)
		if data, ok := s.files[rel]; !ok || fsx.SHA256 != digest(data) {
			run.SourceWritten = append(run.SourceWritten, rel)
		}
		return nil
	})
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				run.StorageBytes += fi.Size()
			}
		}
		return nil
	})
	return run
}

func fileState(rel, path string, d fs.DirEntry) FileState {
	if d.Type()&fs.ModeType != 0 {
		return FileState{Path: rel, State: "SPECIAL"}
	}
	f, err := os.Open(path)
	if err != nil {
		return FileState{Path: rel, State: "UNREADABLE"}
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return FileState{Path: rel, State: "UNREADABLE"}
	}
	return FileState{Path: rel, State: "PRESENT", SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: n}
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// compare fills per-output verdicts and the claims.
func compare(res *Result, p kit.ReproduceProfile) {
	a, b := res.Runs[0], res.Runs[1]
	ran := func(r Run) bool { return r.State == "EXECUTED" && r.Process != nil && r.Process.Succeeded() }
	c := &res.Claims
	c.GeneratorRan, c.Deterministic, c.ReferenceMatch = ClaimFail, ClaimNone, ClaimNone
	if ran(a) && ran(b) {
		c.GeneratorRan = ClaimPass
	}
	same, refOK, refAbsent, refBad := true, true, false, false
	for i, o := range p.Outputs {
		cmp := Comparison{Path: o.Path, AcrossRun: "NOT_RUN", Reference: "NOT_RUN"}
		if ran(a) {
			oa := a.Outputs[i]
			switch {
			case oa.State != "PRESENT":
				cmp.Reference, refBad = "OUTPUT_MISSING", true
			case o.Reference == kit.ReferenceAbsent:
				cmp.Reference, refAbsent = "REFERENCE_ABSENT", true
			case oa.SHA256 == o.SHA256 && uint64(oa.Bytes) == o.Bytes:
				cmp.Reference = "MATCH"
			default:
				cmp.Reference, refBad = "MISMATCH", true
			}
			if ran(b) {
				ob := b.Outputs[i]
				switch {
				case oa.State != "PRESENT" || ob.State != "PRESENT":
					cmp.AcrossRun, same = "MISSING", false
				case oa.SHA256 == ob.SHA256 && oa.Bytes == ob.Bytes:
					cmp.AcrossRun = "EQUAL"
				default:
					cmp.AcrossRun, same = "DIFFERENT", false
				}
			}
		} else {
			refOK = false
		}
		res.Comparisons = append(res.Comparisons, cmp)
	}
	if ran(a) && ran(b) {
		// Unregistered outputs are compared too; a different extra file breaks determinism.
		if !slices.EqualFunc(a.Unregistered, b.Unregistered, func(x, y FileState) bool { return x == y }) {
			same = false
			res.Findings = append(res.Findings, kit.Finding{Code: "UNREGISTERED_OUTPUT_DIFFERENT", Severity: "error", Message: "등록되지 않은 생성물이 두 작업 공간에서 다르다"})
		}
		c.Deterministic = map[bool]string{true: ClaimPass, false: ClaimFail}[same]
	}
	for _, r := range []Run{a, b} {
		for _, u := range r.Unregistered {
			res.Findings = append(res.Findings, kit.Finding{Code: "UNREGISTERED_OUTPUT", Severity: "warning", Path: r.Workspace + ":" + u.Path, Message: "등록되지 않은 생성물이 있다"})
		}
		for _, w := range r.SourceWritten {
			res.Findings = append(res.Findings, kit.Finding{Code: "WORKSPACE_SOURCE_WRITTEN", Severity: "warning", Path: r.Workspace + ":" + w, Message: "생성기가 작업 공간의 source 사본을 바꿨다(원본 root는 별도 확인)"})
		}
	}
	switch {
	case refBad:
		c.ReferenceMatch = ClaimFail
	case ran(a) && refOK && !refAbsent:
		c.ReferenceMatch = ClaimPass
	case refAbsent:
		res.Findings = append(res.Findings, kit.Finding{Code: "REFERENCE_ABSENT", Severity: "info", Message: "기준 생성물이 없어 기준 일치 claim을 하지 않는다"})
	}
	all := c.GeneratorRan == ClaimPass && c.Deterministic == ClaimPass && c.ReferenceMatch == ClaimPass
	c.JSReproduction, c.JSONRegeneration = ClaimNone, ClaimNone
	if p.Mode == kit.ModeJS {
		c.JSReproduction = map[bool]string{true: ClaimPass, false: ClaimNone}[all]
		if c.GeneratorRan == ClaimFail || c.Deterministic == ClaimFail || c.ReferenceMatch == ClaimFail {
			c.JSReproduction = ClaimFail
		}
	} else {
		c.JSONRegeneration = map[bool]string{true: ClaimPass, false: ClaimNone}[all]
		if c.GeneratorRan == ClaimFail || c.Deterministic == ClaimFail || c.ReferenceMatch == ClaimFail {
			c.JSONRegeneration = ClaimFail
		}
		res.Coverage.Unsupported = append(res.Coverage.Unsupported, "js_reproduction: JSON_ONLY_RUN")
	}
	for _, kv := range [][2]string{{"generator_ran", c.GeneratorRan}, {"deterministic", c.Deterministic}, {"reference_match", c.ReferenceMatch}} {
		if kv[1] != ClaimNone {
			res.Coverage.Observed = append(res.Coverage.Observed, kv[0])
		}
	}
}

// status derives execution status and assessment from the runs and claims.
func status(res *Result, storage uint64) {
	res.ExecutionStatus = kit.StatusCompleted
	for _, r := range res.Runs {
		switch {
		case r.State == "NOT_RUN":
		case r.Process == nil:
			res.ExecutionStatus = kit.StatusFailed
			res.Findings = append(res.Findings, kit.Finding{Code: "WORKSPACE_" + r.State, Severity: "error", Path: r.Workspace, Message: "작업 공간 실행을 시작하지 못했다"})
		case r.Process.Reason == runner.ReasonExited && r.Process.ExitCode != 0:
			res.ExecutionStatus = kit.StatusFailed
			res.Findings = append(res.Findings, kit.Finding{Code: "GENERATOR_EXIT_NONZERO", Severity: "error", Path: r.Workspace, Message: "생성기가 실패 종료했다"})
		case r.Process.Status != runner.StatusCompleted:
			res.ExecutionStatus = r.Process.Status
			res.Findings = append(res.Findings, kit.Finding{Code: r.Process.Reason, Severity: "error", Path: r.Workspace, Message: "생성기가 한도나 취소로 끝났다"})
		case !r.Process.Cleanup.Verified:
			res.ExecutionStatus = kit.StatusFailed
			res.Findings = append(res.Findings, kit.Finding{Code: "PROCESS_CLEANUP_UNVERIFIED", Severity: "error", Path: r.Workspace, Message: "process tree 정리를 확인하지 못했다"})
		}
		if r.State == "EXECUTED" && r.StorageBytes > int64(storage) {
			res.Findings = append(res.Findings, kit.Finding{Code: "STORAGE_LIMIT", Severity: "error", Path: r.Workspace, Message: "실행 뒤 작업 공간 용량이 profile 한도를 넘었다"})
			if res.ExecutionStatus == kit.StatusCompleted {
				res.ExecutionStatus = kit.StatusResourceLimit
			}
		}
		if res.ExecutionStatus != kit.StatusCompleted {
			break
		}
	}
	for _, f := range res.Findings {
		if f.Code == "SOURCE_CHANGED" {
			res.ExecutionStatus = kit.StatusFailed
		}
	}
	c := res.Claims
	switch {
	case c.GeneratorRan == ClaimFail && res.ExecutionStatus != kit.StatusCompleted:
		res.Assessment = kit.AssessNotAssessed
		if res.ExecutionStatus == kit.StatusResourceLimit {
			res.Assessment = kit.AssessBlocked
		}
	case c.Deterministic == ClaimFail || c.ReferenceMatch == ClaimFail || c.GeneratorRan == ClaimFail:
		res.Assessment = kit.AssessFail
	case c.ReferenceMatch == ClaimNone || c.Deterministic == ClaimNone:
		res.Assessment = kit.AssessBlocked
	default:
		res.Assessment = kit.AssessPass
	}
}

// publish creates out exclusively and copies each workspace's outputs and logs into it.
func publish(out string, res *Result, wsDirs []string) error {
	if err := os.Mkdir(out, 0o755); err != nil {
		return err
	}
	for i, r := range res.Runs {
		if r.State != "EXECUTED" || i >= len(wsDirs) {
			continue
		}
		dst := filepath.Join(out, "workspace-"+r.Workspace)
		if err := os.Mkdir(dst, 0o755); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(dst, "stdout.log"), r.Process.Stdout); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(dst, "stderr.log"), r.Process.Stderr); err != nil {
			return err
		}
		files := append(slices.Clone(r.Outputs), r.Unregistered...)
		for _, f := range files {
			if f.State != "PRESENT" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(wsDirs[i], "out", filepath.FromSlash(f.Path)))
			if err != nil {
				return err
			}
			if digest(data) != f.SHA256 {
				return fmt.Errorf("%s changed after comparison", f.Path)
			}
			p := filepath.Join(dst, "out", filepath.FromSlash(f.Path))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			if err := writeFile(p, data); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeResult writes result.json last; its absence marks an incomplete publication.
func writeResult(out string, res *Result) error {
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(out, "result.json"), append(data, '\n'))
}

// Encode renders the result as one JSON line for the CLI.
func Encode(res Result) []byte {
	data, _ := json.Marshal(res)
	return append(bytes.TrimSpace(data), '\n')
}
