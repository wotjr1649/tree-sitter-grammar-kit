# v0.3.0 release document (draft, #220)

This is the English body draft of the source-only release `v0.3.0`. The Korean version is [한국어](issue-220-v030-release.md). The final tag target commit and publication date are not selected. Publication requires review of the exact version, commit, Korean/English body and source-only scope, followed by separate user approval. This draft is neither a publication approval request nor publication evidence. The tracking Issue is [#220](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/220). The earlier preparation record is the [#199 document](issue-199-source-release.en.md); its candidates (`33f84fc`, `52c1e2b`) and counts are historical records only.

## Conditions

- Every item under "Query semantics claim" below is valid only after integration PR [#416](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/416) (`integ/362`, `Refs #362`) is merged into main. Before that merge, none of those items is claimed.
- The scope already claimed (see "Product scope") is unchanged and is not reduced. The only new claim is the verified subset of query semantics.
- This document's counts are not final-release evidence until the [pre-publication checks](#pre-publication-checks-still-open) are run again on the final candidate commit. The fan-in counts are recomputed on the final candidate and updated.

## Product scope

Owned by [product scope](../specs/scope.md) and the [platform support contract](../specs/platform-support.md#release-v030의-claim-초안).

- **VERIFIED:** offline `inspect`, `identity`, `verify`, `schema check`/`schema diff` and their public Go API (`src/kit`). The qualification verdict for the registered 26 routes concerns fixed inputs and obligations; it does not certify arbitrary source programs. Current CI verifies Windows and Linux (a derived 52 cells, #332).
- **experimental (no compatibility promise):** `corpus`, `reproduce`, `incremental`, `oracle record`, `replay`, `evidence verify`, `qualify` and their APIs, and all query semantics verification below.
- **C#:** only the verified scope is claimed: the residual C1–C4 corrections (PR [#369](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/369)) and the existing qualification PASS (C# symbol 26 PASS, 0 MISMATCH, 4 NOT_APPLICABLE). The comparison of obligations → independent expected values → execution evidence across the full C# declaration scope is not claimed (see "Remaining constraints" below).
- **Consumer requirement: Go 1.27.4 or newer.** The `go` directive in `go.mod` rose from 1.27.1 to 1.27.4 (#358). Details are in "Public API, CLI and schema changes (against v0.2.0)" below.
- The artifacts are a source tag and a source-only GitHub Release. There is no binary or package.

## Query semantics claim (experimental)

v0.3.0 keeps query semantics experimental and claims only the subsets actually verified. Everything else is listed as "not claimed" with its BLOCKED reason. By the 2026-11-12 user decision, the completion gate of [#362](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/362) is not a #220 shipping condition. This aligns the claim wording of an already-experimental feature with the verified facts; it does not reduce the supported scope.

### Claimed scope

All results are from Windows, with the actual owned native producer and the registered limits unchanged (logical 1 GiB, work 32,000,000, read 256 MiB; large uses the registered large budget). Independent expected values were derived from grammar and query source and sealed before native execution.

| Scope | Verified fact | Evidence |
|---|---|---|
| predicates (`eq?`, `not-eq?`, `any-of?`, `not-any-of?`) | 20/20 independent evaluation tuples VERIFIED; 47/47 rejection and incomplete-policy rows rejected exactly. No new predicate support | [#408](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/408), [expected](../../src/contracts/query-predicate-expected-r1.json), [contract](../specs/instrumented-query-reference.md#362-predicate-의미-의무와-거부-정책-범위-3) |
| SVC offline obligations | 263 VERIFIED (history 110, edit point 41, segment 109, CodeBehind 2, negative 1) | [#410](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/410), [expected](../../src/contracts/query-svc-expected-r1.json) |
| SVC inline C# | 30 of 82 trees and 60 of 164 queries match the real C# grammar native (including zero `facts.dynamic_sql` facts) | [#415](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/415), [expected](../../src/contracts/query-svc-inline-expected-r1.json) |
| Windows large | 31 of 39 obligations VERIFIED. For `cs-large-22m`, source, status, the full CST (11,463,151 nodes, 12 pages) and occurrences match native. The `cs-large-32mib-errors` status is `COMPLETED` under the registered 8 GiB HARD limit. The over-limit control is rejected exactly as `RESOURCE_LIMIT`/`OUTPUT_LIMIT` | [#412](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/412), [#415](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/415), [expected](../../src/contracts/query-large-expected-r1.json) |
| ordinary selected SQL shard public pass | TSQL `alt-tsql-B01-r001` 72/72 and PostgreSQL `alt-postgresql-sql-B04-22` 9/9 VERIFIED. Limits unchanged; the remaining TSQL margin is thin (work 7.3%, logical 3.47%) | [#416](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/416), [#403](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/403) |
| ordinary JSON positives | Multiple cases (2/2) and an edit history (3 edits, 4 versions, 14/14) agree across the public API, r9 replay and `tsgk replay` (JSON grammar fixture tracked in the repository) | [#404](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/404), [#402](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/402) |
| per-scope fan-in | Collects the current state with machine checks; FAILED 0. The tally is VERIFIED 360 of 107,399 obligations (per the PR #416 body, before the large 32 MiB rerun), the rest BLOCKED. This count is recomputed on the final candidate commit and updated | [#413](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/413), [#414](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/414) |

### Not claimed (BLOCKED reasons)

| Not claimed | Reason |
|---|---|
| CST and captures for inputs that produce error recovery (ERROR, MISSING) | The result is not determined by grammar rules alone, so an independent expected value cannot be built without copying results into goldens. This covers 8 SVC inline steps and 8 CST/occurrence obligations of the large fixtures `cs-large-8m-errors` and `cs-large-32mib-errors` |
| SVC inline needing preprocessor scanner state | 44 steps (`#if`, `#else`, `#define`, `#endif`). The external scanner's state machine could not be derived independently from rules. The 104 queries on those steps are also not claimed |
| Linux share | The Linux share of predicates (20+47), SVC and large is registered only; there is no CI collection path |
| ordinary in full | Of the 106,208 ordinary tuples, nothing beyond the selected shards above is verified. Binary-paged TSQL `alt-tsql-B04-r144` has no independent expected values and stays 8/8 BLOCKED |
| reference rows and approval | The reference rows and approval in the public-pass tests are synthetic. They are not evidence of a real external reference run or a caller-approved workload |
| predicate evaluation by the native producer | The native producer's predicate capability stays `NOT_EVALUATED`. The predicate 20/20 above is the result of a separate typed verification path (reusing `VerifyQueryWitness`) |

### Completion gate

The #362 completion gate ships closed (`BLOCKED`, `complete=false`). The current reason is the absent stored ordinary result (`QUERY_SCOPE_RESULT_ABSENT`), and the large error-recovery denominators are also incomplete, so this revision cannot become COMPLETE. #362 stays open after shipping and continues with the independent-expected criterion for error-recovery inputs, Linux collection and the full ordinary set.

## Changes since the previous release

The list of changes since v0.2.0 is owned by the v0.3.0 section of the [changelog](../../CHANGELOG.md). The main groups are:

- Path safety (#196, #215), optional logger observation (#197), old-tree delivery observation before an error-tree route is BLOCKED (#217), accepted reuse evidence (#264).
- The C# conditional symbol mode (#218) and lexical recovery corrections (#233; C1–C4 applied in #369).
- Version-independent historical verification (V2, #393) and the query semantics verification base (#362 P0–P2, budget reduction #385, source restoration P1 #367).
- Go baseline 1.27.4 (#358), the Windows runner Job Object fix (#330), and CI operations changes (macOS exclusion #332 and others). CI changes do not alter product behavior.

The public API, CLI and schema changes are in the next section. The historical comparison (21 additions, nine changes, one flag) belongs only to that earlier candidate in the [#199 document](issue-199-source-release.en.md#api-cli-and-schema-consumption).

## Public API, CLI and schema changes (against v0.2.0)

The baseline is tag `v0.2.0` and the integration candidate `integ/362` (PR #416). The analysis was a normalized comparison of exported declarations (`src/kit/...`, `_test.go` excluded), a comparison of each `tsgk` subcommand's `-h` flag list, and a comparison of schema constants and literals and the `schema` fields of `src/contracts/**/*.json`. The exported elements added by the #416 review fixes after the analysis (the "review-fix" row below) are included. Re-comparison on the final candidate commit is in [Pre-publication checks still open](#pre-publication-checks-still-open).

> **Consumer requirement: Go 1.27.4 or newer.** The `go` directive in `go.mod` rose from `1.27.1` to `1.27.4` (#358). A consumer that builds this module needs Go 1.27.4 or newer. If automatic toolchain switching is disabled, the build fails on an older Go. This applies to consumers of the stable surface too.

> **User notice: the native runtime pin changed (#213).** The Tree-sitter runtime source used by the native paths (`incremental`, `oracle record` and others) moved from commit `659cda7c7f86ebe31cc825dc5da59e9add172dc7` (v0.2.0) to release tag `v0.27.1` (commit `10eb5ac119eec9cb4267a8e035066473983f5f3a`). The directory given to `--runtime` must be the runtime that the `tsgk` in use pins; otherwise both directions are rejected with `RUNTIME_MISMATCH` (`lib/include/tree_sitter/api.h: identity mismatch`, exit 2, `NOT_RUN`). A runtime directory prepared for v0.2.0 cannot be used as is with the candidate `tsgk`, and the reverse is also true. For the same grammar and input, the build identity, driver executable, runtime commit and producer identity differ. The pin is owned by `src/drivers/native-c/runtime-manifest.json`; the basis is in the [upstream sources](../provenance/upstream-sources.md).

### Stable surface: no breaking change

- Public Go API: no exported identifier was deleted, no signature of an existing function or method changed, and no method was added to an existing interface.
- CLI: the flag lists, defaults and descriptions of `inspect`, `identity`, `verify`, `schema check`/`schema diff` (and of the experimental `corpus` and `reproduce`) are the same as in v0.2.0. Every new flag defaults to empty or false.
- Schema: the values of the schema constants used by the stable API (`ReportSchema`, `ProfileSchema`, `ExpectedSchema`, `ManifestSchema`, `FileSetSchema`, the `PolicySchema` family, `SchemaFormat`, `SchemaComparator`, `TreeSummarySchema`, and others) are unchanged.
- Behavior: running `inspect`, `identity`, `schema check` and `schema diff` on the v0.2.0 plain fixture with both versions gives byte-identical stdout (exit 0). On candidate `aca5385`, `corpus`, `verify` (pass, tampered and no-expected cases), an invalid `schema check`, root-command errors, and `inspect` and `corpus` on the repository's `ordinary-json` fixture also gave byte-identical stdout and the same exit codes. The experimental `reproduce` gave the same verdict and output (`parser.c`) on the same JSON-mode fixture, with only timestamps and memory figures differing (no real tree-sitter generator was available, so a test generator with the same behavior was used).
- The Go minimum-version increase above applies to stable consumers as well.

### Counts of changes, additions and deletions

| Surface | Changed | Deleted | Added |
|---|---:|---:|---:|
| Go `src/kit/...` | 7 (3 types, 4 consts) | 0 | 1,826 elements: 323 new top-level (59 funcs, 142 types, 122 consts), 945 members of new types, 42 new members of existing types, and 516 in 5 new sub-packages (`nativewire`, `querypolicy`, `queryregistrycore`, `querywire`, `reuseobs`) |
| CLI `tsgk` | 2 help texts (`qualify --inventory`, the `evidence` usage error) | 0 (subcommands, flags) | 1 new subcommand (`evidence query-reference`), 27 new flags (22 on existing subcommands, 5 on the new one) |
| schema | 3 exported constant values + 1 format string; 1 contracts JSON (`qualification-c1.json`) | 3 literals in `src/kit` | 66 exported constants, 74 new families in `src/kit`, 16 new contracts JSON files |

- The seven Go "changed" items are by v0.2.0 identifier. `TreeNode`, `Point` and `EditPoints`, formerly structs, became aliases of the same structs in `nativewire` (same fields and tags), and the four changed consts are in the table below. No identifier was deleted.
- "Review-fix" means the three exported elements added by the #416 review fix (`0ad8aef`): the constants `SvcInlineRoleIncremental` and `SvcInlineRoleFresh`, and the field `SvcInlineView.Role`. All belong to the SVC inline types that are new in v0.3.0.
- The new surface belongs to the experimental features listed in "Product scope". The scope claimed as VERIFIED is the same as in v0.2.0.

### Compatibility breaks on the experimental surface

These are changes to the experimental surface of the v0.2.0 claim (`qualify`, `incremental`, `replay`, SVC records, the native tree and edit helpers). Compatibility is not promised for that surface, but consumers need to know.

| # | Item | v0.2.0 → v0.3.0 | Effect |
|---|---|---|---|
| 1 | `qualify` rejects inventory r3 | r3 accepted → only r8 and historical r7, r6, r5 accepted; r3 gives `SCHEMA_UNSUPPORTED` (`kit.Qualify`, `tsgk qualify`) | The v0.2.0 `qualification-c1.json` (r3) is rejected as is. The repository's `src/contracts/qualification-c1.json` changed to r8. Rejection was confirmed with the v0.2.0 inventory |
| 2 | `kit.QualificationInventorySchema` | `tsgk-qualification-inventory/r3` → `/r8` | A consumer that builds inventories from the constant now emits r8 |
| 3 | `kit.QualificationResultSchema` | `tsgk-qualification-result/r3` → `/r9` (constants r10–r13 added), r3 literal removed | The qualify output revision changes |
| 4 | `kit.SvcCompositeSchema` | `tsgk-svc-composite/r1` → `/r2`, r1 literal removed | The r2 composite adds `SvcDirective.DiagnosticSpans` (item 6), `AdditionalDirectives`, `SvcLanguage.Origin`/`Source` and `SvcCodeBehind.Case`. Replay checks the composite schema as r2 for the r2 format |
| 5 | `kit.SvcFormat` | `SVC-SERVICEHOST-r1` → `-r2` (r1 is the new `SvcLegacyFormat`) | The candidate code accepts `SvcLegacyFormat` as well on most paths, but external code that compared against `SvcFormat` no longer matches r1 input |
| 6 | Three new JSON keys without omitempty | `QualRole.Deferred` (`deferred`), `QualSet.BranchSelectionNotClaimed` (`branch_selection_not_claimed`), `SvcDirective.DiagnosticSpans` (`diagnostic_spans`) | The JSON output gains new keys even for the same input. The other 37 new fields on existing structs are omitempty, `json:"-"` or untagged, so output does not change. Only consumers using unkeyed struct literals break at source level |
| 7 | **`replay` results for old evidence: gate counters and gate list change** | For the same input, in the reducers `native-result-r1`/`-r2` and `oracle-set-r1`/`-r2`: `case-binding` recomputed 1 → 6, `tree` 3 → 12, oracle `api` recomputed 0 → 1, and the `oracle-set` result has 13 gates instead of 10 (`query-expectations`, `fact-reproduction`, `dynamic-sql` added), with matching entries in `coverage.observed` and `explanation` and a changed reducer description `inputs` text | **The verdict, exit code, codes, `result_schema` (`tsgk-replay-result/r1`) and findings are the same.** A tool that compares or consumes experimental `replay` results per counter or per gate list (for example a golden comparison against an old result) sees different values. The cause is that the candidate's `src/kit/replay_native.go` counts each case's step-index check and each tree's form, envelope and input-identity check in the gate. This change was not in the earlier document or the CHANGELOG, and the result schema number was not raised. It was measured that the new checks do not reject normal v0.2.0 evidence; whether damaged or tampered old evidence that v0.2.0 accepted can now be rejected was not run |
| 8 | Native runtime pin | Tree-sitter `659cda7c` → `v0.27.1` (`10eb5ac`), #213 | See the "User notice" above. Each side rejects the other's runtime with `RUNTIME_MISMATCH`, and build and producer identities differ. A documented, intentional change |

Items 2–5 are the four schema-constant revision bumps (three constants, one format string). Notes:

- `kit.TreeNode`, `kit.Point` and `kit.EditPoints` are source-compatible as aliases, but their `reflect` name, `PkgPath` and `%T` output change to `nativewire`.
- The root USAGE string does not mention `evidence query-reference`. The `evidence` sub-usage error now reads `tsgk evidence <verify|query-reference> ...`.
- `kit.OrdinaryQuerySemanticsBinding` is an alias of an unexported type, so `go doc` does not show its structure (field access works).

### Fixed (a v0.2.0 defect fixed by the candidate)

- With an empty `queries` array in the profile, `oracle record` at v0.2.0 wrote `"queries": null` into the manifest, so the record set it had just published failed its own verification with `JSON_NULL manifest#/queries` (`REFERENCE_SET_INVALID`, exit 4). The candidate writes `[]` and passes (exit 0) on the same input. This is not a direction in which the candidate rejects what v0.2.0 produced; the v0.2.0 behavior of rejecting its own set is gone.

### Measured results: candidate `aca5385`

The execution compatibility of the experimental surface was checked on candidate `aca5385` (`integ/362`) by running it side by side with the v0.2.0 binary. The v0.2.0 tree stores no evidence set, so the inputs are outputs produced by the v0.2.0 binary itself and the tests' fixtures rebuilt in the same shape. Native compile and execution ran within Windows Job Object limits. The same check is repeated on the final candidate commit ([Pre-publication checks still open](#pre-publication-checks-still-open), items 14 and 15).

| Check | Result |
|---|---|
| Replay with the candidate `tsgk replay` (reducers r1 and r2) of an `incremental` result and an `oracle record` set made by v0.2.0 (manifest r1, record r1, result r1, protocol `tsgk-native/r2`) | They replay. **The verdict (UNRESOLVED, `evidence_valid` true, exit 3), `result_schema` and findings are the same as at v0.2.0.** No input was rejected. The counters and gate list differ, as in item 7 above. UNRESOLVED arises in both versions because the `expectations` and `api` gates stay as recorded values instead of being recomputed from raw |
| A `bs-gate-compare` set, `evidence verify` (passing and tampered graph), and the documented example and `invalid/` fixtures, run with both versions | Verdict, exit code, codes and output bytes are the same |
| Replay of a candidate-made set with the v0.2.0 `replay` (reverse direction) | It replays (UNRESOLVED, valid). The counter difference is the same as in the forward direction |
| `incremental` (owned plain, and stateful with a scanner) and `oracle record` (a profile with a query), run without any new flag with both versions | The verdict (PASS) and raw response bytes are the same. The only differences are the build identity, closure, executable, `runtime_commit` and producer identity that follow the runtime pin, and one added driver compile argument (`-DTSGK_RUNTIME_MANIFEST_SHA256=…`) |
| Default path with the new flags omitted | All runs above worked normally without any new flag |
| Mutual rejection of each version's runtime | Confirmed (see "User notice" above) |

### Not checked

- Replay of real stored v0.2.0 evidence sets such as past CI artifacts (none are stored), and replay of the `private-corpus-r1` and `prepare-native-r1` reducers (they need private local-run data).
- Whether damaged or tampered old evidence is newly rejected by the candidate's extended checks (item 7 above).
- A v0.2.0 comparison of the candidate's new-flag paths (`--pins`, `--runtime-pins`, `--build-cache`, `--reuse-*`, `--ordinary-*`, `--source-witness`, `evidence query-reference`); native comparison of other routes (C#, T-SQL, XML, SVC) and large workloads; and `reproduce` with a real tree-sitter generator.

The measurements above are for candidate `aca5385`; they are repeated on the final candidate as items 14 and 15 of [Pre-publication checks still open](#pre-publication-checks-still-open), and the compatibility list is amended depending on the result.

## Remaining constraints

- macOS has no CI evidence after #332 (NOT_RUN). Evidence from before the exclusion is kept as history and no new macOS verification was run (#243 NOT_PLANNED; the new symbol role stays DEFERRED). The exclusion is not counted as PASS or as a new SUPPORTED.
- Experimental features and APIs carry no compatibility promise and do not become stable because main gains evidence.
- C/C++ macro contexts are heuristic and may accept ambiguous input without an error. Selection of non-literal conditional branches by the default C# parser is `NOT_CLAIMED`.
- Observation-limit BLOCKED items: strong macOS isolation, the unclaimed query semantics above, and the Windows-only large normal runs.
- Real consumer performance and whole-26-language accuracy are unmeasured. A verdict on registered cases does not certify arbitrary input.
- **Not claimed: comparison across the full C# declaration scope.** The comparison of obligations → independent expected values → execution evidence across the full C# declaration scope (G1) has not been done and is not claimed. What is claimed is only the residual C1–C4 corrections (#369) and the existing qualification PASS in "Product scope". [#233](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/233) stays open after shipping and continues (2026-11-12 user decision).
- CI operating-margin samples not yet judged, not product behavior: [#280](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/280), [#303](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/303), [#352](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/352), [#329](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/329). These Issues are not shipping preconditions (2026-11-12 user decision). Samples keep accumulating naturally and are judged then.

## Pre-publication checks still open

Before publication, the final candidate commit (`$sha`) is verified in the order below. No new tool is built; only the repository's tests and tools and GitHub read commands are used. Consumption checks and PR success of earlier candidates are not inherited, and if the candidate changes, B–F are redone. This list is an execution plan and has not been run yet (items 14 and 15 were measured once on the earlier candidate `aca5385` and are redone on the final candidate). PowerShell is `pwsh` (7) only; the environment is `CGO_ENABLED=0 GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly`, and Go is the toolchain matching the `go` line of `go.mod` (1.27.4). `$work` is a new empty directory outside the repository.

**A. Preconditions and candidate selection**

1. Read the state of PR #416 and the required checks (`foundation (windows-2025)`, `foundation (ubuntu-24.04)`, `integration gate`), and merge under the user's authority. Confirm that this document's base equals main.
2. Integrate this release document PR through task branch → PR → required CI → merge (the Integration section of the [validation contract](../validation/validation.md)).
3. `$sha` is main HEAD after that PR merges. B–F run only on that SHA.

**B. Candidate identity (read)**

4. `git rev-parse HEAD` == `$sha`; `git status --porcelain` and `git ls-files -u` empty; `git diff --check`.
5. `go version` matches the `go` line of `go.mod`; compare with this document's Go baseline.

**C. Local core gate**

6. `go mod download`, `go mod verify`, then `GOPROXY=off`.
7. `gofmt -l src` (no output), `go vet ./src/...`, `go build ./src/...`.
8. foundation alone: `go test ./src/internal/foundation -count=1` (some tests are load-sensitive, so run it without overlapping tests).
9. The remaining packages and the native package: the commands of the validation contract (`-timeout 300s`; native `-timeout 420s`). A test failing for a local-environment reason is recorded with its cause, and the CI Windows runner result is authoritative.

**D. Public API, CLI and schema consumption and the offline boundary**

10. `go test ./src/cmd/tsgk -run '^(TestExternalConsumerAndCLI|TestExternalConsumerSchema|TestOfflineClosure)$' -count=1 -v`
11. `go test ./src/cmd/tsgk -run '^TestModuleProxyConsumer$' -count=1 -v` (run in a git checkout; the module cache must be ready from step 6).
12. `go test ./src/cmd/tsgk -count=1`.
13. Re-compare the change list against v0.2.0: extract exported declarations (`go doc -all`), each subcommand's `-h` and the schema strings of `src/contracts/*.json` from `v0.2.0` and from `$sha`, compare them, and update the section "Public API, CLI and schema changes (against v0.2.0)" above and the [public API document](../specs/public-go-api.md). No single reproducible command exists, so this is a manual comparison.
14. **Replay of the v0.2.0 evidence set.** Replay records (an evidence set) made at v0.2.0 with the `tsgk replay` of `$sha` and record the results and error codes. If any is rejected or differs, add an item to the compatibility-break list above. **Measured on candidate `aca5385` (verdict, exit code, schema and findings the same; counter and gate-list differences are item 7 above); to be re-checked on the final candidate.**
15. **Two-version run of the native paths.** With the same input (a registered native fixture), run `incremental` and `oracle record` with the v0.2.0 binary and the `$sha` binary without any new flag, and compare. If they differ, record the cause and impact and add it to the list above. This needs approved native capabilities and a host compiler, and the runtime source pinned by each binary prepared separately (see the "User notice" above). **Measured on candidate `aca5385` (verdict and raw responses the same; the differences are the runtime-pin values); to be re-checked on the final candidate.**

**E. Source export, installation, LICENSE/NOTICE**

16. Export into a new empty directory with `git archive --format=tar -o "$work/export.tar" $sha`, unpack it where no parent has `.git`, and record the file count and bytes. Compare the paths and blob SHAs of `git ls-tree -r $sha` with `git hash-object` of each exported file, and confirm there is no forbidden extension (`.exe .dll .so .dylib .a .o .obj .lib .pdb .wasm .node`).
17. Inside the export, with `GOPROXY=off`: `go test ./src/internal/foundation -run '^(TestRepositoryFiles|TestSourceOnlyDoesNotDiscoverParentGit)$' -count=1`, `go vet ./src/...`, `go build ./src/...`.
18. Install: from the export, `go install ./src/cmd/tsgk` with `GOBIN=$work/bin`, then run the installed binary's `--help` and `inspect` on one fixture without `PATH`.
19. LICENSE/NOTICE: compare the SHA-256 of `LICENSE`, `src/dev/prepare-p05/fixtures/LICENSE.mssql`, `src/kit/data/NOTICE-index-euc-kr.md` and `src/testdata/native/samples/NOTICE.md` with the blobs at `$sha` and the fixed bytes of the previous release, and check that no new dependency or upstream license change exists via the `go.mod`/`go.sum` diff and [provenance](../provenance/upstream-sources.md). Automated test: `go test ./src/internal/foundation -run '^TestSampleNotices$' -count=1`.

**F. Required CI, fan-in, actual main**

20. On the final head of the release document PR, confirm with `gh pr checks` that `foundation (windows-2025)`, `foundation (ubuntu-24.04)`, `integration gate` and `CI mode` all succeeded. Skipped, cancelled or missing is not PASS. Just before merging, read the ruleset, checks, conversations and reviews anew.
21. After the merge, on actual main: confirm the merge SHA's `foundation.yml` run and the success of all jobs with `gh run list` and `gh run view`. For a reused run, confirm the PR run, attempt, synthetic SHA and tree in the `CI mode` log.
22. Fan-in and qualification: confirm `integration gate` succeeded and the official artifacts' digests, sizes, members, cohort and merge parents/tree, and compare the Windows/Linux derived 52-cell qualification PASS and the C# symbol 26 PASS, 0 MISMATCH, 4 NOT_APPLICABLE.
23. **Recompute the query semantics fan-in.** Run `go test ./src/kit -run '^TestQueryCompletion' -count=1 -v` and, where native and runtime are ready, `TSGK_NATIVE_REQUIRED=1 go test ./src/internal/native -run '^TestNativeQueryCompletionCurrentState$' -count=1 -v` to confirm the current VERIFIED and BLOCKED totals, gate `BLOCKED`/`complete=false` and FAILED 0. Update the figure in the "per-scope fan-in" row above (VERIFIED 360) to the actual value. The environment follows the "#362 per-scope fan-in" section of the validation contract.
24. (Optional) On the same candidate, reproduce the opt-in tests of the predicate, SVC, large and SQL public passes once, keeping limits unchanged and the HARD Job condition (the #362 section of the validation contract).
25. Read the current state of the CI operating samples (#280, #303, #352, #329) and confirm it matches the "Remaining constraints" wording. They are not preconditions.

**G. Publication proposal**

26. Present the exact version `v0.3.0`, the full commit `$sha`, an annotated-tag proposal, the Korean and English bodies (this document and the CHANGELOG v0.3.0 section with date and commit filled in), source-only scope (no binary or package), and the verified, unverified and rollback boundaries in one document, and ask for the user's publication approval. Before approval, create no tag, push no tag and create no Release.

The publication scope is the annotated source tag `v0.3.0` and a source-only GitHub Release. It excludes uploaded binaries, package-registry publication and consumer-repository changes.
