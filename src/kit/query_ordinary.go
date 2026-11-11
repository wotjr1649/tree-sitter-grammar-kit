package kit

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"io"
	"reflect"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

const OrdinaryContext = "ordinary-batch-full-input-utf8/r1"

// OrdinaryPin binds caller-selected bytes; it is not an approval.
type OrdinaryPin = queryregistrycore.Pin
type OrdinaryCoordinate = queryregistrycore.Coordinate

type OrdinaryLimits struct {
	FileBytes  uint64 `json:"file_bytes"`
	ReadBytes  uint64 `json:"read_bytes"`
	Work       uint64 `json:"work"`
	Logical    uint64 `json:"logical"`
	Entries    uint64 `json:"entries"`
	WallMillis uint64 `json:"wall_millis"`
}

func OrdinaryPlanLimits() OrdinaryLimits {
	return OrdinaryLimits{16 << 20, 256 << 20, 32000000, 1 << 30, 10000, 120000}
}

type OrdinaryMember struct {
	Owner       string      `json:"owner"`
	Path        string      `json:"path"`
	Role        string      `json:"role"`
	CaseOrBatch string      `json:"case_or_batch"`
	Pin         OrdinaryPin `json:"pin"`
}

type OrdinaryBinding struct {
	Route           string           `json:"route"`
	Platform        string           `json:"platform_id"`
	Subject         string           `json:"subject_platform"`
	ClosureEncoding string           `json:"closure_encoding"`
	Grammar         []NativeInput    `json:"grammar"`
	Runtime         OrdinaryPin      `json:"runtime"`
	Members         []OrdinaryMember `json:"members"`
}

// OrdinaryProfile is a preregistered planning profile, not an executable oracle profile.
type OrdinaryProfile struct {
	Schema          string            `json:"schema"`
	Context         string            `json:"context"`
	Candidate       string            `json:"candidate"`
	SemanticCohort  string            `json:"semantic_cohort"`
	NativePolicy    OrdinaryPin       `json:"native_policy"`
	Limits          OrdinaryLimits    `json:"limits"`
	InheritedLimits OrdinaryLimits    `json:"inherited_limits"`
	Hosts           []OrdinaryHost    `json:"hosts"`
	Bindings        []OrdinaryBinding `json:"bindings"`
}

type OrdinaryHost struct {
	Platform string `json:"platform_id"`
	Subject  string `json:"subject_platform"`
}

type OrdinaryAsset struct {
	ID                string       `json:"id"`
	Role              string       `json:"role"`
	State             string       `json:"state"`
	Pin               *OrdinaryPin `json:"pin"`
	AuthoringBasis    *OrdinaryPin `json:"authoring_basis_pin"`
	IndependentReview *OrdinaryPin `json:"independent_review_pin"`
}

type OrdinaryAssetRegistration struct {
	Schema         string          `json:"schema"`
	Registry       OrdinaryPin     `json:"registry_pin"`
	Candidate      string          `json:"candidate"`
	SemanticCohort string          `json:"semantic_cohort"`
	Assets         []OrdinaryAsset `json:"assets"`
}

type OrdinaryCase struct {
	ID               string        `json:"case_id"`
	OriginalSelector string        `json:"original_selector"`
	InitialSource    OrdinaryPin   `json:"initial_source_pin"`
	Edits            []Edit        `json:"edits"`
	VersionPins      []OrdinaryPin `json:"version_pins"`
	Points           []EditPoints  `json:"edit_points"`
}

type OrdinaryQuery struct {
	ID            string      `json:"id"`
	Source        OrdinaryPin `json:"source_pin"`
	MetadataAsset string      `json:"metadata_asset"`
	Kind          string      `json:"kind"`
}

type OrdinaryRequired struct {
	ID           string             `json:"obligation_id"`
	Coordinate   OrdinaryCoordinate `json:"coordinate"`
	TreeAsset    string             `json:"tree_asset"`
	CaptureAsset string             `json:"capture_asset"`
	FactAsset    string             `json:"fact_asset"`
}

