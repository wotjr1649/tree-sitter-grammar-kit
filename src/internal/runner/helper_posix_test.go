//go:build linux || darwin

package runner

import (
	"os/exec"
	"syscall"
)

func detachAttr(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// alive is an independent liveness check outside the runner backend; a zombie whose
// parent has not reaped it yet still answers signal 0, so callers allow for reaping time.
func alive(pid int) bool { return syscall.Kill(pid, 0) == nil && !zombie(pid) }

func killPID(pid int) { syscall.Kill(pid, syscall.SIGKILL) }
