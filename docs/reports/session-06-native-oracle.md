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

* **query 시간 상한.** pinned runtime의 `ts_query_cursor_next_capture`는 앞선 match가 끝나지 않으면 progress callback이 취소한 뒤에도 cursor를 계속 전진시킨다(취소가 유지되지 않음). 개발 중 로컬 진단에서 1 ms 예산 query가 60초 넘게 돌았고, 수정 뒤에는 171 ms에 끝났다. 이 두 수치의 실행 기록은 보존하지 않았다. 지금은 driver가 첫 만료에서 취소하고 loop를 멈춘다. 그 뒤에도 callback이 불리면 step 없는 typed `RESOURCE_LIMIT`/`QUERY_TIME_LIMIT` frame으로 끝난다(`TestQueryLimits/time`과 mutant `query-time-not-enforced`가 지킨다). 이 경로에서는 부분 capture가 남지 않는다. capture·match 상한은 부분 capture를 `partial`로 남긴다.
* **node API와 cursor의 차이**(API claim). 첫 owned 실행에서 `ts_node_next_sibling`이 zero-width MISSING `)`를 건너뛰었다. runtime 원본(`lib/src/node.c` 190–301행)은 형제를 byte 위치로 찾는다. 그래서 zero-width 경계의 형제 차이만 `position_navigation`으로 개수·첫 사례를 기록하고 실패로 세지 않는다. 다른 차이는 모두 FAIL이다. route 실행에서는 두 종류의 runtime 차이가 FAIL로 기록됐다. T-SQL aliased `field` node에서는 `ts_node_child_by_field_id`가 그 안의 손자를 `name`으로 돌려준다. C# ERROR node에서는 cursor가 보고한 child field를 `child_by_field_id`가 찾지 못한다. kit 결함이 아니라 runtime의 실제 API 동작이며, 아래 route 결과에 처분 대상으로 남겼다.
* **동적 SQL known miss.** 등록 fixture는 known miss 세 곳을 사실에서 빼고 범위로 기록했다. grammar는 그중 `AT DATA_SOURCE`와 `WITH RESULT SETS`에서 부분 `execute_statement`를 만든다. 추출은 mapping `dynamic-sql-r1`의 known_misses 목록을 구현해 세 곳을 known miss 범위로 보고한다. 등록 기대값은 바꾸지 않았다. 판정 규칙은 [tree/protocol](../specs/tree-and-adapter-protocol.md) `S06 구현`에 있다. `AT DATA_SOURCE`는 따옴표 없는 `DATA_SOURCE` linked server로 알아본다. `WITH RESULT SETS`는 EXEC 바로 뒤(공백만 사이)에 text가 `WITH RESULT SETS`로 시작하는 ERROR로 알아본다. EXEC 없는 첫 호출은 text가 `sp_executesql`(`sys.` 한정, `[]`·`""` 허용)로 시작하는 ERROR로 알아본다. comment는 제외한다.
* **선언 query와 출력 상한.** 선언 사실은 S05 `locate`처럼 첫 후보에서 멈춰야 하므로 첫 후보 단계마다 pattern이 필요하다. 하지만 모든 깊이를 경로 전체로 잡으면 32 MiB summary가 16 MiB 출력 상한을 넘는다. 그래서 선언 node 한 번과 부모→후보 edge만 잡는다. node의 부모는 하나뿐이라 edge가 경로를 정한다.
* **pack query 일부 선택.** 대용량 합성 fixture에 C# 동적 SQL query를 실행하면, 그 구조 후보 pattern이 모든 `binary_expression`을 잡아 출력 상한(`OUTPUT_LIMIT`)에 닿았다. driver는 predicate를 평가하지 않기 때문이다. profile이 pack query 일부만 고를 수 있게 했다. 대용량은 선언 query만 쓰며, 고른 query는 pack과 정확히 같아야 한다.

## 도구 identity와 로컬 검사

Windows 로컬 실행에는 다음을 썼다.

* Go 1.27.1, PowerShell 7
* 기존 MSYS2 UCRT64 GCC 16.2.0(`gcc.exe` sha256 `75e87953…`, 3299024 bytes, 설치·변경 없음)
* runtime `tree-sitter/tree-sitter@659cda7c`(내장 manifest 83개 파일)
* S05가 준비한 26 route 입력(`.work/session-05/prepared`, 등록부 hash 대조)

새 다운로드는 없다.

최종 후보 `b76c466`에서 다음 검사가 모두 통과했다.

