package kit

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

const classificationCPP4AnchorsSHA = "e807345cb9a528facbaddaae3b914027acc4c99c8148f65f365e83f4b58ee5b4"

type classificationCPP4QuerySpec struct {
	id       string
	sha256   string
	patterns uint32
	captures []string
}

type classificationCPP4Profile struct {
	AnchorFile, AnchorSHA string
	Route, Case, Workload string
	QueryFile, QuerySHA   string
	Set, Symbol, RootType string
	QueryBytes            int
	QueryCount            int
	Grammar               []NativeInput
	ABI                   eligibilityProducerABI
	Counts                classificationGoDefinitionSlotCounts
	Queries               []classificationCPP4QuerySpec
}

func classificationCPP4DefinitionProfile() classificationCPP4Profile {
	return classificationCPP4Profile{
		AnchorFile: "cpp4-classification-anchors.json", AnchorSHA: classificationCPP4AnchorsSHA,
		Route: "cpp", Case: "cpp4-empty-definitions", Workload: "cpp4-empty-definition-single-frame",
		QueryFile: "src/testdata/native/queries/cpp.json", QuerySHA: "fc90268c86c9133e2801b2fb7a20bda479137aacc0445911fa5046f11fd65766",
		Set: "s06-cpp", Symbol: "tree_sitter_cpp", RootType: "translation_unit", QueryBytes: 4734, QueryCount: 4,
		Grammar: classificationCPP4GrammarPins(),
		ABI:     eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: 621, FieldCount: 53},
		Counts:  classificationGoDefinitionSlotCounts{Cases: 77, Edits: 13, CaseSteps: 90, TreeRoles: 103, PotentialQueryTreeRoles: 412},
		Queries: []classificationCPP4QuerySpec{
			{id: "q.structure", sha256: "2a57e6e5cb3c6136878e5d27ced7c7a090d491ec2e9e5e3d51f6305c6227945e", patterns: 4, captures: []string{"namespace", "struct", "concept", "class"}},
			{id: "q.probe-cpp-V14", sha256: "1077f4c6e42da4eb404eb4a819b5338f3c2aeeabbfeb1605678e3a4dd3d28f31", patterns: 3, captures: []string{"init", "vartemplate", "num"}},
			{id: "q.probe-cpp-V17", sha256: "29e660f47b5bb900c4321ea914fe6ea74f94d4662912773075e3aac7bf744c49", patterns: 2, captures: []string{"ns", "binding"}},
			{id: "q.probe-cpp-V23", sha256: "fde3be27ac5b9a89fa6e366ed03a24f035e1f21d0ea4434a80e1d666aef3a467", patterns: 2, captures: []string{"self", "index"}},
		},
	}
}

