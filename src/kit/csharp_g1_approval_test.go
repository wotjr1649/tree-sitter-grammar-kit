package kit

import (
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	jsonv2 "encoding/json/v2"
)

// g1ApprovalFS is the tracked G1 assets plus an empty object directory; mutants edit a copy of the map.
func g1ApprovalFS(t *testing.T) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{csharpG1ApprovalObjects: {Mode: fs.ModeDir}}
	for _, m := range csharpG1ApprovalBundle {
		fsys[m.Path] = &fstest.MapFile{Data: mustRead(t, "../../"+m.Path)}
	}
	return fsys
}

// g1ApprovalPendingPinned are the assets of the second bundle, which are the assets pending while that bundle is not approved, by literal
// path, role, revision and SHA-256 (review of PR2, finding 1): the verifier's list is code, so a fifth asset, a removed one or a changed one
// needs a change here too. When the second tuple is approved the assets leave the pending list and stay pinned by the approval object; they
// must still be these bytes, because a new revision is a new asset and a new approval.
var g1ApprovalPendingPinned = []CSharpG1ApprovalMember{
	{Path: "src/contracts/csharp-g1-expected-b03b04-r1.json", Role: "expected", Revision: "r1", SHA256: "f37563567c1298900a41751bec3eff299e8c81bff25adc72352624c59d631bcc"},
	{Path: "src/contracts/csharp-g1-kind-map-r3.json", Role: "kind-map", Revision: "r3", SHA256: "ecba9e296c22baacf5eb5feabe6c5f25c802681700b5247c83e8cd1c3e87aadf"},
	{Path: "src/contracts/csharp-g1-audit-b03b04-r1.json", Role: "position-audit", Revision: "r1", SHA256: "d18fafc9b60de0bf7d48c05dc4087e6f807d2aafe6d47faebb79021d836008d0"},
	{Path: "src/contracts/csharp-g1-typeaudit-b03b04-r1.json", Role: "type-audit", Revision: "r1", SHA256: "d3a7b23b9f168547ee6722bdf98ec51f33784cf5c4c1e8169313f91eb92e5ceb"},
	{Path: "src/contracts/csharp-g1-executed-b03b04-r1.json", Role: "executed", Revision: "r1", SHA256: "281f4c49087e113478f3f8589cbc59275827eb71e7c7730cfaea44b37bf4d930"},
	{Path: "src/contracts/csharp-g1-ledger-r2.json", Role: "ledger", Revision: "r2", SHA256: "3a5a40e00793852235d262952571a17f3818bdffc0d53df49888ad4dc1f905c7"},
}

// g1ApprovalB34Paths are the paths of g1ApprovalPendingPinned.
func g1ApprovalB34Paths() []string {
	var out []string
	for _, m := range g1ApprovalPendingPinned {
		out = append(out, m.Path)
	}
	return out
}

// g1ApprovalFS2 is g1ApprovalFS with the assets of the second bundle too.
func g1ApprovalFS2(t *testing.T) fstest.MapFS {
	t.Helper()
	fsys := g1ApprovalClone(g1ApprovalFS(t))
	for _, p := range g1ApprovalB34Paths() {
		fsys[p] = &fstest.MapFile{Data: mustRead(t, "../../"+p)}
	}
	return fsys
}

func g1ApprovalClone(fsys fstest.MapFS) fstest.MapFS {
	c := fstest.MapFS{}
	for k, v := range fsys {
		c[k] = v
	}
	return c
}

// g1ApprovalWith stores data as an object named by its SHA-256.
func g1ApprovalWith(fsys fstest.MapFS, data []byte) string {
	name := csharpG1ApprovalSum(data)
	fsys[path.Join(csharpG1ApprovalObjects, name)] = &fstest.MapFile{Data: data}
	return name
}

