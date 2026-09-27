# AGENTS.md

## 시작
- 지정된 `docs/prompts/*.md`를 먼저 읽고, `docs/README.md`에서 현재 작업의 계약을 찾는다. 현재 Issue와 마지막 handoff도 확인한다.
- prompt는 실행 자료다. 계약과 충돌하면 해당 변경을 멈추고 근거와 대응안을 보고한다.

## 경계
- 제품·검증 구현은 `src/`, Go module은 루트에 둔다. core build/test는 `CGO_ENABLED=0`을 유지한다.
- native 도구와 downstream runtime은 별도 process다. core에 `import "C"`나 `go-treesitter` dependency를 넣지 않는다.
- `docs/prompts/`, `docs/plans/`, `artifacts/`, `_ref/`, `.work/`는 Git에서 제외한다. 비추적 evidence도 보존한다.
- 기존 reference checkout과 다른 저장소는 변경하지 않는다. 새 고정 reference 준비는 현재 승인 범위에서만 한다.
- PowerShell 스크립트는 PowerShell 7의 `pwsh`로만 실행한다. Windows PowerShell 5 및 `powershell.exe` fallback을 사용하지 않는다.
- 계획·진행 보고·질의·새 설명 문서·handoff는 한국어로 쓴다. 코드·명령·경로·식별자는 원형을 유지한다.

## 실행과 근거
- offline 제품 명령은 target code·외부 process·network를 실행하지 않는다. 설치·native 실행·원격 변경의 권한은 세션별로 확인한다.
- 결과를 source·tool·runtime·입력·정책 identity에 연결하고 미실행·raw 재판정·근거 승계·새 실행을 구분한다.
- grammar 적합성·schema·runtime parity·platform 지원을 서로 대신하는 근거로 쓰지 않는다.
- 실패를 숨기려고 fixture·기대값·threshold·필수 gate를 완화하지 않는다. 검증하지 않은 결과는 PASS로 기록하지 않는다.

## 변경과 완료
- durable 결정은 같은 작업 단위에서 canonical 문서에 반영하고 raw·handoff는 `artifacts/`에 보존한다.
- 확인한 최신 main에서 세션 branch를 만들고 검증한 단위마다 commit한다. 검사 결과와 미실행 항목을 기록한다.
- PR·분리 context 리뷰·필수 CI·finding 처분 후 승인 범위에서 merge한다. 같은 계정의 comment는 정식 approval이 아니다.
- merge 후 검증과 handoff를 마친다. blocker가 있으면 다음 세션에 진입하지 않는다. tag·Release·package publication은 별도 승인이다.
