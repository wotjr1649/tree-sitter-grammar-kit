package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestSemanticParserPagedOutputGolden(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "grammar"), 0700); err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("p"), kit.MaxDocumentBytes+1)
	if err := os.WriteFile(filepath.Join(dir, "grammar", "parser.c"), data, 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	files, err := semanticParserFiles(context.Background(), root, ClosureFile{Role: "grammar-parser", Path: "parser.c", Bytes: int64(len(data)), SHA256: sha(data)}, 7)
	if err != nil || len(files) != 3 {
		t.Fatalf("files=%d: %v", len(files), err)
	}
	// Captured before helper extraction at db1413b under cx-b2c-c2-golden-probe1.
	want := []kit.ReplayMember{
		{Path: "capture/build-input-00007.part-00000.bin", Role: "grammar-paged-part", Bytes: 16777216, SHA256: "40fea11d72cd8d9146e0d0c2a80f40befaf10e6096623ca58eb8a2996a0f9bc8"},
		{Path: "capture/build-input-00007.part-00001.bin", Role: "grammar-paged-part", Bytes: 1, SHA256: "148de9c5a7a44d19e56cd9ae1a554bf67847afb0c58f6e12fa29ac7ddfca9940"},
		{Path: "capture/build-input-00007.pages.json", Role: "grammar-paged-descriptor", Bytes: 558, SHA256: "93287b1e2e2f439215a672ffe147d2d5a16d814b7777ebb0e5de79c1c5df9c5a"},
	}
	for i, f := range files {
		if f.pin != want[i] || uint64(len(f.data)) != want[i].Bytes || sha(f.data) != want[i].SHA256 {
			t.Fatalf("file %d differs from golden: %+v", i, f.pin)
		}
	}
}

func semanticBinaryTestPlan(parts int) *semanticCapturePlan {
	p := &semanticCapturePlan{Context: semanticPagedBinariesContext, Members: []semanticCaptureMember{
		{Owner: "O", Path: "manifest.json", Role: "manifest"},
		{Owner: "D", Path: "capture/driver.pages.json", Role: "driver-paged-descriptor"},
	}}
	for i := range parts {
		p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: fmt.Sprintf("capture/driver.part-%05d.bin", i), Role: "driver-paged-part"})
	}
	return p
}

func semanticBinaryTestRegistration(t *testing.T, p *semanticCapturePlan) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{"schema": "tsgk-query-semantics-registration/r1", "context": p.Context, "planned_members": p.Members})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func semanticBinaryTestError(t *testing.T, err error, code string) {
	t.Helper()
	if code == "" {
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	var ne *Error
	wantKind := kit.KindInvalidInput
	if code == "SEMANTICS_MEMBER_LIMIT" {
		wantKind = kit.KindResourceLimit
	}
	if !errors.As(err, &ne) || ne.Code != code || ne.Kind != wantKind {
		t.Fatalf("want %s/%s, got %v", wantKind, code, err)
	}
}

func TestSemanticCapturePlanDriverShapeBounds(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"parts-2", "parts-3", "parts-4", "plain", "parts-1", "parts-5", "gap", "no-descriptor", "two-descriptors", "descriptor-path", "mixed", "descriptor-only", "missing-driver", "two-drivers", "plain-and-part", "part-path", "case-collision", "duplicate-part", "unknown-paged-role"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := semanticBinaryTestPlan(2)
			code := "SEMANTICS_MEMBER_PLAN"
			plain := semanticCaptureMember{Owner: "D", Path: "capture/driver.bin", Role: "driver"}
			switch name {
			case "parts-2", "parts-3", "parts-4":
				p = semanticBinaryTestPlan(int(name[len(name)-1] - '0'))
				code = ""
			case "plain":
				p.Members = append(p.Members[:1], plain)
				code = ""
			case "parts-1":
				p = semanticBinaryTestPlan(1)
			case "parts-5":
				p = semanticBinaryTestPlan(5)
			case "gap":
				p.Members[3].Path = "capture/driver.part-00002.bin"
			case "no-descriptor":
				p.Members = append(p.Members[:1], p.Members[2:]...)
			case "two-descriptors":
				p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: "capture/other.pages.json", Role: "driver-paged-descriptor"})
			case "descriptor-path":
				p.Members[1].Path = "capture/drivers.pages.json"
			case "mixed":
				p.Members = append(p.Members, plain)
			case "descriptor-only":
				p.Members = p.Members[:2]
			case "missing-driver":
				p.Members = p.Members[:1]
			case "two-drivers":
				p.Members = append(p.Members[:1], plain, semanticCaptureMember{Owner: "D", Path: "capture/other.bin", Role: "driver"})
			case "plain-and-part":
				p.Members = append(p.Members[:1], plain, p.Members[2])
			case "part-path":
				p.Members[2].Path = "capture/driver.part-0000.bin"
			case "case-collision":
				p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: "capture/DRIVER.part-00000.bin", Role: "driver-paged-part"})
			case "duplicate-part":
				p.Members = append(p.Members, p.Members[2])
			case "unknown-paged-role":
				p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: "capture/future.bin", Role: "driver-paged-future"})
			}
			registered, err := semanticCaptureRegistration(context.Background(), semanticBinaryTestRegistration(t, p))
			if name == "case-collision" || name == "duplicate-part" {
				semanticBinaryTestError(t, err, code)
				return
			}
			semanticBinaryTestError(t, err, "")
			semanticBinaryTestError(t, registered.driverShape(), code)
		})
	}
}

