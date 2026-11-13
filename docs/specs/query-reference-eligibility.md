# Query reference offline eligibility 계약 (#216)

## #370 V2-E legacy 기본 승인 집합

`Pins`/`RuntimePins`가 없는 경로의 도구 identity는 `src/contracts/evidence-pins/repository-defaults.json`이 승인한 보존 자료에서 읽는다. 이는 `approval_source=repository-default`인 **저장소 검토 승인**이며 caller가 특정 evidence subject에 독립 승인한 `caller` 등급과 구분한다. 기본 승인은 기존 eligibility Approval, graph/member 결속, reader 의무 또는 실제 실행 증명을 대신하지 않는다. 명시 Pins의 승인 실패를 기본 집합으로 재시도하지 않는다. Reference-only Pins가 runtime tuple을 지정하지 않은 경우에만 기존과 같이 runtime 기본 정책을 사용한다.

검토된 기본 문서는 content-addressed 집합과 policy의 raw SHA-256을 데이터로 고정한다. 내장 자료의 집합 digest, descriptor 및 object의 원문 길이·digest를 대조한 뒤에만 tuple과 정책을 해석한다. 이 문서와 코드 자체를 함께 바꿀 수 있는 저장소 게시자는 신뢰 기점이다. 같은 권한으로 기본 승인값까지 바꾼 행위를 hash만으로 탐지하는 서명 체계는 아니다. Evidence 안의 자기 hash, 현재 설치 도구, 생산 `selection.json` 또는 live driver 원문에서 기본 승인을 발견하거나 자동 발행하지 않는다. 기존 object/set은 불변이며 새 자료와 집합만 추가한다.

기본 문서는 `tsgk-repository-default-selection/r1`이며 `set_sha256`, reference fallback catalogue/tuple, runtime fallback manifest, reference/runtime obligations와 기존 logger 집합의 digest를 고정한다. `default-sets/<sha256>.json`의 `tsgk-repository-default-set/r1`은 `reader=tsgk-repository-defaults/r1`, policy의 raw SHA/bytes와 object SHA/bytes 목록을 가진다. 이는 기존 `sets/`의 caller용 `tsgk-evidence-pin-set/r1`과 별도 형식이다. `objects/<sha256>`의 `tsgk-repository-default-policy/r1`은 같은 fallback·의무 anchor를 반복해 결속하고 reference tuple 목록, runtime tuple/manifest와 `logger`/`closure` scope를 기록한다. 기존 logger 집합의 항목을 추가 승인 과정에서 삭제할 수 없다. 이 형식은 기본 자료 전용이며 임의 외부 파일이나 새로운 reader를 동적으로 불러오지 않는다.

Reference 기본 tuple은 정확한 catalogue와 rustc/cargo release 조합을, runtime 기본 tuple은 manifest·ABI·driver·CP949·shim 조합을 결속한다. 승인된 추가 catalogue/closure는 등록에 기록된 identity로 선택한다. 모르는 identity에 맞춰 기대값을 생성하지 않는다. 안정된 fallback anchor는 B/C 항목을 추가해도 A를 가리키며, A 자료와 승인 scope를 그대로 보존한다. 과거 logger 전용 entry는 logger만 허용한다. 이를 전체 native closure, accepted-event 또는 preprocess-CST 승인으로 승격하지 않는다. 현재 지원 reader의 의미·필수 의무·분모·한도는 기본 정책으로 줄일 수 없다.

기존 legacy 결과의 schema·판정·code·counter·소비 identity와 JSON bytes를 보존한다. 새 `approval_source` 필드를 과거 결과에 삽입하지 않으며, 승인 출처는 호출의 Pins 선택과 기존 nested pin 결과로 구분한다. `RepositoryEvidenceDefaults()`의 별도 `tsgk-repository-evidence-approval/r1` 결과는 `approval_source=repository-default`, 기본 승인 문서의 raw `approval_sha256`, 실제 `pin_set_sha256`, `policy_sha256`을 반환하므로 caller는 원래 결과를 다시 쓰지 않고 이 provenance를 함께 기록할 수 있다. 명시 Pins의 승인 출처는 `caller`이며 실제 approval/set/subject digest는 기존 nested 결과가 소유한다. 기본 자료를 읽을 수 없거나 무결성이 모순되면 승인하지 않는다.

이 보조 provenance는 개별 legacy 결과나 subject에 암호학적으로 결속한 sidecar가 아니다. 결과와 호출의 Pins 선택·보조 provenance를 함께 보관할 책임은 caller에게 있으며, 나중에 최신 기본 집합을 조회한 것만으로 과거 결과의 승인 출처를 소급 인증하지 않는다.

기본 자료는 배포한 소스에 포함된 유한한 신뢰 fixture이며 초기 검증 뒤 불변 상태로 공유한다. 증거마다 policy를 다시 읽거나 승인 자료 비용을 새로운 evidence counter에 더하지 않는다. 기존 catalogue decode/digest와 native manifest/driver/CP949/shim의 원장 비용은 같은 bytes에 대해 그대로 유지한다. 명시 caller Pins는 기존 상속 원장에서 계속 비용을 낸다. Legacy 판독은 최신 생산 source를 참조하지 않으며 새 생산은 실제 driver/runtime 결속을 계속 검사한다.

“버전 literal 금지”는 검증 코드에 새 release·ABI·manifest allowlist를 박아 넣는 행위를 뜻한다. 보존 원문 SHA·tuple의 ABI/release와 schema/reader ID는 검토 대상 데이터 또는 구현한 판독 의미다. `rust-path-1.97.1/r1`처럼 의미를 식별하는 reader를 버전 문자열로 자동 재해석하지 않는다. 새 identity가 같은 reader 의미와 맞는지는 승인 항목 추가 때 검토한다. 승인 정책의 scope를 늘리거나 A를 철회하는 변경은 단순 도구 업그레이드와 별도 계약 변경이다.

아래 V2-A~D의 “기존/current/legacy” 설명은 원래 결과와 지원 scope를 보존한다는 뜻이다. 해당 기본값의 **저장 위치와 신뢰 출처**는 이 V2-E 절이 소유한다.

## #370 V2-D Roslyn 승인 관측과 provenance

`src/internal/ppexpect`의 reader는 현재 설치 SDK, 생산 selection, 고정 Roslyn release를 조회하지 않는다. 기대값 생성 전에 caller가 신뢰한 `roslyn-pins.json`의 두 독립 승인 digest와 exact observation subject를 기존 `ResolveEvidencePins`로 검증한다. 필수 자료는 정확히 `roslyn/tuple.json`(`roslyn-tuple`), `roslyn/observed.json`(`roslyn-observation`), `roslyn/cases.json`(`roslyn-cases`) 세 개다. 이름·역할·원문 SHA·bytes·필수 개수의 축소나 교환을 허용하지 않는다. 이 집합은 reference build 생산 집합을 대체하지 않는다.

Tuple은 `schema=tsgk-roslyn-pin-tuple/r1`, `reader=tsgk-roslyn-selection/r1`, `observation_sha256`, `cases_sha256`, `observation_schema`, `roslyn`, `language_version`, `scope`를 결속한다. `language_version=Latest`는 구현한 reader 의미이며 서로 다른 Roslyn의 Latest가 같다는 자동 승인이 아니다. 사례의 순서·ID·분모와 cases 원문을 대조하고 선택 기대값은 승인 관측에서만 도출한다. 관측은 평가기를 호출해 생성하지 않는다.

`scope=legacy-observation`은 보존 r2 관측으로 기대값을 생성하는 범위다. 기존 5.9 관측, cases, inventory와 content-addressed object는 그대로 보존한다. 과거 SDK·DLL 원본과 측정값은 없으므로 SDK/DLL provenance 주장은 **BLOCKED**다. Legacy tuple에 provenance를 붙여 과거 측정으로 소급하지 않는다. 새 tuple/object/set은 추가하며 생산 pointer를 바꾸지 않는다.

`scope=sdk-dll-provenance`는 r3 관측과 tuple에 같은 `provenance`를 요구한다. 필드는 `sdk`, `oracle_sha256`, `code_analysis_sha256`, `csharp_sha256`다. SDK identity는 build 시 `NETCoreSdkVersion`을 assembly metadata에 넣고, producer(`src/dev/csharp-g1-roslyn`의 `pp-r3` 모드)는 자신의 실제 DLL 및 loaded Roslyn 두 DLL을 hash한다. SDK release나 DLL SHA를 현재값과 비교하지 않고 승인 tuple의 정확한 조합에 결속한다. 누락·불일치·미지원 reader/scope는 수용하지 않는다. 이 값의 일관성은 외부 승인에 조건부이며 SDK 배포본 전체 또는 실제 실행 진위를 독립 인증하지 않는다.

`roslyn-pins.json`은 `approved_approval_sha256`, `approval_base64`, `approved_pin_set_sha256`, `pin_set_base64`, `tuple_base64`를 가진 caller-trusted 개발 입력이다. Base64는 원문 bytes를 보존한다. 16 MiB 문서 한도와 strict JSON을 적용한 뒤 공유 resolver의 기존 read/work/logical 한도를 그대로 적용한다. 기본 차분 시험과 inventory 생성기는 저장소가 검토한 fixture envelope를 쓴다. 로컬 differential batch는 `observed-*.json.pins.json`을 별도로 공급해야 하며 새 관측을 자동 승인하지 않는다. 새 inventory 생성은 `TSGK_PP_INVENTORY_DIR`로 `cases.json`, `roslyn-observed.json`, `roslyn-pins.json`이 있는 별도 검토 디렉터리를 명시한다. 기본 호출은 기존 inventory bytes·schema를 유지한다. 새 관측을 채택할 때 syntax·CST 수동 등록의 유효성도 별도로 검토해야 한다.

Standalone replay·qualification의 외부 pin 전달은 [V2-D CLI admission](cli-and-profile.md)을 따른다. 기존 API·reducer의 opt-in 없는 의미와 schema를 유지한다. 실제 새 Roslyn/SDK 자격은 합성 자료 시험으로 주장하지 않는다.

이 experimental API는 caller가 evidence 밖에서 신뢰한 승인 문서에 조건부인 저장 근거의 일관성을 검사한다. 실제 실행 진위·서명·게시자 인증이나 전체 route의 qualification 완료를 증명하지 않는다. 검증 중 process·network·임시 파일·원래 호스트 경로 조회를 수행하지 않는다. 기존 [reference connection](instrumented-query-reference.md)의 r1 및 native/r2 의미는 유지한다.

## #370 V2-A 역사 자료 pin 입력

`ResolveEvidencePins(context.Context, EvidencePinsRequest)`와 `SelectEvidenceProductionPin(ctx context.Context, selection, pinSet []byte) (string, error)`는 별도 opt-in의 offline API다. V2-A는 **원문 자료의 승인 결속**만 구현한다. opt-in 없는 eligibility 호출의 catalogue·Rust release·runtime 검사와 기존 reducer 의미를 유지한다. 직접 eligibility의 Rust/catalogue 소비는 아래 V2-B1, 생산 및 공개 호출 전달은 V2-B2 계약을 따른다. Runtime은 아래 V2-C의 명시적 tuple, 일반 logger replay·qualification은 V2-C2, accepted·preprocess-CST는 V2-C3의 별도 reader 범위에서 지원한다. Roslyn 기대값 생성과 provenance는 위 V2-D 계약을 따른다. 이 절의 PASS를 build9/unit4/control53/assertion36 또는 semantic·accepted·preprocess-CST PASS로 사용하지 않는다.

`EvidencePinsRequest`의 `Approval`, `PinSet`, `Subject`는 원문 bytes이고 `Materials`는 정확한 논리 이름→원문 bytes map이다. Caller는 graph 밖에서 **독립 승인한** `ApprovedApprovalSHA256`와 `ApprovedPinSetSHA256`를 제공한다. 증거나 sidecar의 자기 hash를 그대로 이 두 field에 복사하는 것은 독립 승인이 아니다. API는 승인 작성자의 진위를 인증하지 않는다. Caller는 호출 중 slice/map을 변경하지 않는다. 함수는 입력을 수정하거나 file/process/network에 접근하지 않고 결과는 원문 slice/map을 소유하지 않는다.

