# Query reference offline eligibility 계약 (#216)

이 experimental API는 caller가 evidence 밖에서 신뢰한 승인 문서에 조건부인 저장 근거의 일관성을 검사한다. 실제 실행 진위·서명·게시자 인증이나 전체 route의 qualification 완료를 증명하지 않는다. 검증 중 process·network·임시 파일·원래 호스트 경로 조회를 수행하지 않는다. 기존 [reference connection](instrumented-query-reference.md)의 r1 및 native/r2 의미는 유지한다.

## 호출과 판정

```go
func VerifyQueryReferenceEligibility(context.Context, QueryReferenceEligibilityRequest) (QueryReferenceEligibilityResult, error)
type QueryReferenceEligibilityRequest struct {
    Evidence EvidenceRequest
    Approval, Registration, WitnessRegistration []byte
}
```

Caller deadline은 필수다. Approval을 evidence 내부에서 발견하거나 생성하지 않는다. Approval 부재는 BLOCKED이며 `Complete=false`다. 형식 오류는 `*Error`의 `KindInvalidInput`, 검증 가능한 모순은 FAIL, 적용 가능한 필수 근거의 누락·CST 부적격은 BLOCKED다. 선언한 파일의 부재·교차 owner·pin 불일치는 FAIL이다. 취소·예산·I/O 중단은 error와 `Complete=false`를 반환하며 이미 확인한 FAIL과 항목별 실패 수를 유지한다. Caller 취소는 wall 만료보다 우선한다.

`Complete`는 평가가 끝났다는 뜻이며 PASS와 독립이다. `Build`, `Unit`, `Control`, `Assertion`, `Tuple`은 각각 `required = verified + failed + blocked + unprocessed`를 만족한다. 고정 분모는 9/4/53/36이며 Tuple 분모는 외부 승인 witness의 required다. 분모0은 PASS가 아니다. 미처리와 처리 후 누락을 구분한다. 결과는 공통 Report와 `result_schema`, 네 caller/catalogue digest, `graph_sha256`, `evidence`, 위 다섯 카운터, `bytes_read`, `work`, `logical_bytes`, `complete`를 가진다. 네 digest는 `approval_sha256`, `registration_sha256`, `witness_sha256`, `catalogue_sha256`다.

Schema는 `tsgk-query-reference-eligibility-approval/r1`, `tsgk-query-reference-eligibility-registration/r1`, `tsgk-query-reference-eligibility-result/r1`이며 Go 상수 `QueryReferenceEligibilityApprovalSchema`, `QueryReferenceEligibilityRegistrationSchema`, `QueryReferenceEligibilityResultSchema`로 제공한다. 새 문맥은 `full-input-utf8/r2`, witness schema는 `tsgk-query-witness-registration/r2`다. SVC·UTF-16·CP949·included range·composite·large paging은 이번 API의 대상이 아니다.

## 외부 승인 문서

JSON은 unknown/duplicate/type/정확하지 않은 정수와 필수 member 누락을 거부한다. 아래에서 optional이라고 표시한 field만 생략할 수 있고, null은 생략을 대신하지 못한다. 기존 witness의 nullable metadata·anchor 의미는 유지한다. Digest는 lowercase SHA-256, revision은 lowercase 40/64자리 hex, ID는 기존 validID 최대128 bytes, 설명용 식별자는 UTF-8 최대1,024 bytes, attempt는 양의 정확한 정수다.

Approval의 필수 field:

| Field | 내용 |
| --- | --- |
| schema | 위 approval schema |
| registration_sha256, witness_sha256, evidence_policy_sha256, catalogue_sha256 | caller가 승인한 각 원문의 정확한 digest |
| candidate | source_revision, platform, run, attempt |
| cohorts | id, purpose(build/control/current), source_revision, platform, run, attempt, provenance_id의 배열. purpose별 최대1개 |
| provenance | 아래 provenance 배열. 최대3개, 중복·불명·미참조 항목 거부 |

Provenance 공통 field는 `id`, `kind`, `source_revision`, `runner_sha256`, `controller_sources_sha256`, `reviewed_context_sha256`, `review_record_sha256`, `evidence_sha256`, `invocation_ids`다. `kind=reviewed-ci`는 `repository`, `workflow_path`, `workflow_revision`, `run_id`, `attempt`, `artifact_id`, `artifact_sha256`를 더하고 `session_id`는 금지한다. `kind=approved-local`은 `session_id`를 더하고 CI field는 금지한다. 승인 provenance의 invocation 집합과 실제 등록 집합은 정확히 일치해야 한다. 현재 cohort는 candidate와 일치해야 하며 이전 build/control은 독립 승인한 exact closure로만 재사용한다.

Runner/controller/context/review digest는 caller가 검토한 실행 환경·assertion 활성화·입력 안정성에 대한 승인이다. 검증기가 해당 원본을 온라인 취득하거나 hash만으로 그 내용의 진실성을 인증하지 않는다. 별도 원본 supervisor exit 파일이 없으므로 `exit_attestation`의 `basis=reviewed-inline/r1`, `value`, `provenance_id`로 controller의 inline returncode 확인에 대한 승인임을 명시한다. 이를 원본 exit 관측이라고 부르지 않는다.

## Registration과 물리 member

