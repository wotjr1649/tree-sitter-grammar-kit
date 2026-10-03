# Identity와 evidence — draft/r0, manifest r2 구현

## 파일 identity

manifest r0와 그 hash preimage는 아래 역사적 설계로 보존한다. S01 착수 전 채택 설계는 `tsgk-manifest/r1`이며 `algorithm: sha256`, `mode_policy`, `files`를 가진다. 각 파일은 portable `path`, `role`, `mode`(`100644` 또는 `100755`), `mode_provenance`, `size`(bytes), 소문자 64자리 `sha256`으로 나타낸다. path의 UTF-8 bytes 오름차순으로 정렬하고 중복을 거부한다. 디렉터리·symlink·special file은 파일 항목이 아니다. 기본 `mode_policy=portable-default`, `mode_provenance=POLICY_DEFAULT`, mode `100644`는 관측된 executable bit가 아니다. 별도 filesystem mode는 실제 관측 가능할 때 출처와 다른 policy로 등록한다. API/offline 경로가 Git process를 실행해 mode를 추정하지 않는다.

기본 선택 집합은 [discovery](cli-and-profile.md)의 발견 파일이고 strict profile은 exact 목록을 소유한다. verify는 `--expected`의 역할·mode 정책·목록으로 root를 다시 읽고 누락/중복/변조/선택 집합 내 추가 파일을 거부한다. 선택 밖 파일은 report에 excluded로 표시하며 전체 저장소 무결성을 주장하지 않는다. 출처 신뢰는 호출자가 제공하는 expected anchor에서 오고, 같은 root에서 즉석 생성한 manifest로 자체 인증하지 않는다.

파일 집합 hash의 preimage는 ASCII `tsgk-files/r0\n` 다음으로 각 항목의 `path\0role\0mode\0size\0sha256\n` UTF-8 bytes를 정렬 순서로 연결한 것이다. portable path와 role은 제어문자/NUL을 허용하지 않으며 size는 정규 10진수다. manifest 원문 bytes의 SHA-256도 별도로 보존한다. 의미가 같아도 raw JSON key order/공백이 바뀌면 raw hash는 달라진다. hash는 서명이나 게시자 인증이 아니다.

r1은 `tsgk-files/r1\nmode_policy\n`의 `mode_policy`를 실제 policy 값으로 대입하고, 각 record를 `path\0role\0mode\0mode_provenance\0size\0sha256\n`으로 결속한다. policy/provenance는 제어문자 없는 등록 값이다. r0와 r1을 서로 같은 identity로 취급하지 않는다. schema revision을 바꿔 source/정책 구분을 숨기지 않으며 S01의 독립 expected vector와 path/role/membership/mode/byte 변조 negative로 검증한다.

### manifest r2 — S01 구현

r1은 구현되지 않은 설계로 보존하고, S01은 판별 encoding을 identity에 결속한 `tsgk-manifest/r2`를 구현한다. manifest는 `schema`, `algorithm: sha256`, `mode_policy`, `encoding_policy`, `files`를 가진다. 파일 record는 r1 필드에 `encoding{assessment, encoding, source, code}`를 더한다. `encoding_policy`는 `detect-r1`이고 profile이 cp949를 선언하면 `detect-r1;profile=cp949`다. 파일별 선언은 해당 record의 `source=DECLARATION`으로 결속된다. 선언 path는 선택(identity) 또는 inventory(corpus) path와 bytes 그대로 일치해야 하며, 대소문자만 다른 선언을 포함해 일치하지 않는 선언은 `DECLARATION_UNMATCHED` 오류다.

집합 hash preimage는 ASCII `tsgk-files/r2\n` + `mode_policy\n` + `encoding_policy\n` 다음 path의 UTF-8 bytes 오름차순으로 `path\0role\0mode\0mode_provenance\0size\0sha256\0assessment\0encoding\0source\0code\n`을 연결한 것이다. 빈 encoding 필드는 빈 문자열이다. 같은 bytes라도 encoding policy나 선언이 다르면 집합 identity가 다르고, raw `sha256`/`size`는 원본 bytes 그대로다. CRLF·BOM·Unicode를 고쳐 쓰지 않는다. S01 시험은 외부(Python hashlib)에서 계산한 vector와 시험 안의 독립 preimage 구현으로 byte·path·role·membership 변조, CRLF 비정규화, 위치가 다른 두 root의 동일성을 대조한다. `portable-default` mode policy에서는 filesystem mode 변경이 identity를 바꾸지 않으며, mode 필드가 preimage에 들어가는 것은 별도 시험으로 확인한다.

