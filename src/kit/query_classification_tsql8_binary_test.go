package kit

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func classificationLibraryPin(s *eligibilitySession) (eligibilityMember, bool) {
	path := "current/go.dll"
	if s != nil && s.reg.Context == semanticPagedBinariesContext {
		path += ".pages.json"
	}
	if s == nil {
		return eligibilityMember{}, false
	}
	pin, ok := s.owned[path]
	return pin, ok
}

// Core PASS binds the snapshot parts; consumers need only the descriptor and response identities.
func classificationTSQL8Library(s *eligibilitySession) (semanticBinaryLogical, error) {
	if s == nil || s.reg.Context != semanticPagedBinariesContext || !classificationLibraryMatches(s) || len(s.witness.Required) != 8 {
		return semanticBinaryLogical{}, fmt.Errorf("TSQL8 guarded library binding missing")
	}
	var librarySHA string
	for _, tuple := range s.witness.Required {
		o, ok := s.observation(tuple)
		out := s.outputs[o.InvocationID]
		if !ok || out == nil || o.ResponseOrdinal >= uint64(len(out.responses)) || out.responses[o.ResponseOrdinal] == nil {
			return semanticBinaryLogical{}, fmt.Errorf("TSQL8 guarded library response missing")
		}
		sha := out.responses[o.ResponseOrdinal].Grammar.LibrarySHA256
		if !hex64.MatchString(sha) || librarySHA != "" && sha != librarySHA {
			return semanticBinaryLogical{}, fmt.Errorf("TSQL8 response library identities differ")
		}
		librarySHA = sha
	}
	pin, _ := classificationLibraryPin(s)
	data, ok, err := s.member(&pin)
	if err != nil || !ok {
		return semanticBinaryLogical{}, fmt.Errorf("TSQL8 guarded library descriptor absent: %v", err)
	}
	var d semanticBinaryDescriptor
	if err := s.b.decode("classification/library-descriptor", data, &d, true); err != nil {
		return semanticBinaryLogical{}, err
	}
	if d.Schema != semanticBinaryPagesSchema || d.Owner != "go-library-build" || d.Logical.Kind != "library" || d.Logical.Path != "current/go.dll" || d.Logical.Bytes <= MaxDocumentBytes || d.Logical.Bytes > semanticBinaryLogicalMax || d.Logical.SHA256 != librarySHA {
		return semanticBinaryLogical{}, fmt.Errorf("TSQL8 descriptor differs from guarded library responses")
	}
	return d.Logical, nil
}

func classificationGuardedLibrarySize(s *eligibilitySession) (uint64, error) {
	if !classificationLibraryMatches(s) {
		return 0, fmt.Errorf("reference library differs from compiler member")
	}
	if s.reg.Context == semanticPagedBinariesContext {
		logical, err := classificationTSQL8Library(s)
		return logical.Bytes, err
	}
	pin, _ := classificationLibraryPin(s)
	data, ok, err := s.member(&pin)
	if err != nil || !ok {
		return 0, fmt.Errorf("guarded library absent: %v", err)
	}
	return uint64(len(data)), nil
}

func classificationTSQL8BinaryIdentities(s *eligibilitySession, inv *eligibilityInvocation) (driver, library string, err error) {
	if s == nil || s.reg.Context != semanticPagedBinariesContext || s.reg.Native == nil || inv == nil || inv.Executable == nil {
		return "", "", fmt.Errorf("TSQL8 guarded binary binding missing")
	}
	exe := inv.Executable
	if inv.ID != s.reg.Native.InvocationID || inv.Kind != "NATIVE_DRIVER" || exe.Member.Path != "oracle/capture/driver.pages.json" || exe.Member.Role != "driver-paged-descriptor" || exe.NodeID != inv.Node.ID || s.invocations[inv.ID] != inv {
		return "", "", fmt.Errorf("TSQL8 guarded driver structure differs")
	}
	registered := false
	for i := range s.reg.Invocations {
		if &s.reg.Invocations[i] == inv {
			registered = true
		}
	}
	if !registered || !hex64.MatchString(s.witness.Native.ExecutableSHA256) {
		return "", "", fmt.Errorf("TSQL8 guarded driver registration or identity differs")
	}
	logical, err := classificationTSQL8Library(s)
	if err != nil {
		return "", "", err
	}
	return s.witness.Native.ExecutableSHA256, logical.SHA256, nil
}