type OrdinaryRegistration struct {
	Schema            string             `json:"schema"`
	Registry          OrdinaryPin        `json:"registry_pin"`
	Namespace         string             `json:"membership_namespace"`
	Inventory         OrdinaryPin        `json:"inventory_pin"`
	Slots             OrdinaryPin        `json:"slot_pin"`
	Profile           OrdinaryPin        `json:"profile_pin"`
	Assets            OrdinaryPin        `json:"asset_registration_pin"`
	Route             string             `json:"route"`
	Set               string             `json:"set"`
	Platform          string             `json:"platform_id"`
	Subject           string             `json:"subject_platform"`
	MembershipContext string             `json:"membership_context"`
	Context           string             `json:"context"`
	ClosureEncoding   string             `json:"closure_encoding"`
	Candidate         string             `json:"candidate"`
	SemanticCohort    string             `json:"semantic_cohort"`
	Cases             []OrdinaryCase     `json:"cases"`
	Queries           []OrdinaryQuery    `json:"queries"`
	Required          []OrdinaryRequired `json:"required"`
	PlannedMembers    []OrdinaryMember   `json:"planned_members"`
}

func ordinaryBad(code string) error { return fail(KindInvalidInput, code, "ordinary", nil) }

// Roundtrip shape comparison rejects absent keys and null/empty substitutions as well as unknown keys.
func ordinaryDecode(data []byte, out any) error {
	if len(data) > 16<<20 {
		return fail(KindResourceLimit, "ORDINARY_DOCUMENT_LIMIT", "ordinary", nil)
	}
	d := jsontext.NewDecoder(bytes.NewReader(data))
	for n := 0; ; n++ {
		_, err := d.ReadToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ordinaryBad("ORDINARY_JSON")
		}
		if d.StackDepth() > 64 || n > 2000000 {
			return fail(KindResourceLimit, "ORDINARY_SHAPE_LIMIT", "ordinary", nil)
		}
	}
	if err := jsonv2.Unmarshal(data, out, jsonv2.RejectUnknownMembers(true)); err != nil {
		return ordinaryBad("ORDINARY_JSON")
	}
	encoded, err := jsonv2.Marshal(out, jsonv2.FormatNilSliceAsNull(true), jsonv2.FormatNilMapAsNull(true))
	if err != nil {
		return ordinaryBad("ORDINARY_JSON")
	}
	var a, b any
	if jsonv2.Unmarshal(data, &a) != nil || jsonv2.Unmarshal(encoded, &b) != nil || !reflect.DeepEqual(a, b) {
		return ordinaryBad("ORDINARY_FIELD_SHAPE")
	}
	return nil
}

func ordinaryPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\:") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func ordinaryPin(p OrdinaryPin) bool {
	return ordinaryPath(p.Path) && p.Bytes <= 16<<20 && hex64.MatchString(p.SHA256)
}

func ParseOrdinaryProfile(data []byte) (OrdinaryProfile, error) {
	var p OrdinaryProfile
	if err := ordinaryDecode(data, &p); err != nil {
		return p, err
	}
	if p.Schema != "tsgk-oracle/r4" || p.Context != OrdinaryContext || p.Candidate == "" || p.SemanticCohort == "" || !ordinaryPin(p.NativePolicy) || p.Limits != OrdinaryPlanLimits() || p.InheritedLimits != p.Limits {
		return p, ordinaryBad("ORDINARY_PROFILE")
	}
	policy := []byte(NativePolicyText(NativeOperations()["native-query-batch"], OutputTree))
	if p.NativePolicy != queryregistrycore.FilePin(p.NativePolicy.Path, policy) {
		return p, ordinaryBad("ORDINARY_POLICY")
	}
	if !reflect.DeepEqual(p.Hosts, []OrdinaryHost{{"windows-amd64", "windows/amd64"}, {"linux-amd64", "linux/amd64"}}) {
		return p, ordinaryBad("ORDINARY_HOSTS")
	}
	seen := map[string]bool{}
	for _, b := range p.Bindings {
		key := b.Route + "/" + b.Platform
		if b.Route == "" || seen[key] || !ordinaryHost(b.Platform, b.Subject) || !ordinaryClosure(b.ClosureEncoding) || !ordinaryPin(b.Runtime) || len(b.Grammar) == 0 {
			return p, ordinaryBad("ORDINARY_BINDING")
		}
		seen[key] = true
		if err := ordinaryMembers(b.Members, b.ClosureEncoding); err != nil {
			return p, err
		}
	}
	if p.Bindings == nil {
		return p, ordinaryBad("ORDINARY_BINDING")
	}
	return p, nil
}

func ordinaryHost(platform, subject string) bool {
	return platform == "windows-amd64" && subject == "windows/amd64" || platform == "linux-amd64" && subject == "linux/amd64"
}
func ordinaryClosure(s string) bool {
	return s == "plain" || s == "parser-paged" || s == "binary-paged"
}

