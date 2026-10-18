package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestOutputDirectoryAndMemberNoClobber(t *testing.T) {
	base := t.TempDir()
	out := filepath.Join(base, "output")
	d, err := newOutputDirectory(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := createExclusive(d.Root, "member.json", []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := createExclusive(d.Root, "member.json", []byte("replacement")); !errors.Is(err, os.ErrExist) {
		t.Fatalf("member collision: %v", err)
	}
	if other, err := newOutputDirectory(out, nil); other != nil || !errors.Is(err, os.ErrExist) {
		t.Fatalf("directory collision: %v %v", other, err)
	}
	if data, err := d.ReadFile("member.json"); err != nil || string(data) != "original" {
		t.Fatalf("existing member changed: %q %v", data, err)
	}
	child, err := createOutputDirectory(d.Root, "responses")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	if other, err := createOutputDirectory(d.Root, "responses"); other != nil || !errors.Is(err, os.ErrExist) {
		t.Fatalf("child collision: %v %v", other, err)
	}
}

func TestOutputCloseFailureIsReported(t *testing.T) {
	d, err := newOutputDirectory(filepath.Join(t.TempDir(), "output"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.guard == nil {
		d.Close()
		t.Skip("Root.Close is idempotent; this control uses the Windows creation guard")
	}
	if err := d.guard.Close(); err != nil {
		t.Fatal(err)
	}
	report := kit.Report{ExecutionStatus: kit.StatusCompleted}
	var resultErr error
	closeOutput(d, &report, &resultErr)
	if resultErr == nil || report.ExecutionStatus != kit.StatusFailed || len(report.Findings) != 1 || report.Findings[0].Code != "OUTPUT_CLOSE_FAILED" {
		t.Fatalf("close failure hidden: %v %+v", resultErr, report)
	}
	if d.guard != nil {
		if _, err := d.guard.Stat(); err == nil {
			t.Fatal("creation guard remained open after cleanup")
		}
	}
}

func TestIncrementalOutputFinalization(t *testing.T) {
	profile, root := oracleProfile(t, nil, "a = 1;\n")
	var p map[string]any
	if err := json.Unmarshal(profile, &p); err != nil {
		t.Fatal(err)
	}
	p["schema"], p["operation"] = "tsgk-incremental/r2", "native-parse-edit"
	for _, key := range []string{"queries", "fact_pack", "api"} {
		delete(p, key)
	}
	for _, c := range p["cases"].([]any) {
		delete(c.(map[string]any), "query_expect")
		delete(c.(map[string]any), "dynamic_sql_expect")
	}
	profile, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	rt, cc := nativeTools(t)
	cache := t.TempDir() // These cases test output finalization, not repeated compilation.
	for _, which := range []string{"output", "responses", "replacement"} {
		t.Run(which, func(t *testing.T) {
			if which != "replacement" && runtime.GOOS != "windows" {
				t.Skip("the real close-failure control uses the Windows creation guard")
			}
			original := testHookOutputFinalization
			defer func() { testHookOutputFinalization = original }()
			hooked := false
			out := filepath.Join(t.TempDir(), "out")
			moved := out + "-moved"
			replaced := false
			testHookOutputFinalization = func(stage string, output, responses *outputDirectory) {
				if which == "replacement" {
					if stage != "after-close" {
						return
					}
					hooked = true
					if err := os.Rename(out, moved); err != nil {
						if runtime.GOOS == "windows" && (errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(5))) {
							return // The still-owned final file can itself prevent renaming on Windows.
						}
						t.Fatal(err)
					}
					replaced = true
					if err := os.Mkdir(out, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(out, "result.json"), []byte("unrelated"), 0o600); err != nil {
						t.Fatal(err)
					}
					return
				}
				if stage != "before-close" {
					return
				}
				hooked = true
				d := output
				if which == "responses" {
					d = responses
				}
				if err := d.guard.Close(); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			res, runErr := Incremental(ctx, IncrementalRequest{Root: root, GrammarRoot: fixtureRoot(t, "plain"), Profile: profile,
				Runtime: rt, Compiler: cc, Work: t.TempDir(), BuildCache: cache, Out: out, Allow: []string{AllowBuild, AllowExec}, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")})
			if res.Build != nil && res.Build.Reuse != nil {
				t.Logf("output-finalization build reuse: hit=%t", res.Build.Reuse.Hit)
			}
			owned := out
			if replaced {
				owned = moved
				if data, err := os.ReadFile(filepath.Join(out, "result.json")); err != nil || string(data) != "unrelated" {
					t.Fatalf("replacement file changed: %q %v", data, err)
				}
			}
			data, readErr := os.ReadFile(filepath.Join(owned, "result.json"))
			var saved Result
			if !hooked || readErr != nil || json.Unmarshal(data, &saved) != nil {
				t.Fatalf("finalization not reached: hook=%v read=%v run=%v", hooked, readErr, runErr)
			}
			if which == "replacement" {
				returned, err := json.Marshal(res)
				if runErr != nil || err != nil || res.ExecutionStatus != kit.StatusCompleted || res.Assessment != kit.AssessPass || !bytes.Equal(data, append(returned, '\n')) {
					t.Fatalf("final file lost identity or normal result: run=%v encode=%v status=%s assessment=%s", runErr, err, res.ExecutionStatus, res.Assessment)
				}
				return
			}
			var ne *Error
			if !errors.As(runErr, &ne) || ne.Kind != kit.KindIO {
				t.Fatalf("close failure kind: %v", runErr)
			}
			for _, r := range []Result{res, saved} {
				found := false
				for _, f := range r.Findings {
					found = found || f.Code == "OUTPUT_CLOSE_FAILED"
				}
				if runErr == nil || !strings.Contains(runErr.Error(), "OUTPUT_CLOSE_FAILED") || !found ||
					r.ExecutionStatus != kit.StatusFailed || r.Assessment != kit.AssessNotAssessed {
					t.Fatalf("close failure not preserved in returned and saved report: err=%v status=%s assessment=%s findings=%+v", runErr, r.ExecutionStatus, r.Assessment, r.Findings)
				}
			}
		})
	}
}

func TestOutputDirectoryKeepsAcquiredIdentity(t *testing.T) {
	base := t.TempDir()
	out, moved := filepath.Join(base, "output"), filepath.Join(base, "moved")
	d, err := newOutputDirectory(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	err = os.Rename(out, moved)
	blocked := runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(32))
	if !blocked {
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(out, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "member.json"), []byte("unrelated"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := createExclusive(d.Root, "member.json", []byte("owned")); err != nil {
		t.Fatal(err)
	}
	if data, err := d.ReadFile("member.json"); err != nil || string(data) != "owned" {
		t.Fatalf("acquired directory: %q %v", data, err)
	}
	if !blocked {
		if data, err := os.ReadFile(filepath.Join(out, "member.json")); err != nil || string(data) != "unrelated" {
			t.Fatalf("replacement directory changed: %q %v", data, err)
		}
	}
}

func TestOutputClaimProcess(t *testing.T) {
	if os.Getenv("TSGK_OUTPUT_CLAIM_HELPER") != "1" {
		return
	}
	base, id := os.Getenv("TSGK_OUTPUT_CLAIM_BASE"), os.Getenv("TSGK_OUTPUT_CLAIM_ID")
	if err := os.WriteFile(filepath.Join(base, "ready-"+id), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(base, "go")); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) || time.Now().After(deadline) {
			t.Fatalf("claim barrier: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	d, err := newOutputDirectory(filepath.Join(base, "output"), nil)
	if errors.Is(err, os.ErrExist) {
		fmt.Println("CLAIM_LOST", id)
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := createExclusive(d.Root, "winner.json", []byte(id)); err != nil {
		t.Fatal(err)
	}
	fmt.Println("CLAIM_WON", id)
}

func TestTwoProcessesClaimOneOutput(t *testing.T) {
	base := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var commands [2]*exec.Cmd
	var logs [2]bytes.Buffer
	var waited [2]bool
	defer func() {
		cancel()
		for i, command := range commands {
			if command != nil && command.Process != nil && !waited[i] {
				_ = command.Wait()
			}
		}
	}()
	for i := range commands {
		id := fmt.Sprint(i)
		commands[i] = exec.CommandContext(ctx, exe, "-test.run=^TestOutputClaimProcess$", "-test.timeout=10s")
		commands[i].Env = []string{"SystemRoot=" + os.Getenv("SystemRoot"), "TSGK_OUTPUT_CLAIM_HELPER=1", "TSGK_OUTPUT_CLAIM_BASE=" + base, "TSGK_OUTPUT_CLAIM_ID=" + id}
		commands[i].Stdout, commands[i].Stderr = &logs[i], &logs[i]
		if err := commands[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, e0 := os.Stat(filepath.Join(base, "ready-0"))
		_, e1 := os.Stat(filepath.Join(base, "ready-1"))
		if e0 == nil && e1 == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("both claim processes did not reach the barrier")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := os.WriteFile(filepath.Join(base, "go"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	winners, losers := 0, 0
	for i, command := range commands {
		err := command.Wait()
		waited[i] = true
		if err != nil {
			t.Fatalf("claim process %d: %v %s", i, err, logs[i].String())
		}
		winners += strings.Count(logs[i].String(), "CLAIM_WON")
		losers += strings.Count(logs[i].String(), "CLAIM_LOST")
	}
	entries, err := os.ReadDir(filepath.Join(base, "output"))
	if winners != 1 || losers != 1 || err != nil || len(entries) != 1 || entries[0].Name() != "winner.json" {
		t.Fatalf("claim result: winners=%d losers=%d entries=%v err=%v", winners, losers, entries, err)
	}
}

func TestOutputTargetDoesNotReopenPath(t *testing.T) {
	parent, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	display := filepath.Join(t.TempDir(), "missing-parent", "unused")
	d, err := newOutputDirectory(display, &OutputTarget{Parent: parent, Name: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	if err := createExclusive(d.Root, "member", []byte("owned")); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if data, err := parent.ReadFile("owned/member"); err != nil || string(data) != "owned" {
		t.Fatalf("target or caller parent lost: %q %v", data, err)
	}
	if _, err := os.Lstat(filepath.Dir(display)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("display pathname used: %v", err)
	}
	for _, target := range []*OutputTarget{{Name: "out"}, {Parent: parent, Name: ""}, {Parent: parent, Name: "."}, {Parent: parent, Name: "../escape"}, {Parent: parent, Name: "a/b"}} {
		if d, err := newOutputDirectory(display, target); d != nil || !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("invalid bound target: %v %v", d, err)
		}
	}
	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}
	if d, err := newOutputDirectory(display, &OutputTarget{Parent: parent, Name: "closed"}); d != nil || err == nil {
		t.Fatalf("closed parent fallback: %v %v", d, err)
	}
}
