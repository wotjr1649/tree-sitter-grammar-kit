package kit

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

const (
	PreprocessCSTSchema     = "tsgk-preprocess-cst/r1"
	PreprocessPrimarySchema = "tsgk-preprocess-primary/r1"
	PreprocessCSTPolicy     = "csharp-preprocess-cst/r1"
	PreprocessCSTPrefix     = "PREPROCESS_CST_MISMATCH_STEP_"
)

// PreprocessCSTStep compares directive hash-byte positions, not lexer or kind equivalence.
type PreprocessCSTStep struct {
	Step         int      `json:"step"`
	SourceSHA256 string   `json:"source_sha256"`
	SourceBytes  uint64   `json:"source_bytes"`
	Result       string   `json:"result"`
	Code         string   `json:"code"`
	HostHashes   []uint32 `json:"host_hashes"`
	CSTHashes    []uint32 `json:"cst_hashes"`
	HostOnly     []uint32 `json:"host_only"`
	CSTOnly      []uint32 `json:"cst_only"`
}

// PreprocessProcess is the bounded primary process receipt; it contains no process output.
type PreprocessProcess struct {
	Status            string `json:"status"`
	Reason            string `json:"reason"`
	ExitCode          int    `json:"exit_code"`
	CleanupVerified   bool   `json:"cleanup_verified"`
	CleanupScope      string `json:"cleanup_scope"`
	StdoutBytes       int64  `json:"stdout_bytes"`
	StdoutTruncated   bool   `json:"stdout_truncated"`
	WallMillis        int64  `json:"wall_ms"`
	WallBudgetMillis  int64  `json:"wall_budget_ms"`
	MemoryLimitBytes  uint64 `json:"memory_limit_bytes"`
	MemoryEnforcement string `json:"memory_enforcement"`
	PeakBytes         uint64 `json:"peak_bytes"`
	PeakKnown         bool   `json:"peak_known"`
	MemoryLimitHit    bool   `json:"memory_limit_hit"`
}

// PreprocessPrimary binds the structural request's policy, terminal state and supervision.
// Code is the response code when a frame exists, otherwise the pre-frame failure code.
type PreprocessPrimary struct {
	Schema          string             `json:"schema"`
	Role            string             `json:"role"`
	Policy          IdentityRef        `json:"policy"`
	ExecutionStatus string             `json:"execution_status"`
	Code            string             `json:"code"`
	Process         *PreprocessProcess `json:"process"`
}

// PreprocessCSTResult keeps the first mismatch prefix and a nonrecursive primary receipt.
type PreprocessCSTResult struct {
	Schema  string              `json:"schema"`
	Result  string              `json:"result"` // PASS | MISMATCH | UNVERIFIED | FAIL | NOT_APPLICABLE
	Code    string              `json:"code"`
	Steps   []PreprocessCSTStep `json:"steps"`
	Primary *PreprocessPrimary  `json:"primary"`
}

// PreprocessCSTCase is one replayed case's source-bound primary comparison.
type PreprocessCSTCase struct {
	ID     string              `json:"id"`
	Result PreprocessCSTResult `json:"result"`
}

// PreprocessCSTRequest supplies caller-owned source/history and execution identities.
// Raw is the JSON payload after the four-byte frame header. This data-only check cannot
// authenticate execution or prove that the reviewed producer omitted included ranges.
type PreprocessCSTRequest struct {
	Case           IncrementalCase
	Source         []byte
	Preprocess     *Preprocess
	Raw            []byte
	Primary        *PreprocessPrimary
	Operation      NativeOperation
	Platform       string
	RunID          string
	ProducerSHA256 string
}

