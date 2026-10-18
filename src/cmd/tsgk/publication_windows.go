package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var testHookBeforeOutputWrite func(*os.File)
var testHookBeforeOutputCreate func(string)

func publishOutput(destination *os.Root, final string, data []byte) (code string, resultErr error) {
	stem, _, _ := strings.Cut(final, ".")
	if !filepath.IsLocal(final) || (stem != "" && !filepath.IsLocal(strings.TrimRight(stem, " "))) || filepath.Base(final) != final || final == "." ||
		strings.ContainsAny(final, ":/\\<>\"|?*") || strings.TrimRight(final, ". ") != final || strings.ContainsFunc(final, func(r rune) bool { return r < 32 }) {
		return "OUTPUT_INVALID", os.ErrInvalid
	}
	parent, err := destination.Open(".")
	if err != nil {
		return "OUTPUT_PARENT_INVALID", err
	}
	published := false
	defer func() {
		if err := parent.Close(); err != nil {
			if published {
				err = fmt.Errorf("output was published; parent handle close failed: %w", err)
			}
			resultErr = errors.Join(resultErr, err)
			code = "OUTPUT_CLOSE_FAILED"
		}
	}()
	// An 8.3 name has no separate short alias; disjoint first letters keep the
	// requested final name absent until publication, including on 8.3 volumes.
	prefix := "A"
	if final[0] == 'A' || final[0] == 'a' {
		prefix = "B"
	}
	name := prefix + rand.Text()[:7] + ".TMP"
	if testHookBeforeOutputCreate != nil {
		testHookBeforeOutputCreate(name)
	}
	u, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return "OUTPUT_PUBLISH_FAILED", err
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(parent.Fd()), ObjectName: u, Attributes: windows.OBJ_CASE_INSENSITIVE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.DELETE|windows.FILE_GENERIC_WRITE|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		&oa, &status, nil, windows.FILE_ATTRIBUTE_NORMAL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_CREATE, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT, 0, 0)
	if err != nil {
		return "OUTPUT_PUBLISH_FAILED", err
	}
	tmp := os.NewFile(uintptr(handle), name)
	defer func() {
		if published {
			if err := tmp.Close(); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("output was published; file handle close failed: %w", err))
				code = "OUTPUT_CLOSE_FAILED"
			}
			return
		}
		// The creation handle owns the cleanup; never resolve the temp name again.
		deleteFile := byte(1)
		err := windows.NtSetInformationFile(windows.Handle(tmp.Fd()), &windows.IO_STATUS_BLOCK{}, &deleteFile, 1, windows.FileDispositionInformation)
		err = errors.Join(err, tmp.Close())
		if err != nil {
			resultErr = errors.Join(resultErr, err)
			code = "OUTPUT_CLEANUP_FAILED"
		}
	}()
	if testHookBeforeOutputWrite != nil {
		testHookBeforeOutputWrite(tmp)
	}
	if _, err := tmp.Write(data); err != nil {
		return "OUTPUT_PUBLISH_FAILED", err
	}
	if err := tmp.Sync(); err != nil {
		return "OUTPUT_PUBLISH_FAILED", err
	}
	if testHookBeforeLink != nil {
		testHookBeforeLink()
	}
	type renameInformation struct {
		Replace byte
		Root    windows.Handle
		Length  uint32
		Name    [1]uint16
	}
	var layout renameInformation
	encoded := utf16.Encode([]rune(final))
	buffer := make([]byte, int(unsafe.Sizeof(layout))+2*len(encoded))
	info := (*renameInformation)(unsafe.Pointer(&buffer[0]))
	info.Root, info.Length = windows.Handle(parent.Fd()), uint32(2*len(encoded))
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[unsafe.Offsetof(layout.Name)])), len(encoded)), encoded)
	if err := windows.NtSetInformationFile(windows.Handle(tmp.Fd()), &windows.IO_STATUS_BLOCK{}, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation); err != nil {
		if errors.Is(err, windows.STATUS_OBJECT_NAME_COLLISION) {
			return "OUTPUT_EXISTS", err
		}
		return "OUTPUT_PUBLISH_FAILED", err
	}
	published = true
	if testHookAfterLink != nil {
		testHookAfterLink(tmp)
	}
	return "", nil
}