func classificationTSQL8BinarySession(t *testing.T) (*eligibilitySession, *eligibilityInvocation) {
	t.Helper()
	b := semanticTestBudget(t)
	s := &eligibilitySession{r: b.r, b: b, res: &QueryReferenceEligibilityResult{}, owned: map[string]eligibilityMember{}, owners: map[string]string{}, invocations: map[string]*eligibilityInvocation{}, snapshot: &evidenceSnapshot{files: map[string][]byte{}, verifiedMembers: map[string]ReplayMember{}}}
	s.reg.Context = semanticPagedBinariesContext
	inv := &eligibilityInvocation{ID: "native-tsql8-empty-definitions", Kind: "NATIVE_DRIVER", Node: QueryReferenceNode{ID: "native-tsql8-empty-definitions"}}
	s.reg.Invocations = []eligibilityInvocation{*inv}
	inv = &s.reg.Invocations[0]
	s.invocations[inv.ID] = inv
	s.reg.Native = &eligibilityNative{InvocationID: inv.ID}
	add := func(node, path, role string, data []byte) eligibilityMember {
		pin := eligibilityMember{NodeID: node, Member: ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)}}
		s.owned[path], s.owners[path], s.snapshot.files[path], s.snapshot.verifiedMembers[path] = pin, node, data, pin.Member
		s.reg.OwnedMembers = append(s.reg.OwnedMembers, pin)
		return pin
	}
	for _, kind := range []string{"driver", "library"} {
		d, parts, _ := semanticBinaryTestDescriptor(kind, MaxDocumentBytes+1, bytes.Repeat([]byte(kind[:1]), MaxDocumentBytes))
		node, prefix, descPath := inv.ID, "oracle/", "oracle/capture/driver.pages.json"
		if kind == "library" {
			node, prefix, descPath = "go-library-build", "", "current/go.dll.pages.json"
			d.Owner, d.Logical.Path = node, "current/go.dll"
		}
		for i := range d.Parts {
			p := &d.Parts[i]
			data := parts[p.Path]
			if kind == "library" {
				p.Path = fmt.Sprintf("current/go.dll.part-%05d.bin", i)
			}
			add(node, prefix+p.Path, kind+"-paged-part", data)
		}
		pin := add(node, descPath, kind+"-paged-descriptor", eligibilityJSON(t, d))
		if kind == "driver" {
			inv.Executable = &pin
			s.witness.Native.ExecutableSHA256 = d.Logical.SHA256
		} else {
			s.reg.Native.ReferenceLibrary = &pin
		}
	}
	s.outputs = map[string]*eligibilityOutput{}
	for i := range 8 {
		id := fmt.Sprintf("reference-%d", i)
		tuple := QueryWitnessTuple{Query: fmt.Sprintf("q-%d", i), TreeRole: "incremental"}
		s.witness.Required = append(s.witness.Required, tuple)
		s.reg.Observations = append(s.reg.Observations, eligibilityObservation{Query: tuple.Query, TreeRole: tuple.TreeRole, InvocationID: id})
		r := &instrumentedReference{}
		pin := s.reg.Native.ReferenceLibrary
		var d semanticBinaryDescriptor
		if err := s.b.decode("fixture/library", s.snapshot.files[pin.Member.Path], &d, true); err != nil {
			t.Fatal(err)
		}
		r.Grammar.LibrarySHA256 = d.Logical.SHA256
		s.outputs[id] = &eligibilityOutput{responses: []*instrumentedReference{r}}
	}
	return s, inv
}

