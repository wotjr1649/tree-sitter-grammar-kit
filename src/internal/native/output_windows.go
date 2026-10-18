package native

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func createOutputDirectory(parent *os.Root, name string) (directory *outputDirectory, resultErr error) {
	stem, _, _ := strings.Cut(name, ".")
	if !filepath.IsLocal(name) || (stem != "" && !filepath.IsLocal(strings.TrimRight(stem, " "))) || filepath.Base(name) != name || name == "." ||
		strings.ContainsAny(name, ":/\\<>\"|?*") || strings.TrimRight(name, ". ") != name || strings.ContainsFunc(name, func(r rune) bool { return r < 32 }) {
		return nil, os.ErrInvalid
	}
	p, err := parent.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := p.Close(); err != nil {
			if directory != nil {
				err = errors.Join(err, directory.Close())
				directory = nil
			}
			resultErr = errors.Join(resultErr, err)
		}
	}()
	u, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(p.Fd()), ObjectName: u, Attributes: windows.OBJ_CASE_INSENSITIVE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&h, windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		&oa, &iosb, nil, windows.FILE_ATTRIBUTE_DIRECTORY, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_CREATE, windows.FILE_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT, 0, 0)
	if err != nil {
		if errors.Is(err, windows.STATUS_OBJECT_NAME_COLLISION) {
			return nil, os.ErrExist
		}
		return nil, err
	}
	guard := os.NewFile(uintptr(h), name)
	// Do not allow rename/delete between creation and Root acquisition or while
	// writing. Releasing this guard and reopening the pathname would lose ownership.
	root, err := parent.OpenRoot(name)
	if err != nil {
		return nil, errors.Join(err, guard.Close()) // never remove an unverified pathname
	}
	f, err := root.Open(".")
	if err != nil {
		return nil, errors.Join(err, root.Close(), guard.Close())
	}
	created, e1 := guard.Stat()
	opened, e2 := f.Stat()
	e3 := f.Close()
	const fileCreated = 2 // IO_STATUS_BLOCK.Information: FILE_CREATED
	if e1 != nil || e2 != nil || e3 != nil || iosb.Information != fileCreated || !created.IsDir() || !os.SameFile(created, opened) {
		return nil, errors.Join(errors.New("created output directory identity is unavailable or changed"), e1, e2, e3, root.Close(), guard.Close())
	}
	return &outputDirectory{Root: root, guard: guard}, nil
}
