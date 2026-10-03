package kit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Verify assessments, subjects and scopes.
const (
	AssessPass = "PASS"
	AssessFail = "FAIL"

	SubjectDirectory = "DIRECTORY"
	SubjectArchive   = "ARCHIVE"

	ScopeKnownPaths     = DiscoveryPolicy
	ScopeListed         = "listed-r1"
	ScopeArchiveMembers = "archive-members-r1"
)

// VerifyRequest compares one subject, a directory Root or a ZIP Archive (exactly one),
// with a caller-trusted expected document. The expected set is never derived from the
// subject; a document inside the subject is just a member.
type VerifyRequest struct {
	Root             string
	Archive          string
	ArchiveRoot      string   // "" or a portable member directory that maps to expected paths
	Nested           []string // explicitly selected nested ZIP members (paths below ArchiveRoot)
	Selection        Selection
	Expected         []byte // strict tsgk-expected/r1, required
	Profile          []byte // nil, or a strict tsgk-profile/r1 document
	Limits           Limits
	ArchiveLimits    ArchiveLimits // used only for an Archive subject
	Encoding         EncodingPolicy
	LargeFileProfile string
}

// ExpectedRef identifies the trusted expected record the comparison used.
type ExpectedRef struct {
	Provenance     string `json:"provenance"`
	SetSHA256      string `json:"set_sha256"`
	FileCount      uint64 `json:"file_count"`
	DocumentSHA256 string `json:"document_sha256"`
}

// Difference is one distinct mismatch; Expected or Actual is nil when that side is absent.
type Difference struct {
	Code     string        `json:"code"`
	Path     string        `json:"path"`
	Expected *FileIdentity `json:"expected"`
	Actual   *FileIdentity `json:"actual"`
}

// VerifyResult is a completed comparison: PASS means this exact selected set equals the
// expected record, not that a rights holder signed it or that the grammar is correct.
type VerifyResult struct {
	Report
	Policy          Policy       `json:"policy"`
	Subject         string       `json:"subject"`
	Scope           string       `json:"scope"`
	Grammar         string       `json:"grammar,omitempty"`
	ArchiveRoot     string       `json:"archive_root,omitempty"`
	Nested          []string     `json:"nested,omitempty"`
	Expected        ExpectedRef  `json:"expected"`
	Actual          Manifest     `json:"actual"`
	ActualSetSHA256 string       `json:"actual_set_sha256"`
	Differences     []Difference `json:"differences"`
	Excluded        uint64       `json:"excluded"`
}

// Verify computes the actual selected set independently and compares it with the
// expected record. A completed comparison returns a nil error with assessment PASS or
// FAIL; invalid input, unsupported archive features, limits, cancellation and I/O
// failures return a typed *Error and no comparison.
func Verify(ctx context.Context, req VerifyRequest) (VerifyResult, error) {
	v := &verification{req: req, policy: verifyPolicy(req.Limits, req.ArchiveLimits, req.Archive != "", req.LargeFileProfile, encodingPolicyName(req.Encoding), "")}
	res, e := v.run(ctx)
	if e == nil {
		e = sealOutput(res, v.req.Limits.OutputBytes)
	}
	if e != nil {
		out := VerifyResult{Report: newReport("verify"), Policy: v.policy, Subject: v.subject, Scope: v.scope, Nested: slices.Clone(req.Nested), ArchiveRoot: req.ArchiveRoot,
			Actual: Manifest{Schema: ManifestSchema, Algorithm: "sha256", ModePolicy: ModePolicy, EncodingPolicy: v.policy.EncodingPolicy, Files: []FileIdentity{}}, Differences: []Difference{}}
		failReport(&out.Report, e)
		return out, e
	}
	return res, nil
}

type verification struct {
	req            VerifyRequest
	policy         Policy
	subject, scope string
	prof           *profile
	exp            *expected
	findings       []Finding
	archiveSHA     string
	excluded       uint64
	coverage       Coverage
}

func verifyPolicy(l Limits, a ArchiveLimits, archive bool, large, enc, scope string) Policy {
	p := grammarPolicy(l, large, enc)
	p.Operation, p.Discovery = "offline-verify", scope
	if archive {
		p.ArchiveProfile, p.ArchiveEntries, p.ArchiveBytes, p.ArchiveDepth = ArchiveProfile, a.Entries, a.Bytes, a.Depth
	}
	return p
}

