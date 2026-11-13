package kit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"slices"

	jsonv2 "encoding/json/v2"
)

// The approval tuple of the #233 G1 bundle: one content-addressed object under src/contracts/evidence-pins/objects that
// names every G1 asset of the user decision by repository path, role, revision, byte length and SHA-256. It is its own
// schema with its own scope: it does not extend the roslyn-pin tuple (scope legacy-observation) or the historical-materials
// approval, and it does not touch their objects. Until the object exists the state is PENDING_APPROVAL, which is not a pass.
// Nothing here writes: the object is written by hand after the user confirms the tuple that PrintCSharpG1Approval shows.

const (
	CSharpG1ApprovalSchema = "tsgk-csharp-g1-approval/r1"
	CSharpG1ApprovalScope  = "csharp-g1-bundle"
	// CSharpG1ApprovalDecision is the user decision (#233, 2026-11-13) that approves the bundle.
	CSharpG1ApprovalDecision = "https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/233#issuecomment-6851518154"

	CSharpG1ApprovalPending  = "PENDING_APPROVAL"
	CSharpG1ApprovalApproved = "APPROVED"

	csharpG1ApprovalObjects = "src/contracts/evidence-pins/objects"
	csharpG1ApprovalGlob    = "src/contracts/csharp-g1-*.json"
	csharpG1ApprovalMax     = 64 << 10 // a tuple is about 2 KiB; larger objects are materials, not tuples
)

// CSharpG1ApprovalMember is one asset of the bundle.
type CSharpG1ApprovalMember struct {
	Path     string `json:"path"`
	Role     string `json:"role"`
	Revision string `json:"revision"`
	Bytes    int    `json:"bytes"`
	SHA256   string `json:"sha256"`
}

// CSharpG1ApprovalTuple is the canonical object: compact JSON in this member order, one trailing newline.
type CSharpG1ApprovalTuple struct {
	Schema   string                   `json:"schema"`
	Scope    string                   `json:"scope"`
	Decision string                   `json:"decision"`
	Members  []CSharpG1ApprovalMember `json:"members"`
}

// csharpG1ApprovalBundle is the bundle of the decision. A G1 asset that is not listed here is unapproved. The two executed
// paths are literals: the constants live in a comparison file, which the separation proof keeps out of this package state
// (TestCSharpG1ApprovalBundleCoversEveryTrackedAsset checks them against the constants).
var csharpG1ApprovalBundle = []CSharpG1ApprovalMember{
	{Path: CSharpG1ExpectedPath, Role: "expected", Revision: "r1"},
	{Path: CSharpG1KindMapPath, Role: "kind-map", Revision: "r1"},
	{Path: "src/contracts/csharp-g1-executed-b01b02-r1.json", Role: "executed", Revision: "r1"},
	{Path: CSharpG1ExpectedPathR2, Role: "expected", Revision: "r2"},
	{Path: CSharpG1KindMapPathR2, Role: "kind-map", Revision: "r2"},
	{Path: "src/contracts/csharp-g1-executed-b01b02-r2.json", Role: "executed", Revision: "r2"},
	{Path: CSharpG1RoslynPath, Role: "roslyn-observation", Revision: "r2"},
	{Path: CSharpG1RoslynPathR3, Role: "roslyn-observation", Revision: "r3"},
	{Path: CSharpG1LedgerPath, Role: "ledger", Revision: "r1"},
	{Path: CSharpG1Audit76Path, Role: "position-audit", Revision: "r1"},
	{Path: CSharpG1TypeAudit76Path, Role: "type-audit", Revision: "r1"},
}

func csharpG1ApprovalSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func csharpG1ApprovalEncode(t CSharpG1ApprovalTuple) ([]byte, error) {
	b, err := jsonv2.Marshal(t)
	return append(b, '\n'), err
}

// BuildCSharpG1Approval returns the canonical tuple bytes and their SHA-256 for the G1 assets of root (the repository root).
func BuildCSharpG1Approval(root fs.FS) (data []byte, sum string, err error) {
	t := CSharpG1ApprovalTuple{Schema: CSharpG1ApprovalSchema, Scope: CSharpG1ApprovalScope, Decision: CSharpG1ApprovalDecision}
	for _, m := range csharpG1ApprovalBundle {
		b, err := fs.ReadFile(root, m.Path)
		if err != nil {
			return nil, "", fmt.Errorf("G1_APPROVAL_MEMBER: %w", err)
		}
		m.Bytes, m.SHA256 = len(b), csharpG1ApprovalSum(b)
		t.Members = append(t.Members, m)
	}
	if data, err = csharpG1ApprovalEncode(t); err != nil {
		return nil, "", err
	}
	return data, csharpG1ApprovalSum(data), nil
}

