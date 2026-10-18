package kit

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type referenceFixture struct {
	registration QueryReferenceRegistration
	nodes        []EvidenceNode
	files        map[string][]byte
	witness      map[string]any
	raw          map[string]any
	projection   map[string]any
	responses    map[string]map[string]any
}

func newReferenceFixture(t *testing.T) *referenceFixture {
	t.Helper()
	req, w, raw, projection := witnessFixture(t)
	req = marshalWitness(t, req, w, raw, projection)
	f := &referenceFixture{files: map[string][]byte{"witness.json": req.Registration, "source.txt": req.Source, "raw.json": req.Raw, "projection.json": req.Projection}, witness: w, raw: raw, projection: projection, responses: map[string]map[string]any{}}
	member := func(path, role string) ReplayMember {
		b := f.files[path]
		return ReplayMember{Path: path, Role: role, Bytes: uint64(len(b)), SHA256: sum(b)}
	}
	ni, ri := ident("native-policy", "native-run"), ident("reference-policy", "reference-run")
	f.registration = QueryReferenceRegistration{Schema: QueryReferenceRegistrationSchema, Context: "full-input-utf8/r1", Native: QueryReferenceNode{ID: "native", Identities: ni}, Reference: QueryReferenceNode{ID: "reference", Identities: ri}, BinarySHA256: sum([]byte("approved-binary")), LibrarySHA256: sum([]byte("approved-library")), Symbol: "tree_sitter_owned"}
	f.registration.WitnessRegistration = member("witness.json", "registration")
	f.registration.Source = member("source.txt", "source")
	f.registration.Raw = member("raw.json", "raw")
	f.registration.Projection = member("projection.json", "projection")
	f.files["build.json"] = mustJSON(t, map[string]any{"schema": instrumentedReferenceBuildSchema, "upstream_commit": instrumentedReferenceCommit, "target": "x86_64-pc-windows-gnu", "binary_sha256": f.registration.BinarySHA256, "complete": true})
	f.registration.Build = member("build.json", "build")
	for i, source := range []string{"a", "b"} {
		path := []string{"reference-0.json", "reference-1.json"}[i]
		p := projection["steps"].([]any)[i].(map[string]any)["incremental"].(map[string]any)
		q := p["queries"].([]any)[0].(map[string]any)
		structural := slices.Clone(w["required"].([]any)[i].(map[string]any)["structural"].([]Capture))
		r := map[string]any{"schema": instrumentedReferenceSchema, "status": StatusCompleted, "code": "", "complete": true, "upstream_commit": instrumentedReferenceCommit,
			"input": map[string]any{"encoding": EncodingUTF8, "bytes": len(source), "sha256": sum([]byte(source))}, "grammar": map[string]any{"library_sha256": f.registration.LibrarySHA256, "symbol": f.registration.Symbol}, "tree": map[string]any{"nodes": fxTree(source, false)},
			"query":  map[string]any{"id": "q", "sha256": q["sha256"], "patterns": q["patterns"], "capture_names": q["capture_names"], "predicates": q["predicates"], "matches": 1, "match_mapping": []any{map[string]any{"match": 0, "runtime_id": 19}}, "structural": structural, "evaluated": slices.Clone(q["captures"].([]Capture))},
			"limits": map[string]any{"nodes": 100000, "captures": 50000, "group_captures": 200000, "matches": 100000, "wall_ms": 3000}, "cursor_exhausted": true, "did_exceed_match_limit": false}
		f.responses[path] = r
		f.files[path] = mustJSON(t, r)
		f.registration.Observations = append(f.registration.Observations, QueryReferenceObservation{Query: "q", Step: uint32(i), SourceBytes: 1, SourceSHA256: sum([]byte(source)), Response: member(path, "reference-response")})
	}
	f.nodes = []EvidenceNode{{ID: "native", Kind: "run", ExecutionStatus: StatusCompleted, EvidenceMode: ModeNewRun, Assessment: AssessPass, Identities: ni, Files: []ReplayMember{f.registration.WitnessRegistration, f.registration.Source, f.registration.Raw, f.registration.Projection}, Refs: []EvidenceRef{}}, {ID: "reference", Kind: "run", ExecutionStatus: StatusCompleted, EvidenceMode: ModeNewRun, Assessment: AssessPass, Identities: ri, Files: []ReplayMember{f.registration.Build, f.registration.Observations[0].Response, f.registration.Observations[1].Response}, Refs: []EvidenceRef{}}}
	return f
}

