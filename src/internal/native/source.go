package native

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// InputRoots are borrowed from the caller's containment check. A non-nil binding
// requires both roots and never falls back to a pathname.
type InputRoots struct{ Cases, Grammar *os.Root }

func (r *InputRoots) validate() error {
	if r == nil {
		return nil
	}
	for _, root := range []*os.Root{r.Cases, r.Grammar} {
		if root == nil {
			return os.ErrInvalid
		}
		f, err := root.Open(".")
		if err != nil {
			return err
		}
		info, statErr := f.Stat()
		closeErr := f.Close()
		if statErr != nil || closeErr != nil {
			return errors.Join(statErr, closeErr)
		}
		if !info.IsDir() {
			return os.ErrInvalid
		}
	}
	return nil
}

var errSourceLink = errors.New("link or special file in source path")
var testHookBeforeSourceLeaf func(*os.Root, string)
var testHookSourceOpened func(*os.File)

// readBoundSource keeps lookup relative to the selected root. The leaf open does
// not follow links or block on a substituted FIFO; caller verifies these same bytes.
func readBoundSource(root *os.Root, rel string, size int64) (data []byte, resultErr error) {
	if root == nil || !fs.ValidPath(rel) || rel == "." || !filepath.IsLocal(filepath.FromSlash(rel)) || strings.ContainsAny(rel, `\:`) || size < 0 || size == 1<<63-1 {
		return nil, os.ErrInvalid
	}
	current := root
	var owned *os.Root
	defer func() {
		if owned != nil {
			resultErr = errors.Join(resultErr, owned.Close())
		}
	}()
	parts := strings.Split(rel, "/")
	for _, name := range parts[:len(parts)-1] {
		before, err := current.Lstat(name)
		if err != nil {
			return nil, err
		}
		if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
			return nil, errSourceLink
		}
		child, err := current.OpenRoot(name)
		if err != nil {
			return nil, err
		}
		f, err := child.Open(".")
		if err != nil {
			return nil, errors.Join(err, child.Close())
		}
		opened, statErr := f.Stat()
		closeErr := f.Close()
		after, afterErr := current.Lstat(name)
		if statErr != nil || closeErr != nil || afterErr != nil || !opened.IsDir() || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, opened) || !os.SameFile(opened, after) {
			return nil, errors.Join(errors.New("source directory changed during lookup"), statErr, closeErr, afterErr, child.Close())
		}
		if owned != nil {
			if err := owned.Close(); err != nil {
				return nil, errors.Join(err, child.Close())
			}
		}
		owned, current = child, child
	}
	name := parts[len(parts)-1]
	before, err := current.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errSourceLink
	}
	if testHookBeforeSourceLeaf != nil {
		testHookBeforeSourceLeaf(current, name)
	}
	parent, err := current.Open(".")
	if err != nil {
		return nil, err
	}
	f, openErr := openSourceLeaf(parent, name)
	closeErr := parent.Close()
	if openErr != nil || closeErr != nil {
		if f != nil {
			closeErr = errors.Join(closeErr, f.Close())
		}
		return nil, errors.Join(openErr, closeErr)
	}
	defer func() { resultErr = errors.Join(resultErr, f.Close()) }()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() {
		return nil, errSourceLink
	}
	// r0 rejects hard links like the core guard; a same-bytes link would still pass the hash check
	if links, err := sourceLinks(f); err != nil {
		return nil, err
	} else if links != 1 {
		return nil, errSourceLink
	}
	if !os.SameFile(before, opened) {
		return nil, errors.New("source file changed during lookup")
	}
	if testHookSourceOpened != nil {
		testHookSourceOpened(f)
	}
	data, err = io.ReadAll(io.LimitReader(f, size+1))
	if err != nil {
		return nil, err
	}
	after, err := current.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !after.Mode().IsRegular() || !os.SameFile(opened, after) {
		return nil, errors.New("source file changed during read")
	}
	return data, nil
}
