package kit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// guardLstatReference is the guard.lstat of before #362 bounds: every parent prefix is re-joined and
// looked up in order. It is the oracle of the equivalence test.
func guardLstatReference(g *guard, name string) (fs.FileInfo, *Error) {
	if name != "." {
		parts := strings.Split(name, "/")
		for i := 1; i < len(parts); i++ {
			dir := strings.Join(parts[:i], "/")
			if g.dirs[dir] {
				continue
			}
			info, err := g.root.Lstat(dir)
			if errors.Is(err, fs.ErrNotExist) {
				return nil, nil
			}
			if err != nil {
				return nil, fail(KindIO, "UNREADABLE", dir, err)
			}
			if k := kind(info); k != "dir" {
				if k == "file" {
					return nil, nil
				}
				return nil, fail(KindInvalidInput, "LINK_OR_SPECIAL_REJECTED", dir, nil)
			}
			g.dirs[dir] = true
		}
	}
	info, err := g.root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fail(KindIO, "UNREADABLE", name, err)
	}
	if kind(info) == "dir" {
		g.dirs[name] = true
	}
	return info, nil
}

func sameObservation(a, b fs.FileInfo, ea, eb *Error) bool {
	if (ea == nil) != (eb == nil) || (a == nil) != (b == nil) {
		return false
	}
	if ea != nil {
		return ea.Kind == eb.Kind && ea.Code == eb.Code && ea.Path == eb.Path
	}
	return a == nil || os.SameFile(a, b)
}

// The new guard.lstat gives the same observation, error kind, code and path as the old one, for
// existing, missing and non-directory parents, from any cache state and in any order of names.
func TestGuardLstatMatchesReference(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, d := range []string{"a", "a/b", "a/b/c", "x", "x/y"} {
		if err := root.Mkdir(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"a/file", "a/b/c/leaf", "x/y/z.txt", "top"} {
		h, err := root.Create(f)
		if err != nil {
			t.Fatal(err)
		}
		_ = h.Close()
	}
	names := []string{".", "top", "a", "a/b", "a/b/c", "a/b/c/leaf", "a/b/c/missing", "a/missing/deep/er", "a/file/under-a-file", "a/file/under/a-file/deeper", "x/y/z.txt",
		"x/y/z.txt/below", "nope", "nope/deeper", "a/b/c/leaf/x/y", "a//b", "a/b/", "x/y/z.txt/"}
	if err := os.Symlink(dir, dir+"-link"); err == nil { // a link outside the root is only reached by name
		defer os.Remove(dir + "-link")
	}
	if os.Symlink("a", dir+"/lnk") == nil {
		names = append(names, "lnk", "lnk/b", "lnk/b/c/leaf")
	}
	orders := [][]string{names}
	rev := make([]string, len(names))
	for i, n := range names {
		rev[len(names)-1-i] = n
	}
	orders = append(orders, rev)
	for shift := 1; shift < len(names); shift += 3 {
		orders = append(orders, append(append([]string{}, names[shift:]...), names[:shift]...))
	}
	for oi, order := range orders {
		gNew, gOld := newGuard(root), newGuard(root)
		for _, name := range order {
			infoNew, errNew := gNew.lstat(name)
			infoOld, errOld := guardLstatReference(gOld, name)
			if !sameObservation(infoNew, infoOld, errNew, errOld) {
				t.Fatalf("order %d name %q: new (%v, %v), reference (%v, %v)", oi, name, infoNew, errNew, infoOld, errOld)
			}
			if len(gNew.dirs) != len(gOld.dirs) {
				t.Fatalf("order %d name %q: %d cached directories, reference %d", oi, name, len(gNew.dirs), len(gOld.dirs))
			}
			for d := range gOld.dirs {
				if !gNew.dirs[d] {
					t.Fatalf("order %d name %q: directory %q cached only by the reference", oi, name, d)
				}
			}
		}
	}
}

// physicalIdentityFixture is an ordinary snapshot over a real directory tree.
type physicalIdentityFixture struct {
	f ordinaryReaderFixture
}

func (p physicalIdentityFixture) addDir(t *testing.T, name string, inSnapshot bool) {
	t.Helper()
	if err := p.f.root.Mkdir(name, 0700); err != nil {
		t.Fatalf("mkdir (depth %d, %d bytes): %v", strings.Count(name, "/")+1, len(name), err)
	}
	if inSnapshot {
		info, err := p.f.root.Lstat(name)
		if err != nil {
			t.Fatal(err)
		}
		p.f.s.snapshot.directories[name] = info
	}
}

func (p physicalIdentityFixture) addFile(t *testing.T, name string) {
	t.Helper()
	h, err := p.f.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("create (depth %d, %d bytes): %v", strings.Count(name, "/")+1, len(name), err)
	}
	_ = h.Close()
	if p.f.s.snapshot.observed[name], err = p.f.root.Lstat(name); err != nil {
		t.Fatal(err)
	}
}

