# CLI/report r1과 profile r0 — S01 구현과 후속 계약

S01은 `inspect`, `identity`, `corpus` 명령을 구현했고(아래 `S01 구현` 절), S02는 `verify`와 strict profile `tsgk-profile/r1`, expected `tsgk-expected/r1`을 구현했다(아래 `S02 구현` 절). S04는 `reproduce`와 `tsgk-reproduce/r1`을 구현했다(아래 `S04 구현` 절). 나머지 명령은 담당 Session이 구현하기 전의 계약이다. 아래 profile r0 서술은 구현되지 않은 초안으로 보존하며 `S02 구현` 절과 충돌하면 그 절이 우선한다.

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
| `incremental --root PATH --edits FILE --profile FILE --out PATH` | 매 edit의 incremental/fresh 결과 | BUILD_NATIVE + EXEC_NATIVE + WRITE_RESULT, 05 |
| `oracle record --root PATH --profile FILE --out PATH` | native ordered tree/query/API record | BUILD_NATIVE + EXEC_NATIVE + WRITE_RESULT, 06 |
| `replay --input PATH --profile FILE` | 등록된 data-only reducer로 raw의 현재 판정 | READ_DATA, 07; 외부 verifier 진단은 별도 EXEC_ADAPTER |
| `evidence verify --input PATH --profile FILE` | envelope/참조/승계 검증 | READ_DATA, 07 |
| `parity --left PATH --right PATH --profile FILE` | 동일 의미 identity의 결과 대조 | READ_DATA, 08 |

명령은 network·tool 설치를 묵시적으로 하지 않는다. `--allow CAPABILITY`를 반복 지정해 실행 권한을 전달하되 profile 요청과 실제 backend가 모두 충족되어야 한다. fetch 준비는 별도 명시 절차이고 위 offline 명령에 자동 fetch 옵션을 숨기지 않는다. `--git-provenance` 같은 외부 Git 호출은 S01에서 별도 opt-in 계약·권한을 먼저 확정하기 전 구현하지 않는다.

## S01 구현

```text
tsgk inspect  --root PATH [--grammar DIR] [--out PATH]
tsgk identity --root PATH [--grammar DIR] [--file PATH=ROLE ...] [--encoding-profile cp949|none] [--declare PATH=utf-8|cp949 ...] [--large-file-profile pg-large-source-r1|large-parser-source-r1] [--profile FILE] [--out PATH]
tsgk corpus   --root PATH [--encoding-profile cp949|none] [--declare PATH=utf-8|cp949 ...] [--profile FILE] [--out PATH]
```

* `--root`의 기본값은 현재 디렉터리이고 `--grammar`의 기본값은 root sentinel `.`이다. `--file`과 `--declare`의 `PATH=VALUE`는 마지막 `=`에서 나누므로 path에 `=`가 있어도 된다. 부모 탐색은 없다. `--file`을 하나라도 주면 discovery 대신 그 목록만 선택한다.
* `--encoding-profile`은 profile 단위 cp949 선언이다. identity의 기본값은 선언 없음, corpus의 기본값은 `cp949`([NET461 등록부](../validation/net461-workload.md)의 corpus profile)다. `--declare`는 파일별 선언이며 사용자가 제공한 로컬 manifest의 값을 결과 관측 전에 옮길 때만 쓴다. 선택되지 않은 path의 선언은 오류다. `--profile`은 아래 `S02 구현` 절의 profile r1 규칙을 따른다.
* `incremental`, `oracle`, `replay`, `evidence`, `parity`는 담당 Session 전까지 exit 2와 `UNSUPPORTED_COMMAND`로 거부한다. 가짜 성공은 없다.
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

## discovery와 strict profile

zero-config는 알려진 경로의 `grammar.js`, `src/grammar.json`, `src/parser.c`, `src/node-types.json`, scanner, queries, corpus, metadata를 제한 안에서 읽는다. JS를 평가하거나 package lifecycle을 실행하지 않는다. 발견하지 못한 파일은 UNKNOWN/NOT_FOUND다. `tree-sitter.json` 부재만으로 legacy grammar를 잘못됐다고 판정하지 않는다. qualification은 strict profile이 요구한 필수 입력이 없으면 BLOCKED다.

명시 grammar subdirectory와 공유 source를 root 안에서 선택하며 snapshot root와 grammar root를 구분한다. `parser.c`는 native build에만 필요하다. inspect/schema/evidence 비교에서 생성 C 부재를 자동 invalid로 처리하지 않는다. `require()` closure는 실행 없이 완전성을 증명할 수 없으면 미확인으로 남긴다. 알려진 경로·제외·탐색 한도를 결과에 표시하고 숨은 global ignore나 부모 탐색을 사용하지 않는다.

