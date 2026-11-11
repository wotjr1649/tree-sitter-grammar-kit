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

## #362 ordinary admission 입력 범위

P2-A source-only planner는 이 eligibility verifier를 호출해 실행 권한을 만들지 않는다. 제공된 asset 본문의 pin·길이·hash 및 metadata shape, 원문 history와 선언된 member 크기를 검사하고 고정 비용과 reference 추정을 기록한다. Full CST/capture/API/fact의 정확한 개수와 검증된 출력 상한은 아직 지원하는 admission 근거가 아니므로 `upper_bound:null`과 BLOCKED를 유지한다. 이를 이 API의 actual guarded read/work/logical charge로 간주하지 않는다. Ordinary API는 이 문서의 caller 승인·snapshot·physical member 규칙을 같은 session에서 재사용한다.

Ordinary shard admission은 기존 small 상한인 physical file 16 MiB, shared read 256 MiB, work 32,000,000, logical 1 GiB, 10,000 entries, wall 120,000 ms를 유지한다. Native frame 파일은 4-byte header를 포함해 16 MiB 안에 있어야 한다. Parser-paged 또는 binary-paged closure 표기는 eligibility가 직접 시작할 때 허용되는 기존 1.5 GiB 예산을 ordinary semantic/replay 실행으로 확장하지 않는다. 상속된 ordinary pass는 1 GiB다. 상한이나 candidate body를 입증하지 못한 shard는 `CAPACITY_UNPROVEN`/BLOCKED로 남고 ID 집합에서 제거되지 않는다. SQL plan의 BLOCKED는 실행 실패가 아니라 사전 capacity 근거의 미완료다.

P2-A의 과거 full typed decode 모형은 매 ordinary pass의 qualification inventory 및 registry 비용을 353,877,949 B로 계산했다. 현재 K4 경로는 inventory 전체 token·전역 검사와 선택 route typed decode를 수행하고 registry만 전체 typed decode하므로 이 과거 값을 현재 원장으로 그대로 적용하지 않는다. 이 값은 query-slot inventory decode를 포함하지 않으며, slot 및 새 ordinary documents의 추가 charge는 별도 미확정이다. 과거 reuse core anchor와 알려진 set layer 진단값을 뺀 뒤 고정 decode만 차감해도 body 이전 잔액은 PostgreSQL −297,750,339 B, TSQL −341,653,347 B다. 이는 #366 after-reuse anchor를 쓴 정적 진단이지 current exact closure 실측이 아니고, #366 구현은 P2-A base에 없다. P2-A는 K0–K4 또는 다른 구조 절감과 body density 추정으로 이 잔액을 상쇄하지 않으므로 두 SQL route 모두 capacity BLOCKED다. 세부 산술 및 reference row의 수정 추정은 [ordinary capacity 계약](instrumented-query-reference.md#기존-budget-model의-누락과-reference-재계산)을 따른다.

Reference row도 receipt Base64, head/full 두 차례 JSONL decode 및 structural query를 포함한 `reserveReference` 비용을 반영하면 기존 P2 모델보다 약 세 배 크다(예: TSQL nominal 기존 58,141,956 B, 수정 추정 177,534,284 B; dense 기존 232,269,572 B, 수정 추정 708,656,264 B). 이 역시 추정이지 실측 상한이 아니다. `nativeReservation`, API/fact, 남은 U 비용과 expected output body의 안전 상한은 입증되지 않았다. Planner의 cost fields는 per-pass fixed decode와 native/reference 비용을 따로 드러내며, 이 산술 추정값은 admission upper bound로 취급하지 않는다. P2-A 계산은 #366 재사용 구현의 최적화를 가져오지 않으며, 역사 anchor를 이용한 수치는 capacity 진단으로만 표시한다.

P2-A 산출은 strict preregistration과 shard-plan 무결성만 검사한다. Approval, runtime observation, evidence verification이나 `VerifyQueryReferenceEligibility` 판정은 생성하지 않으며, 본 절의 기존 eligibility API가 ordinary registration을 지원한다고 간주하지 않는다.

`OrdinaryBinding.Runtime` pin은 caller의 profile·plan identity에 결속된다. 해당 runtime 자료가 `Members`에 별도로 선언되지 않았다면 그 본문 검증과 크기는 `declared_member_bytes`에 포함되지 않으며 current closure의 미확정 비용으로 남는다. 이 필드를 전체 closure 크기나 actual runtime 검증으로 읽지 않는다.

### #362 승인된 shard plan commitment

Selected ordinary 입력은 전체 plan을 매 pass 다시 decode/coordinate 전개하지 않는다. Full P2-A plan은 source-only planner가 고정 P0 source에 대해 106,208개 ID exact union, 중복/overlap/gap 및 각 shard coordinate를 검증한 다음, caller가 검토할 수 있는 complete-plan pin과 공통 header에 결속된 정렬 Merkle commitment를 발급한다. Pass의 private approval은 full plan pin과 commitment pin을 모두 보유한다. 기존 r8 manifest와 run-binding/r2의 `shard_plan_pin`은 full plan bytes를 계속 가리키고 commitment의 `complete_plan_pin`에 대조한다. 별도 approval `plan_commitment_pin`은 선택 leaf와 포함 증명을 root에 결속한다.

Caller approval은 commitment를 승인된 전체 증명 경로와 묶는 외부 신뢰점이다. 해시 자체가 arbitrary commitment를 semantic-valid로 만들지는 않는다. Merkle proof는 선택 shard의 exact bytes, index, ordered selection 및 공통 header를 승인된 root에 연결하고, pass는 선택 coordinate와 ID/digest를 다시 만든다. 전체 union 보장은 승인된 full plan을 만든 planner의 성공한 exact-union 검증에서 이어진다. 이후 승인되거나 pin이 달라진 plan이 없으면 full plan을 매 pass 읽지 않아도 안전하다. SHA-256 충돌은 현실적으로 탐지 가능한 입력 변조로 취급하지 않으며 collision resistance를 명시적 가정으로 둔다.

선택 registration decoder도 전체 원문을 읽고 strict token 및 얇은 global projection으로 duplicate key, shape, 전역 ID·coordinate 중복/관계 오류를 확인한다. case/query/step/tree-role tuple의 exact union과 교차 참조를 검사한다. 비선택 coordinate ID hash 및 source/asset·고정 scope의 local 의미 검증은 planner가 전수 검증한 registration pin에 대한 승인 provenance를 상속한다. 전체 typed decode·history/query/required 결속은 선택 case와 route에 한정한다. Full registration 원문 read, global scanner와 selected registration work는 shared budget에 들어간다. Registry와 qualification inventory의 기존 고정 pin/검증도 계속 수행한다.

