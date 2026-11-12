package kit

import (
	"bytes"
	_ "embed"
	"reflect"
	"regexp"

	evidencepins "github.com/wotjr1649/tree-sitter-grammar-kit/src/contracts/evidence-pins"
)

// This reader's obligations stay fixed when the production catalogue changes.
//
//go:embed query_eligibility_obligations_r1.json
var eligibilityObligationsR1 []byte

type eligibilityReferenceTuple struct {
	Schema          string `json:"schema"`
	RustcRelease    string `json:"rustc_release"`
	CargoRelease    string `json:"cargo_release"`
	PathReader      string `json:"path_reader"`
	CatalogueSHA256 string `json:"catalogue_sha256"`
}

var referenceRelease = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func (s *eligibilitySession) referencePins(req QueryReferenceEligibilityRequest) *Error {
	if req.Pins == nil {
		store, err := evidencepins.Repository()
		if err != nil {
			return fail(KindInvalidInput, "REPOSITORY_DEFAULTS_INVALID", "repository-defaults", err)
		}
		ref, err := store.Reference()
		if err != nil {
			return fail(KindInvalidInput, "REPOSITORY_DEFAULTS_INVALID", "reference", err)
		}
		if selected, ok := store.ReferenceFor(s.reg.CatalogueSHA256); ok {
			ref = selected
		}
		data, err := store.Object(ref.CatalogueSHA256)
		if err != nil {
			return fail(KindInvalidInput, "REPOSITORY_DEFAULTS_INVALID", "catalogue", err)
		}
		if e := s.b.charge(1, uint64(len(data)), "catalogue-digest"); e != nil {
			return e
		}
		s.res.CatalogueSHA256 = digestHex(data)
		s.referenceTuple = &eligibilityReferenceTuple{RustcRelease: ref.RustcRelease, CargoRelease: ref.CargoRelease}
		return s.b.decode("catalogue", data, &s.catalogue, true)
	}
	s.res.ResultSchema = QueryReferenceEligibilityPinsResultSchema
	pins, err := resolveEvidencePins(*req.Pins, s.b)
	s.res.Pins = &pins
	findings := pins.Findings
	if err != nil && len(findings) > 0 {
		// The resolver's deferred failReport appends the typed stop as its last finding; it is returned as an error, not a contradiction.
		findings = findings[:len(findings)-1]
	}
	for _, f := range findings {
		s.finding(f.Code, f.Path, f.Severity == "error")
	}
	if pins.Assessment == AssessFail && s.res.Assessment != AssessFail {
		s.finding("PIN_REFERENCE_MATERIALS", "pins", true)
	}
	if err != nil {
		return asError(err)
	}
	if pins.Assessment != AssessPass {
		return nil
	}
	if e := s.b.charge(1, uint64(len(req.Approval)), "pin-eligibility-subject"); e != nil {
		return e
	}
	if !bytes.Equal(req.Pins.Subject, req.Approval) {
		s.bound = false
		s.finding("PIN_ELIGIBILITY_SUBJECT", "pin-subject", true)
		return nil
	}
	byName := map[string]EvidencePinMaterial{}
	for _, m := range pins.Materials {
		if e := s.b.charge(1, 128, "reference-pin-index"); e != nil {
			return e
		}
		if m.Role == "reference-catalogue" && m.Name != "reference/catalogue.json" || m.Role == "reference-tuple" && m.Name != "reference/tuple.json" {
			return fail(KindInvalidInput, "PIN_REFERENCE_ROLE", m.Name, nil)
		}
		byName[m.Name] = m
	}
	for _, item := range []struct{ name, role string }{{"reference/catalogue.json", "reference-catalogue"}, {"reference/tuple.json", "reference-tuple"}} {
		name, role := item.name, item.role
		m, ok := byName[name]
		if !ok {
			s.bound = false
			s.finding("PIN_REFERENCE_MISSING", name, false)
		} else if m.Role != role {
			return fail(KindInvalidInput, "PIN_REFERENCE_ROLE", name, nil)
		}
	}
	if !s.bound {
		return nil
	}
	var tuple eligibilityReferenceTuple
	if e := s.b.decode("reference-tuple", req.Pins.Materials["reference/tuple.json"], &tuple, true); e != nil {
		return e
	}
	if tuple.Schema != "tsgk-reference-pin-tuple/r1" || tuple.PathReader != "rust-path-1.97.1/r1" {
		return fail(KindUnsupported, "PIN_REFERENCE_READER", "reference-tuple", nil)
	}
	if !referenceRelease.MatchString(tuple.RustcRelease) || !referenceRelease.MatchString(tuple.CargoRelease) || !hex64.MatchString(tuple.CatalogueSHA256) {
		return fail(KindInvalidInput, "PIN_REFERENCE_TUPLE", "reference-tuple", nil)
	}
	s.res.CatalogueSHA256 = byName["reference/catalogue.json"].SHA256
	s.res.ReferenceTupleSHA256 = byName["reference/tuple.json"].SHA256
	s.res.ReferencePathReader = tuple.PathReader
	if tuple.CatalogueSHA256 != s.res.CatalogueSHA256 {
		s.bound = false
		s.finding("PIN_REFERENCE_CATALOGUE", "reference-tuple", true)
		return nil
	}
	data := req.Pins.Materials["reference/catalogue.json"]
	if e := s.b.decode("historical-catalogue", data, &s.catalogue, true); e != nil {
		return e
	}
	valid, e := s.b.referenceCatalogueObligations(s.catalogue, uint64(len(data)))
	if e != nil {
		return e
	}
	if !valid {
		s.bound = false
		s.finding("PIN_CATALOGUE_OBLIGATIONS", "historical-catalogue", true)
		return nil
	}
	if !eligibilityRevision(s.catalogue.UpstreamCommit) || !hex64.MatchString(s.catalogue.ArchiveSHA256) || !hex64.MatchString(s.catalogue.BindingSHA256) || len(s.catalogue.ReferenceInputs) != 5 || len(s.catalogue.ControllerSHA256) != 3 {
		return fail(KindInvalidInput, "PIN_CATALOGUE_IDENTITY", "historical-catalogue", nil)
	}
	for _, name := range []string{"Cargo.toml", "main.rs", "instrumentation.patch", "build.py", "checks.py", "check.py"} {
		sha := s.catalogue.ReferenceInputs[name]
		if name == "check.py" {
			sha = s.catalogue.ControllerSHA256[name]
		}
		m, ok := byName["reference/"+name]
		if !ok {
			s.bound = false
			s.finding("PIN_REFERENCE_MISSING", "reference/"+name, false)
			continue
		}
		if m.Role != "reference-input" || !hex64.MatchString(sha) || m.SHA256 != sha || (name == "build.py" || name == "checks.py") && s.catalogue.ControllerSHA256[name] != sha {
			s.bound = false
			s.finding("PIN_REFERENCE_INPUT", "reference/"+name, true)
		}
	}
	s.referenceTuple = &tuple
	return nil
}

func (b *eligibilityBudget) referenceCatalogueObligations(catalogue eligibilityCatalogue, catalogueBytes uint64) (bool, *Error) {
	var obligations eligibilityCatalogue
	if e := b.decode("reader-obligations", eligibilityObligationsR1, &obligations, true); e != nil {
		return false, e
	}
	// Only independently bound provenance may vary; rules, inputs and limits belong to the reader.
	catalogue.UpstreamCommit, catalogue.ArchiveSHA256, catalogue.BindingSHA256 = "", "", ""
	catalogue.ControllerSHA256, catalogue.ReferenceInputs = map[string]string{}, map[string]string{}
	if e := b.charge(catalogueBytes+uint64(len(eligibilityObligationsR1)), 0, "catalogue-obligations"); e != nil {
		return false, e
	}
	return reflect.DeepEqual(catalogue, obligations), nil
}
