package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const (
	qualifySemanticsInputSchema = "tsgk-qualification-query-semantics-input/r1"
	qualifyAdmissionFileLimit   = uint64(16 << 20)
	qualifyAdmissionTotalLimit  = uint64(256 << 20)
	qualifyAdmissionWall        = 120 * time.Second
)

var (
	errQualifyCLIWall           = errors.New("qualification CLI wall limit")
	errQualifyAdmissionWall     = errors.New("qualification CLI admission wall limit")
	errQualifyAdmissionDeadline = errors.New("qualification admission deadline exceeded")
	errQualifyPathLimit         = errors.New("qualification path identity limit exceeded")
)

type qualifySemanticsManifest struct {
	Schema  string                            `json:"schema"`
	Plan    string                            `json:"plan"`
	Targets *[]qualifySemanticsManifestTarget `json:"targets"`
}

type qualifySemanticsManifestTarget struct {
	ID       string          `json:"id"`
	Root     string          `json:"root"`
	Profile  string          `json:"profile"`
	Approval json.RawMessage `json:"approval,omitempty"`
	Context  json.RawMessage `json:"context,omitempty"`
}

type qualifySemanticRoot struct {
	path string
	root *os.Root
	info os.FileInfo
}

type qualifySemanticDocument struct {
	path   string
	parent *os.Root
	file   *os.File
	info   os.FileInfo
	data   []byte
}

type qualifySemanticAdmissionError struct {
	kind string
	code string
	path string
	err  error
}

func (e *qualifySemanticAdmissionError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return e.code
}

