# Identity와 evidence — draft/r0, manifest r2 구현

## 파일 identity

manifest r0와 그 hash preimage는 아래 역사적 설계로 보존한다. S01 착수 전 채택 설계는 `tsgk-manifest/r1`이며 `algorithm: sha256`, `mode_policy`, `files`를 가진다. 각 파일은 portable `path`, `role`, `mode`(`100644` 또는 `100755`), `mode_provenance`, `size`(bytes), 소문자 64자리 `sha256`으로 나타낸다. path의 UTF-8 bytes 오름차순으로 정렬하고 중복을 거부한다. 디렉터리·symlink·special file은 파일 항목이 아니다. 기본 `mode_policy=portable-default`, `mode_provenance=POLICY_DEFAULT`, mode `100644`는 관측된 executable bit가 아니다. 별도 filesystem mode는 실제 관측 가능할 때 출처와 다른 policy로 등록한다. API/offline 경로가 Git process를 실행해 mode를 추정하지 않는다.

기본 선택 집합은 [discovery](cli-and-profile.md)의 발견 파일이고 strict profile은 exact 목록을 소유한다. verify는 `--expected`의 역할·mode 정책·목록으로 root를 다시 읽고 누락/중복/변조/선택 집합 내 추가 파일을 거부한다. 선택 밖 파일은 비교하지 않으며(archive는 그 member 수를 `excluded`로 표시) 전체 저장소 무결성을 주장하지 않는다. 출처 신뢰는 호출자가 제공하는 expected anchor에서 오고, 같은 root에서 즉석 생성한 manifest로 자체 인증하지 않는다. S02 구현의 expected `tsgk-expected/r1`, 비교 범위와 difference code는 [CLI/profile](cli-and-profile.md)의 `S02 구현` 절이 소유한다. expected의 `set_sha256`은 아래 r2 preimage로 다시 계산해 문서 안 값과 같아야 하며, 결과는 `expected-set`과 actual `source-set` identity를 따로 기록한다.

파일 집합 hash의 preimage는 ASCII `tsgk-files/r0\n` 다음으로 각 항목의 `path\0role\0mode\0size\0sha256\n` UTF-8 bytes를 정렬 순서로 연결한 것이다. portable path와 role은 제어문자/NUL을 허용하지 않으며 size는 정규 10진수다. manifest 원문 bytes의 SHA-256도 별도로 보존한다. 의미가 같아도 raw JSON key order/공백이 바뀌면 raw hash는 달라진다. hash는 서명이나 게시자 인증이 아니다.

r1은 `tsgk-files/r1\nmode_policy\n`의 `mode_policy`를 실제 policy 값으로 대입하고, 각 record를 `path\0role\0mode\0mode_provenance\0size\0sha256\n`으로 결속한다. policy/provenance는 제어문자 없는 등록 값이다. r0와 r1을 서로 같은 identity로 취급하지 않는다. schema revision을 바꿔 source/정책 구분을 숨기지 않으며 S01의 독립 expected vector와 path/role/membership/mode/byte 변조 negative로 검증한다.

### manifest r2 — S01 구현

r1은 구현되지 않은 설계로 보존하고, S01은 판별 encoding을 identity에 결속한 `tsgk-manifest/r2`를 구현한다. manifest는 `schema`, `algorithm: sha256`, `mode_policy`, `encoding_policy`, `files`를 가진다. 파일 record는 r1 필드에 `encoding{assessment, encoding, source, code}`를 더한다. `encoding_policy`는 `detect-r1`이고 profile이 cp949를 선언하면 `detect-r1;profile=cp949`다. 파일별 선언은 해당 record의 `source=DECLARATION`으로 결속된다. 선언 path는 선택(identity) 또는 inventory(corpus) path와 bytes 그대로 일치해야 하며, 대소문자만 다른 선언을 포함해 일치하지 않는 선언은 `DECLARATION_UNMATCHED` 오류다.

