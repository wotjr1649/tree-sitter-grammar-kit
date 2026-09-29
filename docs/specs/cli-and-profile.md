# CLI/report r1과 profile r0 — 구현 전 계약

아래는 구현을 위한 계약이다. 현재 실행 가능한 제품 명령은 없다. r0는 두 번째 grammar와 실제 consumer를 통해 재검토할 experimental 형식이다.

## 공통 입출력과 오류

root에서 실행할 CLI는 `tsgk <command>`다. 읽기 대상은 명시적 `--root PATH`(기본 현재 디렉터리), JSON 결과는 stdout, 진단은 stderr다. `--out PATH`는 새 파일만 허용한다. 기존 파일·입력 집합 내부·link 경로는 거부한다. caller-owned 같은 filesystem의 임시 파일을 완성한 뒤 충돌 없이 publish하고, 실패 partial은 별도 failure evidence로 보존한다. 결과 쓰기 실패는 성공 결과로 바꾸지 않는다.

exit `0`은 해당 명령이 요청한 범위의 PASS/관측 완료, `1`은 유효한 입력에서 검증 FAIL, `2`는 인자·profile·형식 오류, `3`은 필수 근거/권한/capability가 없어 BLOCKED, `4`는 I/O·tool·내부 실행 오류, `130`은 취소다. unknown layout을 관측한 inspect의 exit 0은 qualification PASS를 뜻하지 않는다. 결과 envelope는 `schema: tsgk-report/r1`, `command`, `execution_status`, `evidence_mode`, `assessment`, `identities`, `findings`, `coverage`를 가진다. finding은 안정적인 `code`, `severity`, root-relative `path`, 한국어 `message`로 표현한다. OS 절대 경로와 환경값은 기본 결과에서 제외한다.

S01부터 [공개 offline API](public-go-api.md)와 같은 operation/guard/E0를 사용한다. E0 r1의 추가 필드와 축은 [identity/evidence](identity-and-evidence.md)가 소유한다. resource limit은 `execution_status=RESOURCE_LIMIT`, `assessment=BLOCKED`, exit 3이다. 인자/형식 오류는 실행 전 exit 2, 시작 후 취소는 130, I/O/tool/protocol/publication 실패는 4, 완료된 검증 불일치는 1의 순서로 실제 원인을 기록한다. 여러 원인이 생기면 모두 보존하고 성공 publication 실패를 earlier PASS로 덮지 않는다. stdout의 partial stream은 rollback할 수 없으며 완전한 최종 report가 아니면 성공으로 소비하지 않는다.

| 명령 | 입력·출력 | capability / 소유 Session |
|---|---|---|
| `inspect --root PATH` | zero-config inventory, metadata/scanner/query/corpus 발견·미확인 사실 | READ_DATA, 01 |
| `identity --root PATH [--profile FILE]` | 선택 파일 집합의 manifest r1 | READ_DATA, 01; profile은 02 |
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

## discovery와 strict profile

zero-config는 알려진 경로의 `grammar.js`, `src/grammar.json`, `src/parser.c`, `src/node-types.json`, scanner, queries, corpus, metadata를 제한 안에서 읽는다. JS를 평가하거나 package lifecycle을 실행하지 않는다. 발견하지 못한 파일은 UNKNOWN/NOT_FOUND다. `tree-sitter.json` 부재만으로 legacy grammar를 잘못됐다고 판정하지 않는다. qualification은 strict profile이 요구한 필수 입력이 없으면 BLOCKED다.

명시 grammar subdirectory와 공유 source를 root 안에서 선택하며 snapshot root와 grammar root를 구분한다. `parser.c`는 native build에만 필요하다. inspect/schema/evidence 비교에서 생성 C 부재를 자동 invalid로 처리하지 않는다. `require()` closure는 실행 없이 완전성을 증명할 수 없으면 미확인으로 남긴다. 알려진 경로·제외·탐색 한도를 결과에 표시하고 숨은 global ignore나 부모 탐색을 사용하지 않는다.

profile 필수 필드는 `schema: tsgk-profile/r0`, `id`(1~128 ASCII 영숫자/._-), `files`, `limits`다. `files`는 `path`(portable root-relative), `role`(grammar/generated/scanner/query/corpus/metadata), `required`(bool)를 가진 항목의 배열이다. 중복 path·unknown role은 오류다. glob/shell hook을 넣지 않고 정렬된 명시 목록으로 고정한다. source root와 output은 profile에 기기 경로로 박지 않고 CLI에서 준다.

`limits`는 양의 정수 `files`, `file_bytes`, `total_bytes`, `json_depth`, `archive_entries`, `output_bytes`다. 기본 상한은 각각 10000, 16777216, 268435456, 64, 10000, 16777216이다. 누적 계산은 overflow를 검사하며 count/bytes를 읽기 전에 가능한 만큼 확인하고 스트리밍 중에도 적용한다. 한도를 올린 profile은 다른 policy identity다. `files[].required=false` 누락은 관측하며 required 누락은 qualification BLOCKED다.

profile과 manifest는 unknown 필드·duplicate key·잘못된 type/version·비유한 숫자·trailing JSON 값을 거부한다. 정수 필드는 소수/지수 표기를 받지 않고 0~2^53-1 범위의 10진 정수(선행 0 없음)로 제한한다. bool/string을 숫자로 바꾸지 않는다. raw hash는 원본 bytes, 의미 비교는 object key order를 무시하되 array order·정수 값을 보존한다. 일반 JSON 결과를 float64로 왕복시키지 않는다. `encoding/json` 기본값은 duplicate key와 unknown key를 엄격하게 거부하지 않으므로 S02에서 token 단계와 typed decode를 함께 검증한다. [공식 JSON 동작](https://pkg.go.dev/encoding/json)

S04의 executable hash/argv/환경 allowlist/resource policy, S05의 edit record, S06의 native build request, S07의 workload/replay registration은 각각 담당 Issue에서 r0 확장 revision과 negative 예시를 먼저 고정한다. 이 필드는 그 결정 전 실행에 사용하지 않는다. 최소 [profile 예시](../../src/contracts/examples/profile-r0.json)는 데이터 예시이며 validator 구현이 아니다.
