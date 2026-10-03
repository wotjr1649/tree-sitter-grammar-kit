# Session 02 — Strict Profile & Verification 결과

Campaign `TSGK-C1-20260929-R1`, 추적 `TSGK-C1-S02`, Issue #4, Milestone 3, branch `session/02-profile-security`(base main `7a513bd`). 이 보고서는 아래에 적은 revision과 로컬 관측만 설명한다. PR·세 OS CI·merge·post-merge 결과는 이 문서 작성 시점에 없으며 그 단계의 receipt가 소유한다.

## 구현 범위

* 하나의 strict decoder(`encoding/json/jsontext` token 단계와 typed 검사)와 문서 상한 `MaxDocumentBytes`. CLI와 공개 API가 같은 decoder를 쓴다.
* strict profile `tsgk-profile/r1`: 선택·역할·필수 여부·encoding 선언·더 낮은 한도. identity·corpus·verify에 적용하며 한도는 낮추기만 한다(corpus는 추적 `private-corpus-local` 값도 상한).
* caller 신뢰 expected `tsgk-expected/r1`: manifest r2와 개수·집합 hash 자기 일치 검사.
* `kit.Verify`/`tsgk verify`: 디렉터리(S01 discovery 또는 profile 목록)와 ZIP(`zip-r1`, 풀지 않음) subject를 expected와 비교해 `MISSING_REQUIRED`·`MISSING_OPTIONAL`·`UNEXPECTED_FILE`·`ROLE_CHANGED`·`SIZE_CHANGED`·`CONTENT_CHANGED`·`MODE_CHANGED`·`ENCODING_CHANGED`를 각각 보고하고 PASS(exit 0)/FAIL(exit 1)을 낸다.
* `zip-r1` 검사: EOCD·central directory·local header 일치, descriptor, 영역 겹침, CRC·크기·남은 압축 bytes, link·special·file/directory 충돌, 중복·대소문자 충돌, 이름 정책 `portable-names-r1`(ASCII 밖 이름 명시 거부), entry 수·archive 크기·member별·누적 해제량·path depth·명시 중첩 깊이 한도. 압축 폭탄은 읽기 전 선언 크기 검사와 streaming 중 선언 크기 초과 중단으로 막는다.
* 계약: [CLI/profile](../specs/cli-and-profile.md) `S02 구현` 절, [공개 API](../specs/public-go-api.md) `S02 함수와 추가 필드` 절, [trust](../specs/trust-and-execution.md) `zip-r1` 절, [identity/evidence](../specs/identity-and-evidence.md). 예시는 `src/contracts/examples/`(유효 2개, 거부 12개)이며 시험이 실제로 decode·거부한다.

제외: extraction(채택 계획이 요구하지 않아 UNAVAILABLE, 쓰기 capability 없음), ZIP64·암호화·다른 압축 방식·tar/gzip(명시 거부), Unicode 정규화 표(ASCII 밖 archive/expected 이름 거부로 대체), schema(S03), generator·native(S04~), npm package bytes 취득(S04).

## 관측 revision과 로컬 검사

코드 commit `3eab282`·`a4f336f`·`422c625`와 리뷰 수정 commit `40f62bc`·`1b45d8a`(base `7a513bd`)에서 Windows amd64, Go 1.27.1, `CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`로 실행했다. 아래 mutant·route·profile 입력·비공개 corpus 근거는 모두 최종 코드 commit `1b45d8a`의 build(sha256 `229b3501e79dfd244ba64c0473eae1e17c7d54713bd8f0ec3763db3660101cd8`)에 결속한다. 리뷰 전 build(`a4f336f`, `422c625`)의 같은 관측도 같은 결과였고 로컬 기록으로 남겼다. 세 OS CI는 PR 단계에서 따로 기록한다.

| 검사 | 결과 |
|---|---|
| `gofmt -l src`, `go vet ./src/...`, `go build ./src/...`, `git diff --check` | 통과 |
| `go test ./src/... -count=1` | `src/kit`, `src/cmd/tsgk`, `src/internal/foundation` 통과. Windows에서 기존 S01 시험 3건(실행 비트, special file, 대소문자 구분 이름)은 skip이며 Linux·macOS CI에서 실행된다 |
| 외부 consumer module(checkout 밖, `PATH` 비움) | inspect·identity에 더해 디렉터리·archive verify의 CLI와 API 결과 JSON bytes가 같고 오류 code가 같다 |
| 제품 dependency closure | `os/exec`, `net`, `plugin` 없음(`TestOfflineClosure`) |
| targeted mutant 20종(S02 13종 + S01 7종, `1b45d8a`) | 모두 컴파일되고 해당 시험이 의도한 진단으로 실패(아래) |

S02 mutant는 exact-set의 추가 파일 무시·내용 비교 제거, JSON 중복 key 허용, ZIP 중복 member 허용, streaming 선언 크기 검사 제거, 읽기 전 단일 파일 한도 제거, 누적 해제량 검사 제거, 영역 겹침·CRC 검사 제거, expected 집합 hash 검사 제거, profile 한도 상향 허용, trust 문서의 root 내부 검사 제거, `--out` no-clobber를 rename으로 바꾸기다. S01 7종은 S02의 Identity 재구성 뒤에도 그대로 검출된다.