선택 plan의 proof 예약은 실제 Merkle hashing 전에 work `H+S+512D+2048`, logical `8(H+S)+256D+4096`이다(`H`: header 원문 bytes, `S`: 선택 shard 원문 bytes, `D`: sibling 수). Header 의미 검증은 아래 row·문자열별 work 합과 별도 logical `4H`를 예약하고, strict typed decode는 기존 container/shape 예약을 유지한다. 선택 coordinate ID 하나는 namespace와 문자열 필드 길이 합 `n`에 대해 canonical JSON escaping 상한 `u=6n+512`를 먼저 예약한다(work `u`, logical `8u+1024`). 선택 shard ID digest와 coordinate/map 관리 비용도 별도이며 비선택 ID hash 비용을 지불했다고 표시하지 않는다.

Registration은 한 strict token scan에서 전체 shape와 case/query/required header, `version_pins` 수와 row span을 함께 수집한다. 구문·중복·깊이·값 한도를 끝까지 검사한 뒤 첫 shape 오류를 반환하고, JSON member 순서와 무관하게 기존 header → case → query → required 의미 검사 순서를 유지한다. 전역 유일성과 exact tuple union은 이 cached header로 대조하며 원문 subtree를 다시 해석하지 않는다. 선택 case·route query·required만 전체 typed decode한다.

Registration scan의 원장은 raw 크기 `n`에 대해 `ordinary-registration`에 decoder/token 문자열·오류 버퍼 `4n+32768` B와 고정 reflection shape 비용을 먼저 예약한다. `ordinary-registration-index`는 실제 읽은 token마다 work 1, 시작 value마다 logical 16 B, schema에 등록된 member name마다 logical 16 B, 알 수 없는 member나 shape 없는 객체의 name에는 폭이 제한되지 않은 중복 이름 table을 포함한 logical 256 B, 객체의 필수 field 검사마다 work 1을 예약한다. `ordinary-registration-headers`는 header 배열을 용량 8부터 두 배로 늘리기 전에 새 backing array의 전체 `capacity × sizeof(row)` B 및 실제 이전 row 복사의 work를 예약한다. 이전 backing array의 비용도 누적하며 row span은 이 배열에 포함한다. 비선택 행에 typed container가 생기지 않으므로 과거 shape scanner의 객체별 `256+512×fields` 예약은 이 경로에 적용하지 않는다. Case/query/required의 map·tuple 예약과 선택 행의 typed decode는 별도다. Required ID는 길이·`qsem1:` prefix와 lowercase hex suffix를 검사하며 suffix 검사와 64-byte key 복사를 한 번에 수행한다. 전역 ID 유일성은 이 고정 key의 map으로 확인하고 전역 tuple exact union은 그대로 대조한다. Required map은 row마다 logical 96 B, work 1을 먼저 예약하고, 각 row 처리 전 work `8+len(id)+len(case)+len(query)+len(role)`를 더한다. Prefix/suffix의 실제 byte 방문은 합쳐 `len(id)` 이하이고 8은 고정 row/map 연산을 덮는다. Tuple 문자열 길이 항은 cached tuple 대조의 보수 상한이며 Go runtime의 hash instruction 수를 뜻하지 않는다. 선택 ID 재구성·typed decode는 기존 별도 label에 남는다. 과거 전체 header의 raw bytes별 unmarshal 예약은 실제 unmarshal 제거와 함께 사라진다. Paging identity의 hash 예약은 새 commitment를 포함한 8개 caller pin 전체를 포함한다. 이들 logical 예약은 heap peak나 CPU instruction 수의 실측값이 아니다.

선택 plan header 의미 검사는 commitment로 승인된 header라도 모든 route·binding·member와 교차 host 참조를 계속 확인한다. `ordinary-plan-header-validation`의 work는 raw header bytes의 32배 대신 아래 합을 검사 전에 예약한다. 원문의 JSON strict decode·commitment pin·proof hash와 logical `4×len(header)`는 별도로 유지한다. `s(x)`는 decoded 문자열 byte 길이의 합이며, 각 항에는 비용을 계산하는 사전 순회도 포함한다.

| 항목 | work |
|---|---:|
| 고정 schema/namespace/limits/budget 비교 | 1024 |
| Registry/Witness/Profile/Assets pin 각각 | `64+64×len(path)+8×len(sha256)` |
| CapacityUnknown 문자열 각각 | `32+8×len(value)` |
| Budget.SQL row 각각 | `96+8×len(route)` |
| Budget.ReferenceRoutes row 각각 | `64+8×s(route,set)` |
| Routes row 각각, NativeQueryIDs 각각 | `128+8×s(route,set)`, `32+8×len(query)` |
| Bindings row 각각 | `128+8×s(route,platform,subject,closure)` |
| Binding member 각각 | `128+8×s(owner,caseOrBatch,role,sha256)+64×s(path,pin.path)` |
| Route와 두 host의 각 교차 조회 | `32+8×s(route,host)` |

비교하지 않는 reference estimate 본문, note 문자열 내용, grammar/runtime와 registration pin 목록의 bytes를 이 의미 검사 label에서 다시 전수 순회한 것으로 계상하지 않는다. 이들 원문은 이미 strict decode와 commitment pin/hash 대상이다. `ReferenceModel`/`SQLModel`은 비어 있지 않은지만 검사하며, `ReferenceRoutes`는 route/set 및 density만 검사한다. 이 예약 교정은 header 의미 검사를 생략하거나 commitment를 semantic 증명의 대체물로 삼지 않는다.

### #362 ordinary base의 실제 순회 예약

Ordinary adapter의 `semanticProjectionInputs`는 raw의 producer/status/code, 각 step의 route/restarted 및 declarations를 대조하며 full raw CST/query/API를 직렬화하지 않는다. Ordinary 호출만 full raw bytes 대신 소비 필드의 JSON escaping 상한 `B`를 전달한다. `B`는 고정 producer/응답 overhead 1,024 B, step마다 route/null/container overhead 512 B, declaration마다 256 B, producer query/API/predicates/reuse-observation·응답 status/code·declaration fact/node-type/status의 문자열 bytes 6배의 합이다. 기존 `semantic-base-inputs` 식 work `16B+1`, logical `16B`를 적용하고 크기 산정의 step/declaration 방문은 `ordinary-base-input-size`에 별도 예약한다. 실제 marshal·AST·구조 대조를 생략하지 않는다.

Root/step 객체의 field lookup도 `ordinary-base-input-size`에 각 member 수 256배로 예약한다. 이는 estimator와 comparator의 최대 8회 lookup 및 길이 32 이내의 고정 field name 대조를 덮으며, 각 role tree의 declaration lookup은 해당 member 수 32배를 예약한다. CST 내부 node 객체는 이 index 비용에 포함하지 않는다. 고정 산정 방문은 root 3단위, step 5단위와 declaration 행 수다.

