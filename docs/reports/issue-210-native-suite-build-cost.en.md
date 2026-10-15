# Windows native test build serialization cost (#210)

## Failure and causal scope

After PR #209 merged, main `1b6287f75733cc4b2b080b94ee9e72974a402804` failed in [Foundation run 38653072957, attempt 1](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/38653072957). The Windows `src/internal/native` package reached its 300-second timeout at 300.246 seconds with compiler/NewBuild calls still active. Skipped downstream native and qualification jobs are not passes.

The preceding PR run passed in 243.242 seconds with identical code, workflow, Go 1.27.1 and GCC 15.2.0 binary. The Windows image changed from `20260925.250.1` to `20261007.264.1`. Host load and other factors were not controlled; the image is not established as the sole cause or the failure as transient infrastructure noise.

The confirmed improvement target is the test helper structure. Previously, `fixtureBuild` held one global mutex throughout C compilation on a cache miss, serializing independent defines. Four fault and ten reuse-observation children also ran sequentially. This is test scheduling overhead, not an observed parser throughput or public API defect.

## Change

Each fixture key uses `sync.Once` to share its actual `NewBuild` result and error. Preparation helpers that may call `t.Fatal` run outside Once; each caller reports the error to its own test. The mutex protects only cache entries and the shared work root. Different builds use distinct directories, with at most four fixture compilations at once. The existing benchmark cache and code are retained.

The fault and reuse parent tests remain sequential; only their independent children call `t.Parallel`. Each group completes before other sequential timing tests run. Each child owns its source, edits and request. Subsequent reuse executions are bounded by ten children and Go test parallelism; the four-build cap is not a four-process cap for all native execution.

Production `NewBuild`, compiler flags, runtime and grammar sources, fault cases and assertions, the 300-second package and 10-minute foundation job limits, and individual process limits remain unchanged. An actual native concurrency test checks that requests for the same key share a Build pointer while different defines have distinct directories and identities. This test alone does not dynamically establish maximum concurrency or cached failure propagation.

## Observations and validation

Separate cold test binaries used the same runtime and compiler under local Go 1.27.2, CGO_ENABLED=0 and GCC 16.2.0. Three paired fault-phase process-wall samples were 33.006/28.672/27.597 seconds before and 12.201/10.888/10.162 seconds for the first candidate, with medians 28.672→10.888 seconds. That candidate did not yet parallelize reuse tests. Pair order was baseline/candidate, candidate/baseline, baseline/candidate; both binaries used parallel=4.

Two paired reuse-phase process-wall samples were 69.955/82.800 seconds before and 26.954/30.841 seconds for the candidate.

The final local native package passed in 150.140 seconds under its unchanged 300-second limit; process wall including the Go command was 150.924 seconds. Full foundation process wall was 57.235 seconds; vet and build passed in 0.898 and 1.015 seconds. Other test-bearing packages reuse passing r3 results at identical bytes; the changed foundation package has a fresh full run. This is an aggregate of package results, not a claim that one complete command passed. `src/drivers` has no test files. Existing platform and benchmark-tool conditional skips are retained.

With optional native tools absent (both missing, runtime only, compiler only), the new parent test skipped and exited successfully. With REQUIRED=1 and both tools absent it failed with the expected reason and exit 1. The configured actual concurrency test passed. A STATIC review found the optional-tool defect, fixed by calling `nativeTools(t)` at parent entry. Intermediate failures from document LF formatting and the explicit source-only report fixture list are preserved; corrected LF and fixture paths passed focused checks and full foundation validation.

Two final-code STATIC_REVIEW passes in the right pane confirmed the optional-path fix and found no further actionable findings. These are neither independent execution nor formal GitHub approvals. Mandatory three-OS CI for a new PR and actual-main post-merge verification have not run yet; local success does not replace them.

Local checks not run include `TestFactMappingAgainstSchemas` without route-schema inputs, CLI SVC observation without the prepared C# closure, and JSON holdout/benchmark-control/reuse checks without the prepared JSON closure. Case-sensitive names, executable-bit and FIFO checks skipped on this Windows filesystem. They are not counted as local passes.

These observations do not establish parser throughput improvements, fewer compilations, guarantees on another host, or resolution of the entire 300-second problem. Fixture warm-up can move costs between tests, so full-package results also matter. The failed run is preserved.
