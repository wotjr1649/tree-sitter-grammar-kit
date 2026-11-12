# Query reference offline eligibility 계약 (#216)

이 experimental API는 caller가 evidence 밖에서 신뢰한 승인 문서에 조건부인 저장 근거의 일관성을 검사한다. 실제 실행 진위·서명·게시자 인증이나 전체 route의 qualification 완료를 증명하지 않는다. 검증 중 process·network·임시 파일·원래 호스트 경로 조회를 수행하지 않는다. 기존 [reference connection](instrumented-query-reference.md)의 r1 및 native/r2 의미는 유지한다.

## #370 V2-A 역사 자료 pin 입력

`ResolveEvidencePins(context.Context, EvidencePinsRequest)`와 `SelectEvidenceProductionPin(ctx context.Context, selection, pinSet []byte) (string, error)`는 별도 opt-in의 offline API다. V2-A는 **원문 자료의 승인 결속**만 구현한다. opt-in 없는 eligibility 호출의 catalogue·Rust release·runtime 검사와 기존 reducer 의미를 유지한다. 직접 eligibility의 Rust/catalogue 소비는 아래 V2-B1 opt-in으로 구현한다. Python/workflow·classification 생산 이행과 runtime 전체 전달·semantic/replay/CLI 통합은 남아 있다. 이 절의 PASS를 build9/unit4/control53/assertion36 또는 semantic·accepted·preprocess-CST PASS로 사용하지 않는다.

`EvidencePinsRequest`의 `Approval`, `PinSet`, `Subject`는 원문 bytes이고 `Materials`는 정확한 논리 이름→원문 bytes map이다. Caller는 graph 밖에서 **독립 승인한** `ApprovedApprovalSHA256`와 `ApprovedPinSetSHA256`를 제공한다. 증거나 sidecar의 자기 hash를 그대로 이 두 field에 복사하는 것은 독립 승인이 아니다. API는 승인 작성자의 진위를 인증하지 않는다. Caller는 호출 중 slice/map을 변경하지 않는다. 함수는 입력을 수정하거나 file/process/network에 접근하지 않고 결과는 원문 slice/map을 소유하지 않는다.

모든 문서는 bounded strict JSON이며 모든 field는 필수다. Duplicate/unknown/null/type 오류, trailing JSON·invalid UTF-8와 기존 depth32 제한을 적용한다.

| 문서 | field와 의미 |
|---|---|
| `tsgk-evidence-pin-selection/r1` | `schema`, `pin_set_sha256` 하나. 생산 전에 한 번 선택해 고정할 후보이며 승인이 아니다. 선택 함수는 caller가 함께 제공한 집합 원문의 digest·strict shape 및 resolver와 공유한 descriptor 검증(schema/reader·행 수·이름·역할·bytes/hash·중복)을 수행한다. 자료 본문 결속과 독립 승인은 resolver가 별도로 검사한다. 역사 resolver는 pointer를 받거나 조회하지 않는다. |
| `tsgk-evidence-pin-set/r1` | `schema`, `reader`, 비어 있지 않은 `materials` 배열. 행은 `name`, `role`, `bytes`, `sha256`이다. |
| `tsgk-evidence-pin-approval/r1` | `schema`, `pin_set_sha256`, `subject_sha256`, `reader`, `scope`. 기존 approval/registration/profile 등 caller가 선택한 **하나의 원본 subject 전체 bytes**에 결속한다. 다른 graph/profile/candidate를 대표하는 원문으로 sidecar를 재사용하면 FAIL이다. 복수 문서 closure의 의미 검증은 해당 소비 API가 담당한다. |

지원 reader는 `tsgk-evidence-materials/r1`, scope는 `historical-materials`뿐이다. 이 reader는 자료를 opaque bytes로 결속하며 catalogue 규칙·Rust version stdout·ABI·Roslyn diagnostic을 해석하지 않는다. 역할은 `reference-catalogue`, `reference-input`, `reference-tuple`, `runtime-manifest`, `runtime-patch`, `driver`, `cp949`, `shim-template`, `roslyn-observation`, `roslyn-cases`다. 알 수 없는 schema/reader/scope/role은 UNSUPPORTED이며 PASS가 아니다. 더 강한 scope는 승인 문서가 요구해도 지원하지 않는다. 후속 semantic reader는 고정 의무·분모·known-rule를 자체 소유해야 하며 이 배열로 줄일 수 없다.

