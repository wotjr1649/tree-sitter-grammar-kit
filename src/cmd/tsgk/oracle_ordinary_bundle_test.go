package main

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These tests run the whole source-only loader on the real inputs of the record end-to-end test
// (src/kit TestOrdinaryRecordCLIOwnedPostgreSQLPublicParity writes them under .work). They do not run
// native code and are skipped when no bundle directory is given.
const ordinaryBundleRegistrySHA = "ffd3552dee2d6a9b476b9de76195d2c118bb58de9079ab76dfb1e469d34aa3c8"

type ordinaryBundle struct {
	dir   string
	roots []string
	req   ordinaryLoadRequest
	size  uint64 // bytes of every input file, counted from the file system
}

func ordinaryBundleCopy(t *testing.T) ordinaryBundle {
	t.Helper()
	src := os.Getenv("TSGK_ORDINARY_CLI_BUNDLE_DIR")
	if src == "" {
		t.Skip("set TSGK_ORDINARY_CLI_BUNDLE_DIR to a bundle written by the record end-to-end test")
	}
	dst, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := ordinaryBundle{dir: dst}
	for _, name := range []string{"ordinary", "witness.json", "plan.json"} {
		err := filepath.WalkDir(filepath.Join(src, name), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			target := filepath.Join(dst, rel)
			if entry.IsDir() {
				return os.MkdirAll(target, 0o700)
			}
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			defer in.Close()
			info, err := in.Stat()
			if err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, in); err != nil {
				out.Close()
				return err
			}
			b.size += uint64(info.Size())
			return out.Close()
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dst, "cases"), 0o700); err != nil {
		t.Fatal(err)
	}
	b.roots = []string{filepath.Join(dst, "cases")}
	b.req = ordinaryLoadRequest{Root: filepath.Join(dst, "ordinary"), Witness: filepath.Join(dst, "witness.json"), Plan: filepath.Join(dst, "plan.json"),
		Profile: "profile/tsgk-oracle-r4.json", Assets: "profile/ordinary-assets-r1.json", RegistrySHA256: ordinaryBundleRegistrySHA,
		Registrations: []string{"registrations/postgresql-sql/windows-amd64.json"}, ShardIDs: []string{"postgresql-sql/windows-amd64/alt-postgresql-sql-B04-22"},
		HasFactPack: true, InputRoots: b.roots}
	return b
}

func (b ordinaryBundle) load(t *testing.T, l *ordinaryLedger) (*ordinaryLoaded, error) {
	t.Helper()
	return loadOrdinaryRecording(context.Background(), l, b.req)
}

func TestOrdinaryLoaderReadsTheRealInputsAndCountsWhatItRead(t *testing.T) {
	b := ordinaryBundleCopy(t)
	l := newOrdinaryLedger()
	loaded, err := b.load(t, l)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	if loaded.recording == nil || len(loaded.recording.Registration) == 0 || len(loaded.factMapping) == 0 {
		t.Fatalf("the loader did not return the recording inputs: %+v", loaded)
	}
	if err := loaded.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Every file is read once and opened again to verify it; the independent count is the bytes on disk.
	if l.read != 2*b.size || l.entries != 2*uint64(len(loaded.bound)) || l.logical < 5*b.size || l.work == 0 {
		t.Fatalf("ledger differs from what was read: ledger read=%d entries=%d logical=%d work=%d; disk bytes=%d files=%d", l.read, l.entries, l.logical, l.work, b.size, len(loaded.bound))
	}
	t.Logf("real loader ledger: read=%d work=%d logical=%d entries=%d files=%d disk-bytes=%d", l.read, l.work, l.logical, l.entries, len(loaded.bound), b.size)
}

// The limit applies to the cumulative ledger of the whole load: the totals the load reached are
// accepted as limits and any one of them minus one stops the loader with a resource error.
func TestOrdinaryLoaderStopsAtEveryCumulativeLimitPlusOne(t *testing.T) {
	b := ordinaryBundleCopy(t)
	probe := newOrdinaryLedger()
	loaded, err := b.load(t, probe)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Close()
	exact := *probe
	for _, tc := range []struct {
		name string
		set  func(l *ordinaryLedger, d uint64)
	}{
		{"entries", func(l *ordinaryLedger, d uint64) { l.limits.Entries = exact.entries - d }},
		{"read", func(l *ordinaryLedger, d uint64) { l.limits.ReadBytes = exact.read - d }},
		{"logical", func(l *ordinaryLedger, d uint64) { l.limits.Logical = exact.logical - d }},
		{"work", func(l *ordinaryLedger, d uint64) { l.limits.Work = exact.work - d }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := newOrdinaryLedger()
			tc.set(l, 0)
			loaded, err := b.load(t, l)
			if err != nil {
				t.Fatalf("a load that reaches the limit exactly must be accepted: %v", err)
			}
			loaded.Close()
			l = newOrdinaryLedger()
			tc.set(l, 1)
			loaded, err = b.load(t, l)
			if ordinaryCode(err) != "RESOURCE_LIMIT/ORDINARY_RESOURCE_LIMIT" || loaded != nil {
				loaded.Close()
				t.Fatalf("a load one over the %s limit must stop with a resource error: %v", tc.name, err)
			}
		})
	}
	t.Run("wall", func(t *testing.T) {
		now := time.Unix(5000, 0)
		old := ordinaryClock
		ordinaryClock = func() time.Time { now = now.Add(time.Second); return now }
		t.Cleanup(func() { ordinaryClock = old })
		l := newOrdinaryLedger()
		l.limits.WallMillis = 1
		if loaded, err := b.load(t, l); ordinaryCode(err) != "RESOURCE_LIMIT/ORDINARY_WALL_LIMIT" || loaded != nil {
			t.Fatalf("a load past the wall limit must stop: %v", err)
		}
	})
}

