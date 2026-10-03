# Session 01 — Inventory & Identity 결과

Campaign `TSGK-C1-20260929-R1`, 추적 `TSGK-C1-S01`, Issue #3, Milestone 2, branch `session/01-inventory-identity`. 이 보고서는 아래에 적은 revision과 로컬 관측만 설명한다. PR·세 OS CI·merge·post-merge 결과는 이 문서 작성 시점에 없으며 그 단계의 receipt가 소유한다.

## 구현 범위

* 공개 offline API `src/kit`: `Inspect`, `Identity`, `Corpus`, `DefaultLimits`, `DefaultCorpusLimits`와 typed 결과·오류. 계약은 [공개 API](../specs/public-go-api.md)가 소유한다.
* CLI `src/cmd/tsgk`: `inspect`, `identity`, `corpus`. 인자 해석·JSON 표현·no-clobber `--out`·종료 코드만 담당한다. 계약은 [CLI/profile](../specs/cli-and-profile.md)의 `S01 구현` 절이다.
* manifest `tsgk-manifest/r2`와 집합 identity `tsgk-files/r2`(판별 encoding 결속), E0 report `tsgk-report/r1`, policy identity `tsgk-policy/r1`. 계약은 [identity/evidence](../specs/identity-and-evidence.md)다.
* known-paths discovery와 실행 없는 정적 closure 관측, no-follow guard(symlink·junction·reparse·special·hard link 거부), 유한 한도, kit wall(`RESOURCE_LIMIT`)과 caller 취소(`CANCELLED`) 구분, 읽는 중 원본 변경 검출.
* 실사용 source encoding 판별 1~6단계와 UTF-16 사전 검증. cp949 표가 필요한 판정은 `UNRESOLVED`/`ENCODING_TABLE_REQUIRED`로 S05에 넘긴다.
* 비공개 corpus `private-corpus-local` inventory: N461 역할 축(`UNCLASSIFIED` 포함), `PRESENCE_ONLY`, 제외 디렉터리, `.csproj` 선언 포함 관계, 줄바꿈·크기 등급·자동 생성 표시·내용 중복.

제외: strict profile·expected 검증(S02), archive, schema(S03), generator·native 실행(S04~), patched tree 재구성(S04), cp949 표와 decode(S05).

## 관측 revision과 로컬 검사

코드 commit `caa0e72`와 리뷰 수정 commit `0e1673d`(branch `session/01-inventory-identity`, base main `631c01e`)에서 Windows amd64, Go 1.27.1, `CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`로 실행했다. 세 OS CI는 PR 단계에서 따로 기록한다.

| 검사 | 결과 |
|---|---|
| `gofmt -l src`, `go vet ./src/...`, `go build ./src/...` | 통과 |
| `go test ./src/... -count=1` | `src/kit`, `src/cmd/tsgk`, `src/internal/foundation` 통과. Windows에서 Unix 전용 2건(실행 비트, FIFO)은 skip이며 Linux·macOS CI에서 실행된다 |
| 외부 consumer module(checkout 밖, `PATH` 비움) | CLI와 API 결과 JSON bytes 동일, 경로 탈출 오류 code 동일 |
| 제품 dependency closure | `os/exec`, `net`, `plugin` 없음 |
| targeted mutant 6종(`0e1673d`에서 재실행) | 파일 누락·고정 digest·CRLF 정규화·grammar 이름 하드코딩·link 추적·선언의 BOM 우선 위반 모두 컴파일되고 해당 시험이 의도한 진단으로 실패 |

acceptance 연결: A01 `TestExternalConsumerAndCLI`; A02·A03 `TestInspectLayouts`와 아래 route 관측; A04·A05·A06 `TestIdentityVector`(외부 Python 계산 vector와 시험 안 독립 preimage); A07 `TestGuards`; A08 `TestLimits`; A09 `TestCancellation`; A10 `TestOutPublication`; A11 `TestOfflineClosure`와 `PATH` 없는 실행; A12 `TestConcurrentRoots`·`TestGuards`; A13 위 mutant; A14 세 OS CI(미실행, PR 단계); A15·A16 `TestEncodingSteps`·`TestEncodingBoundIntoIdentity`; A17 아래 source 결속; A18·A19 `TestCorpusInventory`·`TestCorpusLimits`와 비공개 corpus 실행.

