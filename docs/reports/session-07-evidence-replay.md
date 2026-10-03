# Session 07 — Evidence & Replay 결과

Campaign `TSGK-C1-20260929-R1`, 추적 `TSGK-C1-S07`, Issue #9, Milestone 8, branch `session/07-evidence-replay`(base main `dc2430a`). 이 보고서는 아래에 적은 revision과 로컬 관측만 설명한다. PR·세 OS CI·merge·post-merge 결과는 이 문서 작성 시점에 없으며 그 단계의 receipt가 소유한다.

## 구현 범위

* **`tsgk replay`와 registration `tsgk-replay/r1`**: 계약은 [CLI/profile](../specs/cli-and-profile.md)과 [identity/evidence](../specs/identity-and-evidence.md)의 `S07 구현`이다. evidence set root를 S01 guard로 한 번 나열하고, registration의 독립 member inventory·identity·expected record로 등록 reducer 하나를 실행한다. 결과 `tsgk-replay-result/r1`은 E0에 subject(원 run, 바꾸지 않음), raw의 기록 판정, 다시 계산한 판정, gate별 `REPLAYED_RAW`/`RECORDED_NOT_RECOMPUTED` 수, 소비 계정(누락·중복·미사용·순서·다른 cohort), 관측 identity, 한국어 설명을 더한다.
* **등록 reducer 다섯 개**(`kit.Reducers`): `native-result-r1`(S05 결과), `oracle-set-r1`(S06 기록 set), `private-corpus-r1`(`NET461-PHASE2-LOCAL-r1` 로컬 실행), `prepare-native-r1`(PREPARE probe bundle), `bs-gate-compare-r1`(BrightScript `S07-REPLAY-2ULP-r1` 비교). 앞의 세 개는 같은 사례 reducer로 S05·S06의 공개 비교 함수(`CompareTrees`, `TreeDigest`, `ValidateTree`, `CompareCaptures`)를 다시 쓴다. 새 JSON normalizer, plugin 평가, 보관 코드 실행은 없다.
* **`tsgk evidence verify`와 graph `tsgk-evidence/r1`·policy `tsgk-evidence-policy/r1`**: run·replay·carry·record node의 세 축, 참조·순환, 파일 전수, replay와 subject의 분리, 이전 FAIL과 새 PASS의 분리(relabel 거부), 기본 거부 승계(`unchanged-dependency-r1`만), 필수 node identity, `NEW_RUN` 자격을 검사한다.
* **공개 offline API**: `Replay`, `ParseReplayProfile`, `ReplayOperations`, `Reducers`, `CompareGates`, `VerifyEvidence`([공개 API](../specs/public-go-api.md) `S07 함수`). CLI는 같은 함수를 부른다(`TestReplayCLI`가 같은 bytes를 확인). 공개 closure에 runner·`os/exec`·network가 없음은 `TestOfflineClosure`가 계속 확인한다.
* **연산**: `evidence-replay`(S07 예산 값, 세 OS)와 `private-corpus-replay`(NET461 등록부의 S07 비공개 replay 값, 로컬). 큰 raw는 hash하며 한 번 stream으로 읽고 record 값을 하나씩 decode한다. 필요한 member가 한도를 넘으면 읽지 않고 `RECORDED_NOT_RECOMPUTED`다.
* **계약 예시**: `src/contracts/examples/replay-r1.json`, `evidence-policy-r1.json`과 거부 예시 `invalid/replay-*`, `invalid/evidence-*`(`TestContractExamples`).
* S06 `decodeOracleManifest`는 이제 manifest의 workload·producer·policy·comparator·query·fact pack 값을 채운다(검사 규칙은 같다). replay가 그 값을 identity로 결속한다.

제외: 임의 verifier 실행·archive 안 코드 import, 범용 DSL, 전역 tolerance, BrightScript 17 gate의 raw 재계산(보관된 Python verifier), S06 API 관측 재계산(raw 응답의 protocol decode는 공개 API 밖), 게시자 인증, tool 설치, 다른 저장소 변경.

## 다시 계산하는 것과 기록으로 남는 것

