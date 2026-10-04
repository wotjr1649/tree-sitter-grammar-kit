# Session 08 — Real-world & Cross-platform Qualification 결과

Campaign `TSGK-C1-20260929-R1`, 추적 `TSGK-C1-S08`, Issue #10, Milestone 9, branch `session/08-real-world-qualification`(base main `3fa7ed3`). 이 보고서는 아래에 적은 revision과 로컬 관측만 설명한다. PR·세 OS CI·merge·post-merge 결과와 hosted 78칸 행렬은 이 문서 작성 시점에 없으며 그 단계의 receipt가 소유한다.

## 범위 결정

orchestrator 조정(2026-10-04, 선택지 A)에 따라 S08은 qualification 장치와 현재 후보의 78칸 행렬을 두 축(kit mechanism, 요구 coverage)으로 내고 새 언어 사례·기대값은 작성하지 않는다. 지원 claim은 하지 않는다. 실행하지 않은 필수 feature를 다른 이름으로 바꾸지 않는다. 사례가 없는 필수 의무는 S08 밖의 후속 작업 [#70](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/70)이 추적한다.

## 구현 범위

* **qualification inventory `tsgk-qualification-inventory/r1`**([`qualification-c1.json`](../../src/contracts/qualification-c1.json)): campaign 정의의 26 route × 3 platform(78칸), [feature disposition](../validation/language-feature-disposition.md)의 REQ 178행 × 필수 case kind(826 의무), route마다 `run-routes.ps1`이 실행하는 S06 기록 workload(grammar 파일, query source, fact pack, 등록 사례의 입력·edit·기대값·query 기대값)와 추가 역할 행(NET461 SVC·대용량, owned native fixture, BrightScript maintained·historical, Cooklang audit)을 담는다. `TestQualificationInventory`가 등록부와 사례에서 다시 만들어 bytes로 대조한다.
* **coverage 규칙 `tsgk-coverage-rule/r1`**: 사례가 그 route의 REQ 행을 `features`로 적을 때 P(`NO_ERROR`+구조), N(`ERROR`), R(`ERROR`+보존 구조), E(edit), Q(capture 기대값이 있는 query 사례)를 덮는다. W(허가된 실사용 sample)는 등록 생산자가 없어 덮이지 않는다. inventory parser가 규칙을 강제한다.
* **`kit.Qualify` / `tsgk qualify`**: 계약은 [identity/evidence](../specs/identity-and-evidence.md)·[CLI/profile](../specs/cli-and-profile.md)·[공개 API](../specs/public-go-api.md)의 `S08 구현`이다. host마다 실행 identity(`tsgk-run-identity/r1`)로 cohort·platform·NEW_RUN·후보 commit 자격을 확인하고, S06 기록 set을 공유 set 규칙·등록 대조·S07 `oracle-set-r1` 사례 gate로 검증한 뒤 kit 축과 요구 축을 계산한다. route마다 세 host의 사례별 의미 요약(tree digest·node 수·`has_error`, query capture stream, API 관측, 비교·판정)을 정확히 비교하고 host 관측(시간·메모리·build identity)은 따로 보존한다. 결과 `tsgk-qualification-result/r1`은 칸, 추가 역할 행, 비교, `mechanism_gate`, `support_claim`을 담는다. process·network를 쓰지 않는다.
* **CI**: `native routes (<os>)`가 실행 identity를 쓰고 기록 set·workload profile을 artifact로 올린다. 새 `qualification (ubuntu-24.04)` job이 같은 run·attempt의 세 artifact로 `tsgk qualify`를 실행하고 `mechanism_gate`로 성공을 정한다([validation](../validation/validation.md)). heavy job은 늘지 않는다.
* **공개 사용 검증(A10)**: `TestModuleProxyConsumer`(추적 파일로 만든 module zip을 파일 module proxy로만 받아 별도 module이 version으로 build, CLI와 같은 bytes), `TestGrammarUpdateWorkflow`(CLI만으로 기준 identity → 후보 verify FAIL → schema diff 검토 항목, 입력 불변), `TestQualifyCLI`(PATH 없이 같은 bytes, 신뢰 문서·출력 위치 guard, no-clobber).
* README의 구현 기능 표와 지원 상태(BLOCKED), CHANGELOG, platform 계약 표를 실제 상태로 고쳤다. tag·Release·package·Go 소비자 채택은 하지 않았다.

제외: 새 언어 사례·기대값, grammar·upstream 수정, 비공개 corpus의 hosted 전송, BrightScript bundle 다운로드와 replay.

## 로컬 검사(Windows)

* `gofmt`, `go vet ./src/...`, `go test ./src/kit ./src/cmd/tsgk ./src/internal/foundation`: 통과(`CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOFLAGS=-mod=readonly`).
* 새 시험: `TestQualifyBaseline`(A01·A02), `TestQualifyCompleteness`(A04), `TestQualifyCohort`(A05), `TestQualifyComparison`(A06·A07), `TestQualifyDetectorAndGap`(A08), `TestQualifyEligibility`(A09), `TestQualifyMechanism`(A12), `TestQualifyLimitsAndCancel`(A11), `TestQualificationInventoryGuards`, `TestQualificationInventory`, `TestQualifyCLI`, `TestModuleProxyConsumer`, `TestGrammarUpdateWorkflow`.
* **targeted mutant**: `0cdf3fd`에서 29/29, 리뷰 r1 수정 뒤 33/33 검출(`.work/session-08/mutants.py`, 로컬 `mutants-0cdf3fd.json`·`mutants-r1.json`). r1에서 더한 mutant는 등록 검사 무시, 거부된 host를 cohort 기준으로 사용, 역할 행 MISSING의 완결성 무시, detector 결과 무시다. `SUPPORTED`에 gate를 요구하는 줄을 지우는 mutant는 동등 mutant라 뺐다(모든 칸과 실행 역할 행이 PASS이면 gate도 PASS다). 29개 목록: 실패 route 칸 제외, 다른 host로 빈 칸 채우기, dialect 병합(set route 미검사, 두 route가 set 공유), 역사 relabel(evidence mode·후보 commit 미검사), cohort·architecture(identity·set) 미검사, 등록 대조·workload 결속 생략, 비교에서 capture·digest·API 제외, host 관측 비교, 칸이 비교 실패 무시, 사례 없음을 PASS로, detector의 요구 행 덮기, detector로 gap 숨김, kit claim 무시, 기록 query claim 신뢰, record 수 미검사, 등록되지 않은 파일 허용, 중복 host 허용, 빈 칸 완결성 무시, gate가 kit 축 무시, coverage 규칙·칸 수 미검사, 모든 칸 PASS 없이 SUPPORTED. 모두 의도한 시험이 실패했다(compile 실패로 대신하지 않음).
* **CI 방식 재현(ci-sim, `9aa1500`)**: workflow의 `26 route native build와 등록 사례·query 기록 실행` step과 `78칸 qualification 집계` step을 그대로 꺼내 새 `pwsh -NoProfile`에서 job env와 RUNNER_TEMP 형태 디렉터리로 실행했다. route step은 exit 0(755초, routes 27, failures 0)이고 실행 identity를 썼다. artifact 경로를 그대로 복사해 qualification step을 실행했다. Windows 26칸은 모두 kit 축 PASS다. 리뷰 r1 수정 뒤 같은 근거를 다시 집계하면(`ddf45bb`) 등록 대조(선언 mapping·point·동적 SQL 기대값 포함)도 그대로 맞고, 추가 역할 `n461-large`는 PASS, `n461-svc`는 kit 축 PASS·등록 검사 BLOCKED라 `INCOMPLETE`다. SVC 음성 사례 8개(inline 해석 불가·VB·진단)가 S05 계약대로 `BLOCKED`로 끝나지만 등록된 기대 결과가 없어 통과로 셀 수 없기 때문이다. Linux·macOS host가 없어 52칸은 `MISSING`이고 완결성·`mechanism_gate`가 FAIL이며 step이 실패했다(한 host만 있는 로컬의 예상 결과). Windows에서 실행 파일 이름에 `.exe`를 붙인 것만 job과 다르다(job은 ubuntu).
* **세 host 경로 점검**: Windows host 근거를 실행 identity와 manifest platform만 바꾼 두 복제본으로 `tsgk qualify`를 돌리면 완결성 PASS, kit 축 78/78, 비교 FAIL 0, `mechanism_gate` PASS, 405 MB를 12.5초에 읽었다. 이것은 집계 경로와 한도의 점검이며 교차 OS 근거가 아니다.

## Windows 칸(ci-sim, 후보 `9aa1500`)

| 상태 | 칸 | route |
|---|---|---|
| FAIL(요구 FAIL, kit PASS) | 4 | csharp, typescript, tsx, swift |
| INCOMPLETE(사례 없는 의무, kit PASS) | 22 | 나머지 22 route |

의무 826개 중 PASS 308, FAIL 9, 사례 없음 509다. 등록 검사(모든 등록 기대값)는 22 route가 PASS, 4 route가 FAIL이다(csharp gap 2, typescript gap 4, tsx gap 2, swift gap 2 사례). kind별 사례 없음은 P 66, N 124, R 130, E 47, Q 108, W 34다. 요구 FAIL 9건은 넘겨받은 grammar gap이다: csharp B01·B03·V08·V09의 P(contextual keyword 식별자 등), typescript V40·V50의 P(TypeScript 5.0–5.3 구문), tsx B01·B02의 N과 swift V59의 N(lenient acceptance: 오류여야 할 입력을 받아들임). API claim FAIL은 26 route 22건, SVC 1건이다(S06과 같은 runtime field lookup 차이). 세 host 비교는 hosted CI에서 처음 실행된다.

**#70과의 차이**: #70은 R 미덮음을 97로 적었다. kit 규칙은 R에 보존 구조 기대(`ERROR`+`contains`)를 요구하므로 `contains`가 빈 recovery 사례 26개는 N·E만 덮고 R은 덮지 않는다. 그래서 행렬의 R 미덮음은 130, 합계는 509다.

## 비공개 corpus(A16, Windows 로컬, 개수만)

`NET461-PHASE2-LOCAL-r1`을 clean 후보 `0cdf3fd`에서 `run-corpus.ps1`(`private-corpus-local`, 실행 wall 7200초)로 실행했다. 경로·이름·내용이 담긴 기록(실행 결과, ERROR 진단, 파일별 처분)은 추적하지 않는 로컬 artifacts에만 있다. 이 절은 개수와 판정만 적는다.

* 실행: wall 711초, csharp 6624 파일(14 batch), tsql 6150(13), xml 5163(11), svc 20(native process 없음). 고정 runtime `659cda7c`, 기존 MSYS2 GCC(`75e87953…`). 다운로드 없음.
* inventory 21451 record: route 있는 파일 17957, `PRESENCE_ONLY` 133, route 없음 3494(따로 셈).
* `execution_status`: COMPLETED 17957, CANCELLED·RESOURCE_LIMIT·FAILED 0, `NOT_RUN` 0, 실행하지 않은 `assessment=BLOCKED` 0. 분류할 한도·환경·kit 결함 파일이 없고 kit 결함 0이다.
* `has_error`(COMPLETED이면서 ERROR) 230: csharp 48, tsql 181, xml 1. S05 실행(`81a0538`)과 비교하면 17957 파일의 상태·판정·code·`has_error`·node 수·digest·오류 수·형식이 모두 같다.
* ERROR 230개 처분(로컬 근거로 진단, 비밀 값은 인용·전송하지 않음):

  | 처분 | 파일 | 내용 |
  |---|---|---|
  | grammar gap | 178 | tsql 135: Unicode(한글) regular identifier 54, 테이블 constraint·option 형태 19, scalar subquery 피연산자 17, CTE 앞 `;`·빈 문장 13, `DEFAULT … WITH VALUES` 13, `+=` 4, FROM 없는 SELECT의 WHERE 4, EXEC 없는 첫 procedure 호출 3, `.value()` 2, BACKUP/RESTORE 2, block comment 위치 2, index option 1, CREATE DATABASE 1 / csharp 43: 마지막 줄이 개행 없는 전처리 지시문 40, `#if/#else` 분기 3 |
  | unsupported | 39 | tsql 확장자로 route된 T-SQL 아닌 SQL(Oracle PL/SQL, client named parameter) |
  | source damage | 11 | csharp merge conflict 표지 5, tsql scratch 문구·잘못 붙은 줄 5, xml 앞의 경로 줄 1 |
  | `UNDISPOSITIONED` | 2 | tsql: TOP이 있는 derived table 안 UNION ALL 분기의 ORDER BY. SQL Server [ORDER BY 계약](https://learn.microsoft.com/en-us/sql/t-sql/queries/select-order-by-clause-transact-sql?view=sql-server-ver17)만으로 유효성을 정하지 못해 사용자 처분을 기다린다 |

* C# 40개의 원인은 소유 최소 재현으로 확인했다. `class C { }`와 개행 뒤 `#pragma warning restore 1591`을 파일 끝 개행 없이 두면 root `has_error`가 true인데 cursor tree에는 ERROR·MISSING node가 없다(빠진 지시문 끝이 숨은 node다). 끝 개행이 있으면 오류가 없다. `#region`/`#endregion`을 개행 없이 끝내면 보이는 ERROR가 생긴다. kit는 runtime의 `has_error`와 cursor에 보이는 node만 보고하므로 이런 파일의 ERROR 목록은 비어 있다.
* 판정: route 있는 모든 파일이 실행·집계되었고 kit 결함 0이지만 `UNDISPOSITIONED` 2개가 남아 S08-A16 행은 **통과가 아니다**. 사용자 처분이 기록되면 다시 센다. 비율 임계값은 없다. 이 행은 78칸과 별도다.

S07 `private-corpus-r1`로 이 실행을 다시 검증했다(`ddf45bb` 이전 `6e7ca58`의 CLI): `REPLAYED_RAW`/PASS, evidence 유효, 0.9초(로컬 `private-replay-receipt-s08.json`).

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
| S08-A13 | 로컬 전체 회귀와 ci-sim; 세 OS gate는 PR CI |
| S08-A14 | orchestrator 단계(PR·merge·post-merge·tracking) |
| S08-A15 | README·platform 계약·CHANGELOG(지원 claim BLOCKED, 발행·채택 없음) |
| S08-A16 | 비공개 corpus 절 |
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

* 지원 claim은 BLOCKED다. 필수 의무 509개가 사례 없이 남았고(#70) 채택 route의 grammar gap 9건이 FAIL이다.
* 세 OS 실행·비교, PR·merge·post-merge는 orchestrator가 한다.
* 사실 재현·동적 SQL claim은 S07과 같이 기록값을 쓴다(다시 계산하지 않음). 그 기대값과 선언 mapping은 inventory와 대조한다.
* `n461-svc` 행은 SVC 음성 사례 8개에 등록된 기대 결과가 없어 `INCOMPLETE`다. 기대 결과 등록은 후속 사례 작업이다.
* 실행 identity는 run이 스스로 쓴 주장이며 진위는 artifact 경로와 hash로만 보장한다(게시자 인증 없음).
* `go test -race`는 실행하지 않았다(CGO 없는 build).
* BrightScript maintained·historical, Cooklang audit 행은 `NOT_RUN`이다(bundle 미준비, 다운로드 없음).