// VerifyCSharpG1Approval compares the G1 assets of root with the approval object. It returns CSharpG1ApprovalPending
// with a nil error when no approval object exists (nothing is approved, nothing is checked), and CSharpG1ApprovalApproved
// only when one object names exactly the bundle and every listed file equals it byte for byte and no other G1 asset exists.
func VerifyCSharpG1Approval(root fs.FS) (state, object string, err error) {
	entries, err := fs.ReadDir(root, csharpG1ApprovalObjects)
	if err != nil {
		return "", "", err
	}
	var found []string
	var raw []byte
	for _, e := range entries {
		if info, err := e.Info(); err != nil || !info.Mode().IsRegular() || info.Size() > csharpG1ApprovalMax {
			continue
		}
		b, err := fs.ReadFile(root, path.Join(csharpG1ApprovalObjects, e.Name()))
		if err != nil {
			return "", "", err
		}
		var head struct {
			Schema string `json:"schema"`
		}
		if jsonv2.Unmarshal(b, &head) == nil && head.Schema == CSharpG1ApprovalSchema {
			found, raw = append(found, e.Name()), b
		}
	}
	switch len(found) {
	case 0:
		return CSharpG1ApprovalPending, "", nil
	case 1:
	default:
		return "", "", fmt.Errorf("G1_APPROVAL_AMBIGUOUS: %d approval objects %v", len(found), found)
	}
	object = found[0]
	if csharpG1ApprovalSum(raw) != object {
		return "", object, fmt.Errorf("G1_APPROVAL_OBJECT: name %s is not the SHA-256 of its bytes", object)
	}
	var t CSharpG1ApprovalTuple
	if err = jsonv2.Unmarshal(raw, &t, jsonv2.RejectUnknownMembers(true)); err != nil {
		return "", object, fmt.Errorf("G1_APPROVAL_JSON: %w", err)
	}
	if t.Scope != CSharpG1ApprovalScope {
		return "", object, fmt.Errorf("G1_APPROVAL_SCOPE: %q, want %q", t.Scope, CSharpG1ApprovalScope)
	}
	if canon, err := csharpG1ApprovalEncode(t); err != nil || !bytes.Equal(canon, raw) {
		return "", object, fmt.Errorf("G1_APPROVAL_OBJECT: bytes are not the canonical encoding")
	}
	if t.Decision != CSharpG1ApprovalDecision {
		return "", object, fmt.Errorf("G1_APPROVAL_DECISION: %q", t.Decision)
	}
	if len(t.Members) != len(csharpG1ApprovalBundle) {
		return "", object, fmt.Errorf("G1_APPROVAL_BUNDLE: %d members, want %d", len(t.Members), len(csharpG1ApprovalBundle))
	}
	for i, m := range t.Members {
		want := csharpG1ApprovalBundle[i]
		if m.Path != want.Path || m.Role != want.Role || m.Revision != want.Revision {
			return "", object, fmt.Errorf("G1_APPROVAL_BUNDLE: member %d is %s %s %s, want %s %s %s", i, m.Path, m.Role, m.Revision, want.Path, want.Role, want.Revision)
		}
		b, err := fs.ReadFile(root, m.Path)
		if err != nil {
			return "", object, fmt.Errorf("G1_APPROVAL_MEMBER: %w", err)
		}
		if len(b) != m.Bytes || csharpG1ApprovalSum(b) != m.SHA256 {
			return "", object, fmt.Errorf("G1_APPROVAL_MEMBER: %s is %d bytes, sha256 %s; approved %d bytes, sha256 %s", m.Path, len(b), csharpG1ApprovalSum(b), m.Bytes, m.SHA256)
		}
	}
	all, err := fs.Glob(root, csharpG1ApprovalGlob)
	if err != nil {
		return "", object, err
	}
	for _, p := range all {
		if !slices.ContainsFunc(t.Members, func(m CSharpG1ApprovalMember) bool { return m.Path == p }) {
			return "", object, fmt.Errorf("G1_APPROVAL_UNAPPROVED: %s is a G1 asset outside the approved bundle", p)
		}
	}
	return CSharpG1ApprovalApproved, object, nil
}
