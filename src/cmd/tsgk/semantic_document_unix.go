//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func openQualifySemanticRegular(root *os.Root, name string) (*os.File, os.FileInfo, error) {
	if !filepath.IsLocal(name) || filepath.Base(name) != name || name == "." || name == ".." {
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
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) {
		f.Close()
		return nil, nil, errors.New("opened document is not the same regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
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
