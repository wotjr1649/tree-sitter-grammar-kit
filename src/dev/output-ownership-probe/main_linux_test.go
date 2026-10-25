package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"slices"
	"testing"

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
	fd, err := unix.Open("/dev/null", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	r := outcome{"CANDIDATE_MATCH", "test"}
	closeFD(fd, &r)
	if r != (outcome{"FAILED", "CLOSE_FAILED"}) {
		t.Fatalf("close failure lost: %+v", r)
	}
}
