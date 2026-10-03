package kit

import (
	"bufio"
	"cmp"
	"compress/flate"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"io"
	"slices"
	"strings"
)

// ArchiveProfile is the only archive format this build inspects.
const ArchiveProfile = "zip-r1"

// ArchiveLimits bound archive inspection: Entries counts every central-directory entry of
// the outer and explicitly selected nested archives together, Bytes bounds the outer
// archive file, Depth bounds nesting (1 = the outer archive only). Decoded member bytes
// count against Limits.FileBytes per member and Limits.TotalBytes cumulatively.
type ArchiveLimits struct {
	Entries, Bytes, Depth uint64
}

// DefaultArchiveLimits returns the adopted verify archive bounds used by the CLI.
func DefaultArchiveLimits() ArchiveLimits {
	return ArchiveLimits{Entries: 10000, Bytes: 268435456, Depth: 2}
}

func (l ArchiveLimits) valid() bool { return l.Entries > 0 && l.Bytes > 0 && l.Depth > 0 }

// zipEntry is one validated central-directory entry; name keeps the original bytes.
type zipEntry struct {
	name, path   string
	dir          bool
	flags        uint16
	method       uint16
	crc          uint32
	csize, usize uint64
	offset       uint64 // local header
	data         uint64 // first compressed byte
}

type zipArchive struct {
	ra      io.ReaderAt
	entries []*zipEntry
}

func le16(b []byte) uint64 { return uint64(binary.LittleEndian.Uint16(b)) }
func le32(b []byte) uint64 { return uint64(binary.LittleEndian.Uint32(b)) }

func zipFail(code, p string) *Error { return fail(KindInvalidInput, code, p, nil) }
func zipUnsupported(code, p string) *Error {
	return fail(KindUnsupported, code, p, nil)
}

// openZip validates the structure of one archive of the zip-r1 profile without reading
// member data: EOCD, central directory, local headers, data regions, names and types.
// seen accumulates central entries across nested archives; prefix names the archive in
// diagnostics. Unsupported features fail instead of being skipped.
func openZip(r *run, ra io.ReaderAt, size uint64, lim ArchiveLimits, seen *uint64, depthLimit uint64, prefix string) (*zipArchive, *Error) {
	tail := min(size, 22+65535)
	if tail < 22 {
		return nil, zipFail("ZIP_EOCD_MISSING", prefix)
	}
	buf := make([]byte, tail)
	if _, err := ra.ReadAt(buf, int64(size-tail)); err != nil {
		return nil, fail(KindIO, "READ_FAILED", prefix, err)
	}
	eocd := -1
	for i := len(buf) - 22; i >= 0; i-- {
		if le32(buf[i:]) == 0x06054b50 && uint64(i)+22+le16(buf[i+20:]) == tail {
			eocd = i
			break
		}
	}
	if eocd < 0 {
		return nil, zipFail("ZIP_EOCD_MISSING", prefix)
	}
	b := buf[eocd:]
	eocdAt := size - tail + uint64(eocd)
	disk, cdDisk, onDisk, total, cdSize, cdAt := le16(b[4:]), le16(b[6:]), le16(b[8:]), le16(b[10:]), le32(b[12:]), le32(b[16:])
	if total == 0xFFFF || cdSize == 0xFFFFFFFF || cdAt == 0xFFFFFFFF || (eocd >= 20 && le32(buf[eocd-20:]) == 0x07064b50) {
		return nil, zipUnsupported("ZIP64_UNSUPPORTED", prefix)
	}
	if disk != 0 || cdDisk != 0 || onDisk != total {
		return nil, zipUnsupported("ZIP_MULTIDISK_UNSUPPORTED", prefix)
	}
	if cdAt+cdSize != eocdAt || total*46 > cdSize {
		return nil, zipFail("ZIP_STRUCTURE_INVALID", prefix)
	}
	*seen += total // checked before any entry is allocated
	if *seen > lim.Entries {
		return nil, fail(KindResourceLimit, "ARCHIVE_ENTRY_LIMIT", prefix, nil)
	}
	z := &zipArchive{ra: ra, entries: make([]*zipEntry, 0, total)}
	cd := bufio.NewReader(io.NewSectionReader(ra, int64(cdAt), int64(cdSize)))
	var h [46]byte
	for range total {
		if e := r.check(); e != nil {
			return nil, e
		}
		if _, err := io.ReadFull(cd, h[:]); err != nil || le32(h[:]) != 0x02014b50 {
			return nil, zipFail("ZIP_STRUCTURE_INVALID", prefix)
		}
		raw, extra := make([]byte, le16(h[28:])), make([]byte, le16(h[30:]))
		if _, err := io.ReadFull(cd, raw); err != nil {
			return nil, zipFail("ZIP_STRUCTURE_INVALID", prefix)
		}
		if _, err := io.ReadFull(cd, extra); err != nil {
			return nil, zipFail("ZIP_STRUCTURE_INVALID", prefix)
		}
		if _, err := cd.Discard(int(le16(h[32:]))); err != nil {
			return nil, zipFail("ZIP_STRUCTURE_INVALID", prefix)
		}
		e := &zipEntry{name: string(raw), flags: uint16(le16(h[8:])), method: uint16(le16(h[10:])), crc: uint32(le32(h[16:])),
			csize: le32(h[20:]), usize: le32(h[24:]), offset: le32(h[42:])}
		where := diagName(prefix, e.name)
		switch {
		case hasZip64(extra):
			return nil, zipUnsupported("ZIP64_UNSUPPORTED", where)
		case le16(h[34:]) != 0:
			return nil, zipUnsupported("ZIP_MULTIDISK_UNSUPPORTED", where)
		case e.flags&0x2041 != 0: // encrypted, strong encryption, masked header
			return nil, zipUnsupported("ZIP_ENCRYPTED_UNSUPPORTED", where)
		case e.method != 0 && e.method != 8:
			return nil, zipUnsupported("ZIP_METHOD_UNSUPPORTED", where)
		case e.method == 0 && e.csize != e.usize:
			return nil, zipFail("ZIP_SIZE_MISMATCH", where)
		}
		host, ext := le16(h[4:])>>8, le32(h[38:])
		e.dir = strings.HasSuffix(e.name, "/")
		if host == 3 { // Unix: the mode type travels in the high external attribute bits
			switch t := ext >> 16 & 0xF000; {
			case t == 0xA000:
				return nil, zipFail("ARCHIVE_LINK_REJECTED", where)
			case t == 0x4000 && !e.dir || t == 0x8000 && e.dir:
				return nil, zipFail("ZIP_ENTRY_TYPE_INCONSISTENT", where)
			case t != 0 && t != 0x4000 && t != 0x8000:
				return nil, zipFail("ARCHIVE_SPECIAL_REJECTED", where)
			}
		}
		if (host == 0 || host == 10 || host == 11) && ext&0x400 != 0 { // DOS/NTFS reparse point
			return nil, zipFail("ARCHIVE_LINK_REJECTED", where)
		}
		if e.dir && e.usize != 0 {
			return nil, zipFail("ZIP_DIRECTORY_HAS_DATA", where)
		}
		e.path = strings.TrimSuffix(e.name, "/")
		if code := pathProblem(e.path); code != "" {
			if code == "NOT_ASCII" {
				return nil, zipUnsupported("ARCHIVE_PATH_NOT_ASCII", where)
			}
			return nil, zipFail("ARCHIVE_PATH_"+code, where)
		}
		if depthOf(e.path) > depthLimit {
			return nil, fail(KindResourceLimit, "DEPTH_LIMIT", where, nil)
		}
		z.entries = append(z.entries, e)
	}
	if _, err := cd.ReadByte(); err != io.EOF {
		return nil, zipFail("ZIP_STRUCTURE_INVALID", prefix)
	}
	if e := z.checkNames(prefix); e != nil {
		return nil, e
	}
	return z, z.checkRegions(r, cdAt, prefix)
}

