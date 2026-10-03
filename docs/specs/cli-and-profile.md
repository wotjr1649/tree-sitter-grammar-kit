# CLI/report r1과 profile r0 — S01 구현과 후속 계약

S01은 `inspect`, `identity`, `corpus` 명령을 구현했다(아래 `S01 구현` 절). 나머지 명령과 profile r0는 담당 Session이 구현하기 전의 계약이다. r0는 두 번째 grammar와 실제 consumer를 통해 재검토할 experimental 형식이다.

## 공통 입출력과 오류

root에서 실행할 CLI는 `tsgk <command>`다. 읽기 대상은 명시적 `--root PATH`(기본 현재 디렉터리), JSON 결과는 stdout, 진단은 stderr다. `--out PATH`는 새 파일만 허용한다. 기존 파일·입력 집합 내부·link 경로는 거부한다. caller-owned 같은 filesystem의 임시 파일을 완성한 뒤 충돌 없이 publish하고, 실패 partial은 별도 failure evidence로 보존한다. 결과 쓰기 실패는 성공 결과로 바꾸지 않는다.

exit `0`은 해당 명령이 요청한 범위의 PASS/관측 완료, `1`은 유효한 입력에서 검증 FAIL, `2`는 인자·profile·형식 오류, `3`은 필수 근거/권한/capability가 없어 BLOCKED, `4`는 I/O·tool·내부 실행 오류, `130`은 취소다. unknown layout을 관측한 inspect의 exit 0은 qualification PASS를 뜻하지 않는다. 결과 envelope는 `schema: tsgk-report/r1`, `command`, `execution_status`, `evidence_mode`, `assessment`, `identities`, `findings`, `coverage`를 가진다. finding은 안정적인 `code`, `severity`, root-relative `path`, 한국어 `message`로 표현한다. OS 절대 경로와 환경값은 기본 결과에서 제외한다.

S01부터 [공개 offline API](public-go-api.md)와 같은 operation/guard/E0를 사용한다. E0 r1의 추가 필드와 축은 [identity/evidence](identity-and-evidence.md)가 소유한다. resource limit은 `execution_status=RESOURCE_LIMIT`, `assessment=BLOCKED`, exit 3이다. 인자/형식 오류는 실행 전 exit 2, 시작 후 취소는 130, I/O/tool/protocol/publication 실패는 4, 완료된 검증 불일치는 1의 순서로 실제 원인을 기록한다. 여러 원인이 생기면 모두 보존하고 성공 publication 실패를 earlier PASS로 덮지 않는다. stdout의 partial stream은 rollback할 수 없으며 완전한 최종 report가 아니면 성공으로 소비하지 않는다.

| 명령 | 입력·출력 | capability / 소유 Session |
|---|---|---|
| `inspect --root PATH` | zero-config inventory, metadata/scanner/query/corpus 발견·미확인 사실 | READ_DATA, 01 |
| `identity --root PATH [--profile FILE]` | 선택 파일 집합의 manifest r2 | READ_DATA, 01; profile은 02 |
| `corpus --root PATH` | `private-corpus-local` inventory(역할·PRESENCE_ONLY·encoding·`.csproj` 선언 관측) | READ_DATA, 01 |
| `verify --root PATH --expected FILE [--profile FILE]` | 외부 신뢰 expected와 exact set 대조 | READ_DATA, 02 |
| `schema check --input FILE` | node-types 구조·참조 검증 | READ_DATA, 03 |
| `schema diff --before FILE --after FILE` | node/field/type/required/multiple/supertype 변경 | READ_DATA, 03 |
| `reproduce --root PATH --profile FILE --out PATH` | 두 독립 생성과 기준 생성물 비교 | EXEC_GENERATOR + WRITE_RESULT, 04 |
| `incremental --root PATH --edits FILE --profile FILE --out PATH` | 매 edit의 incremental/fresh 결과 | BUILD_NATIVE + EXEC_NATIVE + WRITE_RESULT, 05 |
| `oracle record --root PATH --profile FILE --out PATH` | native ordered tree/query/API record | BUILD_NATIVE + EXEC_NATIVE + WRITE_RESULT, 06 |
| `replay --input PATH --profile FILE` | 등록된 data-only reducer로 raw의 현재 판정 | READ_DATA, 07; 외부 verifier 진단은 별도 EXEC_ADAPTER |
| `evidence verify --input PATH --profile FILE` | envelope/참조/승계 검증 | READ_DATA, 07 |
| `parity --left PATH --right PATH --profile FILE` | 동일 의미 identity의 결과 대조 | READ_DATA, 08 |

