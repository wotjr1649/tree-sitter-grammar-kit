package kit

import (
	"io/fs"
	"math/bits"
	"os"
	"reflect"
	"strings"
)

// Asset pins are evidence-root-relative, independently authored and review-pinned.
type ordinaryAssetResolver struct {
	session  *eligibilitySession
	assets   map[string]OrdinaryAsset
	resolved map[string][]byte
	pins     map[string]OrdinaryPin // the pin each resolved body was verified against
}

func (a *ordinaryAssetResolver) resolve(id, role string) ([]byte, *Error) {
	s := a.session
	if s == nil || s.b == nil || s.snapshot == nil || !s.snapshot.graphBound {
		return nil, fail(KindInvalidInput, "ORDINARY_ASSET_UNBOUND", id, nil)
	}
	if e := s.b.charge(1, 0, "ordinary-asset-lookup"); e != nil {
		return nil, e
	}
	asset, ok := a.assets[id]
	if !ok || asset.Role != role {
		return nil, fail(KindInvalidInput, "ORDINARY_ASSET_ROLE", id, nil)
	}
	if asset.State == "UNAVAILABLE" {
		return nil, nil
	}
	if asset.State != "PINNED" || asset.Pin == nil || asset.AuthoringBasis == nil || asset.IndependentReview == nil {
		return nil, fail(KindInvalidInput, "ORDINARY_ASSET_STATE", id, nil)
	}
	if data, ok := a.resolved[id]; ok {
		// A shared body is only reused for the pin it was verified against, on every use.
		if a.pins[id] != *asset.Pin || uint64(len(data)) != asset.Pin.Bytes {
			return nil, fail(KindInvalidInput, "ORDINARY_ASSET_PIN", id, nil)
		}
		return data, nil
	}
	var body []byte
	for i, pin := range []*OrdinaryPin{asset.Pin, asset.AuthoringBasis, asset.IndependentReview} {
		if !ordinaryPin(*pin) {
			return nil, fail(KindInvalidInput, "ORDINARY_ASSET_PIN", id, nil)
		}
		owned, ok := s.owned[pin.Path]
		if !ok || owned.NodeID != s.owners[pin.Path] || owned.Member.Path != pin.Path || owned.Member.Bytes != pin.Bytes || owned.Member.SHA256 != pin.SHA256 || i == 0 && owned.Member.Role != role {
			return nil, fail(KindInvalidInput, "ORDINARY_ASSET_OWNER", pin.Path, nil)
		}
		data, ok, e := s.member(&owned)
		if e != nil {
			return nil, e
		}
		if !ok {
			return nil, fail(KindInvalidInput, "ORDINARY_ASSET_MEMBER_MISSING", pin.Path, nil)
		}
		if e := s.b.charge(uint64(len(data))+1, 256+uint64(len(id)), "ordinary-asset-pin"); e != nil {
			return nil, e
		}
		if !ordinaryPinMatchesBytes(*pin, data) {
			return nil, fail(KindInvalidInput, "ORDINARY_ASSET_PIN", pin.Path, nil)
		}
		if i == 0 {
			body = data
		}
	}
	a.resolved[id] = body
	if a.pins == nil {
		a.pins = map[string]OrdinaryPin{}
	}
	a.pins[id] = *asset.Pin
	return body, nil
}

