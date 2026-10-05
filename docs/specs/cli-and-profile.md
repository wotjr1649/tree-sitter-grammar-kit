# CLI/report r1과 profile r0 — S01 구현과 후속 계약

S01은 `inspect`, `identity`, `corpus` 명령을 구현했고(아래 `S01 구현` 절), S02는 `verify`와 strict profile `tsgk-profile/r1`, expected `tsgk-expected/r1`을 구현했다(아래 `S02 구현` 절). S04는 `reproduce`와 `tsgk-reproduce/r1`을, S05는 `incremental`과 `tsgk-incremental/r1`을, S06은 `oracle record`와 `tsgk-oracle/r1`을(C2는 둘을 기대값 anchor가 있는 r2로 바꿨다), S07은 `replay`·`evidence verify`와 `tsgk-replay/r1`·`tsgk-evidence-policy/r1`을, S08은 `qualify`와 `tsgk-qualification-inventory/r1`을 구현했고 C2는 이를 r2를 거쳐 r3으로 바꿨다(아래 `S04 구현`~`S08 구현` 절). S00 계약의 `parity`는 S08이 `qualify`로 대체했다. 아래 profile r0 서술은 구현되지 않은 초안으로 보존하며 `S02 구현` 절과 충돌하면 그 절이 우선한다.

## 공통 입출력과 오류

root에서 실행할 CLI는 `tsgk <command>`다. 읽기 대상은 명시적 `--root PATH`(기본 현재 디렉터리), JSON 결과는 stdout, 진단은 stderr다. `--out PATH`는 새 파일만 허용한다. 기존 파일·입력 집합 내부·link 경로는 거부한다. caller-owned 같은 filesystem의 임시 파일을 완성한 뒤 충돌 없이 publish하고, 실패 partial은 별도 failure evidence로 보존한다. 결과 쓰기 실패는 성공 결과로 바꾸지 않는다.

exit `0`은 해당 명령이 요청한 범위의 PASS/관측 완료, `1`은 유효한 입력에서 검증 FAIL, `2`는 인자·profile·형식 오류, `3`은 필수 근거/권한/capability가 없어 BLOCKED, `4`는 I/O·tool·내부 실행 오류, `130`은 취소다. unknown layout을 관측한 inspect의 exit 0은 qualification PASS를 뜻하지 않는다. 결과 envelope는 `schema: tsgk-report/r1`, `command`, `execution_status`, `evidence_mode`, `assessment`, `identities`, `findings`, `coverage`를 가진다. finding은 안정적인 `code`, `severity`, root-relative `path`, 한국어 `message`로 표현한다. OS 절대 경로와 환경값은 기본 결과에서 제외한다.

S01부터 [공개 offline API](public-go-api.md)와 같은 operation/guard/E0를 사용한다. E0 r1의 추가 필드와 축은 [identity/evidence](identity-and-evidence.md)가 소유한다. resource limit은 `execution_status=RESOURCE_LIMIT`, `assessment=BLOCKED`, exit 3이다. 인자/형식 오류는 실행 전 exit 2, 시작 후 취소는 130, I/O/tool/protocol/publication 실패는 4, 완료된 검증 불일치는 1의 순서로 실제 원인을 기록한다. 여러 원인이 생기면 모두 보존하고 성공 publication 실패를 earlier PASS로 덮지 않는다. stdout의 partial stream은 rollback할 수 없으며 완전한 최종 report가 아니면 성공으로 소비하지 않는다.

| 명령 | 입력·출력 | capability / 소유 Session |
|---|---|---|
| `inspect --root PATH` | zero-config inventory, metadata/scanner/query/corpus 발견·미확인 사실 | READ_DATA, 01 |
| `identity --root PATH [--profile FILE]` | 선택 파일 집합의 manifest r2 | READ_DATA, 01; profile은 02 |
| `corpus --root PATH` | `private-corpus-local` inventory(역할·PRESENCE_ONLY·encoding·`.csproj` 선언 관측) | READ_DATA, 01 |
| `verify (--root PATH \| --archive FILE) --expected FILE [--profile FILE]` | 외부 신뢰 expected와 exact set 대조, ZIP은 풀지 않고 검사 | READ_DATA, 02 |
| `schema check --input FILE` | node-types 구조·참조 검증 | READ_DATA, 03 |
| `schema diff --before FILE --after FILE` | node/field/type/required/multiple/supertype 변경 | READ_DATA, 03 |
| `reproduce --root PATH --profile FILE --out DIR --work DIR --tool NAME=PATH --allow EXEC_GENERATOR` | 두 독립 생성과 기준 생성물 비교 | EXEC_GENERATOR + WRITE_RESULT, 04 |
| `incremental --root PATH --profile FILE --runtime DIR --tool cc=PATH --work DIR --out DIR --allow BUILD_NATIVE --allow EXEC_NATIVE` | 매 edit의 incremental/fresh 결과(edit는 profile 안) | BUILD_NATIVE + EXEC_NATIVE + WRITE_RESULT, 05 |
| `oracle record --root PATH --profile FILE [--fact-pack FILE] --runtime DIR --tool cc=PATH --work DIR --out DIR --allow BUILD_NATIVE --allow EXEC_NATIVE` | native ordered tree/query/API record set | BUILD_NATIVE + EXEC_NATIVE + WRITE_RESULT, 06 |
| `replay --input PATH --profile FILE` | 등록된 data-only reducer로 raw의 현재 판정 | READ_DATA, 07; 외부 verifier 진단은 별도 EXEC_ADAPTER |
| `evidence verify --input PATH --profile FILE` | envelope/참조/승계 검증 | READ_DATA, 07 |
| `qualify --inventory FILE --candidate SHA --host PLATFORM=DIR ...` | 한 후보의 host 실행 기록을 등록 칸(26 route × 3 OS)과 추가 역할 행으로 집계, 세 host 의미 비교 | READ_DATA, 08 |

명령은 network·tool 설치를 묵시적으로 하지 않는다. `--allow CAPABILITY`를 반복 지정해 실행 권한을 전달하되 profile 요청과 실제 backend가 모두 충족되어야 한다. fetch 준비는 별도 명시 절차이고 위 offline 명령에 자동 fetch 옵션을 숨기지 않는다. `--git-provenance` 같은 외부 Git 호출은 S01에서 별도 opt-in 계약·권한을 먼저 확정하기 전 구현하지 않는다.

## S01 구현

```text
tsgk inspect  --root PATH [--grammar DIR] [--out PATH]
tsgk identity --root PATH [--grammar DIR] [--file PATH=ROLE ...] [--encoding-profile cp949|none] [--declare PATH=utf-8|cp949 ...] [--large-file-profile pg-large-source-r1|large-parser-source-r1] [--profile FILE] [--out PATH]
tsgk corpus   --root PATH [--encoding-profile cp949|none] [--declare PATH=utf-8|cp949 ...] [--profile FILE] [--out PATH]
```

* `--root`의 기본값은 현재 디렉터리이고 `--grammar`의 기본값은 root sentinel `.`이다. `--file`과 `--declare`의 `PATH=VALUE`는 마지막 `=`에서 나누므로 path에 `=`가 있어도 된다. 부모 탐색은 없다. `--file`을 하나라도 주면 discovery 대신 그 목록만 선택한다.
* `--encoding-profile`은 profile 단위 cp949 선언이다. identity의 기본값은 선언 없음, corpus의 기본값은 `cp949`([NET461 등록부](../validation/net461-workload.md)의 corpus profile)다. `--declare`는 파일별 선언이며 사용자가 제공한 로컬 manifest의 값을 결과 관측 전에 옮길 때만 쓴다. 선택되지 않은 path의 선언은 오류다. `--profile`은 아래 `S02 구현` 절의 profile r1 규칙을 따른다.
* 모르는 명령(S00의 `parity` 포함)은 exit 2와 `UNKNOWN_COMMAND`다. 가짜 성공은 없다. `incremental`은 S05, `oracle record`는 S06, `replay`와 `evidence verify`는 S07, `qualify`는 S08이 구현했다.
* CLI 기본 한도는 offline-inspect의 files 10000, file_bytes 16777216, total_bytes 268435456, depth 64, output_bytes 16777216, wall 120초이고, corpus는 아래 private-corpus-local 값이다. CLI는 caller deadline을 wall+5초로 두므로 kit wall이 먼저 `RESOURCE_LIMIT`으로 끝나고, Ctrl-C 같은 caller 취소만 130이다.
* 종료 코드: 완료 0, `INVALID_INPUT` 2, `RESOURCE_LIMIT`·`UNSUPPORTED` 3, `IO`와 publication 실패 4, `CANCELLED` 130. inspect/identity/corpus는 비교를 하지 않으므로 1을 쓰지 않는다. verify는 완료된 비교의 FAIL에만 1을 쓴다.
* 출력: 성공 결과는 한 줄 JSON 문서와 줄바꿈이다. `--out`이 없으면 stdout, 있으면 그 파일에만 쓴다. 실패하면 실패 report(E0 축과 실패 finding)를 stdout에 쓰고 stderr에 `tsgk: KIND: CODE PATH`를 쓰며 `--out`에는 쓰지 않는다. exit 0과 완전한 JSON 문서가 함께 있을 때만 완전한 report다. 잘린 stdout이나 0이 아닌 exit의 출력은 성공으로 소비하지 않는다.
* `--out` publication: 대상이 이미 있으면(link 포함) 거부한다. 부모 디렉터리는 `EvalSymlinks`로 해석하고, 그래도 경로에 junction·volume mount point(Windows reparse tag 기준; cloud placeholder 같은 다른 reparse 디렉터리는 별칭이 아님)가 남아 대상을 root와 비교할 수 없으면 `OUTPUT_PARENT_ALIAS`로 거부한다. 해석한 부모 디렉터리나 그 조상이 root와 같은 파일 identity이면(대소문자·symlink·junction으로 지정한 root 포함) 입력 안으로 보고 `OUTPUT_INSIDE_INPUT`으로 거부한다. 이 검사는 경로 구성요소와 파일 identity로 하므로 경로에 드러나지 않는 별칭(Windows `subst` drive, Linux bind mount, 같은 디렉터리를 가리키는 network drive)은 검출하지 못한다. 호출자는 그런 별칭으로 입력 안을 가리키지 않아야 한다. 같은 디렉터리에 임시 파일을 완성·동기화한 뒤 hard link로 최종 이름에 놓아, 그 사이 다른 쓰기가 만든 대상도 덮어쓰지 않는다. 임시 파일은 항상 지운다. hard link를 지원하지 않는 filesystem은 publication 실패(exit 4)다.

