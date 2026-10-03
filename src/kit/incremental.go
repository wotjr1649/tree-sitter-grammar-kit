package kit

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"time"
)

// IncrementalSchema is the native incremental profile accepted by this build. Decoding is
// offline; building and running the driver belongs to the CLI's runner, never to this
// package.
const IncrementalSchema = "tsgk-incremental/r1"

// Native output forms (tsgk-native/r1).
const (
	OutputTree   = "tree"
	OutputAuto   = "auto"
	OutputRecord = "record"
)

// NativeOperation is one adopted native operation with its finite limits. Batch operations
// run up to BatchFiles cases or BatchBytes input bytes per driver process.
type NativeOperation struct {
	Name         string        `json:"name"`
	InputBytes   uint64        `json:"input_bytes"`
	Nodes        uint64        `json:"nodes"`
	FullNodes    uint64        `json:"full_nodes"`
	Depth        uint64        `json:"depth"`
	OutputBytes  uint64        `json:"output_bytes"`
	ParseMillis  uint64        `json:"parse_ms"`
	MemoryBytes  uint64        `json:"memory_bytes"`
	Errors       uint64        `json:"errors"`
	PartialNodes uint64        `json:"partial_nodes"`
	ParseWall    time.Duration `json:"parse_wall_ns"`
	EditWall     time.Duration `json:"edit_wall_ns"`
	MaxEdits     int           `json:"max_edits"`
	MaxCases     int           `json:"max_cases"`
	Outputs      []string      `json:"outputs"`
	Batch        bool          `json:"batch"`
	BatchFiles   int           `json:"batch_files,omitempty"`
	BatchBytes   uint64        `json:"batch_bytes,omitempty"`
	FrameWall    time.Duration `json:"frame_wall_ns,omitempty"`
	BatchWall    time.Duration `json:"batch_wall_ns,omitempty"`
	RunWall      time.Duration `json:"run_wall_ns"`
	Platforms    []string      `json:"platforms,omitempty"` // empty: every supported host
	Matches      uint64        `json:"matches,omitempty"`   // query operations (S06) only
	Captures     uint64        `json:"captures,omitempty"`
	QueryMillis  uint64        `json:"query_ms,omitempty"`
}

// RealWorldR3MemoryBytes is the windows/amd64 memory value of real-world-source-r3 and
// native-query-large (decision C1-REAL-WORLD-SOURCE-WINDOWS-R3): the smallest of 8, 10 or
// 12 GiB at least the measured cs-large-32mib-errors peak plus about 20 percent.
const RealWorldR3MemoryBytes = 8589934592

// R3PlatformReason is why the r3 large-input profile is qualified on windows/amd64 only.
const R3PlatformReason = "NET461 workload is Windows-hosted (WinForms/.NET Framework 4.6.1)"

// NativeOperations returns the adopted operations: the small registered edit cases
// (native-parse-edit), the real-world large-source profiles (real-world-source-r2 kept as
// S05 history, real-world-source-r3 windows/amd64 only), the private corpus batch
// (private-corpus-local, S05 wall 3600 s) and the S06 query operations (native-query and
// the windows/amd64 native-query-large).
func NativeOperations() map[string]NativeOperation {
	rw := NativeOperation{Name: "real-world-source-r2", InputBytes: 33554432, Nodes: 25000000, FullNodes: 50000, Depth: 100000, OutputBytes: 16777216,
		ParseMillis: 60000, MemoryBytes: 4294967296, Errors: 1000, PartialNodes: 1000, ParseWall: 90 * time.Second, EditWall: 300 * time.Second,
		MaxEdits: 4, MaxCases: 64, Outputs: []string{OutputTree, OutputAuto}, RunWall: 3600 * time.Second}
	r3 := rw
	r3.Name, r3.MemoryBytes, r3.Platforms = "real-world-source-r3", RealWorldR3MemoryBytes, []string{"windows/amd64"}
	pc := rw
	pc.Name, pc.Outputs, pc.MaxEdits, pc.MaxCases, pc.Batch = "private-corpus-local", []string{OutputRecord}, 0, 26000, true
	pc.BatchFiles, pc.BatchBytes, pc.FrameWall, pc.BatchWall = 500, 268435456, 60*time.Second, 3600*time.Second
	pe := NativeOperation{Name: "native-parse-edit", InputBytes: 65536, Nodes: 10000, FullNodes: 10000, Depth: 100000, OutputBytes: 8388608,
		ParseMillis: 10000, MemoryBytes: 4294967296, Errors: 1000, PartialNodes: 1000, ParseWall: 10 * time.Second, EditWall: 10 * time.Second,
		MaxEdits: 4, MaxCases: 1000, Outputs: []string{OutputTree}, RunWall: 3600 * time.Second}
	nq := pe
	nq.Name, nq.Matches, nq.Captures, nq.QueryMillis = "native-query", 10000, 10000, 10000
	ql := r3
	ql.Name, ql.MaxEdits, ql.Outputs, ql.Matches, ql.Captures, ql.QueryMillis = "native-query-large", 0, []string{OutputAuto}, 1000000, 1000000, 90000
	return map[string]NativeOperation{pe.Name: pe, rw.Name: rw, r3.Name: r3, pc.Name: pc, nq.Name: nq, ql.Name: ql}
}