// Rehashing a submitted graph never changes the caller's independent member pins.
func (f *referenceFixture) request(t *testing.T) QueryReferenceConnectionRequest {
	t.Helper()
	if _, ok := f.files["witness.json"]; ok {
		f.files["witness.json"] = mustJSON(t, f.witness)
	}
	if _, ok := f.files["raw.json"]; ok {
		f.files["raw.json"] = mustJSON(t, f.raw)
	}
	if _, ok := f.files["projection.json"]; ok && f.projection != nil {
		f.files["projection.json"] = mustJSON(t, f.projection)
	}
	for path, response := range f.responses {
		f.files[path] = mustJSON(t, response)
	}
	for i := range f.nodes {
		for j := range f.nodes[i].Files {
			m := &f.nodes[i].Files[j]
			if b, ok := f.files[m.Path]; ok {
				m.Bytes = uint64(len(b))
				m.SHA256 = sum(b)
			}
		}
	}
	root := t.TempDir()
	for path, b := range f.files {
		if e := os.WriteFile(filepath.Join(root, path), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	doc := mustJSON(t, map[string]any{"schema": EvidenceSchema, "nodes": f.nodes})
	if e := os.WriteFile(filepath.Join(root, "evidence.json"), doc, 0600); e != nil {
		t.Fatal(e)
	}
	policy := mustJSON(t, map[string]any{"schema": EvidencePolicySchema, "id": "connection-policy", "evidence_sha256": sum(doc), "required": []any{}, "carry_forward": []any{}, "eligibility": nil})
	f.registration.EvidencePolicySHA256 = sum(policy)
	return QueryReferenceConnectionRequest{Evidence: EvidenceRequest{Root: root, Policy: policy}, Registration: mustJSON(t, f.registration), WitnessRegistration: mustJSON(t, f.witness)}
}

// Independent approval changes only in tests explicitly exercising semantic validation.
func (f *referenceFixture) approve(t *testing.T, path string) {
	t.Helper()
	if r, ok := f.responses[path]; ok {
		f.files[path] = mustJSON(t, r)
	}
	switch path {
	case "witness.json":
		f.files[path] = mustJSON(t, f.witness)
	case "raw.json":
		f.files[path] = mustJSON(t, f.raw)
	case "projection.json":
		f.files[path] = mustJSON(t, f.projection)
	}
	set := func(m *ReplayMember) {
		if m.Path == path {
			m.Bytes = uint64(len(f.files[path]))
			m.SHA256 = sum(f.files[path])
		}
	}
	for _, m := range []*ReplayMember{&f.registration.WitnessRegistration, &f.registration.Raw, &f.registration.Projection, &f.registration.Build, &f.registration.Source} {
		set(m)
	}
	for i := range f.registration.Observations {
		set(&f.registration.Observations[i].Response)
	}
}

func TestQueryReferenceConnection(t *testing.T) {
	f := newReferenceFixture(t)
	r, e := VerifyQueryReferenceConnection(ctxT(t), f.request(t))
	if e != nil || r.Assessment != AssessPass || r.Required != 3 || r.Verified != 3 || r.NativeWitness == nil || r.NativeWitness.Verified != 3 {
		t.Fatalf("connection: %+v %v", r, e)
	}
}

func TestQueryReferenceConnectionRequiredWitnessInput(t *testing.T) {
	for _, kind := range []string{"missing", "malformed", "different-pin", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			f := newReferenceFixture(t)
			req := f.request(t)
			wantKind, wantCode := KindInvalidInput, "REFERENCE_WITNESS_REQUIRED"
			switch kind {
			case "missing":
				req.WitnessRegistration = nil
			case "malformed":
				req.WitnessRegistration = []byte(`{"schema":`)
				wantCode = ""
			case "different-pin":
				req.WitnessRegistration = append(req.WitnessRegistration, ' ')
				wantCode = "REFERENCE_WITNESS_PIN_MISMATCH"
			case "oversized":
				req.WitnessRegistration = make([]byte, (16<<20)+1)
				wantKind, wantCode = KindResourceLimit, ""
			}
			r, e := VerifyQueryReferenceConnection(ctxT(t), req)
			if e == nil || asError(e).Kind != wantKind || wantCode != "" && asError(e).Code != wantCode || r.Assessment == AssessPass {
				t.Fatalf("required input: result=%+v error=%v", r, e)
			}
		})
	}
}

