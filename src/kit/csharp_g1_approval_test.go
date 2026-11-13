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
	pending := CSharpG1PendingAssets(os.DirFS("../.."))
	if err != nil || len(all) != len(csharpG1ApprovalBundle)+len(pending) || len(csharpG1ApprovalBundle) != 11 || len(pending) != len(csharpG1ApprovalPendingAssets) {
		t.Fatalf("%d tracked G1 assets (%v), %d in the bundle, %d pending of %d", len(all), err, len(csharpG1ApprovalBundle), len(pending), len(csharpG1ApprovalPendingAssets))
	}
	for _, p := range csharpG1ApprovalPendingAssets {
		if slices.ContainsFunc(csharpG1ApprovalBundle, func(m CSharpG1ApprovalMember) bool { return m.Path == p }) {
			t.Errorf("%s is both in the bundle and pending", p)
		}
	}
	if csharpG1ApprovalBundle[2].Path != CSharpG1ExecutedPath || csharpG1ApprovalBundle[5].Path != CSharpG1ExecutedPathR2 {
		t.Errorf("executed paths differ from the constants")
	}
	for _, m := range csharpG1ApprovalBundle {
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
	for _, p := range csharpG1ApprovalPendingAssets {
		fsys[p] = &fstest.MapFile{Data: []byte("{}\n")}
	}
	state, _, err := VerifyCSharpG1Approval(fsys)
	if err != nil || state != CSharpG1ApprovalApproved {
		t.Fatalf("the bundle with its pending assets: %q %v", state, err)
	}
	if got := CSharpG1PendingAssets(fsys); !slices.Equal(got, csharpG1ApprovalPendingAssets) {
		t.Errorf("pending assets %v, want %v", got, csharpG1ApprovalPendingAssets)
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