| reducer | 다시 계산 | 기록으로 남음(현재 판정 UNRESOLVED) |
|---|---|---|
| `native-result-r1` | case 결속(tree identity 포함), 상태-판정 일관성, SVC 관측 전용 사례 판정, full tree 구조·digest·node 수·`has_error`, incremental/fresh 비교, route 증명, 기대값, claim과 판정, summary 수와 run 판정, response 이름 결속 | 응답 payload, r1 full tree의 declarations 기대값, record·summary 형식 tree의 digest |
| `oracle-set-r1` | 위 항목, record 완결성, set 사례 목록·record 수·set 판정, incremental/fresh query 비교 | API claim, query 기대값, 사실 재현, 동적 SQL, UNSUPPORTED query의 구조 stream |
| `private-corpus-r1` | corpus route 묶음 결속, 사례 판정, 파일 projection, summary 수, 로컬 실행 identity | record 형식 tree의 digest |
| `prepare-native-r1` | 한 producer의 등록 row 전수, raw stdout 결속, probe 판정에서 다시 계산한 exit, 기대 syntax 종류 | 등록 fact check와 edit 창(PREPARE 리뷰 helper), 크기만 대조한 inventory 항목, 없는 raw |
| `bs-gate-compare-r1` | `S07-REPLAY-2ULP-r1` 비교 | raw에서 gate를 다시 계산하는 일 |

## 로컬 검사(Windows, `2130380`과 그 뒤 작업 트리)

* `gofmt -l src`, `go vet ./src/...`, `go build ./src/...`, `go test ./src/... -count=1`: 통과(`CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOFLAGS=-mod=readonly`).
* 새 시험: `TestReplayNativeResult`, `TestReplayConsumption`, `TestReplayStaleIdentity`, `TestReplayMutantControls`, `TestReplayUnsupported`, `TestReplayGuards`, `TestReplayLimitsAndCancel`, `TestReplayDeterministic`, `TestReplayOracleSet`, `TestReplayPrivateCorpus`, `TestReplayPrepareNative`, `TestCompareGates`, `TestReplayGateCompare`, `TestVerifyEvidence`(`src/kit`), `TestReplayCLI`(`src/cmd/tsgk`). fixture는 시험이 임시 디렉터리에 만들며 tool·target 실행이 없어 세 OS foundation job에서 그대로 돈다.
* **targeted mutant 23개 모두 검출**(`.work/session-07/mutants.py`, 결과는 로컬 artifacts `mutants-2130380.json`): 미사용 record 무시, PASS label 신뢰, 모르는 reducer를 지원 reducer로 대체, 일부 기록 gate를 replay로 표시, 기록 claim에 기대는 사례를 PASS로, 반올림 비교, 값 비교(+0 = -0), 전역 4 ULP, threshold 구간 미검사, 최대값 항목 미비교, 허가 없는 승계, comparator를 뺀 승계, `NEW_RUN` 자격 미검사, relabel 미검사, summary 신뢰, digest 신뢰, 목록 밖 파일 무시, identity 미비교, tree identity 혼합 허용, PREPARE exit 신뢰, 다른 cohort row 소비, projection 신뢰, dirty 실행 허용. 모두 의도한 시험이 그 code로 실패했고 compile 실패로 대신한 것은 없다.

## S06 기록 set replay(로컬 ci-sim 출력)

S06의 Windows ci-sim 출력 29개 기록 set(26 route, csharp-svc, 대용량 두 개)을 `oracle-set-r1`로 replay했다. registration의 identity와 기록 판정은 첫 실행에서 관측한 값이라 독립 anchor가 아니다(로컬 receipt `s06-set-replays-r1.json`).

* 28개 set은 evidence가 유효했다(member·identity·record 결속·소비 문제 0, claim 불일치 0). 사례 166개를 소비했고 gate 재계산은 tree 836, incremental/fresh 비교 494, route 494, 기대값 644, query 비교 494건이며 실패 0이다.
* API claim 152건, query 기대값 26건, 사실 재현 50건, 동적 SQL 2건은 `RECORDED_NOT_RECOMPUTED`다. 그래서 기록이 PASS인 set의 다시 계산한 판정은 `UNRESOLVED`다. 기록 FAIL인 csharp·swift·tsx·typescript는 다시 계산한 claim에서도 FAIL이고, 기록 FAIL인 csharp-svc·python·tsql은 실패 claim이 API뿐이라 `UNRESOLVED`다(기록 FAIL은 그대로 `recorded`에 남는다). 한도 초과 사례 set은 기록과 같은 `BLOCKED`다.
* 대용량 set 하나는 record 하나가 16 MiB를 넘어 `RAW_OVER_LIMIT`, `RECORDED_NOT_RECOMPUTED`/`UNRESOLVED`다.

## 비공개 corpus(A15, Windows 로컬, 개수만)