// Build ceilings of the c-build operation.
const (
	BuildFiles     = 64
	BuildFileBytes = 104857600
	BuildBytes     = 134217728
)

// NativeInput is one declared file below the profile root.
type NativeInput struct {
	Path   string `json:"path"`
	Role   string `json:"role"`
	SHA256 string `json:"sha256"`
	Bytes  uint64 `json:"bytes"`
}

// NativeDeclaration is one declaration-facts mapping entry passed to the driver.
type NativeDeclaration struct {
	Fact string `json:"fact"`
	Node string `json:"node"`
	Name string `json:"name"`
}

// Declarations binds the mapping revision of the entries.
type Declarations struct {
	Mapping string              `json:"mapping"`
	Items   []NativeDeclaration `json:"items"`
}

// NativePoint is a registered byte of a partial tree.
type NativePoint struct {
	ID   string `json:"id"`
	Byte uint32 `json:"byte"`
}

// StepExpectation is an independent language expectation for one step: Syntax NO_ERROR,
// ERROR or ANY on the root, named node types that must appear (full trees), and the
// declaration assessment ("" means not expected).
type StepExpectation struct {
	Step         int      `json:"step"`
	Syntax       string   `json:"syntax"`
	Contains     []string `json:"contains"`
	Declarations string   `json:"declarations"`
}

// IncrementalCase is one registered input with its edit sequence.
type IncrementalCase struct {
	ID       string            `json:"id"`
	Input    NativeInput       `json:"input"`
	Encoding string            `json:"encoding"`
	Edits    []Edit            `json:"edits"`
	Points   []NativePoint     `json:"points"`
	Expect   []StepExpectation `json:"expect"`
}

// IncrementalProfile is a decoded tsgk-incremental/r1 profile.
type IncrementalProfile struct {
	SHA256       string            `json:"-"`
	ID           string            `json:"id"`
	Route        string            `json:"route"`
	Operation    string            `json:"operation"`
	Symbol       string            `json:"symbol"`
	Encoding     string            `json:"encoding"`
	Output       string            `json:"output"`
	Format       string            `json:"format,omitempty"` // "" or SVC-SERVICEHOST-r1 (inline C# by included range)
	Compiler     ToolIdentity      `json:"compiler"`
	Grammar      []NativeInput     `json:"grammar"`
	Declarations *Declarations     `json:"declarations"`
	Cases        []IncrementalCase `json:"cases"`
}

var symbolPattern = regexp.MustCompile(`^tree_sitter_[a-z0-9_]{1,64}$`)

// ValidLanguageSymbol reports whether s is the restricted grammar entry symbol form the
// shim accepts; nothing else is ever written into generated C.
func ValidLanguageSymbol(s string) bool { return symbolPattern.MatchString(s) }

// ParseIncrementalProfile strictly decodes a tsgk-incremental/r1 profile.
func ParseIncrementalProfile(data []byte) (IncrementalProfile, error) {
	p, e := parseIncremental(data)
	if e != nil {
		return IncrementalProfile{}, e
	}
	return p, nil
}

func nativeText(t typed, v *jv, code string) (string, *Error) {
	s, e := t.str(v)
	if e != nil {
		return "", e
	}
	if s == "" || len(s) > 128 {
		return "", t.bad(code, v)
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e || s[i] == '"' || s[i] == '\\' {
			return "", t.bad(code, v)
		}
	}
	return s, nil
}

func strictBase64(t typed, v *jv) ([]byte, *Error) {
	s, e := t.str(v)
	if e != nil {
		return nil, e
	}
	b, err := base64.StdEncoding.Strict().DecodeString(s)
	if err != nil || base64.StdEncoding.EncodeToString(b) != s {
		return nil, t.bad("BASE64_INVALID", v)
	}
	return b, nil
}