// Inspect all graph members, including expected assets and build inputs outside oracle/.
func (b *eligibilityBudget) ordinarySnapshotIdentity(root *os.Root, s *eligibilitySession) *Error {
	if root == nil || s == nil || s.b != b || s.snapshot == nil || !s.snapshot.graphBound {
		return fail(KindInvalidInput, "ORDINARY_IDENTITY_UNBOUND", "evidence", nil)
	}
	snapshot := s.snapshot
	if snapshot.paging != nil {
		ok, e := snapshot.paging.valid(s)
		if e != nil {
			return e
		}
		if !ok || snapshot.paging.root != root {
			return fail(KindInvalidInput, "ELIGIBILITY_PAGING_IDENTITY", "evidence", nil)
		}
	}
	if e := b.charge(uint64(len(snapshot.observed)+len(snapshot.directories))+1,
		32*uint64(len(snapshot.observed)+len(snapshot.directories)), "ordinary-final-identity-index"); e != nil {
		return e
	}
	paths := make([]string, 0, len(snapshot.observed)+len(snapshot.directories))
	for path := range snapshot.directories {
		paths = append(paths, path)
	}
	for path := range snapshot.observed {
		paths = append(paths, path)
	}
	// Stable ordering makes the first identity failure deterministic.
	var pathBytes uint64
	for _, path := range paths {
		pathBytes += uint64(len(path) + 1)
	}
	if e := b.charge(ordinarySortWork(len(paths), pathBytes), ordinarySortLogical(len(paths)), "ordinary-final-identity-order"); e != nil {
		return e
	}
	sorted := ordinarySortPaths(paths)
	ordinaryVisitObserved("sort", sorted.compares+sorted.bytes+sorted.moved)
	g := newGuard(root)
	for _, path := range paths {
		if e := b.charge(ordinaryPhysicalIdentityWork(path, snapshot.directories), 0, "ordinary-final-physical-identity"); e != nil {
			return e
		}
		before, directory := snapshot.directories[path]
		if !directory {
			before = snapshot.observed[path]
		}
		current, e := g.lstat(path)
		if e != nil {
			return e
		}
		if before == nil || current == nil || !os.SameFile(before, current) || directory && kind(current) != "dir" || !directory && (kind(current) != "file" || before.Size() != current.Size() || !before.ModTime().Equal(current.ModTime())) {
			return fail(KindIO, "SOURCE_CHANGED", path, nil)
		}
	}
	return b.r.check()
}

// ordinaryPhysicalIdentityWork is the work bound of observing one snapshot path with a guard, in the order
// ordinarySnapshotIdentity walks (a path after its prefixes). With the parent in the snapshot the guard has
// cached it, so the observation is one probe of the parent, two scans, one Lstat and one insert: under 6
// units per byte, reserved at 20 as before. Otherwise every ancestor may be uncached: per ancestor a failed
// probe, an Lstat and an insert, each at most one unit per byte of the path.
func ordinaryPhysicalIdentityWork(path string, directories map[string]fs.FileInfo) uint64 {
	work := 20*uint64(len(path)) + 1
	if i := strings.LastIndexByte(path, '/'); i >= 0 && directories[path[:i]] == nil {
		work += 4 * uint64(strings.Count(path, "/")) * uint64(len(path)+1)
	}
	return work
}

// ordinarySortWork is the proven work bound of ordinarySortPaths: ceil(log2 n) merge levels, each
// costing at most one unit per path byte plus one per path (pathBytes counts len+1 per path), and one
// last copy of the buffer into paths.
func ordinarySortWork(n int, pathBytes uint64) uint64 {
	if n < 2 {
		return 1
	}
	return uint64(bits.Len(uint(n-1))+1)*pathBytes + 1
}

// ordinarySortLogical is the buffer of n strings, plus a page for the allocator's rounding.
func ordinarySortLogical(n int) uint64 { return 16*uint64(n) + 8192 }

// ordinarySortCost is what ordinarySortPaths did: comparisons, bytes the comparisons may inspect
// (the shorter operand each) and elements moved without a comparison.
type ordinarySortCost struct{ compares, bytes, moved uint64 }

// ordinarySortPaths sorts paths ascending with a bottom-up merge sort. A merge step emits the
// element it compared and inspects at most the shorter operand, so a level costs at most
// sum(len+1) over the paths and the whole sort at most ordinarySortWork. It needs one buffer of n strings.
func ordinarySortPaths(paths []string) (cost ordinarySortCost) {
	n := len(paths)
	if n < 2 {
		return cost
	}
	src, dst := paths, make([]string, n)
	for width := 1; width < n; width *= 2 {
		for lo := 0; lo < n; lo += 2 * width {
			mid, hi := min(lo+width, n), min(lo+2*width, n)
			i, j, k := lo, mid, lo
			for i < mid && j < hi {
				cost.compares++
				cost.bytes += uint64(min(len(src[i]), len(src[j])))
				if src[j] < src[i] {
					dst[k], j = src[j], j+1
				} else {
					dst[k], i = src[i], i+1
				}
				k++
			}
			cost.moved += uint64(copy(dst[k:], src[i:mid]))
			cost.moved += uint64(copy(dst[k+mid-i:], src[j:hi]))
		}
		src, dst = dst, src
	}
	if &src[0] != &paths[0] {
		cost.moved += uint64(copy(paths, src))
	}
	return cost
}

