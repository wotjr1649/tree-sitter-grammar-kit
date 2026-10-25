// Command output-ownership-probe records Linux API experiments, never support evidence.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const maxHandle = 128

type outcome struct {
	Status string `json:"status"`
	Code   string `json:"code"`
}

type report struct {
	Schema       string  `json:"schema"`
	Support      string  `json:"support_claim"`
	Kernel       string  `json:"kernel_release"`
	Filesystem   int64   `json:"filesystem_type"`
	Capabilities string  `json:"privilege_context"`
	Preserved    string  `json:"preserved_directory"`
	Directory    outcome `json:"directory"`
	Replacement  outcome `json:"replacement"`
	Publication  outcome `json:"publication"`
}

func refused(err error, code string) outcome {
	status := "FAILED"
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOVERFLOW) {
		status = "UNSUPPORTED"
	}
	return outcome{status, code}
}

func closeFD(fd int, result *outcome) {
	if err := unix.Close(fd); err != nil {
		*result = outcome{"FAILED", "CLOSE_FAILED"}
	}
}

type fid struct {
	FSID   [8]byte
	Type   uint32
	Handle string
}

// The fixed buffer rejects larger handles instead of allowing a kernel-sized allocation.
func descriptorFID(fd int) (fid, error) {
	var buf [8 + maxHandle]byte
	binary.NativeEndian.PutUint32(buf[:4], maxHandle)
	var mount int32
	empty := [1]byte{}
	_, _, errno := unix.Syscall6(unix.SYS_NAME_TO_HANDLE_AT, uintptr(fd), uintptr(unsafe.Pointer(&empty[0])), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&mount)), unix.AT_EMPTY_PATH, 0)
	if errno != 0 {
		return fid{}, errno
	}
	n := binary.NativeEndian.Uint32(buf[:4])
	if n == 0 || n > maxHandle {
		return fid{}, fmt.Errorf("handle length")
	}
	var st unix.Statfs_t
	if err := unix.Fstatfs(fd, &st); err != nil {
		return fid{}, err
	}
	f := fid{Type: binary.NativeEndian.Uint32(buf[4:8]), Handle: string(buf[8 : 8+n])}
	binary.NativeEndian.PutUint32(f.FSID[:4], uint32(st.Fsid.Val[0]))
	binary.NativeEndian.PutUint32(f.FSID[4:], uint32(st.Fsid.Val[1]))
	if f.FSID == [8]byte{} {
		return fid{}, unix.EOPNOTSUPP
	}
	return f, nil
}

type creation struct {
	PID    int32
	Parent fid
	Child  fid
	Name   string
}

// Only the requested creation format is accepted. Records may appear in either order.
func creations(buf []byte) ([]creation, error) {
	var events []creation
	for len(buf) != 0 {
		if len(buf) < unix.FAN_EVENT_METADATA_LEN {
			return nil, fmt.Errorf("short metadata")
		}
		n := binary.NativeEndian.Uint32(buf[:4])
		meta := binary.NativeEndian.Uint16(buf[6:8])
		mask := binary.NativeEndian.Uint64(buf[8:16])
		if n < unix.FAN_EVENT_METADATA_LEN || n%4 != 0 || uint64(n) > uint64(len(buf)) || buf[4] != unix.FANOTIFY_METADATA_VERSION || buf[5] != 0 || meta != unix.FAN_EVENT_METADATA_LEN || mask & ^uint64(unix.FAN_EVENT_ON_CHILD) != unix.FAN_CREATE|unix.FAN_ONDIR || int32(binary.NativeEndian.Uint32(buf[16:20])) != unix.FAN_NOFD {
			return nil, fmt.Errorf("unexpected metadata or overflow")
		}
		e := creation{PID: int32(binary.NativeEndian.Uint32(buf[20:24]))}
		seen := map[byte]bool{}
		for records := buf[meta:n]; len(records) != 0; {
			if len(records) < 20 {
				return nil, fmt.Errorf("short FID record")
			}
			kind := records[0]
			size := binary.NativeEndian.Uint16(records[2:4])
			handleSize := binary.NativeEndian.Uint32(records[12:16])
			if records[1] != 0 || size < 20 || int(size) > len(records) || size%4 != 0 || handleSize == 0 || handleSize > maxHandle || uint64(handleSize)+20 > uint64(size) || seen[kind] || kind != unix.FAN_EVENT_INFO_TYPE_FID && kind != unix.FAN_EVENT_INFO_TYPE_DFID_NAME {
				return nil, fmt.Errorf("unexpected FID record")
			}
			seen[kind] = true
			f := fid{Type: binary.NativeEndian.Uint32(records[16:20]), Handle: string(records[20 : 20+handleSize])}
			copy(f.FSID[:], records[4:12])
			if f.FSID == [8]byte{} {
				return nil, fmt.Errorf("zero filesystem identity")
			}
			if kind == unix.FAN_EVENT_INFO_TYPE_FID {
				e.Child = f
			} else {
				e.Parent = f
				name := records[20+handleSize : size]
				end := bytes.IndexByte(name, 0)
				if end < 1 || bytes.ContainsAny(name[:end], "/") {
					return nil, fmt.Errorf("invalid name")
				}
				e.Name = string(name[:end])
			}
			records = records[size:]
		}
		if len(seen) != 2 || e.Name == "" {
			return nil, fmt.Errorf("incomplete creation")
		}
		events = append(events, e)
		if len(events) > 64 {
			return nil, fmt.Errorf("event limit")
		}
		buf = buf[n:]
	}
	return events, nil
}

