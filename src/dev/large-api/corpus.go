package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// These development evidence ceilings do not change any registered native operation.
const corpusStorageBytes = 32 << 30

type pinnedFile struct {
	Path  string `json:"path"`
	SHA   string `json:"sha256"`
	Bytes uint64 `json:"bytes"`
}

type corpusExpected struct {
	ID       string          `json:"id"`
	Route    string          `json:"route"`
	Encoding string          `json:"encoding"`
	Input    kit.NativeInput `json:"input"`
	Nodes    uint64          `json:"nodes"`
	Digest   string          `json:"digest"`
	HasError *bool           `json:"has_error"`
}

type corpusPlan struct {
	Schema    string           `json:"schema"`
	Candidate string           `json:"candidate"`
	Profiles  []pinnedFile     `json:"profiles"`
	Expected  []corpusExpected `json:"expected"`
	Storage   uint64           `json:"storage_limit,omitempty"`
	Wall      uint64           `json:"wall_seconds,omitempty"`
}

func corpusEnvelope(p corpusPlan) (storage, wall uint64, err error) {
	storage, wall = p.Storage, p.Wall
	if storage == 0 {
		storage = corpusStorageBytes
	}
	if wall == 0 {
		wall = 3600
	}
	if storage > corpusStorageBytes || wall > 3600 {
		err = errors.New("CORPUS_ENVELOPE_CANNOT_RAISE_LIMIT")
	}
	return
}

type cstPage struct {
	Types  []string  `json:"types"`
	Fields []string  `json:"fields"`
	Nodes  [][]int64 `json:"nodes"`
}

type corpusPage struct {
	receipt
	CST      cstPage `json:"cst_page"`
	Encoding string  `json:"encoding"`
	HasError bool    `json:"has_error"`
}

// readPinned never reads past a declared input ceiling, follows internal links, or
// accepts bytes different from the pinned input. It does not promise hostile TOCTOU isolation.
func readPinned(root *os.Root, p pinnedFile, limit uint64) ([]byte, error) {
	if !fs.ValidPath(p.Path) || strings.ContainsAny(p.Path, `\:`) || p.Bytes > limit || len(p.SHA) != 64 {
		return nil, errors.New("PINNED_INPUT_INVALID_OR_OVER_LIMIT")
	}
	parts := strings.Split(p.Path, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil || (i < len(parts)-1 && !info.IsDir()) || (i == len(parts)-1 && !info.Mode().IsRegular()) || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("PINNED_INPUT_LINK_OR_UNREADABLE")
		}
	}
	f, err := root.Open(p.Path)
	if err != nil {
		return nil, errors.New("PINNED_INPUT_OPEN_FAILED")
	}
	defer f.Close()
	if err = corpusCheckLinks(f); err != nil {
		return nil, err
	}
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() < 0 || uint64(before.Size()) != p.Bytes {
		return nil, errors.New("PINNED_INPUT_SIZE_MISMATCH")
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(p.Bytes)+1))
	if err != nil || uint64(len(b)) != p.Bytes || digest(b) != p.SHA {
		return nil, errors.New("PINNED_INPUT_IDENTITY_MISMATCH")
	}
	after, err := f.Stat()
	if err != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, errors.New("PINNED_INPUT_CHANGED")
	}
	return b, nil
}

func appendTreeDigest(h hash.Hash, n kit.TreeNode) {
	field := ""
	if n.Field != nil {
		field = *n.Field
	}
	b := strconv.AppendInt(nil, n.Parent, 10)
	for _, s := range []string{n.Type, field} {
		b = append(b, 0)
		b = strconv.AppendInt(b, int64(len(s)), 10)
		b = append(b, ':')
		b = append(b, s...)
	}
	for _, f := range []bool{n.Named, n.Extra, n.IsError, n.HasError, n.IsMissing} {
		v := byte('0')
		if f {
			v = '1'
		}
		b = append(b, 0, v)
	}
	for _, v := range []uint32{n.StartByte, n.EndByte, n.StartPoint.Row, n.StartPoint.Column, n.EndPoint.Row, n.EndPoint.Column} {
		b = append(b, 0)
		b = strconv.AppendUint(b, uint64(v), 10)
	}
	h.Write(append(b, '\n'))
}

