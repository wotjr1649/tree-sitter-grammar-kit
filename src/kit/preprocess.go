package kit

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
)

// The opt-in C# symbol mode of #218 (218-B): a tsgk-incremental/r3 profile's preprocess
// member names the caller symbols, every step's source is preprocessed by csharpPreprocess
// and only its active ranges are parsed.
const (
	PreprocessMode          = "csharp-symbols/r1"
	PreprocessSymbolsSchema = "tsgk-pp-symbols/r1"
	PreprocessEvaluator     = "csharp-pp/r1" // csharpPreprocess with ppDefaultLimits
	NativePolicySchema      = "tsgk-native-policy/r1"
	NativePolicySchemaR2    = "tsgk-native-policy/r2" // r1 plus the preprocess binding
)

// Preprocess is a profile's preprocess member: the caller symbols exactly as given.
type Preprocess struct {
	Mode    string   `json:"mode"`
	Symbols []string `json:"symbols"`
}

// SymbolsDigest is the tsgk-pp-symbols/r1 digest of the symbols sorted by UTF-8 bytes and
// deduplicated: the prefix line, then per symbol its byte length in decimal, ':', its bytes
// and '\n'. The symbols keep their escapes and formatting characters.
func (p Preprocess) SymbolsDigest() string {
	s := slices.Clone(p.Symbols)
	slices.Sort(s)
	s = slices.Compact(s)
	var b bytes.Buffer
	b.WriteString(PreprocessSymbolsSchema + "\n")
	for _, x := range s {
		b.WriteString(strconv.Itoa(len(x)) + ":" + x + "\n")
	}
	return digestHex(b.Bytes())
}

// PreprocessDirective is one directive line of a step: its kind (Roslyn's directive trivia
// name without the suffix), the byte offset of its '#', whether it is active and, for #if,
// #elif and #else, whether its branch is taken.
type PreprocessDirective struct {
	Kind   string `json:"kind"`
	Hash   int    `json:"hash"`
	Active bool   `json:"active"`
	Taken  *bool  `json:"taken"`
}

// PreprocessStep is the observation of one step's source: the evaluator status and, for OK
// and ZERO_ACTIVE, the ordered directives, the active ranges the parser reads and the
// inactive (skipped) ranges. It claims directive positions and selection only; no group
// structure or directive spans.
type PreprocessStep struct {
	Status     string                `json:"status"`
	Directives []PreprocessDirective `json:"directives"`
	Active     []Span                `json:"active"`
	Inactive   []Span                `json:"inactive"`
}

// Preprocess statuses (csharpPreprocess).
const (
	PreprocessOK                  = ppOK
	PreprocessZeroActive          = ppZeroActive
	PreprocessRangeLimit          = ppRangeLimit
	PreprocessLimit               = ppLimit
	PreprocessMalformed           = ppMalformed
	PreprocessUnsupportedEncoding = ppUnsupportedEncoding
	PreprocessInvalidInput        = ppInvalidInput
)

// ObserveCSharpPreprocess evaluates one UTF-8 C# source under the caller symbols with the
// fixed evaluator limits (32 MiB, 4096 directives, depth 64, 16 active ranges, 256
// symbols). It reads src only and returns newly allocated slices; the status alone reports
// a refusal.
func ObserveCSharpPreprocess(src []byte, symbols []string) PreprocessStep {
	r := csharpPreprocess(src, EncodingUTF8, symbols, ppDefaultLimits)
	o := PreprocessStep{Status: r.Status, Directives: []PreprocessDirective{}, Active: []Span{}, Inactive: []Span{}}
	for _, d := range r.Directives {
		o.Directives = append(o.Directives, PreprocessDirective{Kind: d.Kind, Hash: d.Hash, Active: d.Active, Taken: d.Taken})
	}
	o.Active = append(o.Active, r.Active...)
	o.Inactive = append(o.Inactive, r.Inactive...)
	return o
}

