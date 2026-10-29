package kit

import (
	"fmt"
	"testing"
)

func TestQueryClassificationReferenceLibraryBinding(t *testing.T) {
	compiler := eligibilityMember{NodeID: "go-library-build", Member: ReplayMember{Path: "current/go.dll", Role: "runtime-library", Bytes: 1, SHA256: digestHex([]byte("A"))}}
	for _, mismatch := range []string{"", "node", "path", "role", "bytes", "sha", "missing", "other-library"} {
		t.Run(mismatch, func(t *testing.T) {
			reference := compiler
			switch mismatch {
			case "node":
				reference.NodeID = "another-compiler"
			case "path":
				reference.Member.Path = "other/library.dll"
			case "role":
				reference.Member.Role = "other-library"
			case "bytes":
				reference.Member.Bytes++
			case "sha":
				reference.Member.SHA256 = digestHex([]byte("B"))
			case "other-library":
				reference.NodeID, reference.Member.Path, reference.Member.SHA256 = "another-compiler", "other/library.dll", digestHex([]byte("B"))
			}
			s := &eligibilitySession{owned: map[string]eligibilityMember{"current/go.dll": compiler}}
			s.reg.Native = &eligibilityNative{ReferenceLibrary: &reference}
			if mismatch == "missing" {
				delete(s.owned, "current/go.dll")
			}
			if mismatch == "other-library" {
				// Both stored members are individually verified; their cross-library binding still differs.
				b := semanticTestBudget(t)
				s.r, s.b, s.res = b.r, b, &QueryReferenceEligibilityResult{}
				s.owned[reference.Member.Path] = reference
				s.owners = map[string]string{compiler.Member.Path: compiler.NodeID, reference.Member.Path: reference.NodeID}
				s.snapshot = &evidenceSnapshot{files: map[string][]byte{compiler.Member.Path: []byte("A"), reference.Member.Path: []byte("B")}, verifiedMembers: map[string]ReplayMember{compiler.Member.Path: compiler.Member, reference.Member.Path: reference.Member}}
				for _, pin := range []eligibilityMember{compiler, reference} {
					data, ok, err := s.member(&pin)
					if err != nil || !ok || digestHex(data) != pin.Member.SHA256 {
						t.Fatal("independent library member integrity must pass before cross-binding rejection", err)
					}
				}
			}
			if classificationLibraryMatches(s) != (mismatch == "") {
				t.Fatal("reference library was not bound to the complete compiler member identity", mismatch)
			}
		})
	}
}

func TestQueryClassificationStderrBinding(t *testing.T) {
	for _, route := range []string{"go", "csharp"} {
		for _, mismatch := range []string{"", "nonzero-present", "missing-zero", "missing-nonzero", "owner", "count", "role"} {
			t.Run(route+"/"+mismatch, func(t *testing.T) {
				a := classificationAnchors{Route: route}
				b := semanticTestBudget(t)
				path, id := classificationNativePrefix(a)+"capture/stderr.bin", "native-fixture"
				data := []byte{}
				if mismatch == "nonzero-present" || mismatch == "missing-nonzero" || mismatch == "owner" || mismatch == "count" {
					data = []byte("x")
				}
				pin := ReplayMember{Path: "capture/stderr.bin", Role: "stderr", Bytes: uint64(len(data)), SHA256: digestHex(data)}
				mapped := pin
				mapped.Path = path
				member := eligibilityMember{NodeID: id, Member: mapped}
				s := &eligibilitySession{r: b.r, b: b, res: &QueryReferenceEligibilityResult{}, owned: map[string]eligibilityMember{path: member}, owners: map[string]string{path: id}, snapshot: &evidenceSnapshot{files: map[string][]byte{path: data}, verifiedMembers: map[string]ReplayMember{path: mapped}}}
				capture := semanticProcessCapture{Stderr: &pin}
				process := eligibilityExecution{StderrBytes: uint64(len(data))}
				switch mismatch {
				case "missing-zero", "missing-nonzero":
					capture.Stderr = nil
				case "owner":
					member.NodeID = "another-native"
					s.owned[path], s.owners[path] = member, member.NodeID
				case "count":
					process.StderrBytes++
				case "role":
					capture.Stderr.Role = "stdout-frame"
				}
				read := func(pin *ReplayMember) ([]byte, error) {
					mapped := *pin
					mapped.Path = classificationNativePrefix(a) + mapped.Path
					data, ok, err := s.member(&eligibilityMember{NodeID: id, Member: mapped})
					if err != nil || !ok {
						return nil, fmt.Errorf("guarded stderr read: %v", err)
					}
					return data, nil
				}
				err := classificationCaptureStderr(a, capture, process, read)
				wantPass := mismatch == "" || mismatch == "nonzero-present" || route == "go" && mismatch == "missing-zero"
				if (err == nil) != wantPass {
					t.Fatalf("stderr binding %s/%s: %v", route, mismatch, err)
				}
			})
		}
	}
}