func TestSemanticCapturePlanContextSelection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		context string
		paged   bool
	}{
		{"binaries", semanticPagedBinariesContext, true},
		{"parser", semanticPagedContext, true},
		{"revision", "full-input-utf8-paged-binaries/r2", false},
		{"trailing-space", semanticPagedBinariesContext + " ", false},
		{"case", strings.ToUpper(semanticPagedBinariesContext), false},
		{"opaque", "opaque", false},
	} {
		for _, stage := range []string{"registration", "selector"} {
			t.Run(stage+"-"+tc.name, func(t *testing.T) {
				t.Parallel()
				p := semanticBinaryTestPlan(2)
				p.Context = tc.context
				data := semanticBinaryTestRegistration(t, p)
				if stage == "selector" {
					got, err := semanticPagedSelected(data, func() error { return nil })
					if err != nil || got != tc.paged {
						t.Fatalf("selected=%v want=%v: %v", got, tc.paged, err)
					}
					return
				}
				got, err := semanticCaptureRegistration(context.Background(), data)
				semanticBinaryTestError(t, err, "")
				wantContext := "full-input-utf8/r2"
				if tc.paged {
					wantContext = tc.context
				}
				if got.Context != wantContext || got.paged() != tc.paged || got.binaries() != (tc.context == semanticPagedBinariesContext) {
					t.Fatalf("wrong plan: %+v", got)
				}
			})
		}
	}
}

func TestSemanticPagedBinariesContextMatchesKit(t *testing.T) {
	t.Parallel()
	if semanticPagedBinariesContext != "full-input-utf8-paged-binaries/r1" || kit.OraclePagedBinariesSemanticsManifestSchema != "tsgk-oracle-manifest/r7" {
		t.Fatal("native context or kit manifest schema differs from the binary paging contract")
	}
}

func TestSemanticCapturePlanDriverRolesOtherContexts(t *testing.T) {
	t.Parallel()
	for _, context := range []string{semanticPagedContext, "full-input-utf8/r2"} {
		for _, role := range []string{"driver-paged-descriptor", "driver-paged-part", "driver-paged-future", "driver"} {
			t.Run(context+"/"+role, func(t *testing.T) {
				t.Parallel()
				path := "capture/driver.bin"
				if role == "driver-paged-descriptor" {
					path = "capture/driver.pages.json"
				} else if role == "driver-paged-part" {
					path = "capture/driver.part-00000.bin"
				}
				p := &semanticCapturePlan{Context: context, Members: []semanticCaptureMember{{Owner: "D", Path: path, Role: role}}}
				code := "SEMANTICS_MEMBER_PLAN"
				if role == "driver" {
					code = ""
				}
				semanticBinaryTestError(t, p.driverShape(), code)
			})
		}
	}
}

func TestSemanticCapturePlanBuildOnlyDriverRoles(t *testing.T) {
	t.Parallel()
	for _, role := range []string{"driver", "driver-paged-descriptor", "driver-paged-part", "driver-future", "driver-paged-future"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			p := &semanticCapturePlan{Members: []semanticCaptureMember{{Owner: "D", Path: "capture/driver", Role: role}}}
			included := role == "driver" || role == "driver-paged-descriptor" || role == "driver-paged-part"
			files := []captureFile{{pin: kit.ReplayMember{Path: "capture/driver", Role: role}}}
			if p.matches(files, true) != included || p.matches(nil, true) == included || !p.matches(files, false) {
				t.Fatal("buildOnly must include exactly the three driver roles")
			}
		})
	}
}

