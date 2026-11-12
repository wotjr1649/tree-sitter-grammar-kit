package evidencepins

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRepositoryDefaultsLoadAndScope(t *testing.T) {
	s, err := Repository()
	if err != nil {
		t.Fatal(err)
	}
	ref, err := s.Reference()
	if err != nil || ref.TupleSHA256 == "" || ref.RustcRelease == "" || ref.CargoRelease == "" {
		t.Fatalf("reference default unavailable: %#v, %v", ref, err)
	}
	runtime, err := s.Runtime()
	if err != nil || !runtime.Closure || !runtime.Logger || runtime.ManifestSHA256 == "" {
		t.Fatalf("runtime default unavailable: %#v, %v", runtime, err)
	}
	current, ok := s.RuntimeFor(runtime.ManifestSHA256)
	if !ok || !reflect.DeepEqual(current, runtime) || !s.LoggerAllowed(runtime.ManifestSHA256) {
		t.Fatalf("current default lookup differs: %#v", current)
	}
	legacyLoggers, err := s.Object(pointerFrom(t, repositoryFiles).LegacyLoggerSetSHA256)
	if err != nil {
		t.Fatal(err)
	}
	var legacy legacyLoggerSet
	if err := decode(legacyLoggers, &legacy); err != nil || len(legacy.Manifests) != 2 {
		t.Fatalf("legacy logger set: %#v, %v", legacy, err)
	}
	former, ok := s.RuntimeFor(legacy.Manifests[1])
	if !ok || !former.Logger || former.Closure || !s.LoggerAllowed(legacy.Manifests[1]) {
		t.Fatalf("former logger scope widened or lost: %#v", former)
	}
	if _, ok := s.RuntimeFor(strings.Repeat("d", 64)); ok || s.LoggerAllowed(strings.Repeat("d", 64)) {
		t.Fatal("unapproved runtime manifest was accepted")
	}
	identity := s.Identity()
	if identity.ApprovalSource != DefaultApprovalSource || !validHash(identity.SelectionSHA256) || !validHash(identity.PinSetSHA256) || !validHash(identity.PolicySHA256) {
		t.Fatalf("default identity is incomplete: %#v", identity)
	}
	data, err := s.Object(ref.CatalogueSHA256)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte(nil), data...)
	data[0] ^= 0xff
	again, err := s.Object(ref.CatalogueSHA256)
	if err != nil || !bytes.Equal(again, want) {
		t.Fatalf("object reads shared mutable bytes: %v", err)
	}
}

func TestReferenceForKeepsCatalogueWhenMultipleTuplesAreApproved(t *testing.T) {
	store := &Store{references: map[string][]Reference{"catalogue": {{TupleSHA256: "A"}, {TupleSHA256: "B"}}}}
	got, ok := store.ReferenceFor("catalogue")
	if !ok || got.TupleSHA256 != "A" {
		t.Fatalf("catalogue lookup lost a multiply approved catalogue: %#v", got)
	}
}

