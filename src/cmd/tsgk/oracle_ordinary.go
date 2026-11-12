package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// ordinaryClock is replaced by tests to cross the wall boundary without sleeping.
var ordinaryClock = time.Now

// testHookOrdinaryBeforeVerify runs after every source-only input is read and before the paths are
// opened again; tests use it to replace a file between the check and the use.
var testHookOrdinaryBeforeVerify func()

// ordinaryLedger is the single cumulative ledger of the ordinary record loader. It has the limits of
// the public ordinary API ledger and is never reset: every read, decode and re-open charges it. The
// first failure is kept so a later cancellation or output error cannot replace it.
type ordinaryLedger struct {
	limits                       kit.OrdinaryLimits
	work, logical, read, entries uint64
	start                        time.Time
	err                          error
}

func newOrdinaryLedger() *ordinaryLedger {
	return &ordinaryLedger{limits: kit.OrdinaryPlanLimits(), start: ordinaryClock()}
}

func (l *ordinaryLedger) fail(kind, code string, cause error) error {
	if l.err == nil {
		l.err = &native.Error{Kind: kind, Code: code, Cause: cause}
	}
	return l.err
}

// charge reserves a step before it runs. A step that does not fit stops the loader with the
// ledger unchanged, and so does a step after the wall limit.
func (l *ordinaryLedger) charge(ctx context.Context, work, logical, read, entries uint64) error {
	if l.err != nil {
		return l.err
	}
	if err := ctx.Err(); err != nil {
		return l.fail(kit.KindCancelled, "CANCELLED", err)
	}
	if ordinaryClock().Sub(l.start) > time.Duration(l.limits.WallMillis)*time.Millisecond {
		return l.fail(kit.KindResourceLimit, "ORDINARY_WALL_LIMIT", nil)
	}
	if work > l.limits.Work-l.work || logical > l.limits.Logical-l.logical || read > l.limits.ReadBytes-l.read || entries > l.limits.Entries-l.entries {
		return l.fail(kit.KindResourceLimit, "ORDINARY_RESOURCE_LIMIT", nil)
	}
	l.work, l.logical, l.read, l.entries = l.work+work, l.logical+logical, l.read+read, l.entries+entries
	return nil
}

// chargeDecode prices a strict decode in the unit of the API ledger: one work per JSON value visited
// by each of the two passes, and logical bytes for the document, a typed value of at most 64 bytes per
// value and 240 bytes per container. The values are counted by a bounded token scan that is itself
// part of the price, so the scan is charged one work per value before it runs to completion.
func (l *ordinaryLedger) chargeDecode(ctx context.Context, data []byte) error {
	var values, containers uint64
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.ReadToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ordinaryPathError("ORDINARY_JSON", err)
		}
		values++
		if tok.Kind() == '{' || tok.Kind() == '[' {
			containers++
		}
		if values%0x10000 == 0 {
			if err := l.charge(ctx, 2*0x10000, 0, 0, 0); err != nil {
				return err
			}
		}
	}
	return l.charge(ctx, 1+2*(values%0x10000), 2*uint64(len(data))+64*values+240*containers, 0, 0)
}

type ordinaryOpener func() (*os.File, os.FileInfo, error)

// ordinaryBound is one source-only input: the opener that names it, the identity of the file that
// was read, and the bytes' length and hash.
type ordinaryBound struct {
	name string
	open ordinaryOpener
	info os.FileInfo
	size uint64
	sum  [sha256.Size]byte
}

func ordinaryReadError(err error) error {
	return &native.Error{Kind: kit.KindIO, Code: "ORDINARY_READ_FAILED", Cause: err}
}

// readFile opens, reads and hashes one input. As in the API ledger a file costs one work and
// (size+1) logical bytes per pass; the passes are the read, the hash and the retained copy. The file
// limit is checked from the opened handle before anything is read.
func (l *ordinaryLedger) readFile(ctx context.Context, name string, open ordinaryOpener) ([]byte, *ordinaryBound, error) {
	if err := l.charge(ctx, 64, 0, 0, 1); err != nil {
		return nil, nil, err
	}
	f, info, err := open()
	if err != nil {
		var ne *native.Error
		if errors.As(err, &ne) {
			return nil, nil, err
		}
		return nil, nil, ordinaryReadError(err)
	}
	defer f.Close()
	size := info.Size()
	if size < 0 || uint64(size) > l.limits.FileBytes {
		return nil, nil, l.fail(kit.KindResourceLimit, "ORDINARY_FILE_LIMIT", nil)
	}
	if err := l.charge(ctx, 1, 3*(uint64(size)+1), uint64(size), 0); err != nil {
		return nil, nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, size+1))
	if err != nil {
		return nil, nil, ordinaryReadError(err)
	}
	if int64(len(data)) != size {
		return nil, nil, l.fail(kit.KindInvalidInput, "ORDINARY_INPUT_CHANGED", nil)
	}
	return data, &ordinaryBound{name: name, open: open, info: info, size: uint64(size), sum: sha256.Sum256(data)}, nil
}

