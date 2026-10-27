package kit

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"path"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

type semanticExpectationResult struct {
	Step         int            `json:"step"`
	Syntax       string         `json:"syntax"`
	Contains     []string       `json:"contains"`
	Anchors      []ExpectAnchor `json:"anchors,omitempty"`
	Declarations string         `json:"declarations"`
	Result       string         `json:"result"`
	Detail       string         `json:"detail,omitempty"`
}
type semanticQueryExpectationResult struct {
	Query    string                 `json:"query"`
	Step     int                    `json:"step"`
	Status   string                 `json:"status"`
	Code     string                 `json:"code"`
	Captures *[]CaptureExpectation  `json:"captures"`
	Error    *QueryErrorExpectation `json:"error"`
	Result   string                 `json:"result"`
	Detail   string                 `json:"detail,omitempty"`
}

func semanticBuildMemberRole(role string) string {
	if strings.HasPrefix(role, "grammar-") {
		return role
	}
	return "build-input"
}

type semanticBuildInput struct {
	Role   string `json:"role"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  uint64 `json:"bytes"`
}
type semanticBuildDescription struct {
	Identity         string               `json:"identity"`
	ExecutableSHA256 string               `json:"executable_sha256"`
	ExecutableBytes  uint64               `json:"executable_bytes"`
	Compiler         ToolIdentity         `json:"compiler"`
	Runtime          string               `json:"runtime_commit"`
	Symbol           string               `json:"symbol"`
	Closure          []semanticBuildInput `json:"closure"`
	CompilerVersion  string               `json:"compiler_version"`
	Steps            []semanticBuildStep  `json:"steps"`
	Reuse            *semanticBuildReuse  `json:"reuse,omitempty"`
}

type semanticBuildStep struct {
	Name string   `json:"name"`
	Argv []string `json:"argv"`
}
type semanticBuildReuse struct {
	Hit         bool                `json:"hit"`
	SourceSteps []semanticBuildStep `json:"source_steps"`
}

func (b *eligibilityBudget) semanticAST(doc string, data []byte) (*jv, *Error) {
	if uint64(len(data)) > MaxDocumentBytes {
		return nil, fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", doc, nil)
	}
	if e := b.reserveJSON(doc, data, 1); e != nil {
		return nil, e
	}
	return decodeBounded(doc, data, MaxDocumentBytes, &valueBudget{max: 2000000, check: b.r.check, charge: func() *Error { return b.charge(1, 0, doc) }})
}

func (b *eligibilityBudget) semanticValuesEqual(a, c *jv) (bool, *Error) {
	if e := b.charge(1, 0, "semantic-projection-compare"); e != nil {
		return false, e
	}
	if a == nil || c == nil {
		return a == c, nil
	}
	if a.kind != c.kind || len(a.vals) != len(c.vals) {
		return false, nil
	}
	if a.kind != '{' && a.kind != '[' {
		if e := b.charge(uint64(len(a.s)+len(c.s)), 0, "semantic-projection-compare"); e != nil {
			return false, e
		}
		return a.s == c.s, nil
	}
	if a.kind == '[' {
		for i := range a.vals {
			same, e := b.semanticValuesEqual(a.vals[i], c.vals[i])
			if e != nil || !same {
				return same, e
			}
		}
		return true, nil
	}
	if e := b.charge(uint64(len(c.keys)), 256*uint64(len(c.keys)), "semantic-projection-compare"); e != nil {
		return false, e
	}
	members := make(map[string]*jv, len(c.keys))
	for i, k := range c.keys {
		members[k] = c.vals[i]
	}
	for i, k := range a.keys {
		same, e := b.semanticValuesEqual(a.vals[i], members[k])
		if e != nil || !same {
			return same, e
		}
	}
	return true, nil
}

type semanticSetState struct {
	producerFailure  func(string, string, string)
	producerVerified bool
	producerCode     string
	projection       *jv
	local            *OracleQuerySemantics
	capture          *semanticProcessCapture
	execution        *eligibilityExecution
	build            semanticBuildDescription
}

func (b *eligibilityBudget) checkSemanticSet(m OracleManifest, files map[string][]byte, record *jv, state *semanticSetState, add func(string, string, string)) *Error {
	bad := func(code, path string) {
		add(code, path, "producer semantic set의 보존 관계가 일치하지 않는다")
	}
	pins := map[string]OracleMember{}
	roles := map[string][]OracleMember{}
	for _, mem := range m.Members {
		pins[mem.Path] = mem
		roles[mem.Role] = append(roles[mem.Role], mem)
	}
	for _, role := range []string{"record", "source", "projection", "driver", "build", "build-manifest"} {
		if len(roles[role]) != 1 {
			bad("SEMANTICS_MEMBER_COUNT", role)
			return nil
		}
	}
	for _, role := range []string{"capture", "stdin-frame", "process", "stdout-frame", "stderr", "raw"} {
		if len(roles[role]) > 1 {
			bad("SEMANTICS_MEMBER_COUNT", role)
			return nil
		}
	}
	available := func(pin OracleMember) bool {
		data, ok := files[pin.Path]
		return ok && uint64(len(data)) == pin.Bytes && digestHex(data) == pin.SHA256
	}
	for _, mem := range m.Members {
		if !available(mem) {
			return nil
		}
	}
	recordPin, projectionPin := roles["record"][0], roles["projection"][0]
	if record == nil {
		return fail(KindInvalidInput, "SEMANTICS_RECORD_MISSING", recordPin.Path, nil)
	}
	projection, e := b.semanticAST(projectionPin.Path, files[projectionPin.Path])
	if e != nil {
		return e
	}
	if state != nil {
		state.projection = projection
	}
	t := typed{doc: recordPin.Path, check: b.r.check, emptyProjectionQueries: len(m.Queries) == 0}
	if jvMember(record, "query_semantics") == nil {
		bad("SEMANTICS_LOCAL_MISSING", recordPin.Path)
		return nil
	}
	local, e := semanticLocalValue(t, jvMember(record, "query_semantics"))
	if e != nil {
		return e
	}
	if state != nil {
		state.local = local
	}
	if projection.kind != '{' || jvMember(projection, "query_semantics") != nil || len(record.keys) != len(projection.keys)+5 {
		bad("SEMANTICS_PROJECTION_INVALID", projectionPin.Path)
		return nil
	}
	required := []string{"id", "input", "encoding", "execution_status", "assessment", "code", "claims", "response_status", "response_code", "producer", "process", "steps", "expectations", "reuse_observation_claim"}
	pm, e := t.object(projection, required, "oracle_claims", "query_expectations")
	if e != nil {
		return e
	}
	for _, key := range []string{"reuse_observation_claim"} {
		value, e := t.str(pm[key])
		if e != nil {
			return e
		}
		if !slices.Contains([]string{claimPass, claimFail, claimBlocked, claimNotClaimed}, value) {
			return t.bad("CLAIM_INVALID", pm[key])
		}
	}
	if value := pm["oracle_claims"]; value != nil {
		keys := []string{"query_equality", "query_expectations", "api", "fact_reproduction", "dynamic_sql"}
		claims, e := t.object(value, keys)
		if e != nil {
			return e
		}
		for _, key := range keys {
			value, e := t.str(claims[key])
			if e != nil {
				return e
			}
			if !slices.Contains([]string{claimPass, claimFail, claimBlocked, claimNotClaimed}, value) {
				return t.bad("CLAIM_INVALID", claims[key])
			}
		}
	}
	for _, key := range []string{"id", "encoding", "execution_status", "assessment", "code", "response_status", "response_code"} {
		if _, e := t.str(pm[key]); e != nil {
			return e
		}
	}
	if _, e := nativeInput(t, pm["input"], map[string]bool{"case": true}); e != nil {
		return e
	}
	claims, e := t.object(pm["claims"], []string{"incremental_equality", "incremental_route", "expectations"})
	if e != nil {
		return e
	}
	for _, key := range []string{"incremental_equality", "incremental_route", "expectations"} {
		v := claims[key]
		value, e := t.str(v)
		if e != nil {
			return e
		}
		if !slices.Contains([]string{claimPass, claimFail, claimBlocked, claimNotClaimed}, value) {
			return t.bad("CLAIM_INVALID", v)
		}
	}
	if e := witnessShape(t, projection, reflect.TypeFor[witnessProjection](), false, false, &valueBudget{max: 2000000, check: b.r.check, charge: func() *Error { return b.charge(1, 0, "semantic-projection-shape") }, reserve: func(n uint64) *Error { return b.charge(0, n, "semantic-projection-shape") }}); e != nil {
		return e
	}
	for _, key := range []string{"steps", "expectations"} {
		if _, e := t.array(pm[key]); e != nil {
			return e
		}
	}
	for _, key := range []string{"process", "producer"} {
		if pm[key].kind != 'n' && pm[key].kind != '{' {
			return t.bad("SEMANTICS_PROJECTION_SHAPE", pm[key])
		}
	}
	for _, item := range []struct {
		name string
		typ  reflect.Type
	}{{"expectations", reflect.TypeFor[[]semanticExpectationResult]()}, {"query_expectations", reflect.TypeFor[[]semanticQueryExpectationResult]()}} {
		if value := jvMember(projection, item.name); value != nil {
			if e := eligibilityShape(t, value, item.typ, false, &valueBudget{max: 2000000, check: b.r.check, charge: func() *Error { return b.charge(1, 0, "semantic-expectation-shape") }, reserve: func(n uint64) *Error { return b.charge(0, n, "semantic-expectation-shape") }}); e != nil {
				return e
			}
		}
	}
	producer := pm["producer"]
	if producer.kind == 'n' {
		if pm["execution_status"].s == StatusCompleted {
			return t.bad("SEMANTICS_PRODUCER_MISSING", producer)
		}
	} else {
		fields, e := t.object(producer, []string{"language_version", "runtime_language_version", "runtime_min_compatible", "query", "api", "predicates", "symbol_count", "field_count", "reuse_observation"})
		if e != nil {
			return e
		}
		numbers := map[string]uint64{}
		for _, key := range []string{"language_version", "runtime_language_version", "runtime_min_compatible", "symbol_count", "field_count"} {
			value, e := t.uint(fields[key])
			if e != nil {
				return e
			}
			if value > 0xffffffff {
				return t.bad("SEMANTICS_PRODUCER_INVALID", fields[key])
			}
			numbers[key] = value
		}
		if numbers["runtime_min_compatible"] == 0 || numbers["runtime_min_compatible"] > numbers["runtime_language_version"] || pm["execution_status"].s == StatusCompleted && (numbers["runtime_min_compatible"] > numbers["language_version"] || numbers["language_version"] > numbers["runtime_language_version"]) {
			return t.bad("SEMANTICS_PRODUCER_INVALID", producer)
		}
		for _, key := range []string{"query", "api", "predicates"} {
			value, e := t.str(fields[key])
			if e != nil {
				return e
			}
			if value == "" || key == "predicates" && value != "NOT_EVALUATED" {
				return t.bad("SEMANTICS_PRODUCER_INVALID", fields[key])
			}
		}
		capability, e := t.object(fields["reuse_observation"], []string{"policy", "runtime_manifest_sha256"})
		if e != nil {
			return e
		}
		policy, e := t.str(capability["policy"])
		if e != nil {
			return e
		}
		manifest, e := t.str(capability["runtime_manifest_sha256"])
		if e != nil {
			return e
		}
		if policy != reuseobs.Policy || manifest != reuseobs.RuntimeManifestSHA256 {
			return t.bad("SEMANTICS_PRODUCER_INVALID", producer)
		}
	}
	projectionValid := true
	for i, key := range record.keys {
		if slices.Contains([]string{"schema", "case", "workload", "complete", "query_semantics", "assessment", "code"}, key) {
			continue
		}
		same, e := b.semanticValuesEqual(record.vals[i], jvMember(projection, key))
		if e != nil {
			return e
		}
		if !same {
			projectionValid = false
			bad("SEMANTICS_PROJECTION_MISMATCH", key)
		}
	}
	text := func(v *jv, key string) string {
		x := jvMember(v, key)
		if x == nil || x.kind != '"' {
			return ""
		}
		return x.s
	}
	status, assessment, code := text(projection, "execution_status"), text(projection, "assessment"), text(projection, "code")
	if !slices.Contains([]string{AssessPass, AssessFail, AssessBlocked, AssessNotAssessed}, assessment) {
		projectionValid = false
		bad("SEMANTICS_PROJECTION_VERDICT", projectionPin.Path)
	}
	if status != StatusCompleted {
		allowed, ok := statusAssessments[status]
		if !ok || !slices.Contains(allowed, assessment) {
			projectionValid = false
			bad("SEMANTICS_PROJECTION_VERDICT", projectionPin.Path)
		}
	}
	if status == StatusCompleted && len(roles["raw"]) != 1 {
		bad("SEMANTICS_RAW_MISSING", recordPin.Path)
	}
	if status == StatusCompleted && assessment == AssessPass {
		if code != "" {
			projectionValid = false
			bad("SEMANTICS_PROJECTION_VERDICT", projectionPin.Path)
		}
		assessment, code = AssessBlocked, "EXTERNAL_APPROVAL_PENDING"
	}
	producerVerdict := projectionValid && text(record, "assessment") == assessment && text(record, "code") == code && m.Assessment == assessment && m.ExecutionStatus == func() string {
		if status == StatusNotRun {
			return StatusFailed
		}
		return status
	}()
	if !producerVerdict {
		bad("SEMANTICS_PRODUCER_VERDICT", recordPin.Path)
	}
	if status == StatusCompleted {
		stored := []string{pm["reuse_observation_claim"].s}
		for _, value := range claims {
			stored = append(stored, value.s)
		}
		if oracle := pm["oracle_claims"]; oracle != nil {
			for _, value := range oracle.vals {
				stored = append(stored, value.s)
			}
		}
		if foldClaims(stored) != pm["assessment"].s {
			producerVerdict = false
			bad("SEMANTICS_BASE_STORED_VERDICT", projectionPin.Path)
		}
	}
	if state != nil {
		state.producerVerified, state.producerCode = producerVerdict, code
		if producerVerdict && testHookSemanticProducerFold != nil {
			testHookSemanticProducerFold(b)
		}
		if producerVerdict && m.Assessment == AssessFail && state.producerFailure != nil {
			preserved := code
			if preserved == "" {
				preserved = "SEMANTICS_PRODUCER_FAILED"
			}
			state.producerFailure(preserved, recordPin.Path, "이미 확인한 producer FAIL과 최초 code를 유지한다")
		}

	}
	if local.RegistrationSHA256 != m.QuerySemanticsRegistrationSHA256 || local.Projection.Path != projectionPin.Path || local.Projection.Bytes != projectionPin.Bytes || local.Projection.SHA256 != projectionPin.SHA256 {
		bad("SEMANTICS_LOCAL_BINDING", recordPin.Path)
	}
	member := func(pin *ReplayMember, role string) bool {
		if pin == nil {
			return true
		}
		actual, ok := pins[pin.Path]
		if !ok || actual.Case != recordPin.Case || actual.Role != role || actual.Bytes != pin.Bytes || actual.SHA256 != pin.SHA256 {
			bad("SEMANTICS_CAPTURE_MEMBER", pin.Path)
			return false
		}
		return true
	}
	processValue := jvMember(projection, "process")
	invoked := processValue != nil && processValue.kind != 'n' || status == StatusCompleted
	if local.Capture == nil {
		if invoked || len(roles["capture"])+len(roles["stdin-frame"])+len(roles["process"])+len(roles["stdout-frame"])+len(roles["stderr"]) != 0 || len(roles["raw"]) != 0 {
			bad("SEMANTICS_CAPTURE_MISSING", recordPin.Path)
		}
	} else if member(local.Capture, "capture") {
		data := files[local.Capture.Path]
		capture, e := b.processCapture(local.Capture.Path, data)
		if e != nil {
			return e
		}
		if state != nil {
			state.capture = &capture
		}
		expected := map[string]bool{local.Capture.Path: true}
		refs := []struct {
			pin  *ReplayMember
			role string
		}{{capture.Process, "process"}, {capture.Stdout, "stdout-frame"}, {capture.Stderr, "stderr"}}
		if capture.Spec != nil {
			refs = append(refs, struct {
				pin  *ReplayMember
				role string
			}{capture.Spec.Stdin, "stdin-frame"})
		}
		for _, ref := range refs {
			if ref.pin != nil {
				member(ref.pin, ref.role)
				expected[ref.pin.Path] = true
			}
		}
		for _, role := range []string{"capture", "stdin-frame", "process", "stdout-frame", "stderr"} {
			for _, pin := range roles[role] {
				if !expected[pin.Path] {
					bad("SEMANTICS_CAPTURE_UNREFERENCED", pin.Path)
				}
			}
		}
		if status == StatusCompleted && (capture.Stage != "RUN_RETURNED" || capture.Error != nil || capture.Spec == nil || capture.Spec.Stdin == nil || capture.Process == nil || capture.Stdout == nil || processValue == nil || processValue.kind == 'n') {
			bad("SEMANTICS_CAPTURE_COMPLETION", recordPin.Path)
		}
		if capture.Spec != nil && capture.Spec.Stdin != nil && member(capture.Spec.Stdin, "stdin-frame") {
			if _, e := b.semanticFrame(capture.Spec.Stdin.Path, files[capture.Spec.Stdin.Path]); e != nil {
				return e
			}
		}
		if capture.Process != nil && member(capture.Process, "process") {
			processData := files[capture.Process.Path]
			var execution eligibilityExecution
			process, e := b.decodeValue(capture.Process.Path, processData, &execution, true)
			if e != nil {
				return e
			}
			if state != nil {
				state.execution = &execution
			}
			if capture.Spec != nil {
				streams := []struct {
					pin       *ReplayMember
					size      uint64
					truncated bool
					limit     int64
				}{{capture.Stdout, execution.StdoutBytes, execution.StdoutTruncated, capture.Spec.StdoutBytes}, {capture.Stderr, execution.StderrBytes, execution.StderrTruncated, capture.Spec.StderrBytes}}
				for _, stream := range streams {
					stored := uint64(0)
					if stream.pin != nil {
						stored = stream.pin.Bytes
					}
					valid := stored == stream.size && !stream.truncated
					if stream.limit > 0 {
						valid = stream.size <= uint64(stream.limit) && stored == stream.size && !stream.truncated || stream.size > uint64(stream.limit) && stored == uint64(stream.limit) && stream.truncated
					}
					if !valid {
						bad("SEMANTICS_PROCESS_STREAM", capture.Process.Path)
					}
				}
			}
			if status == StatusCompleted && (execution.Status != StatusCompleted || execution.Reason != "EXITED" || execution.ExitCode != 0 || execution.StdoutTruncated || execution.StderrTruncated || !execution.Cleanup.Verified || execution.Cleanup.ResidualAfterExit != 0) {
				bad("SEMANTICS_PROCESS_COMPLETION", capture.Process.Path)
			}
			if processValue != nil && processValue.kind != 'n' {
				same, e := b.semanticValuesEqual(process, processValue)
				if e != nil {
					return e
				}
				if !same {
					bad("SEMANTICS_PROCESS_MISMATCH", capture.Process.Path)
				}
			}
		}
		if len(roles["raw"]) == 1 {
			if capture.Stdout == nil || !member(capture.Stdout, "stdout-frame") {
				bad("SEMANTICS_FRAME_MISSING", recordPin.Path)
			} else {
				payload, e := b.semanticFrame(capture.Stdout.Path, files[capture.Stdout.Path])
				if e != nil {
					return e
				}
				if !bytes.Equal(payload, files[roles["raw"][0].Path]) {
					bad("SEMANTICS_FRAME_MISMATCH", capture.Stdout.Path)
				}
			}
		}
	}
	var build semanticBuildDescription
	buildPin, driverPin, manifestPin := roles["build"][0], roles["driver"][0], roles["build-manifest"][0]
	buildData := files[buildPin.Path]
	if e := b.decode(buildPin.Path, buildData, &build, false); e != nil {
		return e
	}
	if state != nil {
		state.build = build
	}
	if build.Identity != m.Producer["build_identity"] || build.Identity != manifestPin.SHA256 || build.ExecutableSHA256 != driverPin.SHA256 || build.ExecutableBytes != driverPin.Bytes || build.ExecutableSHA256 != m.Producer["executable_sha256"] || len(roles["build-input"])+len(roles["grammar-parser"])+len(roles["grammar-scanner"])+len(roles["grammar-header"])+len(roles["grammar-paged-descriptor"]) != len(build.Closure) {
		bad("SEMANTICS_BUILD_BINDING", buildPin.Path)
	}
	if build.Runtime != m.Producer["runtime_commit"] || build.Compiler.SHA256 != m.Producer["compiler_sha256"] || build.CompilerVersion != m.Producer["compiler_version"] {
		bad("SEMANTICS_BUILD_PRODUCER", buildPin.Path)
	}
	static, commit, e := b.nativeStaticClosure(build.Symbol)
	if e != nil {
		return e
	}
	if build.Runtime != commit {
		bad("SEMANTICS_BUILD_RUNTIME", buildPin.Path)
	}
	seen := map[string]bool{}
	parsers, grammar, scanners := 0, 0, 0
	pagedParsers, pagedParts := 0, 0
	for i, input := range build.Closure {
		if e := b.charge(20*uint64(len(input.Path))+1, 16*uint64(len(input.Path))+256, "semantic-build-closure"); e != nil {
			return e
		}
		key := input.Role + " " + input.Path
		if !portable(input.Path, false) || seen[key] {
			bad("SEMANTICS_BUILD_CLOSURE", buildPin.Path)
		}
		seen[key] = true
		if input.Role == "grammar-parser" && input.Bytes > MaxDocumentBytes && m.Schema == OraclePagedSemanticsManifestSchema {
			pin, ok := pins[fmt.Sprintf("capture/build-input-%05d.pages.json", i)]
			valid := ok && pin.Role == "grammar-paged-descriptor" && pin.Case == ""
			if valid {
				var e *Error
				valid, e = b.semanticPagedParser(files[pin.Path], input, i, func(p ReplayMember) ([]byte, bool, *Error) {
					m, ok := pins[p.Path]
					return files[p.Path], ok && m.Role == p.Role && m.Bytes == p.Bytes && m.SHA256 == p.SHA256 && m.Case == "", nil
				})
				if e != nil {
					return e
				}
			}
			if !valid {
				bad("SEMANTICS_PARSER_PAGES", pin.Path)
			}
			pagedParts += int((input.Bytes + MaxDocumentBytes - 1) / MaxDocumentBytes)
			pagedParsers++
		} else {
			pin, ok := pins[fmt.Sprintf("capture/build-input-%05d.bin", i)]
			if input.Bytes > MaxDocumentBytes || !ok || pin.Role != semanticBuildMemberRole(input.Role) || pin.Bytes != input.Bytes || pin.SHA256 != input.SHA256 || pin.Case != "" {
				bad("SEMANTICS_BUILD_INPUT", buildPin.Path)
			}
		}
		if expected, ok := static[key]; ok {
			if expected.Bytes != input.Bytes || expected.SHA256 != input.SHA256 {
				bad("SEMANTICS_BUILD_STATIC", buildPin.Path)
			}
		} else {
			switch input.Role {
			case "grammar-parser":
				parsers++
				grammar++
			case "grammar-scanner":
				scanners++
				grammar++
			case "grammar-header":
				grammar++
			default:
				bad("SEMANTICS_BUILD_CLOSURE", buildPin.Path)
			}
		}
	}
	for key := range static {
		if !seen[key] {
			bad("SEMANTICS_BUILD_STATIC_MISSING", buildPin.Path)
		}
	}
	if len(roles["grammar-paged-descriptor"]) != pagedParsers || len(roles["grammar-paged-part"]) != pagedParts || (m.Schema == OraclePagedSemanticsManifestSchema && pagedParsers != 1) {
		bad("SEMANTICS_PARSER_PAGES", buildPin.Path)
	}
	if parsers != 1 || grammar > BuildFiles {
		bad("SEMANTICS_BUILD_GRAMMAR", buildPin.Path)
	}
	manifest := files[manifestPin.Path]
	if e := b.charge(8*uint64(len(manifest))+1, 16*uint64(len(manifest)), "semantic-build-manifest"); e != nil {
		return e
	}
	if !utf8.Valid(manifest) {
		bad("SEMANTICS_BUILD_MANIFEST", manifestPin.Path)
		return nil
	}
	var prefix strings.Builder
	fmt.Fprintf(&prefix, "tsgk-native-build/r1\nsymbol=%s\ncompiler=%s %d\nruntime=%s\nos=%s\n", build.Symbol, build.Compiler.SHA256, build.Compiler.Bytes, build.Runtime, m.Producer["platform"])
	closure := slices.Clone(build.Closure)
	slices.SortFunc(closure, func(a, c semanticBuildInput) int { return strings.Compare(a.Role+" "+a.Path, c.Role+" "+c.Path) })
	for _, input := range closure {
		fmt.Fprintf(&prefix, "file=%s %s %s %d\n", input.Role, input.Path, input.SHA256, input.Bytes)
	}
	body, ok := strings.CutPrefix(string(manifest), prefix.String())
	if !ok || !strings.HasSuffix(body, "\n") {
		bad("SEMANTICS_BUILD_MANIFEST", manifestPin.Path)
		return nil
	}
	expectedSteps := []string{"version", "runtime", "parser"}
	for i := 0; i < scanners; i++ {
		expectedSteps = append(expectedSteps, "scanner")
	}
	expectedSteps = append(expectedSteps, "driver", "shim", "link")
	steps := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	if len(steps) != len(expectedSteps) {
		bad("SEMANTICS_BUILD_STEPS", manifestPin.Path)
		return nil
	}
	observedSteps := build.Steps
	if build.Reuse != nil && build.Reuse.Hit {
		if len(build.Steps) != 0 {
			bad("SEMANTICS_BUILD_STEPS", buildPin.Path)
		}
		observedSteps = build.Reuse.SourceSteps
	}
	if len(observedSteps) != len(steps) {
		bad("SEMANTICS_BUILD_STEPS", buildPin.Path)
		return nil
	}
	for i, line := range steps {
		name, argv, ok := eligibilityBuildStep(strings.TrimPrefix(line, "step="))
		if name != observedSteps[i].Name || !slices.Equal(argv, observedSteps[i].Argv) {
			bad("SEMANTICS_BUILD_STEPS", buildPin.Path)
		}
		if !strings.HasPrefix(line, "step=") || !ok || name != expectedSteps[i] {
			bad("SEMANTICS_BUILD_STEPS", manifestPin.Path)
		}
	}
	return nil
}

func (b *eligibilityBudget) walkSemanticSet(fsys fs.FS, listed map[string]bool, add func(string, string, string)) *Error {
	files, dirs := uint64(0), uint64(0)
	var walk func(string, uint64) *Error
	walk = func(dir string, depth uint64) (resultErr *Error) {
		if e := b.charge(1, 0, dir); e != nil {
			return e
		}
		if depth > DefaultLimits().Depth {
			return fail(KindResourceLimit, "DEPTH_LIMIT", dir, nil)
		}
		f, err := fsys.Open(dir)
		if err != nil {
			return fail(KindIO, "SET_UNREADABLE", dir, err)
		}
		defer func() {
			if err := f.Close(); err != nil && resultErr == nil {
				resultErr = fail(KindIO, "SET_CLOSE_FAILED", dir, err)
			}
		}()
		stream, ok := f.(fs.ReadDirFile)
		if !ok {
			return fail(KindIO, "SET_DIRECTORY_UNBOUNDED", dir, nil)
		}
		for {
			if e := b.charge(1, 128*256, dir); e != nil {
				return e
			}
			entries, err := stream.ReadDir(128)
			if len(entries) > 128 {
				return fail(KindIO, "SET_DIRECTORY_UNBOUNDED", dir, nil)
			}
			for _, entry := range entries {
				name := entry.Name()
				if e := b.charge(20*uint64(len(name))+1, 16*uint64(len(name))+256, dir); e != nil {
					return e
				}
				if !portable(name, false) || strings.Contains(name, "/") {
					return fail(KindInvalidInput, "SET_ENTRY_INVALID", dir, nil)
				}
				child := path.Join(dir, name)
				if entry.IsDir() {
					if entry.Type() != fs.ModeDir {
						return fail(KindInvalidInput, "SET_ENTRY_INVALID", child, nil)
					}
					dirs++
					if dirs > ReplayOperations()["evidence-replay"].Files {
						return fail(KindResourceLimit, "FILES_LIMIT", child, nil)
					}
					if e := walk(child, depth+1); e != nil {
						return e
					}
				} else {
					files++
					if files > ReplayOperations()["evidence-replay"].Files {
						return fail(KindResourceLimit, "FILES_LIMIT", child, nil)
					}
					if entry.Type() != 0 || !listed[child] {
						add("MEMBER_UNLISTED", child, "manifest에 없는 파일 또는 비정규 파일이다")
					}
				}
			}
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fail(KindIO, "SET_UNREADABLE", dir, err)
			}
		}
	}
	return walk(".", 0)
}