acceptance 연결: A01 `TestVerifyDirectory`(PASS·선택 집합 identity)와 `TestExternalConsumerAndCLI`(CLI/API 일치); A02 `TestVerifyDirectory`의 제거·추가·rename·변경·크기·encoding·role 하위 시험과 `TestVerifyOptional`(미리 선언한 선택 파일); A03·A04 `TestStrictDocuments`, `TestExpectedDocument`, `TestContractExamples`; A05 `TestArchiveNormalLayouts`(Go writer descriptor·stored+최상위 디렉터리·수작업 무 descriptor, 빈 member, 원본 불변·파일 생성 없음)와 아래 route 관측; A06·A07 `TestArchiveRejects` 38 사례와 `TestVerifyDirectory`의 link 사례; A08 `TestArchiveLimits`(entries·archive bytes·file bytes·total·depth·files 각각 한도+1/같음/−1, 중첩 공유 budget과 깊이, 자동 탐색 없음)와 `TestArchiveBomb`(64 MiB로 풀리는 stream에서 할당 16 MiB 미만); A09 `TestArchiveSelfTrust`와 `TestVerifyCLI`(root 안 expected·profile 거부); A10 extraction 미채택(UNAVAILABLE, 아래); A11 외부 consumer, `TestVerifyCLI`, `TestArchiveSelection`(API 명시 선택도 디렉터리와 같은 규칙); A12 위 mutant; A13 S01 시험 전체 통과, S01 mutant 7종, 아래 26 route·비공개 corpus 동일성; A14 세 OS CI(미실행, PR 단계); A15 아래 profile 입력 관측; A16 `TestProfileLimits`, `TestCorpusProfile`, `TestVerifyCLI`의 corpus profile 사례.

## 26 route와 실제 archive 관측

S01이 결속한 26 route source 사본(`_ref/campaign-01-s01/sources`, 새 download 0)에 `1b45d8a` build를 실행했다. route마다 identity 결과로 expected를 만들어 source 밖에 두고, (1) 디렉터리 verify, (2) identity의 선택 목록을 담은 profile로 identity와 listed verify, (3) Python `zipfile`의 deflate·stored archive(`<route>-src/` 최상위 디렉터리, `--archive-root`) verify를 실행했다. 대용량 parser.c 다섯 route는 등록 예외(`large-parser-source-r1`, `pg-large-source-r1`)를 그대로 썼다.

* 26/26 route가 모든 경로에서 exit 0·PASS이고 actual 집합 hash가 expected와 같다. profile 선택 identity의 집합 hash도 discovery identity와 같다.
* 26/26 route의 identity 집합 hash가 S01 `d99ab52` route 관측과 같다(S02 재구성 뒤 회귀 없음).
* 대용량 다섯 route는 archive member에서도 그 parser.c 하나에만 `LARGE_FILE_EXCEPTION`을 낸다.
* .NET `ZipFile.CreateFromDirectory`(PowerShell 7.6.6) archive를 json·c·go route에 실행했다. 이름은 모두 `/` 구분이며 세 route 모두 PASS다.
* json route의 실제 archive에서 한 member를 바꾸면 `CONTENT_CHANGED`·`SIZE_CHANGED`(exit 1), 하나를 빼면 `MISSING_REQUIRED`(exit 1)다.

ZIP subject는 `.work`에서 다시 만들 수 있는 임시물이며 관측 뒤 지웠다. raw 결과는 로컬 `artifacts/sessions/campaign-01-2026-09-29/session-02/`에 있다.

## profile 입력으로 준 patch subject·T-SQL ESM·npm closure (A15)

* 채택 T-SQL 후보의 ESM module 24개(`grammar.js`와 `grammar/` 아래 `.js`)를 `files` 24·`file_bytes` 1048576·`total_bytes` 8388608·`depth` 8 profile로 선택했다. identity는 24개를 bytes로만 hash했고, 그 결과로 만든 expected와 listed verify는 PASS다. 가장 큰 module보다 1 byte 작은 `file_bytes`는 `FILE_BYTES_LIMIT`(exit 3), operation보다 큰 `file_bytes` 16777217은 `PROFILE_LIMIT_ABOVE_OPERATION`(exit 2)이다.
* 채택 route의 patch subject 5개(저장소 추적 파일)를 metadata로 선택한 profile은 identity가 완료됐고, `total_bytes` 268435457 profile은 `PROFILE_LIMIT_ABOVE_OPERATION`이다.
* npm closure는 추적 기록(`src/dev/prepare-p05/inputs.json`의 package·integrity)만 profile 입력으로 읽었다. package bytes는 로컬에 없고 S02 download 예산이 0이므로 받지 않았다(NOT_RUN, S04 재현성 의무).
* kit은 어느 입력도 실행·평가·fetch하지 않는다. 제품 closure에 process·network capability가 없고 profile에는 hook·명령·대용량 예외 이름 field가 없다(`JSON_UNKNOWN_FIELD`).

