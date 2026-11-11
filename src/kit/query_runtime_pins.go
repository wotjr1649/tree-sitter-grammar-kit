package kit

import (
	_ "embed"
	"fmt"
	"reflect"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// This inventory belongs to the reader, independently of production selection.
//
//go:embed query_runtime_obligations_r1.json
var runtimeObligationsR1 []byte

const runtimeClosureReader = "tsgk-runtime-closure/r1"

var runtimePinRolesR1 = [...]struct{ name, role string }{
	{"native/runtime-tuple.json", "runtime-tuple"},
	{"native/runtime-manifest.json", "runtime-manifest"},
	{"native/driver.c", "driver"},
	{"native/cp949_table.h", "cp949"},
	{"native/shim-template.c", "shim-template"},
	{"native/runtime-field-lookup.patch.json", "runtime-patch"},
	{"native/runtime-navigation.patch.json", "runtime-patch"},
	{"native/runtime-sibling-identity.patch.json", "runtime-patch"},
}

type eligibilityRuntimeTuple struct {
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

// QueryRuntimePinIdentity records the approved runtime interpretation actually consumed.
type QueryRuntimePinIdentity struct {
	TupleSHA256            string `json:"tuple_sha256"`
	ManifestSHA256         string `json:"manifest_sha256"`
	Reader                 string `json:"reader"`
	LoggerReader           string `json:"logger_reader"`
	RuntimeLanguageVersion uint32 `json:"runtime_language_version"`
	RuntimeMinCompatible   uint32 `json:"runtime_min_compatible"`
}

type eligibilityRuntimeManifest struct {
	Schema     string `json:"schema"`
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	URL        string `json:"url"`
	Note       string `json:"note"`
	Patches    []struct {
		Subject       string `json:"subject"`
		SHA256        string `json:"sha256"`
		Bytes         uint64 `json:"bytes"`
		Pointer       string `json:"pointer"`
		Field         string `json:"field"`
		Target        string `json:"target"`
		BeforeSHA256  string `json:"before_sha256"`
		BeforeBytes   uint64 `json:"before_bytes"`
		License       string `json:"license"`
		RemoveWhen    string `json:"remove_when"`
		Origin        string `json:"origin"`
		TrackingIssue string `json:"tracking_issue"`
	} `json:"patches"`
	Files []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Bytes  uint64 `json:"bytes"`
	} `json:"files"`
}

type eligibilityRuntimePins struct {
	tuple         eligibilityRuntimeTuple
	manifest      eligibilityRuntimeManifest
	logger        reuseobs.RuntimePin
	driver, cp949 eligibilitySnapshotRow
	shim          string
}

func (b *eligibilityBudget) runtimeManifest() string {
	if b.runtime != nil {
		return b.runtime.tuple.ManifestSHA256
	}
	return reuseobs.RuntimeManifestSHA256
}

func (b *eligibilityBudget) runtimeMatches(manifest string) bool {
	if b.runtime != nil {
		return manifest == b.runtime.tuple.ManifestSHA256
	}
	return reuseobs.VerifiedRuntimeManifest(manifest)
}

func (b *eligibilityBudget) runtimeLogger() *reuseobs.RuntimePin {
	if b.runtime != nil {
		return &b.runtime.logger
	}
	return nil
}

func (b *eligibilityBudget) runtimeABI(abi eligibilityProducerABI) bool {
	if b.runtime != nil {
		return abi.RuntimeLanguageVersion == b.runtime.tuple.RuntimeLanguageVersion && abi.RuntimeMinCompatible == b.runtime.tuple.RuntimeMinCompatible
	}
	return abi.RuntimeLanguageVersion == 15 && abi.RuntimeMinCompatible == 13
}

func (s *eligibilitySession) runtimePins(req *EvidencePinsRequest) *Error {
	if req == nil {
		return nil
	}
	const tupleName = "native/runtime-tuple.json"
	materials := map[string]EvidencePinMaterial{}
	for _, m := range s.res.Pins.Materials {
		if e := s.b.charge(1, 128, "runtime-pin-index"); e != nil {
			return e
		}
		if m.Role == "runtime-tuple" && m.Name != tupleName {
			return fail(KindInvalidInput, "PIN_RUNTIME_ROLE", m.Name, nil)
		}
		materials[m.Name] = m
	}
	if _, selected := materials[tupleName]; !selected {
		return nil
	}
	material := func(name, role string) ([]byte, bool) {
		m, ok := materials[name]
		if !ok {
			s.bound = false
			s.finding("PIN_RUNTIME_MISSING", name, false)
			return nil, false
		}
		if m.Role != role {
			s.bound = false
			s.finding("PIN_RUNTIME_ROLE", name, true)
			return nil, false
		}
		return req.Materials[name], true
	}
	data, ok := material(tupleName, "runtime-tuple")
	if !ok {
		return nil
	}
	p := &eligibilityRuntimePins{}
	if e := s.b.decode("runtime-tuple", data, &p.tuple, true); e != nil {
		return e
	}
	t := p.tuple
	if t.Schema != "tsgk-runtime-pin-tuple/r1" || t.Reader != runtimeClosureReader || t.LoggerReader != "tsgk-runtime-logger/r1" {
		return fail(KindUnsupported, "PIN_RUNTIME_READER", tupleName, nil)
	}
	if !hex64.MatchString(t.ManifestSHA256) || !hex64.MatchString(t.DriverSHA256) || !hex64.MatchString(t.CP949SHA256) || !hex64.MatchString(t.ShimTemplateSHA256) || t.RuntimeMinCompatible == 0 || t.RuntimeMinCompatible > t.RuntimeLanguageVersion {
		return fail(KindInvalidInput, "PIN_RUNTIME_TUPLE", tupleName, nil)
	}
	for _, item := range []struct{ name, role, sha string }{
		{"native/runtime-manifest.json", "runtime-manifest", t.ManifestSHA256},
		{"native/driver.c", "driver", t.DriverSHA256},
		{"native/cp949_table.h", "cp949", t.CP949SHA256},
		{"native/shim-template.c", "shim-template", t.ShimTemplateSHA256},
	} {
		if _, ok := material(item.name, item.role); ok && (materials[item.name].SHA256 != item.sha || materials[item.name].Bytes == 0) {
			s.bound = false
			s.finding("PIN_RUNTIME_BINDING", item.name, true)
		}
	}
	if !s.bound {
		return nil
	}
	if e := s.b.decode("historical-runtime-manifest", req.Materials["native/runtime-manifest.json"], &p.manifest, true); e != nil {
		return e
	}
	m := p.manifest
	if m.Schema != "tsgk-native-runtime/r1" || m.Repository != "tree-sitter/tree-sitter" || !eligibilityRevision(m.Commit) {
		return fail(KindInvalidInput, "PIN_RUNTIME_MANIFEST", "native/runtime-manifest.json", nil)
	}
	var obligations struct {
		Files   []string `json:"files"`
		Patches []string `json:"patches"`
		Shim    string   `json:"shim"`
	}
	if e := s.b.decode("runtime-reader-obligations", runtimeObligationsR1, &obligations, true); e != nil {
		return e
	}
	paths, patches := []string{}, []string{}
	for _, row := range m.Files {
		if e := s.b.charge(1, uint64(len(row.Path))+128, "runtime-inventory"); e != nil {
			return e
		}
		if !portable(row.Path, false) || !hex64.MatchString(row.SHA256) || row.Bytes == 0 || row.Bytes > MaxDocumentBytes {
			return fail(KindInvalidInput, "PIN_RUNTIME_FILE", row.Path, nil)
		}
		paths = append(paths, row.Path)
	}
	for _, patch := range m.Patches {
		if e := s.b.charge(1, 256, "runtime-patch"); e != nil {
			return e
		}
		patches = append(patches, patch.Subject)
		name := "native/" + strings.TrimPrefix(patch.Subject, "src/drivers/native-c/")
		if _, ok := material(name, "runtime-patch"); ok && (materials[name].SHA256 != patch.SHA256 || materials[name].Bytes != patch.Bytes) {
			s.bound = false
			s.finding("PIN_RUNTIME_PATCH", name, true)
		}
		if patch.Pointer != "/files/0" || patch.Field != "operations" || patch.Target != "lib/src/node.c" || !hex64.MatchString(patch.BeforeSHA256) || patch.BeforeBytes == 0 {
			s.bound = false
			s.finding("PIN_RUNTIME_PATCH", name, true)
		}
	}
	if e := s.b.charge(uint64(len(req.Materials["native/shim-template.c"]))+uint64(len(runtimeObligationsR1)), 0, "runtime-obligations"); e != nil {
		return e
	}
	if !reflect.DeepEqual(paths, obligations.Files) || !reflect.DeepEqual(patches, obligations.Patches) || string(req.Materials["native/shim-template.c"]) != obligations.Shim {
		s.bound = false
		s.finding("PIN_RUNTIME_OBLIGATIONS", "historical-runtime-manifest", true)
	}
	if !s.bound {
		return nil
	}
	logger, err := reuseobs.NewRuntimePin(t.ManifestSHA256, t.LoggerReader)
	if err != nil {
		return fail(KindUnsupported, "PIN_RUNTIME_READER", tupleName, err)
	}
	p.logger, p.shim = logger, obligations.Shim
	p.driver = eligibilitySnapshotRow{Bytes: materials["native/driver.c"].Bytes, SHA256: t.DriverSHA256}
	p.cp949 = eligibilitySnapshotRow{Bytes: materials["native/cp949_table.h"].Bytes, SHA256: t.CP949SHA256}
	s.runtime, s.b.runtime = p, p
	s.res.RuntimePin = &QueryRuntimePinIdentity{TupleSHA256: materials[tupleName].SHA256, ManifestSHA256: t.ManifestSHA256, Reader: t.Reader, LoggerReader: t.LoggerReader, RuntimeLanguageVersion: t.RuntimeLanguageVersion, RuntimeMinCompatible: t.RuntimeMinCompatible}
	return nil
}

func (b *eligibilityBudget) historicalNativeClosure(symbol string) (map[string]eligibilitySnapshotRow, string, *Error) {
	if !validID(symbol) {
		return nil, "", fail(KindInvalidInput, "PIN_RUNTIME_SYMBOL", "native", nil)
	}
	p := b.runtime
	if e := b.charge(uint64(len(p.manifest.Files))+3, 256*uint64(len(p.manifest.Files)+3)+4*uint64(len(p.shim)+2*len(symbol)), "historical-native-static"); e != nil {
		return nil, "", e
	}
	files := map[string]eligibilitySnapshotRow{}
	for _, row := range p.manifest.Files {
		files["runtime "+row.Path] = eligibilitySnapshotRow{Bytes: row.Bytes, SHA256: row.SHA256}
	}
	files["driver driver/driver.c"], files["driver driver/cp949_table.h"] = p.driver, p.cp949
	shim := []byte(fmt.Sprintf(p.shim, symbol, symbol))
	files["driver driver/shim.c"] = eligibilitySnapshotRow{Bytes: uint64(len(shim)), SHA256: digestHex(shim)}
	return files, p.manifest.Commit, nil
}
