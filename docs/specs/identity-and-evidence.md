# Identity와 evidence — draft/r0

## 파일 identity

manifest r0와 그 hash preimage는 아래 역사적 설계로 보존한다. S01 채택 설계는 `tsgk-manifest/r1`이며 `algorithm: sha256`, `mode_policy`, `files`를 가진다. 각 파일은 portable `path`, `role`, `mode`(`100644` 또는 `100755`), `mode_provenance`, `size`(bytes), 소문자 64자리 `sha256`으로 나타낸다. path의 UTF-8 bytes 오름차순으로 정렬하고 중복을 거부한다. 디렉터리·symlink·special file은 파일 항목이 아니다. 기본 `mode_policy=portable-default`, `mode_provenance=POLICY_DEFAULT`, mode `100644`는 관측된 executable bit가 아니다. 별도 filesystem mode는 실제 관측 가능할 때 출처와 다른 policy로 등록한다. API/offline 경로가 Git process를 실행해 mode를 추정하지 않는다.

기본 선택 집합은 [discovery](cli-and-profile.md)의 발견 파일이고 strict profile은 exact 목록을 소유한다. verify는 `--expected`의 역할·mode 정책·목록으로 root를 다시 읽고 누락/중복/변조/선택 집합 내 추가 파일을 거부한다. 선택 밖 파일은 report에 excluded로 표시하며 전체 저장소 무결성을 주장하지 않는다. 출처 신뢰는 호출자가 제공하는 expected anchor에서 오고, 같은 root에서 즉석 생성한 manifest로 자체 인증하지 않는다.

파일 집합 hash의 preimage는 ASCII `tsgk-files/r0\n` 다음으로 각 항목의 `path\0role\0mode\0size\0sha256\n` UTF-8 bytes를 정렬 순서로 연결한 것이다. portable path와 role은 제어문자/NUL을 허용하지 않으며 size는 정규 10진수다. manifest 원문 bytes의 SHA-256도 별도로 보존한다. 의미가 같아도 raw JSON key order/공백이 바뀌면 raw hash는 달라진다. hash는 서명이나 게시자 인증이 아니다.

r1은 `tsgk-files/r1\nmode_policy\n`의 `mode_policy`를 실제 policy 값으로 대입하고, 각 record를 `path\0role\0mode\0mode_provenance\0size\0sha256\n`으로 결속한다. policy/provenance는 제어문자 없는 등록 값이다. r0와 r1을 서로 같은 identity로 취급하지 않는다. schema revision을 바꿔 source/정책 구분을 숨기지 않으며 S01의 독립 expected vector와 path/role/membership/mode/byte 변조 negative로 검증한다.

## 근거의 독립 축

| 축 | 값과 의미 |
|---|---|
| `execution_status` | NOT_RUN / COMPLETED / FAILED / CANCELLED / RESOURCE_LIMIT |
| `evidence_mode` | NEW_RUN / REPLAYED_RAW / CARRIED_FORWARD / RECORDED_NOT_RECOMPUTED / NOT_RUN |
| `assessment` | PASS / FAIL / BLOCKED / NOT_APPLICABLE / UNRESOLVED / NOT_ASSESSED |

현재 판정과 과거 판정을 분리한다. CARRIED_FORWARD의 현재 실행은 NOT_RUN이며 원 실행 receipt를 참조한다. RECORDED_NOT_RECOMPUTED는 원 판정을 보존하되 현재 assessment를 UNRESOLVED로 둔다. 미실행 자체는 PASS가 아니다. 필수 gate의 NOT_APPLICABLE은 profile에 미리 등록된 사유가 있어야 하며 누락 근거를 대신하지 못한다.

S01의 E0는 `schema: tsgk-report/r1`, command와 위 세 축, `identities`, `findings`, `coverage`를 가진다. inspect/identity의 단순 관측은 `COMPLETED/NEW_RUN/NOT_ASSESSED`이며 qualification PASS가 아니다. coverage는 requested/observed/unsupported를 구분한다. S07은 이 envelope를 확장하며 새 report 체계를 만들지 않는다. 과거 subject run과 현재 replay operation은 각각 identity를 갖고 replay 완료가 subject를 NEW_RUN으로 바꾸지 않는다.

identity에는 source snapshot/artifact set, generator/runtime/compiler·옵션·executable, query/fixture/edit/input bytes, profile/policy/threshold/comparator revision, host/OS/arch/runner image와 run ID를 별도 필드로 연결한다. portable semantic hash는 이식 가능한 결정적 결과만 사용하고 host 관측과 raw hash는 분리한다. normalization allowlist는 실행 전에 고정한다.

승계는 바뀐 입력과 무관한 gate에만 원 receipt·원 source/policy hash·변경 영향·승계 이유를 연결해 허용한다. 정책·comparator·threshold·해당 source가 바뀌면 재판정/새 실행 또는 BLOCKED가 필요하다. 검증 실패를 승계로 덮지 않는다. raw 재판정은 registered workload의 expected records 전체를 소비하고 중복·누락·미사용·stale identity를 거부한다.

승계 기본값은 거부이며 명시적으로 승인된 dependency 불변/호환 관계만 허용한다. S08의 현재 후보 native 78칸은 historical/replay/carry-forward로 채울 수 없다. source snapshot·선택 grammar·실제 source/shared/scanner/query closure, 도구와 빌드 산출물을 각각 결속한다. role/type/revision, 중복 reference, schema가 금지하는 cycle, expected collection 경계를 검증한다. unknown schema/reducer는 오류이고 archive 내부 코드를 import/실행하지 않는다. reducer는 실제 구현된 count/status/identity/comparator 및 별도 승인된 수치 정책만 수행한다.

BrightScript v0.1.2는 historical raw replay 기준이다. 그 로그 파생값의 2 ULP 정책과 expected count는 해당 workload adapter에만 속한다. kit의 전역 상수로 복사하지 않는다. S07에서 raw/type/threshold/verdict, signed zero, 경계와 max winner fixture를 고정하고 원 verifier·FAIL·측정을 보존한다. archived Python 실행이 필요하면 EXEC_ADAPTER이며 offline READ_DATA가 아니다.

source package는 추적 blob/mode allowlist, verification package는 별도 등록 manifest에서 구성한다. local 폴더 전체 ZIP은 허용하지 않는다. 작은 추적 manifest/요약 보고와 별도 raw 자산을 연결하되 이번 campaign에서 publication을 자동 수행하지 않는다.

## 준비 tracking의 적용 대상과 보존 결과

PREPARE tracking receipt의 객체별 S00 보존은 `s00_object_preservation.applicable`과 `status`를 구별한다. 대상은 `issue:2`와 `milestone:1`이며 실제 원본 body/description·title·state·관계 대조 결과를 `PRESERVED` 또는 `MISMATCH`로 기록한다. 다른 객체의 상태는 `NOT_APPLICABLE`이다. 별도 `tracking_readback`은 intended object와 실제 readback의 `MATCH`/`MISMATCH`를 기록한다. Parent #1은 S00 객체가 아니므로 그 안의 S00 완료 이력 section을 별도로 대조한다.

과거 prepare-02의 `s00_preserved`는 generator가 S00 객체 predicate를 직렬화한 필드다. non-S00의 false는 실제 보존 실패 판정이 아니며 모두 true로 교체하지 않는다. immutable 원본·generator·관측 시각을 보존하고 새 checkpoint에서 의미와 실제 대조 근거를 연결한다. 수정된 receipt만으로 과거 미실행 검증을 소급 PASS로 만들지 않는다.