Ordinary base만 `semantic-base-replay` decode를 work `n+1`, logical `16n`에 예약한다(`n`: projection 전체 bytes). 이미 strict decode한 ordinary projection의 significant-byte 재scan을 하지 않으며 `rcCase` unmarshal과 공용 reducer의 모든 검사는 유지한다. Replay 직전 추가 work는 `2n+128`에 다음 상한을 더한다: step/기록 expectation/reference와 tree/query/identity/capture 행 64단위, full node 256단위, node type/field bytes 8배, capture name/type bytes 16배, 각 step observation JSON bytes 8배. 등록 expectation마다 contains/anchor 행과 고정 검사를 64단위, contains 문자열 bytes를 4배로 더한다. Contains 검사는 대상 node마다 work 64와 type bytes 4배, logical 256 B를 더한다. Anchor마다 `대상 node 수 × (64+4×len(anchor.Type)) + 4×대상 node type bytes`를 더해 반복 검색을 포함한다. 크기 산정 자체의 step/query/capture/node/expectation 방문은 `ordinary-base-replay-size`에 별도 예약한다.

Identity는 tree/summary/composite/inline의 모든 행을 포함한다. 각 행의 role/schema/SHA bytes와 1의 합에 `8×(bits.Len64(n)+1)`을 곱해 `checkSeen`의 정렬·문자열 대조를 예약한다. Identity 수는 projection bytes `n` 이하이므로 이 식은 실제 중복 제거 뒤의 정렬 수에도 적용된다. Estimator의 identity 행 방문도 별도 size label에 포함한다.

이 식은 collection 방문·명시적 문자열/JSON/hash bytes의 work 상한이며 CPU instruction/heap/RSS 실측이 아니다. Raw·projection·checked CST의 결속이나 공용 reducer의 판정을 생략하지 않는다. Legacy single small semantic, r4–r6, r10은 기존 `16×significant+1` work와 `16n` logical 및 full-raw input 예약을 유지한다. Fact 재계산, 최초 FAIL 보존 및 공유 한도도 유지한다.

### #362 ordinary base fact 도출 예약

Fact 도출은 version → incremental/fresh role → fact query마다 예약한다. Source version SHA, ordered query suffix SHA와 pin 대조, 저장 record decode·payload 대조는 기존 별도 label에 남는다. Declaration/XML 도출은 source 본문을 읽지 않으므로 전체 source bytes를 capture 수의 제곱에 곱하지 않는다. 이전 공통식 `64n²(n+text)` work / `1024n(n+text)` logical 대신 실제 종류와 순회 경계에 따라 아래를 예약한다. 한도·fact 의미·독립 expected 조건은 유지한다.

`C`는 해당 query의 capture 수, `n=C+1`, `M`은 각 capture가 참조하는 Name/Type 문자열 byte 길이의 합이다. Capture 크기 산정 자체는 `ordinary-base-fact-index`에 work `C+1`을 먼저 청구한다. Capture 변환·초기 index와 XML count map은 `ordinary-base-fact-derive`에 work `64n+8M`, logical `1024n+4M`을 예약한다. XML 결과는 capture 이름별 count이며 source 크기와 무관하다.

Declaration의 인접 level owner 탐색과 locator frontier는 별도 checked private derivation을 사용한다. 같은 node가 여러 match/parent 경로를 통해 반복 전개될 수 있어 `C²×depth`를 전체 상한으로 가정하지 않는다. 다음 항을 해당 순회·할당 전에 누적 예약하고 초과 시 `RESOURCE_LIMIT/ELIGIBILITY_WORK_LIMIT`으로 중단한다. 기존 public `DeriveDeclarations`와 ordinary 이외 호출은 budget 없는 경로로 같은 도출 의미를 유지한다.

| Declaration 작업 직전 | work | logical B |
|---|---:|---:|
| match의 capture 수 `m` | `32+32m` | `512+512m` |
| capture 이름 `s` 분해·숫자 parse | `8len(s)+1` | `32len(s)` |
| capture에서 읽은 depth `d`의 빈 level 포함 반복 | `max(d,0)+1` | 0 |
| 자식 하나의 직전 level 부모 후보 `p`개 검색 | `16+p` | 256 |
| 길이 `k`의 key/candidate/frontier clone·sort·compact·append | `64(k+1)(bits.Len64(k)+1)` | `512k` |
| declaration locator `s` 분해 | `64+8len(s)` | `256+64len(s)` |
| 해당 level의 frontier `k`개 방문 | `1+k` | 0 |
| 결과 `k`개, `t=len(fact)+len(node)` | `k(64+8t)` | `k(256+4t)` |

Huge numeric depth는 level 반복 전에 거부하며, frontier가 여러 경로로 증가해도 다음 복제·정렬·출력 전에 예약하므로 선행 단계의 여유로 초과 작업을 숨기지 않는다. 초기 capture/index 예약은 match grouping과 key 수집도 덮는다. Sort 항은 실제 comparator 횟수의 보수 상한이며 원소별 비교는 node/item 정수 비교다.

TSQL dynamic SQL은 위 기본 예약에 work와 logical 각각 `128n² + 64(E+1)A + 64EWS`를 더한다. `E`는 `exec` capture 수, `W`는 `errwith` capture 수, `S`는 source bytes, `A`는 source 안의 유효 capture span 길이 합이다. 고정 개수의 match pair index/정렬·반복 child/known-miss 탐색, exec별 text 검사, exec×errwith의 반복 공백 gap/정규식 span 검사를 덮는다. 문자열 decode와 variable 결과도 포함한다. 각 곱은 overflow 검사 후 별도로 청구하며 아직 실행하지 않은 query의 예산을 소급 청구하지 않는다. 다른 route의 dynamic SQL은 기존 보수적인 `64n²(n+S+M+1)` / `1024n(n+S+M+1)` 식을 유지한다.

Test-only overlay로 실제 loop/comparator 방문, hash/span 입력 bytes와 구체화한 capture·결과 payload를 별도로 관찰한다. PostgreSQL/TSQL 실제 owned 입력과 많은 capture·긴 문자열·큰 XML 및 반복 declaration frontier에서 각 query의 `계상 ≥ 실제`를 확인한다. 이 logical payload 검사는 process 전체 할당이나 heap/RSS peak의 실측이 아니다.

## #362 P2-C ordinary eligibility activation boundary

P2-C는 `VerifyOrdinaryQuerySemantics`의 private binding/session adapter를 연결했다. Required 분모는 public request의 필수 caller `Selection`에서 commitment proof 및 registration 검증으로 먼저 고정하며, 이후 `approval.ordinary`의 동일한 pin bundle이 맞아야만 session을 bind한다. Caller selection은 approval이 아니고 비어 있거나 invalid하면 input error다. Approval이 없으면 selected obligations는 BLOCKED이며 API 결과는 `Complete=false`/`ProcessingComplete=false`다. 기존 공개 `VerifyQueryReferenceEligibility`는 `ordinary-batch-full-input-utf8/r1`을 계속 거부한다. Ordinary API만 `tsgk-query-reference-eligibility-approval/r1` 안의 strict `ordinary` binding을 받아들인다. Binding은 실행 `profile_pin`, 계획 `planning_profile_pin`, ordinary `registration_pin`, `asset_registration_pin`, `shard_plan_pin`, `plan_commitment_pin`, `reference_selection_pin`, `run_binding_pin`, `closure_encoding`, candidate/cohort/run 및 선택 shard ID 전체를 묶는다. 같은 approval의 기존 registration digest는 eligibility registration bytes를 계속 묶는다. 일반 eligibility의 native witness registration은 보내지 않으며 빈 bytes가 필수다. 이 경계를 통해 ordinary batch reader가 구형 single-case witness를 새 의미로 해석하지 않는다.