func nativeInput(t typed, v *jv, roles map[string]bool) (NativeInput, *Error) {
	var in NativeInput
	m, e := t.object(v, []string{"path", "role", "sha256", "bytes"})
	if e != nil {
		return in, e
	}
	if in.Path, e = t.str(m["path"]); e != nil {
		return in, e
	} else if !portable(in.Path, false) {
		return in, t.bad("SELECTION_INVALID", m["path"])
	}
	if in.Role, e = t.str(m["role"]); e != nil {
		return in, e
	} else if !roles[in.Role] {
		return in, t.bad("ROLE_INVALID", m["role"])
	}
	if in.SHA256, e = hexDigest(t, m["sha256"]); e != nil {
		return in, e
	}
	in.Bytes, e = t.uint(m["bytes"])
	return in, e
}

func parseIncremental(data []byte) (IncrementalProfile, *Error) {
	p, _, e := parseNative(data, IncrementalSchema, "incremental", nil, nil)
	return p, e
}

// nativeExtras are the members a profile revision adds to the shared native profile: the
// top-level ones and, per case, the case-level ones.
type nativeExtras struct {
	t     typed
	top   map[string]*jv
	cases []map[string]*jv
}

// parseNative decodes the members shared by tsgk-incremental/r1 and tsgk-oracle/r1; extra
// top-level and case-level members are required and returned undecoded. A query operation
// (S06) is accepted only by a revision with queries, and the other operations only without.
func parseNative(data []byte, schema, doc string, topExtra, caseExtra []string) (IncrementalProfile, nativeExtras, *Error) {
	var p IncrementalProfile
	t := typed{doc: doc}
	x := nativeExtras{t: t, top: map[string]*jv{}}
	v, e := decodeStrict(t.doc, data, MaxDocumentBytes)
	if e != nil {
		return p, x, e
	}
	m, e := t.object(v, append([]string{"schema", "id", "route", "operation", "symbol", "encoding", "output", "compiler", "grammar", "declarations", "cases"}, topExtra...), "format")
	if e != nil {
		return p, x, e
	}
	for _, k := range topExtra {
		x.top[k] = m[k]
	}
	if s, e := t.str(m["schema"]); e != nil {
		return p, x, e
	} else if s != schema {
		return p, x, t.bad("SCHEMA_UNSUPPORTED", m["schema"])
	}
	sum := sha256.Sum256(data)
	p.SHA256 = hex.EncodeToString(sum[:])
	for _, f := range []struct {
		dst  *string
		name string
	}{{&p.ID, "id"}, {&p.Route, "route"}} {
		s, e := t.str(m[f.name])
		if e != nil {
			return p, x, e
		}
		if !validID(s) {
			return p, x, t.bad(map[string]string{"id": "ID_INVALID", "route": "ROUTE_INVALID"}[f.name], m[f.name])
		}
		*f.dst = s
	}
	ops := NativeOperations()
	if p.Operation, e = t.str(m["operation"]); e != nil {
		return p, x, e
	}
	op, ok := ops[p.Operation]
	if !ok || (op.Matches > 0) != (schema == OracleSchema) {
		return p, x, t.bad("OPERATION_UNSUPPORTED", m["operation"])
	}
	if p.Symbol, e = t.str(m["symbol"]); e != nil {
		return p, x, e
	} else if !ValidLanguageSymbol(p.Symbol) {
		return p, x, t.bad("SYMBOL_INVALID", m["symbol"])
	}
	if p.Encoding, e = t.str(m["encoding"]); e != nil {
		return p, x, e
	} else if !encodingNames[p.Encoding] {
		return p, x, t.bad("ENCODING_UNSUPPORTED", m["encoding"])
	}
	if p.Output, e = t.str(m["output"]); e != nil {
		return p, x, e
	}
	allowed := false
	for _, o := range op.Outputs {
		allowed = allowed || o == p.Output
	}
	if !allowed {
		return p, x, t.bad("OUTPUT_NOT_ALLOWED", m["output"])
	}
	if f := m["format"]; f != nil {
		if p.Format, e = t.str(f); e != nil {
			return p, x, e
		} else if p.Format != SvcFormat || p.Symbol != "tree_sitter_c_sharp" {
			return p, x, t.bad("FORMAT_UNSUPPORTED", f)
		}
	}
	if p.Compiler, e = parseTool(t, m["compiler"]); e != nil {
		return p, x, e
	}
	if p.Grammar, e = parseGrammarInputs(t, m["grammar"]); e != nil {
		return p, x, e
	}
	if dv := m["declarations"]; dv.kind != 'n' {
		d, e := parseDeclarations(t, dv)
		if e != nil {
			return p, x, e
		}
		p.Declarations = d
	}
	list, e := t.array(m["cases"])
	if e != nil {
		return p, x, e
	}
	if len(list) == 0 || len(list) > op.MaxCases {
		return p, x, t.bad("CASES_COUNT_INVALID", m["cases"])
	}
	seen := map[string]bool{}
	for _, item := range list {
		c, cm, e := parseCase(t, item, p, op, caseExtra)
		if e != nil {
			return p, x, e
		}
		x.cases = append(x.cases, cm)
		if seen[c.ID] {
			return p, x, t.bad("CASE_DUPLICATE", item)
		}
		seen[c.ID] = true
		p.Cases = append(p.Cases, c)
	}
	return p, x, nil
}

