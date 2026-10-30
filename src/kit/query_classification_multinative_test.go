package kit

import (
	"encoding/binary"
	"strings"
	"testing"
)

// The fixture remains synthetic: it exercises stored consistency, not native execution or
// approval authenticity. Both current-driver invocations carry their own complete closure.
func classificationAddSecondEligibilityNative(f *eligibilityFixture, id string) {
	f.t.Helper()
	var current *eligibilityInvocation
	for i := range f.reg.Invocations {
		if f.reg.Invocations[i].ID == "native-current" {
			current = &f.reg.Invocations[i]
			break
		}
	}
	if current == nil || current.Request == nil || current.Response == nil || current.Process == nil || current.Executable == nil || f.reg.Native == nil || id == "" {
		f.t.Fatal("complete synthetic current native invocation required")
	}
	copyMember := func(pin *eligibilityMember, suffix string) eligibilityMember {
		data, ok := f.files[pin.Member.Path]
		if !ok {
			f.t.Fatalf("synthetic native member missing: %s", pin.Member.Path)
		}
		return f.member(id, id+"/"+suffix, pin.Member.Role, data)
	}
	node := f.node(id, strings.Repeat("a", 40))
	request := copyMember(current.Request, "request.json")
	response := copyMember(current.Response, "response.json")
	process := copyMember(current.Process, "process.json")
	inputs := make(map[string]eligibilityInput, len(current.Inputs))
	for key, input := range current.Inputs {
		inputs[key] = input
	}
	other := *current
	other.ID, other.CatalogueID = id, id
	other.Node = QueryReferenceNode{ID: id, Identities: node.Identities}
	other.Request, other.Response, other.Process = &request, &response, &process
	other.Inputs = inputs
	f.reg.Invocations = append(f.reg.Invocations, other)
	f.reg.Native.InvocationID = id
}

func TestQueryClassificationNativeSelectionAfterSyntheticCorePass(t *testing.T) {
	for _, selected := range []string{"native-current", "native-alternate"} {
		t.Run(selected, func(t *testing.T) {
			f := newEligibilityFixture(t)
			classificationAddSecondEligibilityNative(f, "native-alternate")
			f.reg.Native.InvocationID = selected
			req := f.request()
			r, e := startRun(ctxT(t), ReplayOperations()["evidence-replay"].Wall)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			cache := &semanticNativeCache{}
			scope := &eligibilityScope{r: r, native: cache}
			core, err := verifyQueryReferenceEligibilityInRun(r.caller, req, scope)
			if err != nil || core.Assessment != AssessPass || !core.Complete || core.Tuple != (QueryEligibilityCounts{Required: 3, Verified: 3}) || !cache.ready || scope.session == nil {
				t.Fatalf("synthetic two-driver core did not pass: err=%v assessment=%s complete=%v tuple=%+v findings=%v", err, core.Assessment, core.Complete, core.Tuple, core.Findings)
			}
			// The generic fixture is unrelated to a pinned Go profile; no validated session is mutated.
			invocation, err := classificationNativeInvocation(scope.session, classificationAnchors{Case: "current"})
			if selected == "native-current" {
				if err != nil || invocation.ID != selected {
					t.Fatalf("the registered primary invocation was rejected: invocation=%v err=%v", invocation, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "guarded native invocation differs") {
				t.Fatalf("the coherent alternate selection was accepted: %v", err)
			}
		})
	}
}

func classificationBatchAddMember(s *eligibilitySession, owner, path, role string, data []byte) eligibilityMember {
	pin := eligibilityMember{NodeID: owner, Member: ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)}}
	s.owned[path], s.owners[path] = pin, owner
	s.snapshot.files[path], s.snapshot.verifiedMembers[path] = data, pin.Member
	return pin
}

func classificationBatchCapturePin(s *eligibilitySession, owner, namespace, name, role string, data []byte) *ReplayMember {
	full := "current/native/capture/" + namespace + "/" + name
	pin := classificationBatchAddMember(s, owner, full, role, data).Member
	pin.Path = "capture/" + namespace + "/" + name
	return &pin
}