### discovery `known-paths-r1`

grammar 디렉터리 G(`.` 또는 선택 경로)와 root에서 다음만 본다. link·junction·reparse point·special 항목은 따라가지 않고 `UNSUPPORTED` entry와 `LINK_NOT_FOLLOWED` finding으로 남기며, identity는 그런 entry가 있으면 `LINK_OR_SPECIAL_REJECTED`로 끝난다. hard link 수가 2 이상인 파일은 읽기 전에 `HARDLINK_REJECTED`다. `.gitignore`·전역 ignore·Git은 쓰지 않는다.

| 위치 | role / 상태 |
|---|---|
| root와 G의 `tree-sitter.json`, `package.json` | metadata. root `tree-sitter.json`이 없으면 `NOT_FOUND` |
| G의 `grammar.js`, `src/grammar.json` | grammar. 없으면 `NOT_FOUND` |
| G의 `src/parser.c`, `src/node-types.json` | generated. 없으면 `NOT_FOUND`(관측일 뿐 invalid가 아님) |
| G의 `src/` 직속 `.c .cc .cpp .h .hpp .inc`(위 생성 파일 제외) | scanner |
| G의 `src/tree_sitter/` 아래 `.h` | generated |
| G의 `queries/` 아래 `.scm` | query. 디렉터리가 없으면 `NOT_FOUND` |
| G의 `test/corpus/`, legacy `corpus/` 아래 모든 파일 | corpus. `test/corpus`가 없으면 `NOT_FOUND` |
| metadata가 G에 대해 선언한 `highlights`, `injections`, `locals`, `tags` | query (`METADATA`) |

entry의 `basis`는 `KNOWN_PATH`, `LISTING`, `METADATA`, `REQUIRE`, `INCLUDE`, `SELECTION` 중 하나다. `tree-sitter.json`의 `grammars[].path`와 legacy `package.json`의 `tree-sitter[].path`는 `grammars` 후보로 보고한다. G에 grammar source가 없으면 closure는 `NOT_APPLICABLE`이고 `UNKNOWN_LAYOUT`과 다른 후보마다 `GRAMMAR_CANDIDATE` finding을 낸다. 이 경우 inspect는 관측으로 완료(exit 0)하고 discovery 기반 identity는 `NO_GRAMMAR_SELECTED`(exit 2)다. grammar source가 있고 root `tree-sitter.json`이 없으면 `METADATA_ABSENT` info finding을 낸다.

closure는 실행 없이 정적으로 관측한다. grammar role의 `.js` 파일에서 literal `require('…')`·`import … from '…'`·`import('…')`를 찾고, `./`·`../` 참조만 root 안에서 그대로·`.js`·`.json`·`/index.js` 순서로 해석한다. scanner의 `#include "…"`는 포함하는 파일의 디렉터리, 그다음 tree-sitter build의 `-I G/src` 관례대로 `G/src`에서 해석한다. 찾은 파일은 같은 규칙으로 다시 훑는다. root 밖 module(`EXTERNAL_MODULE_REFERENCE`), root 밖 경로(`REFERENCE_OUTSIDE_ROOT`), 없는 파일(`REFERENCE_NOT_FOUND`, `INCLUDE_NOT_FOUND`), literal로 해석하지 못한 `require`/`import` token(`UNPARSED_REFERENCE_TOKEN`)이 하나라도 있으면 `UNRESOLVED`, 없으면 `OBSERVED`다. `OBSERVED`는 관측한 참조가 모두 root 안에서 해석됐다는 뜻이고 완전성 증명이 아니다. JS를 훑었으면 coverage `unsupported`에 `js-closure-proof`를 넣는다. 명시 선택은 `CALLER_SELECTED`다. 파일 수 한도는 선택 파일 수와 listing 항목 수에 각각 적용하고, depth는 파일·디렉터리 경로의 segment 수다. total_bytes는 연산이 실제로 읽은 bytes 합계이며, discovery 기반 identity는 metadata와 closure source를 discovery에서 한 번, identity에서 다시 한 번 읽으므로 둘 다 센다. inspect는 metadata와 closure 대상 source만 읽으므로 그 밖의 큰 파일(예: `parser.c`)은 크기만 관측한다.

### 비공개 corpus 진입점

`tsgk corpus`와 `kit.Corpus`가 `private-corpus-local`의 S01 inventory 단계다. record와 한도 표현은 [identity/evidence](identity-and-evidence.md)가 소유한다. 한도는 policy 객체의 `files`, `records`, `file_bytes`, `total_bytes`, `depth`, `output_bytes`(보고 한도), `wall_ms`로 공개하고 policy identity에 결속한다.

## S02 구현 — verify, strict profile r1, expected r1

```text
tsgk verify (--root DIR [--grammar DIR] | --archive FILE [--archive-root DIR] [--nested MEMBER ...]) --expected FILE [--profile FILE] [--encoding-profile cp949|none] [--declare PATH=utf-8|cp949 ...] [--large-file-profile ID] [--out PATH]
tsgk identity ... [--profile FILE]
tsgk corpus   ... [--profile FILE]
```