func TestQueryReferenceConnectionGraphWitnessMissing(t *testing.T) {
	f := newReferenceFixture(t)
	f.nodes[0].Files = f.nodes[0].Files[1:]
	delete(f.files, "witness.json")
	r, e := VerifyQueryReferenceConnection(ctxT(t), f.request(t))
	if e != nil || r.Assessment != AssessBlocked || r.Required != 3 || r.Verified != 0 || r.NativeWitness == nil || r.NativeWitness.Assessment != AssessPass || !slices.Contains(codes(r.Findings), "REFERENCE_MEMBER_MISSING") {
		t.Fatalf("caller bytes replaced missing graph member: %+v %v", r, e)
	}

	f = newReferenceFixture(t)
	req := f.request(t)
	if e := os.Remove(filepath.Join(req.Evidence.Root, "witness.json")); e != nil {
		t.Fatal(e)
	}
	r, e = VerifyQueryReferenceConnection(ctxT(t), req)
	if e != nil || r.Assessment != AssessFail || r.Required != 3 || r.Verified != 0 {
		t.Fatalf("declared witness missing: %+v %v", r, e)
	}
}

// Exact emitter bytes from the Windows build9 owned eq-0 control; this is stored
// protocol compatibility, not a new reference execution or host qualification.
func TestQueryReferenceStoredEmitterResponse(t *testing.T) {
	data, e := os.ReadFile(filepath.Join("..", "testdata", "query-reference", "eq-0.json"))
	if e != nil {
		t.Fatal(e)
	}
	if sum(data) != "2652af45685d90c8d59c9661cfc1f1b7f6e89ac72b095206425eb66fc5f504f5" {
		t.Fatal("stored emitter output changed")
	}
	r, e2 := startRun(ctxT(t), ReplayOperations()["evidence-replay"].Wall)
	if e2 != nil {
		t.Fatal(e2)
	}
	defer r.cancel()
	source := []byte("x;\n1;\n")
	reg := QueryReferenceRegistration{LibrarySHA256: "08dbe12d302ed24da98305d793a7ef2bf0037ce00e188e36d524625847eb02ca", Symbol: "tree_sitter_tsgk_plain"}
	obs := QueryReferenceObservation{Query: "q0", Step: 0, SourceBytes: 6, SourceSHA256: sum(source)}
	ref, code, complete, e2 := decodeReferenceResponse(data, obs, reg, QueryWitnessQuery{Source: `((identifier) @x (#eq? @x "x"))`}, true, [][]byte{source}, r)
	anchor := Capture{Match: 0, Pattern: 0, Capture: 0, Name: "x", Node: 2, Type: "identifier", Named: true, EndByte: 1, EndPoint: Point{Column: 1}}
	if e2 != nil || code != "" || !complete || ref == nil {
		t.Fatalf("stored reference: %s %v", code, e2)
	}
	if len(ref.Tree.Nodes) != 7 || ref.Tree.Nodes[0].EndPoint != (Point{Row: 2}) || CompareCaptures(ref.Query.Structural, []Capture{anchor}) != nil || CompareCaptures(ref.Query.Evaluated, []Capture{anchor}) != nil {
		t.Fatalf("stored reference anchors: %+v", ref)
	}
}