모든 문서는 bounded strict JSON이며 모든 field는 필수다. Duplicate/unknown/null/type 오류, trailing JSON·invalid UTF-8와 기존 depth32 제한을 적용한다.

| 문서 | field와 의미 |
|---|---|
| `tsgk-evidence-pin-selection/r1` | `schema`, `pin_set_sha256` 하나. 생산 전에 한 번 선택해 고정할 후보이며 승인이 아니다. 선택 함수는 caller가 함께 제공한 집합 원문의 digest·strict shape 및 resolver와 공유한 descriptor 검증(schema/reader·행 수·이름·역할·bytes/hash·중복)을 수행한다. 자료 본문 결속과 독립 승인은 resolver가 별도로 검사한다. 역사 resolver는 pointer를 받거나 조회하지 않는다. |
| `tsgk-evidence-pin-set/r1` | `schema`, `reader`, 비어 있지 않은 `materials` 배열. 행은 `name`, `role`, `bytes`, `sha256`이다. |
| `tsgk-evidence-pin-approval/r1` | `schema`, `pin_set_sha256`, `subject_sha256`, `reader`, `scope`. 기존 approval/registration/profile 등 caller가 선택한 **하나의 원본 subject 전체 bytes**에 결속한다. 다른 graph/profile/candidate를 대표하는 원문으로 sidecar를 재사용하면 FAIL이다. 복수 문서 closure의 의미 검증은 해당 소비 API가 담당한다. |

지원 reader는 `tsgk-evidence-materials/r1`, scope는 `historical-materials`뿐이다. 이 reader는 자료를 opaque bytes로 결속하며 catalogue 규칙·Rust version stdout·ABI·Roslyn diagnostic을 해석하지 않는다. 역할은 `reference-catalogue`, `reference-input`, `reference-tuple`, `runtime-tuple`, `runtime-manifest`, `runtime-patch`, `runtime-readers`, `driver`, `cp949`, `shim-template`, `roslyn-observation`, `roslyn-cases`, `roslyn-tuple`이다. 알 수 없는 schema/reader/scope/role은 UNSUPPORTED이며 PASS가 아니다. 더 강한 scope는 승인 문서가 요구해도 지원하지 않는다. 후속 semantic reader는 고정 의무·분모·known-rule를 자체 소유해야 하며 이 배열로 줄일 수 없다.

이름은 host 경로로 열지 않는다. 기존 portable 상대 경로 검사, 1,024 bytes 상한과 대소문자 접기 중복 거부를 적용하고 입력 map은 정확한 이름으로 결속한다. 미사용·다른 대소문자 이름은 잘못된 입력이다. 동일 내용의 여러 역할은 각 정확한 이름을 제공한다. 역할·bytes·hash·순서는 승인된 집합 원문 digest에 함께 결속된다.

승인 원문·집합 digest를 외부 두 승인값과 먼저 대조한 뒤 sidecar의 집합/subject digest를 실제 원문과 대조한다. 자료와 sidecar를 모두 재해시해도 외부 승인값이 다르면 `PIN_APPROVAL_MISMATCH` FAIL이다. 다른 subject는 `PIN_SUBJECT_MISMATCH`, 제공된 자료 길이/hash 모순은 `PIN_MATERIAL_MISMATCH` FAIL이다. 외부 승인이나 승인 문서/집합이 없으면 `PIN_APPROVAL_MISSING` BLOCKED이며 완료하지 않는다. 선택한 자료가 없으면 `PIN_MATERIAL_MISSING` BLOCKED로 남기고 다른 행을 계속 검사한다. 확정 FAIL은 이후 누락·취소·자원 오류로 지우지 않는다. 구조 오류는 typed error다. 선택 함수의 원문 누락/불일치는 `PIN_SELECTION_MISSING`/`PIN_SELECTION_MISMATCH` invalid input이다.

`tsgk-evidence-pin-result/r1`은 실제 소비한 approval/set/subject raw SHA-256, reader/scope, 검증한 자료 descriptor 순서, `required`/`verified`, read/work/logical counter와 `complete`를 기록한다. `complete=true`는 배열 평가 종료이며 누락으로 BLOCKED일 수 있다. 조기 승인 거부나 오류는 false다. Subject의 기록 verdict나 NEW_RUN을 재작성하지 않는다. 생산 선택을 B/C로 바꿔도 같은 A 원문·외부 승인·reader의 판정과 소비 identity는 동일하다. 새 계획의 pin 요구는 별도 소비 계약에서 검사한다.

Caller deadline은 필수다. 기존 evidence-replay wall120초·문서/자료별16MiB·총읽기256MiB·자료10,000개, JSON decode/shape 각2,000,000 values, work32,000,000·logical1GiB를 재사용한다. 자료·문서·hash·index·shape·출력 descriptor 비용을 같은 원장에서 처리한다. 입력은 이미 caller memory에 있으므로 이 API의 원장은 caller의 사전 파일 읽기를 대신 제한하지 않는다. V2-A는 CLI file admission이나 nested run에 연결되지 않는다. 후속 통합은 상속 원장과 기존 file identity/hardlink/reparse/alias 검사를 유지해야 한다.

`src/contracts/evidence-pins/selection.json`은 단일 후보 선택이다. `sets/<raw-sha256>.json`과 `objects/<raw-sha256>`는 내용 주소의 보존 자료이며 승인 기관이 아니다. 기존 객체를 갱신하지 않고 새 객체/집합을 추가해 선택만 옮긴다. `.gitattributes`의 `-text`는 원본 개행을 보존한다. 최초 집합은 catalogue와 reference controller 입력, 현재 manifest/patch/driver/CP949·실제 shim 생성식 template, 기존 Roslyn/r2 원문·cases 및 기존 logger가 허용한 과거 manifest를 보관한다. 과거 manifest는 repository commit `10c65c8`의 원문이며 기존 digest `affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080`과 대조했다. Shim template와 reference tuple은 이행 metadata이며 과거 실행 관측으로 소급하지 않는다. Runtime 원본 checkout 전체·과거 driver closure·Roslyn DLL/SDK provenance는 복원하지 않았다. 자료 보관만으로 full historical eligibility를 주장하지 않는다.

