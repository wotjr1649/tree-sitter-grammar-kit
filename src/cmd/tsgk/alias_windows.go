package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
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

// rootAlias queries an irregular entry through its parent handle, without a
// pathname fallback or following a name-surrogate reparse point.
func rootAlias(root *os.Root, name string, info os.FileInfo) (bool, error) {
	if !filepath.IsLocal(name) || filepath.Base(name) != name || name == "." || strings.ContainsAny(name, ":/\\") {
		return false, os.ErrInvalid
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return true, nil
	}
	if info.Mode()&os.ModeIrregular == 0 {
		return false, nil
	}
	parent, err := root.Open(".")
	if err != nil {
		return false, err
	}
	defer parent.Close()
	u, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return false, err
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(parent.Fd()), ObjectName: u}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h windows.Handle
	err = windows.NtCreateFile(&h, windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE, &oa, &windows.IO_STATUS_BLOCK{}, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN,
		windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return false, err
	}
	f := os.NewFile(uintptr(h), name)
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return false, errors.New("input entry identity is unavailable or changed")
	}
	var tag struct{ Attributes, ReparseTag uint32 }
	if err := windows.GetFileInformationByHandleEx(h, windows.FileAttributeTagInfo, (*byte)(unsafe.Pointer(&tag)), uint32(unsafe.Sizeof(tag))); err != nil {
		return false, err
	}
	return tag.ReparseTag&0x20000000 != 0, nil
}
