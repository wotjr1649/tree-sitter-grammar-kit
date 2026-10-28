package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"
)

const readChunk = 1 << 20

// testHookOpen and testHookAfterOpen are package-internal fault-injection seams.
var (
	testHookOpen       func(name string)
	testHookAfterOpen  func(name string)
	testHookBeforeRead func(name string)
)

// openRoot resolves the caller's explicit root once (platform aliases such as macOS
// /var -> /private/var are resolved deliberately) and rejects non-local or non-directory roots.
func openRoot(root string) (*os.Root, string, *Error) {
	if root == "" || strings.ContainsRune(root, 0) {
		return nil, "", fail(KindInvalidInput, "ROOT_INVALID", "", nil)
	}
	if notLocal(root) {
		return nil, "", fail(KindInvalidInput, "ROOT_NOT_LOCAL", "", nil)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, "", fail(KindInvalidInput, "ROOT_INVALID", "", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, "", fail(KindInvalidInput, "ROOT_NOT_FOUND", "", err)
	}
	if notLocal(resolved) { // a link may resolve to a UNC share or device namespace
		return nil, "", fail(KindInvalidInput, "ROOT_NOT_LOCAL", "", nil)
	}
	// The root itself may be a deliberate platform alias (symlink or Windows junction that
	// EvalSymlinks keeps); entries below it are never followed.
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, "", fail(KindIO, "ROOT_UNREADABLE", "", err)
	}
	if !info.IsDir() {
		return nil, "", fail(KindInvalidInput, "ROOT_NOT_DIRECTORY", "", nil)
	}
	r, err := os.OpenRoot(resolved)
	if err != nil {
		return nil, "", fail(KindIO, "ROOT_UNREADABLE", "", err)
	}
	return r, resolved, nil
}

