package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"strings"
)

// Strict document schemas accepted by this build and the fixed size bound of every
// profile or expected document, independent of the subject's limits.
const (
	ProfileSchema    = "tsgk-profile/r1"
	ExpectedSchema   = "tsgk-expected/r1"
	MaxDocumentBytes = 16777216
)

// profile is a decoded tsgk-profile/r1. It selects paths, roles, requiredness, encoding
// declarations and narrower limits; it never grants effects, runs hooks or raises limits.
type profile struct {
	raw      string // sha256 of the original bytes
	files    []FileSelection
	optional map[string]bool
	limits   map[string]uint64
	encoding *EncodingPolicy
}

func parseProfile(data []byte) (*profile, *Error) {
	t := typed{doc: "profile"}
	v, e := decodeStrict(t.doc, data, MaxDocumentBytes)
	if e != nil {
		return nil, e
	}
	m, e := t.object(v, []string{"schema", "id"}, "files", "limits", "encoding")
	if e != nil {
		return nil, e
	}
	if s, e := t.str(m["schema"]); e != nil {
		return nil, e
	} else if s != ProfileSchema {
		return nil, t.bad("SCHEMA_UNSUPPORTED", m["schema"])
	}
	if id, e := t.str(m["id"]); e != nil {
		return nil, e
	} else if !validID(id) {
		return nil, t.bad("PROFILE_ID_INVALID", m["id"])
	}
	sum := sha256.Sum256(data)
	p := &profile{raw: hex.EncodeToString(sum[:]), optional: map[string]bool{}, limits: map[string]uint64{}}
	if v := m["files"]; v != nil {
		list, e := t.array(v)
		if e != nil {
			return nil, e
		}
		if len(list) == 0 {
			return nil, t.bad("EMPTY_SELECTION", v)
		}
		paths := make([]*jv, 0, len(list))
		for _, item := range list {
			f, e := t.object(item, []string{"path", "role", "required"})
			if e != nil {
				return nil, e
			}
			path, e := t.str(f["path"])
			if e != nil {
				return nil, e
			}
			if !portable(path, false) {
				return nil, t.bad("SELECTION_INVALID", f["path"])
			}
			role, e := t.str(f["role"])
			if e != nil {
				return nil, e
			}
			if !roles[role] {
				return nil, t.bad("ROLE_INVALID", f["role"])
			}
			required, e := t.boolean(f["required"])
			if e != nil {
				return nil, e
			}
			p.files = append(p.files, FileSelection{Path: path, Role: role})
			if !required {
				p.optional[path] = true
			}
			paths = append(paths, f["path"])
		}
		if e := sortedUnique(t, "SELECTION", paths); e != nil {
			return nil, e
		}
	}
	if v := m["limits"]; v != nil {
		l, e := t.object(v, nil, "files", "records", "file_bytes", "total_bytes", "depth", "output_bytes", "archive_entries", "archive_bytes", "archive_depth")
		if e != nil {
			return nil, e
		}
		for _, k := range v.keys { // document order keeps the reported error deterministic
			x := l[k]
			n, e := t.uint(x)
			if e != nil {
				return nil, e
			}
			if n == 0 {
				return nil, t.bad("PROFILE_LIMIT_INVALID", x)
			}
			p.limits[k] = n
		}
	}
	if v := m["encoding"]; v != nil {
		enc, e := t.object(v, nil, "profile", "files")
		if e != nil {
			return nil, e
		}
		p.encoding = &EncodingPolicy{}
		if x := enc["profile"]; x != nil {
			s, e := t.str(x)
			if e != nil {
				return nil, e
			}
			if s != "cp949" {
				return nil, t.bad("ENCODING_PROFILE_INVALID", x)
			}
			p.encoding.Profile = s
		}
		if x := enc["files"]; x != nil {
			list, e := t.array(x)
			if e != nil {
				return nil, e
			}
			if len(list) == 0 {
				return nil, t.bad("ENCODING_DECLARATION_INVALID", x)
			}
			seen := map[string]bool{}
			for _, item := range list {
				d, e := t.object(item, []string{"path", "encoding"})
				if e != nil {
					return nil, e
				}
				path, e := t.str(d["path"])
				if e != nil {
					return nil, e
				}
				name, e := t.str(d["encoding"])
				if e != nil {
					return nil, e
				}
				if !portable(path, false) || (name != "utf-8" && name != "cp949") {
					return nil, t.bad("ENCODING_DECLARATION_INVALID", item)
				}
				if seen[path] {
					return nil, t.bad("ENCODING_DECLARATION_DUPLICATE", item)
				}
				seen[path] = true
				p.encoding.Files = append(p.encoding.Files, FileEncoding{Path: path, Encoding: name})
			}
		}
	}
	return p, nil
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// sortedUnique requires byte-ascending paths without exact or ASCII case-fold duplicates.
func sortedUnique(t typed, prefix string, paths []*jv) *Error {
	folded := map[string]bool{}
	for i, v := range paths {
		if i > 0 {
			switch c := strings.Compare(paths[i-1].s, v.s); {
			case c == 0:
				return t.bad(prefix+"_DUPLICATE", v)
			case c > 0:
				return t.bad(prefix+"_UNSORTED", v)
			}
		}
		if folded[strings.ToLower(v.s)] {
			return t.bad(prefix+"_CASE_COLLISION", v)
		}
		folded[strings.ToLower(v.s)] = true
	}
	return nil
}

// narrow applies profile limits under the operation bounds: every key must apply to the
// operation and may only lower its bound. It returns the effective bounds.
func (p *profile) narrow(bounds map[string]uint64) (map[string]uint64, *Error) {
	out := maps.Clone(bounds)
	for _, k := range slices.Sorted(maps.Keys(p.limits)) {
		v := p.limits[k]
		bound, ok := bounds[k]
		if !ok {
			return nil, fail(KindInvalidInput, "PROFILE_LIMIT_NOT_APPLICABLE", "profile#/limits/"+k, nil)
		}
		if v > bound {
			return nil, fail(KindInvalidInput, "PROFILE_LIMIT_ABOVE_OPERATION", "profile#/limits/"+k, nil)
		}
		out[k] = v
	}
	return out, nil
}

// encodingFrom picks the single encoding source: the request or the profile, not both.
func (p *profile) encodingFrom(req EncodingPolicy) (EncodingPolicy, *Error) {
	if p == nil || p.encoding == nil {
		return req, nil
	}
	if req.Profile != "" || req.Files != nil {
		return req, fail(KindInvalidInput, "ENCODING_SOURCE_CONFLICT", "profile#/encoding", nil)
	}
	return *p.encoding, nil
}

func (p *profile) ref() IdentityRef {
	return IdentityRef{Role: "profile", Schema: ProfileSchema, SHA256: p.raw}
}

// expected is a decoded, self-consistent tsgk-expected/r1: the caller-trusted record of
// one selected file set. Its trust comes from the caller's provenance, never from the
// subject being verified.
type expected struct {
	raw        string
	provenance string
	setSHA256  string
	manifest   Manifest
}

var (
	assessments     = map[string]bool{"PASS": true, "BLOCKED": true, "UNRESOLVED": true}
	encodingNames   = map[string]bool{EncodingUTF8: true, EncodingUTF16LE: true, EncodingUTF16BE: true, EncodingCP949: true}
	encodingSources = map[string]bool{SourceBOM: true, SourceValidation: true, SourceDeclaration: true}
	encodingCodes   = map[string]bool{"UTF32_BOM": true, "BOM_CONTENT_INVALID": true, "UTF16_ODD_LENGTH": true, "UTF16_UNPAIRED_SURROGATE": true, "UTF16_NUL": true,
		"NUL_WITHOUT_BOM": true, "DECLARED_ENCODING_INVALID": true, "UNDETERMINED_ENCODING": true, "ENCODING_TABLE_REQUIRED": true}
)

func parseExpected(data []byte) (*expected, *Error) {
	t := typed{doc: "expected"}
	v, e := decodeStrict(t.doc, data, MaxDocumentBytes)
	if e != nil {
		return nil, e
	}
	m, e := t.object(v, []string{"schema", "provenance", "file_count", "set_sha256", "manifest"})
	if e != nil {
		return nil, e
	}
	if s, e := t.str(m["schema"]); e != nil {
		return nil, e
	} else if s != ExpectedSchema {
		return nil, t.bad("SCHEMA_UNSUPPORTED", m["schema"])
	}
	sum := sha256.Sum256(data)
	x := &expected{raw: hex.EncodeToString(sum[:])}
	if x.provenance, e = t.str(m["provenance"]); e != nil {
		return nil, e
	}
	if x.provenance == "" || len(x.provenance) > 1024 || strings.IndexFunc(x.provenance, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
		return nil, t.bad("PROVENANCE_INVALID", m["provenance"])
	}
	count, e := t.uint(m["file_count"])
	if e != nil {
		return nil, e
	}
	if x.setSHA256, e = t.str(m["set_sha256"]); e != nil {
		return nil, e
	}
	if !lowerHex64(x.setSHA256) {
		return nil, t.bad("SHA256_INVALID", m["set_sha256"])
	}
	mf, e := t.object(m["manifest"], []string{"schema", "algorithm", "mode_policy", "encoding_policy", "files"})
	if e != nil {
		return nil, e
	}
	fixed := map[string]string{"schema": ManifestSchema, "algorithm": "sha256", "mode_policy": ModePolicy}
	for k, want := range fixed {
		if s, e := t.str(mf[k]); e != nil {
			return nil, e
		} else if s != want {
			return nil, t.bad("MANIFEST_VALUE_UNSUPPORTED", mf[k])
		}
	}
	x.manifest = Manifest{Schema: ManifestSchema, Algorithm: "sha256", ModePolicy: ModePolicy, Files: []FileIdentity{}}
	if x.manifest.EncodingPolicy, e = t.str(mf["encoding_policy"]); e != nil {
		return nil, e
	}
	if p := x.manifest.EncodingPolicy; p != "detect-r1" && p != "detect-r1;profile=cp949" {
		return nil, t.bad("MANIFEST_VALUE_UNSUPPORTED", mf["encoding_policy"])
	}
	list, e := t.array(mf["files"])
	if e != nil {
		return nil, e
	}
	if len(list) == 0 {
		return nil, t.bad("EMPTY_SELECTION", mf["files"])
	}
	paths := make([]*jv, 0, len(list))
	for _, item := range list {
		f, path, e := x.file(t, item)
		if e != nil {
			return nil, e
		}
		x.manifest.Files = append(x.manifest.Files, f)
		paths = append(paths, path)
	}
	if e := sortedUnique(t, "EXPECTED_PATH", paths); e != nil {
		return nil, e
	}
	if count != uint64(len(list)) {
		return nil, t.bad("EXPECTED_COUNT_MISMATCH", m["file_count"])
	}
	if setSHA256(x.manifest) != x.setSHA256 {
		return nil, t.bad("EXPECTED_SET_MISMATCH", m["set_sha256"])
	}
	return x, nil
}

func (x *expected) file(t typed, item *jv) (FileIdentity, *jv, *Error) {
	var out FileIdentity
	f, e := t.object(item, []string{"path", "role", "mode", "mode_provenance", "size", "sha256", "encoding"})
	if e != nil {
		return out, nil, e
	}
	if out.Path, e = t.str(f["path"]); e != nil {
		return out, nil, e
	}
	if code := pathProblem(out.Path); code != "" {
		kind := KindInvalidInput
		if code == "NOT_ASCII" {
			kind = KindUnsupported
		}
		return out, nil, fail(kind, "EXPECTED_PATH_"+code, t.doc+"#"+f["path"].ptr(), nil)
	}
	if out.Role, e = t.str(f["role"]); e != nil {
		return out, nil, e
	}
	if !roles[out.Role] {
		return out, nil, t.bad("ROLE_INVALID", f["role"])
	}
	if out.Mode, e = t.str(f["mode"]); e != nil {
		return out, nil, e
	}
	if out.ModeProvenance, e = t.str(f["mode_provenance"]); e != nil {
		return out, nil, e
	}
	if out.Mode != "100644" || out.ModeProvenance != "POLICY_DEFAULT" { // the only values portable-default produces
		return out, nil, t.bad("MODE_INVALID", item)
	}
	if out.Size, e = t.uint(f["size"]); e != nil {
		return out, nil, e
	}
	if out.SHA256, e = t.str(f["sha256"]); e != nil {
		return out, nil, e
	}
	if !lowerHex64(out.SHA256) {
		return out, nil, t.bad("SHA256_INVALID", f["sha256"])
	}
	enc, e := t.object(f["encoding"], []string{"assessment"}, "encoding", "source", "code")
	if e != nil {
		return out, nil, e
	}
	for _, k := range []struct {
		name  string
		dst   *string
		valid map[string]bool
	}{{"assessment", &out.Encoding.Assessment, assessments}, {"encoding", &out.Encoding.Encoding, encodingNames}, {"source", &out.Encoding.Source, encodingSources}, {"code", &out.Encoding.Code, encodingCodes}} {
		v := enc[k.name]
		if v == nil {
			continue
		}
		s, e := t.str(v)
		if e != nil {
			return out, nil, e
		}
		if !k.valid[s] { // "" is not a value: an absent field stays absent
			return out, nil, t.bad("ENCODING_OUTCOME_INVALID", v)
		}
		*k.dst = s
	}
	if (out.Encoding.Assessment == "PASS") != (out.Encoding.Code == "") {
		return out, nil, t.bad("ENCODING_OUTCOME_INVALID", f["encoding"])
	}
	return out, f["path"], nil
}

func lowerHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// pathProblem classifies a member or expected path under the verify name policy
// portable-names-r1: the portable path rules plus ASCII only, because Unicode
// normalization collisions cannot be detected without a normalization table. It returns
// "" for an accepted path, otherwise a stable code suffix.
func pathProblem(p string) string {
	switch {
	case p == "":
		return "EMPTY"
	case strings.ContainsRune(p, '\\'):
		return "BACKSLASH"
	case strings.HasPrefix(p, "/"):
		return "ABSOLUTE"
	case len(p) >= 2 && p[1] == ':' && (p[0]|0x20) >= 'a' && (p[0]|0x20) <= 'z':
		return "DRIVE"
	case strings.ContainsRune(p, ':'):
		return "ADS"
	}
	for i := 0; i < len(p); i++ {
		switch {
		case p[i] >= 0x80:
			return "NOT_ASCII"
		case p[i] < 32 || p[i] == 127:
			return "CONTROL"
		}
	}
	for _, seg := range strings.Split(p, "/") {
		switch {
		case seg == "":
			return "EMPTY_SEGMENT"
		case seg == "." || seg == "..":
			return "TRAVERSAL"
		case strings.TrimRight(seg, " .") != seg:
			return "TRAILING_DOT_SPACE"
		}
		if reservedDevice(seg) {
			return "DEVICE"
		}
	}
	if !portable(p, false) {
		return "INVALID"
	}
	return ""
}
