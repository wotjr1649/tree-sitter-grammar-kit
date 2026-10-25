package native

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func TestLinuxOutputParentPolicy(t *testing.T) {
	for _, mode := range []os.FileMode{0o770, 0o777, 0o777 | os.ModeSticky} {
		t.Run(mode.String(), func(t *testing.T) {
			path := t.TempDir()
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			p, err := os.OpenRoot(path)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			if d, err := createOutputDirectory(p, "refused"); d != nil || !errors.Is(err, os.ErrPermission) {
				t.Fatalf("shared parent accepted: %v %v", d, err)
			}
			if names, err := os.ReadDir(path); err != nil || len(names) != 0 {
				t.Fatalf("rejected creation changed parent: %v %v", names, err)
			}
		})
	}
	path := t.TempDir()
	p, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for _, name := range []string{"", ".", "..", "../escape", "two/parts", "/absolute"} {
		if d, err := createOutputDirectory(p, name); d != nil || !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("invalid basename %q: %v %v", name, d, err)
		}
	}
	// OpenRoot is already held: the new gate must not add a read requirement.
	if err := os.Chmod(path, 0o300); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o700)
	d, err := createOutputDirectory(p, "결과-é")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
}

func outputACL(entries ...[3]uint32) []byte {
	b := make([]byte, 4+8*len(entries))
	binary.LittleEndian.PutUint32(b, 2)
	for i, e := range entries {
		binary.LittleEndian.PutUint16(b[4+8*i:], uint16(e[0]))
		binary.LittleEndian.PutUint16(b[6+8*i:], uint16(e[1]))
		binary.LittleEndian.PutUint32(b[8+8*i:], e[2])
	}
	return b
}

func TestLinuxOutputACLAndNested(t *testing.T) {
	path := t.TempDir()
	p, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	const undefined = ^uint32(0)
	for _, mask := range []uint32{7, 5} {
		acl := outputACL([3]uint32{1, 7, undefined}, [3]uint32{2, 7, 65533},
			[3]uint32{4, 0, undefined}, [3]uint32{16, mask, undefined}, [3]uint32{32, 0, undefined})
		if err := unix.Setxattr(path, "system.posix_acl_access", acl, 0); err != nil {
			t.Fatal(err)
		}
		d, err := createOutputDirectory(p, "mask-"+strconv.Itoa(int(mask)))
		if mask == 7 {
			if d != nil || !errors.Is(err, os.ErrPermission) {
				t.Fatalf("ACL write mask accepted: %v %v", d, err)
			}
			if _, err := p.Lstat("mask-7"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("ACL rejection created output: %v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		} else if err := d.Close(); err != nil {
			t.Fatal(err)
		}
	}
	acl := outputACL([3]uint32{1, 5, undefined}, [3]uint32{4, 0, undefined}, [3]uint32{32, 0, undefined})
	if err := unix.Setxattr(path, "system.posix_acl_default", acl, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	d, err := createOutputDirectory(p, "default")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	defer d.Chmod(".", 0o700)
	info, err := d.Stat(".")
	if err != nil || info.Mode().Perm() != 0o500 || info.Mode()&os.ModeSetgid == 0 {
		t.Fatalf("default ACL/setgid not exercised: %v %v", info, err)
	}
	if nested, err := createOutputDirectory(d.Root, "responses"); nested != nil || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("nested parent gate bypassed: %v %v", nested, err)
	}
	if names, err := os.ReadDir(filepath.Join(path, "default")); err != nil || len(names) != 0 {
		t.Fatalf("nested rejection changed acquired output: %v %v", names, err)
	}
}

func TestLinuxOutputSyscallFailure(t *testing.T) {
	if target := os.Getenv("TSGK_OUTPUT_SYSCALL_FAILURE"); target != "" {
		traps := map[string]uint32{"statfs": unix.SYS_FSTATFS, "capget": unix.SYS_CAPGET, "access": unix.SYS_FACCESSAT2}
		trap, ok := traps[target]
		if !ok {
			t.Fatal("unknown failure control")
		}
		path := t.TempDir()
		p, err := os.OpenRoot(path)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		positive, err := createOutputDirectory(p, "positive")
		if err != nil {
			t.Fatal("positive control:", err)
		}
		if err := positive.Close(); err != nil {
			t.Fatal(err)
		}
		// Keep this finite child thread locked so its syscall filter cannot reach another test.
		runtime.LockOSThread()
		filter := []unix.SockFilter{
			{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS},
			{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: trap, Jf: 1},
			{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EIO)},
			{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
		}
		prog := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
		if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
			t.Fatal(err)
		}
		if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&prog)), 0, 0); err != nil {
			t.Fatal(err)
		}
		runtime.KeepAlive(filter)
		if d, err := createOutputDirectory(p, "refused"); d != nil || !errors.Is(err, unix.EIO) {
			t.Fatalf("syscall failure lost: %v %v", d, err)
		}
		if names, err := os.ReadDir(path); err != nil || len(names) != 1 || names[0].Name() != "positive" {
			t.Fatalf("failed gate created output: %v %v", names, err)
		}
		return
	}
	for _, target := range []string{"statfs", "capget", "access"} {
		t.Run(target, func(t *testing.T) {
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, "-test.run", "^TestLinuxOutputSyscallFailure$", "-test.timeout", "8s")
			cmd.Env = []string{"TSGK_OUTPUT_SYSCALL_FAILURE=" + target}
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("syscall control %s: %v %s", target, err, b)
			}
		})
	}
}

