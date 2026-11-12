# v0.3.0 release document (draft, #220)

This is the English body draft of the source-only release `v0.3.0`. The Korean version is [한국어](issue-220-v030-release.md). The final tag target commit and publication date are not selected. Publication requires review of the exact version, commit, Korean/English body and source-only scope, followed by separate user approval. This draft is neither a publication approval request nor publication evidence. The tracking Issue is [#220](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/220). The earlier preparation record is the [#199 document](issue-199-source-release.en.md); its candidates (`33f84fc`, `52c1e2b`) and counts are historical records only.

## Conditions

- Every item under "Query semantics claim" below is valid only after integration PR [#416](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/416) (`integ/362`, `Refs #362`) is merged into main. Before that merge, none of those items is claimed.
- The scope already claimed (see "Product scope") is unchanged and is not reduced. The only new claim is the verified subset of query semantics.
- This document's counts are not final-release evidence until the [pre-publication checks](#pre-publication-checks-still-open) are run again on the final candidate commit.

## Product scope

Owned by [product scope](../specs/scope.md) and the [platform support contract](../specs/platform-support.md#release-v030의-claim-초안).

- **VERIFIED:** offline `inspect`, `identity`, `verify`, `schema check`/`schema diff` and their public Go API (`src/kit`). The qualification verdict for the registered 26 routes concerns fixed inputs and obligations; it does not certify arbitrary source programs. Current CI verifies Windows and Linux (a derived 52 cells, #332).
- **experimental (no compatibility promise):** `corpus`, `reproduce`, `incremental`, `oracle record`, `replay`, `evidence verify`, `qualify` and their APIs, and all query semantics verification below.
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
| per-scope fan-in | Collects the current state with machine checks; FAILED 0. The tally is VERIFIED 360 of 107,399 obligations (before the large 32 MiB rerun), the rest BLOCKED | [#413](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/413), [#414](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/414) |

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

The final-candidate comparison of public API, CLI and schema against v0.2.0 is part of the pre-publication checks and has not been done yet. The historical comparison (21 additions, nine changes, one flag) belongs only to that earlier candidate in the [#199 document](issue-199-source-release.en.md#api-cli-and-schema-consumption). Added fields on experimental structs and the SVC r1→r2 change require explicit review by consumers.

## Remaining constraints

- macOS has no CI evidence after #332 (NOT_RUN). Evidence from before the exclusion is kept as history and no new macOS verification was run (#243 NOT_PLANNED; the new symbol role stays DEFERRED). The exclusion is not counted as PASS or as a new SUPPORTED.
- Experimental features and APIs carry no compatibility promise and do not become stable because main gains evidence.
- C/C++ macro contexts are heuristic and may accept ambiguous input without an error. Selection of non-literal conditional branches by the default C# parser is `NOT_CLAIMED`.
- Observation-limit BLOCKED items: strong macOS isolation, the unclaimed query semantics above, and the Windows-only large normal runs.
- Real consumer performance and whole-26-language accuracy are unmeasured. A verdict on registered cases does not certify arbitrary input.
- The CI time-margin samples of [#280](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/280), [#303](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/303) and [#352](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/352) are not yet judged. This concerns CI operating margin, not product behavior.

## Pre-publication checks still open

Before publication, the final candidate commit must pass source export, installation, public API/CLI/schema consumption and LICENSE/NOTICE checks, plus required CI, fan-in and actual-main verification. The #220 work report owns the command list; no new tool is built. Consumption checks and PR success of earlier candidates are not inherited.

The publication scope is the annotated source tag `v0.3.0` and a source-only GitHub Release. It excludes uploaded binaries, package-registry publication and consumer-repository changes.