func TestRepositoryDefaultsABCPreservesA(t *testing.T) {
	base, err := Repository()
	if err != nil {
		t.Fatal(err)
	}
	refA, _ := base.Reference()
	runtimeA, _ := base.RuntimeFor(base.DefaultRuntimeManifest())
	legacyA, _ := base.RuntimeFor("affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080")
	files := copyRepository(t)
	selection, set, policy := readPolicy(t, files)
	pairs := []struct{ rustc, cargo string }{{"2.3.5", "7.8.9"}, {"3.4.6", "8.9.1"}}
	for _, pair := range pairs {
		tupleBytes, err := jsonv2.Marshal(referenceTuple{Schema: referenceTupleSchema, RustcRelease: pair.rustc, CargoRelease: pair.cargo, PathReader: referenceReader, CatalogueSHA256: refA.CatalogueSHA256})
		if err != nil {
			t.Fatal(err)
		}
		tupleSHA := addObject(files, tupleBytes)
		policy.References = append(policy.References, referenceEntry{CatalogueSHA256: refA.CatalogueSHA256, TupleSHA256: tupleSHA})
		set.Objects = append(set.Objects, objectInfo{SHA256: tupleSHA, Bytes: uint64(len(tupleBytes))})
	}
	for i, pair := range []struct{ language, minimum uint32 }{{16, 14}, {17, 15}} {
		manifest := changedRuntimeManifest(t, base, runtimeA.ManifestSHA256, strings.Repeat(string(rune('e'+i)), 40))
		manifestSHA := addObject(files, manifest)
		tupleBytes, err := jsonv2.Marshal(runtimeTuple{Schema: runtimeTupleSchema, Reader: runtimeReader, LoggerReader: loggerReader,
			ManifestSHA256: manifestSHA, DriverSHA256: runtimeA.DriverSHA256, CP949SHA256: runtimeA.CP949SHA256,
			ShimTemplateSHA256: runtimeA.ShimTemplateSHA256, RuntimeLanguageVersion: pair.language, RuntimeMinCompatible: pair.minimum})
		if err != nil {
			t.Fatal(err)
		}
		tupleSHA := addObject(files, tupleBytes)
		policy.Runtimes = append(policy.Runtimes, runtimeEntry{TupleSHA256: tupleSHA, ManifestSHA256: manifestSHA, Logger: true, Closure: true})
		set.Objects = append(set.Objects, objectInfo{SHA256: manifestSHA, Bytes: uint64(len(manifest))}, objectInfo{SHA256: tupleSHA, Bytes: uint64(len(tupleBytes))})
	}
	next := repin(t, files, selection, set, policy)
	refAfter, err := next.Reference()
	if err != nil || !reflect.DeepEqual(refA, refAfter) {
		t.Fatalf("new catalogue entries retargeted legacy A: %#v != %#v", refAfter, refA)
	}
	runtimeAfter, ok := next.RuntimeFor(runtimeA.ManifestSHA256)
	if !ok || !reflect.DeepEqual(runtimeA, runtimeAfter) {
		t.Fatalf("new runtime entries changed legacy A: %#v != %#v", runtimeAfter, runtimeA)
	}
	legacyAfter, ok := next.RuntimeFor(legacyA.ManifestSHA256)
	if !ok || !reflect.DeepEqual(legacyA, legacyAfter) || !next.LoggerAllowed(legacyA.ManifestSHA256) {
		t.Fatalf("new runtime entries changed former logger A: %#v != %#v", legacyAfter, legacyA)
	}
	for i, pair := range pairs {
		resolved, ok := next.ReferenceForToolchain(refA.CatalogueSHA256, pair.rustc, pair.cargo)
		if !ok || resolved.RustcRelease != pair.rustc || resolved.CargoRelease != pair.cargo || resolved.TupleSHA256 == refA.TupleSHA256 {
			t.Fatalf("B/C reference %d not selected as an indivisible tuple: %#v", i, resolved)
		}
		if !next.ReferenceReleaseAllowed(refA.CatalogueSHA256, "rustc", pair.rustc) || !next.ReferenceReleaseAllowed(refA.CatalogueSHA256, "cargo", pair.cargo) {
			t.Fatalf("B/C toolchain member %d was not allowed", i)
		}
	}
	if _, ok := next.ReferenceForToolchain(refA.CatalogueSHA256, pairs[0].rustc, pairs[1].cargo); ok {
		t.Fatal("rustc and cargo from different approved tuples were mixed")
	}
	if next.ReferenceReleaseAllowed(refA.CatalogueSHA256, "rustc", "1.2.3") || next.ReferenceReleaseAllowed(refA.CatalogueSHA256, "other", pairs[0].rustc) {
		t.Fatal("unapproved or unknown tool release was accepted")
	}
	for i, runtime := range policy.Runtimes[2:] {
		resolved, ok := next.RuntimeFor(runtime.ManifestSHA256)
		if !ok || !resolved.Logger || !resolved.Closure || !next.LoggerAllowed(runtime.ManifestSHA256) || resolved.RuntimeLanguageVersion < runtimeA.RuntimeLanguageVersion {
			t.Fatalf("B/C logger-only scope %d changed: %#v", i, resolved)
		}
	}
	if out := os.Getenv("TSGK_DEFAULTS_CANDIDATE_OUT"); out != "" {
		if err := exportFS(out, files); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRepositoryDefaultsRejectUnboundOrReducedData(t *testing.T) {
	t.Run("set bytes digest", func(t *testing.T) {
		files := copyRepository(t)
		selection := pointerFrom(t, files)
		path := "default-sets/" + selection.SetSHA256 + ".json"
		files[path].Data = append(files[path].Data, ' ')
		if _, err := load(files); err == nil {
			t.Fatal("modified set bytes passed their pinned digest")
		}
	})
	t.Run("content-addressed material changed", func(t *testing.T) {
		files := copyRepository(t)
		selection := pointerFrom(t, files)
		set := setFrom(t, files, selection)
		files["objects/"+set.Objects[0].SHA256].Data[0] ^= 0xff
		if _, err := load(files); err == nil {
			t.Fatal("modified material passed its content address")
		}
	})
	t.Run("repinned reduced runtime obligations", func(t *testing.T) {
		files := copyRepository(t)
		selection, set, policy := readPolicy(t, files)
		policy.RuntimeObligations = strings.Repeat("0", 64)
		if _, err := repinCandidate(t, files, selection, set, policy); err == nil {
			t.Fatal("repinned policy replaced the selection-pinned runtime obligations")
		}
	})
	t.Run("repinned logger allowlist reduction", func(t *testing.T) {
		files := copyRepository(t)
		selection, set, policy := readPolicy(t, files)
		policy.Runtimes = policy.Runtimes[:1]
		if _, err := repinCandidate(t, files, selection, set, policy); err == nil {
			t.Fatal("repinned policy removed a legacy logger entry")
		}
	})
	t.Run("duplicate catalogue tuple", func(t *testing.T) {
		files := copyRepository(t)
		selection, set, policy := readPolicy(t, files)
		policy.References = append(policy.References, policy.References[0])
		if _, err := repinCandidate(t, files, selection, set, policy); err == nil {
			t.Fatal("ambiguous catalogue approval was accepted")
		}
	})
	t.Run("unknown policy member after repin", func(t *testing.T) {
		files := copyRepository(t)
		selection, set, policy := readPolicy(t, files)
		raw, err := jsonv2.Marshal(policy)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := jsonv2.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		fields["unreviewed_scope"] = "full-semantic"
		if _, err := repinRawPolicy(t, files, selection, set, mustMarshal(t, fields)); err == nil {
			t.Fatal("unknown policy field was accepted after its hashes were recomputed")
		}
	})
	t.Run("duplicate policy member after repin", func(t *testing.T) {
		files := copyRepository(t)
		selection := pointerFrom(t, files)
		set := setFrom(t, files, selection)
		raw := bytes.TrimSpace(files["objects/"+set.PolicySHA256].Data)
		raw = append(bytes.TrimSuffix(raw, []byte("}")), []byte(`,"reader":"unsupported/r1"}`)...)
		if _, err := repinRawPolicy(t, files, selection, set, raw); err == nil {
			t.Fatal("duplicate policy member was accepted after its hashes were recomputed")
		}
	})
	t.Run("rehashed shrunk catalogue", func(t *testing.T) {
		files := copyRepository(t)
		selection, set, policy := readPolicy(t, files)
		store, err := load(files)
		if err != nil {
			t.Fatal(err)
		}
		base, _ := store.Reference()
		catalogue, err := store.Object(base.CatalogueSHA256)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if jsonv2.Unmarshal(catalogue, &fields) != nil {
			t.Fatal("decode catalogue")
		}
		builds := fields["builds"].([]any)
		fields["builds"] = builds[:len(builds)-1]
		shrunk, err := jsonv2.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		catalogueSHA := addObject(files, shrunk)
		tuple, err := jsonv2.Marshal(referenceTuple{Schema: referenceTupleSchema, RustcRelease: "2.3.5", CargoRelease: "7.8.9", PathReader: referenceReader, CatalogueSHA256: catalogueSHA})
		if err != nil {
			t.Fatal(err)
		}
		tupleSHA := addObject(files, tuple)
		set.Objects = append(set.Objects, objectInfo{SHA256: catalogueSHA, Bytes: uint64(len(shrunk))}, objectInfo{SHA256: tupleSHA, Bytes: uint64(len(tuple))})
		policy.References = append(policy.References, referenceEntry{CatalogueSHA256: catalogueSHA, TupleSHA256: tupleSHA})
		if _, err := repinRawPolicy(t, files, selection, set, mustMarshal(t, policy)); err == nil {
			t.Fatal("catalogue with a reduced reader obligation was accepted after repin")
		}
	})
	t.Run("rehashed reduced runtime inventory", func(t *testing.T) {
		files := copyRepository(t)
		selection, set, policy := readPolicy(t, files)
		store, err := load(files)
		if err != nil {
			t.Fatal(err)
		}
		base, _ := store.Runtime()
		manifestRaw, err := store.Object(base.ManifestSHA256)
		if err != nil {
			t.Fatal(err)
		}
		var manifest map[string]any
		if jsonv2.Unmarshal(manifestRaw, &manifest) != nil {
			t.Fatal("decode runtime manifest")
		}
		rows := manifest["files"].([]any)
		manifest["files"] = rows[:len(rows)-1]
		shrunk, err := jsonv2.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		manifestSHA := addObject(files, shrunk)
		tuple, err := jsonv2.Marshal(runtimeTuple{Schema: runtimeTupleSchema, Reader: runtimeReader, LoggerReader: loggerReader, ManifestSHA256: manifestSHA,
			DriverSHA256: base.DriverSHA256, CP949SHA256: base.CP949SHA256, ShimTemplateSHA256: base.ShimTemplateSHA256,
			RuntimeLanguageVersion: base.RuntimeLanguageVersion, RuntimeMinCompatible: base.RuntimeMinCompatible})
		if err != nil {
			t.Fatal(err)
		}
		tupleSHA := addObject(files, tuple)
		set.Objects = append(set.Objects, objectInfo{SHA256: manifestSHA, Bytes: uint64(len(shrunk))}, objectInfo{SHA256: tupleSHA, Bytes: uint64(len(tuple))})
		policy.Runtimes = append(policy.Runtimes, runtimeEntry{TupleSHA256: tupleSHA, ManifestSHA256: manifestSHA, Logger: true, Closure: true})
		if _, err := repinRawPolicy(t, files, selection, set, mustMarshal(t, policy)); err == nil {
			t.Fatal("runtime closure with a reduced inventory was accepted after repin")
		}
	})
	t.Run("unsupported tuple reader", func(t *testing.T) {
		files := copyRepository(t)
		selection, set, policy := readPolicy(t, files)
		base := policy.References[0]
		tuple, err := jsonv2.Marshal(referenceTuple{Schema: referenceTupleSchema, RustcRelease: "2.3.5", CargoRelease: "7.8.9", PathReader: "unreviewed/r1", CatalogueSHA256: base.CatalogueSHA256})
		if err != nil {
			t.Fatal(err)
		}
		tupleSHA := addObject(files, tuple)
		set.Objects = append(set.Objects, objectInfo{SHA256: tupleSHA, Bytes: uint64(len(tuple))})
		policy.References = append(policy.References, referenceEntry{CatalogueSHA256: base.CatalogueSHA256, TupleSHA256: tupleSHA})
		if _, err := repinRawPolicy(t, files, selection, set, mustMarshal(t, policy)); err == nil {
			t.Fatal("unsupported tuple reader was accepted after repin")
		}
	})
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := jsonv2.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func copyRepository(t *testing.T) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	if err := fs.WalkDir(repositoryFiles, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(repositoryFiles, path)
		if err == nil {
			out[path] = &fstest.MapFile{Data: data}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func pointerFrom(t *testing.T, files fs.FS) pointer {
	t.Helper()
	raw, err := fs.ReadFile(files, "repository-defaults.json")
	if err != nil {
		t.Fatal(err)
	}
	var selected pointer
	if err := decode(raw, &selected); err != nil {
		t.Fatal(err)
	}
	return selected
}

func setFrom(t *testing.T, files fs.FS, selected pointer) setFile {
	t.Helper()
	raw, err := fs.ReadFile(files, "default-sets/"+selected.SetSHA256+".json")
	if err != nil {
		t.Fatal(err)
	}
	var set setFile
	if err := decode(raw, &set); err != nil {
		t.Fatal(err)
	}
	return set
}

func readPolicy(t *testing.T, files fstest.MapFS) (pointer, setFile, policy) {
	t.Helper()
	selected := pointerFrom(t, files)
	set := setFrom(t, files, selected)
	raw := files["objects/"+set.PolicySHA256].Data
	var p policy
	if err := decode(raw, &p); err != nil {
		t.Fatal(err)
	}
	return selected, set, p
}

func addObject(files fstest.MapFS, raw []byte) string {
	sha := digest(raw)
	files["objects/"+sha] = &fstest.MapFile{Data: append([]byte(nil), raw...)}
	return sha
}

func changedRuntimeManifest(t *testing.T, store *Store, manifestSHA, commit string) []byte {
	t.Helper()
	raw, err := store.Object(manifestSHA)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if jsonv2.Unmarshal(raw, &manifest) != nil {
		t.Fatal("decode runtime manifest")
	}
	manifest["commit"] = commit
	manifest["note"] = "synthetic approved runtime candidate"
	changed, err := jsonv2.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

func repin(t *testing.T, files fstest.MapFS, selected pointer, set setFile, p policy) *Store {
	t.Helper()
	store, err := repinCandidate(t, files, selected, set, p)
	if err != nil {
		t.Fatalf("repinned default set rejected: %v", err)
	}
	return store
}

func repinCandidate(t *testing.T, files fstest.MapFS, selected pointer, set setFile, p policy) (*Store, error) {
	t.Helper()
	policyRaw, err := jsonv2.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return repinRawPolicy(t, files, selected, set, policyRaw)
}

func repinRawPolicy(t *testing.T, files fstest.MapFS, selected pointer, set setFile, policyRaw []byte) (*Store, error) {
	t.Helper()
	policySHA := addObject(files, policyRaw)
	set.PolicySHA256, set.PolicyBytes = policySHA, uint64(len(policyRaw))
	set.Objects = deduplicateObjects(set.Objects)
	setRaw, err := jsonv2.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	setSHA := addDefaultSet(files, setRaw)
	selected.SetSHA256 = setSHA
	selectedRaw, err := jsonv2.Marshal(selected)
	if err != nil {
		t.Fatal(err)
	}
	files["repository-defaults.json"] = &fstest.MapFile{Data: selectedRaw}
	return load(files)
}

func addDefaultSet(files fstest.MapFS, raw []byte) string {
	sha := digest(raw)
	files["default-sets/"+sha+".json"] = &fstest.MapFile{Data: append([]byte(nil), raw...)}
	return sha
}

func deduplicateObjects(objects []objectInfo) []objectInfo {
	byHash := make(map[string]objectInfo, len(objects))
	for _, object := range objects {
		byHash[object.SHA256] = object
	}
	out := make([]objectInfo, 0, len(byHash))
	for _, object := range byHash {
		out = append(out, object)
	}
	slices.SortFunc(out, func(a, b objectInfo) int { return strings.Compare(a.SHA256, b.SHA256) })
	return out
}

func exportFS(root string, files fstest.MapFS) error {
	sourceRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	replacements := make(map[string]string, len(files))
	for name, file := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, file.Data, 0o600); err != nil {
			return err
		}
		replacements[filepath.Join(sourceRoot, filepath.FromSlash(name))] = path
	}
	data, err := jsonv2.Marshal(map[string]any{"Replace": replacements})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "overlay.json"), data, 0o600)
}
