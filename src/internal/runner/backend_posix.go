//go:build linux || darwin

package runner

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// groupTree runs the process as the leader of a new process group and signals the group.
// A descendant that calls setsid or setpgid leaves the group and is not reached; the
// capability says so (EscapedDescendants=NOT_CONTAINED) unless a cgroup holds the tree.
type groupTree struct {
	pgid    int
	members func(pgid int) (int, error)
	rss     func(pgid int) (uint64, error)
}

func (g *groupTree) prepare(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

func (g *groupTree) attach(p *os.Process) error {
	g.pgid = p.Pid
	return nil
}

func (g *groupTree) stop(force bool) {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	syscall.Kill(-g.pgid, sig)
}

func (g *groupTree) live() (int, error)      { return g.members(g.pgid) }
func (g *groupTree) limitHit() bool          { return false }
func (g *groupTree) peak() (uint64, bool)    { return 0, false }
func (g *groupTree) notify() <-chan struct{} { return nil }
func (g *groupTree) close() error            { return nil }

func (g *groupTree) sample() (uint64, error) {
	if g.rss == nil {
		return 0, errors.New("no sampler")
	}
	return g.rss(g.pgid)
}