명령은 network·tool 설치를 묵시적으로 하지 않는다. `--allow CAPABILITY`를 반복 지정해 실행 권한을 전달하되 profile 요청과 실제 backend가 모두 충족되어야 한다. fetch 준비는 별도 명시 절차이고 위 offline 명령에 자동 fetch 옵션을 숨기지 않는다. `--git-provenance` 같은 외부 Git 호출은 S01에서 별도 opt-in 계약·권한을 먼저 확정하기 전 구현하지 않는다.

## S01 구현

```text
tsgk inspect  --root PATH [--grammar DIR] [--out PATH]
tsgk identity --root PATH [--grammar DIR] [--file PATH=ROLE ...] [--encoding-profile cp949|none] [--declare PATH=utf-8|cp949 ...] [--large-file-profile pg-large-source-r1] [--out PATH]
tsgk corpus   --root PATH [--encoding-profile cp949|none] [--declare PATH=utf-8|cp949 ...] [--out PATH]
```

* `--root`의 기본값은 현재 디렉터리이고 `--grammar`의 기본값은 root sentinel `.`이다. `--file`과 `--declare`의 `PATH=VALUE`는 마지막 `=`에서 나누므로 path에 `=`가 있어도 된다. 부모 탐색은 없다. `--file`을 하나라도 주면 discovery 대신 그 목록만 선택한다.
* `--encoding-profile`은 profile 단위 cp949 선언이다. identity의 기본값은 선언 없음, corpus의 기본값은 `cp949`([NET461 등록부](../validation/net461-workload.md)의 corpus profile)다. `--declare`는 파일별 선언이며 사용자가 제공한 로컬 manifest의 값을 결과 관측 전에 옮길 때만 쓴다. 선택되지 않은 path의 선언은 오류다. strict profile 해석은 S02이므로 `--profile`은 exit 2로 거부한다.
* `verify`, `schema`, `reproduce`, `incremental`, `oracle`, `replay`, `evidence`, `parity`는 담당 Session 전까지 exit 2와 `UNSUPPORTED_COMMAND`로 거부한다. 가짜 성공은 없다.
* CLI 기본 한도는 offline-inspect의 files 10000, file_bytes 16777216, total_bytes 268435456, depth 64, output_bytes 16777216, wall 120초이고, corpus는 아래 private-corpus-local 값이다. CLI는 caller deadline을 wall+5초로 두므로 kit wall이 먼저 `RESOURCE_LIMIT`으로 끝나고, Ctrl-C 같은 caller 취소만 130이다.
* 종료 코드: 완료 0, `INVALID_INPUT` 2, `RESOURCE_LIMIT`·`UNSUPPORTED` 3, `IO`와 publication 실패 4, `CANCELLED` 130. inspect/identity/corpus는 비교를 하지 않으므로 1을 쓰지 않는다.
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

## discovery와 strict profile

zero-config는 알려진 경로의 `grammar.js`, `src/grammar.json`, `src/parser.c`, `src/node-types.json`, scanner, queries, corpus, metadata를 제한 안에서 읽는다. JS를 평가하거나 package lifecycle을 실행하지 않는다. 발견하지 못한 파일은 UNKNOWN/NOT_FOUND다. `tree-sitter.json` 부재만으로 legacy grammar를 잘못됐다고 판정하지 않는다. qualification은 strict profile이 요구한 필수 입력이 없으면 BLOCKED다.

