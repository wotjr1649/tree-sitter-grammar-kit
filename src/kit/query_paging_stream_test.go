package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func reorderDriverParts(f *eligibilityFixture) {
	n := f.nodes["native-current"]
	first := -1
	for i, p := range n.Files {
		if p.Role == "driver-paged-part" {
			if first < 0 {
				first = i
			} else {
				n.Files[first], n.Files[i] = n.Files[i], n.Files[first]
				break
			}
		}
	}
	f.nodes[n.ID] = n
}

func TestStreamedPagingStorageAndFallback(t *testing.T) {
	for _, reorder := range []bool{false, true} {
		t.Run(map[bool]string{false: "stream", true: "retained"}[reorder], func(t *testing.T) {
			f := binaryDriverFixture(t, MaxDocumentBytes+1)
			if reorder {
				reorderDriverParts(f)
			}
			s, _ := verifiedEligibilitySession(t, f.request())
			for path, part := range s.snapshot.parts {
				if (part.chain != nil) == reorder || (part.content != nil) != reorder {
					t.Fatalf("wrong storage: %s streamed=%v retained=%d", path, part.chain != nil, len(part.content))
				}
				if !reorder {
					_, ok, e := s.member(&eligibilityMember{NodeID: s.owners[path], Member: s.snapshot.verifiedMembers[path]})
					if ok || e == nil || e.Code != "ELIGIBILITY_MEMBER_BYTES_UNAVAILABLE" {
						t.Fatalf("streamed bytes escaped: %v %v", ok, e)
					}
				}
			}
			source, d, data := verifiedDriverSource(t, s, false)
			logical, ok, e := s.b.semanticPagedBinaryVerified(data, "D", "driver", d.Logical.Path, "capture/driver", noPagingCallback(t), source)
			if e != nil || !ok || logical != d.Logical {
				t.Fatalf("chain: %v %v", ok, e)
			}
		})
	}
}

func TestStreamedPagingLateInvalidNodeFallback(t *testing.T) {
	for _, mutation := range []string{"ref", "duplicate-run", "policy"} {
		t.Run(mutation, func(t *testing.T) {
			f := binaryDriverFixture(t, MaxDocumentBytes+1)
			n := f.nodes["native-current"]
			if mutation == "ref" {
				n.Refs = []EvidenceRef{{Relation: "supersedes", Node: "missing"}}
			} else if mutation == "duplicate-run" {
				other := f.nodes["build-static"]
				for _, role := range []string{"run", "attempt", "platform"} {
					other.Identities[role] = n.Identities[role]
				}
				f.nodes[other.ID] = other
			}
			f.nodes[n.ID] = n
			req := f.request()
			if mutation == "policy" {
				var policy evidencePolicy
				if err := json.Unmarshal(req.Evidence.Policy, &policy); err != nil {
					t.Fatal(err)
				}
				for i := range policy.Required {
					if policy.Required[i].Node == n.ID {
						policy.Required[i].Identities["source"] = strings.Repeat("e", 40)
					}
				}
				req.Evidence.Policy = eligibilityJSON(t, map[string]any{"schema": EvidencePolicySchema, "id": policy.ID, "evidence_sha256": policy.EvidenceSHA256, "required": policy.Required, "carry_forward": policy.CarryForward})
				f.approval.EvidencePolicySHA256 = digestHex(req.Evidence.Policy)
				req.Approval = eligibilityJSON(t, f.approval)
			}
			baseline, err := VerifyQueryReferenceEligibility(ctxT(t), req)
			if err != nil || baseline.Assessment != AssessFail {
				t.Fatalf("baseline: %v %+v", err, baseline)
			}
			root, err := os.OpenRoot(req.Evidence.Root)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			r, e := startRun(ctxT(t), time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			r.eligibility = &eligibilityBudget{r: r}
			scope := &eligibilityScope{r: r, root: root}
			got, err := verifyQueryReferenceEligibilityInRun(ctxT(t), req, scope)
			if err != nil || !reflect.DeepEqual(got.Findings, baseline.Findings) || got.Assessment != baseline.Assessment || got.Complete != baseline.Complete || got.Tuple != baseline.Tuple {
				t.Fatalf("changed classification: %v %+v vs %+v", err, got, baseline)
			}
			for _, part := range scope.session.snapshot.parts {
				if part.chain != nil {
					t.Fatal("invalid node streamed away fallback bytes")
				}
			}
		})
	}
}

// A later chain part that fails file verification must keep the standalone FILE_MEMBER_* findings,
// not surface as an internal paging identity error when an earlier streamed part is looked up.
func TestStreamedPagingLatePartFileFailureKeepsFindings(t *testing.T) {
	for _, mutation := range []string{"missing", "mismatch"} {
		t.Run(mutation, func(t *testing.T) {
			f := binaryDriverFixture(t, MaxDocumentBytes+1)
			req := f.request()
			late := ""
			for _, p := range f.nodes["native-current"].Files {
				if p.Role == "driver-paged-part" && strings.HasSuffix(p.Path, ".part-00001.bin") {
					late = filepath.Join(req.Evidence.Root, filepath.FromSlash(p.Path))
				}
			}
			if late == "" {
				t.Fatal("fixture has no second driver part")
			}
			if mutation == "missing" {
				if err := os.Remove(late); err != nil {
					t.Fatal(err)
				}
			} else {
				fh, err := os.OpenFile(late, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				_, err = fh.Write([]byte("x"))
				if cerr := fh.Close(); err == nil {
					err = cerr
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			baseline, err := VerifyQueryReferenceEligibility(ctxT(t), req)
			if err != nil || baseline.Assessment == AssessPass {
				t.Fatalf("baseline: %v %+v", err, baseline)
			}
			root, err := os.OpenRoot(req.Evidence.Root)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			r, e := startRun(ctxT(t), time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			r.eligibility = &eligibilityBudget{r: r}
			got, err := verifyQueryReferenceEligibilityInRun(ctxT(t), req, &eligibilityScope{r: r, root: root})
			if err != nil || !reflect.DeepEqual(got.Findings, baseline.Findings) || got.Assessment != baseline.Assessment || got.Complete != baseline.Complete {
				t.Fatalf("late part failure changed classification: %v %+v vs %+v", err, got, baseline)
			}
		})
	}
}
