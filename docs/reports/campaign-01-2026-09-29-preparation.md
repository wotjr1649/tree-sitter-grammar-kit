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
| D8 | feature 범위, [validation](../validation/validation.md) | S01 전 scope/feature 채택; 상세 bytes는 담당 단계 |
| D9 | trust, [provenance](../provenance/upstream-sources.md) | 기존 공통 도구 우선, SDK/DB 일괄 설치 제외 |
| D10 | [platform](../specs/platform-support.md), workload | 26×3=78, 실제 세 host core/API/native |
| D11 | [roadmap](../roadmap.md), tree/protocol, Session #6~#9 | S01 E0/S04 runner/S05 최소 driver/S06 확장/S07 replay |
| D12 | validation, trust | 유한 예산·소모량, local heavy1/campaign CI3, 진단된 infra retry1 |
| D13 | roadmap, validation | PREPARE와 master 승인 분리, 실제 review/CI/merge/readback |

## 채택값·관측·남은 입력

공개 entry는 단일 root module의 `src/kit`이다. 첫 Inspect/Identity request/result/error·소유권·취소·한도를 설계했으며 구현과 external-consumer 실행은 S01에 속한다. E0/manifest r1은 mode provenance와 관측 전용 assessment를 명시한다. S05는 base64 bytes를 사용하는 bounded JSON transport와 실제 old-tree edit 경로를 소유하며 S06이 같은 producer를 확장한다.

26개 후보의 공개 commit과 실제 grammar subdir·generated 파일·scanner/shared file 목록을 [등록부](../../src/contracts/language-sources.json)에 기록했다. metadata 존재는 full source closure 검토나 언어 지원 성공이 아니다. Swift 후보의 고정 source에는 `parser.c`가 없어 generation 준비가 필요하다. T-SQL과 PostgreSQL은 서로 다른 dialect 후보로 유지한다.

공식 stable 상한 대조와 사용자가 채택한 26개 숫자/모드는 [scope 표](../validation/language-feature-scope.md)에 있다. 모든 사양 장·버전별 syntax 변화의 전수 disposition, candidate 구조 mapping·required case 종류는 아직 완료로 판정하지 않는다. fixture/expected tree를 생성하거나 검사 대상의 출력을 golden으로 채택하지 않았다.

확인한 필수 위험은 C# 후보의 file-based app directives 미지원과 완전한 dialect/latest-feature 지원의 미확인이다. [후보 C# 문서](https://github.com/tree-sitter/tree-sitter-c-sharp/tree/9150f7d56bb47f1a809fa23623f1ba1413e93fa9)는 지원 예외를 공개한다. 일반 `sql` grammar나 기본 smoke 성공으로 이 격차를 해소했다고 기록하지 않는다. upstream 수정·scope 축소·미해결 격차를 안고 S01 진행하는 결정은 이번 PREPARE가 자동 수행하지 않는다.

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
