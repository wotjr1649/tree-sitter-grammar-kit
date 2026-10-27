package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
)

// A previous producer supplies names only; the registration precedes this distinct run.
func semanticAPIControl(t *testing.T, request OracleRequest, original map[string]any, anchors []byte, previous kit.OracleManifest, failure bool) {
	t.Helper()
	encode := func(value any) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	if failure {
		var cloned map[string]any
		if err := json.Unmarshal(encode(original), &cloned); err != nil {
			t.Fatal(err)
		}
		original = cloned
		original["cases"].([]any)[0].(map[string]any)["expect"] = []any{map[string]any{"step": 0, "syntax": "ERROR", "contains": []string{}, "declarations": ""}}
	}
	var independent struct {
		Queries  []kit.QueryWitnessQuery `json:"queries"`
		Required []kit.QueryWitnessTuple `json:"required"`
	}
	if err := json.Unmarshal(anchors, &independent); err != nil {
		t.Fatal(err)
	}
	profile := map[string]any{}
	meaning := map[string]any{}
	for key, value := range original {
		profile[key] = value
		if key != "schema" && key != "query_semantics_registration_sha256" {
			meaning[key] = value
		}
	}
	parsed, err := kit.ParseOracleProfile(encode(profile))
	if err != nil {
		t.Fatal(err)
	}
	required := uint64(len(independent.Required))
	if required != uint64(len(parsed.Queries)*(1+2*len(parsed.Native.Cases[0].Edits))) {
		t.Fatal("independent tuple coverage changed")
	}
	witness := kit.QueryWitnessRegistration{Schema: "tsgk-query-semantics-registration/r1", Policy: querypolicy.Revision, Case: "c1", Route: "json", Platform: runtime.GOOS + "/" + runtime.GOARCH, Role: "owned", Encoding: kit.EncodingUTF8, Input: parsed.Native.Cases[0].Input, Edits: parsed.Native.Cases[0].Edits, Queries: independent.Queries, Required: independent.Required}
	var reg map[string]any
	if err := json.Unmarshal(encode(witness), &reg); err != nil {
		t.Fatal(err)
	}
	op := kit.NativeOperations()["native-query"]
	var operation map[string]any
	if err := json.Unmarshal(encode(op), &operation); err != nil {
		t.Fatal(err)
	}
	limits := map[string]any{}
	for _, key := range []string{"input_bytes", "nodes", "full_nodes", "depth", "output_bytes", "parse_ms", "memory_bytes", "errors", "partial_nodes", "matches", "captures", "query_ms"} {
		limits[key] = operation[key]
	}
	classes := make([]string, len(witness.Required))
	for i := range classes {
		classes[i] = "semantic-required"
	}
	plan := []map[string]string{{"owner": "O", "path": "manifest.json", "role": "manifest"}}
	for _, member := range previous.Members {
		owner := "D"
		if member.Role == "record" || member.Role == "projection" || member.Role == "source" {
			owner = "O"
		}
		plan = append(plan, map[string]string{"owner": owner, "path": member.Path, "role": member.Role})
	}
	reg["context"], reg["profile"], reg["native_policy"], reg["limits"], reg["classes"], reg["planned_members"] = "full-input-utf8/r2", meaning, kit.NativePolicy(op, kit.OutputTree, nil), limits, classes, plan
	registration := encode(reg)
	profile["query_semantics_registration_sha256"] = sha(registration)
	profileData := encode(profile)
	replayRoot := t.TempDir()
	root := filepath.Join(replayRoot, "evidence")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	request.Profile, request.QuerySemanticsRegistration, request.Out, request.Work = profileData, registration, filepath.Join(root, "oracle"), t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	producer, err := Oracle(ctx, request)
	if err != nil || producer.Set == nil || !producer.Set.Valid {
		t.Fatalf("registered producer: %v %+v", err, producer.Set)
	}
	manifestData, err := os.ReadFile(filepath.Join(request.Out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest kit.OracleManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	identities := map[string]string{}
	for _, role := range kit.EvidenceRoles {
		identities[role] = "synthetic-absent-" + role
	}
	identities["attempt"], identities["platform"] = "1", runtime.GOOS+"/"+runtime.GOARCH
	d := kit.EvidenceNode{ID: "D", Kind: "run", ExecutionStatus: kit.StatusCompleted, EvidenceMode: kit.ModeNewRun, Assessment: kit.AssessPass, RecordedAssessment: "", Identities: identities, Files: []kit.ReplayMember{}, Refs: []kit.EvidenceRef{}}
	o := d
	o.ID, o.Assessment, o.RecordedAssessment, o.Files = "O", manifest.Assessment, "", []kit.ReplayMember{}
	o.Identities = map[string]string{}
	for role, identity := range identities {
		o.Identities[role] = identity
	}
	o.Identities["run"] = "synthetic-oracle-parent-run"

	boundMembers := []map[string]any{}
	for _, planned := range plan {
		local, role, owner := planned["path"], planned["role"], planned["owner"]
		data, err := os.ReadFile(filepath.Join(request.Out, filepath.FromSlash(local)))
		if err != nil {
			t.Fatal(err)
		}
		pin := kit.ReplayMember{Path: "oracle/" + local, Role: role, Bytes: uint64(len(data)), SHA256: sha(data)}
		if owner == "D" {
			d.Files = append(d.Files, pin)
		} else {
			o.Files = append(o.Files, pin)
		}
		boundMembers = append(boundMembers, map[string]any{"owner": owner, "local": local, "node_id": owner, "member": pin})
	}
	graph := encode(map[string]any{"schema": kit.EvidenceSchema, "nodes": []kit.EvidenceNode{d, o}})
	if err := os.WriteFile(filepath.Join(root, "evidence.json"), graph, 0600); err != nil {
		t.Fatal(err)
	}
	policy := encode(map[string]any{"schema": kit.EvidencePolicySchema, "id": "semantic-absent-control", "evidence_sha256": sha(graph), "required": []any{map[string]any{"node": "D", "kind": "run", "identities": identities}, map[string]any{"node": "O", "kind": "run", "identities": o.Identities}}, "carry_forward": []any{}})
	binding := map[string]any{"schema": "tsgk-query-semantics-run-binding/r1", "profile_sha256": sha(profileData), "manifest_sha256": sha(manifestData), "registration_sha256": sha(registration), "eligibility_approval_sha256": sha(nil), "eligibility_registration_sha256": sha(nil), "witness_registration_sha256": sha(nil), "evidence_policy_sha256": sha(policy), "evidence_sha256": sha(graph), "driver_node": "D", "oracle_node": "O", "cohort_id": "", "invocation_id": "", "process": map[string]any{"node_id": "", "member": kit.ReplayMember{}}, "references": []any{}, "members": boundMembers}
	apiRequest := kit.QuerySemanticsRequest{Evidence: kit.EvidenceRequest{Root: root, Policy: policy}, Profile: profileData, Registration: registration, RunBinding: encode(binding)}

	wanted, base := kit.AssessBlocked, kit.AssessPass
	if failure {
		wanted, base = kit.AssessFail, kit.AssessFail
	}
	result, err := kit.VerifyQuerySemantics(ctx, apiRequest)
	if err != nil || !result.Complete || result.Assessment != wanted || result.Semantic != kit.AssessBlocked || !result.BindingValid || result.BaseVerified.Assessment != base || result.Tuple.Required != required || result.Tuple.Blocked != required || result.Tuple.Verified != 0 || result.Tuple.Unprocessed != 0 || result.Evidence == nil || result.Evidence.Assessment != kit.AssessPass {
		t.Fatalf("ABSENT must preserve base and fixed denominator: %v %+v evidence=%+v", err, result, result.Evidence)
	}
	replayMembers := []kit.ReplayMember{}
	for _, doc := range []struct {
		path, role string
		data       []byte
	}{
		{"oracle-profile.json", "workload-profile", profileData}, {"registration.json", "semantics-registration", registration}, {"run-binding.json", "semantics-run-binding", encode(binding)},
		{"approval.json", "eligibility-approval", []byte{}}, {"eligibility.json", "eligibility-registration", []byte{}}, {"witness.json", "query-registration", []byte{}}, {"policy.json", "evidence-policy", policy},
	} {
		if err := os.WriteFile(filepath.Join(replayRoot, doc.path), doc.data, 0600); err != nil {
			t.Fatal(err)
		}
		replayMembers = append(replayMembers, kit.ReplayMember{Path: doc.path, Role: doc.role, Bytes: uint64(len(doc.data)), SHA256: sha(doc.data)})
	}
	replayMembers = append(replayMembers, kit.ReplayMember{Path: "evidence/evidence.json", Role: "semantics-evidence", Bytes: uint64(len(graph)), SHA256: sha(graph)})
	for _, pin := range append(append([]kit.ReplayMember{}, d.Files...), o.Files...) {
		pin.Path, pin.Role = "evidence/"+pin.Path, "semantics-evidence"
		replayMembers = append(replayMembers, pin)
	}
	replayProfile := encode(map[string]any{"schema": kit.ReplaySchema, "id": "semantic-absent-replay", "reducer": "oracle-set-r4", "operation": "evidence-replay",
		"subject":    kit.ReplaySubject{Run: o.Identities["run"], Attempt: 1, Platform: o.Identities["platform"], EvidenceMode: kit.ModeNewRun, ExecutionStatus: kit.StatusCompleted, Assessment: manifest.Assessment},
		"identities": map[string]string{"workload": sha(profileData), "semantics-registration": sha(registration), "semantics-run-binding": sha(encode(binding)), "evidence-policy": sha(policy)},
		"records":    []string{"c1"}, "members": replayMembers})
	replayed, err := kit.Replay(ctx, kit.ReplayRequest{Root: replayRoot, Profile: replayProfile})
	if err != nil || replayed.ResultSchema != kit.ReplaySemanticsResultSchema || replayed.Assessment != wanted || replayed.EvidenceValid == failure || replayed.QuerySemantics == nil || replayed.QuerySemantics.Tuple.Blocked != required || replayed.QuerySemantics.BaseVerified.Assessment != base {
		t.Fatalf("same-core replay: %v %+v", err, replayed)
	}

	if replayed.BytesRead != result.BytesRead || replayed.QuerySemantics.BytesRead != result.BytesRead {
		t.Fatalf("same input was counted twice: API=%d replay=%d core=%d", result.BytesRead, replayed.BytesRead, replayed.QuerySemantics.BytesRead)
	}
	if replayed.QuerySemantics.Work < result.Work+128*uint64(len(replayMembers)) {
		t.Fatalf("outer walk work vanished: API=%d replay=%d members=%d", result.Work, replayed.QuerySemantics.Work, len(replayMembers))
	}
	if failure {
		if result.Code != "EXPECTATION_FAILED" || replayed.QuerySemantics.Code != "EXPECTATION_FAILED" {
			t.Fatalf("producer first code lost: API=%s replay=%s", result.Code, replayed.QuerySemantics.Code)
		}
		return
	}
	for _, mutation := range []string{"owner", "path", "role", "profile"} {
		t.Run(mutation, func(t *testing.T) {
			var copy map[string]any
			if err := json.Unmarshal(encode(binding), &copy); err != nil {
				t.Fatal(err)
			}
			first := copy["members"].([]any)[0].(map[string]any)
			switch mutation {
			case "owner":
				first["node_id"] = "D"
			case "path":
				first["member"].(map[string]any)["path"] = "oracle/same-hash-other-name.json"
			case "role":
				first["member"].(map[string]any)["role"] = "record"
			case "profile":
				copy["profile_sha256"] = strings.Repeat("0", 64)
			}
			changed := apiRequest
			changed.RunBinding = encode(copy)
			res, err := kit.VerifyQuerySemantics(ctx, changed)
			if err != nil || res.Assessment != kit.AssessFail || res.BindingValid || res.Semantic == kit.AssessPass || res.Tuple.Required != required {
				t.Fatalf("binding mutation %s: %v %+v", mutation, err, res)
			}
		})
	}
}