이름은 host 경로로 열지 않는다. 기존 portable 상대 경로 검사, 1,024 bytes 상한과 대소문자 접기 중복 거부를 적용하고 입력 map은 정확한 이름으로 결속한다. 미사용·다른 대소문자 이름은 잘못된 입력이다. 동일 내용의 여러 역할은 각 정확한 이름을 제공한다. 역할·bytes·hash·순서는 승인된 집합 원문 digest에 함께 결속된다.

승인 원문·집합 digest를 외부 두 승인값과 먼저 대조한 뒤 sidecar의 집합/subject digest를 실제 원문과 대조한다. 자료와 sidecar를 모두 재해시해도 외부 승인값이 다르면 `PIN_APPROVAL_MISMATCH` FAIL이다. 다른 subject는 `PIN_SUBJECT_MISMATCH`, 제공된 자료 길이/hash 모순은 `PIN_MATERIAL_MISMATCH` FAIL이다. 외부 승인이나 승인 문서/집합이 없으면 `PIN_APPROVAL_MISSING` BLOCKED이며 완료하지 않는다. 선택한 자료가 없으면 `PIN_MATERIAL_MISSING` BLOCKED로 남기고 다른 행을 계속 검사한다. 확정 FAIL은 이후 누락·취소·자원 오류로 지우지 않는다. 구조 오류는 typed error다. 선택 함수의 원문 누락/불일치는 `PIN_SELECTION_MISSING`/`PIN_SELECTION_MISMATCH` invalid input이다.

`tsgk-evidence-pin-result/r1`은 실제 소비한 approval/set/subject raw SHA-256, reader/scope, 검증한 자료 descriptor 순서, `required`/`verified`, read/work/logical counter와 `complete`를 기록한다. `complete=true`는 배열 평가 종료이며 누락으로 BLOCKED일 수 있다. 조기 승인 거부나 오류는 false다. Subject의 기록 verdict나 NEW_RUN을 재작성하지 않는다. 생산 선택을 B/C로 바꿔도 같은 A 원문·외부 승인·reader의 판정과 소비 identity는 동일하다. 새 계획의 pin 요구는 별도 소비 계약에서 검사한다.

Caller deadline은 필수다. 기존 evidence-replay wall120초·문서/자료별16MiB·총읽기256MiB·자료10,000개, JSON decode/shape 각2,000,000 values, work32,000,000·logical1GiB를 재사용한다. 자료·문서·hash·index·shape·출력 descriptor 비용을 같은 원장에서 처리한다. 입력은 이미 caller memory에 있으므로 이 API의 원장은 caller의 사전 파일 읽기를 대신 제한하지 않는다. V2-A는 CLI file admission이나 nested run에 연결되지 않는다. 후속 통합은 상속 원장과 기존 file identity/hardlink/reparse/alias 검사를 유지해야 한다.

`src/contracts/evidence-pins/selection.json`은 단일 후보 선택이다. `sets/<raw-sha256>.json`과 `objects/<raw-sha256>`는 내용 주소의 보존 자료이며 승인 기관이 아니다. 기존 객체를 갱신하지 않고 새 객체/집합을 추가해 선택만 옮긴다. `.gitattributes`의 `-text`는 원본 개행을 보존한다. 최초 집합은 catalogue와 reference controller 입력, 현재 manifest/patch/driver/CP949·실제 shim 생성식 template, 기존 Roslyn/r2 원문·cases 및 기존 logger가 허용한 과거 manifest를 보관한다. 과거 manifest는 repository commit `10c65c8`의 원문이며 기존 digest `affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080`과 대조했다. Shim template와 reference tuple은 이행 metadata이며 과거 실행 관측으로 소급하지 않는다. Runtime 원본 checkout 전체·과거 driver closure·Roslyn DLL/SDK provenance는 복원하지 않았다. 자료 보관만으로 full historical eligibility를 주장하지 않는다.

현재 생산 Python/workflow는 아직 이 pointer를 소비하지 않는다. 기존 verifier의 opt-in 없는 호출도 이 pointer를 소비하지 않는다. 기존의 현재 catalogue 전용, 현재 runtime 완결 검증 및 과거 runtime BLOCKED 규칙은 해당 legacy API에 계속 적용된다. 후속 opt-in은 caller가 정확한 digest와 reader를 승인한 historical catalogue/closure를 사용하고 과거 결과를 새 reference의 관측으로 승격하지 않아야 한다.

