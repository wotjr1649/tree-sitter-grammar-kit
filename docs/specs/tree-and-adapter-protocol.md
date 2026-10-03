# Tree와 adapter — draft/r0

등록된 `.svc`는 [SVC-SERVICEHOST-r1](../validation/net461-workload.md)의 composite result를 사용한다. directive 관측과 원본 bytes를 included range로 파싱한 inline C# tree를 원본 identity·byte/point로 결속하며 segment PASS를 전체 파일 PASS로 바꾸지 않는다. S03이 record 모양 `tsgk-svc-composite/r1`을 고정하고 생산은 S05다(format 구현은 NOT_RUN).

* `schema`, `format: "SVC-SERVICEHOST-r1"`, `input`(아래 `tsgk-tree/r1`의 전체 파일 input), `identities`(producer·source·policy).
* `directive`: directive가 없으면 `null`이다. 있으면 전체 범위, `<%@`과 `%>`의 범위(`%>`가 없으면 `null`), `ServiceHost` 이름 범위, `attributes[{name, value, quote}]`(이름·값은 원본 범위, `quote`는 `"`·`'`·`null`), `diagnostics`(`DIRECTIVE_DUPLICATE`, `ATTRIBUTE_DUPLICATE`, `ATTRIBUTE_UNKNOWN`, `TERMINATOR_MISSING`, `QUOTE_UNTERMINATED`)다.
* `language{value, status}`: `value`는 Language 값 범위 또는 `null`, `status`는 `CSHARP`(`C#`/`c#`), `UNRESOLVED_LANGUAGE`(inline이 있는데 생략), `UNSUPPORTED_LANGUAGE`(그 밖의 값), `NOT_REQUIRED`(inline 없는 directive에서 생략)다.
* `code_behind`: CodeBehind 값 범위와 `resolution`(`NOT_RESOLVED` 또는 caller가 준 입력 identity를 가진 `CALLER_SUPPLIED`)이며 없으면 `null`이다. kit는 그 파일을 스스로 찾거나 읽지 않는다.
* `inline`: `status`가 `CSHARP`일 때만 `{included_ranges, tree}`이고 그 밖은 `null`이다. `tree`는 원본 좌표의 `tsgk-tree/r1` envelope 또는 `tsgk-tree-summary/r1`이다.
* `coverage{directive, code_behind, inline}`: 각각 `OBSERVED`, `ABSENT`, `UNRESOLVED`, `UNSUPPORTED`다. 모든 범위는 원본 byte와 0-based row·byte column이며 값 텍스트를 추출하지 않는다.

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

`schema check` 판정: error finding 중 위반이 하나라도 있으면 `FAIL`, 해석하지 않는 key(`SCHEMA_KEY_UNSUPPORTED`)만 있으면 `BLOCKED`, 그 밖은 `PASS`(warning 허용)이며 셋 다 `execution_status: COMPLETED`다. 항상 `SCHEMA_STATIC_ONLY` info finding을 낸다. 결과의 입력 요약은 `input{role, name, bytes, sha256, counts}`이며(E0의 `schema`는 report revision `tsgk-report/r1`), `counts`(node·named·anonymous·supertype·field·참조 수, root 목록)는 PASS일 때만 있고 그 밖은 `null`이다. finding path는 `이름#JSON pointer`다. 입력마다 finding은 앞의 1000건만 기록하고 넘치면 `FINDINGS_TRUNCATED` info로 전체 건수를 남긴다. 판정은 기록하지 않은 finding까지 포함해 계산한다.

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

**한도와 취소.** `schema` 연산 한도는 입력당 문서 16777216 bytes(`DOCUMENT_BYTES_LIMIT`), record 200000(node 항목·field·type 참조, `SCHEMA_RECORD_LIMIT`), decode한 JSON 값 8×record(`JSON_VALUE_LIMIT`), JSON 중첩 32(`JSON_DEPTH_LIMIT`), 출력 16777216 bytes(`OUTPUT_LIMIT`), kit wall 120초(`WALL_LIMIT`)이며 모두 `RESOURCE_LIMIT`이다. 8×record는 uint64를 넘으면 최댓값으로 포화한다. 문서 한도를 넘은 입력은 hash하지 않으므로 요약의 `bytes`는 0, `sha256`은 빈 값이고 그 입력의 identity는 없다(전체 파일을 넘긴 API 호출과 한도+1 bytes만 읽는 CLI가 같은 결과를 낸다). finding과 decoder 오류(`JSON_*`)의 path는 1024 bytes를 넘으면 잘라 `...(truncated)`를 붙인다. diff는 path를 만들기 전에 차이 크기를 보수적으로 추정하고(차이마다 고정 160 bytes + 문자열·path 길이, JSON escape 제외) 그 추정이 출력 한도를 넘으면 `OUTPUT_LIMIT`이다. 그래서 실제 JSON이 한도보다 조금 작은 결과도 거부될 수 있고, 추정이 한도 안이면 실제 JSON encoding을 다시 확인한다. decoder와 schema model은 값마다 pointer를 미리 만들지 않으므로 긴 member 이름이 하위 값마다 복사되지 않는다. 출력 한도를 넘는 실패 report(입력 오류 report 포함)는 실패 finding 하나만 담는다. 취소는 JSON 값 4096개, record 1024개, 그래프 단계·비교 node 1024개마다 확인한다. 집합 정규화는 schema의 type·subtype·field 집합에만 쓰며 ordered tree·raw input·query capture 순서로 넓히지 않는다(S03-A12 회귀).