`NET461-PHASE2-LOCAL-r1`의 S05 로컬 raw 세 실행을 `private-corpus-r1`/`private-corpus-replay`로 replay했다. replay 실행 자신의 로컬 실행 identity(후보 commit `2130380`, clean tree, host Windows 11 AMD64, `tsgk` sha256, Go 1.27.1)와 각 subject 실행의 identity(후보 commit, clean tree, host, compiler `75e87953…`, runtime `659cda7c`)를 로컬 receipt에 남겼다. 경로·이름·hash가 담긴 registration·결과·receipt는 추적하지 않는 로컬 artifacts에만 있다. member bytes는 S07에서 처음 결속했다(S05가 그 bytes의 hash를 따로 남기지 않았다).

* `81a0538`(S05 보고 실행): **PASS**, `REPLAYED_RAW`, wall 약 1.3초, 읽은 bytes 210254984. inventory 21451 record와 route 파일 17957개를 모두 한 번씩 소비했다(expected 35914 = 파일 record 17957 + projection 17957, 누락·중복·미사용 0). route 묶음 결속 4, 사례 결속·상태·판정 17957, incremental/기대값 17937, projection 17957, summary 14, 로컬 실행 identity 7 검사가 모두 통과했다. 다시 센 값은 S05 보고서와 같다: COMPLETED 17957, `NOT_RUN` 0, `has_error` 230(csharp 48, tsql 181, xml 1), `PRESENCE_ONLY` 133, route 없음 3494. record 형식 tree digest 17937건은 nodes가 없어 기록으로 남는다(판정은 digest에 기대지 않는다).
* `58f079b`: **FAIL**(`SUMMARY_MISMATCH` 1건). 이 실행의 summary는 수정 전 helper가 쓴 `has_error` key `svc:`(값 없는 관측)을 담는다. 지금 등록한 projection 규칙은 `svc:null`이다. 사례·projection·소비는 모두 통과했다. 옛 형식을 새 형식으로 고쳐 읽지 않는다.
* `fb8934a`: **FAIL**(`STATUS_TREE_INCONSISTENT` 20건). 이 build는 SVC 관측 전용 step에 `incremental: null` 대신 빈 tree 객체를 썼다(같은 `tsgk-incremental-result/r1` 안의 형식 차이, `81a0538`에서 고침). replay는 그 20개를 완료 tree가 없는 COMPLETED 사례로 거부하고 다시 해석하지 않는다.

## PREPARE native 증거(A16, 로컬, gitignored raw)

`language-sources.json` adoption의 `native_evidence` run 다섯 개(36795440494는 typescript·tsx 두 cohort)와 PostgreSQL baseline 재사용 run 36820514519를 `prepare-native-r1`로 replay했다. registration은 bundle의 `evidence-manifest.json`과 `records/case-ledger.json`을 결속하고, expected record는 그 run의 등록 facts 파일에서 채택 producer의 row다. raw는 모두 있고 읽은 member는 모두 16 MiB 안이었다. bundle의 16 MiB 초과 파일(생성 `parser.c` 등)은 inventory 대조에서 크기만 확인하고 읽지 않았다.

| run | route | 등록 row | 소비 | exit·syntax 재계산 | 기록으로 남음 | 결과 |
|---|---|---|---|---|---|---|
| 36917832850 | csharp | 27 | 27 | 실패 0 | fact 27, edit 5 | `REPLAYED_RAW`/`UNRESOLVED` |
| 36795440494 | typescript | 4 | 4 | 실패 0 | fact 4, edit 1 | `REPLAYED_RAW`/`UNRESOLVED` |
| 36795440494 | tsx | 8 | 8 | 실패 0 | fact 8, edit 1 | `REPLAYED_RAW`/`UNRESOLVED` |
| 36741763343 | swift | 14 | 14 | 실패 0 | fact 14, edit 3 | `REPLAYED_RAW`/`UNRESOLVED` |
| 36947507308 | tsql | 41 | 41 | 실패 0 | fact 41, edit 1 | `REPLAYED_RAW`/`UNRESOLVED` |
| 36882649292 | postgresql-sql | 12 | 12 | 실패 0 | fact 12, edit 1 | `REPLAYED_RAW`/`UNRESOLVED` |
| 36820514519 | postgresql-sql baseline | 4 | 4 | 실패 0 | fact 4 | `REPLAYED_RAW`/`UNRESOLVED` |

