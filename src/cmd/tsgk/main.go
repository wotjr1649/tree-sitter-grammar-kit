// Command tsgk is the CLI over the public offline kit API. It only parses arguments,
// renders the API result, publishes --out without clobbering, and maps exit codes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/reproduce"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const (
	exitOK       = 0
	exitFail     = 1
	exitUsage    = 2
	exitBlocked  = 3
	exitIO       = 4
	exitCanceled = 130
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

// future commands are owned by later sessions; this build rejects them clearly.
var future = map[string]string{"replay": "S07", "evidence": "S07", "parity": "S08"}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "tsgk: USAGE: tsgk <inspect|identity|corpus|verify> --root PATH [--out PATH] | tsgk schema <check|diff> | tsgk reproduce | tsgk incremental | tsgk oracle record")
		return exitUsage
	}
	cmd := args[0]
	if cmd == "schema" {
		return runSchema(ctx, args[1:], stdout, stderr)
	}
	if cmd == "reproduce" {
		return runReproduce(ctx, args[1:], stdout, stderr)
	}
	if cmd == "incremental" {
		return runIncremental(ctx, args[1:], stdout, stderr)
	}
	if cmd == "oracle" {
		return runOracle(ctx, args[1:], stdout, stderr)
	}
	if owner, ok := future[cmd]; ok {
		fmt.Fprintf(stderr, "tsgk: UNSUPPORTED_COMMAND: %s는 %s 범위이며 이 build에서 구현되지 않았다\n", cmd, owner)
		return exitUsage
	}
	if cmd != "inspect" && cmd != "identity" && cmd != "corpus" && cmd != "verify" {
		fmt.Fprintf(stderr, "tsgk: UNKNOWN_COMMAND: %s\n", cmd)
		return exitUsage
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "snapshot root")
	out := fs.String("out", "", "new result file outside the root (no clobber)")
	profile := fs.String("profile", "", "strict tsgk-profile/r1 file")
	var grammar, encProfile, large, archive, archiveRoot, expected *string
	var files, declares, nested multi
	if cmd != "corpus" {
		grammar = fs.String("grammar", ".", `grammar directory ("." is the root)`)
	}
	if cmd == "identity" || cmd == "verify" {
		large = fs.String("large-file-profile", "", "registered identity-scoped exception")
	}
	if cmd == "identity" {
		fs.Var(&files, "file", "explicit selection PATH=ROLE (repeatable)")
	}
	if cmd == "verify" {
		archive = fs.String("archive", "", "ZIP archive subject instead of --root")
		archiveRoot = fs.String("archive-root", "", "member directory that maps to expected paths")
		fs.Var(&nested, "nested", "explicitly inspected nested ZIP member (repeatable)")
		expected = fs.String("expected", "", "caller-trusted tsgk-expected/r1 file (required)")
	}
	if cmd != "inspect" {
		def := ""
		if cmd == "corpus" {
			def = "cp949"
		}
		encProfile = fs.String("encoding-profile", def, `profile encoding declaration ("cp949" or "none")`)
		fs.Var(&declares, "declare", "per-file encoding declaration PATH=utf-8|cp949 (repeatable)")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "tsgk: USAGE: unexpected arguments")
		return exitUsage
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if *profile != "" && cmd == "inspect" {
		fmt.Fprintln(stderr, "tsgk: PROFILE_UNSUPPORTED: inspect는 profile을 읽지 않는다")
		return exitUsage
	}
	var enc kit.EncodingPolicy
	if encProfile != nil {
		switch *encProfile {
		case "cp949":
			// With --profile, corpus takes its encoding only from the profile (none if absent).
			if cmd != "corpus" || *profile == "" || set["encoding-profile"] {
				enc.Profile = "cp949"
			}
		case "", "none":
		default:
			fmt.Fprintln(stderr, "tsgk: USAGE: --encoding-profile must be cp949 or none")
			return exitUsage
		}
		for _, d := range declares {
			p, e, ok := cutLast(d)
			if !ok {
				fmt.Fprintln(stderr, "tsgk: USAGE: --declare PATH=ENCODING")
				return exitUsage
			}
			enc.Files = append(enc.Files, kit.FileEncoding{Path: p, Encoding: e})
		}
	}
	limits := kit.DefaultLimits()
	var profileData, expectedData []byte
	if *profile != "" {
		var rerr error
		if profileData, rerr = readDocument(*profile); rerr != nil {
			fmt.Fprintf(stderr, "tsgk: PROFILE_UNREADABLE: %v\n", rerr)
			return exitIO
		}
	}
	var result any
	var err error
	switch cmd {
	case "inspect", "identity":
		sel := kit.Selection{Grammar: *grammar}
		for _, f := range files {
			p, role, ok := cutLast(f)
			if !ok {
				fmt.Fprintln(stderr, "tsgk: USAGE: --file PATH=ROLE")
				return exitUsage
			}
			sel.Files = append(sel.Files, kit.FileSelection{Path: p, Role: role})
		}
		cctx, cancel := context.WithTimeout(ctx, limits.Wall+5*time.Second)
		defer cancel()
		if cmd == "inspect" {
			result, err = kit.Inspect(cctx, kit.InspectRequest{Root: *root, Selection: sel, Limits: limits})
		} else {
			result, err = kit.Identity(cctx, kit.IdentityRequest{Root: *root, Selection: sel, Limits: limits, Encoding: enc, LargeFileProfile: *large, Profile: profileData})
		}
	case "corpus":
		limits := kit.DefaultCorpusLimits()
		cctx, cancel := context.WithTimeout(ctx, limits.Wall+5*time.Second)
		defer cancel()
		result, err = kit.Corpus(cctx, kit.CorpusRequest{Root: *root, Limits: limits, Encoding: enc, Profile: profileData})
	case "verify":
		if *expected == "" {
			fmt.Fprintln(stderr, "tsgk: USAGE: verify requires --expected FILE")
			return exitUsage
		}
		req := kit.VerifyRequest{Root: *root, Archive: *archive, ArchiveRoot: *archiveRoot, Nested: nested, Selection: kit.Selection{Grammar: *grammar},
			Limits: limits, ArchiveLimits: kit.DefaultArchiveLimits(), Encoding: enc, LargeFileProfile: *large, Profile: profileData}
		if *archive != "" {
			if set["root"] || set["grammar"] {
				fmt.Fprintln(stderr, "tsgk: USAGE: --archive excludes --root and --grammar")
				return exitUsage
			}
			req.Root, req.Selection = "", kit.Selection{}
		} else {
			// A trust document taken from inside the subject would certify itself.
			for _, d := range [][2]string{{"EXPECTED_INSIDE_INPUT", *expected}, {"PROFILE_INSIDE_INPUT", *profile}} {
				if name, p := d[0], d[1]; p != "" && documentInside(p, *root) {
					fmt.Fprintf(stderr, "tsgk: %s: 신뢰 문서는 검증 대상 root 밖에 있어야 한다\n", name)
					return exitUsage
				}
			}
		}
		var rerr error
		if expectedData, rerr = readDocument(*expected); rerr != nil {
			fmt.Fprintf(stderr, "tsgk: EXPECTED_UNREADABLE: %v\n", rerr)
			return exitIO
		}
		req.Expected = expectedData
		cctx, cancel := context.WithTimeout(ctx, limits.Wall+5*time.Second)
		defer cancel()
		result, err = kit.Verify(cctx, req)
	}
	publishRoot := *root
	if archive != nil && *archive != "" {
		publishRoot = "" // the input is one file; an existing destination is still refused
	}
	return finish(result, err, *out, publishRoot, stdout, stderr)
}

