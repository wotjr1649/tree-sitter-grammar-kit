package main

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
)

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
