package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const zipLimit = 100 << 20

type artifactMetadata struct {
	ID      uint64 `json:"id"`
	Name    string `json:"name"`
	Size    int64  `json:"size_in_bytes"`
	Digest  string `json:"digest"`
	Expired *bool  `json:"expired"`
	Run     struct {
		ID   uint64 `json:"id"`
		Head string `json:"head_sha"`
	} `json:"workflow_run"`
}
type fileDigest struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type downloadedArtifact struct {
	Name  string                `json:"name"`
	ID    uint64                `json:"id"`
	ZIP   fileDigest            `json:"zip"`
	Files map[string]fileDigest `json:"files"`
}
type downloadReceipt struct {
	Schema    string               `json:"schema"`
	Run       kit.RunIdentity      `json:"run"`
	Artifacts []downloadedArtifact `json:"artifacts"`
}
type downloadPlan struct {
	Schema    string             `json:"schema"`
	Run       kit.RunIdentity    `json:"run"`
	Artifacts []artifactMetadata `json:"artifacts"`
}

// #275: a complete listing that lacks a current-attempt artifact is GitHub's listing lag,
// which fetch.ps1 may list again, or a partial re-run, whose kept producers have only their
// earlier-attempt artifacts. A whole-run re-run lists the earlier attempts' artifacts too, so
// an earlier name alone does not prove a partial re-run: fetch.ps1 lists that case again as
// well and names the cause only if it persists. The gate never accepts an earlier attempt.
var (
	errArtifactLag  = errors.New("required artifact missing for current attempt")
	errPartialRerun = errors.New("a required artifact exists only for an earlier attempt: a partial re-run cannot pass, re-run all jobs (or the listing still lags)")
)

// earlierAttempt reports whether a listed name is want (`<prefix>-<run>-<attempt>`) for an
// earlier attempt of the same run.
func earlierAttempt(listed []string, want string, r kit.RunIdentity) bool {
	attempt, err := strconv.ParseUint(r.RunAttempt, 10, 64)
	prefix, ok := strings.CutSuffix(want, "-"+r.RunAttempt)
	if err != nil || !ok {
		return false
	}
	for _, name := range listed {
		if k, ok := strings.CutPrefix(name, prefix+"-"); ok {
			if n, err := strconv.ParseUint(k, 10, 64); err == nil && n > 0 && n < attempt && strconv.FormatUint(n, 10) == k {
				return true
			}
		}
	}
	return false
}

func validArtifact(a artifactMetadata, expected kit.RunIdentity) bool {
	return a.ID != 0 && a.Expired != nil && !*a.Expired && a.Size > 0 && a.Size <= zipLimit && strings.HasPrefix(a.Digest, "sha256:") && hex64.MatchString(strings.TrimPrefix(a.Digest, "sha256:")) && strconv.FormatUint(a.Run.ID, 10) == expected.RunID && a.Run.Head == expected.HeadSHA
}

