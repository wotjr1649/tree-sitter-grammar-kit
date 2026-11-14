package kit

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	jsonv2 "encoding/json/v2"
)

// The second approval bundle (#233 G1-3b): the B03 and B04 assets, their executed asset and ledger r2. Its object names the first object and
// none of its assets, and it is checked beside the first one.

// g1ApprovalFirstObject is the first approval object of the repository (PR #436), by name.
const g1ApprovalFirstObject = "9a5dbd010ce2e4e0ba9e977f88c9aaf253c082170b03c67802f96d6c655eff94"

func g1ApprovalTupleB34Of(t *testing.T, fsys fstest.MapFS) CSharpG1ApprovalTuple {
	t.Helper()
	data, _, err := BuildCSharpG1ApprovalB34(fsys)
	if err != nil {
		t.Fatal(err)
	}
	var x CSharpG1ApprovalTuple
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	return x
}

// g1ApprovalBoth is a tree with the assets of both bundles and both objects, built the way the user's confirmation would: the first object,
// then the second one, whose tuple is built from the tree.
func g1ApprovalBoth(t *testing.T) (fsys fstest.MapFS, first, second string) {
	t.Helper()
	fsys = g1ApprovalClone(g1ApprovalFS2(t))
	_, data1 := g1ApprovalTupleOf(t, fsys)
	first = g1ApprovalWith(fsys, data1)
	data2, _, err := BuildCSharpG1ApprovalB34(fsys)
	if err != nil {
		t.Fatal(err)
	}
	second = g1ApprovalWith(fsys, data2)
	return fsys, first, second
}

