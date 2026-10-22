# #217 오류 tree route BLOCKED 원본 처분

기준 main은 `8578f5351197080e7284629f0f6e7c58f42b910f`이다. #217의 1/3(#249)은 필수 관측과 old tree 확인을, 2/3(#250)은 qualification 소비를 추가했다. 이 3/3은 그 규칙 위에서 역사적 오류 tree route BLOCKED 원본을 사례별로 처분한다. logger accepted에 근거한 새 재사용 증명 claim과 원본의 BLOCKED→PASS 승격은 하지 않는다. 그 claim은 [tree/adapter 계약](../specs/tree-and-adapter-protocol.md)의 `필수 관측 판정`대로 이 revision에 없으며, 후속 Issue로 분리한다.

## 처분 대상

등록 corpus의 route BLOCKED는 main run [39763842546](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/39763842546)의 세 OS `s05-routes-*` `summary.json`에서 0건이다(`requirement_results` 0). case 3,113개는 PASS이고 BLOCKED는 1건이다. 그 1건은 SVC 등록 사례 `n461-svc-language-vb-n-r1`(`SVC_INLINE_UNSUPPORTED`)이다. 등록 때 BLOCKED를 기대한 사례이며 incremental route BLOCKED가 아니므로 이 처분의 분모에 넣지 않고 따로 센다.

남은 원본 15개는 모두 등록 corpus 밖에 있다. [reuse holdout 원장](../../src/testdata/native/reuse-holdouts.json)이 각 원본의 source, 정확한 byte edit, 출처를 고정한다.

| 원본 | 수 | 출처 | 등록 corpus와의 관계 |
|---|---:|---|---|
| T-SQL #170 원본 | 11 | `246f510`(`1fa9cf3^`)의 `src/testdata/native/gaps/tsql.json` | 보호 sibling을 이번에 새 ID(`-r2`)로 바꿨다. sibling의 모든 version은 29-byte prefix `SELECT NULL;\nSELECT NULL;\nGO\n` 뒤에 원본 version이 온다(`TestReuseHoldoutRegistry`). 원본 ID(`-r1`)는 원장에만 남는다 |
| T-SQL #198 `unclosed-bracket` | 1 | untracked `artifacts/followup-goal-20261014/198-preflight/recovery-edit-preregistration.json` `/pairs/4`(파일 sha256 `79a1d86f…`) | 등록 corpus에 없다. 원장의 original·repaired hash가 사전 등록 값과 같다. forward edit(byte 12에 `]` 삽입)와 reverse edit를 그대로 쓴다 |
| JSON #197 `repair-close`·`break-close`·`repair-colon` | 3 | 이전 `reuse_json_test.go` 인라인 사례 | 등록 corpus에 없다. 이전 시험의 판정을 원장 시험이 대신한다 |

## 사례별 처분

이 원장은 원본을 다시 실행한 결과만 기록한다. 과거 기록의 판정은 바꾸지 않는다. 로컬 Windows/amd64(gcc 15 ucrt64, v0.27.1 prepared closure)에서 15개 모두를 정상 build와 old tree 생략 build(`TSGK_FAULT_OMIT_OLD_TREE`)로 실행했다. 모든 parse는 incremental·fresh 관측이 `COMPLETE`였다.

| 원본 | 정상 build의 BLOCKED step | old tree 생략 build의 첫 FAIL step |
|---|---|---|
| #170 원본 9개(`issue137-*`, `issue138-leading-*`, `issue139-*`) | 1, 2 | 1 |
| #170 `tsql-issue148-negative-pair-00-r1`·`-01-r1` | 2 (step 1·3·4는 route 증명) | 1 |
| #198 `tsql-issue198-unclosed-bracket` | 1, 2 | 1 |
| JSON 3개 | 1 | 1 |

