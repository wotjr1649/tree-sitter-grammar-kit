package kit

import (
	"bytes"
	"context"
	"encoding/base64"
	jsonv2 "encoding/json/v2"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

func TestOrdinaryReferenceSelectionStrictRows(t *testing.T) {
	selection := ordinaryReferenceSelection{Schema: ordinaryReferenceSelectionSchema, Rows: []ordinaryReferenceSelectionRow{
		{Case: "case-a", Step: 0, Query: "q-a", Source: OrdinaryPin{Path: "source-witness/a.utf8", Bytes: 1, SHA256: strings.Repeat("a", 64)}, Invocation: "ref-a", ResponseOrdinal: 0, ByteOffset: 0, ByteLength: 2, SHA256: strings.Repeat("b", 64)},
		{Case: "case-a", Step: 0, Query: "q-b", Source: OrdinaryPin{Path: "source-witness/a.utf8", Bytes: 1, SHA256: strings.Repeat("a", 64)}, Invocation: "ref-a", ResponseOrdinal: 1, ByteOffset: 3, ByteLength: 2, SHA256: strings.Repeat("c", 64)},
	}}
	data := ordinaryJSON(t, selection)
	if _, err := parseOrdinaryReferenceSelection(data); err != nil {
		t.Fatal(err)
	}
	if !ordinaryReferenceSelectionOrdered(selection, []string{"case-a/0/q-a", "case-a/0/q-b"}) {
		t.Fatal("valid pinned row order rejected")
	}
	selection.Rows[0], selection.Rows[1] = selection.Rows[1], selection.Rows[0]
	if ordinaryReferenceSelectionOrdered(selection, []string{"case-a/0/q-a", "case-a/0/q-b"}) {
		t.Fatal("capture row order mutation accepted")
	}
	selection.Rows[1] = selection.Rows[0]
	if _, err := parseOrdinaryReferenceSelection(ordinaryJSON(t, selection)); err == nil {
		t.Fatal("duplicate tuple accepted")
	}
}

func TestOrdinaryReferenceSelectionUsesSelectedCasesAndP0Order(t *testing.T) {
	reg := OrdinaryRegistration{Cases: []OrdinaryCase{{ID: "case-a"}, {ID: "case-b"}}, Required: []OrdinaryRequired{
		{Coordinate: OrdinaryCoordinate{Case: "case-a", Step: 0, Query: "q-a"}},
		{Coordinate: OrdinaryCoordinate{Case: "case-a", Step: 0, Query: "q-b"}},
		{Coordinate: OrdinaryCoordinate{Case: "case-a", Step: 1, Query: "q-a"}},
		{Coordinate: OrdinaryCoordinate{Case: "case-b", Step: 0, Query: "q-a"}},
	}}
	keys := ordinaryReferenceKeys(reg, map[string][][]byte{"case-a": {[]byte("a"), []byte("b")}})
	want := []string{"case-a/0/q-a", "case-a/0/q-b", "case-a/1/q-a"}
	if len(keys) != len(want) {
		t.Fatalf("selected denominator has %d keys, want %d: %v", len(keys), len(want), keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("key %d = %q, want %q", i, keys[i], want[i])
		}
	}
	if ordinaryReferenceKeys(reg, map[string][][]byte{}) != nil {
		t.Fatal("empty selected case set created an obligation")
	}
}

func TestOrdinaryReferenceSelectionRejectsNullableOrTruncatedRows(t *testing.T) {
	for _, raw := range []string{
		`{"schema":"tsgk-query-ordinary-reference-selection/r1","rows":null}`,
		`{"schema":"tsgk-query-ordinary-reference-selection/r1","rows":[{"case_id":"case-a"}]}`,
		`{"schema":"tsgk-query-ordinary-reference-selection/r1","rows":[]}` + ` {}`,
	} {
		if _, err := parseOrdinaryReferenceSelection([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid selection %s", raw)
		}
	}
}

func TestOrdinaryReferencesChecksPinnedStoredRows(t *testing.T) {
	f := newEligibilityFixture(t)
	f.reg.Context, f.reg.Native, f.reg.Observations = OrdinaryContext, nil, []eligibilityObservation{}
	// Use the producer's checked source/query members with two source versions and two separately owned invocations.
	byID := map[string]*eligibilityInvocation{}
	for i := range f.reg.Invocations {
		inv := &f.reg.Invocations[i]
		if inv.ID == "current-ref-0" || inv.ID == "current-ref-1" {
			byID[inv.ID] = inv
		}
	}
	if len(byID) != 2 {
		t.Fatal("ordinary reference fixture rows missing")
	}
	descriptor, pageBytes, _ := semanticBinaryTestDescriptor("library", MaxDocumentBytes+1, bytes.Repeat([]byte("l"), MaxDocumentBytes))
	descriptor.Owner = "D_build"
	libraryParts := make([]OrdinaryMember, 0, len(descriptor.Parts)+1)
	for _, part := range descriptor.Parts {
		path := "oracle/" + part.Path
		f.member(descriptor.Owner, path, "library-paged-part", pageBytes[part.Path])
		libraryParts = append(libraryParts, OrdinaryMember{Owner: descriptor.Owner, Path: part.Path, Role: "library-paged-part", CaseOrBatch: "build", Pin: OrdinaryPin{Path: part.Path, Bytes: part.Bytes, SHA256: part.SHA256}})
	}
	descriptorPath := "oracle/" + descriptor.Logical.Path + ".pages.json"
	libraryData := eligibilityJSON(t, descriptor)
	library := f.member(descriptor.Owner, descriptorPath, "library-paged-descriptor", libraryData)
	libraryProfilePath := strings.TrimPrefix(descriptorPath, "oracle/")
	libraryParts = append(libraryParts, OrdinaryMember{Owner: descriptor.Owner, Path: libraryProfilePath, Role: "library-paged-descriptor", CaseOrBatch: "build", Pin: OrdinaryPin{Path: libraryProfilePath, Bytes: library.Member.Bytes, SHA256: library.Member.SHA256}})
	librarySHA := descriptor.Logical.SHA256
	for _, inv := range byID {
		reqBytes, ok := f.files[inv.Request.Member.Path]
		if !ok {
			t.Fatalf("missing request for %s", inv.ID)
		}
		var processRequest eligibilityProcessRequest
		if err := jsonv2.Unmarshal(reqBytes, &processRequest); err != nil {
			t.Fatal(err)
		}
		processRequest.Args[5] = librarySHA
		f.replace(inv.Request.Member.Path, eligibilityJSON(t, processRequest))
		inv.ApprovedRequest = eligibilityJSON(t, processRequest)
		libraryInput := inv.Inputs["library"]
		libraryInput.MemberPin = &library
		libraryInput.ContentBasis = "PRESERVED_ORIGINAL"
		inv.Inputs["library"] = libraryInput
		var receipt eligibilityReceipt
		if err := jsonv2.Unmarshal(f.files[inv.Receipt.Member.Path], &receipt); err != nil {
			t.Fatal(err)
		}
		stdout, err := base64.StdEncoding.Strict().DecodeString(receipt.StdoutBase64)
		if err != nil || len(stdout) == 0 || stdout[len(stdout)-1] != '\n' {
			t.Fatalf("bad original response bytes for %s", inv.ID)
		}
		responseID := "response-2"
		if inv.ID == "current-ref-1" {
			responseID = "response-3"
		}
		stdout = f.response(responseID, librarySHA)
		receipt.Execution.StdoutBytes = uint64(len(stdout))
		receipt.StdoutBase64 = base64.StdEncoding.EncodeToString(stdout)
		f.replace(inv.Receipt.Member.Path, eligibilityJSON(t, receipt))
	}
	req := f.request()
	req.WitnessRegistration = nil
	queryPin := byID["current-ref-0"].Inputs["query-0"].MemberPin.Member
	sourcePins := [2]OrdinaryPin{}
	sources := map[string][][]byte{"case-a": make([][]byte, 2)}
	for i, id := range []string{"current-ref-0", "current-ref-1"} {
		inv := byID[id]
		sourceData, ok := f.files[inv.Inputs["source"].MemberPin.Member.Path]
		if !ok {
			t.Fatalf("missing source for %s", id)
		}
		sources["case-a"][i] = sourceData
		hash := digestHex(sourceData)
		sourcePins[i] = OrdinaryPin{Path: "source-witness/" + hash + ".utf8", Bytes: uint64(len(sourceData)), SHA256: hash}
	}
	reg := OrdinaryRegistration{
		Cases:          []OrdinaryCase{{ID: "case-a", VersionPins: sourcePins[:]}},
		Queries:        []OrdinaryQuery{{ID: "q0", Source: OrdinaryPin{Path: "queries/q0.scm", Bytes: queryPin.Bytes, SHA256: queryPin.SHA256}}},
		PlannedMembers: libraryParts,
	}
	for step := uint32(0); step < 2; step++ {
		for _, role := range []string{"incremental", "fresh"} {
			if step == 0 && role == "fresh" {
				continue
			}
			reg.Required = append(reg.Required, OrdinaryRequired{Coordinate: OrdinaryCoordinate{Case: "case-a", Query: "q0", Step: step, TreeRole: role}})
		}
	}
	selection := ordinaryReferenceSelection{Schema: ordinaryReferenceSelectionSchema, Rows: []ordinaryReferenceSelectionRow{}}
	for step, id := range []string{"current-ref-0", "current-ref-1"} {
		inv := byID[id]
		var receipt eligibilityReceipt
		if err := jsonv2.Unmarshal(f.files[inv.Receipt.Member.Path], &receipt); err != nil {
			t.Fatal(err)
		}
		stdout, _ := base64.StdEncoding.Strict().DecodeString(receipt.StdoutBase64)
		row := bytesTrimOneLF(stdout)
		selection.Rows = append(selection.Rows, ordinaryReferenceSelectionRow{Case: "case-a", Step: uint32(step), Query: "q0", Source: sourcePins[step], Invocation: id, ResponseOrdinal: 0, ByteOffset: 0, ByteLength: uint64(len(row)), SHA256: digestHex(row)})
	}
	selectionBytes := ordinaryJSON(t, selection)
	pin := func(path string, data []byte) OrdinaryPin { return queryregistrycore.FilePin(path, data) }
	binding := ordinaryEligibilityBinding{Schema: ordinaryEligibilityBindingSchema, Context: OrdinaryContext, ClosureEncoding: "binary-paged",
		Profile: pin("caller/profile.json", []byte("profile")), PlanningProfile: pin("caller/planning.json", []byte("planning")),
		Registration: pin("caller/registration.json", []byte("registration")), Assets: pin("caller/assets.json", []byte("assets")), Plan: pin("caller/plan.json", []byte("plan")), Commitment: pin("caller/commitment.json", []byte("commitment")),
		ReferenceSelection: pin("caller/references.json", selectionBytes), RunBinding: pin("caller/run-binding.json", []byte("run-binding")),
		Candidate: "candidate-362", SemanticCohort: "current", RunID: f.approval.Candidate.Run, ShardIDs: []string{"route/windows/case-a"}}
	f.approval.Ordinary = &binding
	f.approval.WitnessSHA256 = digestHex(nil)
	req.Approval = eligibilityJSON(t, f.approval)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run, e := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		t.Fatal(e)
	}
	defer run.cancel()
	run.eligibility = &eligibilityBudget{r: run}
	root, _, rootErr := openRoot(req.Evidence.Root)
	if rootErr != nil {
		t.Fatal(rootErr)
	}
	defer root.Close()
	ordinaryScope := &ordinaryEligibilityScope{Binding: binding, ClosureEncoding: "binary-paged", TupleRequired: uint64(len(reg.Required))}
	eligibility := &eligibilityScope{r: run, root: root, ordinary: ordinaryScope}
	eligibilityResult, verifyErr := verifyQueryReferenceEligibilityInRunWithOptions(ctx, req, eligibility, nil)
	if verifyErr != nil || eligibilityResult.Build.Verified != 9 || eligibilityResult.Control.Verified != 53 || eligibility.session == nil || !eligibility.session.snapshot.graphBound {
		code := ""
		if len(eligibilityResult.Findings) != 0 {
			code = eligibilityResult.Findings[0].Code
		}
		t.Fatalf("ordinary shared session err=%v result=%s/%s build=%d control=%d", verifyErr, eligibilityResult.Assessment, code, eligibilityResult.Build.Verified, eligibilityResult.Control.Verified)
	}
	streamed := eligibility.session.snapshot.parts["oracle/"+descriptor.Parts[0].Path]
	if streamed == nil || streamed.chain == nil || streamed.chain.next != len(streamed.chain.pins) {
		t.Fatal("ordinary reference library parts did not use the verified streamed chain")
	}
	rows, e := eligibility.session.ordinaryReferences(reg, selectionBytes, sources)
	if e != nil || len(rows) != 2 {
		t.Fatalf("reference rows err=%v got=%d findings=%+v", e, len(rows), eligibility.session.res.Findings)
	}
	for _, key := range []string{"case-a/0/q0", "case-a/1/q0"} {
		if !rows[key].Ready || rows[key].Response == nil || rows[key].Response.Query.ID != "q0" {
			t.Fatalf("reference row %s not ready: %+v", key, rows[key])
		}
	}
	if rows["case-a/1/q0"].Response.Input.SHA256 != sourcePins[1].SHA256 {
		t.Fatal("step-one reference row was not bound to the edited source")
	}
	mutatedSelection := selection
	mutatedSelection.Rows = append([]ordinaryReferenceSelectionRow(nil), selection.Rows...)
	mutatedSelection.Rows[0].Source.SHA256 = strings.Repeat("0", 64)
	if rows, e := eligibility.session.ordinaryReferences(reg, ordinaryJSON(t, mutatedSelection), sources); e != nil || len(rows) != 0 || eligibility.session.firstFailure != "ORDINARY_REFERENCE_SELECTION_PIN" {
		t.Fatalf("unapproved selection mutation err=%v rows=%d first=%s", e, len(rows), eligibility.session.firstFailure)
	}
}

func bytesTrimOneLF(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\n' {
		return data[:len(data)-1]
	}
	return data
}
