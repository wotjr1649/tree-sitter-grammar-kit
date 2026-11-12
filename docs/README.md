# Documentation map

The current implementation consists of repository foundation checks, the Session 01 offline core (`tsgk inspect`, `tsgk identity`, `tsgk corpus`) the Session 02 strict profile/expected decoding and `tsgk verify` with bounded ZIP inspection, the Session 03 static `node-types.json` check and directional diff (`tsgk schema check|diff`), with the matching `src/kit` API, the Session 04 supervised process runner (`src/internal/runner`) with `tsgk reproduce`, and the Session 05 native driver (`src/drivers/native-c`, `tsgk-native/r1`) with `tsgk incremental` and the `src/kit` tree comparator, edit validation and cp949 table. `reproduce`, `incremental`, and `oracle record` can start processes. Session 06 provides the native query/API record set (`tsgk oracle record`); Session 07 provides offline `tsgk replay` with registered data-only reducers and `tsgk evidence verify` over an evidence graph; Session 08 provides read-only `tsgk qualify` that aggregates one candidate's three host runs into the 26 route × 3 platform qualification cells. Other commands and r0 data contracts are planned specifications, not claims of implementation.

| Task | Canonical owner |
|---|---|
| Purpose and release-candidate scope | [scope](specs/scope.md) |
| 목적·설계 이유, 전체 grammar 지원 버전과 테스트·제외 경계 안내 | [한국어](grammar-support.md) · [English](grammar-support.en.md) (계약 소유자는 각 절의 링크 참조) |
| Commands, errors, profile fields | [CLI/profile](specs/cli-and-profile.md) |
| Public offline Go API, ownership, external consumers | [public API](specs/public-go-api.md) |
| Fingerprints, manifests, evidence | [identity/evidence](specs/identity-and-evidence.md) |
| Untrusted inputs, paths/archives, execution authority | [trust/execution](specs/trust-and-execution.md) |
| Static node-types contract, declaration/fact mapping, ordered CST, queries, adapters | [tree/protocol](specs/tree-and-adapter-protocol.md), [fact mapping](../src/contracts/fact-mapping.json), [fact query pack](../src/contracts/fact-query-pack.json) |
| OS/arch/capabilities | [platform](specs/platform-support.md) |
| Structure, dependencies, state transitions | [architecture](design/architecture.md), [decision record](design/decisions/0001-core-and-execution.md) |
| Pinned external references and license observations | [provenance](provenance/upstream-sources.md) |
| #370 역사 자료·reference/runtime 검증, 단일 생산 선택과 공개 pin 전달 | [기본 승인 집합](specs/query-reference-eligibility.md#370-v2-e-legacy-기본-승인-집합), [pin 입력·소비 계약](specs/query-reference-eligibility.md#370-v2-b2-공개-reference-pin-전달), [runtime reader](specs/query-reference-eligibility.md#370-v2-c-승인-historical-runtime-검증), [logger replay/qualification](specs/query-reference-eligibility.md#370-v2-c2-일반-logger-replayqualification-opt-in), [accepted/CST reader](specs/query-reference-eligibility.md#370-v2-c3-accepted-eventpreprocess-cst-runtime-opt-in), [Roslyn 승인](specs/query-reference-eligibility.md#370-v2-d-roslyn-승인-관측과-provenance), [생산 연결](specs/instrumented-query-reference.md#370-v2-b2-단일-생산-pin), [공개 API](specs/public-go-api.md) |
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
| 핵심 소비자 언어의 고정 입력·SDK 대조, CST 편집 복원과 미해결 경계 (#198) | [한국어](reports/issue-198-core-language-holdout.md) · [English](reports/issue-198-core-language-holdout.en.md) |
| 대형 native digest 처리 비용과 Windows WALL_LIMIT 조사 (#208) | [한국어](reports/issue-208-native-digest-cost.md) · [English](reports/issue-208-native-digest-cost.en.md) |
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
| #157–#166·#168–#170 C# 전처리·API 식별자·T-SQL 구문·incremental 근거 | [잔여 구문 보완](reports/issues-157-169-residual-syntax.md) |
| #172–#176 SVC composite·복구·참조·zero-width runtime 탐색 | [SVC/runtime 보완](reports/issues-172-176-svc-and-runtime.md) |
| #178·#179 previous sibling identity·inactive C# directive·SVC 회귀 | [잔여 runtime/C# 보완](reports/issues-178-179-residual-runtime-csharp.md) |
| #182–#185 C# conditional fragment·SVC source·record replay 보완 | [조건부 fragment와 receipt 보완](reports/issue-182-csharp-conditional-fragments.md) |
| #188 로컬 corpus full CST/API·원문 후보·방언 지원 경계 | [corpus 후속 검증](reports/issue-188-corpus-cst-api.md) |
| #193 성능 기준선·독립 정확도·소비자 도입 평가 | [후속 평가](reports/issue-193-evaluation.md) |
| #197 선택적 logger 관측·source witness·기존 판정 유지 | [한국어](reports/issue-197-reuse-observation.md), [English](reports/issue-197-reuse-observation.en.md) |
| #199 다음 source-only release의 API 변화·소비 검증·미완료 발행 준비 | [한국어](reports/issue-199-source-release.md) · [English](reports/issue-199-source-release.en.md) |
| #205 Go 식별자 범주·Unicode 17 수정과 로컬 검증 | [한국어](reports/issue-205-go-identifiers.md), [English](reports/issue-205-go-identifiers.en.md) |
| #210 Windows native suite 빌드 직렬화 비용과 300초 timeout | [한국어](reports/issue-210-native-suite-build-cost.md) · [English](reports/issue-210-native-suite-build-cost.en.md) |
| #217 오류 tree route BLOCKED 원본 처분·reuse holdout 원장·미처분 검사 | [처분 보고서](reports/issue-217-reuse-dispositions.md) |
| #264 accepted event·전체 CST·고정 원장·replay·qualification | [증거 계약](specs/reuse-accepted.md) |
| #216 Windows JSON24 실제 native/reference closure·외부 승인·API/replay | [검증 보고서](reports/issue-216-json24-reference.md) |
| #216 driver·reference library binary paging, manifest/r7·replay r6와 예산·변이 검사 | [binary paging 계약](specs/instrumented-query-reference.md#216-binary-paging), [검증 계약](validation/validation.md#216-binary-paging-계약-검증) |
| #362 전체 의미 의무의 원문 pin·exact membership·typed incomplete 계약 (P0) | [semantic registry](specs/instrumented-query-reference.md#362-전체-query-의미-의무-등록부-p0), [identity/evidence binding](specs/identity-and-evidence.md#362-full-scope-registry-p0-membership-contract), [registry 정적 검증](validation/validation.md#362-query-semantic-registry-p0-정적-검증), [registry r1](../src/contracts/query-semantic-registry-r1.json) |
| #362 기존 네 predicate의 독립 평가 20 tuple과 거부·미완료 정책 47행 (범위 3) | [predicate 범위](specs/instrumented-query-reference.md#362-predicate-의미-의무와-거부-정책-범위-3), [검증](validation/validation.md#362-predicate-의미-의무거부-정책-검증-범위-3), [expected](../src/contracts/query-predicate-expected-r1.json) |
| #362 SVC69 사례·41 edits·두 query의 독립 등록과 확장 분모 (범위 5) | [SVC 범위](specs/instrumented-query-reference.md#362-svc-사례의-독립-등록과-확장-분모-범위-5), [검증](validation/validation.md#362-svc-독립-등록-검증-범위-5), [expected](../src/contracts/query-svc-expected-r1.json) |
| #362 scope별 결과(ordinary·predicate·SVC·large)의 fan-in과 completion gate (범위 7) | [fan-in 계약](specs/instrumented-query-reference.md#362-scope별-결과의-fan-in과-completion-gate-범위-7), [검증](validation/validation.md#362-scope별-fan-incompletion-gate-검증-범위-7), [revision 경계](specs/identity-and-evidence.md) |
| #362 SVC69 사례·41 edits·두 query의 독립 등록과 확장 분모 (범위 5) | [SVC 범위](specs/instrumented-query-reference.md#362-svc-사례의-독립-등록과-확장-분모-범위-5), [검증](validation/validation.md#362-svc-독립-등록-검증-범위-5), [expected](../src/contracts/query-svc-expected-r1.json), [inline expected](../src/contracts/query-svc-inline-expected-r1.json) |
| #362 Windows large 정상3+over-limit1의 독립 등록과 paging (범위 6) | [large 범위](specs/instrumented-query-reference.md#362-windows-large-사례의-독립-등록과-paging-범위-6), [검증](validation/validation.md#362-windows-large-독립-등록-검증-범위-6), [expected](../src/contracts/query-large-expected-r1.json) |
| #362 원문3,201·edit1,852 복원, version 공유와130 query 규칙 골격 (P1) | [원문·규칙 계약](specs/instrumented-query-reference.md#362-원문-witness-복원과-기대값-규칙-대장-p1), [복원 검증](validation/validation.md#362-p1-원문-복원독립-기대-규칙-검증), [규칙 대장](../src/contracts/query-expectation-rules-r1.json) |
| #362 ordinary 사전등록·P0 registry core 공용화·capacity shard plan (P2-A) | [ordinary plan 계약](specs/instrumented-query-reference.md#362-ordinary-사전-등록과-capacity-plan-p2-a), [ID와 asset binding](specs/identity-and-evidence.md#362-full-scope-registry-p0-membership-contract), [admission 한도](specs/query-reference-eligibility.md#362-ordinary-admission-입력-범위), [검증 계약](validation/validation.md#362-p2-a-ordinary-preregistration과-capacity-plan) |
| #362 ordinary producer·file-backed batch·API 비교기·step/role fact (P2-B) | [producer 계약](specs/instrumented-query-reference.md#362-ordinary-batch-capture-준비-p2-b-일부), [record/r5·manifest/r8와 owner](specs/identity-and-evidence.md#362-batch-capture의-process-소유권-p2-b-일부), [검증](validation/validation.md#362-p2-b-actual-capture-기반-검증) |
| #362 selected shard plan/registration 검증과 Merkle commitment (P2-C 부분) | [plan·승인 commitment 계약](specs/instrumented-query-reference.md#362-shard-commitment와-선택-plan-검증-p2-c), [approval identity](specs/identity-and-evidence.md#362-ordinary-reader-identityapprovalpartial-fold-p2-c-boundary), [eligibility 입력 binding](specs/query-reference-eligibility.md#362-승인된-shard-plan-commitment), [검증 계약](validation/validation.md#362-ordinary-shard-plan-commitment-검증) |
| #362 ordinary coordinator·experimental public API (P2-C; selected JSON/SQL measurements recorded, P2-D synthetic r9 positive noted, SQL admission unproven) | [reader·expected·capacity boundary](specs/instrumented-query-reference.md#362-ordinary-guarded-reader와-independent-adjudicator-p2-c), [approval·session identity](specs/identity-and-evidence.md#362-ordinary-reader-identityapprovalpartial-fold-p2-c-boundary), [eligibility binding](specs/query-reference-eligibility.md#362-p2-c-ordinary-eligibility-activation-boundary), [public API and result schema](specs/public-go-api.md#362-p2-c-ordinary-public-api-readiness), [acceptance validation](validation/validation.md#362-p2-c-ordinary-guarded-reader-활성화-전-검증-계약) |
| #362 ordinary stored decoder and r9 replay (P2-D; full qualification and SQL capacity excluded) | [stored API decoder](specs/public-go-api.md#362-p2-d-ordinary-저장-결과와-replay), [r9 identity and result/r7](specs/identity-and-evidence.md#362-ordinary-stored-replay-p2-d), [existing replay CLI](specs/cli-and-profile.md#362-p2-d-ordinary-replay-cli), [replay and source-only boundary](specs/instrumented-query-reference.md#362-ordinary-replaystored-decoder-p2-d), [validation](validation/validation.md#362-p2-d-ordinary-stored-resultreplay-parity) |
| #362 ordinary `oracle record` CLI (source-only loader 누적 원장·경로 결속) | [CLI 계약](specs/cli-and-profile.md#362-ordinary-record-cli), [검증](validation/validation.md#362-ordinary-record-cli-검증) |
| #216 Go3·ordinary 정의의 정적 연결과 당시 Windows metadata 관측 범위 | [분류 보고서](reports/issue-216-go3-classification.md) |
| #216 C·Java·Rust13 정의 연결과 당시 Windows 구조 분류 관측 범위 | [분류 보고서](reports/issue-216-cjr13-classification.md) |
| #216 CPP4 parser paging·독립 수집과 당시 Windows 조건부 구조 분류 | [분류 보고서](reports/issue-216-cpp4-classification.md) |
| #216 JSON1·YAML1·XML2·HTML2·CSS11의 당시 Windows 조건부 구조 분류 | [Small17 보고서](reports/issue-216-small17-classification.md) |
| #216 Web16의 Windows 관측·외부 승인과 조건부 정의 분류 | [Web16 보고서](reports/issue-216-web16-classification.md) |
| #216 Python8·Ruby4·R3·Bash2의 닫힌 profile과 독립 사전 기대값 | [Script17 계약](specs/instrumented-query-reference.md#script17-단일-frame-정의-분류) |
| #216 Dart7·PHP5의 닫힌 profile과 독립 사전 기대값 | [DartPHP12 계약](specs/instrumented-query-reference.md#dartphp12-단일-frame-정의-분류) |
| #216 Kotlin5의 닫힌 paged profile과 독립 사전 기대값 | [Kotlin5 계약](specs/instrumented-query-reference.md#kotlin-5개-정의의-parser-paging-분류) |
| #216 Swift8의 닫힌 paged profile과 독립 사전 기대값 | [Swift8 계약](specs/instrumented-query-reference.md#swift-8개-정의의-parser-paging-분류) |
| #216 PostgreSQL9의 닫힌 paged profile, fact pack 결속과 독립 사전 기대값 | [PostgreSQL9 계약](specs/instrumented-query-reference.md#postgresql-9개-정의의-parser-paging-분류) |
| #216 TSQL8의 관측 전 닫힌 profile, 일반6+fact2와 binary paging 연결 | [TSQL8 source 준비 계약](specs/instrumented-query-reference.md#tsql-8개-정의의-binary-paging-분류-준비) |
| #216 TSQL8의 Windows A6 관측·조건부 정의 분류와 130/130 local candidate의 한계 | [TSQL8 보고서](reports/issue-216-tsql8-classification.md) |
| #216 PowerShell3의 닫힌 1-byte profile과 관측 전 사전 기대값 | [PowerShell3 계약](specs/instrumented-query-reference.md#powershell3-단일-byte-frame-정의-분류) |
| #233 C# lexical producer의 네 directive 오판·실제 ERROR 보존 | [교정 보고서](reports/issue-233-csharp-lexical-recovery.md) |
| #233 kit 내부 identity 채택 경로·replay subject platform 결속 | [검증 경로와 반례](reports/issue-233-identity-paths.md) |
| Explicitly assigned Campaign 01 acceptance, Session rules, and preparation history | [campaign contract and preparation report](reports/campaign-01-2026-09-29-preparation.md#campaign-specific-acceptance-and-execution) |

Read only the documents relevant to the current task. [AGENTS.md](../AGENTS.md) is the entry point for persistent project rules.

## Task-specific development rules

계측 query reference의 독립 identity·완료 조건·빌드·owned 대조와 전체 지원 대상 연결 기준은 [계측 query reference 계약](specs/instrumented-query-reference.md)을 따른다.

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

Query reference의 caller-trusted 승인, build/control closure 및 native/r4 저장 일관성은 [offline eligibility 계약](specs/query-reference-eligibility.md)을 따른다.

일반 query 슬롯의 정적 분류·전개 규칙·추가 역할 보존은 [정적 슬롯 계약](specs/instrumented-query-reference.md#216-정적-query-슬롯-원장)과 [고정 원장](../src/contracts/query-slot-inventory-r1.json)을 따른다. 잠재 슬롯 수는 semantic 필수 분모가 아니다.