Ordinary 호출의 eligibility registration은 ordinary context, `Native == nil`, 빈 legacy `observations`를 사용한다. Build·control invocation, evidence graph, owned member와 caller approval은 기존 typed eligibility 경로에서 같은 `run`, `eligibilityBudget`, held `os.Root`, evidence snapshot으로 검증한다. Ordinary batch reader가 tuple을 직접 검사하므로 기존 단일 native `verifyNative`와 그 tuple witness를 실행하지 않는다. Approval 안의 ordinary cohort ID는 현재 eligibility cohort ID와 같아야 한다. Pin이나 cohort가 불일치하면 session은 bound가 되지 않고 결과에 FAIL이 남는다. 기존 eligibility public entry에서 이 block을 전달할 방법은 없다.

`closure_encoding`은 `plain`, `parser-paged`, `binary-paged` 중 하나를 caller가 고정한다. Paged 선택은 해당 binding과 같은 eligibility snapshot 안에서만 private verified part authority를 만든다. Root·session·approval·cohort가 다른 part record는 재사용할 수 없다. 연결된 authority가 없으면 기존 guarded full-read/hash 경로를 사용한다. Ordinary 호출은 semantic/replay로부터 1 GiB 원장을 상속하고 paged encoding으로 eligibility 전용 1.5 GiB 직접 진입 한도를 받지 않는다. Inventory 및 registry 입력은 fixed source pin을 먼저 확인하고 `parseQualificationInventoryRoute` 선택 decode와 typed registry decoder를 공유 원장에서 실행한다. Valid HARD receipt를 확인한 focused Windows JSON public call은 actual owned r8로 tuple 1/1을 VERIFIED로 끝냈으며 native child는 1 GiB/30 s, `JOB_OBJECT` cleanup, 잔류 자식 0이었다. 여섯 build-step receipt는 각각 4 GiB HARD/120 s였고, stricter native cap은 test-only runner overlay로 강제했다. 제품 기본 4 GiB native operation policy는 바뀌지 않았다. 원장 값은 logical 206,346,731 B, work 7,835,933, read 11,216,999 B다. 이 입력의 complete-plan scaffold는 전체 P0 source-only exact-union provenance가 아니고 approval/reference row도 synthetic이므로 선택 JSON 외의 closure나 전체 P0를 입증하지 않는다.

이전 P2-C SQL 측정은 full admission으로 이어지지 않았다. PostgreSQL `alt-postgresql-sql-B04-22`의 owned r8 selected body/reference/adjudication/fold/identity/seal 원장은 logical 697,519,126 B, work 14,102,601, read 134,443,515 B였다. Independent 10-node grammar-authored CST와 9 PostgreSQL9-anchor metadata, empty captures/declaration facts를 비교했지만 9 tuple은 base fact-reproduction gate 미재계산으로 `ORDINARY_PREREQUISITE_INCOMPLETE`/BLOCKED였다. TSQL `alt-tsql-B01-r001`은 binary-paged owned r8 사용 중 `ordinary-base-tree-projection`에서 logical usage 1,073,275,806 B까지 갔고, 다음 1,731,872 B 예약에 1,265,854 B가 부족해 `ELIGIBILITY_WORK_LIMIT`으로 멈췄다. Work/read는 각각 31,403,098/126,923,139 B였다. TSQL의 40 reference rows는 synthetic placeholders이고 expected assets는 UNAVAILABLE이므로 이 중단 뒤 비용은 측정하지 않았다. 두 SQL route 모두 `CAPACITY_UNPROVEN`/BLOCKED다. 후속 registration·required ID·plan header 최적화 뒤 2026-11-12 같은 owned-native 선택 TSQL 공개 경로는 seal까지 logical 942,338,579 B, work 31,240,589, read 126,923,146 B에서 끝났다. Expected 부재와 synthetic reference는 유지되므로 이는 해당 fixture의 전체 제어 흐름 측정이며 full SQL admission/semantic PASS가 아니다.

2026-11-12 ordinary base fact 후속 검증은 위 PostgreSQL의 미재계산 gate를 해소한다. Adapter는 같은 guarded session에서 fact pack·mapping을 한 번 확인한 뒤, P2-B2의 ordered suffix/query SHA와 mapping revision을 결속하고 편집 후 각 source version 및 incremental/fresh 각각의 raw captures로 declaration/dynamic SQL/XML facts를 다시 도출한다. 전체 step·role·query의 저장 `step_facts`와 step0 base `facts` projection을 대조하고, 기존 case claim의 step0 의미를 유지한 재계산 값을 공용 `semanticBase`에 전달한다. 저장 fact 자체는 독립 expected가 아니며 tuple의 metadata/CST/capture/fact 독립 비교를 생략하지 않는다. Legacy single small semantic admission은 fact pack을 받지 않으며 r4–r6와 r10 의미·한도를 바꾸지 않는다.

2026-11-12 TSQL 독립 expected 후속 검증은 선택 shard `alt-tsql-B01-r001`(원문 267 B, 4 edits, 5 versions, 8 query, 72 tuple = 8 query × [step0 incremental + step1–4 incremental/fresh] 9개)의 expected asset 107개(metadata 8, full CST 9, ordered captures 72, facts 18)를 손으로 작성해 pin한다. CST는 prepared `grammar.json`의 production(`program`/`batch`/`statement`/`select`/`term`/`field`/`invocation`/`window_function`/`set_statement` 등)과 external scanner 원문(GO 줄 끝, `marginalia`, `QUOTED_IDENTIFIER` 상태)에서 도출하고 byte 범위만 원문에서 토큰을 찾아 계산한다. 쿼리 metadata는 query 원문의 pattern 수와 capture 이름 최초 출현 순서이며, ordered capture는 손으로 쓴 tree와 pinned runtime `query.c`의 capture cursor 규칙(finished-state heap의 start byte·pattern index·완료 순서, match id는 첫 yield 시 부여, 복사 state는 원본의 id·consumed 수를 물려받음)에서 도출한다. 작성 시 parser·native·reference 출력은 사용하지 않았고 native 첫 실행 전에 asset 본문의 SHA-256을 먼저 기록했다.

