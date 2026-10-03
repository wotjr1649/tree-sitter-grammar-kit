package kit

import (
	"os"
	"syscall"
)

// checkLinks rejects a regular file with more than one hard link (r0 rejects hardlinks).
func checkLinks(f *os.File, name string) *Error {
	var d syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &d); err != nil {
		return fail(KindIO, "READ_FAILED", name, err)
	}
	if d.NumberOfLinks > 1 {
		return fail(KindInvalidInput, "HARDLINK_REJECTED", name, nil)
	}
	return nil
}