type corpusTree struct {
	h         hash.Hash
	stack     []int64
	next      uint64
	children  uint64
	checks    uint64
	fields    *uint64
	rootError bool
}

func newCorpusTree() *corpusTree {
	h := sha256.New()
	h.Write([]byte("tsgk-tree-digest/r1\n"))
	return &corpusTree{h: h}
}

func decodeCorpusPage(data []byte, req native.Request, start, end, nodes uint64) (corpusPage, error) {
	var p corpusPage
	b, err := native.SplitFrame(data)
	if err != nil {
		return p, err
	}
	if err = jsonv2.Unmarshal(b, &p, jsonv2.RejectUnknownMembers(true)); err != nil {
		return p, err
	}
	var members map[string]json.RawMessage
	if err = jsonv2.Unmarshal(b, &members); err != nil {
		return p, err
	}
	for _, key := range []string{"cst_page", "encoding", "has_error"} {
		if _, ok := members[key]; !ok {
			return p, errors.New("CST_PAGE_MEMBER_MISSING")
		}
	}
	if p.Schema != "tsgk-corpus-cst-api-page/r1" {
		return p, errors.New("CST_PAGE_SCHEMA_INVALID")
	}
	baseReceipt := p.receipt
	baseReceipt.Schema = "tsgk-large-api-audit/r1"
	base, err := json.Marshal(baseReceipt)
	if err != nil {
		return p, err
	}
	if _, err = decode(native.Frame(base), req, start, end, nodes); err != nil {
		return p, err
	}
	if p.Encoding != req.Encoding || uint64(len(p.CST.Nodes)) != p.End-p.Start || p.End-p.Start > req.Limits.FullNodes || len(p.CST.Types) == 0 {
		return p, errors.New("CST_PAGE_ENCODING_OR_COUNT_INVALID")
	}
	return p, nil
}

func (t *corpusTree) consume(p corpusPage, inputBytes uint64) error {
	if p.Start != t.next || p.Differences != 0 || p.Divergences != 0 {
		return errors.New("CST_API_GAP_OR_DIFFERENCE")
	}
	if t.fields == nil {
		f := p.Fields
		t.fields = &f
	} else if *t.fields != p.Fields {
		return errors.New("FIELD_CATALOG_DRIFT")
	}
	for offset, row := range p.CST.Nodes {
		if len(row) != 10 {
			return errors.New("CST_ROW_SHAPE_INVALID")
		}
		i := p.Start + uint64(offset)
		if row[1] < 0 || row[1] >= int64(len(p.CST.Types)) || row[2] < -1 || row[2] >= int64(len(p.CST.Fields)) || row[3] < 0 || row[3] > 31 {
			return errors.New("CST_CATALOG_OR_FLAGS_INVALID")
		}
		for _, v := range row[4:] {
			if v < 0 || v > 0xffffffff {
				return errors.New("CST_COORDINATE_INVALID")
			}
		}
		n := kit.TreeNode{Parent: row[0], Type: p.CST.Types[row[1]], Named: row[3]&1 != 0, Extra: row[3]&2 != 0, IsError: row[3]&4 != 0, HasError: row[3]&8 != 0, IsMissing: row[3]&16 != 0, StartByte: uint32(row[4]), EndByte: uint32(row[5]), StartPoint: kit.Point{Row: uint32(row[6]), Column: uint32(row[7])}, EndPoint: kit.Point{Row: uint32(row[8]), Column: uint32(row[9])}}
		if row[2] >= 0 {
			f := p.CST.Fields[row[2]]
			n.Field = &f
		}
		if (i == 0 && n.Parent != -1) || (i > 0 && (n.Parent < 0 || uint64(n.Parent) >= i)) || (n.Field != nil && (i == 0 || *n.Field == "")) || n.Type == "" {
			return errors.New("CST_PARENT_OR_FIELD_INVALID")
		}
		for len(t.stack) > 0 && t.stack[len(t.stack)-1] != n.Parent {
			t.stack = t.stack[:len(t.stack)-1]
		}
		if i > 0 && len(t.stack) == 0 {
			return errors.New("CST_PREORDER_INVALID")
		}
		t.stack = append(t.stack, int64(i))
		if uint64(len(t.stack)) > 100000 || n.StartByte > n.EndByte || uint64(n.EndByte) > inputBytes || n.StartPoint.Row > n.EndPoint.Row || (n.StartPoint.Row == n.EndPoint.Row && n.StartPoint.Column > n.EndPoint.Column) {
			return errors.New("CST_RANGE_POINT_OR_DEPTH_INVALID")
		}
		if i == 0 {
			t.rootError = n.HasError
		}
		appendTreeDigest(t.h, n)
	}
	if t.rootError != p.HasError {
		return errors.New("CST_ROOT_FLAG_MISMATCH")
	}
	t.next = p.End
	t.children += p.ChildFields
	t.checks += p.Checks
	return nil
}

