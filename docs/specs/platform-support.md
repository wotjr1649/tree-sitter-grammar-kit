# 플랫폼 지원 계약

설계 대상은 Windows Native amd64, Linux amd64, macOS Apple Silicon arm64다. WSL은 선행 조건이 아니다. macOS Intel·Windows ARM64·다른 배포판을 자동으로 포함하지 않는다.

Campaign 01은 이 세 조합 모두에서 core/공개 offline API와 [26개 route](../validation/language-feature-scope.md)의 등록 native workload를 필수로 검증한다. `26 × 3 = 78` 요약 칸은 각 route의 feature·fixture·query·edit·source/tool/policy/comparator 근거를 참조한다. compiler version이나 cross-build만으로 native 실행 칸을 채우지 않는다. 미지원 필수 capability는 전체 지원 완료를 막으며 OS 행을 삭제하는 사유가 아니다.

| capability | Session 00의 범위 | 실제 지원 선언에 필요한 근거 |
|---|---|---|
| foundation build/test | 세 OS 필수 CI | exact Go/OS/arch/runner image와 실행 SHA |
| offline 제품 기능 | NOT_RUN | S01~03 기능·경계·결정성 시험 |
| regenerate | NOT_RUN | S04 tool closure·독립 생성·기준 비교 |
| native parse | NOT_RUN | S05~06 compiler/runtime/scanner/실행 결과 |
| supervision | NOT_RUN | OS별 timeout/output/memory/child cleanup adverse test |
| full qualification | NOT_RUN | 등록 profile의 모든 필수 gate |
| cross-OS semantic parity | NOT_RUN | S08 동일 의미 identity·attempt의 완전한 OS 집합 |

각 행의 상태는 AVAILABLE(기능 존재), VERIFIED(정확한 시험 근거), UNSUPPORTED, NOT_RUN을 구분한다. foundation 성공은 제품/native 지원 근거가 아니다. 실제 관측값은 [Session 00 보고](../reports/session-00-foundation.md)와 CI receipt가 소유한다.

Windows backend의 Job Object와 POSIX process group/rlimit는 같은 강도의 격리가 아니다. timeout, output cap, memory metric별 cap, process-tree cleanup, 환경 정리를 각각 검증한다. Linux RSS/KiB, macOS ru_maxrss/bytes, sampled footprint, Windows working set/private bytes/commit, allocator-live를 합치지 않는다. 원 metric/단위/관측 API·sampling 간격·kernel hard cap 여부를 기록한다. sampled kill을 hard memory cap으로 표현하지 않는다.

semantic 비교는 source/input/profile/protocol/comparator가 같은 결정적 필드에만 exact 적용한다. timing cancellation trigger, peak memory, latency, host path/clock은 별도 observation이다. normalization allowlist를 실행 전 고정하고 원 raw를 보존한다. mismatch 이후 필드를 지워 PASS로 만들지 않는다. 다른 host의 절대 latency 동일성은 주장하지 않는다.

CI 집계는 repo/workflow/candidate/run attempt/input identity와 모든 필수 OS artifact를 확인한다. missing/skip/cancel은 PASS가 아니다. cross-build는 native 실행으로 기록하지 않는다. 향후 race 검사는 CGO가 필요한 별도 diagnostic lane이며 CGO-free core gate와 분리한다.

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