func runQualifySemantics(ctx context.Context, inventory, candidate, out, manifest string, rawHosts []string, stdout, stderr io.Writer) int {
	started := time.Now()
	wallDeadline := started.Add(kit.QualificationLimits().Wall)
	wallCtx, cancelWall := context.WithDeadlineCause(ctx, wallDeadline, errQualifyCLIWall)
	defer cancelWall()
	wallDeadline, _ = wallCtx.Deadline()
	admissionDeadline := started.Add(qualifyAdmissionWall)
	if wallDeadline.Before(admissionDeadline) {
		admissionDeadline = wallDeadline
	}
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(admissionDeadline) {
		admissionDeadline = callerDeadline
	}
	admissionCtx, cancelAdmission := context.WithDeadlineCause(wallCtx, admissionDeadline, errQualifyAdmissionWall)
	defer cancelAdmission()
	var admitted uint64
	fail := func(e *qualifySemanticAdmissionError) int {
		fmt.Fprintf(stderr, "tsgk: %s: %s %s read_prefix_bytes=%d", e.kind, e.code, printable(e.path), admitted)
		if e.err != nil {
			fmt.Fprintf(stderr, ": %s", printable(e.err.Error()))
		}
		fmt.Fprintln(stderr)
		return exitFor(e.kind)
	}
	check := func(path string) *qualifySemanticAdmissionError {
		if ctx.Err() != nil {
			return &qualifySemanticAdmissionError{kind: kit.KindCancelled, code: "CANCELLED", path: path, err: ctx.Err()}
		}
		if admissionCtx.Err() != nil {
			cause := context.Cause(admissionCtx)
			if errors.Is(cause, errQualifyCLIWall) || errors.Is(cause, errQualifyAdmissionWall) {
				return &qualifySemanticAdmissionError{kind: kit.KindResourceLimit, code: "WALL_LIMIT", path: path, err: cause}
			}
			return &qualifySemanticAdmissionError{kind: kit.KindCancelled, code: "CANCELLED", path: path, err: cause}
		}
		return nil
	}
	invalid := func(path, code string, err error) *qualifySemanticAdmissionError {
		return &qualifySemanticAdmissionError{kind: kit.KindInvalidInput, code: code, path: path, err: err}
	}
	resource := func(path, code string, err error) *qualifySemanticAdmissionError {
		return &qualifySemanticAdmissionError{kind: kit.KindResourceLimit, code: code, path: path, err: err}
	}
	pathFailure := func(path, code string, err error) *qualifySemanticAdmissionError {
		if errors.Is(err, errQualifyAdmissionDeadline) {
			if stop := check(path); stop != nil {
				return stop
			}
			return resource(path, "WALL_LIMIT", err)
		}
		if qualifySemanticPathLimit(err) {
			return resource(path, "PATH_IDENTITY_LIMIT", err)
		}
		return invalid(path, code, err)
	}
	if len(rawHosts) > 3 {
		return fail(invalid("hosts", "HOST_COUNT_LIMIT", nil))
	}
	if e := check(manifest); e != nil {
		return fail(e)
	}
	hosts := make([]kit.QualifyHost, 0, len(rawHosts))
	protected := make([]qualifySemanticRoot, 0, len(rawHosts)+2)
	seenPlatforms := map[string]bool{}
	closeRoots := func() {
		for _, root := range protected {
			_ = root.root.Close()
		}
	}
	defer closeRoots()
	for _, raw := range rawHosts {
		platform, path, ok := strings.Cut(raw, "=")
		if !ok || platform == "" || path == "" || seenPlatforms[platform] {
			return fail(invalid("hosts", "HOST_INVALID", nil))
		}
		seenPlatforms[platform] = true
		root, e := openQualifySemanticRoot(path, admissionDeadline)
		if e != nil {
			return fail(pathFailure(path, "ROOT_PATH_UNVERIFIED", e))
		}
		protected = append(protected, root)
		hosts = append(hosts, kit.QualifyHost{Platform: platform, Root: root.path, ExpectedRootIdentity: root.info})
		if e := check(path); e != nil {
			return fail(e)
		}
	}
	if e := disjointQualifySemanticRoots(protected, admissionDeadline); e != nil {
		return fail(pathFailure("hosts", "ROOT_OVERLAP_OR_ALIAS", e))
	}
	if e := check("hosts"); e != nil {
		return fail(e)
	}

	docs := make([]*qualifySemanticDocument, 0, 9)
	closeDocs := func() {
		for _, doc := range docs {
			if doc.file != nil {
				_ = doc.file.Close()
			}
			if doc.parent != nil {
				_ = doc.parent.Close()
			}
		}
	}
	defer closeDocs()
	manifestDoc, e := openQualifySemanticDocument(manifest, protected, admissionDeadline)
	if e != nil {
		return fail(pathFailure(manifest, "TRUST_DOCUMENT_PATH_UNVERIFIED", e))
	}
	docs = append(docs, manifestDoc)
	invDoc, e := openQualifySemanticDocument(inventory, protected, admissionDeadline)
	if e != nil {
		return fail(pathFailure(inventory, "TRUST_DOCUMENT_PATH_UNVERIFIED", e))
	}
	docs = append(docs, invDoc)
	if e := check(manifest); e != nil {
		return fail(e)
	}
	if e := preflightQualifySemanticDocuments(docs); e != nil {
		return fail(resource(manifest, "ADMISSION_BYTES_LIMIT", e))
	}
	if e := duplicateQualifySemanticDocuments(docs); e != nil {
		return fail(invalid(manifest, "TRUST_DOCUMENT_DUPLICATE", e))
	}
	if n, e := readQualifySemanticDocument(manifestDoc, qualifyAdmissionTotalLimit-admitted, admissionDeadline); e != nil {
		admitted += n
		if errors.Is(e, errQualifyAdmissionDeadline) {
			if stop := check(manifest); stop != nil {
				return fail(stop)
			}
		}
		if errors.Is(e, errQualifyPathLimit) {
			return fail(resource(manifest, "PATH_IDENTITY_LIMIT", e))
		}
		return fail(&qualifySemanticAdmissionError{kind: kit.KindIO, code: "SEMANTIC_MANIFEST_UNREADABLE", path: manifest, err: e})
	} else {
		admitted += n
	}
	if e := check(manifest); e != nil {
		return fail(e)
	}
	input, approvals, contexts, e := parseQualifySemanticsManifest(manifestDoc.data)
	if e != nil {
		return fail(invalid(manifest, "SEMANTIC_MANIFEST_INVALID", e))
	}
	manifestAbs, e := filepath.Abs(manifest)
	if e != nil {
		return fail(invalid(manifest, "SEMANTIC_MANIFEST_PATH_INVALID", e))
	}
	manifestDir := filepath.Dir(manifestAbs)
	manifestRoots := make([]qualifySemanticRoot, 0, len(*input.Targets))
	for _, target := range *input.Targets {
		path, e := resolveQualifySemanticRelative(manifestDir, target.Root)
		if e != nil {
			return fail(invalid(target.Root, "SEMANTIC_ROOT_PATH_INVALID", e))
		}
		root, e := openQualifySemanticRoot(path, admissionDeadline)
		if e != nil {
			return fail(pathFailure(path, "ROOT_PATH_UNVERIFIED", e))
		}
		protected = append(protected, root)
		manifestRoots = append(manifestRoots, root)
		if e := check(path); e != nil {
			return fail(e)
		}
	}
	if e := disjointQualifySemanticRoots(protected, admissionDeadline); e != nil {
		return fail(pathFailure(manifest, "ROOT_OVERLAP_OR_ALIAS", e))
	}
	for _, doc := range []*qualifySemanticDocument{manifestDoc, invDoc} {
		if e := qualifySemanticDocumentOutside(doc.parent, protected, admissionDeadline); e != nil {
			return fail(pathFailure(doc.path, "TRUST_DOCUMENT_PATH_UNVERIFIED", e))
		}
	}

	type targetDocuments struct {
		profile, approval, context *qualifySemanticDocument
	}
	targetDocs := make([]targetDocuments, len(*input.Targets))
	docPaths := map[string]bool{}
	for _, doc := range docs {
		docPaths[filepath.Clean(doc.path)] = true
	}
	addDoc := func(raw string) (*qualifySemanticDocument, *qualifySemanticAdmissionError) {
		path, err := resolveQualifySemanticRelative(manifestDir, raw)
		if err != nil {
			return nil, invalid(raw, "TRUST_DOCUMENT_PATH_INVALID", err)
		}
		if docPaths[filepath.Clean(path)] {
			return nil, invalid(path, "TRUST_DOCUMENT_DUPLICATE", nil)
		}
		if stop := check(path); stop != nil {
			return nil, stop
		}
		docPaths[filepath.Clean(path)] = true
		doc, err := openQualifySemanticDocument(path, protected, admissionDeadline)
		if err != nil {
			return nil, pathFailure(path, "TRUST_DOCUMENT_PATH_UNVERIFIED", err)
		}
		if stop := check(path); stop != nil {
			_ = doc.file.Close()
			_ = doc.parent.Close()
			return nil, stop
		}
		docs = append(docs, doc)
		return doc, nil
	}
	planDoc, ae := addDoc(input.Plan)
	if ae != nil {
		return fail(ae)
	}
	for i, target := range *input.Targets {
		profile, ae := addDoc(target.Profile)
		if ae != nil {
			return fail(ae)
		}
		targetDocs[i].profile = profile
		if approvals[i] != nil {
			targetDocs[i].approval, ae = addDoc(*approvals[i])
			if ae != nil {
				return fail(ae)
			}
		}
		if contexts[i] != nil {
			targetDocs[i].context, ae = addDoc(*contexts[i])
			if ae != nil {
				return fail(ae)
			}
		}
	}
	if len(docs) > 9 {
		return fail(invalid(manifest, "TRUST_DOCUMENT_COUNT_LIMIT", nil))
	}
	if e := check(manifest); e != nil {
		return fail(e)
	}
	if e := preflightQualifySemanticDocuments(docs); e != nil {
		return fail(resource(manifest, "ADMISSION_BYTES_LIMIT", e))
	}
	if e := duplicateQualifySemanticDocuments(docs); e != nil {
		return fail(invalid(manifest, "TRUST_DOCUMENT_DUPLICATE", e))
	}
	if e := rejectQualifySemanticFileAliases(docs, protected, admissionDeadline); e != nil {
		return fail(pathFailure(manifest, "TRUST_DOCUMENT_PATH_UNVERIFIED", e))
	}
	for _, doc := range docs {
		if doc == manifestDoc || doc == invDoc {
			continue
		}
		if e := check(doc.path); e != nil {
			return fail(e)
		}
		n, err := readQualifySemanticDocument(doc, qualifyAdmissionTotalLimit-admitted, admissionDeadline)
		admitted += n
		if err != nil {
			if errors.Is(err, errQualifyAdmissionDeadline) {
				if stop := check(doc.path); stop != nil {
					return fail(stop)
				}
				return fail(resource(doc.path, "WALL_LIMIT", err))
			}
			if errors.Is(err, errQualifyPathLimit) {
				return fail(resource(doc.path, "PATH_IDENTITY_LIMIT", err))
			}
			return fail(&qualifySemanticAdmissionError{kind: kit.KindIO, code: "TRUST_DOCUMENT_UNREADABLE", path: doc.path, err: err})
		}
		if e := check(doc.path); e != nil {
			return fail(e)
		}
	}
	if e := check(invDoc.path); e != nil {
		return fail(e)
	}
	if n, err := readQualifySemanticDocument(invDoc, qualifyAdmissionTotalLimit-admitted, admissionDeadline); err != nil {
		admitted += n
		if errors.Is(err, errQualifyAdmissionDeadline) {
			if stop := check(invDoc.path); stop != nil {
				return fail(stop)
			}
			return fail(resource(invDoc.path, "WALL_LIMIT", err))
		}
		if errors.Is(err, errQualifyPathLimit) {
			return fail(resource(invDoc.path, "PATH_IDENTITY_LIMIT", err))
		}
		return fail(&qualifySemanticAdmissionError{kind: kit.KindIO, code: "INVENTORY_UNREADABLE", path: invDoc.path, err: err})
	} else {
		admitted += n
	}
	if e := check(invDoc.path); e != nil {
		return fail(e)
	}
	semantics := &kit.QualificationQuerySemanticsRequest{Plan: planDoc.data, Targets: make([]kit.QualificationQuerySemanticsInput, len(*input.Targets))}
	for _, doc := range docs {
		semantics.ForbiddenFileIdentities = append(semantics.ForbiddenFileIdentities, doc.info)
	}
	for i, target := range *input.Targets {
		entry := kit.QualificationQuerySemanticsInput{ID: target.ID, Replay: kit.ReplayRequest{Root: manifestRoots[i].path, Profile: targetDocs[i].profile.data}, ExpectedRootIdentity: manifestRoots[i].info}
		if targetDocs[i].approval != nil {
			entry.EligibilityApproval = targetDocs[i].approval.data
		}
		if targetDocs[i].context != nil {
			entry.Context = targetDocs[i].context.data
		}
		semantics.Targets[i] = entry
	}
	req := kit.QualifyRequest{Inventory: invDoc.data, Candidate: candidate, Hosts: hosts, QuerySemantics: semantics}
	res, qerr := kit.Qualify(wallCtx, req)
	res, qerr = convertQualifyCLIWall(ctx, wallCtx, res, qerr)
	heldRoots := make([]*os.Root, len(protected))
	for i, root := range protected {
		heldRoots[i] = root.root
	}
	return finishWithRoots(res, qerr, out, "", stdout, stderr, nil, heldRoots, wallDeadline, ctx)
}

