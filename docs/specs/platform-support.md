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
