# 플랫폼 지원 계약

설계 대상은 Windows Native amd64, Linux amd64, macOS Apple Silicon arm64다. WSL은 선행 조건이 아니다. macOS Intel·Windows ARM64·다른 배포판을 자동으로 포함하지 않는다.

Campaign 01은 이 세 조합 모두에서 core/공개 offline API와 [26개 route](../validation/language-feature-scope.md)의 등록 native workload를 필수로 검증한다. `26 × 3 = 78` 요약 칸은 각 route의 feature·fixture·query·edit·source/tool/policy/comparator 근거를 참조한다. compiler version이나 cross-build만으로 native 실행 칸을 채우지 않는다. 미지원 필수 capability는 전체 지원 완료를 막으며 OS 행을 삭제하는 사유가 아니다.

아래 표의 기존 기능은 main `d0fd464`의 [post-merge CI run 37452270854](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/37452270854)(attempt 1, foundation 세 OS·Linux 26 route 재생성·native routes 세 OS·qualification·Linux race 모두 성공)와 각 Session 보고서를 근거로 한다. #108의 새 macOS 진단은 [PR CI run 37462859513](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/37462859513)(attempt 1, head `2ade221`, synthetic checkout `9357864`)에서 실제 step outcome·준비 결과·Go/runner/checkout identity와 artifact를 확인했다. 26 route의 독립 두 작업 공간·등록 출력 6개씩이 모두 일치했고 재사용은 0개다. 같은 run의 세 OS qualification은 SUPPORTED, API 관측은 204건이다.

