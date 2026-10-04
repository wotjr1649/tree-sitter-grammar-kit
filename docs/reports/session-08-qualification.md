# Session 08 — Real-world & Cross-platform Qualification 결과

Campaign `TSGK-C1-20260929-R1`, 추적 `TSGK-C1-S08`, Issue #10, Milestone 9, branch `session/08-real-world-qualification`(base main `3fa7ed3`). 이 보고서는 아래에 적은 revision과 로컬 관측만 설명한다. PR·세 OS CI·merge·post-merge 결과와 hosted 78칸 행렬은 이 문서 작성 시점에 없으며 그 단계의 receipt가 소유한다.

## 범위 결정

orchestrator 조정(2026-10-04, 선택지 A)에 따라 S08은 qualification 장치와 현재 후보의 78칸 행렬을 두 축(kit mechanism, 요구 coverage)으로 내고 새 언어 사례·기대값은 작성하지 않는다. 지원 claim은 하지 않는다. 실행하지 않은 필수 feature를 다른 이름으로 바꾸지 않는다. 사례가 없는 필수 의무는 S08 밖의 후속 작업 [#70](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/70)이 추적한다. 이후 사용자 결정 `C1-TSQL-PATCH-R6`이 T-SQL r6 실패 사례에 한해 이 범위를 고쳤다(아래 `T-SQL patch r6` 절).

## 구현 범위

* **qualification inventory `tsgk-qualification-inventory/r1`**([`qualification-c1.json`](../../src/contracts/qualification-c1.json)): campaign 정의의 26 route × 3 platform(78칸), [feature disposition](../validation/language-feature-disposition.md)의 REQ 178행 × 필수 case kind(826 의무), route마다 `run-routes.ps1`이 실행하는 S06 기록 workload(grammar 파일, query source, fact pack, 등록 사례의 입력·edit·기대값·query 기대값)와 추가 역할 행(NET461 SVC·대용량, owned native fixture, BrightScript maintained·historical, Cooklang audit)을 담는다. `TestQualificationInventory`가 등록부와 사례에서 다시 만들어 bytes로 대조한다.
* **coverage 규칙 `tsgk-coverage-rule/r1`**: 사례가 그 route의 REQ 행을 `features`로 적을 때 P(`NO_ERROR`+구조), N(`ERROR`), R(`ERROR`+보존 구조), E(edit), Q(capture 기대값이 있는 query 사례)를 덮는다. W(허가된 실사용 sample)는 등록 생산자가 없어 덮이지 않는다. inventory parser가 규칙을 강제한다.
* **`kit.Qualify` / `tsgk qualify`**: 계약은 [identity/evidence](../specs/identity-and-evidence.md)·[CLI/profile](../specs/cli-and-profile.md)·[공개 API](../specs/public-go-api.md)의 `S08 구현`이다. host마다 실행 identity(`tsgk-run-identity/r1`)로 cohort·platform·NEW_RUN·후보 commit 자격을 확인하고, S06 기록 set을 공유 set 규칙·등록 대조·S07 `oracle-set-r1` 사례 gate로 검증한 뒤 kit 축과 요구 축을 계산한다. route마다 세 host의 사례별 의미 요약(tree digest·node 수·`has_error`, query capture stream, API 관측, 비교·판정)을 정확히 비교하고 host 관측(시간·메모리·build identity)은 따로 보존한다. 결과 `tsgk-qualification-result/r1`은 칸, 추가 역할 행, 비교, `mechanism_gate`, `support_claim`을 담는다. process·network를 쓰지 않는다.
* **CI**: `native routes (<os>)`가 실행 identity를 쓰고 기록 set·workload profile을 artifact로 올린다. 새 `qualification (ubuntu-24.04)` job이 같은 run·attempt의 세 artifact로 `tsgk qualify`를 실행하고 `mechanism_gate`로 성공을 정한다([validation](../validation/validation.md)). heavy job은 늘지 않는다.
* **공개 사용 검증(A10)**: `TestModuleProxyConsumer`(추적 파일로 만든 module zip을 파일 module proxy로만 받아 별도 module이 version으로 build, CLI와 같은 bytes), `TestGrammarUpdateWorkflow`(CLI만으로 기준 identity → 후보 verify FAIL → schema diff 검토 항목, 입력 불변), `TestQualifyCLI`(PATH 없이 같은 bytes, 신뢰 문서·출력 위치 guard, no-clobber).
* README의 구현 기능 표와 지원 상태(BLOCKED), CHANGELOG, platform 계약 표를 실제 상태로 고쳤다. tag·Release·package·Go 소비자 채택은 하지 않았다.

제외: T-SQL r6 밖의 새 언어 사례·기대값, r6 밖의 grammar 수정, upstream 수정, 비공개 corpus의 hosted 전송, BrightScript bundle 다운로드와 replay.

## T-SQL patch r6(`C1-TSQL-PATCH-R6`)

사용자 결정 `C1-TSQL-PATCH-R6`(2026-10-04)과 보충 `C1-TSQL-PATCH-R6-ADDENDUM-R1`이 채택 T-SQL 후보 `P05-MSSQL-REMEDY-r1`에 patch r6을 더하게 했다. 두 receipt는 총괄 기록(`artifacts/.../orchestrator/`)에 있고, 이 Session은 크기와 sha256을 확인한 뒤 적힌 범위만 적용했다. 이 결정은 위 범위 결정(D5)을 T-SQL r6 실패 사례에 한해 고친다. 다른 언어의 사례·기대값은 여전히 쓰지 않는다.

* **규칙**: class마다 Microsoft 공식 T-SQL 문서나, 그 정확한 모양의 SQL Server 엔진 동작을 보여 주는 독립 공개 출처로 유효성이 확립될 때만 patch했다. 확립되지 않은 class는 조사한 출처와 함께 grammar gap으로 남겼다. 추론으로 정하지 않았다. 모든 class가 채택 범위 행 tsql-B01~B04 안이며 범위 행은 바꾸지 않았다.
* **subject**: [remedy-tsql-r6.json](../../src/dev/prepare-p05/remedy-tsql-r6.json)이 class별 출처 URL과 주장, gap과 조사 출처, 기록된 과잉 수용, upstream 시험 처분, 10개 grammar 파일의 literal 치환(`replacements`)을 소유한다. [native route 등록부](../../src/contracts/native-routes.json)의 tsql `patch_chain`이 기존 chain 뒤에 이 subject를 붙이고, [후보 등록부](../../src/contracts/language-sources.json) `adoption.revisions`가 r6을 기록한다.
* **패치한 class**(비공개 ERROR 파일 수): Unicode regular identifier(54), scalar subquery 피연산자(17), 문장 뒤와 batch 첫머리의 추가 `;`(5), BEGIN 바로 뒤의 `;`(8), `DEFAULT … WITH VALUES`(13), NULL 앞 DEFAULT(1), 열 수준 PRIMARY KEY/UNIQUE 열 목록(2), CREATE TABLE 마지막 열 뒤 쉼표(1), `SELECT @v +=`(4), FROM 없는 WHERE(4), batch 첫 문장의 EXEC 생략 호출(3, CHECKPOINT·SHUTDOWN 문장 포함), BACKUP/RESTORE와 master key·certificate 백업(2), DROP INDEX 옵션(1), CREATE DATABASE file spec(1), subquery union 분기의 TOP … ORDER BY(2).
* **grammar gap으로 남긴 class**: 열 수준 FOREIGN KEY의 자체 열 목록(5), ASC/DESC가 있는 열 수준 key 열 목록(10), 마지막 table constraint 뒤 쉼표(1), CAST 식의 xml method 호출(2), RESTORE MOVE 변수(1).
* **BEGIN 뒤 `;`**: 처음에는 block·본문 첫 문장 앞의 `;`(8)를 일반 주장 출처만 있어 gap으로 두었다(리뷰 r3). 이후 총괄이 전달한 [BEGIN...END 공식 문서](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/begin-end-transact-sql?view=sql-server-ver17)의 `BEGIN [ ; ]` 문법과 Remarks를 이 Session이 직접 확인했다. 8개 파일은 모두 프로시저 안 BEGIN…END block의 BEGIN 바로 뒤(IF 2, 프로시저 본문 2, WHILE 4)라 그 근거로 patch했다(`r6-begin-semicolon`). 같은 페이지가 빈 block은 세미콜론이 있어도 syntax error라고 하므로 `BEGIN ;` 뒤에는 문장이 하나 이상 있어야 한다. BEGIN TRY/CATCH와 함수 본문의 BEGIN은 각 문법에 근거가 없어 바꾸지 않았다. 같은 페이지의 Example A(문장·주석 뒤 단독 `;`)는 빈 문장 class의 공식 근거로 더했다(이미 받던 모양).
* **기록된 과잉 수용**: batch 첫 위치에서 문장이 아닌 예약어 단독 입력(`PERCENT;`)을 모듈 호출로 읽는다(예약어 집합은 GO 없이 이어지는 batch 구조 때문에 모든 문장 끝 상태에 걸려 기존 입력을 깨뜨려 버렸다). 식별자 문자 범위는 Unicode 3.2보다 넓다. ENCRYPTION/DECRYPTION BY 뒤는 공용 `option` 모양이라 `BY CERTIFICATE c`도 받는다. r5부터 받던 빈 `BEGIN END`와 `;` 없는 `BEGIN WITH`도 그대로 둔다.
* **ORDER BY 구조**: subquery 안에서 TOP이 있는 분기 뒤 ORDER BY는 그 분기의 `query_specification`에 붙고, TOP이 없는 마지막 분기 뒤나 OFFSET/FETCH가 따르면 union에 붙는다. top-level `set_operation`은 바뀌지 않는다. 근거는 ORDER BY 공식 문서(최상위 query에만 적용되는 제한, subquery의 ORDER BY는 TOP 행 결정용)와 SQLServerCentral 엔진 관찰(두 TOP 분기 모양이 subquery에서 실행되고 top-level에서 Msg 156)이며, 분리 리뷰가 독립적으로 같은 결론을 냈다. 앞 분기에 TOP이 없고 마지막 분기만 TOP인 모양은 엔진 관찰이 없고 r5와 tree가 다르다는 점을 subject에 적었다.
* **선기록과 기대값**: 실패 사례와 기대 구조는 patch 전 commit `0998332`(BEGIN 뒤 `;`는 `0961f43`)에 소유 합성 사례로 등록했다(등록 당시 parser에서 실패). 기대값은 공식 사실에서 손으로 적었고 parser 출력을 답으로 쓰지 않았다. 최종 사례는 feature 2개(r6 전체, BEGIN 뒤 `;`), 오류 하나씩만 담은 음성 4개(table variable 끝 쉼표, top-level 분기 ORDER BY, view 본문의 `;WITH`, 빈 `BEGIN; END`), 구조 query 2개(EXEC 생략 호출, RESTORE MOVE, 복합 대입, 분기·union ORDER BY와 OFFSET, block 안 CTE와 중첩 block 문장을 순서 있는 capture로 고정)다. 이름은 비공개 corpus와 무관한 중립 값이다.
* **동적 SQL 기대값**(addendum 선택지 A, 별도 commit `b6ffb56`): fixture의 `sp_executesql N'SELECT 5 AS a';`가 execute_statement가 되어 기존 `dynamic-sql-r1` 규칙이 SP_EXECUTESQL 사실 하나(bytes [1174,1190), fixture bytes에서 계산)를 더 내고 그 KNOWN_MISS를 지웠다(사실 13→14, known miss 3→2, 강화). fixture bytes와 comparator는 그대로다. mapping의 known miss 설명은 detector가 실제로 보고하는 경우로 고쳤다. detector는 batch 첫 문장이 아닌 `;sp_executesql`도 known miss로 보고하는데, detector 변경은 허가 범위 밖이라 한계로 기록한다.
* **재생성과 재현**: 등록 tree-sitter 0.27.0·node 24.21.0으로 `prepare-routes.ps1`의 실제 `Get-PatchedText` chain 적용 → `tsgk reproduce` 두 작업 공간 재현이 PASS(deterministic, reference_match)다. 새 parser.c는 29214301 bytes `c0f542b63fdd…`이며 등록부의 identity로만 식별한다(D1 예외 없음). S03 `tsgk schema check`는 PASS, r5 대비 `schema diff`는 추가 31건과 `execute_statement` 필수 child 완화 1건이며 제거는 없다.
* **기존 사례 회귀**: upstream fixture 156개, PREPARE tsql 등록 입력 42개(손상 변형 포함), 등록 tsql route·gap·query 사례의 tree가 r5와 bytes 단위로 같다. 다른 것은 위 동적 SQL fixture뿐이다. tsql route 실행(14 사례)은 PASS이고 API claim FAIL은 r5 기준선과 같은 5건(`field_lookup_extra:name`)이다. EXEC 생략 호출과 복합 대입 규칙은 field가 부모로 새지 않도록 visible 규칙으로 두었다.
* **upstream 시험**: upstream `test/corpus` 351개 중 실패가 r5의 2개에서 13개가 됐다. 새 11개 중 6개는 공식 문서상 유효한 입력을 upstream이 오류로 기대한 경우(EXEC 생략 batch 첫 호출 5, FROM 없는 WHERE 1)이고, 5개는 여전히 ERROR이며 복구 모양만 바뀌었다. upstream 시험은 바꾸지 않았다.

### 분리 context 리뷰(r6)

general-purpose subagent 두 개가 각각 따로 리뷰했다(GitHub approval 아님, EXECUTED probe·재현·시험 + STATIC).

* **r6 grammar·등록부**: r1(`1366d36..034f295`) BLOCKER 0·MATERIAL 3·MINOR 8·NOTE 10 → `8559ad9`. MATERIAL은 EXEC 생략 호출의 예약어 수용, 마지막 TOP 분기 뒤 OFFSET의 결속, 증거보다 넓은 끝 쉼표였다. r2 0·0·2·4 → `472f214`. r3 0·1·1·2 → `54f6872`(문장 목록 첫머리 `;`를 추론으로 넓힌 것을 되돌리고 gap으로 기록, master key 항목 순서). r4 0·0·1·2 → `f4890e7`(조사 출처 URL). 보고서 r5 0·0·0·3. BEGIN 뒤 `;`의 기대 구조와 patch 범위 r6 0·1·0·2 → `b48c255`(`BEGIN; END` 거부와 그 음성 사례), 변경분 r7 0·0·1·2 → 이 보고서 갱신. 남은 BLOCKER·MATERIAL은 0이다.
* **동적 SQL 기대값**(`b6ffb56` 전용 기록): r1 0·0·1·3, r2 0·0·1·2, r3 0·0·1·2, r4 0·0·0·2. MINOR는 모두 mapping 문구였고 `dbe9e21`, `17130e8`, `d2a08b1`에서 처분했다. r1 NOTE의 `procedure` field 누수는 `8559ad9`에서 해소됐다.

## 로컬 검사(Windows)

* `gofmt`, `go vet ./src/...`, `go test ./src/kit ./src/cmd/tsgk ./src/internal/foundation`: 통과(`CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOFLAGS=-mod=readonly`).
* 새 시험: `TestQualifyBaseline`(A01·A02), `TestQualifyCompleteness`(A04), `TestQualifyCohort`(A05), `TestQualifyComparison`(A06·A07), `TestQualifyDetectorAndGap`(A08), `TestQualifyEligibility`(A09), `TestQualifyMechanism`(A12), `TestQualifyLimitsAndCancel`(A11), `TestQualificationInventoryGuards`, `TestQualificationInventory`, `TestQualifyCLI`, `TestModuleProxyConsumer`, `TestGrammarUpdateWorkflow`.
* **targeted mutant**: `0cdf3fd`에서 29/29, 리뷰 r1 수정 뒤 33/33 검출(`.work/session-08/mutants.py`, 로컬 `mutants-0cdf3fd.json`·`mutants-r1.json`). r1에서 더한 mutant는 등록 검사 무시, 거부된 host를 cohort 기준으로 사용, 역할 행 MISSING의 완결성 무시, detector 결과 무시다. `SUPPORTED`에 gate를 요구하는 줄을 지우는 mutant는 동등 mutant라 뺐다(모든 칸과 실행 역할 행이 PASS이면 gate도 PASS다). 29개 목록: 실패 route 칸 제외, 다른 host로 빈 칸 채우기, dialect 병합(set route 미검사, 두 route가 set 공유), 역사 relabel(evidence mode·후보 commit 미검사), cohort·architecture(identity·set) 미검사, 등록 대조·workload 결속 생략, 비교에서 capture·digest·API 제외, host 관측 비교, 칸이 비교 실패 무시, 사례 없음을 PASS로, detector의 요구 행 덮기, detector로 gap 숨김, kit claim 무시, 기록 query claim 신뢰, record 수 미검사, 등록되지 않은 파일 허용, 중복 host 허용, 빈 칸 완결성 무시, gate가 kit 축 무시, coverage 규칙·칸 수 미검사, 모든 칸 PASS 없이 SUPPORTED. 모두 의도한 시험이 실패했다(compile 실패로 대신하지 않음).
* **CI 방식 재현(ci-sim, `9aa1500`)**: workflow의 `26 route native build와 등록 사례·query 기록 실행` step과 `78칸 qualification 집계` step을 그대로 꺼내 새 `pwsh -NoProfile`에서 job env와 RUNNER_TEMP 형태 디렉터리로 실행했다. route step은 exit 0(755초, routes 27, failures 0)이고 실행 identity를 썼다. artifact 경로를 그대로 복사해 qualification step을 실행했다. Windows 26칸은 모두 kit 축 PASS다. 리뷰 r1 수정 뒤 같은 근거를 다시 집계하면(`ddf45bb`) 등록 대조(선언 mapping·point·동적 SQL 기대값 포함)도 그대로 맞고, 추가 역할 `n461-large`는 PASS, `n461-svc`는 kit 축 PASS·등록 검사 BLOCKED라 `INCOMPLETE`다. SVC 음성 사례 8개(inline 해석 불가·VB·진단)가 S05 계약대로 `BLOCKED`로 끝나지만 등록된 기대 결과가 없어 통과로 셀 수 없기 때문이다. Linux·macOS host가 없어 52칸은 `MISSING`이고 완결성·`mechanism_gate`가 FAIL이며 step이 실패했다(한 host만 있는 로컬의 예상 결과). Windows에서 실행 파일 이름에 `.exe`를 붙인 것만 job과 다르다(job은 ubuntu).
* **r6 후보의 ci-sim(`f4890e7`)**: 같은 두 step을 r6 최종 코드 후보에서 다시 실행했다. tsql만 r6 parser로 다시 준비했고 나머지 25 route 준비물은 같은 등록 identity다. route step은 exit 0(830초, routes 27, failures 0), qualification step은 한 host 로컬의 예상대로 완결성·`mechanism_gate` FAIL이다. 결과는 아래 Windows 칸 절이다.
* **최종 후보의 ci-sim(`b48c255`, 중단)**: route step이 27개 route·oracle set(26 route와 SVC)을 모두 마친 뒤(FAILURE 줄 없음, tsql route 14 사례 PASS) Windows 대용량 C# query 기록 단계에서 host 메모리 부족으로 Claude Code가 배경 작업을 중단했다. 이 작업이 만든 자식 process(step `pwsh`, `tsgk`, native driver)를 부모 관계로 확인해 종료했고, 지시대로 다시 시작하지 않았다. 그래서 `b48c255`의 run identity와 qualification 집계는 없다. `f4890e7`과 `b48c255` 사이의 grammar 변경은 tsql `block` 규칙(BEGIN 뒤 `;`)뿐이고, tsql 사례 2개·query 1개가 늘었다.
* **세 host 경로 점검**: Windows host 근거를 실행 identity와 manifest platform만 바꾼 두 복제본으로 `tsgk qualify`를 돌리면 완결성 PASS, kit 축 78/78, 비교 FAIL 0, `mechanism_gate` PASS, 405 MB를 12.5초에 읽었다. 이것은 집계 경로와 한도의 점검이며 교차 OS 근거가 아니다.

## Windows 칸(ci-sim, r6 후보 `f4890e7`)

| 상태 | 칸 | route |
|---|---|---|
| FAIL(요구 FAIL, kit PASS) | 4 | csharp, typescript, tsx, swift |
| INCOMPLETE(사례 없는 의무, kit PASS) | 22 | 나머지 22 route |

의무 826개 중 PASS 316, FAIL 9, 사례 없음 501다(r6 전 `9aa1500`은 PASS 308, 사례 없음 509). tsql 칸은 의무 36개 중 PASS 25, FAIL 0, 사례 없음 11이며 등록 검사 PASS다. 등록 검사(모든 등록 기대값)는 22 route가 PASS, 4 route가 FAIL이다(csharp gap 2, typescript gap 4, tsx gap 2, swift gap 2 사례). kind별 사례 없음은 P 66, N 122, R 126, E 47, Q 106, W 34다. 요구 FAIL 9건은 넘겨받은 grammar gap이다: csharp B01·B03·V08·V09의 P(contextual keyword 식별자 등), typescript V40·V50의 P(TypeScript 5.0–5.3 구문), tsx B01·B02의 N과 swift V59의 N(lenient acceptance: 오류여야 할 입력을 받아들임). API claim FAIL은 26 route 22건, SVC 1건이다(S06과 같은 runtime field lookup 차이, tsql은 r5와 같은 5건). 세 host 비교는 hosted CI에서 처음 실행된다.

**#70과의 차이**: #70은 R 미덮음을 97로 적었다. kit 규칙은 R에 보존 구조 기대(`ERROR`+`contains`)를 요구하므로 `contains`가 빈 recovery 사례 26개는 N·E만 덮고 R은 덮지 않는다. 그래서 행렬의 R 미덮음은 r6 전 130, 합계 509였다(r6 사례로 126, 501).

## 비공개 corpus(A16, Windows 로컬, 개수만)

`NET461-PHASE2-LOCAL-r1`을 r6 최종 후보 `b48c255`(clean)에서 `run-corpus.ps1`(`private-corpus-local`, 실행 wall 7200초)로 다시 실행했다. 경로·이름·내용이 담긴 기록(실행 결과, ERROR 진단, 파일별 처분)은 추적하지 않는 로컬 artifacts에만 있다. 이 절은 개수와 판정만 적는다. 첫 실행(`0cdf3fd`, r5)의 결과는 아래 "이전 실행"에 남긴다.

* 실행: wall 701초, csharp 6624 파일(14 batch), tsql 6150(13), xml 5163(11), svc 20(native process 없음). 고정 runtime `659cda7c`, 기존 MSYS2 GCC(`75e87953…`). 다운로드 없음.
* inventory 21451 record: route 있는 파일 17957, `PRESENCE_ONLY` 133, route 없음 3494(따로 셈).
* `execution_status`: COMPLETED 17957, CANCELLED·RESOURCE_LIMIT·FAILED 0, `NOT_RUN` 0, 실행하지 않은 `assessment=BLOCKED` 0. 분류할 한도·환경·kit 결함 파일이 없고 kit 결함 0이다.
* `has_error`(COMPLETED이면서 ERROR) 111: csharp 48, tsql 62, xml 1. r5 실행보다 119개 줄었고(모두 tsql, 이전 `UNDISPOSITIONED` 2와 source damage 1 포함), 새로 ERROR가 된 파일은 없다.
* r5 실행과 파일별 대조(상태·판정·`has_error`·digest·node 수): csharp 6624와 xml 5163은 모두 같다. tsql은 r5에서 ERROR가 없던 5969개가 모두 같고, 달라진 139개는 ERROR→정상 119, ERROR 유지(복구 모양만 변화) 20이다. 직전 후보 실행(`d2a08b1`)과는 BEGIN 뒤 `;` 파일 8개만 달라졌다(ERROR→정상).
* ERROR 111개 처분(로컬 근거로 진단, 비밀 값은 인용·전송하지 않음):

  | 처분 | 파일 | 내용 |
  |---|---|---|
  | grammar gap | 62 | tsql 19: r6 subject의 gap — ASC/DESC가 있는 열 수준 key 열 목록 10, 열 수준 FOREIGN KEY 자체 열 목록 5, CAST 식의 `.value()` 2, 마지막 table constraint 뒤 쉼표 1, RESTORE MOVE 변수 1 / csharp 43: 마지막 줄이 개행 없는 전처리 지시문 40, `#if/#else` 분기 3 |
  | unsupported | 39 | tsql 확장자로 route된 T-SQL 아닌 SQL(Oracle PL/SQL, client named parameter) |
  | source damage | 10 | csharp merge conflict 표지 5, tsql scratch 문구·잘못 붙은 줄 4, xml 앞의 경로 줄 1 |
  | `UNDISPOSITIONED` | 0 | 이전 2개(subquery union 분기의 TOP … ORDER BY)는 r6으로 ERROR가 없어졌다 |

* 이전에 source damage로 처분한 tsql 1개(파일 첫 줄의 한 글자 단어)는 r6에서 batch 첫 문장의 EXEC 생략 모듈 호출로 읽혀 ERROR가 없다. EXECUTE 문서상 구문으로는 유효한 호출이다.
* 남은 tsql gap 파일의 첫 오류 위치를 masked 문맥으로 하나씩 확인해 gap id를 붙였다. 하나(마지막 table constraint 뒤 쉼표)는 같은 원인에서 이어진 복구 오류가 3개 더 있다.
* 판정: route 있는 모든 파일이 실행·집계되었고, kit 결함 0, `NOT_RUN` 0, ERROR 파일 모두 처분되어 `UNDISPOSITIONED`가 없다. S08-A16 행은 **통과**다. 비율 임계값은 없다. 이 행은 78칸과 별도다.
* S07 `private-corpus-r1`로 이 실행을 다시 검증했다(`b48c255`의 CLI): `REPLAYED_RAW`/PASS, evidence 유효, 1.5초(로컬 `private-replay-receipt-s08-r6b.json`).
* 중간 후보의 실행: `d2a08b1`(완료, 492초, ERROR 119, 직전 후보 보고에 사용), `dbe9e21`(완료, 816초, ERROR 124), `17130e8`·`e1c9b76`(후보가 바뀌어 중단, 이 작업이 만든 process를 부모 관계로 확인해 종료, 부분 출력 보존)은 최종 처분에 쓰지 않았다.

**이전 실행(`0cdf3fd`, r5)**: wall 711초, `has_error` 230(csharp 48, tsql 181, xml 1), 처분 grammar gap 178·unsupported 39·source damage 11·`UNDISPOSITIONED` 2. S05 실행(`81a0538`)과 모든 파일 결과가 같았고 S07 replay도 PASS였다.

## PR #71 hosted CI에서 고친 결함

* run 37179971362(head `70a59fb`): Linux·macOS foundation의 `TestModuleProxyConsumer`가 끝난 뒤 `t.TempDir` 정리에서 실패했다. Go가 module cache 디렉터리를 읽기 전용으로 만들기 때문이다. consumer build에 `-modcacherw`를 더했다(`2385188`). 단언은 그대로다.
* run 37180620293(head `2385188`): 78칸은 모두 kit 축 PASS, 비교 FAIL 0이었으나 `n461-svc` 행이 세 OS에서 kit 축 FAIL이었다. SVC composite 9 사례가 Windows와 Linux·macOS 사이 `Composite`에서 달랐고, 다른 필드는 composite의 producer identity(platform의 native build, `tsgk-native-build/r1`)뿐이었다. source·policy identity는 세 OS가 같아 입력 bytes 차이가 아니다. 계약상 build identity는 비교하지 않는 host 관측인데 비교기가 composite를 원문으로 비교했다(kit 결함). composite 비교에서 producer identity만 빼고, 대신 host마다 composite의 producer·policy identity를 run 값과 대조한다(`TestQualifyCompositeHostIdentity`; 원문 비교와 대조 제거 mutant를 각각 잡는다). 같은 artifact를 고친 CLI로 다시 집계하면 `mechanism_gate` PASS, 78칸 결과와 totals는 원래와 같고 `n461-svc`는 kit 축 PASS·검사 BLOCKED라 `INCOMPLETE`다. 로컬 세 host 경로 점검은 복제본이 같은 build identity를 써서 이 결함을 드러내지 못했다.

## 분리 context 리뷰와 처분

**r1**(general-purpose subagent, `3fa7ed3..6e7ca58`, EXECUTED `go vet`·qualify 시험 + STATIC): BLOCKER 0, MATERIAL 2, MINOR 6, NOTE 1. 처분은 `ddf45bb`(code)와 `475ad4c`(문서)다.

* M1: 어느 kind도 덮지 않는 등록 기대값(edit 뒤 `NO_ERROR` step, 행이 없는 N461 사례, 추가 역할의 support 사례)이 어느 축에도 없었다. 칸에 `registered_checks`(모든 등록 기대값과 query 기대값, SVC 관측 판정)를 두어 요구 축에 접고, 추가 역할 행은 kit 축(`mechanism`)과 검사(`registered_checks`)를 따로 가진다(`TestQualifyRegisteredChecks`).
* M2: 추가 역할 행이 MISSING·INCOMPLETE여도 `SUPPORTED`·exit 0이 가능했다. 실행할 역할 행의 set이 없으면 완결성 FAIL이고, `SUPPORTED`는 gate와 실행 역할 행 PASS를 요구한다(`TestQualifyBaseline`).
* m3: 거부된 host가 cohort 기준이 되어 정상 host를 `COHORT_MISMATCH`로 만들었다. 자기 검사를 통과한 첫 host를 기준으로 한다(`TestQualifyEligibility`). m4: host 순회를 platform 순서로 고정했다. m5: 등록 대조에 선언 mapping, point, 동적 SQL 기대값, 사례 encoding을 더하고 inventory가 그 값을 가진다. m6: 요구 행 없는 route, 범위 밖 step, 요구 행을 덮는 support 사례, route 안 detector를 inventory parser가 거부하고, 생성기는 열 수가 틀린 REQ 행에서 실패하며 178행·826 의무를 고정한다. m7: 한도 초과로 kit 축이 BLOCKED인 칸은 `INCOMPLETE`이고, host 디렉터리 없음·link는 호출 전체 오류라고 문서에 적었으며, host당 record 수 한도를 강제한다(`RECORDS_LIMIT`). platform 표 상태를 `AVAILABLE`로 고쳤다. m8: set이 있으면 그 파일과 profile을 처음부터 그 칸의 근거로 표시해 원인이 다른 completeness 실패로 번지지 않는다.
* n9: set과 실행 identity는 hash가 아니라 같은 artifact 안에 있다는 것으로 묶인다(한계로 기록). PR의 후보 commit은 merge commit(`github.sha`)이다. 실패 job만 재실행하면 attempt가 섞여 완결성이 실패하므로 run 전체를 다시 실행한다([validation](../validation/validation.md)).

**r2**(같은 reviewer, `6e7ca58..0edaa0e`, EXECUTED `go vet`·kit·CLI·foundation 시험 + STATIC): r1 항목이 모두 해소되었거나 문서로 정리되었고 새 BLOCKER·MATERIAL은 없다. MINOR 2, NOTE 5가 나왔다. 처분은 다음 commit이다.

* r2-1: exit·지원 claim 문구(CLI 계약, README, 결과 explanation)와 결과 필드 목록이 새 동작(실행 역할 행 PASS와 gate 필요, `registered_checks`·`check_failures`, 역할 행 `mechanism`)을 따라오지 않았다. 네 곳을 고쳤다.
* r2-2: MISSING 역할 행이 `registered_checks: PASS`를, MISSING 칸이 `check_failures: null`을 냈다. MISSING 행의 검사는 빈 값, 칸 목록은 빈 배열이다.
* n1: 사실이 0개인 동적 SQL fixture가 생겨도 inventory가 `[]`를 쓰게 했다. n4: `expect_status=RESOURCE_LIMIT` 사례가 요구 행을 덮으면 inventory가 거부한다. n6: CI 로그(`gate.ps1`)가 칸의 `check_failures`와 역할 행의 kit 축·검사를 출력한다.
* n2(선언 항목 필터의 빈 node 값), n3(cohort 기준이 다수결이 아님, 결과는 fail-closed)는 기록만 한다. n5: `n461-svc`의 SVC 음성 사례 8개에 기대 결과를 등록하는 일은 후속 사례 작업이며 추적 Issue는 orchestrator가 정한다.

**r3**(같은 reviewer, `0edaa0e..0eb2c36`, EXECUTED `go vet`·kit·CLI·foundation 시험 + STATIC): r2-1, r2-2, n1, n4, n6이 해소되었고 `gate.ps1` 변경에 StrictMode·Linux 위험이 없다. 새 MINOR 1건(r3-1: MISSING 역할 행의 `mechanism`은 빈 값이 아니라 `NOT_ASSESSED`이며 gate는 빈 값인 행만 건너뛴다)은 문서를 고쳐 처분했다. 남은 BLOCKER·MATERIAL은 0이다.

## 후보 `0eb2c36`의 로컬 재실행과 중단

`0eb2c36`에서 ci-sim을 다시 실행했으나 host 메모리 부족으로 Claude Code가 배경 작업을 중단했다. 중단 시점에 route step은 26 route와 SVC를 마치고 windows 대용량 query 기록을 실행 중이었다. 남은 자식 process(route step `pwsh`, `tsgk oracle record`, native driver)는 이 작업이 만든 것임을 부모 관계로 확인한 뒤 종료했고, 지시대로 다시 시작하지 않았다. 그래서 `0eb2c36`의 완전한 ci-sim 결과는 없다.

* `9aa1500..0eb2c36` 사이에 route step 경로(`run-routes.ps1`, `run-identity.ps1`, workflow의 route step, driver, `src/internal`, incremental·oracle code, 등록 사례)는 바뀌지 않았다. 바뀐 것은 qualification 쪽(`kit.Qualify`, inventory, `gate.ps1`)과 시험·문서다.
* qualification 쪽 변경은 `9aa1500` ci-sim의 host 근거를 리뷰 r2 수정 code로 다시 집계하고 바뀐 `gate.ps1`로 출력해 확인했다(세 host 경로 점검과 같은 relabel 복제본 포함, gate PASS).
* 비공개 corpus 실행(`0cdf3fd`)과 그 뒤 후보 사이의 `src` 변경도 qualification 파일과 시험뿐이며 corpus 실행 경로(`tsgk corpus`·`incremental`, driver)는 같다.

## acceptance 연결

| ID | 근거 |
|---|---|
| S08-A01 | inventory 78칸(`TestQualificationInventory`), `TestQualifyBaseline`, 추가 역할 행 별도 |
| S08-A02 | 칸별 의무 목록과 `NOT_COVERED`, `TestQualifyBaseline`(INCOMPLETE는 PASS가 아님), mutant `not-covered-as-pass` |
| S08-A03 | 실행 identity·자격, CI qualification job(hosted 결과는 PR CI) |
| S08-A04 | `TestQualifyCompleteness` |
| S08-A05 | `TestQualifyCohort` |
| S08-A06·A07 | `TestQualifyComparison` |
| S08-A08 | `TestQualifyDetectorAndGap` |
| S08-A09 | `TestQualifyEligibility` |
| S08-A10 | `TestModuleProxyConsumer`, `TestGrammarUpdateWorkflow`, `TestQualifyCLI` |
| S08-A11 | `TestQualifyLimitsAndCancel`, `TestQualifyCLI` |
| S08-A12 | `TestQualifyMechanism`, mutant 29/29 |
| S08-A13 | 로컬 전체 회귀와 ci-sim(`f4890e7` 완료, `b48c255`는 route·oracle set 뒤 중단); 세 OS gate는 PR CI |
| S08-A14 | orchestrator 단계(PR·merge·post-merge·tracking) |
| S08-A15 | README·platform 계약·CHANGELOG(지원 claim BLOCKED, 발행·채택 없음) |
| S08-A16 | 비공개 corpus 절(r6 최종 후보 `b48c255` 실행으로 통과) |
| S08-A17 | `n461-svc` 세 host OWNED_FIXTURE, `n461-large` windows 전용·Linux·macOS `NOT_APPLICABLE`, 비공개 corpus는 로컬만 |

## Linux·macOS 정적 점검

* `run-identity.ps1`: native 명령 출력은 `@(...)`로 전부 받는다(잘린 pipeline 없음). 지정한 run 변수만 읽고 환경 전체를 기록하지 않는다. `exit 0`으로 끝난다. push event에서는 PR head가 비어 checkout을 쓴다.
* `gate.ps1`: native 명령을 부르지 않고 JSON만 읽으며 `exit 0`/`exit 1`로 끝난다.
* qualification job: native 실행·cgroup·root 전용 읽기가 없다. ubuntu의 `go build -o`는 접미사가 필요 없다. artifact 이름은 `s05-routes-<goos>-<run>-<attempt>`이고 pattern 다운로드는 artifact마다 하위 디렉터리를 만든다(v4 동작). host가 없으면 job이 그 host 없이 집계하므로 완결성 실패로 드러난다.
* `TestModuleProxyConsumer`: `git ls-files`와 module cache의 `golang.org/x/sys` download 파일이 필요하다. foundation job은 시험 전에 고정 module을 받으므로 세 OS 모두에 있다. 파일 URL은 Unix에서 `file:///…`다.
* 확인하지 못한 위험:
  * 세 host의 의미 요약이 실제로 같은지(같은 runtime·grammar·driver source지만 compiler가 다름). 차이가 나면 결과의 `differences`가 첫 위치를 준다.
  * Linux·macOS route artifact 크기와 업로드 시간(Windows 로컬 근거 225 MB, 대용량 제외 약 90 MB).
  * hosted ubuntu에서 세 host 근거 집계 시간(로컬 12.5초, wall 360초).

## 남은 일과 한계

* 지원 claim은 BLOCKED다. 필수 의무 501개가 사례 없이 남았고(#70) 채택 route의 grammar gap 9건이 FAIL이다.
* T-SQL r6 뒤에도 grammar gap 5종(비공개 corpus 19개 파일)과 기록된 과잉 수용(r6에서 3종, r5부터 2종)이 남는다.
* 최종 후보 `b48c255`의 ci-sim은 메모리 부족으로 중단되어 그 후보의 Windows 칸 집계가 없다. 위 Windows 칸 절은 `f4890e7` 기준이다.
* r6은 로컬 Windows 검증만 있다. 세 OS에서 같은 parser.c가 재생성되는지(reference_match)와 native 결과는 hosted CI가 처음 확인한다.
* 세 OS 실행·비교, PR·merge·post-merge는 orchestrator가 한다.
* 사실 재현·동적 SQL claim은 S07과 같이 기록값을 쓴다(다시 계산하지 않음). 그 기대값과 선언 mapping은 inventory와 대조한다.
* `n461-svc` 행은 SVC 음성 사례 8개에 등록된 기대 결과가 없어 `INCOMPLETE`다. 기대 결과 등록은 후속 사례 작업이다.
* 실행 identity는 run이 스스로 쓴 주장이며 진위는 artifact 경로와 hash로만 보장한다(게시자 인증 없음).
* `go test -race`는 실행하지 않았다(CGO 없는 build).
* BrightScript maintained·historical, Cooklang audit 행은 `NOT_RUN`이다(bundle 미준비, 다운로드 없음).
