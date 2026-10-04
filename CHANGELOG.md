# 변경 기록

## Unreleased

- native CI의 요구 축과 kit 축을 분리했다(#76). 오늘의 grammar에서 정직하게 실패하는 등록 사례가 kit 결함처럼 job과 mechanism gate를 실패시켰다.
  - 오류 tree 위의 incremental route를 BLOCKED로 둔다. route 계측이 있고 바뀐 것이 있는데 incremental·fresh 어느 parse도 node를 재사용하지 않은 edit step에서, 이전 tree와 새 incremental tree가 모두 full tree이고 그중 하나가 `has_error`인 경우다. summary·record 형식 tree는 replay가 `has_error`를 다시 계산하지 못하므로 FAIL로 남는다. tree-sitter는 ERROR 주변 node를 재사용하지 않으므로 이는 관측 불가다. code는 `INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_<n>`이고, 다른 실패 code가 없을 때만 사례 code가 된다. 깨끗한 tree, 계측 없음, 변경 없음, fresh 재사용은 그대로 FAIL이다. S05 판정, replay와 qualify의 `incremental-route` gate가 같은 규칙으로 다시 계산한다.
  - qualify는 route BLOCKED를 `KIT_CLAIM_FAILED`로 세지 않는다. kit 축은 PASS로 남고 E 의무가 BLOCKED가 되므로 칸은 `INCOMPLETE`다. FULL PASS에는 여전히 E PASS가 필요하다. 기록과 다시 계산한 route claim이 다르면 gate 실패다.
  - `run-routes.ps1`은 oracle `query_expectations` FAIL·BLOCKED와 route BLOCKED로 job을 실패시키지 않는다. 이 결과는 요구 축 결과(Q는 `tsgk qualify`가 판정)로 보고, 실패 내용과 함께 `summary.json`의 `requirement_results`에 기록한다. 그 밖의 job 실패 조건은 그대로다. set 검증 실패, error finding, build 실패·거부, 완료되지 않은 사례, incremental equality FAIL·BLOCKED, route FAIL, query equality·fact reproduction·dynamic SQL FAIL·BLOCKED가 이에 해당한다.
  - 결과·record schema는 claim 값(`BLOCKED` 포함)과 code를 열거하지 않으므로 revision을 올리지 않았다.
- native `incremental_equality` claim이 앞 step의 FAIL 뒤에 비교 없는 step이 오면 BLOCKED로 덮이던 결함을 고쳤다(#76). 이제 replay처럼 step 중 가장 나쁜 값(FAIL > BLOCKED > PASS)이다.

- 단계 기대값에 anchor를 더했다(#76). `contains`는 같은 type이 다른 줄에 있어도 통과하므로, 대상 구조를 잘못 parse한 사례가 통과할 수 있었다. TypeScript generic tagged template이 `binary_expression`으로 parse됐는데도 다른 곳의 `call_expression` 때문에 통과한 것이 그 예다.
  - 기대값의 선택 필드 `anchors`(`{type, start_byte, end_byte}`)는 그 step의 full tree에 정확히 그 type·byte 범위의 named node가 있어야 통과한다. 없으면 FAIL, full tree가 아니면 BLOCKED다. S05 판정, replay, qualify가 같은 규칙으로 다시 계산한다. anchor가 없는 기대값의 판정은 그대로다.
  - profile이 `tsgk-incremental/r2`·`tsgk-oracle/r2`, inventory가 `tsgk-qualification-inventory/r3`이 됐다. 형식이 틀리거나 step source 밖인 anchor는 profile에서 `EXPECT_ANCHOR_INVALID`, inventory에서 `CASE_INVALID`다. r1 profile은 이전 근거를 replay할 수 있게 anchor 없이 계속 읽는다.
  - 원본 사례 파일은 anchor를 `{type, text, occurrence}`로 적는다. `run-routes.ps1`과 inventory 생성기가 같은 방식으로 byte 범위로 바꾸고, foundation test가 둘을 독립 구현과 대조한다. 아직 anchor를 단 등록 사례는 없다.
- `DeriveDeclarations`가 한 match 안에 같은 level의 node가 여럿일 때 마지막 node만 남기던 결함을 고쳤다. PostgreSQL `CREATE TABLE … PARTITION OF …`의 선언 이름이 이 결함 때문에 잘못 나왔다(#77).
- 개발 helper `run-routes.ps1`이 capture 기대값을 잘못 직렬화하던 결함을 고쳤다(#77).
  - capture가 하나이면 scalar가 되어 `JSON_TYPE`으로 거부됐다.
  - capture가 0개이면 `null`, 즉 "검사하지 않음"이 됐다.
- `tsgk qualify` 결과가 `tsgk-qualification-result/r2`가 됐다. r1에 platform별 지원 claim `platform_claims`(platform id → `SUPPORTED`|`BLOCKED`)를 더한다. platform은 근거 무결성(완결성 PASS, finding 없음), 그 platform의 모든 칸 kit 축·요구 축 PASS, 실행된 추가 역할 행 PASS일 때만 `SUPPORTED`다. platform 간 비교는 넣지 않는다. 전역 `support_claim`·`mechanism_gate`·assessment·exit는 그대로다.
- qualification inventory 사례에 `expect_assessment`·`expect_code`를 더했다. SVC 관측 전용이고 요구 행을 덮지 않는 사례만 등록할 수 있으며(그 밖은 `CASE_INVALID`), 기록된 판정과 code가 등록값과 같을 때만 등록 검사가 PASS이고 다르면 FAIL이다. `n461-svc`의 의도된 BLOCKED 8사례(`SVC_INLINE_UNRESOLVED` 4, `SVC_INLINE_UNSUPPORTED` 1, `SVC_INLINE_NOT_PARSED` 1, `SVC_DIRECTIVE_DIAGNOSTICS` 2)를 등록했다. 그 역할 행의 PASS는 다음 CI qualification 근거로 확인한다.
- coverage 규칙이 `tsgk-coverage-rule/r2`, inventory가 `tsgk-qualification-inventory/r2`, 결과가 `tsgk-qualification-result/r3`이 됐다(#76).
  - 행의 P 의무는 production 대안 등록부 `src/contracts/feature-alternatives.json`에서 그 행이 `COMPLETE`이고, 모든 대안을 그 대안을 적은(`alternatives`) P 사례가 덮을 때만 덮인다. 그 밖은 `NOT_COVERED`다. 결과의 P 의무에 `alternatives`·`alternatives_uncovered`를 더한다.
  - 등록부는 178개 REQ 행 전부를 `PENDING`, 대안 없음으로 시작한다. 따라서 지금은 모든 P 의무가 `NOT_COVERED`이고 그 칸은 PASS가 되지 않는다. 정책이 처음부터 요구한 경계를 반영한 것이다.
  - route별 `error_nodes`(`native-routes.json`, html은 `erroneous_end_tag`)를 등록했다. 이 node를 `contains`에 적은 `NO_ERROR` step은 N, 다른 구조 node가 함께 있으면 R을 만든다. P와 W는 만들지 않는다. 이런 route에서 P·W step은 기록된 full tree에 error node가 없어야 한다(`has_error`는 이 node를 보지 않는다). 있으면 FAIL이고, full tree가 아니면 BLOCKED다(#81).
  - W 생산자를 등록했다. `sample`(repository·commit·path·SPDX license·sha256·bytes)이 있는 요구 사례가 대상이다. license는 MIT·Apache-2.0·BSD-2/3-Clause·PostgreSQL이다. 크기는 65536 bytes 이하이고 step 0 tree는 10000 node 이하다. step 0이 `NO_ERROR`를 기대하는 사례가 W를 덮는다. sample마다 `src/testdata/native/samples/NOTICE.md`에 고지 항목이 있어야 한다.

## v0.1.0 — 2026-10-04

첫 release의 범위는 [제품 범위](docs/specs/scope.md)가 정하고, 근거와 한계는 [플랫폼 지원 계약](docs/specs/platform-support.md)에 있다. tag `v0.1.0`(`3625072`)과 GitHub Release로 소스만 발행했다. binary와 package는 없다.

### VERIFIED(Windows amd64, Linux amd64, macOS arm64)

- offline core와 그 공개 API(`src/kit`): `inspect`, `identity`, `verify`(디렉터리와 ZIP, 압축을 풀지 않음), `schema check`/`schema diff`. CGO-free이고 CLI와 API 결과 bytes가 같다.

### experimental(호환성 약속 없음)

- `corpus`(비공개 corpus inventory)
- `reproduce`(두 작업 공간 생성 비교)
- `incremental`·`oracle record`(native parse/edit/query 기록; 승인 capability와 host compiler 필요)
- `replay`·`evidence verify`
- `qualify`(26 route × 3 platform 집계)

### 상태

- 26 route 지원 claim은 **BLOCKED**다. 필수 feature 사례가 부족하고(#70) 채택 route에 grammar gap이 남아 있다. 최종 qualification 근거는 main `03ea1f0`의 CI run 37184449649이다.

### 알려진 한계

- tree-sitter runtime의 field 조회와 cursor가 다른 API claim FAIL 23 사례(6 route)
- T-SQL 과잉 수용(r6에서 3종, r5부터 2종)
- `--out`이 `subst`·bind mount 별칭을 검출하지 못함
- 동적 SQL 탐지는 첫 문장이 아닌 `;sp_executesql`을 known miss로 둠
- BrightScript·cooklang 역할 NOT_RUN, `n461-svc` 역할 INCOMPLETE(SVC negative 기대값 8개, #70)
- `n461-large` 대형 실사용 profile은 Windows에서만 실행(Linux·macOS NOT_APPLICABLE)
- 시험 시간 여유(#65)

### Session 이력

- Session 00: Go CGO-free foundation, 제품·검증 계약, 세 OS CI와 순차 개발 campaign을 준비한다.
- Session 01: `tsgk inspect`/`identity`/`corpus`와 공개 offline API `src/kit`를 구현한다. 다음을 포함한다.
  - manifest r2(판별 encoding 결속), E0 report
  - known-paths discovery와 정적 closure 관측
  - no-follow guard, 유한 한도, kit wall과 caller 취소의 구분
  - no-clobber `--out`
  - 비공개 corpus inventory(N461 역할, PRESENCE_ONLY, `.csproj` 선언 관측)
- Session 02~07: strict verify, schema, reproduce, incremental, oracle record, replay·evidence verify (각 Session 보고서 참고).
- Session 08: 다음을 더한다. 지원 claim은 BLOCKED다.
  - `tsgk qualify`와 `kit.Qualify`
  - qualification inventory(`src/contracts/qualification-c1.json`)
  - CI의 host 실행 identity·기록 set 보관과 qualification job
  - module proxy 소비자 시험
  - T-SQL patch r6
- release 후보 준비(#72):
  - `src/contracts/campaign-01.json`에 Session 통합 상태(`INTEGRATED`, PR, merge commit, post-merge run)를 기록하고 guard로 검사한다.
  - 플랫폼 지원 상태표를 현재 근거로 갱신한다.
  - Linux `race diagnostic` job을 추가한다(비필수, CGO 진단 lane).
