// Command csharp-g1-approval shows and checks the approval tuples of the #233 G1 bundles. It is read-only: it never writes
// into src/contracts/evidence-pins; after the user confirms a tuple, the object is written there by hand.
//
//	go run ./src/dev/csharp-g1-approval -root . -print > tuple.json   canonical tuple bytes on stdout, sha256 and length on stderr
//	go run ./src/dev/csharp-g1-approval -root . -verify               exit 0 everything approved, 3 PENDING_APPROVAL (no object), 4 pending assets, 1 mismatch
//	go run ./src/dev/csharp-g1-approval -root . -b34 -print           the tuple of the second bundle (G1-3b); the first one must be approved
//
// -verify checks the first bundle and then the second one: the second is reported as PENDING_APPROVAL, with its assets listed as pending, until
// its object exists. Exit 0 means that every tracked G1 asset is approved. The first bundle approved with assets pending is exit 4, so that
// "APPROVED by object ..." on the first line is not read as the approval of the pending assets.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const (
	exitMismatch = 1
	exitUsage    = 2
	exitPending  = 3
	// exitPendingAssets: the first bundle is approved, assets of the second one are tracked and not approved
	exitPendingAssets = 4
)

func run(root string, print, verify, b34 bool, stdout, stderr io.Writer) int {
	fsys := os.DirFS(root)
	switch {
	case print == verify:
		fmt.Fprintln(stderr, "exactly one of -print, -verify is required")
		return exitUsage
	case b34 && verify:
		fmt.Fprintln(stderr, "-b34 goes with -print; -verify checks both bundles")
		return exitUsage
	case print:
		build, scope := kit.BuildCSharpG1Approval, kit.CSharpG1ApprovalScope
		if b34 {
			build, scope = kit.BuildCSharpG1ApprovalB34, kit.CSharpG1ApprovalScopeB34
		}
		data, sum, err := build(fsys)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitMismatch
		}
		fmt.Fprintf(stderr, "scope: %s\ntuple: %d bytes, sha256 %s\nobject: src/contracts/evidence-pins/objects/%s\n", scope, len(data), sum, sum)
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
	state2, object2, err := kit.VerifyCSharpG1ApprovalB34(fsys)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitMismatch
	}
	if state2 == kit.CSharpG1ApprovalApproved {
		fmt.Fprintf(stdout, "%s by object %s (scope %s, base %s)\n", state2, object2, kit.CSharpG1ApprovalScopeB34, object)
	} else {
		fmt.Fprintf(stdout, "%s: no approval object for scope %s; the G1-3b bundle is not approved\n", state2, kit.CSharpG1ApprovalScopeB34)
	}
	if pending := kit.CSharpG1PendingAssets(fsys); len(pending) > 0 {
		fmt.Fprintf(stdout, "pending, not approved: %s\n", strings.Join(pending, " "))
		return exitPendingAssets
	}
	return 0
}

func main() {
	root := flag.String("root", ".", "repository root")
	print := flag.Bool("print", false, "write the canonical tuple bytes of the current tracked assets to stdout")
	verify := flag.Bool("verify", false, "verify the tracked assets against the approval objects")
	b34 := flag.Bool("b34", false, "with -print: the tuple of the second bundle (G1-3b) instead of the first")
	flag.Parse()
	os.Exit(run(*root, *print, *verify, *b34, os.Stdout, os.Stderr))
}
