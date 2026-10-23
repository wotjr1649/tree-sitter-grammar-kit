package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type zipEntry struct {
	name, data string
	mode       os.FileMode
}

func zipBytes(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Store}
		h.SetMode(e.mode)
		out, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = out.Write([]byte(e.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func recordedPlan(t *testing.T, control string, paginate bool) (downloadPlan, []byte, error) {
	t.Helper()
	run, pages, z := recordedMetadata(t, control, paginate)
	p, e := makePlan(expectation(), run, pages)
	return p, z, e
}

func recordedMetadata(t *testing.T, control string, paginate bool) ([]byte, [][]byte, []byte) {
	t.Helper()
	r := expectation()
	z := zipBytes(t, zipEntry{"sample.json", "evidence", 0600})
	var rows []artifactMetadata
	if paginate {
		for i := 0; i < 100; i++ {
			rows = append(rows, artifactMetadata{Name: "unrelated-" + strconv.Itoa(i)})
		}
	}
	for i, s := range artifacts(r) {
		a := artifactMetadata{ID: uint64(i + 1), Name: s.name, Size: int64(len(z)), Digest: "sha256:" + digest(z), Expired: new(bool)}
		a.Run.ID = 123
		a.Run.Head = r.HeadSHA
		rows = append(rows, a)
	}
	// the producer configs the margin plan reads from the same listing (#275)
	configs := len(rows)
	for i, j := range marginJobs {
		a := artifactMetadata{ID: uint64(500 + i), Name: configName(j, r), Size: int64(len(z)), Digest: "sha256:" + digest(z), Expired: new(bool)}
		a.Run.ID = 123
		a.Run.Head = r.HeadSHA
		rows = append(rows, a)
	}
	index := 0
	if paginate {
		index = 100
	}
	switch control {
	case "missing":
		rows = rows[:len(rows)-1]
	case "old-attempt":
		rows[index].Name = strings.TrimSuffix(rows[index].Name, "-2") + "-1"
	case "rerun", "rerun-lag", "rerun-config-lag":
		// a whole-run re-run also lists every artifact of attempt 1
		for i, n := index, len(rows); i < n; i++ {
			a := rows[i]
			a.ID += 1000
			a.Name = strings.TrimSuffix(a.Name, "-2") + "-1"
			rows = append(rows, a)
		}
		if control == "rerun-lag" {
			rows = slices.Delete(rows, index, index+1)
		}
		if control == "rerun-config-lag" {
			rows = slices.Delete(rows, configs, configs+1)
		}
	case "config-lag":
		rows = slices.Delete(rows, configs, configs+1)
	case "duplicate":
		rows = append(rows, rows[index])
	case "duplicate-id":
		rows[index+1].ID = rows[index].ID
	case "expired":
		*rows[index].Expired = true
	case "no-digest":
		rows[index].Digest = ""
	case "oversize":
		rows[index].Size = zipLimit + 1
	case "wrong-head":
		rows[index].Run.Head = strings.Repeat("c", 40)
	}
	id := 123
	if control == "wrong-run" {
		id = 124
	}
	run := encoded(t, map[string]any{"id": id, "run_attempt": 2, "event": "pull_request", "head_sha": r.HeadSHA, "path": ".github/workflows/foundation.yml", "repository": map[string]string{"full_name": repository}})
	var pages [][]byte
	for i := 0; i < len(rows); i += 100 {
		pages = append(pages, encoded(t, map[string]any{"total_count": len(rows), "artifacts": rows[i:min(i+100, len(rows))]}))
	}
	if control == "missing-page" {
		pages = pages[:1]
	}
	if control == "duplicate-json" {
		pages[0] = bytes.Replace(pages[0], []byte(`"total_count":`), []byte(`"total_count":0,"total_count":`), 1)
	}
	return run, pages, z
}

// TestPlanListingLagExit: only a plain absence asks fetch.ps1 to list again (#275).
func TestPlanListingLagExit(t *testing.T) {
	dir := t.TempDir()
	expected := filepath.Join(dir, "expected.json")
	if e := os.WriteFile(expected, encoded(t, expectation()), 0600); e != nil {
		t.Fatal(e)
	}
	for control, want := range map[string]int{"": 0, "rerun": 0, "missing": 3, "config-lag": 3, "old-attempt": 4, "rerun-lag": 4, "rerun-config-lag": 4, "wrong-head": 1} {
		t.Run("control="+control, func(t *testing.T) {
			runJSON, pages, _ := recordedMetadata(t, control, false)
			meta := filepath.Join(t.TempDir(), "metadata")
			if e := os.Mkdir(meta, 0700); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(filepath.Join(meta, "run.json"), runJSON, 0600); e != nil {
				t.Fatal(e)
			}
			for i, page := range pages {
				if e := os.WriteFile(filepath.Join(meta, "listing-"+strconv.Itoa(i+1)+".json"), page, 0600); e != nil {
					t.Fatal(e)
				}
			}
			var stderr bytes.Buffer
			out := filepath.Join(t.TempDir(), "plan.json")
			if got := run([]string{"plan", "-expected", expected, "-metadata", meta, "-out", out}, &stderr); got != want {
				t.Fatalf("exit %d, want %d: %s", got, want, stderr.String())
			}
			if _, e := os.Stat(out); (e == nil) != (want == 0) {
				t.Fatal("plan written for a failed listing or missing for a valid one")
			}
		})
	}
}
func TestDownloadPlanAndUnpack(t *testing.T) {
	for _, paginate := range []bool{false, true} {
		p, z, e := recordedPlan(t, "", paginate)
		if e != nil {
			t.Fatal(e)
		}
		dir := t.TempDir()
		root := filepath.Join(dir, "artifacts")
		if e = os.Mkdir(root, 0700); e != nil {
			t.Fatal(e)
		}
		planPath := filepath.Join(dir, "plan.json")
		if e = writeNewJSON(planPath, p, 65536); e != nil {
			t.Fatal(e)
		}
		for _, a := range p.Artifacts {
			if e = os.WriteFile(filepath.Join(root, a.Name+".zip"), z, 0600); e != nil {
				t.Fatal(e)
			}
		}
		if e = unpack(context.Background(), expectation(), root, planPath); e != nil {
			t.Fatal(e)
		}
		if e = checkDownloaded(root, expectation()); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(root, p.Artifacts[0].Name, "sample.json"), []byte("tamper"), 0600); e != nil {
			t.Fatal(e)
		}
		if checkDownloaded(root, expectation()) == nil {
			t.Fatal("changed extracted bytes accepted")
		}
	}
}
func TestDownloadPlanFailsClosed(t *testing.T) {
	for _, control := range []string{"missing", "old-attempt", "wrong-run", "wrong-head", "duplicate", "duplicate-id", "expired", "no-digest", "oversize", "missing-page", "duplicate-json"} {
		t.Run(control, func(t *testing.T) {
			_, _, e := recordedPlan(t, control, true)
			if e == nil {
				t.Fatal("invalid metadata accepted")
			}
			// only a plain absence may be listed again; an earlier attempt's artifact is a
			// partial re-run, and anything else stays a hard failure (#275)
			if errors.Is(e, errArtifactLag) != (control == "missing") || errors.Is(e, errPartialRerun) != (control == "old-attempt") {
				t.Fatalf("%s: wrong failure kind: %v", control, e)
			}
		})
	}
}
func TestUnpackDigestFailsClosed(t *testing.T) {
	p, z, e := recordedPlan(t, "", false)
	if e != nil {
		t.Fatal(e)
	}
	for _, control := range []string{"digest", "partial", "missing", "expired"} {
		t.Run(control, func(t *testing.T) {
			dir := t.TempDir()
			root := filepath.Join(dir, "artifacts")
			if e := os.Mkdir(root, 0700); e != nil {
				t.Fatal(e)
			}
			var copyPlan downloadPlan
			if e := json.Unmarshal(encoded(t, p), &copyPlan); e != nil {
				t.Fatal(e)
			}
			if control == "expired" {
				*copyPlan.Artifacts[0].Expired = true
			}
			planPath := filepath.Join(dir, "plan.json")
			if e := writeNewJSON(planPath, copyPlan, 65536); e != nil {
				t.Fatal(e)
			}
			for i, a := range copyPlan.Artifacts {
				data := append([]byte(nil), z...)
				if i == 0 {
					switch control {
					case "digest":
						data[len(data)-1] ^= 1
					case "partial":
						data = data[:len(data)-1]
					case "missing":
						continue
					}
				}
				if e := os.WriteFile(filepath.Join(root, a.Name+".zip"), data, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if e := unpack(context.Background(), expectation(), root, planPath); e == nil {
				t.Fatal("invalid ZIP input accepted")
			}
			if _, e := os.Stat(filepath.Join(root, "download.json")); !os.IsNotExist(e) {
				t.Fatal("failed unpack published completion")
			}
		})
	}
}

func TestZIPRejectsUnsafeEntriesAndLimits(t *testing.T) {
	controls := map[string][]zipEntry{
		"traversal": {{"../escape", "x", 0600}}, "absolute": {{"/escape", "x", 0600}}, "drive": {{"C:escape", "x", 0600}}, "ADS": {{"a:b", "x", 0600}},
		"backslash": {{`a\b`, "x", 0600}}, "reserved": {{"NUL.txt", "x", 0600}}, "trailing-dot": {{"a.", "x", 0600}},
		"link": {{"link", "../escape", os.ModeSymlink | 0777}}, "pipe": {{"pipe", "", os.ModeNamedPipe | 0600}},
		"link-dir": {{"link/", "", os.ModeSymlink | 0777}}, "pipe-dir": {{"pipe/", "", os.ModeNamedPipe | 0600}},
		"duplicate": {{"same", "a", 0600}, {"same", "b", 0600}}, "case": {{"A/a", "a", 0600}, {"a/b", "b", 0600}},
		"file-dir": {{"a", "a", 0600}, {"a/b", "b", 0600}}, "count": {{"a", "a", 0600}, {"b", "b", 0600}}, "bytes": {{"a", "12345", 0600}},
	}
	for name, entries := range controls {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "input.zip")
			if e := os.WriteFile(p, zipBytes(t, entries...), 0600); e != nil {
				t.Fatal(e)
			}
			spec := artifactSpec{"test", 10, 100}
			if name == "count" {
				spec.files = 1
			}
			if name == "bytes" {
				spec.bytes = 4
			}
			if _, e := extract(context.Background(), p, filepath.Join(dir, "out"), spec); e == nil {
				t.Fatal("unsafe ZIP accepted")
			}
			if _, e := os.Stat(filepath.Join(dir, "escape")); !os.IsNotExist(e) {
				t.Fatal("escaped extraction root")
			}
		})
	}
	t.Run("declared-underflow", func(t *testing.T) {
		z := zipBytes(t, zipEntry{"a", "12345", 0600})
		at := bytes.Index(z, []byte{'P', 'K', 1, 2})
		if at < 0 {
			t.Fatal("missing directory")
		}
		binary.LittleEndian.PutUint32(z[at+24:], 4)
		dir := t.TempDir()
		p := filepath.Join(dir, "input.zip")
		if e := os.WriteFile(p, z, 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := extract(context.Background(), p, filepath.Join(dir, "out"), artifactSpec{"test", 1, 4}); e == nil {
			t.Fatal("actual expansion over limit accepted")
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "input.zip")
		if e := os.WriteFile(p, zipBytes(t, zipEntry{"a", "ok", 0600}), 0600); e != nil {
			t.Fatal(e)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, e := extract(ctx, p, filepath.Join(dir, "out"), artifactSpec{"test", 1, 10}); e == nil {
			t.Fatal("cancelled extraction accepted")
		}
	})
}

func TestZIPDirectoryPreflight(t *testing.T) {
	var entries []zipEntry
	for i := 0; i < 300; i++ {
		entries = append(entries, zipEntry{strconv.Itoa(i), "", 0600})
	}
	good := zipBytes(t, entries...)
	for name, change := range map[string]func([]byte){
		"declared-excess": func([]byte) {},
		"lying-count": func(z []byte) {
			at := len(z) - 22
			binary.LittleEndian.PutUint16(z[at+8:], 1)
			binary.LittleEndian.PutUint16(z[at+10:], 1)
		},
		"zip64": func(z []byte) {
			at := len(z) - 22
			binary.LittleEndian.PutUint16(z[at+8:], 65535)
			binary.LittleEndian.PutUint16(z[at+10:], 65535)
		},
	} {
		t.Run(name, func(t *testing.T) {
			z := append([]byte(nil), good...)
			change(z)
			if e := checkZIPDirectory(bytes.NewReader(z), int64(len(z)), 257); e == nil {
				t.Fatal("excessive central directory accepted")
			}
		})
	}
}

type cancellingReader struct {
	cancel context.CancelFunc
	reads  int
}

func (r *cancellingReader) Read(b []byte) (int, error) {
	r.reads++
	r.cancel()
	for i := range b {
		b[i] = 'x'
	}
	return len(b), nil
}
func TestCopyStopsDuringEntry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &cancellingReader{cancel: cancel}
	n, e := io.Copy(io.Discard, io.LimitReader(contextReader{ctx, r}, 1<<20))
	if e != context.Canceled || n >= 1<<20 || r.reads != 1 {
		t.Fatalf("copy was not interrupted: n=%d reads=%d err=%v", n, r.reads, e)
	}
}
