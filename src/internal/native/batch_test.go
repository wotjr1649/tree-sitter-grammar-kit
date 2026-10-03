package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// S05-A20 (mechanism): the private-corpus batch splits files into driver processes,
// keeps only retained records (status, has_error, digest, capped errors), refuses a file
// whose bytes fail its encoding before any frame, and leaves nothing NOT_RUN.
func TestBatchRecords(t *testing.T) {
	b := fixtureBuild(t, "plain")
	root := t.TempDir()
	var cases []kit.IncrementalCase
	add := func(id, enc string, data []byte) {
		os.WriteFile(filepath.Join(root, id), data, 0o644)
		s := sha256.Sum256(data)
		cases = append(cases, kit.IncrementalCase{ID: id, Encoding: enc, Input: kit.NativeInput{Path: id, Role: "case", SHA256: hex.EncodeToString(s[:]), Bytes: uint64(len(data))}})
	}
	for i := 0; i < 5; i++ {
		add(fmt.Sprintf("f%d.txt", i), kit.EncodingUTF8, []byte(fmt.Sprintf("x%d = f(%d);\ny = (;\n", i, i)))
	}
	add("bad16.txt", kit.EncodingUTF16LE, []byte{'a', 0, 0, 0xd8})
	x := testContext("private-corpus-local")
	x.Output = kit.OutputRecord
	x.Op.BatchFiles = 2
	x.Declarations = &kit.Declarations{Mapping: "owned-r1", Items: []kit.NativeDeclaration{{Fact: "type_declaration", Node: "assignment", Name: "field:name"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, batches := runBatches(ctx, b, x, root, cases)
	if len(batches) != 3 {
		t.Fatalf("batches %d", len(batches))
	}
	for _, bt := range batches {
		if bt.TrailingBytes != 0 || !bt.Process.Cleanup.Verified || bt.Process.ExitCode != 0 || bt.Fatal != "" {
			t.Fatalf("batch %+v", bt)
		}
	}
	for _, c := range out[:5] {
		s := c.Steps
		if c.ExecutionStatus != kit.StatusCompleted || len(s) != 1 || s[0].Incremental.Form != "record" || !s[0].Incremental.HasError ||
			s[0].Incremental.Summary.Errors.Total == 0 || len(s[0].Incremental.Digest) != 64 || s[0].Incremental.Summary.Declarations.Assessment == "" {
			t.Fatalf("record %s: %s %s %+v", c.ID, c.ExecutionStatus, c.Code, s)
		}
	}
	if bad := out[5]; bad.ExecutionStatus != kit.StatusNotRun || bad.Assessment != kit.AssessBlocked || bad.Code != "SOURCE_ENCODING_INVALID" {
		t.Fatalf("encoding-invalid file %+v", bad)
	}
}

// S05-A09: bytes after the last batch response make every frame of that batch unaccepted.
func TestBatchTrailingBytes(t *testing.T) {
	b := fixtureBuild(t, "plain", "TSGK_FAULT_TRAILING")
	root := t.TempDir()
	data := []byte("x = 1;\n")
	os.WriteFile(filepath.Join(root, "a.txt"), data, 0o644)
	s := sha256.Sum256(data)
	cases := []kit.IncrementalCase{{ID: "a", Encoding: kit.EncodingUTF8, Input: kit.NativeInput{Path: "a.txt", Role: "case", SHA256: hex.EncodeToString(s[:]), Bytes: uint64(len(data))}}}
	x := testContext("private-corpus-local")
	x.Output = kit.OutputRecord
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, batches := runBatches(ctx, b, x, root, cases)
	if len(batches) != 1 || batches[0].TrailingBytes != 4 || out[0].ExecutionStatus != kit.StatusFailed || out[0].Code != "RESPONSE_TRAILING_BYTES" {
		t.Fatalf("trailing batch accepted: %+v %s %s", batches, out[0].ExecutionStatus, out[0].Code)
	}
}
