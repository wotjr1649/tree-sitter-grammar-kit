# Session 04 — Reproducibility & Runner 결과

Campaign `TSGK-C1-20260929-R1`, 추적 `TSGK-C1-S04`, Issue #6, Milestone 5, branch `session/04-reproducibility`(base main `24bdba5`). 이 보고서는 아래에 적은 revision과 로컬 관측만 설명한다. PR·세 OS CI·merge·post-merge 결과는 이 문서 작성 시점에 없으며 그 단계의 receipt가 소유한다.

## 구현 범위

* 하나의 외부 process runner `src/internal/runner`: 절대 경로 실행 파일, shell 없는 argv, 정확한 cwd·환경(상속 없음), stdout·stderr 상한, wall·취소, 주 process 종료 뒤 남은 하위 process 종료, process tree가 비고 출력 pipe가 닫혔는지 확인한 cleanup `verified`, capability 보고. backend는 Windows Job Object(정지 상태 시작 → job 배정 → 재개, job commit memory hard cap과 완료 port 통지), Linux 위임 cgroup v2 leaf(`CLONE_INTO_CGROUP`, `memory.max`·`memory.oom.group`, `cgroup.kill`) 또는 process group, macOS process group과 sampled 메모리(non-strict)다. 계약은 [플랫폼](../specs/platform-support.md)의 `S04 runner backend`다.
* 단일 native 요청 정책 `runner.DefaultRequestPolicy`: parse 요청 wall 90초, edit 요청 wall 300초와 최대 4 edit(초과는 실행 전 `EDIT_COUNT_LIMIT`), 메모리 4 GiB(Windows·Linux hard, macOS sampled), 종료 유예 5초(S04-A16). 이 값을 실제로 쓰는 native 요청은 S05가 만든다.
* 길이 접두 frame envelope `runner.RunBatch`: 요청 50331648·응답 16777216 bytes, frame watchdog 60초+5초, batch stdout 268435456 bytes, batch wall 3600초가 기본값이다. 진행 중 frame은 자기 한도면 `RESOURCE_LIMIT`, batch 한도·취소면 `REQUEUED`, process 자체 종료면 `FAILED`이고 끝난 frame은 `COMPLETED`로 남는다(S04-A16).
* `tsgk reproduce`와 `tsgk-reproduce/r1`: 공개 offline API에는 profile 해석 `kit.ParseReproduceProfile`만 있고, 실행은 CLI의 `src/internal/reproduce`가 runner로 한다. 선언 snapshot의 link·hash 검사, 도구 내용 hash 고정과 사본 실행, 매번 새로 만든 두 작업 공간, 작업 공간 안에 가둔 HOME·cache·temp·tree-sitter 설정, 등록 output마다 A↔B와 A↔기준의 별도 판정, 등록되지 않은 생성물과 source 사본 쓰기의 기록, JS/JSON claim 분리, 덮어쓰지 않는 publication과 실패 결과 보존, 작업 공간 삭제 확인. 계약은 [CLI/profile](../specs/cli-and-profile.md)의 `S04 구현`, [공개 API](../specs/public-go-api.md)의 `S04 함수`다.
* 의존성: 승인된 `golang.org/x/sys` v0.48.0(2026-08-31 공개, 채택 시점 최신 안정판, `go.sum` 고정). CI는 별도 step에서 proxy·sumdb로 고정 module만 받고 이후 `GOPROXY=off`·`-mod=readonly`다. 공개 package closure에는 runner·`os/exec`·x/sys·network가 없다. Windows의 x/sys는 socket 타입 정의를 위해 `net`을 import하지만 repository package는 network package를 import하지 않는다(`TestOfflineClosure`).
* 등록부 [`reproduction-routes.json`](../../src/contracts/reproduction-routes.json): 26 route 전부의 JS 재생성 진입점, 기준 종류(채택 6 route는 PREPARE 생성물, 나머지 20은 upstream 생성물), npm grammar 의존 2개의 lockfile integrity, tree-sitter CLI 0.27.0·Node 24.21.0의 세 OS 공식 release digest, PostgreSQL no-optimization 재생성 gap, build 이식성 patch `NOT_APPLICABLE`(`no patch needed`)을 등록한다.

제외: native build·parse(S05), 26 route의 Linux·macOS 재생성 실행(아래 남은 일), 생성기 대체, 도구 설치, 다른 저장소 변경.

## 도구 identity (S04-A18)

