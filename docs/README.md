# Documentation map

The current implementation consists of repository foundation checks, the Session 01 offline core (`tsgk inspect`, `tsgk identity`, `tsgk corpus`) the Session 02 strict profile/expected decoding and `tsgk verify` with bounded ZIP inspection, the Session 03 static `node-types.json` check and directional diff (`tsgk schema check|diff`), with the matching `src/kit` API, the Session 04 supervised process runner (`src/internal/runner`) with `tsgk reproduce`, and the Session 05 native driver (`src/drivers/native-c`, `tsgk-native/r1`) with `tsgk incremental` and the `src/kit` tree comparator, edit validation and cp949 table (`reproduce` and `incremental` are the only commands that start processes), the Session 06 native query/API record set (`tsgk oracle record`), the Session 07 offline `tsgk replay` with registered data-only reducers and `tsgk evidence verify` over an evidence graph, and the Session 08 read-only `tsgk qualify` that aggregates one candidate's three host runs into the 26 route × 3 platform qualification cells. Other commands and r0 data contracts are planned specifications, not claims of implementation.

| Task | Canonical owner |
|---|---|
| Purpose and release-candidate scope | [scope](specs/scope.md) |
| Commands, errors, profile fields | [CLI/profile](specs/cli-and-profile.md) |
| Public offline Go API, ownership, external consumers | [public API](specs/public-go-api.md) |
| Fingerprints, manifests, evidence | [identity/evidence](specs/identity-and-evidence.md) |
| Untrusted inputs, paths/archives, execution authority | [trust/execution](specs/trust-and-execution.md) |
| Static node-types contract, declaration/fact mapping, ordered CST, queries, adapters | [tree/protocol](specs/tree-and-adapter-protocol.md), [fact mapping](../src/contracts/fact-mapping.json), [fact query pack](../src/contracts/fact-query-pack.json) |
| OS/arch/capabilities | [platform](specs/platform-support.md) |
| Structure, dependencies, state transitions | [architecture](design/architecture.md), [decision record](design/decisions/0001-core-and-execution.md) |
| Pinned external references and license observations | [provenance](provenance/upstream-sources.md) |
| Generator reproduction routes, tool digests | [reproduction registry](../src/contracts/reproduction-routes.json) |
| Checks, review, merge | [validation](validation/validation.md), [workload](validation/workload-matrix.md) |
| Required syntax routes, feature scope, source feasibility | [language/feature scope](validation/language-feature-scope.md), [disposition](validation/language-feature-disposition.md), [production alternatives](../src/contracts/feature-alternatives.json), [source risks](validation/source-feature-feasibility.md) |
| NET461 WinForms/DevExpress 20.2/WCF workload, bounded .svc format, real-world source encoding/dynamic SQL/large-file policy | [업무 workload/format 등록부](validation/net461-workload.md) |
| Campaign scope, Issues/Milestones | [roadmap](roadmap.md) |
| Observed Session 00 results | [foundation report](reports/session-00-foundation.md) |
| Observed Session 03 results | [schema contract report](reports/session-03-schema-contract.md) |
| Observed Session 04 results | [reproducibility report](reports/session-04-reproducibility.md) |
| Observed Session 05 results | [incremental report](reports/session-05-incremental.md) |
| Observed Session 06 results | [native oracle report](reports/session-06-native-oracle.md) |
| Observed Session 07 results | [evidence and replay report](reports/session-07-evidence-replay.md) |
| Observed Session 08 results and 78-cell matrix (current support status: [platform](specs/platform-support.md)) | [qualification report](reports/session-08-qualification.md), [qualification inventory](../src/contracts/qualification-c1.json) |
| Historical T-SQL overacceptance and dynamic SQL fallback classification (#109) | [T-SQL 재분류](reports/issue-109-tsql-reclassification.md) |
| T-SQL acceptance boundaries and regression scope (#113) | [T-SQL 경계 수정](reports/issue-113-tsql-boundaries.md) |
| SQL Server 2025 engine comparison and first CTE correction (#121) | [T-SQL 엔진 대조](reports/issue-121-tsql-engine-validation.md) |
| CTE terminators, function target boundaries and compatibility110/170 (#122·#123) | [T-SQL 후속 경계](reports/issue-122-123-tsql-boundaries.md) |
| Whole T-SQL corpus, compatibility110/170 and Windows large API scope (#126) | [확대 검증](reports/issue-126-expanded-validation.md) |
| DEFAULT·TRY/CATCH 및 전체 corpus의 문맥 격차 (#128) | [T-SQL corpus 경계](reports/issue-128-tsql-corpus-gaps.md) |
| 공개 과잉 수용18개 원인·구문 수정·문서/엔진 불일치 (#131) | [T-SQL 과잉 수용](reports/issue-131-tsql-overacceptance.md) |
| SECURITY POLICY 문서 erratum·ALTER 작업 분리 (#132) | [SECURITY POLICY 경계](reports/issue-132-security-policy-erratum.md) |
| ASSEMBLY file_name 문서 erratum·문자열 경계 (#134) | [ASSEMBLY file_name 경계](reports/issue-134-assembly-file-name-erratum.md) |
| #137·#138·#139·#140 토큰 변형의 구문·leading dot·formal parameter 경계 | [T-SQL 토큰 경계](reports/issues-137-139-tsql-token-boundaries.md) |
| T-SQL 문맥 검사·hint·문맥별 구문 경계 (#141·#145·#146·#148) | [T-SQL 문맥 검사](reports/issue-141-tsql-context-and-hints.md) |
| DML target·cache handle·column permission·window frame·trigger 문맥 (#149) | [T-SQL 잔여 corpus 경계](reports/issue-149-tsql-corpus-context.md) |
| #150 TABLE 예약어·XML collection 구문 | [검증 보고서](reports/issue-150-tsql-identifier-xml.md) |
| #151 EXEC OUTPUT 반환 변수 | [검증 보고서](reports/issue-151-tsql-exec-output.md) |
| #157–#166·#168·#169 C# 전처리·API 식별자·T-SQL GO/list/comment 경계 | [잔여 구문 보완](reports/issues-157-169-residual-syntax.md) |
| Campaign preparation adoption and unresolved inputs | [preparation report](reports/campaign-01-2026-09-29-preparation.md) |

Read only the documents relevant to the current task. [AGENTS.md](../AGENTS.md) is the entry point for persistent project rules.

## Task-specific development rules

Ordinary development, fixes, and reviews have no session-prompt prerequisite. The [validation contract](validation/validation.md) owns Issue, task-branch/worktree, and completion requirements. Read the affected code, callers, existing checks, and relevant contracts above. A review-only request ends with findings and evidence; it does not expand into modifications or PR creation.

Use the [Issue template](../.github/ISSUE_TEMPLATE/task.md) to record change-work goals and acceptance criteria, and the [PR template](../.github/PULL_REQUEST_TEMPLATE.md) to identify the criteria satisfied by the actual diff and validation. A partial PR must identify remaining work rather than claim whole-Issue completion. Templates capture intent and evidence; they do not authorize remote posting or replace validation gates.

| Change surface | Read or update together |
|---|---|
| Go implementation or dependencies | Architecture, affected specs, regression tests, and local foundation checks |
| CLI/profile/evidence/adapter contracts | Owning spec, examples, consumers, error paths, and corresponding workload checks |
| Paths/archives or external execution | Trust/execution input boundaries, authority, failure paths, and negative tests |
| Platforms, CI, or validation policy | Platform and validation contracts, workflow, and policy-violation negative tests |
| AGENTS or development rules | Applicable task types, entry conditions, canonical ownership, links, and semantic impact |
| Meaning-preserving documentation typos or formatting | Diff, links, and formatting; the same mandatory CI applies before integration into main |

This map and the canonical documents own task-specific development rules. Do not duplicate them in a separate RULES.md. Foundation/CI checks machine-testable boundaries; review assesses meaning, review applicability, and authority. The AGENTS line limit is a size ceiling, not a quality score.

The shared `development-start` and `pr-review-workflow` skills are optional aids for preparation and PR review. This repository owns its rules and commands; installing skills is not a prerequisite for development or CI. Design command-execution enforcement separately when a concrete prohibited behavior warrants it.

## Campaign material

`docs/prompts/` and `docs/plans/` hold local execution material; `artifacts/` holds retained raw evidence and handoffs; `_ref/` holds pinned references; `.work/` is regenerable workspace. Keep these paths untracked. Public acceptance criteria must be understandable from this map and applicable Issues without local material.
