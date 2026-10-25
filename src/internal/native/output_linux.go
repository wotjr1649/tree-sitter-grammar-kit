package native

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/unix"
)

func createOutputDirectory(parent *os.Root, name string) (directory *outputDirectory, resultErr error) {
	if !filepath.IsLocal(name) || filepath.Base(name) != name || name == "." {
		return nil, os.ErrInvalid
	}
	p, err := parent.OpenFile(".", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
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
	// Permission checks and acquisition must use the same thread's credentials.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	fd := int(p.Fd())
	if err := checkOutputParent(fd); err != nil {
		return nil, fmt.Errorf("Linux output parent: %w", err)
	}
	if err := unix.Mkdirat(fd, name, 0o755); err != nil {
		return nil, err
	}
	// Only non-owner replacement is excluded; owner/credential/mount mutation is outside the contract.
	root, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err // an unverified basename is never cleaned up
	}
	return &outputDirectory{Root: root}, nil
}

func checkOutputParent(fd int) error {
	var fs unix.Statfs_t
	if err := unix.Fstatfs(fd, &fs); err != nil {
		return err
	}
	// This magic is shared by ext2/ext3/ext4; trusted standard-driver DAC/ACL semantics are required.
	if fs.Type != unix.EXT4_SUPER_MAGIC {
		return unix.EOPNOTSUPP
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&0o022 != 0 {
		return os.ErrPermission
	}
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	if err := unix.Capget(&header, &data[0]); err != nil {
		return err
	}
	if data[0].Effective&(1<<unix.CAP_DAC_OVERRIDE) != 0 {
		return os.ErrPermission
	}
	// The direct syscall preserves fsuid/idmap/ACL semantics; no access() emulation or UID comparison.
	return unix.Faccessat2(fd, "", unix.W_OK|unix.X_OK, unix.AT_EMPTY_PATH|unix.AT_EACCESS)
}
