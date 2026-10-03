# Session 04 — Reproducibility & Runner 결과

Campaign `TSGK-C1-20260929-R1`, 추적 `TSGK-C1-S04`, Issue #6, Milestone 5, branch `session/04-reproducibility`(base main `24bdba5`). 이 보고서는 아래에 적은 revision과 로컬 관측만 설명한다. PR·세 OS CI·merge·post-merge 결과는 이 문서 작성 시점에 없으며 그 단계의 receipt가 소유한다.

## 구현 범위

* 하나의 외부 process runner `src/internal/runner`: 절대 경로 실행 파일, shell 없는 argv, 정확한 cwd·환경(상속 없음), stdout·stderr 상한, wall·취소, 주 process 종료 뒤 남은 하위 process 종료, process tree가 비고 출력 pipe가 닫혔는지 확인한 cleanup `verified`, capability 보고. backend는 Windows Job Object(정지 상태 시작 → job 배정 → 재개, job commit memory hard cap과 완료 port 통지), Linux 위임 cgroup v2 leaf(`CLONE_INTO_CGROUP`, `memory.max`·`memory.oom.group`, `cgroup.kill`) 또는 process group, macOS process group과 sampled 메모리(non-strict)다. 계약은 [플랫폼](../specs/platform-support.md)의 `S04 runner backend`다.
* 길이 접두 frame envelope `runner.RunBatch`: 요청 50331648·응답 16777216 bytes, frame watchdog 60초+5초, batch stdout 268435456 bytes, batch wall 3600초가 기본값이다. 진행 중 frame은 자기 한도면 `RESOURCE_LIMIT`, batch 한도·취소면 `REQUEUED`, process 자체 종료면 `FAILED`이고 끝난 frame은 `COMPLETED`로 남는다(S04-A16).
* `tsgk reproduce`와 `tsgk-reproduce/r1`: 공개 offline API에는 profile 해석 `kit.ParseReproduceProfile`만 있고, 실행은 CLI의 `src/internal/reproduce`가 runner로 한다. 선언 snapshot의 link·hash 검사, 도구 내용 hash 고정과 사본 실행, 매번 새로 만든 두 작업 공간, 작업 공간 안에 가둔 HOME·cache·temp·tree-sitter 설정, 등록 output마다 A↔B와 A↔기준의 별도 판정, 등록되지 않은 생성물과 source 사본 쓰기의 기록, JS/JSON claim 분리, 덮어쓰지 않는 publication과 실패 결과 보존, 작업 공간 삭제 확인. 계약은 [CLI/profile](../specs/cli-and-profile.md)의 `S04 구현`, [공개 API](../specs/public-go-api.md)의 `S04 함수`다.
* 의존성: 승인된 `golang.org/x/sys` v0.48.0(2026-08-31 공개, 채택 시점 최신 안정판, `go.sum` 고정). CI는 별도 step에서 proxy·sumdb로 고정 module만 받고 이후 `GOPROXY=off`·`-mod=readonly`다. 공개 package closure에는 runner·`os/exec`·x/sys·network가 없다. Windows의 x/sys는 socket 타입 정의를 위해 `net`을 import하지만 repository package는 network package를 import하지 않는다(`TestOfflineClosure`).
* 등록부 [`reproduction-routes.json`](../../src/contracts/reproduction-routes.json): 26 route 전부의 JS 재생성 진입점, 기준 종류(채택 6 route는 PREPARE 생성물, 나머지 20은 upstream 생성물), npm grammar 의존 2개의 lockfile integrity, tree-sitter CLI 0.27.0·Node 24.21.0의 세 OS 공식 release digest, PostgreSQL no-optimization 재생성 gap, build 이식성 patch `NOT_APPLICABLE`(`no patch needed`)을 등록한다.

제외: native build·parse(S05), 26 route의 Linux·macOS 재생성 실행(아래 남은 일), 생성기 대체, 도구 설치, 다른 저장소 변경.

## 도구 identity (S04-A18)

generator 실행 전에 공식 release asset digest를 기록했다(`artifacts/.../session-04/tools/tool-registry-r1.json`). tree-sitter 0.27.0의 linux-x64 asset digest는 PREPARE tool registry 값과 같다. Windows asset(`tree-sitter-windows-x64.gz`, 3778280 bytes)을 받아 digest를 확인한 뒤 풀었고 실행 파일은 9745408 bytes, sha256 `9fbc4f28…`, `--version`은 `tree-sitter 0.27.0`이다. 로컬 `node.exe`는 Node 24.21.0 `SHASUMS256.txt`의 `win-x64/node.exe`와 같다. macOS asset은 digest만 등록했고 실제 실행은 CI step이 한다. build 이식성 patch는 S04에서 build를 하지 않으므로 필요 없었다(`NOT_APPLICABLE`, `no patch needed`).

## 관측 revision과 로컬 검사

PLACEHOLDER_CHECKS

## 채택 route patch chain과 재생성 (S04-A17)

PLACEHOLDER_A17

## 26 route 재현 (S04-A15)

PLACEHOLDER_A15

## acceptance 연결

PLACEHOLDER_ACCEPT

## 분리 context 리뷰와 처분

PLACEHOLDER_REVIEW

## 남은 일과 한계

PLACEHOLDER_REMAIN
