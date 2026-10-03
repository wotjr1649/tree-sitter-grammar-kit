package kit

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// rz is one hand-built ZIP entry; zero values give a well-formed stored Unix file.
type rz struct {
	name, localName string
	data            []byte
	deflate         bool
	host            uint16 // 3 Unix (default via unixDefault), 0 DOS
	ext             uint32
	flags           uint16
	method          int // -1: from deflate flag
	crc, usize      int64
	extra           []byte
	cut             int   // bytes removed from the end of the compressed stream
	at              int64 // -1: own local header; else the central offset, no local header written
}

func file(name, data string) rz {
	return rz{name: name, data: []byte(data), method: -1, crc: -1, usize: -1, at: -1, host: 3, ext: 0o100644 << 16}
}

func deflated(data []byte) []byte {
	var b bytes.Buffer
	w, _ := flate.NewWriter(&b, flate.BestCompression)
	w.Write(data)
	w.Close()
	return b.Bytes()
}

// buildZip writes local headers, data, central directory and EOCD exactly as described.
func buildZip(entries []rz) []byte {
	var out bytes.Buffer
	type placed struct {
		offset, csize uint32
		method        uint16
		crc, usize    uint32
	}
	var at []placed
	le := func(v ...any) {
		for _, x := range v {
			binary.Write(&out, binary.LittleEndian, x)
		}
	}
	for _, e := range entries {
		method := uint16(0)
		comp := e.data
		if e.deflate {
			method, comp = 8, deflated(e.data)
		}
		if e.method >= 0 {
			method = uint16(e.method)
		}
		comp = comp[:len(comp)-e.cut]
		crc, usize := crc32.ChecksumIEEE(e.data), uint32(len(e.data))
		if e.crc >= 0 {
			crc = uint32(e.crc)
		}
		if e.usize >= 0 {
			usize = uint32(e.usize)
		}
		if e.at >= 0 {
			at = append(at, placed{uint32(e.at), uint32(len(comp)), method, crc, usize})
			continue
		}
		local := e.name
		if e.localName != "" {
			local = e.localName
		}
		at = append(at, placed{uint32(out.Len()), uint32(len(comp)), method, crc, usize})
		le(uint32(0x04034b50), uint16(20), e.flags, method, uint16(0), uint16(0x21), crc, uint32(len(comp)), usize, uint16(len(local)), uint16(len(e.extra)))
		out.WriteString(local)
		out.Write(e.extra)
		out.Write(comp)
	}
	cd := out.Len()
	for i, e := range entries {
		p := at[i]
		le(uint32(0x02014b50), e.host<<8|20, uint16(20), e.flags, p.method, uint16(0), uint16(0x21), p.crc, p.csize, p.usize,
			uint16(len(e.name)), uint16(len(e.extra)), uint16(0), uint16(0), uint16(0), e.ext, p.offset)
		out.WriteString(e.name)
		out.Write(e.extra)
	}
	le(uint32(0x06054b50), uint16(0), uint16(0), uint16(len(entries)), uint16(len(entries)), uint32(out.Len()-cd), uint32(cd), uint16(0))
	return out.Bytes()
}

func writeZip(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "subject.zip")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// expectedFor binds a trusted expected document to the given records.
func expectedFor(t *testing.T, files []FileIdentity) []byte {
	t.Helper()
	m := Manifest{Schema: ManifestSchema, Algorithm: "sha256", ModePolicy: ModePolicy, EncodingPolicy: "detect-r1", Files: files}
	return mustJSON(t, map[string]any{"schema": ExpectedSchema, "provenance": "test fixture prepared independently", "file_count": len(files), "set_sha256": setSHA256(m), "manifest": m})
}

func rec2(path, role, data string) FileIdentity {
	r := rec(path, role, data, "VALIDATION")
	return FileIdentity{Path: path, Role: role, Mode: "100644", ModeProvenance: "POLICY_DEFAULT", Size: uint64(len(data)), SHA256: r[5], Encoding: EncodingOutcome{Assessment: "PASS", Encoding: EncodingUTF8, Source: SourceValidation}}
}

func verifyArchive(t *testing.T, archive string, exp []byte, mutate func(*VerifyRequest)) (VerifyResult, error) {
	t.Helper()
	req := VerifyRequest{Archive: archive, Expected: exp, Limits: DefaultLimits(), ArchiveLimits: DefaultArchiveLimits()}
	if mutate != nil {
		mutate(&req)
	}
	return Verify(testCtx(t), req)
}