func TestQueryClassificationCSharpRegistration(t *testing.T) {
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", "csharp15-empty")
	a, _ := classificationGoAnchors(t)
	w := classificationGoRegistration(a)
	if _, err := validateQueryWitnessRegistration(w, queryEligibilityWitnessSchema); err != nil {
		t.Fatal(err)
	}
	if w.Route != "csharp" || w.Input.Bytes != 0 || w.Input.SHA256 != digestHex(nil) || len(w.Queries) != 15 || len(w.Required) != 15 || a.nativeSymbol() != "tree_sitter_c_sharp" || a.libraryName() != "csharp.dll" || classificationNativePrefix(a) != "oracle/" {
		t.Fatal("C# single-frame registration or paged capture namespace differs")
	}
	plan := classificationPlan{NativeContext: semanticPagedContext}
	if !classificationPlanContextMatches(a, plan) || classificationPlanContextMatches(a, classificationPlan{}) {
		t.Fatal("C# registration did not require its paged context")
	}
	for _, tuple := range w.Required {
		if tuple.Step != 0 || tuple.TreeRole != "incremental" || tuple.Structural == nil || len(tuple.Structural) != 0 || tuple.Expected == nil || len(tuple.Expected) != 0 {
			t.Fatal("C# registration changed the explicit empty capture denominator")
		}
	}
	changed := a
	changed.sha256 = classificationPositiveAnchorsSHA
	if classificationPlanContextMatches(changed, plan) {
		t.Fatal("a different anchor selected the C# context")
	}
	unknown := classificationAnchors{Route: "unknown"}
	if classificationPlanContextMatches(unknown, plan) || unknown.nativeContext() != "" || unknown.nativeSymbol() != "" {
		t.Fatal("an unknown route selected a classification context")
	}
}

// This checks the additional logical pin guard. Page contents remain the core
// eligibility verifier's responsibility and are not synthetic execution evidence.
func TestQueryClassificationCSharpGrammarContext(t *testing.T) {
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", "csharp15-empty")
	a, _ := classificationGoAnchors(t)
	for _, mismatch := range []string{"", "context", "owner", "role", "logical-hash", "logical-path", "descriptor-owner", "missing"} {
		t.Run(mismatch, func(t *testing.T) {
			b := semanticTestBudget(t)
			id := "native-" + a.Case
			grammar := NativeInput{Path: "src/parser.c", Role: "parser", Bytes: 51472926, SHA256: "b1a77b75e034af978ebbde75be71e362cae447f67ec25dcf297702cdca9a1dbb"}
			plan := classificationPlan{NativeContext: semanticPagedContext, Grammar: []NativeInput{grammar}}
			var descriptor semanticParserDescriptor
			descriptor.Schema, descriptor.Owner = "tsgk-query-parser-pages/r1", "D"
			descriptor.Logical.Path, descriptor.Logical.Role = grammar.Path, "grammar-parser"
			descriptor.Logical.Bytes, descriptor.Logical.SHA256 = grammar.Bytes, grammar.SHA256
			switch mismatch {
			case "logical-hash":
				descriptor.Logical.SHA256 = digestHex(nil)
			case "logical-path":
				descriptor.Logical.Path = "src/other.c"
			case "descriptor-owner":
				descriptor.Owner = "other"
			}
			data := eligibilityJSON(t, map[string]any{"schema": descriptor.Schema, "owner": descriptor.Owner, "logical": descriptor.Logical, "parts": []any{}})
			path := "oracle/capture/build-input-00000.pages.json"
			pin := eligibilityMember{NodeID: id, Member: ReplayMember{Path: path, Role: "grammar-paged-descriptor", Bytes: uint64(len(data)), SHA256: digestHex(data)}}
			s := &eligibilitySession{r: b.r, b: b, res: &QueryReferenceEligibilityResult{}, owned: map[string]eligibilityMember{path: pin}, owners: map[string]string{path: id}, snapshot: &evidenceSnapshot{files: map[string][]byte{path: data}, verifiedMembers: map[string]ReplayMember{path: pin.Member}}, invocations: map[string]*eligibilityInvocation{id: {Node: QueryReferenceNode{ID: id}}}}
			s.reg.Context = semanticPagedContext
			s.reg.Native = &eligibilityNative{InvocationID: id, Grammar: map[string]eligibilityMember{grammar.Path: pin}}
			switch mismatch {
			case "context":
				s.reg.Context = "full-input-utf8/r2"
			case "owner":
				pin.NodeID = "another-native"
				s.reg.Native.Grammar[grammar.Path] = pin
			case "role":
				pin.Member.Role = "grammar-parser"
				s.reg.Native.Grammar[grammar.Path] = pin
			case "missing":
				delete(s.reg.Native.Grammar, grammar.Path)
			}
			err := classificationGrammarContext(s, a, plan)
			if (err == nil) != (mismatch == "") {
				t.Fatalf("logical parser guard mismatch %q: %v", mismatch, err)
			}
		})
	}
}