// verify opens the path again and requires the same file (identity), length and hash. Its reads are
// charged to the same ledger.
func (l *ordinaryLedger) verify(ctx context.Context, b *ordinaryBound) error {
	if err := l.charge(ctx, 64, 0, 0, 1); err != nil {
		return err
	}
	f, info, err := b.open()
	if err != nil {
		return l.fail(kit.KindInvalidInput, "ORDINARY_INPUT_CHANGED", err)
	}
	defer f.Close()
	if !os.SameFile(b.info, info) || uint64(info.Size()) != b.size {
		return l.fail(kit.KindInvalidInput, "ORDINARY_INPUT_CHANGED", nil)
	}
	if err := l.charge(ctx, 1, 2*(b.size+1), b.size, 0); err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(b.size)+1))
	if err != nil {
		return ordinaryReadError(err)
	}
	if sha256.Sum256(data) != b.sum {
		return l.fail(kit.KindInvalidInput, "ORDINARY_INPUT_CHANGED", nil)
	}
	return nil
}

func ordinaryCLIPath(path string) bool {
	if path == "" || filepath.IsAbs(path) || strings.ContainsAny(path, "\\:") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func ordinaryPathError(code string, cause error) error {
	return &native.Error{Kind: kit.KindInvalidInput, Code: code, Cause: cause}
}

// ordinaryRootOpener opens a root-relative file through the held root handle, so a link cannot
// leave the root, and binds the leaf to the handle it opened.
func ordinaryRootOpener(root *os.Root, dir, rel string) ordinaryOpener {
	return func() (*os.File, os.FileInfo, error) {
		before, err := root.Lstat(rel)
		if err != nil {
			return nil, nil, err
		}
		if isAlias(filepath.Join(dir, filepath.FromSlash(rel)), before) {
			return nil, nil, ordinaryPathError("ORDINARY_PATH_ALIAS", nil)
		}
		f, err := root.Open(rel)
		if err != nil {
			return nil, nil, err
		}
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) {
			f.Close()
			return nil, nil, ordinaryPathError("ORDINARY_FILE_TYPE", err)
		}
		return f, info, nil
	}
}

// ordinaryExternalOpener opens a trusted document outside every input root. The parent directory is
// resolved, checked against the roots and the leaf is bound to the opened handle on every open.
func ordinaryExternalOpener(path string, roots []string) ordinaryOpener {
	return func() (*os.File, os.FileInfo, error) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, nil, ordinaryPathError("ORDINARY_PATH", err)
		}
		dir, err := filepath.EvalSymlinks(filepath.Dir(abs))
		if err != nil || unresolvedAlias(dir) {
			return nil, nil, ordinaryPathError("ORDINARY_INPUT_PATH_UNVERIFIED", err)
		}
		full := filepath.Join(dir, filepath.Base(abs))
		for _, root := range roots {
			contained, err := documentInside(full, root)
			if err != nil {
				return nil, nil, ordinaryPathError("ORDINARY_INPUT_PATH_UNVERIFIED", err)
			}
			if contained {
				return nil, nil, ordinaryPathError("ORDINARY_INPUT_INSIDE_CASE_ROOT", nil)
			}
		}
		before, err := os.Lstat(full)
		if err != nil {
			return nil, nil, err
		}
		if isAlias(full, before) {
			return nil, nil, ordinaryPathError("ORDINARY_PATH_ALIAS", nil)
		}
		f, err := os.Open(full)
		if err != nil {
			return nil, nil, err
		}
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) {
			f.Close()
			return nil, nil, ordinaryPathError("ORDINARY_FILE_TYPE", err)
		}
		return f, info, nil
	}
}

type ordinaryLoadRequest struct {
	Root, Witness, Profile, Assets, RegistrySHA256, Plan string
	Registrations, ShardIDs                              []string
	HasFactPack                                          bool
	InputRoots                                           []string
}

type ordinaryLoaded struct {
	recording   *native.OrdinaryRecordingRequest
	factMapping []byte
	ledger      *ordinaryLedger
	bound       []*ordinaryBound
	root        *os.Root
}

func (o *ordinaryLoaded) Close() error {
	if o == nil || o.root == nil {
		return nil
	}
	return o.root.Close()
}

