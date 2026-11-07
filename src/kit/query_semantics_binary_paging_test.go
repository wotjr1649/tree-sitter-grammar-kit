package kit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSemanticBinaryPagedReplay(t *testing.T) {
	t.Parallel()
	req, f, binding := semanticBoundFixtureBinaries(t, MaxDocumentBytes+1, false)
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	passed, err := VerifyQuerySemantics(ctx, req)
	if err != nil || passed.Assessment != AssessPass {
		t.Fatalf("binary paging API: %v %+v", err, passed)
	}
	root, profile := semanticBoundReplay(t, req, f, binding)
	replayed, err := Replay(ctx, ReplayRequest{Root: root, Profile: profile})
	if err != nil || replayed.Assessment != AssessPass || replayed.QuerySemantics == nil || replayed.QuerySemantics.Tuple != passed.Tuple || replayed.BytesRead != passed.BytesRead || !validSemanticReplayResult(replayed) {
		t.Fatalf("binary paging replay: %v %+v", err, replayed)
	}
	wrongRevision := replayed
	wrongRevision.Reducer.Revision = "r5"
	if validSemanticReplayResult(wrongRevision) {
		t.Fatal("r6 replay accepted the wrong revision")
	}
	var wrong map[string]any
	if err := json.Unmarshal(profile, &wrong); err != nil {
		t.Fatal(err)
	}
	for _, reducer := range []string{"oracle-set-r5", "oracle-set-r4"} {
		wrong["reducer"] = reducer
		_, err := Replay(ctx, ReplayRequest{Root: root, Profile: eligibilityJSON(t, wrong)})
		if err == nil || asError(err).Code != "SEMANTICS_REDUCER_CONTEXT" {
			t.Fatalf("%s accepted binary paging: %v", reducer, err)
		}
	}
}

func TestSemanticPagedParserRejectsBinariesReducer(t *testing.T) {
	t.Parallel()
	req, f, binding := semanticBoundFixtureMode(t, 0, true, true)
	root, profile := semanticBoundReplay(t, req, f, binding)
	var wrong map[string]any
	if err := json.Unmarshal(profile, &wrong); err != nil {
		t.Fatal(err)
	}
	wrong["reducer"] = "oracle-set-r6"
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	_, err := Replay(ctx, ReplayRequest{Root: root, Profile: eligibilityJSON(t, wrong)})
	if err == nil || asError(err).Code != "SEMANTICS_REDUCER_CONTEXT" {
		t.Fatalf("r6 accepted parser-only paging: %v", err)
	}
}

func TestQualifySemanticReplayRejectsPagedReducers(t *testing.T) {
	t.Parallel()
	for _, reducer := range []string{"oracle-set-r5", "oracle-set-r6"} {
		t.Run(reducer, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			r, e := startRun(ctx, time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			r.eligibility = &eligibilityBudget{r: r}
			_, e = parseQualificationReplay([]byte(`{"reducer":"`+reducer+`"}`), r)
			if e == nil || e.Code != "SEMANTICS_REDUCER_UNSUPPORTED" {
				t.Fatalf("%s: want SEMANTICS_REDUCER_UNSUPPORTED, got %v", reducer, e)
			}
		})
	}
}