var archiveFiles = []FileIdentity{rec2("grammar.js", "grammar", "module.exports = 1;\n"), rec2("src/empty.h", "scanner", ""), rec2("src/scanner.c", "scanner", "int x;\n")}

// S02-A05: normal layouts from different writers, including an empty member, directory
// entries, data descriptors and a top-level directory mapped by ArchiveRoot.
func TestArchiveNormalLayouts(t *testing.T) {
	exp := expectedFor(t, archiveFiles)
	goWriter := func(prefix string, method uint16) []byte {
		var b bytes.Buffer
		w := zip.NewWriter(&b)
		dir := "src/"
		if prefix != "" {
			w.Create(prefix + "/")
			dir = prefix + "/src/"
		}
		w.Create(dir)
		for _, f := range []struct{ n, d string }{{"grammar.js", "module.exports = 1;\n"}, {"src/empty.h", ""}, {"src/scanner.c", "int x;\n"}} {
			name := f.n
			if prefix != "" {
				name = prefix + "/" + f.n
			}
			fw, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: method})
			if err != nil {
				t.Fatal(err)
			}
			fw.Write([]byte(f.d))
		}
		w.Close()
		return b.Bytes()
	}
	raw := []rz{file("grammar.js", "module.exports = 1;\n"), file("src/empty.h", ""), file("src/scanner.c", "int x;\n")}
	raw[0].deflate = true
	for name, tc := range map[string]struct {
		data []byte
		root string
	}{
		"go-deflate-descriptor": {goWriter("", zip.Deflate), ""},
		"go-store-prefixed":     {goWriter("repo-abc", zip.Store), "repo-abc"},
		"raw-no-descriptor":     {buildZip(raw), ""},
	} {
		t.Run(name, func(t *testing.T) {
			p := writeZip(t, tc.data)
			before, _ := os.ReadDir(filepath.Dir(p))
			res, err := verifyArchive(t, p, exp, func(r *VerifyRequest) { r.ArchiveRoot = tc.root })
			if err != nil || res.Assessment != AssessPass || res.ActualSetSHA256 != setSHA256(Manifest{ModePolicy: ModePolicy, EncodingPolicy: "detect-r1", Files: archiveFiles}) {
				t.Fatalf("%v %s %+v", err, res.Assessment, res.Differences)
			}
			if res.Subject != SubjectArchive || res.Scope != ScopeArchiveMembers || len(res.Actual.Files) != 3 || res.Actual.Files[1].Size != 0 {
				t.Fatalf("inventory: %+v", res.Actual.Files)
			}
			after, _ := os.ReadDir(filepath.Dir(p))
			if len(after) != len(before) { // inspection, not extraction
				t.Fatalf("files created next to the archive: %d -> %d", len(before), len(after))
			}
			got, _ := os.ReadFile(p)
			if !bytes.Equal(got, tc.data) {
				t.Fatal("archive changed")
			}
		})
	}
}