## 26 route 입력 관측

채택 6 route의 upstream base bytes는 PREPARE 보존 사본을 기록된 hash로 확인해 썼고, PostgreSQL `postgres/src/parser.c`는 보존된 LFS 실체(97664793 bytes, `pg-large-source-r1` 첫 identity)로 채웠다. 비채택 20 route는 `source-prepare`로 취득 전 목록(19 요청)을 기록한 뒤 고정 commit tarball을 받았고(8397488 bytes), 19개 모두 PREPARE 보존 archive와 byte 단위로 같았다. 이어서 registry가 선언한 26 route의 파일 전부를 bytes·sha256으로 결속했다(누락 0). 채택 route의 registry `patched_files` 10건 중 9건은 보존 candidate 사본과 hash가 같고, C# r5 `grammar.js`는 보존 candidate 사본이 없어 registry 값만 입력으로 기록했다. patched tree는 S01에서 재구성하지 않는다(S04).

아래는 CLI(`0e1673d` build; `caa0e72` build와 결과 같음)로 registry의 `grammar_subdirectory`를 선택해 얻은 관측이다. `UNRESOLVED`는 실행 없이 닫을 수 없는 참조(root 밖 module, literal로 해석하지 못한 `require`/`import` token 등)가 있다는 정직한 관측이며 결함 판정이 아니다. `registry 미선택`은 registry가 저장소 전체 기준으로 적은 형제 grammar의 scanner·헤더, `common.mak` 같은 build 파일처럼 선택 grammar의 관측 closure에 들어오지 않은 파일 수다.

| route | grammar | inspect closure | FOUND | identity | 선택 파일 | registry 미선택 |
|---|---|---|---|---|---|---|
| csharp (채택) | `.` | OBSERVED | 31 | RESOURCE_LIMIT (parser.c) | - | - |
| go | `.` | UNRESOLVED | 18 | COMPLETED | 18 | 0 |
| python | `.` | UNRESOLVED | 17 | COMPLETED | 17 | 0 |
| javascript | `.` | UNRESOLVED | 22 | COMPLETED | 22 | 0 |
| jsx | `.` | UNRESOLVED | 22 | COMPLETED | 22 | 0 |
| typescript (채택) | `typescript` | UNRESOLVED | 16 | COMPLETED | 16 | 5 |
| tsx (채택) | `tsx` | UNRESOLVED | 15 | COMPLETED | 15 | 5 |
| java | `.` | UNRESOLVED | 17 | COMPLETED | 17 | 0 |
| kotlin | `.` | UNRESOLVED | 13 | RESOURCE_LIMIT (parser.c) | - | - |
| c | `.` | OBSERVED | 19 | COMPLETED | 19 | 0 |
| cpp | `.` | UNRESOLVED | 31 | RESOURCE_LIMIT (parser.c) | - | - |
| rust | `.` | OBSERVED | 22 | COMPLETED | 22 | 0 |
| swift (채택) | `.` | UNRESOLVED | 24 | COMPLETED | 24 | 0 |
| dart | `.` | UNRESOLVED | 29 | COMPLETED | 29 | 0 |
| php | `php` | UNRESOLVED | 17 | COMPLETED | 17 | 5 |
| ruby | `.` | UNRESOLVED | 22 | COMPLETED | 22 | 0 |
| r | `.` | OBSERVED | 17 | COMPLETED | 17 | 0 |
| bash | `.` | OBSERVED | 16 | COMPLETED | 16 | 0 |
| powershell | `.` | OBSERVED | 27 | COMPLETED | 27 | 0 |
| html | `.` | OBSERVED | 14 | COMPLETED | 14 | 0 |
| css | `.` | UNRESOLVED | 15 | COMPLETED | 15 | 0 |
| json | `.` | OBSERVED | 11 | COMPLETED | 11 | 0 |
| yaml | `.` | OBSERVED | 22 | COMPLETED | 22 | 9 |
| xml | `xml` | OBSERVED | 14 | COMPLETED | 14 | 5 |
| tsql (채택) | `.` | UNRESOLVED | 69 | RESOURCE_LIMIT (parser.c) | - | - |
| postgresql-sql (채택) | `postgres` | OBSERVED | 25 | COMPLETED (`pg-large-source-r1`) | 25 | 4 |