func diagName(prefix, name string) string {
	if prefix != "" {
		return prefix + "/" + name
	}
	return name
}

func hasZip64(extra []byte) bool {
	for len(extra) >= 4 {
		id, n := le16(extra), le16(extra[2:])
		if id == 0x0001 {
			return true
		}
		if uint64(len(extra)) < 4+n {
			return false
		}
		extra = extra[4+n:]
	}
	return false
}

// checkNames rejects exact duplicates, ASCII case-fold collisions and a member that is a
// file in one entry and a directory (ancestor) in another, case-insensitively.
func (z *zipArchive) checkNames(prefix string) *Error {
	exact := map[string]bool{}
	folded := map[string]*zipEntry{}
	for _, e := range z.entries {
		key := strings.ToLower(e.path)
		if prev := folded[key]; prev != nil {
			switch {
			case exact[e.path] && prev.dir == e.dir:
				return zipFail("ARCHIVE_DUPLICATE_MEMBER", diagName(prefix, e.name))
			case prev.dir != e.dir:
				return zipFail("ARCHIVE_FILE_DIRECTORY_CONFLICT", diagName(prefix, e.name))
			}
			return zipFail("ARCHIVE_CASE_COLLISION", diagName(prefix, e.name))
		}
		exact[e.path], folded[key] = true, e
	}
	for _, e := range z.entries {
		key := strings.ToLower(e.path)
		for i := strings.IndexByte(key, '/'); i >= 0; i = nextSlash(key, i) {
			if parent := folded[key[:i]]; parent != nil && !parent.dir {
				return zipFail("ARCHIVE_FILE_DIRECTORY_CONFLICT", diagName(prefix, e.name))
			}
		}
	}
	return nil
}

func nextSlash(s string, i int) int {
	j := strings.IndexByte(s[i+1:], '/')
	if j < 0 {
		return -1
	}
	return i + 1 + j
}