func TestQueryClassificationTSQL8BinaryBindings(t *testing.T) {
	for _, name := range []string{"normal", "response-sha-mismatch", "response-sha-invalid", "response-missing", "response-count", "driver-id", "driver-kind", "driver-path", "driver-role", "driver-node", "driver-invocation", "driver-registration", "driver-sha-invalid", "library-pin-sha", "library-pin-owner", "library-pin-role", "context", "descriptor-schema", "descriptor-owner", "descriptor-kind", "descriptor-path", "descriptor-sha", "descriptor-size", "descriptor-large"} {
		t.Run(name, func(t *testing.T) {
			s, inv := classificationTSQL8BinarySession(t)
			switch name {
			case "response-sha-mismatch":
				s.outputs["reference-0"].responses[0].Grammar.LibrarySHA256 = strings.Repeat("f", 64)
			case "response-sha-invalid":
				for _, out := range s.outputs {
					out.responses[0].Grammar.LibrarySHA256 = "invalid"
				}
				pin := s.reg.Native.ReferenceLibrary
				var d semanticBinaryDescriptor
				if err := s.b.decode("fixture/invalid-sha", s.snapshot.files[pin.Member.Path], &d, true); err != nil {
					t.Fatal(err)
				}
				d.Logical.SHA256 = "invalid"
				s.snapshot.files[pin.Member.Path] = eligibilityJSON(t, d)
			case "response-missing":
				s.outputs["reference-7"].responses = nil
			case "response-count":
				s.witness.Required = s.witness.Required[:7]
			case "driver-id":
				s.reg.Native.InvocationID = "other"
			case "driver-kind":
				inv.Kind = "REFERENCE_PROCESS"
			case "driver-path":
				inv.Executable.Member.Path = "oracle/capture/other.pages.json"
			case "driver-role":
				inv.Executable.Member.Role = "executable"
			case "driver-node":
				inv.Executable.NodeID = "other"
			case "driver-invocation":
				delete(s.invocations, inv.ID)
			case "driver-registration":
				s.reg.Invocations = nil
			case "driver-sha-invalid":
				s.witness.Native.ExecutableSHA256 = "invalid"
			case "library-pin-sha":
				s.reg.Native.ReferenceLibrary.Member.SHA256 = strings.Repeat("f", 64)
			case "library-pin-owner":
				s.reg.Native.ReferenceLibrary.NodeID = inv.ID
			case "library-pin-role":
				s.reg.Native.ReferenceLibrary.Member.Role = "runtime-library"
			case "context":
				s.reg.Context = semanticPagedContext
				// Keep the remaining library bindings coherent to isolate the context guard.
				pin := *s.reg.Native.ReferenceLibrary
				data := s.snapshot.files[pin.Member.Path]
				pin.Member.Path, pin.Member.Role = "current/go.dll", "runtime-library"
				s.reg.Native.ReferenceLibrary = &pin
				s.owned[pin.Member.Path], s.owners[pin.Member.Path] = pin, pin.NodeID
				s.snapshot.files[pin.Member.Path], s.snapshot.verifiedMembers[pin.Member.Path] = data, pin.Member
			case "descriptor-schema", "descriptor-owner", "descriptor-kind", "descriptor-path", "descriptor-sha", "descriptor-size", "descriptor-large":
				pin := s.reg.Native.ReferenceLibrary
				var d semanticBinaryDescriptor
				if err := s.b.decode("fixture/descriptor", s.snapshot.files[pin.Member.Path], &d, true); err != nil {
					t.Fatal(err)
				}
				switch name {
				case "descriptor-schema":
					d.Schema = "other"
				case "descriptor-owner":
					d.Owner = "other"
				case "descriptor-kind":
					d.Logical.Kind = "driver"
				case "descriptor-path":
					d.Logical.Path = "current/other.dll"
				case "descriptor-sha":
					d.Logical.SHA256 = strings.Repeat("f", 64)
				case "descriptor-size":
					d.Logical.Bytes = 0
				case "descriptor-large":
					d.Logical.Bytes = semanticBinaryLogicalMax + 1
				}
				s.snapshot.files[pin.Member.Path] = eligibilityJSON(t, d)
			}
			d, l, err := classificationTSQL8BinaryIdentities(s, inv)
			if name != "normal" {
				if err == nil {
					t.Fatal("changed binary binding accepted")
				}
				return
			}
			// Equivalence uses core paging only in this synthetic regression, never in projection.
			wantD, goodD, errD := s.nativePagedDriver(inv)
			wantL, goodL, errL := s.nativePagedLibrary(*s.reg.Native.ReferenceLibrary)
			if err != nil || errD != nil || errL != nil || !goodD || !goodL || d != wantD.SHA256 || l != wantL.SHA256 || d == inv.Executable.Member.SHA256 || l == s.reg.Native.ReferenceLibrary.Member.SHA256 {
				t.Fatalf("logical binary equivalence: %s %s %v %v %v", d, l, err, errD, errL)
			}
			// Parts have already been checked by core; consumers must not read or reassemble them.
			for path := range s.snapshot.files {
				if strings.Contains(path, ".part-") {
					delete(s.snapshot.files, path)
				}
			}
			before := s.b.logical
			d2, l2, reuseErr := classificationTSQL8BinaryIdentities(s, inv)
			size, sizeErr := classificationGuardedLibrarySize(s)
			if reuseErr != nil || sizeErr != nil || d2 != d || l2 != l || size != wantL.Bytes || s.b.logical-before >= wantL.Bytes {
				t.Fatalf("guarded reuse read parts or allocated library: size=%d logical=%d errors=%v %v", size, s.b.logical-before, reuseErr, sizeErr)
			}
		})
	}
}