## #370 V2-B1 승인 historical reference 검증

`QueryReferenceEligibilityRequest.Pins *EvidencePinsRequest`는 직접 `VerifyQueryReferenceEligibility` 호출의 명시적 opt-in이다. `nil`이면 기존 r1 결과와 내장 catalogue·Rust release 검사를 유지한다. `Pins`가 있으면 결과 schema는 `QueryReferenceEligibilityPinsResultSchema`(`tsgk-query-reference-eligibility-result/r2`)이며, 승인 누락·취소·오류에도 이 선택을 유지한다. 원래 approval/registration/witness/graph bytes와 schema는 바꾸지 않는다.

V2-A resolver와 같은 구현·상속 원장에서 sidecar·집합·자료를 검증한다. `Pins.Subject`는 **이 요청의 `Approval` 원문 전체와 같은 bytes**여야 한다. 이 approval이 registration·witness·policy·candidate를, registration이 graph와 각 owned member를 기존 방식으로 결속한다. Materials scope의 PASS만으로 eligibility를 승인하지 않고, 기존 caller-trusted eligibility 승인과 아래 reader 검사를 모두 요구한다. 증거가 자기 자료를 승인하는 경로는 없다.

집합에는 정확한 이름과 역할의 `reference/catalogue.json`(`reference-catalogue`), `reference/tuple.json`(`reference-tuple`)이 필요하다. Tuple의 모든 field는 필수다: `schema=tsgk-reference-pin-tuple/r1`, `rustc_release`, `cargo_release`, `path_reader`, `catalogue_sha256`. 두 release는 각각 `숫자.숫자.숫자` 형식이며 build identity의 유일한 `release:` field와 정확히 같아야 한다. 중복 release·접미사·한 도구만 다른 혼합 tuple은 FAIL이다. tuple은 독립 승인 집합에 결속되며 release 문자열만으로 도구를 승인하지 않는다. 지원 path reader는 `rust-path-1.97.1/r1`이다. 이 ID는 역사 경로 의미를 고정하며 새 tuple의 의미 호환은 caller 검토 대상이다. 미지원 reader/schema는 typed UNSUPPORTED다. Target은 기존 platform별 target 및 build paths/identity의 정확한 대조로 고정한다.

Catalogue 원문 digest는 tuple, 기존 approval, registration과 모두 같아야 한다. `src/kit/query_eligibility_obligations_r1.json`은 생산 catalogue와 별개로 보존하는 이 reader의 고정 의무다. Build9/unit4/control53/assertion36의 수뿐 아니라 ID·순서·known-rule·argv·한도·필수 invocation 연결·grammar/control 입력·tree/capture/response 전체를 대조한다. Caller가 축소한 catalogue를 독립 승인해도 `PIN_CATALOGUE_OBLIGATIONS` FAIL이다. 이 파일의 의미를 새 생산 pin에 맞춰 갱신하지 않는다. 다른 의무를 지원하려면 별도 reader 검토가 필요하다.

변경 가능한 catalogue provenance는 upstream/archive/binding identity와 정확한 controller/reference input digest map이다. `reference/` 아래 `Cargo.toml`, `main.rs`, `instrumentation.patch`, `build.py`, `checks.py`, `check.py`의 `reference-input` 원문을 해당 digest에 결속한다. Build identity와 control/current reference 응답의 upstream은 같은 승인 catalogue commit에 결속한다. 기존 reference connection wrapper는 자체 legacy commit을 유지한다. Build identity의 input/tool hash·path·cohort, version stdout과 receipt, source/vendor/lock/metadata/binary 검사와 기존 검증 한도는 모두 유지한다. 원문 자료 누락은 `PIN_REFERENCE_MISSING` BLOCKED, catalogue/입력 결속 모순은 `PIN_REFERENCE_CATALOGUE`/`PIN_REFERENCE_INPUT` FAIL, 다른 subject 사용은 `PIN_ELIGIBILITY_SUBJECT` FAIL이다. V2-A의 strict 형식·alias·미사용·재해시 거부와 확정 FAIL 보존 규칙도 상속한다. 구조적으로 필수 역할이 잘못됐으면 `PIN_REFERENCE_ROLE` invalid input이다.