// checkRegions compares each local header with its central entry and rejects data
// regions that overlap each other or the central directory.
func (z *zipArchive) checkRegions(r *run, cdAt uint64, prefix string) *Error {
	order := slices.Clone(z.entries)
	slices.SortFunc(order, func(a, b *zipEntry) int { return cmp.Compare(a.offset, b.offset) })
	end := uint64(0)
	var h [30]byte
	for _, e := range order {
		if e := r.check(); e != nil {
			return e
		}
		where := diagName(prefix, e.name)
		if e.offset < end {
			return zipFail("ZIP_OVERLAP", where)
		}
		if e.offset+30 > cdAt {
			return zipFail("ZIP_LOCAL_HEADER_INVALID", where)
		}
		if _, err := z.ra.ReadAt(h[:], int64(e.offset)); err != nil || le32(h[:]) != 0x04034b50 {
			return zipFail("ZIP_LOCAL_HEADER_INVALID", where)
		}
		nameLen, extraLen := le16(h[26:]), le16(h[28:])
		e.data = e.offset + 30 + nameLen + extraLen
		if e.data > cdAt {
			return zipFail("ZIP_LOCAL_HEADER_INVALID", where)
		}
		local := make([]byte, nameLen+extraLen)
		if _, err := z.ra.ReadAt(local, int64(e.offset+30)); err != nil {
			return zipFail("ZIP_LOCAL_HEADER_INVALID", where)
		}
		crc, cs, us := le32(h[14:]), le32(h[18:]), le32(h[22:])
		descriptor := e.flags&0x8 != 0
		same := crc == uint64(e.crc) && cs == e.csize && us == e.usize
		zero := crc == 0 && cs == 0 && us == 0
		switch {
		case uint16(le16(h[6:])) != e.flags || uint16(le16(h[8:])) != e.method || string(local[:nameLen]) != e.name:
			return zipFail("ZIP_LOCAL_CENTRAL_MISMATCH", where)
		case hasZip64(local[nameLen:]):
			return zipUnsupported("ZIP64_UNSUPPORTED", where)
		case !same && !(descriptor && zero):
			return zipFail("ZIP_LOCAL_CENTRAL_MISMATCH", where)
		}
		end = e.data + e.csize
		if end > cdAt {
			return zipFail("ZIP_OVERLAP", where)
		}
		if descriptor {
			var d [16]byte
			n := min(uint64(16), cdAt-end)
			if _, err := z.ra.ReadAt(d[:n], int64(end)); err != nil && !errors.Is(err, io.EOF) {
				return zipFail("ZIP_DESCRIPTOR_MISMATCH", where)
			}
			fields := d[:n]
			if n >= 4 && le32(fields) == 0x08074b50 {
				fields = fields[4:]
				end += 4
			}
			if len(fields) < 12 || le32(fields) != uint64(e.crc) || le32(fields[4:]) != e.csize || le32(fields[8:]) != e.usize {
				return zipFail("ZIP_DESCRIPTOR_MISMATCH", where)
			}
			end += 12
		}
	}
	return nil
}

// readMember streams one member's decoded bytes under the same bounds as readFile: the
// declared size is checked before reading and the decoded count, per-file hard maximum
// and cumulative total while streaming. CRC, declared size and compressed length must
// match exactly; keep > 0 retains content (nested archives) up to keep bytes.
func (z *zipArchive) readMember(r *run, e *zipEntry, where string, hardMax, totalLimit, keep uint64) (*readStats, *Error) {
	if e.usize > hardMax {
		return nil, fail(KindResourceLimit, "FILE_BYTES_LIMIT", where, nil)
	}
	section := bufio.NewReader(io.NewSectionReader(z.ra, int64(e.data), int64(e.csize)))
	var src io.Reader = section
	if e.method == 8 {
		fr := flate.NewReader(section)
		defer fr.Close()
		src = fr
	}
	h, c := sha256.New(), crc32.NewIEEE()
	det := newDetector()
	st := &readStats{encoding: det}
	buf := make([]byte, readChunk)
	for {
		if e := r.check(); e != nil {
			return nil, e
		}
		n, err := src.Read(buf)
		if n > 0 {
			st.size += uint64(n)
			if st.size > e.usize { // the stream claims less than it inflates to; usize <= hardMax
				return nil, zipFail("ZIP_SIZE_MISMATCH", where)
			}
			r.totalRead += uint64(n)
			if r.totalRead > totalLimit {
				return nil, fail(KindResourceLimit, "TOTAL_BYTES_LIMIT", where, nil)
			}
			chunk := buf[:n]
			h.Write(chunk)
			c.Write(chunk)
			det.Write(chunk)
			if keep > 0 {
				st.content = append(st.content, chunk...)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, zipFail("ZIP_TRUNCATED", where)
			}
			return nil, zipFail("ZIP_DATA_CORRUPT", where)
		}
	}
	if st.size != e.usize {
		return nil, zipFail("ZIP_TRUNCATED", where)
	}
	if _, err := section.ReadByte(); err != io.EOF { // compressed bytes left over
		return nil, zipFail("ZIP_COMPRESSED_SIZE_MISMATCH", where)
	}
	if c.Sum32() != e.crc {
		return nil, zipFail("ZIP_CRC_MISMATCH", where)
	}
	st.sha256 = hex.EncodeToString(h.Sum(nil))
	return st, nil
}