func TestQueryReferenceConnectionContradictions(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		mutate     func(*referenceFixture)
	}{
		{"member-rehash", "REFERENCE_MEMBER_MISMATCH", func(f *referenceFixture) { f.responses["reference-0.json"]["code"] = "changed" }},
		{"identity", "REFERENCE_IDENTITY_MISMATCH", func(f *referenceFixture) { f.nodes[1].Identities = ident("changed-policy", "reference-run") }},
		{"source-version", "REFERENCE_CONTEXT_MISMATCH", func(f *referenceFixture) {
			f.responses["reference-0.json"]["input"].(map[string]any)["sha256"] = sum([]byte("other"))
			f.approve(t, "reference-0.json")
		}},
		{"query-sha", "REFERENCE_CONTEXT_MISMATCH", func(f *referenceFixture) {
			f.responses["reference-0.json"]["query"].(map[string]any)["sha256"] = sum([]byte("other"))
			f.approve(t, "reference-0.json")
		}},
		{"library", "REFERENCE_CONTEXT_MISMATCH", func(f *referenceFixture) {
			f.responses["reference-0.json"]["grammar"].(map[string]any)["library_sha256"] = sum([]byte("other"))
			f.approve(t, "reference-0.json")
		}},
		{"completion", "REFERENCE_COMPLETION_INVALID", func(f *referenceFixture) {
			f.responses["reference-0.json"]["cursor_exhausted"] = false
			f.approve(t, "reference-0.json")
		}},
		{"complete-string", "REFERENCE_RESPONSE_INVALID", func(f *referenceFixture) {
			f.responses["reference-0.json"]["complete"] = "true"
			f.approve(t, "reference-0.json")
		}},
		{"null-nodes", "REFERENCE_TREE_INVALID", func(f *referenceFixture) {
			f.responses["reference-0.json"]["tree"].(map[string]any)["nodes"] = nil
			f.approve(t, "reference-0.json")
		}},
		{"tree-parent", "REFERENCE_TREE_INVALID", func(f *referenceFixture) {
			nodes := f.responses["reference-0.json"]["tree"].(map[string]any)["nodes"].([]TreeNode)
			nodes[1].Parent = -1
			f.approve(t, "reference-0.json")
		}},
		{"tree-point", "REFERENCE_TREE_POINTS", func(f *referenceFixture) {
			nodes := f.responses["reference-0.json"]["tree"].(map[string]any)["nodes"].([]TreeNode)
			nodes[0].EndPoint.Column = 2
			f.approve(t, "reference-0.json")
		}},
		{"capture-node", "REFERENCE_CAPTURE_INVALID", func(f *referenceFixture) {
			q := f.responses["reference-0.json"]["query"].(map[string]any)
			cs := q["structural"].([]Capture)
			cs[0].Node = 0
			f.approve(t, "reference-0.json")
		}},
		{"match-count", "REFERENCE_MATCH_ACCOUNTING", func(f *referenceFixture) {
			f.responses["reference-0.json"]["query"].(map[string]any)["matches"] = 0
			f.responses["reference-0.json"]["query"].(map[string]any)["match_mapping"] = []any{}
			f.approve(t, "reference-0.json")
		}},
		{"mapping", "REFERENCE_MATCH_MAPPING", func(f *referenceFixture) {
			f.responses["reference-0.json"]["query"].(map[string]any)["match_mapping"].([]any)[0].(map[string]any)["match"] = 1
			f.approve(t, "reference-0.json")
		}},
		{"evaluated-duplicate", "REFERENCE_EVALUATED_MAPPING", func(f *referenceFixture) {
			q := f.responses["reference-0.json"]["query"].(map[string]any)
			cs := q["evaluated"].([]Capture)
			q["evaluated"] = append(slices.Clone(cs), cs[0])
			f.approve(t, "reference-0.json")
		}},
		{"metadata", "REFERENCE_METADATA_MISMATCH", func(f *referenceFixture) {
			f.responses["reference-0.json"]["query"].(map[string]any)["patterns"] = 2
			f.approve(t, "reference-0.json")
		}},
		{"cst-ineligible-does-not-mask-metadata", "REFERENCE_METADATA_MISMATCH", func(f *referenceFixture) {
			f.responses["reference-0.json"]["tree"].(map[string]any)["nodes"].([]TreeNode)[0].Type = "other_root"
			f.responses["reference-0.json"]["query"].(map[string]any)["patterns"] = 2
			f.approve(t, "reference-0.json")
		}},
		{"failed-with-hidden-context", "REFERENCE_RESPONSE_INVALID", func(f *referenceFixture) {
			f.responses["reference-0.json"] = map[string]any{"schema": instrumentedReferenceSchema, "status": StatusFailed, "code": "NODE_LIMIT", "complete": false, "query": map[string]any{"sha256": sum([]byte("wrong"))}}
			f.approve(t, "reference-0.json")
		}},
		{"source-version-reuse", "REFERENCE_CONTEXT_MISMATCH", func(f *referenceFixture) {
			f.responses["reference-1.json"] = f.responses["reference-0.json"]
			f.approve(t, "reference-1.json")
		}},
		{"predicate-decision", "REFERENCE_EVALUATION_MISMATCH", func(f *referenceFixture) {
			f.responses["reference-0.json"]["query"].(map[string]any)["evaluated"] = []Capture{}
			f.approve(t, "reference-0.json")
		}},
		{"build-binary", "REFERENCE_BUILD_MISMATCH", func(f *referenceFixture) {
			f.files["build.json"] = mustJSON(t, map[string]any{"schema": instrumentedReferenceBuildSchema, "upstream_commit": instrumentedReferenceCommit, "target": "x86_64-pc-windows-gnu", "binary_sha256": sum([]byte("other")), "complete": true})
			f.approve(t, "build.json")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReferenceFixture(t)
			tc.mutate(f)
			r, e := VerifyQueryReferenceConnection(ctxT(t), f.request(t))
			if e != nil || r.Assessment != AssessFail || !slices.Contains(codes(r.Findings), tc.code) {
				t.Fatalf("%s: %+v %v", tc.code, r, e)
			}
		})
	}
}

