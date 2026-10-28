package main

import (
	"context"
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
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false, err
	}
	rf, err := os.OpenRoot(real)
	if err != nil {
		return false, err
	}
	defer rf.Close()
	df, err := os.OpenRoot(dir)
	if err != nil {
		return false, err
	}
	defer df.Close()
	return insideRoots(df, rf)
}

// insideRoots always includes the actual destination handle in the comparison;
// pathname ancestors are additional conservative checks, never its replacement.
func insideRoots(dir, root *os.Root) (bool, error) {
	return insideRootsUntil(dir, root, time.Now().Add(pathWall))
}

// insideRootsUntil keeps the per-check cap and also observes the caller's operation deadline.
func insideRootsUntil(dir, root *os.Root, deadline time.Time) (bool, error) {
	return insideRootsUntilContext(context.Background(), dir, root, deadline)
}

func insideRootsUntilContext(caller context.Context, dir, root *os.Root, deadline time.Time) (bool, error) {
	if err := caller.Err(); err != nil {
		return false, err
	}
	localDeadline := time.Now().Add(pathWall)
	if deadline.Before(localDeadline) {
		localDeadline = deadline
	}
	deadline = localDeadline
	rf, err := root.Open(".")
	if err != nil {
		return false, err
	}
	rinfo, serr := rf.Stat()
	cerr := rf.Close()
	if serr != nil || cerr != nil || !rinfo.IsDir() {
		return false, errors.New("input directory identity is unavailable")
	}
	var parents []os.FileInfo
	for p := dir.Name(); ; {
		if err := caller.Err(); err != nil {
			return false, err
		}
		if len(parents) == pathParents || time.Now().After(deadline) {
			return false, errors.New("directory identity check limit exceeded")
		}
		var f *os.File
		var err error
		if len(parents) == 0 {
			f, err = dir.Open(".")
		} else {
			f, err = os.Open(p)
		}
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
	return containsDirectoryContext(caller, root, parents, 0, &entries, deadline)
}

// containsDirectory never reads file contents; observed aliases and identity changes fail closed.
// The deadline is cooperative; synchronous filesystem calls are not preempted.
func containsDirectory(root *os.Root, parents []os.FileInfo, depth int, entries *int, deadline time.Time, files ...os.FileInfo) (bool, error) {
	return containsDirectoryContext(context.Background(), root, parents, depth, entries, deadline, files...)
}

func containsDirectoryContext(caller context.Context, root *os.Root, parents []os.FileInfo, depth int, entries *int, deadline time.Time, files ...os.FileInfo) (bool, error) {
	if err := caller.Err(); err != nil {
		return false, err
	}
	if depth > pathDepth || time.Now().After(deadline) {
		return false, errors.New("directory identity check limit exceeded")
	}
	f, err := root.Open(".")
	if err != nil {
		return false, err
	}
	defer f.Close()
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
		if err := caller.Err(); err != nil {
			return false, err
		}
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
			if err := caller.Err(); err != nil {
				return false, err
			}
			*entries += 1
			if *entries > pathEntries || time.Now().After(deadline) {
				return false, errors.New("directory identity check limit exceeded")
			}
			name := entry.Name()
			info, err := root.Lstat(name)
			if err != nil {
				return false, err
			}
			alias, err := rootAlias(root, name, info)
			if err != nil {
				return false, err
			}
			if alias {
				return false, errors.New("input contains an unresolved path alias")
			}
			if !info.IsDir() {
				if len(files) != 0 {
					file, opened, err := openQualifySemanticRegular(root, name)
					if err != nil {
						return false, err
					}
					if err := file.Close(); err != nil {
						return false, err
					}
					for _, protected := range files {
						if os.SameFile(opened, protected) {
							return true, nil
						}
					}
				}
				continue
			}
			if testHookBeforeChildOpen != nil {
				testHookBeforeChildOpen(root, name)
			}
			if err := caller.Err(); err != nil {
				return false, err
			}
			child, err := root.OpenRoot(name)
			if err != nil {
				return false, err
			}
			cf, err := child.Open(".")
			if err != nil {
				child.Close()
				return false, err
			}
			opened, serr := cf.Stat()
			cerr := cf.Close()
			if serr != nil || cerr != nil || !os.SameFile(info, opened) {
				child.Close()
				return false, errors.New("input directory changed during identity check")
			}
			current, err := root.Lstat(name)
			if err == nil {
				alias, err = rootAlias(root, name, current)
			}
			if err != nil || alias || !os.SameFile(info, current) {
				child.Close()
				return false, errors.New("input directory entry changed during identity check")
			}
			found, err := containsDirectoryContext(caller, child, parents, depth+1, entries, deadline, files...)
			cerr = child.Close()
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

var testHookBeforeChildOpen func(*os.Root, string)