func (v *verification) run(ctx context.Context) (VerifyResult, *Error) {
	req := &v.req
	if (req.Root == "") == (req.Archive == "") {
		return VerifyResult{}, fail(KindInvalidInput, "SUBJECT_INVALID", "", nil)
	}
	v.subject = SubjectDirectory
	if req.Archive != "" {
		v.subject = SubjectArchive
		if !req.ArchiveLimits.valid() {
			return VerifyResult{}, fail(KindInvalidInput, "LIMITS_INVALID", "", nil)
		}
	}
	if !req.Limits.valid() {
		return VerifyResult{}, fail(KindInvalidInput, "LIMITS_INVALID", "", nil)
	}
	if req.Expected == nil {
		return VerifyResult{}, fail(KindInvalidInput, "EXPECTED_REQUIRED", "", nil)
	}
	var e *Error
	if v.exp, e = parseExpected(req.Expected); e != nil {
		return VerifyResult{}, e
	}
	ireq := IdentityRequest{Root: req.Root, Selection: req.Selection, Limits: req.Limits, Encoding: req.Encoding, LargeFileProfile: req.LargeFileProfile}
	var extra map[string]uint64
	if v.subject == SubjectArchive {
		a := req.ArchiveLimits
		extra = map[string]uint64{"archive_entries": a.Entries, "archive_bytes": a.Bytes, "archive_depth": a.Depth}
	}
	if req.Profile != nil {
		if v.prof, e = parseProfile(req.Profile); e != nil {
			return VerifyResult{}, e
		}
		if e := v.prof.applyTo(&ireq, extra); e != nil {
			return VerifyResult{}, e
		}
		if extra != nil {
			req.ArchiveLimits = ArchiveLimits{Entries: extra["archive_entries"], Bytes: extra["archive_bytes"], Depth: extra["archive_depth"]}
		}
	}
	req.Limits, req.Encoding, req.Selection = ireq.Limits, ireq.Encoding, ireq.Selection
	v.scope = ScopeKnownPaths
	switch {
	case v.subject == SubjectArchive && req.Selection.Files == nil:
		v.scope = ScopeArchiveMembers
	case req.Selection.Files != nil:
		v.scope = ScopeListed
	}
	enc := encodingPolicyName(req.Encoding)
	v.policy = verifyPolicy(req.Limits, req.ArchiveLimits, v.subject == SubjectArchive, req.LargeFileProfile, enc, v.scope)
	if enc != v.exp.manifest.EncodingPolicy {
		return VerifyResult{}, fail(KindInvalidInput, "ENCODING_POLICY_MISMATCH", "expected#/manifest/encoding_policy", nil)
	}
	var actual Manifest
	grammar := ""
	if v.subject == SubjectDirectory {
		if req.ArchiveRoot != "" || req.Nested != nil {
			return VerifyResult{}, fail(KindInvalidInput, "ARCHIVE_OPTION_NOT_APPLICABLE", "", nil)
		}
		ires, e := bindIdentity(ctx, ireq, v.prof, true)
		if e != nil {
			return VerifyResult{}, e
		}
		actual, grammar, v.coverage = ires.Manifest, ires.Grammar, ires.Coverage
		v.findings = append(v.findings, ires.Findings...)
	} else {
		if actual, e = v.archive(ctx); e != nil {
			return VerifyResult{}, e
		}
	}
	res := VerifyResult{Report: newReport("verify"), Policy: v.policy, Subject: v.subject, Scope: v.scope, Grammar: grammar, ArchiveRoot: req.ArchiveRoot, Nested: slices.Clone(req.Nested),
		Expected: ExpectedRef{Provenance: v.exp.provenance, SetSHA256: v.exp.setSHA256, FileCount: uint64(len(v.exp.manifest.Files)), DocumentSHA256: v.exp.raw},
		Actual:   actual, ActualSetSHA256: setSHA256(actual), Excluded: v.excluded}
	res.Coverage = v.coverage
	res.Differences = compareSets(v.exp.manifest, actual, req.Selection.Files, v.optional())
	res.Assessment = AssessPass
	for _, d := range res.Differences {
		sev := "error"
		if d.Code == "MISSING_OPTIONAL" {
			sev = "info"
		} else {
			res.Assessment = AssessFail
		}
		res.Findings = append(res.Findings, Finding{Code: d.Code, Severity: sev, Path: d.Path, Message: differenceMessages[d.Code]})
	}
	res.Findings = append(res.Findings, v.findings...)
	res.Findings = append(res.Findings, Finding{Code: "EXPECTED_TRUST_CALLER_DECLARED", Severity: "info", Message: "일치는 이 선택 집합이 호출자가 제공한 expected 기록과 같다는 뜻이며 서명·권리자 인증·grammar 정확성의 증명이 아니다"})
	res.Identities = append(res.Identities,
		IdentityRef{Role: "expected-set", Schema: FileSetSchema, SHA256: v.exp.setSHA256},
		IdentityRef{Role: "source-set", Schema: FileSetSchema, SHA256: res.ActualSetSHA256},
		IdentityRef{Role: "expected-document", Schema: ExpectedSchema, SHA256: v.exp.raw})
	if v.prof != nil {
		res.Identities = append(res.Identities, v.prof.ref())
	}
	if v.archiveSHA != "" {
		res.Identities = append(res.Identities, IdentityRef{Role: "archive", Schema: ArchiveProfile, SHA256: v.archiveSHA})
	}
	res.Identities = append(res.Identities, v.policy.ref())
	return res, nil
}

