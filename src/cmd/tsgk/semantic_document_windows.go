//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func openQualifySemanticRegular(root *os.Root, name string) (*os.File, os.FileInfo, error) {
	if !filepath.IsLocal(name) || filepath.Base(name) != name || name == "." || name == ".." || strings.ContainsAny(name, ":/\\") {
		return nil, nil, os.ErrInvalid
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	alias, err := rootAlias(root, name, before)
	if err != nil || alias {
		return nil, nil, errors.New("document leaf is a link or alias")
	}
	parent, err := root.Open(".")
	if err != nil {
		return nil, nil, err
	}
	defer parent.Close()
	u, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, nil, err
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(parent.Fd()), ObjectName: u}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h windows.Handle
	err = windows.NtCreateFile(&h, windows.FILE_READ_DATA|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE, &oa,
		&windows.IO_STATUS_BLOCK{}, nil, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN, windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_NON_DIRECTORY_FILE, 0, 0)
	if err != nil {
		return nil, nil, err
	}
	f := os.NewFile(uintptr(h), name)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) {
		f.Close()
		return nil, nil, errors.New("opened document is not the same regular file")
	}
	var tag struct{ Attributes, ReparseTag uint32 }
	if err := windows.GetFileInformationByHandleEx(h, windows.FileAttributeTagInfo, (*byte)(unsafe.Pointer(&tag)), uint32(unsafe.Sizeof(tag))); err != nil || tag.Attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		f.Close()
		return nil, nil, errors.New("document leaf is a reparse point")
	}
	var links windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &links); err != nil || links.NumberOfLinks != 1 {
		f.Close()
		return nil, nil, errors.New("document hard-link count is not one")
	}
	after, err := root.Lstat(name)
	if err != nil || !os.SameFile(info, after) {
		f.Close()
		return nil, nil, errors.New("document leaf identity changed while opening")
	}
	alias, err = rootAlias(root, name, after)
	if err != nil || alias {
		f.Close()
		return nil, nil, errors.New("document leaf is a link or alias")
	}
	return f, info, nil
}
