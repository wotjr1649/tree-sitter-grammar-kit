# #233 kit 내부 identity 채택 경로

기준 main은 `45711c0052d1256d10bdb63a866af8bb45bea126`이다. 이 조사는 소비자 연결이나 새 인증 체계를 추가하지 않고, 기존 entry point에서 기대 identity가 결과 채택까지 이어지는 경로를 확인한다. hash는 bytes의 결속이며 게시자·실행자의 정직성을 인증하지 않는다.

## 경로와 검증 소유자

| 채택 경로 | 기대값과 실제 결속 | 실패를 막는 지점과 검사 |
|---|---|---|
| `Identity` / `identity` | 선택한 source-set의 path·role·mode policy·원본 bytes·encoding policy | 집합 hash 재계산, `TestIdentityVector`, `TestEncodingBoundIntoIdentity` |
| `Verify` / `verify` | caller가 별도로 신뢰한 expected와 실제 선택 집합 | owning [identity 계약](../specs/identity-and-evidence.md)의 manifest 대조. 같은 root에서 즉석 생성한 manifest를 출처 인증으로 사용하지 않음 |
| native build / build 재사용 | 등록 source/shared/scanner closure·runtime manifest·compiler bytes·flags·driver·실행 파일 | `NewBuild`의 검증과 실행 직전 executable 대조, `TestBuildIdentity`, `TestBuildCache`. cache는 계산 재사용이며 과거 실행 결과 승계가 아님 |
| `VerifyOracleSet` / record set | manifest가 나열한 정확한 member·case·input bytes/hash·step 0 source | `checkRecord`, `TestVerifyOracleSet`. set 무결성만으로 raw 재판정이나 query 의미 정확성을 주장하지 않음 |
| `Replay` / native·oracle-set reducer | 신뢰 registration의 workload·operation·producer·compiler·runtime·policy·query/fact identity와 실제 record/tree | `TestReplayStaleIdentity`, `TestReplayOracleSet`, `TestReplayRecordEnvelope`. 불일치·누락 identity와 rehashed projection의 잘못된 claim을 거절 |
| logger source witness replay | 초기 source와 편집별 source·좌표·관측 step·runtime policy | `TestReplayReuseRejectsRehashedMutations`, `TestReplayReuseWithoutWitnessBindsInitialWorkload`. witness 부재의 point 검증은 `UNRESOLVED`이며 정상 관측으로 승격하지 않음 |
| `VerifyEvidence` / `evidence verify` | caller-trusted policy의 graph anchor·required identity, graph의 11개 role·참조·carry·eligibility | `TestVerifyEvidence`, `TestEvidenceReplaySubjectPlatform`, `TestEvidenceReplayPlatformCLI`. 아래 재현한 platform 공백을 같은 verifier에서 수정 |
| `Qualify` / `qualify` | 독립 inventory의 grammar·query·profile·case·기대값과 현재 candidate·host·run/attempt cohort | `TestQualifyCohort`, `TestQualifyEligibility`, `TestQualifyPlatformClaims`. 다른 run/attempt·architecture·grammar·query, historical/replay/carry는 current cell을 채우지 못함 |
| hosted integration gate | workflow가 정한 실제 run/attempt/checkout와 공식 artifact 이름·ID·ZIP digest, qualification/large 결과 | [validation 계약](../validation/validation.md)의 fan-in. 같은 run 원본을 재검증하며 missing/failed/skipped/cancelled producer를 성공으로 바꾸지 않음 |

`VerifyEvidence`를 모든 경로에 중복 호출하지 않는다. native build, replay, qualification은 각 owning 계약의 타입과 기대값으로 결속한다. source-set hash, native build hash, source commit은 서로 다른 identity이며 같은 문자열이라고 가정하지 않는다. graph의 `replay-result` 첨부가 없으면 그 첨부를 검증했다고 주장할 수 없다.

## 재현한 결함과 수정

기존 `checkReplayFile`은 첨부 report의 subject run·attempt만 graph subject와 대조했다. graph subject를 `windows/amd64`, run `10`, attempt `1`로 고정한 뒤 첨부 report의 platform만 `linux/amd64` 또는 빈 문자열로 바꾸고 member hash와 graph anchor를 정상 갱신하면 evidence 검증이 `PASS`였다. 파일 hash 손상과 구분되는 내부 identity 불일치다.

같은 함수에서 첨부 `subject.platform`을 읽어 비어 있지 않고 graph subject와 정확히 같은지 검사한다. 불일치·누락·빈 문자열·null은 `REPLAY_RESULT_MISMATCH`다. graph와 첨부를 모두 빈 platform으로 만든 경우도 거절한다. 실제 호스트 값으로 채우거나 OS만 비교하지 않는다. 정상적인 동일 platform의 실패 report는 유효한 실패 근거이므로 evidence 검증 `PASS`를 유지한다. 실행 판정과 근거 정합성을 혼동하지 않는다.

## 관측과 다음 경계

Windows/amd64, Go 1.27.2, PowerShell 7, `CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOFLAGS=-mod=readonly`에서 수정 전 platform 반례의 실패를 확인했다. 수정 후 관련 API·CLI 검사, kit 전체 검사(40.672초), 고정 runtime과 GCC 16.2의 `TestBuildIdentity`(40.568초), kit·CLI의 `go vet`과 `go build ./src/...`가 통과했다. 새 보고서 링크를 포함하는 source-only fixture 누락을 발견해 기존 fixture 목록을 보완했고 `TestRepositoryFiles`·`TestNegativeControls`·`TestSourceOnlyDoesNotDiscoverParentGit`과 foundation package 전체(72.322초)도 통과했다. CI의 Go 1.27.1·Linux/macOS 실제 실행은 이 로컬 결과와 구분한다. 새 후보 PR/actual-main 검증은 통합 전에 별도로 필요하다.

이 목록은 위 채택 경로의 검증 소유자를 정리한 것이며 모든 가능한 입력의 정확성 증명이 아니다. query predicate 재계산·claim(#216), old-tree/reuse 증명(#217), C# 전처리 context(#218), T-SQL 정확한 엔진 oracle(#219)은 각각의 합격 기준으로 진행한다. 출력 namespace 동시 교체(#215)는 파일 소유권 경계이며 identity hash 검사로 해결되지 않는다. 이들을 단순히 미지원으로 바꾸거나 일부 대표 대상의 PASS로 일괄 종결하지 않는다. #233의 전체 완료 대상은 등록 26 route와 추가 형식·workload, 공개 offline API/CLI 및 experimental 경로가 선언한 지원 의무다. 적용 범위는 기존 owning 계약을 따르며, 일부 대상의 우선 실행은 전체 완료 범위를 줄이지 않는다.
