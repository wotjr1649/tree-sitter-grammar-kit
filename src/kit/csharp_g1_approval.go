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
//
// The second bundle (G1-3b: the B03 and B04 assets and their post-seal corrections, the executed assets and ledger r2) is a second object of the same schema with its
// own scope. It names the first object (base) and no asset of the first bundle; the first object stays valid and unchanged, and the
// first bundle knows the second one only by the paths of its assets, so that they may exist beside it.

const (
	CSharpG1ApprovalSchema = "tsgk-csharp-g1-approval/r1"
	CSharpG1ApprovalScope  = "csharp-g1-bundle"
	// CSharpG1ApprovalDecision is the user decision (#233, 2026-11-13) that approves the bundle.
	CSharpG1ApprovalDecision = "https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/233#issuecomment-6851518154"

	// CSharpG1ApprovalScopeB34 is the scope of the second bundle. CSharpG1ApprovalDecisionB34 is the user decision (#233, 2026-11-14) that
	// approves it.
	CSharpG1ApprovalScopeB34    = "csharp-g1-b34-bundle"
	CSharpG1ApprovalDecisionB34 = "https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/233#issuecomment-6873534481"

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
	Base     string                   `json:"base,omitzero"` // second bundle only: the name (SHA-256) of the first approval object
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

// csharpG1ApprovalBundleB34 is the second bundle (#233, G1-3b): the sealed B03 and B04 expectations (r1) with the kind map r3 they use, the two
// audits made before the seal, the native comparison of the sealed asset (executed r1), the post-seal corrections (expected r2), the type audit
// r2 that audits per alternative, the native comparison of the corrected asset (executed r2) and ledger r2. The executed paths are literals for
// the reason given above.
var csharpG1ApprovalBundleB34 = []CSharpG1ApprovalMember{
	{Path: CSharpG1B34Path, Role: "expected", Revision: "r1"},
	{Path: CSharpG1KindMapPathR3, Role: "kind-map", Revision: "r3"},
	{Path: CSharpG1AuditB34Path, Role: "position-audit", Revision: "r1"},
	{Path: CSharpG1TypeAuditB34Path, Role: "type-audit", Revision: "r1"},
	{Path: "src/contracts/csharp-g1-executed-b03b04-r1.json", Role: "executed", Revision: "r1"},
	{Path: CSharpG1B34PathR2, Role: "expected", Revision: "r2"},
	{Path: CSharpG1TypeAuditB34PathR2, Role: "type-audit", Revision: "r2"},
	{Path: "src/contracts/csharp-g1-executed-b03b04-r2.json", Role: "executed", Revision: "r2"},
	{Path: CSharpG1LedgerPathR2, Role: "ledger", Revision: "r2"},
}

// csharpG1Bundle is one approvable bundle: its scope, its decision and its members in order.
type csharpG1Bundle struct {
	scope, decision string
	members         []CSharpG1ApprovalMember
}

var (
	csharpG1Bundle1   = csharpG1Bundle{CSharpG1ApprovalScope, CSharpG1ApprovalDecision, csharpG1ApprovalBundle}
	csharpG1Bundle2   = csharpG1Bundle{CSharpG1ApprovalScopeB34, CSharpG1ApprovalDecisionB34, csharpG1ApprovalBundleB34}
	csharpG1AllBundle = []csharpG1Bundle{csharpG1Bundle1, csharpG1Bundle2}
)

// CSharpG1PendingAssets lists the assets of the second bundle that exist in root while that bundle is not approved: tracked G1 assets that
// no approval object covers. They may exist beside the approved first bundle; the approval of the second bundle makes this list empty.
func CSharpG1PendingAssets(root fs.FS) []string {
	if state, _, err := VerifyCSharpG1ApprovalB34(root); err == nil && state == CSharpG1ApprovalApproved {
		return nil
	}
	var out []string
	for _, m := range csharpG1Bundle2.members {
		if _, err := fs.Stat(root, m.Path); err == nil {
			out = append(out, m.Path)
		}
	}
	return out
}

func csharpG1ApprovalSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func csharpG1ApprovalEncode(t CSharpG1ApprovalTuple) ([]byte, error) {
	b, err := jsonv2.Marshal(t)
	return append(b, '\n'), err
}

func csharpG1BuildBundle(root fs.FS, b csharpG1Bundle, base string) (data []byte, sum string, err error) {
	t := CSharpG1ApprovalTuple{Schema: CSharpG1ApprovalSchema, Scope: b.scope, Decision: b.decision, Base: base}
	for _, m := range b.members {
		raw, err := fs.ReadFile(root, m.Path)
		if err != nil {
			return nil, "", fmt.Errorf("G1_APPROVAL_MEMBER: %w", err)
		}
		m.Bytes, m.SHA256 = len(raw), csharpG1ApprovalSum(raw)
		t.Members = append(t.Members, m)
	}
	if data, err = csharpG1ApprovalEncode(t); err != nil {
		return nil, "", err
	}
	return data, csharpG1ApprovalSum(data), nil
}

// BuildCSharpG1Approval returns the canonical tuple bytes and their SHA-256 for the G1 assets of root (the repository root).
func BuildCSharpG1Approval(root fs.FS) (data []byte, sum string, err error) {
	return csharpG1BuildBundle(root, csharpG1Bundle1, "")
}

// BuildCSharpG1ApprovalB34 is BuildCSharpG1Approval for the second bundle. The first bundle must be approved: its object is the base.
func BuildCSharpG1ApprovalB34(root fs.FS) (data []byte, sum string, err error) {
	state, base, err := VerifyCSharpG1Approval(root)
	if err != nil {
		return nil, "", err
	}
	if state != CSharpG1ApprovalApproved {
		return nil, "", fmt.Errorf("G1_APPROVAL_BASE: the first bundle is %s, not approved", state)
	}
	return csharpG1BuildBundle(root, csharpG1Bundle2, base)
}

// csharpG1ApprovalFound is an approval object of the G1 schema.
type csharpG1ApprovalFound struct {
	name string
	raw  []byte
}

// csharpG1ApprovalScan returns the approval objects of the G1 schema by scope. A scope no bundle has is an error: an object of the schema
// is not ignored.
func csharpG1ApprovalScan(root fs.FS) (map[string][]csharpG1ApprovalFound, error) {
	entries, err := fs.ReadDir(root, csharpG1ApprovalObjects)
	if err != nil {
		return nil, err
	}
	out := map[string][]csharpG1ApprovalFound{}
	for _, e := range entries {
		if info, err := e.Info(); err != nil || !info.Mode().IsRegular() || info.Size() > csharpG1ApprovalMax {
			continue
		}
		b, err := fs.ReadFile(root, path.Join(csharpG1ApprovalObjects, e.Name()))
		if err != nil {
			return nil, err
		}
		var head struct {
			Schema string `json:"schema"`
			Scope  string `json:"scope"`
		}
		if jsonv2.Unmarshal(b, &head) != nil || head.Schema != CSharpG1ApprovalSchema {
			continue
		}
		if !slices.ContainsFunc(csharpG1AllBundle, func(x csharpG1Bundle) bool { return x.scope == head.Scope }) {
			return nil, fmt.Errorf("G1_APPROVAL_SCOPE: %q, want %q or %q", head.Scope, CSharpG1ApprovalScope, CSharpG1ApprovalScopeB34)
		}
		out[head.Scope] = append(out[head.Scope], csharpG1ApprovalFound{e.Name(), b})
	}
	return out, nil
}

// csharpG1VerifyBundle compares the assets of root with the object of one bundle. base is the name of the object the tuple must name as
// its base ("" for the first bundle, which names none). It returns CSharpG1ApprovalPending with a nil error when the bundle has no object.
func csharpG1VerifyBundle(root fs.FS, b csharpG1Bundle, base string) (state, object string, err error) {
	scan, err := csharpG1ApprovalScan(root)
	if err != nil {
		return "", "", err
	}
	found := scan[b.scope]
	switch len(found) {
	case 0:
		return CSharpG1ApprovalPending, "", nil
	case 1:
	default:
		names := make([]string, len(found))
		for i, f := range found {
			names[i] = f.name
		}
		return "", "", fmt.Errorf("G1_APPROVAL_AMBIGUOUS: %d approval objects %v", len(found), names)
	}
	object, raw := found[0].name, found[0].raw
	if csharpG1ApprovalSum(raw) != object {
		return "", object, fmt.Errorf("G1_APPROVAL_OBJECT: name %s is not the SHA-256 of its bytes", object)
	}
	var t CSharpG1ApprovalTuple
	if err = jsonv2.Unmarshal(raw, &t, jsonv2.RejectUnknownMembers(true)); err != nil {
		return "", object, fmt.Errorf("G1_APPROVAL_JSON: %w", err)
	}
	if t.Scope != b.scope {
		return "", object, fmt.Errorf("G1_APPROVAL_SCOPE: %q, want %q", t.Scope, b.scope)
	}
	if canon, err := csharpG1ApprovalEncode(t); err != nil || !bytes.Equal(canon, raw) {
		return "", object, fmt.Errorf("G1_APPROVAL_OBJECT: bytes are not the canonical encoding")
	}
	if t.Decision != b.decision {
		return "", object, fmt.Errorf("G1_APPROVAL_DECISION: %q", t.Decision)
	}
	if t.Base != base {
		return "", object, fmt.Errorf("G1_APPROVAL_BASE: %q, want %q", t.Base, base)
	}
	if len(t.Members) != len(b.members) {
		return "", object, fmt.Errorf("G1_APPROVAL_BUNDLE: %d members, want %d", len(t.Members), len(b.members))
	}
	for i, m := range t.Members {
		want := b.members[i]
		if m.Path != want.Path || m.Role != want.Role || m.Revision != want.Revision {
			return "", object, fmt.Errorf("G1_APPROVAL_BUNDLE: member %d is %s %s %s, want %s %s %s", i, m.Path, m.Role, m.Revision, want.Path, want.Role, want.Revision)
		}
		data, err := fs.ReadFile(root, m.Path)
		if err != nil {
			return "", object, fmt.Errorf("G1_APPROVAL_MEMBER: %w", err)
		}
		if len(data) != m.Bytes || csharpG1ApprovalSum(data) != m.SHA256 {
			return "", object, fmt.Errorf("G1_APPROVAL_MEMBER: %s is %d bytes, sha256 %s; approved %d bytes, sha256 %s", m.Path, len(data), csharpG1ApprovalSum(data), m.Bytes, m.SHA256)
		}
	}
	all, err := fs.Glob(root, csharpG1ApprovalGlob)
	if err != nil {
		return "", object, err
	}
	for _, p := range all {
		if !slices.ContainsFunc(csharpG1AllBundle, func(x csharpG1Bundle) bool {
			return slices.ContainsFunc(x.members, func(m CSharpG1ApprovalMember) bool { return m.Path == p })
		}) {
			return "", object, fmt.Errorf("G1_APPROVAL_UNAPPROVED: %s is a G1 asset outside the approved bundle", p)
		}
	}
	return CSharpG1ApprovalApproved, object, nil
}

// VerifyCSharpG1Approval compares the G1 assets of root with the approval object. It returns CSharpG1ApprovalPending
// with a nil error when no approval object exists (nothing is approved, nothing is checked), and CSharpG1ApprovalApproved
// only when one object names exactly the bundle and every listed file equals it byte for byte and no other G1 asset exists.
// The assets of the second bundle may exist beside it: they are not approved by this object and CSharpG1PendingAssets lists them.
func VerifyCSharpG1Approval(root fs.FS) (state, object string, err error) {
	return csharpG1VerifyBundle(root, csharpG1Bundle1, "")
}

// VerifyCSharpG1ApprovalB34 does the same for the second bundle. Its object must name the object of the first bundle as its base, and the
// first bundle must itself be APPROVED: a second bundle cannot stand on a first one that is pending or has changed. It lists no asset
// of the first bundle, so it cannot approve a different version of one.
func VerifyCSharpG1ApprovalB34(root fs.FS) (state, object string, err error) {
	state, base, err := VerifyCSharpG1Approval(root)
	if err != nil {
		return "", "", err
	}
	scan, err := csharpG1ApprovalScan(root)
	if err != nil {
		return "", "", err
	}
	if len(scan[CSharpG1ApprovalScopeB34]) == 0 {
		return CSharpG1ApprovalPending, "", nil
	}
	if state != CSharpG1ApprovalApproved {
		return "", "", fmt.Errorf("G1_APPROVAL_BASE: the first bundle is %s, so the second cannot be approved", state)
	}
	return csharpG1VerifyBundle(root, csharpG1Bundle2, base)
}
