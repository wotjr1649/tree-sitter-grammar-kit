package runner

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const sZomb = 5 // sys/proc.h SZOMB

// darwinGroup lists the live members of a process group through kern.proc.pgrp.
func darwinGroup(pgid int) ([]int, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pgid)
	if err != nil {
		return nil, err
	}
	pids := make([]int, 0, len(procs))
	for _, p := range procs {
		if p.Proc.P_stat != sZomb {
			pids = append(pids, int(p.Proc.P_pid))
		}
	}
	return pids, nil
}

func darwinMembers(pgid int) (int, error) {
	pids, err := darwinGroup(pgid)
	return len(pids), err
}

// darwinRSS sums the members' resident set from the system ps (KiB). macOS exposes no
// per-process hard memory cap to an unprivileged supervisor; this observation drives a
// sampled termination that is labelled non-strict.
// ponytail: one /bin/ps per sample; proc_pid_rusage needs cgo or private trampolines.
func darwinRSS(pgid int) (uint64, error) {
	pids, err := darwinGroup(pgid)
	if err != nil || len(pids) == 0 {
		return 0, err
	}
	list := make([]string, len(pids))
	for i, p := range pids {
		list[i] = strconv.Itoa(p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/ps", "-o", "rss=", "-p", strings.Join(list, ",")).Output()
	if err != nil && len(out) == 0 {
		return 0, err
	}
	var total uint64
	for _, f := range strings.Fields(string(out)) {
		if kib, err := strconv.ParseUint(f, 10, 64); err == nil {
			total += kib * 1024
		}
	}
	return total, nil
}

func newTree(s *Spec, probe bool) (tree, Capabilities, error) {
	caps := baseCaps()
	caps.PoliteTermination = true
	caps.Backend, caps.TreeCleanup, caps.EscapedDescendants = "posix-process-group", "PROCESS_GROUP", NotContained
	caps.Memory, caps.MemoryMetric = MemorySampled, "process-group resident set (/bin/ps rss KiB as bytes, sampled; non-strict)"
	return &groupTree{members: darwinMembers, rss: darwinRSS}, caps, nil
}