* `gofmt`, `go vet`(windows·`GOOS=linux`·`GOOS=darwin`), `git diff --check`.
* workflow의 `형식과 foundation 검사` step: step 본문을 그대로 꺼내 새 `pwsh -NoProfile`에서 실행했다. job env(`CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOFLAGS=-mod=readonly`), runner 변수, RUNNER_TEMP 형태의 디렉터리, workflow가 `GITHUB_ENV`로 넘기는 native 도구 변수를 줬다. 결과는 step exit 0이다. `go test ./src/...`의 native package는 136초다. 빌드가 무거운 두 시험은 순차 시험이 끝난 뒤 병렬로 돈다.
* 같은 방식의 `26 route native build와 등록 사례·query 기록 실행` step: 이번에는 native 도구 변수 없이 실행했고 step exit 0, failures 0이다. 결과는 아래 절이다.

| 검사 | 결과 |
|---|---|
| r1 호환(A01·A12) | 확장 driver에서 S05 native·CLI 시험이 모두 통과했다. 바뀐 기대는 `TestFrames/protocol` 하나다. 알 수 없는 revision(`r3`)은 `PROTOCOL_MISMATCH`, r2 머리에 r1 본문을 붙이면 `REQUEST_MALFORMED`다. `TestRevisions`: r1 요청은 r1 응답을 받고, r2 응답을 r1 요청 기준으로 검사하면 `RESPONSE_PROTOCOL_MISMATCH`다 |
| capture stream(A03·A06) | `TestQueryStream`: 두 pattern이 같은 node를 잡은 중복과 같은 byte의 동률이 runtime 순서대로 남는다(시작 byte, pattern 번호, match 안 capture 순서). 기대 stream은 이 순서 규칙에서 손으로 도출했다. `TestCaptureIdentity`: 같은 범위의 root·statement와 zero-width MISSING이 서로 다른 index를 갖는다. `TestCaptureLink`: 다른 node로 연결한 capture 행은 `CAPTURE_LINK_MISMATCH`다 |
| 오류·predicate(A04) | `TestQueryErrorsAndPredicates`: `NODE_TYPE`(offset 19, point 0:19)·`SYNTAX`·`FIELD` 오류를 구분한다. `match?`와 `#set!`은 `UNSUPPORTED`이고 capture가 없다. `eq?`·`any-of?`는 평가된 stream이다. 일치 없는 query는 `captures: []`, 요청하지 않은 query는 없음이다. `TestQueryMembersRequired`: `predicates`·`partial`·중첩 `pattern`·단계 `kind`가 빠지거나 `matches`가 틀린 응답은 거부한다 |
| 한도·취소(A05) | `TestQueryLimits`: `CAPTURE_LIMIT`·`MATCH_LIMIT`는 부분 capture를 `partial`로 남기고 사례는 `RESOURCE_LIMIT`/`BLOCKED`이며 cleanup verified다. 1 ms 시간 예산은 typed `QUERY_TIME_LIMIT`이고 완료로 보고하지 않는다 |
| API 관측(A07) | `TestAPIObservations`: ERROR·MISSING·extra(comment)가 있는 tree에서 API claim이 PASS다. zero-width MISSING 경계의 형제 탐색 차이는 `position_navigation`으로 기록한다. 위조한 parent·형제·child 수·depth, 자기 자신을 형제로 돌려주는 값, 빠진 field lookup은 모두 차이로 검출한다 |
| closure 거부(A08) | `TestClosureRejections`: scanner가 빠졌거나 다른 grammar의 scanner면 link 단계 `BUILD_FAILED`다. 바뀐 header는 `SOURCE_MISMATCH`, closure에 없는 symbol은 `BUILD_FAILED`다. ABI 99·12는 build는 되지만 driver가 `FAILED`/`LANGUAGE_INCOMPATIBLE`(step 없음)를 낸다. 바뀐 scanner는 새 identity로만 build된다. S05 `TestBuildIdentity`도 그대로 통과한다 |
| capability(A08) | `TestCapabilityMissing`: query capability를 선언하지 않는 fault build(`TSGK_FAULT_NO_QUERY`)는 관측을 해석하기 전에 `NOT_RUN`/`BLOCKED`(`QUERY_CAPABILITY_MISSING`)다 |
| platform 범위(r3 결정) | `TestPlatformScope`: linux/amd64를 가정하면 `incremental`(r3)과 `oracle record`(`native-query-large`) 모두 build·출력 전에 `OPERATION_PLATFORM_SCOPE`로 거부한다 |
| 기록 set(A02·A09·A10) | `TestOracleRecordSet`: 발행한 set이 검증을 통과하고, record는 build identity와 입력 identity에 결속된다. 기존 출력은 `OUTPUT_EXISTS`로 거부되며 바뀌지 않는다. 같은 출력으로 동시에 두 번 실행하면 하나만 성공한다. member 쓰기 실패를 주입하면 manifest가 없고 set 검증은 `MANIFEST_MISSING`이다. `TestVerifyOracleSet`(도구 없이 실행)은 잘린 member, footer 없는 manifest, 잘린 manifest, 없는 manifest·member, 목록 밖 파일, record 수 불일치, record 없는 사례, manifest 값 형식 오류, 완결되지 않았거나 입력·step·사례가 다른 record를 거부한다 |
| edit와 query(A02·A12) | `TestQueryAcrossEdits`: edit step마다 incremental·fresh query stream이 같고 S05 claim도 PASS다. `TestQueryEquality`: 바뀐 capture·status를 검출한다 |
| 사실 query 세트(A15) | `TestFactQueryPack`: pack의 선언 항목이 mapping과 같고, 선언 query가 `DeclarationQuery` 결과와 같으며, pack identity가 bytes를 결속한다. `TestDeriveDeclarations`: 첫 후보 선택, 첫 후보 아래 이름이 없으면 `NAME_MISSING`, `children:` 전부, 문서 순서를 확인한다 |