### 선언·사실 mapping과 등록 사례 밖 구조 — S03 고정

[`declaration-facts-r1`](../../src/contracts/fact-mapping.json)은 채택 C#·T-SQL·PostgreSQL route의 사실 종류(`type_declaration`, `member_declaration`, `create_object`, `static_exec_target`, `command_text_site`, `command_text_literal`, `dynamic_sql_site`)를 각 route의 등록 upstream schema(repository·commit·path·bytes·sha256)의 named node와 locator(`node`, `field:F`, `child:T`, `children:T`의 `/` 경로)에 대응시키거나 이유를 붙여 `UNSUPPORTED`로 둔다. 같은 파일의 `dynamic-sql-r1`은 동적 SQL 구문 종류(`EXEC_PAREN`, `EXEC_PAREN_AT`, `SP_EXECUTESQL`, heuristic `CSHARP_COMMAND`), 동적이 아닌 `EXEC_MODULE_VARIABLE`(`EXEC @module_var`), 제외 인자(`AS USER/LOGIN`, pass-through 매개변수), 인자 종류 6가지와 그 node 대응, 알려진 누락(`AT DATA_SOURCE`, `WITH RESULT SETS`, `EXEC` 없는 batch 첫 호출)과 C# 오탐(`CommandType.StoredProcedure`)을 닫힌 목록으로 고정한다. XML 구조 mapping `xml-structure-r1`은 element 이름(`STag`·`EmptyElemTag`의 `Name`), attribute 이름과 값 범위(`Attribute`의 `Name`·`AttValue`), 등록 이름 목록에 해당하는 선언 참조 attribute를 범위로만 기록하며 값을 추출·해석·취득하지 않는다. `TestFactMapping`이 형식과 전수성을, `TestFactMappingAgainstSchemas`(로컬 schema 사본 필요)가 모든 locator와 동적 SQL이 의존하는 node·locator 목록(`requires`)이 그 schema에서 해석되는지 검사한다. `sp_executesql` 호출은 `static_exec_target`이 아니라 `dynamic_sql_site`만이다. mapping은 schema에서 도출한 계약이며 tree 위 동작은 S05(선언 순회)·S06(사실 query·동적 SQL 위치 추출)이 native로 검증한다. 채택 후보의 schema는 S04가 재생성하므로 아직 결속하지 않는다. PREPARE의 후보 native tree에는 upstream schema에 없는 node(C# `file_based_app_directive`, T-SQL `generated_always_clause`·`graph_table_type`)가 있어, S04는 재생성 schema를 `schema diff`로 upstream과 대조하고 mapping의 locator를 다시 검사해야 한다.

`known_gaps`가 S03에 배정한 항목의 구조 계약과 처분은 다음과 같다. 어느 항목도 전체 언어 지원 주장이 아니다.

