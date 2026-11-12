package kit

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// ordinaryNativeClosure binds r8's build bytes to the selected plan and current approval.
func (b *eligibilityBudget) ordinaryNativeClosure(root *os.Root, s *eligibilitySession, in ordinaryVerifiedInputs, profile OracleProfile, set *ordinaryGuardedSet, add func(string, string, string)) (bool, *Error) {
	if b == nil || s == nil || s.b != b || root == nil || set == nil || s.ordinary == nil || s.ordinary.session != s || s.snapshot == nil || !s.snapshot.graphBound {
		return false, fail(KindInvalidInput, "ORDINARY_BUILD_UNBOUND", "oracle", nil)
	}
	valid := true
	bad := func(code, at string, message string) {
		valid = false
		add(code, at, message)
	}
	ordinary := s.ordinary.Binding
	registration := in.registration
	if s.approval.Ordinary == nil || !ordinaryEligibilityBindingMatches(*s.approval.Ordinary, ordinary) || ordinary.ClosureEncoding != registration.ClosureEncoding || registration.Context != OrdinaryContext ||
		ordinary.Candidate != registration.Candidate || ordinary.SemanticCohort != registration.SemanticCohort || set.Manifest.Candidate != ordinary.Candidate || set.Manifest.SemanticCohort != ordinary.SemanticCohort || set.Manifest.RunID != ordinary.RunID ||
		set.Manifest.ClosureEncoding != ordinary.ClosureEncoding || !slices.Equal(set.Manifest.ShardIDs, ordinary.ShardIDs) {
		bad("ORDINARY_BUILD_APPROVAL", "oracle/manifest.json", "build가 승인된 ordinary candidate·cohort·run·closure 선택에 결속되지 않았다")
	}
	current, currentOK := s.cohorts[ordinary.SemanticCohort]
	candidate := s.approval.Candidate
	if !currentOK || current.Purpose != "current" || current.ID != ordinary.SemanticCohort || current.SourceRevision != candidate.SourceRevision || current.Platform != candidate.Platform || current.Run != candidate.Run || current.Attempt != candidate.Attempt || current.Platform != registration.Subject {
		bad("ORDINARY_BUILD_COHORT", "approval/ordinary", "build current cohort가 승인 candidate와 선택 route platform에 결속되지 않았다")
	}
	pagingValid := false
	if s.snapshot.paging != nil {
		var err *Error
		pagingValid, err = s.snapshot.paging.valid(s)
		if err != nil {
			return false, err
		}
		if !pagingValid || s.snapshot.paging.root != root || s.snapshot.paging.b != b {
			bad("ORDINARY_BUILD_PAGING", "oracle", "paged closure authority가 동일 root와 ledger에 결속되지 않았다")
		}
	}
	if ordinary.ClosureEncoding != "plain" && !pagingValid {
		bad("ORDINARY_BUILD_PAGING", "oracle", "paged closure에 verified part authority가 없다")
		return false, nil
	}
	var planned *OrdinaryBinding
	for i := range in.profile.Bindings {
		if in.profile.Bindings[i].Route == registration.Route && in.profile.Bindings[i].Platform == registration.Platform {
			planned = &in.profile.Bindings[i]
			break
		}
	}
	if planned == nil || planned.Subject != registration.Subject || planned.ClosureEncoding != registration.ClosureEncoding || !reflect.DeepEqual(planned.Grammar, profile.Native.Grammar) || !reflect.DeepEqual(planned.Grammar, registrationGrammar(in)) ||
		in.profile.Candidate != ordinary.Candidate || in.profile.SemanticCohort != ordinary.SemanticCohort || profile.Native.Route != registration.Route || !ValidLanguageSymbol(profile.Native.Symbol) || profile.Native.Operation != "native-query-batch" || profile.Native.Encoding != EncodingUTF8 || profile.Native.Output != OutputTree || !profile.API ||
		profile.Native.Compiler.Name == "" || profile.Native.Compiler.Version == "" || !hex64.MatchString(profile.Native.Compiler.SHA256) || profile.Native.Compiler.Bytes == 0 {
		bad("ORDINARY_BUILD_PROFILE", "capture/build.json", "실행 profile의 grammar·compiler·route가 승인된 planning profile과 다르다")
	}
	compilerTools := 0
	for _, tool := range s.reg.Tools {
		if e := b.charge(uint64(len(tool.ID)+len(tool.ToolName))+1, 256, "ordinary-compiler-approval"); e != nil {
			return false, e
		}
		if tool.CohortID != ordinary.SemanticCohort || tool.ToolName != profile.Native.Compiler.Name {
			continue
		}
		compilerTools++
		_, ok, e := s.memberIdentity(&tool.IdentityMember)
		if e != nil {
			return false, e
		}
		if !ok || tool.SubjectSHA256 != profile.Native.Compiler.SHA256 {
			bad("ORDINARY_BUILD_COMPILER", "approval/tools/"+tool.ID, "compiler executable이 current cohort의 승인된 tool pin과 다르다")
		}
	}
	if compilerTools != 1 {
		bad("ORDINARY_BUILD_COMPILER", "approval/tools", "current cohort에 일치하는 compiler tool이 정확히 하나 필요하다")
	}
	if planned != nil {
		runtimeInfo, err := fs.Stat(drivers.NativeC, "native-c/runtime-manifest.json")
		if err != nil {
			return false, fail(KindInvalidInput, "NATIVE_STATIC_CLOSURE", "runtime-manifest.json", err)
		}
		if e := b.charge(1, 256+uint64(len(planned.Runtime.Path)), "ordinary-runtime-pin"); e != nil {
			return false, e
		}
		if !runtimeInfo.Mode().IsRegular() || runtimeInfo.Size() < 0 || planned.Runtime.Bytes != uint64(runtimeInfo.Size()) || planned.Runtime.SHA256 != reuseobs.RuntimeManifestSHA256 {
			bad("ORDINARY_BUILD_RUNTIME_PIN", planned.Runtime.Path, "planning runtime pin이 내장된 runtime manifest와 다르다")
		}
		policyText := []byte(NativePolicyText(NativeOperations()["native-query-batch"], OutputTree))
		if in.profile.NativePolicy != queryregistrycore.FilePin(in.profile.NativePolicy.Path, policyText) || set.Manifest.Policy != NativePolicy(NativeOperations()["native-query-batch"], OutputTree, nil) || !reflect.DeepEqual(set.Manifest.Operation, NativeOperations()["native-query-batch"]) {
			bad("ORDINARY_BUILD_POLICY", "approval/ordinary", "planning native policy와 r8 operation policy가 다르다")
		}
	}
	static, runtimeCommit, e := b.nativeStaticClosure(profile.Native.Symbol)
	if e != nil {
		return false, e
	}
	if set.Manifest.Producer["runtime_commit"] != runtimeCommit {
		bad("ORDINARY_BUILD_RUNTIME", "capture/build.json", "실행 runtime이 내장된 verified runtime closure와 다르다")
	}

	members := make(map[string]OrdinaryMember, len(set.Manifest.Members))
	for _, m := range set.Manifest.Members {
		if e := b.charge(20*uint64(len(m.Path)+len(m.Role)+len(m.Owner))+1, 256+16*uint64(len(m.Path)), "ordinary-build-member-index"); e != nil {
			return false, e
		}
		if m.Owner == "D_build" {
			if m.CaseOrBatch != "build" || m.Path == "" || members[m.Path].Path != "" {
				bad("ORDINARY_BUILD_OWNER", m.Path, "build closure member의 owner·역할 단위가 중복 또는 불일치한다")
			}
			members[m.Path] = m
		}
		if strings.HasPrefix(m.Path, "capture/build-") && m.Owner != "D_build" {
			bad("ORDINARY_BUILD_OWNER", m.Path, "capture build member가 D_build에 속하지 않는다")
		}
	}
	member := func(path, role string) (OrdinaryMember, []byte, bool, *Error) {
		m, ok := members[path]
		if !ok || m.Role != role || m.Pin.Path != path || !ordinaryPin(m.Pin) {
			return OrdinaryMember{}, nil, false, nil
		}
		full := "oracle/" + path
		owner, owned := s.owned[full]
		if !owned || owner.NodeID != s.owners[full] || owner.Member != (ReplayMember{Path: full, Role: role, Bytes: m.Pin.Bytes, SHA256: m.Pin.SHA256}) {
			return OrdinaryMember{}, nil, false, nil
		}
		data, found, err := s.member(&owner)
		return m, data, found, err
	}
	buildMember, buildData, found, e := member("capture/build.json", "build")
	if e != nil {
		return false, e
	}
	if !found {
		bad("ORDINARY_BUILD_MISSING", "capture/build.json", "r8에 승인된 build description이 없다")
		return false, nil
	}
	manifestMember, buildManifest, found, e := member("capture/build-manifest.txt", "build-manifest")
	if e != nil {
		return false, e
	}
	if !found {
		bad("ORDINARY_BUILD_MANIFEST", "capture/build-manifest.txt", "r8에 canonical build identity text가 없다")
		return false, nil
	}
	var build semanticBuildDescription
	if e := b.decode(buildMember.Path, buildData, &build, false); e != nil {
		return false, e
	}
	if build.Identity != manifestMember.Pin.SHA256 || build.Identity != set.Manifest.Producer["build_identity"] || build.Identity == "" ||
		build.ExecutableSHA256 != set.Manifest.Producer["executable_sha256"] || build.ExecutableBytes == 0 || build.Compiler != profile.Native.Compiler || build.Compiler.SHA256 != set.Manifest.Producer["compiler_sha256"] || build.CompilerVersion == "" ||
		build.Runtime != runtimeCommit || build.Runtime != set.Manifest.Producer["runtime_commit"] || build.Symbol != profile.Native.Symbol || build.Closure == nil || set.Manifest.Producer["platform"] != registration.Subject {
		bad("ORDINARY_BUILD_BINDING", buildMember.Path, "build identity·compiler·runtime·driver가 producer와 승인 profile에 결속되지 않았다")
	}
	if buildMember.Owner != "D_build" || manifestMember.Owner != "D_build" || buildMember.CaseOrBatch != "build" || manifestMember.CaseOrBatch != "build" {
		bad("ORDINARY_BUILD_OWNER", buildMember.Path, "build description 또는 identity text의 owner가 다르다")
	}
	if len(build.Closure) == 0 || len(build.Closure) != len(static)+len(profile.Native.Grammar) {
		bad("ORDINARY_BUILD_CLOSURE", buildMember.Path, "build closure가 승인된 runtime·driver·grammar file 집합과 다르다")
	}
	if !utf8.Valid(buildManifest) {
		bad("ORDINARY_BUILD_MANIFEST", manifestMember.Path, "canonical build identity text가 UTF-8이 아니다")
		return false, nil
	}
	if e := b.charge(8*uint64(len(buildManifest))+1, 16*uint64(len(buildManifest)), "ordinary-build-manifest"); e != nil {
		return false, e
	}
	if build.Identity != digestHex(buildManifest) {
		bad("ORDINARY_BUILD_MANIFEST", manifestMember.Path, "build identity hash가 canonical text pin과 다르다")
	}
	var identity strings.Builder
	fmt.Fprintf(&identity, "tsgk-native-build/r1\nsymbol=%s\ncompiler=%s %d\nruntime=%s\nos=%s\n", build.Symbol, build.Compiler.SHA256, build.Compiler.Bytes, build.Runtime, set.Manifest.Producer["platform"])
	closure := slices.Clone(build.Closure)
	slices.SortFunc(closure, func(a, c semanticBuildInput) int { return strings.Compare(a.Role+" "+a.Path, c.Role+" "+c.Path) })
	for _, input := range closure {
		fmt.Fprintf(&identity, "file=%s %s %s %d\n", input.Role, input.Path, input.SHA256, input.Bytes)
	}
	steps, validReuse := build.effectiveSteps()
	if !validReuse {
		bad("ORDINARY_BUILD_STEPS", buildMember.Path, "build cache reuse의 steps와 source steps 조합이 유효하지 않다")
	}
	if steps == nil || len(steps) < 6 {
		bad("ORDINARY_BUILD_STEPS", buildMember.Path, "build steps가 불완전하다")
	} else {
		for _, step := range steps {
			if e := b.charge(uint64(len(step.Name))+2*uint64(len(step.Argv))+1, 256+64*uint64(len(step.Argv)), "ordinary-build-step"); e != nil {
				return false, e
			}
			fmt.Fprintf(&identity, "step=%s %q\n", step.Name, step.Argv)
		}
	}
	expectedSteps := []string{"version", "runtime", "parser"}
	if e := b.charge(uint64(len(profile.Native.Grammar)), 16*uint64(len(profile.Native.Grammar)), "ordinary-build-step-plan"); e != nil {
		return false, e
	}
	for _, grammar := range profile.Native.Grammar {
		if grammar.Role == "scanner" {
			expectedSteps = append(expectedSteps, "scanner")
		}
	}
	expectedSteps = append(expectedSteps, "driver", "shim", "link")
	if len(steps) != len(expectedSteps) {
		bad("ORDINARY_BUILD_STEPS", buildMember.Path, "compiler step 순서와 수가 grammar closure와 다르다")
	} else {
		for i, step := range steps {
			if step.Name != expectedSteps[i] || len(step.Argv) == 0 {
				bad("ORDINARY_BUILD_STEPS", buildMember.Path, "compiler step 이름 또는 argv가 canonical build order와 다르다")
			}
		}
	}
	if identity.String() != string(buildManifest) {
		bad("ORDINARY_BUILD_MANIFEST", manifestMember.Path, "build description과 canonical identity text가 다르다")
	}
	if e := b.r.check(); e != nil {
		return false, e
	}

	if e := b.charge(uint64(len(static)), 256*uint64(len(static)), "ordinary-build-static-index"); e != nil {
		return false, e
	}
	expected := make(map[string]eligibilitySnapshotRow, len(static)+len(profile.Native.Grammar))
	allowedMembers := map[string]bool{"capture/build.json": true, "capture/build-manifest.txt": true}
	for key, row := range static {
		expected[key] = row
	}
	grammarSeen := map[string]bool{}
	parsers, scanners, grammarCount := 0, 0, 0
	for _, grammar := range profile.Native.Grammar {
		if e := b.charge(20*uint64(len(grammar.Path)+len(grammar.Role))+1, 256+16*uint64(len(grammar.Path)), "ordinary-grammar-index"); e != nil {
			return false, e
		}
		key := "grammar-" + grammar.Role + " " + grammar.Path
		if !portable(grammar.Path, false) || !hex64.MatchString(grammar.SHA256) || grammar.Bytes == 0 || grammar.Bytes > BuildFileBytes || grammarSeen[key] {
			bad("ORDINARY_BUILD_GRAMMAR", grammar.Path, "profile grammar input이 중복되거나 유효하지 않다")
			continue
		}
		grammarSeen[key] = true
		expected[key] = eligibilitySnapshotRow{Bytes: grammar.Bytes, SHA256: grammar.SHA256}
		switch grammar.Role {
		case "parser":
			parsers++
			if grammar.Bytes > MaxDocumentBytes && ordinary.ClosureEncoding == "plain" {
				bad("ORDINARY_BUILD_GRAMMAR", grammar.Path, "plain closure에 paged parser가 포함됐다")
			}
		case "scanner":
			scanners++
		case "header":
		default:
			bad("ORDINARY_BUILD_GRAMMAR", grammar.Path, "grammar role이 허용되지 않는다")
		}
		grammarCount++
	}
	if parsers != 1 || grammarCount > BuildFiles {
		bad("ORDINARY_BUILD_GRAMMAR", buildMember.Path, "parser 또는 grammar file cardinality가 다르다")
	}
	seen := map[string]bool{}
	for i, input := range build.Closure {
		if e := b.charge(20*uint64(len(input.Path))+1, 256+16*uint64(len(input.Path)), "ordinary-build-closure"); e != nil {
			return false, e
		}
		key := input.Role + " " + input.Path
		if !portable(input.Path, false) || !hex64.MatchString(input.SHA256) || input.Bytes == 0 || input.Bytes > BuildFileBytes || seen[key] {
			bad("ORDINARY_BUILD_CLOSURE", input.Path, "closure input이 중복되거나 유효하지 않다")
			continue
		}
		seen[key] = true
		if row, ok := expected[key]; !ok || row.Bytes != input.Bytes || row.SHA256 != input.SHA256 {
			bad("ORDINARY_BUILD_INPUT", input.Path, "runtime·driver·grammar input이 승인된 logical closure와 다르다")
		}
		if e := b.r.check(); e != nil {
			return false, e
		}
		if input.Role == "grammar-parser" && input.Bytes > MaxDocumentBytes {
			path := fmt.Sprintf("capture/build-input-%05d.pages.json", i)
			allowedMembers[path] = true
			descMember, data, ok, e := member(path, "grammar-paged-descriptor")
			if e != nil {
				return false, e
			}
			if !ok || ordinary.ClosureEncoding == "plain" {
				bad("ORDINARY_BUILD_PARSER_DESCRIPTOR", path, "large parser의 descriptor가 없거나 closure encoding이 이를 허용하지 않는다")
				continue
			}
			var d semanticParserDescriptor
			if e := b.decodeOrdinaryTyped(path, data, &d); e != nil {
				return false, e
			}
			if d.Schema != "tsgk-query-parser-pages/r1" || d.Owner != "D_build" || d.Logical.ClosureIndex != uint64(i) || d.Logical.Role != input.Role || d.Logical.Path != input.Path || d.Logical.Bytes != input.Bytes || d.Logical.SHA256 != input.SHA256 || uint64(len(d.Parts)) != (input.Bytes+MaxDocumentBytes-1)/MaxDocumentBytes {
				bad("ORDINARY_BUILD_PARSER_DESCRIPTOR", path, "logical parser descriptor가 build closure와 다르다")
				continue
			}
			source, e := ordinaryBuildPageSource(s, descMember)
			if e != nil {
				return false, e
			}
			if source == nil {
				bad("ORDINARY_BUILD_PARSER_PART", path, "parser descriptor가 verified paging authority에 결속되지 않았다")
				continue
			}
			h := sha256.New()
			for partIndex, part := range d.Parts {
				if e := b.charge(uint64(len(part.Path)+len(part.SHA256))+1, 256+uint64(len(part.Path)), "ordinary-build-parser-part"); e != nil {
					return false, e
				}
				offset := uint64(partIndex) * MaxDocumentBytes
				size := min(uint64(MaxDocumentBytes), input.Bytes-offset)
				want := ReplayMember{Path: fmt.Sprintf("capture/build-input-%05d.part-%05d.bin", i, partIndex), Role: "grammar-paged-part", Bytes: size, SHA256: part.SHA256}
				allowedMembers[want.Path] = true
				if part.Index != uint64(partIndex) || part.Offset != offset || part.Path != want.Path || part.Bytes != size || !hex64.MatchString(part.SHA256) {
					bad("ORDINARY_BUILD_PARSER_DESCRIPTOR", path, "parser part 순서·offset·크기·경로가 다르다")
					break
				}
				ok, e := b.ordinaryBuildPageHash(h, want, source, members, s)
				if e != nil {
					return false, e
				}
				if !ok {
					bad("ORDINARY_BUILD_PARSER_PART", want.Path, "parser part가 동일 verified snapshot에 없거나 pin이 다르다")
					break
				}
			}
			if source == nil || source.stream == nil && hexDigestHash(h) != input.SHA256 || source.stream != nil && source.chainSum(h) != input.SHA256 {
				bad("ORDINARY_BUILD_PARSER_HASH", path, "streamed parser bytes가 승인된 grammar hash와 다르다")
			}
			continue
		}
		path := fmt.Sprintf("capture/build-input-%05d.bin", i)
		allowedMembers[path] = true
		role := semanticBuildMemberRole(input.Role)
		m, data, ok, e := member(path, role)
		if e != nil {
			return false, e
		}
		if !ok || m.Pin.Bytes != input.Bytes || m.Pin.SHA256 != input.SHA256 {
			bad("ORDINARY_BUILD_INPUT", path, "build input member pin이 logical closure와 다르다")
			continue
		}
		if e := b.charge(input.Bytes+1, 2*(input.Bytes+1), "ordinary-build-input-hash"); e != nil {
			return false, e
		}
		if uint64(len(data)) != input.Bytes || digestHex(data) != input.SHA256 {
			bad("ORDINARY_BUILD_INPUT", path, "build input 실제 bytes가 logical closure와 다르다")
		}
	}
	for key := range expected {
		if !seen[key] {
			bad("ORDINARY_BUILD_STATIC_MISSING", key, "runtime·driver·grammar logical closure input이 빠졌다")
		}
	}
	if scanners > BuildFiles || grammarCount > BuildFiles {
		bad("ORDINARY_BUILD_GRAMMAR", buildMember.Path, "grammar closure가 native build 제한을 넘었다")
	}
	if e := ordinaryNativeDriverClosure(b, s, ordinary, members, build, bad); e != nil {
		return false, e
	}
	if build.ExecutableBytes <= MaxDocumentBytes {
		allowedMembers["capture/driver.bin"] = true
	} else if build.ExecutableBytes <= semanticBinaryLogicalMax {
		allowedMembers["capture/driver.pages.json"] = true
		for i := uint64(0); i < (build.ExecutableBytes+MaxDocumentBytes-1)/MaxDocumentBytes; i++ {
			allowedMembers[fmt.Sprintf("capture/driver.part-%05d.bin", i)] = true
		}
	} else {
		allowedMembers["capture/driver.pages.json"] = true
	}
	if len(members) != len(allowedMembers) {
		bad("ORDINARY_BUILD_MEMBER_SET", "oracle/manifest.json", "D_build member 집합이 build identity closure와 다르다")
	}
	for path := range members {
		if !allowedMembers[path] {
			bad("ORDINARY_BUILD_MEMBER_SET", path, "등록된 D_build member가 build identity에 소비되지 않았다")
		}
	}
	if e := b.r.check(); e != nil {
		return false, e
	}
	return valid, nil
}