func parseGrammarInputs(t typed, v *jv) ([]NativeInput, *Error) {
	list, e := t.array(v)
	if e != nil {
		return nil, e
	}
	if len(list) == 0 || len(list) > BuildFiles {
		return nil, t.bad("GRAMMAR_FILES_INVALID", v)
	}
	var out []NativeInput
	var paths []*jv
	var total uint64
	parsers := 0
	for _, item := range list {
		in, e := nativeInput(t, item, map[string]bool{"parser": true, "scanner": true, "header": true})
		if e != nil {
			return nil, e
		}
		if in.Bytes > BuildFileBytes {
			return nil, t.bad("FILE_BYTES_LIMIT", item)
		}
		if total += in.Bytes; total > BuildBytes {
			return nil, t.bad("INPUT_BYTES_LIMIT", item)
		}
		if in.Role == "parser" {
			parsers++
		}
		out = append(out, in)
		m, _ := t.object(item, []string{"path", "role", "sha256", "bytes"})
		paths = append(paths, m["path"])
	}
	if parsers != 1 {
		return nil, t.bad("GRAMMAR_PARSER_COUNT", v)
	}
	if e := sortedUnique(t, "SELECTION", paths); e != nil {
		return nil, e
	}
	return out, nil
}

func parseDeclarations(t typed, v *jv) (*Declarations, *Error) {
	m, e := t.object(v, []string{"mapping", "items"})
	if e != nil {
		return nil, e
	}
	d := &Declarations{}
	if d.Mapping, e = t.str(m["mapping"]); e != nil {
		return nil, e
	} else if !validID(d.Mapping) {
		return nil, t.bad("MAPPING_INVALID", m["mapping"])
	}
	list, e := t.array(m["items"])
	if e != nil {
		return nil, e
	}
	if len(list) == 0 || len(list) > 64 {
		return nil, t.bad("DECLARATIONS_COUNT_INVALID", m["items"])
	}
	for _, item := range list {
		f, e := t.object(item, []string{"fact", "node", "name"})
		if e != nil {
			return nil, e
		}
		var nd NativeDeclaration
		for _, x := range []struct {
			dst  *string
			name string
		}{{&nd.Fact, "fact"}, {&nd.Node, "node"}, {&nd.Name, "name"}} {
			if *x.dst, e = nativeText(t, f[x.name], "DECLARATION_INVALID"); e != nil {
				return nil, e
			}
		}
		if !validLocator(nd.Name) {
			return nil, t.bad("LOCATOR_INVALID", f["name"])
		}
		d.Items = append(d.Items, nd)
	}
	return d, nil
}

var locatorSegment = regexp.MustCompile(`^(field|child|children):[^:/]+$`)

func validLocator(s string) bool {
	if s == "node" {
		return true
	}
	for _, seg := range splitPath(s) {
		if !locatorSegment.MatchString(seg) {
			return false
		}
	}
	return true
}

func splitPath(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '/' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
}