* **하나의 strict decoder.** profile과 expected 문서는 CLI와 API가 같은 decoder(`encoding/json/jsontext` token 단계와 typed 검사)로 읽는다. 문서 크기 상한은 subject 한도와 별개인 16777216 bytes(`kit.MaxDocumentBytes`, 초과 `DOCUMENT_BYTES_LIMIT`), 중첩 상한은 32다(초과 `JSON_DEPTH_LIMIT`, 둘 다 `RESOURCE_LIMIT`). 잘못된 UTF-8은 `JSON_INVALID_UTF8`, BOM·문법 오류·`NaN`·선행 0·짝 없는 surrogate escape는 `JSON_SYNTAX`, 빈 문서·잘린 문서는 `JSON_TRUNCATED`, escape 표기가 달라도 decode한 이름이 같은 중복 key는 `JSON_DUPLICATE_KEY`, 첫 값 뒤의 값은 `JSON_TRAILING_VALUE`다. typed 검사는 정확한 철자의 key만 받는다(대소문자만 다른 key도 `JSON_UNKNOWN_FIELD`). 필수 key 부재는 `JSON_MISSING_FIELD`, `null`은 부재와 구분해 `JSON_NULL`, bool·string·number를 서로 바꾸지 않아 `JSON_TYPE`이다. 정수는 0~2^53-1의 정규 10진(부호·소수점·지수·선행 0 없음)만 받고 그 밖은 `JSON_NUMBER_NOT_INTEGER` 또는 `JSON_INTEGER_RANGE`다. 숫자를 float64로 왕복하지 않는다. 오류 `path`는 문서 이름과 JSON pointer다(예: `profile#/files/0/path`). 모두 `INVALID_INPUT`(exit 2)이고 사용 전에 거부한다.
* **profile `tsgk-profile/r1`.** 필수 `schema`, `id`(1~128자 ASCII 영숫자·`.`·`_`·`-`)와 선택 `files`, `limits`, `encoding`만 있다. `files`는 비어 있지 않은 `{path, role, required}` 배열이며 path는 portable, byte 오름차순이고 중복(`SELECTION_DUPLICATE`)·순서 위반(`SELECTION_UNSORTED`)·ASCII 대소문자 충돌(`SELECTION_CASE_COLLISION`)을 거부한다. `limits`는 양의 정수 `files`, `records`, `file_bytes`, `total_bytes`, `depth`, `output_bytes`, `archive_entries`, `archive_bytes`, `archive_depth` 중 일부다(0은 `PROFILE_LIMIT_INVALID`). `encoding`은 `profile`(`"cp949"`만)과 비어 있지 않은 `files`(`{path, encoding: "utf-8"|"cp949"}`, 중복은 `ENCODING_DECLARATION_DUPLICATE`, 그 밖은 `ENCODING_DECLARATION_INVALID`)다. hook·명령·대용량 예외 이름 같은 다른 key는 `JSON_UNKNOWN_FIELD`다. profile은 경로·역할·필수 여부·encoding 선언·더 낮은 한도만 정하며 효과를 주거나 무엇을 실행하지 않는다. 비공개 경로는 profile에 넣지 않고 로컬 기록에 둔다.
* **한도는 낮추기만 한다.** profile 값이 그 연산의 caller 한도(CLI는 기본값)보다 크면 `PROFILE_LIMIT_ABOVE_OPERATION`, 그 연산에 없는 key면 `PROFILE_LIMIT_NOT_APPLICABLE`이다. identity는 `files`·`file_bytes`·`total_bytes`·`depth`·`output_bytes`, verify는 여기에 archive subject일 때만 `archive_*`, corpus는 `files`·`records`·`file_bytes`·`total_bytes`·`depth`·`output_bytes`(보고 한도)를 받는다. corpus의 상한은 caller 한도와 추적 `private-corpus-local` 값 중 작은 쪽이다. 적용된 한도는 결과 `policy`와 policy identity에 그대로 나타나고 profile 원문 sha256은 `profile` identity로 결속된다. 등록된 대용량 예외는 요청 필드(`--large-file-profile`)일 뿐 profile로 지정하지 못한다.
* **선택과 encoding의 출처는 하나.** profile `files`가 있으면 identity·verify의 선택은 그 목록이며 `--file`(API `Selection.Files`)과 같이 주면 `SELECTION_SOURCE_CONFLICT`다. profile `encoding`이 있으면 `--encoding-profile`/`--declare`(API `Encoding`)와 같이 줄 수 없다(`ENCODING_SOURCE_CONFLICT`). corpus는 파일 선택이 없어 profile `files`를 `PROFILE_FILES_NOT_APPLICABLE`로 거부한다. `corpus --profile`은 CLI 기본 cp949 선언을 쓰지 않고 profile의 `encoding`만 쓴다. profile에 `encoding`이 없으면 선언 없음(`detect-r1`)이므로 cp949가 필요하면 profile에 `"encoding":{"profile":"cp949"}`를 넣는다. 명시한 `--encoding-profile`은 profile `encoding`과 충돌한다. identity에서 `required: false` 파일이 없으면 `OPTIONAL_FILE_ABSENT` info로 관측하고, 필수 파일이 없으면 S01처럼 `SELECTED_FILE_NOT_FOUND`다. inspect는 profile을 읽지 않는다(exit 2).
* **expected `tsgk-expected/r1`.** `{schema, provenance, file_count, set_sha256, manifest}`이고 `manifest`는 `tsgk-manifest/r2`(`schema`, `algorithm: sha256`, `mode_policy: portable-default`, `encoding_policy`, `files`) 그대로다. `tsgk identity` 결과의 `manifest`와 `set_sha256`으로 만들 수 있다. record는 identity가 내는 필드와 같고, path는 byte 오름차순이며 중복·대소문자 충돌이 없고(`EXPECTED_PATH_DUPLICATE`/`_UNSORTED`/`_CASE_COLLISION`) [이름 정책](trust-and-execution.md) `portable-names-r1`을 따른다(`EXPECTED_PATH_<사유>`). mode는 `100644`/`POLICY_DEFAULT`만, sha256은 소문자 64자, encoding 값은 닫힌 집합이다(빈 문자열 금지; PASS는 code 없음, 그 밖은 code 필수). `file_count`와 `set_sha256`은 record로 다시 계산한 값과 같아야 한다(`EXPECTED_COUNT_MISMATCH`, `EXPECTED_SET_MISMATCH`). `provenance`(1~1024 bytes, 제어문자 없음)는 호출자의 출처 진술이며 kit가 확인하지 않는다.
* **비교 범위(scope).** 디렉터리 subject는 profile `files`가 없으면 S01 identity와 같은 `known-paths-r1` discovery, 있으면 `listed-r1`이다. archive subject는 profile `files`(API는 `Selection.Files`도)가 없으면 `archive-members-r1`(`--archive-root` 아래 모든 file member), 있으면 `listed-r1`이다. 명시 목록은 subject와 관계없이 같은 규칙(비어 있지 않음, portable path, 등록 role, 중복 금지)으로 검사한다. archive에서 scope 밖 file member는 읽지 않고 구조 검사만 한 뒤 `excluded` 수로 보고한다. 디렉터리 subject는 scope 밖 파일을 열거하지 않으므로 `excluded`가 0이며 그 밖의 무결성을 주장하지 않는다. actual은 subject에서 독립적으로 계산한다. archive에서는 role을 관측할 수 없으므로 listed scope는 profile의 role, members scope는 expected의 role을 쓰고 expected에 없는 member의 role은 빈 값이다(coverage `unsupported`의 `member-role-observation`).
* **결과.** `VerifyResult`는 E0에 `policy`, `subject`(`DIRECTORY`|`ARCHIVE`), `scope`, `grammar`, `archive_root`, `nested`, `expected{provenance, set_sha256, file_count, document_sha256}`, `actual`(manifest), `actual_set_sha256`, `differences[{code, path, expected, actual}]`, `excluded`를 더한다. difference는 `MISSING_REQUIRED`, `MISSING_OPTIONAL`(profile이 `required: false`로 미리 선언; 실패 아님), `UNEXPECTED_FILE`, `ROLE_CHANGED`, `SIZE_CHANGED`, `CONTENT_CHANGED`, `MODE_CHANGED`, `ENCODING_CHANGED`이며 한 파일의 여러 필드가 다르면 각각 낸다. 대상 path는 expected·actual·profile 필수 목록의 합집합이다. rename은 `MISSING_REQUIRED`와 `UNEXPECTED_FILE`로 나타난다. `MISSING_OPTIONAL` 밖의 difference가 하나라도 있으면 `assessment: FAIL`(exit 1), 없으면 `PASS`(exit 0)이고 둘 다 `execution_status: COMPLETED`다. identities는 `expected-set`, `source-set`(actual), `expected-document`(원문 sha256), `profile`, `archive`(archive 원본 bytes sha256), `policy`다. 항상 `EXPECTED_TRUST_CALLER_DECLARED` info finding을 낸다. PASS는 이 선택 집합이 그 expected 기록과 같다는 뜻이며 서명·권리자 인증·grammar 정확성이 아니다.
* **encoding policy는 선언이다.** 요청(또는 profile)의 encoding policy 이름이 expected의 `encoding_policy`와 다르면 비교하지 않고 `ENCODING_POLICY_MISMATCH`(exit 2)다.
* **trust 문서 위치.** 디렉터리 subject에서 CLI는 `--expected`·`--profile` 파일의 디렉터리가 root이거나 그 아래면(파일 identity 비교, `--out`과 같은 검사) `EXPECTED_INSIDE_INPUT`/`PROFILE_INSIDE_INPUT`(exit 2)으로 거부한다. archive 안의 manifest·profile은 member일 뿐 읽거나 신뢰하지 않는다(expected에 없으면 `UNEXPECTED_FILE`).
* **오류 path.** profile·expected 오류의 path는 `문서#JSON pointer`이며 공유 strict decoder가 pointer를 1024 bytes에서 잘라 `...(truncated)`를 붙인다(잘린 path는 유효한 pointer가 아니다). 긴 member 이름이 오류 report를 키우지 않게 하는 규칙이며 S03 schema 연산과 같다.
* **실패와 출력.** 잘못된 문서와 subject(archive 구조·이름 공격 포함)는 `INVALID_INPUT`(exit 2), 지원하지 않는 archive 기능과 ASCII 밖 이름은 `UNSUPPORTED`(exit 3), 한도는 `RESOURCE_LIMIT`(exit 3)이다. 이때 actual·`actual_set_sha256`·differences는 비고 결과는 COMPLETED/PASS가 아니다. `--out`은 PASS와 FAIL의 완전한 report만 쓴다. archive subject의 `--out`에는 입력 root 검사가 없고(입력은 파일 하나) 기존 대상 거부와 hard link publication 규칙은 같다.
* **기본 한도.** verify는 offline-inspect 한도와 archive 한도 entries 10000, archive 파일 268435456 bytes, nesting depth 2(`kit.DefaultArchiveLimits`)를 쓴다. policy의 `operation`은 `offline-verify`, `discovery`는 scope다. archive subject에서만 `archive_profile`·`archive_entries`·`archive_bytes`·`archive_depth` 줄이 policy preimage에 붙으므로 S01 연산의 policy identity는 바뀌지 않는다.
* **extraction은 없다(UNAVAILABLE).** 채택된 source 준비 계획이 ZIP extraction을 요구하지 않으므로 이 build에는 풀기 명령과 쓰기 capability가 없다. source는 별도 승인 절차로 준비한다.

## S03 구현 — schema check, schema diff

```text
tsgk schema check --input FILE [--out PATH]
tsgk schema diff  --before FILE --after FILE [--out PATH]
```

* 형식·판정·차이 모델·한도는 [정적 node-types 계약](tree-and-adapter-protocol.md)이 소유한다. CLI는 파일을 `MaxDocumentBytes`+1 bytes까지만 읽어 API에 넘기고 같은 결과를 그대로 출력한다(기본 문서 한도는 `MaxDocumentBytes`를 넘지 않으며, 한도를 넘은 입력은 hash하지 않으므로 잘린 읽기와 전체 파일의 결과가 같다). 결과의 `name`은 파일의 base name이며 디렉터리·절대 경로는 넣지 않는다. 같은 base name의 두 입력은 `role`과 sha256으로 구분된다.
* 필수 인자가 없거나 하위 명령이 `check`·`diff`가 아니면 파일을 읽기 전에 exit 2(`USAGE`)다. 읽을 수 없는 입력은 `SCHEMA_UNREADABLE` exit 4다.
* exit: check는 PASS 0, FAIL 1, BLOCKED 3이다. diff는 같으면 0, 차이가 있으면 1, 잘못된 입력 schema는 `SCHEMA_INVALID` exit 2, 해석하지 않는 key만 가진 입력은 `SCHEMA_KEY_UNSUPPORTED` exit 3이다. 한도 3, 취소 130은 공통 규칙과 같다.
* `--out`은 S01 publication 규칙을 따르되 입력 root가 없으므로 기존 대상 거부와 hard link publication만 적용한다(입력 파일 자신도 기존 대상이라 덮어쓰지 않는다). 기본 한도는 `kit.DefaultSchemaLimits`다.

## S04 구현 — reproduce, reproduce profile r1

```text
tsgk reproduce --root PATH --profile FILE --out DIR --work DIR --tool NAME=PATH ... --allow EXEC_GENERATOR [--cgroup-parent DIR]
```

