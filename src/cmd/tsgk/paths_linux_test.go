package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// CI runs only this test inside a private mount namespace with a clean environment.
func TestBindMountMembership(t *testing.T) {
	if os.Getenv("TSGK_PRIVATE_MOUNT_TEST") != "1" {
		t.Skip("requires an isolated mount namespace; CI supplies it")
	}
	// Refuse a caller that set the flag without actually isolating its mount namespace.
	self, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	parent, err := os.Readlink("/proc/" + strconv.Itoa(os.Getppid()) + "/ns/mnt")
	if err != nil || self == parent {
		t.Fatal("private mount namespace not established")
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	err = privateMountInfo(f)
	f.Close()
	if err != nil {
		t.Fatal("private mount propagation not established")
	}
	base, err := os.MkdirTemp("", "tsgk-196-")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "input")
	sub := filepath.Join(root, "nested", "member")
	alias := filepath.Join(base, "outside-alias")
	for _, dir := range []string{sub, alias} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	original := filepath.Join(sub, "input.txt")
	if err := os.WriteFile(original, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := func(name string, args ...string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = []string{"PATH=/usr/bin:/bin"}
		return cmd.Run()
	}
	aliasInfo, err := os.Stat(alias)
	if err != nil {
		t.Fatal(err)
	}
	// Register before attempting the mount. Never recurse while its presence is unknown.
	defer func() {
		_ = command("/usr/bin/umount", alias)
		info, err := os.Stat(alias)
		if err != nil || !os.SameFile(info, aliasInfo) {
			t.Error("mount release unconfirmed; fixture retained without recursive removal")
			return
		}
		if err := os.RemoveAll(base); err != nil {
			t.Error(err)
		}
	}()
	if err := command("/usr/bin/mount", "--bind", sub, alias); err != nil {
		t.Fatal("private bind mount failed:", err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	// Confirm this fixture evades the previous ancestor-only comparison.
	for p := alias; ; {
		info, err := os.Stat(p)
		if err != nil || os.SameFile(info, rootInfo) {
			t.Fatal("fixture does not isolate descendant-alias coverage")
		}
		up := filepath.Dir(p)
		if up == p {
			break
		}
		p = up
	}
	contained, err := inside(alias, root)
	if err != nil || !contained {
		t.Fatalf("bind alias missed: %v %v", contained, err)
	}
	out := filepath.Join(alias, "result.json")
	if code, err := publish(out, root, []byte("result")); code != "OUTPUT_INSIDE_INPUT" || err == nil {
		t.Fatalf("bind publication accepted: %s %v", code, err)
	}
	if contained, err := documentInside(filepath.Join(alias, "expected.json"), root); err != nil || !contained {
		t.Fatalf("bind trust document accepted: %v %v", contained, err)
	}
	for _, kind := range []string{"reproduce", "incremental", "oracle"} {
		args := []string{kind}
		if kind == "oracle" {
			args = append(args, "record")
		}
		args = append(args, "--root", root, "--profile", filepath.Join(base, "profile.json"), "--work", alias, "--out", filepath.Join(base, "result"))
		if kind != "reproduce" {
			args = append(args, "--runtime", base, "--tool", "cc="+original)
		}
		code, stdout, stderr := cli(t, context.Background(), args...)
		if code != exitUsage || stdout != "" || !strings.Contains(stderr, "WORK_INSIDE_INPUT") {
			t.Fatalf("%s bind work accepted: %d %s %s", kind, code, stdout, stderr)
		}
	}
	data, err := os.ReadFile(original)
	if err != nil || string(data) != "unchanged" {
		t.Fatal("input modified")
	}
	entries, err := os.ReadDir(sub)
	if err != nil || len(entries) != 1 || entries[0].Name() != "input.txt" {
		t.Fatalf("publication left input files: %v %v", entries, err)
	}
}

func privateMountInfo(r io.Reader) error {
	limited := &io.LimitedReader{R: r, N: (1 << 20) + 1}
	scan := bufio.NewScanner(limited)
	rows := 0
	for scan.Scan() {
		rows++
		fields := strings.Fields(scan.Text())
		if rows > 4096 || len(fields) < 10 || fields[6] != "-" {
			return errors.New("unknown or non-private mount metadata")
		}
	}
	if scan.Err() != nil || limited.N <= 1 || rows == 0 {
		return errors.New("incomplete mount metadata")
	}
	return nil
}

func TestPrivateMountMetadata(t *testing.T) {
	private := "1 0 8:1 / / rw - ext4 /dev/test rw\n"
	for _, tc := range []struct {
		input string
		want  bool
	}{{private, true}, {strings.Replace(private, " - ", " shared:1 - ", 1), false},
		{strings.Replace(private, " - ", " master:1 - ", 1), false}, {"", false}, {"invalid\n", false},
		{strings.Repeat(private, 4097), false}, {strings.Repeat("x", (1<<20)+1), false}} {
		if got := privateMountInfo(strings.NewReader(tc.input)) == nil; got != tc.want {
			t.Fatalf("private metadata accepted=%v, want %v", got, tc.want)
		}
	}
}