같은 actual owned r8에서 실제 raw 응답을 `nativewire.Check`로 검증한 뒤 제품 tuple adjudicator(`adjudicateOrdinaryTuple`)에 직접 넣는 `TestOrdinaryTSQLExpectedAgainstNative`(opt-in)는 72/72 tuple을 VERIFIED로 판정한다. 첫 대조에서 dynamic SQL capture 8개 asset(step1–4 × incremental/fresh)의 match 구성이 달랐다. 원인은 작성 모델이 `ts_query_cursor__copy_state`(`query.c`)의 복사 방향을 반대로 가정한 것이었고, 복사본이 원본의 id·consumed 수를 물려받는다는 소스 근거로 교정했다. 이 교정은 봉인 대비 8개 asset만 바꾸며 나머지 99개는 같다. Root capture를 가진 wildcard child pattern의 definite 여부(`tsqlDefinite[12]`)는 생성된 parse table의 정적 분석 결과라 grammar.json만으로 손으로 증명하지 못한 모델 매개변수다. 이 값은 native 관측과 일치하도록 정했으므로 해당 pattern에 한해 독립 증거로 세지 않는다. 같은 test는 expected의 node 하나·capture 하나·fact 하나를 변조하면 변조를 소비하는 tuple만 FAILED(`ORDINARY_EXPECTED_TREE_MISMATCH` 8개 query tuple, `ORDINARY_CAPTURE_MISMATCH` 1개, `ORDINARY_FACT_MISMATCH` 1개)가 되고 나머지는 VERIFIED로 남음을 확인한다.

공개 `TestOrdinarySQLPublicPassCapacity`의 TSQL 경로는 expected 107개를 소비하며 reference row 40개가 expected와 같은 full CST·ordered captures를 담는다. 위 단일 preflight 이후 `reference-rows` 단계는 통과한다. 단계 진입 시 남은 logical 144,733,009 B에서 71,807,549 B와 work 2,736,458을 쓰고 남은 logical은 72,925,460 B, work는 4,638,749다. 다음 단계인 독립 tuple 판정에서 tuple 10개를 VERIFIED로 처리한 뒤 `ordinary-expected-tree`의 다음 logical 예약 6,400 B에 2,815 B가 부족해 `ELIGIBILITY_WORK_LIMIT`(실제 원인 logical)으로 멈춘다. 중단 시 logical 1,073,738,239 B, work 28,380,235, read 129,947,602 B이며 required 72/verified 10/failed 0/blocked 0/unprocessed 62, `CAPACITY_UNPROVEN`/BLOCKED다. 다음 지배 항목은 tuple마다 expected tree asset을 다시 decode하는 비용이다. `adjudicateOrdinaryTuple`의 `ordinary-expected-tree`와 `ordinaryReferenceExpected`의 `ordinary-reference-expected-tree`가 같은 asset을 tuple마다(72번) 각각 `256+512·field` 허용량으로 decode하며 tuple 10개에 35,347,051 B와 32,278,976 B(합 약 6.7 MB/tuple)를 썼다. 서로 다른 tree asset은 9개다. 한도는 바꾸지 않았고 이 단계는 다음 작업의 범위다. PostgreSQL은 같은 공개 pass에서 9/9 VERIFIED를 유지하며 logical 620,458,108 B, work 8,619,803이다. Reference executable/library는 실행·load하지 않았고 approval·reference row는 합성이므로 외부 reference와의 실행 독립성은 입증하지 않는다.

같은 PostgreSQL shard·실제 owned parser-paged r8·독립 DO expected·합성 approval/reference를 사용한 새 공개 호출은 `BaseVerified=PASS`, semantic/assessment `PASS`, required 9/verified 9/failed 0/blocked 0/unprocessed 0, complete/processing true다. 새 원장은 logical 698,039,669 B, work 14,108,124, read 134,443,451 B다. 이 수치는 선택 shard의 `FULL_BODY_MEASURED_VERIFIED`이며 전체 PostgreSQL route/P0 admission이나 reference 실행 증거가 아니다. 앞의 697,519,126 B 측정은 변경 전 역사적 결과다. 같은 후속 실행의 TSQL은 logical 1,073,723,418 B/work 30,896,698/read 126,923,074 B에서 `projection/steps/0/route`의 다음 logical 예약 32,896 B에 14,490 B가 부족해 중단됐다. Fact context를 base 전에 확인해 이전 측정과 중단 위치가 달라졌으며 full capacity는 여전히 미입증이다.

Reader는 전체 required ordinary 집합에서 선택 shard 분모를 먼저 고정한다. 선택하지 않은 ID는 이번 shard PASS로 계산하지 않고 전체 registry도 `INCOMPLETE`로 남긴다. Reference selection은 각 `(case, step, query)` 응답 row의 source pin, invocation, ordinal, offset, 길이 및 SHA-256을 고정한다. 같은 step의 incremental/fresh tuple은 그 reference row를 공유할 수 있지만, 각 native tree role과 independent expected는 따로 판정한다. 실제 raw reference 출력은 기대값이 아니다.

Structural-only 판정은 별도 pin된 query metadata, full CST, ordered captures와 선택적 fact expected를 actual checked tree/raw API/captures 및 source bytes와 비교한다. Metadata와 predicate 빈 목록, capture의 node/index/좌표·순서·중복, tree identity, API table의 의미 비교와 fact pack/mapping pin을 각각 검사한다. Internal fact 재도출이나 producer의 `step_facts`는 independent fact expected를 대신하지 않는다. CST field만 다른 경우는 FAIL 대신 `INELIGIBLE`/BLOCKED로 남기되, 별도로 확정한 metadata/API/capture/fact FAIL은 유지한다. 원본 base projection은 기존 base reducer 의미로 raw에서 다시 판정해 저장 projection 및 query expectation과 대조한다.

Tuple 상태는 `verified`, `failed`, `blocked`, `unprocessed`로 shard required 집합을 나눈다. Finding 1000개 제한·취소·budget 중단은 이미 확정된 FAIL과 최초 code를 지우지 않는다. `processing_complete`는 selected shard 처리가 끝났다는 뜻이고, `full_scope_complete`는 false다. `MEASURED_SELECTED_PASS`는 전체 route capacity 또는 full P0 완료의 증거가 될 수 없다. Focused public JSON call은 선택 tuple 1/1 `VERIFIED`를 확인했고, approval 부재·approval-selection 불일치·다른 cohort·member 변조·final identity 변경·base FAIL 보존의 여섯 public negative case도 실행했다. Approval과 reference response는 synthetic fixture이며 reference process를 실행하지 않았다. SQL ordinary의 exact current closure/body 상한은 별도 실측·입증 전까지 `CAPACITY_UNPROVEN`/BLOCKED다. 전체 P0와 P2-D 저장 결과/replay/CLI는 미완료다.

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

Reference는 non-exact head를 읽고 schema를 확인한 뒤 FAILED envelope의 exact head 또는 성공 response의 full shape를 검사한다. FAILED의 extra 필드·잘못된 failure code 및 뒤쪽 구문 오류의 우선순위는 기존과 같다. Head와 full은 이번에도 각각 decode하며 receipt r1의 stdout/stderr base64와 row 분할·offset·hash 결속도 유지한다. B5의 receipt r2는 구현하지 않는다. Ordinary reference row만 head와 full이 한 번의 preflight를 공유한다(아래 절).