r2 결과의 `pins`는 실제 resolver 결과이며 추가 `reference_tuple_sha256`, `reference_path_reader`와 기존 `catalogue_sha256`은 소비한 역사 identity다. Outer `bytes_read/work/logical_bytes`에는 pin resolve와 의미 검증 비용이 모두 포함되고, nested `pins` counter는 resolver 구간의 차분이다. 바깥 한도를 새로 시작하거나 늘리지 않는다. 이 API는 현재 생산 selection이나 설치 도구를 조회하지 않는다. 같은 역사 입력·승인·reader를 고정하면 생산 선택 A/B/C 변경 뒤 결과·counter·소비 identity가 같다.

이 단위는 **직접 eligibility reference 검증**만 구현한다. `QuerySemanticsRequest`, replay/qualification/CLI의 context 전달, classification donor·assembly, Python build/test/check/acquire와 workflow의 단일 생산 입력은 남아 있다. Runtime 완결 검증은 기존 current-only 경계를 유지하며, 역사 reference PASS를 새 reference 또는 historical runtime의 실제 qualification으로 승격하지 않는다.

## 호출과 판정

```go
func VerifyQueryReferenceEligibility(context.Context, QueryReferenceEligibilityRequest) (QueryReferenceEligibilityResult, error)
type QueryReferenceEligibilityRequest struct {
    Evidence EvidenceRequest
    Approval, Registration, WitnessRegistration []byte
    Pins *EvidencePinsRequest // optional historical reference opt-in
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

`Pins=nil`인 기존 verifier 호출은 현재 내장 catalogue와 정확히 결속된 승인·build 입력만 검증한다. Reference 소스 수정으로 catalogue pin이 바뀌면 이전 catalogue의 승인과 build는 새 verifier의 현재 자격을 충족하지 않는다. 새 reference에는 새 identity의 build·controls·graph·외부 승인이 필요하다. 과거 원본과 그 catalogue에 맞는 source/reader revision은 그대로 보존하고, 과거 조건부 결과를 새 reader나 새 reference의 관측으로 바꾸어 기록하지 않는다. 이 legacy 경계는 과거 catalogue를 자동 허용하지 않는다. 위 V2-B1 opt-in은 graph 밖에서 독립 승인한 정확한 historical catalogue·tuple과 reader 의무를 사용하며, 생산 pin 갱신만으로 같은 과거 증거를 거부하지 않는다.

Witness r2는 기존 QueryWitnessRegistration의 field와 `native`를 가진다. Native는 `protocol`, `request_id`, `node_limit`, `api`, `points`, `runtime_manifest_sha256`, `producer_sha256`, `executable_sha256`, `producer_abi`, `observation_run_id`, `history_start`, `process`다. Producer ABI는 `language_version`, `runtime_language_version`, `runtime_min_compatible`, `symbol_count`, `field_count`다. Process field는 저장 request와 같은 Path/Args/Dir/Env/StdoutBytes/StderrBytes/Wall/Grace/Memory(Bytes,Hard)/CgroupParent다. 시간 단위는 Go duration의 nanoseconds다.

독립 승인한 native/r4 기대값과 실제 source에 ApplyEdits를 적용한 모든 version bytes/hash/points로 nativewire Expect를 구성한다. CheckHeader/Step/Open/End, reuseobs.Decode/CaseClaim/Judge, querywire/querypolicy 및 comparator를 재사용한다. Mandatory logger prefix PASS만으로 full logger COMPLETE를 대신하지 못한다. Build manifest는 승인 ProducerSHA256에 결속하고 full runtime/driver closure와 grammar physical pins를 대조한다. Grammar 역할은 parser/scanner/header이며 parser는 정확히1개다. Canonical quoted argv와 version/runtime/parser/scanner*/driver/shim/link의 필수 순서·개수 및 version argv를 검사한다. 현재 고정 runtime manifest의 ABI 호환 범위13..15도 승인/raw 값과 대조한다. Grammar의 language/symbol/field count는 caller가 독립 승인한 기대값과 raw producer를 대조하며 C source를 새로 해석하거나 컴파일해 이 숫자를 추출하지 않는다. 현재 내장 runtime manifest만 완결 검증 대상이며 과거 manifest는 BLOCKED다. 실제 실행 여부는 승인 provenance의 신뢰 범위다.

Observation 필수 field는 `query`, `step`, `tree_role`, `source_bytes`, `source_sha256`, `invocation_id`, `query_id`, `response_ordinal`, `byte_offset`, `byte_length`, `sha256`다. Witness의 required 밖 observation·중복·불명은 거부한다. Source version과 raw stdout의 정확한 response slice에 결속하며 현재 reference의 독립 full CST·metadata·capture를 대조한다. 유효하지만 다른 CST는 BLOCKED다. 형식이 유효한 current reference FAILED 응답은 producer의 exit1과 일치해야 하며, 성공 exit0 등의 모순은 FAIL이다. CST 동등성 전에는 capture 개수와 match/pattern/capture/name 순서를 비교하며 node index 등 CST 종속 전체 비교는 동등성 확보 뒤에 수행한다. 별도로 승인된 metadata·anchor의 독립 모순은 계속 FAIL이다. Native full CST가 없으면 제공된 reference anchor 자체의 전체 모순 검사도 유지한다. Required 밖도 포함한 모든 제공 full native tree의 EOF·실제 source points·내부 CST 일관성을 tuple 집계 전에 검증하며, 확인된 source 모순은 FAIL이고 검사 중단은 오류로 전파한다. Current 입력 slot이 없을 때 nil·빈 digest로 모순을 만들지 않으며 따로 승인된 observation/source version·request의 library hash에 대한 검사는 유지한다. Build·Control·Native 그룹이나 적용 가능한 observation이 없으면 독립 기대값이 없는 비교는 생략하고 해당 의무를 BLOCKED로 남긴다. Batch의 한 query observation 누락은 다른 query의 검증을 막지 않으며, 제공된 request·pin·response의 독립 모순은 계속 FAIL이다. Reference 및 native의 중첩 query·API·logger JSON 형식 오류는 이 API에서 KindInvalidInput으로 반환하고 Native raw의 response·r4 producer·step·tree와 compact 배열은 필수 member 및 primitive type도 검사한다. Nullable wire arm은 step의 edit/route/fresh, tree의 form/types/fields/nodes/errors/declarations/partial_trees, declaration의 name이며 중첩 query/API/logger arm은 별도 strict 검사를 통과해야 한다. Native 그룹이 생략되거나 source가 없어 version을 구성하지 못해도 등록된 모든 NATIVE_DRIVER invocation이 제공한 raw의 형식 검사는 수행한다. 모든 native invocation에서 자체 approved/stored request의 strict 일치·process 한도/env·실행 파일 owner/pin/hash/path·execution/stream, raw envelope/ABI/capability와 logger의 자체 승인 binding·역할·count/status 일관성을 확인한다. 확보한 자체 source pin으로 초기 source transport·UTF-8·full tree EOF/points/CST를 확인하며, query input은 확보한 각 pin의 길이1..65,536 bytes와 ID별 bytes를 대조하되 map에서 요청 순서를 추론하지 않는다. 모든 완료 full tree의 root EndByte는 초기/후속 actual source 존재나 선택 group과 무관하게 그 step 자체의 SourceBytes와 같아야 한다. Query 원문이 없어도 status·metadata·capture accounting과 확보한 full node 연결을 검증하고, compile-error class/code/shape와 query_ms=0·matches=0·빈 types도 유지하되 실제 원문의 offset/point 대조만 BLOCKED로 남긴다. 0개 query의 명시적 null/빈 배열은 허용하며 확보한 query pin이 응답에서 빠지면 FAIL이다. 제공 step의 순서·형태·내부 status/completion 모순도 검사한다. 초기 outer/step0 길이를 직접 대조하고 edit의 byte 순서·전후 길이 관계·빈 edit·내재 point 관계 및 확보한 초기 source의 첫 edit old point를 검증한다. 실제 후속 source·정확한 edit history의 증명과 구분한다. EOF/CST/point의 확정 모순은 공통 경로에서 먼저 기록하고 중단 오류를 반환하여 연결된 실패 counter를 보존한다. 후속 source를 raw 자체로 증명하지 않으며 전체 edit history·query 순서·build closure·required tuple 연결만 선택된 witness/group에 의존한다. 정상 extra에 선택 history가 없다는 이유로 전역 BLOCKED나 연결되지 않은 tuple 실패를 만들지 않는다. 그룹·source 기대값에 의존하는 비교는 해당 의무의 BLOCKED로 남긴다. 기존 r1 connection과 native r2의 판정 의미는 유지한다. Native version이 없더라도 보관된 current source의 UTF-8·EOF·points와 predicate를 검사하며, 확보된 빈 query bytes도 hash 모순 검사에 사용한다.

## 유한 예산과 검증 범위

공유 한도는 physical member/JSON 각각16 MiB, 총 읽기256 MiB, file10,000개, wall120초, output16 MiB, identity1 MiB다. JSON depth32/decode·shape 각각2,000,000 values, 공유 work32,000,000, 논리 처리량1 GiB를 적용한다. 단, eligibility 검증이 원장을 직접 시작하고 approval에 결속된 registration의 context가 `full-input-utf8-paged-parser/r1` 또는 `full-input-utf8-paged-binaries/r1`일 때만 논리 처리량은 1.5 GiB(1,610,612,736)다. 이 값은 registration 검증 직후·evidence 읽기 전에 한 번 정하며 evidence·descriptor·build manifest·파일 크기 같은 producer 출력은 선택에 쓰지 않는다. 결속이 맞지 않는 요청과 semantic·replay·qualification run에서 물려받은 원장은 1 GiB를 유지한다. Snapshot은 각30,000/합계60,000행, 행64 MiB/선언 합계1 GiB다. JSON 반복 pass·graph pair/cycle·filter/predicate/comparator·unit stdout scan·해시·복사·output sealing의 보수적 비용을 무거운 처리 전에 차감한다. Compact node가 공유하는 type/field 문자열은 node별로, capture가 재참조하는 type 문자열은 capture별로 계상하며 tree digest와 capture 연결에 취소 checkpoint를 적용한다. Partial conversion마다 반복하는 공유 field table의 길이 곱도 사전 계상하고 table·node·partial 순회에 같은 중단 검사를 적용한다. Batch 분할은 예상 응답 수와 footer에 필요한 행 수로 제한한다. Member/tool/input 및 replay-result 예약의 중단 오류는 모순 finding으로 바꾸지 않고 반환하며 이미 확인한 FAIL만 유지한다. 사전 invocation 결속의 실패도 해당 Build/Control 및 연결된 tuple counter에 즉시 보존하며 후속 미완료 상태로 덮지 않는다. Caller가 한도 값을 지정하는 옵션은 없고 값은 승인된 context가 정한다. 서로 맞는 approval·registration 쌍을 만든 caller는 paged context를 선언해 1.5 GiB를 쓸 수 있으므로, public API의 논리 처리량 최악 상한은 paged context에서 1.5 GiB다.

정상/변조 synthetic 검사는 외부 승인 경계의 동작 검사이며 runtime 자격 근거가 아니다. 보관한 Linux raw192는 요청/receipt/stderr·identity·snapshot·summary를 부분 수용하는 근거다. 실제 binary/library/입력/current-r4/승인 graph가 없으므로 전체 PASS로 승격하지 않는다. Producer 보관 확대·claim/reducer/inventory/fan-in·전체 Windows/Linux route 및 C# 실제 재사용 증명은 후속 단위이며 이 API 구현으로 Issue #216을 닫지 않는다.

### #362 ordinary inventory 선택 decode

내부 `parseQualificationInventoryRoute`는 아직 ordinary pass가 없는 P2-C의 입력 처리 기반이다. P2-C는 같은 guarded session에서 읽고 독립 승인된 전체 SHA-256과 대조한 `qualification-c1.json` bytes를 이 함수로 전달해야 한다. 함수도 전체 bytes의 SHA-256을 재대조한다. 공개 `ParseQualificationInventory`와 기존 qualification·semantic·replay 호출자는 전체 decode와 기존 오류 분류를 유지한다. 새 공개 API나 한도 선택 옵션은 없다.

전체 문서는 AST를 만들지 않는 strict token 검사로 UTF-8·surrogate·escape-equivalent 중복 key·문법·trailing value·depth32·2,000,000 values를 확인한다. Token 순회는 header와 전역 검사용 작은 투영 및 원문 route 구간만 수집한다. Header의 cell 수는 전체 route×platform 곱과 같아야 하며 platform ID, route ID, 전체 ordinary/extra workload set, extra-role ID와 route ID 충돌, workload의 route 연결을 검사한다. Extra-role의 platform 참조·실행/미적용/유예 분할 및 `reuse-accepted`의 OS/arch별 실행·유예 조건도 유지한다. Query/case ID·coverage·source/edit·requirement 및 extra workload 내용의 의미 검사는 workload 안의 계약이다. 선택한 route의 내용만 기존 의미 검증기를 적용하고, 다른 route의 내용 검증은 그 route를 처리하는 pass의 의무로 남긴다. 선택 결과는 전체 inventory로 오인되지 않는 별도 내부 type이며 header에서 routes/extra_roles를 제거하고 원래 route index와 선택 route를 돌려준다.

엄격 token 검사의 예약은 `S(n,z,v)=3n+2z+32768+16v`, AST 없는 typed decode의 예약은 `T32(n,z,v)=5n+32z+32768+80v`다. `n`은 bytes, `z`는 문자열 내부 공백을 포함한 유효 JSON bytes, `v`는 member name과 닫는 괄호를 제외한 값 수다. T32는 작은 전역 투영과 선택 route에만 적용한다. Registry는 전체를 typed decode하며 P0의 presence/null·exact membership·pin 검사를 생략하는 근거로 사용하지 않는다. 현재 base에는 P0 reader가 없으므로 실제 P0 자료의 전체 typed decode는 test-only 측정 경로에서 대조한다. P2-C는 통합된 P0 reader의 계약과 registry의 inventory pin을 함께 검사해야 한다.

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

### 같은 session의 paging identity 재사용 (Refs #362)

Semantic caller가 같은 run·원장과 열린 root를 전달한 approval-bound paged session에서만 guarded read의 paging part를 private record로 보관한다. Graph의 같은 owner·role·prefix에 속하는 part가 연속된 index 순서이면 읽기 loop의 같은 chunk를 개별 SHA-256과 전체 chain에 함께 넣고 part bytes를 retain하지 않는다. Descriptor는 뒤의 기존 검증 단계에서 owner·kind·index·offset·경로·크기·개별 pin·전체 logical pin을 모두 대조한다. Chain record의 순서와 소비한 part 집합도 정확히 같아야 한다. Graph/policy/node의 후속 검증이 owner를 무효화할 수 있는 경우와 비연속·역순 group은 bytes를 retain해 기존 검증과 실패 finding 순서를 보존한다. Retained record는 유일한 backing slice를 소유하며 일반 `member()`에 별도 과금한 복사본만 준다. Streamed part의 bytes 요청은 빈 slice 대신 내부 typed 오류 `ELIGIBILITY_MEMBER_BYTES_UNAVAILABLE`다. Callback·standalone eligibility·미검증 snapshot은 기존 part SHA-256과 전체 chain 검증을 유지한다.

내부 record는 live session/run/원장, 열린 root와 read 전후 physical identity, snapshot, graph·approval·registration·witness·policy pin, candidate/cohort, owner/node·role·path·bytes·SHA-256에 결속한다. Open 전후와 읽기 완료 뒤 SameFile·size·mtime, link·regular-file 및 읽기 한도 검사는 그대로다. Streamed chain은 같은 guarded read의 실제 bytes만 hash한다. Retained 경로는 기존 `h.Write`를 수행하고 두 경로 모두 외부 build/approval logical pin과 대조한다. Descriptor도 bytes·digest를 다시 확인해 immutable한 문자열과 ordered part pins로 보관한다.

Eligibility가 전체 검사와 output sealing까지 정상 완료한 같은 session에서만 semantic set이 그 logical identity를 재사용한다. 저장된 PASS 문자열이나 공개 bool/hash는 권한이 아니다. Set은 manifest 전체 member/plan 집합과 `Case=""`, descriptor 물리 pin, ordered part pins, build closure index·grammar/static closure, producer/runtime/compiler/build manifest, 외부 profile·registration·정책·candidate/cohort를 계속 대조한다. Descriptor 또는 build/profile pin이 다르면 재사용으로 치환을 승인하지 않는다. `semanticSnapshotIdentity`의 앞뒤 physical 검사는 유지한다.

Encoding 결과를 소비하지 않는 discovery metadata/source scan·evidence·replay·qualification read는 detector를 생성하거나 실행하지 않는다. Corpus·grammar와 별도 ZIP 검증의 encoding 판정은 유지한다. Guarded read의 논리 비용은 `(bytes+1)*(2+detector+retain+sink 수)`다. `detector`와 `retain`은 실제 실행/보관할 때만 1이다. Streamed part는 retain 없이 chain sink 하나를 쓰므로 추가 chain pass를 계상하지 않는다. Retained verified part는 추가 chain에 `bytes+1`, generic callback은 part digest와 chain에 `2*(bytes+1)`를 계속 계상한다. Set은 완료된 같은 logical identity의 두 추가 hash pass를 회피한다.

Eligibility registration의 bytes 소비 계획을 확인한 snapshot에서만, bytes를 소비하지 않는 `driver`·`native-executable`·`reference-executable`·`runtime-library`·`compiler` member의 identity만 보관한다. 같은 guarded read에서 전 bytes와 pin을 검증한 owner·role·path·bytes·SHA-256이다. Presence-only 소비자는 이 identity를 다시 결속하고 bytes 요청은 `ELIGIBILITY_MEMBER_BYTES_UNAVAILABLE`로 거부한다. Request·receipt·source·query·manifest·descriptor와 기타 실제 bytes 소비 경로는 retain을 유지한다. Registration이 이 bytes 소비 pin에 binary/part role을 붙이더라도 해당 경로는 retain하며, 예외 보관 경로마다 `256+len(path)` B를 예약한다. Role 문자열만으로 bytes 불필요를 판단하지 않는다. 해당 eligibility snapshot을 소비하는 semantic set의 plain driver도 같은 snapshot의 identity로 존재와 manifest pin을 대조한다. Standalone reference connection·승인 없는 semantic snapshot처럼 이 계획이 없는 호출자는 기존 bytes 보관을 유지한다.

새 관리 비용 `E`는 별도 과금한다: session 2,048 B, guarded part `512+len(path)` B, source 512 B, part 결속·ordered pin 보관 `256+len(path)+len(role)+len(SHA256)` B, logical record `1,024+2*(descriptor_bytes+1)` B, set logical 재결속 256 B 및 기존 member metadata 비용이다. Streamed group은 1,024 B와 part마다 `128+len(path)+len(role)+len(SHA256)` B, identity-only member는 `256+len(path)+len(role)+len(SHA256)` B를 더한다. 일반 retained bytes 복사에는 `bytes+1` B를 더한다. Descriptor decode와 신뢰 경계 검증 비용은 남고 신규 결속·비교·planning의 work/checkpoint도 공유 원장을 사용한다. 이 절감은 한도 인상, standalone 1.5 GiB의 상속 또는 ordinary 전체 수용을 뜻하지 않는다.

## Parser-only paging 문맥

`full-input-utf8-paged-parser/r1`은 단일 작은 UTF-8 native-query source의 큰 grammar parser에 한정된다. 세 approval/registration/result r1과 witness r2의 필드 의미는 유지하며 Windows/Linux만 받는다. `Native.Grammar[original_path]`는 실제 `grammar-paged-descriptor` 물리 pin을 가진다. 실제 `Native.InvocationID`의 NATIVE_DRIVER node가 descriptor와 모든 part의 owner여야 하며, 조각은 동일 guard의 검증된 cached bytes에서만 소비한다. 참조하지 않은 paged member와 다른 owner는 거부한다. 전체 logical pin은 승인된 원본 plain build manifest에 대조하고 선언된 descriptor 해시만으로 승인하지 않는다. 정확한 분할 규칙과 변경되지 않는 small 예산은 [계측 계약](instrumented-query-reference.md#216-parser-only-paging)이 소유한다. 이 문맥에서 eligibility 검증이 직접 시작한 원장(approval 결속)의 논리 처리량은 1.5 GiB이며 다른 small 한도는 같다. `VerifyQuerySemantics`·Replay처럼 원장을 상속하는 경로는 이 문맥에서도 1 GiB다. 큰 source/CST 또는 새 macOS 검증을 추가하지 않는다.

`full-input-utf8-paged-binaries/r1`은 paged-parser 규칙을 포함하되 16 MiB를 넘는 grammar parser를 0개 또는 1개 허용한다. driver executable과 reference library 중 하나 이상이 descriptor로 보존돼야 하며, 이 조건은 approval에 결속된 registration의 executable·library pin role로만 판정한다. NATIVE_DRIVER의 executable이 `driver-paged-descriptor`이면 descriptor와 모든 part는 그 invocation node가 소유한다. 재결합한 logical SHA-256을 approval의 `executable_sha256`과 대조하고, 사용하지 않거나 다른 node가 소유한 `driver-paged-*` member는 거부한다. 다른 context에서 descriptor role의 executable은 승인 값과 무관하게 거부한다. reference library의 descriptor는 그 library node가 소유하며 사용하지 않는 `library-paged-*` member는 거부한다.