func TestSemanticCapturePlanGrammarShapeUnderBinaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		size    uint64
		parsers int
		pages   bool
		code    string
	}{
		{"small-plain", kit.MaxDocumentBytes, 1, false, ""},
		{"no-parser", 0, 0, false, ""},
		{"small-with-pages", kit.MaxDocumentBytes, 1, true, "SEMANTICS_MEMBER_PLAN"},
		{"no-parser-with-pages", 0, 0, true, "SEMANTICS_MEMBER_PLAN"},
		{"one-large", kit.MaxDocumentBytes + 1, 1, true, ""},
		{"build-file-boundary", kit.BuildFileBytes, 1, true, ""},
		{"above-build-file", kit.BuildFileBytes + 1, 1, true, "SEMANTICS_MEMBER_LIMIT"},
		{"two-large", kit.MaxDocumentBytes + 1, 2, true, "SEMANTICS_MEMBER_PLAN"},
		{"large-without-pages", kit.MaxDocumentBytes + 1, 1, false, "SEMANTICS_MEMBER_PLAN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := semanticBinaryTestPlan(2)
			var grammar []kit.NativeInput
			for range tc.parsers {
				grammar = append(grammar, kit.NativeInput{Role: "parser", Bytes: tc.size})
			}
			if tc.pages {
				p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: "capture/build-input-00007.pages.json", Role: "grammar-paged-descriptor"})
				count := (tc.size + kit.MaxDocumentBytes - 1) / kit.MaxDocumentBytes
				for i := uint64(0); i < count; i++ {
					p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: fmt.Sprintf("capture/build-input-00007.part-%05d.bin", i), Role: "grammar-paged-part"})
				}
			}
			semanticBinaryTestError(t, p.grammarShape(grammar), tc.code)
		})
	}
	for _, role := range []string{"grammar-paged-descriptor", "grammar-paged-part", "grammar-paged-future"} {
		t.Run(role+"-without-parser", func(t *testing.T) {
			t.Parallel()
			p := semanticBinaryTestPlan(2)
			p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: "capture/unused", Role: role})
			semanticBinaryTestError(t, p.grammarShape(nil), "SEMANTICS_MEMBER_PLAN")
		})
	}
}

func semanticBinaryTestBuild(t *testing.T, size int64) *Build {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "driver.exe")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(f.Truncate(size), f.Close()); err != nil {
		t.Fatal(err)
	}
	digest, _, err := fileDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	return &Build{Dir: dir, Executable: path, ExecutableBytes: size, ExecutableSHA256: digest, identityText: "synthetic build", Identity: sha([]byte("synthetic build"))}
}

func semanticBinaryTestRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	return root
}

func TestSemanticDriverFilesBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		size  int64
		parts int
	}{
		{"16MiB", 16 << 20, 0},
		{"16MiB+1", 16<<20 + 1, 2},
		{"32MiB", 32 << 20, 2},
		{"32MiB+1", 32<<20 + 1, 3},
		{"64MiB", 64 << 20, 4},
		{"64MiB+1", 64<<20 + 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := &Build{Dir: t.TempDir(), Executable: "missing.exe", ExecutableBytes: tc.size}
			if tc.parts != 0 {
				b = semanticBinaryTestBuild(t, tc.size)
			}
			files, err := semanticDriverFiles(context.Background(), semanticBinaryTestRoot(t, b.Dir), b)
			if tc.parts == 0 {
				semanticBinaryTestError(t, err, "SEMANTICS_MEMBER_LIMIT")
				if files != nil {
					t.Fatal("limit returned partial files")
				}
				return
			}
			if err != nil || len(files) != tc.parts+1 {
				t.Fatalf("files=%d, want %d: %v", len(files), tc.parts+1, err)
			}
			var d semanticBinaryPages
			if err := json.Unmarshal(files[tc.parts].data, &d); err != nil {
				t.Fatal(err)
			}
			if len(d.Parts) != tc.parts {
				t.Fatalf("descriptor parts=%d", len(d.Parts))
			}
			for i, part := range d.Parts {
				offset := int64(i) * kit.MaxDocumentBytes
				wantSize := min(int64(kit.MaxDocumentBytes), tc.size-offset)
				if part.Index != i || part.Offset != offset || part.Bytes != wantSize || files[i].pin.Bytes != uint64(wantSize) || int64(len(files[i].data)) != wantSize {
					t.Fatalf("part %d: %+v", i, part)
				}
			}
		})
	}
}

