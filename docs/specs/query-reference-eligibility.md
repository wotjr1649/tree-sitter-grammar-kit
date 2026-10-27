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

Witness r2는 기존 QueryWitnessRegistration의 field와 `native`를 가진다. Native는 `protocol`, `request_id`, `node_limit`, `api`, `points`, `runtime_manifest_sha256`, `producer_sha256`, `executable_sha256`, `producer_abi`, `observation_run_id`, `history_start`, `process`다. Producer ABI는 `language_version`, `runtime_language_version`, `runtime_min_compatible`, `symbol_count`, `field_count`다. Process field는 저장 request와 같은 Path/Args/Dir/Env/StdoutBytes/StderrBytes/Wall/Grace/Memory(Bytes,Hard)/CgroupParent다. 시간 단위는 Go duration의 nanoseconds다.

독립 승인한 native/r4 기대값과 실제 source에 ApplyEdits를 적용한 모든 version bytes/hash/points로 nativewire Expect를 구성한다. CheckHeader/Step/Open/End, reuseobs.Decode/CaseClaim/Judge, querywire/querypolicy 및 comparator를 재사용한다. Mandatory logger prefix PASS만으로 full logger COMPLETE를 대신하지 못한다. Build manifest는 승인 ProducerSHA256에 결속하고 full runtime/driver closure와 grammar physical pins를 대조한다. Grammar 역할은 parser/scanner/header이며 parser는 정확히1개다. Canonical quoted argv와 version/runtime/parser/scanner*/driver/shim/link의 필수 순서·개수 및 version argv를 검사한다. 현재 고정 runtime manifest의 ABI 호환 범위13..15도 승인/raw 값과 대조한다. Grammar의 language/symbol/field count는 caller가 독립 승인한 기대값과 raw producer를 대조하며 C source를 새로 해석하거나 컴파일해 이 숫자를 추출하지 않는다. 현재 내장 runtime manifest만 완결 검증 대상이며 과거 manifest는 BLOCKED다. 실제 실행 여부는 승인 provenance의 신뢰 범위다.

Observation 필수 field는 `query`, `step`, `tree_role`, `source_bytes`, `source_sha256`, `invocation_id`, `query_id`, `response_ordinal`, `byte_offset`, `byte_length`, `sha256`다. Witness의 required 밖 observation·중복·불명은 거부한다. Source version과 raw stdout의 정확한 response slice에 결속하며 현재 reference의 독립 full CST·metadata·capture를 대조한다. 유효하지만 다른 CST는 BLOCKED다. 형식이 유효한 current reference FAILED 응답은 producer의 exit1과 일치해야 하며, 성공 exit0 등의 모순은 FAIL이다. CST 동등성 전에는 capture 개수와 match/pattern/capture/name 순서를 비교하며 node index 등 CST 종속 전체 비교는 동등성 확보 뒤에 수행한다. 별도로 승인된 metadata·anchor의 독립 모순은 계속 FAIL이다. Native full CST가 없으면 제공된 reference anchor 자체의 전체 모순 검사도 유지한다. Required 밖도 포함한 모든 제공 full native tree의 EOF·실제 source points·내부 CST 일관성을 tuple 집계 전에 검증하며, 확인된 source 모순은 FAIL이고 검사 중단은 오류로 전파한다. Current 입력 slot이 없을 때 nil·빈 digest로 모순을 만들지 않으며 따로 승인된 observation/source version·request의 library hash에 대한 검사는 유지한다. Build·Control·Native 그룹이나 적용 가능한 observation이 없으면 독립 기대값이 없는 비교는 생략하고 해당 의무를 BLOCKED로 남긴다. Batch의 한 query observation 누락은 다른 query의 검증을 막지 않으며, 제공된 request·pin·response의 독립 모순은 계속 FAIL이다. Reference 및 native의 중첩 query·API·logger JSON 형식 오류는 이 API에서 KindInvalidInput으로 반환하고 Native raw의 response·r4 producer·step·tree와 compact 배열은 필수 member 및 primitive type도 검사한다. Nullable wire arm은 step의 edit/route/fresh, tree의 form/types/fields/nodes/errors/declarations/partial_trees, declaration의 name이며 중첩 query/API/logger arm은 별도 strict 검사를 통과해야 한다. Native 그룹이 생략되거나 source가 없어 version을 구성하지 못해도 등록된 모든 NATIVE_DRIVER invocation이 제공한 raw의 형식 검사는 수행한다. 모든 native invocation에서 자체 approved/stored request의 strict 일치·process 한도/env·실행 파일 owner/pin/hash/path·execution/stream, raw envelope/ABI/capability와 logger의 자체 승인 binding·역할·count/status 일관성을 확인한다. 확보한 자체 source pin으로 초기 source transport·UTF-8·full tree EOF/points/CST를 확인하며, query input은 확보한 각 pin의 길이1..65,536 bytes와 ID별 bytes를 대조하되 map에서 요청 순서를 추론하지 않는다. 모든 완료 full tree의 root EndByte는 초기/후속 actual source 존재나 선택 group과 무관하게 그 step 자체의 SourceBytes와 같아야 한다. Query 원문이 없어도 status·metadata·capture accounting과 확보한 full node 연결을 검증하고, compile-error class/code/shape와 query_ms=0·matches=0·빈 types도 유지하되 실제 원문의 offset/point 대조만 BLOCKED로 남긴다. 0개 query의 명시적 null/빈 배열은 허용하며 확보한 query pin이 응답에서 빠지면 FAIL이다. 제공 step의 순서·형태·내부 status/completion 모순도 검사한다. 초기 outer/step0 길이를 직접 대조하고 edit의 byte 순서·전후 길이 관계·빈 edit·내재 point 관계 및 확보한 초기 source의 첫 edit old point를 검증한다. 실제 후속 source·정확한 edit history의 증명과 구분한다. EOF/CST/point의 확정 모순은 공통 경로에서 먼저 기록하고 중단 오류를 반환하여 연결된 실패 counter를 보존한다. 후속 source를 raw 자체로 증명하지 않으며 전체 edit history·query 순서·build closure·required tuple 연결만 선택된 witness/group에 의존한다. 정상 extra에 선택 history가 없다는 이유로 전역 BLOCKED나 연결되지 않은 tuple 실패를 만들지 않는다. 그룹·source 기대값에 의존하는 비교는 해당 의무의 BLOCKED로 남긴다. 기존 r1 connection과 native r2의 판정 의미는 유지한다. Native version이 없더라도 보관된 current source의 UTF-8·EOF·points와 predicate를 검사하며, 확보된 빈 query bytes도 hash 모순 검사에 사용한다.