func classificationCPP4GrammarPins() []NativeInput {
	return []NativeInput{
		{Path: "src/parser.c", Role: "parser", Bytes: 49598509, SHA256: "3856d51ead4b6ecf5d4e1ec3fd39c52c9ba0cb535e69ea1785822280c45b75fc"},
		{Path: "src/scanner.c", Role: "scanner", Bytes: 6772, SHA256: "55b66d60ce5551e354112bc0c050237e786402764d59893d0296ac5f88176546"},
		{Path: "src/tree_sitter/alloc.h", Role: "header", Bytes: 985, SHA256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
		{Path: "src/tree_sitter/array.h", Role: "header", Bytes: 13453, SHA256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
		{Path: "src/tree_sitter/parser.h", Role: "header", Bytes: 7624, SHA256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
	}
}

func classificationCPP4GrammarValid(grammar []NativeInput) bool {
	return reflect.DeepEqual(grammar, classificationCPP4GrammarPins())
}

func classificationCPP4AnchorsValid(a classificationAnchors) error {
	p := classificationCPP4DefinitionProfile()
	if a.sha256 != p.AnchorSHA || a.Schema != "tsgk-query-classification-anchors/r1" || a.Route != p.Route || a.Case != p.Case || a.Workload != p.Workload ||
		a.Operation != "native-query" || a.DriverMode != "single" || a.API || a.Source.Bytes != 0 || a.Source.UTF8 != "" || a.Source.SHA256 != digestHex(nil) ||
		a.Edits == nil || len(a.Edits) != 0 || len(a.Queries) != p.QueryCount || len(a.Required) != p.QueryCount || len(a.Nodes) != 1 ||
		!reflect.DeepEqual(a.Expect, []StepExpectation{{Step: 0, Syntax: "NO_ERROR"}}) || a.ABI != p.ABI {
		return fmt.Errorf("CPP4 empty single-frame anchor changed")
	}
	if !reflect.DeepEqual(a.Nodes, []TreeNode{{Parent: -1, Type: p.RootType, Named: true}}) {
		return fmt.Errorf("CPP4 independent empty-source root changed")
	}
	seen := make(map[string]bool, p.QueryCount)
	for i, query := range a.Queries {
		want := p.Queries[i]
		if query.ID != want.id || seen[query.ID] || query.SHA256 != want.sha256 || query.SHA256 != digestHex([]byte(query.Source)) ||
			query.Metadata.Patterns != want.patterns || !reflect.DeepEqual(query.Metadata.CaptureNames, want.captures) || !classificationGoDefinitionMetadataValid(query.Metadata) {
			return fmt.Errorf("CPP4 query source, order, or compiled-metadata expectation changed at %d", i)
		}
		seen[query.ID] = true
		tuple := a.Required[i]
		if tuple.Query != query.ID || tuple.Step != 0 || tuple.TreeRole != "incremental" || tuple.Structural == nil || len(tuple.Structural) != 0 || tuple.Expected == nil || len(tuple.Expected) != 0 {
			return fmt.Errorf("CPP4 empty capture denominator changed at %d", i)
		}
	}
	return nil
}

func classificationCPP4DefinitionPlanValid(a classificationAnchors, plan classificationPlan) error {
	p := classificationCPP4DefinitionProfile()
	if err := classificationCPP4AnchorsValid(a); err != nil {
		return err
	}
	if plan.Schema != "tsgk-query-classification-plan/r1" || plan.AnchorsSHA256 != p.AnchorSHA || plan.APIAnchorsSHA256 != "" || plan.NativeContext != semanticPagedContext ||
		!eligibilityRevision(plan.SourceRevision) || !validID(plan.RunID) || !hex64.MatchString(plan.RuntimeManifestSHA256) || !hex64.MatchString(plan.CompilerSHA256) ||
		!classificationCPP4GrammarValid(plan.Grammar) {
		return fmt.Errorf("CPP4 preregistration differs from its closed paged profile")
	}
	preparedRoot := os.Getenv("TSGK_BENCH_PREPARED")
	if !filepath.IsAbs(preparedRoot) {
		return fmt.Errorf("CPP4 prepared grammar root must be absolute")
	}
	return classificationCPP4LibraryRequestValid(plan, plan.LibraryRequest.Dir, filepath.Join(preparedRoot, "routes", p.Route))
}

func classificationCPP4LibraryRequestValid(plan classificationPlan, root, grammarRoot string) error {
	request := plan.LibraryRequest
	output := filepath.Join(root, "library", "cpp.dll")
	if !hex64.MatchString(plan.CompilerSHA256) || !filepath.IsAbs(root) || !filepath.IsAbs(grammarRoot) || request.Dir != root || !eligibilityAbsolutePath(request.Path, "windows/amd64") ||
		eligibilityValidateRequest(request, "go-library") != nil ||
		request.Memory.Bytes != 4<<30 || !request.Memory.Hard || request.Wall != 120e9 || request.Grace != 2e9 || request.StdoutBytes != 16<<20 || request.StderrBytes != 4<<20 {
		return fmt.Errorf("CPP4 compiler request path or bounded limits differ")
	}
	wantArgs, err := classificationGoCompilerArgs("cpp", grammarRoot, output, plan.Grammar)
	if err != nil || !reflect.DeepEqual(request.Args, wantArgs) {
		return fmt.Errorf("CPP4 canonical compiler argv differs: %v", err)
	}
	return nil
}

func classificationCPP4DefinitionLoadInputs(a classificationAnchors, plan classificationPlan, inventoryData, qualificationData, queryData []byte) (classificationGoDefinitionInputs, error) {
	var out classificationGoDefinitionInputs
	p := classificationCPP4DefinitionProfile()
	if err := classificationCPP4DefinitionPlanValid(a, plan); err != nil {
		return out, err
	}
	if digestHex(inventoryData) != classificationGoDefinitionInventorySHA || digestHex(qualificationData) != classificationGoDefinitionQualificationSHA {
		return out, fmt.Errorf("query slot or qualification inventory pin differs")
	}
	if len(queryData) != p.QueryBytes || digestHex(queryData) != p.QuerySHA {
		return out, fmt.Errorf("CPP4 ordinary query source file pin differs")
	}
	var slots classificationGoDefinitionSlotInventory
	if err := jsonv2.Unmarshal(inventoryData, &slots, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, fmt.Errorf("query slot inventory shape: %w", err)
	}
	if slots.Schema != "tsgk-query-slot-inventory/r1" || slots.ExpansionRule != "case-query-edit-tree-role/r1" ||
		!slices.Equal(slots.Coordinate, []string{"qualification_inventory.sha256", "set", "case", "query", "step", "tree_role"}) || len(slots.Ordinary) != 26 ||
		!bytes.Equal(bytes.TrimSpace(slots.SemanticRequired), []byte("null")) {
		return out, fmt.Errorf("query slot inventory r1 contract differs")
	}
	if slots.Qualification.Path != "src/contracts/qualification-c1.json" || slots.Qualification.Bytes != uint64(len(qualificationData)) || slots.Qualification.SHA256 != classificationGoDefinitionQualificationSHA {
		return out, fmt.Errorf("slot qualification pin differs")
	}
	if err := jsonv2.Unmarshal(queryData, &out.queryFile, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, fmt.Errorf("CPP4 query file shape: %w", err)
	}
	if out.queryFile.Schema != "tsgk-native-query-cases/r1" || out.queryFile.Route != p.Route || len(out.queryFile.Queries) != p.QueryCount {
		return out, fmt.Errorf("CPP4 query file identity differs")
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
		if inv.Routes[i].Route == p.Route {
			routeCount++
			route = &inv.Routes[i]
		}
	}
	if routeCount != 1 || route == nil {
		return out, fmt.Errorf("CPP4 qualification route is missing or duplicated")
	}
	w := route.Workload
	if w.Set != p.Set || w.Profile != p.Set || w.Route != p.Route || w.Operation != "native-query-batch" || w.Output != "tree" || w.Symbol != p.Symbol ||
		!w.API || w.FactPack != nil || len(w.Grammar) != len(p.Grammar) || len(w.Queries) != p.QueryCount || !reflect.DeepEqual(w.Grammar, plan.Grammar) {
		return out, fmt.Errorf("CPP4 ordinary qualification workload differs")
	}
	for _, extra := range inv.ExtraRoles {
		for _, workload := range extra.Workloads {
			if workload.Route == p.Route && workload.Set == p.Set {
				return out, fmt.Errorf("CPP4 workload is duplicated in an extra role")
			}
		}
	}
	groupMatches := 0
	for _, group := range slots.Ordinary {
		if group.Route == p.Route || group.Set == p.Set {
			if group.Route != p.Route || group.Set != p.Set || group.Profile != p.Set {
				return out, fmt.Errorf("CPP4 slot group is aliased under another route or profile")
			}
			groupMatches++
			out.slotGroup = group
		}
	}
	if groupMatches != 1 || out.slotGroup.Profile != p.Set || out.slotGroup.Operation != "native-query-batch" || len(out.slotGroup.Queries) != p.QueryCount || out.slotGroup.Counts != p.Counts {
		return out, fmt.Errorf("CPP4 ordinary query group or fixed counts differ")
	}
	sourcePinMatches := 0
	for _, pin := range slots.QuerySources {
		if pin.Path == p.QueryFile {
			sourcePinMatches++
			if pin.Bytes != uint64(p.QueryBytes) || pin.SHA256 != p.QuerySHA {
				return out, fmt.Errorf("CPP4 query source inventory pin differs")
			}
		}
	}
	if sourcePinMatches != 1 {
		return out, fmt.Errorf("CPP4 query source inventory pin is missing or duplicated")
	}
	queryIDs := make([]string, p.QueryCount)
	for i, anchor := range a.Queries {
		query := out.queryFile.Queries[i]
		registered := w.Queries[i]
		slot := out.slotGroup.Queries[i]
		sha := digestHex([]byte(anchor.Source))
		if !classificationGoDefinitionMetadataValid(anchor.Metadata) || anchor.SHA256 != sha || query.ID != anchor.ID || query.Source != anchor.Source || digestHex([]byte(query.Source)) != sha ||
			registered.ID != anchor.ID || registered.SHA256 != sha || slot.ID != anchor.ID || slot.SHA256 != sha || slot.SourceFile != p.QueryFile || slot.Class != "STRUCTURAL_CANDIDATE" || slot.CompiledMetadata != "UNVERIFIED" {
			return out, fmt.Errorf("CPP4 query %s is not linked in the exact registered order", anchor.ID)
		}
		queryIDs[i] = anchor.ID
	}
	coordinates, err := classificationGoDefinitionCoordinateProjection(inv.SHA256, p.Set, w, queryIDs)
	if err != nil {
		return out, err
	}
	coordinates.QuerySlotInventorySHA256 = classificationGoDefinitionInventorySHA
	coordinates.QuerySourceFileSHA256 = p.QuerySHA
	if coordinates.Cases != p.Counts.Cases || coordinates.Edits != p.Counts.Edits || coordinates.CaseSteps != p.Counts.CaseSteps || coordinates.TreeRoles != p.Counts.TreeRoles ||
		coordinates.Definitions != uint64(p.QueryCount) || coordinates.PotentialQueryTreeRoles != p.Counts.PotentialQueryTreeRoles || coordinates.TreeRolesPerDefinition != p.Counts.TreeRoles {
		return out, fmt.Errorf("CPP4 query coordinate denominator differs")
	}
	out.qualification, out.workload, out.coordinates = inv, w, coordinates
	return out, nil
}

func classificationCPP4DefinitionRows(a classificationAnchors, inputs classificationGoDefinitionInputs, witnessQueries []QueryWitnessQuery, nativeQueries []querywire.Query, references []*instrumentedReference) ([]classificationGoDefinition, error) {
	if err := classificationCPP4AnchorsValid(a); err != nil {
		return nil, err
	}
	p := classificationCPP4DefinitionProfile()
	sourceFiles := make([]string, p.QueryCount)
	for i := range sourceFiles {
		sourceFiles[i] = p.QueryFile
	}
	return classificationDefinitionRows(a, inputs, witnessQueries, nativeQueries, references, "C++", sourceFiles)
}

func classificationCPP4OrderedReferenceIDs(a classificationAnchors, referenceIDs map[string]string) ([]string, error) {
	if len(a.Queries) != 4 || len(referenceIDs) != len(a.Queries) {
		return nil, fmt.Errorf("CPP4 reference invocation denominator differs")
	}
	ordered := make([]string, len(a.Queries))
	seen := make(map[string]bool, len(a.Queries))
	for i, query := range a.Queries {
		id := referenceIDs[query.ID]
		if !validID(id) || seen[id] {
			return nil, fmt.Errorf("CPP4 reference invocation is missing, invalid, or reused: %s", query.ID)
		}
		seen[id] = true
		ordered[i] = id
	}
	return ordered, nil
}

// This projection is called only from the callback after classificationGoVerifyWith has
// completed the guarded eligibility, paged grammar, native frame, compiler, and public parity checks.
func classificationCPP4DefinitionProjection(anchors classificationAnchors, plan classificationPlan, s *eligibilitySession, cache *semanticNativeCache, core QueryReferenceEligibilityResult, inventoryData, qualificationData, queryData []byte, readerRevision string) (classificationGoDefinitionProjectionResult, error) {
	var result classificationGoDefinitionProjectionResult
	inputs, err := classificationCPP4DefinitionLoadInputs(anchors, plan, inventoryData, qualificationData, queryData)
	if err != nil {
		return result, err
	}
	if !hex40.MatchString(readerRevision) {
		return result, fmt.Errorf("separately pinned reader revision is invalid")
	}
	if s == nil || cache == nil || s.res == nil || s.r == nil || s.semantic != cache || !s.bound || !cache.ready || !cache.record.haveRaw {
		return result, fmt.Errorf("CPP4 definition projection requires the live guarded eligibility session")
	}
	if err := s.r.check(); err != nil {
		return result, err
	}
	wantTuple := QueryEligibilityCounts{Required: 4, Verified: 4}
	if core.ResultSchema != QueryReferenceEligibilityResultSchema || core.ExecutionStatus != StatusCompleted || core.Assessment != AssessPass || !core.Complete || core.Tuple != wantTuple ||
		s.res.ResultSchema != core.ResultSchema || s.res.ExecutionStatus != core.ExecutionStatus || s.res.Assessment != core.Assessment || s.res.Complete != core.Complete || s.res.Tuple != wantTuple ||
		s.res.ApprovalSHA256 != core.ApprovalSHA256 || s.res.RegistrationSHA256 != core.RegistrationSHA256 || s.res.WitnessSHA256 != core.WitnessSHA256 || s.res.CatalogueSHA256 != core.CatalogueSHA256 || s.res.GraphSHA256 != core.GraphSHA256 {
		return result, fmt.Errorf("CPP4 definition projection requires the same completed 4-tuple guarded result")
	}
	if !classificationLibraryMatches(s) {
		return result, fmt.Errorf("CPP4 reference library differs from guarded compiler output")
	}
	p := classificationCPP4DefinitionProfile()
	if plan.NativeContext != semanticPagedContext || s.reg.Context != semanticPagedContext || s.reg.Native == nil || s.reg.Native.Symbol != p.Symbol || s.reg.Native.InvocationID != "native-"+anchors.Case ||
		s.witness.Schema != queryEligibilityWitnessSchema || s.witness.Policy != "tsgk-predicates/r1" || s.witness.Case != anchors.Case || s.witness.Route != p.Route || s.witness.Role != "owned" || s.witness.Encoding != EncodingUTF8 ||
		s.witness.Platform != s.approval.Candidate.Platform || s.witness.Input.Path != "source.bin" || s.witness.Input.Bytes != 0 || s.witness.Input.SHA256 != anchors.Source.SHA256 || s.witness.Input.Role != "case" ||
		s.witness.Native.RequestID != anchors.Case || s.witness.Native.API || s.witness.Native.ProducerABI != p.ABI || s.witness.Native.RuntimeManifestSHA256 != plan.RuntimeManifestSHA256 || s.witness.Native.ObservationRunID != plan.RunID || len(s.witness.Native.Points) != 0 ||
		plan.SourceRevision != s.approval.Candidate.SourceRevision || plan.RunID != s.approval.Candidate.Run || s.approval.Candidate.Platform != "windows/amd64" || s.approval.Candidate.Attempt != 1 ||
		!reflect.DeepEqual(s.witness.Required, anchors.Required) || !reflect.DeepEqual(s.witness.Edits, []Edit{}) || len(s.witness.Queries) != p.QueryCount || len(s.reg.Native.Grammar) != len(plan.Grammar) {
		return result, fmt.Errorf("CPP4 guarded candidate, paged context, or witness differs from the fixed anchor")
	}
	if err := classificationGrammarContext(s, anchors, plan); err != nil {
		return result, fmt.Errorf("CPP4 guarded logical grammar pin differs: %w", err)
	}
	if len(cache.checked) != 1 || cache.checked[0].Step.Step != 0 || cache.checked[0].Step.SourceBytes != 0 || cache.checked[0].Step.SourceSHA256 != anchors.Source.SHA256 ||
		cache.checked[0].Step.Edit != nil || cache.checked[0].Fresh != nil || CompareTrees(cache.checked[0].Incremental.Nodes, anchors.Nodes) != nil || len(cache.checked[0].Incremental.Queries) != p.QueryCount {
		return result, fmt.Errorf("guarded CPP4 paged-parser single-frame result differs from the fixed definition scope")
	}
	references := make(map[string]*instrumentedReference, len(anchors.Required))
	referenceIDs := make(map[string]string, len(anchors.Required))
	var referenceExecutableSHA string
	for _, tuple := range anchors.Required {
		observation, ok := s.observation(tuple)
		if !ok || observation.Query != tuple.Query || observation.QueryID != tuple.Query || observation.Step != 0 || observation.TreeRole != "incremental" || observation.SourceBytes != 0 || observation.SourceSHA256 != anchors.Source.SHA256 {
			return result, fmt.Errorf("CPP4 guarded reference observation differs: %s", tuple.Query)
		}
		out := s.outputs[observation.InvocationID]
		if out == nil || observation.ResponseOrdinal >= uint64(len(out.responses)) || observation.ResponseOrdinal >= uint64(len(out.rows)) || out.responses[observation.ResponseOrdinal] == nil {
			return result, fmt.Errorf("CPP4 guarded reference response is missing: %s", tuple.Query)
		}
		response, row := out.responses[observation.ResponseOrdinal], out.rows[observation.ResponseOrdinal]
		if response.Schema != instrumentedReferenceSchema || response.Status != StatusCompleted || response.Code != "" || response.Input.Encoding != EncodingUTF8 || response.Input.Bytes != 0 || response.Input.SHA256 != anchors.Source.SHA256 ||
			response.Grammar.Symbol != p.Symbol || CompareTrees(response.Tree.Nodes, anchors.Nodes) != nil || uint64(len(row)) != observation.ByteLength || digestHex(row) != observation.SHA256 {
			return result, fmt.Errorf("CPP4 guarded reference source frame or complete CST differs: %s", tuple.Query)
		}
		invocation := s.invocations[observation.InvocationID]
		if invocation == nil || invocation.Executable == nil {
			return result, fmt.Errorf("CPP4 guarded reference executable is missing: %s", tuple.Query)
		}
		if referenceExecutableSHA == "" {
			referenceExecutableSHA = invocation.Executable.Member.SHA256
		} else if referenceExecutableSHA != invocation.Executable.Member.SHA256 {
			return result, fmt.Errorf("CPP4 reference executable bindings differ")
		}
		if !hex64.MatchString(referenceExecutableSHA) {
			return result, fmt.Errorf("CPP4 reference executable pin is invalid")
		}
		references[tuple.Query], referenceIDs[tuple.Query] = response, observation.InvocationID
	}
	orderedReferences := make([]*instrumentedReference, p.QueryCount)
	for i, query := range anchors.Queries {
		orderedReferences[i] = references[query.ID]
		if orderedReferences[i] == nil {
			return result, fmt.Errorf("CPP4 guarded reference query is missing: %s", query.ID)
		}
	}
	orderedReferenceIDs, err := classificationCPP4OrderedReferenceIDs(anchors, referenceIDs)
	if err != nil {
		return result, err
	}
	definitions, err := classificationCPP4DefinitionRows(anchors, inputs, s.witness.Queries, cache.checked[0].Incremental.Queries, orderedReferences)
	if err != nil {
		return result, err
	}
	nativeInvocation, err := classificationNativeInvocation(s, anchors)
	if err != nil {
		return result, err
	}
	libraryPin, libraryOK := s.owned["current/go.dll"]
	if nativeInvocation == nil || nativeInvocation.ID != "native-"+anchors.Case || nativeInvocation.Executable == nil || nativeInvocation.Executable.Member.SHA256 != s.witness.Native.ExecutableSHA256 || !hex64.MatchString(nativeInvocation.Executable.Member.SHA256) {
		return result, fmt.Errorf("guarded CPP4 native driver executable is missing or mismatched")
	}
	if !libraryOK || libraryPin.NodeID != "go-library-build" || libraryPin.Member.Role != "runtime-library" || !hex64.MatchString(libraryPin.Member.SHA256) || s.reg.Build == nil || s.reg.Native.ReferenceLibrary == nil || !hex64.MatchString(s.reg.Native.ReferenceLibrary.Member.SHA256) {
		return result, fmt.Errorf("guarded CPP4 build or route library binding is missing")
	}
	libraryNode, ok := s.nodes["go-library-build"]
	if !ok || !classificationGoLibraryNodeMatches(libraryNode, plan, anchors.Workload) || libraryNode.Identities["tool"] != plan.CompilerSHA256 {
		return result, fmt.Errorf("guarded CPP4 compiler node differs from preregistration")
	}
	for _, pin := range []string{s.reg.Build.Identity.Member.SHA256, s.reg.Build.SourceSnapshot.Member.SHA256, s.reg.Build.VendorSnapshot.Member.SHA256} {
		if !hex64.MatchString(pin) {
			return result, fmt.Errorf("guarded CPP4 build identity pin is invalid")
		}
	}
	result = classificationGoDefinitionProjectionResult{
		Scope: "conditional historical classification of 4 predicate-free CPP definitions and 412 potential s06-cpp links; one empty-source paged-parser native-query frame is the guard, not per-case or semantic completion",
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
		Inventory: inputs.coordinates, Definitions: definitions,
	}
	return result, nil
}

type classificationCPP4RowsFixture struct {
	anchors           classificationAnchors
	plan              classificationPlan
	inputs            classificationGoDefinitionInputs
	inventoryData     []byte
	qualificationData []byte
	queryData         []byte
	witness           []QueryWitnessQuery
	native            []querywire.Query
	references        []*instrumentedReference
}

func classificationCPP4RowsTestFixture(t *testing.T) classificationCPP4RowsFixture {
	t.Helper()
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", "cpp4-empty")
	anchors, _ := classificationGoAnchors(t)
	plan := classificationPlan{Schema: "tsgk-query-classification-plan/r1", AnchorsSHA256: anchors.sha256, NativeContext: semanticPagedContext,
		SourceRevision: strings.Repeat("a", 40), RunID: "cpp4-rows-test", RuntimeManifestSHA256: strings.Repeat("b", 64), CompilerSHA256: strings.Repeat("c", 64), Grammar: classificationCPP4GrammarPins()}
	preparedRoot := filepath.Join(t.TempDir(), "prepared")
	t.Setenv("TSGK_BENCH_PREPARED", preparedRoot)
	planRoot := filepath.Join(t.TempDir(), "cpp4-plan")
	grammarRoot, output := filepath.Join(preparedRoot, "routes", "cpp"), filepath.Join(planRoot, "library", "cpp.dll")
	args, err := classificationGoCompilerArgs("cpp", grammarRoot, output, plan.Grammar)
	if err != nil {
		t.Fatal(err)
	}
	plan.LibraryRequest = eligibilityProcessRequest{Path: "C:/toolchain/cc.exe", Args: args, Dir: planRoot, Env: []string{}, StdoutBytes: 16 << 20, StderrBytes: 4 << 20, Wall: 120e9, Grace: 2e9}
	plan.LibraryRequest.Memory.Bytes, plan.LibraryRequest.Memory.Hard = 4<<30, true
	inventoryData := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
	qualificationData := actualJSONRead(t, "../contracts/qualification-c1.json")
	queryData := actualJSONRead(t, "../testdata/native/queries/cpp.json")
	inputs, err := classificationCPP4DefinitionLoadInputs(anchors, plan, inventoryData, qualificationData, queryData)
	if err != nil {
		t.Fatal(err)
	}
	f := classificationCPP4RowsFixture{anchors: anchors, plan: plan, inputs: inputs, inventoryData: inventoryData, qualificationData: qualificationData, queryData: queryData,
		witness: make([]QueryWitnessQuery, len(anchors.Queries)), native: make([]querywire.Query, len(anchors.Queries)), references: make([]*instrumentedReference, len(anchors.Queries))}
	for i, q := range anchors.Queries {
		metadata := classificationGoDefinitionCloneMetadata(q.Metadata)
		f.witness[i] = QueryWitnessQuery{ID: q.ID, Source: q.Source, Metadata: &metadata}
		f.native[i] = querywire.Query{ID: q.ID, Status: StatusCompleted, Patterns: q.Metadata.Patterns, CaptureNames: slices.Clone(q.Metadata.CaptureNames), Predicates: slices.Clone(q.Metadata.Predicates), Captures: [][]int64{}}
		reference := &instrumentedReference{Status: StatusCompleted}
		reference.Query.ID, reference.Query.SHA256 = q.ID, q.SHA256
		reference.Query.Patterns, reference.Query.CaptureNames, reference.Query.Predicates = q.Metadata.Patterns, slices.Clone(q.Metadata.CaptureNames), slices.Clone(q.Metadata.Predicates)
		f.references[i] = reference
	}
	return f
}

func classificationCPP4CloneRowsFixture(f classificationCPP4RowsFixture) classificationCPP4RowsFixture {
	clone := f
	clone.anchors.Queries = slices.Clone(f.anchors.Queries)
	for i := range clone.anchors.Queries {
		clone.anchors.Queries[i].Metadata = classificationGoDefinitionCloneMetadata(f.anchors.Queries[i].Metadata)
	}
	clone.inputs.slotGroup.Queries = slices.Clone(f.inputs.slotGroup.Queries)
	clone.inputs.queryFile.Queries = slices.Clone(f.inputs.queryFile.Queries)
	clone.witness = slices.Clone(f.witness)
	for i := range clone.witness {
		if f.witness[i].Metadata != nil {
			metadata := classificationGoDefinitionCloneMetadata(*f.witness[i].Metadata)
			clone.witness[i].Metadata = &metadata
		}
	}
	clone.native = slices.Clone(f.native)
	for i := range clone.native {
		clone.native[i].CaptureNames = slices.Clone(f.native[i].CaptureNames)
		clone.native[i].Predicates = slices.Clone(f.native[i].Predicates)
		clone.native[i].Captures = slices.Clone(f.native[i].Captures)
	}
	clone.references = make([]*instrumentedReference, len(f.references))
	for i, reference := range f.references {
		value := *reference
		value.Query.CaptureNames = slices.Clone(reference.Query.CaptureNames)
		value.Query.Predicates = slices.Clone(reference.Query.Predicates)
		clone.references[i] = &value
	}
	return clone
}

func TestQueryClassificationCPP4DefinitionBindings(t *testing.T) {
	f := classificationCPP4RowsTestFixture(t)
	p := classificationCPP4DefinitionProfile()
	if f.inputs.coordinates.Cases != 77 || f.inputs.coordinates.Edits != 13 || f.inputs.coordinates.CaseSteps != 90 || f.inputs.coordinates.TreeRoles != 103 ||
		f.inputs.coordinates.Definitions != 4 || f.inputs.coordinates.TreeRolesPerDefinition != 103 || f.inputs.coordinates.PotentialQueryTreeRoles != 412 ||
		f.inputs.coordinates.QuerySourceFileSHA256 != p.QuerySHA || p.QuerySHA != "fc90268c86c9133e2801b2fb7a20bda479137aacc0445911fa5046f11fd65766" {
		t.Fatalf("CPP4 ordinary denominator changed: %#v", f.inputs.coordinates)
	}
	rows, err := classificationCPP4DefinitionRows(f.anchors, f.inputs, f.witness, f.native, f.references)
	if err != nil || len(rows) != 4 {
		t.Fatalf("CPP4 rows: %d %v", len(rows), err)
	}
	for i, row := range rows {
		if row.Query != f.anchors.Queries[i].ID || row.SourceFile != p.QueryFile || row.Class != "STRUCTURAL_ONLY" || row.InventoryClass != "STRUCTURAL_CANDIDATE" ||
			row.CompiledMetadata != "UNVERIFIED" || row.Assessment != "NOT_ASSESSED" || row.SemanticRequired != nil || row.SemanticEvaluated || row.PotentialTreeRoles != 103 {
			t.Fatalf("CPP4 row %d differs: %#v", i, row)
		}
	}
	for _, mutate := range []struct {
		name string
		edit func(*classificationCPP4RowsFixture)
	}{
		{"source-alias", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.slotGroup.Queries[0].SourceFile = "src/testdata/native/queries/c.json"
		}},
		{"witness-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.witness[0].Metadata.Predicates = nil }},
		{"native-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.native[0].Predicates = nil }},
		{"reference-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.references[0].Query.Predicates = nil }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			changed := classificationCPP4CloneRowsFixture(f)
			mutate.edit(&changed)
			if _, err := classificationCPP4DefinitionRows(changed.anchors, changed.inputs, changed.witness, changed.native, changed.references); err == nil {
				t.Fatal("CPP4 definition row mutation accepted")
			}
		})
	}
}

func TestQueryClassificationCPP4ProfileAndPagedContext(t *testing.T) {
	f := classificationCPP4RowsTestFixture(t)
	a, plan := f.anchors, f.plan
	p := classificationCPP4DefinitionProfile()
	if err := classificationCPP4AnchorsValid(a); err != nil || a.nativeContext() != semanticPagedContext || a.nativeSymbol() != p.Symbol || a.libraryName() != "cpp.dll" || classificationNativePrefix(a) != "oracle/" {
		t.Fatalf("CPP4 closed route binding differs: %v", err)
	}
	if !classificationPlanContextMatches(a, plan) {
		t.Fatal("closed CPP4 paged context rejected")
	}
	uniqueReferenceIDs := map[string]string{
		a.Queries[0].ID: "opaque-reference-a", a.Queries[1].ID: "opaque-reference-b",
		a.Queries[2].ID: "opaque-reference-c", a.Queries[3].ID: "opaque-reference-d",
	}
	orderedReferenceIDs, err := classificationCPP4OrderedReferenceIDs(a, uniqueReferenceIDs)
	if err != nil || !reflect.DeepEqual(orderedReferenceIDs, []string{"opaque-reference-a", "opaque-reference-b", "opaque-reference-c", "opaque-reference-d"}) {
		t.Fatalf("four distinct opaque reference invocations rejected: %v %v", orderedReferenceIDs, err)
	}
	for name, repeated := range map[string]map[string]string{
		"one-batch":    {a.Queries[0].ID: "opaque-batch", a.Queries[1].ID: "opaque-batch", a.Queries[2].ID: "opaque-batch", a.Queries[3].ID: "opaque-batch"},
		"two-repeated": {a.Queries[0].ID: "opaque-a", a.Queries[1].ID: "opaque-b", a.Queries[2].ID: "opaque-a", a.Queries[3].ID: "opaque-b"},
	} {
		if _, err := classificationCPP4OrderedReferenceIDs(a, repeated); err == nil {
			t.Fatalf("CPP4 %s reference invocation reuse accepted", name)
		}
	}
	badContext := plan
	badContext.NativeContext = "full-input-utf8/r2"
	badAnchor := a
	badAnchor.sha256 = strings.Repeat("0", 64)
	if classificationPlanContextMatches(a, badContext) || classificationPlanContextMatches(badAnchor, plan) {
		t.Fatal("CPP4 context accepted a cross-context or changed anchor")
	}
	if _, ok := classificationCJRProfileForRoute("cpp"); ok {
		t.Fatal("CPP entered the closed C/Java/Rust route profile")
	}
	if _, err := classificationCJRProfileForName("cpp4-empty"); err == nil {
		t.Fatal("CPP4 entered the closed C/Java/Rust named profile")
	}
	if _, err := classificationCJRDefinitionLoadInputs(a, plan, f.inventoryData, f.qualificationData, f.queryData); err == nil {
		t.Fatal("CPP4 entered the closed C/Java/Rust definition loader")
	}
	badGrammar := plan
	badGrammar.Grammar = slices.Clone(plan.Grammar)
	badGrammar.Grammar[0].Bytes--
	if classificationCPP4DefinitionPlanValid(a, badGrammar) == nil {
		t.Fatal("changed paged logical parser pin accepted")
	}
	badGrammar.Grammar = classificationCPP4GrammarPins()
	badGrammar.Grammar[1].Path = "src/other-scanner.c"
	if classificationCPP4DefinitionPlanValid(a, badGrammar) == nil {
		t.Fatal("changed CPP4 scanner closure accepted")
	}
	if classificationCPP4DefinitionPlanValid(a, badContext) == nil {
		t.Fatal("full-input context accepted for CPP4 paged profile")
	}
	grammarRoot, output := filepath.Join("prepared", "routes", "cpp"), filepath.Join("library", "cpp.dll")
	args, err := classificationGoCompilerArgs("cpp", grammarRoot, output, classificationCPP4GrammarPins())
	if err != nil || !slices.Contains(args, filepath.Join(grammarRoot, "src", "scanner.c")) || args[len(args)-2] != "-o" || args[len(args)-1] != output {
		t.Fatalf("CPP4 scanner or compiler output closure differs: %v %v", args, err)
	}
	for name, mutate := range map[string]func(*eligibilityProcessRequest){
		"compiler-path": func(request *eligibilityProcessRequest) { request.Path = "cc.exe" },
		"output":        func(request *eligibilityProcessRequest) { request.Args[len(request.Args)-1] += ".other" },
		"scanner-argv": func(request *eligibilityProcessRequest) {
			request.Args[6] = filepath.Join(grammarRoot, "src", "other.c")
		},
		"wall":   func(request *eligibilityProcessRequest) { request.Wall++ },
		"memory": func(request *eligibilityProcessRequest) { request.Memory.Hard = false },
		"stdout": func(request *eligibilityProcessRequest) { request.StdoutBytes-- },
		"stderr": func(request *eligibilityProcessRequest) { request.StderrBytes-- },
	} {
		badRequest := plan
		badRequest.LibraryRequest.Args = slices.Clone(plan.LibraryRequest.Args)
		badRequest.LibraryRequest.Env = slices.Clone(plan.LibraryRequest.Env)
		mutate(&badRequest.LibraryRequest)
		if classificationCPP4DefinitionPlanValid(a, badRequest) == nil {
			t.Fatalf("CPP4 noncanonical compiler request %s accepted", name)
		}
	}
	badConsumerPlan := plan
	badConsumerPlan.LibraryRequest.Args = slices.Clone(plan.LibraryRequest.Args)
	badConsumerPlan.LibraryRequest.Args[len(badConsumerPlan.LibraryRequest.Args)-1] += ".preapproved-alias"
	if _, err := classificationCPP4DefinitionLoadInputs(a, badConsumerPlan, f.inventoryData, f.qualificationData, f.queryData); err == nil {
		t.Fatal("CPP4 consumer accepted a noncanonical pre-fixed compiler output plan")
	}
	badABI := a
	badABI.ABI.FieldCount++
	badRoot := a
	badRoot.Nodes = slices.Clone(a.Nodes)
	badRoot.Nodes[0].Type = "program"
	badOrder := a
	badOrder.Queries = slices.Clone(a.Queries)
	badOrder.Queries[0], badOrder.Queries[1] = badOrder.Queries[1], badOrder.Queries[0]
	badRequired := a
	badRequired.Required = slices.Clone(a.Required)
	badRequired.Required[0].Structural = nil
	for name, changed := range map[string]classificationAnchors{"abi": badABI, "root": badRoot, "query-order": badOrder, "required-null": badRequired} {
		if classificationCPP4AnchorsValid(changed) == nil {
			t.Fatalf("changed CPP4 %s anchor accepted", name)
		}
	}
	for _, test := range []struct {
		name                            string
		query, inventory, qualification []byte
	}{
		{"query-bytes", append(append([]byte(nil), f.queryData...), ' '), f.inventoryData, f.qualificationData},
		{"inventory-bytes", f.queryData, append(append([]byte(nil), f.inventoryData...), ' '), f.qualificationData},
		{"qualification-bytes", f.queryData, f.inventoryData, append(append([]byte(nil), f.qualificationData...), ' ')},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := classificationCPP4DefinitionLoadInputs(a, plan, test.inventory, test.qualification, test.query); err == nil {
				t.Fatal("changed CPP4 source inventory input accepted")
			}
		})
	}
	if _, err := classificationCPP4DefinitionProjection(a, plan, nil, nil, QueryReferenceEligibilityResult{}, f.inventoryData, f.qualificationData, f.queryData, strings.Repeat("d", 40)); err == nil || !strings.Contains(err.Error(), "live guarded eligibility session") {
		t.Fatalf("ungrounded CPP4 projection accepted: %v", err)
	}
}

func TestQueryClassificationCPP4StderrBinding(t *testing.T) {
	a := classificationAnchors{Route: "cpp"}
	if err := classificationCaptureStderr(a, semanticProcessCapture{}, eligibilityExecution{}, nil); err == nil {
		t.Fatal("CPP4 missing zero-byte stderr member accepted")
	}
	empty := ReplayMember{Role: "stderr", Bytes: 0, SHA256: digestHex(nil)}
	read := func(pin *ReplayMember) ([]byte, error) {
		if pin.Role != empty.Role || pin.Bytes != empty.Bytes || pin.SHA256 != empty.SHA256 {
			return nil, fmt.Errorf("stderr pin changed")
		}
		return []byte{}, nil
	}
	if err := classificationCaptureStderr(a, semanticProcessCapture{Stderr: &empty}, eligibilityExecution{StderrBytes: 0}, read); err != nil {
		t.Fatalf("CPP4 explicit empty stderr rejected: %v", err)
	}
	nonempty := ReplayMember{Role: "stderr", Bytes: 1, SHA256: digestHex([]byte("x"))}
	if err := classificationCaptureStderr(a, semanticProcessCapture{Stderr: &nonempty}, eligibilityExecution{StderrBytes: 1}, func(*ReplayMember) ([]byte, error) { return []byte("x"), nil }); err != nil {
		t.Fatalf("CPP4 complete nonempty stderr rejected: %v", err)
	}
}
