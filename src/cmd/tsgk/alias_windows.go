package main

import (
	"os"
	"syscall"
)

// isAlias reports name-surrogate reparse tags (including symlinks and mount points);
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
	return d.Reserved0&0x20000000 != 0 // name-surrogate bit, including unknown redirecting tags
}