func (v *verification) optional() map[string]bool {
	if v.prof == nil {
		return nil
	}
	return v.prof.optional
}

var differenceMessages = map[string]string{
	"MISSING_REQUIRED": "필수 파일이 실제 집합에 없다",
	"MISSING_OPTIONAL": "선택적(required=false) 파일이 없다; 미리 선언한 정책대로 실패로 보지 않는다",
	"UNEXPECTED_FILE":  "expected에 없는 파일이 선택 범위 안에 있다",
	"ROLE_CHANGED":     "역할이 expected와 다르다",
	"SIZE_CHANGED":     "크기가 expected와 다르다",
	"CONTENT_CHANGED":  "내용 SHA-256이 expected와 다르다",
	"MODE_CHANGED":     "mode 또는 mode 출처가 expected와 다르다",
	"ENCODING_CHANGED": "encoding 판정이 expected와 다르다",
}

// compareSets reports every distinct difference over the union of expected paths,
// actual paths and required listed paths, in path byte order.
func compareSets(exp, act Manifest, listed []FileSelection, optional map[string]bool) []Difference {
	want, got := map[string]*FileIdentity{}, map[string]*FileIdentity{}
	var paths []string
	for i := range exp.Files {
		want[exp.Files[i].Path] = &exp.Files[i]
		paths = append(paths, exp.Files[i].Path)
	}
	for i := range act.Files {
		got[act.Files[i].Path] = &act.Files[i]
		paths = append(paths, act.Files[i].Path)
	}
	required := map[string]bool{}
	for _, f := range listed {
		if !optional[f.Path] {
			required[f.Path] = true
			paths = append(paths, f.Path)
		}
	}
	slices.Sort(paths)
	out := []Difference{}
	for _, p := range slices.Compact(paths) {
		e, a := want[p], got[p]
		switch {
		case a == nil && optional[p]:
			out = append(out, Difference{Code: "MISSING_OPTIONAL", Path: p, Expected: e})
		case a == nil:
			out = append(out, Difference{Code: "MISSING_REQUIRED", Path: p, Expected: e})
		case e == nil:
			out = append(out, Difference{Code: "UNEXPECTED_FILE", Path: p, Actual: a})
		default:
			for _, c := range []struct {
				code string
				diff bool
			}{
				{"ROLE_CHANGED", e.Role != a.Role},
				{"SIZE_CHANGED", e.Size != a.Size},
				{"CONTENT_CHANGED", e.SHA256 != a.SHA256},
				{"MODE_CHANGED", e.Mode != a.Mode || e.ModeProvenance != a.ModeProvenance},
				{"ENCODING_CHANGED", e.Encoding != a.Encoding},
			} {
				if c.diff {
					out = append(out, Difference{Code: c.code, Path: p, Expected: e, Actual: a})
				}
			}
		}
	}
	return out
}

// member is one selected archive file after scope mapping.
type member struct {
	path string
	z    *zipArchive
	e    *zipEntry
}

