package native

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const buildCacheSchema = "tsgk-native-build-cache/r1"
const buildCacheRecordLimit = 1 << 20
const buildCacheExecutableLimit = 128 << 20
const buildCacheEntries = 64

// BuildReuse distinguishes this invocation's compilation from trusted earlier steps.
type BuildReuse struct {
	Key         string      `json:"key"`
	Hit         bool        `json:"hit"`
	SourceSteps []BuildStep `json:"source_steps"`
}

type cachedBuild struct {
	Schema          string      `json:"schema"`
	Key             string      `json:"key"`
	Identity        string      `json:"identity"`
	ExecutableSHA   string      `json:"executable_sha256"`
	ExecutableBytes int64       `json:"executable_bytes"`
	CompilerVersion string      `json:"compiler_version"`
	Steps           []BuildStep `json:"steps"`
}

func cacheDirectory(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.New("cache directory must be absolute")
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("cache directory must be a regular directory")
	}
	return nil
}

func (b *Build) reuseBuild(req BuildRequest, plan []BuildStep) (bool, error) {
	if err := cacheDirectory(req.CacheDir); err != nil {
		return false, err
	}
	// Temp paths vary per invocation; all other effective compiler environment values bind reuse.
	keyText := fmt.Sprintf("%s\n%s\ncompiler=%q\ncgroup=%q\nlimits=%d/%d/%d/%d\n", buildCacheSchema, b.Identity, req.Compiler, req.CgroupParent, buildWall, buildOutput, buildMemory, buildGrace)
	for _, kv := range b.env {
		name, _, _ := strings.Cut(kv, "=")
		if name != "TMP" && name != "TEMP" && name != "TMPDIR" {
			keyText += fmt.Sprintf("env=%q\n", kv)
		}
	}
	sum := sha256.Sum256([]byte(keyText))
	b.Reuse = &BuildReuse{Key: hex.EncodeToString(sum[:]), SourceSteps: []BuildStep{}}
	entry := filepath.Join(req.CacheDir, b.Reuse.Key)
	if _, err := os.Lstat(entry); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if err := cacheDirectory(entry); err != nil {
		return false, err
	}
	p := filepath.Join(entry, "build.json")
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() || info.Size() > buildCacheRecordLimit {
		return false, errors.New("cache record missing, special or oversized")
	}
	f, err := os.Open(p)
	if err != nil {
		return false, err
	}
	data, err := io.ReadAll(io.LimitReader(f, buildCacheRecordLimit+1))
	err = errors.Join(err, f.Close())
	if err != nil || len(data) > buildCacheRecordLimit {
		return false, errors.New("cache record unreadable or oversized")
	}
	var record cachedBuild
	if err := jsonv2.Unmarshal(data, &record, jsonv2.RejectUnknownMembers(true)); err != nil {
		return false, err
	}
	if record.Schema != buildCacheSchema || record.Key != b.Reuse.Key || record.Identity != b.Identity ||
		record.CompilerVersion == "" || record.ExecutableBytes <= 0 || record.ExecutableBytes > buildCacheExecutableLimit || len(record.ExecutableSHA) != 64 || len(record.Steps) != len(plan) {
		return false, errors.New("cache record identity or bounds mismatch")
	}
	for i, step := range record.Steps {
		if step.Name != plan[i].Name || !slices.Equal(step.Argv, plan[i].Argv) || !step.Executed || !step.Result.Succeeded() {
			return false, errors.New("cache contains an unsuccessful compilation")
		}
	}
	b.Executable = filepath.Join(b.Dir, exeName())
	if err := copyCacheExecutable(filepath.Join(entry, exeName()), b.Executable, record.ExecutableSHA, record.ExecutableBytes); err != nil {
		return false, err
	}
	if err := os.Chmod(b.Executable, 0o755); err != nil {
		return false, err
	}
	b.ExecutableSHA256, b.ExecutableBytes = record.ExecutableSHA, record.ExecutableBytes
	b.CompilerVersion, b.Steps = record.CompilerVersion, []BuildStep{}
	b.Reuse.Hit, b.Reuse.SourceSteps = true, record.Steps
	return true, b.Verify()
}

func (b *Build) saveBuild(cache string) error {
	if b.ExecutableBytes > buildCacheExecutableLimit {
		return errors.New("cache executable exceeds limit")
	}
	record := cachedBuild{buildCacheSchema, b.Reuse.Key, b.Identity, b.ExecutableSHA256, b.ExecutableBytes, b.CompilerVersion, b.Steps}
	data, err := json.Marshal(record)
	if err != nil || len(data) > buildCacheRecordLimit {
		return errors.New("cache record exceeds limit")
	}
	dir, err := os.Open(cache)
	if err != nil {
		return err
	}
	entries, err := dir.ReadDir(buildCacheEntries)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	if err = errors.Join(err, dir.Close()); err != nil {
		return err
	}
	if len(entries) >= buildCacheEntries {
		return errors.New("cache entry limit reached")
	}
	entry := filepath.Join(cache, b.Reuse.Key)
	if err := os.Mkdir(entry, 0o700); err != nil {
		return err // Sequential orchestration only: a competing publisher is a failure.
	}
	if err := copyCacheExecutable(b.Executable, filepath.Join(entry, exeName()), b.ExecutableSHA256, b.ExecutableBytes); err != nil {
		return err
	}
	// Publish completeness last; an incomplete entry is an error, never a cache miss.
	f, err := os.OpenFile(filepath.Join(entry, "build.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return errors.Join(err, f.Close())
}

func copyCacheExecutable(src, dst, expectedSHA string, expectedBytes int64) error {
	info, err := os.Lstat(src)
	if err != nil || !info.Mode().IsRegular() || info.Size() != expectedBytes || expectedBytes <= 0 || expectedBytes > buildCacheExecutableLimit {
		return errors.New("cache executable type or size mismatch")
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(f, buildCacheExecutableLimit+1))
	err = errors.Join(err, f.Close())
	sum := sha256.Sum256(data)
	if err != nil || int64(len(data)) != expectedBytes || hex.EncodeToString(sum[:]) != expectedSHA {
		return errors.New("cache executable identity mismatch")
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return errors.Join(err, out.Close())
}