func TestQueryReferenceConnectionEveryIdentityRole(t *testing.T) {
	for _, role := range EvidenceRoles {
		t.Run(role, func(t *testing.T) {
			f := newReferenceFixture(t)
			ids := map[string]string{}
			for k, v := range f.nodes[1].Identities {
				ids[k] = v
			}
			ids[role] += "changed"
			f.nodes[1].Identities = ids
			r, e := VerifyQueryReferenceConnection(ctxT(t), f.request(t))
			if e != nil || r.Assessment != AssessFail || !slices.Contains(codes(r.Findings), "REFERENCE_IDENTITY_MISMATCH") {
				t.Fatalf("%s: %+v %v", role, r, e)
			}
		})
	}
}

func TestQueryReferenceConnectionDeclaredMissingIsFail(t *testing.T) {
	f := newReferenceFixture(t)
	req := f.request(t)
	if e := os.Remove(filepath.Join(req.Evidence.Root, "reference-1.json")); e != nil {
		t.Fatal(e)
	}
	r, e := VerifyQueryReferenceConnection(ctxT(t), req)
	if e != nil || r.Assessment != AssessFail {
		t.Fatalf("declared missing: %+v %v", r, e)
	}
}

func TestQueryReferenceConnectionMissingAndIneligible(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		mutate     func(*referenceFixture)
	}{
		{"undeclared-response", "REFERENCE_MEMBER_MISSING", func(f *referenceFixture) {
			f.nodes[1].Files = f.nodes[1].Files[:2]
			delete(f.files, "reference-1.json")
			delete(f.responses, "reference-1.json")
		}},
		{"source-missing", "REFERENCE_NATIVE_WITNESS_BLOCKED", func(f *referenceFixture) {
			f.nodes[0].Files = append(f.nodes[0].Files[:1], f.nodes[0].Files[2:]...)
			delete(f.files, "source.txt")
		}},
		{"producer-failed", "REFERENCE_RESPONSE_INCOMPLETE", func(f *referenceFixture) {
			f.responses["reference-0.json"] = map[string]any{"schema": instrumentedReferenceSchema, "status": StatusFailed, "code": "NODE_LIMIT", "complete": false}
			f.approve(t, "reference-0.json")
		}},
		{"uncaptured-cst", "REFERENCE_CST_INELIGIBLE", func(f *referenceFixture) {
			f.responses["reference-0.json"]["tree"].(map[string]any)["nodes"].([]TreeNode)[0].Type = "other_root"
			f.approve(t, "reference-0.json")
		}},
		{"no-observations", "REFERENCE_TUPLE_MISSING", func(f *referenceFixture) { f.registration.Observations = []QueryReferenceObservation{} }},
		{"zero-denominator", "REFERENCE_REQUIRED_ZERO", func(f *referenceFixture) { f.witness["required"] = []any{}; f.approve(t, "witness.json") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReferenceFixture(t)
			tc.mutate(f)
			r, e := VerifyQueryReferenceConnection(ctxT(t), f.request(t))
			if e != nil || r.Assessment != AssessBlocked || !slices.Contains(codes(r.Findings), tc.code) {
				t.Fatalf("%s: %+v %v", tc.code, r, e)
			}
		})
	}
}