- **kit 오분류 수정 완료(15/15).** old tree를 빼면 15개 모두 사례 FAIL이다. code는 `INCREMENTAL_ROUTE_OLD_TREE_UNVERIFIED_STEP_1`이고, 관측은 old tree 미전달·`new_parse=1`·`accepted=0`이다. 1/3 이전에는 이런 결함이 오류 tree BLOCKED 뒤에 숨을 수 있었다. issue148 두 원본은 정상 build의 첫 BLOCKED가 step 2이지만, 생략 build에서는 step 1이 먼저 실패한다. 그래서 원장은 두 값을 따로 고정한다.
- **관측 한계 잔존(15/15).** 정상 build의 BLOCKED step에서 공개 `TSNode.id` 교집합은 0이다(`reused_nodes=0`). 그 step의 logger는 old tree 전달(`parse_after_edit=1`)과 `accepted>0`을 기록했다. 공개 node identity로는 재사용을 증명하지 못하지만 runtime은 subtree를 받아들였다는 뜻이다. 현재 계약에서 이 관측은 route를 증명하지 않으므로 15개는 BLOCKED로 남는다.
- **지원 범위.** 원장의 원본은 qualification inventory 밖이다. 78칸 지원 claim과 무결점 주장의 대상이 아니다.

## 판정 유지 검사

- `TestReuseHoldoutRegistry`(foundation job)가 다음을 고정한다.
  - 원본 15개의 순서와 각 history(route·source·byte edit) digest
  - 등록 case ID와의 비충돌
  - BLOCKED step과 생략 build 첫 FAIL step의 범위
  - #198 사전 등록 hash
  - #170 sibling의 version 대응

  원본을 빼거나 바꾸거나 edit를 바꾸면 실패한다. 같은 최종 bytes를 만드는 다른 edit 범위로 바꿔도 실패한다.
- `TestNativeReuseHoldouts`(세 native routes job)가 다음을 다시 확인한다.
  - 정상 build: 고정한 step에서만 route BLOCKED이고, 그 step의 `accepted>0`, `reused_nodes=0`이다.
  - old tree 생략 build: 고정한 첫 step의 kit FAIL이다.
  - 모든 parse의 관측: `COMPLETE`

  OS별 결과가 원장과 다르면 job이 실패한다. 처분을 다시 정해야 한다는 뜻이다.
- 같은 job이 run-routes `summary.json`의 `requirement_results`에서 등록 사례의 `incremental_route` BLOCKED를 미처분으로 보고 실패한다. route BLOCKED의 판정, E 의무 BLOCKED, kit 축의 의미는 바꾸지 않는다. 새 BLOCKED가 생기면 사례를 corpus에서 빼지 않는다. 원인을 조사하고 처분을 별도 변경으로 검토한다.

## 집계

| 항목 | 값 |
|---|---|
| 처분 대상 원본 | 15 |
| 등록 corpus의 미처분 route BLOCKED | 0. 매 실행 native routes job이 검사한다 |
| 원장의 현재 route BLOCKED | 15 |
| SVC 등록 예상 BLOCKED(별도 분모) | 1 |

- **미처분 0**: true. 원장 15개는 모두 처분됐다. 등록 corpus의 미처분 BLOCKED는 매 실행 0으로 검사한다.
- **실제 BLOCKED 0**: false. 원장 15개가 BLOCKED로 남는다. SVC 예상 BLOCKED 1건도 별도로 있다.

S05와 S06 실행, 세 OS, 여러 step은 원본 수가 아니다. 위 수는 원본 history 단위다. 세 OS의 실제 결과는 이 변경의 필수 CI와 병합 후 main CI의 native routes job이 확인한다. 그 run은 #217에 기록한다.

## 비용

로컬 Windows에서 `TestNativeReuseHoldouts`는 JSON·T-SQL의 정상과 생략 build 4개를 포함해 64초였다. 이전 JSON 시험은 JSON build 2개였다. workflow의 go test 상한은 180초에서 300초로 올렸다. 증가한 job 시간은 기존 margin 감시가 측정한다. 이 증가는 원장 실행과 T-SQL 컴파일 비용이며 logger 단독 overhead가 아니다.