type corpusBinding struct {
	ID       string          `json:"id"`
	Route    string          `json:"route"`
	Encoding string          `json:"encoding"`
	Input    kit.NativeInput `json:"input"`
	Producer string          `json:"producer"`
	Policy   string          `json:"policy"`
	Page     pinnedFile      `json:"page"`
	Start    uint64          `json:"start"`
	End      uint64          `json:"end"`
	Nodes    uint64          `json:"nodes"`
	Process  runner.Result   `json:"process"`
}

type corpusProducer struct {
	Route         string             `json:"route"`
	Build         *native.Build      `json:"build"`
	AuditSHA      string             `json:"audit_sha256"`
	WrapperSHA    string             `json:"wrapper_sha256"`
	ExecutableSHA string             `json:"executable_sha256"`
	Compile       []native.BuildStep `json:"compile"`
}

func corpusRegisteredProfile(p kit.OracleProfile) error {
	data, err := readLocalDocument("src/contracts/native-routes.json", 16777216)
	if err != nil {
		return err
	}
	var registry struct {
		Routes []struct {
			Route  string            `json:"route"`
			Symbol string            `json:"symbol"`
			Files  []kit.NativeInput `json:"files"`
		} `json:"routes"`
	}
	if err = json.Unmarshal(data, &registry); err != nil {
		return err
	}
	for _, r := range registry.Routes {
		if r.Route != p.Native.Route {
			continue
		}
		slices.SortFunc(r.Files, func(a, b kit.NativeInput) int { return strings.Compare(a.Path, b.Path) })
		if r.Symbol != p.Native.Symbol || !slices.Equal(r.Files, p.Native.Grammar) {
			return errors.New("CORPUS_GRAMMAR_NOT_REGISTERED")
		}
		return nil
	}
	return errors.New("CORPUS_ROUTE_NOT_REGISTERED")
}

type corpusResult struct {
	Schema        string                    `json:"schema"`
	PlanSHA       string                    `json:"plan_sha256"`
	Candidate     string                    `json:"candidate"`
	Policy        string                    `json:"policy_sha256"`
	HostSHA       string                    `json:"host_executable_sha256"`
	StorageLimit  uint64                    `json:"storage_limit"`
	Expected      int                       `json:"expected"`
	Completed     int                       `json:"completed"`
	API           int                       `json:"api_cases"`
	SvcNotClaimed int                       `json:"svc_api_not_claimed"`
	Nodes         uint64                    `json:"nodes"`
	Pages         int                       `json:"pages"`
	Bytes         uint64                    `json:"evidence_bytes_before_result"`
	Checks        uint64                    `json:"checks"`
	Producers     map[string]corpusProducer `json:"producers"`
	BindingsSHA   string                    `json:"bindings_sha256"`
	WallMS        int64                     `json:"wall_ms"`
	WallLimit     uint64                    `json:"wall_seconds"`
	Complete      bool                      `json:"complete"`
	Error         string                    `json:"error,omitempty"`
}

