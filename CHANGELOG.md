# 변경 기록

## Unreleased

- `DeriveDeclarations`가 한 match 안에 같은 level의 node가 여럿일 때 마지막 node만 남기던 결함을 고쳤다. PostgreSQL `CREATE TABLE … PARTITION OF …`의 선언 이름이 이 결함 때문에 잘못 나왔다(#77).
- 개발 helper `run-routes.ps1`이 capture 기대값을 잘못 직렬화하던 결함을 고쳤다(#77).
  - capture가 하나이면 scalar가 되어 `JSON_TYPE`으로 거부됐다.
  - capture가 0개이면 `null`, 즉 "검사하지 않음"이 됐다.

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

### qualify 계약 변경(#76)

- `tsgk qualify` 결과가 `tsgk-qualification-result/r2`가 됐다. r1에 platform별 지원 claim `platform_claims`(platform id → `SUPPORTED`|`BLOCKED`)를 더한다. platform은 근거 무결성(완결성 PASS, finding 없음), 그 platform의 모든 칸 kit 축·요구 축 PASS, 실행된 추가 역할 행 PASS일 때만 `SUPPORTED`다. platform 간 비교는 넣지 않는다. 전역 `support_claim`·`mechanism_gate`·assessment·exit는 그대로다.
- qualification inventory 사례에 `expect_assessment`·`expect_code`를 더했다. SVC 관측 전용이고 요구 행을 덮지 않는 사례만 등록할 수 있으며(그 밖은 `CASE_INVALID`), 기록된 판정과 code가 등록값과 같을 때만 등록 검사가 PASS이고 다르면 FAIL이다. `n461-svc`의 의도된 BLOCKED 8사례(`SVC_INLINE_UNRESOLVED` 4, `SVC_INLINE_UNSUPPORTED` 1, `SVC_INLINE_NOT_PARSED` 1, `SVC_DIRECTIVE_DIAGNOSTICS` 2)를 등록했다. 그 역할 행의 PASS는 다음 CI qualification 근거로 확인한다.

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