func makePlan(expected kit.RunIdentity, runBytes []byte, pages [][]byte) (downloadPlan, error) {
	var result downloadPlan
	var run struct {
		ID         uint64 `json:"id"`
		Attempt    uint64 `json:"run_attempt"`
		Event      string `json:"event"`
		Head       string `json:"head_sha"`
		Path       string `json:"path"`
		Repository struct {
			Name string `json:"full_name"`
		} `json:"repository"`
	}
	if decode(runBytes, &run) != nil || strconv.FormatUint(run.ID, 10) != expected.RunID || strconv.FormatUint(run.Attempt, 10) != expected.RunAttempt || run.Head != expected.HeadSHA || run.Event != expected.Event || run.Path != ".github/workflows/foundation.yml" || run.Repository.Name != expected.Repository {
		return result, errors.New("GitHub workflow run mismatch")
	}
	if len(pages) == 0 || len(pages) > 10 {
		return result, errors.New("artifact listing limit")
	}
	selected := map[string]artifactMetadata{}
	ids := map[uint64]bool{}
	var names []string
	var listed uint64
	var total uint64
	for i, data := range pages {
		var listing struct {
			Artifacts []artifactMetadata `json:"artifacts"`
			Total     *uint64            `json:"total_count"`
		}
		if decode(data, &listing) != nil || listing.Total == nil || *listing.Total > 1000 || i > 0 && *listing.Total != total {
			return result, errors.New("invalid artifact listing")
		}
		total = *listing.Total
		if listed > total || uint64(len(listing.Artifacts)) != min(uint64(100), total-listed) || i < len(pages)-1 && listed+uint64(len(listing.Artifacts)) >= total {
			return result, errors.New("incomplete or repeated listing page")
		}
		listed += uint64(len(listing.Artifacts))
		for _, a := range listing.Artifacts {
			names = append(names, a.Name)
			for _, spec := range artifacts(expected) {
				if a.Name != spec.name {
					continue
				}
				if _, exists := selected[a.Name]; exists || ids[a.ID] || !validArtifact(a, expected) {
					return result, errors.New("invalid or duplicate artifact metadata")
				}
				selected[a.Name] = a
				ids[a.ID] = true
			}
		}
	}
	if listed != total {
		return result, errors.New("artifact listing incomplete")
	}
	if len(selected) != 5 {
		for _, spec := range artifacts(expected) {
			if _, ok := selected[spec.name]; !ok && earlierAttempt(names, spec.name, expected) {
				return result, errPartialRerun
			}
		}
		return result, errArtifactLag
	}
	result.Schema = "tsgk-ci-download-plan/r1"
	result.Run = expected
	for _, spec := range artifacts(expected) {
		result.Artifacts = append(result.Artifacts, selected[spec.name])
	}
	return result, nil
}

func planFiles(expected kit.RunIdentity, dir string) (downloadPlan, error) {
	var result downloadPlan
	run, e := readFile(filepath.Join(dir, "run.json"), 1048576)
	if e != nil {
		return result, e
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) < 2 || len(entries) > 11 {
		return result, errors.New("metadata file count invalid")
	}
	var pages [][]byte
	for i := 1; i < len(entries); i++ {
		b, e := readFile(filepath.Join(dir, "listing-"+strconv.Itoa(i)+".json"), 1048576)
		if e != nil {
			return result, e
		}
		pages = append(pages, b)
	}
	return makePlan(expected, run, pages)
}

func unpack(ctx context.Context, expected kit.RunIdentity, root, planPath string) error {
	data, e := readFile(planPath, 65536)
	if e != nil {
		return e
	}
	var plan downloadPlan
	if decode(data, &plan) != nil || plan.Schema != "tsgk-ci-download-plan/r1" || !sameRun(&plan.Run, expected) || len(plan.Artifacts) != 5 {
		return errors.New("download plan invalid")
	}
	entries, e := os.ReadDir(root)
	if e != nil || len(entries) != 5 {
		return errors.New("ZIP input count invalid")
	}
	specs := artifacts(expected)
	ids := map[uint64]bool{}
	receipt := downloadReceipt{Schema: "tsgk-ci-download/r1", Run: expected}
	for i, a := range plan.Artifacts {
		spec := specs[i]
		if a.Name != spec.name || !validArtifact(a, expected) || ids[a.ID] {
			return errors.New("download plan artifact invalid")
		}
		ids[a.ID] = true
		zp := filepath.Join(root, spec.name+".zip")
		b, e := readFile(zp, zipLimit)
		if e != nil || int64(len(b)) != a.Size || "sha256:"+digest(b) != a.Digest {
			return errors.New("ZIP digest mismatch or incomplete download")
		}
		files, e := extract(ctx, zp, filepath.Join(root, spec.name), spec)
		if e != nil {
			return e
		}
		receipt.Artifacts = append(receipt.Artifacts, downloadedArtifact{Name: spec.name, ID: a.ID, ZIP: fileDigest{digest(b), int64(len(b))}, Files: files})
	}
	if ctx.Err() != nil {
		return errors.New("unpack deadline")
	}
	return writeNewJSON(filepath.Join(root, "download.json"), receipt, 4194304)
}

func writeNewJSON(path string, value any, limit int) error {
	b, e := json.Marshal(value)
	if e != nil || len(b)+1 > limit {
		return errors.New("result encoding limit")
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return errors.New("new result unavailable")
	}
	_, e = f.Write(append(b, '\n'))
	ce := f.Close()
	if e != nil || ce != nil {
		return errors.New("result write failed")
	}
	return nil
}