// CheckPreprocessCST checks the entire primary response before comparing any positions.
// Later malformed evidence or equality failure always overrides an earlier mismatch.
func CheckPreprocessCST(r PreprocessCSTRequest) PreprocessCSTResult {
	out := PreprocessCSTResult{Schema: PreprocessCSTSchema, Result: "UNVERIFIED", Code: "PREPROCESS_CST_PRIMARY_UNVERIFIED", Steps: []PreprocessCSTStep{}, Primary: r.Primary}
	stop := func(result, code string) PreprocessCSTResult { out.Result, out.Code = result, code; return out }
	bad := func() PreprocessCSTResult { return stop("FAIL", "PREPROCESS_CST_PRIMARY_INVALID") }
	if r.Preprocess == nil {
		if r.Primary != nil || len(r.Raw) != 0 {
			return bad()
		}
		return stop("NOT_APPLICABLE", "")
	}
	reg, ok := NativeOperations()[r.Operation.Name]
	narrowed := r.Operation
	narrowed.RunWall = reg.RunWall
	if !ok || r.Operation.Name != "native-parse-edit" || !reflect.DeepEqual(narrowed, reg) || r.Operation.RunWall <= 0 || r.Operation.RunWall > reg.RunWall || len(r.Case.Edits) > reg.MaxEdits ||
		uint64(len(r.Source)) > reg.InputBytes || r.Case.Encoding != EncodingUTF8 || r.Preprocess.Mode != PreprocessMode || preprocessSymbolsValid(r.Preprocess.Symbols) != "" {
		return bad()
	}
	versions, points, err := ApplyEdits(r.Case.Encoding, r.Source, r.Case.Edits, r.Operation.InputBytes)
	if err != nil || len(versions) == 0 {
		if r.Primary != nil || len(r.Raw) != 0 {
			return bad()
		}
		return stop("NOT_APPLICABLE", asError(err).Code)
	}
	if digestHex(r.Source) != r.Case.Input.SHA256 || uint64(len(r.Source)) != r.Case.Input.Bytes {
		return bad()
	}
	host := PreprocessSteps(versions, r.Preprocess)
	if host[len(host)-1].Status != PreprocessOK {
		if r.Primary != nil || len(r.Raw) != 0 {
			return bad()
		}
		_, code := PreprocessOutcome(host[len(host)-1].Status)
		return stop("NOT_APPLICABLE", code)
	}
	p := r.Primary
	if p == nil || p.Schema != PreprocessPrimarySchema || p.Role != "primary-full-source" || p.Policy != NativePolicy(r.Operation, OutputTree, nil) {
		return bad()
	}
	if len(r.Raw) == 0 && p.Process == nil {
		if p.Code == "" || p.ExecutionStatus != StatusNotRun && p.ExecutionStatus != StatusFailed && p.ExecutionStatus != StatusResourceLimit {
			return bad()
		}
		return out
	}
	if uint64(len(r.Raw)) > r.Operation.OutputBytes || p.Process == nil || len(r.RunID) == 0 || len(r.RunID) > 128 || !hex64.MatchString(r.ProducerSHA256) {
		return bad()
	}
	proc := p.Process
	wall := r.Operation.ParseWall
	if len(r.Case.Edits) > 0 {
		wall = r.Operation.EditWall
	}
	wall = min(wall, r.Operation.RunWall)
	scope, memory := "", "HARD"
	switch r.Platform {
	case "windows/amd64":
		scope = "JOB_OBJECT"
	case "linux/amd64":
		scope = "CGROUP_KILL"
	case "darwin/arm64":
		scope, memory = "PROCESS_GROUP", "SAMPLED"
	default:
		return bad()
	}
	if proc.CleanupScope != scope || proc.StdoutBytes < 0 ||
		proc.WallMillis < 0 || proc.WallBudgetMillis <= 0 || proc.WallBudgetMillis > max(int64(1), (wall+time.Millisecond-1).Milliseconds()) || proc.WallMillis > proc.WallBudgetMillis+5001 ||
		proc.MemoryLimitBytes != r.Operation.MemoryBytes || proc.MemoryEnforcement != memory || !proc.PeakKnown && proc.PeakBytes != 0 || proc.PeakKnown && proc.PeakBytes > proc.MemoryLimitBytes && !proc.MemoryLimitHit {
		return bad()
	}
	if len(r.Raw) == 0 {
		if p.Code == "" || p.ExecutionStatus == StatusCompleted || proc.StdoutTruncated != (proc.StdoutBytes > int64(r.Operation.OutputBytes)+4) {
			return bad()
		}
		cleanupStop := !proc.CleanupVerified && p.ExecutionStatus == StatusFailed && p.Code == "PROCESS_CLEANUP_UNVERIFIED"
		switch proc.Status {
		case StatusResourceLimit:
			if !cleanupStop && (p.ExecutionStatus != StatusResourceLimit || p.Code != proc.Reason) || proc.Reason != "WALL_LIMIT" && proc.Reason != "OUTPUT_LIMIT" && proc.Reason != "MEMORY_LIMIT" || proc.Reason == "MEMORY_LIMIT" && !proc.MemoryLimitHit {
				return bad()
			}
		case StatusCancelled:
			if !cleanupStop && (p.ExecutionStatus != StatusCancelled || p.Code != "CANCELLED") || proc.Reason != "CANCELLED" || proc.MemoryLimitHit {
				return bad()
			}
		case StatusCompleted, StatusFailed:
			if p.ExecutionStatus != StatusFailed || proc.Reason != "EXITED" || (proc.ExitCode == 0) != (proc.Status == StatusCompleted) || proc.MemoryLimitHit {
				return bad()
			}
		default:
			return bad()
		}
		return out
	}
	if proc.StdoutBytes != int64(len(r.Raw))+4 || proc.StdoutTruncated {
		return bad()
	}
	response, err := nativewire.DecodeResponse(r.Raw)
	if err != nil {
		return bad()
	}
	e := nativewire.Expect{Revision: nativewire.ProtocolR3, ID: r.Case.ID, Nodes: r.Operation.Nodes, SourceBytes: uint64(len(r.Source)), EditPoints: points, Exit: &proc.ExitCode,
		Observation: &nativewire.Binding{Manifest: reuseobs.RuntimeManifestSHA256, RunID: r.RunID, ProducerSHA256: r.ProducerSHA256}}
	for _, v := range versions {
		e.Sources = append(e.Sources, nativewire.SourceID{Bytes: uint64(len(v)), SHA256: digestHex(v)})
	}
	for _, ed := range r.Case.Edits {
		e.Edits = append(e.Edits, nativewire.EditSpan{StartByte: ed.StartByte, OldEndByte: ed.OldEndByte, NewEndByte: ed.NewEndByte})
	}
	checked, err := nativewire.Check(response, e)
	if err != nil || p.ExecutionStatus != response.Status || p.Code != response.Code {
		return bad()
	}
	wantProcess := StatusCompleted
	if proc.ExitCode != 0 {
		wantProcess = StatusFailed
	}
	if proc.Status != wantProcess || proc.Reason != "EXITED" || proc.MemoryLimitHit {
		return bad()
	}
	if !proc.CleanupVerified {
		return out
	}
	if response.Status != StatusCompleted || len(checked.Steps) != len(versions) {
		return out
	}
	unknown := false
	for i, s := range checked.Steps {
		if reuseobs.Mandatory(s.Observations, true) == claimFail {
			return stop("FAIL", "PREPROCESS_CST_OBSERVATION_FAILED")
		}
		if reuseobs.Judge(s.Observations) != claimPass {
			unknown = true
		}
		if s.Incremental.Status != StatusCompleted || s.Incremental.Form != "full" || len(s.Incremental.Nodes) == 0 || i > 0 && (s.Fresh == nil || s.Fresh.Status != StatusCompleted || s.Fresh.Form != "full" || len(s.Fresh.Nodes) == 0) {
			unknown = true
			continue
		}
		if i > 0 && CompareTrees(s.Incremental.Nodes, s.Fresh.Nodes) != nil {
			return stop("FAIL", "PREPROCESS_CST_EQUALITY_FAILED")
		}
	}
	if unknown {
		return out
	}
	allPositions := make([][]uint32, len(checked.Steps))
	for i, s := range checked.Steps {
		nodes := s.Incremental.Nodes
		if i > 0 {
			nodes = s.Fresh.Nodes
		}
		positions, err := preprocessCSTHashes(versions[i], nodes)
		if err != nil {
			return bad()
		}
		allPositions[i] = positions
	}
	for i, positions := range allPositions {
		step := PreprocessCSTStep{Step: i, SourceSHA256: digestHex(versions[i]), SourceBytes: uint64(len(versions[i])), Result: "MATCH", HostHashes: []uint32{}, CSTHashes: positions, HostOnly: []uint32{}, CSTOnly: []uint32{}}
		for _, d := range host[i].Directives {
			step.HostHashes = append(step.HostHashes, uint32(d.Hash))
		}
		step.HostOnly = positionDifference(step.HostHashes, positions)
		step.CSTOnly = positionDifference(positions, step.HostHashes)
		if len(step.HostOnly)+len(step.CSTOnly) != 0 {
			step.Result, step.Code = "MISMATCH", fmt.Sprintf("%s%d", PreprocessCSTPrefix, i)
		}
		out.Steps = append(out.Steps, step)
		if step.Result == "MISMATCH" {
			return stop("MISMATCH", step.Code)
		}
	}
	return stop("PASS", "")
}

