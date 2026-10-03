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
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// runIncremental builds the native driver for a tsgk-incremental/r1 profile and runs its
// cases through the S04 runner (S05). Only this command, oracle record and reproduce
// start processes.
func runIncremental(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runNative(ctx, "incremental", args, stdout, stderr)
}

// runOracle dispatches `tsgk oracle record` (S06): the same build, runner and protocol as
// incremental, with r2 queries and API observations and a published record set.
func runOracle(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "record" {
		fmt.Fprintln(stderr, "tsgk: USAGE: tsgk oracle record --root PATH [--grammar-root PATH] --profile FILE [--fact-pack FILE] --runtime DIR --tool cc=PATH --work DIR --out DIR --allow BUILD_NATIVE --allow EXEC_NATIVE")
		return exitUsage
	}
	return runNative(ctx, "oracle", args[1:], stdout, stderr)
}

func runNative(ctx context.Context, kind string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(kind, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "case root (read only)")
	groot := fs.String("grammar-root", "", "grammar file root (read only; default --root)")
	profile := fs.String("profile", "", "profile outside the root")
	rt := fs.String("runtime", "", "pinned Tree-sitter runtime source directory")
	out := fs.String("out", "", "new result directory outside the root (no clobber)")
	work := fs.String("work", "", "existing caller-owned build directory outside the root")
	cgroup := fs.String("cgroup-parent", "", "delegated cgroup v2 directory (Linux hard memory cap)")
	runWall := fs.Int("run-wall", 0, "seconds; narrows the operation run wall (0 keeps it)")
	var pack *string
	if kind == "oracle" {
		pack = fs.String("fact-pack", "", "fact query pack the profile binds (outside the root)")
	}
	var tools, allow multi
	fs.Var(&tools, "tool", "cc=PATH compiler executable")
	fs.Var(&allow, "allow", "granted capability (BUILD_NATIVE, EXEC_NATIVE)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	usage := "tsgk: USAGE: tsgk incremental --root PATH [--grammar-root PATH] --profile FILE --runtime DIR --tool cc=PATH --work DIR --out DIR --allow BUILD_NATIVE --allow EXEC_NATIVE"
	if kind == "oracle" {
		usage = "tsgk: USAGE: tsgk oracle record --root PATH [--grammar-root PATH] --profile FILE [--fact-pack FILE] --runtime DIR --tool cc=PATH --work DIR --out DIR --allow BUILD_NATIVE --allow EXEC_NATIVE"
	}
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
	docs := []string{*profile}
	if pack != nil && *pack != "" {
		docs = append(docs, *pack)
	}
	for _, d := range docs {
		if documentInside(d, abs["root"]) || documentInside(d, abs["grammar-root"]) {
			fmt.Fprintln(stderr, "tsgk: PROFILE_INSIDE_INPUT: 신뢰 문서는 검증 대상 root 밖에 있어야 한다")
			return exitUsage
		}
	}
	data, err := readDocument(*profile)
	if err != nil {
		fmt.Fprintf(stderr, "tsgk: PROFILE_UNREADABLE: %v\n", err)
		return exitIO
	}
	wall := time.Duration(max(*runWall, 0)) * time.Second
	var res any
	var status, assessment string
	var rerr error
	if kind == "oracle" {
		var packData []byte
		if *pack != "" {
			if packData, err = readDocument(*pack); err != nil {
				fmt.Fprintf(stderr, "tsgk: PROFILE_UNREADABLE: %v\n", err)
				return exitIO
			}
		}
		r, e := native.Oracle(ctx, native.OracleRequest{Root: abs["root"], GrammarRoot: abs["grammar-root"], Profile: data, FactPack: packData, Runtime: abs["runtime"], Compiler: abs["cc"],
			Work: abs["work"], Out: abs["out"], Allow: allow, CgroupParent: *cgroup, RunWall: wall})
		res, status, assessment, rerr = r, r.ExecutionStatus, r.Assessment, e
	} else {
		r, e := native.Incremental(ctx, native.IncrementalRequest{Root: abs["root"], GrammarRoot: abs["grammar-root"], Profile: data, Runtime: abs["runtime"], Compiler: abs["cc"],
			Work: abs["work"], Out: abs["out"], Allow: allow, CgroupParent: *cgroup, RunWall: wall})
		res, status, assessment, rerr = r, r.ExecutionStatus, r.Assessment, e
	}
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
	switch status {
	case kit.StatusCompleted:
		return assessExit(assessment)
	case kit.StatusCancelled:
		return exitCanceled
	case kit.StatusResourceLimit:
		return exitBlocked
	case kit.StatusNotRun:
		return exitUsage
	}
	return exitIO
}