func entryPath(name string) (string, error) {
	if name == "" || len(name) > 256 || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\"*?<>|~") {
		return "", errors.New("unsafe ZIP path")
	}
	for _, c := range name {
		if c < 32 || c > 126 {
			return "", errors.New("nonportable ZIP path")
		}
	}
	name = strings.TrimSuffix(name, "/")
	if name == "" || path.Clean(name) != name {
		return "", errors.New("noncanonical ZIP path")
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", errors.New("unsafe ZIP component")
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9' {
			return "", errors.New("reserved ZIP component")
		}
	}
	return name, nil
}

func extract(ctx context.Context, zipPath, dest string, spec artifactSpec) (map[string]fileDigest, error) {
	f, err := os.Open(zipPath)
	if err != nil {
		return nil, errors.New("invalid ZIP")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > zipLimit {
		return nil, errors.New("ZIP size limit")
	}
	r := contextReaderAt{ctx, f}
	if err := checkZIPDirectory(r, st.Size(), spec.files+256); err != nil {
		return nil, err
	}
	z, err := zip.NewReader(r, st.Size())
	if err != nil {
		return nil, errors.New("invalid ZIP")
	}
	if len(z.File) == 0 || len(z.File) > spec.files+256 {
		return nil, errors.New("ZIP entry count limit")
	}
	if os.Mkdir(dest, 0700) != nil {
		return nil, errors.New("extract root must be new")
	}
	seen := map[string]bool{}
	canonical := map[string]string{}
	kinds := map[string]bool{}
	files := map[string]fileDigest{}
	var total int64
	for _, item := range z.File {
		if ctx.Err() != nil {
			return nil, errors.New("extraction deadline")
		}
		name, err := entryPath(item.Name)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, errors.New("duplicate ZIP path")
		}
		seen[key] = true
		mode := item.Mode()
		if mode.Type() != 0 && mode.Type() != os.ModeDir {
			return nil, errors.New("ZIP link or special entry")
		}
		parts := strings.Split(name, "/")
		for i := range parts {
			prefix := strings.Join(parts[:i+1], "/")
			folded := strings.ToLower(prefix)
			isDir := i < len(parts)-1 || mode.IsDir()
			if prior, ok := canonical[folded]; ok && (prior != prefix || kinds[folded] != isDir) {
				return nil, errors.New("ZIP component collision")
			}
			canonical[folded] = prefix
			kinds[folded] = isDir
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		if mode.IsDir() {
			if item.UncompressedSize64 != 0 || os.MkdirAll(target, 0700) != nil {
				return nil, errors.New("invalid ZIP directory")
			}
			continue
		}
		if len(files) >= spec.files || item.UncompressedSize64 > 67108864 || item.UncompressedSize64 > uint64(spec.bytes-total) {
			return nil, errors.New("ZIP expansion limit")
		}
		if os.MkdirAll(filepath.Dir(target), 0700) != nil {
			return nil, errors.New("ZIP path conflict")
		}
		r, err := item.Open()
		if err != nil {
			return nil, errors.New("ZIP entry unavailable")
		}
		f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			r.Close()
			return nil, errors.New("ZIP output conflict")
		}
		h := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(contextReader{ctx, r}, min(int64(67108864), spec.bytes-total)+1))
		closeErr := f.Close()
		r.Close()
		if copyErr != nil || closeErr != nil || n > 67108864 || n > spec.bytes-total || uint64(n) != item.UncompressedSize64 {
			return nil, errors.New("ZIP entry incomplete or excessive")
		}
		total += n
		files[name] = fileDigest{hex.EncodeToString(h.Sum(nil)), n}
	}
	if len(files) == 0 {
		return nil, errors.New("empty artifact")
	}
	return files, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}

type contextReaderAt struct {
	ctx context.Context
	r   io.ReaderAt
}

func (r contextReaderAt) ReadAt(b []byte, off int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.ReadAt(b, off)
}