func convertQualifyCLIWall(caller, operation context.Context, res kit.QualificationResult, err error) (kit.QualificationResult, error) {
	var ke *kit.Error
	if err == nil || !errors.As(err, &ke) || ke.Kind != kit.KindCancelled || !errors.Is(ke.Cause, context.DeadlineExceeded) || caller.Err() != nil || !errors.Is(context.Cause(operation), errQualifyCLIWall) {
		return res, err
	}
	converted := *ke
	converted.Kind, converted.Code = kit.KindResourceLimit, "WALL_LIMIT"
	res.ExecutionStatus = kit.StatusResourceLimit
	if res.Assessment != kit.AssessFail {
		res.Assessment = kit.AssessBlocked
	}
	res.Explanation = []string{"CLI 전체 wall 한도에 도달해 qualification을 완성하지 못했다"}
	res.Findings = slices.Clone(res.Findings)
	for i := range res.Findings {
		if res.Findings[i].Code == ke.Code && ke.Code == "CANCELLED" {
			res.Findings[i].Code = converted.Code
			res.Findings[i].Message = "CLI 전체 wall 한도에 도달했다"
		}
	}
	return res, &converted
}

func decodeQualifySemanticsManifest(data []byte, dst *qualifySemanticsManifest) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return errors.New("manifest has trailing JSON")
	}
	return nil
}

