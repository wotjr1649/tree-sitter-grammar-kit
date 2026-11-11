package kit

import (
	"reflect"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

// ordinaryDecodedKey names one verified body: the asset resolver returns the same backing array
// for an asset pin for the whole pass, so a different asset (or different bytes) never shares an entry.
type ordinaryDecodedKey struct {
	typ  reflect.Type
	body *byte
	size int
}

// ordinaryDecoded is one expected asset decoded once per pass. Its value is shared by every tuple
// and every use and is read-only: consumers copy what they keep and never write through it.
type ordinaryDecoded struct {
	value any    // *T on success
	err   *Error // the InvalidInput outcome of a failed decode, replayed for later uses
	// tree is the structure outcome per source pin the tree was bound to.
	tree map[OrdinaryPin]ordinaryTreeCheck
}

// ordinaryDecodedAssets is the per-pass share of decoded expected assets. Its own cost is priced:
// every lookup, and each retained entry by twice the body bytes (string payloads and slice growth)
// plus 32 B per JSON value (the Go struct, slice header or string header it became).
type ordinaryDecodedAssets struct {
	entries map[ordinaryDecodedKey]*ordinaryDecoded
}

const (
	ordinaryDecodedLookupLogical = 64   // the key hashed and compared
	ordinaryDecodedEntryLogical  = 1024 // entry, key and map slot
)

func ordinaryDecodedRetainedLogical(bodyBytes int, values uint64) uint64 {
	return ordinaryDecodedEntryLogical + 2*uint64(bodyBytes) + 32*values
}

func newOrdinaryDecodedAssets() *ordinaryDecodedAssets {
	return &ordinaryDecodedAssets{entries: map[ordinaryDecodedKey]*ordinaryDecoded{}}
}

// decodeOrdinaryExpected is the ordinary expected-asset decode: one preflight with the error/buffer
// term priced on a name path, and the decoded values priced by Go size (see decodeTypedScanned).
// It also returns the number of JSON values the preflight walked.
func (b *eligibilityBudget) decodeOrdinaryExpected(doc string, data []byte, target any) (uint64, *Error) {
	if e := b.charge(1, uint64(len(data)), doc); e != nil {
		return 0, e
	}
	values, e := b.reserveJSONContainersCount(doc, data, 3, 80, true)
	if e != nil {
		return 0, e
	}
	return values, b.decodeTypedScanned(doc, data, target, b.unmetered, true, true, true)
}

// ordinaryExpectedDecode decodes data into a T once per pass when the budget carries a share.
// data must be non-empty. The returned entry is nil without a share.
func ordinaryExpectedDecode[T any](b *eligibilityBudget, doc string, data []byte) (*T, *ordinaryDecoded, *Error) {
	share := b.decoded
	if share == nil || len(data) == 0 {
		v := new(T)
		_, e := b.decodeOrdinaryExpected(doc, data, v)
		ordinaryDecodeObserved(doc, false)
		return v, nil, e
	}
	if e := b.charge(1, ordinaryDecodedLookupLogical, "ordinary-expected-cache-lookup"); e != nil {
		return nil, nil, e
	}
	key := ordinaryDecodedKey{typ: reflect.TypeFor[T](), body: &data[0], size: len(data)}
	if hit, ok := share.entries[key]; ok {
		ordinaryDecodeObserved(doc, true)
		if hit.err != nil {
			e := *hit.err
			return nil, hit, &e
		}
		return hit.value.(*T), hit, nil
	}
	v := new(T)
	values, e := b.decodeOrdinaryExpected(doc, data, v)
	ordinaryDecodeObserved(doc, false)
	if e != nil && e.Kind != KindInvalidInput {
		return nil, nil, e
	}
	if ce := b.charge(1, ordinaryDecodedRetainedLogical(len(data), values), "ordinary-expected-cache"); ce != nil {
		return nil, nil, ce
	}
	entry := &ordinaryDecoded{}
	if e != nil {
		failed := *e
		entry.err = &failed
		v = nil
	} else {
		entry.value = v
	}
	share.entries[key] = entry
	return v, entry, e
}

// ordinaryTreeCheck is the structure outcome of an expected tree against its bound source.
type ordinaryTreeCheck struct{ code, detail string }

// ordinaryExpectedTreeCheck validates a bound expected tree against its source. The tuple's source
// pin was already compared with the tree's own source pin and the source bytes were hashed against
// the pin, so the outcome is the same for every tuple of that source: with a share it is computed
// and priced once per (tree, source pin) and later uses pay only the lookup.
func (b *eligibilityBudget) ordinaryExpectedTreeCheck(entry *ordinaryDecoded, v *ordinaryExpectedTree, pin OrdinaryPin, source []byte) (ordinaryTreeCheck, *Error) {
	if entry != nil {
		if e := b.charge(1, ordinaryDecodedLookupLogical, "ordinary-expected-tree-check-lookup"); e != nil {
			return ordinaryTreeCheck{}, e
		}
		if got, ok := entry.tree[pin]; ok {
			return got, nil
		}
	}
	nodes := uint64(len(v.Tree))
	logical, e := eligibilityProduct("ordinary-expected-tree-check", nodes, 512)
	if e != nil {
		return ordinaryTreeCheck{}, e
	}
	work, e := eligibilityProduct("ordinary-expected-tree-check", nodes, 16)
	if e != nil {
		return ordinaryTreeCheck{}, e
	}
	if e := b.charge(work+uint64(len(source))+1, logical, "ordinary-expected-tree-check"); e != nil {
		return ordinaryTreeCheck{}, e
	}
	var got ordinaryTreeCheck
	if err := nativewire.ValidateTree(v.Tree, uint64(len(source))); err != nil {
		got = ordinaryTreeCheck{"ORDINARY_EXPECTED_TREE_INVALID", err.Code}
	} else if !ordinaryTreePointsValid(v.Tree, source) {
		got = ordinaryTreeCheck{"ORDINARY_EXPECTED_TREE_POINTS", "expected tree point가 선택된 source와 다르다"}
	}
	if entry != nil {
		if e := b.charge(1, ordinaryDecodedEntryLogical, "ordinary-expected-cache"); e != nil {
			return ordinaryTreeCheck{}, e
		}
		if entry.tree == nil {
			entry.tree = map[OrdinaryPin]ordinaryTreeCheck{}
		}
		entry.tree[pin] = got
	}
	return got, nil
}