// archive inspects the ZIP subject without extraction and returns the actual manifest of
// the selected members. Roles are not observable in an archive: a listed scope takes the
// profile role, the members scope takes the expected role (an unexpected member has none).
func (v *verification) archive(ctx context.Context) (Manifest, *Error) {
	req := v.req
	out := Manifest{Schema: ManifestSchema, Algorithm: "sha256", ModePolicy: ModePolicy, EncodingPolicy: encodingPolicyName(req.Encoding), Files: []FileIdentity{}}
	v.coverage = Coverage{Requested: []string{"archive-members"}, Observed: []string{}, Unsupported: []string{"member-role-observation"}}
	if req.Selection.Grammar != "" {
		return out, fail(KindInvalidInput, "GRAMMAR_NOT_APPLICABLE", "", nil)
	}
	if req.Selection.Files != nil {
		if e := checkSelection(req.Selection.Files); e != nil {
			return out, e
		}
	}
	if req.ArchiveRoot != "" && !portable(req.ArchiveRoot, false) {
		return out, fail(KindInvalidInput, "ARCHIVE_ROOT_INVALID", "", nil)
	}
	nested := map[string]bool{}
	for _, n := range req.Nested {
		if !portable(n, false) {
			return out, fail(KindInvalidInput, "NESTED_INVALID", n, nil)
		}
		if nested[n] {
			return out, fail(KindInvalidInput, "NESTED_DUPLICATE", n, nil)
		}
		nested[n] = true
	}
	large, ok := largeFileProfiles[req.LargeFileProfile]
	if req.LargeFileProfile != "" && !ok {
		return out, fail(KindInvalidInput, "LARGE_FILE_PROFILE_UNKNOWN", "", nil)
	}
	declared, e := declarations(req.Encoding)
	if e != nil {
		return out, e
	}
	r, e := startRun(ctx, req.Limits.Wall)
	if e != nil {
		return out, e
	}
	defer r.cancel()
	f, info, e := openArchive(req.Archive, req.ArchiveLimits.Bytes)
	if e != nil {
		return out, e
	}
	defer f.Close()
	size := uint64(info.Size())
	h := sha256.New()
	buf := make([]byte, readChunk)
	for off := int64(0); off < int64(size); {
		if e := r.check(); e != nil {
			return out, e
		}
		n, err := f.ReadAt(buf[:min(int64(len(buf)), int64(size)-off)], off)
		if err != nil && err != io.EOF || n == 0 {
			return out, fail(KindIO, "READ_FAILED", "", err)
		}
		h.Write(buf[:n])
		off += int64(n)
	}
	v.archiveSHA = hex.EncodeToString(h.Sum(nil))
	listed := map[string]string{}
	for _, s := range req.Selection.Files {
		listed[s.Path] = s.Role
	}
	var members []member
	seen := uint64(0)
	var walk func(z *zipArchive, prefix string, level uint64) *Error
	walk = func(z *zipArchive, prefix string, level uint64) *Error {
		for _, ent := range z.entries {
			if ent.dir {
				continue
			}
			rel := ent.path
			switch {
			case prefix != "":
				rel = prefix + "/" + ent.path
			case req.ArchiveRoot != "":
				if !strings.HasPrefix(ent.path, req.ArchiveRoot+"/") {
					v.excluded++
					continue
				}
				rel = strings.TrimPrefix(ent.path, req.ArchiveRoot+"/")
			}
			if nested[rel] {
				delete(nested, rel)
				if level+1 > req.ArchiveLimits.Depth {
					return fail(KindResourceLimit, "ARCHIVE_DEPTH_LIMIT", rel, nil)
				}
				st, e := z.readMember(r, ent, rel, req.Limits.FileBytes, req.Limits.TotalBytes, req.Limits.FileBytes)
				if e != nil {
					return e
				}
				if depthOf(rel) >= req.Limits.Depth {
					return fail(KindResourceLimit, "DEPTH_LIMIT", rel, nil)
				}
				nz, e := openZip(r, bytes.NewReader(st.content), st.size, req.ArchiveLimits, &seen, req.Limits.Depth-depthOf(rel), rel)
				if e != nil {
					return e
				}
				if e := walk(nz, rel, level+1); e != nil {
					return e
				}
				continue
			}
			if req.Selection.Files != nil && listed[rel] == "" {
				v.excluded++
				continue
			}
			members = append(members, member{path: rel, z: z, e: ent})
		}
		return nil
	}
	z, e := openZip(r, f, size, req.ArchiveLimits, &seen, req.Limits.Depth, "")
	if e != nil {
		return out, e
	}
	if e := walk(z, "", 1); e != nil {
		return out, e
	}
	if left := slices.Sorted(maps.Keys(nested)); len(left) > 0 {
		return out, fail(KindInvalidInput, "NESTED_MEMBER_NOT_FOUND", left[0], nil)
	}
	if uint64(len(members)) > req.Limits.Files {
		return out, fail(KindResourceLimit, "FILE_COUNT_LIMIT", "", nil)
	}
	slices.SortFunc(members, func(a, b member) int { return strings.Compare(a.path, b.path) })
	present := map[string]bool{}
	for _, m := range members {
		present[m.path] = true
	}
	for _, p := range slices.Sorted(maps.Keys(declared)) {
		if !present[p] {
			return out, fail(KindInvalidInput, "DECLARATION_UNMATCHED", p, nil)
		}
	}
	for _, s := range req.Selection.Files {
		if !present[s.Path] && v.optional()[s.Path] {
			v.findings = append(v.findings, Finding{Code: "OPTIONAL_FILE_ABSENT", Severity: "info", Path: s.Path, Message: "profile이 선택적(required=false)으로 선언한 파일이 없다"})
		}
	}
	expRole := map[string]string{}
	for _, f := range v.exp.manifest.Files {
		expRole[f.Path] = f.Role
	}
	note := func(code, sev, p, msg string) {
		v.findings = append(v.findings, Finding{Code: code, Severity: sev, Path: p, Message: msg})
	}
	for _, m := range members {
		st, e := m.z.readMember(r, m.e, m.path, hardMax(req.Limits.FileBytes, large, ok), req.Limits.TotalBytes, 0)
		if e != nil {
			return out, e
		}
		if e := admitSize(note, m.path, st, req.Limits.FileBytes, req.LargeFileProfile, large, ok); e != nil {
			return out, e
		}
		outcome := st.encoding.result(req.Encoding.Profile == "cp949", declared[m.path])
		if outcome.Assessment != "PASS" {
			note(outcome.Code, "warning", m.path, "encoding 판정 "+outcome.Assessment+"; parse 입력을 만들지 않는다")
		}
		role := expRole[m.path]
		if req.Selection.Files != nil {
			role = listed[m.path]
		}
		out.Files = append(out.Files, FileIdentity{Path: m.path, Role: role, Mode: "100644", ModeProvenance: "POLICY_DEFAULT", Size: st.size, SHA256: st.sha256, Encoding: outcome})
	}
	if len(out.Files) > 0 {
		v.coverage.Observed = append(v.coverage.Observed, "archive-members")
	}
	after, err := f.Stat()
	if err != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return out, fail(KindIO, "SOURCE_CHANGED", "", err)
	}
	return out, r.check()
}