func corpusPolicySHA(storage, wall uint64) string {
	op := kit.NativeOperations()["native-query-large"]
	text := fmt.Sprintf("tsgk-corpus-cst-api-policy/r1\n%s\npage_nodes=%d\nstorage=%d\nprofiles=67108864\nwall=%d\npoints=0,EOF\ncomparator=large-api-r1\nCST=tsgk-tree-digest/r1\n", native.PolicyRef(op, kit.OutputAuto).SHA256, op.FullNodes, storage, wall)
	return digest([]byte(text))
}

func writeExclusive(name string, b []byte) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	cerr := f.Close()
	return errors.Join(err, cerr)
}

type corpusQuota struct{ used, limit uint64 }
type corpusQuotaWriter struct {
	quota  *corpusQuota
	writer io.Writer
}

func (w corpusQuotaWriter) Write(b []byte) (int, error) {
	if uint64(len(b)) > w.quota.limit-w.quota.used {
		return 0, errors.New("CORPUS_STORAGE_LIMIT")
	}
	n, err := w.writer.Write(b)
	w.quota.used += uint64(n)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	return n, err
}
func (q *corpusQuota) put(name string, b []byte) error {
	if uint64(len(b)) > q.limit-q.used {
		return errors.New("CORPUS_STORAGE_LIMIT")
	}
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = (corpusQuotaWriter{q, f}).Write(b)
	return errors.Join(err, f.Close())
}

func corpusPageRun(ctx context.Context, exe, sha, dir string, req native.Request, start, end, nodes uint64) (corpusPage, runner.Result, error) {
	var p corpusPage
	b, err := os.ReadFile(exe)
	if err != nil || digest(b) != sha {
		return p, runner.Result{}, errors.New("AUDIT_EXECUTABLE_CHANGED")
	}
	op := kit.NativeOperations()["native-query-large"]
	frame := native.Frame(req.Encode())
	if len(frame) > native.MaxRequestFrame+4 {
		return p, runner.Result{}, errors.New("REQUEST_FRAME_LIMIT")
	}
	r, err := runner.Run(ctx, runner.Spec{Path: exe, Args: []string{fmt.Sprint(start), fmt.Sprint(end)}, Dir: dir, Stdin: frame, StdoutBytes: int64(op.OutputBytes) + 4, StderrBytes: 65536, Wall: op.ParseWall, Grace: 5 * time.Second, Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: true}})
	if err != nil {
		return p, r, errors.New("AUDIT_RUN_REFUSED")
	}
	b, err = os.ReadFile(exe)
	if err != nil || digest(b) != sha {
		return p, r, errors.New("AUDIT_EXECUTABLE_CHANGED")
	}
	if !validCorpusProcess(r, uint64(len(r.Stdout))) {
		return p, r, errors.New("AUDIT_PROCESS_OR_RESOURCE_FAILED")
	}
	p, err = decodeCorpusPage(r.Stdout, req, start, end, nodes)
	return p, r, err
}

func validCorpusProcess(r runner.Result, bytes uint64) bool {
	op := kit.NativeOperations()["native-query-large"]
	return r.Succeeded() && r.Reason == runner.ReasonExited && r.Cleanup.Scope == "JOB_OBJECT" && r.Cleanup.ResidualAfterExit == 0 && !r.StdoutTruncated && !r.StderrTruncated && r.StdoutBytes >= 0 && uint64(r.StdoutBytes) == bytes && r.StderrBytes >= 0 && r.StderrBytes <= 65536 && r.WallMillis >= 0 && r.WallMillis <= op.ParseWall.Milliseconds()+5000 && r.Memory.Enforcement == runner.MemoryHard && r.Memory.Limit == op.MemoryBytes && r.Memory.PeakKnown && !r.Memory.LimitHit && r.Memory.Peak <= r.Memory.Limit
}