generator 실행 전에 공식 release asset digest를 기록했다(`artifacts/.../session-04/tools/tool-registry-r1.json`). tree-sitter 0.27.0의 linux-x64 asset digest는 PREPARE tool registry 값과 같다. Windows asset(`tree-sitter-windows-x64.gz`, 3778280 bytes)을 받아 digest를 확인한 뒤 풀었고 실행 파일은 9745408 bytes, sha256 `9fbc4f28…`, `--version`은 `tree-sitter 0.27.0`이다. 로컬 `node.exe`는 Node 24.21.0 `SHASUMS256.txt`의 `win-x64/node.exe`와 같다. macOS asset은 digest만 등록했고 실제 실행은 CI step이 한다. build 이식성 patch는 S04에서 build를 하지 않으므로 필요 없었다(`NOT_APPLICABLE`, `no patch needed`).

## 관측 revision과 로컬 검사

구현 commit `f8fd305`(runner·frame), `519c319`(reproduce), `cc3ab68`(등록부·CI owned 재현)과 리뷰 수정 `a4c3fdb`, `4afd455`, `3c412ea`, `f0f2423`에서 Windows amd64, Go 1.27.1, `CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`로 [validation](../validation/validation.md)의 갱신된 명령 블록(고정 module 취득 → `GOPROXY=off`·`-mod=readonly`)을 실행했다. Linux·macOS 실행과 세 OS CI는 PR 단계에서 따로 기록한다.

| 검사 | 결과 |
|---|---|
| `go mod download`·`go mod verify`(proxy·sumdb), `gofmt -l src`, `go vet ./src/...`(windows, `GOOS=linux`, `GOOS=darwin`), `go build ./src/...`, `git diff --check` | 통과 |
| `go test ./src/... -count=1` | `src/kit`, `src/cmd/tsgk`, `src/internal/foundation`, `src/internal/runner`, `src/internal/reproduce` 통과 |
| runner·frame 시험 반복 | `-count=5`와 `-count=3`에서 모두 통과 |
| targeted mutant 14종(`cc3ab68`), 18종(`a4c3fdb`), 20종(`3c412ea`, `f0f2423`) | 14/14, 18/18, 20/20, 20/20 검출. `cc3ab68` 첫 실행은 미사용 변수로 컴파일되지 않은 mutant 3종이 있어 11/14로 기록했고(검출 실패가 아님), 컴파일되는 형태로 고친 재실행이 14/14다. receipt 모두 보존 |
| `src/dev/s04-reproduce/ci-owned.ps1`(로컬 Windows) | asset digest 확인 뒤 owned fixture 재현 PASS, Job Object hard memory backend |
| Windows capability receipt | `windows-job-object`, tree cleanup `JOB_OBJECT`, 그룹 이탈 하위 process 포함, memory `HARD`(job commit bytes) |

mutant는 첫 작업 공간 재사용, 작업 공간 밖 공유 home(cache), source hash 검사 생략, 도구 identity 검사 생략, 부모 process만 종료, 출력 상한 제거, Job memory 상한 제거, batch 한도를 자기 한도로 분류, 기존 결과 덮어쓰기, JSON 실행이 JS claim을 가져감, 작업 공간 삭제 실패 무시, generator 상한 초과 허용, 환경 상속, A↔B 비교 생략이다.

## 채택 route patch chain과 재생성 (S04-A17)