| capability | 현재 상태 | 근거와 범위 |
|---|---|---|
| foundation build/test | VERIFIED(세 OS) | 필수 CI. Go/OS/arch, runner image, checkout SHA를 확인한다. `CGO_ENABLED=0` active closure에 `CgoFiles`와 `runtime/cgo`가 없다 |
| offline 제품 기능(`inspect`·`identity`·`verify`·`schema`와 공개 API) | VERIFIED(세 OS) | S01~S03 기능·경계·결정성 시험. CLI와 API 결과 bytes 동일. checkout 밖 consumer와 module proxy consumer |
| regenerate(`reproduce`) | VERIFIED: owned fixture(세 OS), 26 route(Linux CI의 `native prepare`가 등록 출력과 대조해 26 route 모두 `reference_match` PASS, Windows 로컬). VERIFIED: 26 route(macOS CI, #108) | S04 tool closure, 독립 두 작업 공간, 기준 비교. 26 route 모두 native 입력은 tree-sitter 0.27.0 재생성물이다. 채택 6 route는 patch chain 뒤, 나머지 20 route는 patch 없이(`C2-REGENERATE-r1`) 재생성한다. C2 patch(`C2-PATCH-r1`)가 등록된 route는 그 단계까지 적용한 뒤 재생성한다. 20 route의 upstream 생성물(이전 CLI)과의 차이는 S04에 기록돼 있다 |
| native parse·edit·query(`incremental`·`oracle record`) | VERIFIED(세 OS 등록 사례) | 27 route, failures 0. Linux ASan/UBSan suite. compiler identity는 host별이다 |
| supervision(runner) | VERIFIED(backend별) | Windows Job Object hard, Linux 위임 cgroup v2 hard, macOS process group sampled. macOS에서는 그룹을 떠난 조용한 하위 process를 정리하지 못하며 `NOT_CONTAINED`로 기록한다 |
| full qualification(`qualify`) | AVAILABLE. 현재 판정은 **지원 claim SUPPORTED**(`platform_claims` 세 OS 모두 SUPPORTED) | Campaign 02([#76](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/76)) 결과. 78칸 PASS(kit 축 78, 요구 축 78, 비교 FAIL 0). 필수 의무 2478건(826 × 3 OS) PASS, FAIL·BLOCKED·미충족 0. 실행된 추가 역할 행 `n461-svc`(세 OS)·`n461-large`(Windows) PASS. 개발 main의 API claim 관측 204건(#105 이후)은 판정에 들어가지 않는 기록이다 |
| cross-OS semantic parity | VERIFIED(78칸, 비교 FAIL 0) | S08 동일 의미 identity와 attempt의 완전한 OS 집합. host build identity는 비교하지 않고 host별로 결속한다 |
| race 진단 | AVAILABLE(Linux·macOS CI 비필수 진단 lane); VERIFIED(macOS CI·Windows 로컬) | `CGO_ENABLED=1` `go test -race`. CGO-free 필수 gate와 분리된다. Windows는 로컬 실행 근거다(Go 1.27.1, MSYS2 gcc 16.2.0, 고정 runtime으로 native 시험 포함, 6 package ok, DATA RACE 0, #74). macOS CI는 위 run에서 실제 diagnostic outcome success·CGO_ENABLED=1을 확인했다(#108). process group·sampled memory backend 제약을 유지한다. CI의 native 실행 시험은 runtime 도구 미설정으로 skip된다 |

각 행의 상태는 AVAILABLE(기능 존재), VERIFIED(정확한 시험 근거), UNSUPPORTED, NOT_RUN을 구분한다. foundation 성공은 제품/native 지원 근거가 아니다. 실제 관측값은 Session 보고서와 CI receipt가 소유한다.

## 개발 runtime의 field 조회 (#105)

native 준비와 build는 기반 runtime commit에 [검토된 6줄 patch](../provenance/upstream-sources.md#native-runtime의-field-조회-patch-105)를 적용한 closure를 사용한다. visible alias의 field가 부모로 상속되는 조회를 막으며 일반 hidden-node 상속은 유지한다. 출처 commit, patch 파일·원본·적용 후 hash와 upstream release 포함 시 제거 조건은 provenance와 runtime manifest가 소유한다. 원본 runtime과의 동일 입력 비교 및 세 OS qualification은 [validation 계약](../validation/validation.md#native-runtime-patch-검증-105)을 따른다. API 결과의 변화는 기존 tree·incremental·query 의미와 지원 판정의 유지 근거를 함께 기록한다. 아래 release 시점의 관측값은 v0.2.0 source tag의 역사적 결과다.

2026-10-06 Windows 로컬 비교는 26 route와 추가 csharp-svc·대형 입력을 같은 조건에서 실행했다. 5069개 S05/S06 case record의 tree·incremental·query·사실·요구 결과는 같고 helper failures는 양쪽 모두 0이다. `api_findings`는 142→69건(`field_lookup_extra` 133→62, 다른 자식 반환 5→3, 누락 4→4)이다. csharp-svc 1건을 제외한 qualification 대상은 host당 141→68건이며 세 OS의 실제 합계와 판정은 #105의 CI 근거로 확인한다. 이는 사례별 첫 차이의 수다. 남은 C# ERROR field 관측과 Swift hidden wrapper의 field 우선순위 차이는 이번 visible-alias patch의 처리 범위 밖이며 API 비교 검사는 유지한다.

## Release v0.2.0의 claim

release의 범위는 [제품 범위](scope.md)가 정한다. 아래 한계는 source tag `v0.2.0`의 역사적 상태다. 개발 main의 runtime 수정과 새 macOS CI lane 상태는 위 표와 #105·#108 근거를 따르며 새 release를 뜻하지 않는다.

- **VERIFIED(세 OS):** offline `inspect`, `identity`, `verify`, `schema`와 그 공개 API(`src/kit`).
- **26 route qualification 판정 SUPPORTED(세 OS):** 위 표의 `qualify` 행이다. 등록 사례와 [qualification inventory](../../src/contracts/qualification-c1.json)에 대한 판정이며, 그 밖의 입력을 parse할 수 있다고 인증하지 않는다. 사양이 문맥 자유 구문 밖에 둔 검사(early error 등)는 [disposition](../validation/language-feature-disposition.md)에서 SEM이며 판정 대상이 아니다.
- **experimental:** `corpus`, `reproduce`, `incremental`, `oracle record`, `replay`, `evidence verify`, `qualify`와 그 API.
  - 위 표의 근거로 동작하지만 호환성은 약속하지 않는다.
  - native 실행에는 승인된 capability(`BUILD_NATIVE`, `EXEC_NATIVE`, `EXEC_GENERATOR`)와 host compiler가 필요하다.
- **알려진 한계**
  - tree-sitter runtime의 field 조회와 cursor가 다른 API claim 관측 423건. 판정에 들어가지 않는 기록이며, runtime 결함 수정 뒤 다시 잰다([#105](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/105)).
  - Campaign 02 grammar patch의 heuristic과 미구현 부분. 주요 항목은 C/C++ macro heuristic(모호한 입력을 오류 없이 받을 수 있음), go 자동 세미콜론의 미구현 상태, C# 조건부 section의 제한된 선택이다. 전체 목록은 [변경 기록](../../CHANGELOG.md) v0.2.0 절의 Campaign 02 이력에 있다.
  - T-SQL 과잉 수용(S08 기록: r6에서 3종, r5부터 2종)과, 첫 문장이 아닌 `;sp_executesql`을 known miss로 두는 동적 SQL 탐지. 둘 다 Campaign 02 뒤에 다시 분류하지 않았다.
  - `--out`이 `subst`·bind mount 별칭을 검출하지 못함
  - BrightScript·cooklang 역할 NOT_RUN
  - `n461-large` 대형 실사용 profile은 Windows에서만 실행(Linux·macOS NOT_APPLICABLE)
  - 26 route의 macOS 재생성과 macOS race 진단은 NOT_RUN
  - [#65](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/65)의 시험 시간 여유
- v0.1.0(2026-10-04)의 claim은 offline core VERIFIED였고 26 route 지원은 주장하지 않았다(당시 판정 BLOCKED). 당시 claim과 한계는 [변경 기록](../../CHANGELOG.md)의 v0.1.0 절에 있다.
- release는 binary를 배포하지 않고 소스 tag만 낸다. 임의 입력에 대한 지원을 인증하지 않는다.

Windows backend의 Job Object와 POSIX process group/rlimit는 같은 강도의 격리가 아니다. timeout, output cap, memory metric별 cap, process-tree cleanup, 환경 정리를 각각 검증한다. Linux RSS/KiB, macOS ru_maxrss/bytes, sampled footprint, Windows working set/private bytes/commit, allocator-live를 합치지 않는다. 원 metric/단위/관측 API·sampling 간격·kernel hard cap 여부를 기록한다. sampled kill을 hard memory cap으로 표현하지 않는다.

semantic 비교는 source/input/profile/protocol/comparator가 같은 결정적 필드에만 exact 적용한다. timing cancellation trigger, peak memory, latency, host path/clock은 별도 observation이다. normalization allowlist를 실행 전 고정하고 원 raw를 보존한다. mismatch 이후 필드를 지워 PASS로 만들지 않는다. 다른 host의 절대 latency 동일성은 주장하지 않는다.

CI 집계는 repo/workflow/candidate/run attempt/input identity와 모든 필수 OS artifact를 확인한다. missing/skip/cancel은 PASS가 아니다. cross-build는 native 실행으로 기록하지 않는다. race 검사는 CGO가 필요한 별도 diagnostic lane(`race diagnostic (<os>)`, Linux·macOS 비필수)이며 CGO-free core gate와 분리한다.

compiler·executable·OS image는 host별 identity를 유지한다. 같은 runtime/header compatibility를 사용해도 동일한 빌드 bytes를 요구하지 않는다. 보조 언어 parser 대조는 등록된 모호·최신·불일치 case에 한해 필요한 환경에서 수행하며 다른 OS native 실행을 대체하지 않는다. 세 OS runner label/architecture는 [기계 정의](../../src/contracts/campaign-01.json)에 있고 실제 image와 tool은 run receipt에 기록한다.

## S04 runner backend

`src/internal/runner`는 generator와 이후 native 실행이 함께 쓰는 하나의 runner다. 실행 파일은 절대 경로로만 받고 PATH를 찾지 않으며, argv는 shell 없이 그대로 넘기고 환경 변수는 호출자가 준 목록만 쓴다(Windows에서는 `os/exec`가 `SYSTEMROOT`를 더한다). stdout·stderr 상한을 넘으면 `OUTPUT_LIMIT`, wall을 넘으면 `WALL_LIMIT`, 메모리 상한이면 `MEMORY_LIMIT`로 끝나며 셋 다 `RESOURCE_LIMIT`다. 호출자 취소는 `CANCELLED`다. 주 process가 끝난 뒤 남은 하위 process는 종료하고 수(`residual_after_exit`)를 기록한다. cgroup backend에서는 커널이 아직 해제하지 않은 종료 중 task도 이 수에 들어갈 수 있다. process tree가 grace 안에 비고 출력 pipe가 닫혀야 cleanup `verified`이며, 그렇지 않으면 성공으로 보고하지 않는다. `cleanup.scope`는 verified가 덮는 범위다. `JOB_OBJECT`·`CGROUP_KILL`은 tree 전체, `PROCESS_GROUP`은 process group뿐이다.

| OS | backend | tree 정리 | 그룹 이탈 하위 process | 메모리 |
|---|---|---|---|---|
| windows/amd64 | Job Object(정지 상태로 시작 → job 배정 → 재개, breakaway 불허, KILL_ON_JOB_CLOSE) | `TerminateJobObject`, active process 0 확인 | 포함 | hard: `JobMemoryLimit`(job commit bytes), 완료 port의 memory-limit 통지 |
| linux/amd64 + 위임 cgroup | cgroup v2 leaf에 직접 시작(`CLONE_INTO_CGROUP`), process group leader | SIGTERM → grace → `cgroup.kill`, `cgroup.procs`가 비고 `cgroup.events`가 `populated 0`이 될 때까지 확인(종료 중인 task는 procs에서 먼저 빠짐) 뒤 leaf 제거 | 포함 | hard: `memory.max`·`memory.oom.group`·`memory.swap.max=0`, `memory.events` oom_kill |
| linux/amd64, 위임 cgroup 없음 | process group | SIGTERM → grace → SIGKILL, `/proc` pgid 확인 | 미포함(`NOT_CONTAINED`) | sampled(`/proc/PID/statm`), non-strict |
| darwin/arm64 | process group | SIGTERM → grace → SIGKILL, `kern.proc.pgrp` 확인 | 미포함(`NOT_CONTAINED`) | sampled(`/bin/ps` rss), non-strict |

hard memory를 요구한 요청은 hard backend가 없으면 실행 전에 `MEMORY_HARD_CAP_UNSUPPORTED`로 막는다. sampled 종료는 hard cap 근거가 아니다. 미포함 backend에서 그룹을 떠난 하위 process가 pipe를 잡고 있으면 cleanup은 verified가 아니다. pipe를 잡지 않은 이탈 process는 그 backend가 관측하지 못하므로, scope `PROCESS_GROUP`의 verified는 그런 process의 종료를 뜻하지 않는다. interactive 실행에서는 주 process가 끝나면(종료시킨 경우든 스스로 끝난 경우든) 남은 하위 process를 바로 종료하고, 그래도 stdout을 잡은 이탈 process가 있으면 grace 뒤 읽기 쪽을 닫아 대기를 끝낸다. 호출자는 Wait 전에 stdout을 읽으며, 닫힌 뒤의 읽기는 EOF가 아니라 오류다. Linux 위임 cgroup은 부모의 `cgroup.subtree_control`에 memory가 이미 켜져 있어야 하며 runner는 부모를 바꾸지 않는다. 이 backend들은 승인된 도구를 제한·정리하는 수단이며 적대적 native code 격리가 아니다. hosted Linux CI는 위임 cgroup에서 시험하고, cgroup 없이 실행되면 capability 시험이 실패한다.

길이 접두 frame envelope(`RunBatch`)는 frame마다 4-byte big-endian 길이와 payload를 주고받는다. 기본값은 요청 50331648 bytes·응답 16777216 bytes, frame watchdog 60초+5초, batch stdout 268435456 bytes, batch wall 3600초다. 끝난 frame은 `COMPLETED`로 남는다. 진행 중 frame은 자기 한도(watchdog, 응답 크기, 메모리)면 `RESOURCE_LIMIT`, batch 한도(batch wall, batch stdout)나 호출자 취소면 `REQUEUED`, process가 스스로 끝나거나 protocol을 어기면 `FAILED`다. 보내지 않은 frame은 `REQUEUED`, 요청 상한을 넘는 frame은 보내지 않고 `RESOURCE_LIMIT`다. 단일 native 요청 process의 정책 `DefaultRequestPolicy`는 parse 요청 wall 90초, edit 요청 wall 300초와 요청당 최대 4 edit(5개 이상은 실행 전 `EDIT_COUNT_LIMIT`), 메모리 4 GiB(Windows·Linux hard, macOS sampled), 종료 유예 5초다. batch는 같은 메모리·유예에 batch wall을 쓴다. driver 쪽 60초 progress callback과 allocator hook은 S05 범위다.

`RunBatch`는 모든 frame에 응답이 온 뒤 stdout에 남은 bytes를 `trailing_bytes`로 센다(S05). 호출자는 0이 아니면 그 batch를 protocol 위반으로 본다.

## S05 native build

`src/internal/native`는 `src/drivers/native-c/driver.c`(내장)와 내장 manifest로 고정한 Tree-sitter runtime source(`tree-sitter/tree-sitter@659cda7c7f86ebe31cc825dc5da59e9add172dc7`의 83개 파일), profile이 hash로 고정한 grammar 파일, 생성한 shim을 새 build 디렉터리에 복사하고 runner로 컴파일러를 실행한다. 컴파일러는 profile의 content hash로 고정하고 호출자가 준 절대 경로에서 실행한다(PATH 탐색·설치·복사 없음). 그 경로가 link이면(Ubuntu `/usr/bin/gcc` → `gcc-13` 등) 해석한 실제 파일의 bytes를 대조하고 그 파일을 실행한다. 해석할 수 없는 link는 `TOOL_MISSING`이다. 실행 이름(argv[0])도 해석한 경로이므로, 호출 이름이나 위치로 동작을 나누는 wrapper·shim(ccache masquerade, multi-call binary, macOS `/usr/bin/clang` 같은 xcrun shim 등)은 link 대상으로 지원하지 않는다. shim은 그 경로를 그대로 지정한다. build가 실패하면 결과 finding에 refusal 원인과 실패한 단계의 stderr 끝(단계당 4096 bytes)을 남긴다. 환경은 `PATH`(컴파일러 디렉터리, Unix는 `/usr/bin:/bin` 추가), build 안 `TMP`·`TEMP`·`TMPDIR`, 그리고 있으면 `SystemRoot`·`WINDIR`·`ProgramData`·`ProgramFiles`·`ProgramFiles(x86)`·`PROCESSOR_ARCHITECTURE`·`DEVELOPER_DIR`·`SDKROOT`뿐이며 결과에는 이름만 남긴다. 단계는 `--version`, runtime `lib.c`(`-O2 -std=c11 -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE`), parser(`-O0 -std=gnu11 -DTREE_SITTER_REUSE_ALLOCATOR`), scanner(`-O2 -std=gnu11 -DTREE_SITTER_REUSE_ALLOCATOR`), driver·shim(`-O2 -std=gnu11`), link이고 각 단계는 c-build 상한(wall 120초, 출력 8388608 bytes, 메모리 4 GiB)이다. `TREE_SITTER_REUSE_ALLOCATOR`로 scanner 할당도 driver의 allocator hook을 거친다. build identity(`tsgk-native-build/r1`)는 symbol, 컴파일러 hash, runtime commit, OS/arch, 모든 입력 파일 hash와 모든 단계 argv(define 포함)를 묶는다. 시험 전용 fault define(`TSGK_FAULT_*`, owned scanner의 `TSGK_SCANNER_FAULT`)과 sanitizer(`-fsanitize=address,undefined`) build도 identity에 들어간다. 실행 직전마다 executable hash를 다시 확인한다. build identity는 입력 closure와 argv를 고정할 뿐 executable bytes를 고정하지 않는다. 같은 identity로 다시 build해도 linker가 넣는 시각 등으로 bytes가 달라질 수 있으므로(Windows PE에서 관측) executable sha256은 build마다 따로 기록하고 그 build의 실행에만 쓴다. 같은 runtime·grammar라도 OS별 executable bytes는 다르며 같은 bytes를 요구하지 않는다. 지원 컴파일러 형태는 gcc·clang 계열 명령행이다(MSVC `cl`은 쓰지 않는다).

## S06 query·기록과 r3 대용량 범위

`oracle record`는 S05 native build와 같은 build·실행 경로(같은 driver source, 같은 closure identity, 실행 직전 executable hash 재확인)를 쓰며 세 host 모두에서 `native-query`를 실행한다. query와 API 관측은 pinned runtime의 공개 API만 쓰고 OS별 분기는 없다. 기록의 의미 비교(tree digest, capture stream, 사실)는 host와 무관하고 executable hash·시간·process 결과는 host 관측이다.

`real-world-source-r3`와 `native-query-large`는 windows/amd64 전용이다(사용자 결정 `C1-REAL-WORLD-SOURCE-WINDOWS-R3`: NET461 workload는 Windows에서 실행되는 WinForms/.NET Framework 4.6.1). 메모리는 8589934592(8 GiB)이고 Job Object hard 상한이다. Linux·macOS에서는 실행하지도 측정하지도 않으며, CLI는 build 전에 `OPERATION_PLATFORM_SCOPE`로 거부하고 route helper는 결과를 `NOT_APPLICABLE`로 둔다. S05가 세 host에서 남긴 `real-world-source-r2` 결과(4 GiB에서 32 MiB 입력이 Linux·Windows hard 상한으로 `RESOURCE_LIMIT`, macOS sampled에서 PASS)는 역사로 보존한다. hosted windows-2025 runner(RAM 약 16 GB)는 로컬 Windows 측정 peak(5157146624 bytes)가 들어가는 8 GiB 상한을 수용할 수 있는 크기다. 실제 hosted 측정값은 CI route 결과가 소유한다. grammar route 26개의 세 OS 검증은 바뀌지 않는다.

## S08 qualification

PR·push CI의 `qualification (ubuntu-24.04)` job이 같은 run·attempt의 세 `native routes` host 근거(실행 identity, S06 기록 set, workload profile)를 받아 `tsgk qualify`로 78칸과 추가 역할 행을 만든다. 집계 자체는 어느 OS에서나 같은 offline 계산이며 native 실행은 각 host job이 한다. job은 `mechanism_gate`(완결성, cohort·자격, 칸별 kit 축과 세 host 의미 비교, 실행된 추가 역할 행)로 실패를 정하고, 문법 요구 FAIL과 미충족 의무(`NOT_COVERED`: 사례 없음, production 대안 미완·`PENDING` 행)는 결과 행렬에만 남는다(지원 claim `BLOCKED`). 칸·축·비교 규칙은 [identity/evidence](identity-and-evidence.md) `S08 구현`이 소유한다. windows 전용 NET461 대용량 workload(`n461-large`)는 Linux·macOS 행이 `NOT_APPLICABLE`이다.
