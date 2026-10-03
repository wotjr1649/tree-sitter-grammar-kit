package main

import (
	"os"
	"syscall"
)

// isAlias reports symlinks and mount points (junctions, volume mounts) by reparse tag;
// other directory reparse points such as cloud placeholders are not path aliases.
func isAlias(p string, info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	if info.Mode()&os.ModeIrregular == 0 {
		return false
	}
	name, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return true
	}
	var d syscall.Win32finddata
	h, err := syscall.FindFirstFile(name, &d)
	if err != nil {
		return true // unknown reparse point: refuse rather than guess
	}
	syscall.FindClose(h)
	const mountPoint, symlink = 0xA0000003, 0xA000000C
	return d.Reserved0 == mountPoint || d.Reserved0 == symlink
}