func TestQueryClassificationTSQL8ReuseCharge(t *testing.T) {
	s, inv := classificationTSQL8BinarySession(t)
	pin := *s.reg.Native.ReferenceLibrary
	var d semanticBinaryDescriptor
	if err := s.b.decode("fixture/library", s.snapshot.files[pin.Member.Path], &d, true); err != nil {
		t.Fatal(err)
	}
	// Predicted DLL size and three full-length SHA pins; no binary data is read or allocated.
	d.Logical.Bytes = 34976060
	d.Parts = append(d.Parts, d.Parts[0])
	for i := range d.Parts {
		p := &d.Parts[i]
		p.Index, p.Offset = uint64(i), uint64(i)*MaxDocumentBytes
		p.Path, p.Bytes = fmt.Sprintf("current/go.dll.part-%05d.bin", i), MaxDocumentBytes
		if i == len(d.Parts)-1 {
			p.Bytes = d.Logical.Bytes - p.Offset
		}
	}
	data := eligibilityJSON(t, d)
	pin.Member.Bytes, pin.Member.SHA256 = uint64(len(data)), digestHex(data)
	s.reg.Native.ReferenceLibrary = &pin
	s.owned[pin.Member.Path], s.snapshot.verifiedMembers[pin.Member.Path], s.snapshot.files[pin.Member.Path] = pin, pin.Member, data
	beforeWork, beforeLogical := s.b.work, s.b.logical
	if _, err := classificationGuardedLibrarySize(s); err != nil {
		t.Fatal(err)
	}
	if _, _, err := classificationTSQL8BinaryIdentities(s, inv); err != nil {
		t.Fatal(err)
	}
	work, logical := s.b.work-beforeWork, s.b.logical-beforeLogical
	b := semanticTestBudget(t)
	if err := b.decode("expected/descriptor", data, &d, true); err != nil {
		t.Fatal(err)
	}
	memberLogical := uint64(len(pin.Member.Path) + len(pin.Member.Role) + len(pin.Member.SHA256))
	if work != 2*(b.work+1) || logical != 2*(b.logical+memberLogical) {
		t.Fatalf("reuse must charge only two descriptor reads/decodes: work=%d logical=%d", work, logical)
	}
	t.Logf("predicted DLL descriptor bytes=%d; two reuse calls work=%d logical=%d (decode each work=%d logical=%d; member each logical=%d)", len(data), work, logical, b.work, b.logical, memberLogical)
}

