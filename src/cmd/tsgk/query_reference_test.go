package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func queryReferenceCLISet(t *testing.T, state string) (kit.QueryReferenceConnectionRequest, []string) {
	t.Helper()
	marshal := func(v any) []byte {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	write := func(p string, b []byte) {
		if e := os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	hash := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
	data, e := os.ReadFile("../../testdata/query-witness.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		Registration kit.QueryWitnessRegistration `json:"registration"`
		Source       string                       `json:"source"`
		Raw          json.RawMessage              `json:"raw"`
		Projection   json.RawMessage              `json:"projection"`
	}
	if e := json.Unmarshal(data, &f); e != nil {
		t.Fatal(e)
	}
	var projection struct {
		Steps []struct {
			Incremental struct {
				Tree struct {
					Nodes []kit.TreeNode `json:"nodes"`
				} `json:"tree"`
			} `json:"incremental"`
		} `json:"steps"`
	}
	if e := json.Unmarshal(f.Projection, &projection); e != nil {
		t.Fatal(e)
	}
	versions, _, e := kit.ApplyEdits(kit.EncodingUTF8, []byte(f.Source), f.Registration.Edits, 1<<20)
	if e != nil {
		t.Fatal(e)
	}
	root, outside := t.TempDir(), t.TempDir()
	witness := marshal(f.Registration)
	files := map[string][]byte{"witness.json": witness, "source.txt": []byte(f.Source), "raw.json": f.Raw, "projection.json": f.Projection}
	member := func(p, role string) kit.ReplayMember {
		b := files[p]
		return kit.ReplayMember{Path: p, Role: role, Bytes: uint64(len(b)), SHA256: hash(b)}
	}
	ident := func(run string) map[string]string {
		m := map[string]string{}
		for _, role := range []string{"source", "tool", "input", "query", "policy", "comparator", "protocol", "workload"} {
			m[role] = "approved-" + role
		}
		m["platform"], m["run"], m["attempt"] = "windows/amd64", run, "1"
		return m
	}
	reg := kit.QueryReferenceRegistration{Schema: kit.QueryReferenceRegistrationSchema, Context: "full-input-utf8/r1", Native: kit.QueryReferenceNode{ID: "native", Identities: ident("native-run")}, Reference: kit.QueryReferenceNode{ID: "reference", Identities: ident("reference-run")}, BinarySHA256: hash([]byte("binary")), LibrarySHA256: hash([]byte("library")), Symbol: "tree_sitter_owned"}
	reg.WitnessRegistration, reg.Source, reg.Raw, reg.Projection = member("witness.json", "registration"), member("source.txt", "source"), member("raw.json", "raw"), member("projection.json", "projection")
	files["build.json"] = marshal(map[string]any{"schema": "tsgk-instrumented-reference-build/r1", "upstream_commit": "10eb5ac119eec9cb4267a8e035066473983f5f3a", "target": "x86_64-pc-windows-gnu", "binary_sha256": reg.BinarySHA256, "complete": true})
	reg.Build = member("build.json", "build")
	for step, src := range versions {
		q := f.Registration.Queries[0]
		var anchor kit.QueryWitnessTuple
		for _, a := range f.Registration.Required {
			if int(a.Step) == step && a.TreeRole == "incremental" {
				anchor = a
				break
			}
		}
		name := []string{"reference-0.json", "reference-1.json"}[step]
		files[name] = marshal(map[string]any{"schema": "tsgk-instrumented-query-reference/r1", "status": "COMPLETED", "code": "", "complete": true, "upstream_commit": "10eb5ac119eec9cb4267a8e035066473983f5f3a",
			"input": map[string]any{"encoding": "UTF-8", "bytes": len(src), "sha256": hash(src)}, "grammar": map[string]any{"library_sha256": reg.LibrarySHA256, "symbol": reg.Symbol}, "tree": map[string]any{"nodes": projection.Steps[step].Incremental.Tree.Nodes},
			"query":  map[string]any{"id": q.ID, "sha256": hash([]byte(q.Source)), "patterns": q.Metadata.Patterns, "capture_names": q.Metadata.CaptureNames, "predicates": q.Metadata.Predicates, "matches": 1, "match_mapping": []any{map[string]any{"match": 0, "runtime_id": 19}}, "structural": anchor.Structural, "evaluated": anchor.Expected},
			"limits": map[string]any{"nodes": 100000, "captures": 50000, "group_captures": 200000, "matches": 100000, "wall_ms": 3000}, "cursor_exhausted": true, "did_exceed_match_limit": false})
		reg.Observations = append(reg.Observations, kit.QueryReferenceObservation{Query: q.ID, Step: uint32(step), SourceBytes: uint64(len(src)), SourceSHA256: hash(src), Response: member(name, "reference-response")})
	}
	refFiles := []kit.ReplayMember{reg.Build}
	for _, o := range reg.Observations {
		refFiles = append(refFiles, o.Response)
	}
	if state == "blocked" || state == "fail-blocked" {
		refFiles = refFiles[:len(refFiles)-1]
		delete(files, "reference-1.json")
	}
	nodes := []kit.EvidenceNode{{ID: "native", Kind: "run", ExecutionStatus: kit.StatusCompleted, EvidenceMode: kit.ModeNewRun, Assessment: kit.AssessPass, Identities: reg.Native.Identities, Files: []kit.ReplayMember{reg.WitnessRegistration, reg.Source, reg.Raw, reg.Projection}, Refs: []kit.EvidenceRef{}}, {ID: "reference", Kind: "run", ExecutionStatus: kit.StatusCompleted, EvidenceMode: kit.ModeNewRun, Assessment: kit.AssessPass, Identities: reg.Reference.Identities, Files: refFiles, Refs: []kit.EvidenceRef{}}}
	if state == "graph-witness-missing" {
		nodes[0].Files = nodes[0].Files[1:]
		delete(files, "witness.json")
	}
	files["evidence.json"] = marshal(map[string]any{"schema": kit.EvidenceSchema, "nodes": nodes})
	policy := marshal(map[string]any{"schema": kit.EvidencePolicySchema, "id": "cli-reference", "evidence_sha256": hash(files["evidence.json"]), "required": []any{}, "carry_forward": []any{}, "eligibility": nil})
	reg.EvidencePolicySHA256 = hash(policy)
	if state == "fail" || state == "fail-blocked" {
		files["source.txt"] = []byte("tampered")
	}
	for p, b := range files {
		write(filepath.Join(root, p), b)
	}
	registered := marshal(reg)
	for name, b := range map[string][]byte{"policy.json": policy, "connection.json": registered, "witness.json": witness} {
		write(filepath.Join(outside, name), b)
	}
	return kit.QueryReferenceConnectionRequest{Evidence: kit.EvidenceRequest{Root: root, Policy: policy}, Registration: registered, WitnessRegistration: witness}, []string{"evidence", "query-reference", "--input", root, "--profile", filepath.Join(outside, "policy.json"), "--registration", filepath.Join(outside, "connection.json"), "--witness-registration", filepath.Join(outside, "witness.json")}
}

func TestQueryReferenceCLI(t *testing.T) {
	for _, state := range []string{"pass", "blocked", "fail", "graph-witness-missing", "fail-blocked"} {
		t.Run(state, func(t *testing.T) {
			req, args := queryReferenceCLISet(t, state)
			code, out, errout := cli(t, context.Background(), args...)
			wantCode := map[string]int{"pass": exitOK, "blocked": exitBlocked, "fail": exitFail, "graph-witness-missing": exitBlocked, "fail-blocked": exitFail}[state]
			if code != wantCode {
				t.Fatalf("exit %d want %d: %s %s", code, wantCode, out, errout)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			api, e := kit.VerifyQueryReferenceConnection(ctx, req)
			if e != nil {
				t.Fatal(e)
			}
			want, e := json.Marshal(api)
			if e != nil {
				t.Fatal(e)
			}
			if strings.TrimSuffix(out, "\n") != string(want) {
				t.Fatalf("CLI/API mismatch: %s %s", out, want)
			}
		})
	}
}

func TestQueryReferenceCLIBoundaries(t *testing.T) {
	req, args := queryReferenceCLISet(t, "pass")
	for _, index := range []int{5, 7, 9} {
		inside := append([]string(nil), args...)
		inside[index] = filepath.Join(req.Evidence.Root, "witness.json")
		if code, _, stderr := cli(t, context.Background(), inside...); code != exitUsage || !strings.Contains(stderr, "PROFILE_INSIDE_INPUT") {
			t.Fatalf("inside trust doc: %d %s", code, stderr)
		}
	}
	if code, _, _ := cli(t, context.Background(), args[:8]...); code != exitUsage {
		t.Fatalf("missing required witness flag: %d", code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code, _, stderr := cli(t, ctx, args...); code != exitCanceled {
		t.Fatalf("cancel %d: %s", code, stderr)
	}
	out := filepath.Join(t.TempDir(), "result.json")
	if e := os.WriteFile(out, []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	if code, _, stderr := cli(t, context.Background(), append(args, "--out", out)...); code != exitIO || !strings.Contains(stderr, "OUTPUT_EXISTS") {
		t.Fatalf("out collision: %d %s", code, stderr)
	}
	if b, e := os.ReadFile(out); e != nil || string(b) != "keep" {
		t.Fatal("existing out changed")
	}
	link := filepath.Join(t.TempDir(), "outside-link.json")
	if e := os.Symlink(filepath.Join(req.Evidence.Root, "witness.json"), link); e != nil {
		t.Logf("leaf symlink check unrun: %v", e)
		return
	}
	linked := append([]string(nil), args...)
	linked[9] = link
	if code, _, stderr := cli(t, context.Background(), linked...); code != exitUsage || !strings.Contains(stderr, "PROFILE_INSIDE_INPUT") {
		t.Fatalf("outside leaf alias: %d %s", code, stderr)
	}
	insideLink := filepath.Join(req.Evidence.Root, "inside-link.json")
	if e := os.Symlink(args[9], insideLink); e != nil {
		t.Fatal(e)
	}
	linked[9] = insideLink
	// The shared guard may reject this alias while checking the earlier policy path.
	if code, _, stderr := cli(t, context.Background(), linked...); code != exitUsage || !(strings.Contains(stderr, "PROFILE_INSIDE_INPUT") || strings.Contains(stderr, "INPUT_PATH_UNVERIFIED")) {
		t.Fatalf("inside alias to outside: %d %s", code, stderr)
	}
}

func TestQueryReferenceCLIInvalidDocuments(t *testing.T) {
	for _, kind := range []string{"malformed", "pin", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			req, args := queryReferenceCLISet(t, "pass")
			path, data, wantCode := args[7], []byte("{"), exitUsage
			if kind == "pin" {
				path, data = args[9], append(req.WitnessRegistration, ' ')
			}
			if kind == "oversized" {
				path, data, wantCode = args[9], make([]byte, kit.MaxDocumentBytes+2), exitBlocked
			}
			if e := os.WriteFile(path, data, 0600); e != nil {
				t.Fatal(e)
			}
			out := filepath.Join(t.TempDir(), "not-complete.json")
			code, report, stderr := cli(t, context.Background(), append(args, "--out", out)...)
			if code != wantCode || !json.Valid([]byte(report)) {
				t.Fatalf("invalid document: %d %s %s", code, report, stderr)
			}
			if _, e := os.Lstat(out); !os.IsNotExist(e) {
				t.Fatalf("failure published output: %v", e)
			}
		})
	}
}
