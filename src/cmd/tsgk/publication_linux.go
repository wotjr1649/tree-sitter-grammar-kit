package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

var testHookBeforeOutputWrite func(*os.File)

func publishOutput(destination *os.Root, final string, data []byte) (code string, resultErr error) {
	if !filepath.IsLocal(final) || filepath.Base(final) != final || final == "." {
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
	// An unnamed inode has no source basename that another process can replace.
	fd, err := unix.Openat(int(parent.Fd()), ".", unix.O_TMPFILE|unix.O_WRONLY|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return "OUTPUT_PUBLISH_FAILED", err
	}
	tmp := os.NewFile(uintptr(fd), "publication-temp")
	defer func() {
		if err := tmp.Close(); err != nil {
			if published {
				resultErr = errors.Join(resultErr, fmt.Errorf("output was published; file handle close failed: %w", err))
				code = "OUTPUT_CLOSE_FAILED"
			} else {
				resultErr = errors.Join(resultErr, err)
				code = "OUTPUT_CLEANUP_FAILED"
			}
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
	if err := unix.Linkat(int(tmp.Fd()), "", int(parent.Fd()), final, unix.AT_EMPTY_PATH); err != nil {
		if errors.Is(err, unix.EEXIST) {
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