// Verify opens every input path again immediately before the bytes are used and charges the
// same ledger. It rejects a file that was replaced or changed after it was read.
func (o *ordinaryLoaded) Verify(ctx context.Context) error {
	if testHookOrdinaryBeforeVerify != nil {
		testHookOrdinaryBeforeVerify()
	}
	for _, b := range o.bound {
		if err := o.ledger.verify(ctx, b); err != nil {
			return err
		}
	}
	return nil
}

// loadOrdinaryRecording reads the source-only inputs of one ordinary record: the pinned P0 registry
// documents, the planning profile, assets and registrations, the declared bodies, the P1 witness
// and the full shard plan. Every file is charged to l and bound to the identity it was read from.
func loadOrdinaryRecording(ctx context.Context, l *ordinaryLedger, req ordinaryLoadRequest) (loaded *ordinaryLoaded, err error) {
	rootDir, err := filepath.EvalSymlinks(req.Root)
	if err == nil {
		rootDir, err = filepath.Abs(rootDir)
	}
	if err != nil {
		return nil, &native.Error{Kind: kit.KindIO, Code: "ORDINARY_ROOT_UNREADABLE", Cause: err}
	}
	if info, statErr := os.Stat(rootDir); statErr != nil || !info.IsDir() {
		return nil, ordinaryPathError("ORDINARY_ROOT_INVALID", statErr)
	}
	if unresolvedAlias(rootDir) {
		return nil, ordinaryPathError("ORDINARY_ROOT_ALIAS", nil)
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return nil, &native.Error{Kind: kit.KindIO, Code: "ORDINARY_ROOT_UNREADABLE", Cause: err}
	}
	loaded = &ordinaryLoaded{ledger: l, root: root}
	defer func() {
		if err != nil {
			_ = root.Close() // read-only handle; the first failure stays the result
		}
	}()
	cache := map[string][]byte{}
	track := func(b *ordinaryBound) error {
		for _, other := range loaded.bound {
			if os.SameFile(other.info, b.info) {
				return l.fail(kit.KindInvalidInput, "ORDINARY_PATH_ALIAS", nil)
			}
		}
		loaded.bound = append(loaded.bound, b)
		return nil
	}
	read := func(name string, open ordinaryOpener) ([]byte, error) {
		if data, ok := cache[name]; ok {
			return data, nil
		}
		data, b, err := l.readFile(ctx, name, open)
		if err != nil {
			return nil, err
		}
		if err := track(b); err != nil {
			return nil, err
		}
		cache[name] = data
		return data, nil
	}
	readRoot := func(rel string) ([]byte, error) {
		if !ordinaryCLIPath(rel) {
			return nil, ordinaryPathError("ORDINARY_PATH", nil)
		}
		return read("root:"+rel, ordinaryRootOpener(root, rootDir, rel))
	}
	readExternal := func(path string) ([]byte, error) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, ordinaryPathError("ORDINARY_PATH", err)
		}
		return read("file:"+abs, ordinaryExternalOpener(abs, req.InputRoots))
	}

	registry, err := readRoot(queryregistrycore.Path)
	if err != nil {
		return nil, err
	}
	qualification, err := readRoot(queryregistrycore.QualificationPin.Path)
	if err != nil {
		return nil, err
	}
	slots, err := readRoot(queryregistrycore.SlotPin.Path)
	if err != nil {
		return nil, err
	}
	basis := make(map[string][]byte, 2)
	for _, path := range []string{"src/dev/query-reference/check.py", "src/internal/native/query_policy_test.go"} {
		if basis[path], err = readRoot(path); err != nil {
			return nil, err
		}
	}
	witness, err := readExternal(req.Witness)
	if err != nil {
		return nil, err
	}
	planningProfile, err := readRoot(req.Profile)
	if err != nil {
		return nil, err
	}
	assetRegistration, err := readRoot(req.Assets)
	if err != nil {
		return nil, err
	}
	if err := l.chargeDecode(ctx, assetRegistration); err != nil {
		return nil, err
	}
	assets, err := kit.ParseOrdinaryAssets(assetRegistration)
	if err != nil {
		return nil, err
	}
	if err := l.chargeDecode(ctx, planningProfile); err != nil {
		return nil, err
	}
	planning, err := kit.ParseOrdinaryProfile(planningProfile)
	if err != nil {
		return nil, err
	}
	var factMapping []byte
	if req.HasFactPack {
		if factMapping, err = readRoot("src/contracts/fact-mapping.json"); err != nil {
			return nil, err
		}
	}
	inputs := kit.OrdinaryPlanRequest{Registry: registry, Qualification: qualification, Slots: slots, Witness: witness,
		Profile: planningProfile, Assets: assetRegistration, RegistrySHA256: req.RegistrySHA256,
		Basis: basis, Registrations: [][]byte{}, Bodies: map[string][]byte{}}
	readBody := func(pin kit.OrdinaryPin) error {
		if _, exists := inputs.Bodies[pin.Path]; exists {
			return nil
		}
		data, err := readRoot(pin.Path)
		if errors.Is(err, os.ErrNotExist) {
			return nil // a missing declared body is rejected by the planner, with its own code
		}
		if err == nil {
			inputs.Bodies[pin.Path] = data
		}
		return err
	}
	for _, asset := range assets.Assets {
		if asset.State == "PINNED" {
			for _, pin := range []*kit.OrdinaryPin{asset.Pin, asset.AuthoringBasis, asset.IndependentReview} {
				if err := readBody(*pin); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, binding := range planning.Bindings {
		for _, member := range binding.Members {
			if err := readBody(member.Pin); err != nil {
				return nil, err
			}
		}
	}
	var selected []byte
	seenPaths, seenKeys := map[string]bool{}, map[string]bool{}
	for _, path := range req.Registrations {
		if seenPaths[path] {
			return nil, ordinaryPathError("ORDINARY_REGISTRATION_DUPLICATE", nil)
		}
		seenPaths[path] = true
		raw, err := readRoot(path)
		if err != nil {
			return nil, err
		}
		if err := l.chargeDecode(ctx, raw); err != nil {
			return nil, err
		}
		registration, err := kit.ParseOrdinaryRegistration(raw)
		if err != nil {
			return nil, err
		}
		key := registration.Route + "/" + registration.Platform
		if seenKeys[key] {
			return nil, ordinaryPathError("ORDINARY_REGISTRATION_DUPLICATE", nil)
		}
		seenKeys[key] = true
		inputs.Registrations = append(inputs.Registrations, raw)
		for _, query := range registration.Queries {
			if err := readBody(query.Source); err != nil {
				return nil, err
			}
		}
		// Shard IDs are route/platform/case; the native producer checks every selected shard
		// against the plan and this registration, so a wrong choice is refused there.
		match := len(req.ShardIDs) != 0
		for _, id := range req.ShardIDs {
			match = match && strings.HasPrefix(id, key+"/")
		}
		if match {
			if selected != nil {
				return nil, ordinaryPathError("ORDINARY_REGISTRATION_SELECTION", nil)
			}
			selected = raw
		}
	}
	if selected == nil {
		return nil, ordinaryPathError("ORDINARY_REGISTRATION_SELECTION", nil)
	}
	plan, err := readExternal(req.Plan)
	if err != nil {
		return nil, err
	}
	loaded.recording = &native.OrdinaryRecordingRequest{Inputs: inputs, ShardPlan: plan, Registration: selected, ShardIDs: append([]string(nil), req.ShardIDs...)}
	loaded.factMapping = factMapping
	return loaded, nil
}

func ordinaryInputError(err error) string {
	var nativeErr *native.Error
	if errors.As(err, &nativeErr) {
		return fmt.Sprintf("%s: %s", nativeErr.Kind, nativeErr.Code)
	}
	var kitErr *kit.Error
	if errors.As(err, &kitErr) {
		return fmt.Sprintf("%s: %s %s", kitErr.Kind, kitErr.Code, kitErr.Path)
	}
	return fmt.Sprintf("%s: ORDINARY_INPUT", kit.KindInvalidInput)
}

// loadOrdinaryForRecord runs the loader and the final re-open under one new ledger and reports the
// ledger on stderr. The totals are the wrapper cost of recording and are printed on failure too.
func loadOrdinaryForRecord(ctx context.Context, stderr io.Writer, req ordinaryLoadRequest) (*ordinaryLoaded, int) {
	ledger := newOrdinaryLedger()
	loaded, err := loadOrdinaryRecording(ctx, ledger, req)
	if err == nil {
		err = loaded.Verify(ctx)
	}
	fmt.Fprintf(stderr, "tsgk: ORDINARY_LOADER read=%d work=%d logical=%d entries=%d wall_ms=%d\n", ledger.read, ledger.work, ledger.logical, ledger.entries, ordinaryClock().Sub(ledger.start).Milliseconds())
	if err == nil {
		return loaded, exitOK
	}
	fmt.Fprintf(stderr, "tsgk: %s\n", ordinaryInputError(err))
	_ = loaded.Close()
	var nativeErr *native.Error
	if errors.As(err, &nativeErr) {
		return nil, exitFor(nativeErr.Kind)
	}
	var kitErr *kit.Error
	if errors.As(err, &kitErr) {
		return nil, exitFor(kitErr.Kind)
	}
	return nil, exitUsage
}