// runSchema handles `schema check --input FILE` and `schema diff --before FILE --after FILE`.
// Results name inputs by base name only; roles and hashes keep same-named inputs apart.
func runSchema(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "check" && args[0] != "diff") {
		fmt.Fprintln(stderr, "tsgk: USAGE: tsgk schema check --input FILE | tsgk schema diff --before FILE --after FILE [--out PATH]")
		return exitUsage
	}
	sub := args[0]
	fs := flag.NewFlagSet("schema "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "new result file (no clobber)")
	paths := map[string]*string{}
	names := []string{"input"}
	if sub == "diff" {
		names = []string{"before", "after"}
	}
	for _, n := range names {
		paths[n] = fs.String(n, "", "node-types.json file")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "tsgk: USAGE: unexpected arguments")
		return exitUsage
	}
	for _, n := range names {
		if *paths[n] == "" {
			fmt.Fprintf(stderr, "tsgk: USAGE: schema %s requires --%s FILE\n", sub, n)
			return exitUsage
		}
	}
	inputs := make([]kit.SchemaInput, len(names))
	for i, n := range names {
		data, err := readDocument(*paths[n])
		if err != nil {
			fmt.Fprintf(stderr, "tsgk: SCHEMA_UNREADABLE: %v\n", err)
			return exitIO
		}
		inputs[i] = kit.SchemaInput{Name: filepath.Base(*paths[n]), Data: data}
	}
	limits := kit.DefaultSchemaLimits()
	cctx, cancel := context.WithTimeout(ctx, limits.Wall+5*time.Second)
	defer cancel()
	var result any
	var err error
	if sub == "check" {
		result, err = kit.SchemaCheck(cctx, kit.SchemaCheckRequest{Input: inputs[0], Limits: limits})
	} else {
		result, err = kit.SchemaDiff(cctx, kit.SchemaDiffRequest{Baseline: inputs[0], Candidate: inputs[1], Limits: limits})
	}
	return finish(result, err, *out, "", stdout, stderr)
}