필수 field는 `schema`, `context`, `catalogue_sha256`, `graph`, `owned_members`, `tools`, `invocations`, `observations`다. `build`, `control`, `native`는 optional이며 누락 시 적용 가능한 항목이 BLOCKED다. Graph는 `path=evidence.json`, `role=evidence-graph`, `bytes`, `sha256`의 ReplayMember다. `owned_members` 원소는 `node_id`와 `member`(path, role, bytes, sha256)를 가지며 각 path의 owner는 정확히 하나다. Portable 상대 경로와 대소문자 충돌을 검사한다. 사용되지 않은 원소도 포함해 모든 `owned_members`의 node 존재·graph owner·실제 파일 bytes/hash를 같은 guard snapshot에서 검사한다.

모든 physical pin은 같은 evidence guard pass에서 읽고 검증한 동일 bytes에 결속한다. 보관 대상을 선택해도 나머지 graph/member/policy/한도 검증을 생략하지 않는다. 원래 실행의 absolute path는 문자 그대로 비교할 뿐 검증 호스트에서 열지 않는다. 이전 호출의 snapshot을 재사용하지 않는다.

Tool descriptor는 `id`, `cohort_id`, `tool_name`, `path_literal`, `subject_sha256`, `identity_member`, `provenance_id`다. Build의 cargo/rustc/cc/ar/git/supervisor/python 7개 및 control의 cc/python/supervisor는 승인 identity JSON의 실제 tool hash·path에 결속한다. Descriptor는 reference/native 실행 파일·library 본체 pin을 대신하지 못한다. 사용되지 않은 descriptor를 포함한 모든 identity_member의 선언·owner·실제 파일 bytes/hash를 같은 guard snapshot에서 검사한다. 지원 group의 선택 여부로 이 검사를 생략하지 않는다.

| Group | Field |
| --- | --- |
| build | cohort_id, paths(B/H/S/T/R), identity, source_snapshot, vendor_snapshot, optional reference_binary |
| control | cohort_id, paths(C/G/L/LH/R), summary, grammar(원래 상대 경로→member pin), optional library |
| native | invocation_id, source, build_manifest, symbol, grammar(원래 상대 경로→member pin), optional reference_library |

B는 build root, H는 controller root, S는 B/source, T는 target triple, R은 reference 실행 파일 경로다. C는 control root, G는 C/grammar, L은 생성 library 경로, LH는 실제 library SHA-256이다. Paths/argv/env는 고정 catalogue의 기호를 실행 플랫폼의 구분자로 확장한 정확한 값과 대조한다.

Source/vendor snapshot은 path→{bytes,sha256}의 전체 행을 검사한다. 필수 Cargo.lock·binding·reference manifest/main, 루트 Cargo.toml/config 및 build identity의 전체 snapshot/metadata/selected-tree digest를 결속한다. 이는 vendor 파일 13,738개 본체를 새로 펼쳐 읽는 검사가 아니다. 파일 본체가 필요한 executable/library/grammar/source/query는 별도 physical member가 있어야 한다.

## Invocation과 입력

공통 필수 field는 `id`, `kind`, `cohort_id`, `catalogue_id`, `node`(id와 전체11-role identities), `approved_request`, `inputs`다. Node는 `run`, NEW_RUN, 정확한 source/platform/run/attempt 및 승인 identity를 유지한다. Negative control은 원래 execution_status와 NOT_ASSESSED 축을 유지하며 별도 control 검증 수로 센다.

| kind | 추가 field와 금지 |
| --- | --- |
| REFERENCE_PROCESS | request, receipt, exit_attestation 필수. response/process 금지. 도구 호출에는 tool_id, 실제 reference에는 executable과 executable_path를 사용 |
| NATIVE_DRIVER | request, response, process, executable, executable_path 필수. receipt/tool_id/exit_attestation 금지 |

위 추가 pin field는 JSON에서 해당 arm에만 둔다. Build/control의 catalogue_id는 정본 ID이며 current invocation의 catalogue_id는 자신의 id다. 승인 request는 저장 request와 strict 구조상 같아야 한다. 원래 argv/env/wall/grace/output/memory 조건을 catalogue와 대조한다. Native process도 양의 정확한 한도·정렬되고 중복 없는 env·문자열 경계를 요구한다. 모든 process의 Path·Dir는 승인 cohort의 실행 플랫폼 기준으로 Go 절대 경로 문법을 만족해야 하며, verifier host에서 resolve/open하지 않는다. Windows의 drive-relative·root-relative 및 UNC/device volume 내부의 `..`는 절대 경로가 아니고, POSIX의 leading `/`와 구분한다. 승인·저장 request와 완료 receipt가 서로 일치해도 상대 경로 모순은 FAIL이다. Native response의 process stdout count는 단일 원본 JSON frame bytes+4와 정확히 같아야 한다.

`inputs`는 slot→tagged union이다:

| kind | 허용 payload |
| --- | --- |
| PINNED | member_pin, content_basis(PRESERVED_ORIGINAL 또는 APPROVED_EXPECTED_BYTES). argv 입력이면 argv_index와 path_literal 둘 다 |
| EXPECTED_ABSENT | catalogue가 승인한 argv_index와 path_literal만. member/hash/bytes payload 금지 |
| NOT_APPLICABLE | 다른 payload 없음. 해당 catalogue slot에서만 허용 |