func preprocessCSTHashes(source []byte, nodes []TreeNode) ([]uint32, error) {
	positions := []uint32{}
	for _, n := range nodes {
		if n.IsMissing || n.EndByte <= n.StartByte {
			continue
		}
		prefix := ""
		switch n.Type {
		case "#if", "#elif", "#else", "#endif", "#define", "#undef", "#region", "#endregion", "#nullable", "#line", "#error", "#warning", "#pragma", "#r", "#load":
			prefix = strings.TrimPrefix(n.Type, "#")
		case "shebang_directive":
			prefix = "!"
		case "file_based_app_directive":
			prefix = ":"
		default:
			continue
		}
		if uint64(n.EndByte) > uint64(len(source)) || n.StartByte >= n.EndByte || source[n.StartByte] != '#' {
			return nil, fmt.Errorf("directive span")
		}
		tail := string(source[n.StartByte+1 : n.EndByte])
		if prefix != "!" && prefix != ":" {
			tail = strings.TrimLeftFunc(tail, ppSpace)
		}
		if !strings.HasPrefix(tail, prefix) {
			return nil, fmt.Errorf("directive prefix")
		}
		if len(tail) > len(prefix) && prefix != "!" && prefix != ":" {
			r, _ := utf8.DecodeRuneInString(tail[len(prefix):])
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
				return nil, fmt.Errorf("directive keyword")
			}
		}
		positions = append(positions, n.StartByte)
	}
	slices.Sort(positions)
	return slices.Compact(positions), nil
}