// runReproduce runs one tsgk-reproduce/r1 profile through the runner. It is the only
// command that starts a process, and only with --allow EXEC_GENERATOR.
func runReproduce(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reproduce", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "source root (read only)")
	profile := fs.String("profile", "", "tsgk-reproduce/r1 profile outside the root")
	out := fs.String("out", "", "new result directory outside the root (no clobber)")
	work := fs.String("work", "", "existing caller-owned directory for the two workspaces, outside the root")
	cgroup := fs.String("cgroup-parent", "", "delegated cgroup v2 directory (Linux hard memory cap)")
	var tools, allow multi
	fs.Var(&tools, "tool", "NAME=PATH executable for a profile tool (repeatable)")
	fs.Var(&allow, "allow", "granted capability (EXEC_GENERATOR)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 || *root == "" || *profile == "" || *out == "" || *work == "" {
		fmt.Fprintln(stderr, "tsgk: USAGE: tsgk reproduce --root PATH --profile FILE --out DIR --work DIR --tool NAME=PATH... --allow EXEC_GENERATOR")
		return exitUsage
	}
	paths := map[string]string{}
	for _, t := range tools {
		name, p, ok := strings.Cut(t, "=")
		if !ok || name == "" || p == "" {
			fmt.Fprintln(stderr, "tsgk: USAGE: --tool NAME=PATH")
			return exitUsage
		}
		paths[name] = p
	}
	abs := map[string]string{}
	for name, p := range map[string]string{"root": *root, "out": *out, "work": *work} {
		a, err := filepath.Abs(p)
		if err != nil {
			fmt.Fprintf(stderr, "tsgk: USAGE: --%s: %v\n", name, err)
			return exitUsage
		}
		abs[name] = a
	}
	// Workspaces, outputs and the trust document stay outside the subject root.
	for name, code := range map[string]string{"work": "WORK_INSIDE_INPUT", "out": "OUTPUT_INSIDE_INPUT"} {
		dir := abs[name]
		if name == "out" {
			dir = filepath.Dir(dir)
		}
		real, err := filepath.EvalSymlinks(dir)
		if err != nil || unresolvedAlias(real) || inside(real, abs["root"]) {
			fmt.Fprintf(stderr, "tsgk: %s: --%s는 검증 대상 root 밖의 확인 가능한 경로여야 한다\n", code, name)
			return exitUsage
		}
	}
	if documentInside(*profile, abs["root"]) {
		fmt.Fprintln(stderr, "tsgk: PROFILE_INSIDE_INPUT: 신뢰 문서는 검증 대상 root 밖에 있어야 한다")
		return exitUsage
	}
	data, err := readDocument(*profile)
	if err != nil {
		fmt.Fprintf(stderr, "tsgk: PROFILE_UNREADABLE: %v\n", err)
		return exitIO
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Duration(kit.GenWallSeconds)*time.Second+time.Minute)
	defer cancel()
	res, rerr := reproduce.Reproduce(cctx, reproduce.Request{Root: abs["root"], Profile: data, Tools: paths, Work: abs["work"], Out: abs["out"], Allow: allow, CgroupParent: *cgroup})
	stdout.Write(reproduce.Encode(res))
	if rerr != nil {
		var re *reproduce.Error
		if errors.As(rerr, &re) {
			fmt.Fprintf(stderr, "tsgk: %s: %s\n", re.Kind, re.Code)
			return exitFor(re.Kind)
		}
		return exitIO
	}
	switch res.ExecutionStatus {
	case kit.StatusCompleted:
		return assessExit(res.Assessment)
	case kit.StatusCancelled:
		return exitCanceled
	case kit.StatusResourceLimit:
		return exitBlocked
	}
	return exitFail
}

// finish renders the API result as one JSON line, maps the exit code and publishes --out.
func finish(result any, err error, out, publishRoot string, stdout, stderr io.Writer) int {
	data, merr := json.Marshal(result)
	if merr != nil {
		fmt.Fprintln(stderr, "tsgk: ENCODE_FAILED")
		return exitIO
	}
	data = append(data, '\n')
	if err != nil {
		// A failure report goes to stdout only; --out receives complete results only.
		stdout.Write(data)
		var ke *kit.Error
		if errors.As(err, &ke) {
			fmt.Fprintf(stderr, "tsgk: %s: %s %s\n", ke.Kind, ke.Code, printable(ke.Path))
			return exitFor(ke.Kind)
		}
		fmt.Fprintln(stderr, "tsgk: IO:", err)
		return exitIO
	}
	// A completed comparison or check that found differences or violations is a complete report.
	code := exitOK
	switch v := result.(type) {
	case kit.VerifyResult:
		code = assessExit(v.Assessment)
	case kit.SchemaCheckResult:
		code = assessExit(v.Assessment)
	case kit.SchemaDiffResult:
		code = assessExit(v.Assessment)
	}
	if out == "" {
		if _, werr := stdout.Write(data); werr != nil {
			fmt.Fprintln(stderr, "tsgk: STDOUT_FAILED: 완전한 report를 쓰지 못했다")
			return exitIO
		}
		return code
	}
	if pcode, perr := publish(out, publishRoot, data); perr != nil {
		fmt.Fprintf(stderr, "tsgk: %s: 분석은 끝났지만 결과 publication이 실패했다: %v\n", pcode, perr)
		return exitIO
	}
	return code
}

