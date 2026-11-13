// Command csharp-g1-native parses the source versions the sealed G1 expectations name with the
// registered tree-sitter-c-sharp native build and writes the raw native observation
// (tsgk-csharp-g1-native-raw/r1, a local artifact). It reads the sealed expected asset only to choose
// the sources; it never writes an asset and never changes an expectation (#233 G1-3a).
//
// Run it inside a HARD Job: the native process is limited to 1 GiB and 30 s here (a test-only
// overlay of the registered native-parse-edit operation; the product policy is unchanged) and the
// build steps keep their 4 GiB / 120 s.
//
//	go run ./src/dev/csharp-g1-native -root . -prepared <dir> -cc <compiler> -work <dir> -out <raw.json>
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

const (
	nativeMemory = 1 << 30
	nativeWall   = 30 * time.Second
	buildMemory  = 4 << 30
	buildWall    = 120 * time.Second
	route        = "csharp"
)

func read(root, rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
}

func sources(root string) (kit.CSharpG1Sources, error) {
	src := kit.CSharpG1Sources{Fixtures: map[string][]byte{}}
	var err error
	if src.Inventory, err = read(root, queryregistrycore.QualificationPin.Path); err != nil {
		return src, err
	}
	if src.Alternatives, err = read(root, "src/contracts/feature-alternatives.json"); err != nil {
		return src, err
	}
	if src.FactMapping, err = read(root, "src/contracts/fact-mapping.json"); err != nil {
		return src, err
	}
	for _, dir := range []string{"routes", "gaps", "n461", "queries"} {
		if src.Fixtures[dir], err = read(root, "src/testdata/native/"+dir+"/csharp.json"); err != nil {
			return src, err
		}
	}
	return src, nil
}

// build compiles the registered route like the repository's native tests do.
func build(ctx context.Context, root, prepared, cc, work string) (*native.Build, kit.ToolIdentity, error) {
	var id kit.ToolIdentity
	data, err := read(root, "src/contracts/native-routes.json")
	if err != nil {
		return nil, id, err
	}
	var registry struct {
		Routes []struct {
			Route, Symbol string
			Files         []kit.NativeInput
		}
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, id, err
	}
	var grammar []kit.NativeInput
	var symbol string
	for _, r := range registry.Routes {
		if r.Route == route {
			grammar, symbol = r.Files, r.Symbol
		}
	}
	if len(grammar) == 0 || symbol == "" {
		return nil, id, fmt.Errorf("registered %s closure missing", route)
	}
	compiler, err := os.ReadFile(cc)
	if err != nil {
		return nil, id, err
	}
	sum := sha256.Sum256(compiler)
	id = kit.ToolIdentity{Name: "cc", Version: "g1-native", SHA256: hex.EncodeToString(sum[:]), Bytes: uint64(len(compiler))}
	b, err := native.NewBuild(ctx, native.BuildRequest{Work: work, Runtime: filepath.Join(prepared, "runtime"), GrammarRoot: filepath.Join(prepared, "routes", route),
		Grammar: grammar, Symbol: symbol, Compiler: cc, CompilerID: id})
	return b, id, err
}

