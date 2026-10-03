package runner

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
)

// procGroup counts live (non-zombie) processes whose process group is pgid.
func procGroup(pgid int) (int, error) {
	n := 0
	err := scanProc(func(state byte, pg int, _ int) {
		if pg == pgid && state != 'Z' && state != 'X' {
			n++
		}
	})
	return n, err
}

// procGroupRSS sums resident memory (bytes, /proc/PID/statm) of the group's members.
func procGroupRSS(pgid int) (uint64, error) {
	var total uint64
	err := scanProc(func(state byte, pg int, pid int) {
		if pg != pgid || state == 'Z' {
			return
		}
		data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/statm")
		if err != nil {
			return // the process left between the scan and the read
		}
		f := strings.Fields(string(data))
		if len(f) > 1 {
			if pages, err := strconv.ParseUint(f[1], 10, 64); err == nil {
				total += pages * uint64(os.Getpagesize())
			}
		}
	})
	return total, err
}

func scanProc(each func(state byte, pgid, pid int)) error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		// pid (comm) state ppid pgrp ...; comm may hold spaces and parentheses.
		i := bytes.LastIndexByte(data, ')')
		if i < 0 {
			continue
		}
		f := strings.Fields(string(data[i+1:]))
		if len(f) < 3 || len(f[0]) != 1 {
			continue
		}
		pg, err := strconv.Atoi(f[2])
		if err != nil {
			continue
		}
		each(f[0][0], pg, pid)
	}
	return nil
}

var leafSeq atomic.Uint64

// cgroupTree starts the process directly inside a new cgroup v2 leaf below a delegated
// parent (clone3 CLONE_INTO_CGROUP). memory.max is the kernel hard cap with
// memory.oom.group, cgroup.kill ends every member including escaped descendants, and
// cgroup.procs verifies emptiness. The process is also a process-group leader so the
// polite SIGTERM reaches the group first.
type cgroupTree struct {
	groupTree
	dir string
	fd  *os.File
}

func newTree(s *Spec, probe bool) (tree, Capabilities, error) {
	caps := baseCaps()
	caps.PoliteTermination = true
	if s.CgroupParent == "" {
		caps.Backend, caps.TreeCleanup, caps.EscapedDescendants = "posix-process-group", "PROCESS_GROUP", NotContained
		caps.Memory, caps.MemoryMetric = MemorySampled, "process-group resident set (/proc/PID/statm, bytes, sampled)"
		return &groupTree{members: procGroup, rss: procGroupRSS}, caps, nil
	}
	caps.Backend, caps.TreeCleanup, caps.EscapedDescendants = "linux-cgroup-v2", "CGROUP_KILL", Contained
	caps.Memory, caps.MemoryMetric = MemoryHard, "cgroup v2 memory.max/memory.peak (bytes, includes page cache)"
	if err := checkCgroupParent(s.CgroupParent); err != nil {
		caps.Memory = MemoryUnsupported
		return nil, caps, &Error{Code: "CGROUP_UNAVAILABLE", Blocked: true, Cause: err}
	}
	if probe {
		return nil, caps, nil
	}
	dir := filepath.Join(s.CgroupParent, fmt.Sprintf("tsgk-%d-%d", os.Getpid(), leafSeq.Add(1)))
	if err := os.Mkdir(dir, 0o755); err != nil {
		return nil, caps, &Error{Code: "CGROUP_UNAVAILABLE", Blocked: true, Cause: err}
	}
	t := &cgroupTree{groupTree: groupTree{members: procGroup}, dir: dir}
	writes := [][2]string{{"memory.max", strconv.FormatUint(s.Memory.Bytes, 10)}, {"memory.oom.group", "1"}}
	if _, err := os.Stat(filepath.Join(dir, "memory.swap.max")); err == nil {
		writes = append(writes, [2]string{"memory.swap.max", "0"})
	}
	for _, w := range writes {
		if err := os.WriteFile(filepath.Join(dir, w[0]), []byte(w[1]), 0); err != nil {
			os.Remove(dir)
			return nil, caps, &Error{Code: "CGROUP_UNAVAILABLE", Blocked: true, Cause: fmt.Errorf("%s: %w", w[0], err)}
		}
	}
	fd, err := os.Open(dir)
	if err != nil {
		os.Remove(dir)
		return nil, caps, &Error{Code: "CGROUP_UNAVAILABLE", Blocked: true, Cause: err}
	}
	t.fd = fd
	return t, caps, nil
}

// checkCgroupParent requires a cgroup v2 directory whose children already get the memory
// controller; the runner never changes the delegated parent itself.
func checkCgroupParent(parent string) error {
	if !filepath.IsAbs(parent) {
		return errors.New("cgroup parent must be absolute")
	}
	ctl, err := os.ReadFile(filepath.Join(parent, "cgroup.subtree_control"))
	if err != nil {
		return err
	}
	if !strings.Contains(" "+strings.TrimSpace(string(ctl))+" ", " memory ") {
		return errors.New("memory controller not enabled in the delegated parent's cgroup.subtree_control")
	}
	return nil
}

func (c *cgroupTree) prepare(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, UseCgroupFD: true, CgroupFD: int(c.fd.Fd())}
	return nil
}

func (c *cgroupTree) stop(force bool) {
	c.groupTree.stop(force)
	if force {
		os.WriteFile(filepath.Join(c.dir, "cgroup.kill"), []byte("1"), 0)
	}
}

func (c *cgroupTree) live() (int, error) {
	data, err := os.ReadFile(filepath.Join(c.dir, "cgroup.procs"))
	if err != nil {
		return 0, err
	}
	return len(strings.Fields(string(data))), nil
}

func (c *cgroupTree) limitHit() bool {
	data, err := os.ReadFile(filepath.Join(c.dir, "memory.events"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) == 2 && (f[0] == "oom_kill" || f[0] == "oom_group_kill") && f[1] != "0" {
			return true
		}
	}
	return false
}

func (c *cgroupTree) peak() (uint64, bool) {
	data, err := os.ReadFile(filepath.Join(c.dir, "memory.peak"))
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return v, err == nil
}

func (c *cgroupTree) sample() (uint64, error) { return 0, errors.New("hard cap, not sampled") }

func (c *cgroupTree) close() error {
	c.fd.Close()
	return os.Remove(c.dir)
}
