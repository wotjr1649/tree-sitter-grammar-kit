package kit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

// The owned-native regressions each run one real producer (a test of src/internal/native). The package shares
// these things between them, none of which is part of a check:
//   - the native test binary and the tsgk CLI, each built once instead of once per producer or per test;
//   - the native driver build cache (OracleRequest.BuildCache): one warm-up producer compiles the pinned JSON grammar
//     and runtime for real, every later producer reuses that verified executable (re-hashed before every launch).
//
// Every producer still runs its own native process, case inputs, r8 set and temporary directories.

// ordinaryTempDirs holds the directories shared by the package's tests; TestMain removes them.
var ordinaryTempDirs sync.Map

// TestMain removes the shared producer and CLI directories after the package's tests.
func TestMain(m *testing.M) {
	code := m.Run()
	ordinaryTempDirs.Range(func(dir, _ any) bool {
		_ = os.RemoveAll(dir.(string))
		return true
	})
	os.Exit(code)
}

type ordinaryProducerShare struct {
	exe    string // the native package test binary
	cache  string // the shared native build cache directory
	dir    string // the directory of this share
	warm   sync.Mutex
	warmed map[string]bool // keyed by fixture and grammar: each grammar has its own cache key
}

var ordinaryProducerShareOnce = sync.OnceValues(func() (*ordinaryProducerShare, error) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "tsgk-kit-producer-")
	if err != nil {
		return nil, err
	}
	ordinaryTempDirs.Store(dir, true)
	share := &ordinaryProducerShare{dir: dir, cache: filepath.Join(dir, "build-cache"), exe: filepath.Join(dir, "native.test.exe"), warmed: map[string]bool{}}
	if err := os.Mkdir(share.cache, 0o700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "test", "-c", "-o", share.exe, "./src/internal/native")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("building the native test binary: %w\n%s", err, output)
	}
	return share, nil
})

// ordinaryRunNativeFixture runs one native fixture test as its own process in the native package directory, like
// `go test ./src/internal/native -run ^name$ -count=1 -timeout=180s`. A non-empty buildKey (the grammar, which the
// native build cache key includes) makes the producer use the shared driver build cache. The first producer of each
// fixture and buildKey is preceded by one serialized warm-up run of the same inputs into a throwaway output
// directory, so concurrent first runs do not each compile and race to publish one cache entry.
func ordinaryRunNativeFixture(t *testing.T, name, buildKey string, env []string) ([]byte, error) {
	t.Helper()
	share, err := ordinaryProducerShareOnce()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.Abs(filepath.Join("..", "internal", "native"))
	if err != nil {
		t.Fatal(err)
	}
	run := func(env []string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, share.exe, "-test.run", "^"+name+"$", "-test.count=1", "-test.timeout=180s")
		cmd.Dir, cmd.Env = dir, env
		return cmd.CombinedOutput()
	}
	// The opt-in HARD receipt check reads the compile steps of this run, which a build-cache hit does not execute.
	if buildKey == "" || slices.Contains(env, "TSGK_E2E_ENFORCE_NATIVE_LIMITS=1") {
		return run(env)
	}
	key := name + "\x00" + buildKey
	env = append(slices.Clone(env), "TSGK_ORDINARY_OWNED_BUILD_CACHE="+share.cache)
	share.warm.Lock() // concurrent first runs would race to publish one cache entry
	if !share.warmed[key] {
		warmDir := filepath.Join(share.dir, fmt.Sprintf("warm-up-%d", len(share.warmed)))
		warmEnv := append(slices.Clone(env), "TSGK_ORDINARY_OWNED_FIXTURE="+warmDir)
		if output, err := run(warmEnv); err != nil {
			share.warm.Unlock()
			return output, fmt.Errorf("build cache warm-up: %w", err)
		}
		share.warmed[key] = true
	}
	share.warm.Unlock()
	return run(env)
}

// ordinaryFullPosCLIBuild builds the tsgk CLI once per package run.
var ordinaryFullPosCLIBuild = sync.OnceValues(func() (string, error) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "tsgk-kit-cli-")
	if err != nil {
		return "", err
	}
	ordinaryTempDirs.Store(dir, true)
	exe := filepath.Join(dir, "tsgk.exe")
	build := exec.Command("go", "build", "-o", exe, "./src/cmd/tsgk")
	build.Dir = repo
	if output, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building the CLI: %w\n%s", err, output)
	}
	return exe, nil
})
