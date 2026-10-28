package kit

import (
	"io/fs"
	"os"
	"path"
)

type semanticEntryNamespace struct {
	entries     map[string]fs.FileInfo
	hostEntries *uint64
	hostLimit   uint64
}

type semanticEntryView struct {
	root      *os.Root
	rootInfo  fs.FileInfo
	namespace *semanticEntryNamespace
	prefix    string
}

// semanticEntryBudget counts physical entries across all targets in one semantic run.
type semanticEntryBudget struct {
	limit  uint64
	used   uint64
	views  []*semanticEntryView
	active *semanticEntryView
}

func newSemanticEntryBudget(limit uint64) *semanticEntryBudget {
	return &semanticEntryBudget{limit: limit}
}

func openedRootInfo(root *os.Root, at string) (fs.FileInfo, *Error) {
	if root == nil {
		return nil, fail(KindInvalidInput, "SEMANTIC_ROOT_INVALID", at, nil)
	}
	f, err := root.Open(".")
	if err != nil {
		return nil, fail(KindIO, "ROOT_UNREADABLE", at, err)
	}
	info, statErr := f.Stat()
	closeErr := f.Close()
	if statErr != nil {
		return nil, fail(KindIO, "ROOT_UNREADABLE", at, statErr)
	}
	if closeErr != nil {
		return nil, fail(KindIO, "ROOT_CLOSE_FAILED", at, closeErr)
	}
	if !info.IsDir() {
		return nil, fail(KindInvalidInput, "SEMANTIC_ROOT_INVALID", at, nil)
	}
	return info, nil
}

func (b *semanticEntryBudget) begin(root *os.Root, hostEntries *uint64, hostLimit uint64) *Error {
	if b == nil || hostEntries == nil {
		return fail(KindInvalidInput, "SEMANTIC_ENTRY_BUDGET_INVALID", "", nil)
	}
	info, e := openedRootInfo(root, "")
	if e != nil {
		return e
	}
	if *hostEntries > hostLimit {
		return fail(KindResourceLimit, "FILE_COUNT_LIMIT", "", nil)
	}
	for _, view := range b.views {
		if os.SameFile(view.rootInfo, info) {
			return fail(KindInvalidInput, "SEMANTIC_ROOT_OVERLAP", "", nil)
		}
	}
	ns := &semanticEntryNamespace{entries: map[string]fs.FileInfo{}, hostEntries: hostEntries, hostLimit: hostLimit}
	view := &semanticEntryView{root: root, rootInfo: info, namespace: ns, prefix: "."}
	b.views = append(b.views, view)
	b.active = view
	return nil
}

// bindChild proves that child is the opened directory at prefix in a held root.
func (b *semanticEntryBudget) bindChild(parent *os.Root, verifiedPrefix string, child *os.Root) *Error {
	if b == nil || !portable(verifiedPrefix, false) {
		return fail(KindInvalidInput, "SEMANTIC_ROOT_PREFIX_INVALID", verifiedPrefix, nil)
	}
	parentInfo, e := openedRootInfo(parent, verifiedPrefix)
	if e != nil {
		return e
	}
	parentView := b.viewFor(parentInfo)
	if parentView == nil {
		return fail(KindInvalidInput, "SEMANTIC_ROOT_UNBOUND", verifiedPrefix, nil)
	}
	childInfo, e := openedRootInfo(child, verifiedPrefix)
	if e != nil {
		return e
	}
	guarded, e := newGuard(parent).lstat(verifiedPrefix)
	if e != nil {
		return e
	}
	if guarded == nil || kind(guarded) != "dir" || !os.SameFile(guarded, childInfo) {
		return fail(KindIO, "SOURCE_CHANGED", verifiedPrefix, nil)
	}
	prefix := path.Join(parentView.prefix, verifiedPrefix)
	for _, view := range b.views {
		if !os.SameFile(view.rootInfo, childInfo) {
			continue
		}
		if view.namespace != parentView.namespace || view.prefix != prefix {
			return fail(KindInvalidInput, "SEMANTIC_ROOT_OVERLAP", verifiedPrefix, nil)
		}
		if view.root == child {
			b.active = view
			return nil
		}
		alias := &semanticEntryView{root: child, rootInfo: childInfo, namespace: parentView.namespace, prefix: prefix}
		b.views = append(b.views, alias)
		b.active = alias
		return nil
	}
	view := &semanticEntryView{root: child, rootInfo: childInfo, namespace: parentView.namespace, prefix: prefix}
	b.views = append(b.views, view)
	b.active = view
	return nil
}

func (b *semanticEntryBudget) viewFor(info fs.FileInfo) *semanticEntryView {
	for _, view := range b.views {
		if os.SameFile(view.rootInfo, info) {
			return view
		}
	}
	return nil
}

func (b *semanticEntryBudget) viewForRoot(root *os.Root) *semanticEntryView {
	for _, view := range b.views {
		if view.root == root {
			return view
		}
	}
	return nil
}

// record charges an entry once per opened-root namespace and relative path.
func (b *semanticEntryBudget) record(root *os.Root, name string, info fs.FileInfo) (bool, *Error) {
	if b == nil || !portable(name, false) || info == nil {
		return false, fail(KindInvalidInput, "SEMANTIC_ENTRY_INVALID", name, nil)
	}
	view := b.viewForRoot(root)
	if view == nil {
		return false, fail(KindInvalidInput, "SEMANTIC_ROOT_UNBOUND", name, nil)
	}
	observed, e := newGuard(root).lstat(name)
	if e != nil {
		return false, e
	}
	if observed == nil || !os.SameFile(info, observed) {
		return false, fail(KindIO, "SOURCE_CHANGED", name, nil)
	}
	key := path.Join(view.prefix, name)
	if previous := view.namespace.entries[key]; previous != nil {
		if !os.SameFile(previous, observed) {
			return false, fail(KindIO, "SOURCE_CHANGED", key, nil)
		}
		return false, nil
	}
	if b.used >= b.limit {
		return false, fail(KindResourceLimit, "SEMANTIC_ENTRY_LIMIT", key, nil)
	}
	if *view.namespace.hostEntries >= view.namespace.hostLimit {
		return false, fail(KindResourceLimit, "FILE_COUNT_LIMIT", key, nil)
	}
	b.used++
	*view.namespace.hostEntries++
	view.namespace.entries[key] = observed
	return true, nil
}

func (b *semanticEntryBudget) recordActive(name string, info fs.FileInfo) (bool, *Error) {
	if b == nil || b.active == nil {
		return false, fail(KindInvalidInput, "SEMANTIC_ROOT_UNBOUND", name, nil)
	}
	return b.record(b.active.root, name, info)
}
