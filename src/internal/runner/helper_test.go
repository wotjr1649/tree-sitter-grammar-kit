package runner

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as the owned helper executable: TSGK_RUNNER_HELPER selects a
// behaviour. The runner never inherits the environment, so each spec passes it explicitly.
func TestMain(m *testing.M) {
	if mode := os.Getenv("TSGK_RUNNER_HELPER"); mode != "" {
		os.Exit(helper(mode))
	}
	os.Exit(m.Run())
}

func self() string {
	p, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return p
}

// spawn starts the helper again in mode, inheriting stdout/stderr, and records its pid.
func spawn(mode string, detach bool) *exec.Cmd { return spawnIO(mode, detach, true) }

// spawnIO starts the helper; without inherit its stdio is the null device.
func spawnIO(mode string, detach, inherit bool) *exec.Cmd {
	cmd := exec.Command(self())
	cmd.Env = []string{"TSGK_RUNNER_HELPER=" + mode, "TSGK_PIDS=" + os.Getenv("TSGK_PIDS")}
	if inherit {
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	}
	if detach {
		detachAttr(cmd)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "spawn:", err)
		os.Exit(70)
	}
	recordPID(cmd.Process.Pid)
	return cmd
}

func recordPID(pid int) {
	if path := os.Getenv("TSGK_PIDS"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintln(f, pid)
			f.Close()
		}
	}
}

func helper(mode string) int {
	name, arg, _ := strings.Cut(mode, ":")
	switch name {
	case "args":
		wd, _ := os.Getwd()
		json.NewEncoder(os.Stdout).Encode(map[string]any{"args": os.Args[1:], "dir": wd, "env": os.Environ()})
		return 0
	case "exit":
		n, _ := strconv.Atoi(arg)
		fmt.Print("partial")
		return n
	case "flood":
		buf := make([]byte, 64*1024)
		for {
			if _, err := os.Stdout.Write(buf); err != nil {
				return 0
			}
		}
	case "stderr-flood":
		buf := make([]byte, 64*1024)
		for {
			if _, err := os.Stderr.Write(buf); err != nil {
				return 0
			}
		}
	case "sleep":
		recordPID(os.Getpid())
		time.Sleep(time.Hour)
		return 0
	case "tree": // child and grandchild, all sleeping; the parent sleeps too
		recordPID(os.Getpid())
		spawn("tree-child", false)
		time.Sleep(time.Hour)
		return 0
	case "tree-child":
		spawn("sleep", false)
		time.Sleep(time.Hour)
		return 0
	case "orphan": // the parent exits at once; a grandchild keeps the stdout pipe open
		spawn("tree-child", false)
		time.Sleep(200 * time.Millisecond)
		fmt.Print("parent-done")
		return 0
	case "escape": // a descendant leaves the process group (setsid / detached on Windows)
		spawn("sleep", true)
		time.Sleep(200 * time.Millisecond)
		return 0
	case "escape-quiet": // the same, but the escaped descendant holds no output pipe
		spawnIO("sleep", true, false)
		time.Sleep(200 * time.Millisecond)
		return 0
	case "memory":
		mib, _ := strconv.Atoi(arg)
		var keep [][]byte
		for i := 0; i < mib; i++ {
			b := make([]byte, 1<<20)
			for j := 0; j < len(b); j += 4096 {
				b[j] = 1
			}
			keep = append(keep, b)
		}
		time.Sleep(time.Hour)
		return len(keep)
	case "echo":
		io.Copy(os.Stdout, os.Stdin)
		return 0
	case "exact":
		n, _ := strconv.Atoi(arg)
		os.Stdout.Write(make([]byte, n))
		return 0
	case "frames":
		return frameHelper(arg)
	}
	fmt.Fprintln(os.Stderr, "unknown helper mode", mode)
	return 64
}

// frameHelper answers length-prefixed frames. arg is "<action>@<k>" acting at frame k
// (0-based): hang, crash, big (oversized response header), fat (1000-byte padded response),
// alloc; "echo" answers all.
func frameHelper(arg string) int {
	action, at, _ := strings.Cut(arg, "@")
	k := -1
	if at != "" {
		k, _ = strconv.Atoi(at)
	}
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	for i := 0; ; i++ {
		var hdr [4]byte
		if _, err := io.ReadFull(in, hdr[:]); err != nil {
			return 0
		}
		req := make([]byte, binary.BigEndian.Uint32(hdr[:]))
		if _, err := io.ReadFull(in, req); err != nil {
			return 65
		}
		if i == k {
			switch action {
			case "hang":
				time.Sleep(time.Hour)
			case "escape": // an escaped descendant keeps stdout open while the frame hangs
				spawn("sleep", true)
				time.Sleep(time.Hour)
			case "crash":
				return 3
			case "big":
				binary.BigEndian.PutUint32(hdr[:], 1<<30)
				out.Write(hdr[:])
				out.Flush()
				time.Sleep(time.Hour)
			case "fat": // a well-formed but large response that crosses the batch stdout cap
				req = append(req, make([]byte, 1000)...)
			case "alloc":
				helper("memory:1024")
			}
		}
		resp := append([]byte("ok:"), req...)
		binary.BigEndian.PutUint32(hdr[:], uint32(len(resp)))
		out.Write(hdr[:])
		out.Write(resp)
		out.Flush()
	}
}