각 decode의 기본 logical 예약은 snapshot과 같은 `5n + 32z + 32768 + 80v`이고 descriptor·struct container·고정 배열 비용은 별도다. 중간 AST를 만들지 않는 실제 경로에만 적용하며 후속 tree/capture/predicate 비교·guarded read·base64 비용은 별도 원장에 남는다. 기존 pointer·eligibility→witness handoff를 포함한 shape 값 수 한도도 별도 계수로 유지하되 제거된 AST 순회를 work로 계상하지 않는다. Reference의 source 좌표 검증은 분리된 span을 EOF까지 한 번 순회하므로 실제 검증 직전에 선택 source의 byte 길이를 `reference-points` work로 예약한다. Node 수만의 예약으로 긴 source 순회를 대신하지 않는다. 한도 변경이나 전체 ordinary 수용 근거가 아니다.

### #362 ordinary reference row의 단일 preflight와 원장 (Refs #362)

Ordinary reference row 처리(`ordinaryReferences`의 invocation별 receipt·row decode, `src/kit/query_ordinary_reference.go`)만 아래 식을 쓴다. 공용 reference connection(r4–r6/r10 포함)·eligibility native·control의 `decodeReferenceResponseBound`는 기존 head/full 두 decode 원장을 그대로 유지하며 시험(`TestReferenceRowSharedDecodeUnchanged`)이 이를 고정한다.

**기존 식과 실제 순회.** Row 하나는 head decode(non-exact), 성공 row의 full decode(exact)와 `reserveReference`를 거친다. 두 decode는 각각 preflight 전체 token 순회, shape 순회, `jsonv2.Unmarshal`을 수행하며 원장도 각각 `R + [R + 32·significant + 32,768 + 80·values + 3R] + shape 고정비 + struct 허용량`이다. R은 row byte, significant는 공백을 뺀 byte다. Full decode의 struct 허용량은 JSON member마다 `256 + 512·field`여서 `TreeNode`(15 field)는 7,936 B, `Point`는 1,280 B를 계상했다. TSQL 첫 row(`q.structure/0`, R=36,315, node 155, values 2,672)는 head 1,595,815 + full 3,070,887 + `reserveReference` 3,598 = 4,670,300 B였다. 같은 row의 실제 할당(`runtime.MemStats.TotalAlloc`)은 약 85 KB다. Receipt r1(base64 stdout 한 값)도 같은 `32·significant`를 받아 invocation마다 2,090,874 B였다. 두 번째 head decode는 첫 decode의 preflight를 다시 수행하는 중복이었고, `32·significant`는 오류 경로가 이름을 복사하는 비용을 모든 byte에 곱한 값이었다.

**새 식(ordinary 전용).**

1. Row마다 preflight를 한 번만 예약한다. `charge(1,R)`과 `reserveJSONContainers(passes=3, 80/value)`는 그대로이고 오류·buffer 예약만 `32·(maxPath + maxName) + 32,768`로 바꾼다. `maxPath`는 한 root→value 경로 위에 놓인 member 이름 byte 합의 최댓값, `maxName`은 가장 긴 이름이다(`jsonErrorPathBytes`). Decoder 오류 경로와 중복 이름 오류가 복사하는 것은 이 이름들이다. Bracket 불일치나 깊이 64 초과는 기존 `32·significant + 32,768`로 돌아간다.
2. Head decode와 full decode(실패 envelope의 exact head 포함)는 그 preflight를 공유한다. 첫 decode가 passes=3(preflight, shape 순회, Unmarshal)을 예약하고, 이후 decode는 shape 순회와 Unmarshal 몫으로 work `2·values`, logical `2R`을 더 예약한다. 오류 종류, 우선순위, 반환 코드는 바뀌지 않는다.
3. 이 경로의 typed 값 하나는 Go 크기로 계상한다. 방문한 값마다 logical `8·sizeof(T)`(struct는 member마다 `+16`), 문자열 payload는 입력 길이 R을 한 번 더한다. 8은 slice 증가가 최종 backing array의 약 5배까지 할당한다는 점을 덮고 16은 문자열·포인터 할당의 올림을 덮는다. 기존 `256 + 512·field` 허용량은 이 경로에서 쓰지 않는다.
4. `reserveReference`, `reference-points`, base64 문자열 `4·len`, member read·hash 항은 바뀌지 않는다.
5. Ordinary scope(`s.ordinary != nil`)의 process receipt r1 decode는 1번의 오류·buffer 예약만 바꾼다. 다른 scope의 receipt는 기존 식이다.

위 첫 row는 `R + scan(4R + 32·(26+22) + 32,768 + 80·2,672) + head 40,547 + full 331,339 + 두 번째 decode 2R + reserveReference 3,598 = 877,753 B`가 된다(기존 4,670,300 B의 18.8%). Receipt는 352,922 B다(기존 2,090,874 B).

**계상 ≥ 실제.** 제품 counter 없이 시험이 계측한다. `TestOrdinaryReferenceRowAccounting`은 행마다 `TotalAlloc`(GC 후 세 번 중 최대)과 독립 token 순회로 센 값 수 V에 대해 logical ≥ 실제 할당, work ≥ 5V(preflight 1 + head 2 + full 2 순회) + query source hash byte를 assert한다. 대상은 20,000 node CST, capture 12,000개, 50,000 B 노드 이름, 4,000 B capture 이름, 1 MiB query id, 100,000개의 빈 capture 이름, 60,000개 match mapping과 오류 입력(31단계 중첩의 30,000 B 이름, 1 MiB 문자열의 잘못된 escape, 512 KiB 중복 이름, 100,000개의 `{}` 배열, 20,000개의 넓은 이름, 2 MiB 문자열 schema)이다. 정상 형태 행의 여유는 2.6–15배(가장 낮은 것은 빈 capture 이름 100,000개), 오류 입력은 최소 2.5배(중첩 이름)다. `TestOrdinaryReceiptDecodeAccounting`은 3 MiB stdout receipt와 같은 오류 입력을 검사한다. Opt-in `TestOrdinaryReferenceRowsAccountingSQL`은 선택 TSQL 40행(1,731,429 B)과 PostgreSQL 9행에서 logical 41,291,129 B ≥ 실제 할당 3,623,752 B(TSQL), 1,616,252 B ≥ 352,296 B(PostgreSQL)를 확인한다. 이는 Go runtime 내부 할당에 대한 형식적 증명이 아니라 위 입력군에 대한 측정이며, 32라는 이름 복사 계수는 측정한 최대 14.35배의 약 2.2배다.

