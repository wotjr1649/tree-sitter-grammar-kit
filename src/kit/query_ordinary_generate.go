package kit

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"sort"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// Source-only generator of the ordinary registration data: the planning profile, the asset
// registration and one semantic registration per route and host. It reads only the pinned P0
// documents, the P1 source witness and the committed query texts. It never runs native code and
// never creates an expected asset or an approval.

const (
	ordinaryGeneratedProfilePath = "profile/tsgk-oracle-r4.json"
	ordinaryGeneratedAssetsPath  = "profile/ordinary-assets-r1.json"
	ordinaryGeneratedPolicyPath  = "policy/native.txt"
	ordinaryParserPagedAbove     = uint64(16 << 20)
)

// ordinaryUnavailableAssets are the only expected assets before an independent derivation exists
// (#427 P5). Every required row of every registration points at them, so every tuple stays BLOCKED.
var ordinaryUnavailableAssets = []OrdinaryAsset{
	{ID: "unavailable-metadata", Role: "metadata", State: "UNAVAILABLE"},
	{ID: "unavailable-expected-tree", Role: "expected-tree", State: "UNAVAILABLE"},
	{ID: "unavailable-expected-captures", Role: "expected-captures", State: "UNAVAILABLE"},
	{ID: "unavailable-expected-facts", Role: "expected-facts", State: "UNAVAILABLE"},
}

// ordinaryClosureOverride lists the routes whose closure encoding cannot be read from the pinned
// inputs: the owned TSQL run needed a paged driver and library, which only a build shows.
var ordinaryClosureOverride = map[string]string{"tsql": "binary-paged"}

// OrdinaryGenerateRequest carries the caller-pinned inputs. Candidate and SemanticCohort name the
// build candidate and cohort the documents bind to; the caller owns them.
type OrdinaryGenerateRequest struct {
	Registry       []byte
	RegistrySHA256 string
	Qualification  []byte
	Slots          []byte
	Basis          map[string][]byte
	Witness        []byte
	// QueryFiles maps a route to the bytes of src/testdata/native/queries/<route>.json.
	QueryFiles     map[string][]byte
	FactPack       []byte
	Candidate      string
	SemanticCohort string
}

// OrdinaryGenerated is the generated document set. Registrations is keyed "route/platform" and
// Bodies holds every query text a registration pins, keyed by its pinned path.
type OrdinaryGenerated struct {
	Profile       []byte
	Assets        []byte
	Registrations map[string][]byte
	Bodies        map[string][]byte
}

// OrdinaryClosureEncoding is the closure encoding the generator registers for a route.
func OrdinaryClosureEncoding(route string, grammar []NativeInput) string {
	if encoding, ok := ordinaryClosureOverride[route]; ok {
		return encoding
	}
	for _, input := range grammar {
		if input.Role == "parser" && input.Bytes > ordinaryParserPagedAbove {
			return "parser-paged"
		}
	}
	return "plain"
}

// PlanRequest returns the planner request that consumes the generated documents.
func (g OrdinaryGenerated) PlanRequest(req OrdinaryGenerateRequest) OrdinaryPlanRequest {
	keys := make([]string, 0, len(g.Registrations))
	for key := range g.Registrations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	registrations := make([][]byte, 0, len(keys))
	for _, key := range keys {
		registrations = append(registrations, g.Registrations[key])
	}
	return OrdinaryPlanRequest{Registry: req.Registry, Qualification: req.Qualification, Slots: req.Slots, Witness: req.Witness,
		Profile: g.Profile, Assets: g.Assets, RegistrySHA256: req.RegistrySHA256, Basis: req.Basis, Registrations: registrations, Bodies: g.Bodies}
}