// notLocal reports Windows UNC shares (network access) and \\?\ or \\.\ device namespaces.
func notLocal(p string) bool {
	return runtime.GOOS == "windows" && strings.HasPrefix(strings.ReplaceAll(p, "/", `\`), `\\`)
}

var windowsDevices = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
	"COM¹": true, "COM²": true, "COM³": true, "LPT¹": true, "LPT²": true, "LPT³": true, "CONIN$": true, "CONOUT$": true}

// reservedDevice reports a Windows device name, which also matches with any extension and
// with spaces before the first period ("CON .txt").
func reservedDevice(seg string) bool {
	base, _, _ := strings.Cut(seg, ".")
	return windowsDevices[strings.ToUpper(strings.TrimRight(base, " "))]
}

// portable reports whether value is a portable root-relative path (trust contract).
// The "." sentinel is accepted only where rootSentinel is true.
func portable(value string, rootSentinel bool) bool {
	if value == "." {
		return rootSentinel
	}
	if !utf8.ValidString(value) || !fs.ValidPath(value) || strings.ContainsAny(value, `\:`) {
		return false
	}
	for _, seg := range strings.Split(value, "/") {
		if strings.TrimRight(seg, " .") != seg {
			return false
		}
		if reservedDevice(seg) {
			return false
		}
		for _, r := range seg {
			if r < 32 || r == 127 {
				return false
			}
		}
	}
	return true
}

func depthOf(name string) uint64 {
	if name == "." {
		return 0
	}
	return uint64(strings.Count(name, "/") + 1)
}

// kind classifies a no-follow observation: only plain directories and regular files
// are traversable; symlinks, junctions/reparse points and special files are not.
func kind(info fs.FileInfo) string {
	switch info.Mode().Type() {
	case 0:
		return "file"
	case fs.ModeDir:
		return "dir"
	case fs.ModeSymlink:
		return "link"
	}
	if info.Mode()&fs.ModeIrregular != 0 && info.IsDir() {
		return "link" // Windows junction/mount point or other directory reparse point
	}
	return "special"
}

// guard performs no-follow component checks below an os.Root.
type guard struct {
	root *os.Root
	dirs map[string]bool // verified plain directories
}

func newGuard(r *os.Root) *guard { return &guard{root: r, dirs: map[string]bool{".": true}} }

// lstat checks every parent component is a plain directory, then observes name itself.
// It returns (nil, nil) when name or a parent does not exist.
func (g *guard) lstat(name string) (fs.FileInfo, *Error) {
	if name != "." {
		parts := strings.Split(name, "/")
		for i := 1; i < len(parts); i++ {
			dir := strings.Join(parts[:i], "/")
			if g.dirs[dir] {
				continue
			}
			info, err := g.root.Lstat(dir)
			if errors.Is(err, fs.ErrNotExist) {
				return nil, nil
			}
			if err != nil {
				return nil, fail(KindIO, "UNREADABLE", dir, err)
			}
			if k := kind(info); k != "dir" {
				if k == "file" {
					return nil, nil
				}
				return nil, fail(KindInvalidInput, "LINK_OR_SPECIAL_REJECTED", dir, nil)
			}
			g.dirs[dir] = true
		}
	}
	info, err := g.root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fail(KindIO, "UNREADABLE", name, err)
	}
	if kind(info) == "dir" {
		g.dirs[name] = true
	}
	return info, nil
}

// readDir lists a verified plain directory in sorted order, bounded by budget entries.
func (g *guard) readDir(r *run, name string, budget *uint64, limit uint64) ([]fs.DirEntry, *Error) {
	semantic := r.semanticEntries != nil
	if semantic {
		if r.eligibility == nil {
			return nil, fail(KindInvalidInput, "SEMANTIC_ELIGIBILITY_BUDGET_MISSING", name, nil)
		}
		if e := r.eligibility.charge(1, 0, name); e != nil {
			return nil, e
		}
	}
	f, err := g.root.Open(name)
	if err != nil {
		return nil, fail(KindIO, "UNREADABLE", name, err)
	}
	defer f.Close()
	var openedDirectory fs.FileInfo
	if semantic && name != "." {
		if e := r.eligibility.charge(128, 128*uint64(len(name)+1), name); e != nil {
			return nil, e
		}
		openedDirectory, err = f.Stat()
		if err != nil {
			return nil, fail(KindIO, "UNREADABLE", name, err)
		}
		if _, e := r.semanticEntries.record(g.root, name, openedDirectory); e != nil {
			return nil, e
		}
	}
	var out []fs.DirEntry
	for {
		if e := r.check(); e != nil {
			return nil, e
		}
		if semantic {
			if e := r.eligibility.charge(1, 128*256, name); e != nil {
				return nil, e
			}
		}
		batch, err := f.ReadDir(256)
		for _, entry := range batch {
			var entryPath string
			if semantic {
				entryPath = join(name, entry.Name())
				if e := r.eligibility.charge(128, 128*uint64(len(entryPath)+1), entryPath); e != nil {
					return nil, e
				}
			}
			*budget++
			if *budget > limit {
				return nil, fail(KindResourceLimit, "FILE_COUNT_LIMIT", name, nil)
			}
			if semantic {
				info, infoErr := entry.Info()
				if infoErr != nil {
					return nil, fail(KindIO, "UNREADABLE", entryPath, infoErr)
				}
				if _, e := r.semanticEntries.record(g.root, entryPath, info); e != nil {
					return nil, e
				}
			}
			out = append(out, entry)
		}
		if err == io.EOF || (err == nil && len(batch) == 0) {
			break
		}
		if err != nil {
			return nil, fail(KindIO, "UNREADABLE", name, err)
		}
	}
	if openedDirectory != nil {
		if e := r.eligibility.charge(128, 0, name); e != nil {
			return nil, e
		}
		if _, e := r.semanticEntries.record(g.root, name, openedDirectory); e != nil {
			return nil, e
		}
	}
	if semantic {
		fields := uint64(0)
		for _, entry := range out {
			fields += uint64(len(entry.Name())) + 1
		}
		levels := uint64(1)
		for n := len(out); n > 1; n = (n + 1) / 2 {
			levels++
		}
		if e := r.eligibility.charge(32*levels*(fields+1), 0, "semantic-directory-sort"); e != nil {
			return nil, e
		}
	}
	sortEntries(out)
	return out, nil
}

// readStats is the result of one bounded streaming read.
type readStats struct {
	size     uint64
	sha256   string
	encoding *detector
	content  []byte // kept only when keep > 0 and the file fits
}

// sink receives every chunk read (encoding, newline and marker observers).
type sink interface{ Write([]byte) (int, error) }

// readFile streams one regular file: it re-checks identity after open, enforces the
// per-file and total byte bounds while reading, checkpoints cancellation between
// chunks and detects observable source changes (size/mtime/identity) during reading.
// hardMax is the largest size the caller can accept; the caller judges sizes between
// its ordinary per-file bound and hardMax (registered large-file exceptions).
func (g *guard) readFile(r *run, name string, before fs.FileInfo, hardMax, totalLimit, keep uint64, sinks ...sink) (*readStats, *Error) {
	if e := r.check(); e != nil {
		return nil, e
	}
	if uint64(before.Size()) > hardMax {
		return nil, fail(KindResourceLimit, "FILE_BYTES_LIMIT", name, nil)
	}
	semanticCeiling := r.readCeiling != 0
	if semanticCeiling {
		remaining, e := r.remainingRead(totalLimit, name)
		if e != nil {
			return nil, e
		}
		if uint64(before.Size()) > remaining {
			return nil, fail(KindResourceLimit, "TOTAL_BYTES_LIMIT", name, nil)
		}
	}
	if r.eligibility != nil {
		passes := uint64(3 + len(sinks))
		if keep > 0 {
			passes++
		}
		logical, e := eligibilityProduct(name, uint64(before.Size())+1, passes)
		if e != nil {
			return nil, e
		}
		if e := r.eligibility.charge(1, logical, name); e != nil {
			return nil, e
		}
	}
	if testHookOpen != nil {
		testHookOpen(name)
	}
	f, err := g.root.Open(name)
	if err != nil {
		return nil, fail(KindIO, "READ_FAILED", name, err)
	}
	defer f.Close()
	if testHookAfterOpen != nil {
		testHookAfterOpen(name)
	}
	opened, err := f.Stat()
	if err != nil {
		return nil, fail(KindIO, "READ_FAILED", name, err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) || opened.Size() != before.Size() || !opened.ModTime().Equal(before.ModTime()) {
		return nil, fail(KindIO, "SOURCE_CHANGED", name, nil)
	}
	if e := r.rejectFileIdentity(opened, name); e != nil {
		return nil, e
	}
	confirmedSize := uint64(opened.Size())
	if semanticCeiling {
		remaining, e := r.remainingRead(totalLimit, name)
		if e != nil {
			return nil, e
		}
		if confirmedSize > remaining {
			return nil, fail(KindResourceLimit, "TOTAL_BYTES_LIMIT", name, nil)
		}
	}
	if e := checkLinks(f, name); e != nil {
		return nil, e
	}
	h := sha256.New()
	det := newDetector()
	st := &readStats{encoding: det}
	buf := make([]byte, readChunk)
	for {
		if e := r.check(); e != nil {
			return nil, e
		}
		readBuf := buf
		if semanticCeiling {
			if st.size == confirmedSize {
				break
			}
			remaining, e := r.remainingRead(totalLimit, name)
			if e != nil {
				return nil, e
			}
			fileRemaining := confirmedSize - st.size
			if remaining == 0 {
				return nil, fail(KindResourceLimit, "TOTAL_BYTES_LIMIT", name, nil)
			}
			request := min(uint64(len(readBuf)), remaining, fileRemaining)
			readBuf = readBuf[:int(request)]
		}
		if testHookBeforeRead != nil {
			testHookBeforeRead(name)
		}
		n, err := f.Read(readBuf)
		if n > 0 {
			st.size += uint64(n)
			chargeErr := r.chargeRead(uint64(n), totalLimit, name)
			if st.size > hardMax {
				return nil, fail(KindResourceLimit, "FILE_BYTES_LIMIT", name, nil)
			}
			if chargeErr != nil {
				return nil, chargeErr
			}
			chunk := buf[:n]
			h.Write(chunk)
			det.Write(chunk)
			for _, s := range sinks {
				s.Write(chunk)
			}
			if keep > 0 && st.size <= keep {
				st.content = append(st.content, chunk...)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fail(KindIO, "READ_FAILED", name, err)
		}
		if semanticCeiling && n == 0 {
			return nil, fail(KindIO, "READ_FAILED", name, io.ErrNoProgress)
		}
		if semanticCeiling && st.size == confirmedSize {
			break
		}
	}
	after, err := f.Stat()
	if err != nil {
		return nil, fail(KindIO, "READ_FAILED", name, err)
	}
	if !os.SameFile(opened, after) || st.size != confirmedSize || uint64(after.Size()) != confirmedSize || !after.ModTime().Equal(opened.ModTime()) {
		return nil, fail(KindIO, "SOURCE_CHANGED", name, nil)
	}
	if keep > 0 && st.size > keep {
		st.content = nil
	}
	st.sha256 = hex.EncodeToString(h.Sum(nil))
	return st, nil
}

// sortEntries uses byte order so results do not depend on OS directory order.
func sortEntries(entries []fs.DirEntry) {
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
}

func join(dir, name string) string {
	if dir == "." {
		return name
	}
	return path.Join(dir, name)
}
