# Session 06 — Native Oracle & Tree Protocol 결과

Campaign `TSGK-C1-20260929-R1`, 추적 `TSGK-C1-S06`, Issue #8, Milestone 7, branch `session/06-native-oracle`(base main `ac70005`). 이 보고서는 아래에 적은 revision과 로컬 관측만 설명한다. PR·세 OS CI·merge·post-merge 결과는 이 문서 작성 시점에 없으며 그 단계의 receipt가 소유한다.

## 구현 범위

* **`tsgk-native/r2`**(`src/drivers/native-c/driver.c`): 계약은 [tree/protocol](../specs/tree-and-adapter-protocol.md)의 `S06 구현`이다. S05 driver를 같은 파일·같은 parse/직렬화 engine으로 확장했다. r1 요청은 S05 응답을 그대로 받고, r2는 query 결과, `tsgk-api/r1` 관측, producer capability 선언, query 한도를 더한다. query는 `ts_query_new`로 compile하고 `ts_query_cursor_next_capture`의 stream을 정렬·중복 제거 없이 기록한다. capture node는 `TSNode.id`로 preorder index에 연결한다. predicate·directive는 driver가 평가하지 않고 단계를 그대로 보고한다.
* **Go 검증과 판정**(`src/internal/native`): revision 일치, r1 엄격성, query·API 모양, capture-node 연결(`CAPTURE_LINK_MISMATCH`), match 번호 순서를 검사한다. host predicate subset `tsgk-predicates/r1`(`eq?`·`not-eq?`·`any-of?`·`not-any-of?`, 단일 capture, UTF-8)만 평가하고 나머지는 `UNSUPPORTED`다. API 관측은 cursor 직렬화와 대조하고, edit step은 incremental/fresh query stream을 하나뿐인 `kit.CompareCaptures`로 비교한다. S06 claim 다섯 개(`query_equality`, `query_expectations`, `api`, `fact_reproduction`, `dynamic_sql`)는 S05 claim과 따로 둔다.
* **CLI `tsgk oracle record`**와 profile `tsgk-oracle/r1`: 계약은 [CLI/profile](../specs/cli-and-profile.md)의 `S06 구현`이다. `incremental`과 같은 build·runner·인자 검사를 쓰며, 배타적 출력 디렉터리에 사례 record·원 응답·마지막 manifest를 쓰고 `kit.VerifyOracleSet`으로 다시 검증한다. member 쓰기에 실패하면 manifest를 쓰지 않는다.
* **공개 offline API**: `CompareCaptures`, `ParseOracleProfile`, `VerifyOracleSet`, `ParseFactPack`, `DeclarationQuery`, `DeriveDeclarations`, `DeriveDynamicSQL`, `CaptureText`([공개 API](../specs/public-go-api.md) `S06 함수`). driver 실행은 공개 API가 아니다.
* **사실 query 세트** [`fact-query-pack.json`](../../src/contracts/fact-query-pack.json)(`fact-queries-r1`): C#·T-SQL·PostgreSQL 선언 query(mapping에서 `DeclarationQuery`로 생성, `TestFactQueryPack`이 재생성 대조), C#·T-SQL 동적 SQL query, XML 구조 query. 선언 사실은 같은 tree의 S05 순회 항목과 모든 필드가 같아야 PASS다.
* **연산**: `native-query`(세 OS), `native-query-large`·`real-world-source-r3`(windows/amd64, 8 GiB). driver `memory_bytes` 상한을 8 GiB로 올렸다.
* **사례**: route마다 query 사례 1개(`src/testdata/native/queries`, 26개). feature 사례 source·edit를 재사용하고 기대 capture stream을 등록했다. owned 시험(`oracle_test.go`, `record_test.go`, `closure_test.go`, `src/kit/oracle_test.go`, `facts_test.go`)도 더했다.
* **CI**: `native routes (<os>)`가 `run-routes.ps1 -Large -Oracle`로 S05 실행 뒤 같은 사례와 query 사례를 r2로 기록한다. pack·동적 SQL 대조와 windows 전용 대용량 query도 같이 실행한다([validation](../validation/validation.md)).

제외: Go consumer adapter, 언어 의미 분석, `match?` 등 subset 밖 predicate 평가, runtime 수정, CGO/FFI, 새 dependency.

## 사용자 결정 `C1-REAL-WORLD-SOURCE-WINDOWS-R3` 적용

결정 receipt(3016 bytes, sha256 `a7c8ec22…`)를 크기·hash로 확인한 뒤 적용했다.