집합 hash preimage는 ASCII `tsgk-files/r2\n` + `mode_policy\n` + `encoding_policy\n` 다음 path의 UTF-8 bytes 오름차순으로 `path\0role\0mode\0mode_provenance\0size\0sha256\0assessment\0encoding\0source\0code\n`을 연결한 것이다. 빈 encoding 필드는 빈 문자열이다. 같은 bytes라도 encoding policy나 선언이 다르면 집합 identity가 다르고, raw `sha256`/`size`는 원본 bytes 그대로다. CRLF·BOM·Unicode를 고쳐 쓰지 않는다. S01 시험은 외부(Python hashlib)에서 계산한 vector와 시험 안의 독립 preimage 구현으로 byte·path·role·membership 변조, CRLF 비정규화, 위치가 다른 두 root의 동일성을 대조한다. `portable-default` mode policy에서는 filesystem mode 변경이 identity를 바꾸지 않으며, mode 필드가 preimage에 들어가는 것은 별도 시험으로 확인한다.

encoding 판정은 [실사용 source 정책](../validation/net461-workload.md)의 1~6단계를 한 번의 streaming 읽기로 적용한다. `assessment`는 `PASS`(encoding이 결정되고 내용이 검증됨), `BLOCKED`, `UNRESOLVED`이고, `encoding`은 `UTF-8`/`UTF-16LE`/`UTF-16BE`/`CP949`, `source`는 `BOM`/`VALIDATION`/`DECLARATION`이다. code는 `UTF32_BOM`, `BOM_CONTENT_INVALID`, `UTF16_ODD_LENGTH`, `UTF16_UNPAIRED_SURROGATE`, `UTF16_NUL`, `NUL_WITHOUT_BOM`, `DECLARED_ENCODING_INVALID`, `UNDETERMINED_ENCODING`, `AMBIGUOUS_ENCODING`, `CP949_UNMAPPED`다(`ENCODING_TABLE_REQUIRED`는 S01~S04 build의 값이며 expected 문서에서만 받는다). 파일별 선언은 4~5단계만 대신하며 1~3단계보다 우선하지 않는다. S05부터 cp949는 고정한 WHATWG `index-euc-kr`로 판정한다. profile이 cp949를 선언했고 엄격 UTF-8인 비ASCII 파일의 모든 2-byte 쌍이 표에 있으면 `BLOCKED`/`AMBIGUOUS_ENCODING`, UTF-8이 아니고 구조는 맞지만 표에 없는 쌍(Windows 전용 확장 등)이 있으면 `BLOCKED`/`CP949_UNMAPPED`, 파일별 cp949 선언에서 그런 쌍은 `DECLARED_ENCODING_INVALID`다. 표 없이 증명할 수 있는 구조 위반(WHATWG euc-kr 형태: ASCII, 또는 lead 0x81~0xFE 뒤 trail 0x41~0xFE)만 cp949 후보에서 제외한다. 이것은 추정이 아니라 필요조건 검사다. UTF-16 사전 검증(짝수 길이, 짝 맞는 surrogate, U+0000 부재)에 실패한 파일은 parse 입력을 만들기 전에 `BLOCKED`이며 raw hash/size는 그대로 기록한다. identity/inspect의 report-level assessment는 관측이므로 `NOT_ASSESSED`이고, 파일별 결과는 record와 그 record의 `code`와 같은 finding code(예: `ENCODING_TABLE_REQUIRED`, `UTF16_UNPAIRED_SURROGATE`)로 남는다.

report의 `identities`는 `source-set`(`tsgk-files/r2`, 집합 hash)과 `policy`(`tsgk-policy/r1`)를 담는다. policy preimage는 operation, discovery, 모든 한도, 대용량 예외 id, encoding policy, 제외 규칙을 고정 순서 줄로 쓴 text이며 결과의 `policy` 객체로도 공개한다. 한도를 바꾸면 다른 policy identity다.

### 비공개 corpus record — `private-corpus-local`