func semanticBinaryTestDescriptor(kind string, size uint64, block []byte) (semanticBinaryDescriptor, map[string][]byte, string) {
	owner, path, prefix := "D", "capture/driver.bin", "capture/driver"
	if kind == "library" {
		owner, path, prefix = "control-compile", "current/library.bin", "current/library.bin"
	}
	d := semanticBinaryDescriptor{Schema: semanticBinaryPagesSchema, Owner: owner,
		Logical: semanticBinaryLogical{Kind: kind, Path: path, Bytes: size}}
	parts := map[string][]byte{}
	h := sha256.New()
	for i, offset := uint64(0), uint64(0); offset < size; i++ {
		n := min(uint64(MaxDocumentBytes), size-offset)
		data := block[:n]
		name := fmt.Sprintf("%s.part-%05d.bin", prefix, i)
		parts[name] = data
		d.Parts = append(d.Parts, struct {
			Index  uint64 `json:"index"`
			Offset uint64 `json:"offset"`
			Path   string `json:"path"`
			Bytes  uint64 `json:"bytes"`
			SHA256 string `json:"sha256"`
		}{i, offset, name, n, digestHex(data)})
		_, _ = h.Write(data)
		offset += n
	}
	d.Logical.SHA256 = hex.EncodeToString(h.Sum(nil))
	return d, parts, prefix
}

func TestSemanticPagedBinaryValidation(t *testing.T) {
	t.Parallel()
	block := bytes.Repeat([]byte("p"), MaxDocumentBytes)
	for _, kind := range []string{"driver", "library"} {
		t.Run(kind, func(t *testing.T) {
			d, parts, prefix := semanticBinaryTestDescriptor(kind, MaxDocumentBytes+1, block)
			for _, mutation := range []string{"normal", "offset", "order", "drop", "duplicate", "whole-hash", "part-rehashed", "missing", "index", "owner", "truncated", "kind", "path", "schema", "extra-empty-part"} {
				t.Run(mutation, func(t *testing.T) {
					copy := d
					copy.Parts = append(copy.Parts[:0:0], d.Parts...)
					supplied := map[string][]byte{}
					for p, data := range parts {
						supplied[p] = data
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
						copy.Parts[1].Index++
					case "owner":
						copy.Owner = "other"
					case "truncated":
						supplied[copy.Parts[0].Path] = supplied[copy.Parts[0].Path][:MaxDocumentBytes-1]
					case "kind":
						copy.Logical.Kind = "other"
					case "path":
						copy.Logical.Path = "other.bin"
					case "schema":
						copy.Schema = "tsgk-query-binary-pages/r0"
					case "extra-empty-part":
						extra := copy.Parts[1]
						extra.Index, extra.Offset, extra.Bytes = 2, copy.Logical.Bytes, 0
						extra.Path, extra.SHA256 = fmt.Sprintf("%s.part-00002.bin", prefix), digestHex(nil)
						copy.Parts = append(copy.Parts, extra)
						supplied[extra.Path] = nil
					}
					logical, ok, e := semanticTestBudget(t).semanticPagedBinary(eligibilityJSON(t, copy), d.Owner, kind, d.Logical.Path, prefix, func(p ReplayMember) ([]byte, bool, *Error) {
						data, found := supplied[p.Path]
						return data, found, nil
					})
					if e != nil || ok != (mutation == "normal") || ok && logical != d.Logical {
						t.Fatalf("ok=%v logical=%+v err=%v", ok, logical, e)
					}
				})
			}
		})
	}
}

func TestSemanticPagedBinaryBoundaries(t *testing.T) {
	t.Parallel()
	block := bytes.Repeat([]byte("p"), MaxDocumentBytes)
	for _, kind := range []string{"driver", "library"} {
		t.Run(kind, func(t *testing.T) {
			for _, test := range []struct {
				name  string
				size  uint64
				reads int
			}{
				{"16MiB", MaxDocumentBytes, 0},
				{"16MiB-plus1", MaxDocumentBytes + 1, 2},
				{"32MiB", 2 * MaxDocumentBytes, 2},
				{"32MiB-plus1", 2*MaxDocumentBytes + 1, 3},
				{"64MiB", 4 * MaxDocumentBytes, 4},
				{"64MiB-plus1", 4*MaxDocumentBytes + 1, 0},
			} {
				t.Run(test.name, func(t *testing.T) {
					d, parts, prefix := semanticBinaryTestDescriptor(kind, test.size, block)
					reads := 0
					logical, ok, e := semanticTestBudget(t).semanticPagedBinary(eligibilityJSON(t, d), d.Owner, kind, d.Logical.Path, prefix, func(p ReplayMember) ([]byte, bool, *Error) {
						reads++
						data, found := parts[p.Path]
						return data, found, nil
					})
					if e != nil || ok != (test.reads > 0) || reads != test.reads || ok && logical.Bytes != test.size {
						t.Fatalf("ok=%v reads=%d want=%d logical=%+v err=%v", ok, reads, test.reads, logical, e)
					}
				})
			}
		})
	}
}