**효과.** 합성 reference row 40개의 decode+`reserveReference` 비용은 logical 222,514,364 B에서 41,291,129 B로, work 2,923,400에서 2,680,044로 줄었다. `reference-rows` 단계 전체(40 invocation의 receipt·row 포함)는 logical 71,807,549 B, work 2,736,458이다. Work는 거의 줄지 않는다. 한도, row 내용, expected, 사례·의무는 바꾸지 않았다. `OrdinaryReferenceEstimate`(plan의 density 추정)는 이 변경으로 바꾸지 않았고 현재 원장의 측정값이 아니다.

### Evidence graph의 순회 원장 (Refs #362)

Run 중복은 `(run, attempt, platform)`의 세 문자열을 key로 한 index에서만 찾는다. 정렬된 node 순서와 bucket의 뒤쪽 순서를 사용해 기존 all-pairs 검사의 finding code·순서·첫 실패를 보존한다. 같은 identity의 실제 중복 쌍은 모두 방문한다. Cycle은 정렬된 root와 원래 ref 순서의 DFS 한 번으로 최초 back-edge를 정하고, 역방향 edge의 도달성 memo로 cycle에 도달하는 owner를 구한다. 요청에만 있고 graph에는 없는 node는 cycle owner가 아니다. Carry 규칙은 node별로 policy 순서를 유지해 모으며, 각 carries ref는 그 node의 규칙만 검사한다. 파일 owner·approval·동일 guarded snapshot 결속은 유지한다. Paging의 별도 group 안전성 scan은 기존 `streamed-chain-plan`에서 계속 계상한다.

`N`은 입력 node 수, `E`는 모든 ref·file·identity entry 수의 합, `R=Required+CarryForward` 행 수다. `B(m)=bits.Len(m)`이며 `B(0)=0`, `S=Σ identity_count·B(identity_count)`다. `P`는 required identity마다 `m·(1+B(m))`의 합에 eligibility node 수와 `(1+허용 mode 수)`의 곱을 더한다. `D`는 replay/carry를 제외한 입력 node의 동일 run tuple 쌍 수, `J`는 carries ref마다 그 node에 해당하는 carry rule 수의 합이다. `Q`는 subject ref마다 해당 node의 file 수를 더한 값으로, replay-result 파일 목록의 반복 scan을 포함한다. 중복/잘못된 node도 예약에서는 포함해 실제 수용 graph보다 작아지지 않게 한다. `H`는 replay/carry 밖 node의 run·attempt·platform 문자열 byte 길이 합이다.

Graph work 상한은 `N + CarryForward + 16H + 64·(1+N+E+R+S+N·B(N)+P+D+J+Q)`다. 앞의 `N+CarryForward`는 estimator의 행 방문이고 `16H`는 estimator와 검증 index의 문자열 hashing 예약이며 무거운 처리 전에 차감한다. 64배 항은 estimator·node/role/file/ref 검사·집합 구축·관계 검사·DFS·역방향 closure·owner 판정·finding 생성·replay 파일 목록 scan의 정수 개수 순회와 정렬 비교를 포함한다. 실제 중복 finding `D`와 같은 node의 규칙 결과 `J`는 이차가 될 수 있으므로 제거하지 않는다. 이 상한은 collection 방문·비교 및 명시적 byte 처리의 logical work 원장이며 CPU instruction 수·Go map의 충돌 최악 시간 보장은 아니다. Replay-result decode·파일 hash/read·streamed plan 등 별도 원장은 중복 없이 유지한다.

기존 graph 문자열 logical 예약 `(fields+1)·(N+R+1)`을 유지하고 새 map·slice·역방향 edge를 위해 `512·(1+N+E+R)` B를 더한다. Estimator의 map 공간은 만들기 전에 일부를 예약하며 나머지는 graph 검증 전에 예약한다. Work32,000,000과 기존 context별 logical 한도는 바꾸지 않는다. 계측은 test-only Go overlay에서 실제 loop body 진입·DFS node 진입·정렬 comparator 호출을 세며, 동일 SQL core·기존 fixture·합성 사슬/fan-out/cycle 직전과 cycle에서 예약이 계측값 이상인지 확인한다. 이는 새로운 runtime 관측이나 ordinary 전체 capacity의 증거가 아니다.

### #362 ordinary inventory 선택 decode

내부 `parseQualificationInventoryRoute`는 integrated ordinary pass에서 선택 route 입력을 처리한다. Caller가 제공한 `qualification-c1.json` bytes는 같은 guarded session의 승인 SHA-256과 먼저 대조되고, 함수도 전체 bytes의 SHA-256을 재대조한다. 공개 `ParseQualificationInventory`와 기존 qualification·semantic·replay 호출자는 전체 decode와 기존 오류 분류를 유지한다. Ordinary 입력 처리는 새 공개 parser나 한도 선택 옵션을 추가하지 않는다.

전체 문서는 AST를 만들지 않는 strict token 검사로 UTF-8·surrogate·escape-equivalent 중복 key·문법·trailing value·depth32·2,000,000 values를 확인한다. Token 순회는 header와 전역 검사용 작은 투영 및 원문 route 구간만 수집한다. Header의 cell 수는 전체 route×platform 곱과 같아야 하며 platform ID, route ID, 전체 ordinary/extra workload set, extra-role ID와 route ID 충돌, workload의 route 연결을 검사한다. Extra-role의 platform 참조·실행/미적용/유예 분할 및 `reuse-accepted`의 OS/arch별 실행·유예 조건도 유지한다. Query/case ID·coverage·source/edit·requirement 및 extra workload 내용의 의미 검사는 workload 안의 계약이다. 선택한 route의 내용만 기존 의미 검증기를 적용하고, 다른 route의 내용 검증은 그 route를 처리하는 pass의 의무로 남긴다. 선택 결과는 전체 inventory로 오인되지 않는 별도 내부 type이며 header에서 routes/extra_roles를 제거하고 원래 route index와 선택 route를 돌려준다.

엄격 token 검사의 예약은 `S(n,z,v)=3n+2z+32768+16v`, AST 없는 typed decode의 예약은 `T32(n,z,v)=5n+32z+32768+80v`다. `n`은 bytes, `z`는 문자열 내부 공백을 포함한 유효 JSON bytes, `v`는 member name과 닫는 괄호를 제외한 값 수다. T32는 작은 전역 투영과 선택 route에만 적용한다. Registry는 전체를 typed decode하며 P0의 presence/null·exact membership·pin 검사를 생략하는 근거로 사용하지 않는다. Ordinary coordinator는 typed registry와 선택 route 해석을 공통 ledger에서 실행하고 inventory pin을 재검사한다. 이 문서의 계약은 별도의 public-call 원장 관측이나 current SQL capacity 실측을 대신하지 않는다.