func registrationGrammar(in ordinaryVerifiedInputs) []NativeInput {
	return in.route.Workload.Grammar
}

func ordinaryBuildPageSource(s *eligibilitySession, member OrdinaryMember) (*verifiedPageSource, *Error) {
	full := "oracle/" + member.Path
	pin, ok := s.owned[full]
	if !ok || pin.NodeID != s.owners[full] || pin.Member != (ReplayMember{Path: full, Role: member.Role, Bytes: member.Pin.Bytes, SHA256: member.Pin.SHA256}) {
		return nil, nil
	}
	return s.pageSource(pin, "oracle/", nil)
}

func (b *eligibilityBudget) ordinaryBuildPageHash(h hash.Hash, pin ReplayMember, source *verifiedPageSource, members map[string]OrdinaryMember, s *eligibilitySession) (bool, *Error) {
	callback := func(want ReplayMember) ([]byte, bool, *Error) {
		m, ok := members[want.Path]
		if !ok || m.Owner != "D_build" || m.CaseOrBatch != "build" || m.Role != want.Role || m.Pin.Bytes != want.Bytes || m.Pin.SHA256 != want.SHA256 {
			return nil, false, nil
		}
		data, ok, e := b.ordinaryMemberBytes(s, m)
		return data, ok, e
	}
	if source == nil {
		data, ok, e := callback(pin)
		if e != nil || !ok {
			return false, e
		}
		if e := b.charge(pin.Bytes+1, 2*(pin.Bytes+1), "ordinary-build-page-hash"); e != nil {
			return false, e
		}
		if uint64(len(data)) != pin.Bytes || digestHex(data) != pin.SHA256 {
			return false, nil
		}
		_, _ = h.Write(data)
		return true, nil
	}
	return b.pageHash(h, pin, callback, source, "ordinary-build-page-hash")
}

