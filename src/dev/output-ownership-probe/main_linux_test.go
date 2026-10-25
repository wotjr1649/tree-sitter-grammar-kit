package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func eventBytes() []byte {
	record := func(kind byte, value byte, name string) []byte {
		n := 24
		if name != "" {
			n = (n + len(name) + 1 + 3) &^ 3
		}
		b := make([]byte, n)
		b[0] = kind
		binary.NativeEndian.PutUint16(b[2:4], uint16(n))
		b[4] = 7
		binary.NativeEndian.PutUint32(b[12:16], 4)
		binary.NativeEndian.PutUint32(b[16:20], 1)
		b[20] = value
		copy(b[24:], name)
		return b
	}
	b := make([]byte, unix.FAN_EVENT_METADATA_LEN)
	b[4] = unix.FANOTIFY_METADATA_VERSION
	binary.NativeEndian.PutUint16(b[6:8], unix.FAN_EVENT_METADATA_LEN)
	binary.NativeEndian.PutUint64(b[8:16], unix.FAN_CREATE|unix.FAN_ONDIR)
	binary.NativeEndian.PutUint32(b[16:20], ^uint32(0))
	binary.NativeEndian.PutUint32(b[20:24], uint32(os.Getpid()))
	b = append(b, record(unix.FAN_EVENT_INFO_TYPE_FID, 2, "")...)
	b = append(b, record(unix.FAN_EVENT_INFO_TYPE_DFID_NAME, 1, "child")...)
	binary.NativeEndian.PutUint32(b[:4], uint32(len(b)))
	return b
}

func TestCreationWitness(t *testing.T) {
	b := eventBytes()
	events, err := creations(b)
	if err != nil || len(events) != 1 {
		t.Fatalf("event: %v %v", events, err)
	}
	e := events[0]
	if witness(events, e.Parent, e.Child, int32(os.Getpid()), "child").Status != "CANDIDATE_MATCH" {
		t.Fatal("complete witness not matched")
	}
	if witness(append(slices.Clone(events), e), e.Parent, e.Child, e.PID, e.Name).Status != "REJECTED" {
		t.Fatal("duplicate invocation accepted")
	}
	wrong := e.Child
	wrong.Handle = "replaced"
	if witness(events, e.Parent, wrong, e.PID, e.Name).Status != "REJECTED" || witness(events, e.Parent, e.Child, e.PID+1, e.Name).Status != "UNVERIFIED" {
		t.Fatal("wrong object or process accepted")
	}
	// Information record order is unspecified.
	reversed := append(slices.Clone(b[:24]), b[48:]...)
	reversed = append(reversed, b[24:48]...)
	if r, err := creations(reversed); err != nil || !slices.Equal(r, events) {
		t.Fatalf("record order: %v %v", r, err)
	}
	for _, mutate := range []func([]byte) []byte{
		func(b []byte) []byte { return b[:23] },
		func(b []byte) []byte { b[4]++; return b },
		func(b []byte) []byte { b[5]++; return b },
		func(b []byte) []byte { binary.NativeEndian.PutUint32(b[:4], 65536); return b },
		func(b []byte) []byte { binary.NativeEndian.PutUint64(b[8:16], unix.FAN_Q_OVERFLOW); return b },
		func(b []byte) []byte {
			binary.NativeEndian.PutUint64(b[8:16], unix.FAN_CREATE|unix.FAN_DELETE|unix.FAN_ONDIR)
			return b
		},
		func(b []byte) []byte { b[24] = 99; return b },
		func(b []byte) []byte { b[28] = 0; return b },
		func(b []byte) []byte { binary.NativeEndian.PutUint32(b[36:40], maxHandle+1); return b },
		func(b []byte) []byte { b[48] = unix.FAN_EVENT_INFO_TYPE_FID; return b },
		func(b []byte) []byte { b[72] = 0; return b },
	} {
		if _, err := creations(mutate(slices.Clone(b))); err == nil {
			t.Fatal("malformed or ambiguous event accepted")
		}
	}
}

func TestLinuxCapabilityProbe(t *testing.T) {
	r, err := probe(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if r.Support != "BLOCKED" || r.Schema != "tsgk-output-ownership-probe/r1" {
		t.Fatal("probe promoted to support evidence")
	}
	for _, o := range []outcome{r.Directory, r.Replacement, r.Publication} {
		if o.Status == "FAILED" || o.Code == "" || !slices.Contains([]string{"CANDIDATE_MATCH", "REJECTED", "UNVERIFIED", "UNSUPPORTED"}, o.Status) {
			t.Fatalf("probe failed: %+v", r)
		}
	}
	if r.Replacement.Status == "CANDIDATE_MATCH" || r.Publication.Status == "CANDIDATE_MATCH" {
		t.Fatal("replacement or unproven procfs promoted")
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))
}

func TestCloseFailureIsNotSuccess(t *testing.T) {
	r := outcome{"CANDIDATE_MATCH", "test"}
	closeFD(-1, &r)
	if r != (outcome{"FAILED", fmt.Sprintf("CLOSE:errno=%d", unix.EBADF)}) {
		t.Fatalf("close failure lost: %+v", r)
	}
}