* 110개 등록 row 모두 inventory·ledger·raw 결속이 맞고, raw tree에서 다시 계산한 probe 판정이 기록된 exit와 같으며 기대 syntax 종류와 원본 tree의 오류 여부가 같다. 다른 producer의 ledger row는 소비하지 않고 `excluded`로 셌다.
* 원 PREPARE 판정 `ALL_REGISTERED_ROWS_AND_EDITS_PASS`는 등록 fact check와 edit 창 판정에 기대며, 그 판정은 run마다 쓴 PREPARE 리뷰 helper(PowerShell)의 규칙이라 이식하지 않았다. 그래서 다시 계산한 판정은 `UNRESOLVED`이고 원 판정은 기록으로만 남는다.
* SQL Server runtime `tsql-R01`은 실행하지 않았고 raw도 없어 `NOT_RUN`이다.

## BrightScript historical subset

`S07-REPLAY-2ULP-r1`은 BrightScript v0.1.4 source에 보관된 `replay_compare.py`를 옮긴 비교로 구현하고, 경계·signed zero·threshold·최대값 항목 fixture로 검사했다(`TestCompareGates`). v0.1.2 verification bundle(8875 run raw)은 이 저장소에 없고 S07 다운로드 예산이 0이라 내려받지 않았다. 그래서 그 historical raw의 replay는 실행하지 않았으며(`NOT_RUN`), 17 gate의 raw 재계산은 보관된 Python verifier가 필요해 지원하지 않는다(`raw-recompute` gate는 언제나 `RECORDED_NOT_RECOMPUTED`).

## S01–S06 기록과 차이(A13)

* S01 E0(`tsgk-report/r1`)를 확장했고 새 report 체계는 없다. S05 결과·S06 기록 set·S01 corpus inventory는 schema를 바꾸지 않고 그대로 읽는다.
* S06 manifest 해석 함수는 값을 채우도록 바뀌었을 뿐 검사 규칙은 같다(`TestVerifyOracleSet` 등 기존 시험 통과).
* S05 비공개 raw의 helper·build 형식 차이 두 가지(`58f079b` summary key, `fb8934a` SVC 빈 tree)는 위와 같이 거부로 남겼다. 다시 해석하거나 옛 기록을 고치지 않았다.

## acceptance 연결

| ID | 근거 |
|---|---|
| S07-A01 | `TestReplayNativeResult`, `TestReplayOracleSet`, 로컬 S06 set·비공개·PREPARE replay |
| S07-A02 | `TestReplayConsumption`, `TestReplayPrepareNative`, `TestReplayPrivateCorpus`(미사용 projection) |
| S07-A03 | `TestReplayStaleIdentity`, `TestReplayOracleSet`(comparator), `TestVerifyEvidence`(run·attempt·policy) |
| S07-A04 | `TestReplayUnsupported`, `TestReplayOracleSet`(API 기록), `TestReplayGateCompare` |
| S07-A05 | `TestReplayGuards`, `TestVerifyEvidence`(순환·경로), S02 archive guard 시험(중첩 archive 한도는 S02 소유) |
| S07-A06 | `TestCompareGates` |
| S07-A07 | `TestVerifyEvidence`(relabel), replay의 `subject`·`recorded`·`recomputed` 분리 |
| S07-A08 | `TestVerifyEvidence`(허가·dependency·관계·판정) |
| S07-A09 | `TestVerifyEvidence`(`ELIGIBILITY_REJECTED`) |
| S07-A10 | `TestReplayNativeResult`(`PATH` 비움), `TestOfflineClosure`, `TestReplayCLI` |
| S07-A11 | `TestReplayLimitsAndCancel`, `TestReplayCLI`(`OUTPUT_EXISTS`) |
| S07-A12 | `TestReplayMutantControls`와 mutant 23개 |
| S07-A13 | 위 절, 로컬 S06·S05 raw replay |
| S07-A14 | `TestReplayDeterministic`; 세 OS 실행은 PR CI가 한다 |
| S07-A15 | 비공개 corpus 절 |
| S07-A16 | PREPARE native 증거 절 |

## 남은 일과 한계

* 세 OS CI, PR·merge·post-merge는 이 세션 범위 밖이며 orchestrator가 한다. Linux·macOS 실행은 로컬에서 하지 못했다.
* `go test -race`는 실행하지 않았다(CGO 없는 build).
* S06 API claim, 사실 재현·동적 SQL·query 기대값, PREPARE fact check와 edit 창, BrightScript gate 재계산은 기록으로만 남는다. 해당 replay는 PASS가 아니라 `UNRESOLVED`다.
* registration의 member bytes와 일부 identity는 S07에서 처음 결속했다. 이전 receipt가 그 bytes의 hash를 남기지 않았기 때문이다. 무결성은 그 시점 이후만 보장한다.
* memory는 in-process이며 강제하지 않는다(파일·record·합계 한도로 묶는다).
* evidence graph는 무결성·관계·자격만 검사하며 게시자 인증은 없다.