`kit.Corpus`/`tsgk corpus`는 corpus root 아래 제외 디렉터리 밖의 모든 파일을 record 하나씩으로 기록한다. 제외 디렉터리 이름(`bin`, `obj`, `.vs`, `packages`, `TestResults`, `.git`, `.svn`, `.hg`)은 대소문자를 구분하지 않고 비교하며, 읽지 않고 잘라내고 수에 넣지 않는다. record는 `path`, `role`, `route`, `state`, `size`, `sha256`, `encoding`, `newline`, `size_class`, `generated`, `generated_basis`, `duplicate_count`, `declared_by`를 가진다.

* `state`: `COMPLETED`, `PRESENCE_ONLY`, `RESOURCE_LIMIT`(그 파일만 단일 파일 한도 초과, 읽지 않음), `UNSUPPORTED`(link·reparse point·special·hard link·portable하지 않은 이름, 따라가거나 내용을 hash하지 않음). 이런 항목은 그 record만 `UNSUPPORTED`이고 실행 전체를 끝내지 않는다.
* `PRESENCE_ONLY`: 자격 증명 성격 확장자(`.pfx`, `.p12`, `.snk`, `.key`, `.pem`, `.jks`, `.keystore`, `.pvk`)와 vendor binary(`.dll`, `.exe`, `.pdb`)는 대소문자를 구분하지 않고 판정해 `lstat`의 존재와 size만 기록한다. 파일을 열지 않으므로 `sha256`·`encoding`·`newline`이 없다. 파일 수와 record에는 넣고 bytes와 단일 파일 한도에는 넣지 않는다.
* `route`: 확장자 표(`.cs` csharp, `.sql` tsql, `.svc` svc, `.config`/`.resx`/`.xsd`/`.wsdl`/`.xml`/`.settings`/`.datasource` xml)로 정하며 표에 없으면 빈 값(unrouted)이다. route는 역할과 독립이다.
* `role`: 파일마다 N461 역할 하나 또는 `UNCLASSIFIED`다. 위에서부터 처음 맞는 규칙을 쓴다. 내용 표지는 원본 bytes에서 찾는 ASCII 문자열이며 UTF-16 파일에서는 찾지 못할 수 있다.
  1. `.svc` → `N461-SVC`
  2. `.config`, `.wsdl`, `.xsd` → `N461-XML`
  3. `.resx` → `N461-RESOURCE`
  4. `.cs`가 아닌 나머지 → `UNCLASSIFIED`
  5. 이름이 `.svc.cs`로 끝나거나 `Reference.cs`이거나, 표지 `ServiceContract`·`OperationContract`·`ClientBase<`가 있으면 → `N461-WCF-CS`
  6. `.Designer.cs`이고 표지 `System.Resources.ResourceManager`가 있으면 → `N461-RESOURCE`
  7. `.Designer.cs`이거나 표지 `InitializeComponent`가 있으면 → `N461-FORM`
  8. 표지 `DevExpress.`가 있으면 → `N461-DX202`
  9. 그 밖 `.cs` → `UNCLASSIFIED`