func TestLinuxOutputDACHelper(t *testing.T) {
	mode := os.Getenv("TSGK_OUTPUT_DAC_HELPER")
	if mode == "" {
		t.Skip("finite isolated fixture child only")
	}
	wantUID := 65533
	if mode == "create" {
		wantUID = 65534
	}
	if os.Getuid() != wantUID || os.Geteuid() != wantUID {
		t.Fatal("DAC child credential was not established")
	}
	path := os.Getenv("TSGK_OUTPUT_DAC_PARENT")
	p, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	switch mode {
	case "create":
		d, err := createOutputDirectory(p, "owned")
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()
		if err := createExclusive(d.Root, "member", []byte("owned bytes")); err != nil {
			t.Fatal(err)
		}
	case "nonowner":
		if d, err := createOutputDirectory(p, "refused"); d != nil || !errors.Is(err, os.ErrPermission) {
			t.Fatalf("nonowner acquisition: %v %v", d, err)
		}
		if err := p.Rename("owned", "replaced"); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("nonowner replacement was not denied: %v", err)
		}
	case "shared":
		if d, err := createOutputDirectory(p, "refused"); d != nil || !errors.Is(err, os.ErrPermission) {
			t.Fatalf("shared acquisition: %v %v", d, err)
		}
		if err := p.Rename("owned", "replaced"); err != nil {
			t.Fatal("shared-parent positive control replacement:", err)
		}
		if err := p.Mkdir("owned", 0o755); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("unknown DAC child")
	}
}

func TestLinuxOutputDACIsolation(t *testing.T) {
	if os.Getenv("TSGK_PRIVATE_DAC_TEST") != "1" {
		t.Skip("requires the explicit finite isolated CI fixture")
	}
	self, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	parent, err := os.Readlink("/proc/" + strconv.Itoa(os.Getppid()) + "/ns/mnt")
	if err != nil || self == parent {
		t.Fatal("private mount namespace not established")
	}
	if os.Geteuid() != 0 {
		t.Fatal("fixture requires root only in the explicit isolated child")
	}
	base, err := os.MkdirTemp("", "tsgk-215-dac-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(base); err != nil {
			t.Error(err)
		}
	}()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The go build directory may be private to the runner; expose only this owned binary copy.
	source, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	exe = filepath.Join(base, "fixture.test")
	copy, err := os.OpenFile(exe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		source.Close()
		t.Fatal(err)
	}
	n, copyErr := io.Copy(copy, io.LimitReader(source, (64<<20)+1))
	if err := errors.Join(copyErr, source.Close(), copy.Close()); err != nil || n > 64<<20 {
		t.Fatalf("bounded fixture binary copy: %d %v", n, err)
	}
	if err := os.Chmod(base, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(uid uint32, mode, path string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "-test.run", "^TestLinuxOutputDACHelper$", "-test.timeout", "4s")
		cmd.Dir = base
		cmd.Env = []string{"TSGK_OUTPUT_DAC_HELPER=" + mode, "TSGK_OUTPUT_DAC_PARENT=" + path}
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: uid, Groups: []uint32{}}}
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("DAC child uid%d/%s: %v %s", uid, mode, err, b)
		}
	}
	for _, shared := range []bool{false, true} {
		path := filepath.Join(base, strconv.FormatBool(shared))
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(path, 65534, 65534); err != nil {
			t.Fatal(err)
		}
		run(65534, "create", path)
		if shared {
			if err := os.Chmod(path, 0o777); err != nil {
				t.Fatal(err)
			}
			run(65533, "shared", path)
			t.Log("DAC_SHARED_REPLACEMENT_OBSERVED_AND_ACQUISITION_REFUSED")
		} else {
			run(65533, "nonowner", path)
			if b, err := os.ReadFile(filepath.Join(path, "owned", "member")); err != nil || string(b) != "owned bytes" {
				t.Fatalf("nonowner changed winner bytes: %q %v", b, err)
			}
			t.Log("DAC_NONOWNER_REPLACEMENT_REFUSED_WINNER_PRESERVED")
		}
		if _, err := os.Lstat(filepath.Join(path, "refused")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rejected child left output: %v", err)
		}
	}
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var caps [2]unix.CapUserData
	if err := unix.Capget(&header, &caps[0]); err != nil || caps[0].Effective&(1<<unix.CAP_DAC_OVERRIDE) == 0 {
		t.Fatalf("capability control was not present: %v", err)
	}
	p, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	d, createErr := createOutputDirectory(p, "privileged")
	if closeErr := p.Close(); d != nil || !errors.Is(createErr, os.ErrPermission) || closeErr != nil {
		t.Fatalf("CAP_DAC_OVERRIDE owner accepted: %v %v %v", d, createErr, closeErr)
	}
	if _, err := os.Lstat(filepath.Join(base, "privileged")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("capability rejection left output: %v", err)
	}
	t.Log("DAC_EFFECTIVE_OVERRIDE_CALLER_REFUSED")
}
