//go:build !windows

package kit

import "syscall"

func mkfifo(path string) error { return syscall.Mkfifo(path, 0o644) }