func assessExit(assessment string) int {
	switch assessment {
	case kit.AssessFail:
		return exitFail
	case kit.AssessBlocked:
		return exitBlocked
	}
	return exitOK
}

// printable quotes a diagnostic path that could carry terminal control bytes (archive
// member names and JSON keys come from untrusted input); the JSON report escapes them.
func printable(p string) string {
	if !utf8.ValidString(p) || strings.ContainsFunc(p, unicode.IsControl) {
		return strconv.QuoteToASCII(p)
	}
	return p
}

// readDocument reads at most MaxDocumentBytes+1 bytes so the API, not the CLI, rejects an oversized
// document with the same DOCUMENT_BYTES_LIMIT a direct caller gets.
func readDocument(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, kit.MaxDocumentBytes+1))
}

// documentInside reports whether a trust document's directory is the root or below it.
func documentInside(doc, root string) bool {
	abs, err := filepath.Abs(doc)
	if err != nil {
		return false // readDocument reports the unreadable path
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return false
	}
	return inside(dir, root)
}

func exitFor(kind string) int {
	switch kind {
	case kit.KindInvalidInput:
		return exitUsage
	case kit.KindResourceLimit, kit.KindUnsupported:
		return exitBlocked
	case kit.KindCancelled:
		return exitCanceled
	}
	return exitIO
}

// publish writes data to a new path outside the input root: it completes a temporary
// file in the destination directory, then links it into place, which fails instead of
// replacing an existing (or concurrently created) destination.
func publish(out, root string, data []byte) (string, error) {
	abs, err := filepath.Abs(out)
	if err != nil {
		return "OUTPUT_INVALID", err
	}
	if _, err := os.Lstat(abs); err == nil {
		return "OUTPUT_EXISTS", errors.New("destination exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "OUTPUT_INVALID", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "OUTPUT_PARENT_INVALID", err
	}
	final := filepath.Join(parent, filepath.Base(abs))
	if unresolvedAlias(parent) {
		return "OUTPUT_PARENT_ALIAS", errors.New("destination path keeps a junction or mount point that cannot be compared with the root")
	}
	if inside(parent, root) {
		return "OUTPUT_INSIDE_INPUT", errors.New("destination is inside the input root")
	}
	tmp, err := os.CreateTemp(parent, ".tsgk-out-*.tmp")
	if err != nil {
		return "OUTPUT_PUBLISH_FAILED", err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "OUTPUT_PUBLISH_FAILED", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "OUTPUT_PUBLISH_FAILED", err
	}
	if err := tmp.Close(); err != nil {
		return "OUTPUT_PUBLISH_FAILED", err
	}
	if testHookBeforeLink != nil {
		testHookBeforeLink()
	}
	if err := os.Link(name, final); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "OUTPUT_EXISTS", err
		}
		return "OUTPUT_PUBLISH_FAILED", err
	}
	return "", nil
}

// testHookBeforeLink lets tests create a concurrent destination before the final link.
var testHookBeforeLink func()

// inside reports whether dir is the root or below it, comparing file identities of dir
// and each of its ancestors with the root, so case, symlink and junction aliases match.
func inside(dir, root string) bool {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return false
	}
	for p := dir; ; {
		if info, err := os.Stat(p); err == nil && os.SameFile(info, rootInfo) {
			return true
		}
		up := filepath.Dir(p)
		if up == p {
			return false
		}
		p = up
	}
}

// unresolvedAlias reports a directory component that EvalSymlinks left in place (Windows
// junctions and volume mount points); its target cannot be compared with the root.
// ponytail: path-component check; subst drives and bind mounts are not detected.
func unresolvedAlias(dir string) bool {
	for p := dir; ; {
		if info, err := os.Lstat(p); err == nil && isAlias(p, info) {
			return true
		}
		up := filepath.Dir(p)
		if up == p {
			return false
		}
		p = up
	}
}

// cutLast splits PATH=VALUE at the last '=': portable paths may contain '=', values do not.
func cutLast(s string) (string, string, bool) {
	i := strings.LastIndex(s, "=")
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}
