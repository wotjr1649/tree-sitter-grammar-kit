# #216 TSQL8 정의 분류와 Windows 관측

Windows에서 TSQL8의 고정 빈 입력을 실제 native/reference로 수집하고, 조립·외부 caller approval·guarded 정의 검증과 독립 결과 리뷰를 마쳤다. A6는 중간 STOP과 교정 후 재개를 포함해 42단계를 완료했다. 정의 8개는 모두 predicate가 없어 조건부 `STRUCTURAL_ONLY`이며, 빈 입력 frame 하나의 reference eligibility는 8/8 PASS다. 원장 후보는 독립 리뷰 후 122/130에서 130/130으로 집계했지만 `semantic_assessment: NOT_ASSESSED`와 전체 #216/#195의 미완료 상태는 유지한다.

## 정의와 관측 범위

수집·조립·guarded reader의 기준은 main `29ecdef`, tree `cba565f1`이다. 이후 보고서·PR·merge revision을 당시 실행 source로 소급하지 않는다. [TSQL8 준비 계약](../specs/instrumented-query-reference.md#tsql-8개-정의의-binary-paging-분류-준비)의 닫힌 `tsql8-empty` profile과 [독립 anchor](../../src/testdata/query-semantics/tsql8-classification-anchors.json)를 사용했다.

[일반 query 원문](../../src/testdata/native/queries/tsql.json)의 `q.structure`, `q.r6-structure`, `q.r6-begin`, `q.probe-v16`, `q.probe-v22`, `q.issue128-ownership` 6개와 [fact query pack](../../src/contracts/fact-query-pack.json)의 `facts.declarations`, `facts.dynamic_sql` 2개를 연결했다. 8개 정의의 ID·순서·정확한 UTF-8 원문·metadata를 고정 입력과 대조했고, predicate 목록은 모두 명시적인 빈 배열이다.

| 항목 | 관측 또는 집계 | 해석 범위 |
|---|---|---|
| 실제 입력 | 빈 UTF-8 case 1개, 편집 없음, `native-query`·single frame·API=false | 원래 workload의 사례별 실행이 아님 |
| Native/reference | `program` root 하나, step0 `NO_ERROR`, 빈 match/capture | 빈 입력의 독립 기대값과 일치 |
| 정의 분류 | 8개 모두 `STRUCTURAL_ONLY` | Predicate 의미 평가 없음 |
| Reference eligibility | required 8 / verified 8, failed·blocked·unprocessed 0, PASS | 해당 빈 입력과 승인된 실행 context에 한정 |
| 잠재 query/tree-role 연결 | 정의 8 × 잠재 tree role 2,982 = 23,856 | Inventory 분모이며 실행·검증한 매칭 수가 아님 |
| 원장 후보 누계 | 기존 122/130 + 이번 8 = 130/130 | 독립 리뷰 후 집계한 local candidate |

최종 상태는 `ACTUAL_CANDIDATE_REVIEWED_AGGREGATE_ISSUED`, 집계 상태는 `LOCAL_CANDIDATE_AFTER_ACTUAL_REVIEWS`다. 서로 다른 source/reader/profile의 기존 분류를 합친 후보 누계이며, 단일 source에서 전체 130개 정의를 실행한 결과가 아니다. [정적 원장](../../src/contracts/query-slot-inventory-r1.json)의 `STRUCTURAL_CANDIDATE`·`compiled_metadata:UNVERIFIED`·`semantic_required:null`과 `semantic_evaluated:false`를 유지한다.

빈 입력에서 match가 없다는 사실만으로 정의 분류를 판정하지 않았다. Capture는 고정 query의 실행 경로와 빈 입력 기대값을 확인하고, 최종 정의 결과는 이후 같은 guarded snapshot의 eligibility·CST·metadata·request/frame 결속과 별도 공개 API parity를 통과한 뒤 채택했다. 이 guarded 단계는 새 native/reference 관측을 만들지 않았다. 이번 결과는 구조 분류와 eligibility 확인까지이며, 실제 T-SQL 입력의 매칭이나 의미 평가 완료를 뜻하지 않는다.

## 실행 조건과 독립 리뷰

모든 Job에 HARD 메모리 한도(4 GiB, 일부 내부 Job 1 GiB)를 적용했다. stdin 없음·비대화형 실행을 유지했으며 cleanup verified·residual 0을 확인했다. 실행 기록과 실패 Job·부분 산출물도 보존했다. 과거 reference build/control의 provenance는 별도로 검토해 caller 승인에 연결했으며, 이를 이번 TSQL 입력의 새 실행으로 세지 않았다.

수동 리뷰 지점은 capture, assembly input, disposable, assembly, historical, guarded input, guarded의 7곳이다. Historical 1곳은 runbook에 따라 Root 측이 작성했고, 나머지 6곳은 실행에 참여하지 않은 독립 reviewer가 작성했다. 처분 후 미해결 blocker 0·material 0·open 0이며, material 1건(Step38 writer)은 resolved, minor는 resolved 또는 accepted_limit로 처리했다. Reviewer의 저장 증거 대조를 새 Go 실행이나 API parity 재실행으로 세지 않는다.

Go 검증(Step3–6)은 1.27.2, Step29 이후는 1.27.3 환경이었다. 실행 중 사용자가 보안 패치로 Go를 올렸으며, 사용자가 승인한 예외(별도 결정 기록)에 따라 새 Go pin과 SDK manifest를 만들고 독립 검증 뒤 Step29 이후 필요한 검사만 새 pin을 읽도록 했다. 완료된 Step1–26은 보존했다. Step36의 시험 바이너리는 Step3에서 Go 1.27.2로 빌드한 것이므로, 이 결과를 Go 1.27.3으로 새로 빌드한 시험 결과라고 부르지 않는다.

Step38의 첫 writer는 `__main__` 없는 `guarded_final1.py` 모듈을 직접 실행해 출력 없이 exit 0으로 끝났고 driver는 PASS로 기록했다. 출력 누락을 확인한 뒤 STOP했다. 사용자 일괄 위임 범위의 별도 결정 기록에 따라 Step38·42의 진입점을 `make_guarded_records1.py`로 바꾸고 출력 존재·schema·상태 검사를 더했다. 수정 범위의 독립 검증 후 새 Job 디렉터리에서 Step38 writer부터 재개했고, 이미 완료된 수집·조립·guarded 검증과 records는 반복하지 않았다. 출력이 없던 첫 Job은 실패 증거로 보존하며 최종 결과의 근거로 쓰지 않았다.

Job 밖에서 읽기 전용 `go.exe version`을 1회 실행한 추가 이탈도 자진 보고했다. 뒤이은 SDK warm audit과 전후 불변 검사에 근거해 결과 영향 없음으로 수용했다. Go 재고정과 writer 수정은 각각 독립 검증을 거친 실행 중 예외이며, 원래 실행 조건을 모두 그대로 지킨 무중단 실행으로 기록하지 않는다.

## 이전 시도의 STOP과 후속 교정

A1~A5는 모두 STOP했고 기존 증거를 보존했다. 각 원인은 다음 시도 준비에서 고쳤으며, A6는 해당 지점을 통과했다.

| 시도 | STOP 단계 | 원인 | 다음 준비의 교정 |
|---|---|---|---|
| A1 | Step5 | Foundation `go test -timeout=120s`의 여유 부족 | 내부 timeout을 300s로 정하고 상위 operation cap·reservation을 정렬했다. SDK warm만 300s로 늘리고 단계 안 SDK pre/post 한도는 유지했다. |
| A2 | Step3 | 이전 실행이 남긴 고정 Go scratch 경로와 충돌 | Scratch를 tuple별 경로로 바꾸고 실행 산출물 root 밖의 모든 쓰기 경로 조사와 착수 전 부재 검사를 추가했다. |
| A3 | Step9 | Profile controller가 import하는 helper의 `GitLines`·`SourceSnapshot` 누락 | Helper 연결을 고치고 정적 closure 감사와 heavy dispatch 직전 preflight smoke를 추가했다. |
| A4 | Step11 | PowerShell 자동 변수 `$Input`을 매개변수 이름으로 사용 | 매개변수 이름을 고치고 사전 리허설에서 실제 실행된 코드 대비 변경분의 함수별 fixture와 자동 변수·StrictMode·인코딩 검토를 수행했다. |
| A5 | Step22 | Runner child Env의 `PATHEXT` 누락으로 `pwsh.exe` 호출이 대기·종료 코드 없이 반환 | 사전 리허설의 실제 요청과 같은 제한된 Env 형태로 맞추고 PowerShell 외부 명령 호출 경로와 Step22 caller를 runner 환경에서 사전 확인했다. |

이전 실패의 종료 상태나 부분 효과를 성공으로 덮어쓰지 않았다. A6의 결과 분모에는 사전 리허설이나 A1~A5의 미완료 결과를 더하지 않았다.

## 보존 한계와 후속

독립 리뷰는 원본 cohort writer가 일부 상세 상태를 다시 확인하지 않는 범위를 실제 정의 결과와 eligibility counter의 직접 대조로 보완했다. Step36 SDKBefore/After 값은 별도 산출물에 남지 않아 warm audit PASS·실패 시 final 작성 전 중단하는 코드 순서·final 존재를 통한 간접 근거로 수용했다. 일부 runbook 부록의 과거 consumer pin과 runbook/index 표기 불일치는 배포본 코드·실행 기록·별도 결정의 대조에 근거한 기록 한계로 남는다. 이 수용을 일반적인 writer 검증이나 provenance 보장으로 확대하지 않는다.

Go 버전 하드코딩 제거(go.mod 단일 기준)는 [#346](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/346)과 [PR #347](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/347)의 별도 변경이다. A6의 두 Go 버전 이력을 소급해 하나로 바꾸지 않는다. 캠페인 도구는 별도 결정 기록에 따라 실행 시작 시 측정한 baseline으로 실행 안 일관성을 확인하는 설계만 보존했다. Supervisor·reference·native 입력·검증 코드의 pin은 사람이 검토해 유지한다. 구현은 다음 A 준비에서 successor runbook·index·checklist·driver를 함께 동결하고 smoke와 독립 검증을 거치는 후속 범위이며, 알려진 runbook/index 불일치도 그때 정리한다.

최종 결과·독립 리뷰·실행 log·결정 문서와 원본 graph·approval·request·receipt는 로컬 untracked 증거로 보존하며 저장소 규칙상 커밋하지 않는다. 공식 요약은 [#216 결과 댓글](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/216#issuecomment-6763241123)에 있다. 이 추적 보고서나 hosted CI만으로 당시 원본을 독립 재구성·재승인할 수 없다. Eligibility PASS는 caller가 검토한 로컬 provenance에 조건부인 저장 일관성이며, hash와 receipt를 실행 진위의 서명 인증으로 보지 않는다.

이번 문서화는 새 runtime 관측이나 CI 실행이 아니다. PR 필수 CI와 실제 main 확인은 [validation](../validation/validation.md)의 별도 통합 단계다. 130/130 후보 집계를 전체 사례별 매칭·semantic 평가·후속 replay·inventory·fan-in 또는 #216/#195 완료로 승격하지 않는다.
