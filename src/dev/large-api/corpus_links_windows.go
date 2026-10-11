package main

import (
	"errors"
	"os"
	"syscall"
)

func corpusCheckLinks(f *os.File) error {
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info); err != nil {
		return err
	}
	if info.NumberOfLinks > 1 {
		return errors.New("CORPUS_HARDLINK_REJECTED")
	}
	return nil
}
