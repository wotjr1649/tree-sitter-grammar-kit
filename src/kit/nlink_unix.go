//go:build !windows

package kit

import (
	"os"
	"syscall"
)

// checkLinks rejects a regular file with more than one hard link (r0 rejects hardlinks).
func checkLinks(f *os.File, name string) *Error {
	info, err := f.Stat()
	if err != nil {
		return fail(KindIO, "READ_FAILED", name, err)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Nlink > 1 {
		return fail(KindInvalidInput, "HARDLINK_REJECTED", name, nil)
	}
	return nil
}
