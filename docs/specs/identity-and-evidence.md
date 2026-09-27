# Identity와 evidence — draft/r0

## 파일 identity

manifest는 `schema: tsgk-manifest/r0`, `algorithm: sha256`, `files`를 가진다. 각 파일은 portable `path`, `role`, `mode`(`100644` 또는 `100755`), `size`(bytes), 소문자 64자리 `sha256`으로 나타낸다. path의 UTF-8 bytes 오름차순으로 정렬하고 중복을 거부한다. 디렉터리·symlink·special file은 r0 파일 항목이 아니다. filesystem 기본 mode는 이식 가능한 `100644`다. 실제 executable bit와 Git mode를 관측하는 별도 모드는 그 출처를 명시하고 다른 identity로 취급한다. Windows에서 POSIX mode를 추정하지 않는다.

기본 선택 집합은 [discovery](cli-and-profile.md)의 발견 파일이고 strict profile은 exact 목록을 소유한다. verify는 `--expected`의 역할·mode 정책·목록으로 root를 다시 읽고 누락/중복/변조/선택 집합 내 추가 파일을 거부한다. 선택 밖 파일은 report에 excluded로 표시하며 전체 저장소 무결성을 주장하지 않는다. 출처 신뢰는 호출자가 제공하는 expected anchor에서 오고, 같은 root에서 즉석 생성한 manifest로 자체 인증하지 않는다.

파일 집합 hash의 preimage는 ASCII `tsgk-files/r0\n` 다음으로 각 항목의 `path\0role\0mode\0size\0sha256\n` UTF-8 bytes를 정렬 순서로 연결한 것이다. portable path와 role은 제어문자/NUL을 허용하지 않으며 size는 정규 10진수다. manifest 원문 bytes의 SHA-256도 별도로 보존한다. 의미가 같아도 raw JSON key order/공백이 바뀌면 raw hash는 달라진다. hash는 서명이나 게시자 인증이 아니다.

## 근거의 독립 축

| 축 | 값과 의미 |
|---|---|
| `execution_status` | NOT_RUN / COMPLETED / FAILED / CANCELLED / RESOURCE_LIMIT |
| `evidence_mode` | NEW_RUN / REPLAYED_RAW / CARRIED_FORWARD / RECORDED_NOT_RECOMPUTED |
| `assessment` | PASS / FAIL / BLOCKED / NOT_APPLICABLE / UNRESOLVED |

현재 판정과 과거 판정을 분리한다. CARRIED_FORWARD의 현재 실행은 NOT_RUN이며 원 실행 receipt를 참조한다. RECORDED_NOT_RECOMPUTED는 원 판정을 보존하되 현재 assessment를 UNRESOLVED로 둔다. 미실행 자체는 PASS가 아니다. 필수 gate의 NOT_APPLICABLE은 profile에 미리 등록된 사유가 있어야 하며 누락 근거를 대신하지 못한다.

identity에는 source snapshot/artifact set, generator/runtime/compiler·옵션·executable, query/fixture/edit/input bytes, profile/policy/threshold/comparator revision, host/OS/arch/runner image와 run ID를 별도 필드로 연결한다. portable semantic hash는 이식 가능한 결정적 결과만 사용하고 host 관측과 raw hash는 분리한다. normalization allowlist는 실행 전에 고정한다.

승계는 바뀐 입력과 무관한 gate에만 원 receipt·원 source/policy hash·변경 영향·승계 이유를 연결해 허용한다. 정책·comparator·threshold·해당 source가 바뀌면 재판정/새 실행 또는 BLOCKED가 필요하다. 검증 실패를 승계로 덮지 않는다. raw 재판정은 registered workload의 expected records 전체를 소비하고 중복·누락·미사용·stale identity를 거부한다.

BrightScript v0.1.2는 historical raw replay 기준이다. 그 로그 파생값의 2 ULP 정책과 expected count는 해당 workload adapter에만 속한다. kit의 전역 상수로 복사하지 않는다. S07에서 raw/type/threshold/verdict, signed zero, 경계와 max winner fixture를 고정하고 원 verifier·FAIL·측정을 보존한다. archived Python 실행이 필요하면 EXEC_ADAPTER이며 offline READ_DATA가 아니다.

source package는 추적 blob/mode allowlist, verification package는 별도 등록 manifest에서 구성한다. local 폴더 전체 ZIP은 허용하지 않는다. 작은 추적 manifest/요약 보고와 별도 raw 자산을 연결하되 이번 campaign에서 publication을 자동 수행하지 않는다.