Guarded read의 기존 비용 외에 전체 hash 재대조 `n`, route index당64 B, 투영 buffer의 실제 복사·성장 예약, 전역/선택 의미 검증과 전역 특수-arm shape 검사를 추가 계상한다. 전역 의미 검증은 투영 bytes당 work8/logical4 및 기존 platform 곱을 먼저 예약하고 정확히 한 번 실행한다. 그 성공 후 선택 route는 전역 header·platform·route/set·extra-role 검사를 반복하지 않고 같은 validator의 route-local 부분만 실행한다. 공개 전체 parser의 검사와 순서는 유지한다. 선택 의미 검증의 logical 예약 `8·len(raw)+4·len(projection)`도 유지한다.

선택 work는 `N + L + P + F`다. `N`은 `inventoryRouteValidationWork`가 실제 수행하는 각 counting loop 반복을 그 자리에서1씩 charge한 값이며, `L`과 기존 곱 `P`는 route-local validator를 실행하기 전에 예약한다. `F`는 `SvcContext != nil`일 때만 유지하는 기존 `8·len(raw)` 상한이다. SVC의 marshal/strict decode/재귀 context 검사는 구조 상한을 따로 입증하지 않았으므로 byte 예약을 없애지 않는다. ordinary route의 preprocess/accepted 설정은 `role=false`에서 helper 실행 전에 기존처럼 거부한다.

`L`은 아래 항의 합이다. `|s|`는 decoded 문자열의 byte 길이이고 배열 기호의 길이는 원소 수다. map/hash·문자열 검사의 항은 해당 key/입력 전체 길이를 상한으로 사용하며 CPU instruction 수나 regexp 엔진의 내부 상태 전이 수를 뜻하지 않는다.

| 실제 검증 대상 | `L` 예약 항과 대응하는 순회 |
|---|---|
| route/workload | 고정2: 각각의 validator 진입 |
| error node `s` | `1+|s|`: 행 방문과 node regexp; 중복 검색은 `P` |
| requirement `r` | `1+4·|r.Row|+|route.Route|+7·len(r.Kinds)`: 행, row regexp·prefix·map 조회/삽입, kind마다 행 방문+최대6개 kind 비교 |
| requirement alternative `a` | `1+4·|a|+|r.Row|`: 행, prefix·suffix regexp·대안 map 조회/삽입 |
| grammar `g` / query `q` | 각각 `1+|g.SHA256|`, `1+3·|q.ID|+|q.SHA256|`: 행, hash regexp 및 ID 검사·map 조회/삽입 |
| case `c` | `1+3·|c.ID|+|c.Input.SHA256|+len(c.Edits)`: 행, ID·hash 검사/조회/삽입과 `stepSizes` edit 반복 |
| case source | `2·checks·|source|`: UTF-8와 SHA-256 순회. `checks`는 `Format==SvcFormat`과 `len(QueryExpect)>0`의 참인 조건 수이며 두 조건이 함께 참이면 두 검사를 모두 계상 |
| expectation / anchor | expectation당1, anchor당 `1+|Type|`: callback 방문과 `validAnchorType` byte 순회 |
| query expectation `q` | `2+|q.Query|`: step와 query membership 두 반복 및 key 검사 |
| sample `s` | `8·(1+|Repository|+|Commit|+|Path|+|License|+|SHA256|+|c.Input.SHA256|)`: repository/commit 검사, path 검사·split·segment 방문, 고정 license 집합·pin 비교의 byte 상한 유지 |
| coverage `(row, claims)` | `1+|row|+len(claims)`: 행·row map 조회·claim 반복. 정렬 및 반복 검색은 `P` |
| case alternative `a` | `1+3·|a|+maxRowBytes`: 행, alternative map 조회/중복 검사/삽입 및 coverage row 조회. claim membership는 `P` |

문서 전체의 `reuse_accepted`/`preprocess_cst_expect` arm은 schema gate와 기존 별도 shape 검사를 typed decode 전에 preorder로 적용한다. 이 구간에 한정된 subtree AST·shape 비용과 구간 index를 추가 예약하며, 다른 비선택 subtree의 AST는 만들지 않는다. 실제로 읽거나 보관하지 않은 다른 route의 typed container 비용은 예약하지 않는다. 한 pass의 원장을 새로 만들거나 reset하지 않으며 file/read/work/logical/wall 한도를 바꾸지 않는다. 이 부분 구현과 측정은 ordinary 전체 의미 검증·새 runtime 관측·#362 완료를 뜻하지 않는다.

플랫폼 분할·error-node 중복·coverage의 kinds/expectations/contains 조합·alternatives×coverage 검색과 coverage key 정렬은 byte 선형 예약만으로 상한을 잡지 않는다. 각 입력 길이의 보수적 곱을 overflow-safe하게 추가 work 예약한 뒤 기존 의미 검증기를 호출한다. 반복 coverage와 긴 expectation 목록을 조합해 work를 소진하는 입력은 실제 중첩 순회 전에 `RESOURCE_LIMIT`으로 중단한다.

## Parser-only paging 문맥

`full-input-utf8-paged-parser/r1`은 단일 작은 UTF-8 native-query source의 큰 grammar parser에 한정된다. 세 approval/registration/result r1과 witness r2의 필드 의미는 유지하며 Windows/Linux만 받는다. `Native.Grammar[original_path]`는 실제 `grammar-paged-descriptor` 물리 pin을 가진다. 실제 `Native.InvocationID`의 NATIVE_DRIVER node가 descriptor와 모든 part의 owner여야 하며, 조각은 동일 guard의 검증된 cached bytes에서만 소비한다. 참조하지 않은 paged member와 다른 owner는 거부한다. 전체 logical pin은 승인된 원본 plain build manifest에 대조하고 선언된 descriptor 해시만으로 승인하지 않는다. 정확한 분할 규칙과 변경되지 않는 small 예산은 [계측 계약](instrumented-query-reference.md#216-parser-only-paging)이 소유한다. 이 문맥에서 eligibility 검증이 직접 시작한 원장(approval 결속)의 논리 처리량은 1.5 GiB이며 다른 small 한도는 같다. `VerifyQuerySemantics`·Replay처럼 원장을 상속하는 경로는 이 문맥에서도 1 GiB다. 큰 source/CST 또는 새 macOS 검증을 추가하지 않는다.

`full-input-utf8-paged-binaries/r1`은 paged-parser 규칙을 포함하되 16 MiB를 넘는 grammar parser를 0개 또는 1개 허용한다. driver executable과 reference library 중 하나 이상이 descriptor로 보존돼야 하며, 이 조건은 approval에 결속된 registration의 executable·library pin role로만 판정한다. NATIVE_DRIVER의 executable이 `driver-paged-descriptor`이면 descriptor와 모든 part는 그 invocation node가 소유한다. 재결합한 logical SHA-256을 approval의 `executable_sha256`과 대조하고, 사용하지 않거나 다른 node가 소유한 `driver-paged-*` member는 거부한다. 다른 context에서 descriptor role의 executable은 승인 값과 무관하게 거부한다. reference library의 descriptor는 그 library node가 소유하며 사용하지 않는 `library-paged-*` member는 거부한다.
