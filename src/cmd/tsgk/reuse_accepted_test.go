package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestReuseAcceptedCLIEmptyFlag(t *testing.T) {
	for _, kind := range []string{"incremental", "oracle"} {
		var out, err bytes.Buffer
		code := runNative(context.Background(), kind, []string{"--reuse-accepted="}, &out, &err)
		if code != exitUsage || out.Len() != 0 || !strings.Contains(err.String(), "empty --reuse-accepted") {
			t.Fatalf("%s code=%d out=%s err=%s", kind, code, out.String(), err.String())
		}
	}
}