| 항목 | 구조 계약 | 처분 |
|---|---|---|
| C# `async`·`var`·`await`를 식별자로 쓴 선언(등록 27사례 밖) | upstream schema에서 세 spelling은 anonymous token으로만 선언된다. 이름 위치에 쓰이면 mapping name locator가 가리키는 named `identifier`이고, 암시 형식 `var`는 `type` supertype의 `implicit_type`이다 | 등록 행(P05-CS-ID-ASYNC/AWAIT/VAR)만 후보 r5 native PASS다. 다른 위치(generic 인자, pattern, query 식 등)는 미검증으로 S05 fixture와 S08로 넘긴다 |
| C# file-based directive | `#!`은 upstream `shebang_directive`(field 없음)다. `#:` directive는 후보 r5 native tree(run 36917832850, 등록 directive 사례)에서 `file_based_app_directive{name: identifier, argument: preproc_arg}`이며 argument는 행 끝(EOL 제외)까지다 | upstream schema에 없으므로 S04 재생성 schema가 이 node와 두 field를 선언하는지 `schema check`·`schema diff`로 확인한다. SDK 의미(include 파일 읽기·restore)는 범위 밖(`csharp-V14c`) |
| T-SQL 등록 B01..B05·V16·V22 행 밖 | 최상위는 `program` → `batch` → `statement`/`go_statement`/`block`/`sqlcmd_*`이고 upstream `statement`는 151가지 statement node를 자식으로 선언한다. CREATE·EXEC 사실은 위 mapping이다 | 등록 행 밖 statement는 schema 선언만 있고 native 미검증이다. statement 종류별 지원은 주장하지 않으며 S05가 등록하는 fixture와 S08 판정으로 넘긴다. 알려진 EXEC 누락은 위 목록이다 |
| PostgreSQL 9.6–18 checkpoint | 후보는 PostgreSQL 19 기반 grammar의 legacy-r1 patch다. checkpoint 기능은 upstream schema의 node로 나타난다: V10 `CallStmt`·`generated_when`, V13 `MergeStmt`·`opt_search_clause`·`opt_cycle_clause`·`opt_materialized`, V16 `json_table`·`json_table_column_definition`, V18 `opt_virtual_or_stored`·`opt_without_overlaps`·`returning_with_clause`, L01 `OptWith`·`kw_oids` | 정적 schema는 "어느 version에서만 유효"를 표현하지 못하므로 version 경계(예: 9.6에서 `CALL` 거부)는 schema로 판정하지 않는다(UNSUPPORTED). 등록 12행만 native PASS이며 checkpoint별 fixture는 S05, 지원 claim은 S08이 판정한다 |

## Ordered tree

envelope는 `schema: tsgk-tree/r0`, `input`(`bytes`, `sha256`, `encoding: bytes`; 실사용 source의 판별 encoding·출처는 아래 `tsgk-tree/r1`), `status`, `capabilities`, `nodes`, `captures`와 producer/source/policy identity를 가진다. parse status는 COMPLETED/CANCELLED/RESOURCE_LIMIT/FAILED다. tree가 없으면 `nodes: null`이고 빈 성공 tree로 바꾸지 않는다. 완료된 tree는 최소 root 하나를 가진다.

nodes는 cursor preorder 배열이다. 각 항목은 `parent`(root -1, 나머지는 앞선 index), `type`(이름), `field`(부모 기준 이름 또는 null), `named`, `extra`, `is_error`, `has_error`, `is_missing`(bool), `start_byte`, `end_byte`, `start_point`, `end_point`를 가진다. point는 0-based `row`, byte 단위 `column`이다. 범위는 반열림 `[start,end)`이고 0≤start≤end≤input.bytes를 만족한다. 노드 배열과 sibling 순서, anonymous/extra, ERROR 내부 구조, zero-width·중복 범위를 보존한다. root start가 항상 0이라고 가정하지 않는다. numeric symbol/field ID를 runtime 간 의미 키로 쓰지 않는다.

완전한 tree는 root 하나, parent-before-child, 연속 preorder ancestry와 입력 안의 byte/point 범위를 검증한다. sibling span의 상호 배타성이나 range 유일성은 요구하지 않는다. 취소·한도·실패와 문법 오류를 가진 완료 tree를 구분한다.

captures는 query identity와 함께 원 producer의 ordered `match_index`, `pattern_index`, `capture_index`, `name`, `node_index`, byte/point range를 기록한다. duplicate capture와 같은 위치의 동률도 producer 순서를 보존한다. sorting/dedup/span 보정은 비교 전처리가 아니다. S06에서 Tree-sitter API ordering을 pinned version으로 검증해 comparator revision을 확정한다. runtime에 따라 순서를 보장할 수 없으면 해당 query comparison capability를 UNSUPPORTED로 둔다.

필수 비교 필드는 profile이 선택한다. adapter가 제공하지 못하는 필드는 null과 명시적 unsupported capability로 표시하고 false/0으로 채우지 않는다. strict profile 필드가 미지원이면 BLOCKED다. 부분 결과는 부분 관측만 주장한다. malformed 입력 bytes도 hash/길이를 원형대로 기록한다. edit offset/point 변환은 byte 기준이며 UTF-8 모드의 boundary 정책은 S05에서 fixture와 함께 고정한다.

### 입력 encoding r1과 대용량 summary — S03 고정, S05 생산

`tsgk-tree/r1`은 r0에서 `input`만 바꾼다. `input`은 `{bytes, sha256, encoding, encoding_source}`이고, `encoding`은 parser에 넣은 판별 encoding(`UTF-8`, `UTF-16LE`, `UTF-16BE`, `CP949`), `encoding_source`는 그 출처(`BOM`, `VALIDATION`, `DECLARATION`)다. 값은 S01 identity의 `EncodingOutcome`과 같고 `PASS`가 아닌 파일은 parse 입력을 만들지 않으므로 envelope가 없다. `bytes`·`sha256`은 변환하지 않은 원본 bytes의 값이며 point column은 원본 행 시작부터의 byte 수다. r0의 `encoding: bytes`는 r0에만 쓴다.