func parseQualifySemanticsManifest(data []byte) (qualifySemanticsManifest, []*string, []*string, error) {
	var input qualifySemanticsManifest
	if err := decodeQualifySemanticsManifest(data, &input); err != nil {
		return input, nil, nil, err
	}
	if input.Schema != qualifySemanticsInputSchema || input.Plan == "" || input.Targets == nil || len(*input.Targets) > 2 {
		return input, nil, nil, errors.New("manifest schema or cardinality is invalid")
	}
	seen := map[string]bool{}
	approvals, contexts := make([]*string, len(*input.Targets)), make([]*string, len(*input.Targets))
	for i, target := range *input.Targets {
		if target.ID == "" || target.Root == "" || target.Profile == "" || seen[target.ID] {
			return input, nil, nil, errors.New("target fields or IDs are invalid")
		}
		var err error
		approvals[i], err = optionalQualifySemanticPath(target.Approval)
		if err != nil {
			return input, nil, nil, err
		}
		contexts[i], err = optionalQualifySemanticPath(target.Context)
		if err != nil {
			return input, nil, nil, err
		}
		seen[target.ID] = true
	}
	return input, approvals, contexts, nil
}

func optionalQualifySemanticPath(raw json.RawMessage) (*string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var path string
	if err := json.Unmarshal(raw, &path); err != nil || path == "" {
		return nil, errors.New("optional document path must be a non-empty string when present")
	}
	return &path, nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var value func() error
	value = func() error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok || seen[key] {
					return errors.New("manifest has a duplicate or invalid object key")
				}
				seen[key] = true
				if err := value(); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil || end != json.Delim('}') {
				return errors.New("manifest object is incomplete")
			}
		case '[':
			for dec.More() {
				if err := value(); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil || end != json.Delim(']') {
				return errors.New("manifest array is incomplete")
			}
		default:
			return errors.New("manifest contains an invalid JSON delimiter")
		}
		return nil
	}
	if err := value(); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("manifest has trailing JSON")
	}
	return nil
}

