package kit

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// TestV2ELegacyResultParity freezes complete legacy results, including verdicts,
// codes, schemas and resource counters, across repository-default migrations.
func TestV2ELegacyResultParity(t *testing.T) {
	got := map[string]json.RawMessage{}
	put := func(name string, result any, err error) {
		t.Helper()
		var code string
		if err != nil {
			code = asError(err).Code
		}
		b, marshalErr := json.Marshal(struct {
			Error  string `json:"error"`
			Result any    `json:"result"`
		}{code, result})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		got[name] = b
	}

	t.Run("eligibility", func(t *testing.T) {
		f := newEligibilityFixture(t)
		stableEligibilityFixture(f)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		r, err := VerifyQueryReferenceEligibility(ctx, f.request())
		put("eligibility", r, err)
	})
	t.Run("reference-connection", func(t *testing.T) {
		f := newReferenceFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		r, err := VerifyQueryReferenceConnection(ctx, f.request(t))
		put("reference-connection", r, err)
	})

	for _, manifest := range []struct{ name, sha string }{
		{"current", reuseobs.RuntimeManifestSHA256},
		{"former", "affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080"},
	} {
		for _, mandatory := range []bool{false, true} {
			name := manifest.name + "/optional"
			f := reuseFixtureFor(t, manifest.sha, manifest.sha)
			mutate := func(top map[string]any) {}
			if mandatory {
				name = manifest.name + "/mandatory"
				f = mandatoryFixture(t, false, false, claimPass, AssessPass)
				if manifest.name == "former" {
					rebindReplayFixtureManifest(t, f, manifest.sha)
				}
				mutate = func(top map[string]any) { top["result_schema"] = "tsgk-incremental-result/r3" }
			}
			root, profile := writeReuseFixture(t, f, mutate)
			r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: profile})
			put("replay/"+name, r, err)
		}
	}

	t.Run("accepted", func(t *testing.T) {
		req, raw, projection := acceptedFixture(t, 0)
		setAcceptedEvidence(&req, raw, projection)
		r, err := JudgeReuseAccepted(req)
		put("accepted", r, err)
	})

	t.Run("preprocess-cst", func(t *testing.T) {
		req, _ := preprocessCSTRuntimeRequest(t)
		req.RuntimePins = nil
		put("preprocess-cst", CheckPreprocessCST(req), nil)
	})

	canonical, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("TSGK_V2E_BASELINE_OUT"); path != "" {
		for name, raw := range got {
			sum := sha256.Sum256(raw)
			t.Logf("legacy baseline %q sha256=%x", name, sum)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, canonical, 0600); err != nil {
			t.Fatal(err)
		}
		t.Logf("legacy result snapshot written to %s", path)
		return
	}
	for name, raw := range got {
		sum := sha256.Sum256(raw)
		if expected := v2eLegacyResultSHA256[name]; expected == "" || hex.EncodeToString(sum[:]) != expected {
			t.Errorf("legacy result %q changed: sha256=%x want=%s", name, sum, expected)
		}
	}
}

func stableEligibilityFixture(f *eligibilityFixture) {
	slices.SortFunc(f.reg.OwnedMembers, func(a, b eligibilityMember) int {
		if c := cmp.Compare(a.NodeID, b.NodeID); c != 0 {
			return c
		}
		return cmp.Compare(a.Member.Path, b.Member.Path)
	})
	slices.SortFunc(f.reg.Tools, func(a, b eligibilityTool) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortFunc(f.reg.Invocations, func(a, b eligibilityInvocation) int { return cmp.Compare(a.ID, b.ID) })
	for id, node := range f.nodes {
		slices.SortFunc(node.Files, func(a, b ReplayMember) int { return cmp.Compare(a.Path, b.Path) })
		f.nodes[id] = node
	}
}

func rebindReplayFixtureManifest(t *testing.T, f *fxNative, manifest string) {
	t.Helper()
	for _, c := range f.cases {
		producer := c["producer"].(map[string]any)
		producer["reuse_observation"].(map[string]any)["runtime_manifest_sha256"] = manifest
		steps := c["steps"].([]any)
		claims := make([]reuseobs.Step, 0, len(steps))
		for _, value := range steps {
			step := value.(map[string]any)
			observations := step["observations"].([]reuseobs.Observation)
			for i := range observations {
				observations[i].RuntimeManifestSHA256 = manifest
			}
			step["observations"] = observations
			claims = append(claims, reuseobs.Step{Native: true, Edit: step["step"].(int) > 0, Observations: observations})
		}
		c["reuse_observation_claim"] = reuseobs.CaseClaim(claims, true)
	}
	for path, raw := range f.extra {
		if !strings.HasPrefix(path, "responses/") {
			continue
		}
		var response map[string]any
		if err := json.Unmarshal(raw, &response); err != nil {
			t.Fatal(err)
		}
		producer := response["producer"].(map[string]any)
		producer["reuse_observation"].(map[string]any)["runtime_manifest_sha256"] = manifest
		for _, value := range response["steps"].([]any) {
			step := value.(map[string]any)
			for _, observation := range step["observations"].([]any) {
				observation.(map[string]any)["runtime_manifest_sha256"] = manifest
			}
		}
		data, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		f.extra[path] = data
	}
}

var v2eLegacyResultSHA256 = map[string]string{
	"eligibility":              "699f8a08fa299f6d870e4ba67123519ace27cbd2adde1289b8beac692780a58e",
	"reference-connection":     "85765990401705e2c9c50a1ca48592ea36e3bcafd70d7bc8e9485b98bf06b95a",
	"replay/current/optional":  "ba77d87ba44260abd724c60f00c4515c2655146c05faaec90a87b36d96c05c86",
	"replay/current/mandatory": "78a3bed6e67bfa726e9e2aa2eea8ffee029175b48388d4ced56227d51c58a3fb",
	"replay/former/optional":   "cd6eaa0bcb2f531975d42251787ca6fc492da617aa1ee60b83ab2e820a6168f0",
	"replay/former/mandatory":  "ead99b2a549de9a293bc0d97cabbf69306da3a6db034f78b336dad0451006276",
	"accepted":                 "13edbbb280f61ced1173f0e9a712dbf98ba4ed92099e39ccd4078b900b6f6bb0",
	"preprocess-cst":           "1bcbaa5c7e621e51e72e1d06943bb0035ff06c775d5a16b9557abebd273e6508",
}
