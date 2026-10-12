//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"
)

func corpusCheckLinks(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Nlink > 1 {
		return errors.New("CORPUS_LINK_INFORMATION_UNSUPPORTED_OR_HARDLINK")
	}
	return nil
}