func TestQueryReferenceConnectionMissingDoesNotMaskContradiction(t *testing.T) {
	f := newReferenceFixture(t)
	f.nodes[1].Files = f.nodes[1].Files[:2]
	delete(f.files, "reference-1.json")
	delete(f.responses, "reference-1.json")
	f.responses["reference-0.json"]["query"].(map[string]any)["sha256"] = sum([]byte("wrong"))
	f.approve(t, "reference-0.json")
	r, e := VerifyQueryReferenceConnection(ctxT(t), f.request(t))
	if e != nil || r.Assessment != AssessFail || !slices.Contains(codes(r.Findings), "REFERENCE_CONTEXT_MISMATCH") {
		t.Fatalf("missing masked mismatch: %+v %v", r, e)
	}
}

func TestQueryReferenceConnectionContextAndCancellation(t *testing.T) {
	f := newReferenceFixture(t)
	req := f.request(t)
	if _, e := VerifyQueryReferenceConnection(context.Background(), req); e == nil || asError(e).Code != "DEADLINE_REQUIRED" {
		t.Fatalf("deadline: %v", e)
	}
	ctx, cancel := context.WithCancel(ctxT(t))
	cancel()
	if _, e := VerifyQueryReferenceConnection(ctx, req); e == nil || asError(e).Code != "CANCELLED" {
		t.Fatalf("cancel: %v", e)
	}
	f.registration.Reference.Identities = map[string]string{"platform": "windows/amd64"}
	req.Registration = mustJSON(t, f.registration)
	if _, e := VerifyQueryReferenceConnection(ctxT(t), req); e == nil || asError(e).Code != "REFERENCE_REGISTRATION_INVALID" {
		t.Fatalf("identity registration: %v", e)
	}
	f = newReferenceFixture(t)
	f.registration.Symbol = "not_a_language_symbol"
	if _, e := VerifyQueryReferenceConnection(ctxT(t), f.request(t)); e == nil || asError(e).Code != "REFERENCE_REGISTRATION_INVALID" {
		t.Fatalf("symbol registration: %v", e)
	}
}

func TestQueryReferenceConnectionWallDuringEvidence(t *testing.T) {
	f := newReferenceFixture(t)
	req := f.request(t)
	wall, expire := context.WithCancel(context.Background())
	defer expire()
	calls := 0
	testHookWall = func() (context.Context, context.CancelFunc) {
		calls++
		if calls == 1 {
			return wall, func() {}
		}
		return context.WithCancel(context.Background())
	}
	opened := false
	testHookOpen = func(path string) {
		if path == "evidence.json" {
			opened = true
			expire()
		}
	}
	defer func() { testHookWall = nil; testHookOpen = nil }()
	r, e := VerifyQueryReferenceConnection(ctxT(t), req)
	if !opened || e == nil || asError(e).Code != "WALL_LIMIT" || r.ExecutionStatus != StatusResourceLimit || r.Assessment == AssessPass {
		t.Fatalf("outer wall: opened=%v result=%+v error=%v", opened, r, e)
	}
}

func TestQueryReferenceConnectionPartialEvidenceContradictions(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		mutate     func(*referenceFixture)
	}{
		{"expected-missing-structural-wrong", "REFERENCE_CAPTURE_MISMATCH", func(f *referenceFixture) {
			f.witness["required"].([]any)[0].(map[string]any)["expected"] = nil
			f.approve(t, "witness.json")
			q := f.responses["reference-0.json"]["query"].(map[string]any)
			q["structural"], q["evaluated"], q["matches"], q["match_mapping"] = []Capture{}, []Capture{}, 0, []any{}
			f.approve(t, "reference-0.json")
		}},
		{"no-predicates-reject-capture", "REFERENCE_EVALUATION_MISMATCH", func(f *referenceFixture) {
			f.witness["queries"].([]any)[0].(map[string]any)["metadata"] = nil
			f.witness["required"].([]any)[0].(map[string]any)["expected"] = nil
			f.approve(t, "witness.json")
			q := f.responses["reference-0.json"]["query"].(map[string]any)
			q["predicates"], q["evaluated"] = []QueryPredicate{}, []Capture{}
			f.approve(t, "reference-0.json")
		}},
		{"source-missing-edit-size", "REFERENCE_SOURCE_VERSION_MISMATCH", func(f *referenceFixture) {
			f.nodes[0].Files = append(f.nodes[0].Files[:1], f.nodes[0].Files[2:]...)
			delete(f.files, "source.txt")
			f.registration.Observations[1].SourceBytes = 2
			f.responses["reference-1.json"]["input"].(map[string]any)["bytes"] = 2
			f.approve(t, "reference-1.json")
		}},
		{"metadata-missing-predicate-pattern", "REFERENCE_METADATA_INVALID", func(f *referenceFixture) {
			f.witness["queries"].([]any)[0].(map[string]any)["metadata"] = nil
			f.approve(t, "witness.json")
			q := f.responses["reference-0.json"]["query"].(map[string]any)
			q["predicates"] = []QueryPredicate{{Pattern: 1, Steps: []QueryPredicateStep{{Kind: "string", Value: "eq?"}}}}
			q["structural"], q["evaluated"], q["matches"], q["match_mapping"] = []Capture{}, []Capture{}, 0, []any{}
			f.approve(t, "reference-0.json")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReferenceFixture(t)
			tc.mutate(f)
			r, e := VerifyQueryReferenceConnection(ctxT(t), f.request(t))
			if e != nil || r.Assessment != AssessFail || !slices.Contains(codes(r.Findings), tc.code) {
				t.Fatalf("%s: %+v %v", tc.code, r, e)
			}
		})
	}
}

