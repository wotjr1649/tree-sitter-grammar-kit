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

func TestOracleSemanticsCLIFlags(t *testing.T) {
	for _, tc := range []struct {
		kind string
		args []string
		want string
	}{
		{"oracle", []string{"--query-semantics="}, "empty --query-semantics"},
		{"oracle", []string{"--query-semantics", "registration.json"}, "[--query-semantics FILE]"},
		{"incremental", []string{"--query-semantics", "registration.json"}, "flag provided but not defined"},
	} {
		var out, stderr bytes.Buffer
		code := runNative(context.Background(), tc.kind, tc.args, &out, &stderr)
		if code != exitUsage || out.Len() != 0 || !strings.Contains(stderr.String(), tc.want) {
			t.Fatalf("%s flags: %d %q %q", tc.kind, code, out.String(), stderr.String())
		}
	}
}
