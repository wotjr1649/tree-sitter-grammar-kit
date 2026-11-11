package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
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
	inventory := fs.String("inventory", "", "qualification inventory (tsgk-qualification-inventory/r8 or historical r7/r6/r5), outside every host directory")
	candidate := fs.String("candidate", "", "candidate commit every host must have run (40 hex)")
	out := fs.String("out", "", "new result file outside every host directory (no clobber)")
	querySemantics := fs.String("query-semantics", "", "tsgk-qualification-query-semantics-input/r1 manifest; document paths are relative to its directory; approval and context are optional")
	var hosts multi
	var runtimePins multi
	fs.Var(&hosts, "host", "PLATFORM=DIR host run directory (repeat per platform)")
	fs.Var(&runtimePins, "runtime-pins", "PLATFORM=FILE bounded external runtime pin envelope (repeat per platform)")
	usage := "tsgk: USAGE: tsgk qualify --inventory FILE --candidate SHA --host PLATFORM=DIR [--host PLATFORM=DIR ...] [--query-semantics FILE | --runtime-pins PLATFORM=FILE ...] [--out PATH]"
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 || *inventory == "" || *candidate == "" || len(hosts) == 0 {
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	semanticsProvided := false
	runtimePinsProvided := false
	fs.Visit(func(f *flag.Flag) {
		semanticsProvided = semanticsProvided || f.Name == "query-semantics"
		runtimePinsProvided = runtimePinsProvided || f.Name == "runtime-pins"
	})
	if semanticsProvided {
		if runtimePinsProvided {
			fmt.Fprintln(stderr, "tsgk: USAGE: --query-semantics and --runtime-pins cannot be combined")
			return exitUsage
		}
		if *querySemantics == "" {
			fmt.Fprintln(stderr, "tsgk: INVALID_INPUT: SEMANTIC_MANIFEST_PATH_REQUIRED")
			return exitUsage
		}
		return runQualifySemantics(ctx, *inventory, *candidate, *out, *querySemantics, hosts, stdout, stderr)
	}
	if runtimePinsProvided {
		return runQualifyWithRuntimePins(ctx, *inventory, *candidate, *out, hosts, runtimePins, stdout, stderr)
	}
	req := kit.QualifyRequest{Candidate: *candidate}
	for _, h := range hosts {
		p, dir, ok := strings.Cut(h, "=")
		if !ok || p == "" || dir == "" {
			fmt.Fprintln(stderr, usage)
			return exitUsage
		}
		contained, err := documentInside(*inventory, dir)
		if err != nil {
			fmt.Fprintln(stderr, "tsgk: INPUT_PATH_UNVERIFIED: 신뢰 문서 위치를 확인할 수 없다")
			return exitUsage
		}
		if contained {
			fmt.Fprintln(stderr, "tsgk: INVENTORY_INSIDE_INPUT: 신뢰 문서는 host 근거 밖에 있어야 한다")
			return exitUsage
		}
		req.Hosts = append(req.Hosts, kit.QualifyHost{Platform: p, Root: dir})
	}
	if *out != "" {
		for _, h := range req.Hosts {
			contained, err := documentInside(*out, h.Root)
			if err != nil {
				fmt.Fprintln(stderr, "tsgk: OUTPUT_PATH_UNVERIFIED: 출력 위치를 확인할 수 없다")
				return exitUsage
			}
			if contained {
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
	var additionalRoots []string
	if len(req.Hosts) > 0 {
		publishRoot = req.Hosts[0].Root
		for _, h := range req.Hosts[1:] {
			additionalRoots = append(additionalRoots, h.Root)
		}
	}
	return finish(res, qerr, *out, publishRoot, stdout, stderr, additionalRoots...)
}

func runQualifyWithRuntimePins(ctx context.Context, inventory, candidate, out string, rawHosts, rawPins []string, stdout, stderr io.Writer) int {
	started := time.Now()
	wallDeadline := started.Add(kit.QualificationLimits().Wall)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(wallDeadline) {
		wallDeadline = callerDeadline
	}
	admissionDeadline := started.Add(qualifyAdmissionWall)
	if wallDeadline.Before(admissionDeadline) {
		admissionDeadline = wallDeadline
	}
	if err := checkCLIAdmission(ctx, admissionDeadline); err != nil {
		return reportCLIAdmissionFailure(stderr, ctx, err, "INPUT_PATH_UNVERIFIED", inventory)
	}
	if len(rawHosts) == 0 || len(rawHosts) > 3 || len(rawPins) == 0 || len(rawPins) > len(rawHosts) {
		fmt.Fprintln(stderr, "tsgk: INVALID_INPUT: HOST_OR_RUNTIME_PIN_COUNT")
		return exitUsage
	}
	roots := make([]qualifySemanticRoot, 0, len(rawHosts))
	closeRoots := func() {
		for _, root := range roots {
			_ = root.root.Close()
		}
	}
	defer closeRoots()
	hosts := make([]kit.QualifyHost, 0, len(rawHosts))
	hostIndex := map[string]int{}
	for _, raw := range rawHosts {
		if err := checkCLIAdmission(ctx, admissionDeadline); err != nil {
			return reportCLIAdmissionFailure(stderr, ctx, err, "INPUT_PATH_UNVERIFIED", raw)
		}
		platform, path, ok := strings.Cut(raw, "=")
		if !ok || platform == "" || path == "" {
			fmt.Fprintln(stderr, "tsgk: INVALID_INPUT: HOST_INVALID")
			return exitUsage
		}
		if _, exists := hostIndex[platform]; exists {
			fmt.Fprintln(stderr, "tsgk: INVALID_INPUT: HOST_INVALID")
			return exitUsage
		}
		root, err := openQualifySemanticRoot(path, admissionDeadline)
		if err != nil {
			return reportCLIAdmissionFailure(stderr, ctx, err, "INPUT_PATH_UNVERIFIED", path)
		}
		roots = append(roots, root)
		hostIndex[platform] = len(hosts)
		hosts = append(hosts, kit.QualifyHost{Platform: platform, Root: root.path, ExpectedRootIdentity: root.info})
		if err := checkCLIAdmission(ctx, admissionDeadline); err != nil {
			return reportCLIAdmissionFailure(stderr, ctx, err, "INPUT_PATH_UNVERIFIED", path)
		}
	}
	if err := checkCLIAdmission(ctx, admissionDeadline); err != nil {
		return reportCLIAdmissionFailure(stderr, ctx, err, "INPUT_PATH_UNVERIFIED", "hosts")
	}
	if err := disjointQualifySemanticRoots(roots, admissionDeadline); err != nil {
		return reportCLIAdmissionFailure(stderr, ctx, err, "HOST_ROOT_OVERLAP_OR_ALIAS", "hosts")
	}
	if err := checkCLIAdmission(ctx, admissionDeadline); err != nil {
		return reportCLIAdmissionFailure(stderr, ctx, err, "INPUT_PATH_UNVERIFIED", "hosts")
	}
	pinPaths := make(map[string]string, len(rawPins))
	docPaths := []string{inventory}
	for _, raw := range rawPins {
		if err := checkCLIAdmission(ctx, admissionDeadline); err != nil {
			return reportCLIAdmissionFailure(stderr, ctx, err, "TRUST_DOCUMENT_UNREADABLE", raw)
		}
		platform, path, ok := strings.Cut(raw, "=")
		if !ok || platform == "" || path == "" {
			fmt.Fprintln(stderr, "tsgk: INVALID_INPUT: RUNTIME_PINS_HOST_INVALID")
			return exitUsage
		}
		if _, exists := hostIndex[platform]; !exists {
			fmt.Fprintln(stderr, "tsgk: INVALID_INPUT: RUNTIME_PINS_HOST_INVALID")
			return exitUsage
		}
		if _, exists := pinPaths[platform]; exists {
			fmt.Fprintln(stderr, "tsgk: INVALID_INPUT: RUNTIME_PINS_DUPLICATE_HOST")
			return exitUsage
		}
		pinPaths[platform] = path
		docPaths = append(docPaths, path)
	}
	if err := checkCLIAdmission(ctx, admissionDeadline); err != nil {
		return reportCLIAdmissionFailure(stderr, ctx, err, "TRUST_DOCUMENT_UNREADABLE", inventory)
	}
	docs, err := openCLIEvidencePinsDocuments(docPaths, roots, admissionDeadline)
	if err != nil {
		return reportCLIAdmissionFailure(stderr, ctx, err, "TRUST_DOCUMENT_UNREADABLE", inventory)
	}
	defer func() {
		for _, doc := range docs {
			_ = doc.file.Close()
			_ = doc.parent.Close()
		}
	}()
	if err := checkCLIAdmission(ctx, admissionDeadline); err != nil {
		return reportCLIAdmissionFailure(stderr, ctx, err, "TRUST_DOCUMENT_UNREADABLE", inventory)
	}
	req := kit.QualifyRequest{Candidate: candidate, Inventory: docs[0].data, Hosts: hosts}
	runtimeByPlatform := make(map[string]*kit.EvidencePinsRequest, len(rawPins))
	for i, raw := range rawPins {
		if err := checkCLIAdmission(ctx, admissionDeadline); err != nil {
			return reportCLIAdmissionFailure(stderr, ctx, err, "RUNTIME_PINS_ENVELOPE_INVALID", raw)
		}
		platform, path, _ := strings.Cut(raw, "=")
		runtimeByPlatform[platform], err = decodeCLIEvidencePins(docs[i+1].data, nil)
		if err != nil {
			if stop := checkCLIAdmission(ctx, admissionDeadline); stop != nil {
				return reportCLIAdmissionFailure(stderr, ctx, stop, "RUNTIME_PINS_ENVELOPE_INVALID", path)
			}
			fmt.Fprintf(stderr, "tsgk: INVALID_INPUT: RUNTIME_PINS_ENVELOPE_INVALID %s: %s\n", printable(path), printable(err.Error()))
			return exitUsage
		}
	}
	for i, host := range req.Hosts {
		req.Hosts[i].RuntimePins = runtimeByPlatform[host.Platform]
	}
	if err := checkCLIAdmission(ctx, admissionDeadline); err != nil {
		return reportCLIAdmissionFailure(stderr, ctx, err, "RUNTIME_PINS_ENVELOPE_INVALID", inventory)
	}
	wallCtx, cancelWall := context.WithDeadlineCause(ctx, wallDeadline, errQualifyCLIWall)
	defer cancelWall()
	res, qerr := kit.Qualify(wallCtx, req)
	res, qerr = convertQualifyCLIWall(ctx, wallCtx, res, qerr)
	var additionalRoots []string
	for _, host := range req.Hosts[1:] {
		additionalRoots = append(additionalRoots, host.Root)
	}
	heldRoots := make([]*os.Root, len(roots))
	for i, root := range roots {
		heldRoots[i] = root.root
	}
	return finishWithRoots(res, qerr, out, req.Hosts[0].Root, stdout, stderr, additionalRoots, heldRoots, wallDeadline, ctx)
}