// S02-A06/A07: structural, integrity and name attacks end with a typed error and no
// completed inventory.
func TestArchiveRejects(t *testing.T) {
	ok := func(name string) rz { return file(name, "x\n") }
	with := func(e rz, f func(*rz)) rz { f(&e); return e }
	big := bytes.Repeat([]byte("abcdefgh"), 4096)
	// hidden is a complete local header + data for b.txt stored inside a.txt's data, so a
	// second central entry can point into a.txt's region with a self-consistent header.
	hidden := buildZip([]rz{file("b.txt", "y\n")})[:30+5+2]
	cases := []struct {
		name    string
		entries []rz
		raw     []byte
		kind    string
		code    string
	}{
		{"local-central-name", []rz{with(ok("a.txt"), func(e *rz) { e.localName = "b.txt" })}, nil, KindInvalidInput, "ZIP_LOCAL_CENTRAL_MISMATCH"},
		{"duplicate", []rz{ok("a.txt"), ok("a.txt")}, nil, KindInvalidInput, "ARCHIVE_DUPLICATE_MEMBER"},
		{"overlap", []rz{file("a.txt", string(hidden)), with(file("b.txt", "y\n"), func(e *rz) { e.at = 30 + 5 })}, nil, KindInvalidInput, "ZIP_OVERLAP"},
		{"crc", []rz{with(ok("a.txt"), func(e *rz) { e.crc = 1 })}, nil, KindInvalidInput, "ZIP_CRC_MISMATCH"},
		{"truncated-deflate", []rz{with(file("a.txt", string(big)), func(e *rz) { e.deflate = true; e.cut = 3 })}, nil, KindInvalidInput, "ZIP_TRUNCATED"},
		{"size-lie", []rz{with(file("a.txt", string(big)), func(e *rz) { e.deflate = true; e.usize = 10 })}, nil, KindInvalidInput, "ZIP_SIZE_MISMATCH"},
		{"stored-size", []rz{with(ok("a.txt"), func(e *rz) { e.usize = 1 })}, nil, KindInvalidInput, "ZIP_SIZE_MISMATCH"},
		{"traversal", []rz{ok("../x")}, nil, KindInvalidInput, "ARCHIVE_PATH_TRAVERSAL"},
		{"inner-traversal", []rz{ok("a/../../x")}, nil, KindInvalidInput, "ARCHIVE_PATH_TRAVERSAL"},
		{"absolute", []rz{ok("/etc/x")}, nil, KindInvalidInput, "ARCHIVE_PATH_ABSOLUTE"},
		{"drive", []rz{ok("C:/x")}, nil, KindInvalidInput, "ARCHIVE_PATH_DRIVE"},
		{"unc", []rz{ok(`\\server\share\x`)}, nil, KindInvalidInput, "ARCHIVE_PATH_BACKSLASH"},
		{"unc-slash", []rz{ok("//server/share/x")}, nil, KindInvalidInput, "ARCHIVE_PATH_ABSOLUTE"},
		{"ads", []rz{ok("a.txt:stream")}, nil, KindInvalidInput, "ARCHIVE_PATH_ADS"},
		{"device", []rz{ok("src/CON.txt")}, nil, KindInvalidInput, "ARCHIVE_PATH_DEVICE"},
		{"trailing-dot", []rz{ok("a.txt"), ok("a.txt.")}, nil, KindInvalidInput, "ARCHIVE_PATH_TRAILING_DOT_SPACE"},
		{"trailing-space", []rz{ok("dir /a")}, nil, KindInvalidInput, "ARCHIVE_PATH_TRAILING_DOT_SPACE"},
		{"control", []rz{ok("a\x01b")}, nil, KindInvalidInput, "ARCHIVE_PATH_CONTROL"},
		{"case", []rz{ok("A.txt"), ok("a.txt")}, nil, KindInvalidInput, "ARCHIVE_CASE_COLLISION"},
		{"unicode-nfd", []rz{ok("e\u0301.txt")}, nil, KindUnsupported, "ARCHIVE_PATH_NOT_ASCII"},
		{"unicode-nfc", []rz{ok("\u00e9.txt")}, nil, KindUnsupported, "ARCHIVE_PATH_NOT_ASCII"},
		{"file-dir", []rz{ok("a"), ok("a/b")}, nil, KindInvalidInput, "ARCHIVE_FILE_DIRECTORY_CONFLICT"},
		{"file-dir-case", []rz{ok("A"), ok("a/b")}, nil, KindInvalidInput, "ARCHIVE_FILE_DIRECTORY_CONFLICT"},
		{"symlink", []rz{with(ok("link"), func(e *rz) { e.ext = 0o120777 << 16 })}, nil, KindInvalidInput, "ARCHIVE_LINK_REJECTED"},
		{"junction", []rz{with(ok("j"), func(e *rz) { e.host, e.ext = 0, 0x410 })}, nil, KindInvalidInput, "ARCHIVE_LINK_REJECTED"},
		{"fifo", []rz{with(ok("p"), func(e *rz) { e.ext = 0o010644 << 16 })}, nil, KindInvalidInput, "ARCHIVE_SPECIAL_REJECTED"},
		{"dir-data", []rz{with(ok("d/"), func(e *rz) { e.ext = 0o040755 << 16 })}, nil, KindInvalidInput, "ZIP_DIRECTORY_HAS_DATA"},
		{"encrypted", []rz{with(ok("a.txt"), func(e *rz) { e.flags = 1 })}, nil, KindUnsupported, "ZIP_ENCRYPTED_UNSUPPORTED"},
		{"method", []rz{with(ok("a.txt"), func(e *rz) { e.method = 12 })}, nil, KindUnsupported, "ZIP_METHOD_UNSUPPORTED"},
		{"zip64", []rz{with(ok("a.txt"), func(e *rz) { e.extra = []byte{1, 0, 0, 0} })}, nil, KindUnsupported, "ZIP64_UNSUPPORTED"},
		{"not-zip", nil, []byte("\x1f\x8b\x08\x00 this is a gzip/tar stream"), KindInvalidInput, "ZIP_EOCD_MISSING"},
		{"truncated-file", nil, buildZip([]rz{ok("a.txt")})[:40], KindInvalidInput, "ZIP_EOCD_MISSING"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := tc.raw
			if data == nil {
				data = buildZip(tc.entries)
			}
			res, err := verifyArchive(t, writeZip(t, data), expectedFor(t, archiveFiles), nil)
			kindOf(t, err, tc.kind, tc.code)
			if res.ExecutionStatus == StatusCompleted || res.Assessment == AssessPass || len(res.Actual.Files) != 0 || res.ActualSetSHA256 != "" {
				t.Fatalf("partial inventory after rejection: %+v", res)
			}
		})
	}
}

