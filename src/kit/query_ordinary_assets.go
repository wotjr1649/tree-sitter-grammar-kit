package kit

import (
	"math/bits"
	"os"
	"reflect"
	"slices"
	"strings"
)

// Asset pins are evidence-root-relative, independently authored and review-pinned.
type ordinaryAssetResolver struct {
	session  *eligibilitySession
	assets   map[string]OrdinaryAsset
	resolved map[string][]byte
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
	if e := b.charge(8*uint64(bits.Len(uint(len(paths))))*pathBytes+1, 0, "ordinary-final-identity-order"); e != nil {
		return e
	}
	slices.Sort(paths)
	g := newGuard(root)
	for _, path := range paths {
		if e := b.charge(20*uint64(len(path))+1, 0, "ordinary-final-physical-identity"); e != nil {
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

func (b *eligibilityBudget) ordinaryFinish(root *os.Root, s *eligibilitySession, res *OrdinaryQuerySemanticsResult, replay bool) *Error {
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
	if replay {
		return nil // The outer seal checks the nested JSON with the same output limit.
	}
	upper, e := b.ordinaryOutputBound(reflect.ValueOf(res))
	if e != nil {
		return e
	}
	if e := b.charge(2*upper+1, 4*upper+32768, "ordinary-result-encoding"); e != nil {
		return e
	}
	res.BytesRead, res.Work, res.LogicalBytes = b.r.totalRead, b.work, b.logical
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