func witness(events []creation, parent, child fid, pid int32, name string) outcome {
	count := 0
	changed := false
	for _, e := range events {
		if e.PID == pid && e.Name == name && e.Parent == parent {
			count++
			if e.Child != child {
				changed = true
			}
		}
	}
	if changed || count > 1 {
		return outcome{"REJECTED", "AMBIGUOUS_OR_CHANGED_CREATION"}
	}
	if count == 1 {
		return outcome{"CANDIDATE_MATCH", "STATIC_ACTIVATION_BLOCKERS_REMAIN"}
	}
	return outcome{"UNVERIFIED", "NO_COMPLETE_WITNESS"}
}

func directoryProbe(root string, replace bool) (result outcome) {
	result = outcome{"UNVERIFIED", "NO_COMPLETE_WITNESS"}
	parent, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return refused(err, "PARENT_OPEN")
	}
	defer closeFD(parent, &result)
	pin, err := descriptorFID(parent)
	if err != nil {
		return refused(err, "PARENT_HANDLE")
	}
	queue, err := unix.FanotifyInit(unix.FAN_CLOEXEC|unix.FAN_NONBLOCK|unix.FAN_REPORT_DFID_NAME_TARGET, unix.O_RDONLY|unix.O_CLOEXEC)
	if err != nil {
		return refused(err, "FANOTIFY_INIT")
	}
	defer closeFD(queue, &result)
	if err := unix.FanotifyMark(queue, unix.FAN_MARK_ADD|unix.FAN_MARK_ONLYDIR, unix.FAN_CREATE|unix.FAN_ONDIR|unix.FAN_EVENT_ON_CHILD, parent, "."); err != nil {
		return refused(err, "FANOTIFY_MARK")
	}
	if err := unix.Mkdirat(parent, "child", 0o700); err != nil {
		return refused(err, "MKDIR")
	}
	if replace {
		if err := unix.Renameat(parent, "child", parent, "original"); err != nil {
			return refused(err, "RENAME")
		}
		if err := unix.Mkdirat(parent, "child", 0o700); err != nil {
			return refused(err, "REPLACEMENT_MKDIR")
		}
	}
	child, err := unix.Openat(parent, "child", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return refused(err, "CHILD_OPEN")
	}
	defer closeFD(child, &result)
	identity, err := descriptorFID(child)
	if err != nil {
		return refused(err, "CHILD_HANDLE")
	}
	var events []creation
	buf := make([]byte, 64<<10)
	deadline, total := time.Now().Add(2*time.Second), 0
	for total < 256<<10 && time.Now().Before(deadline) {
		n, err := unix.Read(queue, buf)
		if errors.Is(err, unix.EAGAIN) {
			return witness(events, pin, identity, int32(os.Getpid()), "child")
		}
		if err != nil || n == 0 {
			return outcome{"UNVERIFIED", "QUEUE_READ"}
		}
		total += n
		part, err := creations(buf[:n])
		if err != nil || len(events)+len(part) > 64 {
			return outcome{"UNVERIFIED", "EVENT_INVALID_OR_LIMIT"}
		}
		events = append(events, part...)
	}
	return outcome{"UNVERIFIED", "QUEUE_BUDGET"}
}