profile 필수 필드는 `schema: tsgk-profile/r0`, `id`(1~128 ASCII 영숫자/._-), `files`, `limits`다. `files`는 `path`(portable root-relative), `role`(grammar/generated/scanner/query/corpus/metadata), `required`(bool)를 가진 항목의 배열이다. 중복 path·unknown role은 오류다. glob/shell hook을 넣지 않고 정렬된 명시 목록으로 고정한다. source root와 output은 profile에 기기 경로로 박지 않고 CLI에서 준다.

`limits`는 양의 정수 `files`, `file_bytes`, `total_bytes`, `json_depth`, `archive_entries`, `output_bytes`다. 기본 상한은 각각 10000, 16777216, 268435456, 64, 10000, 16777216이다. 누적 계산은 overflow를 검사하며 count/bytes를 읽기 전에 가능한 만큼 확인하고 스트리밍 중에도 적용한다. 한도를 올린 profile은 다른 policy identity다. campaign의 identity 한정 예외 `pg-large-source-r1`은 PostgreSQL parser.c 두 identity(97664793 bytes `a9090d5082ae5c23892d05aa59e61476f9bd39ad634228f2046024debdf815b5`, 97664835 bytes `cc47959aac26b9e749883dac2fb2852dcb7efd895d51e4b19d38d4b1a3528f7d`)에만 단일 파일 104857600 bytes를 허용하고 그 밖은 기본값과 `RESOURCE_LIMIT`다. 같은 방식의 `large-parser-source-r1`(사용자 결정 `C1-LARGE-PARSER-SOURCE-R1`)은 upstream `src/parser.c` 네 identity — C# 32021728 bytes `2549deeed0c8aeb84f42f9ccd3cf9de047a0c609387075a97784fddb2d1770cd`, T-SQL 26649584 bytes `869b54a39e38e73da254cece5085e497063e699d30e689e0d20b93e5fb3bc2a2`, C++ 25857209 bytes `0007727b6e1fbc07657b6a4bf5dc39f524c6009fa347f9faaf180fa9ab17e999`, Kotlin 22443237 bytes `9ff65161845b9e9c9d62c12e9a4e4b8d8628bdc31c681ec7e6b4bd3bd6444cb3` — 에만 단일 파일 33554432 bytes를 허용한다. 재생성·patch된 parser.c를 포함한 다른 identity는 기본값과 `RESOURCE_LIMIT`이고, 두 예외 모두 다른 한도를 올리지 않는다. 비공개 corpus 연산 `private-corpus-local`의 한도는 [NET461 등록부](../validation/net461-workload.md)가 정한다. 각 연산의 상한은 [trust 계약](trust-and-execution.md)대로 연산마다 명시한다. runner를 쓰는 native·generator 연산은 S04 runner가 결과의 operation policy identity에 기록하고, 그 밖의 offline 연산은 profile 한도와 report identity에 기록한다. `private-corpus-local`은 S01 inventory 단계를 profile 한도와 report identity에, S05·S08의 native 실행 단계를 S04 runner의 operation policy identity에도 기록한다. 연산의 `max_depth`는 요청/JSON 구조 중첩이며(profile `json_depth`와 별개), tree depth 상한은 [NET461 등록부](../validation/net461-workload.md)의 실사용 source 값(100000)을 따른다. `files[].required=false` 누락은 관측하며 required 누락은 qualification BLOCKED다.

profile과 manifest는 unknown 필드·duplicate key·잘못된 type/version·비유한 숫자·trailing JSON 값을 거부한다. 정수 필드는 소수/지수 표기를 받지 않고 0~2^53-1 범위의 10진 정수(선행 0 없음)로 제한한다. bool/string을 숫자로 바꾸지 않는다. raw hash는 원본 bytes, 의미 비교는 object key order를 무시하되 array order·정수 값을 보존한다. 일반 JSON 결과를 float64로 왕복시키지 않는다. `encoding/json` 기본값은 duplicate key와 unknown key를 엄격하게 거부하지 않으므로 S02는 `encoding/json/jsontext` token 단계와 typed 검사를 함께 쓴다(위 `S02 구현` 절). [공식 JSON 동작](https://pkg.go.dev/encoding/json), [jsontext](https://pkg.go.dev/encoding/json/jsontext)

S04의 executable hash/argv/환경 allowlist/resource policy, S05의 edit record, S06의 native build request, S07의 workload/replay registration은 각각 담당 Issue에서 r0 확장 revision과 negative 예시를 먼저 고정한다. 이 필드는 그 결정 전 실행에 사용하지 않는다. [실사용 source 정책](../validation/net461-workload.md)이 요구하는 encoding 선언(S01)·wall·memory(S04)·full tree gate(S05) 필드도 같은 방식으로 담당 Issue가 먼저 고정한다. [profile 예시](../../src/contracts/examples/profile-r1.json)와 [expected 예시](../../src/contracts/examples/expected-r1.json)는 시험이 실제로 decode하고, `src/contracts/examples/invalid/`의 각 예시는 파일 이름의 code로 거부되는지 시험한다(`TestContractExamples`). r0 예시는 `invalid/profile-SCHEMA_UNSUPPORTED.json`으로 옮겼다.