func resolveQualifySemanticRelative(base, path string) (string, error) {
	if path == "" || filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", errors.New("manifest paths must be non-empty and relative")
	}
	return filepath.Abs(filepath.Join(base, path))
}

func openQualifySemanticRoot(path string, deadline time.Time) (qualifySemanticRoot, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return qualifySemanticRoot{}, err
	}
	if err := checkQualifySemanticPath(abs, deadline); err != nil {
		return qualifySemanticRoot{}, err
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return qualifySemanticRoot{}, err
	}
	info, err := qualifySemanticRootInfo(root)
	if err != nil || !info.IsDir() {
		root.Close()
		if err != nil {
			return qualifySemanticRoot{}, err
		}
		return qualifySemanticRoot{}, errors.New("root is not a directory")
	}
	pathInfo, err := os.Stat(abs)
	if err != nil || !os.SameFile(info, pathInfo) {
		root.Close()
		return qualifySemanticRoot{}, errors.New("root identity changed while opening")
	}
	if err := checkQualifySemanticPath(abs, deadline); err != nil {
		root.Close()
		return qualifySemanticRoot{}, err
	}
	return qualifySemanticRoot{path: abs, root: root, info: info}, nil
}

func qualifySemanticPathLimit(err error) bool {
	return errors.Is(err, errQualifyPathLimit) || err != nil && strings.Contains(err.Error(), "limit exceeded")
}

func qualifySemanticIdentityError(err error, deadline time.Time) error {
	if qualifySemanticPathLimit(err) && time.Now().After(deadline) {
		return errQualifyAdmissionDeadline
	}
	return err
}