// S02-A08: one-under/equal/one-over for entries, archive bytes, per-file, total decoded,
// path depth and nesting; nested entries share the one entry budget.
func TestArchiveLimits(t *testing.T) {
	members := []rz{file("a/b/one.txt", strings.Repeat("1", 100)), file("two.txt", strings.Repeat("2", 100))}
	data := buildZip(members)
	p := writeZip(t, data)
	exp := expectedFor(t, []FileIdentity{rec2("a/b/one.txt", "corpus", strings.Repeat("1", 100)), rec2("two.txt", "corpus", strings.Repeat("2", 100))})
	for _, tc := range []struct {
		name   string
		mutate func(*VerifyRequest, int64)
		code   string
	}{
		{"entries", func(r *VerifyRequest, d int64) { r.ArchiveLimits.Entries = uint64(2 + d) }, "ARCHIVE_ENTRY_LIMIT"},
		{"archive-bytes", func(r *VerifyRequest, d int64) { r.ArchiveLimits.Bytes = uint64(int64(len(data)) + d) }, "ARCHIVE_BYTES_LIMIT"},
		{"file-bytes", func(r *VerifyRequest, d int64) { r.Limits.FileBytes = uint64(100 + d) }, "FILE_BYTES_LIMIT"},
		{"total-bytes", func(r *VerifyRequest, d int64) { r.Limits.TotalBytes = uint64(200 + d) }, "TOTAL_BYTES_LIMIT"},
		{"depth", func(r *VerifyRequest, d int64) { r.Limits.Depth = uint64(3 + d) }, "DEPTH_LIMIT"},
		{"files", func(r *VerifyRequest, d int64) { r.Limits.Files = uint64(2 + d) }, "FILE_COUNT_LIMIT"},
	} {
		for _, d := range []int64{1, 0, -1} {
			res, err := verifyArchive(t, p, exp, func(r *VerifyRequest) { tc.mutate(r, d) })
			if d >= 0 {
				if err != nil || res.Assessment != AssessPass {
					t.Fatalf("%s %+d: %v", tc.name, d, err)
				}
				continue
			}
			kindOf(t, err, KindResourceLimit, tc.code)
			if res.ExecutionStatus != StatusResourceLimit || len(res.Actual.Files) != 0 {
				t.Fatalf("%s: partial result", tc.name)
			}
		}
	}
	// Nested: outer has 2 entries, inner has 1; the shared budget is 3.
	inner := buildZip([]rz{file("x.txt", "x\n")})
	outer := writeZip(t, buildZip([]rz{file("lib/inner.zip", string(inner)), file("two.txt", strings.Repeat("2", 100))}))
	nexp := expectedFor(t, []FileIdentity{rec2("lib/inner.zip/x.txt", "corpus", "x\n"), rec2("two.txt", "corpus", strings.Repeat("2", 100))})
	for _, tc := range []struct {
		entries, depth uint64
		code           string
	}{{3, 2, ""}, {4, 3, ""}, {2, 2, "ARCHIVE_ENTRY_LIMIT"}, {3, 1, "ARCHIVE_DEPTH_LIMIT"}} {
		res, err := verifyArchive(t, outer, nexp, func(r *VerifyRequest) {
			r.Nested, r.ArchiveLimits.Entries, r.ArchiveLimits.Depth = []string{"lib/inner.zip"}, tc.entries, tc.depth
		})
		if tc.code == "" {
			if err != nil || res.Assessment != AssessPass {
				t.Fatalf("nested %+v: %v %+v", tc, err, res.Differences)
			}
			continue
		}
		kindOf(t, err, KindResourceLimit, tc.code)
	}
	// Without explicit selection the nested archive is one opaque member, not discovered.
	res, err := verifyArchive(t, outer, nexp, nil)
	if err != nil || res.Assessment != AssessFail || res.Differences[0].Code != "UNEXPECTED_FILE" || res.Differences[0].Path != "lib/inner.zip" {
		t.Fatalf("nested archive auto-discovered: %v %+v", err, res.Differences)
	}
	_, err = verifyArchive(t, outer, nexp, func(r *VerifyRequest) { r.Nested = []string{"lib/missing.zip"} })
	kindOf(t, err, KindInvalidInput, "NESTED_MEMBER_NOT_FOUND")
}