`reproduce`는 process를 시작하는 유일한 명령이며 `--allow EXEC_GENERATOR`가 없으면 실행 전 `CAPABILITY_NOT_GRANTED`(exit 3)다. 공개 offline API(`src/kit`)는 profile 해석(`kit.ParseReproduceProfile`)만 제공하고 실행은 CLI의 `src/internal/reproduce`가 [S04 runner](platform-support.md)로 한다. profile, `--work`, `--out`의 부모는 `--root` 밖이어야 한다(`PROFILE_INSIDE_INPUT`, `WORK_INSIDE_INPUT`, `OUTPUT_INSIDE_INPUT`).

profile `tsgk-reproduce/r1`은 S02와 같은 strict decoder로 읽는다. 필수 필드는 `schema`, `id`, `route`, `mode`(`js`|`json`), `generator`(`name`·`version`·`sha256`·`bytes`), `abi`(14 또는 15, 그 밖은 `ABI_UNSUPPORTED`), `optimize`(bool, false면 `--disable-optimization`), `grammar`(진입 파일; js는 `grammar_js`, json은 `grammar_json` 역할 input이어야 함), `inputs`, `outputs`, `limits`다. `js_runtime`은 js에서 필수, json에서 금지다(`JS_RUNTIME_REQUIRED`, `JS_RUNTIME_IN_JSON_MODE`). `inputs`는 정렬·중복 없는 portable path의 `path`·`role`(grammar_js, js_helper, lock, dependency, grammar_json, scanner, header, metadata)·`sha256`·`bytes`이고 이것이 불변 source snapshot 전부다. `outputs`는 생성기 출력 디렉터리 기준 `path`와 `reference`(`PRESENT`면 `sha256`·`bytes` 필수, `ABSENT`면 금지)다. `limits`의 `wall_seconds`·`output_bytes`·`storage_bytes`·`memory_bytes`·`input_files`·`input_bytes`·`file_bytes`는 모두 필수이고 0이나 generator 연산 상한(300초, 8388608, 536870912, 4294967296, 64, 67108864, 16777216) 초과는 `PROFILE_LIMIT_INVALID`다. 메모리만 `route`가 `postgresql-sql`일 때 6442450944까지 허용한다. 예시는 `src/contracts/examples/reproduce-r1.json`, 거부 예시는 `invalid/reproduce-*.json`이다.

실행 순서: profile 검증 → `--root`의 선언 파일만 읽어 각 경로 구성요소의 link·특수 파일을 거부하고 크기·hash를 대조(`SOURCE_MISSING`, `SOURCE_LINK_REJECTED`, `SOURCE_MISMATCH`) → 도구를 `--tool`의 경로에서 읽어 크기·hash를 대조한 뒤 `--work` 안 새 도구 디렉터리에 복사하고 그 사본을 실행(`TOOL_MISSING`, `TOOL_IDENTITY_MISMATCH`; PATH 탐색·설치·대체 없음) → Windows·Linux는 hard memory backend가 없으면 `MEMORY_HARD_CAP_UNSUPPORTED`(macOS는 sampled, non-strict) → js 모드는 `--work`나 그 상위 디렉터리(symlink를 푼 실제 경로의 상위도)의 `node_modules`, `--work/lib/node`(복사한 runtime의 전역 module 위치)가 있으면 선언하지 않은 module이 해석될 수 있으므로 `JS_CLOSURE_LEAK`로 막는다 → `--work` 안에 매번 새로 만든 빈 작업 공간 A, B를 차례로 만들고(기존 디렉터리는 재사용하지 않음) snapshot을 쓴 뒤 `generate --abi N [--disable-optimization] --output <ws>/out [--js-runtime <node 사본>] <grammar>`를 실행한다. 환경은 `PATH=`(비움)와 작업 공간 안의 HOME·USERPROFILE·APPDATA·LOCALAPPDATA·XDG_CONFIG_HOME·XDG_CACHE_HOME·TMP·TEMP·TMPDIR·TREE_SITTER_DIR·TREE_SITTER_LIBDIR뿐이다. 실행 전 거부는 작업 공간과 결과를 만들지 않는다.

비교: 등록 output마다 A와 B의 bytes(`EQUAL`·`DIFFERENT`·`MISSING`)와 A와 기준(`MATCH`·`MISMATCH`·`REFERENCE_ABSENT`·`OUTPUT_MISSING`)을 따로 기록한다. 출력 디렉터리의 등록되지 않은 파일은 `UNREGISTERED_OUTPUT`으로 남기고 A·B 사이에서도 비교한다. 작업 공간 source 사본에 대한 쓰기는 `WORKSPACE_SOURCE_WRITTEN`, 실행 뒤 원본 root가 달라지면 `SOURCE_CHANGED`다. 정규화는 하지 않는다(byte identity). claim은 `generator_ran`, `deterministic`, `reference_match`, `js_reproduction`(js 모드에서 세 claim이 모두 PASS일 때만), `json_regeneration`(json 모드)이며 값은 `PASS`·`FAIL`·`NOT_CLAIMED`다. json 실행은 `js_reproduction`을 주장하지 않는다. 기준이 없으면 `reference_match`는 `NOT_CLAIMED`이고 assessment는 `BLOCKED`다. A가 실패하면 B는 `NOT_RUN`이다. 생성기의 0이 아닌 종료는 `FAILED`(`GENERATOR_EXIT_NONZERO`), runner 한도는 `RESOURCE_LIMIT`, cleanup 미확인은 `FAILED`(`PROCESS_CLEANUP_UNVERIFIED`)다. `run_identity`는 profile 전체·도구 hash·snapshot·argv·환경 이름·backend capability를 묶으며 어느 하나가 바뀌면 달라진다. 저장 용량은 실행 뒤 작업 공간 합계로 관측하며(실행 중 quota는 아님) `storage_bytes`를 넘으면 그 작업 공간에서 멈추고 `STORAGE_LIMIT` finding을 남긴다. 다른 실패 원인이 없으면 `RESOURCE_LIMIT`(assessment `BLOCKED`)이고, 있으면 그 원인의 상태에 이 finding이 더해진다.

publication: `--out`은 새로 만들어야 하며 있으면 실행 전 `OUTPUT_EXISTS`다. `workspace-a|b/out/`에 생성물을, `stdout.log`·`stderr.log`에 원 출력을 두고 마지막에 `result.json`(`tsgk-reproduce-result/r1`)을 쓴다. 실패한 실행도 partial 생성물과 실패 report를 남긴다. 그 뒤 작업 공간과 도구 사본 디렉터리를 지우며 지우지 못하면 `WORKSPACE_CLEANUP_FAILED`·`TOOL_CLEANUP_FAILED`로 완료가 아니다. 실행이 완료되지 않은 결과(source 변경, 저장 용량 초과, cleanup·결과 쓰기 실패)는 assessment `PASS`를 갖지 않고(`NOT_ASSESSED`, 한도면 `BLOCKED`), `js_reproduction`·`json_regeneration`도 `PASS`로 남지 않는다(`NOT_CLAIMED`). 관측 claim(`generator_ran`, `deterministic`, `reference_match`)은 관측한 그대로 둔다. 저장 용량 초과는 다른 실패 원인과 함께 finding으로 남는다. 결과 쓰기 실패는 `EVIDENCE_WRITE_FAILED`(exit 4)다. stdout에는 같은 report 한 줄을 쓴다. exit: 완료 PASS 0, FAIL 1, BLOCKED 3, `RESOURCE_LIMIT` 3, 생성기·cleanup 실패 1, 취소 130.

## S05 구현 — incremental, incremental profile r2

```text
tsgk incremental --root PATH [--grammar-root PATH] --profile FILE --runtime DIR --tool cc=PATH --work DIR --out DIR --allow BUILD_NATIVE --allow EXEC_NATIVE [--cgroup-parent DIR] [--run-wall SECONDS]
```

`incremental`은 profile의 grammar로 [`tsgk-native/r1` driver](tree-and-adapter-protocol.md)를 build하고 사례마다 driver process 하나를 S04 runner로 실행한다. 두 capability가 없으면 실행 전 `CAPABILITY_NOT_GRANTED`(exit 3)이며 `--out`을 만들지 않는다. 사례 입력은 `--root`, grammar 파일은 `--grammar-root`(기본 `--root`) 아래에서 읽는다(비공개 corpus를 복사하지 않고 읽기 위함). profile·`--work`·`--out`의 부모는 두 root 밖이다(`PROFILE_INSIDE_INPUT`, `WORK_INSIDE_INPUT`, `OUTPUT_INSIDE_INPUT`). `--out`이 있으면 `OUTPUT_EXISTS`(exit 2)다. 처음 계획한 `--edits FILE`은 profile의 사례 안 `edits`로 합쳤다.

profile `tsgk-incremental/r2`는 S02와 같은 strict decoder로 읽는다. 아래에서 선택이라 적은 것 말고 필드는 모두 필수다. `schema`, `id`, `route`(1~128자 ID), `operation`, `symbol`(`^tree_sitter_[a-z0-9_]{1,64}$`; shim에 쓰는 유일한 caller 값이며 그 밖은 `SYMBOL_INVALID`), `encoding`(`UTF-8`·`UTF-16LE`·`UTF-16BE`·`CP949`), `output`, `compiler`(`name`·`version`·`sha256`·`bytes`; `--tool cc=PATH`의 내용이 같아야 함), `grammar`(정렬·중복 없는 portable path의 `path`·`role`(`parser` 정확히 하나, `scanner`, `header`)·`sha256`·`bytes`, 최대 64개·파일당 104857600·합계 134217728 bytes), `declarations`(`null` 또는 `{mapping, items[{fact, node, name}]}`, 최대 64개, `name`은 `node` 또는 `field:`·`child:`·`children:` locator 경로), `cases`다. 선택 필드 `format`은 `SVC-SERVICEHOST-r1`만 받고 `symbol`이 `tree_sitter_c_sharp`여야 한다(그 밖은 `FORMAT_UNSUPPORTED`). 이때 사례 입력은 `.svc` 원본이고 [tree/protocol](tree-and-adapter-protocol.md)의 `.svc` composite 규칙을 따른다. 사례는 `id`, `input`(`role: case`), 선택 `encoding`(기본은 profile 값), `edits`(`start_byte`·`old_end_byte`·`new_end_byte`·`old`·`new`, `old`·`new`는 패딩 있는 표준 base64), `points`(`{id, byte}`, 최대 64), `expect`(`{step, syntax: NO_ERROR|ERROR|ANY, contains: [named type], anchors, declarations: ""|PASS|FAIL|NOT_APPLICABLE}`)다. 예시는 `src/contracts/examples/incremental-r2.json`, 거부 예시는 `invalid/incremental-*.json`이다.

