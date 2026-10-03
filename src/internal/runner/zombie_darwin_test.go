package runner

import "golang.org/x/sys/unix"

func zombie(pid int) bool {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	return err != nil || p.Proc.P_stat == sZomb
}