// PreprocessOutcome is the case execution status and code of a step the evaluator did not
// pass (#218): no driver runs, the assessment is BLOCKED and every claim NOT_CLAIMED.
func PreprocessOutcome(status string) (execution, code string) {
	switch status {
	case PreprocessRangeLimit:
		return StatusResourceLimit, "PREPROCESS_RANGE_LIMIT"
	case PreprocessLimit:
		return StatusResourceLimit, "PREPROCESS_LIMIT"
	case PreprocessZeroActive:
		return StatusNotRun, "PREPROCESS_ZERO_ACTIVE"
	case PreprocessMalformed:
		return StatusNotRun, "PREPROCESS_MALFORMED"
	case PreprocessInvalidInput:
		return StatusNotRun, "PREPROCESS_INVALID_INPUT"
	}
	return StatusNotRun, "PREPROCESS_ENCODING_UNSUPPORTED" // the profile admits UTF-8 cases only
}

// PreprocessSteps evaluates the step versions in order up to the first that is not OK, as a
// symbol-mode run does.
func PreprocessSteps(versions [][]byte, pp *Preprocess) []PreprocessStep {
	out := []PreprocessStep{}
	for _, v := range versions {
		o := ObserveCSharpPreprocess(v, pp.Symbols)
		out = append(out, o)
		if o.Status != PreprocessOK {
			break
		}
	}
	return out
}

// preprocessSymbolsValid checks the caller symbols as the evaluator does (before any
// deduplication): the count, each name's validity and its spelling length.
func preprocessSymbolsValid(symbols []string) string {
	if len(symbols) > ppDefaultLimits.Symbols {
		return "PREPROCESS_SYMBOLS_LIMIT"
	}
	for _, s := range symbols {
		if _, ok := ppSymbol([]byte(s)); !ok || ppKeyword(s) {
			return "PREPROCESS_SYMBOL_INVALID"
		}
		if ppTooLong(s) {
			return "PREPROCESS_SYMBOLS_LIMIT"
		}
	}
	return ""
}

// NativePolicyText is the tsgk-native-policy/r1 preimage of an operation and output form:
// the operation limits, output, walls, edit and batch policy.
func NativePolicyText(op NativeOperation, output string) string {
	l := struct {
		InputBytes, Nodes, FullNodes, Depth, OutputBytes, ParseMillis, MemoryBytes, Errors, PartialNodes uint64
		Matches, Captures, QueryMillis                                                                   uint64
	}{op.InputBytes, op.Nodes, op.FullNodes, op.Depth, op.OutputBytes, op.ParseMillis, op.MemoryBytes, op.Errors, op.PartialNodes, op.Matches, op.Captures, op.QueryMillis}
	return fmt.Sprintf("%s\noperation=%s\noutput=%s\nlimits=%+v\nparse_wall=%s\nedit_wall=%s\nmax_edits=%d\nbatch=%t %d %d %s %s\nrun_wall=%s\n",
		NativePolicySchema, op.Name, output, l, op.ParseWall, op.EditWall, op.MaxEdits, op.Batch, op.BatchFiles, op.BatchBytes, op.FrameWall, op.BatchWall, op.RunWall)
}

// NativePolicy is the policy identity of a native run: tsgk-native-policy/r1, or with a
// preprocess member r2, which binds the r1 preimage, the mode, the symbols digest and the
// evaluator revision with its fixed limits.
func NativePolicy(op NativeOperation, output string, pp *Preprocess) IdentityRef {
	text := NativePolicyText(op, output)
	if pp == nil {
		return IdentityRef{Role: "policy", Schema: NativePolicySchema, SHA256: digestHex([]byte(text))}
	}
	text = fmt.Sprintf("%s\n%spreprocess_mode=%s\nsymbols=%s %s\nevaluator=%s limits=%+v\n", NativePolicySchemaR2, text, pp.Mode, PreprocessSymbolsSchema, pp.SymbolsDigest(), PreprocessEvaluator, ppDefaultLimits)
	return IdentityRef{Role: "policy", Schema: NativePolicySchemaR2, SHA256: digestHex([]byte(text))}
}