func TestSemanticPagedBinaryCharge(t *testing.T) {
	t.Parallel()
	d, parts, prefix := semanticBinaryTestDescriptor("driver", 2*MaxDocumentBytes+1, bytes.Repeat([]byte("p"), MaxDocumentBytes))
	b := semanticTestBudget(t)
	var charges [][2]uint64
	_, ok, e := b.semanticPagedBinary(eligibilityJSON(t, d), d.Owner, "driver", d.Logical.Path, prefix, func(p ReplayMember) ([]byte, bool, *Error) {
		charges = append(charges, [2]uint64{b.work, b.logical})
		data, found := parts[p.Path]
		return data, found, nil
	})
	if e != nil || !ok || len(charges) != 3 {
		t.Fatalf("ok=%v charges=%v err=%v", ok, charges, e)
	}
	charges = append(charges, [2]uint64{b.work, b.logical})
	for i, size := range []uint64{MaxDocumentBytes, MaxDocumentBytes, 1} {
		work, logical := charges[i+1][0]-charges[i][0], charges[i+1][1]-charges[i][1]
		if work != 1 || logical != 2*(size+1) {
			t.Fatalf("part %d charge=(%d,%d) want=(1,%d)", i, work, logical, 2*(size+1))
		}
	}
}

func TestSemanticPagedBinaryRoleBinding(t *testing.T) {
	t.Parallel()
	d, parts, prefix := semanticBinaryTestDescriptor("driver", MaxDocumentBytes+1, bytes.Repeat([]byte("p"), MaxDocumentBytes))
	for _, role := range []string{"driver-paged-part", "grammar-paged-part", "library-paged-part"} {
		t.Run(role, func(t *testing.T) {
			pins := map[string]ReplayMember{}
			for _, p := range d.Parts {
				pins[p.Path] = ReplayMember{Path: p.Path, Role: role, Bytes: p.Bytes, SHA256: p.SHA256}
			}
			_, ok, e := semanticTestBudget(t).semanticPagedBinary(eligibilityJSON(t, d), d.Owner, "driver", d.Logical.Path, prefix, func(p ReplayMember) ([]byte, bool, *Error) {
				pin, found := pins[p.Path]
				return parts[p.Path], found && pin.Role == p.Role && pin.Bytes == p.Bytes && pin.SHA256 == p.SHA256, nil
			})
			if e != nil || ok != (role == "driver-paged-part") {
				t.Fatalf("role=%s ok=%v err=%v", role, ok, e)
			}
		})
	}
}