func TestSemanticDriverFilesContent(t *testing.T) {
	t.Parallel()
	b := semanticBinaryTestBuild(t, kit.MaxDocumentBytes+1)
	data := bytes.Repeat([]byte("driver bytes\x00\xff"), kit.MaxDocumentBytes/14+1)[:kit.MaxDocumentBytes+1]
	if err := os.WriteFile(b.Executable, data, 0600); err != nil {
		t.Fatal(err)
	}
	b.ExecutableSHA256 = sha(data)
	files, err := semanticDriverFiles(context.Background(), semanticBinaryTestRoot(t, b.Dir), b)
	if err != nil || len(files) != 3 {
		t.Fatalf("files=%d: %v", len(files), err)
	}
	descriptor := files[2]
	var d semanticBinaryPages
	if err := json.Unmarshal(descriptor.data, &d); err != nil {
		t.Fatal(err)
	}
	if d.Schema != "tsgk-query-binary-pages/r1" || d.Owner != "D" || d.Logical.Kind != "driver" || d.Logical.Path != "capture/driver.bin" || d.Logical.Bytes != b.ExecutableBytes || d.Logical.SHA256 != b.ExecutableSHA256 || len(d.Parts) != 2 {
		t.Fatalf("wrong descriptor: %+v", d)
	}
	if descriptor.pin.Path != "capture/driver.pages.json" || descriptor.pin.Role != "driver-paged-descriptor" || descriptor.pin.Bytes != uint64(len(descriptor.data)) || descriptor.pin.SHA256 != sha(descriptor.data) || descriptor.pin.SHA256 == d.Logical.SHA256 {
		t.Fatal("descriptor physical identity confused with logical identity")
	}
	var joined []byte
	for i, part := range d.Parts {
		f := files[i]
		if part.Index != i || part.Offset != int64(len(joined)) || part.Path != fmt.Sprintf("capture/driver.part-%05d.bin", i) || part.Bytes != int64(len(f.data)) || part.SHA256 != sha(f.data) || f.pin.Path != part.Path || f.pin.Role != "driver-paged-part" || f.pin.Bytes != uint64(part.Bytes) || f.pin.SHA256 != part.SHA256 {
			t.Fatalf("wrong part %d: %+v", i, part)
		}
		joined = append(joined, f.data...)
	}
	if !bytes.Equal(joined, data) {
		t.Fatal("reassembled parts differ from input")
	}
}

func TestSemanticDriverFilesIntegrity(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"canceled", "wrong-sha", "size-minus-one", "size-plus-one", "directory"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b := semanticBinaryTestBuild(t, kit.MaxDocumentBytes+2)
			ctx := context.Background()
			switch name {
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "wrong-sha":
				b.ExecutableSHA256 = strings.Repeat("0", 64)
			case "size-minus-one":
				b.ExecutableBytes--
			case "size-plus-one":
				b.ExecutableBytes++
			case "directory":
				b.Executable = filepath.Join(b.Dir, "directory")
				if err := os.Mkdir(b.Executable, 0700); err != nil {
					t.Fatal(err)
				}
			}
			files, err := semanticDriverFiles(ctx, semanticBinaryTestRoot(t, b.Dir), b)
			if err == nil || files != nil {
				t.Fatalf("invalid capture returned files=%d: %v", len(files), err)
			}
			if name == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("wrong cancellation: %v", err)
			}
		})
	}
}

type semanticChangedPageFile struct {
	*os.File
	after os.FileInfo
	reads int
}

func (f *semanticChangedPageFile) Read(p []byte) (int, error) {
	f.reads++
	return f.File.Read(p)
}

func (f *semanticChangedPageFile) Stat() (os.FileInfo, error) {
	if f.reads > 0 {
		return f.after, nil
	}
	return f.File.Stat()
}