## 비공개 corpus `NET461-PHASE2-LOCAL-r1`

corpus 경로는 profile 적용을 위해 policy 구성만 재배치했고, 리뷰 수정은 Windows device 이름 규칙(`CON .txt`, `CONIN$`, 위첨자 COM/LPT)을 portable path에 더했다. `422c625` build(wall 153초)와 `1b45d8a` build로 `tsgk corpus`를 각각 한 번 실행했다. 두 실행 모두 summary·records 21451건·projects·policy·identities가 S01 `79f4f49` 결과와 같다. 자격 증명 성격 파일은 열지 않았고 공개 기록에는 개수·판정만 둔다.

## 분리 context 리뷰와 처분

`13d101a`에 대한 분리 context 정적 리뷰(STATIC_REVIEW, devflow reviewer subagent, 명령 미실행; GitHub 승인 아님)는 BLOCKER 0, MATERIAL 2, MINOR 5를 보고했다. 처분은 다음과 같다.

* MATERIAL: Unix mode의 symlink type 검사가 host 3에만 적용되어 OS X(host 19) entry의 symlink가 일반 파일로 통과할 수 있었다. 이제 external attribute 상위 bit의 Unix type을 host와 관계없이 검사하고, MS-DOS·NTFS·VFAT 속성의 reparse와 directory 속성 불일치도 거부한다(`1b45d8a`, 시험 4건).
* MATERIAL: archive subject에서 API의 `Selection.Files`를 검증하지 않아 중복·잘못된 role 목록이 PASS나 FAIL로 끝날 수 있었다. 디렉터리와 같은 `checkSelection`을 쓴다(`1b45d8a`, `TestArchiveSelection`).
* MINOR: 오류 보고가 map 순회 순서에 따라 달라지던 곳을 문서·정렬 순서로 고쳤다. corpus `--profile`의 encoding 규칙과 `excluded` 의미를 계약에 명시했다. 시험의 escape 중복 사례에 실제 escape를 넣었다. `CON .txt`·`CONIN$`·`CONOUT$`·위첨자 COM/LPT device 이름을 [Microsoft 이름 규칙](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file)대로 더했다.
* 자체 점검으로 CLI stderr 진단에 적대적 member 이름의 제어 bytes가 그대로 나가던 문제를 고쳤다(`40f62bc`, quoting). profile이 없을 때 S01 오류 우선순위가 바뀌지 않도록 Identity의 한도 검사 위치도 되돌렸다.

수정분 `13d101a..1b45d8a`의 재리뷰는 NO_FINDINGS였다. 재리뷰 NOTE는 처분이 필요 없는 관측이다. (1) device 규칙 추가로 S01 portable 판정이 조금 엄격해졌다. 위 최종 build 재실행에서 26 route와 corpus 결과는 바뀌지 않았다. (2) DOS 계열 host에서 원시 Windows 속성의 상위 bit(예: 0x20000000)를 쓰는 드문 writer는 `ARCHIVE_SPECIAL_REJECTED`로 거부될 수 있다. host 11은 APPNOTE대로 NTFS로 보지 않는다. (3) 명시 선택 목록을 읽기 전에 모두 검사하므로 여러 오류가 섞인 입력에서는 S01과 다른 오류가 먼저 보고될 수 있다.

## extraction (A10)

채택된 source 준비 계획은 S01이 tar.gz를 별도 승인 절차로 준비했고 S04 이후 재구성도 ZIP extraction을 요구하지 않는다. 그래서 extraction은 구현하지 않았고(UNAVAILABLE) 쓰기 capability·명령이 없다. A10의 기존 출력·redirect·쓰기 실패 사례는 대상 기능이 없어 실행하지 않았다. 구현이 필요해지면 새 caller-owned 목적지·no-clobber·부분 출력 분류를 갖춘 별도 작업이다.

## 남은 일과 한계

* 세 OS CI(A14)는 PR 단계에서 실행한다. macOS lane에서는 `t.TempDir()`가 `/var` 아래이므로 디렉터리·archive 시험 전체가 `/var` → `/private/var` root alias를 거친다.
* `go test -race`는 CGO를 쓰지 않는 조건이라 실행하지 않았다.
* 이름 정책은 archive member와 expected path를 ASCII로 제한한다. 26 route에는 ASCII 밖 이름이 없다(전수 확인). Unicode 이름이 필요한 사용처는 정규화 표 채택이 먼저다.
* archive에서는 role을 관측할 수 없다. members scope의 role은 expected 값을 쓴다(`member-role-observation` 미지원으로 표시).
* 기존 S01 `TestLimits/WALL_LIMIT`(1 ns wall)이 S02 첫 전체 실행에서 한 번 `WALL_LIMIT` 대신 성공으로 끝났다. 1 ns timer가 비동기로 만료되기 전에 작은 fixture 읽기가 끝날 수 있기 때문으로 보이며, 이후 `TestLimits`·`TestCorpusLimits` 200회 반복에서는 실패가 없었다. S02 변경과 무관한 시험 시간 의존성으로 기록만 한다.
