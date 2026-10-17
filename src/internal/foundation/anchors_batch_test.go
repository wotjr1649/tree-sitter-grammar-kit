package foundation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestConversionOutputBound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := conversionOutput{cancel: cancel}
	if n, err := b.Write(nil); n != 0 || err != nil {
		t.Fatal("empty write", n, err)
	}
	if n, err := b.Write([]byte("prefix")); n != 6 || err != nil {
		t.Fatal("normal write", n, err)
	}
	if n, err := b.Write(bytes.Repeat([]byte{'x'}, convertOutputLimit-6)); n != convertOutputLimit-6 || err != nil || b.exceeded || ctx.Err() != nil {
		t.Fatal("exact boundary", n, err)
	}
	for _, p := range [][]byte{[]byte("overflow"), []byte("after"), nil} {
		if n, err := b.Write(p); n != 0 || err == nil || !b.exceeded || ctx.Err() == nil || b.buf.Len() != convertOutputLimit {
			t.Fatal("overflow accepted", n, err)
		}
	}
}

func TestConversionOutputCopyBound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := conversionOutput{cancel: cancel}
	// Hide WriterTo so io.Copy exercises the destination's interfaces as os/exec does.
	src := struct{ io.Reader }{bytes.NewReader(bytes.Repeat([]byte{'x'}, convertOutputLimit+1))}
	if _, err := io.Copy(&b, src); err == nil || !b.exceeded || ctx.Err() == nil || b.buf.Len() > convertOutputLimit {
		t.Fatal("copy bypassed the output bound", err, b.buf.Len())
	}
}

func TestAnchorConversionBatch(t *testing.T) {
	root := repository(t)
	dir := t.TempDir()
	write := func(name, doc string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(doc), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	first := write("first.json", anchorFixture)
	shifted := strings.Replace(anchorFixture, "é = aaa;", "Zé = aaa;", 1)
	if shifted == anchorFixture {
		t.Fatal("fixture prefix was not changed")
	}
	second := write("second.json", shifted)
	files, ok, err := runConvertCaseFiles(t, root, []string{first, second})
	if !ok {
		t.Skip("pwsh not available")
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || len(files[0].Cases) != 1 || len(files[1].Cases) != 1 || files[0].Cases[0].ID != files[1].Cases[0].ID {
		t.Fatal("batch shape or same-id fixture changed")
	}
	for i, e := range files[0].Cases[0].Expect {
		if !slices.Equal(e.Anchors, anchorFixtureWant[i]) {
			t.Fatalf("first source expect %d", i)
		}
		want := append([]kit.ExpectAnchor(nil), anchorFixtureWant[i]...)
		for j := range want {
			want[j].StartByte++
			want[j].EndByte++
		}
		if !slices.Equal(files[1].Cases[0].Expect[i].Anchors, want) {
			t.Fatalf("second source expect %d not shifted: %v", i, files[1].Cases[0].Expect[i].Anchors)
		}
	}
	bad := write("bad.json", strings.Replace(anchorFixture, `{"type": "first", "text": "call"}`, `{"type": "first", "text": "absent"}`, 1))
	for _, paths := range [][]string{{first, bad}, {bad, first}, {first, second, first, second, bad}} {
		got, _, err := runConvertCaseFiles(t, root, paths)
		if !errors.Is(err, errAnchorRefused) || got != nil {
			t.Fatalf("partial batch accepted or wrong refusal: %v", err)
		}
	}
	if got, err := runConvertCaseBatch(t, root, filepath.Join(dir, "missing-pwsh"), []string{first}); err == nil || errors.Is(err, errAnchorRefused) || got != nil {
		t.Fatalf("process start failure accepted: %v", err)
	}
	// StrictMode's null-object error originates inside Resolve-Anchor, not its explicit throws.
	nullAnchor := write("null-anchor.json", strings.Replace(anchorFixture, `{"type": "first", "text": "call"}`, `null`, 1))
	for name, paths := range map[string][]string{"empty": {}, "missing": {filepath.Join(dir, "absent.json")}, "non-domain-resolver-error": {nullAnchor}} {
		t.Run(name, func(t *testing.T) {
			got, _, err := runConvertCaseFiles(t, root, paths)
			if err == nil || errors.Is(err, errAnchorRefused) || got != nil {
				t.Fatalf("infrastructure failure accepted as anchor refusal: %v", err)
			}
		})
	}
}

func TestConvertedCoverageRejectsMissingAndReorderedResults(t *testing.T) {
	root := t.TempDir()
	inputs := make([]conversionInput, 2)
	got := make([]convertedFile, 2)
	for i, src := range []string{"a", "different"} {
		id := "case" + strconv.Itoa(i)
		doc := `{"cases":[{"id":"` + id + `","source_utf8":"` + src + `","expect":[{"step":0}]}]}`
		if err := json.Unmarshal([]byte(doc), &inputs[i]); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(src))
		rel := "cases/" + id + ".txt"
		got[i].Cases = []convertedCase{{ID: id, Input: kit.NativeInput{Path: rel, Role: "case", SHA256: hex.EncodeToString(sum[:]), Bytes: uint64(len(src))}, Expect: []kit.StepExpectation{{Step: 0}}}}
		p := filepath.Join(root, strconv.Itoa(i), filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateConvertedFiles(got, inputs, root); err != nil {
		t.Fatal(err)
	}
	original, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]convertedFile) []convertedFile{
		"missing-file":        func(f []convertedFile) []convertedFile { return f[:1] },
		"empty-files":         func(f []convertedFile) []convertedFile { return nil },
		"missing-case":        func(f []convertedFile) []convertedFile { f[0].Cases = nil; return f },
		"duplicate-case":      func(f []convertedFile) []convertedFile { f[0].Cases = append(f[0].Cases, f[0].Cases[0]); return f },
		"wrong-id":            func(f []convertedFile) []convertedFile { f[0].Cases[0].ID = "other"; return f },
		"reordered-files":     func(f []convertedFile) []convertedFile { f[0], f[1] = f[1], f[0]; return f },
		"missing-expectation": func(f []convertedFile) []convertedFile { f[0].Cases[0].Expect = nil; return f },
		"wrong-step":          func(f []convertedFile) []convertedFile { f[0].Cases[0].Expect[0].Step = 1; return f },
		"unbound-path":        func(f []convertedFile) []convertedFile { f[0].Cases[0].Input.Path = "../other"; return f },
		"wrong-source": func(f []convertedFile) []convertedFile {
			f[0].Cases[0].Input.SHA256 = strings.Repeat("0", 64)
			return f
		},
	} {
		t.Run(name, func(t *testing.T) {
			var f []convertedFile
			if err := json.Unmarshal(original, &f); err != nil {
				t.Fatal(err)
			}
			if validateConvertedFiles(mutate(f), inputs, root) == nil {
				t.Fatal("incomplete or unbound coverage accepted")
			}
		})
	}
	if err := os.WriteFile(filepath.Join(root, "0", "cases", "case0.txt"), []byte("overwritten"), 0600); err != nil {
		t.Fatal(err)
	}
	if validateConvertedFiles(got, inputs, root) == nil {
		t.Fatal("overwritten output accepted")
	}
}