// Hosted artifacts fit classic, single-disk, comment-free ZIP; reject extensions
// before archive/zip allocates entries, and count actual headers independently.
func checkZIPDirectory(r io.ReaderAt, size int64, limit int) error {
	var end [22]byte
	if size < 22 {
		return errors.New("invalid ZIP directory")
	}
	if _, err := r.ReadAt(end[:], size-22); err != nil {
		return errors.New("ZIP directory unavailable")
	}
	u16 := binary.LittleEndian.Uint16
	u32 := binary.LittleEndian.Uint32
	count := int(u16(end[10:12]))
	length := int64(u32(end[12:16]))
	offset := int64(u32(end[16:20]))
	if u32(end[:4]) != 0x06054b50 || u16(end[4:6]) != 0 || u16(end[6:8]) != 0 || int(u16(end[8:10])) != count || count == 0 || count > limit || count == 65535 || u16(end[20:]) != 0 || length > 8<<20 || offset+length != size-22 {
		return errors.New("unsupported or excessive ZIP directory")
	}
	var header [46]byte
	seen := 0
	for at := offset; at < offset+length; {
		if seen >= limit || at+46 > offset+length {
			return errors.New("ZIP directory entry limit")
		}
		if _, err := r.ReadAt(header[:], at); err != nil {
			return errors.New("ZIP directory unavailable")
		}
		name, extra, comment := int64(u16(header[28:30])), int64(u16(header[30:32])), int64(u16(header[32:34]))
		if u32(header[:4]) != 0x02014b50 || name == 0 || name > 256 || extra > 4096 || comment > 1024 || u16(header[34:36]) != 0 || u32(header[20:24]) == 0xffffffff || u32(header[24:28]) == 0xffffffff || u32(header[42:46]) == 0xffffffff || at+46+name+extra+comment > offset+length {
			return errors.New("unsupported ZIP entry header")
		}
		at += 46 + name + extra + comment
		seen++
	}
	if seen != count {
		return errors.New("ZIP directory count mismatch")
	}
	return nil
}

func checkDownloaded(root string, expected kit.RunIdentity) error {
	st, err := os.Lstat(root)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("unsafe download root")
	}
	b, err := readFile(filepath.Join(root, "download.json"), 4194304)
	if err != nil {
		return err
	}
	var receipt downloadReceipt
	if decode(b, &receipt) != nil || receipt.Schema != "tsgk-ci-download/r1" || !sameRun(&receipt.Run, expected) || len(receipt.Artifacts) != 5 {
		return errors.New("download receipt invalid")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 11 {
		return errors.New("unexpected download root entries")
	}
	seen := map[string]bool{}
	for _, a := range receipt.Artifacts {
		var spec *artifactSpec
		for _, s := range artifacts(expected) {
			if s.name == a.Name {
				copy := s
				spec = &copy
			}
		}
		if spec == nil || seen[a.Name] || a.ID == 0 || !hex64.MatchString(a.ZIP.SHA256) || a.ZIP.Bytes <= 0 || a.ZIP.Bytes > zipLimit || len(a.Files) == 0 || len(a.Files) > spec.files {
			return errors.New("download artifact receipt invalid")
		}
		seen[a.Name] = true
		data, err := readFile(filepath.Join(root, a.Name+".zip"), zipLimit)
		if err != nil || int64(len(data)) != a.ZIP.Bytes || digest(data) != a.ZIP.SHA256 {
			return errors.New("downloaded ZIP changed")
		}
		var total int64
		count := 0
		dir := filepath.Join(root, a.Name)
		err = filepath.WalkDir(dir, func(p string, d os.DirEntry, e error) error {
			if e != nil || d.Type()&os.ModeSymlink != 0 || !d.IsDir() && !d.Type().IsRegular() {
				return errors.New("unsafe extracted entry")
			}
			if d.IsDir() {
				return nil
			}
			rel, e := filepath.Rel(dir, p)
			if e != nil {
				return errors.New("invalid extracted path")
			}
			rel = filepath.ToSlash(rel)
			want, ok := a.Files[rel]
			if !ok || !hex64.MatchString(want.SHA256) || want.Bytes < 0 || want.Bytes > 67108864 {
				return errors.New("unexpected extracted file")
			}
			data, e := readFile(p, 67108864)
			if e != nil || int64(len(data)) != want.Bytes || digest(data) != want.SHA256 {
				return errors.New("extracted artifact changed")
			}
			count++
			total += int64(len(data))
			if count > spec.files || total > spec.bytes {
				return errors.New("extracted artifact limit")
			}
			return nil
		})
		if err != nil || count != len(a.Files) {
			return errors.New("extracted artifact verification failed")
		}
	}
	return nil
}
