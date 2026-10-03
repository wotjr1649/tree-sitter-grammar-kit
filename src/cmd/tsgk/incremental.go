package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// runIncremental builds the native driver for a tsgk-incremental/r1 profile and runs its
// cases through the S04 runner (S05). Only this command and reproduce start processes.
func runIncremental(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("incremental", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "case root (read only)")
	groot := fs.String("grammar-root", "", "grammar file root (read only; default --root)")
	profile := fs.String("profile", "", "tsgk-incremental/r1 profile outside the root")
	rt := fs.String("runtime", "", "pinned Tree-sitter runtime source directory")
	out := fs.String("out", "", "new result directory outside the root (no clobber)")
	work := fs.String("work", "", "existing caller-owned build directory outside the root")
	cgroup := fs.String("cgroup-parent", "", "delegated cgroup v2 directory (Linux hard memory cap)")
	var tools, allow multi
	fs.Var(&tools, "tool", "cc=PATH compiler executable")
	fs.Var(&allow, "allow", "granted capability (BUILD_NATIVE, EXEC_NATIVE)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	usage := "tsgk: USAGE: tsgk incremental --root PATH [--grammar-root PATH] --profile FILE --runtime DIR --tool cc=PATH --work DIR --out DIR --allow BUILD_NATIVE --allow EXEC_NATIVE"
	if fs.NArg() != 0 || *root == "" || *profile == "" || *rt == "" || *out == "" || *work == "" || len(tools) != 1 {
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	name, cc, ok := strings.Cut(tools[0], "=")
	if !ok || name != "cc" || cc == "" {
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	abs := map[string]string{}
	if *groot == "" {
		*groot = *root
	}
	for k, p := range map[string]string{"root": *root, "grammar-root": *groot, "out": *out, "work": *work, "runtime": *rt, "cc": cc} {
		a, err := filepath.Abs(p)
		if err != nil {
			fmt.Fprintf(stderr, "tsgk: USAGE: --%s: %v\n", k, err)
			return exitUsage
		}
		abs[k] = a
	}
	for k, code := range map[string]string{"work": "WORK_INSIDE_INPUT", "out": "OUTPUT_INSIDE_INPUT"} {
		dir := abs[k]
		if k == "out" {
			dir = filepath.Dir(dir)
		}
		real, err := filepath.EvalSymlinks(dir)
		if err != nil || unresolvedAlias(real) || inside(real, abs["root"]) || inside(real, abs["grammar-root"]) {
			fmt.Fprintf(stderr, "tsgk: %s: --%s는 검증 대상 root 밖의 확인 가능한 경로여야 한다\n", code, k)
			return exitUsage
		}
	}
	if documentInside(*profile, abs["root"]) || documentInside(*profile, abs["grammar-root"]) {
		fmt.Fprintln(stderr, "tsgk: PROFILE_INSIDE_INPUT: 신뢰 문서는 검증 대상 root 밖에 있어야 한다")
		return exitUsage
	}
	data, err := readDocument(*profile)
	if err != nil {
		fmt.Fprintf(stderr, "tsgk: PROFILE_UNREADABLE: %v\n", err)
		return exitIO
	}
	res, rerr := native.Incremental(ctx, native.IncrementalRequest{Root: abs["root"], GrammarRoot: abs["grammar-root"], Profile: data, Runtime: abs["runtime"], Compiler: abs["cc"],
		Work: abs["work"], Out: abs["out"], Allow: allow, CgroupParent: *cgroup})
	line, merr := json.Marshal(res)
	if merr != nil {
		fmt.Fprintln(stderr, "tsgk: ENCODE_FAILED")
		return exitIO
	}
	stdout.Write(append(line, '\n'))
	if rerr != nil {
		var ne *native.Error
		if errors.As(rerr, &ne) {
			fmt.Fprintf(stderr, "tsgk: %s: %s\n", ne.Kind, ne.Code)
			return exitFor(ne.Kind)
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
	case kit.StatusNotRun:
		return exitUsage
	}
	return exitIO
}
