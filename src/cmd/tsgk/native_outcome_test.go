package main

import (
	"bytes"
	"errors"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"testing"
)

func TestNativeInterruptedCleanupExit(t *testing.T) {
	for _, tc := range []struct {
		status string
		code   int
	}{{kit.StatusCancelled, 130}, {kit.StatusResourceLimit, 3}} {
		for _, err := range []error{nil, &native.Error{Kind: kit.KindIO, Code: "BUILD_CLEANUP_FAILED"}, errors.New("close failure")} {
			var stderr bytes.Buffer
			if got := nativeResultExit(tc.status, kit.AssessBlocked, err, &stderr); got != tc.code {
				t.Fatalf("status=%s err=%v exit=%d", tc.status, err, got)
			}
		}
	}
	if got := nativeResultExit(kit.StatusCompleted, kit.AssessPass, &native.Error{Kind: kit.KindIO, Code: "BUILD_CLEANUP_FAILED"}, &bytes.Buffer{}); got != 4 {
		t.Fatalf("ordinary cleanup error hidden: %d", got)
	}
}