[실사용 source 정책](../validation/net461-workload.md)이 summary를 요구하면(descendant 50000 초과 또는 출력 16777216 bytes 초과) `tsgk-tree-summary/r1`을 낸다. 필드는 정확히 다음과 같다([예시](../../src/contracts/examples/tree-summary-r1.json), `TestTreeSummaryExample`이 key 집합과 값 규칙을 검사한다).

* `schema`, `input`(위 r1), `status`(parse status), `identities`(E0 `IdentityRef` 배열: producer·source·policy), `reason`(`DESCENDANT_LIMIT` 또는 `OUTPUT_LIMIT`), `descendant_count`.
* `digest{canonicalization: "tsgk-tree-digest/r1", sha256}`: preorder node마다 `parent`(root -1), `type`과 `field`(없으면 빈 값)를 각각 10진 byte 길이 + `:` + bytes로, flag 다섯 개(`named`, `extra`, `is_error`, `has_error`, `is_missing`)를 `0`/`1`로, `start_byte`, `end_byte`, 시작·끝 point의 row·column을 10진수로 써 NUL로 잇고 줄 끝에 LF를 붙인다. preimage는 ASCII `tsgk-tree-digest/r1\n` 뒤에 이 줄들을 순서대로 이은 것이다. 같은 bytes·grammar·producer의 전체 tree와 summary는 같은 digest를 가진다.
* `errors{limit: 1000, total, truncated, items}`: preorder 순서의 ERROR·MISSING node이며 각 item은 `kind`(`ERROR`|`MISSING`), `type`, byte·point 범위다. `truncated`는 `total`이 item 수보다 클 때만 true다. 오류 개수만으로 구조 PASS가 아니다.
* `declarations{mapping, route, assessment, items}`: S05가 [선언·사실 mapping](../../src/contracts/fact-mapping.json)의 선언 node 종류(`type_declaration`, `member_declaration`, `create_object`)를 query 없이 순회한 결과다. item은 `fact`, `node_type`, 범위, `name`(name locator가 가리키는 범위 `{start_byte, end_byte}` 또는 locator가 `node`면 `null`), `status`(`PASS`, `NAME_MISSING`, `HAS_ERROR`)다. `children:` locator가 이름을 여러 개 내면(`int a, b;`) 이름마다 item을 하나씩 같은 node 범위로 내고, 하나도 내지 못하면 `name: null`·`NAME_MISSING` item 하나다. 모든 item이 `PASS`면 `PASS`, 하나라도 아니면 `FAIL`, 선언이 하나도 없으면 `NOT_APPLICABLE`, mapping이 없는 route면 `NOT_ASSESSED`다. S06의 사실 query는 같은 item을 재현해야 한다.
* `partial_trees[{point, byte, truncated, nodes}]`: workload가 미리 등록한 지점(`point` id, 원본 byte offset)마다 그 byte를 덮는 가장 깊은 node의 조상 사슬과 그 node의 subtree를 ordered tree node 모양으로 담는다. `parent`는 이 배열 안의 index이고 맨 위 조상이 -1이다. 한 지점에 1000 node를 넘으면 preorder 앞 1000개만 담고 `truncated: true`다.

summary와 envelope는 범위만 담는다. source·XML 값·이름의 텍스트를 추출하지 않으며 이름도 byte 범위로만 기록한다. 비공개 corpus의 summary는 로컬 기록에만 둔다.

## 실행 경계와 단계