## 유한 예산과 검증 범위

공유 한도는 physical member/JSON 각각16 MiB, 총 읽기256 MiB, file10,000개, wall120초, output16 MiB, identity1 MiB다. JSON depth32/decode·shape 각각2,000,000 values, 공유 work32,000,000, 논리 처리량1 GiB를 적용한다. Snapshot은 각30,000/합계60,000행, 행64 MiB/선언 합계1 GiB다. JSON 반복 pass·graph pair/cycle·filter/predicate/comparator·unit stdout scan·해시·복사·output sealing의 보수적 비용을 무거운 처리 전에 차감한다. Compact node가 공유하는 type/field 문자열은 node별로, capture가 재참조하는 type 문자열은 capture별로 계상하며 tree digest와 capture 연결에 취소 checkpoint를 적용한다. Partial conversion마다 반복하는 공유 field table의 길이 곱도 사전 계상하고 table·node·partial 순회에 같은 중단 검사를 적용한다. Batch 분할은 예상 응답 수와 footer에 필요한 행 수로 제한한다. Member/tool/input 및 replay-result 예약의 중단 오류는 모순 finding으로 바꾸지 않고 반환하며 이미 확인한 FAIL만 유지한다. 사전 invocation 결속의 실패도 해당 Build/Control 및 연결된 tuple counter에 즉시 보존하며 후속 미완료 상태로 덮지 않는다. Caller가 이 한도를 높이는 옵션은 없다.

정상/변조 synthetic 검사는 외부 승인 경계의 동작 검사이며 runtime 자격 근거가 아니다. 보관한 Linux raw192는 요청/receipt/stderr·identity·snapshot·summary를 부분 수용하는 근거다. 실제 binary/library/입력/current-r4/승인 graph가 없으므로 전체 PASS로 승격하지 않는다. Producer 보관 확대·claim/reducer/inventory/fan-in·전체 Windows/Linux route 및 C# 실제 재사용 증명은 후속 단위이며 이 API 구현으로 Issue #216을 닫지 않는다.

## Parser-only paging 문맥

`full-input-utf8-paged-parser/r1`은 단일 작은 UTF-8 native-query source의 큰 grammar parser에 한정된다. 세 approval/registration/result r1과 witness r2의 필드 의미는 유지하며 Windows/Linux만 받는다. `Native.Grammar[original_path]`는 실제 `grammar-paged-descriptor` 물리 pin을 가진다. 실제 `Native.InvocationID`의 NATIVE_DRIVER node가 descriptor와 모든 part의 owner여야 하며, 조각은 동일 guard의 검증된 cached bytes에서만 소비한다. 참조하지 않은 paged member와 다른 owner는 거부한다. 전체 logical pin은 승인된 원본 plain build manifest에 대조하고 선언된 descriptor 해시만으로 승인하지 않는다. 정확한 분할 규칙과 변경되지 않는 small 예산은 [계측 계약](instrumented-query-reference.md#216-parser-only-paging)이 소유한다. 큰 source/CST 또는 새 macOS 검증을 추가하지 않는다.