func ordinaryQueryTexts(req OrdinaryGenerateRequest, route string) (map[string]string, error) {
	var file struct {
		Queries []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"queries"`
	}
	if err := json.Unmarshal(req.QueryFiles[route], &file); err != nil {
		return nil, ordinaryBad("ORDINARY_GENERATE_QUERY_FILE")
	}
	texts := make(map[string]string, len(file.Queries))
	for _, q := range file.Queries {
		texts[q.ID] = q.Source
	}
	pack, err := ParseFactPack(req.FactPack)
	if err != nil {
		return nil, err
	}
	for _, r := range pack.Routes {
		if r.Route == route {
			for _, q := range r.Queries {
				texts[q.ID] = q.Source
			}
		}
	}
	return texts, nil
}

// GenerateOrdinaryRegistration derives the planning profile, the asset registration and the 26 x 2
// registrations. The output depends only on the request, so equal inputs give equal bytes, and the
// two hosts differ only where a host is named (see the P3 section of the ordinary plan contract).
func GenerateOrdinaryRegistration(req OrdinaryGenerateRequest) (OrdinaryGenerated, error) {
	var out OrdinaryGenerated
	if req.Candidate == "" || req.SemanticCohort == "" {
		return out, ordinaryBad("ORDINARY_GENERATE_IDENTITY")
	}
	v, inv, err := ordinaryRegistry(OrdinaryPlanRequest{Registry: req.Registry, Qualification: req.Qualification, Slots: req.Slots, RegistrySHA256: req.RegistrySHA256, Basis: req.Basis})
	if err != nil {
		return out, err
	}
	witness, err := parseOrdinarySourceExport(req.Witness, inv, v.SHA256())
	if err != nil {
		return out, err
	}
	runtimeManifest, err := drivers.NativeC.ReadFile("native-c/runtime-manifest.json")
	if err != nil {
		return out, ordinaryBad("ORDINARY_GENERATE_RUNTIME")
	}
	runtimePin := queryregistrycore.FilePin("runtime/runtime-manifest.json", runtimeManifest)
	hosts := []OrdinaryHost{{"windows-amd64", "windows/amd64"}, {"linux-amd64", "linux/amd64"}}
	profile := OrdinaryProfile{Schema: "tsgk-oracle/r4", Context: OrdinaryContext, Candidate: req.Candidate, SemanticCohort: req.SemanticCohort,
		NativePolicy: queryregistrycore.FilePin(ordinaryGeneratedPolicyPath, []byte(NativePolicyText(NativeOperations()["native-query-batch"], OutputTree))),
		Limits:       OrdinaryPlanLimits(), InheritedLimits: OrdinaryPlanLimits(), Hosts: hosts, Bindings: []OrdinaryBinding{}}
	for _, route := range inv.Routes {
		for _, host := range hosts {
			profile.Bindings = append(profile.Bindings, OrdinaryBinding{Route: route.Route, Platform: host.Platform, Subject: host.Subject,
				ClosureEncoding: OrdinaryClosureEncoding(route.Route, route.Workload.Grammar), Grammar: route.Workload.Grammar, Runtime: runtimePin, Members: []OrdinaryMember{}})
		}
	}
	assets := OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Registry: OrdinaryPin{Path: queryregistrycore.Path, Bytes: uint64(len(req.Registry)), SHA256: v.SHA256()},
		Candidate: req.Candidate, SemanticCohort: req.SemanticCohort, Assets: append([]OrdinaryAsset{}, ordinaryUnavailableAssets...)}
	if out.Profile, err = jsonv2.Marshal(profile); err != nil {
		return out, ordinaryBad("ORDINARY_GENERATE_JSON")
	}
	if out.Assets, err = jsonv2.Marshal(assets); err != nil {
		return out, ordinaryBad("ORDINARY_GENERATE_JSON")
	}
	profilePin := queryregistrycore.FilePin(ordinaryGeneratedProfilePath, out.Profile)
	assetsPin := queryregistrycore.FilePin(ordinaryGeneratedAssetsPath, out.Assets)

	coordinates := map[string][]OrdinaryCoordinate{}
	for _, c := range v.Coordinates() {
		if c.Kind == queryregistrycore.Structural && c.Role == "primary" {
			key := c.Route + "/" + c.Platform
			coordinates[key] = append(coordinates[key], c)
		}
	}
	out.Registrations, out.Bodies = map[string][]byte{}, map[string][]byte{}
	for ri, route := range inv.Routes {
		source := witness.Routes[ri]
		texts, err := ordinaryQueryTexts(req, route.Route)
		if err != nil {
			return OrdinaryGenerated{}, err
		}
		versions := make(map[string]uint64, len(source.Versions))
		for _, version := range source.Versions {
			versions[version.SHA256] = version.Bytes
		}
		queries := make([]OrdinaryQuery, 0, len(source.Queries))
		for _, q := range source.Queries {
			text, ok := texts[q.ID]
			if !ok || digestHex([]byte(text)) != q.SHA256 {
				return OrdinaryGenerated{}, ordinaryBad("ORDINARY_GENERATE_QUERY_PIN")
			}
			pin := queryregistrycore.FilePin("queries/"+route.Route+"/"+q.ID+".scm", []byte(text))
			out.Bodies[pin.Path] = []byte(text)
			queries = append(queries, OrdinaryQuery{ID: q.ID, Source: pin, MetadataAsset: "unavailable-metadata", Kind: queryregistrycore.Structural})
		}
		cases := make([]OrdinaryCase, 0, len(source.Cases))
		for ci, c := range source.Cases {
			pins := make([]OrdinaryPin, len(c.Steps))
			for step, sha := range c.Steps {
				pins[step] = OrdinaryPin{Path: "source-witness/" + sha + ".utf8", Bytes: versions[sha], SHA256: sha}
			}
			cases = append(cases, OrdinaryCase{ID: c.ID, OriginalSelector: c.Selector, InitialSource: pins[0], Edits: route.Workload.Cases[ci].Edits,
				VersionPins: pins, Points: append([]EditPoints{}, c.Points...)})
		}
		for _, host := range hosts {
			key := route.Route + "/" + host.Platform
			registration := OrdinaryRegistration{Schema: "tsgk-query-semantics-registration/r2", Registry: witness.Registry, Namespace: v.Namespace(),
				Inventory: witness.Qualification, Slots: witness.Slots, Profile: profilePin, Assets: assetsPin, Route: route.Route, Set: route.Workload.Set,
				Platform: host.Platform, Subject: host.Subject, MembershipContext: "qualification/r8", Context: OrdinaryContext,
				ClosureEncoding: OrdinaryClosureEncoding(route.Route, route.Workload.Grammar), Candidate: req.Candidate, SemanticCohort: req.SemanticCohort,
				Cases: cases, Queries: queries, PlannedMembers: []OrdinaryMember{}}
			for _, c := range coordinates[key] {
				row := OrdinaryRequired{ID: c.ID(v.Namespace()), Coordinate: c, TreeAsset: "unavailable-expected-tree", CaptureAsset: "unavailable-expected-captures"}
				if strings.HasPrefix(c.Query, "facts.") {
					row.FactAsset = "unavailable-expected-facts"
				}
				registration.Required = append(registration.Required, row)
			}
			data, err := jsonv2.Marshal(registration)
			if err != nil {
				return OrdinaryGenerated{}, ordinaryBad("ORDINARY_GENERATE_JSON")
			}
			if len(data) > 16<<20 {
				return OrdinaryGenerated{}, fail(KindResourceLimit, "ORDINARY_GENERATE_SIZE", "ordinary", nil)
			}
			out.Registrations[key] = data
		}
	}
	return out, nil
}