native oracle은 `src/drivers/native-c/`의 generic C driver와 build-local language shim, 고정 runtime/parser/scanner source를 별도 executable로 build한다. ABI/header/compiler/options/source closure/executable hash를 모두 고정한다. [scanner 직렬화 계약](https://tree-sitter.github.io/tree-sitter/creating-parsers/4-external-scanners.html)은 stateful fixture의 필수 대조다.

S05 transport는 `tsgk-native/r1`의 process당 하나의 bounded stdin/stdout JSON request/response로 채택한다. 예외로 비공개 corpus batch는 한 process 안의 길이-접두 frame 연속이다. 원본·replacement는 base64 bytes이며 encoded/decoded 길이 한도를 모두 적용한다. shell 인자에 source bytes를 넣지 않는다. stdout은 protocol만, stderr는 bounded 진단이다. revision/length/record count, trailing JSON, truncation과 최종 completeness를 검증한다. 정확한 request/error 필드와 연산 한도는 S05 구현 전에 이 owner에서 고정하고 native round-trip으로 검증한다. 비공개 corpus batch([NET461 등록부](../validation/net461-workload.md))는 각 frame에 request/response 한도와 길이·record count·trailing 완결성 검증을 적용하며, 아래의 nonzero exit·truncated response 규칙도 frame 단위로 적용한다. 통과한 frame만 파일별 결과이고 batch process 결과는 따로 기록한다. envelope는 S04, frame 내용은 S05가 고정한다. Go adapter는 이번 campaign 밖이다.

S04의 검증된 공통 runner를 S05에서 재사용한다. S05는 최소 generic C driver, 엄격히 제한된 grammar-symbol shim, ordered public CST, byte-edit 및 incremental/fresh comparator를 구현한다. 이전 tree에 `ts_tree_edit`를 호출하고 이를 다음 parse에 실제 전달한 경로를 독립 instrumentation으로 검증한다. 각 중간 bytes를 별도의 fresh parser로 즉시 비교하며 마지막 상태만 검사하지 않는다. scannerless와 stateful scanner, malformed 중간 상태·inverse repair, EOF/CRLF/BOM/NUL/잘못된 UTF-8와 byte point 계산을 포함한다. source와 replacement를 자동 정규화하지 않는다.

S05 query capability는 아직 unsupported다. S06은 같은 driver/protocol에 query·public API 관측·완결된 기록을 추가하고 S05의 전체 영향 회귀를 재실행한다. 새 CLI 출력 해석 경로나 둘째 runner/driver를 만들지 않는다. parser/tree/cursor의 모든 error·cancel 경로에서 수명을 검증하며 nonzero exit·truncated response·cleanup 실패는 earlier tree와 무관하게 성공 완료를 막는다.

query predicate/directive를 host layer가 평가해야 하면 지원 subset을 명시한다. structural match를 완전히 평가된 capture로 바꾸어 말하지 않는다. capture의 node mapping에 `(type,start,end)` 유일성을 가정하지 않으며 duplicate/tie 순서를 보존한다. 반복·edit/fresh·old/candidate·cross-OS 비교마다 목적과 projection을 사전 고정하고 mismatch 후 normalization을 추가하지 않는다.

참고한 go-treesitter driver는 ordered Parent/Named/Extra/Missing/Error/range를 제공하지만 node별 has_error 등 kit 필드를 그대로 충족한다고 가정하지 않는다. 원 record를 adapter에서 변환할 때 누락 정보와 consumer 특유 root 가정을 드러낸다. [고정 출처](../provenance/upstream-sources.md)

## S05 구현 — `tsgk-native/r1` driver protocol과 incremental 비교

S05가 구현 전에 고정한 driver protocol·edit·encoding·한도·상태·비교 projection이다. 위 절의 "process당 하나의 JSON request/response"는 아래 frame 하나로 구체화한다. driver source는 `src/drivers/native-c/`, 실행은 S04 runner, 비교와 판정은 Go(`src/kit`의 tree 비교·edit 검사, `src/internal/native`의 build·실행)다. query는 S06까지 지원하지 않는다(요청에 query 필드가 없고 응답 `producer.query`가 `UNSUPPORTED`).

**Frame.** stdin·stdout 모두 4-byte big-endian 길이 + payload의 frame이다. driver의 유일한 인자는 `single` 또는 `batch`다. `single`은 request frame 하나를 읽은 뒤 stdin이 EOF여야 하고(`FRAME_EXTRA`), response frame 하나를 쓰고 끝난다. `batch`(비공개 corpus 전용)는 frame 경계의 EOF까지 request를 하나씩 읽고 순서대로 response를 하나씩 쓴다. 머리 4 bytes가 모자라거나 payload가 선언 길이보다 짧으면 `FRAME_TRUNCATED`, 선언 길이가 50331648 bytes를 넘으면 payload를 읽기 전에 `FRAME_TOO_LARGE`다. response payload는 요청의 `output_bytes` 이하다. Go는 single에서 정확히 한 response와 그 뒤 stdout 0 bytes를, batch에서 request 수만큼의 response와 그 뒤 0 bytes를 요구한다. 남은 bytes는 `RESPONSE_TRAILING_BYTES`이고 성공이 아니다. stderr는 상한 있는 진단일 뿐 protocol이 아니다.

**Request.** payload는 아래 key 순서와 공백 없는 canonical JSON이며 driver는 이 형태만 받는다(그 밖은 `REQUEST_MALFORMED`). 문자열은 escape 없는 출력 가능 ASCII(`"`·`\` 제외)이고 정수는 선행 0 없는 10진수다.

```text
{"protocol":"tsgk-native/r1","id":ID,"encoding":ENC,"output":OUT,
 "limits":{"input_bytes":N,"nodes":N,"full_nodes":N,"depth":N,"output_bytes":N,"parse_ms":N,"memory_bytes":N,"errors":N,"partial_nodes":N},
 "declarations":[{"fact":S,"node":S,"name":S},...],"points":[{"id":S,"byte":N},...],
 "ranges":[[{"start_byte":N,"end_byte":N,"start_point":[R,C],"end_point":[R,C]},...],...],
 "source":B64,"edits":[{"start_byte":N,"old_end_byte":N,"new_end_byte":N,"old":B64,"new":B64},...]}
```

* `id`는 1~128자의 `[A-Za-z0-9._:-]`, `ENC`는 `UTF-8`·`UTF-16LE`·`UTF-16BE`·`CP949`, `OUT`은 `tree`(전체 tree, edit 허용), `auto`(아래 full tree gate), `record`(비공개 corpus 보존 레코드)다. edit는 `tree`에서만, 최대 4개다(`EDITS_NOT_ALLOWED`, `EDIT_COUNT_LIMIT`).
* driver 고정 상한: `input_bytes` ≤ 33554432, `output_bytes` ≤ 16777216, `nodes` ≤ 25000000, `full_nodes` ≤ `nodes`, `depth` ≤ 100000, `parse_ms` ≤ 60000, `memory_bytes` ≤ 4294967296, `errors` ≤ 1000, `partial_nodes` ≤ 1000, `declarations` ≤ 64개(각 문자열 1~128 bytes), `points` ≤ 64개. 0이거나 넘으면 `LIMIT_INVALID`다. base64는 패딩 있는 표준 alphabet만 받는다(`BASE64_INVALID`). 길이가 `input_bytes`를 넘으면 `INPUT_TOO_LARGE`다.
* 선언 `name` locator는 [fact mapping](../../src/contracts/fact-mapping.json)의 `node`, `field:F`, `child:T`, `children:T`를 `/`로 이은 경로다. `field:F`는 그 field의 첫 child, `child:T`는 type T인 첫 named child, `children:T`는 그런 named child 전부이며 경로 중 `children:`이 이름을 여러 개 내면 이름마다 item 하나다(`LOCATOR_INVALID`). `points`의 byte를 덮는 node가 없으면 그 지점의 `nodes`는 빈 배열이다.
* `ranges`는 비어 있거나(전체 입력) step마다 하나씩(edit 수 + 1) 있는 included range 목록이다. 각 목록은 비어 있지 않고 최대 16개이며, 그 step source 안에서 겹치지 않게 오름차순이고 point는 driver 계산과 같아야 한다(`RANGES_INVALID`). driver는 parse마다 그 step의 목록을 parser A·B에 `ts_parser_set_included_ranges`로 설정하므로 결과 tree는 원본 좌표다. `.svc` inline C#(아래)만 이 필드를 쓴다.

**Encoding과 edit 경계.** source는 변환하지 않은 원본 bytes다. `UTF-8`은 어떤 bytes(NUL·BOM·잘못된 sequence 포함)도 받고, `UTF-16LE/BE`는 짝수 길이·짝 맞는 surrogate·U+0000 없음, `CP949`는 ASCII 또는 lead 0x81–0xFE + trail 0x41–0xFE이면서 WHATWG `index-euc-kr`에 code point가 있는 쌍만 받는다(`SOURCE_ENCODING_INVALID`). `CP949`는 runtime의 custom decode callback(표 lookup)으로, 나머지는 runtime 내장 decode로 파싱한다. read callback은 요청 byte부터 남은 buffer 전체를 돌려준다. edit 경계 정책:

* 공통: `0 ≤ start_byte ≤ old_end_byte ≤ 현재 길이`이고 uint32 안(`EDIT_RANGE`), `old`의 bytes가 현재 source의 `[start_byte, old_end_byte)`와 같고(`EDIT_OLD_MISMATCH`), `new_end_byte = start_byte + len(new)`(`EDIT_LENGTH_INCONSISTENT`), `old`와 `new`가 모두 비면 `EDIT_EMPTY`, 결과 길이가 `input_bytes`를 넘으면 `INPUT_TOO_LARGE`다.
* `UTF-8`: 경계가 올바른 다중 byte 문자(Go `utf8.DecodeRune`이 크기 2 이상의 유효 문자로 읽는 것)의 중간이면 `EDIT_SPLITS_CHARACTER`다. 잘못된 byte는 1 byte 단위이므로 그 앞뒤 경계는 받는다(교정하지 않음).
* `UTF-16LE/BE`: 홀수 offset은 `EDIT_ODD_UTF16`, surrogate 쌍 중간은 `EDIT_SPLITS_CHARACTER`, 결과 source가 위 UTF-16 조건을 깨면 `EDIT_ENCODING_INVALID`다.
* `CP949`: lead와 trail 사이 경계는 `EDIT_SPLITS_CHARACTER`, 결과 source가 표 조건을 깨면 `EDIT_ENCODING_INVALID`다.
* 경계는 적용 전 source에서 `start_byte`·`old_end_byte`, 적용 후 source에서 `start_byte`·`new_end_byte`를 본다. driver는 모든 edit를 순서대로 bytes에 적용해 보며 검사를 끝낸 뒤에야 첫 parse를 시작하므로 거부된 요청은 native 상태를 만들지 않는다. Go(`kit.ApplyEdits`)가 같은 검사를 먼저 하고 driver가 다시 한다.
* point: row는 그 offset 앞의 LF 개수(UTF-8·CP949는 byte 0x0A, UTF-16은 짝수 offset의 code unit 0x000A), column은 마지막 LF 다음 byte부터 그 offset까지의 byte 수다. Unicode scalar 수를 세지 않는다. CR은 보통 byte다.

**실행 순서와 route 계측.** parser A로 초기 source를 parse한다(step 0, `fresh: null`). edit k마다 이전 tree에 `ts_tree_edit`를 호출하고, 그 직후 이전 root의 `has_changes`와 end byte를 기록한 뒤, source bytes를 바꾸고 parser A에 그 이전 tree를 넘겨 incremental tree를 만든다. 이어 old tree 없이 별도 parser B로 같은 bytes를 parse해 fresh tree를 만들고, 다음 edit 전에 두 tree를 직렬화한다. 이전 tree·fresh tree·parser는 성공·오류·취소 경로마다 해제한다. route 계측은 driver가 tree에서 관측한 값이며 자기 보고 flag가 아니다: `edit_has_changes`, `edited_root_end_byte`, `reused_nodes`(incremental tree node 중 edit된 이전 tree의 node와 runtime node identity, 즉 공개 `TSNode.id`가 같은 수. id는 부모 안의 subtree slot이므로 다시 만든 부모 바로 아래에서 재사용된 leaf는 세지 않고 재사용된 subtree 안의 node만 센다. 그래서 하한이다), `fresh_reused_nodes`(fresh tree에 대한 같은 수, 0이어야 함). identity 값은 출력하지 않으며 runtime 간 의미 key가 아니다. 재사용 수는 경로 증거일 뿐 재사용률·성능 주장이 아니다.

**Tree 직렬화.** 반복 `TSTreeCursor` preorder이며 depth는 root를 1로 센 node 깊이다. `depth`를 넘으면 `DEPTH_LIMIT`, node 수가 `nodes`를 넘으면 `NODE_LIMIT`이며 둘 다 그 tree의 `RESOURCE_LIMIT`이다. full tree는 `types`·`fields` 이름 표와 node마다 `[parent, type, field, flags, start_byte, end_byte, start_row, start_column, end_row, end_column]`이다(`field`는 표 index 또는 -1, `flags`는 named 1·extra 2·is_error 4·has_error 8·is_missing 16). 모든 완료 form은 `descendant_count`, `max_depth`, `has_error`와 위 `tsgk-tree-digest/r1` digest를 가진다. Go는 full tree의 digest를 다시 계산해 대조한다(`TREE_DIGEST_MISMATCH`). `auto`는 `descendant_count ≤ full_nodes`이고 full 직렬화가 `output_bytes` 안이면 full, 아니면 summary(`reason` `DESCENDANT_LIMIT` 또는 `OUTPUT_LIMIT`; errors·declarations·partial_trees)다. `record`는 summary에서 partial tree를 뺀 보존 레코드다. 요청에 선언이 있으면 full tree에도 declarations를 붙인다. Go는 full을 `tsgk-tree/r1`, summary를 `tsgk-tree-summary/r1`로 바꾼다.

**`.svc` composite 생산(`SVC-SERVICEHOST-r1`).** 위 첫 절의 `tsgk-svc-composite/r1`을 S05가 만든다. `kit.ObserveServiceHost`가 판별 encoding의 code unit으로 첫 `<%@`부터 directive를 offline으로 관측한다(이름은 `<%@` 뒤 첫 단어, attribute는 `이름 = 값`이며 값은 `"`·`'` quote 또는 공백·`%>` 전까지, 알려진 이름은 대소문자를 가리지 않는 `Service`·`Factory`·`Debug`·`Language`·`CodeBehind`). `%>`가 없으면 끝까지가 directive이고 `TERMINATOR_MISSING`, 닫는 quote가 없으면 `QUOTE_UNTERMINATED`, 같은 이름은 `ATTRIBUTE_DUPLICATE`, 그 밖의 이름은 `ATTRIBUTE_UNKNOWN`, 첫 directive 뒤의 다른 `<%@`는 `DIRECTIVE_DUPLICATE`다. directive 뒤에 공백이 아닌 내용이 있으면 inline이며, Language 값이 `C#`·`c#`이면 그 뒤 원본 전체가 included range 하나다. 값 텍스트는 판정에만 쓰고 결과에는 범위만 남긴다. 사례의 모든 step에 C# inline이 있으면 step마다 다시 관측한 range를 `ranges`로 보내 inline만 C# grammar로 parse하고 step마다 composite에 inline tree를 붙인다. 어느 step이든 C# inline이 없으면 driver를 실행하지 않고 composite(관측만)를 남기며, 첫 step이 C# inline이었거나 기대값이 있으면 `SVC_INLINE_NOT_PARSED`·`BLOCKED`다. directive만 있는 파일은 관측 완료(PASS)다.

**Response.**

```text
{"protocol":"tsgk-native/r1","id":ID,"status":S,"code":C,"producer":{"language_version":N,"runtime_language_version":15,"runtime_min_compatible":13,"query":"UNSUPPORTED"},
 "source_bytes":N,"steps_completed":N,"steps":[{"step":K,"source_bytes":N,"source_sha256":H,"edit":E|null,"route":R|null,"incremental":T,"fresh":T|null},...],"complete":true}
```

`edit`은 요청의 세 byte 값과 driver가 계산한 `start_point`·`old_end_point`·`new_end_point`(`[row,column]`)다. Go는 자기 계산과 다르면 `EDIT_POINT_MISMATCH`, step의 `source_sha256`이 자기 중간 bytes와 다르면 `SOURCE_TRANSPORT_MISMATCH`다. tree는 `status`·`code`·`parse_ms`·`form`과 form별 필드다. 완료되지 않은 tree는 `form: null`이고 node가 없다(빈 성공 tree로 바꾸지 않음).

| 상황 | status, code | driver exit |
|---|---|---|
| 정상 | `COMPLETED`, `""` | 0 |
| request·frame·edit 위반 | response `INVALID_REQUEST`, 위 code; step 없음 | 2 |
| `parse_ms`(실사용 60초)를 넘어 progress callback이 parse를 취소 | tree·response `RESOURCE_LIMIT`, `PARSE_TIME_LIMIT` | 3 |
| node·depth 상한, response가 `output_bytes`를 넘음 | `RESOURCE_LIMIT`, `NODE_LIMIT`·`DEPTH_LIMIT`·`OUTPUT_LIMIT` | 3 |
| allocator hook의 누적 할당이 `memory_bytes`를 넘거나 할당 실패 | `RESOURCE_LIMIT`, `ALLOCATION_LIMIT`(미리 만든 frame을 쓰고 즉시 종료, batch에서도 치명) | 3 |
| runtime이 취소 없이 null tree를 냄 | `FAILED`, `PARSE_NULL` | 4 |

response status는 처음으로 완료되지 않은 tree의 status·code이며 그 뒤 step은 실행하지 않는다. 출력 상한을 넘으면 step 없이 `OUTPUT_LIMIT`과 `steps_completed`만 낸다. 이미 직렬화한 step은 부분 관측으로 보존할 뿐 완료 근거가 아니다. batch에서 치명이 아닌 frame 실패는 그 frame의 response이고 process는 다음 frame을 계속 읽으며, 끝까지 처리하면 exit 0이다. caller 취소와 runner 한도(wall, memory, stdout)는 response 없이 runner 결과로 `CANCELLED`·`RESOURCE_LIMIT`이다. Go는 runner `COMPLETED`·cleanup verified·exit code와 response status의 일치·`complete: true`·step 수(완료면 edit 수 + 1)·node graph(root 하나, `parent < index`이며 직전 node 또는 그 조상, byte·point 범위가 입력 안이고 start ≤ end)·이름 표 index를 모두 확인한 뒤에만 결과를 쓴다. 하나라도 어긋나면 earlier step이 정상이어도 `FAILED`(`RESPONSE_INVALID`)다.

**비교 projection과 판정.** 하나의 비교 함수(`kit.CompareTrees`)가 같은 bytes·grammar·policy의 두 full tree를 node 수와 node별 12개 공개 필드(type·field는 표 index가 아니라 이름) 전부로, 정렬·중복 제거·span 보정 없이 preorder 순서대로 비교하고 첫 차이(node index, 필드, 두 값)를 낸다. 사례마다 claim 세 개를 따로 둔다. `incremental_equality`는 모든 edit step에서 incremental = fresh, `incremental_route`는 모든 edit step에서 `edit_has_changes`·`reused_nodes > 0`·`fresh_reused_nodes = 0`, `expectations`는 등록 사례의 언어 기대값(step별 `NO_ERROR`·`ERROR`와 있어야 할 named node type)이다. native/fresh 일치는 언어 정확성이 아니며, 기대값 불일치는 kit가 충실히 보고한 grammar 결과라면 grammar gap으로 처분한다. 어느 claim이든 FAIL이면 그 사례의 assessment는 FAIL이다. 재사용할 node가 없는 edit는 route를 증명하지 못하므로 등록 사례는 바뀌지 않는 문맥을 남긴다.