func ordinaryMembers(ms []OrdinaryMember, encoding string) error {
	if ms == nil {
		return ordinaryBad("ORDINARY_MEMBERS")
	}
	seen := map[string]bool{}
	for _, m := range ms {
		key := strings.ToLower(m.Path)
		if m.Owner == "" || m.CaseOrBatch == "" || !ordinaryPin(m.Pin) || m.Path != m.Pin.Path || seen[key] {
			return ordinaryBad("ORDINARY_MEMBER")
		}
		seen[key] = true
		if !ordinaryMemberRole(m.Role, encoding) {
			return ordinaryBad("ORDINARY_MEMBER_ROLE")
		}
	}
	return nil
}

func ordinaryMemberRole(role, encoding string) bool {
	switch role {
	case "grammar-paged-descriptor", "grammar-paged-part":
		return encoding == "parser-paged" || encoding == "binary-paged"
	case "driver-paged-descriptor", "driver-paged-part", "library-paged-descriptor", "library-paged-part":
		return encoding == "binary-paged"
	case "grammar", "native-driver", "reference-library", "source-witness", "native-request", "native-response", "native-batch-capture", "stderr", "expected-tree", "expected-captures", "expected-facts", "metadata", "build-control":
		return true
	}
	return false
}

func ordinaryValidateAssetBody(asset OrdinaryAsset, data []byte) error {
	if asset.Role != "metadata" {
		return nil
	}
	var m QueryWitnessMetadata
	if err := ordinaryDecode(data, &m); err != nil {
		return err
	}
	if m.Patterns == 0 || m.CaptureNames == nil || m.Predicates == nil || len(m.Predicates) != 0 {
		return ordinaryBad("ORDINARY_STRUCTURAL_METADATA")
	}
	seen := map[string]bool{}
	for _, name := range m.CaptureNames {
		if name == "" || seen[name] {
			return ordinaryBad("ORDINARY_STRUCTURAL_METADATA")
		}
		seen[name] = true
	}
	return nil
}

// Descriptor shape is checked here; guarded part hashing remains the future reader's job.
func ordinaryValidateMemberBody(m OrdinaryMember, data []byte) error {
	if m.Role == "grammar-paged-descriptor" {
		var d semanticParserDescriptor
		if err := ordinaryDecode(data, &d); err != nil {
			return err
		}
		if d.Schema != "tsgk-query-parser-pages/r1" || d.Owner != m.Owner || d.Logical.Role != "grammar-parser" || !ordinaryPath(d.Logical.Path) || d.Logical.Bytes <= 16<<20 || d.Logical.Bytes > BuildFileBytes || !hex64.MatchString(d.Logical.SHA256) || uint64(len(d.Parts)) != (d.Logical.Bytes+(16<<20)-1)/(16<<20) {
			return ordinaryBad("ORDINARY_PARSER_DESCRIPTOR")
		}
		for i, p := range d.Parts {
			offset := uint64(i) * (16 << 20)
			if p.Index != uint64(i) || p.Offset != offset || p.Bytes != min(uint64(16<<20), d.Logical.Bytes-offset) || !ordinaryPath(p.Path) || !hex64.MatchString(p.SHA256) {
				return ordinaryBad("ORDINARY_PARSER_DESCRIPTOR")
			}
		}
	}
	if m.Role == "driver-paged-descriptor" || m.Role == "library-paged-descriptor" {
		var d semanticBinaryDescriptor
		if err := ordinaryDecode(data, &d); err != nil {
			return err
		}
		kind := "driver"
		if m.Role == "library-paged-descriptor" {
			kind = "library"
		}
		if d.Schema != semanticBinaryPagesSchema || d.Owner != m.Owner || d.Logical.Kind != kind || !ordinaryPath(d.Logical.Path) || d.Logical.Bytes <= 16<<20 || d.Logical.Bytes > semanticBinaryLogicalMax || !hex64.MatchString(d.Logical.SHA256) || uint64(len(d.Parts)) != (d.Logical.Bytes+(16<<20)-1)/(16<<20) {
			return ordinaryBad("ORDINARY_BINARY_DESCRIPTOR")
		}
		for i, p := range d.Parts {
			offset := uint64(i) * (16 << 20)
			if p.Index != uint64(i) || p.Offset != offset || p.Bytes != min(uint64(16<<20), d.Logical.Bytes-offset) || !ordinaryPath(p.Path) || !hex64.MatchString(p.SHA256) {
				return ordinaryBad("ORDINARY_BINARY_DESCRIPTOR")
			}
		}
	}
	return nil
}