inspect는 26 route 모두 완료했다. identity는 22 route에서 완료했고, C#(32021728 bytes), T-SQL(26649584), C++(25857209), Kotlin(22443237)의 upstream `src/parser.c`가 제품 기본 단일 파일 한도 16777216 bytes를 넘어 `RESOURCE_LIMIT`(exit 3)로 끝났다. 이것은 채택 계약대로의 동작이며 S01은 한도를 올리지 않았다. 후속 Session이 이 4개 identity의 전체 manifest를 필요로 하면 `pg-large-source-r1`과 같은 identity 한정 예외가 사용자 결정으로 필요하다. 해당 파일의 bytes는 위 결속 기록에 있다.

## 비공개 corpus `NET461-PHASE2-LOCAL-r1`

로컬 Windows에서 `tsgk corpus`(기본 `private-corpus-local` 한도, profile cp949)를 한 번 실행했다. 경로·이름·hash가 담긴 결과는 추적하지 않는 로컬 artifacts에만 있고 여기에는 개수와 판정만 적는다. 내용 bytes·연결 문자열은 출력하지 않았고 자격 증명 성격 파일과 vendor binary는 열지 않았다.

최종 실행은 `0e1673d` build로 `execution_status=COMPLETED`, `evidence_mode=NEW_RUN`, `assessment=NOT_ASSESSED`, wall 35초(kit wall 1800초 안; 같은 corpus의 첫 실행은 파일 cache가 비어 444초)였다. `caa0e72` build의 실행과 집계가 같다. 실행 단위 finding은 0건이다.

| 항목 | 값 |
|---|---|
| 파일 = record | 21451 (한도 26000) |
| 읽은 bytes | 2613288485 (한도 3489660928) |
| 잘라낸 제외 디렉터리 | 96 |
| `COMPLETED` / `PRESENCE_ONLY` | 21318 / 133 (자격 증명 성격 28, vendor binary 105) |
| 단일 파일 한도 초과 record | 0 (33554432 bytes 이하 최대 등급 `LE_32MIB` 4건) |
| 역할 | `N461-FORM` 4988, `N461-RESOURCE` 2528, `N461-XML` 854, `N461-DX202` 154, `N461-WCF-CS` 88, `N461-SVC` 20, `UNCLASSIFIED` 12819 |
| route | csharp 6624, tsql 6150, xml 5163, svc 20, unrouted 3494 |
| encoding `PASS` | UTF-8 BOM 14631, UTF-8 검증 1912, UTF-16LE BOM 2789 |
| encoding `UNRESOLVED`(`ENCODING_TABLE_REQUIRED`) | 694 (cp949 후보 668, UTF-8과 cp949 모두 가능 26) |
| encoding `BLOCKED` | 1292, 모두 `NUL_WITHOUT_BOM`(이미지·PDF·글꼴 등 binary 형식) |
| 줄바꿈 | CRLF 19197, LF 512, MIXED 270, NONE 47, UNKNOWN 1292 |
| 자동 생성 표시 | 2664 |
| 내용 중복 | 926 묶음, 중복 사본 2465 |
| `.csproj` | 137 모두 해석, 선언 item `MEMBER` 13205·`NOT_FOUND` 258, `Import` 209, 파일이 아닌 item 3044 |

파일 수 21451, 자격 증명 성격 28, vendor binary 105, `.csproj` 137은 PREPARE의 사전 관측과 같다. PREPARE가 적은 "목록 항목 누락 666"은 집계 규칙이 기록되지 않은 값이며, 여기의 258은 위 계약 규칙(파일 item 종류만, 디렉터리 item 제외)의 결과다. 첫 실행(커밋 전 build)은 디렉터리를 가리키는 `WCFMetadataStorage` 42건을 `NOT_FOUND`로 셌고, 이를 파일 item에서 빼는 수정 후 다시 실행했다. 두 실행의 원자료는 모두 로컬에 보존했다. `ENCODING_TABLE_REQUIRED` 694건은 S05가 cp949 표를 pin한 뒤 판정한다.

## 분리 context 리뷰와 처분

`caa0e72`·`5933c13`에 대한 분리 context 정적 리뷰(STATIC_REVIEW, 실행 없음)는 BLOCKER 0, MATERIAL 1, MINOR 6을 보고했다. 모두 `0e1673d`에서 고치고 회귀 시험을 더했다.