// guardLstatUnits prices one guardLstatHook step: its bytes, and for the scan (twice the path) the
// three further hashes of the path the walk makes in the snapshot maps, one per byte.
func guardLstatUnits(step string, n int) uint64 {
	if step == "scan" {
		return uint64(n) + 3*uint64(n)/2
	}
	return uint64(n)
}

// physicalSameFileUnits is what os.Root and os.SameFile may add per observed path beyond the guard's
// own steps: at most 3 syscalls for each of the two SameFile calls and one unit for the identity check.
const physicalSameFileUnits = 8

// measure runs ordinarySnapshotIdentity and compares, path by path, what the walk did with what was
// charged to ordinary-final-physical-identity. The walk's cost is the guard's scanned, hashed and
// handed-over path bytes (guardLstatHook), the walk's map lookups of the path, and physicalSameFileUnits.
func (p physicalIdentityFixture) measure(t *testing.T) (paths int, spent, charged, worstRatioNum, worstRatioDen uint64) {
	t.Helper()
	prevCharge, prevGuard := ordinaryChargeHook, guardLstatHook
	t.Cleanup(func() { ordinaryChargeHook, guardLstatHook = prevCharge, prevGuard })
	var cost, work uint64
	var open bool
	finish := func() {
		if !open {
			return
		}
		cost += physicalSameFileUnits
		if cost > work {
			t.Errorf("a path cost %d units against %d charged", cost, work)
		}
		spent += cost
		charged += work
		if worstRatioDen == 0 || cost*worstRatioDen > worstRatioNum*work {
			worstRatioNum, worstRatioDen = cost, work
		}
		paths++
	}
	ordinaryChargeHook = func(_ *eligibilityBudget, w, _ uint64, at string, _, _ uint64) {
		if at == "ordinary-final-physical-identity" {
			finish()
			open, cost, work = true, 0, w
		}
	}
	guardLstatHook = func(step string, n int) { cost += guardLstatUnits(step, n) }
	e := p.f.b.ordinarySnapshotIdentity(p.f.root, p.f.s)
	finish()
	if e != nil {
		t.Fatalf("identity: %v", e)
	}
	return paths, spent, charged, worstRatioNum, worstRatioDen
}