* `generated`: 이름이 `.Designer.cs`·`Reference.cs`이면 `NAME`, 표지 `<auto-generated`가 있으면 `MARKER`다. 자동 생성 파일도 제외하지 않는다.
* `newline`: `NONE`, `LF`, `CRLF`, `CR`, `MIXED`이며 UTF-16 BOM 파일은 code unit으로 센다. UTF-32 BOM·BOM 없는 NUL 파일은 `UNKNOWN`이다. `size_class`는 `LE_64KIB`, `LE_1MIB`, `LE_4MIB`, `LE_16MIB`, `LE_32MIB`, `OVER_32MIB`다.
* `duplicate_count`: 같은 `sha256`을 가진 `COMPLETED` record가 2개 이상이면 그 수다.
* `.csproj` 선언 포함 관계: MSBuild를 실행하지 않고 `ItemGroup` 아래 `Compile`, `Content`, `None`, `EmbeddedResource`, `Page`, `Resource`, `ApplicationDefinition`, `EntityDeploy`, `ProjectReference`의 `Include`만 관측한다(`;` 분리). 디렉터리를 가리키는 item(`Folder`, `WCFMetadataStorage` 등)과 다른 item 종류는 `ignored_items` 수로만 센다. item 또는 상위 요소의 `Condition`, `Choose`(`When`/`Otherwise`)와 `Target` 아래 item, `Exclude`가 있는 item, `$(`/`@(`/`%(`, wildcard(`*`, `?`)는 평가가 필요하므로 `UNRESOLVED`다. 나머지는 `%xx` decode → `\`를 `/`로 → `.csproj` 디렉터리 기준 lexical `..` 해석 → corpus root 확인 순서로 처리한다. decode 결과에 drive(`:`)·UNC·절대 형태가 생기거나 root 밖이면 `NOT_FOUND`다. root 안 inventory 경로와 bytes 그대로 일치하거나, 그런 경로가 없고 대소문자를 무시한 일치가 하나뿐이면 `MEMBER`이고(대소문자만 다른 후보가 여럿이면 `UNRESOLVED`) 해당 record의 `declared_by`에 project 경로를 넣는다. 제외 디렉터리 안이면 `EXCLUDED`, 그 밖은 `NOT_FOUND`이며 어느 경우에도 대상을 새로 읽지 않는다. `Import` 수는 project record에 남긴다. XML은 데이터이고 외부 entity·schema를 가져오지 않으며 UTF-8/ASCII 선언 외 encoding의 project는 `UNPARSED`다.
* 한도와 집계: [NET461 등록부](../validation/net461-workload.md)의 파일 26000, record 26000, 단일 파일 33554432 bytes, 읽은 합계 3489660928 bytes, 깊이 32(경로 segment 수), 보고 67108864 bytes, S01 wall 1800초다. 단일 파일 초과는 그 record만 `RESOURCE_LIMIT`이고 나머지 한도 초과는 실행 전체 `RESOURCE_LIMIT`이며 record를 반환하지 않는다. 디렉터리 listing은 파일 한도의 2배 항목으로 따로 제한한다. `pg-large-source-r1`은 이 연산에 적용하지 않는다(요청에 그 필드가 없다). Go process memory 2147483648 bytes는 sampled non-strict 한도로 S01에서 강제하지 않는다.
* 공개 투영: `summary`는 개수와 판정만 담는다. `records`·`projects`는 경로·이름·hash를 담으므로 추적하지 않는 로컬 artifacts에만 둔다.

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

## S07 구현 — replay와 evidence graph

S07은 E0를 확장하고 새 report 체계를 만들지 않는다. `tsgk-replay-result/r1`과 `tsgk-evidence-result/r1`은 E0 envelope(`schema: tsgk-report/r1`, 세 축, `identities`, `findings`, `coverage`)에 필드를 더한 것이다. 명령·registration·한도는 [CLI/profile](cli-and-profile.md) `S07 구현`이 소유한다.

### replay: 세 축과 subject

* replay 연산 자체의 축과 과거 subject run을 따로 둔다. 결과의 `subject`는 registration이 적은 run·attempt·platform·commit과 그 원 축이며 replay가 바꾸지 않는다. `recorded`는 raw에 기록된 subject 판정이고 registration의 원 판정과 다르면 `RECORDED_VERDICT_MISMATCH`다. `recomputed`는 reducer가 raw에서 다시 계산한 subject 판정이다.
* gate마다 `recomputed`·`not_recomputed` 수를 센다. 모든 record를 다시 계산한 gate만 `evidence_mode: REPLAYED_RAW`이고, 하나라도 다시 계산하지 못한 gate는 `RECORDED_NOT_RECOMPUTED`이며 실패가 없으면 판정은 `UNRESOLVED`다. 다시 계산하지 못한 claim에 기대는 사례는 다시 계산한 claim이 FAIL이 아닌 한 `UNRESOLVED`이고 PASS가 되지 않는다.
* 결과 `assessment`는 evidence가 유효하면(`evidence_valid`: finding 없음, gate 실패 없음) 다시 계산한 subject 판정이고, 유효하지 않으면 `FAIL`이다. 다시 계산한 gate가 하나도 없으면 결과 `evidence_mode`는 `RECORDED_NOT_RECOMPUTED`다. 등록되지 않은 reducer·schema·연산은 `UNSUPPORTED`(NOT_RUN/NOT_RUN/BLOCKED)이고 replay 성공이 되지 않는다.
* member: registration의 `members`가 독립 inventory다. root의 모든 파일은 member이거나 reducer가 결속한 inventory의 항목이어야 하며(`MEMBER_UNLISTED`), member는 등록한 bytes·sha256과 같아야 한다(`MEMBER_MISSING`, `MEMBER_MISMATCH`). 같은 bundle이 만든 manifest(S06 `manifest.json`, PREPARE `evidence-manifest.json`)는 무결성 metadata이며 registration이 그 bytes를 결속할 때만 그 목록을 쓴다.
* identity: reducer가 raw에서 관측한 identity(`observed_identities`)와 registration의 `identities`를 역할마다 정확히 비교한다. 다르면 `IDENTITY_MISMATCH`(stale·혼합), 결속하지 않은 관측 역할은 `IDENTITY_UNBOUND`, 관측하지 않는 역할은 `IDENTITY_UNKNOWN`이다. tree마다 붙은 producer·policy·source identity가 run의 값과 다르면 `MIXED_IDENTITY`다. summary·manifest만 고쳐 hash를 다시 맞춰도 reducer가 record에서 다시 센 값과 달라 거부된다.
* 소비: reducer는 workload(또는 registration의 `records`)에서 expected record와 순서를 정하고 record마다 정확히 한 번 소비한다. `consumption`은 `expected`, `consumed`, `missing`, `duplicate`, `unused`, `out_of_order`(뒤에 등록된 record 다음에 나온 record), `excluded`(다른 cohort, 소비하지 않음)와 첫 문제를 적는다. 누락·중복·미사용·순서 오류가 있으면 `CONSUMPTION_INCOMPLETE`다.

### 등록 reducer

reducer는 code의 작은 등록부이며 plugin 언어가 아니다(`kit.Reducers`). 각 reducer는 다시 계산하는 gate와 다시 계산하지 않는 기록을 선언한다.

| reducer | 입력 | 다시 계산하는 gate | 기록으로 남는 것 |
|---|---|---|---|
| `native-result-r1` | S05 `tsgk-incremental-result/r1`, 그 workload profile, `responses/` | case 결속(id·입력·step 수), 상태-판정 일관성, full tree 구조·`tsgk-tree-digest/r1`·node 수·`has_error`, incremental/fresh 비교(`CompareTrees`), route 증명, 기대값(workload 등록에서), claim과 판정 fold, summary 수와 run 판정 | driver 응답 payload(protocol decode는 공개 API 밖), r1 full tree의 declarations 기대값, record·summary 형식 tree의 digest |
| `oracle-set-r1` | S06 기록 set(`manifest.json`, `records/`, `raw/`)과 workload profile | 위 S05 gate, record 완결성(S06 기록 set 검사), set 사례 목록·record 수·set 판정, incremental/fresh query 비교(`CompareCaptures`) | API claim(관측은 raw 응답에만), query 기대값·사실 재현·동적 SQL(사례 source bytes 필요), UNSUPPORTED query의 runtime 구조 stream |
| `private-corpus-r1` | `NET461-PHASE2-LOCAL-r1` 로컬 실행: S01 inventory, route profile·결과, 파일별 projection(jsonl), 실행 summary | 아래 비공개 workload 등록 | record 형식 tree의 digest |
| `prepare-native-r1` | PREPARE probe bundle: `evidence-manifest.json`, `records/case-ledger.json`, `raw/case-*.stdout` | bundle inventory, 한 producer의 등록 row 전수, raw stdout 결속, probe 판정(원본 오류, incremental/fresh 출력 동일성, 손상 오류·복구 무오류, 복구=원본)으로 다시 계산한 exit, 기대 syntax 종류 | 등록 fact check와 edit 창(PREPARE 리뷰 helper, 이식하지 않음) |
| `bs-gate-compare-r1` | BrightScript 기록 gate 문서와 다시 계산한 gate 문서 | `S07-REPLAY-2ULP-r1` 비교 | raw에서 gate를 다시 계산하는 일(보관된 Python verifier, 별도 EXEC_ADAPTER) |

`native-result-r1`·`oracle-set-r1`·`private-corpus-r1`은 같은 사례 reducer와 S05·S06의 공개 비교 함수를 쓴다. 새 JSON normalizer는 없고 host 관측(process, 시간, executable)은 raw에 남되 판정에 쓰지 않는다. 같은 reducer와 같은 bytes는 같은 결과다.

### BrightScript 수치 정책 `S07-REPLAY-2ULP-r1`

BrightScript v0.1.2 historical replay의 비교 규칙이며 `bs-gate-compare-r1`에만 속한다. kit 전역 tolerance가 아니다. `kit.CompareGates`는 보관된 `replay_compare.py`를 옮긴 것이다. key 집합·배열 길이와 순서·ID·bool/int/float/string/null 형(정수와 실수 literal은 다른 형)·verdict·원 측정·threshold는 정확히 같아야 한다. gate별 허용 목록의 log 파생 exponent만, 두 값이 같은 부호의 유한 normal이면 2 ULP까지 허용한다. 0과 subnormal은 bit까지 같아야 하므로 `+0`/`-0`도 다르다. 어느 값이든 ±2 ULP 구간이 원 threshold를 걸치면 `NUMERIC_BOUNDARY_INDETERMINATE`, threshold 판정이 다르면 `THRESHOLD_DECISION_DIFFERENCE`, `REGRESSION-SWEEP`의 집계가 exponent 최대값과 다르면 `AGGREGATE_VALUE_DIFFERENCE`, 최대값을 낸 항목이 바뀌면 `AGGREGATE_SOURCE_CHANGED`다. 허용된 차이는 `REPLAY_EQUIVALENT_WITH_DECLARED_ROUNDING`으로 경로·ULP 거리·threshold와 함께 남고 `BITWISE_EQUAL`과 구별한다. v0.1.2 verification bundle은 이 저장소에 없으며 S07은 그것을 내려받지 않았다(이번 다운로드 0). 17개 gate의 raw 재계산은 보관된 verifier 실행이 필요하므로 지원하지 않는다.

### 비공개 corpus workload 등록과 로컬 실행 identity

`private-corpus-r1`은 [NET461 등록부](../validation/net461-workload.md)의 corpus route 표를 workload로 등록한다. inventory 순서대로 route가 있고 `PRESENCE_ONLY`가 아닌 record가 파일 record(`f` + inventory 순번 6자리)다. 그중 `COMPLETED`·encoding `PASS`·portable path인 record만 route 묶음(`csharp`, `tsql`, `xml`, `svc`; svc는 csharp route로 파싱)의 사례이고 나머지는 `BLOCKED`(그 code)로 실행하지 않는다. 각 route profile은 그 묶음을 같은 순서·입력 identity·encoding으로 정확히 담아야 하고(`corpus-binding`), 결과는 위 사례 reducer로 다시 판정한다. 파일별 projection은 모든 파일 record를 한 번씩 소비하고 다시 계산한 상태·판정·code·`has_error`·node 수·digest·형식과 같아야 한다. summary의 inventory 수·route별 상태·`has_error`·code·형식·`not_run`은 다시 센 값과 같아야 한다.

로컬 실행 identity는 후보 commit(40자리 hex), clean tree(`clean_tree: true`, dirty 0), host 정보(OS·arch), 도구 identity(compiler sha256, runtime commit; 각 route 결과의 build 값과 같아야 함)다. 모두 `local-run-identity` gate와 결속 identity(`commit`, `clean_tree`, `host`, `compiler`, `runtime`, route별 `workload`·`policy`·`producer`·`executable`·`operation`·`platform`)로 검사한다. 이 reducer의 결과는 경로·이름을 담지 않는다(record는 순번으로만 나타낸다). 그래도 registration과 결과는 hash와 로컬 실행 정보를 담으므로 추적하지 않는 로컬 artifacts에만 두고, 공개 기록은 개수와 판정만 담는다.

### evidence graph `tsgk-evidence/r1`

`kit.VerifyEvidence`는 caller 신뢰 policy가 anchor한 `evidence.json`과 그 파일로 graph를 검사한다. node 종류와 축은 `run`(NEW_RUN), `replay`(REPLAYED_RAW), `carry`(CARRIED_FORWARD, 현재 실행 NOT_RUN), `record`(RECORDED_NOT_RECOMPUTED, 현재 판정 UNRESOLVED와 `recorded_assessment`)다. 완료되지 않은 실행의 PASS는 없다. identity는 `source`, `tool`, `input`, `query`, `policy`, `comparator`, `protocol`, `workload`, `platform`, `run`, `attempt`를 모두 별도 필드로 가진다.

* 참조는 `subject`(replay → run·record, 정확히 하나), `carries`(carry → run, 정확히 하나), `supersedes`(나중 결과 → 이전 결과)만 허용하고 순환은 `GRAPH_CYCLE`이다. 파일은 graph가 나열한 것뿐이며 bytes·sha256이 같아야 한다.
* replay는 subject와 다른 run·attempt를 가져야 하고(`REPLAY_RELABELS_SUBJECT`) source·input·workload·platform은 subject와 같아야 한다(`REPLAY_SUBJECT_MISMATCH`). `replay-result` 파일이 있으면 그 evidence mode·판정·subject run이 graph와 같아야 한다.
* 이전 FAIL과 수정·새 policy의 PASS는 서로 다른 run identity의 두 node로 남는다. 같은 run·attempt·platform의 run node 둘은 `RUN_IDENTITY_DUPLICATE`, 같은 run identity로 이전 결과를 대체하면 `RUN_RELABELED`다.
* 승계는 기본 거부다. 채택한 관계는 `unchanged-dependency-r1` 하나뿐이다. policy의 `carry_forward` 규칙이 그 node·원 결과·관계·허가자를 적어야 하고(없으면 `CARRY_FORWARD_UNAUTHORIZED`, 다르면 `CARRY_RELATION_UNSUPPORTED`), run·attempt를 뺀 모든 dependency identity(source·tool·input·query·policy·comparator·protocol·workload·platform)가 원 결과와 같아야 한다(`CARRY_DEPENDENCY_CHANGED`). 원 결과는 완료된 NEW_RUN이고(`CARRY_ORIGIN_INCOMPLETE`) 판정은 바뀌지 않는다(`CARRY_ASSESSMENT_CHANGED`).
* `required` node는 종류와 적힌 identity가 정확히 같아야 한다(`IDENTITY_MISMATCH`). `eligibility`가 있으면 그 node의 evidence mode가 목록에 있어야 하며, S08 최종 qualification처럼 `NEW_RUN`만 받는 자격에 replay·carry·record node는 무결성이 맞아도 `ELIGIBILITY_REJECTED`다.
* hash 일치는 무결성이다. 게시자 인증(authenticity)은 지원하지 않으며 결과 coverage에 `unsupported: authenticity`로 남는다.

## 준비 tracking의 적용 대상과 보존 결과

PREPARE tracking receipt의 객체별 S00 보존은 `s00_object_preservation.applicable`과 `status`를 구별한다. 대상은 `issue:2`와 `milestone:1`이며 실제 원본 body/description·title·state·관계 대조 결과를 `PRESERVED` 또는 `MISMATCH`로 기록한다. 다른 객체의 상태는 `NOT_APPLICABLE`이다. 별도 `tracking_readback`은 intended object와 실제 readback의 `MATCH`/`MISMATCH`를 기록한다. Parent #1은 S00 객체가 아니므로 그 안의 S00 완료 이력 section을 별도로 대조한다.

과거 prepare-02의 `s00_preserved`는 generator가 S00 객체 predicate를 직렬화한 필드다. non-S00의 false는 실제 보존 실패 판정이 아니며 모두 true로 교체하지 않는다. immutable 원본·generator·관측 시각을 보존하고 새 checkpoint에서 의미와 실제 대조 근거를 연결한다. 수정된 receipt만으로 과거 미실행 검증을 소급 PASS로 만들지 않는다.