func parseCase(t typed, v *jv, p IncrementalProfile, op NativeOperation, extra []string) (IncrementalCase, map[string]*jv, *Error) {
	var c IncrementalCase
	m, e := t.object(v, append([]string{"id", "input", "edits", "points", "expect"}, extra...), "encoding")
	if e != nil {
		return c, nil, e
	}
	if c.ID, e = t.str(m["id"]); e != nil {
		return c, nil, e
	} else if !validID(c.ID) {
		return c, nil, t.bad("CASE_ID_INVALID", m["id"])
	}
	if c.Input, e = nativeInput(t, m["input"], map[string]bool{"case": true}); e != nil {
		return c, nil, e
	}
	if c.Input.Bytes > op.InputBytes {
		return c, nil, t.bad("INPUT_TOO_LARGE", m["input"])
	}
	c.Encoding = p.Encoding
	if x := m["encoding"]; x != nil {
		if c.Encoding, e = t.str(x); e != nil {
			return c, nil, e
		} else if !encodingNames[c.Encoding] {
			return c, nil, t.bad("ENCODING_UNSUPPORTED", x)
		}
	}
	edits, e := t.array(m["edits"])
	if e != nil {
		return c, nil, e
	}
	if len(edits) > op.MaxEdits {
		return c, nil, t.bad("EDIT_COUNT_LIMIT", m["edits"])
	}
	if len(edits) > 0 && p.Output != OutputTree {
		return c, nil, t.bad("EDITS_NOT_ALLOWED", m["edits"])
	}
	for _, item := range edits {
		f, e := t.object(item, []string{"start_byte", "old_end_byte", "new_end_byte", "old", "new"})
		if e != nil {
			return c, nil, e
		}
		var ed Edit
		for _, x := range []struct {
			dst  *uint32
			name string
		}{{&ed.StartByte, "start_byte"}, {&ed.OldEndByte, "old_end_byte"}, {&ed.NewEndByte, "new_end_byte"}} {
			n, e := t.uint(f[x.name])
			if e != nil {
				return c, nil, e
			}
			if n > 0xffffffff {
				return c, nil, t.bad("EDIT_RANGE", f[x.name])
			}
			*x.dst = uint32(n)
		}
		if ed.Old, e = strictBase64(t, f["old"]); e != nil {
			return c, nil, e
		}
		if ed.New, e = strictBase64(t, f["new"]); e != nil {
			return c, nil, e
		}
		c.Edits = append(c.Edits, ed)
	}
	points, e := t.array(m["points"])
	if e != nil {
		return c, nil, e
	}
	if len(points) > 64 {
		return c, nil, t.bad("POINTS_COUNT_INVALID", m["points"])
	}
	for _, item := range points {
		f, e := t.object(item, []string{"id", "byte"})
		if e != nil {
			return c, nil, e
		}
		var pt NativePoint
		if pt.ID, e = t.str(f["id"]); e != nil {
			return c, nil, e
		} else if !validID(pt.ID) {
			return c, nil, t.bad("POINT_ID_INVALID", f["id"])
		}
		n, e := t.uint(f["byte"])
		if e != nil {
			return c, nil, e
		}
		if n > 0xffffffff {
			return c, nil, t.bad("POINT_RANGE", f["byte"])
		}
		pt.Byte = uint32(n)
		c.Points = append(c.Points, pt)
	}
	expect, e := t.array(m["expect"])
	if e != nil {
		return c, nil, e
	}
	for _, item := range expect {
		f, e := t.object(item, []string{"step", "syntax", "contains", "declarations"})
		if e != nil {
			return c, nil, e
		}
		var x StepExpectation
		n, e := t.uint(f["step"])
		if e != nil {
			return c, nil, e
		}
		if n > uint64(len(c.Edits)) {
			return c, nil, t.bad("EXPECT_STEP_INVALID", f["step"])
		}
		x.Step = int(n)
		if x.Syntax, e = t.str(f["syntax"]); e != nil {
			return c, nil, e
		} else if x.Syntax != "NO_ERROR" && x.Syntax != "ERROR" && x.Syntax != "ANY" {
			return c, nil, t.bad("EXPECT_SYNTAX_INVALID", f["syntax"])
		}
		types, e := t.array(f["contains"])
		if e != nil {
			return c, nil, e
		}
		for _, ty := range types {
			s, e := nativeText(t, ty, "EXPECT_TYPE_INVALID")
			if e != nil {
				return c, nil, e
			}
			x.Contains = append(x.Contains, s)
		}
		if x.Declarations, e = t.str(f["declarations"]); e != nil {
			return c, nil, e
		}
		switch x.Declarations {
		case "", AssessPass, AssessFail, "NOT_APPLICABLE":
		default:
			return c, nil, t.bad("EXPECT_DECLARATIONS_INVALID", f["declarations"])
		}
		if x.Declarations != "" && p.Declarations == nil {
			return c, nil, t.bad("EXPECT_DECLARATIONS_UNMAPPED", f["declarations"])
		}
		c.Expect = append(c.Expect, x)
	}
	return c, m, nil
}