| 등급 | finding | 처분 |
|---|---|---|
| MATERIAL | corpus 파일별 encoding 선언이 대소문자만 다르면 오류 없이 적용되지 않음 | 선언은 record path와 정확히 일치해야 하며 아니면 `DECLARATION_UNMATCHED`. 선언 적용(`source=DECLARATION`) 시험 추가 |
| MINOR | `--out` 입력 내부 검사를 junction 별칭으로 우회 | 부모와 조상의 파일 identity를 root와 비교. junction·symlink 별칭 root 시험 추가. root 자체의 alias는 `Stat`으로 해석 |
| MINOR | corpus hard link 하나가 실행 전체를 끝냄 | 그 record만 `UNSUPPORTED`와 `HARDLINK_REJECTED` finding |
| MINOR | `.csproj`의 `Choose`·`Target`·`Exclude` item을 `MEMBER`로 셈 | `UNRESOLVED`. fixture에 세 경우 추가 |
| MINOR | 한도 깊이의 빈 디렉터리가 `DEPTH_LIMIT` | 디렉터리도 경로 segment 수로 비교. 경계 시험 추가 |
| MINOR | CLI `PATH=VALUE`를 첫 `=`에서 분리 | 마지막 `=`에서 분리. `=`가 든 path 시험 추가 |
| MINOR | A15 finding code가 `ENCODING_TABLE_REQUIRED`가 아님 | finding code를 record code와 같게 |

수정분 재리뷰는 6건을 해결로 확인했고, `--out` junction 건이 root 하위 디렉터리를 가리키는 junction에서 남는다는 점과 새 MINOR 2건(UNC를 가리키는 symlink root, 대소문자 구분 filesystem의 index 충돌)을 보고했다. 경로에 해석되지 않은 junction·mount point가 남으면 `OUTPUT_PARENT_ALIAS`로 거부하고, 해석된 root도 UNC/device 검사를 하며, corpus 선언은 정확한 path 집합으로, `.csproj`는 정확 일치 우선·단일 대소문자 무시 일치만 `MEMBER`로 고쳤다. 각 수정에 시험을 더했다(대소문자 구분 시험은 Windows·macOS 기본 filesystem에서 skip).

2차 재검토는 남은 3건을 해결로 확인하고 MINOR 2건을 더 보고했다. cloud placeholder 같은 일반 reparse 디렉터리를 별칭으로 오인해 `--out`을 거부하던 점은 Windows reparse tag(mount point·symlink)로 좁혀 고쳤다. `subst`·bind mount처럼 경로에 드러나지 않는 별칭은 표준 라이브러리로 검출하지 않으며, 이 한계를 CLI 계약에 적고 아래 남은 일에 남겼다.

리뷰가 남긴 정의 문제(discovery 기반 identity가 같은 source를 두 번 읽어 `total_bytes`에 두 번 셈)는 CLI 계약에 "실제로 읽은 bytes 합계"로 명시했다.

## 남은 일과 한계

* 세 OS CI, PR, 분리 context 리뷰 처분, merge, post-merge 검증과 tracking은 이 보고서 이후 단계다.
* JS closure는 실행 없이 완전성을 증명하지 않는다(`js-closure-proof` unsupported). C include는 포함 파일 디렉터리와 `G/src`만 본다.
* `--out`의 입력 내부 검사는 `subst` drive·bind mount 같은 경로 밖 별칭을 검출하지 못한다(CLI 계약에 한계로 명시, 후속 개선 후보).
* 읽는 중 변경 검출은 관측 가능한 변경(크기·수정 시각·파일 identity·읽은 bytes 수)만 다루며 적대적 동시 교체를 막는 sandbox가 아니다.
* cp949 표가 필요한 판정과 decode는 S05, strict profile·expected 검증은 S02다. N461 역할의 내용 표지는 ASCII bytes 검색이므로 UTF-16 파일에서는 이름 규칙만 적용된다.
* `src/contracts/campaign-01.json`의 Session 구현 상태는 foundation 시험이 PREPARE 값(`NOT_IMPLEMENTED`)으로 고정하고 있어 이 Session에서 바꾸지 않았다. 갱신 시점은 통합 후 결정 사항이다.
