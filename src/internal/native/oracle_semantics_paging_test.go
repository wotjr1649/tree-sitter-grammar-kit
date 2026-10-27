package native

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestSemanticPagedParserCapture(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "grammar"), 0700); err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("p"), kit.MaxDocumentBytes+1)
	file := filepath.Join(dir, "grammar", "parser.c")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	pin := ClosureFile{Role: "grammar-parser", Path: "parser.c", Bytes: int64(len(data)), SHA256: sha(data)}
	files, err := semanticParserFiles(context.Background(), root, pin, 7)
	if err != nil || len(files) != 3 {
		t.Fatalf("capture %v files=%d", err, len(files))
	}
	var d semanticParserPages
	if err := json.Unmarshal(files[2].data, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Parts) != 2 || d.Parts[0].Bytes != kit.MaxDocumentBytes || d.Parts[1].Bytes != 1 || d.Parts[1].Offset != kit.MaxDocumentBytes || d.Logical.SHA256 != pin.SHA256 || files[2].pin.Role != "grammar-paged-descriptor" {
		t.Fatalf("wrong pages %+v", d)
	}
	if !bytes.Equal(files[0].data, data[:kit.MaxDocumentBytes]) || !bytes.Equal(files[1].data, data[kit.MaxDocumentBytes:]) {
		t.Fatal("parts differ from original")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := semanticParserFiles(ctx, root, pin, 7); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	pin.SHA256 = strings.Repeat("0", 64)
	if files, err := semanticParserFiles(context.Background(), root, pin, 7); err == nil || files != nil {
		t.Fatal("wrong logical SHA accepted")
	}
	pin.Bytes--
	if files, err := semanticParserFiles(context.Background(), root, pin, 7); err == nil || files != nil {
		t.Fatal("changed file size accepted")
	}
}