patch chain 재구성은 분리된 하위 작업이 S01이 결속한 upstream base bytes에 각 route의 patch subject를 순서대로 literal 적용했다(`artifacts/.../session-04/patch-chain-reconstruction-r1.json`). 모든 단계에서 `before`가 등록된 `occurrences`만큼 나왔고, 결과 10개 파일(C# `grammar.js`·`src/scanner.c`, TS·TSX `common/define-grammar.js`, Swift `grammar.js`, T-SQL 4개, PostgreSQL `postgres/grammar.js`)이 모두 `language-sources.json`의 adoption hash와 같다. 보존 후보 사본은 대조에만 썼다. C# r5 replacement에는 `occurrences`가 없어 각 1회로 보고 적용했고 실제로 각 1회였다. `remedy-followup-r2.json`의 PG 항목은 같은 patch의 재기재라 다시 적용하지 않고 동일성만 확인했다. 재구성 결과는 아래 재생성의 subject root를 만드는 builder가 adoption hash와 다시 대조했다.

재생성은 tree-sitter 0.27.0(Windows), Node 24.21.0, `--abi 15`, 최적화 기본값으로 `tsgk reproduce`를 실행했다. 6 route 모두 두 작업 공간의 6개 출력이 byte 단위로 같고, PREPARE가 Linux에서 만든 생성물 기록과 `parser.c`·`tree_sitter/parser.h`가 모두 같다(MATCH).

| route | parser.c bytes | PREPARE 기록 | wall A/B ms | job commit 최대 |
|---|---|---|---|---|
| csharp | 32815459 | `native-36917832850-assessment-r6` | 6185 / 5660 | 757 MiB |
| typescript | 8764241 | run 36795440494 build 입력 기록 | 2273 / 1571 | 130 MiB |
| tsx | 8788036 | run 36795440494 build 입력 기록 | 3110 / 1999 | 138 MiB |
| swift | 23472218 | run 36741763343 build 입력 기록 | 3547 / 2976 | 172 MiB |
| tsql | 26919442 | `native-36947507308-assessment-r5` | 7816 / 6164 | 401 MiB |
| postgresql-sql | 97664835 | `native-36882649292-assessment-r5` | 180381 / 111815 | 4622 MiB |

PREPARE 기록에 `grammar.json`·`node-types.json`·`alloc.h`·`array.h`의 생성물 hash가 없어 그 네 output은 `REFERENCE_ABSENT`이고, 그래서 6 route의 `reference_match`는 `NOT_CLAIMED`, assessment는 `BLOCKED`다(PASS로 채우지 않음). S04-A17의 표기로는 이 네 output의 기준 비교가 `RECORDED_NOT_RECOMPUTED`이며 PASS가 아니다. PostgreSQL은 4 GiB를 넘는 job commit(최대 4846997504 bytes)을 썼으므로 6 GiB 예외가 실제로 필요했다. no-optimization 재생성은 PREPARE에서 parse state 387042가 16-bit 상한 65535를 넘어 exit 1이었고 upstream 도구 gap으로 등록했다(`RECORDED_NOT_RECOMPUTED`, 다시 실행하지 않음).

S03이 넘긴 재생성 schema 의무: 6 route의 재생성 `node-types.json`은 모두 `tsgk schema check` PASS다. upstream→재생성 diff는 C# 2건(`file_based_app_directive` 추가 등), TS·TSX 각 4건(`defer` 추가 등), T-SQL 8건(`generated_always_clause`, `graph_table_type`, keyword 4개 추가 등), PostgreSQL 0건이며 S03이 후보 tree에서 관측한 node와 맞는다. Swift는 upstream schema가 S03의 `NODE_DUPLICATE`로 diff 입력이 될 수 없어 diff는 `NOT_ASSESSED`다. 새 관측: 0.27.0으로 실제 재생성한 Swift schema에는 그 중복이 없다(check PASS). S03 보고서의 "재생성 후보 사본에도 같은 중복" 서술은 재생성본이 아닌 upstream 사본을 본 것으로 보이므로, Swift 중복은 upstream에 들어 있는 이전 CLI 생성물의 문제로 S08 처분에 넘긴다.

## 26 route 재현 (S04-A15)

S01이 결속한 26 route source(`_ref/campaign-01-s01/sources`)로 route마다 subject root를 만들었다. 입력은 진입 `grammar.js`에서 정적으로 따라간 상대 require/import, `package.json`·`tree-sitter.json`, 그리고 lockfile integrity로 확인한 npm grammar 2개(cpp의 `tree-sitter-c@0.24.1`, TS·TSX의 `tree-sitter-javascript@0.23.1`; PREPARE 보존 사본과 tarball 내용이 같음)뿐이다. 해석하지 못한 의존은 0이다. 26 route 모두 JS 재생성 경로이며 JSON-only나 checked-in C만으로 JS 재현을 주장한 route는 없다. Swift는 upstream `parser.c`가 없으므로 기준은 PREPARE 생성물뿐이다.

| 결과(Windows, 26 route × 작업 공간 2) | route |
|---|---|
| 생성기 실행·두 작업 공간 byte 동일(`generator_ran`·`deterministic` PASS) | 26 전부 |
| 기준 전부 일치(assessment PASS) | php |
| upstream 생성물과 불일치(assessment FAIL) | json, go, python, javascript, jsx, java, c, rust, dart, ruby, r, bash, powershell, html, css, yaml, xml, cpp, kotlin |
| 채택 route: `parser.c`·`parser.h` 일치, 나머지 기준 없음(BLOCKED) | csharp, typescript, tsx, swift, tsql, postgresql-sql |

비채택 route의 불일치는 upstream이 다른 CLI 버전으로 만든 생성물과 0.27.0 출력의 차이다. 예를 들어 go `parser.c`의 첫 줄은 upstream `/* Automatically @generated by tree-sitter v0.25.8 */`와 0.27.0 `/* Automatically @generated by tree-sitter */`로 다르다. 비채택 20 route 중 `array.h`는 php를 뺀 19 route에서 다르고, `parser.c`가 같은 route는 c, dart, powershell, cpp, php다. 정규화하지 않고 불일치로 기록했으며 kit 결함이 아니라 기준 생성물의 drift다. 이 drift를 받아들일지는 route별 기준 갱신 결정이며 이번 Session에서 기준을 바꾸지 않았다.

세 OS: Windows는 위 실행이 근거다. Linux·macOS는 foundation CI가 같은 등록 tree-sitter 0.27.0 asset으로 owned fixture를 재현한다. 26 route의 Linux·macOS 재생성은 이번 Session에서 실행하지 않았다(`NOT_RUN`, 남은 일 참조). 대신 채택 6 route는 PREPARE Linux 생성물과 Windows 재생성 `parser.c`가 같아, 같은 도구 version의 생성 결과가 두 OS에서 같다는 관측이 있다.

## acceptance 연결

A01 `TestReproducePass`(두 작업 공간, output별 A↔B·기준 판정)와 26 route 실행; A02 `TestReferenceFindings`(기준 변경 → `MISMATCH`, 기준 없음 → `NOT_CLAIMED`, source 불변); A03 `TestJSONOnlyRun`(JS helper 변경 뒤 JSON 실행은 JS claim 없음, JS 실행은 `SOURCE_MISMATCH`); A04 `TestRunIdentityAndPoison`(abi·최적화·도구·header 변경 → 새 identity, 오염된 work 디렉터리·home 미사용); A05 `TestSourceWritesAndFailedPublication`(작업 공간 사본 쓰기, 실행 중 원본 root 변경, 실패 publication), `TestReproducePass`의 `OUTPUT_EXISTS`; A06 `TestBlockedBeforeLaunch`(runner·reproduce 둘 다; 작업 공간·결과를 만들지 않음); A07 `TestNonzeroExit`, `TestOutputLimits`, `TestFailureKinds`; A08 `TestTreeTermination`, `TestPipeHoldingDescendant`, `TestEscapedDescendant`, `TestEscapedQuietDescendant`; A09 `TestEvidenceAndCleanupFailures`(작업 공간·도구 사본 정리 실패, 결과 쓰기 실패); A10 `TestArgsDirEnvExact`, `TestReproduceCLI`; A11 `TestOutputLimits`(같음/하나 초과), `TestStorageLimit`(같음/한 byte 초과), `TestStorageLimitWorkspaceB`, `TestReproduceLimitCeilings`, 아래 예산 소비; A03·A04는 `TestJSClosureLeak`도 포함; A12 위 mutant 20종; A13 `TestCapabilityReceipt`와 세 OS CI(PR 단계); A14 `TestOfflineClosure`와 `PATH`를 비운 기존 CLI·외부 consumer 시험; A15 `TestReproductionRoutes`와 위 26 route 표; A16 `TestDefaultRequestPolicy`(90초·300초·4 edit·4 GiB·5초, 5 edit 거부), `TestDefaultBatchPolicy`, `TestFrameStatusMapping`, `TestFrameEscapedStdoutHolder`, `TestFrameCrashWithStdoutHolder`, `TestMemoryLimit`; A17 위 patch chain·재생성 절; A18 도구 identity 절과 `TestReproductionRoutes`의 digest·patch 상태 검사.

예산 소비(로컬): 다운로드 7,538,845 bytes(x/sys 2,094,997, tree-sitter Windows asset 3,778,280, npm tarball 2개 1,580,215, release metadata·SHASUMS 85,353)와 CI 스크립트 로컬 검증의 asset 재다운로드 3,778,280 bytes. 생성기 process 52회(26 route × 2)와 owned fixture 소량, 보존 출력 663,504,523 bytes(`.work/session-04/results`).

## 분리 context 리뷰와 처분

`24bdba5..8f8bad5`에 대한 분리 context 리뷰(EXECUTED_REVIEW, 별도 general-purpose subagent, Windows에서 vet과 focused test 실행; GitHub 승인 아님)는 BLOCKER 0, MATERIAL 4, MINOR 5, NOTE 4를 보고했다(`artifacts/.../session-04/review-r1.json`). 처분은 `a4c3fdb`다.

* MATERIAL R1-01: S04-A16의 단일 요청 정책(90초·300초·4 edit·4 GiB·5초)이 없었다 → `runner.DefaultRequestPolicy`와 `Apply`(5 edit 이상 실행 전 거부), 값 drift 시험.
* MATERIAL R1-02: `storage_bytes`를 관측값과 비교하지 않았다 → 실행 뒤 초과면 `STORAGE_LIMIT`·`RESOURCE_LIMIT`·`BLOCKED`, 같음/한 byte 초과 시험.
* MATERIAL R1-03: JS 실행이 `--work` 상위의 `node_modules`나 `--work/lib/node`에서 선언하지 않은 module을 찾을 수 있었다 → 실행 전 `JS_CLOSURE_LEAK`, 시험. 이번 26 route 실행 경로의 상위에는 둘 다 없음을 확인해 기존 근거는 유효하다.
* MATERIAL R1-04: process-group backend의 verified가 pipe를 잡지 않은 이탈 process를 덮는 것처럼 서술했다 → `cleanup.scope` 추가, 보고서·플랫폼 문구 정정, `TestEscapedQuietDescendant`.
* MINOR R1-05~R1-09: `Wait`가 supervise 종료를 기다림, 이미 회수한 process에는 종료 신호를 보내지 않음, interactive stdout을 grace 뒤 닫음(`TestFrameEscapedStdoutHolder`), 완료되지 않은 실행의 PASS 제거와 실행 중 원본 변경 시험, 도구 사본 정리 실패 보고, 읽기 pipe 닫기.
* NOTE R1-10: 위임 cgroup 부모에 쓰지 않고 읽기만 한다. R1-11: Windows 메모리 통지는 비동기이며 관측된 오분류가 없어 그대로 두고 위험으로 기록한다. R1-12: CI 단계는 PR CI에서 확인한다. R1-13: A17 표기 대응을 적었다.

수정 뒤 mutant는 새 4종(저장 용량 판정 제거, JS 누출 검사 제거, PASS 정리 제거, edit 상한 제거)을 더해 18/18 검출이다(`mutants-a4c3fdb.json`).

재리뷰(`8f8bad5..f0e38d2`, EXECUTED_REVIEW, `review-r2.json`)는 R1-01~05, 07~10, 13을 RESOLVED, R1-11·12를 ACCEPTED_RISK로 확인했고, R1-06을 NOT_RESOLVED(주 process가 스스로 끝났는데 이탈 process가 stdout을 잡으면 `RunBatch`가 멈춤)로, 새로 NOTE 1·MINOR 3을 보고했다. 처분은 `4afd455`와 시험 보강 `3c412ea`다.

* R1-06: interactive 주 process가 끝나면(종료 경로와 스스로 끝난 경로 모두) 남은 하위 process를 바로 종료하고 grace 뒤 stdout 읽기 쪽을 닫는다. 그 수를 `residual_after_exit`에 남긴다. `TestFrameCrashWithStdoutHolder`.
* R2-01: Stdout은 Wait 전에 읽고, 주 process가 끝나면 grace 뒤 닫힌다는 계약을 API 주석과 플랫폼 계약에 적었다.
* R2-02: 실행이 완료되지 않으면 `js_reproduction`·`json_regeneration`의 PASS를 `NOT_CLAIMED`로 바꾼다. 관측 claim은 그대로 둔다.
* R2-03: 저장 용량 초과는 다른 실패 원인을 가리지 않고 추가 finding이 되며, 다른 원인이 없을 때만 `RESOURCE_LIMIT`다. `TestStorageLimitWorkspaceB`(B만 초과).
* R2-04: `jsLeak`가 symlink를 푼 실제 경로의 상위도 검사한다.

`4afd455`에서 mutant 20종(새 2종: 주 process 종료 뒤 release 제거, 완료되지 않은 실행의 mode claim 정리 제거)은 18/20이었다. release 제거는 시험을 go test 제한 180초까지 멈추게 했고(결함이 드러났지만 시험이 실패로 끝나지 않음), mode claim 시험은 그 경로를 타지 않았다. 시험에 10초 자체 제한을 두고 source 변경 시험에서 mode claim을 확인하도록 고친 `3c412ea`에서 20/20이다(`mutants-4afd455.json`, `mutants-3c412ea.json` 보존).

재리뷰 r3(`f0e38d2..1af8d18`, `review-r3.json`)는 R1-06과 R2-01~04를 모두 RESOLVED로 확인했고, `TestStorageLimitWorkspaceB`가 B의 출력에 파일을 더해 결정성 실패로 먼저 끝나므로 "B만 넘고 나머지 claim은 PASS" 경로를 시험하지 않는다는 MINOR R3-01과 CLI 계약 문구 NOTE R3-02를 보고했다. fake generator가 B의 격리 home에만 64 KiB를 쓰도록 바꿔 출력은 같고 저장 용량만 넘게 했고(deterministic·reference PASS, `js_reproduction` NOT_CLAIMED 확인), 문구를 고쳤다(`f0f2423`). 이 commit에서 전체 로컬 명령 블록과 mutant 20/20(`mutants-f0f2423.json`)을 다시 확인했다.

## PR CI 실패와 수정

PR #63 최종 후보 CI run 37106058599 attempt 1에서 `foundation (ubuntu-24.04)`만 실패했다(windows-2025, macos-15 성공). `TestEscapedQuietDescendant`가 Linux cgroup backend에서 `Cleanup.Verified=false`였고, 원인은 `backend release failed: remove .../jobs/tsgk-4723-21: device or resource busy`였다. kit runner 결함이다. `cgroup.kill` 뒤 종료 중인 task는 `cgroup.procs`에서 먼저 빠지지만, 커널이 종료 과정에서 그 task를 cgroup에서 떼어 낼 때까지 `populated`에 남고, `cgroup.events`가 `populated 0`이 되기 전에는 leaf 삭제를 거부한다. runner는 `cgroup.procs`가 빈 시점을 tree가 빈 것으로 보고 곧바로 `rmdir`해 EBUSY가 났다. 같은 실행을 바꾸지 않고 다시 돌리지 않았다.

수정: cgroup backend의 tree 확인이 `cgroup.procs`가 비어도 `populated 0`이 될 때까지 비지 않은 것으로 센다. 그래서 Wait의 grace 상한 대기 안에서 해제를 기다리고, 상한 안에 해제되지 않으면 cleanup을 verified로 보고하지 않는다(시험과 Verified 요구는 그대로). `TestCgroupLiveUntilReleased`는 leaf 파일 상태(procs 비었고 `populated 1`)를 정확히 재현해 이 판정을 고정한다. 새 시험과 수정된 backend는 Linux 전용이라 이 Windows 호스트에서는 컴파일·교차 vet만 했고 실행은 다음 PR CI가 한다. 분리 context 재리뷰 r5(`review-r5.json`)는 수정을 맞다고 보고, 원인 서술이 특정 해제 시점(고아 process 회수)을 단정한 점(MINOR, 서술을 중립적으로 고침)과 해제 중인 task가 `residual_after_exit`에 들어갈 수 있다는 점(NOTE, 계약에 명시)을 지적했다.

## 남은 일과 한계

* 세 OS CI(A13, Linux cgroup·macOS sampled backend의 실제 시험 포함)는 PR 단계에서 실행한다. Linux·macOS backend 코드는 로컬에서 교차 vet만 했고 실행하지 않았다.
* 26 route의 Linux·macOS 재생성은 실행하지 않았다(`NOT_RUN`). hosted 실행에는 route source·npm 취득과 macOS용 Node 24.21.0 asset 취득이 필요하며, campaign heavy CI 예산 안의 별도 dispatch 결정이 필요하다.
* 비채택 20 route의 기준 drift(이전 CLI 생성물)는 기록만 했다. 채택 route의 `grammar.json`·`node-types.json`·`alloc.h`·`array.h` 기준은 PREPARE 기록에 없어 `NOT_CLAIMED`다.
* 저장 용량 상한은 실행 뒤 작업 공간 합계로 관측하며 실행 중 quota가 아니다. macOS 메모리는 sampled이고 hard cap이 아니다. process-group backend는 그룹을 떠난 하위 process를 포함하지 못한다. 그런 process가 출력 pipe를 잡고 있으면 cleanup은 verified가 아니지만, pipe를 잡지 않으면 관측되지 않으므로 verified의 범위는 `cleanup.scope=PROCESS_GROUP`(group만)으로 표시한다(`TestEscapedQuietDescendant`). 이 backend들은 적대적 native code 격리가 아니다.
* runner의 driver 쪽 60초 progress callback과 allocator hook은 S05 범위다. `go test -race`는 CGO 조건이 맞지 않아 실행하지 않았다.