## targeted mutant (A11)

`b76c466`에서 22/22를 검출했다(`artifacts/.../session-06/mutants-b76c466.json`). mutant는 다음과 같다.

* capture: 정렬, 중복 제거, 범위로 만든 node identity, capture-node 연결 검사 제거
* tree·build: 없는 field의 빈 이름 기본값, build identity에서 scanner 제외, record identity에서 producer 제외
* query 판정: predicate 무시, 잘못된 query를 빈 성공으로 보고, query 한도를 완료로 취급, incremental/fresh query 비교 무력화, query 시간 상한을 한 번만 취소, r2 member 선택적 처리
* API: 비교 생략
* 기록 set: footer 검사 제거, 목록 밖 member 허용, 기존 출력 재사용
* 선언 사실: `child:` locator가 모든 node를 취함, 이름이 있는 후보만 첫 후보로 취함
* 진입 검사: platform 범위 무시, revision 검사 제거, capability 검사 제거

과정에서 생긴 일은 다음과 같다.

* `90071ce`의 첫 실행에서 기존 출력 재사용 mutant가 살아남았다. 기존 출력 검사(`Lstat`)가 배타 생성보다 먼저 걸러, 배타 생성 자체가 시험되지 않았기 때문이다. 배타 `Mkdir`을 유일한 guard로 남겼다.
* 리뷰 수정 뒤에는 r2 member 선택 mutant가 살아남았다. 다른 모양 검사가 겹쳐 잡았기 때문이다. 다른 검사가 없는 member를 지우는 시험을 더했다.
* 자기 자신을 형제로 받는 mutant는 동등 mutant였다. 탐색 loop가 자기 자신을 지나지 않아 그 조건이 필요 없었다. 조건과 mutant를 모두 뺐다.
* 첫 실행의 harness는 한글 출력 decode 오류로 중간에 멈췄다. 도구 결함이며 고친 뒤 전부 다시 실행했다.

## 26 route 기록 (A13·A15·A16·A17, Windows 로컬, `b76c466`)

CI 방식 route step(12분 45초)의 결과는 failures 0이다(`artifacts/.../session-06/routes-windows-b76c466.json`). S05 incremental(r1) 27개 실행의 판정은 S05와 같다(csharp·typescript·tsx·swift의 grammar gap FAIL, SVC BLOCKED). oracle(r2) 27개 실행(26 route + SVC)의 결과는 다음과 같다.