* `cs-large-32mib-errors`를 로컬 Windows(RAM 33820106752 bytes)에서 12 GiB 측정 상한(Job Object)으로 실행했다. 결과는 `COMPLETED`(summary, node 17478169, errors 9463, parse 17.4초, process wall 37.6초)이고 Job peak commit은 5157146624 bytes(4.80 GiB)였다. 같은 실행에서 22m은 3325067264, 8m-errors는 1227366400 bytes였다. 근거는 `artifacts/.../session-06/r3-memory-measurement-r1.json`에 있다.
* peak에 20%를 더하면 6188575948 bytes(5.76 GiB)다. 8·10·12 GiB 중 이 값 이상인 가장 작은 값이 **8 GiB**(8589934592)라 r3 Windows 값으로 정했다. 12 GiB를 넘을 필요는 없었다.
* `real-world-source-r3`는 r2와 같은 값에 memory만 8 GiB이고 windows/amd64 전용이다. 다른 host에서는 CLI가 `OPERATION_PLATFORM_SCOPE`로 거부하고 helper는 `NOT_APPLICABLE`(이유 "NET461 workload is Windows-hosted (WinForms/.NET Framework 4.6.1)")로 기록한다. `native-query-large`도 같은 memory와 범위다. S05의 r2 세 host 결과는 S05-A18의 역사로 보존했다.
* 계약: [NET461 등록부](../validation/net461-workload.md)(r3 줄과 담당 행), [workload matrix](../validation/workload-matrix.md)의 S05-A18·S06-A17·S08-A17, [CLI/profile](../specs/cli-and-profile.md)·[platform](../specs/platform-support.md)의 값 서술.
* hosted windows-2025(RAM 약 16 GB)는 로컬 peak 5.16 GB가 들어가는 8 GiB 상한을 수용할 크기다. hosted 실측은 PR CI의 windows route job이 할 일이며 이 세션에서는 실행하지 않았다.

## 계약 결정과 runtime 관측

* **query 시간 상한.** pinned runtime의 `ts_query_cursor_next_capture`는 앞선 match가 끝나지 않으면 progress callback이 취소한 뒤에도 cursor를 계속 전진시킨다(취소가 유지되지 않음). owned 시험에서 1 ms 예산 query가 60초 넘게 돌았다. driver는 첫 만료에서 취소하고 loop를 멈추며, 그 뒤에도 callback이 불리면 step 없는 typed `RESOURCE_LIMIT`/`QUERY_TIME_LIMIT` frame으로 끝난다(수정 뒤 171 ms). 이 경로에서는 부분 capture가 남지 않는다. capture·match 상한은 부분 capture를 `partial`로 남긴다.
* **node API와 cursor의 차이**(API claim). 첫 owned 실행에서 `ts_node_next_sibling`이 zero-width MISSING `)`를 건너뛰었다. runtime 원본(`lib/src/node.c` 190–301행)은 형제를 byte 위치로 찾는다. 그래서 zero-width 경계의 형제 차이만 `position_navigation`으로 개수·첫 사례를 기록하고 실패로 세지 않는다. 다른 차이는 모두 FAIL이다. route 실행에서는 두 종류의 runtime 차이가 FAIL로 기록됐다. T-SQL aliased `field` node에서는 `ts_node_child_by_field_id`가 그 안의 손자를 `name`으로 돌려준다. C# ERROR node에서는 cursor가 보고한 child field를 `child_by_field_id`가 찾지 못한다. kit 결함이 아니라 runtime의 실제 API 동작이며, 아래 route 결과에 처분 대상으로 남겼다.
* **동적 SQL known miss.** 등록 fixture는 known miss 세 곳을 사실에서 빼고 범위로 기록했다. grammar는 그중 `AT DATA_SOURCE`와 `WITH RESULT SETS`에서 부분 `execute_statement`를 만든다. 추출은 mapping `dynamic-sql-r1`의 known_misses 목록을 구현해 세 곳을 known miss 범위로 보고하며(`AT DATA_SOURCE`: 따옴표 없는 `DATA_SOURCE` linked server, `WITH RESULT SETS`: EXEC 바로 뒤 `WITH`로 시작하는 ERROR, EXEC 없는 첫 호출: 첫 child가 `sp_executesql`인 ERROR), 등록 기대값은 바꾸지 않았다.
* **pack query 일부 선택.** 대용량 합성 fixture에 C# 동적 SQL query를 실행하면, 그 구조 후보 pattern이 모든 `binary_expression`을 잡아 출력 상한(`OUTPUT_LIMIT`)에 닿았다. driver는 predicate를 평가하지 않기 때문이다. profile이 pack query 일부만 고를 수 있게 했다. 대용량은 선언 query만 쓰며, 고른 query는 pack과 정확히 같아야 한다.

<!-- RESULTS -->
