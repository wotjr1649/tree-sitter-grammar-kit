package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// runQualify handles `tsgk qualify` (S08): one candidate's host run directories aggregated
// into the inventory's mandatory cells and extra-role rows. It reads only, starts no
// process and needs no tool.
func runQualify(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("qualify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inventory := fs.String("inventory", "", "qualification inventory (tsgk-qualification-inventory/r1), outside every host directory")
	candidate := fs.String("candidate", "", "candidate commit every host must have run (40 hex)")
	out := fs.String("out", "", "new result file outside every host directory (no clobber)")
	var hosts multi
	fs.Var(&hosts, "host", "PLATFORM=DIR host run directory (repeat per platform)")
	usage := "tsgk: USAGE: tsgk qualify --inventory FILE --candidate SHA --host PLATFORM=DIR [--host PLATFORM=DIR ...] [--out PATH]"
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 || *inventory == "" || *candidate == "" || len(hosts) == 0 {
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	req := kit.QualifyRequest{Candidate: *candidate}
	for _, h := range hosts {
		p, dir, ok := strings.Cut(h, "=")
		if !ok || p == "" || dir == "" {
			fmt.Fprintln(stderr, usage)
			return exitUsage
		}
		if documentInside(*inventory, dir) {
			fmt.Fprintln(stderr, "tsgk: INVENTORY_INSIDE_INPUT: 신뢰 문서는 host 근거 밖에 있어야 한다")
			return exitUsage
		}
		req.Hosts = append(req.Hosts, kit.QualifyHost{Platform: p, Root: dir})
	}
	if *out != "" {
		for _, h := range req.Hosts {
			if documentInside(*out, h.Root) {
				fmt.Fprintln(stderr, "tsgk: OUTPUT_INSIDE_INPUT: 결과는 host 근거 밖에 써야 한다")
				return exitUsage
			}
		}
	}
	data, err := readDocument(*inventory)
	if err != nil {
		fmt.Fprintf(stderr, "tsgk: INVENTORY_UNREADABLE: %v\n", err)
		return exitIO
	}
	req.Inventory = data
	cctx, cancel := context.WithTimeout(ctx, kit.QualificationLimits().Wall+5*time.Second)
	defer cancel()
	res, qerr := kit.Qualify(cctx, req)
	publishRoot := ""
	if len(req.Hosts) > 0 {
		publishRoot = req.Hosts[0].Root
	}
	return finish(res, qerr, *out, publishRoot, stdout, stderr)
}