* 사례 165개 모두 `COMPLETED`이고, 기록 set 27개 모두 `VerifyOracleSet`을 통과했다.
* route query 사례 26개의 기대 capture stream이 26/26 PASS다. 기대값은 별도 작성자가 source와 query 의미에서 도출했고 실행 결과에서 가져오지 않았다.
* edit step의 incremental/fresh query 비교는 140 사례 PASS, 선언 사실 재현은 50 사례 PASS다(C#·T-SQL·PostgreSQL의 모든 등록 사례와 동적 SQL fixture).
* 동적 SQL(A16): T-SQL fixture 사실 13개와 known miss 3개, C# fixture 사실 13개가 등록값과 순서까지 같다. C# 사실은 모두 `heuristic: true`이고 `CommandType.StoredProcedure` 오탐을 포함한다. `AS USER/LOGIN`·pass-through·`EXEC @module_var`는 사실에 없다.
* API claim은 129 사례 PASS, 6개 route의 23 사례 FAIL이다. 모두 node API의 field lookup이 cursor와 다르다. 사례별 node type은 `artifacts/.../session-06/api-findings-b76c466.json`에 있다.
  * cursor 직렬화에 그 field가 없는데 `child_by_field_id`가 node를 돌려준 경우: T-SQL 5(`assignment`·`pivot_clause`)와 Python 2(`match_statement`)는 손자를 돌려준다(child 안으로 내려감). Swift 8(`function_declaration`·`parameter`·`type_annotation`)은 직접 child를 돌려주지만 cursor는 그 child에 그 field를 보고하지 않는다.
  * cursor가 보고한 field를 `child_by_field_id`가 찾지 못한 경우: 8건 모두 ERROR node다(C# 4, TypeScript 2, SVC inline C# 1, Swift 1).
  * kit가 충실히 보고한 runtime API 동작이며 S08로 넘긴다.
* 대용량(A15·A17, `native-query-large`, 선언 query):
  * 세 fixture 모두 `COMPLETED`이고 S05 선언 항목을 재현했다.

    | fixture | Job peak commit | wall | 응답 크기 |
    |---|---|---|---|
    | 22m | 3351478272 bytes | 27.4초 | 9951638 bytes |
    | 8m-errors | 1236459520 bytes | 10.3초 | 3663466 bytes |
    | 32mib-errors | 5201428480 bytes | 43.1초 | 15658478 bytes |

    응답 크기는 그 실행의 기록 set manifest member bytes다(`artifacts/.../session-06/large-record-set-manifest-b76c466.json`).

  * 32 MiB 응답은 출력 상한 16777216의 93%다. pack 선언 항목이나 fixture가 조금만 커져도 `OUTPUT_LIMIT`로 바뀔 수 있는 여유다.
  * 모든 node를 잡는 query를 쓴 상한 초과 입력 하나는 `RESOURCE_LIMIT`/`OUTPUT_LIMIT`다. capture를 잘라 완료로 보고하지 않는다.
  * 같은 실행의 S05 r3 대용량 3개도 8 GiB에서 `COMPLETED`다(32 MiB peak 5157142528).

## 분리 context 리뷰와 처분

같은 분리 context reviewer(general-purpose subagent, GitHub 승인 아님)가 네 차례 리뷰했다. 기록은 `artifacts/.../session-06/review-r1.json`~`review-r4.json`이다.

**r1** (`ac70005..297c13a`, EXECUTED `go vet`·`go test ./src/kit` + STATIC): BLOCKER 0, MATERIAL 2, MINOR 10, NOTE 6. 처분은 `da9278f`, `98c1bc1`, `139ea3c`다.

* M1: `incremental`이 r3 연산을 비Windows에서 거부하지 않았다. 공통 `platformScope`를 build 전에 둔다.
* M2: r2 query 결과에 `predicates`가 없으면 "predicate 없음"으로 읽혔다. member를 필수로 하고 `NOT_EVALUATED` 선언을 확인한다.
* m1: 형제 탐색 차이 규칙이 runtime 동작보다 넓었다. 경계 zero-width 형제를 건너뛸 때만 허용한다.
* m2: 선언 도출이 이름까지 이어진 사슬 안에서만 첫 후보를 골랐다. S05 `locate`처럼 첫 후보에서 멈춘다.
* m3: 사례가 빠진 set이 검증될 수 있었다. 사례 목록, 형식 검사, 직렬화 실패 처리, 실행 상태 노출을 더했다.
* m4: match 수·match당 pattern·API point byte·step 없는 시간 상한 조건을 확인한다.
* m5: driver의 unmapped 검사가 한도 code에 가려졌다.
* m6: `UNSUPPORTED` query의 edit 비교가 빈 비교였다. 구조 stream을 비교한다.
* m7: 동적 SQL을 encoding별로 decode하고, comment를 제외하며, known-miss 범위를 다음 EXEC까지로 제한하고, text로 판정한다.
* m8: mapping `fact_fields`를 구현에 맞췄다.
* m9: query 예산을 process wall 안으로 넣었다(`native-query` 4초, `native-query-large` 20초).
* m10: route query 기대값 FAIL을 CI 실패로 했다.
* n1·n6: 문구를 고쳤고 capability 검사 순서를 바꿨다.
* n3: Swift 분류를 바로잡았다(이 보고서).
* n2·n4·n5는 기록만 한다: 선언 field 의미 차이, field lookup 비용, XML pack 기대값 부재.

r1 수정 과정에서 두 회귀가 생겼고, 로컬 route 실행에서 잡아 `139ea3c`에서 고쳤다.

* 깊이별 선언 pattern이 32 MiB 응답을 출력 상한 위로 밀었다. 선택적 quantifier 변형은 소유 node마다 match를 두 번 만들었다. 선언 node 한 번과 첫 후보·마지막 단계의 부모→후보 edge만 잡게 바꿨다.
* extra capture를 전역에서 지웠더니, tree-sitter-mssql에서 extra인 batch 수준 ERROR가 빠져 known miss를 놓쳤다. comment만 제외하게 바꿨다.

**r2** (`297c13a..139ea3c`, EXECUTED + STATIC): 다시 확인한 r1 항목 15건 중 13건이 RESOLVED였다. m7은 대부분 해결이고 남은 한계(`;` 없는 문장 뒤의 known-miss 범위)는 문서화했다. n3은 보고서를 고치지 않아 NOT RESOLVED였다. 기록만 하기로 한 n2·n4·n5는 다시 확인하지 않았다. 새 MINOR 2건과 NOTE 4건이 나왔다.

* R2-m1: 중첩 member도 필수로 했다.
* R2-m2: capability는 정상 r2 응답에서만 판정한다.
* R2-n1: C# comment 제외 조건을 T-SQL과 맞췄다.
* R2-n2: manifest 사례 목록을 profile에서 만든다.
* R2-n3: 출력 여유를 이 보고서에 기록했다.
* R2-n4: `requireMembers`의 두 번 decode는 기록만 한다.

처분은 `b76c466`이다.

**r3** (`139ea3c..b76c466`, EXECUTED + STATIC): R2 네 항목이 모두 RESOLVED이고 새 결함은 없다. status→exit 표가 `run.go`와 `protocol.go`에 중복된다는 참고는 동작 결함이 아니어서 코드를 바꾸지 않았다.

**r4** (보고서 commit, STATIC): n3·R2-n3은 RESOLVED였다. 근거와 맞지 않는 서술 두 가지(r2 집계, known-miss 규칙 서술)와 근거가 보존되지 않은 서술은 이 개정에서 고쳤다. API 분류, 응답 크기, query 시간 수치가 여기에 해당한다.

## Q 행 연결과 남은 범위

feature disposition에서 `Q`를 요구하는 행은 154개다. 이 중 이번 query 사례의 `features`가 직접 덮는 것은 46행이다. 나머지 108행은 query 사례가 없어 `NOT_RUN`이며 N/A로 바꾸지 않는다. S05 feature 사례가 덮는 Q 행(98)보다도 적은데, 사례마다 query가 실제로 capture하는 구조만 연결했기 때문이다. 이 108행은 S08 qualification 전에 사례를 더해야 한다. XML pack query는 구조 capture 수만 기록하고 기대값이 없다. XML 구조의 기대 capture는 route query 사례가 맡는다.

## 정적 점검과 Linux·macOS 위험

* 모든 native 실행과 build는 cgroup parent를 넘긴다. `Oracle`은 요청의 cgroup parent로 build·실행하고 hard memory backend를 확인한다. 새 시험은 `testBuildRequest` 또는 cgroup이 들어간 `OracleRequest`만 쓴다(`TestHostSettingsReachCI`).
* helper는 `exit 0`/`exit 1`로 끝나고, 잘린 native pipeline이 없다(`TestCIScriptPatterns`). 새 외부 도구는 없으며 경로는 `Join-Path`로 만든다. root 전용 읽기나 GNU/BSD 차이가 있는 새 명령은 없다.
* Linux sanitizer step은 새 owned 시험도 ASan/UBSan driver로 실행한다. 단 `TestOracleRecordSet`의 build는 sanitizer 설정을 받지 않는다(제품 `Oracle` 경로). r2 driver 코드는 다른 시험의 sanitizer build로 실행된다.
* 확인하지 못한 위험:
  * Linux·macOS에서 r2 query·API 시험과 26 route oracle 실행. 로컬은 Windows뿐이다.
  * Linux sanitizer step의 시간. 상한 420초, 로컬 native package 136초다.
  * hosted windows-2025의 8 GiB 대용량 실측과 route job 시간. 이제 route마다 build를 두 번 하고 대용량을 두 번 실행한다.
  * macOS sampled 메모리의 시간 여유(#65).
  * 32 MiB 응답 출력 여유 7%.

## PR CI 1회차 실패와 수정 (Refs #61)

PR #67 run 37141714979(head `a78d5cc`)에서 foundation windows-2025만 실패했다. orchestrator 보고에 따르면 Linux·macOS foundation은 통과했고 native job은 건너뛰었다. 이 run의 CI receipt는 orchestrator가 소유하며 로컬에서는 확인하지 않았다. 실패한 시험은 `src/kit` `TestCorpusLimits/WALL_LIMIT`이고, 결과는 "want RESOURCE_LIMIT/WALL_LIMIT, got <nil>"이었다.

* 원인: 이 사례와 `TestLimits/WALL_LIMIT`은 kit wall을 1 ns로 두고 작은 fixture를 처리했다. Windows timer 해상도에서는 처리가 timer보다 먼저 끝날 수 있다. #61이 추적하는 시험 결함이며 이번 S06 변경과 무관하다. 다만 필수 CI를 무작위로 막으므로 이 PR에서 고쳤다.
* 수정(`910b072`, `6550b6b`, `76db754`과 그 다음 commit):
  * kit에 비공개 시험 hook `testHookWall` 하나를 두었다. 시험이 이 hook을 설정하면 timer 대신 그 wall context를 쓴다. hook이 nil이면 제품 경로는 이전과 같다.
  * 두 사례는 기존 `testHookOpen`으로, walk가 첫 파일을 열 때 wall을 kit의 cause로 만료시킨다. 그래서 작업 도중 `WALL_LIMIT` 경로를 반드시 탄다. 단언(`RESOURCE_LIMIT`/`BLOCKED`, 부분 결과 없음)은 그대로다.
  * caller 취소와 wall 만료가 같은 지점에서 일어나도 `CANCELLED`(부분 결과 없음)인지 보는 사례를 더했다.
  * 실제 timer 경로는 `TestStartRunWallTimer`가 본다. hook 없이 1 ms wall의 `Done`을 기다린 뒤 `WALL_LIMIT`과 cause를 확인하므로 timer와 경쟁하지 않는다.
* 검증:
  * `8ec1767`에서 세 시험(`TestStartRunWallTimer`, `TestCorpusLimits`, `TestLimits`)을 각 200회 반복했다. 600회 모두 PASS다(`.work/session-06/verify-r7-count200.log`).
  * mutant 5개를 추가했고 모두 검출됐다: wall 검사 제거(두 사례), caller 우선순위 역전, timer가 울리지 않음, cause 없는 timer. `910b072`에서는 caller 우선순위 mutant가 살아남아 `6550b6b`에서 두 상한이 같은 지점에서 만료되는 시험을 더했다. 이 첫 실행의 log는 보존하지 않았다.
  * 전체 mutant 27/27(`76db754`). CI 방식 foundation step은 `76db754`와 `8ec1767`에서 exit 0이다.
* 분리 리뷰 r5·r6에서 MINOR 1건(실제 timer 경로의 시험 부재)과 NOTE 4건이 나왔다. 범위 밖으로 기록만 한 R5-n3(native·runner의 여유 큰 시간 가정)을 빼고 모두 처분했다. r7(STATIC)은 R6-n1을 RESOLVED로 확인했다. 함께 지적한 보고서 서술 세 곳(R7-1~3: mutant 생존 시점, 반복 횟수의 근거, CI 결과의 출처)은 이 절에서 고쳤다.

## post-merge CI 실패와 수정 (#65)

merge commit `8b93110`의 post-merge run 37145206819(attempt 1)에서 foundation macos-15만 실패했다. orchestrator 보고와 그가 저장한 로그(`.work/orchestrator/ci-37145206819-failed.log`)에 따르면 ubuntu·windows foundation은 통과했고 native job은 건너뛰었다. 실패한 시험은 `src/internal/runner` `TestEscapedQuietDescendant`이고, 0.23초 만에 "expected the documented limitation: escaped pid 3856 already gone"으로 끝났다. 같은 tree가 PR run 37143724041의 macOS에서는 통과했다.

* 원인(시험 결함, 분리 리뷰 r8이 정적으로 특정):
  * escapee pid는 두 번 기록된다. helper의 `spawnIO`가 한 번, escapee(`sleep` mode)가 자기 pid를 한 번 기록한다.
  * 이전 판정 loop는 pid마다 `alive(pid)`를 본 뒤 바로 `killPID(pid)`를 했다. 그래서 두 번째 항목에서 방금 SIGKILL한 같은 pid를 다시 probe했다. 그 사이 macOS에서 escapee가 zombie가 되거나 회수되면 `alive`가 false가 되어 "already gone"으로 실패한다. kill 신호가 처리되는 시점에 따라 결과가 달라지므로 간헐적이다.
  * 0.23초는 helper의 200 ms sleep과 맞는다. Linux CI(cgroup)와 Windows(Job Object)는 이 loop에 이르지 않는다. `TestEscapedDescendant`는 probe 없이 kill만 하므로 이 문제가 없다.
  * orchestrator 가설("escapee가 group을 떠나기 전에 runner가 group을 끝낸다")은 Go 소스로 확인한 결과 성립하지 않는다. darwin의 `forkAndExecInChild`(`syscall/exec_libc2.go`)는 exec 전에 `setsid`를 부른다. `forkExec`(`syscall/exec_unix.go`)는 exec 성공(CLOEXEC pipe의 EOF)을 확인한 뒤에야 돌아온다. 그래서 helper의 `cmd.Start`가 돌아온 시점에 escapee는 이미 group 밖에 있다.
  * macOS에서 재현하지는 않았다. 로컬은 Windows뿐이다.
* 수정: 판정은 probe 대신 handshake를 쓴다.
  * escapee(새 helper mode `escapee`)는 `.ready`를 만들고, helper는 그것을 본 뒤에만 끝난다(최대 20초, 넘으면 exit 69). posix에서는 escapee가 그 전에 자기 group의 leader인지(`Getpgrp() == Getpid()`) 확인한다. Windows에서는 group 확인이 없다(`ownGroup`이 항상 true). 시험은 run이 `COMPLETED`/exit 0인지 먼저 확인한다.
  * NOT_CONTAINED 증명은 ping/pong이다. run이 끝난 뒤 시험이 `.ping`을 쓰고, escapee는 그 ping을 본 경우에만 `.pong`을 쓴다. 살아 있는 process만 답할 수 있으므로 runner가 escapee를 끝내지 않았다는 증거다. 답이 20초 안에 없으면 실패한다. escapee는 `t.Cleanup`에서 끝내므로 실패 경로에서도 남지 않는다. 중복 기록된 pid는 그대로 두었고, 이제는 해가 없다.
  * Job Object·cgroup backend의 판정(`requireDead`)과 `Scope=PROCESS_GROUP` 검사는 그대로다.
* 함께 고친 #65 사례:
  * 300 ms + 200 ms 정책 frame 사례(`TestFrameStatusMapping`의 small policy 사례, `tail`, `request-cap`, `TestFrameEscapedStdoutHolder`): runner에 비공개 시험 hook `testHookBatchStarted`를 두었다. 시험은 이 hook에서 frame helper가 `.ready`를 만들 때까지 기다린다. 그래서 첫 frame의 watchdog에 helper 시작 시간이 들어가지 않는다.
  * `TestFrameEscapedStdoutHolder`의 stdout holder는 helper가 `.ready` 전에 만든다. 이전에는 frame 1의 watchdog 안에서 만들었다. 시험은 holder pid가 기록됐는지도 확인한다.
  * `batch-wall`: 비공개 시험 hook `testHookWall`은 실제 wall timer가 울린 뒤, wall을 기록하기 전에 실행된다. 시험은 frame 2가 helper에 도착했다는 `.at`을 볼 때까지 wall 기록을 미룬다. 그래서 frame 0·1이 끝나기 전에 wall이 기록되지 않는다. timer 경로 자체는 그대로 쓴다.
  * native `TestLimitsAndCancellation/cancellation`: `Exec`와 같은 spec을 interactive로 시작한다. 요청 frame의 마지막 1 byte만 빼고 쓴 뒤 취소한다. 그 시점의 driver는 요청을 거의 다 읽고 남은 byte를 기다리는 실행 중 상태라 먼저 답할 수 없다. 단언(`CANCELLED`, cleanup `Verified`)은 그대로다. 다만 `Exec` 함수 자체 대신 그 spec의 사본을 쓴다.
  * 두 hook 모두 nil이면 제품 경로는 이전과 같다. runner·driver 상한과 기대값은 바꾸지 않았다.
* 같은 종류의 가정을 runner·native 시험 전체에서 정적으로 점검했다. 고친 것은 다음 두 가지다.
  * `TestTreeTermination`: wall 1.5초 또는 취소 전에 3단계 tree가 모두 시작해야 했다. 지금은 만료(wall hook 또는 취소)가 기록된 pid 3개를 기다린다. 만료가 시작 뒤 1.5초 + 5초 안에 오는지(이전 상한과 같은 여유), 종료가 만료 뒤 grace + 5초 안에 끝나는지를 따로 확인한다.
  * `TestPipeHoldingDescendant`(`orphan`): 200 ms 안에 손자가 시작해야 했다. 지금은 helper가 손자의 pid 기록을 기다린 뒤 끝난다.
* 고치지 않고 이름만 남기는 여유는 다음과 같다.
  * small policy의 frame 1·2 응답: process 시작이 없는 왕복 하나에 500 ms.
  * `own-memory`: 표본 메모리 관측에 frame watchdog 20초.
  * `TestFrameCrashWithStdoutHolder`(10초)·`TestFrameEscapedStdoutHolder`(15초)·`TestPipeHoldingDescendant`(15초)의 hang 감지 상한. run 37145206819의 macOS에서 실제 소요는 2.53초 이하였다.
  * native `parse-timeout`: 4.6 MB parse가 `ParseMillis` 1 ms보다 오래 걸린다고 가정한다. 전체 parse 시간은 따로 재지 않았다.
  * native `TestQueryLimits/time`: 약 60 KB 입력의 query가 `QueryMillis` 1 ms를 넘는다고 가정한다. driver는 정수 ms로 `now - start > limit`을 보므로 실제로는 2 ms 이상 걸려야 한다. 여유는 재지 않았고 위의 `parse-timeout`보다 작다.
  * `requireDead`: 종료 뒤 5초.
* commit: `f603cdc`, `ee36047`, `9aa971b`, 리뷰 수정 `e168458`, `2f95de7`, `a6f0a9e`.
* 검증(Windows amd64 로컬, Job Object backend):
  * 반복: 영향받은 runner 시험 7개(`TestTreeTermination`, `TestPipeHoldingDescendant`, `TestEscapedDescendant`, `TestEscapedQuietDescendant`, `TestFrameEscapedStdoutHolder`, `TestFrameCrashWithStdoutHolder`, `TestFrameStatusMapping`)를 `-count=100 -failfast`로 `ee36047`(662초)과 `2f95de7`(658초)에서 돌렸고 모두 통과했다. native `TestLimitsAndCancellation/cancellation`은 `a6f0a9e`에서 100회 통과했다(`.work/session-06-postmerge/repeat100-*.log`).
  * targeted mutant: `2f95de7`에서 10/10을 검출했다(`.work/session-06-postmerge/mutants-2f95de7.json`). 검출한 mutant는 다음과 같다.
    * wall timer 10배 지연, wall을 취소로 기록, 취소 무시(runner 시험과 native 시험 각각), 잔여 descendant 미종료, Job 종료 no-op
    * frame watchdog 제거(두 시험), batch wall을 `FAILED`로 기록, batch wall 무시
    * native 취소 무시 mutant는 90초 wall에서야 끝났다. driver가 남은 1 byte를 기다리며 실행 중이었다는 뜻이다.
    * `a6f0a9e`는 쓰기 실패 경로만 바꿨고 mutant를 다시 돌리지 않았다.
  * CI 방식 foundation step(`형식과 foundation 검사`, `TSGK_NATIVE_REQUIRED=1`): `a6f0a9e`에서 exit 0, 119초였다. skip 목록은 S06 때의 harness 실행과 같다. 그 밖에 `go vet`(windows·`GOOS=linux`·`GOOS=darwin`), `gofmt`, `go build`, `git diff --check`가 통과했다.
  * 실행하지 못한 것: macOS·Linux(process-group, cgroup) 실행, `-race`. NOT_CONTAINED 분기는 macOS CI에서만 실행된다.
* 분리 context 리뷰:
  * r8(STATIC): MATERIAL 1, MINOR 2, NOTE 3.
    * MATERIAL은 만료 시점부터만 재면 늦은 wall을 놓친다는 지적이었다.
    * MINOR 2건은 원인 미특정과 사전 취소로 인한 의미 축소였다.
  * r9(STATIC): 6건 모두 RESOLVED로 확인했다. 새로 나온 NOTE 1건(쓰기 실패 시 runner 미정리)은 `a6f0a9e`에서 고쳤다. 그 수정은 다시 리뷰받지 않았다.

## 남은 일과 한계

* 세 OS CI(foundation, native prepare, native routes 세 job, Linux sanitizer), PR·merge·post-merge는 이 세션 범위 밖이며 orchestrator가 한다.
* `go test -race`는 실행하지 않았다(CGO 없는 build).
* API claim FAIL 23건(runtime field lookup 동작), grammar gap(S05에서 넘겨받음), Q 행 108개 미연결, corpus ERROR 230개는 S08 처분 대상이다. kit 결함으로 분류한 것은 없다.
* 동적 SQL known miss 중 `;` 없는 문장 뒤의 범위는 다음 EXEC까지로만 제한된다. schema가 `sys`가 아닌 첫 호출은 탐지하지 않는다(mapping 정의상 `sp_executesql`이 아님).
* query 시간 상한에서 runtime이 멈추지 않는 경로는 부분 capture를 남기지 않는다.
* 비공개 corpus는 이번 세션에서 실행하지 않았다(S06 범위 밖).
