package kit

import (
	"bytes"
	"context"
	jsontext "encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

const (
	classificationGoDefinitionInventorySHA     = "f24b4f0b4aad3fe165afd7950d2166fdc7e1d6a59d809faf670d5907c4f12789"
	classificationGoDefinitionQualificationSHA = "cce474cac1f21f8a5ff8dc1fe25ac11840d4e1dcb8c174c049fa0f78672c06cb"
	classificationGoDefinitionQueriesSHA       = "9ba4b6c5ca375214844811aeb06a6fedda92163189cf2b8d1bd42801dd94a192"
	classificationCSharpDefinitionQueriesSHA   = "95d0b9a52015452d80877505041a4e85478fe050480a432d74aad19e2b0d4dfd"
	classificationCSharpFactQueryPackSHA       = "f86e4c8072d30480bfdd7a7a77a3c6d92fb52090b910deeb38b9137fa4b4e21a"
)

type classificationGoDefinitionSlotQuery struct {
	ID               string `json:"id"`
	SHA256           string `json:"sha256"`
	SourceFile       string `json:"source_file"`
	Class            string `json:"class"`
	CompiledMetadata string `json:"compiled_metadata"`
}

type classificationGoDefinitionSlotCounts struct {
	Cases                   uint64 `json:"cases"`
	Edits                   uint64 `json:"edits"`
	CaseSteps               uint64 `json:"case_steps"`
	TreeRoles               uint64 `json:"tree_roles"`
	PotentialQueryTreeRoles uint64 `json:"potential_query_tree_roles"`
}

type classificationGoDefinitionSlotGroup struct {
	Route     string                                `json:"route"`
	Set       string                                `json:"set"`
	Profile   string                                `json:"profile"`
	Operation string                                `json:"operation"`
	Counts    classificationGoDefinitionSlotCounts  `json:"counts"`
	Queries   []classificationGoDefinitionSlotQuery `json:"queries"`
}

type classificationGoDefinitionQuerySourcePin struct {
	Path   string `json:"path"`
	Bytes  uint64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type classificationGoDefinitionSlotInventory struct {
	Schema             string                                `json:"schema"`
	Assessment         jsontext.Value                        `json:"assessment"`
	Coordinate         []string                              `json:"coordinate"`
	ExpansionRule      string                                `json:"expansion_rule"`
	ExtraRoles         jsontext.Value                        `json:"extra_roles"`
	NewMacOSValidation jsontext.Value                        `json:"new_macos_validation"`
	Ordinary           []classificationGoDefinitionSlotGroup `json:"ordinary"`
	OrdinaryTotals     jsontext.Value                        `json:"ordinary_totals"`
	Qualification      struct {
		Path   string `json:"path"`
		Bytes  uint64 `json:"bytes"`
		SHA256 string `json:"sha256"`
	} `json:"qualification_inventory"`
	QuerySources          []classificationGoDefinitionQuerySourcePin `json:"query_sources"`
	Scope                 jsontext.Value                             `json:"scope"`
	SemanticPlatformScope jsontext.Value                             `json:"semantic_platform_scope"`
	SemanticRequired      jsontext.Value                             `json:"semantic_required"`
}

type classificationGoDefinitionQueryFile struct {
	Schema  string           `json:"schema"`
	Route   string           `json:"route"`
	Queries []OracleQuery    `json:"queries"`
	Cases   []jsontext.Value `json:"cases"`
}

type classificationGoDefinitionInputs struct {
	qualification QualificationInventory
	workload      QualWorkload
	slotGroup     classificationGoDefinitionSlotGroup
	queryFile     classificationGoDefinitionQueryFile
	coordinates   classificationGoDefinitionCoordinateSummary
}

// The C# caller passes both exact source files as opaque raw bytes in one envelope.
type classificationCSharpDefinitionQueryInputs struct {
	QueryFile     []byte `json:"query_file"`
	FactQueryPack []byte `json:"fact_query_pack"`
}

type classificationCSharpDefinitionFactQueryPack struct {
	Schema       string `json:"schema"`
	Revision     string `json:"revision"`
	Mapping      string `json:"mapping"`
	DynamicSQL   string `json:"dynamic_sql"`
	XMLStructure string `json:"xml_structure"`
	Routes       []struct {
		Route        string           `json:"route"`
		Declarations []jsontext.Value `json:"declarations"`
		Queries      []struct {
			ID     string `json:"id"`
			Facts  string `json:"facts"`
			Source string `json:"source"`
		} `json:"queries"`
	} `json:"routes"`
}

type classificationCSharpDefinitionInputs struct {
	ordinary    classificationGoDefinitionInputs
	sourceFiles []classificationGoDefinitionQuerySourcePin
}

type classificationGoDefinitionCoordinate struct {
	QualificationInventorySHA256 string `json:"qualification_inventory.sha256"`
	Set                          string `json:"set"`
	Case                         string `json:"case"`
	Query                        string `json:"query"`
	Step                         uint32 `json:"step"`
	TreeRole                     string `json:"tree_role"`
}

type classificationGoDefinitionCoordinateSummary struct {
	QuerySlotInventorySHA256     string                                     `json:"query_slot_inventory_sha256"`
	QualificationInventorySHA256 string                                     `json:"qualification_inventory_sha256"`
	QuerySourceFileSHA256        string                                     `json:"query_source_file_sha256,omitempty"`
	QuerySourceFiles             []classificationGoDefinitionQuerySourcePin `json:"query_source_files,omitempty"`
	Set                          string                                     `json:"set"`
	Cases                        uint64                                     `json:"cases"`
	Edits                        uint64                                     `json:"edits"`
	CaseSteps                    uint64                                     `json:"case_steps"`
	TreeRoles                    uint64                                     `json:"tree_roles"`
	Definitions                  uint64                                     `json:"definitions"`
	PotentialQueryTreeRoles      uint64                                     `json:"potential_query_tree_roles"`
	TreeRolesPerDefinition       uint64                                     `json:"tree_roles_per_definition"`
	CoordinatesSHA256            string                                     `json:"coordinates_sha256"`
	CoordinateOrder              string                                     `json:"coordinate_order"`
}

type classificationGoDefinition struct {
	Query              string               `json:"query"`
	Source             string               `json:"source"`
	SourceSHA256       string               `json:"source_sha256"`
	SourceFile         string               `json:"source_file"`
	Metadata           QueryWitnessMetadata `json:"metadata"`
	Class              string               `json:"class"`
	InventoryClass     string               `json:"inventory_class"`
	CompiledMetadata   string               `json:"inventory_compiled_metadata"`
	Assessment         string               `json:"inventory_assessment"`
	SemanticRequired   *uint64              `json:"inventory_semantic_required"`
	SemanticEvaluated  bool                 `json:"semantic_evaluated"`
	PotentialTreeRoles uint64               `json:"potential_tree_roles"`
	NativeStatus       string               `json:"native_status"`
	ReferenceStatus    string               `json:"reference_status"`
}

type classificationGoDefinitionGuardBinding struct {
	ReaderRevision             string                  `json:"reader_revision"`
	NativeContext              string                  `json:"native_context,omitempty"`
	ObservationSourceRevision  string                  `json:"observation_source_revision"`
	ObservationRunID           string                  `json:"observation_run_id"`
	ObservationAttempt         uint64                  `json:"observation_attempt"`
	ObservationPlatform        string                  `json:"observation_platform"`
	Case                       string                  `json:"case"`
	Route                      string                  `json:"route"`
	Workload                   string                  `json:"workload"`
	Operation                  string                  `json:"operation"`
	NativeInvocationID         string                  `json:"native_invocation_id"`
	ReferenceInvocationIDs     []string                `json:"reference_invocation_ids"`
	CaseSourceSHA256           string                  `json:"case_source_sha256"`
	AnchorsSHA256              string                  `json:"anchors_sha256"`
	RuntimeManifestSHA256      string                  `json:"runtime_manifest_sha256"`
	ProducerABI                eligibilityProducerABI  `json:"producer_abi"`
	DriverSHA256               string                  `json:"driver_sha256"`
	ReferenceExecutableSHA256  string                  `json:"reference_executable_sha256"`
	ReferenceLibrarySHA256     string                  `json:"reference_library_sha256"`
	CurrentLibrarySHA256       string                  `json:"current_library_sha256"`
	CompilerSHA256             string                  `json:"compiler_sha256"`
	BuildIdentitySHA256        string                  `json:"build_identity_sha256"`
	SourceSnapshotSHA256       string                  `json:"source_snapshot_sha256"`
	VendorSnapshotSHA256       string                  `json:"vendor_snapshot_sha256"`
	Grammar                    []NativeInput           `json:"grammar"`
	ApprovalSHA256             string                  `json:"approval_sha256"`
	RegistrationSHA256         string                  `json:"registration_sha256"`
	WitnessSHA256              string                  `json:"witness_sha256"`
	CatalogueSHA256            string                  `json:"catalogue_sha256"`
	GraphSHA256                string                  `json:"graph_sha256"`
	EligibilityResultSchema    string                  `json:"eligibility_result_schema"`
	EligibilityExecutionStatus string                  `json:"eligibility_execution_status"`
	EligibilityAssessment      string                  `json:"eligibility_assessment"`
	EligibilityComplete        bool                    `json:"eligibility_complete"`
	EligibilityTuple           QueryEligibilityCounts  `json:"eligibility_tuple"`
	Provenance                 []eligibilityProvenance `json:"provenance"`
	NativeObservationCreated   bool                    `json:"native_observation_created"`
}

type classificationGoDefinitionProjectionResult struct {
	Scope       string                                      `json:"scope"`
	Guard       classificationGoDefinitionGuardBinding      `json:"guard"`
	Inventory   classificationGoDefinitionCoordinateSummary `json:"inventory"`
	Definitions []classificationGoDefinition                `json:"definitions"`
}

func classificationGoDefinitionMetadataValid(meta QueryWitnessMetadata) bool {
	return meta.Patterns > 0 && meta.CaptureNames != nil && len(meta.CaptureNames) != 0 && meta.Predicates != nil && len(meta.Predicates) == 0 && querywire.ValidMetadata(meta.Patterns, meta.CaptureNames, meta.Predicates)
}

func classificationGoDefinitionMetadataEqual(a, b QueryWitnessMetadata) bool {
	return classificationGoDefinitionMetadataValid(a) && classificationGoDefinitionMetadataValid(b) && reflect.DeepEqual(a, b)
}

func classificationGoDefinitionCloneMetadata(meta QueryWitnessMetadata) QueryWitnessMetadata {
	// Only predicate-free metadata is projected; Clone preserves nil versus [].
	return QueryWitnessMetadata{Patterns: meta.Patterns, CaptureNames: slices.Clone(meta.CaptureNames), Predicates: slices.Clone(meta.Predicates)}
}

func classificationGoDefinitionCoordinateProjection(qualificationSHA, set string, workload QualWorkload, queryIDs []string) (classificationGoDefinitionCoordinateSummary, error) {
	var summary classificationGoDefinitionCoordinateSummary
	if !hex64.MatchString(qualificationSHA) || set == "" || workload.Set != set || len(queryIDs) == 0 {
		return summary, fmt.Errorf("invalid definition coordinate identity")
	}
	querySet := make(map[string]bool, len(queryIDs))
	for _, query := range queryIDs {
		if !validID(query) || querySet[query] {
			return summary, fmt.Errorf("duplicate or invalid definition query %q", query)
		}
		querySet[query] = true
	}
	caseSet := make(map[string]bool, len(workload.Cases))
	coordinates := make([]classificationGoDefinitionCoordinate, 0, len(workload.Cases)*len(queryIDs))
	var edits, steps, treeRoles uint64
	for _, c := range workload.Cases {
		if !validID(c.ID) || caseSet[c.ID] {
			return summary, fmt.Errorf("duplicate or invalid qualification case %q", c.ID)
		}
		caseSet[c.ID] = true
		caseEdits := uint64(len(c.Edits))
		edits += caseEdits
		steps += 1 + caseEdits
		caseRoles := uint64(1) + 2*caseEdits
		treeRoles += caseRoles
		for _, query := range queryIDs {
			for step := uint32(0); step <= uint32(len(c.Edits)); step++ {
				coordinates = append(coordinates, classificationGoDefinitionCoordinate{QualificationInventorySHA256: qualificationSHA, Set: set, Case: c.ID, Query: query, Step: step, TreeRole: "incremental"})
				if step > 0 {
					coordinates = append(coordinates, classificationGoDefinitionCoordinate{QualificationInventorySHA256: qualificationSHA, Set: set, Case: c.ID, Query: query, Step: step, TreeRole: "fresh"})
				}
			}
		}
	}
	if len(coordinates) == 0 {
		return summary, fmt.Errorf("empty query definition coordinate set")
	}
	data, err := jsonv2.Marshal(coordinates)
	if err != nil {
		return summary, err
	}
	summary = classificationGoDefinitionCoordinateSummary{
		QualificationInventorySHA256: qualificationSHA,
		Set:                          set,
		Cases:                        uint64(len(workload.Cases)),
		Edits:                        edits,
		CaseSteps:                    steps,
		TreeRoles:                    treeRoles,
		Definitions:                  uint64(len(queryIDs)),
		PotentialQueryTreeRoles:      uint64(len(coordinates)),
		TreeRolesPerDefinition:       treeRoles,
		CoordinatesSHA256:            digestHex(data),
		CoordinateOrder:              "qualification case order, query definition order, increasing step, incremental then fresh",
	}
	return summary, nil
}

func classificationGoDefinitionLoadInputs(a classificationAnchors, plan classificationPlan, inventoryData, qualificationData, queryData []byte) (classificationGoDefinitionInputs, error) {
	var out classificationGoDefinitionInputs
	if a.sha256 != classificationPositiveAnchorsSHA || a.Case != "probe-go-B04-P-6" || a.Route != "go" || a.Workload != "go-positive-single-frame-batch" || !a.API || len(a.Queries) != 3 || len(a.Required) != 3 || len(a.Edits) != 0 {
		return out, fmt.Errorf("definition anchors are not the closed positive-batch profile")
	}
	if plan.Schema != "tsgk-query-classification-plan/r1" || plan.AnchorsSHA256 != a.sha256 || plan.APIAnchorsSHA256 != classificationPositiveAPISHA || len(plan.Grammar) != 5 {
		return out, fmt.Errorf("definition preregistration differs from the positive-batch anchor")
	}
	if digestHex(inventoryData) != classificationGoDefinitionInventorySHA {
		return out, fmt.Errorf("query slot inventory pin differs")
	}
	if digestHex(qualificationData) != classificationGoDefinitionQualificationSHA {
		return out, fmt.Errorf("qualification inventory pin differs")
	}
	if digestHex(queryData) != classificationGoDefinitionQueriesSHA {
		return out, fmt.Errorf("Go query source file pin differs")
	}
	var slots classificationGoDefinitionSlotInventory
	if err := jsonv2.Unmarshal(inventoryData, &slots, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, fmt.Errorf("query slot inventory shape: %w", err)
	}
	if slots.Schema != "tsgk-query-slot-inventory/r1" || slots.ExpansionRule != "case-query-edit-tree-role/r1" || !slices.Equal(slots.Coordinate, []string{"qualification_inventory.sha256", "set", "case", "query", "step", "tree_role"}) || len(slots.Ordinary) != 26 || len(slots.SemanticRequired) == 0 || !bytes.Equal(bytes.TrimSpace(slots.SemanticRequired), []byte("null")) {
		return out, fmt.Errorf("query slot inventory r1 contract differs")
	}
	if slots.Qualification.Path != "src/contracts/qualification-c1.json" || slots.Qualification.Bytes != uint64(len(qualificationData)) || slots.Qualification.SHA256 != classificationGoDefinitionQualificationSHA {
		return out, fmt.Errorf("query slot qualification pin differs")
	}
	var queryFile classificationGoDefinitionQueryFile
	if err := jsonv2.Unmarshal(queryData, &queryFile, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, fmt.Errorf("Go query file shape: %w", err)
	}
	if queryFile.Schema != "tsgk-native-query-cases/r1" || queryFile.Route != "go" || len(queryFile.Queries) != 3 {
		return out, fmt.Errorf("Go query file identity differs")
	}
	inv, err := ParseQualificationInventory(qualificationData)
	if err != nil {
		return out, fmt.Errorf("qualification inventory: %w", err)
	}
	if inv.SHA256 != classificationGoDefinitionQualificationSHA {
		return out, fmt.Errorf("parsed qualification inventory pin differs")
	}
	var route *QualRoute
	routeCount := 0
	for i := range inv.Routes {
		if inv.Routes[i].Route == "go" {
			routeCount++
			route = &inv.Routes[i]
		}
	}
	if routeCount != 1 || route == nil || route.Workload.Set != "s06-go" || route.Workload.Profile != "s06-go" || route.Workload.Operation != "native-query-batch" || route.Workload.Output != "tree" || route.Workload.Symbol != "tree_sitter_go" || !route.Workload.API || len(route.Workload.Grammar) != 5 {
		return out, fmt.Errorf("qualification Go s06-go workload is not unique")
	}
	if !reflect.DeepEqual(route.Workload.Grammar, plan.Grammar) {
		return out, fmt.Errorf("qualification and preregistration grammar pins differ")
	}
	for _, extra := range inv.ExtraRoles {
		for _, workload := range extra.Workloads {
			if workload.Route == "go" && workload.Set == "s06-go" {
				return out, fmt.Errorf("Go s06-go workload is duplicated in an extra role")
			}
		}
	}
	groupMatches := 0
	for _, group := range slots.Ordinary {
		if group.Set == "s06-go" && (group.Route != "go" || group.Profile != "s06-go") {
			return out, fmt.Errorf("s06-go slot group appears under another route or profile")
		}
		if group.Route == "go" && group.Set == "s06-go" {
			groupMatches++
			out.slotGroup = group
		}
	}
	if groupMatches != 1 || out.slotGroup.Operation != "native-query-batch" || len(out.slotGroup.Queries) != 3 || len(route.Workload.Queries) != 3 {
		return out, fmt.Errorf("ordinary Go definition group is missing or duplicated")
	}
	querySourcePins := 0
	for _, pin := range slots.QuerySources {
		if pin.Path == "src/testdata/native/queries/go.json" {
			querySourcePins++
			if pin.Bytes != uint64(len(queryData)) || pin.SHA256 != classificationGoDefinitionQueriesSHA {
				return out, fmt.Errorf("Go query source pin differs")
			}
		}
	}
	if querySourcePins != 1 {
		return out, fmt.Errorf("Go query source pin is missing or duplicated")
	}
	queryIDs := make([]string, len(a.Queries))
	for i, anchor := range a.Queries {
		if !classificationGoDefinitionMetadataValid(anchor.Metadata) {
			return out, fmt.Errorf("anchor query %s lacks complete predicate-free metadata", anchor.ID)
		}
		queryIDs[i] = anchor.ID
		sha := digestHex([]byte(anchor.Source))
		if anchor.SHA256 != sha || queryFile.Queries[i].ID != anchor.ID || queryFile.Queries[i].Source != anchor.Source || digestHex([]byte(queryFile.Queries[i].Source)) != sha || route.Workload.Queries[i].ID != anchor.ID || route.Workload.Queries[i].SHA256 != sha || out.slotGroup.Queries[i].ID != anchor.ID || out.slotGroup.Queries[i].SHA256 != sha || out.slotGroup.Queries[i].SourceFile != "src/testdata/native/queries/go.json" || out.slotGroup.Queries[i].Class != "STRUCTURAL_CANDIDATE" || out.slotGroup.Queries[i].CompiledMetadata != "UNVERIFIED" {
			return out, fmt.Errorf("query %s is not linked exactly across anchor, source file, qualification, and inventory", anchor.ID)
		}
	}
	coordinates, err := classificationGoDefinitionCoordinateProjection(inv.SHA256, route.Workload.Set, route.Workload, queryIDs)
	if err != nil {
		return out, err
	}
	coordinates.QuerySlotInventorySHA256 = classificationGoDefinitionInventorySHA
	coordinates.QuerySourceFileSHA256 = classificationGoDefinitionQueriesSHA
	if coordinates.Cases != 57 || coordinates.Edits != 37 || coordinates.CaseSteps != 94 || coordinates.TreeRoles != 131 || coordinates.Definitions != 3 || coordinates.PotentialQueryTreeRoles != 393 || out.slotGroup.Counts != (classificationGoDefinitionSlotCounts{Cases: 57, Edits: 37, CaseSteps: 94, TreeRoles: 131, PotentialQueryTreeRoles: 393}) {
		return out, fmt.Errorf("Go s06-go query coordinate denominator differs")
	}
	out.qualification, out.workload, out.coordinates, out.queryFile = inv, route.Workload, coordinates, queryFile
	return out, nil
}

func classificationCSharpDefinitionLoadInputs(a classificationAnchors, plan classificationPlan, inventoryData, qualificationData, queryData []byte) (classificationCSharpDefinitionInputs, error) {
	var out classificationCSharpDefinitionInputs
	if a.sha256 != classificationCSharpAnchorsSHA || a.Case != "csharp15-empty-definitions" || a.Route != "csharp" || a.Workload != "csharp15-empty-definition-single-frame" || a.Operation != "native-query" || a.DriverMode != "single" || a.API || a.Source.Bytes != 0 || a.Source.UTF8 != "" || a.Source.SHA256 != digestHex(nil) || len(a.Queries) != 15 || len(a.Required) != 15 || len(a.Edits) != 0 || len(a.Nodes) != 1 || !reflect.DeepEqual(a.Expect, []StepExpectation{{Step: 0, Syntax: "NO_ERROR"}}) || a.ABI != (eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: 654, FieldCount: 26}) {
		return out, fmt.Errorf("definition anchors are not the closed C# 15 single-frame profile")
	}
	root := a.Nodes[0]
	if root.Parent != -1 || root.Type != "compilation_unit" || root.Field != nil || !root.Named || root.Extra || root.IsError || root.HasError || root.IsMissing || root.StartByte != 0 || root.EndByte != 0 || root.StartPoint != (Point{}) || root.EndPoint != (Point{}) {
		return out, fmt.Errorf("C# empty-source anchor is not the pinned compilation-unit root")
	}
	for i, tuple := range a.Required {
		if tuple.Query != a.Queries[i].ID || tuple.Step != 0 || tuple.TreeRole != "incremental" || tuple.Structural == nil || len(tuple.Structural) != 0 || tuple.Expected == nil || len(tuple.Expected) != 0 {
			return out, fmt.Errorf("C# required definition tuple %d is not an explicit empty single-frame obligation", i)
		}
	}
	if plan.Schema != "tsgk-query-classification-plan/r1" || plan.AnchorsSHA256 != a.sha256 || plan.APIAnchorsSHA256 != "" || plan.NativeContext != semanticPagedContext || len(plan.Grammar) != 5 {
		return out, fmt.Errorf("C# preregistration differs from the paged single-frame anchor")
	}
	if digestHex(inventoryData) != classificationGoDefinitionInventorySHA {
		return out, fmt.Errorf("query slot inventory pin differs")
	}
	if digestHex(qualificationData) != classificationGoDefinitionQualificationSHA {
		return out, fmt.Errorf("qualification inventory pin differs")
	}
	var envelope classificationCSharpDefinitionQueryInputs
	if err := jsonv2.Unmarshal(queryData, &envelope, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, fmt.Errorf("C# query source envelope shape: %w", err)
	}
	if envelope.QueryFile == nil || envelope.FactQueryPack == nil || digestHex(envelope.QueryFile) != classificationCSharpDefinitionQueriesSHA || digestHex(envelope.FactQueryPack) != classificationCSharpFactQueryPackSHA {
		return out, fmt.Errorf("C# exact query source or fact pack pin differs")
	}
	var slots classificationGoDefinitionSlotInventory
	if err := jsonv2.Unmarshal(inventoryData, &slots, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, fmt.Errorf("query slot inventory shape: %w", err)
	}
	if slots.Schema != "tsgk-query-slot-inventory/r1" || slots.ExpansionRule != "case-query-edit-tree-role/r1" || !slices.Equal(slots.Coordinate, []string{"qualification_inventory.sha256", "set", "case", "query", "step", "tree_role"}) || len(slots.Ordinary) != 26 || len(slots.SemanticRequired) == 0 || !bytes.Equal(bytes.TrimSpace(slots.SemanticRequired), []byte("null")) {
		return out, fmt.Errorf("query slot inventory r1 contract differs")
	}
	if slots.Qualification.Path != "src/contracts/qualification-c1.json" || slots.Qualification.Bytes != uint64(len(qualificationData)) || slots.Qualification.SHA256 != classificationGoDefinitionQualificationSHA {
		return out, fmt.Errorf("slot qualification pin differs")
	}
	if err := jsonv2.Unmarshal(envelope.QueryFile, &out.ordinary.queryFile, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, fmt.Errorf("C# query file shape: %w", err)
	}
	if out.ordinary.queryFile.Schema != "tsgk-native-query-cases/r1" || out.ordinary.queryFile.Route != "csharp" || len(out.ordinary.queryFile.Queries) != 13 {
		return out, fmt.Errorf("C# query file identity differs")
	}
	var factPack classificationCSharpDefinitionFactQueryPack
	if err := jsonv2.Unmarshal(envelope.FactQueryPack, &factPack, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, fmt.Errorf("fact query pack shape: %w", err)
	}
	if factPack.Schema != "tsgk-fact-query-pack/r1" || factPack.Revision != "fact-queries-r1" || factPack.Mapping != "declaration-facts-r1" || factPack.DynamicSQL != "dynamic-sql-r1" || factPack.XMLStructure != "xml-structure-r1" || len(factPack.Routes) != 4 {
		return out, fmt.Errorf("fact query pack identity differs")
	}
	var csharpPackRoute *struct {
		Route        string           `json:"route"`
		Declarations []jsontext.Value `json:"declarations"`
		Queries      []struct {
			ID     string `json:"id"`
			Facts  string `json:"facts"`
			Source string `json:"source"`
		} `json:"queries"`
	}
	routeCount := 0
	for i := range factPack.Routes {
		if factPack.Routes[i].Route == "csharp" {
			routeCount++
			csharpPackRoute = &factPack.Routes[i]
		}
	}
	if routeCount != 1 || csharpPackRoute == nil || len(csharpPackRoute.Declarations) != 20 || len(csharpPackRoute.Queries) != 2 || csharpPackRoute.Queries[0].ID != "facts.declarations" || csharpPackRoute.Queries[0].Facts != "declarations" || csharpPackRoute.Queries[1].ID != "facts.dynamic_sql" || csharpPackRoute.Queries[1].Facts != "dynamic_sql" {
		return out, fmt.Errorf("C# fact query route is missing, duplicated, or reordered")
	}
	inv, err := ParseQualificationInventory(qualificationData)
	if err != nil {
		return out, fmt.Errorf("qualification inventory: %w", err)
	}
	if inv.SHA256 != classificationGoDefinitionQualificationSHA {
		return out, fmt.Errorf("parsed qualification inventory pin differs")
	}
	var route *QualRoute
	qualificationRouteCount := 0
	for i := range inv.Routes {
		if inv.Routes[i].Route == "csharp" {
			qualificationRouteCount++
			route = &inv.Routes[i]
		}
	}
	if qualificationRouteCount != 1 || route == nil || route.Workload.Set != "s06-csharp" || route.Workload.Profile != "s06-csharp" || route.Workload.Operation != "native-query-batch" || route.Workload.Output != "tree" || route.Workload.Symbol != "tree_sitter_c_sharp" || len(route.Workload.Grammar) != 5 || len(route.Workload.Queries) != 15 || route.Workload.FactPack == nil || route.Workload.FactPack.Route != "csharp" || route.Workload.FactPack.Revision != "fact-queries-r1" || route.Workload.FactPack.SHA256 != classificationCSharpFactQueryPackSHA {
		return out, fmt.Errorf("qualification C# s06-csharp workload is not unique")
	}
	if !reflect.DeepEqual(route.Workload.Grammar, plan.Grammar) {
		return out, fmt.Errorf("qualification and preregistration grammar pins differ")
	}
	for _, extra := range inv.ExtraRoles {
		for _, workload := range extra.Workloads {
			if workload.Route == "csharp" && workload.Set == "s06-csharp" {
				return out, fmt.Errorf("C# s06-csharp workload is duplicated in an extra role")
			}
		}
	}
	groupMatches := 0
	for _, group := range slots.Ordinary {
		if group.Set == "s06-csharp" && (group.Route != "csharp" || group.Profile != "s06-csharp") {
			return out, fmt.Errorf("s06-csharp slot group appears under another route or profile")
		}
		if group.Route == "csharp" && group.Set == "s06-csharp" {
			groupMatches++
			out.ordinary.slotGroup = group
		}
	}
	if groupMatches != 1 || out.ordinary.slotGroup.Profile != "s06-csharp" || out.ordinary.slotGroup.Operation != "native-query-batch" || len(out.ordinary.slotGroup.Queries) != 15 {
		return out, fmt.Errorf("ordinary C# definition group is missing or duplicated")
	}
	wantPins := []classificationGoDefinitionQuerySourcePin{
		{Path: "src/testdata/native/queries/csharp.json", Bytes: uint64(len(envelope.QueryFile)), SHA256: classificationCSharpDefinitionQueriesSHA},
		{Path: "src/contracts/fact-query-pack.json", Bytes: uint64(len(envelope.FactQueryPack)), SHA256: classificationCSharpFactQueryPackSHA},
	}
	for _, want := range wantPins {
		matches := 0
		for _, pin := range slots.QuerySources {
			if pin.Path == want.Path {
				matches++
				if pin != want {
					return out, fmt.Errorf("C# query source pin differs: %s", want.Path)
				}
			}
		}
		if matches != 1 {
			return out, fmt.Errorf("C# query source pin is missing or duplicated: %s", want.Path)
		}
	}
	queryIDs := make([]string, len(a.Queries))
	combined := make([]OracleQuery, 0, len(a.Queries))
	var patternCount, captureCount uint64
	for i, anchor := range a.Queries {
		if !classificationGoDefinitionMetadataValid(anchor.Metadata) {
			return out, fmt.Errorf("anchor query %s lacks complete predicate-free metadata", anchor.ID)
		}
		queryIDs[i] = anchor.ID
		patternCount += uint64(anchor.Metadata.Patterns)
		captureCount += uint64(len(anchor.Metadata.CaptureNames))
		sha := digestHex([]byte(anchor.Source))
		var source, sourceFile string
		if i < len(out.ordinary.queryFile.Queries) {
			query := out.ordinary.queryFile.Queries[i]
			source, sourceFile = query.Source, wantPins[0].Path
		} else {
			query := csharpPackRoute.Queries[i-len(out.ordinary.queryFile.Queries)]
			source, sourceFile = query.Source, wantPins[1].Path
		}
		registered := route.Workload.Queries[i]
		slot := out.ordinary.slotGroup.Queries[i]
		if anchor.ID == "" || anchor.SHA256 != sha || source != anchor.Source || registered.ID != anchor.ID || registered.SHA256 != sha || slot.ID != anchor.ID || slot.SHA256 != sha || slot.SourceFile != sourceFile || slot.Class != "STRUCTURAL_CANDIDATE" || slot.CompiledMetadata != "UNVERIFIED" {
			return out, fmt.Errorf("query %s is not linked exactly across C# anchors, raw source files, qualification, and inventory", anchor.ID)
		}
		if i >= len(out.ordinary.queryFile.Queries) && csharpPackRoute.Queries[i-len(out.ordinary.queryFile.Queries)].Facts != strings.TrimPrefix(anchor.ID, "facts.") {
			return out, fmt.Errorf("fact query %s is not bound to its fact identity", anchor.ID)
		}
		combined = append(combined, OracleQuery{ID: anchor.ID, Source: source})
	}
	if patternCount != 91 || captureCount != 125 {
		return out, fmt.Errorf("C# anchor metadata denominator differs")
	}
	coordinates, err := classificationGoDefinitionCoordinateProjection(inv.SHA256, route.Workload.Set, route.Workload, queryIDs)
	if err != nil {
		return out, err
	}
	coordinates.QuerySlotInventorySHA256 = classificationGoDefinitionInventorySHA
	coordinates.QuerySourceFiles = slices.Clone(wantPins)
	wantCounts := classificationGoDefinitionSlotCounts{Cases: 390, Edits: 125, CaseSteps: 515, TreeRoles: 640, PotentialQueryTreeRoles: 9600}
	if coordinates.Cases != wantCounts.Cases || coordinates.Edits != wantCounts.Edits || coordinates.CaseSteps != wantCounts.CaseSteps || coordinates.TreeRoles != wantCounts.TreeRoles || coordinates.Definitions != 15 || coordinates.PotentialQueryTreeRoles != wantCounts.PotentialQueryTreeRoles || out.ordinary.slotGroup.Counts != wantCounts {
		return out, fmt.Errorf("C# s06-csharp query coordinate denominator differs")
	}
	out.ordinary.qualification, out.ordinary.workload, out.ordinary.coordinates = inv, route.Workload, coordinates
	out.ordinary.queryFile.Queries = combined
	out.sourceFiles = slices.Clone(wantPins)
	return out, nil
}

func classificationGoDefinitionRows(a classificationAnchors, inputs classificationGoDefinitionInputs, witnessQueries []QueryWitnessQuery, nativeQueries []querywire.Query, references []*instrumentedReference) ([]classificationGoDefinition, error) {
	sourceFiles := make([]string, len(a.Queries))
	for i := range sourceFiles {
		sourceFiles[i] = "src/testdata/native/queries/go.json"
	}
	return classificationDefinitionRows(a, inputs, witnessQueries, nativeQueries, references, "Go", sourceFiles)
}

func classificationCSharpDefinitionRows(a classificationAnchors, inputs classificationCSharpDefinitionInputs, witnessQueries []QueryWitnessQuery, nativeQueries []querywire.Query, references []*instrumentedReference) ([]classificationGoDefinition, error) {
	if len(inputs.sourceFiles) != 2 || inputs.sourceFiles[0].Path != "src/testdata/native/queries/csharp.json" || inputs.sourceFiles[1].Path != "src/contracts/fact-query-pack.json" {
		return nil, fmt.Errorf("C# definition source pins are incomplete or reordered")
	}
	sourceFiles := make([]string, len(a.Queries))
	for i := range sourceFiles {
		sourceFiles[i] = inputs.sourceFiles[0].Path
		if i >= 13 {
			sourceFiles[i] = inputs.sourceFiles[1].Path
		}
	}
	return classificationDefinitionRows(a, inputs.ordinary, witnessQueries, nativeQueries, references, "C#", sourceFiles)
}

func classificationDefinitionRows(a classificationAnchors, inputs classificationGoDefinitionInputs, witnessQueries []QueryWitnessQuery, nativeQueries []querywire.Query, references []*instrumentedReference, language string, sourceFiles []string) ([]classificationGoDefinition, error) {
	want := 0
	switch language {
	case "Go":
		want = 3
	case "C", "C++", "Rust":
		want = 4
	case "Java":
		want = 5
	case "C#":
		want = 15
	default:
		return nil, fmt.Errorf("unsupported closed definition language %q", language)
	}
	if len(a.Queries) != want || len(inputs.queryFile.Queries) != want || len(inputs.slotGroup.Queries) != want || len(inputs.workload.Queries) != want || len(witnessQueries) != want || len(nativeQueries) != want || len(references) != want || len(sourceFiles) != want {
		return nil, fmt.Errorf("%s definition query set is incomplete", language)
	}
	rows := make([]classificationGoDefinition, 0, len(a.Queries))
	seen := make(map[string]bool, len(a.Queries))
	for i, anchor := range a.Queries {
		if seen[anchor.ID] {
			return nil, fmt.Errorf("duplicate %s definition %s", language, anchor.ID)
		}
		seen[anchor.ID] = true
		witness := witnessQueries[i]
		native := nativeQueries[i]
		reference := references[i]
		if witness.Metadata == nil || reference == nil {
			return nil, fmt.Errorf("query %s lacks witness or reference metadata", anchor.ID)
		}
		if native.ID != anchor.ID || native.Status != StatusCompleted || native.Code != "" || native.Partial {
			return nil, fmt.Errorf("native query %s is not a complete compiled result", anchor.ID)
		}
		if reference.Status != StatusCompleted || reference.Code != "" {
			return nil, fmt.Errorf("reference query %s is not complete", anchor.ID)
		}
		referenceQuery := reference.Query
		if referenceQuery.ID != anchor.ID || referenceQuery.SHA256 != anchor.SHA256 {
			return nil, fmt.Errorf("reference query %s identity differs", anchor.ID)
		}
		if witness.ID != anchor.ID || witness.Source != anchor.Source || native.ID != inputs.queryFile.Queries[i].ID || inputs.workload.Queries[i].ID != anchor.ID || inputs.slotGroup.Queries[i].ID != anchor.ID {
			return nil, fmt.Errorf("query %s does not occur exactly once in the registered order", anchor.ID)
		}
		if sourceFiles[i] != inputs.slotGroup.Queries[i].SourceFile || inputs.slotGroup.Queries[i].Class != "STRUCTURAL_CANDIDATE" || inputs.slotGroup.Queries[i].CompiledMetadata != "UNVERIFIED" {
			return nil, fmt.Errorf("query %s static inventory classification changed", anchor.ID)
		}
		if digestHex([]byte(anchor.Source)) != anchor.SHA256 || inputs.queryFile.Queries[i].Source != anchor.Source || inputs.workload.Queries[i].SHA256 != anchor.SHA256 || inputs.slotGroup.Queries[i].SHA256 != anchor.SHA256 {
			return nil, fmt.Errorf("query %s source bytes or hash differ", anchor.ID)
		}
		expected := anchor.Metadata
		witnessMetadata := *witness.Metadata
		nativeMetadata := QueryWitnessMetadata{Patterns: native.Patterns, CaptureNames: native.CaptureNames, Predicates: native.Predicates}
		referenceMetadata := QueryWitnessMetadata{Patterns: referenceQuery.Patterns, CaptureNames: referenceQuery.CaptureNames, Predicates: referenceQuery.Predicates}
		if !classificationGoDefinitionMetadataEqual(expected, witnessMetadata) || !classificationGoDefinitionMetadataEqual(expected, nativeMetadata) || !classificationGoDefinitionMetadataEqual(expected, referenceMetadata) {
			return nil, fmt.Errorf("query %s compiled metadata differs or omits an explicit empty predicate list", anchor.ID)
		}
		rows = append(rows, classificationGoDefinition{
			Query:              anchor.ID,
			Source:             anchor.Source,
			SourceSHA256:       anchor.SHA256,
			SourceFile:         inputs.slotGroup.Queries[i].SourceFile,
			Metadata:           classificationGoDefinitionCloneMetadata(nativeMetadata),
			Class:              "STRUCTURAL_ONLY",
			InventoryClass:     inputs.slotGroup.Queries[i].Class,
			CompiledMetadata:   inputs.slotGroup.Queries[i].CompiledMetadata,
			Assessment:         "NOT_ASSESSED",
			SemanticRequired:   nil,
			SemanticEvaluated:  false,
			PotentialTreeRoles: inputs.coordinates.TreeRolesPerDefinition,
			NativeStatus:       native.Status,
			ReferenceStatus:    reference.Status,
		})
	}
	return rows, nil
}

// This is called only from the callback after classificationGoVerifyWith finishes the
// original guarded core, native frame checks, compiler receipt, and public parity check.
func classificationGoDefinitionProjection(anchors classificationAnchors, plan classificationPlan, s *eligibilitySession, cache *semanticNativeCache, core QueryReferenceEligibilityResult, inventoryData, qualificationData, queryData []byte, readerRevision string) (classificationGoDefinitionProjectionResult, error) {
	var result classificationGoDefinitionProjectionResult
	inputs, err := classificationGoDefinitionLoadInputs(anchors, plan, inventoryData, qualificationData, queryData)
	if err != nil {
		return result, err
	}
	if !hex40.MatchString(readerRevision) {
		return result, fmt.Errorf("separately pinned reader revision is invalid")
	}
	if s == nil || cache == nil || s.res == nil || s.r == nil || s.semantic != cache || !s.bound || !cache.ready || !cache.record.haveRaw {
		return result, fmt.Errorf("definition projection requires the live guarded eligibility session")
	}
	if err := s.r.check(); err != nil {
		return result, err
	}
	if core.ResultSchema != QueryReferenceEligibilityResultSchema || core.ExecutionStatus != StatusCompleted || core.Assessment != AssessPass || !core.Complete || core.Tuple != (QueryEligibilityCounts{Required: 3, Verified: 3}) ||
		s.res.ResultSchema != core.ResultSchema || s.res.ApprovalSHA256 != core.ApprovalSHA256 || s.res.RegistrationSHA256 != core.RegistrationSHA256 || s.res.WitnessSHA256 != core.WitnessSHA256 || s.res.CatalogueSHA256 != core.CatalogueSHA256 || s.res.GraphSHA256 != core.GraphSHA256 || !s.res.Complete {
		return result, fmt.Errorf("definition projection requires the same completed guarded result")
	}
	if !classificationLibraryMatches(s) {
		return result, fmt.Errorf("definition reference library differs from guarded compiler output")
	}
	if s.reg.Native == nil || len(cache.checked) != 1 || cache.checked[0].Step.Step != 0 || len(cache.checked[0].Incremental.Queries) != len(anchors.Queries) {
		return result, fmt.Errorf("guarded native query result differs from the fixed definition scope")
	}
	references := make(map[string]*instrumentedReference, len(anchors.Required))
	referenceIDs := make(map[string]string, len(anchors.Required))
	var referenceExecutableSHA string
	for _, tuple := range anchors.Required {
		observation, ok := s.observation(tuple)
		if !ok {
			return result, fmt.Errorf("guarded reference observation is missing: %s", tuple.Query)
		}
		out := s.outputs[observation.InvocationID]
		if out == nil || observation.ResponseOrdinal >= uint64(len(out.responses)) || out.responses[observation.ResponseOrdinal] == nil {
			return result, fmt.Errorf("guarded reference response is missing: %s", tuple.Query)
		}
		invocation := s.invocations[observation.InvocationID]
		if invocation == nil || invocation.Executable == nil {
			return result, fmt.Errorf("guarded reference executable is missing: %s", tuple.Query)
		}
		if referenceExecutableSHA == "" {
			referenceExecutableSHA = invocation.Executable.Member.SHA256
		} else if referenceExecutableSHA != invocation.Executable.Member.SHA256 {
			return result, fmt.Errorf("definition reference executable bindings differ")
		}
		if referenceExecutableSHA == "" {
			return result, fmt.Errorf("definition reference executable pin is empty")
		}
		references[tuple.Query] = out.responses[observation.ResponseOrdinal]
		referenceIDs[tuple.Query] = observation.InvocationID
	}
	orderedReferences := make([]*instrumentedReference, len(anchors.Queries))
	orderedReferenceIDs := make([]string, len(anchors.Queries))
	for i, query := range anchors.Queries {
		orderedReferences[i], orderedReferenceIDs[i] = references[query.ID], referenceIDs[query.ID]
		if orderedReferences[i] == nil || orderedReferenceIDs[i] == "" {
			return result, fmt.Errorf("guarded reference query is missing: %s", query.ID)
		}
	}
	definitions, err := classificationGoDefinitionRows(anchors, inputs, s.witness.Queries, cache.checked[0].Incremental.Queries, orderedReferences)
	if err != nil {
		return result, err
	}
	nativeInvocation, err := classificationNativeInvocation(s, anchors)
	if err != nil {
		return result, err
	}
	libraryPin, libraryOK := s.owned["current/go.dll"]
	if nativeInvocation == nil || nativeInvocation.ID != "native-"+anchors.Case || nativeInvocation.Executable == nil || nativeInvocation.Executable.Member.SHA256 != s.witness.Native.ExecutableSHA256 {
		return result, fmt.Errorf("guarded native driver executable is missing or mismatched")
	}
	if !libraryOK || libraryPin.NodeID != "go-library-build" || libraryPin.Member.SHA256 == "" || s.reg.Build == nil || s.reg.Native.ReferenceLibrary == nil || s.reg.Native.ReferenceLibrary.Member.SHA256 == "" {
		return result, fmt.Errorf("guarded build or library binding is missing")
	}
	libraryNode, ok := s.nodes["go-library-build"]
	if !ok {
		return result, fmt.Errorf("guarded compiler node is missing")
	}
	result = classificationGoDefinitionProjectionResult{
		Scope: "conditional historical definition classification and potential inventory links; no new native/reference or per-case observation, runtime eligibility or semantic completion",
		Guard: classificationGoDefinitionGuardBinding{
			ReaderRevision: readerRevision, ObservationSourceRevision: plan.SourceRevision, ObservationRunID: plan.RunID,
			ObservationAttempt: s.approval.Candidate.Attempt, ObservationPlatform: s.approval.Candidate.Platform,
			Case: anchors.Case, Route: anchors.Route, Workload: anchors.Workload, Operation: anchors.Operation,
			NativeInvocationID: nativeInvocation.ID, ReferenceInvocationIDs: orderedReferenceIDs,
			CaseSourceSHA256: anchors.Source.SHA256, AnchorsSHA256: anchors.sha256, RuntimeManifestSHA256: plan.RuntimeManifestSHA256,
			ProducerABI: s.witness.Native.ProducerABI, DriverSHA256: nativeInvocation.Executable.Member.SHA256,
			ReferenceExecutableSHA256: referenceExecutableSHA, ReferenceLibrarySHA256: s.reg.Native.ReferenceLibrary.Member.SHA256,
			CurrentLibrarySHA256: libraryPin.Member.SHA256, CompilerSHA256: libraryNode.Identities["tool"],
			BuildIdentitySHA256: s.reg.Build.Identity.Member.SHA256, SourceSnapshotSHA256: s.reg.Build.SourceSnapshot.Member.SHA256,
			VendorSnapshotSHA256: s.reg.Build.VendorSnapshot.Member.SHA256, Grammar: slices.Clone(plan.Grammar),
			ApprovalSHA256: core.ApprovalSHA256, RegistrationSHA256: core.RegistrationSHA256, WitnessSHA256: core.WitnessSHA256,
			CatalogueSHA256: core.CatalogueSHA256, GraphSHA256: core.GraphSHA256, Provenance: slices.Clone(s.approval.Provenance), NativeObservationCreated: false,
			EligibilityResultSchema: core.ResultSchema, EligibilityExecutionStatus: core.ExecutionStatus, EligibilityAssessment: core.Assessment, EligibilityComplete: core.Complete, EligibilityTuple: core.Tuple,
		},
		Inventory:   inputs.coordinates,
		Definitions: definitions,
	}
	return result, nil
}

// The C# projection consumes only the original guarded session and its cached paged-parser result.
// The legacy build node ID is treated as opaque evidence; C# identity comes from the pinned route,
// symbol, grammar, ABI, and native context checked here.
func classificationCSharpDefinitionProjection(anchors classificationAnchors, plan classificationPlan, s *eligibilitySession, cache *semanticNativeCache, core QueryReferenceEligibilityResult, inventoryData, qualificationData, queryData []byte, readerRevision string) (classificationGoDefinitionProjectionResult, error) {
	var result classificationGoDefinitionProjectionResult
	inputs, err := classificationCSharpDefinitionLoadInputs(anchors, plan, inventoryData, qualificationData, queryData)
	if err != nil {
		return result, err
	}
	if !hex40.MatchString(readerRevision) {
		return result, fmt.Errorf("separately pinned reader revision is invalid")
	}
	if s == nil || cache == nil || s.res == nil || s.r == nil || s.semantic != cache || !s.bound || !cache.ready || !cache.record.haveRaw {
		return result, fmt.Errorf("C# definition projection requires the live guarded eligibility session")
	}
	if err := s.r.check(); err != nil {
		return result, err
	}
	wantTuple := QueryEligibilityCounts{Required: 15, Verified: 15}
	if core.ResultSchema != QueryReferenceEligibilityResultSchema || core.ExecutionStatus != StatusCompleted || core.Assessment != AssessPass || !core.Complete || core.Tuple != wantTuple ||
		s.res.ResultSchema != core.ResultSchema || s.res.ExecutionStatus != core.ExecutionStatus || s.res.Assessment != core.Assessment || s.res.Complete != core.Complete || s.res.Tuple != wantTuple ||
		s.res.ApprovalSHA256 != core.ApprovalSHA256 || s.res.RegistrationSHA256 != core.RegistrationSHA256 || s.res.WitnessSHA256 != core.WitnessSHA256 || s.res.CatalogueSHA256 != core.CatalogueSHA256 || s.res.GraphSHA256 != core.GraphSHA256 {
		return result, fmt.Errorf("C# definition projection requires the same completed 15-tuple guarded result")
	}
	if !classificationLibraryMatches(s) {
		return result, fmt.Errorf("C# definition reference library differs from guarded compiler output")
	}
	if plan.NativeContext != semanticPagedContext || s.reg.Context != semanticPagedContext || s.reg.Native == nil || s.reg.Native.Symbol != "tree_sitter_c_sharp" || s.reg.Native.InvocationID != "native-"+anchors.Case ||
		s.witness.Schema != queryEligibilityWitnessSchema || s.witness.Policy != "tsgk-predicates/r1" || s.witness.Case != anchors.Case || s.witness.Route != "csharp" || s.witness.Role != "owned" || s.witness.Encoding != EncodingUTF8 || s.witness.Platform != s.approval.Candidate.Platform || s.witness.Input.Path != "source.bin" || s.witness.Input.Bytes != 0 || s.witness.Input.SHA256 != anchors.Source.SHA256 || s.witness.Input.Role != "case" ||
		s.witness.Native.RequestID != anchors.Case || s.witness.Native.API || s.witness.Native.ProducerABI != anchors.ABI || s.witness.Native.RuntimeManifestSHA256 != plan.RuntimeManifestSHA256 || s.witness.Native.ObservationRunID != plan.RunID || len(s.witness.Native.Points) != 0 ||
		plan.SourceRevision != s.approval.Candidate.SourceRevision || plan.RunID != s.approval.Candidate.Run || !slices.Contains([]string{"windows/amd64", "linux/amd64"}, s.approval.Candidate.Platform) ||
		!reflect.DeepEqual(s.witness.Required, anchors.Required) || !reflect.DeepEqual(s.witness.Edits, []Edit{}) || len(s.witness.Queries) != 15 || len(s.reg.Native.Grammar) != len(plan.Grammar) {
		return result, fmt.Errorf("C# guarded candidate, paged context, or witness differs from the registered anchor")
	}
	if err := classificationGrammarContext(s, anchors, plan); err != nil {
		return result, fmt.Errorf("C# guarded logical grammar pin differs: %w", err)
	}
	if len(cache.checked) != 1 || cache.checked[0].Step.Step != 0 || cache.checked[0].Step.SourceBytes != 0 || cache.checked[0].Step.SourceSHA256 != anchors.Source.SHA256 || cache.checked[0].Step.Edit != nil || cache.checked[0].Fresh != nil || CompareTrees(cache.checked[0].Incremental.Nodes, anchors.Nodes) != nil || len(cache.checked[0].Incremental.Queries) != 15 {
		return result, fmt.Errorf("guarded C# paged-parser single-frame result differs from the fixed definition scope")
	}
	references := make(map[string]*instrumentedReference, len(anchors.Required))
	referenceIDs := make(map[string]string, len(anchors.Required))
	var referenceExecutableSHA string
	for _, tuple := range anchors.Required {
		observation, ok := s.observation(tuple)
		if !ok {
			return result, fmt.Errorf("guarded C# reference observation is missing: %s", tuple.Query)
		}
		if observation.Query != tuple.Query || observation.QueryID != tuple.Query || observation.Step != 0 || observation.TreeRole != "incremental" || observation.SourceBytes != 0 || observation.SourceSHA256 != anchors.Source.SHA256 {
			return result, fmt.Errorf("guarded C# reference observation does not bind the empty single frame: %s", tuple.Query)
		}
		out := s.outputs[observation.InvocationID]
		if out == nil || observation.ResponseOrdinal >= uint64(len(out.responses)) || observation.ResponseOrdinal >= uint64(len(out.rows)) || out.responses[observation.ResponseOrdinal] == nil {
			return result, fmt.Errorf("guarded C# reference response is missing: %s", tuple.Query)
		}
		response := out.responses[observation.ResponseOrdinal]
		row := out.rows[observation.ResponseOrdinal]
		if response.Schema != instrumentedReferenceSchema || response.Status != StatusCompleted || response.Code != "" || response.Input.Encoding != EncodingUTF8 || response.Input.Bytes != 0 || response.Input.SHA256 != anchors.Source.SHA256 ||
			response.Grammar.Symbol != "tree_sitter_c_sharp" || CompareTrees(response.Tree.Nodes, anchors.Nodes) != nil || uint64(len(row)) != observation.ByteLength || digestHex(row) != observation.SHA256 {
			return result, fmt.Errorf("guarded C# reference source frame or complete CST differs: %s", tuple.Query)
		}
		invocation := s.invocations[observation.InvocationID]
		if invocation == nil || invocation.Executable == nil {
			return result, fmt.Errorf("guarded C# reference executable is missing: %s", tuple.Query)
		}
		if referenceExecutableSHA == "" {
			referenceExecutableSHA = invocation.Executable.Member.SHA256
		} else if referenceExecutableSHA != invocation.Executable.Member.SHA256 {
			return result, fmt.Errorf("C# definition reference executable bindings differ")
		}
		if !hex64.MatchString(referenceExecutableSHA) {
			return result, fmt.Errorf("C# definition reference executable pin is invalid")
		}
		references[tuple.Query] = out.responses[observation.ResponseOrdinal]
		referenceIDs[tuple.Query] = observation.InvocationID
	}
	orderedReferences := make([]*instrumentedReference, len(anchors.Queries))
	orderedReferenceIDs := make([]string, len(anchors.Queries))
	for i, query := range anchors.Queries {
		orderedReferences[i], orderedReferenceIDs[i] = references[query.ID], referenceIDs[query.ID]
		if orderedReferences[i] == nil || orderedReferenceIDs[i] == "" {
			return result, fmt.Errorf("guarded C# reference query is missing: %s", query.ID)
		}
	}
	definitions, err := classificationCSharpDefinitionRows(anchors, inputs, s.witness.Queries, cache.checked[0].Incremental.Queries, orderedReferences)
	if err != nil {
		return result, err
	}
	nativeInvocationID := "native-" + anchors.Case
	nativeInvocation, err := classificationNativeInvocation(s, anchors)
	if err != nil {
		return result, err
	}
	libraryPin, libraryOK := s.owned["current/go.dll"]
	if nativeInvocation == nil || nativeInvocation.ID != nativeInvocationID || nativeInvocation.Executable == nil || nativeInvocation.Executable.Member.SHA256 != s.witness.Native.ExecutableSHA256 || !hex64.MatchString(nativeInvocation.Executable.Member.SHA256) {
		return result, fmt.Errorf("guarded C# native driver executable is missing or mismatched")
	}
	if !libraryOK || libraryPin.NodeID != "go-library-build" || !hex64.MatchString(libraryPin.Member.SHA256) || s.reg.Build == nil || s.reg.Native.ReferenceLibrary == nil || !hex64.MatchString(s.reg.Native.ReferenceLibrary.Member.SHA256) {
		return result, fmt.Errorf("guarded C# build or library binding is missing")
	}
	libraryNode, ok := s.nodes["go-library-build"]
	if !ok || !hex64.MatchString(libraryNode.Identities["tool"]) || libraryNode.Identities["tool"] != plan.CompilerSHA256 {
		return result, fmt.Errorf("guarded C# compiler node differs from preregistration")
	}
	for _, pin := range []string{s.reg.Build.Identity.Member.SHA256, s.reg.Build.SourceSnapshot.Member.SHA256, s.reg.Build.VendorSnapshot.Member.SHA256} {
		if !hex64.MatchString(pin) {
			return result, fmt.Errorf("guarded C# build identity pin is invalid")
		}
	}
	result = classificationGoDefinitionProjectionResult{
		Scope: "conditional historical classification of 15 predicate-free C# definitions and 9,600 potential s06-csharp links; the anchor represents one empty-source paged-parser native-query single frame; ordinary batch/API completion, semantic EVALUATED status, and case completion are outside this projection",
		Guard: classificationGoDefinitionGuardBinding{
			ReaderRevision: readerRevision, NativeContext: plan.NativeContext, ObservationSourceRevision: plan.SourceRevision, ObservationRunID: plan.RunID,
			ObservationAttempt: s.approval.Candidate.Attempt, ObservationPlatform: s.approval.Candidate.Platform,
			Case: anchors.Case, Route: anchors.Route, Workload: anchors.Workload, Operation: anchors.Operation,
			NativeInvocationID: nativeInvocation.ID, ReferenceInvocationIDs: orderedReferenceIDs,
			CaseSourceSHA256: anchors.Source.SHA256, AnchorsSHA256: anchors.sha256, RuntimeManifestSHA256: plan.RuntimeManifestSHA256,
			ProducerABI: s.witness.Native.ProducerABI, DriverSHA256: nativeInvocation.Executable.Member.SHA256,
			ReferenceExecutableSHA256: referenceExecutableSHA, ReferenceLibrarySHA256: s.reg.Native.ReferenceLibrary.Member.SHA256,
			CurrentLibrarySHA256: libraryPin.Member.SHA256, CompilerSHA256: libraryNode.Identities["tool"],
			BuildIdentitySHA256: s.reg.Build.Identity.Member.SHA256, SourceSnapshotSHA256: s.reg.Build.SourceSnapshot.Member.SHA256,
			VendorSnapshotSHA256: s.reg.Build.VendorSnapshot.Member.SHA256, Grammar: slices.Clone(plan.Grammar),
			ApprovalSHA256: core.ApprovalSHA256, RegistrationSHA256: core.RegistrationSHA256, WitnessSHA256: core.WitnessSHA256,
			CatalogueSHA256: core.CatalogueSHA256, GraphSHA256: core.GraphSHA256, Provenance: slices.Clone(s.approval.Provenance), NativeObservationCreated: false,
			EligibilityResultSchema: core.ResultSchema, EligibilityExecutionStatus: core.ExecutionStatus, EligibilityAssessment: core.Assessment, EligibilityComplete: core.Complete, EligibilityTuple: core.Tuple,
		},
		Inventory: inputs.ordinary.coordinates, Definitions: definitions,
	}
	return result, nil
}

type classificationGoDefinitionRowsFixture struct {
	anchors    classificationAnchors
	inputs     classificationGoDefinitionInputs
	witness    []QueryWitnessQuery
	native     []querywire.Query
	references []*instrumentedReference
}

func classificationGoDefinitionFixture(t *testing.T) (classificationGoDefinitionRowsFixture, classificationPlan, []byte, []byte, []byte) {
	t.Helper()
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", "positive-batch")
	anchors, _ := classificationGoAnchors(t)
	inventoryData := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
	qualificationData := actualJSONRead(t, "../contracts/qualification-c1.json")
	queryData := actualJSONRead(t, "../testdata/native/queries/go.json")
	qualification, err := ParseQualificationInventory(qualificationData)
	if err != nil {
		t.Fatal(err)
	}
	var workload *QualWorkload
	for i := range qualification.Routes {
		if qualification.Routes[i].Route == "go" {
			workload = &qualification.Routes[i].Workload
		}
	}
	if workload == nil {
		t.Fatal("Go qualification workload missing")
	}
	plan := classificationPlan{Schema: "tsgk-query-classification-plan/r1", SourceRevision: strings.Repeat("a", 40), RunID: "definition-projection-test-run", AnchorsSHA256: anchors.sha256, APIAnchorsSHA256: anchors.apiSHA, RuntimeManifestSHA256: strings.Repeat("b", 64), DonorRegistrationSHA256: classificationDonorRegistrationSHA, Grammar: slices.Clone(workload.Grammar), CompilerSHA256: strings.Repeat("c", 64)}
	inputs, err := classificationGoDefinitionLoadInputs(anchors, plan, inventoryData, qualificationData, queryData)
	if err != nil {
		t.Fatal(err)
	}
	witness := classificationGoRegistration(anchors)
	native := make([]querywire.Query, len(anchors.Queries))
	references := make([]*instrumentedReference, len(anchors.Queries))
	for i, query := range anchors.Queries {
		meta := classificationGoDefinitionCloneMetadata(query.Metadata)
		native[i] = querywire.Query{ID: query.ID, Status: StatusCompleted, Patterns: meta.Patterns, CaptureNames: meta.CaptureNames, Predicates: meta.Predicates}
		reference := &instrumentedReference{Schema: instrumentedReferenceSchema, Status: StatusCompleted}
		reference.Query.ID, reference.Query.SHA256 = query.ID, query.SHA256
		reference.Query.Patterns, reference.Query.CaptureNames, reference.Query.Predicates = meta.Patterns, classificationGoDefinitionCloneMetadata(meta).CaptureNames, classificationGoDefinitionCloneMetadata(meta).Predicates
		references[i] = reference
	}
	return classificationGoDefinitionRowsFixture{anchors: anchors, inputs: inputs, witness: witness.Queries, native: native, references: references}, plan, inventoryData, qualificationData, queryData
}

func classificationGoDefinitionCloneRowsFixture(f classificationGoDefinitionRowsFixture) classificationGoDefinitionRowsFixture {
	copy := classificationGoDefinitionRowsFixture{
		anchors:    f.anchors,
		inputs:     f.inputs,
		witness:    slices.Clone(f.witness),
		native:     slices.Clone(f.native),
		references: make([]*instrumentedReference, len(f.references)),
	}
	copy.anchors.Queries = slices.Clone(f.anchors.Queries)
	for i := range copy.anchors.Queries {
		copy.anchors.Queries[i].Metadata = classificationGoDefinitionCloneMetadata(f.anchors.Queries[i].Metadata)
	}
	copy.inputs.queryFile.Queries = slices.Clone(f.inputs.queryFile.Queries)
	copy.inputs.slotGroup.Queries = slices.Clone(f.inputs.slotGroup.Queries)
	for i := range copy.witness {
		if f.witness[i].Metadata != nil {
			meta := classificationGoDefinitionCloneMetadata(*f.witness[i].Metadata)
			copy.witness[i].Metadata = &meta
		}
	}
	for i := range copy.native {
		meta := QueryWitnessMetadata{Patterns: f.native[i].Patterns, CaptureNames: f.native[i].CaptureNames, Predicates: f.native[i].Predicates}
		meta = classificationGoDefinitionCloneMetadata(meta)
		copy.native[i].CaptureNames, copy.native[i].Predicates = meta.CaptureNames, meta.Predicates
	}
	for i, reference := range f.references {
		if reference != nil {
			value := *reference
			meta := QueryWitnessMetadata{Patterns: reference.Query.Patterns, CaptureNames: reference.Query.CaptureNames, Predicates: reference.Query.Predicates}
			meta = classificationGoDefinitionCloneMetadata(meta)
			value.Query.CaptureNames, value.Query.Predicates = meta.CaptureNames, meta.Predicates
			copy.references[i] = &value
		}
	}
	return copy
}

func (f classificationGoDefinitionRowsFixture) rows() ([]classificationGoDefinition, error) {
	return classificationGoDefinitionRows(f.anchors, f.inputs, f.witness, f.native, f.references)
}

func TestQueryClassificationDefinitionProjectionMetadata(t *testing.T) {
	fixture, plan, inventoryData, qualificationData, queryData := classificationGoDefinitionFixture(t)
	rows, err := fixture.rows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || fixture.inputs.coordinates.PotentialQueryTreeRoles != 393 {
		t.Fatalf("definition denominator differs: rows=%d coordinates=%d", len(rows), fixture.inputs.coordinates.PotentialQueryTreeRoles)
	}
	wantPatterns := []uint32{4, 2, 3}
	for i, row := range rows {
		want := fixture.anchors.Queries[i]
		if row.Query != want.ID || row.Source != want.Source || row.SourceSHA256 != want.SHA256 || row.Metadata.Patterns != wantPatterns[i] || !reflect.DeepEqual(row.Metadata, want.Metadata) || row.Metadata.Predicates == nil || len(row.Metadata.Predicates) != 0 || row.Class != "STRUCTURAL_ONLY" || row.InventoryClass != "STRUCTURAL_CANDIDATE" || row.CompiledMetadata != "UNVERIFIED" || row.Assessment != "NOT_ASSESSED" || row.SemanticRequired != nil || row.SemanticEvaluated || row.PotentialTreeRoles != 131 {
			t.Fatalf("definition row %d differs: %#v", i, row)
		}
	}
	reader := strings.Repeat("d", 40)
	if _, err := classificationGoDefinitionProjection(fixture.anchors, plan, nil, nil, QueryReferenceEligibilityResult{}, inventoryData, qualificationData, queryData, reader); err == nil || !strings.Contains(err.Error(), "live guarded eligibility session") {
		t.Fatalf("saved/core-only projection accepted without the live guarded session: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*classificationGoDefinitionRowsFixture)
	}{
		{"anchor-predicates-null", func(f *classificationGoDefinitionRowsFixture) { f.anchors.Queries[0].Metadata.Predicates = nil }},
		{"anchor-capture-names-null", func(f *classificationGoDefinitionRowsFixture) { f.anchors.Queries[0].Metadata.CaptureNames = nil }},
		{"anchor-pattern-count", func(f *classificationGoDefinitionRowsFixture) { f.anchors.Queries[0].Metadata.Patterns++ }},
		{"witness-metadata-missing", func(f *classificationGoDefinitionRowsFixture) { f.witness[0].Metadata = nil }},
		{"witness-predicates-null", func(f *classificationGoDefinitionRowsFixture) { f.witness[0].Metadata.Predicates = nil }},
		{"native-predicates-null", func(f *classificationGoDefinitionRowsFixture) { f.native[0].Predicates = nil }},
		{"native-predicate-present", func(f *classificationGoDefinitionRowsFixture) { f.native[0].Predicates = []QueryPredicate{{}} }},
		{"reference-predicates-null", func(f *classificationGoDefinitionRowsFixture) { f.references[0].Query.Predicates = nil }},
		{"capture-order", func(f *classificationGoDefinitionRowsFixture) {
			f.native[0].CaptureNames[0], f.native[0].CaptureNames[1] = f.native[0].CaptureNames[1], f.native[0].CaptureNames[0]
		}},
		{"duplicate-source-query", func(f *classificationGoDefinitionRowsFixture) {
			f.inputs.queryFile.Queries[1].ID = f.inputs.queryFile.Queries[0].ID
		}},
		{"inventory-status-upgrade", func(f *classificationGoDefinitionRowsFixture) {
			f.inputs.slotGroup.Queries[0].CompiledMetadata = "VERIFIED"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := classificationGoDefinitionCloneRowsFixture(fixture)
			test.mutate(&changed)
			if _, err := changed.rows(); err == nil {
				t.Fatal("changed metadata, query identity, or r1 status accepted")
			}
		})
	}
}

func TestQueryClassificationDefinitionCoordinateProjection(t *testing.T) {
	fixture, plan, inventoryData, qualificationData, queryData := classificationGoDefinitionFixture(t)
	got := fixture.inputs.coordinates
	if got.Cases != 57 || got.Edits != 37 || got.CaseSteps != 94 || got.TreeRoles != 131 || got.Definitions != 3 || got.PotentialQueryTreeRoles != 393 || got.TreeRolesPerDefinition != 131 || got.QualificationInventorySHA256 != classificationGoDefinitionQualificationSHA || got.QuerySlotInventorySHA256 != classificationGoDefinitionInventorySHA || got.QuerySourceFileSHA256 != classificationGoDefinitionQueriesSHA || !hex64.MatchString(got.CoordinatesSHA256) {
		t.Fatalf("registered query coordinate summary differs: %#v", got)
	}
	ids := []string{"q.structure", "q.probe-v18", "q.probe-v27"}
	duplicateInput := QualWorkload{Set: "synthetic", Cases: []QualCase{
		{ID: "case-a", Input: NativeInput{SHA256: "same-input"}, Edits: []Edit{}},
		{ID: "case-b", Input: NativeInput{SHA256: "same-input"}, Edits: []Edit{}},
	}}
	duplicateSummary, err := classificationGoDefinitionCoordinateProjection(strings.Repeat("a", 64), "synthetic", duplicateInput, ids)
	if err != nil || duplicateSummary.Cases != 2 || duplicateSummary.TreeRoles != 2 || duplicateSummary.PotentialQueryTreeRoles != 6 {
		t.Fatalf("duplicate-input case IDs were deduplicated: %#v, %v", duplicateSummary, err)
	}
	changed := fixture.inputs.workload
	changed.Cases = slices.Clone(changed.Cases)
	changed.Cases[0].ID, changed.Cases[1].ID = changed.Cases[1].ID, changed.Cases[0].ID
	changedSummary, err := classificationGoDefinitionCoordinateProjection(fixture.inputs.qualification.SHA256, changed.Set, changed, ids)
	if err != nil || changedSummary.Cases != got.Cases || changedSummary.Edits != got.Edits || changedSummary.PotentialQueryTreeRoles != got.PotentialQueryTreeRoles || changedSummary.CoordinatesSHA256 == got.CoordinatesSHA256 {
		t.Fatalf("same-count case identity swap did not change the coordinate digest: %#v, %v", changedSummary, err)
	}

	// Swap only two raw case-ID tokens, preserving all formatting and numeric counts.
	a, _ := jsonv2.Marshal(fixture.inputs.workload.Cases[0].ID)
	b, _ := jsonv2.Marshal(fixture.inputs.workload.Cases[1].ID)
	marker := []byte(`"definition-unit-temporary-token"`)
	if bytes.Count(qualificationData, a) != 1 || bytes.Count(qualificationData, b) != 1 || bytes.Contains(qualificationData, marker) {
		t.Fatal("case ID tokens are not unique for the raw-byte counterexample")
	}
	mutatedQualification := bytes.Replace(qualificationData, a, marker, 1)
	mutatedQualification = bytes.Replace(mutatedQualification, b, a, 1)
	mutatedQualification = bytes.Replace(mutatedQualification, marker, b, 1)
	if _, err := classificationGoDefinitionLoadInputs(fixture.anchors, plan, inventoryData, mutatedQualification, queryData); err == nil || err.Error() != "qualification inventory pin differs" {
		t.Fatalf("same-count raw case ID swap passed without the exact qualification source pin: %v", err)
	}
	changedPlan := plan
	changedPlan.Grammar = slices.Clone(plan.Grammar)
	changedPlan.Grammar[0].SHA256 = strings.Repeat("e", 64)
	if _, err := classificationGoDefinitionLoadInputs(fixture.anchors, changedPlan, inventoryData, qualificationData, queryData); err == nil || !strings.Contains(err.Error(), "grammar pins differ") {
		t.Fatal("changed preregistered grammar accepted", err)
	}
}

func TestQueryClassificationDefinitionLiveBinding(t *testing.T) {
	fixture, plan, inventoryData, qualificationData, queryData := classificationGoDefinitionFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	defer cancel()
	r, err := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if err != nil {
		t.Fatal(err)
	}
	defer r.cancel()
	core := QueryReferenceEligibilityResult{ResultSchema: QueryReferenceEligibilityResultSchema, Report: Report{ExecutionStatus: StatusCompleted, Assessment: AssessPass}, Complete: true, Tuple: QueryEligibilityCounts{Required: 3, Verified: 3}}
	owned, other := &semanticNativeCache{ready: true}, &semanticNativeCache{ready: true}
	owned.record.haveRaw, other.record.haveRaw = true, true
	session := &eligibilitySession{r: r, res: &core, semantic: owned, bound: true}
	reader := strings.Repeat("d", 40)
	if _, err := classificationGoDefinitionProjection(fixture.anchors, plan, session, other, core, inventoryData, qualificationData, queryData, reader); err == nil || !strings.Contains(err.Error(), "live guarded eligibility session") {
		t.Fatal("foreign live cache accepted", err)
	}
	foreign := core
	foreign.GraphSHA256 = strings.Repeat("e", 64)
	if _, err := classificationGoDefinitionProjection(fixture.anchors, plan, session, owned, foreign, inventoryData, qualificationData, queryData, reader); err == nil || !strings.Contains(err.Error(), "same completed guarded result") {
		t.Fatal("foreign guarded result accepted", err)
	}
	if _, err := classificationGoDefinitionProjection(fixture.anchors, plan, nil, nil, core, inventoryData, qualificationData, queryData, strings.Repeat("d", 64)); err == nil || !strings.Contains(err.Error(), "reader revision is invalid") {
		t.Fatal("non-commit reader revision accepted", err)
	}
}

type classificationCSharpDefinitionRowsFixture struct {
	anchors    classificationAnchors
	inputs     classificationCSharpDefinitionInputs
	witness    []QueryWitnessQuery
	native     []querywire.Query
	references []*instrumentedReference
}

func classificationCSharpDefinitionFixture(t *testing.T) (classificationCSharpDefinitionRowsFixture, classificationPlan, []byte, []byte, []byte) {
	t.Helper()
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", "csharp15-empty")
	anchors, _ := classificationGoAnchors(t)
	inventoryData := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
	qualificationData := actualJSONRead(t, "../contracts/qualification-c1.json")
	queryFile := actualJSONRead(t, "../testdata/native/queries/csharp.json")
	factPack := actualJSONRead(t, "../contracts/fact-query-pack.json")
	queryData, err := jsonv2.Marshal(classificationCSharpDefinitionQueryInputs{QueryFile: queryFile, FactQueryPack: factPack})
	if err != nil {
		t.Fatal(err)
	}
	qualification, err := ParseQualificationInventory(qualificationData)
	if err != nil {
		t.Fatal(err)
	}
	var workload *QualWorkload
	for i := range qualification.Routes {
		if qualification.Routes[i].Route == "csharp" {
			workload = &qualification.Routes[i].Workload
		}
	}
	if workload == nil {
		t.Fatal("C# qualification workload missing")
	}
	plan := classificationPlan{
		NativeContext: semanticPagedContext, Schema: "tsgk-query-classification-plan/r1",
		SourceRevision: strings.Repeat("a", 40), RunID: "definition-csharp-test-run", AnchorsSHA256: anchors.sha256,
		RuntimeManifestSHA256: strings.Repeat("b", 64), DonorRegistrationSHA256: classificationDonorRegistrationSHA,
		Grammar: slices.Clone(workload.Grammar), CompilerSHA256: strings.Repeat("c", 64),
	}
	inputs, err := classificationCSharpDefinitionLoadInputs(anchors, plan, inventoryData, qualificationData, queryData)
	if err != nil {
		t.Fatal(err)
	}
	witness := classificationGoRegistration(anchors)
	native := make([]querywire.Query, len(anchors.Queries))
	references := make([]*instrumentedReference, len(anchors.Queries))
	for i, query := range anchors.Queries {
		metadata := classificationGoDefinitionCloneMetadata(query.Metadata)
		native[i] = querywire.Query{ID: query.ID, Status: StatusCompleted, Patterns: metadata.Patterns, CaptureNames: metadata.CaptureNames, Predicates: metadata.Predicates}
		reference := &instrumentedReference{Schema: instrumentedReferenceSchema, Status: StatusCompleted}
		reference.Query.ID, reference.Query.SHA256 = query.ID, query.SHA256
		reference.Query.Patterns = metadata.Patterns
		reference.Query.CaptureNames = classificationGoDefinitionCloneMetadata(metadata).CaptureNames
		reference.Query.Predicates = classificationGoDefinitionCloneMetadata(metadata).Predicates
		references[i] = reference
	}
	return classificationCSharpDefinitionRowsFixture{
		anchors: anchors, inputs: inputs, witness: witness.Queries, native: native, references: references,
	}, plan, inventoryData, qualificationData, queryData
}

func classificationCSharpCloneRowsFixture(f classificationCSharpDefinitionRowsFixture) classificationCSharpDefinitionRowsFixture {
	copy := classificationCSharpDefinitionRowsFixture{
		anchors: f.anchors, inputs: f.inputs, witness: slices.Clone(f.witness), native: slices.Clone(f.native),
		references: make([]*instrumentedReference, len(f.references)),
	}
	copy.anchors.Queries = slices.Clone(f.anchors.Queries)
	for i := range copy.anchors.Queries {
		copy.anchors.Queries[i].Metadata = classificationGoDefinitionCloneMetadata(f.anchors.Queries[i].Metadata)
	}
	copy.inputs.ordinary.queryFile.Queries = slices.Clone(f.inputs.ordinary.queryFile.Queries)
	copy.inputs.ordinary.slotGroup.Queries = slices.Clone(f.inputs.ordinary.slotGroup.Queries)
	copy.inputs.ordinary.workload.Queries = slices.Clone(f.inputs.ordinary.workload.Queries)
	copy.inputs.ordinary.coordinates.QuerySourceFiles = slices.Clone(f.inputs.ordinary.coordinates.QuerySourceFiles)
	copy.inputs.sourceFiles = slices.Clone(f.inputs.sourceFiles)
	for i := range copy.witness {
		if f.witness[i].Metadata != nil {
			metadata := classificationGoDefinitionCloneMetadata(*f.witness[i].Metadata)
			copy.witness[i].Metadata = &metadata
		}
	}
	for i := range copy.native {
		metadata := classificationGoDefinitionCloneMetadata(QueryWitnessMetadata{Patterns: f.native[i].Patterns, CaptureNames: f.native[i].CaptureNames, Predicates: f.native[i].Predicates})
		copy.native[i].CaptureNames, copy.native[i].Predicates = metadata.CaptureNames, metadata.Predicates
	}
	for i, reference := range f.references {
		if reference != nil {
			value := *reference
			metadata := classificationGoDefinitionCloneMetadata(QueryWitnessMetadata{Patterns: reference.Query.Patterns, CaptureNames: reference.Query.CaptureNames, Predicates: reference.Query.Predicates})
			value.Query.CaptureNames, value.Query.Predicates = metadata.CaptureNames, metadata.Predicates
			copy.references[i] = &value
		}
	}
	return copy
}

func (f classificationCSharpDefinitionRowsFixture) rows() ([]classificationGoDefinition, error) {
	return classificationCSharpDefinitionRows(f.anchors, f.inputs, f.witness, f.native, f.references)
}

func TestQueryClassificationCSharpDefinitionProjectionMetadata(t *testing.T) {
	fixture, plan, inventoryData, qualificationData, queryData := classificationCSharpDefinitionFixture(t)
	rows, err := fixture.rows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 15 || fixture.inputs.ordinary.coordinates.Cases != 390 || fixture.inputs.ordinary.coordinates.Edits != 125 || fixture.inputs.ordinary.coordinates.CaseSteps != 515 || fixture.inputs.ordinary.coordinates.TreeRoles != 640 || fixture.inputs.ordinary.coordinates.Definitions != 15 || fixture.inputs.ordinary.coordinates.TreeRolesPerDefinition != 640 || fixture.inputs.ordinary.coordinates.PotentialQueryTreeRoles != 9600 || fixture.inputs.ordinary.coordinates.QuerySourceFileSHA256 != "" || len(fixture.inputs.ordinary.coordinates.QuerySourceFiles) != 2 || plan.NativeContext != semanticPagedContext || len(plan.Grammar) != 5 || plan.Grammar[0].Path != "src/parser.c" || plan.Grammar[0].Role != "parser" || plan.Grammar[0].Bytes != 51472926 || plan.Grammar[0].SHA256 != "b1a77b75e034af978ebbde75be71e362cae447f67ec25dcf297702cdca9a1dbb" {
		t.Fatalf("C# definition denominator or dual source pins differ: rows=%d inventory=%#v", len(rows), fixture.inputs.ordinary.coordinates)
	}
	if fixture.inputs.ordinary.coordinates.QuerySourceFiles[0].Path != "src/testdata/native/queries/csharp.json" || fixture.inputs.ordinary.coordinates.QuerySourceFiles[1].Path != "src/contracts/fact-query-pack.json" {
		t.Fatalf("C# query source aliases differ: %#v", fixture.inputs.ordinary.coordinates.QuerySourceFiles)
	}
	var patterns, captureNames uint64
	for i, row := range rows {
		want := fixture.anchors.Queries[i]
		wantSource := "src/testdata/native/queries/csharp.json"
		if i >= 13 {
			wantSource = "src/contracts/fact-query-pack.json"
		}
		patterns += uint64(row.Metadata.Patterns)
		captureNames += uint64(len(row.Metadata.CaptureNames))
		if row.Query != want.ID || row.Source != want.Source || row.SourceSHA256 != want.SHA256 || row.SourceFile != wantSource || !reflect.DeepEqual(row.Metadata, want.Metadata) || row.Metadata.Predicates == nil || len(row.Metadata.Predicates) != 0 || row.Class != "STRUCTURAL_ONLY" || row.InventoryClass != "STRUCTURAL_CANDIDATE" || row.CompiledMetadata != "UNVERIFIED" || row.Assessment != "NOT_ASSESSED" || row.SemanticRequired != nil || row.SemanticEvaluated || row.PotentialTreeRoles != 640 {
			t.Fatalf("C# definition row %d differs: %#v", i, row)
		}
	}
	if patterns != 91 || captureNames != 125 {
		t.Fatalf("C# aggregate query metadata differs: patterns=%d captures=%d", patterns, captureNames)
	}
	reader := strings.Repeat("d", 40)
	if _, err := classificationCSharpDefinitionProjection(fixture.anchors, plan, nil, nil, QueryReferenceEligibilityResult{}, inventoryData, qualificationData, queryData, reader); err == nil || !strings.Contains(err.Error(), "live guarded eligibility session") {
		t.Fatalf("synthetic metadata projection became a live PASS without guarded evidence: %v", err)
	}
}

func TestQueryClassificationCSharpDefinitionBindings(t *testing.T) {
	fixture, plan, inventoryData, qualificationData, queryData := classificationCSharpDefinitionFixture(t)
	badPlan := plan
	badPlan.NativeContext = "full-input-utf8/r2"
	if _, err := classificationCSharpDefinitionLoadInputs(fixture.anchors, badPlan, inventoryData, qualificationData, queryData); err == nil || !strings.Contains(err.Error(), "paged single-frame anchor") {
		t.Fatalf("non-paged preregistration accepted: %v", err)
	}
	badGrammar := plan
	badGrammar.Grammar = slices.Clone(plan.Grammar)
	badGrammar.Grammar[0].Bytes--
	if _, err := classificationCSharpDefinitionLoadInputs(fixture.anchors, badGrammar, inventoryData, qualificationData, queryData); err == nil || !strings.Contains(err.Error(), "grammar pins differ") {
		t.Fatalf("changed original logical C# parser pin accepted: %v", err)
	}
	var envelope classificationCSharpDefinitionQueryInputs
	if err := jsonv2.Unmarshal(queryData, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.FactQueryPack = slices.Clone(envelope.FactQueryPack)
	envelope.FactQueryPack[len(envelope.FactQueryPack)-1] ^= 1
	changedQueryData, err := jsonv2.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := classificationCSharpDefinitionLoadInputs(fixture.anchors, plan, inventoryData, qualificationData, changedQueryData); err == nil || !strings.Contains(err.Error(), "exact query source or fact pack pin") {
		t.Fatalf("changed fact pack bytes accepted: %v", err)
	}
	changedAnchor := fixture.anchors
	changedAnchor.Queries = slices.Clone(fixture.anchors.Queries)
	changedAnchor.Queries[13].ID, changedAnchor.Queries[14].ID = changedAnchor.Queries[14].ID, changedAnchor.Queries[13].ID
	if _, err := classificationCSharpDefinitionLoadInputs(changedAnchor, plan, inventoryData, qualificationData, queryData); err == nil {
		t.Fatal("reordered fact query definitions accepted")
	}
	for _, test := range []struct {
		name   string
		mutate func(*classificationCSharpDefinitionRowsFixture)
	}{
		{"anchor-predicates-null", func(f *classificationCSharpDefinitionRowsFixture) { f.anchors.Queries[0].Metadata.Predicates = nil }},
		{"witness-metadata-missing", func(f *classificationCSharpDefinitionRowsFixture) { f.witness[13].Metadata = nil }},
		{"witness-predicates-null", func(f *classificationCSharpDefinitionRowsFixture) { f.witness[13].Metadata.Predicates = nil }},
		{"native-predicates-null", func(f *classificationCSharpDefinitionRowsFixture) { f.native[13].Predicates = nil }},
		{"native-predicate-present", func(f *classificationCSharpDefinitionRowsFixture) { f.native[13].Predicates = []QueryPredicate{{}} }},
		{"reference-predicates-null", func(f *classificationCSharpDefinitionRowsFixture) { f.references[14].Query.Predicates = nil }},
		{"source-file-alias", func(f *classificationCSharpDefinitionRowsFixture) {
			f.inputs.ordinary.slotGroup.Queries[13].SourceFile = "src/testdata/native/queries/csharp.json"
		}},
		{"duplicate-source-query", func(f *classificationCSharpDefinitionRowsFixture) {
			f.inputs.ordinary.queryFile.Queries[1].ID = f.inputs.ordinary.queryFile.Queries[0].ID
		}},
		{"inventory-status-upgrade", func(f *classificationCSharpDefinitionRowsFixture) {
			f.inputs.ordinary.slotGroup.Queries[0].CompiledMetadata = "VERIFIED"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := classificationCSharpCloneRowsFixture(fixture)
			test.mutate(&changed)
			if _, err := changed.rows(); err == nil {
				t.Fatal("changed C# metadata, source alias, order, or r1 status accepted")
			}
		})
	}
}
