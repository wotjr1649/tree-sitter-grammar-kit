# Tree와 adapter — draft/r0

S03의 정적 node-types 비교와 S05부터 생성하는 runtime CST는 다른 주장이다. schema 일치, ordered CST 일치, query 일치, 언어 사양 적합성은 서로 대체하지 않는다. 아래 형식은 experimental이며 실제 native/두 번째 grammar/consumer 검증 전에 안정 API로 고정하지 않는다.

## Ordered tree

envelope는 `schema: tsgk-tree/r0`, `input`(`bytes`, `sha256`, `encoding: bytes`), `status`, `capabilities`, `nodes`, `captures`와 producer/source/policy identity를 가진다. parse status는 COMPLETED/CANCELLED/RESOURCE_LIMIT/FAILED다. tree가 없으면 `nodes: null`이고 빈 성공 tree로 바꾸지 않는다. 완료된 tree는 최소 root 하나를 가진다.

nodes는 cursor preorder 배열이다. 각 항목은 `parent`(root -1, 나머지는 앞선 index), `type`(이름), `field`(부모 기준 이름 또는 null), `named`, `extra`, `is_error`, `has_error`, `is_missing`(bool), `start_byte`, `end_byte`, `start_point`, `end_point`를 가진다. point는 0-based `row`, byte 단위 `column`이다. 범위는 반열림 `[start,end)`이고 0≤start≤end≤input.bytes를 만족한다. 노드 배열과 sibling 순서, anonymous/extra, ERROR 내부 구조, zero-width·중복 범위를 보존한다. root start가 항상 0이라고 가정하지 않는다. numeric symbol/field ID를 runtime 간 의미 키로 쓰지 않는다.

완전한 tree는 root 하나, parent-before-child, 연속 preorder ancestry와 입력 안의 byte/point 범위를 검증한다. sibling span의 상호 배타성이나 range 유일성은 요구하지 않는다. 취소·한도·실패와 문법 오류를 가진 완료 tree를 구분한다.

captures는 query identity와 함께 원 producer의 ordered `match_index`, `pattern_index`, `capture_index`, `name`, `node_index`, byte/point range를 기록한다. duplicate capture와 같은 위치의 동률도 producer 순서를 보존한다. sorting/dedup/span 보정은 비교 전처리가 아니다. S06에서 Tree-sitter API ordering을 pinned version으로 검증해 comparator revision을 확정한다. runtime에 따라 순서를 보장할 수 없으면 해당 query comparison capability를 UNSUPPORTED로 둔다.

필수 비교 필드는 profile이 선택한다. adapter가 제공하지 못하는 필드는 null과 명시적 unsupported capability로 표시하고 false/0으로 채우지 않는다. strict profile 필드가 미지원이면 BLOCKED다. 부분 결과는 부분 관측만 주장한다. malformed 입력 bytes도 hash/길이를 원형대로 기록한다. edit offset/point 변환은 byte 기준이며 UTF-8 모드의 boundary 정책은 S05에서 fixture와 함께 고정한다.

## 실행 경계와 단계

native oracle은 `src/drivers/native-c/`의 generic C driver와 build-local language shim, 고정 runtime/parser/scanner source를 별도 executable로 build한다. ABI/header/compiler/options/source closure/executable hash를 모두 고정한다. [scanner 직렬화 계약](https://tree-sitter.github.io/tree-sitter/creating-parsers/4-external-scanners.html)은 stateful fixture의 필수 대조다.

S05 transport는 `tsgk-native/r1`의 process당 하나의 bounded stdin/stdout JSON request/response로 채택한다. 원본·replacement는 base64 bytes이며 encoded/decoded 길이 한도를 모두 적용한다. shell 인자에 source bytes를 넣지 않는다. stdout은 protocol만, stderr는 bounded 진단이다. revision/length/record count, trailing JSON, truncation과 최종 completeness를 검증한다. 정확한 request/error 필드와 연산 한도는 S05 구현 전에 이 owner에서 고정하고 native round-trip으로 검증한다. Go adapter는 이번 campaign 밖이다.

S04의 검증된 공통 runner를 S05에서 재사용한다. S05는 최소 generic C driver, 엄격히 제한된 grammar-symbol shim, ordered public CST, byte-edit 및 incremental/fresh comparator를 구현한다. 이전 tree에 `ts_tree_edit`를 호출하고 이를 다음 parse에 실제 전달한 경로를 독립 instrumentation으로 검증한다. 각 중간 bytes를 별도의 fresh parser로 즉시 비교하며 마지막 상태만 검사하지 않는다. scannerless와 stateful scanner, malformed 중간 상태·inverse repair, EOF/CRLF/BOM/NUL/잘못된 UTF-8와 byte point 계산을 포함한다. source와 replacement를 자동 정규화하지 않는다.

S05 query capability는 아직 unsupported다. S06은 같은 driver/protocol에 query·public API 관측·완결된 기록을 추가하고 S05의 전체 영향 회귀를 재실행한다. 새 CLI 출력 해석 경로나 둘째 runner/driver를 만들지 않는다. parser/tree/cursor의 모든 error·cancel 경로에서 수명을 검증하며 nonzero exit·truncated response·cleanup 실패는 earlier tree와 무관하게 성공 완료를 막는다.

query predicate/directive를 host layer가 평가해야 하면 지원 subset을 명시한다. structural match를 완전히 평가된 capture로 바꾸어 말하지 않는다. capture의 node mapping에 `(type,start,end)` 유일성을 가정하지 않으며 duplicate/tie 순서를 보존한다. 반복·edit/fresh·old/candidate·cross-OS 비교마다 목적과 projection을 사전 고정하고 mismatch 후 normalization을 추가하지 않는다.

참고한 go-treesitter driver는 ordered Parent/Named/Extra/Missing/Error/range를 제공하지만 node별 has_error 등 kit 필드를 그대로 충족한다고 가정하지 않는다. 원 record를 adapter에서 변환할 때 누락 정보와 consumer 특유 root 가정을 드러낸다. [고정 출처](../provenance/upstream-sources.md)
