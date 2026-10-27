package kit

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

type semanticBoundMember struct {
	Owner  string       `json:"owner"`
	Local  string       `json:"local"`
	NodeID string       `json:"node_id"`
	Member ReplayMember `json:"member"`
}

type semanticRunBinding struct {
	Schema                        string                     `json:"schema"`
	ProfileSHA256                 string                     `json:"profile_sha256"`
	ManifestSHA256                string                     `json:"manifest_sha256"`
	RegistrationSHA256            string                     `json:"registration_sha256"`
	EligibilityApprovalSHA256     string                     `json:"eligibility_approval_sha256"`
	EligibilityRegistrationSHA256 string                     `json:"eligibility_registration_sha256"`
	WitnessRegistrationSHA256     string                     `json:"witness_registration_sha256"`
	EvidencePolicySHA256          string                     `json:"evidence_policy_sha256"`
	EvidenceSHA256                string                     `json:"evidence_sha256"`
	DriverNode                    string                     `json:"driver_node"`
	OracleNode                    string                     `json:"oracle_node"`
	CohortID                      string                     `json:"cohort_id"`
	InvocationID                  string                     `json:"invocation_id"`
	Process                       eligibilityMember          `json:"process"`
	References                    []semanticReferenceBinding `json:"references"`
	Members                       []semanticBoundMember      `json:"members"`
}

type semanticReferenceBinding struct {
	InvocationID string            `json:"invocation_id"`
	CohortID     string            `json:"cohort_id"`
	Receipt      eligibilityMember `json:"receipt"`
}

