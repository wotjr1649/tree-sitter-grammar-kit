package kit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSemanticPagedRehashCannotReplaceProfile(t *testing.T) {
	req, f, binding := semanticBoundFixtureMode(t, 0, true, true)
	buildPath := "oracle/capture/build.json"
	var build semanticBuildDescription
	if err := json.Unmarshal(f.files[buildPath], &build); err != nil {
		t.Fatal(err)
	}
	index := -1
	for i, input := range build.Closure {
		if input.Role == "grammar-parser" {
			index = i
		}
	}
	if index < 0 {
		t.Fatal("no parser")
	}
	descriptorPath := fmt.Sprintf("oracle/capture/build-input-%05d.pages.json", index)
	var descriptor semanticParserDescriptor
	if err := json.Unmarshal(f.files[descriptorPath], &descriptor); err != nil {
		t.Fatal(err)
	}
	last := len(descriptor.Parts) - 1
	if last < 0 || descriptor.Parts[last].Bytes != 1 {
		t.Fatal("independent synthetic boundary changed")
	}
	f.replace("oracle/"+descriptor.Parts[last].Path, []byte("q"))
	descriptor.Parts[last].SHA256 = digestHex([]byte("q"))
	h := sha256.New()
	for _, part := range descriptor.Parts {
		_, _ = h.Write(f.files["oracle/"+part.Path])
	}
	oldLogical, newLogical := descriptor.Logical.SHA256, hex.EncodeToString(h.Sum(nil))
	descriptor.Logical.SHA256 = newLogical
	f.replace(descriptorPath, eligibilityJSON(t, descriptor))
	build.Closure[index].SHA256 = newLogical
	manifestPath := f.reg.Native.BuildManifest.Member.Path
	plain := bytes.ReplaceAll(f.files[manifestPath], []byte(oldLogical), []byte(newLogical))
	oldIdentity, newIdentity := build.Identity, digestHex(plain)
	build.Identity = newIdentity
	f.replace(manifestPath, plain)
	f.replace(buildPath, eligibilityJSON(t, build))
	// Retain a fully coherent counterfeit, including the native logger and approved request.
	for path, data := range f.files {
		if bytes.Contains(data, []byte(oldIdentity)) {
			f.replace(path, bytes.ReplaceAll(data, []byte(oldIdentity), []byte(newIdentity)))
		}
	}
	f.witness.Native.ProducerSHA256 = newIdentity
	for i := range f.reg.Invocations {
		if f.reg.Invocations[i].ID == binding.InvocationID {
			f.reg.Invocations[i].ApprovedRequest = eligibilityJSON(t, f.witness.Native)
		}
	}
	capturePath := "oracle/capture/process-capture.json"
	var capture map[string]json.RawMessage
	if err := json.Unmarshal(f.files[capturePath], &capture); err != nil {
		t.Fatal(err)
	}
	var stdout ReplayMember
	if err := json.Unmarshal(capture["stdout"], &stdout); err != nil {
		t.Fatal(err)
	}
	stdout.Bytes = uint64(len(f.files["oracle/"+stdout.Path]))
	stdout.SHA256 = digestHex(f.files["oracle/"+stdout.Path])
	capture["stdout"] = eligibilityJSON(t, stdout)
	f.replace(capturePath, eligibilityJSON(t, capture))
	recordPath := "oracle/records/00000-" + f.witness.Case + ".json"
	var record map[string]json.RawMessage
	if err := json.Unmarshal(f.files[recordPath], &record); err != nil {
		t.Fatal(err)
	}
	var local OracleQuerySemantics
	if err := json.Unmarshal(record["query_semantics"], &local); err != nil {
		t.Fatal(err)
	}
	local.Capture.Bytes = uint64(len(f.files[capturePath]))
	local.Capture.SHA256 = digestHex(f.files[capturePath])
	projection := f.files["oracle/"+local.Projection.Path]
	local.Projection.Bytes = uint64(len(projection))
	local.Projection.SHA256 = digestHex(projection)
	record["query_semantics"] = eligibilityJSON(t, local)
	delete(record, "complete")
	data := eligibilityJSON(t, record)
	data = append(data[:len(data)-1], []byte(",\"complete\":true}")...)
	f.replace(recordPath, data)
	semanticBoundReapprove(t, &req, f, &binding)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	eligible, err := VerifyQueryReferenceEligibility(ctx, QueryReferenceEligibilityRequest{Evidence: req.Evidence, Approval: req.EligibilityApproval, Registration: req.EligibilityRegistration, WitnessRegistration: req.WitnessRegistration})
	if err != nil || eligible.Assessment != AssessPass {
		t.Fatalf("counterfeit not internally coherent: err=%v findings=%+v", err, eligible.Findings)
	}
	rejected, err := VerifyQuerySemantics(ctx, req)
	found := false
	for _, finding := range rejected.Findings {
		found = found || finding.Code == "SEMANTICS_BUILD_PROFILE"
	}
	if err != nil || rejected.Assessment != AssessFail || rejected.BindingValid || !found {
		t.Fatalf("independent profile pin bypassed: err=%v findings=%+v", err, rejected.Findings)
	}
	root, profile := semanticBoundReplay(t, req, f, binding)
	replayed, err := Replay(ctx, ReplayRequest{Root: root, Profile: profile})
	if err != nil || replayed.Assessment != AssessFail || replayed.QuerySemantics == nil || replayed.QuerySemantics.BindingValid || !strings.Contains(string(eligibilityJSON(t, replayed.QuerySemantics.Findings)), "SEMANTICS_BUILD_PROFILE") {
		t.Fatalf("replay lost independent profile refusal: %v", err)
	}
}
