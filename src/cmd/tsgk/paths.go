package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	pathEntries = 10000
	pathDepth   = 64
	pathParents = 256
	pathWall    = 10 * time.Second
)

// inside compares directory identities throughout root, including descendants that
// can be reached through a subst drive or bind mount outside the root's pathname.
func inside(dir, root string) (bool, error) {
	if root == "" {
		return false, nil // file-only commands have no directory subject
	}
	deadline := time.Now().Add(pathWall)
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false, err
	}
	rf, err := os.Open(real)
	if err != nil {
		return false, err
	}
	defer rf.Close()
	rinfo, err := rf.Stat()
	if err != nil || !rinfo.IsDir() {
		return false, errors.New("input directory identity is unavailable")
	}
	var parents []os.FileInfo
	for p := dir; ; {
		if len(parents) == pathParents || time.Now().After(deadline) {
			return false, errors.New("directory identity check limit exceeded")
		}
		f, err := os.Open(p)
		if err != nil {
			return false, err
		}
		// File.Stat obtains the identity on Windows now; path-based Stat may defer
		// acquisition until SameFile and then hide an acquisition error as false.
		info, err := f.Stat()
		cerr := f.Close()
		if err != nil || cerr != nil || !info.IsDir() {
			return false, errors.New("destination directory identity is unavailable")
		}
		if time.Now().After(deadline) {
			return false, errors.New("directory identity check limit exceeded")
		}
		if os.SameFile(info, rinfo) {
			return true, nil
		}
		parents = append(parents, info)
		up := filepath.Dir(p)
		if up == p {
			break
		}
		p = up
	}
	entries := 1 // the root directory
	return containsDirectory(rf, real, parents, 0, &entries, deadline)
}

// containsDirectory never reads file contents or follows an internal symlink/reparse alias.
// The deadline is cooperative; synchronous filesystem calls are not preempted.
func containsDirectory(f *os.File, path string, parents []os.FileInfo, depth int, entries *int, deadline time.Time) (bool, error) {
	if depth > pathDepth || time.Now().After(deadline) {
		return false, errors.New("directory identity check limit exceeded")
	}
	info, err := f.Stat()
	if err != nil || !info.IsDir() {
		return false, errors.New("input directory identity is unavailable")
	}
	if time.Now().After(deadline) {
		return false, errors.New("directory identity check limit exceeded")
	}
	for _, parent := range parents {
		if os.SameFile(info, parent) {
			return true, nil
		}
	}
	for {
		if time.Now().After(deadline) {
			return false, errors.New("directory identity check limit exceeded")
		}
		batch, err := f.ReadDir(128)
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		if time.Now().After(deadline) {
			return false, errors.New("directory identity check limit exceeded")
		}
		for _, entry := range batch {
			*entries += 1
			if *entries > pathEntries || time.Now().After(deadline) {
				return false, errors.New("directory identity check limit exceeded")
			}
			name := filepath.Join(path, entry.Name())
			info, err := os.Lstat(name)
			if err != nil {
				return false, err
			}
			if isAlias(name, info) {
				return false, errors.New("input contains an unresolved path alias")
			}
			if !info.IsDir() {
				continue
			}
			child, err := os.Open(name)
			if err != nil {
				return false, err
			}
			found, err := containsDirectory(child, name, parents, depth+1, entries, deadline)
			cerr := child.Close()
			if err != nil {
				return false, err
			}
			if cerr != nil {
				return false, cerr
			}
			if found {
				return true, nil
			}
		}
		if errors.Is(err, io.EOF) {
			return false, nil
		}
	}
}
