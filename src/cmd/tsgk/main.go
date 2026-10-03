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
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const (
	exitOK       = 0
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
var future = map[string]string{"verify": "S02", "schema": "S03", "reproduce": "S04", "incremental": "S05", "oracle": "S06", "replay": "S07", "evidence": "S07", "parity": "S08"}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "tsgk: USAGE: tsgk <inspect|identity|corpus> --root PATH [--out PATH]")
		return exitUsage
	}
	cmd := args[0]
	if owner, ok := future[cmd]; ok {
		fmt.Fprintf(stderr, "tsgk: UNSUPPORTED_COMMAND: %s는 %s 범위이며 이 build에서 구현되지 않았다\n", cmd, owner)
		return exitUsage
	}
	if cmd != "inspect" && cmd != "identity" && cmd != "corpus" {
		fmt.Fprintf(stderr, "tsgk: UNKNOWN_COMMAND: %s\n", cmd)
		return exitUsage
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "snapshot root")
	out := fs.String("out", "", "new result file outside the root (no clobber)")
	profile := fs.String("profile", "", "strict profile (S02)")
	var grammar, encProfile, large *string
	var files, declares multi
	if cmd != "corpus" {
		grammar = fs.String("grammar", ".", `grammar directory ("." is the root)`)
	}
	if cmd == "identity" {
		fs.Var(&files, "file", "explicit selection PATH=ROLE (repeatable)")
		large = fs.String("large-file-profile", "", "registered identity-scoped exception")
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
	if *profile != "" {
		fmt.Fprintln(stderr, "tsgk: PROFILE_UNSUPPORTED: strict profile 해석은 S02 범위이며 이 build는 profile을 읽지 않는다")
		return exitUsage
	}
	var enc kit.EncodingPolicy
	if encProfile != nil {
		switch *encProfile {
		case "cp949":
			enc.Profile = "cp949"
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
	var result any
	var err error
	var wall time.Duration
	switch cmd {
	case "inspect", "identity":
		limits := kit.DefaultLimits()
		wall = limits.Wall
		sel := kit.Selection{Grammar: *grammar}
		for _, f := range files {
			p, role, ok := cutLast(f)
			if !ok {
				fmt.Fprintln(stderr, "tsgk: USAGE: --file PATH=ROLE")
				return exitUsage
			}
			sel.Files = append(sel.Files, kit.FileSelection{Path: p, Role: role})
		}
		cctx, cancel := context.WithTimeout(ctx, wall+5*time.Second)
		defer cancel()
		if cmd == "inspect" {
			result, err = kit.Inspect(cctx, kit.InspectRequest{Root: *root, Selection: sel, Limits: limits})
		} else {
			result, err = kit.Identity(cctx, kit.IdentityRequest{Root: *root, Selection: sel, Limits: limits, Encoding: enc, LargeFileProfile: *large})
		}
	case "corpus":
		limits := kit.DefaultCorpusLimits()
		cctx, cancel := context.WithTimeout(ctx, limits.Wall+5*time.Second)
		defer cancel()
		result, err = kit.Corpus(cctx, kit.CorpusRequest{Root: *root, Limits: limits, Encoding: enc})
	}
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
			fmt.Fprintf(stderr, "tsgk: %s: %s %s\n", ke.Kind, ke.Code, ke.Path)
			return exitFor(ke.Kind)
		}
		fmt.Fprintln(stderr, "tsgk: IO:", err)
		return exitIO
	}
	if *out == "" {
		if _, werr := stdout.Write(data); werr != nil {
			fmt.Fprintln(stderr, "tsgk: STDOUT_FAILED: 완전한 report를 쓰지 못했다")
			return exitIO
		}
		return exitOK
	}
	if code, perr := publish(*out, *root, data); perr != nil {
		fmt.Fprintf(stderr, "tsgk: %s: 분석은 끝났지만 결과 publication이 실패했다: %v\n", code, perr)
		return exitIO
	}
	return exitOK
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
func unresolvedAlias(dir string) bool {
	for p := dir; ; {
		if info, err := os.Lstat(p); err == nil && info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
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