func TestQueryReferenceConnectionSecondReviewContradictions(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		mutate     func(*referenceFixture)
	}{
		{"raw-missing-structural-wrong", "REFERENCE_CAPTURE_MISMATCH", func(f *referenceFixture) {
			f.nodes[0].Files = append(f.nodes[0].Files[:2], f.nodes[0].Files[3:]...)
			delete(f.files, "raw.json")
			f.witness["required"].([]any)[0].(map[string]any)["expected"] = nil
			f.approve(t, "witness.json")
			q := f.responses["reference-0.json"]["query"].(map[string]any)
			q["structural"], q["evaluated"], q["matches"], q["match_mapping"] = []Capture{}, []Capture{}, 0, []any{}
			f.approve(t, "reference-0.json")
		}},
		{"response-missing-observation-size", "REFERENCE_SOURCE_VERSION_MISMATCH", func(f *referenceFixture) {
			f.registration.Observations[1].SourceBytes = 2
			f.nodes[1].Files = f.nodes[1].Files[:2]
			delete(f.files, "reference-1.json")
			delete(f.responses, "reference-1.json")
		}},
		{"member-wrong-owner", "REFERENCE_MEMBER_OWNER_MISMATCH", func(f *referenceFixture) {
			f.nodes[0].Files = append(f.nodes[0].Files, f.nodes[1].Files[0])
			f.nodes[1].Files = f.nodes[1].Files[1:]
		}},
		{"source-missing-unpredicated-pattern", "REFERENCE_EVALUATION_MISMATCH", func(f *referenceFixture) {
			f.nodes[0].Files = append(f.nodes[0].Files[:1], f.nodes[0].Files[2:]...)
			delete(f.files, "source.txt")
			f.witness["queries"].([]any)[0].(map[string]any)["metadata"] = nil
			f.approve(t, "witness.json")
			q := f.responses["reference-0.json"]["query"].(map[string]any)
			q["patterns"], q["evaluated"] = 2, []Capture{}
			one := "ONE"
			q["predicates"] = []QueryPredicate{{Pattern: 1, Steps: []QueryPredicateStep{{Kind: "string", Value: "eq?"}, {Kind: "capture", Value: "x", Quantifier: &one}, {Kind: "string", Value: "a"}}}}
			f.approve(t, "reference-0.json")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReferenceFixture(t)
			tc.mutate(f)
			r, e := VerifyQueryReferenceConnection(ctxT(t), f.request(t))
			if e != nil || r.Assessment != AssessFail || !slices.Contains(codes(r.Findings), tc.code) {
				t.Fatalf("%s: %+v %v", tc.code, r, e)
			}
		})
	}
}