func g1ApprovalTupleOf(t *testing.T, fsys fstest.MapFS) (CSharpG1ApprovalTuple, []byte) {
	t.Helper()
	data, _, err := BuildCSharpG1Approval(fsys)
	if err != nil {
		t.Fatal(err)
	}
	var x CSharpG1ApprovalTuple
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	return x, data
}

// Until the user confirms the tuple and the object is written, the tracked repository is PENDING_APPROVAL; with the object it
// must be APPROVED. Either way the state is logged, and pending is never an approval.
func TestCSharpG1ApprovalOfTheTrackedBundle(t *testing.T) {
	state, object, err := VerifyCSharpG1Approval(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	switch state {
	case CSharpG1ApprovalPending:
		t.Logf("%s: no tsgk-csharp-g1-approval/r1 object under %s; the G1 bundle is NOT approved", state, csharpG1ApprovalObjects)
	case CSharpG1ApprovalApproved:
		t.Logf("%s by object %s", state, object)
	default:
		t.Fatalf("unknown state %q", state)
	}
	// the first bundle APPROVED does not make the assets of the second one approved: the log says so (review of PR2, finding 2)
	state2, object2, err := VerifyCSharpG1ApprovalB34(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	pending := CSharpG1PendingAssets(os.DirFS("../.."))
	switch {
	case state2 == CSharpG1ApprovalApproved && len(pending) == 0:
		t.Logf("%s by object %s (scope %s)", state2, object2, CSharpG1ApprovalScopeB34)
	case state2 == CSharpG1ApprovalPending && slices.Equal(pending, g1ApprovalB34Paths()):
		t.Logf("%s: no object for scope %s; pending, NOT approved: %s", state2, CSharpG1ApprovalScopeB34, strings.Join(pending, " "))
	default:
		t.Errorf("second bundle %q with pending assets %v", state2, pending)
	}
}

// The pending assets are pinned by literal (review of PR2, finding 1): the second bundle's list equals the literal table, the tracked files
// are exactly these bytes, and the tracked G1 assets are the first bundle and these, no fifth one.
func TestCSharpG1ApprovalPendingAssetsArePinned(t *testing.T) {
	if len(csharpG1ApprovalBundleB34) != len(g1ApprovalPendingPinned) {
		t.Fatalf("the second bundle has %d members, the pinned table %d", len(csharpG1ApprovalBundleB34), len(g1ApprovalPendingPinned))
	}
	for i, want := range g1ApprovalPendingPinned {
		got := csharpG1ApprovalBundleB34[i]
		if got.Path != want.Path || got.Role != want.Role || got.Revision != want.Revision {
			t.Errorf("member %d is %s %s %s, pinned %s %s %s", i, got.Path, got.Role, got.Revision, want.Path, want.Role, want.Revision)
		}
		if sum := csharpG1ApprovalSum(mustRead(t, "../../"+want.Path)); sum != want.SHA256 {
			t.Errorf("%s is %s, pinned %s", want.Path, sum, want.SHA256)
		}
	}
	all, err := fs.Glob(os.DirFS("../.."), csharpG1ApprovalGlob)
	if err != nil || len(all) != len(csharpG1ApprovalBundle)+len(g1ApprovalPendingPinned) {
		t.Fatalf("%d tracked G1 assets (%v), want the %d of the first bundle and the %d pinned", len(all), err, len(csharpG1ApprovalBundle), len(g1ApprovalPendingPinned))
	}
	// the list the verifier reports is the pinned table while the second bundle has no object, and empty with it
	pending := CSharpG1PendingAssets(os.DirFS("../.."))
	state, _, err := VerifyCSharpG1ApprovalB34(os.DirFS("../.."))
	if err != nil || state == CSharpG1ApprovalApproved && len(pending) != 0 || state != CSharpG1ApprovalApproved && !slices.Equal(pending, g1ApprovalB34Paths()) {
		t.Errorf("second bundle %q (%v), pending %v", state, err, pending)
	}
	// a fifth G1 asset is neither pending nor approved: the first bundle fails on it
	fsys := g1ApprovalClone(g1ApprovalFS2(t))
	_, data := g1ApprovalTupleOf(t, fsys)
	g1ApprovalWith(fsys, data)
	fsys["src/contracts/csharp-g1-fifth-r1.json"] = &fstest.MapFile{Data: []byte("{}\n")}
	if slices.Contains(CSharpG1PendingAssets(fsys), "src/contracts/csharp-g1-fifth-r1.json") {
		t.Error("a fifth asset is listed as pending")
	}
	if _, _, err := VerifyCSharpG1Approval(fsys); err == nil || !strings.Contains(err.Error(), "G1_APPROVAL_UNAPPROVED") {
		t.Errorf("a fifth asset: %v", err)
	}
}

func TestCSharpG1ApprovalPendingIsNotApproved(t *testing.T) {
	state, object, err := VerifyCSharpG1Approval(g1ApprovalFS(t))
	if err != nil || state != CSharpG1ApprovalPending || object != "" || state == CSharpG1ApprovalApproved {
		t.Fatalf("no object: %q %q %v", state, object, err)
	}
	// an object of another schema (the legacy-observation tuple) is not an approval
	fsys := g1ApprovalClone(g1ApprovalFS(t))
	g1ApprovalWith(fsys, []byte(`{"schema":"tsgk-roslyn-pin-tuple/r1","scope":"legacy-observation"}`+"\n"))
	if state, _, err := VerifyCSharpG1Approval(fsys); err != nil || state != CSharpG1ApprovalPending {
		t.Fatalf("foreign tuple: %q %v", state, err)
	}
}

func TestCSharpG1ApprovalBundleCoversEveryTrackedAsset(t *testing.T) {
	all, err := fs.Glob(os.DirFS("../.."), csharpG1ApprovalGlob)
	// every tracked G1 asset is in exactly one of the two bundles; the second bundle's are pending until its object exists
	pending := CSharpG1PendingAssets(os.DirFS("../.."))
	if state, _, err := VerifyCSharpG1ApprovalB34(os.DirFS("../..")); err != nil || state == CSharpG1ApprovalApproved && len(pending) != 0 || state != CSharpG1ApprovalApproved && len(pending) != len(csharpG1ApprovalBundleB34) {
		t.Fatalf("second bundle %q (%v) with %d pending assets", state, err, len(pending))
	}
	if err != nil || len(all) != len(csharpG1ApprovalBundle)+len(csharpG1ApprovalBundleB34) || len(csharpG1ApprovalBundle) != 11 || len(csharpG1ApprovalBundleB34) != 6 {
		t.Fatalf("%d tracked G1 assets (%v), %d in the first bundle, %d in the second", len(all), err, len(csharpG1ApprovalBundle), len(csharpG1ApprovalBundleB34))
	}
	for _, p := range g1ApprovalB34Paths() {
		if slices.ContainsFunc(csharpG1ApprovalBundle, func(m CSharpG1ApprovalMember) bool { return m.Path == p }) {
			t.Errorf("%s is in both bundles", p)
		}
		if !slices.Contains(all, p) {
			t.Errorf("%s is not a tracked G1 asset", p)
		}
	}
	if csharpG1ApprovalBundle[2].Path != CSharpG1ExecutedPath || csharpG1ApprovalBundle[5].Path != CSharpG1ExecutedPathR2 || csharpG1ApprovalBundleB34[4].Path != CSharpG1B34ExecutedPath || csharpG1ApprovalBundleB34[5].Path != CSharpG1LedgerPathR2 {
		t.Errorf("executed or ledger paths differ from the constants")
	}
	for _, m := range slices.Concat(csharpG1ApprovalBundle, csharpG1ApprovalBundleB34) {
		if !strings.HasSuffix(m.Path, "-"+m.Revision+".json") {
			t.Errorf("%s: revision %s is not the revision of the file name", m.Path, m.Revision)
		}
	}
}

// The assets of a later stage (G1-3b) are tracked beside the approved bundle and pending: they do not make the bundle fail, they are not
// approved (the verifier lists them), and a G1 asset that is neither in the bundle nor pending still fails.
func TestCSharpG1ApprovalToleratesOnlyThePendingAssets(t *testing.T) {
	fsys := g1ApprovalClone(g1ApprovalFS(t))
	_, data := g1ApprovalTupleOf(t, fsys)
	g1ApprovalWith(fsys, data)
	for _, p := range g1ApprovalB34Paths() {
		fsys[p] = &fstest.MapFile{Data: []byte("{}\n")}
	}
	state, _, err := VerifyCSharpG1Approval(fsys)
	if err != nil || state != CSharpG1ApprovalApproved {
		t.Fatalf("the bundle with its pending assets: %q %v", state, err)
	}
	if got := CSharpG1PendingAssets(fsys); !slices.Equal(got, g1ApprovalB34Paths()) {
		t.Errorf("pending assets %v, want %v", got, g1ApprovalB34Paths())
	}
	if got := CSharpG1PendingAssets(g1ApprovalFS(t)); len(got) != 0 {
		t.Errorf("pending assets of a tree without them: %v", got)
	}
	fsys["src/contracts/csharp-g1-other-r1.json"] = &fstest.MapFile{Data: []byte("{}\n")}
	if _, _, err := VerifyCSharpG1Approval(fsys); err == nil || !strings.Contains(err.Error(), "G1_APPROVAL_UNAPPROVED") {
		t.Errorf("an asset that is not pending: %v", err)
	}
}

func TestCSharpG1ApprovalAcceptsTheExactBundle(t *testing.T) {
	fsys := g1ApprovalClone(g1ApprovalFS(t))
	x, data := g1ApprovalTupleOf(t, fsys)
	if len(x.Members) != 11 || x.Scope != CSharpG1ApprovalScope || x.Decision != CSharpG1ApprovalDecision || data[len(data)-1] != '\n' {
		t.Fatalf("tuple: %+v", x)
	}
	name := g1ApprovalWith(fsys, data)
	state, object, err := VerifyCSharpG1Approval(fsys)
	if err != nil || state != CSharpG1ApprovalApproved || object != name {
		t.Fatalf("exact bundle: %q %q %v", state, object, err)
	}
}

func TestCSharpG1ApprovalRejects(t *testing.T) {
	base := g1ApprovalFS(t)
	_, data := g1ApprovalTupleOf(t, base)
	approved := func() fstest.MapFS {
		fsys := g1ApprovalClone(base)
		g1ApprovalWith(fsys, data)
		return fsys
	}
	// the tuple of the approved assets, altered and stored under its own hash
	altered := func(edit func(*CSharpG1ApprovalTuple)) fstest.MapFS {
		x, _ := g1ApprovalTupleOf(t, base)
		edit(&x)
		b, err := csharpG1ApprovalEncode(x)
		if err != nil {
			t.Fatal(err)
		}
		fsys := g1ApprovalClone(base)
		g1ApprovalWith(fsys, b)
		return fsys
	}
	type mutant struct {
		name string
		fsys fstest.MapFS
		code string
	}
	var ms []mutant
	for _, m := range csharpG1ApprovalBundle {
		// one byte of any member
		fsys := approved()
		b := append([]byte(nil), fsys[m.Path].Data...)
		b[len(b)/2] ^= 1
		fsys[m.Path] = &fstest.MapFile{Data: b}
		ms = append(ms, mutant{"byte of " + m.Path, fsys, "G1_APPROVAL_MEMBER"})
		// a member that is absent, a member that is longer by one byte
		fsys = approved()
		delete(fsys, m.Path)
		ms = append(ms, mutant{"missing " + m.Path, fsys, "G1_APPROVAL_MEMBER"})
		fsys = approved()
		fsys[m.Path] = &fstest.MapFile{Data: append(append([]byte(nil), fsys[m.Path].Data...), '\n')}
		ms = append(ms, mutant{"appended byte of " + m.Path, fsys, "G1_APPROVAL_MEMBER"})
	}
	extra := approved()
	extra["src/contracts/csharp-g1-extra-r1.json"] = &fstest.MapFile{Data: []byte("{}\n")}
	ms = append(ms, mutant{"extra unapproved G1 asset", extra, "G1_APPROVAL_UNAPPROVED"})
	ms = append(ms,
		mutant{"wrong scope", altered(func(x *CSharpG1ApprovalTuple) { x.Scope = "legacy-observation" }), "G1_APPROVAL_SCOPE"},
		mutant{"wrong decision", altered(func(x *CSharpG1ApprovalTuple) { x.Decision += "0" }), "G1_APPROVAL_DECISION"},
		mutant{"member left out of the tuple", altered(func(x *CSharpG1ApprovalTuple) { x.Members = x.Members[:10] }), "G1_APPROVAL_BUNDLE"},
		mutant{"member added to the tuple", altered(func(x *CSharpG1ApprovalTuple) { x.Members = append(x.Members, x.Members[0]) }), "G1_APPROVAL_BUNDLE"},
		mutant{"members swapped", altered(func(x *CSharpG1ApprovalTuple) { x.Members[0], x.Members[1] = x.Members[1], x.Members[0] }), "G1_APPROVAL_BUNDLE"},
		mutant{"wrong role", altered(func(x *CSharpG1ApprovalTuple) { x.Members[0].Role = "ledger" }), "G1_APPROVAL_BUNDLE"},
		mutant{"wrong revision", altered(func(x *CSharpG1ApprovalTuple) { x.Members[3].Revision = "r1" }), "G1_APPROVAL_BUNDLE"},
		mutant{"wrong sha", altered(func(x *CSharpG1ApprovalTuple) { x.Members[2].SHA256 = strings.Repeat("0", 64) }), "G1_APPROVAL_MEMBER"},
		mutant{"wrong length", altered(func(x *CSharpG1ApprovalTuple) { x.Members[2].Bytes++ }), "G1_APPROVAL_MEMBER"},
	)
	// objects that are not the canonical, content-addressed tuple
	renamed := g1ApprovalClone(base)
	renamed[path.Join(csharpG1ApprovalObjects, strings.Repeat("0", 64))] = &fstest.MapFile{Data: data}
	ms = append(ms, mutant{"object not named by its hash", renamed, "G1_APPROVAL_OBJECT"})
	spaced := g1ApprovalClone(base)
	g1ApprovalWith(spaced, append([]byte(" "), data...))
	ms = append(ms, mutant{"non-canonical bytes", spaced, "G1_APPROVAL_OBJECT"})
	unknown := g1ApprovalClone(base)
	g1ApprovalWith(unknown, []byte(strings.Replace(string(data), `"members"`, `"note":"x","members"`, 1)))
	ms = append(ms, mutant{"unknown member", unknown, "G1_APPROVAL_JSON"})
	two := approved()
	other := altered(func(x *CSharpG1ApprovalTuple) { x.Decision = "x" })
	for k, v := range other {
		if strings.HasPrefix(k, csharpG1ApprovalObjects+"/") {
			two[k] = v
		}
	}
	ms = append(ms, mutant{"two approval objects", two, "G1_APPROVAL_AMBIGUOUS"})

	for _, m := range ms {
		state, _, err := VerifyCSharpG1Approval(m.fsys)
		if err == nil || state == CSharpG1ApprovalApproved || !strings.Contains(err.Error(), m.code) {
			t.Errorf("%s: state %q, error %v, want %s", m.name, state, err, m.code)
		}
	}
}