func TestSyscallRefusalKeepsContextAndErrno(t *testing.T) {
	for _, tc := range []struct {
		operation string
		errno     unix.Errno
		status    string
	}{
		{"FANOTIFY_MARK", unix.ENODEV, "UNSUPPORTED"},
		{"FANOTIFY_MARK", unix.EXDEV, "UNSUPPORTED"},
		{"FANOTIFY_INIT", unix.EPERM, "UNSUPPORTED"},
		{"FANOTIFY_INIT", unix.EINVAL, "FAILED"},
		{"FANOTIFY_MARK", unix.EINVAL, "FAILED"},
		{"PARENT_HANDLE", unix.EOVERFLOW, "UNSUPPORTED"},
		{"CHILD_HANDLE", unix.ENOSYS, "UNSUPPORTED"},
		{"O_TMPFILE", unix.EOPNOTSUPP, "UNSUPPORTED"},
		{"O_TMPFILE", unix.EISDIR, "UNSUPPORTED"},
		{"O_TMPFILE", unix.EINVAL, "FAILED"},
		{"O_TMPFILE", unix.EACCES, "UNSUPPORTED"},
		{"PARENT_OPEN", unix.EACCES, "FAILED"},
		{"PROCFS_OPEN", unix.ENOENT, "UNSUPPORTED"},
		{"LINKAT", unix.EXDEV, "UNSUPPORTED"},
		{"LINKAT", unix.ENOENT, "UNSUPPORTED"},
		{"LINKAT", unix.EINVAL, "FAILED"},
		{"FSYNC", unix.EINVAL, "FAILED"},
		{"FSYNC", unix.EIO, "FAILED"},
		{"FSYNC", unix.EOPNOTSUPP, "UNSUPPORTED"},
		{"FANOTIFY_INIT", unix.EMFILE, "UNVERIFIED"},
		{"FANOTIFY_INIT", unix.ENOMEM, "UNVERIFIED"},
		{"O_TMPFILE", unix.ENOSPC, "UNVERIFIED"},
		{"WRITE", unix.ENOSPC, "UNVERIFIED"},
		{"WRITE", unix.EDQUOT, "UNVERIFIED"},
		{"WRITE", unix.ENOMEM, "UNVERIFIED"},
		{"WRITE", unix.EIO, "FAILED"},
		{"WRITE", unix.EBADF, "FAILED"},
		{"QUEUE_READ", unix.EINTR, "UNVERIFIED"},
	} {
		r := refused(tc.errno, tc.operation)
		if r.Status != tc.status || r.Code != fmt.Sprintf("%s:errno=%d", tc.operation, tc.errno) {
			t.Fatalf("refusal %s/%d: %+v", tc.operation, tc.errno, r)
		}
	}
	if r := refused(zeroFSID, "PARENT_HANDLE"); r.Status != "UNSUPPORTED" || r.Code != "PARENT_HANDLE:ZERO_FSID" {
		t.Fatalf("zero fsid: %+v", r)
	}
}

func TestPublicationWriteOutcome(t *testing.T) {
	for _, errno := range []unix.Errno{unix.ENOSPC, unix.EDQUOT, unix.ENOMEM} {
		if r := publicationWrite(0, errno); r == nil || r.Status != "UNVERIFIED" || r.Code != fmt.Sprintf("WRITE:errno=%d", errno) {
			t.Fatalf("write resource %d: %+v", errno, r)
		}
	}
	if r := publicationWrite(4, nil); r == nil || *r != (outcome{"UNVERIFIED", "WRITE_SHORT"}) {
		t.Fatalf("short write: %+v", r)
	}
	if r := publicationWrite(8, nil); r != nil {
		t.Fatalf("complete write: %+v", r)
	}
	fd, err := unix.Open("/dev/full", unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFCHR || st.Rdev != unix.Mkdev(1, 7) {
		t.Fatal("verified /dev/full character device required before write")
	}
	if r := publicationWrite(unix.Write(fd, []byte("original"))); r == nil || r.Status != "UNVERIFIED" || r.Code != fmt.Sprintf("WRITE:errno=%d", unix.ENOSPC) {
		t.Fatalf("actual ENOSPC write: %+v", r)
	}
}

func TestQueueBudgetIncludesReadAndWitness(t *testing.T) {
	for _, tc := range []struct {
		deadline         time.Time
		total, remaining int
	}{
		{time.Now().Add(time.Hour), 0, 64 << 10},
		{time.Now().Add(time.Hour), (256 << 10) - (64 << 10), 64 << 10},
		{time.Now().Add(time.Hour), (256 << 10) - (64 << 10) + 1, 0},
		{time.Now().Add(time.Hour), (256 << 10) - 17, 0},
		{time.Now().Add(time.Hour), (256 << 10) - 1, 0},
		{time.Now().Add(time.Hour), (256 << 10) - len(eventBytes()) + 1, 0},
		{time.Now().Add(time.Hour), 256 << 10, 0},
		{time.Now().Add(time.Hour), (256 << 10) + 1, 0},
		{time.Now().Add(-time.Second), 0, 0},
		{time.Now().Add(-time.Second), (256 << 10) - 17, 0},
	} {
		if got := queueRemaining(tc.deadline, tc.total); got != tc.remaining {
			t.Fatalf("queue allowance after read/witness: got %d want %d", got, tc.remaining)
		}
	}
}

func TestQueueErrorSurvivesExpiredBudget(t *testing.T) {
	expired := time.Now().Add(-time.Second)
	if r := queueReadFailure(expired, 256<<10, unix.EIO); r == nil || *r != (outcome{"FAILED", fmt.Sprintf("QUEUE_READ:errno=%d", unix.EIO)}) {
		t.Fatalf("I/O failure hidden by budget: %+v", r)
	}
	for _, err := range []error{nil, unix.EAGAIN} {
		if r := queueReadFailure(expired, 0, err); r == nil || *r != (outcome{"UNVERIFIED", "QUEUE_BUDGET"}) {
			t.Fatalf("expired observation accepted: %+v", r)
		}
	}
	if r := queueReadFailure(time.Now().Add(time.Hour), 0, unix.EAGAIN); r != nil {
		t.Fatalf("in-budget EAGAIN not available for witness: %+v", r)
	}
}
