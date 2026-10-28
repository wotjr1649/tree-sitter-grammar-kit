package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSemanticPagedParserValidation(t *testing.T) {
	data := bytes.Repeat([]byte("p"), MaxDocumentBytes+1)
	input := semanticBuildInput{Role: "grammar-parser", Path: "parser.c", Bytes: uint64(len(data)), SHA256: digestHex(data)}
	d := semanticParserDescriptor{Schema: "tsgk-query-parser-pages/r1", Owner: "D"}
	d.Logical.ClosureIndex = 7
	d.Logical.Role = input.Role
	d.Logical.Path = input.Path
	d.Logical.Bytes = input.Bytes
	d.Logical.SHA256 = input.SHA256
	parts := map[string][]byte{}
	for i, offset := 0, 0; offset < len(data); i++ {
		size := min(MaxDocumentBytes, len(data)-offset)
		name := fmt.Sprintf("capture/build-input-00007.part-%05d.bin", i)
		part := data[offset : offset+size]
		parts[name] = part
		d.Parts = append(d.Parts, struct {
			Index  uint64 `json:"index"`
			Offset uint64 `json:"offset"`
			Path   string `json:"path"`
			Bytes  uint64 `json:"bytes"`
			SHA256 string `json:"sha256"`
		}{uint64(i), uint64(offset), name, uint64(size), digestHex(part)})
		offset += size
	}
	for _, mutation := range []string{"normal", "offset", "order", "drop", "duplicate", "whole-hash", "part-rehashed", "missing", "index", "owner", "truncated"} {
		t.Run(mutation, func(t *testing.T) {
			copy := d
			copy.Parts = append(copy.Parts[:0:0], d.Parts...)
			supplied := map[string][]byte{}
			for p, b := range parts {
				supplied[p] = b
			}
			switch mutation {
			case "offset":
				copy.Parts[1].Offset--
			case "order":
				copy.Parts[0], copy.Parts[1] = copy.Parts[1], copy.Parts[0]
			case "drop":
				copy.Parts = copy.Parts[:1]
			case "duplicate":
				copy.Parts[1] = copy.Parts[0]
			case "whole-hash":
				copy.Logical.SHA256 = digestHex([]byte("other"))
			case "part-rehashed":
				supplied[copy.Parts[1].Path] = []byte("q")
				copy.Parts[1].SHA256 = digestHex([]byte("q"))
			case "missing":
				delete(supplied, copy.Parts[1].Path)
			case "index":
				copy.Logical.ClosureIndex++
			case "owner":
				copy.Owner = "O"
			case "truncated":
				supplied[copy.Parts[0].Path] = supplied[copy.Parts[0].Path][:MaxDocumentBytes-1]
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			r, e := startRun(ctx, time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			b := &eligibilityBudget{r: r}
			good, e := b.semanticPagedParser(eligibilityJSON(t, copy), input, 7, func(pin ReplayMember) ([]byte, bool, *Error) { data, ok := supplied[pin.Path]; return data, ok, nil })
			if e != nil || good != (mutation == "normal") {
				t.Fatalf("mutation=%s good=%v err=%v", mutation, good, e)
			}
		})
	}
}

func TestSemanticPagedPublicAPIAndReplay(t *testing.T) {
	req, f, binding := semanticBoundFixtureMode(t, 0, true, true)
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	set := VerifyOracleSet(os.DirFS(filepath.Join(f.root, "oracle")))
	if !set.Valid {
		t.Fatalf("paged set invalid: %+v", set.Findings)
	}
	var old OracleManifest
	if e := decodeOracleManifestVersion(f.files["oracle/manifest.json"], &old, OracleSemanticsManifestSchema); e == nil || e.Code != "SCHEMA_UNSUPPORTED" {
		t.Fatalf("r5 reader accepted r6: %v", e)
	}
	passed, err := VerifyQuerySemantics(ctx, req)
	if err != nil || !passed.Complete || !passed.BindingValid || passed.Assessment != AssessPass || passed.BaseVerified.Assessment != AssessPass || passed.Tuple.Verified != 3 {
		t.Fatalf("BOUND paging: %v %+v eligibility=%+v", err, passed, passed.Eligibility)
	}
	standalone, err := VerifyQueryReferenceEligibility(ctx, QueryReferenceEligibilityRequest{Evidence: req.Evidence, Approval: req.EligibilityApproval, Registration: req.EligibilityRegistration, WitnessRegistration: req.WitnessRegistration})
	if err != nil || standalone.Assessment != AssessPass || standalone.Tuple.Verified != 3 {
		t.Fatalf("standalone paging: %v %+v", err, standalone)
	}
	absent := req
	absent.EligibilityApproval = nil
	absent.EligibilityRegistration = nil
	absent.WitnessRegistration = nil
	absentBinding := binding
	absentBinding.EligibilityApprovalSHA256 = digestHex(nil)
	absentBinding.EligibilityRegistrationSHA256 = digestHex(nil)
	absentBinding.WitnessRegistrationSHA256 = digestHex(nil)
	absentBinding.CohortID = ""
	absentBinding.InvocationID = ""
	absentBinding.Process = eligibilityMember{}
	absentBinding.References = []semanticReferenceBinding{}
	absent.RunBinding = eligibilityJSON(t, absentBinding)
	blocked, err := VerifyQuerySemantics(ctx, absent)
	if err != nil || !blocked.Complete || blocked.Assessment != AssessBlocked || blocked.BaseVerified.Assessment != AssessPass || blocked.Tuple.Required != 3 || blocked.Tuple.Blocked != 3 {
		t.Fatalf("ABSENT paging: %v %+v", err, blocked)
	}
	root, profile := semanticBoundReplay(t, req, f, binding)
	replayed, err := Replay(ctx, ReplayRequest{Root: root, Profile: profile})
	if err != nil || replayed.Assessment != AssessPass || replayed.QuerySemantics == nil || replayed.QuerySemantics.Tuple != passed.Tuple || replayed.BytesRead != passed.BytesRead || !validSemanticReplayResult(replayed) {
		t.Fatalf("paged replay: %v %+v", err, replayed)
	}
	var wrong map[string]any
	if err := json.Unmarshal(profile, &wrong); err != nil {
		t.Fatal(err)
	}
	wrong["reducer"] = "oracle-set-r4"
	_, err = Replay(ctx, ReplayRequest{Root: root, Profile: eligibilityJSON(t, wrong)})
	if err == nil || asError(err).Code != "SEMANTICS_REDUCER_CONTEXT" {
		t.Fatalf("r4 accepted paged registration: %v", err)
	}
	f.member(binding.DriverNode, "native/unreferenced.part.bin", "grammar-paged-part", []byte("unused"))
	orphaned, err := VerifyQueryReferenceEligibility(ctx, f.request())
	if err != nil || orphaned.Assessment != AssessFail || !eligibilityFinding(orphaned, "ELIGIBILITY_NATIVE_PARSER_PAGES") {
		t.Fatalf("unreferenced paged member accepted: %v %+v", err, orphaned)
	}
	// Approve a consistent graph with a part owned by another real node.
	part := ""
	for p := range f.files {
		if strings.Contains(p, ".part-00000.bin") {
			part = p
			break
		}
	}
	if part == "" {
		t.Fatal("missing physical part")
	}
	native := f.nodes[binding.DriverNode]
	other := f.nodes["control-static"]
	for i, p := range native.Files {
		if p.Path == part {
			other.Files = append(other.Files, p)
			native.Files = append(native.Files[:i], native.Files[i+1:]...)
			break
		}
	}
	f.nodes[native.ID] = native
	f.nodes[other.ID] = other
	for i := range f.reg.OwnedMembers {
		if f.reg.OwnedMembers[i].Member.Path == part {
			f.reg.OwnedMembers[i].NodeID = other.ID
		}
	}
	mutated := f.request()
	rejected, err := VerifyQueryReferenceEligibility(ctx, mutated)
	if err != nil || rejected.Assessment != AssessFail || !eligibilityFinding(rejected, "ELIGIBILITY_MEMBER_OWNER") {
		t.Fatalf("foreign native part owner accepted: %v %+v", err, rejected)
	}
}

func TestSemanticPagedContextRequiresLargeParser(t *testing.T) {
	f := newEligibilityFixture(t)
	f.reg.Context = semanticPagedContext
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got, err := VerifyQueryReferenceEligibility(ctx, f.request())
	if err != nil || got.Assessment != AssessFail || !eligibilityFinding(got, "ELIGIBILITY_NATIVE_PARSER_PAGES") {
		t.Fatalf("small normal parser accepted as paged: %v %+v", err, got)
	}
}
