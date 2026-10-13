package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// Native benchmark configuration is explicit and mandatory; missing tools are a failure.
func benchmarkJSONBuild(b testing.TB) *Build {
	b.Helper()
	prepared, cc := os.Getenv("TSGK_BENCH_PREPARED"), os.Getenv("TSGK_NATIVE_CC")
	if !filepath.IsAbs(prepared) || !filepath.IsAbs(cc) {
		b.Fatal("absolute TSGK_BENCH_PREPARED and TSGK_NATIVE_CC required")
	}
	buildMu.Lock()
	defer buildMu.Unlock()
	key := "benchmark-json|" + prepared + "|" + cc + "|" + os.Getenv("TSGK_CGROUP_PARENT") + "|" + os.Getenv("TSGK_NATIVE_SANITIZE")
	if cached := buildCache[key]; cached != nil {
		return cached
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "contracts", "native-routes.json"))
	if err != nil {
		b.Fatal(err)
	}
	var registry struct {
		Routes []struct {
			Route, Symbol string
			Files         []kit.NativeInput
		}
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		b.Fatal(err)
	}
	var grammar []kit.NativeInput
	var symbol string
	for _, route := range registry.Routes {
		if route.Route == "json" {
			grammar, symbol = route.Files, route.Symbol
		}
	}
	if len(grammar) == 0 || symbol != "tree_sitter_json" {
		b.Fatal("registered JSON closure missing")
	}
	compiler, err := os.ReadFile(cc)
	if err != nil {
		b.Fatal(err)
	}
	sum := sha256.Sum256(compiler)
	if buildWork == "" {
		buildWork, err = os.MkdirTemp("", "tsgk-native-benchmark-")
		if err != nil {
			b.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	req := testBuildRequest(buildWork, filepath.Join(prepared, "runtime"), filepath.Join(prepared, "routes", "json"), grammar, cc, kit.ToolIdentity{Name: "cc", Version: "benchmark", SHA256: hex.EncodeToString(sum[:]), Bytes: uint64(len(compiler))})
	req.Symbol = symbol
	built, err := NewBuild(ctx, req)
	if err != nil {
		b.Fatalf("registered build refused: %v", err)
	}
	baseline, err := os.ReadFile(filepath.Join("..", "..", "dev", "benchmark", "native", "parse.c"))
	if err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(built.Dir, "baseline.c"), baseline, 0600); err != nil {
		b.Fatal(err)
	}
	args := []string{"-O2", "-std=c11", "-D_POSIX_C_SOURCE=200112L", "-I", "runtime/lib/include", "baseline.c", "lib.o", "parser.o", "-o", "baseline.exe"}
	if req.Sanitize {
		args = append(args, "-fsanitize=address,undefined", "-fno-omit-frame-pointer", "-fno-sanitize-recover=undefined")
	}
	result, err := runner.Run(ctx, runner.Spec{Path: cc, Dir: built.Dir, Env: buildEnv(cc, built.Dir),
		Args: args,
		Wall: 2 * time.Minute, Grace: 5 * time.Second, StdoutBytes: 65536, StderrBytes: 65536, Memory: runner.Memory{Bytes: buildMemory, Hard: runtime.GOOS != "darwin"}, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")})
	if err != nil || !result.Succeeded() {
		b.Fatalf("baseline build: %v %s", err, result.Status)
	}
	baselineHash, _, err := fileDigest(filepath.Join(built.Dir, "baseline.exe"))
	if err != nil {
		b.Fatal(err)
	}
	buildCache[key] = built
	benchmarkBaselineHashes[built.Dir] = baselineHash
	b.Logf("native build=%s runtime=%s compiler_sha256=%s direct_exe_sha256=%s", built.Identity, built.Runtime, built.Compiler.SHA256, baselineHash)
	return built
}

var benchmarkBaselineHashes = map[string]string{}

type benchmarkParseObservation struct {
	ParseNS  uint64 `json:"parse_ns"`
	Nodes    uint64 `json:"nodes"`
	EndByte  uint64 `json:"end_byte"`
	HasError bool   `json:"has_error"`
}

func benchmarkDirect(ctx context.Context, built *Build, source []byte) (runner.Result, benchmarkParseObservation, error) {
	file := filepath.Join(built.Dir, "baseline.exe")
	hash, _, err := fileDigest(file)
	if err != nil || hash != benchmarkBaselineHashes[built.Dir] {
		return runner.Result{}, benchmarkParseObservation{}, fmt.Errorf("direct executable changed: %v", err)
	}
	result, err := runner.Run(ctx, runner.Spec{Path: file, Dir: built.Dir, Env: buildEnv(os.Getenv("TSGK_NATIVE_CC"), built.Dir), Stdin: source,
		Wall: 10 * time.Second, Grace: 5 * time.Second, StdoutBytes: 65536, StderrBytes: 65536, Memory: runner.Memory{Bytes: buildMemory, Hard: runtime.GOOS != "darwin"}, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")})
	var parsed benchmarkParseObservation
	if err == nil && len(result.Stdout) != 0 {
		err = json.Unmarshal(result.Stdout, &parsed)
	}
	return result, parsed, err
}

func TestNativeBenchmarkControls(t *testing.T) {
	if os.Getenv("TSGK_BENCH_PREPARED") == "" {
		t.Skip("native benchmark prepared closure not configured")
	}
	built := benchmarkJSONBuild(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, input := range []string{"", strings.Repeat(" ", 65537)} {
		process, _, err := benchmarkDirect(ctx, built, []byte(input))
		if err != nil || process.ExitCode != 2 || !process.Cleanup.Verified {
			t.Fatalf("direct input-bound control: %v %d", err, process.ExitCode)
		}
	}
	process, observation, err := benchmarkDirect(ctx, built, []byte("["))
	if err != nil || process.ExitCode != 1 || !process.Cleanup.Verified || !observation.HasError {
		t.Fatalf("direct syntax control: %v %d %+v", err, process.ExitCode, observation)
	}
	result := built.RunCase(ctx, testContext("native-parse-edit"), kit.IncrementalCase{ID: "invalid-json-control", Encoding: kit.EncodingUTF8, Expect: []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR"}}}, []byte("["))
	if result.ExecutionStatus != kit.StatusCompleted || result.Assessment != kit.AssessFail || result.Claims.Expectations != ClaimFail {
		t.Fatalf("kit syntax negative control: %s %s %+v", result.ExecutionStatus, result.Assessment, result.Claims)
	}
}

func BenchmarkNativeJSON(b *testing.B) {
	built := benchmarkJSONBuild(b)
	for _, elements := range []int{64, 512, 2048} {
		source := []byte("[" + strings.Repeat("12345,", elements) + "0]")
		for _, mode := range []string{"direct-fresh", "kit-tree", "kit-edit", "kit-api"} {
			b.Run(fmt.Sprintf("numbers=%d/mode=%s", elements+1, mode), func(b *testing.B) {
				originalHash := sha256.Sum256(source)
				edited := append([]byte("[9"), source[6:]...)
				editedHash := sha256.Sum256(edited)
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				var processMS, parseTime, parseCalls float64
				var peak uint64
				var observed int
				var memoryMetric, enforcement string
				b.ReportAllocs()
				b.SetBytes(int64(len(source)))
				b.ResetTimer()
				for b.Loop() {
					if mode == "direct-fresh" {
						result, parsed, err := benchmarkDirect(ctx, built, source)
						if err != nil || !result.Succeeded() {
							b.Fatalf("direct parse: %v %s", err, result.Status)
						}
						if parsed.HasError || parsed.EndByte != uint64(len(source)) || parsed.Nodes != uint64(2*elements+5) {
							b.Fatalf("direct parse observation: %+v", parsed)
						}
						processMS += float64(result.WallMillis)
						parseTime += float64(parsed.ParseNS)
						parseCalls++
						memoryMetric, enforcement = result.Memory.Metric, result.Memory.Enforcement
						if result.Memory.PeakKnown {
							observed++
							if result.Memory.Peak > peak {
								peak = result.Memory.Peak
							}
						}
					} else {
						op := kit.NativeOperations()["native-parse-edit"]
						run := Context{Op: op, Route: "json", Output: kit.OutputTree, PolicyRef: PolicyRef(op, kit.OutputTree), CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")}
						c := kit.IncrementalCase{ID: "benchmark", Encoding: kit.EncodingUTF8, Expect: []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR"}}}
						if mode == "kit-edit" {
							c.Edits = []kit.Edit{{StartByte: 1, OldEndByte: 6, NewEndByte: 2, Old: []byte("12345"), New: []byte("9")}}
							c.Expect = append(c.Expect, kit.StepExpectation{Step: 1, Syntax: "NO_ERROR"})
						}
						if mode == "kit-api" {
							run.Op = kit.NativeOperations()["native-query"]
							run.PolicyRef = PolicyRef(run.Op, run.Output)
							run.Protocol, run.API = ProtocolR2, true
						}
						result := built.RunCase(ctx, run, c, source)
						if result.ExecutionStatus != kit.StatusCompleted || result.Assessment != kit.AssessPass || result.Process == nil || !result.Process.Succeeded() {
							b.Fatalf("native measurement: %s %s %s", result.ExecutionStatus, result.Assessment, result.Code)
						}
						if mode == "kit-edit" && result.Claims.IncrementalEquality != ClaimPass {
							b.Fatal("edit/fresh equality missing")
						}
						if mode == "kit-api" && (result.Oracle == nil || result.Oracle.API != ClaimPass) {
							b.Fatal("API audit claim missing")
						}
						wantSteps := 1
						if mode == "kit-edit" {
							wantSteps = 2
						}
						if len(result.Steps) != wantSteps {
							b.Fatal("native step count changed")
						}
						processMS += float64(result.Process.WallMillis)
						for i, step := range result.Steps {
							bytes, inputHash := len(source), hex.EncodeToString(originalHash[:])
							if i == 1 {
								bytes, inputHash = len(edited), hex.EncodeToString(editedHash[:])
							}
							if step.SourceBytes != uint64(bytes) || step.SourceSHA256 != inputHash || step.Incremental == nil || step.Incremental.HasError || step.Incremental.DescendantCount != uint64(2*elements+5) || step.Incremental.Tree == nil || len(step.Incremental.Tree.Nodes) != 2*elements+5 || step.Incremental.Tree.Nodes[0].EndByte != uint32(bytes) {
								b.Fatal("kit fixture observation changed")
							}
							if i == 1 {
								if step.Fresh == nil || step.Comparison == nil || !step.Comparison.Equal || step.Route == nil || !step.Route.Proven || step.Incremental.Tree.Nodes[3].StartByte != 1 || step.Incremental.Tree.Nodes[3].EndByte != 2 || step.Incremental.Tree.Nodes[3].Type != "number" {
									b.Fatal("edit/fresh observation missing")
								}
							}
							parseTime += float64(step.Incremental.ParseMillis)
							parseCalls++
							if step.Fresh != nil {
								parseTime += float64(step.Fresh.ParseMillis)
								parseCalls++
							}
						}
						memoryMetric, enforcement = result.Process.Memory.Metric, result.Process.Memory.Enforcement
						if result.Process.Memory.PeakKnown {
							observed++
							if result.Process.Memory.Peak > peak {
								peak = result.Process.Memory.Peak
							}
						}
					}
				}
				b.ReportMetric(processMS/float64(b.N), "process-ms/op")
				unit := "kit-reported-parse-ms/op"
				if mode == "direct-fresh" {
					unit = "direct-parse-ns/op"
				}
				b.ReportMetric(parseTime/float64(b.N), unit)
				b.ReportMetric(parseCalls/float64(b.N), "parse-calls/op")
				b.ReportMetric(float64(observed), "native-peak-observations")
				if observed != 0 {
					b.ReportMetric(float64(peak), "native-max-observed-peak-bytes")
				}
				sum := sha256.Sum256(source)
				b.Logf("input_sha256=%s bytes=%d memory_metric=%s enforcement=%s observed=%d/%d", hex.EncodeToString(sum[:]), len(source), memoryMetric, enforcement, observed, b.N)
			})
		}
	}
}