func executeCorpus(prepared, cc, work, out, rootName, planName string) (returnErr error) {
	start := time.Now()
	if rootName == "" {
		return errors.New("CORPUS_ROOT_REQUIRED")
	}
	planBytes, err := readLocalDocument(planName, 16777216)
	if err != nil {
		return err
	}
	var plan corpusPlan
	if err = jsonv2.Unmarshal(planBytes, &plan, jsonv2.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if plan.Schema != "tsgk-corpus-cst-api-plan/r1" || len(plan.Expected) == 0 || len(plan.Expected) > 26000 || len(plan.Profiles) == 0 || len(plan.Profiles) > 500 || len(plan.Candidate) != 40 {
		return errors.New("CORPUS_PLAN_INVALID")
	}
	storage, wall, err := corpusEnvelope(plan)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(context.Background(), start.Add(time.Duration(wall)*time.Second))
	defer cancel()
	profileRoot, err := os.OpenRoot(filepath.Dir(planName))
	if err != nil {
		return err
	}
	defer profileRoot.Close()
	expected := map[string]corpusExpected{}
	inputPaths := map[string]bool{}
	var sourceBytes uint64
	for _, e := range plan.Expected {
		if expected[e.ID].ID != "" || inputPaths[strings.ToLower(e.Input.Path)] || e.Input.Bytes > 33554432 || e.Input.Role != "case" || !slices.Contains([]string{"csharp", "tsql", "xml", "svc"}, e.Route) || !kit.SourceEncodingValid(e.Encoding, []byte{}) {
			return errors.New("CORPUS_EXPECTED_DUPLICATE_OR_INVALID")
		}
		if e.Route != "svc" && (e.Nodes == 0 || e.Nodes > 25000000 || len(e.Digest) != 64 || e.HasError == nil) {
			return errors.New("CORPUS_EXPECTED_TREE_INVALID")
		}
		expected[e.ID] = e
		inputPaths[strings.ToLower(e.Input.Path)] = true
		sourceBytes += e.Input.Bytes
	}
	if sourceBytes > kit.DefaultCorpusLimits().TotalBytes {
		return errors.New("CORPUS_TOTAL_SOURCE_LIMIT")
	}
	profiles := []kit.OracleProfile{}
	seen := map[string]bool{}
	var profileBytes uint64
	for _, p := range plan.Profiles {
		if p.Bytes > 67108864-profileBytes {
			return errors.New("CORPUS_PROFILE_TOTAL_BYTES_LIMIT")
		}
		profileBytes += p.Bytes
		b, err := readPinned(profileRoot, p, 16777216)
		if err != nil {
			return err
		}
		prof, err := kit.ParseOracleProfile(b)
		if err != nil {
			return err
		}
		if err = corpusRegisteredProfile(prof); err != nil {
			return err
		}
		if prof.Native.Operation != "native-query-large" || prof.Native.Output != kit.OutputAuto || len(prof.Queries) != 0 || prof.FactPack != nil || !prof.API || prof.Native.SvcContext != nil {
			return errors.New("CORPUS_PROFILE_POLICY_MISMATCH")
		}
		for _, c := range prof.Native.Cases {
			e, ok := expected[c.ID]
			route := prof.Native.Route
			if prof.Native.Format == kit.SvcFormat {
				route = "svc"
			}
			if !ok || seen[c.ID] || e.Route != route || e.Encoding != c.Encoding || e.Input != c.Input || len(c.Edits) != 0 || len(c.Points) != 0 || len(c.Expect) != 0 {
				return errors.New("CORPUS_PROFILE_SET_MISMATCH")
			}
			seen[c.ID] = true
		}
		profiles = append(profiles, prof)
	}
	if len(seen) != len(expected) {
		return errors.New("CORPUS_PROFILE_CASE_MISSING")
	}
	root, err := os.OpenRoot(rootName)
	if err != nil {
		return err
	}
	defer root.Close()
	if err = os.Mkdir(work, 0700); err != nil {
		return err
	}
	if err = os.Mkdir(out, 0700); err != nil {
		return err
	}
	if err = os.Mkdir(filepath.Join(out, "pages"), 0700); err != nil {
		return err
	}
	hostPath, err := os.Executable()
	if err != nil {
		return err
	}
	hostBytes, err := os.ReadFile(hostPath)
	if err != nil {
		return err
	}
	bindings, err := os.OpenFile(filepath.Join(out, "bindings.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	quota := &corpusQuota{limit: storage}
	result := corpusResult{Schema: "tsgk-corpus-cst-api-result/r1", PlanSHA: digest(planBytes), Candidate: plan.Candidate, HostSHA: digest(hostBytes), StorageLimit: storage, WallLimit: wall, Expected: len(expected), Producers: map[string]corpusProducer{}}
	op := kit.NativeOperations()["native-query-large"]
	result.Policy = corpusPolicySHA(storage, wall)
	bindingHash := sha256.New()
	bindEncoder := json.NewEncoder(io.MultiWriter(corpusQuotaWriter{quota, bindings}, bindingHash))
	defer func() {
		returnErr = errors.Join(returnErr, bindings.Sync(), bindings.Close())
		result.WallMS = time.Since(start).Milliseconds()
		if ctx.Err() != nil || result.WallMS > int64(wall)*1000 {
			returnErr = errors.Join(returnErr, errors.New("CORPUS_GLOBAL_WALL_LIMIT"))
		}
		result.Complete = returnErr == nil && result.Completed == result.Expected
		if returnErr != nil {
			result.Error = returnErr.Error()
		}
		result.BindingsSHA = hex.EncodeToString(bindingHash.Sum(nil))
		result.Bytes = quota.used
		data, err := json.MarshalIndent(result, "", "  ")
		if err == nil {
			err = quota.put(filepath.Join(out, "result.json"), data)
		}
		returnErr = errors.Join(returnErr, err)
	}()
	auditSource, err := os.ReadFile("src/dev/large-api/audit.c")
	if err != nil {
		return err
	}
	wrapper := []byte("#define TSGK_CORPUS_CST\n#include \"audit.c\"\n")
	builds := map[string]*native.Build{}
	exes := map[string]string{}
	producerIDs := map[string]string{}
	grammarIDs := map[string]string{}
	defer func() {
		for _, b := range builds {
			returnErr = errors.Join(returnErr, b.Remove())
		}
	}()
	for _, prof := range profiles {
		route := prof.Native.Route
		b := builds[route]
		grammarBytes, err := json.Marshal(prof.Native.Grammar)
		if err != nil {
			return err
		}
		grammarID := digest(grammarBytes)
		if b == nil {
			grammarIDs[route] = grammarID
			b, err = native.NewBuild(ctx, native.BuildRequest{Work: work, Runtime: filepath.Join(prepared, "runtime"), GrammarRoot: filepath.Join(prepared, "routes", route), Grammar: prof.Native.Grammar, Symbol: prof.Native.Symbol, Compiler: cc, CompilerID: prof.Native.Compiler})
			if err != nil {
				if b != nil {
					builds[route] = b
				}
				return err
			}
			builds[route] = b
			if err = writeExclusive(filepath.Join(b.Dir, "audit.c"), auditSource); err != nil {
				return err
			}
			if err = writeExclusive(filepath.Join(b.Dir, "corpus-audit.c"), wrapper); err != nil {
				return err
			}
			exe, steps, err := compile(ctx, b, cc, filepath.Join(b.Dir, "corpus-audit.c"), "")
			if err != nil {
				return err
			}
			exes[route] = exe
			bytes, err := os.ReadFile(exe)
			if err != nil {
				return err
			}
			producer := corpusProducer{Route: route, Build: b, AuditSHA: digest(auditSource), WrapperSHA: digest(wrapper), ExecutableSHA: digest(bytes), Compile: steps}
			encoded, err := json.Marshal(producer)
			if err != nil {
				return err
			}
			producerIDs[route] = digest(encoded)
			result.Producers[route] = producer
		} else if b.Symbol != prof.Native.Symbol || b.Compiler != prof.Native.Compiler || grammarIDs[route] != grammarID {
			return errors.New("CORPUS_PRODUCER_MISMATCH")
		}
		for _, c := range prof.Native.Cases {
			if ctx.Err() != nil {
				return errors.New("CORPUS_GLOBAL_WALL_LIMIT")
			}
			e := expected[c.ID]
			source, err := readPinned(root, pinnedFile{Path: c.Input.Path, SHA: c.Input.SHA256, Bytes: c.Input.Bytes}, op.InputBytes)
			if err != nil {
				return err
			}
			if !kit.SourceEncodingValid(c.Encoding, source) {
				return errors.New("CORPUS_SOURCE_ENCODING_INVALID")
			}
			if e.Route == "svc" {
				obs := kit.ObserveServiceHost(c.Encoding, source)
				if !slices.Equal(source, c.SvcSource) || obs.IncludedRanges != nil || obs.Coverage.Directive != "OBSERVED" || obs.Coverage.Inline != "ABSENT" || obs.Directive == nil || len(obs.Directive.Diagnostics) != 0 {
					return errors.New("SVC_OBSERVATION_OR_SOURCE_MISMATCH")
				}
				observation, err := json.Marshal(struct {
					Case        corpusExpected     `json:"case"`
					Policy      string             `json:"policy"`
					Observation kit.SvcObservation `json:"observation"`
					API         string             `json:"api"`
				}{e, result.Policy, obs, "NOT_CLAIMED"})
				if err != nil {
					return err
				}
				name := fmt.Sprintf("pages/svc-%06d.json", result.SvcNotClaimed)
				if err = quota.put(filepath.Join(out, filepath.FromSlash(name)), observation); err != nil {
					return err
				}
				if err = bindEncoder.Encode(corpusBinding{ID: c.ID, Route: e.Route, Encoding: c.Encoding, Input: c.Input, Producer: producerIDs[route], Policy: result.Policy, Page: pinnedFile{Path: name, SHA: digest(observation), Bytes: uint64(len(observation))}}); err != nil {
					return err
				}
				result.SvcNotClaimed++
				result.Completed++
				continue
			}
			req := native.Request{ID: c.ID, Encoding: c.Encoding, Output: kit.OutputAuto, Limits: native.LimitsFor(op), Source: source, Protocol: native.ProtocolR2, API: true}
			tree := newCorpusTree()
			for tree.next < e.Nodes {
				begin := tree.next
				end := min(e.Nodes, begin+op.FullNodes)
				page, process, err := corpusPageRun(ctx, exes[route], result.Producers[route].ExecutableSHA, b.Dir, req, begin, end, e.Nodes)
				if err != nil {
					failure, _ := json.Marshal(struct {
						Case    corpusExpected `json:"case"`
						Error   string         `json:"error"`
						Process runner.Result  `json:"process"`
					}{e, err.Error(), process})
					return errors.Join(err, quota.put(filepath.Join(out, "failed-process.json"), failure), quota.put(filepath.Join(out, "failed-stdout.bin"), process.Stdout), quota.put(filepath.Join(out, "failed-stderr.bin"), process.Stderr))
				}
				name := fmt.Sprintf("pages/%06d.bin", result.Pages)
				if err = quota.put(filepath.Join(out, filepath.FromSlash(name)), process.Stdout); err != nil {
					return err
				}
				if err = tree.consume(page, e.Input.Bytes); err != nil {
					return err
				}
				binding := corpusBinding{ID: c.ID, Route: e.Route, Encoding: c.Encoding, Input: c.Input, Producer: producerIDs[route], Policy: result.Policy, Page: pinnedFile{Path: name, SHA: digest(process.Stdout), Bytes: uint64(len(process.Stdout))}, Start: begin, End: tree.next, Nodes: e.Nodes, Process: process}
				process.Stdout = nil
				binding.Process.Stdout = nil
				if err = bindEncoder.Encode(binding); err != nil {
					return err
				}
				result.Pages++
			}
			if hex.EncodeToString(tree.h.Sum(nil)) != e.Digest || tree.children != e.Nodes-1 || tree.rootError != *e.HasError {
				return errors.New("CORPUS_CST_DIGEST_COUNT_OR_FLAG_MISMATCH")
			}
			result.Completed++
			result.API++
			result.Nodes += e.Nodes
			result.Checks += tree.checks
			if result.Completed%20 == 0 {
				fmt.Printf("corpus completed=%d/%d pages=%d bytes=%d wall=%ds\n", result.Completed, result.Expected, result.Pages, quota.used, int(time.Since(start).Seconds()))
			}
		}
	}
	return nil
}