func TestQueryClassificationTSQL8Assembly(t *testing.T) {
	f := classificationTSQL8RowsTestFixture(t)
	anchors, plan := f.anchors, f.plan
	s, driver := classificationTSQL8BinarySession(t)
	root := t.TempDir()
	write := func(path string, data []byte) {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		actualJSONWrite(t, full, data)
	}
	compiler := []byte("synthetic compiler; never executed")
	plan.CompilerSHA256 = digestHex(compiler)
	plan.LibraryRequest.Path = filepath.Join(root, "compiler.bin")
	write("compiler.bin", compiler)
	write("library/request.json", eligibilityJSON(t, plan.LibraryRequest))
	ex := eligibilityFixtureExecution(plan.LibraryRequest, 0, false, nil, nil)
	ex.Capabilities.OS, ex.Capabilities.Backend = "windows/amd64", "windows-job-object"
	ex.Cleanup.Method, ex.Cleanup.Scope, ex.Capabilities.TreeCleanup = "JOB_OBJECT", "JOB_OBJECT", "JOB_OBJECT"
	write("library/receipt.json", eligibilityJSON(t, eligibilityReceipt{Schema: "tsgk-reference-process/r1", Execution: ex}))
	var files []ReplayMember
	for path, data := range s.snapshot.files {
		if strings.HasPrefix(path, "oracle/") {
			pin := s.owned[path].Member
			pin.Path = strings.TrimPrefix(path, "oracle/")
			files = append(files, pin)
			write("native/"+pin.Path, data)
		} else {
			name := filepath.Base(path)
			if path == "current/go.dll.pages.json" {
				name = "tsql.dll.pages.json"
			}
			write("library/"+name, data)
		}
	}
	add := func(path, role string, data []byte) ReplayMember {
		pin := ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)}
		files = append(files, pin)
		write("native/"+path, data)
		return pin
	}
	// This checks assembly wiring only, not grammar or native execution eligibility.
	add("capture/build.json", "build", eligibilityJSON(t, map[string]any{"executable_sha256": s.witness.Native.ExecutableSHA256, "closure": []map[string]string{{"path": "src/parser.c", "role": "grammar-parser"}}}))
	add("capture/build-input-00000.pages.json", "grammar-paged-descriptor", []byte("synthetic parser descriptor"))
	add("capture/build-manifest.txt", "build-manifest", []byte("synthetic manifest"))
	stdin := add("capture/stdin.bin", "stdin-frame", []byte{})
	process := add("capture/process.json", "process", []byte("{}"))
	stdout := add("capture/stdout.bin", "stdout-frame", []byte("{}"))
	write("native/capture/"+anchors.Case+".json", eligibilityJSON(t, semanticProcessCapture{Stage: "RUN_RETURNED", Spec: &semanticCaptureSpec{Path: "C:/driver.exe", Args: []string{"single"}, Stdin: &stdin}, Process: &process, Stdout: &stdout}))
	write("native/input/source.bin", []byte{})
	write("native/raw/"+anchors.Case+".json", []byte("{}"))
	write("native/native-complete.json", eligibilityJSON(t, map[string]any{"schema": "tsgk-query-classification-native-capture/r1", "route": "tsql", "structural_definitions": 8, "semantic_pass": false, "anchors_sha256": anchors.sha256, "build_identity": strings.Repeat("b", 64), "executable_path": "C:/driver.exe", "files": files, "raw_sha256": digestHex([]byte("{}"))}))
	librarySHA := digestHex(append(bytes.Repeat([]byte("l"), MaxDocumentBytes), 'l'))
	for i, q := range anchors.Queries {
		write("native/input/"+q.ID+".scm", []byte(q.Source))
		req := eligibilityProcessRequest{Path: "C:/reference.exe", Args: []string{"C:/tsql.dll", anchors.nativeSymbol(), "C:/source.bin", "C:/query.scm", q.ID, librarySHA}}
		dir := fmt.Sprintf("references/current-ref-%d/", i)
		write(dir+"request.json", eligibilityJSON(t, req))
		write(dir+"receipt.json", eligibilityJSON(t, eligibilityReceipt{Schema: "tsgk-reference-process/r1", Execution: eligibilityExecution{Status: StatusCompleted}, StdoutBase64: base64.StdEncoding.EncodeToString([]byte("{}"))}))
	}
	assembly := &actualJSONAssembly{t: t, evidence: filepath.Join(t.TempDir(), "evidence"), nodes: map[string]EvidenceNode{}, cohortSource: map[string]string{"current": plan.SourceRevision}, workload: anchors.Workload}
	if err := os.MkdirAll(filepath.Join(filepath.Dir(assembly.evidence), "caller"), 0700); err != nil {
		t.Fatal(err)
	}
	reference := eligibilityMember{Member: ReplayMember{SHA256: strings.Repeat("f", 64)}}
	assembly.reg.Build = &eligibilityBuild{ReferenceBinary: &reference}
	classificationGoCurrent(t, assembly, root, anchors, plan)
	inv := assembly.reg.Invocations[0]
	node := assembly.nodes[inv.Node.ID]
	if len(node.Files) < len(files) || node.Identities["tool"] != s.witness.Native.ExecutableSHA256 || inv.Executable.Member.SHA256 != driver.Executable.Member.SHA256 || assembly.reg.Native.Grammar["src/parser.c"].Member.Role != "grammar-paged-descriptor" {
		t.Fatal("assembly lost captured members or used descriptor identity as tool")
	}
	if *assembly.reg.Native.ReferenceLibrary != s.owned["current/go.dll.pages.json"] || len(assembly.reg.Observations) != 8 {
		t.Fatal("physical library binding or fact references lost")
	}
	for _, inv := range assembly.reg.Invocations[1:] {
		if inv.Inputs["library"].MemberPin.Member.SHA256 == librarySHA {
			t.Fatal("registration must pin the physical descriptor")
		}
	}
}