// classificationBatchCompleteSelectionPair upgrades the lightweight table fixture with
// self-consistent two-invocation process closures. The frame guard still rejects at selection.
func classificationBatchCompleteSelectionPair(t *testing.T, s *eligibilitySession, anchors classificationAnchors, request semanticNativeRequest, producer string, raw []byte, primary *semanticProcessCapture, mismatch string) (eligibilityExecution, eligibilityExecution, semanticProcessCapture) {
	t.Helper()
	primaryID, otherID := "native-"+anchors.Case, "native-other"
	for _, path := range []string{"current/native/raw.json", "current/native/process.json", "current/native/capture/stdin.bin", "current/native/capture/process.json", "current/native/capture/stdout.bin"} {
		delete(s.owned, path)
		delete(s.owners, path)
		delete(s.snapshot.files, path)
		delete(s.snapshot.verifiedMembers, path)
	}
	requestFrame := classificationBatchFrame(t, request)
	executable := []byte("synthetic native executable; never executed")
	executableSHA := digestHex(executable)
	s.witness.Native.ExecutableSHA256 = executableSHA
	s.witness.Native.ProducerSHA256 = producer

	install := func(owner, namespace string, stderr []byte, template semanticCaptureSpec) (*eligibilityInvocation, semanticProcessCapture, eligibilityExecution) {
		stdout := make([]byte, len(raw)+4)
		stdout[0], stdout[1], stdout[2], stdout[3] = byte(uint32(len(raw))>>24), byte(uint32(len(raw))>>16), byte(uint32(len(raw))>>8), byte(len(raw))
		copy(stdout[4:], raw)
		template.Stdin = classificationBatchCapturePin(s, owner, namespace, "stdin.bin", "stdin-frame", requestFrame)
		template.StdoutBytes = int64(NativeOperations()[anchors.Operation].OutputBytes + 4)
		processRequest, err := classificationGoProcessSpec(template)
		if err != nil {
			t.Fatal(err)
		}
		process := eligibilityFixtureExecution(processRequest, 0, false, stdout, stderr)
		process.Capabilities.OS, process.Capabilities.Backend = "windows/amd64", "windows-job-object"
		process.Cleanup.Method, process.Cleanup.Scope, process.Capabilities.TreeCleanup = "JOB_OBJECT", "JOB_OBJECT", "JOB_OBJECT"
		processData := eligibilityJSON(t, process)
		capture := semanticProcessCapture{Schema: "tsgk-native-process-capture/r1", Stage: "RUN_RETURNED", Spec: &template, Process: classificationBatchCapturePin(s, owner, namespace, "process.json", "process", processData), Stdout: classificationBatchCapturePin(s, owner, namespace, "stdout.bin", "stdout-frame", stdout), Stderr: classificationBatchCapturePin(s, owner, namespace, "stderr.bin", "stderr", stderr)}
		requestData := eligibilityJSON(t, s.witness.Native)
		requestPin := classificationBatchAddMember(s, owner, "current/native/selection-"+namespace+"/request.json", "approved-request", requestData)
		responsePin := classificationBatchAddMember(s, owner, "current/native/selection-"+namespace+"/raw.json", "raw", raw)
		processPin := classificationBatchAddMember(s, owner, "current/native/selection-"+namespace+"/process.json", "process", processData)
		executablePin := classificationBatchAddMember(s, owner, "current/native/selection-"+namespace+"/executable.bin", "native-executable", executable)
		node := s.nodes[primaryID]
		if owner != primaryID {
			node.ID = owner
			identities := make(map[string]string, len(node.Identities))
			for key, value := range node.Identities {
				identities[key] = value
			}
			node.Identities = identities
			node.Identities["run"] = owner
		}
		node.Identities["tool"] = executableSHA
		s.nodes[owner] = node
		invocation := &eligibilityInvocation{ID: owner, Kind: "NATIVE_DRIVER", CohortID: "current", CatalogueID: owner, Node: QueryReferenceNode{ID: owner, Identities: node.Identities}, Request: &requestPin, Response: &responsePin, Process: &processPin, ApprovedRequest: requestData, Executable: &executablePin, ExecutablePath: template.Path, Inputs: map[string]eligibilityInput{}}
		s.invocations[owner] = invocation
		if owner != primaryID {
			descriptorPath := "current/native/capture/" + namespace + ".json"
			classificationBatchAddMember(s, owner, descriptorPath, "capture", eligibilityJSON(t, capture))
		}
		return invocation, capture, process
	}

	primarySpec := *primary.Spec
	primaryInvocation, completePrimary, primaryProcess := install(primaryID, "primary", []byte{}, primarySpec)
	*primary = completePrimary
	s.semantic.record.process = primaryProcess
	otherStderr := []byte{}
	if mismatch == "selected-other-count" {
		otherStderr = []byte("e")
	}
	otherSpec := *primary.Spec
	otherInvocation, otherCapture, otherProcess := install(otherID, "selected-other", otherStderr, otherSpec)
	if mismatch == "selected-same-count" && otherProcess.StderrBytes != primaryProcess.StderrBytes || mismatch == "selected-other-count" && otherProcess.StderrBytes == primaryProcess.StderrBytes {
		t.Fatalf("selected process count counterexample did not retain its intended axis")
	}
	assertClosure := func(owner string, invocation *eligibilityInvocation, capture semanticProcessCapture, process eligibilityExecution, stderr []byte) {
		pinMatches := func(pin *eligibilityMember, role string, data []byte) bool {
			return pin != nil && pin.NodeID == owner && pin.Member.Role == role && pin.Member.Bytes == uint64(len(data)) && pin.Member.SHA256 == digestHex(data)
		}
		captureMatches := func(pin *ReplayMember, role string, data []byte) bool {
			return pin != nil && pin.Role == role && pin.Bytes == uint64(len(data)) && pin.SHA256 == digestHex(data)
		}
		processData := eligibilityJSON(t, process)
		stdout := make([]byte, len(raw)+4)
		binary.BigEndian.PutUint32(stdout, uint32(len(raw)))
		copy(stdout[4:], raw)
		if invocation.ID != owner || invocation.Node.ID != owner || !pinMatches(invocation.Request, "approved-request", eligibilityJSON(t, s.witness.Native)) || !pinMatches(invocation.Response, "raw", raw) || !pinMatches(invocation.Process, "process", processData) || !pinMatches(invocation.Executable, "native-executable", executable) ||
			!captureMatches(capture.Spec.Stdin, "stdin-frame", requestFrame) || !captureMatches(capture.Stdout, "stdout-frame", stdout) || !captureMatches(capture.Process, "process", processData) || !captureMatches(capture.Stderr, "stderr", stderr) || process.StderrBytes != uint64(len(stderr)) {
			t.Fatalf("%s invocation request/response/executable/process/stderr closure differs", owner)
		}
	}
	assertClosure(primaryID, primaryInvocation, completePrimary, primaryProcess, []byte{})
	assertClosure(otherID, otherInvocation, otherCapture, otherProcess, otherStderr)
	s.reg.Native.InvocationID = otherID
	s.semantic.record.process = otherProcess
	if mismatch == "selected-other-count" && s.semantic.record.process.StderrBytes == primaryProcess.StderrBytes {
		t.Fatal("selected cache did not retain the alternate process byte count")
	}
	return primaryProcess, otherProcess, completePrimary
}
