// Command csharp-g1-approval shows and checks the approval tuple of the #233 G1 bundle. It is read-only: it never writes
// into src/contracts/evidence-pins; after the user confirms the tuple, the object is written there by hand.
//
//	go run ./src/dev/csharp-g1-approval -root . -print > tuple.json   canonical tuple bytes on stdout, sha256 and length on stderr
//	go run ./src/dev/csharp-g1-approval -root . -verify               exit 0 APPROVED, 3 PENDING_APPROVAL (no object), 1 mismatch
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const (
	exitMismatch = 1
	exitUsage    = 2
	exitPending  = 3
)

func run(root string, print, verify bool, stdout, stderr io.Writer) int {
	fsys := os.DirFS(root)
	switch {
	case print == verify:
		fmt.Fprintln(stderr, "exactly one of -print, -verify is required")
		return exitUsage
	case print:
		data, sum, err := kit.BuildCSharpG1Approval(fsys)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitMismatch
		}
		fmt.Fprintf(stderr, "tuple: %d bytes, sha256 %s\nobject: src/contracts/evidence-pins/objects/%s\n", len(data), sum, sum)
		_, _ = stdout.Write(data)
		return 0
	}
	state, object, err := kit.VerifyCSharpG1Approval(fsys)
	switch {
	case err != nil:
		fmt.Fprintln(stderr, err)
		return exitMismatch
	case state == kit.CSharpG1ApprovalPending:
		fmt.Fprintf(stdout, "%s: no approval object; the G1 bundle is not approved\n", state)
		return exitPending
	}
	fmt.Fprintf(stdout, "%s by object %s\n", state, object)
	return 0
}

func main() {
	root := flag.String("root", ".", "repository root")
	print := flag.Bool("print", false, "write the canonical tuple bytes of the current tracked assets to stdout")
	verify := flag.Bool("verify", false, "verify the tracked assets against the approval object")
	flag.Parse()
	os.Exit(run(*root, *print, *verify, os.Stdout, os.Stderr))
}
