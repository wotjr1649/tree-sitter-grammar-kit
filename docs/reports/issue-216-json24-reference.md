# #216 Windows JSON24 실제 reference 연결

이 작업은 단일 JSON case의 실제 native 캡처와 독립 reference 응답을 기존 offline API에 연결한다. 전체 26 route, SVC, 다른 encoding, large paging 또는 다른 host의 완료를 뜻하지 않으며 #216은 열린 상태로 유지한다.

## 실행과 승인 경계

첫 사전 등록은 UTF-8 원문 `{"a":[1,1]}`의 byte8을 `2`로 바꾸는 한 번의 편집이다. 고정 query8개에 initial incremental, 편집 후 incremental, 편집 후 fresh를 적용해 semantic 분모24개를 유지한다. 독립 reference는 source version2개 × query8개의 single invocation16개이며, 편집 후 같은 reference 응답을 두 native tree 역할과 비교한다. 원본 producer의 `PRODUCER_LOCAL/BLOCKED/EXTERNAL_APPROVAL_PENDING`은 외부 승인 뒤에도 수정하지 않는다.

`TestQuerySemanticsJSONLocal`의 `plan` 단계는 caller가 지정한 새 local root에 profile, semantic registration, 원문·query와 고정 anchor를 작성한다. `TestQuerySemanticsJSONAssemble`의 `assemble` 단계는 이미 완료된 fresh build/control과 native/reference 캡처에서 실제 byte를 복사해 evidence graph, policy, eligibility registration과 witness를 구성한다. 조립물에 approval을 만들지 않는다. `TestQuerySemanticsJSONVerify`의 `verify` 단계는 caller가 evidence 밖에서 별도로 제공한 approval을 읽어 공개 API와 `oracle-set-r4`를 대조한다. 기본 `go test`에서는 세 수동 단계가 SKIP이며 실제 reference 검증 성공으로 세지 않는다.

수동 입력은 절대 경로인 `TSGK_QUERY_JSON_LOCAL_ROOT`, `TSGK_QUERY_JSON_FRESH_REFERENCE`, `TSGK_QUERY_JSON_CAPTURE`를 사용한다. 조립 후 검증에는 `TSGK_QUERY_JSON_ASSEMBLY`, 독립 승인 후에는 `TSGK_QUERY_JSON_APPROVAL`을 지정한다. 단계는 `TSGK_QUERY_JSON_LOCAL_STAGE=plan|assemble|verify`로 명시한다. 각 단계는 해당 최상위 시험 하나를 정확한 `-run`으로 선택한다(`^TestQuerySemanticsJSONLocal$`, `^TestQuerySemanticsJSONAssemble$`, `^TestQuerySemanticsJSONVerify$`). 입력과 기존 산출물을 덮어쓰지 않고 각 조립·검증의 별도 디렉터리를 보존한다. 증거와 승인 문서는 저장소에 commit하지 않는다.

기존 supervisor의 HARD memory와 유한 wall을 유지하고 stdout/stderr를 파일로 보관한다. 각 최상위 수동 시험의 PASS, 원래 실행 상태·exit, cleanup 검증·잔여0, 입력 hash 불변성을 확인한다. 실패한 수집 controller와 완료된 자식 실행은 구분하며, 자식이 완료됐다는 이유로 원본 바깥 receipt의 실패 상태를 고치지 않는다.

`approved-local`은 caller가 검토한 local provenance에 조건부인 저장 일관성이다. 실행 전 hash나 controller 설정이 영속화되지 않았다면 현재 hash로 과거 관측을 만들어내지 않는다. 승인 context에 operator 진술, 독립 검토 근거와 남는 연속성 가정을 구분해 기록한다. 이 검증은 서명·실행 진위나 악의적인 owner에 대한 보호를 제공하지 않는다. 정본은 [eligibility 계약](../specs/query-reference-eligibility.md)과 [실제 small 캡처 계약](../specs/instrumented-query-reference.md)이다.

## 현재 확인된 단계

실제 fresh Windows build9, Rust unit4, control53과 pipeline14를 보관했다. 현재 native의 내부 실행은 COMPLETED/exit0이고, 바깥 CLI의 expected blocked exit3와 producer BLOCKED는 원형대로 보존했다. 별도 reference16개는 exit0과 cleanup 잔여0을 확인했다. 계획된 native member102개를 조립한 graph 무결성은 PASS이며, 같은 실제 bytes를 approval 없이 공개 API에 넣은 결과 base PASS와 required24/blocked24를 확인했다.

실제 캡처의 source revision은 `ce4d1a8b5b183b0b709763d5b3690160212a74e3`, fresh reference build/control의 source revision은 `cecec482eb4219e449f1150b38b1f10d6c7bf0b5`다. 서로 다른 두 source의 cohort를 유지하며 이후 추가한 offline 조립 코드의 revision을 원래 실행에 소급하지 않는다.

2026-10-28 KST에 독립 정적·데이터 검토와 그 교정 검토를 반영한 외부 caller 승인으로 실제 BOUND 공개 API와 replay를 평가했다. 두 결과는 COMPLETED/PASS, binding valid, base PASS, semantic PASS이며 읽기 원장도 각각 11,245,099 bytes로 같았다. 다음 카운터는 모두 `failed=blocked=unprocessed=0`이다.

| 분모 | API verified/required | replay verified/required |
|---|---:|---:|
| Build | 9/9 | 9/9 |
| Rust unit | 4/4 | 4/4 |
| Control | 53/53 | 53/53 |
| Assertion | 36/36 | 36/36 |
| Semantic tuple | 24/24 | 24/24 |

같은 실제 bytes에서 승인 부재는 COMPLETED/BLOCKED와 required24/blocked24다. Driver owner를 바꾼 음성 자료는 `SEMANTICS_BINDING_GRAPH`, 첫 reference 출력 길이를 바꾸고 관련 caller digest를 다시 맞춘 음성 자료는 `ELIGIBILITY_OBSERVATION_SLICE`로 FAIL이었다. 후자는 verified23/failed1을 보존한다. 음성 검사용 승인 변형은 메모리에서만 만들며 실제 caller 승인 자료로 발행하지 않는다.

최종 graph SHA-256은 `bd3cf153dc79e0ee19b8095ebe25c40a836575106acf7f6a57eacfba886e134a`, 승인 bytes SHA-256은 `3552b92ccc0add4c5ada5e2892c27177f1b81d359471868134794ab4d2eb543f`다. graph83 node와 물리 member326개를 검토했고 native 계획102 member와 원본 상태를 유지했다. 승인 DTO의 `Env:null`을 runner가 `exec.Cmd`에 전달하는 요청 배열 `[]`로 맞췄으며 원본 capture는 바꾸지 않았다. 이는 Windows의 유효 자식 환경이 완전히 비었다는 주장이 아니다. Go가 SYSTEMROOT를 추가할 수 있다.

이 PASS는 명시한 local provenance 신뢰 범위에 조건부인 저장 일관성 결과다. CLI/DLL의 최초 영속 hash가 생성·사용 후에 남은 점, 과거 go.exe hash와 controller 환경의 `PYTHONOPTIMIZE` 관측이 없는 점을 승인 context에 그대로 남겼다. 현재 hash·운영자 진술을 독립적인 과거 prehash·flag·서명 증명으로 바꾸지 않는다. 후속 변경의 전체 local 검사, 두 관점 최종 diff 검토, 필수 PR CI와 actual-main 검증은 별도 통합 단계다.