// openArchive opens the caller's explicit archive file. Like a root, the path itself may
// be a deliberate alias; it must resolve to a local regular file within the byte bound.
func openArchive(p string, maxBytes uint64) (*os.File, os.FileInfo, *Error) {
	if p == "" || strings.ContainsRune(p, 0) {
		return nil, nil, fail(KindInvalidInput, "ARCHIVE_INVALID", "", nil)
	}
	if notLocal(p) {
		return nil, nil, fail(KindInvalidInput, "ARCHIVE_NOT_LOCAL", "", nil)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, nil, fail(KindInvalidInput, "ARCHIVE_INVALID", "", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, nil, fail(KindInvalidInput, "ARCHIVE_NOT_FOUND", "", err)
	}
	if notLocal(resolved) {
		return nil, nil, fail(KindInvalidInput, "ARCHIVE_NOT_LOCAL", "", nil)
	}
	before, err := os.Stat(resolved)
	if err != nil {
		return nil, nil, fail(KindIO, "ARCHIVE_UNREADABLE", "", err)
	}
	if !before.Mode().IsRegular() {
		return nil, nil, fail(KindInvalidInput, "ARCHIVE_NOT_FILE", "", nil)
	}
	if uint64(before.Size()) > maxBytes {
		return nil, nil, fail(KindResourceLimit, "ARCHIVE_BYTES_LIMIT", "", nil)
	}
	f, err := os.Open(resolved)
	if err != nil {
		return nil, nil, fail(KindIO, "ARCHIVE_UNREADABLE", "", err)
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) || opened.Size() != before.Size() || !opened.ModTime().Equal(before.ModTime()) {
		f.Close()
		return nil, nil, fail(KindIO, "SOURCE_CHANGED", "", err)
	}
	return f, opened, nil
}
