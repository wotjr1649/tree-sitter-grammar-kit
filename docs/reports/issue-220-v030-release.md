# v0.3.0 release 문서 (초안, #220)

이 문서는 source-only release `v0.3.0`의 한국어 본문 초안이다. 영어판은 [English](issue-220-v030-release.en.md)다. 최종 tag 대상 commit과 발행일은 미정이며, 발행은 정확한 version·commit·한영 본문·source-only 범위를 검토한 뒤 사용자가 별도로 승인한다. 이 초안은 발행 승인 요청이나 발행 완료 증거가 아니다. 추적 Issue는 [#220](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/220)이다. 이전 준비 기록은 [#199 문서](issue-199-source-release.md)이며, 그 안의 후보(`33f84fc`, `52c1e2b`)와 수치는 역사 기록으로만 둔다.

## 조건

- 아래 "query 의미 claim"의 모든 항목은 통합 PR [#416](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/416)(`integ/362`, `Refs #362`)이 main에 병합된 뒤에만 유효하다. 병합 전에는 어떤 항목도 주장하지 않는다.
- 이미 주장하던 범위(아래 "제품 범위")는 그대로이고 줄이지 않는다. 새로 주장하는 것은 query 의미의 검증된 하위 범위뿐이다.
- 최종 후보 commit에서 [발행 전 검증](#발행-전-미완료-검증)을 다시 실행하기 전에는 이 문서의 수치를 최종 release 근거로 쓰지 않는다.

## 제품 범위

[제품 범위](../specs/scope.md)와 [플랫폼 지원 계약](../specs/platform-support.md#release-v030의-claim-초안)이 정한다.

- **VERIFIED:** offline `inspect`, `identity`, `verify`, `schema check`/`schema diff`와 해당 공개 Go API(`src/kit`). 등록 26 route qualification 판정은 고정 입력·의무에 대한 것이며 임의 입력의 문법 적합성 인증이 아니다. 현재 CI의 검증 host는 Windows·Linux다(derived 52칸, #332).
- **experimental(호환성 약속 없음):** `corpus`, `reproduce`, `incremental`, `oracle record`, `replay`, `evidence verify`, `qualify`와 그 API, 그리고 아래 query 의미 검증 전부.
- 발행물은 source tag와 source-only GitHub Release다. binary와 package는 없다.

## query 의미 claim (experimental)

v0.3.0은 query 의미를 experimental로 유지하고, 실제로 검증한 하위 범위만 주장한다. 나머지는 BLOCKED 사유와 함께 "미주장"으로 적는다. 2026-11-12 사용자 결정으로 [#362](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/362)의 completion gate는 #220 출하 조건이 아니다. 이는 이미 experimental인 기능의 주장 문구를 검증된 사실과 맞추는 정합화이며 지원 범위를 줄이는 것이 아니다.

### 주장하는 범위

모두 Windows에서, 실제 owned native producer와 등록 한도(logical 1 GiB, work 32,000,000, read 256 MiB; large는 등록된 large 예산)를 바꾸지 않고 얻은 결과다. 독립 expected는 grammar·query 원문에서 도출해 native 실행 전에 봉인했다.

| 범위 | 검증된 사실 | 근거 |
|---|---|---|
| predicate(`eq?`, `not-eq?`, `any-of?`, `not-any-of?`) | 독립 평가 tuple 20/20 VERIFIED, 거부·미완료 정책 47/47 정확한 거부. 새 predicate 지원은 없다 | [#408](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/408), [expected](../../src/contracts/query-predicate-expected-r1.json), [계약](../specs/instrumented-query-reference.md#362-predicate-의미-의무와-거부-정책-범위-3) |
| SVC 오프라인 의무 | 263개 VERIFIED(history 110, edit point 41, segment 109, CodeBehind 2, negative 1) | [#410](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/410), [expected](../../src/contracts/query-svc-expected-r1.json) |
| SVC inline C# | tree 82개 중 30, 두 query 164개 중 60이 실제 C# grammar native와 일치(`facts.dynamic_sql` fact 0개 포함) | [#415](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/415), [expected](../../src/contracts/query-svc-inline-expected-r1.json) |
| Windows large | 39개 의무 중 31 VERIFIED. `cs-large-22m`은 source·status·전체 CST(node 11,463,151, page 12)·occurrence가 native와 일치하고, `cs-large-32mib-errors` status는 등록 8 GiB HARD 한도에서 `COMPLETED`다. over-limit control은 `RESOURCE_LIMIT`/`OUTPUT_LIMIT`로 정확히 거부된다 | [#412](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/412), [#415](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/415), [expected](../../src/contracts/query-large-expected-r1.json) |
| ordinary 선택 SQL shard 공개 pass | TSQL `alt-tsql-B01-r001` 72/72, PostgreSQL `alt-postgresql-sql-B04-22` 9/9 VERIFIED. 한도 불변, TSQL 남은 여유는 work 7.3%·logical 3.47%로 얇다 | [#416](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/416), [#403](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/403) |
| ordinary JSON 양성 | 여러 case(2/2)와 편집 이력(3 edit·4 version, 14/14)의 공개 API·r9 replay·`tsgk replay` 결과 일치(저장소에 추적되는 JSON grammar fixture) | [#404](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/404), [#402](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/402) |
| scope별 fan-in | 현재 상태를 기계 검증으로 모으며 FAILED 0. 집계는 전체 107,399 의무 중 VERIFIED 360(large 32 MiB 재실행 반영 전), 나머지 BLOCKED | [#413](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/413), [#414](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/414) |

### 주장하지 않는 범위(BLOCKED 사유)

| 미주장 | 사유 |
|---|---|
| 오류 복구(ERROR·MISSING)가 생기는 입력의 CST·capture | grammar 규칙만으로 결과가 정해지지 않아, 결과 복사 golden 없이는 독립 expected를 만들 수 없다. SVC inline 8 step, large `cs-large-8m-errors`·`cs-large-32mib-errors`의 CST·occurrence 8개가 해당한다 |
| 전처리 scanner 상태가 필요한 SVC inline | 44 step(`#if`·`#else`·`#define`·`#endif`). external scanner의 상태 기계를 규칙에서 독립 도출하지 못했다. 이 step의 query 104개도 미주장이다 |
| Linux 몫 | predicate 20+47, SVC, large의 Linux 몫은 등록만 되어 있고 CI 수집 경로가 없다 |
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

공개 API·CLI·schema 변화의 최종 후보 대조(v0.2.0 대비)는 발행 전 검증의 일부이며 아직 수행하지 않았다. 과거 후보의 대조(추가 21·변경 9, flag 하나)는 [#199 문서](issue-199-source-release.md)의 당시 후보에만 귀속한다. experimental struct 필드 추가와 SVC r1→r2는 소비자의 명시적 검토가 필요하다.

## 잔존 제약

- macOS는 #332 이후 CI 근거가 없다(NOT_RUN). 제외 전 근거는 역사로 보존하며 새 macOS 검증은 하지 않았다(#243 NOT_PLANNED, 신규 symbol 역할 DEFERRED). 제외를 PASS나 신규 SUPPORTED로 세지 않는다.
- experimental 기능과 API는 호환성을 약속하지 않으며 개발 main의 근거 추가로 stable이 되지 않는다.
- C/C++ macro 문맥은 heuristic이며 모호한 입력을 오류 없이 받을 수 있다. 기본 C# parser의 비리터럴 조건 분기 선택은 `NOT_CLAIMED`다.
- 관측 한계의 BLOCKED: macOS 강한 격리, 위 query 의미 미주장 범위, large 정상 실행의 Windows 한정.
- 실사용 소비자 성능과 26 언어 전체 정확도는 측정하지 않았다. 등록 사례의 판정이 임의 입력 지원을 인증하지 않는다.
- [#280](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/280)·[#303](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/303)·[#352](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/352)의 CI 시간 여유 표본은 판정이 끝나지 않았다. 제품 동작이 아니라 CI 운영 여유의 문제다.

## 발행 전 미완료 검증

발행 전에 최종 후보 commit에서 source export·설치·공개 API/CLI/schema 소비·LICENSE/NOTICE, 필수 CI·fan-in·actual main을 검증한다. 실행 목록은 #220 작업 보고서가 소유하며 새 도구는 만들지 않는다. 이전 후보의 소비 검증과 PR 성공은 승계하지 않는다.

발행 범위는 source tag `v0.3.0`(주석 tag)과 source-only GitHub Release다. uploaded binary, package registry 발행, 소비자 저장소 변경은 포함하지 않는다.