func hexDigestHash(h hash.Hash) string { return fmt.Sprintf("%x", h.Sum(nil)) }

func ordinaryNativeDriverClosure(b *eligibilityBudget, s *eligibilitySession, ordinary ordinaryEligibilityBinding, members map[string]OrdinaryMember, build semanticBuildDescription, add func(string, string, string)) *Error {
	bad := func(code, at, message string) { add(code, at, message) }
	if build.ExecutableBytes == 0 || build.ExecutableBytes > semanticBinaryLogicalMax || !hex64.MatchString(build.ExecutableSHA256) {
		bad("ORDINARY_BUILD_DRIVER", "capture/driver.bin", "native driver logical identity가 유효하지 않다")
		return nil
	}
	if build.ExecutableBytes <= MaxDocumentBytes {
		m, ok, e := b.ordinaryBuildMemberIdentity(s, members, "capture/driver.bin", "driver")
		if e != nil {
			return e
		}
		if !ok || m.Pin.Bytes != build.ExecutableBytes || m.Pin.SHA256 != build.ExecutableSHA256 {
			bad("ORDINARY_BUILD_DRIVER", "capture/driver.bin", "native driver bytes가 build identity와 다르다")
		}
		for path := range members {
			if strings.HasPrefix(path, "capture/driver.pages.json") || strings.HasPrefix(path, "capture/driver.part-") {
				bad("ORDINARY_BUILD_DRIVER_DESCRIPTOR", path, "plain driver와 paging descriptor가 함께 등록됐다")
			}
		}
		return nil
	}
	if ordinary.ClosureEncoding != "binary-paged" {
		bad("ORDINARY_BUILD_DRIVER_DESCRIPTOR", "capture/driver.pages.json", "큰 driver에는 binary-paged closure가 필요하다")
		return nil
	}
	dm, descriptorBytes, ok, e := b.ordinaryBuildMemberBytes(s, members, "capture/driver.pages.json", "driver-paged-descriptor")
	if e != nil {
		return e
	}
	if !ok {
		bad("ORDINARY_BUILD_DRIVER_DESCRIPTOR", "capture/driver.pages.json", "큰 driver의 logical descriptor가 없다")
		return nil
	}
	var descriptor semanticBinaryDescriptor
	if e := b.decodeOrdinaryTyped(dm.Path, descriptorBytes, &descriptor); e != nil {
		return e
	}
	if descriptor.Schema != semanticBinaryPagesSchema || descriptor.Owner != "D_build" || descriptor.Logical.Kind != "driver" || descriptor.Logical.Path != "capture/driver.bin" || descriptor.Logical.Bytes != build.ExecutableBytes || descriptor.Logical.SHA256 != build.ExecutableSHA256 || uint64(len(descriptor.Parts)) != (build.ExecutableBytes+MaxDocumentBytes-1)/MaxDocumentBytes {
		bad("ORDINARY_BUILD_DRIVER_DESCRIPTOR", dm.Path, "driver logical descriptor가 build/compiler output과 다르다")
		return nil
	}
	source, e := ordinaryBuildPageSource(s, dm)
	if e != nil {
		return e
	}
	if source == nil {
		bad("ORDINARY_BUILD_DRIVER_PART", dm.Path, "driver descriptor가 verified paging authority에 결속되지 않았다")
		return nil
	}
	h := sha256.New()
	for i, part := range descriptor.Parts {
		if e := b.charge(uint64(len(part.Path)+len(part.SHA256))+1, 256+uint64(len(part.Path)), "ordinary-build-driver-part"); e != nil {
			return e
		}
		size := min(uint64(MaxDocumentBytes), build.ExecutableBytes-uint64(i)*MaxDocumentBytes)
		pin := ReplayMember{Path: fmt.Sprintf("capture/driver.part-%05d.bin", i), Role: "driver-paged-part", Bytes: size, SHA256: part.SHA256}
		if part.Index != uint64(i) || part.Offset != uint64(i)*MaxDocumentBytes || part.Path != pin.Path || part.Bytes != size || !hex64.MatchString(part.SHA256) {
			bad("ORDINARY_BUILD_DRIVER_DESCRIPTOR", dm.Path, "driver part 순서·offset·크기·경로가 다르다")
			return nil
		}
		valid, e := b.ordinaryBuildPageHash(h, pin, source, members, s)
		if e != nil {
			return e
		}
		if !valid {
			bad("ORDINARY_BUILD_DRIVER_PART", pin.Path, "driver part가 동일 verified snapshot에 없거나 pin이 다르다")
			return nil
		}
	}
	if source != nil && source.stream != nil {
		if source.chainSum(h) != build.ExecutableSHA256 {
			bad("ORDINARY_BUILD_DRIVER_HASH", dm.Path, "streamed driver hash가 logical descriptor와 다르다")
		}
	} else if hexDigestHash(h) != build.ExecutableSHA256 {
		bad("ORDINARY_BUILD_DRIVER_HASH", dm.Path, "driver part hash가 logical descriptor와 다르다")
	}
	return nil
}

