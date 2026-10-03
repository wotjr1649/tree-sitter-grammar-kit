# Tree와 adapter — draft/r0

등록된 `.svc`는 [SVC-SERVICEHOST-r1](../validation/net461-workload.md)의 composite result를 사용한다. directive 관측과 원본 bytes를 included range로 파싱한 inline C# tree를 원본 identity·byte/point로 결속하며 segment PASS를 전체 파일 PASS로 바꾸지 않는다. 이 계약은 계획이며 format 구현은 NOT_RUN이다.

S03의 정적 node-types 비교와 S05부터 생성하는 runtime CST는 다른 주장이다. schema 일치, ordered CST 일치, query 일치, 언어 사양 적합성은 서로 대체하지 않는다. 아래 형식은 experimental이며 실제 native/두 번째 grammar/consumer 검증 전에 안정 API로 고정하지 않는다.

## 정적 node-types 계약 — S03 구현

`node-types-r1`은 Tree-sitter `node-types.json`을 읽는 채택 규칙이다. 근거는 [공식 Static Node Types 문서](https://tree-sitter.github.io/tree-sitter/using-parsers/6-static-node-types.html)와 generator의 [node_types.rs](https://github.com/tree-sitter/tree-sitter/blob/master/crates/generate/src/node_types.rs)(2026-10-03 확인), 그리고 등록 26 route schema의 실제 형태다. `node-types-diff-r1`은 baseline → candidate 방향의 차이 모델이다. 두 연산은 parser·`parser.c`·compiler·process·network를 쓰지 않는 정적 분석이며 runtime tree 일치, 언어 정확성, API 호환 인증, SemVer 판정이 아니다.

**형식 검사.** 문서는 공유 strict JSON decoder를 거친다(중복·escape 동등 key, 잘못된 UTF-8, trailing 값은 각 `JSON_*`). 아래 위반은 모두 finding으로 모으고 첫 위반에서 멈추지 않는다.

* 최상위는 비어 있지 않은 node 배열이다. `[]`은 유효한 빈 schema가 아니라 `SCHEMA_EMPTY`, `null`은 `JSON_NULL`, 다른 값은 `JSON_TYPE`이다. 형식이 허용하지 않는 `null`은 어디서든 없는 값으로 보지 않고 `JSON_NULL`이다.
* node identity는 `(type, named)`이다. `type`은 비어 있지 않은 문자열(`NODE_TYPE_EMPTY`), `named`는 boolean이고 둘 다 필수다(`JSON_MISSING_FIELD`). 같은 spelling이라도 named가 다르면 다른 node다. 같은 identity가 두 번 나오면 모양이 달라도 `NODE_DUPLICATE`다(공식 문서의 유일성 규칙).
* 선택 key는 `root`, `extra`(boolean), `fields`(field 이름 → set object), `children`(set), `subtypes`(참조 배열)뿐이다. 다른 key는 이 revision이 해석하지 않는 형식 확장이므로 `SCHEMA_KEY_UNSUPPORTED`다. 없는 `root`/`extra`는 선언 없음으로 보존하며 `false`와 구분한다. `fields`가 없는 것(token/leaf)과 `fields: {}`(field 없는 nonterminal)는 다른 계약이다. field 이름은 비어 있지 않다(`FIELD_NAME_EMPTY`).
* set은 `multiple`, `required`(boolean), `types`(참조 배열)를 모두 가진다. `types`와 `subtypes`는 비어 있지 않고(`TYPES_EMPTY`, `SUBTYPES_EMPTY`), 참조는 `{type, named}`만 가지며, 한 목록 안의 같은 참조는 합치지 않고 `MEMBER_DUPLICATE`다. `types`·`subtypes`·`fields`는 순서 없는 집합이고, 결과의 정렬은 보고용일 뿐 의미 비교와 별개다.
* supertype(`subtypes`를 가진 node)은 `fields`·`children`을 가질 수 없다(`SUPERTYPE_SHAPE`). `subtypes` 참조는 선언된 node여야 한다(`REFERENCE_UNRESOLVED`). 중첩 supertype은 허용하고, supertype → subtype 그래프의 순환은 `SUPERTYPE_CYCLE`이다. 그래프는 각 node를 한 번만 방문하는 반복 DFS로 걷는다. `root: true`가 둘 이상이면 `ROOT_MULTIPLE`이다.
* field·children의 허용 type이 별도 node로 선언되지 않은 것은 형식 위반이 아니라 `REFERENCE_UNDECLARED` warning이다. generator가 alias를 이렇게 내보내고(등록 python schema의 `as_pattern_target`), 이름에 1:1 node가 없다는 이유만으로 정당한 grammar 관례를 거부하지 않는다.

`schema check` 판정: error finding 중 위반이 하나라도 있으면 `FAIL`, 해석하지 않는 key(`SCHEMA_KEY_UNSUPPORTED`)만 있으면 `BLOCKED`, 그 밖은 `PASS`(warning 허용)이며 셋 다 `execution_status: COMPLETED`다. 항상 `SCHEMA_STATIC_ONLY` info finding을 낸다. `counts`(node·named·anonymous·supertype·field·참조 수, root 목록)는 PASS일 때만 있고 그 밖은 `null`이다. finding path는 `이름#JSON pointer`다.

**차이 모델.** `schema diff`는 두 입력이 모두 PASS일 때만 비교한다. 그렇지 않으면 그 입력의 finding(path 앞에 `baseline:`/`candidate:`)을 보존하고 `INVALID_INPUT`/`SCHEMA_INVALID` 또는 `UNSUPPORTED`/`SCHEMA_KEY_UNSUPPORTED`로 끝나며 부분 비교는 없다. 차이가 없으면 `PASS`(등록 정적 계약이 같다), 있으면 `FAIL`이다. FAIL은 결함 판정이 아니며 항상 `SCHEMA_DIFF_DESCRIPTIVE` info finding을 낸다.

| code | 의미 | risk |
|---|---|---|
| `NODE_ADDED` / `NODE_REMOVED` | identity가 한쪽에만 있다 | `ADDITION` / `REMOVAL` |
| `NODE_NAMED_CHANGED` | 같은 spelling의 identity가 baseline에서 정확히 하나 사라지고 candidate에 정확히 하나 생겼다. 이때는 제거·추가 대신 이 하나만 낸다 | `IDENTITY` |
| `ROOT_CHANGED`, `EXTRA_CHANGED` | `root`·`extra`의 값 또는 선언 유무 변화 | `IDENTITY`, `CLASSIFICATION` |
| `FIELDS_PRESENCE_CHANGED` | `fields` key 유무 변화(leaf ↔ nonterminal) | `IDENTITY` |
| `FIELD_ADDED` / `FIELD_REMOVED` | field가 한쪽에만 있다 | `ADDITION` / `REMOVAL` |
| `FIELD_REQUIRED_CHANGED`, `FIELD_MULTIPLE_CHANGED` | 양쪽에 있는 field의 cardinality 변화 | 새 값이 `required: true` 또는 `multiple: false`이면 `CARDINALITY_NARROWED`, 아니면 `CARDINALITY_WIDENED` |
| `FIELD_TYPE_ADDED` / `FIELD_TYPE_REMOVED` | field 허용 type 집합의 원소 변화 | `ADDITION` / `REMOVAL` |
| `CHILDREN_PRESENCE_CHANGED` | `children` 유무 변화 | 생기면 `ADDITION`, 없어지면 `REMOVAL` |
| `CHILDREN_REQUIRED_CHANGED`, `CHILDREN_MULTIPLE_CHANGED`, `CHILDREN_TYPE_ADDED` / `_REMOVED` | field와 같은 규칙 | field와 같음 |
| `SUBTYPES_PRESENCE_CHANGED`, `SUBTYPE_ADDED` / `SUBTYPE_REMOVED` | supertype 여부와 subtype 집합 변화 | `ADDITION` / `REMOVAL` |

없는 `fields`·`children`·`subtypes`는 원소 비교에서 빈 집합이고 그 유무 변화는 위 `*_PRESENCE_CHANGED`로 따로 낸다. cardinality는 양쪽에 set이 있을 때만 비교한다. 다른 spelling 사이의 rename은 추정하지 않으며 `NODE_REMOVED`와 `NODE_ADDED`로 나타난다. field가 supertype을 가리킬 때 subtype 변화는 그 supertype의 `SUBTYPE_*`로만 보고하고 field의 실효 type 집합으로 펼치지 않는다(coverage `unsupported`의 `supertype-expansion`). risk는 검토 범주일 뿐 안전하다는 뜻의 값은 없다. 새 optional field도 정확한 구조에 의존하는 consumer에 영향을 줄 수 있다.

각 차이는 `code`, `risk`, `node{type, named}`, field 이름(`field`), 원소(`member`), 비교한 값 `before`/`after`, 원본 문서 안의 JSON pointer `baseline_path`/`candidate_path`를 가진다. 값은 원소를 `(type, named)`로, object key를 이름순으로 정렬한 canonical JSON이며 `null`은 그쪽에 없음을 뜻한다(형식이 `null`을 허용하지 않으므로 모호하지 않다). 없는 쪽의 path는 빈 문자열이다. 차이는 node(type bytes 오름차순, anonymous 먼저), 위 표의 code 순서, field, member 순서로 정렬한다. 입력을 바꾸면 추가와 제거, narrowed와 widened가 뒤집히고 같은 입력은 같은 결과다. identity는 `schema`(check) 또는 `baseline`·`candidate`(diff)의 원본 bytes sha256(schema `node-types-r1`)과 `policy`(`tsgk-schema-policy/r1` text: 연산, format, comparator, 한도)다. 같은 base name의 두 입력도 role과 hash로 구분한다.

**한도와 취소.** `schema` 연산 한도는 입력당 문서 16777216 bytes(`DOCUMENT_BYTES_LIMIT`), record 200000(node 항목·field·type 참조, `SCHEMA_RECORD_LIMIT`), decode한 JSON 값 8×record(`JSON_VALUE_LIMIT`), JSON 중첩 32(`JSON_DEPTH_LIMIT`), 출력 16777216 bytes(`OUTPUT_LIMIT`), kit wall 120초(`WALL_LIMIT`)이며 모두 `RESOURCE_LIMIT`이다. 취소는 JSON 값 4096개, record 1024개, 그래프 단계·비교 node 1024개마다 확인한다. 집합 정규화는 schema의 type·subtype·field 집합에만 쓰며 ordered tree·raw input·query capture 순서로 넓히지 않는다(S03-A12 회귀).

## Ordered tree

envelope는 `schema: tsgk-tree/r0`, `input`(`bytes`, `sha256`, `encoding: bytes`; 실사용 source의 판별 encoding·출처 필드는 S03 확장 revision), `status`, `capabilities`, `nodes`, `captures`와 producer/source/policy identity를 가진다. parse status는 COMPLETED/CANCELLED/RESOURCE_LIMIT/FAILED다. tree가 없으면 `nodes: null`이고 빈 성공 tree로 바꾸지 않는다. 완료된 tree는 최소 root 하나를 가진다.

nodes는 cursor preorder 배열이다. 각 항목은 `parent`(root -1, 나머지는 앞선 index), `type`(이름), `field`(부모 기준 이름 또는 null), `named`, `extra`, `is_error`, `has_error`, `is_missing`(bool), `start_byte`, `end_byte`, `start_point`, `end_point`를 가진다. point는 0-based `row`, byte 단위 `column`이다. 범위는 반열림 `[start,end)`이고 0≤start≤end≤input.bytes를 만족한다. 노드 배열과 sibling 순서, anonymous/extra, ERROR 내부 구조, zero-width·중복 범위를 보존한다. root start가 항상 0이라고 가정하지 않는다. numeric symbol/field ID를 runtime 간 의미 키로 쓰지 않는다.

완전한 tree는 root 하나, parent-before-child, 연속 preorder ancestry와 입력 안의 byte/point 범위를 검증한다. sibling span의 상호 배타성이나 range 유일성은 요구하지 않는다. 취소·한도·실패와 문법 오류를 가진 완료 tree를 구분한다.

captures는 query identity와 함께 원 producer의 ordered `match_index`, `pattern_index`, `capture_index`, `name`, `node_index`, byte/point range를 기록한다. duplicate capture와 같은 위치의 동률도 producer 순서를 보존한다. sorting/dedup/span 보정은 비교 전처리가 아니다. S06에서 Tree-sitter API ordering을 pinned version으로 검증해 comparator revision을 확정한다. runtime에 따라 순서를 보장할 수 없으면 해당 query comparison capability를 UNSUPPORTED로 둔다.

필수 비교 필드는 profile이 선택한다. adapter가 제공하지 못하는 필드는 null과 명시적 unsupported capability로 표시하고 false/0으로 채우지 않는다. strict profile 필드가 미지원이면 BLOCKED다. 부분 결과는 부분 관측만 주장한다. malformed 입력 bytes도 hash/길이를 원형대로 기록한다. edit offset/point 변환은 byte 기준이며 UTF-8 모드의 boundary 정책은 S05에서 fixture와 함께 고정한다.

## 실행 경계와 단계

native oracle은 `src/drivers/native-c/`의 generic C driver와 build-local language shim, 고정 runtime/parser/scanner source를 별도 executable로 build한다. ABI/header/compiler/options/source closure/executable hash를 모두 고정한다. [scanner 직렬화 계약](https://tree-sitter.github.io/tree-sitter/creating-parsers/4-external-scanners.html)은 stateful fixture의 필수 대조다.

S05 transport는 `tsgk-native/r1`의 process당 하나의 bounded stdin/stdout JSON request/response로 채택한다. 예외로 비공개 corpus batch는 한 process 안의 길이-접두 frame 연속이다. 원본·replacement는 base64 bytes이며 encoded/decoded 길이 한도를 모두 적용한다. shell 인자에 source bytes를 넣지 않는다. stdout은 protocol만, stderr는 bounded 진단이다. revision/length/record count, trailing JSON, truncation과 최종 completeness를 검증한다. 정확한 request/error 필드와 연산 한도는 S05 구현 전에 이 owner에서 고정하고 native round-trip으로 검증한다. 비공개 corpus batch([NET461 등록부](../validation/net461-workload.md))는 각 frame에 request/response 한도와 길이·record count·trailing 완결성 검증을 적용하며, 아래의 nonzero exit·truncated response 규칙도 frame 단위로 적용한다. 통과한 frame만 파일별 결과이고 batch process 결과는 따로 기록한다. envelope는 S04, frame 내용은 S05가 고정한다. Go adapter는 이번 campaign 밖이다.

S04의 검증된 공통 runner를 S05에서 재사용한다. S05는 최소 generic C driver, 엄격히 제한된 grammar-symbol shim, ordered public CST, byte-edit 및 incremental/fresh comparator를 구현한다. 이전 tree에 `ts_tree_edit`를 호출하고 이를 다음 parse에 실제 전달한 경로를 독립 instrumentation으로 검증한다. 각 중간 bytes를 별도의 fresh parser로 즉시 비교하며 마지막 상태만 검사하지 않는다. scannerless와 stateful scanner, malformed 중간 상태·inverse repair, EOF/CRLF/BOM/NUL/잘못된 UTF-8와 byte point 계산을 포함한다. source와 replacement를 자동 정규화하지 않는다.

S05 query capability는 아직 unsupported다. S06은 같은 driver/protocol에 query·public API 관측·완결된 기록을 추가하고 S05의 전체 영향 회귀를 재실행한다. 새 CLI 출력 해석 경로나 둘째 runner/driver를 만들지 않는다. parser/tree/cursor의 모든 error·cancel 경로에서 수명을 검증하며 nonzero exit·truncated response·cleanup 실패는 earlier tree와 무관하게 성공 완료를 막는다.

query predicate/directive를 host layer가 평가해야 하면 지원 subset을 명시한다. structural match를 완전히 평가된 capture로 바꾸어 말하지 않는다. capture의 node mapping에 `(type,start,end)` 유일성을 가정하지 않으며 duplicate/tie 순서를 보존한다. 반복·edit/fresh·old/candidate·cross-OS 비교마다 목적과 projection을 사전 고정하고 mismatch 후 normalization을 추가하지 않는다.

참고한 go-treesitter driver는 ordered Parent/Named/Extra/Missing/Error/range를 제공하지만 node별 has_error 등 kit 필드를 그대로 충족한다고 가정하지 않는다. 원 record를 adapter에서 변환할 때 누락 정보와 consumer 특유 root 가정을 드러낸다. [고정 출처](../provenance/upstream-sources.md)