func qualifySemanticRootInfo(root *os.Root) (os.FileInfo, error) {
	f, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func disjointQualifySemanticRoots(roots []qualifySemanticRoot, deadline time.Time) error {
	for i := range roots {
		for j := 0; j < i; j++ {
			if time.Now().After(deadline) {
				return errQualifyAdmissionDeadline
			}
			if os.SameFile(roots[i].info, roots[j].info) {
				return errors.New("roots have the same directory identity")
			}
			inside, err := insideRootsUntil(roots[i].root, roots[j].root, deadline)
			if err != nil || inside {
				if err != nil {
					return qualifySemanticIdentityError(err, deadline)
				}
				return errors.New("roots overlap or alias")
			}
			inside, err = insideRootsUntil(roots[j].root, roots[i].root, deadline)
			if err != nil || inside {
				if err != nil {
					return qualifySemanticIdentityError(err, deadline)
				}
				return errors.New("roots overlap or alias")
			}
		}
	}
	return nil
}

func openQualifySemanticDocument(path string, roots []qualifySemanticRoot, deadline time.Time) (*qualifySemanticDocument, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := checkQualifySemanticPath(abs, deadline); err != nil {
		return nil, err
	}
	parentPath, name := filepath.Dir(abs), filepath.Base(abs)
	if !filepath.IsLocal(name) || name == "." || name == ".." {
		return nil, errors.New("document leaf is not local")
	}
	parent, err := os.OpenRoot(parentPath)
	if err != nil {
		return nil, err
	}
	parentInfo, err := qualifySemanticRootInfo(parent)
	if err != nil || !parentInfo.IsDir() {
		parent.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("document parent is not a directory")
	}
	if err := checkQualifySemanticPath(parentPath, deadline); err != nil {
		parent.Close()
		return nil, err
	}
	pathInfo, err := os.Stat(parentPath)
	if err != nil || !os.SameFile(parentInfo, pathInfo) {
		parent.Close()
		return nil, errors.New("document parent identity changed")
	}
	if err := checkQualifySemanticPath(parentPath, deadline); err != nil {
		parent.Close()
		return nil, err
	}
	for _, root := range roots {
		if time.Now().After(deadline) {
			parent.Close()
			return nil, errQualifyAdmissionDeadline
		}
		inside, err := insideRootsUntil(parent, root.root, deadline)
		if err != nil || inside {
			parent.Close()
			if err != nil {
				return nil, qualifySemanticIdentityError(err, deadline)
			}
			return nil, errors.New("trust document is inside a protected root")
		}
	}
	f, info, err := openQualifySemanticRegular(parent, name)
	if err != nil {
		parent.Close()
		return nil, err
	}
	return &qualifySemanticDocument{path: abs, parent: parent, file: f, info: info}, nil
}

func qualifySemanticDocumentOutside(parent *os.Root, roots []qualifySemanticRoot, deadline time.Time) error {
	for _, root := range roots {
		if time.Now().After(deadline) {
			return errQualifyAdmissionDeadline
		}
		inside, err := insideRootsUntil(parent, root.root, deadline)
		if err != nil {
			return qualifySemanticIdentityError(err, deadline)
		}
		if inside {
			return errors.New("trust document is inside a protected root")
		}
	}
	return nil
}

func checkQualifySemanticPath(path string, deadline time.Time) error {
	localDeadline := time.Now().Add(pathWall)
	if deadline.Before(localDeadline) {
		localDeadline = deadline
	}
	for depth, p := 0, filepath.Clean(path); ; depth++ {
		if time.Now().After(localDeadline) {
			if time.Now().After(deadline) {
				return errQualifyAdmissionDeadline
			}
			return errQualifyPathLimit
		}
		if depth == pathParents {
			return errQualifyPathLimit
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if time.Now().After(localDeadline) {
			if time.Now().After(deadline) {
				return errQualifyAdmissionDeadline
			}
			return errQualifyPathLimit
		}
		if isAlias(p, info) {
			return errors.New("path contains a link or reparse point")
		}
		up := filepath.Dir(p)
		if up == p {
			return nil
		}
		p = up
	}
}

func preflightQualifySemanticDocuments(docs []*qualifySemanticDocument) error {
	if len(docs) > 9 {
		return errors.New("admission total limit exceeded")
	}
	remaining := qualifyAdmissionTotalLimit
	for _, doc := range docs {
		if doc.info.Size() < 0 {
			return errors.New("document has an invalid length")
		}
		size := uint64(doc.info.Size())
		if size > qualifyAdmissionFileLimit || size > remaining {
			return errors.New("document length exceeds the admission limit")
		}
		remaining -= size
	}
	return nil
}

// Scan metadata once per root, comparing opened leaf identities rather than link counts alone.
func rejectQualifySemanticFileAliases(docs []*qualifySemanticDocument, roots []qualifySemanticRoot, deadline time.Time) error {
	infos := make([]os.FileInfo, len(docs))
	for i, doc := range docs {
		infos[i] = doc.info
	}
	for _, root := range roots {
		entries := 1
		localDeadline := time.Now().Add(pathWall)
		if deadline.Before(localDeadline) {
			localDeadline = deadline
		}
		found, err := containsDirectory(root.root, nil, 0, &entries, localDeadline, infos...)
		if err != nil {
			return qualifySemanticIdentityError(err, deadline)
		}
		if found {
			return errors.New("a trust document aliases a file inside an input root")
		}
	}
	return nil
}

func duplicateQualifySemanticDocuments(docs []*qualifySemanticDocument) error {
	for i := range docs {
		for j := 0; j < i; j++ {
			if filepath.Clean(docs[i].path) == filepath.Clean(docs[j].path) || os.SameFile(docs[i].info, docs[j].info) {
				return errors.New("trust documents name the same file")
			}
		}
	}
	return nil
}

func readQualifySemanticDocument(doc *qualifySemanticDocument, totalRemaining uint64, deadline time.Time) (uint64, error) {
	if doc.info.Size() < 0 {
		return 0, errors.New("document has an invalid length")
	}
	data, n, err := readQualifySemanticPrefix(doc.file, uint64(doc.info.Size()), qualifyAdmissionFileLimit, totalRemaining, deadline)
	doc.data = data
	if err != nil {
		return n, err
	}
	after, err := doc.file.Stat()
	if err != nil || !os.SameFile(doc.info, after) || after.Size() != doc.info.Size() || !after.ModTime().Equal(doc.info.ModTime()) {
		return n, errors.New("document identity or size changed during admission")
	}
	parentInfo, err := qualifySemanticRootInfo(doc.parent)
	if err != nil {
		return n, err
	}
	pathInfo, err := os.Stat(filepath.Dir(doc.path))
	if err != nil || !os.SameFile(parentInfo, pathInfo) {
		return n, errors.New("document parent identity changed during admission")
	}
	if err := checkQualifySemanticPath(filepath.Dir(doc.path), deadline); err != nil {
		return n, err
	}
	current, currentInfo, err := openQualifySemanticRegular(doc.parent, filepath.Base(doc.path))
	if err != nil {
		return n, errors.New("document leaf became a link, hard link, or reparse point")
	}
	_ = current.Close()
	if !os.SameFile(doc.info, currentInfo) {
		return n, errors.New("document leaf identity changed during admission")
	}
	return n, nil
}

func readQualifySemanticPrefix(r io.Reader, size, fileLimit, totalRemaining uint64, deadline time.Time) ([]byte, uint64, error) {
	if size > fileLimit || size > totalRemaining {
		return nil, 0, errors.New("document length exceeds the admission limit")
	}
	if size > uint64(int(^uint(0)>>1)) {
		return nil, 0, errors.New("document length cannot be represented")
	}
	data := make([]byte, 0, int(size))
	buf := make([]byte, min(uint64(32<<10), size))
	var read uint64
	for read < size {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return data, read, errQualifyAdmissionDeadline
		}
		want := min(uint64(len(buf)), size-read)
		n, err := r.Read(buf[:int(want)])
		if n < 0 || uint64(n) > want {
			return data, read, errors.New("reader returned an invalid byte count")
		}
		if n > 0 {
			data = append(data, buf[:n]...)
			read += uint64(n)
		}
		if err != nil {
			return data, read, err
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return data, read, errQualifyAdmissionDeadline
		}
		if n == 0 {
			return data, read, io.ErrNoProgress
		}
	}
	return data, read, nil
}
