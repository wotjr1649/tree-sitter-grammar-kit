package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func runQueryReference(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("evidence query-reference", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "read-only evidence root")
	profile := fs.String("profile", "", "externally approved evidence policy")
	registration := fs.String("registration", "", "externally approved query reference connection registration")
	witness := fs.String("witness-registration", "", "externally approved witness registration")
	out := fs.String("out", "", "new result file outside the evidence root (no clobber)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 || *input == "" || *profile == "" || *registration == "" || *witness == "" {
		fmt.Fprintln(stderr, "tsgk: USAGE: tsgk evidence query-reference --input DIR --profile FILE --registration FILE --witness-registration FILE [--out PATH]")
		return exitUsage
	}
	var policy, registered, witnessBytes []byte
	for _, doc := range []struct {
		path string
		dst  *[]byte
	}{{*profile, &policy}, {*registration, &registered}, {*witness, &witnessBytes}} {
		// Check the resolved leaf too: an outside symlink must not approve an inside document.
		resolved, err := filepath.EvalSymlinks(doc.path)
		if err != nil {
			fmt.Fprintln(stderr, "tsgk: INPUT_PATH_UNVERIFIED: 신뢰 문서 위치를 확인할 수 없다")
			return exitUsage
		}
		for _, path := range []string{doc.path, resolved} {
			contained, err := documentInside(path, *input)
			if err != nil {
				fmt.Fprintln(stderr, "tsgk: INPUT_PATH_UNVERIFIED: 신뢰 문서 위치를 확인할 수 없다")
				return exitUsage
			}
			if contained {
				fmt.Fprintln(stderr, "tsgk: PROFILE_INSIDE_INPUT: 신뢰 문서는 검증 대상 input 밖에 있어야 한다")
				return exitUsage
			}
		}
		*doc.dst, err = readDocument(resolved)
		if err != nil {
			fmt.Fprintln(stderr, "tsgk: PROFILE_UNREADABLE: 신뢰 문서를 읽을 수 없다")
			return exitIO
		}
	}
	cctx, cancel := context.WithTimeout(ctx, kit.ReplayOperations()["evidence-replay"].Wall+5*time.Second)
	defer cancel()
	result, err := kit.VerifyQueryReferenceConnection(cctx, kit.QueryReferenceConnectionRequest{
		Evidence: kit.EvidenceRequest{Root: *input, Policy: policy}, Registration: registered, WitnessRegistration: witnessBytes,
	})
	return finish(result, err, *out, *input, stdout, stderr)
}
