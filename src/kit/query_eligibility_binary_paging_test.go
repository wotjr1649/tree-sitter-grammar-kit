package kit

import (
	"bytes"
	jsonv2 "encoding/json/v2"
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

func binaryLibraryFixture(t *testing.T, argvSHA func(desc eligibilityMember, logical string) string) *eligibilityFixture {
	t.Helper()
	f := newEligibilityFixtureLibrary(t, func(f *eligibilityFixture) (eligibilityMember, string) {
		d, parts, _ := semanticBinaryTestDescriptor("library", MaxDocumentBytes+1, bytes.Repeat([]byte("l"), MaxDocumentBytes))
		for _, part := range d.Parts {
			f.member("control-compile", part.Path, "library-paged-part", parts[part.Path])
		}
		pin := f.member("control-compile", "current/library.bin.pages.json", "library-paged-descriptor", eligibilityJSON(t, d))
		sha := d.Logical.SHA256
		if argvSHA != nil {
			sha = argvSHA(pin, sha)
		}
		return pin, sha
	})
	f.reg.Context = semanticPagedBinariesContext
	return f
}

func TestEligibilityBinaryPagedLibrary(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"normal", "argv-descriptor-sha", "descriptor-owner-field", "unused-part", "part-owned-by-driver-node", "driver-and-library", "non-binaries-context"} {
		t.Run(name, func(t *testing.T) {
			var argvSHA func(eligibilityMember, string) string
			if name == "argv-descriptor-sha" {
				argvSHA = func(desc eligibilityMember, logical string) string { return desc.Member.SHA256 }
			}
			f := binaryLibraryFixture(t, argvSHA)
			code := ""
			switch name {
			case "argv-descriptor-sha":
				code = "ELIGIBILITY_CURRENT_LIBRARY_ARGV"
			case "descriptor-owner-field":
				path := f.reg.Native.ReferenceLibrary.Member.Path
				var d semanticBinaryDescriptor
				if err := jsonv2.Unmarshal(f.files[path], &d); err != nil {
					t.Fatal(err)
				}
				d.Owner = "native-current"
				f.replace(path, eligibilityJSON(t, d))
				code = "ELIGIBILITY_NATIVE_LIBRARY_PAGES"
			case "unused-part":
				f.member("control-compile", "current/library.bin.part-00002.bin", "library-paged-part", []byte("unused"))
				code = "ELIGIBILITY_NATIVE_LIBRARY_PAGES"
			case "part-owned-by-driver-node":
				path := "current/library.bin.part-00000.bin"
				for i := range f.reg.OwnedMembers {
					m := &f.reg.OwnedMembers[i]
					if m.Member.Path == path {
						m.NodeID = "native-current"
						n := f.nodes[m.NodeID]
						n.Files = append(n.Files, m.Member)
						f.nodes[m.NodeID] = n
					}
				}
				n := f.nodes["control-compile"]
				n.Files = slices.DeleteFunc(n.Files, func(m ReplayMember) bool { return m.Path == path })
				f.nodes[n.ID] = n
				code = "ELIGIBILITY_MEMBER_OWNER"
			case "driver-and-library":
				d, parts, _ := semanticBinaryTestDescriptor("driver", MaxDocumentBytes+1, bytes.Repeat([]byte("d"), MaxDocumentBytes))
				for _, part := range d.Parts {
					f.member("native-current", "oracle/"+part.Path, "driver-paged-part", parts[part.Path])
				}
				pin := f.member("native-current", "oracle/capture/driver.pages.json", "driver-paged-descriptor", eligibilityJSON(t, d))
				for i := range f.reg.Invocations {
					inv := &f.reg.Invocations[i]
					if inv.ID == f.reg.Native.InvocationID {
						inv.Executable = &pin
					}
				}
				binaryDriverApproval(f, d.Logical.SHA256)
			case "non-binaries-context":
				f.reg.Context = "full-input-utf8/r2"
				code = "ELIGIBILITY_CURRENT_LIBRARY_ARGV"
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
