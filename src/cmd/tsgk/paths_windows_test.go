package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

func TestSubstGuardRefusesDeveloperHost(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestHostedSubstMembership$", "-test.timeout=10s")
	cmd.Env = []string{"SystemRoot=" + os.Getenv("SystemRoot"), "TSGK_HOSTED_SUBST_TEST=1", "GITHUB_ACTIONS=false"}
	data, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 || !bytes.Contains(data, []byte("subst fixture requires a GitHub-hosted Windows runner")) {
		t.Fatalf("developer host guard: %v %s", err, data)
	}
}

// Only the dedicated hosted CI step opts in; never create mappings on a developer host.
func TestHostedSubstMembership(t *testing.T) {
	if os.Getenv("TSGK_HOSTED_SUBST_TEST") != "1" {
		t.Skip("requires the dedicated GitHub-hosted Windows step")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" || os.Getenv("RUNNER_OS") != "Windows" {
		t.Fatal("subst fixture requires a GitHub-hosted Windows runner")
	}
	base, err := os.MkdirTemp(os.Getenv("RUNNER_TEMP"), "tsgk-215-subst-")
	if err != nil {
		t.Fatal(err)
	}
	released := true
	defer func() {
		if !released {
			t.Error("mapping removal unconfirmed; fixture retained")
			return
		}
		if err := os.RemoveAll(base); err != nil {
			t.Error(err)
		}
	}()
	input, nested, outside := filepath.Join(base, "input"), filepath.Join(base, "input", "nested"), filepath.Join(base, "outside")
	for _, dir := range []string{input, nested, outside} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(nested, "original.txt"), []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	query := func(drive string) ([]string, error) {
		name, err := windows.UTF16PtrFromString(drive)
		if err != nil {
			return nil, err
		}
		buffer := make([]uint16, 32768)
		n, err := windows.QueryDosDevice(name, &buffer[0], uint32(len(buffer)))
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var targets []string
		for _, target := range strings.Split(string(utf16.Decode(buffer[:n])), "\x00") {
			if target != "" {
				targets = append(targets, target)
			}
		}
		return targets, nil
	}
	runSubst := func(args ...string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(os.Getenv("SystemRoot"), "System32", "subst.exe"), args...)
		cmd.Env = []string{"SystemRoot=" + os.Getenv("SystemRoot")}
		return cmd.Run()
	}
	withMapping := func(target string, check func(string)) {
		mask, err := windows.GetLogicalDrives()
		if err != nil {
			t.Fatal(err)
		}
		drive := ""
		for letter := byte('Z'); letter >= 'P'; letter-- {
			if mask&(1<<uint(letter-'A')) == 0 {
				drive = string(letter) + ":"
				break
			}
		}
		if drive == "" {
			t.Fatal("no unused drive for the hosted subst fixture")
		}
		if targets, err := query(drive); err != nil || len(targets) != 0 {
			t.Fatal("selected drive is not confirmed unused")
		}
		want := `\??\` + filepath.Clean(target)
		owned := func(targets []string) bool { return len(targets) == 1 && strings.EqualFold(targets[0], want) }
		removeExact := func(expected string) error {
			device, err := windows.UTF16PtrFromString(drive)
			if err != nil {
				return err
			}
			path, err := windows.UTF16PtrFromString(expected)
			if err != nil {
				return err
			}
			return windows.DefineDosDevice(windows.DDD_RAW_TARGET_PATH|windows.DDD_REMOVE_DEFINITION|windows.DDD_EXACT_MATCH_ON_REMOVE|windows.DDD_NO_BROADCAST_SYSTEM, device, path)
		}
		released = false
		defer func() {
			targets, err := query(drive)
			if err != nil || !owned(targets) {
				t.Error("mapping identity changed or is unavailable; refusing removal")
				return
			}
			if err := removeExact(want); err != nil {
				t.Error("subst removal failed:", err)
				return
			}
			if targets, err := query(drive); err != nil || len(targets) != 0 {
				t.Error("subst removal was not confirmed")
				return
			}
			released = true
			t.Log("SUBST_REMOVED")
		}()
		if err := runSubst(drive, target); err != nil {
			t.Fatal("subst creation failed:", err)
		}
		if targets, err := query(drive); err != nil || !owned(targets) {
			t.Fatal("subst target was not confirmed")
		}
		if err := removeExact(want + "-not-owned"); err == nil {
			t.Fatal("exact removal accepted a different target")
		}
		if targets, err := query(drive); err != nil || !owned(targets) {
			t.Fatal("exact removal changed a different mapping")
		}
		t.Log("SUBST_DIFFERENT_TARGET_PRESERVED")
		check(drive + `\`)
	}
	withMapping(nested, func(mapped string) {
		contained, err := inside(mapped, input)
		if err != nil || !contained {
			t.Fatalf("subst descendant identity missed: %v %v", contained, err)
		}
		if code, err := publish(filepath.Join(mapped, "result.json"), input, []byte("result")); code != "OUTPUT_INSIDE_INPUT" || err == nil {
			t.Fatalf("subst inside publication: %s %v", code, err)
		}
		entries, err := os.ReadDir(nested)
		if err != nil || len(entries) != 1 || entries[0].Name() != "original.txt" {
			t.Fatalf("inside refusal left partial entries: %v %v", entries, err)
		}
		if data, err := os.ReadFile(filepath.Join(nested, "original.txt")); err != nil || string(data) != "unchanged" {
			t.Fatal("input bytes changed")
		}
		t.Log("SUBST_INSIDE_REJECTED")
	})
	if !released {
		t.Fatal("previous mapping cleanup unconfirmed; refusing another mapping")
	}
	withMapping(outside, func(mapped string) {
		if code, err := publish(filepath.Join(mapped, "result.json"), input, []byte("outside result")); code != "" || err != nil {
			t.Fatalf("subst outside publication: %s %v", code, err)
		}
		if data, err := os.ReadFile(filepath.Join(outside, "result.json")); err != nil || string(data) != "outside result" {
			t.Fatalf("outside result bytes: %q %v", data, err)
		}
		t.Log("SUBST_OUTSIDE_PUBLISHED")
	})
}