func (b *eligibilityBudget) semanticRunBinding(binding *semanticRunBinding, req QuerySemanticsRequest, profile OracleProfile, reg semanticRegistration, manifest OracleManifest, files map[string][]byte, snapshot *evidenceSnapshot, session *eligibilitySession, state semanticSetState, add func(string, string, string)) (bool, *Error) {
	if e := b.decode("run-binding", req.RunBinding, binding, true); e != nil {
		return false, e
	}
	valid := true
	bad := func(code, at string) {
		valid = false
		add(code, at, "외부 run-binding이 사전 등록 또는 동일 evidence pass의 소유 관계와 다르다")
	}
	if binding.Schema != "tsgk-query-semantics-run-binding/r1" {
		return false, fail(KindUnsupported, "SCHEMA_UNSUPPORTED", "run-binding", nil)
	}
	for _, pin := range []struct {
		want string
		data []byte
	}{
		{binding.ProfileSHA256, req.Profile}, {binding.ManifestSHA256, files["manifest.json"]}, {binding.RegistrationSHA256, req.Registration},
		{binding.EligibilityApprovalSHA256, req.EligibilityApproval}, {binding.EligibilityRegistrationSHA256, req.EligibilityRegistration},
		{binding.WitnessRegistrationSHA256, req.WitnessRegistration}, {binding.EvidencePolicySHA256, req.Evidence.Policy},
	} {
		if e := b.charge(uint64(len(pin.data))+1, 0, "semantic-binding-digest"); e != nil {
			return false, e
		}
		if pin.want != digestHex(pin.data) {
			bad("SEMANTICS_BINDING_DIGEST", "run-binding")
		}
	}
	if binding.EvidenceSHA256 != snapshot.verifiedMembers["evidence.json"].SHA256 || !validID(binding.DriverNode) || !validID(binding.OracleNode) || binding.DriverNode == binding.OracleNode || len(binding.Members) != len(reg.PlannedMembers) {
		bad("SEMANTICS_BINDING_GRAPH", "run-binding")
	}
	owners, nodes := map[string]string{}, map[string]EvidenceNode{}
	for _, node := range snapshot.nodes {
		nodes[node.ID] = node
		for _, member := range node.Files {
			owners[member.Path] = node.ID
		}
	}
	seen := map[string]bool{}
	for i, member := range binding.Members {
		if e := b.charge(20*uint64(len(member.Local))+1, 512+16*uint64(len(member.Local)), "semantic-binding-member"); e != nil {
			return false, e
		}
		if i >= len(reg.PlannedMembers) {
			bad("SEMANTICS_BINDING_MEMBER", member.Local)
			continue
		}
		plan := reg.PlannedMembers[i]
		expectedNode := binding.OracleNode
		if plan.Owner == "D" {
			expectedNode = binding.DriverNode
		}
		path := "oracle/" + plan.Path
		if seen[member.Local] || member.Local != plan.Path || member.Owner != plan.Owner || member.NodeID != expectedNode || owners[path] != expectedNode || member.Member != snapshot.verifiedMembers[path] || member.Member.Path != path || member.Member.Role != plan.Role || snapshot.invalidNodes[expectedNode] {
			bad("SEMANTICS_BINDING_MEMBER", path)
		}
		seen[member.Local] = true
	}
	o, ook := nodes[binding.OracleNode]
	d, dok := nodes[binding.DriverNode]
	if !ook || !dok || o.Kind != "run" || d.Kind != "run" || o.EvidenceMode != ModeNewRun || d.EvidenceMode != ModeNewRun || o.Assessment != manifest.Assessment || o.ExecutionStatus != manifest.ExecutionStatus {
		bad("SEMANTICS_BINDING_OWNER", "run-binding")
	}
	if manifest.Producer["platform"] != reg.Platform || manifest.QuerySemanticsRegistrationSHA256 != profile.QuerySemanticsRegistrationSHA256 || manifest.Workload["profile_sha256"] != digestHex(req.Profile) || manifest.Workload["profile_id"] != profile.Native.ID || manifest.Workload["route"] != reg.Route || manifest.Workload["operation"] != profile.Native.Operation || manifest.Workload["output"] != profile.Native.Output || manifest.Policy != reg.NativePolicy || manifest.Operation == nil || !reflect.DeepEqual(*manifest.Operation, NativeOperations()[profile.Native.Operation]) {
		bad("SEMANTICS_MANIFEST_BINDING", "oracle/manifest.json")
	}
	if len(manifest.Queries) != len(reg.Queries) {
		bad("SEMANTICS_MANIFEST_QUERIES", "oracle/manifest.json")
	} else {
		for i, q := range reg.Queries {
			if manifest.Queries[i] != (IdentityRef{Role: "query:" + q.ID, Schema: "tsgk-query-source/r1", SHA256: digestHex([]byte(q.Source))}) {
				bad("SEMANTICS_MANIFEST_QUERIES", q.ID)
			}
		}
	}
	if state.build.Symbol != profile.Native.Symbol || state.build.Compiler != profile.Native.Compiler {
		bad("SEMANTICS_BUILD_PROFILE", "oracle/capture/build.json")
	}
	closure := map[string]semanticBuildInput{}
	for _, input := range state.build.Closure {
		if e := b.charge(1, 256+uint64(len(input.Path)), "semantic-profile-closure"); e != nil {
			return false, e
		}
		closure[input.Role+" "+input.Path] = input
	}
	for _, expected := range profile.Native.Grammar {
		if e := b.charge(1, 0, "semantic-profile-grammar"); e != nil {
			return false, e
		}
		input, found := closure["grammar-"+expected.Role+" "+expected.Path]
		if !found || input.Bytes != expected.Bytes || input.SHA256 != expected.SHA256 {
			bad("SEMANTICS_BUILD_PROFILE", "oracle/capture/build.json")
		}
		delete(closure, "grammar-"+expected.Role+" "+expected.Path)
	}
	for _, input := range closure {
		if strings.HasPrefix(input.Role, "grammar-") {
			bad("SEMANTICS_BUILD_PROFILE", "oracle/capture/build.json")
		}
	}

	if len(req.EligibilityApproval) == 0 {
		if binding.CohortID != "" || binding.InvocationID != "" || binding.Process != (eligibilityMember{}) || len(binding.References) != 0 {
			bad("SEMANTICS_ABSENT_BINDING", "run-binding")
		}
		return valid, nil
	}
	if session == nil || session.reg.Native == nil {
		bad("SEMANTICS_NATIVE_BINDING", "run-binding")
		return false, nil
	}
	if session.reg.Context != reg.Context {
		bad("SEMANTICS_CONTEXT_BINDING", "eligibility-registration")
	}
	n := session.reg.Native
	inv := session.invocations[binding.InvocationID]
	if inv == nil || n.InvocationID != binding.InvocationID || inv.CohortID != binding.CohortID || session.cohorts[inv.CohortID].Purpose != "current" || inv.Node.ID != binding.DriverNode || inv.Process == nil || binding.Process != *inv.Process {
		bad("SEMANTICS_NATIVE_BINDING", "run-binding")
		return false, nil
	}
	references := map[string]bool{}
	for _, ref := range binding.References {
		current := session.invocations[ref.InvocationID]
		if references[ref.InvocationID] || current == nil || current.Kind != "REFERENCE_PROCESS" || current.CohortID != ref.CohortID || session.cohorts[current.CohortID].Purpose != "current" || current.Receipt == nil || *current.Receipt != ref.Receipt {
			bad("SEMANTICS_REFERENCE_BINDING", ref.InvocationID)
		}
		references[ref.InvocationID] = true
	}
	for _, observation := range session.reg.Observations {
		if !references[observation.InvocationID] {
			bad("SEMANTICS_REFERENCE_BINDING", observation.InvocationID)
		}
	}

	meaning := session.witness.QueryWitnessRegistration
	meaning.Schema = querySemanticsRegistrationSchema
	if !reflect.DeepEqual(meaning, reg.QueryWitnessRegistration) {
		bad("SEMANTICS_WITNESS_BINDING", "witness-registration")
	}
	if state.capture == nil || state.capture.Spec == nil || state.capture.Process == nil || state.capture.Stdout == nil || state.execution == nil {
		bad("SEMANTICS_NATIVE_CAPTURE", "oracle")
		return false, nil
	}
	full := func(pin *ReplayMember, owner string) eligibilityMember {
		if pin == nil {
			return eligibilityMember{}
		}
		p := *pin
		p.Path = "oracle/" + p.Path
		return eligibilityMember{NodeID: owner, Member: p}
	}
	find := func(role string) *ReplayMember {
		for _, mem := range manifest.Members {
			if mem.Role == role {
				return &ReplayMember{Path: mem.Path, Role: mem.Role, Bytes: mem.Bytes, SHA256: mem.SHA256}
			}
		}
		return nil
	}
	if n.Source != full(find("source"), binding.OracleNode) || n.BuildManifest != full(find("build-manifest"), binding.DriverNode) || inv.Process == nil || *inv.Process != full(state.capture.Process, binding.DriverNode) || inv.Response == nil || *inv.Response != full(find("raw"), binding.DriverNode) || inv.Executable == nil || *inv.Executable != full(find("driver"), binding.DriverNode) {
		bad("SEMANTICS_NATIVE_PIN", "run-binding")
	}
	for i, input := range state.build.Closure {
		if e := b.charge(1, 0, "semantic-native-grammar-pin"); e != nil {
			return false, e
		}
		if !strings.HasPrefix(input.Role, "grammar-") {
			continue
		}
		path := fmt.Sprintf("oracle/capture/build-input-%05d.bin", i)
		want := eligibilityMember{NodeID: binding.DriverNode, Member: ReplayMember{Path: path, Role: input.Role, Bytes: input.Bytes, SHA256: input.SHA256}}
		if manifest.Schema == OraclePagedSemanticsManifestSchema && input.Role == "grammar-parser" && input.Bytes > MaxDocumentBytes {
			path = fmt.Sprintf("oracle/capture/build-input-%05d.pages.json", i)
			want = eligibilityMember{NodeID: binding.DriverNode, Member: snapshot.verifiedMembers[path]}
			if want.Member.Role != "grammar-paged-descriptor" {
				bad("SEMANTICS_NATIVE_PIN", path)
			}
		}
		if n.Grammar[input.Path] != want {
			bad("SEMANTICS_NATIVE_PIN", path)
		}
	}
	spec, approved := state.capture.Spec, session.witness.Native
	p := approved.Process
	if spec.Path != p.Path || !slices.Equal(spec.Args, p.Args) || spec.Dir != p.Dir || !slices.Equal(spec.Env, p.Env) || spec.Interactive || spec.StdoutBytes < 0 || uint64(spec.StdoutBytes) != p.StdoutBytes || spec.StderrBytes < 0 || uint64(spec.StderrBytes) != p.StderrBytes || spec.Wall < 0 || uint64(spec.Wall) != p.Wall || spec.Grace < 0 || uint64(spec.Grace) != p.Grace || spec.Memory != p.Memory || spec.CgroupParent != p.CgroupParent {
		bad("SEMANTICS_ACTUAL_SPEC", "oracle/capture")
	}
	if !reflect.DeepEqual(*state.execution, session.semantic.record.process) {
		bad("SEMANTICS_ACTUAL_PROCESS", "oracle/capture")
	}
	return valid, nil
}