빈 source는 bytes0 PINNED다. Build의 workload source/library는 NOT_APPLICABLE, patch/check는 실제 patch pin이다. Compile의 입력 library는 NOT_APPLICABLE, parser 입력과 생성 library는 별도로 결속한다. Reference의 source/library/query는 실제 argv 순서와 일치해야 한다. Current reference의 single/batch library·source·모든 query 경로는 실행 플랫폼의 Rust 1.97.1 `Path::is_absolute` 문법도 만족해야 한다. Go process Path/Dir 문법과 구분하며 verifier host의 경로를 열지 않는다. Windows drive는 ASCII 영문자이며 UNC의 server/share가 필요하고 verbatim/device prefix는 Rust 규칙을 따른다. [고정 Rust 구현](https://github.com/rust-lang/rust/blob/1.97.1/library/std/src/sys/path/windows_prefix.rs)과 [절대 경로 판정](https://github.com/rust-lang/rust/blob/1.97.1/library/std/src/sys/path/windows.rs)을 기준으로 한다. Unsupported/wrong-library-sha는 empty source를 사용하며 후자의 실제 library hash와 의도적으로 틀린 argv의 64개 0은 구분한다. Missing-query 반례는 EXPECTED_ABSENT, oversized/invalid UTF-8 query는 존재하는 raw bytes pin이다. Native source와 query-<id>는 frame 입력의 승인 bytes pin이며 library slot은 NOT_APPLICABLE이다.

Process receipt는 `tsgk-reference-process/r1`의 schema/execution/stdout_base64/stderr_base64 정확한 네 field다. Canonical Base64와 원본 byte count를 대조하고 정상 출력은 truncation을 허용하지 않는다. 의도한 batch-truncated 하나만 stdout1024 bytes의 prefix와 1024보다 큰 원본 byte count, RESOURCE_LIMIT/OUTPUT_LIMIT, HARD memory·검증된 cleanup을 요구한다. 일반 negative와 이 예외를 섞지 않는다. Windows Job Object와 Linux cgroup v2의 HARD memory/잔여0/CONTAINED를 요구한다. Linux HARD request의 CgroupParent는 비어 있지 않은 POSIX 절대 경로여야 한다. 승인·저장 request가 모두 이 조건을 만족해야 하며, 비어 있거나 상대 경로인 request와 HARD receipt의 모순은 FAIL이다. 실제 delegated cgroup의 controller·권한·존재는 승인 provenance와 receipt의 신뢰 범위이며 verifier host에서 재조회하지 않는다. 신규 macOS 검증은 수행하지 않으며 기존 CI baseline은 별도다.

## 독립 기대값과 현재 native 연결

[고정 catalogue](../../src/contracts/query-reference-eligibility-catalogue-r1.json)는 reviewed controller의 build9/Rust unit4/control53/assertion36, 정확한 입력 bytes/hash, 수작업 full CST·metadata·structural/evaluated capture 순서와 batch footer/prefix 기대값을 소유한다. Runtime match ID는 고정 숫자로 정규화하지 않으며 opaque 관계 검증과 single/batch 전체 parsed-response parity를 모두 요구한다. Rust unit은 네 이름 각각 한 번과 정확한 전체 summary를 요구한다.

현재 verifier는 현재 내장 catalogue와 정확히 결속된 승인·build 입력만 검증한다. Reference 소스 수정으로 catalogue pin이 바뀌면 이전 catalogue의 승인과 build는 새 verifier의 현재 자격을 충족하지 않는다. 새 reference에는 새 identity의 build·controls·graph·외부 승인이 필요하다. 과거 원본과 그 catalogue에 맞는 source/reader revision은 그대로 보존하고, 과거 조건부 결과를 새 reader나 새 reference의 관측으로 바꾸어 기록하지 않는다. 이 경계는 과거 catalogue를 자동 허용하는 호환성 경로를 추가하지 않는다.

Witness r2는 기존 QueryWitnessRegistration의 field와 `native`를 가진다. Native는 `protocol`, `request_id`, `node_limit`, `api`, `points`, `runtime_manifest_sha256`, `producer_sha256`, `executable_sha256`, `producer_abi`, `observation_run_id`, `history_start`, `process`다. Producer ABI는 `language_version`, `runtime_language_version`, `runtime_min_compatible`, `symbol_count`, `field_count`다. Process field는 저장 request와 같은 Path/Args/Dir/Env/StdoutBytes/StderrBytes/Wall/Grace/Memory(Bytes,Hard)/CgroupParent다. 시간 단위는 Go duration의 nanoseconds다.

독립 승인한 native/r4 기대값과 실제 source에 ApplyEdits를 적용한 모든 version bytes/hash/points로 nativewire Expect를 구성한다. CheckHeader/Step/Open/End, reuseobs.Decode/CaseClaim/Judge, querywire/querypolicy 및 comparator를 재사용한다. Mandatory logger prefix PASS만으로 full logger COMPLETE를 대신하지 못한다. Build manifest는 승인 ProducerSHA256에 결속하고 full runtime/driver closure와 grammar physical pins를 대조한다. Grammar 역할은 parser/scanner/header이며 parser는 정확히1개다. Canonical quoted argv와 version/runtime/parser/scanner*/driver/shim/link의 필수 순서·개수 및 version argv를 검사한다. 현재 고정 runtime manifest의 ABI 호환 범위13..15도 승인/raw 값과 대조한다. Grammar의 language/symbol/field count는 caller가 독립 승인한 기대값과 raw producer를 대조하며 C source를 새로 해석하거나 컴파일해 이 숫자를 추출하지 않는다. 현재 내장 runtime manifest만 완결 검증 대상이며 과거 manifest는 BLOCKED다. 실제 실행 여부는 승인 provenance의 신뢰 범위다.

Observation 필수 field는 `query`, `step`, `tree_role`, `source_bytes`, `source_sha256`, `invocation_id`, `query_id`, `response_ordinal`, `byte_offset`, `byte_length`, `sha256`다. Witness의 required 밖 observation·중복·불명은 거부한다. Source version과 raw stdout의 정확한 response slice에 결속하며 현재 reference의 독립 full CST·metadata·capture를 대조한다. 유효하지만 다른 CST는 BLOCKED다. 형식이 유효한 current reference FAILED 응답은 producer의 exit1과 일치해야 하며, 성공 exit0 등의 모순은 FAIL이다. CST 동등성 전에는 capture 개수와 match/pattern/capture/name 순서를 비교하며 node index 등 CST 종속 전체 비교는 동등성 확보 뒤에 수행한다. 별도로 승인된 metadata·anchor의 독립 모순은 계속 FAIL이다. Native full CST가 없으면 제공된 reference anchor 자체의 전체 모순 검사도 유지한다. Required 밖도 포함한 모든 제공 full native tree의 EOF·실제 source points·내부 CST 일관성을 tuple 집계 전에 검증하며, 확인된 source 모순은 FAIL이고 검사 중단은 오류로 전파한다. Current 입력 slot이 없을 때 nil·빈 digest로 모순을 만들지 않으며 따로 승인된 observation/source version·request의 library hash에 대한 검사는 유지한다. Build·Control·Native 그룹이나 적용 가능한 observation이 없으면 독립 기대값이 없는 비교는 생략하고 해당 의무를 BLOCKED로 남긴다. Batch의 한 query observation 누락은 다른 query의 검증을 막지 않으며, 제공된 request·pin·response의 독립 모순은 계속 FAIL이다. Reference 및 native의 중첩 query·API·logger JSON 형식 오류는 이 API에서 KindInvalidInput으로 반환하고 Native raw의 response·r4 producer·step·tree와 compact 배열은 필수 member 및 primitive type도 검사한다. Nullable wire arm은 step의 edit/route/fresh, tree의 form/types/fields/nodes/errors/declarations/partial_trees, declaration의 name이며 중첩 query/API/logger arm은 별도 strict 검사를 통과해야 한다. Native 그룹이 생략되거나 source가 없어 version을 구성하지 못해도 등록된 모든 NATIVE_DRIVER invocation이 제공한 raw의 형식 검사는 수행한다. 모든 native invocation에서 자체 approved/stored request의 strict 일치·process 한도/env·실행 파일 owner/pin/hash/path·execution/stream, raw envelope/ABI/capability와 logger의 자체 승인 binding·역할·count/status 일관성을 확인한다. 확보한 자체 source pin으로 초기 source transport·UTF-8·full tree EOF/points/CST를 확인하며, query input은 확보한 각 pin의 길이1..65,536 bytes와 ID별 bytes를 대조하되 map에서 요청 순서를 추론하지 않는다. 모든 완료 full tree의 root EndByte는 초기/후속 actual source 존재나 선택 group과 무관하게 그 step 자체의 SourceBytes와 같아야 한다. Query 원문이 없어도 status·metadata·capture accounting과 확보한 full node 연결을 검증하고, compile-error class/code/shape와 query_ms=0·matches=0·빈 types도 유지하되 실제 원문의 offset/point 대조만 BLOCKED로 남긴다. 0개 query의 명시적 null/빈 배열은 허용하며 확보한 query pin이 응답에서 빠지면 FAIL이다. 제공 step의 순서·형태·내부 status/completion 모순도 검사한다. 초기 outer/step0 길이를 직접 대조하고 edit의 byte 순서·전후 길이 관계·빈 edit·내재 point 관계 및 확보한 초기 source의 첫 edit old point를 검증한다. 실제 후속 source·정확한 edit history의 증명과 구분한다. EOF/CST/point의 확정 모순은 공통 경로에서 먼저 기록하고 중단 오류를 반환하여 연결된 실패 counter를 보존한다. 후속 source를 raw 자체로 증명하지 않으며 전체 edit history·query 순서·build closure·required tuple 연결만 선택된 witness/group에 의존한다. 정상 extra에 선택 history가 없다는 이유로 전역 BLOCKED나 연결되지 않은 tuple 실패를 만들지 않는다. 그룹·source 기대값에 의존하는 비교는 해당 의무의 BLOCKED로 남긴다. 기존 r1 connection과 native r2의 판정 의미는 유지한다. Native version이 없더라도 보관된 current source의 UTF-8·EOF·points와 predicate를 검사하며, 확보된 빈 query bytes도 hash 모순 검사에 사용한다.

## 유한 예산과 검증 범위

공유 한도는 physical member/JSON 각각16 MiB, 총 읽기256 MiB, file10,000개, wall120초, output16 MiB, identity1 MiB다. JSON depth32/decode·shape 각각2,000,000 values, 공유 work32,000,000, 논리 처리량1 GiB를 적용한다. 단, eligibility 검증이 원장을 직접 시작하고 approval에 결속된 registration의 context가 `full-input-utf8-paged-parser/r1` 또는 `full-input-utf8-paged-binaries/r1`일 때만 논리 처리량은 1.5 GiB(1,610,612,736)다. 이 값은 registration 검증 직후·evidence 읽기 전에 한 번 정하며 evidence·descriptor·build manifest·파일 크기 같은 producer 출력은 선택에 쓰지 않는다. 결속이 맞지 않는 요청과 semantic·replay·qualification run에서 물려받은 원장은 1 GiB를 유지한다. Snapshot은 각30,000/합계60,000행, 행64 MiB/선언 합계1 GiB다. JSON 반복 pass·graph pair/cycle·filter/predicate/comparator·unit stdout scan·해시·복사·output sealing의 보수적 비용을 무거운 처리 전에 차감한다. Compact node가 공유하는 type/field 문자열은 node별로, capture가 재참조하는 type 문자열은 capture별로 계상하며 tree digest와 capture 연결에 취소 checkpoint를 적용한다. Partial conversion마다 반복하는 공유 field table의 길이 곱도 사전 계상하고 table·node·partial 순회에 같은 중단 검사를 적용한다. Batch 분할은 예상 응답 수와 footer에 필요한 행 수로 제한한다. Member/tool/input 및 replay-result 예약의 중단 오류는 모순 finding으로 바꾸지 않고 반환하며 이미 확인한 FAIL만 유지한다. 사전 invocation 결속의 실패도 해당 Build/Control 및 연결된 tuple counter에 즉시 보존하며 후속 미완료 상태로 덮지 않는다. Caller가 한도 값을 지정하는 옵션은 없고 값은 승인된 context가 정한다. 서로 맞는 approval·registration 쌍을 만든 caller는 paged context를 선언해 1.5 GiB를 쓸 수 있으므로, public API의 논리 처리량 최악 상한은 paged context에서 1.5 GiB다.

정상/변조 synthetic 검사는 외부 승인 경계의 동작 검사이며 runtime 자격 근거가 아니다. 보관한 Linux raw192는 요청/receipt/stderr·identity·snapshot·summary를 부분 수용하는 근거다. 실제 binary/library/입력/current-r4/승인 graph가 없으므로 전체 PASS로 승격하지 않는다. Producer 보관 확대·claim/reducer/inventory/fan-in·전체 Windows/Linux route 및 C# 실제 재사용 증명은 후속 단위이며 이 API 구현으로 Issue #216을 닫지 않는다.

### 같은 session의 paging identity 재사용 (Refs #362)

Semantic caller가 같은 run·원장과 열린 root를 전달한 approval-bound paged session에서만 guarded read의 paging part를 private record로 보관한다. Graph의 같은 owner·role·prefix에 속하는 part가 연속된 index 순서이면 읽기 loop의 같은 chunk를 개별 SHA-256과 전체 chain에 함께 넣고 part bytes를 retain하지 않는다. Descriptor는 뒤의 기존 검증 단계에서 owner·kind·index·offset·경로·크기·개별 pin·전체 logical pin을 모두 대조한다. Chain record의 순서와 소비한 part 집합도 정확히 같아야 한다. Graph/policy/node의 후속 검증이 owner를 무효화할 수 있는 경우와 비연속·역순 group은 bytes를 retain해 기존 검증과 실패 finding 순서를 보존한다. Retained record는 유일한 backing slice를 소유하며 일반 `member()`에 별도 과금한 복사본만 준다. Streamed part의 bytes 요청은 빈 slice 대신 내부 typed 오류 `ELIGIBILITY_MEMBER_BYTES_UNAVAILABLE`다. Callback·standalone eligibility·미검증 snapshot은 기존 part SHA-256과 전체 chain 검증을 유지한다.

내부 record는 live session/run/원장, 열린 root와 read 전후 physical identity, snapshot, graph·approval·registration·witness·policy pin, candidate/cohort, owner/node·role·path·bytes·SHA-256에 결속한다. Open 전후와 읽기 완료 뒤 SameFile·size·mtime, link·regular-file 및 읽기 한도 검사는 그대로다. Streamed chain은 같은 guarded read의 실제 bytes만 hash한다. Retained 경로는 기존 `h.Write`를 수행하고 두 경로 모두 외부 build/approval logical pin과 대조한다. Descriptor도 bytes·digest를 다시 확인해 immutable한 문자열과 ordered part pins로 보관한다.

Eligibility가 전체 검사와 output sealing까지 정상 완료한 같은 session에서만 semantic set이 그 logical identity를 재사용한다. 저장된 PASS 문자열이나 공개 bool/hash는 권한이 아니다. Set은 manifest 전체 member/plan 집합과 `Case=""`, descriptor 물리 pin, ordered part pins, build closure index·grammar/static closure, producer/runtime/compiler/build manifest, 외부 profile·registration·정책·candidate/cohort를 계속 대조한다. Descriptor 또는 build/profile pin이 다르면 재사용으로 치환을 승인하지 않는다. `semanticSnapshotIdentity`의 앞뒤 physical 검사는 유지한다.

Encoding 결과를 소비하지 않는 discovery metadata/source scan·evidence·replay·qualification read는 detector를 생성하거나 실행하지 않는다. Corpus·grammar와 별도 ZIP 검증의 encoding 판정은 유지한다. Guarded read의 논리 비용은 `(bytes+1)*(2+detector+retain+sink 수)`다. `detector`와 `retain`은 실제 실행/보관할 때만 1이다. Streamed part는 retain 없이 chain sink 하나를 쓰므로 추가 chain pass를 계상하지 않는다. Retained verified part는 추가 chain에 `bytes+1`, generic callback은 part digest와 chain에 `2*(bytes+1)`를 계속 계상한다. Set은 완료된 같은 logical identity의 두 추가 hash pass를 회피한다.

Eligibility registration의 bytes 소비 계획을 확인한 snapshot에서만, bytes를 소비하지 않는 `driver`·`native-executable`·`reference-executable`·`runtime-library`·`compiler` member의 identity만 보관한다. 같은 guarded read에서 전 bytes와 pin을 검증한 owner·role·path·bytes·SHA-256이다. Presence-only 소비자는 이 identity를 다시 결속하고 bytes 요청은 `ELIGIBILITY_MEMBER_BYTES_UNAVAILABLE`로 거부한다. Request·receipt·source·query·manifest·descriptor와 기타 실제 bytes 소비 경로는 retain을 유지한다. Registration이 이 bytes 소비 pin에 binary/part role을 붙이더라도 해당 경로는 retain하며, 예외 보관 경로마다 `256+len(path)` B를 예약한다. Role 문자열만으로 bytes 불필요를 판단하지 않는다. 해당 eligibility snapshot을 소비하는 semantic set의 plain driver도 같은 snapshot의 identity로 존재와 manifest pin을 대조한다. Standalone reference connection·승인 없는 semantic snapshot처럼 이 계획이 없는 호출자는 기존 bytes 보관을 유지한다.

새 관리 비용 `E`는 별도 과금한다: session 2,048 B, guarded part `512+len(path)` B, source 512 B, part 결속·ordered pin 보관 `256+len(path)+len(role)+len(SHA256)` B, logical record `1,024+2*(descriptor_bytes+1)` B, set logical 재결속 256 B 및 기존 member metadata 비용이다. Streamed group은 1,024 B와 part마다 `128+len(path)+len(role)+len(SHA256)` B, identity-only member는 `256+len(path)+len(role)+len(SHA256)` B를 더한다. 일반 retained bytes 복사에는 `bytes+1` B를 더한다. Descriptor decode와 신뢰 경계 검증 비용은 남고 신규 결속·비교·planning의 work/checkpoint도 공유 원장을 사용한다. 이 절감은 한도 인상, standalone 1.5 GiB의 상속 또는 ordinary 전체 수용을 뜻하지 않는다.

### Source/vendor snapshot의 typed decode (Refs #362)

Source/vendor snapshot의 고정 `map[string]eligibilitySnapshotRow`는 전체 strict token preflight, AST 없는 streaming shape 검사, typed unmarshal로 읽는다. 구문 검사를 shape보다 먼저 끝내며 unknown·중복·필수 필드 누락·null·타입·정수 형식/범위·UTF-8·trailing data와 기존 depth/value/document 한도를 유지한다. 행별 portable path·case 충돌·SHA·크기/합계 검사도 그대로다. Native response와 아래 reference·witness 문서도 같은 엔진을 사용한다.

현재 내부 엔진은 string/bool/uint/uint32/uint64/int/int64, string-key map, 명시적 tag의 struct와 slice/array/pointer 및 opaque `jsontext.Value`를 지원한다. Eligibility의 inline struct와 `[]byte`의 base64 문자열도 지원하며 null·필수 필드는 소비 경로의 기존 규칙을 따른다. 임의 custom unmarshal을 지원하는 범용 decoder는 아니다.

이 경로의 decode 기본 logical 예약은 `5n + 32z + 32768 + 80v`다(`n`: 원문 bytes, `z`: 공백을 뺀 유효 bytes, `v`: JSON 값 수). AST와 행마다 생성하던 reflection 필드 표를 보관하지 않는다. 기존 snapshot 행별 1,280 B의 보수적 container/validation 예약은 유지하며, 문서당 shape descriptor에 type마다 256 B와 struct field마다 512 B를 추가한다. Snapshot 전체 descriptor는 2,048 B다. Work는 공통 preflight의 `4v+1`에 decode 진입 1, streaming shape의 값·member name·필수 field 순회, typed pass의 `v`와 descriptor 구축을 더한다. 정상 snapshot 행 수가 `r`이면 decode work는 기존 `24r+9`에서 `23r+14`가 된다. Guarded read와 후속 snapshot 의미 검증의 비용은 별도다. 80 B/value는 typed container 예약이며 실제 heap peak를 뜻하지 않는다. 이 절감으로 work/logical/read 한도를 늘리거나 ordinary 전체 수용을 주장하지 않는다.

### Native response의 typed decode (Refs #362 B1)

Eligibility의 `nativeResponse`와 승인 없는 semantic native 응답은 `decodeNativeResponse`를 공유한다. 전체 문서의 strict preflight 뒤 streaming shape와 typed unmarshal을 실행하며 중간 AST를 만들지 않는다. `Producer`·`Step`의 omitempty 필드도 기존처럼 필수이고, native nullable arm만 null을 허용한다. `WireTree`의 status/form 조건부 필수 필드는 member 순서와 무관하게 검사하되, 존재하는 필드의 잘못된 shape를 실패 envelope로 숨기지 않는다. Opaque query/API/observation의 전체 문법을 검사하고 후속 원래 검증도 유지한다. 고정 배열은 원래 AST 경로처럼 길이 오류가 원소 shape 오류보다 우선한다.

기존 `reserveJSON(12)`와 AST decode의 기본 logical `20n + 64z + 65536 + 896v`를 `17n + 32z + 32768 + 80v`로 대체한다. 새 식은 snapshot 기본 예약에 `Check`의 기존 work `12v`·logical `12n`을 더한 것이다. Descriptor와 struct container/validation 예약은 별도이며, 고정 배열마다 길이 우선 검사를 위한 borrowed raw slice와 두 decoder의 `배열 bytes + 2048 B`, 추가 값 순회 work를 별도로 예약한다. 최종 typed pass의 값 수에는 같은 배열을 중복 합산하지 않는다. `nativeReservation`·`nativewire.Check`/`CheckStep`·source/history/points·query/API 검사는 그대로 모든 step/role에 적용한다. Receipt/row B5 및 전체 ordinary capacity는 이 단위의 완료 범위가 아니다.

### Reference·expected·witness의 typed decode (Refs #362 B4)

Reference connection의 registration/build/native projection과 response, query witness의 registration/raw/projection 및 eligibility의 witness registration·reference receipt r1·failure/footer는 같은 streaming 엔진을 쓴다. 기존 witness tuple의 `structural`·`expected`가 독립 capture 기대값이며 이 base에 별도의 ordinary expected wire 형식은 없다. 새 schema를 만들거나 기대값을 producer 결과에서 채우지 않는다.

Eligibility에 결속된 문서는 기존 eligibility shape를 따르며 `TreeNode`·`Capture`·predicate step·witness query/metadata/tuple의 하위 구조만 기존 witness 규칙으로 전환한다. 독립 witness 경로는 처음부터 witness 규칙을 따른다. Witness의 pointer-null, 지정된 nullable 배열, metadata/registration의 non-null 배열, `encoding_source`와 optional/조건부 tree 필드를 유지한다. Non-exact projection은 알 수 없는 subtree의 shape를 적용하지 않지만 전체 문법·중복·깊이·값 수는 먼저 검사한다. 독립 경로에 eligibility의 공유 logical/work 한도를 새로 부여하지 않으며 기존 document/depth/value·caller/wall 한도를 유지한다.

Reference row는 전체 strict token preflight에서 네 head scalar를 같은 typed 결과 객체에 수집한다(Refs #362 B5(b)). Head shape 오류는 전체 문법·중복·UTF-8·depth/value 검사를 마친 뒤 반환한다. Non-exact head → schema → FAILED exact head 또는 full shape의 순서를 유지한다. FAILED arm은 첫 extra member를 거부하며, 성공 arm만 기존 streaming full shape 검사와 한 번의 typed unmarshal을 적용한다. 별도의 전체 head shape 순회·typed unmarshal과 두 번째 syntax preflight는 없다. FAILED의 extra 필드·잘못된 failure code 및 뒤쪽 구문 오류의 우선순위는 기존과 같다. Receipt r1의 stdout/stderr base64와 row 분할·offset·hash 결속은 유지한다. B5(a)의 receipt r2·별도 stdout pin과 B5(c)의 capture span 예약 변경은 후속 범위다.

결합 row의 기본 logical 예약은 full decode의 `5n + 32z + 32768 + 80v`에 head/result 상태의 1,024 B를 더한다. Full descriptor·container·고정 배열 예약은 그대로다. Preflight에서 실제 관찰하는 root와 top-level member token마다 work 1을 추가하고, head 필수 필드 네 개 검사에는 work 4를 예약한다. FAILED/schema 거부 arm도 같은 보수적 preflight 예약을 사용하되 수행하지 않은 full shape·typed pass를 추가로 청구하지 않는다. 별도 head 문서 decode의 예약을 제거한 것이며 한도를 낮추거나 높이지 않는다.

각 decode의 기본 logical 예약은 snapshot과 같은 `5n + 32z + 32768 + 80v`이고 descriptor·struct container·고정 배열 비용은 별도다. 중간 AST를 만들지 않는 실제 경로에만 적용하며 후속 tree/capture/predicate 비교·guarded read·base64 비용은 별도 원장에 남는다. 기존 pointer·eligibility→witness handoff를 포함한 shape 값 수 한도도 별도 계수로 유지하되 제거된 AST 순회를 work로 계상하지 않는다. Reference의 source 좌표 검증은 분리된 span을 EOF까지 한 번 순회하므로 실제 검증 직전에 선택 source의 byte 길이를 `reference-points` work로 예약한다. Node 수만의 예약으로 긴 source 순회를 대신하지 않는다. 한도 변경이나 전체 ordinary 수용 근거가 아니다.

### Evidence graph의 순회 원장 (Refs #362)

Run 중복은 `(run, attempt, platform)`의 세 문자열을 key로 한 index에서만 찾는다. 정렬된 node 순서와 bucket의 뒤쪽 순서를 사용해 기존 all-pairs 검사의 finding code·순서·첫 실패를 보존한다. 같은 identity의 실제 중복 쌍은 모두 방문한다. Cycle은 정렬된 root와 원래 ref 순서의 DFS 한 번으로 최초 back-edge를 정하고, 역방향 edge의 도달성 memo로 cycle에 도달하는 owner를 구한다. 요청에만 있고 graph에는 없는 node는 cycle owner가 아니다. Carry 규칙은 node별로 policy 순서를 유지해 모으며, 각 carries ref는 그 node의 규칙만 검사한다. 파일 owner·approval·동일 guarded snapshot 결속은 유지한다. Paging의 별도 group 안전성 scan은 기존 `streamed-chain-plan`에서 계속 계상한다.

`N`은 입력 node 수, `E`는 모든 ref·file·identity entry 수의 합, `R=Required+CarryForward` 행 수다. `B(m)=bits.Len(m)`이며 `B(0)=0`, `S=Σ identity_count·B(identity_count)`다. `P`는 required identity마다 `m·(1+B(m))`의 합에 eligibility node 수와 `(1+허용 mode 수)`의 곱을 더한다. `D`는 replay/carry를 제외한 입력 node의 동일 run tuple 쌍 수, `J`는 carries ref마다 그 node에 해당하는 carry rule 수의 합이다. `Q`는 subject ref마다 해당 node의 file 수를 더한 값으로, replay-result 파일 목록의 반복 scan을 포함한다. 중복/잘못된 node도 예약에서는 포함해 실제 수용 graph보다 작아지지 않게 한다. `H`는 replay/carry 밖 node의 run·attempt·platform 문자열 byte 길이 합이다.

Graph work 상한은 `N + CarryForward + 16H + 64·(1+N+E+R+S+N·B(N)+P+D+J+Q)`다. 앞의 `N+CarryForward`는 estimator의 행 방문이고 `16H`는 estimator와 검증 index의 문자열 hashing 예약이며 무거운 처리 전에 차감한다. 64배 항은 estimator·node/role/file/ref 검사·집합 구축·관계 검사·DFS·역방향 closure·owner 판정·finding 생성·replay 파일 목록 scan의 정수 개수 순회와 정렬 비교를 포함한다. 실제 중복 finding `D`와 같은 node의 규칙 결과 `J`는 이차가 될 수 있으므로 제거하지 않는다. 이 상한은 collection 방문·비교 및 명시적 byte 처리의 logical work 원장이며 CPU instruction 수·Go map의 충돌 최악 시간 보장은 아니다. Replay-result decode·파일 hash/read·streamed plan 등 별도 원장은 중복 없이 유지한다.

기존 graph 문자열 logical 예약 `(fields+1)·(N+R+1)`을 유지하고 새 map·slice·역방향 edge를 위해 `512·(1+N+E+R)` B를 더한다. Estimator의 map 공간은 만들기 전에 일부를 예약하며 나머지는 graph 검증 전에 예약한다. Work32,000,000과 기존 context별 logical 한도는 바꾸지 않는다. 계측은 test-only Go overlay에서 실제 loop body 진입·DFS node 진입·정렬 comparator 호출을 세며, 동일 SQL core·기존 fixture·합성 사슬/fan-out/cycle 직전과 cycle에서 예약이 계측값 이상인지 확인한다. 이는 새로운 runtime 관측이나 ordinary 전체 capacity의 증거가 아니다.

## Parser-only paging 문맥

`full-input-utf8-paged-parser/r1`은 단일 작은 UTF-8 native-query source의 큰 grammar parser에 한정된다. 세 approval/registration/result r1과 witness r2의 필드 의미는 유지하며 Windows/Linux만 받는다. `Native.Grammar[original_path]`는 실제 `grammar-paged-descriptor` 물리 pin을 가진다. 실제 `Native.InvocationID`의 NATIVE_DRIVER node가 descriptor와 모든 part의 owner여야 하며, 조각은 동일 guard의 검증된 cached bytes에서만 소비한다. 참조하지 않은 paged member와 다른 owner는 거부한다. 전체 logical pin은 승인된 원본 plain build manifest에 대조하고 선언된 descriptor 해시만으로 승인하지 않는다. 정확한 분할 규칙과 변경되지 않는 small 예산은 [계측 계약](instrumented-query-reference.md#216-parser-only-paging)이 소유한다. 이 문맥에서 eligibility 검증이 직접 시작한 원장(approval 결속)의 논리 처리량은 1.5 GiB이며 다른 small 한도는 같다. `VerifyQuerySemantics`·Replay처럼 원장을 상속하는 경로는 이 문맥에서도 1 GiB다. 큰 source/CST 또는 새 macOS 검증을 추가하지 않는다.

`full-input-utf8-paged-binaries/r1`은 paged-parser 규칙을 포함하되 16 MiB를 넘는 grammar parser를 0개 또는 1개 허용한다. driver executable과 reference library 중 하나 이상이 descriptor로 보존돼야 하며, 이 조건은 approval에 결속된 registration의 executable·library pin role로만 판정한다. NATIVE_DRIVER의 executable이 `driver-paged-descriptor`이면 descriptor와 모든 part는 그 invocation node가 소유한다. 재결합한 logical SHA-256을 approval의 `executable_sha256`과 대조하고, 사용하지 않거나 다른 node가 소유한 `driver-paged-*` member는 거부한다. 다른 context에서 descriptor role의 executable은 승인 값과 무관하게 거부한다. reference library의 descriptor는 그 library node가 소유하며 사용하지 않는 `library-paged-*` member는 거부한다.