func TestSemanticCapturePlanPreflight(t *testing.T) {
	valid := `{"schema":"tsgk-query-semantics-registration/r1","context":"full-input-utf8-paged-parser/r1","planned_members":[{"owner":"O","path":"manifest.json","role":"manifest"},{"owner":"D","path":"capture/build-input-00007.pages.json","role":"grammar-paged-descriptor"},{"owner":"D","path":"capture/build-input-00007.part-00000.bin","role":"grammar-paged-part"},{"owner":"D","path":"capture/build-input-00007.part-00001.bin","role":"grammar-paged-part"}]}`
	p, err := semanticCaptureRegistration(context.Background(), []byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	grammar := []kit.NativeInput{{Role: "parser", Bytes: kit.MaxDocumentBytes + 1}}
	if err := p.parserShape(grammar); err != nil {
		t.Fatal(err)
	}
	p.Members = p.Members[:3]
	if err := p.parserShape(grammar); err == nil {
		t.Fatal("missing part accepted before build")
	}
	for _, bad := range []string{
		valid + `{}`, strings.Replace(valid, `"schema":`, `"schema":"duplicate","schema":`, 1), strings.Replace(valid, `"owner":"D"`, `"owner":"O"`, 1), strings.Replace(valid, `capture/build-input-00007.pages.json`, `../escape`, 1), strings.Replace(valid, `"owner":"D"`, `"owner":"D","extra":1`, 1), strings.Replace(valid, `"context"`, `"bad":"`+string([]byte{0xff})+`","context"`, 1), strings.Replace(valid, `"schema":`, `"padding":`+strings.Repeat("[", 33)+"0"+strings.Repeat("]", 33)+`,"schema":`, 1),
	} {
		if p, err := semanticCaptureRegistration(context.Background(), []byte(bad)); err == nil && p.Context == semanticPagedContext {
			t.Fatal("invalid registration accepted")
		}
	}
	for _, raw := range []string{`{"producer_control":"not_external_approval"}`, `opaque legacy bytes`, `["context","full-input-utf8-paged-parser/r1"]`} {
		p, err := semanticCaptureRegistration(context.Background(), []byte(raw))
		if err != nil || p.Context != "full-input-utf8/r2" {
			t.Fatalf("legacy changed: %v", err)
		}
	}
	for _, raw := range []string{
		strings.Replace(valid, `{"owner":"O","path":"manifest.json","role":"manifest"},`, "", 1),
		strings.Replace(valid, `"path":"manifest.json"`, `"path":"capture/CON"`, 1),
		strings.Replace(valid, `"role":"manifest"`, `"role":"bad!role"`, 1),
		strings.Replace(valid, `"path":"manifest.json"`, `"path":"wrong.json"`, 1),
	} {
		if _, err := semanticCaptureRegistration(context.Background(), []byte(raw)); err == nil {
			t.Fatal("missing/invalid manifest or portable plan accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := semanticCaptureRegistration(ctx, []byte(valid)); !errors.Is(err, context.Canceled) {
		t.Fatalf("preflight cancel: %v", err)
	}
}

func TestSemanticIncompletePreservesInterruption(t *testing.T) {
	for _, status := range []string{kit.StatusCancelled, kit.StatusResourceLimit} {
		for _, cleanupFailure := range []bool{false, true} {
			report := kit.Report{ExecutionStatus: status, Assessment: kit.AssessFail}
			var err error
			if cleanupFailure {
				recordCleanupFailure(&report, &err, "BUILD_CLEANUP_FAILED", errors.New("synthetic close failure"))
			}
			incompleteOracleSet(&report, true)
			if cleanupFailure {
				var ne *Error
				want := kit.KindCancelled
				if status == kit.StatusResourceLimit {
					want = kit.KindResourceLimit
				}
				if !errors.As(err, &ne) || ne.Kind != want {
					t.Fatalf("cleanup masked interruption error: %v", err)
				}
			}
			if report.ExecutionStatus != status || report.Assessment != kit.AssessFail || cleanupFailure && err == nil {
				t.Fatalf("interruption overwritten: %+v %v", report, err)
			}
		}
	}
	report := kit.Report{ExecutionStatus: kit.StatusCompleted, Assessment: kit.AssessPass}
	incompleteOracleSet(&report, true)
	if report.ExecutionStatus != kit.StatusFailed || report.Assessment != kit.AssessNotAssessed {
		t.Fatal("incomplete normal set retained PASS")
	}
}

func TestSemanticPagedPreflightInterruptions(t *testing.T) {
	valid := []byte(`{"schema":"tsgk-query-semantics-registration/r1","context":"full-input-utf8-paged-parser/r1","planned_members":[{"owner":"O","path":"manifest.json","role":"manifest"}],"padding":[1,2,3,4,5,6]}`)
	for _, kind := range []string{kit.KindCancelled, kit.KindResourceLimit} {
		calls := 0
		_, err := semanticPagedRegistration(valid, func() error {
			calls++
			if calls == 15 {
				return refuse(kind, "SYNTHETIC_INTERRUPT", nil)
			}
			return nil
		})
		var ne *Error
		if !errors.As(err, &ne) || ne.Kind != kind || ne.Code != "SYNTHETIC_INTERRUPT" {
			t.Fatalf("typed token error lost: %v", err)
		}
	}
	late := []byte(`{"padding":[` + strings.Repeat("0,", 2000001) + `0],"context":"full-input-utf8-paged-parser/r1","schema":"tsgk-query-semantics-registration/r1","planned_members":[{"owner":"O","path":"manifest.json","role":"manifest"}]}`)
	_, err := semanticCaptureRegistration(context.Background(), late)
	var limit *Error
	if !errors.As(err, &limit) || limit.Kind != kit.KindResourceLimit || limit.Code != "SEMANTICS_REGISTRATION_LIMIT" {
		t.Fatalf("late context silently downgraded: %v", err)
	}
	for _, name := range []string{"capture/CON", "capture/COM¹.log", "capture/a.", "capture/a ", "capture/../x", "capture/a\x01", "capture/a\\b"} {
		if semanticPortablePath(name) {
			t.Fatalf("nonportable accepted: %q", name)
		}
	}
	if !semanticPortablePath("capture/정상.bin") || !semanticMemberRole("grammar-paged-part") || semanticMemberRole("bad!role") {
		t.Fatal("portable/identifier contract differs")
	}
}

type semanticEOFReader struct {
	io.Reader
	ended *bool
}

func (r *semanticEOFReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		*r.ended = true
	}
	return n, err
}

func TestSemanticTokenFinalReadInterruption(t *testing.T) {
	for _, kind := range []string{kit.KindCancelled, kit.KindResourceLimit} {
		ended := false
		dec := jsontext.NewDecoder(&semanticEOFReader{Reader: strings.NewReader("{}" + strings.Repeat(" ", 2<<20)), ended: &ended})
		check := func() error {
			if ended {
				return refuse(kind, "FINAL_READ_INTERRUPTED", nil)
			}
			return nil
		}
		for {
			_, err := semanticToken(dec, check)
			if err == nil {
				continue
			}
			var ne *Error
			if !ended || !errors.As(err, &ne) || ne.Kind != kind {
				t.Fatalf("last read lost interruption: %v", err)
			}
			break
		}
	}
	_, err := semanticPagedSelected([]byte("opaque"), func() error { return refuse(kit.KindCancelled, "CANCELLED", nil) })
	var ne *Error
	if !errors.As(err, &ne) || ne.Kind != kit.KindCancelled {
		t.Fatalf("selector final check lost: %v", err)
	}
}

func TestSemanticContextAfterDeepValue(t *testing.T) {
	data := []byte(`{"padding":` + strings.Repeat("[", 10001) + "0" + strings.Repeat("]", 10001) + `,"context":"full-input-utf8-paged-parser/r1"}`)
	_, err := semanticCaptureRegistration(context.Background(), data)
	var ne *Error
	if !errors.As(err, &ne) || ne.Kind != kit.KindResourceLimit || ne.Code != "SEMANTICS_REGISTRATION_LIMIT" {
		t.Fatalf("deep value masked context: %v", err)
	}
}

func TestSemanticFailureKeepsCloseCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	closeErr := errors.New("owned parser close error")
	for _, cause := range []error{errors.Join(context.Canceled, closeErr), errors.Join(refuse(kit.KindCancelled, "CANCELLED", context.Canceled), closeErr)} {
		status, _, err := semanticFailure(ctx, ctx, cause)
		var ne *Error
		if status != kit.StatusCancelled || !errors.As(err, &ne) || ne.Kind != kit.KindCancelled || !errors.Is(err, context.Canceled) || !errors.Is(err, closeErr) {
			t.Fatalf("cancel/close chain lost: %v", err)
		}
	}
	resource := refuse(kit.KindResourceLimit, "SEMANTICS_MEMBER_LIMIT", nil)
	status, _, err := semanticFailure(context.Background(), context.Background(), errors.Join(resource, closeErr))
	var ne *Error
	if status != kit.StatusResourceLimit || !errors.As(err, &ne) || ne.Kind != kit.KindResourceLimit || !errors.Is(err, resource) || !errors.Is(err, closeErr) {
		t.Fatalf("resource/close chain lost: %v", err)
	}
}
