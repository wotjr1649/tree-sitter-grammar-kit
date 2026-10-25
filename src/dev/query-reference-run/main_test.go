package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv("TSGK_REFERENCE_BYTES_HELPER"); mode != "" {
		if mode == "raw" {
			_, _ = os.Stdout.Write([]byte{0xff, 0, 'A', '\n', 0xc3, 0xa9})
			_, _ = os.Stderr.Write([]byte{0xfe})
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestReceiptPreservesRawStreams(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("new macOS verification excluded by user scope")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"raw", "empty"} {
		t.Run(mode, func(t *testing.T) {
			spec := runner.Spec{Path: self, Dir: t.TempDir(), Env: []string{"TSGK_REFERENCE_BYTES_HELPER=" + mode},
				StdoutBytes: 1024, StderrBytes: 1024, Wall: 5 * time.Second, Grace: time.Second,
				Memory: runner.Memory{Bytes: 1 << 30, Hard: runtime.GOOS == "windows"}}
			request, err := json.Marshal(spec)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := run(bytes.NewReader(request), &out); err != nil {
				t.Fatal(err)
			}
			var receipt struct {
				Schema    string        `json:"schema"`
				Execution runner.Result `json:"execution"`
				Stdout    string        `json:"stdout_base64"`
				Stderr    string        `json:"stderr_base64"`
			}
			if err := json.Unmarshal(out.Bytes(), &receipt); err != nil {
				t.Fatal(err)
			}
			wantOut, wantErr := []byte{}, []byte{}
			if mode == "raw" {
				wantOut, wantErr = []byte{0xff, 0, 'A', '\n', 0xc3, 0xa9}, []byte{0xfe}
			}
			if receipt.Schema != "tsgk-reference-process/r1" || receipt.Stdout != base64.StdEncoding.EncodeToString(wantOut) || receipt.Stderr != base64.StdEncoding.EncodeToString(wantErr) || receipt.Execution.StdoutBytes != int64(len(wantOut)) || receipt.Execution.StderrBytes != int64(len(wantErr)) || !receipt.Execution.Succeeded() || receipt.Execution.StdoutTruncated || receipt.Execution.StderrTruncated || receipt.Execution.Cleanup.ResidualAfterExit != 0 {
				t.Fatalf("raw stream receipt mismatch: %s", out.Bytes())
			}
		})
	}
}

func TestRejectRequestBeforeLaunch(t *testing.T) {
	for _, input := range []string{`{}`, `{"unknown":1}`, `{"Wall":1,"Wall":2}`, `{"Wall":900000000001}`, `{"Wall":1000000000,"Grace":1000000000,"Memory":{"Bytes":4294967297}}`, strings.Repeat(" ", 65537), `null`} {
		if err := run(strings.NewReader(input), io.Discard); err == nil {
			t.Fatalf("accepted invalid request of %d bytes", len(input))
		}
	}
}

func TestNanosecondDurationsReachRunner(t *testing.T) {
	err := run(strings.NewReader(`{"Path":"relative","Wall":1000000000,"Grace":1000000000,"StdoutBytes":1024,"StderrBytes":1024,"Memory":{"Bytes":1048576}}`), io.Discard)
	var refusal *runner.Error
	if !errors.As(err, &refusal) || refusal.Code != "TOOL_PATH_NOT_ABSOLUTE" {
		t.Fatalf("valid duration did not reach runner: %v", err)
	}
}