**기대값 anchor(r2).** `contains`는 그 type의 named node가 tree 어디에든 있으면 통과한다. 그래서 한 사례에 구조 여러 개를 넣으면, 대상 구조를 잘못 parse해도 같은 type이 다른 줄에 있어 통과할 수 있다. 선택 필드 `anchors`(`[{type, start_byte, end_byte}]`, 최대 64개)는 이를 막는다. anchor 하나는 그 step의 기록된 full tree에 정확히 그 type이고 정확히 그 byte 범위 `[start_byte, end_byte)`인 named node가 있을 때만 통과한다. 그런 node가 없으면 그 기대값은 FAIL이고, tree가 full이 아니면(요약·record 형식) BLOCKED다. 검사 순서는 `syntax` → `contains` → `anchors` → `declarations`이며 앞의 검사가 PASS가 아니면 그 결과가 남는다. `anchors`가 없는 기대값은 r1과 같게 판정한다. `type`은 비어 있지 않은 1~128자 출력 가능 ASCII(`"`·`\` 제외)여야 한다. `start_byte ≤ end_byte`여야 하고 `end_byte`는 그 step source 길이(입력 `bytes`에서 앞 edit들의 `old`·`new` 길이로 계산) 이하여야 한다. 어기면 `EXPECT_ANCHOR_INVALID`다. 사례 결과의 기대값(`expectations`)은 등록 기대값을 그대로 담으므로 anchor가 있으면 `anchors`도 담는다. 결과 schema `tsgk-incremental-result/r1`·`tsgk-oracle-record/r1`은 그대로다.

r1 profile(`tsgk-incremental/r1`, `anchors` 없음)도 이전 근거를 replay할 수 있게 계속 읽는다. r1 기대값에 `anchors`가 있으면 `JSON_UNKNOWN_FIELD`다. 결과의 profile identity는 문서가 선언한 revision을 적는다. `run-routes.ps1`·`run-corpus.ps1`은 r2를 쓴다.

원본 사례 파일(`src/testdata/native/{routes,gaps,n461}/*.json`)은 anchor를 `{type, text, occurrence}`로 적는다. `type`·`text`는 문자열이고 `text`는 정확한 source 문자열이다. `occurrence`는 선택이며 1부터 센다(없으면 1). 있으면 1 이상의 정수여야 하고 `null`은 받지 않는다. anchor 객체는 정확히 이 세 이름 말고 다른 멤버를 받지 않는다(대소문자도 구분한다). 잘못 쓴 이름이 조용히 기본 출현 1로 바뀌어 기대값이 공허하게 통과하는 일을 막기 위해서다. 변환은 그 step source(앞 edit를 적용한 것)에서 `text`의 UTF-8 bytes가 시작하는 offset 중 `occurrence`번째를 고른다. 모든 offset을 세므로 겹치는 출현도 센다. 결과는 `start_byte`=그 offset, `end_byte`=offset+bytes 길이다. 찾지 못하거나, `type`·`text`가 비었거나, 위 형식을 어기면 두 변환 모두 실패한다. `run-routes.ps1`(`Convert-Cases`, `Resolve-Anchor`)과 inventory 생성기가 같은 방식으로 변환하며 `TestAnchorConversion`·`TestRegisteredAnchors`가 둘을 독립 구현과 대조한다(pwsh가 있으면 `Convert-Cases`를 직접 실행한다).

| `operation` | 입력 | node / full gate | 출력 | parse당 | process wall | edit | 사례 |
|---|---|---|---|---|---|---|---|
| `native-parse-edit` | 65536 | 10000 / 10000 | 8388608 | 10초 | 10초 | 4 (`tree`만) | 1000 |
| `real-world-source-r2` | 33554432 | 25000000 / 50000 | 16777216 | 60초 | parse 90초, edit 300초 | 4 (`tree`만) | 64 |
| `private-corpus-local` | 33554432 | 25000000 / 50000 | 16777216 | 60초 | batch 3600초, frame 60초+5초 | 0 (`record`만) | 26000 |

`--run-wall`은 run 전체 wall을 연산 값보다 낮게만 줄인다(여러 호출을 한 실행 wall 안에 넣을 때). 줄인 값은 결과 `operation.run_wall_ns`와 policy identity에 남는다. 세 연산 모두 tree depth 100000, 메모리 4294967296(Windows·Linux hard, macOS sampled non-strict; S06의 `real-world-source-r3`는 아래 `S06 구현`), ERROR/MISSING 1000건, partial tree 1000 node이고 run 전체 wall은 3600초다. `private-corpus-local`은 process 하나에 사례 500개 또는 입력 268435456 bytes까지 `batch` frame으로 보내며, batch 한도나 치명 frame(`ALLOCATION_LIMIT`) 뒤에 답하지 않은 사례는 새 process로 다시 보낸다(재시도로 세지 않음). 마지막 응답 뒤 stdout bytes·cleanup 미확인·비정상 exit가 있으면 그 batch의 어떤 frame도 완료로 받지 않는다. run wall 안에 처리하지 못한 사례는 `NOT_RUN`이다.

실행 순서: profile 검증 → hard memory backend 확인(macOS 제외, 없으면 `MEMORY_HARD_CAP_UNSUPPORTED`) → `--work` 안 새 build 디렉터리에 runtime(내장 manifest와 hash 대조, `RUNTIME_MISMATCH`)·grammar(`SOURCE_MISMATCH`, link 거부)·driver source·shim 복사 → 컴파일러 `--version`과 compile/link(아래 [플랫폼](platform-support.md) `S05 native build`) → 사례마다 입력 hash 확인(`SOURCE_MISMATCH`), edit 검사(`kit.ApplyEdits`), 실행 직전 executable hash 재확인(`EXECUTABLE_MISMATCH`), runner 실행, response 검증 → 판정 → `--out/result.json`(`tsgk-incremental-result/r1`)과 `--out/responses/<순번 5자리>-<사례>.json`(원 response payload; 대소문자를 구분하지 않는 filesystem에서도 겹치지 않음, 쓰기 실패면 실행 `FAILED`) → build 디렉터리 삭제 확인(`BUILD_CLEANUP_FAILED`면 완료가 아님). stdout에는 같은 결과 한 줄을 쓴다.

사례 결과는 `execution_status`, `assessment`, `code`(첫 실패 사유; 실패가 없으면 오류 tree에서 관측할 수 없는 첫 route step의 `INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_<n>`), claim 세 개(`incremental_equality`, `incremental_route`, `expectations`; `PASS`·`FAIL`·`BLOCKED`·`NOT_CLAIMED`), driver status·code·producer, runner 결과, step별 source bytes·sha256, edit와 point, route 계측과 `proven`, 비교(`equal`, `first_difference`), 공개 `tsgk-tree/r1`·`tsgk-tree-summary/r1` envelope, 기대값 결과다. 판정은 [tree/protocol](tree-and-adapter-protocol.md) `S05 구현`의 비교 projection을 따른다. exit: 모든 사례가 완료되고 PASS면 0, FAIL 1, BLOCKED·`RESOURCE_LIMIT` 3, 실행 전 거부 2, 실패(FAILED·cleanup·결과 쓰기) 4, 취소 130이다.

## S06 구현 — oracle record, oracle profile r2

```text
tsgk oracle record --root PATH [--grammar-root PATH] --profile FILE [--fact-pack FILE] --runtime DIR --tool cc=PATH --work DIR --out DIR --allow BUILD_NATIVE --allow EXEC_NATIVE [--cgroup-parent DIR] [--run-wall SECONDS]
```

`oracle record`는 `incremental`과 같은 인자 검사·build·runner·driver를 쓰고 요청만 [`tsgk-native/r2`](tree-and-adapter-protocol.md)로 보낸다. capability 거부, root 밖 경로 규칙(`PROFILE_INSIDE_INPUT`은 `--fact-pack`에도 적용), hard memory backend 확인, build 단계와 실패 code, 사례 입력 hash, edit 검사, 실행 직전 executable hash 재확인은 `S05 구현`과 같다. `--out`은 배타적으로 만든다. 이미 있거나 동시 실행이 먼저 만들었으면 `OUTPUT_EXISTS`(exit 2)이며 그 디렉터리에 아무것도 쓰지 않는다. 결과 디렉터리는 [기록 set](tree-and-adapter-protocol.md)(`records/`, `raw/`, 마지막 `manifest.json`)뿐이다. stdout에는 `tsgk-oracle-result/r1` 한 줄을 쓴다. 이 결과는 E0 report, profile·route·operation, build, build 삭제 결과, 사례 요약(상태·판정·code·S05 claim·S06 claim), set 검증 결과(`kit.VerifyOracleSet`: `valid`, record 수, 실행 상태·판정, finding), platform, wall을 담는다. exit는 `incremental`과 같다. member 쓰기 실패·set 검증 실패는 실행 `FAILED`(exit 4)이고 완결 set이 아니다.

profile `tsgk-oracle/r2`는 `tsgk-incremental/r2`의 모든 필드(같은 strict decoder와 같은 규칙, 기대값 `anchors` 포함)에 세 필드를 더한다. `tsgk-oracle/r1`(`anchors` 없음)도 replay를 위해 계속 읽는다. `queries`는 `{id, source}` 최대 16개이고, id는 유일한 1~128자 ID, source는 UTF-8 1~65536 bytes다. `fact_pack`은 `null` 또는 `{revision, sha256, route}`이며 route는 profile route와 같아야 한다. `api`는 boolean이다. 사례에는 두 필드를 더한다. `query_expect`는 `{query, step, status, code, captures, error}` 목록이다. `captures`는 `null`(capture를 주장하지 않음) 또는 그 step incremental tree의 평가된 stream 전체를 순서대로 담은 `{name, type, text}`이고 `text`는 원본 UTF-8 bytes다. `error`는 `INVALID_QUERY`일 때만 `{type, offset}`이다. `dynamic_sql_expect`는 `null` 또는 `{facts, known_misses}`이며 `fact_pack`이 있어야 한다. `operation`은 query 연산만 받고, `tsgk-incremental` profile은 query 연산을 받지 않는다(`OPERATION_UNSUPPORTED`). profile이 pack을 결속하면 `--fact-pack`의 revision·sha256이 같아야 한다. 그 route의 pack query를 하나 이상 같은 id로 담아야 하고, 담은 query는 pack과 source가 같아야 한다(대용량 입력에는 선언 query만 고르는 식으로 일부만 담을 수 있다). 선언 query는 profile `declarations`로 만든 것과 같아야 한다. 어긋나면 실행 전 `FACT_PACK_MISMATCH`다.

| `operation` | 입력 | node / full gate | 출력 | match / capture | parse당 | query | process wall | edit | 사례 | host |
|---|---|---|---|---|---|---|---|---|---|---|
| `native-query` | 65536 | 10000 / 10000 | 8388608 | 10000 / 10000 | 10초 | 4초 | 10초 | 4 (`tree`만) | 1000 | 세 OS |
| `native-query-large` | 33554432 | 25000000 / 50000 | 16777216 | 1000000 / 1000000 | 60초 | 20초 | 90초 | 0 (`auto`만) | 64 | windows/amd64 |

query 열은 query 실행 하나의 시간 예산이며 process wall 안에 들어가도록 정했다(등록 wall 10초·90초는 그대로). 사용자 등록 값이 아니라 S06이 wall 안에서 정한 값이다.

두 연산의 tree depth는 100000, ERROR/MISSING 목록은 1000건, partial tree는 1000 node, run 전체 wall은 3600초다. 메모리는 `native-query`가 4294967296(Windows·Linux hard, macOS sampled non-strict), `native-query-large`가 아래 `real-world-source-r3`와 같은 8589934592다.

**`real-world-source-r3`(2026-10-03 사용자 결정 `C1-REAL-WORLD-SOURCE-WINDOWS-R3`).** S05의 `real-world-source-r2`는 역사로 보존한다. r3는 r2와 같은 값이고 메모리만 8589934592(8 GiB, Windows Job Object hard)이며 windows/amd64에서만 실행한다. 값은 결정 receipt의 규칙대로 정했다. `cs-large-32mib-errors`를 로컬 Windows에서 12 GiB 측정 상한으로 실행한 peak commit은 5157146624 bytes였고, 여기에 약 20%를 더한 값 이상인 8·10·12 GiB 중 가장 작은 값이 8 GiB다. 다른 host에서 r3(`incremental`)나 `native-query-large`(`oracle record`) profile을 실행하면 두 명령 모두 build 전에 `OPERATION_PLATFORM_SCOPE`(BLOCKED, exit 3)이며 이유는 "NET461 workload is Windows-hosted (WinForms/.NET Framework 4.6.1)"이다. route helper는 그 host의 대용량 행을 `NOT_APPLICABLE`로 기록하고 실행하지 않는다. grammar route 26개의 세 OS 검증은 그대로다. driver의 `memory_bytes` 상한도 8589934592로 올렸다. 다른 상한은 바꾸지 않았다.

사례 결과는 `S05 구현`의 모든 필드에 다음을 더한다. tree마다 `queries`(query마다 `id`, `sha256`, status·code, `evaluation`, 오류, pattern 수, capture 이름, predicate 단계, match 수, `partial`, 평가된 `captures`)를 둔다. 완료되지 않은 tree에는 모든 query를 `NOT_RUN`/`TREE_NOT_COMPLETED`로 둔다. full tree에는 S05 선언 항목(`declarations`)과 API 판정(`api{revision, consistent, first_difference, position_navigation_divergences, first_divergence}`)도 둔다. edit step에는 incremental/fresh query 비교(`query_comparison`)를 둔다. 사례 수준에는 S06 claim 다섯 개(`oracle_claims`), query 기대값 결과(`query_expectations`), pack 사실(`facts`: 재현한 선언 항목과 S05 일치 여부, 동적 SQL 사실과 known miss, XML 구조 capture 수, 첫 차이)를 둔다. full tree envelope의 `capabilities`는 producer의 `query`·`api` 값이고 `captures`는 `null`이다(capture는 query identity와 함께 query마다 있다). 판정은 S05 claim과 S06 claim 중 가장 나쁜 값이다.

## S07 구현 — replay, evidence verify

```text
tsgk replay          --input DIR --profile FILE [--out PATH]
tsgk evidence verify --input DIR --profile FILE [--out PATH]
```

두 명령은 READ_DATA만 쓴다. process·network·tool을 시작하지 않고 evidence 안의 코드를 import·실행하지 않으며 archive를 풀지 않는다(archive member도 hash만 확인하는 불투명 member다). `--input`은 evidence set root이고 S01 guard(no-follow, link·special 거부, 깊이 64)로 전체를 한 번 나열한다. `--profile`은 caller가 신뢰하는 등록 문서이며 `--input` 안에 있으면 `PROFILE_INSIDE_INPUT`(exit 2)이다. 결과는 한 줄 JSON이고 `--out`은 S01 publication 규칙(no-clobber, 입력 밖)을 따른다. exit: `PASS` 0, `FAIL` 1(완료된 검사가 불일치를 찾았거나 다시 계산한 subject 판정이 FAIL), `UNRESOLVED`·`BLOCKED`·`NOT_ASSESSED` 3, 실행 전 거부 2, I/O·publication 4, 취소 130. 의미와 축은 [identity/evidence](identity-and-evidence.md) `S07 구현`이 소유한다.

registration `tsgk-replay/r1`(S02 strict decoder, 모든 필드 필수): `schema`, `id`, `reducer`(등록된 reducer id, 그 밖은 `REDUCER_UNSUPPORTED`/BLOCKED), `operation`(아래 표, reducer의 연산과 같아야 함), `subject`(`run`, `attempt`, `platform`, `commit`, `evidence_mode`, `execution_status`, `assessment`: 기록된 subject run과 그 원 판정), `identities`(역할 → 값, reducer가 관측하는 역할을 빠짐없이 정확히 결속), `records`(`null`이면 reducer가 workload에서 expected record를 정하고, 목록이면 그 순서의 독립 expected 목록), `members`(`path`·`role`·`bytes`·`sha256`: 독립 inventory. 경로는 portable이고 대소문자만 다른 중복은 `MEMBER_DUPLICATE`, 역할은 reducer의 `member_roles` 또는 evidence가 아닌 동반 파일의 `retained`)다. 예시는 `src/contracts/examples/replay-r1.json`, 거부 예시는 `invalid/replay-*.json`이다.

| `operation` | 파일 | 파일당 | 합계 | record 수 | record 하나 | 출력 | wall | host |
|---|---|---|---|---|---|---|---|---|
| `evidence-replay` | 10000 | 16777216 | 268435456 | 100000 | 16777216 | 16777216 | 120초 | 세 OS |
| `private-corpus-replay` | 26000 | 2147483648 | 2147483648 | 26000 | 16777216 | 67108864 | 1800초 | 로컬(비공개 corpus) |

`evidence-replay`는 S07 예산 연산(파일 10000, 파일당 16 MiB, 합계 256 MiB, wall 120초, 출력 16 MiB)이다. record 수 상한은 record 묶음(사례, inventory record, ledger row) 하나에 적용한다. `private-corpus-replay`는 [NET461 등록부](../validation/net461-workload.md)의 S07 비공개 replay 값(파일 26000, record 합계 2 GiB, wall 1800초)이다. 큰 raw는 member 하나를 hash하며 한 번 stream으로 읽고 record 값을 하나씩(`record_bytes` 이하) decode한다. 등록 member가 파일당 한도(필요한 member는 record 한도도)를 넘으면 그 member를 읽지 않고, 다른 실패가 없으면 결과는 `RECORDED_NOT_RECOMPUTED`/`UNRESOLVED`와 `RAW_OVER_LIMIT`이다(exit 3). 그 밖의 한도는 `RESOURCE_LIMIT`(exit 3)이고, `evidence verify`에서 graph 파일이 한도를 넘어도 `RESOURCE_LIMIT`다. memory는 강제하지 않는다(in-process, 위 한도로 묶는다).

policy `tsgk-evidence-policy/r1`(strict, 모르는 필드 거부): `schema`, `id`, `evidence_sha256`(입력 root의 `evidence.json` bytes에 대한 caller 신뢰 anchor), `required`(`node`, `kind`, `identities`), `carry_forward`(`node`, `origin`, `relation`, `authorized_by`), `eligibility`(`null` 또는 `{nodes, modes}`)다. 예시는 `src/contracts/examples/evidence-policy-r1.json`, 거부 예시는 `invalid/evidence-*.json`이다. graph 문서 `tsgk-evidence/r1`은 입력 root의 `evidence.json`이며 node(`id`, `kind`, 세 축, `recorded_assessment`, `identities`, `files`, `refs`)를 담는다. 검사는 `evidence-replay` 한도로 한다.

## S08 구현 — qualify, qualification inventory r3

```text
tsgk qualify --inventory FILE --candidate SHA --host PLATFORM=DIR [--host PLATFORM=DIR ...] [--out PATH]
```

READ_DATA만 쓴다. process·network·tool을 시작하지 않는다. `--inventory`는 caller가 신뢰하는 `tsgk-qualification-inventory/r3`이고 어느 host 디렉터리 안에 있어도 `INVENTORY_INSIDE_INPUT`(exit 2)이다. `--candidate`는 모든 host가 실행해야 하는 commit(40자리 16진)이며 그 밖은 `CANDIDATE_INVALID`(exit 2)다. `--host`는 inventory의 platform id와 그 host의 실행 디렉터리다. `--out`은 S01 publication 규칙을 따르고 어느 host 디렉터리 안이어도 `OUTPUT_INSIDE_INPUT`이다. exit: `PASS` 0(모든 필수 칸·실행된 추가 역할 행 PASS와 `mechanism_gate` PASS, 지원 claim `SUPPORTED`), `FAIL` 1(완결성·cohort·자격·kit 축·비교 실패, 필수 요구·등록 검사 FAIL, 추가 역할 행 FAIL), `BLOCKED` 3(실패는 없고 미충족 의무(`NOT_COVERED`: 사례 없음, production 대안 미완·`PENDING` 행), 등록 검사 BLOCKED, kit 축 BLOCKED, INCOMPLETE 추가 역할 행이 남음), 실행 전 거부 2, I/O·publication 4, 취소 130. 의미와 축은 [identity/evidence](identity-and-evidence.md) `S08 구현`이 소유한다.

host 디렉터리는 `src/dev/s05-native/run-routes.ps1 -Oracle`의 출력에서 CI가 올리는 부분이다: `run-identity.json`(`tsgk-run-identity/r1`, `src/dev/s08-qualify/run-identity.ps1`이 씀), `profiles/<workload>.json`(실행에 쓴 `tsgk-oracle/r2` profile), `records/<set>/`(S06 기록 set), helper의 `summary.json`(보존만 하며 근거가 아님). 그 밖의 파일은 `HOST_FILE_UNREGISTERED`로 완결성을 실패시킨다.

inventory `tsgk-qualification-inventory/r3`(S02 strict decoder, 모르는 필드 거부): `id`, `campaign`, `qualification_cells`(= route 수 × platform 수, 다르면 `CELL_COUNT_MISMATCH`), `coverage_rule`(`tsgk-coverage-rule/r2`), `kinds`(`P N R E Q W`), `platforms`(`id`, `goos`, `goarch`), `routes`(`route`, `error_nodes`, `workload`, `requirements`), `extra_roles`다. r2는 r1에 route의 `error_nodes`, 요구 행의 `alternatives_status`·`alternatives`, case의 `alternatives`·`sample`을 더했고, r3은 case `expect`의 `anchors`(`S05 구현`의 기대값 anchor, byte 범위)를 더한다. 기대값 하나의 anchor가 64개를 넘거나, anchor의 type이 형식에 맞지 않거나, `start_byte > end_byte`이거나, `end_byte`가 그 step source 길이(case `input.bytes`와 `edits`로 계산)를 넘으면 `CASE_INVALID`다. 원본 사례 파일의 `{type, text, occurrence}` anchor를 `run-routes.ps1`과 같은 방식으로 byte 범위로 바꿔 옮기며, host profile의 anchor가 inventory와 다르면 `REGISTRATION_MISMATCH`다. P·N·R·W step 판정은 anchor 검사를 포함한 그 step의 기대값 결과다. `error_nodes`는 그 route 문법이 parse 오류를 ERROR 대신 나타내는 named node type 목록이다(빈 목록 가능, 중복·`ERROR`·이름 형식 위반은 `ROUTE_INVALID`). 요구 행은 `row`, `kinds`, `alternatives_status`(`COMPLETE`·`PENDING`), `alternatives`(`<row>.aNN` id, NN은 두 자리 이상 숫자)이고 `COMPLETE`인데 대안이 없거나 상태·id 형식·중복이 틀리면 `REQUIREMENT_INVALID`다. `workload`는 `set`, `profile`, `route`, `operation`, `output`, `symbol`, `format`, `grammar`, `queries`(id·source sha256), `fact_pack`, `api`, `cases`이고 case는 `id`, `role`(`requirement`·`support`·`detector`), `input`(sha256·bytes), `edits`, `expect`, `query_expect`, `expect_status`(빈 값은 `COMPLETED`), `expect_assessment`·`expect_code`(SVC 관측 전용 사례가 끝나야 할 S05 판정과 code, 빈 값은 등록 없음), `covers`(요구 행 → kind), `alternatives`(사례가 실행하는 production 대안 id), `sample`(W 생산자 등록, 아래), `source`(query 기대값을 다시 계산할 case의 UTF-8 원본)다. `alternatives`의 id는 그 route에 등록된 대안이어야 하고 사례가 그 대안의 행을 P로 덮어야 하며 한 사례 안에서 중복되지 않는다. 그 밖은 `CASE_INVALID`다. `sample`은 `repository`(`owner/name`), `commit`(40자리 16진), `path`(저장소 안 상대 slash 경로), `license`(SPDX id `MIT`·`Apache-2.0`·`BSD-2-Clause`·`BSD-3-Clause`·`PostgreSQL`), `sha256`, `bytes`이고 `requirement` 사례에만 쓸 수 있다. `sha256`·`bytes`는 사례 `input`과 같고 `bytes`는 1..65536이다. 그 밖은 `CASE_INVALID`다. 원본 사례 파일(`src/testdata/native/{routes,gaps,n461}/*.json`)의 같은 이름 필드에서 inventory로 옮기며, sample마다 [`src/testdata/native/samples/NOTICE.md`](../../src/testdata/native/samples/NOTICE.md)에 항목이 있어야 한다(`TestSampleNotices`). 항목은 제목 `## <repository>@<commit>`, 정확한 줄 `Path: <path>`와 `License: <SPDX id>`, 고지 원문 순서이며 경로와 license는 문자열 전체로 대조한다. 비공개 corpus는 W 생산자가 아니다. `expect_assessment`·`expect_code`는 `format`이 `SVC-SERVICEHOST-r1`인 workload에서 `covers`가 없고 tree가 필요 없는(`expect`·`query_expect`·`dynamic_sql_expect` 없음, `expect_status` 빈 값) detector가 아닌 사례에만 쓸 수 있고, `PASS`와 빈 code 또는 `BLOCKED`와 빈 값이 아닌 code의 짝이어야 한다. 그 밖은 `CASE_INVALID`다. 원본 사례 파일 `src/testdata/native/n461/svc.json`의 같은 이름 필드에서 inventory로 옮긴다. `covers`는 아래 규칙을 지켜야 한다(`COVERAGE_RULE_VIOLATION`). detector는 요구 행을 덮지 못한다(`DETECTOR_COVERS`). 두 route가 같은 set을 쓰면 거부한다(SQL dialect 병합 방지). 추가 역할은 `maintained`·`historical`·`owned`이고 상태는 `EXECUTED`(workload 필수)·`NOT_RUN`·`EXTERNAL`, `not_applicable`은 platform별 이유다.

`tsgk-coverage-rule/r2`: 오류 step은 `ERROR`를 기대하는 step이거나, `NO_ERROR`를 기대하면서 `contains`에 route의 `error_nodes` 중 하나를 적은 step이다(html은 `erroneous_end_tag`, `erroneous_comment`). 오류 step의 자기 기대값 판정은 바뀌지 않는다. `NO_ERROR`+`contains` step은 tree에 오류가 없고 적은 node가 모두 있어야 PASS다. 사례 kind는 다음과 같이 정해진다.

* P: `NO_ERROR`와 named 구조(`contains`)를 기대하는 step 중 오류 step이 아닌 것.
* N: 오류 step.
* R: `ERROR`와 named 구조를 기대하는 step, 또는 `contains`에 error node 말고 다른 node type도 적은 `NO_ERROR` 오류 step.
* E: edit 하나 이상(incremental/fresh 비교).
* Q: capture 기대값이 있는 query 사례.
* W: 유효한 `sample`이 있고 step 0이 `NO_ERROR`를 기대하는(오류 step이 아닌) `requirement` 사례. W 판정은 step 0 결과이며, 기록된 step 0 tree의 `descendant_count`가 10000을 넘거나 tree가 없으면 BLOCKED다.

error node를 등록하지 않은 route에서는 같은 `contains`가 N·R을 만들지 않는다.

tree-sitter의 `has_error`는 `erroneous_end_tag` 같은 named 오류 node를 보지 않는다. 그래서 `error_nodes`를 등록한 route에서는 P·W step 판정에 검사 하나를 더한다. 그 step의 기록된 incremental tree는 등록된 error node type의 node를 하나도 갖지 않아야 한다.

* 그런 node가 있으면 그 step의 P·W는 FAIL이다. `NO_ERROR`가 `has_error`로 틀린 경우와 같다.
* 기록이 full tree가 아니어서(요약·record 형식, tree 없음) 부재를 확인할 수 없으면 BLOCKED이며 PASS가 되지 않는다.
* 이 검사는 kind 판정만 바꾼다. 그 step의 등록 기대값 판정(`registered_checks`)은 그대로다.

사례는 `features`에 적힌 그 route의 REQ 행만 덮는다(N461 역할 id와 gap 설명 문구는 행이 아니다). query 사례는 자기 행의 Q만 덮는다.

P 의무는 행의 `alternatives_status`가 `COMPLETE`이고, 행의 모든 대안 id가 그 행을 P로 덮으며 `alternatives`에 그 id를 적은 `requirement` 사례를 하나 이상 가질 때만 덮인다. 그때 결과는 P로 덮는 모든 사례 중 가장 나쁜 것이므로, 대안 사례 하나가 FAIL이면 행이 FAIL이다. 그 밖(`PENDING`이거나 사례 없는 대안이 남음)의 P는 `NOT_COVERED`다. 이때 P를 덮는 사례가 FAIL이어도 의무는 `NOT_COVERED`이고, 그 FAIL은 `registered_checks`로 요구 축을 FAIL로 만든다. 예시 하나로 계열을 완료하지 않는다는 [disposition 완료 경계](../validation/language-feature-disposition.md)를 기계화한 것이다.

추적되는 inventory는 `src/contracts/qualification-c1.json`이다. 다음 입력으로 `run-routes.ps1`과 같은 방식으로 만든다. `TestQualificationInventory`가 다시 만들어 bytes로 대조한다(`TSGK_WRITE_INVENTORY=1`이면 다시 쓴다).

* campaign 정의(26 route, 세 platform).
* [feature disposition](../validation/language-feature-disposition.md)의 REQ 행.
* production 대안 등록부 [`src/contracts/feature-alternatives.json`](../../src/contracts/feature-alternatives.json).
* `native-routes.json`. route별 선택 필드 `error_nodes`도 여기서 온다.
* fact query pack.
* 등록 사례(`src/testdata/native`).

등록부 `tsgk-feature-alternatives/r1`은 `routes.<route>.<row>`마다 `status`(`COMPLETE`·`PENDING`)와 `alternatives`를 가진다. 대안 하나는 `id`(`<row>.aNN`), `kind`(`production`·`fact`), `variant_of`(null 또는 다른 등록 대안 id), `fact`, `production`, `ref`(https URL)다. `TestFeatureAlternatives`가 다음을 검사한다.

* disposition의 REQ 행이 모두 있고 다른 행은 없다.
* id는 전체에서 유일하고 자기 행으로 시작한다.
* `ref`는 https URL이다.
* `variant_of`는 자기 자신이 아닌 등록 id다.
* kind에 맞는 `production` 또는 `fact`가 비어 있지 않다.
* `COMPLETE` 행은 대안이 하나 이상이다.

처음 등록부는 모든 행이 `PENDING`이고 대안이 비어 있으므로 모든 P 의무가 `NOT_COVERED`다.

| `operation` | host당 파일 | 파일당 | host당 합계 | record 수 | 출력 | wall | host |
|---|---|---|---|---|---|---|---|
| `qualification` | 10000 | 67108864 | 536870912 | 100000 | 16777216 | 360초(전체) | 세 OS 근거, 집계는 어디서나 |

`qualification`은 S08이 정한 연산이다. 파일당 64 MiB는 windows 전용 NET461 대용량 기록(최대 56 MiB)을 읽기 위해서이고, host당 합계는 그 host 근거 전체(로컬 windows 약 225 MB)를 한 번 읽는 크기다. 한도를 넘은 member는 읽지 않고 그 set의 kit 축을 `BLOCKED`로 둔다. 칸은 `INCOMPLETE`이고 `mechanism_gate`는 FAIL이다(PASS가 아니다). host당 record 수 한도(`RECORDS_LIMIT`), 파일·byte 한도와 wall은 `RESOURCE_LIMIT`(exit 3)다. host 디렉터리가 없거나 link·special 파일이 있으면 exit 2다. memory는 강제하지 않는다(record 하나씩 decode).

## discovery와 strict profile

zero-config는 알려진 경로의 `grammar.js`, `src/grammar.json`, `src/parser.c`, `src/node-types.json`, scanner, queries, corpus, metadata를 제한 안에서 읽는다. JS를 평가하거나 package lifecycle을 실행하지 않는다. 발견하지 못한 파일은 UNKNOWN/NOT_FOUND다. `tree-sitter.json` 부재만으로 legacy grammar를 잘못됐다고 판정하지 않는다. qualification은 strict profile이 요구한 필수 입력이 없으면 BLOCKED다.

명시 grammar subdirectory와 공유 source를 root 안에서 선택하며 snapshot root와 grammar root를 구분한다. `parser.c`는 native build에만 필요하다. inspect/schema/evidence 비교에서 생성 C 부재를 자동 invalid로 처리하지 않는다. `require()` closure는 실행 없이 완전성을 증명할 수 없으면 미확인으로 남긴다. 알려진 경로·제외·탐색 한도를 결과에 표시하고 숨은 global ignore나 부모 탐색을 사용하지 않는다.

profile 필수 필드는 `schema: tsgk-profile/r0`, `id`(1~128 ASCII 영숫자/._-), `files`, `limits`다. `files`는 `path`(portable root-relative), `role`(grammar/generated/scanner/query/corpus/metadata), `required`(bool)를 가진 항목의 배열이다. 중복 path·unknown role은 오류다. glob/shell hook을 넣지 않고 정렬된 명시 목록으로 고정한다. source root와 output은 profile에 기기 경로로 박지 않고 CLI에서 준다.

`limits`는 양의 정수 `files`, `file_bytes`, `total_bytes`, `json_depth`, `archive_entries`, `output_bytes`다. 기본 상한은 각각 10000, 16777216, 268435456, 64, 10000, 16777216이다. 누적 계산은 overflow를 검사하며 count/bytes를 읽기 전에 가능한 만큼 확인하고 스트리밍 중에도 적용한다. 한도를 올린 profile은 다른 policy identity다. campaign의 identity 한정 예외 `pg-large-source-r1`은 PostgreSQL parser.c 두 identity(97664793 bytes `a9090d5082ae5c23892d05aa59e61476f9bd39ad634228f2046024debdf815b5`, 97664835 bytes `cc47959aac26b9e749883dac2fb2852dcb7efd895d51e4b19d38d4b1a3528f7d`)에만 단일 파일 104857600 bytes를 허용하고 그 밖은 기본값과 `RESOURCE_LIMIT`다. 같은 방식의 `large-parser-source-r1`(사용자 결정 `C1-LARGE-PARSER-SOURCE-R1`)은 upstream `src/parser.c` 네 identity — C# 32021728 bytes `2549deeed0c8aeb84f42f9ccd3cf9de047a0c609387075a97784fddb2d1770cd`, T-SQL 26649584 bytes `869b54a39e38e73da254cece5085e497063e699d30e689e0d20b93e5fb3bc2a2`, C++ 25857209 bytes `0007727b6e1fbc07657b6a4bf5dc39f524c6009fa347f9faaf180fa9ab17e999`, Kotlin 22443237 bytes `9ff65161845b9e9c9d62c12e9a4e4b8d8628bdc31c681ec7e6b4bd3bd6444cb3` — 에만 단일 파일 33554432 bytes를 허용한다. 재생성·patch된 parser.c를 포함한 다른 identity는 기본값과 `RESOURCE_LIMIT`이고, 두 예외 모두 다른 한도를 올리지 않는다. 비공개 corpus 연산 `private-corpus-local`의 한도는 [NET461 등록부](../validation/net461-workload.md)가 정한다. 각 연산의 상한은 [trust 계약](trust-and-execution.md)대로 연산마다 명시한다. runner를 쓰는 native·generator 연산은 S04 runner가 결과의 operation policy identity에 기록하고, 그 밖의 offline 연산은 profile 한도와 report identity에 기록한다. `private-corpus-local`은 S01 inventory 단계를 profile 한도와 report identity에, S05·S08의 native 실행 단계를 S04 runner의 operation policy identity에도 기록한다. 연산의 `max_depth`는 요청/JSON 구조 중첩이며(profile `json_depth`와 별개), tree depth 상한은 [NET461 등록부](../validation/net461-workload.md)의 실사용 source 값(100000)을 따른다. `files[].required=false` 누락은 관측하며 required 누락은 qualification BLOCKED다.

profile과 manifest는 unknown 필드·duplicate key·잘못된 type/version·비유한 숫자·trailing JSON 값을 거부한다. 정수 필드는 소수/지수 표기를 받지 않고 0~2^53-1 범위의 10진 정수(선행 0 없음)로 제한한다. bool/string을 숫자로 바꾸지 않는다. raw hash는 원본 bytes, 의미 비교는 object key order를 무시하되 array order·정수 값을 보존한다. 일반 JSON 결과를 float64로 왕복시키지 않는다. `encoding/json` 기본값은 duplicate key와 unknown key를 엄격하게 거부하지 않으므로 S02는 `encoding/json/jsontext` token 단계와 typed 검사를 함께 쓴다(위 `S02 구현` 절). [공식 JSON 동작](https://pkg.go.dev/encoding/json), [jsontext](https://pkg.go.dev/encoding/json/jsontext)

S04의 executable hash/argv/환경 allowlist/resource policy, S05의 edit record, S06의 native build request, S07의 workload/replay registration은 각각 담당 Issue에서 r0 확장 revision과 negative 예시를 먼저 고정한다. 이 필드는 그 결정 전 실행에 사용하지 않는다. [실사용 source 정책](../validation/net461-workload.md)이 요구하는 encoding 선언(S01)·wall·memory(S04)·full tree gate(S05) 필드도 같은 방식으로 담당 Issue가 먼저 고정한다. [profile 예시](../../src/contracts/examples/profile-r1.json)와 [expected 예시](../../src/contracts/examples/expected-r1.json)는 시험이 실제로 decode하고, `src/contracts/examples/invalid/`의 각 예시는 파일 이름의 code로 거부되는지 시험한다(`TestContractExamples`). r0 예시는 `invalid/profile-SCHEMA_UNSUPPORTED.json`으로 옮겼다.
