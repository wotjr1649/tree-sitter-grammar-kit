package foundation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise the documented Git primitive, not a live cleanup service.
func TestConditionalRemoteDeletion(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	if err := os.Mkdir(empty, 0700); err != nil {
		t.Fatal(err)
	}
	// No user Git configuration, template hooks, credentials, or network transport.
	env := []string{"PATH=" + os.Getenv("PATH"), "SystemRoot=" + os.Getenv("SystemRoot"),
		"HOME=" + root, "USERPROFILE=" + root, "TMP=" + root, "TEMP=" + root,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=file"}
	run := func(wantOK bool, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, git, args...)
		cmd.Dir, cmd.Env = root, env
		out, err := cmd.CombinedOutput()
		if ctx.Err() != nil || (err == nil) != wantOK {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	run(true, "init", "--bare", "--initial-branch=main", "--template="+empty, remote)
	run(true, "init", "--initial-branch=main", "--template="+empty, work)
	commit := func(message string) string {
		run(true, "-C", work, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", message)
		return run(true, "-C", work, "rev-parse", "HEAD")
	}
	first := commit("first")
	run(true, "-C", work, "push", remote, "HEAD:refs/heads/main", "HEAD:refs/heads/matched", "HEAD:refs/heads/stale", "HEAD:refs/heads/denied")
	second := commit("second")
	run(true, "-C", work, "push", remote, "HEAD:refs/heads/stale")
	deletion := func(name, expected string, ok bool) string {
		return run(ok, "-C", work, "push", "--porcelain", "--force-with-lease=refs/heads/"+name+":"+expected, remote, ":refs/heads/"+name)
	}
	deletion("matched", first, true)
	if got := run(true, "--git-dir="+remote, "for-each-ref", "--format=%(refname)", "refs/heads/matched"); got != "" {
		t.Fatal("matched ref survived deletion")
	}
	if out := deletion("stale", first, false); !strings.Contains(out, "[rejected]") {
		t.Fatalf("missing lease rejection: %s", out)
	}
	run(true, "--git-dir="+remote, "config", "receive.denyDeletes", "true")
	if out := deletion("denied", first, false); !strings.Contains(out, "[remote rejected]") {
		t.Fatalf("missing server rejection: %s", out)
	}
	for name, want := range map[string]string{"main": first, "stale": second, "denied": first} {
		if got := run(true, "--git-dir="+remote, "rev-parse", "refs/heads/"+name); got != want {
			t.Fatalf("%s moved: %s != %s", name, got, want)
		}
	}
}
