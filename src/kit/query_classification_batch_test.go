package kit

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

func TestQueryClassificationDefinitionExpired(t *testing.T) {
	for _, phase := range []string{"before", "during"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if phase == "before" {
				cancel()
			}
			called := false
			var result []byte
			output := filepath.Join(t.TempDir(), "definition-classification-result.json")
			err := classificationGoDefinitionPublish(t, output, func() ([]byte, error) {
				err := classificationGoDefinitionDuring(ctx, func() {
					called = true
					result = []byte(`{"class":"STRUCTURAL_ONLY"}`)
					cancel()
				})
				return result, err
			})
			if err != context.Canceled || called != (phase == "during") {
				t.Fatal("expired projection was not rejected", phase, called, err)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatal("expired projection published an output", err)
			}
		})
	}
}

func TestQueryClassificationDefinitionOutput(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, assembly, output := filepath.Join(base, "retained-case"), filepath.Join(base, "retained-snapshot"), filepath.Join(base, "new-output")
	for _, path := range []string{root, assembly, output, filepath.Join(root, "child"), filepath.Join(assembly, "child")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := classificationGoDefinitionOutput(root, assembly, output); err != nil {
		t.Fatal("separate physical output rejected", err)
	}
	for _, path := range []string{"relative", filepath.Join(root, "child"), filepath.Join(assembly, "child"), filepath.Join(base, "absent")} {
		if err := classificationGoDefinitionOutput(root, assembly, path); err == nil {
			t.Fatal("unsafe output accepted", path)
		}
	}
	retained := filepath.Join(output, "keep.json")
	if err := os.WriteFile(retained, []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := classificationGoDefinitionOutput(root, assembly, output); err == nil {
		t.Fatal("occupied output accepted")
	}
	data, err := os.ReadFile(retained)
	if err != nil || string(data) != "retained" {
		t.Fatal("prior result changed", err)
	}
}

func TestQueryClassificationPositiveBatchRegistration(t *testing.T) {
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", "positive-batch")
	a, _ := classificationGoAnchors(t)
	w := classificationGoRegistration(a)
	if w.Case != a.Case || w.Input.Bytes != 50 || len(w.Required[0].Expected) != 1 || len(w.Required[1].Expected) != 0 || len(w.Required[2].Expected) != 0 {
		t.Fatal("positive source or capture denominator")
	}
	if _, err := validateQueryWitnessRegistration(w, queryEligibilityWitnessSchema); err != nil {
		t.Fatal(err)
	}
	var routes struct {
		Cases []struct {
			ID         string `json:"id"`
			SourceUTF8 string `json:"source_utf8"`
			Edits      []any  `json:"edits"`
			Expect     []struct {
				Step     int      `json:"step"`
				Syntax   string   `json:"syntax"`
				Contains []string `json:"contains"`
				Anchors  []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"anchors"`
			} `json:"expect"`
		} `json:"cases"`
	}
	actualJSONDecode(t, "../testdata/native/routes/go.json", &routes)
	found := 0
	for _, c := range routes.Cases {
		if c.ID != a.Case {
			continue
		}
		found++
		if c.SourceUTF8 != a.Source.UTF8 || len(c.Edits) != 0 || len(c.Expect) != 1 || c.Expect[0].Step != a.Expect[0].Step || c.Expect[0].Syntax != a.Expect[0].Syntax || !reflect.DeepEqual(c.Expect[0].Contains, a.Expect[0].Contains) || len(c.Expect[0].Anchors) != 1 || len(a.Expect[0].Anchors) != 1 {
			t.Fatal("registered source/edit/syntax assertions changed")
		}
		span := a.Expect[0].Anchors[0]
		if c.Expect[0].Anchors[0].Type != span.Type || string([]byte(c.SourceUTF8)[span.StartByte:span.EndByte]) != c.Expect[0].Anchors[0].Text || strings.Count(c.SourceUTF8, c.Expect[0].Anchors[0].Text) != 1 {
			t.Fatal("registered for span changed")
		}
	}
	if found != 1 {
		t.Fatal("registered case missing or duplicated")
	}
	var queries struct {
		Queries []OracleQuery `json:"queries"`
	}
	actualJSONDecode(t, "../testdata/native/queries/go.json", &queries)
	if len(queries.Queries) != len(a.Queries) {
		t.Fatal("registered query denominator")
	}
	for i, q := range queries.Queries {
		if q.ID != a.Queries[i].ID || q.Source != a.Queries[i].Source || digestHex([]byte(q.Source)) != a.Queries[i].SHA256 {
			t.Fatal("registered query changed", q.ID)
		}
	}
}

func classificationBatchRequest(t *testing.T) (classificationAnchors, classificationPlan, string, semanticNativeRequest) {
	t.Helper()
	a, _ := classificationGoAnchors(t)
	plan := classificationPlan{RunID: "positive-preregistered"}
	producer := strings.Repeat("a", 64)
	r := semanticNativeRequest{Protocol: "tsgk-native/r4", ID: a.Case, Encoding: EncodingUTF8, Output: OutputTree, Limits: semanticLimitsFor(NativeOperations()[a.Operation]), API: true, Source: base64.StdEncoding.EncodeToString([]byte(a.Source.UTF8)), Edits: []semanticRequestEdit{}, Queries: []semanticRequestQuery{}, Points: []NativePoint{}, Declarations: []NativeDeclaration{}, Ranges: [][]semanticRequestRange{}, ReuseObservation: reuseobs.Policy, ObservationRunID: plan.RunID, ObservationProducerSHA256: producer}
	for _, q := range a.Queries {
		r.Queries = append(r.Queries, semanticRequestQuery{ID: q.ID, Source: base64.StdEncoding.EncodeToString([]byte(q.Source))})
	}
	return a, plan, producer, r
}

func classificationBatchFrame(t *testing.T, r semanticNativeRequest) []byte {
	data := eligibilityJSON(t, r)
	out := make([]byte, len(data)+4)
	binary.BigEndian.PutUint32(out, uint32(len(data)))
	copy(out[4:], data)
	return out
}

func TestQueryClassificationBatchFrameBindings(t *testing.T) {
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", "positive-batch")
	a, plan, producer, r := classificationBatchRequest(t)
	frame := func(r semanticNativeRequest) []byte { return classificationBatchFrame(t, r) }
	accept := func(r semanticNativeRequest, args []string) bool {
		parsed, err := semanticTestBudget(t).nativeRequestFrame("batch-request", frame(r))
		return err == nil && classificationGoRequestMatches(parsed, args, a, plan, producer)
	}
	if !accept(r, []string{"batch"}) {
		t.Fatal("closed positive batch request rejected")
	}
	for _, change := range []func(*semanticNativeRequest){
		func(r *semanticNativeRequest) { r.ID = "go3" },
		func(r *semanticNativeRequest) { r.API = false },
		func(r *semanticNativeRequest) {
			r.Source = base64.StdEncoding.EncodeToString([]byte(strings.Replace(a.Source.UTF8, "break", "defer", 1)))
		},
		func(r *semanticNativeRequest) { r.ObservationRunID = "another-run" },
		func(r *semanticNativeRequest) { r.ObservationProducerSHA256 = strings.Repeat("b", 64) },
		func(r *semanticNativeRequest) { r.ObservationHistoryStart = 1 },
		func(r *semanticNativeRequest) {
			r.Queries = append([]semanticRequestQuery{}, r.Queries...)
			r.Queries[0].Source = base64.StdEncoding.EncodeToString([]byte("(source_file) @x"))
		},
		func(r *semanticNativeRequest) { r.Queries = r.Queries[:2] },
	} {
		changed := r
		change(&changed)
		if accept(changed, []string{"batch"}) {
			t.Fatal("changed request accepted")
		}
	}
	for _, args := range [][]string{nil, {"single"}, {"batch", "extra"}} {
		if accept(r, args) {
			t.Fatal("different process mode accepted")
		}
	}
	valid := frame(r)
	for _, extra := range [][]byte{append(append([]byte{}, valid...), valid...), append(append([]byte{}, valid...), 0), valid[:len(valid)-1]} {
		if _, err := semanticTestBudget(t).nativeRequestFrame("extra-frame", extra); err == nil {
			t.Fatal("extra/truncated input accepted")
		}
		if _, err := semanticTestBudget(t).semanticFrame("extra-output", extra); err == nil {
			t.Fatal("extra/truncated output accepted")
		}
	}
}

func TestQueryClassificationBatchGuardedCapture(t *testing.T) {
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", "positive-batch")
	for _, mismatch := range []string{"", "stdout", "process", "witness-api", "witness-node-limit", "witness-points", "api-child-count", "api-row-order", "api-missing", "memory", "wall", "grace", "stdout-limit", "stderr-limit", "env", "interactive", "descriptor-owner", "node-policy", "node-workload", "node-tool", "node-input", "node-query", "node-source", "node-comparator", "node-protocol", "node-platform", "node-run", "node-attempt", "selected-same-count", "selected-other-count", "invocation-id", "invocation-node", "invocation-kind"} {
		t.Run(mismatch, func(t *testing.T) {
			a, plan, producer, request := classificationBatchRequest(t)
			b := semanticTestBudget(t)
			s := &eligibilitySession{r: b.r, b: b, res: &QueryReferenceEligibilityResult{}, owned: map[string]eligibilityMember{}, owners: map[string]string{}, snapshot: &evidenceSnapshot{files: map[string][]byte{}, verifiedMembers: map[string]ReplayMember{}}, invocations: map[string]*eligibilityInvocation{}}
			id := "native-" + a.Case
			s.reg.Native = &eligibilityNative{InvocationID: id}
			add := func(path, role string, data []byte) eligibilityMember {
				pin := eligibilityMember{NodeID: id, Member: ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)}}
				s.owned[path], s.owners[path] = pin, id
				s.snapshot.files[path], s.snapshot.verifiedMembers[path] = data, pin.Member
				return pin
			}
			captured := func(name, role string, data []byte) *ReplayMember {
				pin := add("current/native/capture/"+name, role, data).Member
				pin.Path = "capture/" + name
				return &pin
			}
			raw, receipt := []byte(`{"fixture":"raw"}`), []byte(`{"fixture":"process"}`)
			stdout := make([]byte, len(raw)+4)
			binary.BigEndian.PutUint32(stdout, uint32(len(raw)))
			copy(stdout[4:], raw)
			op := NativeOperations()[a.Operation]
			capture := semanticProcessCapture{Schema: "tsgk-native-process-capture/r1", Stage: "RUN_RETURNED", Spec: &semanticCaptureSpec{Path: "C:/driver.exe", Args: []string{"batch"}, Dir: "C:/work", Env: []string{}, Wall: int64(op.ParseWall), Grace: 5e9, StdoutBytes: int64(op.OutputBytes + 4), StderrBytes: 65536, Stdin: captured("stdin.bin", "stdin-frame", classificationBatchFrame(t, request))}, Process: captured("process.json", "process", receipt), Stdout: captured("stdout.bin", "stdout-frame", stdout)}
			capture.Spec.Memory.Bytes, capture.Spec.Memory.Hard = op.MemoryBytes, true
			switch mismatch {
			case "memory":
				capture.Spec.Memory.Bytes *= 2
			case "wall":
				capture.Spec.Wall++
			case "grace":
				capture.Spec.Grace++
			case "stdout-limit":
				capture.Spec.StdoutBytes++
			case "stderr-limit":
				capture.Spec.StderrBytes++
			case "env":
				capture.Spec.Env = []string{"OTHER=1"}
			case "interactive":
				capture.Spec.Interactive = true
			}
			s.witness.Native.Process, _ = classificationGoProcessSpec(*capture.Spec)
			s.witness.Native.Protocol, s.witness.Native.RequestID = "tsgk-native/r4", a.Case
			s.witness.Native.NodeLimit, s.witness.Native.API = op.Nodes, true
			s.witness.Native.ExecutableSHA256 = strings.Repeat("e", 64)
			s.semantic = &semanticNativeCache{checked: []nativewire.CheckedStep{{Incremental: nativewire.Tree{API: a.expectedAPI}}}}
			switch mismatch {
			case "witness-api":
				s.witness.Native.API = false
			case "witness-node-limit":
				s.witness.Native.NodeLimit++
			case "witness-points":
				s.witness.Native.Points = []uint32{0}
			case "api-child-count", "api-row-order", "api-missing":
				api := *a.expectedAPI
				api.Nodes = slices.Clone(api.Nodes)
				api.Nodes[0] = slices.Clone(api.Nodes[0])
				switch mismatch {
				case "api-child-count":
					api.Nodes[0][7] = 0
				case "api-row-order":
					api.Nodes[0], api.Nodes[1] = api.Nodes[1], api.Nodes[0]
				case "api-missing":
					api.Nodes = api.Nodes[:len(api.Nodes)-1]
				}
				s.semantic.checked[0].Incremental.API = &api
			}
			queries := []OracleQuery{}
			for _, q := range a.Queries {
				queries = append(queries, OracleQuery{ID: q.ID, Source: q.Source})
			}
			assembly := &actualJSONAssembly{workload: a.Workload, cohortSource: map[string]string{"current": plan.SourceRevision}, nodes: map[string]EvidenceNode{}}
			node := assembly.node(id, "current", s.witness.Native.ExecutableSHA256, a.Source.SHA256, digestHex(eligibilityJSON(t, queries)), "tsgk-native/r4", id, StatusCompleted, AssessPass)
			node.Identities["policy"] = NativePolicy(op, OutputTree, nil).SHA256
			if strings.HasPrefix(mismatch, "node-") {
				node.Identities[strings.TrimPrefix(mismatch, "node-")] = "changed"
			}
			s.nodes = map[string]EvidenceNode{id: node}
			s.witness.Native.ProducerSHA256 = producer
			// Each changed member has consistent pins; only the cross-capture binding contradicts it.
			if mismatch == "stdout" {
				raw = []byte(`{"fixture":"other-raw"}`)
			}
			if mismatch == "process" {
				receipt = []byte(`{"fixture":"other-process"}`)
			}
			response := add("current/native/raw.json", "raw", raw)
			process := add("current/native/process.json", "process", receipt)
			s.invocations[id] = &eligibilityInvocation{ID: id, Kind: "NATIVE_DRIVER", Node: QueryReferenceNode{ID: id}, Response: &response, Process: &process}
			switch mismatch {
			case "selected-same-count", "selected-other-count":
				_, _, completePrimary := classificationBatchCompleteSelectionPair(t, s, a, request, producer, raw, &capture, mismatch)
				capture = completePrimary
			case "invocation-id":
				s.invocations[id].ID = "another-id"
			case "invocation-node":
				s.invocations[id].Node.ID = "another-node"
			case "invocation-kind":
				s.invocations[id].Kind = "REFERENCE"
			}
			descriptor := add("current/native/capture/"+a.Case+".json", "capture", eligibilityJSON(t, capture))
			if mismatch == "descriptor-owner" {
				descriptor.NodeID = "other-owner"
				s.owned[descriptor.Member.Path], s.owners[descriptor.Member.Path] = descriptor, "other-owner"
			}
			err := classificationGoFrames(s, a, plan)
			if strings.HasPrefix(mismatch, "selected-") || strings.HasPrefix(mismatch, "invocation-") {
				if err == nil || err.Error() != "guarded native invocation differs from profile" {
					t.Fatal("native selection mismatch did not fail at its invocation binding", mismatch, err)
				}
				return
			}
			if mismatch == "" {
				if err != nil {
					t.Fatal("matching guarded snapshot rejected", err)
				}
			} else if err == nil || (mismatch == "stdout" || mismatch == "process") && !strings.Contains(err.Error(), "guarded "+mismatch+" differs") {
				t.Fatal("rehashed cross-capture mismatch not rejected at its binding", mismatch, err)
			}
		})
	}
}