생산 Python/workflow의 선택 입력은 [생산 연결 계약](instrumented-query-reference.md#370-v2-b2-단일-생산-pin)을 따른다. 기존 verifier의 opt-in 없는 호출은 이 pointer를 소비하지 않는다. 기존의 현재 catalogue 전용, 현재 runtime 완결 검증 및 과거 runtime BLOCKED 규칙은 해당 legacy API에 계속 적용된다. Historical opt-in도 과거 결과를 새 reference의 관측으로 승격하지 않는다.

## #370 V2-B1 승인 historical reference 검증

`QueryReferenceEligibilityRequest.Pins *EvidencePinsRequest`는 직접 `VerifyQueryReferenceEligibility` 호출의 명시적 opt-in이다. `nil`이면 기존 r1 결과와 내장 catalogue·Rust release 검사를 유지한다. `Pins`가 있으면 결과 schema는 `QueryReferenceEligibilityPinsResultSchema`(`tsgk-query-reference-eligibility-result/r2`)이며, 승인 누락·취소·오류에도 이 선택을 유지한다. 원래 approval/registration/witness/graph bytes와 schema는 바꾸지 않는다.

V2-A resolver와 같은 구현·상속 원장에서 sidecar·집합·자료를 검증한다. `Pins.Subject`는 **이 요청의 `Approval` 원문 전체와 같은 bytes**여야 한다. 이 approval이 registration·witness·policy·candidate를, registration이 graph와 각 owned member를 기존 방식으로 결속한다. Materials scope의 PASS만으로 eligibility를 승인하지 않고, 기존 caller-trusted eligibility 승인과 아래 reader 검사를 모두 요구한다. 증거가 자기 자료를 승인하는 경로는 없다.

집합에는 정확한 이름과 역할의 `reference/catalogue.json`(`reference-catalogue`), `reference/tuple.json`(`reference-tuple`)이 필요하다. Tuple의 모든 field는 필수다: `schema=tsgk-reference-pin-tuple/r1`, `rustc_release`, `cargo_release`, `path_reader`, `catalogue_sha256`. 두 release는 각각 `숫자.숫자.숫자` 형식이며 build identity의 유일한 `release:` field와 정확히 같아야 한다. 중복 release·접미사·한 도구만 다른 혼합 tuple은 FAIL이다. tuple은 독립 승인 집합에 결속되며 release 문자열만으로 도구를 승인하지 않는다. 지원 path reader는 `rust-path-1.97.1/r1`이다. 이 ID는 역사 경로 의미를 고정하며 새 tuple의 의미 호환은 caller 검토 대상이다. 미지원 reader/schema는 typed UNSUPPORTED다. Target은 기존 platform별 target 및 build paths/identity의 정확한 대조로 고정한다.

Catalogue 원문 digest는 tuple, 기존 approval, registration과 모두 같아야 한다. `src/kit/query_eligibility_obligations_r1.json`은 생산 catalogue와 별개로 보존하는 이 reader의 고정 의무다. Build9/unit4/control53/assertion36의 수뿐 아니라 ID·순서·known-rule·argv·한도·필수 invocation 연결·grammar/control 입력·tree/capture/response 전체를 대조한다. Caller가 축소한 catalogue를 독립 승인해도 `PIN_CATALOGUE_OBLIGATIONS` FAIL이다. 이 파일의 의미를 새 생산 pin에 맞춰 갱신하지 않는다. 다른 의무를 지원하려면 별도 reader 검토가 필요하다.

변경 가능한 catalogue provenance는 upstream/archive/binding identity와 정확한 controller/reference input digest map이다. `reference/` 아래 `Cargo.toml`, `main.rs`, `instrumentation.patch`, `build.py`, `checks.py`, `check.py`의 `reference-input` 원문을 해당 digest에 결속한다. Build identity와 control/current reference 응답의 upstream은 같은 승인 catalogue commit에 결속한다. 기존 reference connection wrapper는 자체 legacy commit을 유지한다. Build identity의 input/tool hash·path·cohort, version stdout과 receipt, source/vendor/lock/metadata/binary 검사와 기존 검증 한도는 모두 유지한다. 원문 자료 누락은 `PIN_REFERENCE_MISSING` BLOCKED, catalogue/입력 결속 모순은 `PIN_REFERENCE_CATALOGUE`/`PIN_REFERENCE_INPUT` FAIL, 다른 subject 사용은 `PIN_ELIGIBILITY_SUBJECT` FAIL이다. V2-A의 strict 형식·alias·미사용·재해시 거부와 확정 FAIL 보존 규칙도 상속한다. 구조적으로 필수 역할이 잘못됐으면 `PIN_REFERENCE_ROLE` invalid input이다.

r2 결과의 `pins`는 실제 resolver 결과이며 추가 `reference_tuple_sha256`, `reference_path_reader`와 기존 `catalogue_sha256`은 소비한 역사 identity다. Outer `bytes_read/work/logical_bytes`에는 pin resolve와 의미 검증 비용이 모두 포함되고, nested `pins` counter는 resolver 구간의 차분이다. 바깥 한도를 새로 시작하거나 늘리지 않는다. 이 API는 현재 생산 selection이나 설치 도구를 조회하지 않는다. 같은 역사 입력·승인·reader를 고정하면 생산 선택 A/B/C 변경 뒤 결과·counter·소비 identity가 같다.

V2-B1의 직접 eligibility reference 검증에 이어 V2-B2는 아래 공개 전달 계약과 [생산 연결 계약](instrumented-query-reference.md#370-v2-b2-단일-생산-pin)을 사용한다. Runtime tuple이 없는 호출은 기존 current-only 경계를 유지한다. 아래 V2-C opt-in의 저장 closure PASS도 새 reference나 historical runtime의 실제 실행 자격으로 승격하지 않는다.

## #370 V2-B2 공개 reference pin 전달

`QuerySemanticsRequest.Pins *EvidencePinsRequest`는 내부 eligibility에 같은 원문·독립 승인값을 전달한다. Subject는 `EligibilityApproval` 원문이다. Opt-in 결과는 `QuerySemanticsPinsResultSchema`(`tsgk-query-semantics-result/r2`)이며 내부 `eligibility`는 V2-B1 r2와 실제 소비한 `pins`, tuple/catalogue identity를 보존한다. 취소·승인 누락·형식 오류에서도 선택한 바깥 schema를 유지한다. `Pins=nil`이면 기존 semantic r1과 eligibility r1을 유지한다.

Pin 자료를 새 원장에서 다시 검증하지 않는다. 기존 semantic/replay/qualification 원장에 resolve·의미 검사·자료 비용을 합산하고 small 한도와 확정 FAIL/최초 code/부분 counter 보존 규칙을 유지한다. 생산 selection과 설치 도구는 역사 검증 입력이 아니다. Raw subject·run-binding·producer의 recorded verdict·NEW_RUN과 기존 approval/registration/witness schema는 다시 쓰지 않는다.

Replay·qualification 및 CLI의 명시적 opt-in은 [identity/evidence](identity-and-evidence.md#370-v2-b2-공개-pin-context와-저장-결과)와 [CLI admission](cli-and-profile.md#370-v2-b2-historical-reference-pin-admission)을 따른다. 기존 reducer의 의미를 넓히지 않는다. Historical runtime tuple의 추가 소비는 아래 V2-C를 따른다. Accepted·preprocess-CST는 아래 V2-C3, Roslyn provenance는 위 V2-D를 따른다.

## #370 V2-C 승인 historical runtime 검증

기존 `Pins` opt-in의 승인 집합에 `native/runtime-tuple.json`(`runtime-tuple`)을 추가하면 eligibility와 semantic 소비 경로가 historical runtime reader를 선택한다. Tuple이 없으면 V2-B의 reference-only 의미와 기존 runtime 검사를 유지한다. `Pins=nil`인 호출의 결과 schema·현재 runtime 완결 검증·과거 runtime BLOCKED 경계는 바꾸지 않는다. Runtime 선택은 생산 `selection.json`이나 설치된 도구를 조회하지 않는다.

Tuple의 필수 field는 `schema=tsgk-runtime-pin-tuple/r1`, `reader=tsgk-runtime-closure/r1`, `logger_reader=tsgk-runtime-logger/r1`, `manifest_sha256`, `driver_sha256`, `cp949_sha256`, `shim_template_sha256`, `runtime_language_version`, `runtime_min_compatible`다. ABI 두 값은 양의 uint32이고 min≤max여야 한다. 승인 tuple은 하나의 호환 조합이며 manifest·driver·ABI를 다른 tuple에서 골라 섞지 않는다. Logger reader는 기존 `tsgk-reuse-log/r1`의 의미와 수집 한도를 그대로 구현한다. 다른 reader/schema는 UNSUPPORTED다. Tuple·자료·sidecar를 재해시해도 외부 두 승인 digest와 subject 결속을 생략할 수 없다.

필수 원문은 `native/runtime-manifest.json`(`runtime-manifest`), `native/driver.c`(`driver`), `native/cp949_table.h`(`cp949`), `native/shim-template.c`(`shim-template`) 및 manifest가 지정한 `native/runtime-*.patch.json` 세 개(`runtime-patch`)다. Tuple digest와 승인 집합 descriptor, resolver가 확인한 실제 bytes/hash를 함께 대조한다. Manifest는 strict `tsgk-native-runtime/r1`이며 repository·commit·file/patch 행을 검증한다. `query_runtime_obligations_r1.json`은 생산 선택과 별개인 reader 소유 의무다. 파일83개·patch3개의 정확한 경로/순서와 shim 생성식을 고정하며, 축소·중복·교체한 inventory나 shim을 caller가 승인해도 `PIN_RUNTIME_OBLIGATIONS` FAIL이다. Patch 원문의 hash/bytes와 manifest의 patch 결속도 요구한다. 이 의무 파일을 새 생산 pin에 맞춰 수정하지 않는다. 다른 inventory/생성식 의미에는 새 reader 검토가 필요하다.

검증된 manifest의 전체83개 file hash/bytes와 당시 driver·CP949·symbol별 shim을 저장 build manifest의 전체 closure에 대조한다. Grammar physical pins·source·producer build digest·compiler/step 순서·raw response·witness·실행 파일의 기존 검사는 유지한다. 모든 NATIVE_DRIVER invocation의 기록 manifest와 ABI 상하한을 승인 tuple과 정확히 대조하며, historical이라는 이유로 ABI를 생략하지 않는다. Logger capability·각 marker·raw/nativewire와 CaseClaim/Judge까지 같은 검증 문맥을 전달한다. 자료 누락은 `PIN_RUNTIME_MISSING` BLOCKED, tuple/자료·patch 불일치는 `PIN_RUNTIME_BINDING`/`PIN_RUNTIME_PATCH` FAIL이다. 이미 확인한 FAIL은 이후 누락·취소·자원 오류가 지우지 않는다.

Eligibility r2 결과의 선택적 `runtime_pin`(`QueryRuntimePinIdentity`)은 실제 tuple raw SHA, manifest raw SHA, reader/logger reader 및 ABI 상하한을 기록한다. Reference-only 결과에는 이 field를 내보내지 않는다. 기존 historical Pins용 semantic r2·`oracle-set-r7`/replay r4·qualification r11 경로는 같은 context를 전달한다. 저장된 eligibility PASS는 `runtime_pin`과 nested `pins.materials`의 tuple·manifest identity가 일치하고 driver·CP949·shim·patch3개를 포함한 고정8개 descriptor가 올바른 역할·양의 bytes로 각각 한 번씩 있어야 한다. Legacy result/reducer의 의미를 넓히지 않는다.

Semantic record의 producer·ABI·static closure, BOUND native cache와 raw Expected도 검증한 runtime을 사용한다. Semantic base의 기존 claim 재계산에도 같은 logger context를 전달하며, 일반 native-result replay용 wrapper는 nil 의미를 유지한다. Raw response에서 기대 manifest를 복사하거나 승인 ABSENT에서 runtime 자료만으로 BOUND 권한을 만들지 않는다. 검증 권한은 subject별로 분리하고, 공유 budget을 다음 target에 재사용해도 이전 runtime context를 상속하지 않는다. 자료/decode/hash/의무/closure의 비용은 기존 read/work/logical/wall 원장에 합산하며 한도를 늘리지 않는다.

보존 집합 `6cd6a117001a1d90ab174e65df6fd735773e6af682c5ede43978cdfedb50f408`은 V2-A의 현재 원문과 runtime tuple, `d301defb285c098a95bb5d6e4e7e01209eee545761969bca14cdd54994416801`은 runtime 상향 직전 repository commit `29dd21a`의 manifest·driver와 같은 CP949·patch·shim 생성식에 결속한다. 원래 object/set과 생산 selection은 바꾸지 않는다. 이 자료는 승인 후보이며 외부 caller 승인이 아니다. 당시 실제 evidence의 build identity가 이 driver를 사용했는지도 별도로 대조해야 한다.

과거 runtime checkout 원문83개와 당시 실제 r4/build/승인 graph를 복원하거나 실행하지 않았다. 기존 verifier와 같은 **승인 manifest inventory에 대한 저장 closure 일관성**을 검증할 수 있을 뿐, 미보존 실제 실행 증거를 PASS로 생성하지 않는다. Logger 호환만으로 accepted event나 preprocess-CST 지원을 추정하지 않는다. 일반 logger replay·qualification은 아래 V2-C2의 새 opt-in으로 확장한다. Accepted API/replay와 preprocess-CST API/replay의 historical runtime 전달은 V2-C3에서 추가했다. Roslyn 및 standalone replay CLI admission은 위 V2-D, 미보존 실제 역사 provenance는 BLOCKED 경계를 따른다.

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

독립 승인한 native/r4 기대값과 실제 source에 ApplyEdits를 적용한 모든 version bytes/hash/points로 nativewire Expect를 구성한다. CheckHeader/Step/Open/End, reuseobs.Decode/CaseClaim/Judge, querywire/querypolicy 및 comparator를 재사용한다. Mandatory logger prefix PASS만으로 full logger COMPLETE를 대신하지 못한다. Build manifest는 승인 ProducerSHA256에 결속하고 full runtime/driver closure와 grammar physical pins를 대조한다. Grammar 역할은 parser/scanner/header이며 parser는 정확히1개다. Canonical quoted argv와 version/runtime/parser/scanner*/driver/shim/link의 필수 순서·개수 및 version argv를 검사한다. 현재 고정 runtime manifest의 ABI 호환 범위13..15도 승인/raw 값과 대조한다. Grammar의 language/symbol/field count는 caller가 독립 승인한 기대값과 raw producer를 대조하며 C source를 새로 해석하거나 컴파일해 이 숫자를 추출하지 않는다. Runtime tuple이 없는 기존 호출에서는 현재 내장 runtime manifest만 완결 검증 대상이며 과거 manifest는 BLOCKED다. 명시적 runtime tuple은 위 V2-C reader를 사용한다. 실제 실행 여부는 승인 provenance의 신뢰 범위다.

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

TSQL dynamic SQL은 위 기본 예약에 work와 logical 각각 `128n² + 64(E+1)A + 64EWS`를 더한다. `E`는 `exec` capture 수, `W`는 `errwith` capture 수, `S`는 source bytes, `A`는 source 안의 유효 capture span 길이 합이다. 고정 개수의 match pair index/정렬·반복 child/known-miss 탐색, exec별 text 검사, exec×errwith의 반복 공백 gap/정규식 span 검사를 덮는다. 문자열 decode와 variable 결과도 포함한다. 각 곱은 overflow 검사 후 별도로 청구하며 아직 실행하지 않은 query의 예산을 소급 청구하지 않는다. C#과 TSQL 이외의 dynamic SQL(현재 fact pack에는 없다)은 기존 보수적인 `64n²(n+S+M+1)` / `1024n(n+S+M+1)` 식을 유지한다.

C# dynamic SQL(#438)은 위 기본 예약에 아래 세 항을 순서대로 별도 청구한다. 이전 식 `64n²(n+S+M+1)`은 파생의 실제 연산(`n log n`)보다 capture 수에 대해 훨씬 빨리 커서 n이 25 안팎(source 1 KB 안팎)만 넘어도 한 shard가 pass work 한도를 단독으로 넘었다. `A`는 `oc.arg` capture 수, `L`은 `ini.left`와 `ma.name` capture 수, `Q`는 `qn.name`·`aq.name`·`gn.name` capture 수, `Z`는 type이 `identifier`이거나 이름이 `bin.op`·`ini.left`·`ma.name`인 capture 중 유효한 span(`0 ≤ start ≤ end`) 길이의 최댓값이다. 각 곱은 overflow 검사 후 청구한다. 한도와 fact 의미는 바꾸지 않는다.

| C# 항 | work | logical |
|---|---:|---:|
| 정렬 pass | `64·12·n·(bits.Len64(n)+2)` | `1024·12·n` |
| type 이름 사슬 | `64·A·(Q+1)` | 0 |
| span 해석 | `128·2(A+L)(Z+1)` | `64·2(A+L)(Z+1)` |

이 식은 `deriveCSharp`의 상한이다. 한 step(요소 하나를 한 번 건드리는 연산)을 work 64로 세고 text 한 byte의 읽기·쓰기를 work 8로 세는 위 공통 capture 변환 예약의 단위를 그대로 쓴다. **이 표의 logical은 "도출이 만드는 모든 할당의 누적 byte(해제 여부와 무관, 한 번의 peak나 RSS가 아님) + 변환한 capture 크기"의 상한이다.** (1) 정렬 pass: `capIndex.pairs`는 정확히 11회 불리고(lastIdent 3, `bin`, `arg`, `oc`, `oc.a`, `ini.as`·`ma.as`의 left·right 4) 호출마다 match를 두 번 훑고 pair를 만들어 정렬하며 site 정렬이 한 번 더 있다. 11개 호출의 자식 capture 이름은 모두 달라서 정렬 요소 합은 capture 수 이하이고, `pdqsort`의 비교를 요소 수×`bits.Len64`의 3배 이하로 보면 전체 step은 `22n` 스캔 + `2n` 출력 + `6n·bits.Len64(n)` 비교 이하다. 출력은 pair 출력(11개 호출의 자식 capture 수 합 ≤ n)과 site 정렬의 출력(site 수 ≤ A+L ≤ n)을 합쳐 `2n`이다. 합은 `24n + 6n·bits.Len64(n)`이고 식 `12n·(bits.Len64(n)+2)`는 `24n + 12n·bits.Len64(n)`이라 상수항은 정확히 같고 비교 항에 2배 여유가 있다. 식의 값이 모든 n에서 이 합 이상이다. logical은 pair 출력·map 항목·site·결과의 capture당 약 830 B 이하를 덮고 시험이 실측한 누적 할당은 이 항의 약 0.2배 이하다. (2) 사슬: `resolve`는 `(oc.a, oc.arg)` pair마다 한 번 불리고(≤ A) 한 번에 걷는 hop은 아래 guard로 lastIdent 항목 수(≤ Q) 이하다. (3) span 해석: `srcText.span` 호출은 pair마다 사슬 끝 identifier 한 번, site마다 identifier 값 또는 `+` 연산자 한 번, assignment의 left 한 번이라 `2(A+L)` 이하이고 각 호출은 `Z` 이하의 span만 읽는다. 호출 하나의 work는 원본 byte마다 64(읽기 1, 쓰기 최대 3: CP949의 잘못된 byte 하나가 3 byte `U+FFFD`가 된다)에 `csharpIdentifierName`이 디코드한 text(최대 3Z byte)를 읽고 쓰는 byte당 8을 더한 `64 + 8·6 = 112 ≤ 128`이다. 그래서 `128·2(A+L)(Z+1)`은 `64·decoded-bytes + 8·text-bytes`의 상한이다. 호출 하나의 logical은 `srcText.span`과 `csharpIdentifierName`이 원본 byte당 누적 할당하는 byte다. 1 MiB span에서 실측한 최댓값은 CP949 잘못된 byte 32.03(span 16.01 + 이름 정규화 16.01, `strings.Builder` 성장 포함), UTF-16LE 12.6~20.6, UTF-8 1.0~6.0, CP949 정상 text 10~17.5이고 계수 64는 최댓값의 2배다. 유효하지 않은 capture 범위는 `DeriveDynamicSQLChecked`가 도출 전에 거부한다.

C# 형 이름 해석(`resolve`)은 lastIdent 사슬을 반복으로 걷고, hop이 항목 수를 넘으면 그 type을 미해결로 두어 해당 site를 건너뛴다. 서로 다른 노드의 사슬은 항목 수보다 길 수 없으므로 순환이 없는 입력의 hop과 결과는 같다. 조작되거나 저장된 capture 목록에서 노드가 자기 자신 또는 서로를 마지막 identifier로 가리켜도 도출은 스택 overflow(회복 불가 fatal) 없이 끝나고 나머지 site의 fact는 유지된다.

Test-only overlay로 실제 loop/comparator 방문, hash/span 입력 bytes와 구체화한 capture·결과 payload를 별도로 관찰한다. PostgreSQL/TSQL 실제 owned 입력과 많은 capture·긴 문자열·큰 XML 및 반복 declaration frontier에서 각 query의 `계상 ≥ 실제`를 확인한다. 이 logical payload 검사는 process 전체 할당이나 heap/RSS peak의 실측이 아니다. C# 도출의 실제 step은 `factStepHook`(`pair-steps`·`chain-hops`·`decoded-bytes`·`text-bytes`)이 센다. C# 항의 "상한"은 이 hook이 센 step과 `runtime.MemStats.TotalAlloc`의 누적 증가분에 대한 것이다(그 밖의 연산은 위에서 항목별로 묶었다). 정렬 비교 횟수가 map 순회 순서에 따라 달라지므로 이 hook은 원장 하네스가 쓰는 `ordinaryVisitHook`과 분리되어 있고, 시험 전용이며 nil이면 아무것도 하지 않는다.

## #362 P2-C ordinary eligibility activation boundary

P2-C는 `VerifyOrdinaryQuerySemantics`의 private binding/session adapter를 연결했다. Required 분모는 public request의 필수 caller `Selection`에서 commitment proof 및 registration 검증으로 먼저 고정하며, 이후 `approval.ordinary`의 동일한 pin bundle이 맞아야만 session을 bind한다. Caller selection은 approval이 아니고 비어 있거나 invalid하면 input error다. Approval이 없으면 selected obligations는 BLOCKED이며 API 결과는 `Complete=false`/`ProcessingComplete=false`다. 기존 공개 `VerifyQueryReferenceEligibility`는 `ordinary-batch-full-input-utf8/r1`을 계속 거부한다. Ordinary API만 `tsgk-query-reference-eligibility-approval/r1` 안의 strict `ordinary` binding을 받아들인다. Binding은 실행 `profile_pin`, 계획 `planning_profile_pin`, ordinary `registration_pin`, `asset_registration_pin`, `shard_plan_pin`, `plan_commitment_pin`, `reference_selection_pin`, `run_binding_pin`, `closure_encoding`, candidate/cohort/run 및 선택 shard ID 전체를 묶는다. 같은 approval의 기존 registration digest는 eligibility registration bytes를 계속 묶는다. 일반 eligibility의 native witness registration은 보내지 않으며 빈 bytes가 필수다. 이 경계를 통해 ordinary batch reader가 구형 single-case witness를 새 의미로 해석하지 않는다.

Ordinary 호출의 eligibility registration은 ordinary context, `Native == nil`, 빈 legacy `observations`를 사용한다. Build·control invocation, evidence graph, owned member와 caller approval은 기존 typed eligibility 경로에서 같은 `run`, `eligibilityBudget`, held `os.Root`, evidence snapshot으로 검증한다. Ordinary batch reader가 tuple을 직접 검사하므로 기존 단일 native `verifyNative`와 그 tuple witness를 실행하지 않는다. Approval 안의 ordinary cohort ID는 현재 eligibility cohort ID와 같아야 한다. Pin이나 cohort가 불일치하면 session은 bound가 되지 않고 결과에 FAIL이 남는다. 기존 eligibility public entry에서 이 block을 전달할 방법은 없다.

`closure_encoding`은 `plain`, `parser-paged`, `binary-paged` 중 하나를 caller가 고정한다. Paged 선택은 해당 binding과 같은 eligibility snapshot 안에서만 private verified part authority를 만든다. Root·session·approval·cohort가 다른 part record는 재사용할 수 없다. 연결된 authority가 없으면 기존 guarded full-read/hash 경로를 사용한다. Ordinary 호출은 semantic/replay로부터 1 GiB 원장을 상속하고 paged encoding으로 eligibility 전용 1.5 GiB 직접 진입 한도를 받지 않는다. Inventory 및 registry 입력은 fixed source pin을 먼저 확인하고 `parseQualificationInventoryRoute` 선택 decode와 typed registry decoder를 공유 원장에서 실행한다. Valid HARD receipt를 확인한 focused Windows JSON public call은 actual owned r8로 tuple 1/1을 VERIFIED로 끝냈으며 native child는 1 GiB/30 s, `JOB_OBJECT` cleanup, 잔류 자식 0이었다. 여섯 build-step receipt는 각각 4 GiB HARD/120 s였고, stricter native cap은 test-only runner overlay로 강제했다. 제품 기본 4 GiB native operation policy는 바뀌지 않았다. 원장 값은 logical 206,346,731 B, work 7,835,933, read 11,216,999 B다. 이 입력의 complete-plan scaffold는 전체 P0 source-only exact-union provenance가 아니고 approval/reference row도 synthetic이므로 선택 JSON 외의 closure나 전체 P0를 입증하지 않는다.

이전 P2-C SQL 측정은 full admission으로 이어지지 않았다. PostgreSQL `alt-postgresql-sql-B04-22`의 owned r8 selected body/reference/adjudication/fold/identity/seal 원장은 logical 697,519,126 B, work 14,102,601, read 134,443,515 B였다. Independent 10-node grammar-authored CST와 9 PostgreSQL9-anchor metadata, empty captures/declaration facts를 비교했지만 9 tuple은 base fact-reproduction gate 미재계산으로 `ORDINARY_PREREQUISITE_INCOMPLETE`/BLOCKED였다. TSQL `alt-tsql-B01-r001`은 binary-paged owned r8 사용 중 `ordinary-base-tree-projection`에서 logical usage 1,073,275,806 B까지 갔고, 다음 1,731,872 B 예약에 1,265,854 B가 부족해 `ELIGIBILITY_WORK_LIMIT`으로 멈췄다. Work/read는 각각 31,403,098/126,923,139 B였다. TSQL의 40 reference rows는 synthetic placeholders이고 expected assets는 UNAVAILABLE이므로 이 중단 뒤 비용은 측정하지 않았다. 두 SQL route 모두 `CAPACITY_UNPROVEN`/BLOCKED다. 후속 registration·required ID·plan header 최적화 뒤 2026-11-12 같은 owned-native 선택 TSQL 공개 경로는 seal까지 logical 942,338,579 B, work 31,240,589, read 126,923,146 B에서 끝났다. Expected 부재와 synthetic reference는 유지되므로 이는 해당 fixture의 전체 제어 흐름 측정이며 full SQL admission/semantic PASS가 아니다.

2026-11-12 ordinary base fact 후속 검증은 위 PostgreSQL의 미재계산 gate를 해소한다. Adapter는 같은 guarded session에서 fact pack·mapping을 한 번 확인한 뒤, P2-B2의 ordered suffix/query SHA와 mapping revision을 결속하고 편집 후 각 source version 및 incremental/fresh 각각의 raw captures로 declaration/dynamic SQL/XML facts를 다시 도출한다. 전체 step·role·query의 저장 `step_facts`와 step0 base `facts` projection을 대조하고, 기존 case claim의 step0 의미를 유지한 재계산 값을 공용 `semanticBase`에 전달한다. 저장 fact 자체는 독립 expected가 아니며 tuple의 metadata/CST/capture/fact 독립 비교를 생략하지 않는다. Legacy single small semantic admission은 fact pack을 받지 않으며 r4–r6와 r10 의미·한도를 바꾸지 않는다.

2026-11-12 TSQL 독립 expected 후속 검증은 선택 shard `alt-tsql-B01-r001`(원문 267 B, 4 edits, 5 versions, 8 query, 72 tuple = 8 query × [step0 incremental + step1–4 incremental/fresh] 9개)의 expected asset 107개(metadata 8, full CST 9, ordered captures 72, facts 18)를 손으로 작성해 pin한다. CST는 prepared `grammar.json`의 production(`program`/`batch`/`statement`/`select`/`term`/`field`/`invocation`/`window_function`/`set_statement` 등)과 external scanner 원문(GO 줄 끝, `marginalia`, `QUOTED_IDENTIFIER` 상태)에서 도출하고 byte 범위만 원문에서 토큰을 찾아 계산한다. 쿼리 metadata는 query 원문의 pattern 수와 capture 이름 최초 출현 순서이며, ordered capture는 손으로 쓴 tree와 pinned runtime `query.c`의 capture cursor 규칙(finished-state heap의 start byte·pattern index·완료 순서, match id는 첫 yield 시 부여, 복사 state는 원본의 id·consumed 수를 물려받음)에서 도출한다. 작성 시 parser·native·reference 출력은 사용하지 않았고 native 첫 실행 전에 asset 본문의 SHA-256을 먼저 기록했다.

같은 actual owned r8에서 실제 raw 응답을 `nativewire.Check`로 검증한 뒤 제품 tuple adjudicator(`adjudicateOrdinaryTuple`)에 직접 넣는 `TestOrdinaryTSQLExpectedAgainstNative`(opt-in)는 72/72 tuple을 VERIFIED로 판정한다. 첫 대조에서 dynamic SQL capture 8개 asset(step1–4 × incremental/fresh)의 match 구성이 달랐다. 원인은 작성 모델이 `ts_query_cursor__copy_state`(`query.c`)의 복사 방향을 반대로 가정한 것이었고, 복사본이 원본의 id·consumed 수를 물려받는다는 소스 근거로 교정했다. 이 교정은 봉인 대비 8개 asset만 바꾸며 나머지 99개는 같다. Root capture를 가진 wildcard child pattern의 definite 여부(`tsqlDefinite[12]`)는 생성된 parse table의 정적 분석 결과라 grammar.json만으로 손으로 증명하지 못한 모델 매개변수다. 이 값은 native 관측과 일치하도록 정했으므로 해당 pattern에 한해 독립 증거로 세지 않는다. 같은 test는 expected의 node 하나·capture 하나·fact 하나를 변조하면 변조를 소비하는 tuple만 FAILED(`ORDINARY_EXPECTED_TREE_MISMATCH` 8개 query tuple, `ORDINARY_CAPTURE_MISMATCH` 1개, `ORDINARY_FACT_MISMATCH` 1개)가 되고 나머지는 VERIFIED로 남음을 확인한다.

공개 `TestOrdinarySQLPublicPassCapacity`의 TSQL 경로는 expected 107개를 소비하며 reference row 40개가 expected와 같은 full CST·ordered captures를 담는다. 위 단일 preflight 이후 `reference-rows` 단계는 통과한다. 단계 진입 시 남은 logical 144,733,009 B에서 71,807,549 B와 work 2,736,458을 쓰고 남은 logical은 72,925,460 B, work는 4,638,749다. 다음 단계인 독립 tuple 판정에서 tuple 10개를 VERIFIED로 처리한 뒤 `ordinary-expected-tree`의 다음 logical 예약 6,400 B에 2,815 B가 부족해 `ELIGIBILITY_WORK_LIMIT`(실제 원인 logical)으로 멈춘다. 중단 시 logical 1,073,738,239 B, work 28,380,235, read 129,947,602 B이며 required 72/verified 10/failed 0/blocked 0/unprocessed 62, `CAPACITY_UNPROVEN`/BLOCKED다. 다음 지배 항목은 tuple마다 expected tree asset을 다시 decode하는 비용이다. `adjudicateOrdinaryTuple`의 `ordinary-expected-tree`와 `ordinaryReferenceExpected`의 `ordinary-reference-expected-tree`가 같은 asset을 tuple마다(72번) 각각 `256+512·field` 허용량으로 decode하며 tuple 10개에 35,347,051 B와 32,278,976 B(합 약 6.7 MB/tuple)를 썼다. 서로 다른 tree asset은 9개다. 한도는 바꾸지 않았다. 이 단계의 후속 결과는 아래 “#362 ordinary expected asset의 공유 decode와 원장” 절이 대체한다. PostgreSQL은 같은 공개 pass에서 9/9 VERIFIED를 유지하며 logical 620,458,108 B, work 8,619,803이다. Reference executable/library는 실행·load하지 않았고 approval·reference row는 합성이므로 외부 reference와의 실행 독립성은 입증하지 않는다.

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

Reference row는 전체 strict token preflight에서 네 head scalar를 같은 typed 결과 객체에 수집한다(Refs #362 B5(b)). Head shape 오류는 전체 문법·중복·UTF-8·depth/value 검사를 마친 뒤 반환한다. Non-exact head → schema → FAILED exact head 또는 full shape의 순서를 유지한다. FAILED arm은 첫 extra member를 거부하며, 성공 arm만 기존 streaming full shape 검사와 한 번의 typed unmarshal을 적용한다. 별도의 전체 head shape 순회·typed unmarshal과 두 번째 syntax preflight는 없다. FAILED의 extra 필드·잘못된 failure code 및 뒤쪽 구문 오류의 우선순위는 기존과 같다. Receipt r1의 stdout/stderr base64와 row 분할·offset·hash 결속은 유지한다. B5(a)의 receipt r2·별도 stdout pin과 B5(c)의 capture span 예약 변경은 후속 범위다.

결합 row의 기본 logical 예약은 full decode의 `5n + 32z + 32768 + 80v`에 head/result 상태의 1,024 B를 더한다. Full descriptor·container·고정 배열 예약은 그대로다. Preflight에서 실제 관찰하는 root와 top-level member token마다 work 1을 추가하고, head 필수 필드 네 개 검사에는 work 4를 예약한다. FAILED/schema 거부 arm도 같은 보수적 preflight 예약을 사용하되 수행하지 않은 full shape·typed pass를 추가로 청구하지 않는다. 별도 head 문서 decode의 예약을 제거한 것이며 한도를 낮추거나 높이지 않는다.

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

**passes=3 고정(경화).** Row는 preflight, typed shape 순회, `Unmarshal`로 문서를 세 번 걷는다. 소스에 counter를 주입해 세어 본 결과 preflight와 shape 순회는 각각 문서의 모든 token(이름·값·여는·닫는 token)을 정확히 한 번 읽는다(tiny row 222/222, 20,000 node row 720,078/720,078). `TestReferenceRowWorkCoversWalkVisits`는 독립 token 순회로 센 T에 대해 decode 계상 work ≥ 3·T를 ordinary·공유 호출 모두에서 assert한다(Unmarshal은 token 한 번 순회로 모델링한다). 이 시험은 passes를 3에서 1로 낮춘 mutant를 20,000 node row에서 죽인다(work 2,000,428 < 2,160,234). 이 값을 직접 고정하는 시험이 없었던 이전에는 같은 mutant가 모든 시험을 통과했다. 허용량 `8·sizeof(T)`(+16/member) 항은 이 값을 빼도 실패하는 입력을 찾지 못해(문자열 slice 100,000개에서 logical/실제가 2.5배에서 1.1배로 줄지만 1 이상) 필요성이 입증되지 않았지만, 같은 입력군에서 가장 얇은 여유(1.10배)를 지키므로 줄이지 않았다. 다만 ordinary expected facts의 `map[string]int`(`xml_structure`) 값은 항목당 약 350–425 B(키 64 B면 1,150 B)를 할당해 logical이 실제의 0.45–0.6배에 그쳤고(2차 보강 이전), 아래 map 항목 항으로 고쳤다.

**map 항목 항(compact와 비 compact 공통, 2·4차 보강).** Typed decode의 map은 항목마다 `15·len(key) + 389` B를 logical에 더한다(`compactMapEntryLogical`). 비용은 값이 아니라 이름에서 나온다. `jsontext`는 object의 member가 64개나 이름 1,024 B를 넘으면 중복 이름 검사용 namespace에 모든 이름을 복사한다(append 버퍼 최대 6배, 8 B offset 6배, 이름 복사본의 `map[string]struct{}` 슬롯과 이름 `len+16`). 이것이 preflight와 shape 순회 두 walk에서 각각 `7·len+144`이다. `Unmarshal`은 namespace를 끄지만 Go map slot 5개(`17·5`)와 key 문자열(`len+16`)을 할당한다. 값 자체의 크기는 기존 `8·sizeof`가 이미 계상한다. 측정(항목 50,000개, `map[string]int`, 키 3 B)은 preflight 137 B + shape 137 B + Unmarshal 75 B = 351 B이고 키 64 B는 508 + 508 + 134 B이다. Token만 읽어도 이 비용이 나오므로 항목 수가 아니라 이름 token이 원인이다. `TestCompactMapEntryAccounting`은 기본 실행에서 값 14종(int·bool·빈 문자열·16 B 문자열·빈 slice·slice 2·struct·포인터·중첩 map 등)을 키 3·8·64 B, 항목 2,000개와 10,000개로 디코드해 합계 logical ≥ 실제 할당과, 늘어난 8,000개의 한계 비용(logical 증가분/8,000 ≥ 실제 할당 증가분/8,000)을 assert하고(약 1.4초), 짧은 키(2 B 3,000개, 1 B 60개)와 map의 slice도 본다. 측정에 쓴 전체 행렬(항목 100,000개, 키 3·8·64 B, slice 안의 map 20,000·100,000개, 비 compact 바닥 확인 포함, CI에서 약 70초)은 `TSGK_LONG_BUDGET_TESTS=1`에서만 돈다. 전체 행렬에서 logical ≥ 실제 할당(`TotalAlloc` 세 번 중 최대)을 assert하며 최소 여유는 1.49배(중첩 빈 map, 키 64 B)다. 항을 0으로 한 mutant는 기본 실행의 44개 시험이, 키 길이 계수를 뺀 mutant는 키 64 B 12개가 실패한다(기본 실행에서 둘 다 죽는다). 실제 대상 `ordinaryExpectedFact.XML`은 `TestOrdinaryExpectedDecodeAccounting`의 `facts-xml-map`이 같은 방식으로 지킨다. 4차 보강에서 같은 항을 비 compact decode(`decodeTyped` 등)에도 넣었다. 이전에는 비 compact가 입력 byte마다 32 B를 더하는 구형 계상만 써서 항목 100,000개에서는 logical ≥ 실제였지만(여유 1.01–1.06배) 항목 1,000–3,500개·키 2–4 B에서는 실제 할당의 0.79–0.9배로 모자랐다(180개 입력 중 38개). 값 14종 × 키 1·2·3·4·8·16·64 B × 항목 50–100,000개(672개 입력)에서 compact와 비 compact 모두 logical ≥ 실제이며 최소 여유는 compact 1.29배, 비 compact 1.64배다. 이 항은 구형 결과 `eligibility`의 `logical_bytes`를 108,980,106에서 108,985,109로 늘렸고(work와 다른 모든 field, 다른 7개 구형 결과는 그대로다) `TestV2ELegacyResultParity`의 `eligibility` 고정 SHA를 `45651b3c…`로 갱신했다. 기본 실행의 `TestTypedMapEntryAccounting`은 비 compact를 키 3 B(모든 값)와 키 64 B(두 값)로 확인한다. 다른 compact 대상(expected tree·captures·metadata·facts의 slice·reference row·receipt)은 같은 패턴을 보이지 않았다(slice 원소 최소 여유 2.5배).

**효과.** 합성 reference row 40개의 decode+`reserveReference` 비용은 logical 222,514,364 B에서 41,291,129 B로, work 2,923,400에서 2,680,044로 줄었다. `reference-rows` 단계 전체(40 invocation의 receipt·row 포함)는 logical 71,807,549 B, work 2,736,458이다. Work는 거의 줄지 않는다. 한도, row 내용, expected, 사례·의무는 바꾸지 않았다. `OrdinaryReferenceEstimate`(plan의 density 추정)는 이 변경으로 바꾸지 않았고 현재 원장의 측정값이 아니다.

### #362 ordinary expected asset의 공유 decode와 원장 (Refs #362)

Ordinary 독립 tuple 단계(`ordinaryAdjudicateSet`)는 tuple마다 expected asset을 판정(`adjudicateOrdinaryTuple`)과 reference 대조(`ordinaryReferenceExpected`)에서 각각 decode했다. 선택 TSQL의 72 tuple은 서로 다른 tree asset 9개, metadata 8개, captures 72개, facts 18개를 읽으므로 tree와 metadata는 같은 asset을 여러 번, captures는 tuple마다 두 번 decode한 셈이었다. 같은 expected tree를 tuple당 6.7 MB(판정 3.5 MB + reference 대조 3.2 MB)로 계상해 tuple 10개에서 남은 logical이 바닥났다. 이 절은 ordinary 독립 tuple 단계에만 적용된다. 공용 호출과 직접 호출(`budget.decoded == nil`)은 매번 decode하며 같은 판정을 낸다.

**asset별 decode 횟수(tuple당, 변경 전 → 후).** 같은 body는 pass당 `(타입, 검증된 body)`마다 한 번만 decode한다.

| asset | 변경 전 | 변경 후 | 사용처 |
|---|---|---|---|
| tree | 2회/tuple (72 tuple에 144회) | pass당 asset마다 1회(TSQL 9회, 공유 사용 135회) | 판정, reference 대조 |
| metadata | 2회/tuple | pass당 asset마다 1회(TSQL 8회) | 판정, reference 대조 |
| captures | 2회/tuple | tuple마다 1회(captures는 tuple마다 다른 asset, reference 대조가 판정의 값을 공유) | 판정, reference 대조 |
| facts | 1회/tuple | 1회/tuple(공유 없음, compact 계상만 적용) | 판정 |
| tree 구조 검사(`ValidateTree`·point) | 1회/tuple | `(tree, source pin)`마다 1회 | 판정 |

**결속.** 공유 값은 `ordinaryAssetResolver`가 pin 3개(asset·authoring basis·independent review)를 검증해 돌려준 byte slice에 묶인다. Cache key는 `(decode 대상 타입, body 첫 byte 주소, 길이)`라 다른 asset이나 다른 byte가 값을 대신 쓸 수 없다. Resolver는 매 사용마다 asset의 role·state·pin 존재를 확인하고, 이미 풀어 둔 body를 돌려줄 때도 asset 기록의 pin이 검증했던 pin과 같고 `pin.Bytes`가 body 길이와 같은지 다시 확인한다(`ORDINARY_ASSET_PIN`). 한 asset ID에 다른 pin을 주장하는 사용은 검증된 body를 받지 못한다. Tuple별 binding(case·query·step·role·source pin 비교)은 공유 값에 대해 매번 수행하므로 다른 tuple에 묶인 body를 읽은 tuple만 `ORDINARY_EXPECTED_TREE_BINDING`/`ORDINARY_EXPECTED_CAPTURE_BINDING`으로 FAIL한다. Tree 구조 검사의 결과는 tree의 source pin(binding으로 tuple의 pin과 같음이 확인되고 source byte는 그 pin의 hash로 확인됨)별로 저장한다.

**변경 불가.** Go에는 읽기 전용 slice가 없으므로 공유 값의 소비자(`ValidateTree`, `ordinaryTreePointsValid`, `ordinaryTreeFieldOnlyDifference`, `CompareTrees`, `CompareCaptures`, `ordinaryCapturesValid`, `reflect.DeepEqual`)가 쓰지 않는다는 점을 `TestOrdinaryExpectedShareValuesStayUnchanged`가 고정한다(한 pass를 마친 뒤 모든 공유 값이 새로 decode한 값과 `DeepEqual`). Decode 실패(`KindInvalidInput`)도 같은 오류로 재사용하고 자원 한도·취소 오류는 저장하지 않는다.

**Decode 원장(`decodeOrdinaryExpected`, metadata·tree·captures·facts 공통).** 기존 `decodeWitnessTyped`의 `32·significant + 32,768`과 `256 + 512·field`를 #405의 ordinary row 식으로 바꾼다. 바깥 `charge(1,R)`, preflight `charge(1,R)`와 `3R`(preflight·shape 순회·Unmarshal), 오류·buffer `32·(maxPath+maxName) + 32,768`, 값마다 `80`, compact typed 값(`8·sizeof(T)`, struct는 member마다 `+16`)과 문자열 payload `R` 한 번이다. 오류 종류와 우선순위는 그대로다. 같은 asset을 다시 쓰는 사용은 decode 비용을 내지 않는다.

**공유 cache 자체의 비용.** 사용마다 조회 `work 1, logical 64`(`ordinary-expected-cache-lookup`)를 낸다. Pass당 container 한 번 `1, 1,024`, 새로 보관하는 항목마다 `work 1, logical 1,024 + 2R + 32·values`(`ordinary-expected-cache`)를 낸다. Tree 구조 검사 결과는 사용마다 조회 `1, 64`(`ordinary-expected-tree-check-lookup`)를 내고 새 `(tree, source pin)`마다 기존 검사 비용(`work 16·nodes + source + 1`, `logical 512·nodes`)과 결과 항목 `1, 1,024`를 낸다. 보관 항은 decode가 만든 Go 값이 body의 약 두 배와 JSON 값마다 32 B를 넘지 않는다는 측정에 근거하며 `TestOrdinaryExpectedCacheRetentionPriced`가 `HeapAlloc`(GC 두 번 뒤)으로 확인한다.

**계상 ≥ 실제.** 제품 counter는 없다. `TestOrdinaryExpectedDecodeAccounting`은 asset마다 `TotalAlloc`(GC 뒤 세 번 중 최대)에 대해 logical ≥ 실제 할당, work ≥ 3·(독립 token 순회로 센 값 수)를 assert한다. 정상 입력은 20,000 node tree, 12,000 capture, 4,000 B capture 이름, 50,000 B node type, 100,000개의 빈 capture 이름, 20,000개의 넓은 capture 이름 집합, 5,000 declaration fact이고 오류 입력은 31단계 중첩의 긴 이름, 1 MiB 잘못된 escape, 512 KiB 중복 이름, 100,000개 `{}` 배열, 넓은 미지 이름이다(관측 여유는 정상 형태 2.5–12.5배, 오류 입력 2.5배 이상). Opt-in `TestOrdinaryExpectedDecodeAccountingSQL`은 선택 TSQL의 실제 expected 107개에서 같은 부등식을 확인한다(tree 9개 alloc 507,344 B ≤ logical 6,777,546 B, captures 72개 1,261,944 ≤ 7,159,662, facts 18개 387,568 ≤ 1,715,724, metadata 8개 90,624 ≤ 361,026). 한 판정+reference 열의 순서·first FAIL code·finding 순서는 공유 유무와 무관하게 같음을 `TestOrdinaryExpectedShareKeepsJudgments`가 고정한다.

**효과(2026-11-12, 같은 owned-native r8, 한도 불변).** 이 절의 `final-identity` 중단은 아래 "#362 ordinary final identity 정렬 상한과 actual tree 검사 공유" 절이 해소해 TSQL이 72/72 VERIFIED로 끝나며, 다음은 공유 decode만 적용했을 때의 측정이다. TSQL 공개 pass는 독립 tuple 72개를 모두 처리한다(이전에는 tuple 10개에서 `ordinary-expected-tree` logical로 중단). 이 단계는 logical 41,837,162 B, work 1,999,968이고 `ordinary-expected-tree` 6,777,546 B·`ordinary-expected-captures` 7,226,094 B·`ordinary-expected-facts` 1,715,724 B·`ordinary-expected-metadata` 361,026 B·`ordinary-expected-cache` 2,334,318 B·`ordinary-expected-tree-check` 780,800 B와 조회(`ordinary-expected-cache-lookup` 27,648 B, `ordinary-expected-tree-check-lookup` 4,608 B)를 쓴다. 실제 decode는 tree 9회(공유 사용 135회), metadata 8회(공유 136회), captures 72회(공유 72회)다. 이 단계에서 FAIL은 없고, 최종 결과의 72 obligation은 모두 코드가 `ELIGIBILITY_WORK_LIMIT`인 BLOCKED다. 판정 시점의 BLOCKED였다면 자기 code를 유지하므로 72개 모두 이 단계에서는 VERIFIED였다가 아래 중단 때문에 BLOCKED로 바뀐 것이다. 그러나 다음 `final-identity` 단계의 `ordinary-final-identity-order` work 3,104,961이 남은 work 2,522,101을 582,860 넘어 `ELIGIBILITY_WORK_LIMIT`으로 멈춘다(logical은 29,995,902 B 남음). 따라서 TSQL은 72/72 VERIFIED에 도달하지 못했고, 중단 이후 요약은 `CAPACITY_UNPROVEN`이다. 이 값은 tuple 판정의 결과가 아니라 final-identity의 work 계상 때문이다. 다음 지배 항목은 `ordinarySnapshotIdentity`의 `8·bits.Len(paths)·pathBytes`(경로 913개, pathBytes 약 38.8 KB로 3,104,961) 뒤에 이어지는 경로별 physical identity `20·pathBytes`(약 0.78 M)와 result encoding(약 0.06 M)이며 이 값들은 추정이다. 추정한 final-identity 소요는 약 3.95 M으로 남은 2.52 M에 약 1.4 M이 모자란다. 한도는 바꾸지 않았다. 그 밖에 tuple 단계 work 2.0 M 중 같은 actual tree를 query마다 다시 검사하는 항(`ordinary-api-compare` 531,656, `ordinary-actual-tree-check` 217,264, `ordinary-tree-compare` 195,272)은 서로 다른 actual tree 9개를 72번 검사하는 중복이지만 이 변경의 범위가 아니다. PostgreSQL은 9/9 VERIFIED를 유지하며 `independent-tuples` 단계가 7,284,425 B에서 1,480,934 B로, 총 logical이 620,458,108 B에서 614,654,537 B로(work 8,619,803 → 8,591,349) 줄었다.

### #362 ordinary final identity 정렬 상한과 actual tree 검사 공유 (Refs #362)

위 공유 decode 뒤 TSQL 공개 pass는 `final-identity`의 `ordinary-final-identity-order`(work 3,104,961)에서 멈췄다. 이 절은 그 계상식과 독립 tuple 단계의 actual tree 검사를 실제 순회에 맞춘 결과다. 한도(logical 1 GiB, work 32,000,000, read 256 MiB, file 16 MiB, entries 10,000, wall 120초), expected·reference 내용, 사례·의무, 오류 분류·우선순위는 바꾸지 않았다. 두 변경은 ordinary 전용(`ordinarySnapshotIdentity`, `adjudicateOrdinaryTuple`)이며 공용 호출의 원장은 바뀌지 않는다.

**정렬 상한.** `ordinarySnapshotIdentity`는 경로를 `ordinarySortPaths`(bottom-up 병합 정렬)로 정렬한다. 병합 한 단계의 비교는 두 피연산자 중 짧은 쪽까지만 보고, 방출되는 원소는 비교한 두 원소 중 하나이므로 한 단계의 비용은 `Σ(len+1)=pathBytes` 이하다(비교 없이 복사하는 꼬리는 원소당 1). 단계 수는 `⌈log2 n⌉ = bits.Len(n−1)`이고 마지막에 buffer를 `paths`로 한 번 복사한다. 따라서 work는 `(bits.Len(n−1)+1)·pathBytes + 1`(n<2이면 1), logical은 buffer `16·n`에 allocator 반올림 한 page(8,192 B)를 더한 값이다(`ordinarySortWork`, `ordinarySortLogical`). 이전 식 `8·bits.Len(n)·pathBytes`의 계수 8은 증명 가능한 상한이 아니라 여유였고 logical은 0이었다. 정렬 결과는 문자열 오름차순으로 `slices.Sort`와 같으므로 첫 identity 실패의 결정성은 그대로다. `TestOrdinarySortPathsStaysWithinChargedWork`가 15개 입력군(빈 목록, n=1·2, 중복, 빈 문자열 4,096개, 공통 prefix 1,920 B의 경로 3,000개의 무작위·정렬·역순, 1 MiB 경로, 짧은 경로 100,000개, 2의 거듭제곱과 그 +1)에서 `slices.Sort`와의 동일 결과, 비교 횟수 ≤ `n·⌈log2 n⌉`, 비교 횟수+비교 bytes(짧은 쪽 길이의 상한)+이동 수 < 예약 work, 예약 ≤ 이전 식, 입력 복사를 뺀 할당 ≤ 예약 logical을 assert한다. `TestOrdinaryFinalIdentityOrderChargeCoversSortAndBuffer`는 긴 이름의 파일 1,500개로 `ordinarySnapshotIdentity`의 실제 charge가 정렬이 실제로 쓴 값과 buffer 할당을 덮는지, 바뀐 파일을 여전히 `SOURCE_CHANGED`로 거절하는지 확인한다.

**actual tree 검사 공유.** `ordinary-actual-tree-check`(`ValidateTree`와 point 검사), `ordinary-api-compare`(API shape와 `CompareAPI`), `ordinary-tree-compare`(actual과 expected tree 비교)는 query와 무관하게 `cache.checked`의 같은 node table에서 tuple마다 반복됐다. `eligibilityBudget.decoded`가 있는 ordinary pass에서는 `(node table 첫 node 주소, node 수, API table 주소)`를 key로 한 항목에 결과를 보관하고 `(tree, source pin)`마다 구조·point 검사를, tree마다 API 검사를, `(tree, 공유 expected tree)`마다 비교를 한 번 한다. 공유 표시는 읽기 전용이며 결과는 문자열 두 개와 bool뿐이다. 사용마다 tuple의 source pin(source byte는 pin의 hash로 이미 확인됨)과 tuple이 결속한 expected tree를 결과의 key로 다시 쓰므로 같은 node table을 다른 source나 다른 expected tree로 읽으면 새로 검사한다. 공유 비용도 계상한다: 사용마다 조회 `work 1, logical 64`(`ordinary-actual-cache-lookup`), 새 항목과 결과마다 `work 1, logical 1,024`(+보관하는 detail 길이, `ordinary-actual-cache`). 새로 하는 검사의 charge는 이전과 같은 식이다(`16·nodes + source + 1`/`512·nodes`, `16·(nodes+apiRows)+1`/`512·nodes`, `16·expected nodes+1`). 한 가지 달라진 점은 API shape 순회(`ordinaryAPIShapeValid`)가 이전에는 어떤 charge보다 앞서 예약 없이 실행됐으나 이제 `ordinary-api-compare` charge 안에서 먼저 예약한다는 것이다.

시험: `TestOrdinaryActualShareKeepsJudgmentsAndRunsOncePerTree`는 query 24개가 한 node table을 읽는 tuple과 API가 어긋난 tree, node table이 잘못된 tree, API shape가 잘못된 tree를 섞은 열을 공유 있음/없음으로 두 번 실행해 판정·finding 순서가 `DeepEqual`이고, 검사가 tree 4개·API 3개·비교 5개만 실제로 실행되며(공유 없음은 tuple마다), 각 검사가 charge한 work ≥ test-only hook(`ordinaryVisitHook`)이 센 실제 방문 수이고, 조회 work가 정확히 예상 횟수임을 확인한다. `TestOrdinaryActualShareRebindsToTheTupleSource`는 같은 node table을 서로 다른 source로 읽는 tuple을 두 순서로 실행해 point가 어긋나는 source의 tuple만 `ORDINARY_ACTUAL_TREE_POINTS`가 되는지 본다. `TestOrdinaryActualShareLargeTreeManyQueries`는 20,000 node tree를 query 64개가 읽을 때 검사가 1회이고(공유 없음 64회) 공유 pass의 logical이 할당을 덮으며 work가 23.0 M에서 1.6 M으로 줄어드는 것을 확인한다. `TestOrdinaryActualShareValuesStayUnchanged`는 pass 뒤 모든 공유 결과가 새로 한 검사와 같고 node table이 변하지 않았음을 확인한다. `TestOrdinarySQLPublicPassCapacity`는 실제 pass의 방문 수가 label의 charge 이하인지 확인한다(TSQL: 정렬 292,032 ≤ 426,933, tree 검사 4,274 ≤ 27,158, API 4,153 ≤ 66,457, 비교 1,525 ≤ 24,409). 검사에 쓰는 방문 수는 제품 counter가 아니라 같은 식의 입력 크기이고, 정렬의 비교 bytes는 짧은 쪽 길이의 상한이다.

**효과(2026-11-12, 같은 owned-native r8, 한도 불변).** TSQL `alt-tsql-B01-r001`이 72/72 VERIFIED로 seal까지 끝난다(`FULL_BODY_MEASURED_VERIFIED`, 선택 shard 결과, full_scope_complete는 false). 총 logical 1,033,483,056 B(남음 40,258,768 B), work 29,975,701(남음 2,024,299), read 129,947,593 B다.

| 단계 | logical B | work | 남은 logical B | 남은 work |
|---|---:|---:|---:|---:|
| reference-rows | 1,000,816,006 | 27,361,251 | 72,925,818 | 4,638,749 |
| independent-tuples | 1,031,777,264 | 28,535,375 | 41,964,560 | 3,464,625 |
| runtime-fold | 1,032,839,408 | 28,649,928 | 40,902,416 | 3,350,072 |
| final-identity | 1,032,892,620 | 29,837,881 | 40,849,204 | 2,162,119 |
| seal | 1,033,483,056 | 29,975,701 | 40,258,768 | 2,024,299 |

독립 tuple 단계는 logical 41,837,162 B/work 1,999,968에서 30,961,258 B/1,174,124로 줄었다(`ordinary-actual-tree-check` 217,264 → 27,158 work, `ordinary-api-compare` 531,656 → 66,457, `ordinary-tree-compare` 195,272 → 24,409, 검사는 각 9회, 공유 조회 288회 work 288, 항목 36개 work 36·logical 36,864 B). `final-identity` 단계는 work 1,187,953이며 `ordinary-final-identity-order` 426,933(logical 22,800 B), `ordinary-final-physical-identity` 758,893(경로 913개, `20·pathBytes`는 바꾸지 않았다), index 914가 대부분이다. 이어지는 `seal` 단계의 work 137,820은 `ordinary-result-encoding` 137,267과 `ordinary-result-bound` 553이다. PostgreSQL `alt-postgresql-sql-B04-22`는 9/9 VERIFIED를 유지하며 총 logical 614,594,867 B, work 7,498,825(이전 614,654,537 B, 8,591,349)이다. 이 절의 계상 근거 중 경로별 physical identity `20·pathBytes`는 이번 변경이 닿지 않은 항이고, 그 실제 비용은 경로의 component 수와 syscall에 따라 달라 증명된 상한이 아니다.

### #362 ordinary physical identity와 `CompareAPI`의 증명된 상한 (Refs #362)

위 절은 `ordinary-final-physical-identity`(`20·len(path)+1`)와 `ordinary-api-compare`(`16·(nodes+apiRows)+1`)가 증명된 상한이 아니라고 밝혔다. 이 절은 두 식을 실제 최악 비용으로 맞춘 결과다. 한도(logical 1 GiB, work 32,000,000, read 256 MiB, file 16 MiB, entries 10,000, wall 120초), expected·reference 내용, 사례·의무, 오류 분류·우선순위는 바꾸지 않았고 계상은 줄이지 않았다.

**physical identity.** 기존 `guard.lstat`은 경로를 `strings.Split`하고 component마다 `strings.Join`으로 prefix를 다시 만들어 `g.dirs`를 조회했다. 깊이 d, 길이 ℓ인 경로 하나가 이미 cache된 parent를 가져도 prefix 복사·hash가 약 `d·ℓ/2` byte여서 `20·ℓ`를 넘을 수 있다(깊이 d가 수백이면 비선형이다. cache된 ancestor를 건너뛰지 못하게 한 mutant는 깊이 300 시험이 2.4초에서 102초로 늘었다). `g.dirs`에는 모든 ancestor를 확인한 뒤에만 항목이 들어가므로 cache된 directory는 ancestor를 보증한다. 새 `lstat`은 가장 긴 prefix부터 한 번씩 조회해 가장 깊은 cache 항목을 찾고(parent가 cache면 조회 1번), 그 아래 component만 순서대로 확인한다. 반환값·오류 kind·code·path와 cache 상태는 기존과 같다(`TestGuardLstatMatchesReference`가 존재·부재·파일 parent·link 이름과 여러 순서에서 비교). `ordinarySnapshotIdentity`는 정렬된 경로를 걸으므로 parent가 snapshot에 있으면 parent가 먼저 확인돼 cache에 있다. 이 경로의 비용은 parent 조회 1번, 경로 scan 2번, Lstat 1번, insert 1번, snapshot map 조회 3번으로 byte당 약 8 단위 이하이며 식 `20·len(path)+1`(`ordinaryPhysicalIdentityWork`)을 그대로 둔다. Parent가 snapshot에 없으면 모든 ancestor가 uncached일 수 있어 ancestor마다 실패한 조회, Lstat, insert가 각각 경로 길이 이하이므로 `4·depth·(len(path)+1)`을 더한다. 실제 evidence snapshot은 ancestor를 포함하므로(walk가 root에서 내려오며 기록) 이 항은 합성 snapshot에서만 쓰이고 TSQL·PostgreSQL 공개 pass의 charge는 그대로다. 계상은 `os.Root`/`os.SameFile` 내부의 syscall을 직접 세지 않는다. 경로 하나의 Lstat은 component마다 최대 syscall 한 번이라는 가정(≤ ℓ/2+1)과 `SameFile` 두 번에 상수 8을 둔 모형이다.

**`CompareAPI`.** 기존 구현의 비선형 항: (1) 형제 탐색 `slices.Index`가 노드마다 4번(`Σ deg(parent)²`), (2) named 형제 탐색이 unnamed 형제를 훑음, (3) `positional`의 zero-width 형제 walk가 divergence마다 최대 `deg`, (4) field lookup마다 `a.FieldLookups` 전체와 `a.Names`를 선형 탐색(distinct (parent, field) 수 × 행 수), (5) point마다 root 자식 전체 scan, (6) parent마다 `seen` map 할당. 새 구현은 같은 결과를 내면서 children을 한 배열의 view로 만들고 노드의 형제 내 위치 `pos`, 가장 가까운 named 형제·named rank·parent별 첫 named 자식을 한 번에 구해 노드당 O(1)로 조회한다. `positional`은 방향·named별로 "x 뒤에 이어지는 같은 boundary의 zero-width 연속 길이"를 필요할 때 한 번에 계산하고(`apiRuns`) got의 rank와 비교해 판정한다. Field lookup 행은 `(parent, 이름 text의 첫 index)`로 한 번 색인하고(`first`, 그와 다른 첫 값 `other`로 "첫 불일치 행"을 보존), `seen`은 이름 index별 마지막 parent 표시로 바꿨다. Point는 root 자식의 end 누적 최댓값에서 이분 탐색한다("end가 b보다 큰 첫 자식"과 같다). 결과·첫 difference·divergence 순서와 panic하는 입력의 범위는 그대로다.

계상식은 `nativewire.CompareAPIWork`(work)와 `CompareAPILogical`(logical)이며 kit의 `ordinary-api-compare`, `ordinary-base-api`가 호출한다.
- work = `16·(nodes + rows) + points·bits.Len(nodes) + 2·(field bytes + name bytes) + 1`. `rows`는 API 표의 행 수 합이다. 모든 pass가 node·행에 선형이고(실측 노드당 약 11 방문), point는 이분 탐색 한 번이며, field·name 문자열은 hash와 비교로 최대 두 번씩 읽는다. 기존 식에 마지막 두 항이 더해진 것이므로 줄어든 계상은 없다.
- logical = `2048·nodes + 128·(names + fieldLookups)`. 기존 `512·nodes`는 표 값이고 node당 divergence가 최대 4개(`position_navigation` 하나씩)인데 divergence 한 개가 문자열과 결과 slice 증가를 포함해 약 350 B라서 실제 최악 할당이 `512·nodes`를 넘었다(zero-width 형제 3만 개가 모두 divergence를 내는 입력이 `TotalAlloc` 50.3 MB 대 15.4 MB 예약). 이 항을 `1,536·nodes`로 더하고, 이름 index와 lookup 행 map을 위해 항목당 128 B를 더했다. `ordinary-base-api`는 기존 식(`64·(nodes+rows+1)` work, `512·(nodes+rows+1)` logical)과 비교 상한(+`16·(nodes+rows)`: `ValidateTree`와 shape 순회) 중 큰 쪽을 쓴다.
- Native live producer(`internal/native/run.go`)와 offline 소비자가 같은 `CompareAPI`를 호출하며 native 쪽은 원장이 없다. 공용 결과·동작은 같고 호출 비용만 선형으로 내려가므로 native 쪽에서 확인할 계상이 없다. `ordinary-base-api`와 `ordinary-api-compare` 외에 `CompareAPI`를 예약하는 호출자는 없다.

**"계상 ≥ 실제".** test-only hook으로 실제 방문·바이트·할당을 세서 계상 이하임을 assert한다. 제품 counter는 없고 hook은 nil이면 아무것도 하지 않는다(`compareAPICostHook`, `guardLstatHook`).

| 시험 | 내용 |
|---|---|
| `TestCompareAPIMatchesReference` | 이전 구현을 시험 파일에 그대로 두고, 무작위 tree 30,000개(노드 1–40, byte 범위가 좁아 zero-width 형제가 흔함, 넓은 root·사슬 포함)와 무작위로 변조한 API 표에서 결과·difference·divergence가 `DeepEqual`임을 확인한다. 모든 check 종류와 divergence 100건 이상이 실제로 생성되는지 시험이 확인한다 |
| `TestCompareAPIDivergenceRuns` | 60개 zero-width 형제 run에서 방향 4개와 named 혼합, 노드마다 아홉 가지 got 값을 이전 구현과 비교 |
| `TestCompareAPIStaysWithinCompareAPIWork` | 형제 6만 개의 root, unnamed 형제 3만 개, zero-width 형제 3만 개가 모두 divergence인 표, 깊이 4만 사슬, 5만 노드 무작위 tree, 서로 다른 field 2만 개를 가진 노드, 4,000 B field 이름, tree보다 훨씬 큰 표(중복 행 4만 개씩)와 작은 표(노드 10행), point 3만 개. 방문+문자열 bytes ≤ `CompareAPIWork`, `TotalAlloc` ≤ `CompareAPILogical`(valid와 마지막 항목만 어긋난 표 모두) |
| `TestCompareAPIAdversarialMatchesReference` | 위 큰 입력 중 이전 구현이 감당할 수 있는 것을 결과 비교 |
| `TestGuardLstatMatchesReference` | 새 `lstat`이 이전 구현과 같은 관측·오류·cache를 내는지 |
| `TestOrdinaryPhysicalIdentityChargeCoversTheWalk` | 실제 directory 사슬: 깊이 300(단일 byte component), 깊이 100×200 B component(경로 20 KB), 255 B component와 같은 directory의 파일 2,000개, ancestor가 snapshot에 없는 깊이 200, 일부만 있는 깊이 120. 경로마다 guard가 scan·hash·넘긴 byte와 snapshot map 조회, `SameFile` 상수를 센 값이 그 경로의 charge 이하 |
| `TestOrdinaryPhysicalIdentityWorkFormula` | parent가 snapshot에 있으면 기존 식, 없으면 기존 식보다 큼 |
| `TestOrdinarySQLPublicPassCapacity`(확장) | 공개 pass의 physical 방문이 label charge 이하 |

**효과(2026-11-12, 같은 owned-native r8, 한도 불변).** TSQL `alt-tsql-B01-r001`은 72/72 VERIFIED(`FULL_BODY_MEASURED_VERIFIED`)로 seal까지 끝나며 총 logical 1,036,969,062 B(남음 36,772,762 B), work 29,981,965(남음 2,018,035), read 129,947,588 B다. 이전(#409) 대비 logical +3,486,006 B(`base-recomputation` 단계 +1,067,830 B는 `ordinary-base-api`의 `2048·nodes` 하한, 독립 tuple 단계 +2,418,176 B는 `ordinary-api-compare`의 logical), work +6,264(`ordinary-api-compare` 66,457 → 72,721, field·name bytes 항)다. PostgreSQL `alt-postgresql-sql-B04-22`는 9/9 VERIFIED를 유지하며 logical 614,619,833 B, work 7,498,825(변화 없음)다. `ordinary-final-physical-identity`는 TSQL 758,893(실제 방문 252,729), PostgreSQL 341,336(116,298)이다. 공개 pass의 `CompareAPI` 실제 방문은 kit에서 센 구조 방문(TSQL 4,153 ≤ 72,721)이며, `CompareAPI` 내부 방문 hook은 `nativewire` 시험에서만 보이므로 실제 TSQL tree의 내부 방문은 직접 세지 못했다.

### #362 replay profile member 정렬의 증명된 상한 (Refs #362)

Ordinary가 아닌 semantic replay(`replay.go`의 `chargeReplayMembers`)는 member identity text(`membersText`)를 만들기 위해 `replay-profile-members`를 예약한다. 이전 식 `32·levels·(fields+1)`은 이 정렬의 실제 비교량보다 수십 배 컸다(3,000 member·경로 200 B 공통 접두 입력에서 약 3.8억 work, 실제 방문 약 645만). `membersText`를 ordinary와 같은 `ordinaryMergeSort`(안정 병합 정렬)로 바꿔 상한을 증명했다. 정렬은 `ordinarySortWork(n, Σ(len(path)+1))`이고, 이어지는 text 작성은 member마다 `len(path)+len(role)+len(sha256)+32`(byte 수 20자리와 구분자 포함)를 합한 `fields`에 header 32를 더한다. logical은 기존 `3·fields + 128·n`에 가장 긴 줄의 4배(`fmt` 줄 buffer는 두 배씩 자란다)와 고정 256을 더했다(늘었다). Builder를 한 번에 확보(`Grow`)하며 member 경로는 파싱 시 대소문자 접은 뒤 유일하므로(`MEMBER_DUPLICATE`) 정렬은 전순서이고 text는 이전 정렬(`slices.SortFunc`)과 같다. `TestReplayMembersChargeCoversWork`가 경로 공통 접두·역순·이미 정렬·짧은 경로·혼합 길이·깊은 공유 디렉터리 입력에서 이전 정렬 text와의 동일성, 정렬 방문(비교·비교한 byte·이동) + 작성 byte ≤ work, 실제 할당 ≤ logical을 assert하며 정렬 상한 항을 한 단계분으로 줄인 mutant를 죽인다. 비 semantic `readDir`의 병합 buffer(항목 수 ≤ 한도 10,000, corpus 52,000, 항목당 16 B)는 계상 대상이 없는 경로(원장 없음)이고 같은 항목을 담는 `out` slice와 `DirEntry`도 계상되지 않으므로 계상하지 않았다.

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

## #370 V2-C3 accepted-event·preprocess-CST runtime opt-in

Logger 판독 권한은 accepted-event 또는 preprocess-CST 권한을 포함하지 않는다. 새 경로는 V2-C의 승인 tuple·83-file/3-patch reader 의무에 더해, 같은 외부 승인 pin 집합의 `native/runtime-readers.json`(`role=runtime-readers`)을 요구한다. 문서는 `schema=tsgk-runtime-readers/r1`, 정확한 `tuple_sha256`, 선택적인 `accepted_reader=tsgk-runtime-accepted/r1`과 `preprocess_cst_reader=tsgk-runtime-preprocess-cst/r1`을 갖는다. Unknown/duplicate/type·원문 digest 검사를 유지한다. 요청 범위 누락은 `PIN_RUNTIME_READER_MISSING`/BLOCKED, tuple 모순은 `PIN_RUNTIME_READER_BINDING`/FAIL, 미지원 schema/reader는 `PIN_RUNTIME_READER_UNSUPPORTED`다. 승인 자료가 reader의 검사 의무나 분모를 선택하지 않는다.

직접 `ReuseAcceptedRequest.RuntimePins`와 `PreprocessCSTRequest.RuntimePins`의 subject는 같은 요청의 **Raw 응답 원문 bytes 전체**다. source와 등록 edit에서 version/point를 다시 만들고 raw의 source/run/producer와 projection 또는 primary receipt·전체 CST·fresh equality를 대조한다. 공개 accepted 결과는 `tsgk-reuse-accepted/r2`, CST 결과는 `tsgk-preprocess-cst/r2`이며 선택 reader를 명시한 `runtime_pins`를 덧붙인다. 각 API는 기존 유한 replay 한도 안에서 승인 자료를 해석하고, nil 호출의 결과 schema·current manifest 검증을 유지한다.

Replay는 exact Profile 원문 subject를 사용한다. 새 `native-result-r11`은 기존 accepted/r8, `oracle-set-r8`은 accepted/r3, `native-result-r12`는 CST/r7의 판독 의미에 해당 reader 승인을 더한다. `RuntimePins` 없는 새 reducer 또는 기존 reducer에 승인 입력을 붙이면 거부한다. 결과는 `tsgk-replay-result/r6`이며 nested `RuntimeEvidenceResult.reader`로 범위를 기록한다. 저장 PASS reader는 승인·tuple identity·고정8개 자료와 reader 자료, 필수 proof gate의 PASS·consumption·graph subject 일관성을 검사한다. CST 사례는 유일한 ID와 원본 r1/PASS·빈 code·중첩 승인 부재·비어 있지 않은 step을 요구한다. 원본 incremental/Oracle/CST/accepted projection schema는 그대로다.

Qualification은 inventory/r8과 host의 exact run-identity subject를 유지한다. Reader 자료를 명시한 host가 있으면 `tsgk-qualification-result/r13`을 사용한다. 제출된 accepted/symbol workload마다 같은 host 원장에서 해당 scope를 확인하고 `QualSet.runtime_pins`에 실제 소비 범위를 남긴다. 검증된 private token만 내부 replay로 전달하고 다음 set/host로 권한을 유출하지 않는다. Candidate·cohort·platform·당시 NEW_RUN·등록 runtime commit·고정 original/target/control 분모는 유지한다. `QuerySemantics` 동시 입력과 이전 inventory admission은 지원하지 않는다. Standalone CLI admission은 V2-D에서 추가했다.

자료 결속 PASS는 실행 진위나 새 runtime의 실제 자격이 아니다. 미보존 raw/build/checkout·독립 provenance를 추측하지 않으며 실제 역사 자료가 없으면 BLOCKED다. 생산 pointer 변경은 동일 과거 승인·증거의 결과를 바꾸지 않는다.

## #370 V2-C2 일반 logger replay·qualification opt-in

`ReplayRequest.RuntimePins *EvidencePinsRequest`는 caller가 graph 밖에서 승인한 runtime logger 판독 입력이다. `Pins`(reference semantic용)와 구분한다. `RuntimePins.Subject`는 같은 호출의 **Replay Profile 원문 bytes 전체**여야 한다. 새 `native-result-r9`는 기존 r3의 선택적 logger, `native-result-r10`은 기존 r4의 mandatory logger 의미에 이 승인 경계를 추가한다. 결과는 `tsgk-replay-result/r5`다. 새 reducer에 입력이 없거나 기존 reducer에 RuntimePins를 붙이면 `REPLAY_RUNTIME_PINS_REDUCER_MISMATCH`로 거부한다. 기존 result/record/profile bytes와 schema는 재작성하지 않는다.

`QualifyHost.RuntimePins`는 해당 host의 **run-identity.json 원문 bytes 전체**에 결속한다. 독립 승인 digest 두 개, subject equality 뒤 V2-C tuple/manifest·driver/CP949/shim·83-file/3-patch 의무를 같은 원장으로 검사한다. Host identity의 candidate·checkout·platform·run cohort·NEW_RUN 검사는 그대로다. 이때 NEW_RUN은 subject 당시 실행이며 오늘 새 native 실행을 했다는 뜻이 아니다. 결과 `tsgk-qualification-result/r12`의 `runtime_pins` map은 명시 입력한 platform별 실제 소비 identity를 담는다. 현재 inventory/r8만 지원하며 `QuerySemantics`와 host RuntimePins 동시 입력은 `LOGGER_PINS_QUALIFICATION_SCOPE`로 거부한다. 각 host의 승인은 다른 host로 전달되지 않는다.

`RuntimeEvidenceResult`는 E0 Report, `pins`, `runtime_pin`, `complete`, `bytes_read`, `work`, `logical_bytes`를 기록한다. `pins`는 자료 원문 결속 결과이고 `runtime_pin`은 V2-C의 `QueryRuntimePinIdentity`다. 이 부분 결과 PASS는 승인 material과 고정 logger reader의 해석 선택을 뜻한다. Native build 전체나 실행 진위, accepted event·preprocess-CST를 검증했다는 뜻이 아니다. 전체 replay/qualification 판정은 기존 source/edit/CST/query/API·route·관측 검사를 추가로 통과해야 한다. 저장 replay r5 PASS는 profile subject와 nested pin PASS·완료, tuple/manifest identity, 고정8개 자료의 역할·양의 bytes·유일성, 관측/판정 gate와 소비 수를 요구한다. Descriptor와 counter를 함께 축소해도 허용하지 않는다.

일반 logger 판독은 기대 manifest를 승인 tuple에서만 얻는다. Producer 없는 host marker도 첫 marker나 current fallback으로 권한을 만들지 않는다. Projection·raw capability·각 observation과 mandatory claim은 동일 token에 결속하며, native producer의 language ABI는 tuple 범위 안이고 runtime ABI 상하한은 tuple과 정확히 같아야 한다. 기록 build/set runtime commit은 승인 manifest commit과 같아야 한다. r4 raw는 기존 strict nativewire header/step/end 검사를 같은 token으로 수행한다. Manifest·marker·raw·ABI·commit 혼합은 FAIL이다.

자료/tuple 누락은 BLOCKED이고 승인/실제 bytes 또는 subject 모순은 FAIL, 미지원 reader는 UNSUPPORTED다. 확정 FAIL과 부분 counter는 뒤의 취소·한도 오류에도 남는다. Resolve와 runtime 의미 검사는 기존 read/work/logical/wall 원장을 공유하며 새 한도를 만들거나 완화하지 않는다. Context는 검증 함수와 ordinary set 종료 때 복원된다. 기존 nil 호출의 schema·current 검증·오류 순서는 유지한다.

V2-C2 단계의 opt-in은 ordinary host set logger 판독과 native-result logger replay를 지원한다. Accepted·preprocess-CST의 별도 reader 전달은 V2-C3, standalone replay·qualification CLI admission과 Roslyn은 V2-D에서 추가했다. 실제 역사 build/raw/provenance가 없는 자료는 계속 BLOCKED이며 자료를 추측해 PASS로 만들지 않는다.