func TestSemanticDriverFilesChangedWhileReading(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"modtime", "size", "same-file"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b := semanticBinaryTestBuild(t, kit.MaxDocumentBytes+1)
			f, err := os.Open(b.Executable)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			before, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			after := before
			switch name {
			case "modtime":
				if err := os.Chtimes(b.Executable, before.ModTime(), before.ModTime().Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			case "size":
				if err := os.Truncate(b.Executable, before.Size()+1); err != nil {
					t.Fatal(err)
				}
			case "same-file":
				other := semanticBinaryTestBuild(t, b.ExecutableBytes)
				after, err = os.Stat(other.Executable)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(other.Executable, before.ModTime(), before.ModTime()); err != nil {
					t.Fatal(err)
				}
				after, err = os.Stat(other.Executable)
				if err != nil || os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
					t.Fatalf("invalid same-file control: %v", err)
				}
			}
			// os.SameFile requires concrete OS FileInfo snapshots, not a metadata wrapper.
			if name != "same-file" {
				after, err = f.Stat()
				if err != nil {
					t.Fatal(err)
				}
				if name == "size" {
					if err := os.Truncate(b.Executable, before.Size()); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Chtimes(b.Executable, before.ModTime(), before.ModTime()); err != nil {
					t.Fatal(err)
				}
				if !os.SameFile(before, after) || (name == "size" && (after.Size() == before.Size())) || (name == "modtime" && after.ModTime().Equal(before.ModTime())) {
					t.Fatal("invalid metadata-change control")
				}
			}
			seam := &semanticChangedPageFile{File: f, after: after}
			files, parts, err := semanticPageFiles(context.Background(), seam, b.ExecutableBytes, b.ExecutableSHA256, "capture/driver", "driver-paged-part", "driver")
			if err == nil || files != nil || parts != nil || seam.reads < 3 {
				t.Fatalf("changed file accepted: files=%d parts=%d reads=%d: %v", len(files), len(parts), seam.reads, err)
			}
		})
	}
}

func TestSemanticBuildFilesBinariesDriver(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		size  int64
		parts int
	}{
		{"small", 1, 0}, {"16MiB", 16 << 20, 0}, {"16MiB+1", 16<<20 + 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := semanticBinaryTestBuild(t, tc.size)
			files, err := semanticBuildFilesIn(context.Background(), b, semanticPagedBinariesContext)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"capture/build.json": "build", "capture/build-manifest.txt": "build-manifest"}
			if tc.parts == 0 {
				want["capture/driver.bin"] = "driver"
			} else {
				want["capture/driver.pages.json"] = "driver-paged-descriptor"
				for i := range tc.parts {
					want[fmt.Sprintf("capture/driver.part-%05d.bin", i)] = "driver-paged-part"
				}
			}
			if len(files) != len(want) {
				t.Fatalf("files=%d want=%d", len(files), len(want))
			}
			for _, f := range files {
				if want[f.pin.Path] != f.pin.Role || f.pin.Bytes > kit.MaxDocumentBytes || f.pin.SHA256 != sha(f.data) {
					t.Fatalf("unexpected file: %+v", f.pin)
				}
				delete(want, f.pin.Path)
			}
			if len(want) != 0 {
				t.Fatalf("missing files: %v", want)
			}
		})
	}
}

func TestSemanticBuildFilesOtherContextsKeepDriverLimit(t *testing.T) {
	t.Parallel()
	for _, captureContext := range []string{"full-input-utf8/r2", semanticPagedContext} {
		t.Run(captureContext, func(t *testing.T) {
			t.Parallel()
			// A missing file distinguishes the pre-read limit from the final physical limit.
			b := &Build{Dir: t.TempDir(), Executable: "missing.exe", ExecutableBytes: kit.MaxDocumentBytes + 1}
			files, err := semanticBuildFilesIn(context.Background(), b, captureContext)
			semanticBinaryTestError(t, err, "SEMANTICS_MEMBER_LIMIT")
			if files != nil {
				t.Fatal("limit returned partial capture")
			}
		})
	}
}

func TestSemanticPlanMatchesDriverParts(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"same", "three-parts", "plain"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			plan := semanticBinaryTestPlan(2)
			actual := semanticBinaryTestPlan(2)
			if name == "three-parts" {
				actual = semanticBinaryTestPlan(3)
			}
			if name == "plain" {
				actual.Members = []semanticCaptureMember{{Owner: "D", Path: "capture/driver.bin", Role: "driver"}}
			}
			var files []captureFile
			for _, m := range actual.Members {
				if m.Role != "manifest" {
					files = append(files, captureFile{pin: kit.ReplayMember{Path: m.Path, Role: m.Role}})
				}
			}
			if plan.matches(files, true) != (name == "same") {
				t.Fatal("buildOnly did not enforce the exact driver plan")
			}
		})
	}
}