func run(root, prepared, cc, work, out string) error {
	for _, p := range []string{root, prepared, cc, work, out} {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("absolute paths required: %s", p)
		}
	}
	src, err := sources(root)
	if err != nil {
		return err
	}
	all, err := kit.CSharpG1RoslynSources(src)
	if err != nil {
		return err
	}
	expData, err := read(root, kit.CSharpG1ExpectedPath)
	if err != nil {
		return err
	}
	exp, err := kit.DecodeCSharpG1Expected(expData)
	if err != nil {
		return err
	}
	ledger, err := kit.DeriveCSharpG1Ledger(src)
	if err != nil {
		return err
	}
	byID := map[string]kit.G1RoslynCaseSource{}
	for _, s := range all {
		byID[s.ID] = s
	}
	var todo []kit.G1RoslynCaseSource
	for _, id := range kit.CSharpG1NativeSourceIDs(ledger, exp) {
		s, ok := byID[id]
		if !ok {
			return fmt.Errorf("source %s is not a ledger source", id)
		}
		todo = append(todo, s)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	b, cc2, err := build(ctx, root, prepared, cc, work)
	if err != nil {
		return fmt.Errorf("native build: %w", err)
	}
	op := kit.NativeOperations()["native-parse-edit"]
	op.MemoryBytes, op.ParseWall, op.EditWall = nativeMemory, nativeWall, nativeWall
	x := native.Context{Op: op, Route: route, Output: kit.OutputTree, PolicyRef: native.PolicyRef(op, kit.OutputTree), CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")}

	raw := kit.CSharpG1NativeRaw{Schema: kit.CSharpG1NativeRawSchema, Provenance: kit.G1NativeProvenance{
		Route: route, Symbol: b.Symbol, Platform: runtime.GOOS + "/" + runtime.GOARCH, DriverIdentity: b.Identity,
		Compiler: cc2.Name, CompilerSHA256: cc2.SHA256, CompilerVersion: b.CompilerVersion, RuntimeCommit: b.Runtime, Operation: op.Name,
		NativeMemory: nativeMemory, NativeWallMS: nativeWall.Milliseconds(), BuildMemory: buildMemory, BuildWallMS: buildWall.Milliseconds()}}
	for _, f := range b.Closure {
		if f.Role != "runtime" {
			raw.Provenance.Grammar = append(raw.Provenance.Grammar, kit.G1NativeFile{Path: f.Path, SHA256: f.SHA256, Bytes: f.Bytes})
		}
	}
	reported := false
	for i, s := range todo {
		c := kit.IncrementalCase{ID: fmt.Sprintf("g1n-%03d", i), Encoding: kit.EncodingUTF8, Input: kit.NativeInput{SHA256: s.SHA256, Bytes: uint64(len(s.Data))}}
		rctx, rcancel := context.WithTimeout(ctx, nativeWall+time.Minute)
		r := b.RunCase(rctx, x, c, s.Data)
		rcancel()
		rec := kit.G1NativeSource{ID: s.ID, SHA256: s.SHA256, Bytes: len(s.Data), Status: r.ExecutionStatus, Code: r.Code, Types: []string{}, Nodes: [][5]int{}}
		if r.ExecutionStatus != kit.StatusCompleted && !reported {
			reported = true
			fmt.Fprintf(os.Stderr, "first failure %s: status=%s code=%s assessment=%s raw=%.400q\n", s.ID, r.ExecutionStatus, r.Code, r.Assessment, r.Raw)
			if r.Process != nil {
				fmt.Fprintf(os.Stderr, "process status=%s reason=%s exit=%d stderr=%.400q\n", r.Process.Status, r.Process.Reason, r.Process.ExitCode, r.Process.Stderr)
			}
		}
		if r.Process == nil || !r.Process.Cleanup.Verified {
			rec.Status = "CLEANUP_UNVERIFIED"
			if rec.Code == "" {
				rec.Code = "NO_PROCESS_RESULT"
			}
		}
		if rec.Status == kit.StatusCompleted && len(r.Steps) == 1 && r.Steps[0].Incremental != nil && r.Steps[0].Incremental.Tree != nil {
			t := r.Steps[0].Incremental
			rec.Status, rec.HasError = kit.G1NativeCompleted, t.HasError
			index := map[string]int{}
			for _, n := range t.Tree.Nodes {
				name := n.Type
				if !n.Named {
					name = `"` + n.Type + `"` // the kind map quotes anonymous types
				}
				ti, ok := index[name]
				if !ok {
					ti = len(rec.Types)
					index[name] = ti
					rec.Types = append(rec.Types, name)
				}
				flags := 0
				for bit, on := range map[int]bool{kit.G1NodeNamed: n.Named, kit.G1NodeExtra: n.Extra, kit.G1NodeError: n.IsError, kit.G1NodeMissing: n.IsMissing} {
					if on {
						flags |= bit
					}
				}
				rec.Nodes = append(rec.Nodes, [5]int{ti, flags, int(n.StartByte), int(n.EndByte), int(n.Parent)})
			}
			rec.TreeDigest = kit.G1NativeNodesDigest(rec)
		} else if rec.Status == kit.StatusCompleted {
			rec.Status, rec.Code = "NO_FULL_TREE", "STEP_TREE_MISSING"
		}
		if rec.Status != kit.G1NativeCompleted && rec.Code == "" {
			rec.Code = r.Assessment
		}
		raw.Sources = append(raw.Sources, rec)
	}
	slices.SortFunc(raw.Sources, func(a, b kit.G1NativeSource) int { return compareIDs(a.ID, b.ID) })
	data, err := kit.MarshalCSharpG1NativeRaw(raw)
	if err != nil {
		return err
	}
	if _, err := kit.DecodeCSharpG1NativeRaw(data); err != nil {
		return fmt.Errorf("raw document does not decode: %w", err)
	}
	if _, err := os.Stat(out); err == nil {
		return fmt.Errorf("preserve existing %s", out)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return err
	}
	completed := 0
	for _, s := range raw.Sources {
		if s.Status == kit.G1NativeCompleted {
			completed++
		}
	}
	fmt.Printf("sources %d completed %d bytes %d\n", len(raw.Sources), completed, len(data))
	return nil
}

func compareIDs(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func main() {
	root := flag.String("root", "", "repository root (absolute)")
	prepared := flag.String("prepared", "", "prepared routes directory holding runtime/ and routes/csharp (absolute)")
	cc := flag.String("cc", "", "C compiler (absolute)")
	work := flag.String("work", "", "build work directory (absolute, existing)")
	out := flag.String("out", "", "raw observation to write (absolute, must not exist)")
	flag.Parse()
	if err := run(*root, *prepared, *cc, *work, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
