# #216 Web16 정의 분류

현재 상태는 사전 기대값 등록과 test-only helper 구현 단계다. 새 candidate의 compile·native/reference 관측, 외부 caller approval, guarded 정의 검증은 아직 실행하지 않았다. 아래 값은 실행 전에 고정한 기대값이며 실제 관측 결과가 아니다.

## 이번 단위

JavaScript4·JSX3·TypeScript7·TSX2를 별도 닫힌 네 profile로 다룬다. 기존 Small17 closed5/17과 Go·C#·CJR·CPP 경계를 유지한다. 새로운 공개 API·CLI·schema·dependency나 resource 한도 상향은 추가하지 않는다. 구현 기준은 PR310 merge `150621791da9ade381c034c5d0f3bbaa7f8033e9`, tree `59a11104ed13be9947f1dbc47391dd7b167ebfb5`다. 이 기준을 아직 실행하지 않은 Web16 관측의 source로 표시하지 않는다.

| Profile | Query 정의 | Pattern 기대값 합 | Query별 capture-name 항목 합 | ABI symbol/field |
|---|---:|---:|---:|---:|
| `javascript4-empty` | 4 | 17 | 20 | 291/36 |
| `jsx3-empty` | 3 | 7 | 7 | 291/36 |
| `typescript7-empty` | 7 | 20 | 20 | 388/40 |
| `tsx2-empty` | 2 | 7 | 7 | 408/43 |

합계는16 queries·51 patterns·54개의 query별 capture-name 항목이고 전역 고유 이름은48개다. 빈 UTF-8 source의 공개 CST는 named `program` root 하나, parent=-1·field=null·byte span0..0·양쪽 point(0,0)·extra/error/missing=false를 기대한다. 공개 child 수0과 runtime 내부의 숨겨진 EOF subtree는 구분한다. ABI language/runtime/min-compatible은15/15/13을 기대한다.

## 기대값의 근거와 한계

고정 parser의 EOF reduction·goto·accept와 external scanner의 해당 state를 정적으로 검토했다. Query 원문16개의 ID·전체 source hash·순서·수작업 pattern 수와 첫 등장 capture 이름 목록은 별도 후보와 대조했다. 모든 최상위 pattern이 `program` 이외의 시작 node를 요구하므로 위 빈 CST에서 structural/expected tuple은 빈 배열 후보이며, predicate 목록은 명시적 `[]`를 사전등록한다. 실제 compiled metadata와 native/reference 결과가 이 값에 맞는지는 후속 실행에서 확인해야 한다.

JavaScript/JSX는 grammar pin과 symbol이 같아도 다른 route·query·anchor·승인 identity다. TypeScript/TSX의 중첩 parser/scanner 경로와 공통 scanner header도 각각 정확한 closure로 결속한다. 원래 ordinary workload의268 cases·112 edits·380 case steps·492 tree roles·2,039 potential query/tree-role 좌표를 이번16개 정의의 실행 분모로 사용하지 않는다.

## 검증 및 잔여 작업

Source-only focused 회귀, 두 관점의 독립 검토, frozen Windows 관측의 profile별 유한 예산과 원본 종료·정리 증거, 외부 caller approval, 같은 guarded snapshot의 정의 projection과 별도 공개 API parity가 필요하다. PR·필수 CI·공식 artifact와 실제 main 통합 검증도 아직 후속 단계다. 실행 실패·불충분한 한도·partial·정리 미확인은 원본을 보존하고 완료로 세지 않는다.

현재 Web16을 조건부 `STRUCTURAL_ONLY`로 기록하거나 기존 역사적52/130에16개를 더하지 않는다. 전체 #216/#195, 원래 per-case/semantic, Windows/Linux, SVC·large·replay·inventory·fan-in 의무는 미완료다. 신규 macOS 관측은 제외하고 기존 mandatory baseline은 유지한다. #303의 알려진 macOS race timeout 비차단·release 후 분석 처분과 최종 release skip 미완료도 유지한다.