func (b *eligibilityBudget) ordinaryBuildMemberBytes(s *eligibilitySession, members map[string]OrdinaryMember, path, role string) (OrdinaryMember, []byte, bool, *Error) {
	m, ok := members[path]
	if !ok || m.Owner != "D_build" || m.CaseOrBatch != "build" || m.Role != role || m.Pin.Path != path {
		return OrdinaryMember{}, nil, false, nil
	}
	data, ok, e := b.ordinaryMemberBytes(s, m)
	return m, data, ok, e
}

func (b *eligibilityBudget) ordinaryBuildMemberIdentity(s *eligibilitySession, members map[string]OrdinaryMember, path, role string) (OrdinaryMember, bool, *Error) {
	m, ok := members[path]
	if !ok || m.Owner != "D_build" || m.CaseOrBatch != "build" || m.Role != role || m.Pin.Path != path {
		return OrdinaryMember{}, false, nil
	}
	full := "oracle/" + path
	pin, ok := s.owned[full]
	if !ok || pin.NodeID != s.owners[full] || pin.Member != (ReplayMember{Path: full, Role: role, Bytes: m.Pin.Bytes, SHA256: m.Pin.SHA256}) {
		return OrdinaryMember{}, false, nil
	}
	identity, ok, e := s.memberIdentity(&pin)
	return m, ok && identity.member == pin.Member, e
}