// S02-A08: a decompression bomb never allocates its expanded size. The declared size is
// checked before reading and the stream may never exceed it.
func TestArchiveBomb(t *testing.T) {
	zeros := make([]byte, 64<<20)
	comp := deflated(zeros)
	zeros = nil
	mk := func(usize int64) string {
		e := rz{name: "bomb.bin", method: 8, crc: 0, usize: usize, at: -1, host: 3, ext: 0o100644 << 16}
		data := buildZip([]rz{e})
		// buildZip compresses e.data (empty); splice the real bomb stream in its place.
		return writeZip(t, spliceStream(data, comp))
	}
	exp := expectedFor(t, []FileIdentity{rec2("bomb.bin", "corpus", "x")})
	for _, tc := range []struct {
		usize int64
		code  string
	}{{1 << 30, "FILE_BYTES_LIMIT"}, {1 << 20, "ZIP_SIZE_MISMATCH"}} {
		p := mk(tc.usize)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		_, err := verifyArchive(t, p, exp, nil)
		runtime.ReadMemStats(&after)
		kindOf(t, err, map[string]string{"FILE_BYTES_LIMIT": KindResourceLimit, "ZIP_SIZE_MISMATCH": KindInvalidInput}[tc.code], tc.code)
		if grown := after.TotalAlloc - before.TotalAlloc; grown > 16<<20 {
			t.Fatalf("%s: allocated %d bytes for a %d-byte stream", tc.code, grown, len(comp))
		}
	}
}

// spliceStream replaces the single entry's (empty) stored data with stream and fixes the
// compressed sizes and offsets of a one-entry archive built by buildZip.
func spliceStream(z, stream []byte) []byte {
	nameLen := int(binary.LittleEndian.Uint16(z[26:]))
	head := append([]byte(nil), z[:30+nameLen]...)
	binary.LittleEndian.PutUint32(head[18:], uint32(len(stream)))
	cd := append([]byte(nil), z[30+nameLen:]...)
	binary.LittleEndian.PutUint32(cd[20:], uint32(len(stream)))
	eocd := cd[len(cd)-22:]
	binary.LittleEndian.PutUint32(eocd[16:], uint32(len(head)+len(stream)))
	return append(append(head, stream...), cd...)
}

// S02-A09: a manifest or profile inside the archive is only a member. It cannot become
// the trusted expected record or exclude anything.
func TestArchiveSelfTrust(t *testing.T) {
	selfDoc := expectedFor(t, []FileIdentity{rec2("grammar.js", "grammar", "evil\n")})
	selfProfile := `{"schema":"tsgk-profile/r1","id":"self","files":[{"path":"grammar.js","role":"grammar","required":false}]}`
	p := writeZip(t, buildZip([]rz{file("grammar.js", "evil\n"), file("tsgk-expected.json", string(selfDoc)), file("tsgk-profile.json", selfProfile)}))
	res, err := verifyArchive(t, p, expectedFor(t, archiveFiles), nil)
	if err != nil || res.Assessment != AssessFail {
		t.Fatalf("%v %s", err, res.Assessment)
	}
	codes := map[string]string{}
	for _, d := range res.Differences {
		codes[d.Path] += d.Code + " "
	}
	if !strings.Contains(codes["tsgk-expected.json"], "UNEXPECTED_FILE") || !strings.Contains(codes["tsgk-profile.json"], "UNEXPECTED_FILE") ||
		!strings.Contains(codes["grammar.js"], "CONTENT_CHANGED") || !strings.Contains(codes["src/scanner.c"], "MISSING_REQUIRED") {
		t.Fatalf("embedded trust documents changed the comparison: %v", codes)
	}
}