func TestQueryReferenceConnectionThirdReviewContradictions(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		mutate     func(*referenceFixture)
	}{
		{"witness-missing-malformed-projection", "REFERENCE_NATIVE_PROJECTION_INVALID", func(f *referenceFixture) {
			f.nodes[0].Files = f.nodes[0].Files[1:]
			delete(f.files, "witness.json")
			f.projection = nil
			f.files["projection.json"] = []byte("{")
			f.registration.Projection.Bytes = 1
			f.registration.Projection.SHA256 = sum([]byte("{"))
		}},
		{"source-missing-declared-input", "REFERENCE_SOURCE_MEMBER_MISMATCH", func(f *referenceFixture) {
			f.nodes[0].Files = append(f.nodes[0].Files[:1], f.nodes[0].Files[2:]...)
			delete(f.files, "source.txt")
			f.registration.Source.Bytes = 2
			f.registration.Source.SHA256 = sum([]byte("ab"))
		}},
		{"witness-missing-cross-platform", "REFERENCE_PLATFORM_MISMATCH", func(f *referenceFixture) {
			f.nodes[0].Files = f.nodes[0].Files[1:]
			delete(f.files, "witness.json")
			ids := map[string]string{}
			for k, v := range f.registration.Reference.Identities {
				ids[k] = v
			}
			ids["platform"] = "linux/amd64"
			f.registration.Reference.Identities = ids
			f.nodes[1].Identities = ids
			f.files["build.json"] = mustJSON(t, map[string]any{"schema": instrumentedReferenceBuildSchema, "upstream_commit": instrumentedReferenceCommit, "target": "x86_64-unknown-linux-gnu", "binary_sha256": f.registration.BinarySHA256, "complete": true})
			f.approve(t, "build.json")
		}},
		{"witness-missing-initial-version", "REFERENCE_SOURCE_VERSION_MISMATCH", func(f *referenceFixture) {
			f.nodes[0].Files = f.nodes[0].Files[1:]
			delete(f.files, "witness.json")
			f.registration.Observations[0].SourceBytes = 2
			f.registration.Observations[0].SourceSHA256 = sum([]byte("bb"))
			input := f.responses["reference-0.json"]["input"].(map[string]any)
			input["bytes"], input["sha256"] = 2, sum([]byte("bb"))
			f.approve(t, "reference-0.json")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReferenceFixture(t)
			tc.mutate(f)
			req := f.request(t)
			r, e := VerifyQueryReferenceConnection(ctxT(t), req)
			if e != nil || r.Assessment != AssessFail || !slices.Contains(codes(r.Findings), tc.code) {
				t.Fatalf("%s: %+v %v", tc.code, r, e)
			}
		})
	}
}

func TestQueryReferenceConnectionMissingSourceVersionPair(t *testing.T) {
	for _, withoutRaw := range []bool{false, true} {
		t.Run(map[bool]string{false: "raw", true: "projection"}[withoutRaw], func(t *testing.T) {
			f := newReferenceFixture(t)
			members := []ReplayMember{}
			for _, m := range f.nodes[0].Files {
				if m.Path == "source.txt" || withoutRaw && m.Path == "raw.json" {
					delete(f.files, m.Path)
				} else {
					members = append(members, m)
				}
			}
			f.nodes[0].Files = members
			f.registration.Observations[1].SourceSHA256 = sum([]byte("c"))
			f.responses["reference-1.json"]["input"].(map[string]any)["sha256"] = sum([]byte("c"))
			f.approve(t, "reference-1.json")
			r, e := VerifyQueryReferenceConnection(ctxT(t), f.request(t))
			if e != nil || r.Assessment != AssessFail || !slices.Contains(codes(r.Findings), "REFERENCE_SOURCE_VERSION_MISMATCH") {
				t.Fatalf("version pair: %+v %v", r, e)
			}
		})
	}
}

func TestQueryReferenceConnectionSourceWithoutWitnessRegistration(t *testing.T) {
	f := newReferenceFixture(t)
	f.nodes[0].Files = f.nodes[0].Files[1:]
	delete(f.files, "witness.json")
	f.files["source.txt"] = []byte("\n")
	f.approve(t, "source.txt")
	sha := sum([]byte("\n"))
	f.raw["steps"].([]any)[0].(map[string]any)["source_sha256"] = sha
	f.approve(t, "raw.json")
	in := f.projection["input"].(NativeInput)
	in.SHA256 = sha
	f.projection["input"] = in
	f.projection["steps"].([]any)[0].(map[string]any)["source_sha256"] = sha
	f.approve(t, "projection.json")
	f.registration.Observations[0].SourceSHA256 = sha
	f.responses["reference-0.json"]["input"].(map[string]any)["sha256"] = sha
	f.approve(t, "reference-0.json")
	r, e := VerifyQueryReferenceConnection(ctxT(t), f.request(t))
	if e != nil || r.Assessment != AssessFail || !slices.Contains(codes(r.Findings), "REFERENCE_TREE_POINTS") {
		t.Fatalf("known initial source: %+v %v", r, e)
	}
}