func (b *eligibilityBudget) ordinaryFinish(root *os.Root, s *eligibilitySession, res *OrdinaryQuerySemanticsResult) *Error {
	if e := b.ordinarySnapshotIdentity(root, s); e != nil {
		res.Complete, res.ProcessingComplete, res.Semantic = false, false, AssessBlocked
		// A changed shared prerequisite invalidates positive judgments made from it.
		for i := range res.Obligations {
			if res.Obligations[i].State == ordinaryTupleVerified {
				res.Obligations[i].State, res.Obligations[i].Code = ordinaryTupleBlocked, e.Code
			}
		}
		res.Tuple.Blocked += res.Tuple.Verified
		res.Tuple.Verified = 0
		res.BindingValid = false
		return e
	}
	ordinaryStage("final-identity", b)
	if res.Tuple.Failed > 0 {
		res.Semantic = AssessFail
	} else if res.BindingValid && res.ProcessingComplete && res.Tuple.Required > 0 && res.Tuple.Verified == res.Tuple.Required &&
		s.bound && s.res.Complete && s.res.Assessment == AssessPass && res.BaseVerified.Assessment == AssessPass && res.Assessment != AssessFail {
		res.Semantic = AssessPass
	}
	if res.Assessment != AssessFail {
		res.Assessment = foldClaims([]string{res.Semantic, res.BaseVerified.Assessment})
		if res.Assessment == AssessBlocked && res.Code == "" {
			res.Code = "ORDINARY_INCOMPLETE"
		}
	}
	res.Complete = true
	upper, e := b.ordinaryOutputBound(reflect.ValueOf(res))
	if e != nil {
		return e
	}
	if e := b.charge(2*upper+1, 4*upper+32768, "ordinary-result-encoding"); e != nil {
		return e
	}
	res.BytesRead, res.Work, res.LogicalBytes = b.r.totalRead, b.work, b.logical
	ordinaryStage("seal", b)
	return sealOutput(res, ReplayOperations()["evidence-replay"].OutputBytes)
}

// Bound JSON escaping before the encoder allocates; numeric counters may still grow.
func (b *eligibilityBudget) ordinaryOutputBound(v reflect.Value) (uint64, *Error) {
	if e := b.charge(1, 512, "ordinary-result-bound"); e != nil {
		return 0, e
	}
	if !v.IsValid() {
		return 4, nil
	}
	var n uint64 = 2
	add := func(child reflect.Value) *Error {
		part, e := b.ordinaryOutputBound(child)
		if e == nil {
			n += part + 1
		}
		return e
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return 4, nil
		}
		return b.ordinaryOutputBound(v.Elem())
	case reflect.String:
		n += 6 * uint64(v.Len())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() || f.Tag.Get("json") == "-" {
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "" {
				name = f.Name
			}
			n += 6*uint64(len(name)) + 3
			if e := add(v.Field(i)); e != nil {
				return 0, e
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return 4, nil
		}
		for i := 0; i < v.Len(); i++ {
			if e := add(v.Index(i)); e != nil {
				return 0, e
			}
		}
	case reflect.Map:
		if v.IsNil() {
			return 4, nil
		}
		iter := v.MapRange()
		for iter.Next() {
			if e := add(iter.Key()); e != nil {
				return 0, e
			}
			if e := add(iter.Value()); e != nil {
				return 0, e
			}
			n++
		}
	default:
		n = 24
	}
	if n > ReplayOperations()["evidence-replay"].OutputBytes {
		return 0, fail(KindResourceLimit, "OUTPUT_LIMIT", "ordinary-result-bound", nil)
	}
	return n, nil
}