// "Charge >= actual" for the physical identity walk: the work reserved for each path covers what the
// guard really scanned, hashed and handed to the root, for very deep paths, very long components, a
// wide directory, and snapshots that omit the ancestors of a path.
//
// The deepest case (depth 300, the tightest: its worst path spends 16 of 21 charged units) stays in every run. The
// wide directory and the two cases with missing ancestors, whose worst path is the same 51 of 121, use a smaller
// size by default; TSGK_LONG_BUDGET_TESTS=1 runs the full sizes and a file on every level of the deep chain.
func TestOrdinaryPhysicalIdentityChargeCoversTheWalk(t *testing.T) {
	long := os.Getenv("TSGK_LONG_BUDGET_TESTS") == "1"
	size := func(full, short int) int {
		if long {
			return full
		}
		return short
	}
	wide, missing, partial := size(2000, 500), size(200, 100), size(120, 60)
	cases := []struct {
		name  string
		build func(t *testing.T, p physicalIdentityFixture)
	}{
		// Every directory is in the snapshot and a file hangs on every tenth level; the walk of each path scales with
		// its depth, so a file on every level is the long matrix (TSGK_LONG_BUDGET_TESTS=1).
		{"depth-300-single-byte-components", func(t *testing.T, p physicalIdentityFixture) {
			path := ""
			for i := 0; i < 300; i++ {
				path += string(rune('a' + i%26))
				p.addDir(t, path, true)
				if i%10 == 9 {
					p.addFile(t, path+"/_")
				}
				path += "/"
			}
		}},
		{"depth-100-components-of-200-bytes", func(t *testing.T, p physicalIdentityFixture) {
			path := ""
			for i := 0; i < 100; i++ {
				path += strings.Repeat(string(rune('a'+i%26)), 200)
				p.addDir(t, path, true)
				path += "/"
			}
			p.addFile(t, path+"leaf")
		}},
		{"one-component-of-255-bytes-and-wide-directory", func(t *testing.T, p physicalIdentityFixture) {
			long := strings.Repeat("l", 255)
			p.addDir(t, long, true)
			for i := 0; i < wide; i++ {
				p.addFile(t, fmt.Sprintf("%s/%s%04d", long, strings.Repeat("n", 200), i))
			}
		}},
		{"deep-path-whose-ancestors-are-not-in-the-snapshot", func(t *testing.T, p physicalIdentityFixture) {
			path := ""
			for i := 0; i < missing; i++ {
				path += strings.Repeat("d", 8) + "/"
				p.addDir(t, strings.TrimSuffix(path, "/"), false)
			}
			p.addFile(t, path+"leaf")
			p.addFile(t, path+"leaf2")
		}},
		{"deep-path-with-only-some-ancestors-in-the-snapshot", func(t *testing.T, p physicalIdentityFixture) {
			path := ""
			for i := 0; i < partial; i++ {
				path += strings.Repeat("e", 30) + "/"
				p.addDir(t, strings.TrimSuffix(path, "/"), i%7 == 0)
			}
			p.addFile(t, path+"leaf")
		}},
	}
	if long {
		cases = append(cases, struct {
			name  string
			build func(t *testing.T, p physicalIdentityFixture)
		}{"depth-300-a-file-on-every-level", func(t *testing.T, p physicalIdentityFixture) {
			path := ""
			for i := 0; i < 300; i++ {
				path += string(rune('a' + i%26))
				p.addDir(t, path, true)
				p.addFile(t, path+"/_")
				path += "/"
			}
		}})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := physicalIdentityFixture{newOrdinaryReaderFixture(t)}
			tc.build(t, p)
			paths, spent, charged, num, den := p.measure(t)
			if paths == 0 || spent > charged {
				t.Fatalf("%d paths: spent %d, charged %d", paths, spent, charged)
			}
			t.Logf("%d paths; units spent %d, charged %d; worst path %d of %d", paths, spent, charged, num, den)
		})
	}
}

// A path whose parent is in the snapshot keeps the old charge; one whose parent is not pays for every
// ancestor, and the charge never drops below the old formula.
func TestOrdinaryPhysicalIdentityWorkFormula(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := map[string]fs.FileInfo{"a": info, "a/b": info}
	if got := ordinaryPhysicalIdentityWork("a/b/c", snapshot); got != 20*5+1 {
		t.Errorf("parent in the snapshot: %d", got)
	}
	if got := ordinaryPhysicalIdentityWork("top", snapshot); got != 20*3+1 {
		t.Errorf("top-level path: %d", got)
	}
	if got, old := ordinaryPhysicalIdentityWork("p/q/r/leaf", snapshot), uint64(20*10+1); got <= old {
		t.Errorf("parent missing from the snapshot: %d, not above the old formula %d", got, old)
	}
}