encoding 판정은 [실사용 source 정책](../validation/net461-workload.md)의 1~6단계를 한 번의 streaming 읽기로 적용한다. `assessment`는 `PASS`(encoding이 결정되고 내용이 검증됨), `BLOCKED`, `UNRESOLVED`이고, `encoding`은 `UTF-8`/`UTF-16LE`/`UTF-16BE`/`CP949`, `source`는 `BOM`/`VALIDATION`/`DECLARATION`이다. code는 `UTF32_BOM`, `BOM_CONTENT_INVALID`, `UTF16_ODD_LENGTH`, `UTF16_UNPAIRED_SURROGATE`, `UTF16_NUL`, `NUL_WITHOUT_BOM`, `DECLARED_ENCODING_INVALID`, `UNDETERMINED_ENCODING`, `ENCODING_TABLE_REQUIRED`다. 파일별 선언은 4~5단계만 대신하며 1~3단계보다 우선하지 않는다. cp949 표가 필요한 판정(AMBIGUOUS 포함)은 S05까지 `UNRESOLVED`/`ENCODING_TABLE_REQUIRED`다. 표 없이 증명할 수 있는 구조 위반(WHATWG euc-kr 형태: ASCII, 또는 lead 0x81~0xFE 뒤 trail 0x41~0xFE)만 cp949 후보에서 제외한다. 이것은 추정이 아니라 필요조건 검사다. UTF-16 사전 검증(짝수 길이, 짝 맞는 surrogate, U+0000 부재)에 실패한 파일은 parse 입력을 만들기 전에 `BLOCKED`이며 raw hash/size는 그대로 기록한다. identity/inspect의 report-level assessment는 관측이므로 `NOT_ASSESSED`이고, 파일별 결과는 record와 그 record의 `code`와 같은 finding code(예: `ENCODING_TABLE_REQUIRED`, `UTF16_UNPAIRED_SURROGATE`)로 남는다.

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
* `.csproj` 선언 포함 관계: MSBuild를 실행하지 않고 `ItemGroup` 아래 `Compile`, `Content`, `None`, `EmbeddedResource`, `Page`, `Resource`, `ApplicationDefinition`, `EntityDeploy`, `ProjectReference`의 `Include`만 관측한다(`;` 분리). 디렉터리를 가리키는 item(`Folder`, `WCFMetadataStorage` 등)과 다른 item 종류는 `ignored_items` 수로만 센다. item 또는 상위 요소의 `Condition`, `Choose`(`When`/`Otherwise`)와 `Target` 아래 item, `Exclude`가 있는 item, `$(`/`@(`/`%(`, wildcard(`*`, `?`)는 평가가 필요하므로 `UNRESOLVED`다. 나머지는 `%xx` decode → `\`를 `/`로 → `.csproj` 디렉터리 기준 lexical `..` 해석 → corpus root 확인 순서로 처리한다. decode 결과에 drive(`:`)·UNC·절대 형태가 생기거나 root 밖이면 `NOT_FOUND`다. root 안 inventory 경로와 대소문자를 구분하지 않고 일치하면 `MEMBER`이고 해당 record의 `declared_by`에 project 경로를 넣는다. 제외 디렉터리 안이면 `EXCLUDED`, 그 밖은 `NOT_FOUND`이며 어느 경우에도 대상을 새로 읽지 않는다. `Import` 수는 project record에 남긴다. XML은 데이터이고 외부 entity·schema를 가져오지 않으며 UTF-8/ASCII 선언 외 encoding의 project는 `UNPARSED`다.
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

## 준비 tracking의 적용 대상과 보존 결과

PREPARE tracking receipt의 객체별 S00 보존은 `s00_object_preservation.applicable`과 `status`를 구별한다. 대상은 `issue:2`와 `milestone:1`이며 실제 원본 body/description·title·state·관계 대조 결과를 `PRESERVED` 또는 `MISMATCH`로 기록한다. 다른 객체의 상태는 `NOT_APPLICABLE`이다. 별도 `tracking_readback`은 intended object와 실제 readback의 `MATCH`/`MISMATCH`를 기록한다. Parent #1은 S00 객체가 아니므로 그 안의 S00 완료 이력 section을 별도로 대조한다.

과거 prepare-02의 `s00_preserved`는 generator가 S00 객체 predicate를 직렬화한 필드다. non-S00의 false는 실제 보존 실패 판정이 아니며 모두 true로 교체하지 않는다. immutable 원본·generator·관측 시각을 보존하고 새 checkpoint에서 의미와 실제 대조 근거를 연결한다. 수정된 receipt만으로 과거 미실행 검증을 소급 PASS로 만들지 않는다.
