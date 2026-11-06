# Campaign 01 준비 계약 채택 기록

추적: [PREPARE #20](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/20), Parent [#1](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/1). 공급 명세 `TSGK-C1-20260929-R1`, 기준일 2026-09-29. 이 보고서는 준비 변경의 내용과 미해결 입력을 설명하며 미래 merge/CI 성공을 기록하지 않는다. 실제 통합·tracking readback·최종 readiness는 Issue #20과 해당 PR의 검증 receipt를 확인한다.

## 확인한 기반과 보존

착수 local/remote main은 `176da0d408194b843851dff9f43924e2a20a26b9`, tree는 `0ca898ae8195cd5b97d3d27c14e6bf69c7a7e6d6`이며 추적 파일 변경이 없었다. 기존 S00·r2·2026-09-28 prompt·handoff·reference·실패 근거를 보존한다. 공급 패키지 46개 파일의 size/SHA-256 및 역사적 prompt 11개가 기록된 hash와 일치했다. 패키지 확인은 execution/feature 성공 증거가 아니다.

기존 공개 문서의 API 비공개 기본안, S05 CLI adapter 후 S06 별도 driver, 제한된 실사용 grammar 위주 qualification을 승인된 D4/D6/D10/D11에 맞게 정정했다. 기존 src/root module/CGO0/PowerShell7/비추적 자료/일반 작업 진입/PR13·15·19의 검증·조건부 cleanup 계약을 유지한다.

## D1~D13 traceability

| 결정 | 채택 소유 문서 | 검증 책임 |
|---|---|---|
| D1 | [architecture](../design/architecture.md), [trust](../specs/trust-and-execution.md) | CGO0/source-boundary, offline 무실행, S01~S08 회귀 |
| D2 | [scope](../specs/scope.md), README | 독립 CLI/API 사용, S08 consumer workflow |
| D3 | scope, [tree/protocol](../specs/tree-and-adapter-protocol.md) | consumer runtime/변환/채택 분리, 같은 edit/comparator |
| D4 | [public API](../specs/public-go-api.md), architecture | S01~S03 CLI/API equivalence와 외부 module |
| D5 | [CLI/profile](../specs/cli-and-profile.md), identity | 명령별 parser.c 필요성, source/grammar/closure 구분 |
| D6 | [feature 범위](../validation/language-feature-scope.md), [기계 정의](../../src/contracts/campaign-01.json) | 정확한 26 route, 네 fixture 역할 |
| D7 | feature 범위, [workload](../validation/workload-matrix.md) | 공식 syntax fact·검토된 구조·선택 보조 parser |
| D8 | feature 범위, [campaign 계약](#campaign-specific-acceptance-and-execution) | S01 전 scope/feature 채택; 상세 bytes는 담당 단계 |
| D9 | trust, [provenance](../provenance/upstream-sources.md) | 기존 공통 도구 우선, SDK/DB 일괄 설치 제외 |
| D10 | [platform](../specs/platform-support.md), workload | 26×3=78, 실제 세 host core/API/native |
| D11 | [roadmap](../roadmap.md), tree/protocol, Session #6~#9 | S01 E0/S04 runner/S05 최소 driver/S06 확장/S07 replay |
| D12 | validation, trust | 유한 예산·소모량, local heavy1/campaign CI3, 진단된 infra retry1 |
| D13 | roadmap, validation | PREPARE와 master 승인 분리, 실제 review/CI/merge/readback |

## 채택값·관측·남은 입력

2026-10-02 사용자 결정으로 6 route(csharp·typescript·tsx·swift·postgresql-sql·tsql)의 patched 후보를 채택했다. 근거와 후속 Session 의무는 [6-route 채택 절](../validation/source-feature-feasibility.md#prepare-06-6-route-최종-채택-2026-10-02)과 [등록부](../../src/contracts/language-sources.json)를 따른다. 아래 checkpoint 기록은 역사적 관측이다.

이 절의 PR #21/#22 초기 checkpoint 관측과 수치 예산은 역사적 기록이다. 후속 승인·실제 A/B r2·SQL30·PG baseline/noopt 결과와 현재 필수 격차는 [P05 관측표](../validation/source-feature-feasibility.md#prepare-06의-실제-ab-및-sql-결과)와 [source closure](../validation/p05-source-closure.md)를 따른다. 새 [NET461 업무 계약](../validation/net461-workload.md)은 기존 scope에 연결한 별도 workload/format이며 전체 corpus·제품·78셀은 NOT_RUN이다.

후속 PR #43~#45의 wiring/export 보완을 통합한 main `ea262f5a8568118fbc7d3346d140c777e496573b`에서 승인 exact3 시험을 실제 수행했다. [새 관측](../validation/source-feature-feasibility.md#승인된-exact3의-실제-관측과-기대값-충돌)은 C# 생성 conflict, PG의 이번 OOM과 baseline 새4개, MSSQL의 두 producer60개·구조/negative/recovery 결과를 구분한다. T-SQL bracket negative 기대의 공식 구문 충돌도 별도 채택 전까지 남는다. 기존 A/B·exact3 효과 및 지속 예산을 다시 묻지 않으며 추가 source 변경·개별 한도·기대값 정정과 최종 채택만 결정 대상으로 관리한다. 이 결과 때문에 P05는 미완료이며 #20은 OPEN, S01/MASTER는 미착수다.

공개 entry는 단일 root module의 `src/kit`이다. 첫 Inspect/Identity request/result/error·소유권·취소·한도를 설계했으며 구현과 external-consumer 실행은 S01에 속한다. E0/manifest r1은 mode provenance와 관측 전용 assessment를 명시한다. S05는 base64 bytes를 사용하는 bounded JSON transport와 실제 old-tree edit 경로를 소유하며 S06이 같은 producer를 확장한다.

26개 후보의 공개 commit과 실제 grammar subdir·generated 파일·scanner/shared file 목록을 [등록부](../../src/contracts/language-sources.json)에 기록했다. metadata 존재는 full source closure 검토나 언어 지원 성공이 아니다. Swift 후보의 고정 source에는 `parser.c`가 없어 generation 준비가 필요하다. T-SQL과 PostgreSQL은 서로 다른 dialect 후보로 유지한다.

공식 stable 상한 대조와 사용자가 채택한 26개 숫자/모드는 [scope 표](../validation/language-feature-scope.md)에 있다. PR #21 checkpoint 당시 feature disposition은 26개 route placeholder와 C#14 일부 항목만 있어 P04가 남았다. 후속 PREPARE의 [feature disposition](../validation/language-feature-disposition.md)은 26개 기본 구문 계열·중간 version delta·legacy 보존·syntax/semantic/runtime/extension 구분·구조 목표·case 종류·담당과 기한을 등록한다. 독립 리뷰와 이 정확한 내용의 사용자 채택은 별도 실제 receipt로 확인한다. candidate node/field mapping과 상세 fixture/expected tree는 담당 S03/S05/S06/S08 산출물이며 지금 생성하거나 parser 출력으로 golden을 채택하지 않았다.

초기 checkpoint에서 확인한 필수 위험은 C# 후보의 contextual identifier/file-based directives 문서상 격차, T-SQL 고정 grammar source의 SELECT/EXEC 중심 경로와 DDL/다른 DML/CTE TODO, TS/TSX import-defer 및 dependency closure, PG19 기반 후보의 채택 9.6~18 legacy 지원 미확인이었다. [P05 관측표](../validation/source-feature-feasibility.md)는 26개 행을 feature ID에 연결하고 upstream 선언·정적 관측·지원 미확인·실제 재현 실패를 분리한다. 그 초기 checkpoint의 upstream 재현 실패 receipt는0개이며 native는 미실행이었다. 이후 실제 run을 당시 기록에 소급 적용하지 않으며 generic SQL/smoke로 격차를 대체하지 않는다. upstream 수정·다른 grammar·scope 축소·미해결 gap을 안고 S01 진행하는 결정은 자동 수행하지 않는다.

후속 착수 시 PR [#21](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/21)은 MERGED, main은 `0406b7c9c53e062abab73682b1be80baea68372b`, tree는 `9c68da29a02273e7eb0e9ba7299f1f8c85fe92c3`로 확인했다. 이전 immutable receipt·소모 ledger·46개 공급 파일·14개 역사적 identity가 일치했다. 재개는 Issue #20의 `campaign/01-20260929-prepare-p04-p05`에서 수행한다. 이전 checkpoint를 덮어쓰지 않고 새 receipt가 이전 receipt와 변경 전 operational manifest를 연결한다. 새 transaction의 실제 merge/CI/readback은 그 후속 receipt가 소유한다.

현재 요청에서 kit Git transaction과 지정 tracking 쓰기를 승인했고, 후속 답변으로 기존 CLI/GCC/runtime을 사용한 owned probe와 유한 예산을 승인했다. PREPARE는 승인 후 누적 4시간, 저장 2 GiB, CI 120 job-minutes, 새 다운로드 0 bytes, 유료 KRW0이다. campaign envelope는 64시간/CI1,440분/native20,000회/다운로드2 GiB/보관20 GiB/KRW0, 각 Session은 8시간/CI180분/native2,500회/2.5 GiB다. 세부 연산의 null 한도는 담당 효과 전에 채택해야 하며 이 숫자 승인으로 master 실행 권한이 생기지 않는다.

## 실행한 준비 probe

Windows amd64에서 source closure 83파일/892,311 bytes의 고정 runtime과 자체 scannerless/stateful grammar JSON을 새 scratch로 준비했다. CLI0.27.0으로 ABI15 C를 생성하고 GCC16.2.0 `-Wall -Wextra -Werror`로 각각 build했다. 각 fixture에서 실제 old-tree edit와 3개 중간 상태의 fresh tree를 비교했고 stateful의 malformed/repair, scannerless의 NUL/CRLF 원본 길이·row를 확인했다. 공백·한글이 있는 root에서 generate/build/실행이 성공했다. 직접 실행 process의 종료와 stdout/stderr 완결을 기록했다. 악성 descendant의 격리·S04 감독·S05의 완전한 ordered-tree/protocol 검증은 이 probe의 주장이 아니다.

첫 compiler 시도는 `unicode/umachine.h` include 경로 누락으로 실패했다. 고정 runtime의 Makefile을 대조해 `-I runtime/lib/src`를 추가한 새 build가 통과했고 실패 로그도 보존했다. generator2/build3/native2의 총 7회이며 기존 한도를 늘리거나 실패를 지우지 않았다. 새 다운로드·설치·유료 효과는 없었다. 26개 upstream native 실행과 Linux/macOS native probe는 미실행이며 이 결과가 제품 단계의 성공이나 78칸을 대신하지 않는다.

## Readiness의 부정 상태 검토

| 주입/누락 상태 | 허용 판정과 재개 조건 |
|---|---|
| template receipt 또는 null approval | NOT_READY; 실제 관측·명시 승인 필요 |
| 26 route 중 하나 누락/중복, 미승인 범위 | P04 미충족; 정확한 집합과 scope 채택 필요 |
| hash가 다른 prompt를 같은 package로 사용 | P02 미충족; 원본 보존 및 별도 revision/의미 리뷰 필요 |
| Issue/Milestone body가 intended readback과 다름 | P12 미충족; 동시 변경을 보존하며 재정합화 |
| review/세 OS CI/정확한 attempt가 누락됨 | P09/P10 미충족; 같은 이름의 과거 PASS로 대체 금지 |
| candidate의 base가 이동하거나 다른 tree가 통합됨 | P10/P11 재평가; actual combined candidate 검증 |
| 필수 native/latest syntax 외부 격차가 남음 | BLOCKED_EXTERNAL; 별도 remediation/disposition 필요 |
| 계약만 통합되고 입력이 남음 | ADOPTED_PENDING_INPUTS; S01 branch/PR 생성 금지 |
| 준비가 모두 검증됐으나 master 지시가 없음 | PREPARATION_READY는 준비 판정만; 별도 live 실행 지시 필요 |

이 표는 문서 의미의 negative review 기준이다. 자동 readiness validator를 실행했다고 주장하지 않는다. 기계 검사는 추적된 route/platform/session/acceptance와 source candidate 정의를 검증하고 product 실행 결과를 만들지 않는다.

## Campaign-specific acceptance and execution

이 절은 검증 계약에서 옮겨 온 Campaign 01 PREPARE acceptance와 Session 실행 규칙을 소유한다. 현재 요청이나 적용되는 권위 있는 배정이 그 campaign 범위를 명시적으로 선택할 때만 적용한다. 기록된 승인은 원래 효과와 조건에 묶이며 이 문서는 새 권한을 주지 않는다. 위의 역사적 관측은 원래 checkpoint에 묶인다. 일반 작업은 [검증·병합 계약](../validation/validation.md)을 따른다.

### Campaign 01 준비와 후속 단계

PREPARE는 [준비 보고서](campaign-01-2026-09-29-preparation.md)의 D1~D13 소유와 [26 route/feature 범위](../validation/language-feature-scope.md)를 채택한다. 공급 prompt와 역사적 S00/r2는 서로 다른 revision으로 보존한다. local hash/receipt는 권한이나 성공을 만들어내지 않으며 CI와 공개 API는 local prompt 없이 동작해야 한다.

아래 표가 공개 PREPARE acceptance의 소유자다. 로컬 prompt나 template은 이 정의를 대신하지 않는다. gate status는 관측 receipt에 기록하며 테스트가 ID나 행을 확인한 것만으로 PASS가 되지 않는다.

| ID | PASS에 필요한 근거 | 필수 반례/미충족 판정 |
|---|---|---|
| P01 | 정확한 root/remote/worktree/base와 live 권한, 시작 diff/보존 inventory | 다른 root 또는 관련 없는 기존 변경을 덮어쓰지 않음 |
| P02 | 공급 파일 집합과 역사적 원본의 recorded size/hash 대조 | 누락/변경 prompt를 자체 rehash로 승인하지 않음 |
| P03 | 준비 보고서의 D1~D13 owner와 Session acceptance 정합화 | 폐기한 S05 CLI-only/S06 별도 driver/S08 축소/API 비공개 계약이 활성 상태로 남지 않음 |
| P04 | 26 route의 version/dialect/mode와 완전한 feature inventory 명시 채택 | 빈 목록/global latest/미승인 하한/누락 mode/미완료 feature는 미충족 |
| P05 | 26 source/tool feasibility 행의 관측·unknown·gap과 primary 근거 | repo/README 존재만으로 full support를 주장하지 않음 |
| P06 | 정확한 준비 효과/유한 한도 승인과 선택 probe의 command/input/tool/result | version 조회를 build로 표시하거나 denied effect를 우회하지 않음 |
| P07 | 공개 API·root module·src 경계·ordinary 진입 계약과 source/link 검사 | tracked nested module 또는 ignored prompt에 의존하는 canonical 계약 금지 |
| P08 | 현재 후보의 영향별 foundation/format/test/vet/build/dependency 검사 | 과거 S00 CI 승계/zero tests/미실행을 PASS로 표시하지 않음 |
| P09 | 정확한 diff/head의 분리 context 의미 리뷰와 finding 처분 | open BLOCKER/MATERIAL, self-review 또는 same-account comment를 formal approval로 대체 불가 |
| P10 | 실제 PR의 base/head/synthetic checkout/run/attempt/job와 세 OS 필수 CI | wrong SHA/attempt, skipped/missing job은 NOT_VERIFIED |
| P11 | 허용된 actual merge commit/tree와 main post-merge 검증 | 예정 SHA 또는 PR CI만으로 actual merge 검증 완료 처리 불가 |
| P12 | Parent #1/Issue #3~#10/Milestone 2~9의 intended body/state readback | 동시 사용자 변경 보존, 부분 API 성공이면 pending 유지 |
| P13 | 실제 operational manifest/immutable receipt/승인 budget/hash/tracking의 일치 | template/null approval/변경된 prompt는 S01을 활성화하지 않음 |
| P14 | 한국어 handoff의 실제 판정·미실행·한도·정확한 재개 entry | PREPARE에서 S01 또는 release를 시작하지 않음 |

source 등록부의 개발 검사는 현재 후보 metadata의 필수 값·상태·portable path와 공개 26-route 표의 전수성을 확인한다. feature 개발 검사는 [disposition](../validation/language-feature-disposition.md)의 route/ID/처분/case 종류와 [source 위험](../validation/source-feature-feasibility.md)의 feature 참조를 확인하고, 미정 scope·없는 feature·재현하지 않은 실패 등급을 넣은 negative control을 거절한다. 문법 목록의 의미상 완전성·사용자 채택·실제 upstream bytes/closure·지원 성공은 독립 리뷰/실제 답변/primary 조사/native receipt가 검증하며, 정적 행 검사가 이를 대신하지 않는다.

필수 입력이 남은 계약 통합은 ADOPTED_PENDING_INPUTS, 필수 외부 격차는 BLOCKED_EXTERNAL, 미처리 finding은 HOLD_FOR_CORRECTION이다. 검증·리뷰가 완료되고 통합만 남으면 READY_FOR_INTEGRATION이다. PREPARATION_READY는 P01~P14와 필수 scope/효과/예산이 확인된 경우만 사용하고 S01 실행은 별도 live 지시를 요구한다. 준비 Issue는 전체 기준 충족 후에만 완료 처리하며 Parent/#3~#10/MS2~9는 PREPARE에서 닫지 않는다.

S01~S03은 공개 offline API/CLI equivalence와 직접 API guard, checkout 밖 별도 consumer module을 검사한다. S04의 한 runner, S05 최소 native producer, S06 동일 producer 확장, S07 등록 reducer를 영향별 회귀로 검증한다. S08 최종 78칸은 새 현재 후보의 실제 세 host native 근거가 필요하다. PR synthetic tree와 actual merge tree가 동일하면 관계를 기록하고 필수 post-merge 검증을 수행하며, 다르면 영향 검사를 다시 수행한다. source-export/versioned local module-proxy 소비도 S08에 확인한다.

시간/입력/출력/저장/지원되는 memory·process 한도와 campaign/session 소모량을 등록한다. local heavy는 1, campaign heavy CI는 겹치는 workflow 전체를 합쳐 최대 4다(#317, 이전 3). 현재 Foundation은 workflow 전체의 공통 concurrency group으로 run을 직렬화하고, foundation 세 leg와 병행하는 native-prepare(합 4), 이어서 native Windows/Linux(폭 2)·Mac(폭 1)·large API(폭 1)(합 4)로 heavy 폭 4를 지킨다. 이후 native workflow도 같은 공유 lane을 사용하거나 동등한 scheduler 근거를 갖춰야 한다. 무변경 재시도는 원인이 확인된 일시적 infrastructure 오류에 한해 1회이며 제품 수정은 새 후보·남은 예산으로 검증한다. null/0을 무제한으로 해석하거나 한도·golden·필수 범위를 자동 완화하지 않는다.

#### Campaign 01 Session 진행 규칙 — 2026-10-03 PREPARE 보완

* kit 결함은 다음 Session 진입을 막는다. grammar 지원 gap은 기록·처분한 뒤 S08 판정으로 넘기며 S06/S07 진입을 막지 않는다. kit 결함은 kit 코드·계약이 입력을 잘못 읽거나 판정·guard·회수·결과를 틀리게 내는 경우이고, grammar gap은 kit가 충실히 보고한 결과에서 grammar가 유효한 source를 기대 구조로 파싱하지 못하는 경우다. generator 같은 도구 실패는 kit runner가 잘못 다루면 kit 결함이고, upstream 도구 자체의 실패는 gap처럼 기록·처분한다. 필수 mainstream gap은 S08의 지원 claim을 막는다.
* patch 사전 승인: 26 route의 등록 실패 사례를 고치는 저장소 안 patch subject 수정과, 26 route 전부의 build 이식성 수정(compiler 옵션·플랫폼 shim·build 스크립트)은 미리 승인됐다. patch subject 수정은 처음에 채택 6 route만 대상이었고, 비채택 20 route는 #76에 기록된 사용자 결정으로 같은 규칙 아래 들어왔다. 20 route는 고정 생성기로 patch 없이 재생성한 parser(`C2-REGENERATE-r1`)에서 출발한다. 26 route 어느 것이든 FULL PASS를 위한 저장소 안 patch는 `C2-PATCH-r1` 기록으로 등록한다. 등록에는 `language-sources.json`의 `c2_patch`(subject와 chain 전체 뒤의 patched file hash), `native-routes.json`의 chain(채택 chain 뒤 C2 subject의 file 순서대로)과 patched file, `reproduction-routes.json`의 출력 6개 고정이 모두 필요하다. 채택 route의 adoption 기록은 P05 identity 그대로 둔다. C2 subject가 만드는 파일(`create`)은 upstream scanner·공유 파일, 생성기 입력, adoption 파일, upstream route 파일이 아니어야 하고, `c2_patch`와 native patched file, route 파일(origin `patched`, role `scanner`)에 hash로 고정돼야 한다. build 이식성의 platform shim은 kit 쪽 compat header·build 옵션·스크립트이며 비채택 route의 scanner.c 직접 수정은 포함하지 않는다. 조건은 실패 사례·원문·기대값 선기록, 분리 context 리뷰 BLOCKER/MATERIAL 0, 세 OS 필수 CI 통과, 기존 등록 사례 회귀 없음, native는 Session envelope·운영 상한 안, 결과의 tracking·registry 기록이다. upstream 변경, 기대값·comparator 완화, provider/candidate 교체, 상한 상향, tag/Release/package publication은 제외다.
* campaign 예비분(wall 115200초, CI 720 job-분, download 1 GiB, 유료 0원)은 목표가 아니다. Session envelope 합계 밖의 추가분으로 본다(PREPARE 보완의 기록된 가정). Session이 envelope를 넘으면 사유와 소비를 ledger에 기록하고 차감한다. 소진되면 멈추고 사용자에게 묻는다.
* S04 runner는 Windows Job Object와 프로세스 제어에 `golang.org/x/sys`를 쓸 수 있다(2026-10-03 승인). 채택 시점 안정 버전을 고정하고 `go.sum` checksum을 확인하며 `CGO_ENABLED=0`을 유지한다. 취득은 S04 이후 x/sys를 요구하는 모든 CI run의 별도 step에서 `GOPROXY=https://proxy.golang.org`·`GOSUMDB=sum.golang.org`로 `go.sum`에 고정된 module만 내려받고(`src/dev/ci-gate/modules.ps1`; 무변경 재시도 규칙에 따라 보고된 모든 오류가 TLS handshake timeout·i/o timeout·연결 reset/refused·502/503/504 같은 알려진 일시적 network 오류일 때만 15초 뒤 1번 다시 시도하고, checksum·version·형식 오류가 하나라도 있거나 실행 자체가 실패하면 즉시 실패하며 `go mod verify`는 그대로 실행한다), 이후 build·test는 `GOPROXY=off`와 `-mod=readonly`로 실행한다. 로컬 검증도 같은 순서를 따르며, S04는 [로컬 명령 블록](../validation/validation.md#local-foundation-checks)과 workflow를 같은 작업 단위에서 갱신한다.
* 은퇴한 진입 조건: PR #47은 `631c01e`로 병합되었고, 이전의 S01 전 사용자 처리 조건은 역사적 기록이며 현재 gate가 아니다.
* 비공개 corpus와 실사용 source 값은 [NET461 등록부](../validation/net461-workload.md)를 따른다.
