// Package evidencepins reads the repository-reviewed defaults used by legacy readers.
package evidencepins

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"io/fs"
)

const (
	DefaultApprovalSource = "repository-default"
	defaultReader         = "tsgk-repository-defaults/r1"
	defaultSetSchema      = "tsgk-repository-default-set/r1"
	defaultPolicySchema   = "tsgk-repository-default-policy/r1"
	referenceTupleSchema  = "tsgk-reference-pin-tuple/r1"
	runtimeTupleSchema    = "tsgk-runtime-pin-tuple/r1"
	runtimeReader         = "tsgk-runtime-closure/r1"
	loggerReader          = "tsgk-runtime-logger/r1"
	referenceReader       = "rust-path-1.97.1/r1"
)

//go:embed repository-defaults.json default-sets/* objects/*
var repositoryFiles embed.FS

var (
	repositoryOnce sync.Once
	repositoryData *Store
	repositoryErr  error
	versionFormat  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	commitFormat   = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

type pointer struct {
	Schema                     string `json:"schema"`
	ApprovalSource             string `json:"approval_source"`
	SetSHA256                  string `json:"set_sha256"`
	ReferenceFallbackCatalogue string `json:"reference_fallback_catalogue_sha256"`
	ReferenceFallbackTuple     string `json:"reference_fallback_tuple_sha256"`
	RuntimeFallbackManifest    string `json:"runtime_fallback_manifest_sha256"`
	ReferenceObligations       string `json:"reference_obligations_sha256"`
	RuntimeObligations         string `json:"runtime_obligations_sha256"`
	LegacyLoggerSetSHA256      string `json:"legacy_logger_set_sha256"`
}

type setFile struct {
	Schema       string       `json:"schema"`
	Reader       string       `json:"reader"`
	PolicySHA256 string       `json:"policy_sha256"`
	PolicyBytes  uint64       `json:"policy_bytes"`
	Objects      []objectInfo `json:"objects"`
}

type objectInfo struct {
	SHA256 string `json:"sha256"`
	Bytes  uint64 `json:"bytes"`
}

type policy struct {
	Schema                     string           `json:"schema"`
	ApprovalSource             string           `json:"approval_source"`
	Reader                     string           `json:"reader"`
	ReferenceFallbackCatalogue string           `json:"reference_fallback_catalogue_sha256"`
	ReferenceFallbackTuple     string           `json:"reference_fallback_tuple_sha256"`
	ReferenceObligations       string           `json:"reference_obligations_sha256"`
	References                 []referenceEntry `json:"references"`
	RuntimeFallbackManifest    string           `json:"runtime_fallback_manifest_sha256"`
	RuntimeObligations         string           `json:"runtime_obligations_sha256"`
	Runtimes                   []runtimeEntry   `json:"runtimes"`
}

type referenceEntry struct {
	CatalogueSHA256 string `json:"catalogue_sha256"`
	TupleSHA256     string `json:"tuple_sha256"`
}

type runtimeEntry struct {
	TupleSHA256    string `json:"tuple_sha256"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Logger         bool   `json:"logger"`
	Closure        bool   `json:"closure"`
}

type referenceTuple struct {
	Schema          string `json:"schema"`
	RustcRelease    string `json:"rustc_release"`
	CargoRelease    string `json:"cargo_release"`
	PathReader      string `json:"path_reader"`
	CatalogueSHA256 string `json:"catalogue_sha256"`
}

type runtimeTuple struct {
	Schema                 string `json:"schema"`
	Reader                 string `json:"reader"`
	LoggerReader           string `json:"logger_reader"`
	ManifestSHA256         string `json:"manifest_sha256"`
	DriverSHA256           string `json:"driver_sha256"`
	CP949SHA256            string `json:"cp949_sha256"`
	ShimTemplateSHA256     string `json:"shim_template_sha256"`
	RuntimeLanguageVersion uint32 `json:"runtime_language_version"`
	RuntimeMinCompatible   uint32 `json:"runtime_min_compatible"`
}

type runtimeManifest struct {
	Schema     string `json:"schema"`
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Files      []struct {
		Path string `json:"path"`
	} `json:"files"`
	Patches []struct {
		Subject string `json:"subject"`
	} `json:"patches"`
}

type runtimeReaderObligations struct {
	Files   []string `json:"files"`
	Patches []string `json:"patches"`
	Shim    string   `json:"shim"`
}

type legacyLoggerSet struct {
	Schema    string   `json:"schema"`
	Reader    string   `json:"reader"`
	Manifests []string `json:"manifests"`
}

// Identity identifies the immutable repository fixture that supplied default approval.
type Identity struct {
	ApprovalSource  string `json:"approval_source"`
	SelectionSHA256 string `json:"selection_sha256"`
	PinSetSHA256    string `json:"pin_set_sha256"`
	PolicySHA256    string `json:"policy_sha256"`
}

// Reference is one exact catalogue and rustc/cargo tuple approved by repository policy.
type Reference struct {
	CatalogueSHA256 string
	TupleSHA256     string
	RustcRelease    string
	CargoRelease    string
	UpstreamCommit  string
}

// Runtime contains the exact reader scope and runtime identities from one approved tuple.
type Runtime struct {
	TupleSHA256            string
	ManifestSHA256         string
	DriverSHA256           string
	CP949SHA256            string
	ShimTemplateSHA256     string
	Commit                 string
	RuntimeLanguageVersion uint32
	RuntimeMinCompatible   uint32
	Logger                 bool
	Closure                bool
}

// Store has no exported mutable fields; object reads return fresh byte slices.
type Store struct {
	identity      Identity
	objects       map[string]objectInfo
	references    map[string][]Reference
	runtimes      map[string]Runtime
	referenceBase Reference
	runtimeBase   Runtime
	files         fs.FS
}

// Repository returns the validated embedded default set. Invalid data is never accepted.
func Repository() (*Store, error) {
	repositoryOnce.Do(func() { repositoryData, repositoryErr = load(repositoryFiles) })
	return repositoryData, repositoryErr
}

// Identity reports the reviewed data source and its content addresses.
func (s *Store) Identity() Identity { return s.identity }

// Reference returns the stable repository fallback for calls without caller Pins.
func (s *Store) Reference() (Reference, error) {
	if s == nil || s.referenceBase.TupleSHA256 == "" {
		return Reference{}, errors.New("repository defaults unavailable")
	}
	return s.referenceBase, nil
}

// ReferenceFor returns a representative tuple for a catalogue; tool checks use the exact pair.
func (s *Store) ReferenceFor(catalogueSHA256 string) (Reference, bool) {
	if s == nil {
		return Reference{}, false
	}
	refs := s.references[catalogueSHA256]
	if len(refs) == 1 {
		return refs[0], true
	}
	if len(refs) > 1 && catalogueSHA256 == s.referenceBase.CatalogueSHA256 {
		return s.referenceBase, true
	}
	if len(refs) > 1 {
		return refs[0], true
	}
	return Reference{}, false
}

// ReferenceForToolchain selects the exact approved catalogue and indivisible rustc/cargo pair.
func (s *Store) ReferenceForToolchain(catalogueSHA256, rustcRelease, cargoRelease string) (Reference, bool) {
	if s == nil {
		return Reference{}, false
	}
	var found Reference
	for _, r := range s.references[catalogueSHA256] {
		if r.RustcRelease == rustcRelease && r.CargoRelease == cargoRelease {
			if found.TupleSHA256 != "" {
				return Reference{}, false
			}
			found = r
		}
	}
	return found, found.TupleSHA256 != ""
}

// ReferenceReleaseAllowed tests one recorded tool version against complete approved tuples.
func (s *Store) ReferenceReleaseAllowed(catalogueSHA256, tool, release string) bool {
	if s == nil {
		return false
	}
	for _, r := range s.references[catalogueSHA256] {
		if tool == "rustc" && r.RustcRelease == release || tool == "cargo" && r.CargoRelease == release {
			return true
		}
	}
	return false
}

// ReferenceUpstreamAllowed reports whether repository policy approves the commit.
func (s *Store) ReferenceUpstreamAllowed(commit string) bool {
	if s == nil {
		return false
	}
	for _, references := range s.references {
		for _, r := range references {
			if r.UpstreamCommit == commit {
				return true
			}
		}
	}
	return false
}

// Runtime returns the stable repository fallback runtime tuple.
func (s *Store) Runtime() (Runtime, error) {
	if s == nil || s.runtimeBase.TupleSHA256 == "" {
		return Runtime{}, errors.New("repository defaults unavailable")
	}
	return s.runtimeBase, nil
}

// RuntimeFor looks up an approved manifest and preserves its declared reader scope.
func (s *Store) RuntimeFor(manifestSHA256 string) (Runtime, bool) {
	if s == nil {
		return Runtime{}, false
	}
	r, ok := s.runtimes[manifestSHA256]
	return r, ok
}

// LoggerAllowed reports whether repository policy permits logger decoding for the manifest.
func (s *Store) LoggerAllowed(manifestSHA256 string) bool {
	r, ok := s.RuntimeFor(manifestSHA256)
	return ok && r.Logger
}

// DefaultRuntimeManifest returns the stable runtime fallback, or empty when data is invalid.
func (s *Store) DefaultRuntimeManifest() string {
	r, err := s.Runtime()
	if err != nil {
		return ""
	}
	return r.ManifestSHA256
}

// Object returns a verified copy of a material in the approved default set.
func (s *Store) Object(sha256Hex string) ([]byte, error) {
	if s == nil {
		return nil, errors.New("repository defaults unavailable")
	}
	if _, ok := s.objects[sha256Hex]; !ok {
		return nil, errors.New("object is not approved by repository defaults")
	}
	raw, err := fs.ReadFile(s.files, "objects/"+sha256Hex)
	if err != nil || !validDigest(raw, sha256Hex) {
		return nil, errors.New("repository default object integrity failure")
	}
	return raw, nil
}

func decode(raw []byte, out any) error {
	return jsonv2.Unmarshal(raw, out, jsonv2.RejectUnknownMembers(true), jsontext.AllowDuplicateNames(false))
}

func digest(raw []byte) string {
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func validDigest(raw []byte, want string) bool { return len(want) == 64 && digest(raw) == want }

func load(files fs.FS) (*Store, error) {
	stage := "selection"
	fail := func() (*Store, error) { return nil, fmt.Errorf("invalid repository default set: %s", stage) }
	pointerRaw, err := fs.ReadFile(files, "repository-defaults.json")
	if err != nil {
		return fail()
	}
	var selected pointer
	if decode(pointerRaw, &selected) != nil || selected.Schema != "tsgk-repository-default-selection/r1" || selected.ApprovalSource != DefaultApprovalSource || !validHash(selected.SetSHA256) || !validHash(selected.ReferenceFallbackCatalogue) || !validHash(selected.ReferenceFallbackTuple) || !validHash(selected.RuntimeFallbackManifest) || !validHash(selected.ReferenceObligations) || !validHash(selected.RuntimeObligations) || !validHash(selected.LegacyLoggerSetSHA256) {
		return fail()
	}
	stage = "set"
	setRaw, err := fs.ReadFile(files, "default-sets/"+selected.SetSHA256+".json")
	if err != nil || !validDigest(setRaw, selected.SetSHA256) {
		return fail()
	}
	var set setFile
	if decode(setRaw, &set) != nil || set.Schema != defaultSetSchema || set.Reader != defaultReader || !validHash(set.PolicySHA256) || set.PolicyBytes == 0 {
		return fail()
	}
	stage = "policy"
	policyRaw, err := fs.ReadFile(files, "objects/"+set.PolicySHA256)
	if err != nil || uint64(len(policyRaw)) != set.PolicyBytes || !validDigest(policyRaw, set.PolicySHA256) {
		return fail()
	}
	var p policy
	if decode(policyRaw, &p) != nil || p.Schema != defaultPolicySchema || p.Reader != defaultReader || p.ApprovalSource != DefaultApprovalSource || len(p.References) == 0 || len(p.Runtimes) == 0 || p.ReferenceFallbackCatalogue != selected.ReferenceFallbackCatalogue || p.ReferenceFallbackTuple != selected.ReferenceFallbackTuple || p.RuntimeFallbackManifest != selected.RuntimeFallbackManifest || p.ReferenceObligations != selected.ReferenceObligations || p.RuntimeObligations != selected.RuntimeObligations {
		return fail()
	}
	stage = "objects"
	objects := make(map[string]objectInfo, len(set.Objects))
	for _, object := range set.Objects {
		if !validHash(object.SHA256) || object.Bytes == 0 || objects[object.SHA256].SHA256 != "" {
			return fail()
		}
		raw, err := fs.ReadFile(files, "objects/"+object.SHA256)
		if err != nil || uint64(len(raw)) != object.Bytes || !validDigest(raw, object.SHA256) {
			return fail()
		}
		objects[object.SHA256] = object
	}
	store := &Store{identity: Identity{ApprovalSource: selected.ApprovalSource, SelectionSHA256: digest(pointerRaw), PinSetSHA256: selected.SetSHA256, PolicySHA256: set.PolicySHA256}, objects: objects, files: files,
		references: map[string][]Reference{}, runtimes: map[string]Runtime{}}
	stage = "reference obligations"
	refObligationsRaw, err := approvedObject(files, objects, p.ReferenceObligations)
	if err != nil {
		return fail()
	}
	var referenceObligations map[string]jsontext.Value
	if decode(refObligationsRaw, &referenceObligations) != nil || len(referenceObligations) == 0 {
		return fail()
	}
	stage = "runtime obligations"
	runtimeObligationsRaw, err := approvedObject(files, objects, p.RuntimeObligations)
	if err != nil {
		return fail()
	}
	var runtimeObligations runtimeReaderObligations
	if decode(runtimeObligationsRaw, &runtimeObligations) != nil || len(runtimeObligations.Files) == 0 || len(runtimeObligations.Patches) == 0 || runtimeObligations.Shim == "" {
		return fail()
	}
	stage = "legacy logger set"
	legacyLoggerRaw, err := approvedObject(files, objects, selected.LegacyLoggerSetSHA256)
	if err != nil {
		return fail()
	}
	var legacyLogger legacyLoggerSet
	if decode(legacyLoggerRaw, &legacyLogger) != nil || legacyLogger.Schema != "tsgk-repository-legacy-logger-set/r1" || legacyLogger.Reader != loggerReader || len(legacyLogger.Manifests) == 0 {
		return fail()
	}
	stage = "references"
	for _, entry := range p.References {
		if !validHash(entry.CatalogueSHA256) || !validHash(entry.TupleSHA256) {
			return fail()
		}
		tupleRaw, err := approvedObject(files, objects, entry.TupleSHA256)
		if err != nil {
			return fail()
		}
		var tuple referenceTuple
		if decode(tupleRaw, &tuple) != nil || tuple.Schema != referenceTupleSchema || tuple.PathReader != referenceReader || !versionFormat.MatchString(tuple.RustcRelease) || !versionFormat.MatchString(tuple.CargoRelease) || tuple.CatalogueSHA256 != entry.CatalogueSHA256 {
			return fail()
		}
		catalogueRaw, err := approvedObject(files, objects, entry.CatalogueSHA256)
		if err != nil {
			return fail()
		}
		if !normalizedCatalogueMatches(catalogueRaw, refObligationsRaw) {
			return fail()
		}
		var catalogue struct {
			UpstreamCommit string `json:"upstream_commit"`
		}
		var upstream string
		if jsonv2.Unmarshal(catalogueRaw, &catalogue, jsontext.AllowDuplicateNames(false)) != nil {
			return fail()
		}
		upstream = catalogue.UpstreamCommit
		if !commitFormat.MatchString(upstream) {
			return fail()
		}
		r := Reference{CatalogueSHA256: entry.CatalogueSHA256, TupleSHA256: entry.TupleSHA256, RustcRelease: tuple.RustcRelease, CargoRelease: tuple.CargoRelease, UpstreamCommit: upstream}
		for _, previous := range store.references[entry.CatalogueSHA256] {
			if previous.TupleSHA256 == entry.TupleSHA256 || previous.RustcRelease == r.RustcRelease && previous.CargoRelease == r.CargoRelease {
				return fail()
			}
		}
		store.references[entry.CatalogueSHA256] = append(store.references[entry.CatalogueSHA256], r)
	}
	if !validHash(p.ReferenceFallbackCatalogue) || !validHash(p.ReferenceFallbackTuple) {
		return fail()
	}
	for _, candidate := range store.references[p.ReferenceFallbackCatalogue] {
		if candidate.TupleSHA256 == p.ReferenceFallbackTuple {
			store.referenceBase = candidate
		}
	}
	if store.referenceBase.TupleSHA256 == "" {
		return fail()
	}
	stage = "runtimes"
	for _, entry := range p.Runtimes {
		if !validHash(entry.TupleSHA256) || !validHash(entry.ManifestSHA256) || store.runtimes[entry.ManifestSHA256].TupleSHA256 != "" {
			return fail()
		}
		tupleRaw, err := approvedObject(files, objects, entry.TupleSHA256)
		if err != nil {
			return fail()
		}
		var tuple runtimeTuple
		if decode(tupleRaw, &tuple) != nil || tuple.Schema != runtimeTupleSchema || tuple.Reader != runtimeReader || tuple.LoggerReader != loggerReader || tuple.ManifestSHA256 != entry.ManifestSHA256 || !validHash(tuple.DriverSHA256) || !validHash(tuple.CP949SHA256) || !validHash(tuple.ShimTemplateSHA256) || tuple.RuntimeMinCompatible == 0 || tuple.RuntimeMinCompatible > tuple.RuntimeLanguageVersion {
			return fail()
		}
		manifestRaw, err := approvedObject(files, objects, entry.ManifestSHA256)
		if err != nil {
			return fail()
		}
		var manifest runtimeManifest
		if jsonv2.Unmarshal(manifestRaw, &manifest, jsontext.AllowDuplicateNames(false)) != nil || manifest.Schema != "tsgk-native-runtime/r1" || manifest.Repository != "tree-sitter/tree-sitter" || !commitFormat.MatchString(manifest.Commit) {
			return fail()
		}
		if entry.Closure {
			paths, patches := make([]string, 0, len(manifest.Files)), make([]string, 0, len(manifest.Patches))
			for _, file := range manifest.Files {
				paths = append(paths, file.Path)
			}
			for _, patch := range manifest.Patches {
				patches = append(patches, patch.Subject)
			}
			if !reflect.DeepEqual(paths, runtimeObligations.Files) || !reflect.DeepEqual(patches, runtimeObligations.Patches) {
				return fail()
			}
			shim, err := approvedObject(files, objects, tuple.ShimTemplateSHA256)
			if err != nil || string(shim) != runtimeObligations.Shim {
				return fail()
			}
			for _, material := range []string{tuple.DriverSHA256, tuple.CP949SHA256, tuple.ShimTemplateSHA256} {
				if _, err := approvedObject(files, objects, material); err != nil {
					return fail()
				}
			}
		}
		r := Runtime{TupleSHA256: entry.TupleSHA256, ManifestSHA256: entry.ManifestSHA256, DriverSHA256: tuple.DriverSHA256, CP949SHA256: tuple.CP949SHA256,
			ShimTemplateSHA256: tuple.ShimTemplateSHA256, Commit: manifest.Commit, RuntimeLanguageVersion: tuple.RuntimeLanguageVersion, RuntimeMinCompatible: tuple.RuntimeMinCompatible, Logger: entry.Logger, Closure: entry.Closure}
		if r.Closure && !r.Logger {
			return fail()
		}
		store.runtimes[entry.ManifestSHA256] = r
	}
	var ok bool
	store.runtimeBase, ok = store.runtimes[p.RuntimeFallbackManifest]
	if !ok || !store.runtimeBase.Closure || !store.runtimeBase.Logger || len(store.references) == 0 || len(store.runtimes) == 0 {
		return fail()
	}
	for _, manifest := range legacyLogger.Manifests {
		runtime, ok := store.runtimes[manifest]
		if !validHash(manifest) || !ok || !runtime.Logger {
			return fail()
		}
	}
	return store, nil
}

func normalizedCatalogueMatches(catalogueRaw, baselineRaw []byte) bool {
	var catalogue, baseline map[string]any
	if jsonv2.Unmarshal(catalogueRaw, &catalogue, jsontext.AllowDuplicateNames(false)) != nil ||
		jsonv2.Unmarshal(baselineRaw, &baseline, jsontext.AllowDuplicateNames(false)) != nil || len(catalogue) == 0 || len(baseline) == 0 {
		return false
	}
	for _, key := range []string{"upstream_commit", "archive_sha256", "binding_sha256", "controller_sha256", "reference_inputs"} {
		value, ok := baseline[key]
		if !ok {
			return false
		}
		if _, ok := catalogue[key]; !ok {
			return false
		}
		catalogue[key] = value
	}
	return reflect.DeepEqual(catalogue, baseline)
}

func approvedObject(files fs.FS, objects map[string]objectInfo, sha string) ([]byte, error) {
	info, ok := objects[sha]
	if !ok {
		return nil, errors.New("unlisted object")
	}
	raw, err := fs.ReadFile(files, "objects/"+sha)
	if err != nil || uint64(len(raw)) != info.Bytes || !validDigest(raw, sha) {
		return nil, errors.New("object digest mismatch")
	}
	return raw, nil
}

func validHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.Trim(value, "0123456789abcdef") == ""
}