func publicationProbe(root string) (result outcome) {
	result = outcome{"UNVERIFIED", "PROCFS_TRUST_NOT_PROVEN"}
	parent, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return refused(err, "PARENT_OPEN")
	}
	defer closeFD(parent, &result)
	proc, err := unix.Open("/proc/self/fd", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return refused(err, "PROCFS_OPEN")
	}
	defer closeFD(proc, &result)
	var st unix.Statfs_t
	if err := unix.Fstatfs(proc, &st); err != nil || st.Type != unix.PROC_SUPER_MAGIC {
		return outcome{"UNVERIFIED", "PROCFS_TYPE"}
	}
	fd, err := unix.Openat(parent, ".", unix.O_TMPFILE|unix.O_RDWR|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return refused(err, "O_TMPFILE")
	}
	defer closeFD(fd, &result)
	if n, err := unix.Write(fd, []byte("original")); err != nil || n != len("original") {
		return outcome{"FAILED", "WRITE"}
	}
	if err := unix.Fsync(fd); err != nil {
		return refused(err, "FSYNC")
	}
	name := strconv.Itoa(fd)
	if err := unix.Linkat(proc, name, parent, "final", unix.AT_SYMLINK_FOLLOW); err != nil {
		return refused(err, "LINKAT")
	}
	if err := unix.Linkat(proc, name, parent, "final", unix.AT_SYMLINK_FOLLOW); !errors.Is(err, unix.EEXIST) {
		return outcome{"FAILED", "NO_CLOBBER"}
	}
	check, err := unix.Openat(parent, "final", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return refused(err, "FINAL_OPEN")
	}
	f := os.NewFile(uintptr(check), "probe-final")
	data, readErr := io.ReadAll(io.LimitReader(f, 9))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || string(data) != "original" {
		return outcome{"FAILED", "FINAL_BYTES_OR_CLOSE"}
	}
	// shortcut: procfs type is only a capability check; prove process binding before adoption.
	return outcome{"UNVERIFIED", "LINK_AND_EEXIST_OBSERVED_PROCFS_TRUST_NOT_PROVEN"}
}

func probe(parent string) (report, error) {
	r := report{Schema: "tsgk-output-ownership-probe/r1", Support: "BLOCKED"}
	if !filepath.IsAbs(parent) || len(parent) > 4096 {
		return r, fmt.Errorf("absolute task parent required")
	}
	root, err := os.MkdirTemp(parent, "probe-215-")
	if err != nil {
		return r, err
	}
	r.Preserved = filepath.Base(root)
	var host unix.Utsname
	var fs unix.Statfs_t
	if err := unix.Uname(&host); err != nil {
		return r, err
	}
	if err := unix.Statfs(root, &fs); err != nil {
		return r, err
	}
	r.Kernel, r.Filesystem = unix.ByteSliceToString(host.Release[:]), int64(fs.Type)
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var caps [2]unix.CapUserData
	r.Capabilities = "UNKNOWN"
	if err := unix.Capget(&header, &caps[0]); err == nil {
		r.Capabilities = "NO_SYS_ADMIN_OR_DAC_READ_SEARCH"
		if os.Geteuid() == 0 || caps[0].Effective&(1<<unix.CAP_SYS_ADMIN|1<<unix.CAP_DAC_READ_SEARCH) != 0 {
			r.Capabilities = "PRIVILEGED_NOT_UNPRIVILEGED_EVIDENCE"
		}
	}
	// Preserve all task-created entries. Unproven ownership never causes pathname cleanup.
	for _, name := range []string{"directory", "replacement", "publication"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			return r, err
		}
	}
	r.Directory = directoryProbe(filepath.Join(root, "directory"), false)
	r.Replacement = directoryProbe(filepath.Join(root, "replacement"), true)
	r.Publication = publicationProbe(filepath.Join(root, "publication"))
	return r, nil
}

func main() {
	parent := flag.String("parent", "", "existing absolute task directory; new probe entries are preserved")
	flag.Parse()
	if flag.NArg() != 0 {
		os.Exit(2)
	}
	timer := time.AfterFunc(10*time.Second, func() { os.Exit(124) })
	defer timer.Stop()
	r, err := probe(*parent)
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe setup failed")
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(r); err != nil {
		os.Exit(1)
	}
	for _, result := range []outcome{r.Directory, r.Replacement, r.Publication} {
		if result.Status == "FAILED" {
			os.Exit(1)
		}
	}
}