// Every CST member is required; only the two receipt pointers may explicitly be null.
// The strict tree rejects duplicate names/invalid Unicode before typed decoding.
func decodePreprocessCST(raw jsontext.Value) (*PreprocessCSTResult, error) {
	v, err := decodeStrict("preprocess_cst", raw, 16<<20)
	if err != nil {
		return nil, err
	}
	if !preprocessCSTShape(v, reflect.TypeFor[PreprocessCSTResult]()) {
		return nil, fmt.Errorf("preprocess_cst required member/type")
	}
	var out PreprocessCSTResult
	if err := jsonv2.Unmarshal(raw, &out, jsonv2.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	return &out, nil
}

func preprocessCSTShape(v *jv, t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		return v.kind == 'n' || preprocessCSTShape(v, t.Elem())
	}
	switch t.Kind() {
	case reflect.Struct:
		if v.kind != '{' || len(v.keys) != t.NumField() {
			return false
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			at := slices.Index(v.keys, name)
			if at < 0 || !preprocessCSTShape(v.vals[at], f.Type) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if v.kind != '[' {
			return false
		}
		for _, item := range v.vals {
			if !preprocessCSTShape(item, t.Elem()) {
				return false
			}
		}
		return true
	case reflect.String:
		return v.kind == '"'
	case reflect.Bool:
		return v.kind == 't' || v.kind == 'f'
	case reflect.Int, reflect.Int64, reflect.Uint32, reflect.Uint64:
		return v.kind == '0'
	}
	return false
}
