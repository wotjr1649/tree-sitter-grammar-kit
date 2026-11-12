# v0.3.0 release 문서 (초안, #220)

이 문서는 source-only release `v0.3.0`의 한국어 본문 초안이다. 영어판은 [English](issue-220-v030-release.en.md)다. 최종 tag 대상 commit과 발행일은 미정이며, 발행은 정확한 version·commit·한영 본문·source-only 범위를 검토한 뒤 사용자가 별도로 승인한다. 이 초안은 발행 승인 요청이나 발행 완료 증거가 아니다. 추적 Issue는 [#220](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/220)이다. 이전 준비 기록은 [#199 문서](issue-199-source-release.md)이며, 그 안의 후보(`33f84fc`, `52c1e2b`)와 수치는 역사 기록으로만 둔다.

## 조건

- 아래 "query 의미 claim"의 모든 항목은 통합 PR [#416](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/416)(`integ/362`, `Refs #362`)이 main에 병합된 뒤에만 유효하다. 병합 전에는 어떤 항목도 주장하지 않는다.
- 이미 주장하던 범위(아래 "제품 범위")는 새로 줄이거나 넓히지 않는다. 다만 macOS는 v0.2.0 시점 근거만 있고 v0.3.0에서 재검증하지 않았다(NOT_RUN, 제외를 PASS로 세지 않는다). 새로 주장하는 것은 query 의미의 검증된 하위 범위뿐이다.
- 최종 후보 commit에서 [발행 전 남은 검증](#발행-전-남은-검증)을 다시 실행하기 전에는 이 문서의 수치를 최종 release 근거로 쓰지 않는다. fan-in 수치는 최종 후보에서 다시 계산해 갱신한다.

## 제품 범위

[제품 범위](../specs/scope.md)와 [플랫폼 지원 계약](../specs/platform-support.md#release-v030의-claim-초안)이 정한다.

- **VERIFIED:** offline `inspect`, `identity`, `verify`, `schema check`/`schema diff`와 해당 공개 Go API(`src/kit`). 등록 26 route qualification 판정은 고정 입력·의무에 대한 것이며 임의 입력의 문법 적합성 인증이 아니다. 현재 CI의 검증 host는 Windows·Linux다(derived 52칸, #332).
- **experimental(호환성 약속 없음):** `corpus`, `reproduce`, `incremental`, `oracle record`, `replay`, `evidence verify`, `qualify`와 그 API, 그리고 아래 query 의미 검증 전부.
- **C#:** 검증된 범위만 주장한다. C1–C4 잔여 교정(PR [#369](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/369))과 기존 qualification PASS(C# symbol 26 PASS·0 MISMATCH·4 NOT_APPLICABLE)다. C# 전체 선언 범위의 의무 → 독립 기대값 → 실행 근거 대조는 주장하지 않는다(아래 "잔존 제약").
- **소비자 요구 사항: Go 1.27.4 이상.** `go.mod`의 `go` 지시어가 1.27.1에서 1.27.4로 올랐다(#358). 자세한 내용은 아래 "공개 API·CLI·schema 변화(v0.2.0 대비)"에 있다.
- 발행물은 source tag와 source-only GitHub Release다. binary와 package는 없다.

## query 의미 claim (experimental)

v0.3.0은 query 의미를 experimental로 유지하고, 실제로 검증한 하위 범위만 주장한다. 나머지는 BLOCKED 사유와 함께 "미주장"으로 적는다. 2026-11-12 사용자 결정으로 [#362](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/362)의 completion gate는 #220 출하 조건이 아니다. 이는 이미 experimental인 기능의 주장 문구를 검증된 사실과 맞추는 정합화이며 지원 범위를 줄이는 것이 아니다.

### 주장하는 범위

모두 Windows에서, 실제 owned native producer와 등록 한도(logical 1 GiB, work 32,000,000, read 256 MiB; large는 등록된 large 예산)를 바꾸지 않고 얻은 결과다. 독립 expected는 grammar·query 원문에서 도출해 native 첫 실행 전에 봉인했다. 예외가 있다. TSQL의 dynamic SQL capture asset 8개(107개 중)는 첫 대조 뒤 소스 근거로 교정했고, TSQL wildcard child pattern의 definite 여부(`tsqlDefinite[12]`)는 native 관측과 맞춘 모델 매개변수라 해당 pattern에 한해 독립 증거가 아니다. large는 봉인 뒤 capture 위치 한 줄을 정정했다.

| 범위 | 검증된 사실 | 근거 |
|---|---|---|
| predicate(`eq?`, `not-eq?`, `any-of?`, `not-any-of?`) | 독립 평가 tuple 20/20 VERIFIED, 거부·미완료 정책 47/47 정확한 거부. 새 predicate 지원은 없다 | [#408](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/408), [expected](../../src/contracts/query-predicate-expected-r1.json), [계약](../specs/instrumented-query-reference.md#362-predicate-의미-의무와-거부-정책-범위-3) |
| SVC 오프라인 의무 | 263개 VERIFIED(history 110, edit point 41, segment 109, CodeBehind 2, negative 1) | [#410](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/410), [expected](../../src/contracts/query-svc-expected-r1.json) |
| SVC inline C# | tree 82개 중 30, 두 query 164개 중 60이 실제 C# grammar native와 일치(`facts.dynamic_sql` fact 0개 포함) | [#415](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/415), [expected](../../src/contracts/query-svc-inline-expected-r1.json) |
| Windows large | 39개 중 31 VERIFIED: `cs-large-22m` 26(source 1, status 1, CST page 12, footer 1, occurrence page 10, footer 1), 오류 복구 fixture 둘의 source·status 4, over-limit control 1. `cs-large-22m`은 전체 CST(node 11,463,151, page 12)·occurrence가 native와 일치한다. `cs-large-32mib-errors` status는 등록 8 GiB HARD 한도에서 `COMPLETED`(syntax ERROR)다. control은 `RESOURCE_LIMIT`/`OUTPUT_LIMIT`로 정확히 거부됐다는 확인이며 의미 PASS가 아니다. 31번째(32 MiB status)는 이 한 사례를 따로 재실행한 기록이며 아래 fan-in 시험에는 들어 있지 않다(시험은 30으로 고정) | [#412](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/412), [#415](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/415), [expected](../../src/contracts/query-large-expected-r1.json) |
| ordinary 선택 SQL shard 공개 pass | TSQL `alt-tsql-B01-r001` 72/72, PostgreSQL `alt-postgresql-sql-B04-22` 9/9(독립 10-node CST, 빈 capture·fact) VERIFIED. 한도 불변, TSQL 남은 여유는 work 7.3%·logical 3.47%로 얇다 | [#416](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/416), [#403](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/403) |
| ordinary JSON 양성 | 여러 case(2/2)와 편집 이력(3 edit·4 version, 14/14)의 공개 API·r9 replay·`tsgk replay` 결과 일치(저장소에 추적되는 JSON grammar fixture) | [#404](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/404), [#402](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/402) |
| scope별 fan-in | 현재 상태를 기계 검증으로 모으며 FAILED 0. 집계는 전체 107,399 의무 중 VERIFIED 360, 나머지 BLOCKED다. 시험이 반영하는 것은 predicate 67·SVC 오프라인 263·large 30(#412 기록)이다. SVC inline 90, large 32 MiB status 1, ordinary 선택 shard(TSQL·PostgreSQL·JSON)는 fan-in 밖에서 따로 검증했다. 분모 107,399는 #362 Issue 본문의 107,364에서 large 상위 의무 4개를 하위 39개로 바꾼 값이다(차이 35). 이 수치는 최종 후보 commit에서 시험이 고정한 값(360)과 같은지 확인한다 | [#413](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/413), [#414](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/414) |

위 「근거」 열의 개별 PR(#402·#403·#404·#408·#410·#412·#413·#414·#415)은 #416에 통합돼 병합 없이 닫혔다(통합 전 개별 검토 기록).

### 주장하지 않는 범위(BLOCKED 사유)

| 미주장 | 사유 |
|---|---|
| 오류 복구(ERROR·MISSING)가 생기는 입력의 CST·capture | grammar 규칙만으로 결과가 정해지지 않아, 결과 복사 golden 없이는 독립 expected를 만들 수 없다. SVC inline 8 step, large `cs-large-8m-errors`·`cs-large-32mib-errors`의 CST·occurrence 8개가 해당한다. 이 8 step의 query 16개도 미주장이다 |
| 전처리 scanner 상태가 필요한 SVC inline | 44 step(`#if`·`#else`·`#define`·`#endif`). external scanner의 상태 기계를 규칙에서 독립 도출하지 못했다. 이 step의 query 88개도 미주장이다(오류 복구 8 step의 16개와 합쳐 104개) |
| Linux 몫 | predicate 20+47과 SVC의 Linux 몫은 등록만 되어 있고 CI 수집 경로가 없다. large는 Windows 전용 workload라 Linux·macOS가 NOT_APPLICABLE이며 Linux 몫이 없다 |
| ordinary 전량 | 전체 ordinary 106,208 tuple 중 위 선택 shard 외에는 검증하지 않았다. binary-paged TSQL `alt-tsql-B04-r144`는 독립 expected가 없어 8/8 BLOCKED다 |
| reference row·approval | 공개 pass 시험의 reference row와 approval은 synthetic이다. 실제 외부 reference 실행이나 caller 승인 workload의 증거가 아니다 |
| native producer의 predicate 평가 | native producer의 predicate capability는 `NOT_EVALUATED`로 유지한다. 위 predicate 20/20은 별도 typed 검증 경로(`VerifyQueryWitness` 재사용)의 결과다 |

### completion gate

#362의 completion gate는 닫힌 상태(`BLOCKED`, `complete=false`)로 출하한다. 현재 판정 사유는 ordinary 저장 결과가 없는 것(`QUERY_SCOPE_RESULT_ABSENT`)이며 large 오류 복구 분모도 미완이라 이 revision은 COMPLETE가 될 수 없다. #362는 출하 뒤에도 열어 두고, 오류 복구 입력의 독립 expected 기준·Linux 수집·ordinary 전량을 이어서 다룬다.

## 이전 release 이후 변경

v0.2.0 이후 변경의 목록은 [변경 기록](../../CHANGELOG.md)의 v0.3.0 절이 소유한다. 큰 묶음은 다음과 같다.

- 경로 안전성(#196, #215), 선택적 logger 관측(#197), 오류 tree route BLOCKED 전 old-tree 전달 관측(#217), accepted reuse 증거(#264).
- C# 조건부 symbol 모드(#218)와 lexical recovery 교정(#233, C1–C4 반영 #369).
- 버전 독립 historical 검증(V2, #393)과 query 의미 검증 기반(#362 P0–P2, 예산 절감 #385, 원문 복원 P1 #367).
- Go 기준 1.27.4(#358), Windows runner Job Object 교정(#330), CI 운영 변경(macOS 제외 #332 등). CI 변경은 제품 동작을 바꾸지 않는다.

공개 API·CLI·schema 변화는 아래 절에 있다. 과거 후보의 대조(추가 21·변경 9, flag 하나)는 [#199 문서](issue-199-source-release.md)의 당시 후보에만 귀속한다.

## 공개 API·CLI·schema 변화(v0.2.0 대비)

비교 기준은 tag `v0.2.0`과 통합 후보 `integ/362`(PR #416)다. 분석은 exported 선언의 정규화 비교(`src/kit/...`, `_test.go` 제외), `tsgk` 각 하위 명령의 `-h` flag 목록 비교, schema 상수·literal과 `src/contracts/**/*.json`의 `schema` 필드 비교로 했다. 분석 뒤 #416의 리뷰 교정이 더한 exported 요소(아래 표의 "교정 반영")까지 반영했다. 최종 후보 commit에서의 재대조는 [발행 전 남은 검증](#발행-전-남은-검증)에 있다.

> **소비자 요구 사항: Go 1.27.4 이상.** `go.mod`의 `go` 지시어가 `1.27.1`에서 `1.27.4`로 올랐다(#358). 이 module을 build하는 소비자는 Go 1.27.4 이상이 필요하다. toolchain 자동 전환이 꺼져 있으면 낮은 버전에서 build가 실패한다. stable 표면의 소비자에게도 해당한다.

> **사용자 안내: native runtime pin이 바뀌었다(#213).** `incremental`·`oracle record` 등 native 경로가 쓰는 Tree-sitter runtime source가 commit `659cda7c7f86ebe31cc825dc5da59e9add172dc7`(v0.2.0)에서 release tag `v0.27.1`(commit `10eb5ac119eec9cb4267a8e035066473983f5f3a`)로 올랐다. `--runtime`에 줄 디렉터리는 사용하는 `tsgk`의 pin과 같은 runtime이어야 하며, 다르면 두 방향 모두 `RUNTIME_MISMATCH`(`lib/include/tree_sitter/api.h: identity mismatch`, exit 2, `NOT_RUN`)로 거부된다. v0.2.0용으로 준비한 runtime 디렉터리를 후보 `tsgk`에 그대로 쓸 수 없고, 반대도 마찬가지다. 같은 grammar·입력이라도 build identity·driver executable·runtime commit·producer identity가 달라진다. pin의 정본은 `src/drivers/native-c/runtime-manifest.json`이고 근거는 [upstream 출처](../provenance/upstream-sources.md)에 있다.

### stable 표면: 깨짐 없음

- 공개 Go API: exported 식별자의 삭제와 기존 함수·method의 시그니처 변경이 없고, 기존 interface에 더해진 method도 없다.
- CLI: `inspect`, `identity`, `verify`, `schema check`/`schema diff`(그리고 experimental인 `corpus`, `reproduce`)의 flag 목록·기본값·설명이 v0.2.0과 같다. 새 flag의 기본값은 모두 빈 값 또는 false다.
- schema: stable API가 쓰는 schema 상수(`ReportSchema`, `ProfileSchema`, `ExpectedSchema`, `ManifestSchema`, `FileSetSchema`, `PolicySchema` 계열, `SchemaFormat`, `SchemaComparator`, `TreeSummarySchema` 등)의 값이 바뀌지 않았다.
- 동작: v0.2.0의 plain fixture에서 `inspect`, `identity`, `schema check`, `schema diff`를 두 버전으로 실행한 stdout이 byte 단위로 같다(exit 0). 후보 `aca5385`에서 `corpus`, `verify`(통과·변조·expected 없음 세 경우), 잘못된 `schema check`, 루트 명령 오류, 저장소의 `ordinary-json` fixture에 대한 `inspect`·`corpus`도 stdout과 exit가 byte 단위로 같았다. experimental인 `reproduce`는 같은 JSON-mode fixture에서 판정·산출물(`parser.c`)이 같고 시각·memory 수치만 달랐다(실제 tree-sitter generator는 쓰지 못해 같은 동작의 시험용 generator를 썼다).
- 단, 위 Go 최소 버전 상승은 stable 소비자에게도 적용된다.

### 변경·추가·삭제 개수

| 표면 | 변경 | 삭제 | 추가 |
|---|---:|---:|---:|
| Go `src/kit/...` | 7(type 3, const 4) | 0 | 1,826 요소: 새 최상위 323(func 59, type 142, const 122), 새 type의 멤버 945, 기존 type의 새 멤버 42, 새 하위 package 5개 516(`nativewire`, `querypolicy`, `queryregistrycore`, `querywire`, `reuseobs`) |
| CLI `tsgk` | 도움말 문구 2(`qualify --inventory`, `evidence` 오류 문구) | 0(subcommand·flag) | 새 subcommand 1(`evidence query-reference`), 새 flag 27(기존 subcommand 22, 새 subcommand 5) |
| schema | exported 상수 값 3 + format 문자열 1, contracts JSON 1(`qualification-c1.json`) | `src/kit`의 literal 3 | exported 상수 66, `src/kit` 새 family 74, contracts JSON 새 파일 16 |

- Go "변경" 7개는 v0.2.0 식별자 기준이다. struct였던 `TreeNode`, `Point`, `EditPoints`가 `nativewire`의 같은 struct에 대한 alias가 됐고(field·tag 동일), 값이 바뀐 const 4개는 아래 표에 있다. 식별자의 삭제는 없다.
- "교정 반영"은 #416 리뷰 교정(`0ad8aef`)이 더한 exported 요소 3개(상수 `SvcInlineRoleIncremental`·`SvcInlineRoleFresh`, field `SvcInlineView.Role`)다. 모두 v0.3.0에서 새로 생긴 SVC inline type에 속한다.
- 새 표면은 위 "제품 범위"의 experimental 기능에 속한다. VERIFIED로 주장하는 범위는 v0.2.0과 같다.

### experimental 표면의 호환성 깨짐

v0.2.0 claim의 experimental 표면(`qualify`, `incremental`, `replay`, SVC 기록, native tree·edit helper)에 대한 변화다. 호환성은 약속하지 않는 표면이지만 소비자가 알아야 하므로 적는다.

| # | 항목 | v0.2.0 → v0.3.0 | 영향 |
|---|---|---|---|
| 1 | `qualify`의 inventory r3 거부 | r3 수용 → r8과 historical r7·r6·r5만 수용, r3은 `SCHEMA_UNSUPPORTED`(`kit.Qualify`, `tsgk qualify`) | v0.2.0의 `qualification-c1.json`(r3)을 그대로 주면 거부된다. 저장소의 `src/contracts/qualification-c1.json`은 r8로 바뀌었다. v0.2.0 inventory로 실제 거부됨을 확인했다 |
| 2 | `kit.QualificationInventorySchema` | `tsgk-qualification-inventory/r3` → `/r8` | 상수로 inventory를 만들던 소비자는 r8을 낸다 |
| 3 | `kit.QualificationResultSchema` | `tsgk-qualification-result/r3` → `/r9`(r10–r13 상수 추가), r3 literal 제거 | qualify 출력 revision이 바뀐다 |
| 4 | `kit.SvcCompositeSchema` | `tsgk-svc-composite/r1` → `/r2`, r1 literal 제거 | r2 composite에 `SvcDirective.DiagnosticSpans`(6번 항목)·`AdditionalDirectives`·`SvcLanguage.Origin`/`Source`·`SvcCodeBehind.Case`가 더해진다. replay는 r2 format에서 composite schema를 r2로 검사한다 |
| 5 | `kit.SvcFormat` | `SVC-SERVICEHOST-r1` → `-r2`(r1은 새 `SvcLegacyFormat`) | 후보 코드는 대부분의 경로에서 `SvcLegacyFormat`도 받지만, `SvcFormat`과 비교하던 외부 코드는 r1 입력을 더 이상 맞추지 못한다 |
| 6 | omitempty 없는 새 JSON key 3개 | `QualRole.Deferred`(`deferred`), `QualSet.BranchSelectionNotClaimed`(`branch_selection_not_claimed`), `SvcDirective.DiagnosticSpans`(`diagnostic_spans`) | 같은 입력에도 JSON 출력에 새 key가 생긴다. 이 밖의 기존 struct 새 field 37개는 omitempty·`json:"-"`·tag 없음이라 출력이 바뀌지 않는다. unkeyed struct literal을 쓰는 소비자만 source 수준에서 깨진다 |
| 7 | **옛 evidence의 `replay` 결과: gate counter·gate 목록 변화** | 같은 입력에서 `native-result-r1`·`-r2`, `oracle-set-r1`·`-r2` reducer의 `case-binding` recomputed 1 → 6, `tree` 3 → 12, oracle `api` recomputed 0 → 1, `oracle-set` 결과의 gate 10개 → 13개(`query-expectations`, `fact-reproduction`, `dynamic-sql` 추가)와 `coverage.observed`·`explanation`의 대응 항목, reducer 설명 `inputs` 문구 | **판정·exit·code·`result_schema`(`tsgk-replay-result/r1`)·findings는 같다.** 그러나 experimental `replay` 결과를 counter나 gate 목록 단위로 비교하거나(예: 옛 결과와의 golden 비교) 소비하는 도구는 값이 달라진다. 원인은 후보 `src/kit/replay_native.go`가 case의 step index와 tree의 form·envelope·입력 identity 검사를 각각 gate에 세도록 넓힌 것이다. 이 변화는 이전 문서와 CHANGELOG에 없었고 결과 schema 번호도 올리지 않았다. 새 검사가 정상 v0.2.0 evidence는 거부하지 않음을 실측했으나, 손상됐거나 변조된 옛 evidence가 v0.2.0에서는 통과하고 후보에서는 거부될 수 있는지는 실행하지 않았다 |
| 8 | native runtime pin | Tree-sitter `659cda7c` → `v0.27.1`(`10eb5ac`), #213 | 위 "사용자 안내"를 본다. 서로의 runtime을 `RUNTIME_MISMATCH`로 거부하며 build·producer identity가 달라진다. 문서화된 의도적 변경이다 |

위 2–5가 schema 상수 revision bump 4건(상수 3, format 문자열 1)이다. 참고:

- `kit.TreeNode`, `kit.Point`, `kit.EditPoints`는 alias가 되어 source는 호환이지만 `reflect` 이름·`PkgPath`·`%T` 출력이 `nativewire`로 바뀐다.
- 루트 USAGE 문자열은 `evidence query-reference`를 언급하지 않는다. `evidence` 하위 USAGE 오류 문구는 `tsgk evidence <verify|query-reference> ...`로 바뀌었다.
- `kit.OrdinaryQuerySemanticsBinding`은 unexported type의 alias여서 `go doc`에 구조가 드러나지 않는다(field 접근은 된다).

### 수정 사항 (v0.2.0의 결함을 후보가 고침)

- `oracle record`는 profile의 `queries`가 빈 배열이면 v0.2.0에서 manifest에 `"queries": null`을 써서 자신이 발행한 record set의 검증이 `JSON_NULL manifest#/queries`로 실패했다(`REFERENCE_SET_INVALID`, exit 4). 후보는 `[]`를 써서 같은 입력에서 PASS(exit 0)한다. 같은 입력의 v0.2.0 결과를 후보 `tsgk`가 거부하는 방향의 변화가 아니라 v0.2.0이 스스로 만든 set을 거부하던 동작이 사라진 것이다.

### 실측 결과: 후보 `aca5385`

experimental 표면의 실행 호환성을 후보 `aca5385`(`integ/362`)에서 v0.2.0 binary와 나란히 실행해 확인했다. v0.2.0 tree에는 저장된 evidence set이 없어 입력은 v0.2.0 binary가 직접 만든 산출물과 시험의 fixture를 같은 구성으로 재현한 것이다. 네이티브 컴파일·실행은 Windows Job Object 한도 안에서 했다. 최종 후보 commit에서 같은 확인을 다시 한다([발행 전 남은 검증](#발행-전-남은-검증) 14·15).

| 확인 | 결과 |
|---|---|
| v0.2.0이 만든 `incremental` result·`oracle record` set(manifest r1·record r1, result r1, protocol `tsgk-native/r2`)을 후보 `tsgk replay`로 재생(reducer r1·r2) | 재생된다. **판정(UNRESOLVED, `evidence_valid` true, exit 3)·`result_schema`·findings가 v0.2.0과 같다.** 거부된 입력은 없다. 단 counter·gate 목록은 위 7번처럼 다르다. UNRESOLVED는 두 버전 모두 `expectations`·`api` gate가 raw에서 재계산되지 않고 기록값으로 남기 때문이다 |
| `bs-gate-compare` set, `evidence verify`(통과·변조 그래프), 문서의 예시와 `invalid/` fixture를 두 버전으로 실행 | 판정·exit·code·출력 byte가 같다 |
| 후보가 만든 set을 v0.2.0 `replay`로 재생(역방향) | 재생된다(UNRESOLVED, valid). counter 차이는 정방향과 같다 |
| `incremental`(owned plain, scanner가 있는 stateful), `oracle record`(query 있는 profile)를 새 flag 없이 두 버전으로 실행 | 판정(PASS)·raw 응답 byte가 같다. 다른 것은 runtime pin에 따른 build identity·closure·executable·`runtime_commit`·producer identity와 driver compile 인자 한 개(`-DTSGK_RUNTIME_MANIFEST_SHA256=…`) 추가뿐이다 |
| 새 flag를 생략한 기본 경로 | 위 실행이 모두 새 flag 없이 정상 동작했다 |
| 두 버전의 runtime 상호 거부 | 확인했다(위 "사용자 안내") |

### 확인하지 않은 것

- 실제 과거 CI 산출물 등 git에 저장된 v0.2.0 evidence set의 재생(저장본이 없다). `private-corpus-r1`·`prepare-native-r1` reducer의 replay(비공개 local run 자료 필요).
- 손상·변조된 옛 evidence가 후보의 확장된 검사로 새로 거부되는지(위 7번).
- typed decode의 map 항목 계상 증가(`5e64ec5`, 한도 불변)가 v0.2.0에 있던 표면(`replay`·`evidence verify`·`qualify`)에 주는 영향. 계상이 늘기만 하므로 한도 근처 입력은 v0.2.0보다 먼저 `RESOURCE_LIMIT`가 될 수 있다. 측정하지 않았다.
- 후보의 새 flag 경로(`--pins`, `--runtime-pins`, `--build-cache`, `--reuse-*`, `--ordinary-*`, `--source-witness`, `evidence query-reference`)의 v0.2.0 대비 비교. 다른 route(C#·T-SQL·XML·SVC)와 큰 workload의 native 비교. 실제 tree-sitter generator를 쓴 `reproduce`.

위 실측은 후보 `aca5385` 기준이며 최종 후보에서 [발행 전 남은 검증](#발행-전-남은-검증) 14·15로 다시 확인하고, 결과에 따라 이 절의 호환성 목록을 고친다.

## 잔존 제약

- macOS는 #332 이후 CI 근거가 없다(NOT_RUN). 제외 전 근거는 역사로 보존하며 새 macOS 검증은 하지 않았다(#243 NOT_PLANNED, 신규 symbol 역할 DEFERRED). 제외를 PASS나 신규 SUPPORTED로 세지 않는다.
- experimental 기능과 API는 호환성을 약속하지 않으며 개발 main의 근거 추가로 stable이 되지 않는다.
- C/C++ macro 문맥은 heuristic이며 모호한 입력을 오류 없이 받을 수 있다. 기본 C# parser의 비리터럴 조건 분기 선택은 `NOT_CLAIMED`다.
- 관측 한계의 BLOCKED: macOS 강한 격리, 위 query 의미 미주장 범위, large 정상 실행의 Windows 한정.
- 실사용 소비자 성능과 26 언어 전체 정확도는 측정하지 않았다. 등록 사례의 판정이 임의 입력 지원을 인증하지 않는다.
- **미주장: C# 전체 선언 범위 대조.** C# 전체 선언 범위의 의무 → 독립 기대값 → 실행 근거 대조(G1)는 하지 않았고 주장하지 않는다. 주장하는 것은 위 "제품 범위"의 C1–C4 잔여 교정(#369)과 기존 qualification PASS뿐이다. [#233](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/233)은 출하 뒤에도 열어 두고 이어서 진행한다(2026-11-12 사용자 결정).
- CI 운영 여유 표본 미판정, 제품 동작 아님: [#280](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/280), [#303](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/303), [#352](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/352). 표본은 계속 자연스럽게 모아 판정한다.
- [#329](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/329): hosted Windows runner에서 Go runtime 치명 오류가 한 번 관측된 상류 의심 사건이며 재현 계측(E0)을 남겨 두었다. 제품 동작이 아니다.
- #280·#303·#352·#329는 출하 선행 조건이 아니다(2026-11-12 사용자 결정).
- 최근 nightly [43129863719](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/43129863719)(main `15dac94`)에서 비필수 `race diagnostic (ubuntu-24.04)` lane이 failure였다. data race 보고가 아니라 `src/kit` package의 10분 test timeout(`panic: test timed out after 10m0s`)이다. 같은 run의 필수 job은 모두 success다.

## 발행 전 남은 검증

발행 전에 최종 후보 commit(`$sha`)에서 아래를 순서대로 검증한다. 새 도구는 만들지 않고 저장소의 시험·도구와 GitHub 읽기 명령만 쓴다. 이전 후보의 소비 검증과 PR 성공은 승계하지 않으며, 후보가 바뀌면 B~F를 다시 한다. 이 목록은 실행 계획이며 최종 후보에서는 아직 실행하지 않았다(B~E의 명령은 후보 `5e64ec5`(`integ/362`)에서 한 번 예행 실행해 점검했다(예행 뒤 #416에 `c4527d0`까지 더해졌다). 그 결과는 이 문서의 근거가 아니다. 14·15는 이전 후보 `aca5385`에서 한 번 실측했으며 최종 후보에서 다시 한다). PowerShell은 `pwsh`(7)만 쓰고, 환경은 `CGO_ENABLED=0 GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly`, Go는 `go.mod`의 `go` 줄(1.27.4)과 같은 toolchain이다. `$work`는 저장소 밖의 새 빈 디렉터리다. `pwsh`에서 `-test.count=1`처럼 점이 든 인자를 native 실행 파일(예: `go test -c`로 만든 시험 binary)에 줄 때는 `'-test.count=1'`처럼 따옴표로 묶는다. 묶지 않으면 `-test`와 `.count=1`로 쪼개져 exit 2가 난다. 각 항목 끝의 시간은 예행 실행의 Windows 개발 host 실측이며 다른 시험이 겹치면 늘어난다.

**A. 전제와 후보 결정**

1. PR #416 상태와 필수 check(`foundation (windows-2025)`, `foundation (ubuntu-24.04)`, `integration gate`)를 읽고, 사용자 권한 아래 병합한다. 이 문서의 base가 main과 같은지 확인한다.
2. 이 release 문서 PR을 task branch → PR → 필수 CI → 병합으로 통합한다([검증 계약](../validation/validation.md)의 Integration 절).
3. `$sha`는 위 PR이 병합된 뒤의 main HEAD다. 이후 같은 SHA에서만 B~F를 수행한다.

**B. 후보 정체성(읽기)**

4. `git rev-parse HEAD` == `$sha`, `git status --porcelain`과 `git ls-files -u`가 비어 있음, `git diff --check`. (1초 미만)
5. `go version`과 `go.mod`의 `go` 줄 일치, 이 문서의 Go 기준과 대조. (1초 미만)

**C. 로컬 core 게이트**

6. `go mod download`(`GOPROXY=https://proxy.golang.org`, `GOSUMDB=sum.golang.org`로 외부 네트워크를 쓴다), `go mod verify` 뒤 `GOPROXY=off`와 `GOFLAGS=-mod=readonly`. (6·7 합쳐 약 12초)
7. `gofmt -l src`(출력 없음), `go vet ./src/...`, `go build ./src/...`.
8. foundation 단독: `go test ./src/internal/foundation -count=1`(부하에 민감한 시험이 있어 다른 시험과 겹치지 않게 실행). 다른 `go test`·gcc·native 프로세스가 없는 것을 확인한 뒤 시작한다. (약 83초)
9. foundation을 뺀 나머지 package와 native package: 검증 계약의 명령(`-count=1 -v`, `-timeout 300s`, native는 `-timeout 420s`)에서 `$otherPackages`를 `go list ./src/...`에서 native와 foundation을 제외한 목록으로 한다(foundation은 8번이 이미 단독으로 돌렸다). `TSGK_NATIVE_REQUIRED`·`TSGK_NATIVE_CC`·`TSGK_NATIVE_RUNTIME` 없이는 native 의존 시험이 SKIP이다. 시험 결과에서 SKIP 개수를 PASS와 따로 기록한다(예행 실행: native 제외 package에 `--- SKIP` 99줄, native package 708 PASS·129 SKIP). SKIP은 PASS가 아니며, native compile·실행 시험의 정본은 CI Windows runner 결과다. 로컬 환경 문제로 실패하는 시험은 원인과 함께 기록한다. (native 제외 약 142초, native package 약 3초)

**D. 공개 API·CLI·schema 소비와 offline 경계**

10. `go test ./src/cmd/tsgk -run '^(TestExternalConsumerAndCLI|TestExternalConsumerSchema|TestOfflineClosure)$' -count=1 -v` (약 4초)
11. `go test ./src/cmd/tsgk -run '^TestModuleProxyConsumer$' -count=1 -v`(git checkout에서 실행. 6번으로 module cache가 준비돼 있어야 한다). (약 4초)
12. `go test ./src/cmd/tsgk -count=1`. (약 10초)
13. v0.2.0 대비 변화 목록 재대조: `v0.2.0`과 `$sha`에서 각각 exported 선언(`go doc -all`), 각 하위 명령의 `-h`, `src/contracts/*.json`의 schema 문자열을 추출해 비교하고 위 "공개 API·CLI·schema 변화(v0.2.0 대비)" 절과 [공개 API 문서](../specs/public-go-api.md)를 갱신한다. `go doc -all`은 package별로 따로 저장해 package 단위로 비교한다. 합쳐서 비교하면 `kit.TreeNode`·`kit.Point`·`kit.EditPoints`가 `nativewire`의 같은 struct에 대한 alias가 된 변화가 `nativewire`의 같은 이름 선언에 가려진다. 재현 가능한 단일 명령이 없어 수동 대조다. (도구 실행 약 10초, 판독은 수동으로 10~30분 추정)

**14·15 입력 준비.** 입력은 저장소에 없다. 입력은 v0.2.0 tag의 binary로 생성한다.

- binary: `git archive --format=tar -o "$work/v020.tar" v0.2.0`를 `$work/v020`에 풀고, 거기서 위와 같은 환경으로 `go build -o "$work/tsgk-v020" ./src/cmd/tsgk`를 한다. 후보 binary는 `$sha`의 export(16번)에서 같은 방식으로 `$work/tsgk-cand`를 만든다.
- fixture: `v0.2.0` tag의 `src/testdata/native/plain/src/parser.c`(50,994 bytes)와 `src/testdata/native/plain/src/tree_sitter/{parser,alloc,array}.h`를 `$work/in/root/src/parser.c`와 `$work/in/root/src/tree_sitter/`로 복사한다. `$work/in/root/cases/one.txt`는 `a = f(1, [2, 3]);`와 `{ b = "s"; }` 두 줄(각 줄 끝 LF, 31 bytes)이다.
- profile 두 개는 `v0.2.0` tag의 `src/contracts/examples/incremental-r2.json` 형식을 따르며 아래를 공통으로 가진다: `id` `cli-plain`, `route` `owned-plain`, `symbol` `tree_sitter_tsgk_plain`, `encoding` `UTF-8`, `output` `tree`, `compiler`(`name` `cc`, 쓸 gcc의 `sha256`·`bytes`), `grammar`(위 파서와 헤더 세 파일의 `path`·`role`(`parser`·`header`)·`sha256`·`bytes`), case `one`(`input` `cases/one.txt` role `case`, edit 하나: `start_byte` 4, `old_end_byte` 5, `new_end_byte` 5, `old` `Zg==`, `new` `Zw==`, `points` `[]`).
  - `incremental`: `schema` `tsgk-incremental/r2`, `operation` `native-parse-edit`, `declarations` `{"mapping":"owned-r1","items":[{"fact":"type_declaration","node":"assignment","name":"field:name"}]}`, case `expect` `[{"step":1,"syntax":"NO_ERROR","contains":["call"],"declarations":"PASS"}]`.
  - `oracle record`: `schema` `tsgk-oracle/r2`, `operation` `native-query`, `declarations` `null`, `queries` `[{"id":"q1","source":"(assignment) @a"}]`, `fact_pack` `null`, `api` `true`, case의 `expect`·`query_expect` `[]`, `dynamic_sql_expect` `null`. `queries`만 `[]`로 바꾼 변형은 v0.2.0이 exit 4(`JSON_NULL manifest#/queries`), `$sha`가 exit 0이 기대 결과이며 위 "수정 사항"의 확인이다.
- 14번 replay 입력은 v0.2.0 binary가 15번 방식으로 낸 `incremental` 결과(`result.json`, `responses/`)와 `oracle record` set(`manifest.json`, `records/`, `raw/`)에 profile을 그대로 복사한 `workload.json`을 더한 디렉터리다. `--profile`에는 `tsgk-replay/r1` 등록 파일(`v0.2.0` tag의 `src/contracts/examples/replay-r1.json` 형식)을 준다. `members`는 각 파일의 `path`·`bytes`·`sha256`(role은 `workload-profile`과 `result`·`response`, 또는 `manifest`·`record`·`raw`)이고, `identities`는 result나 manifest가 이미 담은 workload·policy·producer·executable·compiler·runtime 값이다. reducer는 `native-result-r1`·`native-result-r2`와 `oracle-set-r1`·`oracle-set-r2`다. 등록 파일은 이 값들에서 일회용 script로 만들며 저장소에 두지 않는다.
- 같은 목록의 나머지 14번 입력은 작은 합성 JSON이다. `bs-gate-compare-r1`은 `recorded/`와 `recomputed/`의 gate 파일 한 쌍(같은 PASS, 그리고 recomputed만 FAIL), `evidence verify`는 `tsgk-evidence/r1` graph와 `tsgk-evidence-policy/r1` policy(정상, 그리고 raw 한 파일의 내용을 바꾼 변조본)다. `v0.2.0` tag의 `src/contracts/examples/`의 예시와 `invalid/` 파일도 빈 root에 대해 두 binary로 실행한다.

14. **v0.2.0 evidence set 재생.** 위 입력(v0.2.0 binary로 만든 기록)을 v0.2.0 binary와 `$sha`의 `tsgk replay`·`tsgk evidence verify`로 각각 재생해 결과와 오류 code, exit를 두 버전끼리 비교해 기록한다. 거부되거나 결과가 다르면 위 호환성 깨짐 목록에 항목을 더한다. **후보 `aca5385`에서 실측 완료(판정·exit·schema·findings 동일, counter·gate 목록 차이는 위 7번), 최종 후보에서 재확인.** (재생 약 1초, 입력 준비 시간은 포함하지 않는다)
15. **native 경로의 두 버전 실행 비교.** 위 입력으로 v0.2.0 binary와 `$sha` binary의 `incremental`·`oracle record`를 새 flag 없이 `--root "$work/in/root" --profile <profile> --work <새 디렉터리> --out <새 디렉터리> --runtime <그 binary가 pin한 runtime source> --tool cc=<gcc> --allow BUILD_NATIVE --allow EXEC_NATIVE --run-wall 30`으로 실행해 결과를 비교한다. 같지 않으면 원인과 영향을 기록하고 위 목록에 더한다. 승인된 native capability와 host compiler가 필요하고, 컴파일·native 실행은 승인된 자원 한도 안에서 한 번에 하나만 실행한다. 이 확인은 각 binary가 pin한 runtime source를 따로 준비해야 한다(위 "사용자 안내"; `$sha`의 pin은 `src/drivers/native-c/runtime-manifest.json`이다). **후보 `aca5385`에서 실측 완료(판정·raw 응답 동일, 차이는 runtime pin 관련 값), 최종 후보에서 재확인.** (실행 한 번에 약 7초, 네 번이면 약 30초. runtime 준비 시간은 포함하지 않는다)

**E. source export·설치·LICENSE/NOTICE**

16. 새 빈 디렉터리 `$work`에 `git archive --format=tar -o "$work/export.tar" $sha`로 export하고, 별도 디렉터리 `$work/src`에 풀어(부모에 `.git`이 없는 위치) 파일 수·bytes는 `$work/src`만 세어 기록한다(`export.tar`를 같은 디렉터리에 풀면 합계에 섞인다). `git ls-tree -r $sha`의 경로·blob SHA와 `$work/src`의 각 파일을 `git hash-object --no-filters --stdin-paths`(경로 목록은 `git ls-tree -r`에서 뽑아 `$work/src`에서 실행)로 비교한다. 저장소의 `.gitattributes`가 `* text=auto eol=lf`이고 `core.autocrlf`가 켜져 있을 수 있어 필터를 끈다. 금지 확장자(`.exe .dll .so .dylib .a .o .obj .lib .pdb .wasm .node`)가 없는지도 확인한다. (약 2초)
17. export(`$work/src`) 안에서 `GOPROXY=off`로 `go test ./src/internal/foundation -run '^(TestRepositoryFiles|TestSourceOnlyDoesNotDiscoverParentGit)$' -count=1`, `go vet ./src/...`, `go build ./src/...`. (약 12초)
18. 설치: export에서 `GOBIN=$work/bin`으로 `go install ./src/cmd/tsgk` 뒤 설치된 binary를 `PATH` 없이 실행한다. bash의 `env -i`는 `SystemRoot`가 없어 쓰지 않는다. `pwsh -NoProfile -File <script>`에서 `$env:PATH = ''`로 비운 뒤 절대 경로로 실행하는 script를 쓴다(전역 환경은 바꾸지 않는다). 기대 결과는 다음과 같다. 루트에는 `--help`·`-h`·`help`가 없다(v0.2.0 binary도 `--help`에 `UNKNOWN_COMMAND`·exit 2다). 인자 없는 실행은 `USAGE`와 exit 2이고, `inspect --root src/testdata/native/plain`(export 안의 경로)은 exit 0, `execution_status` `COMPLETED`, `assessment` `NOT_ASSESSED`, finding `METADATA_ABSENT`다. `src/testdata/ordinary-json`은 `UNKNOWN_LAYOUT` warning만 내므로 이 확인에 쓰지 않는다. (약 3초)
19. LICENSE/NOTICE: `LICENSE`, `src/dev/prepare-p05/fixtures/LICENSE.mssql`, `src/kit/data/NOTICE-index-euc-kr.md`, `src/testdata/native/samples/NOTICE.md`의 SHA-256이 `$sha`의 blob 및 직전 release의 고정 bytes와 같은지 대조하고, 새 dependency·upstream license 변화가 없는지 `go.mod`·`go.sum` diff와 [provenance](../provenance/upstream-sources.md)를 본다. 자동 시험: `go test ./src/internal/foundation -run '^TestSampleNotices$' -count=1`. (약 2초)

**F. 필수 CI·fan-in·actual main**

20. release 문서 PR의 최종 head에서 `gh pr checks`로 `foundation (windows-2025)`, `foundation (ubuntu-24.04)`, `integration gate`, `CI mode` 모두 성공을 확인한다. skipped·cancelled·missing은 PASS가 아니다. 병합 직전에 ruleset·checks·conversation·review를 새로 읽는다.
21. 병합 뒤 actual main: 병합 SHA의 `foundation.yml` run과 전체 job 성공을 `gh run list`·`gh run view`로 확인한다. 재사용된 run이면 `CI mode` 로그의 PR run·attempt·synthetic SHA·tree를 확인한다.
22. fan-in·qualification: `integration gate` 성공, 공식 artifact의 digest·크기·member·cohort·병합 parent/tree를 확인하고, Windows·Linux 파생 52칸 qualification PASS와 C# symbol 26 PASS·0 MISMATCH·4 NOT_APPLICABLE를 대조한다.
23. **query 의미 fan-in 재계산.** `go test ./src/kit -run '^TestQueryCompletion' -count=1 -v`와, native·runtime이 준비된 환경에서 `TSGK_NATIVE_REQUIRED=1 go test ./src/internal/native -run '^TestNativeQueryCompletionCurrentState$' -count=1 -v`로 현재 VERIFIED·BLOCKED 합계, gate `BLOCKED`·`complete=false`, FAILED 0을 확인한다. 위 "scope별 fan-in" 행의 수치(VERIFIED 360)가 시험이 고정한 값(predicate 67 + SVC 263 + large 30)과 같은지 확인한다. 환경은 검증 계약의 "#362 scope별 fan-in" 절을 따른다.
24. (선택) 같은 후보에서 predicate·SVC·large·SQL 공개 pass의 opt-in 시험을 한 번 재현한다. 한도 불변·HARD Job 조건을 지킨다(검증 계약의 #362 절).
25. CI 운영 표본(#280·#303·#352·#329)의 현재 상태를 읽고 "잔존 제약"의 문구와 같은지 확인한다. 선행 조건이 아니다.

**G. 발행안**

26. 정확한 version `v0.3.0`, full commit `$sha`, 주석 tag 제안, 한·영 본문(이 문서와 CHANGELOG v0.3.0 절의 날짜·commit 채움), source-only(binary·package 없음), 검증·미검증·rollback 경계를 한 문서로 제시하고 사용자 발행 승인을 요청한다. 승인 전에는 tag 생성·push·Release 생성을 하지 않는다.

발행 범위는 source tag `v0.3.0`(주석 tag)과 source-only GitHub Release다. uploaded binary, package registry 발행, 소비자 저장소 변경은 포함하지 않는다.