func TestSemanticBinaryPagedDriverPublicAPI(t *testing.T) {
	t.Parallel()
	req, f, binding := semanticBoundFixtureBinaries(t, MaxDocumentBytes+1, false)
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	set := VerifyOracleSet(os.DirFS(filepath.Join(f.root, "oracle")))
	if !set.Valid {
		t.Fatalf("binary-paged set invalid: %+v", set.Findings)
	}
	var old OracleManifest
	if e := decodeOracleManifestVersion(f.files["oracle/manifest.json"], &old, OraclePagedSemanticsManifestSchema); e == nil || e.Code != "SCHEMA_UNSUPPORTED" {
		t.Fatalf("r6 reader accepted r7: %v", e)
	}
	passed, err := VerifyQuerySemantics(ctx, req)
	if err != nil || !passed.Complete || !passed.BindingValid || passed.Assessment != AssessPass || passed.Tuple.Verified != 3 {
		t.Fatalf("BOUND binary paging: %v %+v eligibility=%+v", err, passed, passed.Eligibility)
	}
	standalone, err := VerifyQueryReferenceEligibility(ctx, QueryReferenceEligibilityRequest{Evidence: req.Evidence, Approval: req.EligibilityApproval, Registration: req.EligibilityRegistration, WitnessRegistration: req.WitnessRegistration})
	if err != nil || standalone.Assessment != AssessPass {
		t.Fatalf("standalone binary paging: %v %+v", err, standalone)
	}
	absent := req
	absent.EligibilityApproval, absent.EligibilityRegistration, absent.WitnessRegistration = nil, nil, nil
	binding.EligibilityApprovalSHA256, binding.EligibilityRegistrationSHA256, binding.WitnessRegistrationSHA256 = digestHex(nil), digestHex(nil), digestHex(nil)
	binding.CohortID, binding.InvocationID = "", ""
	binding.Process = eligibilityMember{}
	binding.References = []semanticReferenceBinding{}
	absent.RunBinding = eligibilityJSON(t, binding)
	blocked, err := VerifyQuerySemantics(ctx, absent)
	if err != nil || !blocked.Complete || blocked.Assessment != AssessBlocked || blocked.BaseVerified.Assessment != AssessPass || blocked.Tuple.Blocked != 3 {
		t.Fatalf("ABSENT binary paging: %v %+v", err, blocked)
	}
}

func TestSemanticBinaryPagedDriverAndParser(t *testing.T) {
	t.Parallel()
	req, f, _ := semanticBoundFixtureBinaries(t, MaxDocumentBytes+1, true)
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	result, err := VerifyQuerySemantics(ctx, req)
	if err != nil || result.Assessment != AssessPass || !result.BindingValid || result.Tuple.Verified != 3 {
		t.Fatalf("driver and parser paging: %v %+v eligibility=%+v", err, result, result.Eligibility)
	}
	var m OracleManifest
	if err := json.Unmarshal(f.files["oracle/manifest.json"], &m); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, member := range m.Members {
		counts[member.Role]++
	}
	if counts["grammar-paged-descriptor"] != 1 || counts["driver-paged-descriptor"] != 1 || counts["driver"] != 0 {
		t.Fatalf("unexpected paging roles: %v", counts)
	}
}

func TestSemanticBinaryPagedSetRejects(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"build-descriptor-identity", "r6-manifest", "unused-driver-part", "driver-and-descriptor", "parser-only"} {
		t.Run(name, func(t *testing.T) {
			o := semanticBoundOptions{DriverBytes: MaxDocumentBytes + 1}
			code, path := "", ""
			switch name {
			case "build-descriptor-identity":
				code, path = "SEMANTICS_BUILD_BINDING", "capture/build.json"
				o.Build = func(build *semanticBuildDescription) {
					d, _, _ := semanticBinaryTestDescriptor("driver", MaxDocumentBytes+1, bytes.Repeat([]byte("d"), MaxDocumentBytes))
					data := eligibilityJSON(t, d)
					build.ExecutableSHA256, build.ExecutableBytes = digestHex(data), uint64(len(data))
				}
			case "r6-manifest":
				o.ManifestSchema = OraclePagedSemanticsManifestSchema
				code, path = "MEMBER_INVALID", "capture/driver.pages.json"
			case "unused-driver-part":
				code, path = "SEMANTICS_DRIVER_PAGES", "capture/driver.pages.json"
				o.Extra = func(add func(string, string, []byte) ReplayMember) {
					add("capture/driver.part-00002.bin", "driver-paged-part", []byte("unused"))
				}
			case "driver-and-descriptor":
				code, path = "SEMANTICS_MEMBER_COUNT", "driver"
				o.Extra = func(add func(string, string, []byte) ReplayMember) {
					add("capture/driver.bin", "driver", []byte("plain driver"))
				}
			case "parser-only":
				o.DriverBytes, o.PagedParser, o.ManifestSchema = 0, true, OraclePagedBinariesSemanticsManifestSchema
			}
			req, f, _ := semanticBoundFixtureWith(t, 0, true, o)
			if name == "parser-only" {
				ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
				defer cancel()
				result, err := VerifyQuerySemantics(ctx, req)
				if err != nil || result.Assessment != AssessFail || result.Eligibility == nil || !eligibilityFinding(*result.Eligibility, "ELIGIBILITY_NATIVE_BINARY_PAGES") {
					t.Fatalf("parser-only binaries accepted: %v %+v eligibility=%+v", err, result, result.Eligibility)
				}
				return
			}
			set := VerifyOracleSet(os.DirFS(filepath.Join(f.root, "oracle")))
			found := false
			for _, finding := range set.Findings {
				found = found || finding.Code == code && finding.Path == path
			}
			if set.Valid || !found {
				t.Fatalf("%s: want %s at %s, got %+v", name, code, path, set.Findings)
			}
		})
	}
}

