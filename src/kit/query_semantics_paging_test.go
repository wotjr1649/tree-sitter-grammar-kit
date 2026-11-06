package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSemanticPagedParserValidation(t *testing.T) {
	data := bytes.Repeat([]byte("p"), MaxDocumentBytes+1)
	input := semanticBuildInput{Role: "grammar-parser", Path: "parser.c", Bytes: uint64(len(data)), SHA256: digestHex(data)}
	d := semanticParserDescriptor{Schema: "tsgk-query-parser-pages/r1", Owner: "D"}
	d.Logical.ClosureIndex = 7
	d.Logical.Role = input.Role
	d.Logical.Path = input.Path
	d.Logical.Bytes = input.Bytes
	d.Logical.SHA256 = input.SHA256
	parts := map[string][]byte{}
	for i, offset := 0, 0; offset < len(data); i++ {
		size := min(MaxDocumentBytes, len(data)-offset)
		name := fmt.Sprintf("capture/build-input-00007.part-%05d.bin", i)
		part := data[offset : offset+size]
		parts[name] = part
		d.Parts = append(d.Parts, struct {
			Index  uint64 `json:"index"`
			Offset uint64 `json:"offset"`
			Path   string `json:"path"`
			Bytes  uint64 `json:"bytes"`
			SHA256 string `json:"sha256"`
		}{uint64(i), uint64(offset), name, uint64(size), digestHex(part)})
		offset += size
	}
	for _, mutation := range []string{"normal", "offset", "order", "drop", "duplicate", "whole-hash", "part-rehashed", "missing", "index", "owner", "truncated"} {
		t.Run(mutation, func(t *testing.T) {
			copy := d
			copy.Parts = append(copy.Parts[:0:0], d.Parts...)
			supplied := map[string][]byte{}
			for p, b := range parts {
				supplied[p] = b
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
				copy.Logical.ClosureIndex++
			case "owner":
				copy.Owner = "O"
			case "truncated":
				supplied[copy.Parts[0].Path] = supplied[copy.Parts[0].Path][:MaxDocumentBytes-1]
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			r, e := startRun(ctx, time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			b := &eligibilityBudget{r: r}
			good, e := b.semanticPagedParser(eligibilityJSON(t, copy), input, 7, func(pin ReplayMember) ([]byte, bool, *Error) { data, ok := supplied[pin.Path]; return data, ok, nil })
			if e != nil || good != (mutation == "normal") {
				t.Fatalf("mutation=%s good=%v err=%v", mutation, good, e)
			}
		})
	}
}

// Part-count boundaries: exactly 16 MiB is not paged; 2 parts (2x16 MiB, 16 MiB+n Swift8), 3 parts (2x16 MiB+n Kotlin5), and 7 parts (6x16 MiB+n PostgreSQL9) are.
func TestSemanticPagedParserPartBoundaries(t *testing.T) {
	const tail, swiftTail, pgTail = 315585, 13858951, 3622435 // Kotlin5 3-part, Swift8 2-part, and PostgreSQL9 7-part parser tails
	type built struct {
		input semanticBuildInput
		d     semanticParserDescriptor
		parts map[string][]byte
	}
	cache := map[int]built{}
	build := func(size int) built {
		if b, ok := cache[size]; ok {
			return b
		}
		data := bytes.Repeat([]byte("p"), size)
		b := built{input: semanticBuildInput{Role: "grammar-parser", Path: "parser.c", Bytes: uint64(size), SHA256: digestHex(data)}, d: semanticParserDescriptor{Schema: "tsgk-query-parser-pages/r1", Owner: "D"}, parts: map[string][]byte{}}
		b.d.Logical.ClosureIndex = 7
		b.d.Logical.Role = b.input.Role
		b.d.Logical.Path = b.input.Path
		b.d.Logical.Bytes = b.input.Bytes
		b.d.Logical.SHA256 = b.input.SHA256
		for i, offset := 0, 0; offset < size; i++ {
			n := min(MaxDocumentBytes, size-offset)
			name := fmt.Sprintf("capture/build-input-00007.part-%05d.bin", i)
			part := data[offset : offset+n]
			b.parts[name] = part
			b.d.Parts = append(b.d.Parts, struct {
				Index  uint64 `json:"index"`
				Offset uint64 `json:"offset"`
				Path   string `json:"path"`
				Bytes  uint64 `json:"bytes"`
				SHA256 string `json:"sha256"`
			}{uint64(i), uint64(offset), name, uint64(n), digestHex(part)})
			offset += n
		}
		cache[size] = b
		return b
	}
	for _, test := range []struct {
		name     string
		size     int
		mutation string
		paged    bool
		good     bool
	}{
		{"single-part-exactly-16MiB", MaxDocumentBytes, "", false, false},
		{"two-full-parts", 2 * MaxDocumentBytes, "", true, true},
		{"two-parts-partial-tail", MaxDocumentBytes + swiftTail, "", true, true},
		{"two-parts-partial-tail-drop-last", MaxDocumentBytes + swiftTail, "drop-last", true, false},
		{"two-parts-partial-tail-short-last", MaxDocumentBytes + swiftTail, "short-last", true, false},
		{"three-parts", 2*MaxDocumentBytes + tail, "", true, true},
		{"three-parts-drop-last", 2*MaxDocumentBytes + tail, "drop-last", true, false},
		{"three-parts-extra-part", 2*MaxDocumentBytes + tail, "extra-part", true, false},
		{"three-parts-short-last", 2*MaxDocumentBytes + tail, "short-last", true, false},
		{"three-parts-offset-last", 2*MaxDocumentBytes + tail, "offset-last", true, false},
		{"seven-parts-partial-tail", 6*MaxDocumentBytes + pgTail, "", true, true},
		{"seven-parts-drop-last", 6*MaxDocumentBytes + pgTail, "drop-last", true, false},
		{"seven-parts-short-last", 6*MaxDocumentBytes + pgTail, "short-last", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := build(test.size)
			d := b.d
			d.Parts = append(d.Parts[:0:0], b.d.Parts...)
			supplied := map[string][]byte{}
			for p, data := range b.parts {
				supplied[p] = data
			}
			last := len(d.Parts) - 1
			switch test.mutation {
			case "drop-last":
				d.Parts = d.Parts[:last]
			case "extra-part":
				extra := d.Parts[last]
				extra.Index, extra.Offset = extra.Index+1, extra.Offset+extra.Bytes
				d.Parts = append(d.Parts, extra)
			case "short-last":
				supplied[d.Parts[last].Path] = supplied[d.Parts[last].Path][:d.Parts[last].Bytes-1]
			case "offset-last":
				d.Parts[last].Offset--
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			r, e := startRun(ctx, time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			reads := 0
			ok, e := (&eligibilityBudget{r: r}).semanticPagedParser(eligibilityJSON(t, d), b.input, 7, func(pin ReplayMember) ([]byte, bool, *Error) {
				reads++
				data, found := supplied[pin.Path]
				return data, found, nil
			})
			if e != nil || ok != test.good || (!test.paged && reads != 0) {
				t.Fatalf("good=%v want=%v reads=%d err=%v", ok, test.good, reads, e)
			}
		})
	}
}

// A logical parser above BuildFileBytes is rejected from the declared size alone: no 100 MiB allocation and no part read.
func TestSemanticPagedParserBuildFileCeiling(t *testing.T) {
	const size = BuildFileBytes + 1
	input := semanticBuildInput{Role: "grammar-parser", Path: "parser.c", Bytes: size, SHA256: digestHex([]byte("ceiling"))}
	d := semanticParserDescriptor{Schema: "tsgk-query-parser-pages/r1", Owner: "D"}
	d.Logical.ClosureIndex, d.Logical.Role, d.Logical.Path, d.Logical.Bytes, d.Logical.SHA256 = 7, input.Role, input.Path, input.Bytes, input.SHA256
	for i := range uint64(7) { // ceil((BuildFileBytes+1)/16 MiB) is 7, so the part count itself is consistent
		offset := i * MaxDocumentBytes
		d.Parts = append(d.Parts, struct {
			Index  uint64 `json:"index"`
			Offset uint64 `json:"offset"`
			Path   string `json:"path"`
			Bytes  uint64 `json:"bytes"`
			SHA256 string `json:"sha256"`
		}{i, offset, fmt.Sprintf("capture/build-input-00007.part-%05d.bin", i), min(uint64(MaxDocumentBytes), size-offset), digestHex(nil)})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, e := startRun(ctx, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	defer r.cancel()
	reads := 0
	ok, e := (&eligibilityBudget{r: r}).semanticPagedParser(eligibilityJSON(t, d), input, 7, func(ReplayMember) ([]byte, bool, *Error) {
		reads++
		return nil, false, nil
	})
	if e != nil || ok || reads != 0 {
		t.Fatalf("parser above BuildFileBytes: good=%v reads=%d err=%v", ok, reads, e)
	}
}

func TestSemanticPagedPublicAPIAndReplay(t *testing.T) {
	req, f, binding := semanticBoundFixtureMode(t, 0, true, true)
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	set := VerifyOracleSet(os.DirFS(filepath.Join(f.root, "oracle")))
	if !set.Valid {
		t.Fatalf("paged set invalid: %+v", set.Findings)
	}
	var old OracleManifest
	if e := decodeOracleManifestVersion(f.files["oracle/manifest.json"], &old, OracleSemanticsManifestSchema); e == nil || e.Code != "SCHEMA_UNSUPPORTED" {
		t.Fatalf("r5 reader accepted r6: %v", e)
	}
	passed, err := VerifyQuerySemantics(ctx, req)
	if err != nil || !passed.Complete || !passed.BindingValid || passed.Assessment != AssessPass || passed.BaseVerified.Assessment != AssessPass || passed.Tuple.Verified != 3 {
		t.Fatalf("BOUND paging: %v %+v eligibility=%+v", err, passed, passed.Eligibility)
	}
	standalone, err := VerifyQueryReferenceEligibility(ctx, QueryReferenceEligibilityRequest{Evidence: req.Evidence, Approval: req.EligibilityApproval, Registration: req.EligibilityRegistration, WitnessRegistration: req.WitnessRegistration})
	if err != nil || standalone.Assessment != AssessPass || standalone.Tuple.Verified != 3 {
		t.Fatalf("standalone paging: %v %+v", err, standalone)
	}
	absent := req
	absent.EligibilityApproval = nil
	absent.EligibilityRegistration = nil
	absent.WitnessRegistration = nil
	absentBinding := binding
	absentBinding.EligibilityApprovalSHA256 = digestHex(nil)
	absentBinding.EligibilityRegistrationSHA256 = digestHex(nil)
	absentBinding.WitnessRegistrationSHA256 = digestHex(nil)
	absentBinding.CohortID = ""
	absentBinding.InvocationID = ""
	absentBinding.Process = eligibilityMember{}
	absentBinding.References = []semanticReferenceBinding{}
	absent.RunBinding = eligibilityJSON(t, absentBinding)
	blocked, err := VerifyQuerySemantics(ctx, absent)
	if err != nil || !blocked.Complete || blocked.Assessment != AssessBlocked || blocked.BaseVerified.Assessment != AssessPass || blocked.Tuple.Required != 3 || blocked.Tuple.Blocked != 3 {
		t.Fatalf("ABSENT paging: %v %+v", err, blocked)
	}
	root, profile := semanticBoundReplay(t, req, f, binding)
	replayed, err := Replay(ctx, ReplayRequest{Root: root, Profile: profile})
	if err != nil || replayed.Assessment != AssessPass || replayed.QuerySemantics == nil || replayed.QuerySemantics.Tuple != passed.Tuple || replayed.BytesRead != passed.BytesRead || !validSemanticReplayResult(replayed) {
		t.Fatalf("paged replay: %v %+v", err, replayed)
	}
	var wrong map[string]any
	if err := json.Unmarshal(profile, &wrong); err != nil {
		t.Fatal(err)
	}
	wrong["reducer"] = "oracle-set-r4"
	_, err = Replay(ctx, ReplayRequest{Root: root, Profile: eligibilityJSON(t, wrong)})
	if err == nil || asError(err).Code != "SEMANTICS_REDUCER_CONTEXT" {
		t.Fatalf("r4 accepted paged registration: %v", err)
	}
	f.member(binding.DriverNode, "native/unreferenced.part.bin", "grammar-paged-part", []byte("unused"))
	orphaned, err := VerifyQueryReferenceEligibility(ctx, f.request())
	if err != nil || orphaned.Assessment != AssessFail || !eligibilityFinding(orphaned, "ELIGIBILITY_NATIVE_PARSER_PAGES") {
		t.Fatalf("unreferenced paged member accepted: %v %+v", err, orphaned)
	}
	// Approve a consistent graph with a part owned by another real node.
	part := ""
	for p := range f.files {
		if strings.Contains(p, ".part-00000.bin") {
			part = p
			break
		}
	}
	if part == "" {
		t.Fatal("missing physical part")
	}
	native := f.nodes[binding.DriverNode]
	other := f.nodes["control-static"]
	for i, p := range native.Files {
		if p.Path == part {
			other.Files = append(other.Files, p)
			native.Files = append(native.Files[:i], native.Files[i+1:]...)
			break
		}
	}
	f.nodes[native.ID] = native
	f.nodes[other.ID] = other
	for i := range f.reg.OwnedMembers {
		if f.reg.OwnedMembers[i].Member.Path == part {
			f.reg.OwnedMembers[i].NodeID = other.ID
		}
	}
	mutated := f.request()
	rejected, err := VerifyQueryReferenceEligibility(ctx, mutated)
	if err != nil || rejected.Assessment != AssessFail || !eligibilityFinding(rejected, "ELIGIBILITY_MEMBER_OWNER") {
		t.Fatalf("foreign native part owner accepted: %v %+v", err, rejected)
	}
}

func TestSemanticPagedContextRequiresLargeParser(t *testing.T) {
	f := newEligibilityFixture(t)
	f.reg.Context = semanticPagedContext
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got, err := VerifyQueryReferenceEligibility(ctx, f.request())
	if err != nil || got.Assessment != AssessFail || !eligibilityFinding(got, "ELIGIBILITY_NATIVE_PARSER_PAGES") {
		t.Fatalf("small normal parser accepted as paged: %v %+v", err, got)
	}
}
