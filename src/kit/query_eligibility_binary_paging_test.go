package kit

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
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
	for _, name := range []string{"current-runtime", "historical-runtime"} {
		t.Run(name, func(t *testing.T) {
			f := newEligibilityFixture(t)
			f.reg.Context = semanticPagedBinariesContext
			if name == "historical-runtime" {
				historical := "affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080"
				f.witness.Native.RuntimeManifestSHA256 = historical
				binaryDriverApproval(f, f.witness.Native.ExecutableSHA256)
				f.replace("native/response.json", bytes.ReplaceAll(f.files["native/response.json"], []byte(reuseobs.RuntimeManifestSHA256), []byte(historical)))
			}
			got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
			if err != nil || got.Assessment != AssessFail || !eligibilityFinding(got, "ELIGIBILITY_NATIVE_BINARY_PAGES") {
				t.Fatalf("err=%v assessment=%s findings=%+v", err, got.Assessment, got.Findings)
			}
			if name == "historical-runtime" && !eligibilityFinding(got, "ELIGIBILITY_NATIVE_HISTORICAL_RUNTIME") {
				t.Fatalf("historical runtime path not reached: %+v", got.Findings)
			}
		})
	}
}

func TestEligibilityBinaryLibraryRoleOnly(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"ordinary-library", "descriptor"} {
		t.Run(name, func(t *testing.T) {
			var f *eligibilityFixture
			if name == "descriptor" {
				f = binaryLibraryFixture(t, func(desc eligibilityMember, logical string) string { return desc.Member.SHA256 })
			} else {
				f = newEligibilityFixtureLibrary(t, func(f *eligibilityFixture) (eligibilityMember, string) {
					pin := f.member("control-compile", "current/library.bin", "library-paged-descriptor", []byte("synthetic library; never loaded"))
					return pin, pin.Member.SHA256
				})
				f.reg.Context = semanticPagedBinariesContext
			}
			got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
			if err != nil || got.Assessment != AssessFail || !eligibilityFinding(got, "ELIGIBILITY_CURRENT_LIBRARY_ARGV") {
				t.Fatalf("err=%v assessment=%s findings=%+v", err, got.Assessment, got.Findings)
			}
		})
	}
}

func TestEligibilityBinaryReservedRoles(t *testing.T) {
	t.Parallel()
	for _, context := range []string{"full-input-utf8/r2", semanticPagedContext} {
		t.Run(context, func(t *testing.T) {
			for _, kind := range []string{"driver", "library"} {
				t.Run(kind, func(t *testing.T) {
					var f *eligibilityFixture
					code := "ELIGIBILITY_NATIVE_EXECUTABLE"
					if kind == "driver" {
						f = binaryDriverFixture(t, MaxDocumentBytes+1)
						binaryDriverApproval(f, digestHex(f.files["oracle/capture/driver.pages.json"]))
					} else {
						f = binaryLibraryFixture(t, func(desc eligibilityMember, logical string) string { return desc.Member.SHA256 })
						code = "ELIGIBILITY_CURRENT_LIBRARY_ARGV"
					}
					f.reg.Context = context
					got, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
					if err != nil || got.Assessment != AssessFail || !eligibilityFinding(got, code) {
						t.Fatalf("err=%v assessment=%s missing=%s findings=%+v", err, got.Assessment, code, got.Findings)
					}
				})
			}
		})
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