func TestSemanticBinaryPagedRehashCannotReplaceProfile(t *testing.T) {
	t.Parallel()
	req, f, binding := semanticBoundFixtureBinaries(t, MaxDocumentBytes+1, false)
	var d semanticBinaryDescriptor
	if err := json.Unmarshal(f.files["oracle/capture/driver.pages.json"], &d); err != nil {
		t.Fatal(err)
	}
	last := len(d.Parts) - 1
	if last < 0 || d.Parts[last].Bytes != 1 {
		t.Fatal("independent synthetic boundary changed")
	}
	f.replace("oracle/"+d.Parts[last].Path, []byte("q"))
	d.Parts[last].SHA256 = digestHex([]byte("q"))
	h := sha256.New()
	for _, p := range d.Parts {
		_, _ = h.Write(f.files["oracle/"+p.Path])
	}
	d.Logical.SHA256 = hex.EncodeToString(h.Sum(nil))
	f.replace("oracle/capture/driver.pages.json", eligibilityJSON(t, d))
	semanticBoundReapprove(t, &req, f, &binding)
	set := VerifyOracleSet(os.DirFS(filepath.Join(f.root, "oracle")))
	found := false
	for _, finding := range set.Findings {
		found = found || finding.Code == "SEMANTICS_BUILD_BINDING"
	}
	if set.Valid || !found {
		t.Fatalf("rehashed driver replaced build identity: %+v", set.Findings)
	}
	// Driver identity is pinned by build and approval; the profile pins grammar inputs.
	var build semanticBuildDescription
	if err := json.Unmarshal(f.files["oracle/capture/build.json"], &build); err != nil {
		t.Fatal(err)
	}
	build.ExecutableSHA256 = d.Logical.SHA256
	f.replace("oracle/capture/build.json", eligibilityJSON(t, build))
	var manifest OracleManifest
	if err := json.Unmarshal(f.files["oracle/manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Producer["executable_sha256"] = d.Logical.SHA256
	f.replace("oracle/manifest.json", mustJSON(t, manifest))
	semanticBoundReapprove(t, &req, f, &binding)
	set = VerifyOracleSet(os.DirFS(filepath.Join(f.root, "oracle")))
	if !set.Valid {
		t.Fatalf("rehashed set is not internally coherent: %+v", set.Findings)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	result, err := VerifyQuerySemantics(ctx, req)
	if err != nil || result.Assessment != AssessFail || result.Eligibility == nil || !eligibilityFinding(*result.Eligibility, "ELIGIBILITY_NATIVE_EXECUTABLE") {
		t.Fatalf("rehashed driver bypassed approved executable identity: %v %+v eligibility=%+v", err, result, result.Eligibility)
	}
}