func TestOrdinaryLoaderRejectsAFileChangedBetweenTheCheckAndTheUse(t *testing.T) {
	registration := filepath.FromSlash("registrations/postgresql-sql/windows-amd64.json")
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, b ordinaryBundle)
		reject bool
	}{
		{"unchanged", func(t *testing.T, b ordinaryBundle) {}, false},
		{"content-changed-same-size", func(t *testing.T, b ordinaryBundle) {
			path := filepath.Join(b.req.Root, registration)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data[len(data)/2] ^= 1
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"replaced-by-identical-bytes", func(t *testing.T, b ordinaryBundle) {
			path := filepath.Join(b.req.Root, registration)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"plan-replaced-by-identical-bytes", func(t *testing.T, b ordinaryBundle) {
			data, err := os.ReadFile(b.req.Plan)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(b.req.Plan); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(b.req.Plan, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"witness-deleted", func(t *testing.T, b ordinaryBundle) {
			if err := os.Remove(b.req.Witness); err != nil {
				t.Fatal(err)
			}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := ordinaryBundleCopy(t)
			l := newOrdinaryLedger()
			loaded, err := b.load(t, l)
			if err != nil {
				t.Fatal(err)
			}
			defer loaded.Close()
			testHookOrdinaryBeforeVerify = func() { tc.change(t, b) }
			t.Cleanup(func() { testHookOrdinaryBeforeVerify = nil })
			err = loaded.Verify(context.Background())
			if tc.reject && ordinaryCode(err) != "INVALID_INPUT/ORDINARY_INPUT_CHANGED" {
				t.Fatalf("a file changed between the check and the use must be rejected: %v", err)
			}
			if !tc.reject && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOrdinaryLoaderRejectsTwoPathsToOneFile(t *testing.T) {
	b := ordinaryBundleCopy(t)
	alias := "registrations/postgresql-sql/alias.json"
	if err := os.Link(filepath.Join(b.req.Root, filepath.FromSlash(b.req.Registrations[0])), filepath.Join(b.req.Root, filepath.FromSlash(alias))); err != nil {
		t.Skipf("hard links are unavailable: %v", err)
	}
	b.req.Registrations = append(b.req.Registrations, alias)
	loaded, err := b.load(t, newOrdinaryLedger())
	loaded.Close()
	if ordinaryCode(err) != "INVALID_INPUT/ORDINARY_PATH_ALIAS" {
		t.Fatalf("two registration paths to one file must be refused as an alias: %v", err)
	}
	// A trusted document inside an input root is refused before it is read.
	b = ordinaryBundleCopy(t)
	b.roots = append(b.roots, filepath.Dir(b.req.Plan))
	b.req.InputRoots = b.roots
	loaded, err = b.load(t, newOrdinaryLedger())
	loaded.Close()
	if ordinaryCode(err) != "INVALID_INPUT/ORDINARY_INPUT_INSIDE_CASE_ROOT" {
		t.Fatalf("a witness inside an input root must be refused: %v", err)
	}
}

func TestOrdinaryLoaderKeepsTheFirstFailureAndRefusesAWrongSelection(t *testing.T) {
	b := ordinaryBundleCopy(t)
	l := newOrdinaryLedger()
	l.limits.Entries = 1
	loaded, first := b.load(t, l)
	loaded.Close()
	if ordinaryCode(first) != "RESOURCE_LIMIT/ORDINARY_RESOURCE_LIMIT" {
		t.Fatalf("setup: %v", first)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	loaded, again := loadOrdinaryRecording(canceled, l, b.req)
	loaded.Close()
	if again != first {
		t.Fatalf("a later cancellation replaced the first failure: %v", again)
	}
	b = ordinaryBundleCopy(t)
	b.req.ShardIDs = []string{"tsql/windows-amd64/alt-tsql-B01-r001"}
	loaded, err := b.load(t, newOrdinaryLedger())
	loaded.Close()
	if ordinaryCode(err) != "INVALID_INPUT/ORDINARY_REGISTRATION_SELECTION" {
		t.Fatalf("a shard without a registration must be refused: %v", err)
	}
}