명시 grammar subdirectory와 공유 source를 root 안에서 선택하며 snapshot root와 grammar root를 구분한다. `parser.c`는 native build에만 필요하다. inspect/schema/evidence 비교에서 생성 C 부재를 자동 invalid로 처리하지 않는다. `require()` closure는 실행 없이 완전성을 증명할 수 없으면 미확인으로 남긴다. 알려진 경로·제외·탐색 한도를 결과에 표시하고 숨은 global ignore나 부모 탐색을 사용하지 않는다.

profile 필수 필드는 `schema: tsgk-profile/r0`, `id`(1~128 ASCII 영숫자/._-), `files`, `limits`다. `files`는 `path`(portable root-relative), `role`(grammar/generated/scanner/query/corpus/metadata), `required`(bool)를 가진 항목의 배열이다. 중복 path·unknown role은 오류다. glob/shell hook을 넣지 않고 정렬된 명시 목록으로 고정한다. source root와 output은 profile에 기기 경로로 박지 않고 CLI에서 준다.

`limits`는 양의 정수 `files`, `file_bytes`, `total_bytes`, `json_depth`, `archive_entries`, `output_bytes`다. 기본 상한은 각각 10000, 16777216, 268435456, 64, 10000, 16777216이다. 누적 계산은 overflow를 검사하며 count/bytes를 읽기 전에 가능한 만큼 확인하고 스트리밍 중에도 적용한다. 한도를 올린 profile은 다른 policy identity다. campaign의 identity 한정 예외 `pg-large-source-r1`은 PostgreSQL parser.c 두 identity(97664793 bytes `a9090d5082ae5c23892d05aa59e61476f9bd39ad634228f2046024debdf815b5`, 97664835 bytes `cc47959aac26b9e749883dac2fb2852dcb7efd895d51e4b19d38d4b1a3528f7d`)에만 단일 파일 104857600 bytes를 허용하고 그 밖은 기본값과 `RESOURCE_LIMIT`다. 비공개 corpus 연산 `private-corpus-local`의 한도는 [NET461 등록부](../validation/net461-workload.md)가 정한다. 각 연산의 상한은 [trust 계약](trust-and-execution.md)대로 연산마다 명시한다. runner를 쓰는 native·generator 연산은 S04 runner가 결과의 operation policy identity에 기록하고, 그 밖의 offline 연산은 profile 한도와 report identity에 기록한다. `private-corpus-local`은 S01 inventory 단계를 profile 한도와 report identity에, S05·S08의 native 실행 단계를 S04 runner의 operation policy identity에도 기록한다. 연산의 `max_depth`는 요청/JSON 구조 중첩이며(profile `json_depth`와 별개), tree depth 상한은 [NET461 등록부](../validation/net461-workload.md)의 실사용 source 값(100000)을 따른다. `files[].required=false` 누락은 관측하며 required 누락은 qualification BLOCKED다.

profile과 manifest는 unknown 필드·duplicate key·잘못된 type/version·비유한 숫자·trailing JSON 값을 거부한다. 정수 필드는 소수/지수 표기를 받지 않고 0~2^53-1 범위의 10진 정수(선행 0 없음)로 제한한다. bool/string을 숫자로 바꾸지 않는다. raw hash는 원본 bytes, 의미 비교는 object key order를 무시하되 array order·정수 값을 보존한다. 일반 JSON 결과를 float64로 왕복시키지 않는다. `encoding/json` 기본값은 duplicate key와 unknown key를 엄격하게 거부하지 않으므로 S02에서 token 단계와 typed decode를 함께 검증한다. [공식 JSON 동작](https://pkg.go.dev/encoding/json)

S04의 executable hash/argv/환경 allowlist/resource policy, S05의 edit record, S06의 native build request, S07의 workload/replay registration은 각각 담당 Issue에서 r0 확장 revision과 negative 예시를 먼저 고정한다. 이 필드는 그 결정 전 실행에 사용하지 않는다. [실사용 source 정책](../validation/net461-workload.md)이 요구하는 encoding 선언(S01)·wall·memory(S04)·full tree gate(S05) 필드도 같은 방식으로 담당 Issue가 먼저 고정한다. 최소 [profile 예시](../../src/contracts/examples/profile-r0.json)는 데이터 예시이며 validator 구현이 아니다.
