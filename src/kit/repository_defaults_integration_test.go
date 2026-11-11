package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evidencepins "github.com/wotjr1649/tree-sitter-grammar-kit/src/contracts/evidence-pins"
)

func TestRepositoryDefaultsProvenanceAndAuthority(t *testing.T) {
	store, err := evidencepins.Repository()
	if err != nil {
		t.Fatal(err)
	}
	id, err := RepositoryEvidenceDefaults()
	if err != nil || id.ResultSchema != "tsgk-repository-evidence-approval/r1" || id.ApprovalSource != "repository-default" ||
		id.ApprovalSHA256 != store.Identity().SelectionSHA256 || id.PinSetSHA256 != store.Identity().PinSetSHA256 || id.PolicySHA256 != store.Identity().PolicySHA256 {
		t.Fatalf("default provenance lost: %+v %v", id, err)
	}
	for _, missing := range []bool{false, true} {
		f := newEligibilityFixture(t)
		req := f.request()
		if missing {
			req.Approval = nil
		}
		r, e := startRun(ctxT(t), ReplayOperations()["evidence-replay"].Wall)
		if e != nil {
			t.Fatal(e)
		}
		previousCaller := &eligibilityRuntimePins{}
		previousDefault := &evidencepins.Runtime{ManifestSHA256: strings.Repeat("f", 64)}
		b := &eligibilityBudget{r: r, runtime: previousCaller, defaultRuntime: previousDefault}
		r.eligibility = b
		result, err := verifyQueryReferenceEligibilityInRun(ctxT(t), req, &eligibilityScope{r: r})
		r.cancel()
		if err != nil || !missing && result.Assessment != AssessPass || b.runtime != previousCaller || b.defaultRuntime != previousDefault {
			t.Fatalf("default/caller authority leaked across subject: missing=%v result=%+v error=%v", missing, result, err)
		}
	}
}

func defaultToolchainRequest(t *testing.T, rustc, cargo string) QueryReferenceEligibilityRequest {
	t.Helper()
	f := newEligibilityFixture(t)
	var identity eligibilityBuildIdentity
	if err := json.Unmarshal(f.files["build/identity.json"], &identity); err != nil {
		t.Fatal(err)
	}
	for name, release := range map[string]string{"rustc": rustc, "cargo": cargo} {
		identity.Versions[name] = "release: " + release + "\n"
		f.stdoutMutation("build-"+name+"-version", func([]byte) []byte { return []byte(identity.Versions[name]) })
	}
	f.replace("build/identity.json", eligibilityJSON(t, identity))
	var summary eligibilityControlSummary
	if err := json.Unmarshal(f.files[f.reg.Control.Summary.Member.Path], &summary); err != nil {
		t.Fatal(err)
	}
	summary.IdentitySHA256 = f.reg.Build.Identity.Member.SHA256
	f.replace(f.reg.Control.Summary.Member.Path, eligibilityJSON(t, summary))
	return f.request()
}

func TestRepositoryDefaultsRejectUnapprovedToolchain(t *testing.T) {
	result, err := VerifyQueryReferenceEligibility(ctxT(t), defaultToolchainRequest(t, "9.99.99", "8.88.88"))
	if err != nil || result.Assessment != AssessFail || !eligibilityFinding(result, "ELIGIBILITY_TOOL_VERSION") {
		t.Fatalf("unapproved default toolchain accepted: %+v %v", result, err)
	}
}

// The package fixture exports independently pinned B/C additions for this integration check.
func TestRepositoryDefaultsCandidateAdmission(t *testing.T) {
	candidate := os.Getenv("TSGK_DEFAULTS_CANDIDATE_OUT")
	if candidate == "" {
		t.Skip("requires the exported, separately pinned B/C default fixture and Go overlay")
	}
	for _, pair := range [][2]string{{"2.3.5", "7.8.9"}, {"3.4.6", "8.9.1"}} {
		result, err := VerifyQueryReferenceEligibility(ctxT(t), defaultToolchainRequest(t, pair[0], pair[1]))
		if err != nil || result.Assessment != AssessPass || !result.Complete || result.Build.Verified != 9 || result.Tuple.Verified != 3 {
			t.Fatalf("approved B/C toolchain rejected: %+v %v", result, err)
		}
	}
	for _, pair := range [][2]string{{"2.3.5", "8.9.1"}, {"3.4.6", "7.8.9"}} {
		result, err := VerifyQueryReferenceEligibility(ctxT(t), defaultToolchainRequest(t, pair[0], pair[1]))
		if err != nil || result.Assessment != AssessFail || !eligibilityFinding(result, "ELIGIBILITY_TOOL_VERSION") {
			t.Fatalf("mixed approved toolchains accepted: %+v %v", result, err)
		}
	}
	read := func(relative string, out any) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(candidate, filepath.FromSlash(relative)))
		if err != nil || json.Unmarshal(data, out) != nil {
			t.Fatalf("candidate fixture %s: %v", relative, err)
		}
	}
	var pointer struct {
		Set string `json:"set_sha256"`
	}
	read("repository-defaults.json", &pointer)
	var set struct {
		Policy string `json:"policy_sha256"`
	}
	read("default-sets/"+pointer.Set+".json", &set)
	var policy struct {
		Runtimes []struct {
			Manifest string `json:"manifest_sha256"`
		} `json:"runtimes"`
	}
	read("objects/"+set.Policy, &policy)
	store, err := evidencepins.Repository()
	if err != nil || len(policy.Runtimes) < 4 {
		t.Fatalf("B/C runtime fixture absent: %v", err)
	}
	for _, entry := range policy.Runtimes[2:] {
		runtime, ok := store.RuntimeFor(entry.Manifest)
		if !ok || !runtime.Closure {
			t.Fatal("B/C runtime not approved for closure")
		}
		materials := runtimePinMaterials(t)
		for name, sha := range map[string]string{
			"native/runtime-tuple.json": runtime.TupleSHA256, "native/runtime-manifest.json": runtime.ManifestSHA256,
			"native/driver.c": runtime.DriverSHA256, "native/cp949_table.h": runtime.CP949SHA256, "native/shim-template.c": runtime.ShimTemplateSHA256,
		} {
			data, err := store.Object(sha)
			if err != nil {
				t.Fatal(err)
			}
			materials[name] = data
		}
		f := newEligibilityFixture(t)
		runtimePinRebind(t, f, materials)
		result, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
		if err != nil || result.Assessment != AssessPass || !result.Complete || result.Tuple.Verified != 3 || result.RuntimePin != nil {
			t.Fatalf("approved default B/C runtime rejected or elevated to caller: %+v %v", result, err)
		}
	}
}
