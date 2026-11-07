package kit

import (
	"bytes"
	"slices"
	"testing"
)

func binaryDriverApproval(f *eligibilityFixture, sha string) {
	f.witness.Native.ExecutableSHA256 = sha
	for i := range f.reg.Invocations {
		inv := &f.reg.Invocations[i]
		if inv.ID == f.reg.Native.InvocationID {
			inv.ApprovedRequest = eligibilityJSON(f.t, f.witness.Native)
			f.replace(inv.Request.Member.Path, inv.ApprovedRequest)
		}
	}
}

func binaryDriverFixture(t *testing.T, size int) *eligibilityFixture {
	t.Helper()
	f := newEligibilityFixture(t)
	f.reg.Context = semanticPagedBinariesContext
	d, parts, _ := semanticBinaryTestDescriptor("driver", uint64(size), bytes.Repeat([]byte("d"), MaxDocumentBytes))
	for _, part := range d.Parts {
		f.member("native-current", "oracle/"+part.Path, "driver-paged-part", parts[part.Path])
	}
	pin := f.member("native-current", "oracle/capture/driver.pages.json", "driver-paged-descriptor", eligibilityJSON(t, d))
	for i := range f.reg.Invocations {
		if f.reg.Invocations[i].ID == f.reg.Native.InvocationID {
			f.reg.Invocations[i].Executable = &pin
		}
	}
	binaryDriverApproval(f, d.Logical.SHA256)
	return f
}

func TestEligibilityBinaryPagedDriver(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"normal", "approval-descriptor-sha", "descriptor-foreign-owner", "unused-part", "three-parts", "non-binaries-context"} {
		t.Run(name, func(t *testing.T) {
			size := MaxDocumentBytes + 1
			if name == "three-parts" {
				size = 2*MaxDocumentBytes + 1
			}
			f := binaryDriverFixture(t, size)
			code := ""
			switch name {
			case "approval-descriptor-sha":
				binaryDriverApproval(f, digestHex(f.files["oracle/capture/driver.pages.json"]))
				code = "ELIGIBILITY_NATIVE_EXECUTABLE"
			case "descriptor-foreign-owner":
				path := "oracle/capture/driver.pages.json"
				for i := range f.reg.OwnedMembers {
					m := &f.reg.OwnedMembers[i]
					if m.Member.Path == path {
						m.NodeID = "native-static"
						n := f.nodes[m.NodeID]
						n.Files = append(n.Files, m.Member)
						f.nodes[m.NodeID] = n
					}
				}
				n := f.nodes["native-current"]
				n.Files = slices.DeleteFunc(n.Files, func(m ReplayMember) bool { return m.Path == path })
				f.nodes[n.ID] = n
				for i := range f.reg.Invocations {
					inv := &f.reg.Invocations[i]
					if inv.ID == f.reg.Native.InvocationID {
						inv.Executable.NodeID = "native-static"
					}
				}
				code = "ELIGIBILITY_NATIVE_DRIVER_PAGES"
			case "unused-part":
				f.member("native-current", "oracle/capture/driver.part-00002.bin", "driver-paged-part", []byte("unused"))
				code = "ELIGIBILITY_NATIVE_DRIVER_PAGES"
			case "non-binaries-context":
				f.reg.Context = "full-input-utf8/r2"
				code = "ELIGIBILITY_NATIVE_EXECUTABLE"
			}
			got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
			if err != nil {
				t.Fatal(err)
			}
			if code == "" {
				if got.Assessment != AssessPass || !got.Complete {
					t.Fatalf("assessment=%s complete=%v findings=%+v", got.Assessment, got.Complete, got.Findings)
				}
			} else if !eligibilityFinding(got, code) {
				t.Fatalf("missing %s: %+v", code, got.Findings)
			}
		})
	}
}

func TestEligibilityBinaryPagesRequired(t *testing.T) {
	t.Parallel()
	f := newEligibilityFixture(t)
	f.reg.Context = semanticPagedBinariesContext
	got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
	if err != nil || !eligibilityFinding(got, "ELIGIBILITY_NATIVE_BINARY_PAGES") {
		t.Fatalf("err=%v findings=%+v", err, got.Findings)
	}
}