// The first approval object is the one the repository has, and the second bundle does not change what it covers: the tuple built from the
// tracked first-bundle assets is, byte for byte, that object. The new member of the tuple type must not show in it.
func TestCSharpG1ApprovalFirstObjectIsUnchanged(t *testing.T) {
	data, sum, err := BuildCSharpG1Approval(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	if sum != g1ApprovalFirstObject || strings.Contains(string(data), `"base"`) {
		t.Fatalf("the first tuple is %s (want %s), base member %v", sum, g1ApprovalFirstObject, strings.Contains(string(data), `"base"`))
	}
	if !bytes.Equal(data, mustRead(t, "../../"+csharpG1ApprovalObjects+"/"+g1ApprovalFirstObject)) {
		t.Fatal("the tracked first approval object is not the tuple of the tracked assets")
	}
}

func TestCSharpG1ApprovalSecondBundle(t *testing.T) {
	// pending: no object for the scope; the first bundle is approved and the assets are listed as pending, not approved
	pend := g1ApprovalClone(g1ApprovalFS2(t))
	_, data1 := g1ApprovalTupleOf(t, pend)
	first := g1ApprovalWith(pend, data1)
	if state, object, err := VerifyCSharpG1ApprovalB34(pend); err != nil || state != CSharpG1ApprovalPending || object != "" {
		t.Fatalf("no second object: %q %q %v", state, object, err)
	}
	if state, object, err := VerifyCSharpG1Approval(pend); err != nil || state != CSharpG1ApprovalApproved || object != first {
		t.Fatalf("first bundle with the second one pending: %q %q %v", state, object, err)
	}
	if got := CSharpG1PendingAssets(pend); !slices.Equal(got, g1ApprovalB34Paths()) {
		t.Errorf("pending %v", got)
	}
	// the second tuple names the first object and the six assets, none of the first bundle
	x := g1ApprovalTupleB34Of(t, pend)
	if x.Scope != CSharpG1ApprovalScopeB34 || x.Decision != CSharpG1ApprovalDecisionB34 || x.Base != first || len(x.Members) != 6 || x.Schema != CSharpG1ApprovalSchema {
		t.Fatalf("tuple: %+v", x)
	}
	for _, m := range x.Members {
		if slices.ContainsFunc(csharpG1ApprovalBundle, func(f CSharpG1ApprovalMember) bool { return f.Path == m.Path }) {
			t.Errorf("%s is in both bundles", m.Path)
		}
	}
	// approved: both objects, the first one unchanged, nothing pending
	both, first, second := g1ApprovalBoth(t)
	if state, object, err := VerifyCSharpG1ApprovalB34(both); err != nil || state != CSharpG1ApprovalApproved || object != second {
		t.Fatalf("second bundle: %q %q %v", state, object, err)
	}
	if state, object, err := VerifyCSharpG1Approval(both); err != nil || state != CSharpG1ApprovalApproved || object != first {
		t.Fatalf("first bundle beside the second: %q %q %v", state, object, err)
	}
	if first == second {
		t.Fatal("the two bundles are one object")
	}
	if got := CSharpG1PendingAssets(both); len(got) != 0 {
		t.Errorf("approved assets still pending: %v", got)
	}
	// a second bundle cannot be built on a first bundle that is not approved
	if _, _, err := BuildCSharpG1ApprovalB34(g1ApprovalFS2(t)); err == nil || !strings.Contains(err.Error(), "G1_APPROVAL_BASE") {
		t.Errorf("build without a first object: %v", err)
	}
}

// What a second bundle must not be able to do to the first one, and what a change of either does to the other.
func TestCSharpG1ApprovalSecondBundleRejects(t *testing.T) {
	both, first, _ := g1ApprovalBoth(t)
	// the second tuple altered and stored under its own hash, beside the first object
	altered := func(edit func(x *CSharpG1ApprovalTuple)) fstest.MapFS {
		fsys := g1ApprovalClone(g1ApprovalFS2(t))
		g1ApprovalWith(fsys, mustRead(t, "../../"+csharpG1ApprovalObjects+"/"+g1ApprovalFirstObject))
		x := g1ApprovalTupleB34Of(t, fsys)
		edit(&x)
		b, err := csharpG1ApprovalEncode(x)
		if err != nil {
			t.Fatal(err)
		}
		g1ApprovalWith(fsys, b)
		return fsys
	}
	type mutant struct {
		name string
		fsys fstest.MapFS
		code string
	}
	ms := []mutant{
		{"another base", altered(func(x *CSharpG1ApprovalTuple) { x.Base = strings.Repeat("0", 64) }), "G1_APPROVAL_BASE"},
		{"no base", altered(func(x *CSharpG1ApprovalTuple) { x.Base = "" }), "G1_APPROVAL_BASE"},
		{"wrong decision", altered(func(x *CSharpG1ApprovalTuple) { x.Decision = CSharpG1ApprovalDecision }), "G1_APPROVAL_DECISION"},
		// the second tuple lists an asset of the first bundle, in place of one of its own or beside them
		{"a first-bundle asset instead of a member", altered(func(x *CSharpG1ApprovalTuple) { x.Members[0] = csharpG1ApprovalBundle[0] }), "G1_APPROVAL_BUNDLE"},
		{"a first-bundle asset added", altered(func(x *CSharpG1ApprovalTuple) { x.Members = append(x.Members, csharpG1ApprovalBundle[0]) }), "G1_APPROVAL_BUNDLE"},
		{"a member left out", altered(func(x *CSharpG1ApprovalTuple) { x.Members = x.Members[:5] }), "G1_APPROVAL_BUNDLE"},
		{"members swapped", altered(func(x *CSharpG1ApprovalTuple) { x.Members[0], x.Members[1] = x.Members[1], x.Members[0] }), "G1_APPROVAL_BUNDLE"},
		{"wrong revision", altered(func(x *CSharpG1ApprovalTuple) { x.Members[5].Revision = "r1" }), "G1_APPROVAL_BUNDLE"},
		{"wrong sha", altered(func(x *CSharpG1ApprovalTuple) { x.Members[4].SHA256 = strings.Repeat("0", 64) }), "G1_APPROVAL_MEMBER"},
		{"wrong length", altered(func(x *CSharpG1ApprovalTuple) { x.Members[4].Bytes++ }), "G1_APPROVAL_MEMBER"},
	}
	// a second object in the scope of the first one, an object of a third scope, and two objects of the scope of the second one
	x2 := g1ApprovalTupleB34Of(t, both)
	with := func(edit func(x *CSharpG1ApprovalTuple)) fstest.MapFS {
		fsys := g1ApprovalClone(both)
		y := x2
		edit(&y)
		b, err := csharpG1ApprovalEncode(y)
		if err != nil {
			t.Fatal(err)
		}
		g1ApprovalWith(fsys, b)
		return fsys
	}
	ms = append(ms,
		mutant{"a second object in the scope of the first", with(func(y *CSharpG1ApprovalTuple) { y.Scope = CSharpG1ApprovalScope }), "G1_APPROVAL_AMBIGUOUS"},
		mutant{"an object of another scope", with(func(y *CSharpG1ApprovalTuple) { y.Scope = "csharp-g1-other" }), "G1_APPROVAL_SCOPE"},
		mutant{"two objects of the second scope", with(func(y *CSharpG1ApprovalTuple) { y.Decision = "x" }), "G1_APPROVAL_AMBIGUOUS"},
	)
	// every member of the second bundle: one byte of it, and absent
	for _, p := range g1ApprovalB34Paths() {
		fsys := g1ApprovalClone(both)
		b := append([]byte(nil), fsys[p].Data...)
		b[len(b)/2] ^= 1
		fsys[p] = &fstest.MapFile{Data: b}
		ms = append(ms, mutant{"byte of " + p, fsys, "G1_APPROVAL_MEMBER"})
		fsys = g1ApprovalClone(both)
		delete(fsys, p)
		ms = append(ms, mutant{"missing " + p, fsys, "G1_APPROVAL_MEMBER"})
	}
	for _, m := range ms {
		state, _, err := VerifyCSharpG1ApprovalB34(m.fsys)
		if err == nil || state == CSharpG1ApprovalApproved || !strings.Contains(err.Error(), m.code) {
			t.Errorf("%s: state %q, error %v, want %s", m.name, state, err, m.code)
		}
	}

	// a changed first-bundle asset fails the first bundle, and the second cannot stand on it
	for _, m := range csharpG1ApprovalBundle {
		fsys := g1ApprovalClone(both)
		b := append([]byte(nil), fsys[m.Path].Data...)
		b[len(b)/2] ^= 1
		fsys[m.Path] = &fstest.MapFile{Data: b}
		if _, _, err := VerifyCSharpG1Approval(fsys); err == nil || !strings.Contains(err.Error(), "G1_APPROVAL_MEMBER") {
			t.Errorf("first bundle with a changed %s: %v", m.Path, err)
		}
		if state, _, err := VerifyCSharpG1ApprovalB34(fsys); err == nil || state == CSharpG1ApprovalApproved {
			t.Errorf("second bundle on a changed %s: %q %v", m.Path, state, err)
		}
	}
	// a changed asset of the second bundle leaves the first bundle approved: the second object never covered the first
	for _, p := range g1ApprovalB34Paths() {
		fsys := g1ApprovalClone(both)
		b := append([]byte(nil), fsys[p].Data...)
		b[len(b)/2] ^= 1
		fsys[p] = &fstest.MapFile{Data: b}
		if state, _, err := VerifyCSharpG1Approval(fsys); err != nil || state != CSharpG1ApprovalApproved {
			t.Errorf("first bundle with a changed %s: %q %v", p, state, err)
		}
	}
	// the second object without the first one: nothing approves it
	noFirst := g1ApprovalClone(both)
	delete(noFirst, path.Join(csharpG1ApprovalObjects, first))
	if state, _, err := VerifyCSharpG1ApprovalB34(noFirst); err == nil || state == CSharpG1ApprovalApproved || !strings.Contains(err.Error(), "G1_APPROVAL_BASE") {
		t.Errorf("second bundle without the first object: %q %v", state, err)
	}
	// a G1 asset that belongs to neither bundle still fails both
	extra := g1ApprovalClone(both)
	extra["src/contracts/csharp-g1-extra-r1.json"] = &fstest.MapFile{Data: []byte("{}")}
	for _, v := range []func(fs.FS) (string, string, error){VerifyCSharpG1Approval, VerifyCSharpG1ApprovalB34} {
		if _, _, err := v(extra); err == nil || !strings.Contains(err.Error(), "G1_APPROVAL_UNAPPROVED") {
			t.Errorf("an unapproved asset: %v", err)
		}
	}
}

// While the second bundle is pending its assets are not approved: a changed one changes the tuple that is shown for approval (so an approval
// is of exactly the bytes shown), and it does not touch the first bundle.
func TestCSharpG1ApprovalPendingAssetsAreBoundByTheTuple(t *testing.T) {
	pend := g1ApprovalClone(g1ApprovalFS2(t))
	_, data1 := g1ApprovalTupleOf(t, pend)
	g1ApprovalWith(pend, data1)
	_, before, err := BuildCSharpG1ApprovalB34(pend)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range g1ApprovalB34Paths() {
		changed := g1ApprovalClone(pend)
		changed[p] = &fstest.MapFile{Data: append(slices.Clone(changed[p].Data), ' ')}
		if _, after, err := BuildCSharpG1ApprovalB34(changed); err != nil || after == before {
			t.Errorf("a changed pending %s does not change the tuple (%v)", p, err)
		}
		if state, _, err := VerifyCSharpG1Approval(changed); err != nil || state != CSharpG1ApprovalApproved {
			t.Errorf("a changed pending asset breaks the first bundle: %q %v", state, err)
		}
	}
}