func ParseOrdinaryAssets(data []byte) (OrdinaryAssetRegistration, error) {
	var a OrdinaryAssetRegistration
	if err := ordinaryDecode(data, &a); err != nil {
		return a, err
	}
	if a.Schema != "tsgk-query-ordinary-assets/r1" || !ordinaryPin(a.Registry) || a.Candidate == "" || a.SemanticCohort == "" || a.Assets == nil {
		return a, ordinaryBad("ORDINARY_ASSETS")
	}
	seen := map[string]bool{}
	for _, v := range a.Assets {
		if v.ID == "" || seen[v.ID] || (v.Role != "metadata" && v.Role != "expected-tree" && v.Role != "expected-captures" && v.Role != "expected-facts") {
			return a, ordinaryBad("ORDINARY_ASSET")
		}
		seen[v.ID] = true
		switch v.State {
		case "UNAVAILABLE":
			if v.Pin != nil || v.AuthoringBasis != nil || v.IndependentReview != nil {
				return a, ordinaryBad("ORDINARY_ASSET_STATE")
			}
		case "PINNED":
			if v.Pin == nil || v.AuthoringBasis == nil || v.IndependentReview == nil || !ordinaryPin(*v.Pin) || !ordinaryPin(*v.AuthoringBasis) || !ordinaryPin(*v.IndependentReview) || *v.Pin == *v.AuthoringBasis || *v.Pin == *v.IndependentReview {
				return a, ordinaryBad("ORDINARY_ASSET_STATE")
			}
		default:
			return a, ordinaryBad("ORDINARY_ASSET_STATE")
		}
	}
	return a, nil
}

func ParseOrdinaryRegistration(data []byte) (OrdinaryRegistration, error) {
	var r OrdinaryRegistration
	if err := ordinaryDecode(data, &r); err != nil {
		return r, err
	}
	if r.Schema != "tsgk-query-semantics-registration/r2" || r.Context != OrdinaryContext || r.MembershipContext != "qualification/r8" || !ordinaryClosure(r.ClosureEncoding) || !ordinaryHost(r.Platform, r.Subject) || r.Route == "" || r.Set == "" || r.Candidate == "" || r.SemanticCohort == "" || !hex64.MatchString(r.Namespace) || len(r.Cases) == 0 || len(r.Queries) == 0 || len(r.Required) == 0 {
		return r, ordinaryBad("ORDINARY_REGISTRATION")
	}
	for _, p := range []OrdinaryPin{r.Registry, r.Inventory, r.Slots, r.Profile, r.Assets} {
		if !ordinaryPin(p) {
			return r, ordinaryBad("ORDINARY_REGISTRATION_PIN")
		}
	}
	if err := ordinaryMembers(r.PlannedMembers, r.ClosureEncoding); err != nil {
		return r, err
	}
	cases := map[string]bool{}
	for _, c := range r.Cases {
		if c.ID == "" || cases[c.ID] || c.OriginalSelector == "" || len(c.VersionPins) != len(c.Edits)+1 || len(c.Points) != len(c.Edits) || c.InitialSource != c.VersionPins[0] {
			return r, ordinaryBad("ORDINARY_CASE")
		}
		cases[c.ID] = true
		for _, pin := range c.VersionPins {
			if !ordinaryPin(pin) || pin.Path != "source-witness/"+pin.SHA256+".utf8" {
				return r, ordinaryBad("ORDINARY_SOURCE_BINDING")
			}
		}
	}
	queries := map[string]bool{}
	for _, q := range r.Queries {
		if q.ID == "" || queries[q.ID] || !ordinaryPin(q.Source) || q.Kind != queryregistrycore.Structural || q.MetadataAsset == "" {
			return r, ordinaryBad("ORDINARY_QUERY_BINDING")
		}
		queries[q.ID] = true
	}
	ids := map[string]bool{}
	for _, required := range r.Required {
		c := required.Coordinate
		if ids[required.ID] || c.ID(r.Namespace) != required.ID || c.Kind != queryregistrycore.Structural || c.Role != "primary" || c.Route != r.Route || c.Set != r.Set || c.Platform != r.Platform || c.Subject != r.Subject || c.Context != r.MembershipContext || !cases[c.Case] || !queries[c.Query] || c.TreeRole != "incremental" && c.TreeRole != "fresh" || c.Step == 0 && c.TreeRole == "fresh" || required.TreeAsset == "" || required.CaptureAsset == "" {
			return r, ordinaryBad("ORDINARY_REQUIRED_COORDINATE")
		}
		ids[required.ID] = true
	}
	return r, nil
}
